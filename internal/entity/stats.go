package entity

import (
	"math"
	"math/rand"
	"strconv"
)

// RaceScale returns the visual scale factor for a player race.
// All humanoid models in CHAR.ESF are authored at ~1.8-2.2m height.
// The scale adjusts them to correct proportional sizes at runtime.
// Race IDs are the NETWORK enum, the same one defaultBaseStats below and
// handlers.Race use: 0=Human, 1=Elf, 2=DarkElf, 3=Gnome, 4=Dwarf, 5=Troll,
// 6=Barbarian, 7=Halfling, 8=Erudite, 9=Ogre.
//
// This table used to be laid out on the OLD ordering (5=Halfling, 6=Troll,
// 7=Barbarian) while every other race-keyed table in the server had already
// moved to the network enum, so Trolls were scaled 0.60 and Halflings 1.10 —
// each wearing the other's factor. The scales themselves are unchanged; only
// the rows they sit on moved. This reaches the wire: world.go feeds it to
// Player.Size / NPC.Size, which is ModelSize at state-channel offset 36-39.
var raceScaleTable = [10]float32{
	1.0,  // 0: Human (reference)
	1.0,  // 1: Elf
	1.0,  // 2: Dark Elf
	0.45, // 3: Gnome (small — ~3ft, model is 2.2m)
	0.78, // 4: Dwarf (~4.5ft)
	1.20, // 5: Troll (tall, model is 1.87m)
	1.10, // 6: Barbarian (large, model is 1.87m)
	0.60, // 7: Halfling (~3.5ft, model is 1.78m)
	1.0,  // 8: Erudite
	1.50, // 9: Ogre (large — ~9ft, model is 1.88m)
}

// RaceScale returns the visual scale for a given race index.
func RaceScale(race int) float32 {
	if race >= 0 && race < len(raceScaleTable) {
		return raceScaleTable[race]
	}
	return 1.0
}

// raceScaleFromString parses a race string (e.g. "3") and returns the scale.
func raceScaleFromString(race string) float32 {
	r, err := strconv.Atoi(race)
	if err != nil {
		return 1.0
	}
	return RaceScale(r)
}

// baseStats holds default base stats for a race/class combination.
type baseStats struct {
	Str, Sta, Agi, Dex, Wis, Int, Cha int
}

// raceClass is the lookup key for default base stats.
type raceClass struct {
	Race, Class int
}

// defaultBaseStats maps race/class to starting base stats.
// Source: C# ReturnHome DefaultCharacter table (eqoabase_schema.sql).
// HumanType/Sex variants share the same stats per race/class in practice.
var defaultBaseStats = map[raceClass]baseStats{
	// Human (0) — Eastern (humType 1)
	{0, 8}:  {65, 65, 60, 55, 80, 55, 75}, // Shaman
	{0, 5}:  {60, 75, 80, 65, 55, 55, 65}, // Bard
	{0, 14}: {55, 65, 60, 65, 75, 80, 55}, // Alchemist
	{0, 0}:  {75, 80, 65, 65, 60, 55, 55}, // Warrior
	{0, 13}: {65, 60, 65, 75, 55, 80, 55}, // Wizard
	{0, 3}:  {75, 80, 65, 65, 55, 60, 55}, // ShadowKnight
	{0, 6}:  {65, 65, 75, 80, 55, 55, 60}, // Rogue
	{0, 11}: {65, 75, 55, 65, 60, 80, 55}, // Necromancer
	{0, 10}: {55, 65, 75, 60, 65, 80, 55}, // Magician
	{0, 12}: {55, 55, 65, 65, 60, 80, 75}, // Enchanter
	{0, 1}:  {65, 65, 75, 80, 60, 55, 55}, // Ranger (Western)
	{0, 2}:  {75, 80, 65, 65, 60, 55, 55}, // Paladin (Western)
	{0, 4}:  {65, 75, 65, 80, 60, 55, 55}, // Monk (Western)
	{0, 7}:  {65, 55, 55, 75, 80, 60, 65}, // Druid (Western)
	{0, 9}:  {0, 0, 0, 0, 0, 0, 0},        // Cleric — not available for Human

	// Elf (1)
	{1, 14}: {50, 60, 55, 70, 80, 80, 60}, // Alchemist
	{1, 5}:  {55, 70, 75, 70, 60, 55, 70}, // Bard
	{1, 8}:  {60, 60, 55, 60, 85, 55, 80}, // Shaman
	{1, 7}:  {60, 50, 50, 80, 85, 60, 70}, // Druid
	{1, 12}: {50, 50, 60, 70, 65, 80, 80}, // Enchanter
	{1, 10}: {50, 60, 70, 65, 70, 80, 60}, // Magician
	{1, 2}:  {70, 85, 60, 70, 65, 55, 60}, // Paladin
	{1, 1}:  {60, 60, 70, 85, 65, 55, 60}, // Ranger
	{1, 6}:  {60, 60, 70, 85, 60, 55, 65}, // Rogue
	{1, 9}:  {60, 55, 60, 80, 60, 80, 60}, // Cleric

	// Dark Elf (2)
	{2, 14}: {50, 65, 55, 70, 75, 90, 50}, // Alchemist
	{2, 8}:  {60, 65, 55, 60, 80, 65, 70}, // Shaman
	{2, 12}: {50, 55, 60, 70, 60, 90, 70}, // Enchanter
	{2, 10}: {50, 65, 70, 65, 65, 90, 50}, // Magician
	{2, 11}: {60, 75, 50, 70, 60, 90, 50}, // Necromancer
	{2, 6}:  {60, 65, 70, 85, 55, 65, 55}, // Rogue
	{2, 3}:  {70, 80, 60, 70, 55, 70, 50}, // ShadowKnight
	{2, 0}:  {70, 80, 60, 70, 60, 65, 50}, // Warrior
	{2, 13}: {60, 60, 60, 80, 55, 90, 50}, // Wizard

	// Gnome (3)
	{3, 8}:  {70, 70, 55, 55, 85, 50, 70}, // Shaman (old entry)
	{3, 2}:  {80, 85, 60, 65, 65, 50, 50}, // Paladin
	{3, 0}:  {70, 75, 70, 70, 55, 65, 50}, // Warrior (2nd entry)
	{3, 6}:  {60, 60, 80, 85, 50, 65, 55}, // Rogue (2nd entry)
	{3, 14}: {50, 60, 65, 70, 70, 90, 50}, // Alchemist
	{3, 12}: {50, 50, 70, 70, 55, 90, 70}, // Enchanter
	{3, 10}: {50, 60, 80, 65, 60, 90, 50}, // Magician
	{3, 11}: {60, 70, 60, 70, 55, 90, 50}, // Necromancer
	{3, 13}: {60, 55, 70, 80, 50, 90, 50}, // Wizard

	// Dwarf (4)
	{4, 0}:  {90, 90, 60, 60, 55, 50, 50}, // Warrior
	{4, 3}:  {90, 90, 60, 60, 50, 55, 50}, // ShadowKnight
	{4, 8}:  {70, 85, 60, 55, 75, 50, 60}, // Shaman

	// Troll (5)
	{5, 8}:  {65, 80, 60, 60, 75, 55, 60}, // Shaman
	{5, 0}:  {65, 80, 60, 60, 75, 55, 60}, // Warrior
	{5, 3}:  {65, 80, 60, 60, 70, 60, 60}, // ShadowKnight
	{5, 6}:  {75, 70, 70, 80, 50, 55, 55}, // Rogue

	// Halfling (7)
	{7, 7}:  {60, 60, 60, 85, 75, 55, 60}, // Druid
	{7, 6}:  {60, 70, 80, 90, 50, 50, 55}, // Rogue
	{7, 0}:  {70, 85, 70, 75, 55, 50, 50}, // Warrior
	{7, 8}:  {60, 70, 65, 65, 75, 50, 70}, // Shaman

	// Erudite (8)
	{8, 14}: {50, 60, 55, 65, 80, 95, 50}, // Alchemist
	{8, 8}:  {60, 60, 55, 55, 85, 70, 70}, // Shaman
	{8, 12}: {50, 50, 60, 65, 65, 95, 70}, // Enchanter
	{8, 10}: {50, 60, 70, 60, 70, 95, 50}, // Magician
	{8, 2}:  {70, 75, 60, 65, 65, 70, 50}, // Paladin
	{8, 3}:  {70, 75, 60, 65, 60, 75, 50}, // ShadowKnight
	{8, 11}: {60, 70, 50, 65, 65, 95, 50}, // Necromancer
	{8, 9}:  {60, 55, 60, 75, 60, 95, 50}, // Cleric

	// Ogre (9)
	{9, 3}:  {90, 90, 60, 60, 50, 55, 50}, // ShadowKnight
	{9, 0}:  {90, 90, 60, 60, 55, 50, 50}, // Warrior
	{9, 8}:  {70, 85, 60, 55, 75, 50, 60}, // Shaman
	{9, 11}: {80, 85, 50, 60, 55, 75, 50}, // Necromancer
}

// LookupBaseStats returns the base stats for a race/class combo.
// Falls back to a generic default if the combo isn't in the table.
func LookupBaseStats(race, class int) baseStats {
	if bs, ok := defaultBaseStats[raceClass{race, class}]; ok {
		return bs
	}
	// Fallback for unknown combos
	return baseStats{60, 60, 60, 60, 60, 60, 60}
}

// hpKClass returns the HP scaling constant K for a given class.
// SOE retail formula: HP = level * (4.167 * STA * K + 19.2)
// Verified exact: Rogue=0.090355, Alchemist=0.044058
// Other classes estimated from C# ratio (tank:melee:healer:caster = 2.4:1.6:1.3:1.0)
func hpKClass(class int) float64 {
	switch class {
	case 0, 2, 3: // Warrior, Paladin, ShadowKnight (tank)
		return 0.1057 // estimated: 0.044058 * 2.4
	case 1, 4, 5, 6: // Ranger, Monk, Bard, Rogue (melee DPS)
		return 0.090355 // VERIFIED from Corsten (L60 Rogue, HPFactor=1274)
	case 7, 8, 9: // Druid, Shaman, Cleric (healer)
		return 0.0573 // estimated: 0.044058 * 1.3
	case 10, 11, 12, 13, 14: // Magician, Necromancer, Enchanter, Wizard, Alchemist (caster)
		return 0.044058 // VERIFIED from Lostologist (L45 Alchemist, HPFactor=228)
	default:
		return 0.0441
	}
}

// CalculateMaxHP computes max HP from level, class, and stamina.
// SOE retail formula: HP = level * (4.167 * STA * K_class + 19.2)
// Verified exact against Corsten (L60 Rogue, HP=6459) and Lostologist (L45 Alch, HP=1814).
func CalculateMaxHP(level, class, stamina int) int {
	k := hpKClass(class)
	hp := float64(level) * (4.167*float64(stamina)*k + 19.2)
	result := int(math.Floor(hp))
	if result < 1 {
		return 1
	}
	return result
}

// powerStats returns the two stat values used for power calculation for a class.
// Source: C# Entity.Stats.cs CalculatePower()
func powerStats(class, str, sta, agi, dex, wis, intel, cha int) (primary, secondary int) {
	switch class {
	case 0, 2, 3: // Warrior, Paladin, ShadowKnight
		return str, sta
	case 1, 6: // Ranger, Rogue
		return agi, dex
	case 4, 5: // Monk, Bard
		return agi, wis
	case 7: // Druid
		return wis, dex
	case 8: // Shaman
		return wis, sta
	case 9: // Cleric
		return wis, cha
	case 10: // Magician
		return intel, agi
	case 11: // Necromancer
		return intel, sta
	case 12: // Enchanter
		return intel, cha
	case 13: // Wizard
		return intel, dex
	case 14: // Alchemist
		return intel, wis
	default:
		return str, sta
	}
}

// powerBaseByClass returns the base power constant for a class.
// SOE retail: Power = ((base*100 + primary*10 + secondary*5) * level) / 100
// Verified: Rogue base=79 (AGI+DEX), Alchemist base≈32 (INT+WIS)
// Other classes estimated from C# ratio patterns.
func powerBaseByClass(class int) int {
	switch class {
	case 0, 2, 3: // Warrior, Paladin, ShadowKnight (STR+STA) — high base
		return 70
	case 1, 6: // Ranger, Rogue (AGI+DEX) — high base
		return 79 // VERIFIED from Corsten
	case 4, 5: // Monk, Bard (AGI+WIS)
		return 60
	case 7: // Druid (WIS+DEX)
		return 45
	case 8: // Shaman (WIS+STA)
		return 45
	case 9: // Cleric (WIS+CHA)
		return 40
	case 10: // Magician (INT+AGI)
		return 35
	case 11: // Necromancer (INT+STA)
		return 35
	case 12: // Enchanter (INT+CHA)
		return 35
	case 13: // Wizard (INT+DEX)
		return 35
	case 14: // Alchemist (INT+WIS)
		return 32 // VERIFIED from Lostologist
	default:
		return 28
	}
}

// CalculateMaxPower computes max power from level, class, and stats.
// SOE retail formula: Power = ((base*100 + primary*10 + secondary*5) * level) / 100
// Verified against Corsten (Rogue, Power=5490) and Lostologist (Alch, Power=2481).
func CalculateMaxPower(level, class, str, sta, agi, dex, wis, intel, cha int) int {
	primary, secondary := powerStats(class, str, sta, agi, dex, wis, intel, cha)
	base := powerBaseByClass(class)
	pow := ((base*100 + primary*10 + secondary*5) * level) / 100
	if pow < 1 {
		return 1
	}
	return pow
}

// ---------------------------------------------------------------------------
// SOE Retail Combat Formulas (derived from Hagley test server save states)
// Verified exact against Corsten (L60 Rogue) and Lostologist (L45 Alchemist).
// See claude-notes/combat-formula-final.md for full derivation.
// ---------------------------------------------------------------------------

// CalculateOffMod computes the offensive modifier from base stats.
// OFF = floor((base_STR + (161/173) * base_DEX) * level / 143.93) + equip_bonus
// Exact coefficients derived from SOE retail: DEX weight = 161/173 ≈ 0.9306
func CalculateOffMod(baseStr, baseDex, level int) int {
	off := (float64(baseStr) + 161.0/173.0*float64(baseDex)) * float64(level) / 143.93
	result := int(math.Floor(off))
	if result < 1 {
		return 1
	}
	return result
}

// CalculateDefMod computes the defensive modifier from AC and base agility.
// DEF = floor(AC / 41.5 + base_AGI * level / 135.6) + equip_bonus
func CalculateDefMod(ac, baseAgi, level int) int {
	def := float64(ac)/41.5 + float64(baseAgi)*float64(level)/135.6
	return int(math.Floor(def))
}

// CalculateMeleeDamage computes melee auto-attack damage using the SOE retail formula.
// damage = floor(OFF * weapondamage * (1 + 0.02 * level_diff) / (1 + target_DEF / 3.68))
// Returns the damage as a positive integer (caller negates for DamageIndication).
func CalculateMeleeDamage(attackerOff, weaponDamage, attackerLevel, targetLevel, targetDef int) int {
	levelDiff := attackerLevel - targetLevel
	levelScale := 1.0 + 0.02*float64(levelDiff)
	if levelScale < 0.1 {
		levelScale = 0.1 // floor at 10% to avoid zero/negative
	}
	defScale := 1.0 + float64(targetDef)/3.68
	dmg := float64(attackerOff) * float64(weaponDamage) * levelScale / defScale
	result := int(math.Floor(dmg))
	if result < 1 {
		return 1 // minimum 1 damage
	}
	return result
}

// NPCMaxHP computes NPC max HP from level.
// L35 guard ≈ 2100 HP (player report), scales with level squared.
// Formula: HP = 60 * level (gives L35=2100, L60=3600, L10=600)
func NPCMaxHP(level int) int {
	hp := level * 60
	if hp < 10 {
		return 10
	}
	return hp
}

// NPCWeaponDamage returns a weapon damage value for an NPC based on level.
// SOE retail NPCs have weapondamage stored per-weapon in the item struct.
// Guard Jahn (L35) had weapondamage=4 → ratio 4/35 = 0.114.
// This approximation scales linearly with level for NPCs without item data.
func NPCWeaponDamage(level int) int {
	wd := (level + 4) / 9 // L10=1, L20=2, L35=4, L50=6, L60=7
	if wd < 1 {
		return 1
	}
	return wd
}

// NPCOffMod estimates an NPC's offensive modifier based on level.
// Uses the OFF formula with estimated NPC base stats (STR≈2*level, DEX≈2*level).
func NPCOffMod(level int) int {
	npcStr := level * 2
	npcDex := level * 2
	return CalculateOffMod(npcStr, npcDex, level)
}

// NPCDefMod estimates an NPC's defensive modifier based on level.
// Most basic NPCs have low DEF (guards had DEF≈0).
func NPCDefMod(level int) int {
	// Basic NPCs: minimal AC, low AGI
	npcAC := level * 10 // rough estimate
	npcAgi := level
	return CalculateDefMod(npcAC, npcAgi, level)
}

// RollWeaponDamage returns a random weapon damage roll for a player.
// Uses the weapondamage value from the equipped weapon item.
// If no weapon data available, estimates from level.
func RollWeaponDamage(weaponDamage int) int {
	if weaponDamage <= 0 {
		return 1
	}
	// Add ±20% variance to make combat feel natural
	variance := float64(weaponDamage) * 0.2
	min := float64(weaponDamage) - variance
	max := float64(weaponDamage) + variance
	return int(min + rand.Float64()*(max-min))
}
