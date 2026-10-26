package handlers

import (
	"bytes"
	"encoding/binary"
	"math"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// The writers in ps2opcodes.go, checked against the bytes CLIENT's handlers
// read (decomp/modules/client/src/viclient.cpp). vimsReader mirrors the
// VIMemStream reads those handlers use.

type vimsReader struct {
	b   []byte
	p   int
	err bool
}

func (r *vimsReader) raw(n int) []byte {
	if r.p+n > len(r.b) {
		r.err = true
		return make([]byte, n)
	}
	v := r.b[r.p : r.p+n]
	r.p += n
	return v
}
func (r *vimsReader) u8() byte    { return r.raw(1)[0] }
func (r *vimsReader) u16() uint16 { return binary.LittleEndian.Uint16(r.raw(2)) }
func (r *vimsReader) u32() uint32 { return binary.LittleEndian.Uint32(r.raw(4)) }

// signed is VIMemStream::ReadSigned: seven bits a byte gathered into 32 bits,
// then zigzag.
func (r *vimsReader) signed() int32 {
	var u uint32
	var shift uint
	for {
		b := r.u8()
		if r.err {
			return 0
		}
		u |= uint32(b&0x7F) << (shift & 31)
		shift += 7
		if b&0x80 == 0 {
			break
		}
	}
	return int32(u>>1) ^ -int32(u&1)
}

// str8 is ReadString(char *, maxBytes).
func (r *vimsReader) str8(maxBytes int) string {
	n := int(r.u32())
	b := r.raw(n)
	if n+1 > maxBytes {
		return ""
	}
	return string(b)
}

// wide is ReadString(unsigned short *, maxBytes).
func (r *vimsReader) wide(maxBytes int) string {
	n := int(r.u32())
	b := r.raw(2 * n)
	if n+1 > maxBytes/2 {
		return ""
	}
	u := make([]uint16, n)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	return string(utf16.Decode(u))
}

func (r *vimsReader) done(t *testing.T) {
	t.Helper()
	if r.err {
		t.Fatalf("read past the end")
	}
	if r.p != len(r.b) {
		t.Fatalf("%d bytes left over", len(r.b)-r.p)
	}
}

// open checks the opcode and returns a reader positioned after it.
func open(t *testing.T, payload []byte, op protocol.GameOpcode) *vimsReader {
	t.Helper()
	if len(payload) < 2 || protocol.GameOpcode(binary.LittleEndian.Uint16(payload)) != op {
		t.Fatalf("payload % X does not start with opcode 0x%04X", payload[:min(len(payload), 4)], uint16(op))
	}
	return &vimsReader{b: payload, p: 2}
}

func TestWideStringIsCountedInUnitsAndCut(t *testing.T) {
	enc := func(s string, maxBytes int) []byte {
		buf := make([]byte, 8192)
		w := buffer.NewWriter(buf)
		writeWideString(w, s, maxBytes)
		return buf[:w.Position()]
	}
	if got, want := enc("hé", 2048), []byte{2, 0, 0, 0, 'h', 0, 0xE9, 0}; !bytes.Equal(got, want) {
		t.Fatalf("hé: % X, want % X", got, want)
	}
	// A character past the BMP is two units, and the count says two.
	if got := enc("\U0001F600", 2048); binary.LittleEndian.Uint32(got) != 2 || len(got) != 8 {
		t.Fatalf("emoji: % X", got)
	}
	// CLIENT keeps count+1 <= MaxBytes/2: 1023 units for 2048.
	r := &vimsReader{b: enc(strings.Repeat("a", 1500), 2048)}
	if s := r.wide(2048); len(s) != 1023 {
		t.Fatalf("cut to %d, want 1023", len(s))
	}
	r.done(t)
	// A cut never leaves half a surrogate pair.
	got := enc(strings.Repeat("a", 62)+"\U0001F600", 128)
	if n := binary.LittleEndian.Uint32(got); n != 62 {
		t.Fatalf("surrogate cut: %d units, want 62", n)
	}
}

func TestString8IsCut(t *testing.T) {
	buf := make([]byte, 64)
	w := buffer.NewWriter(buf)
	writeString8(w, strings.Repeat("x", 30), 24)
	r := &vimsReader{b: buf[:w.Position()]}
	if s := r.str8(24); len(s) != 23 {
		t.Fatalf("cut to %d, want 23", len(s))
	}
	r.done(t)
}

func TestDisconnects(t *testing.T) {
	if got := buildDisconnect(protocol.OpcodeBadLoginPassword); !bytes.Equal(got, []byte{0x29, 0x07}) {
		t.Fatalf("0x0729: % X", got)
	}
	// Msg07A1 reads eight bytes before it disconnects.
	if got := buildDisconnect(protocol.OpcodeZoneUnavailableDC); len(got) != 10 || got[0] != 0xA1 || got[1] != 0x07 {
		t.Fatalf("0x07A1: % X", got)
	}
	for _, op := range []protocol.GameOpcode{protocol.OpcodeCreateCharFailed, protocol.OpcodeCreateCharBadName, protocol.OpcodeNameTaken} {
		if got := buildCreateCharFailed(op); len(got) != 2 {
			t.Fatalf("0x%04X carries % X", uint16(op), got)
		}
	}
	if got := buildCharSelectNotice(protocol.OpcodeCharSelectNeedsFrontiers); !bytes.Equal(got, []byte{0xF4, 0x07}) {
		t.Fatalf("0x07F4: % X", got)
	}
}

func TestAbilityAddIsVIAbilityRead(t *testing.T) {
	e := &database.CharSpellEntry{
		AddedOrder: 9, OnHotBar: 1, WhereOnHotBar: 4, Unk1: 2500, ShowHide: 1,
		Pattern: database.SpellPatternRecord{
			SpellID: 143, AbilityLvl: 3, Range: 0x41A0, // 20.0f's high half
			CastTime: 1, Power: 36, IconColor: 0xC0A1B2C3, Icon: -7, Scope: 2,
			Recast: 5, EqpRequire: 0x10, SpellName: "Chant of Battle", SpellDesc: "Buff STR,DEX",
		},
	}
	r := open(t, buildAbilityAdd(0x01020304, true, e), protocol.OpcodeAbilityAdd)
	if stamp, flag := r.u32(), r.u32(); stamp != 0x01020304 || flag != 1 {
		t.Fatalf("stamp %X flag %d", stamp, flag)
	}
	ints := []int32{r.signed(), r.signed(), r.signed(), r.signed(), r.signed(), r.signed(), r.signed()}
	if want := []int32{143, 9, 1, 4, 2500, 1, 3}; !equalInts(ints, want) {
		t.Fatalf("first seven %v, want %v", ints, want)
	}
	if rng := math.Float32frombits(r.u32()); rng != 20.0 {
		t.Fatalf("range %v, want 20", rng)
	}
	ints = []int32{r.signed(), r.signed(), r.signed(), r.signed(), r.signed(), r.signed(), r.signed()}
	// An icon hash stored unsigned comes back as the same 32 bits.
	if want := []int32{1, 36, int32(-1063144765), -7, 2, 5, 0x10}; !equalInts(ints, want) {
		t.Fatalf("second seven %v, want %v", ints, want)
	}
	if name, desc := r.wide(128), r.wide(256); name != "Chant of Battle" || desc != "Buff STR,DEX" {
		t.Fatalf("name %q desc %q", name, desc)
	}
	if n := r.signed(); n != 0 {
		t.Fatalf("%d sub-records", n)
	}
	r.done(t)
}

func TestPlayerRecordWriters(t *testing.T) {
	if got, want := buildTrainingPoints(true, 25), []byte{0x36, 0x00, 0x01, 50}; !bytes.Equal(got, want) {
		t.Fatalf("0x0036: % X, want % X", got, want)
	}
	if got, want := buildTrainingPointsSpent(3), []byte{0x1D, 0x00, 6}; !bytes.Equal(got, want) {
		t.Fatalf("0x001D: % X, want % X", got, want)
	}
	fixed := []struct {
		name string
		got  []byte
		want []byte
	}{
		{"0x001C", buildPlayerP44(0x11223344), []byte{0x1C, 0x00, 0x44, 0x33, 0x22, 0x11}},
		{"0x00D9", buildExpDebt(7), []byte{0xD9, 0x00, 7, 0, 0, 0}},
		{"0x007E", buildSetPet(0xAABBCCDD), []byte{0x7E, 0x00, 0xDD, 0xCC, 0xBB, 0xAA}},
		{"0x007F", buildClearPet(), []byte{0x7F, 0x00}},
		{"0x007B", buildAssistTarget(0xFFFFFFFF), []byte{0x7B, 0x00, 0xFF, 0xFF, 0xFF, 0xFF}},
		{"0x0775", buildEntityAction(0x01020304, 34), []byte{0x75, 0x07, 4, 3, 2, 1, 34}},
		{"0x07B2", buildZoneTransferConfirm(100), []byte{0xB2, 0x07, 100, 0, 0, 0}},
		{"0x001A", buildLootTakeAll(), []byte{0x1A, 0x00}},
	}
	for _, c := range fixed {
		if !bytes.Equal(c.got, c.want) {
			t.Errorf("%s: % X, want % X", c.name, c.got, c.want)
		}
	}
}

func TestClassicTargetInfoEndsWithTheToken(t *testing.T) {
	got := buildClassicTargetInfo(0xDEADBEEF, 2, 5, 1, "Title", "Guild")
	if len(got) != 265 {
		t.Fatalf("%d bytes, want 265", len(got))
	}
	r := open(t, got, protocol.OpcodeClassicTargetInfo)
	if d, b, c := r.u8(), r.u8(), r.u8(); d != 2 || b != 5 || c != 1 {
		t.Fatalf("bytes %d %d %d", d, b, c)
	}
	readFixed := func() string {
		u := make([]uint16, 64)
		for i := range u {
			u[i] = r.u16()
		}
		n := 0
		for n < len(u) && u[n] != 0 {
			n++
		}
		return string(utf16.Decode(u[:n]))
	}
	// Msg0010 reads the title first (VIClient+0x4FCA8, shown in place of the
	// class), then the guild (+0x4FC28, shown under the name)
	if ti, g := readFixed(), readFixed(); ti != "Title" || g != "Guild" {
		t.Fatalf("lines %q %q, want the title then the guild", ti, g)
	}
	if tok := r.u32(); tok != 0xDEADBEEF {
		t.Fatalf("token %X", tok)
	}
	r.done(t)
}

func TestLootIsAnItemList(t *testing.T) {
	items := make([]database.CharInventoryEntry, maxCorpseItems+5)
	r := open(t, buildLoot(items), protocol.OpcodeLoot)
	if n, next := r.signed(), r.u32(); n != maxCorpseItems || next != maxCorpseItems {
		t.Fatalf("count %d, next key %d; want %d each", n, next, maxCorpseItems)
	}
}

func TestTextWriters(t *testing.T) {
	if got, want := coloredChatPayload("hi", 1, 2, 3), []byte{0x7B, 0x0A, 2, 0, 0, 0, 'h', 0, 'i', 0, 1, 2, 3}; !bytes.Equal(got, want) {
		t.Fatalf("0x0A7B: % X, want % X", got, want)
	}
	if got := buildColoredMessage("hi").Data(); !bytes.HasSuffix(got, []byte{'i', 0, 0x3F, 0xFF, 0x3F}) || len(got) != 13 {
		t.Fatalf("buildColoredMessage: % X", got)
	}
	for _, c := range []struct {
		op  protocol.GameOpcode
		got []byte
	}{
		{protocol.OpcodeClientMessage, clientMessagePayload(strings.Repeat("a", 2000))},
		{protocol.OpcodeErrorMessage, buildErrorMessage(strings.Repeat("a", 2000))},
	} {
		r := open(t, c.got, c.op)
		if s := r.wide(2048); len(s) != 1023 {
			t.Fatalf("0x%04X: %d characters, want 1023", uint16(c.op), len(s))
		}
		r.done(t)
	}
	r := open(t, buildReplyName("Mcgregor"), protocol.OpcodeReplyName)
	if s := r.str8(24); s != "Mcgregor" {
		t.Fatalf("reply name %q", s)
	}
	r.done(t)
	r = open(t, buildSummonPrompt("Healer", true), protocol.OpcodeSummonPrompt)
	if s, c := r.str8(64), r.u8(); s != "Healer" || c != 1 {
		t.Fatalf("summon %q %d", s, c)
	}
	r.done(t)
}

func TestDialogsAndJournal(t *testing.T) {
	r := open(t, buildDialogueBox(7, strings.Repeat("d", 600)), protocol.OpcodeDialogueBox)
	if id, s := r.u32(), r.wide(1024); id != 7 || len(s) != 511 {
		t.Fatalf("0x0046: id %d, %d characters", id, len(s))
	}
	r.done(t)

	opts := []string{"one", strings.Repeat("o", 80), "3", "4", "5", "6", "7", "8"}
	r = open(t, buildQuestPopupMulti(9, strings.Repeat("q", 200), opts), protocol.OpcodeQuestPopupMulti)
	if id, q := r.u32(), r.wide(256); id != 9 || len(q) != 127 {
		t.Fatalf("0x0034: id %d, question %d characters", id, len(q))
	}
	n := int(r.u8())
	if n != maxDialogOptions {
		t.Fatalf("%d options, want %d", n, maxDialogOptions)
	}
	for i := 0; i < n; i++ {
		o := r.wide(128)
		if i == 1 && len(o) != 63 {
			t.Fatalf("option cut to %d, want 63", len(o))
		}
	}
	r.done(t)

	r = open(t, buildQuestAdd(0x55, "A Title"), protocol.OpcodeAddQuestLog)
	if tok, s := r.u32(), r.wide(256); tok != 0x55 || s != "A Title" {
		t.Fatalf("0x007C: %X %q", tok, s)
	}
	r.done(t)
	r = open(t, buildQuestRemove(0x55, 3), protocol.OpcodeDeleteQuest)
	if tok, i := r.u32(), r.signed(); tok != 0x55 || i != 3 {
		t.Fatalf("0x007D: %X %d", tok, i)
	}
	r.done(t)
}

func TestGroupWriters(t *testing.T) {
	r := open(t, buildGroupInvite(11, 22, strings.Repeat("n", 40)), protocol.OpcodeGroupInviteReceived)
	if id, x, name := r.u32(), r.u32(), r.str8(24); id != 11 || x != 22 || len(name) != 23 {
		t.Fatalf("0x0773: %d %d %q", id, x, name)
	}
	r.done(t)
	r = open(t, buildGroupChat(5, "hello"), protocol.OpcodeGroupChat)
	if id, s := r.u32(), r.wide(2048); id != 5 || s != "hello" {
		t.Fatalf("0x069E: %d %q", id, s)
	}
	r.done(t)
}

func equalInts(a, b []int32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
