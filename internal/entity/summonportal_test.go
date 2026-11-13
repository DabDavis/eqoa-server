package entity

import (
	"math"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// The portal a Summoning Portal opens (summonportal.go), in a world of its
// own. The people and places are made up.

type portalFixture struct {
	t    *testing.T
	w    *World
	mgr  *session.Manager
	now  time.Time
	sent []GateTravel
	port int
}

func newPortalFixture(t *testing.T) *portalFixture {
	t.Helper()
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	f := &portalFixture{t: t, w: w, mgr: mgr, now: time.Unix(1_800_000_000, 0), port: 7200}
	w.OnTravel(func(tr GateTravel) { f.sent = append(f.sent, tr) })
	return f
}

func (f *portalFixture) player(id EntityID, name string, x, z float32, facing float32) (*Player, *session.Session) {
	f.t.Helper()
	f.port++
	s := f.mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 2, 1), Port: f.port}, uint32(f.port), 1, 1, 2, true)
	p := &Player{ServerID: id, Name: name, Level: 40, HP: 100, MaxHP: 100, X: x, Y: 50, Z: z, Facing: facing}
	f.w.Players[id] = p
	s.SetECSEntity(p)
	s.SetCharacterInWorld(true)
	return p, s
}

// portals is every portal NPC standing.
func (f *portalFixture) portals() []*NPC {
	var out []*NPC
	for _, n := range f.w.NPCs {
		if n.PortalOf != 0 {
			out = append(out, n)
		}
	}
	return out
}

// The portal stands in front of its owner, facing it, in the dark portal's
// body, an NPC anyone talks to and no one attacks, never a pet. The control for
// "one a player": a second player's stands beside the first's.
func TestAPortalStandsInFrontOfItsOwner(t *testing.T) {
	f := newPortalFixture(t)
	p, s := f.player(1, "Anna", 1000, 1000, float32(math.Pi/2)) // facing +x
	id, err := f.w.OpenPortal(s, f.now)
	if err != nil {
		t.Fatal(err)
	}
	n := f.w.NPCs[id]
	if n == nil || n.Name != PortalName || n.ModelID != PortalModel || n.PortalOf != p.ServerID {
		t.Fatalf("the portal: %+v", n)
	}
	if n.Owner != 0 || !n.Temporary || n.NPCType != 0x80 || n.Dead {
		t.Errorf("owner %d, temporary %v, type %#x, dead %v: want no pet's owner, Temporary, the NPC bit alone", n.Owner, n.Temporary, n.NPCType, n.Dead)
	}
	if math.Abs(float64(n.X-1003)) > 0.01 || math.Abs(float64(n.Z-1000)) > 0.01 || n.Y != 50 || n.World != 0 {
		t.Errorf("at (%.2f, %.2f, %.2f) world %d, want 3 in front of (1000, 50, 1000)", n.X, n.Y, n.Z, n.World)
	}
	if h := headingTo(p.X-n.X, p.Z-n.Z); math.Abs(float64(n.Facing-h)) > 0.01 {
		t.Errorf("facing %.2f, want %.2f, to its owner", n.Facing, h)
	}
	// the second, the first's owner's next use: the first gone
	again, err := f.w.OpenPortal(s, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if again == id || f.w.NPCs[id] != nil || len(f.portals()) != 1 {
		t.Fatalf("a second use: %d portals, the first still there: %v", len(f.portals()), f.w.NPCs[id] != nil)
	}
	_, s2 := f.player(2, "Bert", 1100, 1000, 0)
	if _, err := f.w.OpenPortal(s2, f.now); err != nil || len(f.portals()) != 2 {
		t.Fatalf("another player's: %d portals, %v", len(f.portals()), err)
	}
	// the dead open none
	q, s3 := f.player(3, "Cora", 1200, 1000, 0)
	q.HP = 0
	if _, err := f.w.OpenPortal(s3, f.now); err != ErrPortalNoPlayer || len(f.portals()) != 2 {
		t.Errorf("a dead player's: %v, %d portals", err, len(f.portals()))
	}
}

// It leaves at its time, when its owner changes world or leaves, or a GM takes
// it away. The controls: a moment before its time, and a move inside the world.
func TestAPortalLeaves(t *testing.T) {
	cases := []struct {
		name string
		do   func(f *portalFixture, p *Player, s *session.Session, id EntityID)
	}{
		{"its time", func(f *portalFixture, p *Player, s *session.Session, id EntityID) {
			f.now = f.now.Add(PortalLasts - time.Second)
			f.w.Tick(f.now)
			if f.w.NPCs[id] == nil {
				f.t.Fatal("gone before its time")
			}
			f.now = f.now.Add(2 * time.Second)
		}},
		{"another world", func(f *portalFixture, p *Player, s *session.Session, id EntityID) {
			f.w.UpdatePosition(p.ServerID, 9000, 50, 9000, 0) // far, in its world
			f.w.Tick(f.now)
			if f.w.NPCs[id] == nil {
				f.t.Fatal("gone at a move inside its world")
			}
			if _, _, ok := f.w.PlaceRespawn(p.ServerID, 1000, 50, 1000, 2); !ok {
				f.t.Fatal("no move")
			}
		}},
		{"logged out", func(f *portalFixture, p *Player, s *session.Session, id EntityID) {
			f.w.RemovePlayerBySession(s)
		}},
		{"a GM's #despawn", func(f *portalFixture, p *Player, s *session.Session, id EntityID) {
			if _, ok := f.w.DespawnNPC(id); !ok {
				f.t.Fatal("no despawn")
			}
		}},
		{"a GM's kill", func(f *portalFixture, p *Player, s *session.Session, id EntityID) {
			if !f.w.KillNPCNow(id) {
				f.t.Fatal("no kill")
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newPortalFixture(t)
			p, s := f.player(1, "Anna", 1000, 1000, 0)
			id, err := f.w.OpenPortal(s, f.now)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := f.w.PortalSpot(id); !ok {
				t.Fatal("no portal to begin with")
			}
			c.do(f, p, s, id)
			f.w.Tick(f.now)
			if f.w.NPCs[id] != nil || len(f.w.portals) != 0 || p.portal != 0 {
				t.Errorf("still there: npc %v, portals %d, on its owner %d", f.w.NPCs[id] != nil, len(f.w.portals), p.portal)
			}
			if _, ok := f.w.PortalSpot(id); ok {
				t.Error("PortalSpot on a portal gone")
			}
		})
	}
}

// PortalHears: by it, in its world, alive; the controls are each one away.
func TestAPortalHearsWhoIsByIt(t *testing.T) {
	f := newPortalFixture(t)
	_, s := f.player(1, "Anna", 1000, 1000, 0)
	id, err := f.w.OpenPortal(s, f.now)
	if err != nil {
		t.Fatal(err)
	}
	near, _ := f.player(2, "Near", 1000, 1020, 0)
	if !f.w.PortalHears(id, near.ServerID) {
		t.Fatal("not heard beside it")
	}
	far, _ := f.player(3, "Far", 1000, 1040, 0)
	if f.w.PortalHears(id, far.ServerID) {
		t.Error("heard from past its reach")
	}
	other, _ := f.player(4, "Other", 1000, 1020, 0)
	other.World = 1
	if f.w.PortalHears(id, other.ServerID) {
		t.Error("heard from another world")
	}
	dead, _ := f.player(5, "Dead", 1000, 1020, 0)
	dead.HP = 0
	if f.w.PortalHears(id, dead.ServerID) {
		t.Error("heard from a dead player")
	}
	if f.w.PortalHears(id+1, near.ServerID) || f.w.PortalHears(id, 99) {
		t.Error("heard with no such portal or player")
	}
}

// A bring is a travel to the exact spot the owner stood on as it opened the
// portal (the GM Jibwiz's petition 1922: ring 0, not a ring round it, and not
// where the owner stands now), handed to the hook by the tick. A player from
// another world goes the same. The controls: no portal, a dead player.
func TestAPortalBringsPlayersWhereItsOwnerStood(t *testing.T) {
	f := newPortalFixture(t)
	a, s := f.player(1, "Anna", 1000, 1000, 0)
	id, err := f.w.OpenPortal(s, f.now)
	if err != nil {
		t.Fatal(err)
	}
	a.X, a.Z = 1040, 1040 // the owner walks off after opening it
	b, bs := f.player(2, "Bert", 9000, 9000, 0)
	b.World = 2
	if !f.w.PortalBring(id, b.ServerID, bs) {
		t.Fatal("not brought")
	}
	f.w.Tick(f.now)
	if len(f.sent) != 1 {
		t.Fatalf("%d travels", len(f.sent))
	}
	tr := f.sent[0]
	if tr.P != b || tr.Sess != bs || tr.To.World != 0 || tr.To.X != 1000 || tr.To.Y != 50 || tr.To.Z != 1000 || tr.Ring != 0 || tr.Spell != PortalName {
		t.Errorf("the travel: %+v", tr)
	}
	f.sent = nil
	b.HP = 0
	if f.w.PortalBring(id, b.ServerID, bs) {
		t.Error("a dead player brought")
	}
	if f.w.PortalBring(id+1, 1, s) {
		t.Error("brought by no portal")
	}
	f.w.Tick(f.now)
	if len(f.sent) != 0 {
		t.Errorf("%d travels after the refusals", len(f.sent))
	}
}

