package entity

import (
	"encoding/binary"
	"math"
	"path/filepath"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Petitions 1765 and 1766 (the GM Jibrish): a title over the Testing Heroes
// Vendor's name, "XP Grinding Rewards", and over Dabrish's, "XP/CM's". A
// titled NPC in view brings its plate on the next slot: under the title, where
// it stands, on its own body at titleScale of its size (its name a quarter of
// the body's plate height over the NPC's own), not selectable, its skins
// hidden, nothing worn or held. An NPC with no title brings none; the NPC's own
// channel is unchanged.
func TestATitledNPCBringsItsTitlePlate(t *testing.T) {
	w, sess := slotsFixture(t, 0)
	vendor := &NPC{ServerID: 163008, Name: "Testing Heroes Vendor", X: 1005, Y: 54, Z: 1000, Facing: 1.25, ModelID: int64(bodyHumanMale),
		Size: 1, NPCType: 0x80, Level: 60, HP: 10, MaxHP: 10, Head: 3, Torso: 2}
	other := &NPC{ServerID: 162587, Name: "Feastmaster Holloway", X: 1006, Z: 1000, ModelID: int64(bodyHumanMale), Size: 1, NPCType: 0x80, Level: 1, HP: 10, MaxHP: 10}
	for _, n := range []*NPC{vendor, other} {
		w.NPCs[n.ServerID] = n
		w.SpatialIndex.Insert(n.ServerID, n.X, n.Z)
	}
	w.SetNPCTitles(map[EntityID]string{vendor.ServerID: "XP Grinding Rewards"})
	if vendor.Title != "XP Grinding Rewards" || other.Title != "" {
		t.Fatalf("titles %q, %q", vendor.Title, other.Title)
	}
	visible(w, sess)
	vs := GetOrCreateVisibility(sess)
	if vs.ChannelEntity[0] != vendor.ServerID || vs.ChannelEntity[1] != plateIDBase+vendor.ServerID || vs.ChannelEntity[2] != other.ServerID ||
		vs.ChannelEntity[3] != 0 {
		t.Fatalf("slots %v, want the vendor, its title, the other", vs.ChannelEntity[:4])
	}
	buf := w.serializeEntityForChannel(sess, vs.ChannelEntity[1], vs.ChannelObjectID[1])
	own := w.serializeEntityForChannel(sess, vendor.ServerID, vs.ChannelObjectID[1])
	if name := string(buf[116 : 116+len(vendor.Title)]); name != vendor.Title || buf[116+len(vendor.Title)] != 0 {
		t.Errorf("plate named %q", name)
	}
	if string(own[116:116+len(vendor.Name)]) != vendor.Name {
		t.Errorf("the vendor itself named %q", own[116:140])
	}
	if model := binary.LittleEndian.Uint32(buf[28:32]); model != bodyHumanMale {
		t.Errorf("plate's body %#x, want the vendor's", model)
	}
	if size := math.Float32frombits(binary.LittleEndian.Uint32(buf[36:40])); math.Abs(float64(size-1.25)) > 1e-6 {
		t.Errorf("plate's size %v, want 1.25", size)
	}
	for _, r := range [][2]int{{5, 18}, {140, 141}, {145, 147}} {
		for i := r[0]; i < r[1]; i++ {
			if buf[i] != own[i] {
				t.Errorf("byte %d: plate %#x, vendor %#x", i, buf[i], own[i])
			}
		}
	}
	if buf[26] != 0 || buf[154] != 1 || buf[143] != 0 {
		t.Errorf("plate: selectable %d, hidden %d, fight %d; want 0, 1, 0", buf[26], buf[154], buf[143])
	}
	for i := 93; i <= 98; i++ {
		if buf[i] != 0 {
			t.Errorf("plate wears armour set %d at byte %d", buf[i], i)
		}
	}
	if own[26] != 1 || own[154] != 0 {
		t.Errorf("the vendor itself changed: selectable %d, hidden %d", own[26], own[154])
	}

	// a bigger vendor, a bigger plate; its title taken away, the plate goes
	vendor.Size = 2
	buf = w.serializeEntityForChannel(sess, vs.ChannelEntity[1], vs.ChannelObjectID[1])
	if size := math.Float32frombits(binary.LittleEndian.Uint32(buf[36:40])); math.Abs(float64(size-2.5)) > 1e-6 {
		t.Errorf("size 2 vendor's plate's size %v, want 2.5", size)
	}
	w.SetNPCTitles(nil)
	visible(w, sess)
	for _, id := range vs.ChannelEntity {
		if id >= plateIDBase {
			t.Errorf("a plate %#x in view of an NPC with no title", id)
		}
	}
}

// A board keeps its own name on its plate, a title or not; a title is cut to a
// name's 24 letters; an NPC made again from its row takes its row's title, a
// copy of it (Temporary) none.
func TestTitleRules(t *testing.T) {
	if got := cleanTitle("  XP/CM's "); got != "XP/CM's" {
		t.Errorf("cleanTitle: %q", got)
	}
	if got := cleanTitle("abcdefghijklmnopqrstuvwxyz"); got != "abcdefghijklmnopqrstuvwx" {
		t.Errorf("cleanTitle long: %q", got)
	}
	w, _ := slotsFixture(t, 0)
	board := &NPC{ServerID: 162678, Name: "Mercenary Recruitment", ModelID: int64(PaperedBoardBody), Size: 1, NPCType: 0x80, Level: 1}
	dab := &NPC{ServerID: 162568, Name: "Dabrish", ModelID: 0x61CA3EDF, Size: 1, NPCType: 0x80, Level: 60}
	cp := &NPC{ServerID: 900001, Name: "Dabrish", ModelID: 0x61CA3EDF, Size: 1, Temporary: true}
	for _, n := range []*NPC{board, dab, cp} {
		w.NPCs[n.ServerID] = n
	}
	w.SetNPCTitles(map[EntityID]string{board.ServerID: "Hires", dab.ServerID: "XP/CM's", cp.ServerID: "no"})
	if cp.Title != "" {
		t.Errorf("a copy took a title %q", cp.Title)
	}
	buf := serializePlate(1, board)
	if string(buf[116:116+len(board.Name)]) != board.Name {
		t.Errorf("a titled board's plate named %q", buf[116:140])
	}
	buf = serializePlate(2, dab)
	if string(buf[116:123]) != "XP/CM's" || binary.LittleEndian.Uint32(buf[28:32]) != 0x61CA3EDF {
		t.Errorf("Dabrish's plate: %q on %#x", buf[116:123], binary.LittleEndian.Uint32(buf[28:32]))
	}
	again := &NPC{ServerID: dab.ServerID, Name: "Dabrish"}
	w.mu.Lock()
	w.applyNPCSettingsLocked(again)
	w.mu.Unlock()
	if again.Title != "XP/CM's" {
		t.Errorf("made again from its row: title %q", again.Title)
	}
}

// The table: made by the schema, read with its blank rows left out.
func TestLoadNPCTitles(t *testing.T) {
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO npctitle (npcid, title) VALUES (163008, 'XP Grinding Rewards'), (162568, 'XP/CM''s'), (5, '')`); err != nil {
		t.Fatal(err)
	}
	w := NewWorld()
	n := &NPC{ServerID: 163008, Name: "Testing Heroes Vendor"}
	w.NPCs[n.ServerID] = n
	got, err := w.LoadNPCTitles(db)
	if err != nil || got != 2 || n.Title != "XP Grinding Rewards" {
		t.Errorf("loaded %d (%v), the vendor's %q", got, err, n.Title)
	}
}
