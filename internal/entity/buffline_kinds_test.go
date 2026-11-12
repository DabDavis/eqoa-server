package entity

import (
	"strings"
	"testing"
	"time"
)

// Lines across the two kinds (buffline.go, 2026-11-12: the GM Jibrish's
// three cleric mercs). The cleric's wards are plain lines, each an element
// more: a lower one under a higher one is refused (it adds nothing), a
// higher one takes the lower one off. Blessing (plain, self, +CHA) and Minor
// Blessing (the workbook's self +CHA) are one line. Lightning Ward (plain,
// single target) and Consume Affliction (the workbook's single-target PR/DR)
// are nested: both stay, PR and DR the stronger once; with Group Resist
// Affliction (the workbook's group PR/DR, another stack type) beside them,
// its PR and DR count apart, as before. One that adds nothing over the ward
// (60 PR/DR under its 100) is refused.
func TestBuffLinesAcrossKinds(t *testing.T) {
	noResist(t)
	disease := buffOf(700184, 20, 1, "Disease Ward", "Target, buff DR by 30, 30min")
	ice := buffOf(700615, 49, 1, "Ice Ward", "Target, buff DR, PR, FR, CR by 100, 30min")
	lightning := buffOf(700620, 49, 1, "Lightning Ward", "Target, buff DR, PR, FR, CR, LR by 100, 30min")
	if ice.Def.Workbook || !nestedLines(EffectLine(&ice), EffectLine(&lightning)) {
		t.Fatalf("lines %q %q: not nested", EffectLine(&ice), EffectLine(&lightning))
	}
	c := newCaster(t)
	c.castOn(t, disease, 0)
	c.castOn(t, ice, 10*time.Second)
	if got := c.effectNames(); strings.Join(got, ",") != "Ice Ward" || c.p.Mods.DR != 100 {
		t.Fatalf("Ice Ward over Disease Ward: %v, DR %d", got, c.p.Mods.DR)
	}
	c.castOn(t, lightning, 20*time.Second)
	if got := c.effectNames(); strings.Join(got, ",") != "Lightning Ward" || c.p.Mods.LR != 100 || c.p.Mods.PR != 100 {
		t.Fatalf("Lightning Ward over Ice Ward: %v, %+v", got, c.p.Mods)
	}
	c.castOn(t, ice, 30*time.Second)
	if got := c.effectNames(); strings.Join(got, ",") != "Lightning Ward" {
		t.Fatalf("Ice Ward under Lightning Ward: %v", got)
	}
	if red := redLines(c.sess); len(red) == 0 || !strings.Contains(red[len(red)-1], "Lightning Ward, which is stronger") {
		t.Errorf("not told: %q", red)
	}

	minor := wbSpell(236, 3, 0, "Minor Blessing", `{"v":1,"scope":"self","stack":2,"level":3,"duration_s":1800,"effects":[{"kind":"stat","stat":"cha","amount":1}]}`)
	blessing := buffOf(700017, 3, 0, "Blessing", "Self, Increase CHA by 1, 30min")
	if !sameLine(EffectLine(&minor), EffectLine(&blessing)) {
		t.Fatalf("%q and %q: not one line", EffectLine(&minor), EffectLine(&blessing))
	}
	c = newCaster(t)
	c.castOn(t, minor, 0)
	c.castOn(t, blessing, 10*time.Second)
	if got := c.effectNames(); strings.Join(got, ",") != "Blessing" || c.p.Mods.Cha != 1 {
		t.Fatalf("Blessing over Minor Blessing: %v, CHA %d", got, c.p.Mods.Cha)
	}

	consume := wbSpell(700690, 54, 1, "Consume Affliction", `{"v":1,"scope":"target","stack":1,"level":54,"duration_s":1800,"effects":[{"kind":"stat","stat":"pr","amount":120},{"kind":"stat","stat":"dr","amount":120}]}`)
	group := wbSpell(700396, 35, 2, "Group Resist Affliction", `{"v":1,"scope":"group","stack":3,"level":35,"duration_s":1800,"effects":[{"kind":"stat","stat":"pr","amount":45},{"kind":"stat","stat":"dr","amount":45}]}`)
	c = newCaster(t)
	c.castOn(t, lightning, 0)
	c.castOn(t, consume, 10*time.Second)
	if got := c.effectNames(); len(got) != 2 || c.p.Mods.PR != 120 || c.p.Mods.FR != 100 {
		t.Fatalf("Lightning Ward and Consume Affliction: %v, PR %d FR %d (want both on, PR 120 once)", got, c.p.Mods.PR, c.p.Mods.FR)
	}
	c.castOn(t, group, 20*time.Second)
	if got := c.effectNames(); len(got) != 3 || c.p.Mods.PR != 165 {
		t.Fatalf("and Group Resist Affliction: %v, PR %d (want 120 + 45)", got, c.p.Mods.PR)
	}
	// a single-target PR/DR buff that adds nothing over the ward is refused
	weak := wbSpell(700518, 45, 1, "Absorb Affliction", `{"v":1,"scope":"target","stack":1,"level":45,"duration_s":1800,"effects":[{"kind":"stat","stat":"pr","amount":60},{"kind":"stat","stat":"dr","amount":60}]}`)
	c = newCaster(t)
	c.castOn(t, lightning, 0)
	c.castOn(t, weak, 10*time.Second)
	if got := c.effectNames(); strings.Join(got, ",") != "Lightning Ward" {
		t.Fatalf("Absorb Affliction under Lightning Ward: %v", got)
	}

	// a pet (a merc) by the same rule
	pet := &NPC{Name: "Jorador"}
	for _, sp := range []CastSpell{disease, lightning, ice, minor, blessing} {
		ae := &ActiveEffect{SpellID: sp.SpellID, Name: sp.Name, Until: c.at(time.Hour), Mods: staticMods(&sp)}
		putOnPetLocked(pet, &sp, ae)
	}
	var names []string
	for _, e := range pet.Effects {
		names = append(names, e.Name)
	}
	if strings.Join(names, ",") != "Lightning Ward,Blessing" {
		t.Errorf("the pet's: %v", names)
	}
}

// A merc's book keeps each line's best: no Ice Ward under Lightning Ward, one
// of Blessing and Minor Blessing, nothing far beneath its level; and buffOn
// counts a covering nested line as on.
func TestMercBookKeepsEachLinesBest(t *testing.T) {
	disease := buffOf(700184, 20, 1, "Disease Ward", "Target, buff DR by 30, 30min")
	ice := buffOf(700615, 49, 1, "Ice Ward", "Target, buff DR, PR, FR, CR by 100, 30min")
	lightning := buffOf(700620, 49, 1, "Lightning Ward", "Target, buff DR, PR, FR, CR, LR by 100, 30min")
	order := buffOf(700805, 60, 1, "Order", "Target, Increase AC by 650, Increase Maximum HP by 650, 30min")
	wisdom := buffOf(234, 1, 0, "Divine Wisdom", "Self, Increase INT by 1, 30min")
	var book []mercSpell
	for _, sp := range []CastSpell{disease, ice, lightning, order, wisdom} {
		s := sp
		book = append(book, mercSpell{s: &s, use: useBuff})
	}
	var names []string
	for _, sp := range mercBestBuffs(book) {
		names = append(names, sp.s.Name)
	}
	if strings.Join(names, ",") != "Lightning Ward,Order,Divine Wisdom" {
		t.Errorf("best: %v", names)
	}
	on := []*ActiveEffect{{SpellID: lightning.SpellID, Line: EffectLine(&lightning), Rank: 49, Mods: staticMods(&lightning)}}
	if !buffOn(on, &ice) || buffOn(on, &order) {
		t.Errorf("buffOn: Ice Ward %v (want on), Order %v (want off)", buffOn(on, &ice), buffOn(on, &order))
	}
}
