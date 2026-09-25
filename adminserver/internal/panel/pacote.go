package panel

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/donate"
)

// efWday é o efeito de duração em dias na forma não iniciada (0123): a montaria
// do pacote conta a partir do primeiro uso.
const efWday = 106

// pacoteView é um pacote de apoiador como o cartão da aba Itens mostra.
type pacoteView struct {
	ID       string
	Nome     string
	Creditos int32
	Espacos  int
	Brindes  []string
}

// pacotesParaTela lê os pacotes e dá nome aos brindes.
//
// O catálogo é lido UMA vez para todos: nomeDoItem por brinde faria quarenta
// chamadas ao webServer para desenhar um cartão. Sem catálogo, o brinde aparece
// pelo índice, que continua sendo a verdade do que vai ser entregue.
func (h *Handler) pacotesParaTela(r *http.Request, moderatorID int64) ([]pacoteView, error) {
	pacotes, err := h.cfg.Carteira.Pacotes(r.Context())
	if err != nil {
		return nil, err
	}
	nomes := map[int32]string{}
	if h.cfg.GameData != nil {
		if itens, ierr := h.cfg.GameData.Items(r.Context(), moderatorID, ""); ierr != nil {
			h.cfg.Logger.Warn("item catalog unavailable for supporter packs", "err", ierr)
		} else {
			for _, it := range itens {
				nomes[it.Index] = firstNonEmpty(it.DisplayName, it.Name)
			}
		}
	}

	out := make([]pacoteView, 0, len(pacotes))
	for _, p := range pacotes {
		v := pacoteView{ID: p.ID, Nome: p.Nome(), Creditos: p.Creditos, Espacos: p.Espacos()}
		for _, b := range p.Brindes {
			v.Brindes = append(v.Brindes, textoDoBrinde(b, nomes[b.Index]))
		}
		out = append(out, v)
	}
	return out, nil
}

// textoDoBrinde escreve "10× Baú de Experiência" ou "Tigre de Fogo (7 dias)".
func textoDoBrinde(b donate.Brinde, nome string) string {
	if nome == "" {
		nome = fmt.Sprintf("item %d", b.Index)
	}
	if b.Quantidade > 1 {
		nome = fmt.Sprintf("%d× %s", b.Quantidade, nome)
	}
	for _, e := range b.Eff {
		if e[0] == efWday && e[1] > 0 {
			nome = fmt.Sprintf("%s (%d dias)", nome, e[1])
		}
	}
	return nome
}

// enviarPacote dá um pacote de apoiador inteiro, Rcoins e brindes, a uma conta.
// Só admin: cria moeda de donate, como o ajuste de saldo.
func (h *Handler) enviarPacote(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	sess, _ := staffFrom(r.Context())
	conta, auth, ok := h.alvo(w, r)
	if !ok {
		return
	}
	// O cartão já some na própria conta; a recusa aqui é para quem chama a rota
	// direto. Dar a si mesmo 20 mil Rcoins é o caso que o registro existe para
	// impedir, e não só para anotar.
	if auth.ID == sess.AccountID {
		http.Error(w, "O pacote não pode ser enviado para a sua própria conta.", http.StatusForbidden)
		return
	}

	pacoteID := strings.TrimSpace(r.PostFormValue("pacote"))
	motivo := strings.TrimSpace(r.PostFormValue("motivo"))
	if pacoteID == "" {
		h.redirectItens(w, r, conta, "Escolha o pacote.")
		return
	}

	env, err := h.cfg.Carteira.EnviarPacote(r.Context(), sess.AccountID, auth.ID, pacoteID, motivo)
	switch {
	case errors.Is(err, donate.ErrMotivoVazio):
		h.redirectItens(w, r, conta, "Informe o motivo do envio (ex.: parceria com o influencer Fulano).")
		return
	case errors.Is(err, donate.ErrPacoteRepetido):
		h.redirectItens(w, r, conta, "Este pacote já foi enviado para esta conta há menos de um minuto. "+
			"Se era mesmo para mandar outro, espere um minuto e envie de novo.")
		return
	case errors.Is(err, donate.ErrPacoteDesconhecido), errors.Is(err, donate.ErrPacoteIndisponivel):
		h.redirectItens(w, r, conta, "Esse pacote não está disponível para envio.")
		return
	case errors.Is(err, donate.ErrNaoEncontrado):
		http.NotFound(w, r)
		return
	case err != nil:
		h.cfg.Logger.Error("supporter pack failed", "account", conta, "pacote", pacoteID, "err", err)
		http.Error(w, "Erro ao enviar o pacote. Nada foi creditado nem enfileirado.", http.StatusInternalServerError)
		return
	}

	if err := h.cfg.Audit.Write(r.Context(), audit.Record{
		ActorID: sess.AccountID, AtorPainelID: sess.PainelUsuarioID, ActorRole: roleFrom(r.Context()),
		Action: audit.ActionSendSupporterPack, TargetID: auth.ID,
		New: map[string]any{
			"pacote": env.Pacote.ID, "rcoins": env.Pacote.Creditos, "saldo": env.Saldo,
			"entregas": env.Entregas, "motivo": motivo,
		},
	}); err != nil {
		h.cfg.Logger.Error("supporter pack sent but NOT audited", "account", conta, "pacote", env.Pacote.ID, "err", err)
		h.auditoriaFalhou(w, err)
		return
	}

	h.cfg.Logger.Info("supporter pack sent",
		"actor", sess.AccountName, "account", conta, "pacote", env.Pacote.ID,
		"rcoins", env.Pacote.Creditos, "entregas", env.Entregas)

	// Os Rcoins já estão no saldo; os brindes seguem o caminho de qualquer
	// entrega, e o aviso diz se chegaram agora ou no próximo login.
	aviso := fmt.Sprintf("%d Rcoins creditados (saldo: %d). ", env.Pacote.Creditos, env.Saldo) +
		h.avisoDaEntrega(r, conta, "O pacote "+env.Pacote.Nome())
	h.redirectItens(w, r, conta, aviso)
}

func (h *Handler) redirectItens(w http.ResponseWriter, r *http.Request, nome, msg string) {
	http.Redirect(w, r, "/contas/"+url.PathEscape(nome)+"?aba=itens&aviso="+url.QueryEscape(msg),
		http.StatusSeeOther)
}
