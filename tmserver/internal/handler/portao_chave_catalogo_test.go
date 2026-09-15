package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Os testes de portão de gate_test.go montam a chave com EF_KEYID DENTRO do item
// (keyItem). No jogo não é assim: a Primeira_Porta (458) e a Chave_da_Primeira_Porta
// (451) só carregam EF_KEYID no catálogo (ItemList.csv:753 e :767), e é pelo
// catálogo que o servidor tem que enxergar. Por isso estes testes carregam o
// ItemList.csv de verdade e ligam o Dispatcher do jeito que o main.go liga.
//
// Medido na cópia em 14/09/2026 antes destes testes existirem: a Primeira_Porta
// abriu 9 vezes com 12 chaves na bolsa e não gastou nenhuma, e abriu 2 de 2 com a
// bolsa vazia.

// configComCatalogoReal devolve o Config com os mapas do catálogo que o main.go
// passa para a porta e a chave. Pula quando o Release/ não está montado, como os
// outros testes que leem o conteúdo.
func configComCatalogoReal(t *testing.T) Config {
	t.Helper()
	items, err := content.LoadItemList(filepath.Join("..", "..", "..", "Release", "Common", "ItemList.csv"))
	if err != nil {
		t.Skipf("ItemList.csv indisponível: %v", err)
	}
	return Config{
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		ItemEffects: items.BaseEffects(),
		ItemKeyIDs:  items.KeyIDs(),
	}
}

// startServerPortaoReal sobe um mundo com um portão semeado em (x,y) e o catálogo
// real ligado, e devolve o ItemID de fio do portão.
func startServerPortaoReal(t *testing.T, persist world.Persistence, gate world.Item, x, y, state int16) (string, func(), int32) {
	t.Helper()
	cfg := configComCatalogoReal(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	d := New(cfg)
	w := world.New(world.Config{GridDim: 16}, cfg.Log, persist, d.Handle)
	id := w.SeedWorldItem(gate, x, y, state) // antes do Serve: sem corrida com o laço
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
	}, int32(world.GroundItemIDOffset + id)
}

// TestPrimeiraPortaSemChaveRecusa: a Primeira_Porta trancada, na posição real do
// campo de treino, com a bolsa vazia, recusa e diz que falta a chave.
func TestPrimeiraPortaSemChaveRecusa(t *testing.T) {
	addr, stop, itemID := startServerPortaoReal(t, carryDB(), world.Item{Index: 458}, 2075, 2015, world.StateLocked)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	send(t, c, protocol.MsgUpdateItem, (&protocol.MsgUpdateItemBody{ItemID: itemID, State: world.StateOpen}).Encode())
	for i := 0; i < 40; i++ {
		ty, payload, ok := readMaybe(t, c)
		if !ok {
			t.Fatal("nenhuma recusa chegou: a Primeira_Porta sem chave não respondeu nada")
		}
		if ty == protocol.MsgUpdateItem {
			t.Fatal("a Primeira_Porta abriu sem chave nenhuma na bolsa")
		}
		if ty == protocol.MsgMessagePanel && strings.Contains(strings.ToLower(decodePanel(payload)), "chave") {
			return
		}
	}
	t.Fatal("a recusa sem chave não chegou em 40 quadros")
}

// TestPrimeiraPortaComAChaveDoCatalogoAbreEGasta: com a Chave_da_Primeira_Porta
// como ela existe no jogo — sem efeito nenhum no item, só o índice —, a porta abre
// e a chave é gasta.
func TestPrimeiraPortaComAChaveDoCatalogoAbreEGasta(t *testing.T) {
	addr, stop, itemID := startServerPortaoReal(t, carryDB(world.Item{Index: 451}), world.Item{Index: 458}, 5, 5, world.StateLocked)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	send(t, c, protocol.MsgUpdateItem, (&protocol.MsgUpdateItemBody{ItemID: itemID, State: world.StateOpen}).Encode())
	gastou := false
	for i := 0; i < 40; i++ {
		ty, payload, ok := readMaybe(t, c)
		if !ok {
			break
		}
		switch ty {
		case protocol.MsgSendItem:
			if _, slot, index, _ := sendItemSlotAmount(payload); slot == 0 && index == 0 {
				gastou = true
			}
		case protocol.MsgUpdateItem:
			if !gastou {
				t.Fatal("a Primeira_Porta abriu sem gastar a chave")
			}
			return
		}
	}
	if !gastou {
		t.Fatal("a chave não foi gasta")
	}
	t.Fatal("a chave foi gasta e a porta não abriu")
}
