package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Âmago: the mount growth item (EF_VOLATILE 16, _MSG_UseItem.cpp:1564).
//
// Dropped onto the mount worn in Equip[14], it feeds it: the hunger meter is
// refilled and the mount gains a growth level. A cria (2330-2359) grows for
// free; an adult (2360-2389) rolls against its growth rate and can fail.
// Crossing the cria's level threshold turns it into the adult, sIndex+30 — the
// same +30 step the egg took to become the cria.
const (
	// mountLo/mountHi bound the mounts an Âmago accepts: cria and adult both.
	mountLo = 2330
	mountHi = 2390
	// criaHi ends the cria band; from here up the mount is an adult.
	criaHi = 2360
	// amagoBase is where the Âmago row starts, so (amago-amagoBase)%30 gives the
	// same slot (mount-mountLo)%30 gives — that is how the two are matched.
	amagoBase = 2390
	// mountRowSize is the stride of each row (cria, adult, âmago).
	mountRowSize = 30

	// mountFedValue is what a feed restores stEffect[0] to (:1596).
	mountFedValue = 20000
	// mountFedEffect is stamped alongside it (:1597).
	mountFedEffect = 100
	// mountLevelUp is the growth a feed grants. The legacy uses 10 under
	// LOCALSERVER and 1 otherwise (:1644-1647); 1 is the shipped behaviour.
	mountLevelUp = 1
	// adultMaxLevel caps an adult's growth (:1600).
	adultMaxLevel = 120

	// The cria's level threshold to become an adult, per row (:1654-1656).
	criaGrowth2330 = 25
	criaGrowth2331 = 50
	criaGrowthRest = 100

	// Sleipnir and Svadilfari share their Âmago with another row (:1583-1587).
	mountSlotSleipnir   = 28
	mountSlotSvadilfari = 27
	amagoSlotSleipnir   = 21
	amagoSlotSvadilfari = 10
)

// mountAmagoSlot maps a mount's sIndex to the Âmago row that feeds it.
func mountAmagoSlot(index int16) int {
	slot := (int(index) - mountLo) % mountRowSize
	switch slot {
	case mountSlotSleipnir:
		return amagoSlotSleipnir
	case mountSlotSvadilfari:
		return amagoSlotSvadilfari
	}
	return slot
}

// criaGrowsAt is the level at which a cria becomes its adult form.
func criaGrowsAt(index int16) int {
	switch {
	case index == mountLo:
		return criaGrowth2330
	case index == mountLo+1:
		return criaGrowth2331
	case index > mountLo+1 && index < criaHi:
		return criaGrowthRest
	}
	return 0 // not a cria
}

// useAmago feeds the worn mount (_MSG_UseItem.cpp:1564-1684).
func (d *Dispatcher) useAmago(w *world.World, s *world.Session, e *world.Entity, body protocol.MsgUseItemBody, src int) {
	// The target is ONLY the worn mount. The legacy hands the item back without a
	// word for anything else, which is why dropping an Âmago on a bag slot looks
	// like nothing happened — reproduced, since the client draws no dialogue here.
	if body.DestType != 0 || body.DestPos != mountEquipSlot {
		d.sendSlot(w, s, world.ItemPlaceCarry, src, e.Carry[src])
		return
	}
	dst := &e.Equip[mountEquipSlot]
	// stEffect[0] is the hunger meter: a mount at zero is starved and refuses to
	// be fed further (:1574).
	if dst.Index < mountLo || dst.Index >= mountHi || amagoHunger(*dst) <= 0 {
		d.sendSlot(w, s, world.ItemPlaceCarry, src, e.Carry[src])
		return
	}
	if mountAmagoSlot(dst.Index) != (int(e.Carry[src].Index)-amagoBase)%mountRowSize {
		d.notify(w, s, NoticeMountNotMatch)
		d.sendSlot(w, s, world.ItemPlaceCarry, src, e.Carry[src])
		return
	}

	// The feed lands before any growth roll: even a failed refine leaves the mount
	// fed (:1596-1598).
	putShort(&dst.Effects[0], mountFedValue)
	dst.Effects[2].Effect = mountFedEffect

	level := int(dst.Effects[1].Effect)
	adult := dst.Index >= criaHi && dst.Index < mountHi
	if adult && level >= adultMaxLevel {
		d.notify(w, s, NoticeCantUpgradeMore)
		d.sendSlot(w, s, world.ItemPlaceCarry, src, e.Carry[src])
		d.sendSlot(w, s, world.ItemPlaceEquip, mountEquipSlot, *dst)
		return
	}

	// Only an adult can fail. A cria grows on every feed, which is what makes the
	// early mount levels deterministic.
	if adult && w.Rand().Intn(101) > amagoGrowthRate(*dst) {
		d.notify(w, s, NoticeFailToRefine)
		// One feed in five costs the adult a level (:1633).
		if w.Rand().Intn(5) == 0 && dst.Effects[1].Effect > 0 {
			dst.Effects[1].Effect--
		}
		consumeOneItem(&e.Carry[src])
		d.sendSlot(w, s, world.ItemPlaceCarry, src, e.Carry[src])
		d.sendSlot(w, s, world.ItemPlaceEquip, mountEquipSlot, *dst)
		d.refreshBabyMountSummon(w, s, e)
		return
	}

	level += mountLevelUp
	dst.Effects[1].Effect = uint8(level)
	dst.Effects[2].Value = 1

	if at := criaGrowsAt(dst.Index); at > 0 && level >= at {
		dst.Index += mountRowSize
		dst.Effects[1].Value = uint8(w.Rand().Intn(20) + int(dst.Effects[1].Effect))
		dst.Effects[1].Effect = 0
		dst.Effects[2].Value = 0
		d.notify(w, s, NoticeMountGrowth)
	}

	consumeOneItem(&e.Carry[src])
	d.sendSlot(w, s, world.ItemPlaceCarry, src, e.Carry[src])
	d.sendSlot(w, s, world.ItemPlaceEquip, mountEquipSlot, *dst)
	d.refreshBabyMountSummon(w, s, e)
	d.log.Info("amago fed",
		"conn", s.Conn, "account", s.AccountName,
		"mount", dst.Index, "level", dst.Effects[1].Effect)
}

// amagoHunger reads stEffect[0] as the 16-bit hunger meter the legacy keeps
// there (sValue), the same packing putShort writes.
func amagoHunger(it world.Item) int {
	return int(it.Effects[0].Effect) | int(it.Effects[0].Value)<<8
}

// amagoGrowthRate is BASE_GetGrowthRate: an adult's chance to gain a level.
//
// UNVERIFIED: the function is not in this repo's sources, so the curve it reads
// is unknown. A flat rate keeps adults progressing rather than stuck, and is the
// one number to replace once a capture or the missing source turns up.
func amagoGrowthRate(world.Item) int { return 50 }
