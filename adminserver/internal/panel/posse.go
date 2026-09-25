package panel

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/accounts"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
)

// soltarPosse devolve à mão uma conta presa a uma execução do jogo. Só admin.
//
// A posse se solta sozinha — no save de saída, ou pelo prazo do batimento. Este
// botão é para o dia em que ela NÃO se soltar: sem ele, o jogador ficaria de fora
// da própria conta esperando um conserto que só um deploy traria.
//
// A observação é obrigatória, e a auditoria é gravada na mesma transação da
// soltura (accounts.SoltarPosse): se a soltura estiver errada, alguém precisa
// poder ler depois quem disse que o dono não existia mais.
func (h *Handler) soltarPosse(w http.ResponseWriter, r *http.Request) {
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
	nota := strings.TrimSpace(r.PostFormValue("observacao"))
	sess, _ := staffFrom(r.Context())

	err := h.cfg.Writer.SoltarPosse(r.Context(), sess.AccountID, sess.PainelUsuarioID,
		roleFrom(r.Context()), auth.ID, nota)
	switch {
	case errors.Is(err, accounts.ErrNotaDaPosse):
		http.Error(w,
			"Escreva o que você conferiu antes de soltar. É o que explica a decisão se ela estiver errada.",
			http.StatusBadRequest)
		return
	case errors.Is(err, accounts.ErrSemPosse):
		h.voltaComAviso(w, r, "/contas/"+urlPath(nome),
			"Esta conta não está presa a nenhuma execução do jogo. Se o jogador não consegue entrar, a causa é outra.")
		return
	case errors.Is(err, accounts.ErrNotFound):
		http.NotFound(w, r)
		return
	case err != nil:
		h.cfg.Logger.Error("soltar posse falhou", "account", nome, "id", auth.ID, "err", err)
		http.Error(w, "Erro ao soltar a posse.", http.StatusInternalServerError)
		return
	}
	h.cfg.Logger.Info("posse da conta solta a mao", "account", nome, "id", auth.ID,
		"ator", sess.AccountID, "ator_painel", sess.PainelUsuarioID, "acao", audit.ActionSoltarPosse)
	h.voltaComAviso(w, r, "/contas/"+urlPath(nome),
		"Posse solta. O jogador já pode entrar. Se ele estava mesmo em jogo em outro servidor, o que ele fez de lá pode se perder.")
}
