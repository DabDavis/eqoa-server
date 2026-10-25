package entity

import (
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
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
		// the class's swing, as a player's (Sandstorm's classes: a Crag Wisp
		// is a magician, Guard Findel a shadow knight)
		{"a Lv27 magician: Attack1 always", NPC{Level: 27, Class: 10}, 3200, 3000, 0xDA9BEA11, []int{500}},
		{"a Lv60 enchanter: Attack1 always", NPC{Level: 60, Class: 12}, 3200, 3000, 0xDA9BEA11, []int{500}},
		{"a Lv70 shadow knight: Attack3", NPC{Level: 70, Class: 3}, 4000, 4000, 0xDA9BEA13, []int{500, 1000}},
		{"a Lv25 paladin: Attack3 from Lv20", NPC{Level: 25, Class: 2}, 4000, 4000, 0xDA9BEA13, []int{500, 1000}},
		{"a Lv40 rogue: Attack4", NPC{Level: 40, Class: 6}, 6000, 6000, 0xDA9BEA14, []int{500, 1000, 1700, 2400}},
		{"a class out of range is a warrior's", NPC{Level: 40, Class: 224}, 6000, 6000, 0xDA9BEA14, []int{500, 1000, 1700, 2400}},
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

	// Out of reach: it runs at its enemy (chase.go), landing nothing while
	// it closes in. Until 2026-10-25 the enemy was dropped here.
	c.p.X += 20
	tick(4500 * time.Millisecond)
	if got := c.sent(); len(got) != 0 || c.npc.CurrentEnemy != c.p.ServerID || !c.npc.moving() || c.npc.Loco != LocoRun {
		t.Errorf("a player 20 away: sent %x, enemy %d, moving %v, loco %d", got, c.npc.CurrentEnemy, c.npc.moving(), c.npc.Loco)
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

// An NPC's rounds keep their cadence whatever the tick: a round begins one
// round after the last began, not at the tick that found it due.
func TestNPCRoundsKeepTheirCadence(t *testing.T) {
	c := newCaster(t)
	c.npc.Name, c.npc.Level, c.npc.HP, c.npc.MaxHP = "Desert Crab", 34, 2000, 2000
	c.npc.CurrentEnemy = c.p.ServerID
	c.p.HP, c.p.MaxHP = 5000, 5000
	swings := 0
	for _, d := range []time.Duration{0, 4050 * time.Millisecond, 8 * time.Second} {
		c.w.mu.Lock()
		c.w.aiSystem(c.at(d))
		c.w.mu.Unlock()
		for _, m := range c.sent() {
			if opOfMsg(m) == protocol.OpcodeCastingCombat {
				swings++
			}
		}
	}
	if swings != 3 {
		t.Errorf("%d swings at 0, 4.05 and 8 s, want 3: the round late by a tick put off the next", swings)
	}
}

// An NPC loaded from the database takes its class from npcclass by its name
// (a trailing space or '$' and case not counting), and one the table does
// not name is a warrior.
func TestLoadNPCsTakesTheirClass(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/npcs.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`INSERT INTO npcs (npc_name, x_coord, y_coord, z_coord, world, npc_level) VALUES ('Crag Wisp ', 23186, 56, 19623, 0, 27), ('Gharial ', 23516, 48, 19556, 0, 33)`,
		`INSERT INTO npcclass (name, class, source) VALUES ('crag wisp', 10, 'sandstorm')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	w := NewWorld()
	w.LoadNPCs(database.NewNPCStore(db))
	classes := map[string]int{}
	for _, n := range w.NPCs {
		classes[n.Name] = n.Class
	}
	if classes["Crag Wisp "] != 10 || classes["Gharial "] != 0 || len(classes) != 2 {
		t.Errorf("classes %v, want the wisp a magician and the gharial a warrior", classes)
	}
}
