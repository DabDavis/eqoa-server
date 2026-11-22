package entity

import (
	"log"
	"math"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/eqoa/server/internal/session"
)

// hardcore.go — [HC] Hardcore characters (the owner, 2026-11-18: "lets add
// [HC] tag for characters. Hardcore mode chars stay dead and cannot respawn").
// Chosen for a character when it is made, for good (handlers/hardcore.go,
// database/hardcore.go); on a shard with world.hardcore its name is drawn
// "[HC] Name" (ShownName) and its first real death is its last:
//
//   - every death that would send it to its respawn (a creature, a PvP flag's
//     foe, a fall, drowning, lava, the sun, a GM's KILL or a GM event's spawn)
//     makes it FALLEN (playerDownLocked): no respawn ever comes due
//     (playerRespawnSystem), no rez may land or offer its way back
//     (resurrect.go), no experience debt is added (it is past paying). A duel
//     never kills (duel.go leaves the loser 1 HP) and stays so; a GM's #testmode
//     leaves 1 HP as for anyone;
//   - the fall is told once, off the lock, to the hook the handlers set
//     (OnHardcoreFall): it is written to the character's row before anything
//     else, announced to the shard, and the client sent back to its login;
//   - what killed it (fallKillerLocked) is read at the moment of death: the
//     PvP foe, else the creature with the most hate on it, else the death's
//     own cause (deathCause, set by the hazard that dealt it).

// HCTag goes before a Hardcore character's name where it is drawn.
const HCTag = "[HC] "

// hardcoreTags is world.hardcore: the [HC] tag drawn and the deaths final
// (handlers.SetHardcore).
var hardcoreTags atomic.Bool

// SetHardcoreOn turns Hardcore on or off for the shard (world.hardcore).
func SetHardcoreOn(on bool) { hardcoreTags.Store(on) }

// HardcoreOn reports world.hardcore.
func HardcoreOn() bool { return hardcoreTags.Load() }

// Death causes kept with a fall (database charhardcore's fallen_cause).
const (
	CauseCreature = "creature" // a creature's blow, spell, proc or shield
	CauseGMEvent  = "gm_event" // a creature a GM spawned or a GM trigger's
	CausePvP      = "pvp"      // a foe of the PvP flag
	CauseDrowning = "drowning"
	CauseLava     = "lava"
	CauseSun      = "sun"    // a vampire's daylight
	CauseHazard   = "hazard" // the ground giving way, a fall
	CauseGM       = "gm"     // a GM's KILL
	CauseSelf     = "self"   // a spell's price on its caster
	CauseOther    = "other"
)

// HardcoreFall is a Hardcore character's death as it happened: what the
// memorial, the fallen-heroes pages and the announcement show.
type HardcoreFall struct {
	CharServerID       uint32
	Name               string
	Level, Class, Race int
	At                 time.Time
	Cause              string // one of the Cause* words
	Killer             string // "a Zek sentry", a foe's name, "drowning"
	World              uint8
	X, Y, Z            float32
}

// HardcoreFallFunc takes a fall: called once a fall, off the World's lock, on
// a goroutine of its own. sess is the fallen character's session (nil: none).
type HardcoreFallFunc func(sess *session.Session, f HardcoreFall)

var hardcoreFallHook atomic.Pointer[HardcoreFallFunc]

// OnHardcoreFall sets the hook a fall is handed to (handlers/hardcore.go).
func (w *World) OnHardcoreFall(fn HardcoreFallFunc) {
	if fn == nil {
		hardcoreFallHook.Store(nil)
		return
	}
	hardcoreFallHook.Store(&fn)
}

// HardcoreBound reports whether a death of p is final: a Hardcore character
// on a shard with world.hardcore.
func (p *Player) HardcoreBound() bool { return p.Hardcore && HardcoreOn() }

// setDeathCause names the cause of the death about to be dealt p by a hazard
// with no creature behind it (drowning, lava, the sun); playerDownLocked reads
// and clears it.
func (p *Player) setDeathCause(cause, by string) {
	p.deathCause, p.deathBy = cause, by
}

// hardcoreFallLocked makes p fallen: called by playerDownLocked for a death of
// a HardcoreBound character, before its hate lists drop it. It reports the
// fall, which the caller hands to the hook once the death is done. Must be
// called with the lock held.
func (w *World) hardcoreFallLocked(p *Player, now time.Time) HardcoreFall {
	cause, killer := w.fallKillerLocked(p)
	p.Fallen = true
	p.Rez = RezChance{}
	p.RezReturn = RezReturn{}
	f := HardcoreFall{CharServerID: p.CharServerID, Name: p.Name, Level: p.Level, Class: p.Class, Race: p.Race, At: now,
		Cause: cause, Killer: killer, World: p.World, X: p.X, Y: p.Y, Z: p.Z}
	log.Printf("Hardcore: %s (level %d) has FALLEN at (%.1f, %.1f, %.1f) world %d: %s, %s", p.Name, p.Level, p.X, p.Y, p.Z, p.World, cause, killer)
	return f
}

// handHardcoreFall gives f to the hook on a goroutine of its own (the hook
// takes the World's lock).
func handHardcoreFall(sess *session.Session, f HardcoreFall) {
	if fn := hardcoreFallHook.Load(); fn != nil {
		go (*fn)(sess, f)
	}
}

// fallKillerLocked is what killed p: a cause set by its hazard (setDeathCause),
// else the creature hating it most (one a GM spawned, or a trigger's, a GM
// event's), else "other". Must be called with the lock held.
func (w *World) fallKillerLocked(p *Player) (cause, killer string) {
	if p.deathCause != "" {
		cause, killer = p.deathCause, p.deathBy
		p.deathCause, p.deathBy = "", ""
		return cause, killer
	}
	var best *NPC
	var bestThreat int64 = -1
	for _, npc := range w.NPCs {
		t, hates := npc.Threats[p.ServerID]
		if !hates && npc.CurrentEnemy != p.ServerID {
			continue
		}
		if npc.CurrentEnemy == p.ServerID {
			t += 1 << 40 // the one fighting it first
		}
		if t > bestThreat {
			best, bestThreat = npc, t
		}
	}
	if best == nil {
		return CauseOther, "unknown causes"
	}
	cause = CauseCreature
	if best.GMSpawn || best.EventAggro {
		cause = CauseGMEvent
	}
	return cause, KillerName(best.Name)
}

// KillerName is a creature's name as a fall's killer reads: a generic name
// lowercased as SOE wrote it gets "a"/"an" before it ("a Zek sentry"), an
// individual's stands as it is ("Lord Nagafen").
func KillerName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "a creature"
	}
	r := name[0]
	if r < 'a' || r > 'z' {
		return name
	}
	if strings.HasPrefix(name, "a ") || strings.HasPrefix(name, "an ") || strings.HasPrefix(name, "the ") {
		return name
	}
	if strings.ContainsRune("aeiou", rune(r)) {
		return "an " + name
	}
	return "a " + name
}

// NearestTown is the Tunaria coach town nearest (x, z) in world 0 and how far
// it is (any distance); ok false in any other world.
func NearestTown(world uint8, x, z float32) (string, float64, bool) {
	if world != 0 || len(coachTowns) == 0 {
		return "", 0, false
	}
	best, bestD := "", math.MaxFloat64
	for _, t := range coachTowns {
		if d := math.Hypot(float64(t.x-x), float64(t.z-z)); d < bestD {
			best, bestD = t.name, d
		}
	}
	return best, bestD, true
}

// FallenInWorld reports whether the character's Player is in the world fallen.
func (w *World) FallenInWorld(serverID uint32) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.playerByCharacterLocked(serverID)
	return p != nil && p.Fallen
}

// SetPlayerHardcore records the session's character's Hardcore state, read at
// its entry (handlers/hardcore.go).
func (w *World) SetPlayerHardcore(sess *session.Session, hardcore, fallen bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p := w.sessionPlayerLocked(sess); p != nil {
		p.Hardcore, p.Fallen = hardcore, fallen
	}
}

// AnnounceAll tells every player in the world a coloured line.
func (w *World) AnnounceAll(line string, rgb [3]byte) int {
	if w.SessionManager == nil {
		return 0
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	n := 0
	w.SessionManager.ForEach(func(s *session.Session) bool {
		if !s.CharacterInWorld() || s.PendingTermination() {
			return true
		}
		if _, replaced := s.ReplacedFor(); replaced {
			return true
		}
		if w.sessionPlayerLocked(s) == nil {
			return true
		}
		w.sendColoredLocked(s, line, rgb)
		n++
		return true
	})
	return n
}

// composeTags is the drawn name of a character with tags (in their order, each
// like "[GM] ") before name, fitted to ShownNameMost bytes: the tags joined
// with no space between them, one space before the name ("[OG][HC] Name"; the
// owner, 2026-11-18). The name is never cut: where it does not fit, the space
// goes first ("[OG][HC]Name", which a 15-letter name with two tags needs), then
// the tags in drop's order (the least telling first), each with its space back.
func composeTags(name string, tags []string, drop []string) string {
	keep := append([]string(nil), tags...)
	for {
		if len(keep) == 0 {
			return cutShownName(name)
		}
		var b strings.Builder
		for _, t := range keep {
			b.WriteString(strings.TrimSpace(t))
		}
		joined := b.String()
		if s := joined + " " + name; len(s) <= ShownNameMost {
			return s
		}
		if s := joined + name; len(s) <= ShownNameMost {
			return s
		}
		dropped := false
		for _, d := range drop {
			for i, t := range keep {
				if t == d {
					keep = append(keep[:i], keep[i+1:]...)
					dropped = true
					break
				}
			}
			if dropped {
				break
			}
		}
		if !dropped {
			return cutShownName(strings.Join(keep, "") + name) // only the GM's and the banner's left: cut as before
		}
	}
}

// tagDropOrder is the order tags go in when a name does not fit: an away
// player's [AFK] first (afk.go: a passing state, never at another tag's cost),
// then the [OG] (the least of what a glance needs), then a sub GM's [Test]
// (testtag.go), then the [HC]; the [GM] and a carried banner are never dropped.
var tagDropOrder = []string{AFKTag, OGTag, TestTag, HCTag}

// shownWith is name drawn with the tags gm, test, banner, og, hc and afk set (in
// that order: "[GM][Test][Banner][OG][HC][AFK] Name"), fitted (composeTags).
func shownWith(name string, gm, test, banner, og, hc, afk bool) string {
	var tags []string
	if gm {
		tags = append(tags, GMTag)
	}
	if test {
		tags = append(tags, TestTag)
	}
	if banner {
		tags = append(tags, CTFCarrierTag)
	}
	if og {
		tags = append(tags, OGTag)
	}
	if hc {
		tags = append(tags, HCTag)
	}
	if afk {
		tags = append(tags, AFKTag)
	}
	return composeTags(name, tags, tagDropOrder)
}

// ListName is a character's name in a PS2's character list (0x002C): with
// its [OG] and [HC] tags (handlers.ps2ListName), fitted to CLIENT's 23 bytes
// as the channel's name is (composeTags), the name never cut.
func ListName(name string, og, hc bool) string {
	return shownWith(name, false, false, false, og, hc, false)
}

// sortStrings is s sorted, without repeats.
func uniqueStrings(s []string) []string {
	sort.Strings(s)
	out := s[:0]
	for i, v := range s {
		if i == 0 || v != s[i-1] {
			out = append(out, v)
		}
	}
	return out
}

// HomeAt is the homeland home a place in a server's own world stands in.
func HomeAt(world uint8, x, z float32) (string, bool) {
	if !Composed(world) {
		return "", false
	}
	return coachHomeAt(world, x, z)
}
