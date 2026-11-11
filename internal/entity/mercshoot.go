package entity

import (
	"log"
	"time"
)

// mercshoot.go — a tank's or a melee fighter's bow (the owner, 2026-11-11:
// "lets make melee/tank mercs have a bow also and shoot when not within range
// so they can pull mobs").
//
// Its kit carries a bow (MercKit.Bow, the handlers': its class's heirloom bow
// if it has one, the ranger's, else the ranger's plain Heirloom Longbow, at
// its level's figures). While its target is out of its melee reach (mercReach
// and mercBowFrom more) and within its bow's (the bow's range, the target's
// allowance: a player's TargetInRange, attack.go's inReach) and in its sight
// (sight.go), it takes the bow in hand as a player's ranged switch leaves the
// hands (its bow alone, in the ranged slot: equip.go's equipDisplaces) and
// shoots a player's ranged swing of its class and level (GetSwing with slot 18
// worn: Range1 below 20, Range2 to 34, Range4 from 35), its round's delay
// between two shots (SwingDelay), its arrow landing a swing's hit time after
// the draw (swingHits: 0.5 s), each shot a player's (attack.go's
// playerShotHit: rangedShare times the ranged roll on the bow, DEX and WIS,
// two arrows at Range4) through the balance as a player's melee
// (meleeDealtLocked). Its 0x00B1 is the Range effect, which looses the second
// hand's arrow at the target (npcshoot.go).
//
// Between two shots it closes in as before (mercfight.go's mercMeleeLocked),
// but on a pull, its target coming at it (fighting it and on the move), it
// holds where it stands and lets the creature come. In its melee reach (its
// reach and mercBowTo more), and with no target, its melee hands are back.
// While the bow is in hand its arts that want one (a ranger's arrows:
// "Range") may be cast, and those that want a melee weapon may not, as a
// player's (equipFits).

// MercBow is the bow a merc's kit carries (the file's doc): its model and
// attack type for its channel's second hand, its DMG and range as a worn
// bow's (itempattern.weapondamage, unk5). Damage 0: none.
type MercBow struct {
	Model  uint32
	Type   byte
	Damage int
	Range  int
	Name   string
}

// The distances past its melee reach at which a merc takes up its bow and
// puts it away again (apart, so a creature at the edge of its reach does not
// switch its hands every tick).
const (
	mercBowFrom float32 = 3
	mercBowTo   float32 = 1
)

// mercShot is a shot on its way: its target and when its arrow lands.
type mercShot struct {
	target EntityID
	lands  time.Time
}

// mercShoots is whether the merc fights with a bow at range: a tank or a
// melee fighter whose kit carries one.
func (ms *mercState) mercShoots() bool {
	return (ms.role == roleTank || ms.role == roleMelee) && ms.kit.Bow.Damage > 0
}

// mercBowSwing is the merc's swing with its bow alone in hand (the file's
// doc).
func (ms *mercState) mercBowSwing() Swing {
	var worn [21]bool
	worn[18] = true
	b := &ms.kit.Bow
	return SwingFor(ms.kit.Class, ms.body.Level, worn, true, int(b.Type), b.Range)
}

// mercHandsLocked puts the bow or the melee hands in the merc's hands for t,
// its target (nil none), by the distance (the file's doc). Must be called
// with the lock held.
func (w *World) mercHandsLocked(ms *mercState, t *NPC) {
	if !ms.mercShoots() {
		return
	}
	m := ms.npc
	switch {
	case t == nil:
		w.mercBowLocked(ms, false)
	case !ms.bowOut && distXZ(m.X, m.Z, t.X, t.Z) > mercReach(t)+mercBowFrom:
		w.mercBowLocked(ms, true)
	case ms.bowOut && distXZ(m.X, m.Z, t.X, t.Z) <= mercReach(t)+mercBowTo:
		w.mercBowLocked(ms, false)
	}
}

// mercBowLocked takes the bow in hand (out) or the melee hands back: the
// NPC's hands on its channel and the body's for its arts' required
// equipment. Must be called with the lock held.
func (w *World) mercBowLocked(ms *mercState, out bool) {
	if ms.bowOut == out {
		return
	}
	ms.bowOut = out
	m, b, l := ms.npc, ms.body, &ms.kit.Look
	m.endRound()
	if out {
		bow := &ms.kit.Bow
		m.PrimaryWeapon, m.SecondaryWeapon, m.Shield = 0, int(bow.Model), 0
		m.PrimaryType, m.SecondaryType = 0, bow.Type
		b.PrimaryType, b.SecondaryType, b.Shield = 0, bow.Type, 0
		return
	}
	m.PrimaryWeapon, m.SecondaryWeapon, m.Shield = int(l.Primary), int(l.Secondary), int(l.Shield)
	m.PrimaryType, m.SecondaryType = l.PrimaryType, l.SecondaryType
	b.PrimaryType, b.SecondaryType, b.Shield = l.PrimaryType, l.SecondaryType, 0
	if ms.kit.Worn[17] {
		b.Shield = max(l.Shield, 1)
	}
	ms.shot = nil // an arrow not yet loosed is not
}

// mercBowReaches is whether the merc's bow reaches t from where it stands
// and it sees t (the file's doc). Must be called with the lock held.
func (w *World) mercBowReaches(ms *mercState, t *NPC) bool {
	m := ms.npc
	r := ms.mercBowSwing().Reach + npcReachExtra(t)
	if dist3(m.X, m.Y, m.Z, t.X, t.Y, t.Z) > r {
		return false
	}
	return w.inSightLocked(m.World, m.X, m.Y, m.Z, t.X, t.Y, t.Z)
}

// mercShootLocked is the merc's turn at a target out of its melee reach with
// its bow in hand: a shot when its round allows one, standing for its draw;
// it reports the merc standing (drawing, or holding for a pull), false when
// it closes in. Must be called with the lock held.
func (w *World) mercShootLocked(ms *mercState, t *NPC, now time.Time) bool {
	if !ms.bowOut {
		return false
	}
	m := ms.npc
	if ms.shot != nil {
		m.stopMoving()
		m.Facing = headingTo(t.X-m.X, t.Z-m.Z)
		return true // its draw
	}
	if !w.mercBowReaches(ms, t) {
		return false
	}
	if !now.Before(ms.shotReady) {
		s := ms.mercBowSwing()
		m.stopMoving()
		m.Facing = headingTo(t.X-m.X, t.Z-m.Z)
		ms.shotReady = now.Add(s.Delay)
		ms.shot = &mercShot{target: t.ServerID, lands: now.Add(swingHits(s.Kind)[0])}
		breakNPCInvis(m)
		w.broadcastNPCSwingLocked(m, t.ServerID, uint32(s.Delay/time.Millisecond), HashResourceID(SwingEffectName(s.Kind)))
		return true
	}
	// a pull: what it shot comes at it, and it lets it come
	if t.CurrentEnemy == m.ServerID && t.Loco != LocoStand {
		m.stopMoving()
		m.Facing = headingTo(t.X-m.X, t.Z-m.Z)
		return true
	}
	return false
}

// mercShotLandLocked lands the merc's arrow whose time has come: on its
// target still there and one it may fight, in its bow's reach. It reports a
// kill. Must be called with the lock held.
func (w *World) mercShotLandLocked(ms *mercState, now time.Time) bool {
	c := ms.shot
	if c == nil || now.Before(c.lands) {
		return false
	}
	ms.shot = nil
	m := ms.npc
	t := w.NPCs[c.target]
	if t == nil || t.Dead || t.World != m.World || !mercMayFight(m, t) {
		return false
	}
	if r := ms.mercBowSwing().Reach + npcReachExtra(t); dist3(m.X, m.Y, m.Z, t.X, t.Y, t.Z) > r {
		return false // shot at a target that went out of its reach
	}
	b := ms.body
	saveGear, saveSwing := b.Gear, b.Swing
	bow := &ms.kit.Bow
	b.Gear.WeaponDamage, b.Gear.WeaponCond, b.Gear.WeaponTwoHanded = bow.Damage, 1, false
	b.Gear.WeaponSlot, b.Gear.WeaponType, b.Gear.BareHands = 18, int(bow.Type), false
	b.Swing = ms.mercBowSwing()
	n := w.meleeDealtLocked(t, b, playerShotHit(b, t, meleeIntn), meleeIntn) // attack.go, balance.go
	b.Gear, b.Swing = saveGear, saveSwing
	log.Printf("Merc: %s's %q shoots %s", ms.owner.Name, m.Name, t.Name)
	return w.mercDamageLocked(ms, t, n, numberMelee, now)
}
