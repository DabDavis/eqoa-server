package handlers

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

func TestFroglokRefugeeConversations(t *testing.T) {
	for _, tc := range []struct {
		name, shard string
		race        int
		wantLines   bool
	}{
		{"Heirloom Froglok", "heirloom", int(RaceFroglok), true},
		{"Heirloom human", "heirloom", 0, false},
		{"Heirloom Kappa", "heirloom", 16, false},
		{"Vanilla Froglok", "live", int(RaceFroglok), false},
		{"disabled Froglok", "off", int(RaceFroglok), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newQuestFixture(t)
			f.tw.p.Race, f.tw.p.Class = tc.race, 0
			f.exec(`INSERT INTO quests (questid, name, enabled, giver, offer, accept)
				VALUES (100061, 'The Militia Bracer', 1, 'Commander Nothard',
					'Come inside.' || char(10) || char(10) || 'We keep the peace in this city.' || char(10) || 'Will you serve?',
					'Hello.' || char(10) || 'I will.'),
					(100064, 'The Militia Short Sword', 1, 'Captain Norgam', 'Ready?', 'Yes.')`)
			f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, says, rewarditems)
				VALUES (100061,1,'talk','Commander Nothard','Speak to Nothard.','Welcome.',''),
					(100064,1,'talk','Commander Nothard','See Nothard.','',''),
					(100064,2,'talk','Captain Norgam','Return to Norgam.',
						'There.' || char(10) || 'The Militia Short Sword. Short enough for an alley.',?)`, f.aura)
			if err := f.qe.Reload(); err != nil {
				t.Fatal(err)
			}
			if tc.shard != "off" {
				err := f.qe.EnableFroglokRefugeeDialogue(tc.shard)
				if (err != nil) != (tc.shard == "live") {
					t.Fatalf("enable for %q: %v", tc.shard, err)
				}
			}
			nothard := f.tw.add(1, &entity.NPC{ServerID: 701, Name: "Commander Nothard", SpawnX: 25403, SpawnY: 54, SpawnZ: 15725})
			norgam := f.tw.add(2, &entity.NPC{ServerID: 702, Name: "Captain Norgam", SpawnX: 25404, SpawnY: 54, SpawnZ: 15726})
			f.tw.talk(nothard)
			first := f.msgs()
			if strings.Contains(strings.Join(first, "\n"), "I didn't expect a froglok") {
				t.Fatal("the aside was spoken before its paragraph")
			}
			f.tw.pick(boxID(first), 0)
			offer := f.msgs()
			checkDialogueSequence(t, offer, "I didn't expect a froglok", "We keep the peace in this city.", tc.wantLines)
			if !strings.Contains(strings.Join(offer, "\n"), "Will you serve?|I will.") {
				t.Fatalf("lost the original question or answer: %q", offer)
			}
			f.tw.pick(boxID(offer), 0)
			f.tw.talk(nothard)
			f.msgs()
			f.tw.talk(norgam)
			f.tw.pick(boxID(f.msgs()), 0)
			f.tw.talk(nothard)
			f.msgs()
			f.tw.talk(norgam)
			end := f.msgs()
			checkDialogueSequence(t, end, "Try the grip.", "The Militia Short Sword. Short enough for an alley.", tc.wantLines)
			if _, done := f.progress(100064); done != 1 || f.held(f.aura) != 1 {
				t.Fatalf("quest done %d, reward count %d", done, f.held(f.aura))
			}
		})
	}
}

func checkDialogueSequence(t *testing.T, messages []string, aside, original string, want bool) {
	t.Helper()
	text := strings.Join(messages, "\n")
	pos, base := strings.Index(text, aside), strings.Index(text, original)
	if base < 0 || (pos >= 0) != want || (want && (pos >= base || strings.Count(text, aside) != 1)) {
		t.Fatalf("unexpected dialogue sequence: %q", messages)
	}
}

func TestFroglokDialogueRequiresOriginalScene(t *testing.T) {
	q := &QuestEngine{}
	if err := q.EnableFroglokRefugeeDialogue(""); err != nil {
		t.Fatal(err)
	}
	frog := &entity.Player{Race: int(RaceFroglok)}
	for _, tc := range []struct {
		speaker, original string
	}{
		{"Someone else", "We keep the peace in this city."},
		{"Commander Nothard", "A changed offer."},
		{"Commander Nothard", "They say: We keep the peace in this city."},
	} {
		if got := q.froglokWords(frog, 100061, 0, tc.speaker, tc.original); got != tc.original {
			t.Errorf("changed a different scene: %q", got)
		}
	}
}
