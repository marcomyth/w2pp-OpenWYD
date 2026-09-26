package handler

import (
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// SÓ ITEM TEMPORÁRIO GANHA PRAZO. Em produção, em 26/09/2026, o pulso de um
// minuto leu os bytes de uma montaria como EF_WDAY/HOUR/MIN (106-108) e
// "iniciou o prazo" dela, apagando HP, nível, vitalidade e ração; a cura
// seguinte, com vitalidade zero, a destruiu. O corpo do slot 0 de outro
// personagem ganhou prazo do mesmo jeito.
func TestSoItemTemporarioGanhaPrazo(t *testing.T) {
	d := New(Config{})
	now := time.Unix(1_800_000_000, 0)
	casos := []struct {
		nome  string
		item  world.Item
		ganha bool
	}{
		{"montaria com nível 107", world.Item{Index: 2362, Effects: [3]world.Effect{{Effect: 32, Value: 78}, {Effect: 107, Value: 5}, {Effect: 100, Value: 1}}}, false},
		{"montaria com o byte baixo do HP em 106", world.Item{Index: 2362, Effects: [3]world.Effect{{Effect: 106, Value: 1}, {Effect: 120, Value: 5}, {Effect: 100, Value: 1}}}, false},
		{"montaria com a ração em 108", world.Item{Index: 2363, Effects: [3]world.Effect{{Effect: 32, Value: 78}, {Effect: 50, Value: 5}, {Effect: 108, Value: 0}}}, false},
		{"corpo da Foema com um byte em 106", world.Item{Index: 16, Effects: [3]world.Effect{{Effect: 106, Value: 3}}}, false},
		{"montaria da loja com 7 dias", world.Item{Index: 3990, Effects: [3]world.Effect{{Effect: efWDay, Value: 7}}}, true},
		{"montaria da loja sem os dias", world.Item{Index: 3980}, true},
		{"fada sem efeitos", world.Item{Index: 3901}, true},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			it := c.item
			antes := it.Effects
			ganhou := d.startTimedItem(&it, now)
			if ganhou != c.ganha {
				t.Fatalf("startTimedItem = %v, queria %v", ganhou, c.ganha)
			}
			if !c.ganha && (it.ExpiresAt != 0 || it.Effects != antes) {
				t.Errorf("um item permanente foi mexido: ExpiresAt=%d efeitos %v → %v", it.ExpiresAt, antes, it.Effects)
			}
		})
	}
}

// O reparo do que o engano já fez: a montaria e o corpo com ExpiresAt perdem o
// prazo em vez de sumir no vencimento. O prazo legítimo fica — o da montaria da
// loja e o que o /gm item pôs num item qualquer.
func TestPrazoIndevidoSoNaMontariaENoCorpo(t *testing.T) {
	d := New(Config{})
	const prazo = 1_800_000_000
	casos := []struct {
		nome     string
		item     world.Item
		noCorpo  bool
		indevido bool
	}{
		{"montaria 2362 com prazo", world.Item{Index: 2362, ExpiresAt: prazo}, false, true},
		{"corpo no slot 0 com prazo", world.Item{Index: 16, ExpiresAt: prazo}, true, true},
		{"montaria da loja com prazo", world.Item{Index: 3990, ExpiresAt: prazo}, false, false},
		{"item qualquer com prazo do GM", world.Item{Index: 1100, ExpiresAt: prazo}, false, false},
		{"montaria sem prazo", world.Item{Index: 2362}, false, false},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := d.prazoIndevido(c.item, c.noCorpo); got != c.indevido {
				t.Errorf("prazoIndevido = %v, queria %v", got, c.indevido)
			}
		})
	}

	equip := []world.Item{{Index: 16, ExpiresAt: prazo}, {Index: 1100, ExpiresAt: prazo}}
	if n := d.desfazerPrazosIndevidos(equip, true); n != 1 || equip[0].ExpiresAt != 0 || equip[1].ExpiresAt != prazo {
		t.Errorf("desfez %d; corpo %d, item do GM %d — queria só o corpo", n, equip[0].ExpiresAt, equip[1].ExpiresAt)
	}
}
