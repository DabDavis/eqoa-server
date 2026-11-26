package entity

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"testing"
	"time"
)

// The line caches (linecache.go) change nothing: a line read from the cache
// is its text's reading, a spell's lines follow every change of its figures,
// a merc's sum of its effects every change of its effects, and a scene of
// mercs buffing, healing and keeping a pet, with a spell and an effect edited
// in place mid-run, sends and decides tick for tick what it does with the
// caches off. The controls: with the memos' checks off (lineMemoTrusted) the
// edits go unseen, so the checks are what keeps them right.

// withLineCaches runs f with the caches off (off) or on, and with the spell
// memo's and the sum memo's checks off (spellTrusted, modsTrusted), put back
// after.
func withLineCaches(off, spellTrusted, modsTrusted bool, f func()) {
	o, st, mt := lineCachesOff, spellMemoTrusted, modsMemoTrusted
	lineCachesOff, spellMemoTrusted, modsMemoTrusted = off, spellTrusted, modsTrusted
	defer func() { lineCachesOff, spellMemoTrusted, modsMemoTrusted = o, st, mt }()
	f()
}

// lineCorpus is lines as the server makes them (BuffLine, EffectLine,
// DebuffLine of spells of every kind and scope) and texts no spell makes.
func lineCorpus() []string {
	texts := []string{
		"Target, Increase STR by 20, 30min", "Group, Increase AGI by 10, Radius 30, 30min",
		"Self, Increase AC by 40, Increase STA by 10, 30min", "Target, Increase DR by 10, 30min",
		"Target, Increase DR by 10, Increase PR by 10, 30min", "Target, Increase FR by 20, Increase CR by 20, Increase DR by 20, Increase LR by 20, Increase PR by 20, 30min",
		"Target, Decrease AGI by 10, Increase STR by 15, 10min", "Target, Increase movement speed by 30%, 30min",
		"Target, Decrease STR by 20, 2min", "Target, Decrease AC by 30, Decrease AGI by 5, 1min",
		"Pet, Increase AGI by 30, Increase AC by 70, 30min", "Increase Hitpoints by 100",
		"Summons a Fire Elemental to aid you", "Target, Increase maximum hit points by 200, 30min",
	}
	var out []string
	for i, tx := range texts {
		for scope := 0; scope <= 4; scope++ {
			s := bookSpell(800000+i, fmt.Sprintf("S%d", i), scope, 0, 0, 1, tx)
			out = append(out, BuffLine(&s), EffectLine(&s), DebuffLine(&s, i%15), creatureLine(&s, 3))
			s.Def.Workbook, s.Def.Stack = true, 1+i%3
			out = append(out, buffLineOf(&s), effectLineOf(&s, buffLineOf(&s)))
			s.RecastGroup = "Group " + fmt.Sprint(i%2)
			out = append(out, buffLineOf(&s), effectLineOf(&s, buffLineOf(&s)))
		}
	}
	return append(out, "", " ", "x", "player", "stack", "stack1", "stack1 player", "stack0 player +STR", "stackx player +STR",
		"stack-2 pet +AC", "player ^s2", "player +STR ^sx", "pet +AC@self ^s1 +STR", "recast:G_1 player", "  player   +STR  ")
}

func TestParseLineCacheIsTheTextsReading(t *testing.T) {
	for _, l := range lineCorpus() {
		want, wantOK := parseLineText(l)
		for range 3 { // the first read and the kept ones
			got, ok := parseLine(l)
			if ok != wantOK || !reflect.DeepEqual(got, want) {
				t.Fatalf("%q: %+v %v, the text reads %+v %v", l, got, ok, want, wantOK)
			}
			if cap(got.keys) != len(got.keys) {
				t.Fatalf("%q: kept keys with room to append into (cap %d len %d)", l, cap(got.keys), len(got.keys))
			}
		}
	}
	// a reading's category is categoryKeys'
	for _, a := range lineCorpus() {
		pa, _ := parseLine(a)
		for _, b := range lineCorpus() {
			pb, _ := parseLine(b)
			ka, kb := categoryKeys(pa.keys, pb.keys)
			ga, gb := categoryParts(pa, pb)
			if !slices.Equal(ka, ga) || !slices.Equal(kb, gb) {
				t.Fatalf("%q / %q: categoryParts %v %v, categoryKeys %v %v", a, b, ga, gb, ka, kb)
			}
		}
	}
	// every pair weighed as before
	ls := lineCorpus()
	var mismatch int
	for _, a := range ls {
		for _, b := range ls {
			var sa, na, sb, nb bool
			withLineCaches(true, false, false, func() { sa, na = sameLine(a, b), nestedLines(a, b) })
			sb, nb = sameLine(a, b), nestedLines(a, b)
			if sa != sb || na != nb {
				mismatch++
				t.Errorf("%q / %q: same %v nested %v, uncached %v %v", a, b, sb, nb, sa, na)
			}
		}
	}
	if mismatch > 0 {
		t.Fatalf("%d pairs differ", mismatch)
	}
}

// A spell's kept lines follow each of its figures edited in place; the
// control: unchecked (lineMemoTrusted) they do not.
func TestSpellLinesFollowAnEditedSpell(t *testing.T) {
	s := bookSpell(810001, "Tribal Brawn", 1, 0, 0, 20, "Target, Increase STR by 20, 30min")
	edits := []struct {
		what string
		do   func(s *CastSpell)
	}{
		{"its text (a reload)", func(s *CastSpell) { s.Def = ParseSpellText("Target, Increase AGI by 10, Increase AC by 5, 30min") }},
		{"its scope", func(s *CastSpell) { s.Scope = 2 }},
		{"an effect's sign", func(s *CastSpell) { s.Def.Effects[0].Amount = -s.Def.Effects[0].Amount }},
		{"an effect's stat", func(s *CastSpell) { s.Def.Effects[0].Stat = StatWis }},
		{"an effect's self", func(s *CastSpell) { s.Def.Effects[0].Self = true }},
		{"an effect added", func(s *CastSpell) {
			s.Def.Effects = append(s.Def.Effects, SpellEffect{Kind: EffectSpeed, SowPct: 140})
		}},
		{"its workbook stack", func(s *CastSpell) { s.Def.Workbook, s.Def.Stack = true, 2 }},
		{"its recast group", func(s *CastSpell) { s.RecastGroup = "Brawn Line" }},
		{"its form", func(s *CastSpell) { s.Def.AltersLook = true }},
		{"its kind (a summoning)", func(s *CastSpell) { s.Def = ParseSpellText("Summons a Fire Elemental to aid you") }},
	}
	for _, e := range edits {
		before := EffectLine(&s)
		e.do(&s)
		fresh := buffLineOf(&s)
		if got := BuffLine(&s); got != fresh {
			t.Errorf("after %s: BuffLine %q, made again %q", e.what, got, fresh)
		}
		if got, want := EffectLine(&s), effectLineOf(&s, fresh); got != want {
			t.Errorf("after %s: EffectLine %q, made again %q", e.what, got, want)
		}
		if before == EffectLine(&s) && e.what != "its form" && e.what != "its kind (a summoning)" {
			t.Logf("after %s the line is unchanged: %q", e.what, before)
		}
	}
	// the control: the memo unchecked keeps the line of the spell before its edit
	c := bookSpell(810002, "Breeze", 1, 0, 0, 20, "Target, Increase AGI by 10, 30min")
	old := EffectLine(&c)
	c.Def = ParseSpellText("Target, Increase STA by 10, 30min")
	withLineCaches(false, true, false, func() {
		if got := EffectLine(&c); got != old {
			t.Fatalf("unchecked, the memo made the line again (%q): the control shows nothing", got)
		}
	})
	if got := EffectLine(&c); got == old {
		t.Fatalf("checked, the edited spell kept its old line %q", old)
	}
}

// A merc's kept sum of its effects follows every change of them; the
// control: unchecked, it does not.
func TestEffectModsMemoFollowsTheEffects(t *testing.T) {
	ward := func(id int, line string, mods StatMods) *ActiveEffect {
		return &ActiveEffect{SpellID: id, Line: line, Mods: mods}
	}
	a := ward(1, "player +DR ^s1", StatMods{DR: 10})
	b := ward(2, "player +DR +PR ^s1", StatMods{DR: 15, PR: 15})
	c := ward(3, "player +STR ^s1", StatMods{Str: 20})
	var m effectModsMemo
	effects := []*ActiveEffect{a, b, c}
	check := func(what string) {
		t.Helper()
		if got, want := m.sum(effects), sumEffectMods(effects); got != want {
			t.Fatalf("after %s: kept %+v, summed %+v", what, got, want)
		}
	}
	check("the first sum")
	check("nothing")
	b.Mods.DR = 30
	check("a Mods edited in place")
	a.Line = "player +DR +PR ^s1"
	check("a Line edited in place")
	effects = []*ActiveEffect{a, c}
	check("an effect off")
	effects = append(effects, ward(4, "player +DR ^s1", StatMods{DR: 50}))
	check("an effect on")
	effects[0] = ward(1, "player +DR ^s1", StatMods{DR: 10})
	check("an effect replaced by a copy of another")
	effects = []*ActiveEffect{}
	check("no effects")
	// the control: unchecked, a change goes unseen
	effects = []*ActiveEffect{a, b}
	m.sum(effects)
	b.Mods.PR += 100
	withLineCaches(false, false, true, func() {
		if m.sum(effects) == sumEffectMods(effects) {
			t.Fatal("unchecked, the memo summed again: the control shows nothing")
		}
	})
	check("unchecked's change, checked")
}

// mercScene is a merc team's run: a shaman (a target buff, a group buff, a
// heal), a cleric (nested resist wards, a self buff, a heal) and a
// necromancer with its pet and its pet book, a second player beside them out
// of the group, the players hurt twice; at 25 s the shaman's target buff is
// edited in place to a weaker form of the group buff's line (a reload of its
// spell) and taken off the owner, and at 40 s the cleric's buffs have their
// Mods raised in place. It returns a digest a tick of what every session was
// sent and every player's, merc's and pet's state.
func mercScene(t *testing.T) [][32]byte {
	t.Helper()
	r := newMercRig(t)
	_, mate := r.player("Mate", 2, 0)
	brawn := bookSpell(700901, "Tribal Brawn", 1, 3*time.Second, 0, 20, "Target, Increase STR by 20, 30min")
	brawn.Level = mercOwnerLevel
	breeze := bookSpell(700902, "Breeze", 2, 3*time.Second, 0, 20, "Group, Increase AGI by 10, Radius 30, 30min")
	breeze.Level = mercOwnerLevel - 2
	mend := bookSpell(700903, "Mend", 1, 2*time.Second, 2*time.Second, 10, "Increase Hitpoints by 300")
	dward := bookSpell(700904, "Disease Ward", 1, 2*time.Second, 0, 10, "Target, Increase DR by 15, 30min")
	dward.Level = 20
	pward := bookSpell(700905, "Poison Ward", 1, 2*time.Second, 0, 10, "Target, Increase DR by 20, Increase PR by 20, 30min")
	pward.Level = 26
	aegis := bookSpell(700906, "Aegis", 0, 2*time.Second, 0, 10, "Self, Increase AC by 40, Increase STA by 10, 30min")
	aegis.Level = 28
	summon := bookSpell(717, "Bone Servant", 0, 4*time.Second, 45*time.Second, 30, "Summons a Fire Elemental to aid you")
	summon.Level = 28
	r.w.mu.Lock()
	r.w.putNPCLocked(&NPC{ServerID: 4900, Name: "Fire Elemental", Level: 28, HP: 2000, MaxHP: 2000, ModelID: 0x8A416241, Size: 1,
		NPCType: 0x80, X: 20000, Z: 20000, World: 0})
	r.w.mu.Unlock()
	hire := func(slot int, k MercKit) *mercState {
		t.Helper()
		if _, why, _ := r.w.HireMercAt(r.sess, slot, k, MercAssist, r.at(0)); why != "" {
			t.Fatalf("slot %d: %s", slot, why)
		}
		r.w.mu.Lock()
		_, up := r.w.invites[r.p.ServerID][r.p.ServerID]
		r.w.mu.Unlock()
		if up {
			r.w.GroupAccept(r.sess, r.sess.PlayerObjectID())
		}
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		for _, ms := range r.w.mercsOfLocked(r.p) {
			if ms.slot == slot {
				return ms
			}
		}
		t.Fatalf("no merc in slot %d", slot)
		return nil
	}
	shamanKit := r.kit(8, 3, 1, brawn, breeze, mend)
	shamanKit.Name = "Aliela"
	shaman := hire(1, shamanKit)
	clericKit := r.kit(9, 0, 0, mend, dward, pward, aegis)
	clericKit.Name = "Joranusis"
	cleric := hire(2, clericKit)
	necroKit := r.kit(11, 0, 0, append([]CastSpell{summon}, necroPetBook()...)...)
	necroKit.Name = "Garoris"
	necro := hire(3, necroKit)
	_ = mate // beside the camp, out of the group (a fourth member would bench the necromancer)

	var out [][32]byte
	for d := time.Duration(0); d <= 60*time.Second; d += 100 * time.Millisecond {
		r.w.mu.Lock()
		switch d {
		case 5 * time.Second, 31 * time.Second:
			r.p.HP = r.p.MaxHPNow() / 3 // hurt: the heals
			mate.HP = mate.MaxHPNow() / 2
		case 25 * time.Second:
			// its spell reloaded with another text, in place: a weaker form of
			// the group buff's line, which the owner has on, so with it read
			// again the shaman does not put it back up when it is taken off
			// (and read as it was, +STR, it would)
			for i := range shaman.kit.Spells {
				if s := &shaman.kit.Spells[i]; s.SpellID == brawn.SpellID {
					s.Def = ParseSpellText("Target, Increase AGI by 5, 30min")
				}
			}
			r.w.removeEffectLocked(r.p, brawn.SpellID)
		case 40 * time.Second:
			for _, e := range cleric.npc.Effects {
				e.Mods.DR += 7 // an effect's figures raised in place
			}
		}
		for _, ms := range []*mercState{shaman, cleric, necro} {
			ms.npc.Power = ms.npc.MaxPower // casts not held back by power
		}
		r.w.mu.Unlock()
		r.tick(d)
		out = append(out, mercSceneDigest(r))
	}
	return out
}

// mercSceneDigest is what the rig's sessions were sent since the last (taken)
// and its players', mercs' and pets' state.
func mercSceneDigest(r *mercRig) [32]byte {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	h := sha256.New()
	ids := make([]EntityID, 0, len(r.w.Players)+len(r.w.NPCs))
	for id := range r.w.Players {
		ids = append(ids, id)
	}
	for id := range r.w.NPCs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	effects := func(es []*ActiveEffect) {
		for _, e := range es {
			fmt.Fprintf(h, " [%d %q %d %+v %d]", e.SpellID, e.Line, e.Rank, e.Mods, e.Until.UnixNano())
		}
	}
	for _, id := range ids {
		if p, ok := r.w.Players[id]; ok {
			fmt.Fprintf(h, "P%d %d/%d %d/%d %+v", id, p.HP, p.MaxHPNow(), p.Power, p.MaxPower, p.Mods)
			effects(p.Effects)
			if sess := r.w.sessionOfLocked(p); sess != nil {
				for _, m := range sess.Queue().GetPending() {
					fmt.Fprintf(h, "%x", m.Data())
				}
				sess.Queue().Clear()
			}
		}
		if n, ok := r.w.NPCs[id]; ok {
			// (not its place: which follow place each merc takes goes by a
			// map's order, and every spell here reaches across the camp)
			fmt.Fprintf(h, "N%d %d/%d %d/%d %v %d", id, n.HP, n.MaxHP, n.Power, n.MaxPower, n.Dead, n.Owner)
			effects(n.Effects)
			if n.merc != nil {
				fmt.Fprintf(h, " %+v", n.merc.body.Mods)
				ready := make([]int, 0, len(n.merc.ready))
				for id := range n.merc.ready {
					ready = append(ready, id)
				}
				sort.Ints(ready)
				for _, id := range ready {
					fmt.Fprintf(h, " r%d@%d", id, n.merc.ready[id].UnixNano())
				}
			}
		}
		fmt.Fprintln(h)
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum
}

// The merc scene is the same tick for tick with the caches on as off; the
// controls: with the spell memo's checks off the in-place edit of the
// shaman's spell (25 s) goes unseen, with the sum memo's every new effect
// does, and the scene is not. A run with the caches off twice is the same
// (the scene repeats).
func TestMercLineCachesChangeNothing(t *testing.T) {
	var off, off2, on [][32]byte
	withLineCaches(true, false, false, func() { off = mercScene(t) })
	withLineCaches(true, false, false, func() { off2 = mercScene(t) })
	if !reflect.DeepEqual(off, off2) {
		t.Fatal("the scene does not repeat with the caches off: no comparison can be made")
	}
	withLineCaches(false, false, false, func() { on = mercScene(t) })
	for i := range off {
		if on[i] != off[i] {
			t.Fatalf("tick %d (%v): the caches changed what was sent or decided", i, time.Duration(i)*100*time.Millisecond)
		}
	}
	for _, c := range []struct {
		name               string
		spell, mods        bool
		notBefore, notPast time.Duration
	}{
		{"the spell memo unchecked", true, false, 25 * time.Second, 60 * time.Second},
		{"the sum memo unchecked", false, true, 0, 60 * time.Second},
	} {
		var trusted [][32]byte
		withLineCaches(false, c.spell, c.mods, func() { trusted = mercScene(t) })
		first := -1
		for i := range off {
			if trusted[i] != off[i] {
				first = i
				break
			}
		}
		at := time.Duration(first) * 100 * time.Millisecond
		if first < 0 || at < c.notBefore || at > c.notPast {
			t.Errorf("control, %s: first differs at tick %d (%v), want within %v-%v", c.name, first, at, c.notBefore, c.notPast)
			continue
		}
		t.Logf("control, %s: first differs at %v", c.name, at)
	}
}
