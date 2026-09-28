package panel

import (
	"net/http"
	"strconv"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// pagamentoView é uma linha da fila de pagar, do jeito que a tela mostra.
type pagamentoView struct {
	store.PagamentoNaFila
	Valor   string
	Espera  string
	Vence   string
	Atrasou bool
	// Chave e Doc INTEIROS só aparecem na linha que a pessoa pediu para ver, e só na
	// resposta daquele clique. Não voltam numa recarga da página: quem quiser de novo
	// pede de novo, e a segunda leitura também fica registrada.
	Chave string
	Doc   string
}

// filaDePagamento monta a fila para a página.
func (h *Handler) filaDePagamento(r *http.Request) ([]pagamentoView, error) {
	fila, err := h.cfg.Repasses.FilaDePagamentoAMao(r.Context(), 0)
	if err != nil {
		return nil, err
	}
	agora := time.Now()
	vistas := make([]pagamentoView, 0, len(fila))
	for _, p := range fila {
		vistas = append(vistas, pagamentoView{
			PagamentoNaFila: p,
			Valor:           emReais(p.ValorCentavos),
			Espera:          idade(p.CriadoEm, agora),
			// A data inteira, e não "em 12 h": quem paga está olhando o relógio do
			// banco, e "vence em 12 h" obriga a pessoa a fazer a conta de cabeça para
			// saber se ainda dá tempo hoje.
			Vence:   p.VenceEm.Format("02/01 15:04"),
			Atrasou: agora.After(p.VenceEm),
		})
	}
	return vistas, nil
}

// pagarRepasse é o "Marcar como pago": a staff pagou fora do sistema e fecha a dívida.
//
// SÓ ADMIN, como as outras decisões de dinheiro desta tela. E a observação é obrigatória
// aqui pelo motivo mais forte de todos: é a única escrita de dinheiro do servidor sem
// nenhum comprovante nosso atrás dela.
func (h *Handler) pagarRepasse(w http.ResponseWriter, r *http.Request) {
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
	nota := r.PostFormValue("nota")
	if nota == "" {
		http.Redirect(w, r, "/repasses?aviso="+urlQuery(
			"Escreva onde você pagou e como achar o comprovante."), http.StatusSeeOther)
		return
	}
	ator := store.AtorDoAjuste{ContaID: sess.AccountID, PainelID: sess.PainelUsuarioID, Papel: roleFrom(r.Context()), Nome: sess.AccountName}
	if err := h.cfg.Repasses.MarcarRepassePagoAMao(r.Context(), id, ator, nota); err != nil {
		// A RECUSA NÃO É ERRO DE SERVIDOR, e é o caso do segundo clique: a linha saiu
		// de pendente entre a página e o botão. Dizer isso é melhor que um 500, porque
		// quem clicou precisa saber se pagou duas vezes.
		h.cfg.Logger.Warn("pagamento a mao recusado", "repasse", id, "err", err)
		http.Redirect(w, r, "/repasses?aviso="+urlQuery(
			"Não foi possível marcar como pago: a linha já não está pendente. "+
				"Recarregue a fila e confira se ela já foi paga."), http.StatusSeeOther)
		return
	}
	h.cfg.Logger.Info("repasse pago a mao", "repasse", id, "por", sess.AccountName)
	http.Redirect(w, r, "/repasses?aviso="+urlQuery("Repasse marcado como pago."),
		http.StatusSeeOther)
}

// chaveDoRepasse mostra a chave e o CPF INTEIROS de uma linha, e deixa registrado que
// esta pessoa os leu.
//
// RESPONDE RENDERIZANDO, e não com um redirect: a chave num redirect viajaria na URL, e
// URL entra em log de acesso, em histórico de navegador e no cabeçalho Referer da
// próxima requisição. O dado pessoal fica no CORPO da resposta daquele clique.
//
// SÓ ADMIN, pelo mesmo motivo do pagar: quem não pode pagar não tem por que ler a chave.
func (h *Handler) chaveDoRepasse(w http.ResponseWriter, r *http.Request) {
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
	ator := store.AtorDoAjuste{ContaID: sess.AccountID, PainelID: sess.PainelUsuarioID, Papel: roleFrom(r.Context()), Nome: sess.AccountName}
	chave, err := h.cfg.Repasses.ChaveParaPagar(r.Context(), id, ator)
	if err != nil {
		h.cfg.Logger.Warn("leitura da chave recusada", "repasse", id, "err", err)
		http.Redirect(w, r, "/repasses?aviso="+urlQuery(
			"Não foi possível ler a chave: a linha já não está pendente."),
			http.StatusSeeOther)
		return
	}
	// O log de aplicação NÃO leva a chave, só a conta e o repasse: a trilha de quem leu
	// é a auditoria, que fica no banco e tem dono. O log vai para a plataforma.
	h.cfg.Logger.Info("chave revelada para pagamento", "repasse", id, "por", sess.AccountName)
	h.renderRepasses(w, r, "", id, chave)
}
