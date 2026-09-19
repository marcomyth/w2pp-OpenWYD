//go:build simulacao

package handler

// O BEAST MASTER (19/09/2026): Urso de pé, Fera Flamejante e Som das Fadas na
// mão, o bando batendo em volta — contra o TK-MAGO.
//
// A ficha do BM NÃO vem de janela do jogo (ele ainda não foi montado em jogo).
// Vem da Black: mesmos atributos, mesma Magia, mesmo refino. Isso é de propósito
// — o que se quer medir é a ÁRVORE, não a build, e com as duas fichas iguais a
// diferença que sobra é só a classe.
//
// A arma é LANÇA, não cajado, como pedido. Perde o dano de arma do cajado, que
// é parte de por que o elemental dele fica abaixo do da Black.
//
//	go test -tags simulacao -run TestSimulacaoBM -v ./tmserver/internal/handler/

import (
	"fmt"
	"path/filepath"
	"sort"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const (
	simLancaDeOdin = 1008 // lança de duas mãos
	// A 8ª do BM é a Succubus (Evocação). As outras duas árvores ficam no teto de
	// quem não tem a oitava — regra que AINDA NÃO existe no servidor, então aqui
	// ela é um número da simulação, para medir o efeito antes de escrever.
	simBMTetoSemOitava = 200
	simBMEvocacao      = 320
	simBMMaestriaCheia = 400
	simCajado2Maos     = simEirenus // Eirenus, cajado de duas mãos (EF_WTYPE 32)
)

// simBMLearned: tudo menos o Espírito Vingador (8ª da Elemental, bit 7) e o Éden
// (8ª da Transformação, bit 23). A única oitava dele é a Succubus (bit 15).
// learnedSuccubus é a 8ª da Evocação (skill 63, bit 63%24 = 15).
const learnedSuccubus = 1 << 15

// simBMLearned: tudo MENOS as três oitavas. Qual delas o personagem tem entra
// por montarBMCom — o BM só pode ter uma.
const simBMLearned = 0xFFFFFF &^ (1 << 7) &^ (1 << 15) &^ (1 << 23)

var nomeSkillBM = map[int]string{
	48: "Fera Flamejante", 50: "Som das Fadas", 52: "Fúria de Gaia",
}

// ordemDoBM: as três que o operador escolheu, da recarga maior para a menor.
var ordemDoBM = []int{52, 50, 48}

// montarBM devolve o BM, transformado em Urso ou não.
//
// oitava é a ÚNICA 8ª que ele tem — learnedEspiritoVingador (Elemental) ou
// learnedSuccubus (Evocação). É a escolha que decide se o bônus de arma da
// Elemental (120%/140%, arvore_elemental.go) existe para este personagem.
func (sm *simulador) montarBMCom(id int, urso bool, tetoSemOitava int, arma int16, oitava int32) *world.Entity {
	e := &world.Entity{ID: id, Class: 2, ClassMaster: classMasterMortal, Level: 399,
		Str: 12, BaseStr: 12, Dex: 12, BaseDex: 12, Int: 3148, BaseInt: 3148, Con: 211, BaseCon: 211,
		LearnedSkill: simBMLearned | oitava}
	// [1] Elemental, [2] Evocação, [3] Transformação (SkillKind das faixas 48/56/64).
	// A árvore da 8ª vai cheia; as outras duas param no teto de quem não tem oitava.
	e.Special = [4]int16{0, int16(tetoSemOitava), int16(tetoSemOitava), int16(tetoSemOitava)}
	switch oitava {
	case learnedEspiritoVingador:
		e.Special[1] = simBMMaestriaCheia
	case learnedSuccubus:
		e.Special[2] = simBMEvocacao
	}
	e.BaseSpecial = e.Special
	e.Equip[weaponSlotR] = world.Item{Index: arma, Effects: [3]world.Effect{{Effect: simEfSanc, Value: simRefino11}}}
	// CALIBRAR PRIMEIRO, TRANSFORMAR DEPOIS. Calibrado com o Urso de pé, a conta
	// `MaxHP = (alvo - AffMaxHP)/2` desconta exatamente o que o Urso deu, e as
	// duas fichas saem idênticas — o bônus da transformação some dentro da
	// própria calibragem. A base é sempre a da Black; o Urso entra por cima.
	e.Affect[0] = world.Affect{Type: affectControleDeMana, Time: 5000}
	sm.d.applyAffectScore(e)
	e.MaxHP = (simBlackHP - e.AffMaxHP) / 2
	e.MaxMP = (simBlackMP - e.AffMaxMP) / 2
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
	if urso {
		// Afeto 16, Value 2 = Urso (transMesh 23). O Level é a maestria de
		// Transformação no momento do lançamento (transform.go).
		e.Affect[1] = world.Affect{Type: affectTransform, Value: 2, Level: uint16(tetoSemOitava), Time: 5000}
		sm.d.applyAffectScore(e)
	}
	e.HP, e.MP = effectiveMaxHP(e), scoreMaxMP(e)
	// A Magia é a mesma que a Black alcança com estes atributos: isola a árvore.
	e.Magic = sm.montarBlack(id + 90).Magic
	return e
}

// montarBM: o BM EVOCADOR (8ª = Succubus) de lança, que é o que os diagnósticos
// anteriores mediam. Sem a 8ª Elemental ele NÃO tem o bônus de arma.
func (sm *simulador) montarBM(id int, urso bool, tetoSemOitava int) *world.Entity {
	return sm.montarBMCom(id, urso, tetoSemOitava, simLancaDeOdin, learnedSuccubus)
}

// bmLutador: o BM com o bando de uma criatura batendo junto, na cadência da
// evocação. O dano do bando não passa pela conta do servidor — é a regra de
// arvore_evocacao.go — mas ainda passa pela absorção do alvo.
func (sm *simulador) bmLutador(id int, urso bool, criatura int) *lutador {
	e := sm.montarBM(id, urso, simBMTetoSemOitava)
	nome := "BM"
	if urso {
		nome = "BM Urso"
	}
	l := &lutador{lado: &lado{e: e, cd: map[int]int64{}}, nome: nome, maxHP: effectiveMaxHP(e)}
	var proximoBando int64
	l.acao = func(ld *lado, alvo *world.Entity, agora int64) golpe {
		// O bando bate em paralelo, na cadência dele, faça o dono o que fizer.
		bando := 0
		if agora >= proximoBando {
			proximoBando = agora + evocacaoCadenciaMs
			evo := int(effectiveSpecial(ld.e, 2))
			n := summonCount(criatura+1, evo)
			porCabeca := danoDaEvocacaoEmJogador(criatura, evo, int(effectiveAC(alvo)))
			bando = sm.d.absorbBlow(sm.w, alvo, porCabeca*n, false)
			alvo.HP = max(0, alvo.HP-int32(bando))
		}
		g := sm.acaoDoBM(l, alvo, agora)
		g.extra = bando
		g.dano += bando
		return g
	}
	return l
}

func (sm *simulador) acaoDoBM(l *lutador, alvo *world.Entity, agora int64) golpe {
	ld := l.lado
	for _, sk := range ordemDoBM {
		if agora < ld.cd[sk] {
			continue
		}
		sp, _ := sm.d.spells.Get(sk)
		ld.cd[sk] = agora + max(int64(sp.Delay)*1000, simPasso)
		cast, _ := sm.cast(ld, alvo, sk)
		dmg := sm.d.resolveSkillHit(sm.w, ld.e, alvo, alvo.ID, sk, cast)
		// SEM crítico de mago: magoCritico (arvore_magia_negra.go) só cobre o TK
		// Espada Mágica e a FM Magia Negra. O BM nunca teve, e é o que o operador
		// quer que continue — o kit dele são as evocações.
		return golpe{tipo: nomeSkillBM[sk], dano: sm.aplicar(ld, alvo, dmg, 0, true)}
	}
	return sm.fisico(ld, alvo)
}

// A ficha: o que o Urso muda, e quanto cada skill tira do TK-MAGO.
func TestSimulacaoBMFicha(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37
	alvo := comVidaExtra(sm.tkMagoLutador(2), simVidaExtra)
	fmt.Printf("alvo: TK-MAGO, defesa %d, HP %d\n", effectiveAC(alvo.e), alvo.e.HP)

	for _, urso := range []bool{false, true} {
		e := sm.montarBM(1, urso, simBMTetoSemOitava)
		rot := "sem transformação"
		if urso {
			rot = fmt.Sprintf("Homem-Urso (Transformação %d)", simBMTetoSemOitava)
		}
		fmt.Printf("\n=== %s ===\n", rot)
		fmt.Printf("ataque %d  defesa %d  HP %d  MP %d  Magia %d  mult.dano %d%%\n",
			sm.d.effectiveDamage(e), effectiveAC(e), effectiveMaxHP(e), scoreMaxMP(e), e.Magic, e.AffDamageMultiPct)
		for _, sk := range ordemDoBM {
			l := &lutador{lado: &lado{e: sm.montarBM(1, urso, simBMTetoSemOitava), cd: map[int]int64{}}}
			v := []int{}
			for range 400 {
				hp := alvo.e.HP
				cast, _ := sm.cast(l.lado, alvo.e, sk)
				dmg := sm.d.resolveSkillHit(sm.w, l.e, alvo.e, alvo.e.ID, sk, cast)
				v = append(v, sm.aplicar(l.lado, alvo.e, dmg, 0, true))
				alvo.e.HP = hp
			}
			sort.Ints(v)
			sp, _ := sm.d.spells.Get(sk)
			fmt.Printf("  %-18s mediana %6d  p90 %6d  recarga %2ds  → %6.0f /s\n",
				nomeSkillBM[sk], v[len(v)/2], v[len(v)*9/10], sp.Delay,
				float64(v[len(v)/2])/float64(max(sp.Delay, 1)))
		}
	}
}

// A luta pedida: BM Urso com skill e bando, contra o TK-MAGO, com poção dos dois lados.
func TestSimulacaoBMContraTKMago(t *testing.T) {
	const lutas = 10
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37

	// A Ultra Poção de Cura é 500; os 2.000 são o TETO do tique, que só um macro
	// de muitas poções por segundo alcança (pocao-nao-cura-levanta-a-barra).
	pocaoOriginal := simPocaoPorSegundo
	defer func() { simPocaoPorSegundo = pocaoOriginal }()
	for _, pocao := range []int32{500, 2000} {
		simPocaoPorSegundo = pocao
		fmt.Printf("\n=== poção %d/s, PvP %d%%, cadência do bando %d ms, Transformação %d ===\n",
			pocao, sm.d.combatRules.PvPSkillPct, evocacaoCadenciaMs, simBMTetoSemOitava)
		sm.lutasDoBM(lutas)
	}
}

func (sm *simulador) lutasDoBM(lutas int) {
	for _, criatura := range []int{7, 4, 6} {
		for _, urso := range []bool{true, false} {
			vit, der, emp := 0, 0, 0
			var somaMs int64
			var somaBando, somaSkill int
			for k := range lutas {
				a := comVidaExtra(sm.bmLutador(1, urso, criatura), simVidaExtra)
				b := comVidaExtra(sm.tkMagoLutador(2), simVidaExtra)
				nome := a.nome
				r := sm.lutaEntre(a, b, k%2 == 0)
				somaMs += r.ms
				for _, g := range r.golpesA {
					somaBando += g.extra
					somaSkill += g.dano - g.extra
				}
				switch r.vencedor {
				case nome:
					vit++
				case "":
					emp++
				default:
					der++
				}
			}
			rot := "BM"
			if urso {
				rot = "BM Urso"
			}
			total := somaBando + somaSkill
			pct := 0
			if total > 0 {
				pct = somaBando * 100 / total
			}
			fmt.Printf("%-8s + %2d×%-13s  %2d vitórias, %2d derrotas, %2d empates   %5.0f s   bando = %d%% do dano\n",
				rot, summonCount(criatura+1, simBMEvocacao), nomeDaCriatura[criatura],
				vit, der, emp, float64(somaMs)/float64(lutas)/1000, pct)
		}
	}
}

// O BM Urso com a Succubus contra o elenco inteiro, para saber se ele está fora
// de curva ou se é o elenco que está.
func TestSimulacaoBMContraOElenco(t *testing.T) {
	const lutas = 10
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37
	pocaoOriginal := simPocaoPorSegundo
	defer func() { simPocaoPorSegundo = pocaoOriginal }()
	simPocaoPorSegundo = 500

	for _, criatura := range []int{7, 4} {
		fmt.Printf("\n=== BM Urso + %d×%s, poção 500/s ===\n",
			summonCount(criatura+1, simBMEvocacao), nomeDaCriatura[criatura])
		for _, o := range sm.elenco() {
			vit, der, emp := 0, 0, 0
			var somaMs int64
			for k := range lutas {
				a := comVidaExtra(sm.bmLutador(1, true, criatura), simVidaExtra)
				b := comVidaExtra(o.novo(2), simVidaExtra)
				nome := a.nome
				r := sm.lutaEntre(a, b, k%2 == 0)
				somaMs += r.ms
				switch r.vencedor {
				case nome:
					vit++
				case "":
					emp++
				default:
					der++
				}
			}
			fmt.Printf("  contra %-28s %2d vitórias, %2d derrotas, %2d empates   %5.0f s\n",
				o.nome, vit, der, emp, float64(somaMs)/float64(lutas)/1000)
		}
	}
}

// O que cada transformação entrega ao BM, campo a campo, em três maestrias.
// Dano mágico não entra (o multiplicador fica fora da magia, decisão de 10/09);
// o que vale é vida, defesa e resistência.
func TestSimulacaoBMTransformacoes(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	base := sm.montarBM(1, false, simBMTetoSemOitava)
	fmt.Printf("BM sem transformação: HP %d, defesa %d, resist %v, vel.ataque %d, crítico %d\n\n",
		effectiveMaxHP(base), effectiveAC(base), base.AffResist, base.AffAttackSpeed, base.AffCritical)

	nomes := [5]string{"Lobo", "Urso", "Astaroth", "Titã", "Éden"}
	for _, nivel := range []int{50, 100, 200} {
		fmt.Printf("=== Transformação %d ===\n", nivel)
		fmt.Printf("%-10s %8s %8s %8s %8s %8s %8s\n",
			"forma", "HP", "+HP%", "defesa", "+def%", "resist", "vel.atq")
		for v := range 5 {
			e := sm.montarBM(1, false, simBMTetoSemOitava)
			e.Affect[1] = world.Affect{Type: affectTransform, Value: uint8(v + 1), Level: uint16(nivel), Time: 5000}
			sm.d.applyAffectScore(e)
			hp, ac := effectiveMaxHP(e), effectiveAC(e)
			fmt.Printf("%-10s %8d %7d%% %8d %7d%% %8d %8d\n", nomes[v],
				hp, (hp-effectiveMaxHP(base))*100/effectiveMaxHP(base),
				ac, (ac-effectiveAC(base))*100/effectiveAC(base),
				e.AffResist[0], e.AffAttackSpeed)
		}
		fmt.Println()
	}
}

// As duas formas que o BM elemental usa — Urso e Titã — com e sem a passiva da
// árvore de Transformação (Escudo do Tormento, bit 19). O Titã não tem passiva
// associada no servidor (transform.go: "no learned-skill gate").
func TestSimulacaoBMUrsoETitaComPassiva(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	base := sm.montarBM(1, false, simBMTetoSemOitava)
	fmt.Printf("BM sem transformação: HP %d, defesa %d, resist %d, vel.ataque %d\n\n",
		effectiveMaxHP(base), effectiveAC(base), base.AffResist[0], base.AffAttackSpeed)

	monta := func(forma uint8, nivel int, passiva bool) *world.Entity {
		e := sm.montarBM(1, false, simBMTetoSemOitava)
		if !passiva {
			e.LearnedSkill &^= learnedBear
		}
		e.Affect[1] = world.Affect{Type: affectTransform, Value: forma, Level: uint16(nivel), Time: 5000}
		sm.d.applyAffectScore(e)
		return e
	}
	linha := func(rot string, e *world.Entity) {
		fmt.Printf("  %-26s HP %6d (%+3d%%)  defesa %5d (%+3d%%)  resist %3d  vel.atq %3d\n", rot,
			effectiveMaxHP(e), (effectiveMaxHP(e)-effectiveMaxHP(base))*100/effectiveMaxHP(base),
			effectiveAC(e), (effectiveAC(e)-effectiveAC(base))*100/effectiveAC(base),
			e.AffResist[0], e.AffAttackSpeed)
	}
	for _, nivel := range []int{100, 200, 255} {
		fmt.Printf("=== Transformação %d ===\n", nivel)
		linha("Urso SEM Escudo Tormento", monta(2, nivel, false))
		linha("Urso COM Escudo Tormento", monta(2, nivel, true))
		linha("Titã (não tem passiva)", monta(4, nivel, true))
		fmt.Println()
	}
}

// O BM ELEMENTAL (8ª = Espírito Vingador) com cada arma, contra o TK-MAGO.
// Mostra o que os 120% da lança e os 140% do cajado valem em dano de verdade, e
// o que o BM Evocador (8ª = Succubus) perde por não ter a 8ª da Elemental.
func TestSimulacaoBMElementalPorArma(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37
	alvo := comVidaExtra(sm.tkMagoLutador(2), simVidaExtra)
	fmt.Printf("alvo: TK-MAGO, defesa %d, HP %d\n\n", effectiveAC(alvo.e), alvo.e.HP)

	casos := []struct {
		rot    string
		arma   int16
		oitava int32
	}{
		{"Elemental, lança (120%)", simLancaDeOdin, learnedEspiritoVingador},
		{"Elemental, cajado 2 mãos (140%)", simCajado2Maos, learnedEspiritoVingador},
		{"Evocador (Succubus), lança", simLancaDeOdin, learnedSuccubus},
		{"Evocador (Succubus), cajado", simCajado2Maos, learnedSuccubus},
	}
	fmt.Printf("%-34s %6s %8s %8s %8s %9s\n", "build", "Arma%", "Gaia", "Fadas", "Fera", "soma/s")
	for _, c := range casos {
		e := sm.montarBMCom(1, false, simBMTetoSemOitava, c.arma, c.oitava)
		// O pct EFETIVO: combat.go só aplica quando bmElemental vale, isto é, com
		// a 8ª da Elemental na mão.
		pct := elementalArmaPct
		if bmElemental(e) {
			pct = armaPctElemental(e, sm.d.itemAbility)
		}
		fmt.Printf("%-34s %5d%%", c.rot, pct)
		var porSeg float64
		for _, sk := range ordemDoBM {
			l := &lutador{lado: &lado{e: sm.montarBMCom(1, false, simBMTetoSemOitava, c.arma, c.oitava), cd: map[int]int64{}}}
			v := []int{}
			for range 400 {
				hp := alvo.e.HP
				cast, _ := sm.cast(l.lado, alvo.e, sk)
				dmg := sm.d.resolveSkillHit(sm.w, l.e, alvo.e, alvo.e.ID, sk, cast)
				v = append(v, sm.aplicar(l.lado, alvo.e, dmg, 0, true))
				alvo.e.HP = hp
			}
			sort.Ints(v)
			sp, _ := sm.d.spells.Get(sk)
			fmt.Printf(" %8d", v[len(v)/2])
			porSeg += float64(v[len(v)/2]) / float64(max(sp.Delay, 1))
		}
		fmt.Printf(" %9.0f\n", porSeg)
	}

	fmt.Printf("\ncura da lança e mana do cajado, por golpe que entra (1.000 amostras):\n")
	for _, c := range casos[:2] {
		e := sm.montarBMCom(1, false, simBMTetoSemOitava, c.arma, c.oitava)
		var somaV, somaM int32
		pegouV, pegouM := 0, 0
		for range 1000 {
			v, m := sm.d.rouboDaElemental(sm.w.Rand(), e, 1880, tetoDoRouboDeMana(e))
			somaV, somaM = somaV+v, somaM+m
			if v > 0 {
				pegouV++
			}
			if m > 0 {
				pegouM++
			}
		}
		fmt.Printf("  %-34s vida: %d%% dos golpes, média %d | mana: %d%% dos golpes, média %d\n",
			c.rot, pegouV/10, somaV/1000, pegouM/10, somaM/1000)
	}
}
