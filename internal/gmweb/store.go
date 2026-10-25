package gmweb

import (
	"time"

	"github.com/eqoa/server/internal/database"
)

// The panel's own tables, in the game's database: the audit log (its
// statement the database package's: a GM command in the game writes it too),
// the GMs' authenticators, their API and OAuth tokens and the OAuth clients.
// Times are TEXT in RFC 3339 (UTC), which every reader of the file can sort
// and parse.
const panelSchema = database.GMAuditSchema + `
CREATE TABLE IF NOT EXISTS gm_totp (
	accountid INTEGER PRIMARY KEY,
	secret    TEXT NOT NULL,
	enabled   TEXT NOT NULL,
	last_step INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS gm_tokens (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	accountid INTEGER NOT NULL,
	name      TEXT NOT NULL,
	kind      TEXT NOT NULL,
	hash      TEXT NOT NULL UNIQUE,
	prefix    TEXT NOT NULL,
	scope     TEXT NOT NULL,
	client    TEXT NOT NULL DEFAULT '',
	parent    INTEGER,
	created   TEXT NOT NULL,
	expires   TEXT,
	last_used TEXT,
	last_ip   TEXT NOT NULL DEFAULT '',
	revoked   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_gm_tokens_acct ON gm_tokens(accountid);

CREATE TABLE IF NOT EXISTS gm_oauth_clients (
	client_id     TEXT PRIMARY KEY,
	name          TEXT NOT NULL,
	redirect_uris TEXT NOT NULL,
	created       TEXT NOT NULL,
	last_used     TEXT
);
`

func (s *Server) initSchema() error {
	_, err := s.d.DB.Exec(panelSchema)
	return err
}

// stamp is a time as the panel's tables keep it.
func stamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func now() string { return stamp(time.Now()) }

// parseStamp reads a stamp back; the zero time for anything else.
func parseStamp(v string) time.Time {
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}
	}
	return t
}
