//go:build simulacao

package handler

// Todos contra todos (17/09/2026): as árvores redesenhadas do TK e da Foema
// contra a Huntress, com poção dos dois lados.
//
//	go test -tags simulacao -run TestSimulacaoTodosContraTodos -v ./tmserver/internal/handler/
//
// As duas Foemas que faltavam entram aqui:
//
//   - FM Cancelamento: a CanceleiVoce do print (INT 2.147, DES 712, CON 512, HP
//     5.993, MP 21.011, Ataque 3.105, Defesa 2.078, crítico 31,6%), com duas
//     espadas de 1 mão e o Controle de Mana ativo.
//   - FM Magia Branca: HIPOTÉTICA, não há referência. CON alta: 30 mil de HP, a
//     defesa da black e cajado de 1 mão. Ela se cura, marca com o Choque Divino e
//     usa o Julgamento Divino quando a vida está cheia.

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

// simCancelVidaPct escala a vida da FM Cancel na varredura de botões: 100 é a
// ficha do print.
var simCancelVidaPct = 100

const (
	simEspadaUmaMao = 1009 // Lâmina Magni, espada de 1 mão
	simCajadoUmaMao = 904  // Neorion
	simBrancaHP     = 30_000
	simBrancaMP     = 10_000
)

// ---------------------------------------------------------------------------
// FM Cancelamento (CanceleiVoce)

// A janela da CanceleiVoce em 18/09/2026, JÁ com as regras da árvore no ar e com
// os buffs dela de pé (print do Marco): a maestria da Magia Especial aparece em
// 366, acima do teto de 255, que é o Toque de Athena valendo em dobro nela.
//
// Esta ficha é o resultado FINAL. A calibração roda com tudo ligado — 8ª, duas
// armas, buffs — e resolve a base para o total cair no número da janela. Somar
// as regras por cima daqui seria contá-las duas vezes.
const (
	cancelAtaque     = 8726
	cancelDefesa     = 2233
	cancelHP         = 5432
	cancelMP         = 17332
	cancelAtqMagico  = 3887
	cancelCritico10  = 316 // décimos de %
	cancelMaestria10 = 366 // Magia Especial na janela, com o buff
)

func (sm *simulador) montarCancel(id int) *world.Entity {
	e := &world.Entity{ID: id, Class: 1, ClassMaster: classMasterMortal, Level: 399,
		Str: 12, BaseStr: 12, Int: 2147, BaseInt: 2147, Dex: 712, BaseDex: 712, Con: 512, BaseCon: 512,
		LearnedSkill: 0xFF0000} // as oito da Magia Especial
	e.Special = [4]int16{0, 0, 0, brancaMaestriaMax}
	e.BaseSpecial = e.Special
	arma := world.Item{Index: simEspadaUmaMao, Effects: [3]world.Effect{{Effect: simEfSanc, Value: simRefino11}}}
	e.Equip[weaponSlotR], e.Equip[weaponSlotL] = arma, arma
	// Controle de Mana e os quatro buffs dela, que no print estão de pé.
	e.Affect[0] = world.Affect{Type: affectControleDeMana, Time: 5000}
	for i, sk := range []int{skillVelocidade, skillEscudoMagico, skillArmaMagica, skillToqueDeAthena} {
		sp, ok := sm.d.spells.Get(sk)
		if !ok || sp.AffectType <= 0 {
			continue
		}
		e.Affect[i+1] = world.Affect{Type: uint8(sp.AffectType), Value: uint8(sp.AffectValue),
			Level: uint16(e.BaseSpecial[3]), Time: 5000}
	}
	// As maestrias da janela (Aprender Arma 272, Branca e Negra 111, Especial 366)
	// não saem só do Toque de Athena em dobro, que rende 64: o resto vem do
	// EQUIPAMENTO, que esta ficha não veste — o mesmo motivo de a vida precisar do
	// comVidaExtra. A base é resolvida para o total cair no número da janela.
	alvo := [4]int16{272, 111, 111, cancelMaestria10}
	for range 4 {
		for k := range e.BaseSpecial {
			e.BaseSpecial[k] = alvo[k] - e.AffSpecial[k]
			e.Special[k] = e.BaseSpecial[k]
		}
		sm.d.applyAffectScore(e)
	}

	sm.d.applyAffectScore(e)

	vida := int32(cancelHP * simCancelVidaPct / 100)
	e.MaxHP = (vida - e.AffMaxHP) / 2
	e.MaxMP = (cancelMP - e.AffMaxMP) / 2
	e.HP, e.MP = vida, cancelMP
	e.AC = cancelDefesa - e.AffAC
	e.Critical = uint8(max(0, cancelCritico10/4-int(e.AffCritical)))
	lo, hi := int32(0), int32(200_000)
	for lo < hi {
		mid := (lo + hi) / 2
		e.Damage = mid
		if sm.d.effectiveDamage(e) < cancelAtaque {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	e.Damage = lo
	// A Magia sai do Atq Mágico da janela, medido na Névoa Venenosa como o cliente
	// a mostra.
	sp, _ := sm.d.spells.Get(skillNevoaVenenosa)
	spell := combat.SkillSpell{InstanceType: sp.InstanceType, InstanceValue: sp.InstanceValue, AffectValue: sp.AffectValue}
	for m := int16(0); m <= maxMagic; m++ {
		e.Magic = m
		caster := combat.SkillCaster{Class: 1, Level: int(e.Level), Int: int(effectiveInt(e)), Magic: int(effectiveMagic(e)),
			Special: effectiveSpecial(e, 3), DamageMultiPct: 100, Mortal: true, LearnedSkill: e.LearnedSkill}
		if combat.SkillBaseDamage(skillNevoaVenenosa, spell, caster, 0, int(sm.d.weaponDamage(e))) >= cancelAtqMagico {
			break
		}
	}
	return e
}

func (sm *simulador) cancelLutador(id int) *lutador {
	e := sm.montarCancel(id)
	l := &lutador{lado: &lado{e: e, cd: map[int]int64{}}, nome: "FM Cancel", maxHP: e.HP}
	l.acao = func(ld *lado, alvo *world.Entity, agora int64) golpe {
		// O Cancelamento tranca a poção do alvo por 20 s. O Escudo de Habilidade
		// (afeto 19) come o primeiro cancel e a trava não sai.
		if agora >= ld.cd[skillCancelamento] {
			sp, _ := sm.d.spells.Get(skillCancelamento)
			ld.cd[skillCancelamento] = agora + max(int64(sp.Delay)*1000, simPasso)
			if !alvo.ClearFirstAffect(19) {
				trancarAPocao(alvo, alvo.ID, uint32(agora))
			}
			return golpe{tipo: "Cancelamento"}
		}
		// A Névoa Venenosa é a skill de dano dela; entre uma e outra, golpe físico.
		if agora >= ld.cd[skillNevoaVenenosa] {
			sp, _ := sm.d.spells.Get(skillNevoaVenenosa)
			ld.cd[skillNevoaVenenosa] = agora + max(int64(sp.Delay)*1000, simPasso)
			cast, _ := sm.cast(ld, alvo, skillNevoaVenenosa)
			dmg := sm.d.resolveSkillHit(sm.w, ld.e, alvo, alvo.ID, skillNevoaVenenosa, cast)
			if dmg > 0 {
				miss := combat.ResolveParry(sm.w.Rand(), skillNevoaVenenosa, sm.d.skillParryRate(ld.e, alvo), alvo.Rsv&world.RsvBlock != 0)
				if miss = capMissStreak(ld.e, alvo.ID, miss, int(sm.d.combatRules.MaxMissStreak)); miss != 0 {
					dmg = miss
				}
			}
			return golpe{tipo: "Névoa Venenosa", dano: sm.aplicar(ld, alvo, dmg, 0, true)}
		}
		return sm.fisico(ld, alvo)
	}
	return l
}

// ---------------------------------------------------------------------------
// FM Magia Branca (hipotética)

func (sm *simulador) montarBranca(id int) *world.Entity {
	e := &world.Entity{ID: id, Class: 1, ClassMaster: classMasterMortal, Level: 399,
		Str: 12, BaseStr: 12, Int: 1000, BaseInt: 1000, Dex: 200, BaseDex: 200, Con: 2500, BaseCon: 2500,
		LearnedSkill: 0xFF} // as oito da Magia Branca
	e.Special = [4]int16{0, 255, 0, 0}
	e.BaseSpecial = e.Special
	e.Equip[weaponSlotR] = world.Item{Index: simCajadoUmaMao, Effects: [3]world.Effect{{Effect: simEfSanc, Value: simRefino11}}}
	sm.d.applyAffectScore(e)
	e.MaxHP = (simBrancaHP - e.AffMaxHP) / 2
	e.MaxMP = (simBrancaMP - e.AffMaxMP) / 2
	e.HP, e.MP = simBrancaHP, simBrancaMP
	e.AC = 2357 - e.AffAC
	e.Critical = uint8(max(0, 208/4-int(e.AffCritical)))
	e.Magic = 60
	e.Damage = 500
	return e
}

var nomeSkillBrancaSim = map[int]string{
	skillCura: "Cura", skillJulgamento: "Julgamento", skillChoqueDivino: "Choque Divino", skillFlechaMagica: "Flecha Mágica",
}

func (sm *simulador) brancaLutador(id int) *lutador {
	e := sm.montarBranca(id)
	l := &lutador{lado: &lado{e: e, cd: map[int]int64{}}, nome: "FM Branca", maxHP: simBrancaHP}
	l.acao = func(ld *lado, alvo *world.Entity, agora int64) golpe {
		// Cura em si mesma quando a vida cai, Julgamento só com a vida cheia (ele
		// cobra 70% dela), e as duas marcas no resto do tempo.
		if ld.e.HP < l.maxHP*70/100 && agora >= ld.cd[skillCura] {
			sp, _ := sm.d.spells.Get(skillCura)
			ld.cd[skillCura] = agora + max(int64(sp.Delay)*1000, simPasso)
			cast, _ := sm.cast(ld, ld.e, skillCura)
			cura := -sm.d.resolveSkillHit(sm.w, ld.e, ld.e, ld.e.ID, skillCura, cast)
			l.curar(curaReduzida(ld.e, int32(cura), uint32(agora)+1))
			return golpe{tipo: "Cura", extra: cura}
		}
		for _, sk := range []int{skillJulgamento, skillChoqueDivino, skillFlechaMagica} {
			if agora < ld.cd[sk] {
				continue
			}
			if sk == skillJulgamento && ld.e.HP < l.maxHP*90/100 {
				continue
			}
			sp, _ := sm.d.spells.Get(sk)
			ld.cd[sk] = agora + max(int64(sp.Delay)*1000, simPasso)
			return sm.skillDaBranca(l, alvo, sk, agora)
		}
		return sm.fisico(ld, alvo)
	}
	return l
}

func (sm *simulador) skillDaBranca(l *lutador, alvo *world.Entity, sk int, agora int64) golpe {
	e := l.e
	cast, _ := sm.cast(l.lado, alvo, sk)
	dmg := sm.d.resolveSkillHit(sm.w, e, alvo, alvo.ID, sk, cast)
	if sk == skillJulgamento {
		gasto, add := custoDoJulgamento(e)
		dmg += int(add)
		e.HP -= gasto
	}
	if dmg > 0 {
		if sk == skillJulgamento {
			if !julgamentoAcerta(sm.w.Rand()) {
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
	if final > 0 {
		// As marcas chamam refreshScore, que remonta a ficha pelo equipamento; estes
		// personagens vêm da janela, então a foto volta e o score dos afetos é refeito.
		fixo := fotografar(alvo)
		sm.d.aplicarMarcasDaBranca(sm.w, e, alvo, alvo.ID, sk)
		fixo.restaurar(alvo)
		sm.d.applyAffectScore(alvo)
	}
	return golpe{tipo: nomeSkillBrancaSim[sk], dano: final}
}

// ---------------------------------------------------------------------------

func TestSimulacaoTodosContraTodos(t *testing.T) {
	const lutas = 20
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	var b strings.Builder
	fmt.Fprintf(&b, "# Todos contra todos — %d lutas por confronto, com poção\n\n", lutas)

	lutadores := sm.elenco()

	fmt.Fprintf(&b, "## Fichas\n\n| Personagem | Ataque | Defesa | HP | Crítico |\n|---|---|---|---|---|\n")
	for _, c := range lutadores {
		l := comVidaExtra(c.novo(1), simVidaExtra)
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %.1f%% |\n", c.nome, sm.d.effectiveDamage(l.e), effectiveAC(l.e), l.maxHP, float64(effectiveCritical(l.e))*0.4)
	}

	for _, pct := range []int32{100, 37} {
		sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = pct, pct
		fmt.Fprintf(&b, "\n## Com %d%% de dano em jogador\n\n| Confronto | Vitórias | Mediana | 1-2 min |\n|---|---|---|---|\n", pct)
		pontos := map[string]int{}
		for i := range lutadores {
			for j := i + 1; j < len(lutadores); j++ {
				a, c := lutadores[i], lutadores[j]
				var tempos []float64
				vit := map[string]int{}
				meta := 0
				for k := range lutas {
					l := sm.lutaEntre(comVidaExtra(a.novo(1), simVidaExtra), comVidaExtra(c.novo(2), simVidaExtra), k%2 == 0)
					s := float64(l.ms) / 1000
					tempos = append(tempos, s)
					if s >= 60 && s <= 120 {
						meta++
					}
					switch {
					case strings.HasPrefix(l.vencedor, "empate"):
						vit["empate"]++
					case k%2 == 0 && l.vencedor != "":
						vit[vencedorDoPar(l, a.nome, c.nome)]++
					default:
						vit[vencedorDoPar(l, a.nome, c.nome)]++
					}
				}
				sort.Float64s(tempos)
				pontos[a.nome] += vit[a.nome]
				pontos[c.nome] += vit[c.nome]
				fmt.Fprintf(&b, "| %s × %s | %s %d, %s %d, empate %d | %.0f s | %d |\n", a.nome, c.nome,
					a.nome, vit[a.nome], c.nome, vit[c.nome], vit["empate"], tempos[len(tempos)/2], meta)
			}
		}
		tipo := make([]string, 0, len(pontos))
		for nome := range pontos {
			tipo = append(tipo, nome)
		}
		sort.Slice(tipo, func(x, y int) bool { return pontos[tipo[x]] > pontos[tipo[y]] })
		fmt.Fprintf(&b, "\n**Vitórias somadas (%d lutas por confronto, 8 adversários)**\n\n| Personagem | Vitórias |\n|---|---|\n", lutas)
		for _, nome := range tipo {
			fmt.Fprintf(&b, "| %s | %d |\n", nome, pontos[nome])
		}
	}

	t.Log("\n" + b.String())
	if out := os.Getenv("SIM_OUT"); out != "" {
		if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// vencedorDoPar traduz o nome curto do lutador para o nome do confronto.
func vencedorDoPar(l lutaConfianca, a, c string) string {
	if l.vidaA <= 0 {
		return c
	}
	if l.vidaB <= 0 {
		return a
	}
	return "empate"
}

// personagemDaSimulacao é um lutador do todos contra todos.
type personagemDaSimulacao struct {
	nome string
	novo func(int) *lutador
}

func (sm *simulador) elenco() []personagemDaSimulacao {
	return []personagemDaSimulacao{
		{"Paladino (TK Confiança)", sm.paladinoLutador},
		{"Porradeiro Trans (Éden)", func(id int) *lutador { return sm.transLutador(id, simEdenAnct) }},
		{"TK-MAGO (Espada Mágica)", sm.tkMagoLutador},
		{"Black (FM Magia Negra)", sm.blackLutador},
		{"FM Branca", sm.brancaLutador},
		{"FM Cancel", sm.cancelLutador},
		{"Xorimpas 8ª Sobrevivência", func(id int) *lutador { return sm.htUmaOitava(id, learnedTempestade) }},
		{"Xorimpas 8ª Troca", func(id int) *lutador { return sm.htUmaOitava(id, 1<<15) }},
		{"Xorimpas 8ª Captura", func(id int) *lutador { return sm.htUmaOitava(id, learnedInvisibilidade) }},
	}
}

// TestSimulacaoBotoes varre os três botões de balanceamento no todos contra
// todos e mostra o placar de cada combinação.
//
//	go test -tags simulacao -run TestSimulacaoBotoes -v ./tmserver/internal/handler/
func TestSimulacaoBotoes(t *testing.T) {
	const lutas = 10
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37
	defer func(a, b, c, d, e, f, g, h int) {
		confiancaEsquivaPctAtual, laminaSombrasForcaPctAtual, laminaSombrasMultBase10Atual, manaControlCustoPctCancel = a, b, c, d
		esquivaTetoAtual, confiancaDanoPct, manaControlDivisorCancel, simCancelVidaPct = e, f, int32(g), h
	}(confiancaEsquivaPctAtual, laminaSombrasForcaPctAtual, laminaSombrasMultBase10Atual, manaControlCustoPctCancel,
		esquivaTetoAtual, confiancaDanoPct, int(manaControlDivisorCancel), simCancelVidaPct)

	combos := []struct {
		nome                                     string
		esquiva, lamina, mult, cus               int
		teto, danoConfianca, divisorCancel, vida int
	}{
		{"como está", 60, 50, 20, 100, 650, 100, 82, 100},
		{"teto de esquiva 50%", 60, 50, 20, 100, 500, 100, 82, 100},
		{"teto 50% + Confiança bate 70%", 60, 50, 20, 100, 500, 70, 82, 100},
		{"teto 50% + Confiança 70% + Lâmina 30/×1,5", 60, 30, 15, 100, 500, 70, 82, 100},
		{"acima + FM Cancel absorve o dobro", 60, 30, 15, 60, 500, 70, 160, 100},
		{"acima + FM Cancel com o dobro de vida", 60, 30, 15, 60, 500, 70, 160, 200},
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Botões de balanceamento — %d lutas por confronto, 37%%, com poção\n\n", lutas)
	for _, c := range combos {
		confiancaEsquivaPctAtual, laminaSombrasForcaPctAtual = c.esquiva, c.lamina
		laminaSombrasMultBase10Atual, manaControlCustoPctCancel = c.mult, c.cus
		esquivaTetoAtual, confiancaDanoPct = c.teto, c.danoConfianca
		manaControlDivisorCancel, simCancelVidaPct = int32(c.divisorCancel), c.vida
		pontos, empates := sm.placarDaRodada(lutas)
		fmt.Fprintf(&b, "## %s\n\n| Personagem | Vitórias |\n|---|---|\n", c.nome)
		nomes := make([]string, 0, len(pontos))
		for n := range pontos {
			nomes = append(nomes, n)
		}
		sort.Slice(nomes, func(x, y int) bool { return pontos[nomes[x]] > pontos[nomes[y]] })
		for _, n := range nomes {
			fmt.Fprintf(&b, "| %s | %d |\n", n, pontos[n])
		}
		fmt.Fprintf(&b, "\nEmpates de 15 min: %d de %d lutas.\n\n", empates, 36*lutas)
	}
	t.Log("\n" + b.String())
	if out := os.Getenv("SIM_OUT"); out != "" {
		if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// placarDaRodada roda todos contra todos e devolve as vitórias por personagem e
// quantas lutas passaram de 15 minutos.
func (sm *simulador) placarDaRodada(lutas int) (map[string]int, int) {
	pontos, empates := map[string]int{}, 0
	l := sm.elenco()
	for i := range l {
		for j := i + 1; j < len(l); j++ {
			for k := range lutas {
				r := sm.lutaEntre(l[i].novo(1), l[j].novo(2), k%2 == 0)
				switch {
				case r.vidaB <= 0:
					pontos[l[i].nome]++
				case r.vidaA <= 0:
					pontos[l[j].nome]++
				default:
					empates++
				}
			}
			pontos[l[i].nome] += 0
			pontos[l[j].nome] += 0
		}
	}
	return pontos, empates
}

// TestSimulacaoVidaEPct mede o que muda quando a VIDA de todo mundo cresce e
// quando a % de dano em PvP muda. A poção é fixa em 2.000 por segundo, então é a
// razão entre dano e vida que decide se a luta acaba e em quanto tempo.
func TestSimulacaoVidaEPct(t *testing.T) {
	const lutas = 6
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	var b strings.Builder
	fmt.Fprintf(&b, "# Vida × %% de dano em PvP — %d lutas por confronto, com poção\n\n", lutas)
	fmt.Fprintf(&b, "| %% PvP | Vida de todos | Lutas que acabam | De 1 a 2 min | Mediana das que acabam |\n|---|---|---|---|---|\n")
	for _, pct := range []int32{37, 60, 100} {
		sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = pct, pct
		for _, vezes := range []int32{1, 2, 3} {
			var tempos []float64
			acabaram, meta, total := 0, 0, 0
			l := sm.elenco()
			for i := range l {
				for j := i + 1; j < len(l); j++ {
					for k := range lutas {
						a, c := l[i].novo(1), l[j].novo(2)
						for _, x := range []*lutador{a, c} {
							x.maxHP *= vezes
							x.e.HP = x.maxHP
							x.e.MaxHP *= vezes
						}
						r := sm.lutaEntre(a, c, k%2 == 0)
						total++
						if r.vidaA > 0 && r.vidaB > 0 {
							continue
						}
						acabaram++
						s := float64(r.ms) / 1000
						tempos = append(tempos, s)
						if s >= 60 && s <= 120 {
							meta++
						}
					}
				}
			}
			sort.Float64s(tempos)
			mediana := 0.0
			if len(tempos) > 0 {
				mediana = tempos[len(tempos)/2]
			}
			fmt.Fprintf(&b, "| %d%% | ×%d | %d de %d | %d | %.0f s |\n", pct, vezes, acabaram, total, meta, mediana)
		}
	}
	t.Log("\n" + b.String())
	if out := os.Getenv("SIM_OUT"); out != "" {
		if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// simVidaExtra é a vida dos acessórios que as fichas de referência não tinham
// (brinco, ankhs +11, místicos, planetas e capa): o Marco estima até 7 mil. Entra
// em todo mundo no todos contra todos.
var simVidaExtra = int32(7000)

// comVidaExtra soma a vida que os acessórios dão e que as fichas de referência
// não tinham: brinco, ankhs +11, místicos, planetas e capa (o Marco estima até
// 7 mil). O HP máximo do jogador é o dobro do MaxHP guardado (scoreMaxHP).
func comVidaExtra(l *lutador, extra int32) *lutador {
	l.e.MaxHP += extra / 2
	l.maxHP += extra
	l.e.HP = l.maxHP
	return l
}

// TestSimulacaoVidaDosAcessorios mede o todos contra todos com a vida dos
// acessórios somada a todo mundo.
func TestSimulacaoVidaDosAcessorios(t *testing.T) {
	const lutas = 10
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37
	var b strings.Builder
	fmt.Fprintf(&b, "# Vida dos acessórios — %d lutas por confronto, 37%%, com poção\n\n", lutas)
	fmt.Fprintf(&b, "| Vida a mais | Lutas que acabam | De 1 a 2 min | Mediana das que acabam | Quem mais venceu |\n|---|---|---|---|---|\n")
	for _, extra := range []int32{0, 3500, 7000, 10_000} {
		var tempos []float64
		acabaram, meta, total := 0, 0, 0
		pontos := map[string]int{}
		l := sm.elenco()
		for i := range l {
			for j := i + 1; j < len(l); j++ {
				for k := range lutas {
					a := comVidaExtra(l[i].novo(1), extra)
					c := comVidaExtra(l[j].novo(2), extra)
					r := sm.lutaEntre(a, c, k%2 == 0)
					total++
					switch {
					case r.vidaB <= 0:
						pontos[l[i].nome]++
					case r.vidaA <= 0:
						pontos[l[j].nome]++
					default:
						continue
					}
					acabaram++
					s := float64(r.ms) / 1000
					tempos = append(tempos, s)
					if s >= 60 && s <= 120 {
						meta++
					}
				}
			}
		}
		sort.Float64s(tempos)
		mediana := 0.0
		if len(tempos) > 0 {
			mediana = tempos[len(tempos)/2]
		}
		melhor, melhorN := "-", 0
		for nome, n := range pontos {
			if n > melhorN || (n == melhorN && nome < melhor) {
				melhor, melhorN = nome, n
			}
		}
		fmt.Fprintf(&b, "| +%d | %d de %d | %d | %.0f s | %s (%d) |\n", extra, acabaram, total, meta, mediana, melhor, melhorN)
	}
	t.Log("\n" + b.String())
	if out := os.Getenv("SIM_OUT"); out != "" {
		if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSimulacaoCancel(t *testing.T) {
	const lutas = 10
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37
	defer func(p, d int) { cancelPerfuracaoPct, cancelDanoDuasArmas = p, d }(cancelPerfuracaoPct, cancelDanoDuasArmas)

	var b strings.Builder
	fmt.Fprintf(&b, "# FM Cancelamento física — %d lutas por confronto, 37%%, com poção e +%d de vida\n\n", lutas, simVidaExtra)
	fmt.Fprintf(&b, "| Perfuração | Dano com duas armas | Ataque dela | × Trans | × TK-MAGO | × Black | × Xorimpas Captura | × Paladino | Vitórias somadas |\n|---|---|---|---|---|---|---|---|---|\n")
	adversarios := []struct {
		nome string
		novo func(int) *lutador
	}{
		{"Trans", func(id int) *lutador { return sm.transLutador(id, simEdenAnct) }},
		{"TK-MAGO", sm.tkMagoLutador},
		{"Black", sm.blackLutador},
		{"Xorimpas Captura", func(id int) *lutador { return sm.htUmaOitava(id, learnedInvisibilidade) }},
		{"Paladino", sm.paladinoLutador},
	}
	for _, perf := range []int{0, 40, 60, 80} {
		for _, fis := range []int{0, 100, 150, 200} {
			cancelPerfuracaoPct, cancelDanoDuasArmas = perf, fis
			ataque := sm.d.effectiveDamage(sm.cancelLutador(1).e)
			linha := []string{}
			total := 0
			for _, o := range adversarios {
				vit, empates := 0, 0
				for k := range lutas {
					r := sm.lutaEntre(comVidaExtra(sm.cancelLutador(1), simVidaExtra), comVidaExtra(o.novo(2), simVidaExtra), k%2 == 0)
					switch {
					case r.vidaB <= 0:
						vit++
					case r.vidaA > 0:
						empates++
					}
				}
				total += vit
				linha = append(linha, fmt.Sprintf("%d de %d (%d emp)", vit, lutas, empates))
			}
			fmt.Fprintf(&b, "| %d%% | +%d%% | %d | %s | %d |\n", perf, fis, ataque, strings.Join(linha, " | "), total)
		}
	}
	t.Log("\n" + b.String())
	if out := os.Getenv("SIM_OUT"); out != "" {
		if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
