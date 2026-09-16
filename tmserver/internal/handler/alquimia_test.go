package handler

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/mountrate"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/refine"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const classHuntress = 3

// huntress monta uma HT com as skills pedidas: alq liga a Alquimia (bit 12),
// troca liga a Troca de Espírito (bit 15).
func huntress(alq, troca bool) *world.Entity {
	e := &world.Entity{ID: 1, HP: 100, Class: classHuntress}
	if alq {
		e.LearnedSkill |= alquimiaSkillBit
	}
	if troca {
		e.LearnedSkill |= learnedTrocaDeEspirito
	}
	return e
}

func TestBonusAlquimia(t *testing.T) {
	outraClasse := &world.Entity{Class: 1, LearnedSkill: alquimiaSkillBit | learnedTrocaDeEspirito}
	cases := []struct {
		name string
		e    *world.Entity
		want int
	}{
		{"sem entidade", nil, 0},
		{"não é HT, mesmo com os bits", outraClasse, 0},
		{"HT sem a Alquimia", huntress(false, false), 0},
		{"HT só com a Troca de Espírito", huntress(false, true), 0},
		{"HT com a Alquimia", huntress(true, false), 2},
		{"HT com Alquimia e Troca de Espírito", huntress(true, true), 4},
	}
	for _, c := range cases {
		if got := bonusAlquimia(c.e); got != c.want {
			t.Errorf("%s: bonusAlquimia = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestChanceComAlquimia(t *testing.T) {
	cases := []struct {
		name                 string
		e                    *world.Entity
		chance               int
		wantFinal, wantBonus int
	}{
		{"não é HT", &world.Entity{Class: 1, LearnedSkill: alquimiaSkillBit}, 41, 41, 0},
		{"HT sem a skill", huntress(false, false), 41, 41, 0},
		{"+2", huntress(true, false), 41, 43, 2},
		{"+4", huntress(true, true), 41, 45, 4},
		{"teto 100", huntress(true, true), 98, 100, 2},
		{"já em 100", huntress(true, true), 100, 100, 0},
		{"acima de 100 não é cortado", huntress(true, false), 104, 104, 0},
		{"chance 0 continua impossível", huntress(true, true), 0, 0, 0},
		{"chance negativa fica", huntress(true, true), -5, -5, 0},
	}
	for _, c := range cases {
		final, bonus := chanceComAlquimia(c.e, c.chance)
		if final != c.wantFinal || bonus != c.wantBonus {
			t.Errorf("%s: chanceComAlquimia(%d) = (%d, %d), want (%d, %d)",
				c.name, c.chance, final, bonus, c.wantFinal, c.wantBonus)
		}
	}
}

// O primeiro rand() do mundo é 41 (MSVC, semente 1): com taxa 40 o refino falha
// para qualquer um, e a HT com a Alquimia passa, porque 41 <= 42.
func TestRefineComAlquimia(t *testing.T) {
	cases := []struct {
		name  string
		e     *world.Entity
		level int
	}{
		{"sem a skill falha", huntress(false, false), 0},
		{"outra classe falha", &world.Entity{ID: 1, HP: 100, Class: 1, LearnedSkill: alquimiaSkillBit}, 0},
		{"HT com a Alquimia passa", huntress(true, false), 1},
	}
	for _, c := range cases {
		f := newRefineFixture(t, alwaysRate(40), nil)
		f.e = c.e
		f.e.Carry[1] = world.Item{Index: itemArmor}
		f.refine(itemPoeiraLac)
		if got := refine.Level(f.target()); got != c.level {
			t.Errorf("%s: nível = %d, want %d", c.name, got, c.level)
		}
	}
}

// A máquina anuncia a chance real e marca de onde vieram os pontos a mais. Taxa
// 40 e primeiro sorteio 41: sem a Alquimia é falha, com ela é sucesso em 41/42.
func TestCompositorAnunciaAlquimia(t *testing.T) {
	cases := []struct {
		name    string
		learned int32
		class   int
		parm    int32
		want    string
	}{
		{"sem a skill", 0, classHuntress, combineFailed, "Hero falhou em 41/40 ao compor #9999."},
		{"+2", alquimiaSkillBit, classHuntress, combineSuccess, "Hero conseguiu em 41/42 (+2 Alquimia) compor #9999!"},
		{"+4", alquimiaSkillBit | learnedTrocaDeEspirito, classHuntress, combineSuccess, "Hero conseguiu em 41/44 (+4 Alquimia) compor #9999!"},
		{"outra classe", alquimiaSkillBit, 1, combineFailed, "Hero falhou em 41/40 ao compor #9999."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := combineDB()
			db.loadResult.Class = c.class
			db.loadResult.LearnedSkill = c.learned
			addr, stop := startServerCombine(t, db, 40)
			defer stop()
			conn := enterWorld(t, addr)
			defer conn.Close()

			combineFrame(t, conn)
			p, _, texts := readOutcome(t, conn)
			if parmOf(t, p) != c.parm {
				t.Errorf("parm = %d, want %d", parmOf(t, p), c.parm)
			}
			if len(texts) != 1 || texts[0] != c.want {
				t.Errorf("anúncio = %q, want [%q]", texts, c.want)
			}
		})
	}
}

// Linha que a marca empurra além do painel troca "Alquimia" por "Alq.", em vez
// de o cliente cortar o nome do item.
func TestRollLineMarcaCurtaQuandoNaoCabe(t *testing.T) {
	curta := rollLine("Hero", "compor Espada", 41, 42, 2, true)
	if curta != "Hero conseguiu em 41/42 (+2 Alquimia) compor Espada!" {
		t.Errorf("linha curta = %q", curta)
	}
	longa := rollLine("NomeDeDezesseis!", "passar o ADD para Machado_de_Arremesso(Anct)", 99, 100, 2, false)
	if !strings.Contains(longa, "99/100 (+2 Alq.) ao") {
		t.Errorf("linha longa não usou a marca curta: %q", longa)
	}
	if n := len(protocol.ClientText(longa)); n > anuncioPainelMax {
		t.Errorf("linha longa tem %d bytes, acima dos %d do painel", n, anuncioPainelMax)
	}
	if got := rollLine("Hero", "compor Espada", 41, 40, 0, false); got != "Hero falhou em 41/40 ao compor Espada." {
		t.Errorf("sem bônus a linha mudou: %q", got)
	}
}

// Primeiro Intn(101) é 41: montaria adulta numa faixa de 40% falha sem a
// Alquimia e cresce com ela (41 > 42 é falso).
func TestAmagoComAlquimia(t *testing.T) {
	cases := []struct {
		name  string
		e     *world.Entity
		level uint8
	}{
		{"sem a skill não cresce", huntress(false, false), 5},
		{"HT com a Alquimia cresce", huntress(true, false), 5 + mountLevelUp},
	}
	for _, c := range cases {
		curve := mountrate.UnsetCurve()
		curve[0] = 40 // níveis 1..20
		adultIndex := int16(itemCriaAndaluzN + mountRowSize)
		d := New(Config{
			Log:        slog.New(slog.DiscardHandler),
			MountRates: mountrate.Table{adultIndex: curve},
		})
		w := world.New(world.Config{GridDim: 16}, slog.New(slog.DiscardHandler), nil, nil)
		s := &world.Session{Conn: 1, Mode: world.UserPlay}
		e := c.e
		m := world.Item{Index: adultIndex}
		putShort(&m.Effects[0], 20000)
		m.Effects[1].Effect = 5
		e.Equip[mountEquipSlot] = m
		e.Carry[0] = world.Item{Index: itemAmagoAndaluzN}
		body := protocol.MsgUseItemBody{SourType: world.ItemPlaceCarry, SourPos: 0, DestType: 0, DestPos: mountEquipSlot}

		d.useAmago(w, s, e, body, 0)

		if got := e.Equip[mountEquipSlot].Effects[1].Effect; got != c.level {
			t.Errorf("%s: nível da montaria = %d, want %d", c.name, got, c.level)
		}
	}
}

// Pedra Arch: a Alquimia só estende o teto de sucesso; o que ela ganha cai no
// último degrau, e a escada abaixo da taxa fica como está.
func TestPedraArchComAlquimia(t *testing.T) {
	const source, rate, rarest, first = 1759, 60, 1751, 1748
	cases := []struct {
		name   string
		e      *world.Entity
		roll   int
		want   int16
		wantOK bool
	}{
		{"sem a skill, acima da taxa falha", huntress(false, false), rate + 1, 0, false},
		{"+2 ganha rate+1", huntress(true, false), rate + 1, rarest, true},
		{"+2 ganha rate+2", huntress(true, false), rate + 2, rarest, true},
		{"+2 não ganha rate+3", huntress(true, false), rate + 3, 0, false},
		{"+4 ganha rate+4", huntress(true, true), rate + 4, rarest, true},
		{"escada abaixo da taxa intacta", huntress(true, true), 0, first, true},
	}
	for _, c := range cases {
		got, ok := pedraArchResult(source, c.roll, c.e)
		if ok != c.wantOK || got != c.want {
			t.Errorf("%s: pedraArchResult(roll %d) = (%d, %v), want (%d, %v)", c.name, c.roll, got, ok, c.want, c.wantOK)
		}
	}
}
