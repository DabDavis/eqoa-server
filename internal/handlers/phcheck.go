package handlers

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// phcheck.go — #phcheck, a GM's and a sub GM's look at placeholders (the
// owner, 2026-11-26: "GMs and sub GMs ... a command to check placeholders":
// the placeholder spawns that roll a named). Read off the world as it stands
// (entity/phcheck.go: npcrare's rares and /ph's bindings, a binding's named
// its kind of the lowest odds); it spawns, kills and changes nothing. As
// /phcheck, #phcheck, or a PS2's "/t phcheck ...":
//
//   - #phcheck <name>: a named's placeholders (each spot's row, what it stands
//     as, where, how far, up or dead with its return, its chance) and whether
//     the named is up; a placeholder's name, the nameds it rolls.
//   - #phcheck with a target: the same for the target, a named or a placeholder.
//   - #phcheck alone, no target: the nameds with a placeholder or up within
//     phCheckRadius of the GM, nearest first.
//
// Every line is a [GM] line under 200 characters with no '|' (a PS2's TextBox).

// phCheckUsage is #phcheck's usage line.
const phCheckUsage = "Usage: /phcheck [named or placeholder name] - target one, name one, or alone for those near you (read only)"

// phCheckHelp is /help's line for /phcheck, to a sub GM (a GM's is in #help).
const phCheckHelp = "  /phcheck [name] - placeholders: a named's PH spawns (up/dead, respawn, odds), a PH's named, or alone those near you; /t phcheck on a PS2"

// phCheckRadius is how near #phcheck alone looks.
const phCheckRadius = 400

// phCheckSpotLines is the most spots #phcheck lists of one named.
const phCheckSpotLines = 8

// phCheckGroupLines is the most nameds #phcheck alone lists.
const phCheckGroupLines = 6

// phLine is s fit for a PS2's text box: no '|', at most 190 characters.
func phLine(s string) string {
	s = strings.ReplaceAll(s, "|", "/")
	if len(s) > 190 {
		s = s[:187] + "..."
	}
	return s
}

// phLeft is a time left as a GM reads it: "45s", "4m10s", "1h02m".
func phLeft(d time.Duration) string {
	if d < time.Second {
		return "now"
	}
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d/time.Second))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%02ds", int(d/time.Minute), int(d%time.Minute/time.Second))
	}
	return fmt.Sprintf("%dh%02dm", int(d/time.Hour), int(d%time.Hour/time.Minute))
}

// phWhere is a place as the GM reads it: how far, in its world, or which world.
type phWhere struct {
	world   uint8
	x, z    float32
	gmWorld uint8
	gx, gz  float32
}

func (p phWhere) dist() float64 {
	if p.world != p.gmWorld {
		return math.Inf(1)
	}
	dx, dz := float64(p.x-p.gx), float64(p.z-p.gz)
	return math.Sqrt(dx*dx + dz*dz)
}

func (p phWhere) String() string {
	if p.world != p.gmWorld {
		return fmt.Sprintf("world %d (%.0f, %.0f)", p.world, p.x, p.z)
	}
	return fmt.Sprintf("%.0f away (%.0f, %.0f)", p.dist(), p.x, p.z)
}

// phState is a spot's state: up (as what), or dead and when it is back.
func phState(s entity.PHSpot, named string, now time.Time) string {
	switch {
	case !s.Dead && s.AsNamed:
		return "UP as " + named
	case !s.Dead:
		return "up"
	case s.Held:
		return "dead, held by a GM"
	case s.Back.IsZero():
		return "dead"
	case s.Corpse:
		return "corpse lies, back in ~" + phLeft(s.Back.Sub(now))
	default:
		return "dead, back in " + phLeft(s.Back.Sub(now))
	}
}

// phPct is a percent to a tenth.
func phPct(p float64) string {
	return strings.TrimSuffix(fmt.Sprintf("%.1f", math.Round(p*10)/10), ".0") + "%"
}

// phNamedUp says whether a named is up, and where the nearest is.
func phNamedUp(g entity.PHGroup, at func(uint8, float32, float32) phWhere) string {
	if len(g.Up) == 0 {
		return "not up"
	}
	best := g.Up[0]
	for _, u := range g.Up[1:] {
		if at(u.World, u.X, u.Z).dist() < at(best.World, best.X, best.Z).dist() {
			best = u
		}
	}
	s := fmt.Sprintf("UP (row %d, %s", best.ID, at(best.World, best.X, best.Z))
	if !best.OnSpot {
		s += ", not on a PH"
	}
	s += ")"
	if len(g.Up) > 1 {
		s += fmt.Sprintf(", %d up in all", len(g.Up))
	}
	return s
}

// phCheckCommand is #phcheck [name] (the file's doc).
func (h *ChatHandler) phCheckCommand(sess *session.Session, player *entity.Player, args []string) {
	now := time.Now()
	arg := strings.TrimSpace(strings.Join(args, " "))
	if strings.EqualFold(arg, "help") || arg == "?" {
		h.gmReply(sess, phCheckUsage)
		return
	}
	groups := h.gameWorld.PHGroups(now)
	at := func(world uint8, x, z float32) phWhere {
		return phWhere{world: world, x: x, z: z, gmWorld: player.World, gx: player.X, gz: player.Z}
	}
	say := func(s string) { h.gmReply(sess, phLine(s)) }

	if arg != "" {
		key := database.NPCClassKey(arg)
		for _, g := range groups {
			if g.Key == key {
				h.phNamedLines(say, g, at, now)
				return
			}
		}
		if h.phPlaceholderNameLines(say, arg, key, groups, at) {
			return
		}
		say(fmt.Sprintf("%s: no named with a placeholder and no placeholder is called that. %s", arg, phCheckUsage))
		return
	}

	if player.TargetObjectID != 0 && player.TargetObjectID != 0xFFFFFFFF {
		ent, ok := h.gameWorld.FindEntityByObjectID(sess, player.TargetObjectID)
		if !ok {
			say("/phcheck: your target has left the world")
			return
		}
		npc, isNPC := ent.(*entity.NPC)
		if !isNPC {
			say("/phcheck: your target is no creature. Target a named or a placeholder, or clear your target for those near you.")
			return
		}
		name, id := npc.Name, npc.ServerID
		key := database.NPCClassKey(name)
		for _, g := range groups {
			if g.Key == key {
				h.phNamedLines(say, g, at, now)
				return
			}
		}
		if h.phSpotLines(say, id, groups, at, now) {
			return
		}
		say(fmt.Sprintf("%s (row %d) is neither a named with a placeholder nor a placeholder.", name, id))
		return
	}

	h.phNearLines(say, groups, at, player.World)
}

// phNamedLines is a named's lines: whether it is up, then its spots nearest
// first.
func (h *ChatHandler) phNamedLines(say func(string), g entity.PHGroup, at func(uint8, float32, float32) phWhere, now time.Time) {
	head := fmt.Sprintf("%s: %d placeholder%s, %s.", g.Named, len(g.Spots), plural1(len(g.Spots)), phNamedUp(g, at))
	if g.OneAtATime {
		head += " Its npcrare PHs roll only while none is up."
	}
	say(head)
	spots := append([]entity.PHSpot(nil), g.Spots...)
	sort.SliceStable(spots, func(i, j int) bool {
		return at(spots[i].World, spots[i].X, spots[i].Z).dist() < at(spots[j].World, spots[j].X, spots[j].Z).dist()
	})
	for i, s := range spots {
		if i == phCheckSpotLines {
			say(fmt.Sprintf("  ...and %d more, farther.", len(spots)-i))
			break
		}
		say(fmt.Sprintf("  %d %s, %s: %s, %s", s.ID, s.Own, at(s.World, s.X, s.Z), phState(s, g.Named, now), phPct(s.Chance)))
	}
}

// phSpotLines is a placeholder spot's lines, by its row; false when it is no
// placeholder.
func (h *ChatHandler) phSpotLines(say func(string), id entity.EntityID, groups []entity.PHGroup, at func(uint8, float32, float32) phWhere, now time.Time) bool {
	var spot entity.PHSpot
	var mine []entity.PHGroup
	for _, g := range groups {
		for _, s := range g.Spots {
			if s.ID == id {
				spot = s
				mine = append(mine, g)
			}
		}
	}
	if len(mine) == 0 {
		return false
	}
	state := phState(spot, "", now)
	if !spot.Dead && database.NPCClassKey(spot.Name) != database.NPCClassKey(spot.Own) {
		state = "up as " + spot.Name
	}
	say(fmt.Sprintf("%s (row %d) is a placeholder, %s: %s.", spot.Own, id, at(spot.World, spot.X, spot.Z), state))
	say("  Pops: " + spot.Odds)
	for _, g := range mine {
		for _, s := range g.Spots {
			if s.ID == id {
				say(fmt.Sprintf("  %s %s a respawn; %s (%d PH%s).", g.Named, phPct(s.Chance), phNamedUp(g, at), len(g.Spots), plural1(len(g.Spots))))
			}
		}
	}
	return true
}

// phPlaceholderNameLines is a placeholder's lines by its name: the nameds its
// spots roll. False when no spot is of the name.
func (h *ChatHandler) phPlaceholderNameLines(say func(string), arg, key string, groups []entity.PHGroup, at func(uint8, float32, float32) phWhere) bool {
	type hit struct {
		g      entity.PHGroup
		n      int
		lo, hi float64
	}
	var hits []hit
	all := map[entity.EntityID]bool{}
	shown := arg
	for _, g := range groups {
		x := hit{g: g}
		for _, s := range g.Spots {
			if database.NPCClassKey(s.Own) != key && database.NPCClassKey(s.Name) != key {
				continue
			}
			if database.NPCClassKey(s.Own) == key {
				shown = s.Own
			}
			if x.n == 0 || s.Chance < x.lo {
				x.lo = s.Chance
			}
			x.hi = max(x.hi, s.Chance)
			x.n++
			all[s.ID] = true
		}
		if x.n > 0 {
			hits = append(hits, x)
		}
	}
	if len(hits) == 0 {
		return false
	}
	say(fmt.Sprintf("%s is a placeholder on %d spot%s, for %d named%s:", shown, len(all), plural1(len(all)), len(hits), plural1(len(hits))))
	for i, x := range hits {
		if i == phCheckGroupLines {
			say(fmt.Sprintf("  ...and %d more.", len(hits)-i))
			break
		}
		odds := phPct(x.lo)
		if x.hi != x.lo {
			odds += "-" + phPct(x.hi)
		}
		say(fmt.Sprintf("  %s %s a respawn on %d of its %d PHs; %s. /phcheck %s lists them.", x.g.Named, odds, x.n, len(x.g.Spots), phNamedUp(x.g, at), x.g.Named))
	}
	return true
}

// phNearLines is #phcheck alone: the nameds with a placeholder spot or one up
// within phCheckRadius, nearest first.
func (h *ChatHandler) phNearLines(say func(string), groups []entity.PHGroup, at func(uint8, float32, float32) phWhere, world uint8) {
	type near struct {
		g    entity.PHGroup
		d    float64 // its nearest spot or one of it up
		sd   float64 // its nearest spot
		spot entity.PHSpot
		in   int // its spots within the radius
	}
	var list []near
	for _, g := range groups {
		n := near{g: g, d: math.Inf(1), sd: math.Inf(1)}
		for _, s := range g.Spots {
			d := at(s.World, s.X, s.Z).dist()
			if d <= phCheckRadius {
				n.in++
			}
			if d < n.sd {
				n.sd, n.spot = d, s
			}
		}
		n.d = n.sd
		for _, u := range g.Up {
			if d := at(u.World, u.X, u.Z).dist(); d < n.d {
				n.d = d
			}
		}
		if n.d <= phCheckRadius {
			list = append(list, n)
		}
	}
	if len(list) == 0 {
		say(fmt.Sprintf("No named with a placeholder within %d of you. %s", phCheckRadius, phCheckUsage))
		return
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].d < list[j].d })
	say(fmt.Sprintf("Nameds with placeholders within %d (%d):", phCheckRadius, len(list)))
	for i, n := range list {
		if i == phCheckGroupLines {
			say(fmt.Sprintf("  ...and %d more. /phcheck <name> for one.", len(list)-i))
			break
		}
		line := fmt.Sprintf("  %s: %s; %d PH%s near (%d in all)", n.g.Named, phNamedUp(n.g, at), n.in, plural1(n.in), len(n.g.Spots))
		if n.spot.ID != 0 && !math.IsInf(n.sd, 1) {
			line += fmt.Sprintf(", nearest row %d %s %.0f away, %s", n.spot.ID, n.spot.Own, n.sd, phPct(n.spot.Chance))
		}
		say(line + ".")
	}
}

// plural1 is "s" but for one.
func plural1(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
