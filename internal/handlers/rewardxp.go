package handlers

import (
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// rewardxp.go — #reward and /reward on the Live shard: the reward of experience (petition 5033, the GM Jibrish: "Add
// a GM only ability /reward for LIVE ONLY NOT HEIRLOOM, this gives a 10% XP bonus for 30 minutes to the target").
//
// A server with world.gm_reward_xp (the Live shard's config alone; the two shards run one binary) answers a GM's
// #reward with this in place of GM's Reward (reward.go, petition 1501), which a server without it keeps. It is a GM's
// command (gmCommands, as #reward always was) on the GM's target, a player, or on the player named after it
// ("#reward Bob"); a creature or no target is refused with a line. What it gives is entity.GiveRewardXP's effect: 10%
// more of every experience award for 30 minutes of play, kept over a logout and a restart, a second reward putting it
// back to its whole 30 minutes (gmrewardxp.go). The player is told who rewarded it, the GM what was given, and each
// reward is a "live" line in gm_audit (database.AuditGMLive), as GM's Reward's is.

// rewardXPOn is world.gm_reward_xp.
var rewardXPOn atomic.Bool

// SetRewardXP makes #reward the reward of experience (true: the Live shard) or GM's Reward (false).
func SetRewardXP(on bool) { rewardXPOn.Store(on) }

// RewardXPOn is whether #reward is the reward of experience here.
func RewardXPOn() bool { return rewardXPOn.Load() }

// The lines (ours).
const (
	rewardXPUsage     = "Usage: #reward (your target, a player) or #reward <player>: 10% more experience for 30 minutes"
	rewardXPNotPlayer = "#reward: %s is not a player. A reward of experience is for players."
	rewardXPRefused   = "#reward: %s cannot take it now (dead, or not in the world)."
	rewardXPGMLine    = "%s has 10%% more experience for 30 minutes."
	rewardXPGMRenewed = "%s's reward of experience is back to 30 minutes (%s were left): it does not stack."
	rewardXPYouLine   = "%s rewards you: 10%% more experience for 30 minutes."
)

// rewardXPCommand is #reward [player] on a server with world.gm_reward_xp.
func (h *ChatHandler) rewardXPCommand(sess *session.Session, gm *entity.Player, args []string) {
	if h.gameWorld == nil {
		return
	}
	if len(args) > 0 && strings.EqualFold(args[0], "help") {
		h.gmReply(sess, rewardXPUsage)
		return
	}
	t, ts, ok := h.rewardTarget(sess, gm, args, "#reward", rewardNoTarget, rewardXPNotPlayer)
	if !ok {
		return
	}
	name, renewed, left, ok := h.gameWorld.GiveRewardXP(ts, gm.Name, time.Now())
	if !ok {
		h.gmReply(sess, fmt.Sprintf(rewardXPRefused, t.Name))
		return
	}
	detail := fmt.Sprintf("#reward in the game: %d%% experience for %v on %s (character %d)", entity.RewardXPPct,
		entity.RewardXPDuration, name, t.CharServerID)
	if renewed {
		detail += fmt.Sprintf(", renewed (%v were left)", left.Round(time.Second))
	}
	if h.rewardAudit != nil {
		who := database.GMEdit{AccountID: sess.AccountID(), Via: "game"}
		if a := sess.RemoteAddr(); a != nil {
			who.IP = a.IP.String()
		}
		if _, err := h.rewardAudit.AuditGMLive(who, detail); err != nil {
			log.Printf("GM: #reward's audit line not written: %v", err)
		}
	}
	log.Printf("GM: account %d (%s): %s", sess.AccountID(), gm.Name, detail)
	if renewed {
		h.gmReply(sess, fmt.Sprintf(rewardXPGMRenewed, name, entity.RewardXPWords(left)))
	} else {
		h.gmReply(sess, fmt.Sprintf(rewardXPGMLine, name))
	}
	SendColoredChat(ts, fmt.Sprintf(rewardXPYouLine, gm.Name), 255, 255, 0)
}
