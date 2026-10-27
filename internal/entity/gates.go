package entity

import (
	"fmt"
	"log"
	"math"
	"regexp"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// gates.go — the spells that send players to a named place: the wizard's
// gates, portals and evacuations and the druid's circles (jib's review, F3
// pass 19: 27 spells, wizard 20, druid 7, took their power and did nothing).
// Their lines say where ("Self, Teleports the caster to the Kara Village
// (s)", "Group, Set Maximum Power to 0, Teleport to SBC"), and table
// spellgates (database/spellgates.go, the GM panel's "Spell gates") holds
// each one's place in the game's coordinates and whom it sends, seeded by
// tools/seed_spell_gates.py.
//
// Whom: a self gate its caster; a group gate (a portal, a circle) its caster
// and the members of its group within the row's radius of it as it lands,
// alive, in its world and not loading one (Allakhazam's April 2004 records,
// archived, give the wizard's 60-second portals an AE radius of 15 and the
// other group teleports 20, GateGroupRadius); an evacuation the same group,
// to its caster's bind point, where Return Home takes the caster (ours: of
// a safe spot in the caster's zone and its bind, the bind; the lines name
// Freeport, Melton and Highpass, and a GM who wants a group landed there
// makes the row a group gate). A line that says "Set Maximum Power to 0"
// (Allakhazam: a 1 to 10 second effect of the fast group teleports) leaves
// its caster no power when it lands: ours, the caster's alone (the records
// do not say whose). What else the line says lands as any spell's (Commons
// Escape's "Increase Maximum Power by 10 for 12 seconds").
//
// Refused as the cast begins, before its power is spent and with no lockout:
// a line that teleports with no row here, and a row in a world the caster's
// game cannot load (handlers.LoadableWorlds). A member whose game cannot load
// the place is left behind, and told (the travel hook). Where they land:
// Return Home's message path (handlers' travel hook), the caster on a spot
// found on the ground near the place as the XP camps' landing finds one
// (camplanding.go: on the terrain, under the open sky, out of the water,
// clear of creatures), its group round it; a pet comes along as it does
// through any teleport (pet.go). The refusals' words are ours.

// GateGroupRadius is how near its caster a member of its group must stand
// for a group gate or an evacuation to take it, where its row gives none:
// the AE radius Allakhazam's April 2004 records give the group teleports
// (Freeport Evacuation, Stonebrunt Portal, Circle of Stonebrunt: 20).
const GateGroupRadius float32 = 20

// Where a gate lands round its place (gateLandingLocked), ours, as
// CampLanding's: on the terrain within gateRise of the place's floor, no
// floor gateOpen above it (the open air), out of the water, no creature that
// attacks on sight within gateClear (by where it stands or spawned: a roamer
// comes back; a town's guards and folk do not count) and no one's body
// within gateApart, nearest the place first, out to gateSteps' last; with no
// spot that clear, the clearest with gateCrowded at least, else the place
// itself. A place more than gateAbove over the terrain under it stands on
// what the ground files leave out (the top of a wizard's pyramid, 31 above
// its origin and 12 to 38 over the terrain round it, where the gates set a
// player: "on top of a Pyramid ... safe place", Allakhazam's comments,
// archived) and lands as written. The group lands gateRing round its caster.
const (
	gateRise    float32 = 12
	gateOpen    float32 = 8
	gateClear   float32 = campClear
	gateCrowded float32 = campCrowded
	gateApart   float32 = 2.5
	gateAbove   float32 = 6
	gateRing    float32 = 3
	gateSky     float32 = 1e6 // above every floor: the floor there is the topmost
)

var gateSteps = [...]float32{0, 10, 20, 35, 50, 70, 90, 120, 150, 200}

// The refusals' words, ours: no recording of the live game's exists.
const (
	gateNoPlace    = "That spell has no destination here."
	gateCannotLoad = "Your game cannot load %s."
)

// reGateTeleport and reGateDrain are what a gate's line says that the line
// reader leaves unread (spelltext.go): a teleport ("Teleports the Group to
// the Baga Village (se)", "teleport to SBC"), and its price ("Set Maximum
// Power to 0", "Set Max Power to 0").
var (
	reGateTeleport = regexp.MustCompile(`\bteleports?\b`)
	reGateDrain    = regexp.MustCompile(`\bset\s+max(?:imum)?\s+power\s+to\s+0\b`)
)

// gateLine reads a line for a gate's travel and its price: whether it
// teleports, and whether it takes its caster's power (a clause the line
// reader reads as a ceiling of 0, powercap.go, or one it leaves unread with
// the teleport: "Set Maximum Power to 0 and Teleport to Melton").
func gateLine(d *SpellDef) (teleports, drains bool) {
	for _, u := range d.Unread {
		teleports = teleports || reGateTeleport.MatchString(u)
		drains = drains || reGateDrain.MatchString(u)
	}
	for _, e := range d.Effects {
		drains = drains || (e.Kind == EffectPowerCap && e.Amount == 0)
	}
	return
}

// GateSpot is where a gate lands someone.
type GateSpot struct {
	World   uint8
	X, Y, Z float32
	Place   string // as a player reads it
}

// GateTravel is one player a gate sends, for the travel hook: to To, or to
// the home (Return Home's place) of the player Home names, an evacuation's
// caster (0 for none); Ring is its place round that spot (0 the spot itself,
// its caster's), Spell the gate's name.
type GateTravel struct {
	Sess  *session.Session
	P     *Player
	To    GateSpot
	Home  EntityID
	Ring  int
	Spell string
}

// TravelFunc lands a player a gate sends (handlers' teleport, Return Home's
// message path). It runs on the tick goroutine after the tick, with the
// World lock released.
type TravelFunc func(t GateTravel)

// gateState is the gates by spell (table spellgates), the travellers on
// their way, the travel hook, and the test of the worlds a session's game
// loads (handlers.LoadableWorlds).
type gateState struct {
	rows     map[string]database.SpellGate
	due      []GateTravel
	travel   TravelFunc
	loadable func(sess *session.Session) func(world uint8) bool
}

// SetSpellGates sets the gates (table spellgates), keyed by
// database.SpellGateKey.
func (w *World) SetSpellGates(rows map[string]database.SpellGate) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.gates.rows = rows
}

// LoadSpellGates reads table spellgates into the world: at the start and
// after a GM's edit. Call it without the database held. A row the game
// cannot use is logged and left out. It reports how many it read.
func (w *World) LoadSpellGates(db *database.SQLiteDB) (int, error) {
	rows, bad, err := db.SpellGates()
	if err != nil {
		return 0, fmt.Errorf("spellgates: %w", err)
	}
	for _, b := range bad {
		log.Printf("WARNING: spellgates %s: left out", b)
	}
	w.SetSpellGates(rows)
	return len(rows), nil
}

// SpellGate is the gate a spell of that name is, if it is one.
func (w *World) SpellGate(name string) (database.SpellGate, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	g, ok := w.gates.rows[database.SpellGateKey(name)]
	return g, ok
}

// SpellGatesSummary says what the gates are, for the log at the start.
func (w *World) SpellGatesSummary() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	kinds := map[string]int{}
	for _, g := range w.gates.rows {
		kinds[g.Kind]++
	}
	return fmt.Sprintf("spell gates: %d in spellgates (%d self, %d group, %d evacuations)",
		len(w.gates.rows), kinds[database.GateSelf], kinds[database.GateGroup], kinds[database.GateEvac])
}

// OnTravel sets the travel hook.
func (w *World) OnTravel(fn TravelFunc) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.gates.travel = fn
}

// SetLoadableWorlds sets the test of which worlds a session's game loads
// (handlers.LoadableWorlds); with none, every world is.
func (w *World) SetLoadableWorlds(fn func(sess *session.Session) func(world uint8) bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.gates.loadable = fn
}

// loadsLocked reports whether sess's game loads world. Must be called with
// the lock held.
func (w *World) loadsLocked(sess *session.Session, world uint8) bool {
	if w.gates.loadable == nil {
		return true
	}
	return w.gates.loadable(sess)(world)
}

// gatePlace is a gate's place as a player reads it: its row's, else the
// spell's name.
func gatePlace(g database.SpellGate) string {
	if p := strings.TrimSpace(g.Place); p != "" {
		return p
	}
	return g.Spell
}

// gateStartLocked is a gate's refusal as its cast begins, "" for none: a
// line that teleports with no row here, or a row in a world the caster's
// game cannot load (an evacuation's is its caster's bind, which its game
// loads). Must be called with the lock held.
func (w *World) gateStartLocked(sess *session.Session, spell *CastSpell) string {
	g, ok := w.gates.rows[database.SpellGateKey(spell.Name)]
	if !ok {
		if teleports, _ := gateLine(&spell.Def); teleports {
			return gateNoPlace
		}
		return ""
	}
	if g.Kind != database.GateEvac && !w.loadsLocked(sess, g.World) {
		return fmt.Sprintf(gateCannotLoad, gatePlace(g))
	}
	return ""
}

// gateLandLocked is a gate's landing: its travellers queued for the travel
// hook (runTravels), its caster's power taken when its line says. It reports
// whether the spell is a gate; the rest of its line lands as any spell's.
// Must be called with the lock held.
func (w *World) gateLandLocked(cs *castState, caster *Player, casterSess *session.Session) bool {
	g, ok := w.gates.rows[database.SpellGateKey(cs.spell.Name)]
	if !ok {
		return false
	}
	if _, drains := gateLine(&cs.spell.Def); drains && caster.Power > 0 {
		caster.Power = 0
		caster.StatsDirty = true
	}
	var at GateSpot
	where := "its caster's bind point"
	if g.Kind != database.GateEvac {
		x, y, z := w.gateLandingLocked(g)
		at = GateSpot{World: g.World, X: x, Y: y, Z: z, Place: gatePlace(g)}
		where = fmt.Sprintf("%s (%.1f, %.1f, %.1f) world %d", at.Place, x, y, z, g.World)
	}
	var names []string
	for i, t := range w.gateTravellersLocked(caster, g) {
		sess := casterSess
		if t != caster {
			sess = w.findSessionByEntityIDLocked(t.ServerID)
		}
		if sess == nil {
			continue
		}
		tr := GateTravel{Sess: sess, P: t, To: at, Ring: i, Spell: cs.spell.Name}
		if g.Kind == database.GateEvac {
			tr.Home = caster.ServerID
		}
		w.gates.due = append(w.gates.due, tr)
		names = append(names, t.Name)
	}
	log.Printf("Gate: %s's %q (%s) sends %s to %s", caster.Name, cs.spell.Name, g.Kind, strings.Join(names, ", "), where)
	return true
}

// gateTravellersLocked is whom a gate sends, its caster first: the caster
// alone for a self gate; for a group gate or an evacuation also each member
// of its group within the row's radius of it (as a spell's range, 3D),
// alive, in its world and not loading one. Must be called with the lock
// held.
func (w *World) gateTravellersLocked(caster *Player, g database.SpellGate) []*Player {
	out := []*Player{caster}
	if g.Kind == database.GateSelf {
		return out
	}
	r := g.Radius
	if r <= 0 {
		r = GateGroupRadius
	}
	for _, m := range w.groupMembersLocked(caster) {
		if m == caster || m.IsDead() || m.respawnPending() || m.World != caster.World || outOfRange(caster, m.X, m.Y, m.Z, r) {
			continue
		}
		if s := w.findSessionByEntityIDLocked(m.ServerID); s == nil || s.StateChannelsHeld() {
			continue // gone, or loading a world
		}
		out = append(out, m)
	}
	return out
}

// gateLandingLocked is where a gate to g lands (the rules above gateRise):
// the place as written without the ground, with no terrain there, or where
// it stands on what the ground leaves out. Must be called with the lock
// held.
func (w *World) gateLandingLocked(g database.SpellGate) (x, y, z float32) {
	x, y, z = g.X, g.Y, g.Z
	if w.ground == nil {
		return
	}
	// the place's floor: the terrain nearest its height; a place well above
	// every floor there stands on what the ground leaves out (a pyramid's
	// top), and one under the terrain is lifted to its top
	base, ok := w.ground.Floor(g.World, g.X, g.Y+gateRise, g.Z)
	switch {
	case ok && base < g.Y-gateAbove:
		return
	case !ok:
		if base, ok = w.ground.Floor(g.World, g.X, gateSky, g.Z); !ok {
			return
		}
	}
	y = base
	// the NPCs near: the creatures that attack on sight kept gateClear off (by
	// where they stand or spawned), every body gateApart
	reach := gateSteps[len(gateSteps)-1] + gateClear
	var hostile, bodies []*NPC
	consider := func(npc *NPC) {
		if npc == nil || npc.World != g.World || npc.Dead {
			return
		}
		near := distXZ(npc.X, npc.Z, g.X, g.Z) <= reach
		if near {
			bodies = append(bodies, npc)
		}
		if npc.Dummy != NotDummy || npc.Owner != 0 || !w.hostileLocked(npc) { // aggro.go
			return
		}
		if near || distXZ(npc.SpawnX, npc.SpawnZ, g.X, g.Z) <= reach {
			hostile = append(hostile, npc)
		}
	}
	if w.SpatialIndex != nil {
		for _, id := range w.SpatialIndex.QueryRadius(g.X, g.Z, reach+w.roamRadius) {
			consider(w.NPCs[id])
		}
	} else {
		for _, npc := range w.NPCs {
			consider(npc)
		}
	}
	room := func(px, pz float32) float32 {
		r := gateClear
		for _, npc := range hostile {
			r = min(r, distXZ(npc.SpawnX, npc.SpawnZ, px, pz), distXZ(npc.X, npc.Z, px, pz))
		}
		return r
	}
	apart := func(px, pz float32) bool {
		for _, npc := range bodies {
			if distXZ(npc.X, npc.Z, px, pz) < gateApart {
				return false
			}
		}
		return true
	}
	var bx, by, bz, broom float32
	for _, d := range gateSteps {
		n := 16
		if d == 0 {
			n = 1
		}
		for k := 0; k < n; k++ {
			h := float64(k) * math.Pi / 8
			px, pz := g.X+d*float32(math.Cos(h)), g.Z+d*float32(math.Sin(h))
			fy, ok := w.ground.Floor(g.World, px, base+gateRise, pz)
			if !ok || float32(math.Abs(float64(fy-base))) > gateRise || !apart(px, pz) {
				continue
			}
			if top, ok := w.ground.Floor(g.World, px, fy+1000, pz); ok && top > fy+gateOpen {
				continue // under a roof, an overhang, another floor
			}
			if w.waterAtLocked(g.World, px, fy+bodyAbove, pz) {
				continue
			}
			r := room(px, pz)
			if r >= gateClear {
				return px, fy, pz
			}
			if r > broom {
				bx, by, bz, broom = px, fy, pz, r
			}
		}
	}
	if broom >= gateCrowded {
		return bx, by, bz
	}
	return g.X, base, g.Z
}

// GateRingSpot is a group's ring-th place round at, gateRing off it a
// quarter turn apart (ring 0: at itself): on the terrain there when at is on
// the terrain and the ground has a floor there near at's height, out of the
// water; at at's height when at stands above the terrain (a pyramid's top);
// else at itself, as a player without the ground lands (a height guessed off
// a slope could be under it).
func (w *World) GateRingSpot(at GateSpot, ring int) GateSpot {
	if ring <= 0 {
		return at
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.ground == nil {
		return at
	}
	h := float64(ring-1) * math.Pi / 2
	r := gateRing * float32(1+(ring-1)/4)
	px, pz := at.X+r*float32(math.Cos(h)), at.Z+r*float32(math.Sin(h))
	if cf, ok := w.ground.Floor(at.World, at.X, at.Y+gateRise, at.Z); ok && cf < at.Y-gateAbove {
		return GateSpot{World: at.World, X: px, Y: at.Y, Z: pz, Place: at.Place}
	}
	fy, ok := w.ground.Floor(at.World, px, at.Y+gateRise, pz)
	if !ok || float32(math.Abs(float64(fy-at.Y))) > gateRise || w.waterAtLocked(at.World, px, fy+bodyAbove, pz) {
		return at
	}
	return GateSpot{World: at.World, X: px, Y: fy, Z: pz, Place: at.Place}
}

// runTravels hands each player a gate sent to the travel hook. Must be
// called WITHOUT the lock.
func (w *World) runTravels() {
	w.mu.Lock()
	due, fn := w.gates.due, w.gates.travel
	w.gates.due = nil
	w.mu.Unlock()
	if fn == nil {
		return
	}
	for _, t := range due {
		fn(t)
	}
}
