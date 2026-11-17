package entity

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// rampageRig is the template rig's Gnoll Brute (level 34) as a warrior whose template rampages by
// abilities, a pool of hit points, and the tank it fights beside it.
func rampageRig(t *testing.T, abilities string) (*templateRig, *NPC, *Player, *session.Session) {
	t.Helper()
	r := newTemplateRig(t)
	r.godTemplate("rampager", ", class", ", 0", abilities, "gnoll brute")
	boss := r.npc("Gnoll Brute")
	pool(boss, 1_000_000)
	tank, sess := r.rampagePlayer("Tank", boss.X+1.5, boss.Z)
	r.fight(boss, tank.ServerID)
	return r, boss, tank, sess
}

// rampagePlayer is the rig's player with a character id of its own (safe mode is kept by it).
func (r *templateRig) rampagePlayer(name string, x, z float32) (*Player, *session.Session) {
	p, sess := r.player(name, x, z)
	p.CharServerID = uint32(p.ServerID)
	return p, sess
}

// rampageNow runs one rampage of boss's at target, under the lock, and reports how many it hit.
func (r *templateRig) rampageNow(boss *NPC, target EntityID) int {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	return r.w.areaRampageLocked(boss, target, r.at(time.Second))
}

// rampageLines is the RAMPAGE lines among a session's messages.
func rampageLines(sess *session.Session) int {
	n := 0
	for _, m := range drain(sess) {
		if opOfMsg(m) == protocol.OpcodeColoredChat && strings.Contains(chatText(m), "goes on a RAMPAGE!") {
			n++
		}
	}
	return n
}

// The owner, 2026-11-17: "lets introduce rampage the area of effect melee damage ability with 10m".
// A rampage hits the tank it fights and every player, pet and mercenary within 10 of it, one melee
// swing on the whole crowd and a line to each player hit; a player and a pet at 10.5 are spared, and
// a creature beside it (its own side) is never hit.
func TestAreaRampageHitsEveryoneWithinTen(t *testing.T) {
	r, boss, tank, tankSess := rampageRig(t, `[{"kind":"rampage","chance":100}]`)
	in := []*Player{tank}
	for i, at := range [][2]float32{{5, 0}, {0, -7}, {-9.9, 0}, {6, 7.9}} {
		p, _ := r.rampagePlayer(string(rune('A'+i)), boss.X+at[0], boss.Z+at[1])
		in = append(in, p)
	}
	pet := r.petOf(in[1], 0x00300001, "Pet", false, boss.X-8, boss.Z)
	merc := r.petOf(in[2], 0x00300002, "Merc", true, boss.X, boss.Z+9.5)
	out, outSess := r.rampagePlayer("Outside", boss.X+10.5, boss.Z)
	outPet := r.petOf(out, 0x00300003, "Far Pet", false, boss.X, boss.Z-10.5)
	ally := r.npc("Gnoll Guard")
	r.place(ally, boss.X+2, boss.Z+2)
	pool(ally, 50_000)
	drain(tankSess)

	if n := r.rampageNow(boss, tank.ServerID); n != len(in)+2 {
		t.Errorf("it hit %d, want the %d players and the pet and the mercenary", n, len(in))
	}
	for _, p := range in {
		if p.HP == p.MaxHP {
			t.Errorf("%s (%.1f off) took nothing", p.Name, dist3(p.X, p.Y, p.Z, boss.X, boss.Y, boss.Z))
		}
	}
	for _, n := range []*NPC{pet, merc} {
		if n.HP == n.MaxHP {
			t.Errorf("%s took nothing", n.Name)
		}
	}
	if out.HP != out.MaxHP || outPet.HP != outPet.MaxHP {
		t.Errorf("at 10.5: the player %d of %d, the pet %d of %d: want both untouched", out.HP, out.MaxHP, outPet.HP, outPet.MaxHP)
	}
	if ally.HP != ally.MaxHP {
		t.Errorf("its own side was hit: %d of %d", ally.HP, ally.MaxHP)
	}
	var swings []b1
	lines := 0
	for _, m := range drain(tankSess) {
		if opOfMsg(m) == protocol.OpcodeCastingCombat {
			swings = append(swings, castsIn([][]byte{m})...)
		}
		if opOfMsg(m) == protocol.OpcodeColoredChat && chatText(m) == "Gnoll Brute goes on a RAMPAGE!" {
			lines++
		}
	}
	if len(swings) != 1 || swings[0].target != 0x14000000+uint32(tank.ServerID) {
		t.Errorf("the tank saw %d swings (%+v): want one, at it", len(swings), swings)
	}
	if lines != 1 {
		t.Errorf("the tank had %d RAMPAGE lines, want 1", lines)
	}
	// the player outside sees the players (the rig's views hold no pets): the one swing names the
	// four others it hit, and no line is his
	msgs := drain(outSess)
	if seen := castsIn(msgs); len(seen) != 1 || len(seen[0].others) != 4 {
		t.Errorf("outside, the swings seen: %+v, want one naming the four other players hit", seen)
	}
	for _, m := range msgs {
		if opOfMsg(m) == protocol.OpcodeColoredChat && strings.Contains(chatText(m), "RAMPAGE") {
			t.Errorf("the player outside had a RAMPAGE line")
		}
	}
}

// Each one it reaches takes a hit rolled on its own: five players alike take five figures as the dice
// fall for each, and damage_pct 200 doubles the hit of the same roll.
func TestAreaRampageRollsEachTargetAndScales(t *testing.T) {
	defer func(f func(int) int) { meleeIntn = f }(meleeIntn)
	k := 0
	meleeIntn = func(n int) int { k += 17; return k % n }
	r, boss, tank, _ := rampageRig(t, `[{"kind":"rampage","chance":100}]`)
	ps := []*Player{tank}
	for i := 0; i < 4; i++ {
		p, _ := r.rampagePlayer(string(rune('A'+i)), boss.X+float32(2+i), boss.Z+1)
		ps = append(ps, p)
	}
	r.rampageNow(boss, tank.ServerID)
	figures := map[int]bool{}
	for _, p := range ps {
		figures[p.MaxHP-p.HP] = true
	}
	if len(figures) < 3 {
		t.Errorf("five players took %d different figures: %v, want each its own roll", len(figures), figures)
	}

	meleeIntn = func(n int) int { return n / 2 }
	one := func(abilities string) int {
		r, boss, tank, _ := rampageRig(t, abilities)
		r.rampageNow(boss, tank.ServerID)
		return tank.MaxHP - tank.HP
	}
	base, double := one(`[{"kind":"rampage","chance":100}]`), one(`[{"kind":"rampage","chance":100,"damage_pct":200}]`)
	if base < 1 || double != 2*base {
		t.Errorf("a hit of %d at damage_pct 100, %d at 200: want double", base, double)
	}
}

// Its line of sight is the area spells': a player behind a wall within its 10 is spared, one in the
// open beside it is hit, and the tank it fights is hit whatever stands about.
func TestAreaRampageNeedsItsLineOfSight(t *testing.T) {
	r, boss, tank, _ := rampageRig(t, `[{"kind":"rampage","chance":100}]`)
	tank.X = boss.X - 1.5
	hidden, _ := r.rampagePlayer("Hidden", boss.X+7, boss.Z)
	open, _ := r.rampagePlayer("Open", boss.X, boss.Z+7)
	r.w.SetGround(sightGround{{boss.X + 3, 0, boss.Z - 3, boss.X + 4, 40, boss.Z + 3}})
	r.rampageNow(boss, tank.ServerID)
	if hidden.HP != hidden.MaxHP {
		t.Errorf("the player behind the wall took %d", hidden.MaxHP-hidden.HP)
	}
	if open.HP == open.MaxHP || tank.HP == tank.MaxHP {
		t.Errorf("in the open: %d, the tank: %d, want both hit", open.MaxHP-open.HP, tank.MaxHP-tank.HP)
	}
}

// A player in safe mode (/testinvis) who is no part of its fight is spared; one in safe mode on its
// hate list is hit as anyone.
func TestAreaRampageSparesSafeModeBystanders(t *testing.T) {
	r, boss, tank, _ := rampageRig(t, `[{"kind":"rampage","chance":100}]`)
	safe, _ := r.rampagePlayer("Safe", boss.X+4, boss.Z)
	fighting, _ := r.rampagePlayer("Fighting", boss.X-4, boss.Z)
	r.w.aggro.safe = map[uint32]bool{safe.CharServerID: true, fighting.CharServerID: true}
	boss.Threats[fighting.ServerID] = 1
	r.rampageNow(boss, tank.ServerID)
	if safe.HP != safe.MaxHP {
		t.Errorf("the safe bystander took %d", safe.MaxHP-safe.HP)
	}
	if fighting.HP == fighting.MaxHP {
		t.Errorf("the safe player in its fight took nothing")
	}
}

// On a clock (every 12, first 3) it rampages at 3, 15 and 27 s of a 30 s fight, whatever its
// rounds; with a chance of its rounds it goes at every round the dice allow and at none they refuse.
func TestAreaRampageClockAndChance(t *testing.T) {
	count := func(abilities string, roll func(int) int) int {
		defer func(f func(int) int) { areaRampageIntn = f }(areaRampageIntn)
		areaRampageIntn = roll
		r, boss, _, _ := rampageRig(t, abilities)
		by, sess := r.rampagePlayer("Bystander", boss.X+6, boss.Z)
		r.run(0, 30*time.Second)
		if by.HP == by.MaxHP && abilities == `[{"kind":"rampage","every":12,"first":3}]` {
			t.Errorf("%s: the bystander took nothing", abilities)
		}
		return rampageLines(sess)
	}
	if n := count(`[{"kind":"rampage","every":12,"first":3}]`, func(int) int { return 99 }); n != 3 {
		t.Errorf("on a clock of 12 from 3 over 30 s: %d rampages, want 3", n)
	}
	if n := count(`[{"kind":"rampage","every":12,"first":3,"chance":50}]`, func(int) int { return 99 }); n != 0 {
		t.Errorf("on a clock at 50%% with the dice against it: %d rampages, want 0", n)
	}
	all := count(`[{"kind":"rampage","chance":20}]`, func(int) int { return 0 })
	if all < 6 {
		t.Errorf("at every round over 30 s: %d rampages, want one a round (6 or more)", all)
	}
	if n := count(`[{"kind":"rampage","chance":20}]`, func(int) int { return 99 }); n != 0 {
		t.Errorf("with the dice against every round: %d rampages", n)
	}
	k := 0
	half := count(`[{"kind":"rampage","chance":20}]`, func(int) int { k++; return (k % 2) * 99 })
	if half < all/2-1 || half > all/2+1 {
		t.Errorf("one round in two: %d rampages of %d rounds", half, all)
	}
}

// The template's fields are held to their bounds: a rampage needs a chance or a clock, a radius to
// 50, and takes no cast's fields.
func TestAreaRampageFields(t *testing.T) {
	for _, c := range []struct {
		abilities string
		ok        bool
	}{
		{`[{"kind":"rampage","chance":20}]`, true},
		{`[{"kind":"rampage","every":12,"first":3,"radius":10,"damage_pct":150,"name":"Flurry","say":"Feel my wrath!"}]`, true},
		{`[{"kind":"rampage"}]`, false},
		{`[{"kind":"rampage","chance":20,"radius":60}]`, false},
		{`[{"kind":"rampage","chance":20,"first":3}]`, false},
		{`[{"kind":"rampage","every":1}]`, false},
		{`[{"kind":"rampage","chance":20,"spell":"Fire Storm"}]`, false},
		{`[{"kind":"rampage","chance":20,"damage_pct":2000}]`, false},
	} {
		_, err := database.ParseTemplateAbilities(c.abilities)
		if (err == nil) != c.ok {
			t.Errorf("%s: %v, want ok %v", c.abilities, err, c.ok)
		}
	}
}
