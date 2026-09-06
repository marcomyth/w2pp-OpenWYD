// Package mountrate holds the mount growth curves: the chance an âmago raises an
// adult mount one level, per lineage and per band of twenty levels.
//
// It exists as its own package for one reason — the handler must not import the
// database client, and the client must not import the handler. The table is the
// shape both agree on.
//
// There is no legacy curve to match here. BASE_GetGrowthRate is absent from the
// sources this port has, so the shape is a balance decision of this server,
// configured through the panel (0030_mount_growth_rate) rather than compiled in.
package mountrate

import "github.com/jeanluca/w2pp-openwyd/internal/domain"

// Unset marks a band nobody configured. It is -1 rather than 0 because 0 is a
// legitimate setting — a band the operator deliberately made impossible — and
// the two must not collapse into each other.
const Unset int8 = -1

// Curve is one lineage's six bands, indexed by band (0 = levels 1..20).
type Curve [domain.MountGrowthBands]int8

// UnsetCurve returns a curve with every band unconfigured, which is what a
// lineage looks like before anyone edits it.
func UnsetCurve() Curve {
	var c Curve
	for i := range c {
		c[i] = Unset
	}
	return c
}

// Table maps an ADULT mount index (2360..2389) to its curve. A lineage absent
// from the table has no configuration at all, and the caller's default applies.
type Table map[int16]Curve

// Rate returns the configured chance for a mount at a level, and whether one
// exists. The caller keeps its own default for the false case rather than this
// package inventing one: the default belongs with the game rule, not with the
// storage shape.
func (t Table) Rate(mountIndex int16, level int) (int, bool) {
	if t == nil {
		return 0, false
	}
	curve, ok := t[mountIndex]
	if !ok {
		return 0, false
	}
	band := domain.MountGrowthBandFor(level)
	if int(band) >= len(curve) {
		return 0, false
	}
	if curve[band] == Unset {
		return 0, false
	}
	return int(curve[band]), true
}
