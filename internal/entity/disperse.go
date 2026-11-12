package entity

// disperse.go: what cmd/disperseplan asks of the world to spread stacked camps
// and give static creatures their wander and patrols (the owner, 2026-11-12:
// "there are many zones with mobs not dispersed or patroling, lets fix that
// disperse and patrol"; tools/disperse_patrol_20261112.py applies its plan).
// Nothing here runs in the server: it reads the world as LoadNPCs and the
// rules left it, and asks the wander's own leg check (movement.go) whether a
// creature could walk somewhere.

// DisperseRow is an NPC as the plan sees it: its row, how it moves now and why
// (movement.go's verdict, in a GM's words), and where it stands.
type DisperseRow struct {
	ID        EntityID
	World     uint8
	Name      string
	Level     int
	NPCType   int
	Model     int64
	Size      float32
	X, Y, Z   float32
	Facing    float32
	Wanders   bool
	Why       string
	WhyCode   int
	Word      string
	OnGround  bool
	InWater   bool
	Keeps     string
	Body      float32 // half its body's width (models.txt), 0 unknown
	Guard     bool
	InTown    bool
	Townsman  bool // a players' race's body
	Pathed    bool
	Camp      bool
	SpawnOnly bool
	PetOf     EntityID // a creature's own pet (companion.go): its owner
}

// Why codes cmd/disperseplan reads (movement.go's moveWhy).
const (
	DisperseWhyRowStands  = int(whyRowStands)
	DisperseWhyRowWanders = int(whyRowWanders)
	DisperseWhyWildlife   = int(whyWildlife)
	DisperseWhyUndead     = int(whyUndead)
	DisperseWhyNotWild    = int(whyNotWildlife)
	DisperseWhyOffGround  = int(whyOffGround)
	DisperseWhyTownsfolk  = int(whyTownsfolk)
)

// DisperseSurvey is every NPC as DisperseRow, in no order.
func (w *World) DisperseSurvey() []DisperseRow {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.linkCompanionsLocked()
	out := make([]DisperseRow, 0, len(w.NPCs))
	for _, npc := range w.NPCs {
		ok, why, word := w.movementLocked(npc)
		r := DisperseRow{
			ID: npc.ServerID, World: npc.World, Name: npc.Name, Level: npc.Level, NPCType: npc.NPCType,
			Model: npc.ModelID, Size: npc.Size, X: npc.SpawnX, Y: npc.SpawnY, Z: npc.SpawnZ, Facing: npc.Facing,
			Wanders: ok, Why: MoveWhyText(why, word), WhyCode: int(why), Word: word,
			Pathed: w.pathedLocked(npc.ServerID), Camp: w.campSpawnLocked(npc.ServerID), SpawnOnly: npc.SpawnOnly,
		}
		if w.ground != nil {
			r.OnGround = w.spawnOnGroundLocked(npc)
		}
		if w.env != nil {
			r.InWater = w.spawnInWaterLocked(npc)
			r.Keeps = w.keepsToLocked(npc)
		}
		if b, ok := ModelRadius(uint32(npc.ModelID)); ok {
			r.Body = b * max(npc.Size, 0.1)
		}
		r.PetOf = w.companionOf[npc.ServerID]
		r.Guard = w.townGuardLocked(npc)
		r.InTown = w.inTownLocked(npc.World, npc.SpawnX, npc.SpawnZ)
		r.Townsman = npc.ModelID != 0 && w.playerModels[uint32(npc.ModelID)]
		out = append(out, r)
	}
	return out
}

// DisperseWalk reports whether creature id could walk from (fx, fy, fz) to
// (x, z) as its wander walks a leg (movement.go's whole check: a floor all the
// way, no fall, no wall or anything placed in the way, CLIENT setting it down
// on its own floor, and the water it keeps to), and the floor it ends on.
// The creature is not moved.
func (w *World) DisperseWalk(id EntityID, fx, fy, fz, x, z float32) (y float32, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	npc := w.NPCs[id]
	if npc == nil || w.ground == nil {
		return 0, false
	}
	var hs []float32
	if !w.legFromLocked(npc, fx, fy, fz, x, z, &hs, legWhole) || len(hs) == 0 {
		return 0, false
	}
	return hs[len(hs)-1], true
}

// DisperseInTown reports a point within a town's margin (towns.go): a
// wanderer's leg never ends there, so a spot or a patrol point is not put
// there either.
func (w *World) DisperseInTown(world uint8, x, z float32) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.inTownLocked(world, x, z) || w.legInTownLocked(world, x, z, x, z)
}

// DisperseFloor is the ground's floor at (x, z) nearest under y+1.5 (as a
// creature is set down), false with none.
func (w *World) DisperseFloor(world uint8, x, y, z float32) (float32, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ground == nil {
		return 0, false
	}
	return w.ground.Floor(world, x, y, z)
}

// DisperseHeading is headingTo's, for the plan's facings (a creature faces
// away from its camp's middle, give or take).
func DisperseHeading(dx, dz float32) float32 {
	if dx == 0 && dz == 0 {
		return 0
	}
	return headingTo(dx, dz)
}
