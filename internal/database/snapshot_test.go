package database

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecretLikeNames(t *testing.T) {
	for _, name := range []string{
		"pwhash", "password", "user_password", "Passwd", "passcode", "pw", "token", "gm_tokens", "refresh_token",
		"secret", "client_secret", "hash", "addr_hash", "sender_hash", "salt", "key", "rowkey", "api_key", "apiKey",
		"email", "e_mail", "mailbox", "ip", "last_ip", "ipaddress", "IPAddress", "ipAddress", "clientIP", "remoteip",
		"ipv4", "ip_addr", "address", "session", "sessions", "ticket", "login_tickets", "lastlogin", "cookie", "nonce",
		"oauth_codes", "totp", "otp", "auth", "firstname", "lastName", "surname", "birthday", "birthyear", "zip",
		"phone", "mac", "device_id", "install_id", "private_note", "signing_keys", "bearer", "jwt", "credentials",
	} {
		if !SecretLikeName(name) {
			t.Errorf("%q is not read as a secret's name", name)
		}
	}
	for _, name := range []string{
		"characters", "charName", "accountid", "serverid", "npcs", "npc_name", "itempattern", "equipslot", "equiploc",
		"description", "recipe", "craftrecipes", "spelldesc", "tip", "ship", "zipper", "modelid", "tunar", "bankTunar",
		"x_coord", "facing", "world", "quests", "queststeps", "journal", "offer", "username", "acctlevel", "countryAB",
		"weapondamage", "unk42_1", "player_flags", "workboard", "notes", "classtemplates", "npctemplate", "racesize",
		"auction_lots", "seller_name", "duel_stakes", "spellfx_suggestions", "mapmarks", "gmtriggers", "steps",
	} {
		if SecretLikeName(name) {
			t.Errorf("%q is read as a secret's name", name)
		}
	}
}

// panelTablesForTest are the GM panel's own tables (internal/gmweb/store.go),
// which the sanitiser clears by name whatever their columns.
const panelTablesForTest = `
CREATE TABLE IF NOT EXISTS gm_totp (accountid INTEGER PRIMARY KEY, secret TEXT NOT NULL, enabled TEXT NOT NULL,
	last_step INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS gm_tokens (id INTEGER PRIMARY KEY AUTOINCREMENT, accountid INTEGER NOT NULL, name TEXT NOT NULL,
	kind TEXT NOT NULL, hash TEXT NOT NULL UNIQUE, prefix TEXT NOT NULL, scope TEXT NOT NULL, client TEXT NOT NULL DEFAULT '',
	parent INTEGER, created TEXT NOT NULL, expires TEXT, last_used TEXT, last_ip TEXT NOT NULL DEFAULT '',
	revoked INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS gm_oauth_clients (client_id TEXT PRIMARY KEY, name TEXT NOT NULL, redirect_uris TEXT NOT NULL,
	created TEXT NOT NULL, last_used TEXT);`

// The values planted in every column that holds a secret or a person's
// data: none may survive a snapshot (and each is in the source file: the
// control, without which "not found" proves nothing).
var planted = map[string]string{
	"accountinfo.pwhash":    "$2a$04$plantedplantedplantedplantedplantedplantedplantedplant",
	"accountinfo.email":     "planted.person@example.org",
	"accountinfo.ipaddress": "203.0.113.77",
	"accountinfo.firstname": "Plantedfirstname",
	"accountinfo.lastname":  "Plantedlastname",
	"accountinfo.zip":       "90210-PLANTED",
	"accountinfo.birthday":  "PLANTED-1999-12-31",
	"accountinfo.username":  "planted-player-login",
	"gm_tokens.hash":        "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	"gm_tokens.last_ip":     "198.51.100.23",
	"gm_totp.secret":        "PLANTEDTOTPSECRETBASE32AAAAAAAAA",
	"gm_oauth_clients.id":   "planted-oauth-client-id",
	"gm_oauth_clients.uri":  "http://127.0.0.1:33418/planted-callback",
	"gm_audit.ip":           "192.0.2.44",
	"gm_audit.detail":       "UPDATE accountinfo SET pwhash = 'planted-in-an-sql-line'",
	"spellfx.addr_hash":     "aaaa1111bbbb2222cccc3333dddd4444eeee5555ffff6666aaaa7777bbbb8888",
	"mapmark.sender_hash":   "9999eeee8888dddd7777cccc6666bbbb5555aaaa4444ffff3333eeee2222dddd",
	"workboard.notes":       "the test login is keeper / hunter2-planted-password",
	"workboard.overview":    "PLANTED overview: the box's root password is not here",
}

// seededDB is a database with the game's tables, the panel's, content, a GM
// and a player, and a planted secret in every column that holds one.
func seededDB(t *testing.T) (*SQLiteDB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "eqoa.db")
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(panelTablesForTest)
	p := planted
	exec(`INSERT INTO accountinfo (accountid, username, pwhash, acctstatus, acctlevel, acctcreation, lastlogin, ipaddress,
		firstname, lastname, zip, birthday, email) VALUES
		(1, 'keeper', ?, 1, 2, '2026-10-01 10:00:00', '2026-10-29 10:00:00', '203.0.113.78', 'Gmfirst', 'Gmlast', '', '', ''),
		(2, ?, ?, 1, 0, '2026-10-02 10:00:00', '2026-10-29 11:00:00', ?, ?, ?, ?, ?, ?)`,
		p["accountinfo.pwhash"][:len(p["accountinfo.pwhash"])-1]+"K", p["accountinfo.username"], p["accountinfo.pwhash"],
		p["accountinfo.ipaddress"], p["accountinfo.firstname"], p["accountinfo.lastname"], p["accountinfo.zip"],
		p["accountinfo.birthday"], p["accountinfo.email"])
	exec(`INSERT INTO characters (charName, accountid, serverid, modelid, tclass, race, humType, level, haircolor,
		hairlength, hairstyle, faceoption, sex, world, x, y, z, tunar) VALUES
		('Hero', 2, 7, 1, 0, 0, 1, 10, 0, 0, 0, 0, 0, 0, 25300, 54, 15700, 100),
		('Keepsake', 1, 8, 1, 9, 1, 0, 60, 0, 0, 0, 0, 1, 0, 25310, 54, 15710, 5)`)
	exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race)
		VALUES (500, 'Guard Dahvel', 0, 25000, 50, 15000, 0, 1893243078, 1.0, 30, 128, '0'),
		       (501, 'Merchant Shadehand', 0, 25010, 50, 15010, 0, 1893243078, 1.0, 20, 129, '0')`)
	exec(`INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname,
		itemdesc, model, color) VALUES (10, 5, 0, 0, 0, 12, 0, 1, 0, 0, 0, 7, 0, 1, 1, 100, 0, 32767, 1023, 0, 0, 0, 0,
		'Rusty Sword', '', 0, 0)`)
	exec(`INSERT INTO gm_tokens (accountid, name, kind, hash, prefix, scope, created, last_ip)
		VALUES (1, 'Claude', 'oauth', ?, 'gmo_abc', 'admin', '2026-10-29T10:00:00Z', ?)`, p["gm_tokens.hash"], p["gm_tokens.last_ip"])
	exec(`INSERT INTO gm_totp (accountid, secret, enabled) VALUES (1, ?, '2026-10-29T10:00:00Z')`, p["gm_totp.secret"])
	exec(`INSERT INTO gm_oauth_clients (client_id, name, redirect_uris, created) VALUES (?, 'Claude Code', ?, '2026-10-29T10:00:00Z')`,
		p["gm_oauth_clients.id"], `["`+p["gm_oauth_clients.uri"]+`"]`)
	exec(`INSERT INTO gm_audit (at, accountid, username, via, ip, action, detail, before)
		VALUES ('2026-10-29T10:00:00Z', 1, 'keeper', 'web', ?, 'sql', ?, ?)`, p["gm_audit.ip"], p["gm_audit.detail"],
		`{"email": "`+p["accountinfo.email"]+`"}`)
	if _, err := db.SpellFXSalt(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.MapMarkSalt(); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO spellfx_suggestions (spellid, fxdict, name, note, addr_hash, created) VALUES
		(100, 1, 'Hero', 'looks right', ?, '2026-10-29T10:00:00Z'),
		(101, 2, 'Hero', 'this one too', ?, '2026-10-29T10:01:00Z'),
		(102, 3, 'Someone', '', 'ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff0000', '2026-10-29T10:02:00Z')`,
		p["spellfx.addr_hash"], p["spellfx.addr_hash"])
	exec(`INSERT INTO mapmark_suggestions (world, x, z, name, note, client, sender_hash, created) VALUES
		(0, 25000, 15000, 'The well', '', 'vanilla', ?, '2026-10-29T10:00:00Z')`, p["mapmark.sender_hash"])
	who := GMEdit{AccountID: 1, Username: "keeper", Via: "web", IP: "203.0.113.78"}
	if _, _, err := db.PostWork(WorkPost{Title: workStr("Test the snapshot"), Note: p["workboard.notes"]}, who); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.SetWorkOverview(p["workboard.overview"], 1, who); err != nil {
		t.Fatal(err)
	}
	return db, path
}

// plantedValues is every planted value, with the salts the database made.
func plantedValues(t *testing.T, db *SQLiteDB) map[string]string {
	t.Helper()
	out := map[string]string{}
	for k, v := range planted {
		out[k] = v
	}
	var s1, s2 string
	db.QueryRow(`SELECT value FROM spellfx_meta WHERE key = 'salt'`).Scan(&s1)
	db.QueryRow(`SELECT value FROM mapmark_meta WHERE key = 'salt'`).Scan(&s2)
	if s1 == "" || s2 == "" {
		t.Fatal("no salts made")
	}
	out["spellfx_meta.salt"], out["mapmark_meta.salt"] = s1, s2
	return out
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestASnapshotTakesOutEverySecretAndKeepsTheGame(t *testing.T) {
	db, path := seededDB(t)
	values := plantedValues(t, db)
	src := readFile(t, path)
	for k, v := range values {
		if !bytes.Contains(src, []byte(v)) {
			t.Fatalf("control: planted %s is not in the source file, so not finding it later would prove nothing", k)
		}
	}

	out := filepath.Join(t.TempDir(), "snap.db")
	rep, err := db.WriteSnapshot(out, "keeper")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Version != SnapshotVersion || rep.Bytes == 0 || rep.Scanned == 0 {
		t.Fatalf("report %+v", rep)
	}
	got := readFile(t, out)
	for k, v := range values {
		if bytes.Contains(got, []byte(v)) {
			t.Errorf("planted %s survived the snapshot", k)
		}
	}
	if _, err := os.Stat(out + ".copy"); !os.IsNotExist(err) {
		t.Errorf("the unsanitised copy was left beside the snapshot: %v", err)
	}
	if fi, _ := os.Stat(out); fi.Mode().Perm() != 0o600 {
		t.Errorf("the snapshot is mode %v, not 0600", fi.Mode().Perm())
	}

	// the copy opens as the real-data tests open one, and the game is whole
	copyPath := filepath.Join(t.TempDir(), "copy.db")
	os.WriteFile(copyPath, got, 0o600)
	snap, err := NewSQLite(copyPath)
	if err != nil {
		t.Fatalf("the snapshot does not open as a server database: %v", err)
	}
	defer snap.Close()
	count := func(d *SQLiteDB, q string) (n int) {
		t.Helper()
		if err := d.QueryRow(q).Scan(&n); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return
	}
	for _, table := range []string{"characters", "npcs", "itempattern", "accountinfo", "spellfx_suggestions", "mapmark_suggestions"} {
		if a, b := count(db, `SELECT COUNT(*) FROM `+table), count(snap, `SELECT COUNT(*) FROM `+table); a != b {
			t.Errorf("%s: %d rows, %d in the snapshot", table, a, b)
		}
	}
	for _, table := range []string{"gm_tokens", "gm_totp", "gm_oauth_clients", "gm_audit", "spellfx_meta", "mapmark_meta", "workboard"} {
		if n := count(snap, `SELECT COUNT(*) FROM `+table); n != 0 {
			t.Errorf("%s kept %d rows", table, n)
		}
	}
	if n := count(snap, `SELECT COUNT(*) FROM characters WHERE charName = 'Hero' AND accountid = 2 AND level = 10 AND tunar = 100`); n != 1 {
		t.Error("the player's character is not as it was")
	}
	var name string
	snap.QueryRow(`SELECT username FROM accountinfo WHERE accountid = 2`).Scan(&name)
	if name != "account-2" {
		t.Errorf("the player's login name is %q, not account-2", name)
	}
	snap.QueryRow(`SELECT username FROM accountinfo WHERE accountid = 1`).Scan(&name)
	if name != "keeper" {
		t.Errorf("the GM's login name is %q, not kept", name)
	}
	if n := count(snap, `SELECT COUNT(*) FROM accountinfo WHERE pwhash IS NOT NULL OR email IS NOT NULL OR ipaddress IS NOT NULL
		OR lastlogin IS NOT NULL OR firstname IS NOT NULL`); n != 0 {
		t.Errorf("%d accounts kept a person's data", n)
	}
	if n := count(snap, `SELECT COUNT(*) FROM accountinfo WHERE acctlevel = 2 AND acctstatus = 1 AND acctcreation IS NOT NULL`); n != 1 {
		t.Error("the GM's level, status or creation were lost")
	}
	// one sender's suggestions still group, under a stand-in of the same shape
	if n := count(snap, `SELECT COUNT(DISTINCT addr_hash) FROM spellfx_suggestions`); n != 2 {
		t.Errorf("%d senders among the suggestions, not 2", n)
	}
	if n := count(snap, `SELECT COUNT(*) FROM spellfx_suggestions WHERE length(addr_hash) != 64`); n != 0 {
		t.Error("a stand-in is not as long as the hash it replaced")
	}
	// the work board's triggers are there, and the overview is seeded again on open
	if n := count(snap, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'trigger' AND name LIKE 'workboard%'`); n != 2 {
		t.Errorf("%d work board triggers in the snapshot, not 2", n)
	}
	info, err := ReadSnapshotInfo(snap.DB)
	if err != nil || info["sanitiser"] != "1" || info["by"] != "keeper" || !strings.Contains(info["removed"], "gm_tokens") {
		t.Errorf("snapshot_info %v, %v", info, err)
	}
	if _, err := ReadSnapshotInfo(db.DB); !errors.Is(err, ErrSnapshotNotSanitised) {
		t.Errorf("the live database read as a snapshot: %v", err)
	}
	// and a snapshot of the snapshot is refused nothing
	again := filepath.Join(t.TempDir(), "again.db")
	if _, err := snap.WriteSnapshot(again, "keeper"); err != nil {
		t.Errorf("a snapshot of a snapshot: %v", err)
	}
}

// refused makes a snapshot that must be refused, and says why.
func refused(t *testing.T, db *SQLiteDB) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "snap.db")
	_, err := db.WriteSnapshot(out, "keeper")
	var r *SnapshotRefused
	if !errors.As(err, &r) {
		t.Fatalf("not refused: %v", err)
	}
	left, _ := os.ReadDir(dir)
	if len(left) != 0 {
		t.Fatalf("a refused snapshot left %d files (%s...)", len(left), left[0].Name())
	}
	return err.Error()
}

func TestASnapshotRefusesWhatItDoesNotKnow(t *testing.T) {
	cases := []struct {
		name, stmt, want string
	}{
		{"an unknown table named like a secret", `CREATE TABLE login_attempts (id INTEGER, at TEXT)`, "table login_attempts"},
		{"an unknown table with a column named like one", `CREATE TABLE guild_notes (id INTEGER, author_email TEXT)`,
			"guild_notes.author_email"},
		{"a known table's new column named like one", `ALTER TABLE characters ADD COLUMN recovery_email TEXT`,
			"characters.recovery_email"},
		{"a tool's log with a key the rules do not name", `CREATE TABLE new_fix_log (seq INTEGER, rowkey TEXT)`,
			"new_fix_log.rowkey"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, _ := seededDB(t)
			if _, err := db.Exec(tc.stmt); err != nil {
				t.Fatal(err)
			}
			if msg := refused(t, db); !strings.Contains(msg, tc.want) || !strings.Contains(msg, "snapshotRules") {
				t.Errorf("refused, but the message does not name %s and the fix: %s", tc.want, msg)
			}
		})
	}
}

func TestASnapshotKeepsAnUnknownTableNamedPlainly(t *testing.T) {
	db, _ := seededDB(t)
	if _, err := db.Exec(`CREATE TABLE fan_art (id INTEGER PRIMARY KEY, title TEXT); INSERT INTO fan_art VALUES (1, 'A gnome')`); err != nil {
		t.Fatal(err)
	}
	// and accountinfo keeps only the game's columns: one added later is blanked, not refused
	if _, err := db.Exec(`ALTER TABLE accountinfo ADD COLUMN phone2 TEXT; UPDATE accountinfo SET phone2 = '555-0100-planted'`); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "snap.db")
	if _, err := db.WriteSnapshot(out, "keeper"); err != nil {
		t.Fatal(err)
	}
	snap, err := NewSQLite(out)
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Close()
	var title string
	if err := snap.QueryRow(`SELECT title FROM fan_art WHERE id = 1`).Scan(&title); err != nil || title != "A gnome" {
		t.Errorf("fan_art: %q %v", title, err)
	}
	if bytes.Contains(readFile(t, out), []byte("555-0100-planted")) {
		t.Error("accountinfo's added column survived")
	}
}

func TestASnapshotRefusesASecretCopiedIntoTheContent(t *testing.T) {
	db, _ := seededDB(t)
	// control: as seeded it is made
	if _, err := db.WriteSnapshot(filepath.Join(t.TempDir(), "ok.db"), "keeper"); err != nil {
		t.Fatalf("control: %v", err)
	}
	// a player's e-mail typed into an NPC's name, and an address into a
	// quest's text: no rule covers those columns, the search finds them
	if _, err := db.Exec(`UPDATE npcs SET npc_name = 'Guard ' || ? WHERE id = 500`, planted["accountinfo.email"]); err != nil {
		t.Fatal(err)
	}
	msg := refused(t, db)
	if !strings.Contains(msg, "accountinfo.email is still in npcs.npc_name") {
		t.Errorf("refused, but not naming where: %s", msg)
	}
	if strings.Contains(msg, planted["accountinfo.email"]) {
		t.Error("the refusal repeats the secret it found")
	}
}

func TestASnapshotNeverOverwritesAFile(t *testing.T) {
	db, _ := seededDB(t)
	out := filepath.Join(t.TempDir(), "snap.db")
	os.WriteFile(out, []byte("someone's file"), 0o600)
	if _, err := db.WriteSnapshot(out, "keeper"); err == nil {
		t.Fatal("a snapshot overwrote a file")
	}
	if string(readFile(t, out)) != "someone's file" {
		t.Fatal("the file was changed")
	}
}

func TestASnapshotSearchesOnlyForWhatNamesSomeone(t *testing.T) {
	for v, want := range map[string]bool{
		"203.0.113.77": true, "203.0.113.77:5555": true, "2001:db8::77": true, "planted.person@example.org": true,
		"127.0.0.1": false, "::1": false, "10.0.0.1": false, "172.16.0.20": false, "fe80::1": false, "short": false, "": false,
	} {
		if got := worthScanning([]byte(v)); got != want {
			t.Errorf("worthScanning(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestTheSecretMatcherFindsAnyOfMany(t *testing.T) {
	pats := [][]byte{[]byte("hunter2-secret"), []byte("203.0.113.77"), []byte("zzzzzzzzzz")}
	m := newSecretMatcher(pats)
	for cell, want := range map[string]int{
		"nothing here": -1, "a hunter2-secret inside": 0, "from 203.0.113.77 today": 1, "203.0.113.7": -1,
		"zzzzzzzzz": -1, "yzzzzzzzzzz": 2, "": -1, "203.0.113.77": 1,
	} {
		if got := m.find([]byte(cell)); got != want {
			t.Errorf("find(%q) = %d, want %d", cell, got, want)
		}
	}
	if newSecretMatcher(nil) != nil {
		t.Error("a matcher of nothing")
	}
}
