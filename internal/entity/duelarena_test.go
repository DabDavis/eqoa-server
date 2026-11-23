package entity

import (
	"fmt"
	"net"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/eqoa/server/internal/ground"
	"github.com/eqoa/server/internal/session"
)

func privateDuel(t *testing.T) (*duelists, *Duel) {
	t.Helper()
	d := newDuelists(t)
	d.w.SetDuelArena(func(*session.Session, GateSpot, float32) {})
	t.Cleanup(func() { d.w.SetDuelArena(nil) })
	d.sa.SetServerWorlds([]uint8{DuelArenaWorld})
	d.sb.SetServerWorlds([]uint8{DuelArenaWorld})
	d.a.DuelWindow, d.b.DuelWindow = true, true
	d.a.CharServerID, d.b.CharServerID = 11, 12
	d.a.Effects = []*ActiveEffect{{SpellID: 99, Name: "Ward"}}
	d.a.Buffs = []BuffSlot{{Name: "Ward"}, {Name: "Badge"}}
	d.a.HP, d.b.Power = 5, 10
	d.w.ArenaChallenge(d.sa, d.aSeesB, "", d.at(0))
	match := d.w.duels.of[d.a.ServerID]
	if match == nil || d.a.World != 0 || len(d.a.Effects) == 0 {
		t.Fatal("challenge moved the players or missed the invitation")
	}
	d.w.DuelBoxAnswer(d.sb, DuelBoxID(match.ID), 0, d.at(time.Second))
	if d.a.World != DuelArenaWorld || !SamePlace(d.a, d.b) || d.a.Instance != match.ID || !match.Starts.IsZero() || len(d.a.Effects)+len(d.a.Buffs) != 0 || d.a.HP != 5 || d.b.Power != 10 {
		t.Fatal("wrong landing, buffs, or premature heal")
	}
	return d, match
}

func TestNeriakArenaCountdownAndWinner(t *testing.T) {
	d, m := privateDuel(t)
	origin, _, ok := d.w.ArenaReturn(d.b)
	if !ok || origin.World != 0 || origin.X != 104 {
		t.Fatalf("saved return %+v", origin)
	}
	d.w.ArenaLoaded(d.sa)
	d.tick(2 * time.Second)
	if !m.Starts.IsZero() {
		t.Fatal("first client started both clocks")
	}
	d.w.ArenaLoaded(d.sb)
	duelDrain(d.sa)
	duelDrain(d.sb)
	d.tick(3 * time.Second)
	expect := func(line string) {
		t.Helper()
		for _, s := range []*session.Session{d.sa, d.sb} {
			if got := duelLines(duelDrain(s)); !slices.Contains(got, line) {
				t.Fatalf("wanted %q, got %q", line, got)
			}
		}
	}
	expect("Duel starts in 60 seconds.")
	// Opposite ends of the outside ring are farther apart than the ordinary
	// duel radius. Both remain inside this field for the entire countdown.
	d.a.X, d.b.X = arenaX-45, arenaX+45
	for left := 55; left >= 5; left -= 5 {
		d.tick(time.Duration(63-left) * time.Second)
		expect(fmt.Sprintf("Duel starts in %d seconds.", left))
	}
	for left := 3; left >= 2; left-- {
		d.tick(time.Duration(63-left) * time.Second)
		expect(fmt.Sprint(left))
	}
	if d.a.HP != 5 || d.b.Power != 10 || d.w.duelOnLocked(d.a.ServerID) != nil {
		t.Fatal("heal or combat before one")
	}
	d.tick(62 * time.Second)
	expect("1")
	if d.a.HP != d.a.MaxHPNow() || d.b.Power != d.b.MaxPowerNow() || d.a.Power != d.a.MaxPowerNow() || d.b.HP != d.b.MaxHPNow() {
		t.Fatal("missing full HP or mana")
	}
	d.tick(63 * time.Second)
	expect("FIGHT")
	if m.Phase != duelOn {
		t.Fatal("fight did not open")
	}
	d.w.duelHurtAtLocked(m, d.a, d.a.ServerID, d.b, 100000, 100000, d.at(64*time.Second), numberSpell)
	if d.w.duels.byID[m.ID] != nil || d.a.X != 100 || d.b.X != 104 || d.a.World != 0 || d.b.World != 0 || d.b.HP != 1 || d.a.arenaStay != nil || d.b.arenaStay != nil {
		t.Fatal("winner did not close the match and return both to their own origins")
	}
}

func TestNeriakArenaAdmissionsAndInterruptions(t *testing.T) {
	d, m := privateDuel(t)
	d.c.GroupID, d.a.GroupID = 40, 40
	d.c.World, d.c.Instance = DuelArenaWorld, m.ID
	if SamePlace(d.a, d.c) {
		t.Fatal("a group member with a stale instance ID entered")
	}
	if _, _, ok := d.w.PlaceFacing(d.c.ServerID, arenaX, arenaFloor, arenaZ, DuelArenaWorld, 0); ok {
		t.Fatal("ordinary warp entered the match")
	}
	d.c.World, d.c.Instance = 0, 0
	before := d.a.X
	d.w.ApplyClientUpdate(d.a.ServerID, ClientUpdate{X: arenaX + 90, Y: 1, Z: arenaZ, World: DuelArenaWorld, HasWorld: true})
	if d.a.X != before {
		t.Fatal("crossed a sealed doorway")
	}
	d.sc.SetServerWorlds([]uint8{DuelArenaWorld})
	d.c.Flying = true
	d.w.ObserveArena(d.sc, d.a.Name)
	if d.c.arenaStay != nil {
		t.Fatal("flight without GM entered")
	}
	d.c.GM, d.c.Flying = true, false
	d.w.ObserveArena(d.sc, d.a.Name)
	if d.c.arenaStay != nil {
		t.Fatal("GM on foot entered")
	}
	d.c.Flying = true
	d.w.ObserveArena(d.sc, d.a.Name)
	if !SamePlace(d.a, d.c) || !d.c.arenaStay.observer {
		t.Fatal("flying GM could not observe")
	}
	d.w.SetFlying(d.sc, false, d.at(2*time.Second))
	if d.c.World != 0 || d.c.arenaStay != nil {
		t.Fatal("observer stayed after flight ended")
	}
	d.c.Flying = true
	d.w.ObserveArena(d.sc, d.a.Name)
	d.w.DuelCancel(d.sb, d.at(3*time.Second))
	if d.w.duels.byID[m.ID] != nil || d.a.World != 0 || d.b.World != 0 || d.c.World != 0 {
		t.Fatal("cancel left a private instance behind")
	}
}

func TestTwoNeriakArenasDoNotSharePlayers(t *testing.T) {
	d, first := privateDuel(t)
	p := &Player{ServerID: 500000004, CharServerID: 14, Name: "Delta", HP: 300, MaxHP: 300, X: 110, Y: 10, Z: 100}
	d.c.X, d.c.Y, d.c.Z, d.c.CharServerID = 109, 10, 100, 13
	sd := d.w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 22), Port: 8100}, 22, 22, 1, 2, true)
	sd.SetECSEntity(p)
	sd.SetPlayerObjectID(1212)
	sd.SetCharacterInWorld(true)
	sd.SetServerWorlds([]uint8{DuelArenaWorld})
	d.w.Players[p.ServerID] = p
	d.sc.SetServerWorlds([]uint8{DuelArenaWorld})
	d.w.ArenaChallenge(d.sc, 0, p.Name, d.at(2*time.Second))
	second := d.w.duels.of[d.c.ServerID]
	if second == nil || second.ID == first.ID {
		t.Fatal("second private invitation missing")
	}
	d.w.DuelAnswer(sd, second.ID, true, d.at(3*time.Second))
	if !SamePlace(p, d.c) || SamePlace(p, d.a) || SamePlace(d.c, d.b) || d.c.X != d.a.X {
		t.Fatal("co-located matches share players")
	}
	d.w.DuelCancel(d.sb, d.at(4*time.Second))
	if d.c.arenaStay == nil || p.arenaStay == nil || d.w.duels.byID[second.ID] != second {
		t.Fatal("closing the first arena closed the second")
	}
}

func TestNeriakArenaLoadTimeoutAndLateClock(t *testing.T) {
	d, m := privateDuel(t)
	d.tick(92 * time.Second)
	if d.a.World != 0 || d.b.World != 0 || d.w.duels.byID[m.ID] != nil {
		t.Fatal("failed load stranded the other player")
	}
	d, m = privateDuel(t)
	d.w.ArenaLoaded(d.sa)
	d.w.ArenaLoaded(d.sb)
	d.tick(2 * time.Second)
	d.tick(65 * time.Second)
	if m.Phase != duelCountdown || d.a.HP != d.a.MaxHPNow() || m.said != 1 {
		t.Fatal("stalled clock skipped the one-second restoration")
	}
	d.tick(66 * time.Second)
	d.w.RemovePlayer(d.b.ServerID)
	if m.Phase != duelOn || d.a.World != 0 || d.w.duels.byID[m.ID] != nil {
		t.Fatal("opponent did not return after disconnect")
	}
}

func TestNeriakDiscArena(t *testing.T) {
	dir := os.Getenv("EQOA_NERIAK_GROUND")
	if dir == "" {
		t.Skip("set EQOA_NERIAK_GROUND to the baked arena")
	}
	g, err := ground.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	env, err := ground.OpenEnv(dir)
	if err != nil {
		t.Fatal(err)
	}
	d, m := privateDuel(t)
	d.w.SetEnv(env)
	if !d.w.ArenaLavaReady() {
		t.Fatal("arena startup did not recognize all four lava volumes")
	}
	for _, x := range []float32{-16, 16} {
		for _, z := range []float32{-16, 16} {
			d.a.X, d.a.Z = arenaX+x, arenaZ+z
			y, ok := g.Floor(DuelArenaWorld, d.a.X, 12, d.a.Z)
			if !ok {
				t.Fatal("missing lava basin")
			}
			d.a.Y = y
			d.a.HP = 500
			d.a.lavaNext = time.Time{}
			d.w.lavaSystem(d.at(2 * time.Second))
			if d.a.HP >= 500 {
				t.Fatalf("no damage in lava at %v,%v", x, z)
			}
		}
	}
	for _, v := range [][2]float32{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		dx, dz := v[0], v[1]
		for _, radius := range []float32{0, 41} {
			x, z := arenaX+dx*radius, arenaZ+dz*radius
			y, ok := g.Floor(DuelArenaWorld, x, 12, z)
			f, _ := env.Flags(DuelArenaWorld, x, y+lavaFeet, z)
			if !ok || f&envLava != 0 {
				t.Fatalf("missing safe floor at %v,%v", x, z)
			}
		}
		if !g.Wall(DuelArenaWorld, arenaX+45*dx, 3, arenaZ+45*dz, arenaX+51*dx, 3, arenaZ+51*dz) {
			t.Fatalf("open doorway %v", v)
		}
		if _, ok := g.Floor(DuelArenaWorld, arenaX+55*dx, 50, arenaZ+55*dz); ok {
			t.Fatalf("floor outside arena %v", v)
		}
	}
	// Lethal lava ends this duel under duel rules, including for Hardcore.
	d.w.ArenaLoaded(d.sa)
	d.w.ArenaLoaded(d.sb)
	d.tick(3 * time.Second)
	d.tick(62 * time.Second)
	d.tick(63 * time.Second)
	d.a.HP = 1
	d.a.lavaNext = time.Time{}
	d.w.lavaSystem(d.at(64 * time.Second))
	if d.a.HP <= 0 || d.a.World != 0 || d.b.World != 0 || d.w.duels.byID[m.ID] != nil {
		t.Fatal("lethal lava failed to settle the duel")
	}
}
