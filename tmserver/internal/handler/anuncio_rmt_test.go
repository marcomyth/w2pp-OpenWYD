package handler

import (
	"errors"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// bancoDeAnuncio monta o vendedor com um item no slot 0 do baú, com efeitos, para
// a fotografia do anúncio ter o que carregar.
func bancoDeAnuncio() *fakeDB {
	db := newDB()
	db.loads = map[int64]world.CharacterState{
		7: {Slot: 0, Name: "Vendedor", Level: 1, HP: 1000, MaxHP: 1000},
	}
	var cargo world.CargoState
	cargo.Items[0] = world.Item{Index: 1030}
	cargo.Items[0].Effects[0] = world.Effect{Effect: 3, Value: 9}
	cargo.Items[0].Effects[1] = world.Effect{Effect: 4, Value: 11}
	db.accounts["tester"].cargo = cargo
	return db
}

// SEM CHAVE PIX NÃO ANUNCIA, e a recusa é na hora de montar.
//
// No momento de montar, o vendedor está olhando para a tela e resolve sozinho —
// cadastra a chave no site. Se a recusa fosse na hora de pagar, quem estaria com
// o QR aberto é o COMPRADOR, e o único que poderia resolver já foi embora.
func TestSemChavePixNaoAnuncia(t *testing.T) {
	db := bancoDeAnuncio()
	db.semChavePix = true
	addr, stop, w := startServerNovato(t, db)
	defer stop()
	c := enterWorldAs(t, addr, "tester")
	defer c.Close()
	drena(t, c)

	mandaAbrirBarraca(t, c, "Loja", 0, 5000, protocol.LojaMoedaRMT)

	if !recebeu(t, c, msgSemChavePix) {
		t.Error("recusou em silencio, ou nao recusou")
	}
	if n := len(db.abertos()); n != 0 {
		t.Errorf("criou %d anuncio(s) para quem nao tem onde receber", n)
	}
	// E a barraca NÃO subiu: uma vitrine com oferta em dinheiro real sem anúncio
	// atrás é o que este caminho inteiro existe para impedir.
	drena(t, c)
	if v := pedeVitrine(t, c, 0, protocol.LojaFiltroTodos); v.Qtd != 0 {
		t.Errorf("a vitrine tem %d oferta(s) sem anuncio atras", v.Qtd)
	}
	if bau := bauDoVendedor(t, w); bau.Items[0].AnuncioRMT != 0 {
		t.Error("marcou o item de um anuncio que nao existe")
	}
}

// Com chave, o anúncio nasce com a FOTOGRAFIA certa e a marca vai para o slot.
//
// A fotografia é o que responde "o que exatamente foi vendido" numa disputa,
// depois de o item já ter saído do baú. Um efeito que não atravessa aqui é um
// efeito que some da prova.
func TestAnuncioNasceComAFotografiaEAMarca(t *testing.T) {
	db := bancoDeAnuncio()
	addr, stop, w := startServerNovato(t, db)
	defer stop()
	c := enterWorldAs(t, addr, "tester")
	defer c.Close()
	drena(t, c)

	abreBarraca(t, c, "Loja", 0, 5000, protocol.LojaMoedaRMT)

	abertos := db.abertos()
	if len(abertos) != 1 {
		t.Fatalf("anuncios criados = %d, quero 1", len(abertos))
	}
	a := abertos[0]
	if a.CargoSlot != 0 {
		t.Errorf("slot do anuncio = %d, quero 0", a.CargoSlot)
	}
	if a.Item.Index != 1030 {
		t.Errorf("item do anuncio = %d, quero 1030", a.Item.Index)
	}
	if a.Item.Effects[0] != (world.Effect{Effect: 3, Value: 9}) ||
		a.Item.Effects[1] != (world.Effect{Effect: 4, Value: 11}) {
		t.Errorf("os efeitos nao entraram na fotografia: %+v", a.Item.Effects)
	}
	// O preço em dinheiro real é em CENTAVOS, e é o mesmo campo do preço em ouro.
	if a.PrecoCentavos != 5000 {
		t.Errorf("preco = %d centavos, quero 5000", a.PrecoCentavos)
	}
	bau := bauDoVendedor(t, w)
	if bau.Items[0].AnuncioRMT == 0 {
		t.Error("o item ficou a venda sem marca de escrow; qualquer caminho poderia move-lo")
	}
}

// Barraca só de ouro não fala com o banco. A ida ao banco é para dinheiro real, e
// cobrar uma viagem de rede de quem vende por ouro seria pagar por nada em todo
// mundo para servir a poucos.
func TestBarracaDeOuroNaoAbreAnuncio(t *testing.T) {
	db := bancoDeAnuncio()
	addr, stop, w := startServerNovato(t, db)
	defer stop()
	c := enterWorldAs(t, addr, "tester")
	defer c.Close()
	drena(t, c)

	abreBarraca(t, c, "Loja", 0, 1000, protocol.LojaMoedaOuro)

	if n := len(db.abertos()); n != 0 {
		t.Errorf("uma barraca de ouro criou %d anuncio(s)", n)
	}
	if bau := bauDoVendedor(t, w); bau.Items[0].AnuncioRMT != 0 {
		t.Error("marcou o item de uma venda em ouro")
	}
}

// Banco fora do ar: a barraca não sobe, e o vendedor sabe.
//
// O pior aqui seria a barraca subir assim mesmo — ficaria uma oferta em dinheiro
// real sem anúncio, e quem comprasse pagaria por nada.
func TestBancoForaDoArNaoSobeABarraca(t *testing.T) {
	db := bancoDeAnuncio()
	db.erroAnuncio = errors.New("banco fora do ar")
	addr, stop, w := startServerNovato(t, db)
	defer stop()
	c := enterWorldAs(t, addr, "tester")
	defer c.Close()
	drena(t, c)

	mandaAbrirBarraca(t, c, "Loja", 0, 5000, protocol.LojaMoedaRMT)

	if !recebeu(t, c, msgAnuncioNaoSaiu) {
		t.Error("a falha do banco nao chegou ao vendedor")
	}
	drena(t, c)
	if v := pedeVitrine(t, c, 0, protocol.LojaFiltroTodos); v.Qtd != 0 {
		t.Errorf("a barraca subiu com %d oferta(s) sem anuncio atras", v.Qtd)
	}
	if bau := bauDoVendedor(t, w); bau.Items[0].AnuncioRMT != 0 {
		t.Error("marcou o item sem anuncio nenhum ter nascido")
	}
}

// Item preso no escrow não sai pela compra do painel.
//
// A armadilha do itemSlot não cobre este caminho: o lojaCompra mexe no baú
// DIRETAMENTE. Sem esta recusa, uma compra em ouro levaria o item de baixo de um
// anúncio ativo, e sobraria uma oferta em dinheiro real sem nada atrás.
func TestCompraNaoLevaItemPresoNoEscrow(t *testing.T) {
	db := bancoDeAnuncio()
	db.loads[11] = world.CharacterState{Slot: 0, Name: "Comprador", Level: 1, HP: 1000, MaxHP: 1000, Coin: 1_000_000}

	addr, stop, w := startServerNovato(t, db)
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()
	comprador := enterWorldAs(t, addr, "tradeb")
	defer comprador.Close()

	barraca := abreBarraca(t, vendedor, "Loja", 0, 1000, protocol.LojaMoedaOuro)
	// A marca chega com a barraca JÁ de pé: a montagem recusa item marcado
	// (msgItemJaAnunciado), então esta é a ordem que resta — e é a que a retirada
	// imediata do vendedor online vai produzir de verdade.
	noLacoDoMundo(t, w, func(w *world.World) {
		if c := w.Cargo(7); c != nil {
			c.Items[0].AnuncioRMT = 777
		}
	})
	drena(t, comprador)

	compra := protocol.LojaCompraBody{Vendedor: barraca, Slot: 0, Moeda: protocol.LojaMoedaOuro}
	send(t, comprador, protocol.MsgLojaCompra, compra.Encode())

	for {
		ty, _, ok := readMaybe(t, comprador)
		if !ok {
			break
		}
		if ty == protocol.MsgSendItem {
			t.Fatal("o comprador levou um item que estava preso num anuncio em dinheiro real")
		}
	}
}
