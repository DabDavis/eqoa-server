package entity

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

func thanksgivingReward(t *testing.T, id int) PotionEffect {
	t.Helper()
	var plan struct {
		Rewards []struct {
			ID   int `json:"patternid"`
			Name string
			Line string
		}
	}
	data, err := os.ReadFile("../../tools/thanksgiving/mark-rewards.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &plan); err != nil {
		t.Fatal(err)
	}
	for _, reward := range plan.Rewards {
		if reward.ID == id {
			return NewPotionEffect(id, reward.Name, reward.Line, reward.Name, 0, "", 0, 0)
		}
	}
	t.Fatalf("missing Thanksgiving reward %d", id)
	return PotionEffect{}
}

func TestThanksgivingProtectionStacksAndRefreshes(t *testing.T) {
	c, crab := casterAndCrab(t)
	c.p.Level, c.p.Cha = 60, 200
	worn, ok := WornShieldOf(1, "Breastplate of the Anointed")
	if !ok {
		t.Fatal("missing breastplate effect")
	}
	c.p.Gear.Shields = []WornShield{worn}
	spirit := SpellOfRow(&database.SpellPatternRecord{SpellID: 701228, AbilityLvl: 60, Scope: 1, SpellName: "Spirit Shield"}, 0,
		"Target, Add Defensive Proc: Increase Hitpoints by 52 to Self when hit, 30min",
		`{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"duration_s":1800,"stack":1,"level":60,"min_level":36,"req_equipment":"Any","sheet_type":"Defensive Proc","arch_type":"Damage Shield","effects":[{"id":"0716","type":"Defensive Proc","kind":"def_proc","procs":[{"id":"0D71","type":"Dmg/Heal","kind":"hp","amount":52,"on":"defense","scope":"self","target_type":"Self"}]}]}`, 0)
	if !c.w.LandOnSelf(c.sess, spirit, c.at(0)) {
		t.Fatal("Spirit Shield did not land")
	}
	pe := thanksgivingReward(t, drunkUncleProtection)
	if !pe.Def.NoRecharge || pe.Def.Duration != 15*time.Minute || len(pe.Def.Unread) != 0 {
		t.Fatalf("potion definition: %+v", pe.Def)
	}
	for _, offset := range []time.Duration{0, time.Second} {
		if why, err := c.w.UsePotion(c.sess, pe, 0, c.at(offset)); why != "" || err != nil {
			t.Fatalf("using protection: %q, %v", why, err)
		}
	}
	shield := CastSpell{SpellID: 390000, Name: "Ordinary Shield", Def: ParseSpellText("Self, Damage Shield 90, 30min")}
	if !c.w.LandOnSelf(c.sess, shield, c.at(time.Second)) || len(c.p.Effects) != 3 {
		t.Fatalf("expected the potion, Spirit Shield and ordinary shield, got %+v", c.p.Effects)
	}
	rows, _ := c.w.SavedEffects(c.p.ServerID, c.at(2*time.Second))
	c.p.Effects, c.p.Buffs = nil, nil
	if n := c.w.RestoreEffects(c.sess, rows, c.at(2*time.Second)); n != 3 {
		t.Fatalf("restored %d effects, want 3", n)
	}

	old := procIntn
	t.Cleanup(func() { procIntn = old })
	procIntn = dice(0)
	baseHeal := 52 + 15 + c.p.StatNow(StatCha)/10
	bonus := 25 + c.p.StatNow(StatCha)/10
	for _, hit := range []struct {
		at     time.Duration
		potion int
	}{
		{2 * time.Second, bonus},
		{15*time.Minute + 2*time.Second, 0},
	} {
		c.p.HP = c.p.MaxHPNow() - 200
		hp, crabHP := c.p.HP, crab.HP
		c.w.mu.Lock()
		c.w.playerShieldsLocked(c.sess, c.p, crab, c.at(hit.at))
		c.w.defenceProcsLocked(c.sess, c.p, crab, c.at(hit.at))
		c.w.mu.Unlock()
		if healed, damage := c.p.HP-hp, crabHP-crab.HP; healed != baseHeal+hit.potion || damage != 90+hit.potion {
			t.Fatalf("at %v: healed %d, dealt %d, want %d and %d", hit.at, healed, damage, baseHeal+hit.potion, 90+hit.potion)
		}
	}
}

func TestThanksgivingCharmUsesLevelAndPermanentImmunity(t *testing.T) {
	r, _, _ := charmRig(t)
	r.p.Class = 0 // warrior, with a level well below the target's
	r.p.Level = 1
	npc := r.enemy
	pe := thanksgivingReward(t, yellingAtCousins)
	if !pe.Def.NoRecharge || pe.Def.Duration != 2*time.Hour || pe.Def.CharmMax != 55 {
		t.Fatalf("charm item definition: %+v", pe.Def)
	}
	try := func(want string) {
		t.Helper()
		if why, err := r.w.UsePotion(r.sess, pe, crabID, r.at(0)); why != want || err != nil {
			t.Fatalf("using charm: %q, %v, want %q", why, err, want)
		}
	}
	npc.Level = 56
	try(charmTooHigh)
	npc.Level = 55
	key := database.NPCClassKey(npc.Name)
	for _, rule := range []database.CharmRule{{Charmable: false}, {Charmable: true, ResistPct: 100}} {
		r.w.SetCharmRules(map[string]database.CharmRule{key: rule})
		try(charmCannot)
	}
	r.w.SetCharmRules(map[string]database.CharmRule{key: {Charmable: true, ResistPct: 99}})
	old := &petState{owner: r.p, npc: &NPC{HP: 100}}
	r.w.pets = map[EntityID]*petState{r.p.ServerID: old}
	try(charmHavePet)
	if r.w.pets[r.p.ServerID] != old {
		t.Fatal("the summoned pet was replaced")
	}
	delete(r.w.pets, r.p.ServerID)
	if npc.Owner != 0 || npc.Charm != nil {
		t.Fatal("a refused charm took the target")
	}
	oldRoll := charmIntn
	t.Cleanup(func() { charmIntn = oldRoll })
	charmIntn = func(int) int {
		t.Fatal("unresistable charm rolled for resistance")
		return 0
	}
	try("")
	if npc.Owner != r.p.ServerID || npc.Charm == nil || !npc.Charm.until.Equal(r.at(2*time.Hour)) {
		t.Fatalf("the level 55 target was not charmed for two hours: owner %d, charm %+v", npc.Owner, npc.Charm)
	}
}
