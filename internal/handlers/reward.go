package handlers

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// reward.go — #reward and /reward, a GM's buff on a player (petition 1501, the GM Jibrish: "create a GM only
// command /reward — this puts a buff on the target that gives them a 25 heal damage shield aka they heal for 25
// whenever hit, and it deals 25 to who hits them. They also gain 100ac, 50 str, 50 dex, 50 wis, 50 int, 50 cha, 50
// sta. Lasts 1 hour.").
//
// It is a GM's command (gmCommands: the # spelling, and the / spelling from a client that forwards a /command it
// does not know, as vanilla does) on the GM's target, a player, or on the player named after it ("#reward Bob"); a
// creature or no target is refused with a line. What it gives is the spell record RewardSpellID, "GM's Reward"
// (tools/gm_reward_20261111.py writes it; no class has it): its workbook effects a damage shield of 25 and a heal of
// 25 at every creature's melee hit on its bearer (dshield.go's shield and workbook.go's defensive proc heal, the
// workbook's defensive procs at every hit), +100 AC and +50 STR, STA, DEX, WIS, INT and CHA, for an hour of play,
// on a stack type of its own (its id: it takes no other buff off and none takes it off). It lands as a gift on its
// bearer (entity.World.LandOnSelf, as a board's gift lands): its icon and name in the bearer's effects, kept over a
// logout as every buff is (buffsave.go), a second reward renewing it at its whole hour. The record is read at each
// use, checked by its name (a record renamed or missing is said and nothing given). Each reward is a "live" line in
// gm_audit (database.AuditGMLive), as the panel's kicks and messages are; the GM and the player are both told.

// RewardSpellID is GM's Reward's record (tools/gm_reward_20261111.py writes it, beside Buffer's GM Armor 905120).
const RewardSpellID = 905121

// RewardSpellName is its record's name.
const RewardSpellName = "GM's Reward"

// The lines (ours).
const (
	rewardUsage    = "Usage: #reward (your target, a player) or #reward <player>: GM's Reward for an hour"
	rewardNotNPC   = "#reward: %s is not a player. GM's Reward is for players."
	rewardNoTarget = "#reward: no target. Target a player, or #reward <player>."
	rewardNoRecord = "#reward: GM's Reward's record (spell %d) is not on this server: %s"
	rewardRefused  = "#reward: %s cannot take it now (dead, or not in the world)."
	rewardGMLine   = "GM's Reward is on %s for an hour: a damage shield of 25 and a heal of 25 at every hit taken, +100 AC, +50 STR, STA, DEX, WIS, INT and CHA."
	rewardYouLine  = "A GM has rewarded you: GM's Reward is on you for an hour of play. You heal 25 and deal 25 back at every hit you take, and gain 100 AC and 50 STR, STA, DEX, WIS, INT and CHA."
)

// rewardStore is where #reward reads its record and writes its audit line.
type rewardStore interface {
	SpellPattern(spellID int) (*database.SpellPatternRecord, error)
	SpellEffectRow(spellID int) (database.SpellEffectRow, bool, error)
}

// rewardAuditor writes #reward's gm_audit line (database.SQLiteDB).
type rewardAuditor interface {
	AuditGMLive(who database.GMEdit, detail string) (int64, error)
}

// SetRewardStore gives #reward its spell record and its audit log. Without the store nothing is given.
func (h *ChatHandler) SetRewardStore(s rewardStore, a rewardAuditor) { h.rewards, h.rewardAudit = s, a }

// rewardSpell is GM's Reward as the World lands it, or why it cannot be.
func (h *ChatHandler) rewardSpell() (entity.CastSpell, string) {
	if h.rewards == nil {
		return entity.CastSpell{}, "this server keeps no spells"
	}
	sp, err := h.rewards.SpellPattern(RewardSpellID)
	if err != nil || sp == nil {
		return entity.CastSpell{}, fmt.Sprintf("no record (%v)", err)
	}
	if !strings.EqualFold(strings.TrimSpace(sp.SpellName), RewardSpellName) {
		return entity.CastSpell{}, fmt.Sprintf("it is %q", sp.SpellName)
	}
	row, found, err := h.rewards.SpellEffectRow(RewardSpellID)
	if err != nil || !found || strings.TrimSpace(row.Effects) == "" {
		return entity.CastSpell{}, fmt.Sprintf("it has no effects (%v)", err)
	}
	return entity.SpellOfRow(sp, 0, row.Line, row.Effects, row.FX), ""
}

// rewardCommand is #reward [player].
func (h *ChatHandler) rewardCommand(sess *session.Session, gm *entity.Player, args []string) {
	if h.gameWorld == nil {
		return
	}
	var t *entity.Player
	var ts *session.Session
	switch {
	case len(args) > 0 && strings.EqualFold(args[0], "help"):
		h.gmReply(sess, rewardUsage)
		return
	case len(args) > 0:
		var ok bool
		if t, ts, ok = h.gmNamed(sess, strings.Join(args, " "), "#reward"); !ok {
			return
		}
	default:
		id := gm.TargetObjectID
		if id == 0 || id == 0xFFFFFFFF {
			h.gmReply(sess, rewardNoTarget)
			return
		}
		ent, found := h.gameWorld.FindEntityByObjectID(sess, id)
		if !found {
			h.gmReply(sess, rewardNoTarget)
			return
		}
		p, isPlayer := ent.(*entity.Player)
		if !isPlayer {
			name := "your target"
			if n, ok := ent.(*entity.NPC); ok {
				name = n.Name
			}
			h.gmReply(sess, fmt.Sprintf(rewardNotNPC, name))
			return
		}
		t, ts = p, h.gameWorld.FindSessionByEntityID(p.ServerID)
		if ts == nil {
			h.gmReply(sess, fmt.Sprintf("%s has no session.", p.Name))
			return
		}
	}
	spell, why := h.rewardSpell()
	if why != "" {
		h.gmReply(sess, fmt.Sprintf(rewardNoRecord, RewardSpellID, why))
		return
	}
	if !h.gameWorld.LandOnSelf(ts, spell, time.Now()) {
		h.gmReply(sess, fmt.Sprintf(rewardRefused, t.Name))
		return
	}
	detail := fmt.Sprintf("#reward in the game: %s (spell %d) on %s (character %d) for an hour", RewardSpellName, RewardSpellID,
		t.Name, t.CharServerID)
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
	h.gmReply(sess, fmt.Sprintf(rewardGMLine, t.Name))
	SendColoredChat(ts, rewardYouLine, 255, 255, 0)
}
