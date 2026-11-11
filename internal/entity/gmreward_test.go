package entity

import (
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Petition 1501 (the GM Jibrish): GM's Reward, the record tools/gm_reward_20261111.py writes and #reward gives
// (handlers/reward.go): "a 25 heal damage shield aka they heal for 25 whenever hit, and it deals 25 to who hits
// them. They also gain 100ac, 50 str, 50 dex, 50 wis, 50 int, 50 cha, 50 sta. Lasts 1 hour." Its effects are read
// from the tool itself, so the record and this test are one.

// gmRewardEffects is the tool's EFFECTS.
func gmRewardEffects(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../tools/gm_reward_20261111.py")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^EFFECTS = '(.*)'$`).FindSubmatch(b)
	if m == nil {
		t.Fatal("no EFFECTS line in the tool")
	}
	return string(m[1])
}

// gmReward is the record as SpellOfRow makes it from the tool's rows.
func gmReward(t *testing.T) CastSpell {
	sp := &database.SpellPatternRecord{SpellID: 905121, AbilityLvl: 60, Scope: 1, SpellName: "GM's Reward", Icon: -213673665}
	return SpellOfRow(sp, 0, "Target, Damage Shield 25, Heal 25 when hit, 1hr", gmRewardEffects(t), 497989046)
}

// Landed as a gift (LandOnSelf, as #reward lands it): one effect of an hour, +100 AC and +50 of the six stats, a
// shield of 25; a creature's hit on its bearer takes 25 off the creature and gives the bearer 25 back. Kept over a
// logout: saved and restored, the shield and the heal with it. The control: the bearer's stats without it.
func TestGMRewardIsTheGMsAsk(t *testing.T) {
	c, crab := casterAndCrab(t)
	stats := []Stat{StatStr, StatSta, StatDex, StatWis, StatInt, StatCha}
	spell := gmReward(t)
	if spell.Def.Duration != time.Hour || len(spell.Def.Unsupported) != 0 {
		t.Fatalf("the record reads %v long, unsupported %q", spell.Def.Duration, spell.Def.Unsupported)
	}
	if !c.w.LandOnSelf(c.sess, spell, c.at(0)) {
		t.Fatal("not landed")
	}
	if len(c.p.Effects) != 1 {
		t.Fatalf("%d effects, want 1", len(c.p.Effects))
	}
	ae := c.p.Effects[0]
	if ae.Shield == nil || ae.Shield.Min != 25 || ae.Shield.Max != 25 || ae.Rules.ProcHeal != 25 || !ae.Rules.ProcHealDef ||
		ae.Rules.ProcHealChance != 100 || !ae.Until.Equal(c.at(time.Hour)) {
		t.Fatalf("the effect: shield %+v, heal %d def %v at %d%%, until %v", ae.Shield, ae.Rules.ProcHeal, ae.Rules.ProcHealDef,
			ae.Rules.ProcHealChance, ae.Until.Sub(c.at(0)))
	}
	if ae.Mods.AC != 100 || ae.Mods.Str != 50 || ae.Mods.Sta != 50 || ae.Mods.Dex != 50 || ae.Mods.Wis != 50 || ae.Mods.Int != 50 ||
		ae.Mods.Cha != 50 || ae.Mods.Agi != 0 {
		t.Errorf("its mods: %+v", ae.Mods)
	}
	// what the client shows: each stat 50 more with it than without (AC 100), below their caps
	with, without := map[Stat]int{}, map[Stat]int{}
	mods := c.p.Mods
	for _, st := range append(stats, StatAC) {
		with[st] = c.p.StatNow(st)
	}
	c.p.Mods = sumEffectMods(nil)
	for _, st := range append(stats, StatAC) {
		without[st] = c.p.StatNow(st)
	}
	c.p.Mods = mods
	for _, st := range stats {
		if with[st]-without[st] != 50 {
			t.Errorf("stat %d: %d with it, %d without", st, with[st], without[st])
		}
	}
	if with[StatAC]-without[StatAC] != 100 {
		t.Errorf("AC: %d with it, %d without", with[StatAC], without[StatAC])
	}
	// a creature's hit: 25 back on it, 25 to the bearer
	c.p.HP = c.p.MaxHPNow() - 200
	c.sent()
	hp, crabHP := c.p.HP, crab.HP
	c.w.mu.Lock()
	c.w.npcAttackPlayer(crab, c.p.ServerID, c.at(time.Second))
	c.w.mu.Unlock()
	if crab.HP != crabHP-25 {
		t.Errorf("the crab after its hit: %d, want %d", crab.HP, crabHP-25)
	}
	hit := hp - c.p.HP + 25 // the crab's hit, the heal taken back out
	if hit < 0 || c.p.HP != hp-hit+25 {
		t.Errorf("the bearer: %d -> %d", hp, c.p.HP)
	}
	healed := false
	for _, m := range c.sent() {
		if sameMsgs([][]byte{m}, [][]byte{damageMsg(t, c.me, 25, c.me)}) {
			healed = true
		}
	}
	if !healed {
		t.Error("the bearer's heal of 25 was not told")
	}
	// kept over a logout
	rows, ok := c.w.SavedEffects(c.p.ServerID, c.at(10*time.Minute))
	if !ok || len(rows) != 1 || rows[0].SpellID != 905121 {
		t.Fatalf("saved: %+v %v", rows, ok)
	}
	c.p.Effects, c.p.Buffs = nil, nil
	if n := c.w.RestoreEffects(c.sess, rows, c.at(time.Hour)); n != 1 {
		t.Fatalf("restored %d", n)
	}
	ae = c.p.Effects[0]
	if ae.Shield == nil || ae.Shield.Min != 25 || ae.Rules.ProcHeal != 25 || ae.Mods.AC != 100 || !ae.Until.Equal(c.at(time.Hour+50*time.Minute)) {
		t.Errorf("restored: shield %+v, heal %d, AC %d, %v left", ae.Shield, ae.Rules.ProcHeal, ae.Mods.AC, ae.Until.Sub(c.at(time.Hour)))
	}
}
