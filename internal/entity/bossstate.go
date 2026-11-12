package entity

import (
	"strings"
	"time"
)

// What an overseer tells of a boss (handlers/overseer.go; petition 1699,
// Skywalker: "Overseer Yurgulch can be used to check whether Bertoxxulous,
// Uzrathixius, or Poxathixius spawns are up and available to kill"): whether
// one of the name stands alive in a world now, and if not, when its row brings
// it back.

// BossState is a boss by its name in one world, as the world holds it now.
type BossState struct {
	// Up: one of the name is alive in the world (its row's or a spawn's copy).
	Up bool
	// Back is when the first of its rows' dead comes back (zero: none will by
	// its respawn: no row of the name stands in the world, or a GM holds every one).
	Back time.Time
	// Rows is how many of its own rows stand in the world (not spawn-only, not a
	// copy): 0 for a boss only a spawn (a trigger's, a quest's) brings.
	Rows int
}

// BossState is the boss called name (any case) in world at now. A spawn-only
// original (spawnonly.go) is no boss in the world; a temporary copy alive is up
// but never comes back once dead; a dead row comes back its respawn time after
// its corpse goes (corpse.go), a GM's Kill Spawner holding it (GMHeld) never.
func (w *World) BossState(name string, world uint8, now time.Time) BossState {
	name = strings.TrimSpace(name)
	var st BossState
	if name == "" {
		return st
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, n := range w.NPCs {
		if n.World != world || n.SpawnOnly || !strings.EqualFold(strings.TrimSpace(n.Name), name) {
			continue
		}
		if !n.Dead {
			st.Up = true
		}
		if n.Temporary {
			continue
		}
		st.Rows++
		if !n.Dead || n.GMHeld || n.DeathTime.IsZero() {
			continue
		}
		gone := n.CorpseGone
		if gone.IsZero() {
			gone = n.corpseEnds(w.corpseDecay)
		}
		back := gone.Add(n.RespawnTime)
		if back.Before(now) {
			back = now // due at the next tick
		}
		if st.Back.IsZero() || back.Before(st.Back) {
			st.Back = back
		}
	}
	return st
}
