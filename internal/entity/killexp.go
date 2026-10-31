package entity

import (
	"log"

	"github.com/eqoa/server/internal/session"
)

// killexp.go — the experience a kill gives, as the live game's server gave it.
//
// CLIENT carries the live server's experience model compiled in and never
// called (decomp modules/client/src/viexperience.cpp, 0x00CC6A70 ..
// 0x00CC6E88: GroupExp, LevelExp, LevelDiffScale, GroupScale, KillExp) beside
// ExpForLevel, the level table its experience bar divides by. Sandstorm gives
// exactly it: a level 2 warrior's solo kill of a level 2 Orc Pawn moved its
// 0x0020 from 6 to 380 experience into the level, 374 = LevelExp(2)
// (claude-notes/quests-sandstorm-20261025.md). Until F3 20261026-031118 this
// server gave a creature's level x 100 (200 for that pawn, 6,300 for a level
// 63 creature where the model gives a level 60 425,820), so a mastery point
// (125,000 experience and more, mastery.go) was out of reach.
//
// The model's floats were the server's (an x86 machine, not the EE), so they
// are Go's float32 here, operation for operation.

// ExpForLevel is the experience level needs to be reached from the level
// below (CLIENT 0x00CC68D8): what the HUD's bar divides the experience into
// the level by, level + 1's for the current level.
func ExpForLevel(level int) int {
	l := uint32(level)
	if level >= 51 {
		return int(l * l * l * ((l-1)*27 + 20))
	}
	return int(l * l * l * ((l-1)*28 + 25))
}

// levelExp is what a creature of a level is worth (LevelExp, 0x00CC6BF0).
func levelExp(level int) float32 {
	base := float32(float32(level+1)*float32(level+1)) * 40
	return float32(float32(level)*0.02*base) + base // conversions: no fused multiply-add
}

// levelDiffScale is 5% more or less for each level a creature is above or
// below the group's highest (LevelDiffScale, 0x00CC6C38).
func levelDiffScale(mob, top int) float32 {
	return float32(float32(mob-top)*0.05) + 1
}

// groupScale is a group's kill worth: 0.8 a member and 0.2 (GroupScale,
// 0x00CC6CE8).
func groupScale(n int) float32 {
	return float32(float32(n)*0.8) + 0.2
}

// maxLevel is the highest of levels (MaxLevel, 0x00CC6C68).
func maxLevel(levels []int) int {
	top := levels[0]
	for _, l := range levels[1:] {
		if top < l {
			top = l
		}
	}
	return top
}

// killExp is a kill's whole worth to a group of the levels given (KillExp,
// 0x00CC6D18): nothing for a creature below three quarters of the highest
// level (within 2 of it, three levels under), else the creature's worth by
// its level against the highest and by the group's size.
func killExp(mob int, levels []int) float32 {
	top := maxLevel(levels)
	r := int(float64(float32(top)) * 0.75) // (int)((float)max * 0.75): a double
	limit := r
	if !(top-r > 2) {
		limit = top - 3
	}
	if mob < limit {
		return 0
	}
	return levelExp(mob) * levelDiffScale(mob, top) * groupScale(len(levels))
}

// groupExp is one member's share of a kill (GroupExp, 0x00CC6A70): none for a
// member below three quarters of the group's highest level (six under, when
// that is nearer), else the kill's worth by the member's level squared over
// the group's, at most three times what the member's own level would give it
// alone.
func groupExp(mob, level int, levels []int) int {
	if len(levels) == 0 {
		return 0
	}
	top := maxLevel(levels)
	limit := int(float32(top) * 0.75)
	if top-limit <= 5 {
		limit = top - 6
	}
	if level < limit {
		return 0
	}
	exp := killExp(mob, levels)
	if exp == 0 {
		return 0
	}
	lsq := float32(level * level)
	var sum float32
	for _, l := range levels {
		sum += float32(l * l)
	}
	share := lsq / sum
	limitExp := killExp(level, []int{level}) * 3
	if limitExp <= 0 {
		return 0
	}
	e := max(int(exp*share), 0)
	return min(int(limitExp), e)
}

// killSharersLocked is who shares a player's kill of npc: the player and its
// group's members in npc's world within KillShareRange of it, alive (the
// quest kills' rule, questhooks.go). Must be called with the lock held.
func (w *World) killSharersLocked(killer *Player, npc *NPC) []*Player {
	out := []*Player{killer}
	for _, m := range w.groupMembersLocked(killer) {
		if m == killer || m.World != npc.World || m.IsDead() {
			continue
		}
		dx, dz := m.X-npc.X, m.Z-npc.Z
		if dx*dx+dz*dz > KillShareRange*KillShareRange {
			continue
		}
		out = append(out, m)
	}
	return out
}

// awardKillXPLocked gives a player's kill of npc its experience: each sharer
// its GroupExp, through awardXP (the masteries' share, the lines, 0x0020).
// killerSess is the killer's session when the caller has it. Must be called
// with the lock held.
func (w *World) awardKillXPLocked(killerSess *session.Session, killer *Player, npc *NPC) {
	if killer == nil || npc == nil {
		return
	}
	if guardsKillLocked(npc) {
		log.Printf("Guards: %s's kill of %s is the guards': no experience", killer.Name, npc.Name)
		return
	}
	sharers := w.killSharersLocked(killer, npc)
	levels := make([]int, len(sharers))
	for i, m := range sharers {
		levels[i] = m.Level
	}
	for _, m := range sharers {
		xp := groupExp(npc.Level, m.Level, levels)
		if xp <= 0 {
			continue
		}
		sess := killerSess
		if m != killer || sess == nil {
			sess = w.sessionOfLocked(m)
		}
		if sess == nil {
			continue
		}
		w.awardXP(sess, m, xp)
	}
}
