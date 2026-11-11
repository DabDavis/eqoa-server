package handlers

import (
	"database/sql"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// fakeRewards is GM's Reward's record as tools/gm_reward_20261111.py writes it (its effects read from the tool), and
// the audit lines written.
type fakeRewards struct {
	name    string
	effects string
	audits  []string
}

func (f *fakeRewards) SpellPattern(id int) (*database.SpellPatternRecord, error) {
	if id != RewardSpellID || f.name == "" {
		return nil, sql.ErrNoRows
	}
	return &database.SpellPatternRecord{SpellID: id, AbilityLvl: 60, Scope: 1, SpellName: f.name, Icon: -213673665}, nil
}

func (f *fakeRewards) SpellEffectRow(id int) (database.SpellEffectRow, bool, error) {
	if id != RewardSpellID {
		return database.SpellEffectRow{}, false, nil
	}
	return database.SpellEffectRow{Line: "Target, Damage Shield 25, Heal 25 when hit", Effects: f.effects, FX: 497989046}, true, nil
}

func (f *fakeRewards) AuditGMLive(_ database.GMEdit, detail string) (int64, error) {
	f.audits = append(f.audits, detail)
	return int64(len(f.audits)), nil
}

func newFakeRewards(t *testing.T) *fakeRewards {
	t.Helper()
	b, err := os.ReadFile("../../tools/gm_reward_20261111.py")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^EFFECTS = '(.*)'$`).FindSubmatch(b)
	if m == nil {
		t.Fatal("no EFFECTS in the tool")
	}
	return &fakeRewards{name: RewardSpellName, effects: string(m[1])}
}

// rewarded reports GM's Reward on p, and its shield's figure.
func rewarded(p *entity.Player) (bool, int) {
	for _, e := range p.Effects {
		if e.SpellID == RewardSpellID && e.Shield != nil {
			return true, e.Shield.Min
		}
	}
	return false, 0
}

// Petition 1501 (the GM Jibrish): #reward is a GM's; on the GM's target, a player, it lands GM's Reward (its shield
// of 25), writes a gm_audit line and tells the GM and the player; /reward is the same. A creature targeted, no
// target, and a record renamed are refused with a line, nothing landed. The control: a player's #reward is an
// unknown command.
func TestRewardIsAGMsBuffOnAPlayer(t *testing.T) {
	c, gs, gp, ps, pp := gmWorld(t)
	store := newFakeRewards(t)
	c.h.SetRewardStore(store, store)

	gp.TargetObjectID = c.gw.IDSeenBy(ps, gp)
	c.say(ps, "#reward")
	if got := heard(t, ps); len(got) != 1 || got[0].text != `Unrecognized command "#reward".` {
		t.Fatalf("a player's #reward: %+v", got)
	}
	ps.Queue().GetPending()
	if ok, _ := rewarded(gp); ok || len(store.audits) != 0 {
		t.Fatal("a player's #reward gave it")
	}

	// no target
	gp.TargetObjectID = 0xFFFFFFFF
	c.say(gs, "#reward")
	if got := texts(heard(t, gs)); !slices.Contains(got, "[GM] "+rewardNoTarget) {
		t.Fatalf("no target: %q", got)
	}
	gs.Queue().GetPending()

	// a creature
	npc := &entity.NPC{ServerID: 7001, Name: "Desert Crab", X: 25001, Y: 50, Z: 15001, Level: 10, HP: 100, MaxHP: 100}
	c.gw.NPCs[npc.ServerID] = npc
	vs := entity.GetOrCreateVisibility(gs)
	vs.ChannelEntity[0], vs.ChannelObjectID[0] = npc.ServerID, 100001
	gp.TargetObjectID = 100001
	c.say(gs, "#reward")
	if got := texts(heard(t, gs)); !slices.Contains(got, "[GM] #reward: Desert Crab is not a player. GM's Reward is for players.") {
		t.Fatalf("a creature: %q", got)
	}
	gs.Queue().GetPending()

	// the player targeted
	gp.TargetObjectID = c.gw.IDSeenBy(gs, pp)
	c.say(gs, "#reward")
	if ok, ds := rewarded(pp); !ok || ds != 25 {
		t.Fatalf("not on the player (%v, shield %d): %q", ok, ds, texts(heard(t, gs)))
	}
	if got := texts(heard(t, gs)); !slices.Contains(got, "[GM] GM's Reward is on Player for an hour: a damage shield of 25 and a heal of 25 at every hit taken, +100 AC, +50 STR, STA, DEX, WIS, INT and CHA.") {
		t.Errorf("the GM's line: %q", got)
	}
	if got := texts(heard(t, ps)); !slices.Contains(got, rewardYouLine) {
		t.Errorf("the player's lines: %q", got)
	}
	if len(store.audits) != 1 || !strings.Contains(store.audits[0], "on Player") {
		t.Errorf("audit lines: %q", store.audits)
	}
	if ok, _ := rewarded(gp); ok {
		t.Error("the GM took it too")
	}

	// /reward <name>: the GM itself
	gs.Queue().GetPending()
	c.say(gs, "/reward Gm")
	if ok, _ := rewarded(gp); !ok || len(store.audits) != 2 {
		t.Fatalf("/reward Gm: on the GM %v, %d audit lines: %q", ok, len(store.audits), texts(heard(t, gs)))
	}

	// a record renamed: refused, nothing given
	store.name = "Something Else"
	pp.Effects = nil
	gs.Queue().GetPending()
	c.say(gs, "#reward Player")
	if ok, _ := rewarded(pp); ok {
		t.Error("a renamed record was given")
	}
	if got := texts(heard(t, gs)); len(got) != 1 || !strings.HasPrefix(got[0], "[GM] #reward: GM's Reward's record (spell 905121) is not on this server") {
		t.Errorf("a renamed record: %q", got)
	}
}
