package udp

import (
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// A client past character select that sends nothing for InGameSilentEnd is
// gone (a closed tab, a crashed or hung page, a dropped line) and its session
// ends, its character leaving the world, where it stood about 500 s: 50
// pings unanswered at world.session_timeout_game's 10 s. The control: one
// silent a little under it (a PS2 reading a world off the disc is silent about
// 22 s, CLIENT's keepalive comes every 30 s) plays on.
func TestASilentClientInTheWorldEndsAfterAMinute(t *testing.T) {
	r, sess, _ := slotsTick(t, 24)
	sess.SetInGame(true)
	sess.SetLastActivity(time.Now().Add(-InGameSilentEnd + 5*time.Second))
	r.TickSession(sess)
	if sess.PendingTermination() {
		t.Fatalf("a client silent %v ended", InGameSilentEnd-5*time.Second)
	}
	sess.SetLastActivity(time.Now().Add(-InGameSilentEnd - time.Second))
	r.TickSession(sess)
	if !sess.PendingTermination() {
		t.Fatalf("a client silent past %v lives on (its 50 pings: %d sent)", InGameSilentEnd, sess.PingCount())
	}
	if InGameSilentEnd < 45*time.Second || InGameSilentEnd > session.LiveWithin {
		t.Fatalf("InGameSilentEnd %v: under 45 s a PS2's disc read or a keepalive's wait could cut a live client; past LiveWithin (%v) a gone one would still hold its login", InGameSilentEnd, session.LiveWithin)
	}
}

// Not every silent session ends at once: one another login took the
// character from goes at its own grace's end (session.ReplacedGrace); one
// whose character is dead with its respawn to come waits for it, so it goes
// home to its bind point, as a Quit in a fight does; and one past character
// select that never sent its 0x0014 (gone during the world's load) ends from
// the tick as one in the world does. The controls: alive, the silent session
// ends; heard from, the one before its 0x0014 plays on.
func TestSilentEndsWaitForARespawnAndCoverAWorldLoad(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/silent.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	rec := &database.CharacterRecord{CharName: "Quietus", AccountID: 1, Level: 20, X: 25300, Y: 54, Z: 15700}
	if err := store.Create(rec); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(rec); err != nil {
		t.Fatal(err)
	}
	srv, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	w := entity.NewWorld()
	sm := session.NewManager()
	w.SessionManager = sm
	l := NewListener(nil, sm, nil, w)
	l.conn = srv
	l.SetCharStore(store)
	addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9}
	silent := time.Now().Add(-InGameSilentEnd - time.Second)

	sess := sm.CreateSession(addr, 1, 1, 1, 2, true)
	loaded, err := store.GetByServerID(rec.ServerID)
	if err != nil {
		t.Fatal(err)
	}
	p := w.CreatePlayer(sess, loaded)
	sess.SetInGame(true)
	sess.SetCharacterInWorld(true)
	p.DiedAt = time.Now() // dead, its respawn to come
	sess.SetLastActivity(silent)
	if l.silentInWorldOver(sess, time.Since(sess.LastActivity())) || sess.PendingTermination() {
		t.Fatal("a silent session ended with its character dead and its respawn to come")
	}
	p.DiedAt = time.Time{} // respawned
	if !l.silentInWorldOver(sess, time.Since(sess.LastActivity())) {
		t.Fatal("the control: alive, the silent session lives on")
	}

	replaced := sm.CreateSession(addr, 2, 2, 1, 2, true)
	replaced.SetInGame(true)
	replaced.SetCharacterInWorld(true)
	replaced.MarkReplaced()
	replaced.SetLastActivity(silent)
	if l.silentInWorldOver(replaced, time.Since(replaced.LastActivity())) {
		t.Fatal("a replaced session ended by its silence, not its grace")
	}

	// past character select, before its 0x0014: the tick ends it
	loading := sm.CreateSession(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 10}, 3, 3, 1, 2, true)
	loading.SetInGame(true)
	loading.UpdateActivity()
	l.tickAllSessions()
	if sm.GetSession(loading.Key()) == nil {
		t.Fatal("the control: a session heard from before its 0x0014 ended")
	}
	loading.SetLastActivity(silent)
	l.tickAllSessions()
	if sm.GetSession(loading.Key()) != nil {
		t.Fatalf("a session past character select silent %v before its 0x0014 lives on", InGameSilentEnd)
	}
}
