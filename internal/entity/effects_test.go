package entity

import (
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/eqoa/server/internal/session"
)

// channel43 reads a value the way CLIENT's Channel43 (0x00BD3D18) does: the
// u32 count, at most 8, then for record i the u32 icon and the name by
// VIMemStream::ReadString(name, 64) (a u32 character count, and the characters
// only when count + 1 <= 32, else skipped and shown empty), then on to
// 4 + stride*(i+1). A read past the end fails, and so does every read after.
func channel43(data []byte, stride int) (icons []uint32, names []string, short bool) {
	pos, bad := 0, false
	take := func(n int) []byte {
		if bad || len(data)-pos < n {
			bad = true
			return nil
		}
		pos += n
		return data[pos-n : pos]
	}
	b := take(4)
	if b == nil {
		return nil, nil, true
	}
	count := int32(binary.LittleEndian.Uint32(b))
	if count > 8 {
		count = 8
	}
	for i := 0; i < int(count); i++ {
		var icon uint32
		if b := take(4); b != nil {
			icon = binary.LittleEndian.Uint32(b)
		}
		name := ""
		if b := take(4); b != nil {
			n := binary.LittleEndian.Uint32(b)
			if c := take(int(n) * 2); n+1 <= 32 && c != nil {
				u := make([]uint16, n)
				for j := range u {
					u[j] = binary.LittleEndian.Uint16(c[2*j:])
				}
				name = string(utf16.Decode(u))
			}
		}
		icons, names = append(icons, icon), append(names, name)
		pos = 4 + stride*(i+1)
	}
	return icons, names, bad
}

// The value is 4 + 132 bytes an effect, and CLIENT reads back every icon and
// name from it; at the 66-byte stride the server used to write, the second
// record is not there.
func TestEffectsChannelIsCLIENTsRecords(t *testing.T) {
	p := &Player{Buffs: []BuffSlot{{Icon: 0x1A2B3C4D, Name: "First Effect"}, {Icon: 0x00C0FFEE, Name: "Second"}, {Icon: 7}}}
	v := SerializeBuffUpdate(p)
	if len(v) != 4+3*EffectRecordSize {
		t.Fatalf("%d bytes for 3 effects, want %d", len(v), 4+3*EffectRecordSize)
	}
	icons, names, short := channel43(v, EffectRecordSize)
	if short || len(icons) != 3 {
		t.Fatalf("read %d records (short %v)", len(icons), short)
	}
	for i, e := range p.Buffs {
		if icons[i] != e.Icon || names[i] != e.Name {
			t.Errorf("record %d read as %08X %q, want %08X %q", i, icons[i], names[i], e.Icon, e.Name)
		}
	}
	// past each name, zeros to the record's end
	for i := 4 + 8 + 2*len("First Effect"); i < 4+EffectRecordSize; i++ {
		if v[i] != 0 {
			t.Fatalf("byte %d of the first record is %#x, want the zero padding", i, v[i])
		}
	}
	if icons, names, _ := channel43(v, 66); len(icons) > 1 && icons[1] == 0x00C0FFEE && names[1] == "Second" {
		t.Error("a 66-byte stride read the second record: the control proves nothing")
	}
	// none: the count alone
	if v := SerializeBuffUpdate(&Player{}); len(v) != 4 || binary.LittleEndian.Uint32(v) != 0 {
		t.Errorf("no effects serialized as %x, want four zero bytes", v)
	}
}

// A name is cut to the 31 characters the client shows (one of 32 would be
// skipped and shown empty), never between the halves of a surrogate pair;
// and no more than 8 records go out, the most the client keeps.
func TestEffectNamesAndCountAreWhatTheClientShows(t *testing.T) {
	long := strings.Repeat("abcdefghij", 4)
	_, names, _ := channel43(SerializeBuffUpdate(&Player{Buffs: []BuffSlot{{Icon: 1, Name: long}}}), EffectRecordSize)
	if names[0] != long[:MaxEffectName] {
		t.Errorf("a 40-character name reads as %q, want its first %d", names[0], MaxEffectName)
	}
	// control: uncut, the client would show nothing
	if _, names, _ := channel43(rawEffect(long), EffectRecordSize); names[0] != "" {
		t.Errorf("an uncut 40-character name read as %q, want it skipped", names[0])
	}
	split := strings.Repeat("a", 30) + "\U0001F600"
	_, names, _ = channel43(SerializeBuffUpdate(&Player{Buffs: []BuffSlot{{Icon: 1, Name: split}}}), EffectRecordSize)
	if names[0] != strings.Repeat("a", 30) {
		t.Errorf("a name ending in a pair at character 31 reads as %q", names[0])
	}
	nine := make([]BuffSlot, 9)
	for i := range nine {
		nine[i] = BuffSlot{Icon: uint32(i + 1), Name: "E"}
	}
	if v := SerializeBuffUpdate(&Player{Buffs: nine}); len(v) != 4+8*EffectRecordSize || binary.LittleEndian.Uint32(v) != 8 {
		t.Errorf("nine effects serialized as %d bytes, count %d; want 8 records", len(v), binary.LittleEndian.Uint32(v))
	}
}

// rawEffect is one record with the name written whole, however long.
func rawEffect(name string) []byte {
	u := utf16.Encode([]rune(name))
	v := make([]byte, 4+EffectRecordSize+2*len(u))
	binary.LittleEndian.PutUint32(v, 1)
	binary.LittleEndian.PutUint32(v[8:], uint32(len(u)))
	for j, c := range u {
		binary.LittleEndian.PutUint16(v[12+2*j:], c)
	}
	return v
}

// AddEffect keeps the order effects came in and stops at 8; ClearEffects
// takes them all off; PlayerEffects is a copy.
func TestAddEffectKeepsEightInOrder(t *testing.T) {
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 7), Port: 7001}, 7001, 7001, 1, 2, true)
	p := &Player{ServerID: 1001, Name: "P"}
	w.Players[p.ServerID] = p
	s.SetECSEntity(p)
	for i := 0; i < 8; i++ {
		if !w.AddEffect(s, BuffSlot{Icon: uint32(100 + i), Name: "E"}) {
			t.Fatalf("effect %d refused", i)
		}
	}
	if w.AddEffect(s, BuffSlot{Icon: 999}) {
		t.Error("a ninth effect was taken")
	}
	fx := w.PlayerEffects(s)
	if len(fx) != 8 || fx[0].Icon != 100 || fx[7].Icon != 107 {
		t.Fatalf("effects %v", fx)
	}
	fx[0].Icon = 0
	if p.Buffs[0].Icon != 100 {
		t.Error("PlayerEffects handed out the player's own slice")
	}
	w.ClearEffects(s)
	if len(w.PlayerEffects(s)) != 0 || len(SerializeBuffUpdate(p)) != 4 {
		t.Error("ClearEffects left effects on")
	}
}
