package entity

import (
	"testing"
	"time"
)

// The items: by their record's name in any case, nothing for another.
func TestWornShieldOf(t *testing.T) {
	if s, ok := WornShieldOf("Splendor"); !ok || s.Damage != 30 || s.Heal != 15 || s.Scale != StatNone || s.Item != "Splendor" {
		t.Errorf("Splendor: %+v %v", s, ok)
	}
	if s, ok := WornShieldOf("breastplate of the ANOINTED "); !ok || s.Damage != 0 || s.Heal != 25 || s.HealMax != 75 || s.Scale != StatCha {
		t.Errorf("the Anointed: %+v %v", s, ok)
	}
	for _, name := range []string{"seraphic planar chestplate", "Seraphic Planar Breastplate"} {
		if s, ok := WornShieldOf(name); !ok || s.Damage != 75 || s.Heal != 75 || s.Scale != StatInt {
			t.Errorf("%s: %+v %v", name, s, ok)
		}
	}
	for _, name := range []string{"traitorous breastplate", "worldbreaker chestguard", "Worldbreaker Breastplate", "Grundleskin Guard"} {
		if s, ok := WornShieldOf(name); !ok || s.Damage != 0 || s.Heal != 25 || s.Scale != StatNone {
			t.Errorf("%s: %+v %v", name, s, ok)
		}
	}
	for _, name := range []string{"seraphic planar robe", "Seraphic Planar Greaves", "Worldbreaker Greaves", "Grundleskin Bracers", "Traitorous Greaves"} {
		if _, ok := WornShieldOf(name); ok {
			t.Errorf("%s answers a hit", name)
		}
	}
	for _, name := range []string{"Crown of the Jaguar", "Nightshade Gloves", "nightshade gloves"} {
		if s, ok := WornShieldOf(name); !ok || s.Damage != 30 || s.Heal != 15 || s.Scale != StatNone {
			t.Errorf("%s: %+v %v", name, s, ok)
		}
	}
	for _, name := range []string{"Robe of the Anointed", "Nightshade Boots", "Nite Jaguar-Hide Gloves"} {
		if _, ok := WornShieldOf(name); ok {
			t.Errorf("%s answers a hit", name)
		}
	}
}

// Splendor (the user, 2026-11-05: "does 30 damage and heals 15 per hit. no
// scaling"): the crab's hit on its wearer is answered as a damage shield's
// (the wearer's 0x00B1 at the crab with the shields' visual, then its 30 as
// the wearer's), and the wearer gets 15 back, told as a heal is. At full
// hit points the heal is neither given nor told; the damage still is.
func TestSplendorHitsBackAndHeals(t *testing.T) {
	c, crab := casterAndCrab(t)
	s, _ := WornShieldOf("Splendor")
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

	// at full hit points: the damage, no heal
	c.p.HP = c.p.MaxHPNow()
	c.w.mu.Lock()
	c.w.playerShieldsLocked(c.sess, c.p, crab, c.at(2*time.Second))
	c.w.mu.Unlock()
	if got := c.sent(); !sameMsgs(got, [][]byte{castMsg(c.me, c.npcO, 0, fxShieldHit), damageMsg(t, c.me, -30, c.npcO)}) {
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

// The Breastplate of the Anointed (the user, 2026-11-05: "heals on contact
// base starting at 25, max at 75 cha scales it"): no damage, the wearer
// healed 25 at no CHA, 75 at its CHA cap and in a line between; CHA past
// the cap counts as the cap.
func TestTheAnointedHealsByCha(t *testing.T) {
	c, crab := casterAndCrab(t)
	s, _ := WornShieldOf("Breastplate of the Anointed")
	c.p.Gear.Shields = []WornShield{s}
	capCha := c.p.StatCapOf(StatCha)
	for _, k := range []struct {
		cha, want int
	}{{0, 25}, {capCha / 2, 50}, {capCha, 75}, {capCha + 100, 75}} {
		c.p.Cha, c.p.Mods.Cha = k.cha, 0
		c.p.HP = c.p.MaxHPNow() - 200
		c.sent()
		c.w.mu.Lock()
		c.w.playerShieldsLocked(c.sess, c.p, crab, c.at(time.Second))
		c.w.mu.Unlock()
		got := c.sent()
		if healed := c.p.HP - (c.p.MaxHPNow() - 200); healed != k.want ||
			!sameMsgs(got, [][]byte{damageMsg(t, c.me, k.want, c.me)}) {
			t.Errorf("CHA %d of %d: healed %d, want %d; sent % X", k.cha, capCha, healed, k.want, got)
		}
	}
	if crab.HP != 2000 {
		t.Errorf("the Anointed hurt the crab: %d HP", crab.HP)
	}
	// short by less than the heal: up to the most, the whole heal told
	c.p.Cha = capCha
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
	s, _ := WornShieldOf("Splendor")
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
// int higher, one of the very best in the game"): both 75 at no INT, 225 at
// the wearer's INT cap (the user's choice: the Anointed's 3x) and in a line
// between, past
// the cap the cap's; CHA moves nothing.
func TestTheSeraphicPlanarScalesByInt(t *testing.T) {
	c, crab := casterAndCrab(t)
	s, _ := WornShieldOf("seraphic planar chestplate")
	c.p.Gear.Shields = []WornShield{s}
	capInt := c.p.StatCapOf(StatInt)
	c.p.Cha = c.p.StatCapOf(StatCha)
	crabHP := crab.HP
	for _, k := range []struct {
		intel, want int
	}{{0, 75}, {capInt / 2, 150}, {capInt, 225}, {capInt + 50, 225}} {
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
