package entity

import (
	"bytes"
	"encoding/binary"
	"math"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/ground"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// sunFixture is a vampire and a mortal, each in its own session, whose
// clients were set to the same hour at t0 (none set for a negative hour).
type sunFixture struct {
	w      *World
	vs, ms *session.Session
	v, m   *Player
	t0     time.Time
}

// readingAt is a 0x0013's second reading that shows the hour.
func readingAt(hour float32) int64 {
	return int64(session.GameDay)*472_000 + int64(hour*float32(session.GameDay)/24)
}

func newSunFixture(t *testing.T, hour float32) *sunFixture {
	t.Helper()
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	join := func(n byte, p *Player, objectID uint32) *session.Session {
		s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, n), Port: 7000}, uint32(n), uint32(n), 1, 2, true)
		w.Players[p.ServerID] = p
		s.SetECSEntity(p)
		s.SetCharacterInWorld(true)
		s.SetPlayerObjectID(objectID)
		return s
	}
	// Afkqq as Sandstorm had it: a level 43 cleric of 1849 hit points, its
	// object id Sandstorm's
	v := &Player{ServerID: 1001, CharServerID: 501, Name: "Afkqq", Class: 9, Level: 43, HP: 1849, MaxHP: 1849,
		X: 15290.4, Y: 54.1, Z: 9248.6, Vampire: true}
	m := &Player{ServerID: 1002, CharServerID: 502, Name: "Mortal", Class: 9, Level: 43, HP: 1849, MaxHP: 1849,
		X: 15295, Y: 54.1, Z: 9250}
	f := &sunFixture{w: w, v: v, m: m, t0: time.Unix(1_700_000_000, 0)}
	f.vs = join(7, v, 0x284AEFD9)
	f.ms = join(8, m, 0x284AEFDA)
	if hour >= 0 {
		f.vs.SetGameClock(readingAt(hour), f.t0)
		f.ms.SetGameClock(readingAt(hour), f.t0)
	}
	return f
}

// tick runs the whole world's tick n times, 100 ms apart, from t0.
func (f *sunFixture) tick(n int) {
	for i := 0; i < n; i++ {
		f.w.Tick(f.t0.Add(time.Duration(f.w.TickCount+1) * 100 * time.Millisecond))
	}
}

// sun runs the sun alone n ticks (the world's count moved on), 100 ms apart.
func (f *sunFixture) sun(n int) {
	f.w.mu.Lock()
	defer f.w.mu.Unlock()
	for i := 0; i < n; i++ {
		f.w.TickCount++
		f.w.vampireSunSystem(f.t0.Add(time.Duration(f.w.TickCount) * 100 * time.Millisecond))
	}
}

// burns is each 0x00DB from no one a session was sent, whole, taken off its
// queue.
func burns(s *session.Session) [][]byte {
	var out [][]byte
	for _, m := range s.Queue().GetPending() {
		d := m.Data()
		if len(d) >= 6 && opOfMsg(d) == protocol.OpcodeDamageIndication && binary.LittleEndian.Uint32(d[2:]) == sunSource {
			out = append(out, append([]byte(nil), d...))
		}
	}
	return out
}

// countOp is how many messages of op are waiting for a session.
func countOp(s *session.Session, op protocol.GameOpcode) int {
	n := 0
	for _, m := range s.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 2 && opOfMsg(d) == op {
			n++
		}
	}
	return n
}

// By day the sun takes 50 off a vampire every 6 s, on the regeneration's tick
// and after it, told as Sandstorm told Afkqq at its 10:18 (afkqq-1, byte for
// byte: db00 ffffffff 63 d9ef4a28, from no one, zigzag -50, to the player);
// the mortal beside it takes nothing. Nothing before the tick, nothing again
// until the next.
func TestTheSunBurnsAVampireByDayAsSandstormDoes(t *testing.T) {
	f := newSunFixture(t, 10.3)
	f.tick(59)
	if f.v.HP != 1849 || len(burns(f.vs)) != 0 {
		t.Fatalf("before the tick: %d HP, burned", f.v.HP)
	}
	f.tick(1)
	if f.v.HP != 1799 {
		t.Errorf("at 6 s the vampire has %d HP, want 1799", f.v.HP)
	}
	b := burns(f.vs)
	if len(b) != 1 || !bytes.Equal(b[0], unhex(t, "db00ffffffff63d9ef4a28")) {
		t.Errorf("the vampire was told %x, want Sandstorm's db00ffffffff63d9ef4a28", b)
	}
	if f.m.HP != 1849 || len(burns(f.ms)) != 0 {
		t.Errorf("the mortal: %d HP, burned", f.m.HP)
	}
	// the next tick: the regeneration's HoT first, then the sun
	hot := f.v.HoTNow()
	f.tick(59)
	if f.v.HP != 1799 || len(burns(f.vs)) != 0 {
		t.Errorf("between the ticks: %d HP, burned", f.v.HP)
	}
	f.tick(1)
	if f.v.HP != 1799+hot-50 || len(burns(f.vs)) != 1 {
		t.Errorf("at 12 s: %d HP, want %d (1799 + HoT %d - 50)", f.v.HP, 1799+hot-50, hot)
	}
}

// At night the sun takes nothing; the same vampire by day, the control,
// burns. The sun is up after 6:00 and before 18:00, as the client's sky has
// it.
func TestTheSunSparesAVampireAtNight(t *testing.T) {
	night := newSunFixture(t, 2.7)
	night.sun(600)
	if night.v.HP != 1849 || len(burns(night.vs)) != 0 {
		t.Errorf("at night: %d HP, burned", night.v.HP)
	}
	day := newSunFixture(t, 15)
	day.sun(600)
	if day.v.HP != 1849-10*50 || len(burns(day.vs)) != 10 {
		t.Errorf("the control, by day: %d HP over a minute, want %d", day.v.HP, 1849-10*50)
	}
	for _, c := range []struct {
		h   float32
		day bool
	}{{0, false}, {5.99, false}, {6.01, true}, {12, true}, {17.99, true}, {18.01, false}, {23.99999, false}} {
		if Daylight(c.h) != c.day {
			t.Errorf("Daylight(%v) = %v", c.h, !c.day)
		}
	}
}

// A non-vampire never burns, whatever the hour; the vampire beside it, the
// control, does by day.
func TestTheSunNeverBurnsAMortal(t *testing.T) {
	for _, h := range []float32{0.5, 6.5, 12, 17.5, 20} {
		f := newSunFixture(t, h)
		f.sun(60)
		if f.m.HP != 1849 || len(burns(f.ms)) != 0 {
			t.Errorf("at %v the mortal has %d HP", h, f.m.HP)
		}
		if burned := f.v.HP < 1849; burned != Daylight(h) {
			t.Errorf("at %v the vampire burned: %v", h, burned)
		}
	}
}

// The amount and the interval are the settings' (world.vampire_sun_*): the
// default 50 every 6 s; 7 every 2 s; 0 nothing. Three minutes' worth each.
func TestTheSunsDamageAndIntervalAreTheSettings(t *testing.T) {
	for _, c := range []struct {
		rule  VampireSun
		burns int
		each  int
	}{
		{DefaultVampireSun, 30, 50},
		{VampireSun{Damage: 7, Interval: 2 * time.Second, Shade: true, GMs: true}, 90, 7},
		{VampireSun{Damage: 0, Interval: 6 * time.Second}, 0, 0},
	} {
		f := newSunFixture(t, 9)
		f.v.MaxHP, f.v.HP = 100000, 100000
		f.w.SetVampireSun(c.rule)
		f.sun(1800)
		b := burns(f.vs)
		if len(b) != c.burns || f.v.HP != 100000-c.burns*c.each {
			t.Errorf("%+v: %d burns, %d HP; want %d of %d", c.rule, len(b), f.v.HP, c.burns, c.each)
			continue
		}
		for _, m := range b {
			if got := int(int32(zigzagDecode(m[6:]))); got != -c.each {
				t.Errorf("%+v: told %d, want %d", c.rule, got, -c.each)
			}
		}
	}
}

// A vampire burned to 0 dies as any player does (playerDiedLocked): 0x005E
// once, its death's debt, off the creatures' threat lists, the respawn 15 s on
// with its hit points whole; the sun takes nothing off the corpse.
func TestTheSunKillsAsAnyDeath(t *testing.T) {
	f := newSunFixture(t, 12)
	f.v.Level, f.v.HP = 20, 30
	npc := &NPC{ServerID: 2001, Name: "orc pawn", Level: 20, HP: 100, MaxHP: 100, X: 15300, Y: 54, Z: 9250,
		CurrentEnemy: f.v.ServerID, Threats: map[EntityID]int64{f.v.ServerID: 10}}
	f.w.NPCs[npc.ServerID] = npc
	f.sun(60)
	if f.v.HP != 0 || !f.v.respawnPending() || f.v.Debt != DeathDebt(20) {
		t.Fatalf("burned to 0: %d HP, dead %v, debt %d (want %d)", f.v.HP, f.v.respawnPending(), f.v.Debt, DeathDebt(20))
	}
	if n := countOp(f.vs, protocol.OpcodeCharacterDied); n != 1 {
		t.Errorf("%d deaths told, want 1", n)
	}
	if _, hates := npc.Threats[f.v.ServerID]; hates || npc.CurrentEnemy != 0 {
		t.Errorf("the creature still hates the dead vampire")
	}
	f.vs.Queue().GetPending()
	f.sun(120)
	if len(burns(f.vs)) != 0 || countOp(f.vs, protocol.OpcodeCharacterDied) != 0 {
		t.Error("the sun burned the corpse")
	}
	f.w.mu.Lock()
	due := f.w.playerRespawnSystem(f.v.DiedAt.Add(PlayerRespawnDelay))
	f.w.mu.Unlock()
	if len(due) != 1 || f.v.HP != f.v.MaxHP || f.v.respawnPending() {
		t.Errorf("the respawn: %d due, %d HP, dead %v", len(due), f.v.HP, f.v.respawnPending())
	}
}

// roofGround is a ground whose roofs are boxes over (x0, z0)-(x1, z1) at a
// height, and which notes the worlds asked about.
type roofGround struct {
	roofs []struct{ x0, z0, x1, z1, y float32 }
	asked []uint8
}

func (g *roofGround) Floor(_ uint8, _, y, _ float32) (float32, bool) { return y, false }

func (g *roofGround) Roof(world uint8, x, y0, y1, z float32) bool {
	g.asked = append(g.asked, world)
	for _, r := range g.roofs {
		if world == 5 && x >= r.x0 && x <= r.x1 && z >= r.z0 && z <= r.z1 && r.y > y0 && r.y <= y1 {
			return true
		}
	}
	return false
}

// Under a roof a vampire does not burn by day, and burns once it steps out;
// with vampire_sun_shade off, the control, the roof shelters nothing. S2's
// world 7 (Secrets.esf again) is asked of as world 5.
func TestAVampireInTheShadeDoesNotBurn(t *testing.T) {
	f := newSunFixture(t, 11)
	g := &roofGround{roofs: []struct{ x0, z0, x1, z1, y float32 }{{4300, 6170, 4340, 6190, 300}}}
	f.w.SetGround(g)
	f.v.World, f.v.X, f.v.Y, f.v.Z = 7, 4322.19, 274.13, 6179.78
	f.sun(120)
	if f.v.HP != 1849 || len(burns(f.vs)) != 0 {
		t.Errorf("under the roof: %d HP, burned", f.v.HP)
	}
	for _, w := range g.asked {
		if w != 5 {
			t.Errorf("world %d asked of, want 5 for 7", w)
		}
	}
	f.v.X = 4350 // out from under it
	f.sun(60)
	if f.v.HP != 1799 {
		t.Errorf("stepped out: %d HP, want 1799", f.v.HP)
	}
	f.v.X = 4322.19
	f.w.SetVampireSun(VampireSun{Damage: 50, Interval: 6 * time.Second, Shade: false, GMs: true})
	f.sun(60)
	if f.v.HP != 1749 {
		t.Errorf("the control, shade off, under the roof: %d HP, want 1749", f.v.HP)
	}
}

// A GM's vampire burns as anyone's unless vampire_sun_gms is false; a
// player's vampire, the control, burns either way.
func TestAGMsVampireBurnsUnlessTheSettingsSpareIt(t *testing.T) {
	f := newSunFixture(t, 13)
	f.m.Vampire = true
	f.w.SetPlayerGM(f.vs, true)
	if !f.v.GM || f.m.GM {
		t.Fatal("the GM flag")
	}
	f.sun(60)
	if f.v.HP != 1799 || f.m.HP != 1799 {
		t.Errorf("by default: the GM %d, the player %d; want both 1799", f.v.HP, f.m.HP)
	}
	f.w.SetVampireSun(VampireSun{Damage: 50, Interval: 6 * time.Second, Shade: true, GMs: false})
	f.sun(60)
	if f.v.HP != 1799 || f.m.HP != 1749 {
		t.Errorf("GMs spared: the GM %d (want 1799), the player %d (want 1749)", f.v.HP, f.m.HP)
	}
}

// Not in the world, the sun takes nothing: a client that has not entered it,
// one loading a world (its state channels held); the control, the same
// vampire entered, burns.
func TestTheSunWaitsForAVampireInTheWorld(t *testing.T) {
	f := newSunFixture(t, 8)
	f.vs.SetCharacterInWorld(false)
	f.sun(60)
	f.vs.SetCharacterInWorld(true)
	f.vs.HoldStateChannels(time.Hour)
	f.sun(60)
	if f.v.HP != 1849 {
		t.Errorf("out of the world or loading one: %d HP", f.v.HP)
	}
	f.vs.ReleaseStateChannels()
	f.sun(60)
	if f.v.HP != 1799 {
		t.Errorf("the control, in the world: %d HP, want 1799", f.v.HP)
	}
}

// Day is the client's, not the server's: an hour after a 0x0013 that showed
// 6:12 the client shows 5:47, its clock 983.04 ticks a second where the
// server's 60-minute day has come round to 6:12 again, so the vampire does
// not burn; the server's own hour then (the control) is day.
func TestTheSunFollowsTheClientsClock(t *testing.T) {
	f := newSunFixture(t, -1)
	day := int64(session.GameDay)
	t0 := time.UnixMilli(day*472_000 + 3_180_000) // the server's 6:12
	f.t0 = t0
	f.vs.SetGameClock(t0.UnixMilli()+session.ClockOffset, t0) // as sendTimeOnly sends it
	at := t0.Add(time.Hour)
	if h := session.ServerHour(at); !Daylight(h) {
		t.Fatalf("the control: the server's hour an hour on is %v, not day", h)
	}
	if h := ClientHour(f.vs, at); Daylight(h) {
		t.Fatalf("the client's hour an hour on is %v, day", h)
	}
	f.w.mu.Lock()
	f.w.TickCount = 0
	f.w.vampireSunSystem(at)
	f.w.mu.Unlock()
	if f.v.HP != 1849 {
		t.Errorf("burned by the server's clock: %d HP", f.v.HP)
	}
}

// A real bake (EQOA_GROUND_DIR, with its walls files): by day a vampire in
// Freeport's inn, where Grindal stands, does not burn and burns once it steps
// out into the street north of it; in Neriak's street, under the cavern, it
// does not; the control, with shade off, it burns in the inn. Skipped
// without a bake: none is in this repository.
func TestRealShadeInFreeportsInn(t *testing.T) {
	dir := os.Getenv("EQOA_GROUND_DIR")
	if dir == "" {
		t.Skip("EQOA_GROUND_DIR names a baked ground")
	}
	if _, err := os.Stat(filepath.Join(dir, "0.walls")); err != nil {
		t.Skip("the bake has no walls files")
	}
	g, err := ground.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	f := newSunFixture(t, 12)
	f.w.SetGround(g)
	place := func(x, y, z float32) { f.v.X, f.v.Y, f.v.Z = x, y, z }
	place(25284.0, 57.1, 15657.5)
	f.sun(60)
	if f.v.HP != 1849 {
		t.Errorf("in the inn: %d HP", f.v.HP)
	}
	place(25284.0, 54.09, 15645.5)
	f.sun(60)
	if f.v.HP != 1799 {
		t.Errorf("in the street: %d HP, want 1799", f.v.HP)
	}
	place(25285.2, 0.6, 8905.7)
	f.sun(60)
	if f.v.HP != 1799 {
		t.Errorf("in Neriak's street: %d HP, want 1799", f.v.HP)
	}
	place(25284.0, 57.1, 15657.5)
	f.w.SetVampireSun(VampireSun{Damage: 50, Interval: 6 * time.Second, Shade: false, GMs: true})
	f.sun(60)
	if f.v.HP != 1749 {
		t.Errorf("the control, shade off, in the inn: %d HP, want 1749", f.v.HP)
	}
}

// A real bake (EQOA_GROUND_DIR, with its walls files): the Sanctuary, where
// Sandstorm's Sanctuary spell lands and tools/seed_vampires.py puts its
// people (Hinder, Creed, Weaver, Simmons, its coffin), is a floor at 273.62
// under the rock: shade for each spot, its gate landing on its place, and by
// day a vampire there does not burn. The control: on the open ground north
// of the building, where the four stood before, it burns. Skipped without a
// bake: none is in this repository.
func TestRealSanctuary(t *testing.T) {
	dir := os.Getenv("EQOA_GROUND_DIR")
	if dir == "" {
		t.Skip("EQOA_GROUND_DIR names a baked ground")
	}
	if _, err := os.Stat(filepath.Join(dir, "5.walls")); err != nil {
		t.Skip("the bake has no walls files")
	}
	g, err := ground.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	for _, at := range [][2]float32{{4322.19, 6179.78}, {4327.73, 6182.08}, {4326.43, 6184.02}, {4326.43, 6175.54}, {4327.73, 6177.48}, {4323.72, 6183.48}} {
		fy, ok := g.Floor(5, at[0], 274.134+0.5, at[1])
		if !ok || fy < 273.57 || fy > 273.67 {
			t.Errorf("(%.2f, %.2f): floor %.2f (%v), want 273.62", at[0], at[1], fy, ok)
		}
		if !g.Roof(5, at[0], fy+1, fy+1000, at[1]) {
			t.Errorf("(%.2f, %.2f): no roof over the floor", at[0], at[1])
		}
	}
	row := database.SpellGate{Spell: "sanctuary", Kind: database.GateSelf, Place: "The Sanctuary", World: 5, X: 4322.19, Y: 273.62, Z: 6179.78}
	f := newSunFixture(t, 12)
	f.w.SetGround(g)
	f.w.mu.Lock()
	x, y, z := f.w.gateLandingLocked(row)
	f.w.mu.Unlock()
	if x != row.X || math.Abs(float64(y-row.Y)) > 0.05 || z != row.Z {
		t.Errorf("the Sanctuary's gate lands at (%.2f, %.2f, %.2f), want its place", x, y, z)
	}
	f.v.World, f.v.X, f.v.Y, f.v.Z = 5, 4322.19, 273.62, 6179.78
	f.sun(60)
	if f.v.HP != 1849 {
		t.Errorf("by day in the Sanctuary: %d HP", f.v.HP)
	}
	f.v.X, f.v.Y, f.v.Z = 4300, 253.5, 6118
	f.sun(60)
	if f.v.HP != 1799 {
		t.Errorf("the control, the open ground north of the building: %d HP, want 1799", f.v.HP)
	}
}
