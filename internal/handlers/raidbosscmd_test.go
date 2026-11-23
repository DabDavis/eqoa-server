package handlers

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// fakeRaidBossList is a raidbosslist as #raidboss reads it.
type fakeRaidBossList struct {
	names []string
	err   error
}

func (f *fakeRaidBossList) RaidBossList() ([]string, error) { return f.names, f.err }

// #raidboss (raidbosscmd.go) lists the GMs' list's bosses up and nothing the
// raid loot's rule alone takes; #raidboss all lists by that rule, as it did;
// an empty list, a list that cannot be read and a word it does not know are
// answered; without a list it lists by the rule. The control: a player's
// #raidboss is an unknown command.
func TestRaidBossListsTheGMsBosses(t *testing.T) {
	c, gs, _, ps, _ := gmWorld(t)
	put := func(id entity.EntityID, name string) {
		c.gw.NPCs[id] = &entity.NPC{ServerID: id, Name: name, Level: 65, HP: 100, MaxHP: 100, World: 5}
	}
	put(90001, "Poxathixius")
	put(90002, "Welahi-nai Serpent")
	c.gw.SetRaidBossNames([]string{"Welahi-nai Serpent"}) // the raid loot's rule takes the serpent
	list := &fakeRaidBossList{names: []string{"poxathixius", "uzrathixius"}}
	c.h.SetRaidBossListStore(list)

	ask := func(cmd string) []string {
		t.Helper()
		gs.Queue().GetPending()
		c.say(gs, cmd)
		return texts(heard(t, gs))
	}
	if got := ask("#raidboss"); len(got) != 2 || got[0] != "[GM] 1 raid boss up:" ||
		!strings.HasPrefix(got[1], "[GM] Poxathixius (id 90001): world 5 ") {
		t.Errorf("#raidboss: %q", got)
	}
	if got := ask("/raidboss ALL"); len(got) != 2 || !strings.HasPrefix(got[1], "[GM] Welahi-nai Serpent (id 90002): ") {
		t.Errorf("#raidboss all: %q", got)
	}
	if got := ask("#raidboss bosses"); len(got) != 1 || !strings.Contains(got[0], "#raidboss all: every creature") {
		t.Errorf("a word #raidboss does not know: %q", got)
	}

	list.names = nil
	if got := ask("#raidboss"); len(got) != 1 || !strings.HasPrefix(got[0], "[GM] No raid boss is listed") {
		t.Errorf("an empty list: %q", got)
	}
	list.names = []string{"uzrathixius"}
	if got := ask("#raidboss"); !slices.Equal(got, []string{"[GM] No raid boss is up."}) {
		t.Errorf("no listed boss up: %q", got)
	}
	list.err = errors.New("database is locked")
	if got := ask("#raidboss"); len(got) != 1 || !strings.HasSuffix(got[0], "could not be read: database is locked") {
		t.Errorf("a list that cannot be read: %q", got)
	}

	c.h.SetRaidBossListStore(nil)
	if got := ask("#raidboss"); len(got) != 2 || !strings.HasPrefix(got[1], "[GM] Welahi-nai Serpent (id 90002): ") {
		t.Errorf("without a list: %q", got)
	}

	c.say(ps, "#raidboss")
	if got := heard(t, ps); len(got) != 1 || got[0].op != protocol.OpcodeErrorMessage || got[0].text != `Unrecognized command "#raidboss".` {
		t.Errorf("a player's #raidboss: %+v", got)
	}
}
