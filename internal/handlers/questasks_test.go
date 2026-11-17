package handlers

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// askLine is the msgs' last question box ("ask#id:question|a|b"), "" none.
func askLine(m []string) string {
	line := ""
	for _, s := range m {
		if strings.HasPrefix(s, "ask#") {
			line = s[strings.Index(s, ":")+1:]
		}
	}
	return line
}

// pickLabel answers the msgs' question with the answer labelled label.
func (f *questFixture) pickLabel(m []string, label string) []string {
	f.t.Helper()
	opts := strings.Split(askLine(m), "|")[1:]
	for i, o := range opts {
		if o == label {
			f.tw.pick(boxID(m), byte(i))
			return f.msgs()
		}
	}
	f.t.Fatalf("no answer %q in %q", label, askLine(m))
	return nil
}

// Work entry 1597: questions at an offer. The offer's box has the accept, the
// decline and the open asks; an ask is answered and the offer asked again, it
// out and its follow-up in; nothing is kept; the accept still takes it.
func TestAQuestOffersQuestions(t *testing.T) {
	f := newQuestFixture(t)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, offer, accept, decline, started, asks)
		VALUES (102901, 'Gifts for Freeport', 1, 'Holiday Elf', 1, 60, 'Time skipped, %p.' || char(10) || 'Will you carry a few for me?',
			'I will carry them.', 'Not now.', 'Bless you!',
			'[{"answer": "Timeskipped? What do you mean?", "says": "Just that!\nNo one knows why.", "asks": [{"answer": "Why once a day?", "says": "FOMO."}]}]')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (102901, 1, 'talk', 'Tailor Bariston', 'Bring a gift to Tailor Bariston.')`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	elf := f.tw.add(11, &entity.NPC{ServerID: 811, Name: "Holiday Elf", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	f.tw.talk(elf)
	m := f.msgs()
	if got := askLine(m); got != "Will you carry a few for me?|I will carry them.|Not now.|Timeskipped? What do you mean?" {
		t.Fatalf("the offer: %q", got)
	}
	m = f.pickLabel(m, "Timeskipped? What do you mean?")
	if !hasMsg(m, "box:Holiday Elf: Just that!") || !hasMsg(m, "box:Holiday Elf: No one knows why.") {
		t.Fatalf("the reply: %q", m)
	}
	if got := askLine(m); got != "Will you carry a few for me?|I will carry them.|Not now.|Why once a day?" {
		t.Fatalf("asked again, the follow-up in: %q", got)
	}
	if s, _ := f.progress(102901); s != -1 {
		t.Fatalf("an ask kept something: step %d", s)
	}
	m = f.pickLabel(m, "Why once a day?")
	if got := askLine(m); got != "Will you carry a few for me?|I will carry them.|Not now." {
		t.Fatalf("both asked: %q", got)
	}
	f.pickLabel(m, "I will carry them.")
	if s, _ := f.progress(102901); s != 1 {
		t.Fatalf("taken: step %d", s)
	}
	// a fresh offer starts with none asked
	f.exec(`DELETE FROM charquests`)
	f.qe.Stop()
}

// Work entry 1597: a day group's quests are one a day: one finished today
// keeps the others from being offered; the next day the giver offers the
// next (fewest finished first).
func TestADayGroupIsOneADay(t *testing.T) {
	f := newQuestFixture(t)
	for _, q := range []string{
		`(102901, 'Gifts for Freeport', 0, '', 'christmas-gifts')`,
		`(102904, 'More Gifts', 2, '102901', 'christmas-gifts')`,
		`(102905, 'Still More Gifts', 2, '102901', 'christmas-gifts')`,
	} {
		f.exec(`INSERT INTO quests (questid, name, repeatable, requires, daygroup, enabled, giver, minlevel, maxlevel, offer, accept) VALUES ` +
			strings.Replace(q, "(", "(", 1)[:len(q)-1] + `, 1, 'Holiday Elf', 1, 60, 'Will you?', 'I will.')`)
	}
	for _, id := range []string{"102901", "102904", "102905"} {
		f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (` + id + `, 1, 'talk', 'Holiday Elf', 'Talk to the elf.')`)
	}
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	elf := f.tw.add(11, &entity.NPC{ServerID: 811, Name: "Holiday Elf", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	take := func() {
		t.Helper()
		f.tw.talk(elf)
		f.pickLabel(f.msgs(), "I will.")
		f.tw.talk(elf) // the step: talk to the elf
		f.msgs()
	}
	take()
	if _, d := f.progress(102901); d != 1 {
		t.Fatalf("102901 not finished")
	}
	f.tw.talk(elf)
	if m := f.msgs(); askLine(m) != "" {
		t.Fatalf("a run offered the day 102901 was finished: %q", m)
	}
	// the next day
	yesterday := func() { f.exec(`UPDATE charquests SET updated = '2000-01-01 00:00:00'`) }
	f.qe.Stop()
	yesterday()
	qe, err := NewQuestEngine(f.db, f.tw.w, f.tw.mgr, f.inv, nil)
	if err != nil {
		t.Fatal(err)
	}
	qe.Start()
	t.Cleanup(qe.Stop)
	f.qe = qe
	take()
	if _, d := f.progress(102904); d != 1 {
		t.Fatalf("the next day: run 102904 not taken and finished")
	}
	f.tw.talk(elf)
	if m := f.msgs(); askLine(m) != "" {
		t.Fatalf("a second run offered the same day: %q", m)
	}
}
