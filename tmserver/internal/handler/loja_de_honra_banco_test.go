package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/combine"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/npccfg"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O estoque da Loja de Honra vem do banco, pelas vagas do God of War (26/09/2026).

func pontosDeHonra(n int32) *int32 { return &n }

// estoqueDeHonraDeTeste é o estoque que a 0166 grava no God of War: os sete itens
// combinados em 25/09, nas vagas 0 a 6.
func estoqueDeHonraDeTeste() []npccfg.ShopItem {
	return []npccfg.ShopItem{
		{Slot: 0, Index: 413, Quantity: 1, PricePoints: pontosDeHonra(100)},
		{Slot: 1, Index: 3438, Quantity: 1, PricePoints: pontosDeHonra(360)},
		{Slot: 2, Index: 412, Quantity: 3, PricePoints: pontosDeHonra(480)},
		{Slot: 3, Index: 3901, Quantity: 1, Eff: [3][2]uint8{{efWDay, 1}}, PricePoints: pontosDeHonra(960)},
		{Slot: 4, Index: 4140, Quantity: 1, PricePoints: pontosDeHonra(1440)},
		{Slot: 5, Index: 3173, Quantity: 3, PricePoints: pontosDeHonra(1440)},
		{Slot: 6, Index: 3467, Quantity: 1, PricePoints: pontosDeHonra(2400)},
	}
}

// honraEsperada é a troca que a vaga slot do estoque de teste oferece, na forma
// em que a loja a descreve.
func honraEsperada(t *testing.T, slot int) itemDeHonra {
	t.Helper()
	for _, it := range estoqueDeHonraDeTeste() {
		if it.Slot == slot {
			// Sem catálogo no teste, toda aba é Consumo (categoriaDeHonra).
			return itemDeHonra{Slot: int16(it.Slot), Indice: int16(it.Index), Preco: *it.PricePoints,
				Cat: protocol.HonraCatConsumo}
		}
	}
	t.Fatalf("o estoque de teste não tem a vaga %d", slot)
	return itemDeHonra{}
}

// godOfWarComEstoque é o God of War como a recarga o deixa: marcado e com as
// vagas escritas pelo applyShop.
func godOfWarComEstoque(shop []npccfg.ShopItem) *world.Entity {
	npc := &world.Entity{ID: shopNPCID, Merchant: merchantLojaDeHonra}
	applyShop(npc, shop)
	return npc
}

func TestEstoqueDeHonraLeAsVagasDoNPC(t *testing.T) {
	d := New(Config{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	shop := []npccfg.ShopItem{
		{Slot: 0, Index: 413, Quantity: 1, PricePoints: pontosDeHonra(100)},
		// Cobrada em ouro: a loja de honra não sabe cobrar ouro, então não mostra.
		{Slot: 1, Index: 3438, Quantity: 1},
		// A zero: item de graça é engano de cadastro.
		{Slot: 2, Index: 412, Quantity: 1, PricePoints: pontosDeHonra(0)},
		// Da segunda página do painel: a vaga 9 mora no Carry[27] (protocol.ShopSlot),
		// e ler Carry[9] direto a perderia.
		{Slot: 9, Index: 4140, Quantity: 1, PricePoints: pontosDeHonra(1440)},
	}
	estoque := d.estoqueDeHonra(godOfWarComEstoque(shop))
	if len(estoque) != 2 {
		t.Fatalf("a loja oferece %d itens, quer 2 (vagas 0 e 9): %+v", len(estoque), estoque)
	}
	if estoque[0].Slot != 0 || estoque[0].Indice != 413 || estoque[0].Preco != 100 {
		t.Errorf("primeira troca = %+v, quer a vaga 0 (413 por 100)", estoque[0])
	}
	if estoque[1].Slot != 9 || estoque[1].Indice != 4140 || estoque[1].Preco != 1440 {
		t.Errorf("segunda troca = %+v, quer a vaga 9 (4140 por 1440)", estoque[1])
	}

	// Um NPC que não é a loja de honra não vende nada por aqui, mesmo com vagas
	// cobradas em pontos: essas são da loja em pontos comum (shoppointsshop.go).
	outro := godOfWarComEstoque(shop)
	outro.Merchant = 1
	if got := d.estoqueDeHonra(outro); len(got) != 0 {
		t.Errorf("um lojista comum virou loja de honra: %+v", got)
	}
}

func TestCategoriaDeHonraPelaCasaDeEquipar(t *testing.T) {
	d := New(Config{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		CombineCatalog: combine.Catalog{Pos: map[int]int{
			811:  64,    // arma
			1200: 16,    // luva: armadura
			3901: 16384, // fada
			413:  0,     // material
		}},
	})
	casos := []struct {
		nome   string
		indice int16
		quer   uint8
	}{
		{"arma vai para Armas", 811, protocol.HonraCatArmas},
		{"peça de armadura vai para Set", 1200, protocol.HonraCatSet},
		{"fada fica em Consumo", 3901, protocol.HonraCatConsumo},
		{"material fica em Consumo", 413, protocol.HonraCatConsumo},
		{"item fora do catálogo fica em Consumo", 9999, protocol.HonraCatConsumo},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := d.categoriaDeHonra(c.indice); got != c.quer {
				t.Fatalf("categoriaDeHonra(%d) = %d, quer %d", c.indice, got, c.quer)
			}
		})
	}
}

// snapshotDeHonra é a config de NPC com o God of War e mais um lojista na frente
// dele. O lojista da frente é o que faz o id do God of War MUDAR entre as duas
// recargas do teste — sem ele o id seria reaproveitado e um painel apontando para
// o id velho funcionaria por acaso.
func snapshotDeHonra(comOutroNaFrente bool, shop []npccfg.ShopItem) npccfg.Snapshot {
	god := npccfg.Definition{
		Slug: "God_of_War-1", Template: godOfWarTemplate(), TemplateName: "God_of_War",
		Enabled: true, X: 8, Y: 8, Merchant: 104, Shop: shop,
	}
	var snap npccfg.Snapshot
	if comOutroNaFrente {
		outro := godOfWarTemplate()
		copy(outro[0:16], "Outro\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
		snap.Defs = append(snap.Defs, npccfg.Definition{
			Slug: "Outro-2", Template: outro, TemplateName: "Outro",
			Enabled: true, X: 10, Y: 10, Merchant: 1,
		})
	}
	snap.Defs = append(snap.Defs, god)
	return snap
}

func startServerHonraGerida(t *testing.T, persist world.Persistence) (string, func(), *Dispatcher, *world.World) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log})
	w := world.New(world.Config{GridDim: 16}, log, persist, d.Handle)
	d.applyNPCConfig(w, snapshotDeHonra(false, estoqueDeHonraDeTeste()), false)
	if id := d.managedNPCs["God_of_War-1"]; id != shopNPCID {
		t.Fatalf("o God of War nasceu como %d, esperado %d", id, shopNPCID)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	return ln.Addr().String(), func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("o servidor não parou")
		}
	}, d, w
}

// Uma edição em OUTRA loja recarrega todos os NPCs, e o God of War volta com
// outro id. Quem estava com o painel de honra aberto continua comprando.
func TestRecargaSemMudarAHonraMantemOPainel(t *testing.T) {
	db := contaComPontos(1000, 5, 5)
	addr, stop, d, w := startServerHonraGerida(t, db)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	clicaNoGodOfWar(t, c)
	expect(t, c, protocol.MsgHonraAbre)

	noLaco(t, w, func(w *world.World) {
		d.applyNPCConfig(w, snapshotDeHonra(true, estoqueDeHonraDeTeste()), true)
	})
	var novoID int
	noLaco(t, w, func(*world.World) { novoID = d.managedNPCs["God_of_War-1"] })
	if novoID == shopNPCID {
		t.Fatalf("o God of War manteve o id %d; o teste precisa que ele mude", novoID)
	}

	quer := honraEsperada(t, 0)
	compraDeHonra(t, c, 0)
	chegou := false
	for i := 0; i < 12 && !chegou; i++ {
		ty, p, ok := readMaybe(t, c)
		if !ok {
			break
		}
		if ty == protocol.MsgHonraFechou {
			t.Fatal("o painel fechou numa recarga que não mexeu na Loja de Honra")
		}
		if ty == protocol.MsgSendItem && int16(le16(p[4:6])) == quer.Indice {
			chegou = true
		}
	}
	if !chegou {
		t.Fatal("depois da recarga a compra no painel aberto não entregou o item")
	}
	if got := db.pontosLojinha0(); got != 1000-quer.Preco {
		t.Errorf("carteira = %d, quer %d", got, 1000-quer.Preco)
	}
}

// Quando a equipe muda um preço da Loja de Honra, o painel aberto mostra um preço
// que a loja não cobra mais: ele fecha, com aviso.
func TestRecargaQueMudaAHonraFechaOPainel(t *testing.T) {
	db := contaComPontos(1000, 5, 5)
	addr, stop, d, w := startServerHonraGerida(t, db)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	clicaNoGodOfWar(t, c)
	expect(t, c, protocol.MsgHonraAbre)

	novo := estoqueDeHonraDeTeste()
	novo[0].PricePoints = pontosDeHonra(150)
	noLaco(t, w, func(w *world.World) { d.applyNPCConfig(w, snapshotDeHonra(true, novo), true) })

	fechou, avisou := false, false
	for i := 0; i < 12 && (!fechou || !avisou); i++ {
		ty, p, ok := readMaybe(t, c)
		if !ok {
			break
		}
		switch ty {
		case protocol.MsgHonraFechou:
			fechou = true
		case protocol.MsgMessagePanel:
			if strings.Contains(decodePanel(p), "Loja de Honra foi atualizada") {
				avisou = true
			}
		}
	}
	if !fechou || !avisou {
		t.Fatalf("depois de mudar o preço: painel fechado = %v, aviso = %v; quer os dois", fechou, avisou)
	}

	// E a compra pelo painel velho não passa, nem pelo preço velho.
	compraDeHonra(t, c, 0)
	time.Sleep(200 * time.Millisecond)
	if got := db.pontosLojinha0(); got != 1000 {
		t.Errorf("o painel fechado ainda cobrou: carteira = %d, quer 1000", got)
	}
}
