package handler

import (
	"errors"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O FIM DO ANÚNCIO, do lado do jogo.
//
// Estes testes cobrem o que faltava para o escrow ser um ciclo e não uma
// armadilha: o item preso tem de conseguir sair. Sem eles a única saída era
// vender, e quem montasse uma barraca em dinheiro real e desistisse ficaria com o
// item travado para sempre.

// esperaEncerramento espera a ida ao banco, que roda por GoDetached.
func esperaEncerramento(t *testing.T, db *fakeDB) []int64 {
	t.Helper()
	for i := 0; i < 200; i++ {
		if ids := db.encerrouAnuncios(); len(ids) > 0 {
			return ids
		}
		esperaUmPouco()
	}
	t.Fatal("a barraca desceu e ninguem mandou encerrar o anuncio")
	return nil
}

// esperaMarcaSolta espera o cadeado sair do baú, que acontece na volta ao laço.
func esperaMarcaSolta(t *testing.T, w *world.World, conta int64, slot int) bool {
	t.Helper()
	for i := 0; i < 200; i++ {
		solto := false
		noLacoDoMundo(t, w, func(w *world.World) {
			if c := w.Cargo(conta); c != nil {
				solto = c.Items[slot].AnuncioRMT == 0
			}
		})
		if solto {
			return true
		}
		esperaUmPouco()
	}
	return false
}

// FECHAR A BARRACA CANCELA O ANÚNCIO E DEVOLVE O ITEM.
//
// É o critério de aceite deste caminho inteiro. O jogo já dizia ao vendedor
// "cancele o anúncio antes de vendê-lo de novo", e até aqui não existia como.
func TestFecharBarracaCancelaOAnuncioEDevolveOItem(t *testing.T) {
	db := bancoDeAnuncio()
	addr, stop, w := startServerNovato(t, db)
	defer stop()
	c := enterWorldAs(t, addr, "tester")
	defer c.Close()
	drena(t, c)

	abreBarraca(t, c, "Loja", 0, 5000, protocol.LojaMoedaRMT)
	// O id que o fake devolveu ao abrir é o que tem de voltar ao fechar.
	if !esperaMarcaPosta(t, w, 7, 0) {
		t.Fatal("o anuncio nem chegou a marcar o item")
	}
	var anuncio int64
	noLacoDoMundo(t, w, func(w *world.World) { anuncio = w.Cargo(7).Items[0].AnuncioRMT })

	send(t, c, protocol.MsgQuitTrade, nil)

	ids := esperaEncerramento(t, db)
	if len(ids) != 1 || ids[0] != anuncio {
		t.Errorf("mandou encerrar %v, quero [%d]", ids, anuncio)
	}
	if !esperaMarcaSolta(t, w, 7, 0) {
		t.Error("a barraca desceu e o cadeado ficou; o item esta preso para sempre")
	}
	// E o item CONTINUA no baú: soltar o cadeado não é retirar. Confundir as duas
	// coisas apagaria o item de quem não vendeu nada.
	noLacoDoMundo(t, w, func(w *world.World) {
		if w.Cargo(7).Items[0].Empty() {
			t.Error("o item sumiu do bau de quem nao vendeu nada")
		}
	})
}

// COM COBRANÇA ABERTA O CADEADO FICA.
//
// Alguém está com o QR na mão. Soltar o item agora seria deixar vendê-lo de novo
// enquanto o Pix do primeiro comprador ainda está a caminho — e a confirmação
// dele chegaria sem ter o que entregar.
func TestBarracaFechadaComCobrancaAbertaNaoSoltaOItem(t *testing.T) {
	db := bancoDeAnuncio()
	db.encerrados = []world.AnuncioEncerrado{{AnuncioID: 900, CargoSlot: 0, CobrancaAberta: true}}
	addr, stop, w := startServerNovato(t, db)
	defer stop()
	c := enterWorldAs(t, addr, "tester")
	defer c.Close()
	drena(t, c)

	abreBarraca(t, c, "Loja", 0, 5000, protocol.LojaMoedaRMT)
	if !esperaMarcaPosta(t, w, 7, 0) {
		t.Fatal("o anuncio nem chegou a marcar o item")
	}

	send(t, c, protocol.MsgQuitTrade, nil)
	esperaEncerramento(t, db)

	if esperaMarcaSolta(t, w, 7, 0) {
		t.Error("soltou o item com uma cobranca aberta; o Pix a caminho ficaria sem o que entregar")
	}
}

// Barraca de OURO não vai ao banco. Uma viagem de rede por barraca fechada, em
// todo mundo, para servir aos poucos que vendem em reais.
func TestFecharBarracaDeOuroNaoVaiAoBanco(t *testing.T) {
	db := bancoDeAnuncio()
	addr, stop, _ := startServerNovato(t, db)
	defer stop()
	c := enterWorldAs(t, addr, "tester")
	defer c.Close()
	drena(t, c)

	abreBarraca(t, c, "Loja", 0, 1000, protocol.LojaMoedaOuro)
	send(t, c, protocol.MsgQuitTrade, nil)
	drena(t, c)

	if ids := db.encerrouAnuncios(); len(ids) != 0 {
		t.Errorf("uma barraca de ouro mandou encerrar %v", ids)
	}
}

// Banco fora do ar ao fechar: o cadeado FICA, e isso é o certo.
//
// Soltar por otimismo deixaria o anúncio ativo no banco e o item livre no jogo —
// o mesmo item vendido duas vezes. Ficar preso é o erro barato: a faxina do
// próximo login refaz a pergunta.
func TestBancoForaDoArAoFecharNaoSoltaOItem(t *testing.T) {
	db := bancoDeAnuncio()
	db.erroEncerrar = errors.New("banco fora do ar")
	addr, stop, w := startServerNovato(t, db)
	defer stop()
	c := enterWorldAs(t, addr, "tester")
	defer c.Close()
	drena(t, c)

	abreBarraca(t, c, "Loja", 0, 5000, protocol.LojaMoedaRMT)
	if !esperaMarcaPosta(t, w, 7, 0) {
		t.Fatal("o anuncio nem chegou a marcar o item")
	}

	send(t, c, protocol.MsgQuitTrade, nil)

	if esperaMarcaSolta(t, w, 7, 0) {
		t.Error("soltou o cadeado sem o banco ter confirmado o cancelamento")
	}
}

// A FAXINA DO LOGIN solta o cadeado que já não segura nada.
//
// É o que cobre todos os casos em que o anúncio acabou sem o vendedor estar em
// jogo — que são a maioria deles, porque o jeito mais comum de uma barraca
// descer é o dono sair.
func TestFaxinaDoLoginSoltaOCadeadoMorto(t *testing.T) {
	db := newDB()
	db.loads = map[int64]world.CharacterState{
		7: {Slot: 0, Name: "Vendedor", Level: 1, HP: 1000, MaxHP: 1000},
	}
	var cargo world.CargoState
	cargo.Items[2] = world.Item{Index: 1030, AnuncioRMT: 4242}
	cargo.Items[5] = world.Item{Index: 2020, AnuncioRMT: 777} // este NÃO está na lista
	db.accounts["tester"].cargo = cargo
	db.slotsSoltos = map[int64][]int16{7: {2}}

	addr, stop, w := startServerNovato(t, db)
	defer stop()
	c := entraNaConta(t, addr)
	defer c.Close()
	drena(t, c)

	bau := bauDoVendedor(t, w)
	if bau.Items[2].AnuncioRMT != 0 {
		t.Error("a faxina nao soltou o cadeado que o banco apontou")
	}
	if bau.Items[2].Empty() {
		t.Error("a faxina APAGOU o item; ela devolve, nao retira")
	}
	if bau.Items[5].AnuncioRMT != 777 {
		t.Error("soltou um cadeado que o banco nao apontou; esse item pode estar a venda agora")
	}
}

// E a faxina NÃO avisa o jogador. Nada mudou no baú que ele pudesse notar — o
// item está lá, como sempre esteve. Um aviso sobre uma trava que ele nunca viu
// seria conversa sobre encanamento.
func TestFaxinaDoLoginNaoAvisa(t *testing.T) {
	db := newDB()
	db.loads = map[int64]world.CharacterState{
		7: {Slot: 0, Name: "Vendedor", Level: 1, HP: 1000, MaxHP: 1000},
	}
	var cargo world.CargoState
	cargo.Items[2] = world.Item{Index: 1030, AnuncioRMT: 4242}
	db.accounts["tester"].cargo = cargo
	db.slotsSoltos = map[int64][]int16{7: {2}}

	addr, stop, _ := startServerNovato(t, db)
	defer stop()
	c := entraNaConta(t, addr)
	defer c.Close()

	for {
		ty, _, ok := readMaybe(t, c)
		if !ok {
			break
		}
		if ty == protocol.MsgMessagePanel {
			t.Error("a faxina falou com o jogador sobre uma trava que ele nunca viu")
		}
	}
}

// PRATELEIRA COM ANÚNCIO VIVO NÃO VIRA OURO.
//
// Sem esta recusa o vendedor fica com uma oferta que NUNCA vende: a compra
// confere o cadeado e recusa, sempre, e ele não teria como saber por quê. Pior,
// pensaria ter vendido em ouro uma coisa ainda prometida por Pix.
func TestPrateleiraComAnuncioVivoNaoViraOuro(t *testing.T) {
	db := bancoDeAnuncio()
	addr, stop, w := startServerNovato(t, db)
	defer stop()
	c := enterWorldAs(t, addr, "tester")
	defer c.Close()
	drena(t, c)

	abreBarraca(t, c, "Loja", 0, 5000, protocol.LojaMoedaRMT)
	if !esperaMarcaPosta(t, w, 7, 0) {
		t.Fatal("o anuncio nem chegou a marcar o item")
	}
	drena(t, c)

	send(t, c, protocol.MsgLojaMoeda,
		(&protocol.LojaMoedaBody{Slot: 0, Moeda: protocol.LojaMoedaOuro}).Encode())

	if !recebeu(t, c, msgAnuncioVivoNaPrateleira) {
		t.Error("recusou em silencio, ou deixou virar")
	}
	drena(t, c)
	v := pedeVitrine(t, c, 0, protocol.LojaFiltroTodos)
	if v.Qtd != 1 || v.Ofertas[0].Moeda != protocol.LojaMoedaRMT {
		t.Errorf("a prateleira ficou em %d oferta(s) na moeda %d, quero 1 em RMT",
			v.Qtd, v.Ofertas[0].Moeda)
	}
}

// O COMPRADOR QUE SAI DO JOGO leva as cobranças dele junto.
//
// Ele não vai voltar para aquele QR, e cada cobrança aberta prende o item de
// OUTRA pessoa até o prazo acabar. O vendedor não fez nada de errado e está
// esperando.
//
// A volta NÃO solta cadeado nenhum, e isso é decisão e não esquecimento: os
// cadeados são do VENDEDOR, que é outra conta e pode nem estar em jogo. Quem os
// solta é a reconciliação do login dele.
func TestCompradorQueSaiFechaAsCobrancas(t *testing.T) {
	db := newDB()
	addr, stop, _ := startServerNovato(t, db)
	defer stop()
	c := enterWorldAs(t, addr, "tester")
	drena(t, c)

	_ = c.Close()

	pediu := false
	for i := 0; i < 200; i++ {
		for _, conta := range db.cancelouComprador() {
			if conta == 7 {
				pediu = true
			}
		}
		if pediu {
			break
		}
		esperaUmPouco()
	}
	if !pediu {
		t.Error("o comprador saiu e as cobrancas dele ficaram abertas, prendendo item de terceiro")
	}
}

// LOGIN DUPLICADO NÃO CANCELA A VENDA DA BARRACA VIVA.
//
// A reconciliação só é segura porque "quem está entrando não tem barraca de pé".
// Numa tentativa que vai ser RECUSADA — a conta já em jogo — essa frase é falsa:
// quem tem a barraca é a sessão antiga, que continua viva.
//
// O buraco era de ordem. A reconciliação vinha de carona no mesmo pedido de login
// (dbclient), e o `accountInUse` (handler/login.go) só derruba a conexão nova
// DEPOIS de esse pedido voltar. Quando ele derrubava, o anúncio da barraca viva já
// tinha sido cancelado — venda desfeita em silêncio, sem ninguém saber.
//
// O conserto é a reconciliação virar chamada própria, depois de esta conexão
// ganhar a conta. Este teste é a prova: a segunda entrada não reconcilia nada.
func TestLoginDuplicadoNaoReconciliaEscrow(t *testing.T) {
	db := bancoDeAnuncio()
	addr, stop, _ := startServerNovato(t, db)
	defer stop()

	primeira := enterWorldAs(t, addr, "tester")
	defer primeira.Close()
	drena(t, primeira)
	abreBarraca(t, primeira, "Loja", 0, 5000, protocol.LojaMoedaRMT)
	esperaReconciliacao(t, db, 1) // a primeira entrada reconcilia, e deve

	// A segunda tentativa, com a senha certa e sem pedir para assumir a conta.
	segunda := dial(t, addr)
	defer segunda.Close()
	send(t, segunda, protocol.MsgAccountLogin, loginBody("tester", "secret", protocol.AppVersion))

	if h := readHeader(t, segunda); h.Type != protocol.MsgAlreadyPlaying {
		t.Fatalf("resposta = %#x, quero AlreadyPlaying(%#x)", h.Type, protocol.MsgAlreadyPlaying)
	}
	// Dá tempo de uma reconciliação indevida chegar ao banco, se houver.
	for i := 0; i < 20; i++ {
		esperaUmPouco()
	}
	if n := len(db.reconciliou()); n != 1 {
		t.Errorf("a reconciliacao rodou %d vez(es); a segunda cancelaria a venda da "+
			"barraca que a primeira sessao tem de pe", n)
	}
}

// esperaReconciliacao espera a reconciliação chegar ao banco n vezes.
func esperaReconciliacao(t *testing.T, db *fakeDB, n int) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if len(db.reconciliou()) >= n {
			return
		}
		esperaUmPouco()
	}
	t.Fatalf("a reconciliacao nao rodou %d vez(es)", n)
}
