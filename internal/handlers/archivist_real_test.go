package handlers

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The Spell Archivist's grant on a real server's data (EQOA_REAL_DB names its
// database, a copy of which is opened), every class open to every race as the
// Heirloom Test Server runs (world.all_classes), its merchants' part alone (no
// quests run here; the quests' side spells of petition 1361 are
// TestRealArchivistSideSpells'): each class's basic spells at
// levels 20, 49 and 60 number what the petition's own checks counted at 60
// (Jibrish's agent's Go code and a separate Python check, on the Heirloom
// server's data of 2026-11-08) and what a Python reading of the same rule
// counted at 20 and 49; none is a master class's or mastery's scroll a
// merchant sells (Djinni Bolt, Summon Djinn, Silent Blade Burglar, Virulent
// Blade Assassin, Remove Mask, a monk's Freezing Strike, the warriors' mastery
// Tranquility) nor a class quest's reward (Kick, Taunt and Rapid Strike for a
// warrior, Crawling Skin, Alarming Visage and Spacious Mind for an enchanter).
// Every NPC named Spell Archivist carries the merchant flag, so CLIENT asks
// his window and his question is reached. Then, through his box, a level 60
// wizard whose book holds Return Home alone is granted every one of them,
// nothing a second time, and is left with Return Home by a clear. Skipped
// without a database.
func TestRealArchivistBasicSpells(t *testing.T) {
	db := realRehabDB(t)
	was := entity.Creation()
	defer entity.SetCreationRules(was)
	entity.SetCreationRules(entity.CreationRules{Races: true, AllClasses: true})

	arch, err := loadSpellArchive(database.NewSpellbookStore(db))
	if err != nil {
		t.Fatal(err)
	}
	names := [15]string{"WAR", "RAN", "PAL", "SK", "MNK", "BRD", "ROG", "DRU", "SHM", "CLR", "MAG", "NEC", "ENC", "WIZ", "ALC"}
	want := map[int][15]int{
		20: {12, 14, 15, 11, 12, 12, 13, 27, 26, 28, 29, 24, 25, 33, 28},
		49: {25, 27, 27, 24, 24, 24, 25, 51, 50, 55, 57, 49, 49, 65, 54},
		60: {39, 41, 38, 42, 38, 38, 40, 78, 70, 74, 74, 68, 69, 93, 76},
	}
	for _, level := range []int{20, 49, 60} {
		var got []string
		for class, n := range want[level] {
			if c := len(arch.basic(class, 0, level)); c != n {
				t.Errorf("level %d, %s: %d basic spells, want %d", level, names[class], c, n)
			}
			got = append(got, fmt.Sprintf("%s %d", names[class], len(arch.basic(class, 0, level))))
		}
		t.Logf("level %d: %s", level, strings.Join(got, ", "))
	}
	if n := len(arch.basic(archivistWizard, 0, 70)); n != want[60][archivistWizard] {
		t.Errorf("a wizard of 70: %d basic spells, want the 60's %d", n, want[60][archivistWizard])
	}

	not := map[int][]string{
		0:  {"Kick", "Taunt", "Rapid Strike", "Tranquility"},
		4:  {"Freezing Strike"},
		6:  {"Silent Blade Burglar", "Virulent Blade Assassin", "Remove Mask"},
		10: {"Djinni Bolt", "Summon Djinn"},
		12: {"Crawling Skin", "Alarming Visage", "Spacious Mind"},
	}
	for class, spells := range not {
		for _, s := range arch.basic(class, 0, 60) {
			for _, no := range spells {
				if exactKey(s.Name) == exactKey(no) {
					t.Errorf("%s is granted %s (%d)", names[class], s.Name, s.SpellID)
				}
			}
		}
	}

	recs, err := database.NewNPCStore(db).GetAllNPCs()
	if err != nil {
		t.Fatal(err)
	}
	archivists := 0
	for _, rec := range recs {
		if npc := entity.NPCFromRecord(rec, 0, nil); isSpellArchivist(npc) {
			archivists++
			if npc.NPCType&0xFF&npcFlagMerchant == 0 {
				t.Errorf("the Spell Archivist (npcs %d, npc_type %d) is no merchant: CLIENT talks to him, and nothing asks", rec.ID, rec.NPCType)
			}
		}
	}
	t.Logf("%d Spell Archivists in the database", archivists)

	// through his box
	f := newArchivistFixture(t, db)
	f.p.Level, f.p.Class = 60, archivistWizard
	f.know(2300, 0, 0)
	var adds int
	got := f.grant()
	for _, m := range got {
		if strings.HasPrefix(m, "add:") {
			adds++
			if !strings.HasSuffix(m, " flag 0") {
				t.Errorf("a grant's 0x00B3 says word 721: %q", m)
			}
		}
	}
	wantBox := fmt.Sprintf("box:Spell Archivist: There: %d spells of your calling written into your book, every one the merchants sell or its quests teach up to level 60 that you lacked.",
		want[60][archivistWizard])
	if adds != want[60][archivistWizard] || len(got) != adds+1 || got[len(got)-1] != wantBox {
		t.Errorf("the wizard's grant: %d added, the last %q; want %d and %q", adds, got[len(got)-1], want[60][archivistWizard], wantBox)
	}
	f.wantMsgs("the wizard's second grant", f.grant(),
		"box:Spell Archivist: Your book holds every spell of your calling the merchants sell or its quests teach up to level 60 already.")
	box := boxID(f.pick(f.menu(), archivistClear))
	got = f.pick(box, 0)
	if n := len(got); n != want[60][archivistWizard]+1 || got[n-1] != fmt.Sprintf("box:Spell Archivist: Your book is clear but for Return Home: %d spells gone.",
		want[60][archivistWizard]) {
		t.Errorf("the wizard's clear: %q", got)
	}
	if b := f.book(); strings.Join(b, " ") != "2300@0/0" {
		t.Errorf("the wizard's book after the clear: %v", b)
	}
}
