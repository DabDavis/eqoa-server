package entity

import (
	"time"

	"github.com/eqoa/server/internal/session"
)

// These row and item IDs are reserved by the Heirloom Thanksgiving installer.
// A name alone must never give a Christmas actor, ordinary aviak or GM copy a raid mechanic.
const turkeyKingRow EntityID = 197601
const leftoverRow EntityID = 197607
const turkeySmasherPattern = 1000812

func (n *NPC) turkeyKing() bool {
	return n != nil && n.row() == turkeyKingRow && n.Name == "Turkey King"
}

func (n *NPC) turkeyHP(normal int) int {
	if n.turkeyKing() {
		return 2000000
	}
	return normal
}

func (n *NPC) leftover() bool {
	return n != nil && n.row() == leftoverRow && n.Name == "Leftovers" && n.summoner().turkeyKing()
}

// Resolve every crossed boundary at the hit, including exact multiples and a hit spanning several waves.
func (w *World) turkeyWavesLocked(n *NPC, damage int, now time.Time) {
	if !n.turkeyKing() || !n.Temporary || n.NPCType&0x80 != 0 || n.tmpl == nil || n.Dead {
		return
	}
	f := n.fight()
	for i := range n.tmpl.summons {
		s := &n.tmpl.summons[i]
		if i >= 32 || f.summoned&(1<<i) != 0 || (n.HP-damage)*100 > n.MaxHP*s.Below {
			continue
		}
		f.summoned |= 1 << i
		w.summonLocked(n, f, s, now)
	}
}

// Called with the actual last hitter, before group damage is used to assign XP and loot.
func (w *World) leftoverDeathLocked(n *NPC, s *session.Session, p *Player) {
	if !n.leftover() || !n.Dead || p == nil || p.IsDead() || n.add.burst {
		return
	}
	n.add.burst = true
	p.HP = max(0, p.HP-2000)
	p.StatsDirty = true
	p.LastCombatTime = time.Now()
	if s != nil {
		from := w.objectIDForEntityLocked(s, n.ServerID)
		w.sendDamageIndication(s, from, -2000, s.PlayerObjectID(), 0)
		w.sendColoredLocked(s, "The Leftovers burst for 2,000 damage!", duelOrange)
	}
	if p.HP == 0 {
		w.playerDiedLocked(s, p, p.LastCombatTime)
	}
}

// SceneStrike operates only on the two temporary entities selected from a single trigger run.
// Zero damage knocks the target down to one. A finishing hit plays an actual lootless death.
func (w *World) SceneStrike(attacker, target EntityID, damage int, finish bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	a, t := w.NPCs[attacker], w.NPCs[target]
	if a == nil || t == nil || a == t || !a.Temporary || !t.Temporary || a.Dead || t.Dead || a.World != t.World || a.NPCType&0x80 == 0 || t.NPCType&0x80 == 0 {
		return false
	}
	w.broadcastNPCSwingLocked(a, t.ServerID, 0, templateSpecialFX)
	now := time.Now()
	a.LastCombat, t.LastCombat = now, now
	if finish {
		t.NoLoot = true
		t.HP = 0
		w.killNPC(t)
		w.dropAggroLocked(t.ServerID)
	} else if damage == 0 {
		t.HP = 1
		t.sceneDown = true
		t.Threats = nil
		t.CurrentEnemy = 0
		t.EventAggro = false
		t.stopMoving()

	} else {
		t.HP = max(1, t.HP-damage)
	}
	return true
}

func (w *World) mysteryTravelLocked(sess *session.Session, p *Player) {
	if sess == nil || p == nil || p.IsDead() || p.respawnPending() || w.travelDueLocked(p) || sess.StateChannelsHeld() {
		return
	}
	w.gates.due = append(w.gates.due, GateTravel{Sess: sess, P: p, Spell: "Turkey Smasher", To: GateSpot{World: 0, X: 14607, Y: 54.188, Z: 19045}})
}
