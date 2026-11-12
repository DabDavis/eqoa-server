package entity

import (
	"fmt"
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// familiar.go — familiars: a follower an item calls, which neither fights nor
// is fought and does one thing of its own (the GM Jibrish's petition 1445,
// 2026-11-10: "On the genie heirloom vendor add a familiar pet people can buy.
// They can resummon the pet as much as they want, its a 2 second cast time on
// item use, however they can only have 1 familiar at a time. Familiars do not
// attack, and usually only sit there and do nothing even if attacked unless we
// program them otherwise. This particular one should share a model with the
// gnome tinker pet. He should be called "Healbot". He is immune to all
// resists, regens 2k health a tick. Does not generate agro or attack. All he
// does is when the player who summoned him falls below 75% health, he will
// cast an instant cast 600 hp heal. He will do this every 6 seconds until the
// player is above 75% health, depops when player dies or when dismissed. This
// pet will not heal the player if pvp flagged, or in a duel. Pet should have
// 100k health.").
//
// A familiar is data (table familiars, database/familiars.go: FamiliarDef):
// the item that calls it, its name, body and size, its hit points and what it
// regenerates each regen tick, the item's cast and visual, and its act: a heal
// of its owner of so much, below so much of its owner's hit points, at most so
// often (0: it does nothing). Healbot is the first row (tools/seed_familiars.py:
// the Invention's clockwork gnome, 0xFBABEB00, the gnome's tinker pet).
//
// The item's Use (handlers/familiar.go) is a cast through the World's own
// casting (CastSpell.Familiar, as the Rock of Freeport's travel): its cast
// time and visual to all who see its caster, no lockout and no 0x00CD (no
// ability of the book), landing castLand after its cast time with the
// familiar beside its caster, in place of one already out: one familiar a
// player, beside its pet and its merc. It follows its owner as a pet does and
// comes along through a teleport (never into capture the flag's arena: it
// waits outside, ctfaway.go), goes when its owner dies, logs out (not kept
// over a logout: the item calls it again) or dismisses it (/familiar dismiss),
// and is carried through a keep restart.
//
// It is never fought: an NPC with the NPC bit (npc flags 0x80, as the genie's,
// never owned) is no player's or pet's to attack, no guard's, no area spell's
// of a player or a creature (theirs reach players and pets alone), and nothing
// puts it on a creature's hate list (addThreatLocked refuses it: its heals add
// none, it never hits), so no creature takes it on and nothing harmful lands
// on it: that is the petition's "immune to all resists". It never attacks.
// Its heal is instant (no cast, no 0x00B1 time): its owner, below its
// HealBelow percent and in its reach, not flagged for PvP, in no duel and not
// in Mr. Moogie's challenge (ownfight.go), is
// healed its figure (at most what is missing), told as a heal from the
// familiar (0x00DB, a pet's figure: numbertags.go), and the next is HealEvery
// later at the soonest; at or above the percent it heals nothing.

// FamiliarDef is a familiar as table familiars has it.
type FamiliarDef struct {
	Item  int // the item record whose Use calls it
	Name  string
	Model uint32
	Size  float32
	HP    int
	// Regen is the hit points it regenerates each regen tick (RegenInterval),
	// in a fight or out of one
	Regen int
	// Cast is the item's cast and CastFX its visual (0: a buff's stand-in)
	Cast   time.Duration
	CastFX uint32
	// Heal is its heal on its owner (0: it heals nothing), HealBelow the
	// percent of its owner's hit points below which it heals, HealEvery the
	// least time between two heals and HealFX its visual (0: a heal's)
	Heal      int
	HealBelow int
	HealEvery time.Duration
	HealFX    uint32
}

// familiarState is one familiar, kept by its owner's id.
type familiarState struct {
	owner     *Player
	npc       *NPC
	def       FamiliarDef
	nextHeal  time.Time
	last      time.Time
	noCollide bool
}

// familiarCastID is a familiar item's cast's id: no spell's, no effect's (the
// Rock of Freeport's cast is -1005).
const familiarCastID = -1006

// familiarNoCollideID is a familiar's entry on its owner's non-collide list,
// after the pet's and the merc's.
const familiarNoCollideID uint32 = petNoCollideID + 2

// familiarReach is how far from its owner a familiar's heal reaches (its
// owner is petLost away at most before it is put beside it again).
const familiarReach float32 = 30

// FamiliarCast is the cast an item of a familiar's begins (its Use): its
// cast time and visual on its caster alone, the familiar called as it lands.
func FamiliarCast(d FamiliarDef) CastSpell {
	fx := d.CastFX
	if fx == 0 {
		fx = fxBuff
	}
	def := d
	return CastSpell{SpellID: familiarCastID, Name: d.Name, CastTime: d.Cast, FX: fx, Familiar: &def}
}

// familiarSpot is a place behind its owner's right, petFollow off it: its
// follow slot's (followslot.go; the pet takes the right, the merc behind the
// left).
func familiarSpot(p *Player) (x, y, z float32) {
	return followSpot(p, slotFamiliar)
}

// isFamiliarIDLocked reports a familiar's entity id (addThreatLocked's
// refusal). Must be called with the lock held.
func (w *World) isFamiliarIDLocked(id EntityID) bool {
	if len(w.familiars) == 0 {
		return false
	}
	return w.NPCs[id].isFamiliar()
}

// summonFamiliarLocked is a familiar item's cast landing: the familiar beside
// its caster, in place of any it has out. sess is the caster's, or nil. Must
// be called with the lock held.
func (w *World) summonFamiliarLocked(p *Player, sess *session.Session, d *FamiliarDef, now time.Time) {
	if p.IsDead() || p.respawnPending() {
		return
	}
	if w.ctfKeepsOutLocked(p) {
		w.ctfFamiliarWaitsLocked(p, sess, *d, 1) // not in the arena: it comes when its owner leaves (ctfaway.go)
		return
	}
	if old := w.familiars[p.ServerID]; old != nil {
		w.familiarGoesLocked(old, sess)
	}
	id := w.allocFreeIDLocked()
	n := &NPC{ServerID: id, Name: d.Name, World: p.World, Level: p.Level, ModelID: int64(d.Model), Size: NPCSize(d.Size),
		NPCType: 0x80, Faction: FactionFriendly, Temporary: true, RespawnTime: petCorpseTime, CorpseDecay: petCorpseTime,
		MaxHP: max(d.HP, 1), HP: max(d.HP, 1)}
	n.X, n.Y, n.Z = familiarSpot(p)
	n.Facing = headingTo(p.X-n.X, p.Z-n.Z)
	n.SpawnX, n.SpawnY, n.SpawnZ, n.SpawnFacing = n.X, n.Y, n.Z, n.Facing
	fs := &familiarState{owner: p, npc: n, def: *d, last: now}
	n.familiar = fs
	w.putNPCLocked(n)
	w.SpatialIndex.Insert(n.ServerID, n.X, n.Z)
	if w.familiars == nil {
		w.familiars = make(map[EntityID]*familiarState)
	}
	w.familiars[p.ServerID] = fs
	w.familiarTellLocked(fs, sess)
	log.Printf("Familiar: %s calls %q (body %#x, size %.2f): %d HP, %d a tick, heals %d below %d%% every %v",
		p.Name, n.Name, d.Model, n.Size, n.MaxHP, d.Regen, d.Heal, d.HealBelow, d.HealEvery)
}

// RestoreFamiliar calls the session's player's familiar back with no cast (a
// keep restart carrying it: keepmerc.go), its hit points' share kept. It
// reports false with no player in the world.
func (w *World) RestoreFamiliar(sess *session.Session, d FamiliarDef, hpShare float64, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.IsDead() || p.respawnPending() {
		return false
	}
	if w.ctfKeepsOutLocked(p) {
		w.ctfFamiliarWaitsLocked(p, sess, d, hpShare) // it waits outside the arena (ctfaway.go)
		return false
	}
	w.summonFamiliarLocked(p, nil, &d, now)
	fs := w.familiars[p.ServerID]
	if fs == nil {
		return false
	}
	fs.npc.HP = keptShare(hpShare, fs.npc.MaxHP)
	return true
}

// familiarGoesLocked takes the familiar out of the world at once, off its
// owner's non-collide list (sess, the owner's, or nil). Must be called with
// the lock held.
func (w *World) familiarGoesLocked(fs *familiarState, sess *session.Session) {
	n := fs.npc
	if w.familiars[fs.owner.ServerID] == fs {
		delete(w.familiars, fs.owner.ServerID)
	}
	if sess != nil && fs.noCollide {
		sendNoCollideRemove(sess, familiarNoCollideID)
	}
	if w.NPCs[n.ServerID] == n {
		w.removeNPCLocked(n)
	}
	n.Dead = true
	log.Printf("Familiar: %s's %q goes", fs.owner.Name, n.Name)
}

// familiarTellLocked puts the familiar on its owner's non-collide list once,
// as a pet's (0x1003).
func (w *World) familiarTellLocked(fs *familiarState, sess *session.Session) {
	if sess == nil || fs.noCollide {
		return
	}
	sendNoCollideAdd(sess, fs.npc.Name, familiarNoCollideID)
	fs.noCollide = true
}

// DismissFamiliar sends the session's player's familiar away. It reports its
// name, false for none out.
func (w *World) DismissFamiliar(sess *session.Session) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	fs := w.familiars[p.idOrZero()]
	if p == nil {
		return "", false
	}
	if fs == nil || fs.owner != p {
		return w.ctfDismissAwayLocked(p, false) // one waiting outside the arena (ctfaway.go)
	}
	name := fs.npc.Name
	w.familiarGoesLocked(fs, sess)
	return name, true
}

// FamiliarOf is the session's player's familiar's name and item, false for
// none out.
func (w *World) FamiliarOf(sess *session.Session) (name string, item int, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	fs := w.familiars[p.idOrZero()]
	if p == nil || fs == nil || fs.owner != p {
		return "", 0, false
	}
	return fs.npc.Name, fs.def.Item, true
}

// familiarLeavingLocked is the owner leaving the world: its familiar goes
// (not kept over a logout). Must be called with the lock held.
func (w *World) familiarLeavingLocked(p *Player) {
	if fs := w.familiars[p.ServerID]; fs != nil && fs.owner == p {
		w.familiarGoesLocked(fs, nil)
	}
}

// familiarSystem moves every familiar with its owner, regenerates it and does
// its act; a familiar whose owner is gone or dead goes. Every tick; must be
// called with the lock held.
func (w *World) familiarSystem(now time.Time) {
	if len(w.familiars) == 0 {
		return
	}
	regen := w.TickCount%uint64(RegenInterval/(100*time.Millisecond)) == 0
	for id, fs := range w.familiars {
		owner, n := fs.owner, fs.npc
		if w.Players[id] != owner || w.NPCs[n.ServerID] != n {
			w.familiarGoesLocked(fs, nil) // its owner logged out or in again, or a GM took it
			continue
		}
		sess := w.findSessionByEntityIDLocked(id)
		if owner.IsDead() || owner.respawnPending() || n.Dead {
			w.familiarGoesLocked(fs, sess) // the petition's "depops when player dies"
			continue
		}
		if w.ctfKeepsOutLocked(owner) {
			w.ctfSendAwayLocked(owner, now) // its owner in the arena: it waits outside (ctfaway.go)
			continue
		}
		dt := float32(now.Sub(fs.last).Seconds())
		fs.last = now
		dt = min(max(dt, 0), 0.5)
		if regen && fs.def.Regen > 0 && n.HP < n.MaxHP {
			n.HP = min(n.HP+fs.def.Regen, n.MaxHP)
		}
		if n.World != owner.World || distXZ(n.X, n.Z, owner.X, owner.Z) > petLost {
			oldX, oldZ := n.X, n.Z
			n.World = owner.World
			n.X, n.Y, n.Z = familiarSpot(owner)
			n.stopMoving()
			w.SpatialIndex.Update(n.ServerID, oldX, oldZ, n.X, n.Z)
		} else {
			w.followLocked(n, owner, slotFamiliar, dt, now)
		}
		n.Level = owner.Level
		w.familiarActLocked(fs, sess, now)
		w.familiarTellLocked(fs, sess)
	}
}

// familiarActLocked is the familiar's heal of its owner, when one is due (the
// file's doc). Must be called with the lock held.
func (w *World) familiarActLocked(fs *familiarState, sess *session.Session, now time.Time) {
	d, o, n := &fs.def, fs.owner, fs.npc
	if d.Heal <= 0 || now.Before(fs.nextHeal) {
		return
	}
	top := o.MaxHPNow()
	if top <= 0 || o.HP <= 0 || o.HP*100 >= top*d.HealBelow {
		return
	}
	if w.ownFightLocked(o) {
		return // no help in a fight that is its owner's alone (ownfight.go)
	}
	if n.World != o.World || dist3(n.X, n.Y, n.Z, o.X, o.Y, o.Z) > familiarReach {
		return
	}
	heal := min(d.Heal, top-o.HP)
	o.HP += heal
	o.StatsDirty = true
	fs.nextHeal = now.Add(d.HealEvery)
	fx := d.HealFX
	if fx == 0 {
		fx = fxHeal
	}
	w.broadcastNPCCastLocked(n, o.ServerID, 0, fx)
	if sess != nil {
		if fid := w.objectIDForEntityLocked(sess, n.ServerID); fid != 0 {
			w.sendDamageIndication(sess, fid, heal, sess.PlayerObjectID(), numberHeal.byPet())
		}
	}
	log.Printf("Familiar: %s's %q heals it for %d (HP %d/%d)", o.Name, n.Name, heal, o.HP, top)
}

// FamiliarLine is a familiar's figures for a line (the handlers' /familiar).
func FamiliarLine(d FamiliarDef) string {
	if d.Heal <= 0 {
		return fmt.Sprintf("%s follows you and does nothing else.", d.Name)
	}
	return fmt.Sprintf("%s heals you %d whenever you are below %d%% of your hit points, every %v at most.", d.Name, d.Heal, d.HealBelow,
		d.HealEvery.Round(time.Second))
}
