package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A login in one of the disc's worlds lands on the stored height, where the
// client stood when it was saved: no drop to fall through
// (F3 20261025-163229). The control: world 22, off the disc, is still lifted.
func TestADiscWorldLoginLandsOnTheStoredHeight(t *testing.T) {
	for world := 0; world < DiscWorlds; world++ {
		w := NewWorld()
		rec := &database.CharacterRecord{CharName: "Lander", Level: 1, X: 25285, Y: 54.12, Z: 15720, World: world}
		p := w.CreatePlayer(nil, rec)
		if p == nil {
			t.Fatal("no player")
		}
		if p.Y != rec.Y {
			t.Errorf("world %d: spawned at y=%.2f over a stored %.2f: a fall at login", world, p.Y, rec.Y)
		}
	}
	w := NewWorld()
	rec := &database.CharacterRecord{CharName: "Faller", Level: 1, X: 8996.8, Y: 58.3, Z: 9002.3, World: 22}
	if p := w.CreatePlayer(nil, rec); p == nil || p.Y != rec.Y+SpawnLift {
		t.Errorf("world 22 (the control) is lifted as before: got %+v", p)
	}
}

// A logout in the air (a fall: the last 0x40's walker flags) saves where the
// player last stood, so a login never begins in mid-air; standing, swimming
// or a report with no flags saves where it is; a place stood in another world
// is not used; a server-side move is a place it stands.
func TestALogoutInTheAirSavesWhereItStood(t *testing.T) {
	w := NewWorld()
	rec := &database.CharacterRecord{CharName: "Walker", Level: 1, X: 25285, Y: 54.12, Z: 15720}
	p := w.CreatePlayer(nil, rec)
	report := func(x, y, z float32, flags uint32, has bool) {
		w.ApplyClientUpdate(p.ServerID, ClientUpdate{X: x, Y: y, Z: z, WalkFlags: flags, HasWalkFlags: has})
	}
	place := func() (float32, float32, float32, bool) { return p.SavePlace() }

	report(25290, 54.12, 15720, 0x0001, true) // standing at a ledge
	if x, y, _, back := place(); back || x != 25290 || y != 54.12 {
		t.Errorf("standing: saved (%.2f, %.2f), back %v; want where it is", x, y, back)
	}
	report(25292, 50.0, 15720, 0x0000, true) // walked off it, falling
	if x, y, z, back := place(); !back || x != 25290 || y != 54.12 || z != 15720 {
		t.Errorf("in a fall: saved (%.2f, %.2f, %.2f), back %v; want the ledge", x, y, z, back)
	}
	report(25300, 40.0, 15720, 0x0004, true) // in water, under its surface
	if _, y, _, back := place(); back || y != 40.0 {
		t.Errorf("swimming: saved y=%.2f, back %v; want where it swims", y, back)
	}
	report(25300, 41.0, 15720, 0x0000, false) // no flags: says nothing of it
	if _, y, _, back := place(); back || y != 41.0 {
		t.Errorf("no flags: saved y=%.2f, back %v; want where it is", y, back)
	}
	report(25300, 45.0, 15720, 0x0000, true)
	p.World = 3 // in the air in another world than it last stood in
	if _, y, _, back := place(); back || y != 45.0 {
		t.Errorf("another world's place: saved y=%.2f, back %v; want where it is", y, back)
	}
	w.UpdatePosition(p.ServerID, 100, 20, 100, 0) // a teleport
	report(100, 19, 100, 0x0000, true)
	if x, y, _, back := place(); !back || x != 100 || y != 20 {
		t.Errorf("after a teleport, in the air: saved (%.2f, %.2f), back %v; want the arrival", x, y, back)
	}
}

// No climb: a session that ends at once, or in the fall out of a lifted
// login (world 22 keeps its lift), logs in again no higher, login after
// login. The controls: the old rules (every login lifted, the height saved
// wherever the player is) climb the lift's three units a login with no
// report at all, and what is left of the fall when the session ends in it.
func TestNoLoginClimbs(t *testing.T) {
	type rules struct {
		name       string
		world      int  // a disc world, or 22, which keeps the lift
		oldSave    bool // the height saved wherever the player is
		quitFall   bool // the session ends half a unit into the lift's fall
		wantClimbs bool
	}
	for _, r := range []rules{
		{"a session that ends at once", 0, false, false, false},
		{"a session that ends in a lifted login's fall", 22, false, true, false},
		{"the old rules, ending at once (control)", 22, true, false, true},
		{"the old rules, ending in the fall (control)", 22, true, true, true},
	} {
		y := float32(54.12)
		for login := 0; login < 10; login++ {
			w := NewWorld()
			p := w.CreatePlayer(nil, &database.CharacterRecord{CharName: "Climber", Level: 1, X: 25285, Y: y, Z: 15720, World: r.world})
			if r.quitFall {
				w.ApplyClientUpdate(p.ServerID, ClientUpdate{X: p.X, Y: p.Y - 0.5, Z: p.Z, WalkFlags: 0, HasWalkFlags: true})
			}
			if r.oldSave {
				y = p.Y
			} else {
				_, y, _, _ = p.SavePlace()
			}
		}
		climbed := y - 54.12
		if r.wantClimbs && climbed < 20 { // at least two units a login
			t.Errorf("%s: the control climbed only %.2f in ten logins", r.name, climbed)
		}
		if !r.wantClimbs && climbed != 0 {
			t.Errorf("%s: climbed %.2f in ten logins", r.name, climbed)
		}
	}
}
