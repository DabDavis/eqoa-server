package entity

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// dungeonsRig is a world with the dungeons on in world 14: two dungeons ("guk" round (1000, 1000), "bb" round
// (5000, 1000)), their templates (rows of world 14), the warden in Freeport (world 0) and its twin at Guk's entrance,
// and n players by the Freeport warden, each holding world 14.
type dungeonsRig struct {
	w   *World
	ps  []*Player
	ss  []*session.Session
	now time.Time
}

func dungeonsTestDefs() []DungeonDef {
	return []DungeonDef{
		{Key: "guk", Name: "Test Guk", Boxes: []DungeonBox{{X0: 900, Z0: 900, X1: 1100, Z1: 1100}},
			Entrance: HomeSpot{X: 1000, Y: 50, Z: 1150, Facing: 3.14}},
		{Key: "bb", Name: "Test Burrow", Boxes: []DungeonBox{{X0: 4900, Z0: 900, X1: 5100, Z1: 1100}},
			Entrance: HomeSpot{X: 5000, Y: 50, Z: 1150}},
	}
}

func newDungeons(t *testing.T, n int) *dungeonsRig {
	t.Helper()
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	r := &dungeonsRig{w: w, now: time.Unix(1_800_000_000, 0)}
	w.mu.Lock()
	for _, b := range []*NPC{
		{ServerID: 910001, Name: DungeonWardenName, X: 5000, Y: 50, Z: 5000, World: 0, NPCType: 0x80, Level: 60, HP: 1, MaxHP: 1},
		{ServerID: 910002, Name: DungeonWardenName, X: 1004, Y: 50, Z: 1150, World: 14, NPCType: 0x80, Level: 60, HP: 1, MaxHP: 1},
		// Guk's creatures (templates), one far in the other dungeon
		{ServerID: 920001, Name: "froglok vis knight", X: 1000, Y: 50, Z: 1095, World: 14, Level: 61, HP: 5000, MaxHP: 5000, EventAggro: true},
		{ServerID: 920002, Name: "froglok tal wizard", X: 1010, Y: 50, Z: 1000, World: 14, Level: 61, HP: 5000, MaxHP: 5000},
		{ServerID: 920003, Name: "Lord Hoptor", X: 990, Y: 50, Z: 950, World: 14, Level: 64, HP: 50000, MaxHP: 50000},
		{ServerID: 920004, Name: "gnoll scout", X: 5000, Y: 50, Z: 1000, World: 14, Level: 60, HP: 5000, MaxHP: 5000},
	} {
		b.SpawnX, b.SpawnY, b.SpawnZ = b.X, b.Y, b.Z
		w.putNPCLocked(b)
		w.SpatialIndex.Insert(b.ServerID, b.X, b.Z)
	}
	w.mu.Unlock()
	w.SetDungeons(true, 14, dungeonsTestDefs(), DungeonSettings{CloseAfter: 10 * time.Minute})
	t.Cleanup(func() { SetInstancedWorld(14, false); dungeonWorlds[14].Store(false) })
	for i := 0; i < n; i++ {
		p := &Player{ServerID: EntityID(620000001 + i), Name: fmt.Sprintf("Raider%d", i), Level: 60, HP: 9000, MaxHP: 9000,
			X: 5000 + float32(i), Y: 50, Z: 5004, World: 0, CharServerID: uint32(620000001 + i)}
		s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 3, 0, byte(i+1)), Port: 7300 + i}, uint32(300+i), uint32(300+i), 1, 2, true)
		w.Players[p.ServerID] = p
		w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
		s.SetECSEntity(p)
		s.SetCharacterInWorld(true)
		s.SetPlayerObjectID(uint32(0x17000000 + i))
		s.SetServerWorlds([]uint8{14})
		r.ps = append(r.ps, p)
		r.ss = append(r.ss, s)
	}
	return r
}

func (r *dungeonsRig) land() []GateTravel {
	r.w.mu.Lock()
	due := r.w.gates.due
	r.w.gates.due = nil
	for _, t := range due {
		r.w.SpatialIndex.Remove(t.P.ServerID, t.P.X, t.P.Z)
		t.P.World, t.P.X, t.P.Y, t.P.Z = t.To.World, t.To.X, t.To.Y, t.To.Z
		r.w.SpatialIndex.Insert(t.P.ServerID, t.P.X, t.P.Z)
	}
	r.w.mu.Unlock()
	return due
}

func (r *dungeonsRig) tick(d time.Duration) {
	r.now = r.now.Add(d)
	r.w.mu.Lock()
	r.w.dungeonsSystem(r.now)
	r.w.mu.Unlock()
}

// group puts the players in one group, the first its leader.
func (r *dungeonsRig) group(ps ...*Player) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	g := r.w.formGroupLocked(ps[0])
	for _, p := range ps[1:] {
		g.Members = append(g.Members, p.ServerID)
		p.GroupID = g.ID
	}
}

// copies is the dungeon creatures standing in an instance, by name.
func (r *dungeonsRig) copies(inst uint32) map[string]*NPC {
	r.w.mu.RLock()
	defer r.w.mu.RUnlock()
	out := map[string]*NPC{}
	for _, n := range r.w.NPCs {
		if n.Temporary && n.World == 14 && n.Instance == inst {
			out[n.Name] = n
		}
	}
	return out
}

func TestDungeonTemplatesStandOutOfTheWorld(t *testing.T) {
	r := newDungeons(t, 0)
	r.w.mu.RLock()
	defer r.w.mu.RUnlock()
	for id, want := range map[EntityID]bool{920001: true, 920003: true, 920004: true, 910002: false, 910001: false} {
		if got := r.w.NPCs[id].SpawnOnly; got != want {
			t.Errorf("npc %d (%s) held out of the world: %v, want %v", id, r.w.NPCs[id].Name, got, want)
		}
	}
	// a spawn-only names' read again keeps them out
	r.w.mu.RUnlock()
	r.w.SetSpawnOnly(map[string]bool{})
	r.w.mu.RLock()
	if !r.w.NPCs[920001].SpawnOnly {
		t.Error("npcspawnonly read again let a dungeon's template into the world")
	}
}

func TestDungeonInstancesPerParty(t *testing.T) {
	r := newDungeons(t, 5)
	a, b, c, d, e := r.ps[0], r.ps[1], r.ps[2], r.ps[3], r.ps[4]
	r.group(a, b, c) // raid A: a leads
	r.group(d, e)    // group B: d leads
	// a member cannot open
	if why, ok := r.w.DungeonEnter(r.ss[1], "guk", r.now); ok || !strings.Contains(why, "leader must open") {
		t.Fatalf("a member opened: %v %q", ok, why)
	}
	if _, ok := r.w.DungeonEnter(r.ss[0], "Guk", r.now); !ok {
		t.Fatal("the leader could not open Guk")
	}
	for _, i := range []int{1, 2} {
		if why, ok := r.w.DungeonEnter(r.ss[i], "guk", r.now); !ok {
			t.Fatalf("member %d: %s", i, why)
		}
	}
	if why, ok := r.w.DungeonEnter(r.ss[3], "guk", r.now); !ok {
		t.Fatalf("the second group's leader: %s", why)
	}
	if _, ok := r.w.DungeonEnter(r.ss[4], "guk", r.now); !ok {
		t.Fatal("the second group's member")
	}
	r.land()
	ia, id := a.InstanceKey(), d.InstanceKey()
	if ia == 0 || id == 0 || ia == id || b.InstanceKey() != ia || c.InstanceKey() != ia || e.InstanceKey() != id {
		t.Fatalf("instances: a %d b %d c %d d %d e %d", ia, b.InstanceKey(), c.InstanceKey(), id, e.InstanceKey())
	}
	if a.World != 14 || a.Z != 1150 || d.Z != 1150 {
		t.Fatalf("not at Guk's entrance: %v %v", a, d)
	}
	ca, cd := r.copies(ia), r.copies(id)
	if len(ca) != 3 || len(cd) != 3 || ca["gnoll scout"] != nil {
		t.Fatalf("Guk's creatures in each: %d %d (%v)", len(ca), len(cd), ca)
	}
	if ca["Lord Hoptor"].Row() != 920003 || ca["Lord Hoptor"].Level != 64 || ca["Lord Hoptor"].SpawnOnly {
		t.Fatalf("a copy of its row, in the world: %+v", ca["Lord Hoptor"])
	}
	if SamePlace(a, d) || !SamePlace(a, b) {
		t.Fatal("the two parties share a place, or a raid does not")
	}
	r.w.mu.Lock()
	sees := [4]bool{r.w.npcSharesLocked(ca["Lord Hoptor"], a), r.w.npcSharesLocked(ca["Lord Hoptor"], d),
		r.w.npcSharesLocked(r.w.NPCs[910002], a), r.w.npcSharesLocked(r.w.NPCs[910002], d)}
	// aggro: the knight by the entrance (EventAggro) attacks on sight, its own party alone
	d.Z = 1105 // 10 from the knights
	r.w.drawNearLocked(d, r.now, true)
	r.w.mu.Unlock()
	if sees != [4]bool{true, false, true, true} {
		t.Fatalf("a's Hoptor seen by a, d; the warden by a, d: %v", sees)
	}
	if ca["froglok vis knight"].Threats[d.ServerID] > 0 {
		t.Fatal("raid A's knight attacked group B's leader")
	}
	if cd["froglok vis knight"].Threats[d.ServerID] <= 0 {
		t.Fatalf("group B's knight did not attack its own: %v", cd["froglok vis knight"].Threats)
	}
	// the choices as each sees them
	for _, ch := range r.w.DungeonChoices(r.ss[1]) {
		if ch.Key == "guk" && (!ch.Open || ch.MayOpen || ch.Owner != a.Name) {
			t.Fatalf("b's Guk: %+v", ch)
		}
	}
}

func TestDungeonSendsOutAndCloses(t *testing.T) {
	r := newDungeons(t, 3)
	a, b, c := r.ps[0], r.ps[1], r.ps[2]
	r.group(a, b)
	for _, i := range []int{0, 1} {
		if why, ok := r.w.DungeonEnter(r.ss[i], "guk", r.now); !ok {
			t.Fatal(why)
		}
	}
	r.land()
	inst := a.InstanceKey()
	r.tick(10 * time.Second)
	if len(r.w.gates.due) != 0 {
		t.Fatal("a member of the party sent out")
	}
	// b leaves the group: out after the grace, back where it came from
	r.w.mu.Lock()
	r.w.groups[a.GroupID].Members = r.w.groups[a.GroupID].Members[:1]
	b.GroupID = 0
	r.w.mu.Unlock()
	r.tick(time.Second)
	r.tick(2 * time.Second)
	if len(r.w.gates.due) != 0 {
		t.Fatal("sent out within the grace")
	}
	r.tick(6 * time.Second)
	if due := r.land(); len(due) != 1 || due[0].P != b || b.World != 0 || b.X != 5001 {
		t.Fatalf("b not sent back where it came from: %v %v", due, b)
	}
	// c stands in world 14 with no instance (a GM's move): out too
	r.w.mu.Lock()
	c.World, c.X, c.Z = 14, 1000, 1150
	r.w.mu.Unlock()
	r.tick(2 * time.Second)
	r.tick(6 * time.Second)
	if r.land(); c.World != 0 {
		t.Fatalf("c stayed in no instance: world %d", c.World)
	}
	// a logs out inside: its instance stays open, and a login resumes in it
	r.w.mu.Lock()
	delete(r.w.Players, a.ServerID)
	r.w.mu.Unlock()
	if dg, resume, in, _ := r.w.DungeonLogin(a.CharServerID, 14, 1000, 1150); !dg || !resume || in != inst {
		t.Fatalf("a's login: %v %v %d", dg, resume, in)
	}
	// no one in it for CloseAfter: closed, its creatures gone, and a login goes out
	r.tick(9 * time.Minute)
	if len(r.copies(inst)) != 3 {
		t.Fatal("closed early")
	}
	r.tick(2 * time.Minute)
	if len(r.copies(inst)) != 0 {
		t.Fatalf("closed instance's creatures: %d", len(r.copies(inst)))
	}
	if dg, resume, _, out := r.w.DungeonLogin(a.CharServerID, 14, 1000, 1150); !dg || resume || out.World != 0 {
		t.Fatalf("a's login after the close: %v %v %v", dg, resume, out)
	}
}

func TestDungeonResetAndKilledStayDead(t *testing.T) {
	r := newDungeons(t, 1)
	a := r.ps[0]
	if _, ok := r.w.DungeonEnter(r.ss[0], "guk", r.now); !ok {
		t.Fatal("alone may open")
	}
	r.land()
	inst := a.InstanceKey()
	if line := r.w.DungeonReset(r.ss[0], "guk"); !strings.Contains(line, "no one inside") {
		t.Fatalf("reset with its owner inside: %q", line)
	}
	if _, ok := r.w.DungeonLeave(r.ss[0]); !ok {
		t.Fatal("leave")
	}
	r.land()
	if line := r.w.DungeonReset(r.ss[0], "guk"); !strings.Contains(line, "is closed") {
		t.Fatalf("reset: %q", line)
	}
	if len(r.copies(inst)) != 0 {
		t.Fatal("reset left creatures")
	}
	// a fresh instance: a copy is Temporary, so a killed one never comes back in it
	if _, ok := r.w.DungeonEnter(r.ss[0], "guk", r.now); !ok {
		t.Fatal("reopen")
	}
	if a.Instance == inst || len(r.copies(a.Instance)) != 3 {
		t.Fatal("the same instance again, or an empty one")
	}
	for _, n := range r.copies(a.Instance) {
		if !n.Temporary || n.CopyOf == 0 {
			t.Fatalf("%s is no copy of a row", n.Name)
		}
	}
}

// Petition 1832 (Waldorf, 2026-11-12): "Running the Guk 60 Instance and shamans aren't tied to their pets". An
// instance's copies of a shaman and the Companion Spirit 1.5 off it fight as one, within their own instance alone;
// the templates and a GM's copy (Temporary, no instance) stay out of every pair.
func TestDungeonShamansKeepTheirPets(t *testing.T) {
	r := newDungeons(t, 2)
	r.w.mu.Lock()
	for _, b := range []*NPC{
		{ServerID: 920011, Name: "froglok tal shaman", X: 1020, Y: 50, Z: 1020, World: 14, Level: 60, HP: 5000, MaxHP: 5000},
		{ServerID: 920012, Name: "Companion Spirit", X: 1021.5, Y: 50, Z: 1020, World: 14, Level: 58, HP: 3000, MaxHP: 3000},
	} {
		b.SpawnX, b.SpawnY, b.SpawnZ = b.X, b.Y, b.Z
		r.w.putNPCLocked(b)
		r.w.SpatialIndex.Insert(b.ServerID, b.X, b.Z)
	}
	r.w.mu.Unlock()
	r.w.SetDungeons(true, 14, dungeonsTestDefs(), DungeonSettings{CloseAfter: 10 * time.Minute})
	a, d := r.ps[0], r.ps[1]
	for i := range r.ps {
		if why, ok := r.w.DungeonEnter(r.ss[i], "guk", r.now); !ok {
			t.Fatalf("player %d: %s", i, why)
		}
	}
	r.land()
	ca, cd := r.copies(a.InstanceKey()), r.copies(d.InstanceKey())
	sa, pa, sd, pd := ca["froglok tal shaman"], ca["Companion Spirit"], cd["froglok tal shaman"], cd["Companion Spirit"]
	if sa == nil || pa == nil || sd == nil || pd == nil || a.InstanceKey() == d.InstanceKey() {
		t.Fatalf("copies: %v %v", ca, cd)
	}
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	ids := func(ns []*NPC) []EntityID {
		var out []EntityID
		for _, n := range ns {
			out = append(out, n.ServerID)
		}
		return out
	}
	if got := ids(r.w.companionPartnersLocked(sa)); len(got) != 1 || got[0] != pa.ServerID {
		t.Fatalf("a's shaman's partners %v, want its own instance's spirit %d", got, pa.ServerID)
	}
	if got := ids(r.w.companionPartnersLocked(pd)); len(got) != 1 || got[0] != sd.ServerID {
		t.Fatalf("d's spirit's partners %v, want its own instance's shaman %d", got, sd.ServerID)
	}
	// control: the templates pair with no one
	if got := r.w.companionPartnersLocked(r.w.NPCs[920011]); len(got) != 0 {
		t.Fatalf("a template paired: %v", ids(got))
	}
	// a hit on a's shaman brings a's spirit, not d's
	r.w.companionJoinLocked(sa, a.ServerID)
	if pa.Threats[a.ServerID] <= 0 {
		t.Fatalf("a's spirit did not join its shaman: %v", pa.Threats)
	}
	if pd.Threats[a.ServerID] > 0 {
		t.Fatal("d's spirit joined a's fight")
	}
}

// Petition 1841 (Waldorf, 2026-11-12): "a tracker to see how many of the total we killed ... Like xx of yy killed".
// /dungeon's line counts the party's kills against the instance's creatures.
func TestDungeonStatusCountsKills(t *testing.T) {
	r := newDungeons(t, 1)
	if why, ok := r.w.DungeonEnter(r.ss[0], "guk", r.now); !ok {
		t.Fatal(why)
	}
	r.land()
	c := r.copies(r.ps[0].InstanceKey())
	r.w.mu.Lock()
	c["Lord Hoptor"].Dead = true
	r.w.mu.Unlock()
	if got := r.w.DungeonStatus(r.ss[0]); !strings.Contains(got, "Test Guk (Raider0's): 1 of 3 creatures killed, 1 inside") {
		t.Fatalf("status %q", got)
	}
}
