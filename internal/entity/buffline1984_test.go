package entity

import (
	"strings"
	"testing"
	"time"
)

// Ticket 1984 (Deathcore, "bards 54 sow+invis is not overwriting 35 sow"):
// Selo's Song of Travel (54: invis, speed) over Selo's Accelerando (35:
// speed) lands and takes the 35 off, its bearer invisible; so does Lila's
// Refrain (46). The reverse: the 35 over the 54 adds nothing and is refused,
// its caster told. The rows are the database's.
func TestSongOfTravelOverAccelerando(t *testing.T) {
	noResist(t)
	acc := wbSpell(184, 35, 2, "Selo's Accelerando", `{"v":1,"scope":"group","target_type":"Friendly","harmful":false,"duration_s":900,"stack":3,"level":35,"min_level":21,"radius":30,"effects":[{"id":"00C7","type":"Increase Speed","kind":"speed"}]}`)
	travel := wbSpell(700698, 54, 2, "Selo's Song of Travel", `{"v":1,"scope":"group","target_type":"Friendly","harmful":false,"duration_s":1200,"stack":3,"level":54,"min_level":0,"radius":30,"effects":[{"id":"00E7","type":"Invisible","kind":"invis","dispel_on_cast":true,"dispel_on_contact":true},{"id":"00E8","type":"Increase Speed","kind":"speed","amount":130,"dispel_on_cast":true,"dispel_on_contact":true}]}`)
	lila := wbSpell(199, 46, 2, "Lila's Refrain", `{"v":1,"scope":"group","target_type":"Friendly","harmful":false,"duration_s":900,"stack":3,"level":46,"min_level":27,"radius":30,"effects":[{"id":"00D8","type":"Increase Speed","kind":"speed","amount":130,"dispel_on_cast":true,"dispel_on_contact":true},{"id":"00D9","type":"Invisible","kind":"invis","dispel_on_cast":true,"dispel_on_contact":true}]}`)
	if !nestedLines(EffectLine(&acc), EffectLine(&travel)) {
		t.Fatalf("lines %q %q: not nested", EffectLine(&acc), EffectLine(&travel))
	}
	for _, over := range []CastSpell{travel, lila} {
		c := newCaster(t)
		c.castOn(t, acc, 0)
		c.castOn(t, over, 10*time.Second)
		if got := c.effectNames(); strings.Join(got, ",") != over.Name || !c.p.Invisible() || c.p.MoveRate() != moveRateMax {
			t.Errorf("%s over Accelerando: %v, invisible %v, rate %d", over.Name, got, c.p.Invisible(), c.p.MoveRate())
		}
	}

	// the reverse: the 35 under the 54 adds nothing
	c := newCaster(t)
	c.castOn(t, travel, 0)
	c.castOn(t, acc, 10*time.Second)
	if got := c.effectNames(); strings.Join(got, ",") != travel.Name {
		t.Fatalf("Accelerando over Song of Travel: %v", got)
	}
	if red := redLines(c.sess); len(red) == 0 || !strings.Contains(red[len(red)-1], "Selo's Song of Travel, which is stronger") {
		t.Errorf("not told: %q", red)
	}
}
