package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// pkResetGuilty is the Guilty value set on landing (or receiving) a chaotic PvP
// hit (_MSG_Attack.cpp "PK - War - Miss": SetGuilty(conn,8) / SetGuilty(idx,8)).
const pkResetGuilty uint8 = 8

// guiltyDecayPeriod staggers the per-player Guilty decay the same way
// affectTickPeriod staggers affects (affect_tick.go): the legacy RegenMob runs
// once per connection every ~8 real seconds (Server.cpp RegenMob,
// ProcessSecMinTimer.cpp's Sec16-staggered per-connection cadence), and
// unconditionally decrements Guilty by 1 there. A distinct constant from
// affectTickPeriod on purpose — the two port unrelated legacy fields that only
// coincidentally share a period.
const guiltyDecayPeriod = 8

// pkPointRecoverPeriod is the PKPoint natural-recovery cadence: legacy gates it
// behind a counter that increments once per RegenMob call and fires every 450
// of those (Server.cpp: `if (!(Unk_2736 % 450))`), i.e. 450×8s = 3600s = 1h.
// Global (not per-connection staggered) — an hour-scale gate needs no
// load-spreading, unlike the ~8s Guilty decay.
const pkPointRecoverPeriod = 3600

// PK nickname coloring: the client reads MobName[12] (PKPoint) from a player's
// MSG_CreateMob to color the nick — 75 is NEUTRAL (white), 0 is red/blinking
// (chaos/guilty), <75 is chaos (GetFunc.cpp:1082-1101). This is the real driver,
// NOT _MSG_PKInfo (which only carries the "attackable/war" state).
const (
	pkPointNeutral uint8 = 75 // white nick
	pkPointChaos   uint8 = 0  // red blinking nick
)

// pkPointPerLevel são os Chaos Points pagos por nível subido.
//
// CINCO, E NÃO UM, POR DECISÃO DA HANNA de 25/09/2026, e o teto passou de 75 para 150.
//
// ISTO REVERTE O QUE ESTAVA ESCRITO AQUI. O comentário anterior dizia "divergência
// deliberada do legado, não conserte de volta para paridade sem ler a issue": o #279
// tinha escolhido +1 e teto 75, o mesmo passo e o mesmo teto da recuperação por hora.
// A decisão nova vai ao encontro do LEGADO, que paga +5 por nível até 150
// (SendFunc.cpp:1061, _MSG_Attack.cpp:1791) — é lá que o /cp chega a +75.
//
// O QUE MUDA NA PRÁTICA: quem está em chaos sai dele subindo de nível, e bem mais
// rápido. E, passado o neutro, o excesso vira a faixa 76..150 — uma folga acima do
// branco, que aguenta os próximos atos caóticos antes de o apelido voltar a vermelho.
// Antes o ganho PARAVA no 75 e a faixa de cima só vinha de quest e de item.
//
// Continua empilhando com a recuperação por hora, e não a substitui.
const pkPointPerLevel = 5

// pkGuilty reports whether e is currently in the chaotic (red-blinking-nick)
// state: its Guilty decay counter (GetFunc.cpp KILL_MARK slot) hasn't reached 0.
func pkGuilty(e *world.Entity) bool {
	return e.Guilty > 0
}

// pkPoint is the MobName[12] byte for e's current state (GetFunc.cpp
// GetCreateMob:1094-1099): the real PKPoint counter unless Guilty > 0, in which
// case 0 (chaos/red) regardless of PKPoint. Packed into every player
// MSG_CreateMob and the CNFCharacterLogin blob so the nick renders the right
// color.
func pkPoint(e *world.Entity) uint8 {
	if pkGuilty(e) {
		return pkPointChaos
	}
	return e.PKPoint
}

// pkInfoParm is the S→C PKInfo Parm for e: 1 = PK/attackable state, 0 = clean.
// This drives the "can be attacked / at war" flag, NOT the nick color (that's
// MobName[12]); we still send it for parity with the original login/view flow.
func pkInfoParm(e *world.Entity) int32 {
	if pkGuilty(e) || e.PKMode {
		return 1
	}
	return 0
}

// broadcastPKState re-sends e's current PK state to itself and everyone in view
// whenever it changes (a PvP hit lands, or Guilty decays to 0): a fresh
// MSG_CreateMob (whose MobName[12..15] recolors the nick and refreshes the
// kill-streak bytes — Server.cpp:4933-4941 does exactly this when Guilty hits
// 0) plus the MSG_PKInfo attackable flag.
func (d *Dispatcher) broadcastPKState(w *world.World, s *world.Session, e *world.Entity) {
	mob := protocol.EncodeCreateMobBody(createMobFrom(w, e, 0))
	info := protocol.EncodeStandardParm(pkInfoParm(e))
	send := func(vs *world.Session) {
		w.SendTo(vs, protocol.Header{Type: protocol.MsgCreateMob, ID: protocol.IDScene}, mob)
		w.SendTo(vs, protocol.Header{Type: protocol.MsgPKInfo, ID: uint16(s.Conn)}, info)
	}
	send(s) // the player's own client (recolors the own nick)
	w.ForEachInView(s.Conn, func(vs *world.Session, _ *world.Entity) { send(vs) })
}

// markGuilty sets e's Guilty counter to pkResetGuilty (landing/receiving a
// chaotic PvP hit outside a duel) and, if it wasn't already guilty, re-broadcasts
// the PK state so the nick turns red. s may be nil (e.g. the victim disconnected
// mid-fight) — broadcastPKState needs a session only to address the "own client"
// send, so a nil session just skips that self-send via the guard below.
func (d *Dispatcher) markGuilty(w *world.World, s *world.Session, e *world.Entity) {
	wasGuilty := pkGuilty(e)
	e.Guilty = pkResetGuilty
	if !wasGuilty && s != nil {
		d.broadcastPKState(w, s, e)
	}
}

// grantLevelUpPKPoint paga levels × pkPointPerLevel a cada subida de nível, até o teto
// de 150 que o clampPKPoint já guarda (GetFunc.cpp:2271-2285, a mesma faixa 1..150 do
// SetPKPoint do legado).
//
// ELE JÁ PAROU NO 75, e o teto subiu junto com o passo: a faixa 76..150 deixou de ser
// exclusiva de quest e de item. Quem sobe de nível agora atravessa o neutro e acumula a
// folga, que é o que o legado faz.
//
// The gain is reported once, not per level, so a multi-level jump (Poeira de
// Fada, GM /setlevel) produces a single chat line, and the PK state is
// re-broadcast so the nick recolors without a relog. While Guilty > 0 the visible
// pkPoint(e) is pinned at 0, so the CreateMob would carry the same byte as before
// — skip the broadcast then and let the Guilty decay in sweepGuilty do it.
// s may be nil (the GM/offline applyLevelUps callers).
func (d *Dispatcher) grantLevelUpPKPoint(w *world.World, s *world.Session, e *world.Entity, levels int32) {
	if levels <= 0 {
		return
	}
	// A CONTA EM int ANTES DO uint8, e isto não é preciosismo: levels chega às centenas
	// pelo /setlevel, e 300 × 5 estoura o contador de um byte com folga. Somar em uint8
	// daria a volta e devolveria um número pequeno — um jogador em chaos ganharia menos
	// que um que subiu um nível.
	//
	// Quem guarda o teto é o clampPKPoint, o mesmo do resto do arquivo: uma segunda
	// conta de teto aqui é como as duas ficam diferentes no dia em que uma mudar.
	antes := e.PKPoint
	e.PKPoint = clampPKPoint(int(e.PKPoint) + int(levels)*pkPointPerLevel)
	gain := int(e.PKPoint) - int(antes)
	// JÁ ESTAVA NO TETO: não há o que dizer nem o que redesenhar, e uma linha de chat
	// dizendo "+0" em cada nível seria barulho em quem já está branco.
	if gain <= 0 {
		return
	}
	if s == nil {
		return
	}
	d.sendChatText(w, s, notifyPKPointDelta(e.PKPoint, gain))
	if !pkGuilty(e) {
		d.broadcastPKState(w, s, e)
	}
}

// pkPointPardon is what the Pergaminho do Perdão restores. The legacy calls
// SetPKPoint(conn, 150) and SetPKPoint clamps to its own 1..150 range
// (GetFunc.cpp:2271-2285), so 150 is the value, not an overflow: it is the top of
// the 76..150 band grantLevelUpPKPoint already documents as belonging to the
// quest/item paths. That band is a buffer above the neutral 75 — the nick is
// white and stays white through the next chaotic acts until the counter falls
// back under 75.
const pkPointPardon uint8 = 150

// usePerdaoScroll consumes the Pergaminho do Perdão (item 3343, EF_VOLATILE 203):
// it wipes the chaos counter and re-broadcasts the entity so the nick recolors
// without a relog (_MSG_UseItem.cpp:3865-3887).
//
// Until now EF_VOLATILE 203 had no case at all, so the scroll fell through to
// rejectUnimplementedConsumable and answered _NN_Cant_Use_That_Here — "not
// available in this area", which reads as a place problem and sent players
// hunting for the right spot.
//
// Guilty is deliberately NOT cleared, matching the original: the red-blinking
// nick comes from the Guilty counter, which pkPoint() pins to 0 while it runs and
// sweepGuilty decays on its own within about a minute. Clearing it here would
// hand a fresh killer an instant laundering of the PvP flag, which is a different
// item's job.
func (d *Dispatcher) usePerdaoScroll(w *world.World, s *world.Session, e *world.Entity, src int) {
	before := e.PKPoint
	e.PKPoint = pkPointPardon
	consumeOneItem(&e.Carry[src])
	d.sendSlot(w, s, world.ItemPlaceCarry, src, e.Carry[src])
	// While Guilty > 0 the visible byte is pinned at 0, so the CreateMob would
	// carry what it already carried — same skip grantLevelUpPKPoint makes.
	if !pkGuilty(e) {
		d.broadcastPKState(w, s, e)
	}
	sendClientMessage(w, s, msgPerdaoDone)
	d.log.Info("perdao scroll used", "conn", s.Conn, "account", s.AccountName,
		"pkpoint_before", before, "pkpoint_after", e.PKPoint, "guilty", e.Guilty)
}

// msgPerdaoDone confirms the pardon. The scroll otherwise finishes in silence
// whenever Guilty is still running, since the nick cannot recolor yet.
const msgPerdaoDone = "Seus pontos de caos foram perdoados."

// pkMode handles _MSG_PKMode (0x0399): the client's K-key Player-Killer consent
// toggle (Exec_MSG_PKMode, _MSG_PKMode.cpp). Toggling only flips the consent gate
// checked by attack() — it cancels any active trade (same anti-dup guard as
// legacy's OpponentID check) and echoes the current PKInfo state. Pressing K does
// NOT by itself blink the nickname (that needs a landed PvP hit → guilty).
func (d *Dispatcher) pkMode(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	if s.Mode != world.UserPlay {
		return
	}
	e := w.Entity(s.Conn)
	if e == nil {
		return
	}
	parm, _ := protocol.StandardParm(payload)
	e.PKMode = parm != 0

	d.removeTrade(w, s)
	// Only the attackable flag (PKInfo) changes on a K toggle — the nick color
	// (MobName[12]) is unaffected, so no CreateMob re-broadcast is needed.
	info := protocol.EncodeStandardParm(pkInfoParm(e))
	w.SendTo(s, protocol.Header{Type: protocol.MsgPKInfo, ID: uint16(s.Conn)}, info)
	w.ForEachInView(s.Conn, func(vs *world.Session, _ *world.Entity) {
		w.SendTo(vs, protocol.Header{Type: protocol.MsgPKInfo, ID: uint16(s.Conn)}, info)
	})
}

// sweepGuilty is the RegenMob port for Guilty decay + PKPoint recovery
// (Server.cpp RegenMob): Guilty ticks down by 1 on each player's ~8s stagger
// phase (only for currently-connected players — RegenMob never runs for an
// offline connection, so decay correctly pauses across logout with no extra
// bookkeeping), and PKPoint recovers +1 toward neutral (75) on a global hourly
// gate. Unchanged call site (mobai.go Tick()).
func (d *Dispatcher) sweepGuilty(w *world.World) {
	phase := d.tickCount % guiltyDecayPeriod
	pkTick := d.tickCount%pkPointRecoverPeriod == 0
	w.ForEachPlayer(func(s *world.Session, e *world.Entity) {
		if s.Conn%guiltyDecayPeriod == phase && e.Guilty > 0 {
			e.Guilty--
			if e.Guilty == 0 {
				d.broadcastPKState(w, s, e)
			}
		}
		if pkTick && e.PKPoint < pkPointNeutral {
			e.PKPoint++
			d.sendChatText(w, s, notifyPKPointDelta(e.PKPoint, 1))
			// The recovered point changes MobName[12], so the nick has to be
			// repainted here too — otherwise it keeps the stale color until the
			// next CreateMob (a relog, or leaving and re-entering someone's view).
			// Pinned at 0 while Guilty > 0, so nothing to repaint in that case.
			if !pkGuilty(e) {
				d.broadcastPKState(w, s, e)
			}
		}
		// The same hourly RegenMob gate charges the mount's ração
		// (Server.cpp:4895, inside the Unk_2736 % 450 block).
		if pkTick {
			d.tickMountFeed(w, s, e)
		}
	})
}
