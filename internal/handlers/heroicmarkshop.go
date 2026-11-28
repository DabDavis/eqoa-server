package handlers

import (
	"errors"
	"fmt"
	"log"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

func heroicRewardName(reward database.HeroicReward, set *database.HeroicMarkSet, role *database.HeroicRole) string {
	switch reward {
	case database.HeroicUnlock:
		return "Unlock " + set.Mastery.Name
	case database.HeroicRelic:
		return role.RelicName
	case database.HeroicTeleport:
		return set.Teleport.Name
	}
	return ""
}

func (h *InteractHandler) heroicMarkShop(sess *session.Session, p *entity.Player, npc entity.EntityID, boss string) {
	set := database.HeroicMarksFor(boss)
	if !h.gameWorld.HeroicMarksEnabled() || h.heroicMarkInventory == nil || set == nil || p.Reliving() {
		return
	}
	role := set.RoleFor(p.Class)
	inv := sessionInvList(sess)
	if role == nil || inv == nil {
		return
	}
	prompt := fmt.Sprintf("You carry %d of %s. Which reward would you like?", inv.held(role.MarkID), role.MarkName)
	asked := heroicRaidsAsked{npc: npc, boss: boss}
	var labels []string
	for _, reward := range []database.HeroicReward{database.HeroicUnlock, database.HeroicRelic, database.HeroicTeleport} {
		if reward == database.HeroicUnlock && set.Mastery.ID == 0 || reward == database.HeroicTeleport && set.Teleport.ID == 0 {
			continue
		}
		asked.rewards = append(asked.rewards, reward)
		cost := "1 mark"
		if reward.MarkCost() != 1 {
			cost = fmt.Sprintf("%d marks", reward.MarkCost())
		}
		labels = append(labels, fmt.Sprintf("%s (%s)", heroicRewardName(reward, set, role), cost))
	}
	askHeroicRaids(sess, p, asked, prompt, append(labels, "Leave"))
}

func (h *InteractHandler) answerHeroicMarkShop(sess *session.Session, p *entity.Player, asked heroicRaidsAsked, option byte) {
	set := database.HeroicMarksFor(asked.boss)
	if !h.gameWorld.HeroicMarksEnabled() || h.heroicMarkInventory == nil || set == nil || p.Reliving() {
		return
	}
	role := set.RoleFor(p.Class)
	if role == nil {
		return
	}
	if asked.confirm != 0 {
		switch option {
		case 0:
			h.redeemHeroicMarks(sess, p, set, role, asked.confirm)
		case 1:
			h.heroicMarkShop(sess, p, asked.npc, asked.boss)
		}
		return
	}
	if int(option) >= len(asked.rewards) {
		return
	}
	reward := asked.rewards[option]
	prompt := fmt.Sprintf("Spend %d of %s on this reward?", reward.MarkCost(), role.MarkName)
	switch reward {
	case database.HeroicUnlock:
		prompt += fmt.Sprintf(" %s costs %d mastery points and requires rank I.", set.Mastery.Name, database.HeroicCMCost)
	case database.HeroicRelic:
		prompt += " " + role.RelicDescription + " One relic from this boss per character."
	case database.HeroicTeleport:
		prompt += fmt.Sprintf(" %s has %d charges and a %d-second cast.", set.Teleport.Name, database.HeroicTeleportCharges, database.HeroicTeleportSeconds)
	}
	askHeroicRaids(sess, p, heroicRaidsAsked{npc: asked.npc, boss: asked.boss, confirm: reward}, prompt,
		[]string{"Spend marks", "Back"})
}

func (h *InteractHandler) redeemHeroicMarks(sess *session.Session, p *entity.Player, set *database.HeroicMarkSet, role *database.HeroicRole, reward database.HeroicReward) {
	inv := sessionInvList(sess)
	if inv == nil {
		return
	}
	if reward == database.HeroicRelic && inv.loreHeld(role.RelicID) {
		SendErrorMessage(sess, fmt.Sprintf(loreLine, role.RelicName))
		return
	}
	done, err := h.heroicMarkInventory.RedeemHeroicMarks(p.CharServerID, p.Class, inv.roomBound(), set.Boss, reward)
	switch {
	case errors.Is(err, database.ErrHeroicShort):
		SendErrorMessage(sess, fmt.Sprintf("You need %d of %s in your pack.", reward.MarkCost(), role.MarkName))
		return
	case errors.Is(err, database.ErrHeroicClaimed):
		SendErrorMessage(sess, "This character already redeemed that reward.")
		return
	case errors.Is(err, database.ErrHeroicPackFull):
		SendErrorMessage(sess, "Make room in your pack for the reward.")
		return
	case err != nil:
		log.Printf("Heroic marks: redeeming %s's reward: %v", p.Name, err)
		SendErrorMessage(sess, "The exchange could not be saved.")
		return
	}
	drift := false
	for _, take := range done.Taken {
		at := inv.indexOfItem(take.ItemID)
		if inv.took(at, take.Amount) {
			queueReliable(sess, protocol.OpcodeRemoveInvItem, removeInvItemPayload(at, 0, take.Amount))
		} else {
			drift = true
		}
	}
	if done.Item.ItemID != 0 && !drift {
		rec := wireRecord(done.Item)
		if key, qty := inv.add(rec.ItemID, rec.Pattern.PatternID, 1, rec.Pattern.MaxStack); qty == 1 {
			queueReliable(sess, protocol.OpcodeAddInvItem, addInvItemPayload(&rec, key, readsCapes(sess)))
		} else {
			drift = true
		}
	}
	if drift {
		resyncItems(sess, p.Name, "heroic mark redemption")
	}
	if reward == database.HeroicUnlock {
		SendClientMessage(sess, fmt.Sprintf("%s is unlocked. Reopen the Class Mastery window to view it.", set.Mastery.Name))
	} else {
		SendClientMessage(sess, fmt.Sprintf("You receive %s.", done.Item.Pattern.ItemName))
	}
}
