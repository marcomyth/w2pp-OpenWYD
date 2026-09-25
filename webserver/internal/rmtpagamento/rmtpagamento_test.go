package rmtpagamento

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

func mudo() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

type pontefake struct {
	t   Transacao
	err error

	consultas int
}

func (p *pontefake) ConsultarTransacao(context.Context, string) (Transacao, error) {
	p.consultas++
	return p.t, p.err
}

type bancofake struct {
	// Respostas.
	refGravada string
	achou      bool
	erroAchar  error
	resultado  store.ResultadoCobranca
	venda      store.VendaRMT
	erroConf   error
	erroReemb  error
	erroOrfao  error
	nomes      map[int64]string

	// Observado.
	confirmou     int
	refConfirmada string
	pagoEm        time.Time
	origem        store.OrigemDaHora
	valorVisto    int64
	// taxaVista guarda o PONTEIRO como chegou, e não o valor: é a única forma de o
	// teste distinguir "taxa zero" de "taxa desconhecida", que é a distinção que vale
	// dinheiro neste caminho.
	taxaVista     *int64
	taxaRecebida  bool
	reembolsos    []int64
	orfaos        []store.PagamentoOrfao
	identGravados [][2]string
	erroGravarID  error
}

func (b *bancofake) GravarIdentifierSeFaltar(_ context.Context, ref, ident string) error {
	b.identGravados = append(b.identGravados, [2]string{ref, ident})
	return b.erroGravarID
}

func (b *bancofake) CobrancaDoIdentifier(context.Context, string) (string, bool, error) {
	return b.refGravada, b.achou, b.erroAchar
}

func (b *bancofake) ConfirmarCobrancaRMT(_ context.Context, ref string, pagoEm time.Time,
	origem store.OrigemDaHora, valor int64, taxa *int64,
) (store.ResultadoCobranca, store.VendaRMT, error) {
	b.confirmou++
	b.refConfirmada, b.pagoEm, b.origem, b.valorVisto = ref, pagoEm, origem, valor
	b.taxaVista, b.taxaRecebida = taxa, true
	return b.resultado, b.venda, b.erroConf
}

func (b *bancofake) MarcarReembolsoPendente(_ context.Context, id int64) error {
	b.reembolsos = append(b.reembolsos, id)
	return b.erroReemb
}

func (b *bancofake) RegistrarPagamentoOrfao(_ context.Context, p store.PagamentoOrfao) error {
	b.orfaos = append(b.orfaos, p)
	return b.erroOrfao
}

func (b *bancofake) NomeDaConta(_ context.Context, id int64) (string, error) {
	if n, ok := b.nomes[id]; ok {
		return n, nil
	}
	return "", errors.New("conta nao existe")
}

type jogofake struct {
	erroEntrega error
	erroLiberar error
	emJogo      bool

	entregou []string
	liberou  []string
}

func (j *jogofake) Entrega(_ context.Context, conta string) (bool, error) {
	j.entregou = append(j.entregou, conta)
	return j.emJogo, j.erroEntrega
}

func (j *jogofake) LiberaVenda(_ context.Context, conta string) (bool, error) {
	j.liberou = append(j.liberou, conta)
	return j.emJogo, j.erroLiberar
}

// vendaBoa é o cenário do caminho feliz, reusado pelos testes.
func vendaBoa() (*pontefake, *bancofake) {
	agora := time.Now().UTC()
	p := &pontefake{t: Transacao{
		Existe: true, Status: "completed", Referencia: "ref-1",
		ValorCentavos: 5000, PagoEm: &agora,
	}}
	b := &bancofake{
		refGravada: "ref-1", achou: true,
		resultado: store.CobrancaConfirmada,
		venda: store.VendaRMT{
			CobrancaID: 42, CompradorConta: 7, VendedorConta: 9,
		},
		nomes: map[int64]string{7: "comprador", 9: "vendedor"},
	}
	return p, b
}

// A METADE QUE A PLANEJADORA PEDIU: se a chamada ao jogo falhar, a venda JÁ ESTÁ
// GRAVADA e não se desfaz. A falha sai como aviso e não como erro.
//
// Por que isto tem de ser testado e não só comentado: se a falha da entrega imediata
// virasse erro, quem avisou repetiria o aviso em laço por uma venda que já deu certo.
// E se ela desfizesse algo, o comprador teria pagado e perdido o item.
func TestFalhaNoJogoNaoDesfazAVenda(t *testing.T) {
	p, b := vendaBoa()
	j := &jogofake{
		erroEntrega: errors.New("servidor de jogo fora do ar"),
		erroLiberar: errors.New("servidor de jogo fora do ar"),
	}

	res, err := Novo(p, b, j, mudo()).ConferirEConcluir(context.Background(), "id-1")

	if err != nil {
		t.Fatalf("a falha do jogo virou erro: %v", err)
	}
	if !res.Confirmada || !res.Entregue || res.CobrancaID != 42 {
		t.Errorf("resultado = %+v, queria a venda concluida", res)
	}
	if b.confirmou != 1 {
		t.Errorf("confirmou %d vezes, queria 1", b.confirmou)
	}
	// As duas foram TENTADAS: engolir a falha não é deixar de tentar.
	if len(j.entregou) != 1 || len(j.liberou) != 1 {
		t.Errorf("tentativas: entregou=%v liberou=%v", j.entregou, j.liberou)
	}
}

// Sem link com o jogo, a venda acontece igual. É o estado de quem sobe o webserver
// sem o endereço do servidor de jogo, e não pode impedir ninguém de comprar.
func TestSemLinkComOJogoAVendaAcontece(t *testing.T) {
	p, b := vendaBoa()

	res, err := Novo(p, b, nil, mudo()).ConferirEConcluir(context.Background(), "id-1")

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !res.Confirmada || b.confirmou != 1 {
		t.Errorf("resultado = %+v, confirmou=%d", res, b.confirmou)
	}
}

// O CORAÇÃO DA CORREÇÃO DO PAR DO SITE: estorno e contestação do Pix CONTINUAM com
// paid_at preenchido, porque foram pagos um dia. Decidir por "tem paid_at" entregaria
// item em cima de dinheiro que já voltou para o comprador.
//
// Aqui o paid_at está preenchido DE PROPÓSITO. Se alguém trocar a regra de volta para
// olhar só a hora, este teste é o que pega.
func TestEstornoEContestacaoNaoEntregamMesmoComPaidAt(t *testing.T) {
	for _, status := range []string{"refunded", "refunding", "med"} {
		agora := time.Now().UTC()
		p := &pontefake{t: Transacao{
			Existe: true, Status: status, Referencia: "ref-1",
			ValorCentavos: 5000, PagoEm: &agora,
		}}
		// achou=true: é cobrança NOSSA, que é o caso que precisa de gente.
		b := &bancofake{refGravada: "ref-1", achou: true}

		res, err := Novo(p, b, nil, mudo()).ConferirEConcluir(context.Background(), "id-1")

		if err != nil {
			t.Fatalf("%s: erro inesperado: %v", status, err)
		}
		if res.Confirmada || res.Entregue {
			t.Errorf("%s: resultado = %+v, NAO devia confirmar nem entregar", status, res)
		}
		if b.confirmou != 0 {
			t.Errorf("%s: chamou a confirmacao %d vezes, queria 0", status, b.confirmou)
		}
		if len(b.orfaos) != 1 || b.orfaos[0].Motivo != store.MotivoOrfaoContestado {
			t.Errorf("%s: fila da staff = %+v, queria uma linha de contestacao", status, b.orfaos)
		}
	}
}

// Estorno que NÃO é de cobrança nossa não enche a fila da staff. É o estorno da loja
// de doação chegando pelo mesmo webhook: pôr na fila faria a fila do mercado encher
// de coisa de outro sistema, e uma fila que enche de ruído deixa de ser olhada.
func TestEstornoDeOutroSistemaNaoVaiParaAFila(t *testing.T) {
	agora := time.Now().UTC()
	p := &pontefake{t: Transacao{
		Existe: true, Status: "refunded", Referencia: "outra", ValorCentavos: 5000, PagoEm: &agora,
	}}
	b := &bancofake{achou: false}

	if _, err := Novo(p, b, nil, mudo()).ConferirEConcluir(context.Background(), "id-1"); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(b.orfaos) != 0 {
		t.Errorf("fila = %+v, queria vazia", b.orfaos)
	}
}

// Status que este código não conhece NÃO ENTREGA. O vocabulário inteiro é
// documentação e nunca foi medido, então o desconhecido é o caso provável, não o
// improvável.
func TestStatusDesconhecidoNaoEntrega(t *testing.T) {
	agora := time.Now().UTC()
	p := &pontefake{t: Transacao{
		Existe: true, Status: "liquidado_parcial", Referencia: "ref-1",
		ValorCentavos: 5000, PagoEm: &agora,
	}}
	b := &bancofake{refGravada: "ref-1", achou: true, resultado: store.CobrancaConfirmada}

	res, err := Novo(p, b, nil, mudo()).ConferirEConcluir(context.Background(), "id-1")

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if res.Entregue || b.confirmou != 0 {
		t.Errorf("resultado = %+v, confirmou=%d — nao devia mexer em nada", res, b.confirmou)
	}
}

// "completed" com caixa e espaço diferentes continua sendo pago. Um contrato que
// muda de caixa não deveria parar de vender, e o contrário — comparar cru — falharia
// calado.
func TestStatusPagoToleraCaixaEEspaco(t *testing.T) {
	p, b := vendaBoa()
	p.t.Status = "  Completed "

	if _, err := Novo(p, b, nil, mudo()).ConferirEConcluir(context.Background(), "id-1"); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if b.confirmou != 1 {
		t.Errorf("confirmou %d vezes, queria 1", b.confirmou)
	}
}

// A REFERÊNCIA DIVERGENTE NÃO CONFIRMA NADA. É o caso em que confirmar pelo palpite
// daria o item ao comprador errado e marcaria vendido o item de outro vendedor.
func TestReferenciaDivergenteNaoConfirma(t *testing.T) {
	p, b := vendaBoa()
	p.t.Referencia = "ref-de-outra"

	res, err := Novo(p, b, nil, mudo()).ConferirEConcluir(context.Background(), "id-1")

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if res.Confirmada || b.confirmou != 0 {
		t.Errorf("resultado = %+v, confirmou=%d, queria nenhuma confirmacao", res, b.confirmou)
	}
	if len(b.orfaos) != 1 || b.orfaos[0].Motivo != store.MotivoOrfaoReferenciaDivergente {
		t.Errorf("fila = %+v, queria uma linha de divergencia", b.orfaos)
	}
}

// A CORRIDA NO NASCIMENTO: o aviso chegou antes de a gente gravar o identifier. A
// referência que a processadora devolveu é nossa de origem e vale — sem ela, todo
// pagamento cujo webhook corre mais rápido que o nosso UPDATE iria para a fila da
// staff, e a fila teria mais falso alarme do que problema.
func TestAvisoAntesDeGravarOIdentifierUsaAReferenciaDaProcessadora(t *testing.T) {
	p, b := vendaBoa()
	b.achou, b.refGravada = false, ""

	res, err := Novo(p, b, nil, mudo()).ConferirEConcluir(context.Background(), "id-1")

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !res.Confirmada || b.refConfirmada != "ref-1" {
		t.Errorf("resultado = %+v, confirmou a referencia %q", res, b.refConfirmada)
	}
	if len(b.orfaos) != 0 {
		t.Errorf("fila = %+v, queria vazia: nao e orfao, e corrida", b.orfaos)
	}
}

// Sem nenhuma das duas referências, o dinheiro entrou e não há a quem ligá-lo. Vai
// para a fila, e é o caso que a tabela de órfãos existe para não perder.
func TestSemReferenciaNenhumaVaiParaAFila(t *testing.T) {
	p, b := vendaBoa()
	b.achou, b.refGravada = false, ""
	p.t.Referencia = ""

	if _, err := Novo(p, b, nil, mudo()).ConferirEConcluir(context.Background(), "id-1"); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if b.confirmou != 0 {
		t.Errorf("confirmou %d vezes sem saber de que cobranca era", b.confirmou)
	}
	if len(b.orfaos) != 1 || b.orfaos[0].Motivo != store.MotivoOrfaoSemCobranca {
		t.Errorf("fila = %+v", b.orfaos)
	}
}

// Transação que a processadora não conhece NÃO é pagamento órfão: não existe
// pagamento nenhum. Pôr na fila aqui faria todo aviso forjado virar uma linha para
// alguém olhar, e é assim que uma fila deixa de ser olhada.
func TestTransacaoInexistenteNaoEnchemAFila(t *testing.T) {
	p := &pontefake{t: Transacao{Existe: false}}
	b := &bancofake{}

	res, err := Novo(p, b, nil, mudo()).ConferirEConcluir(context.Background(), "id-1")

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if res.Tratado || len(b.orfaos) != 0 || b.confirmou != 0 {
		t.Errorf("resultado = %+v, fila = %+v", res, b.orfaos)
	}
}

// A ORIGEM DA HORA fica gravada, e as duas pontas importam: com paid_at vale o
// relógio da processadora; sem ele, o do servidor, que é MAIS TARDE que o real e
// portanto erra para o lado de "fora do prazo" — o lado que devolve dinheiro em vez
// de dar item de graça.
func TestOrigemDaHoraSegueQuemTemOPaidAt(t *testing.T) {
	quando := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)

	p, b := vendaBoa()
	p.t.PagoEm = &quando
	if _, err := Novo(p, b, nil, mudo()).ConferirEConcluir(context.Background(), "id-1"); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if b.origem != store.HoraDaProcessadora || !b.pagoEm.Equal(quando) {
		t.Errorf("com paid_at: origem=%q pagoEm=%v", b.origem, b.pagoEm)
	}

	p2, b2 := vendaBoa()
	p2.t.PagoEm = nil
	antes := time.Now().UTC().Add(-time.Second)
	if _, err := Novo(p2, b2, nil, mudo()).ConferirEConcluir(context.Background(), "id-1"); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if b2.origem != store.HoraDoServidor {
		t.Errorf("sem paid_at: origem=%q, queria servidor", b2.origem)
	}
	if b2.pagoEm.Before(antes) {
		t.Errorf("sem paid_at: pagoEm=%v, queria o relogio de agora", b2.pagoEm)
	}
}

// O valor observado CHEGA ao banco, que é quem compara. Se ele não chegasse, a
// divergência de valor nunca seria detectada — e pagar menos e receber o item é
// comprar com desconto de si mesmo.
func TestOValorObservadoChegaAoBanco(t *testing.T) {
	p, b := vendaBoa()
	p.t.ValorCentavos = 4999

	if _, err := Novo(p, b, nil, mudo()).ConferirEConcluir(context.Background(), "id-1"); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if b.valorVisto != 4999 {
		t.Errorf("valor que chegou ao banco = %d, queria 4999", b.valorVisto)
	}
}

// Pagamento fora do prazo pede o reembolso, e a FALHA de marcar isso É ERRO. Sem a
// marca, o dinheiro de alguém fica sem destino e ninguém sabe — é a única falha deste
// caminho que tem de fazer quem chamou repetir.
func TestPagoSemItemMarcaReembolsoEAFalhaEErro(t *testing.T) {
	p, b := vendaBoa()
	b.resultado = store.CobrancaPagaSemItem
	b.venda.PagoComAtraso = true

	res, err := Novo(p, b, nil, mudo()).ConferirEConcluir(context.Background(), "id-1")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if res.Entregue {
		t.Error("entregue = true num pagamento sem item")
	}
	if len(b.reembolsos) != 1 || b.reembolsos[0] != 42 {
		t.Errorf("reembolsos = %v, queria [42]", b.reembolsos)
	}

	p2, b2 := vendaBoa()
	b2.resultado = store.CobrancaPagaSemItem
	b2.erroReemb = errors.New("banco fora do ar")
	if _, err := Novo(p2, b2, nil, mudo()).ConferirEConcluir(context.Background(), "id-1"); err == nil {
		t.Error("falhar em marcar o reembolso NAO virou erro; o dinheiro ficaria sem destino")
	}
}

// Valor divergente não entrega e não vai para a fila dos órfãos: ele tem fila
// própria, a ValoresDivergentes, e a cobrança fica ABERTA segurando o item.
func TestValorDivergenteNaoEntregaENaoEOrfao(t *testing.T) {
	p, b := vendaBoa()
	b.resultado = store.CobrancaValorDivergente

	res, err := Novo(p, b, nil, mudo()).ConferirEConcluir(context.Background(), "id-1")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if res.Entregue {
		t.Error("entregue = true com valor divergente")
	}
	if len(b.orfaos) != 0 {
		t.Errorf("fila de orfaos = %+v, queria vazia: a fila dele e outra", b.orfaos)
	}
}

// Um resultado de confirmação que este arquivo não conhece NÃO entrega.
//
// Teste de um caminho que hoje é inalcançável, de propósito: ele existe para o dia em
// que alguém acrescentar um estado no banco. Sem ele, o estado novo cairia em
// "entrega o item" sem ninguém ter escrito essa decisão.
func TestResultadoDesconhecidoNaoEntrega(t *testing.T) {
	p, b := vendaBoa()
	b.resultado = store.ResultadoCobranca(99)
	j := &jogofake{emJogo: true}

	res, err := Novo(p, b, j, mudo()).ConferirEConcluir(context.Background(), "id-1")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if res.Entregue {
		t.Error("entregue = true para um resultado desconhecido")
	}
	if len(j.entregou) != 0 {
		t.Errorf("mandou entregar: %v", j.entregou)
	}
}

// Falha na CONSULTA é erro, e tem de ser: sem ela não se decide nada, e quem chamou
// precisa repetir. Devolver "não tratei" faria o site desistir de um pagamento real.
func TestFalhaNaConsultaEErro(t *testing.T) {
	p := &pontefake{err: errors.New("ponte fora do ar")}
	b := &bancofake{}

	if _, err := Novo(p, b, nil, mudo()).ConferirEConcluir(context.Background(), "id-1"); err == nil {
		t.Error("falha na consulta nao virou erro")
	}
	if b.confirmou != 0 {
		t.Error("confirmou sem ter conseguido consultar")
	}
}

// Identifier vazio não gasta consulta. Barato, e evita que um campo não preenchido
// vire tráfego na processadora.
func TestIdentifierVazioNaoConsulta(t *testing.T) {
	p := &pontefake{}
	b := &bancofake{}

	if _, err := Novo(p, b, nil, mudo()).ConferirEConcluir(context.Background(), "   "); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if p.consultas != 0 {
		t.Errorf("consultou %d vezes com identifier vazio", p.consultas)
	}
}

// Não conseguir LER a cobrança gravada não confirma nada. Confirmar sem a conferência
// seria desligar a proteção da referência divergente justamente quando o banco está
// tropeçando.
func TestFalhaAoLerACobrancaNaoConfirma(t *testing.T) {
	p, b := vendaBoa()
	b.erroAchar = errors.New("banco fora do ar")

	res, err := Novo(p, b, nil, mudo()).ConferirEConcluir(context.Background(), "id-1")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if res.Confirmada || b.confirmou != 0 {
		t.Errorf("resultado = %+v, confirmou=%d", res, b.confirmou)
	}
}

// A descrição que vai para a processadora NÃO pode conter "rmt:": a ponte recusa com
// 400, porque a marca é ela que põe e uma segunda marca no texto faria a consulta ler
// a referência errada.
func TestDescricaoNaoLevaAMarcaENaoPassaDoLimite(t *testing.T) {
	casos := []struct {
		nome, item string
		refino     int
	}{
		{"item comum", "Espada Sagrada", 9},
		{"sem nome", "", 0},
		{"nome com a marca dentro", "rmt:coisa", 0},
		{"nome comprido", "Armadura Divina Celestial Sagrada dos Ancestrais do Norte", 11},
		{"nome com controle", "Botas\x00Douradas\n(N)", 0},
	}
	for _, c := range casos {
		d := Descricao(c.item, c.refino)
		if len([]rune(d)) > LimiteDaDescricao {
			t.Errorf("%s: %d runas, limite %d: %q", c.nome, len([]rune(d)), LimiteDaDescricao, d)
		}
		for _, r := range d {
			if r < 0x20 {
				t.Errorf("%s: caractere de controle na descricao: %q", c.nome, d)
				break
			}
		}
	}
	// O nome que CONTÉM a marca é o caso que mais importa, e hoje ele passa a marca
	// adiante — o nome do item vem do catálogo do cliente legado, que ninguém nosso
	// controla. Ver o teste seguinte.
}

// A marca é ARRANCADA do nome do item, e não só evitada no texto que a gente escreve.
//
// O nome vem do ItemList.csv do cliente legado. Ninguém nosso escolhe esses nomes, e
// um item chamado "rmt:algo" — por acidente ou porque alguém editou o conteúdo — faria
// a ponte recusar TODA cobrança daquele item com 400, e a recusa não diria por quê.
func TestAMarcaEArrancadaDoNomeDoItem(t *testing.T) {
	d := Descricao("Espada rmt:falsa", 0)
	if contemMarca(d) {
		t.Errorf("a descricao levou a marca adiante: %q", d)
	}
}

// VALOR AUSENTE NÃO ENTREGA, e este é o guarda que faltava.
//
// A regra é status E valor E referência, mas o valor é conferido no banco, e lá o
// ZERO significa "não observado" e PULA a conferência. Então uma resposta
// "completed" com valor zero — ponte quebrando o contrato, fonte antiga sem o campo,
// erro de leitura — entregaria o item sem prova nenhuma de quanto entrou.
//
// Vale para o zero e para o negativo: os dois não são "um valor que bate".
func TestValorAusenteNaoEntrega(t *testing.T) {
	for _, valor := range []int64{0, -1} {
		p, b := vendaBoa()
		p.t.ValorCentavos = valor

		res, err := Novo(p, b, nil, mudo()).ConferirEConcluir(context.Background(), "id-1")

		if err != nil {
			t.Fatalf("valor %d: erro inesperado: %v", valor, err)
		}
		if res.Confirmada || res.Entregue {
			t.Errorf("valor %d: resultado = %+v, NAO devia confirmar nem entregar", valor, res)
		}
		if b.confirmou != 0 {
			t.Errorf("valor %d: chamou a confirmacao %d vezes", valor, b.confirmou)
		}
		if len(b.orfaos) != 1 || b.orfaos[0].Motivo != store.MotivoOrfaoSemValor {
			t.Errorf("valor %d: fila = %+v, queria uma linha de valor ausente", valor, b.orfaos)
		}
	}
}

// O IDENTIFIER QUE A CORRIDA DEIXOU PARA TRÁS É COMPLETADO. Sem isso a cobrança fica
// PAGA com identifier nulo, e quem precisar pedir o reembolso dela não tem por onde.
func TestIdentifierQueFicouParaTrasEGravado(t *testing.T) {
	p, b := vendaBoa()
	b.achou, b.refGravada = false, "" // a corrida: o aviso chegou antes do UPDATE

	if _, err := Novo(p, b, nil, mudo()).ConferirEConcluir(context.Background(), "id-1"); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(b.identGravados) != 1 || b.identGravados[0] != [2]string{"ref-1", "id-1"} {
		t.Errorf("gravou %v, queria a referencia com o identifier do aviso", b.identGravados)
	}
}

// E NÃO grava quando o identifier já estava lá: sobrescrever não é trabalho desta
// função, e dois identifiers diferentes na mesma cobrança é caso para uma pessoa.
func TestNaoRegravaOIdentifierQueJaEstava(t *testing.T) {
	p, b := vendaBoa() // achou=true, o caminho normal

	if _, err := Novo(p, b, nil, mudo()).ConferirEConcluir(context.Background(), "id-1"); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(b.identGravados) != 0 {
		t.Errorf("gravou %v numa cobranca que ja tinha identifier", b.identGravados)
	}
}

// Falhar em completar o identifier é AVISO e não erro: a venda já está gravada, e
// insistir não melhora nada de ninguém.
func TestFalhaAoCompletarOIdentifierNaoDesfazAVenda(t *testing.T) {
	p, b := vendaBoa()
	b.achou, b.refGravada = false, ""
	b.erroGravarID = errors.New("banco fora do ar")

	res, err := Novo(p, b, nil, mudo()).ConferirEConcluir(context.Background(), "id-1")
	if err != nil {
		t.Fatalf("a falha de completar o identifier virou erro: %v", err)
	}
	if !res.Confirmada || !res.Entregue {
		t.Errorf("resultado = %+v, a venda tinha de continuar valendo", res)
	}
}

// E não tenta gravar quando a referência não é de cobrança nossa: gravar o identifier
// contra uma referência que não existe não faz nada e esconde o problema real.
func TestNaoGravaOIdentifierQuandoNaoAchouCobranca(t *testing.T) {
	p, b := vendaBoa()
	b.achou, b.refGravada = false, ""
	b.resultado = store.CobrancaNaoEncontrada

	if _, err := Novo(p, b, nil, mudo()).ConferirEConcluir(context.Background(), "id-1"); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(b.identGravados) != 0 {
		t.Errorf("gravou %v para uma referencia que nao existe", b.identGravados)
	}
}
