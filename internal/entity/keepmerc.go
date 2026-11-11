package entity

import (
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// keepmerc.go — a player's merc and familiar through a restart that keeps the
// sessions (keep.go), as its pet is carried (keeppet.go). KeepPlayer writes
// down what the hire's row does not hold: the merc's share of its hit points
// and power, where it stood, its stance and whether it held its place, and
// whether it was in its owner's group; the familiar's item and its share of
// hit points. The new process makes the merc again from its row's kit
// (handlers/keep.go's restoreMerc: its class, race and name, its level's
// heirlooms and book) where it stood, and the familiar from its item's row.
// A merc that was in its owner's group is in it again: the group itself,
// formed again from its players (RestoreGroups), or, its owner and it alone,
// the two of them again with no box (the client holds the group it was in
// through the restart, its m_inGroup set), at the group upkeep after the
// kept groups are formed again. A merc out of every group (its owner said no)
// is not asked again.

// KeptMerc is a player's merc as a restart carries it.
type KeptMerc struct {
	Name     string     `json:"name"`
	HPPct    float64    `json:"hp_pct"`
	PowerPct float64    `json:"power_pct"`
	Stance   byte       `json:"stance,omitempty"`
	Stay     bool       `json:"stay,omitempty"`
	StayAt   [3]float32 `json:"stay_at"`
	Place    [4]float32 `json:"place"` // x, y, z, facing
	World    uint8      `json:"world"`
	Grouped  bool       `json:"grouped,omitempty"`
	// Away: it waited outside capture the flag's arena (ctfaway.go), in no
	// group of its owner's: it comes back as its return from there does, its
	// owner asked into a group with it again
	Away bool `json:"away,omitempty"`
}

// KeptFamiliar is a player's familiar as a restart carries it: the item that
// called it (its row says the rest).
type KeptFamiliar struct {
	Item  int     `json:"item"`
	HPPct float64 `json:"hp_pct"`
}

// keptMercLocked is p's living merc as a restart carries it, nil for none.
// Must be called with the lock held.
func (w *World) keptMercLocked(p *Player) *KeptMerc {
	ms := w.mercOfLocked(p)
	if ms == nil {
		return nil
	}
	n := ms.npc
	k := &KeptMerc{Name: n.Name, HPPct: share(n.HP, n.MaxHP), PowerPct: share(n.Power, n.MaxPower), Stance: byte(ms.stance),
		Stay: ms.stay, StayAt: ms.stayAt, Place: [4]float32{n.X, n.Y, n.Z, n.Facing}, World: n.World}
	if g := w.groups[p.GroupID]; p.GroupID != 0 && g != nil && groupHas(g, n.ServerID) {
		k.Grouped = true
	}
	return k
}

// keptFamiliarLocked is p's familiar as a restart carries it, nil for none.
// Must be called with the lock held.
func (w *World) keptFamiliarLocked(p *Player) *KeptFamiliar {
	fs := w.familiars[p.ServerID]
	if fs == nil || fs.owner != p || fs.npc.Dead {
		return nil
	}
	return &KeptFamiliar{Item: fs.def.Item, HPPct: share(fs.npc.HP, fs.npc.MaxHP)}
}

// RestoreMerc gives the session's player back the merc a restart carried,
// made from kit (its row's, at its owner's level), where it stood, with its
// shares of hit points and power, its stance and its place in its owner's
// group (the file's doc). It reports whether it came back.
func (w *World) RestoreMerc(sess *session.Session, kit MercKit, k KeptMerc, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.IsDead() || p.respawnPending() || w.mercOfLocked(p) != nil {
		return false
	}
	if w.ctfKeepsOutLocked(p) {
		w.ctfMercWaitsLocked(p, sess, kit.Name) // its owner in the arena's world: it waits outside (ctfaway.go)
		return false
	}
	ms := w.spawnMercLocked(p, kit, MercStance(k.Stance), now)
	n := ms.npc
	if k.World == p.World && distXZ(k.Place[0], k.Place[2], p.X, p.Z) <= petLost {
		oldX, oldZ := n.X, n.Z
		n.X, n.Y, n.Z, n.Facing = k.Place[0], k.Place[1], k.Place[2], k.Place[3]
		w.SpatialIndex.Update(n.ServerID, oldX, oldZ, n.X, n.Z)
		ms.stay, ms.stayAt = k.Stay, k.StayAt
	}
	n.HP = keptShare(k.HPPct, n.MaxHP)
	n.Power = min(n.MaxPower, int(k.PowerPct*float64(n.MaxPower)+0.5))
	syncMercBody(ms)
	// grouped, not asked again: its client holds the group (regroup); out of
	// a group (its box was up at the restart, which the new process does not
	// hold) or waiting outside the arena, asked, as at its return from there:
	// a merc fights only in its owner's group (mercgroup.go)
	ms.asked = k.Grouped && !k.Away
	// its group place at the group upkeep, once the kept groups are formed
	// again (RestoreGroups comes after every session's character)
	ms.regroup = k.Grouped
	log.Printf("Keep: %s's mercenary %q back (%d/%d HP, %s, grouped %v)", p.Name, n.Name, n.HP, n.MaxHP,
		MercStanceNames[ms.stance], k.Grouped)
	return true
}
