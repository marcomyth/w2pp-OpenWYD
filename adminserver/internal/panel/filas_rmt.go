package panel

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// AS TRÊS FILAS DE GENTE QUE FALTAVAM, depois da do repasse.
//
// As quatro existiam no banco e nenhuma tinha tela: as linhas paravam lá e ficavam.
// A do repasse veio primeiro porque nela o dinheiro é de um vendedor que já entregou
// o item. Nestas três o dinheiro está com a gente — o que não quer dizer que possa
// esperar, quer dizer que quem espera é o comprador.
//
// TRÊS TELAS E NÃO UMA, embora todas sejam "dinheiro parado". A pergunta que cada uma
// responde é diferente, e juntá-las faria uma tela em que ninguém sabe o que está
// olhando:
//
//   - ÓRFÃO: entrou dinheiro e não achamos a venda. A pergunta é "de quem é isto?".
//   - REEMBOLSO: devíamos devolver e não conseguimos. A pergunta é "por que travou?".
//   - DIVERGENTE: entrou valor diferente do cobrado. A pergunta é "o que fazer?", e
//     ela não tem resposta automática nenhuma.
type FilasRMT interface {
	PagamentosOrfaos(ctx context.Context) ([]store.PagamentoOrfao, error)
	ResolverPagamentoOrfao(ctx context.Context, id int64, por, nota string) error
	ReembolsosRecusados(ctx context.Context) ([]store.ReembolsoRecusadoNaFila, error)
	ReabrirReembolsoRecusado(ctx context.Context, cobrancaID int64, ator store.AtorDaStaff) error
	ResolverReembolsoNaMao(ctx context.Context, cobrancaID int64, ator store.AtorDaStaff) error
	ConfirmarReembolsoPedido(ctx context.Context, cobrancaID int64, ator store.AtorDaStaff) error
	ValoresDivergentes(ctx context.Context) ([]store.ValorDivergenteNaFila, error)
	ResolverDivergenteDevolvido(ctx context.Context, cobrancaID int64, ator store.AtorDaStaff, nota string) error
	// ReembolsosSemIdentifier é a fila invisível: devoluções devidas que a varredura
	// não consegue nem tentar, porque falta o id da processadora.
	ReembolsosSemIdentifier(ctx context.Context) (int, error)
}

// --- PAGAMENTO ÓRFÃO ---

type orfaoView struct {
	store.PagamentoOrfao
	Valor  string
	Espera string
	Pago   string
}

// orfaos mostra o dinheiro que entrou e não achou dono.
func (h *Handler) orfaos(w http.ResponseWriter, r *http.Request) {
	fila, err := h.cfg.FilasRMT.PagamentosOrfaos(r.Context())
	if err != nil {
		h.cfg.Logger.Error("fila de orfaos falhou", "err", err)
		http.Error(w, "Erro ao ler a fila de pagamentos órfãos.", http.StatusInternalServerError)
		return
	}
	agora := time.Now()
	vistas := make([]orfaoView, 0, len(fila))
	for _, o := range fila {
		v := orfaoView{PagamentoOrfao: o, Espera: idade(o.VistoEm, agora)}
		// O VALOR PODE SER NULO, e o motivo é o caso mais comum de órfão: a
		// processadora disse "pago" e não disse quanto. Um zero na tela seria
		// afirmar que entrou nada, que é diferente de "ela não contou".
		if o.ValorCentavos != nil {
			v.Valor = emReais(*o.ValorCentavos)
		}
		if o.PagoEm != nil {
			v.Pago = o.PagoEm.Format("02/01/2006 15:04")
		}
		vistas = append(vistas, v)
	}
	h.render(w, "orfaos.html", struct {
		page
		Fila  []orfaoView
		Aviso string
	}{h.pageFor(r, "orfaos"), vistas, r.URL.Query().Get("aviso")})
}

// resolverOrfao tira uma linha da fila.
//
// SÓ A MARCA, E NUNCA O DINHEIRO. O que fazer com o pagamento — devolver, achar a
// venda na mão, deixar como está — é decisão de quem manda, e é executada por fora,
// no painel da processadora. Esta tela registra que foi decidido, por quem, e o que
// a pessoa escreveu.
//
// A NOTA É OBRIGATÓRIA porque ela é a única coisa que sobra. A linha sai da fila e
// não volta; sem a nota, daqui a um mês ninguém sabe o que foi feito com aquele
// dinheiro — e o dinheiro é de uma pessoa que pagou e não recebeu nada.
func (h *Handler) resolverOrfao(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	sess, _ := staffFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("orfao"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	nota := r.PostFormValue("nota")
	if nota == "" {
		http.Redirect(w, r, "/orfaos?aviso="+urlQuery(
			"Escreva o que foi feito com este dinheiro."), http.StatusSeeOther)
		return
	}

	if err := h.cfg.FilasRMT.ResolverPagamentoOrfao(r.Context(), id, sess.AccountName, nota); err != nil {
		h.cfg.Logger.Error("resolver orfao falhou", "orfao", id, "err", err)
		http.Error(w, "Erro ao resolver o pagamento órfão.", http.StatusInternalServerError)
		return
	}
	if err := h.cfg.Audit.Write(r.Context(), audit.Record{
		ActorID: sess.AccountID, AtorPainelID: sess.PainelUsuarioID, ActorRole: roleFrom(r.Context()),
		Action: audit.ActionOrfaoResolvido,
		New:    map[string]any{"orfao": id, "nota": nota},
	}); err != nil {
		h.cfg.Logger.Error("orfao resolvido mas NAO auditado", "orfao", id, "err", err)
		http.Error(w, "O pagamento foi marcado, mas a auditoria falhou. Avise quem cuida do servidor.",
			http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/orfaos?aviso="+urlQuery("Marcado como resolvido."), http.StatusSeeOther)
}

// --- REEMBOLSO PARADO ---

type reembolsoView struct {
	store.ReembolsoRecusadoNaFila
	Valor   string
	Incerto bool
}

// reembolsos mostra as devoluções que travaram.
func (h *Handler) reembolsos(w http.ResponseWriter, r *http.Request) {
	fila, err := h.cfg.FilasRMT.ReembolsosRecusados(r.Context())
	if err != nil {
		h.cfg.Logger.Error("fila de reembolsos falhou", "err", err)
		http.Error(w, "Erro ao ler a fila de reembolsos.", http.StatusInternalServerError)
		return
	}
	var naoLeu falhas
	semID, err := h.cfg.FilasRMT.ReembolsosSemIdentifier(r.Context())
	if err != nil {
		// A lista é a página; perder o contador não pode apagá-la. Mas um zero ao
		// lado de uma lista é uma afirmação, então a página diz que não conseguiu
		// ler em vez de mostrar um número que não mediu.
		h.cfg.Logger.Error("contagem de reembolsos sem identifier falhou", "err", err)
		naoLeu.nao("semid")
	}
	vistas := make([]reembolsoView, 0, len(fila))
	for _, l := range fila {
		vistas = append(vistas, reembolsoView{
			ReembolsoRecusadoNaFila: l,
			Valor:                   emReais(l.ValorCentavos),
			Incerto:                 l.Estado == store.ReembolsoIncerto,
		})
	}
	h.render(w, "reembolsos.html", struct {
		page
		Fila   []reembolsoView
		SemID  int
		NaoLeu falhas
		Aviso  string
	}{h.pageFor(r, "reembolsos"), vistas, semID, naoLeu, r.URL.Query().Get("aviso")})
}

// resolverReembolso é a decisão da staff sobre uma devolução parada.
func (h *Handler) resolverReembolso(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	sess, _ := staffFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("cobranca"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	ator := store.AtorDaStaff{ContaID: sess.AccountID, PainelID: sess.PainelUsuarioID, Papel: roleFrom(r.Context())}

	var acao, aviso string
	switch r.PostFormValue("decisao") {
	case "de-novo":
		err = h.cfg.FilasRMT.ReabrirReembolsoRecusado(r.Context(), id, ator)
		acao, aviso = audit.ActionReembolsoDeNovo, "De volta na fila para pedir."
	case "na-mao":
		err = h.cfg.FilasRMT.ResolverReembolsoNaMao(r.Context(), id, ator)
		acao, aviso = audit.ActionReembolsoNaMao, "Marcado como resolvido na mão."
	case "achado":
		err = h.cfg.FilasRMT.ConfirmarReembolsoPedido(r.Context(), id, ator)
		acao, aviso = audit.ActionReembolsoAchadoNoPainel, "Marcado como em análise na processadora."
	default:
		http.Error(w, "Decisão desconhecida.", http.StatusBadRequest)
		return
	}

	if err != nil {
		if errors.Is(err, store.ErrReembolsoNaoEstaRecusado) {
			http.Redirect(w, r, "/reembolsos?aviso="+urlQuery(
				"Esta linha mudou de estado enquanto a tela estava aberta. Recarregue e olhe de novo."),
				http.StatusSeeOther)
			return
		}
		h.cfg.Logger.Error("resolver reembolso falhou", "cobranca", id, "err", err)
		http.Error(w, "Erro ao resolver o reembolso.", http.StatusInternalServerError)
		return
	}

	// SEM SEGUNDA ESCRITA DE AUDITORIA AQUI. A transição do reembolso já grava o
	// registro DENTRO da mesma transação (ver store/reembolso_rmt.go), que é mais
	// estrito do que o resto do painel faz de propósito: se o registro falha, a
	// mudança não acontece. Escrever de novo daqui criaria duas linhas para o mesmo
	// ato, e quem lesse o log contaria duas decisões.
	h.cfg.Logger.Info("reembolso resolvido", "ator", sess.AccountName, "cobranca", id, "acao", acao)
	http.Redirect(w, r, "/reembolsos?aviso="+urlQuery(aviso), http.StatusSeeOther)
}

// --- VALOR DIVERGENTE ---

type divergenteView struct {
	store.ValorDivergenteNaFila
	Cobrado   string
	Recebido  string
	Diferenca string
	AMenos    bool
}

// divergentes mostra as cobranças em que entrou valor diferente do pedido.
//
// A TELA NÃO DECIDE O DESTINO DO DINHEIRO, e essa parte continua sendo de fora:
// devolver, cobrar a diferença ou entregar assim mesmo é escolha sobre o dinheiro de
// duas pessoas, e acontece no painel da processadora e conversando com quem comprou.
//
// MAS ELA PRECISA DE UMA SAÍDA, e a primeira versão desta tela não tinha — foi um
// erro meu que a planejadora pegou. A cobrança divergente fica ABERTA, e enquanto ela
// estiver aberta duas pessoas estão presas no jogo: o comprador não consegue comprar
// mais nada, porque o índice de uma cobrança aberta por comprador o recusa, e o item
// do vendedor fica marcado, porque o anúncio continua esperando uma cobrança que
// nunca fecha. Sem saída, a staff resolvia o dinheiro e as duas continuavam travadas
// para sempre, porque nada no sistema sabia que tinha acabado.
//
// UMA SAÍDA SÓ, e é de registro e não de dinheiro: "devolvido ao comprador por fora".
// Ela não devolve nada — diz que alguém já devolveu, e destrava o que estava preso
// por causa daquela cobrança.
//
// "ENTREGAR ASSIM MESMO" NÃO GANHOU BOTÃO. Quem quiser isso entrega o item pelo painel
// de entrega que já existe e depois usa esta mesma saída, escrevendo na nota o que
// fez. Um botão com dois destinos para o item seria exatamente o caminho automático
// que esta tela existe para não ter.
func (h *Handler) divergentes(w http.ResponseWriter, r *http.Request) {
	fila, err := h.cfg.FilasRMT.ValoresDivergentes(r.Context())
	if err != nil {
		h.cfg.Logger.Error("fila de divergentes falhou", "err", err)
		http.Error(w, "Erro ao ler a fila de valores divergentes.", http.StatusInternalServerError)
		return
	}
	vistas := make([]divergenteView, 0, len(fila))
	for _, d := range fila {
		dif := d.ValorRecebido - d.ValorCobrado
		v := divergenteView{
			ValorDivergenteNaFila: d,
			Cobrado:               emReais(d.ValorCobrado),
			Recebido:              emReais(d.ValorRecebido),
			AMenos:                dif < 0,
		}
		if dif < 0 {
			dif = -dif
		}
		v.Diferenca = emReais(dif)
		vistas = append(vistas, v)
	}
	h.render(w, "divergentes.html", struct {
		page
		Fila  []divergenteView
		Aviso string
	}{h.pageFor(r, "divergentes"), vistas, r.URL.Query().Get("aviso")})
}

// resolverDivergente registra que uma pessoa já devolveu o dinheiro por fora, e
// destrava as duas pontas.
//
// A NOTA É OBRIGATÓRIA, e aqui ela carrega mais peso do que nas outras telas: esta é
// a única linha que vai dizer o que foi feito com uma diferença de valor que nenhum
// caminho automático tocou. Sem ela, fica registrado que "alguém fechou", e mais nada.
func (h *Handler) resolverDivergente(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	sess, _ := staffFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("cobranca"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	nota := r.PostFormValue("nota")
	if nota == "" {
		http.Redirect(w, r, "/divergentes?aviso="+urlQuery(
			"Escreva o que foi feito com este dinheiro."), http.StatusSeeOther)
		return
	}
	ator := store.AtorDaStaff{ContaID: sess.AccountID, PainelID: sess.PainelUsuarioID, Papel: roleFrom(r.Context())}

	if err := h.cfg.FilasRMT.ResolverDivergenteDevolvido(r.Context(), id, ator, nota); err != nil {
		if errors.Is(err, store.ErrDivergenteNaoEstaAberta) {
			http.Redirect(w, r, "/divergentes?aviso="+urlQuery(
				"Esta cobranca ja foi fechada por alguem. Recarregue e olhe de novo."),
				http.StatusSeeOther)
			return
		}
		h.cfg.Logger.Error("resolver divergente falhou", "cobranca", id, "err", err)
		http.Error(w, "Erro ao resolver a cobrança.", http.StatusInternalServerError)
		return
	}
	// A auditoria já foi escrita DENTRO da transição, na mesma transação. Ver a nota
	// em resolverReembolso: uma segunda escrita daqui viraria duas linhas para o
	// mesmo ato.
	h.cfg.Logger.Info("divergente resolvida", "ator", sess.AccountName, "cobranca", id)
	http.Redirect(w, r, "/divergentes?aviso="+urlQuery("Registrado, e as duas pontas destravadas."),
		http.StatusSeeOther)
}
