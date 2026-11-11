package udp

import (
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// progressRig is a character at level 1 in a world over a test database, with
// the listener's store set or not.
func progressRig(t *testing.T, hook bool) (*database.SQLiteCharacterStore, *entity.World, *session.Session, *entity.Player, uint32) {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/progress.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	rec := &database.CharacterRecord{CharName: "Dinger", AccountID: 1, Level: 1, X: 25300, Y: 54, Z: 15700}
	if err := store.Create(rec); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(rec); err != nil {
		t.Fatal(err)
	}
	w := entity.NewWorld()
	sm := session.NewManager()
	w.SessionManager = sm
	sess := sm.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 9), Port: 7000}, 1, 1, 1, 2, true)
	loaded, err := store.GetByServerID(rec.ServerID)
	if err != nil {
		t.Fatal(err)
	}
	p := w.CreatePlayer(sess, loaded)
	l := NewListener(nil, sm, nil, w)
	if hook {
		l.SetCharStore(store)
	}
	return store, w, sess, p, rec.ServerID
}

func rowLevel(t *testing.T, store *database.SQLiteCharacterStore, id uint32) (int, int) {
	t.Helper()
	got, err := store.GetByServerID(id)
	if err != nil {
		t.Fatal(err)
	}
	return got.Level, got.TotalXP
}

// A level gained is in the row at the next tick, before any logout: a crash or
// a kill of the process after it no longer loses it (a Live Server character
// showed level 2 in the game with 1 in its row, 2026-11-11). The control: a
// world with no store hooked (as every world was before) leaves the row at 1.
func TestADingIsWrittenAtTheNextTick(t *testing.T) {
	for _, hook := range []bool{false, true} {
		store, w, sess, p, id := progressRig(t, hook)
		w.AwardXP(sess, p, entity.CharXPDict[1]+10)
		if p.Level != 2 {
			t.Fatalf("level %d after the award; want 2", p.Level)
		}
		w.Tick(time.Now())
		level, xp := rowLevel(t, store, id)
		if !hook {
			if level != 1 {
				t.Fatalf("control: the row says level %d with no hook; want 1 (nothing writes it)", level)
			}
			continue
		}
		if level != 2 || xp != entity.TotalXP(2, p.XPInLevel) {
			t.Fatalf("the row says level %d, %d xp after the ding's tick; want 2, %d", level, xp, entity.TotalXP(2, p.XPInLevel))
		}
	}
}

// The experience between dings is written every ProgressSaveEvery (only when
// it changed), so a crash loses at most that much of it.
func TestExperienceIsWrittenEveryFewMinutes(t *testing.T) {
	store, w, sess, p, id := progressRig(t, true)
	t0 := time.Now()
	w.Tick(t0) // the player's first look: its figures are its row's
	w.AwardXP(sess, p, 50)
	if p.Level != 1 {
		t.Fatalf("level %d; want 1 (no ding)", p.Level)
	}
	w.Tick(t0.Add(entity.ProgressSaveEvery - 10*time.Second))
	if _, xp := rowLevel(t, store, id); xp == entity.TotalXP(1, p.XPInLevel) {
		t.Fatalf("written before ProgressSaveEvery")
	}
	w.Tick(t0.Add(entity.ProgressSaveEvery + 2*time.Second))
	if level, xp := rowLevel(t, store, id); level != 1 || xp != entity.TotalXP(1, p.XPInLevel) {
		t.Fatalf("the row says level %d, %d xp after %v; want 1, %d", level, xp, entity.ProgressSaveEvery, entity.TotalXP(1, p.XPInLevel))
	}
}

// The logout's save holds the writes off (ProgressHold), and a ding queued for
// a character that has since left writes nothing: the logout wrote it.
func TestAQueuedDingAfterTheLogoutWritesNothing(t *testing.T) {
	store, w, sess, p, id := progressRig(t, true)
	w.AwardXP(sess, p, entity.CharXPDict[1]+10)
	// the logout's save writes level 2, then a GM sets the row to 7 (as a
	// later change would) before the tick: the stale queue must not undo it
	w.RemovePlayerBySession(sess)
	if err := store.SetProgress(id, 7, entity.TotalXP(7, 0)); err != nil {
		t.Fatal(err)
	}
	w.Tick(time.Now())
	if level, _ := rowLevel(t, store, id); level != 7 {
		t.Fatalf("the row says level %d; want 7 (a departed character's queued ding writes nothing)", level)
	}
}
