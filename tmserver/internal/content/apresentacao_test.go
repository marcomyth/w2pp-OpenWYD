package content

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// A APRESENTAÇÃO DOS NPCs (pedido de 21/09/2026).
//
// O Equip de um NPC é só aparência no port — o spawn lê dele o visual e o
// alcance, e nada entra no score ([[equipamento-de-mob-nao-conta]]). Estes
// testes são, portanto, sobre o que o jogador VÊ, e sobre a única coisa que o
// Equip de um NPC carrega além do visual: o grau da quest, no Equip[0].

const (
	structMobEquip = 140 // Equip[16], 8 bytes por peça (data-formats.md §0.1)
	structMobCarry = 268 // Carry[64], logo depois do Equip

	efSancVisual = 43  // EF_SANC
	sanc11       = 234 // refine: 230 + (11-10)*4
	sanc15       = 250 // refine: 230 + (15-10)*4

	slotMontaria = 14
	// A faixa que o cliente desenha como montaria, com o modelo escolhido pelo
	// nível (protocol.visualMountLo/Hi). Fora dela o nível não vale nada.
	montariaLo      = 2360
	montariaHi      = 2390
	nivelDaMontaria = 90

	efGrade0 = 100 // o grau da quest, no Equip[0] de um NPC (world/api.go)
)

// peca lê uma vaga de Equip: índice e os três pares de efeito.
func peca(b []byte, slot int) (idx int, ef [3][2]byte) {
	o := structMobEquip + slot*8
	idx = int(binary.LittleEndian.Uint16(b[o : o+2]))
	for k := 0; k < 3; k++ {
		ef[k] = [2]byte{b[o+2+k*2], b[o+3+k*2]}
	}
	return idx, ef
}

func templateNPC(t *testing.T, nome string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(release(t, "TMsrv", "run", "npc"), nome))
	if err != nil {
		t.Skipf("Release content unavailable: %v", err)
	}
	if len(b) != BaseMobSize {
		t.Fatalf("%s tem %d bytes, esperava %d", nome, len(b), BaseMobSize)
	}
	return b
}

// efeito procura um par de efeito pelo byte de efeito.
func efeito(ef [3][2]byte, qual byte) (byte, bool) {
	for _, p := range ef {
		if p[0] == qual {
			return p[1], true
		}
	}
	return 0, false
}

// montada confere a vaga 14: o índice tem de cair na faixa que o cliente
// desenha, o primeiro par precisa ser um short POSITIVO (sem ele o cliente não
// desenha montaria nenhuma) e o byte de EFEITO do segundo par é o nível, que
// escolhe o modelo de dez em dez.
func montada(t *testing.T, nome string, b []byte, nivel int) {
	t.Helper()
	idx, ef := peca(b, slotMontaria)
	if idx < montariaLo || idx >= montariaHi {
		t.Errorf("%s: montaria %d fora da faixa %d-%d, o nível não vale", nome, idx, montariaLo, montariaHi-1)
	}
	if int(ef[0][0])+int(ef[0][1])<<8 <= 0 {
		t.Errorf("%s: o primeiro par da montaria é zero; o cliente não desenha nada", nome)
	}
	if int(ef[1][0]) != nivel {
		t.Errorf("%s: montaria de nível %d, esperava %d", nome, ef[1][0], nivel)
	}
}

// Os NPCs de Armia vestem o Set Mortal E a +11 e montam um Cavalo Equipado de
// nível 90. O corpo (Equip[0]) e a arma (6/7) NÃO se mexem: o corpo é a
// identidade de cada um — o ferreiro continua ferreiro — e a arma do guarda é
// dele.
func TestArmiaVesteSetMortalMontado(t *testing.T) {
	setMortalE := [5]int{1225, 1226, 1227, 1228, 1229}
	// O Mestre Grifo monta um Grifo, que é o nome dele; ficou com o dele.
	semCavalo := map[string]bool{"Mestre_Grifo": true}

	for _, nome := range []string{
		"Galford", "Aki", "Foema_Ancian", "Guarda_Carga", "Guard", "Guard_", "Ferreiro",
		"Rapein", "Cap.Cavaleiros", "ForeLearner", "Rainy", "Mestre_Haby", "Balmus",
		"Gate_Keeper", "Martin", "Arnod", "Mestre_Archi", "Kibita", "Mestre_Grifo",
		"God_of_War", "Curandeiro",
	} {
		b := templateNPC(t, nome)
		for i, quer := range setMortalE {
			idx, ef := peca(b, i+1)
			if idx != quer {
				t.Errorf("%s: vaga %d tem %d, esperava %d", nome, i+1, idx, quer)
				continue
			}
			if v, ok := efeito(ef, efSancVisual); !ok || v != sanc11 {
				t.Errorf("%s: vaga %d com EF_SANC %d, esperava %d (+11)", nome, i+1, v, sanc11)
			}
		}
		if corpo, _ := peca(b, 0); corpo == 0 {
			t.Errorf("%s ficou sem corpo no Equip[0]", nome)
		}
		if !semCavalo[nome] {
			montada(t, nome, b, nivelDaMontaria)
		}
	}
}

// Os quatro Perzen vestem Coroa Celestial, set de topo a +15 e o Demolidor
// Celestial, montados num Tigre de Fogo de nível 90.
//
// E a parte que importa mais que o visual: vestir não pode ter mexido no
// Equip[0] nem no Carry. O Equip[0] carrega o GRAU da quest (efeito 100), que é
// o que diz qual Esfera o NPC aceita, e o Carry[0]/Carry[1] é o par
// troca→recompensa que o perzenExchange lê (handler/misc.go). Zerar qualquer um
// dos dois desliga o NPC sem tirá-lo do mundo — uma falha calada.
func TestPerzenVesteCelestialSemPerderAQuest(t *testing.T) {
	casos := []struct {
		nome       string
		grau       byte
		troca, rec int
	}{
		{"Perzen_Normal", 7, 4128, 3987},
		{"Perzen_Mistico", 8, 4129, 3988},
		{"Perzen_Arcano", 9, 4130, 3987},
		{"Perzen", 10, 0, 0}, // grau fora de 7-9: não é NPC de troca
	}
	setTopo := [4]int{1365, 1366, 1367, 1368}

	for _, c := range casos {
		b := templateNPC(t, c.nome)

		if _, ef := peca(b, 0); true {
			v, ok := efeito(ef, efGrade0)
			if !ok || v != c.grau {
				t.Errorf("%s: grau %d no Equip[0], esperava %d — a quest dele depende disso", c.nome, v, c.grau)
			}
		}
		if idx, _ := peca(b, 1); idx != 3303 {
			t.Errorf("%s: vaga 1 tem %d, esperava a Coroa Celestial 3303", c.nome, idx)
		}
		for i, quer := range setTopo {
			idx, ef := peca(b, i+2)
			if idx != quer {
				t.Errorf("%s: vaga %d tem %d, esperava %d", c.nome, i+2, idx, quer)
				continue
			}
			if v, ok := efeito(ef, efSancVisual); !ok || v != sanc15 {
				t.Errorf("%s: vaga %d com EF_SANC %d, esperava %d (+15)", c.nome, i+2, v, sanc15)
			}
		}
		if idx, _ := peca(b, 6); idx != 3781 {
			t.Errorf("%s: vaga 6 tem %d, esperava o Demolidor Celestial(Anct) 3781", c.nome, idx)
		}
		montada(t, c.nome, b, nivelDaMontaria)

		// O par da troca vive no Carry, e vestir não chega perto dele.
		troca := int(binary.LittleEndian.Uint16(b[structMobCarry : structMobCarry+2]))
		rec := int(binary.LittleEndian.Uint16(b[structMobCarry+8 : structMobCarry+10]))
		if troca != c.troca || rec != c.rec {
			t.Errorf("%s: par da troca virou %d→%d, esperava %d→%d", c.nome, troca, rec, c.troca, c.rec)
		}
	}
}
