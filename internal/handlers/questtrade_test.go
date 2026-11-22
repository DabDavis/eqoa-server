package handlers

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// tradeFixture is Ionma Oschba as work board 747 filmed her: her talk opens "Greetings!" with three token
// trades and More. . ., each trade a repeatable one-step quest whose give step takes the token and offers
// its tier's rewards.
type tradeFixture struct {
	*questFixture
	ionma                 uint32
	superior, greater     int
	band, stick, circlet  int
	greaterStick, goblins int
}

const (
	tradeGreater  = 109747
	tradeSuperior = 109749
)

func newTradeFixture(t *testing.T) *tradeFixture {
	t.Helper()
	f := &tradeFixture{questFixture: newQuestFixture(t)}
	f.tw.p.Level = 60
	f.ionma = f.tw.add(1, &entity.NPC{ServerID: 901, Name: "Ionma Oschba", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	f.superior, f.greater = f.pattern("Token of Superior Benediction", 1), f.pattern("Token of Greater Benediction", 1)
	f.band, f.stick, f.circlet = f.pattern("Superior Orcblood Band", 1), f.pattern("Superior Corrosive Stick", 1), f.pattern("Superior Tainted Circlet", 1)
	f.greaterStick, f.goblins = f.pattern("Greater Corrosive Stick", 1), f.pattern("Superior Goblinate", 1)
	for _, q := range []struct {
		id, token int
		tier      string
		rewards   []int
	}{{tradeSuperior, f.superior, "Superior", []int{f.band, f.stick, f.circlet, f.goblins}}, {tradeGreater, f.greater, "Greater", []int{f.greaterStick}}} {
		var opts []string
		for _, r := range q.rewards {
			opts = append(opts, fmt.Sprintf(`{"answer":"Reward %d.","items":"%d"}`, r, r))
		}
		f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, repeatable, offer, accept, decline, declined, started)
			VALUES (?, ?, 1, 'Ionma Oschba', 0, 0, 1, '', '', '', '', '')`, q.id, "Token of "+q.tier+" Benediction trade")
		f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, journal, says, waiting, choices)
			VALUES (?, 1, 'give', 'Ionma Oschba', ?, 'Trade the token to Ionma Oschba.', 'Wear it in good health.', 'Choose your reward.', ?)`,
			q.id, fmt.Sprint(q.token), `{"ask":"Which will you have?","options":[`+strings.Join(opts, ",")+`]}`)
	}
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	f.tw.h.SetTalkTrees(func() (map[string]map[int]database.TalkNode, error) {
		return map[string]map[int]database.TalkNode{"ionma oschba": {
			1: {Says: []string{"Greetings!"}, Choices: []database.TalkChoice{
				{Text: "I heard that you will trade my Token of Greater Benediction.", Quest: tradeGreater, Lacking: "You do not have that token."},
				{Text: "I heard that you will trade my Token of Major Benediction.", Quest: 109748, Lacking: "You do not have that token."},
				{Text: "I heard that you will trade my Token of Superior Benediction.", Quest: tradeSuperior, Lacking: "You do not have that token."},
				{Text: "More. . .", Next: 2}}},
			2: {Says: []string{"Greetings!"}},
		}}, nil
	})
	return f
}

func (f *tradeFixture) talk() []string {
	f.t.Helper()
	f.msgs()
	f.tw.talk(f.ionma)
	return f.msgs()
}

const ionmaBox = "Greetings!|I heard that you will trade my Token of Greater Benediction.|I heard that you will trade my Token of Major Benediction.|I heard that you will trade my Token of Superior Benediction.|More. . .|Exit"

// Her talk is the filmed box, whether or not the player holds a token (the trades' quests are never offered
// by a talk); a trade without the token says so and takes nothing; with it the token goes and the tier's
// rewards are offered, the pick given; it trades again (repeatable). More. . . says Greetings! and closes.
func TestIonmaTradesTokens(t *testing.T) {
	f := newTradeFixture(t)
	m := f.talk()
	if !hasMsg(m, "ask#*") || !strings.Contains(strings.Join(m, "\n"), ":"+ionmaBox) {
		t.Fatalf("her box: %q", m)
	}
	f.tw.pick(boxID(m), 2)
	f.want(f.msgs(), "box:Ionma Oschba: You do not have that token.")
	f.own(f.superior, 1)
	m = f.talk()
	if !strings.Contains(strings.Join(m, "\n"), ":"+ionmaBox) {
		t.Fatalf("holding a token, her talk: %q (a hand-in's offer took the menu's place)", m)
	}
	f.tw.pick(boxID(m), 2)
	m = f.msgs()
	if !asks(m, "Which will you have?") {
		t.Fatalf("the superior trade: %q", m)
	}
	f.tw.pick(boxID(m), 1)
	f.msgs()
	if f.held(f.superior) != 0 || f.held(f.stick) != 1 || f.held(f.band) != 0 {
		t.Errorf("held: token %d, stick %d, band %d", f.held(f.superior), f.held(f.stick), f.held(f.band))
	}
	// again
	f.own(f.superior, 1)
	m = f.talk()
	f.tw.pick(boxID(m), 2)
	m = f.msgs()
	f.tw.pick(boxID(m), 0)
	f.msgs()
	if f.held(f.superior) != 0 || f.held(f.band) != 1 {
		t.Errorf("the second trade: token %d, band %d", f.held(f.superior), f.held(f.band))
	}
	// a trade whose quest is not there says nothing and takes nothing; More. . . says Greetings! again
	m = f.talk()
	f.tw.pick(boxID(m), 3)
	f.want(f.msgs(), "box:Ionma Oschba: Greetings!")
	// the greater token trades only for the greater tier
	f.own(f.greater, 1)
	m = f.talk()
	f.tw.pick(boxID(m), 0)
	m = f.msgs()
	f.tw.pick(boxID(m), 0)
	f.msgs()
	if f.held(f.greater) != 0 || f.held(f.greaterStick) != 1 {
		t.Errorf("the greater trade: token %d, stick %d", f.held(f.greater), f.held(f.greaterStick))
	}
}
