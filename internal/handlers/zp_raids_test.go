package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// Zaoran Plateau's Tallon and Vallon raids as tools/guidegaps/tallon.json and
// vallon.json write them, played on the triggers: what each step spawns, what a
// death or a talk moves on, and what is left at the end.

type zpSetFile struct {
	Triggers []struct {
		Slot   int             `json:"slot"`
		Name   string          `json:"name"`
		Scope  string          `json:"scope"`
		Radius float32         `json:"radius"`
		World  int             `json:"world"`
		X      float32         `json:"x"`
		Y      float32         `json:"y"`
		Z      float32         `json:"z"`
		Steps  json.RawMessage `json:"steps"`
	} `json:"triggers"`
	NPCs []struct {
		Name      string `json:"name"`
		SpawnOnly bool   `json:"spawnonly"`
	} `json:"npcs"`
	Deaths []struct {
		Name    string  `json:"name"`
		Trigger string  `json:"trigger"`
		Chance  float64 `json:"chance"`
	} `json:"deaths"`
}

// zpSet writes a guide-gap set's triggers and deaths into the test world, and the
// creatures its spawns copy, in world 5 at the place of the first step naming each.
func zpSet(w *trigWorld, set string) {
	w.t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "tools", "guidegaps", set+".json"))
	if err != nil {
		w.t.Fatal(err)
	}
	var f zpSetFile
	if err := json.Unmarshal(data, &f); err != nil {
		w.t.Fatal(err)
	}
	id := entity.EntityID(9000)
	for _, n := range w.gw.NPCs {
		id = max(id, n.ServerID+1)
	}
	// each NPC a step names stands where its first spawn is placed (a boss's own row is
	// where its copy comes), else at its trigger's place (a post talked to)
	type spot struct{ x, y, z float32 }
	at, order := map[string]spot{}, []string{}
	for _, tr := range f.Triggers {
		w.setAt(tr.Slot, tr.Name, tr.Scope, tr.Radius, tr.World, tr.X, tr.Y, tr.Z, string(tr.Steps), true)
		var steps []database.TriggerStep
		if err := json.Unmarshal(tr.Steps, &steps); err != nil {
			w.t.Fatal(err)
		}
		for _, s := range steps {
			k := strings.ToLower(s.NPC)
			switch {
			case s.Do == database.TriggerSpawn && s.World != nil:
				if old, ok := at[k]; !ok || (old == spot{tr.X, tr.Y, tr.Z}) {
					if !ok {
						order = append(order, s.NPC)
					}
					at[k] = spot{s.X, s.Y, s.Z}
				}
			case s.Do == database.TriggerAwaitTalk:
				if _, ok := at[k]; !ok {
					order = append(order, s.NPC)
					at[k] = spot{tr.X, tr.Y, tr.Z}
				}
			}
		}
	}
	// the rows the set makes spawn-only, and two already so on live: dab's Zek captain, and the sentry
	// (tallon.json's sql)
	only := map[string]bool{"zek captain": true, "zek sentry": true}
	for _, n := range f.NPCs {
		if n.SpawnOnly {
			only[strings.ToLower(n.Name)] = true
		}
	}
	for _, name := range order {
		p := at[strings.ToLower(name)]
		w.gw.NPCs[id] = &entity.NPC{ServerID: id, Name: name, HP: 1000, MaxHP: 1000, Level: 60, NPCType: 0x80,
			X: p.x, Y: p.y, Z: p.z, World: 5, SpawnOnly: only[strings.ToLower(name)]}
		id++
	}
	for _, d := range f.Deaths {
		if _, err := w.db.Exec(`INSERT OR REPLACE INTO npcdeath (name, trigger, chance) VALUES (?, ?, ?)`,
			database.NPCClassKey(d.Name), d.Trigger, d.Chance); err != nil {
			w.t.Fatal(err)
		}
	}
	if err := w.e.Reload(); err != nil {
		w.t.Fatal(err)
	}
}

// alive counts the trigger copies of a name that live.
func (w *trigWorld) alive(name string) (n int) {
	for _, s := range w.spawned() {
		if strings.EqualFold(s.Name, name) {
			n++
		}
	}
	return n
}

// first is a living trigger copy of a name, or nil.
func (w *trigWorld) first(name string) *entity.NPC {
	for _, s := range w.spawned() {
		if strings.EqualFold(s.Name, name) {
			return s
		}
	}
	return nil
}

// standing is the NPC of a name that is no trigger's copy.
func (w *trigWorld) standing(name string) *entity.NPC {
	for _, n := range w.gw.NPCs {
		if strings.EqualFold(n.Name, name) && !n.Temporary {
			return n
		}
	}
	return nil
}

// The key's bearer wakes the fortress: the sentry, Tallon's Rages, the captains and
// the halls' guards; the sentry's death crumbles the Rages and stands the golems at
// the ramps; Tallon spoken to comes with his archers; his death is the raid's end,
// fifteen minutes later everything the run brought is gone.
func TestTallonsFortressRuns(t *testing.T) {
	w := newTrigWorld(t)
	zpSet(w, "tallon")
	rs, rp := w.join(10, "Raider", 4730, 7600, 5)
	if _, err := w.e.FireNamed("Tallon's Fortress", TriggerFirer{Name: rp.Name, Sess: rs, Player: rp, Quiet: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.e.FireNamed("Fortress Post", TriggerFirer{Name: rp.Name, Sess: rs, Player: rp, Quiet: true}); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]int{"Zek sentry": 1, "Tallon's Rage": 5, "Zek captain": 12, "Zek hall guard": 10,
		"fortress golem": 0, "Tallon Zek": 0, "Zek archer": 0, "Zek assassin": 0} {
		if got := w.alive(name); got != want {
			t.Errorf("woken: %d %s, want %d", got, name, want)
		}
	}
	if _, err := w.e.FireNamed("Tallon's Fortress", TriggerFirer{Name: rp.Name, Sess: rs, Player: rp, Quiet: true}); err == nil {
		t.Fatal("a second raid woke the fortress while the first is in it")
	}
	w.kill(w.first("Zek sentry"))
	w.at(time.Second)
	if n := w.alive("Tallon's Rage"); n != 0 {
		t.Fatalf("%d of Tallon's Rages outlive the sentry", n)
	}
	if n := w.alive("fortress golem"); n != 13 {
		t.Fatalf("%d golems at the ramps' feet, want 13", n)
	}
	// the post's trap
	if !w.e.Talk(rs, rp, w.standing("fortress post")) {
		t.Fatal("the post's run did not answer")
	}
	if n := w.alive("Zek assassin"); n != 15 {
		t.Fatalf("%d assassins, want 15", n)
	}
	tallon := w.standing("Tallon Zek")
	if !w.e.Talk(rs, rp, tallon) {
		t.Fatal("Tallon's run did not answer the talk")
	}
	if tallon.HiddenUntil.IsZero() {
		t.Fatal("Tallon's own row stands beside his copy: not hidden")
	}
	if w.alive("Tallon Zek") != 1 || w.alive("Zek archer") != 6 {
		t.Fatalf("Tallon %d, archers %d: want 1 and 6", w.alive("Tallon Zek"), w.alive("Zek archer"))
	}
	w.clear()
	w.kill(w.first("Tallon Zek"))
	w.at(2 * time.Second)
	if got := strings.Join(yellow(t, rs), "|"); !strings.Contains(got, "Tallon Zek has fallen.") {
		t.Fatalf("no word of his fall: %q", got)
	}
	w.at(2*time.Second + 901*time.Second)
	for _, name := range []string{"Zek captain", "Zek hall guard", "fortress golem", "Zek archer"} {
		if n := w.alive(name); n != 0 {
			t.Errorf("%d %s left after the end", n, name)
		}
	}
	if _, err := w.e.FireNamed("Tallon's Fortress", TriggerFirer{Name: rp.Name, Sess: rs, Player: rp, Quiet: true}); err != nil {
		t.Fatalf("the fortress cannot be woken again after the raid: %v", err)
	}
}

// Tallon spawned and left alone: his stay ends, and the run does not call it a death.
func TestTallonsStayIsNoDeath(t *testing.T) {
	w := newTrigWorld(t)
	zpSet(w, "tallon")
	rs, rp := w.join(10, "Raider", 4730, 7600, 5)
	if _, err := w.e.FireNamed("Tallon's Fortress", TriggerFirer{Name: rp.Name, Sess: rs, Player: rp, Quiet: true}); err != nil {
		t.Fatal(err)
	}
	w.kill(w.first("Zek sentry"))
	w.at(time.Second)
	w.e.Talk(rs, rp, w.standing("Tallon Zek"))
	w.clear()
	w.at(31 * time.Minute)
	if w.alive("Tallon Zek") != 0 {
		t.Fatal("Tallon outstayed his thirty minutes")
	}
	if got := strings.Join(yellow(t, rs), "|"); strings.Contains(got, "has fallen") {
		t.Fatalf("his stay's end was told as his fall: %q", got)
	}
}

// Spoken to, Vallon comes with three guards; one killed, another comes fifteen
// seconds later, three at most; Vallon's death takes them all, and the raid is sent
// back to the plateau's port-in.
func TestVallonsTrainingGroundsRun(t *testing.T) {
	w := newTrigWorld(t)
	zpSet(w, "vallon")
	rs, rp := w.join(10, "Raider", 4372, 7110, 5)
	if _, err := w.e.FireNamed("Vallon Zek", TriggerFirer{Name: rp.Name, Sess: rs, Player: rp, Quiet: true}); err != nil {
		t.Fatal(err)
	}
	if w.standing("Vallon Zek").HiddenUntil.IsZero() {
		t.Fatal("Vallon's own row stands beside his copy: not hidden")
	}
	if w.alive("Vallon Zek") != 1 || w.alive("Guard of Vallon") != 3 {
		t.Fatalf("Vallon %d, guards %d: want 1 and 3", w.alive("Vallon Zek"), w.alive("Guard of Vallon"))
	}
	l := w.first("Guard of Vallon")
	w.kill(l)
	w.e.Killed(entity.KillCredit{NPCName: l.Name, NPCID: l.ServerID})
	w.at(time.Second)
	if n := w.alive("Guard of Vallon"); n != 2 {
		t.Fatalf("%d guards a second after one fell, want 2", n)
	}
	w.at(17 * time.Second)
	if n := w.alive("Guard of Vallon"); n != 3 {
		t.Fatalf("%d guards after the respawn, want 3", n)
	}
	w.clear()
	w.kill(w.first("Vallon Zek"))
	w.at(18 * time.Second)
	if n := w.alive("Guard of Vallon"); n != 0 {
		t.Fatalf("%d guards outlive Vallon", n)
	}
	if got := strings.Join(yellow(t, rs), "|"); !strings.Contains(got, "You have won the battle, but not the war!") || !strings.Contains(got, "Vallon Zek has fallen.") {
		t.Fatalf("no word of his fall: %q", got)
	}
	w.at(18*time.Second + 901*time.Second)
	if x, z, world, ok := teleported(rs); !ok || world != 5 || x < 4890 || x > 4910 || z < 7040 || z > 7060 {
		t.Fatalf("the raid was not sent to the port-in: %v (%.0f, %.0f) world %d", ok, x, z, world)
	}
}
