package handlers

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
)

// Board entry 1250's gate (questdefgate.go): quests held out of play while a guarded edit
// runs, publications one at a time, a failed publication keeping its quests held. Quest
// 10010 is newQuestFixture's (Malsis offers it).

func (f *questFixture) offered() bool {
	f.t.Helper()
	f.msgs()
	f.qe.Talk(f.tw.s, f.tw.p, &entity.NPC{Name: "Malsis", ServerID: 901})
	return strings.Contains(strings.Join(f.msgs(), "|"), "Will you prove your worth?")
}

// Held during the edit: no offer, and an old box's accept starts nothing; let go after.
func TestAGuardedEditHoldsItsQuests(t *testing.T) {
	f := newQuestFixture(t)
	if !f.offered() {
		t.Fatal("the control: Malsis offers before any edit")
	}
	f.qe.Talk(f.tw.s, f.tw.p, &entity.NPC{Name: "Malsis", ServerID: 901})
	box := boxID(f.msgs())
	_, err := f.qe.GuardedEdit([]int{10010}, func() error {
		f.qe.Answer(f.tw.s, f.tw.p, box, 0) // the old box's accept
		if m := strings.Join(f.msgs(), "|"); !strings.Contains(m, questHeldLine) {
			t.Errorf("no word on the held quest: %s", m)
		}
		if f.offered() {
			t.Error("offered while held")
		}
		if _, err := f.qe.SetCharacterQuest(f.sid, 10010, QuestGive, 0); err == nil {
			t.Error("a GM's give went through while held")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if step, _ := f.progress(10010); step > 0 {
		t.Fatal("the held quest was started")
	}
	if !f.offered() {
		t.Fatal("not offered again after the edit")
	}
}

// Someone with the quest in memory refuses the edit before its commit runs.
func TestAGuardedEditRefusesAQuestInPlay(t *testing.T) {
	f := newQuestFixture(t)
	if _, err := f.qe.SetCharacterQuest(f.sid, 10010, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	ran := false
	_, err := f.qe.GuardedEdit([]int{10010}, func() error { ran = true; return nil })
	if !errors.Is(err, ErrQuestInPlay) || ran {
		t.Fatalf("err %v, commit ran %v", err, ran)
	}
}

// A reload asked while an edit is under way waits for it, and publishes after it: never a
// load begun earlier over the edit's publication.
func TestPublicationsAreOneAtATime(t *testing.T) {
	f := newQuestFixture(t)
	var mu sync.Mutex
	var order []string
	note := func(s string) { mu.Lock(); order = append(order, s); mu.Unlock() }
	f.qe.loadHook = func() { note("load") }
	release := make(chan struct{})
	inCommit := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		f.qe.GuardedEdit([]int{10010}, func() error {
			close(inCommit)
			<-release
			note("commit")
			return nil
		})
	}()
	<-inCommit
	reloaded := make(chan struct{})
	go func() {
		defer wg.Done()
		f.qe.Reload()
		close(reloaded)
	}()
	select {
	case <-reloaded:
		t.Fatal("a reload published during the edit's commit")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	wg.Wait()
	if got := strings.Join(order, ","); got != "commit,load,load" {
		t.Fatalf("order %s, want the edit's commit and load before the reload's", got)
	}
}

// A publication that fails after the commit keeps the quests held until a reload succeeds.
func TestAnUnpublishedEditStaysHeld(t *testing.T) {
	f := newQuestFixture(t)
	_, err := f.qe.GuardedEdit([]int{10010}, func() error {
		f.exec(`ALTER TABLE quests RENAME TO quests_away`)
		return nil
	})
	if !errors.Is(err, ErrQuestUnpublished) {
		t.Fatalf("err %v", err)
	}
	f.exec(`ALTER TABLE quests_away RENAME TO quests`)
	if f.offered() {
		t.Fatal("offered while its edit is unpublished")
	}
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	if !f.offered() {
		t.Fatal("still held after a reload succeeded")
	}
}
