package handlers

import (
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// takeoverRig is a character handler over a database of one account's
// characters, and windows of that account to select them from.
type takeoverRig struct {
	t     *testing.T
	db    *database.SQLiteDB
	store *database.SQLiteCharacterStore
	w     *entity.World
	sm    *session.Manager
	h     *CharacterHandler
	port  int
}

func newTakeoverRig(t *testing.T, chars ...*database.CharacterRecord) *takeoverRig {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/takeover.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := &takeoverRig{t: t, db: db, store: database.NewSQLiteCharacterStore(db), w: entity.NewWorld(), sm: session.NewManager(), port: 6400}
	for _, c := range chars {
		if err := r.store.Create(c); err != nil {
			t.Fatal(err)
		}
	}
	r.sm.SetOnReplaced(SendLoggedInElsewhere)
	r.w.SessionManager = r.sm
	r.h = &CharacterHandler{store: r.store, sessionManager: r.sm, gameWorld: r.w}
	return r
}

// window is a new session of the account, as a client at character select.
func (r *takeoverRig) window(account int) *session.Session {
	r.port++
	inst := uint32(r.port)
	s := r.sm.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 9), Port: r.port}, inst, inst, uint16(inst), 2, true)
	s.SetAccountID(account)
	s.UpdateActivity()
	return s
}

// enter selects the character and, as the client does once its world is
// loaded, says it is in the world (0x0014).
func (r *takeoverRig) enter(s *session.Session, id uint32) {
	r.t.Helper()
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeSelectedChar)
	msg.SetData(u32b(id))
	if err := r.h.HandleSelectedChar(s, msg); err != nil {
		r.t.Fatal(err)
	}
	if s.InGame() {
		if err := r.h.HandleCharacterInWorld(s, protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCharacterInWorld)); err != nil {
			r.t.Fatal(err)
		}
	}
}

// sentDump is the last MemoryDump the session was sent.
func sentDump(t *testing.T, s *session.Session) []byte {
	t.Helper()
	d := sentOp(s, protocol.OpcodeMemoryDump)
	if d == nil {
		t.Fatal("no MemoryDump sent")
	}
	return d
}

// jib's F3 20261029-212855: a character whose client dropped lingers in the
// world, and choosing it again from a new window of the account takes it
// over at once, as it stands: the same Player with its hit points and power,
// its effect and the time left of it, the creature fighting it and its
// threat, where it stood, a MemoryDump putting the new client there; the
// window it was in (still there here) told it logged in elsewhere. The
// control is the old wait: before, the select was refused (0x07F0) while the
// dropped window had been heard from within session.LiveWithin, and a login
// after that built the character anew from its row, full of hit points, as
// one does here once the holder's session has ended and taken it out. The
// takeover writes the row too (saveTakenOver), reading nothing back.
func TestATakeoverKeepsTheCharacterAsItStands(t *testing.T) {
	jib := &database.CharacterRecord{CharName: "Jibrish", AccountID: 7, Level: 60, Race: 3, TClass: 12,
		X: 15180.75, Y: 54.13, Z: 22745.81}
	r := newTakeoverRig(t, jib)
	first := r.window(7)
	r.enter(first, jib.ServerID)
	p := r.w.SessionPlayer(first)
	if p == nil || !first.CharacterInWorld() {
		t.Fatal("setup: not in the world")
	}
	// mid-fight, hurt, buffed, moved on from where it logged in
	now := time.Now()
	p.HP, p.Power = 41, 17
	charm := &entity.ActiveEffect{SpellID: 7001, Name: "Enthrall", Icon: 0x1111, Until: now.Add(75 * time.Second)}
	p.Effects = append(p.Effects, charm)
	p.X, p.Y, p.Z = 15210.5, 54.13, 22760.25
	orc := &entity.NPC{ServerID: 90001, Name: "an orc pawn", HP: 90, MaxHP: 90, Level: 12,
		Threats: map[entity.EntityID]int64{p.ServerID: 320}, CurrentEnemy: p.ServerID, X: 15211, Z: 22761}
	r.w.NPCs[orc.ServerID] = orc

	// the client drops: nothing more from it (its session stays, live);
	// a new window of the account chooses the character at once
	second := r.window(7)
	r.enter(second, jib.ServerID)
	if sentOp(second, protocol.OpcodeCharSelectAreaUnavailable) != nil {
		t.Fatal("the new window was told to wait (0x07F0)")
	}
	if got := r.w.SessionPlayer(second); got != p || !second.CharacterInWorld() {
		t.Fatalf("the new window has %p, want the live character %p", got, p)
	}
	if p.HP != 41 || p.Power != 17 || len(p.Effects) != 1 || p.Effects[0] != charm || !charm.Until.Equal(now.Add(75*time.Second)) {
		t.Fatalf("not as it stood: HP %d power %d effects %+v", p.HP, p.Power, p.Effects)
	}
	if orc.Threats[p.ServerID] != 320 || orc.CurrentEnemy != p.ServerID {
		t.Fatal("the creature fighting it let it go")
	}
	path, world, at, _ := dumpPlace(t, sentDump(t, second))
	if path != worldPathFor(0) || world != 0 || at != [3]float32{15210.5, 54.13, 22760.25} {
		t.Fatalf("the dump puts it at %q world %d %v, want where it stands", path, world, at)
	}
	if _, r := first.ReplacedFor(); !r || sentOp(first, protocol.OpcodeLoggedInFromAnotherLocationDC) == nil {
		t.Fatal("the dropped window was not told it logged in elsewhere")
	}
	// and its row written as it stands, should the new client never get in
	if row, err := r.store.GetByServerID(jib.ServerID); err != nil || row.X != 15210.5 || row.Z != 22760.25 {
		t.Fatalf("the row after the takeover: %+v (%v)", row, err)
	}
	if kept, err := r.store.LoadBuffs(jib.ServerID); err != nil || len(kept) != 1 || kept[0].SpellID != 7001 {
		t.Fatalf("the effects kept after the takeover: %+v (%v)", kept, err)
	}
	// its session's end, after the grace, leaves the character where it is
	r.w.RemovePlayerBySession(first)
	r.sm.RemoveSession(first)
	if r.w.SessionPlayer(second) != p || r.w.PlayerCount() != 1 {
		t.Fatal("the dropped window's end took the character out of the world")
	}

	// the control: once the character's session has ended (its end saving
	// and removing it), a login builds it from its row, hit points full
	r.w.RemovePlayerBySession(second)
	r.sm.RemoveSession(second)
	third := r.window(7)
	r.enter(third, jib.ServerID)
	fresh := r.w.SessionPlayer(third)
	if fresh == nil || fresh == p || fresh.HP != fresh.MaxHPNow() || fresh.HP == 41 {
		t.Fatalf("the control: a login with no character in the world kept the old one's hit points (%p, HP %d/%d)",
			fresh, fresh.HP, fresh.MaxHPNow())
	}
}

// A character taken over in a world the new client may not have (world 8,
// a hard disk's) starts where a login would, beside Vaal in Tunaria, and its
// dump says so; one in a disc's world stays (the test above).
func TestATakeoverOutOfAWorldTheClientLacks(t *testing.T) {
	gnome := &database.CharacterRecord{CharName: "Jibcro", AccountID: 7, Level: 60, Race: 3, TClass: 11,
		X: 25375, Y: 54.13, Z: 15721}
	r := newTakeoverRig(t, gnome)
	first := r.window(7)
	r.enter(first, gnome.ServerID)
	p := r.w.SessionPlayer(first)
	p.World, p.X, p.Y, p.Z = 8, 1000, 20, 1000
	second := r.window(7)
	r.enter(second, gnome.ServerID)
	if r.w.SessionPlayer(second) != p {
		t.Fatal("not taken over")
	}
	path, world, at, _ := dumpPlace(t, sentDump(t, second))
	if p.World != 0 || path != worldPathFor(0) || world != 0 || at[0] != p.X || at[2] != p.Z || p.X == 1000 {
		t.Fatalf("world %d at (%.0f, %.0f), dump %q world %d %v: not moved to where a login starts", p.World, p.X, p.Z, path, world, at)
	}
}
