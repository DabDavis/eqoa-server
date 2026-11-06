package entity

import (
	"testing"
	"time"
)

// Petition 1125: "These 2 crabs were not dying to combat damage. I had to /kill both
// with a GM command". What lives shows at least 1 of 255 on the channel: CLIENT's
// TargetInRange takes a 0 for no life, swings at nothing and tells the server out of
// reach for good. Only what is dead shows 0; a whole one shows 255.
func TestWhatLivesShowsSomeHealth(t *testing.T) {
	for _, c := range []struct {
		hp, top int
		want    byte
	}{{0, 1613, 0}, {-5, 1613, 0}, {1, 1613, 1}, {6, 1613, 1}, {7, 1613, 1}, {13, 1613, 2}, {1613, 1613, 255}, {1, 30000, 1},
		{5, 0, 0}, {2000, 1613, 255}} {
		if got := healthBar(c.hp, c.top); got != c.want {
			t.Errorf("healthBar(%d, %d) = %d, want %d", c.hp, c.top, got, c.want)
		}
	}
	n := &NPC{ServerID: 9, Name: "crab", HP: 1, MaxHP: 1613, Level: 12}
	if b := SerializeNPCUpdate(1, n); b[27] != 1 {
		t.Errorf("a crab on 1 of 1613 shows %d on the channel, want 1", b[27])
	}
	n.HP, n.Dead = 0, true
	if b := SerializeNPCUpdate(1, n); b[27] != 0 {
		t.Errorf("a dead crab shows %d on the channel, want 0", b[27])
	}
}

// The round it stops: a creature on 1 of 1613 still takes a hit and dies of it, as
// the client now swings at it.
func TestACreatureOnOneDiesOfAHit(t *testing.T) {
	f := newFight(t)
	f.npc.MaxHP, f.npc.HP = 1613, 1
	f.p.AutoAttack = true
	if !f.w.EngageAttack(f.sess, f.npcID, Swing{Kind: 0, Delay: 3 * time.Second, Reach: 4.5}, f.t0) {
		t.Fatal("not engaged")
	}
	f.tick(300 * time.Millisecond)
	f.tick(time.Second)
	if !f.npc.Dead || f.p.AttackTarget != 0 {
		t.Errorf("after a hit: dead %v, fighting %d", f.npc.Dead, f.p.AttackTarget)
	}
}
