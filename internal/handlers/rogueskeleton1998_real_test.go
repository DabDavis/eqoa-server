package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// Over a real server's database with tools/rogue_skeleton_1998_20261113.py
// applied (EQOA_REAL_DB, copied; Hoofbite's ticket 1998, a skeleton disguise
// for rogues): the Spell Archivist's Grant basic spells gives a rogue of 60
// Skeletal Mask (its scroll sold by Merchant Morian and the archivists, its
// classspells row a vendor's), a rogue of 59 and an enchanter of 60 not.
func TestRealTicket1998ArchivistGrantsSkeletalMask(t *testing.T) {
	db := realRehabDB(t)
	was := entity.Creation()
	defer entity.SetCreationRules(was)
	entity.SetCreationRules(entity.CreationRules{Races: true, AllClasses: true})
	arch, err := loadSpellArchive(database.NewSpellbookStore(db))
	if err != nil {
		t.Fatal(err)
	}
	has := func(class, level int) bool {
		for _, s := range arch.basic(class, 0, level) {
			if s.Name == "Skeletal Mask" {
				return true
			}
		}
		return false
	}
	if !has(6, 60) {
		t.Fatal("a rogue of 60 is not granted Skeletal Mask: the tool not applied to this database?")
	}
	if has(6, 59) || has(12, 60) {
		t.Errorf("granted to a rogue of 59 %v, an enchanter of 60 %v; want neither", has(6, 59), has(12, 60))
	}
}
