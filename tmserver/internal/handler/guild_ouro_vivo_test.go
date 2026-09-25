package handler

import (
	"errors"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// TestCreateGuildComOuroRecemSacadoDaCarga: o ouro que a pessoa acabou de sacar
// da carga só existe na memória até o próximo save, e o dbServer confere o custo
// contra character.coin NO BANCO. Em produção, 25/09/2026, isso recusou nove
// tentativas seguidas de /create — e a frase de então falava em nome repetido,
// então ninguém tinha como descobrir que o problema era ouro.
//
// A cura é gravar o personagem ANTES de pedir a criação, não afrouxar a
// conferência: o banco continua sendo o dono da verdade, só que em dia. O fake
// aqui recusa por ouro olhando o último save, que é o que o store faz.
func TestCreateGuildComOuroRecemSacadoDaCarga(t *testing.T) {
	db := newDB()
	db.guildaConfereOuroGravado = true
	db.loadResult = world.CharacterState{
		Slot: 0, Name: "Hero", X: 5, Y: 5, HP: 1000, MaxHP: 1000,
		Clan: 7, Citizen: 1, Coin: 0,
	}
	db.accounts["tester"].cargo = world.CargoState{Coin: guildCreateCost}

	addr, stop, _ := startServerClock(t, db)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	withdrawFrame(t, c, guildCreateCost)
	expect(t, c, protocol.MsgWithdraw)
	drainRaw(t, c)

	whisperFrame(t, c, "create", "Lobos")
	linha := ""
	for i := 0; i < 8 && linha == ""; i++ {
		ty, p, ok := readMaybe(t, c)
		if !ok {
			break
		}
		if ty == protocol.MsgMessagePanel {
			linha = decodePanel(p)
		}
	}
	if linha == "" {
		t.Fatal("o /create não respondeu nada")
	}
	if linha == msgGuildSemOuro {
		t.Fatalf("recusa por ouro com o ouro na mão: o banco leu o saldo velho (%q)", linha)
	}
	if !criouGuilda(db, "Lobos") {
		t.Fatalf("a guilda não foi criada; resposta = %q", linha)
	}
}

// TestCreateGuildNaoCriaSeOSaveFalhar: se a gravação que põe o banco em dia
// falha, seguir adiante devolveria a decisão ao ouro velho — e poderia criar a
// guilda cobrando de um saldo que não existe lá. Tem de recusar.
func TestCreateGuildNaoCriaSeOSaveFalhar(t *testing.T) {
	db := newDB()
	db.loadResult = world.CharacterState{
		Slot: 0, Name: "Hero", X: 5, Y: 5, HP: 1000, MaxHP: 1000,
		Clan: 7, Citizen: 1, Coin: guildCreateCost,
	}
	addr, stop, _ := startServerClock(t, db)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()
	drainRaw(t, c)

	db.saveErr = errors.New("save falhou de propósito")
	whisperFrame(t, c, "create", "Lobos")
	for i := 0; i < 8; i++ {
		if _, _, ok := readMaybe(t, c); !ok {
			break
		}
	}
	if criouGuilda(db, "Lobos") {
		t.Fatal("criou a guilda mesmo com o save falhando")
	}
}

// criouGuilda diz se o dbServer de mentira registrou a guilda com esse nome.
func criouGuilda(db *fakeDB, nome string) bool {
	db.mu.Lock()
	defer db.mu.Unlock()
	for _, g := range db.createdGuilds {
		if g.Name == nome {
			return true
		}
	}
	return false
}
