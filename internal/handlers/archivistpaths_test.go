package handlers

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The Spell Archivist's paths (petition 1325) against a database, a real
// world, the window and interact handlers and the quests running: an
// enchanter of the paths' reward quests as the import made them (an ability
// and a weapon a choice; a second city's group giving the same abilities), the
// spells, a basic spell and Return Home in its book. The data is made up, its
// names and ids the live server's.

const (
	spReturnHome = 1623
	spArcaneBolt = 6101 // a basic spell, on the bar
	spExpansive  = 700328
	spPowerJet   = 700340
	spLimitless  = 700462
	spPowerSurge = 700468
	spBoundless  = 700575
	spInfinite   = 700618
	spMindRush   = 6102 // sold by a merchant: a basic spell, though a reward offers it

	qEnc30     = 100969
	qEnc40     = 100973
	qEnc49     = 100977
	qEnc30Evil = 100992
	qEnc40Evil = 100995

	itBeheader = 700878
	itWand     = 700990
	classEnc   = 12
)

type pathRig struct {
	*archivistFixture
	qe *QuestEngine
}

// quest adds a reward quest by its import key at its giver, a talk step a
// choices column (choice, rehab_test.go).
func (f *pathRig) quest(id int, key string, steps ...string) {
	f.t.Helper()
	if _, err := f.db.Exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, classes, source)
		VALUES (?, ?, 1, 'Trillon Zeber', 30, 60, '12', ?)`, id, "Quest "+key, "EQOA-Data "+key+" (tools/import_quests.py)"); err != nil {
		f.t.Fatal(err)
	}
	for i, st := range steps {
		if _, err := f.db.Exec(`INSERT INTO queststeps (questid, step, kind, target, journal, choices) VALUES (?, ?, 'talk', 'Trillon Zeber', 'Talk.', ?)`,
			id, i+1, st); err != nil {
			f.t.Fatal(err)
		}
	}
}

// done marks a quest finished with its picks ("" none kept).
func (f *pathRig) done(id int, picks string) {
	f.t.Helper()
	if _, err := f.db.Exec(`INSERT INTO charquests (serverid, questid, step, count, done, slot, picks) VALUES (?, ?, 0, 0, 1, 0, ?)`,
		f.p.CharServerID, id, picks); err != nil {
		f.t.Fatal(err)
	}
	f.qe.ReloadCharacter(f.p.CharServerID)
}

func newPathRig(t *testing.T, level int) *pathRig {
	t.Helper()
	f := &pathRig{archivistFixture: newArchivistFixture(t, nil)}
	f.p.Class, f.p.Level = classEnc, level
	f.p.Power, f.p.MaxPower = 2000, 2000
	for _, s := range []struct {
		id    int
		name  string
		level int
		desc  string
	}{
		{spReturnHome, "Return Home", 1, ""},
		{spArcaneBolt, "Arcane Bolt", 1, "Target, Decrease Hitpoints by 46"},
		{spExpansive, "Expansive Mind", 30, "Self, Increase Maximum Power by 260, Increase Power by 210, 20min"},
		{spPowerJet, "Power Jet", 30, "Target, increase Power by 200"},
		{spLimitless, "Limitless Mind", 40, "Self, Increase Maximum Power by 360, Power by 360, 20min"},
		{spPowerSurge, "Power Surge", 40, "Target, Increase Power by 300"},
		{spBoundless, "Boundless Mind", 49, "Self, Increase Maximum Power by 540, Increase Power by 540, 20min"},
		{spInfinite, "Infinite Mind", 49, "Self, Increase Maximum Power by 640, Increase Power by 640, 20min"},
		{spMindRush, "Mind Rush", 40, "Target, Increase Power by 100"},
	} {
		f.spell(s.id, s.name, s.level, s.desc)
	}
	if _, err := f.db.Exec(`UPDATE spellpattern SET recast = 300 WHERE spellid = ?`, spExpansive); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE spellpattern SET scope = 0 WHERE spellid IN (?, ?, ?, ?)`, spExpansive, spLimitless, spBoundless, spInfinite); err != nil {
		t.Fatal(err)
	}
	f.shop(SpellArchivistName, 0x81, archivistShop)
	f.shop("Scroll Merchant", 0x81, 41, f.scroll("Mind Rush", 1<<classEnc, 40))
	ability := func(spell, item int, name, weapon string) string {
		return fmt.Sprintf("%s and the %s=%d:%d", name, weapon, spell, item)
	}
	f.quest(qEnc30, "30,40,49/group-2-good/L30-enchanter", choice(
		ability(spExpansive, itBeheader, "Expansive Mind", "Lycaon Beheader"), ability(spExpansive, itWand, "Expansive Mind", "Tainted Black Wand"),
		ability(spPowerJet, itBeheader, "Power Jet", "Lycaon Beheader"), ability(spPowerJet, itWand, "Power Jet", "Tainted Black Wand")))
	f.quest(qEnc40, "30,40,49/group-2-good/L40-enchanter", choice(
		ability(spLimitless, itBeheader, "Limitless Mind", "Hammer of Lightwolf"), ability(spPowerSurge, itBeheader, "Power Surge", "Hammer of Lightwolf"),
		ability(spMindRush, itWand, "Mind Rush", "Wand of Mystery")))
	f.quest(qEnc49, "30,40,49/group-2-good/L49-enchanter", choice(
		ability(spBoundless, itWand, "Boundless Mind", "Mithril Eagle Rod"), ability(spInfinite, itWand, "Infinite Mind", "Mithril Eagle Rod")))
	f.quest(qEnc30Evil, "30,40,49/group-4-evil/L30-enchanter", choice(
		ability(spExpansive, itWand, "Expansive Mind", "Deceptive Staff"), ability(spPowerJet, itWand, "Power Jet", "Tainted Crystal Staff")))
	f.quest(qEnc40Evil, "30,40,49/group-4-evil/L40-enchanter", choice(
		ability(spLimitless, itWand, "Limitless Mind", "Enthralling Branch"), ability(spPowerSurge, itWand, "Power Surge", "Dark Visage Rattle")))
	qe, err := NewQuestEngine(f.db, f.w, f.mgr, f.inv, f.spells)
	if err != nil {
		t.Fatal(err)
	}
	qe.Start()
	t.Cleanup(qe.Stop)
	f.qe = qe
	f.a.quests = func() *QuestEngine { return qe }
	f.know(spReturnHome, 0, 0)
	f.know(spArcaneBolt, 1, 1)
	return f
}

// paths answers his box "Grant path spells" and returns what he said and
// asked, his question's box checked against the answers want.
func (f *pathRig) paths(want ...string) ([]string, uint32) {
	f.t.Helper()
	got := f.pick(f.menu(), archivistPath)
	if len(got) == 0 || !archivistLike(got[len(got)-1], "ask#*:"+archivistPathAsk+"|"+strings.Join(want, "|")) {
		f.t.Fatalf("the paths asked: %q, want the answers %q", got, want)
	}
	return got, boxID(got)
}

// wantBook checks the book's rows ("id@index", "id@index/slot" on the bar).
func (f *pathRig) wantBook(what string, want ...string) {
	f.t.Helper()
	if got := f.book(); strings.Join(got, " ") != strings.Join(want, " ") {
		f.t.Errorf("%s: the book %v, want %v", what, got, want)
	}
}

func bookRow(id, index int) string { return fmt.Sprintf("%d@%d", id, index) }

var encAnswers = []string{"The path of Expansive Mind", "The path of Power Jet", archivistPathNeither}

// The enchanter's paths asked, and a change asked: their questions as sent.
var (
	encAsked  = "ask#*:" + archivistPathAsk + "|" + strings.Join(encAnswers, "|")
	swapAsked = "ask#*:" + archivistPathAskSwap + "|Yes, change it|No"
)

// "Grant path spells" says each path's spells to the player's level (the
// common spells none here) and asks which, one answer a path and Neither;
// the one picked is written in at once when nothing of another is in the
// book: its spells of the levels reached, after the book's last index, off
// the bar, each 0x00B3 with the flag 0, then one box. A level gained gives
// the next level's on the same pick; the same pick again gives nothing. The
// controls: the 49's spells are not given at 45, the other path's never, a
// spell a merchant sells (Mind Rush, a 40 option too) is no path's, Neither
// sends nothing, and an answer out of reach is refused.
func TestTheArchivistGivesAPathsSpells(t *testing.T) {
	f := newPathRig(t, 45)
	got, box := f.paths(encAnswers...)
	f.wantMsgs("the paths at 45", got,
		"box:Spell Archivist: The path of Expansive Mind: Expansive Mind and Limitless Mind. The path of Power Jet: Power Jet and Power Surge.",
		encAsked)
	f.wantMsgs("Neither", f.pick(box, 2))
	f.wantBook("Neither", "1623@0/0", "6101@1/1")

	_, box = f.paths(encAnswers...)
	f.wantMsgs("the path of Expansive Mind at 45", f.pick(box, 0), "add:700328@2 flag 0", "add:700462@3 flag 0",
		"box:Spell Archivist: There: Expansive Mind and Limitless Mind written into your book. The path of Expansive Mind is yours.")
	f.wantBook("the path of Expansive Mind at 45", "1623@0/0", "6101@1/1", bookRow(spExpansive, 2), bookRow(spLimitless, 3))

	f.p.Level = 49
	got, box = f.paths(encAnswers...)
	f.wantMsgs("the paths at 49", got,
		"box:Spell Archivist: The path of Expansive Mind: Expansive Mind, Limitless Mind, Boundless Mind and Infinite Mind. The path of Power Jet: Power Jet and Power Surge.",
		"box:Spell Archivist: Your book holds the path of Expansive Mind. "+archivistPathOne,
		encAsked)
	f.wantMsgs("the same path at 49", f.pick(box, 0), "add:700575@4 flag 0", "add:700618@5 flag 0",
		"box:Spell Archivist: There: Boundless Mind and Infinite Mind written into your book. The path of Expansive Mind is yours.")
	_, box = f.paths(encAnswers...)
	f.wantMsgs("the same path again", f.pick(box, 0),
		"box:Spell Archivist: Your book holds the path of Expansive Mind to your level already.")
	f.wantBook("at 49", "1623@0/0", "6101@1/1", bookRow(spExpansive, 2), bookRow(spLimitless, 3), bookRow(spBoundless, 4), bookRow(spInfinite, 5))

	_, box = f.paths(encAnswers...)
	f.p.X += 50
	f.wantMsgs("out of reach", f.pick(box, 1), "red:You are too far from the Spell Archivist.")
	f.p.X -= 50
	f.wantBook("out of reach", "1623@0/0", "6101@1/1", bookRow(spExpansive, 2), bookRow(spLimitless, 3), bookRow(spBoundless, 4), bookRow(spInfinite, 5))
}

// The owner's example: an enchanter of self power who takes power for others
// has the self power line taken out of the book and power for others written
// in, asked first, and back again the other way. The change is said and asked
// Yes or No (No keeps the book); Yes sends 0x00B8 a row, then 0x00B3 a spell
// at the indices the gone ones left, then one box; a memorized spell leaves
// the bar with its row, and the new ones come in off it. The controls: the
// basic spell and Return Home stay, bar slots and all; No changes nothing.
func TestTheArchivistSwapsTheEnchantersPowerLines(t *testing.T) {
	f := newPathRig(t, 50)
	_, box := f.paths(encAnswers...)
	f.pick(box, 0)
	if _, err := f.db.Exec(`UPDATE spells SET onHotBar = 1, whereonBar = 2 WHERE serverid = ? AND spellid = ?`, f.p.CharServerID, spLimitless); err != nil {
		t.Fatal(err)
	}
	self := []string{"1623@0/0", "6101@1/1", bookRow(spExpansive, 2), "700462@3/2", bookRow(spBoundless, 4), bookRow(spInfinite, 5)}
	f.wantBook("self power", self...)

	ask := func() uint32 {
		t.Helper()
		_, box := f.paths(encAnswers...)
		got := f.pick(box, 1)
		f.wantMsgs("power for others asked", got,
			"box:Spell Archivist: For the path of Power Jet, Expansive Mind, Limitless Mind, Boundless Mind and Infinite Mind leave your book, and Power Jet and Power Surge enter it.",
			"ask#*:"+archivistPathAskSwap+"|Yes, change it|No")
		return boxID(got)
	}
	f.wantMsgs("No", f.pick(ask(), 1), "box:Spell Archivist: "+archivistKept)
	f.wantBook("No", self...)

	box = ask()
	f.wantMsgs("a close of the question", f.pick(box, 255))
	f.wantMsgs("power for others", f.pick(box, 0), "remove:2", "remove:3", "remove:4", "remove:5", "add:700340@2 flag 0", "add:700468@3 flag 0",
		"box:Spell Archivist: It is done: the path of Power Jet is yours now. Out of your book: Expansive Mind, Limitless Mind, Boundless Mind, Infinite Mind. Into it: Power Jet, Power Surge.")
	f.wantBook("power for others", "1623@0/0", "6101@1/1", bookRow(spPowerJet, 2), bookRow(spPowerSurge, 3))
	f.wantMsgs("an old box", f.pick(box, 0))

	got, box := f.paths(encAnswers...)
	f.wantMsgs("the paths held", got, "box:*", "box:Spell Archivist: Your book holds the path of Power Jet. "+archivistPathOne, encAsked)
	f.wantMsgs("self power again, asked", f.pick(box, 0),
		"box:Spell Archivist: For the path of Expansive Mind, Power Jet and Power Surge leave your book, and Expansive Mind, Limitless Mind, Boundless Mind and Infinite Mind enter it.",
		swapAsked)
	f.wantMsgs("self power again", f.pick(boxID(f.lastAsk()), 0), "remove:2", "remove:3",
		"add:700328@2 flag 0", "add:700462@3 flag 0", "add:700575@4 flag 0", "add:700618@5 flag 0",
		"box:Spell Archivist: It is done: the path of Expansive Mind is yours now. Out of your book: Power Jet, Power Surge. Into it: Expansive Mind, Limitless Mind, Boundless Mind, Infinite Mind.")
	f.wantBook("self power again", "1623@0/0", "6101@1/1", bookRow(spExpansive, 2), bookRow(spLimitless, 3), bookRow(spBoundless, 4), bookRow(spInfinite, 5))
}

// lastAsk is a pretend message holding the box the Archivist asked last, for
// boxID.
func (f *pathRig) lastAsk() []string {
	f.a.mu.Lock()
	defer f.a.mu.Unlock()
	return []string{fmt.Sprintf("ask#%d:", f.a.asked[f.p.CharServerID].id)}
}

// What a quest gave stays, whatever path he writes: an enchanter whose level
// 30 reward was Power Jet (its pick kept) is given the self power path beside
// it at once (nothing of another path comes out), and Power Jet stays through
// a change to self power and back, said so; only what he wrote comes out. A
// pick from before picks were kept counts every option of its step as given
// (the level 40 reward's Limitless Mind and Power Surge both stay). The
// control is TestTheArchivistSwapsTheEnchantersPowerLines, where no quest gave
// Power Jet and it comes out.
func TestTheArchivistKeepsWhatTheQuestsGave(t *testing.T) {
	f := newPathRig(t, 45)
	f.done(qEnc30, "1:2") // Power Jet and the Lycaon Beheader
	f.know(spPowerJet, 2, -1)
	got, box := f.paths(encAnswers...)
	f.wantMsgs("the paths, Power Jet the quest's", got, "box:*", "box:Spell Archivist: Your book holds the path of Power Jet. "+archivistPathOne, encAsked)
	f.wantMsgs("self power beside the quest's", f.pick(box, 0), "add:700328@3 flag 0", "add:700462@4 flag 0",
		"box:Spell Archivist: There: Expansive Mind and Limitless Mind written into your book. The path of Expansive Mind is yours.")
	f.wantBook("self power beside the quest's", "1623@0/0", "6101@1/1", bookRow(spPowerJet, 2), bookRow(spExpansive, 3), bookRow(spLimitless, 4))

	_, box = f.paths(encAnswers...)
	f.wantMsgs("power for others asked", f.pick(box, 1),
		"box:Spell Archivist: For the path of Power Jet, Expansive Mind and Limitless Mind leave your book, and Power Surge enters it.", swapAsked)
	f.wantMsgs("power for others", f.pick(boxID(f.lastAsk()), 0), "remove:3", "remove:4", "add:700468@3 flag 0", "box:*")
	f.wantBook("power for others", "1623@0/0", "6101@1/1", bookRow(spPowerJet, 2), bookRow(spPowerSurge, 3))

	_, box = f.paths(encAnswers...)
	f.wantMsgs("self power asked", f.pick(box, 0),
		"box:Spell Archivist: For the path of Expansive Mind, Power Surge leaves your book, and Expansive Mind and Limitless Mind enter it. Power Jet stays in your book: your quests gave it.",
		swapAsked)
	f.wantMsgs("self power", f.pick(boxID(f.lastAsk()), 0), "remove:3", "add:700328@3 flag 0", "add:700462@4 flag 0", "box:*")
	f.wantBook("self power", "1623@0/0", "6101@1/1", bookRow(spPowerJet, 2), bookRow(spExpansive, 3), bookRow(spLimitless, 4))

	// a pick not kept: both of the step's options count as the quest's
	g := newPathRig(t, 45)
	g.done(qEnc40, "")
	g.know(spLimitless, 2, -1)
	g.know(spPowerSurge, 3, -1)
	_, box = g.paths(encAnswers...)
	g.wantMsgs("self power, the 40 pick not kept", g.pick(box, 0), "add:700328@4 flag 0",
		"box:Spell Archivist: There: Expansive Mind written into your book. The path of Expansive Mind is yours.")
	_, box = g.paths(encAnswers...)
	g.wantMsgs("power for others asked", g.pick(box, 1),
		"box:Spell Archivist: For the path of Power Jet, Expansive Mind leaves your book, and Power Jet enters it. Limitless Mind stays in your book: your quests gave it.",
		swapAsked)
	g.pick(boxID(g.lastAsk()), 0)
	g.wantBook("power for others, the 40 pick not kept", "1623@0/0", "6101@1/1", bookRow(spLimitless, 2), bookRow(spPowerSurge, 3), bookRow(spPowerJet, 4))
}

// Below level 30 he offers no path: one box saying when, no question, the
// book as it was. The control: at 30 the paths are offered, each its 30's.
func TestTheArchivistOffersNoPathBelowThirty(t *testing.T) {
	f := newPathRig(t, 29)
	f.wantMsgs("at 29", f.pick(f.menu(), archivistPath), "box:Spell Archivist: "+fmt.Sprintf(archivistPathsTooLow, 30))
	f.wantBook("at 29", "1623@0/0", "6101@1/1")

	f.p.Level = 30
	got, box := f.paths(encAnswers...)
	f.wantMsgs("at 30", got, "box:Spell Archivist: The path of Expansive Mind: Expansive Mind. The path of Power Jet: Power Jet.", encAsked)
	// a level lost between the question and the answer: nothing either
	f.p.Level = 29
	f.wantMsgs("29 again at the answer", f.pick(box, 0), "box:Spell Archivist: "+fmt.Sprintf(archivistPathsTooLow, 30))
	f.wantBook("29 again", "1623@0/0", "6101@1/1")
}

// A cast on its way holds the change back (it would land after it and put
// back what it ended), nothing moved; once it lands the change is made, the
// effect the cast put on the player ends with its spell, and its lockout runs
// on: given back within it, the spell is shown locked out at its new index
// (0x00CD) and refused. The controls: Limitless Mind, never cast, comes back
// with no lockout; a first path with nothing coming out is not held back by a
// cast.
func TestTheArchivistWaitsForACastAndKeepsItsLockout(t *testing.T) {
	f := newPathRig(t, 45)
	_, box := f.paths(encAnswers...)
	f.pick(box, 0)
	book, err := f.spells.GetCharacterSpells(f.p.CharServerID)
	if err != nil {
		t.Fatal(err)
	}
	var expansive database.CharSpellEntry
	for _, e := range book {
		if e.Pattern.SpellID == spExpansive {
			expansive = e
		}
	}
	spell := entity.SpellOf(&expansive.Pattern, expansive.AddedOrder, expansive.Pattern.SpellDesc, 0)
	if !f.w.StartCast(f.sess, spell, 0xFFFFFFFF, time.Now()) {
		t.Fatal("Expansive Mind was not cast")
	}
	f.sess.Queue().GetPending()

	_, box = f.paths(encAnswers...)
	f.pick(box, 1)
	f.wantMsgs("the change while casting", f.pick(boxID(f.lastAsk()), 0), "box:Spell Archivist: "+archivistPathCasting)
	f.wantBook("while casting", "1623@0/0", "6101@1/1", bookRow(spExpansive, 2), bookRow(spLimitless, 3))

	f.w.TickCount = 0
	f.w.Tick(time.Now().Add(spell.CastTime + 2*time.Second)) // landed: the effect on
	f.sess.Queue().GetPending()
	if got := f.effects(); got != "Expansive Mind" {
		t.Fatalf("after the landing the effects are %q, want Expansive Mind", got)
	}
	_, box = f.paths(encAnswers...)
	f.pick(box, 1)
	f.wantMsgs("the change landed", f.pick(boxID(f.lastAsk()), 0), "remove:2", "remove:3", "add:700340@2 flag 0", "add:700468@3 flag 0",
		"box:*It is done*", "box:Spell Archivist: The old path's effects on you have ended: Expansive Mind.")
	if got := f.effects(); got != "" {
		t.Errorf("the effects after the change: %q, want none", got)
	}

	_, box = f.paths(encAnswers...)
	f.pick(box, 0)
	f.wantMsgs("self power back", f.pick(boxID(f.lastAsk()), 0), "remove:2", "remove:3", "add:700328@2 flag 0", "add:700462@3 flag 0",
		"lockout:2", "box:*It is done*")
	if f.w.StartCast(f.sess, spell, 0xFFFFFFFF, time.Now()) {
		t.Error("Expansive Mind given back was cast within its lockout")
	}
}

// effects is the player's effects' names, joined.
func (f *pathRig) effects() string {
	var names []string
	for _, e := range f.p.Effects {
		names = append(names, e.Name)
	}
	return strings.Join(names, ", ")
}

// A book with no room for the path's spells takes none: said, nothing moved,
// before anything comes out. The control: room for both, both go in.
func TestTheArchivistsPathNeedsRoom(t *testing.T) {
	f := newPathRig(t, 45)
	if _, err := f.db.Exec(`UPDATE spells SET addedorder = 250 WHERE serverid = ? AND spellid = ?`, f.p.CharServerID, spArcaneBolt); err != nil {
		t.Fatal(err)
	}
	_, box := f.paths(encAnswers...)
	f.wantMsgs("a book to 250", f.pick(box, 0), "box:Spell Archivist: Your book has no room for Expansive Mind and Limitless Mind.")
	f.wantBook("a book to 250", "1623@0/0", "6101@250/1")
	if _, err := f.db.Exec(`UPDATE spells SET addedorder = 249 WHERE serverid = ? AND spellid = ?`, f.p.CharServerID, spArcaneBolt); err != nil {
		t.Fatal(err)
	}
	_, box = f.paths(encAnswers...)
	f.wantMsgs("a book to 249", f.pick(box, 0), "add:700328@250 flag 0", "add:700462@251 flag 0", "box:*")
}

// His path words fit CLIENT's boxes: the questions 127 characters, the answers
// 63 (every path's of every class), a box 217 with his name, at their longest
// (the joined path lines are cut between sentences: splitSay).
func TestTheArchivistsPathWordsFit(t *testing.T) {
	for _, q := range []string{archivistPathAsk, archivistPathAskSwap} {
		if n := len([]rune(q)); n > questAskChars {
			t.Errorf("%q is %d characters, a question shows %d", q, n, questAskChars)
		}
	}
	answers := []string{archivistPathNeither, archivistPathYes, archivistPathNo}
	for _, lines := range archivistPathLines {
		if len(lines)+1 > maxDialogOptions {
			t.Errorf("%d paths and Neither: a box holds %d", len(lines), maxDialogOptions)
		}
		for _, line := range lines {
			answers = append(answers, fmt.Sprintf(archivistPathAnswer, line[0]))
		}
	}
	for _, a := range answers {
		if n := len([]rune(a)); n > questAnswerChars {
			t.Errorf("%q is %d characters, an answer shows %d", a, n, questAnswerChars)
		}
	}
	long := "Gleaming Bastion, Hallowed Bastion, Sacred Bastion and Blessed Bastion"
	for _, s := range []string{fmt.Sprintf(archivistPathsTooLow, 49), archivistNoPaths, fmt.Sprintf(archivistPathHeld, "Moradhim Cocktail"),
		fmt.Sprintf(archivistPathsHeld, "Gleaming Bastion, Secure and Marble Barrier"), fmt.Sprintf(archivistPathHasAll, "Moradhim Cocktail"),
		fmt.Sprintf(archivistPathGiven, long, "Gleaming Bastion"), archivistPathCasting, fmt.Sprintf(archivistPathNoRoom, long),
		archivistPathsChanged, fmt.Sprintf(archivistPathEnded, "Expansive Mind, Limitless Mind, Boundless Mind and Infinite Mind")} {
		if n := len([]rune(SpellArchivistName + ": " + s)); n > questBoxChars {
			t.Errorf("%q is %d characters with his name, a box shows %d", s, n, questBoxChars)
		}
	}
	if n := len([]rune(archivistQuestion)); n > questAskChars {
		t.Errorf("his question is %d characters, a question shows %d", n, questAskChars)
	}
	if len(archivistAnswers) > maxDialogOptions {
		t.Errorf("%d answers, a box holds %d", len(archivistAnswers), maxDialogOptions)
	}
}

// A GM's edit of the quests between his question and its answer (the level
// 30 and 40 rewards taken out of the game): the answer does nothing but say
// so, the book as it was, though its index still names a path. The control:
// asked again, the paths as they are now, the self power line named by its
// first spell left.
func TestTheArchivistsPathIsTheOneOffered(t *testing.T) {
	f := newPathRig(t, 49)
	_, box := f.paths(encAnswers...)
	if _, err := f.db.Exec(`UPDATE quests SET enabled = 0 WHERE questid IN (?, ?, ?, ?)`, qEnc30, qEnc40, qEnc30Evil, qEnc40Evil); err != nil {
		t.Fatal(err)
	}
	if msg := f.qe.ReloadQuests(); !strings.HasPrefix(msg, "at once") {
		t.Fatalf("reload: %s", msg)
	}
	f.wantMsgs("the old question", f.pick(box, 0), "box:Spell Archivist: "+archivistPathsChanged)
	f.wantBook("the old question", "1623@0/0", "6101@1/1")
	got, _ := f.paths("The path of Boundless Mind", archivistPathNeither)
	f.wantMsgs("asked again", got, "box:Spell Archivist: The path of Boundless Mind: Boundless Mind and Infinite Mind.",
		"ask#*:"+archivistPathAsk+"|The path of Boundless Mind|"+archivistPathNeither)
}

// Petition 1361: the level 20 class quest's pick is its path's first spell
// (the path rule: archivistside.go), so "Grant path spells" gives it from
// level 20: at 25 each path is its level 20 scroll's spell, read off the scroll
// as the scribe reads it, the one picked written at once; at 45 a change takes
// it out with the rest of its path and gives the other path's, its level 20
// first. The controls: at 19 nothing is offered; a city's level 20 that kept
// one path (its scroll handed over with no choice) makes no spell every
// path's; "Grant basic spells" gives neither pick, only the merchants' scroll.
func TestTheArchivistsPathsBeginAtTheLevel20Pick(t *testing.T) {
	const spSpacious, spBoon = 700215, 700207
	f := newPathRig(t, 25)
	f.spell(spSpacious, "Spacious Mind", 20, "Self, Increase Maximum Power by 160, Power by 110, 20min")
	f.spell(spBoon, "Power Boon", 20, "Target, Increase Power by 100")
	spacious, boon := f.scroll("Spacious Mind", 1<<classEnc, 20), f.scroll("Power Boon", 1<<classEnc, 20)
	f.quest(100100, "1-20/qeynos/enchanter/L20", choice(fmt.Sprintf("Spacious Mind=:%d", spacious), fmt.Sprintf("Power Boon=:%d", boon)))
	f.sideQuest(100101, "1-20/fayspire/enchanter/L20", true, "12", 20, "", fmt.Sprintf("%d*1", spacious), "", "")
	if msg := f.qe.ReloadQuests(); !strings.HasPrefix(msg, "at once") {
		t.Fatalf("reload: %s", msg)
	}

	got, box := f.paths(encAnswers...)
	f.wantMsgs("the paths at 25", got, "box:Spell Archivist: The path of Expansive Mind: Spacious Mind. The path of Power Jet: Power Boon.", encAsked)
	f.wantMsgs("self power at 25", f.pick(box, 0), "add:700215@2 flag 0",
		"box:Spell Archivist: There: Spacious Mind written into your book. The path of Expansive Mind is yours.")

	f.p.Level = 45
	_, box = f.paths(encAnswers...)
	f.wantMsgs("power for others asked at 45", f.pick(box, 1),
		"box:Spell Archivist: For the path of Power Jet, Spacious Mind leaves your book, and Power Boon, Power Jet and Power Surge enter it.",
		swapAsked)
	f.wantMsgs("power for others at 45", f.pick(boxID(f.lastAsk()), 0), "remove:2", "add:700207@2 flag 0", "add:700340@3 flag 0",
		"add:700468@4 flag 0", "box:*It is done: the path of Power Jet is yours now.*")
	f.wantBook("power for others at 45", "1623@0/0", "6101@1/1", bookRow(spBoon, 2), bookRow(spPowerJet, 3), bookRow(spPowerSurge, 4))

	f.wantMsgs("the basic grant at 45", f.grant(), "add:6102@5 flag 0",
		"box:Spell Archivist: "+fmt.Sprintf(archivistGranted, spellCount(1), 45))

	f.p.Level = 19
	f.wantMsgs("at 19", f.pick(f.menu(), archivistPath), "box:Spell Archivist: "+fmt.Sprintf(archivistPathsTooLow, 20))
}
