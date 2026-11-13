package entity

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
)

// hazards.go — places that hurt a player standing in them (table hazards, database/hazards.go; the
// GM Hoofbite's petition 1937, 2026-11-13: "spikey vines in toxxulia cave should damage player for
// 1000 dmg when walking through them. player must navigate around vines").
//
// An area is a capsule a row gives (database.HazardHolds: its feet within radius across of a
// segment, and from below under its height to above over it). hazardSystem looks at every living
// player twice a second: one standing in an area is hit for its damage at once, and again every
// every_ms while it stays in one (a player's clock, Player.hazardNext, so walking from one vine into
// the next does not hit twice in the time), told its words in red with the figure on it (0x00DB, as
// a drowning's), dying of it as of a drowning. Not hit: the dead, a GM flying (#fly), a session
// loading a world. The Toxxulia cave's vines are tools/seed_hazards.py's rows, read off the disc's
// zone actors (cmd/hazardscan).
//
// An on_move area (the GM Jibwiz's petition 1960, the thorns: "500 damage on a 6 second cool down
// per individual player. If 5 people walk into the thorn, all of them should take 500 damage and if
// they remain in it for 6 seconds THEN MOVE they take 500 again. However if they stay still it
// should not continuously damage them") hits a player at once on stepping in, and after every_ms
// again only once it has moved hazardStillReach from where the last hit took it. A GM places one
// with /hazard (handlers/gmhazard.go).

// hazardEvery is how often, in ticks, hazardSystem looks (0.5 s); hazardStillReach how far a
// player in an on_move area must be from its last hit's spot to be hit again.
const (
	hazardEvery      = 5
	hazardStillReach = 0.5
)

// hazardState is the hazards by the world they lie in.
type hazardState struct {
	byWorld map[uint8][]database.Hazard
	n       int
}

// SetHazards sets the hazards (table hazards).
func (w *World) SetHazards(hs []database.Hazard) {
	by := make(map[uint8][]database.Hazard)
	for _, h := range hs {
		by[h.World] = append(by[h.World], h)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.hazards = hazardState{byWorld: by, n: len(hs)}
}

// LoadHazards reads table hazards into the world: at the start and after a GM's edit. Call it
// without the database held. A row the game cannot use is logged and left out. It reports how many
// it read.
func (w *World) LoadHazards(db *database.SQLiteDB) (int, error) {
	hs, bad, err := db.Hazards()
	if err != nil {
		return 0, fmt.Errorf("hazards: %w", err)
	}
	for _, b := range bad {
		log.Printf("WARNING: hazards %s: left out", b)
	}
	w.SetHazards(hs)
	return len(hs), nil
}

// HazardsSummary says what the hazards are, for the log at the start.
func (w *World) HazardsSummary() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var nums []int
	for wn := range w.hazards.byWorld {
		nums = append(nums, int(wn))
	}
	if len(nums) == 0 {
		return "hazards: none in table hazards"
	}
	sort.Ints(nums)
	worlds := make([]string, len(nums))
	for i, wn := range nums {
		worlds[i] = fmt.Sprintf("world %d: %d", wn, len(w.hazards.byWorld[uint8(wn)]))
	}
	return fmt.Sprintf("hazards: %d in table hazards (%s)", w.hazards.n, strings.Join(worlds, ", "))
}

// hazardAtLocked is the area holding feet at (x, y, z) in world, the one of most damage when
// several do; nil for none. Must be called with the lock held.
func (w *World) hazardAtLocked(world uint8, x, y, z float32) *database.Hazard {
	var best *database.Hazard
	for i := range w.hazards.byWorld[world] {
		h := &w.hazards.byWorld[world][i]
		if (best == nil || h.Damage > best.Damage) && database.HazardHolds(*h, x, y, z) {
			best = h
		}
	}
	return best
}

// hazardWords is what a hit by h tells its player.
func hazardWords(h *database.Hazard, n int) string {
	if strings.Contains(h.Words, "%d") {
		return fmt.Sprintf(h.Words, n)
	}
	if h.Words != "" {
		return h.Words
	}
	name := h.Name
	if name == "" {
		name = "Something here"
	}
	return fmt.Sprintf("%s hurts you for %d.", name, n)
}

// hazardSystem hits each living player standing in an area (the file's doc). Must be called with
// the lock held.
func (w *World) hazardSystem(now time.Time) {
	if w.TickCount%hazardEvery != 0 || w.hazards.n == 0 {
		return
	}
	for _, p := range w.Players {
		if len(w.hazards.byWorld[p.World]) == 0 || p.HP <= 0 || p.respawnPending() || p.flyingNow(now) || now.Before(p.hazardNext) {
			continue
		}
		h := w.hazardAtLocked(p.World, p.X, p.Y, p.Z)
		if h == nil {
			p.hazardHit = false // out of every area: the next step in is a hit
			continue
		}
		if h.OnMove && p.hazardHit && distXZ(p.X, p.Z, p.hazardX, p.hazardZ) < hazardStillReach {
			continue // standing still where the last hit took it (petition 1960)
		}
		sess := w.findSessionByEntityIDLocked(p.ServerID)
		if sess != nil && sess.StateChannelsHeld() {
			continue // loading a world
		}
		p.hazardNext = now.Add(time.Duration(h.EveryOr()) * time.Millisecond)
		p.hazardX, p.hazardZ, p.hazardHit = p.X, p.Z, true
		n := min(h.Damage, p.HP)
		p.HP -= n
		p.StatsDirty = true
		log.Printf("Hazard: %s in %d %q at (%.1f, %.1f, %.1f) world=%d takes %d (HP %d/%d)",
			p.Name, h.ID, h.Name, p.X, p.Y, p.Z, p.World, n, p.HP, p.MaxHPNow())
		if sess != nil {
			me := sess.PlayerObjectID()
			w.sendDamageIndication(sess, me, -n, me, 0)
			w.sendColoredLocked(sess, hazardWords(h, n), castRefusalRGB)
		}
		if p.HP <= 0 {
			w.playerDiedLocked(sess, p, now)
		}
	}
}
