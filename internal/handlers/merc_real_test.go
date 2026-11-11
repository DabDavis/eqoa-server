package handlers

import (
	"os"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// On a real database with tools/seed_heirlooms.py applied (EQOA_REAL_DB, a
// copy): a merc of every class at levels 10, 30 and 60 wears its class's
// heirlooms (a chest piece's look, its hands as its role holds them: a tank
// or a priest a weapon and a shield, a melee class two weapons from 20 but
// a monk's bare hands, a caster a two-hander), carries a book, and its role
// has something of its own in it: a healer a heal, a tank a taunt, a caster a
// hit; a tank or a melee fighter a bow. Each kit is logged with its figures.
func TestRealMercKits(t *testing.T) {
	path := os.Getenv("EQOA_REAL_DB")
	if path == "" {
		t.Skip("EQOA_REAL_DB unset")
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if defs, err := db.Heirlooms(); err != nil || len(defs) == 0 {
		t.Skipf("no heirlooms in %s (%v)", path, err)
	}
	m := NewMercs(nil, db, NewHeirlooms(db))
	for class := 0; class <= 14; class++ {
		for _, level := range []int{10, 30, 60} {
			k, err := m.kit(database.MercRow{Name: "Real", Class: class, Race: 0}, level)
			if err != nil {
				t.Fatalf("%s %d: %v", entity.ClassName(class), level, err)
			}
			role := entity.MercRoleName(class)
			if k.Look.Chest == 0 && k.Look.Robe < 0 {
				t.Errorf("%s %d: no chest piece and no robe", entity.ClassName(class), level)
			}
			switch {
			case class == 4:
				if k.Worn[14] || k.Worn[16] {
					t.Errorf("a monk of %d holds a weapon", level)
				}
			case role == "caster":
				if !k.Worn[16] {
					t.Errorf("%s %d: no two-hander", entity.ClassName(class), level)
				}
			case role == "melee fighter":
				if !k.Worn[14] || (level >= mercDualFrom) != k.Worn[15] {
					t.Errorf("%s %d: hands %v", entity.ClassName(class), level, k.Worn[14:19])
				}
			default:
				if !k.Worn[14] && !k.Worn[16] {
					t.Errorf("%s %d: no weapon", entity.ClassName(class), level)
				}
			}
			if len(k.Spells) == 0 {
				t.Errorf("%s %d: no book", entity.ClassName(class), level)
			}
			// a tank or a melee fighter carries a bow at its level's figures
			// (entity/mercshoot.go); a healer or a caster none
			if shoots := role == "tank" || role == "melee fighter"; shoots != (k.Bow.Damage > 0) || shoots && (k.Bow.Type != 7 || k.Bow.Range <= 0 || k.Bow.Model == 0) {
				t.Errorf("%s %d: bow %+v", entity.ClassName(class), level, k.Bow)
			}
			heals, taunts, hits := 0, 0, 0
			for i := range k.Spells {
				switch entity.MercUseName(&k.Spells[i]) {
				case "heal", "heal over time", "group heal":
					heals++
				case "taunt":
					taunts++
				case "hit", "damage over time":
					hits++
				}
			}
			switch {
			case role == "healer" && heals == 0, role == "tank" && level >= 30 && taunts == 0, role == "caster" && hits == 0:
				t.Errorf("%s %d: %d heals, %d taunts, %d hits in %d spells", entity.ClassName(class), level, heals, taunts, hits, len(k.Spells))
			}
			t.Logf("%-13s %2d: AC %4d, weapon %4d, chest %d robe %d, hands %v, %3d spells (%d heals, %d taunts, %d hits), bow %q %d reach %d",
				entity.ClassName(class), level, k.Gear.AC, k.Gear.WeaponDamage, k.Look.Chest, k.Look.Robe, k.Worn[14:19],
				len(k.Spells), heals, taunts, hits, k.Bow.Name, k.Bow.Damage, k.Bow.Range)
		}
	}
}
