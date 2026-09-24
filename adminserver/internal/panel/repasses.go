package panel

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// Repasses é a fila do dinheiro que devemos a vendedores e que parou de andar.
//
// A PRIMEIRA DAS QUATRO TELAS DE GENTE, e é a primeira porque é a única em que a
// espera custa dinheiro a alguém de fora: nas outras três o dinheiro está com a
// gente e parado, aqui ele é de um vendedor que já entregou o item dele.
//
// Satisfeita por *store.Store, como as outras filas: as linhas são escritas pela
// varredura do webserver e lidas aqui.
type Repasses interface {
	RepassesQuePrecisamDeGente(ctx context.Context) ([]store.RepasseNaFila, error)
	ResolverIncertoComoPago(ctx context.Context, id int64, ator store.AtorDoRepasse, chegouCentavos int64) error
	ResolverIncertoComoNaoPago(ctx context.Context, id int64, ator store.AtorDoRepasse, nota string) error
	ResolverRecusa(ctx context.Context, id int64, ator store.AtorDoRepasse) error
}

// repasseView é uma linha da fila do jeito que a página mostra.
type repasseView struct {
	store.RepasseNaFila
	Espera  string
	Valor   string
	Incerto bool
	Recusa  bool
	Espera2 bool
}

// repasses mostra a fila.
func (h *Handler) repasses(w http.ResponseWriter, r *http.Request) {
	fila, err := h.cfg.Repasses.RepassesQuePrecisamDeGente(r.Context())
	if err != nil {
		h.cfg.Logger.Error("fila de repasses falhou", "err", err)
		http.Error(w, "Erro ao ler a fila de repasses.", http.StatusInternalServerError)
		return
	}

	agora := time.Now()
	vistas := make([]repasseView, 0, len(fila))
	var incertos int
	for _, l := range fila {
		v := repasseView{
			RepasseNaFila: l,
			Espera:        idade(l.CriadoEm, agora),
			Valor:         emReais(l.ValorCentavos),
			Incerto:       l.Estado == store.RepasseIncerto,
			Recusa:        l.Estado == store.RepasseRecusado,
			Espera2:       l.Estado == store.RepasseEsperandoCadastro,
		}
		if v.Incerto {
			incertos++
		}
		vistas = append(vistas, v)
	}

	h.render(w, "repasses.html", struct {
		page
		Fila     []repasseView
		Incertos int
		Aviso    string
	}{h.pageFor(r, "repasses"), vistas, incertos, r.URL.Query().Get("aviso")})
}

// emReais escreve os centavos como a pessoa lê.
//
// Formatado aqui e não no template porque quem confere este número está olhando
// para o extrato da processadora do lado — e centavo fora de lugar numa tela de
// dinheiro é o tipo de erro que só aparece depois de alguém agir sobre ele.
func emReais(centavos int64) string {
	sinal := ""
	if centavos < 0 {
		sinal, centavos = "-", -centavos
	}
	return sinal + "R$ " + strconv.FormatInt(centavos/100, 10) +
		"," + pad2(centavos%100)
}

// centavosDoFormulario le "12,34", "12.34" ou "12" e devolve centavos.
//
// ESCRITO A MAO em vez de usar um parser de moeda porque o que entra aqui vem de
// uma pessoa copiando do extrato da processadora, e as duas formas de separador
// convivem na mesma tela. Recusa o que nao entender: numa tela de dinheiro, adivinhar
// o que alguem quis digitar e pior do que pedir de novo.
func centavosDoFormulario(txt string) (int64, error) {
	txt = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(txt), "R$"))
	txt = strings.ReplaceAll(txt, " ", "")
	txt = strings.Replace(txt, ".", ",", 1)
	if txt == "" {
		return 0, fmt.Errorf("panel: valor vazio")
	}
	inteiro, centavos, temVirgula := strings.Cut(txt, ",")
	if strings.Contains(centavos, ",") {
		return 0, fmt.Errorf("panel: valor com duas virgulas")
	}
	reais, err := strconv.ParseInt(inteiro, 10, 64)
	if err != nil || reais < 0 {
		return 0, fmt.Errorf("panel: reais invalidos em %q", txt)
	}
	var cents int64
	if temVirgula {
		switch len(centavos) {
		case 1:
			// "12,3" e doze reais e trinta, e nao doze e tres. E como se le em voz
			// alta, e ler diferente do que a pessoa leu erraria por dez vezes.
			centavos += "0"
		case 2:
		default:
			return 0, fmt.Errorf("panel: centavos com %d digitos", len(centavos))
		}
		if cents, err = strconv.ParseInt(centavos, 10, 64); err != nil || cents < 0 {
			return 0, fmt.Errorf("panel: centavos invalidos em %q", txt)
		}
	}
	if reais > (1<<62)/100 {
		return 0, fmt.Errorf("panel: valor absurdo")
	}
	return reais*100 + cents, nil
}

func pad2(n int64) string {
	if n < 10 {
		return "0" + strconv.FormatInt(n, 10)
	}
	return strconv.FormatInt(n, 10)
}

// resolverRepasse é a ação da staff sobre uma linha parada.
//
// UMA ROTA SÓ PARA AS TRÊS DECISÕES, escolhidas por um campo do formulário, porque
// elas partem da mesma pergunta — "o que aconteceu com este dinheiro?" — e porque
// o caminho de auditoria é idêntico. Três rotas quase iguais divergiriam no dia em
// que só uma ganhasse uma trava nova.
//
// SÓ ADMIN. Aqui não se configura jogo: afirma-se o que aconteceu com o dinheiro de
// uma pessoa, e "foi pago" fecha uma dívida sem que nada tenha saído da conta. Isso
// não é decisão de plantão.
func (h *Handler) resolverRepasse(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	sess, _ := staffFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("repasse"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	ator := store.AtorDoRepasse{Nome: sess.AccountName}

	var acao, aviso string
	var dados map[string]any
	switch r.PostFormValue("decisao") {
	case "pago":
		// O VALOR VEM DIGITADO, e não copiado da dívida, porque a pessoa está
		// olhando o extrato da processadora: o que ela tem na frente é quanto
		// CHEGOU, que pode não ser o que foi pedido. Copiar a dívida faria a tela
		// inventar um número e chamar de medição.
		chegou, erro := centavosDoFormulario(r.PostFormValue("chegou"))
		if erro != nil {
			http.Redirect(w, r, "/repasses?aviso="+urlQuery(
				"Valor inválido. Escreva quanto chegou, em reais, como 12,34."),
				http.StatusSeeOther)
			return
		}
		err = h.cfg.Repasses.ResolverIncertoComoPago(r.Context(), id, ator, chegou)
		acao, aviso = audit.ActionRepasseIncertoPago, "Marcado como pago."
		dados = map[string]any{"repasse": id, "chegou_centavos": chegou}
	case "nao-pago":
		nota := r.PostFormValue("nota")
		if nota == "" {
			// A NOTA É OBRIGATÓRIA nesta, e não nas outras. Dizer que um incerto NÃO
			// foi pago devolve a dívida para a fila e faz o dinheiro sair de novo —
			// e a única proteção contra pagar duas vezes é alguém ter escrito onde
			// olhou para ter certeza.
			http.Redirect(w, r, "/repasses?aviso="+urlQuery(
				"Escreva onde você conferiu que o pagamento não saiu."), http.StatusSeeOther)
			return
		}
		err = h.cfg.Repasses.ResolverIncertoComoNaoPago(r.Context(), id, ator, nota)
		acao, aviso = audit.ActionRepasseIncertoNaoPago, "Devolvido para a fila de pagamento."
		dados = map[string]any{"repasse": id, "nota": nota}
	case "tentar-de-novo":
		err = h.cfg.Repasses.ResolverRecusa(r.Context(), id, ator)
		acao, aviso = audit.ActionRepasseRecusaResolvida, "De volta na fila."
		dados = map[string]any{"repasse": id}
	default:
		http.Error(w, "Decisão desconhecida.", http.StatusBadRequest)
		return
	}

	if err != nil {
		// A transição recusada NÃO é falha de infraestrutura: é duas pessoas na
		// mesma linha, ou a varredura tendo mexido nela enquanto a tela estava
		// aberta. A pessoa precisa saber que não foi ela, e recarregar resolve.
		if errors.Is(err, store.ErrRepasseInexistente) {
			http.Redirect(w, r, "/repasses?aviso="+urlQuery(
				"Esta linha mudou de estado enquanto a tela estava aberta. Recarregue e olhe de novo."),
				http.StatusSeeOther)
			return
		}
		h.cfg.Logger.Error("resolver repasse falhou", "repasse", id, "err", err)
		http.Error(w, "Erro ao resolver o repasse.", http.StatusInternalServerError)
		return
	}

	if err := h.cfg.Audit.Write(r.Context(), audit.Record{
		ActorID: sess.AccountID, ActorRole: roleFrom(r.Context()),
		Action: acao, New: dados,
	}); err != nil {
		// A MUDANÇA JÁ ACONTECEU e a auditoria não. Erro visível, e não aviso de
		// rodapé: numa tela de dinheiro, a mudança sem registro é exatamente a que
		// ninguém consegue explicar depois.
		h.cfg.Logger.Error("repasse resolvido mas NAO auditado", "repasse", id, "err", err)
		http.Error(w, "O repasse foi resolvido, mas a auditoria falhou. Avise quem cuida do servidor.",
			http.StatusInternalServerError)
		return
	}

	h.cfg.Logger.Info("repasse resolvido", "ator", sess.AccountName, "repasse", id, "acao", acao)
	http.Redirect(w, r, "/repasses?aviso="+urlQuery(aviso), http.StatusSeeOther)
}
