package entity

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// homesRig is a world with the homes on in world 13, a board in Freeport (world 0) and its twin in the yard, and
// n players standing by the Freeport board, each holding the homes' world.
type homesRig struct {
	w   *World
	ps  []*Player
	ss  []*session.Session
	now time.Time
}

func homesTestLayout() HomesLayout {
	return HomesLayout{Name: "the Test Homesteads", Door: HomeSpot{X: 1000, Y: 50, Z: 1000, Facing: 1},
		Board: HomeSpot{X: 1010, Y: 50, Z: 1000}, YardX: 1000, YardZ: 1000, YardRadius: 60}
}

func newHomes(t *testing.T, n int) *homesRig {
	t.Helper()
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	w.SetHomes(true, 13, homesTestLayout(), nil, nil, nil)
	t.Cleanup(func() { SetInstancedWorld(13, false) })
	r := &homesRig{w: w, now: time.Unix(1_800_000_000, 0)}
	w.mu.Lock()
	for i, b := range []*NPC{
		{ServerID: 900001, Name: HomesBoardName, X: 5000, Y: 50, Z: 5000, World: 0, NPCType: 0x80, Level: 1, HP: 1, MaxHP: 1},
		{ServerID: 900002, Name: HomesBoardName, X: 1010, Y: 50, Z: 1000, World: 13, NPCType: 0x80, Level: 1, HP: 1, MaxHP: 1},
	} {
		_ = i
		w.putNPCLocked(b)
		w.SpatialIndex.Insert(b.ServerID, b.X, b.Z)
	}
	w.mu.Unlock()
	for i := 0; i < n; i++ {
		p := &Player{ServerID: EntityID(610000001 + i), Name: fmt.Sprintf("Homer%d", i), Level: 30, HP: 1000, MaxHP: 1000,
			X: 5000 + float32(i), Y: 50, Z: 5004, World: 0, CharServerID: uint32(800 + i)}
		s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 2, 0, byte(i+1)), Port: 7200 + i}, uint32(200+i), uint32(200+i), 1, 2, true)
		w.Players[p.ServerID] = p
		w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
		s.SetECSEntity(p)
		s.SetCharacterInWorld(true)
		s.SetPlayerObjectID(uint32(0x16000000 + i))
		s.SetServerWorlds([]uint8{13})
		r.ps = append(r.ps, p)
		r.ss = append(r.ss, s)
	}
	return r
}

// land does the travels the World queued, as the handlers' travel hook would (the place, the world).
func (r *homesRig) land() []GateTravel {
	r.w.mu.Lock()
	due := r.w.gates.due
	r.w.gates.due = nil
	for _, t := range due {
		t.P.World, t.P.X, t.P.Y, t.P.Z = t.To.World, t.To.X, t.To.Y, t.To.Z
	}
	r.w.mu.Unlock()
	return due
}

// tick runs the homes' check at r.now + d (past the quiet after a move).
func (r *homesRig) tick(d time.Duration) {
	r.now = r.now.Add(d)
	r.w.mu.Lock()
	r.w.homesSystem(r.now)
	r.w.mu.Unlock()
}

func TestHomesEachInItsOwn(t *testing.T) {
	r := newHomes(t, 2)
	a, b := r.ps[0], r.ps[1]
	for i := range r.ps {
		if _, why, ok := r.w.HomeEnter(r.ss[i], r.ps[i].CharServerID, r.now); !ok {
			t.Fatalf("%s home: %s", r.ps[i].Name, why)
		}
	}
	r.land()
	if a.World != 13 || b.World != 13 || a.X != b.X || a.Z != b.Z {
		t.Fatalf("both at the door of world 13: %v %v", a, b)
	}
	if a.InstanceKey() != a.CharServerID || b.InstanceKey() != b.CharServerID {
		t.Fatalf("instances %d %d", a.InstanceKey(), b.InstanceKey())
	}
	if SamePlace(a, b) {
		t.Fatal("two homes at the same spot are one place")
	}
	r.w.mu.Lock()
	board := r.w.NPCs[900002]
	shared := r.w.npcSharesLocked(board, a) && r.w.npcSharesLocked(board, b)
	pet := &NPC{ServerID: 900100, Name: "pet", World: 13, X: 1000, Z: 1000, Owner: a.ServerID}
	r.w.putNPCLocked(pet)
	petSeen := [2]bool{r.w.npcSharesLocked(pet, a), r.w.npcSharesLocked(pet, b)}
	r.w.mu.Unlock()
	if !shared {
		t.Fatal("the yard's board stands in every home")
	}
	if !petSeen[0] || petSeen[1] {
		t.Fatalf("a's pet is in a's home alone: %v", petSeen)
	}
	// outside the homes the instance is nothing
	if _, ok := r.w.HomeLeave(r.ss[0]); !ok {
		t.Fatal("a cannot leave")
	}
	r.land()
	if a.World != 0 || a.InstanceKey() != 0 {
		t.Fatalf("a out: world %d instance %d", a.World, a.InstanceKey())
	}
	if a.X != 5000 || a.Z != 5004 {
		t.Fatalf("a back where it came from: %.0f %.0f", a.X, a.Z)
	}
}

func TestHomesInviteVisitUninvite(t *testing.T) {
	r := newHomes(t, 2)
	a, b := r.ps[0], r.ps[1]
	r.w.HomeEnter(r.ss[0], a.CharServerID, r.now)
	r.land()
	if _, why, ok := r.w.HomeEnter(r.ss[1], a.CharServerID, r.now); ok {
		t.Fatal("b went into a's home uninvited")
	} else if why == "" {
		t.Fatal("no word why")
	}
	if !r.w.HomeInvite(a.CharServerID, a.Name, HomeGuest{ID: b.CharServerID, Name: b.Name}) {
		t.Fatal("invite refused")
	}
	if r.w.HomeInvite(a.CharServerID, a.Name, HomeGuest{ID: b.CharServerID, Name: b.Name}) {
		t.Fatal("a second invite counted")
	}
	if open := r.w.HomesOpenTo(r.ss[1]); len(open) != 1 || open[0].Host != a.CharServerID || open[0].Name != a.Name {
		t.Fatalf("b's open homes %v", open)
	}
	if _, why, ok := r.w.HomeEnter(r.ss[1], a.CharServerID, r.now); !ok {
		t.Fatalf("b invited: %s", why)
	}
	r.land()
	if !SamePlace(a, b) {
		t.Fatal("a and b in a's home are not one place")
	}
	r.tick(5 * time.Second)
	if b.World != 13 {
		t.Fatal("an invited guest was sent out")
	}
	was, sent := r.w.HomeUninvite(a.CharServerID, b.CharServerID)
	if !was || !sent {
		t.Fatalf("uninvite: was %v sent %v", was, sent)
	}
	r.land()
	if b.World != 0 || b.X != 5001 {
		t.Fatalf("b out to where it came from: world %d (%.0f, %.0f)", b.World, b.X, b.Z)
	}
}

func TestHomesGroupLetsInWhileGrouped(t *testing.T) {
	r := newHomes(t, 2)
	a, b := r.ps[0], r.ps[1]
	r.w.mu.Lock()
	g := &Group{ID: 77, Members: []EntityID{a.ServerID, b.ServerID}}
	if r.w.groups == nil {
		r.w.groups = map[uint32]*Group{}
	}
	r.w.groups[77] = g
	a.GroupID, b.GroupID = 77, 77
	r.w.mu.Unlock()
	if open := r.w.HomesOpenTo(r.ss[1]); len(open) != 1 || open[0].Host != a.CharServerID {
		t.Fatalf("a group member's home is open: %v", open)
	}
	r.w.HomeEnter(r.ss[0], a.CharServerID, r.now)
	if _, why, ok := r.w.HomeEnter(r.ss[1], a.CharServerID, r.now); !ok {
		t.Fatalf("grouped: %s", why)
	}
	r.land()
	r.tick(5 * time.Second)
	if b.World != 13 {
		t.Fatal("a grouped guest sent out")
	}
	r.w.mu.Lock()
	b.GroupID = 0
	g.Members = g.Members[:1]
	r.w.mu.Unlock()
	r.tick(5 * time.Second)
	r.land()
	if b.World != 0 {
		t.Fatal("a guest out of the group stays")
	}
}

func TestHomesKickAndYard(t *testing.T) {
	r := newHomes(t, 2)
	a, b := r.ps[0], r.ps[1]
	r.w.HomeInvite(a.CharServerID, a.Name, HomeGuest{ID: b.CharServerID, Name: b.Name})
	r.w.HomeEnter(r.ss[0], a.CharServerID, r.now)
	r.w.HomeEnter(r.ss[1], a.CharServerID, r.now)
	r.land()
	if n, _ := r.w.HomeKick(r.ss[1], a.Name); n != "" {
		t.Fatal("a guest kicked the owner")
	}
	if n, why := r.w.HomeKick(r.ss[0], b.Name); n != b.Name {
		t.Fatalf("kick: %q %s", n, why)
	}
	r.land()
	if b.World != 0 {
		t.Fatal("kicked, b stays")
	}
	// past the yard: back to the door
	a.X = 1000 + 100
	r.tick(5 * time.Second)
	if due := r.land(); len(due) != 1 || a.X != 1000 {
		t.Fatalf("past the yard: %d travels, x %.0f", len(due), a.X)
	}
	// a player in the world with no instance (a GM's move) is put in its own
	r.w.mu.Lock()
	b.World, b.Instance, b.X, b.Z = 13, 0, 1000, 1000
	r.w.mu.Unlock()
	r.tick(5 * time.Second)
	if b.Instance != b.CharServerID || b.World != 13 {
		t.Fatalf("no instance: %d world %d", b.Instance, b.World)
	}
}

func TestHomesLogin(t *testing.T) {
	r := newHomes(t, 2)
	a, b := r.ps[0], r.ps[1]
	r.w.HomeInvite(a.CharServerID, a.Name, HomeGuest{ID: b.CharServerID, Name: b.Name})
	r.w.HomeEnter(r.ss[1], a.CharServerID, r.now)
	r.land()
	home, resume, host, _ := r.w.HomeLogin(b.CharServerID, 13)
	if !home || !resume || host != a.CharServerID {
		t.Fatalf("invited guest's login: %v %v %d", home, resume, host)
	}
	r.w.mu.Lock()
	inst := r.w.homeInstanceOnLoadLocked(b.CharServerID, 13)
	r.w.mu.Unlock()
	if inst != a.CharServerID {
		t.Fatalf("created in instance %d", inst)
	}
	r.w.HomeUninvite(a.CharServerID, b.CharServerID)
	home, resume, _, out := r.w.HomeLogin(b.CharServerID, 13)
	if !home || resume || out.World != 0 || out.X != 5001 {
		t.Fatalf("uninvited guest's login: %v %v %+v", home, resume, out)
	}
	if home, _, _, _ := r.w.HomeLogin(b.CharServerID, 0); home {
		t.Fatal("a login outside the homes is the homes'")
	}
	// the owner's own: always back in
	if home, resume, host, _ := r.w.HomeLogin(a.CharServerID, 13); !home || !resume || host != a.CharServerID {
		t.Fatalf("owner's login: %v %v %d", home, resume, host)
	}
}
