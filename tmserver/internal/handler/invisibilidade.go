package handler

import (
	"fmt"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/combat"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Invisibilidade (Huntress, skill 95) — REGRA DO SERVIDOR, decidida pelo Marco em
// 16/09/2026. Não é o legado, e não tenta ser.
//
// No legado a skill liga RSV_HIDE por 1-3 ticks e some quando a HT ataca ou é
// atingida (DoRemoveHide, _MSG_Attack.cpp:272/315). O port nunca trouxe o
// DoRemoveHide, e o que sobrou foi um exploit: invisível por um minuto, batendo
// em monstro que não revida, porque a lista de inimigos recusa alvo escondido.
//
// A regra agora:
//   - fica invisível por 10 s, e a skill só pode ser lançada de novo 80 s depois
//     do último cast;
//   - o PRÓXIMO ATAQUE FÍSICO sai multiplicado por 2, 3 ou 4, sorteado
//     (rolarGolpeFurtivo), e esse ataque a revela — acertando ou errando;
//   - skill agressiva também a revela, mas sem multiplicar; buff não revela;
//   - ser atingida (jogador ou monstro) a revela e perde o bônus.
const (
	skillInvisibilidade  = 95
	affectInvisibilidade = 28

	invisDuracaoMs = 10_000
	invisRecargaMs = 80_000

	// invisIconTicks é o tempo gravado no slot do afeto, em ticks de 8 s. Não é a
	// duração: quem encerra a invisibilidade é sweepInvisibilidade, pelo relógio
	// da entidade. O slot só precisa sobreviver aos 10 s — com 2 ticks a varredura
	// de afetos poderia apagá-lo aos 8 s, se o cast caísse logo antes da fase do
	// jogador; com 3, o mínimo é 16 s.
	invisIconTicks = 3

	// A chance de X3 é fixa; a de X4 vem da Força e a de X2 é o resto.
	invisChanceX3 = 30
	// invisChanceX4Max é o teto do X4, alcançado só por Força pura (Destreza 0).
	invisChanceX4Max = 50
)

// personagem identifica um personagem por (conta, slot). O nome não serve: Arch
// e Celestial herdam o nome do Mortal.
type personagem struct {
	conta int64
	slot  int
}

// chanceGolpeFurtivoX4 é a chance, em %, de o golpe furtivo sair X4.
//
// A Invisibilidade é uma skill de Força: conta a parcela de Força em Força+Destreza,
// e não o valor bruto, para que a curva não dependa da escala de atributos do
// servidor. Com Destreza igual ou maior que a Força o personagem é build de
// Destreza, e o golpe nunca passa de X2.
//
//	X4 = 50 × (STR − DEX) ÷ (STR + DEX)   — que é (STR÷(STR+DEX) − 0,5) × 100
//
// STR 1500 / DEX 1000 → 10%; STR 3000 / DEX 1000 → 25%; Força pura → 50%.
func chanceGolpeFurtivoX4(str, dex int) int {
	if dex < 0 {
		dex = 0
	}
	if str <= dex {
		return 0
	}
	c := invisChanceX4Max * (str - dex) / (str + dex)
	if c > invisChanceX4Max {
		c = invisChanceX4Max
	}
	return c
}

// rolarGolpeFurtivo sorteia o multiplicador do golpe que sai da invisibilidade.
// Build de Destreza é sempre X2 e não gasta sorteio.
func rolarGolpeFurtivo(r combat.Rand, str, dex int) int {
	x4 := chanceGolpeFurtivoX4(str, dex)
	if x4 == 0 {
		return 2
	}
	switch roll := r.Intn(100); {
	case roll < x4:
		return 4
	case roll < x4+invisChanceX3:
		return 3
	default:
		return 2
	}
}

// invisRecargaRestante devolve quanto falta, em ms, para a Invisibilidade poder
// ser lançada de novo por este personagem.
//
// A recarga fica no Dispatcher, por (conta, slot), e não na sessão nem na
// entidade: as duas nascem de novo a cada login, e sair e entrar zeraria os 80 s.
func (d *Dispatcher) invisRecargaRestante(s *world.Session, now uint32) uint32 {
	desde, ok := d.invisRecarga[personagem{s.AccountID, s.Slot}]
	if !ok {
		return 0
	}
	// Subtração em uint32: atravessa a volta do relógio de 49 dias sem erro.
	if passou := now - desde; passou < invisRecargaMs {
		return invisRecargaMs - passou
	}
	return 0
}

// armarInvisibilidade liga o relógio de 10 s e a recarga, no momento em que o
// afeto da skill 95 acaba de ser instalado em e.
func (d *Dispatcher) armarInvisibilidade(w *world.World, e *world.Entity) {
	now := w.Now()
	e.InvisivelDesde = now
	e.InvisivelAtiva = true
	for i := range e.Affect {
		if e.Affect[i].Type == affectInvisibilidade {
			e.Affect[i].Time = invisIconTicks
			break
		}
	}
	if s := w.Session(e.ID); s != nil {
		d.marcarRecargaInvis(s, now)
	}
}

// marcarRecargaInvis começa a contar os 80 s de recarga do personagem de s.
func (d *Dispatcher) marcarRecargaInvis(s *world.Session, now uint32) {
	if d.invisRecarga == nil {
		d.invisRecarga = map[personagem]uint32{}
	}
	d.invisRecarga[personagem{s.AccountID, s.Slot}] = now
}

// revelarInvisivel tira a invisibilidade de e e avisa o cliente. Devolve false
// quando e não estava invisível.
func (d *Dispatcher) revelarInvisivel(w *world.World, e *world.Entity) bool {
	tinha := e.ClearFirstAffect(affectInvisibilidade)
	ativa := e.InvisivelAtiva
	e.InvisivelAtiva = false
	if !tinha {
		return ativa
	}
	d.refreshScore(e)
	if s := w.Session(e.ID); s != nil {
		d.sendScore(w, s, e)
		d.sendAffect(w, s, e)
	}
	return true
}

// sweepInvisibilidade encerra, a cada tick de 1 s, a invisibilidade que passou dos
// 10 s. Também revela quem tem o afeto sem o relógio ligado — o afeto que voltou
// salvo de um relog —, que de outro modo ficaria invisível até o slot expirar.
func (d *Dispatcher) sweepInvisibilidade(w *world.World) {
	now := w.Now()
	w.ForEachPlayer(func(_ *world.Session, e *world.Entity) {
		d.expirarInvisibilidade(w, e, now)
	})
}

// expirarInvisibilidade é a decisão de sweepInvisibilidade para um personagem.
func (d *Dispatcher) expirarInvisibilidade(w *world.World, e *world.Entity, now uint32) {
	if !e.InvisivelAtiva && !e.HasAffect(affectInvisibilidade) {
		return
	}
	if e.InvisivelAtiva && e.HasAffect(affectInvisibilidade) && now-e.InvisivelDesde < invisDuracaoMs {
		return
	}
	d.revelarInvisivel(w, e)
}

// sairDaInvisibilidadeAoAtacar é o começo de todo ataque de um personagem
// invisível. Ataque físico sorteia o multiplicador do golpe furtivo e devolve-o;
// skill agressiva revela sem multiplicar (devolve 0); buff não revela.
//
// A revelação vem ANTES do laço de dano de propósito: o monstro atingido precisa
// poder pôr a HT na lista de inimigos (addEnemyList recusa alvo escondido), senão
// ela continua batendo em quem não revida.
func (d *Dispatcher) sairDaInvisibilidadeAoAtacar(w *world.World, e *world.Entity, cast castInfo) int {
	if !e.InvisivelAtiva && !e.HasAffect(affectInvisibilidade) {
		return 0
	}
	if cast.isSkill {
		if cast.spell.Aggressive != 0 {
			d.revelarInvisivel(w, e)
		}
		return 0
	}
	mult := rolarGolpeFurtivo(w.Rand(), int(effectiveStr(e)), int(effectiveDex(e)))
	d.revelarInvisivel(w, e)
	return mult
}

// msgInvisRecarga é a recusa do cast em recarga. O cliente mostra o próprio tempo
// de recarga, tirado do SkillData dele, e não sabe dos 80 s — sem esta linha o
// clique cedo demais seria silêncio.
const msgInvisRecarga = "Invisibilidade em recarga: faltam %d s."

// msgGolpeFurtivo avisa o multiplicador que saiu no golpe.
const msgGolpeFurtivo = "Golpe furtivo: dano x%d."

func textoRecargaInvis(faltaMs uint32) string {
	return fmt.Sprintf(msgInvisRecarga, (faltaMs+999)/1000)
}
