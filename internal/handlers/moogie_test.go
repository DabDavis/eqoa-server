package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
)

// Mr. Moogie (petition 1246, Dwarfrish; the user: build it): his box offers to clear buffs or heal;
// clearing ends every effect but the town's Boon and says whose buffs are clear, healing brings hit
// points and power to their most. Flagged for PvP, he will not help (the control: the same talk
// unflagged asks).
func TestMrMoogieClearsBuffsAndHeals(t *testing.T) {
	r := newPortalRig(t)
	moogie := &entity.NPC{ServerID: 7777, Name: entity.MoogieName, X: 5001, Y: 50, Z: 5000, Level: 50, HP: 1000, MaxHP: 1000, NPCType: 0x80}
	r.gw.NPCs[moogie.ServerID] = moogie
	ward := entity.CastSpell{SpellID: 9201, Name: "Test Ward", Scope: 0, Def: entity.ParseSpellNamed("Test Ward", "Self, Increase AC by 10, 5min", "")}
	r.gw.LandOnSelf(r.anna, ward, r.now)
	r.gw.GiveBoon(r.anna, "Freeport", r.now)
	if names := r.gw.PlayerEffects(r.anna); len(names) != 2 {
		t.Fatalf("effects before: %+v", names)
	}
	ask := func() uint32 {
		t.Helper()
		r.drain()
		r.talk(r.anna, moogie)
		got := r.msgs(r.anna)
		if len(got) != 1 || !strings.HasPrefix(got[0], "ask#") || !strings.Contains(got[0], "|Clear my buffs|Fully heal me|") {
			t.Fatalf("his talk: %q", got)
		}
		return boxID(got)
	}
	r.pick(r.anna, ask(), 0)
	if got := r.msgs(r.anna); len(got) != 1 || got[0] != "chat:Mr. Moogie says, 'Anna's buffs are clear.'" {
		t.Errorf("cleared: %q", got)
	}
	if left := r.gw.PlayerEffects(r.anna); len(left) != 1 {
		t.Errorf("effects after clearing: %+v, want the Boon alone", left)
	}
	r.pa.HP, r.pa.Power = 10, 0
	r.pick(r.anna, ask(), 1)
	if r.pa.HP != r.pa.MaxHPNow() || r.pa.Power != r.pa.MaxPowerNow() {
		t.Errorf("healed: HP %d/%d, power %d/%d", r.pa.HP, r.pa.MaxHPNow(), r.pa.Power, r.pa.MaxPowerNow())
	}
	// fighting him: refused, no box
	moogie.Threats = map[entity.EntityID]int64{r.pa.ServerID: 10}
	r.drain()
	r.talk(r.anna, moogie)
	if got := r.msgs(r.anna); len(got) != 1 || got[0] != "colour:"+entity.MoogieLineFoe {
		t.Errorf("fighting him: %q", got)
	}
	moogie.Threats = nil
	// flagged for PvP: refused, no box
	r.gw.SetPvP(true)
	r.gw.PvPCommand(r.anna, r.now)
	r.drain()
	r.talk(r.anna, moogie)
	if got := r.msgs(r.anna); len(got) != 1 || got[0] != "colour:"+entity.MoogieLinePvP {
		t.Errorf("flagged: %q", got)
	}
	_ = time.Second
}

// Mr. Moogie's question fits a box's (questAskChars: a 0x0034's question is cut
// at 127 characters; the user, 2026-11-08: "Mr. Moogie's text is cut off").
func TestMrMoogiesQuestionFitsItsBox(t *testing.T) {
	for _, q := range []string{moogieQuestion, moogieTributeQuestion} {
		if n := len([]rune(q)); n > questAskChars {
			t.Errorf("his question %q is %d characters, a box shows %d", q, n, questAskChars)
		}
	}
	for _, o := range []string{moogieClearOpt, moogieHealOpt, moogieResetOpt, moogieNoOpt, moogieFavorOpt, moogieV2Opt} {
		if n := len([]rune(o)); n > questAnswerChars {
			t.Errorf("answer %q is %d characters, a box shows %d", o, n, questAnswerChars)
		}
	}
}
