package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// twoShards is the owner's database and a shard's, its mirror of the owner's.
func twoShards(t *testing.T) (owner, shard *SQLiteDB, m *AccountMirror, ownerPath string) {
	t.Helper()
	dir := t.TempDir()
	ownerPath = filepath.Join(dir, "owner.db")
	owner, err := NewSQLite(ownerPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Close() })
	shard, err = NewSQLite(filepath.Join(dir, "shard.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { shard.Close() })
	return owner, shard, NewAccountMirror(shard, ownerPath), ownerPath
}

// force makes the next Pull read the file whatever its size and time say.
func (m *AccountMirror) force() {
	m.mu.Lock()
	m.lastTime = time.Time{}
	m.mu.Unlock()
}

func accountOf(t *testing.T, db *SQLiteDB, name string) *AccountRecord {
	t.Helper()
	a, err := NewSQLiteAccountStore(db).GetByUsername(name)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return a
}

// A Pull copies accounts with their ids, passwords, levels and bans; what changes
// at the owner reaches the copy; a local last login stays; an account gone at the
// owner is gone from the copy. Nothing is written back.
func TestTheMirrorFollowsTheOwner(t *testing.T) {
	owner, shard, m, _ := twoShards(t)
	store := NewSQLiteAccountStore(owner)
	dab, err := store.Create("dab", "hash-1")
	if err != nil {
		t.Fatal(err)
	}
	jib, _ := store.Create("jib", "hash-2")
	if _, err := owner.Exec(`UPDATE accountinfo SET acctlevel = 2 WHERE accountid = ?`, jib.ID); err != nil {
		t.Fatal(err)
	}
	ch, gone, total, err := m.Pull()
	if err != nil || ch != 2 || gone != 0 || total != 2 {
		t.Fatalf("the first pull: %d changed, %d gone, %d in all, %v", ch, gone, total, err)
	}
	if a := accountOf(t, shard, "DAB"); a.ID != dab.ID || a.PasswordHash != "hash-1" || a.AccessLevel != 0 {
		t.Errorf("dab's copy: %+v", a)
	}
	if a := accountOf(t, shard, "jib"); a.ID != jib.ID || a.AccessLevel != 2 {
		t.Errorf("jib's copy: %+v", a)
	}
	// a date is copied as the text it is, not in the driver's spelling of a time
	var dateThere, dateHere string
	if err := owner.QueryRow(`SELECT CAST(acctcreation AS TEXT) FROM accountinfo WHERE accountid = ?`, dab.ID).Scan(&dateThere); err != nil {
		t.Fatal(err)
	}
	if err := shard.QueryRow(`SELECT CAST(acctcreation AS TEXT) FROM accountinfo WHERE accountid = ?`, dab.ID).Scan(&dateHere); err != nil {
		t.Fatal(err)
	}
	if dateThere != dateHere {
		t.Errorf("acctcreation copied from %q to %q", dateThere, dateHere)
	}

	// nothing changed: nothing written
	m.force()
	if ch, gone, _, err := m.Pull(); err != nil || ch != 0 || gone != 0 {
		t.Errorf("an idle pull: %d changed, %d gone, %v", ch, gone, err)
	}

	// a shard's own last login stays; a new password, a ban and a new account come
	shardStore := NewSQLiteAccountStore(shard)
	if err := shardStore.UpdateLastLogin(dab.ID); err != nil {
		t.Fatal(err)
	}
	var loginHere string
	if err := shard.QueryRow(`SELECT lastlogin FROM accountinfo WHERE accountid = ?`, dab.ID).Scan(&loginHere); err != nil {
		t.Fatal(err)
	}
	store.UpdatePasswordHash(dab.ID, "hash-3")
	store.SetBanned(jib.ID, true)
	newcomer, _ := store.Create("newcomer", "hash-4")
	m.force()
	if ch, _, total, err := m.Pull(); err != nil || ch != 3 || total != 3 {
		t.Fatalf("the next pull: %d changed, %d in all, %v", ch, total, err)
	}
	if a := accountOf(t, shard, "dab"); a.PasswordHash != "hash-3" {
		t.Errorf("dab's new password: %+v", a)
	}
	var loginAfter string
	if err := shard.QueryRow(`SELECT lastlogin FROM accountinfo WHERE accountid = ?`, dab.ID).Scan(&loginAfter); err != nil {
		t.Fatal(err)
	}
	if loginAfter != loginHere {
		t.Errorf("the shard's own last login went from %q to %q", loginHere, loginAfter)
	}
	if a := accountOf(t, shard, "jib"); !a.Banned {
		t.Errorf("jib's ban: %+v", a)
	}
	if a := accountOf(t, shard, "newcomer"); a.ID != newcomer.ID {
		t.Errorf("newcomer: %+v", a)
	}

	// a shard-side change never reaches the owner, and is put right
	shardStore.UpdatePasswordHash(dab.ID, "written here")
	m.force()
	if _, _, _, err := m.Pull(); err != nil {
		t.Fatal(err)
	}
	if a := accountOf(t, shard, "dab"); a.PasswordHash != "hash-3" {
		t.Errorf("the copy kept a change of its own: %+v", a)
	}
	if a := accountOf(t, owner, "dab"); a.PasswordHash != "hash-3" {
		t.Errorf("the owner's row changed: %+v", a)
	}

	// an account deleted at the owner
	if _, err := owner.Exec(`DELETE FROM accountinfo WHERE accountid = ?`, newcomer.ID); err != nil {
		t.Fatal(err)
	}
	m.force()
	if _, gone, _, err := m.Pull(); err != nil || gone != 1 {
		t.Fatalf("a deleted account: %d gone, %v", gone, err)
	}
	if _, err := shardStore.GetByUsername("newcomer"); err == nil {
		t.Error("the deleted account can still be found")
	}
}

// A name that moved between accounts comes across whole, whichever order the
// rows are copied in: UNIQUE(username) must not lose one.
func TestTheMirrorTakesARenameAndANameReused(t *testing.T) {
	owner, shard, m, _ := twoShards(t)
	store := NewSQLiteAccountStore(owner)
	a, _ := store.Create("alpha", "h1")
	b, _ := store.Create("beta", "h2")
	if _, _, _, err := m.Pull(); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(`UPDATE accountinfo SET username = 'beta2' WHERE accountid = ?`, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(`UPDATE accountinfo SET username = 'beta' WHERE accountid = ?`, a.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // a REPLACE that took another's row is put back by the next
		m.force()
		if _, _, _, err := m.Pull(); err != nil {
			t.Fatal(err)
		}
	}
	if got := accountOf(t, shard, "beta"); got.ID != a.ID {
		t.Errorf("beta is account %d, want %d", got.ID, a.ID)
	}
	if got := accountOf(t, shard, "beta2"); got.ID != b.ID {
		t.Errorf("beta2 is account %d, want %d", got.ID, b.ID)
	}
}

// PullOne has a sign-in see the account as it is now: one made since the last
// Pull, a new password, a ban; one gone at the owner is gone here. With the file
// missing it says so and the copy stands.
func TestPullOneForASignIn(t *testing.T) {
	owner, shard, m, path := twoShards(t)
	store := NewSQLiteAccountStore(owner)
	if err := m.PullOne("nobody"); err != nil {
		t.Fatalf("no such account anywhere: %v", err)
	}
	made, _ := store.Create("Made", "h1")
	if err := m.PullOne("made"); err != nil {
		t.Fatal(err)
	}
	if a := accountOf(t, shard, "made"); a.ID != made.ID || a.PasswordHash != "h1" {
		t.Fatalf("after PullOne: %+v", a)
	}
	store.UpdatePasswordHash(made.ID, "h2")
	if err := m.PullOne("MADE"); err != nil {
		t.Fatal(err)
	}
	if a := accountOf(t, shard, "made"); a.PasswordHash != "h2" {
		t.Errorf("the new password: %+v", a)
	}
	owner.Close()
	moved := path + ".moved"
	if err := os.Rename(path, moved); err != nil {
		t.Fatal(err)
	}
	if err := m.PullOne("made"); err == nil {
		t.Error("a missing file: no error")
	}
	if a := accountOf(t, shard, "made"); a.PasswordHash != "h2" {
		t.Errorf("the copy after a failed read: %+v", a)
	}
	if err := os.Rename(moved, path); err != nil {
		t.Fatal(err)
	}
	again, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if _, err := again.Exec(`DELETE FROM accountinfo WHERE accountid = ?`, made.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.PullOne("made"); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSQLiteAccountStore(shard).GetByUsername("made"); err == nil {
		t.Error("an account gone at the owner still signs in here")
	}
}

// A file put back by mistake, or a path naming another database, takes no more
// than a few accounts away: the copy stands and the log says so. A file with no
// accounts at all is refused outright. The control: a few gone are taken away.
func TestAPullWillNotEmptyTheAccounts(t *testing.T) {
	owner, shard, m, _ := twoShards(t)
	store := NewSQLiteAccountStore(owner)
	for _, n := range strings.Fields("a1 a2 a3 a4 a5 a6 a7 a8 a9 b1 b2 b3 c1 c2") {
		if _, err := store.Create(n, "h"); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, _, err := m.Pull(); err != nil {
		t.Fatal(err)
	}
	count := func() (n int) {
		shard.QueryRow(`SELECT COUNT(*) FROM accountinfo`).Scan(&n)
		return
	}
	if _, err := owner.Exec(`DELETE FROM accountinfo WHERE username LIKE 'c%'`); err != nil {
		t.Fatal(err)
	}
	m.force()
	if _, gone, _, err := m.Pull(); err != nil || gone != 2 || count() != 12 {
		t.Errorf("the control, two of fourteen gone: %d taken away, %d left, %v", gone, count(), err)
	}
	if _, err := owner.Exec(`DELETE FROM accountinfo WHERE username LIKE 'a%'`); err != nil {
		t.Fatal(err)
	}
	m.force()
	if _, gone, _, err := m.Pull(); err != nil || gone != 0 || count() != 12 {
		t.Errorf("nine of twelve gone at once: %d taken away, %d left, %v", gone, count(), err)
	}
	if _, err := owner.Exec(`DELETE FROM accountinfo`); err != nil {
		t.Fatal(err)
	}
	m.force()
	if _, _, _, err := m.Pull(); err == nil || count() != 12 {
		t.Errorf("no accounts at all: %v, %d left", err, count())
	}
}

// An unchanged file is not opened: an idle pair of shards does nothing here,
// until the time limit says to look anyway. Hold stops every pull.
func TestAnUnchangedFileIsNotRead(t *testing.T) {
	owner, _, m, path := twoShards(t)
	NewSQLiteAccountStore(owner).Create("dab", "h")
	if _, _, total, err := m.Pull(); err != nil || total != 1 {
		t.Fatalf("the first pull: %d, %v", total, err)
	}
	if _, _, total, err := m.Pull(); err != nil || total != 0 {
		t.Errorf("an unchanged file was read: total %d, %v", total, err)
	}
	later := time.Now().Add(2 * mirrorMaxQuiet)
	m.now = func() time.Time { return later }
	if _, _, total, err := m.Pull(); err != nil || total != 1 {
		t.Errorf("past the limit: total %d, %v", total, err)
	}
	if _, _, total, err := m.Pull(); err != nil || total != 0 {
		t.Errorf("right after: total %d, %v", total, err)
	}
	m.Hold(true)
	future := later.Add(time.Hour)
	m.now = func() time.Time { return future }
	if _, _, total, err := m.Pull(); err != nil || total != 0 {
		t.Errorf("held: total %d, %v", total, err)
	}
	if err := m.PullOne("dab"); err != nil {
		t.Errorf("held: %v", err)
	}
	m.Hold(false)
	if _, _, total, err := m.Pull(); err != nil || total != 1 {
		t.Errorf("let go: total %d, %v", total, err)
	}
	_ = path
}

// The mirror reads while the owner holds its database, and the owner writes while
// the mirror reads: a commit waits out a read (busy_timeout), never fails.
func TestTheMirrorAndTheOwnerTakeTurns(t *testing.T) {
	owner, _, m, _ := twoShards(t)
	store := NewSQLiteAccountStore(owner)
	store.Create("dab", "h")
	stop := make(chan struct{})
	errs := make(chan error, 1)
	go func() {
		defer close(errs)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			if err := store.UpdatePasswordHash(1, "h"+strings.Repeat("x", i%7)); err != nil {
				errs <- err
				return
			}
		}
	}()
	for i := 0; i < 50; i++ {
		m.force()
		if _, _, _, err := m.Pull(); err != nil {
			t.Errorf("pull %d: %v", i, err)
			break
		}
	}
	close(stop)
	if err := <-errs; err != nil {
		t.Errorf("the owner's write: %v", err)
	}
}

// The panel's roles go with the accounts (gmweb/shard.go): a tester, a sub GM
// and a storyteller of the owner's are one in the copy too, a role taken away
// there is gone here, and a SubGM check here reads it. The control: a shard
// whose database has no role table (its panel not started yet) is passed over.
func TestTheMirrorCarriesTheRoles(t *testing.T) {
	owner, shard, m, _ := twoShards(t)
	store := NewSQLiteAccountStore(owner)
	kobe, _ := store.Create("forestwood", "h")
	tess, _ := store.Create("zaeire", "h")
	story, _ := store.Create("new1", "h")
	ddl := `CREATE TABLE gm_testers (accountid INTEGER PRIMARY KEY, granted_by TEXT NOT NULL DEFAULT '',
		granted_at TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', subgm INTEGER NOT NULL DEFAULT 0);
		CREATE TABLE gm_storytellers (accountid INTEGER PRIMARY KEY, granted_by TEXT NOT NULL DEFAULT '',
		granted_at TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '');`
	if _, err := owner.Exec(ddl); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(`INSERT INTO gm_testers (accountid, granted_by, subgm) VALUES (?, 'dab', 1), (?, 'dab', 0)`, kobe.ID, tess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(`INSERT INTO gm_storytellers (accountid, granted_by) VALUES (?, 'dab')`, story.ID); err != nil {
		t.Fatal(err)
	}
	// the shard's panel has not made its tables yet: accounts copied, roles passed over
	if _, _, _, err := m.Pull(); err != nil {
		t.Fatal(err)
	}
	if _, err := shard.Exec(ddl); err != nil {
		t.Fatal(err)
	}
	m.force()
	if _, _, _, err := m.Pull(); err != nil {
		t.Fatal(err)
	}
	here := NewSQLiteAccountStore(shard)
	if yes, err := here.SubGM(kobe.ID); err != nil || !yes {
		t.Fatalf("the sub GM here: %v %v", yes, err)
	}
	if yes, _ := here.SubGM(tess.ID); yes {
		t.Fatal("a tester made a sub GM")
	}
	var n int
	shard.QueryRow(`SELECT COUNT(*) FROM gm_testers`).Scan(&n)
	if n != 2 {
		t.Fatalf("%d testers here", n)
	}
	shard.QueryRow(`SELECT COUNT(*) FROM gm_storytellers WHERE accountid = ?`, story.ID).Scan(&n)
	if n != 1 {
		t.Fatal("the storyteller not copied")
	}
	// a role taken away there
	if _, err := owner.Exec(`DELETE FROM gm_testers WHERE accountid = ?`, kobe.ID); err != nil {
		t.Fatal(err)
	}
	m.force()
	if _, _, _, err := m.Pull(); err != nil {
		t.Fatal(err)
	}
	if yes, _ := here.SubGM(kobe.ID); yes {
		t.Fatal("a sub GM taken away there is one here still")
	}
	// nothing changed: nothing written
	m.force()
	if ch, gone, _, err := m.Pull(); err != nil || ch != 0 || gone != 0 {
		t.Fatalf("an unchanged pull: %d %d %v", ch, gone, err)
	}
}
