package entity

import (
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"os"
	"testing"
	"time"
)

// bossAoERig is the template rig's Gnoll Brute (2000, 100) as a raid boss
// whose one ability is Fire Storm round itself, radius 30 by a spellarea row
// with no cap of its own (centre 2, maxtargets 0), and the player it fights
// beside it.
func bossAoERig(t *testing.T) (*templateRig, *NPC, *Player) {
	t.Helper()
	r := newTemplateRig(t)
	if _, err := r.db.Exec(`INSERT INTO spellarea (spellid, centre, radius, maxtargets, note) VALUES (700145, 2, 30, 0, 'test')`); err != nil {
		t.Fatal(err)
	}
	r.godTemplate("raid boss", ", class", ", 13", `[{"kind":"aoe","spell_id":700145,"every":10,"first":0}]`, "gnoll brute")
	boss := r.npc("Gnoll Brute")
	pool(boss, 1_000_000)
	tank, _ := r.player("Tank", boss.X+1.5, boss.Z)
	return r, boss, tank
}

// castAndLand begins the boss's AoE at target and lands it, its others.
func (r *templateRig) castAndLand(boss *NPC, target EntityID) []EntityID {
	r.t.Helper()
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.fight(boss, target)
	if !r.w.templateCastLocked(boss, 0, target, r.at(0)) {
		r.t.Fatal("the boss began no AoE")
	}
	others := r.w.npcCasts[len(r.w.npcCasts)-1].others
	r.w.templateSystem(r.at(10 * time.Second))
	return others
}

// ring puts n players round (x, z) at distance d, named by what.
func (r *templateRig) ring(what string, n int, x, z, d float32) []*Player {
	var out []*Player
	for i := 0; i < n; i++ {
		s, c := math.Sincos(2 * math.Pi * float64(i) / float64(n))
		px, pz := x+d*float32(c), z+d*float32(s)
		if n > 10 {
			out = append(out, r.crowdPlayer(fmt.Sprintf("%s%d", what, i), px, pz))
			continue
		}
		p, _ := r.player(fmt.Sprintf("%s%d", what, i), px, pz)
		out = append(out, p)
	}
	return out
}

// crowdPlayer is the rig's player (a session in the world) without its view of
// the others, which holds 23: one of a crowd.
func (r *templateRig) crowdPlayer(name string, x, z float32) *Player {
	id := EntityID(1001 + len(r.w.Players))
	sess := r.mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, byte(id>>8), byte(id)), Port: 7000}, uint32(id), 1, 1, 2, true)
	p := &Player{ServerID: id, Name: name, Level: 20, HP: 100000, MaxHP: 100000, X: x, Y: 10, Z: z}
	r.w.Players[id] = p
	r.w.SpatialIndex.Insert(id, x, z)
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	sess.SetPlayerObjectID(0x14000000 + uint32(id))
	return p
}

// group makes ps one group (a raid past GroupWindowSize), the first its leader.
func (r *templateRig) group(ps []*Player) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	g := r.w.formGroupLocked(ps[0])
	for _, p := range ps[1:] {
		g.Members = append(g.Members, p.ServerID)
		p.GroupID = g.ID
	}
}

// petOf puts a pet (or a mercenary) of owner's at (x, z).
func (r *templateRig) petOf(owner *Player, id EntityID, name string, merc bool, x, z float32) *NPC {
	n := &NPC{ServerID: id, Name: name, Level: 20, HP: 100000, MaxHP: 100000, X: x, Y: 10, Z: z, Owner: owner.ServerID}
	if merc {
		n.merc = &mercState{owner: owner, npc: n, body: &Player{ServerID: id, Name: name, Level: 20, HP: 100000, MaxHP: 100000}}
	}
	r.w.NPCs[id] = n
	r.w.SpatialIndex.Insert(id, x, z)
	return n
}

// The owner, 2026-11-17: "the raid bosses aoes should be able to hit all
// targets within range". A raid boss's AoE round itself takes everyone within
// its radius: the player it fights and his group, a raid group of six beside
// them, a player in no group, two players' pets and a mercenary, 14 bodies,
// where AreaMaxTargets had stopped it at the target and the seven nearest. A
// player a step past the radius is spared, and a spellarea row's cap still
// bounds no creature's area.
func TestABossAoEHitsEveryoneInRange(t *testing.T) {
	r, boss, tank := bossAoERig(t)
	groupA := append([]*Player{tank}, r.ring("A", 3, boss.X, boss.Z, 8)...)
	raidB := r.ring("B", 6, boss.X, boss.Z, 20)
	r.group(groupA)
	r.group(raidB)
	solo, _ := r.player("Solo", boss.X, boss.Z+27)
	pet := r.petOf(groupA[1], 0x00300001, "Pet A", false, boss.X-12, boss.Z)
	petB := r.petOf(raidB[2], 0x00300002, "Pet B", false, boss.X, boss.Z-25)
	merc := r.petOf(solo, 0x00300003, "Merc", true, boss.X+29, boss.Z)
	out, _ := r.player("Outside", boss.X-31, boss.Z)

	others := r.castAndLand(boss, tank.ServerID)
	if len(others) != 13 {
		t.Errorf("its others: %d, want the 13 in its radius beside its target", len(others))
	}
	for _, p := range append(append(groupA, raidB...), solo) {
		if p.HP == p.MaxHP {
			t.Errorf("%s (%.1f from the boss) took nothing", p.Name, dist3(p.X, p.Y, p.Z, boss.X, boss.Y, boss.Z))
		}
	}
	for _, n := range []*NPC{pet, petB, merc} {
		if n.HP == n.MaxHP {
			t.Errorf("%s took nothing", n.Name)
		}
	}
	if out.HP != out.MaxHP {
		t.Errorf("the player 31 off, past its 30: HP %d of %d", out.HP, out.MaxHP)
	}

	// a GM's row's cap bounds a player's cast of the spell, not a creature's
	r, boss, tank = bossAoERig(t)
	if _, err := r.db.Exec(`UPDATE spellarea SET maxtargets = 3 WHERE spellid = 700145`); err != nil {
		t.Fatal(err)
	}
	r.assign("gnoll brute", "raid boss")
	ps := r.ring("C", 10, boss.X, boss.Z, 15)
	r.castAndLand(boss, tank.ServerID)
	for _, p := range ps {
		if p.HP == p.MaxHP {
			t.Errorf("under a row's cap of 3: %s took nothing", p.Name)
		}
	}
}

// Its line of sight is looked at from the boss's own heights too: a dragon
// whose feet stand in a hollow under the floor round it (its feet at 7, the
// floor a slab from 8 to 9.5 the players stand on) breathes on all of them,
// where a man's head and waist over its feet were under the slab and spared
// everyone. The controls: a creature of a man's size there still reaches no
// one, and a wall as high as the dragon's eyes still hides the one behind it.
func TestABigBossAoESeesOverItsOwnFloor(t *testing.T) {
	const dragon uint32 = 0x0D1A9017
	SetModelRadii(map[uint32]float32{dragon: 16.186})
	t.Cleanup(func() { SetModelRadii(nil) })
	for _, c := range []struct {
		what string
		big  bool
		wall bool
		hit  int
	}{
		{"a dragon in its hollow", true, false, 5},
		{"a man's size in the hollow", false, false, 0},
		{"a dragon, one behind a wall to its eyes", true, true, 4},
	} {
		r, boss, tank := bossAoERig(t)
		boss.Y = 7
		if c.big {
			boss.ModelID, boss.Size = int64(dragon), 2
		}
		g := sightGround{{boss.X - 4, 8, boss.Z - 100, boss.X + 100, 9.5, boss.Z + 100}} // the floor round its hollow
		if c.wall {
			g = append(g, sightBox{boss.X + 4, 9.5, boss.Z + 9, boss.X + 30, 20, boss.Z + 11})
		}
		r.w.SetGround(g)
		tank.X, tank.Y = boss.X+5, 9.5
		ps := []*Player{tank}
		for i, at := range [][2]float32{{10, 0}, {15, 10}, {20, -10}, {25, -5}} {
			p, _ := r.player(fmt.Sprintf("P%d", i), boss.X+at[0], boss.Z+at[1])
			p.Y = 9.5
			ps = append(ps, p)
		}
		r.castAndLand(boss, tank.ServerID)
		hit := 0
		for _, p := range ps {
			if p.HP < p.MaxHP {
				hit++
			}
		}
		if hit != c.hit {
			t.Errorf("%s: %d hit, want %d", c.what, hit, c.hit)
		}
	}
}

// Fifty players and their fifty pets round a boss: every one of the hundred
// takes its AoE, in well under a tick's time for the whole cast and landing.
func TestABossAoEOnAHundredBodies(t *testing.T) {
	r, boss, tank := bossAoERig(t)
	ps := r.ring("R", 49, boss.X, boss.Z, 12)
	var pets []*NPC
	for i, p := range append(ps, tank) {
		pets = append(pets, r.petOf(p, EntityID(0x00310000+i), fmt.Sprintf("Pet%d", i), i%2 == 0, p.X+1, p.Z+1))
	}
	r.w.SetGround(sightGround{{boss.X + 50, 0, boss.Z, boss.X + 51, 50, boss.Z + 1}}) // a box nowhere near: every line looked at
	log.SetOutput(io.Discard)                                                         // the lines' writing is the log file's, not the tick's figuring
	defer log.SetOutput(os.Stderr)
	start := time.Now()
	others := r.castAndLand(boss, tank.ServerID)
	took := time.Since(start)
	log.SetOutput(os.Stderr)
	if len(others) != 99 {
		t.Errorf("its others: %d, want 99", len(others))
	}
	for _, p := range ps {
		if p.HP == p.MaxHP {
			t.Errorf("%s took nothing", p.Name)
		}
	}
	for _, n := range pets {
		if n.HP == n.MaxHP {
			t.Errorf("%s took nothing", n.Name)
		}
	}
	t.Logf("cast and landing on 100 bodies: %v", took)
	// 100 ms, not 20: on a quiet machine it is a few ms, but the full suite
	// shares the dev box with other builds and once read 34.8 ms under that load
	// (2026-11-18, build346); 100 still catches a real slowdown (a tick is 50).
	if took > 100*time.Millisecond {
		t.Errorf("cast and landing on 100 bodies took %v", took)
	}
}
