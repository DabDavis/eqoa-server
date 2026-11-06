package handlers

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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

func (l ledgerBook) SignCoachLedgers(id uint32, towns []string, _ string, at time.Time) (int, error) {
	n := 0
	for _, t := range towns {
		if ok, _ := l.SignCoachLedger(id, t, at); ok {
			n++
		}
	}
	return n, nil
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

// offered is the towns the open coach menu lists, every page of it (the
// offer to sign every ledger at once is no town: offersEvery).
func offered(p *entity.Player) []string {
	var out []string
	for _, d := range p.PendingCoachDests {
		if !d.Ledgers {
			out = append(out, d.Name)
		}
	}
	return out
}

// offersEvery is whether the open coach menu offers to sign every ledger at
// once: its last entry.
func offersEvery(p *entity.Player) bool {
	n := len(p.PendingCoachDests)
	return n > 0 && p.PendingCoachDests[n-1].Ledgers
}

// A coachman's menu offers the towns his coach runs to whose ledgers the
// player has signed, and nothing else (but, last, his offer to sign every
// ledger at once): Rivervale and Qeynos signed but never on Freeport's coach
// are not offered, nor Neriak, Bobble-by-Water and Muniel's Tea Garden, on it
// but not signed. The control: Neriak signed is offered too (EQOA-Data lists
// Neriak only as Freeport's destination, so the road is read both ways), and
// a pick goes there.
func TestACoachRunsOnlyToSignedNeighbours(t *testing.T) {
	tw, ronks, book := coachWorld(t)
	book[44] = map[string]bool{"Freeport": true, "Highpass Hold": true, "Rivervale": true, "Qeynos": true}

	tw.talk(ronks)
	if got := optionBox(t, tw.s); !slices.Equal(got, []string{"Highpass Hold", everyLedgerOption}) {
		t.Fatalf("Ronks offered %q; want Highpass Hold alone and his offer (Rivervale and Qeynos are signed but his "+
			"coach never ran there; Neriak, Bobble-by-Water and Muniel's Tea Garden it ran to are not signed)", got)
	}

	book[44]["Neriak"] = true
	tw.talk(ronks)
	if got := optionBox(t, tw.s); !slices.Equal(got, []string{"Highpass Hold", "Neriak", everyLedgerOption}) {
		t.Fatalf("with Neriak signed Ronks offered %q, want Highpass Hold, Neriak and his offer", got)
	}
	tw.pick(ronks, 1)
	if x, z, world, moved := teleported(tw.s); !moved || x != 24925 || z != 9429 || world != 0 {
		t.Errorf("the pick went to (%.0f, %.0f) world %d (%v), want Neriak's coach (24925, 9429)", x, z, world, moved)
	}
}

// Talking to a coachman signs his town's ledger, said once in chat; with no
// town his coach runs to signed, he says so (the towns it runs to, Ronks's
// words) where the menu would be, and offers no town, only to sign every
// ledger at once, so a pick then moves nobody. At Highpass Hold the return
// trip is offered: Quicksteed signs his ledger and offers Freeport, signed at
// Ronks's. The controls: before the talk the book is empty, and a second talk
// says nothing of the ledger.
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
	if len(offered(tw.p)) != 0 || !offersEvery(tw.p) {
		t.Errorf("after the refusal the menu holds %v (every ledger: %v), want no town and his offer alone",
			offered(tw.p), offersEvery(tw.p))
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
	if got := optionBox(t, tw.s); !slices.Equal(got, []string{"Freeport", everyLedgerOption}) {
		t.Errorf("Quicksteed offered %q, want Freeport (signed at Ronks's) and his offer", got)
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
	if offersEvery(tw.p) {
		t.Error("the GM, offered every town already, was offered to sign every ledger")
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

// everyAsked is the coachman's answer to his offer picked, out of msgs: what he
// says it does, then his question (its id under coachBoxMark), the warning in
// it, with Yes and No.
func everyAsked(t *testing.T, msgs []string, coachman string) uint32 {
	t.Helper()
	id := boxID(msgs)
	want := []string{
		"box:" + coachman + ": " + everyLedgerTell,
		fmt.Sprintf("ask#%d:%s|%s|%s", id, everyLedgerAsk, everyLedgerYes, everyLedgerNo),
	}
	if !slices.Equal(msgs, want) || !IsCoachBox(id) {
		t.Fatalf("the offer picked: %q (box %#x), want %q under box mark %#x", msgs, id, want, coachBoxMark)
	}
	return id
}

// The words of the offer fit CLIENT's boxes: an option and an answer at most
// questAnswerChars, the question at most questAskChars, which carries the
// warning; what he says it does and his word when it is done, one box each.
func TestTheEveryLedgerWordsFit(t *testing.T) {
	for _, o := range []string{everyLedgerOption, everyLedgerYes, everyLedgerNo} {
		if n := utf8.RuneCountInString(o); n > questAnswerChars {
			t.Errorf("%q is %d characters, more than an answer's %d", o, n, questAnswerChars)
		}
	}
	for _, q := range []string{everyLedgerAsk, everyLedgerAlone} {
		if n := utf8.RuneCountInString(q); n > questAskChars {
			t.Errorf("%q is %d characters, more than a question's %d", q, n, questAskChars)
		}
	}
	if !strings.Contains(everyLedgerAsk, "may break immersion") {
		t.Errorf("the question %q does not warn of immersion", everyLedgerAsk)
	}
	for _, l := range []string{everyLedgerTell, everyLedgerDone} {
		if parts := splitSay("Coachwoman Dernia: "+l, questBoxChars); len(parts) != 1 {
			t.Errorf("%q takes %d boxes", l, len(parts))
		}
	}
}

// A coachman offers, last in his menu, to sign every ledger at once; picked,
// he says what it does and asks, warning that it may break immersion. No
// signs nothing; Yes signs every coach town's ledger (all 36), says so and
// opens his menu again, which offers his coach's neighbours alone, no longer
// the offer: Rivervale and Qeynos are signed now and still not on Freeport's
// coach. The controls: before Yes the book holds what the player signed, and
// a pick of a town still travels.
func TestACoachmanSignsEveryLedgerAtOnce(t *testing.T) {
	tw, ronks, book := coachWorld(t)
	book[44] = map[string]bool{"Highpass Hold": true}

	tw.talk(ronks)
	if got := optionBox(t, tw.s); !slices.Equal(got, []string{"Highpass Hold", everyLedgerOption}) {
		t.Fatalf("Ronks offered %q, want Highpass Hold and his offer last", got)
	}
	tw.pick(ronks, 1)
	box := everyAsked(t, questMsgs(tw.s), "Coachman Ronks")
	if len(tw.p.PendingCoachDests) != 0 {
		t.Errorf("the question left %v to pick from", tw.p.PendingCoachDests)
	}
	tw.pick(box, 1) // No
	if msgs := questMsgs(tw.s); len(msgs) != 0 {
		t.Errorf("No answered %q, want nothing", msgs)
	}
	if want := map[string]bool{"Freeport": true, "Highpass Hold": true}; !maps.Equal(book[44], want) {
		t.Fatalf("after No the book holds %v, want %v (Freeport signed by the talk)", book[44], want)
	}
	tw.pick(box, 0) // the same box again: answered already
	if len(book[44]) != 2 {
		t.Fatalf("a second answer to the old box signed %v", book[44])
	}

	tw.talk(ronks)
	if got := optionBox(t, tw.s); !slices.Equal(got, []string{"Highpass Hold", everyLedgerOption}) {
		t.Fatalf("after No Ronks offered %q, want Highpass Hold and his offer again", got)
	}
	tw.pick(ronks, 1)
	box = everyAsked(t, questMsgs(tw.s), "Coachman Ronks")
	tw.pick(box, 0) // Yes
	towns := entity.CoachTownNames()
	if len(towns) != 36 || len(book[44]) != len(towns) {
		t.Fatalf("after Yes the book holds %d towns of %d (want 36): %v", len(book[44]), len(towns), book[44])
	}
	for _, town := range towns {
		if !book[44][town] {
			t.Errorf("%s not signed", town)
		}
	}
	msgs := questMsgs(tw.s)
	want := []string{"box:Coachman Ronks: " + everyLedgerDone,
		"ask#0:Greetings, traveler! Where would you like to go?|Bobble-by-Water|Highpass Hold|Muniel's Tea Garden|Neriak"}
	if !slices.Equal(msgs, want) {
		t.Fatalf("Yes answered %q, want %q", msgs, want)
	}
	if offersEvery(tw.p) {
		t.Error("with every ledger signed the menu still offers to sign them")
	}
	tw.pick(ronks, 3)
	if x, z, _, moved := teleported(tw.s); !moved || x != 24925 || z != 9429 {
		t.Errorf("the pick went to (%.0f, %.0f) (%v), want Neriak's coach", x, z, moved)
	}
}

// With no town his coach runs to signed, a coachman says so and then makes
// his offer alone, asked as a question of its own; a close (255) of his
// question changes nothing and leaves it to answer, and Yes then signs.
func TestACoachmanOffersEveryLedgerWhenNoneHeRunsToIsSigned(t *testing.T) {
	tw, ronks, book := coachWorld(t)
	tw.talk(ronks)
	msgs := questMsgs(tw.s)
	if len(msgs) != 3 || msgs[0] != "chat:Coachman Ronks: "+ledgerSigned ||
		!strings.HasPrefix(msgs[1], "box:Coachman Ronks: My horses run to ") ||
		msgs[2] != "ask#0:"+everyLedgerAlone+"|"+everyLedgerOption {
		t.Fatalf("Ronks answered %q, want his ledger signed, why he offers no town, then his offer alone", msgs)
	}
	tw.pick(ronks, 0)
	box := everyAsked(t, questMsgs(tw.s), "Coachman Ronks")
	tw.pick(box, 255)
	if len(book[44]) != 1 {
		t.Fatalf("a close signed %v", book[44])
	}
	tw.pick(box, 0)
	if len(book[44]) != 36 {
		t.Errorf("Yes after a close signed %d towns, want 36", len(book[44]))
	}
}

// No offer is made where it would grant nothing: to a character that has
// signed every ledger, to a GM (offered every town already) or at a coachman
// off the network; nor without a ledger store. A box another character's
// question had, or one never asked, signs nothing. The control: the same
// character one town short is offered it.
func TestNoEveryLedgerOfferWhereItGrantsNothing(t *testing.T) {
	tw, ronks, book := coachWorld(t)
	book[44] = map[string]bool{}
	for _, town := range entity.CoachTownNames() {
		book[44][town] = true
	}
	tw.talk(ronks)
	if got := optionBox(t, tw.s); !slices.Equal(got, []string{"Bobble-by-Water", "Highpass Hold", "Muniel's Tea Garden", "Neriak"}) {
		t.Errorf("with every ledger signed Ronks offered %q, want his neighbours alone", got)
	}

	delete(book[44], "Qeynos")
	tw.talk(ronks)
	if !offersEvery(tw.p) {
		t.Fatalf("the control: one town short, no offer (%v)", tw.p.PendingCoachDests)
	}
	tw.s.Queue().GetPending()

	tw.h.isGM = func(*session.Session) bool { return true }
	tw.talk(ronks)
	if offersEvery(tw.p) {
		t.Error("a GM was offered to sign every ledger")
	}
	tw.h.isGM = func(*session.Session) bool { return false }
	tw.s.Queue().GetPending()

	wanderer := tw.add(2, &entity.NPC{ServerID: 8000, Name: "Coachman Wanderer", SpawnX: 100, SpawnY: 54, SpawnZ: 100})
	tw.w.UpdatePosition(tw.p.ServerID, 100, 54, 103, 0)
	tw.talk(wanderer)
	if offersEvery(tw.p) {
		t.Error("a coachman off the network offered to sign every ledger")
	}
	tw.s.Queue().GetPending()

	tw.pick(coachBoxMark|77, 0)
	if book[44]["Qeynos"] {
		t.Error("a box never asked signed every ledger")
	}

	tw.h.ledger = nil
	tw.w.UpdatePosition(tw.p.ServerID, 25274, 54, 15716, 0)
	tw.talk(ronks)
	if offersEvery(tw.p) {
		t.Error("with no ledger store Ronks offered to sign every ledger")
	}
}

// Yes writes, through the database's ledgers, a row for each town not yet
// signed, source coachman-all; a ledger signed at its stable keeps its row
// (source talk, its time).
func TestEveryLedgerAtOnceIsKeptAsItsOwnSource(t *testing.T) {
	tw, ronks, _ := coachWorld(t)
	db, err := database.NewSQLite(t.TempDir() + "/ledgers.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tw.h.ledger = database.NewSQLiteCharacterStore(db)

	tw.talk(ronks)
	questMsgs(tw.s)
	tw.pick(ronks, 0)
	tw.pick(everyAsked(t, questMsgs(tw.s), "Coachman Ronks"), 0)
	rows := map[string]string{}
	r, err := db.Query(`SELECT town, source FROM charcoaches WHERE serverid = 44`)
	if err != nil {
		t.Fatal(err)
	}
	for r.Next() {
		var town, source string
		r.Scan(&town, &source)
		rows[town] = source
	}
	r.Close()
	if len(rows) != 36 || rows["Freeport"] != "talk" {
		t.Fatalf("%d rows, Freeport's source %q; want 36, Freeport signed by the talk", len(rows), rows["Freeport"])
	}
	for town, source := range rows {
		if town != "Freeport" && source != database.CoachLedgersAll {
			t.Errorf("%s's source %q, want %s", town, source, database.CoachLedgersAll)
		}
	}
}

// Petition 1124, the GM Jibrish: "The Oggok coachman is not on the coach network
// with any connected paths." Coachman Dilga, in the Rathe, is Oggok's: his coach
// runs to Dark Solace in Tunaria and Dark Solace's back, each once the other's
// ledger is signed, and a ride changes the player's world. The network's other
// towns stay off his menu (Freeport, no route of his). The control: a coachman of
// no town in the Rathe offers only the Rathe's.
func TestOggoksCoachRunsAcrossWorlds(t *testing.T) {
	tw := newTalkWorld(t, 8962, 7214)
	tw.p.CharServerID, tw.p.World = 44, 1
	book := ledgerBook{}
	tw.h.ledger = book
	tw.h.isGM = func(*session.Session) bool { return false }
	dilga := &entity.NPC{ServerID: 101444, Name: "Coachman Dilga", World: 1, SpawnX: 8962, SpawnY: 54, SpawnZ: 7214}
	nioclas := &entity.NPC{ServerID: 101026, Name: "Coachman Nioclas", World: 0, SpawnX: 12539, SpawnY: 84, SpawnZ: 21182}
	ronks := &entity.NPC{ServerID: 416, Name: "Coachman Ronks", World: 0, SpawnX: 25273.7, SpawnY: 54.1, SpawnZ: 15719.1}
	for _, n := range []*entity.NPC{dilga, nioclas, ronks} {
		n.X, n.Y, n.Z = n.SpawnX, n.SpawnY, n.SpawnZ
		tw.w.NPCs[n.ServerID] = n
	}

	dests, none, _ := tw.h.coachDests(tw.s, tw.p, dilga)
	if len(dests) != 0 || !strings.Contains(none, "Dark Solace") {
		t.Fatalf("no ledger signed: offered %v, says %q; want Dark Solace named and not offered", dests, none)
	}
	book[44] = map[string]bool{"Dark Solace": true, "Freeport": true, "Oggok": true}
	dests, _, _ = tw.h.coachDests(tw.s, tw.p, dilga)
	if len(dests) != 1 || dests[0].Name != "Dark Solace" || dests[0].World != 0 {
		t.Fatalf("from Oggok: %+v, want Dark Solace of world 0 alone", dests)
	}
	back, _, _ := tw.h.coachDests(tw.s, tw.p, nioclas)
	if len(back) != 1 || back[0].Name != "Oggok" || back[0].World != 1 || back[0].X != 8962 {
		t.Fatalf("from Dark Solace: %+v, want Oggok of world 1, at Dilga's", back)
	}
	tw.h.travel(tw.s, tw.p, dests[0])
	if tw.p.World != 0 {
		t.Errorf("the ride left the player in world %d, want Tunaria", tw.p.World)
	}

	wanderer := &entity.NPC{ServerID: 9001, Name: "Coachman Wanderer", World: 1, SpawnX: 7000, SpawnY: 54, SpawnZ: 5000}
	tw.w.NPCs[wanderer.ServerID] = wanderer
	off, _, _ := tw.h.coachDests(tw.s, tw.p, wanderer)
	if len(off) != 1 || off[0].Name != "Oggok" {
		t.Errorf("a coachman of no town in the Rathe offers %+v, want the Rathe's Oggok alone", off)
	}
}
