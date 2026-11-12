package entity

import (
	"testing"
	"time"
)

// Petition 1496 (the magician Waldorf, in Permafrost): "pets in general don't
// respect walls/barriers/floors/ceilings etc. They just take the quickest line
// to target". On the chase routes' own ground (chaseroute_test.go's terrace
// over a field, a cliff between them but for a ramp): a pet following its
// owner and one sent at a creature go by the ramp, never off the ledge nor up
// through the cliff; so does a mercenary; a pet 3 from its owner across the
// ground but on the floor under it goes up to it. The controls: with the
// routes off, each takes the straight line through the cliff, as before; with
// no ramp at all the pet still comes (straight, as before); and a familiar,
// never owned, follows as it did.

// walkRig is the pet rig on the ledge's ground: its owner at (ox, oy, 0), its
// pet summoned and put at (px, py, 0).
func walkRig(t *testing.T, g Ground, ox, oy, px, py float32) (*petRig, *NPC) {
	t.Helper()
	r := newPetRig(t)
	r.w.SetGround(g)
	move := func(id EntityID, x0, z0, x, z float32) { r.w.SpatialIndex.Update(id, x0, z0, x, z) }
	move(r.p.ServerID, r.p.X, r.p.Z, ox, 0)
	r.p.X, r.p.Y, r.p.Z, r.p.World = ox, oy, 0, 0
	pet, _ := r.summon(t, fireElemental(), 0)
	move(pet.ServerID, pet.X, pet.Z, px, 0)
	pet.X, pet.Y, pet.Z = px, py, 0
	r.enemy.World = 0
	move(r.enemy.ServerID, r.enemy.X, r.enemy.Z, 500, 500)
	r.enemy.X, r.enemy.Y, r.enemy.Z = 500, 0, 500
	return r, pet
}

// walk ticks the rig's pets from 10 s on, a tenth of a second at a time, until
// done says so or ticks pass, and gives back where n stood, a tick apart.
func (r *petRig) walk(n *NPC, ticks int, done func() bool) [][3]float32 {
	at := [][3]float32{{n.X, n.Y, n.Z}}
	for i := 1; i <= ticks; i++ {
		r.w.TickCount++
		now := r.at(10*time.Second + time.Duration(i)*100*time.Millisecond)
		r.w.mu.Lock()
		r.w.aiSystem(now)
		r.w.petSystem(now)
		r.w.mercSystem(now)
		r.w.familiarSystem(now)
		r.w.mu.Unlock()
		at = append(at, [3]float32{n.X, n.Y, n.Z})
		if done() {
			break
		}
	}
	return at
}

func onRamp(at [][3]float32) bool {
	for _, p := range at {
		if p[0] > 1 && p[0] < 19 && p[2] >= 16 && p[2] <= 24 && p[1] > 0.5 && p[1] < 9.5 {
			return true
		}
	}
	return false
}

func withRoutes(routes bool, f func()) {
	was := chaseRoutes
	chaseRoutes = routes
	defer func() { chaseRoutes = was }()
	f()
}

func TestAPetFollowsItsOwnerRoundALedgeByTheRamp(t *testing.T) {
	ramp := ledgeGround{rampZ0: 16, rampZ1: 24}
	for _, c := range []struct {
		what           string
		ox, oy, px, py float32
	}{{"down", 12, 0, -6, 10}, {"up", -12, 10, 8, 0}} {
		for _, routes := range []bool{true, false} {
			withRoutes(routes, func() {
				r, pet := walkRig(t, ramp, c.ox, c.oy, c.px, c.py)
				at := r.walk(pet, 600, func() bool {
					return distXZ(pet.X, pet.Z, r.p.X, r.p.Z) <= petFollowStart && absf(pet.Y-r.p.Y) <= 1 && pet.Loco == LocoStand
				})
				last := at[len(at)-1]
				arrived := distXZ(last[0], last[2], r.p.X, r.p.Z) <= petFollowStart && absf(last[1]-r.p.Y) <= 1
				i, bad := fallsOrClimbs(at)
				switch {
				case routes && !arrived:
					t.Errorf("%s: the pet never came to its owner: at %v after %d ticks", c.what, last, len(at)-1)
				case routes && bad:
					t.Errorf("%s: tick %d took the pet from %v to %v", c.what, i, at[i-1], at[i])
				case routes && !onRamp(at):
					t.Errorf("%s: the pet was never on the ramp", c.what)
				case !routes && !bad:
					t.Errorf("%s, the control: with the routes off the pet never went through the cliff", c.what)
				}
			})
		}
	}
}

func TestAPetSentAtACreatureGoesRoundByTheRamp(t *testing.T) {
	ramp := ledgeGround{rampZ0: 16, rampZ1: 24}
	for _, routes := range []bool{true, false} {
		withRoutes(routes, func() {
			r, pet := walkRig(t, ramp, -8, 10, -6, 10)
			r.w.SpatialIndex.Update(r.enemy.ServerID, r.enemy.X, r.enemy.Z, 10, 0)
			r.enemy.X, r.enemy.Y, r.enemy.Z = 10, 0, 0
			r.enemy.HP, r.enemy.MaxHP = 1_000_000, 1_000_000
			r.w.PetCommand(r.sess, petCmdAttack, crabObject, r.at(10*time.Second))
			at := r.walk(pet, 600, func() bool { return r.enemy.HP < r.enemy.MaxHP })
			i, bad := fallsOrClimbs(at)
			switch {
			case r.enemy.HP == r.enemy.MaxHP:
				t.Errorf("routes %v: the pet never hit the crab below: at %v", routes, at[len(at)-1])
			case routes && (bad || !onRamp(at)):
				t.Errorf("the pet's way to the crab: on the ramp %v, tick %d from %v to %v", onRamp(at), i, at[max(i-1, 0)], at[i])
			case !routes && !bad:
				t.Error("the control: with the routes off the pet never went off the ledge")
			}
		})
	}
}

// A pet beside its owner across the ground but on the floor under it does not
// stand there, nor fight a creature over it from there: it goes up to it. The
// control: with the routes off it stands under its owner, as before.
func TestAPetUnderItsOwnersFloorGoesUpToIt(t *testing.T) {
	ramp := ledgeGround{rampZ0: 16, rampZ1: 24}
	for _, routes := range []bool{true, false} {
		withRoutes(routes, func() {
			r, pet := walkRig(t, ramp, -1.5, 10, 1.5, 0)
			at := r.walk(pet, 600, func() bool { return absf(pet.Y-r.p.Y) <= 1 && distXZ(pet.X, pet.Z, r.p.X, r.p.Z) <= petFollowStart })
			up := absf(pet.Y-r.p.Y) <= 1
			i, bad := fallsOrClimbs(at)
			switch {
			case routes && (!up || bad || !onRamp(at)):
				t.Errorf("the pet under its owner's floor: up %v, on the ramp %v, tick %d from %v to %v", up, onRamp(at), i, at[max(i-1, 0)], at[i])
			case !routes && up:
				t.Error("the control: with the routes off the pet came up to its owner's floor")
			}
		})
	}
	// its target over it: no fight through the floor
	r, pet := walkRig(t, ramp, 3, 0, 1.5, 0)
	r.w.SpatialIndex.Update(r.enemy.ServerID, r.enemy.X, r.enemy.Z, -1.5, 0)
	r.enemy.X, r.enemy.Y, r.enemy.Z = -1.5, 10, 0
	r.enemy.HP, r.enemy.MaxHP = 1_000_000, 1_000_000
	r.w.PetCommand(r.sess, petCmdAttack, crabObject, r.at(10*time.Second))
	below := true
	at := r.walk(pet, 600, func() bool {
		if r.enemy.HP < r.enemy.MaxHP {
			below = below && absf(pet.Y-r.enemy.Y) > routeLevel
			return true
		}
		return false
	})
	if r.enemy.HP == r.enemy.MaxHP || below {
		t.Errorf("the pet under its target: hit it %v, from the floor below %v (at %v)", r.enemy.HP < r.enemy.MaxHP, below, at[len(at)-1])
	}
}

// With no way round (the terrace with no ramp) the pet still comes to its
// owner, straight, as it always did; and it looks for a way again only after
// the chase's wait.
func TestAPetWithNoWayRoundGoesStraight(t *testing.T) {
	r, pet := walkRig(t, ledgeGround{rampZ0: 1e6, rampZ1: 1e6 + 1}, 12, 0, -6, 10)
	at := r.walk(pet, 300, func() bool { return distXZ(pet.X, pet.Z, r.p.X, r.p.Z) <= petFollowStart && pet.Loco == LocoStand })
	last := at[len(at)-1]
	if distXZ(last[0], last[2], r.p.X, r.p.Z) > petFollowStart || absf(last[1]-r.p.Y) > 1 {
		t.Fatalf("with no way round the pet never came to its owner: at %v after %d ticks", last, len(at)-1)
	}
	if pet.way.route.failedUntil.IsZero() {
		t.Error("no search was made")
	}
	// the control: on flat ground it never searches
	f, fp := walkRig(t, ledgeGround{rampZ0: 1e6, rampZ1: 1e6 + 1}, 60, 0, 30, 0)
	f.walk(fp, 100, func() bool { return distXZ(fp.X, fp.Z, f.p.X, f.p.Z) <= petFollowStart })
	if !fp.way.route.failedUntil.IsZero() || len(fp.way.route.pts) != 0 || distXZ(fp.X, fp.Z, f.p.X, f.p.Z) > petFollowStart {
		t.Errorf("the control on flat ground: searched %v, %.1f from its owner", !fp.way.route.failedUntil.IsZero(), distXZ(fp.X, fp.Z, f.p.X, f.p.Z))
	}
}

// A mercenary follows its owner round the ledge as a pet does; a familiar,
// never owned, takes the straight line as before.
func TestAMercFollowsRoundALedgeAndAFamiliarAsBefore(t *testing.T) {
	r := newMercRig(t)
	r.w.SetGround(ledgeGround{rampZ0: 16, rampZ1: 24})
	r.w.SpatialIndex.Update(r.p.ServerID, r.p.X, r.p.Z, 12, 0)
	r.p.X, r.p.Y, r.p.Z = 12, 0, 0
	ms := r.hire(r.kit(0, 0, 0), MercPassive)
	fs := r.callFamiliar(0)
	place := func(n *NPC) {
		r.w.SpatialIndex.Update(n.ServerID, n.X, n.Z, -6, 0)
		n.X, n.Y, n.Z = -6, 10, 0
	}
	place(ms.npc)
	place(fs.npc)
	merc, fam := [][3]float32{{-6, 10, 0}}, [][3]float32{{-6, 10, 0}}
	for i := 1; i <= 600; i++ {
		r.w.TickCount++
		r.tick(10*time.Second + time.Duration(i)*100*time.Millisecond)
		merc = append(merc, [3]float32{ms.npc.X, ms.npc.Y, ms.npc.Z})
		fam = append(fam, [3]float32{fs.npc.X, fs.npc.Y, fs.npc.Z})
	}
	if d := distXZ(ms.npc.X, ms.npc.Z, r.p.X, r.p.Z); d > mercStrollFar+followArrive || // at ease it strolls round its owner (mercidle.go)
		absf(ms.npc.Y-r.p.Y) > 1 {
		t.Fatalf("the merc never came to its owner: %.1f off at height %.1f", d, ms.npc.Y)
	}
	if i, bad := fallsOrClimbs(merc); bad || !onRamp(merc) {
		t.Errorf("the merc's way: on the ramp %v, tick %d from %v to %v", onRamp(merc), i, merc[max(i-1, 0)], merc[i])
	}
	if _, bad := fallsOrClimbs(fam); !bad || onRamp(fam) {
		t.Errorf("the familiar, never owned: fell off the ledge %v, on the ramp %v; want its straight line as before", bad, onRamp(fam))
	}
}
