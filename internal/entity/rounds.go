package entity

import (
	"time"

	"github.com/eqoa/server/internal/session"
)

// NPC melee in rounds, as Sandstorm's NPCs fight (recorded 2026-10-21 on
// training dummies and the beach creatures by Muniel's Tea Garden, and in
// pcaps/ps2_main_capture.pcap and ps2_sandstorm_cast_spells_and_autoattack.pcap;
// claude-notes/training-dummies-20261021.md):
//
//   - A round opens with the NPC's swing at its target, 0x00B1 with the
//     swing's delay (VISwing::Delay) as the cast time and VISwing::Effect's
//     id, sent to everyone who sees the NPC (Sandstorm showed us a guard's
//     swing at another player).
//   - Its hits come at fixed times into the round, each a 0x00DB to the
//     player hit alone.
//   - The swing is CLIENT's GetSwing for a warrior of the NPC's level
//     carrying what it carries: below Lv20 Attack1, one hit at 0.5 s in a
//     3.2 s round; Lv20-34 Attack2, hits at 0.5 and 1.5 s in 4 s; from Lv35
//     Attack4, hits at 0.5, 1, 1.7 and 2.4 s in 6 s; from Lv35 with a shield
//     Attack3, hits at 0.5 and 1 s in 4 s (a Lv49 guard with sword and
//     shield, the PS2 capture's).

// npcSwingKind is the swing an NPC fights with: GetSwing for a warrior of
// its level, a weapon in the primary and a shield as it carries them.
func npcSwingKind(npc *NPC) int {
	var worn [21]bool
	worn[14] = npc.PrimaryWeapon != 0
	worn[17] = npc.Shield != 0
	return GetSwing(0, npc.Level, worn)
}

// npcRound is the shape of the NPC's round: its length, the swing (the
// delay its 0x00B1 carries, its effect id) and the times of its hits. The
// length is the delay but for Attack1's: Sandstorm's said 3000 and came
// every 3.2 s, twelve in a row.
func npcRound(npc *NPC) (length, delay time.Duration, fx uint32, hits []time.Duration) {
	kind := npcSwingKind(npc)
	delay = SwingDelay(kind)
	length = delay
	if kind == 0 {
		length = 3200 * time.Millisecond
	}
	return length, delay, HashResourceID(SwingEffectName(kind)), swingHits(kind)
}

// swingHits is the times of a round's hits by swing kind, as Sandstorm's
// NPCs and players both land them (a player's measured from the first hit:
// a bard's Attack4 0, 0.5, 1.2, 1.9 s; an SK's Attack3 0, 0.5; an
// enchanter's Attack1 one): Attack1 one hit at 0.5 s; Attack2 at 0.5 and
// 1.5; Attack3 at 0.5 and 1; Attack4 at 0.5, 1, 1.7 and 2.4; a ranged swing
// one.
func swingHits(kind int) []time.Duration {
	ms := func(v ...int) []time.Duration {
		out := make([]time.Duration, len(v))
		for i, x := range v {
			out[i] = time.Duration(x) * time.Millisecond
		}
		return out
	}
	switch SwingEffectName(kind) {
	case "Attack2":
		return ms(500, 1500)
	case "Attack3":
		return ms(500, 1000)
	case "Attack4":
		return ms(500, 1000, 1700, 2400)
	}
	return ms(500)
}

// npcRoundLocked runs the NPC's round against its target: a new round's
// swing when the last is over, then the hits that are due, each through hit
// (which reports whether the fight ended with it). Must be called with the
// lock held.
func (w *World) npcRoundLocked(npc *NPC, target *Player, now time.Time, hit func() bool) {
	length, delay, fx, hits := npcRound(npc)
	if npc.RoundStart.IsZero() || !now.Before(npc.RoundStart.Add(length)) {
		// a round on from the last one's start, a steady cadence as
		// Sandstorm's; the first, or one after a pause, from now
		next := npc.RoundStart.Add(length)
		if npc.RoundStart.IsZero() || now.Sub(next) >= length {
			next = now
		}
		npc.RoundStart = next
		npc.RoundHits = 0
		w.broadcastNPCSwingLocked(npc, target.ServerID, uint32(delay/time.Millisecond), fx)
	}
	for npc.RoundHits < len(hits) && !now.Before(npc.RoundStart.Add(hits[npc.RoundHits])) {
		npc.RoundHits++
		if hit() {
			return
		}
	}
}

// endRound forgets the NPC's round: its next fight starts a new one.
func (n *NPC) endRound() {
	n.RoundStart = time.Time{}
	n.RoundHits = 0
}

// broadcastNPCSwingLocked sends an NPC's swing (0x00B1) to everyone who
// sees the NPC, each with the ids it knows the NPC and the target by.
func (w *World) broadcastNPCSwingLocked(npc *NPC, target EntityID, ms, fx uint32) {
	if w.SessionManager == nil {
		return
	}
	w.SessionManager.ForEach(func(s *session.Session) bool {
		if !s.CharacterInWorld() {
			return true
		}
		c := w.objectIDSeenByLocked(s, npc.ServerID)
		if c == 0 {
			return true
		}
		t := w.objectIDSeenByLocked(s, target)
		if t == 0 {
			t = c
		}
		w.sendCastingCombat(s, c, t, ms, fx)
		return true
	})
}
