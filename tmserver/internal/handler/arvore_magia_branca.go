package handler

import "github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"

// Árvore MAGIA BRANCA da Foema (skills 24-31, maestria Special[1]) — REGRAS DO
// SERVIDOR, decididas pelo Marco em 17/09/2026.
//
// A "FM Magia Branca" é a Foema com o Renascimento (31, a 8ª da árvore)
// aprendido. É suporte de ponta a ponta e escala com a CONSTITUIÇÃO: quanto mais
// vida ela tem, mais cura, melhor ressuscita e mais forte fica o Julgamento
// Divino. O cajado de 1 mão soma 40% na cura.
//
// A vida que conta para a cura para no teto da evolução (30 mil no Mortal, 40 mil
// no Arch, 50 mil do Celestial em diante), para a CON não virar cura infinita.
const (
	skillFlechaMagica    = 24
	skillDesintoxicar    = 25
	skillFlash           = 26
	skillChoqueDivino    = 28
	skillJulgamento      = 30
	learnedRenascimento  = 1 << 7 // skill 31
	brancaMaestriaMax    = 255
	brancaVidaTetoMortal = 30_000
	brancaVidaTetoArch   = 40_000
	brancaVidaTetoCeles  = 50_000
)

// fmMagiaBranca diz se as regras da árvore valem para e.
func fmMagiaBranca(e *world.Entity) bool {
	return e != nil && e.Class == 1 && world.IsPlayer(e.ID) && e.LearnedSkill&learnedRenascimento != 0
}

// vidaQueConta é o HP máximo da FM limitado ao teto da evolução dela.
func vidaQueConta(e *world.Entity) int32 {
	teto := int32(brancaVidaTetoCeles)
	switch e.ClassMaster {
	case classMasterMortal, 0: // 0 é a ficha antiga, tratada como Mortal (score_derive.go)
		teto = brancaVidaTetoMortal
	case classMasterArch:
		teto = brancaVidaTetoArch
	}
	return min(effectiveMaxHP(e), teto)
}

// ---------------------------------------------------------------------------
// Cura pela vida: a Cura (27) soma 10% da vida que conta, e o Recuperar (29)
// soma 6% em cada alvo. Com a 8ª os tetos sobem para 2.500 e 1.800; sem ela
// valem os do legado. O cajado de 1 mão soma 40%, antes do teto, como o Amuleto
// dos Amantes.
const (
	curaVidaPct       = 10
	recuperarVidaPct  = 6
	curaTeto8a        = 2500
	recuperarTeto8a   = 1800
	brancaCajadoPct   = 140
	brancaArmaNeutra  = 100
	brancaImunidadeMs = 4000
)

// curaDaMagiaBranca soma a parte que vem da vida da FM ao valor bruto da skill.
func curaDaMagiaBranca(e *world.Entity, skillnum, heal int, itemAbility func(world.Item, uint8) int) int {
	if !fmMagiaBranca(e) {
		return heal
	}
	switch skillnum {
	case skillCura:
		heal += int(vidaQueConta(e)) * curaVidaPct / 100
	case skillRecuperar:
		heal += int(vidaQueConta(e)) * recuperarVidaPct / 100
	default:
		return heal
	}
	return heal * curaPctDaArma(e, itemAbility) / 100
}

// curaPctDaArma: cajado de 1 mão 140%, qualquer outra arma 100%. itemAbility nulo
// cai no Dispatcher, porque quem chama a cura já tem o catálogo.
func curaPctDaArma(e *world.Entity, itemAbility func(world.Item, uint8) int) int {
	if itemAbility == nil {
		return brancaArmaNeutra
	}
	if itemAbility(e.Equip[weaponSlotR], efWType) == wtypeCajadoUmaMao ||
		itemAbility(e.Equip[weaponSlotL], efWType) == wtypeCajadoUmaMao {
		return brancaCajadoPct
	}
	return brancaArmaNeutra
}

// tetoDaCura é o teto do valor curado: com a 8ª, 2.500 na Cura e 1.800 no
// Recuperar; sem ela, o do legado que o chamador já trazia.
func tetoDaCura(e *world.Entity, skillnum, tetoLegado int) int {
	if !fmMagiaBranca(e) {
		return tetoLegado
	}
	switch skillnum {
	case skillCura:
		return curaTeto8a
	case skillRecuperar:
		return recuperarTeto8a
	}
	return tetoLegado
}

// ---------------------------------------------------------------------------
// 30 · Julgamento Divino: o "tudo ou nada" dela. Gasta 70% da vida atual, soma o
// DOBRO disso ao golpe e só acerta 30% das vezes. Sem a 8ª segue o legado (soma a
// vida inteira e fica com um sexto).
const (
	julgamentoCustoPct  = 70
	julgamentoDanoMult  = 2
	julgamentoAcertoPct = 30
)

// custoDoJulgamento devolve quanto de vida a FM gasta e quanto isso soma ao golpe.
func custoDoJulgamento(e *world.Entity) (gasto, dano int32) {
	gasto = e.HP * julgamentoCustoPct / 100
	return gasto, gasto * julgamentoDanoMult
}

func julgamentoAcerta(r interface{ Intn(int) int }) bool {
	return r.Intn(100) < julgamentoAcertoPct
}

// ---------------------------------------------------------------------------
// 31 · Renascimento em até três: o alvo clicado volta sempre, e cada um dos dois
// mortos mais próximos (até 3 casas dele) tem 10% de chance. Sem a 8ª segue o
// legado: um alvo, 70% de chance. A vida com que o morto volta é a do legado —
// 10% a 19% do HP máximo da FM —, que já cresce com a CON dela.
const (
	renascimentoAlcance      = 3
	renascimentoExtras       = 2
	renascimentoChanceExtra  = 10
	renascimentoChanceLegado = 70
)

// ---------------------------------------------------------------------------
// 25 · Desintoxicar: limpa os debuffs e, com a 8ª, segura debuff novo por 4 s.
func marcarImunidadeADebuff(caster, target *world.Entity, now uint32) {
	if !fmMagiaBranca(caster) || target == nil || !world.IsPlayer(target.ID) {
		return
	}
	target.ImuneDebuffAte = now + brancaImunidadeMs
}

// imuneADebuff diz se o alvo está dentro da janela do Desintoxicar.
func imuneADebuff(target *world.Entity, now uint32) bool {
	return target != nil && target.ImuneDebuffAte != 0 && now < target.ImuneDebuffAte
}

// tirarDaMira solta o alvo da mira de quem está por perto: todo jogador na volta
// que estava mirando nele perde o alvo. É o Flash (26) valendo em PvP.
func (d *Dispatcher) tirarDaMira(w *world.World, target *world.Entity, tid int) {
	w.ForEachInView(tid, func(_ *world.Session, outro *world.Entity) {
		if outro != nil && outro.Target == tid {
			outro.Target = 0
		}
	})
}

// mortosPertoDoAlvo devolve até dois mortos a até 3 casas do alvo, para o
// Renascimento em três. Só jogadores, e nunca o próprio alvo.
func (d *Dispatcher) mortosPertoDoAlvo(w *world.World, target *world.Entity, tid int) []int {
	var ids []int
	w.ForEachInView(tid, func(_ *world.Session, outro *world.Entity) {
		if len(ids) >= renascimentoExtras || outro == nil || outro.ID == tid || outro.HP > 0 {
			return
		}
		if !world.IsPlayer(outro.ID) || mobDistance(outro.X, outro.Y, target.X, target.Y) > renascimentoAlcance {
			return
		}
		ids = append(ids, outro.ID)
	})
	return ids
}
