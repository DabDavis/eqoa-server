package entity

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// questmasters_test.go: what QuestMasters' lists (2006, the owner's source
// 2026-11-12) needed of the engine: a weapon's root proc, a calm by name, and a
// template strong in one resist more than the rest ("Avatar of Fear- *LR* AR PR
// DR").

// The Noroxus Spine Spear roots by its name: the creature held for procRootFor,
// told as a root. The controls: a roll over the chance, and a resisted one, root
// nothing.
func TestNoroxusSpineSpearRoots(t *testing.T) {
	wp, ok := WeaponProcOf(5458, "noroxus spine spear", "A masterwork spear. It reeks of the Plane of Disease.")
	if !ok || wp.Kind != ProcRoot || wp.FX != fxBuff || !strings.Contains(wp.String(), "a root for 12 s") ||
		!strings.Contains(wp.Explain(), "roots the creature for 12 s") {
		t.Fatalf("%+v %v %q", wp, ok, wp.String())
	}
	defer func(f, g func(int) int) { procIntn, rootIntn = f, g }(procIntn, rootIntn)
	rootIntn = func(int) int { return 9999 } // the root's own resist (root.go) never
	for _, c := range []struct {
		name  string
		rolls []int
		root  bool
	}{{"over the chance", []int{99}, false}, {"resisted", []int{0, 0}, false}, {"lands", []int{0, 99}, true}} {
		r := weaponRig(t, "noroxus spine spear", "")
		r.npc.Level = 20
		procIntn = dice(c.rolls...)
		now := time.Now()
		r.w.mu.Lock()
		r.w.weaponProcLocked(r.fsess, r.friend, r.npc, now)
		rooted := r.npc.rooted(now.Add(time.Second))
		later := r.npc.rooted(now.Add(procRootFor + time.Second))
		r.w.mu.Unlock()
		if rooted != c.root || later {
			t.Errorf("%s: rooted %v (want %v), still after %v: %v", c.name, rooted, c.root, procRootFor, later)
		}
	}
}

// The Song of the Melospiza calms as Sunsilver does, by its name.
func TestMelospizaCalms(t *testing.T) {
	r := weaponRig(t, "Song of the Melospiza", "A small bird if carved into the side of this wand.")
	defer func(f func(int) int) { procIntn = f }(procIntn)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.npc.Threats = map[EntityID]int64{r.friend.ServerID: 1000}
	procIntn = dice(0, 99)
	r.w.weaponProcLocked(r.fsess, r.friend, r.npc, time.Now())
	if r.npc.Threats[r.friend.ServerID] != 500 {
		t.Errorf("its hate %d, want 500", r.npc.Threats[r.friend.ServerID])
	}
}

// A kind with its own percent holds it, the rest resist_pct, any other none; a
// template's check takes "LR:75" and refuses a percent out of 1-100 or on nothing.
func TestTemplateResistOwnPercent(t *testing.T) {
	kinds := "LR:75, AR,pr,DR:50"
	for st, want := range map[Stat]int{StatLR: 75, StatAR: 40, StatPR: 40, StatDR: 50, StatFR: 0, StatCR: 0} {
		if got := templateResistPct(kinds, 40, st); got != want {
			t.Errorf("%s: %d, want %d", statName(st), got, want)
		}
		if got := resistKindHas(kinds, st); got != (want > 0) {
			t.Errorf("%s: named %v", statName(st), got)
		}
	}
	npc := &NPC{tmpl: &ClassTemplate{ClassTemplate: database.ClassTemplate{ResistKind: kinds, ResistPct: 40}}}
	for st, want := range map[Stat]int{StatLR: -25, StatAR: -60, StatFR: -100} {
		if got := creatureResists(st, npc, -100, time.Now()); got != want {
			t.Errorf("a 100 %s hit lands %d, want %d", statName(st), -got, -want)
		}
	}
	// the old form as it was: one percent for every kind it names
	npc.tmpl.ResistKind, npc.tmpl.ResistPct = "FR,CR", 50
	if got := creatureResists(StatCR, npc, -100, time.Now()); got != -50 {
		t.Errorf("FR,CR 50: a 100 CR hit lands %d", -got)
	}
	ok := database.ClassTemplate{Name: "x", Class: -1, HPPct: 100, TakenPct: 100, MeleePct: 100, ResistKind: "LR:75,AR,PR,DR", ResistPct: 50}
	if err := database.CheckClassTemplate(ok); err != nil {
		t.Errorf("LR:75,AR,PR,DR refused: %v", err)
	}
	for _, bad := range []string{"LR:0", "LR:101", "LR:x", ":50", "XR:50"} {
		b := ok
		b.ResistKind = bad
		if database.CheckClassTemplate(b) == nil {
			t.Errorf("%q taken", bad)
		}
	}
}
