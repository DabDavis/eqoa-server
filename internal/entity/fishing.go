package entity

import (
	"math"
	"time"

	"github.com/eqoa/server/internal/session"
)

// What fishing (handlers/fishing.go) needs of a player as the world holds it:
// who and where it is, whether it may fish now, and what the server knows of
// the water a cast from where it stands would land in.
//
// The cast is CLIENT's (VIWndFishing::FindWater, 0x00C4D5F8): ten ahead along
// the heading, an environment ray from a unit above that point to ten below
// it for water or slime (the zone leaf's flags 1 and 2). The client throws
// only a cast that found it, so the server's own look is a second opinion:
// the same column through the baked leaves, without the solids the client
// also tests (no ground file holds them).

// The water a cast lands in, as the server knows it.
const (
	FishWaterUnknown = -1 // no ground files: the server knows no water
	FishWaterNone    = 0
	FishWaterThere   = 1
)

// fishReach is how far ahead the cast lands, and fishAbove/fishBelow the
// column the client's environment ray searches there.
const (
	fishReach float32 = 10
	fishAbove float32 = 1
	fishBelow float32 = 10
)

// fishFightWindow is how long after a fight's last blow a player counts as
// fighting for fishing: ours.
const fishFightWindow = 6 * time.Second

// envSlime is a zone leaf's slime flag (ground.EnvSlime): FindWater's ray
// takes water and slime (VITransEnvRay::Init's 3).
const envSlime uint32 = 2

// FishingView is a player as fishing sees it.
type FishingView struct {
	Name         string
	CharServerID uint32
	Level        int
	World        uint8
	X, Y, Z      float32
	Facing       float32
	Dead         bool
	Fighting     bool
	// LandX, LandZ: where a cast from here lands; Water what is there
	LandX, LandZ float32
	Water        int
}

// FishingView is the session's player as fishing sees it, false when the
// session has none in the world.
func (w *World) FishingView(sess *session.Session, now time.Time) (FishingView, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return FishingView{}, false
	}
	v := FishingView{
		Name: p.Name, CharServerID: p.CharServerID, Level: p.Level, World: p.World,
		X: p.X, Y: p.Y, Z: p.Z, Facing: p.Facing,
		Dead:     p.IsDead(),
		Fighting: p.Attacking() || (!p.LastCombatTime.IsZero() && now.Sub(p.LastCombatTime) < fishFightWindow),
	}
	// CLIENT's heading: x along sin, z along cos (VIVect3::HPR)
	s, c := math.Sincos(float64(p.Facing))
	v.LandX = p.X + fishReach*float32(s)
	v.LandZ = p.Z + fishReach*float32(c)
	v.Water = w.fishWaterLocked(p.World, v.LandX, p.Y, v.LandZ)
	return v, true
}

// fishWaterLocked is what the server knows of the water in the cast's column
// at (x, z) under a player standing at height y: half a unit at a time from
// fishAbove over it to fishBelow under it. Must be called with the lock held.
func (w *World) fishWaterLocked(world uint8, x, y, z float32) int {
	if w.env == nil {
		return FishWaterUnknown
	}
	known := false
	for dy := fishAbove; dy >= -fishBelow; dy -= 0.5 {
		f, ok := w.env.Flags(world, x, y+dy, z)
		if !ok {
			continue
		}
		known = true
		if f&(envWater|envSlime) != 0 {
			return FishWaterThere
		}
	}
	if !known {
		// no baked leaf there (a world the bake does not hold)
		return FishWaterUnknown
	}
	return FishWaterNone
}
