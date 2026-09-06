package world

import (
	"context"
	"errors"
	"testing"
)

// persistSerial hands out consecutive blocks, or fails on demand.
type persistSerial struct {
	NopPersistence
	proximo int64
	pedidos int
	err     error
}

func (p *persistSerial) ReserveSerials(_ context.Context, quantos int64) (int64, error) {
	p.pedidos++
	if p.err != nil {
		return 0, p.err
	}
	primeiro := p.proximo + 1
	p.proximo += quantos
	return primeiro, nil
}

// tudoMarcavel accepts every non-empty item, so the tests exercise the stamping
// and not the rule (the rule has its own tests, in the handler package).
func tudoMarcavel(it Item) bool { return !it.Empty() }

func mundoSerial(t *testing.T, p Persistence) *World {
	t.Helper()
	return New(Config{GridDim: 16, Marcavel: tudoMarcavel}, slogDiscard(), p, nil)
}

func TestNewSerialEntregaConsecutivos(t *testing.T) {
	p := &persistSerial{}
	w := mundoSerial(t, p)
	if err := w.PrimeSerials(context.Background()); err != nil {
		t.Fatalf("PrimeSerials: %v", err)
	}

	visto := map[int64]bool{}
	for i := 0; i < 50; i++ {
		s := w.NewSerial()
		if s == 0 {
			t.Fatalf("serial %d saiu zero com bloco cheio", i)
		}
		if visto[s] {
			t.Fatalf("serial %d entregue duas vezes", s)
		}
		visto[s] = true
	}
}

// Zero is the honest answer when there is no block. Handing out a number that
// might already belong to another item would turn the whole feature — "two
// items with one serial is proof" — into a false accusation.
func TestNewSerialDaZeroSemBloco(t *testing.T) {
	p := &persistSerial{err: errors.New("sem banco")}
	w := mundoSerial(t, p)
	if err := w.PrimeSerials(context.Background()); err == nil {
		t.Fatal("PrimeSerials devia falhar sem banco")
	}
	if s := w.NewSerial(); s != 0 {
		t.Errorf("serial = %d, want 0 sem bloco", s)
	}
}

// A block that arrives after a newer one is already in use must be dropped, or
// its numbers go out a second time.
func TestInstalarBlocoIgnoraRespostaAtrasada(t *testing.T) {
	w := mundoSerial(t, &persistSerial{})
	w.instalarBloco(10_000)
	primeiro := w.NewSerial()

	w.instalarBloco(500) // resposta velha, de um pedido anterior
	if s := w.NewSerial(); s <= primeiro {
		t.Errorf("depois do bloco atrasado veio %d, que não é maior que %d", s, primeiro)
	}
}

func TestMarcarNaoRemarcaOQueJaTemNumero(t *testing.T) {
	w := mundoSerial(t, &persistSerial{})
	if err := w.PrimeSerials(context.Background()); err != nil {
		t.Fatalf("PrimeSerials: %v", err)
	}

	it := Item{Index: 1100, Serial: 777}
	// Re-stamping on every save is the failure that would hide every duplicate:
	// the copy and the original would each pick up a fresh number and never
	// match again.
	if got := w.marcar(it); got.Serial != 777 {
		t.Errorf("serial = %d, want o 777 que já estava lá", got.Serial)
	}
}

func TestMarcarIgnoraOQueARegraRecusa(t *testing.T) {
	w := New(Config{GridDim: 16, Marcavel: func(it Item) bool { return it.Index == 1100 }},
		slogDiscard(), &persistSerial{}, nil)
	if err := w.PrimeSerials(context.Background()); err != nil {
		t.Fatalf("PrimeSerials: %v", err)
	}

	if got := w.marcar(Item{Index: 30}); got.Serial != 0 {
		t.Errorf("item recusado pela regra ganhou serial %d", got.Serial)
	}
	if got := w.marcar(Item{Index: 1100}); got.Serial == 0 {
		t.Error("item aceito pela regra saiu sem serial")
	}
	// Sem regra nenhuma nada é marcado, que é o que um mundo sem catálogo quer.
	semRegra := New(Config{GridDim: 16}, slogDiscard(), &persistSerial{}, nil)
	_ = semRegra.PrimeSerials(context.Background())
	if got := semRegra.marcar(Item{Index: 1100}); got.Serial != 0 {
		t.Errorf("mundo sem regra marcou o item com %d", got.Serial)
	}
}

// The load-bearing one: savedItems has to write the number back into the live
// array. If it only put the serial on the outgoing row, the next save would mint
// a second number for the same item and no duplicate would ever match.
func TestSavedItemsGravaOSerialNoItemVivo(t *testing.T) {
	w := mundoSerial(t, &persistSerial{})
	if err := w.PrimeSerials(context.Background()); err != nil {
		t.Fatalf("PrimeSerials: %v", err)
	}

	var mochila [MaxCarry]Item
	mochila[0] = Item{Index: 1100}
	mochila[3] = Item{Index: 2200}

	linhas := w.savedItems(mochila[:])
	if len(linhas) != 2 {
		t.Fatalf("linhas = %d, want 2", len(linhas))
	}
	for _, l := range linhas {
		if l.Serial == 0 {
			t.Fatalf("linha do slot %d saiu sem serial", l.Slot)
		}
		if mochila[l.Slot].Serial != l.Serial {
			t.Errorf("slot %d: item vivo tem %d, linha salva tem %d",
				l.Slot, mochila[l.Slot].Serial, l.Serial)
		}
	}
	if mochila[0].Serial == mochila[3].Serial {
		t.Error("dois itens diferentes ficaram com o mesmo serial")
	}

	// E o segundo save não pode inventar números novos.
	antes0, antes3 := mochila[0].Serial, mochila[3].Serial
	w.savedItems(mochila[:])
	if mochila[0].Serial != antes0 || mochila[3].Serial != antes3 {
		t.Errorf("o segundo save trocou os números: %d,%d viraram %d,%d",
			antes0, antes3, mochila[0].Serial, mochila[3].Serial)
	}
}

// Slots vazios não gastam número: o mundo tem muito mais slot vazio do que item.
func TestSavedItemsNaoGastaNumeroComSlotVazio(t *testing.T) {
	p := &persistSerial{}
	w := mundoSerial(t, p)
	if err := w.PrimeSerials(context.Background()); err != nil {
		t.Fatalf("PrimeSerials: %v", err)
	}
	var mochila [MaxCarry]Item
	mochila[0] = Item{Index: 1100}

	w.savedItems(mochila[:])
	if w.serialProximo != w.serialFim-serialBloco+1 {
		t.Errorf("gastou %d números para um item só",
			w.serialProximo-(w.serialFim-serialBloco))
	}
}
