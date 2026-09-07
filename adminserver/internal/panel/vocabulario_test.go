package panel

import (
	"errors"
	"io/fs"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
)

// The panel's shared vocabulary, checked against the templates themselves.
//
// These are the rules the panel felt worst for not having: every screen decided
// the same five situations on its own, so nothing was where you expected it.
// Fixing that once is easy; keeping it fixed is the part that needs a test,
// because the next page is written by copying whichever page happens to be open.

// verbosPerigosos are the words that mean the button destroys something. A
// button carrying one of them has to look dangerous.
var verbosPerigosos = regexp.MustCompile(
	`(?i)\b(apagar|excluir|remover|banir|bloquear|desligar|limpar|zerar|reiniciar|derrubar|descartar)\b`)

// botao matches one button element and splits its attributes from its label.
var botao = regexp.MustCompile(`(?s)<button([^>]*)>(.*?)</button>`)

func lerTemplates(t *testing.T) map[string]string {
	t.Helper()
	nomes, err := fs.Glob(uiFS, "ui/*.html")
	if err != nil {
		t.Fatalf("listar templates: %v", err)
	}
	if len(nomes) == 0 {
		t.Fatal("nenhum template embutido; a varredura quebrou")
	}
	out := map[string]string{}
	for _, n := range nomes {
		b, err := fs.ReadFile(uiFS, n)
		if err != nil {
			t.Fatalf("ler %s: %v", n, err)
		}
		out[n] = string(b)
	}
	return out
}

// TestAcaoPerigosaTemCaraDePerigosa: apagar, banir e desligar são as ações que
// não têm desfazer, e um botão que apaga com a mesma cara de um que busca é
// como se perde um dado por engano.
func TestAcaoPerigosaTemCaraDePerigosa(t *testing.T) {
	// Desbloquear e desatolar carregam um verbo perigoso e desfazem um dano em
	// vez de causá-lo; marcá-los de vermelho gastaria o sinal à toa.
	excecoes := []string{"desbloquear", "desatolar"}

	for nome, corpo := range lerTemplates(t) {
		for _, m := range botao.FindAllStringSubmatch(corpo, -1) {
			attrs, rotulo := m[1], strings.TrimSpace(m[2])
			if !verbosPerigosos.MatchString(rotulo) {
				continue
			}
			manso := false
			for _, e := range excecoes {
				if strings.Contains(strings.ToLower(rotulo), e) {
					manso = true
				}
			}
			if manso || strings.Contains(attrs, "perigo") {
				continue
			}
			t.Errorf("%s: o botão %q apaga ou desliga algo e não tem class=\"perigo\"",
				nome, primeiraLinha(rotulo))
		}
	}
}

// TestNinguemInventaAPropriaFraseDeQuandoVale: havia oito redações para três
// situações, e descobrir em qual delas você estava era parte do trabalho. A
// resposta agora mora num bloco só.
func TestNinguemInventaAPropriaFraseDeQuandoVale(t *testing.T) {
	// Redações que existiam antes do bloco. Se uma voltar, é porque alguém
	// copiou de uma página velha em vez de usar o bloco.
	proibidas := []string{
		"só vale depois de reiniciar o servidor",
		"Também só vale depois de reiniciar",
		"entram sozinhos em até 15 segundos",
		"em até 15 segundos, sem reiniciar",
		"lê estas tabelas quando liga",
		"lê estas curvas quando liga",
	}
	for nome, corpo := range lerTemplates(t) {
		if nome == "ui/_shared.html" {
			continue // é onde o bloco mora
		}
		baixo := strings.ToLower(corpo)
		for _, p := range proibidas {
			if strings.Contains(baixo, strings.ToLower(p)) {
				t.Errorf("%s traz a redação própria %q; use {{template \"quando-vale\" ...}}", nome, p)
			}
		}
	}
}

// TestQuandoValeSoUsaOsTresEstados: um quarto estado inventado num template
// renderiza o ramo "reiniciar" sem reclamar, porque o bloco cai nele por
// padrão — ou seja, uma tela que grava na hora diria para reiniciar.
func TestQuandoValeSoUsaOsTresEstados(t *testing.T) {
	uso := regexp.MustCompile(`{{template\s+"quando-vale"\s+"([^"]*)"`)
	validos := map[string]bool{"agora": true, "segundos": true, "reiniciar": true}
	achou := 0
	for nome, corpo := range lerTemplates(t) {
		for _, m := range uso.FindAllStringSubmatch(corpo, -1) {
			achou++
			if !validos[m[1]] {
				t.Errorf("%s pede o estado %q, que não existe", nome, m[1])
			}
		}
	}
	if achou == 0 {
		t.Error("nenhuma página usa o bloco; ou ele sumiu, ou a varredura quebrou")
	}
}

// TestTodaTelaQueGravaDizQuandoVale cobre as telas de conteúdo — as que editam
// o que o jogo carrega. Elas são as que mais confundem, porque o resultado da
// gravação não aparece no jogo na mesma hora.
func TestTodaTelaQueGravaDizQuandoVale(t *testing.T) {
	// A página da conta fica de fora de propósito: a resposta dela depende do
	// caso — bloquear derruba quem está online SE o painel falar com o jogo, e
	// só impede o próximo login se não falar —, e ela já diz isso na mensagem
	// de cada ação. Um aviso fixo no topo seria menos verdadeiro.
	querem := []string{
		"ui/itens.html", "ui/item_atributos.html",
		"ui/monstros.html", "ui/monstro.html",
		"ui/npcs.html", "ui/npc.html",
		"ui/mesaxp.html", "ui/montarias.html",
		"ui/eventos.html",
	}
	tpl := lerTemplates(t)
	for _, nome := range querem {
		corpo, ok := tpl[nome]
		if !ok {
			t.Errorf("%s não existe mais; tire da lista ou conserte o nome", nome)
			continue
		}
		if !strings.Contains(corpo, `{{template "quando-vale"`) {
			t.Errorf("%s grava conteúdo e não diz quando a mudança vale", nome)
		}
	}
}

func primeiraLinha(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i]) + "…"
	}
	return s
}

// --- a auditoria, que era a última página a responder um erro de leitura com
//     uma tela de erro em vez do bloco compartilhado ---

func TestAuditoriaMostraAIdadeAoLadoDaHora(t *testing.T) {
	aud := newFakeAudit()
	aud.add(audit.Entry{
		ActorName: "hanna", ActorRole: "admin", Action: "role",
		CreatedAt: time.Now().Add(-5 * time.Hour),
	})
	get := signedIn(t, newTestPanelWith(t, withTarget(roleAdmin), aud))
	body := get("/auditoria").Body.String()

	// A pergunta que a auditoria responde é "o que a equipe andou fazendo", e
	// essa é uma pergunta sobre frescor: a hora crua faz o leitor subtrair.
	if !strings.Contains(body, "há 5 h") {
		t.Error("a auditoria não diz há quanto tempo a ação aconteceu")
	}
	// E a hora exata continua, porque o outro motivo de abrir esta página é
	// alinhar uma ação com outra coisa que aconteceu.
	if !strings.Contains(body, "hanna") {
		t.Error("a linha sumiu")
	}
}

func TestAuditoriaQuebradaNaoViraTelaDeErro(t *testing.T) {
	aud := newFakeAudit()
	aud.failList = errors.New("sem banco")
	get := signedIn(t, newTestPanelWith(t, withTarget(roleAdmin), aud))
	rec := get("/auditoria")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; uma consulta que falha não é o painel fora do ar", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "a auditoria") {
		t.Error("a página não avisa que não conseguiu ler")
	}
}
