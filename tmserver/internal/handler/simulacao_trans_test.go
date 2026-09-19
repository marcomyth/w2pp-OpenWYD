//go:build simulacao

package handler

// Simulação do TK Trans (17/09/2026): o Porradeiro de hoje e o Porradeiro com as
// regras da árvore Trans (arvore_trans.go), contra a Xorimpas, com poção.
//
//	go test -tags simulacao -run TestSimulacaoTrans -v ./tmserver/internal/handler/
//
// O Porradeiro Trans parte da MESMA janela. O que a janela já traz do legado sai
// antes de as regras novas entrarem: o crítico da Armadura (Special[3]/10 +
// DES/75, mínimo 4) e os +10% de defesa. O bônus de dano da espada fica fora da
// calibração do Ataque, para somar por cima dos 5.515. A maestria da Trans é 255,
// e ele tem as oito skills da árvore (a Noção de Combate entre elas).

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const (
	simEdenAnct      = 3761
	simDemolidorAnct = 3781
	simArvoreTrans   = 0xFF00 // skills 8-15
)

func (sm *simulador) montarTrans(j janela, id int, arma int16) (*world.Entity, int32) {
	legado := max(4, (int(j.special[3])+1)/10+int(j.dex)/75)
	j.special[2] = 255
	e := &world.Entity{ID: id, Class: j.classe, ClassMaster: classMasterMortal, Level: 399,
		Str: j.str, BaseStr: j.str, Dex: j.dex, BaseDex: j.dex, Con: j.con, BaseCon: j.con,
		HP: j.hp, MaxHP: j.hp, Special: j.special, BaseSpecial: j.special, LearnedSkill: simArvoreTrans}
	e.Equip[weaponSlotR] = world.Item{Index: arma, Effects: [3]world.Effect{{Effect: simEfSanc, Value: simRefino11}}}
	sm.d.applyAffectScore(e)

	plana := j.defesa * 1000 / 1100
	e.AC = plana + skillDerivedACBonus(e, plana) - e.AffAC
	e.Critical = uint8(max(0, min(j.criticoPct10/4-legado, 255)))

	multi := e.AffDamageMultiPct
	e.AffDamageMultiPct = 100
	lo, hi := int32(0), int32(200_000)
	for lo < hi {
		mid := (lo + hi) / 2
		e.Damage = mid
		if sm.d.effectiveDamage(e) < j.ataque {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	e.Damage = lo
	e.AffDamageMultiPct = multi

	hp := j.hp * int32(1000+hpDoTransPermil(e, sm.d.itemAbility(e.Equip[weaponSlotR], efWType))) / 1000
	e.HP = hp
	return e, hp
}

func (sm *simulador) transLutador(id int, arma int16) *lutador {
	e, hp := sm.montarTrans(porradeiro, id, arma)
	l := &lutador{lado: &lado{e: e, cd: map[int]int64{}}, nome: "TK", maxHP: hp}
	l.acao = func(ld *lado, alvo *world.Entity, _ int64) golpe { return sm.fisico(ld, alvo) }
	return l
}

func TestSimulacaoTrans(t *testing.T) {
	const lutas = 30
	root := filepath.Join("..", "..", "..", "Release")
	sm := novoSimulador(t, root)
	var b strings.Builder
	fmt.Fprintf(&b, "# TK Trans contra a Xorimpas — %d lutas por linha, com poção\n\n", lutas)

	tks := []struct {
		nome string
		novo func(int) *lutador
	}{
		{"Porradeiro hoje", sm.tkLutador},
		{"Porradeiro Trans, Éden (Anct) +11", func(id int) *lutador { return sm.transLutador(id, simEdenAnct) }},
		{"Porradeiro Trans, Demolidor Celestial (Anct) +11", func(id int) *lutador { return sm.transLutador(id, simDemolidorAnct) }},
	}

	fmt.Fprintf(&b, "## Personagens\n\n| | Ataque | Defesa | HP | Crítico na janela | Esquiva da HT contra ele | Esquiva dele contra a HT |\n|---|---|---|---|---|---|---|\n")
	ht := sm.htLutador(1)
	for _, c := range tks {
		tk := c.novo(2)
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %.1f%% | %.1f%% | %.1f%% |\n", c.nome, sm.d.effectiveDamage(tk.e), effectiveAC(tk.e), tk.maxHP,
			float64(effectiveCritical(tk.e))*0.4, float64(sm.d.parryRate(tk.e, ht.e))/10, float64(sm.d.parryRate(ht.e, tk.e))/10)
	}
	fmt.Fprintf(&b, "| Xorimpas (HT) | %d | %d | %d | %.1f%% | | |\n\n", sm.d.effectiveDamage(ht.e), effectiveAC(ht.e), ht.maxHP, float64(effectiveCritical(ht.e))*0.4)

	fmt.Fprintf(&b, "## Lutas\n\n| %% em jogador | TK | Vitórias | Mínimo | Mediana | Máximo | 1-2 min | Acerto do TK | Críticos do TK | Dano médio do TK | Acerto da HT | Dano médio da HT | Dano/s do TK | Dano/s da HT |\n|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	var detalhe strings.Builder
	for _, pct := range []int32{100, 37} {
		sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = pct, pct
		for _, c := range tks {
			var tempos []float64
			vitorias := map[string]int{}
			naMeta := 0
			var golpesTK, golpesHT []golpe
			for i := range lutas {
				l := sm.lutaEntre(sm.htLutador(1), c.novo(2), i%2 == 0)
				s := float64(l.ms) / 1000
				tempos = append(tempos, s)
				vitorias[l.vencedor]++
				if s >= 60 && s <= 120 {
					naMeta++
				}
				golpesHT, golpesTK = append(golpesHT, l.golpesA...), append(golpesTK, l.golpesB...)
			}
			somaTempo := 0.0
			for _, x := range tempos {
				somaTempo += x
			}
			sort.Float64s(tempos)
			partes := []string{}
			for nome, n := range vitorias {
				partes = append(partes, fmt.Sprintf("%s %d", nome, n))
			}
			sort.Strings(partes)
			tk, hh := resumir(golpesTK, ""), resumir(golpesHT, "")
			fmt.Fprintf(&b, "| %d%% | %s | %s | %.0f s | %.0f s | %.0f s | %d | %d%% | %d%% | %d | %d%% | %d | %.0f | %.0f |\n", pct, c.nome, strings.Join(partes, ", "),
				tempos[0], tempos[len(tempos)/2], tempos[len(tempos)-1], naMeta,
				div(100*tk.acertos, tk.n), div(100*tk.crits, tk.acertos), div(tk.soma, tk.acertos), div(100*hh.acertos, hh.n), div(hh.soma, hh.acertos),
				float64(tk.soma)/somaTempo, float64(hh.soma)/somaTempo)
			if pct == 37 && c.nome != "Porradeiro hoje" {
				tabelaPorGolpe(&detalhe, "HT no "+c.nome+" (37%)", golpesHT)
			}
		}
	}
	b.WriteString("\n")
	b.WriteString(detalhe.String())

	t.Log("\n" + b.String())
	if out := os.Getenv("SIM_OUT"); out != "" {
		if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
