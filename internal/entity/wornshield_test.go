package entity

import (
	"testing"
	"time"
)

// The items: by their record's name in any case, nothing for another; a
// record keyed by its id answers by it alone.
func TestWornShieldOf(t *testing.T) {
	of := func(name string) (WornShield, bool) { return WornShieldOf(0, name) }
	if s, ok := of("Splendor"); !ok || s.Damage != 30 || s.Heal != 15 || s.Scale != StatNone || s.Item != "Splendor" {
		t.Errorf("Splendor: %+v %v", s, ok)
	}
	if s, ok := of("breastplate of the ANOINTED "); !ok || s.Damage != 0 || s.Heal != 15 || s.HealPct != 10 || s.HealMax != 75 ||
		s.Scale != StatCha || s.Line {
		t.Errorf("the Anointed: %+v %v", s, ok)
	}
	for _, name := range []string{"seraphic planar chestplate", "Seraphic Planar Breastplate"} {
		if s, ok := of(name); !ok || s.Damage != 75 || s.Heal != 75 || s.Scale != StatInt || !s.Line {
			t.Errorf("%s: %+v %v", name, s, ok)
		}
	}
	for _, name := range []string{"traitorous breastplate", "worldbreaker chestguard", "Worldbreaker Breastplate", "Grundleskin Guard"} {
		if s, ok := of(name); !ok || s.Damage != 0 || s.Heal != 25 || s.Scale != StatNone {
			t.Errorf("%s: %+v %v", name, s, ok)
		}
	}
	for _, name := range []string{"Crown of the Jaguar", "Nightshade Gloves", "nightshade gloves"} {
		if s, ok := of(name); !ok || s.Damage != 30 || s.Heal != 15 || s.Scale != StatNone {
			t.Errorf("%s: %+v %v", name, s, ok)
		}
	}
	// the sheet's, as this server names them
	for name, want := range map[string]WornShield{
		"Baleful Hide Chestguard": {Damage: 23, DamageTo: 24}, "Coat of Jagged Rust": {Damage: 39, DamageTo: 40},
		"Golden Spiked Boots": {Damage: 30}, "Guard of the Deep": {Damage: 20}, "Rune Crested Gloves": {Damage: 30},
		"Stinging Kite Shield": {Damage: 20}, "Thornlink": {Damage: 40}, "positronian energy shield": {Damage: 200, Undead: true},
		"Marauder Breastplate": {Damage: 10, Heal: 5}, "Tower of Woe": {Damage: 90, Heal: 45},
		"Mithron Chestplate": {Heal: 15, Power: 15}, "coralium chestguard": {Power: 15}, "Coral Tunic": {Power: 25},
		"fathomsteel guard": {Power: 15}, "FesterTwig": {Power: 15},
		// QuestMasters' (2006)
		"bracers of toxicity": {Damage: 64}, "Golden Spiked Gloves": {Damage: 20}, "Sandals of the Burning Sands": {Damage: 35},
	} {
		want.Item = name
		if s, ok := of(name); !ok || s != want {
			t.Errorf("%s: %+v %v, want %+v", name, s, ok, want)
		}
	}
	// the named Mithril Breastplate by its record; the smiths' of its name answers nothing
	if s, ok := WornShieldOf(2251, "Mithril Breastplate"); !ok || s.Power != 15 {
		t.Errorf("the Mithril Breastplate (2251): %+v %v", s, ok)
	}
	if _, ok := WornShieldOf(703150, "mithril breastplate"); ok {
		t.Error("a smith's mithril breastplate answers a hit")
	}
	for _, name := range []string{"Robe of the Anointed", "Nightshade Boots", "Nite Jaguar-Hide Gloves", "seraphic planar robe",
		"Seraphic Planar Greaves", "Worldbreaker Greaves", "Grundleskin Bracers", "Traitorous Greaves", "Golden Spiked Bracers",
		"shield of the isle", "Imperial Breastplate", "Tobrin's Eyepatch", "Martyr's Breastplate"} {
		if _, ok := of(name); ok {
			t.Errorf("%s answers a hit", name)
		}
	}
}

// Splendor (the user, 2026-11-05: "does 30 damage and heals 15 per hit. no
// scaling"): the crab's hit on its wearer is answered as a damage shield's
// (the wearer's 0x00B1 at the crab with the shields' visual, then its 30 as
// the wearer's), and the wearer gets 15 back, told as a heal is. At full
// hit points the heal is not given but still told, green (the user,
// 2026-11-08: "the heal proc should show green numbers for player"); the
// damage is as ever.
func TestSplendorHitsBackAndHeals(t *testing.T) {
	c, crab := casterAndCrab(t)
	s, _ := WornShieldOf(2256, "Splendor")
	c.p.Gear.Shields = []WornShield{s}
	c.p.HP = c.p.MaxHPNow() - 100
	c.sent()

	c.w.mu.Lock()
	c.w.npcAttackPlayer(crab, c.p.ServerID, c.at(time.Second))
	c.w.mu.Unlock()
	got := c.sent()
	if len(got) != 4 {
		t.Fatalf("the crab's hit sent %d messages, want its hit, the answer's cast and damage, the heal:\n% X", len(got), got)
	}
	hit := c.p.MaxHPNow() - 100 - (c.p.HP - 15)
	want := [][]byte{damageMsg(t, c.npcO, -hit, c.me), castMsg(c.me, c.npcO, 0, fxShieldHit),
		damageMsg(t, c.me, -30, c.npcO), damageMsg(t, c.me, 15, c.me)}
	if !sameMsgs(got, want) {
		t.Fatalf("the crab's hit sent\n% X\nwant\n% X", got, want)
	}
	if crab.HP != 1970 {
		t.Errorf("the crab after the answer: %d HP, want 1970", crab.HP)
	}

	// at full hit points: the damage, and the heal's figure with nothing healed
	c.p.HP = c.p.MaxHPNow()
	c.w.mu.Lock()
	c.w.playerShieldsLocked(c.sess, c.p, crab, c.at(2*time.Second))
	c.w.mu.Unlock()
	if got := c.sent(); !sameMsgs(got, [][]byte{castMsg(c.me, c.npcO, 0, fxShieldHit), damageMsg(t, c.me, -30, c.npcO), damageMsg(t, c.me, 15, c.me)}) {
		t.Errorf("at full hit points sent\n% X", got)
	}
	if crab.HP != 1940 || c.p.HP != c.p.MaxHPNow() {
		t.Errorf("at full hit points: the crab %d, the wearer %d/%d", crab.HP, c.p.HP, c.p.MaxHPNow())
	}

	// the control: nothing worn, nothing answers
	c.p.Gear.Shields = nil
	c.p.HP = c.p.MaxHPNow() - 100
	c.w.mu.Lock()
	c.w.playerShieldsLocked(c.sess, c.p, crab, c.at(3*time.Second))
	c.w.mu.Unlock()
	if got := c.sent(); len(got) != 0 || crab.HP != 1940 || c.p.HP != c.p.MaxHPNow()-100 {
		t.Errorf("nothing worn: sent % X, the crab %d, the wearer short %d", got, crab.HP, c.p.MaxHPNow()-c.p.HP)
	}
}

// The Breastplate of the Anointed (the sheet's formula, the user's choice:
// "procs 15 + 10% cha, so at 600 cha procs for 75"): no damage, the wearer
// healed 15 and a tenth of its CHA as the client shows it (within the cap:
// 400 at level 46 and up), never past 75; 600 CHA reached with the cap
// masteries heals 75, and more heals no more.
func TestTheAnointedHealsByCha(t *testing.T) {
	c, crab := casterAndCrab(t)
	s, _ := WornShieldOf(1076, "Breastplate of the Anointed")
	c.p.Gear.Shields = []WornShield{s}
	capCha := c.p.StatCapOf(StatCha)
	for _, k := range []struct {
		cha, raise, want int
	}{{0, 0, 15}, {100, 0, 25}, {200, 0, 35}, {capCha, 0, 15 + capCha/10}, {capCha + 100, 0, 15 + capCha/10},
		{600, 600 - capCha, 75}, {800, 800 - capCha, 75}} {
		c.p.Cha, c.p.Mods.Cha = k.cha, 0
		c.p.CMEffects[cmSlotCap+int(StatCha-StatStr)] = k.raise
		c.p.HP = c.p.MaxHPNow() - 200
		c.sent()
		c.w.mu.Lock()
		c.w.playerShieldsLocked(c.sess, c.p, crab, c.at(time.Second))
		c.w.mu.Unlock()
		got := c.sent()
		if healed := c.p.HP - (c.p.MaxHPNow() - 200); healed != k.want ||
			!sameMsgs(got, [][]byte{damageMsg(t, c.me, k.want, c.me)}) {
			t.Errorf("CHA %d (cap %d): healed %d, want %d; sent % X", k.cha, c.p.StatCapOf(StatCha), healed, k.want, got)
		}
	}
	if crab.HP != 2000 {
		t.Errorf("the Anointed hurt the crab: %d HP", crab.HP)
	}
	// short by less than the heal: up to the most, the whole heal told
	c.p.HP = c.p.MaxHPNow() - 10
	c.w.mu.Lock()
	c.w.playerShieldsLocked(c.sess, c.p, crab, c.at(time.Second))
	c.w.mu.Unlock()
	if c.p.HP != c.p.MaxHPNow() {
		t.Errorf("a heal past the most: %d/%d", c.p.HP, c.p.MaxHPNow())
	}
}

// Worn beside a spell's damage shield, both answer: the item's effect is its
// own, not one of a buff line (only the strongest of those answers).
func TestAWornShieldAnswersBesideASpellShield(t *testing.T) {
	c, crab := casterAndCrab(t)
	now := c.at(time.Minute)
	c.p.Effects = []*ActiveEffect{shieldEffect("Barbshield", 8, 0, now.Add(time.Minute))}
	s, _ := WornShieldOf(2256, "Splendor")
	c.p.Gear.Shields = []WornShield{s}
	c.p.HP = c.p.MaxHPNow()
	c.sent()
	c.w.mu.Lock()
	c.w.playerShieldsLocked(c.sess, c.p, crab, now)
	c.w.mu.Unlock()
	c.sent()
	if crab.HP != 2000-8-30 {
		t.Errorf("both answering: the crab %d HP, want %d", crab.HP, 2000-8-30)
	}
}

// The Seraphic Planar (the user, 2026-11-05: "75 heal and damage. scales with
// int higher, one of the very best in the game"): both 75 at no INT, 120 at
// the wearer's INT cap (the user, 2026-11-08: "the damage/heal shield should
// scale to 120 damage"; 225 before) and in a line between, past the cap the
// cap's; CHA moves nothing. Its record is the Seraphic Planar Breastplate.
func TestTheSeraphicPlanarScalesByInt(t *testing.T) {
	c, crab := casterAndCrab(t)
	s, _ := WornShieldOf(8527, "Seraphic Planar Breastplate")
	c.p.Gear.Shields = []WornShield{s}
	capInt := c.p.StatCapOf(StatInt)
	c.p.Cha = c.p.StatCapOf(StatCha)
	crabHP := crab.HP
	for _, k := range []struct {
		intel, want int
	}{{0, 75}, {capInt / 2, 97}, {capInt, 120}, {capInt + 50, 120}} {
		c.p.Int, c.p.Mods.Int = k.intel, 0
		c.p.HP = c.p.MaxHPNow() - 500
		c.sent()
		c.w.mu.Lock()
		c.w.playerShieldsLocked(c.sess, c.p, crab, c.at(time.Second))
		c.w.mu.Unlock()
		got := c.sent()
		want := [][]byte{castMsg(c.me, c.npcO, 0, fxShieldHit), damageMsg(t, c.me, -k.want, c.npcO), damageMsg(t, c.me, k.want, c.me)}
		if healed := c.p.HP - (c.p.MaxHPNow() - 500); healed != k.want || crabHP-crab.HP != k.want || !sameMsgs(got, want) {
			t.Errorf("INT %d of %d: healed %d, hit %d, want %d; sent % X", k.intel, capInt, healed, crabHP-crab.HP, k.want, got)
		}
		crabHP = crab.HP
	}
}

// A range is rolled each hit (the Coat of Jagged Rust's "Ds 39-40": both
// ends come); power comes back to the wearer up to its most, told nothing
// (the Coral Tunic's "25 mana back per hit"); the Positronian energy
// shield's 200 lands on undead alone.
func TestTheSheetsShields(t *testing.T) {
	defer func(f func(int) int) { procIntn = f }(procIntn)

	c, crab := casterAndCrab(t)
	rust, _ := WornShieldOf(2273, "Coat of Jagged Rust")
	c.p.Gear.Shields = []WornShield{rust}
	hp := crab.HP
	for _, roll := range []int{0, 1} {
		procIntn = dice(roll)
		c.w.mu.Lock()
		c.w.playerShieldsLocked(c.sess, c.p, crab, c.at(time.Second))
		c.w.mu.Unlock()
		if want := 39 + roll; hp-crab.HP != want {
			t.Errorf("the rust, rolling %d: hit %d, want %d", roll, hp-crab.HP, want)
		}
		hp = crab.HP
	}

	tunic, _ := WornShieldOf(3512, "Coral Tunic")
	c.p.Gear.Shields = []WornShield{tunic}
	top := c.p.MaxPowerNow()
	c.p.Power = top - 30
	c.sent()
	for _, want := range []int{top - 5, top} {
		c.w.mu.Lock()
		c.w.playerShieldsLocked(c.sess, c.p, crab, c.at(time.Second))
		c.w.mu.Unlock()
		if c.p.Power != want {
			t.Errorf("the tunic: power %d, want %d (of %d)", c.p.Power, want, top)
		}
	}
	if got := c.sent(); len(got) != 0 || crab.HP != hp {
		t.Errorf("the tunic: sent % X, the crab %d (was %d)", got, crab.HP, hp)
	}

	positronian, _ := WornShieldOf(6067, "positronian energy shield")
	c.p.Gear.Shields = []WornShield{positronian}
	c.w.mu.Lock()
	c.w.playerShieldsLocked(c.sess, c.p, crab, c.at(time.Second))
	c.w.mu.Unlock()
	if crab.HP != hp {
		t.Errorf("the positronian shield hurt a crab: %d (was %d)", crab.HP, hp)
	}
	crab.Name = "a decaying skeleton"
	c.w.mu.Lock()
	c.w.playerShieldsLocked(c.sess, c.p, crab, c.at(time.Second))
	c.w.mu.Unlock()
	if hp-crab.HP != 200 {
		t.Errorf("the positronian shield on a skeleton: hit %d, want 200", hp-crab.HP)
	}
}

// The Anointed's heal is hate on the creature that hit its wearer, at full
// health too, as much as it would heal, and a wizard's lesser damage does not
// turn the creature away; the control, Worldbreaker's heal, is no hate.
func TestTheAnointedHoldsHate(t *testing.T) {
	r := newProcRig(t)
	r.friend.Cha = 700
	for name, hates := range map[string]bool{"Breastplate of the Anointed": true, "worldbreaker chestguard": false} {
		s, ok := WornShieldOf(0, name)
		if !ok {
			t.Fatal(name)
		}
		r.friend.Gear.Shields = []WornShield{s}
		r.friend.HP = r.friend.MaxHPNow()
		_, heal, _ := s.forWearer(r.friend, procIntn)
		want := int64(0)
		if hates {
			want = int64(heal)
		}
		if heal <= 0 {
			t.Fatalf("%s heals %d", name, heal)
		}
		r.npc.Threats = nil
		r.w.mu.Lock()
		r.w.wornShieldsLocked(r.fsess, r.friend, r.npc)
		r.w.mu.Unlock()
		if got := r.npc.Threats[r.friend.ServerID]; got != want {
			t.Errorf("%s: hate %d, want %d", name, got, want)
		}
	}
	s, _ := WornShieldOf(0, "Breastplate of the Anointed")
	r.friend.Gear.Shields = []WornShield{s}
	r.npc.Threats = nil
	r.w.mu.Lock()
	r.w.addThreatLocked(r.npc, r.p.ServerID, 56) // the wizard's area spell
	for range 2 {
		r.w.wornShieldsLocked(r.fsess, r.friend, r.npc)
	}
	r.w.mu.Unlock()
	if r.npc.CurrentEnemy != r.friend.ServerID {
		t.Errorf("its enemy is %d, not the wearer %d", r.npc.CurrentEnemy, r.friend.ServerID)
	}
}
