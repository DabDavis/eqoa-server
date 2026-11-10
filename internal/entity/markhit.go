package entity

import (
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// markhit.go — the cleric's damage marks and brands: a curse on a creature
// that answers the hits landed on it (jib's PR #1, the GM Jibrish's ruling of
// 2026-10-29: "cleric marks and brands were reverse damage shields or heals.
// Eg; put it on a mob, if you attack the mob the effect happens (heals you) or
// the mob takes damage."). The heal mark, Mark of Faith, is markheal.go's.
//
// The workbook writes the ten damage marks (Mark and Brand of Woe, Pain,
// Agony, Anguish and Sorrow: 5 to 160) as a harmful spell at an enemy whose one
// effect is an offensive proc, "Add Offensive Proc: Decrease Hitpoints by N
// when it hits" (spelljson.go readProcsJSON: EffectProc, resisted by AR). Cast
// at a creature it lands on the creature (landOnNPCLocked: the workbook's AR
// resist, a hate of 1), and until 2026-11-10 that was all: no code ever rolls
// a creature's own procs, so every mark did nothing for its minute. On today's
// spells such an effect is these ten alone.
//
// The rules (ours where it says so, the PR's as far as main's code lets them be):
//   - a mark is an effect on a free creature that carries an offensive proc
//     and came of a harmful spell (Rules.Harm), for its time; it is kept by
//     nothing, and goes at the creature's death;
//   - each weapon hit a player (a melee swing or a bow's shot: playerHitLocked)
//     or a pet (petHitLocked) lands on the marked creature and leaves it
//     standing is answered after the hit's own procs: the mark's figure more,
//     as the hitter's hit (spellHitLocked, petDamageLocked: the 0x00DB told as a
//     proc's, the creature's hate for the hitter, the kill and its XP the
//     hitter's), no resist of its own (the mark's landing was resisted) and no
//     0x00B1 (ours, as the PR's); a training dummy is told and loses nothing
//     (dummy.go). A spell's damage, a proc's, a tick's or a shield's answer
//     sets off no mark, as none sets off Mark of Faith (markheal.go): the PR
//     had a pet's taunt answered too;
//   - only the strongest mark on a creature answers, by its top (the PR's, as
//     only the strongest damage shield answers: dshield.go);
//   - its figure is its proc's as lasting() made it (cast.go: the workbook's
//     own figure, a scale or level share of its caster's when it has one). The
//     PR's stand-in for a line naming no figure (half the spell's level, a
//     brand's twice that) is not needed: every live mark names its figure.

// damageMarkAt is the strongest damage mark on n at now, nil for none.
func (n *NPC) damageMarkAt(now time.Time) *ActiveEffect {
	if n.Owner != 0 {
		return nil
	}
	var best *ActiveEffect
	for _, e := range n.Effects {
		if e.Proc <= 0 || !e.Rules.Harm || !now.Before(e.Until) {
			continue
		}
		if best == nil || max(e.ProcMax, e.Proc) > max(best.ProcMax, best.Proc) {
			best = e
		}
	}
	return best
}

// markRoll is one answer's figure: a roll in the mark's range, at most its
// cap.
func markRoll(ae *ActiveEffect, intn func(int) int) int {
	n := ae.Proc
	if ae.ProcMax > ae.Proc {
		n += intn(ae.ProcMax - ae.Proc + 1)
	}
	return capAmount(n, ae.ProcCap)
}

// markHitLocked answers p's weapon hit on npc, still standing, with npc's
// damage mark: its figure more, as p's hit. It reports a kill. Must be called
// with the lock held.
func (w *World) markHitLocked(sess *session.Session, p *Player, npc *NPC, now time.Time) bool {
	if npc.Dead || npc.HP <= 0 || p.IsDead() {
		return false
	}
	ae := npc.damageMarkAt(now)
	if ae == nil {
		return false
	}
	n := markRoll(ae, procIntn)
	if n <= 0 {
		return false
	}
	log.Printf("Mark: %s's hit on %s under %q does %d more", p.Name, npc.Name, ae.Name, n)
	return w.spellHitLocked(p, sess, npc, n, numberProc)
}

// petMarkHitLocked answers a pet's hit on t, still standing, with t's damage
// mark: its figure more, as the pet's hit, its owner's. It reports a kill.
// Must be called with the lock held.
func (w *World) petMarkHitLocked(ps *petState, t *NPC, now time.Time) bool {
	if t.Dead || t.HP <= 0 || ps.npc.Dead {
		return false
	}
	ae := t.damageMarkAt(now)
	if ae == nil {
		return false
	}
	n := markRoll(ae, procIntn)
	if n <= 0 {
		return false
	}
	log.Printf("Mark: the pet %q's hit on %s under %q does %d more", ps.npc.Name, t.Name, ae.Name, n)
	return w.petDamageLocked(ps, t, n, now, numberProc)
}
