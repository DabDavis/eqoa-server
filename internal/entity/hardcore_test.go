package entity

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// [HC] Hardcore characters (hardcore.go): a Hardcore character's first real
// death is final, its killer read as it falls; its name is drawn with the
// tags fitted to CLIENT's 23 bytes.

func hardcoreOn(t *testing.T) {
	t.Helper()
	SetHardcoreOn(true)
	SetOGTags(true)
	t.Cleanup(func() { SetHardcoreOn(false); SetOGTags(false) })
}

// falls catches the hook's falls.
func falls(t *testing.T, w *World) chan HardcoreFall {
	t.Helper()
	ch := make(chan HardcoreFall, 4)
	w.OnHardcoreFall(func(_ *session.Session, f HardcoreFall) { ch <- f })
	t.Cleanup(func() { w.OnHardcoreFall(nil) })
	return ch
}

func oneFall(t *testing.T, ch chan HardcoreFall) HardcoreFall {
	t.Helper()
	select {
	case f := <-ch:
		return f
	case <-time.After(2 * time.Second):
		t.Fatal("no fall handed to the hook")
	}
	return HardcoreFall{}
}

// Bran, Hardcore, is killed by the crab hating him: he falls (no debt, no
// chance of a rez, the hook told what killed him and where), never respawns,
// and Gnolie's Revive is refused with the Hardcore line. The control: Liss,
// not Hardcore, dies the same way and respawns at 15 s with her debt.
func TestAHardcoreDeathIsFinal(t *testing.T) {
	hardcoreOn(t)
	r := newRezRig(t)
	ch := falls(t, r.w)
	r.b.Hardcore = true
	crab := r.w.NPCs[3002]
	crab.Threats = map[EntityID]int64{r.b.ServerID: 40}
	crab.CurrentEnemy = r.b.ServerID
	r.die(r.b, r.sb, 0)
	f := oneFall(t, ch)
	if !r.b.Fallen || r.b.Debt != 0 || r.b.Rez != (RezChance{}) {
		t.Fatalf("fallen %v, debt %d, chance %+v", r.b.Fallen, r.b.Debt, r.b.Rez)
	}
	if f.Name != "Bran" || f.CharServerID != 72 || f.Level != 49 || f.Cause != CauseCreature || f.Killer != "Desert Crab" || f.World != 0 {
		t.Errorf("the fall %+v", f)
	}
	r.respawn(60 * time.Second)
	if !r.b.IsDead() || r.b.DiedAt.IsZero() {
		t.Error("a fallen Hardcore character respawned")
	}
	drain(r.sa)
	began := r.w.StartCast(r.sa, revive(), rezB, r.at(61*time.Second))
	refusedNoRez(t, began, drain(r.sa), "Bran was a Hardcore character and has fallen for good: no resurrection can bring them back.", r.a, 800, 0)
	if _, _, _, _, ok := r.w.TakeRezReturn(r.sb, true, r.at(62*time.Second)); ok {
		t.Error("a fallen character took a rez's way back")
	}
	// a second death (a hit on the body) hands nothing more
	r.die(r.b, r.sb, 63*time.Second)
	select {
	case f := <-ch:
		t.Errorf("a second fall: %+v", f)
	case <-time.After(50 * time.Millisecond):
	}

	r.die(r.c, r.sc, 0)
	if r.c.Fallen || r.c.Debt == 0 {
		t.Errorf("the control: fallen %v, debt %d", r.c.Fallen, r.c.Debt)
	}
	r.respawn(15 * time.Second)
	if r.c.IsDead() {
		t.Error("the control did not respawn")
	}
}

// A hazard names its own cause, a PvP foe its name; with world.hardcore off a
// Hardcore character dies as anyone.
func TestAHardcoreFallsCauses(t *testing.T) {
	hardcoreOn(t)
	r := newRezRig(t)
	ch := falls(t, r.w)
	r.b.Hardcore = true
	r.w.mu.Lock()
	r.b.setDeathCause(CauseDrowning, "drowning")
	r.w.playerDiedLocked(r.sb, r.b, r.at(0))
	r.w.mu.Unlock()
	if f := oneFall(t, ch); f.Cause != CauseDrowning || f.Killer != "drowning" {
		t.Errorf("drowned: %+v", f)
	}
	r.a.Hardcore = true
	r.w.mu.Lock()
	r.w.pvpDeathLocked(r.c, r.a, r.at(time.Second))
	r.w.mu.Unlock()
	if f := oneFall(t, ch); f.Cause != CausePvP || f.Killer != "Liss" {
		t.Errorf("pvp: %+v", f)
	}

	SetHardcoreOn(false)
	r.c.Hardcore = true
	r.die(r.c, r.sc, 0)
	if r.c.Fallen || r.c.Debt == 0 {
		t.Errorf("world.hardcore off: fallen %v, debt %d", r.c.Fallen, r.c.Debt)
	}
}

// The tags, joined with no space and one before the name, fitted to 23 bytes:
// the space goes first, then [OG], then [HC]; the name is never cut.
func TestTheTagsFit(t *testing.T) {
	hardcoreOn(t)
	for _, c := range []struct {
		name       string
		gm, og, hc bool
		want       string
	}{
		{"Meshuggah", false, false, true, "[HC] Meshuggah"},
		{"Meshuggah", false, true, true, "[OG][HC] Meshuggah"},
		{"Meshuggahhhhhhh", false, true, true, "[OG][HC]Meshuggahhhhhhh"}, // 15 letters: the space goes
		{"Meshuggah", true, false, false, "[GM] Meshuggah"},
		{"Meshuggah", true, true, false, "[GM][OG] Meshuggah"},
		{"Meshuggah", true, true, true, "[GM][OG][HC] Meshuggah"},        // 9 letters: all fit
		{"Meshuggahhh", true, true, true, "[GM][OG][HC]Meshuggahhh"},     // 11: no space
		{"Meshuggahhhh", true, true, true, "[GM][HC] Meshuggahhhh"},      // 12: [OG] goes
		{"Meshuggahhhhhhh", true, true, true, "[GM][HC]Meshuggahhhhhhh"}, // 15
		{"Meshuggahhhhhhh", true, false, false, "[GM] Meshuggahhhhhhh"},
	} {
		p := &Player{Name: c.name, GM: c.gm, OG: c.og, Hardcore: c.hc}
		got := p.ShownName()
		if got != c.want || len(got) > ShownNameMost {
			t.Errorf("%+v: %q, want %q", c, got, c.want)
		}
		if PlainName(got) != c.name {
			t.Errorf("PlainName(%q) = %q", got, PlainName(got))
		}
	}
	if !IsNameTag("[OG][HC]") || !IsNameTag("[HC]") || IsNameTag("[HC]Bob") || IsNameTag("Bob") {
		t.Error("IsNameTag")
	}
	names := ShownNames("Bob", false, true, true)
	if strings.Join(names, ",") != "Bob,[HC] Bob,[OG] Bob,[OG][HC] Bob" {
		t.Errorf("ShownNames %q", names)
	}
	SetHardcoreOn(false)
	if got := (&Player{Name: "Bob", Hardcore: true}).ShownName(); got != "Bob" {
		t.Errorf("world.hardcore off: %q", got)
	}
}

func TestKillerName(t *testing.T) {
	for in, want := range map[string]string{"Zek sentry": "Zek sentry", "orc pawn": "an orc pawn", "gnoll": "a gnoll",
		"a bat": "a bat", "Lord Nagafen": "Lord Nagafen", "": "a creature"} {
		if got := KillerName(in); got != want {
			t.Errorf("KillerName(%q) = %q, want %q", in, got, want)
		}
	}
}
