package panel

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/accounts"
)

// TestSoltarPosseExigeObservacao: soltar a posse é afirmar que o dono da conta não
// existe mais. Se a afirmação estiver errada, duas cópias do mesmo personagem
// entram em jogo — e quem lê a auditoria depois precisa saber o que foi conferido.
func TestSoltarPosseExigeObservacao(t *testing.T) {
	wr := newFakeWriter()
	wr.posse = accounts.Posse{Presa: true, Epoca: 9}
	wr.erroSoltarPosse = accounts.ErrNotaDaPosse
	h := newTestPanelFull(t, withTarget(roleAdmin), newFakeAudit(), wr)
	post, token := signedInPost(t, h)

	rec := post("/contas/ana/posse", url.Values{"csrf": {token}, "observacao": {"   "}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if len(wr.posseSolta) != 0 {
		t.Fatal("soltou sem observação")
	}
}

// TestSoltarPosseGuardaQuemESoltouEPorQue: a auditoria é gravada na mesma
// transação da soltura (accounts.SoltarPosse), então o que o painel precisa provar
// é que ela recebe o ator, o papel e a observação.
func TestSoltarPosseGuardaQuemESoltouEPorQue(t *testing.T) {
	wr := newFakeWriter()
	wr.posse = accounts.Posse{Presa: true, Epoca: 9}
	h := newTestPanelFull(t, withTarget(roleAdmin), newFakeAudit(), wr)
	post, token := signedInPost(t, h)

	rec := post("/contas/ana/posse", url.Values{
		"csrf": {token}, "observacao": {"servidor velho fora do ar ha 10 min"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303: %s", rec.Code, rec.Body.String())
	}
	if len(wr.posseSolta) != 1 {
		t.Fatalf("solturas = %d, want 1", len(wr.posseSolta))
	}
	s := wr.posseSolta[0]
	if s.nota != "servidor velho fora do ar ha 10 min" {
		t.Errorf("observação = %q", s.nota)
	}
	if s.papel == "" {
		t.Error("o papel de quem soltou não chegou à auditoria")
	}
	if s.atorConta == 0 && s.atorPainel == 0 {
		t.Error("a soltura foi gravada sem ator")
	}
}

// TestSoltarPosseDeContaLivreNaoMente: quem clicou está investigando um jogador que
// não consegue entrar. Dizer "soltei" o faria procurar a causa no lugar errado.
func TestSoltarPosseDeContaLivreNaoMente(t *testing.T) {
	wr := newFakeWriter()
	wr.erroSoltarPosse = accounts.ErrSemPosse
	h := newTestPanelFull(t, withTarget(roleAdmin), newFakeAudit(), wr)
	post, token := signedInPost(t, h)

	rec := post("/contas/ana/posse", url.Values{"csrf": {token}, "observacao": {"conferido"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	destino := rec.Header().Get("Location")
	if !strings.Contains(destino, "nenhuma+execu") && !strings.Contains(destino, "nenhuma%20execu") {
		t.Errorf("o aviso não diz que a conta não estava presa: %q", destino)
	}
}

// TestSoltarPosseSoAdmin: moderador não solta. O clique errado põe duas cópias do
// personagem em jogo.
func TestSoltarPosseSoAdmin(t *testing.T) {
	wr := newFakeWriter()
	wr.posse = accounts.Posse{Presa: true, Epoca: 9}
	// Como no TestSetCargoIsAdminOnly: quem está logado aqui é moderador.
	h := newTestPanelFull(t, withTarget(roleModerator), newFakeAudit(), wr)
	post, token := signedInPost(t, h)

	rec := post("/contas/ana/posse", url.Values{"csrf": {token}, "observacao": {"conferido"}})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if len(wr.posseSolta) != 0 {
		t.Fatal("moderador soltou a posse")
	}
}

// TestPosseParaTelaDizHaQuantoTempo: é esse texto que decide o clique — batimento
// de segundos atrás é servidor rodando, de minutos é processo que se foi.
func TestPosseParaTelaDizHaQuantoTempo(t *testing.T) {
	agora := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	casos := []struct {
		nome  string
		posse accounts.Posse
		quer  string
	}{
		{"livre", accounts.Posse{}, ""},
		{"sem batimento", accounts.Posse{Presa: true}, "nenhum registrado"},
		{"segundos", posseCom(agora.Add(-20 * time.Second)), "há 20 segundo(s)"},
		{"minutos", posseCom(agora.Add(-5 * time.Minute)), "há 5 minuto(s)"},
		{"horas", posseCom(agora.Add(-3 * time.Hour)), "há 3 hora(s)"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			v := posseParaTela(c.posse, agora)
			if v.Presa != c.posse.Presa {
				t.Errorf("presa = %v", v.Presa)
			}
			if v.BatimentoTexto != c.quer {
				t.Errorf("texto = %q, quer %q", v.BatimentoTexto, c.quer)
			}
		})
	}
}

func posseCom(t time.Time) accounts.Posse {
	return accounts.Posse{Presa: true, Epoca: 9, UltimoBatimento: &t}
}
