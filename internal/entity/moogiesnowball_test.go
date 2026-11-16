package entity

import (
	"testing"
	"time"
)

// A snowball worn as a thrown weapon (moogieitems.go; petition 2204): an attack press at a creature with it is
// refused in red and nothing is engaged; a round of a fight at a creature it is somehow in (the hands changed
// mid-fight) lets the fight go, nothing thrown; a duel's or a PvP fight's round at the foe is a snowball's hit
// (1 off, never below 1), no fight harm, one off the worn stack, and the last lets the fight go.
func TestWornSnowballPlayersOnly(t *testing.T) {
	r := newMetalRigWith(t, func(exec func(q string, args ...any)) {
		exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race)
			VALUES (702, 'a snow rat', 0, 1005, 10, 1000, 0, 1213913426, 1.0, 10, 0, '0')`)
	})
	a, sa := r.player("Anna", 1000, 1000)
	b, sb := r.player("Bert", 1010, 1000)
	pe := NewPotionEffect(9901, "Snowball", "Target, Snowball 0x15C66F59", "", 0, "", 0, 0)
	r.w.NotePotionEffect(pe)
	var worn [21]bool
	worn[18] = true
	swing := SwingFor(a.Class, a.Level, worn, true, 10, int(SnowballRange))
	swing.Potion, swing.PotionLeft = 9901, 2

	rat := r.w.NPCs[702]
	if rat == nil {
		t.Fatal("no rat")
	}
	drain(sa)
	if r.w.EngageAttack(sa, r.see(sa, rat.ServerID), swing, r.at(0)) {
		t.Fatal("a worn snowball engaged a creature")
	}
	if a.AttackTarget != 0 || !hasLine(linesOf(drain(sa)), SnowballRefusal) {
		t.Errorf("engaged %d; want nothing and the refusal said", a.AttackTarget)
	}

	// a round at a creature with one worn: let go, nothing thrown, the rat untouched
	hp := rat.HP
	r.w.mu.Lock()
	a.Swing = swing
	a.snowballToldAt = time.Time{}
	over := r.w.thrownPotionHitLocked(sa, a, rat, r.see(sa, rat.ServerID))
	r.w.mu.Unlock()
	if !over || rat.HP != hp || a.Swing.PotionLeft != 2 {
		t.Errorf("a round at the rat: over %v, rat %d -> %d, %d left; want over, untouched, 2", over, hp, rat.HP, a.Swing.PotionLeft)
	}

	// a duel's round at Bert: the snowball's hit, one off the stack; the last lets go
	r.see(sb, a.ServerID)
	drain(sb)
	bhp := b.HP
	r.w.mu.Lock()
	a.Swing = swing
	over = r.w.thrownPotionAtFoeLocked(a, b, r.at(time.Second))
	left := a.Swing.PotionLeft
	r.w.mu.Unlock()
	if over || b.HP != bhp-1 || left != 1 {
		t.Errorf("a round at Bert: over %v, HP %d -> %d, %d left; want on, 1 off, 1", over, bhp, b.HP, left)
	}
	if !hasLine(linesOf(drain(sb)), "Anna hits you with a snowball!") {
		t.Error("Bert was not told")
	}
	b.HP = 1
	r.w.mu.Lock()
	over = r.w.thrownPotionAtFoeLocked(a, b, r.at(2*time.Second))
	r.w.mu.Unlock()
	if !over || b.HP != 1 || b.IsDead() || a.Swing.Potion != 0 {
		t.Errorf("the last at Bert on 1 HP: over %v, HP %d, worn %d; want over, 1, none", over, b.HP, a.Swing.Potion)
	}
	if a.PvPFlagged() || b.PvPFlagged() {
		t.Error("a snowball flagged someone")
	}
}
