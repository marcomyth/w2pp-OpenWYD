package handler

import (
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/level"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// TestLevelUpGrantsPKPoint is the issue #279 core: every level crossed pays one
// Chaos Point back, capped at the neutral 75 and never touching a character
// already at or past it (the quest/item 76..150 range).
func TestLevelUpGrantsPKPoint(t *testing.T) {
	cases := []struct {
		name       string
		pkPoint    uint8
		guilty     uint8
		toLevel    int32 // Exp is set to NextLevelExp(toLevel-1), so levels gained = toLevel-1
		wantPoint  uint8
		wantLevels int32
	}{
		// +5 POR NÍVEL E TETO 150, decisão da Hanna de 25/09/2026. Antes era +1 com
		// teto no neutro (75), e os casos abaixo mudaram de valor junto — os nomes
		// dizem o que cada um pergunta, e as perguntas continuam as mesmas.
		{name: "um nivel, cinco pontos", pkPoint: 70, toLevel: 2, wantPoint: 75, wantLevels: 1},
		{name: "tres niveis, quinze pontos", pkPoint: 0, toLevel: 4, wantPoint: 15, wantLevels: 3},
		// ATRAVESSA O NEUTRO em vez de parar nele: é a mudança que mais se vê em jogo.
		{name: "passa do neutro e acumula a folga", pkPoint: 74, toLevel: 3, wantPoint: 84, wantLevels: 2},
		{name: "quem esta no neutro continua subindo", pkPoint: pkPointNeutral, toLevel: 6, wantPoint: 100, wantLevels: 5},
		// O TETO AGORA É 150, e ele para lá — antes 150 era intocável pelo nível.
		{name: "para no teto de 150", pkPoint: 148, toLevel: 6, wantPoint: 150, wantLevels: 5},
		{name: "no teto, nada muda", pkPoint: 150, toLevel: 6, wantPoint: 150, wantLevels: 5},
		// Guilty pins the *displayed* pkPoint at 0, but the counter underneath must
		// still accrue — otherwise a chaotic player's grind would be wasted.
		{name: "acumula mesmo Guilty", pkPoint: 70, guilty: 5, toLevel: 3, wantPoint: 80, wantLevels: 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, w, e := mobKilledWorld(t) // mortal, level 1
			e.PKPoint, e.Guilty = c.pkPoint, c.guilty
			e.Exp = level.NextLevelExp(c.toLevel - 1)

			d.applyLevelUps(w, nil, e)

			if e.Level != c.toLevel {
				t.Fatalf("level = %d, want %d (%d levels gained)", e.Level, c.toLevel, c.wantLevels)
			}
			if e.PKPoint != c.wantPoint {
				t.Errorf("PKPoint = %d, want %d", e.PKPoint, c.wantPoint)
			}
		})
	}
}

// TestLevelUpPKPointNoLevelNoGrant: applyLevelUps returning false (not enough Exp)
// must not move the counter — the grant hangs off levels crossed, not off calls.
func TestLevelUpPKPointNoLevelNoGrant(t *testing.T) {
	d, w, e := mobKilledWorld(t)
	e.PKPoint = 70
	e.Exp = level.NextLevelExp(1) - 1

	if d.applyLevelUps(w, nil, e) {
		t.Fatal("applyLevelUps = true, want false (one Exp short of level 2)")
	}
	if e.PKPoint != 70 {
		t.Errorf("PKPoint = %d, want 70 (unchanged without a level-up)", e.PKPoint)
	}
}

// pkLevelUpDB seeds a moderator (for /gm setlevel) at level 10 sitting one point
// below neutral, so a single level-up crosses to 75 = white nick.
func pkLevelUpDB(pkPoint uint8) *fakeDB {
	db := &fakeDB{accounts: map[string]*fakeAccount{
		"mod": {id: 20, pass: "secret", role: "moderator", chars: []world.CharSummary{{Slot: 0, Name: "Mod"}}},
	}}
	db.loads = map[int64]world.CharacterState{
		20: {Slot: 0, Name: "Mod", Class: 0, Level: 10, X: 5, Y: 5, HP: 1000, MaxHP: 1000, PKPoint: pkPoint},
	}
	return db
}

// TestLevelUpPKPointNotifiesAndRecolorsNick é a metade do fio: subir de nível tem de
// AVISAR o jogador e REPINTAR o apelido por um CreateMob novo, sem esperar relogin. Um
// salto de vários níveis dá UMA linha, e não uma por nível.
//
// COM O +5 E O TETO 150, este cenário mudou de resultado: partindo de 74 e subindo cinco
// níveis, o contador vai a 99 em vez de parar em 75. O apelido continua BRANCO — é isso
// que a conferência pergunta agora, em vez de exigir o 75 exato, porque qualquer valor do
// neutro para cima pinta branco e travar no número faria o teste reprovar a folga que a
// decisão nova existe para criar.
func TestLevelUpPKPointNotifiesAndRecolorsNick(t *testing.T) {
	addr, stop, _ := startServerClock(t, pkLevelUpDB(74))
	defer stop()

	c := enterWorldAs(t, addr, "mod") // conn 1
	defer c.Close()
	drainRaw(t, c)

	gmFrame(t, c, "setlevel 15") // 5 níveis: 74 + 5×5 = 99, atravessando o neutro

	chatLines, sawWhiteNick := []string{}, false
	for i := 0; i < 40; i++ {
		ty, payload, ok := readMaybeRaw(t, c)
		if !ok {
			break
		}
		switch ty {
		case protocol.MsgMessageChat:
			chatLines = append(chatLines, cstr(payload))
		case protocol.MsgCreateMob:
			if _, _, id := createMobFields(t, payload); id == 1 {
				if payload[6+12] < pkPointNeutral {
					t.Errorf("self-CreateMob MobName[12] = %d, queria %d ou mais (apelido branco)",
						payload[6+12], pkPointNeutral)
				}
				sawWhiteNick = true
			}
		}
	}

	if len(chatLines) != 1 {
		t.Fatalf("chat lines = %q, want exactly one Chaos Point notice for a multi-level jump", chatLines)
	}
	// PKPoint 74 → 99: a tela mostra a folga acima do neutro (99-75 = 24) e o ganho
	// inteiro de +25, num aviso só para os cinco níveis.
	if want := "Pontos Caos atual: 24 (+25)"; !strings.HasPrefix(chatLines[0], want) {
		t.Errorf("chat line = %q, want prefix %q", chatLines[0], want)
	}
	if !sawWhiteNick {
		t.Error("nick was not recolored (no self-CreateMob after the level-up)")
	}
}

// TestLevelUpPKPointCaladoNoTeto: quem já está no teto não recebe linha de chat a cada
// nível — o aviso só sai quando há ponto a pagar.
//
// ESTE TESTE PERGUNTAVA NO NEUTRO (75) e passou a perguntar no TETO (150). A pergunta não
// mudou: "não encher o chat quando não há nada a dar". O que mudou foi ONDE isso é
// verdade — com o +5 até 150, quem está no neutro AINDA GANHA, e o silêncio ali passou a
// ser o comportamento errado.
//
// Mover em vez de apagar importa: sem ele, um ganho de zero passaria a mandar "+0" a cada
// nível para todo personagem no teto, e ninguém veria isso num teste.
func TestLevelUpPKPointCaladoNoTeto(t *testing.T) {
	addr, stop, _ := startServerClock(t, pkLevelUpDB(pkPointPardon))
	defer stop()

	c := enterWorldAs(t, addr, "mod")
	defer c.Close()
	drainRaw(t, c)

	gmFrame(t, c, "setlevel 15")

	for i := 0; i < 40; i++ {
		ty, payload, ok := readMaybeRaw(t, c)
		if !ok {
			break
		}
		if ty == protocol.MsgMessageChat {
			t.Errorf("linha de chat inesperada (%q) subindo de nivel ja no teto", cstr(payload))
		}
	}
}
