//go:build simulacao

package handler

// Simulação de combate da Huntress (17/09/2026): 10 lutas contra o Cav. Lugefer
// e 10 contra um TK físico, com as funções de combate do servidor chamadas na
// mesma ordem do handler de ataque (combat.go) e do golpe de monstro (mobai.go).
//
//	go test -tags simulacao -run TestSimulacaoHuntress -v ./tmserver/internal/handler/
//
// SIM_OUT=<arquivo> grava o relatório em Markdown. A meta e o resultado de
// 17/09 estão em docs/balanceamento/simulacao-ht-2026-09-17.md.

import (
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/combatrule"
	"github.com/jeanluca/w2pp-openwyd/internal/npctemplate"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/combat"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const (
	simPasso      = 800     // ms: a trava de cadência do servidor (attackCadence)
	simLimiteMs   = 900_000 // 15 min: luta sem fim é empate
	simRodadas    = 10
	simGarra      = 841 // Khyrius, garra do catálogo
	simRefino11   = 234 // EF_SANC +11 (RefineOf: 230 + 4)
	simEfSanc     = 43
	simTickAfetoS = 8
)

// janela são os números da janela C de um personagem.
type janela struct {
	nome           string
	classe         uint8
	str, dex, con  int16
	hp             int32
	ataque, defesa int32
	criticoPct10   int // crítico da janela × 10 (54,0% → 540)
	special        [4]int16
	learned        int32
}

var xorimpas = janela{nome: "Xorimpas (HT)", classe: 3, str: 2172, dex: 700, con: 512, hp: 7526,
	ataque: 8111, defesa: 2348, criticoPct10: 540, special: [4]int16{174, 278, 224, 223}, learned: 0xFFFFFF}

var porradeiro = janela{nome: "Porradeiro (TK)", classe: 0, str: 2802, dex: 712, con: 189, hp: 14277,
	ataque: 5515, defesa: 2414, criticoPct10: 316}

type simulador struct {
	t     *testing.T
	d     *Dispatcher
	w     *world.World
	tick  int64
	nextM map[int]int64 // próximo golpe do monstro (ms)
	// garnet é a absorção da Garnet por personagem, ainda só da simulação
	// (simulacao_garnet_test.go); vazio, nada muda.
	garnet map[int]int
	// semPocao: até quando cada personagem está impedido de beber poção pelo
	// Cancelamento da FM (regra em estudo, ainda NÃO existe no servidor).
	semPocao map[int]int64
}

func novoSimulador(t *testing.T, root string) *simulador {
	items, err := content.LoadItemList(filepath.Join(root, "Common", "ItemList.csv"))
	if err != nil {
		t.Skipf("ItemList: %v", err)
	}
	spells, err := content.LoadSkillData(filepath.Join(root, "Common", "SkillData.csv"))
	if err != nil {
		t.Skipf("SkillData: %v", err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	regras := combatrule.Default()
	d := New(Config{Log: log, Spells: spells, ItemEffects: items.BaseEffects(), CombatRules: &regras})
	w := world.New(world.Config{GridDim: 64}, log, nil, nil)
	return &simulador{t: t, d: d, w: w, nextM: map[int]int64{}, garnet: map[int]int{}}
}

// montar cria o personagem com os números da janela: o Ataque, a Defesa e o
// crítico são calibrados DEPOIS dos afetos, para as passivas não contarem duas
// vezes (a janela já as mostra).
func (sm *simulador) montar(j janela, id int, afetos []world.Affect) *world.Entity {
	e := &world.Entity{ID: id, Class: j.classe, ClassMaster: classMasterMortal, Level: 399,
		Str: j.str, BaseStr: j.str, Dex: j.dex, BaseDex: j.dex, Con: j.con, BaseCon: j.con,
		HP: j.hp, MaxHP: j.hp, Special: j.special, BaseSpecial: j.special, LearnedSkill: j.learned}
	if j.classe == 3 {
		e.Equip[weaponSlotR] = world.Item{Index: simGarra, Effects: [3]world.Effect{{Effect: simEfSanc, Value: simRefino11}}}
	}
	for i, af := range afetos {
		e.Affect[i] = af
	}
	sm.d.applyAffectScore(e)
	e.AC = j.defesa - e.AffAC
	crit := j.criticoPct10/4 - int(skillCriticalBonus(e)) // janela = byte × 0,4%
	e.Critical = uint8(max(0, min(crit, 255)))
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
	return e
}

func (sm *simulador) lugefer(id int, vida int32, danoX10, defesaX10 int) *world.Entity {
	root := filepath.Join("..", "..", "..", "Release")
	tmpl, _, err := npctemplate.Load(root, "Cav._Lugefer")
	if err != nil {
		sm.t.Skipf("Cav._Lugefer: %v", err)
	}
	mid := sm.w.SpawnMob(tmpl, int16(10+id%20), 10)
	m := sm.w.Entity(mid)
	if vida > 0 {
		m.MaxHP, m.HP = vida, vida
	}
	m.Damage = m.Damage * int32(danoX10) / 10
	m.AC = m.AC * int32(defesaX10) / 10
	return m
}

// resultado de um golpe, para as contas.
type golpe struct {
	tipo  string
	dano  int
	crit  bool
	extra int
}

type lado struct {
	e        *world.Entity
	sess     *world.Session
	progress uint16
	cd       map[int]int64 // skill → pronta em (ms)
	golpes   []golpe
}

func (sm *simulador) cast(l *lado, alvo *world.Entity, skillnum int) (castInfo, bool) {
	sp, ok := sm.d.spells.Get(skillnum)
	if !ok {
		return castInfo{}, false
	}
	return castInfo{isSkill: true, spell: sp, special: effectiveSpecial(l.e, content.SkillKind(skillnum))}, true
}

// aplicar é o trecho comum do handler depois do dano resolvido (combat.go:
// perfuração, regra PvP, força, evolução, stats PvP, mana, montaria, HP, afetos).
func (sm *simulador) aplicar(l *lado, alvo *world.Entity, dmg, airBlade int, skill bool) int {
	if dmg <= 0 {
		return dmg
	}
	pvp := world.IsPlayer(alvo.ID)
	dmg = perfuracao(alvo, alvo.ID, dmg, airBlade)
	if pvp {
		dmg = sm.d.applyPvPRule(dmg, skill)
		dmg = danoDoTransContraHT(l.e, alvo, dmg)
	}
	dmg = applyForceDamage(l.e, alvo, alvo.ID, dmg)
	if pvp {
		dmg = applyTierDefense(l.e.ClassMaster, alvo.ClassMaster, dmg)
		dmg = sm.d.applyPvPStats(l.e, alvo, dmg)
		dmg = sm.absorveGarnet(l.e, alvo, dmg)
	}
	dmg = sm.d.applyManaControl(sm.w, l.e, alvo, alvo.ID, dmg)
	// Sem sessão o applyManaControl não age: o Controle de Mana (46) do alvo entra
	// pela conta pura, com a mana saindo da barra dele.
	if world.IsPlayer(alvo.ID) && sm.w.Session(alvo.ID) == nil {
		if r, _, ok := manaControlDamage(alvo, dmg, l.e.LearnedSkill&(1<<23) != 0); ok {
			dmg = r
		}
	}
	dmg = sm.d.absorbBlow(sm.w, alvo, dmg, true)
	alvo.HP = max(0, alvo.HP-int32(dmg))
	// O afeto que pega chama refreshScore, que remonta o score pelo equipamento — e
	// estes personagens vêm da janela, sem equipamento. Os números da janela voltam
	// depois; os afetos (lentidão, veneno) ficam.
	fixo := fotografar(alvo)
	sm.d.applyOnHitAffects(sm.w, l.e, alvo, alvo.ID)
	fixo.restaurar(alvo)
	return dmg
}

type fotografia struct {
	damage, ac, maxHP, hp int32
	critical              uint8
	str, dex, con         int16
	special               [4]int16
	parry                 int
	equipForce            int32
}

func fotografar(e *world.Entity) fotografia {
	return fotografia{e.Damage, e.AC, e.MaxHP, e.HP, e.Critical, e.Str, e.Dex, e.Con, e.Special, e.Parry, e.EquipForceDamage}
}

func (f fotografia) restaurar(e *world.Entity) {
	e.Damage, e.AC, e.MaxHP, e.HP, e.Critical = f.damage, f.ac, f.maxHP, f.hp, f.critical
	e.Str, e.Dex, e.Con, e.Special, e.Parry = f.str, f.dex, f.con, f.special, f.parry
	e.EquipForceDamage = f.equipForce
}

func (sm *simulador) fisico(l *lado, alvo *world.Entity) golpe {
	e := l.e
	r := sm.w.Rand()
	var serverProg uint16
	dc, _ := combat.DoubleCritical(r, attackRunOf(e), int(effectiveCritical(e)),
		int(sm.d.combatRules.DoubleCriticalMaxPct), &serverProg, &l.progress)
	dmg := combat.ResolveHit(r, combat.HitInput{
		AttackerDamage: int(sm.d.effectiveDamage(e)), TargetAC: sm.d.defesaPerfurada(e, int(effectiveAC(alvo))),
		TargetIsPlayer: world.IsPlayer(alvo.ID), AttackerIsPlayer: true, DoubleCritical: dc,
		Master: masterDoGolpe(e), SkillIndex: -1, ParryRate: sm.d.parryRate(e, alvo),
		TargetRsvBlock: alvo.Rsv&world.RsvBlock != 0,
	})
	body := &protocol.MsgAttackBody{}
	payload := make([]byte, protocol.MsgAttackDamOffset)
	air := 0
	if dmg > 0 {
		dmg, air = sm.d.applyAirBladeProc(sm.w, e, alvo, protocol.MsgAttackTwo, body, payload, dmg)
	}
	final := sm.aplicar(l, alvo, dmg, air, false)
	return golpe{tipo: "físico", dano: final, crit: dc != 0, extra: air}
}

func (sm *simulador) skill(l *lado, alvo *world.Entity, skillnum int) golpe {
	e := l.e
	cast, _ := sm.cast(l, alvo, skillnum)
	dmg := sm.d.resolveSkillHit(sm.w, e, alvo, alvo.ID, skillnum, cast)
	crit := false
	if skillnum == skillGolpeFelino && dmg > 0 {
		if mult := rolarCriticoGolpeFelino(sm.w.Rand(), int(effectiveStr(e)), int(effectiveDex(e))); mult > 0 {
			dmg, crit = dmg*mult/10, true
		}
	}
	if skillnum == skillLaminaDasSombras && dmg > 0 {
		dmg, crit = danoLaminaDasSombras(sm.w.Rand(), e, dmg)
	}
	if dmg > 0 {
		miss := combat.ResolveParry(sm.w.Rand(), skillnum, sm.d.skillParryRate(e, alvo), alvo.Rsv&world.RsvBlock != 0)
		if miss = capMissStreak(e, alvo.ID, miss, int(sm.d.combatRules.MaxMissStreak)); miss != 0 {
			dmg = miss
		}
	}
	nome := map[int]string{skillTempestadeDeFlechas: "Tempestade", skillGolpeFelino: "Golpe Felino", skillLaminaDasSombras: "Lâmina das Sombras"}[skillnum]
	return golpe{tipo: nome, dano: sm.aplicar(l, alvo, dmg, 0, true), crit: crit}
}

// acaoHT: Tempestade quando pronta, depois Golpe Felino, depois Lâmina das
// Sombras; senão golpe físico. Recargas pelo Delay do SkillData (segundos), a
// Tempestade pelos 40 s do servidor.
func (sm *simulador) acaoHT(l *lado, alvo *world.Entity, agora int64) golpe {
	for _, sk := range []int{skillTempestadeDeFlechas, skillGolpeFelino, skillLaminaDasSombras} {
		if agora < l.cd[sk] {
			continue
		}
		sp, _ := sm.d.spells.Get(sk)
		espera := int64(sp.Delay) * 1000
		if sk == skillTempestadeDeFlechas {
			espera = tempestadeRecargaMs
		}
		l.cd[sk] = agora + max(espera, simPasso)
		return sm.skill(l, alvo, sk)
	}
	return sm.fisico(l, alvo)
}

// tickVeneno roda a cada 8 s: monstro pelo processMobAffect do servidor, jogador
// pelos −1000 do legado (applyPoisonTick, sem a sessão).
func (sm *simulador) tickAfetos(e *world.Entity) int32 {
	antes := e.HP
	if !world.IsPlayer(e.ID) {
		fixo := fotografar(e)
		sm.d.processMobAffect(sm.w, e.ID, e)
		fixo.hp = e.HP
		fixo.restaurar(e)
		return antes - e.HP
	}
	for i := range e.Affect {
		af := &e.Affect[i]
		if af.Type == affectPoison {
			e.HP = max(1, e.HP-poisonTickDamage)
		}
		if af.Type != 0 && af.Time > 0 && af.Time < affectInfiniteTime {
			if af.Time--; af.Time == 0 {
				*af = world.Affect{}
				fixo := fotografar(e)
				sm.d.refreshScore(e)
				fixo.restaurar(e)
			}
		}
	}
	return antes - e.HP
}

type luta struct {
	vencedor   string
	ms         int64
	vidaFinalA int32
	vidaFinalB int32
	golpesA    []golpe
	golpesB    []golpe
	veneno     int32
}

func buffsHT() []world.Affect {
	const longo = 5000
	return []world.Affect{
		{Type: affectEvasao, Value: 1, Level: 223, Time: longo},
		{Type: 27, Value: 1, Level: 278, Time: longo}, // Encantar Gelo
		{Type: 36, Value: 1, Level: 223, Time: longo}, // Toxina de Serpente
	}
}

func (sm *simulador) lutaPvE(vida int32, danoX10, defesaX10 int) luta {
	ht := &lado{e: sm.montar(xorimpas, 1, buffsHT()), cd: map[int]int64{}}
	mob := sm.lugefer(2000, vida, danoX10, defesaX10)
	var agora int64
	var veneno int32
	proxMob := int64(0)
	for agora < simLimiteMs && ht.e.HP > 0 && mob.HP > 0 {
		ht.golpes = append(ht.golpes, sm.acaoHT(ht, mob, agora))
		if mob.HP <= 0 {
			break
		}
		for proxMob <= agora && ht.e.HP > 0 {
			sm.d.tickCount = int(agora / 1000)
			dmg := sm.d.danoDoGolpeDeMonstro(sm.w, mob, ht.e)
			if dmg > 0 {
				ht.e.HP = max(0, ht.e.HP-int32(dmg))
			}
			proxMob += int64(cadenciaDoGolpe(mob))
		}
		agora += simPasso
		if agora%(simTickAfetoS*1000) < simPasso {
			veneno += sm.tickAfetos(mob)
		}
	}
	l := luta{ms: agora, vidaFinalA: ht.e.HP, vidaFinalB: mob.HP, golpesA: ht.golpes, veneno: veneno}
	switch {
	case mob.HP <= 0:
		l.vencedor = "HT"
	case ht.e.HP <= 0:
		l.vencedor = "Lugefer"
	default:
		l.vencedor = "empate"
	}
	sm.w.DespawnMob(mob.ID, 1)
	return l
}

func (sm *simulador) lutaPvP(htPrimeiro bool) luta {
	ht := &lado{e: sm.montar(xorimpas, 1, buffsHT()), cd: map[int]int64{}}
	tk := &lado{e: sm.montar(porradeiro, 2, nil), cd: map[int]int64{}}
	var agora int64
	var veneno int32
	for agora < simLimiteMs && ht.e.HP > 0 && tk.e.HP > 0 {
		if htPrimeiro {
			ht.golpes = append(ht.golpes, sm.acaoHT(ht, tk.e, agora))
			if tk.e.HP > 0 {
				tk.golpes = append(tk.golpes, sm.fisico(tk, ht.e))
			}
		} else {
			tk.golpes = append(tk.golpes, sm.fisico(tk, ht.e))
			if ht.e.HP > 0 {
				ht.golpes = append(ht.golpes, sm.acaoHT(ht, tk.e, agora))
			}
		}
		agora += simPasso
		if agora%(simTickAfetoS*1000) < simPasso {
			veneno += sm.tickAfetos(tk.e)
		}
	}
	l := luta{ms: agora, vidaFinalA: ht.e.HP, vidaFinalB: tk.e.HP, golpesA: ht.golpes, golpesB: tk.golpes, veneno: veneno}
	switch {
	case tk.e.HP <= 0:
		l.vencedor = "HT"
	case ht.e.HP <= 0:
		l.vencedor = "TK"
	default:
		l.vencedor = "empate"
	}
	return l
}

type resumoGolpes struct {
	n, acertos, crits, extras int
	soma, somaExtra           int
	max                       int
}

func resumir(gs []golpe, tipo string) resumoGolpes {
	var r resumoGolpes
	for _, g := range gs {
		if tipo != "" && g.tipo != tipo {
			continue
		}
		r.n++
		if g.dano > 0 {
			r.acertos++
			r.soma += g.dano
			r.max = max(r.max, g.dano)
		}
		if g.crit && g.dano > 0 {
			r.crits++
		}
		if g.extra > 0 {
			r.extras++
			r.somaExtra += g.extra
		}
	}
	return r
}

func media(v []float64) (float64, float64) {
	if len(v) == 0 {
		return 0, 0
	}
	var s, q float64
	for _, x := range v {
		s += x
	}
	m := s / float64(len(v))
	for _, x := range v {
		q += (x - m) * (x - m)
	}
	return m, math.Sqrt(q / float64(len(v)))
}

func TestSimulacaoHuntress(t *testing.T) {
	root := filepath.Join("..", "..", "..", "Release")
	var b strings.Builder
	fmt.Fprintf(&b, "# Simulação da Huntress — %d lutas por cenário\n\n", simRodadas)

	sm := novoSimulador(t, root)
	ht := sm.montar(xorimpas, 1, buffsHT())
	tk := sm.montar(porradeiro, 2, nil)
	mob := sm.lugefer(2000, 0, 10, 10)
	fmt.Fprintf(&b, "## Personagens montados\n\n| | Ataque | Defesa | HP | Crítico (byte) | Esquiva contra o outro |\n|---|---|---|---|---|---|\n")
	fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %d‰ vs TK, %d‰ vs Lugefer |\n", xorimpas.nome, sm.d.effectiveDamage(ht), effectiveAC(ht), ht.HP, effectiveCritical(ht), sm.d.parryRate(tk, ht), sm.d.monsterParryRate(mob, ht))
	fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %d‰ vs HT |\n", porradeiro.nome, sm.d.effectiveDamage(tk), effectiveAC(tk), tk.HP, effectiveCritical(tk), sm.d.parryRate(ht, tk))
	fmt.Fprintf(&b, "| Cav. Lugefer (arquivo) | %d | %d | %d | — | %d‰ vs HT |\n\n", mob.Damage, effectiveAC(mob), mob.MaxHP, sm.d.parryRate(ht, mob))
	fmt.Fprintf(&b, "HT: esquiva da Captura +%d%%, perfuração da Lança de Ferro %d%%.\n\n", ht.AffEsquivaPct, perfuracaoLancaDeFerro(ht))
	sm.w.DespawnMob(mob.ID, 1)

	cenariosPvE := []struct {
		nome               string
		vida               int32
		danoX10, defesaX10 int
	}{
		{"Cav. Lugefer como está", 0, 10, 10},
		{"Cav. Lugefer proposto: 1 milhão de vida, dano ×2, defesa ×1,5", 1_000_000, 20, 15},
	}
	for _, c := range cenariosPvE {
		fmt.Fprintf(&b, "## PvE — %s\n\n| Luta | Vencedor | Tempo (s) | Vida final da HT | Golpes | Dano médio físico | Tempestade média | Veneno total |\n|---|---|---|---|---|---|---|---|\n", c.nome)
		var tempos, vidas []float64
		var todos []golpe
		vitorias := 0
		for i := range simRodadas {
			l := sm.lutaPvE(c.vida, c.danoX10, c.defesaX10)
			fis, tem := resumir(l.golpesA, "físico"), resumir(l.golpesA, "Tempestade")
			fmt.Fprintf(&b, "| %d | %s | %.1f | %d | %d | %d | %d | %d |\n", i+1, l.vencedor, float64(l.ms)/1000, l.vidaFinalA, len(l.golpesA), div(fis.soma, fis.acertos), div(tem.soma, tem.acertos), l.veneno)
			todos = append(todos, l.golpesA...)
			tempos = append(tempos, float64(l.ms)/1000)
			vidas = append(vidas, float64(l.vidaFinalA))
			if l.vencedor == "HT" {
				vitorias++
			}
		}
		mt, dt := media(tempos)
		mv, dv := media(vidas)
		fmt.Fprintf(&b, "\nHT venceu %d de %d; tempo %.1f s (±%.1f); vida final %.0f (±%.0f).\n\n", vitorias, simRodadas, mt, dt, mv, dv)
		tabelaPorGolpe(&b, "HT no Lugefer", todos)
	}

	fmt.Fprintf(&b, "## PvP — Xorimpas (HT) contra Porradeiro (TK)\n\n| Luta | Quem abre | Vencedor | Tempo (s) | Vida final HT | Vida final TK | Físico HT (médio) | Lâmina Aérea (vezes/média) | Tempestade HT | Físico TK (médio) | Críticos TK | Esquivas HT |\n|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	var tempos []float64
	var todosHT, todosTK []golpe
	vHT := 0
	for i := range simRodadas {
		l := sm.lutaPvP(i%2 == 0)
		fis, tem, tkf := resumir(l.golpesA, "físico"), resumir(l.golpesA, "Tempestade"), resumir(l.golpesB, "físico")
		quem := "HT"
		if i%2 == 1 {
			quem = "TK"
		}
		esq := tkf.n - tkf.acertos
		fmt.Fprintf(&b, "| %d | %s | %s | %.1f | %d | %d | %d | %d / %d | %d | %d | %d | %d de %d |\n", i+1, quem, l.vencedor, float64(l.ms)/1000,
			l.vidaFinalA, l.vidaFinalB, div(fis.soma, fis.acertos), fis.extras, div(fis.somaExtra, fis.extras), div(tem.soma, tem.acertos),
			div(tkf.soma, tkf.acertos), tkf.crits, esq, tkf.n)
		todosHT, todosTK = append(todosHT, l.golpesA...), append(todosTK, l.golpesB...)
		tempos = append(tempos, float64(l.ms)/1000)
		if l.vencedor == "HT" {
			vHT++
		}
	}
	mt, dt := media(tempos)
	fmt.Fprintf(&b, "\nHT venceu %d de %d; tempo %.1f s (±%.1f).\n\n", vHT, simRodadas, mt, dt)
	tabelaPorGolpe(&b, "HT no TK", todosHT)
	tabelaPorGolpe(&b, "TK na HT", todosTK)

	t.Log("\n" + b.String())
	if out := os.Getenv("SIM_OUT"); out != "" {
		if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func tabelaPorGolpe(b *strings.Builder, titulo string, gs []golpe) {
	fmt.Fprintf(b, "**%s, somando as 10 lutas**\n\n| Golpe | Usos | Acertos | Críticos | Dano médio | Maior | Lâmina Aérea (vezes / média) |\n|---|---|---|---|---|---|---|\n", titulo)
	for _, tipo := range []string{"físico", "Tempestade", "Golpe Felino", "Lâmina das Sombras"} {
		r := resumir(gs, tipo)
		if r.n == 0 {
			continue
		}
		fmt.Fprintf(b, "| %s | %d | %d | %d | %d | %d | %d / %d |\n", tipo, r.n, r.acertos, r.crits, div(r.soma, r.acertos), r.max, r.extras, div(r.somaExtra, r.extras))
	}
	fmt.Fprintln(b)
}

func div(a, b int) int {
	if b == 0 {
		return 0
	}
	return a / b
}
