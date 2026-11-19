package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// Three mercs a character (the owner, 2026-11-12): the liaison hires into the
// first free slot and no fourth; /merc lists them by slot; an order names one
// by its slot or its name (first or last), else every one out; a dismissal of
// several asks which, and one named ends its row alone.
func TestThreeMercsByCommand(t *testing.T) {
	r, m, db := newMercFixture(t)
	for _, class := range []int{0, 9, 13} {
		m.hire(r.anna, r.pa, class, 0, 0)
	}
	r.gw.AskMercGroup(r.anna)
	rows, err := db.Mercs(r.pa.CharServerID)
	if err != nil || len(rows) != 3 || rows[0].Slot != 1 || rows[1].Slot != 2 || rows[2].Slot != 3 || rows[2].Class != 13 {
		t.Fatalf("rows: %+v %v", rows, err)
	}
	if got := r.gw.MercInfosOf(r.anna); len(got) != 3 {
		t.Fatalf("%d out, want 3", len(got))
	}
	r.drain()
	m.hire(r.anna, r.pa, 4, 0, 0)
	if got := r.msgs(r.anna); !hasMsg(got, "box:Mercenary Recruitment: "+mercFullLine) {
		t.Errorf("a fourth hire: %q", got)
	}
	if rows, _ := db.Mercs(r.pa.CharServerID); len(rows) != 3 {
		t.Fatalf("a fourth hire made a row: %d", len(rows))
	}
	r.drain()
	r.say(r.anna, "/merc")
	got := r.msgs(r.anna)
	if !hasMsg(got, "chat:Your mercenaries, 3 of 3:") || !hasMsg(got, "chat:2. Your mercenary "+rows[1].Name+"*") ||
		!hasMsg(got, "chat:3. Your mercenary "+rows[2].Name+"*") {
		t.Errorf("/merc: %q", got)
	}
	// "/merc 2 stance passive": the second alone, its row alone
	r.say(r.anna, "/merc 2 stance passive")
	for _, info := range r.gw.MercInfosOf(r.anna) {
		if want := info.Slot == 2; (info.Stance == 1) != want {
			t.Errorf("after /merc 2 stance passive, slot %d is %v", info.Slot, info.Stance)
		}
	}
	if row, _, _ := db.Merc(r.pa.CharServerID, 2); row.Stance != 1 {
		t.Errorf("slot 2's stance not kept: %d", row.Stance)
	}
	if row, _, _ := db.Merc(r.pa.CharServerID, 1); row.Stance == 1 {
		t.Error("slot 1's row took slot 2's stance")
	}
	// by name, last: "/merc stance aggressive <name>"
	r.say(r.anna, "/merc stance aggressive "+strings.ToLower(rows[2].Name))
	for _, info := range r.gw.MercInfosOf(r.anna) {
		if want := info.Slot == 3; (info.Stance == 3) != want {
			t.Errorf("after the stance by name, slot %d is %v", info.Slot, info.Stance)
		}
	}
	// no name: every one out
	r.say(r.anna, "/merc stance assist")
	for _, info := range r.gw.MercInfosOf(r.anna) {
		if info.Stance != 2 {
			t.Errorf("/merc stance assist left slot %d %v", info.Slot, info.Stance)
		}
	}
	// a dismissal of several asks which
	r.drain()
	r.say(r.anna, "/merc dismiss")
	if got := r.msgs(r.anna); len(r.gw.MercInfosOf(r.anna)) != 3 || !hasMsg(got, "red:Name which mercenary: /merc <name or 1-3> dismiss.*") {
		t.Errorf("/merc dismiss with three: %q", got)
	}
	r.say(r.anna, "/t merc 2 dismiss")
	if got := r.gw.MercInfosOf(r.anna); len(got) != 2 || got[0].Slot != 1 || got[1].Slot != 3 {
		t.Fatalf("after /t merc 2 dismiss: %+v", got)
	}
	if _, kept, _ := db.Merc(r.pa.CharServerID, 2); kept {
		t.Error("slot 2's row kept")
	}
	r.say(r.anna, "/merc dismiss "+rows[0].Name)
	if got := r.gw.MercInfosOf(r.anna); len(got) != 1 || got[0].Slot != 3 {
		t.Fatalf("after the dismissal by name: %+v", got)
	}
	// a new hire takes the first free slot
	m.hire(r.anna, r.pa, 7, 0, 0)
	if row, kept, _ := db.Merc(r.pa.CharServerID, 1); !kept || row.Class != 7 {
		t.Errorf("the new hire's slot: %+v %v", row, kept)
	}
	// one hired: /merc dismiss needs no name
	r.say(r.anna, "/merc dismiss 1")
	r.say(r.anna, "/merc dismiss")
	if rows, _ := db.Mercs(r.pa.CharServerID); len(rows) != 0 || len(r.gw.MercInfosOf(r.anna)) != 0 {
		t.Errorf("after the last dismissals: %d rows", len(rows))
	}
}

// A login brings every hire back, by slot; one with no place in its owner's
// group goes on standby, and /merc says so.
func TestMercLoginBringsEveryHire(t *testing.T) {
	r, m, db := newMercFixture(t)
	for i, name := range []string{"Ralthas", "Ullin", "Mirra"} {
		if err := db.SetMerc(database.MercRow{ServerID: r.pa.CharServerID, Slot: i + 1, Name: name, Class: 0, Race: 0, Stance: 2}); err != nil {
			t.Fatal(err)
		}
	}
	// Anna in a group with two other players: a place for one merc
	r.gw.GroupInvite(r.anna, r.gw.IDSeenBy(r.anna, r.pb))
	r.gw.GroupAccept(r.bert, r.gw.IDSeenBy(r.bert, r.pa))
	r.gw.GroupInvite(r.anna, r.gw.IDSeenBy(r.anna, r.pc))
	r.gw.GroupAccept(r.cora, r.gw.IDSeenBy(r.cora, r.pa))
	r.drain()
	m.comeBack(r.anna, r.pa)
	out := r.gw.MercInfosOf(r.anna)
	if len(out) != 1 || out[0].Name != "Ralthas" {
		t.Fatalf("out: %+v", out)
	}
	sb := r.gw.MercStandby(r.anna)
	if len(sb) != 2 || sb[2] != "Ullin" || sb[3] != "Mirra" {
		t.Fatalf("standby: %v", sb)
	}
	r.drain()
	r.say(r.anna, "/merc")
	if got := r.msgs(r.anna); !hasMsg(got, "chat:3. Your mercenary Mirra, a Human Warrior, is on standby*") {
		t.Errorf("/merc: %q", got)
	}
}

// The owner, 2026-11-18: "/merc will only add 2 mercs back and not the
// third". A player's three died over a minute, he died after them, and the
// third's rest ended while he was dead: HireMercAt refused its return ("The
// dead hire no one.") and nothing asked again, so two came back and the third
// never did. A rested merc's return now waits for its owner to stand; and
// /merc brings every rested one, three of three. The controls: none comes to
// an owner still dead, and none past the group's place (standby).
func TestThreeMercsComeBackAfterTheirOwnersDeath(t *testing.T) {
	r, m, db := newMercFixture(t)
	now := time.Unix(1_800_000_000, 0)
	m.now = func() time.Time { return now }
	var due []func()
	var waits []time.Duration
	m.after = func(d time.Duration, f func()) { waits, due = append(waits, d), append(due, f) }
	for i, name := range []string{"Peldor", "Wilanus", "Yordor"} {
		if err := db.SetMerc(database.MercRow{ServerID: r.pa.CharServerID, Slot: i + 1, Name: name, Class: 0, Race: 0, Stance: 2}); err != nil {
			t.Fatal(err)
		}
	}
	m.comeBack(r.anna, r.pa)
	if got := r.gw.MercInfosOf(r.anna); len(got) != 3 {
		t.Fatalf("%d out at the login, want 3", len(got))
	}
	// all three die, ten seconds apart
	for slot := 1; slot <= 3; slot++ {
		name := []string{"", "Peldor", "Wilanus", "Yordor"}[slot]
		m.note(entity.MercNote{Kind: entity.MercNoteDied, Owner: r.pa, Slot: slot, Sess: r.anna, Name: name, Until: now.Add(entity.MercRest)})
		r.gw.DismissMerc(r.anna, slot, now)
		now = now.Add(10 * time.Second)
	}
	r.drain()
	if len(due) != 3 {
		t.Fatalf("%d returns asked, want 3", len(due))
	}
	// the first back before its owner dies
	now = now.Add(entity.MercRest - 20*time.Second + mercRestSlack)
	due[0]()
	if got := r.gw.MercInfosOf(r.anna); len(got) != 1 {
		t.Fatalf("%d out after the first rest, want 1", len(got))
	}
	// the owner dies; the second's and third's rests end while he is dead
	r.pa.HP, r.pa.DiedAt = 0, now
	now = now.Add(20 * time.Second)
	due[1]()
	due[2]()
	if got := r.gw.MercInfosOf(r.anna); len(got) != 1 {
		t.Fatalf("the control: %d out to a dead owner, want 1", len(got))
	}
	if len(due) != 5 || waits[3] != mercDeadOwnerRetry || waits[4] != mercDeadOwnerRetry {
		t.Fatalf("the returns refused by the death not asked again: %d asked, waits %v", len(due), waits)
	}
	// still dead at the retry: asked again, none out
	now = now.Add(mercDeadOwnerRetry)
	due[3]()
	if len(due) != 6 || len(r.gw.MercInfosOf(r.anna)) != 1 {
		t.Fatalf("a retry while dead: %d asked, %d out", len(due), len(r.gw.MercInfosOf(r.anna)))
	}
	// he stands at his bind point: the next retries bring both
	r.pa.HP, r.pa.DiedAt = r.pa.MaxHP, time.Time{}
	if r.pa.HP <= 0 {
		r.pa.HP = 100
	}
	now = now.Add(mercDeadOwnerRetry)
	due[4]()
	due[5]()
	if got := r.gw.MercInfosOf(r.anna); len(got) != 3 {
		t.Fatalf("%d out after the owner stood, want 3: %+v", len(got), got)
	}
	if len(due) != 6 {
		t.Errorf("returns asked again once he stood: %d", len(due))
	}

	// /merc brings all three rested ones at once
	for slot := 1; slot <= 3; slot++ {
		r.gw.DismissMerc(r.anna, slot, now)
	}
	r.drain()
	r.say(r.anna, "/merc")
	if got := r.gw.MercInfosOf(r.anna); len(got) != 3 {
		t.Fatalf("/merc brought %d of 3: %q", len(got), r.msgs(r.anna))
	}
}
