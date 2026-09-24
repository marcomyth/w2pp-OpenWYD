package rmtrepasse

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/store"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/ponte"
)

func mudo() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

type pontefake struct {
	resp ponte.RespostaRepasse
	err  error

	chamadas   int
	referencia string
	tipo       string
	documento  string
	centavos   int64
}

func (p *pontefake) Repassar(_ context.Context, referencia string, centavos int64,
	_, tipoChave, documento string,
) (ponte.RespostaRepasse, error) {
	p.chamadas++
	p.referencia, p.tipo, p.documento, p.centavos = referencia, tipoChave, documento, centavos
	return p.resp, p.err
}

type bancofake struct {
	fila      []store.RepasseAPagar
	erroAbrir error

	abertas   []int64
	enviados  []string
	incertos  []int64
	recusados []int64
	fechadas  []store.EstadoRepasse
}

func (b *bancofake) RepassesAPagar(context.Context, int) ([]store.RepasseAPagar, error) {
	return b.fila, nil
}

func (b *bancofake) AbrirTentativa(_ context.Context, repasseID int64, _ string) (store.TentativaDeRepasse, error) {
	if b.erroAbrir != nil {
		return store.TentativaDeRepasse{}, b.erroAbrir
	}
	b.abertas = append(b.abertas, repasseID)
	return store.TentativaDeRepasse{
		ID: repasseID * 10, RepasseID: repasseID, Numero: 1,
		Referencia: store.ReferenciaDaTentativa(repasseID, 1),
	}, nil
}

func (b *bancofake) FecharTentativa(_ context.Context, _ int64, resultado store.EstadoRepasse,
	_ string, _ *int32, _, _ string,
) error {
	b.fechadas = append(b.fechadas, resultado)
	return nil
}

func (b *bancofake) MarcarRepasseEnviado(_ context.Context, _ int64, saque string, _ int64) error {
	b.enviados = append(b.enviados, saque)
	return nil
}

func (b *bancofake) MarcarRepasseIncerto(_ context.Context, id int64, _ string) error {
	b.incertos = append(b.incertos, id)
	return nil
}

func (b *bancofake) MarcarRepasseRecusado(_ context.Context, id int64, _ *int32, _, _ string) error {
	b.recusados = append(b.recusados, id)
	return nil
}

func umaDivida() store.RepasseAPagar {
	return store.RepasseAPagar{
		ID: 7, CobrancaID: 70, Referencia: "ref-cobranca", VendedorConta: 9,
		ValorCentavos: 4990, ChavePix: "v@exemplo.com",
		TipoChave: store.ChavePixEmail, Documento: "11144477735",
	}
}

// O INCERTO NUNCA VIRA RECUSA, e este é o teste que mais importa deste pacote.
//
// A ponte devolve o incerto como ERRO Go, e um erro parece falha — e falha, no resto do
// sistema, é coisa que se tenta de novo. Aqui não: o pagamento PODE ter saído, não há
// consulta de saque para desempatar, e reenviar é a única coisa que não se desfaz.
//
// Se este caminho marcasse "recusado", a dívida voltaria para PENDENTE e a varredura
// seguinte pagaria o vendedor pela segunda vez.
func TestIncertoNaoViraRecusa(t *testing.T) {
	p := &pontefake{err: ponte.ErrIncerta}
	b := &bancofake{fila: []store.RepasseAPagar{umaDivida()}}

	rod := Novo(p, b, mudo()).PagarPendentes(context.Background(), 10)

	// O INCERTO TEM CONTADOR PRÓPRIO, e não some nem vira "pago".
	//
	// Somem os dois erros: contá-lo como pago faria o log dizer que a gente pagou
	// alguém que talvez não tenha recebido; tirá-lo de tudo faria uma rodada com três
	// incertos sair como "nada aconteceu" — e três pagamentos que talvez tenham saído é
	// o oposto de nada.
	if rod.Aceitos != 0 || rod.Falhas != 0 || rod.Recusados != 0 {
		t.Errorf("rodada = %+v; o incerto nao e nenhum dos outros tres", rod)
	}
	if rod.Incertos != 1 {
		t.Errorf("incertos = %d, quero 1: e o numero que precisa gritar", rod.Incertos)
	}
	if len(b.incertos) != 1 || b.incertos[0] != 7 {
		t.Errorf("marcou incerto = %v, queria [7]", b.incertos)
	}
	if len(b.recusados) != 0 {
		t.Errorf("marcou RECUSADO um incerto: a divida voltaria para a fila e pagaria duas vezes")
	}
	if len(b.enviados) != 0 {
		t.Errorf("marcou enviado = %v", b.enviados)
	}
	if len(b.fechadas) != 1 || b.fechadas[0] != store.RepasseIncerto {
		t.Errorf("a tentativa fechou como %v", b.fechadas)
	}
}

// UM ESTADO DESCONHECIDO TAMBÉM VIRA INCERTO, e não recusa.
//
// A escolha é deliberada e é a cara: uma recusa libera a dívida para tentativa nova, e
// mandar de novo o que talvez tenha sido pago é o erro que não se desfaz. Uma linha
// parada esperando gente custa tempo; uma linha paga duas vezes custa dinheiro.
func TestEstadoDesconhecidoViraIncertoENaoRecusa(t *testing.T) {
	p := &pontefake{resp: ponte.RespostaRepasse{Estado: "processando"}}
	b := &bancofake{fila: []store.RepasseAPagar{umaDivida()}}

	Novo(p, b, mudo()).PagarPendentes(context.Background(), 10)

	if len(b.incertos) != 1 {
		t.Errorf("incertos = %v, queria a divida parada para uma pessoa olhar", b.incertos)
	}
	if len(b.recusados) != 0 {
		t.Error("um estado desconhecido liberou a divida para nova tentativa")
	}
}

// ACEITO E REPETIDO SÃO OS DOIS SUCESSOS, e o repetido é o que faz a idempotência valer:
// a nossa chamada anterior se perdeu, esta MESMA tentativa foi remandada, e a ponte
// devolveu o id da primeira vez sem chamar a processadora de novo.
func TestAceitoERepetidoSaoSucesso(t *testing.T) {
	for _, estado := range []string{"aceito", "repetido"} {
		p := &pontefake{resp: ponte.RespostaRepasse{Estado: estado, ChaveGateway: "saque-1"}}
		b := &bancofake{fila: []store.RepasseAPagar{umaDivida()}}

		rod := Novo(p, b, mudo()).PagarPendentes(context.Background(), 10)

		if rod.Aceitos != 1 || rod.Falhas != 0 || rod.Incertos != 0 {
			t.Errorf("%s: rodada = %+v", estado, rod)
		}
		if len(b.enviados) != 1 || b.enviados[0] != "saque-1" {
			t.Errorf("%s: enviados = %v; sem o id do saque a taxa fica sem dono", estado, b.enviados)
		}
	}
}

// A RECUSA PARA NA FILA DA STAFF e não vira incerto: ela tem CERTEZA de que nada saiu, e
// é o único estado em que tentar de novo é seguro.
func TestRecusadoMarcaRecusadoENaoIncerto(t *testing.T) {
	http := int32(422)
	p := &pontefake{resp: ponte.RespostaRepasse{
		Estado: "recusado", HTTPSyncpay: &http,
		CodigoSyncpay: "PIX_KEY_NOT_FOUND", Motivo: "chave nao existe",
	}}
	b := &bancofake{fila: []store.RepasseAPagar{umaDivida()}}

	Novo(p, b, mudo()).PagarPendentes(context.Background(), 10)

	if len(b.recusados) != 1 {
		t.Errorf("recusados = %v", b.recusados)
	}
	if len(b.incertos) != 0 {
		t.Error("uma recusa com certeza virou incerto; ela ficaria travada sem precisar")
	}
}

// TIPO DE CHAVE QUE O MAPA NÃO CONHECE NÃO VAI AO FIO.
//
// Mandar vazio é 400 na ponte, e o 400 não diz qual era o tipo. E isto não é erro do
// vendedor: é defeito nosso, então nem gasta uma tentativa.
func TestTipoDesconhecidoNaoChamaAPonte(t *testing.T) {
	d := umaDivida()
	d.TipoChave = store.TipoChavePix(99)
	p := &pontefake{}
	b := &bancofake{fila: []store.RepasseAPagar{d}}

	Novo(p, b, mudo()).PagarPendentes(context.Background(), 10)

	if p.chamadas != 0 {
		t.Error("chamou a ponte com um tipo de chave que o mapa nao traduz")
	}
	if len(b.abertas) != 0 {
		t.Error("gastou uma tentativa num defeito nosso")
	}
	if len(b.recusados) != 1 {
		t.Errorf("recusados = %v, queria a divida parada com o motivo", b.recusados)
	}
}

// O QUE VAI AO FIO é a referência DA TENTATIVA, e não a da cobrança.
//
// A da cobrança travaria para sempre no primeiro incerto, e a dívida nunca mais poderia
// ser paga. Este teste é o que impede alguém de "simplificar" trocando as duas.
func TestOQueVaiAoFioEAReferenciaDaTentativa(t *testing.T) {
	p := &pontefake{resp: ponte.RespostaRepasse{Estado: "aceito", ChaveGateway: "s"}}
	d := umaDivida()
	b := &bancofake{fila: []store.RepasseAPagar{d}}

	Novo(p, b, mudo()).PagarPendentes(context.Background(), 10)

	if p.referencia == d.Referencia {
		t.Error("mandou a referencia da COBRANCA; no primeiro incerto a divida travaria para sempre")
	}
	if p.referencia != store.ReferenciaDaTentativa(d.ID, 1) {
		t.Errorf("referencia = %q", p.referencia)
	}
	if p.tipo != "email" {
		t.Errorf("tipo = %q, quero email", p.tipo)
	}
	if p.documento != "11144477735" || p.centavos != 4990 {
		t.Errorf("documento = %q centavos = %d", p.documento, p.centavos)
	}
}

// A FALHA DE UMA NÃO PARA AS OUTRAS: uma chave inválida de um vendedor não pode segurar
// o pagamento de quem está atrás dele na fila.
func TestUmaFalhaNaoParaAFila(t *testing.T) {
	ruim := umaDivida()
	ruim.ID = 1
	boa := umaDivida()
	boa.ID = 2

	p := &pontefake{resp: ponte.RespostaRepasse{Estado: "aceito", ChaveGateway: "s"}}
	b := &bancofake{fila: []store.RepasseAPagar{ruim, boa}, erroAbrir: nil}

	// A primeira falha ao abrir a tentativa; a segunda tem de ser tentada assim mesmo.
	chamou := 0
	falhaNaPrimeira := &bancoQueFalhaUmaVez{bancofake: b, falharNo: 1, chamou: &chamou}

	rod := Novo(p, falhaNaPrimeira, mudo()).PagarPendentes(context.Background(), 10)

	if rod.Falhas != 1 {
		t.Errorf("falhas = %d, queria 1", rod.Falhas)
	}
	if rod.Aceitos != 1 {
		t.Errorf("aceitos = %d; a divida de tras nao foi tentada", rod.Aceitos)
	}
}

// bancoQueFalhaUmaVez faz o AbrirTentativa falhar para um repasse específico.
type bancoQueFalhaUmaVez struct {
	*bancofake
	falharNo int64
	chamou   *int
}

func (b *bancoQueFalhaUmaVez) AbrirTentativa(ctx context.Context, repasseID int64, por string) (store.TentativaDeRepasse, error) {
	if repasseID == b.falharNo {
		return store.TentativaDeRepasse{}, errors.New("a divida nao esta pendente")
	}
	return b.bancofake.AbrirTentativa(ctx, repasseID, por)
}

// A RODADA VAZIA NÃO ESCREVE NADA, e a com incerto escreve em WARN.
//
// A varredura roda a cada dois minutos: uma linha por rodada encheria o log de "não fiz
// nada" e afogaria as que dizem alguma coisa. E o incerto sobe de nível mesmo quando o
// resto correu bem, porque ele é o único número da linha que exige uma pessoa.
func TestRegistrarEscolheONivelPeloQueSignifica(t *testing.T) {
	var linhas []slog.Record
	log := slog.New(gravador{fn: func(r slog.Record) { linhas = append(linhas, r) }})

	Rodada{}.Registrar(log)
	if len(linhas) != 0 {
		t.Errorf("a rodada vazia escreveu %d linha(s); o log encheria de 'nao fiz nada'", len(linhas))
	}

	Rodada{Aceitos: 2}.Registrar(log)
	if len(linhas) != 1 || linhas[0].Level != slog.LevelInfo {
		t.Fatalf("rodada boa = %+v", linhas)
	}

	Rodada{Aceitos: 2, Incertos: 1}.Registrar(log)
	if len(linhas) != 2 {
		t.Fatalf("a rodada com incerto nao escreveu")
	}
	if linhas[1].Level != slog.LevelWarn {
		t.Errorf("a rodada com incerto saiu em %v; ela tem de subir de nivel mesmo com o resto bom",
			linhas[1].Level)
	}
}

// gravador guarda as linhas em vez de escrevê-las.
type gravador struct{ fn func(slog.Record) }

func (g gravador) Enabled(context.Context, slog.Level) bool      { return true }
func (g gravador) Handle(_ context.Context, r slog.Record) error { g.fn(r); return nil }
func (g gravador) WithAttrs([]slog.Attr) slog.Handler            { return g }
func (g gravador) WithGroup(string) slog.Handler                 { return g }
