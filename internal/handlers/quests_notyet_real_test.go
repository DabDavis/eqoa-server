package handlers

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A real server's quests (EQOA_REAL_DB names its database, a copy of which is
// opened: its questnotyet made with the seed lines when it has none). Every
// enabled quest above level 1 is put to a character of its first class and
// race that has finished every quest before it (the quests it asks for and
// theirs), a level short of it, at the first spawn of its giver: the giver
// says a line of the city its words are looked up by (a starting city's
// quest: that city's own), or it has something else for the player (a quest
// to take now) or the quest asks a standing the character's start does not
// meet, and it says nothing; at the quest's level it never says it. Dust's "Frog Legs for
// Grobb" (the user's case, 2026-10-30) is checked by name when the database
// has it: Sythonis U`Dedne says one of Neriak's lines to a level 6 dark elf
// shadow knight who has finished "The Fire Bats". The givers whose city is
// unclear (no starting city, no town) are logged. Skipped without a
// database.
func TestRealQuestNotYet(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	path := filepath.Join(t.TempDir(), "notyet.db")
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	in.Close()
	out.Close()
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	database.InvalidateQuestNotYet()
	lines, err := db.QuestNotYet()
	if err != nil || len(lines) == 0 {
		t.Fatalf("questnotyet: %d lines (%v)", len(lines), err)
	}
	byCity := map[string]map[string]bool{}
	for _, l := range lines {
		k := database.QuestCityKey(l.City)
		if byCity[k] == nil {
			byCity[k] = map[string]bool{}
		}
		byCity[k][strings.TrimSpace(l.Line)] = true
	}

	store := database.NewNPCStore(db)
	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	w.LoadNPCs(store)
	if err := w.LoadFactions(db); err != nil {
		t.Fatal(err)
	}
	spawn := map[string]*entity.NPC{}
	for _, npc := range w.NPCs {
		k := foldName(npc.Name)
		if old := spawn[k]; old == nil || npc.ServerID < old.ServerID {
			spawn[k] = npc
		}
	}
	qe, err := NewQuestEngine(db, w, mgr, database.NewInventoryStore(db), nil)
	if err != nil {
		t.Fatal(err)
	}

	// before is every quest a quest asks finished first, and theirs
	var before func(qu *database.Quest, into map[int]bool)
	before = func(qu *database.Quest, into map[int]bool) {
		for _, id := range qu.Requires {
			if !into[id] {
				into[id] = true
				if r := qe.cat.byID[id]; r != nil {
					before(r, into)
				}
			}
		}
	}
	// probe is a character of the quest's first class and race at a level,
	// in the world, that has finished every quest before it and nothing else;
	// it reports what the giver says to it and, when that is nothing, why.
	next := uint32(4_000_000_000)
	probe := func(qu *database.Quest, npc *entity.NPC, level int) ([]string, string) {
		t.Helper()
		next++
		s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(192, 0, 2, 9), Port: int(next % 60000)}, next, 1, 1, 2, true)
		p := &entity.Player{ServerID: entity.EntityID(next), CharServerID: next, Name: "Probe", Level: level}
		if len(qu.Classes) > 0 {
			p.Class = qu.Classes[0]
		}
		if len(qu.Races) > 0 {
			p.Race = qu.Races[0]
		}
		w.Players[p.ServerID] = p
		defer delete(w.Players, p.ServerID)
		s.SetECSEntity(p)
		s.SetCharacterInWorld(true)
		s.SetDiscVersion(protocol.GameVersionEQOAVanilla)
		done := map[int]bool{}
		before(qu, done)
		qe.mu.Lock()
		c := qe.charLocked(next)
		for id := range done {
			c.quests[id] = &database.CharQuest{ServerID: next, QuestID: id, Done: 1}
		}
		qe.mu.Unlock()
		qe.NotYet(s, p, npc)
		said := questMsgs(s)
		if len(said) > 0 {
			return said, ""
		}
		qe.mu.Lock()
		defer qe.mu.Unlock()
		look := qe.lookAt(s)
		for _, o := range qe.cat.givers[foldName(npc.Name)] {
			if qe.mayTakeLocked(look, p, c, o) == "" {
				return nil, "a quest to take now"
			}
		}
		if why := qe.mayTakeAtLocked(look, p, c, qu, qu.MinLevel); why != "" {
			if len(qu.FactionReqs) > 0 && strings.HasPrefix(why, "its standing") {
				return nil, "a standing unmet"
			}
			return nil, why
		}
		return nil, "nothing to explain it"
	}

	type tally struct{ own, plain, offer, standing int }
	tallies := map[string]*tally{}
	unclear := map[string][]string{}
	checked := 0
	for _, qu := range qe.cat.all {
		if !qu.Enabled || qu.MinLevel <= 1 || qu.First() == nil {
			continue
		}
		npc := spawn[foldName(qu.Giver)]
		if npc == nil {
			continue
		}
		checked++
		city := qe.questCity(qu, npc)
		key := database.QuestCityKey(city)
		tl := tallies[city]
		if tl == nil {
			tl = &tally{}
			tallies[city] = tl
		}
		if city == "" {
			unclear[qu.Giver] = append(unclear[qu.Giver], fmt.Sprintf("%s L%d", strings.SplitN(strings.TrimPrefix(qu.Source, "EQOA-Data "), "/", 2)[0], qu.MinLevel))
		}
		said, why := probe(qu, npc, qu.MinLevel-1)
		switch {
		case why == "a quest to take now":
			tl.offer++
		case why == "a standing unmet":
			tl.standing++
		case len(said) == 0:
			t.Errorf("%q (L%d) at %s said nothing: %s", qu.Name, qu.MinLevel, qu.Giver, why)
		case len(said) != 1 || !strings.HasPrefix(said[0], "box:"+strings.TrimSpace(qu.Giver)+": "):
			t.Errorf("%q (L%d) at %s: %q, want its giver's line alone", qu.Name, qu.MinLevel, qu.Giver, said)
		default:
			line := strings.TrimPrefix(said[0], "box:"+strings.TrimSpace(qu.Giver)+": ")
			switch {
			case byCity[key][line] && key != "":
				tl.own++
			case byCity[""][line] && (key == "" || len(byCity[key]) == 0):
				tl.plain++
			default:
				t.Errorf("%q (L%d) at %s (%s): %q is not a line of its city", qu.Name, qu.MinLevel, qu.Giver, city, line)
			}
			if c := importCity(qu.Source); c != "" && c != "Weres" && !byCity[database.QuestCityKey(c)][line] {
				t.Errorf("%q, a %s quest, said %q: not that city's own", qu.Name, c, line)
			}
		}
		// at its level the quest is the player's to take (a standing aside),
		// which Talk offers: no "not yet"
		if len(qu.FactionReqs) == 0 {
			if at, _ := probe(qu, npc, qu.MinLevel); len(at) != 0 {
				t.Errorf("%q at its own level %d: the giver still says %q", qu.Name, qu.MinLevel, at)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no quest above level 1 has a giver in the world")
	}
	var cities []string
	for c := range tallies {
		cities = append(cities, c)
	}
	sort.Strings(cities)
	for _, c := range cities {
		tl := tallies[c]
		name := c
		if name == "" {
			name = "(no city)"
		}
		t.Logf("%-22s its own line %4d, the line of no city %4d; nothing: a quest to take now %3d, a standing unmet %3d", name, tl.own, tl.plain, tl.offer, tl.standing)
	}
	var givers []string
	for g := range unclear {
		givers = append(givers, g)
	}
	sort.Strings(givers)
	for _, g := range givers {
		t.Logf("city unclear: %-28s %s", g, strings.Join(unclear[g], ", "))
	}
	t.Logf("%d quests checked; %d givers of no known city", checked, len(givers))

	// Dust's case
	for _, qu := range qe.cat.all {
		if qu.Name != "Frog Legs for Grobb" || foldName(qu.Giver) != foldName("Sythonis U`Dedne") || !qu.Enabled {
			continue
		}
		npc := spawn[foldName(qu.Giver)]
		if npc == nil {
			t.Fatal("no Sythonis U`Dedne in the world")
		}
		said, _ := probe(qu, npc, 6)
		if len(said) != 1 || !byCity["neriak"][strings.TrimPrefix(said[0], "box:Sythonis U`Dedne: ")] {
			t.Errorf("a level 6 at Sythonis U`Dedne heard %q, want a line of Neriak's", said)
		}
		t.Logf("Dust's case: %q", said)
	}
}
