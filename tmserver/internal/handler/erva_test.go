package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// AS ERVAS DE CURA (415).
//
// Até 21/09/2026 a erva era engolida sem fazer nada: ela carrega o volátil 243
// da Jóia da Recuperação, e o handler só limpava afeto quando o índice era a
// jóia. A regra pedida é "limpa lentidão e debuff básico, nada como um cancel",
// e é a segunda metade dela que estes testes existem para guardar — a primeira
// qualquer um vê em jogo, a segunda só aparece quando alguém perde um buff que
// não devia ter perdido.

// ervaComAfetos monta um personagem com a erva na vaga 0 e os afetos dados.
func ervaComAfetos(afetos ...world.Affect) *fakeDB {
	db := joiaDB(ervasDeCura)
	st := db.loadResult
	st.Affects = afetos
	db.loadResult = st
	return db
}

func servidorDaErva(t *testing.T, db *fakeDB) (string, func()) {
	t.Helper()
	return startServerClockVol(t, db, map[int]int{ervasDeCura: volJoiaRecovery})
}

// A lentidão (afeto 1) é o caso que deu nome ao pedido.
func TestErvaTiraALentidao(t *testing.T) {
	addr, stop := servidorDaErva(t, ervaComAfetos(world.Affect{Type: 1, Level: 1, Time: 100}))
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	drain(t, c) // o retrato de afetos do login carrega a lentidão
	useCarry(t, c, 0)

	for range 8 {
		ty, payload, ok := readMaybe(t, c)
		if !ok {
			break
		}
		if ty == protocol.MsgSendAffect {
			if payload[0] != 0 {
				t.Fatalf("a vaga 0 de afeto ficou com o tipo %d, esperava 0 — a lentidão não saiu", payload[0])
			}
			return
		}
	}
	t.Fatal("nenhum MsgSendAffect depois da erva")
}

// O veneno (afeto 20) é o outro que o jogador chama de debuff básico.
func TestErvaTiraOVeneno(t *testing.T) {
	addr, stop := servidorDaErva(t, ervaComAfetos(world.Affect{Type: 20, Value: 10, Time: 100}))
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	drain(t, c)
	useCarry(t, c, 0)

	for range 8 {
		ty, payload, ok := readMaybe(t, c)
		if !ok {
			break
		}
		if ty == protocol.MsgSendAffect {
			if payload[0] != 0 {
				t.Fatalf("a vaga 0 de afeto ficou com o tipo %d, esperava 0 — o veneno não saiu", payload[0])
			}
			return
		}
	}
	t.Fatal("nenhum MsgSendAffect depois da erva")
}

// O CASO QUE IMPORTA: a erva NÃO é um cancelamento.
//
// O afeto 5 (Fanatismo) é buff, e está na lista da Jóia da Recuperação. Se um
// dia alguém apontar a erva para joiaRecoveryCleanse "porque é a mesma família",
// é aqui que isso aparece: o buff tem de sobreviver, a erva tem de ficar na
// bolsa e o jogador tem de ouvir o porquê.
func TestErvaNaoTiraBuffNemSeGasta(t *testing.T) {
	addr, stop := servidorDaErva(t, ervaComAfetos(world.Affect{Type: 5, Level: 1, Time: 100}))
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	drain(t, c)
	useCarry(t, c, 0)

	viuItem := false
	for range 8 {
		ty, payload, ok := readMaybe(t, c)
		if !ok {
			break
		}
		switch ty {
		case protocol.MsgSendAffect:
			if payload[0] != 5 {
				t.Errorf("a vaga 0 de afeto virou %d: a erva levou um BUFF junto", payload[0])
			}
		case protocol.MsgSendItem:
			viuItem = true
			if le16(payload[4:6]) != ervasDeCura {
				t.Errorf("a vaga 0 da bolsa virou %d: a erva se gastou sem ter o que curar",
					le16(payload[4:6]))
			}
		}
	}
	if !viuItem {
		t.Error("o servidor não devolveu a vaga da bolsa; o cliente fica com a erva fantasma")
	}
}

// Sem afeto nenhum, o mesmo: a erva fica na bolsa.
func TestErvaSemNadaParaCurarNaoSeGasta(t *testing.T) {
	addr, stop := servidorDaErva(t, ervaComAfetos())
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	drain(t, c)
	useCarry(t, c, 0)

	item := expect(t, c, protocol.MsgSendItem)
	if le16(item[4:6]) != ervasDeCura {
		t.Errorf("a vaga 0 da bolsa virou %d, esperava a erva %d de volta", le16(item[4:6]), ervasDeCura)
	}
}

// A lista é sobre debuff, e só. Qualquer afeto que entre nela precisa piorar a
// ficha de quem o carrega — este teste é a trava contra a lista crescer para o
// lado dos buffs, que é o que transformaria a erva num cancelamento barato.
func TestListaDaErvaSoTemDebuff(t *testing.T) {
	debuffs := map[uint8]string{
		1:  "Toque Sagrado (lentidão)",
		3:  "Perseguição",
		10: "Enfraquecer",
		12: "quebra de AC",
		20: "veneno",
	}
	for tipo := range ervasDeCuraCleanse {
		if _, ok := debuffs[tipo]; !ok {
			t.Errorf("o afeto %d entrou na lista da erva e não é debuff conhecido", tipo)
		}
	}
	for tipo, nome := range debuffs {
		if !ervasDeCuraCleanse[tipo] {
			t.Errorf("o afeto %d (%s) saiu da lista da erva", tipo, nome)
		}
	}
	// E não é a lista da jóia: a jóia leva buff junto, a erva não.
	for _, buff := range []uint8{5, 7, 32} {
		if ervasDeCuraCleanse[buff] {
			t.Errorf("o afeto %d é buff da lista da Jóia da Recuperação e não pode estar na erva", buff)
		}
	}
}
