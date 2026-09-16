package handler

import (
	"net"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O piso do N: um Mortal só entra na perga N e no pesadelo N a partir do nível
// guardado 351 (waterNMortalMinLevel, pesaNMortalMinLevel). As bordas são 350 e
// 351, sozinho e em grupo, nas duas masmorras.

// pergaNLV1 é o Pergaminho da Água (N) LV1 e o seu EF_VOLATILE (waterVariants).
const (
	pergaNLV1    = 3173
	volPergaNLV1 = 131
)

// O quadrado que aceita o pergaminho fora das salas: x/4 == 491, y/4 == 443.
const portaAguaX, portaAguaY = 1965, 1773

func TestPisoDoNNaAgua(t *testing.T) {
	for _, tc := range []struct {
		nome    string
		variant int
		classe  uint8
		nivel   int32
		abaixo  bool
	}{
		{"N: Mortal 350 fica de fora", waterN, classMasterMortal, 350, true},
		{"N: Mortal 351 entra", waterN, classMasterMortal, 351, false},
		{"N: Mortal 399 entra", waterN, classMasterMortal, 399, false},
		{"N: Mortal 0 fica de fora", waterN, classMasterMortal, 0, true},
		// O piso é do Mortal no N e de mais ninguém: as outras portas seguem só com
		// a classe e os tetos de sempre.
		{"N: Arch baixo nao cai no piso", waterN, classMasterArch, 10, false},
		{"M: Arch baixo nao tem piso", waterM, classMasterArch, 1, false},
		{"A: Celestial baixo nao tem piso", waterA, classMasterCelestial, 1, false},
	} {
		t.Run(tc.nome, func(t *testing.T) {
			if got := waterBelowMinLevel(tc.variant, tc.classe, tc.nivel); got != tc.abaixo {
				t.Errorf("waterBelowMinLevel(%d, classe %d, nivel %d) = %v, esperado %v",
					tc.variant, tc.classe, tc.nivel, got, tc.abaixo)
			}
		})
	}
}

// A borda é UMA regra nas duas masmorras: se um lado mudar sozinho, um nível
// passa a entrar numa e não na outra, e o plano de XP deixa de valer.
func TestPisoDoNEhOMesmoNasDuasMasmorras(t *testing.T) {
	if waterNMortalMinLevel != pesaNMortalMinLevel {
		t.Errorf("piso da Água N = %d, piso do Pesadelo N = %d; a regra é um número só",
			waterNMortalMinLevel, pesaNMortalMinLevel)
	}
	if waterNMortalMinLevel != 351 {
		t.Errorf("piso = %d, esperado 351 (nível guardado; o cliente mostra 352)", waterNMortalMinLevel)
	}
}

// --- sozinho ---

func TestPergaNSozinhoNaBorda(t *testing.T) {
	t.Run("350 recusado, com aviso e o pergaminho de volta", func(t *testing.T) {
		c, stop := pergaNSozinho(t, 350)
		defer stop()
		defer c.Close()

		useItemFrame(t, c, 0)
		if got := noticeCode(t, expect(t, c, protocol.MsgMessageBoxOk)); got != NoticeWaterLevelTooLow {
			t.Errorf("aviso = %v, esperado NoticeWaterLevelTooLow", got)
		}
		if got := decodePanel(expect(t, c, protocol.MsgMessagePanel)); got != "Pergaminho da Água N: entrada a partir do nível 352." {
			t.Errorf("linha = %q, esperado o piso com o número que o cliente mostra", got)
		}
		item := expect(t, c, protocol.MsgSendItem)
		if le16(item[4:6]) != pergaNLV1 {
			t.Errorf("slot = %d, esperado o pergaminho devolvido inteiro", le16(item[4:6]))
		}
	})

	t.Run("351 entra e gasta o pergaminho", func(t *testing.T) {
		c, stop := pergaNSozinho(t, 351)
		defer stop()
		defer c.Close()

		useItemFrame(t, c, 0)
		if !recebeContador(t, c) {
			t.Fatal("o 351 não recebeu o contador da sala")
		}
		item := expect(t, c, protocol.MsgSendItem)
		if le16(item[4:6]) != 0 {
			t.Errorf("slot = %d, esperado vazio depois de entrar", le16(item[4:6]))
		}
	})
}

func TestPesadeloNSozinhoNaBorda(t *testing.T) {
	t.Run("350 recusado, com aviso e o pergaminho de volta", func(t *testing.T) {
		db := pesadeloDB(stageNX, stageNY, classMasterMortal, itemPesadeloGrupoN)
		db.loadResult.Level = 350
		addr, stop := startPesadeloServer(t, db, map[int]int{itemPesadeloGrupoN: volPesadeloN}, at(0, 10))
		defer stop()
		c := enterWorld(t, addr)
		defer c.Close()

		useItemFrame(t, c, 0)
		if got := noticeCode(t, expect(t, c, protocol.MsgMessageBoxOk)); got != NoticePesadeloLevelTooLow {
			t.Errorf("aviso = %v, esperado NoticePesadeloLevelTooLow", got)
		}
		if got := decodePanel(expect(t, c, protocol.MsgMessagePanel)); got != "Pesadelo N: entrada a partir do nível 352." {
			t.Errorf("linha = %q, esperado o piso com o número que o cliente mostra", got)
		}
		if got := le16(expect(t, c, protocol.MsgSendItem)[4:6]); got != itemPesadeloGrupoN {
			t.Errorf("slot = %d, esperado o pergaminho devolvido inteiro", got)
		}
	})

	t.Run("351 entra", func(t *testing.T) {
		db := pesadeloDB(stageNX, stageNY, classMasterMortal, itemPesadeloGrupoN)
		db.loadResult.Level = 351
		addr, stop := startPesadeloServer(t, db, map[int]int{itemPesadeloGrupoN: volPesadeloN}, at(0, 10))
		defer stop()
		c := enterWorld(t, addr)
		defer c.Close()

		useItemFrame(t, c, 0)
		if !recebeContador(t, c) {
			t.Fatal("o 351 não recebeu o contador da janela")
		}
	})
}

// --- em grupo ---
//
// O furo que o piso fecha: o pergaminho é do LÍDER, então sem a checagem no laço
// do grupo qualquer Mortal entrava no N pela mão de um 351.

func TestPergaNGrupoNaBorda(t *testing.T) {
	for _, tc := range []struct {
		nome        string
		nivelMembro int32
		entra       bool
	}{
		{"membro 350 fica para trás", 350, false},
		{"membro 351 vai junto", 351, true},
	} {
		t.Run(tc.nome, func(t *testing.T) {
			db := grupoDeMortais(portaAguaX, portaAguaY, pergaNLV1, 351, tc.nivelMembro)
			addr, stop := startServerClockVolGrid(t, db, map[int]int{pergaNLV1: volPergaNLV1}, 4096)
			defer stop()
			lider, membro := formaGrupo(t, addr)
			defer lider.Close()
			defer membro.Close()

			useItemFrame(t, lider, 0)
			if !recebeContador(t, lider) {
				t.Fatal("o líder 351 não entrou na sala")
			}
			if got := recebeContador(t, membro); got != tc.entra {
				t.Errorf("membro nível %d recebeu o contador = %v, esperado %v", tc.nivelMembro, got, tc.entra)
			}
		})
	}
}

func TestPesadeloNGrupoNaBorda(t *testing.T) {
	for _, tc := range []struct {
		nome        string
		nivelMembro int32
		entra       bool
	}{
		{"membro 350 fica para trás", 350, false},
		{"membro 351 vai junto", 351, true},
	} {
		t.Run(tc.nome, func(t *testing.T) {
			db := grupoDeMortais(stageNX, stageNY, itemPesadeloGrupoN, 351, tc.nivelMembro)
			addr, stop := startPesadeloServer(t, db, map[int]int{itemPesadeloGrupoN: volPesadeloN}, at(0, 10))
			defer stop()
			lider, membro := formaGrupo(t, addr)
			defer lider.Close()
			defer membro.Close()

			useItemFrame(t, lider, 0)
			if !recebeContador(t, lider) {
				t.Fatal("o líder 351 não entrou no pesadelo")
			}
			if got := recebeContador(t, membro); got != tc.entra {
				t.Errorf("membro nível %d recebeu o contador = %v, esperado %v", tc.nivelMembro, got, tc.entra)
			}
		})
	}
}

// Um líder abaixo do piso não abre nada para o grupo, nem com um membro que
// passaria: a porta é a do líder.
func TestPesadeloNLiderAbaixoDoPisoNaoLevaNinguem(t *testing.T) {
	db := grupoDeMortais(stageNX, stageNY, itemPesadeloGrupoN, 350, 351)
	addr, stop := startPesadeloServer(t, db, map[int]int{itemPesadeloGrupoN: volPesadeloN}, at(0, 10))
	defer stop()
	lider, membro := formaGrupo(t, addr)
	defer lider.Close()
	defer membro.Close()

	useItemFrame(t, lider, 0)
	if got := noticeCode(t, expect(t, lider, protocol.MsgMessageBoxOk)); got != NoticePesadeloLevelTooLow {
		t.Errorf("aviso = %v, esperado NoticePesadeloLevelTooLow", got)
	}
	if recebeContador(t, membro) {
		t.Error("o membro 351 entrou pela porta de um líder 350")
	}
}

// --- apoio ---

// pergaNSozinho põe um Mortal do nível pedido no quadrado da porta, com o
// pergaminho N LV1 na bolsa.
func pergaNSozinho(t *testing.T, nivel int32) (net.Conn, func()) {
	t.Helper()
	db := newDB()
	st := world.CharacterState{
		Slot: 0, Name: "Hero", X: portaAguaX, Y: portaAguaY, HP: 1000, MaxHP: 1000,
		ClassMaster: classMasterMortal, Level: int(nivel),
	}
	st.Carry[0] = world.Item{Index: pergaNLV1}
	db.loadResult = st
	addr, stop := startServerClockVolGrid(t, db, map[int]int{pergaNLV1: volPergaNLV1}, 4096)
	return enterWorld(t, addr), stop
}

// grupoDeMortais carrega dois Mortais lado a lado: o líder (conta tester, com o
// pergaminho) e o membro (conta tradeb).
func grupoDeMortais(x, y int16, pergaminho int16, nivelLider, nivelMembro int32) *fakeDB {
	db := newDB()
	lider := world.CharacterState{
		Slot: 0, Name: "Hero", X: x, Y: y, HP: 1000, MaxHP: 1000,
		ClassMaster: classMasterMortal, Level: int(nivelLider),
	}
	lider.Carry[0] = world.Item{Index: pergaminho}
	membro := world.CharacterState{
		Slot: 0, Name: "HeroB", X: x + 1, Y: y, HP: 1000, MaxHP: 1000,
		ClassMaster: classMasterMortal, Level: int(nivelMembro),
	}
	db.loads = map[int64]world.CharacterState{7: lider, 11: membro}
	return db
}

// formaGrupo entra com as duas contas e fecha o grupo: o 1 convida o 2 e o 2
// aceita.
func formaGrupo(t *testing.T, addr string) (lider, membro net.Conn) {
	t.Helper()
	lider = enterWorldAs(t, addr, "tester")
	membro = enterWorldAs(t, addr, "tradeb")
	reqPartyFrame(t, lider, 1, 2)
	expectPartyFrame(t, membro, protocol.MsgSendReqParty)
	acceptPartyFrame(t, membro, 1, "Hero")
	drainRaw(t, lider)
	drainRaw(t, membro)
	return lider, membro
}

// recebeContador lê até o fluxo calar e diz se chegou o MSG_StartTime: a perga e
// o pesadelo mandam o contador só para quem de fato colocaram lá dentro.
func recebeContador(t *testing.T, c net.Conn) bool {
	t.Helper()
	for {
		ty, _, ok := readMaybe(t, c)
		if !ok {
			return false
		}
		if ty == protocol.MsgStartTime {
			return true
		}
	}
}
