package database

import (
	"bytes"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A snapshot of a real server's database (EQOA_REAL_DB names it, a copy of
// which is made from): no value of what the rules take out survives anywhere
// in the file, searched for here byte by byte and apart from the sanitiser's
// own search (each value found in the copy first: the control); every table
// the rules keep has every row it had; and it opens as a server database.
// Skipped without a database: none is in this repository.
func TestRealSnapshotOfAServer(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "real.db")
	copyFile(t, src, path)
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// what must not survive, read here as the rules describe it
	type secret struct{ from, value string }
	var secrets []secret
	add := func(from, q string) {
		rows, err := db.Query(q)
		if err != nil {
			if strings.Contains(err.Error(), "no such") {
				return // an older database without the table
			}
			t.Fatalf("%s: %v", q, err)
		}
		defer rows.Close()
		for rows.Next() {
			var v sql.NullString
			rows.Scan(&v)
			if v.Valid && worthScanning([]byte(v.String)) {
				secrets = append(secrets, secret{from, v.String})
			}
		}
	}
	for _, col := range []string{"pwhash", "email", "ipaddress", "firstname", "lastname", "zip", "birthday"} {
		add("accountinfo."+col, `SELECT CAST(`+col+` AS TEXT) FROM accountinfo`)
	}
	add("gm_tokens.hash", `SELECT hash FROM gm_tokens`)
	add("gm_tokens.last_ip", `SELECT last_ip FROM gm_tokens`)
	add("gm_totp.secret", `SELECT secret FROM gm_totp`)
	add("gm_oauth_clients.client_id", `SELECT client_id FROM gm_oauth_clients`)
	add("gm_oauth_clients.redirect_uris", `SELECT redirect_uris FROM gm_oauth_clients`)
	add("gm_audit.ip", `SELECT DISTINCT ip FROM gm_audit`)
	add("spellfx_meta.salt", `SELECT value FROM spellfx_meta WHERE key = 'salt'`)
	add("mapmark_meta.salt", `SELECT value FROM mapmark_meta WHERE key = 'salt'`)
	add("spellfx_suggestions.addr_hash", `SELECT DISTINCT addr_hash FROM spellfx_suggestions`)
	add("mapmark_suggestions.sender_hash", `SELECT DISTINCT sender_hash FROM mapmark_suggestions`)
	before := readFile(t, path)
	for _, s := range secrets {
		if !bytes.Contains(before, []byte(s.value)) {
			t.Fatalf("control: a value of %s is not in the copy's file", s.from)
		}
	}
	counts := tableCounts(t, db.DB)

	out := filepath.Join(dir, "snapshot.db")
	rep, err := db.WriteSnapshot(out, "the real-data test")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d tables, %d bytes, %d values searched for by the sanitiser, %d here", rep.Tables, rep.Bytes, rep.Scanned, len(secrets))
	for _, st := range rep.Steps {
		t.Logf("  %s: %s", st.Table, st.Did)
	}
	got := readFile(t, out)
	leaked := map[string]int{}
	for _, s := range secrets {
		if bytes.Contains(got, []byte(s.value)) {
			leaked[s.from]++
		}
	}
	for from, n := range leaked {
		t.Errorf("%d values of %s survived the snapshot", n, from)
	}

	// the file as it is handed out (a server opening it seeds the board's
	// overview again)
	plain, err := sql.Open("sqlite", out)
	if err != nil {
		t.Fatal(err)
	}
	after := tableCounts(t, plain)
	plain.Close()
	for table, n := range counts {
		rule := snapshotRules[table]
		switch {
		case rule.clear && after[table] != 0:
			t.Errorf("%s kept %d rows", table, after[table])
		case !rule.clear && after[table] != n:
			t.Errorf("%s: %d rows, %d in the snapshot", table, n, after[table])
		}
	}
	snap, err := NewSQLite(out)
	if err != nil {
		t.Fatalf("the snapshot does not open as a server database: %v", err)
	}
	defer snap.Close()
	var players int
	if err := snap.QueryRow(`SELECT COUNT(*) FROM accountinfo WHERE COALESCE(acctlevel, 0) <= 0
		AND username IS NOT 'account-' || accountid`).Scan(&players); err != nil || players != 0 {
		t.Errorf("%d players kept their login names (%v)", players, err)
	}
	npcs, err := NewNPCStore(snap).GetAllNPCs()
	if err != nil || len(npcs) != counts["npcs"] {
		t.Errorf("the NPC store read %d NPCs of %d (%v)", len(npcs), counts["npcs"], err)
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	in, err := os.Open(from)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

// tableCounts is every table's rows.
func tableCounts(t *testing.T, db *sql.DB) map[string]int {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for rows.Next() {
		var n string
		rows.Scan(&n)
		names = append(names, n)
	}
	rows.Close()
	out := map[string]int{}
	for _, n := range names {
		var c int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + quoteSnap(n)).Scan(&c); err != nil {
			t.Fatal(err)
		}
		out[n] = c
	}
	return out
}
