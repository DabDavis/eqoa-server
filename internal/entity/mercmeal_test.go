package entity

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Petition 3061 (mercmeal.go): every merc takes the food and drink it is given, shown among
// its effects to its group, and a caster or a hybrid says once a dip that its power is low.

// groupLines is the group chat lines (0x069E) the session was sent since the last look.
func groupLines(sess *session.Session) []string {
	var out []string
	for _, m := range sess.Queue().GetPending() {
		d := m.Data()
		if opOfMsg(d) == protocol.OpcodeGroupChat {
			out = append(out, strings.ReplaceAll(string(d), "\x00", ""))
		}
	}
	return out
}

func TestMercDrinksAGrocersDrinkShownToItsGroup(t *testing.T) {
	r := newMercRig(t)
	ms := r.hire(r.kit(9, 0, 0), MercAssist) // a cleric
	m := ms.npc
	r.run(0, time.Second)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	if m.MaxPower <= 0 {
		t.Fatal("a cleric merc with no power")
	}
	ms.kit.Carried = []MercCarried{{ID: 7, PatternID: 777002, Count: 2, Name: "Elven Wine", Drink: true, PoT: 25}}
	m.Power = m.MaxPower / 2
	if r.w.mercGrocerDrinkTurnLocked(ms, r.at(2*time.Second)) {
		t.Error("drank at half its power")
	}
	m.Power = m.MaxPower / 5
	pw := m.Power
	now := r.at(3 * time.Second)
	if !r.w.mercGrocerDrinkTurnLocked(ms, now) {
		t.Fatal("at a fifth of its power it did not drink what it carries")
	}
	ae := mercDrinking(m)
	if ae == nil || ae.Name != "Elven Wine" || ae.Mods.PoT != 25 || ae.Icon != mercMealIcon {
		t.Fatalf("its drink's effect: %+v", ae)
	}
	if m.Power != pw+25 {
		t.Errorf("power %d after the drink's first tick, want %d", m.Power, pw+25)
	}
	if ms.body.Mods.PoT != 25 {
		t.Errorf("its body's PoT %d, want the drink's 25 (its regen's)", ms.body.Mods.PoT)
	}
	if ms.kit.Carried[0].Count != 1 {
		t.Error("the wine was not drunk from its pack")
	}
	// its group sees it among its effects, its time with it
	var shown *memberEffect
	for _, e := range r.w.memberEffectsLocked(m.ServerID, now.Add(10*time.Second)) {
		if e.name == "Elven Wine" {
			shown = &e
		}
	}
	if shown == nil || shown.icon != mercMealIcon || shown.leftMS == 0 || shown.leftMS > uint32(ConsumableDuration.Milliseconds()) {
		t.Errorf("the group's look at its effects: %+v", shown)
	}
	// none again while it drinks
	m.Power = 0
	if r.w.mercGrocerDrinkTurnLocked(ms, now.Add(time.Second)) {
		t.Error("drank a second while the first worked")
	}
}

func TestEveryMercEatsTheMealItCarries(t *testing.T) {
	r := newMercRig(t)
	ms := r.hire(r.kit(0, 0, 0), MercAssist) // a warrior, no alchemist
	m := ms.npc
	r.run(0, time.Second)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	m.HP = m.MaxHP / 2
	if r.w.mercEatTurnLocked(ms, r.at(2*time.Second)) {
		t.Error("a warrior with no meal ate a ration's figures (an alchemist's alone)")
	}
	ms.kit.Carried = []MercCarried{{ID: 5, PatternID: testMeal, Count: 1, Name: "Spiced Jerky", Food: true, HoT: 40}}
	hp := m.HP
	if !r.w.mercEatTurnLocked(ms, r.at(3*time.Second)) {
		t.Fatal("hurt out of a fight, the warrior did not eat the jerky it carries")
	}
	if ae := mercEating(m); ae == nil || ae.Name != "Spiced Jerky" || m.HP != hp+40 {
		t.Errorf("its meal %+v, health %d (want %d)", ae, m.HP, hp+40)
	}
	if len(ms.kit.Carried) != 0 {
		t.Error("the jerky was not eaten from its pack")
	}
}

func TestMercSaysItsPowerIsLowOnceADip(t *testing.T) {
	r := newMercRig(t)
	ms := r.hire(r.kit(13, 0, 0), MercAssist) // a wizard
	m := ms.npc
	r.run(0, time.Second)
	groupLines(r.sess)
	say := func(pct int, d time.Duration) []string {
		r.w.mu.Lock()
		m.Power = m.MaxPower * pct / 100
		r.w.mercLowPowerLocked(ms, r.at(d))
		r.w.mu.Unlock()
		return groupLines(r.sess)
	}
	if got := say(50, 2*time.Second); len(got) != 0 {
		t.Errorf("at half its power it said %q", got)
	}
	got := say(5, 3*time.Second)
	if len(got) != 1 || !strings.Contains(got[0], m.Name+" tells the group: I'm nearly out of power") {
		t.Fatalf("at 5%% of its power it said %q", got)
	}
	if got := say(4, 4*time.Second); len(got) != 0 {
		t.Errorf("said it again in the same dip: %q", got)
	}
	if got := say(20, 5*time.Second); len(got) != 0 {
		t.Errorf("said it at a fifth: %q", got)
	}
	if got := say(5, 6*time.Second); len(got) != 0 { // still the same dip: under 30% all along
		t.Errorf("said it again without coming back over 30%%: %q", got)
	}
	say(40, 7*time.Second) // back: re-armed
	if got := say(5, 8*time.Second); len(got) != 0 {
		t.Errorf("said it again within its quiet minute: %q", got)
	}
	say(40, 70*time.Second)
	if got := say(5, 71*time.Second); len(got) != 1 {
		t.Errorf("a new dip after its quiet minute: said %q", got)
	}
	// a warrior never speaks of it
	r2 := newMercRig(t)
	ms2 := r2.hire(r2.kit(0, 0, 0), MercAssist)
	r2.run(0, time.Second)
	groupLines(r2.sess)
	r2.w.mu.Lock()
	ms2.npc.MaxPower, ms2.npc.Power = 100, 1
	r2.w.mercLowPowerLocked(ms2, r2.at(2*time.Second))
	r2.w.mu.Unlock()
	if got := groupLines(r2.sess); len(got) != 0 {
		t.Errorf("a warrior said %q", got)
	}
}
