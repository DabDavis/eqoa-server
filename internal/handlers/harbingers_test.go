package handlers

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The Harbingers of Khalimet's encounters as tools/harbingers_20261122.py writes them (its --dump: work board 682,
// 684, 699), played on the triggers: the hounds at the portal and their pack leader, the post at his death; the
// General on his isle, his hail's waves and the Assassin standing by; the Assassin's death a Dark Templar at 5%,
// a War Templar at 95%, another Assassin two minutes on; the Dark Templar's death the Viceroy's own fight, the waves
// gone; his death the raid sent to Freeport's docks; the Warlord at a camp's death, one at a time; a Gnadriin in
// place of each weary adventurer killed.

func hbSet(w *trigWorld) {
	w.t.Helper()
	py, err := exec.LookPath("python3")
	if err != nil {
		w.t.Skip("no python3 to dump the set")
	}
	out, err := exec.Command(py, "-I", filepath.Join("..", "..", "tools", "harbingers_20261122.py"), "--dump").Output()
	if err != nil {
		w.t.Fatal(err)
	}
	zpSetData(w, out)
	// every trigger as the panel would judge it
	all, err := w.db.GMTriggers()
	if err != nil {
		w.t.Fatal(err)
	}
	for _, tr := range all {
		if probs := database.TriggerProblems(tr); len(probs) > 0 {
			w.t.Errorf("%s: %v", tr.Title(), probs)
		}
	}
}

// hbKill is a credited kill of a living copy of name: dead in the world, and heard by the triggers.
func (w *trigWorld) hbKill(name string) {
	w.t.Helper()
	n := w.first(name)
	if n == nil {
		w.t.Fatalf("no living %s to kill", name)
	}
	w.kill(n)
	w.e.Killed(entity.KillCredit{NPCName: n.Name, NPCID: n.ServerID})
}

func TestHarbingersHoundsRaiseThePost(t *testing.T) {
	w := newTrigWorld(t)
	hbSet(w)
	rs, rp := w.join(10, "Questor", 17880, 20120, 0)
	if _, err := w.e.FireNamed("Khal Hounds", TriggerFirer{Name: rp.Name, Sess: rs, Player: rp, Quiet: true}); err != nil {
		t.Fatal(err)
	}
	if n := w.alive("Khal Hound"); n != 3 {
		t.Fatalf("%d hounds at the portal, want 3", n)
	}
	if _, err := w.e.FireNamed("Khal Hounds", TriggerFirer{Name: rp.Name, Sess: rs, Player: rp, Quiet: true}); err == nil {
		t.Fatal("a second questor raised a second pack while the first runs")
	}
	d := time.Second
	for k := 0; k < 9; k++ {
		w.hbKill("Khal Hound")
		d += time.Second
		w.at(d)
		d += 11 * time.Second
		w.at(d)
		if n := w.alive("Khal Hound"); n != 3 {
			t.Fatalf("after kill %d: %d hounds, want 3 (each back 10 s on)", k+1, n)
		}
		if w.alive("Khal Pack Leader") != 0 {
			t.Fatalf("the pack leader came after %d kills", k+1)
		}
	}
	w.hbKill("Khal Hound")
	d += time.Second
	w.at(d)
	if w.alive("Khal Pack Leader") != 1 || w.alive("Khal Hound") != 0 {
		t.Fatalf("after ten kills: pack leader %d, hounds %d; want 1 and 0", w.alive("Khal Pack Leader"), w.alive("Khal Hound"))
	}
	d += 12 * time.Second
	w.at(d)
	if n := w.alive("Khal Hound"); n != 0 {
		t.Fatalf("%d hounds came back after the pack leader", n)
	}
	w.clear()
	w.hbKill("Khal Pack Leader")
	d += time.Second
	w.at(d)
	post := w.first("Khal signpost")
	if post == nil || post.NPCType&0x80 == 0 {
		t.Fatalf("no untouchable post at the pack leader's death: %+v", post)
	}
	if got := strings.Join(yellow(t, rs), "|"); !strings.Contains(got, "With a flicker of energy, a post appears to have been activated.") {
		t.Errorf("the portal said %q", got)
	}
}

func TestHarbingersViceroyEncounter(t *testing.T) {
	w := newTrigWorld(t)
	hbSet(w)
	rs, rp := w.join(10, "Raider", 4455, 6460, 5)
	if _, err := w.e.FireNamed("Khalimet's General", TriggerFirer{Name: rp.Name, Sess: rs, Player: rp, Quiet: true}); err != nil {
		t.Fatal(err)
	}
	general := w.first("Khal Viceroy")
	if general == nil || general.NPCType&0x80 == 0 {
		t.Fatalf("the General is not standing untouchable: %+v", general)
	}
	if w.alive("Khal Assault Infantry") != 0 {
		t.Fatal("waves before the hail")
	}
	// the hail
	if !w.e.Talk(rs, rp, general) {
		t.Fatal("the General's hail started nothing")
	}
	if w.alive("Khal Assassin") != 1 {
		t.Fatalf("%d Assassins by the General, want 1", w.alive("Khal Assassin"))
	}
	if a := w.first("Khal Assassin"); a.EventAggro {
		t.Error("the Assassin comes unpulled")
	}
	w.at(13 * time.Second)
	if w.alive("Khal Assault Infantry") != 2 || w.alive("Khal Elite Infantry") != 1 || w.alive("Khal Cavalry") != 1 {
		t.Fatalf("the first wave: %d infantry, %d elite, %d cavalry", w.alive("Khal Assault Infantry"), w.alive("Khal Elite Infantry"), w.alive("Khal Cavalry"))
	}
	w.at(74 * time.Second)
	if n := w.alive("Khal Assault Infantry"); n != 4 {
		t.Fatalf("after the second wave: %d infantry, want 4", n)
	}
	// the Assassin's death at a 50 roll: a War Templar 30 s on, no Dark Templar; another Assassin two minutes on
	w.e.rand = func() float64 { return 0.5 }
	w.hbKill("Khal Assassin")
	w.at(75 * time.Second)
	w.at(106 * time.Second)
	if w.alive("Khal War Templar") != 1 || w.alive("Khal Dark Templar") != 0 {
		t.Fatalf("at a 50 roll: War Templar %d, Dark Templar %d; want 1 and 0", w.alive("Khal War Templar"), w.alive("Khal Dark Templar"))
	}
	w.at(196 * time.Second)
	if n := w.alive("Khal Assassin"); n != 1 {
		t.Fatalf("%d Assassins two minutes after one fell, want 1", n)
	}
	// at a 1 roll: the Dark Templar (and a War Templar, rolled apart)
	w.e.rand = func() float64 { return 0.01 }
	w.hbKill("Khal Assassin")
	w.at(197 * time.Second)
	w.at(228 * time.Second)
	if w.alive("Khal Dark Templar") != 1 {
		t.Fatalf("at a 1 roll: Dark Templar %d, want 1", w.alive("Khal Dark Templar"))
	}
	// at a 97 roll: neither
	w.e.rand = func() float64 { return 0.97 }
	w.at(318 * time.Second)
	if n := w.alive("Khal Assassin"); n != 1 {
		t.Fatalf("%d Assassins, want 1", n)
	}
	wt := w.alive("Khal War Templar")
	w.hbKill("Khal Assassin")
	w.at(319 * time.Second)
	w.at(350 * time.Second)
	if w.alive("Khal Dark Templar") != 1 || w.alive("Khal War Templar") != wt {
		t.Fatalf("at a 97 roll: Dark Templar %d, War Templar %d (was %d)", w.alive("Khal Dark Templar"), w.alive("Khal War Templar"), wt)
	}
	if v := w.alive("Khal Viceroy"); v != 1 {
		t.Fatalf("%d Viceroys before the Dark Templar's death", v)
	}
	// the Dark Templar's death: the waves gone, the General hidden, the Viceroy hostile
	w.hbKill("Khal Dark Templar")
	w.at(351 * time.Second)
	for _, n := range []string{"Khal Assault Infantry", "Khal Elite Infantry", "Khal Cavalry", "Khal Assassin", "Khal War Templar"} {
		if k := w.alive(n); k != 0 {
			t.Errorf("%d %s left at the Dark Templar's death", k, n)
		}
	}
	if general.HiddenUntil.IsZero() {
		t.Error("the General stands beside his own fight")
	}
	var foe *entity.NPC
	for _, s := range w.spawned() {
		if s.Name == "Khal Viceroy" && s.ServerID != general.ServerID {
			foe = s
		}
	}
	if foe == nil || foe.NPCType&0x80 != 0 || !foe.EventAggro {
		t.Fatalf("no hostile Viceroy: %+v", foe)
	}
	// his death: three minutes, then the docks
	w.kill(foe)
	w.at(352 * time.Second)
	w.at(533 * time.Second)
	if rp.World != 0 || rp.X < 25300 || rp.X > 25500 {
		t.Errorf("the raid was not sent to Freeport's docks: world %d (%.0f, %.0f)", rp.World, rp.X, rp.Z)
	}
	if w.alive("Khal Viceroy") != 0 {
		t.Errorf("%d Viceroys after the end", w.alive("Khal Viceroy"))
	}
}

// A hail and no Dark Templar: 15 minutes on the waves are gone and the General can be hailed again.
func TestHarbingersRunOutStandsTheGeneralAgain(t *testing.T) {
	w := newTrigWorld(t)
	hbSet(w)
	rs, rp := w.join(10, "Raider", 4455, 6460, 5)
	if _, err := w.e.FireNamed("Khalimet's General", TriggerFirer{Name: rp.Name, Sess: rs, Player: rp, Quiet: true}); err != nil {
		t.Fatal(err)
	}
	general := w.first("Khal Viceroy")
	w.e.Talk(rs, rp, general)
	if w.e.Talk(rs, rp, general) {
		t.Fatal("a second hail during the run started another")
	}
	for s := 1; s <= 902; s += 30 {
		w.at(time.Duration(s) * time.Second)
	}
	w.at(903 * time.Second)
	for _, n := range []string{"Khal Assault Infantry", "Khal Elite Infantry", "Khal Cavalry", "Khal Assassin", "Khal Berserker"} {
		if k := w.alive(n); k != 0 {
			t.Errorf("%d %s left after the run ran out", k, n)
		}
	}
	if w.alive("Khal Viceroy") != 1 {
		t.Fatalf("the General is gone with the run: %d", w.alive("Khal Viceroy"))
	}
	if !w.e.Talk(rs, rp, general) {
		t.Fatal("the General cannot be hailed again after a run without a kill")
	}
}

// A camp's death at 3% raises the Warlord where it fell, one at a time; a weary adventurer's death a Gnadriin.
func TestHarbingersWarlordAndGnadriin(t *testing.T) {
	w := newTrigWorld(t)
	hbSet(w)
	_, _ = w.join(10, "Camper", 4980, 5145, 0)
	for i, name := range []string{"Khal Warlord", "Gnadriin", "Khal Conscript", "weary adventurer"} {
		id := entity.EntityID(9900 + i)
		w.gw.NPCs[id] = &entity.NPC{ServerID: id, Name: name, HP: 100, MaxHP: 100, Level: 62, World: 0, X: 5200, Y: 75, Z: 5145,
			SpawnX: 5200, SpawnY: 75, SpawnZ: 5145, SpawnOnly: name == "Khal Warlord" || name == "Gnadriin"}
	}
	w.e.rand = func() float64 { return 0.5 }
	w.e.Killed(entity.KillCredit{NPCName: "Khal Conscript", NPCID: 9902})
	if w.alive("Khal Warlord") != 0 {
		t.Fatal("a Warlord at a 50 roll")
	}
	w.e.rand = func() float64 { return 0.02 }
	w.e.Killed(entity.KillCredit{NPCName: "Khal Conscript", NPCID: 9902})
	w.at(time.Second)
	if w.alive("Khal Warlord") != 1 {
		t.Fatalf("%d Warlords at a 2 roll, want 1", w.alive("Khal Warlord"))
	}
	w.e.Killed(entity.KillCredit{NPCName: "Khal Conscript", NPCID: 9902})
	w.at(2 * time.Second)
	if w.alive("Khal Warlord") != 1 {
		t.Fatalf("a second Warlord while one stands: %d", w.alive("Khal Warlord"))
	}
	w.e.Killed(entity.KillCredit{NPCName: "weary adventurer", NPCID: 9903})
	w.at(3 * time.Second)
	if g := w.first("Gnadriin"); g == nil || !g.EventAggro {
		t.Fatalf("no hostile Gnadriin in a weary adventurer's place: %+v", g)
	}
}
