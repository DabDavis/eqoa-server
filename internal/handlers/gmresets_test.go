package handlers

import (
	"encoding/binary"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// resetWorld is gmWorld with the rows of the GM (1), the player (2) and a
// character out of the world, "Away" (3), the reset store, the test catalog
// and a mastery window keeping its ranks in those rows, its book fakeBook.
type resetWorld struct {
	*chatWorld
	gs, ps *session.Session
	gp, pp *entity.Player
	store  *database.SQLiteCharacterStore
	inv    *database.InventoryStore
	cm     *ClassMasteryHandler
	book   *fakeBook
}

func newResetWorld(t *testing.T) *resetWorld {
	c, gs, gp, ps, pp := gmWorld(t)
	store, inv := gmRows(t, c)
	if err := store.Create(&database.CharacterRecord{CharName: "Away", AccountID: 3, Level: 40}); err != nil {
		t.Fatal(err)
	}
	if rec, err := store.GetByName("Away"); err != nil || rec.ServerID != 3 {
		t.Fatalf("Away: %+v %v", rec, err)
	}
	c.h.SetResetStore(store)
	c.gw.SetCMCatalog(testCatalog())
	cm := NewClassMasteryHandler(c.sm)
	cm.SetGameWorld(c.gw)
	cm.SetStore(store)
	book := &fakeBook{spells: map[int]string{500: "Taunt"}}
	cm.SetSpellbook(book)
	c.h.SetClassMastery(cm)
	for _, p := range []*entity.Player{gp, pp} {
		p.Class, p.Level, p.BaseStr = 0, 60, 130
		p.RecalcStats(true)
	}
	return &resetWorld{chatWorld: c, gs: gs, ps: ps, gp: gp, pp: pp, store: store, inv: inv, cm: cm, book: book}
}

// buy is a 0x1401 purchase of rank id by s's player.
func (w *resetWorld) buy(s *session.Session, id int) {
	w.t.Helper()
	req := make([]byte, 9)
	req[0] = cmBuy
	binary.LittleEndian.PutUint32(req[5:], uint32(id))
	if err := w.cm.HandleClassMastery(s, incoming(protocol.OpcodeClassMastery, req)); err != nil {
		w.t.Fatal(err)
	}
}

// lines is the text of every chat line s was sent, then forgotten.
func lines(t *testing.T, s *session.Session) []string {
	return gmReplies(t, s)
}

// #rst_mastr names any character: one in the world has its ranks taken where
// it stands, what they cost back, kept, and is told as the disc reads a
// removal (0x1402 sub-op 6 a rank, then 3) and by the live word 1885; one out
// of it on its rows. The control: a player's #rst_mastr resets nothing.
func TestResetMasteriesByName(t *testing.T) {
	w := newResetWorld(t)
	w.pp.Mastery = entity.Mastery{Points: 100}
	w.buy(w.ps, 1)
	if w.pp.Mastery.Points != 90 || len(w.pp.CMs) != 1 {
		t.Fatalf("bought: %+v %v", w.pp.Mastery, w.pp.CMs)
	}
	w.say(w.ps, "#rst_mastr player")
	if len(w.pp.CMs) != 1 {
		t.Fatal("the control: a player's #rst_mastr reset its masteries")
	}

	w.ps.Queue().GetPending()
	w.gs.Queue().GetPending()
	w.say(w.gs, "#rst_mastr player")
	if w.pp.Mastery.Points != 100 || w.pp.Mastery.Spent != 0 || len(w.pp.CMs) != 0 {
		t.Fatalf("after: %+v %v", w.pp.Mastery, w.pp.CMs)
	}
	if owned, _ := w.store.LoadOwnedCMs(2); len(owned) != 0 {
		t.Errorf("ranks kept: %v", owned)
	}
	if m, _ := w.store.LoadMastery(2); m.Points != 100 || m.Spent != 0 {
		t.Errorf("points kept: %+v", m)
	}
	var subs []byte
	for _, m := range w.ps.Queue().PeekPending() {
		if d := m.Data(); opcode(d) == protocol.OpcodeClassMasteryServer {
			subs = append(subs, d[2])
		}
	}
	if string(subs) != string([]byte{cmRespRemoved, cmRespPoints}) || !hasLine(texts(heard(t, w.ps)), MasteryResetText) {
		t.Errorf("the player was sent sub-ops %v, the lines %q", subs, texts(heard(t, w.ps)))
	}
	if got := lines(t, w.gs); !hasLine(got, "Player: 1 masteries taken away, 10 points back") {
		t.Errorf("the GM heard %q", got)
	}

	// out of the world: its rows
	if err := w.store.BuyCM(3, database.OwnedCM{ID: 1, Cost: 10}, database.MasteryRow{Points: 5, Spent: 10}); err != nil {
		t.Fatal(err)
	}
	w.say(w.gs, "#rst_mastr away")
	if owned, _ := w.store.LoadOwnedCMs(3); len(owned) != 0 {
		t.Errorf("Away's ranks: %v", owned)
	}
	if m, _ := w.store.LoadMastery(3); m.Points != 15 || m.Spent != 0 {
		t.Errorf("Away's points: %+v", m)
	}
	if got := lines(t, w.gs); !hasLine(got, "Away (not in the world") {
		t.Errorf("the GM heard %q", got)
	}
	w.say(w.gs, "#rst_mastr nobody")
	if got := lines(t, w.gs); !hasLine(got, `no character named "nobody"`) {
		t.Errorf("an unknown name: %q", got)
	}
}

// #rst_both gives back the training points too: in the world the base stats
// lose what the points bought, the unspent points get them, the trainer is
// told the difference (0x001D, old - new: negative), the live word 1886 is
// said, and the row's tp columns are zero; out of the world, the row alone.
func TestResetBothGivesTrainingBack(t *testing.T) {
	w := newResetWorld(t)
	rec, _ := w.store.GetByServerID(2)
	rec.TPStrength, rec.TPStamina, rec.UnusedTP, rec.TotalTP = 5, 3, 2, 10
	if err := w.store.Update(rec); err != nil {
		t.Fatal(err)
	}
	w.pp.BaseStr, w.pp.BaseSta = 130+5, 100+3
	w.pp.UnusedTP, w.pp.TotalTP = 2, 10
	w.pp.RecalcStats(true)

	w.ps.Queue().GetPending()
	w.say(w.gs, "#rst_both Player")
	if w.pp.BaseStr != 130 || w.pp.BaseSta != 100 || w.pp.UnusedTP != 10 || w.pp.TotalTP != 10 {
		t.Fatalf("in the world: STR %d STA %d, %d of %d", w.pp.BaseStr, w.pp.BaseSta, w.pp.UnusedTP, w.pp.TotalTP)
	}
	rec, _ = w.store.GetByServerID(2)
	if rec.TPStrength != 0 || rec.TPStamina != 0 || rec.UnusedTP != 10 || rec.TotalTP != 10 {
		t.Errorf("kept: %+v", rec)
	}
	z := sentOp(w.ps, protocol.OpcodeUpdateTrainingPoints)
	if v, err := buffer.NewReader(z[2:]).Read7BitEncodedInt64(); err != nil || v != -8 {
		t.Errorf("0x001D % X: %d, want -8", z, v)
	}
	if said := texts(heard(t, w.ps)); !hasLine(said, TrainingResetText) {
		t.Errorf("the player heard %q", said)
	}
	if got := lines(t, w.gs); !hasLine(got, "Player: 8 training points back (STR 5, STA 3), 10 unspent of 10") {
		t.Errorf("the GM heard %q", got)
	}

	away, _ := w.store.GetByServerID(3)
	away.TPAgility, away.UnusedTP, away.TotalTP = 4, 0, 4
	if err := w.store.Update(away); err != nil {
		t.Fatal(err)
	}
	w.say(w.gs, "#rst_both away")
	away, _ = w.store.GetByServerID(3)
	if away.TPAgility != 0 || away.UnusedTP != 4 || away.TotalTP != 4 {
		t.Errorf("Away kept: %+v", away)
	}
}

// #rem_abil takes an ability out of the named character's book, by its name
// (any case), the one name it begins or its spell id, and out of the client's
// at once (0x00B8, its display index); several that begin so are named, not
// taken.
func TestRemoveAbility(t *testing.T) {
	w := newResetWorld(t)
	w.cm.SetStore(&keptMastery{book: w.book})
	w.book.spells = map[int]string{500: "Taunt", 600: "Fire Bolt", 601: "Fire Storm", 700: "Spirit of the Wolf"}
	w.ps.Queue().GetPending()
	w.say(w.gs, "#rem_abil player taunt")
	if _, ok := w.book.spells[500]; ok {
		t.Fatal("Taunt still in the book")
	}
	if d := sentOp(w.ps, protocol.OpcodeAbilityRemove); len(d) < 6 || binary.LittleEndian.Uint32(d[2:]) != 0 {
		t.Errorf("0x00B8 % X", d)
	}
	w.say(w.gs, "#rem_abil player fire")
	if got := lines(t, w.gs); !hasLine(got, `"fire" is several abilities: Fire Bolt, Fire Storm`) || len(w.book.spells) != 3 {
		t.Errorf("an ambiguous name: %q, %v", got, w.book.spells)
	}
	w.say(w.gs, "#rem_abil player spirit of")
	w.say(w.gs, "#rem_abil player 601")
	if len(w.book.spells) != 1 || w.book.spells[600] != "Fire Bolt" {
		t.Errorf("left %v", w.book.spells)
	}
	w.say(w.gs, "#rem_abil player")
	if got := lines(t, w.gs); !hasLine(got, "Usage: #rem_abil") {
		t.Errorf("no ability: %q", got)
	}
}

// #banktunar adds to a bank: a character in the world where the game holds
// it, kept and told (0x1253), never below 0; one out of it on its row. The
// live client's 0x0701 56 (a zigzag number) adds to the GM's own with no
// target. The control: a player's 56 is dropped.
func TestGMBankTunar(t *testing.T) {
	w := newResetWorld(t)
	w.pp.BankTunar = 100
	w.say(w.gs, "#banktunar player 50")
	rec, _ := w.store.GetByServerID(2)
	if w.pp.BankTunar != 150 || rec.BankTunar != 150 {
		t.Fatalf("150: the game %d, the row %d", w.pp.BankTunar, rec.BankTunar)
	}
	if d := sentOp(w.ps, protocol.OpcodeConfirmBankTunar); len(d) != 4 || d[2] != 0xAC || d[3] != 0x02 {
		t.Errorf("0x1253 % X", d)
	}
	w.say(w.gs, "#banktunar player -500")
	if w.pp.BankTunar != 0 {
		t.Errorf("below 0: %d", w.pp.BankTunar)
	}
	w.say(w.gs, "#banktunar away 75")
	if away, _ := w.store.GetByServerID(3); away.BankTunar != 75 {
		t.Errorf("Away's bank %d", away.BankTunar)
	}
	w.gp.TargetObjectID = 0xFFFFFFFF
	w.gm(w.gs, "38"+"32") // 56, zigzag 25
	if w.gp.BankTunar != 25 {
		t.Errorf("the live client's /banktunar 25: %d", w.gp.BankTunar)
	}
	w.gm(w.ps, "38"+"32")
	if w.pp.BankTunar != 0 {
		t.Errorf("the control: a player's 56 gave %d", w.pp.BankTunar)
	}
}

// The live client's five sub-ops read as its senders write them: 56 a zigzag
// (negative too), 57-59 an 8-bit string, 101 nothing; and run as the #
// commands do (57 resets by the name it carries).
func TestLiveGMSubOps(t *testing.T) {
	for _, c := range []struct {
		hex  string
		want gmRequest
	}{
		{"38e707", gmRequest{Sub: 56, A: uint32(0xFFFFFE0C)}}, // zigzag(-500) = 999
		{"39" + gmText("Bob"), gmRequest{Sub: 57, Text: "Bob"}},
		{"3a" + gmText("Bob"), gmRequest{Sub: 58, Text: "Bob"}},
		{"3b" + gmText("Bob Taunt"), gmRequest{Sub: 59, Text: "Bob Taunt"}},
		{"65", gmRequest{Sub: 101}},
	} {
		b, _ := hex.DecodeString(c.hex)
		got, err := parseGMRequest(b)
		if err != nil || got != c.want {
			t.Errorf("%s: %+v %v, want %+v", c.hex, got, err, c.want)
		}
	}
	w := newResetWorld(t)
	w.pp.Mastery = entity.Mastery{Points: 100}
	w.buy(w.ps, 1)
	w.gm(w.gs, "39"+gmText("Player"))
	if len(w.pp.CMs) != 0 || w.pp.Mastery.Points != 100 {
		t.Errorf("57 Player: %+v %v", w.pp.Mastery, w.pp.CMs)
	}
}

// #servdown is an admin's (gm.admin_level), and the first asks: the second
// within the window restarts. The control: with no restart given it says so.
func TestServerDownAsksTwice(t *testing.T) {
	c, gs, _, _, _ := gmWorld(t)
	called := make(chan struct{}, 1)
	SetServerDown(func() error { called <- struct{}{}; return nil }, 2)
	t.Cleanup(func() { SetServerDown(nil, 0) })
	c.say(gs, "#servdown")
	if got := gmReplies(t, gs); !hasLine(got, "an admin's") {
		t.Fatalf("a level 1 GM: %q", got)
	}
	c.h.SetAccessLevels(levels{1: 2, 2: 0})
	c.say(gs, "#servdown")
	if got := gmReplies(t, gs); !hasLine(got, "Again within 30 seconds") {
		t.Fatalf("the first: %q", got)
	}
	select {
	case <-called:
		t.Fatal("restarted on the first")
	case <-time.After(700 * time.Millisecond):
	}
	c.gm(gs, "65") // the live client's /servdown
	if got := gmReplies(t, gs); !hasLine(got, "restarting now") {
		t.Fatalf("the second: %q", got)
	}
	select {
	case <-called:
	case <-time.After(3 * time.Second):
		t.Fatal("no restart")
	}
	SetServerDown(nil, 2)
	c.say(gs, "#servdown")
	if got := gmReplies(t, gs); !hasLine(got, "cannot restart itself") {
		t.Errorf("no restart given: %q", got)
	}
}

// The / spelling reaches the new commands (vanilla forwards an unknown
// /command).
func TestResetCommandsHaveTheSlashSpelling(t *testing.T) {
	for _, l := range []string{"/rst_mastr Bob", "/rst_both Bob", "/rem_abil Bob Taunt", "/banktunar 5", "/servdown"} {
		if got := slashGM(l); !strings.HasPrefix(got, "#") {
			t.Errorf("%q: %q", l, got)
		}
	}
}
