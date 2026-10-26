package handlers

import (
	"net"
	"slices"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The teleport spells landed as Return Home lands a player (gates.go,
// entity/gates.go; jib's review, F3 pass 19): a wizard and its group of two
// in Tunaria, the hooks set as RegisterDefaultHandlers sets them. The places
// and people are made up.

type gateTrip struct {
	t              *testing.T
	w              *entity.World
	caster, friend *entity.Player
	cs, fs         *session.Session
	t0             time.Time
}

func newGateTrip(t *testing.T) *gateTrip {
	t.Helper()
	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	mk := func(id entity.EntityID, name string, x float32, port int) (*entity.Player, *session.Session) {
		s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 20), Port: port}, uint32(port), 1, 1, 2, true)
		p := &entity.Player{ServerID: id, Name: name, Class: 13, Level: 50, HP: 900, MaxHP: 900, Power: 2000, MaxPower: 2400,
			Speed: 8.833333, X: x, Y: 50, Z: 1000, Facing: 1.25}
		w.Players[id] = p
		s.SetECSEntity(p)
		s.SetCharacterInWorld(true)
		s.SetPlayerObjectID(500000000 + uint32(id))
		s.SetDiscVersion(protocol.GameVersionEQOAVanilla)
		return p, s
	}
	g := &gateTrip{t: t, w: w, t0: time.Unix(1_800_000_000, 0)}
	g.caster, g.cs = mk(9001, "Wizard", 1000, 6101)
	g.friend, g.fs = mk(9002, "Friend", 1005, 6102)
	ch := &CharacterHandler{sessionManager: mgr, gameWorld: w}
	w.OnTravel(ch.travelPlayer)
	w.SetLoadableWorlds(LoadableWorlds)
	w.GroupInvite(g.cs, 0x7F000000|uint32(g.friend.ServerID))
	w.GroupAccept(g.fs, 0x7F000000|uint32(g.caster.ServerID))
	if len(w.GroupMembers(g.caster)) != 2 {
		t.Fatal("no group of two")
	}
	g.cs.Queue().GetPending()
	g.fs.Queue().GetPending()
	return g
}

func (g *gateTrip) gate(row database.SpellGate) {
	row.Spell = database.SpellGateKey(row.Spell)
	g.w.SetSpellGates(map[string]database.SpellGate{row.Spell: row})
}

// cast casts a group gate of that name and line and ticks it to its landing.
func (g *gateTrip) cast(name, line string) {
	g.t.Helper()
	sp := entity.CastSpell{SpellID: 700001, Scope: 2, Name: name, CastTime: time.Second, PowerCost: 100, Range: 30,
		Def: entity.ParseSpellNamed(name, line, "")}
	if !g.w.StartCast(g.cs, sp, 0xFFFFFFFF, g.t0) {
		g.t.Fatalf("%s not begun", name)
	}
	g.w.Tick(g.t0.Add(time.Second + 1500*time.Millisecond))
}

// A portal lands its caster and the group beside it at its place (no ground
// here: as written), by the teleport Return Home sends, the world loaded:
// the state channels held for it, the world changed in the World too.
func TestAPortalLandsTheGroupThere(t *testing.T) {
	g := newGateTrip(t)
	g.gate(database.SpellGate{Spell: "Circle of Stonebrunt", Kind: database.GateGroup, Place: "Stonebrunt Mountains", World: 2, X: 9000, Y: 80, Z: 7000})
	g.cast("Circle of Stonebrunt", "Group, Set Maximum Power to 0, teleport to SBM")
	for _, c := range []struct {
		p *entity.Player
		s *session.Session
	}{{g.caster, g.cs}, {g.friend, g.fs}} {
		x, z, world, ok := teleported(c.s)
		if !ok || x != 9000 || z != 7000 || world != 2 {
			t.Errorf("%s teleported to (%.0f, %.0f) world %d (%v), want Stonebrunt's (9000, 7000) world 2", c.p.Name, x, z, world, ok)
		}
		snap, _ := g.w.PlayerSnapshot(c.p.ServerID)
		if snap.World != 2 || snap.X != 9000 || snap.Y != 80 || snap.Facing != 1.25 {
			t.Errorf("%s in the World at world %d (%.0f, %.1f), facing %.2f", c.p.Name, snap.World, snap.X, snap.Y, snap.Facing)
		}
		if !c.s.StateChannelsHeld() {
			t.Errorf("%s's state channels not held for the load", c.p.Name)
		}
	}
	if snap, _ := g.w.PlayerSnapshot(g.caster.ServerID); snap.Power != 0 {
		t.Errorf("the caster's power %d, want 0 (its line's price)", snap.Power)
	}
}

// A member whose game cannot load the place stays where it is, told so in
// red; its caster, whose game loads it, goes. The control is the caster.
func TestAMemberWhoseGameCannotLoadThePlaceStays(t *testing.T) {
	g := newGateTrip(t)
	g.cs.SetDiscVersion(protocol.GameVersionSandstormS2Live) // S2's eight worlds
	g.gate(database.SpellGate{Spell: "Far Portal", Kind: database.GateGroup, Place: "the far shore", World: 7, X: 1200, Y: 40, Z: 1300})
	g.cast("Far Portal", "Group, Teleport to the far shore")
	if _, _, world, ok := teleported(g.cs); !ok || world != 7 {
		t.Fatalf("the control: the caster went to world %d (%v), want 7", world, ok)
	}
	if _, _, _, ok := teleported(g.fs); ok {
		t.Error("a game of the disc's six was sent to world 7")
	}
	if snap, _ := g.w.PlayerSnapshot(g.friend.ServerID); snap.World != 0 || snap.X != 1005 {
		t.Errorf("the member is at world %d (%.0f), want where it was", snap.World, snap.X)
	}
	var lines []string
	for _, l := range heard(t, g.fs) {
		lines = append(lines, l.text)
	}
	if want := "Your game cannot load the far shore: you stay behind."; !slices.Contains(lines, want) {
		t.Errorf("the member was told %q, want %q", lines, want)
	}
}

// An evacuation lands its caster and the group beside it at the CASTER's
// home, Return Home's (its bind point), the member too though its own bind
// is elsewhere. The control: Return Home (the gate hook) still takes the
// member to its own bind.
func TestAnEvacuationLandsAtTheCastersBind(t *testing.T) {
	g := newGateTrip(t)
	g.w.SetBind(g.caster.ServerID, entity.Bind{Set: true, World: 0, X: 6800, Y: 60, Z: 19450, By: "Spiritmaster Here"})
	g.w.SetBind(g.friend.ServerID, entity.Bind{Set: true, World: 0, X: 12000, Y: 52, Z: 5000, By: "Spiritmaster There"})
	g.gate(database.SpellGate{Spell: "Melton Evacuation", Kind: database.GateEvac, Place: "Melton", World: 0, X: 1, Y: 1, Z: 1})
	g.cast("Melton Evacuation", "Group, Set Maximum Power to 0 and Teleport to Melton")
	for _, s := range []*session.Session{g.cs, g.fs} {
		if x, z, world, ok := teleported(s); !ok || x != 6800 || z != 19450 || world != 0 {
			t.Errorf("teleported to (%.0f, %.0f) world %d (%v), want the caster's bind (6800, 19450)", x, z, world, ok)
		}
	}
	g.fs.Queue().GetPending()
	ch := &CharacterHandler{gameWorld: g.w}
	ch.gatePlayer(g.fs, g.friend)
	if x, z, _, ok := teleported(g.fs); !ok || x != 12000 || z != 5000 {
		t.Errorf("the control: Return Home took the member to (%.0f, %.0f) %v, want its own bind", x, z, ok)
	}
}
