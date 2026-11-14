package entity

import (
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// gmrewardxp.go — a GM's reward of experience (petition 5033, the GM Jibrish, on the Live shard: "Add a GM only
// ability /reward for LIVE ONLY NOT HEIRLOOM, this gives a 10% XP bonus for 30 minutes to the target").
//
// An effect on the effects channel, as the towns' boon is (boon.go): its own id, a name and the Divine Blessing
// spells' icon, that makes every experience award (a kill's, a quest's: awardXP) RewardXPPct of itself while it
// lasts, on top of a boon (a boon's 200 and the reward's 110 are 220), the award's line saying so. A second reward
// puts it back to its whole 30 minutes (it never stacks). Its time runs only in the world and it is kept over a
// logout and a keep restart, as every effect's (buffsave.go); a death, a strip and Metal Moogie's clear leave it, a
// GM's #clearbuffs ends it. When its time is over its bearer is told (effectSystem). The command and the shard's
// gate are handlers/rewardxp.go's; none of it is a record's.

// RewardXPEffectID keys it among a player's effects (the boon is -1003, the CTF's speed -1010).
const RewardXPEffectID = -1011

// RewardXPPct is the share of an award with it on: 10% more.
const RewardXPPct = 110

// RewardXPDuration is how long of play it lasts.
const RewardXPDuration = 30 * time.Minute

// RewardXPName is its name in the effects window.
const RewardXPName = "GM's Reward of Experience"

// RewardXPIcon is the Divine Blessing spells' icon (Divine Blessing, Holy Blessing, Marr's Blessing), apart from the
// boon's Blessing icon.
const RewardXPIcon uint32 = 0x828563AF

// RewardXPEndLine is its bearer's line when its time is over.
const RewardXPEndLine = "Your GM's reward has ended: your experience is no longer increased by 10%."

// rewardXPTag is what an award's line says of its share.
const rewardXPTag = "reward +10%"

// GiveRewardXP puts the reward on the session's player for its whole 30 minutes, in place of one on it (renewed, with
// what was left of it). False with no living player in the world.
func (w *World) GiveRewardXP(sess *session.Session, gm string, now time.Time) (name string, renewed bool, left time.Duration, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.IsDead() {
		return "", false, 0, false
	}
	for _, e := range p.Effects {
		if e.SpellID == RewardXPEffectID {
			renewed, left = true, max(0, e.Until.Sub(now))
		}
	}
	w.addEffectLocked(p, &ActiveEffect{SpellID: RewardXPEffectID, Name: RewardXPName, Icon: RewardXPIcon, Caster: p.ServerID,
		Until: now.Add(RewardXPDuration)}, now)
	if !renewed {
		w.broadcastCastLocked(p, p.ServerID, 0, fxBuff)
	}
	log.Printf("Reward: %s has %s from %s for %v (renewed %v, %v left)", p.Name, RewardXPName, gm, RewardXPDuration, renewed,
		left.Round(time.Second))
	return p.Name, renewed, left, true
}

// xpPctLocked is the share of an experience award a player gets, and its line's tag ("" for none): the boon's
// percent while one is on it, the GM's reward's 10% more while that is. Must be called with the lock held.
func (w *World) xpPctLocked(p *Player) (int, string) {
	pct := w.boonPctLocked(p)
	tag := ""
	if pct != 100 {
		tag = boonTag(pct)
	}
	if p.hasEffect(RewardXPEffectID) {
		pct = pct * RewardXPPct / 100
		if tag != "" {
			tag += ", "
		}
		tag += rewardXPTag
	}
	return pct, tag
}

// outlivesDeath reports an effect a death, a strip and a buffs' clear (but a GM's #clearbuffs) leave on: the towns'
// boon and the GM's reward.
func outlivesDeath(spellID int) bool {
	return spellID == BoonEffectID || spellID == RewardXPEffectID
}

// rewardXPEndedLocked tells p its reward's time is over. Must be called with the lock held.
func (w *World) rewardXPEndedLocked(p *Player) {
	if sess := w.sessionOfLocked(p); sess != nil {
		w.sendColoredLocked(sess, RewardXPEndLine, [3]byte{255, 255, 0})
	}
	log.Printf("Reward: %s's %s is over", p.Name, RewardXPName)
}

// RewardXPLeft is what is left of the reward on the session's player (0: none).
func (w *World) RewardXPLeft(sess *session.Session, now time.Time) time.Duration {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return 0
	}
	for _, e := range p.Effects {
		if e.SpellID == RewardXPEffectID {
			return max(0, e.Until.Sub(now))
		}
	}
	return 0
}

// RewardXPWords is a time left in words for the GM's lines: "30 minutes".
func RewardXPWords(d time.Duration) string { return spanWords(d) }
