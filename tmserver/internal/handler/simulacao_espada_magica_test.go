//go:build simulacao

package handler

// Simulação do TK Espada Mágica (17/09/2026): o TK-MAGO do print do Marco, com as
// regras de arvore_espada_magica.go, contra a Xorimpas (uma 8ª de cada vez), o
// Porradeiro de hoje, o Porradeiro Trans com Éden e o Paladino, com poção.
//
//	go test -tags simulacao -run TestSimulacaoEspadaMagica -v ./tmserver/internal/handler/
//
// O TK-MAGO é Mortal: FOR 12, INT 2.848, DES 12, CON 513, HP 21.437, MP 22.622,
// Ataque 886, Defesa 2.393, crítico 22,8%, Espada Mágica 255, lança +11. A Magia
// não aparece na janela: é calibrada para o Atq Mágico (12.663) bater com a
// Tempestade de Gelo pela conta do cliente, que não conhece a lança de 140%.
//
// Ele entra com o Possuído e o Samaritano ativos (+500 de CON cada). A mana conta:
// cada skill gasta a sua, a poção de mana devolve até 2.000 por segundo, e o
// Exterminar só sai com a barra cheia.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/combat"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const (
	simTkMagoHP       = 21_437
	simTkMagoMP       = 22_622
	simTkMagoAtqMagic = 12_663
)

var nomeSkillEspada = map[int]string{
	17: "Espada Flamejante", 18: "Contra-Ataque", 19: "Lâmina Congelada", 20: "Ataque da Alma",
	21: "Punhalada Venenosa", 22: "Exterminar", 23: "Tempestade de Gelo",
}

// ordemDoMago: a 8ª e o Exterminar primeiro, depois do maior valor para o menor.
var ordemDoMago = []int{23, 22, 21, 20, 19, 18, 17}

func (sm *simulador) casterDoCliente(e *world.Entity) combat.SkillCaster {
	return combat.SkillCaster{
		Class: int(e.Class), Level: int(e.Level), Str: int(effectiveStr(e)), Int: int(effectiveInt(e)),
		Magic: int(effectiveMagic(e)), Special: effectiveSpecial(e, 3),
		DamageMultiPct: sm.d.spellDamageMultiPct(e) + int(e.DanoMagicoPct),
		Mortal:         e.ClassMaster == classMasterMortal, LearnedSkill: e.LearnedSkill,
	}
}

func (sm *simulador) montarTkMago(id int, buffs bool) *world.Entity {
	e := &world.Entity{ID: id, Class: 0, ClassMaster: classMasterMortal, Level: 399,
		Str: 12, BaseStr: 12, Dex: 12, BaseDex: 12, Int: 2848, BaseInt: 2848, Con: 513, BaseCon: 513,
		MP: simTkMagoMP, MaxMP: simTkMagoMP / 2, LearnedSkill: 0xFF0000}
	e.Special[3], e.BaseSpecial[3] = 255, 255
	e.Equip[weaponSlotR] = world.Item{Index: simLancaTriunfo, Effects: [3]world.Effect{{Effect: simEfSanc, Value: simRefino11}}}
	sm.d.applyAffectScore(e)
	e.MaxHP = (simTkMagoHP - e.AffMaxHP) / 2
	e.AC = 2393 - e.AffAC
	e.Critical = uint8(228 / 4)
	lo, hi := int32(0), int32(200_000)
	for lo < hi {
		mid := (lo + hi) / 2
		e.Damage = mid
		if sm.d.effectiveDamage(e) < 886 {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	e.Damage = lo
	sp, _ := sm.d.spells.Get(skillTempestadeDeGelo)
	spell := combat.SkillSpell{InstanceType: sp.InstanceType, InstanceValue: sp.InstanceValue, AffectValue: sp.AffectValue}
	for m := int16(0); m <= maxMagic; m++ {
		e.Magic = m
		if combat.SkillBaseDamage(skillTempestadeDeGelo, spell, sm.casterDoCliente(e), 0, int(sm.d.weaponDamage(e))) >= simTkMagoAtqMagic {
			break
		}
	}
	if buffs {
		e.Affect[0] = world.Affect{Type: 14, Time: 5000}
		e.Affect[1] = world.Affect{Type: affectSamaritano, Time: 5000}
		sm.d.applyAffectScore(e)
	}
	e.HP = effectiveMaxHP(e)
	return e
}

func (sm *simulador) tkMagoLutador(id int) *lutador {
	e := sm.montarTkMago(id, true)
	l := &lutador{lado: &lado{e: e, cd: map[int]int64{}}, nome: "TK-MAGO", maxHP: e.HP}
	var ultimo int64
	l.acao = func(ld *lado, alvo *world.Entity, agora int64) golpe {
		ld.e.MP = min(simTkMagoMP, ld.e.MP+int32((agora-ultimo)*applyCasting/1000))
		ultimo = agora
		for _, sk := range ordemDoMago {
			if agora < ld.cd[sk] {
				continue
			}
			sp, _ := sm.d.spells.Get(sk)
			custo := int32(combat.ManaSpent(sp.ManaSpent, 0, 255))
			if sk == skillExterminar && ld.e.MP < simTkMagoMP || ld.e.MP < custo {
				continue
			}
			ld.e.MP -= custo
			ld.cd[sk] = agora + max(int64(sp.Delay)*1000, simPasso)
			return sm.skillDoMago(l, alvo, sk, sp)
		}
		return sm.fisico(ld, alvo)
	}
	return l
}

// skillDoMago segue a ordem do handler (combat.go): dano da skill, mana do
// Exterminar, crítico da árvore, acerto (10% no Exterminar, esquiva nas outras),
// o bloco PvP e, sobre o que entrou, o roubo de vida.
func (sm *simulador) skillDoMago(l *lutador, alvo *world.Entity, sk int, sp content.Spell) golpe {
	e := l.e
	cast, _ := sm.cast(l.lado, alvo, sk)
	dmg := sm.d.resolveSkillHit(sm.w, e, alvo, alvo.ID, sk, cast)
	if sk == skillExterminar {
		dmg += int(e.MP) + int(effectiveInt(e))/2
		e.MP = 0
	}
	crit := false
	if dmg > 0 {
		if mult := rolarCriticoEspadaMagica(sm.w.Rand(), e); mult > 0 {
			dmg, crit = dmg*mult/10, true
		}
	}
	if dmg > 0 {
		if sk == skillExterminar {
			if !exterminarAcerta(sm.w.Rand()) {
				dmg = -3
			}
		} else {
			miss := combat.ResolveParry(sm.w.Rand(), sk, sm.d.skillParryRate(e, alvo), alvo.Rsv&world.RsvBlock != 0)
			if miss = capMissStreak(e, alvo.ID, miss, int(sm.d.combatRules.MaxMissStreak)); miss != 0 {
				dmg = miss
			}
		}
	}
	final := sm.aplicar(l.lado, alvo, dmg, 0, true)
	cura := rouboDeVida(sm.w.Rand(), e, final)
	antes := e.HP
	l.curar(cura)
	return golpe{tipo: nomeSkillEspada[sk], dano: final, crit: crit, extra: int(e.HP - antes)}
}

func TestSimulacaoEspadaMagica(t *testing.T) {
	const lutas = 30
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	var b strings.Builder
	fmt.Fprintf(&b, "# TK Espada Mágica (TK-MAGO) — %d lutas por linha, com poção\n\n", lutas)

	sem, com := sm.montarTkMago(1, false), sm.montarTkMago(1, true)
	sp, _ := sm.d.spells.Get(skillTempestadeDeGelo)
	spell := combat.SkillSpell{InstanceType: sp.InstanceType, InstanceValue: sp.InstanceValue, AffectValue: sp.AffectValue}
	cliente := combat.SkillBaseDamage(skillTempestadeDeGelo, spell, sm.casterDoCliente(com), 0, int(sm.d.weaponDamage(com)))
	cast := sm.casterDoCliente(com)
	cast.ArmaPct = armaPctEspadaMagica(com, sm.d.itemAbility)
	servidor := combat.SkillBaseDamage(skillTempestadeDeGelo, spell, cast, 0, int(sm.d.weaponDamage(com)))
	fmt.Fprintf(&b, "## TK-MAGO montado\n\n- Magia calibrada: %d. Tempestade de Gelo: %d na conta do cliente, %d com a lança de 140%%.\n", com.Magic, cliente, servidor)
	fmt.Fprintf(&b, "- HP %d sem buffs, %d com Possuído e Samaritano (CON %d → %d).\n", effectiveMaxHP(sem), effectiveMaxHP(com), sem.Con+sem.AffCon, com.Con+com.AffCon)
	fmt.Fprintf(&b, "- Régua de INT: %d/1000 → crítico %d%%, ×2,0 a ×%.1f; roubo de vida %d%%.\n\n",
		reguaDeInt(com), espadaCritChanceBase+espadaCritChanceInt*reguaDeInt(com)/1000,
		float64(espadaCritMultBase10+espadaCritMultInt10*reguaDeInt(com)/1000)/10, rouboDeVidaChance*reguaDeInt(com)/1000)

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
	}
	fmt.Fprintf(&b, "| %% PvP | Oponente | Vitórias | Mínimo | Mediana | 1-2 min | Dano/s TK-MAGO | Dano/s oponente | Maior golpe TK-MAGO | Maior golpe oponente | Críticos nas skills | Exterminar (acertos/usos) | Roubo de vida/s |\n|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	var detalhe strings.Builder
	for _, pct := range []int32{100, 37} {
		sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = pct, pct
		for _, o := range oponentes {
			var tempos []float64
			vit := map[string]int{}
			var total float64
			meta := 0
			var gm, go_ []golpe
			for i := range lutas {
				l := sm.lutaEntre(sm.tkMagoLutador(1), o.novo(2), i%2 == 0)
				s := float64(l.ms) / 1000
				tempos = append(tempos, s)
				total += s
				if s >= 60 && s <= 120 {
					meta++
				}
				nome := l.vencedor
				if nome != "TK-MAGO" && !strings.HasPrefix(nome, "empate") {
					nome = "oponente"
				}
				vit[nome]++
				gm, go_ = append(gm, l.golpesA...), append(go_, l.golpesB...)
			}
			sort.Float64s(tempos)
			rm, ro := resumir(gm, ""), resumir(go_, "")
			skills, crits, roubo := 0, 0, 0
			for _, g := range gm {
				if g.tipo != "físico" {
					skills++
					if g.crit && g.dano > 0 {
						crits++
					}
					roubo += g.extra
				}
			}
			ex := resumir(gm, "Exterminar")
			fmt.Fprintf(&b, "| %d%% | %s | TK-MAGO %d, oponente %d, empate %d | %.0f s | %.0f s | %d | %.0f | %.0f | %d | %d | %d%% | %d/%d | %.0f |\n",
				pct, o.nome, vit["TK-MAGO"], vit["oponente"], vit["empate (15 min)"], tempos[0], tempos[lutas/2], meta,
				float64(rm.soma)/total, float64(ro.soma)/total, rm.max, ro.max, div(100*crits, skills), ex.acertos, ex.n, float64(roubo)/total)
			if pct == 37 && o.nome == "Xorimpas, 8ª Captura" {
				tabelaMago(&detalhe, "TK-MAGO na Xorimpas da Captura (37%)", gm)
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

func tabelaMago(b *strings.Builder, titulo string, gs []golpe) {
	fmt.Fprintf(b, "**%s, somando as lutas**\n\n| Golpe | Usos | Acertos | Críticos | Dano médio | Maior |\n|---|---|---|---|---|---|\n", titulo)
	for _, sk := range append([]int{0}, ordemDoMago...) {
		tipo := "físico"
		if sk != 0 {
			tipo = nomeSkillEspada[sk]
		}
		r := resumir(gs, tipo)
		if r.n == 0 {
			continue
		}
		fmt.Fprintf(b, "| %s | %d | %d | %d | %d | %d |\n", tipo, r.n, r.acertos, r.crits, div(r.soma, r.acertos), r.max)
	}
	fmt.Fprintln(b)
}
