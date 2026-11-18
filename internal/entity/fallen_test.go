package entity

import (
	"sort"
	"strings"
	"testing"
	"time"
)

// The owner, 2026-11-18: "if the raid boss dies and players are dead they dont get to roll on
// loot" (fallen.go). A raid of eight under need before greed fights a boss; at the kill:
//   - Bob died in the fight and was released to his bind in another world: a seat and a
//     KillCredit player (his rolls, his share, his quest credit and drops);
//   - Cal died in the fight and lies there: the same (a KillCredit player now too);
//   - Gus is alive beside the boss: as ever;
//   - Dee is alive far off and never fought: no seat (the control: living reach as today);
//   - Eve died in the fight, released, then left the raid: no seat, no credit;
//   - Fay died far from the boss (another fight's death): no mark, no seat;
//   - Hal died in the fight, released, and logged out: no seat.
//
// Then the control on the fight's end: Bob dies to a second creature, which leaves its fight
// alive (no one on its hate list) and the marks are swept; the raid pulls it again and kills it
// with Bob at his bind: no seat for him.
func TestARaidMemberDeadInTheFightSharesTheKill(t *testing.T) {
	r := newRaidRig(t, 8, 50)
	r.form(8)
	g := r.raidOf(0)
	if !raidingLocked(g) {
		t.Fatal("not a raid")
	}
	r.w.RaidLootRule(r.sess[0], "needgreed")
	if g.rules.loot != LootNeedGreed {
		t.Fatal("no need before greed")
	}
	var heard []KillCredit
	r.w.OnKill(func(k KillCredit) { heard = append(heard, k) })
	now := time.Now()
	die := func(i int) {
		r.w.mu.Lock()
		r.w.playerDiedLocked(r.sess[i], r.p[i], now)
		r.w.mu.Unlock()
	}
	release := func(i int) { // the respawn at a bind in another world
		r.w.mu.Lock()
		p := r.p[i]
		p.DiedAt, p.HP = time.Time{}, p.MaxHP
		p.World, p.X, p.Z = 1, 24000, 9000
		r.w.mu.Unlock()
	}
	kill := func(npc *NPC) KillCredit {
		r.w.mu.Lock()
		npc.Threats, npc.CurrentEnemy = nil, 0
		r.w.killNPC(npc)
		r.w.creditKillLocked(r.sess[0], r.p[0], npc)
		r.w.mu.Unlock()
		return heard[len(heard)-1]
	}

	boss := r.fight(9700, 0)
	r.w.mu.Lock()
	r.p[3].X = 1000 + KillShareRange + 50 // Dee: alive, far off, never fought
	r.p[5].X = 1000 + KillShareRange + 50 // Fay: far off when she dies
	r.w.mu.Unlock()
	for _, i := range []int{1, 2, 4, 5, 7} {
		die(i)
	}
	release(1)
	release(4)
	release(7)
	r.w.GroupLeave(r.sess[4]) // Eve leaves the raid
	if r.p[4].GroupID != 0 {
		t.Fatal("Eve is still in the raid")
	}
	r.w.RemovePlayer(r.p[7].ServerID) // Hal logs out
	if !r.p[2].IsDead() || r.p[1].IsDead() {
		t.Fatal("Cal should lie dead, Bob be up at his bind")
	}

	k := kill(boss)
	if k.Raid == nil {
		t.Fatal("no raid kill")
	}
	want := "Ann,Bob,Cal,Gus"
	if got := seatNames(k.Raid.Seats); got != want {
		t.Errorf("the boss's seats: %s; want %s", got, want)
	}
	var players []string
	for _, p := range k.Players {
		players = append(players, p.Name)
	}
	sort.Strings(players)
	if got := strings.Join(players, ","); got != want {
		t.Errorf("the kill's players: %s; want %s", got, want)
	}

	// the control: the fight's end forgets the marks
	second := r.fight(9701, 0)
	r.w.mu.Lock()
	r.p[1].World, r.p[1].X, r.p[1].Z = 0, 1000, 1000 // Bob back at the fight
	r.w.mu.Unlock()
	die(1)
	release(1)
	r.w.mu.Lock()
	second.Threats, second.CurrentEnemy = nil, 0 // it leaves its fight alive
	r.w.sweepRaidClaimsLocked()
	r.w.addThreatLocked(second, r.p[0].ServerID, 10) // pulled again
	r.w.mu.Unlock()
	k = kill(second)
	if got := seatNames(k.Raid.Seats); strings.Contains(got, "Bob") {
		t.Errorf("a death in a fight that ended still seats Bob: %s", got)
	}
}

// A plain group's member who died in the fight and was released is one of its kill's
// players (quest credit, its corpse, its No Trade share), its free for all as before.
func TestAGroupMemberDeadInTheFightSharesTheKill(t *testing.T) {
	r := newRaidRig(t, 3, 30)
	r.form(3)
	var heard []KillCredit
	r.w.OnKill(func(k KillCredit) { heard = append(heard, k) })
	npc := r.fight(9800, 0)
	r.w.mu.Lock()
	r.w.playerDiedLocked(r.sess[1], r.p[1], time.Now())
	r.p[1].DiedAt, r.p[1].HP, r.p[1].World = time.Time{}, r.p[1].MaxHP, 1
	r.p[2].X = 1000 + KillShareRange + 50 // the control: alive, far off
	npc.Threats, npc.CurrentEnemy = nil, 0
	r.w.killNPC(npc)
	r.w.creditKillLocked(r.sess[0], r.p[0], npc)
	r.w.mu.Unlock()
	var names []string
	for _, p := range heard[len(heard)-1].Players {
		names = append(names, p.Name)
	}
	if got := strings.Join(names, ","); got != "Ann,Bob" {
		t.Errorf("the group's kill's players: %s; want Ann,Bob", got)
	}
}

func seatNames(seats []RaidSeat) string {
	var out []string
	for _, s := range seats {
		out = append(out, s.Name)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}
