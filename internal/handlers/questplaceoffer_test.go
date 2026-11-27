package handlers

import (
	"reflect"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// questplaceoffer.go's tests: a quest offered at a place (Heirloom petitions
// 3322/3325, Envaric Ascension's altars; the owner 2026-11-27).

// placeFixture is the quest fixture with quest 500 offered at the player's
// spot (25403, 15725 in world 0, within 6), no giver, its engine's own ticks
// stopped (each look below is the test's: the player moves between them).
func placeFixture(t *testing.T, cols string, vals string) *questFixture {
	t.Helper()
	f := newQuestFixture(t)
	f.qe.Stop()
	questEngine.Store(f.qe)            // the talks' and answers' engine, its ticker stopped
	f.tw.p.HP, f.tw.p.MaxHP = 100, 100 // a dead player stands nowhere
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, offer, accept, decline, declined, started, events,
		offerworld, offerx, offery, offerz, offerradius` + cols + `)
		VALUES (500, 'Envaric Ascension', 1, '', 1, 60, 'The altar''s voice: four brooches, laid here.' || char(10) || 'Will you gather them?',
		'I will gather them.', 'Not now.', 'Then stay below.', 'Four brooches, from the tops of the towers.',
		'[{"on":"accept","do":"popup","text":"The altar hums."}]', 0, 25403, 54, 15725, 6` + vals + `)`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (500, 1, 'kill', 'grave usher', 'Kill a Grave Usher.')`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	return f
}

// at puts the player at x, z.
func (f *questFixture) at(x, z float32) { f.tw.p.X, f.tw.p.Z = x, z }

// placeOffered is whether msgs open quest 500's offer.
func placeOffered(msgs []string) bool {
	return hasMsg(msgs, "box:The altar's voice: four brooches, laid here.") &&
		strings.Contains(strings.Join(msgs, "\n"), ":Will you gather them?|I will gather them.|Not now.")
}

// Stepping onto the place offers the quest, its words with no name before
// them (a place has no voice: the original's pop-up); not again while the
// player stays; taken by its answer as from a giver: the row at step 1, the
// journal's line, the words once taken, "You received a quest!" and the
// quest's accept event. The controls: away from the place nothing is said, and
// once taken it is not offered again, stepping out and in.
func TestAQuestIsOfferedAtItsPlace(t *testing.T) {
	f := placeFixture(t, "", "")
	f.at(25500, 15725)
	f.qe.checkPlaceOffers()
	if m := f.msgs(); len(m) != 0 {
		t.Fatalf("offered 97 away: %q", m)
	}
	f.at(25405, 15727) // within 6
	f.qe.checkPlaceOffers()
	m := f.msgs()
	if !placeOffered(m) {
		t.Fatalf("not offered at the place: %q", m)
	}
	for _, s := range m {
		if strings.HasPrefix(s, "box:") && strings.Contains(s, ": The altar's voice") {
			t.Errorf("a place's words have a name before them: %q", s)
		}
	}
	f.qe.checkPlaceOffers()
	f.qe.checkPlaceOffers()
	if m := f.msgs(); len(m) != 0 {
		t.Fatalf("offered again while standing there: %q", m)
	}
	f.tw.pick(boxID(m), 0)
	m = f.msgs()
	f.want(m, "add:Kill a Grave Usher.", "box:Four brooches, from the tops of the towers.", "box:"+questReceived, "ask#*")
	if !strings.Contains(strings.Join(m, "\n"), ":The altar hums.|OK") {
		t.Errorf("the quest's accept event did not run: %q", m)
	}
	if s, d := f.progress(500); s != 1 || d != 0 {
		t.Fatalf("taken: step %d done %d, want 1 0", s, d)
	}
	f.at(25500, 15725)
	f.qe.checkPlaceOffers()
	f.at(25403, 15725)
	f.qe.checkPlaceOffers()
	if m := f.msgs(); placeOffered(m) {
		t.Errorf("offered while under way: %q", m)
	}
}

// A refusal leaves nothing (no row, the giver's declined words said); the
// quest is not offered again while the player stays, and is once it steps out
// and in again. A login there is a coming in too.
func TestAPlaceOffersAgainOnlyAfterLeaving(t *testing.T) {
	f := placeFixture(t, "", "")
	f.qe.checkPlaceOffers()
	m := f.msgs()
	if !placeOffered(m) {
		t.Fatalf("not offered: %q", m)
	}
	f.tw.pick(boxID(m), 1) // Not now.
	f.want(f.msgs(), "box:Then stay below.")
	if s, d := f.progress(500); s != -1 || d != -1 {
		t.Fatalf("a refusal left a row: step %d done %d", s, d)
	}
	for i := 0; i < 3; i++ {
		f.qe.checkPlaceOffers()
	}
	if m := f.msgs(); len(m) != 0 {
		t.Fatalf("offered again before leaving: %q", m)
	}
	f.at(25420, 15725) // 17 away: out
	f.qe.checkPlaceOffers()
	if m := f.msgs(); len(m) != 0 {
		t.Fatalf("offered outside: %q", m)
	}
	f.at(25403, 15725)
	f.qe.checkPlaceOffers()
	if m := f.msgs(); !placeOffered(m) {
		t.Fatalf("not offered again on coming back: %q", m)
	}
	f.qe.checkPlaceOffers()
	f.msgs()
	f.qe.LoggedIn(f.sid)
	f.qe.checkPlaceOffers()
	if m := f.msgs(); !placeOffered(m) {
		t.Fatalf("not offered at a login on the place: %q", m)
	}
}

// The giver's rules: a player of another class, or below the quest's level,
// is not offered it standing there; the level gained while it stands there, it
// is offered at the next look. One who has finished it is not offered it (one
// that may be done again is); a dead player is offered nothing.
func TestAPlaceOffersOnlyToWhoMayTakeIt(t *testing.T) {
	f := placeFixture(t, ", classes, minlevel", ", '10', 8")
	f.exec(`UPDATE quests SET minlevel = 8 WHERE questid = 500`)
	f.qe.Reload()
	f.tw.p.Class, f.tw.p.Level = 0, 20 // a warrior
	f.qe.checkPlaceOffers()
	if m := f.msgs(); len(m) != 0 {
		t.Fatalf("a warrior offered a magician's quest: %q", m)
	}
	f.tw.p.Class, f.tw.p.Level = 10, 5 // a magician too low
	f.qe.checkPlaceOffers()
	if m := f.msgs(); len(m) != 0 {
		t.Fatalf("a level 5 offered a level 8 quest: %q", m)
	}
	f.tw.p.Level = 8 // levelled there
	f.qe.checkPlaceOffers()
	if m := f.msgs(); !placeOffered(m) {
		t.Fatalf("not offered at its level: %q", m)
	}

	// finished: not offered; repeatable: offered
	if _, err := f.qe.SetCharacterQuest(f.sid, 500, QuestComplete, 0); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	f.at(25500, 15725)
	f.qe.checkPlaceOffers()
	f.at(25403, 15725)
	f.qe.checkPlaceOffers()
	if m := f.msgs(); placeOffered(m) {
		t.Fatalf("offered once finished: %q", m)
	}
	f.exec(`UPDATE quests SET repeatable = 1 WHERE questid = 500`)
	f.qe.Reload()
	f.qe.checkPlaceOffers()
	if m := f.msgs(); !placeOffered(m) {
		t.Fatalf("a repeatable one finished not offered: %q", m)
	}

	// dead: nothing (and where it stood stands)
	f.at(25500, 15725)
	f.qe.checkPlaceOffers()
	f.tw.p.HP = 0
	f.at(25403, 15725)
	f.qe.checkPlaceOffers()
	if m := f.msgs(); len(m) != 0 {
		t.Fatalf("a dead player offered: %q", m)
	}
}

// A place with a height asks the player's y too (a tower's floor): above it
// nothing; on it, the offer.
func TestAPlacesHeightIsAsked(t *testing.T) {
	f := placeFixture(t, ", offerheight", ", 3")
	f.tw.p.Y = 70
	f.qe.checkPlaceOffers()
	if m := f.msgs(); len(m) != 0 {
		t.Fatalf("offered 16 above the floor: %q", m)
	}
	f.tw.p.Y = 55
	f.qe.checkPlaceOffers()
	if m := f.msgs(); !placeOffered(m) {
		t.Fatalf("not offered on the floor: %q", m)
	}
}

// Taking a quest at its place is taking it from its giver: the same charquests
// row, the same journal, the same lines but for the giver's name before its
// words. Two fixtures, one quest each: one offered by its giver alone, one at a
// place (its giver kept, as Envaric Ascension's is).
func TestAPlaceAcceptIsAGiversAccept(t *testing.T) {
	type took struct {
		step, count, slot, done int
		journal                 []string
		msgs                    []string
	}
	run := func(place bool) took {
		f := newQuestFixture(t)
		f.qe.Stop()
		questEngine.Store(f.qe)
		f.tw.p.HP, f.tw.p.MaxHP = 100, 100
		f.exec(`INSERT INTO quests (questid, name, enabled, giver, offer, accept, decline, started, events) VALUES
			(501, 'The altar', 1, 'Altar', 'Lay them here.' || char(10) || 'Will you?', 'I will.', 'No.', 'Go to the towers.',
			'[{"on":"accept","do":"give","items":"` + itoa(f.bone) + `*2"}]')`)
		f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (501, 1, 'kill', 'grave usher', 'Kill a Grave Usher.')`)
		if place {
			f.exec(`UPDATE quests SET offerworld = 0, offerx = 25403, offerz = 15725, offerradius = 6 WHERE questid = 501`)
		}
		f.qe.Reload()
		f.msgs()
		if place {
			f.qe.checkPlaceOffers()
		} else {
			altar := f.tw.add(1, &entity.NPC{ServerID: 701, Name: "Altar", SpawnX: 25403.5, SpawnY: 54, SpawnZ: 15725.5})
			f.tw.talk(altar)
		}
		m := f.msgs()
		if boxID(m) == 0 {
			t.Fatalf("place %v: not offered: %q", place, m)
		}
		f.tw.pick(boxID(m), 0)
		var out took
		for _, s := range f.msgs() {
			out.msgs = append(out.msgs, strings.Replace(s, "Altar: ", "", 1))
		}
		f.db.QueryRow(`SELECT step, count, slot, done FROM charquests WHERE serverid = ? AND questid = 501`, f.sid).
			Scan(&out.step, &out.count, &out.slot, &out.done)
		_, out.journal = f.journal()
		if f.held(f.bone) != 2 {
			t.Errorf("place %v: the accept event's give: %d bone chips, want 2", place, f.held(f.bone))
		}
		return out
	}
	byGiver, atPlace := run(false), run(true)
	if byGiver.step != 1 {
		t.Fatalf("the giver's accept: step %d", byGiver.step)
	}
	if !reflect.DeepEqual(byGiver, atPlace) {
		t.Errorf("a place's accept differs from a giver's:\n giver %+v\n place %+v", byGiver, atPlace)
	}
}

// The map's "!" of a quest offered at a place stands at the place, not at its
// giver's spawn far off; the control: the same quest with no place is marked
// at its giver.
func TestAPlaceQuestIsMarkedAtItsPlace(t *testing.T) {
	f := placeFixture(t, "", "")
	f.tw.add(1, &entity.NPC{ServerID: 701, Name: "Northeast Altar", SpawnX: 26000, SpawnY: 54, SpawnZ: 15725})
	f.exec(`UPDATE quests SET giver = 'Northeast Altar', offerx = 25450, offerz = 15800 WHERE questid = 500`)
	f.qe.Reload()
	where := func() (float32, float32, bool) {
		f.qe.SetMarkers(f.tw.s, f.tw.p, true)
		marks, _ := sentMarks(t, f.tw.s)
		f.qe.SetMarkers(f.tw.s, f.tw.p, false)
		for _, m := range marks {
			if m.kind == markTake && len(m.quests) > 0 && m.quests[0].name == "Envaric Ascension" {
				return m.x, m.z, true
			}
		}
		return 0, 0, false
	}
	if x, z, ok := where(); !ok || x != 25450 || z != 15800 {
		t.Errorf("the place's mark at %v, %v (%v), want 25450, 15800", x, z, ok)
	}
	f.exec(`UPDATE quests SET offerradius = 0 WHERE questid = 500`)
	f.qe.Reload()
	if x, z, ok := where(); !ok || x != 26000 || z != 15725 {
		t.Errorf("with no place the mark at %v, %v (%v), want the giver's 26000, 15725", x, z, ok)
	}
	// and a place quest a player may not take is marked nowhere
	f.exec(`UPDATE quests SET offerradius = 6, classes = '0' WHERE questid = 500`)
	f.qe.Reload()
	if _, _, ok := where(); ok {
		t.Error("a quest not for the player's class marked at its place")
	}
}

// A quest's place is read with it (database.Quest.OfferAt): none without a
// radius.
func TestAQuestsPlaceIsRead(t *testing.T) {
	f := placeFixture(t, ", offerheight", ", 2.5")
	f.qe.mu.Lock()
	qu := f.qe.cat.byID[500]
	places := len(f.qe.cat.places)
	f.qe.mu.Unlock()
	want := &database.QuestPlace{World: 0, X: 25403, Y: 54, Z: 15725, Radius: 6, Height: 2.5}
	if qu == nil || !reflect.DeepEqual(qu.OfferAt, want) || places != 1 {
		t.Fatalf("read %+v (%d places), want %+v", qu.OfferAt, places, want)
	}
	f.exec(`UPDATE quests SET offerradius = 0 WHERE questid = 500`)
	f.qe.Reload()
	f.qe.mu.Lock()
	defer f.qe.mu.Unlock()
	if qu := f.qe.cat.byID[500]; qu.OfferAt != nil || len(f.qe.cat.places) != 0 {
		t.Errorf("radius 0 is no place: %+v, %d places", qu.OfferAt, len(f.qe.cat.places))
	}
}
