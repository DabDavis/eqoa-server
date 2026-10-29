// workboard is the work board on the box: who is working on what on this
// server (the GM panel's Work board page, and the work_board / post_work /
// update_work_overview operations jib's agents call), read and written in the
// server's database beside the running server, as dab's sessions unless -as
// names someone else. Every change is written to gm_audit as the panel writes
// one, so the panel's audit log lists it and revert_change undoes it. It opens
// the database for the board alone (database.OpenWorkBoard): the board's
// tables and the overview are made when missing, nothing else is touched.
//
//	workboard list                     the board: in progress, blocked, planned, done lately
//	workboard add -title "..." -touches "npcs, npcpaths" -status in_progress
//	workboard note 12 "half moved"
//	workboard done 12 -result "18 guards moved"
//	workboard overview                 the overview and its version
//	workboard overview set -version 4 -file overview.md
//
// Run it where config.yaml is (the database is its server.db_path), or give
// -db. `workboard help` says it all.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
)

const usage = `workboard: the work board (the GM panel's Work board page) on the box.

Who is working on what on this server: the GMs, their AI agents and dab's
sessions. Read it before you change anything on eqoaclassic, post what you
are about to do with the tables and files it touches, keep its status
current, and mark it done with a one-line result. Never change what another
entry has in progress without asking the person.

Usage: workboard [flags] <command> [arguments]

Commands:
  init                              make the board's tables and seed the overview (any command does)
  list [-status s] [-area a] [-owner o] [-touches t] [-search q] [-all]
                                    the board; -all: the done ones older than 14 days too
  show <id>                         one entry whole, with its history and what it overlaps
  add -title t [-area a] [-touches t] [-status s] [-owner o] [-details text | -details-file f] [-note text]
                                    post a new entry (status planned unless given)
  update <id> [-title] [-area] [-touches] [-status] [-owner] [-details | -details-file] [-note] [-result] [-version n]
  start <id> [-note text]           in progress
  block <id> -note why              blocked, saying on what
  done <id> -result "one line" [-note text]
  note <id> text...                 a note, with the time and who
  overview [show]                   the overview's text and its version
  overview set -version n [-file f | -text t]
                                    replace it (stdin when neither); refused if it changed since version n
  overview default                  the text a new board starts with

Statuses: in_progress, blocked, planned, done. Areas: code, data, quests, client, gm_tools, other.
-touches is comma separated: table names (npcs, queststeps), paths (internal/gmweb/ops.go) or folders.

Flags (before or after the command):
`

// exit codes
const (
	exitOK       = 0
	exitFail     = 1
	exitUsage    = 2
	exitConflict = 3 // a stale version: nothing written
)

// opt is a flag that says whether it was given.
type opt struct {
	v   string
	set bool
}

func (o *opt) String() string     { return o.v }
func (o *opt) Set(v string) error { o.v, o.set = v, true; return nil }
func (o *opt) ptr() *string {
	if !o.set {
		return nil
	}
	v := o.v
	return &v
}

// cli is one run.
type cli struct {
	stdin          io.Reader
	stdout, stderr io.Writer
	db, cfg, as    string
	asJSON         bool
	now            time.Time
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// globals adds the flags every command takes to fs.
func (c *cli) globals(fs *flag.FlagSet) {
	fs.StringVar(&c.db, "db", c.db, "the database (default: config.yaml's server.db_path, else eqoa.db)")
	fs.StringVar(&c.cfg, "config", c.cfg, "the server's config.yaml, for its database")
	fs.StringVar(&c.as, "as", c.as, "who you are on the board: the owner of what you post and the name its changes are logged under")
	fs.BoolVar(&c.asJSON, "json", c.asJSON, "print JSON (list, show, overview)")
}

func (c *cli) flags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet("workboard "+name, flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	c.globals(fs)
	return fs
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	c := &cli{stdin: stdin, stdout: stdout, stderr: stderr, cfg: "config.yaml", as: "dab's sessions", now: time.Now()}
	top := flag.NewFlagSet("workboard", flag.ContinueOnError)
	top.SetOutput(stderr)
	c.globals(top)
	top.Usage = func() {
		fmt.Fprint(stderr, usage)
		top.PrintDefaults()
	}
	if err := top.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	rest := top.Args()
	if len(rest) == 0 || rest[0] == "help" || rest[0] == "-h" {
		top.Usage()
		if len(rest) == 0 {
			return exitUsage
		}
		return exitOK
	}
	cmd, rest := rest[0], rest[1:]
	if cmd == "overview" && len(rest) > 0 && rest[0] == "default" {
		fmt.Fprint(stdout, database.DefaultWorkOverview)
		return exitOK
	}
	var err error
	switch cmd {
	case "init":
		err = c.withDB(rest, "init", nil, c.initBoard)
	case "list", "ls":
		err = c.list(rest)
	case "show":
		err = c.show(rest)
	case "add", "post":
		err = c.add(rest)
	case "update", "set":
		err = c.update(rest, "update", nil)
	case "start":
		st := database.WorkInProgress
		err = c.update(rest, "start", &st)
	case "block":
		st := database.WorkBlocked
		err = c.update(rest, "block", &st)
	case "done":
		st := database.WorkDone
		err = c.update(rest, "done", &st)
	case "note":
		err = c.note(rest)
	case "overview":
		err = c.overview(rest)
	default:
		fmt.Fprintf(stderr, "workboard: no command %q (workboard help lists them)\n", cmd)
		return exitUsage
	}
	var conflict *database.WorkConflict
	var ue usageError
	switch {
	case err == nil:
		return exitOK
	case errors.As(err, &conflict):
		fmt.Fprintf(stderr, "workboard: %v\n", err)
		return exitConflict
	case errors.As(err, &ue):
		fmt.Fprintf(stderr, "workboard: %v\n", err)
		return exitUsage
	case errors.Is(err, flag.ErrHelp):
		return exitOK
	}
	fmt.Fprintf(stderr, "workboard: %v\n", err)
	return exitFail
}

type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

func usagef(format string, a ...any) error { return usageError{fmt.Sprintf(format, a...)} }

// who is the name changes are logged under.
func (c *cli) who() database.GMEdit {
	return database.GMEdit{Username: strings.TrimSpace(c.as), Via: "cli", IP: "local"}
}

// path is the database: -db, else config.yaml's server.db_path (beside the
// config when relative), else eqoa.db.
func (c *cli) path() (string, error) {
	if c.db != "" {
		return c.db, nil
	}
	if _, err := os.Stat(c.cfg); err != nil {
		return "eqoa.db", nil
	}
	cfg, err := config.LoadOrDefault(c.cfg)
	if err != nil {
		return "", fmt.Errorf("%s: %w", c.cfg, err)
	}
	p := cfg.Server.DBPath
	if p == "" {
		p = "eqoa.db"
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(filepath.Dir(c.cfg), p)
	}
	return p, nil
}

func (c *cli) open() (*database.SQLiteDB, error) {
	p, err := c.path()
	if err != nil {
		return nil, err
	}
	return database.OpenWorkBoard(p)
}

// withDB parses a command's flags (fs may be nil: none of its own) and runs
// fn on the opened database with what is left of the arguments.
func (c *cli) withDB(args []string, name string, fs *flag.FlagSet, fn func(db *database.SQLiteDB, args []string) error) error {
	if fs == nil {
		fs = c.flags(name)
	}
	pos, err := parseMixed(fs, args)
	if err != nil {
		return err
	}
	db, err := c.open()
	if err != nil {
		return err
	}
	defer db.Close()
	return fn(db, pos)
}

// parseMixed parses flags and positional arguments in any order ("done 12
// -result x" and "done -result x 12").
func parseMixed(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

func entryID(pos []string, what string) (int64, []string, error) {
	if len(pos) == 0 {
		return 0, nil, usagef("%s needs an entry's id (workboard list shows them)", what)
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(pos[0], "#"), 10, 64)
	if err != nil || id <= 0 {
		return 0, nil, usagef("%q is not an entry's id", pos[0])
	}
	return id, pos[1:], nil
}

func (c *cli) initBoard(db *database.SQLiteDB, _ []string) error {
	b, err := db.WorkBoard(database.WorkQuery{OldDone: true, Limit: 1})
	if err != nil {
		return err
	}
	fmt.Fprintf(c.stdout, "the work board is ready: %d entries; the overview at version %d (changed %s by %s)\n",
		b.Matching, b.Overview.Version, b.Overview.Updated, b.Overview.UpdatedBy)
	return nil
}

func (c *cli) printJSON(v any) error {
	enc := json.NewEncoder(c.stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// ago is a time as "3 h ago".
func (c *cli) ago(stamp string) string {
	t, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		return stamp
	}
	d := c.now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	}
	return fmt.Sprintf("%d d ago", int(d.Hours()/24))
}

var statusTitle = map[string]string{
	database.WorkInProgress: "IN PROGRESS", database.WorkBlocked: "BLOCKED", database.WorkPlanned: "PLANNED",
	database.WorkDone: "DONE",
}

func (c *cli) list(args []string) error {
	fs := c.flags("list")
	status := fs.String("status", "", "only these statuses, comma separated")
	area := fs.String("area", "", "only this area")
	owner := fs.String("owner", "", "only entries whose owner holds this")
	touches := fs.String("touches", "", "only entries touching any of these tables or files, comma separated")
	search := fs.String("search", "", "only entries holding this text")
	all := fs.Bool("all", false, "the done entries older than 14 days too")
	return c.withDB(args, "list", fs, func(db *database.SQLiteDB, _ []string) error {
		q := database.WorkQuery{Statuses: database.ParseWorkTouches(*status), Area: *area, Owner: *owner,
			Touches: database.ParseWorkTouches(*touches), Search: *search, OldDone: *all, Limit: 1000}
		b, err := db.WorkBoard(q)
		if err != nil {
			return usagef("%v", err)
		}
		if c.asJSON {
			return c.printJSON(b)
		}
		w := c.stdout
		fmt.Fprintf(w, "Work board: %d in progress, %d blocked, %d planned, %d done in the last %d days, %d done before\n",
			b.Counts[database.WorkInProgress], b.Counts[database.WorkBlocked], b.Counts[database.WorkPlanned],
			b.Counts[database.WorkDone], database.WorkDoneDays, b.OlderDone)
		fmt.Fprintf(w, "Overview: version %d, changed %s by %s (workboard overview)\n", b.Overview.Version,
			c.ago(b.Overview.Updated), b.Overview.UpdatedBy)
		last := ""
		for _, e := range b.Entries {
			if e.Status != last {
				last = e.Status
				title := statusTitle[e.Status]
				if e.Status == database.WorkDone && !*all {
					title = fmt.Sprintf("DONE IN THE LAST %d DAYS", database.WorkDoneDays)
				}
				fmt.Fprintf(w, "\n%s\n", title)
			}
			c.printBrief(e)
		}
		if len(b.Entries) == 0 {
			fmt.Fprintln(w, "\n(no entries)")
		}
		if b.OlderDone > 0 && !*all {
			fmt.Fprintf(w, "\n%d done before %s: workboard list -all\n", b.OlderDone, b.DoneSince[:10])
		}
		return nil
	})
}

func (c *cli) printBrief(e database.WorkEntry) {
	w := c.stdout
	fmt.Fprintf(w, "  #%d %s\n", e.ID, e.Title)
	parts := []string{e.Area}
	if e.Owner != "" {
		parts = append(parts, "owner "+e.Owner)
	}
	if len(e.Touches) > 0 {
		parts = append(parts, "touches "+strings.Join(e.Touches, ", "))
	}
	parts = append(parts, fmt.Sprintf("changed %s by %s", c.ago(e.Updated), e.UpdatedBy), fmt.Sprintf("v%d", e.Version))
	fmt.Fprintf(w, "      %s\n", strings.Join(parts, " · "))
	if e.Status == database.WorkDone && e.Result != "" {
		fmt.Fprintf(w, "      result: %s\n", e.Result)
	} else if note := lastNote(e.Notes); note != "" {
		fmt.Fprintf(w, "      last note %s\n", note)
	}
}

// lastNote is the last note's first line, cut to fit a line.
func lastNote(notes string) string {
	i := strings.LastIndex("\n"+notes, "\n- ")
	if i < 0 {
		return ""
	}
	line := strings.SplitN(strings.TrimPrefix(notes[i:], "- "), "\n", 2)[0]
	if r := []rune(line); len(r) > 140 {
		line = string(r[:140]) + "…"
	}
	return line
}

func (c *cli) show(args []string) error {
	return c.withDB(args, "show", nil, func(db *database.SQLiteDB, pos []string) error {
		id, _, err := entryID(pos, "show")
		if err != nil {
			return err
		}
		e, ok, err := db.WorkEntryByID(id)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("no work entry %d (workboard list shows them)", id)
		}
		hist, err := db.WorkHistory("workboard", id, 100)
		if err != nil {
			return err
		}
		var overlaps []database.WorkOverlap
		if e.Status != database.WorkDone {
			if overlaps, err = db.WorkOverlaps(id, e.Touches); err != nil {
				return err
			}
		}
		if c.asJSON {
			return c.printJSON(map[string]any{"entry": e, "history": hist, "overlaps": overlaps})
		}
		w := c.stdout
		fmt.Fprintf(w, "#%d %s\n", e.ID, e.Title)
		fmt.Fprintf(w, "status %s · area %s · owner %s · version %d\n", e.Status, e.Area, orNone(e.Owner), e.Version)
		fmt.Fprintf(w, "touches %s\n", orNone(strings.Join(e.Touches, ", ")))
		fmt.Fprintf(w, "posted %s by %s · changed %s by %s\n", e.Created, e.CreatedBy, e.Updated, e.UpdatedBy)
		if e.Status == database.WorkDone {
			fmt.Fprintf(w, "done %s: %s\n", e.DoneAt, e.Result)
		}
		if e.Details != "" {
			fmt.Fprintf(w, "\nDetails:\n%s\n", indent(e.Details))
		}
		if e.Notes != "" {
			fmt.Fprintf(w, "\nNotes:\n%s\n", indent(e.Notes))
		}
		c.printOverlaps(overlaps)
		if len(hist) > 0 {
			fmt.Fprintln(w, "\nHistory (gm_audit; the panel's revert_change undoes a line):")
			for _, h := range hist {
				what := h.Action
				if len(h.Changed) > 0 {
					what += " " + strings.Join(h.Changed, ", ")
				}
				if h.RevertedBy != 0 {
					what += fmt.Sprintf(" (reverted by audit %d)", h.RevertedBy)
				}
				fmt.Fprintf(w, "  audit %d  %s  %s  %s\n", h.Audit, h.At, h.By, what)
			}
		}
		return nil
	})
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func indent(s string) string { return "  " + strings.ReplaceAll(s, "\n", "\n  ") }

// printOverlaps says which open entries touch the same things, those that
// hold them (in progress or blocked) as a warning.
func (c *cli) printOverlaps(overlaps []database.WorkOverlap) {
	for _, o := range overlaps {
		mark := "overlap"
		if o.Taken() {
			mark = "WARNING"
		}
		fmt.Fprintf(c.stdout, "%s: entry #%d (%s, owner %s) touches %s too: %s\n", mark, o.ID, o.Status, orNone(o.Owner),
			strings.Join(o.Touches, ", "), o.Title)
	}
	for _, o := range overlaps {
		if o.Taken() {
			fmt.Fprintln(c.stdout, "Ask the person before changing what another entry has in progress.")
			return
		}
	}
}

// entryFlags are the flags add and update share.
type entryFlags struct {
	title, area, touches, status, owner, details, result, note opt
	detailsFile                                                string
	version                                                    int64
}

func (c *cli) entryFlagSet(name string, f *entryFlags, withVersion bool) *flag.FlagSet {
	fs := c.flags(name)
	fs.Var(&f.title, "title", "what is being done, one line")
	fs.Var(&f.area, "area", "code, data, quests, client, gm_tools or other")
	fs.Var(&f.touches, "touches", "the tables and files it changes, comma separated")
	fs.Var(&f.status, "status", "planned, in_progress, blocked or done")
	fs.Var(&f.owner, "owner", "whose it is (default: -as)")
	fs.Var(&f.details, "details", "what and why, markdown (replaces the details)")
	fs.StringVar(&f.detailsFile, "details-file", "", "the details from a file (- for stdin)")
	fs.Var(&f.result, "result", "one line: what came of it (done needs it)")
	fs.Var(&f.note, "note", "a note added with the time and who")
	if withVersion {
		fs.Int64Var(&f.version, "version", 0, "the entry's version as you read it (refused if it changed since)")
	}
	return fs
}

func (c *cli) post(db *database.SQLiteDB, f *entryFlags, id int64) error {
	p := database.WorkPost{ID: id, Title: f.title.ptr(), Area: f.area.ptr(), Touches: f.touches.ptr(),
		Status: f.status.ptr(), Owner: f.owner.ptr(), Details: f.details.ptr(), Result: f.result.ptr(),
		Note: f.note.v, Version: f.version}
	if f.detailsFile != "" {
		if f.details.set {
			return usagef("give -details or -details-file, not both")
		}
		var b []byte
		var err error
		if f.detailsFile == "-" {
			b, err = io.ReadAll(c.stdin)
		} else {
			b, err = os.ReadFile(f.detailsFile)
		}
		if err != nil {
			return err
		}
		d := string(b)
		p.Details = &d
	}
	e, audit, err := db.PostWork(p, c.who())
	if err != nil {
		var conflict *database.WorkConflict
		if errors.As(err, &conflict) {
			return err
		}
		return usagef("%v", err)
	}
	switch {
	case id == 0:
		fmt.Fprintf(c.stdout, "posted #%d: %s (%s, v%d; audit %d)\n", e.ID, e.Title, e.Status, e.Version, audit)
	case audit == 0:
		fmt.Fprintf(c.stdout, "#%d unchanged: %s (%s, v%d)\n", e.ID, e.Title, e.Status, e.Version)
	default:
		fmt.Fprintf(c.stdout, "#%d changed: %s (%s, v%d; audit %d)\n", e.ID, e.Title, e.Status, e.Version, audit)
	}
	if e.Status != database.WorkDone {
		overlaps, err := db.WorkOverlaps(e.ID, e.Touches)
		if err != nil {
			return err
		}
		c.printOverlaps(overlaps)
	}
	return nil
}

func (c *cli) add(args []string) error {
	f := &entryFlags{}
	fs := c.entryFlagSet("add", f, false)
	return c.withDB(args, "add", fs, func(db *database.SQLiteDB, pos []string) error {
		if len(pos) > 0 {
			return usagef("add takes flags only (-title \"...\"), not %q", strings.Join(pos, " "))
		}
		if !f.title.set {
			return usagef("add needs -title (one line saying what is being done)")
		}
		if !f.owner.set {
			f.owner.Set(c.as)
		}
		return c.post(db, f, 0)
	})
}

// update changes an entry; status, when given, is what the command sets
// (start, block, done).
func (c *cli) update(args []string, name string, status *string) error {
	f := &entryFlags{}
	fs := c.entryFlagSet(name, f, true)
	return c.withDB(args, name, fs, func(db *database.SQLiteDB, pos []string) error {
		id, rest, err := entryID(pos, name)
		if err != nil {
			return err
		}
		if len(rest) > 0 {
			return usagef("%s takes one id, then flags; a note is -note \"...\" (or workboard note %d ...)", name, id)
		}
		if status != nil {
			if f.status.set {
				return usagef("%s sets the status itself; use update for -status", name)
			}
			f.status.Set(*status)
			if *status == database.WorkBlocked && strings.TrimSpace(f.note.v) == "" {
				return usagef("block needs -note saying what it is waiting on")
			}
		}
		return c.post(db, f, id)
	})
}

func (c *cli) note(args []string) error {
	return c.withDB(args, "note", nil, func(db *database.SQLiteDB, pos []string) error {
		id, rest, err := entryID(pos, "note")
		if err != nil {
			return err
		}
		text := strings.Join(rest, " ")
		if text == "-" {
			b, err := io.ReadAll(c.stdin)
			if err != nil {
				return err
			}
			text = string(b)
		}
		if strings.TrimSpace(text) == "" {
			return usagef("note needs its text: workboard note %d \"what happened\" (- reads it from stdin)", id)
		}
		f := &entryFlags{note: opt{v: text, set: true}}
		return c.post(db, f, id)
	})
}

func (c *cli) overview(args []string) error {
	sub := "show"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}
	switch sub {
	case "show":
		return c.withDB(args, "overview", nil, func(db *database.SQLiteDB, pos []string) error {
			if len(pos) > 0 {
				return usagef("overview show takes no arguments")
			}
			ov, err := db.WorkBoardOverview()
			if err != nil {
				return err
			}
			if c.asJSON {
				return c.printJSON(ov)
			}
			fmt.Fprintln(c.stdout, ov.Text)
			fmt.Fprintf(c.stdout, "\n-- version %d, changed %s by %s; edit: workboard overview set -version %d -file <file>\n",
				ov.Version, ov.Updated, ov.UpdatedBy, ov.Version)
			return nil
		})
	case "set":
		fs := c.flags("overview set")
		version := fs.Int64("version", -1, "the overview's version as you read it (workboard overview shows it)")
		file := fs.String("file", "", "the new text from a file")
		text := fs.String("text", "", "the new text")
		return c.withDB(args, "overview set", fs, func(db *database.SQLiteDB, pos []string) error {
			if len(pos) > 0 {
				return usagef("overview set takes flags only, not %q", strings.Join(pos, " "))
			}
			if *version < 0 {
				return usagef("overview set needs -version: the version you read (workboard overview shows it)")
			}
			var body string
			switch {
			case *file != "" && *text != "":
				return usagef("give -file or -text, not both")
			case *file != "":
				b, err := os.ReadFile(*file)
				if err != nil {
					return err
				}
				body = string(b)
			case *text != "":
				body = *text
			default:
				b, err := io.ReadAll(c.stdin)
				if err != nil {
					return err
				}
				body = string(b)
			}
			ov, audit, err := db.SetWorkOverview(body, *version, c.who())
			if err != nil {
				var conflict *database.WorkConflict
				if errors.As(err, &conflict) {
					return fmt.Errorf("%w\n(workboard overview shows the text now; nothing was written)", err)
				}
				return usagef("%v", err)
			}
			if audit == 0 {
				fmt.Fprintf(c.stdout, "the overview is unchanged (version %d): the text was the same\n", ov.Version)
				return nil
			}
			fmt.Fprintf(c.stdout, "the overview is at version %d (audit %d)\n", ov.Version, audit)
			return nil
		})
	}
	return usagef("overview %s: overview takes show, set or default", sub)
}
