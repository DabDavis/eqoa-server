package entity

import "testing"

// A hate-reducer cast at a friendly player takes his hate off what fights him (hatedrop.go,
// petition 3258).

func concussion() CastSpell {
	return spellOf(1630, 1, "Concussion", "Target, Decrease Hate by 144 ; Increase 5% based on DEX")
}

func TestAHateReducerOnAFriendTakesHisHate(t *testing.T) {
	r := newAreaRig(t)
	friend := r.player(1002, "Tank", 1010, 1000)
	a := r.creature(2001, "a gnoll", 1015, 1000)
	b := r.creature(2002, "a gnoll pup", 1012, 1000)
	other := r.creature(2003, "an orc", 1020, 1000)
	r.w.mu.Lock()
	r.w.updateAggro(a, friend.ServerID, 500)
	r.w.updateAggro(b, friend.ServerID, 100)
	r.w.updateAggro(other, 9999, 300) // fights someone else
	r.w.mu.Unlock()

	r.cast(t, concussion(), friend.ServerID) // 144 + 5% of DEX 100 = 149
	if got := a.Threats[friend.ServerID]; got != 500-149 {
		t.Errorf("the gnoll holds %d for the friend, want %d", got, 500-149)
	}
	if got := b.Threats[friend.ServerID]; got != 0 {
		t.Errorf("the pup holds %d for the friend, want 0 (the floor)", got)
	}
	if _, ok := other.Threats[friend.ServerID]; ok {
		t.Errorf("the orc, fighting no friend, got a hate for him: %v", other.Threats)
	}
	for _, n := range []*NPC{a, b, other} {
		if _, ok := n.Threats[r.p.ServerID]; ok {
			t.Errorf("%s holds hate for the caster: %v", n.Name, n.Threats)
		}
	}
}

func TestOnlyAHateReducerIsCastAtAFriend(t *testing.T) {
	r := newAreaRig(t)
	friend := r.player(1002, "Tank", 1010, 1000)
	for _, s := range []CastSpell{
		spellOf(700169, 1, "Blazing Clash", "Target, Decrease Hate by 106, Decrease Hitpoints by 284"),
		spellOf(760304, 1, "Cat Scratch Fever", "Target, Decrease Hate by 1000, Decrease AGI by 50, 1min"),
		spellOf(1606, 1, "Taunt", "Target, Increase Hate by 106 ; Increase 16% based on STR"),
		spellOf(700145, 1, "Static Bolt", "Target, Decrease Hitpoints by 70"),
	} {
		r.w.mu.Lock()
		_, why := r.w.castTargetsLocked(r.sess, r.p, &s, r.objectID(friend.ServerID))
		r.w.mu.Unlock()
		if why != castFriendly {
			t.Errorf("%s at a friend: %q, want %q", s.Name, why, castFriendly)
		}
	}
	for _, s := range []CastSpell{concussion(),
		spellOf(700167, 1, "Avoidance", "Target, Decrease Hate by 225"),
		spellOf(700153, 1, "Impact", "Target, Decrease Hate by 578 ; Increase 20% based on DEX, Decrease Hate by 578 ; Increase 20% based on DEX"),
	} {
		r.w.mu.Lock()
		ids, why := r.w.castTargetsLocked(r.sess, r.p, &s, r.objectID(friend.ServerID))
		self, whySelf := r.w.castTargetsLocked(r.sess, r.p, &s, r.me)
		r.w.mu.Unlock()
		if why != "" || len(ids) != 1 || ids[0] != friend.ServerID {
			t.Errorf("%s at a friend: %v %q, want the friend", s.Name, ids, why)
		}
		if whySelf != castFriendly || self != nil {
			t.Errorf("%s at its caster: %v %q, want %q", s.Name, self, whySelf, castFriendly)
		}
	}
}

// The records as the server loads them (spelleffect.effects, EQOAEmu's workbook).
func TestTheRecordsHateReducersAreCastAtAFriend(t *testing.T) {
	for _, c := range []struct {
		name, line, effects string
		want                bool
	}{
		{"Jolt", "Target, Decrease Hate by 1000 ; Increase 100% based on DEX",
			`{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":60,"min_level":1,"req_equipment":"Range","sheet_type":"Modify Hate","effects":[{"id":"05C2","type":"Modify Hate","kind":"hate","amount":-1000,"scale":"dex","scale_pct":100}]}`, true},
		{"Concussion", "Target, Decrease Hate by 144 ; Increase 5% based on DEX",
			`{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":4,"min_level":1,"req_equipment":"Any","sheet_type":"Modify Hate","effects":[{"id":"0854","type":"Modify Hate","kind":"hate","amount":-144,"level_pct":330,"scale":"dex","scale_pct":5}]}`, true},
		{"Avoidance", "Target, Decrease Hate by 225",
			`{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":20,"min_level":1,"req_equipment":"Any","sheet_type":"Modify Hate","effects":[{"id":"0603","type":"Modify Hate","kind":"hate","amount":-225}]}`, true},
		{"Jolting Discharge", "Target, Decrease LR by 20, 1min",
			`{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":60,"stack":0,"level":50,"min_level":1,"req_equipment":"Any","sheet_type":"Statistic Change","arch_type":"Debuff LR","effects":[{"id":"0070","type":"Statistic Change","kind":"stat","stat":"lr","amount":-20,"resist":"lr"}]}`, false},
	} {
		d := SpellDefFrom(c.name, c.line, "", c.effects)
		if got := d.dropsHateOnly(); got != c.want || !d.Harmful {
			t.Errorf("%s: dropsHateOnly %v (harmful %v), want %v; effects %+v", c.name, got, d.Harmful, c.want, d.Effects)
		}
	}
}
