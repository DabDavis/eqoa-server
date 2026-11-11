package handlers

import (
	"bytes"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// jarLine is the jar's itemeffect line as tools/jar_of_breath_20261111.py
// writes it.
const jarLine = "Self, Enduring Breath, 4hr"

// addJarOfBreath puts the jar's record in a test database as the tool writes
// it (the gem type, worn nowhere, NO TRADE, LORE, a stack of 1, Breath
// Potion's jar icon), with its itemeffect row when withUse.
func addJarOfBreath(t *testing.T, db *database.SQLiteDB, withUse bool) int {
	t.Helper()
	res, err := db.Exec(`INSERT INTO itempattern (itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, 1342617881, 0, -1, 0, 1, 1, 0, 0, 0, 0, 1, 1, 0, 80, 32767, 32767, 7, 1, 0, 0, ?, '', 0, 0)`, JarOfBreathName)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if withUse {
		if _, err := db.Exec(`INSERT INTO itemeffect (patternid, line, name, icon, source) VALUES (?, ?, 'Enduring Breath', 1533256884, 'jarofbreath')`,
			id, jarLine); err != nil {
			t.Fatal(err)
		}
	}
	return int(id)
}

// The jar at the login, as the fang: none without the record, then one in
// the pack, never a second.
func TestALoginGivesTheJarOfBreath(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/jar.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	rec := &database.CharacterRecord{CharName: "Diver", AccountID: 1}
	if err := store.Create(rec); err != nil {
		t.Fatal(err)
	}
	h := NewCharacterHandler(store, nil, database.NewInventoryStore(db), database.NewSpellbookStore(db))
	count := func() (n int) {
		if err := db.QueryRow(`SELECT COUNT(*) FROM charinventory c JOIN itempattern p ON p.patternid = c.patternid
			WHERE c.serverid = ? AND c.location = 0 AND p.itemname = ?`, rec.ServerID, JarOfBreathName).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return
	}
	h.giveKeptItems(rec.ServerID, clientLists)
	if count() != 0 {
		t.Fatal("a jar with no record")
	}
	addJarOfBreath(t, db, true)
	h.giveKeptItems(rec.ServerID, clientLists)
	h.giveKeptItems(rec.ServerID, clientLists)
	if n := count(); n != 1 {
		t.Fatalf("%d jars in the pack, want one", n)
	}
	if !IsJarOfBreath(&database.ItemPatternRecord{EquipSlot: -1, ItemName: " heirloom jar of breath"}) ||
		IsJarOfBreath(&database.ItemPatternRecord{EquipSlot: 13, ItemName: JarOfBreathName}) ||
		IsJarOfBreath(&database.ItemPatternRecord{EquipSlot: -1, ItemName: "Breath Potion"}) {
		t.Error("IsJarOfBreath")
	}
}

// The jar used from the item menu (0x003E): its Enduring Breath on its user
// for four hours, said in a line; the jar stays, nothing taken off (no 0x003C,
// no charge's 0x003E back); used again it renews the one effect. The control:
// a potion of the same line is drunk and gone.
func TestUsingTheJarOfBreathHoldsBreathAndKeepsTheJar(t *testing.T) {
	f := newScribeFixture(t)
	f.potionWorld()
	jar := addJarOfBreath(t, f.db, true)
	f.own(jar, 1)
	f.loadList()
	f.sess.Queue().GetPending()
	breaths := func() (n int, left time.Duration) {
		for _, e := range f.player.Effects {
			if e.SpellID == entity.PotionEffectID(jar) && e.Rules.Breath {
				n++
				left = time.Until(e.Until)
			}
		}
		return
	}
	spent := func(out [][]byte) bool {
		for _, m := range out {
			if len(m) >= 2 && (bytes.HasPrefix(m, opBytes(protocol.OpcodeRemoveInvItem)) || bytes.HasPrefix(m, opBytes(protocol.OpcodeInteractItem))) {
				return true
			}
		}
		return false
	}
	for i := range 2 {
		out := f.use(0)
		if !says(out, jarUsedLine) {
			t.Errorf("use %d: not the line", i+1)
		}
		if spent(out) {
			t.Errorf("use %d: the client was told an item or a charge was spent", i+1)
		}
		n, left := breaths()
		if n != 1 || left < 4*time.Hour-time.Minute || left > 4*time.Hour {
			t.Fatalf("use %d: %d breath effects, %v left; want one, four hours", i+1, n, left)
		}
		if f.stackOf(JarOfBreathName) != 1 {
			t.Fatalf("use %d: %d jars left, want the one", i+1, f.stackOf(JarOfBreathName))
		}
	}

	// the control: the same line on a potion is drunk and taken
	pot := f.potion("Breath Draught", -1, 3, 1, jarLine, "", "")
	f.own(pot, 1)
	f.loadList()
	if out := f.use(1); !spent(out) || f.stackOf("Breath Draught") != 0 {
		t.Fatalf("a potion of the jar's line: spent %v, %d left; want spent, none", spent(out), f.stackOf("Breath Draught"))
	}
}

// A jar with no itemeffect row says so and puts nothing on.
func TestTheJarOfBreathWithoutItsRowIsQuiet(t *testing.T) {
	f := newScribeFixture(t)
	f.potionWorld()
	jar := addJarOfBreath(t, f.db, false)
	f.own(jar, 1)
	f.loadList()
	f.sess.Queue().GetPending()
	f.use(0)
	if len(f.player.Effects) != 0 || f.stackOf(JarOfBreathName) != 1 {
		t.Fatalf("no row: %d effects, %d jars", len(f.player.Effects), f.stackOf(JarOfBreathName))
	}
}

// opBytes is an opcode as a message's data begins with it, little-endian.
func opBytes(op protocol.GameOpcode) []byte { return []byte{byte(op), byte(op >> 8)} }
