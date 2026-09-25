package panel

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/accounts"
)

// setForaDoRanking marca ou desmarca uma conta como escondida do ranking.
//
// SÓ ADMIN. O ranking é a vitrine pública do servidor, e tirar alguém dela é decisão de
// quem responde por ele — não é ferramenta de moderação do dia a dia, como silenciar ou
// bloquear.
//
// A observação é OPCIONAL, ao contrário da nota do pagamento à mão. Lá não havia
// comprovante nenhum atrás da escrita e a nota era a única prova; aqui não há dinheiro,
// e exigir texto para marcar três contas de teste produziria "teste" digitado três
// vezes, que não informa nada a quem ler a auditoria depois.
func (h *Handler) setForaDoRanking(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	nome, auth, ok := h.alvo(w, r)
	if !ok {
		return
	}
	sess, _ := staffFrom(r.Context())
	fora := r.PostFormValue("fora") == "1"
	nota := strings.TrimSpace(r.PostFormValue("observacao"))

	// OS DOIS ATORES, como a soltura da posse: o painel aceita login de conta do jogo
	// e login de usuário do painel, e a auditoria guarda o que for. Passar zero fixo
	// num deles — erro que eu cometi na primeira versão disto — faria toda ação de
	// usuário do painel ser recusada pela trava de "um ator e só um", e a recusa
	// apareceria como erro de servidor sem explicação.
	mudou, err := h.cfg.Writer.ForaDoRanking(r.Context(), sess.AccountID, sess.PainelUsuarioID,
		roleFrom(r.Context()), auth.ID, fora, nota)
	if errors.Is(err, accounts.ErrNotaLonga) {
		h.redirectConta(w, r, nome, "A observação é longa demais.")
		return
	}
	if err != nil {
		h.recusa(w, r, nome, err)
		return
	}
	// O SEGUNDO CLIQUE NÃO É ERRO: duas pessoas resolvendo a mesma coisa, ou o botão
	// apertado duas vezes. Dizer "nada mudou" evita que alguém vá procurar defeito.
	if !mudou {
		h.redirectConta(w, r, nome, "Nada mudou: a conta já estava assim.")
		return
	}
	aviso := "Conta escondida do ranking."
	if !fora {
		aviso = "Conta de volta ao ranking."
	}
	h.cfg.Logger.Info("fora do ranking", "actor", sess.AccountName, "target", nome, "fora", fora)
	h.redirectConta(w, r, nome, aviso)
}
