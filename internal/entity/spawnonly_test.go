package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// What stands out of the world (spawnonly.go): an NPC of a name in
// npcspawnonly is in no view and attacks and helps no one, where its copies
// (a quest's spawn, a GM's, a pet of its body) are in the world and fight.

// creationBody is a creature's body (no players' race has it): the beetle's
// Jarek's creation wore until 2026-10-30.
const creationBody uint32 = 0x4140233B

// inView is whether the NPC is on one of the session's channels after a
// visibility pass.
func (f *aggroFix) inView(n *NPC) bool {
	for _, id := range visible(f.w, f.sess) {
		if id == n.ServerID {
			return true
		}
	}
	return false
}

// A spawn-only creature 10 off a player is in no view, does not attack the
// player on sight nor come to a hit creature's help, and does not wander. The
// controls, of the same body and level beside it: a creature of another name,
// seen and attacking; a social guard of the victim's faction, coming.
func TestASpawnOnlyNPCIsInNoViewAndAttacksNoOne(t *testing.T) {
	f := newAggroFix(t, 10)
	creation := f.npc(2001, "Jarek's Creation", 11, creationBody, 1010, 1000)
	beetle := f.npc(2002, "giant beetle", 11, creationBody, 1000, 1010)
	if held, changed := f.w.SetSpawnOnly(map[string]bool{database.NPCClassKey("Jarek's Creation"): true}); held != 1 || changed != 1 {
		t.Fatalf("SetSpawnOnly: %d held, %d changed; want 1 and 1", held, changed)
	}
	if !creation.SpawnOnly || beetle.SpawnOnly {
		t.Fatalf("spawn-only: the creation %v, the beetle %v", creation.SpawnOnly, beetle.SpawnOnly)
	}
	if f.inView(creation) || !f.inView(beetle) {
		t.Fatalf("in view: the creation %v (want not), the beetle %v (the control, want it)", f.inView(creation), f.inView(beetle))
	}
	f.look()
	if f.attacks(creation) || len(creation.Threats) != 0 {
		t.Errorf("the spawn-only creation attacks: enemy %d, threats %v", creation.CurrentEnemy, creation.Threats)
	}
	if !f.attacks(beetle) {
		t.Errorf("the control: the beetle 10 off does not attack a level 10 (enemy %d)", beetle.CurrentEnemy)
	}
	if wanders, why, _ := f.w.movementLocked(creation); wanders || why != whySpawnOnly {
		t.Errorf("the creation's movement: wanders %v, why %d; want it standing, spawn only", wanders, why)
	}
	if st, _ := f.w.NPCStateOf(creation.ServerID); !st.SpawnOnly || st.Movement == "" {
		t.Errorf("the panel's state: %+v", st)
	}

	// the social pull: a guard hit calls the guards of its faction, never one
	// out of the world
	s := newSocialFix(t)
	victim := s.npc(100, "Guard Alinon", 30, humanBody, 1000, 1000)
	hidden := s.npc(101, "Guard Brenna", 30, humanBody, 1020, 1000)
	near := s.npc(102, "Guard Cadwell", 30, humanBody, 1000, 1020)
	s.w.SetSpawnOnly(map[string]bool{database.NPCClassKey("Guard Brenna"): true})
	s.hit(victim, 12)
	if !s.attacks(victim) || !s.attacks(near) {
		t.Fatalf("the control: the guard hit and the one 20 off fight the player: %d, %d", victim.CurrentEnemy, near.CurrentEnemy)
	}
	if hidden.CurrentEnemy != 0 {
		t.Errorf("a spawn-only guard 20 off came to the help (enemy %d)", hidden.CurrentEnemy)
	}
}

// A quest's spawn of a spawn-only NPC is a copy in the world: seen, hostile and
// at the player, while the original stays out of it; a GM's copy and a pet of
// its body are in the world too. The control: the original, before and after.
func TestASpawnOnlyNPCsCopiesAreInTheWorld(t *testing.T) {
	f := newAggroFix(t, 10)
	f.p.Facing = 0
	creation := f.npc(2001, "Jarek's Creation", 11, creationBody, 1300, 1300)
	f.w.SetSpawnOnly(map[string]bool{database.NPCClassKey("Jarek's Creation"): true})

	id, spelled, ok := f.w.SpawnForQuest("jarek's creation", f.p, true)
	if !ok || spelled != "Jarek's Creation" {
		t.Fatalf("SpawnForQuest: %d %q %v", id, spelled, ok)
	}
	c := f.w.NPCs[id]
	if c.SpawnOnly || !c.HiddenUntil.IsZero() || !c.Temporary || !c.QuestSpawn {
		t.Fatalf("the copy: spawn-only %v, hidden until %v, temporary %v, quest's %v", c.SpawnOnly, c.HiddenUntil, c.Temporary, c.QuestSpawn)
	}
	if !f.inView(c) {
		t.Errorf("the quest's copy is in no view")
	}
	if !f.attacks(c) || c.NPCType&0x80 != 0 {
		t.Errorf("the hostile copy: enemy %d, type %#x; want at the player, attackable", c.CurrentEnemy, c.NPCType)
	}
	if d := DistanceBetween(f.p, c); d > 5 {
		t.Errorf("the copy stands %.1f off the player", d)
	}
	if !creation.SpawnOnly || f.inView(creation) || creation.CurrentEnemy != 0 {
		t.Errorf("the original after its copy: spawn-only %v, in view %v, enemy %d", creation.SpawnOnly, f.inView(creation), creation.CurrentEnemy)
	}

	gm, _, ok := f.w.SpawnNPCCopy("Jarek's Creation", 1005, 10, 1005, 0, 0)
	if !ok || f.w.NPCs[gm].SpawnOnly || !f.inView(f.w.NPCs[gm]) {
		t.Errorf("a GM's copy: ok %v, out of the world", ok)
	}
	if pet := newPet(creation, 3000, f.p, "Creation", 10); pet.SpawnOnly {
		t.Errorf("a pet of the creation's body is out of the world")
	}
}

// npcspawnonly is live: an NPC marked while it fights is out of the fight, back
// on its spawn and out of view; unmarked it is seen again. A GM's edit of its row
// (UpsertNPC, ReloadNPCs) keeps it out once the refresh hold is over. The
// control: an NPC of another name, edited the same way, is in the world.
func TestSpawnOnlyIsLiveAndOutlastsARowEdit(t *testing.T) {
	f := newAggroFix(t, 10)
	creation := f.npc(2001, "Jarek's Creation", 11, creationBody, 1010, 1000)
	f.look()
	if !f.attacks(creation) {
		t.Fatalf("the control: before it is marked the creation attacks (enemy %d)", creation.CurrentEnemy)
	}
	creation.X = 1015 // chasing
	f.w.SetSpawnOnly(map[string]bool{database.NPCClassKey("Jarek's Creation"): true})
	if creation.CurrentEnemy != 0 || len(creation.Threats) != 0 || creation.X != creation.SpawnX {
		t.Errorf("marked mid-fight: enemy %d, threats %v, at %.0f (spawn %.0f)", creation.CurrentEnemy, creation.Threats, creation.X, creation.SpawnX)
	}
	if f.inView(creation) {
		t.Errorf("marked: still in view")
	}
	f.w.SetSpawnOnly(nil)
	if creation.SpawnOnly || !f.inView(creation) {
		t.Errorf("unmarked: spawn-only %v, in view %v", creation.SpawnOnly, f.inView(creation))
	}

	f.w.SetSpawnOnly(map[string]bool{database.NPCClassKey("Jarek's Creation"): true})
	row := func(id int, name string) *database.NPCRecord {
		return &database.NPCRecord{ID: id, NPCName: name, XCoord: 1010, YCoord: 10, ZCoord: 1000, ModelID: int(creationBody), Size: 1, NPCLevel: 11}
	}
	if _, err := f.w.UpsertNPC(row(2001, "Jarek's Creation"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := f.w.UpsertNPC(row(2003, "fungus cockroach"), 0); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(2 * NPCRefreshHold)
	if n := f.w.NPCs[2001]; !n.SpawnOnly || !n.hiddenAt(later) {
		t.Errorf("after its row's edit and the hold: spawn-only %v", n.SpawnOnly)
	}
	if n := f.w.NPCs[2003]; n.SpawnOnly || n.hiddenAt(later) {
		t.Errorf("the control after its edit and the hold: spawn-only %v, hidden %v", n.SpawnOnly, n.hiddenAt(later))
	}
}
