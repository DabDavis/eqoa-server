package handlers

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A new character's first-login welcome and merchant question (cityintro.go,
// work board 825) driven as a server drives them: the World's entry hook, the
// box's 0x0035 through HandleDialogueOption, the timers on the rig's clock, a
// merchant window's 0x004C. The words of the rows are made up.

type introRig struct {
	*welcomeRig
	c *CityIntro
}

func newIntroRig(t *testing.T) *introRig {
	t.Helper()
	w := newWelcomeRig(t)
	SetWelcomeGuide(false) // the guide's own box is the welcome guide's tests'
	r := &introRig{welcomeRig: w}
	c := NewCityIntro(w.gw, w.db)
	c.now = func() time.Time { return w.now }
	c.later = func(d time.Duration, f func()) { w.due = append(w.due, welcomeTimer{at: w.now.Add(d), f: f}) }
	r.c, w.ih.intro = c, c
	w.gw.OnPlayerEntered(func(s *session.Session, p *entity.Player) { r.c.Entered(s, p) })
	// Anna's start place (human warrior, class 0), and a rogue's beside it
	for _, q := range []string{
		`INSERT INTO cityintro (world, x, z, radius, race, class, boxes, yes_last, no_last, source)
			VALUES (0, 5010, 5000, 60, 0, 0, 'Welcome to Testhold young warrior.' || char(10) || 'You have joined the Test Guard.',
			'You are now ready. Speak to Captain Test.', 'Then you are ready. Speak to Captain Test.', 'video-audit')`,
		`INSERT INTO cityintro (world, x, z, radius, race, class, boxes, yes_last, no_last)
			VALUES (0, 5000, 5000, 60, -1, 6, 'Welcome young rogue.', 'Y', 'N')`,
	} {
		if _, err := w.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

// The welcome goes out introAfter after a new character's first entry: its
// row's boxes, then the controls question; Yes says the shared control boxes
// and the row's last; it is never said again (a second entry, a restart). The
// controls (no welcome): a character with experience, one far from any start
// place, one in another world, and a class no row names near it.
func TestCityIntroWelcomesANewCharacterOnce(t *testing.T) {
	r := newIntroRig(t)
	r.pc.TotalXP = 10            // Cora has played
	r.pb.Class = 0               // Bert stands far off
	r.pd.Class, r.pd.X = 0, 5000 // Dora is in world 2
	r.gw.PlayerEntered(r.anna)
	r.none("Anna, at the entry itself", r.anna)
	for _, s := range []*session.Session{r.cora, r.bert, r.dora} {
		r.gw.PlayerEntered(s)
	}
	r.run(introAfter)
	got := r.msgs(r.anna)
	box := boxID(got)
	want := []string{"box:Welcome to Testhold young warrior.", "box:You have joined the Test Guard.",
		fmt.Sprintf("ask#%d:%s|%s|%s", box, introControlsAsk, introYes, introNo)}
	if strings.Join(got, "\n") != strings.Join(want, "\n") || !IsIntroBox(box) || box&0xFF000000 != 0x7A000000 {
		t.Fatalf("Anna was sent %q, want %q", got, want)
	}
	r.none("Cora (has experience)", r.cora)
	r.none("Bert (no start place near)", r.bert)
	r.none("Dora (another world)", r.dora)

	r.pick(r.anna, box, 0)
	got = r.msgs(r.anna)
	if len(got) != len(introControls)+1 || got[len(got)-1] != "box:You are now ready. Speak to Captain Test." ||
		got[0] != "box:"+introControls[0] {
		t.Fatalf("Yes: %q", got)
	}
	r.pick(r.anna, box, 0) // the same box again: nothing
	r.none("Anna, an old box", r.anna)

	r.gw.PlayerEntered(r.anna)
	r.run(introAfter)
	r.none("Anna, a second entry", r.anna)
	r.restart()
	r.c = NewCityIntro(r.gw, r.db)
	r.c.later = func(d time.Duration, f func()) { r.due = append(r.due, welcomeTimer{at: r.now.Add(d), f: f}) }
	r.gw.PlayerEntered(r.anna)
	r.run(introAfter)
	r.none("Anna, after a restart", r.anna)
}

// No says the row's no_last alone. A class's own row goes before a race's: a
// rogue standing between both rows' centres gets the rogue's.
func TestCityIntroNoAndTheClassesRow(t *testing.T) {
	r := newIntroRig(t)
	r.pa.Class = 6
	r.gw.PlayerEntered(r.anna)
	r.run(introAfter)
	got := r.msgs(r.anna)
	if len(got) != 2 || got[0] != "box:Welcome young rogue." {
		t.Fatalf("the rogue was sent %q", got)
	}
	r.pick(r.anna, boxID(got), 1)
	r.want("Anna, No", r.msgs(r.anna), "box:N")
}

// A box CLIENT declines (a 255 at once: another box up) goes out again, the
// question alone; a 255 later is the player's Exit, and the welcome stays
// taken. While the character is loading a world the welcome waits.
func TestCityIntroDeclinedAndWaiting(t *testing.T) {
	r := newIntroRig(t)
	r.anna.HoldStateChannels(time.Minute)
	r.gw.PlayerEntered(r.anna)
	r.run(introAfter)
	r.none("Anna, loading", r.anna)
	r.anna.ReleaseStateChannels()
	r.run(introRetry)
	got := r.msgs(r.anna)
	box := boxID(got)
	if len(got) != 3 {
		t.Fatalf("after the load: %q", got)
	}
	r.pick(r.anna, box, 255)
	r.none("Anna, just declined", r.anna)
	r.run(introRetry)
	got = r.msgs(r.anna)
	if len(got) != 1 || boxID(got) != box {
		t.Fatalf("the declined box again: %q, want the question alone", got)
	}
	r.run(introDeclined + time.Second)
	r.pick(r.anna, box, 255) // the player's Exit
	r.run(introRetry)
	r.none("Anna, after her Exit", r.anna)
	ci, err := r.db.CharIntroOf(r.pa.CharServerID)
	if err != nil || !ci.Welcomed {
		t.Fatalf("charintro: %+v, %v", ci, err)
	}
}

// The merchant question: a welcomed character's first merchant window asks it
// instead of opening (the request cleared), Yes says ours, and the next window
// opens the shop. The control: a character never welcomed opens the shop at
// once.
func TestCityIntroMerchantQuestion(t *testing.T) {
	f := newArchivistFixture(t, nil)
	f.ih.intro = NewCityIntro(f.w, f.db)
	f.windows.intro = f.ih.intro
	const object = 100002
	f.stand("Merchant Test", 4343, npcFlagMerchant, object)

	got := f.open(object)
	if len(got) != 1 || !strings.HasPrefix(got[0], "shop:") {
		t.Fatalf("not welcomed: %q, want the shop", got)
	}
	if _, err := f.db.Exec(`INSERT INTO charintro (serverid, welcomed) VALUES (?, '2026-11-22T00:00:00Z')`,
		f.p.CharServerID); err != nil {
		t.Fatal(err)
	}
	got = f.open(object)
	box := boxID(got)
	want := fmt.Sprintf("ask#%d:%s|%s|%s", box, introMerchantAsk, introYes, introNo)
	if len(got) != 2 || got[0] != want || got[1] != "op:00D7" || !IsIntroBox(box) {
		t.Fatalf("the first window: %q, want %q and 0x00D7", got, want)
	}
	got = f.pick(box, 0)
	if len(got) != len(introMerchantYes) || got[0] != "box:"+introMerchantYes[0] {
		t.Fatalf("Yes: %q", got)
	}
	got = f.open(object)
	if len(got) != 1 || !strings.HasPrefix(got[0], "shop:") {
		t.Fatalf("the next window: %q, want the shop", got)
	}
}

// The row check holds what a PS2 can show.
func TestCityIntroRowsAreRead(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/intro.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO cityintro (world, x, z, radius, race, class, boxes) VALUES
		(0, 100, 100, 50, 4, 6, 'a' || char(10) || '' || char(10) || ' b '), (0, 100, 100, 50, 4, -1, 'any')`); err != nil {
		t.Fatal(err)
	}
	c, ok, err := db.CityIntroFor(0, 4, 6, 120, 100)
	if err != nil || !ok || strings.Join(c.Boxes, "/") != "a/b" {
		t.Fatalf("rogue: %+v %v %v", c, ok, err)
	}
	if c, ok, _ = db.CityIntroFor(0, 4, 2, 120, 100); !ok || c.Boxes[0] != "any" {
		t.Fatalf("paladin: %+v %v", c, ok)
	}
	if _, ok, _ = db.CityIntroFor(0, 4, 6, 200, 100); ok {
		t.Fatal("outside the radius")
	}
	if _, ok, _ = db.CityIntroFor(0, 1, 6, 120, 100); ok {
		t.Fatal("another race")
	}
	_ = protocol.OpcodeQuestPopupMulti
}
