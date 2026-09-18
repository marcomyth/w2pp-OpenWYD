package handler

import "github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"

// Árvore TRANS do TransKnight (skills 8-15, maestria Special[2]) — REGRAS DO
// SERVIDOR, desenhadas com o Marco em 17/09/2026 (física).
//
// O "TK Trans" é o TK com a Armadura Crítica (15, a 8ª da árvore) aprendida. Um
// personagem só aprende uma 8ª, então ele nunca é também TK Confiança. É o
// porradeiro: Força acima da Destreza, tanque e batedor, o "sicário de HT" — o
// golpe dele não sofre a esquiva da Huntress — e um lutador competente contra
// as outras classes. A Destreza serve para a velocidade de ataque e um pouco de
// esquiva; nada novo lê a Destreza dele.
//
// m é a maestria da árvore (0..255) e f a régua parcelaDeForca (1000 = Força pura,
// 3× a Destreza).
const (
	learnedNocaoDeCombate  = 1 << 14 // skill 14
	learnedArmaduraCritica = 1 << 15 // skill 15

	transMaestriaMax = 255

	wtypeEspadaDuasMaos  = 3  // Luna, Solaris, Éden, Tsurugi... e as Anct
	wtypeMarteloDuasMaos = 13 // Martelo Psíquico, Basileus, Demolidor Celestial, os machados de 2 mãos...
)

// tkTrans diz se as regras da 8ª da árvore valem para e.
func tkTrans(e *world.Entity) bool {
	return e != nil && e.Class == 0 && world.IsPlayer(e.ID) && e.LearnedSkill&learnedArmaduraCritica != 0
}

// maestriaTrans é a maestria da árvore, limitada a 255 para as contas.
func maestriaTrans(e *world.Entity) int {
	return max(0, min(effectiveSpecial(e, 2), transMaestriaMax))
}

func parcelaDoTrans(e *world.Entity) int {
	return parcelaDeForca(int(effectiveStr(e)), int(effectiveDex(e)))
}

// ---------------------------------------------------------------------------
// 15 · Armadura Crítica.
//
// Crítico: até +10% na janela C × m × f, no lugar do legado (Special[3]/10 +
// DES/75, mínimo 4), que lia a Destreza que o porradeiro não tem. A janela
// mostra o byte × 0,4%, então 10% são 25.
//
// Defesa: +10% da defesa plana (o legado) mais até +10% × m.
//
// HP: até +15% × f.
const (
	armaduraCriticoMax    = 25  // byte do crítico
	armaduraDefesaBase    = 100 // ‰ da defesa plana
	armaduraDefesaMaestri = 100 // ‰ × m
	armaduraHPForca       = 150 // ‰ × f
)

func criticoDaArmadura(e *world.Entity) int {
	return armaduraCriticoMax * maestriaTrans(e) * parcelaDoTrans(e) / (transMaestriaMax * 1000)
}

func defesaDaArmadura(e *world.Entity, flatAC int32) int32 {
	permil := int32(armaduraDefesaBase + armaduraDefesaMaestri*maestriaTrans(e)/transMaestriaMax)
	return flatAC * permil / 1000
}

// ignoraEsquivaDaHT: o golpe do TK Trans não sofre a esquiva da Huntress, tenha
// ela quanta tiver — físico e skill. Contra as outras classes a esquiva segue.
func ignoraEsquivaDaHT(attacker, target *world.Entity) bool {
	return tkTrans(attacker) && target != nil && target.Class == 3 && world.IsPlayer(target.ID)
}

// ---------------------------------------------------------------------------
// Armas de 2 mãos do TK Trans, pelo EF_WTYPE (as Anct herdam o tipo):
//
//	espada de 2 mãos: até +20% de dano × m
//	martelo de 2 mãos: até +15% de HP e +10% de crítico na janela × m
const (
	espadaDanoPct      = 20
	marteloHPPermil    = 150
	marteloCriticoByte = 25
)

// ---------------------------------------------------------------------------
// 14 · Noção de Combate (bit 14, qualquer TK que a aprenda):
//
//	piso do sorteio: Special[2]/20 (teto 15), agora também no golpe normal
//	esquiva: até +10% × m
//	acerto: tira até 30% × m da esquiva do alvo — rende mais contra quem tem
//	  mais Destreza, que é quem esquiva mais
const (
	nocaoEsquivaPct   = 10
	nocaoAcertoPermil = 300
	nocaoPisoTeto     = 15
)

func temNocaoDeCombate(e *world.Entity) bool {
	return e != nil && e.Class == 0 && e.LearnedSkill&learnedNocaoDeCombate != 0
}

// pisoDaNocao é o combat do sorteio de dano (combat.Damage/SkillDamage) que a
// Noção de Combate dá; 0 sem ela.
func pisoDaNocao(e *world.Entity) int {
	if !temNocaoDeCombate(e) {
		return 0
	}
	return max(0, min(int(e.Special[2])/20, nocaoPisoTeto))
}

// masterDoGolpe é o combat do golpe normal: o do legado ou o piso da Noção.
func masterDoGolpe(e *world.Entity) int {
	return max(e.Master, pisoDaNocao(e))
}

func maestriaDaNocao(e *world.Entity) int {
	if !temNocaoDeCombate(e) {
		return 0
	}
	return maestriaTrans(e)
}

// esquivaComAcerto aplica a esquiva final do alvo contra este atacante: nenhuma
// se o atacante é TK Trans e o alvo é HT; senão a esquiva menos o acerto da Noção.
func esquivaComAcerto(parry int, attacker, target *world.Entity) int {
	if ignoraEsquivaDaHT(attacker, target) {
		return 0
	}
	if m := maestriaDaNocao(attacker); m > 0 {
		parry = parry * (1000 - nocaoAcertoPermil*m/transMaestriaMax) / 1000
	}
	return parry
}

// ---------------------------------------------------------------------------

// hpDoTransPermil é o HP a mais do TK Trans, em milésimos do HP máximo: a
// Armadura Crítica pela Força e o martelo de 2 mãos pela maestria.
func hpDoTransPermil(e *world.Entity, wtype int) int {
	if !tkTrans(e) {
		return 0
	}
	permil := armaduraHPForca * parcelaDoTrans(e) / 1000
	if wtype == wtypeMarteloDuasMaos {
		permil += marteloHPPermil * maestriaTrans(e) / transMaestriaMax
	}
	return permil
}

// applyPassivasDoTrans grava as passivas da árvore que dependem da arma e da
// maestria. Roda no fim do score de afetos, depois da Captura e da Confiança
// (que gravam AffEsquivaPct): a esquiva da Noção soma por cima.
func applyPassivasDoTrans(e *world.Entity, itemAbility func(world.Item, uint8) int) {
	if !world.IsPlayer(e.ID) || e.Class != 0 {
		return
	}
	if m := maestriaDaNocao(e); m > 0 {
		e.AffEsquivaPct += int32(nocaoEsquivaPct * m / transMaestriaMax)
	}
	if !tkTrans(e) {
		return
	}
	wtype := 0
	if itemAbility != nil {
		wtype = itemAbility(e.Equip[weaponSlotR], efWType)
	}
	m := maestriaTrans(e)
	switch wtype {
	case wtypeEspadaDuasMaos:
		e.AffDamageMultiPct += int32(espadaDanoPct * m / transMaestriaMax)
	case wtypeMarteloDuasMaos:
		e.AffCritical += int16(marteloCriticoByte * m / transMaestriaMax)
	}
	e.AffMaxHP += scoreMaxHP(e) * int32(hpDoTransPermil(e, wtype)) / 1000
}

// ---------------------------------------------------------------------------
// 15 · Armadura Crítica: SICÁRIO DE HT. O golpe do TK Trans numa Huntress
// (jogadora) ganha dano extra — físico e skill —, e SÓ nela: contra as outras
// classes o PvP dele segue sem o bônus. Entra logo depois da % de PvP do painel.
//
// +250% é provisório (17/09): com o corte de 37% e poção, é o menor valor em que
// o Trans de espada vence a HT das três 8ªs. Recalibrar depois do corte de dano
// da HT e da regra da poção em PvP. É var só para a varredura da simulação.
var transContraHTPct = 250 // % a mais

func danoDoTransContraHT(attacker, target *world.Entity, dmg int) int {
	return danoContraHTComPct(attacker, target, dmg, transContraHTPct)
}

func danoContraHTComPct(attacker, target *world.Entity, dmg, pct int) int {
	if dmg <= 0 || !ignoraEsquivaDaHT(attacker, target) {
		return dmg
	}
	return dmg * (100 + pct) / 100
}
