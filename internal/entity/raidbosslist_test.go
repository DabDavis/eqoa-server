package entity

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// #raidboss's lines (heroes.go): the GMs' list names the bosses alone, a
// listed boss with no raid template included and The Hunt's wildlife of a
// raid's hit points left out; the raid loot's rule (#raidboss all) is as it
// was, the wildlife in and the untemplated boss out. Neither lists the dead,
// a spawn-only original or a pet of a boss's name. The creatures are made up
// but for their names.
func TestTheRaidBossListShowsTheBossesAlone(t *testing.T) {
	w := NewWorld()
	boss := &ClassTemplate{ClassTemplate: database.ClassTemplate{HPPct: 4217}}
	hunt := &ClassTemplate{ClassTemplate: database.ClassTemplate{HPPct: 1417}}
	add := func(id EntityID, name string, tmpl *ClassTemplate) *NPC {
		n := &NPC{ServerID: id, Name: name, Level: 65, HP: 1000, MaxHP: 2000, World: 3, tmpl: tmpl}
		w.NPCs[id] = n
		return n
	}
	add(1, "Lord Nagafen", boss)
	add(2, "Welahi-nai Serpent", hunt)
	add(3, "Welahi-nai Serpent", hunt)
	add(4, "Poxathixius", nil)
	add(5, "Uzrathixius", boss).Dead = true
	add(6, "Bertoxxulous", boss).SpawnOnly = true
	add(7, "Lord Nagafen", nil).Owner = 1001

	listed := map[string]bool{"lord nagafen": true, "poxathixius": true, "uzrathixius": true, "bertoxxulous": true}
	got := w.ListedRaidBossesUp(listed)
	if len(got) != 3 || got[0] != "2 raid bosses up:" ||
		got[1] != "Lord Nagafen (id 1): world 3 at (0, 0, 0), HP 1000/2000 (50%), idle" ||
		!strings.HasPrefix(got[2], "Poxathixius (id 4): ") {
		t.Errorf("the list's bosses up: %q", got)
	}

	all := w.RaidBossesUp()
	if len(all) != 4 || all[0] != "3 raid bosses up:" || !strings.HasPrefix(all[1], "Lord Nagafen (id 1): ") ||
		!strings.HasPrefix(all[2], "Welahi-nai Serpent (id 2): ") || !strings.HasPrefix(all[3], "Welahi-nai Serpent (id 3): ") {
		t.Errorf("the raid loot's rule (#raidboss all): %q", all)
	}

	if got := w.ListedRaidBossesUp(map[string]bool{"poxathixius": true}); len(got) != 2 || got[0] != "1 raid boss up:" {
		t.Errorf("one boss up: %q", got)
	}
	if got := w.ListedRaidBossesUp(nil); len(got) != 0 {
		t.Errorf("an empty list lists %q", got)
	}
}
