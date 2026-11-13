package database

import (
	"fmt"
	"testing"
)

func TestQuestEarnedSourceClassGrantPolicy(t *testing.T) {
	for _, source := range []string{"quest", "eqoa-data quest 101300", " EQOA-Data QuEsT Sky "} {
		if got := GotBy(source); got != "quest" || Granted(source) {
			t.Errorf("source %q: GotBy=%q Granted=%v", source, got, Granted(source))
		}
		for _, every := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/every=%v", source, every), func(t *testing.T) {
				db, store := newCharDB(t)
				// Init supplies ordinary class spells; isolate this test's two choices.
				if _, err := db.Exec(`DELETE FROM classspells WHERE class=7`); err != nil {
					t.Fatal(err)
				}
				_, err := db.Exec(`INSERT INTO spellpattern
                    (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
                    VALUES (875,1,0,0,0,0,0,0,0,1,0,-1,'Ordinary spell',''),
                           (900441,53,0,0,0,0,0,0,0,1,0,-1,'Sky Portal','')`)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = db.Exec(`INSERT INTO classspells (class, spellid, source) VALUES
                    (7,875,'dump'),(7,900441,?)`, source); err != nil {
					t.Fatal(err)
				}
				character := &CharacterRecord{CharName: "QuestDruid", AccountID: 1, TClass: 7}
				if err = store.Create(character); err != nil {
					t.Fatal(err)
				}
				var added int
				if every {
					added, err = store.GrantEveryClassSpell(character.ServerID, 7)
				} else {
					added, err = store.GrantClassSpells(character.ServerID, 7)
				}
				if err != nil {
					t.Fatal(err)
				}
				want := 1
				if every {
					want = 2
				}
				if added != want {
					t.Fatalf("added %d spells; want %d", added, want)
				}
				if rows := book(t, db, character.ServerID); len(rows) != want || rows[0].spell != 875 {
					t.Fatalf("book %#v; wanted %d spells including ordinary spell", rows, want)
				}
				// A quest-earned/explicitly granted existing spell is preserved.
				if !every {
					if _, err = db.Exec(`INSERT INTO spells
                    (serverid,spellid,addedorder,onHotBar,whereonBar,unk1,showhide)
                    VALUES (?,900441,1,0,-1,0,-1)`, character.ServerID); err != nil {
						t.Fatal(err)
					}
				}
				if _, err = store.GrantClassSpells(character.ServerID, 7); err != nil {
					t.Fatal(err)
				}
				if rows := book(t, db, character.ServerID); len(rows) != 2 {
					t.Fatalf("earned spell removed: book %#v", rows)
				}
			})
		}
	}
}
