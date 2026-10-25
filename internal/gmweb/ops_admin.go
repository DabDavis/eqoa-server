package gmweb

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/handlers"
)

// The audit log and its reverts; the server's own settings; raw SQL; database
// backups; the server's log; a restart.

func (s *Server) registerAdminOps() {
	s.addOp(&Op{
		Name: "audit_log", Title: "The audit log", Scope: scopeRead, Core: true,
		Description: "Every change made through the panel, by an agent or by a GM command in the game, newest first: who, " +
			"through what (web, a token, OAuth, game), when, and for a row the row before and after. Filter by GM, table, " +
			"action (insert, update, delete, live, account, settings, sql, login, token, security) or text.",
		Input: obj(map[string]any{
			"username": str("only this GM's"),
			"table":    str("only changes to this table"),
			"action":   str("only this action"),
			"key":      freeObject("only changes to this row (its key)"),
			"search":   str("text in the entry"),
			"limit":    integer("default 50, at most 500"),
			"offset":   integer("entries to skip"),
		}),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				auditFilter
				Key map[string]any `json:"key"` // the row's key; shadows auditFilter's text form
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			f := req.auditFilter
			if req.Key != nil {
				f.Key = keyText(req.Key)
			}
			f.hideSQL = !c.a.Admin
			list, total, err := c.s.auditList(f)
			if err != nil {
				return nil, err
			}
			return map[string]any{"entries": list, "total": total}, nil
		},
	})
	s.addOp(&Op{
		Name: "revert_change", Title: "Revert a change", Scope: scopeWrite, Destructive: true, Core: true,
		Description: "Undo a row change from the audit log by its id: an added row is deleted, a deleted row put " +
			"back, a changed row's columns set back. Refused when the row has changed since (force overrides). The " +
			"revert is itself logged and can be reverted. dry_run shows it and changes nothing.",
		Input: obj(map[string]any{
			"id":      integer("the audit entry"),
			"force":   boolean("revert even if the row changed since"),
			"dry_run": boolean("show, change nothing"),
		}, "id"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				ID     int64
				Force  bool
				DryRun bool `json:"dry_run"`
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.revert(c.a, req.ID, req.Force, req.DryRun)
		},
	})
	s.addOp(&Op{
		Name: "get_settings", Title: "Server settings", Scope: scopeRead,
		Description: "The settings a GM panel can change while the server runs (npc_text: what NPCs say by name; " +
			"start_level and start_all_spells for new characters; allow_auto_register: whether an unknown name " +
			"registers itself at login), and the rest of the configuration, read-only.",
		Handler: func(c *opCtx) (any, error) { return c.s.settingsView(), nil },
	})
	s.addOp(&Op{
		Name: "update_settings", Title: "Change server settings", Scope: scopeAdmin,
		Description: "Change the live settings: they apply at once and are written to config.yaml (backed up " +
			"first), so a restart keeps them. npc_text replaces the whole map: read it with get_settings first.",
		Input: obj(map[string]any{
			"npc_text":            freeObject("NPC name -> what it says when talked to (the whole map)"),
			"start_level":         integer("a new character's level, 1-60"),
			"start_all_spells":    boolean("a new character gets every spell of its class"),
			"allow_auto_register": boolean("an unknown name registers itself at login"),
		}),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				NPCText           map[string]string `json:"npc_text"`
				StartLevel        *int              `json:"start_level"`
				StartAllSpells    *bool             `json:"start_all_spells"`
				AllowAutoRegister *bool             `json:"allow_auto_register"`
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.updateSettings(c.a, req.NPCText, req.StartLevel, req.StartAllSpells, req.AllowAutoRegister)
		},
	})
	s.addOp(&Op{
		Name: "sql_query", Title: "Run a read-only SQL query", Scope: scopeAdmin,
		Description: "Run one SELECT (or WITH, PRAGMA, EXPLAIN) on the database, read-only, at most 5 s and 1000 " +
			"rows: for questions the other operations do not answer. An admin only.",
		Input: obj(map[string]any{"sql": str("one read-only statement"), "limit": integer("rows, default 200, at most 1000")}, "sql"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				SQL   string
				Limit int
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.sqlQuery(c, req.SQL, req.Limit)
		},
	})
	s.addOp(&Op{
		Name: "sql_execute", Title: "Run SQL that changes things", Scope: scopeAdmin, Destructive: true,
		Description: "Run statements that change the database (UPDATE, INSERT, DELETE, ALTER...), in one transaction, " +
			"after a backup of the whole database. The audit log keeps the statements but not the rows, so revert " +
			"is by the backup. Prefer the row operations, which are revertible. NPCs changed this way reach the world " +
			"with reload_npcs. An admin only.",
		Input: obj(map[string]any{
			"sql":    str("the statements"),
			"backup": boolean("back the database up first (default true)"),
		}, "sql"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				SQL    string
				Backup *bool
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			backup := req.Backup == nil || *req.Backup
			return c.s.sqlExecute(c, req.SQL, backup)
		},
	})
	s.addOp(&Op{
		Name: "create_backup", Title: "Back the database up", Scope: scopeAdmin,
		Description: "Write a consistent copy of the whole database (SQLite VACUUM INTO) to the backup directory.",
		Input:       obj(map[string]any{"label": str("a word for the file name")}),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ Label string }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			path, size, err := c.s.backup(req.Label)
			if err != nil {
				return nil, err
			}
			c.s.audit(c.a, "backup", "", nil, nil, nil, "backed the database up to "+path)
			return map[string]any{"path": path, "bytes": size}, nil
		},
	})
	s.addOp(&Op{
		Name: "list_backups", Title: "Database backups", Scope: scopeAdmin,
		Description: "The database backups the panel made, and the older eqoa.db.bak-* copies beside the database.",
		Handler:     func(c *opCtx) (any, error) { return c.s.listBackups() },
	})
	s.addOp(&Op{
		Name: "server_log", Title: "The server's log", Scope: scopeRead,
		Description: "The end of the game server's log: logins, chat commands, errors. filter keeps the lines " +
			"containing it (any case).",
		Input: obj(map[string]any{
			"lines":  integer("how many lines, default 200, at most 2000"),
			"filter": str("only lines containing this"),
		}),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Lines  int
				Filter string
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.tailLog(req.Lines, req.Filter)
		},
	})
	s.addOp(&Op{
		Name: "restart_server", Title: "Restart the server", Scope: scopeAdmin, Destructive: true,
		Description: "Save every player as at a logout and start the server again (about ten seconds; everyone " +
			"in the game is disconnected and logs in again). confirm must be \"restart\".",
		Input: obj(map[string]any{"confirm": str("the word restart")}, "confirm"),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ Confirm string }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			if req.Confirm != "restart" {
				return nil, badRequest(`confirm must be "restart"`)
			}
			if c.s.d.Restart == nil {
				return nil, badRequest("this server cannot restart itself")
			}
			c.s.audit(c.a, "server", "", nil, nil, nil, "restarted the server")
			go func() {
				time.Sleep(500 * time.Millisecond) // the answer goes out first
				if err := c.s.d.Restart(); err != nil {
					c.s.audit(c.a, "server", "", nil, nil, nil, "the restart failed: "+err.Error())
				}
			}()
			return map[string]any{"restarting": true}, nil
		},
	})
}

// settingsNow is the live settings' values.
func (s *Server) settingsNow() map[string]any {
	rules := database.NewCharacterRules{}
	if s.d.Characters != nil {
		rules = s.d.Characters.NewCharacterRules()
	}
	auto := s.d.Config.Auth.AllowAutoRegister
	if s.d.AutoRegister != nil {
		auto = s.d.AutoRegister()
	}
	return map[string]any{
		"npc_text":            handlers.NPCText(),
		"start_level":         max(1, rules.Level),
		"start_all_spells":    rules.AllSpells,
		"allow_auto_register": auto,
	}
}

// settingsView is the live settings and the rest of the configuration,
// without a password.
func (s *Server) settingsView() map[string]any {
	cfg := *s.d.Config
	cfg.Auth.Accounts = nil
	var seeds []string
	for _, a := range s.d.Config.Auth.Accounts {
		seeds = append(seeds, a.Username)
	}
	return map[string]any{
		"live":       s.settingsNow(),
		"configFile": s.d.ConfigPath,
		"config": map[string]any{
			"server": cfg.Server, "world": map[string]any{
				"tick_rate": cfg.World.TickRate.String(), "session_timeout_pre": cfg.World.SessionTimeoutPre.String(),
				"session_timeout_game": cfg.World.SessionTimeoutGame.String(),
			},
			"patch": cfg.Patch,
			"auth": map[string]any{
				"bcrypt_cost": cfg.Auth.BCryptCost, "require_tcp_login": cfg.Auth.RequireTCPLogin,
				"ticket_ttl": cfg.Auth.TicketTTL.String(), "one_login": cfg.Auth.OneLogin,
				"one_login_exempt": cfg.Auth.OneLoginExempt, "relay_lookup": cfg.Auth.RelayLookup,
				"seeded_accounts": seeds,
			},
			"gm": map[string]any{
				"port": s.cfg.Port, "bind": s.cfg.Bind, "public_url": s.cfg.PublicURL,
				"min_level": s.cfg.MinLevel, "admin_level": s.cfg.AdminLevel, "require_totp": s.cfg.RequireTOTP,
				"session_idle": s.cfg.SessionIdle.String(), "token_rate": s.cfg.TokenRate,
			},
		},
	}
}

func (s *Server) updateSettings(a *actor, npcText map[string]string, startLevel *int, allSpells, autoReg *bool) (any, error) {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	before := s.settingsNow()
	changes := map[string]any{}
	if npcText != nil {
		clean := map[string]string{}
		for k, v := range npcText {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			if len([]rune(v)) > 1000 {
				return nil, badRequest("what %q says is over 1000 characters", k)
			}
			clean[k] = v
		}
		changes["world.npc_text"] = clean
	}
	rules := s.d.Characters.NewCharacterRules()
	rulesChanged := false
	if startLevel != nil {
		if *startLevel < 1 || *startLevel > 60 {
			return nil, badRequest("start_level is 1 to 60")
		}
		changes["world.start_level"] = *startLevel
		rules.Level = *startLevel
		rulesChanged = true
	}
	if allSpells != nil {
		changes["world.start_all_spells"] = *allSpells
		rules.AllSpells = *allSpells
		rulesChanged = true
	}
	if autoReg != nil {
		changes["auth.allow_auto_register"] = *autoReg
	}
	if len(changes) == 0 {
		return nil, badRequest("nothing to change")
	}
	backup := ""
	if s.d.ConfigPath != "" {
		b, err := config.SetInFile(s.d.ConfigPath, changes)
		if err != nil {
			return nil, fmt.Errorf("config.yaml was not changed, and neither was anything else: %w", err)
		}
		backup = b
	}
	if v, ok := changes["world.npc_text"].(map[string]string); ok {
		handlers.SetNPCText(v)
		s.d.Config.World.NPCText = v
	}
	if rulesChanged && s.d.ApplyNewCharacterRules != nil {
		s.d.ApplyNewCharacterRules(rules)
		s.d.Config.World.StartLevel, s.d.Config.World.StartAllSpells = rules.Level, rules.AllSpells
	}
	if autoReg != nil && s.d.SetAutoRegister != nil {
		s.d.SetAutoRegister(*autoReg)
		s.d.Config.Auth.AllowAutoRegister = *autoReg
	}
	after := s.settingsNow()
	s.audit(a, "settings", "", nil, before, after, "changed server settings")
	return map[string]any{"settings": after, "configBackup": backup}, nil
}

// readOnlySQL is the statements sql_query runs.
var readOnlySQL = regexp.MustCompile(`(?is)^\s*(select|with|pragma|explain|values)\b`)

// readPragma is the PRAGMAs sql_query runs: the ones that only read, named
// with at most a table or index for an argument. A PRAGMA that sets
// something would set it on the game's own connection (there is one) for good.
var readPragma = regexp.MustCompile(`(?is)^\s*pragma\s+(table_info|table_xinfo|table_list|index_list|index_info|index_xinfo|` +
	`foreign_key_list|foreign_key_check|integrity_check|quick_check|database_list|collation_list|compile_options|` +
	`function_list|pragma_list|page_count|page_size|freelist_count|user_version|schema_version|application_id|` +
	`journal_mode|foreign_keys|encoding)\s*(\(\s*['"\w.]*\s*\))?\s*$`)

func (s *Server) sqlQuery(c *opCtx, query string, limit int) (any, error) {
	q := strings.TrimSpace(query)
	q = strings.TrimSuffix(q, ";")
	if q == "" {
		return nil, badRequest("no SQL")
	}
	if strings.Contains(q, ";") {
		return nil, badRequest("one statement at a time")
	}
	if !readOnlySQL.MatchString(q) {
		return nil, badRequest("sql_query runs SELECT, WITH, PRAGMA, EXPLAIN or VALUES; changes go through sql_execute")
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(q)), "pragma") && !readPragma.MatchString(q) {
		return nil, badRequest("sql_query runs only the PRAGMAs that read (table_info(x), index_list(x), integrity_check...); " +
			"one that sets something would set it on the game's connection")
	}
	if limit <= 0 {
		limit = 200
	}
	limit = min(limit, 1000)
	start := time.Now()
	cols, out, more, err := s.readOnlyQuery(q, limit)
	if err != nil {
		return nil, err
	}
	// the audit line only once the connection is let go: the database has one
	s.audit(c.a, "sql", "", nil, nil, nil, "query: "+clip(q, 500))
	return map[string]any{"columns": cols, "rows": out, "more": more, "ms": time.Since(start).Milliseconds()}, nil
}

// readOnlyQuery runs one statement on a connection of its own held with
// query_only on, so nothing it does can write; the game waits for it (the
// database has one connection), at most 5 s.
func (s *Server) readOnlyQuery(q string, limit int) ([]string, [][]any, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := s.d.DB.Conn(ctx)
	if err != nil {
		return nil, nil, false, err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `PRAGMA query_only = 1`); err != nil {
		return nil, nil, false, err
	}
	defer conn.ExecContext(context.Background(), `PRAGMA query_only = 0`)
	rs, err := conn.QueryContext(ctx, q)
	if err != nil {
		return nil, nil, false, badRequest("%v", err)
	}
	defer rs.Close()
	cols, _ := rs.Columns()
	out := [][]any{}
	more := false
	for rs.Next() {
		if len(out) == limit {
			more = true
			break
		}
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rs.Scan(ptrs...); err != nil {
			return nil, nil, false, err
		}
		for i := range vals {
			vals[i] = fromSQL(vals[i])
		}
		out = append(out, vals)
	}
	if err := rs.Err(); err != nil {
		return nil, nil, false, badRequest("%v", err)
	}
	return cols, out, more, nil
}

func (s *Server) sqlExecute(c *opCtx, stmts string, backup bool) (any, error) {
	q := strings.TrimSpace(stmts)
	if q == "" {
		return nil, badRequest("no SQL")
	}
	backupPath := ""
	if backup {
		p, _, err := s.backup("before-sql")
		if err != nil {
			return nil, fmt.Errorf("the backup failed, so nothing was run: %w", err)
		}
		backupPath = p
		s.pruneBackups("before-sql", keepSQLBackups)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	tx, err := s.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, q)
	if err != nil {
		return nil, badRequest("%v (nothing was changed)", err)
	}
	n, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	s.schema.invalidate()
	database.InvalidateDefaultCharacters()
	s.audit(c.a, "sql", "", nil, nil, nil, fmt.Sprintf("execute (%d rows; backup %s): %s", n, backupPath, clip(q, 2000)))
	return map[string]any{"rowsAffected": n, "backup": backupPath,
		"note": "NPCs changed this way reach the running world with reload_npcs"}, nil
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// backupDir is where backups go: gm.backup_dir, beside the database when it
// is relative.
func (s *Server) backupDir() string {
	d := s.cfg.BackupDir
	if !filepath.IsAbs(d) {
		d = filepath.Join(filepath.Dir(s.d.Config.Server.DBPath), d)
	}
	return d
}

var labelChars = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// backup writes a consistent copy of the database and returns its path.
func (s *Server) backup(label string) (string, int64, error) {
	dir := s.backupDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, err
	}
	label = strings.Trim(labelChars.ReplaceAllString(label, "-"), "-")
	if len(label) > 40 {
		label = label[:40]
	}
	name := "eqoa-" + time.Now().UTC().Format("20060102-150405")
	if label != "" {
		name += "-" + label
	}
	path := filepath.Join(dir, name+".db")
	if _, err := os.Stat(path); err == nil {
		path = filepath.Join(dir, name+"-"+randomHex(3)+".db")
	}
	if _, err := s.d.DB.Exec(`VACUUM INTO ?`, path); err != nil {
		return "", 0, err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return path, 0, err
	}
	return path, fi.Size(), nil
}

// keepSQLBackups is how many of sql_execute's own backups are kept: each is
// the whole database.
const keepSQLBackups = 20

// pruneBackups deletes all but the newest keep backups with that label.
func (s *Server) pruneBackups(label string, keep int) {
	matches, _ := filepath.Glob(filepath.Join(s.backupDir(), "eqoa-*-"+label+"*.db"))
	sort.Strings(matches) // the names start with their time
	for i := 0; i < len(matches)-keep; i++ {
		os.Remove(matches[i])
	}
}

func (s *Server) listBackups() (any, error) {
	type file struct {
		Path  string `json:"path"`
		Bytes int64  `json:"bytes"`
		At    string `json:"at"`
	}
	var out []file
	add := func(pattern string) {
		matches, _ := filepath.Glob(pattern)
		for _, m := range matches {
			if fi, err := os.Stat(m); err == nil && !fi.IsDir() {
				out = append(out, file{m, fi.Size(), stamp(fi.ModTime())})
			}
		}
	}
	add(filepath.Join(s.backupDir(), "*.db"))
	add(s.d.Config.Server.DBPath + ".bak*")
	sort.Slice(out, func(i, j int) bool { return out[i].At > out[j].At })
	if out == nil {
		out = []file{}
	}
	return map[string]any{"backups": out, "dir": s.backupDir()}, nil
}

// tailLog is the end of the server's log.
func (s *Server) tailLog(lines int, filter string) (any, error) {
	if lines <= 0 {
		lines = 200
	}
	lines = min(lines, 2000)
	f, err := os.Open(s.cfg.LogPath)
	if err != nil {
		return nil, badRequest("the log %s cannot be read: %v", s.cfg.LogPath, err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	// read back from the end: 2 MB, 8 MB when filtering
	window := int64(2 << 20)
	if filter != "" {
		window = 8 << 20
	}
	start := max(0, fi.Size()-window)
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}
	buf, err := io.ReadAll(io.LimitReader(f, window))
	if err != nil {
		return nil, err
	}
	if start > 0 {
		if i := bytes.IndexByte(buf, '\n'); i >= 0 {
			buf = buf[i+1:]
		}
	}
	all := strings.Split(strings.TrimRight(string(buf), "\n"), "\n")
	want := strings.ToLower(filter)
	var out []string
	for i := len(all) - 1; i >= 0 && len(out) < lines; i-- {
		if want == "" || strings.Contains(strings.ToLower(all[i]), want) {
			out = append(out, all[i])
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	if out == nil {
		out = []string{}
	}
	return map[string]any{"lines": out, "file": s.cfg.LogPath, "bytes": fi.Size()}, nil
}
