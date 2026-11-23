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

// wereFixture is a level 45 character with 30,000 tunar, the wolf's and the
// bear's lines (each its fever's marker, a draft no one is offered, and its
// third quest) and the werehunters' brand, on a running quest engine, by
// their import keys as tools/import_quests.py writes them.
type wereFixture struct {
	t    *testing.T
	tw   *talkWorld
	db   *database.SQLiteDB
	qe   *QuestEngine
	sid  uint32
	keys int
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
		id                         int
		key, name, giver, requires string
		on                         int
	}{
		{wolfFever, "Weres/wolfwere/infection", "The Wolf's Fever", "", "", 0},
		{wolfTurned, "Weres/wolfwere/p3", "Duskrender's Trial", "Edward Duskrender", "", 1},
		{bearFever, "Weres/bearwere/infection", "The Bear's Fever", "", "", 0},
		{bearTurned, "Weres/bearwere/p3", "Mirin", "Dorandas", "", 1},
		{hunterBrand, "Weres/werehunter/brand", "The Brand", "Levandius", "", 1},
	} {
		f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, classes, source, requires)
			VALUES (?, ?, ?, ?, 1, 60, '', ?, ?)`, q.id, q.name, q.on, q.giver, "EQOA-Data "+q.key+" (tools/import_quests.py)", q.requires)
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
	f.keys++
	f.qe.infect(entity.InfectionHit{Session: f.tw.s, ServerID: f.sid, Name: "Talker", Line: line, NPCName: "Elder " + line,
		At: time.Now(), Key: fmt.Sprintf("test/%d", f.keys)})
	return questMsgs(f.tw.s)
}

// A bite gives its line's fever and says so; one fever at a time: another
// line's bite takes nothing while it lasts, and takes once it is gone.
func TestABiteGivesItsLinesFever(t *testing.T) {
	f := newWereFixture(t)
	got := f.bite("wolfwere")
	if !f.done(wolfFever) || !hasMsg(got, "chat:A fever burns where the elder wolfwere's claws found you.") ||
		!hasMsg(got, "chat:You have contracted the were-wolf infection: 7 days, 0 hours left.") {
		t.Fatalf("the wolf's bite: fever %v, sent %q", f.done(wolfFever), got)
	}
	if got := f.bite("bearwere"); f.done(bearFever) || len(got) != 0 {
		t.Errorf("a bear's bite on the wolf's fever: fever %v, sent %q", f.done(bearFever), got)
	}
	f.qe.mu.Lock()
	f.qe.endLocked(f.qe.charLocked(f.sid), database.InfectionCured, time.Now())
	f.qe.mu.Unlock()
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

// row is the character's infection row as the database has it, nil for none.
func (f *wereFixture) row() *database.Infection {
	f.t.Helper()
	inf, err := database.NewQuestStore(f.db).CharInfection(f.sid)
	if err != nil {
		f.t.Fatal(err)
	}
	return inf
}

// shown is the infection on the player: its line and end.
func (f *wereFixture) shown() (string, time.Time) { return f.tw.w.Infection(f.tw.s) }

// fresh is a new engine on the same database: a restart's.
func (f *wereFixture) fresh() {
	f.t.Helper()
	f.qe.Stop()
	qe, err := NewQuestEngine(f.db, f.tw.w, f.tw.mgr, database.NewInventoryStore(f.db), database.NewSpellbookStore(f.db))
	if err != nil {
		f.t.Fatal(err)
	}
	f.qe = qe
	f.t.Cleanup(qe.Stop)
}

// The infection is its row (work 1463): a successful crit writes one active row
// of the line with a seven-day absolute end from the crit's time, the line's
// marker in the same step, and puts one harmful icon on the player named for the
// family; the effect ends with the row. Repeated delivery of the same grant
// (its key) writes nothing more.
func TestAnInfectionIsItsRowAndItsIcon(t *testing.T) {
	for _, line := range []string{"wolfwere", "bearwere", "ratwere", "lionwere", "gatorwere"} {
		f := newWereFixture(t)
		at := time.Now().Add(-time.Minute)
		f.qe.infect(entity.InfectionHit{Session: f.tw.s, ServerID: f.sid, Name: "Talker", Line: line, NPCName: "Elder " + line, At: at, Key: "k1"})
		inf := f.row()
		if inf == nil || inf.Line != line || !inf.Active(time.Now()) || inf.AppliedAt.Unix() != at.Unix() ||
			inf.ExpiresAt.Unix() != at.Add(7*24*time.Hour).Unix() || inf.Key != "k1" {
			t.Fatalf("%s: row %+v", line, inf)
		}
		got, until := f.shown()
		if got != line || until.Unix() != inf.ExpiresAt.Unix() {
			t.Errorf("%s: shown %q until %v, want the row's end %v", line, got, until, inf.ExpiresAt)
		}
		effs := f.tw.w.PlayerEffects(f.tw.s)
		if len(effs) != 1 || effs[0].Name != entity.InfectionName(line) || effs[0].Icon != entity.InfectionIcon {
			t.Errorf("%s: effects %+v", line, effs)
		}
		if marker := map[string]int{"wolfwere": wolfFever, "bearwere": bearFever}[line]; marker != 0 && !f.done(marker) {
			t.Errorf("%s: no marker", line)
		}
		// the same grant again: one row, still k1, one effect
		f.qe.infect(entity.InfectionHit{Session: f.tw.s, ServerID: f.sid, Name: "Talker", Line: line, NPCName: "Elder " + line, At: at, Key: "k1"})
		if again := f.row(); again.Key != "k1" || again.ExpiresAt != inf.ExpiresAt || len(f.tw.w.PlayerEffects(f.tw.s)) != 1 {
			t.Errorf("%s: a repeated grant changed it: %+v, %d effects", line, again, len(f.tw.w.PlayerEffects(f.tw.s)))
		}
	}
	// a failed roll is nothing heard: no row, no icon (entity's TestTheInfectionsChanceByCon rolls it)
	f := newWereFixture(t)
	if f.row() != nil || len(f.tw.w.PlayerEffects(f.tw.s)) != 0 {
		t.Error("an infection with no grant")
	}
}

// A grant whose keeping fails is kept for later and tried again with the same
// key, never rolled again: the table renamed away fails it, put back it takes.
func TestAFailedKeepingIsTriedAgain(t *testing.T) {
	f := newWereFixture(t)
	f.exec(`ALTER TABLE charinfection RENAME TO charinfection_away`)
	f.qe.infect(entity.InfectionHit{Session: f.tw.s, ServerID: f.sid, Name: "Talker", Line: "wolfwere", NPCName: "wolfwere", At: time.Now(), Key: "k9"})
	if line, _ := f.shown(); line != "" || f.done(wolfFever) {
		t.Fatalf("shown %q, marker %v before it was kept", line, f.done(wolfFever))
	}
	f.exec(`ALTER TABLE charinfection_away RENAME TO charinfection`)
	f.qe.retryInfections()
	if inf := f.row(); inf == nil || inf.Key != "k9" || !f.done(wolfFever) {
		t.Fatalf("the retry: %+v, marker %v", inf, f.done(wolfFever))
	}
	if line, _ := f.shown(); line != "wolfwere" {
		t.Errorf("the retry shows %q", line)
	}
	f.qe.retryInfections() // nothing left to try
	if n := len(f.qe.later.take()); n != 0 {
		t.Errorf("%d grants still waiting", n)
	}
}

// Seven days of real time from its catching, online or not: its end in the
// past ends the row expired, takes its marker and its icon off, and says so to
// one online; a login before its end puts it on to its end, one after it
// nothing. A restart (a new engine on the database) reads the same.
func TestAnInfectionEndsAfterSevenDays(t *testing.T) {
	f := newWereFixture(t)
	f.bite("wolfwere")
	// a restart: the row is the flag
	f.fresh()
	f.tw.w.TakeInfection(f.tw.s)
	f.qe.InfectionLogin(f.tw.s, f.tw.p)
	if line, until := f.shown(); line != "wolfwere" || until.Unix() != f.row().ExpiresAt.Unix() {
		t.Fatalf("after a restart's login: %q until %v", line, until)
	}
	// its end comes while online
	f.exec(`UPDATE charinfection SET expires_at = ? WHERE serverid = ?`, time.Now().Add(-time.Second).Unix(), f.sid)
	questMsgs(f.tw.s)
	f.qe.sweepInfections(time.Now())
	inf := f.row()
	if inf.EndedHow != database.InfectionExpired || f.done(wolfFever) {
		t.Errorf("at its end: %+v, marker %v", inf, f.done(wolfFever))
	}
	if !hasMsg(questMsgs(f.tw.s), "chat:The were-wolf infection has run its course.") {
		t.Error("its end not said")
	}
	if line, _ := f.shown(); line != "" {
		t.Errorf("still shown: %q", line)
	}
	// a login after its end: nothing
	f.qe.InfectionLogin(f.tw.s, f.tw.p)
	if line, _ := f.shown(); line != "" {
		t.Errorf("shown after its end: %q", line)
	}
	// offline: the sweep ends it with no one to tell
	g := newWereFixture(t)
	g.bite("bearwere")
	g.tw.s.SetCharacterInWorld(false)
	g.exec(`UPDATE charinfection SET expires_at = ? WHERE serverid = ?`, time.Now().Add(-time.Second).Unix(), g.sid)
	g.qe.sweepInfections(time.Now())
	if inf := g.row(); inf.EndedHow != database.InfectionExpired || g.done(bearFever) {
		t.Errorf("offline at its end: %+v, marker %v", inf, g.done(bearFever))
	}
}

// The third quest asks the infection, not the marker: an active infection
// offers it; a marker left with its infection over, or a marker of no infection
// at all once its migration is recorded, does not.
func TestTheFormsQuestAsksTheInfection(t *testing.T) {
	f := newWereFixture(t)
	f.exec(`UPDATE quests SET requires = ? WHERE questid = ?`, fmt.Sprint(wolfFever), wolfTurned) // as the live quests' 101346 requires 101422
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	why := func() string {
		f.qe.mu.Lock()
		defer f.qe.mu.Unlock()
		return f.qe.mayTakeLocked(nil, f.tw.p, f.qe.charLocked(f.sid), f.qe.cat.byID[wolfTurned])
	}
	if w := why(); w == "" {
		t.Fatal("the form's quest offered with no infection")
	}
	f.bite("wolfwere")
	if w := why(); w != "" {
		t.Fatalf("the form's quest with the infection: %q", w)
	}
	// the row's end passes; the marker is still there (no sweep yet)
	f.exec(`UPDATE charinfection SET expires_at = ? WHERE serverid = ?`, time.Now().Add(-time.Second).Unix(), f.sid)
	f.qe.mu.Lock()
	f.qe.charLocked(f.sid).infectionRead = false
	f.qe.mu.Unlock()
	if w := why(); w == "" {
		t.Error("the form's quest offered on an expired infection's marker")
	}
}

// The cure, the antidote and the form each end the infection at once: the row
// ended as each says, the marker and the icon gone together.
func TestCureAntidoteAndFormEndIt(t *testing.T) {
	for _, how := range []string{database.InfectionCured, database.InfectionAntidote, database.InfectionTurned} {
		f := newWereFixture(t)
		f.bite("wolfwere")
		f.qe.mu.Lock()
		c := f.qe.charLocked(f.sid)
		switch how {
		case database.InfectionCured:
			f.qe.endLocked(c, how, time.Now())
		case database.InfectionAntidote:
			f.qe.antidoteLocked(c, f.qe.cat.byID[wolfTurned])
		case database.InfectionTurned:
			f.qe.turnedLocked(c, f.qe.cat.byID[wolfTurned])
		}
		f.qe.mu.Unlock()
		inf := f.row()
		line, _ := f.shown()
		if inf.EndedHow != how || f.done(wolfFever) || line != "" {
			t.Errorf("%s: row %+v, marker %v, shown %q", how, inf, f.done(wolfFever), line)
		}
	}
}

// Migration: a fever marker held from before the table, with no row and no
// record, is migrated once at the character's first ask: from its timestamp
// (its end seven days after it), seven days from now with none, ended expired
// (the marker taken) when its seven days are over, the marker taken with no
// infection for a character already turned; a second ask (a restart) renews
// nothing; the migration row records what the marker was.
func TestFeverMarkersAreMigratedOnce(t *testing.T) {
	now := time.Now()
	for _, c := range []struct {
		name    string
		updated string
		turned  bool
		active  bool
		applied time.Time
	}{
		{"a marker of two days ago", now.Add(-48 * time.Hour).UTC().Format(time.RFC3339), false, true, now.Add(-48 * time.Hour)},
		{"a marker with no timestamp", "", false, true, now},
		{"a marker of eight days ago", now.Add(-8 * 24 * time.Hour).UTC().Format(time.RFC3339), false, false, now.Add(-8 * 24 * time.Hour)},
		{"a marker on a character turned", now.Add(-time.Hour).UTC().Format(time.RFC3339), true, false, now.Add(-time.Hour)},
	} {
		f := newWereFixture(t)
		f.exec(`INSERT INTO charquests (serverid, questid, step, count, done, slot, updated) VALUES (?, ?, 0, 0, 1, 0, ?)`, f.sid, wolfFever, c.updated)
		if c.turned {
			f.exec(`INSERT INTO charquests (serverid, questid, step, count, done, slot, updated) VALUES (?, ?, 0, 0, 1, 0, '')`, f.sid, bearTurned)
		}
		migrationStart := time.Now().Truncate(time.Second)
		f.fresh() // the engine reads the marker
		f.qe.InfectionLogin(f.tw.s, f.tw.p)
		inf := f.row()
		if inf == nil {
			t.Fatalf("%s: no row", c.name)
		}
		if inf.Active(time.Now()) != c.active || f.done(wolfFever) != c.active || inf.Source != "migration" {
			t.Errorf("%s: row %+v (active %v), marker %v; want active %v", c.name, inf, inf.Active(time.Now()), f.done(wolfFever), c.active)
		}
		if c.updated == "" {
			if inf.AppliedAt.Before(migrationStart) || inf.AppliedAt.After(time.Now()) {
				t.Errorf("%s: applied %v outside this migration", c.name, inf.AppliedAt)
			}
		} else if !inf.AppliedAt.Equal(c.applied.Truncate(time.Second)) {
			t.Errorf("%s: applied %v, want the saved timestamp %s", c.name, inf.AppliedAt, c.updated)
		}
		if line, _ := f.shown(); (line == "wolfwere") != c.active {
			t.Errorf("%s: shown %q", c.name, line)
		}
		var n int
		var outcome, before string
		f.db.QueryRow(`SELECT COUNT(*), MAX(outcome), MAX(marker_updated) FROM charinfection_migration WHERE serverid = ?`, f.sid).Scan(&n, &outcome, &before)
		if n != 1 || before != c.updated {
			t.Errorf("%s: %d migration rows (%q, before %q)", c.name, n, outcome, before)
		}
		// again: nothing renewed
		end := inf.ExpiresAt
		f.fresh()
		f.qe.InfectionLogin(f.tw.s, f.tw.p)
		if again := f.row(); !again.ExpiresAt.Equal(end) {
			t.Errorf("%s: a second start renewed it: %v -> %v", c.name, end, again.ExpiresAt)
		}
		f.db.QueryRow(`SELECT COUNT(*) FROM charinfection_migration WHERE serverid = ?`, f.sid).Scan(&n)
		if n != 1 {
			t.Errorf("%s: %d migration rows after a second start", c.name, n)
		}
	}
}

// An infection caught before the werehunters' brand ends with the brand at the
// next showing (a login, a keep restart): no effect, the row ended "brand"
// (Heirloom petition 2619). The control: the same infection without the brand
// is shown.
func TestTheBrandEndsAnInfectionCaughtBefore(t *testing.T) {
	for _, branded := range []bool{false, true} {
		f := newWereFixture(t)
		f.qe.infect(entity.InfectionHit{Session: f.tw.s, ServerID: f.sid, Name: "Talker", Line: "lionwere", NPCName: "Elder lionwere", At: time.Now().Add(-time.Hour), Key: "k1"})
		if branded {
			f.set(hunterBrand, QuestComplete)
		}
		shown := f.qe.show(f.sid, false)
		inf := f.row()
		switch {
		case !branded && (!shown || inf == nil || !inf.Active(time.Now())):
			t.Errorf("unbranded: shown %v, row %+v; want it carried", shown, inf)
		case branded && (shown || len(f.tw.w.PlayerEffects(f.tw.s)) != 0 || inf == nil || inf.EndedHow != database.InfectionBrand):
			t.Errorf("branded: shown %v, %d effects, row %+v; want it ended by the brand", shown, len(f.tw.w.PlayerEffects(f.tw.s)), inf)
		}
	}
}

// The same at a login or a keep restart's resumption (InfectionLogin), the way
// the character in petition 2619 met it: the branded one's infection ends and
// no effect is put on; the control, unbranded, carries it.
func TestTheBrandEndsAnInfectionAtTheLogin(t *testing.T) {
	for _, branded := range []bool{false, true} {
		f := newWereFixture(t)
		f.qe.infect(entity.InfectionHit{Session: f.tw.s, ServerID: f.sid, Name: "Talker", Line: "lionwere", NPCName: "Elder lionwere", At: time.Now().Add(-time.Hour), Key: "k1"})
		if branded {
			f.set(hunterBrand, QuestComplete)
		}
		f.tw.w.TakeInfection(f.tw.s)
		f.qe.InfectionLogin(f.tw.s, f.tw.p)
		inf, effs := f.row(), len(f.tw.w.PlayerEffects(f.tw.s))
		switch {
		case !branded && (effs != 1 || inf == nil || !inf.Active(time.Now())):
			t.Errorf("unbranded at the login: %d effects, row %+v; want it carried", effs, inf)
		case branded && (effs != 0 || inf == nil || inf.EndedHow != database.InfectionBrand):
			t.Errorf("branded at the login: %d effects, row %+v; want it ended by the brand", effs, inf)
		}
	}
}
