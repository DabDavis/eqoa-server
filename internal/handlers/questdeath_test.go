package handlers

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// A playerdeath event (questdeath.go; Heirloom petition 3488, Envaric
// Ascension): the step done kills the player at no cost (no debt), the words
// before it said at once, and the popup after it held for the respawn, then
// shown. A playerdeath on a step's start is refused.
func TestAPlayerDeathEventHoldsTheRestForTheRespawn(t *testing.T) {
	old := priceAfterRespawn
	priceAfterRespawn = 0
	t.Cleanup(func() { priceAfterRespawn = old })
	f := newQuestFixture(t)
	f.tw.p.HP, f.tw.p.MaxHP = 100, 100
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (90, 'Envaric Ascension', 1, 'Malsis')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, events) VALUES (90, 1, 'talk', 'Kellina', 'The altar.',
		'[{"on":"done","do":"say","text":"Its price is your life."},{"on":"done","do":"playerdeath"},{"on":"done","do":"popup","text":"You have the ascension."}]')`)
	f.qe.Reload()
	kellina := f.tw.add(2, &entity.NPC{ServerID: 702, Name: "Kellina", SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	if _, err := f.qe.SetCharacterQuest(f.sid, 90, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	f.tw.talk(kellina)
	m := f.msgs()
	if !hasMsg(m, "box:Its price is your life.") {
		t.Fatalf("the words before the death: %q", m)
	}
	for _, x := range m {
		if strings.HasPrefix(x, "ask#") {
			t.Fatalf("the popup came before the respawn: %q", m)
		}
	}
	if !f.tw.p.IsDead() || f.tw.p.Debt != 0 {
		t.Fatalf("after the turn-in: dead %v, debt %d", f.tw.p.IsDead(), f.tw.p.Debt)
	}
	if s, _ := f.progress(90); s != 0 && s != 2 {
		t.Logf("progress %d", s)
	}

	f.qe.AfterRespawn(f.tw.s, f.tw.p)
	var got []string
	for end := time.Now().Add(2 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		if got = append(got, f.msgs()...); len(got) > 0 {
			break
		}
	}
	if !hasMsg(got, fmt.Sprintf("ask#%d:You have the ascension.|OK", boxID(got))) {
		t.Fatalf("after the respawn: %q", got)
	}
	f.qe.AfterRespawn(f.tw.s, f.tw.p) // a second respawn shows nothing again
	time.Sleep(50 * time.Millisecond)
	if m := f.msgs(); len(m) != 0 {
		t.Errorf("a second respawn: %q", m)
	}

	if p := database.EventProblems(`[{"on":"start","do":"playerdeath"}]`, true); len(p) == 0 {
		t.Error("a playerdeath on a step's start was taken")
	}
	if p := database.EventProblems(`[{"on":"done","do":"playerdeath"}]`, true); len(p) != 0 {
		t.Errorf("a step's playerdeath refused: %v", p)
	}
}
