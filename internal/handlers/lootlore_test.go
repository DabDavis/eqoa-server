package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/protocol"
)

// A take-all over a Lore item the character holds already is not answered
// 0x001A: CLIENT's Msg001A knows no Lore and would keep the item in its own
// list while the server left it on the corpse, to be "taken" again and again
// (petition 1498, Waldorf). The rest comes item by item (0x0016), the Lore one
// stays and is said, and the server's rows hold one. Held in the pack, held in
// the bank (counted since 2026-11-10), and two of it on one corpse. The
// control: a corpse with no Lore item held is answered 0x001A as before.
func TestATakeAllOverAHeldLoreItemLeavesItOnTheCorpse(t *testing.T) {
	for _, tc := range []struct {
		name     string
		held     int // where one is held already: 0 pack, 1 bank, -1 nowhere
		onCorpse int // Lore items on the corpse (beside one sword)
		want     int // Lore rows after the take-all
		byOne    bool
	}{
		{"held in the pack", 0, 1, 1, true},
		{"held in the bank", 1, 1, 1, true},
		{"two on the corpse, none held", -1, 2, 1, true},
		{"control: one on the corpse, none held", -1, 1, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCorpseFixture(t, 1)
			charm := f.pattern("Unique Charm", 100, 1, 100, -1)
			if _, err := f.db.Exec(`UPDATE itempattern SET lore = 1 WHERE patternid = ?`, charm); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < tc.onCorpse; i++ {
				if _, err := f.db.Exec(`INSERT INTO npcitem (npcid, stackleft, remainHP, patternid, listnumber)
					VALUES (?, 1, 100, ?, ?)`, corpseNPCID, charm, 1+i); err != nil {
					t.Fatal(err)
				}
			}
			if tc.held >= 0 {
				f.own(charm, 1, 100, tc.held)
			}
			f.loadLists()
			f.h.OpenCorpse(f.sess, f.npc)
			f.sent()

			f.send(protocol.OpcodeLootTakeAll, nil)
			got := f.sent()
			all, adds := false, 0
			for _, m := range got {
				switch opOf(m) {
				case protocol.OpcodeLootTakeAll:
					all = true
				case protocol.OpcodeAddInvItem:
					adds++
				}
			}
			if all == tc.byOne {
				t.Fatalf("answered %s; 0x001A back: %v, want %v", ops(got), all, !tc.byOne)
			}
			charms, swords := 0, 0
			for _, r := range f.rows() {
				switch r.Pattern.PatternID {
				case charm:
					charms++
				case f.sword:
					swords++
				}
			}
			if charms != tc.want || swords != 1 {
				t.Fatalf("%d Lore rows and %d swords, want %d and 1", charms, swords, tc.want)
			}
			if !tc.byOne {
				return
			}
			// item by item: the sword (and a first Lore one) added, the window sent
			// again with the Lore one left on it
			wantAdds := 1
			if tc.held < 0 {
				wantAdds = 2
			}
			if adds != wantAdds {
				t.Fatalf("%d items added (%s), want %d", adds, ops(got), wantAdds)
			}
			last := got[len(got)-1]
			if opOf(last) != protocol.OpcodeLoot || lootCount(t, last) != 1 {
				t.Fatalf("after the take-all: %s, want the window sent again with the one Lore item", ops(got))
			}
			// again: nothing more comes, however often
			f.send(protocol.OpcodeLootTakeAll, nil)
			f.sent()
			n := 0
			for _, r := range f.rows() {
				if r.Pattern.PatternID == charm {
					n++
				}
			}
			if n != tc.want {
				t.Fatalf("a second take-all: %d Lore rows, want %d", n, tc.want)
			}
		})
	}
}
