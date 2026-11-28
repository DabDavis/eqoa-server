package database

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	HeroicMarkStack       = 99
	HeroicCMCost          = 75
	HeroicTeleportCharges = 50
	HeroicTeleportSeconds = 2
	heroicMarksSource     = "heroic raid marks"
)

// HeroicRole describes a currency and one physical reward for a class mask.
type HeroicRole struct {
	Name             string      `json:"name"`
	Classes          int         `json:"classes"`
	MarkID           int         `json:"mark_id"`
	MarkName         string      `json:"mark_name"`
	RelicID          int         `json:"relic_id"`
	RelicName        string      `json:"relic_name"`
	RelicDescription string      `json:"relic_description"`
	AttackChance     int         `json:"attack_chance"`
	AttackDamage     int         `json:"attack_damage"`
	SpellChance      int         `json:"spell_chance"`
	SpellDamage      int         `json:"spell_damage"`
	HealCharismaPct  int         `json:"heal_charisma_pct"`
	Taunts           map[int]int `json:"taunts"` // class -> ordinary taunt spell ID
}

// HeroicMarkSet describes the rewards from one boss. Relics keep their own
// pattern IDs by boss, element and class mask so a future recipe can take the
// actual components from the character's inventory.
type HeroicMarkSet struct {
	Boss, Name, Element string
	Roles               []HeroicRole
	Mastery             struct {
		Base, ID int
		Name     string
	}
	Teleport struct {
		ID, Spell, World int
		Name, Place      string
		X, Y, Z          float32
	}
}

//go:embed heroicmarks.json
var heroicMarksJSON []byte

var heroicMarkSets = func() []HeroicMarkSet {
	var sets []HeroicMarkSet
	if err := json.Unmarshal(heroicMarksJSON, &sets); err != nil {
		panic(fmt.Sprintf("heroic marks catalog: %v", err))
	}
	return sets
}()

func HeroicMarksFor(boss string) *HeroicMarkSet {
	for i := range heroicMarkSets {
		if heroicMarkSets[i].Boss == boss {
			return &heroicMarkSets[i]
		}
	}
	return nil
}

func (r HeroicRole) Matches(class int) bool {
	return class >= 0 && r.Classes&(1<<uint(class)) != 0
}

func (s *HeroicMarkSet) RoleFor(class int) *HeroicRole {
	for i := range s.Roles {
		if s.Roles[i].Matches(class) {
			return &s.Roles[i]
		}
	}
	return nil
}

func HeroicMarkPattern(id int) bool {
	for _, set := range heroicMarkSets {
		for _, role := range set.Roles {
			if role.MarkID == id {
				return true
			}
		}
	}
	return false
}

func HeroicPersonalPattern(id int) bool {
	if HeroicMarkPattern(id) {
		return true
	}
	if _, _, ok := HeroicRelicOf(id); ok {
		return true
	}
	for _, set := range heroicMarkSets {
		if set.Teleport.ID != 0 && set.Teleport.ID == id {
			return true
		}
	}
	return false
}

func HeroicRelicOf(id int) (HeroicRole, string, bool) {
	for _, set := range heroicMarkSets {
		for _, role := range set.Roles {
			if role.RelicID == id {
				return role, set.Element, true
			}
		}
	}
	return HeroicRole{}, "", false
}

func HeroicMasteryBoss(id int) string {
	for _, set := range heroicMarkSets {
		if set.Mastery.ID == id {
			return set.Boss
		}
	}
	return ""
}

func HeroicTauntSpellIDs() map[int]bool {
	ids := make(map[int]bool)
	for _, set := range heroicMarkSets {
		for _, role := range set.Roles {
			for _, id := range role.Taunts {
				ids[id] = true
			}
		}
	}
	return ids
}

// InstallHeroicMarks stages the catalog in the shard database in one
// transaction. The server calls it only when heroic marks are enabled.
func (db *SQLiteDB) InstallHeroicMarks() error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS charheroicrewards (
		serverid INTEGER NOT NULL REFERENCES characters(serverid) ON DELETE CASCADE,
		boss TEXT NOT NULL,
		reward INTEGER NOT NULL,
		PRIMARY KEY (serverid,boss,reward)
	)`); err != nil {
		return err
	}
	for _, set := range heroicMarkSets {
		for _, role := range set.Roles {
			mark := heroicItem(role.MarkID, role.MarkName, fmt.Sprintf("A %s mark from Heroic %s. Spend it at the Heroic Raids board.", role.Name, set.Name), role.Classes)
			mark.MaxStack = HeroicMarkStack
			if err := ensureHeroicItem(tx, &mark); err != nil {
				return err
			}
			relic := heroicItem(role.RelicID, role.RelicName, role.RelicDescription, role.Classes)
			relic.EquipSlot, relic.Lore = 8, 1 // ring
			if err := ensureHeroicItem(tx, &relic); err != nil {
				return err
			}
		}
		if set.Mastery.ID != 0 {
			if err := installHeroicMastery(tx, &set); err != nil {
				return err
			}
		}
		if set.Teleport.ID != 0 {
			if err := installHeroicTeleport(tx, &set); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func heroicItem(id int, name, description string, classes int) ItemPatternRecord {
	return ItemPatternRecord{
		PatternID: id, ItemName: name, ItemDesc: description, ItemIcon: 756343796,
		EquipSlot: -1, Trade: 1, Rent: 1, LevelReq: 1, MaxStack: 1, ClassUse: classes,
		RaceUse: 32767, Color: 0xFFFFFFFF,
	}
}

func ensureHeroicItem(tx *sql.Tx, rec *ItemPatternRecord) error {
	var existing ItemPatternRecord
	err := tx.QueryRow(`SELECT itemname, itemdesc, trade, rent, maxstack, classuse, equipslot, lore
		FROM itempattern WHERE patternid = ?`, rec.PatternID).Scan(&existing.ItemName, &existing.ItemDesc, &existing.Trade,
		&existing.Rent, &existing.MaxStack, &existing.ClassUse, &existing.EquipSlot, &existing.Lore)
	if errors.Is(err, sql.ErrNoRows) {
		return insertItemRecord(tx, rec.PatternID, rec)
	}
	if err != nil {
		return err
	}
	if existing.ItemName != rec.ItemName || existing.ItemDesc != rec.ItemDesc || existing.Trade != rec.Trade ||
		existing.Rent != rec.Rent || existing.MaxStack != rec.MaxStack || existing.ClassUse != rec.ClassUse ||
		existing.EquipSlot != rec.EquipSlot || existing.Lore != rec.Lore {
		return fmt.Errorf("heroic item %d conflicts with %q", rec.PatternID, existing.ItemName)
	}
	return nil
}

func installHeroicMastery(tx *sql.Tx, set *HeroicMarkSet) error {
	m := set.Mastery
	res, err := tx.Exec(`INSERT INTO cmcatalog
		(id, category, classes, races, mastery, rank, name, parent, cost, level, spent,
		minstats, requires, require_spells, require_ranks, require_quests, effects,
		adds, add_spells, removes, remove_spells, description, enabled, source)
		SELECT ?, category, classes, races, mastery, rank+1, ?, id, ?, level, spent,
		minstats, requires, require_spells, require_ranks, require_quests, effects,
		adds, add_spells, removes, remove_spells, description, enabled, ?
		FROM cmcatalog WHERE id = ? AND enabled = 1
		ON CONFLICT(id) DO UPDATE SET category=excluded.category, classes=excluded.classes, races=excluded.races,
		mastery=excluded.mastery, rank=excluded.rank, name=excluded.name, parent=excluded.parent, cost=excluded.cost,
		level=excluded.level, spent=excluded.spent, minstats=excluded.minstats, requires=excluded.requires,
		require_spells=excluded.require_spells, require_ranks=excluded.require_ranks, require_quests=excluded.require_quests,
		effects=excluded.effects, adds=excluded.adds, add_spells=excluded.add_spells, removes=excluded.removes,
		remove_spells=excluded.remove_spells, description=excluded.description, enabled=excluded.enabled
		WHERE cmcatalog.source=excluded.source`, m.ID, m.Name, HeroicCMCost, heroicMarksSource, m.Base)
	return heroicInstalled(res, err, fmt.Sprintf("mastery %d (base %d)", m.ID, m.Base))
}

func heroicInstalled(res sql.Result, err error, name string) error {
	if err != nil {
		return fmt.Errorf("heroic %s: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		return fmt.Errorf("heroic %s: missing base record or conflicting ID", name)
	}
	return err
}

func installHeroicTeleport(tx *sql.Tx, set *HeroicMarkSet) error {
	to := set.Teleport
	rec := heroicItem(to.ID, to.Name,
		fmt.Sprintf("Use to travel to %s. %d charges. %d-second cast.", to.Place, HeroicTeleportCharges, HeroicTeleportSeconds), 32767)
	rec.ItemIcon = 414194432 // travel stone
	if err := ensureHeroicItem(tx, &rec); err != nil {
		return err
	}
	line := "Self, Teleport to " + to.Place
	res, err := tx.Exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime,
		power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		VALUES (?,1,0,0,0,?,0,0,0,0,0,0,?,?)
		ON CONFLICT(spellid) DO UPDATE SET casttime=excluded.casttime, spelldesc=excluded.spelldesc
		WHERE spellpattern.spellname=excluded.spellname AND spellpattern.scope=0`, to.Spell, HeroicTeleportSeconds, to.Name, line)
	if err := heroicInstalled(res, err, "teleport spell"); err != nil {
		return err
	}
	res, err = tx.Exec(`INSERT INTO spelleffect (spellid,line,fxdict,source) VALUES (?,?,?,?)
		ON CONFLICT(spellid) DO UPDATE SET line=excluded.line, fxdict=excluded.fxdict WHERE spelleffect.source=excluded.source`,
		to.Spell, line, 0x8FB28233, heroicMarksSource)
	if err := heroicInstalled(res, err, "teleport effect"); err != nil {
		return err
	}
	res, err = tx.Exec(`INSERT INTO spellgates (spell,kind,place,world,x,y,z,source) VALUES (?,'self',?,?,?,?,?,?)
		ON CONFLICT(spell) DO UPDATE SET place=excluded.place,world=excluded.world,x=excluded.x,y=excluded.y,z=excluded.z
		WHERE spellgates.source=excluded.source AND spellgates.kind='self'`,
		SpellGateKey(to.Name), to.Place, to.World, to.X, to.Y, to.Z, heroicMarksSource)
	if err := heroicInstalled(res, err, "teleport destination"); err != nil {
		return err
	}
	res, err = tx.Exec(`INSERT INTO itemeffect (patternid,line,charges,source) VALUES (?,?,?,?)
		ON CONFLICT(patternid) DO UPDATE SET charges=excluded.charges WHERE itemeffect.source=excluded.source`, to.ID, line, HeroicTeleportCharges, heroicMarksSource)
	if err := heroicInstalled(res, err, "teleport charges"); err != nil {
		return err
	}
	res, err = tx.Exec(`INSERT INTO itemspell (patternid,spellid,source) VALUES (?,?,?)
		ON CONFLICT(patternid) DO UPDATE SET spellid=excluded.spellid WHERE itemspell.source=excluded.source`, to.ID, to.Spell, heroicMarksSource)
	return heroicInstalled(res, err, "teleport item spell")
}
