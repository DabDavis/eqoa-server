package handlers

import (
	"slices"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// ledgerBook is a coachLedger in memory: each character's signed towns.
type ledgerBook map[uint32]map[string]bool

func (l ledgerBook) CoachTowns(id uint32) (map[string]bool, error) {
	out := map[string]bool{}
	for t := range l[id] {
		out[t] = true
	}
	return out, nil
}

func (l ledgerBook) SignCoachLedger(id uint32, town string, _ time.Time) (bool, error) {
	if l[id][town] {
		return false, nil
	}
	if l[id] == nil {
		l[id] = map[string]bool{}
	}
	l[id][town] = true
	return true, nil
}

// coachWorld is Freeport's stable: Coachman Ronks on the session's slot 1, the
// coachmen of the towns Freeport's coach runs to (Highpass Hold, Neriak,
// Bobble-by-Water, Muniel's Tea Garden) and of two it never ran to
// (Rivervale, Qeynos), and the player, character 44, beside Ronks, with an
// empty ledger book and no GM.
func coachWorld(t *testing.T) (*talkWorld, uint32, ledgerBook) {
	tw := newTalkWorld(t, 25274, 15716)
	tw.p.CharServerID = 44
	book := ledgerBook{}
	tw.h.ledger = book
	tw.h.isGM = func(*session.Session) bool { return false }
	ronks := tw.add(1, &entity.NPC{ServerID: 416, Name: "Coachman Ronks", SpawnX: 25273.7, SpawnY: 54.1, SpawnZ: 15719.1})
	for _, n := range []*entity.NPC{
		{ServerID: 415, Name: "Coachman Quicksteed", SpawnX: 16781, SpawnY: 188, SpawnZ: 15356}, // Highpass Hold
		{ServerID: 978, Name: "Coachman Lothi", SpawnX: 24925, SpawnY: 29, SpawnZ: 9429},        // Neriak
		{ServerID: 100014, Name: "Coachman Boblon", SpawnX: 24641, SpawnY: 54, SpawnZ: 11865},   // Bobble-by-Water
		{ServerID: 976, Name: "Coachman Farn", SpawnX: 23370, SpawnY: 54, SpawnZ: 19636},        // Muniel's Tea Garden
		{ServerID: 977, Name: "Coachman Huff", SpawnX: 18335, SpawnY: 54, SpawnZ: 11358},        // Rivervale
		{ServerID: 101428, Name: "Coachman Billfer", SpawnX: 4813, SpawnY: 58, SpawnZ: 17144},   // Qeynos
	} {
		n.X, n.Y, n.Z = n.SpawnX, n.SpawnY, n.SpawnZ
		tw.w.NPCs[n.ServerID] = n
	}
	return tw, ronks, book
}

// offered is the towns the open coach menu lists, every page of it.
func offered(p *entity.Player) []string {
	var out []string
	for _, d := range p.PendingCoachDests {
		out = append(out, d.Name)
	}
	return out
}

// A coachman's menu offers the towns his coach runs to whose ledgers the
// player has signed, and nothing else: Rivervale and Qeynos signed but never
// on Freeport's coach are not offered, nor Neriak, Bobble-by-Water and
// Muniel's Tea Garden, on it but not signed. The control: Neriak signed is
// offered too (EQOA-Data lists Neriak only as Freeport's destination, so the
// road is read both ways), and a pick goes there.
func TestACoachRunsOnlyToSignedNeighbours(t *testing.T) {
	tw, ronks, book := coachWorld(t)
	book[44] = map[string]bool{"Freeport": true, "Highpass Hold": true, "Rivervale": true, "Qeynos": true}

	tw.talk(ronks)
	if got := optionBox(t, tw.s); !slices.Equal(got, []string{"Highpass Hold"}) {
		t.Fatalf("Ronks offered %q; want Highpass Hold alone (Rivervale and Qeynos are signed but his coach never "+
			"ran there; Neriak, Bobble-by-Water and Muniel's Tea Garden it ran to are not signed)", got)
	}

	book[44]["Neriak"] = true
	tw.talk(ronks)
	if got := optionBox(t, tw.s); !slices.Equal(got, []string{"Highpass Hold", "Neriak"}) {
		t.Fatalf("with Neriak signed Ronks offered %q, want Highpass Hold and Neriak", got)
	}
	tw.pick(ronks, 1)
	if x, z, world, moved := teleported(tw.s); !moved || x != 24925 || z != 9429 || world != 0 {
		t.Errorf("the pick went to (%.0f, %.0f) world %d (%v), want Neriak's coach (24925, 9429)", x, z, world, moved)
	}
}

// Talking to a coachman signs his town's ledger, said once in chat; with no
// town his coach runs to signed, he says so (the towns it runs to, Ronks's
// words) where the menu would be, and a pick then moves nobody. At Highpass
// Hold the return trip is offered: Quicksteed signs his ledger and offers
// Freeport, signed at Ronks's. The controls: before the talk the book is
// empty, and a second talk says nothing of the ledger.
func TestTalkingToACoachmanSignsHisLedger(t *testing.T) {
	tw, ronks, book := coachWorld(t)
	if len(book[44]) != 0 {
		t.Fatal("the control: a ledger signed before any talk")
	}
	tw.talk(ronks)
	if !book[44]["Freeport"] || len(book[44]) != 1 {
		t.Fatalf("after talking to Ronks the book holds %v, want Freeport", book[44])
	}
	if got := said(tw); !slices.Contains(got, "Coachman Ronks: "+ledgerSigned) {
		t.Errorf("Ronks said %q in chat, want his ledger signed", got)
	}
	want := "Coachman Ronks: My horses run to Bobble-by-Water, Highpass Hold, Muniel's Tea Garden and Neriak, but you " +
		"are in none of their stables' ledgers. Without that record, I cannot offer you passage."
	if got := dialogueBoxes(tw.s); !slices.Equal(got, []string{want}) {
		t.Errorf("Ronks's answer %q, want %q", got, want)
	}
	if len(tw.p.PendingCoachDests) != 0 {
		t.Errorf("an empty menu left %v to pick from", offered(tw.p))
	}
	tw.pick(ronks, 0)
	if _, _, _, moved := teleported(tw.s); moved {
		t.Error("a pick after the refusal moved the player")
	}

	tw.talk(ronks)
	if got := said(tw); slices.Contains(got, "Coachman Ronks: "+ledgerSigned) {
		t.Errorf("the control: a second talk said %q", got)
	}
	tw.s.Queue().GetPending()

	quick := tw.add(2, tw.w.NPCs[415])
	tw.w.UpdatePosition(tw.p.ServerID, 16781, 188, 15353, 0)
	tw.talk(quick)
	if !book[44]["Highpass Hold"] {
		t.Errorf("Quicksteed did not sign Highpass Hold: %v", book[44])
	}
	if got := optionBox(t, tw.s); !slices.Equal(got, []string{"Freeport"}) {
		t.Errorf("Quicksteed offered %q, want Freeport (signed at Ronks's)", got)
	}
}

// A GM (acctlevel 1 or more) is offered every coach town of the world, as
// everyone was before, ledgers or not, five a page; the GM's talk signs the
// ledger as anyone's does. The control: the same character not a GM is
// offered nothing.
func TestAGMIsOfferedEveryCoachTown(t *testing.T) {
	tw, ronks, book := coachWorld(t)
	tw.h.isGM = func(*session.Session) bool { return true }
	tw.talk(ronks)
	want := []string{"Bobble-by-Water", "Highpass Hold", "Muniel's Tea Garden", "Neriak", "Qeynos", "Rivervale"}
	if got := offered(tw.p); !slices.Equal(got, want) {
		t.Fatalf("the GM was offered %q, want every other coach town %q", got, want)
	}
	if got := optionBox(t, tw.s); len(got) != 6 || got[5] != "More..." {
		t.Errorf("the GM's first page %q, want five towns and More...", got)
	}
	if !book[44]["Freeport"] {
		t.Error("a GM's talk signed no ledger")
	}

	tw.h.isGM = func(*session.Session) bool { return false }
	tw.talk(ronks)
	if got := offered(tw.p); len(got) != 0 {
		t.Errorf("the control: not a GM, offered %q", got)
	}
}

// A coachman of no town on the network (a GM's, standing in no town) keeps the
// menu every coachman had: every other coach town of the world, ledgers or
// not; and talking to him signs nothing. The control: Ronks in the same world
// offers the same player nothing.
func TestACoachmanOffTheNetworkKeepsHisMenu(t *testing.T) {
	tw, ronks, book := coachWorld(t)
	tw.talk(ronks)
	if got := offered(tw.p); len(got) != 0 {
		t.Fatalf("the control: Ronks offered %q to a player with no ledger", got)
	}
	tw.s.Queue().GetPending()

	wanderer := tw.add(2, &entity.NPC{ServerID: 8000, Name: "Coachman Wanderer", SpawnX: 100, SpawnY: 54, SpawnZ: 100})
	tw.w.UpdatePosition(tw.p.ServerID, 100, 54, 103, 0)
	tw.talk(wanderer)
	want := []string{"Bobble-by-Water", "Freeport", "Highpass Hold", "Muniel's Tea Garden", "Neriak", "Qeynos", "Rivervale"}
	if got := offered(tw.p); !slices.Equal(got, want) {
		t.Errorf("the Wanderer offered %q, want every other coach town %q", got, want)
	}
	if len(book[44]) != 1 {
		t.Errorf("the book holds %v, want Ronks's Freeport alone: the Wanderer serves no town", book[44])
	}
}

// A quest's step at a coachman ("Sign Coachman Ronks's ledger") is answered
// by the quest, and the talk signs his ledger all the same, kept in
// charcoaches: the quest says "Your name is in my ledger." and it is. The
// control: the book is empty before the talk.
func TestALedgerQuestStepSignsTheLedger(t *testing.T) {
	f := newQuestFixture(t)
	store := database.NewSQLiteCharacterStore(f.db)
	f.tw.h.ledger = store
	f.exec(`UPDATE queststeps SET target = 'Coachman Ronks', journal = 'Sign Coachman Ronks''s ledger.',
		says = 'Coachman Ronks: Your name is in my ledger.' WHERE questid = 10010 AND step = 1`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	malsis := f.tw.add(1, &entity.NPC{ServerID: 701, Name: "Malsis", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	ronks := f.tw.add(2, &entity.NPC{ServerID: 416, Name: "Coachman Ronks", SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	f.tw.talk(malsis)
	f.tw.pick(boxID(f.msgs()), 0)
	f.msgs()
	if towns, _ := store.CoachTowns(f.sid); len(towns) != 0 {
		t.Fatalf("the control: signed %v before the talk", towns)
	}

	f.tw.talk(ronks)
	f.want(f.msgs(), "box:Coachman Ronks: Your name is in my ledger.", "drop:0")
	if s, _ := f.progress(10010); s != 2 {
		t.Errorf("the quest at step %d after the talk, want 2", s)
	}
	if towns, err := store.CoachTowns(f.sid); err != nil || !towns["Freeport"] || len(towns) != 1 {
		t.Errorf("charcoaches holds %v (%v), want Freeport", towns, err)
	}
}
