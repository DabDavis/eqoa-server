package handlers

import (
	"fmt"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// lockoutBook gives the character a book of two abilities, spell 901 at
// display index 3 and 902 at 4, and the handler the book's store.
func lockoutBook(t *testing.T, r *takeoverRig, id uint32) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast,
			eqprequire, spellname, spelldesc) VALUES (901, 50, 0, 0, 0, 0, 0, 0, 0, 0, 600, 0, 'Long Ward', '')`,
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast,
			eqprequire, spellname, spelldesc) VALUES (902, 50, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 'Quick Ward', '')`,
		fmt.Sprintf(`INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide) VALUES (%d, 901, 3, 1, 0, 0, 0)`, id),
		fmt.Sprintf(`INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide) VALUES (%d, 902, 4, 1, 1, 0, 0)`, id),
	} {
		if _, err := r.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	r.h.spellbookStore = database.NewSpellbookStore(r.db)
}

// toldLockouts is each 0x00CD queued on s after its MemoryDump, display
// index to the lockout's ms, and how many went in all; a 0x00CD before the
// dump fails the test.
func toldLockouts(t *testing.T, s *session.Session) (map[byte]int64, int) {
	t.Helper()
	dump, told, n := false, map[byte]int64{}, 0
	for _, m := range s.Queue().PeekPending() {
		d := m.Data()
		if len(d) < 2 {
			continue
		}
		switch opcode(d) {
		case protocol.OpcodeMemoryDump:
			dump = true
		case protocol.OpcodeCooldownTimer:
			if !dump {
				t.Fatalf("a lockout went before the dump: % X", d)
			}
			ms, err := buffer.NewReader(d[7:]).Read7BitEncodedInt64()
			if err != nil {
				t.Fatal(err)
			}
			told[d[2]] = ms
			n++
		}
	}
	if !dump {
		t.Fatal("no MemoryDump")
	}
	return told, n
}

// A login with lockouts kept (charlockouts): the one still running of a
// spell in the book is put back on the ability the book has it at now and
// its badge told after the dump, once; one of a spell the book has no more
// refuses its spell and is told on nothing; one over by the login is not put
// back, and its spell is cast (the control).
func TestALoginPutsBackItsLockoutsOnItsBook(t *testing.T) {
	rec := &database.CharacterRecord{CharName: "Recaster", AccountID: 7, Level: 50, TClass: 9}
	r := newTakeoverRig(t, rec)
	lockoutBook(t, r, rec.ServerID)
	now := time.Now()
	if err := r.store.SaveLockouts(rec.ServerID, []database.LockoutRow{
		{SpellID: 901, Until: now.Add(10 * time.Minute)},
		{SpellID: 902, Until: now.Add(-time.Second)},
		{SpellID: 903, Until: now.Add(5 * time.Minute)}, // a spell the book has no more
	}); err != nil {
		t.Fatal(err)
	}
	win := r.window(7)
	r.enter(win, rec.ServerID)
	told, n := toldLockouts(t, win)
	if ms, ok := told[3]; !ok || n != 1 || ms > 600_000 || ms < 590_000 {
		t.Fatalf("told after the dump: %v (%d sent); want ability 3 alone, about 600000 ms", told, n)
	}
	gone := entity.CastSpell{SpellID: 903, DisplayIndex: 9, Name: "Gone Ward"}
	if r.w.StartCast(win, gone, win.PlayerObjectID(), time.Now()) {
		t.Fatal("a spell out of the book was cast inside its kept lockout")
	}
	quick := entity.CastSpell{SpellID: 902, DisplayIndex: 4, Name: "Quick Ward", Recast: time.Second}
	if !r.w.StartCast(win, quick, win.PlayerObjectID(), time.Now()) {
		t.Fatal("the control: a spell whose kept lockout was over was refused")
	}
}

// A takeover is as it was: the live Player's lockout goes on, its badge told
// once after the new window's dump (SendTakenOver, not again by the login's
// SendLockouts); and the takeover writes it to the row too (saveTakenOver),
// so a later login has it whatever becomes of the new client.
func TestATakeoverKeepsAndWritesItsLockouts(t *testing.T) {
	rec := &database.CharacterRecord{CharName: "Recaster", AccountID: 7, Level: 50, TClass: 9}
	r := newTakeoverRig(t, rec)
	lockoutBook(t, r, rec.ServerID)
	first := r.window(7)
	r.enter(first, rec.ServerID)
	long := entity.CastSpell{SpellID: 901, DisplayIndex: 3, Name: "Long Ward", Recast: 600 * time.Second}
	p := r.w.SessionPlayer(first)
	cast := time.Now()
	if !r.w.StartCast(first, long, first.PlayerObjectID(), cast) {
		t.Fatal("not cast")
	}
	if rows, _ := r.store.LoadLockouts(rec.ServerID, cast); len(rows) != 0 {
		t.Fatalf("written before any session ended: %+v", rows)
	}

	second := r.window(7)
	r.enter(second, rec.ServerID)
	if p == nil || r.w.SessionPlayer(second) != p {
		t.Fatal("not taken over")
	}
	told, n := toldLockouts(t, second)
	if ms, ok := told[3]; !ok || n != 1 || ms > 600_300 || ms < 590_000 {
		t.Fatalf("told after the takeover's dump: %v (%d sent); want ability 3 once, about 600300 ms", told, n)
	}
	rows, err := r.store.LoadLockouts(rec.ServerID, time.Now())
	want := cast.Add(600*time.Second + 300*time.Millisecond).Truncate(time.Millisecond)
	if err != nil || len(rows) != 1 || rows[0].SpellID != 901 || !rows[0].Until.Equal(want) {
		t.Fatalf("the takeover kept %+v (%v), want spell 901 until %v", rows, err, want)
	}
	if r.w.StartCast(second, long, second.PlayerObjectID(), time.Now()) {
		t.Fatal("the taken-over character cast inside its lockout")
	}
}

// A session kept through a restart is as it was: built from its row (whose
// lockouts the hold's SaveAll wrote), then given the lockouts its old process
// kept of it, which are the ones it has (the row's are replaced); its resume
// tells each once after its dump.
func TestAKeptSessionsLockoutsAreItsOwn(t *testing.T) {
	db := keepDB(t)
	_, gw, sm := keepProcess(t, db)
	store := database.NewSQLiteCharacterStore(db)
	if err := store.SaveLockouts(7, []database.LockoutRow{{SpellID: 901, Until: time.Now().Add(10 * time.Minute)}}); err != nil {
		t.Fatal(err)
	}
	k := session.Kept{Addr: "203.0.113.7:5007", InstanceID: 7, SessionID: 500000007, ClientEndpoint: 7, ServerEndpoint: 1,
		State: session.StateInGame, HasInstance: true, DidServerInitiate: true, CharacterInWorld: true, InGame: true,
		AccountID: 3, CharServerID: 7, PlayerObjectID: 500000007}
	rs, err := sm.Resume(k)
	if err != nil {
		t.Fatal(err)
	}
	kp := entity.KeptPlayer{CharServerID: 7, Name: "Hero", Lockouts: []entity.KeptLockout{
		{Spell: 901, Index: 6, LeftMS: 300_000}, {Spell: 902, Index: 7, LeftMS: 60_000}}}
	if _, err := ResumeCharacter(rs, KeptExtras{}, kp, time.Now()); err != nil {
		t.Fatal(err)
	}
	kept, ok := gw.KeepPlayer(rs, time.Now())
	if !ok || len(kept.Lockouts) != 2 || kept.Lockouts[0].Spell != 901 || kept.Lockouts[0].Index != 6 ||
		kept.Lockouts[0].LeftMS > 300_000 || kept.Lockouts[0].LeftMS < 290_000 || kept.Lockouts[1].Spell != 902 {
		t.Fatalf("the resumed character's lockouts: %+v; want the kept 901 (5 minutes, ability 6) and 902", kept.Lockouts)
	}
	if !ResumeDump(rs, false) {
		t.Fatal("no dump")
	}
	told, n := toldLockouts(t, rs)
	if n != 2 || told[6] > 300_000 || told[6] < 290_000 || told[7] > 60_000 || told[7] < 50_000 {
		t.Fatalf("told after the resume's dump: %v (%d sent); want abilities 6 and 7 once each", told, n)
	}
}
