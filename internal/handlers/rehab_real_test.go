package handlers

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The rehabilitators on a real server's data (EQOA_REAL_DB names its
// database, a copy of which is opened) once tools/seed_rehabilitation.py has
// written it: every rehabilitator (Anja, Frejo), spawned as the server spawns
// a row (NPCFromRecord), talks: its talk opens the menu and the World will not
// have it attacked; the Rose of Rehabilitation is the tool's record (a rose's
// art, no trade); and every class's level 60 epic, read by the quest engine,
// gives a rose at its last step. Before the tool Frejo's row makes a creature
// (npc_type 0) and there is no rose: this fails. Skipped without a database.
func TestRealRehabilitators(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	path := filepath.Join(t.TempDir(), "rehab.db")
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	in.Close()
	out.Close()
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	inv := database.NewInventoryStore(db)

	rose, ok, err := inv.ItemPatternByID(database.RoseOfRehabilitation)
	if err != nil || !ok {
		t.Errorf("no Rose of Rehabilitation (%v): tools/seed_rehabilitation.py --apply first", err)
	} else if rose.ItemName != database.RoseOfRehabilitationName || int32(rose.ItemIcon) != 759960290 || rose.Trade != 1 ||
		rose.ItemCost != 10_000_000 || rose.EquipSlot != -1 {
		t.Errorf("the rose's record: %q icon %d trade %d value %d slot %d", rose.ItemName, rose.ItemIcon, rose.Trade,
			rose.ItemCost, rose.EquipSlot)
	}

	recs, err := database.NewNPCStore(db).GetAllNPCs()
	if err != nil {
		t.Fatal(err)
	}
	tw := newTalkWorld(t, 0, 0)
	tw.h.rehab = NewRehab(db, tw.w, inv, database.NewSpellbookStore(db), nil, database.NewSQLiteCharacterStore(db))
	tw.p.Level, tw.p.BaseStr = 60, 130
	tw.p.RecalcStats(true) // alive: a dead player engages nothing
	if tw.p.IsDead() {
		t.Fatal("the player is dead")
	}
	slot, names := 1, map[string]bool{}
	for _, rec := range recs {
		if !IsRehabilitator(rec.NPCName) {
			continue
		}
		npc := entity.NPCFromRecord(rec, 0, nil)
		names[npc.Name] = true
		tw.p.X, tw.p.Y, tw.p.Z, tw.p.World = npc.SpawnX, npc.SpawnY, npc.SpawnZ, npc.World
		id := tw.add(slot, npc)
		slot++
		tw.talk(id)
		if m := questMsgs(tw.s); !IsRehabBox(boxID(m)) {
			t.Errorf("%s (npc_type %d) opens no menu: %q", npc.Name, rec.NPCType, m)
		}
		if tw.w.EngageAttack(tw.s, id, entity.Swing{Reach: 50}, time.Now()) {
			t.Errorf("%s (npc_type %d) can be attacked", npc.Name, rec.NPCType)
		}
	}
	if !names["Rehabilitator Anja"] || !names["Rehabilitator Frejo"] {
		t.Errorf("the rehabilitators: %v", names)
	}

	qe, err := NewQuestEngine(db, nil, nil, inv, nil)
	if err != nil {
		t.Fatal(err)
	}
	epic := regexp.MustCompile(`^60/[^/]+/reward-[a-z]+$`)
	n := 0
	for _, qu := range qe.cat.all {
		if !epic.MatchString(importKey(qu.Source)) {
			continue
		}
		n++
		last := qu.Steps[len(qu.Steps)-1]
		given := false
		for _, it := range last.RewardItems {
			given = given || it.PatternID == database.RoseOfRehabilitation
		}
		if !given || !qu.Enabled {
			t.Errorf("%s (%d, enabled %v) gives no rose at its last step: %v", qu.Name, qu.ID, qu.Enabled, last.RewardItems)
		}
	}
	if n != 23 {
		t.Errorf("%d epic reward quests, want 23", n)
	}
}
