package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Árvore CONFIANÇA do TransKnight (skills 0-7, maestria Special[1]) — REGRAS DO
// SERVIDOR, decididas pelo Marco em 17/09/2026.
//
// O "TK Confiança" é o TK com o Destino (7, a 8ª da árvore) aprendido. É uma
// build de Destreza e Inteligência, mais Destreza, que bate só com quatro skills
// (Giro da Fúria, Golpe Duplo, Fanatismo e Destino), mais fraca que a Espada
// Mágica, e compensa com esquiva, Constituição e a Aura da Vida. A Destreza dele
// não dá dano físico: só esquiva e dano de skill.
//
// d é a régua da build, parcelaDeForca lida com Destreza no lugar da Força e
// Inteligência no lugar da Destreza: 1000 é Destreza pura (3× a Inteligência),
// 500 meio a meio.
const (
	skillGiroDaFuria = 0
	skillGolpeDuplo  = 2
	skillFanatismo   = 4
	skillDestino     = 7

	learnedDestino = 1 << 7 // skill 7

	confiancaMaestriaMax = 255

	wtypeMachadoUmaMao = 11 // machados e martelos de 1 mão, a Balmung
	wtypeLanca         = 21
	wtypeCajadoUmaMao  = 31 // Neorion, Cajado de Âmbar, Cajado da Jóia Azul
)

// tkConfianca diz se as regras da árvore valem para e.
func tkConfianca(e *world.Entity) bool {
	return e != nil && e.Class == 0 && world.IsPlayer(e.ID) && e.LearnedSkill&learnedDestino != 0
}

// maestriaConfianca é a maestria da árvore, limitada a 255 para as contas.
func maestriaConfianca(e *world.Entity) int {
	return max(0, min(effectiveSpecial(e, 1), confiancaMaestriaMax))
}

func parcelaDeDestreza(e *world.Entity) int {
	return parcelaDeForca(int(effectiveDex(e)), int(effectiveInt(e)))
}

// skillDeDanoDaConfianca são as quatro skills que batem pela régua da árvore.
func skillDeDanoDaConfianca(skillnum int) bool {
	switch skillnum {
	case skillGiroDaFuria, skillGolpeDuplo, skillFanatismo, skillDestino:
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Dano das quatro skills (combat.SkillBaseDamage, ramo Confianca):
//
//	(3 × arma + 3 × (0,6 DES + 0,4 INT) + nível + maestria + valor) × % de dano
//	de skill × arma × 5/4 × 1,15 da 8ª
//
// A arma: cajado de 1 mão com machado ou martelo de 1 mão, 140%; a lança, 140%
// no Mortal e 100% no Arch e no Celestial (o Mortal não equipa cajado — o
// BASE_CanEquip só deixa arma de outra classe para quem não é Mortal); qualquer
// outra arma, 100%.
const (
	confiancaArmaComboPct  = 140
	confiancaLancaMortal   = 140
	confiancaArmaNeutraPct = 100
)

func armaPctConfianca(e *world.Entity, itemAbility func(world.Item, uint8) int) int {
	if itemAbility == nil {
		return confiancaArmaNeutraPct
	}
	r := itemAbility(e.Equip[weaponSlotR], efWType)
	l := itemAbility(e.Equip[weaponSlotL], efWType)
	switch {
	case r == wtypeCajadoUmaMao && l == wtypeMachadoUmaMao, r == wtypeMachadoUmaMao && l == wtypeCajadoUmaMao:
		return confiancaArmaComboPct
	case r == wtypeLanca && e.ClassMaster == classMasterMortal:
		return confiancaLancaMortal
	}
	return confiancaArmaNeutraPct
}

// ---------------------------------------------------------------------------
// Esquiva e perfuração pela Destreza.
//
//	esquiva     = +60% × d (multiplica a esquiva, como a Captura da Huntress)
//	perfuração  = ignora até 25% × d da defesa, só no Fanatismo e no Destino
const (
	confiancaEsquivaPct    = 60
	confiancaPerfuracaoPct = 25
)

func esquivaDaConfianca(e *world.Entity) int {
	return confiancaEsquivaPct * parcelaDeDestreza(e) / 1000
}

// defesaPerfuradaConfianca é a defesa que o Fanatismo e o Destino enfrentam.
func defesaPerfuradaConfianca(e *world.Entity, skillnum, def int) int {
	if def <= 0 || !tkConfianca(e) || (skillnum != skillFanatismo && skillnum != skillDestino) {
		return def
	}
	return def * (100000 - confiancaPerfuracaoPct*parcelaDeDestreza(e)) / 100000
}

// ---------------------------------------------------------------------------
// 4 · Fanatismo: o golpe que acerta tira até 15% da defesa do alvo pela maestria,
// por 16 s (2 ticks). Usar de novo renova o tempo; não acumula. Pega em jogador e
// em monstro. É o afeto 12 (defesa em %), sem sorteio de resistência.
const (
	affectDefesaPct       = 12
	fanatismoDefesaPct    = 15
	fanatismoAffectTime   = 1 // (AffectTime+1) × 100/100 = 2 ticks
	fanatismoAffectDelay  = 100
	fanatismoAffectHostil = 1
)

func debuffDoFanatismo(e *world.Entity) int {
	return fanatismoDefesaPct * maestriaConfianca(e) / confiancaMaestriaMax
}

func (d *Dispatcher) aplicarDebuffDoFanatismo(w *world.World, e, target *world.Entity, tid int) {
	valor := debuffDoFanatismo(e)
	if valor <= 0 || target == nil || (!world.IsPlayer(tid) && target.NonCombatNPC) {
		return
	}
	if imuneADebuff(target, w.Now()) {
		return // Desintoxicar (arvore_magia_branca.go)
	}
	// Renova em vez de acumular: o slot do afeto 12 é reaproveitado pelo SetAffect.
	var applied bool
	if world.IsPlayer(tid) {
		applied = target.SetAffect(affectDefesaPct, valor, fanatismoAffectTime, fanatismoAffectHostil, fanatismoAffectDelay, maestriaConfianca(e), duracaoDoLegado)
	} else {
		applied = target.SetAffectOnMob(affectDefesaPct, valor, fanatismoAffectTime, fanatismoAffectHostil, fanatismoAffectDelay, maestriaConfianca(e), duracaoDoLegado)
	}
	if !applied {
		return
	}
	d.scoreDepoisDoDebuff(w, target, tid)
}

// scoreDepoisDoDebuff refaz a ficha de quem levou um debuff de skill e avisa o
// dono. Em MONSTRO só o score dos afetos é refeito: o refreshScore remonta a
// ficha pelo equipamento e pelos Base*, que no monstro são zero — ele zerava o
// HP máximo do bicho e o matava no lugar de debufá-lo.
func (d *Dispatcher) scoreDepoisDoDebuff(w *world.World, target *world.Entity, tid int) {
	if !world.IsPlayer(tid) {
		d.applyAffectScore(target)
		return
	}
	d.refreshScore(target)
	if ts := w.Session(tid); ts != nil {
		d.sendScore(w, ts, target)
		d.sendAffect(w, ts, target)
	}
}

// ---------------------------------------------------------------------------
// 7 · Destino (passiva): +300 de INT e +500 de CON pela maestria. Como os buffs
// de atributo do jogo, a CON soma o dobro em HP e a INT o dobro em MP.
const (
	destinoInt = 300
	destinoCon = 500
)

func atributosDoDestino(e *world.Entity) (intel, con int32) {
	m := int32(maestriaConfianca(e))
	return destinoInt * m / confiancaMaestriaMax, destinoCon * m / confiancaMaestriaMax
}

// applyPassivasDaConfianca grava as passivas que leem atributo: o Destino e a
// esquiva. Roda no fim do score de afetos, depois da Captura (que zera
// AffEsquivaPct de quem não é Huntress).
func applyPassivasDaConfianca(e *world.Entity) {
	if !tkConfianca(e) {
		return
	}
	intel, con := atributosDoDestino(e)
	e.AffInt += int16(intel)
	e.AffMaxMP += 2 * intel
	e.AffCon += int16(con)
	e.AffMaxHP += 2 * con
	e.AffEsquivaPct = int32(esquivaDaConfianca(e))
}

// ---------------------------------------------------------------------------
// 5 · Aura da Vida do TK Confiança: a cada 5 s, cura até 15% do HP bruto pela
// maestria do cast; 5% enquanto o personagem estiver em PvP (deu ou levou dano
// de jogador nos últimos 10 s). Sem o Destino, a Aura segue a cura do legado a
// cada 8 s (affect_tick.go).
//
// HP bruto é o HP máximo com equipamento e passivas — a CON do Destino entra —
// sem os buffs temporários: a cura não cresce em cima do Samaritano.
const (
	affectAuraDaVida      = 17
	auraConfiancaPeriodo  = 5 // ticks do mundo, 1 s cada
	auraConfiancaPct      = 15
	auraConfiancaPvPPct   = 5
	auraConfiancaJanelaMs = 10_000
)

func hpBrutoConfianca(e *world.Entity) int32 {
	_, con := atributosDoDestino(e)
	return (scoreMaxHP(e) + 2*con) * (e.HpAddPct + 100) / 100
}

func emPvP(e *world.Entity, now uint32) bool {
	return e.UltimoPvP != 0 && now-e.UltimoPvP < auraConfiancaJanelaMs
}

// curaDaAuraConfianca é quanto um tique cura, com level a maestria do cast.
func curaDaAuraConfianca(e *world.Entity, level int, now uint32) int32 {
	pct := int64(auraConfiancaPct)
	if emPvP(e, now) {
		pct = auraConfiancaPvPPct
	}
	m := int64(max(0, min(level, confiancaMaestriaMax)))
	return int32(int64(hpBrutoConfianca(e)) * pct * m / (100 * confiancaMaestriaMax))
}

// marcarPvP anota o golpe de jogador em jogador nos dois lados.
func marcarPvP(attacker, target *world.Entity, now uint32) {
	if now == 0 {
		now = 1 // 0 é "nunca"
	}
	attacker.UltimoPvP = now
	target.UltimoPvP = now
}

// tickAuraDaConfianca cura quem tem a Aura ativa, cada um na sua fase de 5 s.
func (d *Dispatcher) tickAuraDaConfianca(w *world.World) {
	phase := d.tickCount % auraConfiancaPeriodo
	now := w.Now()
	w.ForEachPlayer(func(s *world.Session, e *world.Entity) {
		if s.Conn%auraConfiancaPeriodo != phase || e.HP <= 0 || !tkConfianca(e) {
			return
		}
		for _, af := range e.Affect {
			if af.Type != affectAuraDaVida || af.Time == 0 {
				continue
			}
			d.curarPelaAura(w, s, e, curaDaAuraConfianca(e, int(af.Level), now))
			return
		}
	})
}

func (d *Dispatcher) curarPelaAura(w *world.World, s *world.Session, e *world.Entity, cura int32) {
	if cura <= 0 {
		return
	}
	hp := min(e.HP+cura, effectiveMaxHP(e))
	if hp <= e.HP {
		return
	}
	delta := hp - e.HP
	e.HP = hp
	setReqHp(s, e)
	body := protocol.EncodeSetHpDam(e.HP, delta)
	hdr := protocol.Header{Type: protocol.MsgSetHpDam, ID: uint16(s.Conn)}
	w.SendTo(s, hdr, body)
	w.ForEachInView(s.Conn, func(vs *world.Session, _ *world.Entity) {
		w.SendTo(vs, hdr, body)
	})
}
