package entity

import (
	"strings"
	"testing"
	"time"
)

// The Testing Heroes Vendor's items as the World plays them (petition 1471;
// tools/seed_testing_heroes.py writes their rows): the Elixir of the Giant
// Gnome's body and size, the Tome of Heroic Training's points, the Weakened
// and the Ascended Breastplate of the Anointed's heals. Every body and id but
// the breastplate's own record (1076) is made up.

const (
	testGnomeMale   uint32 = 0x0A0B0C11
	testGnomeFemale uint32 = 0x0A0B0C12
)

// elixir is the Elixir of the Giant Gnome's row as the tool writes it, on the
// test's gnome body.
func elixir() PotionEffect {
	pe := NewPotionEffect(9101, "Elixir of the Giant Gnome", "Self, Body Size 4, 30min", "Giant Gnome", 0, "", 0, 0)
	pe.SetBody(testGnomeMale)
	return pe
}

func gnomeRig(t *testing.T) *caster {
	t.Helper()
	c := bodyRig(t)
	c.w.SetRaceModels(map[[2]int]uint32{{3, 0}: testGnomeMale, {3, 1}: testGnomeFemale})
	c.w.SetRaceSizes(map[int]float64{3: 0.9}) // the players' gnomes are a tenth smaller: the elixir's size is its own
	return c
}

// The words: a body size and training points are read off a line, bounded,
// and a line without them holds neither.
func TestTheLinesWordsForABodySizeAndTrainingPoints(t *testing.T) {
	d := ParseSpellText("Self, Body Size 4, 30min")
	if d.FormSize != 4 || len(d.Unread) != 0 || d.Duration != 30*time.Minute {
		t.Errorf("the elixir's line: size %v, unread %q, %v", d.FormSize, d.Unread, d.Duration)
	}
	for line, want := range map[string]float32{"Self, Body Size 4.05, 30min": 4.05, "Self, Form Size 2, 1min": 2,
		"Self, Body Size 400, 1min": FormSizeMost, "Self, Body Size 0.01, 1min": FormSizeLeast, "Self, Increase STR by 5, 1min": 0} {
		if got := ParseSpellText(line).FormSize; got != want {
			t.Errorf("%q: size %v, want %v", line, got, want)
		}
	}
	tome := ParseSpellText("Self, Increase Training Points by 5")
	if tome.TrainingPoints() != 5 || len(tome.Unread) != 0 {
		t.Errorf("the tome's line: %d points, unread %q", tome.TrainingPoints(), tome.Unread)
	}
	for line, want := range map[string]int{"Self, Increase Unspent Training Points by 7": 7, "Self, Increase Training Points by 5000": TrainingGiftMost,
		"Self, Increase Training Points by 0": 0, "Self, Increase STR by 5, 1min": 0} {
		d := ParseSpellText(line)
		if got := d.TrainingPoints(); got != want {
			t.Errorf("%q: %d points, want %d", line, got, want)
		}
	}
}

// The elixir shows its drinker as a gnome of its own sex at the line's size
// (never the gnome players' 0.9), for thirty minutes and no longer; a death
// ends it (the petition: "Does not persist through death").
func TestTheElixirOfTheGiantGnome(t *testing.T) {
	c := gnomeRig(t)
	own, ownSize := channelBody(c.p)
	if r := c.use(t, elixir(), 0, 0); r != "" {
		t.Fatalf("refused: %q", r)
	}
	if m, s := channelBody(c.p); m != testGnomeMale || s != 4 {
		t.Fatalf("drunk: body %#x at %v, want the gnome %#x at 4", m, s, testGnomeMale)
	}
	c.potionTick(29 * time.Minute)
	if m, s := channelBody(c.p); m != testGnomeMale || s != 4 {
		t.Errorf("at 29 minutes: body %#x at %v", m, s)
	}
	c.potionTick(31 * time.Minute)
	if m, s := channelBody(c.p); m != own || s != ownSize {
		t.Errorf("after its thirty minutes: body %#x at %v, want its own %#x at %v", m, s, own, ownSize)
	}

	f := gnomeRig(t)
	f.p.Sex = 1
	f.use(t, elixir(), 0, 0)
	if m, s := channelBody(f.p); m != testGnomeFemale || s != 4 {
		t.Errorf("a woman drinks it: body %#x at %v, want the gnome woman's %#x at 4", m, s, testGnomeFemale)
	}

	d := gnomeRig(t)
	own, ownSize = channelBody(d.p)
	d.use(t, elixir(), 0, 0)
	d.w.mu.Lock()
	d.w.playerDiedLocked(d.sess, d.p, d.at(time.Minute))
	d.w.mu.Unlock()
	if m, s := channelBody(d.p); m != own || s != ownSize {
		t.Errorf("after a death: body %#x at %v, want its own %#x at %v", m, s, own, ownSize)
	}
	for _, e := range d.p.Effects {
		if e.Form != 0 {
			t.Errorf("after a death the effect %q still shows a body", e.Name)
		}
	}
}

// tome is the Tome of Heroic Training's row.
func tome() PotionEffect {
	return NewPotionEffect(9102, "Tome of Heroic Training", "Self, Increase Training Points by 5", "", 0, "", 0, 0)
}

// The tome gives five unspent training points, in the total and counted as no
// level's, told as a level's are (0x0036); where the reader's stats could not
// take five more it is refused and nothing is given.
func TestTheTomeOfHeroicTraining(t *testing.T) {
	c := newCaster(t)
	room := c.p.TrainingRoom()
	if room < 10 {
		t.Fatalf("no test: the caster's stats have room for %d", room)
	}
	c.p.UnusedTP, c.p.TotalTP, c.p.BonusTP = 3, 40, 0
	c.sent()
	if r := c.use(t, tome(), 0, 0); r != "" {
		t.Fatalf("refused: %q", r)
	}
	if c.p.UnusedTP != 8 || c.p.TotalTP != 45 || c.p.BonusTP != 5 {
		t.Fatalf("after the tome: %d unspent, %d in all, %d bonus; want 8, 45, 5", c.p.UnusedTP, c.p.TotalTP, c.p.BonusTP)
	}
	told := false
	for _, m := range c.sent() {
		if len(m) >= 4 && m[0] == 0x36 && m[1] == 0x00 && m[3] == 16 { // zigzag(8)
			told = true
		}
	}
	if !told {
		t.Error("the unspent points were not told (0x0036, 8)")
	}

	// at the cap: room for 4 more beside what is unspent
	c.p.UnusedTP = room - 4
	before := *c.p
	r := c.use(t, tome(), 0, time.Second)
	if r == "" || !strings.Contains(r, "Tome of Heroic Training is kept") {
		t.Fatalf("at the cap: %q, want the refusal", r)
	}
	if c.p.UnusedTP != before.UnusedTP || c.p.TotalTP != before.TotalTP || c.p.BonusTP != before.BonusTP {
		t.Errorf("a refused tome gave points: %d/%d/%d", c.p.UnusedTP, c.p.TotalTP, c.p.BonusTP)
	}
	// exactly room for five: given
	c.p.UnusedTP = room - 5
	if r := c.use(t, tome(), 0, 2*time.Second); r != "" || c.p.UnusedTP != room {
		t.Errorf("with room for exactly five: %q, %d unspent, want %d", r, c.p.UnusedTP, room)
	}
}

// A level's top-up leaves an item's points on top: a character of 50 holding
// five bonus points is given 50's own when it comes to 51, where without the
// bonus's count the total would have "held" five of them already.
func TestALevelsTopUpLeavesTheBonusPoints(t *testing.T) {
	due50, due51 := TrainingPointsDue(50), TrainingPointsDue(51)
	unused, total, give := TopUpTrainingBonus(51, 5, due50+5, 5)
	if give != due51-due50 || total != due51+5 || unused != 5+give {
		t.Errorf("to 51 with 5 bonus: gave %d (%d unspent, %d in all), want %d (%d in all)", give, unused, total, due51-due50, due51+5)
	}
	// the control: the same row without the count loses five of the level's
	if _, _, g := TopUpTraining(51, 5, due50+5); g != due51-due50-5 {
		t.Errorf("the control gave %d", g)
	}
	// no bonus: as TopUpTraining
	u1, t1, g1 := TopUpTrainingBonus(51, 0, due50, 0)
	u2, t2, g2 := TopUpTraining(51, 0, due50)
	if u1 != u2 || t1 != t2 || g1 != g2 {
		t.Errorf("no bonus: %d %d %d, want %d %d %d", u1, t1, g1, u2, t2, g2)
	}
	// a login at the same level gives nothing and keeps the total
	if u, tot, g := TopUpTrainingBonus(50, 5, due50+5, 5); g != 0 || u != 5 || tot != due50+5 {
		t.Errorf("at its level: gave %d (%d, %d)", g, u, tot)
	}
	p := &Player{Level: 51, UnusedTP: 5, TotalTP: due50 + 5, BonusTP: 5}
	if g := p.TopUpTrainingPoints(); g != due51-due50 || p.BonusTP != 5 {
		t.Errorf("the player's top-up gave %d, bonus %d", g, p.BonusTP)
	}
}

// The three breastplates' heals: the Weakened's half (petition 1520: 8 and 5% of
// CHA to 38), the Breastplate's own (15 and 10% to 75), the Ascended's a tenth more
// (the Ascended records' rule), each holding hate as the Anointed's does.
func TestTheThreeBreastplatesOfTheAnointed(t *testing.T) {
	defer SetAscendedItems(nil)
	SetAscendedItems(map[int]int{980777: 1076})
	base, ok := WornShieldOf(1076, "Breastplate of the Anointed")
	if !ok || base.Heal != 15 || base.HealPct != 10 || base.HealMax != 75 || !base.Hate || base.Scale != StatCha {
		t.Fatalf("the Breastplate: %+v %v", base, ok)
	}
	weak, ok := WornShieldOf(960777, "Weakened Breastplate of the Anointed")
	if !ok || weak.Heal != 8 || weak.HealPct != 5 || weak.HealMax != 38 || !weak.Hate || weak.Scale != StatCha || weak.Damage != 0 {
		t.Fatalf("the Weakened: %+v %v", weak, ok)
	}
	asc, ok := WornShieldOf(980777, "Ascended Breastplate of the Anointed")
	if !ok || asc.Heal <= base.Heal || asc.HealMax <= base.HealMax || !asc.Hate {
		t.Fatalf("the Ascended: %+v %v", asc, ok)
	}
	if WeakenedPercent != 50 {
		t.Errorf("petition 1520's 50%%: %d%%", WeakenedPercent)
	}

	// on a wearer, hit by the crab, at three CHAs: each heal of the Weakened's
	// is within a point of half the Breastplate's, the Ascended's of 110%
	c, crab := casterAndCrab(t)
	heal := func(s WornShield, cha int) int {
		c.p.Gear.Shields = []WornShield{s}
		c.p.Cha, c.p.Mods.Cha = cha, 0
		c.p.HP = c.p.MaxHPNow() - 300
		c.w.mu.Lock()
		c.w.playerShieldsLocked(c.sess, c.p, crab, c.at(time.Second))
		c.w.mu.Unlock()
		return c.p.HP - (c.p.MaxHPNow() - 300)
	}
	for _, cha := range []int{0, 100, 200} {
		b, w, a := heal(base, cha), heal(weak, cha), heal(asc, cha)
		if d := w*100 - b*WeakenedPercent; d < -100 || d > 100 {
			t.Errorf("CHA %d: the Weakened heals %d of the Breastplate's %d, want %d%% within a point", cha, w, b, WeakenedPercent)
		}
		if d := a*100 - b*110; d < -100 || d > 100 {
			t.Errorf("CHA %d: the Ascended heals %d of the Breastplate's %d, want 110%% within a point", cha, a, b)
		}
		if !(w < b && b < a) {
			t.Errorf("CHA %d: %d, %d, %d: not weakened < normal < ascended", cha, w, b, a)
		}
	}
}

// Any record named "Weakened <a worn item's name>" has that item's hit effect at
// WeakenedPercent, with no name of its own to list (petitions 1516, 1520: the
// weakened Seraphic Planar Breastplate, its 75-to-120 line at half, 38 to 60, its
// Ascended record a tenth more); an item's own name with no effect, "Weakened"
// or not, has none.
func TestAWeakenedRecordIsItsBasesByName(t *testing.T) {
	defer SetAscendedItems(nil)
	SetAscendedItems(map[int]int{980778: 8527})
	base, ok := WornShieldOf(8527, "Seraphic Planar Breastplate")
	if !ok || base.Damage != 75 || base.DamageMax != 120 || base.Heal != 75 || base.HealMax != 120 || !base.Line || base.Scale != StatInt {
		t.Fatalf("the Seraphic Planar: %+v %v", base, ok)
	}
	weak, ok := WornShieldOf(965004, "  weakened Seraphic Planar Breastplate ")
	if !ok || weak.Damage != 38 || weak.DamageMax != 60 || weak.Heal != 38 || weak.HealMax != 60 || !weak.Line || weak.Scale != StatInt {
		t.Fatalf("the Weakened Seraphic Planar: %+v %v", weak, ok)
	}
	if weak.Item != "  weakened Seraphic Planar Breastplate " {
		t.Errorf("the log's name: %q", weak.Item)
	}
	asc, ok := WornShieldOf(980778, "Ascended Seraphic Planar Breastplate")
	if !ok || asc.Damage != 83 || asc.DamageMax != 132 || asc.Heal != 83 || asc.HealMax != 132 {
		t.Fatalf("the Ascended Seraphic Planar: %+v %v", asc, ok)
	}
	// a weakened figure scales a stat as its base's does: half, a point either way
	c, _ := casterAndCrab(t)
	for _, v := range []int{0, 200, 400} {
		c.p.Int, c.p.Mods.Int = v, 0
		bd, bh, _ := base.forWearer(c.p, func(int) int { return 0 })
		wd, wh, _ := weak.forWearer(c.p, func(int) int { return 0 })
		if d := wd*100 - bd*WeakenedPercent; d < -100 || d > 100 || wh != wd || bh != bd {
			t.Errorf("INT %d: the Weakened %d/%d of %d/%d", v, wd, wh, bd, bh)
		}
	}
	for _, name := range []string{"Weakened Ceremonial Vestments", "Weakened ", "Ceremonial Vestments"} {
		if s, ok := WornShieldOf(965005, name); ok {
			t.Errorf("%q: %+v", name, s)
		}
	}
}
