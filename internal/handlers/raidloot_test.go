package handlers

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A raid's loot rules (raidloot.go; the GM Jibrish's petitions 1448 and 1451): each rule
// beside its control, on a corpse fixture with a raid of players, each its own session,
// pack and character row, the World's hooks stood in.

type raidLootRig struct {
	t       *testing.T
	f       *corpseFixture
	p       []*entity.Player
	sess    []*session.Session
	windows map[*session.Session]bool
	members []int // the raid's members now, by index
	boss    bool
	now     time.Time
	dice    []int
	holds   []time.Time
	pat     map[string]int
}

func newRaidLootRig(t *testing.T, n int) *raidLootRig {
	t.Helper()
	f := newCorpseFixture(t, 0)
	f.npc.Name = "Lady Vox"
	r := &raidLootRig{t: t, f: f, windows: map[*session.Session]bool{}, now: time.Now(), pat: map[string]int{}}
	f.player.Name, f.player.Level, f.player.Class = "Ann", 50, 0
	r.p, r.sess, r.members = append(r.p, f.player), append(r.sess, f.sess), []int{0}
	for i := 1; i < n; i++ {
		rec := &database.CharacterRecord{CharName: fmt.Sprintf("R%c", 'a'+i), AccountID: 10 + i}
		if err := database.NewSQLiteCharacterStore(f.db).Create(rec); err != nil {
			t.Fatal(err)
		}
		p := &entity.Player{ServerID: entity.EntityID(1001 + i), CharServerID: rec.ServerID, Name: rec.CharName, Level: 50}
		s := testSession()
		s.SetInventoryList(newInvList(nil))
		s.SetBankList(newInvList(nil))
		r.p, r.sess, r.members = append(r.p, p), append(r.sess, s), append(r.members, i)
	}
	f.h.playerOf = func(s *session.Session) *entity.Player {
		for i := range r.sess {
			if r.sess[i] == s {
				return r.p[i]
			}
		}
		return nil
	}
	f.h.raid = raidHooks{
		seat: func(char uint32) (*session.Session, *entity.Player) {
			for i, p := range r.p {
				if p.CharServerID == char {
					return r.sess[i], p
				}
			}
			return nil, nil
		},
		sessions: func(uint32) []*session.Session {
			var out []*session.Session
			for _, i := range r.members {
				out = append(out, r.sess[i])
			}
			return out
		},
		windows: func(s *session.Session) bool { return r.windows[s] },
		boss:    func(string) bool { return r.boss },
		hold: func(_ entity.EntityID, _, until time.Time) bool {
			r.holds = append(r.holds, until)
			return true
		},
		npc:  func(entity.EntityID) *entity.NPC { return f.npc },
		seen: func(_ *session.Session, p *entity.Player) uint32 { return 0x7F000000 | uint32(p.ServerID) },
		seenBy: func(_ *session.Session, id uint32) *entity.Player {
			for _, p := range r.p {
				if 0x7F000000|uint32(p.ServerID) == id {
					return p
				}
			}
			return nil
		},
		decay: func() time.Duration { return 3 * time.Minute },
		now:   func() time.Time { return r.now },
		intn: func(n int) int {
			if len(r.dice) == 0 {
				r.t.Fatal("a roll with no dice left")
			}
			d := r.dice[0]
			r.dice = r.dice[1:]
			if d >= n {
				r.t.Fatalf("a die of %d for %d", d, n)
			}
			return d
		},
	}
	return r
}

// item adds a pattern with its fields: stack, wear slot, no trade, STR, class mask and
// level, onto the creature's own items.
func (r *raidLootRig) item(name string, stack, wear, trade, str, classes, level int) int {
	r.t.Helper()
	var strv any
	if str != 0 {
		strv = str
	}
	res, err := r.f.db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color, str)
		VALUES (50, 0, 0, 0, ?, 0, ?, 1, 0, 0, 0, 0, ?, ?, 100, 0, ?, 0, 0, 0, 0, 0, ?, 'a thing', 0, 0, ?)`,
		wear, trade, level, stack, classes, name, strv)
	if err != nil {
		r.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := r.f.db.Exec(`INSERT INTO npcitem (npcid, stackleft, remainHP, patternid, listnumber)
		VALUES (?, ?, 100, ?, ?)`, corpseNPCID, max(1, stack/2), id, len(r.pat)); err != nil {
		r.t.Fatal(err)
	}
	r.pat[name] = int(id)
	return int(id)
}

// kill is the raid's kill of the fixture's creature under rule, every player a seat but
// those left out.
func (r *raidLootRig) kill(rule entity.LootRule, out ...int) {
	r.t.Helper()
	skip := map[int]bool{}
	for _, i := range out {
		skip[i] = true
	}
	seat := func(i int) entity.RaidSeat {
		p := r.p[i]
		return entity.RaidSeat{ServerID: p.CharServerID, Name: p.Name, Class: p.Class, Race: p.Race, Level: p.Level}
	}
	k := entity.KillCredit{NPCName: r.f.npc.Name, NPCID: r.f.npc.ServerID, Died: r.f.npc.DeathTime,
		Raid: &entity.RaidKill{Group: 1, Rule: rule, Leader: seat(0)}}
	for i := range r.p {
		k.Raid.Members = append(k.Raid.Members, seat(i))
		if !skip[i] {
			k.Raid.Seats = append(k.Raid.Seats, seat(i))
		}
	}
	r.f.h.heardRaidKill(k)
	r.f.h.takeRaidKills()
}

func (r *raidLootRig) msgs(i int) [][]byte {
	var out [][]byte
	for _, m := range r.sess[i].Queue().GetPending() {
		out = append(out, append([]byte(nil), m.Data()...))
	}
	return out
}

// lines are the chat lines session i was queued, drained.
func (r *raidLootRig) lines(i int) []string { return raidLinesOf(r.msgs(i)) }

func raidLinesOf(msgs [][]byte) []string {
	var out []string
	for _, m := range msgs {
		switch opOf(m) {
		case protocol.OpcodeClientMessage, protocol.OpcodeColoredChat, protocol.OpcodeErrorMessage:
			line, _ := readWideLine(m[2:])
			out = append(out, line)
		}
	}
	return out
}

func raidSaid(lines []string, part string) bool {
	for _, l := range lines {
		if strings.Contains(l, part) {
			return true
		}
	}
	return false
}

func (r *raidLootRig) drain() {
	for i := range r.sess {
		r.msgs(i)
	}
}

func (r *raidLootRig) corpse() *corpse {
	ls := r.f.h.loots()
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.corpses[r.f.npc.ServerID]
}

// freeNames is the names of the items anyone may take off the corpse now.
func (r *raidLootRig) freeNames() []string {
	var out []string
	for _, it := range r.corpse().items {
		out = append(out, it.Pattern.ItemName)
	}
	return out
}

// rollOf is the open roll on the item named, or nil.
func (r *raidLootRig) rollOf(name string) *lootRoll {
	ls := r.f.h.loots()
	ls.mu.Lock()
	defer ls.mu.Unlock()
	for _, x := range ls.rolls {
		if x.item.rec.Pattern.ItemName == name {
			return x
		}
	}
	return nil
}

// owns is how many of the item named character i holds (its rows).
func (r *raidLootRig) owns(i int, name string) int {
	r.t.Helper()
	rows, err := r.f.inv.GetCharacterItems(r.p[i].CharServerID)
	if err != nil {
		r.t.Fatal(err)
	}
	n := 0
	for _, row := range rows {
		if row.Pattern.ItemName == name {
			n += row.StackLeft
		}
	}
	return n
}

// The rules hold a raid's no trade items and items with stats, any item a raid boss
// drops but its vendor trash, never a stackable (petition 1448); under free for all
// nothing is held (the control), nor what they leave out.
func TestNeedBeforeGreedHoldsTheRulesItems(t *testing.T) {
	r := newRaidLootRig(t, 3)
	r.item("Fine Steel Sword", 1, 12, 0, 5, 0, 1) // stats
	r.item("Ring of Vox", 1, 7, 1, 0, 0, 1)       // no trade
	r.item("Bone Chips", 20, 0, 0, 0, 0, 1)       // a stack
	r.item("Rat Whisker", 1, 0, 0, 0, 0, 1)       // vendor trash
	r.item("Plain Robe", 1, 18, 0, 0, 0, 1)       // wearable, no stats: a boss's alone
	r.windows[r.sess[1]] = true
	r.kill(entity.LootNeedGreed)
	free := strings.Join(r.freeNames(), ",")
	if free != "Bone Chips,Rat Whisker,Plain Robe" {
		t.Fatalf("free: %s", free)
	}
	for _, n := range []string{"Fine Steel Sword", "Ring of Vox"} {
		if r.rollOf(n) == nil {
			t.Fatalf("no roll on %s", n)
		}
	}
	// a window client's roll: its bytes read as vanilla reads them
	var rolls [][]byte
	for _, m := range r.msgs(1) {
		if opOf(m) == protocol.OpcodeRaidRoll {
			rolls = append(rolls, m)
		}
	}
	if len(rolls) != 2 {
		t.Fatalf("Rb's windows: %d", len(rolls))
	}
	m := rolls[0]
	id := binary.LittleEndian.Uint32(m[2:])
	if secs := binary.LittleEndian.Uint16(m[6:]); secs != 120 || m[8]&1 == 0 || id == 0 {
		t.Fatalf("roll %d: %d s, flags %x", id, secs, m[8])
	}
	rd := buffer.NewReader(m[9:])
	qty, _ := rd.Read7BitEncodedInt64()
	for k := 0; k < 4; k++ {
		rd.Read7BitEncodedInt64()
	}
	key, _ := rd.ReadUint32()
	if qty != 1 || key != id {
		t.Fatalf("the record: qty %d key %d (roll %d)", qty, key, id)
	}
	// a PS2's: the lines
	if l := r.lines(2); !raidSaid(l, fmt.Sprintf("Roll %d: Fine Steel Sword. /t need %d", id, id)) || !raidSaid(l, "Need before greed on Lady Vox's loot") {
		t.Fatalf("Rc's lines: %q", l)
	}
	// a raid boss's robe too; and the control, free for all
	r2 := newRaidLootRig(t, 2)
	r2.boss = true
	r2.item("Plain Robe", 1, 18, 0, 0, 0, 1)
	r2.item("Rat Whisker", 1, 0, 0, 0, 0, 1)
	r2.kill(entity.LootNeedGreed)
	if got := strings.Join(r2.freeNames(), ","); got != "Rat Whisker" || r2.rollOf("Plain Robe") == nil {
		t.Fatalf("a boss's: free %s", got)
	}
	r3 := newRaidLootRig(t, 2)
	r3.item("Fine Steel Sword", 1, 12, 0, 5, 0, 1)
	r3.f.h.heardRaidKill(entity.KillCredit{NPCID: r3.f.npc.ServerID, Died: r3.f.npc.DeathTime}) // no raid: free for all
	r3.f.h.takeRaidKills()
	if r3.corpse() != nil {
		t.Fatal("a free for all kill was rolled at its death")
	}
}

// Any Need beats any Greed and the highest of the highest tier wins; the item goes to
// the winner's pack; all passing leaves it on the corpse for anyone (the control).
func TestNeedBeatsGreedAndTheWinnerGetsIt(t *testing.T) {
	r := newRaidLootRig(t, 3)
	r.item("Fine Steel Sword", 1, 12, 0, 5, 0, 1)
	r.windows[r.sess[0]] = true
	r.kill(entity.LootNeedGreed)
	roll := r.rollOf("Fine Steel Sword")
	r.drain()
	r.dice = []int{99, 3}
	r.f.h.RaidRollChoice(r.sess[0], roll.id, rollGreed)
	r.f.h.RaidRollChoice(r.sess[1], roll.id, rollNeed)
	if l := r.lines(2); !raidSaid(l, "Ann rolls Greed on Fine Steel Sword: 99.") || !raidSaid(l, "Rb rolls Need on Fine Steel Sword: 3.") {
		t.Fatalf("the raid heard %q", l)
	}
	r.f.h.RaidRollChoice(r.sess[2], 0, rollPass) // its oldest
	if r.rollOf("Fine Steel Sword") != nil {
		t.Fatal("the roll goes on with every seat chosen")
	}
	if r.owns(1, "Fine Steel Sword") != 1 || r.owns(0, "Fine Steel Sword") != 0 {
		t.Fatal("Need 3 did not beat Greed 99")
	}
	if l := r.lines(2); !raidSaid(l, "Rb wins Fine Steel Sword with Need 3.") {
		t.Fatalf("no win line: %q", l)
	}
	if strings.Join(r.freeNames(), ",") != "" || len(r.corpse().raid.held) != 0 {
		t.Fatal("the won item is still on the corpse")
	}
	// Ann's window closed at her choice: a 0 seconds roll
	closed := false
	for _, m := range r.msgs(0) {
		closed = closed || opOf(m) == protocol.OpcodeRaidRoll && binary.LittleEndian.Uint16(m[6:]) == 0
	}
	if !closed {
		t.Fatal("Ann's window was not closed")
	}
	// the control: every one passes
	r2 := newRaidLootRig(t, 2)
	r2.item("Fine Steel Sword", 1, 12, 0, 5, 0, 1)
	r2.kill(entity.LootNeedGreed)
	x := r2.rollOf("Fine Steel Sword")
	r2.f.h.RaidRollChoice(r2.sess[0], x.id, rollPass)
	r2.f.h.RaidRollChoice(r2.sess[1], x.id, rollPass)
	if got := strings.Join(r2.freeNames(), ","); got != "Fine Steel Sword" || !raidSaid(r2.lines(0), "No one rolled on Fine Steel Sword") {
		t.Fatalf("all passed: free %q", got)
	}
}

// A tie rolls again between the tied; a choice is taken once (the control).
func TestATieRollsAgain(t *testing.T) {
	r := newRaidLootRig(t, 3)
	r.item("Fine Steel Sword", 1, 12, 0, 5, 0, 1)
	r.kill(entity.LootNeedGreed)
	roll := r.rollOf("Fine Steel Sword")
	r.dice = []int{50, 50, 10, 70}
	r.f.h.RaidRollChoice(r.sess[0], roll.id, rollNeed)
	r.f.h.RaidRollChoice(r.sess[0], roll.id, rollGreed) // again: refused
	r.f.h.RaidRollChoice(r.sess[1], roll.id, rollNeed)
	r.f.h.RaidRollChoice(r.sess[2], roll.id, rollPass)
	if r.owns(1, "Fine Steel Sword") != 1 {
		t.Fatal("the tie's second roll did not decide it")
	}
	l := r.lines(0)
	if !raidSaid(l, "You chose Need on roll") || !raidSaid(l, "A tie at Need 50 on Fine Steel Sword between Ann and Rb: again, Ann 10, Rb 70.") {
		t.Fatalf("Ann heard %q", l)
	}
}

// Need is the equip rules': a seat whose class may not wear it rolls Greed or Pass, its
// window says so and its Need is refused; one that may, needs (the control).
func TestNeedGoesByTheEquipRules(t *testing.T) {
	r := newRaidLootRig(t, 2)
	r.p[1].Class = 9                                // a cleric
	r.item("Warrior's Blade", 1, 12, 0, 5, 1<<0, 1) // warriors' alone
	r.windows[r.sess[0]], r.windows[r.sess[1]] = true, true
	r.kill(entity.LootNeedGreed)
	roll := r.rollOf("Warrior's Blade")
	flags := func(i int) byte {
		for _, m := range r.msgs(i) {
			if opOf(m) == protocol.OpcodeRaidRoll && len(m) > 8 {
				return m[8]
			}
		}
		return 0xFF
	}
	if f0, f1 := flags(0), flags(1); f0 != 1 || f1 != 0 {
		t.Fatalf("may Need: Ann %x, the cleric %x", f0, f1)
	}
	r.f.h.RaidRollChoice(r.sess[1], roll.id, rollNeed)
	if !raidSaid(r.lines(1), "You cannot use Warrior's Blade: Greed or Pass.") || roll.seats[1].choice != rollNone {
		t.Fatal("the cleric's Need was taken")
	}
	r.dice = []int{12}
	r.f.h.RaidRollChoice(r.sess[0], roll.id, rollNeed)
	if roll.seats[0].choice != rollNeed {
		t.Fatal("the warrior's Need was refused")
	}
}

// A winner with a full pack is told; the item waits for it alone, out of everyone's
// window; its opening the corpse puts it in its pack and opens no window; past five
// minutes it is anyone's (the control). A roll no one answers ends at 120 s.
func TestAWonItemWaitsForItsWinner(t *testing.T) {
	r := newRaidLootRig(t, 3)
	r.item("Fine Steel Sword", 1, 12, 0, 5, 0, 1)
	r.item("Bone Chips", 20, 0, 0, 0, 0, 1)
	full := sessionInvList(r.sess[1])
	for k := 0; k < invListCap; k++ {
		full.add(90000+k, 1, 1, 1)
	}
	r.kill(entity.LootNeedGreed)
	roll := r.rollOf("Fine Steel Sword")
	r.dice = []int{40}
	r.f.h.RaidRollChoice(r.sess[1], roll.id, rollNeed)
	r.now = r.now.Add(raidRollFor) // the others never answer
	r.f.h.RaidTick(r.now)
	if r.rollOf("Fine Steel Sword") != nil {
		t.Fatal("the roll outlived its 120 s")
	}
	if l := r.lines(1); !raidSaid(l, "your pack is full") || !raidSaid(l, "Rb wins Fine Steel Sword with Need 40.") {
		t.Fatalf("the winner heard %q", l)
	}
	if got := strings.Join(r.freeNames(), ","); got != "Bone Chips" {
		t.Fatalf("free %q: the won item shows", got)
	}
	// Ann opens: the chips alone
	r.drain()
	r.f.h.OpenCorpse(r.sess[0], r.f.npc)
	if got := r.msgs(0); len(got) == 0 || lootCount(t, got[len(got)-1]) != 1 {
		t.Fatalf("Ann's window: %s", ops(got))
	}
	r.f.h.HandleLootClose(r.sess[0], protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeClientCloseLoot))
	// the winner opens with its pack still full: told, no window
	r.drain()
	r.f.h.OpenCorpse(r.sess[1], r.f.npc)
	got := r.msgs(1)
	for _, m := range got {
		if opOf(m) == protocol.OpcodeLoot {
			t.Fatal("the winner's opening opened the window")
		}
	}
	if !raidSaid(raidLinesOf(got), lootNoRoom) {
		t.Fatalf("no full line: %q", raidLinesOf(got))
	}
	// room made: its opening takes it
	full.recs = full.recs[:len(full.recs)-1]
	r.f.h.OpenCorpse(r.sess[1], r.f.npc)
	if r.owns(1, "Fine Steel Sword") != 1 {
		t.Fatal("the winner's opening did not take its item")
	}
	for _, m := range r.msgs(1) {
		if opOf(m) == protocol.OpcodeLoot {
			t.Fatal("the winner's opening opened the window")
		}
	}
	// the control: unclaimed for five minutes, it is anyone's
	r2 := newRaidLootRig(t, 2)
	r2.item("Fine Steel Sword", 1, 12, 0, 5, 0, 1)
	full2 := sessionInvList(r2.sess[1])
	for k := 0; k < invListCap; k++ {
		full2.add(90000+k, 1, 1, 1)
	}
	r2.kill(entity.LootNeedGreed)
	x := r2.rollOf("Fine Steel Sword")
	r2.dice = []int{40}
	r2.f.h.RaidRollChoice(r2.sess[1], x.id, rollGreed)
	r2.f.h.RaidRollChoice(r2.sess[0], x.id, rollPass)
	r2.now = r2.now.Add(raidHoldFor - time.Second)
	r2.f.h.RaidTick(r2.now)
	if len(r2.freeNames()) != 0 {
		t.Fatal("anyone's before five minutes")
	}
	r2.now = r2.now.Add(2 * time.Second)
	r2.f.h.RaidTick(r2.now)
	if got := strings.Join(r2.freeNames(), ","); got != "Fine Steel Sword" {
		t.Fatalf("after five minutes: %q", got)
	}
	// and the corpse is held to lie until the rolls and the winners' time are over
	if len(r.holds) == 0 || !r.holds[0].After(r.f.npc.DeathTime.Add(raidRollFor+raidHoldFor)) {
		t.Fatalf("holds %v", r.holds)
	}
}

// Under master looter the corpse is the leader's alone: anyone else is told so and gets
// no window; the leader's window comes with its list (0x7E47, again after each resend);
// an Assign puts the item in that member's pack; Abandon lets everyone at it. A kill
// with nothing the rules hold is free for all (the control).
func TestTheMasterLooterHoldsTheCorpse(t *testing.T) {
	r := newRaidLootRig(t, 3)
	r.item("Fine Steel Sword", 1, 12, 0, 5, 0, 1)
	r.item("Bone Chips", 20, 0, 0, 0, 0, 1)
	r.windows[r.sess[0]] = true
	r.kill(entity.LootMaster)
	r.drain()
	r.f.h.OpenCorpse(r.sess[1], r.f.npc)
	got := r.msgs(1)
	if len(got) == 0 || opOf(got[0]) != protocol.OpcodeLootOptions || got[0][2] != lootAnotherOwner {
		t.Fatalf("Rb's opening: %s", ops(got))
	}
	r.f.h.OpenCorpse(r.sess[0], r.f.npc)
	got = r.msgs(0)
	var list []byte
	for _, m := range got {
		if opOf(m) == protocol.OpcodeRaidMasterLoot {
			list = m
		}
	}
	if len(list) < 6 || list[2] != 1 || list[5] != 3 {
		t.Fatalf("the master's list % x", list)
	}
	// Assign the sword (key 0) to Rc by her id
	r.f.h.MasterAssign(r.sess[0], 0, 0x7F000000|uint32(r.p[2].ServerID), "")
	if r.owns(2, "Fine Steel Sword") != 1 {
		t.Fatal("the assign did not give it")
	}
	got = r.msgs(0)
	if ops(got) == "" || opOf(got[len(got)-1]) != protocol.OpcodeRaidMasterLoot {
		t.Fatalf("after the resend: %s (no list)", ops(got))
	}
	if !raidSaid(r.lines(1), "Ann assigns Fine Steel Sword to Rc.") {
		t.Fatal("the raid was not told")
	}
	r.f.h.MasterAbandon(r.sess[0])
	r.drain()
	r.f.h.OpenCorpse(r.sess[1], r.f.npc)
	got = r.msgs(1)
	if len(got) == 0 || opOf(got[len(got)-1]) != protocol.OpcodeLoot {
		t.Fatalf("after Abandon Rb's opening: %s", ops(got))
	}
	// the control
	r2 := newRaidLootRig(t, 2)
	r2.item("Bone Chips", 20, 0, 0, 0, 0, 1)
	r2.kill(entity.LootMaster)
	r2.f.h.OpenCorpse(r2.sess[1], r2.f.npc)
	if got := r2.msgs(1); opOf(got[len(got)-1]) != protocol.OpcodeLoot {
		t.Fatalf("a master looter's kill of nothing the rules hold: %s", ops(got))
	}
}

// A PS2 master looter is shown its items by number and gives by name; past five minutes
// the corpse is anyone's (the control: before, it is not).
func TestAPS2MasterLooterAssignsByName(t *testing.T) {
	r := newRaidLootRig(t, 2)
	r.item("Fine Steel Sword", 1, 12, 0, 5, 0, 1)
	r.item("Ring of Vox", 1, 7, 1, 0, 0, 1)
	r.kill(entity.LootMaster)
	r.drain()
	r.f.h.OpenCorpse(r.sess[0], r.f.npc)
	if l := r.lines(0); !raidSaid(l, "1 Fine Steel Sword, 2 Ring of Vox") || !raidSaid(l, "/t assign <number> <name>") {
		t.Fatalf("the master's lines %q", l)
	}
	r.f.h.HandleLootClose(r.sess[0], protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeClientCloseLoot))
	if !r.f.h.HoldsMasterCorpse(r.sess[0]) || r.f.h.HoldsMasterCorpse(r.sess[1]) {
		t.Fatal("who holds the corpse")
	}
	r.f.h.MasterAssign(r.sess[0], 1, 0, "rb") // its window closed: the corpse it was last listed
	if r.owns(1, "Ring of Vox") != 1 {
		t.Fatal("the typed assign did not give it")
	}
	if l := r.lines(0); !raidSaid(l, "1 Fine Steel Sword.") {
		t.Fatalf("the list again: %q", l)
	}
	r.now = r.now.Add(raidHoldFor - time.Second)
	r.f.h.RaidTick(r.now)
	r.drain()
	r.f.h.OpenCorpse(r.sess[1], r.f.npc)
	if got := r.msgs(1); opOf(got[0]) != protocol.OpcodeLootOptions {
		t.Fatal("the control: anyone's before its five minutes")
	}
	r.now = r.now.Add(2 * time.Second)
	r.f.h.RaidTick(r.now)
	r.f.h.OpenCorpse(r.sess[1], r.f.npc)
	if got := r.msgs(1); opOf(got[len(got)-1]) != protocol.OpcodeLoot {
		t.Fatalf("past five minutes: %s", ops(got))
	}
}

// What the rules hold survives a keep restart: a roll goes on under its number with its
// choices, and ends by the choice still to come.
func TestRaidLootIsKeptOverARestart(t *testing.T) {
	r := newRaidLootRig(t, 2)
	r.item("Fine Steel Sword", 1, 12, 0, 5, 0, 1)
	r.kill(entity.LootNeedGreed)
	roll := r.rollOf("Fine Steel Sword")
	r.dice = []int{77}
	r.f.h.RaidRollChoice(r.sess[0], roll.id, rollNeed)
	prev := keepWindows.Swap(r.f.h)
	t.Cleanup(func() { keepWindows.Store(prev) })
	b, err := json.Marshal(KeepCorpses())
	if err != nil {
		t.Fatal(err)
	}
	var back []KeptCorpse
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	ls := r.f.h.loots()
	ls.mu.Lock()
	ls.corpses, ls.rolls = map[entity.EntityID]*corpse{}, map[int]*lootRoll{}
	ls.mu.Unlock()
	if n := RestoreCorpses(back); n != 1 {
		t.Fatalf("%d back", n)
	}
	x := r.rollOf("Fine Steel Sword")
	if x == nil || x.id != roll.id || x.seats[0].choice != rollNeed || x.seats[0].roll != 77 {
		t.Fatalf("the roll back: %+v", x)
	}
	r.f.h.RaidRollChoice(r.sess[1], roll.id, rollPass)
	if r.owns(0, "Fine Steel Sword") != 1 {
		t.Fatal("the kept roll did not end in Ann's pack")
	}
}

// The wire: 0x7E42's u8, 0x7E45 closed, 0x7E47 off.
func TestRaidWireBytes(t *testing.T) {
	if got := buildRaidOffer(); string(got) != "\x42\x7E\x01" {
		t.Errorf("offer % x", got)
	}
	r := &lootRoll{id: 7}
	if got := rollPayload(r, &rollSeat{closed: true}, time.Now(), false); string(got) != "\x45\x7E\x07\x00\x00\x00\x00\x00" {
		t.Errorf("closed % x", got)
	}
	if got := masterLootOffPayload(); string(got) != "\x47\x7E\x00" {
		t.Errorf("off % x", got)
	}
}

// /help lists every raid command and /invite, /target and /tar (petitions 1452, 1453);
// the check finds one taken out (the control).
func TestHelpListsTheRaidsCommands(t *testing.T) {
	want := []string{"/raidaoe", "/raid)", "/raidinvite", "/raidleave", "/rsay", "/raidkick", "/raidleader", "/raidhelper",
		"/raidassist", "/raidloot", "/need", "/greed", "/pass", "/assign", "/abandon", "/invite", "/target", "/tar)", "/t raidinvite",
		"/tutorial"}
	missing := func(lines []string) []string {
		all := strings.Join(lines, "\n")
		var out []string
		for _, w := range want {
			if !strings.Contains(all, w) {
				out = append(out, w)
			}
		}
		return out
	}
	if m := missing(playerHelpLines); len(m) > 0 {
		t.Fatalf("/help lacks %v", m)
	}
	var cut []string
	for _, l := range playerHelpLines {
		if !strings.Contains(l, "/raidkick") {
			cut = append(cut, l)
		}
	}
	if m := missing(cut); len(m) != 1 || m[0] != "/raidkick" {
		t.Fatalf("the control: %v", m)
	}
}
