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

// raceBonus is each race's seven stat bonuses (STR STA AGI DEX WIS INT CHA),
// gRaces' m_bonus in SUPPORT (decomp src/virace.cpp), on the network race
// enum; every race's total 35. The froglok's (10) is the S2's, a copy of
// Human's (re-tools/extend_s2_race.py).
var raceBonus = [...][7]int{
	{5, 5, 5, 5, 5, 5, 5},    // 0 Human
	{0, 0, 0, 10, 10, 5, 10}, // 1 Elf
	{0, 5, 0, 10, 5, 15, 0},  // 2 Dark Elf
	{0, 0, 10, 10, 0, 15, 0}, // 3 Gnome
	{10, 10, 0, 5, 10, 0, 0}, // 4 Dwarf
	{20, 15, 0, 0, 0, 0, 0},  // 5 Troll
	{15, 10, 0, 5, 0, 5, 0},  // 6 Barbarian
	{0, 10, 10, 15, 0, 0, 0}, // 7 Halfling
	{0, 0, 0, 5, 10, 20, 0},  // 8 Erudite
	{20, 15, 0, 0, 0, 0, 0},  // 9 Ogre
	{5, 5, 5, 5, 5, 5, 5},    // 10 Froglok
}

// classBonus is each class's, VIClassDef's m_bonus (decomp src/viclass.cpp).
// Each class has one +15 stat and one +10: the two its create page
// highlights (m_statMask), and its power's (powerStats).
var classBonus = [...][7]int{
	{10, 15, 0, 0, -5, -10, -10}, // 0 Warrior
	{0, 0, 10, 15, -5, -10, -10}, // 1 Ranger
	{10, 15, 0, 0, -5, -10, -10}, // 2 Paladin
	{10, 15, 0, 0, -10, -5, -10}, // 3 Shadow Knight
	{0, 10, 0, 15, -5, -10, -10}, // 4 Monk
	{-5, 10, 15, 0, -10, -10, 0}, // 5 Bard
	{0, 0, 10, 15, -10, -10, -5}, // 6 Rogue
	{0, -10, -10, 10, 15, -5, 0}, // 7 Druid
	{-10, 10, 0, -5, 15, -10, 0}, // 8 Shaman
	{0, 0, -5, -10, 15, -10, 10}, // 9 Cleric
	{-10, 0, 10, -5, 0, 15, -10}, // 10 Magician
	{0, 10, -10, 0, -5, 15, -10}, // 11 Necromancer
	{-10, -10, 0, 0, -5, 15, 10}, // 12 Enchanter
	{0, -5, 0, 10, -10, 15, -10}, // 13 Wizard
	{-10, 0, -5, 0, 10, 15, -10}, // 14 Alchemist
}

// LookupBaseStats is a new character's stats before the 20 points it
// spends, as the create screen shows them: 60, plus the race's bonus, plus
// the class's (viwndcharcreatereview.cpp). Sandstorm's level 1 characters
// have exactly these (2026-10-21: seven race/class pairs, the points spent
// aside), where Return Home's defaultcharacter table, this server's source
// until then, has its Barbarian Shaman's row for the Barbarian Warrior and a
// Troll Shaman of WIS 5. A race or class out of range takes no bonus.
func LookupBaseStats(race, class int) baseStats {
	var v [7]int
	for i := range v {
		v[i] = 60
		if race >= 0 && race < len(raceBonus) {
			v[i] += raceBonus[race][i]
		}
		if class >= 0 && class < len(classBonus) {
			v[i] += classBonus[class][i]
		}
	}
	return baseStats{v[0], v[1], v[2], v[3], v[4], v[5], v[6]}
}

// hpFactor is the class's hit points a level before stamina: Return Home's
// HPFactor (Character.cs), 24 the warrior, paladin and shadow knight, 16 the
// ranger, monk, bard and rogue, 13 the priests, 10 the casters. Sandstorm's
// are the same.
func hpFactor(class int) int {
	switch class {
	case 0, 2, 3:
		return 24
	case 1, 4, 5, 6:
		return 16
	case 7, 8, 9:
		return 13
	}
	return 10
}

// CalculateMaxHP is the maximum hit points stamina gives, the live game's
// formula (eqoa.fandom.com's Formulas, the user's choice of 2026-10-25):
// (HP factor + STA/11) x level, STA/11 a real division and the whole
// truncated, worked in integers so no rounding error can drop a point; all
// of the stamina counted, worn and from spells, within its cap. Sandstorm
// rounds STA/11 to a whole number first (stat channel byte 148, 32
// characters measured 2026-10-21): of the 25 with no hit point mastery, 11
// come out the same and 14 from 23 below to 8 above. Worn items' and
// spells' own hit points come on top (MaxHPNow).
func CalculateMaxHP(level, class, stamina int) int {
	hp := (hpFactor(class)*11 + max(stamina, 0)) * max(level, 0) / 11
	return max(hp, 1)
}

// powerStats is the two stats a class's power is made of: the one its class
// bonus raises 15, then the one it raises 10 (classBonus). Sandstorm's power
// fits these pairs and no other for every class measured; Return Home's
// table had the warrior's, paladin's and shadow knight's swapped, the bard's
// and monk's on AGI and WIS, the ranger's and rogue's swapped.
func powerStats(class, str, sta, agi, dex, wis, intel, cha int) (primary, secondary int) {
	if class < 0 || class >= len(classBonus) {
		class = 0
	}
	stats := [7]int{str, sta, agi, dex, wis, intel, cha}
	for i, b := range classBonus[class] {
		switch b {
		case 15:
			primary = stats[i]
		case 10:
			secondary = stats[i]
		}
	}
	return primary, secondary
}

// CalculateMaxPower is the maximum power the stats give, the live game's
// formula (eqoa.fandom.com's Formulas): (28 + primary/10 + secondary/20) x
// level, truncated, worked in integers as (560 + 2 primary + secondary) x
// level / 20; the pair the class's own (powerStats). The wiki names a
// primary by archetype (tanks STR, melee AGI, casters INT, healers WIS),
// which CLIENT's class table and Sandstorm's measurements refute for tanks
// and most melee (a level 50 shadow knight: 2608 measured, 2607 by the
// class's pair, 2285 by STR). The stat channel's base maximum (byte 156) is
// this of the base stats; its maximum (byte 40), this of the stats worn and
// from spells too, plus their power. Sandstorm rounds half up: of the 32
// characters measured, 15 come out the same, 16 one lower, and one bought
// a mastery that costs power.
func CalculateMaxPower(level, class, str, sta, agi, dex, wis, intel, cha int) int {
	primary, secondary := powerStats(class, str, sta, agi, dex, wis, intel, cha)
	pow := (560 + 2*max(primary, 0) + max(secondary, 0)) * max(level, 0) / 20
	return max(pow, 1)
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
// A placeholder: both constants are fitted to two characters, not verified
// (claude-notes/combat-formula-final.md, "Status of the OFF/DEF MOD formulas").
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
