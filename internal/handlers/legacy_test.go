package handlers

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// legacyOf makes the Legacy record of record (database/legacy.go) and names it
// to the World.
func legacyOf(t *testing.T, db *database.SQLiteDB, id, record int) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.LegacyCopyTx(tx, id, record, "test", time.Unix(1_800_000_000, 0)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	entity.SetLegacyItems(map[int]int{id: record})
	t.Cleanup(func() { entity.SetLegacyItems(nil) })
}

// A Legacy Ascended item counts at a hand-in as its Ascended record (work
// board 1714): the step comes Ascended, the Legacy ring given away. The
// control: the Legacy record unknown to the World meets nothing.
func TestALegacyAscendedItemCountsAtAHandIn(t *testing.T) {
	for _, known := range []bool{true, false} {
		f := newAscendedQuestFixture(t)
		const legacy = 967001
		legacyOf(t, f.db, legacy, f.ascended)
		if !known {
			entity.SetLegacyItems(nil)
		}
		f.own(f.talisman, 1)
		f.own(legacy, 1)
		m := f.take(f.stark, "Will you make the breastplate?")
		if !known {
			if f.held(legacy) != 1 {
				t.Errorf("the control: the unknown Legacy ring was taken: %q", m)
			}
			continue
		}
		f.want(m, "box:Stark: There.", "box:"+questFinished, "box:You have given away Legacy Ascended Ring of the Test.",
			"box:You have received Ascended Breastplate of the Test.", "box:You have received draught of the test.")
		if f.held(legacy) != 0 {
			t.Errorf("the Legacy ring is still held")
		}
	}
}

// A Legacy scroll scribes its record's spell. The control: unknown to the
// World as a Legacy record, it is refused.
func TestALegacyScrollScribes(t *testing.T) {
	for _, known := range []bool{true, false} {
		f := newScribeFixture(t)
		f.spell(458, "Minor Remedy", 4)
		rec := f.item("Minor Remedy", true, healerMask, 4, 1)
		const legacy = 967002
		legacyOf(t, f.db, legacy, rec)
		if !known {
			entity.SetLegacyItems(nil)
		}
		f.own(legacy, 1)
		f.loadList()
		msgs := f.use(0)
		if known {
			mustOps(t, msgs, protocol.OpcodeRemoveInvItem, protocol.OpcodeAbilityAdd)
		} else if len(f.book()) != 0 {
			t.Error("the control: an unknown Legacy scroll was scribed")
		}
	}
}
