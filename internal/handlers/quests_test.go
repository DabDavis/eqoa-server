package handlers

import (
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// questFixture is a talk world with a database, a quest engine running and
// one character, a level 5 human magician, in the world.
type questFixture struct {
	t   *testing.T
	tw  *talkWorld
	db  *database.SQLiteDB
	inv *database.InventoryStore
	qe  *QuestEngine
	sid uint32

	pincer, aura, bone int
}

func newQuestFixture(t *testing.T) *questFixture {
	t.Helper()
	tw := newTalkWorld(t, 25403, 15725)
	db, err := database.NewSQLite(t.TempDir() + "/quests.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	rec := &database.CharacterRecord{CharName: "Talker", AccountID: 1}
	if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
		t.Fatal(err)
	}
	tw.p.CharServerID, tw.p.Class, tw.p.Race, tw.p.Level = rec.ServerID, 10, 0, 5
	f := &questFixture{t: t, tw: tw, db: db, inv: database.NewInventoryStore(db), sid: rec.ServerID}
	f.pincer = f.pattern("cracked ant pincer", 20)
	f.aura = f.pattern("Smoldering Aura", 1)
	f.bone = f.pattern("bone chips", 20)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, classes, offer, accept, decline, started)
		VALUES (10010, 'The pincers', 1, 'Malsis', 1, 10, '10', 'Malsis: You there.' || char(10) || 'Will you prove your worth?',
			'I will.', 'Not now.', 'Malsis: Go and see Kellina.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (10010, 1, 'talk', 'Kellina', 'Go speak to Kellina.')`)
	f.exec(fmt.Sprintf(`INSERT INTO queststeps (questid, step, kind, target, items, journal, answer, says, waiting, xp, rewarditems)
		VALUES (10010, 2, 'give', 'Kellina', '%d*2', 'Bring Kellina two cracked ant pincers.', 'I have the pincers.',
			'Kellina: I am a woman of my word.', 'Kellina: Bring me two cracked ant pincers.', 430, '%d')`, f.pincer, f.aura))
	qe, err := NewQuestEngine(db, tw.w, tw.mgr, f.inv, database.NewSpellbookStore(db))
	if err != nil {
		t.Fatal(err)
	}
	qe.Start()
	t.Cleanup(qe.Stop)
	f.qe = qe
	tw.s.SetInventoryList(newInvList(nil))
	f.journal() // as the MemoryDump leaves it
	return f
}

func (f *questFixture) exec(q string, args ...any) {
	f.t.Helper()
	if _, err := f.db.Exec(q, args...); err != nil {
		f.t.Fatal(err)
	}
}

// pattern adds an item that stacks to maxStack and returns its patternid.
func (f *questFixture) pattern(name string, maxStack int) int {
	f.t.Helper()
	res, err := f.db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (10, 0, 209763529, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, ?, 0, 0, 32767, 1023, 0, 0, 0, 0, ?, '', 0, 0)`,
		maxStack, name)
	if err != nil {
		f.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

// own puts n of an item in the inventory and the client's list.
func (f *questFixture) own(pattern, n int) {
	f.t.Helper()
	if _, err := f.inv.AddCharItem(f.sid, pattern, n, 0, 0, 0); err != nil {
		f.t.Fatal(err)
	}
	rows, err := f.inv.GetCharacterItems(f.sid)
	if err != nil {
		f.t.Fatal(err)
	}
	f.tw.s.SetInventoryList(newInvList(rows))
}

// held is how many of an item the inventory holds.
func (f *questFixture) held(pattern int) int {
	f.t.Helper()
	var n int
	f.db.QueryRow(`SELECT COALESCE(SUM(stackleft), 0) FROM charinventory WHERE serverid = ? AND patternid = ?`, f.sid, pattern).Scan(&n)
	return n
}

// journal is the dump's journal: its token and lines.
func (f *questFixture) journal() (uint32, []string) { return f.qe.Journal(f.sid) }

// progress is the character's row for a quest: step, done; -1s for none.
func (f *questFixture) progress(quest int) (int, int) {
	var step, done int
	if err := f.db.QueryRow(`SELECT step, done FROM charquests WHERE serverid = ? AND questid = ?`, f.sid, quest).Scan(&step, &done); err != nil {
		return -1, -1
	}
	return step, done
}

// msgs is every message queued for the session since the last call, each as
// "box:text", "ask#id:text|a|b", "add:text", "drop:i", "take:i*n", "item",
// "xp", "chat:text" or "op:XXXX".
func (f *questFixture) msgs() []string { return questMsgs(f.tw.s) }

func questMsgs(s *session.Session) []string {
	var out []string
	for _, m := range s.Queue().GetPending() {
		d := m.Data()
		if len(d) < 2 {
			continue
		}
		op := protocol.GameOpcode(binary.LittleEndian.Uint16(d))
		r := buffer.NewReader(d[2:])
		wide := func() string {
			n, _ := r.ReadUint32()
			u := make([]uint16, n)
			for i := range u {
				u[i], _ = r.ReadUint16()
			}
			return string(utf16.Decode(u))
		}
		switch op {
		case protocol.OpcodeDialogueBox:
			r.ReadUint32()
			out = append(out, "box:"+wide())
		case protocol.OpcodeQuestPopupMulti:
			id, _ := r.ReadUint32()
			text := wide()
			n, _ := r.ReadByte()
			opts := []string{}
			for i := 0; i < int(n); i++ {
				opts = append(opts, wide())
			}
			out = append(out, fmt.Sprintf("ask#%d:%s|%s", id, text, strings.Join(opts, "|")))
		case protocol.OpcodeAddQuestLog:
			r.ReadUint32()
			out = append(out, "add:"+wide())
		case protocol.OpcodeDeleteQuest:
			r.ReadUint32()
			i, _ := r.Read7BitEncodedInt64()
			out = append(out, fmt.Sprintf("drop:%d", i))
		case protocol.OpcodeRemoveInvItem:
			i, _ := r.ReadByte()
			r.ReadByte()
			n, _ := r.Read7BitEncodedInt64()
			out = append(out, fmt.Sprintf("take:%d*%d", i, n))
		case protocol.OpcodeAddInvItem:
			out = append(out, "item")
		case protocol.OpcodeGrantXP:
			out = append(out, "xp")
		case protocol.OpcodeClientMessage:
			out = append(out, "chat:"+wide())
		default:
			out = append(out, fmt.Sprintf("op:%04X", uint16(op)))
		}
	}
	return out
}

// boxID is the id of the last quest box asked in msgs, or 0.
func boxID(msgs []string) uint32 {
	var id uint32
	for _, m := range msgs {
		if strings.HasPrefix(m, "ask#") {
			fmt.Sscanf(m, "ask#%d:", &id)
		}
	}
	return id
}

func hasMsg(msgs []string, want string) bool {
	for _, m := range msgs {
		if m == want || (strings.HasSuffix(want, "*") && strings.HasPrefix(m, strings.TrimSuffix(want, "*"))) {
			return true
		}
	}
	return false
}

func (f *questFixture) want(msgs []string, wants ...string) {
	f.t.Helper()
	for _, w := range wants {
		if !hasMsg(msgs, w) {
			f.t.Errorf("no %q in %q", w, msgs)
		}
	}
}

func (f *questFixture) not(msgs []string, nots ...string) {
	f.t.Helper()
	for _, w := range nots {
		if hasMsg(msgs, w) {
			f.t.Errorf("%q in %q", w, msgs)
		}
	}
}

// The whole quest: the giver's offer, taken; the talk step; the give step
// refused while the pincers lack (a wrong item is no pincer), then answered
// and done: the pincers taken, the reward given, the quest finished, its line
// gone, and the giver offering it no more.
func TestAQuestFromOfferToReward(t *testing.T) {
	f := newQuestFixture(t)
	malsis := f.tw.add(1, &entity.NPC{ServerID: 701, Name: "Malsis", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	kellina := f.tw.add(2, &entity.NPC{ServerID: 702, Name: "Kellina", SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})

	f.tw.talk(malsis)
	m := f.msgs()
	f.want(m, "box:Malsis: You there.", "ask#*")
	if !strings.Contains(strings.Join(m, "\n"), ":Will you prove your worth?|I will.|Not now.") {
		t.Fatalf("the offer's last line asks with the answers: %q", m)
	}
	f.tw.pick(boxID(m), 0)
	m = f.msgs()
	f.want(m, "box:Malsis: Go and see Kellina.", "box:"+questReceived, "add:Go speak to Kellina.")
	if s, d := f.progress(10010); s != 1 || d != 0 {
		t.Fatalf("taken: step %d done %d, want 1 0", s, d)
	}

	f.tw.talk(kellina)
	f.want(f.msgs(), "drop:0", "add:Bring Kellina two cracked ant pincers.")

	f.tw.talk(kellina)
	m = f.msgs()
	f.want(m, "box:Kellina: Bring me two cracked ant pincers.")
	f.not(m, "ask#*", "drop:*")

	// the control: two of another item are not two pincers
	f.own(f.bone, 2)
	f.tw.talk(kellina)
	f.not(f.msgs(), "ask#*")

	f.own(f.pincer, 2)
	f.tw.talk(kellina)
	m = f.msgs()
	if !strings.Contains(strings.Join(m, "\n"), ":Kellina: Bring me two cracked ant pincers.|I have the pincers.") {
		t.Fatalf("with the pincers the step asks for its answer: %q", m)
	}
	f.tw.pick(boxID(m), 0)
	m = f.msgs()
	f.want(m, "box:Kellina: I am a woman of my word.", "box:"+questFinished,
		"box:You have given away 2 cracked ant pincer.", "box:You have received Smoldering Aura.",
		"take:1*2", "item", "xp", "drop:0")
	if f.held(f.pincer) != 0 || f.held(f.bone) != 2 || f.held(f.aura) != 1 {
		t.Errorf("after: pincers %d bones %d auras %d, want 0 2 1", f.held(f.pincer), f.held(f.bone), f.held(f.aura))
	}
	if s, d := f.progress(10010); s != 0 || d != 1 {
		t.Errorf("finished: step %d done %d, want 0 1", s, d)
	}
	if _, lines := f.journal(); len(lines) != 0 {
		t.Errorf("the journal after: %q", lines)
	}
	f.tw.talk(malsis)
	if m := f.msgs(); len(m) != 0 {
		t.Errorf("a finished quest offered again: %q", m)
	}
}

// A quest for magicians is not offered to a warrior, nor below its level; the
// control: the magician at its level is offered it.
func TestAQuestNotForYouIsNotOffered(t *testing.T) {
	f := newQuestFixture(t)
	malsis := f.tw.add(1, &entity.NPC{ServerID: 701, Name: "Malsis", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	f.tw.p.Class = 0
	f.tw.talk(malsis)
	if m := f.msgs(); len(m) != 0 {
		t.Errorf("a warrior was offered a magician's quest: %q", m)
	}
	f.tw.p.Class, f.tw.p.Level = 10, 11
	f.tw.talk(malsis)
	if m := f.msgs(); len(m) != 0 {
		t.Errorf("a level 11 was offered a quest to level 10: %q", m)
	}
	f.tw.p.Level = 5
	f.tw.talk(malsis)
	f.want(f.msgs(), "ask#*")
}

// The player deletes the quest mid-way (0x007D with the journal's token): the
// line goes, the quest is forgotten and offered again. A delete under an old
// token is ignored.
func TestDeletingAQuestMidWay(t *testing.T) {
	f := newQuestFixture(t)
	malsis := f.tw.add(1, &entity.NPC{ServerID: 701, Name: "Malsis", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	f.tw.talk(malsis)
	f.tw.pick(boxID(f.msgs()), 0)
	f.msgs()
	token, lines := f.journal()
	if len(lines) != 1 {
		t.Fatalf("the journal: %q", lines)
	}
	drop := func(token uint32, i int) {
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeDeleteQuest)
		msg.SetData(build(protocol.OpcodeDeleteQuest, 16, func(w *buffer.Writer) {
			w.WriteUint32(token)
			w.Write7BitEncodedInt64(int64(i))
		})[2:])
		if err := f.qe.HandleDrop(f.tw.s, msg); err != nil {
			t.Fatal(err)
		}
	}
	drop(token-1, 0)
	if m := f.msgs(); len(m) != 0 || func() bool { s, _ := f.progress(10010); return s != 1 }() {
		t.Errorf("a delete under an old token was taken: %q", m)
	}
	drop(token, 0)
	f.want(f.msgs(), "drop:0")
	if s, _ := f.progress(10010); s != -1 {
		t.Errorf("the deleted quest's row is still there at step %d", s)
	}
	f.tw.talk(malsis)
	f.want(f.msgs(), "ask#*")
}

// Progress is kept: an engine started anew (a restart, a login) reads the
// quest under way and its journal line.
func TestQuestProgressOutlivesTheEngine(t *testing.T) {
	f := newQuestFixture(t)
	malsis := f.tw.add(1, &entity.NPC{ServerID: 701, Name: "Malsis", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	kellina := f.tw.add(2, &entity.NPC{ServerID: 702, Name: "Kellina", SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	f.tw.talk(malsis)
	f.tw.pick(boxID(f.msgs()), 0)
	f.tw.talk(kellina)
	f.msgs()
	f.qe.Stop()
	again, err := NewQuestEngine(f.db, f.tw.w, f.tw.mgr, f.inv, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, lines := again.Journal(f.sid); len(lines) != 1 || lines[0] != "Bring Kellina two cracked ant pincers." {
		t.Errorf("after a restart the journal is %q", lines)
	}
	if got := again.CharacterQuests(f.sid); len(got) != 1 || got[0].Step != 2 || got[0].Line != 0 {
		t.Errorf("after a restart: %+v", got)
	}
}

// A kill step counts the kills of its creature, a line each, and is done at
// its count; another creature counts for nothing.
func TestAKillStepCountsItsCreature(t *testing.T) {
	f := newQuestFixture(t)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (20, 'Ants', 1, 'Malsis')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, count, journal) VALUES (20, 1, 'kill', 'cracked ant', 2, 'Kill two ants.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (20, 2, 'talk', 'Malsis', 'Return to Malsis.')`)
	f.qe.Reload()
	if _, err := f.qe.SetCharacterQuest(f.sid, 20, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	kill := func(name string) {
		f.qe.killed(entity.KillCredit{NPCName: name, NPCSpawn: 900, Players: []entity.KillPlayer{{Session: f.tw.s, ServerID: f.sid, Name: "Talker"}}})
	}
	kill("a bat")
	if m := f.msgs(); len(m) != 0 {
		t.Errorf("a bat counted: %q", m)
	}
	kill("Cracked Ant")
	f.want(f.msgs(), "chat:Cracked Ant: 1 of 2.")
	kill("cracked ant")
	f.want(f.msgs(), "drop:0", "add:Return to Malsis.")
	if s, _ := f.progress(20); s != 2 {
		t.Errorf("after two ants the step is %d", s)
	}
}

// A GM's changes reach the journal of a character in the world: given, set to
// a step, given up, finished, forgotten.
func TestAGMsQuestChanges(t *testing.T) {
	f := newQuestFixture(t)
	say := func(action string, step int) {
		t.Helper()
		if _, err := f.qe.SetCharacterQuest(f.sid, 10010, action, step); err != nil {
			t.Fatalf("%s: %v", action, err)
		}
	}
	say(QuestGive, 0)
	f.want(f.msgs(), "box:"+questReceived, "add:Go speak to Kellina.")
	say(QuestSetStep, 2)
	f.want(f.msgs(), "drop:0", "add:Bring Kellina two cracked ant pincers.")
	say(QuestGiveUp, 0)
	f.want(f.msgs(), "drop:0")
	say(QuestComplete, 0)
	if s, d := f.progress(10010); s != 0 || d != 1 {
		t.Errorf("completed: step %d done %d", s, d)
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 10010, QuestGiveUp, 0); err == nil {
		t.Error("a quest not under way was given up")
	}
	say(QuestReset, 0)
	if s, _ := f.progress(10010); s != -1 {
		t.Error("a reset quest kept its row")
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 999, QuestGive, 0); err == nil {
		t.Error("a quest that does not exist was given")
	}
}

// A row edited outside the engine (the panel's table, a revert) is read again
// and the journal of a character in the world redrawn.
func TestReloadingACharacterRedrawsItsJournal(t *testing.T) {
	f := newQuestFixture(t)
	if _, err := f.qe.SetCharacterQuest(f.sid, 10010, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	f.exec(`UPDATE charquests SET step = 2 WHERE serverid = ? AND questid = 10010`, f.sid)
	f.qe.ReloadCharacter(f.sid)
	f.want(f.msgs(), "drop:0", "add:Bring Kellina two cracked ant pincers.")
	f.qe.ReloadCharacter(f.sid)
	if m := f.msgs(); len(m) != 0 {
		t.Errorf("an unchanged journal was redrawn: %q", m)
	}
}

// Another dialog's answer (id 0) is not the quests': the quests leave it to
// its own handler.
func TestOtherBoxesAreNotTheQuests(t *testing.T) {
	f := newQuestFixture(t)
	if f.qe.Answer(f.tw.s, f.tw.p, 0, 0) {
		t.Error("the quests took an answer to a box of id 0")
	}
	if !f.qe.Answer(f.tw.s, f.tw.p, questBoxMark|5, 0) {
		t.Error("the quests left an answer to an old quest box to the others")
	}
}

// A line longer than a box shows is cut after a sentence.
func TestLongWordsSplitIntoBoxes(t *testing.T) {
	long := strings.Repeat("A sentence that goes on. ", 12)
	parts := splitSay(long, questBoxChars)
	if len(parts) < 2 {
		t.Fatalf("not split: %d parts", len(parts))
	}
	for _, p := range parts {
		if len([]rune(p)) > questBoxChars || !strings.HasSuffix(p, ".") {
			t.Errorf("part %q: %d characters", p, len([]rune(p)))
		}
	}
}
