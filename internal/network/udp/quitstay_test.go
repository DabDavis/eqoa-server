package udp

import (
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// A quit in a fight, end to end (entity/linkdead.go; the user, 2026-10-31:
// "i just logged out with jedd and i fully healed and mobs did not kill
// me"): the session stays while the creature fights the character, ends when
// the fight does, and its end keeps the hit points and power the character
// had, which the next login takes. The control: dead, it comes back whole.
func TestAQuitInAFightIsFoughtOnAndKeepsItsWounds(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/quit.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	rec := &database.CharacterRecord{CharName: "Jedd", AccountID: 1, Level: 20, X: 25300, Y: 54, Z: 15700}
	if err := store.Create(rec); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(rec); err != nil {
		t.Fatal(err)
	}
	w := entity.NewWorld()
	sm := session.NewManager()
	w.SessionManager = sm
	l := NewListener(nil, sm, nil, w)
	l.SetCharStore(store)
	login := func() (*session.Session, *entity.Player) {
		sess := sm.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 9), Port: 7000}, 1, 1, 1, 2, true)
		loaded, err := store.GetByServerID(rec.ServerID)
		if err != nil {
			t.Fatal(err)
		}
		p := w.CreatePlayer(sess, loaded)
		sess.SetCharacterInWorld(true)
		return sess, p
	}
	sess, p := login()
	p.HP, p.Power = 40, 7
	bear := &entity.NPC{ServerID: 5000, Name: "grizzly bear", Level: 22, HP: 900, MaxHP: 900, ModelID: 0x1234, Size: 1,
		X: p.X + 3, Z: p.Z, World: p.World, NPCType: 0x0100}
	w.NPCs[bear.ServerID] = bear
	w.SpatialIndex.Insert(bear.ServerID, bear.X, bear.Z)
	bear.Threats = map[entity.EntityID]int64{p.ServerID: 10}
	bear.CurrentEnemy = p.ServerID

	t0 := time.Now()
	if w.QuitInFight(sess) != 1 {
		t.Fatal("the bear does not count as fighting it")
	}
	sess.SetQuitting(t0)
	l.endQuitStay(sess, t0.Add(time.Second))
	if sess.PendingTermination() {
		t.Fatal("the session ended with the bear on its character")
	}
	// the bear lets go
	bear.Threats, bear.CurrentEnemy = nil, 0
	l.endQuitStay(sess, t0.Add(2*time.Second))
	if !sess.PendingTermination() {
		t.Fatal("the session stayed after the fight")
	}
	l.endSession(sess)
	if w.SessionPlayer(sess) != nil && w.IsLivePlayer(p) {
		t.Fatal("the character is still in the world")
	}
	// the next login: 40 HP, 7 power, not whole
	sess2, p2 := login()
	hp, power, saved, ok, err := store.TakeVitals(rec.ServerID)
	if err != nil || !ok || hp != 40 || power != 7 || time.Since(saved) > time.Minute {
		t.Fatalf("kept %d HP %d power at %v (%v, %v), want 40 7 now", hp, power, saved, ok, err)
	}
	w.RestoreVitals(sess2, hp, power, 0)
	if p2.HP != 40 || p2.Power != 7 {
		t.Fatalf("logged in with %d HP %d power, want 40 7", p2.HP, p2.Power)
	}

	// the control: it leaves dead, and comes back whole
	p2.HP = 0
	sess2.SetPendingTermination(true)
	l.endSession(sess2)
	if _, _, _, ok, _ := store.TakeVitals(rec.ServerID); ok {
		t.Error("a dead character's hit points kept")
	}
}
