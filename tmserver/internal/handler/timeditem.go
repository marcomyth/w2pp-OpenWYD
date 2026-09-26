package handler

import (
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A temporary item has two lives, and the difference is the whole point of this
// file. Before it is activated it holds a DURATION and does not age: a Conjunto
// bought today and left in the bag is still worth thirty days next month. Once
// activated it holds a DEADLINE and ages on the wall clock.
//
// Those two states are the item's own fields, so they persist with it and no new
// column is needed:
//
//	un-started: Effects = EF_WDAY/EF_HOUR/EF_MIN, ExpiresAt = 0
//	running:    ExpiresAt = the instant it dies, Effects cleared
//
// The fairies are the exception the legacy already made: their tooltip says "Não
// gasta caso não esteja equipado", and ProcessSecMinTimer.cpp:612 backs it by
// ticking BASE_CheckFairyDate only on Equip[13]. They therefore never convert —
// they stay in the duration form for life and are decremented a minute at a time
// while worn.
// fairyEquipSlot (Equip[13], the slot ProcessSecMinTimer watches) lives in
// exp_bonus.go, which already needed it for the fairy's EXP bonus.
const (
	// Fairy item range (BASE_CheckFairyDate bails outside it).
	fairyFirstIndex = 3900
	fairyLastIndex  = 3913
	// fadaDoValeIndex is the Fada do Vale: a fairy by name, by slot and by
	// the shop that sells it (the same Fadas stock as 3900-3908), but outside the
	// legacy range. Left out, it never burned while worn and never showed its time.
	fadaDoValeIndex = 3916
	// fairyTickPeriod is how many 1s loop ticks make the legacy's minute pulse.
	fairyTickPeriod = 60
)

func isFairy(index int16) bool {
	return index >= fairyFirstIndex && index <= fairyLastIndex || index == fadaDoValeIndex
}

// durationEffects renders a lifetime as the three effect slots an un-started
// item carries. This is also exactly what the client shows, so an item waiting
// in the bag reads "30 Dia(s)" without any conversion.
func durationEffects(d time.Duration) [3]world.Effect {
	if d < 0 {
		d = 0
	}
	days := int64(d.Hours()) / 24
	if days > 255 {
		days = 255
	}
	return [3]world.Effect{
		{Effect: efWDay, Value: uint8(days)},
		{Effect: efHour, Value: uint8(int64(d.Hours()) % 24)},
		{Effect: efMin, Value: uint8(int64(d.Minutes()) % 60)},
	}
}

// effectsDuration reads back what durationEffects wrote. It reports 0 when the
// slots hold something else — a refine, a damage bonus — which is the common case.
func effectsDuration(eff [3]world.Effect) time.Duration {
	var out time.Duration
	for _, e := range eff {
		switch e.Effect {
		case efWDay:
			out += time.Duration(e.Value) * 24 * time.Hour
		case efHour:
			out += time.Duration(e.Value) * time.Hour
		case efMin:
			out += time.Duration(e.Value) * time.Minute
		}
	}
	return out
}

// itemLifetime is how long an un-started item will run once activated. The item's
// own effects win, so a GM or the item editor can hand out a shorter one; the
// catalog is the fallback and covers everything obtained normally, because the
// lifetime is written into the item's NAME — "Conjunto_Yin-Yang(30dias)". That is
// what makes 7- and 14-day variants work with no code change: a new row in the
// catalog is enough.
//
// SÓ ITEM TEMPORÁRIO TEM VIDA. Os efeitos só são lidos como duração num item que o
// catálogo, o nome ou defaultLifetimeDays já dizem ser temporário, ou numa fada.
// Em qualquer outro, os três pares não são efeitos: na montaria (2330-2389) são o
// HP empacotado, o nível, a vitalidade e a ração; no corpo do slot 0, os dados da
// classe. Um desses bytes valendo 106, 107 ou 108 — nível 107, ou o byte baixo do
// HP passando por ali — era lido como EF_WDAY/HOUR/MIN, e startTimedItem "iniciava
// o prazo" apagando os três pares. Em produção, em 26/09/2026, o pulso de um minuto
// zerou assim a montaria de um jogador (HP, nível, vitalidade e ração), e a cura
// seguinte a destruiu; o corpo de outro ganhou prazo para sumir.
func (d *Dispatcher) itemLifetime(it world.Item) time.Duration {
	catalogo := time.Duration(0)
	if days := d.itemDurations[int(it.Index)]; days > 0 {
		catalogo = time.Duration(days) * 24 * time.Hour
	} else if days := defaultLifetimeDays[it.Index]; days > 0 {
		catalogo = time.Duration(days) * 24 * time.Hour
	}
	if catalogo == 0 && !isFairy(it.Index) {
		return 0 // permanente: os efeitos dele não são prazo, sejam quais forem
	}
	if life := effectsDuration(it.Effects); life > 0 {
		return life
	}
	return catalogo
}

// defaultLifetimeDays is the lifetime of the items whose names lost their
// "(Ndias)", for when the item carries none. Each number is what the old name
// said, so nothing lives longer or shorter than before; without it the catalog
// knows no lifetime for them.
//
// The three cash-shop mounts lost theirs on 2026-09-11 — the shop sells each one
// for 24h, 3 or 5 days, written on the item it delivers — and one handed out
// with no duration (a GM, an old delivery) would never run out.
//
// The fairies lost theirs on 2026-09-23, so that the duration is the one the
// item carries (EF_WDAY) and the name is just the fairy. For them this table is
// not optional: the shop sells fairies with no effects at all, and one that
// reaches Equip[13] with no lifetime is deleted by tickFairies on the first
// pulse. The 15- and 30-day Verdes keep 15 and 30 even though their catalog
// rows say EF_WDAY 7: the name is what they ran on.
var defaultLifetimeDays = map[int16]int{
	3980: 3, // Shire
	3981: 3, // Thoroughbred
	3982: 3, // Klazedale

	// Every mount that lends XP runs out, the rule TestMontariaComXPTemPrazo
	// holds. These never had a "(Ndias)" in the name, so without a row here one
	// handed out with no duration was permanent XP. The Tigre and the Dragão take
	// what the supporter packs deliver (0123: 7 and 15); the Esferas carry the
	// Tigre's line (350/50, +12%), so they take its 7.
	3990: 7,  // Tigre de Fogo da loja
	3991: 15, // Dragão Vermelho da loja
	2969: 7,  // Tigre de Cristal (Esfera)
	2970: 7,  // Tigre Negro (Esfera)
	2971: 7,  // Rinoceronte Espectral (Esfera)
	2972: 7,  // Unicórnio de Gelo (Esfera)
	2973: 7,  // Tigre de Gelo (Esfera)
	2974: 7,  // Fenrir Sombrio (Esfera)
	2975: 7,  // Dragão de Gelo (Esfera)

	3900: 3,  // Fada Verde
	3901: 3,  // Fada Azul
	3902: 3,  // Fada Vermelha
	3903: 5,  // Fada Verde
	3904: 5,  // Fada Azul
	3905: 5,  // Fada Vermelha
	3906: 7,  // Fada Verde
	3907: 7,  // Fada Azul
	3908: 7,  // Fada Vermelha
	3911: 7,  // Fada Verde
	3912: 15, // Fada Verde
	3913: 30, // Fada Suprema
	3916: 7,  // Fada do Vale
}

// prazoIndevido diz se o prazo de it nasceu do engano que itemLifetime corrige:
// um item que não é temporário (itemLifetime do índice puro é zero) e que é de
// um dos dois tipos cujos efeitos não são efeitos — a montaria de 2330-2389 e o
// corpo do slot 0 (noCorpo). Esses itens ganharam ExpiresAt quando um byte deles
// passou por 106-108, e sumiriam no vencimento.
//
// Restrito aos dois tipos de propósito: o /gm item põe prazo de verdade em
// qualquer item (gm.go), e desfazer todo prazo fora do catálogo apagaria esse.
func (d *Dispatcher) prazoIndevido(it world.Item, noCorpo bool) bool {
	if it.ExpiresAt == 0 || d.itemLifetime(world.Item{Index: it.Index}) > 0 {
		return false
	}
	return noCorpo || (it.Index >= mountLo && it.Index < mountHi)
}

// desfazerPrazosIndevidos aplica prazoIndevido a uma lista de itens carregada do
// banco, antes do dropExpired do login, e devolve quantos desfez. corpo diz se o
// primeiro item da lista é o do slot 0 (a lista é o Equip).
func (d *Dispatcher) desfazerPrazosIndevidos(items []world.Item, corpo bool) int {
	n := 0
	for i := range items {
		if d.prazoIndevido(items[i], corpo && i == 0) {
			items[i].ExpiresAt = 0
			n++
		}
	}
	return n
}

// startTimedItem begins a temporary item's life the first time it is equipped,
// and reports whether it changed anything. Costumes, spheres and mounts convert
// to a deadline; a fairy only has its duration seeded, because it burns solely
// while worn (tickFairies).
//
// It is a no-op for a permanent item, and for one already running — equipping a
// costume for the second time must not hand back another thirty days.
func (d *Dispatcher) startTimedItem(it *world.Item, now time.Time) bool {
	if it == nil || it.Index == 0 || it.ExpiresAt != 0 {
		return false
	}
	life := d.itemLifetime(*it)
	if life <= 0 {
		return false
	}
	if isFairy(it.Index) {
		if effectsDuration(it.Effects) > 0 {
			return false // already counting
		}
		it.Effects = durationEffects(life)
		return true
	}
	it.ExpiresAt = now.Add(life).Unix()
	it.Effects = [3]world.Effect{}
	return true
}

// tickTimedItems is the minute pulse for every other temporary item — the cash
// mounts, the Esferas, the costumes — which run on a deadline (ExpiresAt), not
// on a burned duration like the fairies.
//
// Deriving the countdown on every send (expiryEffects) was only half of it: the
// client draws what it was last sent and never ticks it, and before this nothing
// was sent while the player stayed online. A mount read the same "N Dia(s)" all
// session, and one whose deadline passed kept its damage, ABS and XP until the
// next login, the only place dropExpired ran.
func (d *Dispatcher) tickTimedItems(w *world.World) {
	if d.tickCount%fairyTickPeriod != 0 {
		return
	}
	now := time.Now()
	w.ForEachPlaying(-1, func(s *world.Session, e *world.Entity) {
		d.pulseTimedItems(w, s, e, now)
	})
}

// pulseTimedItems does the three things a worn deadline needs each minute:
// resend it so the client's countdown moves, clear it once it is due, and start
// one that sits in the gear un-started. That last one is a mount dragged on
// before 16c867bb, when dragging did not start the clock: it has been worn ever
// since with its duration intact and would otherwise never run out.
//
// The bag gets only the expiry. An item there is either un-started, and must not
// age, or taken off after starting ("Consumo contínuo, mesmo não equipado"), and
// then it dies on time like the worn one. The Bolsa do Andarilho is left to
// expireWandererBags, which also has to shrink the bag.
func (d *Dispatcher) pulseTimedItems(w *world.World, s *world.Session, e *world.Entity, now time.Time) {
	gearChanged, mountGone := false, false
	for slot := range e.Equip {
		if slot == fairyEquipSlot {
			continue // tickFairies: a fairy burns only while worn
		}
		it := &e.Equip[slot]
		if it.Empty() {
			continue
		}
		switch {
		case d.prazoIndevido(*it, slot == 0):
			it.ExpiresAt = 0
			d.log.Warn("prazo indevido desfeito", "account", s.AccountName, "conn", s.Conn, "slot", slot, "item", it.Index)
		case it.ExpiresAt == 0:
			if !d.startTimedItem(it, now) {
				continue // permanent
			}
			d.log.Info("timed item started on the pulse", "account", s.AccountName, "conn", s.Conn, "slot", slot, "item", it.Index)
		case now.Unix() >= it.ExpiresAt:
			d.log.Info("timed item expired", "account", s.AccountName, "conn", s.Conn, "slot", slot, "item", it.Index)
			*it = world.Item{}
			gearChanged = true
			mountGone = mountGone || slot == mountEquipSlot
		}
		d.sendSlot(w, s, world.ItemPlaceEquip, slot, *it)
	}
	for slot := range e.Carry {
		it := &e.Carry[slot]
		if d.prazoIndevido(*it, false) {
			it.ExpiresAt = 0
			d.log.Warn("prazo indevido desfeito na bolsa", "account", s.AccountName, "conn", s.Conn, "slot", slot, "item", it.Index)
			d.sendSlot(w, s, world.ItemPlaceCarry, slot, *it)
			continue
		}
		if it.ExpiresAt == 0 || it.Index == itemWandererBag || now.Unix() < it.ExpiresAt {
			continue
		}
		d.log.Info("timed item expired in the bag", "account", s.AccountName, "conn", s.Conn, "slot", slot, "item", it.Index)
		*it = world.Item{}
		d.sendSlot(w, s, world.ItemPlaceCarry, slot, *it)
	}
	if gearChanged {
		d.refreshEquip(w, s, e)
	}
	if mountGone {
		d.refreshBabyMountSummon(w, s, e)
	}
}

// tickFairies burns a minute off every equipped fairy, and only off those:
// ProcessSecMinTimer.cpp:612 passes Equip[13] and nothing else, which is what
// the item's own "Não gasta caso não esteja equipado" promises. One that runs out
// is cleared from the slot, as BASE_CheckFairyDate does at day 0 hour 0 min <= 1.
func (d *Dispatcher) tickFairies(w *world.World) {
	if d.tickCount%fairyTickPeriod != 0 {
		return
	}
	w.ForEachPlaying(-1, func(s *world.Session, e *world.Entity) {
		it := &e.Equip[fairyEquipSlot]
		if !isFairy(it.Index) {
			return
		}
		// A Fada do Vale worn before it counted as a fairy may already run on a
		// deadline (startTimedItem converted it then). dropExpired owns that one;
		// reading its empty effects as "no time left" would delete it.
		if it.ExpiresAt != 0 {
			return
		}
		// A fairy that reaches the slot never started — its effects hold no
		// duration — is started here, not deleted. The shop sells fairies with no
		// effects at all, so every path that equips one without startTimedItem used
		// to lose it on the first pulse, with no time ever on screen.
		if effectsDuration(it.Effects) <= 0 && d.startTimedItem(it, time.Now()) {
			d.sendSlot(w, s, world.ItemPlaceEquip, fairyEquipSlot, *it)
			d.log.Info("fairy started on the pulse", "account", s.AccountName, "conn", s.Conn, "item", it.Index)
			return
		}
		left := effectsDuration(it.Effects) - time.Minute
		if left <= 0 {
			*it = world.Item{}
			d.sendSlot(w, s, world.ItemPlaceEquip, fairyEquipSlot, *it)
			d.refreshEquip(w, s, e)
			d.log.Info("fairy expired", "account", s.AccountName, "conn", s.Conn)
			return
		}
		it.Effects = durationEffects(left)
		d.sendSlot(w, s, world.ItemPlaceEquip, fairyEquipSlot, *it)
	})
}
