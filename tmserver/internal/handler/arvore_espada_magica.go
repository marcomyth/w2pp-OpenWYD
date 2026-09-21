package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/combat"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Árvore ESPADA MÁGICA do TransKnight (skills 16-23, maestria Special[3]) —
// REGRAS DO SERVIDOR, decididas pelo Marco em 17/09/2026.
//
// O "TK Espada Mágica" é o TK com a Tempestade de Gelo (23, a 8ª da árvore)
// aprendida. É full INT: o dano segue a fórmula mágica pela INT, a lança dá
// 140%, as skills da árvore podem critar de ×2 a ×4, o Exterminar vira tudo ou
// nada e a INT rouba vida. O Possuído e o Samaritano dão a CON que ele não tem.
//
// i é a régua de INT, em milésimos: INT ÷ 2.500, teto 1000 (o TK-MAGO Mortal de
// referência, com 2.848, já fica no teto).
const (
	skillPerseguicao      = 16
	skillExterminar       = 22
	skillTempestadeDeGelo = 23

	learnedTempestadeDeGelo = 1 << 23 // skill 23

	espadaMagicaMaestriaMax = 255
	espadaMagicaIntTeto     = 2500

	espadaMagicaArmaPct = 100
)

// espadaMagicaLancaPct é a lança da árvore, e é var porque a simulação o varre.
//
// Subiu de 140 para 190 em 21/09/2026. Com 140 o TK-MAGO fazia 34 vitórias e 82
// derrotas no torneio contra as 55 da Black, que é a referência da faixa mágica:
// ele tirava 2.582 de dano por segundo contra os 3.603 dela, e levava 2.392 com
// uma poção que só levanta 2.000 — morria devagar sem conseguir matar. Com 190 os
// dois empatam em 55, e o duelo dele passa de 20 s para 58 s, dentro da meta.
//
// A curva é íngreme: 200 já o põe em 71 e 260 em 108, porque o bônus multiplica o
// dano BRUTO da skill, antes de a defesa do alvo ser subtraída.
var espadaMagicaLancaPct = 190

// tkEspadaMagica diz se as regras da árvore valem para e.
func tkEspadaMagica(e *world.Entity) bool {
	return e != nil && e.Class == 0 && world.IsPlayer(e.ID) && e.LearnedSkill&learnedTempestadeDeGelo != 0
}

func maestriaEspadaMagica(e *world.Entity) int {
	return max(0, min(effectiveSpecial(e, 3), espadaMagicaMaestriaMax))
}

// reguaDeInt é i: 0 a 1000 pela INT, com teto em 2.500.
func reguaDeInt(e *world.Entity) int {
	return max(0, min(int(effectiveInt(e)), espadaMagicaIntTeto)) * 1000 / espadaMagicaIntTeto
}

// skillDeDanoDaEspadaMagica são as skills de dano da árvore (a 16, Perseguição,
// é buff).
func skillDeDanoDaEspadaMagica(skillnum int) bool {
	return skillnum > skillPerseguicao && skillnum <= skillTempestadeDeGelo
}

// armaPctEspadaMagica: toda lança (EF_WTYPE 21, com as Anct) 140%, qualquer
// outra arma 100%, em toda evolução.
func armaPctEspadaMagica(e *world.Entity, itemAbility func(world.Item, uint8) int) int {
	if itemAbility != nil && itemAbility(e.Equip[weaponSlotR], efWType) == wtypeLanca {
		return espadaMagicaLancaPct
	}
	return espadaMagicaArmaPct
}

// ---------------------------------------------------------------------------
// Crítico de mago, nas skills de dano da árvore — do TK Espada Mágica e da FM
// Magia Negra (arvore_magia_negra.go, magoCritico): chance 10% + 15% × i, multiplicador sorteado de
// ×2,0 até ×(2 + 2 × i), em passos de 0,1. A INT sobe a chance e o teto; o
// sorteio dentro da faixa é a sorte.
const (
	espadaCritChanceBase = 10
	espadaCritChanceInt  = 15
	espadaCritMultBase10 = 20
	espadaCritMultInt10  = 20
)

// rolarCriticoDeMago devolve o multiplicador × 10 (20 a 40), ou 0 sem
// crítico.
func rolarCriticoDeMago(r combat.Rand, e *world.Entity) int {
	i := reguaDeInt(e)
	if r.Intn(100) >= espadaCritChanceBase+espadaCritChanceInt*i/1000 {
		return 0
	}
	return espadaCritMultBase10 + r.Intn(espadaCritMultInt10*i/1000+1)
}

// ---------------------------------------------------------------------------
// 22 · Exterminar: gasta toda a mana como no legado, mas só acerta em 10% das
// vezes — tudo ou nada. O sorteio substitui a esquiva do alvo, e a mana vai
// embora mesmo no erro.
const exterminarAcertoPct = 10

func exterminarAcerta(r combat.Rand) bool {
	return r.Intn(100) < exterminarAcertoPct
}

// ---------------------------------------------------------------------------
// Roubo de vida pela INT: a cada skill da árvore que acerta, chance de 30% × i
// de curar 25% do dano causado, até 10% do HP máximo por golpe.
const (
	rouboDeVidaChance = 30
	rouboDeVidaPct    = 25
	rouboDeVidaTetoHP = 10
)

// rouboDeVida devolve quanto o golpe cura (0 quando o sorteio não pega).
func rouboDeVida(r combat.Rand, e *world.Entity, dano int) int32 {
	if dano <= 0 {
		return 0
	}
	if r.Intn(1000) >= rouboDeVidaChance*reguaDeInt(e)/100 {
		return 0
	}
	return min(int32(dano*rouboDeVidaPct/100), effectiveMaxHP(e)*rouboDeVidaTetoHP/100)
}

func (d *Dispatcher) curarPeloRoubo(w *world.World, s *world.Session, e *world.Entity, cura int32) {
	if cura <= 0 || e.HP <= 0 {
		return
	}
	e.HP = min(effectiveMaxHP(e), e.HP+cura)
	if s != nil {
		s.ReqHp = e.HP
		setReqHp(s, e)
		d.sendSetHpMp(w, s, e)
	}
}

// ---------------------------------------------------------------------------
// Possuído (affect 14) e Samaritano (affect 24): no TK Espada Mágica cada um dá
// mais até +500 de CON pela maestria da Espada Mágica, e os dois somam.
const espadaMagicaConPorBuff = 500

func conDaEspadaMagica(e *world.Entity) int32 {
	if !tkEspadaMagica(e) {
		return 0
	}
	return int32(espadaMagicaConPorBuff * maestriaEspadaMagica(e) / espadaMagicaMaestriaMax)
}

func applyConDaEspadaMagica(e *world.Entity) {
	con := conDaEspadaMagica(e)
	e.AffCon += clampInt16(con)
	e.AffMaxHP += 2 * con
}
