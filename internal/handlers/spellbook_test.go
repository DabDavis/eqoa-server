package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Msg00B4 and Msg00B6 as the server keeps them: a toggle that bumps
// whatever held the slot, and a move that shifts the display indices.
func TestSpellbookMemorizeAndArrange(t *testing.T) {
	row := func(id, order, on, where int) database.SpellRow {
		return database.SpellRow{SpellID: id, AddedOrder: order, OnHotBar: on, WhereOnHotBar: where}
	}
	rows := []database.SpellRow{row(1, 0, 0, -1), row(2, 1, 1, 3), row(3, 2, 0, -1), row(4, 252, 0, -1)}
	if !memorize(rows, 0, 3) {
		t.Fatal("display index 0 not found")
	}
	if rows[0].OnHotBar != 1 || rows[0].WhereOnHotBar != 3 || rows[1].OnHotBar != 0 || rows[1].WhereOnHotBar != -1 {
		t.Fatalf("memorize: %+v", rows)
	}
	memorize(rows, 0, 3)
	if rows[0].OnHotBar != 0 {
		t.Fatal("a second memorize on the same slot toggles it off")
	}
	if memorize(rows, 9, 0) {
		t.Fatal("an unknown display index changes nothing")
	}
	arrange(rows, 0, 2)
	got := []int{rows[0].AddedOrder, rows[1].AddedOrder, rows[2].AddedOrder, rows[3].AddedOrder}
	want := []int{2, 0, 1, 252}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arrange: %v, want %v", got, want)
		}
	}
}
