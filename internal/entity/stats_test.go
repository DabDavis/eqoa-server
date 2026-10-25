package entity

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// TestCalculateOffMod verifies OFF MOD against SOE retail save state data.
// Source: Corsten (L60 Barbarian Rogue) and Lostologist (L45 Gnome Alchemist)
// from Hagley test server save states via PCSX2 0.9.7.
func TestCalculateOffMod(t *testing.T) {
	tests := []struct {
		name     string
		str, dex int
		level    int
		want     int
	}{
		{"Corsten L60 Rogue", 235, 80, 60, 129},
		{"Lostologist L45 Alchemist", 50, 70, 45, 36},
		{"L1 minimum", 60, 60, 1, 1},                // should be at least 1
		{"L35 NPC (STR=70 DEX=70)", 70, 70, 35, 32}, // Guard Jahn estimate
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateOffMod(tt.str, tt.dex, tt.level)
			if got != tt.want {
				t.Errorf("CalculateOffMod(%d, %d, %d) = %d, want %d", tt.str, tt.dex, tt.level, got, tt.want)
			}
		})
	}
}

// TestCalculateDefMod verifies DEF MOD against SOE retail data.
func TestCalculateDefMod(t *testing.T) {
	tests := []struct {
		name    string
		ac, agi int
		level   int
		want    int
	}{
		{"Corsten L60 Rogue", 3174, 85, 60, 114},
		{"Lostologist L45 Alchemist", 1264, 65, 45, 52},
		{"No armor L1", 0, 60, 1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateDefMod(tt.ac, tt.agi, tt.level)
			if got != tt.want {
				t.Errorf("CalculateDefMod(%d, %d, %d) = %d, want %d", tt.ac, tt.agi, tt.level, got, tt.want)
			}
		})
	}
}

// TestCalculateMeleeDamage verifies damage formula against SOE retail combat data.
func TestCalculateMeleeDamage(t *testing.T) {
	tests := []struct {
		name                                  string
		attackerOff, weaponDmg                int
		attackerLevel, targetLevel, targetDef int
		want                                  int
	}{
		// Corsten (OFF=129, weapondamage=41) → Guard Jahn (L35, DEF≈0)
		// 129 × 41 × 1.5 / 1.0 = 7933.5 → 7933
		{"Corsten→Guard Jahn", 129, 41, 60, 35, 0, 7933},

		// Guard Jahn (OFF≈32, weapondamage=4) → Corsten (L60, DEF=114)
		// 32 × 4 × 0.5 / (1 + 114/3.68) = 2.001 → 2
		{"Guard Jahn→Corsten", 32, 4, 35, 60, 114, 2},

		// Equal level, no defense
		{"Same level no DEF", 100, 10, 50, 50, 0, 1000},

		// Minimum damage floor
		{"Minimum damage", 1, 1, 1, 60, 100, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateMeleeDamage(tt.attackerOff, tt.weaponDmg, tt.attackerLevel, tt.targetLevel, tt.targetDef)
			if got != tt.want {
				t.Errorf("CalculateMeleeDamage(%d, %d, %d, %d, %d) = %d, want %d",
					tt.attackerOff, tt.weaponDmg, tt.attackerLevel, tt.targetLevel, tt.targetDef, got, tt.want)
			}
		})
	}
}

// floorRat is floor(x), x an exact fraction: the wiki's formulas worked
// without any rounding error, to hold the integer forms to.
func floorRat(x *big.Rat) int {
	q := new(big.Int).Quo(x.Num(), x.Denom())
	return int(q.Int64())
}

// TestCalculateMaxHP holds the formula to the live game's (eqoa.fandom.com
// Formulas): (HP factor + STA/11) x level, STA/11 a real division, the whole
// truncated. The wiki's worked example: a level 60 priest's factor 13 (its
// caster with Hearty 1 and 2) and 400 stamina, 2961 (its text rounds 400/11
// to 36.4 and says 2964). Then every (class, level, stamina) of Sandstorm's
// measured characters with no hit point mastery, against the exact fraction;
// Sandstorm's own values (stat channel byte 148) are kept beside them, 11 of
// 25 equal. The control: Sandstorm's rounding (STA/11 to a whole number
// first) misses the wiki's example.
func TestCalculateMaxHP(t *testing.T) {
	if got := CalculateMaxHP(60, 9, 400); got != 2961 {
		t.Errorf("the wiki's example: %d, want 2961", got)
	}
	tests := []struct {
		class, level, stamina, sandstorm int
	}{
		{0, 1, 85, 32}, {0, 1, 80, 31}, {0, 51, 515, 3621}, // warrior
		{2, 1, 80, 31},                  // paladin
		{3, 1, 110, 34}, {3, 2, 90, 64}, // shadow knight
		{6, 1, 70, 22},                  // rogue
		{4, 5, 100, 125},                // monk
		{5, 51, 515, 3213},              // bard
		{9, 1, 80, 20}, {9, 7, 98, 154}, // cleric
		{7, 23, 128, 575}, {7, 23, 120, 552}, {7, 23, 103, 506}, // druid
		{14, 1, 65, 16},                       // alchemist
		{13, 6, 86, 108},                      // wizard
		{12, 15, 54, 225}, {12, 49, 105, 980}, // enchanter
	}
	same := 0
	for _, tt := range tests {
		x := new(big.Rat).SetFrac64(int64(hpFactor(tt.class)*11+tt.stamina), 11)
		want := floorRat(x.Mul(x, big.NewRat(int64(tt.level), 1)))
		got := CalculateMaxHP(tt.level, tt.class, tt.stamina)
		if got != want {
			t.Errorf("CalculateMaxHP(level %d, class %d, STA %d) = %d, the wiki's %d", tt.level, tt.class, tt.stamina, got, want)
		}
		if got == tt.sandstorm {
			same++
		}
	}
	t.Logf("%d of %d as Sandstorm's", same, len(tests))
	if sandstorm := 60 * (13 + (400+5)/11); sandstorm == 2961 {
		t.Fatal("the control: Sandstorm's rounding must miss the wiki's example")
	}
}

// TestCalculateMaxPower holds the formula to the live game's
// (eqoa.fandom.com Formulas): (28 + primary/10 + secondary/20) x level,
// truncated, the pair the class's (powerStats). The wiki's worked example: a
// level 31 magician of 267 INT and 204 AGI, 2011 (its arithmetic writes 206
// AGI and gets 2011.9). Then the stats of Sandstorm's measured characters,
// against the exact fraction; Sandstorm's values (byte 156, rounded half up)
// kept beside them. The control: rounding half up gives the example 2012.
func TestCalculateMaxPower(t *testing.T) {
	if got := CalculateMaxPower(31, 10, 50, 50, 204, 50, 50, 267, 50); got != 2011 {
		t.Errorf("the wiki's example: %d, want 2011", got)
	}
	tests := []struct {
		class, level int
		stats        [7]int // STR STA AGI DEX WIS INT CHA
		sandstorm    int
	}{
		{0, 1, [7]int{85, 85, 60, 65, 55, 55, 50}, 41},
		{0, 1, [7]int{70, 80, 60, 70, 60, 65, 50}, 40},
		{0, 51, [7]int{239, 254, 222, 209, 60, 60, 55}, 3333},
		{2, 1, [7]int{75, 80, 65, 65, 60, 55, 55}, 40},
		{3, 1, [7]int{90, 110, 60, 60, 50, 55, 50}, 44},
		{3, 2, [7]int{90, 90, 60, 60, 50, 55, 50}, 83},
		{3, 50, [7]int{75, 204, 60, 351, 55, 232, 50}, 2608},
		{4, 5, [7]int{65, 100, 65, 80, 60, 55, 55}, 205},
		{5, 51, [7]int{65, 301, 207, 70, 60, 60, 227}, 3251},
		{6, 1, [7]int{75, 70, 70, 80, 50, 55, 55}, 40},
		{7, 23, [7]int{60, 100, 100, 105, 75, 55, 60}, 937},
		{7, 23, [7]int{60, 103, 100, 105, 75, 55, 60}, 937},
		{9, 1, [7]int{60, 80, 65, 60, 75, 65, 70}, 39},
		{9, 7, [7]int{60, 98, 65, 60, 75, 65, 70}, 273},
		{10, 45, [7]int{55, 229, 194, 60, 65, 80, 55}, 2057},
		{11, 51, [7]int{80, 268, 50, 60, 55, 75, 306}, 2494},
		{12, 15, [7]int{50, 50, 60, 109, 65, 95, 90}, 630},
		{12, 49, [7]int{50, 50, 397, 65, 65, 95, 71}, 2011},
		{13, 6, [7]int{60, 86, 60, 80, 55, 90, 50}, 246},
		{14, 1, [7]int{50, 65, 55, 70, 75, 90, 50}, 41},
		{14, 32, [7]int{55, 165, 158, 65, 85, 80, 55}, 1288},
	}
	same := 0
	for _, tt := range tests {
		s := tt.stats
		p, q := powerStats(tt.class, s[0], s[1], s[2], s[3], s[4], s[5], s[6])
		x := new(big.Rat).SetFrac64(int64(560+2*p+q), 20)
		want := floorRat(x.Mul(x, big.NewRat(int64(tt.level), 1)))
		got := CalculateMaxPower(tt.level, tt.class, s[0], s[1], s[2], s[3], s[4], s[5], s[6])
		if got != want {
			t.Errorf("CalculateMaxPower(level %d, class %d, %v) = %d, the wiki's %d", tt.level, tt.class, s, got, want)
		}
		if got == tt.sandstorm {
			same++
		}
	}
	t.Logf("%d of %d as Sandstorm's", same, len(tests))
	if halfUp := ((2800+10*267+5*204)*31 + 50) / 100; halfUp == 2011 {
		t.Fatal("the control: rounding half up must miss the wiki's example")
	}
}

// TestLookupBaseStats holds the starting stats to Sandstorm's level 1
// characters, less the 20 points each spent (a character that put them all
// in STA shows it 20 over). The Barbarian Warrior is the row Return Home's
// table has wrong.
func TestLookupBaseStats(t *testing.T) {
	tests := []struct {
		race, class int
		want        baseStats
	}{
		{6, 0, baseStats{85, 85, 60, 65, 55, 55, 50}},  // Barbarian Warrior
		{6, 6, baseStats{75, 70, 70, 80, 50, 55, 55}},  // Barbarian Rogue
		{2, 0, baseStats{70, 80, 60, 70, 60, 65, 50}},  // Dark Elf Warrior
		{2, 14, baseStats{50, 65, 55, 70, 75, 90, 50}}, // Dark Elf Alchemist
		{0, 2, baseStats{75, 80, 65, 65, 60, 55, 55}},  // Human Paladin
		{5, 3, baseStats{90, 90, 60, 60, 50, 55, 50}},  // Troll Shadow Knight
		{3, 9, baseStats{60, 60, 65, 60, 75, 65, 70}},  // Gnome Cleric (Sandstorm's 20 more STA)
	}
	for _, tt := range tests {
		if got := LookupBaseStats(tt.race, tt.class); got != tt.want {
			t.Errorf("LookupBaseStats(race %d, class %d) = %v, Sandstorm's %v", tt.race, tt.class, got, tt.want)
		}
	}
}

// TestNPCOffMod verifies NPC OFF estimation matches Guard Jahn data.
func TestNPCOffMod(t *testing.T) {
	// Guard Jahn L35: OFF≈32 (derived from damage=2 back-calculation)
	got := NPCOffMod(35)
	if got != 32 {
		t.Errorf("NPCOffMod(35) = %d, want 32", got)
	}
}

// TestSizesAreSandstormsNotScaledByRace: an NPC sends its row's size and a
// player 1.0, whatever the race (F3 20261025-172359, -173632: the gnome
// bankers and Psova Brinkbar drawn at under half size). Sandstorm's captures
// carry 1.00 for players of every race and each NPC's own value (a gnome NPC
// at 1.00 and one at 1.20). The control: the race factors this replaced (a
// gnome 0.45, an ogre 1.5) fail every row below but the human's.
func TestSizesAreSandstormsNotScaledByRace(t *testing.T) {
	for _, tt := range []struct {
		race string
		size float32
		want float32
	}{
		{"3", 1.0, 1.0},   // Psova Brinkbar, a gnome body
		{"3", 1.2, 1.2},   // Sandstorm's Craigwold, a gnome at 1.20
		{"9", 1.0, 1.0},   // an ogre
		{"4", 0.75, 0.75}, // a dwarf row with its own size
		{"0", 2.0, 2.0},   // the Tests Program
		{"3", 0, 1.0},     // no size: 1.0
		{"6", 1e-40, 1.0}, // a denormal (Return Home's dump has 234)
	} {
		rec := &database.NPCRecord{ID: 1, NPCName: "Sized", Race: tt.race, Size: tt.size, NPCLevel: 10}
		if got := NPCFromRecord(rec, 0, nil).Size; got != tt.want {
			t.Errorf("race %s size %v: NPC sent at %v, want %v", tt.race, tt.size, got, tt.want)
		}
	}
	w := NewWorld()
	for race := 0; race <= 10; race++ {
		p := w.CreatePlayer(nil, &database.CharacterRecord{ServerID: uint32(100 + race), CharName: fmt.Sprintf("Race%d", race), Race: race, Level: 10})
		if p == nil || p.Size != 1.0 {
			t.Errorf("race %d: player sent at %v, want 1.0", race, p.Size)
		}
	}
}
