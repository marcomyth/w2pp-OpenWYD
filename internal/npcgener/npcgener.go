// Package npcgener parses Release/TMsrv/run/NPCGener.txt, the legacy spawn-block
// list that decides which mob/NPC template appears where.
//
// It lives at the repo root rather than under tmserver/ because three services
// need the same recipe and Go's internal rule kept them from sharing it: the
// tmServer spawns from it at boot, dbServer's `import-npcs` seeds npc_definition
// from it, and the moderator NPC panel in webServer reads it to show the NPCs a
// database may not have been seeded with yet. dbServer previously carried a
// 97-line copy of this parser for exactly that reason.
package npcgener

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Generator is one spawn block of NPCGener.txt (CNPCGene.cpp ParseString): it
// spawns MinGroup..MaxGroup mobs of Leader (plus Followers) patrolling the
// SegX/SegY waypoints per RouteType, regenerating every MinuteGenerate minutes.
//
// Waypoint index mapping is the original's: Start*→[0], Segment1..3*→[1..3],
// Dest*→[4]. Unused waypoints stay 0 and the segment walker skips them
// (SetSegment, CMob.cpp:608) — so the common Start/Dest-only block patrols
// 0→4→0.
type Generator struct {
	Leader         string
	Follower       string
	MinuteGenerate int // respawn period in minutes; <=0 = never regenerate
	MinGroup       int
	MaxGroup       int
	MaxNumMob      int
	RouteType      int
	Formation      int
	SegX, SegY     [5]int16
	SegRange       [5]int
	SegWait        [5]int
	FightAction    [4]string
	DieAction      [4]string
}

// Load parses NPCGener.txt. Blocks start with '#'; lines are "Key:\tvalue";
// '//' lines are comments. The returned slice is in file order, so a block's
// index is the stable generator_index used by npc_definition.
func Load(path string) ([]Generator, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("npcgener: open NPCGener: %w", err)
	}
	defer f.Close()

	var out []Generator
	var cur *Generator
	flush := func() {
		if cur != nil && cur.Leader != "" {
			out = append(out, *cur)
		}
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.HasPrefix(line, "#") {
			flush()
			cur = &Generator{MinGroup: 1}
			continue
		}
		if cur == nil {
			continue
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		switch key {
		case "Leader":
			cur.Leader = val
		case "Follower":
			if val != "0" { // "0" = no follower template (ParseString rejects it)
				cur.Follower = val
			}
		case "MinuteGenerate":
			cur.MinuteGenerate = atoi(val)
		case "MinGroup":
			cur.MinGroup = atoi(val)
		case "MaxGroup":
			cur.MaxGroup = atoi(val)
		case "MaxNumMob":
			cur.MaxNumMob = atoi(val)
		case "RouteType":
			cur.RouteType = atoi(val)
		case "Formation":
			cur.Formation = atoi(val)
		case "FightAction":
			setAction(&cur.FightAction, val)
		case "DieAction":
			setAction(&cur.DieAction, val)
		// Waypoints: Start→[0], Segment1..3→[1..3], Dest→[4] (ParseString).
		case "StartX":
			cur.SegX[0] = int16(atoi(val))
		case "StartY":
			cur.SegY[0] = int16(atoi(val))
		case "StartRange":
			cur.SegRange[0] = atoi(val)
		case "StartWait":
			cur.SegWait[0] = atoi(val)
		case "DestX":
			cur.SegX[4] = int16(atoi(val))
		case "DestY":
			cur.SegY[4] = int16(atoi(val))
		case "DestRange":
			cur.SegRange[4] = atoi(val)
		case "DestWait":
			cur.SegWait[4] = atoi(val)
		default:
			if setIndexedAction(key, "FightAction", &cur.FightAction, val) ||
				setIndexedAction(key, "DieAction", &cur.DieAction, val) {
				continue
			}
			// Segment1..3{X,Y,Range,Wait} → indices 1..3. Segment*Action keys are
			// route flavor text and remain unmapped; FightAction/DieAction above are
			// combat-visible chat lines.
			if strings.HasPrefix(key, "Segment") && len(key) > 8 {
				if i := int(key[7] - '0'); i >= 1 && i <= 3 {
					switch key[8:] {
					case "X":
						cur.SegX[i] = int16(atoi(val))
					case "Y":
						cur.SegY[i] = int16(atoi(val))
					case "Range":
						cur.SegRange[i] = atoi(val)
					case "Wait":
						cur.SegWait[i] = atoi(val)
					}
				}
			}
		}
	}
	flush()
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("npcgener: scan NPCGener: %w", err)
	}
	return out, nil
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func setAction(dst *[4]string, val string) {
	if len(val) >= 79 {
		return // CNPCGene::SetAct rejects action strings that would overflow [80].
	}
	for i := range dst {
		if dst[i] == "" {
			dst[i] = val
			return
		}
	}
}

func setIndexedAction(key, prefix string, dst *[4]string, val string) bool {
	if !strings.HasPrefix(key, prefix) || key == prefix {
		return false
	}
	n, err := strconv.Atoi(key[len(prefix):])
	if err != nil {
		return false
	}
	if n < 1 || n > 4 {
		return false
	}
	if len(val) < 79 {
		dst[n-1] = val
	}
	return true
}
