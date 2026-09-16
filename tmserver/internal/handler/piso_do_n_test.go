package handler

import (
	"net"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O piso do N: um Mortal só entra na perga N e no pesadelo N a partir do nível
// 351 da tela, que é o guardado 350 (waterNMortalMinLevel, pesaNMortalMinLevel).
// Todos os números daqui são o nível GUARDADO. As bordas são 349 e 350, sozinho
// e em grupo, nas duas masmorras.

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
		{"N: Mortal 349 fica de fora", waterN, classMasterMortal, 349, true},
		{"N: Mortal 350 entra", waterN, classMasterMortal, 350, false},
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

func TestPisoEClasseJuntosNaAgua(t *testing.T) {
	for _, tc := range []struct {
		nome    string
		variant int
		classe  uint8
		nivel   int32
		entra   bool
	}{
		{"N: Mortal 349", waterN, classMasterMortal, 349, false},
		{"N: Mortal 350", waterN, classMasterMortal, 350, true},
		{"N: Arch 399", waterN, classMasterArch, 399, false},
		{"M: Arch 1", waterM, classMasterArch, 1, true},
		{"M: Celestial 41", waterM, classMasterCelestial, 41, false},
		{"A: Mortal 399", waterA, classMasterMortal, 399, false},
		{"A: Celestial 1", waterA, classMasterCelestial, 1, true},
	} {
		t.Run(tc.nome, func(t *testing.T) {
			if got := waterAllowed(tc.variant, tc.classe, tc.nivel); got != tc.entra {
				t.Errorf("waterAllowed(%d, classe %d, nivel %d) = %v, esperado %v",
					tc.variant, tc.classe, tc.nivel, got, tc.entra)
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
	if waterNMortalMinLevel != 350 {
		t.Errorf("piso = %d, esperado 350 (guardado; é o 351 da tela)", waterNMortalMinLevel)
	}
}

// A perga abre no nível em que a última quest fecha: nem sobra nível sem as
// duas, nem nível com as duas.
func TestPisoDoNComecaOndeAQuestTermina(t *testing.T) {
	ultima := quest256Steps[len(quest256Steps)-1]
	if ultima.maxLevel != waterNMortalMinLevel {
		t.Errorf("a última quest recusa a partir do %d e a perga N abre no %d; as duas bordas são uma só",
			ultima.maxLevel, waterNMortalMinLevel)
	}
}

// --- sozinho ---

func TestPergaNSozinhoNaBorda(t *testing.T) {
	t.Run("349 recusado, com aviso e o pergaminho de volta", func(t *testing.T) {
		c, stop := pergaNSozinho(t, 349)
		defer stop()
		defer c.Close()

		useItemFrame(t, c, 0)
		if got := noticeCode(t, expect(t, c, protocol.MsgMessageBoxOk)); got != NoticeWaterLevelTooLow {
			t.Errorf("aviso = %v, esperado NoticeWaterLevelTooLow", got)
		}
		if got := decodePanel(expect(t, c, protocol.MsgMessagePanel)); got != "Pergaminho da Água N: entrada a partir do nível 351." {
			t.Errorf("linha = %q, esperado o piso com o número que o cliente mostra", got)
		}
		item := expect(t, c, protocol.MsgSendItem)
		if le16(item[4:6]) != pergaNLV1 {
			t.Errorf("slot = %d, esperado o pergaminho devolvido inteiro", le16(item[4:6]))
		}
	})

	t.Run("350 entra e gasta o pergaminho", func(t *testing.T) {
		c, stop := pergaNSozinho(t, 350)
		defer stop()
		defer c.Close()

		useItemFrame(t, c, 0)
		if !recebeContador(t, c) {
			t.Fatal("o 350 não recebeu o contador da sala")
		}
		item := expect(t, c, protocol.MsgSendItem)
		if le16(item[4:6]) != 0 {
			t.Errorf("slot = %d, esperado vazio depois de entrar", le16(item[4:6]))
		}
	})
}

func TestPesadeloNSozinhoNaBorda(t *testing.T) {
	t.Run("349 recusado, com aviso e o pergaminho de volta", func(t *testing.T) {
		db := pesadeloDB(stageNX, stageNY, classMasterMortal, itemPesadeloGrupoN)
		db.loadResult.Level = 349
		addr, stop := startPesadeloServer(t, db, map[int]int{itemPesadeloGrupoN: volPesadeloN}, at(0, 10))
		defer stop()
		c := enterWorld(t, addr)
		defer c.Close()

		useItemFrame(t, c, 0)
		if got := noticeCode(t, expect(t, c, protocol.MsgMessageBoxOk)); got != NoticePesadeloLevelTooLow {
			t.Errorf("aviso = %v, esperado NoticePesadeloLevelTooLow", got)
		}
		if got := decodePanel(expect(t, c, protocol.MsgMessagePanel)); got != "Pesadelo N: entrada a partir do nível 351." {
			t.Errorf("linha = %q, esperado o piso com o número que o cliente mostra", got)
		}
		if got := le16(expect(t, c, protocol.MsgSendItem)[4:6]); got != itemPesadeloGrupoN {
			t.Errorf("slot = %d, esperado o pergaminho devolvido inteiro", got)
		}
	})

	t.Run("350 entra", func(t *testing.T) {
		db := pesadeloDB(stageNX, stageNY, classMasterMortal, itemPesadeloGrupoN)
		db.loadResult.Level = 350
		addr, stop := startPesadeloServer(t, db, map[int]int{itemPesadeloGrupoN: volPesadeloN}, at(0, 10))
		defer stop()
		c := enterWorld(t, addr)
		defer c.Close()

		useItemFrame(t, c, 0)
		if !recebeContador(t, c) {
			t.Fatal("o 350 não recebeu o contador da janela")
		}
	})
}

// --- em grupo ---
//
// O furo que o piso fecha: o pergaminho é do LÍDER, então sem a checagem no laço
// do grupo qualquer Mortal entrava no N pela mão de um 350.

func TestPergaNGrupoNaBorda(t *testing.T) {
	for _, tc := range []struct {
		nome        string
		nivelMembro int32
		entra       bool
	}{
		{"membro 349 fica para trás", 349, false},
		{"membro 350 vai junto", 350, true},
	} {
		t.Run(tc.nome, func(t *testing.T) {
			db := grupoNaPorta(portaAguaX, portaAguaY, pergaNLV1, 350, classMasterMortal, tc.nivelMembro)
			addr, stop := startServerClockVolGrid(t, db, map[int]int{pergaNLV1: volPergaNLV1}, 4096)
			defer stop()
			lider, membro := formaGrupo(t, addr)
			defer lider.Close()
			defer membro.Close()

			useItemFrame(t, lider, 0)
			if !recebeContador(t, lider) {
				t.Fatal("o líder 350 não entrou na sala")
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
		{"membro 349 fica para trás", 349, false},
		{"membro 350 vai junto", 350, true},
	} {
		t.Run(tc.nome, func(t *testing.T) {
			db := grupoNaPorta(stageNX, stageNY, itemPesadeloGrupoN, 350, classMasterMortal, tc.nivelMembro)
			addr, stop := startPesadeloServer(t, db, map[int]int{itemPesadeloGrupoN: volPesadeloN}, at(0, 10))
			defer stop()
			lider, membro := formaGrupo(t, addr)
			defer lider.Close()
			defer membro.Close()

			useItemFrame(t, lider, 0)
			if !recebeContador(t, lider) {
				t.Fatal("o líder 350 não entrou no pesadelo")
			}
			if got := recebeContador(t, membro); got != tc.entra {
				t.Errorf("membro nível %d recebeu o contador = %v, esperado %v", tc.nivelMembro, got, tc.entra)
			}
		})
	}
}

// A classe também vale no laço da perga, como no do pesadelo: o pergaminho testa
// só o líder, e um Arch entrava na N pela mão de um Mortal.
func TestPergaNArchNoGrupoFicaParaTras(t *testing.T) {
	db := grupoNaPorta(portaAguaX, portaAguaY, pergaNLV1, 350, classMasterArch, 350)
	addr, stop := startServerClockVolGrid(t, db, map[int]int{pergaNLV1: volPergaNLV1}, 4096)
	defer stop()
	lider, membro := formaGrupo(t, addr)
	defer lider.Close()
	defer membro.Close()

	useItemFrame(t, lider, 0)
	if !recebeContador(t, lider) {
		t.Fatal("o líder Mortal 350 não entrou na sala")
	}
	if recebeContador(t, membro) {
		t.Error("um Arch entrou na perga N no grupo de um Mortal")
	}
}

// Um líder abaixo do piso não abre nada para o grupo, nem com um membro que
// passaria: a porta é a do líder.
func TestPesadeloNLiderAbaixoDoPisoNaoLevaNinguem(t *testing.T) {
	db := grupoNaPorta(stageNX, stageNY, itemPesadeloGrupoN, 349, classMasterMortal, 350)
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
		t.Error("o membro 350 entrou pela porta de um líder 349")
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

// grupoNaPorta carrega dois personagens lado a lado: o líder Mortal (conta
// tester, com o pergaminho) e o membro da classe pedida (conta tradeb).
func grupoNaPorta(x, y int16, pergaminho int16, nivelLider int32, classeMembro uint8, nivelMembro int32) *fakeDB {
	db := newDB()
	lider := world.CharacterState{
		Slot: 0, Name: "Hero", X: x, Y: y, HP: 1000, MaxHP: 1000,
		ClassMaster: classMasterMortal, Level: int(nivelLider),
	}
	lider.Carry[0] = world.Item{Index: pergaminho}
	membro := world.CharacterState{
		Slot: 0, Name: "HeroB", X: x + 1, Y: y, HP: 1000, MaxHP: 1000,
		ClassMaster: classeMembro, Level: int(nivelMembro),
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
	if !chegaAte(t, membro, protocol.MsgSendReqParty, prazoDoQuadro) {
		t.Fatal("o convite de grupo não chegou ao membro")
	}
	acceptPartyFrame(t, membro, 1, "Hero")
	drainRaw(t, lider)
	drainRaw(t, membro)
	return lider, membro
}

// recebeContador diz se chegou o MSG_StartTime: a perga e o pesadelo mandam o
// contador só para quem de fato colocaram lá dentro.
func recebeContador(t *testing.T, c net.Conn) bool {
	t.Helper()
	return chegaAte(t, c, protocol.MsgStartTime, prazoDoQuadro)
}

// prazoDoQuadro é quanto se espera por um quadro antes de concluir que ele não
// vem. Folgado de propósito: um silêncio de 300 ms (o readMaybe) numa máquina
// carregada não é prova de ausência, e o caso "não entra" só custa a espera.
const prazoDoQuadro = 2 * time.Second

// chegaAte lê quadros até achar o tipo pedido ou vencer o prazo, atravessando os
// silêncios curtos do readMaybe.
func chegaAte(t *testing.T, c net.Conn, tipo protocol.Type, prazo time.Duration) bool {
	t.Helper()
	fim := time.Now().Add(prazo)
	for time.Now().Before(fim) {
		ty, _, ok := readMaybe(t, c)
		if ok && ty == tipo {
			return true
		}
	}
	return false
}
