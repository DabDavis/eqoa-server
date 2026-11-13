package entity

import (
	"errors"
	"log"
	"math"
	"time"

	"github.com/eqoa/server/internal/session"
)

// summonportal.go — the portal a Summoning Portal brings (the user,
// 2026-11-07: "lets add heirloom item like the genie lamp, fang of SoW, it is
// summoning portal infinite charges to cast on anyone, it spawns portal NPC,
// players talks to it, just type name of player to summon(doesnt have to be in
// group so different from Call of the Hero)"; handlers/summonportal.go has the
// item, the boxes and the name typed in chat).
//
// A portal is a Temporary NPC of its own (never a pet: Owner stays 0, nothing
// of petSystem), one a player at a time, PortalAhead in front of its owner and
// facing it, in the dark portal's body the game's own portal NPCs wear, with
// only the NPC bit among its flags, so CLIENT's InteractWithTarget talks to it
// (0x0004) and nothing can attack it. Anyone may talk to it, not its owner
// alone. It leaves PortalLasts after it came, when its owner uses the item
// again (a new one in front of it, as a lamp's second genie), changes world or
// leaves; or as any NPC does at a GM's #despawn or #kill. Those it brings land
// on the exact spot its owner stood on as it opened the portal (a travel:
// gates.go, ring 0), a place a body stood on: not at the NPC's own feet, which
// may be behind a wall the owner was facing, and whose body is solid, nor
// round the spot (the GM Jibwiz's petition 1922: until 2026-11-13 arrivals
// landed a ring 3 out, as a Call of the Hero's, and next to a wall a ring's
// place out under the sky was past it, outside the map).
//
// Everything here is ours: the live game's records hold no such item.

// The portal's constants (ours, but for the body).
const (
	PortalName = "Summoning Portal"
	// PortalModel is the dark portal: npcs.modelid of Fuming Portal and 22
	// other rows on eqoaclassic (2026-11-07), its edge (models.txt) 1.14, a
	// man's 1.40
	PortalModel int64 = 2012826467
	// PortalAhead is how far in front of its owner it stands: the two bodies'
	// edges and a little
	PortalAhead float32 = 3
	// PortalLasts is how long it stands
	PortalLasts = 3 * time.Minute
	// PortalReach is how far from it someone who talked to it is still
	// listened to (handlers: the name typed): three times InteractRange
	PortalReach float32 = 30
)

// ErrPortalNoPlayer is a portal refused: no living player in the world.
var ErrPortalNoPlayer = errors.New("no living player to open a portal for")

// portalState is a portal standing: its NPC, its owner, the place it lands
// those it brings (where its owner stood as it opened) and its time.
type portalState struct {
	npc   *NPC
	owner EntityID
	at    GateSpot
	until time.Time
}

// OpenPortal brings the session's player's portal: a new one PortalAhead in
// front of it, any earlier one gone.
func (w *World) OpenPortal(sess *session.Session, now time.Time) (EntityID, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.IsDead() {
		return 0, ErrPortalNoPlayer
	}
	w.dropPortalLocked(p)
	f := float64(p.Facing)
	x := p.X + PortalAhead*float32(math.Sin(f))
	z := p.Z + PortalAhead*float32(math.Cos(f))
	npc := &NPC{
		ServerID: w.allocFreeIDLocked(),
		Name:     PortalName,
		X:        x, Y: p.Y, Z: z,
		Facing:  headingTo(p.X-x, p.Z-z),
		World:   p.World,
		ModelID: PortalModel,
		Size:    1,
		NPCType: 0x80, // the NPC bit alone: CLIENT talks to it
		Level:   max(p.Level, 1),
		HP:      100, MaxHP: 100,
		Temporary: true,
		PortalOf:  p.ServerID,
	}
	npc.SpawnX, npc.SpawnY, npc.SpawnZ, npc.SpawnFacing = npc.X, npc.Y, npc.Z, npc.Facing
	w.putNPCLocked(npc)
	w.SpatialIndex.Insert(npc.ServerID, npc.X, npc.Z)
	if w.portals == nil {
		w.portals = map[EntityID]*portalState{}
	}
	w.portals[npc.ServerID] = &portalState{
		npc:    npc,
		owner:  p.ServerID,
		at:     GateSpot{World: p.World, X: p.X, Y: p.Y, Z: p.Z, Place: "a summoning portal"},
		until:  now.Add(PortalLasts),
	}
	p.portal = npc.ServerID
	log.Printf("Portal: %s's portal %d at (%.1f, %.1f) world=%d", p.Name, npc.ServerID, x, z, p.World)
	return npc.ServerID, nil
}

// portalStandingLocked is the portal while it stands. Must be called with the lock
// held.
func (w *World) portalStandingLocked(portal EntityID) *portalState {
	st := w.portals[portal]
	if st == nil || st.npc.Dead {
		return nil
	}
	return st
}

// PortalSpot is where the portal lands those it brings, while it stands.
func (w *World) PortalSpot(portal EntityID) (GateSpot, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	st := w.portalStandingLocked(portal)
	if st == nil {
		return GateSpot{}, false
	}
	return st.at, true
}

// PortalHears is whether the portal stands and the player is still by it: in
// its world, alive, within PortalReach of it.
func (w *World) PortalHears(portal, player EntityID) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	st, p := w.portalStandingLocked(portal), w.Players[player]
	if st == nil || p == nil || p.IsDead() || p.World != st.npc.World {
		return false
	}
	return math.Hypot(float64(p.X-st.npc.X), float64(p.Z-st.npc.Z)) <= float64(PortalReach)
}

// PortalBring sends the player through the portal, as summonPlayerLocked sends
// a Call of the Hero's: a travel the tick hands to the travel hook (runTravels:
// handlers' travelPlayer, the world loaded when it changes). False when the
// portal no longer stands or the player is gone or dead.
func (w *World) PortalBring(portal, player EntityID, sess *session.Session) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	st, p := w.portalStandingLocked(portal), w.Players[player]
	if st == nil || p == nil || p.IsDead() {
		return false
	}
	log.Printf("Portal: %d brings %s", portal, p.Name)
	w.gates.due = append(w.gates.due, GateTravel{Sess: sess, P: p, To: st.at, Spell: PortalName})
	return true
}

// PortalSend sends player through the portal to a place of its own (the
// portal's "Take me to Freeport", handlers/summonportal.go), as a call's travel
// goes (the gates' queue, travelPlayer), and reports whether it went: not for a
// dead player, nor through a portal closed.
func (w *World) PortalSend(portal, player EntityID, sess *session.Session, to GateSpot) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	st, p := w.portalStandingLocked(portal), w.Players[player]
	if st == nil || p == nil || p.IsDead() {
		return false
	}
	log.Printf("Portal: %d sends %s to %s", portal, p.Name, to.Place)
	w.gates.due = append(w.gates.due, GateTravel{Sess: sess, P: p, To: to, Spell: PortalName})
	return true
}

// closePortalLocked takes the portal out of the world and off its owner. Must
// be called with the lock held.
func (w *World) closePortalLocked(id EntityID, st *portalState) {
	if npc, ok := w.NPCs[id]; ok && npc == st.npc {
		w.removeNPCLocked(npc)
	}
	delete(w.portals, id)
	if p := w.Players[st.owner]; p != nil && p.portal == id {
		p.portal = 0
	}
}

// dropPortalLocked ends the player's portal. Must be called with the lock
// held.
func (w *World) dropPortalLocked(p *Player) {
	if st := w.portals[p.portal]; st != nil {
		w.closePortalLocked(p.portal, st)
	}
	p.portal = 0
}

// portalSystem closes the portals whose time is up, whose owner has changed
// world or is gone, and those a GM took away or killed. Every tick; must be
// called with the lock held.
func (w *World) portalSystem(now time.Time) {
	for id, st := range w.portals {
		owner := w.Players[st.owner]
		npc, stands := w.NPCs[id]
		switch {
		case !stands || npc != st.npc || npc.Dead:
		case owner == nil || owner.portal != id:
		case owner.World != st.at.World:
		case now.After(st.until):
		default:
			continue
		}
		log.Printf("Portal: %d closes", id)
		w.closePortalLocked(id, st)
	}
}
