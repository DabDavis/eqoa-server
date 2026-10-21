package entity

import "testing"

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

// TestCalculateMaxHP holds the formula to Sandstorm's base maximum hit
// points (stat channel byte 148) as measured 2026-10-21, one case for each
// different (class, level, stamina) among the characters with no mastery
// that adds hit points: the stamina is all of it, worn and from spells.
func TestCalculateMaxHP(t *testing.T) {
	tests := []struct {
		class, level, stamina, want int
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
	for _, tt := range tests {
		if got := CalculateMaxHP(tt.level, tt.class, tt.stamina); got != tt.want {
			t.Errorf("CalculateMaxHP(level %d, class %d, STA %d) = %d, Sandstorm's %d", tt.level, tt.class, tt.stamina, got, tt.want)
		}
	}
}

// TestCalculateMaxPower holds the formula to Sandstorm's base maximum power
// (stat channel byte 156, of the base stats), measured on the same
// characters: one case for each different (class, level, base stats),
// every class but the ranger and the shaman (none was measured).
func TestCalculateMaxPower(t *testing.T) {
	tests := []struct {
		class, level int
		stats        [7]int // STR STA AGI DEX WIS INT CHA
		want         int
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
	for _, tt := range tests {
		s := tt.stats
		if got := CalculateMaxPower(tt.level, tt.class, s[0], s[1], s[2], s[3], s[4], s[5], s[6]); got != tt.want {
			t.Errorf("CalculateMaxPower(level %d, class %d, %v) = %d, Sandstorm's %d", tt.level, tt.class, s, got, tt.want)
		}
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

// TestRaceScaleUsesNetworkEnum pins raceScaleTable to the network race enum.
//
// The table was originally laid out on the old ordering (5=Halfling, 6=Troll,
// 7=Barbarian) after the rest of the server had moved to the network enum
// (5=Troll, 6=Barbarian, 7=Halfling), so Trolls rendered at 0.60 and Halflings
// at 1.10 — each with the other's factor, and nothing failed. RaceScale feeds
// Player.Size and NPC.Size, i.e. ModelSize on the wire, so the only signal was
// how the model looked in-world.
func TestRaceScaleUsesNetworkEnum(t *testing.T) {
	// Anchor the enum in a table that is independently keyed on it, by bonuses
	// that DISCRIMINATE between the two orderings: only the Troll's STR is +20
	// of the three, only the Halfling's DEX +15. If the enum is ever re-swapped,
	// it fails here rather than in the scale assertions.
	if raceBonus[5][0] != 20 { // the Troll's STR +20
		t.Fatal("raceBonus[5] is not the Troll's — race 5 is not Troll?")
	}
	if raceBonus[7][3] != 15 { // the Halfling's DEX +15
		t.Fatal("raceBonus[7] is not the Halfling's — race 7 is not Halfling?")
	}

	// Big races scale up, small races scale down. Values are the model-height
	// derivations in stats.go; what this guards is which row they sit on.
	tests := []struct {
		race int
		name string
		want float32
	}{
		{3, "Gnome", 0.45},
		{4, "Dwarf", 0.78},
		{5, "Troll", 1.20},
		{6, "Barbarian", 1.10},
		{7, "Halfling", 0.60},
		{9, "Ogre", 1.50},
	}
	for _, tt := range tests {
		if got := RaceScale(tt.race); got != tt.want {
			t.Errorf("RaceScale(%d) [%s] = %v, want %v", tt.race, tt.name, got, tt.want)
		}
	}

	// Out of range (including race 10, which has no row) falls back to 1.0.
	for _, race := range []int{-1, 10, 255} {
		if got := RaceScale(race); got != 1.0 {
			t.Errorf("RaceScale(%d) = %v, want 1.0 fallback", race, got)
		}
	}
}
