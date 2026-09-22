package handler

import "github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"

// The fairy that walks a party through the Água (a server rule decided
// 2026-09-12; the legacy has nothing like it).
//
// Clearing a room hands the leader the next scroll and stops there: somebody has
// to open the bag and use it, once per room, eight times a run. With a Fada Verde
// (the XP one) or a Fada Vermelha in the leader's fairy slot the chain walks
// itself — a few seconds after the room falls the whole party is moved into the
// next one, and on to the boss after the last numbered room.
//
// The scroll that ride replaces is NOT minted: it is the same reward, spent as a
// ride instead of handed over, and printing it too would turn one fairy into an
// endless supply of entries. It is handed over on every path that fails, so a
// party that cannot be moved is never left with nothing.
//
// A ride whose next room is TAKEN waits for it (2026-09-20). It used to hand the
// scroll back once the cleared room's countdown ran out, which from the player's
// chair was the fairy freezing in the middle of a run — and the scroll it handed
// back walked into the same occupied room and was refused through a notice this
// client never draws. See fadaEsperaSalaLivre.
//
// The Fada Azul (3901, 3904, 3907) is deliberately out — it was not asked for.

const (
	// fadaEsperaNaAgua is the pause between the last monster dying and the party
	// being moved, in 1s ticks.
	//
	// Three seconds, not five: there is nothing on the floor to pick up — mob
	// loot is delivered straight into the killer's bag (putMobDrop) — so the
	// pause only has to read as a beat between rooms rather than a teleport in
	// the middle of the fight.
	fadaEsperaNaAgua = 3

	// Waiting for a busy room only lasts if the party is not thrown out from
	// under it: the room it is standing in was cut to 30s when it was cleared,
	// and that countdown expiring teleports everyone to the dungeon entrance. So
	// a waiting ride keeps its own room topped up.
	//
	// fadaPisoDaEspera is how low that countdown may fall before it is refilled
	// to fadaRecargaDaEspera, both in the water's 2-second units. Refilling at a
	// floor instead of every tick keeps it at one MSG_StartTime per 20 seconds
	// rather than one per second.
	fadaPisoDaEspera    = 5
	fadaRecargaDaEspera = waterRoomClearTime

	// fadaAvisoDeEspera is how often a waiting party is told it is still waiting,
	// in 1s ticks. Silence is exactly what made the old give-up read as a freeze.
	fadaAvisoDeEspera = 10
)

// fadaLevaNaAgua reports whether a fairy carries the party through the chain:
// the Verde family (which includes the Suprema, 3913) and the Vermelha. The
// indices are the ones fairyExpBonus already pays experience for, minus the Azul
// and the Verde-Azul.
func fadaLevaNaAgua(idx int16) bool {
	switch idx {
	case 3900, 3903, 3906, 3911, 3912, 3913: // Fada Verde, incl. a Suprema
		return true
	case 3902, 3905, 3908: // Fada Vermelha
		return true
	}
	return false
}

// avancoDaFada is one party waiting to be moved on.
type avancoDaFada struct {
	variant   int // which chain (N, M or A)
	room      int // the room just cleared — also the room whose scroll is owed
	leader    int // entity id of the party leader
	espera    int // ticks left before the first attempt
	esperando int // ticks spent waiting for the next room to empty
}

// proximaSalaDaAgua is the room the chain moves to. The last numbered room (7,
// the one whose reward is the Evocação Neses) leads to the boss, the boss starts
// the chain over at Sala 1, and the dead room 8 is never entered.
func proximaSalaDaAgua(room int) int {
	switch {
	case room >= waterDeadRoom:
		return 0 // the boss is down: run it again
	case room == waterDeadRoom-1:
		return waterBossRoom
	}
	return room + 1
}

// pergaDaBolsaParaRecomecar finds the scroll that pays for a lap after the boss,
// and the room it opens.
//
// Every ride between numbered rooms is paid by the reward that was not minted
// (see the note at the top of this file). A lap after the boss has no reward
// behind it — the boss pays loot, not a scroll — so it spends a real scroll out
// of the bag, and running out of them is what ends the cycle.
//
// The LOWEST room available is taken, so the run restarts as early as the bag
// allows instead of jumping to whatever deep scroll is lying around. Slots the
// bag has not unlocked are not touched: charging one would consume an item the
// player could not have used himself.
func pergaDaBolsaParaRecomecar(vols map[int]int, e *world.Entity, variant int) (sala, slot int, ok bool) {
	sala, slot = 0, -1
	limite := activeCarryLimit(e)
	for i := 0; i < limite; i++ {
		it := e.Carry[i]
		if it.Empty() {
			continue
		}
		v, r, pergaminho := waterRoomForVolatile(vols[int(it.Index)])
		if !pergaminho || v != variant {
			continue
		}
		if slot < 0 || r < sala {
			sala, slot = r, i
		}
	}
	return sala, slot, slot >= 0
}

// agendarAvancoDaFada queues the ride, and reports whether it took it. False
// means the caller keeps the old behaviour and hands out the scroll.
func (d *Dispatcher) agendarAvancoDaFada(w *world.World, leader *world.Entity, variant, room int) bool {
	if leader == nil {
		return false
	}
	if fada := leader.Equip[fairyEquipSlot].Index; !fadaLevaNaAgua(fada) {
		// Logged and not announced: most parties clear rooms with no fairy at all,
		// and a panel line on every clear would be noise. It is the one refusal
		// that happens BEFORE a ride is queued, so without this line a fairy
		// carried in the bag instead of the slot left no trace anywhere.
		d.log.Info("fairy ride not scheduled: no carrying fairy in the slot",
			"leader", leader.Name, "variant", variant, "room", room, "fairy_slot_item", fada)
		return false
	}
	s := w.Session(leader.ID)
	if s == nil || s.Mode != world.UserPlay {
		return false
	}
	// No deadline is carried: a ride that finds the next room taken waits for it
	// (fadaEsperaSalaLivre), and the ways out are the leader leaving the Água,
	// losing the lead or taking the fairy off.
	return d.enfileirarAvancoDaFada(avancoDaFada{
		variant: variant,
		room:    room,
		leader:  leader.ID,
		espera:  fadaEsperaNaAgua,
	})
}

// enfileirarAvancoDaFada adds one ride to the queue, refusing a second one for
// the same room. The clear hook can fire more than once per run (a respawn, a
// second GenerateMob), and claimWaterReward only guards the payout — without
// this, two rides would queue and the party would be moved twice.
func (d *Dispatcher) enfileirarAvancoDaFada(a avancoDaFada) bool {
	for _, j := range d.events.aguaFada {
		if j.variant == a.variant && j.room == a.room {
			return true
		}
	}
	d.events.aguaFada = append(d.events.aguaFada, a)
	return true
}

// tickFadaDaAgua moves the queued parties. It runs every tick (1s), not on the
// water's 2s cadence, because the wait is counted in seconds.
func (d *Dispatcher) tickFadaDaAgua(w *world.World) {
	if len(d.events.aguaFada) == 0 {
		return
	}
	// Filter in place: every entry either survives to the next tick or is resolved
	// here, so the queue is rewritten from its own head.
	restantes := d.events.aguaFada[:0]
	for _, a := range d.events.aguaFada {
		if a.espera > 0 {
			a.espera--
			restantes = append(restantes, a)
			continue
		}
		leader := w.Entity(a.leader)
		s := w.Session(a.leader)
		// Logged out, no longer the leader, or the fairy came off: whatever earned
		// the ride is gone, but the room was cleared and the scroll is still owed.
		// Three checks and not one, so the player is told WHICH — the one-line
		// "saiu ou tirou a fada" left a party that lost its leader wondering about
		// a fairy nobody had touched.
		if leader == nil || s == nil || s.Mode != world.UserPlay {
			d.entregarPergaminhoDaFada(w, leader, a, motivoFadaSaiuDoJogo)
			continue
		}
		if leader.Leader != 0 {
			d.entregarPergaminhoDaFada(w, leader, a, motivoFadaNaoELider)
			continue
		}
		if !fadaLevaNaAgua(leader.Equip[fairyEquipSlot].Index) {
			d.entregarPergaminhoDaFada(w, leader, a, motivoFadaForaDoSlot)
			continue
		}
		// Walked out of the dungeon on foot: moving the party from outside would
		// teleport people who already left the run.
		if !insideAnyWaterRoom(a.variant, leader.X, leader.Y) {
			d.entregarPergaminhoDaFada(w, leader, a, motivoFadaForaDaAgua)
			continue
		}
		proxima := proximaSalaDaAgua(a.room)
		// A lap after the boss is paid from the bag instead of by the reward that
		// was not minted, and the scroll found there decides which room opens.
		// No scroll left is the one thing that ends the cycle for good.
		cobrar := -1
		if a.room >= waterDeadRoom {
			sala, slot, achou := pergaDaBolsaParaRecomecar(d.itemVolatiles, leader, a.variant)
			if !achou {
				d.announceWaterRoom(w, leader, "Sem pergaminho na bolsa: a fada para aqui.")
				d.log.Info("fairy cycle ended: no scroll in the bag",
					"leader", leader.Name, "variant", a.variant)
				continue
			}
			proxima, cobrar = sala, slot
		}
		if ocupante, ocupada := d.waterRoomBusy(w, a.variant, proxima); ocupada {
			// The ride does NOT give up here. The room in front empties on its own —
			// the party ahead moves on, or its countdown throws it out — so the only
			// thing waiting costs is time, and giving up cost the run: the scroll
			// went back to a bag whose owner then clicked it into the same occupied
			// room, and the refusal there is one the client does not draw.
			d.fadaEsperaSalaLivre(w, leader, &a, proxima, ocupante)
			restantes = append(restantes, a)
			continue
		}
		// Charged only now, with the room about to open: every path above leaves
		// on a refusal, and a scroll eaten there would be a scroll paid for a
		// room nobody entered.
		if cobrar >= 0 {
			consumeOneItem(&leader.Carry[cobrar])
			d.sendSlot(w, s, world.ItemPlaceCarry, cobrar, leader.Carry[cobrar])
		}
		d.abrirSalaDaAgua(w, s, leader, a.variant, proxima)
		d.announceWaterRoom(w, leader, "A fada levou o grupo: "+waterRoomLabel(proxima)+".")
		d.log.Info("fairy advanced the party",
			"leader", leader.Name, "variant", a.variant, "from", a.room, "to", proxima)
	}
	d.events.aguaFada = restantes
}

// fadaEsperaSalaLivre holds a ride whose next room is taken, and keeps the party
// in place until that room empties.
//
// Two things have to hold for a wait to actually last. The party must stay in
// the dungeon: the room it is standing in is a CLEARED room, cut to 30 seconds
// when the last monster died, and its expiry teleports everyone to the entrance
// — which would end the wait by eviction, the very stop this is meant to remove.
// So that room's countdown is refilled as it runs down. And the party must SEE
// that it is waiting, or waiting is indistinguishable from the freeze it
// replaces.
//
// The cost is deliberate: a waiting party holds its own room for as long as it
// waits, so a party behind it waits too. That is the same room it would have
// been sitting in anyway, and walking out of the Água still ends the ride and
// hands the scroll back.
func (d *Dispatcher) fadaEsperaSalaLivre(w *world.World, leader *world.Entity, a *avancoDaFada, proxima int, ocupante string) {
	if d.events.water[a.variant][a.room] < fadaPisoDaEspera {
		d.events.water[a.variant][a.room] = fadaRecargaDaEspera
		d.broadcastWaterCountdown(w, leader, fadaRecargaDaEspera)
		d.log.Info("fairy wait: holding the cleared room open",
			"leader", leader.Name, "variant", a.variant, "room", a.room, "next", proxima)
	}
	if a.esperando%fadaAvisoDeEspera == 0 {
		// Plain ASCII, like the motives below: the panel copies the bytes raw.
		d.announceWaterRoom(w, leader, textoDaFadaEsperando(proxima, ocupante))
		d.log.Info("fairy waiting for the next room",
			"leader", leader.Name, "variant", a.variant, "next", proxima,
			"occupant", ocupante, "waited_s", a.esperando)
	}
	a.esperando++
}

// textoDaFadaEsperando is the panel line while a ride waits for the room in
// front to empty. Plain ASCII and short, for the same reason as
// textoDaFadaQueParou: the panel copies the bytes raw and cuts at 96.
func textoDaFadaEsperando(proxima int, ocupante string) string {
	return waterRoomLabel(proxima) + " ocupada por " + ocupante + ": a fada espera liberar."
}

// Why a ride was given up. They go to the log AND to the player's panel: a
// fairy that stopped used to look exactly like a fairy that never worked, and
// telling the two apart took someone reading the server log.
//
// Plain ASCII on purpose. The panel copies the bytes raw (EncodeExpPanelBody)
// and the client reads CP1252, so an accent here reaches the screen as mojibake.
const (
	motivoFadaSaiuDoJogo = "saiu do jogo"
	motivoFadaNaoELider  = "nao e mais lider"
	motivoFadaForaDoSlot = "fada fora do slot"
	motivoFadaForaDaAgua = "fora da agua"
)

// textoDaFadaQueParou is the panel line for a ride given up after room.
//
// A numbered room still hands the scroll over, so the line says what to do with
// it; after the boss there is nothing to hand back, so it only says why.
func textoDaFadaQueParou(room int, motivo string) string {
	if room >= waterDeadRoom {
		return "A fada parou: " + motivo + "."
	}
	return waterRoomLabel(room) + " limpa! A fada parou: " + motivo + ". Use o proximo pergaminho."
}

// entregarPergaminhoDaFada is every failed path: the party gets the scroll it
// would have got without a fairy, and the announcement that goes with it.
func (d *Dispatcher) entregarPergaminhoDaFada(w *world.World, leader *world.Entity, a avancoDaFada, motivo string) {
	if leader == nil {
		// Nobody to hand it to. The reward dies with the run, exactly as it does
		// when a leader disconnects mid-room today.
		d.log.Info("fairy advance dropped: leader gone",
			"variant", a.variant, "room", a.room, "motivo", motivo)
		return
	}
	// A failed lap after the boss has nothing to hand back: the boss pays loot,
	// never a scroll, and rewardBase+9 is not an item at all. The cycle just ends.
	if a.room >= waterDeadRoom {
		d.announceWaterRoom(w, leader, textoDaFadaQueParou(a.room, motivo))
		d.log.Info("fairy cycle ended after the boss",
			"leader", leader.Name, "variant", a.variant, "motivo", motivo)
		return
	}
	d.grantNextWaterScroll(w, leader, a.variant, a.room)
	d.announceWaterRoom(w, leader, textoDaFadaQueParou(a.room, motivo))
	d.log.Info("fairy advance fell back to the scroll",
		"leader", leader.Name, "variant", a.variant, "room", a.room, "motivo", motivo)
}
