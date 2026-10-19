package handlers

import (
	"encoding/binary"
	"testing"

	"github.com/eqoa/server/internal/protocol"
)

// 0x0012 is read by the name page as VIMemStream::ReadString(name, 24): a u32
// length and the bytes (decomp viclient.cpp). It was a 7-bit length.
func TestRandomNameIsAReadString(t *testing.T) {
	c := newChatWorld(t)
	sess, _ := c.join(1, "Alice", 25000, 16000, 0)
	h := &CharacterHandler{}
	for i := 0; i < 20; i++ {
		before := len(sess.Queue().PeekPending())
		if err := h.HandleRandomName(sess, incoming(protocol.OpcodeRandomName, []byte{0, 2})); err != nil {
			t.Fatal(err)
		}
		msgs := sess.Queue().PeekPending()
		if len(msgs) != before+1 {
			t.Fatal("no answer")
		}
		d := msgs[len(msgs)-1].Data()
		op, n := binary.LittleEndian.Uint16(d), binary.LittleEndian.Uint32(d[2:])
		if op != uint16(protocol.OpcodeRandomName) || int(n) != len(d)-6 || n < 3 || n >= 24 {
			t.Fatalf("op %#x length %d of %d bytes: % x", op, n, len(d), d)
		}
		if !h.isValidName(string(d[6:])) {
			t.Errorf("%q is not a name the server would accept", d[6:])
		}
	}
}

func TestCheckNewChar(t *testing.T) {
	ok := []int32{5, 0, 3, 2, 4, 6, 0}
	if why := checkNewChar(0, 9, 1, 7, 3, 3, 3, 1, ok); why != "" {
		t.Fatalf("a form the pages can make refused: %s", why)
	}
	if why := checkNewChar(10, 0, 0, 0, 0, 0, 0, 0, make([]int32, 7)); why != "" {
		t.Fatalf("the froglok refused: %s", why)
	}
	bad := map[string]func() string{
		"race 11":       func() string { return checkNewChar(11, 0, 0, 0, 0, 0, 0, 0, ok) },
		"class 15":      func() string { return checkNewChar(0, 15, 0, 0, 0, 0, 0, 0, ok) },
		"sex 2":         func() string { return checkNewChar(0, 0, 2, 0, 0, 0, 0, 0, ok) },
		"hair colour 8": func() string { return checkNewChar(0, 0, 0, 8, 0, 0, 0, 0, ok) },
		"face 4":        func() string { return checkNewChar(0, 0, 0, 0, 0, 0, 4, 0, ok) },
		"city 3":        func() string { return checkNewChar(0, 0, 0, 0, 0, 0, 0, 3, ok) },
		"21 points":     func() string { return checkNewChar(0, 0, 0, 0, 0, 0, 0, 0, []int32{21, 0, 0, 0, 0, 0, 0}) },
		"sum 22":        func() string { return checkNewChar(0, 0, 0, 0, 0, 0, 0, 0, []int32{10, 10, 2, 0, 0, 0, 0}) },
		"negative":      func() string { return checkNewChar(0, 0, 0, 0, 0, 0, 0, 0, []int32{-5, 10, 10, 0, 0, 0, 0}) },
	}
	for name, f := range bad {
		if f() == "" {
			t.Errorf("%s accepted", name)
		}
	}
}
