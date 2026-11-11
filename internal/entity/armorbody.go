package entity

import (
	"fmt"
	"strings"

	"github.com/eqoa/server/internal/database"
)

// armorbody.go — a race whose own body takes no armour, shown in another body by what it wears (the
// user, 2026-11-08: "lets add armor display to player hill giants", then "there are sand giant tah
// wear cloth. fire giants weear plate. maybe a chain or leather also giant we can use, doesnt have
// to be limited to hill giants"). The hill giant's body (CHAR.ESF 0xFFB1EA2C) has no TSlotList, so
// no client draws armour on it (claude-notes/custom-races-20261107.md); other giants' bodies wear
// it: table racearmorbody gives a race a body for each kind of armour on its chest, a robe counting
// as cloth (Gear.Armor, from the worn items: handlers' wornGear), and Body() shows it under a GM's
// form and an illusion and over the race's own. A kind with no row, or a bare chest, is the race's
// own body. The bodies the giants' are within 3% of one another (worlds/models.txt), so the
// player's size stays its race's.

// ArmorKind is the kind of armour on a player's chest.
type ArmorKind int

// The kinds.
const (
	ArmorNone ArmorKind = iota
	ArmorCloth
	ArmorLeather
	ArmorChain
	ArmorPlate
)

var armorKindNames = [...]string{"none", "cloth", "leather", "chain", "plate"}

func (k ArmorKind) String() string {
	if k < 0 || int(k) >= len(armorKindNames) {
		return fmt.Sprintf("armor(%d)", int(k))
	}
	return armorKindNames[k]
}

// ArmorKindOf is the kind of an armour piece by its itempattern model, its material: 1 cloth; 2 and
// 6 leather (6: hides, the whale's and the fire newt's); 3, 7 and 8 chain (7 scale, 8 heavy chain);
// 4, 5 and 9 plate (5 the banded and brass breastplates, 9 ornate); anything else none.
func ArmorKindOf(model int64) ArmorKind {
	switch model {
	case 1:
		return ArmorCloth
	case 2, 6:
		return ArmorLeather
	case 3, 7, 8:
		return ArmorChain
	case 4, 5, 9:
		return ArmorPlate
	}
	return ArmorNone
}

// armorKindNamed is a kind by its name in racearmorbody ("" and none: ArmorNone, ok false).
func armorKindNamed(s string) (ArmorKind, bool) {
	for i, n := range armorKindNames[1:] {
		if strings.EqualFold(strings.TrimSpace(s), n) {
			return ArmorKind(i + 1), true
		}
	}
	return ArmorNone, false
}

// SetRaceArmorBodies puts the races' armour bodies in place (racearmorbody's rows), every player's
// body taken again from them.
func (w *World) SetRaceArmorBodies(rows []database.RaceArmorBody) (n int) {
	m := make(map[int]map[ArmorKind]uint32)
	for _, r := range rows {
		k, ok := armorKindNamed(r.Armor)
		if !ok || r.Body == 0 {
			continue
		}
		if m[r.Race] == nil {
			m[r.Race] = make(map[ArmorKind]uint32)
		}
		m[r.Race][k] = r.Body
		n++
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.armorBodies = m
	for _, p := range w.Players {
		p.ArmorBody = w.armorBodyLocked(p)
	}
	return n
}

// LoadRaceArmorBodies reads table racearmorbody into the world: at the start. It reports how many
// bodies it read.
func (w *World) LoadRaceArmorBodies(db *database.SQLiteDB) (int, error) {
	rows, err := db.RaceArmorBodies()
	if err != nil {
		return 0, fmt.Errorf("racearmorbody: %w", err)
	}
	return w.SetRaceArmorBodies(rows), nil
}

// armorBodyLocked is p's body for the armour on its chest: its race's row, else 0 (its own). A body
// the character chose of its race's options (a fire giant's, an ice giant's: raceoptions.go) is its
// look and is kept in any armour, as no row was made for it (the rows are the race's own body's armour);
// a client that lays armour on a body with no slots (vanilla) lays it on that one. Must be called
// with the lock held.
func (w *World) armorBodyLocked(p *Player) uint32 {
	if !takesArmorBody(p.Race, uint32(p.ModelID)) {
		return 0
	}
	return w.armorBodies[p.Race][p.Gear.Armor]
}

// takesArmorBody reports whether a character of race wearing model is shown in its race's armour
// bodies: not a body it chose (ChosenBody), and not its race's own female body where that is
// another than the male's (the hill giantess, F34501CB: the rows are the male's armour, a male
// giant's body each, and she keeps her own look in any armour as a chosen female giant does; the
// GM Jibrish's petition 1647, 2026-11-11, gave the hill giant, the orc and the satyr their female
// bodies in racedefs). Any other body (the race's own male one, a GM's) takes them as before.
func takesArmorBody(race int, model uint32) bool {
	if ChosenBody(race, model) {
		return false
	}
	if f := uint32(raceOwnBody(race, 1)); f != 0 && f != uint32(raceOwnBody(race, 0)) && model == f {
		return false
	}
	return true
}
