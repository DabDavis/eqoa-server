package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// wereFixture is a level 45 character with 30,000 tunar, the wolf's and the
// bear's lines (each its fever's marker, a draft no one is offered, and its
// third quest) and the werehunters' brand, on a running quest engine, by
// their import keys as tools/import_quests.py writes them.
type wereFixture struct {
	t   *testing.T
	tw  *talkWorld
	db  *database.SQLiteDB
	qe  *QuestEngine
	sid uint32
}

const (
	wolfFever, wolfTurned, bearFever, bearTurned, hunterBrand = 990101, 990102, 990103, 990104, 990105
)

func newWereFixture(t *testing.T) *wereFixture {
	t.Helper()
	tw := newTalkWorld(t, 23431, 19601)
	db, err := database.NewSQLite(t.TempDir() + "/were.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	rec := &database.CharacterRecord{CharName: "Talker", AccountID: 1, Level: 45}
	if err := store.Create(rec); err != nil {
		t.Fatal(err)
	}
	tw.p.CharServerID, tw.p.Level, tw.p.Tunar = rec.ServerID, 45, 30000
	f := &wereFixture{t: t, tw: tw, db: db, sid: rec.ServerID}
	for _, q := range []struct {
		id               int
		key, name, giver string
		on               int
	}{
		{wolfFever, "Weres/wolfwere/infection", "The Wolf's Fever", "", 0},
		{wolfTurned, "Weres/wolfwere/p3", "Duskrender's Trial", "Edward Duskrender", 1},
		{bearFever, "Weres/bearwere/infection", "The Bear's Fever", "", 0},
		{bearTurned, "Weres/bearwere/p3", "Mirin", "Dorandas", 1},
		{hunterBrand, "Weres/werehunter/brand", "The Brand", "Levandius", 1},
	} {
		f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, classes, source)
			VALUES (?, ?, ?, ?, 1, 60, '0', ?)`, q.id, q.name, q.on, q.giver, "EQOA-Data "+q.key+" (tools/import_quests.py)")
		if q.giver != "" {
			f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (?, 1, 'talk', ?, 'Talk.')`, q.id, q.giver)
		}
	}
	qe, err := NewQuestEngine(db, tw.w, tw.mgr, database.NewInventoryStore(db), database.NewSpellbookStore(db))
	if err != nil {
		t.Fatal(err)
	}
	qe.Start()
	t.Cleanup(qe.Stop)
	f.qe = qe
	tw.h.spend = func(_ *session.Session, p *entity.Player, n int) bool {
		if n > p.Tunar {
			return false
		}
		p.Tunar -= n
		return true
	}
	return f
}

func (f *wereFixture) exec(q string, args ...any) {
	f.t.Helper()
	if _, err := f.db.Exec(q, args...); err != nil {
		f.t.Fatal(err)
	}
}

// done is whether the character has finished the quest (its row).
func (f *wereFixture) done(id int) bool {
	var n int
	f.db.QueryRow(`SELECT COUNT(*) FROM charquests WHERE serverid = ? AND questid = ? AND done > 0`, f.sid, id).Scan(&n)
	return n > 0
}

func (f *wereFixture) set(id int, action string) {
	f.t.Helper()
	if _, err := f.qe.SetCharacterQuest(f.sid, id, action, 0); err != nil {
		f.t.Fatal(err)
	}
}

// bite is a bite of the line's were that infects, as the World hears it;
// what the character was sent.
func (f *wereFixture) bite(line string) []string {
	questMsgs(f.tw.s)
	f.qe.infect(entity.InfectionHit{Session: f.tw.s, ServerID: f.sid, Name: "Talker", Line: line, NPCName: "Elder " + line})
	return questMsgs(f.tw.s)
}

// A bite gives its line's fever and says so; one fever at a time: another
// line's bite takes nothing while it lasts, and takes once it is gone.
func TestABiteGivesItsLinesFever(t *testing.T) {
	f := newWereFixture(t)
	got := f.bite("wolfwere")
	if !f.done(wolfFever) || !hasMsg(got, "chat:A fever burns where the elder wolfwere's claws found you.") {
		t.Fatalf("the wolf's bite: fever %v, sent %q", f.done(wolfFever), got)
	}
	if got := f.bite("bearwere"); f.done(bearFever) || len(got) != 0 {
		t.Errorf("a bear's bite on the wolf's fever: fever %v, sent %q", f.done(bearFever), got)
	}
	f.set(wolfFever, QuestReset)
	if f.bite("bearwere"); !f.done(bearFever) || f.done(wolfFever) {
		t.Errorf("a bear's bite once the wolf's fever is gone: bear %v, wolf %v", f.done(bearFever), f.done(wolfFever))
	}
}

// The quest engine takes a bite off the World's hearers as it takes a kill:
// queued, then given on its own goroutine.
func TestABiteHeardIsTaken(t *testing.T) {
	f := newWereFixture(t)
	f.qe.heardInfection(entity.InfectionHit{Session: f.tw.s, ServerID: f.sid, Name: "Talker", Line: "wolfwere", NPCName: "wolfwere"})
	for end := time.Now().Add(2 * time.Second); !f.done(wolfFever) && time.Now().Before(end); {
		time.Sleep(10 * time.Millisecond)
	}
	if !f.done(wolfFever) {
		t.Error("a heard bite gave no fever")
	}
}

// A werehunter's brand and a form already (a line's third quest finished)
// keep a bite from taking. The control is TestABiteGivesItsLinesFever's.
func TestABrandOrAFormKeepsABiteFromTaking(t *testing.T) {
	for name, id := range map[string]int{"the brand": hunterBrand, "the bear's form": bearTurned} {
		f := newWereFixture(t)
		f.set(id, QuestComplete)
		if got := f.bite("wolfwere"); f.done(wolfFever) || len(got) != 0 {
			t.Errorf("%s: the wolf's bite took (%v), sent %q", name, f.done(wolfFever), got)
		}
	}
}

// The campfire: an advisor says what the character carries and offers a
// fever's cure for 25,000 tunar; "Not now." keeps it, too little tunar is
// refused with nothing taken, and the cure takes the price and the fever. A
// line's own advisor sees its line's fever alone; a turned were is told no
// cure reaches it, and offered none.
func TestTheCampfireCuresAFever(t *testing.T) {
	f := newWereFixture(t)
	gerahl := f.tw.add(1, &entity.NPC{ServerID: 3001, Name: "Advisor Gerahl", NPCType: 0x80, SpawnX: 23431.5, SpawnY: 54, SpawnZ: 19601.5})
	nilarha := f.tw.add(2, &entity.NPC{ServerID: 3002, Name: "Advisor Nilarha", NPCType: 0x80, SpawnX: 23431.5, SpawnY: 54, SpawnZ: 19600.5})
	talk := func(id uint32) []string {
		t.Helper()
		f.tw.talk(id)
		return questMsgs(f.tw.s)
	}
	pick := func(msgs []string, n byte) []string {
		t.Helper()
		id := boxID(msgs)
		if id == 0 {
			t.Fatalf("no question in %q", msgs)
		}
		f.tw.pick(id, n)
		return questMsgs(f.tw.s)
	}
	says := func(msgs []string, part string) bool {
		for _, m := range msgs {
			if strings.HasPrefix(m, "box:") && strings.Contains(m, part) {
				return true
			}
		}
		return false
	}

	if got := talk(gerahl); !says(got, "Advisor Gerahl: You carry no were's fever.") || boxID(got) != 0 {
		t.Fatalf("no fever: %q", got)
	}
	f.bite("wolfwere")
	got := talk(gerahl)
	if !says(got, "You carry the wolf's fever. I can draw it out of you for 25,000 tunar.") ||
		!hasMsg(got, "ask#*") || !strings.HasSuffix(got[len(got)-1], ":Be cured of the wolf's fever for 25,000 tunar?|Cure me.|Not now.") {
		t.Fatalf("the wolf's fever at Gerahl's: %q", got)
	}
	if after := pick(got, 1); !says(after, "As you wish.") || !f.done(wolfFever) || f.tw.p.Tunar != 30000 {
		t.Fatalf("Not now: %q, fever %v, purse %d", after, f.done(wolfFever), f.tw.p.Tunar)
	}
	f.tw.p.Tunar = 20000
	if after := pick(talk(gerahl), 0); !says(after, "The cure costs 25,000 tunar.") || !f.done(wolfFever) || f.tw.p.Tunar != 20000 {
		t.Fatalf("too little tunar: %q, fever %v, purse %d", after, f.done(wolfFever), f.tw.p.Tunar)
	}
	f.tw.p.Tunar = 30000
	got = talk(gerahl)
	if after := pick(got, 0); !says(after, "The wolf's fever leaves your blood.") || f.done(wolfFever) || f.tw.p.Tunar != 5000 {
		t.Fatalf("the cure: %q, fever %v, purse %d", after, f.done(wolfFever), f.tw.p.Tunar)
	}
	f.tw.pick(boxID(got), 0) // the same box again: nothing
	if after := questMsgs(f.tw.s); len(after) != 0 || f.tw.p.Tunar != 5000 {
		t.Errorf("an answered box answered again: %q, purse %d", after, f.tw.p.Tunar)
	}

	f.bite("bearwere")
	if got := talk(nilarha); !says(got, "You carry no wolf's fever.") || boxID(got) != 0 {
		t.Errorf("the bear's fever at the wolf's advisor: %q", got)
	}
	if got := talk(gerahl); !says(got, "You carry the bear's fever.") {
		t.Errorf("the bear's fever at Gerahl's: %q", got)
	}
	f.set(bearTurned, QuestComplete)
	if got := talk(gerahl); !says(got, "The bear has already claimed your blood. No cure reaches what has turned.") || boxID(got) != 0 {
		t.Errorf("a bear turned: %q", got)
	}
}

// Declining a line's third quest is its giver's antidote: that line's fever
// passes; another line's third quest declined leaves it.
func TestTheAntidoteTakesItsLinesFever(t *testing.T) {
	f := newWereFixture(t)
	f.bite("wolfwere")
	f.qe.mu.Lock()
	c := f.qe.charLocked(f.sid)
	f.qe.antidoteLocked(c, f.qe.cat.byID[bearTurned])
	bearDeclined := c.hasFinished(wolfFever)
	f.qe.antidoteLocked(c, f.qe.cat.byID[wolfTurned])
	wolfDeclined := c.hasFinished(wolfFever)
	f.qe.mu.Unlock()
	if !bearDeclined {
		t.Errorf("the bear's antidote took the wolf's fever")
	}
	if wolfDeclined || f.done(wolfFever) {
		t.Errorf("the wolf's antidote left its fever (engine %v, row %v)", wolfDeclined, f.done(wolfFever))
	}
}
