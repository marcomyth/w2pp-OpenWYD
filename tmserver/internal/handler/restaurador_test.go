package handler

import (
	"testing"
)

// TestRestauradorGrupoEspelhaOCatalisador confere que a adulta cai no mesmo grupo
// da cria da sua linhagem: o legado roda a mesma cadeia de ifs sobre sIndex-2363
// (_MSG_UseItem.cpp:5124-5146), 30 acima da do catalisador.
func TestRestauradorGrupoEspelhaOCatalisador(t *testing.T) {
	esperado := map[int16]int{
		2363: 0, 2365: 0,
		2366: 1, 2374: 1,
		2370: 2, 2375: 2, 2387: 2,
		2376: 3, 2378: 3,
		2381: 4, 2388: 4,
		2384: 5, 2386: 5,
		2379: 6, 2380: 6,
	}
	for adulta, grupo := range esperado {
		got, ok := restauradorGrupo(adulta)
		if !ok || got != grupo {
			t.Errorf("adulta %d: grupo %d (ok=%v), want %d", adulta, got, ok, grupo)
		}
	}
	// Cria, as três adultas sem grupo no legado, e slot vazio.
	for _, idx := range []int16{2340, 2360, 2361, 2362, 2389, 0} {
		if _, ok := restauradorGrupo(idx); ok {
			t.Errorf("montaria %d ganhou um grupo e não devia ter", idx)
		}
	}
}

func TestRestauradorCertoSobeUmOuDoisDeVitalidade(t *testing.T) {
	// Andaluz adulto 2370: grupo 2, o Restaurador de Mencar (3353).
	d, w, s, e := catalisadorFixture(t, 2370, 3353, 10)
	e.Equip[mountEquipSlot].Effects[1].Value = 20
	d.useRestaurador(w, s, e, corpoNoSlotDaMontaria(), 0)

	mount := e.Equip[mountEquipSlot]
	if v := mount.Effects[1].Value; v != 21 && v != 22 {
		t.Errorf("vitalidade = %d, want 21 ou 22", v)
	}
	if mount.Index != 2370 || mount.Effects[1].Effect != 10 || mountHP(mount) != 20000 {
		t.Errorf("o restaurador mexeu em outra coisa da montaria: %+v", mount)
	}
	if !e.Carry[0].Empty() {
		t.Errorf("o restaurador não foi consumido: %+v", e.Carry[0])
	}
}

func TestRestauradorGastaUmDaPilha(t *testing.T) {
	d, w, s, e := catalisadorFixture(t, 2370, 3353, 10)
	e.Equip[mountEquipSlot].Effects[1].Value = 20
	setItemAmount(&e.Carry[0], 3)
	d.useRestaurador(w, s, e, corpoNoSlotDaMontaria(), 0)

	if n := itemAmount(e.Carry[0]); e.Carry[0].Index != 3353 || n != 2 {
		t.Errorf("pilha = %d x%d, want 3353 x2", e.Carry[0].Index, n)
	}
}

func TestRestauradorRecusaSemGastar(t *testing.T) {
	casos := []struct {
		nome        string
		montaria    int16
		restaurador int16
		vitalidade  uint8
		hp          uint16
		destPos     int32
	}{
		{"linhagem errada", 2370, 3351, 20, 20000, mountEquipSlot},
		{"cria no slot", 2340, 3353, 20, 20000, mountEquipSlot},
		{"vitalidade no teto", 2370, 3353, 50, 20000, mountEquipSlot},
		{"vitalidade no piso", 2370, 3353, 5, 20000, mountEquipSlot},
		{"montaria morta", 2370, 3353, 20, 0, mountEquipSlot},
		{"fora do slot da montaria", 2370, 3353, 20, 20000, 3},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			d, w, s, e := catalisadorFixture(t, c.montaria, c.restaurador, 10)
			e.Equip[mountEquipSlot].Effects[1].Value = c.vitalidade
			putShort(&e.Equip[mountEquipSlot].Effects[0], c.hp)
			antes := e.Equip[mountEquipSlot]
			body := corpoNoSlotDaMontaria()
			body.DestPos = c.destPos
			d.useRestaurador(w, s, e, body, 0)

			if e.Equip[mountEquipSlot] != antes {
				t.Errorf("montaria mudou: %+v, want %+v", e.Equip[mountEquipSlot], antes)
			}
			if e.Carry[0].Index != c.restaurador {
				t.Errorf("o restaurador foi consumido numa recusa: %+v", e.Carry[0])
			}
		})
	}
}
