package database

import (
	"database/sql"
	"errors"
	"fmt"
)

// keptrecord.go — an item record the server keeps by its name, made at its
// start when none is so named (the Rock of Freeport, handlers/rockfreeport.go:
// a record no data step need write).

// EnsureItemRecord is the record of rec's name worn where rec is (equipslot;
// the lowest id of those), or else rec made at the first id from `from` that
// no record holds, with made true. A record once there is never changed
// here, so a GM's edit of it (the panel) stands. rec's PatternID is not read;
// its stat columns go in as 0 where nil, as the kept items' records have them.
func EnsureItemRecord(db *sql.DB, rec ItemPatternRecord, from int) (id int, made bool, err error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	err = tx.QueryRow(`SELECT patternid FROM itempattern WHERE itemname = ? AND equipslot = ? ORDER BY patternid LIMIT 1`,
		rec.ItemName, rec.EquipSlot).Scan(&id)
	switch {
	case err == nil:
		return id, false, nil
	case !errors.Is(err, sql.ErrNoRows):
		return 0, false, err
	}
	id = from
	for {
		var held int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM itempattern WHERE patternid = ?`, id).Scan(&held); err != nil {
			return 0, false, err
		}
		if held == 0 {
			break
		}
		id++
	}
	z := func(p *int) int {
		if p == nil {
			return 0
		}
		return *p
	}
	r := &rec
	if _, err := tx.Exec(`
		INSERT INTO itempattern (
			patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3,
			trade, rent, unk4, attacktype, weapondamage, unk5,
			levelreq, maxstack, maxhp, duration, classuse, raceuse,
			procanim, lore, unk6, craft, itemname, itemdesc, model, color,
			str, sta, agi, wis, dex, cha, intelligence,
			HPMAX, POWMAX, PoT, HoT, AC, PR, DR, FR, CR, LR, AR,
			weaponproc, fish
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, r.ItemCost, r.Unk1, r.ItemIcon, r.Unk2,
		r.EquipSlot, r.Unk3, r.Trade, r.Rent, r.Unk4,
		r.AttackType, r.WeaponDamage, r.Unk5,
		r.LevelReq, r.MaxStack, r.MaxHP, r.Duration,
		r.ClassUse, r.RaceUse, r.ProcAnim, r.Lore, r.Unk6, r.Craft,
		r.ItemName, r.ItemDesc, r.Model, r.Color,
		z(r.Str), z(r.Sta), z(r.Agi), z(r.Wis), z(r.Dex), z(r.Cha), z(r.Intelligence),
		z(r.HPMAX), z(r.POWMAX), z(r.PoT), z(r.HoT), z(r.AC),
		z(r.PR), z(r.DR), z(r.FR), z(r.CR), z(r.LR), z(r.AR),
		z(r.WeaponProc), z(r.Fish),
	); err != nil {
		return 0, false, fmt.Errorf("making %q at %d: %w", rec.ItemName, id, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, false, err
	}
	return id, true, nil
}
