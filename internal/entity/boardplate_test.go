package entity

import (
	"encoding/binary"
	"math"
	"testing"
)

// A papered board in view brings its plate on the next slot: under the board's
// name, where it stands, on the human's body at the size that puts its name
// half a unit over the board's top, not selectable, its skins hidden, nothing
// worn, never a fight's. A plain sign post brings none.
func TestAPaperedBoardBringsItsNamePlate(t *testing.T) {
	w, sess := slotsFixture(t, 0)
	board := &NPC{ServerID: 162678, Name: "Mercenary Recruitment", X: 1005, Y: 54, Z: 1000, Facing: 1.25, ModelID: int64(PaperedBoardBody),
		Size: 1, NPCType: 0x80, Level: 1, HP: 10, MaxHP: 10}
	sign := &NPC{ServerID: 645, Name: "signpost", X: 1006, Z: 1000, ModelID: 0x6A82352C, Size: 1, NPCType: 0x80, Level: 1, HP: 10, MaxHP: 10}
	for _, n := range []*NPC{board, sign} {
		w.NPCs[n.ServerID] = n
		w.SpatialIndex.Insert(n.ServerID, n.X, n.Z)
	}
	got := visible(w, sess)
	if len(got) != 3 {
		t.Fatalf("in view %v, want the board, its plate and the sign", got)
	}
	vs := GetOrCreateVisibility(sess)
	if vs.ChannelEntity[0] != board.ServerID || vs.ChannelEntity[1] != plateIDBase+board.ServerID || vs.ChannelEntity[2] != sign.ServerID {
		t.Fatalf("slots %v, want the board, its plate, the sign", vs.ChannelEntity[:3])
	}
	buf := w.serializeEntityForChannel(sess, vs.ChannelEntity[1], vs.ChannelObjectID[1])
	own := w.serializeEntityForChannel(sess, board.ServerID, vs.ChannelObjectID[1])
	if buf == nil || len(buf) != EntityStateSize {
		t.Fatalf("plate's channel %d bytes", len(buf))
	}
	name := string(buf[116 : 116+len(board.Name)])
	if name != board.Name || buf[116+len(board.Name)] != 0 {
		t.Errorf("plate named %q", name)
	}
	if model := binary.LittleEndian.Uint32(buf[28:32]); model != bodyHumanMale {
		t.Errorf("plate's body %#x, want the human's", model)
	}
	size := math.Float32frombits(binary.LittleEndian.Uint32(buf[36:40]))
	if top := size * humanPlate; math.Abs(float64(top-3.5)) > 1e-5 {
		t.Errorf("plate's size %v: its name at %v, want 3.5", size, top)
	}
	// the board's own place, facing, world, NPC bits and level
	for _, r := range [][2]int{{5, 18}, {140, 141}, {145, 147}} {
		for i := r[0]; i < r[1]; i++ {
			if buf[i] != own[i] {
				t.Errorf("byte %d: plate %#x, board %#x", i, buf[i], own[i])
			}
		}
	}
	if buf[26] != 0 || buf[154] != 1 || buf[143] != 0 || buf[27] != 255 {
		t.Errorf("plate: selectable byte %d, hidden %d, plate/fight %d, health %d; want 0, 1, 0, 255", buf[26], buf[154], buf[143], buf[27])
	}
	for i := 67; i < 79; i++ {
		if buf[i] != 0 {
			t.Errorf("plate holds something: byte %d = %#x", i, buf[i])
		}
	}
	for i := 93; i <= 98; i++ {
		if buf[i] != 0 {
			t.Errorf("plate wears armour set %d at byte %d", buf[i], i)
		}
	}
	// the plate holds still (plateLoco), its NPC keeps its own idle
	if buf[58] != plateLoco || own[58] != LocoStand {
		t.Errorf("locomotion: plate %d, board %d; want %d, %d", buf[58], own[58], plateLoco, LocoStand)
	}
	if own[26] != 1 || own[154] != 0 {
		t.Errorf("the board itself changed: selectable %d, hidden %d", own[26], own[154])
	}

	// a bigger board, a bigger plate; moved and renamed, the plate follows
	board.Size, board.X, board.Name = 2, 1004, "Real Estate Listings"
	buf = w.serializeEntityForChannel(sess, vs.ChannelEntity[1], vs.ChannelObjectID[1])
	if top := math.Float32frombits(binary.LittleEndian.Uint32(buf[36:40])) * humanPlate; math.Abs(float64(top-6.5)) > 1e-5 {
		t.Errorf("size 2 board's plate's name at %v, want 6.5", top)
	}
	if string(buf[116:116+len(board.Name)]) != board.Name {
		t.Errorf("renamed board's plate named %q", buf[116:140])
	}
	if x := int32(buf[6])<<16 | int32(buf[7])<<8 | int32(buf[8]); x != 1004*128 {
		t.Errorf("moved board's plate at x %v", float32(x)/128)
	}

	// the board out of view (another body): its plate goes with it
	board.ModelID = 0x70D898C6
	visible(w, sess)
	for _, id := range vs.ChannelEntity {
		if id >= plateIDBase {
			t.Errorf("a plate %#x in view of a board on a CSprite body", id)
		}
	}
	if w.plateBoardLocked(plateIDBase+board.ServerID) != nil {
		t.Error("a plate id still names a board with no plate")
	}
}

// A plate is no NPC of the world: none of World.NPCs or the spatial index, so
// nothing that walks them meets it.
func TestAPlateIsNoNPCOfTheWorld(t *testing.T) {
	w, sess := slotsFixture(t, 0)
	board := &NPC{ServerID: 163625, Name: "Mercenary Recruitment", X: 1005, Z: 1000, ModelID: int64(PaperedBoardBody), Size: 1, NPCType: 0x80, Level: 1, HP: 10, MaxHP: 10}
	w.NPCs[board.ServerID] = board
	w.SpatialIndex.Insert(board.ServerID, board.X, board.Z)
	visible(w, sess)
	if len(w.NPCs) != 1 {
		t.Errorf("%d NPCs, want the board alone", len(w.NPCs))
	}
	for _, id := range w.SpatialIndex.QueryRadius(1000, 1000, 50) {
		if id >= plateIDBase {
			t.Errorf("plate %#x in the spatial index", id)
		}
	}
	if w.plateBoardLocked(board.ServerID) != nil || w.plateBoardLocked(plateIDBase+999) != nil {
		t.Error("a board's own id, or a plate of no board, taken for a plate")
	}
}

// A keep restart keeps a plate on its slot as its board's: the client's
// entity there goes on being described under the same object id.
func TestAKeepRestartKeepsAPlate(t *testing.T) {
	w, sess := slotsFixture(t, 0)
	board := &NPC{ServerID: 162678, Name: "Mercenary Recruitment", X: 1005, Z: 1000, ModelID: int64(PaperedBoardBody), Size: 1, NPCType: 0x80, Level: 1, HP: 10, MaxHP: 10}
	w.NPCs[board.ServerID] = board
	w.SpatialIndex.Insert(board.ServerID, board.X, board.Z)
	visible(w, sess)
	k := w.KeepView(sess)
	if len(k.Slots) < 2 || k.Slots[1].Kind != "plate" || k.Slots[1].ID != uint32(board.ServerID) || k.Slots[1].Name != board.Name {
		t.Fatalf("kept slots %+v, want the board then its plate", k.Slots[:2])
	}
	obj := k.Slots[1].Object
	vs := GetOrCreateVisibility(sess)
	for i := range vs.ChannelEntity {
		vs.ChannelEntity[i], vs.ChannelObjectID[i] = 0, 0
	}
	if kept, gone := w.RestoreView(sess, k); kept != 2 || gone != 0 {
		t.Fatalf("restored %d kept, %d gone; want 2, 0", kept, gone)
	}
	if vs.ChannelEntity[1] != plateIDBase+board.ServerID || vs.ChannelObjectID[1] != obj {
		t.Errorf("slot 1 is %#x under %d, want the plate under %d", vs.ChannelEntity[1], vs.ChannelObjectID[1], obj)
	}
	// renamed over the restart: another board's, taken away
	board.Name = "Other"
	for i := range vs.ChannelEntity {
		vs.ChannelEntity[i], vs.ChannelObjectID[i] = 0, 0
	}
	if _, gone := w.RestoreView(sess, k); gone != 2 {
		t.Errorf("a renamed board's plate kept (gone %d)", gone)
	}
}

// Which NPCs are boards with a plate: the papered board whatever its name; on
// the sign post and the grave's body (no name node either) one named as a
// board; nothing else on them, and no CSprite with a name node of its own.
func TestWhichBoardsHaveAPlate(t *testing.T) {
	for _, c := range []struct {
		name  string
		model uint32
		size  float32
		want  bool
		top   float32 // its name's height over its feet
	}{
		{"Mercenary Recruitment", PaperedBoardBody, 1, true, 3.5},
		{"Real Estate Listings", PaperedBoardBody, 1, true, 3.5},
		{"PvP Events", PaperedBoardBody, 1, true, 3.5},
		{"Freeport Announcements", SignpostBody, 1, true, 4.266},
		{"Surefall Anouncements", SignpostBody, 1, true, 4.266},
		{"Guild Board", 0x6DE5B2CC, 0.3, true, 1.025},
		{"Duel Tournament Board", 0x6DE5B2CC, 0.5, true, 1.375},
		{"signpost", SignpostBody, 1, false, 0},
		{"Akessan Bridge", SignpostBody, 1, false, 0},
		{"cursed grave", 0x6DE5B2CC, 1, false, 0},
		{"Bookshelf", 0x6DE5B2CC, 0.3, false, 0},
		{"Herald Moonbright", bodyHumanMale, 1, false, 0},
		{"Guild Board", bodyHumanMale, 1, false, 0},
		{"", PaperedBoardBody, 1, false, 0},
	} {
		n := &NPC{Name: c.name, ModelID: int64(c.model), Size: c.size}
		if got := n.hasPlate(); got != c.want {
			t.Errorf("%q on %#x: plate %v, want %v", c.name, c.model, got, c.want)
		}
		if c.want {
			if top := plateSize(n) * humanPlate; math.Abs(float64(top-c.top)) > 1e-4 {
				t.Errorf("%q: name at %v, want %v", c.name, top, c.top)
			}
		}
	}
	dead := &NPC{Name: "Mercenary Recruitment", ModelID: int64(PaperedBoardBody), Dead: true}
	if dead.hasPlate() {
		t.Error("a dead board has a plate")
	}
	// an illusion's body is the one drawn: a board shown as a human has none
	ill := &NPC{Name: "Mercenary Recruitment", ModelID: int64(PaperedBoardBody), Size: 1, Effects: []*ActiveEffect{{Form: bodyHumanMale}}}
	if ill.hasPlate() {
		t.Error("a board in a human's illusion has a plate")
	}
}
