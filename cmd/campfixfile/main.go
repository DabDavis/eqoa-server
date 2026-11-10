// campfixfile files a petition's spawn request as a camp report, on the box
// beside the running server, for a session with no GM panel token (the owner,
// 2026-11-10: "lets require GM approval"). It writes a waiting row of
// campfix_reports and its work board entry, exactly as the panel's
// campfix_from_petition does (handlers.PlanCampfixPetition): the mark where
// the petition's character stood and faced when it filed it, matched as
// /campfix matches a name. The report then waits on the GM panel's Camp
// reports page like any other: nothing spawns until a GM approves it, and this
// applies nothing. It opens the database for the board and the reports alone
// (database.OpenCampfix); every row it writes is in gm_audit.
//
//	campfixfile -petition 1423 -add "bandit" -count 3 -note "the camp is empty"
//	campfixfile -petition 1424 -move "kappa guard"
//	campfixfile -petition 1425 -remove "bandit" -dry
//
// Run it where config.yaml is (the database is its server.db_path), or give
// -db.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/handlers"
)

const usage = `campfixfile: a petition's spawn request filed as a camp report.

The report waits for a GM's approval on the GM panel's Camp reports page:
nothing spawns until a GM approves it, and this applies nothing. The mark is
where the petition's character stood and faced when it filed the petition.

Usage: campfixfile -petition N (-add | -move | -remove | -turn) <creature> [-count n] [-note text] [-dry]

Give one kind. An add of -count n puts its marks on rings 3 apart round the
place; a move's, a turn's or a removal's spawn is the petition's target when
it is one of the name, else the nearest (a move or turn within 300, a removal
within 15). The creature is named as /campfix takes a name: any case, a
leading a/an/the left off, or a part of one name near.

Flags:
`

// exit codes
const (
	exitOK    = 0
	exitFail  = 1
	exitUsage = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, time.Now()))
}

func run(args []string, stdout, stderr io.Writer, now time.Time) int {
	fs := flag.NewFlagSet("campfixfile", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, usage)
		fs.PrintDefaults()
	}
	dbPath := fs.String("db", "", "the database (default: config.yaml's server.db_path, else eqoa.db)")
	cfgPath := fs.String("config", "config.yaml", "the server's config.yaml, for its database")
	as := fs.String("as", "dab's sessions", "who files it: the board entry's poster and the details' \"made a camp report by\"")
	petition := fs.Int64("petition", 0, "the petition (its ticket number)")
	kinds := map[string]*string{}
	for _, k := range handlers.CampfixKinds {
		kinds[k] = fs.String(k, "", "the creature, for a mark of kind "+k)
	}
	count := fs.Int("count", 0, "an add's: how many (1 to 15, default 1)")
	note := fs.String("note", "", "why: kept in the report's note after \"From petition N:\"")
	dry := fs.Bool("dry", false, "say what it would file, file nothing")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "campfixfile: %q is not a flag (put the creature after its kind: -add \"bandit captain\")\n", fs.Arg(0))
		return exitUsage
	}
	kind, name := "", ""
	for _, k := range handlers.CampfixKinds {
		if v := strings.TrimSpace(*kinds[k]); v != "" {
			if kind != "" {
				fmt.Fprintf(stderr, "campfixfile: one kind a report: -%s and -%s both given\n", kind, k)
				return exitUsage
			}
			kind, name = k, v
		}
	}
	if *petition <= 0 || kind == "" {
		fs.Usage()
		return exitUsage
	}
	path := *dbPath
	if path == "" {
		var err error
		if path, err = configDB(*cfgPath); err != nil {
			fmt.Fprintf(stderr, "campfixfile: %v\n", err)
			return exitFail
		}
	}
	db, err := database.OpenCampfix(path)
	if err != nil {
		fmt.Fprintf(stderr, "campfixfile: %v\n", err)
		return exitFail
	}
	defer db.Close()
	plan, err := handlers.PlanCampfixPetition(db, handlers.CampfixPetition{Petition: *petition, Kind: kind, Name: name,
		Count: *count, Note: *note}, now)
	if err != nil {
		fmt.Fprintf(stderr, "campfixfile: %v\n", err)
		return exitFail
	}
	for _, l := range plan.Said {
		fmt.Fprintln(stdout, l)
	}
	verb := "Would file"
	if !*dry {
		r, entry, err := plan.File(db, database.GMEdit{Username: strings.TrimSpace(*as), Via: "cli", IP: "local"}, now)
		if err != nil {
			fmt.Fprintf(stderr, "campfixfile: %v\n", err)
			return exitFail
		}
		plan.Report = r
		verb = fmt.Sprintf("Filed camp report %d (board entry %d)", r.ID, entry.ID)
	}
	fmt.Fprintf(stdout, "%s from petition %d: %d %s for %s in world %d.\n", verb, *petition, len(plan.Marks),
		map[bool]string{true: "mark", false: "marks"}[len(plan.Marks) == 1], plan.Report.CharName, plan.Report.World)
	for i, m := range plan.Marks {
		what := fmt.Sprintf("row %d, from (%.2f, %.2f, %.2f) facing %d, %.1f away", m.Row, m.FromX, m.FromY, m.FromZ, m.FromFacing, m.Distance)
		if m.Kind == "add" {
			what = fmt.Sprintf("the nearest row of the name now, row %d, %.1f away", m.Template, m.Distance)
		}
		fmt.Fprintf(stdout, "  %d: %s %s at (%.2f, %.2f, %.2f), facing %d; %s\n", i+1, m.Kind, m.Name, m.X, m.Y, m.Z, m.FacingByte, what)
	}
	for _, f := range plan.Flags {
		fmt.Fprintf(stdout, "  flag on mark %d (%s): %s\n", f.Mark, f.Flag, f.Text)
	}
	fmt.Fprintf(stdout, "Note: %s\n", plan.Report.Note)
	fmt.Fprintln(stdout, "It waits for a GM's approval on the GM panel's Camp reports page: nothing spawns before that.")
	return exitOK
}

// configDB is config.yaml's server.db_path (beside the config when relative),
// else eqoa.db (workboard's rule).
func configDB(cfgPath string) (string, error) {
	if _, err := os.Stat(cfgPath); err != nil {
		return "eqoa.db", nil
	}
	cfg, err := config.LoadOrDefault(cfgPath)
	if err != nil {
		return "", fmt.Errorf("%s: %w", cfgPath, err)
	}
	p := cfg.Server.DBPath
	if p == "" {
		p = "eqoa.db"
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(filepath.Dir(cfgPath), p)
	}
	return p, nil
}
