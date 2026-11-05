// charmigrate moves a character in from another server by a plan a GM writes,
// from a shell (internal/charmove does the work; the server runs a GM's approved
// transfer the same way at the character's next login). The plan is a JSON
// file kept outside this repository (it names a player's character): see
// charmove.Plan for its fields.
//
// It writes the server's database beside the running server, so the character
// must be OUT of the world: the logout save would write its level back over
// the new one, and the game keeps its inventory in memory while it plays.
// -wait <gameserver.log> waits for that: it reads the log until the session of
// the character's latest login has been cleaned up ("Cleaning up terminated
// session ... objectID=N", N the playerObjectID its "SelectedChar" lines gave),
// and only then applies.
//
//	charmigrate -db eqoa.db -plan plan.json             a dry run: what it would do
//	charmigrate -db eqoa.db -plan plan.json -apply      do it, in one transaction
//	charmigrate -db eqoa.db -plan plan.json -apply -wait gameserver.log -backup eqoa.db.bak-<date>-pre-<name>
//	charmigrate -db eqoa.db -runs                       the runs it has made
//	charmigrate -db eqoa.db -undo <run>                 take a run back
package main

import (
	"bufio"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/eqoa/server/internal/charmove"
	"github.com/eqoa/server/internal/database"
)

func main() {
	dbPath := flag.String("db", "eqoa.db", "the server's database")
	planPath := flag.String("plan", "", "the migration plan (JSON)")
	apply := flag.Bool("apply", false, "do it (else a dry run)")
	wait := flag.String("wait", "", "the server's log: apply only once the character's session there has been cleaned up")
	waitFor := flag.Duration("wait-for", 24*time.Hour, "how long -wait waits")
	undo := flag.String("undo", "", "take this run back")
	runs := flag.Bool("runs", false, "list the runs")
	force := flag.Bool("force", false, "past the checks: a full bank, rows changed since a run")
	backup := flag.String("backup", "", "with -apply: copy the database here first (VACUUM INTO), after any -wait")
	flag.Parse()
	log.SetFlags(log.LstdFlags | log.LUTC)

	db, err := open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	switch {
	case *runs:
		if err := charmove.Runs(db, os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	case *undo != "":
		if _, _, err := charmove.Undo(db, *undo, *force, os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *planPath == "" {
		log.Fatal("give -plan (or -runs, -undo)")
	}
	p, err := charmove.ReadPlan(*planPath)
	if err != nil {
		log.Fatal(err)
	}
	if *wait != "" {
		if !*apply {
			log.Fatal("-wait goes with -apply")
		}
		if p.Character == "" {
			log.Fatal("-wait needs the plan to name the character")
		}
		if err := waitOffline(*wait, p.Character, *waitFor); err != nil {
			log.Fatal(err)
		}
	}
	if *apply && *backup != "" {
		if _, err := os.Stat(*backup); err == nil {
			log.Fatalf("%s is there already", *backup)
		}
		if _, err := db.Exec(`VACUUM INTO ?`, *backup); err != nil {
			log.Fatalf("the backup: %v", err)
		}
		log.Printf("database copied to %s", *backup)
	}
	if _, err := charmove.Migrate(db, p, *apply, *force, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func open(path string) (*sql.DB, error) {
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		return nil, fmt.Errorf("no database at %s", path)
	}
	db, err := sql.Open("sqlite", path+"?"+database.BusyTimeoutDSN)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// The log's lines a login and its end leave (handlers/character.go,
// cmd/gameserver): the character selected, then the player object it gets;
// the session's clean-up names that object.
var (
	reSelected = regexp.MustCompile(`SelectedChar: selected '([^']+)'`)
	reObject   = regexp.MustCompile(`SelectedChar: generated playerObjectID=(\d+)`)
	reCleanup  = regexp.MustCompile(`Cleaning up terminated session \S+ \(instance=\S+ objectID=(\d+)\)`)
)

// offlineInLog reads a log and says whether the character's latest login has
// ended: the session holding the object its latest selection got has been
// cleaned up. ok is false when the log holds no login of it (it may have come
// in before the log began).
func offlineInLog(path, name string) (offline, ok bool, object string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return false, false, "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	pending := 0 // lines left to find the selection's object in
	for sc.Scan() {
		line := sc.Text()
		if m := reSelected.FindStringSubmatch(line); m != nil {
			if strings.EqualFold(m[1], name) {
				pending, offline, ok, object = 8, false, false, ""
			}
			continue
		}
		if pending > 0 {
			pending--
			if m := reObject.FindStringSubmatch(line); m != nil {
				object, ok, pending = m[1], true, 0
			}
			continue
		}
		if object != "" && !offline {
			if m := reCleanup.FindStringSubmatch(line); m != nil && m[1] == object {
				offline = true
			}
		}
	}
	return offline, ok, object, sc.Err()
}

// waitOffline waits until the log says the character's session is over, then
// a few seconds more for the save behind the clean-up.
func waitOffline(path, name string, limit time.Duration) error {
	until := time.Now().Add(limit)
	said := false
	for {
		off, ok, object, err := offlineInLog(path, name)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%s holds no login of %s: is it the running server's log? (apply without -wait once you know it is out)", path, name)
		}
		if off {
			log.Printf("%s's session (object %s) has been cleaned up", name, object)
			time.Sleep(5 * time.Second)
			if off2, ok2, object2, err := offlineInLog(path, name); err == nil && ok2 && off2 && object2 == object {
				return nil
			}
			log.Printf("%s came back in meanwhile: waiting again", name)
			continue
		}
		if !said {
			log.Printf("%s is in the world (object %s): waiting for its session to end", name, object)
			said = true
		}
		if time.Now().After(until) {
			return fmt.Errorf("%s still in the world after %v", name, limit)
		}
		time.Sleep(5 * time.Second)
	}
}
