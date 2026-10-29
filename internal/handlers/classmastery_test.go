package handlers

import (
	"encoding/binary"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// cmReader reads a 0x1402 reply as CLIENT's VIClientPart4EC58 does (decomp
// viclientpart4ec58.cpp): ReadSigned zigzag varints, raw u32s and floats,
// ReadString's u32 count of wide characters. A reply that does not read to
// its last byte exactly would leave the window reading garbage.
type cmReader struct {
	b   []byte
	o   int
	err error
}

func (r *cmReader) need(n int) bool {
	if r.err == nil && r.o+n > len(r.b) {
		r.err = fmt.Errorf("short at %d (need %d of %d)", r.o, n, len(r.b))
	}
	return r.err == nil
}
func (r *cmReader) u8() int {
	if !r.need(1) {
		return 0
	}
	r.o++
	return int(r.b[r.o-1])
}
func (r *cmReader) u32() uint32 {
	if !r.need(4) {
		return 0
	}
	r.o += 4
	return binary.LittleEndian.Uint32(r.b[r.o-4:])
}
func (r *cmReader) zz() int {
	var u uint32
	for sh := 0; ; sh += 7 {
		c := r.u8()
		if r.err != nil || sh > 28 {
			return 0
		}
		u |= uint32(c&0x7F) << sh
		if c&0x80 == 0 {
			break
		}
	}
	if u&1 != 0 {
		return int(^int32(u >> 1))
	}
	return int(u >> 1)
}
func (r *cmReader) wide(max int) string {
	n := int(r.u32())
	if !r.need(2 * n) {
		return ""
	}
	u := make([]uint16, n)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(r.b[r.o+2*i:])
	}
	r.o += 2 * n
	if n+1 > max {
		r.err = fmt.Errorf("a string of %d characters: CLIENT keeps at most %d", n, max-1)
	}
	return string(utf16.Decode(u))
}

type cmSkill struct {
	key, id, parent, cat, cost int
	owned, known               bool
	name                       string
}

func (r *cmReader) skill() cmSkill {
	s := cmSkill{key: r.zz()}
	s.owned = r.u8() != 0
	s.known = r.u8() != 0
	s.id, s.parent, s.cat, s.cost = r.zz(), r.zz(), r.u8(), r.zz()
	s.name = r.wide(64)
	return s
}

type cmNodeRead struct {
	id, key, parent, cat, level, x14, spent int
	owned, known                            bool
	min                                     [7]int
	name, title, text                       string
	rank                                    int
	mods                                    [][2]int
	abilities                               []string
	adds                                    []bool
	masteries                               []string
}

func (r *cmReader) node() cmNodeRead {
	n := cmNodeRead{key: r.zz()}
	n.owned = r.u8() != 0
	n.known = r.u8() != 0
	n.id, n.parent = r.zz(), r.zz()
	n.cat, n.level = r.u8(), r.u8()
	n.x14, n.spent = r.zz(), r.zz()
	for i := range n.min {
		n.min[i] = r.zz()
	}
	n.name = r.wide(64)
	n.rank = r.zz()
	n.title = r.wide(64)
	n.text = r.wide(256)
	for k := r.zz(); k > 0 && r.err == nil; k-- {
		n.mods = append(n.mods, [2]int{r.u8(), r.zz()})
	}
	for k := r.zz(); k > 0 && r.err == nil; k-- {
		n.abilities = append(n.abilities, r.wide(64))
		n.adds = append(n.adds, r.u8() == 1)
	}
	for k := r.zz(); k > 0 && r.err == nil; k-- {
		n.masteries = append(n.masteries, r.wide(64))
	}
	return n
}

type cmStateRead struct {
	speed   float32
	stats   [29]int
	bonuses [7]int
	flags   map[int]bool
}

func (r *cmReader) state() cmStateRead {
	s := cmStateRead{speed: math.Float32frombits(r.u32()), flags: map[int]bool{}}
	for i := range s.stats {
		s.stats[i] = r.zz()
	}
	for i := range s.bonuses {
		s.bonuses[i] = r.zz()
	}
	for k := int(r.u32()); k > 0 && r.err == nil; k-- {
		id := int(r.u32())
		s.flags[id] = r.u32() != 0
	}
	return s
}

// testCatalog is a small cmcatalog: two general ranks of one chain, Godborn
// (500 spent), a warrior's class mastery that adds an ability, two
// warrior master classes, a gnome's racial and an enchanter's class one.
func testCatalog() []database.CMRankRow {
	all, races := 32767, 2047
	return []database.CMRankRow{
		{ID: 1, Category: 0, Classes: all, Races: races, Mastery: "Brawn", Rank: 1, Name: "Brawn 1", Cost: 10, Level: 10,
			MinStats: "STR:100", Effects: "STR:1,STRMax:15", Description: "Brawn wins most battles.", Enabled: true},
		{ID: 2, Category: 0, Classes: all, Races: races, Mastery: "Brawn", Rank: 2, Name: "Brawn 2", Parent: 1, Cost: 15, Level: 20,
			MinStats: "STR:125", Effects: "STR:2,STRMax:20", Enabled: true},
		{ID: 3, Category: 0, Classes: all, Races: races, Mastery: "Godborn", Rank: 1, Name: "Godborn", Cost: 100, Level: 50, Spent: 500,
			Effects: "STRMax:25,STAMax:25,AGIMax:25,DEXMax:25,WISMax:25,INTMax:25,CHAMax:25", Enabled: true},
		{ID: 10, Category: 3, Classes: 1 << 0, Races: races, Mastery: "Battle Cry", Rank: 1, Name: "Battle Cry 1", Cost: 20, Level: 20,
			Requires: "Taunt@1", RequireSpells: "500", Effects: "AC:25,HP Factor:1", Adds: "Battle Cry@20", AddSpells: "501", Removes: "Taunt@1", RemoveSpells: "500", Enabled: true},
		{ID: 20, Category: 4, Classes: 1 << 0, Races: races, Mastery: "Destroyer", Rank: 1, Name: "Destroyer 1", Cost: 200, Level: 55,
			Effects: "STR:20,STRMax:50", Enabled: true},
		{ID: 21, Category: 4, Classes: 1 << 0, Races: races, Mastery: "Defender", Rank: 1, Name: "Defender 1", Cost: 200, Level: 55,
			Effects: "STA:20,STAMax:50,MovRate:10", Enabled: true},
		{ID: 30, Category: 1, Classes: all, Races: 1 << 3, Mastery: "Tinker", Rank: 1, Name: "Tinker 1", Cost: 5, Level: 10, Enabled: true},
		{ID: 40, Category: 3, Classes: 1 << 12, Races: races, Mastery: "Empathy", Rank: 1, Name: "Empathy 1", Cost: 20, Level: 25, Enabled: true},
	}
}

// fakeBook is a spell book by spell id and name.
type fakeBook struct {
	spells map[int]string
	added  []int
}

func (b *fakeBook) GetCharacterSpells(uint32) ([]database.CharSpellEntry, error) {
	var out []database.CharSpellEntry
	i := 0
	for id, name := range b.spells {
		e := database.CharSpellEntry{AddedOrder: i}
		e.Pattern.SpellID, e.Pattern.SpellName = id, name
		out = append(out, e)
		i++
	}
	return out, nil
}
func (b *fakeBook) AddSpell(_ uint32, id, _ int) error {
	b.spells[id] = fmt.Sprintf("spell %d", id)
	b.added = append(b.added, id)
	return nil
}

// cmTest is a world with one warrior in it, the test catalog, a store and a book.
type cmTest struct {
	t    *testing.T
	w    *chatWorld
	h    *ClassMasteryHandler
	sess *session.Session
	p    *entity.Player
	kept *keptMastery
	book *fakeBook
}

func newCMTest(t *testing.T, class, race, level int) *cmTest {
	c := newChatWorld(t)
	c.gw.SetCMCatalog(testCatalog())
	sess, p := c.join(5, "Buyer", 25000, 15000, 0)
	p.Class, p.Race, p.Level = class, race, level
	p.BaseStr, p.BaseSta, p.BaseAgi, p.BaseDex, p.BaseWis, p.BaseInt, p.BaseCha = 130, 100, 90, 90, 80, 80, 70
	p.RecalcStats(true)
	h := NewClassMasteryHandler(c.sm)
	h.SetGameWorld(c.gw)
	kept := &keptMastery{}
	h.SetStore(kept)
	book := &fakeBook{spells: map[int]string{500: "Taunt"}}
	h.SetSpellbook(book)
	return &cmTest{t: t, w: c, h: h, sess: sess, p: p, kept: kept, book: book}
}

// send handles one 0x1401 and gives back every 0x1402 payload (after the
// opcode) and every red line it queued.
func (c *cmTest) send(sub byte, arg int) (replies [][]byte, red []string) {
	c.sess.Queue().GetPending()
	req := make([]byte, 9)
	req[0] = sub
	binary.LittleEndian.PutUint32(req[5:], uint32(int32(arg)))
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeClassMastery)
	msg.SetData(req)
	if err := c.h.HandleClassMastery(c.sess, msg); err != nil {
		c.t.Fatal(err)
	}
	for _, m := range c.sess.Queue().PeekPending() {
		d := m.Data()
		switch protocol.GameOpcode(binary.LittleEndian.Uint16(d)) {
		case protocol.OpcodeClassMasteryServer:
			replies = append(replies, d[2:])
		case protocol.OpcodeErrorMessage:
			r := &cmReader{b: d[2:]}
			red = append(red, r.wide(1024))
		}
	}
	return replies, red
}

func (c *cmTest) list() []cmSkill {
	replies, _ := c.send(cmOpen, 0)
	if len(replies) != 1 || replies[0][0] != cmRespList {
		c.t.Fatalf("open: replies % x", replies)
	}
	r := &cmReader{b: replies[0], o: 1}
	r.u32()
	n := int(r.u32())
	var out []cmSkill
	for i := 0; i < n; i++ {
		out = append(out, r.skill())
	}
	if r.err != nil || r.o != len(r.b) {
		c.t.Fatalf("the list does not read as CLIENT reads it: %v, %d of %d bytes", r.err, r.o, len(r.b))
	}
	return out
}

func skillIDs(list []cmSkill) map[int]cmSkill {
	m := map[int]cmSkill{}
	for _, s := range list {
		m[s.id] = s
	}
	return m
}

// The list is the character's own: what its class and race are offered, each
// rank chained to the one before, known when it could be bought now. The
// control: a gnome enchanter is offered another set.
func TestCMListIsTheCharactersOwn(t *testing.T) {
	c := newCMTest(t, 0, 0, 60)
	got := skillIDs(c.list())
	for _, id := range []int{1, 2, 3, 10, 20, 21} {
		if _, ok := got[id]; !ok {
			t.Errorf("a human warrior is not offered mastery %d", id)
		}
	}
	for _, id := range []int{30, 40} {
		if _, ok := got[id]; ok {
			t.Errorf("a human warrior is offered mastery %d (a gnome's or an enchanter's)", id)
		}
	}
	if s := got[2]; s.parent != 1 || s.key != 2 || s.cost != 15 || s.name != "Brawn 2" || s.cat != 0 {
		t.Errorf("Brawn 2 listed as %+v", s)
	}
	if !got[1].known || got[2].known || got[3].known {
		t.Errorf("known: Brawn 1 %v (want true), Brawn 2 %v (its parent unowned), Godborn %v (500 unspent)", got[1].known, got[2].known, got[3].known)
	}
	if got[20].cat != 4 || got[10].cat != 3 {
		t.Errorf("categories: master %d, class %d", got[20].cat, got[10].cat)
	}
	other := skillIDs(newCMTest(t, 12, 3, 60).list())
	if _, ok := other[30]; !ok {
		t.Error("the control: a gnome is offered the gnome's mastery")
	}
	if _, ok := other[10]; ok {
		t.Error("the control: an enchanter is not offered the warrior's")
	}
}

// One rank opened reads as CLIENT's VICMNode::Read, to its last byte: what it
// needs, the stats it modifies by the window's slots, the abilities it adds
// (1), removes or needs (0), the mastery it still needs. The control: the
// same bytes one short do not read.
func TestCMNodeReadsAsCLIENTsNode(t *testing.T) {
	c := newCMTest(t, 0, 0, 60)
	replies, red := c.send(cmBrowse, 10)
	if len(red) != 0 || len(replies) != 1 || replies[0][0] != cmRespNode {
		t.Fatalf("browse: % x, red %v", replies, red)
	}
	r := &cmReader{b: replies[0], o: 5}
	n := r.node()
	if r.err != nil || r.o != len(r.b) {
		t.Fatalf("the node does not read as CLIENT reads it: %v, %d of %d", r.err, r.o, len(r.b))
	}
	if n.id != 10 || n.key != 10 || n.cat != 3 || n.level != 20 || n.x14 != 20 || n.title != "Battle Cry 1" {
		t.Errorf("node %+v", n)
	}
	mods := map[int]int{}
	for _, m := range n.mods {
		mods[m[0]] = m[1]
	}
	if mods[18] != 25 || mods[21] != 1 || len(mods) != 2 {
		t.Errorf("mods %v: want AC (18) 25 and HP FACTOR (21) 1", n.mods)
	}
	if strings.Join(n.abilities, "|") != "Battle Cry|Taunt" || !n.adds[0] || n.adds[1] {
		t.Errorf("abilities %v adds %v", n.abilities, n.adds)
	}
	if !n.known {
		t.Error("the warrior knows Taunt: Battle Cry 1 should be known")
	}
	b2, _ := c.send(cmBrowse, 2)
	r2 := &cmReader{b: b2[0], o: 5}
	n2 := r2.node()
	if len(n2.masteries) != 1 || n2.masteries[0] != "Brawn 1" || n2.min[0] != 125 {
		t.Errorf("Brawn 2 needs %v and STR %d: want Brawn 1 and 125", n2.masteries, n2.min[0])
	}
	short := &cmReader{b: replies[0][:len(replies[0])-1], o: 5}
	short.node()
	if short.err == nil {
		t.Fatal("the control: a node a byte short must not read")
	}
}

// A purchase spends the points, owns the rank, gives what it gives (the cap
// raised) and answers CLIENT's Recv05, whose known flags open the next rank;
// what it lacks is refused in red and answered Recv02. The controls: each
// refusal changes nothing.
func TestCMBuyAndItsRefusals(t *testing.T) {
	c := newCMTest(t, 0, 0, 60)
	refused := func(id int, want string) {
		t.Helper()
		before := c.p.Mastery
		replies, red := c.send(cmBuy, id)
		if len(replies) != 1 || replies[0][0] != cmRespDone || len(red) != 1 || !strings.Contains(red[0], want) {
			t.Errorf("buying %d: replies % x, red %v; want Recv02 and %q", id, replies, red, want)
		}
		if c.p.Mastery != before {
			t.Errorf("buying %d refused changed the points: %+v -> %+v", id, before, c.p.Mastery)
		}
	}
	refused(1, "mastery points")
	c.p.Mastery.Points = 600
	refused(2, "Brawn 1 first")
	refused(3, "spent 500")
	refused(40, "not offered")

	replies, red := c.send(cmBuy, 1)
	if len(red) != 0 || len(replies) != 1 || replies[0][0] != cmRespBought {
		t.Fatalf("buying Brawn 1: % x, red %v", replies, red)
	}
	r := &cmReader{b: replies[0], o: 5}
	id, unspent, spent := r.u32(), r.u32(), r.u32()
	st := r.state()
	if r.err != nil || r.o != len(r.b) {
		t.Fatalf("Recv05 does not read as CLIENT reads it: %v, %d of %d", r.err, r.o, len(r.b))
	}
	if id != 1 || unspent != 590 || spent != 10 {
		t.Errorf("Recv05 id %d, points %d/%d: want 1, 590/10", id, unspent, spent)
	}
	if st.stats[0] != 1 || st.stats[7] != 15 || st.bonuses[0] != 15 {
		t.Errorf("the masteries' stats: STR %d, STR cap %d, bonus %d", st.stats[0], st.stats[7], st.bonuses[0])
	}
	if !st.flags[2] {
		t.Error("Brawn 2 should be known once Brawn 1 is owned (STR 131 of 125)")
	}
	if c.p.StatCapOf(entity.StatStr) != 415 || c.p.StatCapOf(entity.StatSta) != 400 {
		t.Errorf("caps STR %d, STA %d: want 415, 400", c.p.StatCapOf(entity.StatStr), c.p.StatCapOf(entity.StatSta))
	}
	if c.kept.owned[c.p.CharServerID][1] != 10 || c.kept.rows[c.p.CharServerID].Points != 590 {
		t.Errorf("not kept: owned %v, points %+v", c.kept.owned, c.kept.rows)
	}
	refused(1, "already have")

	// a master class, then another refused until the first is removed
	if rs, red := c.send(cmBuy, 20); len(red) != 0 || rs[0][0] != cmRespBought {
		t.Fatalf("buying Destroyer 1: % x %v", rs, red)
	}
	refused(21, entity.OneMasterClass)
	if rs, red := c.send(cmRemove, 20); len(red) != 0 || len(rs) != 2 || rs[0][0] != cmRespRemoved || rs[1][0] != cmRespPoints {
		t.Fatalf("removing Destroyer 1: % x %v", rs, red)
	}
	if rs, red := c.send(cmBuy, 21); len(red) != 0 || rs[0][0] != cmRespBought {
		t.Errorf("buying Defender 1 after removing Destroyer 1: % x %v", rs, red)
	}
	if c.p.MoveRateBonus != 10 {
		t.Errorf("Defender 1's movement: %d, want 10", c.p.MoveRateBonus)
	}

	low := newCMTest(t, 0, 0, 5)
	low.p.Mastery.Points = 50
	before := low.p.Mastery
	if _, red := low.send(cmBuy, 1); len(red) != 1 || !strings.Contains(red[0], "level 10") || low.p.Mastery != before {
		t.Errorf("a level 5 buying Brawn 1: %v", red)
	}
	weak := newCMTest(t, 0, 0, 60)
	weak.p.BaseStr = 90
	weak.p.Mastery.Points = 50
	if _, red := weak.send(cmBuy, 1); len(red) != 1 || !strings.Contains(red[0], "STR must be at least 100") {
		t.Errorf("STR 90 buying Brawn 1: %v", red)
	}

	// an ability it needs (by the spell it was found as) must be in the book;
	// one never found is shown and not checked
	untaught := newCMTest(t, 0, 0, 60)
	untaught.p.Mastery.Points = 50
	delete(untaught.book.spells, 500)
	if _, red := untaught.send(cmBuy, 10); len(red) != 1 || !strings.Contains(red[0], "must know Taunt") {
		t.Errorf("buying Battle Cry 1 without Taunt: %v", red)
	}
	rows := testCatalog()
	rows[3].RequireSpells = "0"
	untaught.w.gw.SetCMCatalog(rows)
	if rs, red := untaught.send(cmBuy, 10); len(red) != 0 || rs[0][0] != cmRespBought {
		t.Errorf("buying it when Taunt was never found: % x %v", rs, red)
	}
}

// A rank may need another mastery's rank (require_ranks): it is refused, not
// known and said in the window until that rank is owned, and the rank it
// needs is not removed before it. An id the catalog lacks is not checked.
// The control: the same rank with nothing in require_ranks is bought at once.
func TestCMNeedsAnotherMasterysRank(t *testing.T) {
	needs := func(ids string) []database.CMRankRow {
		rows := testCatalog()
		for i := range rows {
			if rows[i].ID == 20 {
				rows[i].RequireRanks = ids
			}
		}
		return rows
	}
	known := func(c *cmTest, id int) bool {
		for _, e := range c.w.gw.CMList(c.sess, entity.CMKnows{Book: map[int]bool{500: true}}) {
			if e.ID == id {
				return e.Known
			}
		}
		return false
	}

	control := newCMTest(t, 0, 0, 60)
	control.p.Mastery.Points = 600
	if rs, red := control.send(cmBuy, 20); len(red) != 0 || rs[0][0] != cmRespBought {
		t.Fatalf("the control: Destroyer 1 with no rank needed: % x %v", rs, red)
	}

	c := newCMTest(t, 0, 0, 60)
	c.w.gw.SetCMCatalog(needs("2"))
	c.p.Mastery.Points = 600
	if known(c, 20) {
		t.Error("Destroyer 1 is known without Brawn 2")
	}
	if n, ok := c.w.gw.CMNodeOf(c.sess, 20, entity.CMKnows{}); !ok || !slices.Contains(n.Masteries, "Brawn 2") || !strings.Contains(n.Text, "Brawn 2") {
		t.Errorf("the window does not say Brawn 2 is needed: %v %+v", ok, n)
	}
	before := c.p.Mastery
	if _, red := c.send(cmBuy, 20); len(red) != 1 || !strings.Contains(red[0], "You must have Brawn 2 first.") || c.p.Mastery != before {
		t.Errorf("buying Destroyer 1 without Brawn 2: %v", red)
	}
	c.send(cmBuy, 1)
	c.send(cmBuy, 2)
	if !known(c, 20) {
		t.Error("Destroyer 1 is not known with Brawn 2 owned")
	}
	if rs, red := c.send(cmBuy, 20); len(red) != 0 || rs[0][0] != cmRespBought {
		t.Fatalf("buying Destroyer 1 with Brawn 2: % x %v", rs, red)
	}
	if n, _ := c.w.gw.CMNodeOf(c.sess, 21, entity.CMKnows{}); slices.Contains(n.Masteries, "Brawn 2") {
		t.Errorf("Defender 1 says a rank it does not need: %v", n.Masteries)
	}
	if _, red := c.send(cmRemove, 2); len(red) != 1 || !strings.Contains(red[0], "Remove Destroyer 1 before Brawn 2") {
		t.Errorf("removing Brawn 2 under Destroyer 1: %v", red)
	}
	if _, ok := c.p.CMs[2]; !ok {
		t.Error("Brawn 2 was removed")
	}

	unknown := newCMTest(t, 0, 0, 60)
	unknown.w.gw.SetCMCatalog(needs("999"))
	unknown.p.Mastery.Points = 600
	if rs, red := unknown.send(cmBuy, 20); len(red) != 0 || rs[0][0] != cmRespBought {
		t.Errorf("a rank needed that the catalog lacks blocked Destroyer 1: % x %v", rs, red)
	}
}

// A rank may need a quest finished (require_quests: the stat-boost quests open
// the stat-boost lines): until the character has finished it the rank is
// refused, not known, and the window says which quest; once the quest is done
// (played through, the engine's own count) the finish says the rank is open,
// and it is known and bought. A quest not
// in the game bars no one, as a trip a draft unlocks: one the database lacks,
// and a draft until a GM puts it in the game, which counts at once.
func TestCMNeedsAQuestFinished(t *testing.T) {
	f := newQuestFixture(t)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (10020, 'A draft', 0, 'Malsis')`)
	f.exec(`INSERT INTO cmcatalog (id, mastery, rank, name, parent, cost, level, effects, require_quests) VALUES
		(11, 'Buldging Muscles', 1, 'Buldging Muscles 1', 0, 5, 1, 'STR:10', '10010'),
		(12, 'Buldging Muscles', 2, 'Buldging Muscles 2', 11, 10, 1, 'STR:20', '999999'),
		(31, 'Intestinal Fortitude', 1, 'Intestinal Fortitude 1', 0, 5, 1, 'STA:10', '10020'),
		(32, 'Intestinal Fortitude', 2, 'Intestinal Fortitude 2', 31, 10, 1, 'STA:20', '10020')`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.tw.w.LoadCMCatalog(f.db); err != nil {
		t.Fatal(err)
	}
	h := NewClassMasteryHandler(f.tw.mgr)
	h.SetGameWorld(f.tw.w)
	kept := &keptMastery{}
	h.SetStore(kept)
	h.SetSpellbook(&fakeBook{spells: map[int]string{}})
	c := &cmTest{t: t, h: h, sess: f.tw.s, p: f.tw.p, kept: kept}
	c.p.Mastery.Points = 100
	known := func(id int) bool {
		for _, e := range f.tw.w.CMList(c.sess, h.cmKnowsOf(c.p)) {
			if e.ID == id {
				return e.Known
			}
		}
		return false
	}
	const why = "You must finish The pincers to buy Buldging Muscles 1."

	if known(11) {
		t.Error("Buldging Muscles 1 is known with its quest not done")
	}
	if n, ok := f.tw.w.CMNodeOf(c.sess, 11, h.cmKnowsOf(c.p)); !ok || !strings.Contains(n.Text, why) {
		t.Errorf("the window does not say the quest is needed: %v %q", ok, n.Text)
	}
	before := c.p.Mastery
	if _, red := c.send(cmBuy, 11); len(red) != 1 || red[0] != why || c.p.Mastery != before {
		t.Errorf("buying Buldging Muscles 1 with the quest not done: %v", red)
	}

	// the draft bars no one; put in the game, it bars the next rank at once
	if rs, red := c.send(cmBuy, 31); len(red) != 0 || rs[0][0] != cmRespBought {
		t.Errorf("a draft's quest blocked Intestinal Fortitude 1: % x %v", rs, red)
	}
	f.exec(`UPDATE quests SET enabled = 1 WHERE questid = 10020`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	if _, red := c.send(cmBuy, 32); len(red) != 1 || red[0] != "You must finish A draft to buy Intestinal Fortitude 2." {
		t.Errorf("buying Intestinal Fortitude 2 once its quest is in the game: %v", red)
	}

	malsis := f.tw.add(1, &entity.NPC{ServerID: 701, Name: "Malsis", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	kellina := f.tw.add(2, &entity.NPC{ServerID: 702, Name: "Kellina", SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	f.tw.talk(malsis)
	f.tw.pick(boxID(f.msgs()), 0)
	f.tw.talk(kellina)
	f.own(f.pincer, 2)
	f.tw.talk(kellina)
	f.tw.pick(boxID(f.msgs()), 0)
	m := f.msgs()
	if _, done := f.progress(10010); done != 1 {
		t.Fatalf("the quest was not finished (done %d)", done)
	}
	f.want(m, "box:"+questFinished, "box:Buldging Muscles 1 is now available for purchase.")
	f.not(m, "box:Buldging Muscles 2*", "box:Intestinal Fortitude*")

	if !known(11) {
		t.Error("Buldging Muscles 1 is not known with its quest done")
	}
	if rs, red := c.send(cmBuy, 11); len(red) != 0 || rs[0][0] != cmRespBought {
		t.Fatalf("buying Buldging Muscles 1 with the quest done: % x %v", rs, red)
	}
	if rs, red := c.send(cmBuy, 12); len(red) != 0 || rs[0][0] != cmRespBought {
		t.Errorf("a quest the database lacks blocked Buldging Muscles 2: % x %v", rs, red)
	}
}

// A removal gives nothing back (jib, F3 20261026-043542): the rank's cost
// leaves the spent points and the unspent stay, so the character holds that
// many fewer, to earn again; the later rank goes first; a rank whose
// abilities it added takes them back and gives back those it took. The
// control: the points Recv03 carries are the unspent as before the removal.
func TestCMRemoveGivesNothingBackAndKeepsOrder(t *testing.T) {
	c := newCMTest(t, 0, 0, 60)
	c.p.Mastery.Points = 100
	c.send(cmBuy, 1)
	c.send(cmBuy, 2)
	if _, red := c.send(cmRemove, 1); len(red) != 1 || !strings.Contains(red[0], "Remove Brawn 2 before Brawn 1") {
		t.Errorf("removing Brawn 1 under Brawn 2: %v", red)
	}
	rs, red := c.send(cmRemove, 2)
	if len(red) != 0 || len(rs) != 2 || rs[0][0] != cmRespRemoved {
		t.Fatalf("removing Brawn 2: % x %v", rs, red)
	}
	r := &cmReader{b: rs[0], o: 5}
	if id := r.u32(); id != 2 {
		t.Errorf("Recv06 id %d", id)
	}
	r.state()
	if r.err != nil || r.o != len(r.b) {
		t.Fatalf("Recv06 does not read as CLIENT reads it: %v", r.err)
	}
	if c.p.Mastery.Points != 75 || c.p.Mastery.Spent != 10 || c.p.StatCapOf(entity.StatStr) != 415 {
		t.Errorf("after removing Brawn 2: points %+v, STR cap %d; want 75 unspent, 10 spent", c.p.Mastery, c.p.StatCapOf(entity.StatStr))
	}
	pts := &cmReader{b: rs[1], o: 1}
	if un, sp := pts.u32(), pts.u32(); pts.err != nil || un != 75 || sp != 10 {
		t.Errorf("Recv03 after the removal: %d unspent, %d spent; want 75, 10 (nothing back)", un, sp)
	}
	if c.kept.rows[c.p.CharServerID].Points != 75 || c.kept.rows[c.p.CharServerID].Spent != 10 {
		t.Errorf("kept after the removal: %+v", c.kept.rows[c.p.CharServerID])
	}

	// abilities: Battle Cry 1 adds 501 and takes Taunt (500)
	c.p.Mastery.Points = 100
	c.send(cmBuy, 10)
	if len(c.book.added) != 1 || c.book.added[0] != 501 || len(c.kept.removedSpells) != 1 || c.kept.removedSpells[0] != 500 {
		t.Errorf("Battle Cry 1: added %v, taken %v; want 501 added, 500 taken", c.book.added, c.kept.removedSpells)
	}
}

// A rank that adds and removes one spell improves it in place (EQOA-Data's
// Quick Blade, Hydromancy...; jib's review, F3 pass 19): buying it leaves the
// spell in the book, so the next rank, which needs the same spell, can be
// bought; removing it and #cmreset leave it too. The control: Battle Cry 1,
// which adds one spell and removes another, still swaps them.
func TestCMRankImprovingASpellKeepsIt(t *testing.T) {
	rows := append(testCatalog(),
		database.CMRankRow{ID: 11, Category: 3, Classes: 1 << 0, Races: 2047, Mastery: "Keen Taunt", Rank: 1, Name: "Keen Taunt 1",
			Cost: 20, Level: 20, Requires: "Taunt@1", RequireSpells: "500", Adds: "Taunt@1", AddSpells: "500",
			Removes: "Taunt@1", RemoveSpells: "500", Enabled: true},
		database.CMRankRow{ID: 12, Category: 3, Classes: 1 << 0, Races: 2047, Mastery: "Keen Taunt", Rank: 2, Name: "Keen Taunt 2",
			Parent: 11, Cost: 20, Level: 30, Requires: "Taunt@1", RequireSpells: "500", Enabled: true})
	c := newCMTest(t, 0, 0, 60)
	c.w.gw.SetCMCatalog(rows)
	c.kept.book = c.book
	c.p.Mastery.Points = 200

	if rs, red := c.send(cmBuy, 11); len(red) != 0 || rs[0][0] != cmRespBought {
		t.Fatalf("buying Keen Taunt 1: % x %v", rs, red)
	}
	if _, ok := c.book.spells[500]; !ok || len(c.kept.removedSpells) != 0 || len(c.book.added) != 0 {
		t.Errorf("Keen Taunt 1 changed the book: Taunt kept %v, taken %v, added %v", ok, c.kept.removedSpells, c.book.added)
	}
	if rs, red := c.send(cmBuy, 12); len(red) != 0 || rs[0][0] != cmRespBought {
		t.Errorf("Keen Taunt 2, which needs Taunt, after Keen Taunt 1: % x %v", rs, red)
	}
	c.send(cmRemove, 12)
	if rs, red := c.send(cmRemove, 11); len(red) != 0 || rs[0][0] != cmRespRemoved {
		t.Fatalf("removing Keen Taunt 1: % x %v", rs, red)
	}
	if _, ok := c.book.spells[500]; !ok || len(c.kept.removedSpells) != 0 || len(c.book.added) != 0 {
		t.Errorf("removing Keen Taunt 1 changed the book: Taunt kept %v, taken %v, added %v", ok, c.kept.removedSpells, c.book.added)
	}
	c.send(cmBuy, 11)
	c.h.GMReset(c.sess, 0)
	if _, ok := c.book.spells[500]; !ok || len(c.kept.removedSpells) != 0 || len(c.book.added) != 0 {
		t.Errorf("#cmreset changed the book: Taunt kept %v, taken %v, added %v", ok, c.kept.removedSpells, c.book.added)
	}

	c.send(cmBuy, 10)
	if _, ok := c.book.spells[500]; ok || len(c.book.added) != 1 || c.book.added[0] != 501 || !slices.Equal(c.kept.removedSpells, []int{500}) {
		t.Errorf("the control: Battle Cry 1 added %v and took %v, Taunt kept %v; want 501 added, 500 taken", c.book.added, c.kept.removedSpells, ok)
	}
}

// What was bought is the character's after a restart: its ranks put back at
// the login give the same stats. The control: a player with none has the
// level's cap alone.
func TestCMOwnershipSurvivesARestart(t *testing.T) {
	c := newCMTest(t, 0, 0, 60)
	c.p.Mastery.Points = 200
	c.send(cmBuy, 1)
	c.send(cmBuy, 2)
	capBefore := c.p.StatCapOf(entity.StatStr)
	strBefore := c.p.StatNow(entity.StatStr)

	again := newCMTest(t, 0, 0, 60)
	if again.p.StatCapOf(entity.StatStr) != 400 {
		t.Fatalf("the control: no masteries, cap %d, want 400", again.p.StatCapOf(entity.StatStr))
	}
	var owned []database.OwnedCM
	for id, cost := range c.kept.owned[c.p.CharServerID] {
		owned = append(owned, database.OwnedCM{ID: id, Cost: cost})
	}
	again.w.gw.RestoreCMs(again.sess, owned)
	if again.p.StatCapOf(entity.StatStr) != capBefore || again.p.StatNow(entity.StatStr) != strBefore {
		t.Errorf("after the restart: cap %d STR %d, before %d %d", again.p.StatCapOf(entity.StatStr), again.p.StatNow(entity.StatStr), capBefore, strBefore)
	}
}

// TestEveryCMRequestEndsTheWait is the regression the handler exists for:
// the window has NO client-side timeout, and only Recv01, 02, 05, 06, 08 and
// 09 end its wait. Every sub-op, bought or refused, gets one of those as its
// first reply, and none but the share gets Recv01, which puts its argument
// where CLIENT keeps the share. The control: the old answer to a buy (Recv01
// with the skill's id) fails.
func TestEveryCMRequestEndsTheWait(t *testing.T) {
	ends := map[byte]bool{1: true, 2: true, 5: true, 6: true, 8: true, 9: true}
	check := func(sub byte, reply []byte) string {
		if len(reply) == 0 || !ends[reply[0]] {
			return "no answer that ends the wait"
		}
		if reply[0] == cmRespShare && sub != cmShare {
			return "Recv01 for a sub-op that is not the share"
		}
		return ""
	}
	c := newCMTest(t, 0, 0, 60)
	c.p.Mastery.Points = 1000
	for i := 0; i < 256; i++ {
		for _, arg := range []int{1, 99} {
			sub := byte(i)
			replies, _ := c.send(sub, arg)
			if len(replies) == 0 {
				t.Errorf("sub-op %#x arg %d: no reply: the CM window would hang", sub, arg)
				continue
			}
			if why := check(sub, replies[0]); why != "" {
				t.Errorf("sub-op %#x arg %d: %s", sub, arg, why)
			}
		}
	}
	// a handler with no world (nothing of the character's to say) answers too
	bare := *c
	bare.h = NewClassMasteryHandler(c.w.sm)
	for i := 0; i < 256; i++ {
		replies, _ := bare.send(byte(i), 3)
		if len(replies) != 1 {
			t.Errorf("sub-op %#x without a world: %d replies", i, len(replies))
		} else if why := check(byte(i), replies[0]); why != "" {
			t.Errorf("sub-op %#x without a world: %s", i, why)
		}
	}
	old := []byte{0x01, 0, 0, 0, 0, 0x6A, 0x01, 0, 0}
	if check(cmBuy, old) == "" {
		t.Fatal("the control: the old buy answer must fail")
	}
}

// The share's answer is CLIENT's Recv01: the token, then the share it then
// shows. Retail answered `01 00 00 00 00 06 00 00 00` with the same bytes:
// a share of 6% (cm-wire-format.md had it as "select item 6").
func TestCMShareAnswerIsRecv01(t *testing.T) {
	want := []byte{0x01, 0, 0, 0, 0, 0x06, 0, 0, 0}
	if got := cmShareReply(0, 6)[2:]; string(got) != string(want) {
		t.Errorf("share 6: % x, want % x", got, want)
	}
	if got := cmShareReply(0x2A, 100)[2:]; binary.LittleEndian.Uint32(got[1:]) != 0x2A || binary.LittleEndian.Uint32(got[5:]) != 100 {
		t.Errorf("share 100 under token 0x2A: % x", got)
	}
}

// The share a player sets is the player's, within 0-100 as CLIENT's Select
// clamps it, kept, and answered with what was kept. The control: a request
// for 150 is not kept as 150.
func TestTheShareIsKept(t *testing.T) {
	c := newChatWorld(t)
	sess, p := c.join(3, "Sharer", 25000, 15000, 0)
	h := NewClassMasteryHandler(c.sm)
	h.SetGameWorld(c.gw)
	kept := &keptMastery{}
	h.SetStore(kept)
	for _, c := range []struct{ asked, want int }{{42, 42}, {150, 100}, {-3, 0}, {100, 100}} {
		sess.Queue().GetPending()
		req := make([]byte, 9)
		req[0] = cmShare
		binary.LittleEndian.PutUint32(req[1:], 0)
		binary.LittleEndian.PutUint32(req[5:], uint32(int32(c.asked)))
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeClassMastery)
		msg.SetData(req)
		if err := h.HandleClassMastery(sess, msg); err != nil {
			t.Fatal(err)
		}
		if p.Mastery.Percent != c.want || kept.rows[p.CharServerID].Percent != c.want {
			t.Errorf("asked %d: share %d, kept %d; want %d", c.asked, p.Mastery.Percent, kept.rows[p.CharServerID].Percent, c.want)
		}
		var reply []byte
		for _, m := range sess.Queue().PeekPending() {
			if d := m.Data(); protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeClassMasteryServer {
				reply = d[2:]
			}
		}
		if len(reply) != 9 || reply[0] != cmRespShare || binary.LittleEndian.Uint32(reply[5:]) != uint32(c.want) {
			t.Errorf("asked %d: answered % x", c.asked, reply)
		}
	}
	if p.Mastery.Percent == 150 {
		t.Fatal("the control: 150 must not be kept")
	}
}

// keptMastery is a store of mastery rows, ranks owned and spells taken out,
// by character; with a book, a spell taken out leaves it.
type keptMastery struct {
	rows          map[uint32]database.MasteryRow
	owned         map[uint32]map[int]int
	removedSpells []int
	book          *fakeBook
}

func (k *keptMastery) SaveMastery(id uint32, m database.MasteryRow) error {
	if k.rows == nil {
		k.rows = map[uint32]database.MasteryRow{}
	}
	k.rows[id] = m
	return nil
}

func (k *keptMastery) BuyCM(id uint32, o database.OwnedCM, m database.MasteryRow) error {
	if k.owned == nil {
		k.owned = map[uint32]map[int]int{}
	}
	if k.owned[id] == nil {
		k.owned[id] = map[int]int{}
	}
	k.owned[id][o.ID] = o.Cost
	return k.SaveMastery(id, m)
}

func (k *keptMastery) RemoveCMs(id uint32, ids []int, m database.MasteryRow) error {
	for _, cm := range ids {
		delete(k.owned[id], cm)
	}
	return k.SaveMastery(id, m)
}

func (k *keptMastery) RemoveSpellFromBook(_ uint32, spellID int) (bool, error) {
	k.removedSpells = append(k.removedSpells, spellID)
	if k.book != nil {
		delete(k.book.spells, spellID)
	}
	return true, nil
}

// A second Master Class is refused with the live client's word 1840 before
// anything else it needs is looked at (the live client tests it first): a
// warrior holding Destroyer 1, below Defender 1's level 55, is told 1840, not
// the level, and the list shows Defender 1 locked. The control: with
// Destroyer 1 gone the same buy is refused for the level.
func TestCMOneMasterClassIsWord1840(t *testing.T) {
	c := newCMTest(t, 0, 0, 60)
	c.p.Mastery.Points = 1000
	if rs, red := c.send(cmBuy, 20); len(red) != 0 || rs[0][0] != cmRespBought {
		t.Fatalf("buying Destroyer 1: % x %v", rs, red)
	}
	c.p.Level = 50
	if replies, red := c.send(cmBuy, 21); len(replies) != 1 || replies[0][0] != cmRespDone || len(red) != 1 || red[0] != entity.OneMasterClass {
		t.Fatalf("a second master class: replies % x, red %q", replies, red)
	}
	if s := skillIDs(c.list())[21]; s.known {
		t.Error("Defender 1 listed as buyable beside Destroyer 1")
	}
	c.p.Level = 60
	if rs, red := c.send(cmRemove, 20); len(red) != 0 || len(rs) != 2 {
		t.Fatalf("removing Destroyer 1: % x %v", rs, red)
	}
	c.p.Level = 50
	if _, red := c.send(cmBuy, 21); len(red) != 1 || !strings.Contains(red[0], "level 55") {
		t.Errorf("the control, no master class held: %q", red)
	}
}
