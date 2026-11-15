package handlers

import (
	"strings"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// The faction stone (itemfactions.go) through the item menu's Use (0x003E):
// each of its factions up to its row's standing, never down, the lines a
// kill's change says, the stone kept; a second Use says it is quiet. The
// factions and the stone are made up.

// textsOf is every line of op among msgs, in order.
func textsOf(msgs [][]byte, op protocol.GameOpcode) []string {
	var out []string
	for _, m := range msgs {
		if len(m) < 2 || opOf(m) != op {
			continue
		}
		rd := buffer.NewReader(m[2:])
		n, _ := rd.ReadUint32()
		var sb strings.Builder
		for i := uint32(0); i < n; i++ {
			u, _ := rd.ReadUint16()
			sb.WriteRune(rune(u))
		}
		out = append(out, sb.String())
	}
	return out
}

func TestTheFactionStoneRaisesItsFactions(t *testing.T) {
	r := newRockRig(t)
	r.gw.SetFactionDefaults([]database.FactionDefault{
		{Name: "Citizens of Qeynos", Standing: 1}, {Name: "Citizens of Neriak", Standing: -2000},
		{Name: "Citizens of Halas", Standing: 1}, {Name: "Citizens of Oggok", Standing: -2000},
	})
	r.gw.RestoreFactions(r.sess, map[string]int{"Citizens of Halas": 900})
	stone, _, err := database.EnsureItemRecord(r.db.DB, database.ItemPatternRecord{ItemName: "Stone of Seventeen Banners",
		EquipSlot: -1, Trade: 1, Rent: 1, LevelReq: 1, MaxStack: 1, ClassUse: 32767, RaceUse: 32767, ProcAnim: 7, Lore: 1,
		Color: 0xFFFFFFFF}, 799000)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"Citizens of Qeynos", "Citizens of Neriak", "Citizens of Halas", "No Such Faction"} {
		if _, err := r.db.Exec(`INSERT INTO itemfactions (patternid, faction, standing) VALUES (?, ?, 700)`, stone, f); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.inv.AddCharItem(r.sid, stone, 1, 0, 0, 0); err != nil {
		t.Fatal(err)
	}

	got := textsOf(r.use(r.key(stone), 0), protocol.OpcodeClientMessage)
	// Halas stands past the floor: untouched, unsaid; the unknown faction passed by
	wantLines := []string{
		"700 faction with Citizens of Neriak (+2700)",
		"700 faction with Citizens of Qeynos (+699)",
		"The Stone of Seventeen Banners glows. Your standing has risen with 2 of its factions.",
	}
	if strings.Join(got, "\n") != strings.Join(wantLines, "\n") {
		t.Errorf("the lines:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(wantLines, "\n"))
	}
	for f, w := range map[string]int{"Citizens of Qeynos": 700, "Citizens of Neriak": 700, "Citizens of Halas": 900,
		"Citizens of Oggok": -2000} {
		if v, _ := r.gw.Standing(r.sess, f); v != w {
			t.Errorf("%s: %d, want %d", f, v, w)
		}
	}
	// the stone is kept, and a second Use moves nothing
	got = textsOf(r.use(r.key(stone), 1), protocol.OpcodeClientMessage)
	quiet := "The Stone of Seventeen Banners is quiet: your standing with every one of its factions is already as high as it can raise it."
	if len(got) != 1 || got[0] != quiet {
		t.Errorf("the second Use's lines %q, want %q", got, quiet)
	}
}

// An item with no rows is not the stone's: its Use goes on as before.
func TestAnItemWithoutFactionRowsIsNotAnswered(t *testing.T) {
	r := newRockRig(t)
	if r.h.useItemFactions(r.sess, r.p, &database.ItemPatternRecord{PatternID: 424242, ItemName: "Plain Pebble"}) {
		t.Error("an item with no itemfactions rows was answered")
	}
}
