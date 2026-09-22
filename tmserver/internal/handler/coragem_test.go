package handler

import (
	"io"
	"log/slog"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// mundoDaCoragem monta o mínimo para usar um consumível: o Dispatcher, o mundo e
// um Mortal com o item no primeiro espaço da bolsa.
func mundoDaCoragem(t *testing.T, item int16, nivel int32) (*Dispatcher, *world.World, *world.Session, *world.Entity) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log, ItemVolatiles: map[int]int{int(item): volCoragem}})
	w := world.New(world.Config{GridDim: 16}, log, nil, d.Handle)
	e := &world.Entity{ID: 0, Mode: world.MobUser, Name: "Herói", Level: nivel,
		ClassMaster: classMasterMortal, MaxHP: 1000, HP: 1000}
	e.Carry[0] = world.Item{Index: item}
	return d, w, &world.Session{Conn: 0, Mode: world.UserPlay}, e
}

func afetoDeCoragem(e *world.Entity) (world.Affect, bool) {
	for _, a := range e.Affect {
		if a.Type == world.AffectForceMobDamage {
			return a, true
		}
	}
	return world.Affect{}, false
}

// TestRemedioDaCoragemDaOBonusEmMonstro é o caso que não existia: até
// 22/09/2026 o EF_VOLATILE 230 não tinha caso no switch de uso, então os três
// itens caíam no default e eram recusados — inclusive o que cai no Cemitério e o
// que o Aki vende por 1.500.000.
func TestRemedioDaCoragemDaOBonusEmMonstro(t *testing.T) {
	d, w, s, e := mundoDaCoragem(t, itemRemedioDaCoragem, 255)
	d.useRemedioDaCoragem(w, s, e, 0)

	af, ok := afetoDeCoragem(e)
	if !ok {
		t.Fatal("o Remédio não deixou afeto nenhum")
	}
	if int(af.Level) != coragemAtaqueEmMob {
		t.Errorf("bônus = %d, want %d", af.Level, coragemAtaqueEmMob)
	}
	if af.Time != uint32(coragemDuracaoRemedio) {
		t.Errorf("duração = %d, want %d", af.Time, coragemDuracaoRemedio)
	}
	if e.Carry[0].Index != 0 {
		t.Errorf("o item ficou na bolsa (%d): o uso tem de consumir", e.Carry[0].Index)
	}
}

// O Elixir custa o dobro do Remédio no catálogo e por isso dura o dobro. O de 30
// dias é o mesmo Elixir: o prazo do nome é do ITEM, não do bônus.
func TestElixirDaCoragemDuraODobro(t *testing.T) {
	for _, item := range []int16{itemElixirDaCoragem, itemElixirCoragem30} {
		d, w, s, e := mundoDaCoragem(t, item, 255)
		d.useRemedioDaCoragem(w, s, e, 0)
		af, ok := afetoDeCoragem(e)
		if !ok {
			t.Fatalf("item %d: nenhum afeto", item)
		}
		if af.Time != uint32(coragemDuracaoElixir) {
			t.Errorf("item %d: duração %d, want %d", item, af.Time, coragemDuracaoElixir)
		}
	}
}

// A descrição do cliente promete "Disponível a partir do level 255", e é a única
// promessa dela que o servidor pode quebrar calado.
func TestCoragemRecusaAbaixoDo255(t *testing.T) {
	d, w, s, e := mundoDaCoragem(t, itemRemedioDaCoragem, 254)
	d.useRemedioDaCoragem(w, s, e, 0)

	if _, ok := afetoDeCoragem(e); ok {
		t.Error("o nível 254 recebeu o bônus")
	}
	if e.Carry[0].Index != itemRemedioDaCoragem {
		t.Error("o item foi consumido numa recusa")
	}
}

// A guarda que não se vê: EmptyAffect devolve o slot que JÁ tem o tipo, então
// sem ela um Remédio (+500) tomado por cima de um Frango (+2000) trocaria um
// pelo outro e o jogador sairia batendo MENOS depois de gastar o item.
func TestCoragemNaoRebaixaUmBonusMaior(t *testing.T) {
	d, w, s, e := mundoDaCoragem(t, itemRemedioDaCoragem, 300)
	e.Affect[0] = world.Affect{Type: world.AffectForceMobDamage, Level: 2000, Time: affect1H}
	d.useRemedioDaCoragem(w, s, e, 0)

	af, ok := afetoDeCoragem(e)
	if !ok {
		t.Fatal("o afeto do Frango sumiu")
	}
	if af.Level != 2000 {
		t.Errorf("bônus = %d, want 2000: o Remédio rebaixou o Frango", af.Level)
	}
	if e.Carry[0].Index != itemRemedioDaCoragem {
		t.Error("o item foi gasto sem dar nada")
	}
}

// Um bônus MENOR já ativo é substituído: o item vale de renovação.
func TestCoragemRenovaUmBonusMenorOuIgual(t *testing.T) {
	d, w, s, e := mundoDaCoragem(t, itemRemedioDaCoragem, 300)
	e.Affect[0] = world.Affect{Type: world.AffectForceMobDamage, Level: 100, Time: 5}
	d.useRemedioDaCoragem(w, s, e, 0)

	af, _ := afetoDeCoragem(e)
	if int(af.Level) != coragemAtaqueEmMob || af.Time != uint32(coragemDuracaoRemedio) {
		t.Errorf("afeto = %d por %d, want %d por %d", af.Level, af.Time, coragemAtaqueEmMob, coragemDuracaoRemedio)
	}
}
