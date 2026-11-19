package entity

import (
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/eqoa/server/internal/database"
)

// questdoors.go — a door an NPC keeps, kept by the server too (petition
// 2212, a player, Heirloom 2026-11-16: "The Guardian in IOD blocking
// entrance to cazic thule house is able to be bypassed via logging and then
// running forward while relogging").
//
// The Estate's Guardian (npcs 118859) stands in the doorway, and his body is
// the door: CLIENT stops its walk at a creature's capsule
// (claude-notes/creature-collision-20261024.md), and only CLIENT. A player
// walked up on to him until he stood in his body ((25705.8, 55.0, 3231.9),
// 1.3 off his middle: "held up in the air", MOVECHECK 01:14:35), logged out
// there (01:15:03), and logged in there (01:15:11): a walker that starts in a
// capsule is not stopped by it, and he walked on in to Kithgrin (offered
// Kithgrin's Talisman 01:16:24). A client whose view has not filled since a
// login walks through him the same way.
//
// So the server keeps the door (table questdoors, database/questdoors.go):
// each character's side of each door in its world is followed from its
// reports. Its outer boxes (the near side, the Guardian's body up to his
// middle included) put it outside; its inner boxes (the far side, just past
// him, where every way in from the door goes) put it inside; anywhere else
// leaves the side as it was. A report that steps into an inner box from
// outside while the door is shut and the character does not pass it is not
// taken: the character is put at the row's outside place (a teleport, 0x00B2,
// as movecheck's move back) and told the row's line. A login, a teleport, a
// respawn (any place the server put it) sets the side from that place alone,
// so a login in the doorway is outside, and the talisman's landing past the
// Guardian is inside; one who was let in (the Guardian out of the world for
// Cazic's Scent's ten minutes) stays in when he comes back, and walks out
// again freely.
//
// Shut: the NPC alive, in the world (not hidden, not spawn-only) and within
// doorPost of its spawn (a Guardian off chasing a scent's holder leaves his
// doorway open, as CLIENT's walk finds it). Passed: a GM; past the row's quest
// step, or holding its item (the quest engine says: SetQuestState's passes,
// by the door's NPC, handlers/questwalls.go).

// doorPost is how far from its spawn the NPC of a door still keeps it.
const doorPost float32 = 8

// doorArrive is how near the place a door put a character a report must be
// for the client to stand there (it heard the teleport); doorResend how long
// a turn back waits for that before it is sent again.
const (
	doorArrive float32 = 4
	doorResend         = 2 * time.Second
)

// doorTurnedBack is the line one turned back at a door is told when its row
// has none.
const doorTurnedBack = "The way is barred."

// doorWatch is a player's standing with the doors (Player.door).
type doorWatch struct {
	known   bool
	world   uint8
	x, y, z float32           // its place after the last report
	outside map[EntityID]bool // the doors it is on the near side of

	back           bool // a turn back the client has not reported from
	backDoor       EntityID
	backWorld      uint8
	bx, by, bz     float32
	backAt, sentAt time.Time
}

// SetQuestDoors sets table questdoors' rows (at the start).
func (w *World) SetQuestDoors(rows []database.QuestDoor) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.qdoors = make(map[EntityID]database.QuestDoor, len(rows))
	for _, r := range rows {
		w.qdoors[EntityID(r.NPCID)] = r
	}
}

// LoadQuestDoors reads table questdoors into the world; it reports how many.
func (w *World) LoadQuestDoors(db *database.SQLiteDB) (int, error) {
	rows, err := db.QuestDoors()
	if err != nil {
		return 0, fmt.Errorf("questdoors: %w", err)
	}
	w.SetQuestDoors(rows)
	return len(rows), nil
}

// QuestDoors is every door, by npcs row.
func (w *World) QuestDoors() []database.QuestDoor {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make([]database.QuestDoor, 0, len(w.qdoors))
	for _, d := range w.qdoors {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NPCID < out[j].NPCID })
	return out
}

// inBoxes is whether one of the boxes holds the place.
func inBoxes(bs []database.WallBox, x, y, z float32) bool {
	for _, b := range bs {
		if b.Holds(x, y, z) {
			return true
		}
	}
	return false
}

// doorShutLocked is whether the door's NPC stands at its post: alive, in the
// world, near its spawn; the NPC itself (nil: none). Must be called with the
// lock held.
func (w *World) doorShutLocked(id EntityID, now time.Time) (*NPC, bool) {
	n := w.NPCs[id]
	if n == nil || n.Dead || n.hiddenAt(now) {
		return n, false
	}
	return n, distXZ(n.X, n.Z, n.SpawnX, n.SpawnZ) <= doorPost
}

// doorPassedLocked is whether p walks through the door: a GM, or one the
// quest engine says passes it (its quest's step, its item). Must be called
// with the lock held.
func (w *World) doorPassedLocked(p *Player, id EntityID) bool {
	return p.GM || w.qwalls.passes[p.CharServerID][id]
}

// doorStepLocked follows p's sides of the doors in its world over one report
// (u, the place before it: before; p's place is the one taken, or before
// when none was) and turns it back at one it may not go through. True: the
// report's place is not taken (p stands where the turn back put it). It is
// ApplyClientUpdate's. Must be called with the lock held.
func (w *World) doorStepLocked(p *Player, before [3]float32, u *ClientUpdate, now time.Time) bool {
	if len(w.qdoors) == 0 {
		return false
	}
	st := &p.door
	if st.outside == nil {
		st.outside = map[EntityID]bool{}
	}
	defer func() { st.known, st.world, st.x, st.y, st.z = true, p.World, p.X, p.Y, p.Z }()
	if !st.known || st.world != p.World || st.x != before[0] || st.y != before[1] || st.z != before[2] {
		// put there by the server since the last report (a login, a
		// teleport, a respawn): the side is that place's alone
		st.back = false
		for id, d := range w.qdoors {
			if n := w.NPCs[id]; n != nil && n.World == p.World {
				st.outside[id] = inBoxes(d.Outer, before[0], before[1], before[2])
			} else {
				delete(st.outside, id)
			}
		}
	}
	if st.back {
		dx, dz := u.X-st.bx, u.Z-st.bz
		if p.World == st.backWorld && dx*dx+dz*dz <= doorArrive*doorArrive && u.Y <= st.by+doorArrive && u.Y >= st.by-doorArrive {
			st.back = false // the client stands where it was put
		} else {
			// from before the client heard of the turn back: not a place it is
			w.putAtLocked(p, st.backWorld, st.bx, st.by, st.bz)
			if now.Sub(st.sentAt) >= doorResend {
				st.sentAt = now
				w.sendMoveBackLocked(p, st.backWorld, st.bx, st.by, st.bz)
			}
			return true
		}
	}
	if p.IsDead() {
		return false
	}
	// a report from before the client heard of a server move (movecheck's
	// pending: it was not from where the server put it) says nothing of the
	// near side: a teleport past a door from its porch is not undone by the
	// porch's last reports. It may still step through one.
	stale := p.mv.init && p.mv.pending
	for id, d := range w.qdoors {
		n := w.NPCs[id]
		if n == nil || n.World != p.World {
			continue
		}
		in := inBoxes(d.Inner, p.X, p.Y, p.Z)
		if in && st.outside[id] && !w.doorPassedLocked(p, id) {
			if _, shut := w.doorShutLocked(id, now); shut {
				w.turnBackLocked(p, id, d, before, now)
				return true
			}
		}
		switch {
		case in:
			st.outside[id] = false
		case !stale && inBoxes(d.Outer, p.X, p.Y, p.Z):
			st.outside[id] = true
		}
	}
	return false
}

// putAtLocked sets p's place (its world's) and stops it. Must be called with
// the lock held.
func (w *World) putAtLocked(p *Player, world uint8, x, y, z float32) {
	if p.World != world {
		return
	}
	oldX, oldZ := p.X, p.Z
	p.X, p.Y, p.Z = x, y, z
	if w.SpatialIndex != nil && (oldX != x || oldZ != z) {
		w.SpatialIndex.Update(p.ServerID, oldX, oldZ, x, z)
	}
	p.VelX, p.VelY, p.VelZ = 0, 0, 0
}

// turnBackLocked puts p at the door's outside place, sends its client there
// and tells it the door's line. Must be called with the lock held.
func (w *World) turnBackLocked(p *Player, id EntityID, d database.QuestDoor, before [3]float32, now time.Time) {
	st := &p.door
	from := [3]float32{p.X, p.Y, p.Z}
	o := d.Outside
	w.putAtLocked(p, p.World, o[0], o[1], o[2])
	st.outside[id] = true
	st.back, st.backDoor, st.backWorld = true, id, p.World
	st.bx, st.by, st.bz = o[0], o[1], o[2]
	st.backAt, st.sentAt = now, now
	w.sendMoveBackLocked(p, p.World, o[0], o[1], o[2])
	text := d.Text
	if text == "" {
		text = doorTurnedBack
	}
	if w.SessionManager != nil {
		if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
			w.sendChatToSession(sess, text)
		}
	}
	name := ""
	if n := w.NPCs[id]; n != nil {
		name = n.Name
	}
	log.Printf("QuestDoor: %s turned back at %q (%d): stepped from (%.1f, %.1f, %.1f) to (%.1f, %.1f, %.1f) world %d; put at (%.1f, %.1f, %.1f)",
		p.Name, name, id, before[0], before[1], before[2], from[0], from[1], from[2], p.World, o[0], o[1], o[2])
}

// nearDoorItemLocked is whether p stands within reach of a door an item
// opens (NearHoldingRule). Must be called with the lock held.
func (w *World) nearDoorItemLocked(p *Player, reach float32) bool {
	for id, d := range w.qdoors {
		if d.Item <= 0 {
			continue
		}
		if n := w.NPCs[id]; n != nil && n.World == p.World && distXZ(n.SpawnX, n.SpawnZ, p.X, p.Z) <= reach {
			return true
		}
	}
	return false
}
