package handlers

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/server/internal/worlds"
)

// Petition 2972 (Heirloom, 2026-11-25): "Summoning keeps booting the player when they accept". A Summoning Portal
// opened inside a dungeon instance brought its player to world 14 in no instance, and the dungeon's stray check sent
// it out six seconds later, every time. Anna opens an instance of the Test Burrow alone and her portal there.

func portalInDungeon(t *testing.T) (*portalRig, *entity.NPC) {
	t.Helper()
	r := newPortalRig(t)
	block := `[{"key":"bb","name":"Test Burrow","boxes":[[900,900,1100,1100]],"entrance":{"x":1000,"y":50,"z":1150,"toward":[1000,1000]}}]`
	SetServerWorlds([]worlds.Built{{Recipe: worlds.Recipe{World: 14, Name: "the Test Dungeons", Dungeons: json.RawMessage(block)}}})
	t.Cleanup(func() {
		SetServerWorlds(nil)
		r.gw.SetDungeons(false, 14, nil, entity.DungeonSettings{})
	})
	if !SetupDungeons(r.gw, config.DungeonsConfig{Enabled: true}, nil) {
		t.Fatal("the dungeons are not on")
	}
	for _, s := range []*session.Session{r.anna, r.cora, r.bert, r.dora} {
		s.SetServerWorlds([]uint8{14})
	}
	dungeonCommand(r.gw, r.anna, "enter bb")
	r.travel()
	if r.pa.World != 14 || r.pa.InstanceKey() == 0 {
		t.Fatalf("Anna not in the dungeon: world %d instance %d", r.pa.World, r.pa.InstanceKey())
	}
	r.anna.ConsumeWorldLoadPending()
	return r, r.open()
}

// A player of the instance's party (Bert, grouped with Anna) answers yes: he lands in world 14 IN Anna's instance,
// and the stray check, past its grace and his load, leaves him there. The control: before the fix his instance
// stayed 0 and the check sent him out (the log's "goes out to in front of the Heroic Dungeons").
func TestAPortalInADungeonBringsIntoItsInstance(t *testing.T) {
	r, n := portalInDungeon(t)
	r.group(t, r.anna, r.pa, r.bert, r.pb)
	r.ask(r.anna, n)
	r.say(r.anna, "Bert")
	call := boxID(r.msgs(r.bert))
	if !IsPortalBox(call) {
		t.Fatal("no call to Bert")
	}
	r.drain()
	r.now = r.now.Add(4 * time.Second)
	r.pick(r.bert, call, 0)
	r.tick()
	if r.pb.World != 14 || r.pb.InstanceKey() != r.pa.InstanceKey() {
		t.Fatalf("Bert in world %d instance %d, want Anna's %d", r.pb.World, r.pb.InstanceKey(), r.pa.InstanceKey())
	}
	r.want("Anna", r.msgs(r.anna), "chat:Bert answers your call.")
	r.bert.ConsumeWorldLoadPending() // his client stands in world 14
	for i := 0; i < 4; i++ {
		r.now = r.now.Add(10 * time.Second)
		r.tick()
	}
	if r.pb.World != 14 || r.pb.InstanceKey() != r.pa.InstanceKey() {
		t.Fatalf("the stray check put Bert out: world %d instance %d", r.pb.World, r.pb.InstanceKey())
	}
	// his way out is where he was called from
	if why, ok := r.gw.DungeonLeave(r.bert); !ok {
		t.Fatal(why)
	}
	r.tick()
	if r.pb.World != 0 || r.pb.X != 20000 || r.pb.Z != 20000 {
		t.Fatalf("Bert's way out: world %d (%.0f, %.0f), want where he was called from", r.pb.World, r.pb.X, r.pb.Z)
	}
}

// A player the instance does not hold (Cora, in no group with Anna) is refused up front: both told why in red, no
// call box, no move. The control: Bert, of the party, is asked (above).
func TestAPortalInADungeonRefusesOneNotOfItsParty(t *testing.T) {
	r, n := portalInDungeon(t)
	r.ask(r.anna, n)
	r.say(r.anna, "Cora")
	why := "red:Cora cannot be summoned into Anna's instance of Test Burrow: they are not in its party."
	got := r.msgs(r.anna)
	if len(got) != 1 || !strings.HasPrefix(got[0], why) {
		t.Fatalf("Anna was sent %q, want %q", got, why)
	}
	got = r.msgs(r.cora)
	if len(got) != 1 || !strings.HasPrefix(got[0], why) {
		t.Fatalf("Cora was sent %q, want only %q (no call box)", got, why)
	}
	r.now = r.now.Add(10 * time.Second)
	r.tick()
	if r.pc.World != 0 || r.pc.X != 5001 {
		t.Fatalf("Cora moved: world %d x %.0f", r.pc.World, r.pc.X)
	}
	// grouped after the refusal, she may be called
	r.group(t, r.anna, r.pa, r.cora, r.pc)
	r.ask(r.anna, n)
	r.say(r.anna, "Cora")
	if call := boxID(r.msgs(r.cora)); !IsPortalBox(call) {
		t.Fatal("Cora, now of the party, was not asked")
	}
}

// A call asked while the party held its player, answered after it no longer does (Bert left the group): the yes is
// refused, both told, no move.
func TestAPortalInADungeonRefusesAtTheAnswer(t *testing.T) {
	r, n := portalInDungeon(t)
	r.group(t, r.anna, r.pa, r.bert, r.pb)
	r.ask(r.anna, n)
	r.say(r.anna, "Bert")
	call := boxID(r.msgs(r.bert))
	r.gw.GroupLeave(r.bert)
	r.drain()
	r.now = r.now.Add(4 * time.Second)
	r.pick(r.bert, call, 0)
	r.tick()
	if r.pb.World != 0 {
		t.Fatalf("Bert went: world %d", r.pb.World)
	}
	for who, s := range map[string]*session.Session{"Anna": r.anna, "Bert": r.bert} {
		found := false
		for _, m := range r.msgs(s) {
			found = found || strings.HasPrefix(m, "red:Bert cannot be summoned into Anna's instance")
		}
		if !found {
			t.Errorf("%s not told why", who)
		}
	}
}

// A GM's #summon from inside an instance brings anyone into it (added, as /addinstance adds), and the stray check
// leaves the player there.
func TestAGMSummonIntoADungeonInstance(t *testing.T) {
	r, _ := portalInDungeon(t)
	r.pa.GM = true
	r.h.summonCommand(r.anna, r.pa, []string{"Dora"})
	if r.pd.World != 14 || r.pd.InstanceKey() != r.pa.InstanceKey() {
		t.Fatalf("Dora in world %d instance %d, want Anna's %d", r.pd.World, r.pd.InstanceKey(), r.pa.InstanceKey())
	}
	r.dora.ConsumeWorldLoadPending()
	for i := 0; i < 4; i++ {
		r.now = r.now.Add(10 * time.Second)
		r.tick()
	}
	if r.pd.World != 14 {
		t.Fatalf("the stray check put Dora out: world %d", r.pd.World)
	}
}
