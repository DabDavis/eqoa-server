package handlers

import (
	"encoding/binary"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// selectWith is the 0x002A a client sends to play a character: its id, then the
// face, hair style, hair length and hair colour its select screen holds
// (VIClient::SelectCharacter), and 0x0014 when the world is in.
func (r *takeoverRig) selectWith(s *session.Session, id uint32, look ...uint32) {
	r.t.Helper()
	data := u32b(id)
	for _, v := range look {
		data = binary.LittleEndian.AppendUint32(data, v)
	}
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeSelectedChar)
	msg.SetData(data)
	if err := r.h.HandleSelectedChar(s, msg); err != nil {
		r.t.Fatal(err)
	}
	if s.InGame() {
		if err := r.h.HandleCharacterInWorld(s, protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCharacterInWorld)); err != nil {
			r.t.Fatal(err)
		}
	}
}

// rowLook is the row's look: face, hair style, hair length, hair colour.
func (r *takeoverRig) rowLook(id uint32) [4]int {
	r.t.Helper()
	row, err := r.store.GetByServerID(id)
	if err != nil {
		r.t.Fatal(err)
	}
	return [4]int{row.FaceOption, row.HairStyle, row.HairLength, row.HairColor}
}

// listedLook is what a new session's character list (0x002C) says of its first
// character: hair colour, length and style and the face, as zigzags after the
// name, the id, the model, the class, the race and the level.
func (r *takeoverRig) listedLook(account int) [4]int {
	r.t.Helper()
	s := r.window(account)
	r.h.SendCharacterSelectEQOA(s)
	d := sentOp(s, protocol.OpcodeCharacterSelect)
	if d == nil {
		r.t.Fatal("no character list sent")
	}
	rd := buffer.NewReader(d[2:])                        // past the opcode
	if _, err := rd.Read7BitEncodedInt64(); err != nil { // the count
		r.t.Fatal(err)
	}
	n, err := rd.ReadInt32()
	if err != nil {
		r.t.Fatal(err)
	}
	if _, err := rd.ReadStringUTF8(int(n)); err != nil {
		r.t.Fatal(err)
	}
	var f [9]int64
	for i := range f {
		if f[i], err = rd.Read7BitEncodedInt64(); err != nil {
			r.t.Fatal(err)
		}
	}
	return [4]int{int(f[8]), int(f[7]), int(f[6]), int(f[5])}
}

// channelLook is what the character's state channel says: bytes 105-108, hair
// colour, length and style and the face.
func channelLook(p *entity.Player) [4]int {
	b := entity.SerializePlayerUpdate(1, p)
	return [4]int{int(b[108]), int(b[107]), int(b[106]), int(b[105])}
}

// annika is the character these tests choose: account 7's, its look
// face 1, hair style 2, length 2, colour 0.
func annika() *database.CharacterRecord {
	return &database.CharacterRecord{CharName: "Annika", AccountID: 7, Level: 5, Race: 8, TClass: 14,
		FaceOption: 1, HairStyle: 2, HairLength: 2, HairColor: 0, X: 15180.75, Y: 54.13, Z: 22745.81}
}

// Character select's Customize Character (Options, then the four rows, Accept)
// changes the look only on the client (SetCharLook); the 0x002A that plays the
// character is the one message that carries it. The server read the id alone,
// so the world showed the old look and the next login's list did too (players,
// 2026-11-07: "hair color, style, etc. is not saving"). The look now goes into
// the row, the world and the list; the wire is the disc's order, not the row's.
func TestPlayingACharacterKeepsTheLookChosenAtSelect(t *testing.T) {
	ann := annika()
	r := newTakeoverRig(t, ann)
	want := [4]int{2, 3, 1, 5}

	s := r.window(7)
	r.selectWith(s, ann.ServerID, 2, 3, 1, 5)
	if got := r.rowLook(ann.ServerID); got != want {
		t.Errorf("the row's look %v, want %v", got, want)
	}
	p := r.w.SessionPlayer(s)
	if p == nil {
		t.Fatal("not in the world")
	}
	if got := channelLook(p); got != want {
		t.Errorf("the channel's look %v, want %v", got, want)
	}
	if got := r.listedLook(7); got != want {
		t.Errorf("the next list's look %v, want %v", got, want)
	}
}

// The controls: a message with no look leaves the row's (a client that sends the
// id alone); a look the pages could not make is refused whole; a character of
// another account is refused before its look is read; playing without changing
// anything writes nothing new.
func TestALookIsKeptOnlyFromItsOwnerAndInRange(t *testing.T) {
	ann := annika()
	was := [4]int{1, 2, 2, 0}

	r := newTakeoverRig(t, ann)
	r.selectWith(r.window(7), ann.ServerID)
	if got := r.rowLook(ann.ServerID); got != was {
		t.Errorf("the id alone: the row's look %v, want it kept as %v", got, was)
	}

	r = newTakeoverRig(t, annika())
	for _, bad := range [][]uint32{{4, 0, 0, 0}, {0, 4, 0, 0}, {0, 0, 4, 0}, {0, 0, 0, 8}, {0xFFFFFFFF, 0, 0, 0}} {
		s := r.window(7)
		r.selectWith(s, ann.ServerID, bad...)
		if got := r.rowLook(ann.ServerID); got != was {
			t.Errorf("look %v: the row's look %v, want it kept as %v", bad, got, was)
		}
	}

	r = newTakeoverRig(t, annika())
	r.selectWith(r.window(9), ann.ServerID, 3, 3, 3, 7)
	if got := r.rowLook(ann.ServerID); got != was {
		t.Errorf("another account's look: the row's look %v, want it kept as %v", got, was)
	}

	r = newTakeoverRig(t, annika())
	s := r.window(7)
	r.selectWith(s, ann.ServerID, 1, 2, 2, 0)
	if got := r.rowLook(ann.ServerID); got != was {
		t.Errorf("the same look: the row's look %v, want %v", got, was)
	}
}

// A character still in the world when it is chosen again is taken over as it
// stands (entity/takeover.go), but for the look its new window sends: the live
// Player wears it at once, as a new login's would be built in it.
func TestATakeoverWearsTheLookChosenAtSelect(t *testing.T) {
	ann := annika()
	r := newTakeoverRig(t, ann)
	first := r.window(7)
	r.selectWith(first, ann.ServerID, 1, 2, 2, 0)
	p := r.w.SessionPlayer(first)
	if p == nil {
		t.Fatal("not in the world")
	}
	if got := channelLook(p); got != [4]int{1, 2, 2, 0} {
		t.Fatalf("setup: the channel's look %v", got)
	}
	second := r.window(7)
	r.selectWith(second, ann.ServerID, 0, 1, 3, 6)
	if got := r.w.SessionPlayer(second); got != p {
		t.Fatalf("the new window has %p, want the live character %p", got, p)
	}
	want := [4]int{0, 1, 3, 6}
	if got := channelLook(p); got != want {
		t.Errorf("the live character's look %v, want %v", got, want)
	}
	if got := r.rowLook(ann.ServerID); got != want {
		t.Errorf("the row's look %v, want %v", got, want)
	}
}

// A refused 0x002B is answered with its refusal alone: a list after it makes
// CLIENT reset creation's form (CharacterList), so a taken name cost every choice
// the player had made. The control: a creation that goes through is answered
// with the list.
func TestARefusedCreationKeepsTheForm(t *testing.T) {
	r := newTakeoverRig(t, annika())
	create := func(s *session.Session, name string) {
		t.Helper()
		payload := build(protocol.OpcodeCreateCharacter, 128, func(w *buffer.Writer) {
			w.WriteInt32(int32(len(name)))
			w.WriteBytes([]byte(name))
			for _, v := range []int64{1, 0, 0, 0, 1, 1, 1, 1, 1} { // a human warrior of Freeport
				w.Write7BitEncodedInt64(v)
			}
			for i := 0; i < 7; i++ {
				w.WriteInt32(0)
			}
		})[2:]
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCreateCharacter)
		msg.SetData(payload)
		if err := r.h.HandleCreateCharacter(s, msg); err != nil {
			t.Fatal(err)
		}
	}
	taken := r.window(7)
	create(taken, "Annika")
	if sentOp(taken, protocol.OpcodeNameTaken) == nil {
		t.Fatal("no refusal for a taken name")
	}
	if sentOp(taken, protocol.OpcodeCharacterSelect) != nil {
		t.Error("a list followed the refusal: it resets the client's form")
	}
	bad := r.window(7)
	create(bad, "A1")
	if sentOp(bad, protocol.OpcodeCreateCharBadName) == nil {
		t.Fatal("no refusal for a bad name")
	}
	if sentOp(bad, protocol.OpcodeCharacterSelect) != nil {
		t.Error("a list followed the refusal of a bad name")
	}
	made := r.window(7)
	create(made, "Brunhilde")
	if sentOp(made, protocol.OpcodeCharacterSelect) == nil {
		t.Error("the control: no list after a character was made")
	}
}
