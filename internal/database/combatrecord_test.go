package database

import (
	"testing"
	"time"
)

func recordDB(t *testing.T) (*SQLiteDB, func(name string) RecordFighter) {
	t.Helper()
	db, err := NewSQLite(t.TempDir() + "/record.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	chars := NewSQLiteCharacterStore(db)
	mk := func(name string) RecordFighter {
		t.Helper()
		rec := &CharacterRecord{CharName: name, AccountID: 1, Level: 40}
		if err := chars.Create(rec); err != nil {
			t.Fatal(err)
		}
		return RecordFighter{ServerID: uint32(rec.ServerID), Name: name, Level: 40}
	}
	return db, mk
}

func recordOf(t *testing.T, db *SQLiteDB, f RecordFighter) CombatRecord {
	t.Helper()
	r, err := db.CombatRecordOf(f.ServerID)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// PvP kills count up the killer's kills, streak and best streak, its last kill and its
// best kill by level (the first of a level stays); the victim's death ends its streak and
// names its killer; each helper gets one assist, once, and neither the killer nor the
// victim gets one. A character with no row has the zero record; a kill of oneself is
// refused and writes nothing.
func TestCombatRecordPvP(t *testing.T) {
	db, mk := recordDB(t)
	a, b, c := mk("Alpha"), mk("Beta"), mk("Gamma")
	t0 := time.Unix(1_800_000_000, 0)
	if r := recordOf(t, db, a); !r.Empty() || r.ServerID != a.ServerID {
		t.Fatalf("no row: %+v", r)
	}
	b.Level = 45
	kill := func(k, v RecordFighter, at time.Time, helpers ...RecordFighter) {
		t.Helper()
		if err := db.RecordPvPKill(PvPKill{Killer: k, Victim: v, Assists: helpers, At: at}); err != nil {
			t.Fatal(err)
		}
	}
	kill(a, b, t0, c, c, a, b)
	kill(a, c, t0.Add(time.Minute))
	ra := recordOf(t, db, a)
	if ra.PvPKills != 2 || ra.Streak != 2 || ra.BestStreak != 2 || ra.PvPDeaths != 0 || ra.Assists != 0 {
		t.Fatalf("the killer: %+v", ra)
	}
	if ra.BestKill != "Beta" || ra.BestKillLevel != 45 || ra.LastKill != "Gamma" || !ra.LastKillAt.Equal(t0.Add(time.Minute)) {
		t.Fatalf("best %s %d, last %s at %v", ra.BestKill, ra.BestKillLevel, ra.LastKill, ra.LastKillAt)
	}
	rc := recordOf(t, db, c)
	if rc.Assists != 1 || rc.PvPDeaths != 1 || rc.LastDeath != "Alpha" || rc.LastDeathLevel != 40 {
		t.Fatalf("the helper, killed after: %+v", rc)
	}
	// a death ends the streak; the best stays
	kill(b, a, t0.Add(2*time.Minute))
	kill(a, b, t0.Add(3*time.Minute))
	ra = recordOf(t, db, a)
	if ra.Streak != 1 || ra.BestStreak != 2 || ra.PvPDeaths != 1 || ra.LastDeath != "Beta" || ra.LastDeathLevel != 45 {
		t.Fatalf("after a death: %+v", ra)
	}
	// a higher kill takes the best; one of the same level does not
	c.Level = 50
	kill(a, c, t0.Add(4*time.Minute))
	b.Level = 50
	kill(a, b, t0.Add(5*time.Minute))
	if ra = recordOf(t, db, a); ra.BestKill != "Gamma" || ra.BestKillLevel != 50 || ra.BestStreak != 3 {
		t.Fatalf("best kill %s %d, best streak %d", ra.BestKill, ra.BestKillLevel, ra.BestStreak)
	}
	before := recordOf(t, db, a)
	if err := db.RecordPvPKill(PvPKill{Killer: a, Victim: a, At: t0}); err == nil {
		t.Fatal("a kill of oneself was written")
	}
	if err := db.RecordPvPKill(PvPKill{Killer: a, Victim: RecordFighter{Name: "Nobody"}, At: t0}); err == nil {
		t.Fatal("a kill of no character was written")
	}
	if after := recordOf(t, db, a); after != before {
		t.Fatalf("a refused kill changed the record: %+v", after)
	}
}

// A duel's end: the winner's win and the wager it took, the loser's loss, its surrender
// among them and its stake; a draw to both; a loser who is neither is refused.
func TestCombatRecordDuels(t *testing.T) {
	db, mk := recordDB(t)
	a, b := mk("Alpha"), mk("Beta")
	t0 := time.Unix(1_800_000_000, 0)
	end := func(e DuelEnd) {
		t.Helper()
		e.At = t0
		if err := db.RecordDuelEnd(e); err != nil {
			t.Fatal(err)
		}
	}
	end(DuelEnd{A: a, B: b, Loser: b.ServerID, Wager: 100})
	end(DuelEnd{A: b, B: a, Loser: b.ServerID, Surrendered: true, Wager: 25})
	end(DuelEnd{A: a, B: b, Loser: a.ServerID})
	end(DuelEnd{A: a, B: b})
	ra, rb := recordOf(t, db, a), recordOf(t, db, b)
	if ra.DuelWins != 2 || ra.DuelLosses != 1 || ra.DuelSurrenders != 0 || ra.DuelDraws != 1 || ra.WagerWon != 125 || ra.WagerLost != 0 {
		t.Fatalf("Alpha: %+v", ra)
	}
	if rb.DuelWins != 1 || rb.DuelLosses != 2 || rb.DuelSurrenders != 1 || rb.DuelDraws != 1 || rb.WagerWon != 0 || rb.WagerLost != 125 {
		t.Fatalf("Beta: %+v", rb)
	}
	if !rb.HasDuels() || rb.HasPvP() || rb.Empty() {
		t.Fatal("Beta's record says it has no duels, or some PvP")
	}
	if err := db.RecordDuelEnd(DuelEnd{A: a, B: b, Loser: 999, At: t0}); err == nil {
		t.Fatal("a loser of neither side was written")
	}
	if err := db.RecordDuelEnd(DuelEnd{A: a, B: a, At: t0}); err == nil {
		t.Fatal("a duel with itself was written")
	}
	if got := recordOf(t, db, a); got.DuelWins != 2 || got.DuelDraws != 1 {
		t.Fatalf("a refused end changed the record: %+v", got)
	}
}

// By name in any case, spelt as the character is; a name nobody has is not found; a
// deleted character's record goes with it, so no other comes back holding it.
func TestCombatRecordByNameAndDelete(t *testing.T) {
	db, mk := recordDB(t)
	a, b := mk("Alpha"), mk("Beta")
	if err := db.RecordDuelEnd(DuelEnd{A: a, B: b, Loser: b.ServerID, At: time.Unix(1_800_000_000, 0)}); err != nil {
		t.Fatal(err)
	}
	r, spelt, found, err := db.CombatRecordByName("  aLPHA ")
	if err != nil || !found || spelt != "Alpha" || r.DuelWins != 1 {
		t.Fatalf("by name: %+v %q %v %v", r, spelt, found, err)
	}
	if _, _, found, err := db.CombatRecordByName("Nobody"); found || err != nil {
		t.Fatalf("a name nobody has: found %v, %v", found, err)
	}
	if err := NewSQLiteCharacterStore(db).Delete(b.ServerID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM charrecord WHERE serverid = ?`, b.ServerID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("the deleted character's record: %d rows, %v", n, err)
	}
	if got := recordOf(t, db, a); got.DuelWins != 1 {
		t.Fatalf("the other's record went too: %+v", got)
	}
}
