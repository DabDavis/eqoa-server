package database

import (
	"fmt"
	"testing"
	"time"
)

func TestRecentArenaDuels(t *testing.T) {
	db, mk := recordDB(t)
	a, b := mk("ArenaAlpha"), mk("ArenaBeta")
	for i := 0; i < 12; i++ {
		a.Name = fmt.Sprintf("Winner%d", i)
		if err := db.RecordDuelEnd(DuelEnd{A: a, B: b, Loser: b.ServerID, At: time.Unix(200, 0), Arena: true}); err != nil {
			t.Fatal(err)
		}
	}
	for _, other := range []DuelEnd{{A: a, B: b, Loser: a.ServerID}, {A: a, B: b, Arena: true}} {
		if err := db.RecordDuelEnd(other); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.RecentArenaDuels()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 10 {
		t.Fatal(got)
	}
	for i, result := range got {
		if result != [2]string{fmt.Sprintf("Winner%d", 11-i), b.Name} {
			t.Fatal(got)
		}
	}
}
