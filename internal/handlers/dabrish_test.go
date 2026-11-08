package handlers

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// Dabrish (the GM Jibrish's petition 1266): his box offers two ranges of
// levels and two of mastery totals, each the next box's six or five; a level
// is set up or down as the player picks it (kept, sent as 0x0020), and a
// mastery total is made up to with unspent points, never taken down: 500 in
// all from none gives 500, then 300 gives nothing, then 1000 gives 500 more. A
// close keeps the question open (a PS2 closes one box before the pick in the
// next), an old box does nothing, and an NPC of another name is not him.
func TestDabrishSetsLevelsAndAddsMasteryPoints(t *testing.T) {
	r := newPortalRig(t)
	store, _ := gmRows(t, r.chatWorld)
	r.h.SetMasteryStore(store)
	r.ih.dabrishLevel, r.ih.dabrishMastery = r.h.applyLevel, r.h.giveMastery
	dab := &entity.NPC{ServerID: 7778, Name: DabrishName, X: 5001, Y: 50, Z: 5000, Level: 50, HP: 1000, MaxHP: 1000, NPCType: 0x80}
	r.gw.NPCs[dab.ServerID] = dab
	r.pa.Level = 10

	first := func() uint32 {
		t.Helper()
		r.drain()
		r.talk(r.anna, dab)
		got := r.msgs(r.anna)
		if len(got) != 1 || !strings.HasPrefix(got[0], "ask#") ||
			!strings.HasSuffix(got[0], "|A level from 5 to 30|A level from 35 to 60|Mastery points: 100 to 500 in all|Mastery points: 600 to 1000 in all") {
			t.Fatalf("his talk: %q", got)
		}
		return boxID(got)
	}
	// then the range's box, and its pick
	pick := func(rng, n int, wantAsk string) []string {
		t.Helper()
		r.pick(r.anna, first(), byte(rng))
		got := r.msgs(r.anna)
		if len(got) != 1 || !strings.Contains(got[0], wantAsk) {
			t.Fatalf("range %d's box: %q, want %q", rng, got, wantAsk)
		}
		r.pick(r.anna, boxID(got), byte(n))
		return r.msgs(r.anna)
	}

	got := pick(1, 2, "|Level 35|Level 40|Level 45|Level 50|Level 55|Level 60")
	row, _ := store.GetByServerID(1)
	if r.pa.Level != 45 || row.Level != 45 || !hasMsg(got, "box:Dabrish: There: you are level 45.") || !hasMsg(got, "op:0020") {
		t.Fatalf("level 45: the game %d, the row %d, sent %q", r.pa.Level, row.Level, got)
	}
	pick(0, 0, "|Level 5|Level 10|Level 15|Level 20|Level 25|Level 30")
	if row, _ = store.GetByServerID(1); r.pa.Level != 5 || row.Level != 5 {
		t.Fatalf("level 5, down: the game %d, the row %d", r.pa.Level, row.Level)
	}

	total := func() int {
		m, _ := r.gw.MasteryOf(r.pa.ServerID)
		return m.Points + m.Spent
	}
	got = pick(2, 4, "|100 in all|200 in all|300 in all|400 in all|500 in all")
	if total() != 500 || !hasMsg(got, "box:Dabrish: There: 500 mastery points more, 500 in all, 500 of them unspent.") {
		t.Fatalf("500 in all: %d, sent %q", total(), got)
	}
	got = pick(2, 2, "|300 in all")
	if total() != 500 || !hasMsg(got, "box:Dabrish: You hold 500 mastery points already*") {
		t.Fatalf("300 in all with 500: %d, sent %q", total(), got)
	}
	pick(3, 4, "|600 in all|700 in all|800 in all|900 in all|1000 in all")
	if total() != 1000 {
		t.Fatalf("1000 in all with 500: %d", total())
	}
	if m, err := store.LoadMastery(1); err != nil || m.Points+m.Spent != 1000 {
		t.Errorf("the row's mastery: %+v %v", m, err)
	}

	// a close keeps it; an old box does nothing
	box := first()
	r.pick(r.anna, box, 255)
	if got := r.msgs(r.anna); len(got) != 0 {
		t.Errorf("a close: %q", got)
	}
	r.pick(r.anna, box, 0)
	next := r.msgs(r.anna)
	if len(next) != 1 || !strings.Contains(next[0], "|Level 5|") {
		t.Fatalf("the pick after a close: %q", next)
	}
	r.pick(r.anna, box, 1)
	if got := r.msgs(r.anna); len(got) != 0 || r.pa.Level != 5 {
		t.Errorf("an old box: %q, level %d", got, r.pa.Level)
	}

	other := &entity.NPC{ServerID: 7779, Name: "Dabrishe", X: 5001, Y: 50, Z: 5000, Level: 50, HP: 1000, MaxHP: 1000, NPCType: 0x80}
	r.gw.NPCs[other.ServerID] = other
	r.drain()
	r.talk(r.anna, other)
	if got := r.msgs(r.anna); boxID(got)&0xFF000000 == dabrishBoxMark {
		t.Errorf("another name asked his question: %q", got)
	}
}

// His words fit a box (questAskChars, questAnswerChars).
func TestDabrishsWordsFitTheirBoxes(t *testing.T) {
	for _, q := range []string{dabrishQuestion, dabrishLevelAsk, dabrishCMAsk} {
		if n := len([]rune(q)); n > questAskChars {
			t.Errorf("%q is %d characters, a box shows %d", q, n, questAskChars)
		}
	}
	for _, rg := range dabrishRanges {
		if n := len([]rune(rg.label)); n > questAnswerChars {
			t.Errorf("%q is %d characters, an answer shows %d", rg.label, n, questAnswerChars)
		}
		if rg.many > maxDialogOptions {
			t.Errorf("%q has %d answers, a box holds %d", rg.label, rg.many, maxDialogOptions)
		}
	}
	if len(dabrishRanges) > maxDialogOptions {
		t.Errorf("%d ranges, a box holds %d", len(dabrishRanges), maxDialogOptions)
	}
}
