package entity

import (
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// Aggro: a creature attacks a player who comes near it (the user, 2026-10-25:
// "we also need aggro from mobs that are red faced and within level range").
// Until then a creature fought only when it was attacked (chase.go).
//
// What the player sees is what attacks: the target box's face (0x0010's
// disposition, handlers/target.go) is TargetFace, and a creature attacks on
// sight exactly the players it shows the red face to (threatensLocked): the
// Aggressive (0), which is also what a creature that attacks shows, whatever
// its standing band (CLIENT's VIWndGame::DrawTarget draws the Dubious, 1,
// alike, so no NPC shows it: a red face is one that attacks).
//
// An NPC of a faction (npcfaction, faction.go) attacks a player on sight when
// the player's standing with it is in a red band (below -350, FactionKOSBelow:
// Aggressive below -700, Dubious from there),
// at any level (a city's guards defend it whatever comes: ours), and never on
// sight otherwise, whatever its body: Neriak's ghoul servant, a creature's
// body 52 from the nearest dark elf, killed a new dark elf by the coach
// (2026-10-29) until it was of Citizens of Neriak. Its face is the standing's
// disposition (StandingDisposition). The NPC bit still means it never
// attacks, and a GM's npcaggro row for its name still decides as below;
// world.faction_aggro false leaves factions out of both (the rules below).
//
// Measured on Sandstorm (faces off 0x0010's replies, levels off the entity
// channels; claude-notes/aggro-20261025.md):
//   - pcaps/ps2_sandstorm_1min10secs.pcap, a level-51 player among Tunaria's
//     wildlife: the level 18 and 19 creatures (a Black Bear Cub, a
//     Lavacrawler, a Young Lioness) showed the red face, the level 17 ones
//     (two Young Bears and a Black Bear Cub) the white, every one of them
//     green to the player. One kind both ways (Black Bear Cub 18 red, 17
//     white): it is the level, not the kind.
//   - The white level-17 Black Bear Cub came within 7.1 of that player and did
//     not attack; the red ones came no nearer than 38.9 and did not either.
//   - pcaps/breath_capture.pcap, a level-1 player in Neriak: a level-7 Fire Bat
//     passed within 8.0 and tavern rats within 23.5 without attacking, and a
//     level-2 Cave Minnow showed the white face. A town's creatures attack no
//     one.
//   - Con colours (calcConColor) agree with every reply the captures hold.
//
// Chosen, where no capture says more:
//   - the level range: a creature attacks a player below three times its
//     level (inAggroLevels). It fits the level-51 cut at 17/18; a window of 33
//     levels below the player fits it as well, and only another player level
//     would tell them apart;
//   - who is hostile by nature (hostileNatureLocked): an attackable creature
//     (no NPC bit 0x80) with a body that is not a players' race's, spawned
//     outside the towns (towns.go); townsfolk, guards, and bandits with
//     players' bodies are not, unless a GM's rule says so (table npcaggro, by
//     name: AggroRule; tools/seed_npc_aggro.py seeds the players' pets the
//     NPC rows hold and the Cave Minnow). The npc_type bits the loader reads
//     as hostile (FactionHostile) count for nothing here: eqoaclassic's 22
//     such NPCs hold a human yeoman at Freeport's gate (0x800, the big
//     creature's reach) and players' pets. With no race models (an empty
//     charactermodel) nothing attacks but by a GM's rule;
//   - the radius: the creature's AggroRadius, 30 (the loader's own, under the
//     38.9 measured), a GM's per name;
//   - no line of sight, and no social pull on sight: the NPCs of a creature's
//     own come only when a player attacks it (socialaggro.go);
//   - an NPC of no faction attacks by the rules above alone: until 2026-10-29
//     every character started with one gnome's capture (Qeynos -2000,
//     faction.go), so standings would have set Qeynos's guards on every
//     player; they come into it now through npcfaction, with each race,
//     class and city's own starts (factionstart).
//
// The fight it starts is a hit's (addThreatLocked), so the chase, the leash and
// the way home are the fight's own (chase.go). A creature already fighting,
// or on its way home after a fight, looks for no one.

// DefaultAggroRadius is how near a creature notices a player it attacks on
// sight when nothing else says: the loader's own AggroRadius, under the 38.9 a
// red-faced creature stood off without attacking (the file's doc).
const DefaultAggroRadius float32 = 30

// aggroScanTicks: the creatures near each player look for it every this many
// ticks (half a second at the world's 100 ms).
const aggroScanTicks = 5

// The target box's faces for an NPC of no faction (0x0010's disposition, as
// faction.go's DispAggressive..DispAllied; VIWndGame::DrawTarget draws 0-1
// the red face, 2-3 the white, 4-6 the blue).
const (
	FaceHostile byte = 0 // red: it attacks you on sight, or fights you
	FaceNeutral byte = 2 // white: indifferent
	faceDefault byte = 3
	FaceAmiable byte = 4 // blue: a merchant (the NPC bit and 0x01)
	FaceAlly    byte = 5 // blue: a player, or a player's pet
)

// AggroRule is a GM's word on the creatures of one name (table npcaggro):
// whether they attack a player on sight, and how near (0: their own radius).
type AggroRule struct {
	Aggressive bool
	Radius     float32
}

// aggroState is the world's aggro: off (SetAggro), factions left out of it
// (SetFactionAggro), the GMs' rules by name, their generation (a creature's
// kept nature is looked at again when it changes), the farthest any creature
// looks, and the characters a GM's #aggro off leaves alone (by character id,
// until #aggro on or a restart).
//
// A rule is for its name as npcclass keys one (database.NPCClassKey: any case,
// trimmed, no trailing $), so one row covers every spelling the imports gave
// a name ("Cave Minnow" Sandstorm's, the lake's "cave minnow" Return Home's);
// byName keeps each spelling's lookup, made once (cacheMu: a lookup may come
// under the World's read lock).
type aggroState struct {
	off        bool
	factionOff bool
	rules      map[string]AggroRule // by NPCClassKey
	gen        uint32
	reach      float32
	ignored    map[uint32]bool
	cacheMu    sync.Mutex
	byName     map[string]aggroLookup
}

// aggroLookup is a spelling's rule, or none.
type aggroLookup struct {
	rule AggroRule
	ok   bool
}

// ruleFor is the GMs' rule for an NPC of this name, if any.
func (a *aggroState) ruleFor(name string) (AggroRule, bool) {
	if len(a.rules) == 0 {
		return AggroRule{}, false
	}
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	l, seen := a.byName[name]
	if !seen {
		l.rule, l.ok = a.rules[database.NPCClassKey(name)]
		if a.byName == nil {
			a.byName = make(map[string]aggroLookup)
		}
		a.byName[name] = l
	}
	return l.rule, l.ok
}

// inAggroLevels reports a player within a creature's level range: below three
// times its level (the file's doc: a level-51 player saw the red face on level
// 18 and 19 creatures and the white on level 17).
func inAggroLevels(npcLevel, playerLevel int) bool {
	return npcLevel*3 > playerLevel
}

// hostileNatureLocked reports a creature that attacks on sight (the file's
// doc), its level range aside. Must be called with the lock held.
func (w *World) hostileNatureLocked(npc *NPC) bool {
	if npc.NPCType&0x80 != 0 {
		return false // the NPC bit: no one may attack it, so it attacks no one
	}
	if r, ok := w.aggro.ruleFor(npc.Name); ok {
		return r.Aggressive
	}
	if len(w.playerModels) == 0 {
		// no race models (charactermodel empty): a townsperson cannot be
		// told from a creature, so none attacks but by a GM's rule (a local
		// database without the table set every townsperson on its players)
		return false
	}
	if npc.ModelID == 0 || w.playerModels[uint32(npc.ModelID)] {
		return false // townsfolk, guards and bandits: a players' race's body
	}
	return !w.inTownLocked(npc.World, npc.SpawnX, npc.SpawnZ)
}

// AggroSummary is a line for the server's start: the rule, its GM rules,
// whether it can tell a townsperson from a creature, and the factions' part.
func (w *World) AggroSummary() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var s string
	switch {
	case w.aggro.off:
		return "none attacks a player on sight (world.aggro false)"
	case len(w.playerModels) == 0:
		s = fmt.Sprintf("no race models (charactermodel is empty), so only the %d names of the GMs' rules (npcaggro) attack a player on sight", len(w.aggro.rules))
	default:
		s = fmt.Sprintf("a hostile one attacks a player below three times its level within %.0f (%d GM rules by name)", DefaultAggroRadius, len(w.aggro.rules))
	}
	switch {
	case w.aggro.factionOff:
		s += "; factions count for nothing (world.faction_aggro false)"
	case w.factions.nNPC > 0:
		s += fmt.Sprintf("; an NPC of a faction (%d npcfaction rows) attacks only a player whose standing with it is below %d, at any level", w.factions.nNPC, FactionKOSBelow)
	}
	return s
}

// factionAggroLocked is the faction an NPC attacks for (the file's doc): its
// npcfaction faction, when factions count, it has no NPC bit and no GM's
// npcaggro row names it; false: the rules of no faction. Must be called with
// the lock held.
func (w *World) factionAggroLocked(npc *NPC) (string, bool) {
	if w.aggro.factionOff || npc.NPCType&0x80 != 0 {
		return "", false
	}
	if _, ok := w.aggro.ruleFor(npc.Name); ok {
		return "", false
	}
	f := w.npcFactionLocked(npc)
	return f, f != ""
}

// SetFactionAggro lets factions count in aggro and the target face, or leaves
// them out (world.faction_aggro false: the rules of no faction for every NPC).
func (w *World) SetFactionAggro(on bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.aggro.factionOff = !on
}

// hostileLocked is hostileNatureLocked, kept on the creature until the GMs'
// rules change. Must be called with the lock held.
func (w *World) hostileLocked(npc *NPC) bool {
	if npc.nature == 0 || npc.natureGen != w.aggro.gen {
		npc.nature, npc.natureGen = -1, w.aggro.gen
		if w.hostileNatureLocked(npc) {
			npc.nature = 1
		}
	}
	return npc.nature > 0
}

// threatensLocked reports a creature that attacks p on sight: alive, its own
// (no owner, not a dummy), and either of a faction p's standing with is
// Aggressive, or of none, hostile by nature and p within its level range;
// a GM trigger's hostile spawn attacks every player (gmtriggers.go).
// Whether it has noticed p is aggroSystem's. Must be called with the lock
// held.
func (w *World) threatensLocked(npc *NPC, p *Player) bool {
	if npc.Dead || npc.Owner != 0 || npc.Dummy != NotDummy {
		return false
	}
	if npc.EventAggro {
		return true
	}
	if f, ok := w.factionAggroLocked(npc); ok {
		return w.standingLocked(p, f) < FactionKOSBelow
	}
	return inAggroLevels(npc.Level, p.Level) && w.hostileLocked(npc)
}

// TargetFace is the face p's target box shows for npc (0x0010's
// disposition): the Aggressive red for a creature that attacks p on sight or
// fights it, the players' for a pet, else for an NPC of a faction p's
// standing's disposition (never Aggressive: it does not attack), else by its
// kind: a merchant's blue, everyone else's white. p nil: as no player in
// particular sees it.
func (w *World) TargetFace(p *Player, npc *NPC) byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.targetFaceLocked(p, npc)
}

func (w *World) targetFaceLocked(p *Player, npc *NPC) byte {
	switch {
	case npc.Owner != 0:
		return FaceAlly
	case p != nil && !npc.Dead && npc.CurrentEnemy == p.ServerID:
		return FaceHostile
	case p != nil && w.threatensLocked(npc, p):
		return FaceHostile
	}
	if p != nil && !w.aggro.factionOff {
		if f := w.npcFactionLocked(npc); f != "" {
			return max(StandingDisposition(w.standingLocked(p, f)), DispDubious)
		}
	}
	switch npc.Faction {
	case FactionFriendly:
		return FaceAmiable
	case FactionNeutral, FactionHostile:
		return FaceNeutral
	}
	return faceDefault
}

// aggroRadiusLocked is how near a player must come for npc to notice it: a
// GM's radius for its name, else its own, else DefaultAggroRadius. Must be
// called with the lock held.
func (w *World) aggroRadiusLocked(npc *NPC) float32 {
	if r, ok := w.aggro.ruleFor(npc.Name); ok && r.Radius > 0 {
		return r.Radius
	}
	if npc.AggroRadius > 0 {
		return npc.AggroRadius
	}
	return DefaultAggroRadius
}

// noticeableLocked reports a player a creature may notice: alive, in the
// world and not loading one (its state channels held), not invisible
// (invis.go) and not a GM who turned aggro off. Must be called with the lock
// held.
func (w *World) noticeableLocked(p *Player) bool {
	if p.IsDead() || p.respawnPending() || p.Invisible() || w.aggro.ignored[p.CharServerID] {
		return false
	}
	if w.SessionManager == nil {
		return true
	}
	sess := w.findSessionByEntityIDLocked(p.ServerID)
	return sess != nil && !sess.StateChannelsHeld()
}

// aggroSystem is the creatures' first move (the file's doc): every
// aggroScanTicks, the creatures within reach of each player it may notice
// look at it, and one that attacks it on sight, free and near enough, starts
// a fight with it as a hit would. Must be called with the lock held.
func (w *World) aggroSystem(now time.Time) {
	if w.aggro.off || w.TickCount%aggroScanTicks != 0 || w.SpatialIndex == nil {
		return
	}
	reach := max(w.aggro.reach, DefaultAggroRadius)
	for _, p := range w.Players {
		if !w.noticeableLocked(p) {
			continue
		}
		for _, id := range w.SpatialIndex.QueryRadius(p.X, p.Z, reach) {
			npc, ok := w.NPCs[id]
			if !ok || npc.World != p.World || npc.CurrentEnemy != 0 || npc.chase.returning || now.Before(npc.HiddenUntil) {
				continue
			}
			if !w.threatensLocked(npc, p) {
				continue
			}
			r := w.aggroRadiusLocked(npc)
			dx, dy, dz := p.X-npc.X, p.Y-npc.Y, p.Z-npc.Z
			d2 := dx*dx + dy*dy + dz*dz
			if d2 > r*r {
				continue
			}
			// a hit's own entry, but no one else told: the NPCs of its
			// own come only to a player's attack (socialaggro.go)
			w.addThreatLocked(npc, p.ServerID, 1)
			log.Printf("Aggro: %s (level %d) attacks %s (level %d) from %.1f away",
				npc.Name, npc.Level, p.Name, p.Level, math.Sqrt(float64(d2)))
		}
	}
}

// SetAggro turns the creatures' attacks on sight on or off (world.aggro).
func (w *World) SetAggro(on bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.aggro.off = !on
}

// AggroOff reports the creatures' attacks on sight turned off (world.aggro
// false).
func (w *World) AggroOff() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.aggro.off
}

// SetAggroRules sets the GMs' rules by NPC name (table npcaggro): at the start
// and when a GM edits one. Every creature's nature is looked at again.
func (w *World) SetAggroRules(rules map[string]AggroRule) {
	w.mu.Lock()
	defer w.mu.Unlock()
	// by NPCClassKey; two spellings of one name: the peaceful rule wins (the
	// GMs' rows agree on eqoaclassic, every spelling pair of them)
	keyed := make(map[string]AggroRule, len(rules))
	for name, r := range rules {
		k := database.NPCClassKey(name)
		if o, dup := keyed[k]; dup && !o.Aggressive && r.Aggressive {
			continue
		}
		keyed[k] = r
	}
	w.aggro.rules = keyed
	w.aggro.cacheMu.Lock()
	w.aggro.byName = nil
	w.aggro.cacheMu.Unlock()
	w.aggro.gen++
	w.aggro.reach = DefaultAggroRadius
	for _, r := range rules {
		if r.Aggressive && r.Radius > w.aggro.reach {
			w.aggro.reach = r.Radius
		}
	}
}

// LoadAggroRules reads table npcaggro (database.NPCAggroRules) into the world:
// at the start and after a GM's edit. Call it without the database held. It
// reports how many rules it read.
func (w *World) LoadAggroRules(db *database.SQLiteDB) (int, error) {
	rows, err := db.NPCAggroRules()
	if err != nil {
		return 0, fmt.Errorf("npcaggro: %w", err)
	}
	rules := make(map[string]AggroRule, len(rows))
	for _, r := range rows {
		rules[r.Name] = AggroRule{Aggressive: r.Aggressive, Radius: float32(r.Radius)}
	}
	w.SetAggroRules(rules)
	return len(rules), nil
}

// AggroIgnored reports whether creatures leave the session's character alone
// (a GM's #aggro off), and its name; ok false out of the world.
func (w *World) AggroIgnored(sess *session.Session) (ignored bool, name string, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false, "", false
	}
	return w.aggro.ignored[p.CharServerID], p.Name, true
}

// SetAggroIgnored is a GM's #aggro: ignored, no creature attacks the session's
// character on sight until it is set back (or the server restarts); a fight
// already on goes on. It reports false out of the world.
func (w *World) SetAggroIgnored(sess *session.Session, ignored bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	if ignored {
		if w.aggro.ignored == nil {
			w.aggro.ignored = make(map[uint32]bool)
		}
		w.aggro.ignored[p.CharServerID] = true
	} else {
		delete(w.aggro.ignored, p.CharServerID)
	}
	return true
}
