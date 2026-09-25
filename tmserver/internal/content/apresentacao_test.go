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
//
// Os quatro mestres de skill saíram desta lista em 21/09: cada um passou a
// vestir o set da própria classe (TestMestresDeSkillVestemOSetDaClasse), e o
// Guarda Carga saiu em 22/09 para a aparência do Cav. Lugefer
// (TestGuardaCargaVesteOLugefer).
//
// O God_of_War (a Honor Store) desceu do cavalo em 25/09/2026, a pedido: foi
// para o canteiro cercado em 2130,2088, e montado ele não cabia ali. Continua de
// Set Mortal E; só a vaga 14 ficou vazia.
//
// O Rapein saiu em 25/09/2026 para o set da Foema (TestArmasDeArmia); o Ferreiro
// e a Rainy continuam aqui e só ganharam arma.
func TestArmiaVesteSetMortalMontado(t *testing.T) {
	setMortalE := [5]int{1225, 1226, 1227, 1228, 1229}
	// O Mestre Grifo monta um Grifo, que é o nome dele; ficou com o dele.
	semCavalo := map[string]bool{"Mestre_Grifo": true}
	aPe := map[string]bool{"God_of_War": true}

	for _, nome := range []string{
		"Galford", "Aki", "Guard", "Guard_", "Ferreiro",
		"Rainy", "Mestre_Haby", "Balmus",
		"Gate_Keeper", "Martin", "Arnod", "Kibita", "Mestre_Grifo",
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
		switch {
		case aPe[nome]:
			if idx, _ := peca(b, slotMontaria); idx != 0 {
				t.Errorf("%s: vaga 14 tem %d, esperava vazia (a pé)", nome, idx)
			}
		case !semCavalo[nome]:
			montada(t, nome, b, nivelDaMontaria)
		}
	}
}

// OS QUATRO MESTRES DE SKILL (pedido de 21/09/2026) vestem, cada um, o set e a
// arma da PRÓPRIA classe, em vez do Set Mortal E que o resto de Armia usa — o
// set de TransKnight em todos os quatro fazia o mestre de cada classe anunciar
// a classe errada. A classe de cada um é a que o handler já usa para saber que
// skills vender (handler/misc.go: Class 1..4 = TK, FM, BM, HT).
//
// O que NÃO se mexe: o corpo, que é a identidade do NPC, e a montaria, que veio
// da apresentação de Armia. A arma vai na vaga 6, que é onde 957 dos templates
// do legado põem arma; a 7 fica vazia para o mestre não empunhar duas.
//
// Alcance, contado antes: os quatro têm dois blocos cada no NPCGener, e os oito
// estão dentro de Armia — vestir o template não alcança nenhum outro mapa. E o
// EF_RANGE da arma, que o spawn lê (world/api.go), é inerte aqui: os quatro são
// Merchant, e Merchant != 0 já os faz NonCombatNPC (world/city.go).
func TestMestresDeSkillVestemOSetDaClasse(t *testing.T) {
	casos := []struct {
		nome  string
		set   [5]int
		arma  int
		corpo int
	}{
		{"Cap.Cavaleiros", [5]int{1225, 1226, 1227, 1228, 1229}, 912, 60}, // TK: Set Mortal + Thrasytes
		{"Foema_Ancian", [5]int{1360, 1361, 1362, 1363, 1364}, 903, 61},   // FM: Templário + Eirenus
		{"Mestre_Archi", [5]int{1510, 1511, 1512, 1513, 1514}, 856, 63},   // BM: do Corvo + Gleipnir
		{"ForeLearner", [5]int{1660, 1661, 1662, 1663, 1664}, 826, 51},    // HT: Legionário + Skytalos
	}

	for _, c := range casos {
		b := templateNPC(t, c.nome)

		for i, quer := range c.set {
			idx, ef := peca(b, i+1)
			if idx != quer {
				t.Errorf("%s: vaga %d tem %d, esperava %d", c.nome, i+1, idx, quer)
				continue
			}
			if v, ok := efeito(ef, efSancVisual); !ok || v != sanc11 {
				t.Errorf("%s: vaga %d com EF_SANC %d, esperava %d (+11)", c.nome, i+1, v, sanc11)
			}
		}
		if idx, _ := peca(b, 6); idx != c.arma {
			t.Errorf("%s: vaga 6 tem %d, esperava a arma %d da classe dele", c.nome, idx, c.arma)
		}
		if idx, _ := peca(b, 7); idx != 0 {
			t.Errorf("%s: vaga 7 tem %d; com a arma na 6 o mestre empunha duas", c.nome, idx)
		}
		// O corpo é a identidade: trocá-lo é trocar o NPC, não vesti-lo.
		if idx, _ := peca(b, 0); idx != c.corpo {
			t.Errorf("%s: corpo virou %d, esperava %d", c.nome, idx, c.corpo)
		}
		montada(t, c.nome, b, nivelDaMontaria)
	}
}

// AS ARMAS DE ARMIA (pedido de 25/09/2026). O Rapein veste o set da Foema — o
// Templário que o Foema_Ancian usa — com a Fúria Divina; a Rainy empunha o Arco
// Divino, o Ferreiro a Solaris e o Arnod, que estava de mãos vazias, a Lança do
// Triunfo. A Kibita leva o Cajado de Âmbar e o Escudo de Runas. Tudo a +11: a
// arma na vaga 6 e, só na Kibita, o escudo na 7 (nPos 128 = vaga 7).
//
// O mestre BM (Mestre_Archi) NÃO entra aqui: fica com a Gleipnir dele, intocado
// (TestMestresDeSkillVestemOSetDaClasse).
//
// É aparência: todos são Merchant, então o EF_RANGE da arma, que o spawn lê, não
// vira alcance de ataque (world/city.go). E os templates só nascem em Armia —
// vestir o arquivo não alcança outro mapa.
func TestArmasDeArmia(t *testing.T) {
	templario := [5]int{1360, 1361, 1362, 1363, 1364}
	casos := []struct {
		nome   string
		arma   int
		escudo int
	}{
		{"Rapein", 900, 0},    // Fúria_Divina
		{"Rainy", 825, 0},     // Arco_Divino
		{"Ferreiro", 911, 0},  // Solaris
		{"Arnod", 855, 0},     // Lança_do_Triunfo
		{"Kibita", 902, 1710}, // Cajado_de_Âmbar + Escudo_de_Runas
	}
	for _, c := range casos {
		b := templateNPC(t, c.nome)
		idx, ef := peca(b, 6)
		if idx != c.arma {
			t.Errorf("%s: vaga 6 tem %d, esperava %d", c.nome, idx, c.arma)
		}
		if v, ok := efeito(ef, efSancVisual); !ok || v != sanc11 {
			t.Errorf("%s: arma com EF_SANC %d, esperava %d (+11)", c.nome, v, sanc11)
		}
		idx, ef = peca(b, 7)
		if idx != c.escudo {
			t.Errorf("%s: vaga 7 tem %d, esperava %d", c.nome, idx, c.escudo)
		}
		if c.escudo != 0 {
			if v, ok := efeito(ef, efSancVisual); !ok || v != sanc11 {
				t.Errorf("%s: escudo com EF_SANC %d, esperava %d (+11)", c.nome, v, sanc11)
			}
		}
		montada(t, c.nome, b, nivelDaMontaria)
	}

	b := templateNPC(t, "Rapein")
	for i, quer := range templario {
		idx, ef := peca(b, i+1)
		if idx != quer {
			t.Errorf("Rapein: vaga %d tem %d, esperava %d (Templário)", i+1, idx, quer)
			continue
		}
		if v, ok := efeito(ef, efSancVisual); !ok || v != sanc11 {
			t.Errorf("Rapein: vaga %d com EF_SANC %d, esperava %d (+11)", i+1, v, sanc11)
		}
	}
	if corpo, _ := peca(b, 0); corpo != 59 {
		t.Errorf("Rapein: corpo virou %d, esperava 59 — o corpo é a identidade dele", corpo)
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

// O TAMANHO DE UM NPC É A CON (pedido de 22/09/2026). O cliente escala o corpo
// de um mob por (CON/2000 + 1) * 0,9 (WYD.exe 0x50D43F), e é só isso que a CON de
// um mob faz: o servidor não a lê para HP, dano ou defesa — esses são campos
// próprios do template. O God_of_War era a referência do servidor, com 3000, e o
// Guarda_Carga e o Dragão da praça dos Reinos passaram a acompanhá-lo.
//
// Em 25/09/2026 o God_of_War (a Honor Store) encolheu 30%: de 3000, escala
// (1,5+1)*0,9 = 2,25, para 1500, escala (0,75+1)*0,9 = 1,575 — 2,25 * 0,7.
//
// A CON é escrita nos DOIS scores. O que o cliente recebe é o CurrentScore
// (protocol/mob.go escreve Con em cs+38); o BaseScore é o que world/api.go copia
// para a entidade. Gravar só um deixa o tamanho dependendo de qual caminho leu.
func TestTamanhoDosNPCsGrandes(t *testing.T) {
	const (
		offBase  = 44 + 38 // BaseScore.Con
		offAtual = 92 + 38 // CurrentScore.Con
	)
	for _, c := range []struct {
		nome string
		con  int16
	}{
		{"God_of_War", 1500},
		{"Guarda_Carga", 3000},
		{"Dragao_Dourado", 3000},
	} {
		b := templateNPC(t, c.nome)
		base := int16(binary.LittleEndian.Uint16(b[offBase : offBase+2]))
		atual := int16(binary.LittleEndian.Uint16(b[offAtual : offAtual+2]))
		if base != c.con {
			t.Errorf("%s: CON do BaseScore = %d, esperava %d", c.nome, base, c.con)
		}
		if atual != c.con {
			t.Errorf("%s: CON do CurrentScore = %d, esperava %d — é esta que o cliente lê", c.nome, atual, c.con)
		}
	}
}

// O GUARDA CARGA VESTE O CAV. LUGEFER (pedido de 22/09/2026).
//
// A aparência do Cavaleiro Lugefer é UM item, o 175 (Cavaleiro_Negro_Lendário),
// repetido nas seis primeiras vagas — é assim que o template dele monta o
// visual, e a vaga 0 é o corpo, que é o que troca a malha do NPC. O manto 290
// (Manto_Negro_Lendário) fecha o conjunto, com o mesmo EF_SANC 6 que ele carrega.
//
// A arma NÃO é a do Lugefer comum: ele empunha a Luna 910, e o pedido foi a
// Luna ANCIENTE. As duas têm a mesma malha (897.0 no ItemList) — quem dá o
// brilho de ancião é o índice, que o cliente conhece. 2890 é o grau que o
// Lugefer_Maligno já usa, o único template do jogo com uma Luna(Anct).
//
// O cavalo é o mesmo Cavalo Equipado N de nível 90 da apresentação de Armia: o
// de Armia já o montava e não foi reescrito, e o segundo ganhou um igual.
//
// As vagas 7 a 13 ficam vazias de propósito. O Lugefer carrega ali os itens
// 786/1936 (os divisores de dano de mob, handler/combat.go), que são MECÂNICA e
// não aparência — copiá-los junto com o visual poria um guarda de cidade
// dividindo o dano que recebe.
func TestGuardaCargaVesteOLugefer(t *testing.T) {
	const (
		corpoLugefer = 175  // Cavaleiro_Negro_Lendário, nas vagas 0 a 5
		sancLugefer  = 3    // o EF_SANC que o Cav._Lugefer carrega no conjunto
		lunaAnct     = 2890 // Luna(Anct), a mesma do Lugefer_Maligno
		mantoNegro   = 290  // Manto_Negro_Lendário
		sancDoManto  = 6
		slotArma     = 6
		slotManto    = 15
	)

	// Os dois que NASCEM. Guarda_Carga__ e ___ existem como arquivo e não têm
	// bloco no NPCGener: vesti-los gravaria bytes que ninguém vê.
	for _, nome := range []string{"Guarda_Carga", "Guarda_Carga_"} {
		b := templateNPC(t, nome)

		for i := 0; i <= 5; i++ {
			idx, ef := peca(b, i)
			if idx != corpoLugefer {
				t.Errorf("%s: vaga %d tem %d, esperava %d (Cavaleiro Negro Lendário)", nome, i, idx, corpoLugefer)
				continue
			}
			if v, ok := efeito(ef, efSancVisual); !ok || v != sancLugefer {
				t.Errorf("%s: vaga %d com EF_SANC %d, esperava %d", nome, i, v, sancLugefer)
			}
		}

		idx, ef := peca(b, slotArma)
		if idx != lunaAnct {
			t.Errorf("%s: arma = %d, esperava a Luna(Anct) %d", nome, idx, lunaAnct)
		}
		if v, ok := efeito(ef, efSancVisual); !ok || v != sanc11 {
			t.Errorf("%s: Luna com EF_SANC %d, esperava %d (+11)", nome, v, sanc11)
		}

		idx, ef = peca(b, slotManto)
		if idx != mantoNegro {
			t.Errorf("%s: manto = %d, esperava %d", nome, idx, mantoNegro)
		}
		if v, ok := efeito(ef, efSancVisual); !ok || v != sancDoManto {
			t.Errorf("%s: manto com EF_SANC %d, esperava %d", nome, v, sancDoManto)
		}

		montada(t, nome, b, nivelDaMontaria)

		// Nada de mecânica de mob entre a arma e a montaria.
		for i := 7; i <= 13; i++ {
			if idx, _ := peca(b, i); idx != 0 {
				t.Errorf("%s: vaga %d tem %d e devia estar vazia — o visual do Lugefer não leva os divisores de dano dele", nome, i, idx)
			}
		}
	}
}
