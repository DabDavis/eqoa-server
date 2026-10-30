package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// classHit is what one NPC melee hit (the dice at their top) does to a player
// of the class at level 50 under the balance.
func classHit(t *testing.T, b *Balance, class int) int {
	t.Helper()
	c := newCaster(t)
	c.w.balance = b
	c.p.Class, c.p.Level, c.p.HP, c.p.MaxHP = class, 50, 100000, 100000
	npc := &NPC{ServerID: 2003, Name: "a test orc", Level: 52, HP: 5000, MaxHP: 5000, X: 101, Y: 10, Z: 100}
	c.w.NPCs[npc.ServerID] = npc
	was := meleeIntn
	meleeIntn = func(n int) int { return n - 1 }
	defer func() { meleeIntn = was }()
	c.w.mu.Lock()
	c.w.npcAttackPlayer(npc, c.p.ServerID, c.at(time.Second))
	c.w.mu.Unlock()
	return 100000 - c.p.HP
}

// A ranger, a bard and a rogue take their class's share of an NPC's melee
// (70, 80 and 65%, the seed of 2026-10-29) where a warrior and a wizard take
// it all; the control: with no class rows every class takes the whole hit.
func TestAClassTakesItsShareOfNPCMelee(t *testing.T) {
	seed := SeedBalance().WithClasses([]database.BalanceClass{{Class: 1, MeleeTakenPct: 70}, {Class: 5, MeleeTakenPct: 80},
		{Class: 6, MeleeTakenPct: 65}}).WithClassLevels(nil)
	whole := SeedBalance().WithClasses(nil).WithClassLevels(nil)
	for _, tc := range []struct {
		class int
		pct   float64
	}{{0, 100}, {13, 100}, {1, 70}, {5, 80}, {6, 65}} {
		full := classHit(t, whole, tc.class)
		got := classHit(t, seed, tc.class)
		if want := pct(full, tc.pct); got != want || full <= 0 {
			t.Errorf("class %d: %d of a %d hit, want %d (%g%%)", tc.class, got, full, want, tc.pct)
		}
	}
	if a, b := classHit(t, whole, 1), classHit(t, whole, 0); a == 0 || b == 0 {
		t.Errorf("no hit landed: %d %d", a, b)
	}
	// a row's share is the GM's: 50 halves it
	half := SeedBalance().WithClasses([]database.BalanceClass{{Class: 6, MeleeTakenPct: 50}}).WithClassLevels(nil)
	if full, got := classHit(t, whole, 6), classHit(t, half, 6); got != pct(full, 50) {
		t.Errorf("a 50%% row: %d of %d", got, full)
	}
}

// Balance by class and level shapes a class's share by its level: the line
// between its rows, the end rows' past them, times Balance by class's
// figure; a class with no rows keeps its figure at every level (the
// control), and so does every class with no rows at all.
func TestAClassShareByLevel(t *testing.T) {
	b := SeedBalance().WithClasses([]database.BalanceClass{{Class: 6, MeleeTakenPct: 80}}).
		WithClassLevels([]database.BalanceClassLevel{{Class: 6, Level: 10, MeleeTakenPct: 100}, {Class: 6, Level: 50, MeleeTakenPct: 50}})
	for _, tc := range []struct {
		class, level int
		want         float64
	}{{6, 1, 80}, {6, 10, 80}, {6, 30, 60}, {6, 50, 40}, {6, 60, 40}, {0, 30, 100}} {
		if got := b.MeleeTakenPctAt(tc.class, tc.level); got != tc.want {
			t.Errorf("class %d level %d: %g%%, want %g%%", tc.class, tc.level, got, tc.want)
		}
	}
	if got := SeedBalance().WithClassLevels(nil).MeleeTakenPctAt(6, 30); got != SeedBalance().MeleeTakenPct(6) {
		t.Errorf("no class and level rows: %g%%, want Balance by class's %g%%", got, SeedBalance().MeleeTakenPct(6))
	}
	// a hit lands at the share: a rogue of 50 takes 40% of the whole
	whole := SeedBalance().WithClasses(nil).WithClassLevels(nil)
	full := classHit(t, whole, 6)
	if got := classHit(t, b, 6); got != pct(full, 40) {
		t.Errorf("a rogue of 50 took %d of %d, want 40%%", got, full)
	}
}

// A summoned pet's hit points take its owner's class and level share
// (balanceclasslevel's pet_hp_pct) on Balance by level's pet column: a level
// 50 magician's pet with a row of 150 at 40 and 200 at 60 has 175% of what
// the table alone gives it, a necromancer's (no rows) the table's; the
// control: a charmed creature (not summoned) keeps a creature's.
func TestAPetsHitPointsTakeItsOwnersClassShare(t *testing.T) {
	w := NewWorld()
	const classMagician = 10
	mag := &Player{ServerID: 1001, Class: classMagician, Level: 50, HP: 900, MaxHP: 900}
	nec := &Player{ServerID: 1002, Class: classNecromancer, Level: 50, HP: 900, MaxHP: 900}
	w.Players[mag.ServerID], w.Players[nec.ServerID] = mag, nec
	pet := &NPC{ServerID: 3000, Name: "Air Elemental", Level: 45, Owner: mag.ServerID}
	other := &NPC{ServerID: 3001, Name: "Ancient Death", Level: 45, Owner: nec.ServerID}
	charmed := &NPC{ServerID: 3002, Name: "a skeleton", Level: 45, Owner: nec.ServerID, Charm: &charmHold{}}
	for _, n := range []*NPC{pet, other, charmed} {
		n.HP, n.MaxHP = 1, 1 // whole: the balance keeps a share
		w.NPCs[n.ServerID] = n
	}
	b := testBalance().WithClassLevels([]database.BalanceClassLevel{
		{Class: classMagician, Level: 40, MeleeTakenPct: 100, PetHPPct: 150},
		{Class: classMagician, Level: 60, MeleeTakenPct: 100, PetHPPct: 200}})
	w.SetBalance(b)
	plain := pct(NPCMaxHP(45), b.Level(50).PetHPPct)
	if want := pct(plain, 175); pet.MaxHP != want || pet.HP != want {
		t.Errorf("the magician's pet: %d/%d hit points, want %d (175%% of %d)", pet.HP, pet.MaxHP, want, plain)
	}
	if other.MaxHP != plain {
		t.Errorf("the necromancer's pet: %d hit points, want the table's %d", other.MaxHP, plain)
	}
	if charmed.MaxHP != b.NPCMaxHP(45) {
		t.Errorf("the charmed creature: %d hit points, want a creature's %d", charmed.MaxHP, b.NPCMaxHP(45))
	}
	if got := b.PetHPPctAt(classNecromancer, 50); got != 100 {
		t.Errorf("a class with no rows: %g%%, want 100", got)
	}
}

// A player's own damage on an NPC lands at its class's share at its level
// (balanceclasslevel's dealt_pct, after the con's): a wizard with a row of 150
// at 10 and 50 at 30 lands 150% at 1-10, the line's 100% at 20 and 50% from
// 30; a row made in code without the column is all of it. On the hits: a
// level 10 wizard's spell of 100 on an even NPC lands 150 and its melee hit
// half again what it lands with no rows; the control: a warrior of 10 (no
// rows) lands its spell's 100 and its whole melee hit.
func TestAClassDealtShareByLevel(t *testing.T) {
	b := SeedBalance().WithClassLevels([]database.BalanceClassLevel{
		{Class: 13, Level: 10, MeleeTakenPct: 100, DealtPct: 150}, {Class: 13, Level: 30, MeleeTakenPct: 100, DealtPct: 50}})
	for _, tc := range []struct {
		class, level int
		want         float64
	}{{13, 1, 150}, {13, 10, 150}, {13, 20, 100}, {13, 30, 50}, {13, 60, 50}, {0, 10, 100}} {
		if got := b.DealtPctAt(tc.class, tc.level); got != tc.want {
			t.Errorf("class %d level %d: dealt %g%%, want %g%%", tc.class, tc.level, got, tc.want)
		}
	}
	if got := SeedBalance().WithClassLevels([]database.BalanceClassLevel{{Class: 13, Level: 10, MeleeTakenPct: 80}}).DealtPctAt(13, 10); got != 100 {
		t.Errorf("a row without dealt_pct: %g%%, want all of it", got)
	}
	hits := func(b *Balance, class int) (spell, melee int) {
		c := newCaster(t)
		c.w.balance = b
		c.p.Class, c.p.Level = class, 10
		c.p.Gear.WeaponDamage, c.p.Gear.WeaponCond = 40, 1 // a whole weapon, whose hit is more than 1
		npc := &NPC{ServerID: 2003, Name: "a test orc", Level: 10, HP: 50000, MaxHP: 50000, X: 101, Y: 10, Z: 100}
		c.w.NPCs[npc.ServerID] = npc
		was := meleeIntn
		meleeIntn = func(n int) int { return n - 1 }
		defer func() { meleeIntn = was }()
		c.w.mu.Lock()
		defer c.w.mu.Unlock()
		c.w.spellHitLocked(c.p, c.sess, npc, 100)
		spell = 50000 - npc.HP
		npc.HP = 50000
		c.w.playerHitLocked(c.sess, c.p, npc, 0x01312E99)
		return spell, 50000 - npc.HP
	}
	plain := SeedBalance().WithClassLevels(nil)
	s, m := hits(b, 13)
	_, m0 := hits(plain, 13)
	if s != 150 || m != pct(m0, 150) || m0 < 2 {
		t.Errorf("a wizard of 10 at 150%%: its spell's 100 landed %d (want 150), its melee %d (want %d of %d)", s, m, pct(m0, 150), m0)
	}
	ws, wm := hits(b, 0)
	_, wm0 := hits(plain, 0)
	if ws != 100 || wm != wm0 {
		t.Errorf("the control, a warrior of 10 with no rows: its spell's 100 landed %d, its melee %d of %d", ws, wm, wm0)
	}
}
