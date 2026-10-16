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

// TestCalculateMaxHP verifies HP formula against SOE retail data.
func TestCalculateMaxHP(t *testing.T) {
	tests := []struct {
		name             string
		level, class     int
		stamina          int
		wantMin, wantMax int // allow ±2 for rounding
	}{
		// Corsten: L60 Rogue (class 6), BaseSTA=235, HP=6459
		{"Corsten L60 Rogue", 60, 6, 235, 6458, 6462},
		// Lostologist: L45 Alchemist (class 14), BaseSTA=115, HP=1814
		{"Lostologist L45 Alch", 45, 14, 115, 1813, 1815},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateMaxHP(tt.level, tt.class, tt.stamina)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("CalculateMaxHP(%d, %d, %d) = %d, want [%d, %d]",
					tt.level, tt.class, tt.stamina, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

// TestCalculateMaxPower verifies Power formula against SOE retail data.
func TestCalculateMaxPower(t *testing.T) {
	tests := []struct {
		name                                string
		level, class                        int
		str, sta, agi, dex, wis, intel, cha int
		wantMin, wantMax                    int
	}{
		// Corsten: L60 Rogue, base stats, BasePower=5490
		{"Corsten L60 Rogue", 60, 6, 235, 235, 85, 80, 195, 55, 55, 5489, 5491},
		// Lostologist: L45 Alch, base stats, BasePower=2481
		{"Lostologist L45 Alch", 45, 14, 50, 115, 65, 70, 291, 90, 50, 2470, 2500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateMaxPower(tt.level, tt.class, tt.str, tt.sta, tt.agi, tt.dex, tt.wis, tt.intel, tt.cha)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("CalculateMaxPower(%d, %d, ...) = %d, want [%d, %d]",
					tt.level, tt.class, got, tt.wantMin, tt.wantMax)
			}
		})
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
	// Anchor the enum in a table that is independently keyed on it. The classes
	// have to be ones that DISCRIMINATE between the two orderings: Shaman does
	// not (both Troll and Halfling have one), but ShadowKnight is Troll-only and
	// Druid is Halfling-only, so this pair is false under the old layout. If the
	// enum is ever re-swapped, it fails here rather than in the scale assertions.
	if _, ok := defaultBaseStats[raceClass{5, 3}]; !ok { // Troll ShadowKnight
		t.Fatal("defaultBaseStats has no race 5 ShadowKnight — race 5 is not Troll?")
	}
	if _, ok := defaultBaseStats[raceClass{7, 7}]; !ok { // Halfling Druid
		t.Fatal("defaultBaseStats has no race 7 Druid — race 7 is not Halfling?")
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
