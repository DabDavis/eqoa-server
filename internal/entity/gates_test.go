package entity

import (
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The spells that send players to a named place (gates.go; jib's review, F3
// pass 19). The lines are EQOA-Data's; the places, ids and people made up.

type gateFixture struct {
	t      *testing.T
	w      *World
	mgr    *session.Manager
	sess   *session.Session
	caster *Player
	t0     time.Time
	sent   []GateTravel
	port   int
}

// newGateFixture is a wizard of 50 at (1000, 50, 1000) in Tunaria, in a
// group of its own, the travel hook recording what it is handed.
func newGateFixture(t *testing.T) *gateFixture {
	t.Helper()
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	f := &gateFixture{t: t, w: w, mgr: mgr, t0: time.Unix(1_800_000_000, 0), port: 7100}
	f.caster, f.sess = f.player(1001, "Wizard", 1000, 50, 1000)
	f.caster.Class, f.caster.Level, f.caster.Power, f.caster.MaxPower = 13, 50, 2000, 2400
	w.groups = map[uint32]*Group{7: {ID: 7, Members: []EntityID{f.caster.ServerID}}}
	f.caster.GroupID = 7
	w.OnTravel(func(tr GateTravel) { f.sent = append(f.sent, tr) })
	return f
}

// player puts a player in the world with a session of its own, in world 0.
func (f *gateFixture) player(id EntityID, name string, x, y, z float32) (*Player, *session.Session) {
	f.t.Helper()
	f.port++
	s := f.mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 1, 1), Port: f.port}, uint32(f.port), 1, 1, 2, true)
	p := &Player{ServerID: id, Name: name, Level: 40, HP: 900, MaxHP: 900, Power: 500, MaxPower: 600, Speed: 8.833333, X: x, Y: y, Z: z}
	f.w.Players[id] = p
	s.SetECSEntity(p)
	s.SetCharacterInWorld(true)
	s.SetPlayerObjectID(0x10000000 + uint32(id))
	return p, s
}

// member is a player of the caster's group.
func (f *gateFixture) member(id EntityID, name string, x, y, z float32) (*Player, *session.Session) {
	p, s := f.player(id, name, x, y, z)
	g := f.w.groups[7]
	g.Members = append(g.Members, id)
	p.GroupID = 7
	return p, s
}

// gate is a spellgates row.
func (f *gateFixture) gate(g database.SpellGate) {
	rows := map[string]database.SpellGate{}
	f.w.mu.RLock()
	for k, v := range f.w.gates.rows {
		rows[k] = v
	}
	f.w.mu.RUnlock()
	g.Spell = database.SpellGateKey(g.Spell)
	rows[g.Spell] = g
	f.w.SetSpellGates(rows)
}

// cast casts the spell and lands it, the travellers handed over; it
// reports whether it began.
func (f *gateFixture) cast(spell CastSpell) bool {
	f.t.Helper()
	if !f.w.StartCast(f.sess, spell, 0xFFFFFFFF, f.t0) {
		return false
	}
	f.w.mu.Lock()
	f.w.castSystem(f.t0.Add(spell.CastTime + castLand))
	f.w.mu.Unlock()
	f.w.runTravels()
	return true
}

// travellers are the names the travel hook was handed, in its order.
func (f *gateFixture) travellers() []string {
	var out []string
	for _, tr := range f.sent {
		out = append(out, tr.P.Name)
	}
	return out
}

// redLines are the 0x0A7B lines queued for the session.
func redLines(s *session.Session) []string {
	var out []string
	for _, m := range s.Queue().PeekPending() {
		d := m.Data()
		if len(d) < 6 || protocol.GameOpcode(binary.LittleEndian.Uint16(d)) != protocol.OpcodeColoredChat {
			continue
		}
		n := int(binary.LittleEndian.Uint32(d[2:]))
		var b strings.Builder
		for i := 0; i < n && 6+2*i+1 < len(d); i++ {
			b.WriteRune(rune(binary.LittleEndian.Uint16(d[6+2*i:])))
		}
		out = append(out, b.String())
	}
	return out
}

func neriusGate() CastSpell {
	return CastSpell{SpellID: 700157, DisplayIndex: 3, Scope: 0, Name: "Nerius Gate", CastTime: 2 * time.Second,
		Recast: 4 * time.Second, PowerCost: 96, Range: 4.5,
		Def: ParseSpellNamed("Nerius Gate", "Self, Teleports the caster to the Kara Village (s)", "")}
}

func stonebruntCircle() CastSpell {
	return CastSpell{SpellID: 700709, DisplayIndex: 4, Scope: 2, Name: "Circle of Stonebrunt", CastTime: time.Second,
		Recast: 5 * time.Second, PowerCost: 660, Range: 30,
		Def: ParseSpellNamed("Circle of Stonebrunt", "Group, Set Maximum Power to 0, teleport to SBM", "")}
}

func meltonEvacuation() CastSpell {
	return CastSpell{SpellID: 700415, DisplayIndex: 5, Scope: 2, Name: "Melton Evacuation", CastTime: time.Second,
		Recast: 5 * time.Second, PowerCost: 444, Range: 30,
		Def: ParseSpellNamed("Melton Evacuation", "Group, Set Maximum Power to 0 and Teleport to Melton", "")}
}

// The table, read into the world: a spell finds its row by its name in any
// case; a row the game cannot use is left out; the control, a spell with no
// row, finds none.
func TestSpellGatesReadIntoTheWorld(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/gates.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO spellgates (spell, kind, place, world, x, y, z, radius) VALUES
		('nerius gate', 'self', 'Kara Village', 0, 21000, 61, 7667, 0),
		('circle of stonebrunt', 'group', 'Stonebrunt Mountains', 2, 9000, 80, 7000, 20),
		('broken gate', 'nowhere', 'x', 0, 0, 0, 0, 0)`); err != nil {
		t.Fatal(err)
	}
	w := NewWorld()
	n, err := w.LoadSpellGates(db)
	if err != nil || n != 2 {
		t.Fatalf("read %d gates (%v), want 2: the broken row left out", n, err)
	}
	g, ok := w.SpellGate("Nerius GATE")
	if !ok || g.World != 0 || g.X != 21000 || g.Z != 7667 || g.Place != "Kara Village" {
		t.Errorf("Nerius Gate: %+v %v", g, ok)
	}
	if g, ok := w.SpellGate("Circle of Stonebrunt"); !ok || g.Kind != database.GateGroup || g.World != 2 {
		t.Errorf("Circle of Stonebrunt: %+v %v", g, ok)
	}
	if _, ok := w.SpellGate("Broken Gate"); ok {
		t.Error("the broken row was read")
	}
	if _, ok := w.SpellGate("Minor Healing"); ok {
		t.Error("the control: a spell with no row has a gate")
	}
	if s := w.SpellGatesSummary(); !strings.Contains(s, "2 in spellgates (1 self, 1 group") {
		t.Errorf("summary %q", s)
	}
}

// A self gate sends its caster alone, to its row's place (no ground here:
// as written), at its landing; its power is the cost the cast took, and the
// group member beside it stays. The control: before the landing nothing is
// handed over.
func TestASelfGateSendsItsCaster(t *testing.T) {
	f := newGateFixture(t)
	f.member(1002, "Friend", 1003, 50, 1000)
	f.gate(database.SpellGate{Spell: "Nerius Gate", Kind: database.GateSelf, Place: "Kara Village", World: 0, X: 21000, Y: 61, Z: 7667})
	if !f.w.StartCast(f.sess, neriusGate(), 0xFFFFFFFF, f.t0) {
		t.Fatal("not begun")
	}
	f.w.mu.Lock()
	f.w.castSystem(f.t0.Add(2*time.Second + castLand - time.Millisecond))
	f.w.mu.Unlock()
	f.w.runTravels()
	if len(f.sent) != 0 {
		t.Fatalf("the control: handed %v before the landing", f.travellers())
	}
	f.w.mu.Lock()
	f.w.castSystem(f.t0.Add(2*time.Second + castLand))
	f.w.mu.Unlock()
	f.w.runTravels()
	if got := f.travellers(); len(got) != 1 || got[0] != "Wizard" {
		t.Fatalf("sent %v, want the caster alone", got)
	}
	tr := f.sent[0]
	if tr.To != (GateSpot{World: 0, X: 21000, Y: 61, Z: 7667, Place: "Kara Village"}) || tr.Home != 0 || tr.Ring != 0 || tr.Sess != f.sess {
		t.Errorf("sent to %+v (home %d, ring %d)", tr.To, tr.Home, tr.Ring)
	}
	if f.caster.Power != 2000-96 {
		t.Errorf("power %d, want %d: the cast's cost alone (its line takes none)", f.caster.Power, 2000-96)
	}
}

// A group gate takes its caster and the members of its group within the
// row's radius of it as it lands (20 for a row of 0), alive, in its world,
// not loading one; never anyone outside the group. Its line's "Set Maximum
// Power to 0" leaves the caster no power; the members keep theirs. The
// control: a row's own radius of 30 takes the member at 25 too.
func TestAGroupGateTakesOnlyTheNearGroup(t *testing.T) {
	f := newGateFixture(t)
	near, _ := f.member(1002, "Near", 1010, 50, 1000)
	f.member(1003, "Far", 1025, 50, 1000)
	other, _ := f.member(1004, "Elsewhere", 1005, 50, 1000)
	other.World = 2
	dead, _ := f.member(1005, "Dead", 1000, 50, 1005)
	dead.HP = 0
	_, loading := f.member(1006, "Loading", 1000, 50, 995)
	loading.HoldStateChannels(time.Minute)
	f.player(1007, "Stranger", 1002, 50, 1002) // not of the group
	f.gate(database.SpellGate{Spell: "Circle of Stonebrunt", Kind: database.GateGroup, Place: "Stonebrunt Mountains", World: 2, X: 9000, Y: 80, Z: 7000})
	if !f.cast(stonebruntCircle()) {
		t.Fatal("not begun")
	}
	if got := f.travellers(); strings.Join(got, ",") != "Wizard,Near" {
		t.Fatalf("sent %v, want the caster and the member within 20", got)
	}
	for i, tr := range f.sent {
		if tr.To.World != 2 || tr.To.X != 9000 || tr.Ring != i {
			t.Errorf("%s sent to %+v ring %d", tr.P.Name, tr.To, tr.Ring)
		}
	}
	if f.caster.Power != 0 || near.Power != 500 {
		t.Errorf("power: caster %d (want 0), member %d (want 500)", f.caster.Power, near.Power)
	}

	// the control: the row's radius 30 takes the member at 25
	f.sent = nil
	f.caster.Power = 2000
	f.caster.Lockouts = nil
	f.gate(database.SpellGate{Spell: "Circle of Stonebrunt", Kind: database.GateGroup, Place: "Stonebrunt Mountains", World: 2, X: 9000, Y: 80, Z: 7000, Radius: 30})
	if !f.cast(stonebruntCircle()) {
		t.Fatal("the control: not begun")
	}
	if got := f.travellers(); strings.Join(got, ",") != "Wizard,Near,Far" {
		t.Fatalf("the control: sent %v, want the member at 25 too", got)
	}
}

// A gate whose place is in a world the caster's game cannot load is refused
// as it begins, in red, its power not spent and no lockout set; a line that
// teleports with no row here likewise. The controls: the same gate for a
// game that loads the world begins; a spell that does not teleport, with no
// row, begins.
func TestAGateToAWorldTheGameCannotLoadIsRefused(t *testing.T) {
	f := newGateFixture(t)
	discSix := func(*session.Session) func(uint8) bool { return func(w uint8) bool { return w < 6 } }
	f.w.SetLoadableWorlds(discSix)
	f.gate(database.SpellGate{Spell: "Nerius Gate", Kind: database.GateSelf, Place: "a far shore", World: 7, X: 1000, Y: 50, Z: 1000})
	if f.w.StartCast(f.sess, neriusGate(), 0xFFFFFFFF, f.t0) {
		t.Fatal("a gate to world 7 began for a game of the disc's six")
	}
	if got := redLines(f.sess); len(got) != 1 || got[0] != "Your game cannot load a far shore." {
		t.Errorf("told %q", got)
	}
	if f.caster.Power != 2000 || len(f.caster.Lockouts) != 0 || f.caster.Casting != nil {
		t.Errorf("power %d, lockouts %d, casting %v: want nothing spent", f.caster.Power, len(f.caster.Lockouts), f.caster.Casting)
	}
	f.sess.Queue().GetPending()

	// the control: a game that loads it
	f.w.SetLoadableWorlds(func(*session.Session) func(uint8) bool { return func(uint8) bool { return true } })
	if !f.w.StartCast(f.sess, neriusGate(), 0xFFFFFFFF, f.t0) {
		t.Fatal("the control: not begun for a game that loads world 7")
	}

	// a teleport with no row here
	g := newGateFixture(t)
	if g.w.StartCast(g.sess, neriusGate(), 0xFFFFFFFF, g.t0) {
		t.Fatal("a teleport with no row began")
	}
	if got := redLines(g.sess); len(got) != 1 || got[0] != gateNoPlace || g.caster.Power != 2000 {
		t.Errorf("told %q, power %d", got, g.caster.Power)
	}
	plain := neriusGate()
	plain.Name, plain.Def = "Minor Shielding", ParseSpellText("Self, Buff MaxHP by 15 & AC by 28")
	if !g.w.StartCast(g.sess, plain, 0xFFFFFFFF, g.t0) {
		t.Fatal("the control: a spell that does not teleport was refused")
	}
}

// An evacuation takes the group near its caster to the caster's home (the
// travel hook resolves it as Return Home's), the group round the caster, and
// its line leaves the caster no power. The control: the same row made a
// group gate lands at its place instead.
func TestAnEvacuationSendsTheGroupHome(t *testing.T) {
	f := newGateFixture(t)
	f.member(1002, "Near", 1005, 50, 1000)
	f.member(1003, "Far", 1100, 50, 1000)
	row := database.SpellGate{Spell: "Melton Evacuation", Kind: database.GateEvac, Place: "Melton", World: 0, X: 6780, Y: 60, Z: 19450}
	f.gate(row)
	if !f.cast(meltonEvacuation()) {
		t.Fatal("not begun")
	}
	if got := f.travellers(); strings.Join(got, ",") != "Wizard,Near" {
		t.Fatalf("sent %v, want the caster and the near member", got)
	}
	for i, tr := range f.sent {
		if tr.Home != f.caster.ServerID || tr.Ring != i {
			t.Errorf("%s: home %d ring %d, want the caster's home, ring %d", tr.P.Name, tr.Home, tr.Ring, i)
		}
	}
	if f.caster.Power != 0 {
		t.Errorf("power %d, want 0", f.caster.Power)
	}

	// the control: made a group gate, it lands at its place
	f.sent = nil
	f.caster.Lockouts = nil
	f.caster.Power = 2000
	row.Kind = database.GateGroup
	f.gate(row)
	if !f.cast(meltonEvacuation()) {
		t.Fatal("the control: not begun")
	}
	if len(f.sent) != 2 || f.sent[0].Home != 0 || f.sent[0].To.X != 6780 {
		t.Errorf("the control: sent %+v", f.sent)
	}
}

// gateGround is a flat terrain at 50, with a pond round (1000, 1100) and an
// overhang 30 above the floor round (1000, 900), a cave's roof.
type gateGround struct{}

func (gateGround) Floor(_ uint8, x, y, z float32) (float32, bool) {
	if distXZ(x, z, 1000, 900) < 25 && y+2 >= 80 {
		return 80, true // the overhang
	}
	return 50, 50 <= y+2
}

type gatePond struct{}

func (gatePond) Flags(_ uint8, x, y, z float32) (uint32, bool) {
	if distXZ(x, z, 1000, 1100) < 30 && y < 52 {
		return envWater, true
	}
	return 0, true
}

// The landing on the ground: the place itself when it is open, dry and
// clear; else the nearest spot out of the water, from under a roof, 40 from
// a creature that attacks on sight and 2.5 from anyone's body (a town's
// guard pushes it no farther); a place under the terrain lifted onto it; a
// place well above the terrain (a pyramid's top, which the ground files do
// not hold) as written, creatures or not. The control: no ground lands on
// the place as written.
func TestAGateLandsOnOpenGround(t *testing.T) {
	w := NewWorld()
	w.ground = gateGround{}
	w.env = gatePond{}
	w.playerModels = map[uint32]bool{0x70D898C6: true} // a human's body: townsfolk
	land := func(x, y, z float32) (float32, float32, float32) {
		w.mu.Lock()
		defer w.mu.Unlock()
		return w.gateLandingLocked(database.SpellGate{World: 0, X: x, Y: y, Z: z})
	}
	if x, y, z := land(1000, 55, 1000); x != 1000 || y != 50 || z != 1000 {
		t.Errorf("an open, dry, clear place landed at (%.0f, %.1f, %.0f), want itself on its floor", x, y, z)
	}
	if x, y, z := land(1000, 0, 1000); x != 1000 || y != 50 || z != 1000 {
		t.Errorf("a place under the terrain landed at (%.0f, %.1f, %.0f), want it lifted onto the terrain", x, y, z)
	}
	if x, y, z := land(1000, 50, 1100); distXZ(x, z, 1000, 1100) < 30 || y != 50 {
		t.Errorf("a pond's middle landed at (%.0f, %.1f, %.0f), in the pond", x, y, z)
	}
	if x, _, z := land(1000, 50, 900); distXZ(x, z, 1000, 900) < 25 {
		t.Errorf("under the roof landed at (%.0f, %.0f), still under it", x, z)
	}
	// a town's guard on the place: its body alone kept clear
	w.NPCs[2] = &NPC{ServerID: 2, Name: "guard", ModelID: 0x70D898C6, X: 1000, Y: 50, Z: 1000, SpawnX: 1000, SpawnY: 50, SpawnZ: 1000}
	w.SpatialIndex.Insert(2, 1000, 1000)
	if x, _, z := land(1000, 50, 1000); distXZ(x, z, 1000, 1000) < gateApart || distXZ(x, z, 1000, 1000) > 10.1 {
		t.Errorf("a guard on the place: landed %.1f off, want the nearest spot off its body (10)", distXZ(x, z, 1000, 1000))
	}
	// a creature beside it: 40 clear
	w.NPCs[1] = &NPC{ServerID: 1, Name: "orc", ModelID: 0x1234, X: 1003, Y: 50, Z: 1000, SpawnX: 1003, SpawnY: 50, SpawnZ: 1000}
	w.SpatialIndex.Insert(1, 1003, 1000)
	x, _, z := land(1000, 50, 1000)
	if d := distXZ(x, z, 1003, 1000); d < gateClear {
		t.Errorf("landed %.0f from a creature, want %.0f clear", d, gateClear)
	}
	if d := distXZ(x, z, 1000, 1000); d > 60 {
		t.Errorf("landed %.0f off, want the nearest clear spot", d)
	}
	// a pyramid's top, 31 or 7 above the terrain, the creature beside it
	for _, top := range []float32{81, 57} {
		if x, y, z := land(1000, top, 1000); x != 1000 || y != top || z != 1000 {
			t.Errorf("a place %.0f above the terrain landed at (%.0f, %.1f, %.0f), want it as written", top-50, x, y, z)
		}
	}
	w.ground = nil
	if x, y, z := land(1000, 70, 1100); x != 1000 || y != 70 || z != 1100 {
		t.Errorf("the control: no ground landed at (%.0f, %.1f, %.0f), want the place as written", x, y, z)
	}
}

// The group lands round its caster's spot, on the floor there and out of
// the water, or at its height on a pyramid's top; with no floor near, or no
// ground, on the spot itself.
func TestAGroupLandsRoundItsCaster(t *testing.T) {
	w := NewWorld()
	w.ground = gateGround{}
	w.env = gatePond{}
	at := GateSpot{World: 0, X: 1000, Y: 50, Z: 1000, Place: "here"}
	if s := w.GateRingSpot(at, 0); s != at {
		t.Errorf("ring 0 at %+v, want the spot", s)
	}
	seen := map[[2]float32]bool{}
	for ring := 1; ring <= 3; ring++ {
		s := w.GateRingSpot(at, ring)
		if d := distXZ(s.X, s.Z, at.X, at.Z); d < 2.9 || d > 3.1 || s.Y != 50 {
			t.Errorf("ring %d at (%.1f, %.1f, %.1f), want 3 off on the floor", ring, s.X, s.Y, s.Z)
		}
		seen[[2]float32{s.X, s.Z}] = true
	}
	if len(seen) != 3 {
		t.Errorf("rings 1-3 share spots: %v", seen)
	}
	// in the pond: on the spot
	wet := GateSpot{World: 0, X: 1000, Y: 50, Z: 1100}
	if s := w.GateRingSpot(wet, 1); s != wet {
		t.Errorf("a ring in the pond at %+v, want the spot", s)
	}
	// on a pyramid's top (31 above the terrain): round it at its height
	top := GateSpot{World: 0, X: 1000, Y: 81, Z: 1000}
	if s := w.GateRingSpot(top, 1); s.Y != 81 || distXZ(s.X, s.Z, 1000, 1000) < 2.9 {
		t.Errorf("a ring on a pyramid's top at %+v, want 3 off at its height", s)
	}
	w.ground = nil
	if s := w.GateRingSpot(at, 2); s != at {
		t.Errorf("the control: no ground put ring 2 at %+v, want the spot", s)
	}
}
