package handlers

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// The welcome guide (welcome.go, petition 1457) driven as a server drives it:
// the World's entry hook, the box's 0x0035 through HandleDialogueOption, typed
// lines through HandleSayChat, its timers run as they come due on the rig's
// clock. Anna and Cora stand together in Tunaria (the portal rig's), with a PS2's
// session unless a test says otherwise; the people are made up.

type welcomeRig struct {
	*portalRig
	db  *database.SQLiteDB
	g   *WelcomeGuide
	due []welcomeTimer
}

// welcomeTimer is a timer the guide set, on the rig's clock.
type welcomeTimer struct {
	at time.Time
	f  func()
}

func newWelcomeRig(t *testing.T) *welcomeRig {
	t.Helper()
	r := newPortalRig(t)
	db, err := database.NewSQLite(t.TempDir() + "/welcome.db")
	if err != nil {
		t.Fatal(err)
	}
	SetWelcomeGuide(true)
	t.Cleanup(func() { db.Close(); SetWelcomeGuide(true) })
	w := &welcomeRig{portalRig: r, db: db}
	w.restart()
	// the hook as RegisterDefaultHandlers sets it, reaching whichever guide runs now
	r.gw.OnPlayerEntered(func(s *session.Session, p *entity.Player) { w.g.Entered(s, p) })
	return w
}

// restart makes the guide anew on the same database, as a server's start does:
// nothing of the last one's memory comes through.
func (w *welcomeRig) restart() {
	g := NewWelcomeGuide(w.gw, w.db)
	g.now = func() time.Time { return w.now }
	g.later = func(d time.Duration, f func()) { w.due = append(w.due, welcomeTimer{at: w.now.Add(d), f: f}) }
	w.g, w.h.welcome, w.ih.welcome = g, g, g
	w.due = nil
}

// run moves the clock d on and runs every timer then due, the ones they set
// that come due by then too.
func (w *welcomeRig) run(d time.Duration) {
	w.now = w.now.Add(d)
	for {
		var now []welcomeTimer
		later := w.due[:0:0]
		for _, x := range w.due {
			if x.at.After(w.now) {
				later = append(later, x)
			} else {
				now = append(now, x)
			}
		}
		if len(now) == 0 {
			return
		}
		w.due = later
		for _, x := range now {
			x.f()
		}
	}
}

// enter is the character's client saying it is in the world (0x0014's hook),
// and its box's time coming.
func (w *welcomeRig) enter(s ...*session.Session) {
	for _, x := range s {
		w.gw.PlayerEntered(x)
	}
	w.run(welcomeAfter)
}

// asked is the one welcome box s was sent, by its id, failing on anything else;
// question is its question.
func (w *welcomeRig) asked(who string, s *session.Session, question string) uint32 {
	w.t.Helper()
	got := w.msgs(s)
	box := boxID(got)
	want := fmt.Sprintf("ask#%d:%s|%s|%s", box, question, welcomeGuideAnswer, welcomeNeverAnswer)
	if len(got) != 1 || !IsWelcomeBox(box) || box&0xFF000000 != 0x61000000 || got[0] != want {
		w.t.Fatalf("%s was sent %q, want %q", who, got, want)
	}
	return box
}

// none is s sent nothing.
func (w *welcomeRig) none(who string, s *session.Session) {
	w.t.Helper()
	if got := w.msgs(s); len(got) != 0 {
		w.t.Fatalf("%s was sent %q, want nothing", who, got)
	}
}

// guideBoxes is the dialogue boxes in got, failing on anything else.
func (w *welcomeRig) guideBoxes(who string, got []string) []string {
	w.t.Helper()
	var boxes []string
	for _, m := range got {
		b, ok := strings.CutPrefix(m, "box:")
		if !ok {
			w.t.Fatalf("%s was sent %q: not only the guide's boxes", who, got)
		}
		boxes = append(boxes, b)
	}
	if len(boxes) == 0 {
		w.t.Fatalf("%s was sent no guide", who)
	}
	return boxes
}

func daily(name string) string { return fmt.Sprintf(welcomeQuestionDaily, name) }

// The day's box: welcomeAfter after a character's first world entry of a UTC
// day (none at the entry itself), once that day: not at a second entry, nor
// after a restart, whatever the box's answer (here the Close); the next day it
// asks again, and its guide leaves the day after asking too. The control: Cora,
// first in the world on the second day, is asked then.
func TestWelcomeBoxAtTheFirstEntryOfADay(t *testing.T) {
	w := newWelcomeRig(t)
	w.gw.PlayerEntered(w.anna)
	w.none("Anna at the entry", w.anna)
	w.run(welcomeAfter - time.Second)
	w.none("Anna a second before the box's time", w.anna)
	w.run(time.Second)
	box := w.asked("Anna's first entry", w.anna, daily("Anna"))
	if due, _ := w.db.WelcomeDue(1, welcomeDay(w.now)); due {
		t.Fatal("the day not taken as the box went out")
	}

	// closed by hand: nothing more, nothing to come
	w.run(5 * time.Second)
	w.pick(w.anna, box, 255)
	w.none("Anna's Close", w.anna)
	if len(w.due) != 0 {
		t.Fatalf("%d timers after a Close", len(w.due))
	}

	// a second entry that day, and after a restart: not asked
	w.run(time.Hour)
	w.enter(w.anna)
	w.none("Anna's second entry that day", w.anna)
	w.restart()
	w.run(time.Hour)
	w.enter(w.anna)
	w.none("Anna after a restart that day", w.anna)

	// the next day: asked again, and Cora (her first) too
	w.run(24 * time.Hour)
	w.enter(w.anna, w.cora)
	box = w.asked("Anna the next day", w.anna, daily("Anna"))
	w.asked("Cora's first entry", w.cora, daily("Cora"))
	w.pick(w.anna, box, 0)
	w.guideBoxes("Anna's guide", w.msgs(w.anna))
	// the guide leaves the next day's box on
	w.run(24 * time.Hour)
	w.enter(w.anna)
	w.asked("Anna the day after the guide", w.anna, daily("Anna"))
}

// The day is UTC's calendar day: an entry at 23:58 and another at 00:01 are
// two days' (two boxes), two at 00:01 and 23:58 of one day are one.
func TestWelcomeDayIsUTCsCalendarDay(t *testing.T) {
	w := newWelcomeRig(t)
	w.now = time.Date(2027, 1, 15, 23, 58, 0, 0, time.UTC)
	w.enter(w.anna) // the box goes out at 23:58:10
	w.asked("Anna at 23:58", w.anna, daily("Anna"))
	w.now = time.Date(2027, 1, 16, 0, 1, 0, 0, time.UTC)
	w.enter(w.anna)
	w.asked("Anna at 00:01", w.anna, daily("Anna"))
	w.now = time.Date(2027, 1, 16, 23, 58, 0, 0, time.UTC)
	w.enter(w.anna)
	w.none("Anna at 23:58 of the same day", w.anna)
}

// Never again: kept in charwelcome, so the next day's entry, a restart's too,
// asks Anna nothing, while Cora is asked (the flag is the character's). Anna's
// /tutorial shows the box at once all the same, and its guide leaves the flag
// on: the day after asks nothing. The line says how to see it again in the
// spelling the client can send: a PS2's "/t tutorial", vanilla's "/tutorial".
func TestWelcomeNeverAgainOnThatCharacter(t *testing.T) {
	w := newWelcomeRig(t)
	w.enter(w.anna, w.cora)
	box := w.asked("Anna", w.anna, daily("Anna"))
	w.asked("Cora", w.cora, daily("Cora"))
	w.pick(w.anna, box, 1)
	w.want("Anna's never", w.msgs(w.anna), "colour:You will not be asked again on Anna. Type /t tutorial any time to see the guide.")
	if never, err := w.db.WelcomeNever(1); err != nil || !never {
		t.Fatalf("never not kept: %v %v", never, err)
	}

	w.run(24 * time.Hour)
	w.restart()
	w.enter(w.anna, w.cora)
	w.none("Anna the next day, after a restart", w.anna)
	w.asked("Cora the next day", w.cora, daily("Cora"))

	w.say(w.anna, "/tutorial")
	box = w.asked("Anna's /tutorial", w.anna, welcomeQuestionTutorial)
	w.pick(w.anna, box, 0)
	w.guideBoxes("Anna's guide from /tutorial", w.msgs(w.anna))
	w.run(24 * time.Hour)
	w.enter(w.anna)
	w.none("Anna the day after her /tutorial's guide", w.anna)
	if never, _ := w.db.WelcomeNever(1); !never {
		t.Fatal("the guide from /tutorial turned the daily box back on")
	}

	// vanilla's spelling, on Cora's next box
	w.cora.SetAckMasks()
	w.cora.SetBugReports(true)
	w.say(w.cora, "/tutorial")
	w.pick(w.cora, w.asked("Cora's /tutorial", w.cora, welcomeQuestionTutorial), 1)
	w.want("Cora's never", w.msgs(w.cora), "colour:You will not be asked again on Cora. Type /tutorial any time to see the guide.")
}

// /tutorial is everyone's: "/tutorial" and "#tutorial" (anything after them),
// "/t tutorial" and "/tell tutorial" alone (a PS2's), in any case, show the
// sender alone the box at once. The controls: "/t tutorial hello" is a tell, a
// plain "tutorial" is said to the room, and with world.welcome_guide off the
// command says there is no guide and an entry asks nothing (the day not taken,
// so once it is on again the entry asks).
func TestTutorialForEveryone(t *testing.T) {
	w := newWelcomeRig(t)
	for _, line := range []string{"/tutorial", "#tutorial", "/t tutorial", "/tell tutorial", "/TUTORIAL", "#Tutorial", "/tutorial please", "/t Tutorial  "} {
		w.say(w.anna, line)
		w.asked(line, w.anna, welcomeQuestionTutorial)
		w.none(line+" to Cora", w.cora)
	}
	w.say(w.anna, "/t tutorial hello")
	w.want("a tell", w.msgs(w.anna), "red:tutorial is not connected.")
	w.say(w.anna, "tutorial")
	w.want("a say", w.msgs(w.cora), "chat:Anna: tutorial")
	w.drain()

	SetWelcomeGuide(false)
	w.say(w.anna, "/tutorial")
	w.want("/tutorial with the guide off", w.msgs(w.anna), "red:"+welcomeOffLine)
	w.enter(w.anna)
	w.none("an entry with the guide off", w.anna)
	if len(w.due) != 0 {
		t.Fatalf("%d timers with the guide off", len(w.due))
	}
	SetWelcomeGuide(true)
	w.enter(w.anna)
	w.asked("an entry with the guide on again", w.anna, daily("Anna"))
}

// A box never goes out over a death, the way back from one, or a world loading:
// it waits, looking again every welcomeRetry, and goes out once the player is
// free. One that waits past welcomeGiveUp is let go with the day not taken (the
// next entry asks); one whose session left the world is let go at once.
func TestWelcomeWaitsForWhatComesFirst(t *testing.T) {
	w := newWelcomeRig(t)
	w.pa.HP = 0
	w.enter(w.anna)
	w.none("Anna dead", w.anna)
	w.run(welcomeRetry)
	w.none("Anna still dead", w.anna)
	w.pa.HP = 100
	w.pa.DiedAt = w.now // her way back from it
	w.run(welcomeRetry)
	w.none("Anna on her way back", w.anna)
	w.pa.DiedAt = time.Time{}
	w.anna.HoldStateChannels(time.Minute) // a world loading
	w.run(welcomeRetry)
	w.none("Anna loading a world", w.anna)
	w.anna.ReleaseStateChannels()
	w.run(welcomeRetry)
	w.asked("Anna free", w.anna, daily("Anna"))

	// past welcomeGiveUp: let go, the day not taken
	w.pc.HP = 0
	w.enter(w.cora)
	for i := 0; len(w.due) > 0; i++ {
		if i > 1000 {
			t.Fatal("never let go")
		}
		w.run(welcomeRetry)
	}
	w.none("Cora dead past the wait", w.cora)
	if due, _ := w.db.WelcomeDue(2, welcomeDay(w.now)); !due {
		t.Fatal("a box that never went out took the day")
	}
	w.pc.HP = 100
	w.enter(w.cora)
	w.asked("Cora's next entry", w.cora, daily("Cora"))

	// a session gone from the world before its box's time
	w.run(24 * time.Hour)
	w.gw.PlayerEntered(w.anna)
	w.anna.SetCharacterInWorld(false)
	w.run(welcomeAfter)
	w.none("Anna gone", w.anna)
	if len(w.due) != 0 {
		t.Fatalf("%d timers for a session gone", len(w.due))
	}
	if due, _ := w.db.WelcomeDue(1, welcomeDay(w.now)); !due {
		t.Fatal("a session gone took the day")
	}
}

// A box CLIENT declines itself (a menu up: a 255 within welcomeDeclined) goes
// out again welcomeRetry on, the same box, the day taken once; a 255 later is
// the player's Close, and the question stands for a pick right after it (a PS2
// closes one box just before its pick in the next). An answered box, and one
// from before a restart, do nothing.
func TestWelcomeDeclinedBoxGoesOutAgain(t *testing.T) {
	w := newWelcomeRig(t)
	w.enter(w.anna)
	box := w.asked("Anna", w.anna, daily("Anna"))
	w.run(500 * time.Millisecond)
	w.pick(w.anna, box, 255)
	w.none("Anna's declined box at once", w.anna)
	w.run(welcomeRetry - time.Second)
	w.none("Anna's declined box before its retry", w.anna)
	w.run(time.Second)
	if again := w.asked("Anna's declined box again", w.anna, daily("Anna")); again != box {
		t.Fatalf("a new box %08X for the declined %08X", again, box)
	}
	if due, _ := w.db.WelcomeDue(1, welcomeDay(w.now)); due {
		t.Fatal("the day free again after a decline")
	}

	w.run(5 * time.Second)
	w.pick(w.anna, box, 255)
	w.none("Anna's Close", w.anna)
	if len(w.due) != 0 {
		t.Fatalf("a Close sent the box again (%d timers)", len(w.due))
	}
	w.pick(w.anna, box, 0)
	w.guideBoxes("Anna's pick after her Close", w.msgs(w.anna))
	w.pick(w.anna, box, 0)
	w.none("an answered box picked again", w.anna)

	w.say(w.anna, "/tutorial")
	old := w.asked("Anna's /tutorial", w.anna, welcomeQuestionTutorial)
	w.restart()
	w.pick(w.anna, old, 1)
	w.none("a box from before a restart", w.anna)
	if never, _ := w.db.WelcomeNever(1); never {
		t.Fatal("a box from before a restart was answered")
	}
}

// The guide is what the server has, in the client's spelling: every box within
// a 0x0046's 217 characters, in a PS2's own characters (no '%' or '|', which
// CLIENT's text expander reads as codes), at most the eleven CLIENT queues; the
// question and the answers within a 0x0034's 127 and 63. The Heirloom Test
// Server's (the kept items, the Outfitters, Dabrish and the Spell Archivist by
// Freeport's board) to vanilla and to a PS2 word for word; a server with none
// of it (the Live Server) says none of it, nor "And more!"; and a helper not by
// the board is not said to be in Freeport.
func TestWelcomeGuideWords(t *testing.T) {
	full := welcomeFacts{lamp: true, fang: true, portal: true, rock: true, outfitters: true,
		dabrish: true, archivist: true, dabrishBoard: true, archivistBoard: true}
	heirloom := []string{
		"Your Genie's Lamp calls a genie anywhere: your bank, heirloom wares, repairs and more. Heirloom gear is free and grows with your level: an Heirloom Outfitter stands by your first quest giver.",
		"Put the Heirloom Fang of the Wolf on a quick slot: it gives you run speed. The Summoning Portal calls any player to you. The Rock of Freeport takes you to Freeport.",
		"In Freeport, by the Announcements board, Dabrish sets your level from 5 to 60 in steps of 5 and adds class mastery points. The Spell Archivist there has your class's spells. And more!",
	}
	ours := []string{
		"Found a bug? Press F3, or type /bug and what went wrong, right where it happened. A GM answers in Support's inbox (start menu).",
		"Things to explore: raids, with the Raid List window and the raid commands (/help lists them), and the raid loot rules: free for all, need before greed and master looter.",
		"Options > Extended has more graphics options and settings. Type /tutorial any time to bring this guide back. Have fun!",
	}
	ps2 := []string{
		"Found a bug? Type #bug and what went wrong, right where it happened, or use Support in the start menu. A GM answers in Support's inbox.",
		"Things to explore: raids, with the raid commands (/t help lists them), and the raid loot rules: free for all, need before greed and master looter.",
		"On the PC or browser client, Options > Extended has more graphics options and settings. Type /t tutorial any time to bring this guide back. Have fun!",
	}
	for _, c := range []struct {
		name     string
		f        welcomeFacts
		ours, f3 bool
		want     []string
	}{
		{"the Heirloom Test Server to vanilla", full, true, true, append(append([]string{}, heirloom...), ours...)},
		{"the Heirloom Test Server to a PS2", full, false, false, append(append([]string{}, heirloom...), ps2...)},
		{"a server with none of it to vanilla", welcomeFacts{}, true, true, ours},
		{"a server with none of it to a PS2", welcomeFacts{}, false, false, ps2},
	} {
		got := welcomeBoxes(c.f, c.ours, c.f3)
		if strings.Join(got, "\n") != strings.Join(c.want, "\n") {
			t.Errorf("%s:\n%s\nwant\n%s", c.name, strings.Join(got, "\n"), strings.Join(c.want, "\n"))
		}
		all := strings.Join(got, " ")
		tutorial, bug := "/t tutorial", "#bug"
		if c.ours {
			tutorial, bug = "/tutorial", "/bug"
		}
		if !strings.Contains(all, tutorial) || !strings.Contains(all, bug) || !strings.Contains(got[len(got)-1], tutorial) {
			t.Errorf("%s: no %s or %s (last: %q)", c.name, tutorial, bug, got[len(got)-1])
		}
		if !c.ours && (strings.Contains(all, " /tutorial") || strings.Contains(all, "/bug") || strings.Contains(all, "F3")) {
			t.Errorf("%s: a spelling a PS2 cannot send", c.name)
		}
		if len(got) > 11 {
			t.Errorf("%s: %d boxes, more than CLIENT queues", c.name, len(got))
		}
		for _, b := range got {
			checkConsoleText(t, c.name, b, questBoxChars)
		}
	}

	none := strings.Join(welcomeBoxes(welcomeFacts{}, true, true), " ")
	for _, word := range []string{"Genie", "Heirloom", "Portal", "Rock", "Dabrish", "Archivist", "And more!", "In Freeport"} {
		if strings.Contains(none, word) {
			t.Errorf("a server with none of it speaks of %q", word)
		}
	}
	// Dabrish away from the board, the Archivist by it; a portal with no rock
	some := welcomeBoxes(welcomeFacts{portal: true, dabrish: true, archivist: true, archivistBoard: true}, true, true)
	if some[0] != "The Summoning Portal calls any player to you, or takes you to Freeport." ||
		some[1] != "Dabrish sets your level from 5 to 60 in steps of 5 and adds class mastery points. In Freeport, by the Announcements board, the Spell Archivist has your class's spells. And more!" {
		t.Errorf("some of it: %q", some[:2])
	}

	for _, name := range []string{"Al", "Abcdefghijkl", "Abcdefghijklmnopqrstuvw"} {
		checkConsoleText(t, "the question", fmt.Sprintf(welcomeQuestionDaily, name), questAskChars)
	}
	checkConsoleText(t, "the /tutorial question", welcomeQuestionTutorial, questAskChars)
	checkConsoleText(t, "the anonymous question", welcomeQuestionAnon, questAskChars)
	for _, a := range []string{welcomeGuideAnswer, welcomeNeverAnswer} {
		checkConsoleText(t, "an answer", a, questAnswerChars)
	}
}

// checkConsoleText fails a text CLIENT would not show whole: past most
// characters, past ASCII, or carrying a code of its text expander.
func checkConsoleText(t *testing.T, what, s string, most int) {
	t.Helper()
	if n := len([]rune(s)); n > most || n == 0 {
		t.Errorf("%s: %d characters (at most %d): %q", what, n, most, s)
	}
	for _, r := range s {
		if r < ' ' || r > '~' || r == '%' || r == '|' {
			t.Errorf("%s: %q holds %q", what, s, r)
		}
	}
}

// What the guide speaks of is read off the server: the kept items' records
// (only with world.heirlooms on), the Rock of Freeport's switch, the helpers
// in the world and whether they stand by Freeport's Announcements board; and
// Anna's guide is that, as her client spells it. The controls: with
// world.heirlooms off the kept items go unsaid, and a Dabrish 200 off the board
// is not by it.
func TestWelcomeFactsAreTheServers(t *testing.T) {
	w := newWelcomeRig(t)
	defer SetHeirloomsGiven(true)
	defer SetRockOfFreeport(RockOfFreeportOn())
	record := func(name string) {
		t.Helper()
		if _, err := w.db.Exec(`INSERT INTO itempattern
			(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
			 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
			VALUES (0, 0, 0, 0, -1, 0, 0, 1, 0, 0, 0, 0, 1, 1, 0, 0, 32767, 32767, 7, 0, 0, 0, ?, '', 0, 0)`, name); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{GenieLampName, WolfFangName, SummoningPortalName} {
		record(n)
	}
	SetRockOfFreeport(true)
	npc := func(id entity.EntityID, name string, x, z float32) *entity.NPC {
		n := &entity.NPC{ServerID: id, Name: name, X: x, Y: 50, Z: z, SpawnX: x, SpawnZ: z, Level: 60, HP: 1000, MaxHP: 1000, NPCType: 0x80}
		w.gw.NPCs[id] = n
		return n
	}
	npc(9001, welcomeBoardName, 25306, 15748)
	dab := npc(9002, DabrishName, 25306, 15752)
	npc(9003, SpellArchivistName, 25310, 15755)
	npc(9004, heirloomOutfitterName, 4189, 4289)

	full := welcomeFacts{lamp: true, fang: true, portal: true, rock: true, outfitters: true,
		dabrish: true, archivist: true, dabrishBoard: true, archivistBoard: true}
	if got := w.g.readFacts(); got != full {
		t.Fatalf("the facts: %+v, want %+v", got, full)
	}
	w.anna.SetAckMasks()
	w.anna.SetBugReports(true)
	w.say(w.anna, "/tutorial")
	w.pick(w.anna, w.asked("Anna's /tutorial", w.anna, welcomeQuestionTutorial), 0)
	got := w.guideBoxes("Anna's guide", w.msgs(w.anna))
	if want := welcomeBoxes(full, true, true); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("Anna's guide:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	SetHeirloomsGiven(false)
	dab.SpawnX += 200
	f := w.g.readFacts()
	if f.lamp || f.fang || f.portal || !f.dabrish || f.dabrishBoard || !f.archivistBoard {
		t.Fatalf("heirlooms off, Dabrish 200 off the board: %+v", f)
	}
}
