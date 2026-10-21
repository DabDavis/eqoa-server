package entity

import (
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The NPC rounds held to Sandstorm's: the swing by level and gear, its
// length and hit times, the swing to everyone who sees the NPC and the hits
// to the player hit, and the fight's end with the enemy.

func TestNPCRoundShapes(t *testing.T) {
	ms := func(d ...time.Duration) []int {
		out := make([]int, len(d))
		for i, x := range d {
			out[i] = int(x / time.Millisecond)
		}
		return out
	}
	tests := []struct {
		name          string
		npc           NPC
		length, delay int
		fx            uint32
		hits          []int
	}{
		{"a Lv10 creature: Attack1 of 3000 every 3.2 s", NPC{Level: 10}, 3200, 3000, 0xDA9BEA11, []int{500}},
		{"the Lv34 crab: Attack2 every 4 s", NPC{Level: 34}, 4000, 4000, 0xDA9BEA12, []int{500, 1500}},
		{"the Lv35 gharial: Attack4 every 6 s", NPC{Level: 35}, 6000, 6000, 0xDA9BEA14, []int{500, 1000, 1700, 2400}},
		{"a Lv49 guard with sword and shield: Attack3 every 4 s", NPC{Level: 49, PrimaryWeapon: 0x64B86194, Shield: 0x77CAADDE}, 4000, 4000, 0xDA9BEA13, []int{500, 1000}},
		{"a Lv60 with a sword alone: Attack4", NPC{Level: 60, PrimaryWeapon: 1}, 6000, 6000, 0xDA9BEA14, []int{500, 1000, 1700, 2400}},
	}
	for _, tt := range tests {
		length, delay, fx, hits := npcRound(&tt.npc)
		got := ms(hits...)
		if int(length/time.Millisecond) != tt.length || int(delay/time.Millisecond) != tt.delay || fx != tt.fx || len(got) != len(tt.hits) {
			t.Errorf("%s: %v %v %08X %v", tt.name, length, delay, fx, got)
			continue
		}
		for i := range got {
			if got[i] != tt.hits[i] {
				t.Errorf("%s: hits %v, want %v", tt.name, got, tt.hits)
				break
			}
		}
	}
}

// A creature's fight: the swing at the round's start to the player and to a
// bystander who sees the creature, the hits to the player alone at their
// times, a new round when the last is over, and none once the enemy is out
// of reach.
func TestNPCFightsInRounds(t *testing.T) {
	c := newCaster(t)
	c.npc.Name, c.npc.Level, c.npc.HP, c.npc.MaxHP = "Desert Crab", 34, 2000, 2000
	c.npc.OffMod, c.npc.WeaponDamage = NPCOffMod(34), NPCWeaponDamage(34)
	c.npc.CurrentEnemy = c.p.ServerID
	c.npc.Threats = map[EntityID]int64{c.p.ServerID: 10}
	c.p.HP, c.p.MaxHP = 500, 536

	other, otherSess := c.addViewer(t)
	_ = other
	tick := func(d time.Duration) {
		c.w.mu.Lock()
		c.w.aiSystem(c.at(d))
		c.w.mu.Unlock()
	}

	tick(0)
	start := c.sent()
	if len(start) != 1 || opOfMsg(start[0]) != protocol.OpcodeCastingCombat {
		t.Fatalf("round start sent %x, want the swing", start)
	}
	want := unhex(t, "b100982e3101d0a1241400a00f000012ea9bda") // crab -> player, 4000 ms, Attack2
	if string(start[0]) != string(want) {
		t.Errorf("swing % X, want % X", start[0], want)
	}
	if seen := drain(otherSess); len(seen) != 1 || opOfMsg(seen[0]) != protocol.OpcodeCastingCombat {
		t.Errorf("the bystander got %x, want the swing alone", seen)
	}

	tick(499 * time.Millisecond)
	if got := c.sent(); len(got) != 0 {
		t.Errorf("a hit before 0.5 s: %x", got)
	}
	tick(500 * time.Millisecond)
	hit := c.sent()
	if len(hit) != 1 || opOfMsg(hit[0]) != protocol.OpcodeDamageIndication || c.p.HP >= 500 {
		t.Fatalf("the first hit: %x, HP %d", hit, c.p.HP)
	}
	tick(1500 * time.Millisecond)
	if got := c.sent(); len(got) != 1 || opOfMsg(got[0]) != protocol.OpcodeDamageIndication {
		t.Errorf("the second hit: %x", got)
	}
	tick(3999 * time.Millisecond)
	if got := c.sent(); len(got) != 0 {
		t.Errorf("a third hit in a two-hit round: %x", got)
	}
	if seen := drain(otherSess); len(seen) != 0 {
		t.Errorf("the bystander got the hits: %x", seen)
	}
	tick(4 * time.Second)
	if got := c.sent(); len(got) != 1 || opOfMsg(got[0]) != protocol.OpcodeCastingCombat {
		t.Errorf("no second round at 4 s: %x", got)
	}

	// Out of reach: the leash drops the enemy, and the round with it.
	c.p.X += 20
	tick(4500 * time.Millisecond)
	if got := c.sent(); len(got) != 0 || c.npc.CurrentEnemy != 0 || !c.npc.RoundStart.IsZero() {
		t.Errorf("hit a player 20 away: %x, enemy %d", got, c.npc.CurrentEnemy)
	}
}

// addViewer is a second player in the caster fixture's world, beside it,
// who sees its creature (by another object id).
func (c *caster) addViewer(t *testing.T) (*Player, *session.Session) {
	t.Helper()
	sess := c.w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 7001}, 2, 2, 2, 3, true)
	p := &Player{ServerID: 1003, Name: "Watcher", HP: 100, MaxHP: 100, X: 104, Y: 10, Z: 100}
	c.w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	sess.SetPlayerObjectID(0x1424A1D1)
	vs := GetOrCreateVisibility(sess)
	vs.ChannelEntity[0] = c.npc.ServerID
	vs.ChannelObjectID[0] = 0x01312E99
	return p, sess
}

// drain is a session's queue, each message whole, opcode first.
func drain(s *session.Session) [][]byte {
	var out [][]byte
	for _, m := range s.Queue().GetPending() {
		out = append(out, append([]byte(nil), m.Data()...))
	}
	return out
}
