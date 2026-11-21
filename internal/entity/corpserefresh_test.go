package entity

import (
	"testing"
	"time"
)

// Heirloom petition 2590 (Mightyeagle): "checking body doesnt refresh timer like it should".
// A corpse opened near its end lies the corpse decay (3 minutes) from the open: one with no
// linger (an instance's) opened 10 s before it would go is still there at its old end and goes
// 3 minutes after the open; a corpse of another death is left alone. The control: unopened,
// it goes at its old end.
func TestACorpseOpenedLiesItsDecayFromTheOpen(t *testing.T) {
	for _, opened := range []bool{false, true} {
		died := time.Now().Add(-DefaultCorpseDecay + 10*time.Second)
		w, npc := corpseWorld(t, died)
		w.SetCorpseLinger(-1)
		if w.RefreshCorpse(npc.ServerID, died.Add(time.Second)) {
			t.Fatal("another death's corpse held")
		}
		var at time.Time
		if opened {
			at = time.Now()
			if !w.RefreshCorpse(npc.ServerID, died) {
				t.Fatal("the corpse opened not held")
			}
			end, ok := w.CorpseDecaysAt(npc.ServerID)
			if !ok || end.Before(at.Add(DefaultCorpseDecay)) || end.After(time.Now().Add(DefaultCorpseDecay)) {
				t.Fatalf("opened: decays %v after the open, want %v", end.Sub(at), DefaultCorpseDecay)
			}
		}
		w.mu.Lock()
		gone := w.corpseSystem(died.Add(DefaultCorpseDecay))
		w.mu.Unlock()
		if opened != (len(gone) == 0) {
			t.Fatalf("opened %v: at its old end gone %v", opened, gone)
		}
		if !opened {
			continue
		}
		w.mu.Lock()
		gone = w.corpseSystem(at.Add(DefaultCorpseDecay - time.Second))
		w.mu.Unlock()
		if len(gone) != 0 {
			t.Fatal("opened: gone before 3 minutes from the open")
		}
		w.mu.Lock()
		gone = w.corpseSystem(time.Now().Add(DefaultCorpseDecay + time.Second))
		w.mu.Unlock()
		if len(gone) != 1 || gone[0] != npc.ServerID {
			t.Fatalf("opened: not gone 3 minutes after the open: %v", gone)
		}
	}
}
