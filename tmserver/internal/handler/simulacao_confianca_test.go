//go:build simulacao

package handler

// Simulação do TK Confiança (17/09/2026), agora COM POÇÃO: a meta do Marco é
// PvP de 1 a 2 minutos entre personagens equivalentes, com poção.
//
//	go test -tags simulacao -run TestSimulacaoConfianca -v ./tmserver/internal/handler/
//
// A poção é o teto do servidor: a poção sobe o ReqHp e o tick de 1 s fecha a
// barra em no máximo applyCasting (2.000) por segundo, e a trava entre poções é
// de 100 ms. Quem tem poção boa e auto-poção cura, então, até 2.000 por segundo
// enquanto não está com a vida cheia — é o que a simulação faz, dos dois lados.
//
// O Paladino é HIPOTÉTICO: ninguém joga TK Confiança ainda. Ele tem os mesmos
// pontos de atributo que o Porradeiro (3.700) postos em DES e INT, a mesma
// defesa e o mesmo crítico, e é Mortal para lutar com as duas referências sem a
// Defesa de Evolução no meio. A lança Mortal vale 140%, o mesmo que o cajado com
// martelo do Arch.

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
	simLancaTriunfo = 855
	simPocaoS       = 1
	simAuraS        = auraConfiancaPeriodo
)

type fichaConfianca struct {
	nome                 string
	str, dex, intel, con int16
	hp                   int32
	ataque, defesa       int32
	criticoPct10         int
	special              [4]int16
}

var paladino = fichaConfianca{nome: "Paladino (TK Confiança, hipotético)", str: 400, dex: 2000, intel: 1000, con: 300,
	hp: 15_500, ataque: 2500, defesa: 2414, criticoPct10: 316, special: [4]int16{100, 255, 50, 50}}

// montarConfianca monta o Paladino: janela como montar, mais a INT, a lança +11 e
// as oito skills da Confiança. O MaxHP é posto para a vida máxima efetiva bater
// com a ficha depois da CON do Destino.
func (sm *simulador) montarConfianca(f fichaConfianca, id int) *world.Entity {
	e := &world.Entity{ID: id, Class: 0, ClassMaster: classMasterMortal, Level: 399,
		Str: f.str, BaseStr: f.str, Dex: f.dex, BaseDex: f.dex, Int: f.intel, BaseInt: f.intel, Con: f.con, BaseCon: f.con,
		Special: f.special, BaseSpecial: f.special, LearnedSkill: 0xFF}
	e.Equip[weaponSlotR] = world.Item{Index: simLancaTriunfo, Effects: [3]world.Effect{{Effect: simEfSanc, Value: simRefino11}}}
	sm.d.applyAffectScore(e)
	e.MaxHP = (f.hp - e.AffMaxHP) / 2
	e.HP = f.hp
	e.AC = f.defesa - e.AffAC
	e.Critical = uint8(max(0, min(f.criticoPct10/4-int(skillCriticalBonus(e)), 255)))
	lo, hi := int32(0), int32(200_000)
	for lo < hi {
		mid := (lo + hi) / 2
		e.Damage = mid
		if sm.d.effectiveDamage(e) < f.ataque {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	e.Damage = lo
	return e
}

var nomeSkillConfianca = map[int]string{
	skillDestino: "Destino", skillFanatismo: "Fanatismo", skillGolpeDuplo: "Golpe Duplo", skillGiroDaFuria: "Giro da Fúria",
}

func (sm *simulador) skillConfianca(l *lado, alvo *world.Entity, skillnum int) golpe {
	e := l.e
	cast, _ := sm.cast(l, alvo, skillnum)
	dmg := sm.d.resolveSkillHit(sm.w, e, alvo, alvo.ID, skillnum, cast)
	if dmg > 0 {
		miss := combat.ResolveParry(sm.w.Rand(), skillnum, sm.d.skillParryRate(e, alvo), alvo.Rsv&world.RsvBlock != 0)
		if miss = capMissStreak(e, alvo.ID, miss, int(sm.d.combatRules.MaxMissStreak)); miss != 0 {
			dmg = miss
		}
	}
	if skillnum == skillFanatismo && dmg > 0 {
		// O debuff chama refreshScore; a janela volta e o score dos afetos é refeito
		// em cima dela, para o −15% valer sobre a defesa da janela.
		fixo := fotografar(alvo)
		sm.d.aplicarDebuffDoFanatismo(sm.w, e, alvo, alvo.ID)
		fixo.restaurar(alvo)
		sm.d.applyAffectScore(alvo)
	}
	return golpe{tipo: nomeSkillConfianca[skillnum], dano: sm.aplicar(l, alvo, dmg, 0, true)}
}

// acaoConfianca: Destino, Fanatismo, Golpe Duplo e Giro da Fúria pela recarga do
// SkillData; senão golpe físico.
func (sm *simulador) acaoConfianca(l *lado, alvo *world.Entity, agora int64) golpe {
	for _, sk := range []int{skillDestino, skillFanatismo, skillGolpeDuplo, skillGiroDaFuria} {
		if agora < l.cd[sk] {
			continue
		}
		sp, _ := sm.d.spells.Get(sk)
		l.cd[sk] = agora + max(int64(sp.Delay)*1000, simPasso)
		return sm.skillConfianca(l, alvo, sk)
	}
	return sm.fisico(l, alvo)
}

// lutador é um lado da luta com o que a simulação precisa além do combate.
type lutador struct {
	*lado
	nome  string
	maxHP int32
	aura  bool
	acao  func(*lado, *world.Entity, int64) golpe
	cura  int64 // HP curado por poção e Aura
}

func (sm *simulador) htLutador(id int) *lutador {
	return &lutador{lado: &lado{e: sm.montar(xorimpas, id, buffsHT()), cd: map[int]int64{}}, nome: "HT", maxHP: xorimpas.hp, acao: sm.acaoHT}
}

func (sm *simulador) tkLutador(id int) *lutador {
	l := &lutador{lado: &lado{e: sm.montar(porradeiro, id, nil), cd: map[int]int64{}}, nome: "TK", maxHP: porradeiro.hp}
	l.acao = func(ld *lado, alvo *world.Entity, _ int64) golpe { return sm.fisico(ld, alvo) }
	return l
}

// auraDaVida é o buff ativo do Paladino: nível 255, o cast com a maestria cheia.
var auraDaVida = world.Affect{Type: affectAuraDaVida, Value: 75, Level: 255, Time: 5000}

func (sm *simulador) paladinoLutador(id int) *lutador {
	e := sm.montarConfianca(paladino, id)
	e.Affect[0] = auraDaVida
	return &lutador{lado: &lado{e: e, cd: map[int]int64{}}, nome: "Paladino", maxHP: paladino.hp, aura: true, acao: sm.acaoConfianca}
}

func (l *lutador) curar(v int32) {
	if l.e.HP <= 0 || v <= 0 {
		return
	}
	novo := min(l.maxHP, l.e.HP+v)
	l.cura += int64(novo - l.e.HP)
	l.e.HP = novo
}

// relogio roda poção (1 s), Aura (5 s) e afetos (8 s) de quem estiver vivo.
func (sm *simulador) relogio(agora int64, lados ...*lutador) {
	for _, l := range lados {
		if agora%(simPocaoS*1000) < simPasso {
			l.curar(applyCasting)
		}
		if l.aura && agora%(simAuraS*1000) < simPasso {
			l.curar(curaDaAuraConfianca(l.e, int(auraDaVida.Level), uint32(agora)+1))
		}
		if agora%(simTickAfetoS*1000) < simPasso {
			sm.tickAfetos(l.e)
		}
	}
}

type lutaConfianca struct {
	vencedor         string
	ms               int64
	vidaA, vidaB     int32
	curaA, curaB     int64
	golpesA, golpesB []golpe
}

func (sm *simulador) lutaEntre(a, b *lutador, aPrimeiro bool) lutaConfianca {
	var agora int64
	vez := func(x, y *lutador) {
		if x.e.HP <= 0 || y.e.HP <= 0 {
			return
		}
		g := x.acao(x.lado, y.e, agora)
		x.golpes = append(x.golpes, g)
		if g.dano > 0 {
			marcarPvP(x.e, y.e, uint32(agora)+1)
		}
	}
	for agora < simLimiteMs && a.e.HP > 0 && b.e.HP > 0 {
		if aPrimeiro {
			vez(a, b)
			vez(b, a)
		} else {
			vez(b, a)
			vez(a, b)
		}
		agora += simPasso
		if a.e.HP > 0 && b.e.HP > 0 {
			sm.relogio(agora, a, b)
		}
	}
	l := lutaConfianca{ms: agora, vidaA: a.e.HP, vidaB: b.e.HP, curaA: a.cura, curaB: b.cura, golpesA: a.golpes, golpesB: b.golpes}
	switch {
	case b.e.HP <= 0:
		l.vencedor = a.nome
	case a.e.HP <= 0:
		l.vencedor = b.nome
	default:
		l.vencedor = "empate (15 min)"
	}
	return l
}

func (sm *simulador) lutaContraLugefer(a *lutador, vida int32, danoX10, defesaX10 int) lutaConfianca {
	mob := sm.lugefer(2000, vida, danoX10, defesaX10)
	var agora, proxMob int64
	for agora < simLimiteMs && a.e.HP > 0 && mob.HP > 0 {
		a.golpes = append(a.golpes, a.acao(a.lado, mob, agora))
		if mob.HP <= 0 {
			break
		}
		for proxMob <= agora && a.e.HP > 0 {
			sm.d.tickCount = int(agora / 1000)
			if dmg := sm.d.danoDoGolpeDeMonstro(sm.w, mob, a.e); dmg > 0 {
				a.e.HP = max(0, a.e.HP-int32(dmg))
			}
			proxMob += int64(cadenciaDoGolpe(mob))
		}
		agora += simPasso
		if a.e.HP > 0 {
			sm.relogio(agora, a)
			if agora%(simTickAfetoS*1000) < simPasso {
				sm.tickAfetos(mob)
			}
		}
	}
	l := lutaConfianca{ms: agora, vidaA: a.e.HP, vidaB: mob.HP, curaA: a.cura, golpesA: a.golpes}
	switch {
	case mob.HP <= 0:
		l.vencedor = a.nome
	case a.e.HP <= 0:
		l.vencedor = "Lugefer"
	default:
		l.vencedor = "empate (15 min)"
	}
	sm.w.DespawnMob(mob.ID, 1)
	return l
}

func TestSimulacaoConfianca(t *testing.T) {
	root := filepath.Join("..", "..", "..", "Release")
	sm := novoSimulador(t, root)
	var b strings.Builder
	fmt.Fprintf(&b, "# Simulação do TK Confiança — %d lutas por cenário, com poção\n\n", simRodadas)

	pal, ht, tk := sm.paladinoLutador(1), sm.htLutador(2), sm.tkLutador(3)
	intel, con := atributosDoDestino(pal.e)
	fmt.Fprintf(&b, "## Paladino montado\n\n")
	fmt.Fprintf(&b, "- FOR %d, DES %d, INT %d (+%d do Destino), CON %d (+%d do Destino); Lança do Triunfo +11 (%d%% na Confiança).\n",
		paladino.str, paladino.dex, paladino.intel, intel, paladino.con, con, armaPctConfianca(pal.e, sm.d.itemAbility))
	fmt.Fprintf(&b, "- Ataque físico %d, defesa %d, HP %d; régua de Destreza %d‰: esquiva +%d%%, perfuração %d%% no Fanatismo e no Destino.\n",
		sm.d.effectiveDamage(pal.e), effectiveAC(pal.e), pal.e.HP, parcelaDeDestreza(pal.e), pal.e.AffEsquivaPct, confiancaPerfuracaoPct*parcelaDeDestreza(pal.e)/1000)
	fmt.Fprintf(&b, "- Esquiva contra o físico do Porradeiro %d‰ e da Xorimpas %d‰; contra a skill da Xorimpas %d‰.\n",
		sm.d.parryRate(tk.e, pal.e), sm.d.parryRate(ht.e, pal.e), sm.d.skillParryRate(ht.e, pal.e))
	fmt.Fprintf(&b, "- Aura da Vida a cada 5 s: %d fora de PvP, %d em PvP. Poção: até %d por segundo, para todos.\n\n",
		curaDaAuraConfianca(pal.e, 255, 1_000_000), func() int32 { pal.e.UltimoPvP = 999_999; return curaDaAuraConfianca(pal.e, 255, 1_000_000) }(), applyCasting)

	pvp := []struct {
		nome string
		a, b func(int) *lutador
	}{
		{"Paladino contra Porradeiro (TK físico)", sm.paladinoLutador, sm.tkLutador},
		{"Paladino contra Xorimpas (HT)", sm.paladinoLutador, sm.htLutador},
		{"Referência: Xorimpas (HT) contra Porradeiro (TK), agora com poção", sm.htLutador, sm.tkLutador},
	}
	for _, c := range pvp {
		fmt.Fprintf(&b, "## PvP — %s\n\n| Luta | Vencedor | Tempo (s) | Vida final A | Vida final B | Cura A | Cura B |\n|---|---|---|---|---|---|---|\n", c.nome)
		var tempos []float64
		var golpesA, golpesB []golpe
		var nomeA, nomeB string
		vitorias := map[string]int{}
		for i := range simRodadas {
			a, bb := c.a(1), c.b(2)
			nomeA, nomeB = a.nome, bb.nome
			l := sm.lutaEntre(a, bb, i%2 == 0)
			fmt.Fprintf(&b, "| %d | %s | %.1f | %d | %d | %d | %d |\n", i+1, l.vencedor, float64(l.ms)/1000, l.vidaA, l.vidaB, l.curaA, l.curaB)
			tempos = append(tempos, float64(l.ms)/1000)
			golpesA, golpesB = append(golpesA, l.golpesA...), append(golpesB, l.golpesB...)
			vitorias[l.vencedor]++
		}
		mt, dt := media(tempos)
		fmt.Fprintf(&b, "\nVitórias: %v; tempo %.1f s (±%.1f).\n\n", vitorias, mt, dt)
		tabelaDeGolpes(&b, nomeA, golpesA)
		tabelaDeGolpes(&b, nomeB, golpesB)
	}

	pve := []struct {
		nome               string
		vida               int32
		danoX10, defesaX10 int
	}{
		{"Cav. Lugefer como está", 0, 10, 10},
		{"Cav. Lugefer proposto: 1 milhão de vida, dano ×2, defesa ×1,5", 1_000_000, 20, 15},
	}
	for _, c := range pve {
		fmt.Fprintf(&b, "## PvE — Paladino no %s\n\n| Luta | Vencedor | Tempo (s) | Vida final | Vida do Lugefer | Cura |\n|---|---|---|---|---|---|\n", c.nome)
		var tempos []float64
		var golpes []golpe
		vitorias := map[string]int{}
		for i := range simRodadas {
			l := sm.lutaContraLugefer(sm.paladinoLutador(1), c.vida, c.danoX10, c.defesaX10)
			fmt.Fprintf(&b, "| %d | %s | %.1f | %d | %d | %d |\n", i+1, l.vencedor, float64(l.ms)/1000, l.vidaA, l.vidaB, l.curaA)
			tempos = append(tempos, float64(l.ms)/1000)
			golpes = append(golpes, l.golpesA...)
			vitorias[l.vencedor]++
		}
		mt, dt := media(tempos)
		fmt.Fprintf(&b, "\nVitórias: %v; tempo %.1f s (±%.1f).\n\n", vitorias, mt, dt)
		tabelaDeGolpes(&b, "Paladino", golpes)
	}

	t.Log("\n" + b.String())
	if out := os.Getenv("SIM_OUT"); out != "" {
		if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// tabelaDeGolpes resume os golpes por tipo, sem lista fixa de tipos.
func tabelaDeGolpes(b *strings.Builder, quem string, gs []golpe) {
	tipos := map[string]bool{}
	for _, g := range gs {
		tipos[g.tipo] = true
	}
	ordem := make([]string, 0, len(tipos))
	for tp := range tipos {
		ordem = append(ordem, tp)
	}
	sort.Strings(ordem)
	fmt.Fprintf(b, "**Golpes de %s, somando as lutas**\n\n| Golpe | Usos | Acertos | Críticos | Dano médio | Maior |\n|---|---|---|---|---|---|\n", quem)
	for _, tp := range ordem {
		r := resumir(gs, tp)
		fmt.Fprintf(b, "| %s | %d | %d | %d | %d | %d |\n", tp, r.n, r.acertos, r.crits, div(r.soma, r.acertos), r.max)
	}
	fmt.Fprintln(b)
}

// TestSimulacaoCortePvP mede quanto o dano em jogador precisa cair para as lutas
// chegarem a 1-2 minutos com poção. O corte é o do painel (/rates/combate): %
// de skill e % de físico em jogador, os dois iguais, aplicados depois do ÷4 do
// legado. Nenhum número de produção muda aqui.
//
//	go test -tags simulacao -run TestSimulacaoCortePvP -v ./tmserver/internal/handler/
func TestSimulacaoCortePvP(t *testing.T) {
	root := filepath.Join("..", "..", "..", "Release")
	sm := novoSimulador(t, root)
	var b strings.Builder
	fmt.Fprintf(&b, "# Corte de dano em PvP — %d lutas por célula, com poção\n\n", simRodadas)
	fmt.Fprintf(&b, "Cada célula: tempo médio (s) e vitórias. Empate é luta que passou de 15 min.\n\n")
	fmt.Fprintf(&b, "| %% em jogador | Paladino × Xorimpas (HT) | Paladino × Porradeiro (TK) | Xorimpas × Porradeiro |\n|---|---|---|---|\n")
	confrontos := []struct{ a, b func(int) *lutador }{
		{sm.paladinoLutador, sm.htLutador},
		{sm.paladinoLutador, sm.tkLutador},
		{sm.htLutador, sm.tkLutador},
	}
	for _, pct := range []int32{100, 50, 45, 40, 37, 35, 32, 30, 20} {
		sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = pct, pct
		fmt.Fprintf(&b, "| %d%% |", pct)
		for _, c := range confrontos {
			var tempos []float64
			vitorias := map[string]int{}
			for i := range simRodadas {
				l := sm.lutaEntre(c.a(1), c.b(2), i%2 == 0)
				tempos = append(tempos, float64(l.ms)/1000)
				vitorias[l.vencedor]++
			}
			mt, _ := media(tempos)
			partes := []string{}
			for nome, n := range vitorias {
				partes = append(partes, fmt.Sprintf("%s %d", nome, n))
			}
			sort.Strings(partes)
			fmt.Fprintf(&b, " %.0f s (%s) |", mt, strings.Join(partes, ", "))
		}
		fmt.Fprintln(&b)
	}
	t.Log("\n" + b.String())
	if out := os.Getenv("SIM_OUT"); out != "" {
		if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestSimulacaoCorte37 olha o corte de 37% de perto: 30 lutas por confronto, com
// 36% e 38% ao lado para ver a sensibilidade, e a distribuição dos tempos.
//
//	go test -tags simulacao -run TestSimulacaoCorte37 -v ./tmserver/internal/handler/
func TestSimulacaoCorte37(t *testing.T) {
	const lutas = 30
	root := filepath.Join("..", "..", "..", "Release")
	sm := novoSimulador(t, root)
	var b strings.Builder
	fmt.Fprintf(&b, "# Corte de 37%% em PvP — %d lutas por confronto, com poção\n\n", lutas)
	fmt.Fprintf(&b, "| %% | Confronto | Vitórias | Mínimo | Mediana | Máximo | Lutas de 1-2 min | Mais de 15 min |\n|---|---|---|---|---|---|---|---|\n")
	confrontos := []struct {
		nome string
		a, b func(int) *lutador
	}{
		{"Paladino × Xorimpas", sm.paladinoLutador, sm.htLutador},
		{"Paladino × Porradeiro", sm.paladinoLutador, sm.tkLutador},
		{"Xorimpas × Porradeiro", sm.htLutador, sm.tkLutador},
	}
	for _, pct := range []int32{36, 37, 38} {
		sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = pct, pct
		for _, c := range confrontos {
			var tempos []float64
			vitorias := map[string]int{}
			naMeta, semFim := 0, 0
			for i := range lutas {
				l := sm.lutaEntre(c.a(1), c.b(2), i%2 == 0)
				s := float64(l.ms) / 1000
				tempos = append(tempos, s)
				vitorias[l.vencedor]++
				if s >= 60 && s <= 120 {
					naMeta++
				}
				if l.ms >= simLimiteMs {
					semFim++
				}
			}
			sort.Float64s(tempos)
			partes := []string{}
			for nome, n := range vitorias {
				partes = append(partes, fmt.Sprintf("%s %d", nome, n))
			}
			sort.Strings(partes)
			fmt.Fprintf(&b, "| %d%% | %s | %s | %.0f s | %.0f s | %.0f s | %d | %d |\n", pct, c.nome, strings.Join(partes, ", "),
				tempos[0], tempos[len(tempos)/2], tempos[len(tempos)-1], naMeta, semFim)
		}
	}
	t.Log("\n" + b.String())
	if out := os.Getenv("SIM_OUT"); out != "" {
		if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
