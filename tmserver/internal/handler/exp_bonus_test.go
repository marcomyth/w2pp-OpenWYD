package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

func TestFairyExpBonus(t *testing.T) {
	d := New(Config{})
	e := &world.Entity{}
	e.Equip[fairyEquipSlot] = world.Item{Index: 3900}
	if got := d.equipExpBonus(e); got != 16 {
		t.Errorf("fada verde 3d = %d, want 16", got)
	}
	e.Equip[fairyEquipSlot] = world.Item{Index: 3902}
	if got := d.equipExpBonus(e); got != 32 {
		t.Errorf("fada vermelha = %d, want 32", got)
	}
}

// TestEsferaExpBonus: a esfera (client/montarias) entra no /xp pelo mesmo
// caminho das montarias da loja, mountbonus.TempExtra. É o único caminho que
// ela tem: a tabela de bônus do cliente não tem coluna de XP e a faixa
// 2969-2975 não desenha linha de atributo nenhuma, então o que não aparecer
// aqui não aparece em lugar nenhum.
func TestEsferaExpBonus(t *testing.T) {
	d := New(Config{})
	e := &world.Entity{}
	for idx := int16(2969); idx <= 2975; idx++ {
		e.Equip[mountEquipSlot] = world.Item{Index: idx}
		p := d.equipExpBonusParcelas(e)
		if p.Montaria != 12 || p.Total() != 12 {
			t.Errorf("esfera %d = %d (total %d), want 12", idx, p.Montaria, p.Total())
		}
	}
	// A montaria adulta continua sem coluna de XP: as esferas não a criaram.
	e.Equip[mountEquipSlot] = world.Item{Index: 2379} // Tigre de Fogo adulto
	if got := d.equipExpBonusParcelas(e).Montaria; got != 0 {
		t.Errorf("adulta = %d, want 0", got)
	}
}

func TestEquipGrade7ExpBonus(t *testing.T) {
	d := New(Config{ItemGrades: map[int]int{900: 7}})
	e := &world.Entity{}
	e.Equip[0] = world.Item{Index: 900}
	if got := d.equipExpBonus(e); got != 2 {
		t.Errorf("grade 7 = %d, want 2", got)
	}
}

func TestEquipGemSapphireExpBonus(t *testing.T) {
	d := New(Config{})
	e := &world.Entity{}
	e.Equip[1] = world.Item{Index: 100, Effects: [3]world.Effect{{Effect: efSanc, Value: 232}}}
	if got := itemGem(e.Equip[1]); got != 2 {
		t.Fatalf("itemGem(232) = %d, want 2", got)
	}
	if got := d.equipExpBonus(e); got != 2 {
		t.Errorf("safira gem = %d, want 2", got)
	}
}

func TestExpBonusCombined(t *testing.T) {
	d := New(Config{ItemGrades: map[int]int{900: 7}})
	e := &world.Entity{}
	e.Affect[0] = world.Affect{Type: world.AffectExpChest}
	e.Equip[fairyEquipSlot] = world.Item{Index: 3900}
	e.Equip[0] = world.Item{Index: 900}
	applyAffectScore(e)
	e.EquipExpBonus = d.equipExpBonus(e)
	if got := d.expBonus(e); got != 118 {
		t.Errorf("total exp bonus = %d, want 118 (100+16+2)", got)
	}
}
