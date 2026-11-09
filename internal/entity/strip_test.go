package entity

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// A templated creature's strip (strip.go; the GM Jibrish's petition 1368, Mr. Moogie's "caster
// debuff on his target every 30 seconds that strips any and all buffs, including immunity to
// damage / god mode").

const stripLanding = `[{"kind":"strip","every":30,"first":30,"name":"Royal Decree"}]`

// stripTemplate writes a template of abilities and gives it to the gnoll guard, as a GM's edit
// does.
func stripTemplate(t *testing.T, r *templateRig, abilities string) *NPC {
	t.Helper()
	if _, err := r.db.Exec(`INSERT INTO classtemplates (name, class, abilities) VALUES ('king', -1, ?)`, abilities); err != nil {
		t.Fatal(err)
	}
	if got := r.assign("gnoll guard", "king"); got != 1 {
		t.Fatalf("%d creatures of the template; problems %q", got, r.w.ClassTemplateProblems())
	}
	return r.npc("Gnoll Guard")
}

// Every kind of effect a strip decides on, as landings put them on (addEffectLocked): the first
// five what it leaves, the last nine buffs it takes off (the client keeps eight slots, so the
// first eight have one).
var kept = []string{"Withering Plague", "Recluse Venom", "Duelist's Bane", "Infernal Pact", "Boon of Freeport"}

func wearEverything(r *templateRig, p *Player, creature EntityID) {
	until := r.at(time.Hour)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	for _, ae := range []*ActiveEffect{
		{SpellID: 201, Name: "Withering Plague", Caster: creature, Until: until, Mods: StatMods{Sta: -125, DR: -125}, Rules: EffectRules{Harm: true}},
		{SpellID: 202, Name: "Recluse Venom", Caster: creature, Until: until, HPTick: -300, npcFoe: creature},
		{SpellID: 203, Name: "Duelist's Bane", Until: until, Mods: StatMods{Str: -5}, duelFoe: 4242},
		{SpellID: -109, Name: "Infernal Pact", Caster: p.ServerID, Until: until, Mods: StatMods{Str: -10}}, // a pact's price on its caster
		{SpellID: BoonEffectID, Name: "Boon of Freeport", Caster: p.ServerID, Until: r.at(4 * time.Hour)},
		{SpellID: 101, Name: "Courage", Until: until, Mods: StatMods{AC: 50, MaxHP: 40}, Line: "courage", Rank: 10},
		{SpellID: 102, Name: "Thistlecoat", Until: until, Shield: &Shield{Min: 10, Max: 12}},
		{SpellID: 103, Name: "Mending Breeze", Until: until, HPTick: 50, Ticks: 10},
		{SpellID: 104, Name: "Divine Intervention", Until: until, Rules: EffectRules{MitigatePct: 100, MitigateLeft: 1000000, MitigateCapped: true}},
		{SpellID: 105, Name: "Spirit of the Wolf", Until: until, Speed: true},
		{SpellID: 106, Name: "Talimus's Adamantine Barrier", Until: until, Root: true, Rules: EffectRules{MitigatePct: 65, MitigateLeft: 15000, MitigateCapped: true}},
		{SpellID: 107, Name: "Lich", Until: until, Form: 0x0E030048, HPTick: -196, Mods: StatMods{Int: 30}},
		{SpellID: FoodEffectID, Name: "Field Rations", Until: until, Mods: StatMods{HoT: 5}},
		{SpellID: 108, Name: "Mystic Glyph", Until: until}, // a buff whose effect the server does not read
	} {
		r.w.addEffectLocked(p, ae, r.at(0))
	}
}

func effectNames(p *Player) []string {
	var out []string
	for _, e := range p.Effects {
		out = append(out, e.Name)
	}
	return out
}

func slotNames(p *Player) []string {
	var out []string
	for _, b := range p.Buffs {
		out = append(out, b.Name)
	}
	return out
}

// On a player: from its first into the fight, then on its clock, a strip's 0x00B1 from the creature
// (no cast time, a harmful spell's stand-in visual); at its landing every buff is off it, each as its
// time running out ends it (its slot, its stats, its hold and speed told in a 0x00F8, its body), the
// harm and the Boon left on, a GM's #testmode off; told in a line (the names past the line's room
// counted) and the god mode in another. The next finds nothing and says so. The control, the same
// effects on a player fought by a creature of no template, keeps them all, and its god mode.
func TestATemplatesStripTakesEveryBuff(t *testing.T) {
	r := newTemplateRig(t)
	guard := stripTemplate(t, r, stripLanding)
	brute := r.npc("Gnoll Brute")
	hero, sess := r.player("Hero", guard.X+1, guard.Z)
	ctl, csess := r.player("Control", brute.X+1, brute.Z)
	for _, p := range []*Player{hero, ctl} {
		wearEverything(r, p, guard.ServerID)
		p.GMTestMode = true
		if p.SpeedNow() != 0 || len(p.Effects) != 14 || len(p.Buffs) != MaxEffects {
			t.Fatalf("%s wears %d effects in %d slots, speed %v (the barrier's hold: 0)", p.Name, len(p.Effects), len(p.Buffs), p.SpeedNow())
		}
	}
	r.fight(guard, hero.ServerID)
	r.fight(brute, ctl.ServerID)
	drain(sess)
	drain(csess)
	strips := func(msgs [][]byte) []b1 {
		var out []b1
		for _, c := range castsIn(msgs) {
			if c.caster == r.objectID(guard.ServerID) && c.fx == fxDamage {
				out = append(out, c)
			}
		}
		return out
	}

	r.tick(0)
	r.tick(29 * time.Second)
	if got := strips(drain(sess)); len(got) != 0 {
		t.Fatalf("a strip before its first: %+v", got)
	}
	r.tick(30 * time.Second)
	if got := strips(drain(sess)); len(got) != 1 || got[0].ms != 0 {
		t.Fatalf("its cast at 30 s: %+v, want one of no cast time", got)
	}
	if len(hero.Effects) != 14 {
		t.Fatalf("taken off before its landing: %q", effectNames(hero))
	}
	r.tick(30*time.Second + castLand)
	msgs := drain(sess)
	lines := linesOf(msgs)
	want := []string{
		"Gnoll Guard's Royal Decree strips your buffs: Courage, Thistlecoat, Mending Breeze, Divine Intervention, Spirit of the Wolf, " +
			"Talimus's Adamantine Barrier, Lich and 2 more.",
		"Gnoll Guard's Royal Decree strips your god mode: #testmode is off.",
	}
	if got := strings.Join(lines, "|"); got != strings.Join(want, "|") {
		t.Errorf("its lines:\n%q\nwant\n%q", lines, want)
	}
	if got, want := strings.Join(effectNames(hero), "|"), strings.Join(kept, "|"); got != want {
		t.Errorf("left on: %q, want %q", got, want)
	}
	if got, want := strings.Join(slotNames(hero), "|"), strings.Join(kept, "|"); got != want {
		t.Errorf("its slots: %q, want %q", got, want)
	}
	if hero.GMTestMode {
		t.Error("its god mode is still on")
	}
	if want := (StatMods{Str: -15, Sta: -125, DR: -125}); hero.Mods != want {
		t.Errorf("its stats' changes %+v, want the harm's alone %+v", hero.Mods, want)
	}
	if hero.SpeedNow() != hero.Speed || !hero.StatsDirty {
		t.Errorf("speed %v (want its own %v, the hold and the wolf gone), dirty %v", hero.SpeedNow(), hero.Speed, hero.StatsDirty)
	}
	told := false
	for _, m := range msgs {
		told = told || opOfMsg(m) == protocol.OpcodePlayerSpeed
	}
	if !told {
		t.Error("no 0x00F8: its client still holds it")
	}
	if model, _ := hero.Body(); model == 0x0E030048 {
		t.Error("still in the Lich's body")
	}

	// the control keeps everything, and says nothing of a strip
	if len(ctl.Effects) != 14 || len(ctl.Buffs) != MaxEffects || !ctl.GMTestMode {
		t.Errorf("the control: %q, %d slots, god mode %v", effectNames(ctl), len(ctl.Buffs), ctl.GMTestMode)
	}
	if got := strings.Join(linesOf(drain(csess)), "|"); strings.Contains(got, "strips") {
		t.Errorf("the control was told %q", got)
	}

	// the next, on its clock, finds nothing to take
	r.tick(59 * time.Second)
	if got := strips(drain(sess)); len(got) != 0 {
		t.Fatalf("a strip before its clock came round: %+v", got)
	}
	r.tick(60 * time.Second)
	r.tick(60*time.Second + castLand)
	msgs = drain(sess)
	if got := strips(msgs); len(got) != 1 {
		t.Errorf("its second cast: %+v", got)
	}
	if got := strings.Join(linesOf(msgs), "|"); got != "Gnoll Guard's Royal Decree finds no buffs on you." {
		t.Errorf("the second's lines: %q", got)
	}
	if got, want := strings.Join(effectNames(hero), "|"), strings.Join(kept, "|"); got != want {
		t.Errorf("after the second: %q, want %q", got, want)
	}
}

// On a pet a creature fights: the pet's buffs end as at their time (its maximum hit points taken
// back), a harm on it left, its owner told.
func TestATemplatesStripAtAPet(t *testing.T) {
	r := newTemplateRig(t)
	guard := stripTemplate(t, r, stripLanding)
	owner, osess := r.player("Owner", guard.X+20, guard.Z)
	pet := &NPC{ServerID: 3000, Name: "skeleton", Level: 20, HP: 100000, MaxHP: 100000, X: guard.X + 1.5, Y: guard.Y, Z: guard.Z, Owner: owner.ServerID}
	r.w.NPCs[pet.ServerID] = pet
	r.w.SpatialIndex.Insert(pet.ServerID, pet.X, pet.Z)
	pet.Effects = []*ActiveEffect{
		{SpellID: 301, Name: "Avatar", Caster: owner.ServerID, Until: r.at(time.Hour), Mods: StatMods{MaxHP: 500, Str: 50}},
		{SpellID: 302, Name: "Weaken", Until: r.at(time.Hour), Mods: StatMods{AC: -40}, Rules: EffectRules{Harm: true}},
	}
	pet.syncEffectMaxHP()
	pet.HP = pet.MaxHP
	if pet.MaxHP != 100500 {
		t.Fatalf("the pet's maximum with Avatar: %d", pet.MaxHP)
	}
	r.fight(guard, pet.ServerID)
	drain(osess)
	r.tick(0)
	r.tick(30 * time.Second)
	r.tick(30*time.Second + castLand)
	if len(pet.Effects) != 1 || pet.Effects[0].Name != "Weaken" {
		t.Errorf("the pet keeps %d effects: %+v", len(pet.Effects), pet.Effects)
	}
	if pet.MaxHP != 100000 || pet.HP > pet.MaxHP {
		t.Errorf("the pet's hit points %d/%d: want Avatar's 500 taken back", pet.HP, pet.MaxHP)
	}
	if got := strings.Join(linesOf(drain(osess)), "|"); got != "Gnoll Guard's Royal Decree strips your pet's buffs: Avatar." {
		t.Errorf("its owner was told %q", got)
	}
}

// A strip that pins a spell takes that record's name, cast time and visual, and nothing else of it:
// the spell's own debuff does not land. One named for none is called Dispel.
func TestATemplatesStripTakesASpellsLook(t *testing.T) {
	r := newTemplateRig(t)
	if _, err := r.db.Exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast,
		eqprequire, spellname, spelldesc) VALUES (7001, 60, 0, 0, 16880, 2, 0, 0, 5, 1, 0, 0, 'Searing Dread', '')`); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (7001, 'Target, Decrease AGI by 100, 2min', 99, 'test')`); err != nil {
		t.Fatal(err)
	}
	guard := stripTemplate(t, r, `[{"kind":"strip","spell_id":7001,"every":30,"first":5}]`)
	hero, sess := r.player("Hero", guard.X+1, guard.Z)
	r.w.mu.Lock()
	r.w.addEffectLocked(hero, &ActiveEffect{SpellID: 101, Name: "Courage", Until: r.at(time.Hour), Mods: StatMods{AC: 50}}, r.at(0))
	r.w.mu.Unlock()
	r.fight(guard, hero.ServerID)
	drain(sess)
	r.tick(0)
	r.tick(5 * time.Second)
	var cast []b1
	for _, c := range castsIn(drain(sess)) {
		if c.caster == r.objectID(guard.ServerID) && c.fx == 99 {
			cast = append(cast, c)
		}
	}
	if len(cast) != 1 || cast[0].ms != 2000 {
		t.Fatalf("its cast: %+v, want the spell's 2 s and visual", cast)
	}
	r.tick(7*time.Second + castLand)
	if got := strings.Join(linesOf(drain(sess)), "|"); got != "Gnoll Guard's Searing Dread strips your buffs: Courage." {
		t.Errorf("its line: %q", got)
	}
	if hero.hasEffect(7001) || hero.hasEffect(101) || hero.Mods.Agi != 0 {
		t.Errorf("after it: %q, AGI %+d (the spell's own debuff must not land)", effectNames(hero), hero.Mods.Agi)
	}
	if s := stripSpell(database.TemplateAbility{Kind: database.AbilityStrip}); s.Name != "Dispel" || s.visual() != fxDamage || s.CastTime != 0 {
		t.Errorf("a strip of no name: %q, visual %#x, %v cast", s.Name, s.visual(), s.CastTime)
	}
}

// What a strip takes and what it leaves, one effect at a time: a buff whose price harmfulEffect
// would keep goes whole; a price that gives nothing, a harmful spell's, a foe's and the Boon stay.
func TestWhatAStripTakes(t *testing.T) {
	for _, c := range []struct {
		name string
		ae   ActiveEffect
		take bool
	}{
		{"a stat buff", ActiveEffect{Mods: StatMods{AC: 50}}, true},
		{"a damage shield", ActiveEffect{Shield: &Shield{Min: 5, Max: 5}}, true},
		{"a heal over time", ActiveEffect{HPTick: 40}, true},
		{"power over time", ActiveEffect{PowerTick: 10}, true},
		{"an invulnerability", ActiveEffect{Rules: EffectRules{MitigatePct: 100}}, true},
		{"a run speed", ActiveEffect{Speed: true}, true},
		{"invisibility", ActiveEffect{Invis: true}, true},
		{"an illusion", ActiveEffect{Form: 0x1234}, true},
		{"a proc", ActiveEffect{Proc: 30, ProcMax: 40}, true},
		{"a proc's heal", ActiveEffect{Rules: EffectRules{ProcHeal: 50}}, true},
		{"enduring breath", ActiveEffect{Rules: EffectRules{Breath: true}}, true},
		{"a disguise", ActiveEffect{Rules: EffectRules{Disguise: true}}, true},
		{"a meal", ActiveEffect{SpellID: FoodEffectID, Mods: StatMods{HoT: 5}}, true},
		{"a vampire's frenzy", ActiveEffect{Mods: StatMods{Melee: 20}}, true},
		{"a buff the server does not read", ActiveEffect{}, true},
		{"a barrier with its hold", ActiveEffect{Root: true, Rules: EffectRules{MitigatePct: 65}}, true},
		{"a form with its tick", ActiveEffect{Form: 0x1234, HPTick: -196}, true},
		{"a stance's gain and price", ActiveEffect{Mods: StatMods{Str: 40, AC: -30}}, true},
		{"the Eye line's ceiling with its stats", ActiveEffect{Mods: StatMods{Wis: 140}, PowerCapped: true}, true},
		{"a pact's price", ActiveEffect{Mods: StatMods{Str: -10}}, false},
		{"a ceiling alone", ActiveEffect{PowerCapped: true}, false},
		{"a hold alone", ActiveEffect{Root: true}, false},
		{"a slow alone", ActiveEffect{Rules: EffectRules{SpeedPct: -30}}, false},
		{"a harmful spell's", ActiveEffect{Mods: StatMods{Agi: -100}, Rules: EffectRules{Harm: true}}, false},
		{"a harmful spell's that reads as nothing", ActiveEffect{Rules: EffectRules{Harm: true}}, false},
		{"a creature's damage over time", ActiveEffect{HPTick: -300, npcFoe: 77}, false},
		{"a duel's harm", ActiveEffect{Mods: StatMods{Str: -5}, duelFoe: 9}, false},
		{"the Boon", ActiveEffect{SpellID: BoonEffectID}, false},
	} {
		if got := beneficialEffect(&c.ae); got != c.take {
			t.Errorf("%s: taken %v, want %v", c.name, got, c.take)
		}
	}
}

// A strip's line names what went within its room and counts the rest; #stats says a strip, its
// clock and what it does.
func TestAStripsWords(t *testing.T) {
	if got := stripList([]string{"A"}); got != "A" {
		t.Errorf("one: %q", got)
	}
	long := make([]string, 40)
	for i := range long {
		long[i] = fmt.Sprintf("Buff number %02d", i)
	}
	got := stripList(long)
	if len(got) > stripListMost+len(" and 40 more") || !strings.HasSuffix(got, " more") {
		t.Errorf("forty: %d characters, %q", len(got), got)
	}
	r := newTemplateRig(t)
	guard := stripTemplate(t, r, stripLanding)
	_, sess := r.player("GM", guard.X+1, guard.Z)
	lines := strings.Join(r.w.StatsReport(sess, r.objectID(guard.ServerID), r.at(0)), "\n")
	if want := "strip Royal Decree (0s cast) every 30s: every buff off what it fights, and a GM's #testmode"; !strings.Contains(lines, want) {
		t.Errorf("#stats lacks %q:\n%s", want, lines)
	}
}
