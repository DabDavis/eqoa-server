package entity

import (
	"testing"
	"time"
)

// A quest's price (questdeath.go; Heirloom petition 3488, Envaric Ascension):
// the player dies and respawns as from any death, with no debt, no tunar lost
// and, for a Hardcore character, no fall.
func TestAQuestsPriceIsADeathThatCostsNothing(t *testing.T) {
	hardcoreOn(t)
	r := newRezRig(t)
	ch := falls(t, r.w)
	r.b.Hardcore = true
	r.b.Tunar, r.b.Debt = 5000, 0
	r.c.Tunar = 700

	r.w.mu.Lock()
	okB := r.w.questDeathLocked(r.sb, r.b, r.at(0))
	okC := r.w.questDeathLocked(r.sc, r.c, r.at(0))
	again := r.w.questDeathLocked(r.sc, r.c, r.at(time.Second))
	r.w.mu.Unlock()
	if !okB || !okC || again {
		t.Fatalf("deaths %v %v, a second on the dead %v", okB, okC, again)
	}
	select {
	case f := <-ch:
		t.Fatalf("a quest's price made a Hardcore fall: %+v", f)
	case <-time.After(50 * time.Millisecond):
	}
	for _, p := range []*Player{r.b, r.c} {
		if !p.IsDead() || !p.respawnPending() || p.Fallen || p.Debt != 0 {
			t.Errorf("%s: dead %v, pending %v, fallen %v, debt %d", p.Name, p.IsDead(), p.respawnPending(), p.Fallen, p.Debt)
		}
	}
	if r.b.Tunar != 5000 || r.c.Tunar != 700 {
		t.Errorf("tunar %d, %d after the price, want 5000, 700", r.b.Tunar, r.c.Tunar)
	}
	r.respawn(PlayerRespawnDelay)
	if r.b.IsDead() || r.c.IsDead() {
		t.Error("not respawned after the delay")
	}

	// the same Hardcore character's next real death is still final
	r.die(r.b, r.sb, 30*time.Second)
	if f := oneFall(t, ch); f.Name != "Bran" || !r.b.Fallen {
		t.Errorf("a real death after the price: fall %+v, fallen %v", f, r.b.Fallen)
	}
}
