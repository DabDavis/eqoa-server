package entity

import "testing"

// A use's line may name its visual ("Visual 0x..."): the clause is read off the line before its words are
// (its effects as the line's without it), and a thrown one's 0x00B1 from its user to its target plays it
// in place of its kind's stand-in, by Use and by a worn one's auto-attack throw (Live petition 5173: the
// magician's Lava Stone, Fingers of Flame's projectile).
func TestItemUseVisual(t *testing.T) {
	const fireBolt uint32 = 0x889D7E79 // Fingers of Flame's
	for _, tc := range []struct {
		line, words string
		fx          uint32
	}{
		{"Target, Decrease Hitpoints by 214, Visual 0x889D7E79", "Target, Decrease Hitpoints by 214", fireBolt},
		{"Target, Visual 2292022905, Decrease Hitpoints by 214", "Target, Decrease Hitpoints by 214", fireBolt},
		{"Target; visual 0x889d7e79.", "Target", fireBolt},
		{"Target, Decrease Hitpoints by 214", "Target, Decrease Hitpoints by 214", 0},
		{"Visual 0x889D7E79", "Visual 0x889D7E79", 0}, // never the first clause
	} {
		fx, words := readVisual(tc.line)
		if fx != tc.fx || words != tc.words {
			t.Errorf("%q: %#x %q, want %#x %q", tc.line, fx, words, tc.fx, tc.words)
		}
	}
	with := NewPotionEffect(705299, "Lava Stone", "Target, Decrease Hitpoints by 214, Visual 0x889D7E79", "", 0, "", 0, 0)
	plain := NewPotionEffect(705299, "Lava Stone", "Target, Decrease Hitpoints by 214", "", 0, "", 0, 0)
	if with.FX != fireBolt || len(with.Def.Unread) != 0 || len(with.Def.Effects) != 1 || with.Def.Effects[0].Amount != -214 ||
		!with.Def.Harmful || !with.Thrown() {
		t.Fatalf("read: fx %#x, def %+v", with.FX, with.Def)
	}
	if s := with.castSpell(); s.visual() != fireBolt {
		t.Errorf("its spell plays %#x", s.visual())
	}
	if s := plain.castSpell(); s.visual() == fireBolt || s.visual() == 0 {
		t.Errorf("the control plays %#x, want its kind's stand-in", s.visual())
	}

	for _, pe := range []PotionEffect{with, plain} {
		c := newCaster(t)
		c.npc.HP = 1000
		if r := c.use(t, pe, c.npcO, 0); r != "" {
			t.Fatalf("refused: %q", r)
		}
		casts := castsIn(c.sent())
		if len(casts) != 1 || casts[0].caster != c.sess.PlayerObjectID() || casts[0].target != c.npcO {
			t.Fatalf("0x00B1s %+v", casts)
		}
		if want := pe.castSpell(); casts[0].fx != want.visual() || (pe.FX != 0 && casts[0].fx != fireBolt) {
			t.Errorf("fx %#x, line's %#x", casts[0].fx, pe.FX)
		}
		if c.npc.HP != 1000-214 {
			t.Errorf("the creature at %d", c.npc.HP)
		}
	}

	// a drink's visual on its drinker; none without one
	for _, line := range []string{"Self, Increase Hitpoints by 5, Visual 0x889D7E79", "Self, Increase Hitpoints by 5"} {
		c := newCaster(t)
		pe := NewPotionEffect(705300, "Draught", line, "", 0, "", 0, 0)
		if r := c.use(t, pe, 0, 0); r != "" {
			t.Fatalf("refused: %q", r)
		}
		n := 0
		for _, b := range castsIn(c.sent()) {
			if b.fx == fireBolt {
				n++
			}
		}
		if want := map[bool]int{true: 1, false: 0}[pe.FX != 0]; n != want {
			t.Errorf("%q: %d of its visual, want %d", line, n, want)
		}
	}

	// a worn one's auto-attack throw plays it too; without one, none (the client's swing alone)
	for _, pe := range []PotionEffect{with, plain} {
		c := newCaster(t)
		c.npc.HP = 1000
		c.w.mu.Lock()
		c.w.potions.note(pe)
		c.p.Swing.Potion, c.p.Swing.PotionLeft = pe.PatternID, 5
		c.w.thrownPotionHitLocked(c.sess, c.p, c.npc, c.npcO)
		c.w.mu.Unlock()
		casts := castsIn(c.sent())
		switch {
		case pe.FX != 0 && (len(casts) != 1 || casts[0].fx != fireBolt || casts[0].target != c.npcO):
			t.Errorf("auto-attack throw with a visual: %+v", casts)
		case pe.FX == 0 && len(casts) != 0:
			t.Errorf("auto-attack throw without one: %+v", casts)
		}
		if c.npc.HP >= 1000 {
			t.Errorf("auto-attack throw: the creature at %d", c.npc.HP)
		}
	}
}
