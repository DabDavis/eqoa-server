package entity

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Petition 2198 (Heirloom, Isle of Dread: "Dracoliche is still
// attackable with pets thru the walls in IoD"): no fight through a quest wall
// its owner does not pass (questwallfight.go).

// putWall stands a quest wall (an invisible blocker of size 1, a box 5 a side)
// at (x, y, z) in the world, of quest 101312's step 7, and returns it.
func putWall(w *World, id EntityID, world uint8, x, y, z float32) *NPC {
	n := &NPC{ServerID: id, Name: "invisible wall", ModelID: int64(BlockerBody), Size: 1, NPCType: 0x80, X: x, Y: y, Z: z, World: world}
	w.mu.Lock()
	w.NPCs[n.ServerID] = n
	w.mu.Unlock()
	w.SpatialIndex.Insert(n.ServerID, n.X, n.Z)
	w.SetQuestWalls([]database.QuestWall{{NPCID: int(id), QuestID: 101312, Step: 7}})
	return n
}

// The geometry: a line through the box is walled, one past it, one on
// another floor and one the owner passes are not.
func TestWallBetween(t *testing.T) {
	w := NewWorld()
	p := &Player{ServerID: 1, CharServerID: 77, World: 0}
	putWall(w, 900, 0, 100, 10, 100)
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, c := range []struct {
		name                   string
		ax, ay, az, bx, by, bz float32
		want                   bool
	}{
		{"through", 90, 10, 100, 110, 10, 100, true},
		{"slantwise through a corner", 90, 10, 96, 110, 10, 104, true},
		{"past it", 90, 10, 110, 110, 10, 110, false},
		{"short of it", 90, 10, 100, 96, 10, 100, false},
		{"a floor under it", 90, -20, 100, 110, -20, 100, false},
		{"a floor over it", 90, 40, 100, 110, 40, 100, false},
	} {
		if _, got := w.wallBetweenLocked(p, 0, c.ax, c.ay, c.az, c.bx, c.by, c.bz); got != c.want {
			t.Errorf("%s: walled %v, want %v", c.name, got, c.want)
		}
	}
	if _, got := w.wallBetweenLocked(p, 1, 90, 10, 100, 110, 10, 100); got {
		t.Error("a wall of world 0 stood between two places of world 1")
	}
	w.qwalls.passes = map[uint32]map[EntityID]bool{77: {900: true}}
	if _, got := w.wallBetweenLocked(p, 0, 90, 10, 100, 110, 10, 100); got {
		t.Error("a wall its owner passes stood between")
	}
}

// A summoned pet told to attack a creature behind a wall is refused; once its
// owner passes the wall it goes and fights; a fight is let go once the wall
// stands between again (its owner no longer passing it); a creature behind
// it fighting its owner is no target of a defensive pet's.
func TestThePetFightsNothingBehindAWall(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	// the owner at x 8629.6, the crab at 8640: a wall at 8635 between
	putWall(r.w, 3900, 2, 8635, 75, 5421.5)
	r.w.PetCommand(r.sess, petCmdAttack, 0x0133E8A0, r.at(9600*time.Millisecond))
	msgs := r.sent()
	if len(msgs) != 1 || !strings.Contains(chatText(msgs[0]), petBehindWall) {
		t.Fatalf("the attack's answer through a wall: %q", chatText(msgs[0]))
	}
	if r.w.pets[r.p.ServerID].target != 0 {
		t.Fatal("the pet took a target behind the wall")
	}
	// the crab on its owner, behind the wall: a defensive pet stays
	r.w.mu.Lock()
	r.w.pets[r.p.ServerID].stance = petDefensive
	r.enemy.CurrentEnemy = r.p.ServerID
	r.w.mu.Unlock()
	r.tick(9700 * time.Millisecond)
	if r.w.pets[r.p.ServerID].target != 0 {
		t.Fatal("a defensive pet took up a fight behind the wall")
	}
	// its owner passes the wall: the attack goes
	r.w.SetQuestState(r.p.CharServerID, map[EntityID]bool{3900: true}, nil)
	r.w.PetCommand(r.sess, petCmdAttack, 0x0133E8A0, r.at(9800*time.Millisecond))
	if msgs := r.sent(); len(msgs) != 1 || !strings.Contains(chatText(msgs[0]), "attacks Desert Crab") {
		t.Fatalf("the attack's answer past a wall its owner passes: %x", msgs)
	}
	r.tick(9900 * time.Millisecond)
	if r.w.pets[r.p.ServerID].target != r.enemy.ServerID {
		t.Fatal("the pet let its fight go with its owner passing the wall")
	}
	// passed no more (a GM's step back): the fight is let go
	r.w.ForgetQuestState(r.p.CharServerID)
	r.tick(10000 * time.Millisecond)
	if r.w.pets[r.p.ServerID].target != 0 || pet.RoundTarget == r.enemy.ServerID && !pet.RoundStart.IsZero() {
		t.Fatal("the pet fought on through the wall")
	}
}

// A merc: /merc attack at a creature behind a wall is refused, and it takes
// up no fight of its owner's behind one (his auto-attack's target).
func TestTheMercFightsNothingBehindAWall(t *testing.T) {
	r := newMercRig(t)
	ms := r.hire(r.kit(0, 0, 0), MercAssist)
	crab := r.creature("Desert Crab", 20, 10)
	putWall(r.w, 4900, r.p.World, r.p.X+5, r.p.Y, r.p.Z)
	r.charmTarget(crab) // the owner's target
	if _, why := r.w.MercAttack(r.sess, 0); why != petBehindWall {
		t.Fatalf("/merc attack through a wall: %q", why)
	}
	r.w.mu.Lock()
	r.p.AttackTarget, r.p.AutoAttack = crab.ServerID, true
	r.w.mu.Unlock()
	r.run(0, 2*time.Second)
	if ms.target != 0 {
		t.Fatal("the merc took up its owner's fight behind the wall")
	}
	r.w.SetQuestState(r.p.CharServerID, map[EntityID]bool{4900: true}, nil)
	if name, why := r.w.MercAttack(r.sess, 0); why != "" || name != "Desert Crab" {
		t.Fatalf("/merc attack past a wall its owner passes: %q %q", name, why)
	}
}

// A player's harmful spell at a creature behind a wall he does not pass is
// refused as out of sight; past it, cast.
func TestNoHarmfulCastThroughAWall(t *testing.T) {
	c := newCaster(t)
	putWall(c.w, 2900, 0, 101, 10, 100) // between the caster at x 100 and the creature at 102
	if c.w.StartCast(c.sess, strike(), c.npcO, c.at(0)) {
		t.Fatal("a strike went through the wall")
	}
	c.w.SetQuestState(c.p.CharServerID, map[EntityID]bool{2900: true}, nil)
	if !c.w.StartCast(c.sess, strike(), c.npcO, c.at(time.Second)) {
		t.Fatal("a strike past a wall its caster passes was refused")
	}
}
