package entity

import (
	"errors"
	"log"
	"math"
	"time"

	"github.com/eqoa/server/internal/session"
)

// genie.go — the genie a player's lamp brings (the user, 2026-11-06: "lets
// give players a heirloom shop and bank NPC(doesnt replace pet slot) make it a
// genie. the item is a lamp in their inventory, infinite uses. when the player
// prompts, choice for bank and choice to buy/sell at shop. genie disappears
// after the prompt is gone", then "also give third option to genie for
// blacksmith"; handlers/genie.go has the lamp, the box and the windows).
//
// A genie is a Temporary NPC of its own (never a pet: Owner stays 0, nothing
// of petSystem), one a player at a time, GenieAhead in front of its owner and
// facing it, in the magician's Djinn's body, with only the NPC bit among its
// flags, so CLIENT's InteractWithTarget talks to it (0x0004) rather than
// asking for a window. It leaves the world as soon as its box is answered or
// closed ("disappears after the prompt is gone"), or GenieWait after it came
// when no box was answered (a box CLIENT declined at once because a menu was
// up: the genie waits to be talked to). A window it opened (the bank, its
// shop, its forge) is the player's to use while it is: CLIENT sends nothing
// when the bank or the merchant window closes (VIWndMercMenu, ClosePanel6AC)
// and asks for the bank's moves with its own target's id (VIBankControl reads
// m_target), so the server keeps the grant on the player, not on an entity
// (GenieWindowNPC), until GenieWindowIdle passes without a request, the owner
// is GenieReach from where the genie stood, changes world, dies or leaves.
//
// It never touches a pet (petition 1351, Jibrish, a GM's rule: "All charm
// lines and pet lines need to be able to be out and not lose the pet while
// they summon the genie in their inventory. This is an example where dual
// pets are allowed but only for that sort of thing. Dont make it so they can
// summon their spell pet and charm at the same time. Its only for the vendor
// stones."): its coming, its windows and its going leave a summoned or
// charmed pet out, its master's, on its slot, its bar never cleared (no
// 0x007F), and it counts as no pet: not on w.pets, so a charm, a summoning, a
// pet command and a pet's spell go as with no genie by. A genie and a pet are
// the one pair of "pets" a player may have; a summoned and a charmed pet never
// are (charm.go). Held by genie_pet_test.go here and in handlers.

// The genie's constants (ours).
const (
	GenieName = "Genie"
	// GenieModel is the magician's Djinn (petstats 0035, an air elemental)
	GenieModel int64 = 793682180
	// GenieAhead is how far in front of its owner it stands
	GenieAhead float32 = 2.5
	// GenieWait is how long it waits to be talked to after it came
	GenieWait = 2 * time.Minute
	// GenieWindowIdle is how long a window it opened stays the player's
	// with no request made of it
	GenieWindowIdle = 5 * time.Minute
	// GenieReach is how far from where the genie stood its window holds
	GenieReach float32 = 15
)

// GenieWindow is the window a genie opened.
type GenieWindow int

const (
	GenieNoWindow GenieWindow = iota
	GenieBank
	GenieShop
	GenieSmith
)

// ErrGenieNoPlayer is a summons refused: no living player in the world.
var ErrGenieNoPlayer = errors.New("no living player to summon a genie for")

// genieState is a player's genie: the NPC while it stands (0 once it left),
// where it stood, when it came, and the window it opened with the id its
// requests carry (the shop's and the forge's: 0x00B7's merchant, 0x0061's
// blacksmith) and when a request was last made of it.
type genieState struct {
	npc    EntityID
	rec    *NPC // the genie's record, a window's stand-in once it left
	world  uint8
	x, z   float32
	since  time.Time
	window GenieWindow
	id     uint32
	used   time.Time
}

// GenieWindowID is the id a genie's shop and forge go by on the wire (0x00B7,
// 0x0061; their requests carry it back): beside the per-session object ids
// (from 100001) and the group stand-ins (0x7F...), never one of them.
func GenieWindowID(npc EntityID) uint32 { return 0x7E000000 | uint32(npc)&0x00FFFFFF }

// SummonGenie brings the session's player's genie: a new one GenieAhead in
// front of it, any earlier one gone with its window. catalog is the npcitem
// list its shop sells (an Heirloom Outfitter's), the genie's NPCID.
func (w *World) SummonGenie(sess *session.Session, catalog int, now time.Time) (EntityID, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.IsDead() {
		return 0, ErrGenieNoPlayer
	}
	w.dropGenieLocked(p)
	f := float64(p.Facing)
	x := p.X + GenieAhead*float32(math.Sin(f))
	z := p.Z + GenieAhead*float32(math.Cos(f))
	g := &NPC{
		ServerID: w.allocFreeIDLocked(),
		Name:     GenieName,
		X:        x, Y: p.Y, Z: z,
		Facing:  headingTo(p.X-x, p.Z-z),
		World:   p.World,
		ModelID: GenieModel,
		Size:    1,
		NPCID:   catalog,
		NPCType: 0x80, // the NPC bit alone: CLIENT talks to it
		Level:   max(p.Level, 1),
		HP:      100, MaxHP: 100,
		Temporary: true,
		GenieOf:   p.ServerID,
	}
	g.SpawnX, g.SpawnY, g.SpawnZ, g.SpawnFacing = g.X, g.Y, g.Z, g.Facing
	w.putNPCLocked(g)
	w.SpatialIndex.Insert(g.ServerID, g.X, g.Z)
	if w.genies == nil {
		w.genies = map[EntityID]EntityID{}
	}
	w.genies[g.ServerID] = p.ServerID
	p.genie = &genieState{npc: g.ServerID, rec: g, world: p.World, x: x, z: z, since: now}
	log.Printf("Genie: %s's genie %d at (%.1f, %.1f)", p.Name, g.ServerID, x, z)
	return g.ServerID, nil
}

// SetGenieCatalog makes the npcitem list the session's genie's shop sells
// catalog (handlers/genie.go: its capes', where it was summoned with its
// class's outfitter's): the genie's record's NPCID, which its window's
// requests are served from. False when the player has no genie.
func (w *World) SetGenieCatalog(sess *session.Session, catalog int) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.genie == nil || p.genie.rec == nil {
		return false
	}
	p.genie.rec.NPCID = catalog
	return true
}

// GenieOf reports whether npc is a genie and whether the session's player is
// the one it serves.
func (w *World) GenieOf(sess *session.Session, npc *NPC) (genie, mine bool) {
	if npc == nil || npc.GenieOf == 0 {
		return false, false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	return true, p != nil && npc.GenieOf == p.ServerID && p.genie != nil && p.genie.npc == npc.ServerID
}

// GenieNPC is the session's player's genie while it stands.
func (w *World) GenieNPC(sess *session.Session) (*NPC, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.genie == nil || p.genie.npc == 0 {
		return nil, false
	}
	return p.genie.rec, true
}

// EndGenieWindow ends the session's genie's window when it is of the kind
// (the forge's close, 0x0063: the one window CLIENT says it closed).
func (w *World) EndGenieWindow(sess *session.Session, window GenieWindow) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p := w.sessionPlayerLocked(sess); p != nil && p.genie != nil && p.genie.window == window {
		w.dropGenieLocked(p)
	}
}

// GenieOpened is the session's genie's box answered with a window: the genie
// leaves the world and the window is the player's (GenieWindowNPC), its
// requests carrying id (the shop's, the forge's; the bank's carry anything).
// False when the player has no genie.
func (w *World) GenieOpened(sess *session.Session, window GenieWindow, id uint32, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.genie == nil {
		return false
	}
	w.genieLeavesLocked(p)
	p.genie.window, p.genie.id, p.genie.used = window, id, now
	log.Printf("Genie: %s's genie opens window %d (%#x)", p.Name, window, id)
	return true
}

// DismissGenie sends the session's player's genie away, with its window.
func (w *World) DismissGenie(sess *session.Session) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p := w.sessionPlayerLocked(sess); p != nil {
		w.dropGenieLocked(p)
	}
}

// GenieWindowNPC is the genie's record when the session's request is one of
// the window its genie opened (a bank's request with any id, a shop's or a
// forge's with the id it was opened with), the player near where it stood,
// in its world and alive, the window used within GenieWindowIdle; the request
// counts as the window's use. A window the player can no longer have ends.
func (w *World) GenieWindowNPC(sess *session.Session, window GenieWindow, id uint32, now time.Time) (*NPC, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.genie == nil || p.genie.window == GenieNoWindow || p.genie.window != window {
		return nil, false
	}
	if window != GenieBank && id != p.genie.id {
		return nil, false
	}
	if w.genieGoneLocked(p, now) {
		w.dropGenieLocked(p)
		return nil, false
	}
	p.genie.used = now
	return p.genie.rec, true
}

// genieGoneLocked is whether the player's genie (or its window) is no longer
// its: dead, another world, away from where it stood, or idle past its time.
// Must be called with the lock held.
func (w *World) genieGoneLocked(p *Player, now time.Time) bool {
	g := p.genie
	if p.IsDead() || p.World != g.world || math.Hypot(float64(p.X-g.x), float64(p.Z-g.z)) > float64(GenieReach) {
		return true
	}
	if g.window == GenieNoWindow {
		return now.Sub(g.since) > GenieWait
	}
	return now.Sub(g.used) > GenieWindowIdle
}

// genieLeavesLocked takes the player's genie NPC out of the world (its
// window, if any, stays). Must be called with the lock held.
func (w *World) genieLeavesLocked(p *Player) {
	g := p.genie
	if g == nil || g.npc == 0 {
		return
	}
	if npc, ok := w.NPCs[g.npc]; ok && npc.GenieOf == p.ServerID {
		w.removeNPCLocked(npc)
	}
	delete(w.genies, g.npc)
	g.npc = 0
}

// dropGenieLocked ends the player's genie, its NPC and its window. Must be
// called with the lock held.
func (w *World) dropGenieLocked(p *Player) {
	if p.genie == nil {
		return
	}
	w.genieLeavesLocked(p)
	p.genie = nil
}

// genieSystem ends the genies and windows no longer their players' (dead,
// another world, away, idle), and takes away a genie whose player is gone
// (w.genies: the genies standing, by their owners). Every tick; must be
// called with the lock held.
func (w *World) genieSystem(now time.Time) {
	for id, owner := range w.genies {
		if p, ok := w.Players[owner]; !ok || p.genie == nil || p.genie.npc != id {
			if npc, ok := w.NPCs[id]; ok {
				w.removeNPCLocked(npc)
			}
			delete(w.genies, id)
		}
	}
	for _, p := range w.Players {
		if p.genie != nil && w.genieGoneLocked(p, now) {
			log.Printf("Genie: %s's genie goes", p.Name)
			w.dropGenieLocked(p)
		}
	}
}
