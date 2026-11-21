package entity

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Over a real server's database (EQOA_REAL_DB; a copy is opened): for every
// defaultcharacter row, a new character of its race, class and city is
// refused (Refuses, board #742) neither by its home city's coachman nor by
// any merchant of its town. Its town is the chain of townsfolk (towns.go:
// players' race bodies each within townLink of the next, in one world) one of
// whom stands within townMargin of its start; its coachmen and merchants are
// the NPCs within townMargin of that chain (a start in no town, a custom
// race's home: those within homeReach of it). A refusal listed here is a start
// standing to look at, not a rule to weaken. Skipped without a database: none
// is in this repository.
// homeReach is how far from a start in no town its home's coachmen and
// merchants are looked for.
const homeReach float32 = 250

func TestRealHomeCityServes(t *testing.T) {
	if os.Getenv("EQOA_REAL_DB") == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	w := realWorld(t)
	data, err := os.ReadFile(os.Getenv("EQOA_REAL_DB"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "refusal.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := w.LoadFactions(db); err != nil {
		t.Fatal(err)
	}
	defs := database.NewDefaultCharacterStore(db).All()
	if len(defs) == 0 {
		t.Fatal("no defaultcharacter rows")
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	// the townsfolk, by world
	type folk struct {
		x, z float32
	}
	people := map[uint8][]folk{}
	for _, npc := range w.NPCs {
		if npc.ModelID != 0 && w.playerModels[uint32(npc.ModelID)] && npc.Owner == 0 {
			people[npc.World] = append(people[npc.World], folk{npc.SpawnX, npc.SpawnZ})
		}
	}
	// town is the chain of townsfolk reached from the start (x, z)
	town := func(world uint8, x, z float32) []folk {
		ps := people[world]
		in := make([]bool, len(ps))
		var queue []int
		for i, p := range ps {
			if distXZ(p.x, p.z, x, z) <= townMargin {
				in[i] = true
				queue = append(queue, i)
			}
		}
		for len(queue) > 0 {
			c := ps[queue[0]]
			queue = queue[1:]
			for i, p := range ps {
				if !in[i] && distXZ(p.x, p.z, c.x, c.z) <= townLink {
					in[i] = true
					queue = append(queue, i)
				}
			}
		}
		var out []folk
		for i, p := range ps {
			if in[i] {
				out = append(out, p)
			}
		}
		return out
	}

	checked, served, away := 0, 0, 0
	var refusals []string
	homeless := 0
	for _, d := range defs {
		chain, reach := town(uint8(d.World), d.X, d.Z), townMargin
		if len(chain) < townMinFolk {
			// a home of a custom race (world 10's, its people on creature
			// bodies): the coachmen and merchants within homeReach of the start
			homeless++
			chain, reach = []folk{{d.X, d.Z}}, homeReach
		}
		p := &Player{Name: "a new character", Level: 1, Race: d.Race, Class: d.TClass, HumType: d.HumType}
		for _, npc := range w.NPCs {
			if npc.World != uint8(d.World) || !(IsCoachman(npc.Name) || npc.NPCType&0xFF&0x01 != 0) {
				continue
			}
			if w.refusesLocked(p, npc) {
				away++ // the control: refused somewhere in its world (an evil city's, a good one's)
			}
			near := false
			for _, c := range chain {
				if distXZ(c.x, c.z, npc.SpawnX, npc.SpawnZ) <= reach {
					near = true
					break
				}
			}
			if !near {
				continue
			}
			checked++
			if w.refusesLocked(p, npc) {
				f := w.npcFactionLocked(npc)
				refusals = append(refusals, fmt.Sprintf("a new %s %s (city %d) is refused by %q at (%.0f, %.0f) world %d: %s %d",
					nameIn(raceNames, d.Race), nameIn(classNames, d.TClass), d.HumType, npc.Name, npc.SpawnX, npc.SpawnZ,
					npc.World, f, w.standingLocked(p, f)))
			} else {
				served++
			}
		}
	}
	sort.Strings(refusals)
	for _, r := range refusals {
		t.Error(r)
	}
	t.Logf("%d defaultcharacter rows (%d in no town: within %.0f of the start): %d coachman/merchant checks, %d served, %d refused",
		len(defs), homeless, homeReach, checked, served, len(refusals))
	t.Logf("the control: %d refusals by coachmen and merchants anywhere in a start's world", away)
	if away == 0 {
		t.Error("the control: no coachman or merchant anywhere refuses any new character: the rule does not bite")
	}
	if checked == 0 {
		t.Error("no home-city coachman or merchant found: the town search is wrong")
	}
}
