package main

import (
	"encoding/binary"
	"path/filepath"
	"testing"

	"github.com/eqoa/server/internal/database"
)

func TestReadCompleteRecordSections(t *testing.T) {
	// A character with no hotkeys, quests, bank rows or spells, carrying one item.
	dump := []byte{13, 0, 0}
	dump = append(dump, make([]byte, 4+1+4+5+1+5+24+1+8+4)...)
	dump = append(dump, 2, 1, 0, 0, 0) // one inventory record
	dump = append(dump, make([]byte, 5+4)...)
	dump = binary.AppendVarint(dump, 71)        // pattern
	dump = append(dump, make([]byte, 22+8)...)  // remaining fields and two empty strings
	dump = append(dump, 2, 24, 14)              // one stat: HoT 7
	dump = append(dump, make([]byte, 5+4+1)...) // bank, flags, spells
	items, bank, spells, _, err := parseMemoryDump(dump)
	if err != nil || len(items) != 1 || len(bank)+len(spells) != 0 {
		t.Fatalf("read item list: %v, %v", items, err)
	}
	rec := itemToRecord(&items[0])
	if rec.PatternID != 71 || rec.HoT == nil || *rec.HoT != 7 || rec.PoT != nil {
		t.Fatalf("incorrect item: %+v", rec)
	}
	for end := range len(dump) {
		if _, _, _, _, err := parseMemoryDump(dump[:end]); err == nil {
			t.Fatalf("accepted a record section truncated at byte %d", end)
		}
	}
}

func TestRecordImportRollsBackPatternsAndCharacter(t *testing.T) {
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "import.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO characters (charName, accountid, serverid, modelid, tclass, race, humType, level, haircolor, hairlength, hairstyle, faceoption, sex, world, x, y, z, tunar) VALUES ('sample',1,17,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1)`)
	if err != nil {
		t.Fatal(err)
	}
	if err := importRecords(db.DB, 17, []parsedItem{{ItemID: 1}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("CREATE TRIGGER reject_spell BEFORE INSERT ON spells BEGIN SELECT RAISE(ABORT, 'write failure'); END")
	if err != nil {
		t.Fatal(err)
	}
	if err := importRecords(db.DB, 17, []parsedItem{{ItemID: 2}}, nil, []parsedSpell{{SpellID: 3}}); err == nil {
		t.Fatal("expected spell write to fail")
	}
	var held, patterns int
	if err := db.QueryRow("SELECT patternid, (SELECT count(*) FROM itempattern) FROM charinventory WHERE serverid = 17").Scan(&held, &patterns); err != nil || held != 1 || patterns != 1 {
		t.Fatalf("held=%d, patterns=%d, error=%v", held, patterns, err)
	}
}
