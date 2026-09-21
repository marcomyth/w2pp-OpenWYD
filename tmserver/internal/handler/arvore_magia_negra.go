package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/combat"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Árvore MAGIA NEGRA da Foema (skills 32-39, maestria Special[2]) — REGRAS DO
// SERVIDOR, decididas pelo Marco em 17/09/2026.
//
// A "FM Magia Negra" (a black) é a Foema com o Inferno (39, a 8ª da árvore)
// aprendido. É o molde do TK Espada Mágica (arvore_espada_magica.go): full INT,
// mesma régua i, mesmo crítico ×2 a ×4 — em área, cada alvo sorteia o seu —, mas
// o bônus vem do cajado e ela rouba MANA em vez de vida, para o Controle de Mana
// (46) seguir segurando o dano. Mata em área, crita, aguenta pela mana.
const (
	skillTrovao  = 37
	skillInferno = 39

	learnedInferno = 1 << 15 // skill 39

	wtypeCajadoDuasMaos = 32

	magiaNegraCajado1MaoPct = 120
	magiaNegraArmaPct       = 100
)

// magiaNegraCajado2MaosPct é o multiplicador do cajado de duas mãos, e é um
// BOTÃO de balanceamento (var, não const).
//
// A Black é de longe a mais forte do elenco: no torneio de 20/09/2026 ela fez
// 64 vitórias e ZERO derrotas em 110 duelos, tirando 4.153/s — o dobro do
// segundo colocado e mais que o dobro da poção. Este é o botão por onde ela
// desce sem mexer no crítico de mago, que ela divide com o TK Espada Mágica.
var magiaNegraCajado2MaosPct = 120

// fmMagiaNegra diz se as regras da árvore valem para e.
func fmMagiaNegra(e *world.Entity) bool {
	return e != nil && e.Class == 1 && world.IsPlayer(e.ID) && e.LearnedSkill&learnedInferno != 0
}

// skillDeDanoDaMagiaNegra são as skills de dano da árvore; o Trovão (37) é o buff,
// e os Relâmpagos que ele solta entram pela skill 33.
func skillDeDanoDaMagiaNegra(skillnum int) bool {
	return skillnum >= 32 && skillnum <= skillInferno && skillnum != skillTrovao
}

// armaPctMagiaNegra: cajado de 2 mãos 140%, cajado de 1 mão (com escudo ou não)
// 120%, qualquer outra arma 100%.
func armaPctMagiaNegra(e *world.Entity, itemAbility func(world.Item, uint8) int) int {
	if itemAbility == nil {
		return magiaNegraArmaPct
	}
	switch wtype := itemAbility(e.Equip[weaponSlotR], efWType); {
	case wtype == wtypeCajadoDuasMaos:
		return magiaNegraCajado2MaosPct
	case wtype == wtypeCajadoUmaMao, itemAbility(e.Equip[weaponSlotL], efWType) == wtypeCajadoUmaMao:
		return magiaNegraCajado1MaoPct
	}
	return magiaNegraArmaPct
}

// magoCritico diz se o golpe de skill sorteia o crítico de mago (TK Espada Mágica
// ou FM Magia Negra, nas skills de dano da própria árvore).
func magoCritico(e *world.Entity, skillnum int) bool {
	return tkEspadaMagica(e) && skillDeDanoDaEspadaMagica(skillnum) ||
		fmMagiaNegra(e) && skillDeDanoDaMagiaNegra(skillnum)
}

// ---------------------------------------------------------------------------
// Roubo de mana: a cada alvo atingido, chance de 30% × i de devolver 25% do dano
// em mana. O lançamento inteiro devolve no máximo 10% da mana máxima, somando os
// alvos — um Inferno em 13 alvos não enche a barra de uma vez.
const rouboDeManaTetoPct = 10

func tetoDoRouboDeMana(e *world.Entity) int32 {
	return effectiveMaxMP(e) * rouboDeManaTetoPct / 100
}

// rouboDeMana devolve quanto o golpe repõe, dentro do que ainda cabe no teto.
func rouboDeMana(r combat.Rand, e *world.Entity, dano int, restante int32) int32 {
	if dano <= 0 || restante <= 0 {
		return 0
	}
	if r.Intn(1000) >= rouboDeVidaChance*reguaDeInt(e)/100 {
		return 0
	}
	return min(int32(dano*rouboDeVidaPct/100), restante)
}

func (d *Dispatcher) reporMana(w *world.World, s *world.Session, e *world.Entity, mana int32) int32 {
	if mana <= 0 || s == nil {
		return 0
	}
	antes := e.MP
	e.MP += mana
	s.ReqMp += mana
	setReqMp(s, e)
	d.sendSetHpMp(w, s, e)
	return e.MP - antes
}
