package handlers

import (
	"bytes"
	"testing"
	"unicode/utf16"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// addWolfFang puts the fang's record in a test database as the seed writes it
// (tools/seed_wolf_fang.py: the gem type, worn nowhere, NO TRADE, a stack of 1).
func addWolfFang(t *testing.T, db *database.SQLiteDB) int {
	t.Helper()
	res, err := db.Exec(`INSERT INTO itempattern (itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, -720619736, 0, -1, 0, 1, 1, 0, 0, 0, 0, 1, 1, 0, 80, 32767, 32767, 7, 0, 0, 0, ?, '', 0, 0)`, WolfFangName)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

// The fang at the login, as the lamp: a character holding none gets one in
// the pack, never a second, and nothing while there is no record; the lamp's
// gift beside it is unchanged.
func TestALoginGivesTheFang(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/fang.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	rec := &database.CharacterRecord{CharName: "Runner", AccountID: 1}
	if err := store.Create(rec); err != nil {
		t.Fatal(err)
	}
	h := NewCharacterHandler(store, nil, database.NewInventoryStore(db), database.NewSpellbookStore(db))
	count := func(name string) int {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM charinventory c JOIN itempattern p ON p.patternid = c.patternid
			WHERE c.serverid = ? AND c.location = 0 AND p.itemname = ?`, rec.ServerID, name).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	h.giveWolfFang(rec.ServerID)
	if count(WolfFangName) != 0 {
		t.Fatal("a fang with no record")
	}
	addWolfFang(t, db)
	if _, err := db.Exec(`INSERT INTO itempattern (itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, 0, 0, -1, 0, 1, 1, 0, 0, 0, 0, 1, 1, 0, 80, 32767, 32767, 2, 0, 0, 0, ?, '', 0, 0)`, GenieLampName); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		h.giveGenieLamp(rec.ServerID)
		h.giveWolfFang(rec.ServerID)
	}
	if count(WolfFangName) != 1 || count(GenieLampName) != 1 {
		t.Fatalf("fangs %d, lamps %d in the pack; want one each", count(WolfFangName), count(GenieLampName))
	}
	if !IsWolfFang(&database.ItemPatternRecord{EquipSlot: -1, ItemName: " heirloom fang of the wolf"}) ||
		IsWolfFang(&database.ItemPatternRecord{EquipSlot: 13, ItemName: WolfFangName}) ||
		IsWolfFang(&database.ItemPatternRecord{EquipSlot: -1, ItemName: GenieLampName}) {
		t.Error("IsWolfFang")
	}
}

// The fang used from the inventory (0x003E) puts the boards' Spirit of the
// Wolf on its holder, the run speed 1.3 times, says so and stays; used again
// it renews the hour (one effect, one slot); the lamp's Use is not the fang's.
// A hostile act ends it (entity's TestTheBoardsWolfRunsUntilAHostileAct), and
// the dead are given nothing and told nothing.
func TestUsingTheFangQuickensTheStep(t *testing.T) {
	f := newScribeFixture(t)
	w := entity.NewWorld()
	w.SessionManager = session.NewManager()
	f.player.HP, f.player.MaxHP = 100, 100
	f.player.Speed = entity.RunSpeed
	w.Players[f.player.ServerID] = f.player
	f.sess.SetECSEntity(f.player)
	f.h.gameWorld = w
	base := f.player.SpeedNow()
	fang := addWolfFang(t, f.db)
	f.own(fang, 1)
	f.loadList()
	f.sess.Queue().GetPending()
	wolves := func() int {
		n := 0
		for _, e := range f.player.Effects {
			if e.SpellID == entity.BoardWolfEffectID && e.Name == entity.BoardWolfName {
				n++
			}
		}
		return n
	}

	if out := f.use(0); !says(out, wolfFangLine(false)) {
		t.Errorf("the first use: %d messages, not the line", len(out))
	}
	if got := f.player.SpeedNow(); wolves() != 1 || got < base*1.29 || got > base*1.31 {
		t.Fatalf("the first use: %d wolves, speed %v (base %v)", wolves(), got, base)
	}
	if out := f.use(0); !says(out, wolfFangLine(true)) {
		t.Errorf("the second use: %d messages, not the line", len(out))
	}
	if wolves() != 1 || len(f.player.Buffs) != 1 || f.stackOf(WolfFangName) != 1 {
		t.Fatalf("again: %d wolves, %d slots, %d fangs left", wolves(), len(f.player.Buffs), f.stackOf(WolfFangName))
	}

	f.player.Effects, f.player.Buffs = nil, nil
	f.player.HP = 0 // dead
	if out := f.use(0); wolves() != 0 || says(out, wolfFangLine(false)) {
		t.Errorf("the dead: %d wolves, told %v", wolves(), says(out, wolfFangLine(false)))
	}
}

// says is whether any of the queued messages carries line as CLIENT's wide
// text (SendClientMessage's).
func says(out [][]byte, line string) bool {
	var wide []byte
	for _, u := range utf16.Encode([]rune(line)) {
		wide = append(wide, byte(u), byte(u>>8))
	}
	for _, m := range out {
		if bytes.Contains(m, wide) {
			return true
		}
	}
	return false
}
