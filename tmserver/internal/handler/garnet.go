package handler

import "github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"

// absorverGarnet takes the defender's Garnet off a blow that reached a player,
// from another player or from a monster.
//
// DELIBERATE DIVERGENCE, decided by Marco on 2026-09-17 ("A2" in
// docs/balanceamento/garnet-esmeralda-2026-09-17.md). The legacy subtracts the
// whole Garnet total flat (CMob.cpp:873, _MSG_Attack.cpp:1496, GetFunc.cpp:1639):
// a full +15 set is 2.640, so every blow under that became 1 — immunity to
// ordinary monsters and to anyone without Esmeralda. Simulated, a percentage cap
// on the whole blow fixed that but broke the other job of the gem, cancelling
// the Esmeralda: at 30% a full Esmeralda beat a full Garnet 10 to 0. So the
// Garnet works in two steps:
//
//  1. it cancels the attacker's Esmeralda (EquipForceDamage — not the Ligação
//     Espectral) whole, up to its own total;
//  2. what is left of it takes at most GarnetPct% of the rest of the blow.
//
// A monster carries no Esmeralda, so against one only step 2 applies. 100 in
// the panel is the legacy exactly; 0 leaves only step 1.
//
// Floored at 1, as the legacy and every other step of the PvP block are: a blow
// that landed must not read as a miss.
func (d *Dispatcher) absorverGarnet(attacker, target *world.Entity, dmg int) int {
	if dmg <= 0 || attacker == nil || target == nil || !world.IsPlayer(target.ID) {
		return dmg
	}
	garnet := int(target.EquipGarnet)
	if garnet <= 0 {
		return dmg
	}
	anula := min(garnet, max(int(attacker.EquipForceDamage), 0), dmg)
	tira := anula + min(garnet-anula, (dmg-anula)*int(d.combatRules.GarnetPct)/100)
	return max(dmg-tira, 1)
}
