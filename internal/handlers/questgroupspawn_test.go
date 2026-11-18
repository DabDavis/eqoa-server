package handlers

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The Siliskor quest (the owner, 2026-11-18, forumotion t177): Siliskor rises
// in his pit by the crooked elephant statue when the player comes to it, once
// for a group (a group spawn event), from the original that stands nearest the
// place (the Graveyard's, not the Brog Fens' row of a lower id), and a second
// group raises its own (the forum's quest had a second group unable to).

// siliskorFixture is a quest of a reach step at the player and a kill step of
// Siliskor whose start spawns him at a place 20 off, one set a group; two
// spawn-only Siliskors, the lower row far off and small, the nearer one 2.5.
func siliskorFixture(t *testing.T) (*questFixture, *entity.NPC) {
	t.Helper()
	f := newQuestFixture(t)
	f.tw.p.HP, f.tw.p.MaxHP = 100, 100
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (130, 'Siliskor', 1, 'Zaryn Boome')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, world, x, y, z, radius, journal) VALUES (130, 1, 'reach', 0, 25403, 54, 15725, 10, 'The statue.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, count, journal, events) VALUES (130, 2, 'kill', 'Siliskor', 1, 'Kill Siliskor.',
		'[{"on":"start","do":"spawn","npc":"Siliskor","hostile":true,"group":true,"world":0,"x":25415,"y":54,"z":15741}]')`)
	f.qe.Reload()
	far := &entity.NPC{ServerID: 101337, Name: "Siliskor", Level: 55, Size: 1, HP: 100, MaxHP: 100,
		SpawnX: 25100, SpawnY: 54, SpawnZ: 15400, X: 25100, Y: 54, Z: 15400}
	near := &entity.NPC{ServerID: 111881, Name: "Siliskor", Level: 57, Size: 2.5, HP: 100, MaxHP: 100,
		SpawnX: 25418, SpawnY: 54, SpawnZ: 15738, X: 25418, Y: 54, Z: 15738}
	for _, n := range []*entity.NPC{far, near} {
		f.tw.w.NPCs[n.ServerID] = n
		f.tw.w.SpatialIndex.Insert(n.ServerID, n.X, n.Z)
	}
	f.tw.w.SetSpawnOnly(map[string]bool{database.NPCClassKey("Siliskor"): true})
	if _, err := f.qe.SetCharacterQuest(f.sid, 130, QuestSetStep, 1); err != nil {
		t.Fatal(err)
	}
	return f, near
}

// The step reached raises Siliskor at the place, as the nearer original, at the
// player; the controls: the far one of a lower row is not what is copied, and
// the originals stay out of the world.
func TestAPlacedSpawnRisesAtItsPlaceFromTheNearestOriginal(t *testing.T) {
	f, near := siliskorFixture(t)
	f.qe.checkReach()
	if s, _ := f.progress(130); s != 2 {
		t.Fatalf("at the statue, the quest is at step %d, want 2", s)
	}
	cs := f.questCopies()
	if len(cs) != 1 {
		t.Fatalf("%d Siliskors raised, want one", len(cs))
	}
	c := cs[0]
	if c.Size != near.Size || c.Level != near.Level {
		t.Errorf("the copy is of size %.1f level %d, want the Graveyard's %.1f, %d", c.Size, c.Level, near.Size, near.Level)
	}
	if dx, dz := c.X-25415, c.Z-15741; dx*dx+dz*dz > 0.01 {
		t.Errorf("the copy stands at %.1f, %.1f, not its place", c.X, c.Z)
	}
	if c.NPCType&0x80 != 0 || c.CurrentEnemy != f.tw.p.ServerID {
		t.Errorf("the copy: type %#x, enemy %d: want attackable, at the player", c.NPCType, c.CurrentEnemy)
	}
	if !near.SpawnOnly || near.CurrentEnemy != 0 {
		t.Errorf("the original: spawn-only %v, enemy %d", near.SpawnOnly, near.CurrentEnemy)
	}
}

// A group mate whose Siliskor of the step stands (alive, or its corpse lying)
// means the player raises none, at the step's start and on coming back near
// the original (respawnLost); one of another group, or a mate whose own is gone,
// does not stop it.
func TestAGroupSpawnIsOneSetForAGroup(t *testing.T) {
	f, _ := siliskorFixture(t)
	mate := &entity.Player{ServerID: 2009, CharServerID: f.sid + 1000, Name: "Mate"}
	mates := []*entity.Player{f.tw.p, mate}
	f.qe.mates = func(*entity.Player) []*entity.Player { return mates }
	theirs, _, ok := f.tw.w.SpawnNPCCopy("Siliskor", 25416, 54, 15742, 0, 0)
	if !ok {
		t.Fatal("no copy for the mate")
	}
	f.qe.mu.Lock()
	f.qe.spawns = map[uint32][]questSpawn{mate.CharServerID: {{id: theirs, quest: 130, step: 2, until: time.Now().Add(time.Minute), npc: "Siliskor"}}}
	f.qe.mu.Unlock()

	f.qe.checkReach()
	if s, _ := f.progress(130); s != 2 {
		t.Fatalf("at the statue, the quest is at step %d, want 2", s)
	}
	if n := liveSiliskors(f); n != 1 {
		t.Fatalf("%d Siliskors with the mate's standing, want the mate's one alone", n)
	}
	f.tw.p.X, f.tw.p.Z = 25418, 15738 // beside the spawn-only original: a lost spawn would come again
	f.qe.respawnLost()
	if n := liveSiliskors(f); n != 1 {
		t.Fatalf("%d Siliskors after a look near the original, want the mate's one alone", n)
	}

	mates = []*entity.Player{f.tw.p} // the mate left the group: no group's spawn stands for the player
	f.qe.respawnLost()
	if n := liveSiliskors(f); n != 2 {
		t.Fatalf("%d Siliskors, the mate gone from the group: want the player's own beside the other group's", n)
	}
}

// liveSiliskors is how many copies of Siliskor are alive (a quest's, or the
// mate's made beside it).
func liveSiliskors(f *questFixture) int {
	n := 0
	for _, c := range f.tw.w.NPCs {
		if c.Temporary && !c.Dead && c.Name == "Siliskor" {
			n++
		}
	}
	return n
}

// The good casters' and the evil melees' epics raise the same Boldunder (the
// owner's Evil Melee epic, 2026-11-18): a group mate's group spawn of the NPC
// on ANOTHER quest stands for the player too, so a mixed raid fights one; a
// mate's spawn of another NPC, or one not made by a group event, does not.
func TestAGroupSpawnOfTheSameNPCOnAnotherQuestStands(t *testing.T) {
	for _, c := range []struct {
		name  string
		npc   string
		group bool
		want  int
	}{
		{"the same NPC, a group's", "Siliskor", true, 1},
		{"the same NPC, not a group's", "Siliskor", false, 2},
		{"another NPC", "Pomus Magemell", true, 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, _ := siliskorFixture(t)
			mate := &entity.Player{ServerID: 2009, CharServerID: f.sid + 1000, Name: "Mate"}
			f.qe.mates = func(*entity.Player) []*entity.Player { return []*entity.Player{f.tw.p, mate} }
			theirs, _, ok := f.tw.w.SpawnNPCCopy("Siliskor", 25416, 54, 15742, 0, 0)
			if !ok {
				t.Fatal("no copy for the mate")
			}
			f.qe.mu.Lock()
			f.qe.spawns = map[uint32][]questSpawn{mate.CharServerID: {{id: theirs, quest: 131, step: 4,
				until: time.Now().Add(time.Minute), npc: c.npc, group: c.group}}}
			f.qe.mu.Unlock()
			f.qe.checkReach()
			if s, _ := f.progress(130); s != 2 {
				t.Fatalf("at the statue, the quest is at step %d, want 2", s)
			}
			if n := liveSiliskors(f); n != c.want {
				t.Fatalf("%d Siliskors, want %d", n, c.want)
			}
		})
	}
}
