//go:build simulacao

package handler

// Simulação da FM Magia Negra (17/09/2026): a TestPErg do print do Marco, com as
// regras de arvore_magia_negra.go, contra a Xorimpas (uma 8ª de cada vez), os
// TKs de físico, o Paladino e o TK-MAGO, com poção.
//
//	go test -tags simulacao -run TestSimulacaoMagiaNegra -v ./tmserver/internal/handler/
//
// A TestPErg é tratada como Mortal, como as outras referências: FOR 12, INT
// 3.148, DES 12, CON 211, HP 8.258, MP 28.548, Ataque 1.323, Defesa 2.357,
// crítico 20,8%, Magia Negra 255, Magia Especial 200, cajado de 2 mãos +11. A
// Magia é calibrada para o Atq Mágico (20.142) bater com o Inferno pela conta do
// cliente, que não conhece o cajado de 140%.
//
// Ela entra com o Controle de Mana (46) ativo: enquanto a mana passa de 10% da
// barra, o golpe sai da mana e só um terço chega à vida. A poção de mana devolve
// até 2.000 por segundo, e o roubo de mana soma. O Trovão (37) fica de fora — os
// Relâmpagos automáticos procuram alvos no mapa, que a simulação não tem.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/combat"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const (
	simBlackHP           = 8_258
	simBlackMP           = 28_548
	simBlackAtqMagic     = 20_142
	simEirenus           = 903 // cajado de 2 mãos
	affectControleDeMana = 18
)

var nomeSkillBlack = map[int]string{
	32: "Ataque de Fogo", 33: "Relâmpago", 34: "Lança de Gelo", 35: "Tempestade de Meteoros",
	36: "Nevasca", 38: "Fênix de Fogo", 39: "Inferno",
}

// ordemDaBlack: do maior valor para o menor, pela recarga do SkillData.
var ordemDaBlack = []int{39, 38, 36, 35, 34, 33, 32}

func (sm *simulador) casterDaBlack(e *world.Entity) combat.SkillCaster {
	return combat.SkillCaster{
		Class: int(e.Class), Level: int(e.Level), Str: int(effectiveStr(e)), Int: int(effectiveInt(e)),
		Magic: int(effectiveMagic(e)), Special: effectiveSpecial(e, 2),
		DamageMultiPct: sm.d.spellDamageMultiPct(e) + int(e.DanoMagicoPct),
		Mortal:         e.ClassMaster == classMasterMortal, LearnedSkill: e.LearnedSkill,
	}
}

func (sm *simulador) montarBlack(id int) *world.Entity {
	e := &world.Entity{ID: id, Class: 1, ClassMaster: classMasterMortal, Level: 399,
		Str: 12, BaseStr: 12, Dex: 12, BaseDex: 12, Int: 3148, BaseInt: 3148, Con: 211, BaseCon: 211,
		LearnedSkill: 0x7FFF00} // Magia Negra inteira e a Magia Especial até o Controle de Mana
	e.Special = [4]int16{0, 1, 255, 200}
	e.BaseSpecial = e.Special
	e.Equip[weaponSlotR] = world.Item{Index: simEirenus, Effects: [3]world.Effect{{Effect: simEfSanc, Value: simRefino11}}}
	e.Affect[0] = world.Affect{Type: affectControleDeMana, Time: 5000}
	sm.d.applyAffectScore(e)
	e.MaxHP = (simBlackHP - e.AffMaxHP) / 2
	e.MaxMP = (simBlackMP - e.AffMaxMP) / 2
	e.HP, e.MP = simBlackHP, simBlackMP
	e.AC = 2357 - e.AffAC
	e.Critical = uint8(208 / 4)
	lo, hi := int32(0), int32(200_000)
	for lo < hi {
		mid := (lo + hi) / 2
		e.Damage = mid
		if sm.d.effectiveDamage(e) < 1323 {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	e.Damage = lo
	sp, _ := sm.d.spells.Get(skillInferno)
	spell := combat.SkillSpell{InstanceType: sp.InstanceType, InstanceValue: sp.InstanceValue, AffectValue: sp.AffectValue}
	for m := int16(0); m <= maxMagic; m++ {
		e.Magic = m
		if combat.SkillBaseDamage(skillInferno, spell, sm.casterDaBlack(e), 0, int(sm.d.weaponDamage(e))) >= simBlackAtqMagic {
			break
		}
	}
	return e
}

func (sm *simulador) blackLutador(id int) *lutador {
	e := sm.montarBlack(id)
	l := &lutador{lado: &lado{e: e, cd: map[int]int64{}}, nome: "Black", maxHP: simBlackHP}
	var ultimo int64
	l.acao = func(ld *lado, alvo *world.Entity, agora int64) golpe {
		ld.e.MP = min(simBlackMP, ld.e.MP+int32((agora-ultimo)*applyCasting/1000))
		ultimo = agora
		for _, sk := range ordemDaBlack {
			if agora < ld.cd[sk] {
				continue
			}
			sp, _ := sm.d.spells.Get(sk)
			custo := int32(combat.ManaSpent(sp.ManaSpent, 0, 255))
			if ld.e.MP < custo {
				continue
			}
			ld.e.MP -= custo
			ld.cd[sk] = agora + max(int64(sp.Delay)*1000, simPasso)
			return sm.skillDaBlack(l, alvo, sk)
		}
		return sm.fisico(ld, alvo)
	}
	return l
}

// skillDaBlack segue a ordem do handler: dano, crítico de mago, esquiva, bloco PvP
// e, sobre o que entrou, o roubo de mana. O extra do golpe é a mana devolvida.
func (sm *simulador) skillDaBlack(l *lutador, alvo *world.Entity, sk int) golpe {
	e := l.e
	cast, _ := sm.cast(l.lado, alvo, sk)
	dmg := sm.d.resolveSkillHit(sm.w, e, alvo, alvo.ID, sk, cast)
	crit := false
	if dmg > 0 {
		if mult := rolarCriticoDeMago(sm.w.Rand(), e); mult > 0 {
			dmg, crit = dmg*mult/10, true
		}
		miss := combat.ResolveParry(sm.w.Rand(), sk, sm.d.skillParryRate(e, alvo), alvo.Rsv&world.RsvBlock != 0)
		if miss = capMissStreak(e, alvo.ID, miss, int(sm.d.combatRules.MaxMissStreak)); miss != 0 {
			dmg = miss
		}
	}
	final := sm.aplicar(l.lado, alvo, dmg, 0, true)
	mana := rouboDeMana(sm.w.Rand(), e, final, tetoDoRouboDeMana(e))
	antes := e.MP
	e.MP = min(simBlackMP, e.MP+mana)
	return golpe{tipo: nomeSkillBlack[sk], dano: final, crit: crit, extra: int(e.MP - antes)}
}

func TestSimulacaoMagiaNegra(t *testing.T) {
	const lutas = 30
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	var b strings.Builder
	fmt.Fprintf(&b, "# FM Magia Negra (TestPErg) — %d lutas por linha, com poção\n\n", lutas)

	black := sm.montarBlack(1)
	sp, _ := sm.d.spells.Get(skillInferno)
	spell := combat.SkillSpell{InstanceType: sp.InstanceType, InstanceValue: sp.InstanceValue, AffectValue: sp.AffectValue}
	cliente := combat.SkillBaseDamage(skillInferno, spell, sm.casterDaBlack(black), 0, int(sm.d.weaponDamage(black)))
	cast := sm.casterDaBlack(black)
	cast.ArmaPct = armaPctMagiaNegra(black, sm.d.itemAbility)
	servidor := combat.SkillBaseDamage(skillInferno, spell, cast, 0, int(sm.d.weaponDamage(black)))
	fmt.Fprintf(&b, "## TestPErg montada\n\n- Magia calibrada: %d. Inferno: %d na conta do cliente, %d com o cajado de %d%%.\n", black.Magic, cliente, servidor, cast.ArmaPct)
	fmt.Fprintf(&b, "- HP %d, MP %d (mana máxima efetiva %d); teto do roubo de mana por lançamento: %d.\n\n", black.HP, black.MP, effectiveMaxMP(black), tetoDoRouboDeMana(black))

	oponentes := []struct {
		nome string
		novo func(int) *lutador
	}{
		{"Xorimpas, 8ª Sobrevivência", func(id int) *lutador { return sm.htUmaOitava(id, learnedTempestade) }},
		{"Xorimpas, 8ª Troca", func(id int) *lutador { return sm.htUmaOitava(id, 1<<15) }},
		{"Xorimpas, 8ª Captura", func(id int) *lutador { return sm.htUmaOitava(id, learnedInvisibilidade) }},
		{"Porradeiro hoje", sm.tkLutador},
		{"Porradeiro Trans, Éden", func(id int) *lutador { return sm.transLutador(id, simEdenAnct) }},
		{"Paladino (Confiança)", sm.paladinoLutador},
		{"TK-MAGO (Espada Mágica)", sm.tkMagoLutador},
	}
	fmt.Fprintf(&b, "| %% PvP | Oponente | Vitórias | Mínimo | Mediana | 1-2 min | Dano/s Black | Dano/s oponente | Maior golpe Black | Maior golpe oponente | Críticos nas skills | Mana roubada/s |\n|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	var detalhe strings.Builder
	for _, pct := range []int32{100, 37} {
		sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = pct, pct
		for _, o := range oponentes {
			var tempos []float64
			vit := map[string]int{}
			var total float64
			meta := 0
			var gb, gopo []golpe
			for i := range lutas {
				l := sm.lutaEntre(sm.blackLutador(1), o.novo(2), i%2 == 0)
				s := float64(l.ms) / 1000
				tempos = append(tempos, s)
				total += s
				if s >= 60 && s <= 120 {
					meta++
				}
				nome := l.vencedor
				if nome != "Black" && !strings.HasPrefix(nome, "empate") {
					nome = "oponente"
				}
				vit[nome]++
				gb, gopo = append(gb, l.golpesA...), append(gopo, l.golpesB...)
			}
			sort.Float64s(tempos)
			rb, ro := resumir(gb, ""), resumir(gopo, "")
			skills, crits, mana := 0, 0, 0
			for _, g := range gb {
				if g.tipo != "físico" {
					skills++
					if g.crit && g.dano > 0 {
						crits++
					}
					mana += g.extra
				}
			}
			fmt.Fprintf(&b, "| %d%% | %s | Black %d, oponente %d, empate %d | %.0f s | %.0f s | %d | %.0f | %.0f | %d | %d | %d%% | %.0f |\n",
				pct, o.nome, vit["Black"], vit["oponente"], vit["empate (15 min)"], tempos[0], tempos[lutas/2], meta,
				float64(rb.soma)/total, float64(ro.soma)/total, rb.max, ro.max, div(100*crits, skills), float64(mana)/total)
			if pct == 37 && o.nome == "Porradeiro Trans, Éden" {
				tabelaBlack(&detalhe, "Black no Porradeiro Trans (37%)", gb)
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

func tabelaBlack(b *strings.Builder, titulo string, gs []golpe) {
	fmt.Fprintf(b, "**%s, somando as lutas**\n\n| Golpe | Usos | Acertos | Críticos | Dano médio | Maior |\n|---|---|---|---|---|---|\n", titulo)
	for _, sk := range append([]int{0}, ordemDaBlack...) {
		tipo := "físico"
		if sk != 0 {
			tipo = nomeSkillBlack[sk]
		}
		r := resumir(gs, tipo)
		if r.n == 0 {
			continue
		}
		fmt.Fprintf(b, "| %s | %d | %d | %d | %d | %d |\n", tipo, r.n, r.acertos, r.crits, div(r.soma, r.acertos), r.max)
	}
	fmt.Fprintln(b)
}
