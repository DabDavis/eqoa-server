package handlers

import (
	"bytes"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
)

// A header is written the way VICSMessageHeader's reader takes it: the id
// raw, seven date bytes, the category and subcategory raw, the unread byte.
func TestCSHeaderLayout(t *testing.T) {
	tk := &csTicket{id: 0x01020304, date: time.Date(2026, 10, 19, 21, 5, 3, 0, time.UTC), category: 2, subcategory: 21, unread: true}
	buf := make([]byte, 64)
	w := buffer.NewWriter(buf)
	writeCSHeader(w, tk)
	want := []byte{4, 3, 2, 1, 0xEA, 0x07, 10, 19, 21, 5, 3, 2, 0, 0, 0, 21, 0, 0, 0, 1}
	if got := buf[:w.Position()]; !bytes.Equal(got, want) {
		t.Fatalf("header\n got %v\nwant %v", got, want)
	}
}

// The lists: a zigzag count, then per category a zigzag id, a hidden byte,
// the wide name and its subcategories the same way. The control: the hidden
// category is flagged, the first is not.
func TestCSCategoriesLayout(t *testing.T) {
	buf := make([]byte, 4096)
	w := buffer.NewWriter(buf)
	writeCSCategories(w)
	r := buffer.NewReader(buf[:w.Position()])
	n, _ := r.Read7BitEncodedInt64()
	if int(n) != len(csCategories) {
		t.Fatalf("count %d", n)
	}
	for k := 0; k < int(n); k++ {
		id, _ := r.Read7BitEncodedInt64()
		hidden, _ := r.ReadByte()
		name, _ := readWide(r)
		c := csCategories[k]
		if id != c.id || (hidden != 0) != c.hidden || name != c.name {
			t.Fatalf("category %d: %d %d %q", k, id, hidden, name)
		}
		m, _ := r.Read7BitEncodedInt64()
		for j := 0; j < int(m); j++ {
			sid, _ := r.Read7BitEncodedInt64()
			sh, _ := r.ReadByte()
			sname, _ := readWide(r)
			s := c.subs[j]
			if sid != s.id || (sh != 0) != s.hidden || sname != s.name {
				t.Fatalf("category %d sub %d: %d %d %q", k, j, sid, sh, sname)
			}
		}
	}
	if r.Remaining() != 0 {
		t.Fatalf("%d bytes left", r.Remaining())
	}
	if csCategories[0].hidden || !csCategories[len(csCategories)-1].hidden {
		t.Fatal("the control: one hidden category, last")
	}
}
