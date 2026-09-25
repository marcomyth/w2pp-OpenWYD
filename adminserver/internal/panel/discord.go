package panel

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/accounts"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
)

// desvincularDiscord tira o Discord de uma conta. Só admin.
//
// POR QUE O BOTÃO EXISTE. O jogador não troca o próprio vínculo: trocar é recusado no
// servidor, senão quem tomasse uma conta trocaria o Discord em silêncio e levaria junto
// o cargo que ele dá. Essa recusa só é sustentável porque existe esta saída — sem ela,
// "só a staff desfaz" seria uma frase sem ninguém atrás, e quem vinculou errado ficaria
// preso para sempre.
//
// A observação é obrigatória e a auditoria vai na MESMA transação da soltura
// (accounts.DesvincularDiscord): desvincular abre a porta para outra pessoa pegar
// aquele Discord, e se foi por engano, a linha da auditoria é a única coisa que sobra
// para reconstruir a história.
func (h *Handler) desvincularDiscord(w http.ResponseWriter, r *http.Request) {
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

	err := h.cfg.Writer.DesvincularDiscord(r.Context(), sess.AccountID, sess.PainelUsuarioID,
		roleFrom(r.Context()), auth.ID, nota)
	switch {
	case errors.Is(err, accounts.ErrNotaDoDiscord):
		http.Error(w,
			"Escreva por que está desvinculando. É o que explica a decisão se ela estiver errada.",
			http.StatusBadRequest)
		return
	case errors.Is(err, accounts.ErrSemDiscord):
		h.voltaComAviso(w, r, "/contas/"+urlPath(nome),
			"Esta conta não tem Discord vinculado. Se o jogador não consegue vincular, a causa é outra.")
		return
	case errors.Is(err, accounts.ErrNotFound):
		http.NotFound(w, r)
		return
	case err != nil:
		h.cfg.Logger.Error("desvincular discord falhou", "account", nome, "id", auth.ID, "err", err)
		http.Error(w, "Erro ao desvincular o Discord.", http.StatusInternalServerError)
		return
	}
	// O LOG DIZ A CONTA E NÃO O DISCORD. O id vai para a auditoria, que é o lugar
	// certo dele; o log de aplicação não precisa do dado pessoal para servir.
	h.cfg.Logger.Info("discord desvinculado a mao", "account", nome, "id", auth.ID,
		"ator", sess.AccountID, "ator_painel", sess.PainelUsuarioID, "acao", audit.ActionDesvincularDiscord)
	h.voltaComAviso(w, r, "/contas/"+urlPath(nome),
		"Discord desvinculado. O jogador já pode vincular outro, e aquele Discord já pode ir para outra conta.")
}
