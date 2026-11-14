package handlers

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// rewardXPOn reports the GM's reward of experience on p.
func rewardXPOnPlayer(p *entity.Player) int {
	n := 0
	for _, e := range p.Effects {
		if e.SpellID == entity.RewardXPEffectID {
			n++
		}
	}
	return n
}

// Petition 5033 (the GM Jibrish): on a server with world.gm_reward_xp (the Live shard) a GM's #reward on its target,
// a player, gives the reward of experience: the player told who gave it, the GM told, a gm_audit line; a second one
// renews it (one on, the GM told it does not stack); /reward <name> is the same. A creature targeted is refused. The
// controls: a player's #reward is an unknown command; without world.gm_reward_xp (Heirloom) #reward is GM's Reward
// and gives no reward of experience.
func TestRewardIsTheRewardOfExperienceOnLive(t *testing.T) {
	c, gs, gp, ps, pp := gmWorld(t)
	store := newFakeRewards(t)
	c.h.SetRewardStore(store, store)
	SetRewardXP(true)
	t.Cleanup(func() { SetRewardXP(false) })

	// a player: unknown
	gp.TargetObjectID = c.gw.IDSeenBy(ps, gp)
	c.say(ps, "#reward")
	if got := heard(t, ps); len(got) != 1 || got[0].text != `Unrecognized command "#reward".` {
		t.Fatalf("a player's #reward: %+v", got)
	}
	ps.Queue().GetPending()
	if rewardXPOnPlayer(gp) != 0 || len(store.audits) != 0 {
		t.Fatal("a player's #reward gave it")
	}

	// a creature
	npc := &entity.NPC{ServerID: 7002, Name: "Desert Crab", X: 25001, Y: 50, Z: 15001, Level: 10, HP: 100, MaxHP: 100}
	c.gw.NPCs[npc.ServerID] = npc
	vs := entity.GetOrCreateVisibility(gs)
	vs.ChannelEntity[0], vs.ChannelObjectID[0] = npc.ServerID, 100002
	gp.TargetObjectID = 100002
	c.say(gs, "#reward")
	if got := texts(heard(t, gs)); !slices.Contains(got, "[GM] #reward: Desert Crab is not a player. A reward of experience is for players.") {
		t.Fatalf("a creature: %q", got)
	}
	gs.Queue().GetPending()

	// the player targeted
	gp.TargetObjectID = c.gw.IDSeenBy(gs, pp)
	c.say(gs, "#reward")
	if rewardXPOnPlayer(pp) != 1 {
		t.Fatalf("not on the player: %q", texts(heard(t, gs)))
	}
	if ok, _ := rewarded(pp); ok {
		t.Error("GM's Reward was given as well")
	}
	if got := texts(heard(t, gs)); !slices.Contains(got, "[GM] Player has 10% more experience for 30 minutes.") {
		t.Errorf("the GM's line: %q", got)
	}
	if got := texts(heard(t, ps)); !slices.Contains(got, fmt.Sprintf("%s rewards you: 10%% more experience for 30 minutes.", gp.Name)) {
		t.Errorf("the player's lines: %q", got)
	}
	if len(store.audits) != 1 || !strings.Contains(store.audits[0], "110% experience for 30m0s on Player") {
		t.Errorf("audit lines: %q", store.audits)
	}

	// again, by name with /reward: renewed, one on
	gs.Queue().GetPending()
	ps.Queue().GetPending()
	c.say(gs, "/reward Player")
	if rewardXPOnPlayer(pp) != 1 || len(store.audits) != 2 || !strings.Contains(store.audits[1], "renewed") {
		t.Fatalf("again: %d on, audits %q", rewardXPOnPlayer(pp), store.audits)
	}
	if got := texts(heard(t, gs)); len(got) != 1 || !strings.HasPrefix(got[0], "[GM] Player's reward of experience is back to 30 minutes (") {
		t.Errorf("the GM's renewal line: %q", got)
	}

	// Heirloom: GM's Reward, no reward of experience
	SetRewardXP(false)
	pp.Effects, pp.Buffs = nil, nil
	gs.Queue().GetPending()
	c.say(gs, "#reward Player")
	if ok, _ := rewarded(pp); !ok || rewardXPOnPlayer(pp) != 0 {
		t.Errorf("without world.gm_reward_xp: GM's Reward %v, reward of experience %d", ok, rewardXPOnPlayer(pp))
	}
}
