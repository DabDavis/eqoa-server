package handlers

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The Metal Moogie chain's turn-ins (work board #688, a GM's): the metal fixture with the four
// parts (lore, no trade) and the rewards in its item records, V3's original spawn-only beside Metal Moogie's.

// item adds an item record of the name; its id.
func (f *metalFixture) item(name string, maxStack, lore int) int {
	f.t.Helper()
	res, err := f.db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, 123, 0, -1, 0, 1, 1, 0, 0, 0, 0, 1, ?, 0, 0, 32767, 32767, 0, ?, 0, 0, ?, '', 0, 0)`, maxStack, lore, name)
	if err != nil {
		f.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

// hand puts one of the item in the player's pack.
func (f *metalFixture) hand(sess *session.Session, p *entity.Player, id int) {
	f.t.Helper()
	f.exec(`INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
		VALUES (?, 1, 0, 0, ?, -1, 0, ?)`, p.CharServerID, id, f.count(p, -1))
	f.loadPack(sess, p)
}

// count is how many of the item's records (-1: of any item) the player's rows hold, and their stacks' sum.
func (f *metalFixture) count(p *entity.Player, id int) int {
	f.t.Helper()
	var n int
	q := `SELECT count(*) FROM charinventory WHERE serverid = ? AND (patternid = ? OR ? = -1)`
	if err := f.db.QueryRow(q, p.CharServerID, id, id).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

// stack is the sum of the item's stacks the player holds.
func (f *metalFixture) stack(p *entity.Player, id int) int {
	f.t.Helper()
	var n int
	if err := f.db.QueryRow(`SELECT COALESCE(sum(stackleft), 0) FROM charinventory WHERE serverid = ? AND patternid = ?`, p.CharServerID, id).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

type chainItems struct{ legs, torso, arms, head, snow, insight, deputy, stone int }

func (f *metalFixture) chainItems() chainItems {
	return chainItems{
		legs: f.item(entity.MetalLegsName, 1, 1), torso: f.item(entity.MetalTorsoName, 1, 1), arms: f.item(entity.MetalArmsName, 1, 1),
		head: f.item(entity.MetalHeadName, 1, 1), snow: f.item("Snowball", 99, 0), insight: f.item("Potion of Moogie's Insight", 1, 0),
		deputy: f.item("Deputy's Draught", 1, 0), stone: f.item("Moogie Winding Stone", 1, 1),
	}
}

// version puts the spawn-only original of the version named beside Metal Moogie's.
func (f *metalFixture) version(name string, id entity.EntityID) *entity.NPC {
	n := &entity.NPC{ServerID: id, Name: name, NPCID: int(id), X: 101, Y: 54, Z: 130, Level: 60, HP: 60000, MaxHP: 60000, Size: 9, SpawnOnly: true}
	n.SpawnX, n.SpawnY, n.SpawnZ = n.X, n.Y, n.Z
	f.w.NPCs[n.ServerID] = n
	f.w.SpatialIndex.Insert(n.ServerID, n.X, n.Z)
	return n
}

// standingNamed is the copy of the name standing, nil none.
func (f *metalFixture) standingNamed(name string) *entity.NPC {
	for _, n := range f.w.NPCs {
		if n.Name == name && !n.SpawnOnly && !n.Dead {
			return n
		}
	}
	return nil
}

// Legs held: Mr. Moogie's box adds their answer (none without them); its question offers the snowballs, V3's
// duel (while V3's original is there) and No; the snowballs taken, 99 go into the pack as one stack, the legs
// leave it (0x003C) and he says "A fair trade"; his box offers the legs no more.
func TestMetalChainLegsForSnowballs(t *testing.T) {
	f := newMetalFixture(t)
	it := f.chainItems()
	f.version(entity.MetalMoogieV3Name, 7790)
	anna, pa := f.join("Anna", 100, 100)
	legs := moogieParts[0]
	if got := f.talk(anna, f.moogie); len(got) != 1 || strings.Contains(got[0], legs.answer) {
		t.Fatalf("no legs held: %q", got)
	}
	f.hand(anna, pa, it.legs)
	got := f.talk(anna, f.moogie)
	want := "|" + strings.Join([]string{moogieClearOpt, moogieHealOpt, moogieResetOpt, moogieProveOpt, legs.answer, moogieNoOpt}, "|")
	if len(got) != 1 || !strings.HasSuffix(got[0], want) {
		t.Fatalf("legs held: %q, want %q", got, want)
	}
	got = f.pick(anna, boxID(got), 4)
	want = legs.question + "|" + strings.Join([]string{"I'll take the snowballs", "Summon Metal Moogie V3", "No"}, "|")
	if len(got) != 1 || !strings.HasSuffix(got[0], want) {
		t.Fatalf("the legs' question: %q, want %q", got, want)
	}
	got = f.pick(anna, boxID(got), 0)
	if !hasMsg(got, "chat:"+fmt.Sprintf(moogieFairTrade, "Anna")) || !hasMsg(got, fmt.Sprintf("op:%04X", uint16(protocol.OpcodeRemoveInvItem))) ||
		!hasMsg(got, fmt.Sprintf("op:%04X", uint16(protocol.OpcodeAddInvItem))) {
		t.Errorf("the snowballs taken: %q", got)
	}
	if f.count(pa, it.legs) != 0 || f.stack(pa, it.snow) != 99 || f.count(pa, it.snow) != 1 {
		t.Errorf("after: legs %d, snowballs %d in %d records", f.count(pa, it.legs), f.stack(pa, it.snow), f.count(pa, it.snow))
	}
	if got := f.talk(anna, f.moogie); len(got) != 1 || strings.Contains(got[0], legs.answer) {
		t.Errorf("his box after the turn-in: %q", got)
	}
	if f.standingNamed(entity.MetalMoogieV3Name) != nil {
		t.Error("V3 rose for the snowballs")
	}
}

// Legs spent on V3: he rises as V2 does, the legs taken only once he stands. Cora's legs while Anna duels:
// refused naming her, kept. With no V3 on the server the question offers no duel.
func TestMetalChainPartTakenOnlyOnceTheNextStands(t *testing.T) {
	f := newMetalFixture(t)
	it := f.chainItems()
	v3 := f.version(entity.MetalMoogieV3Name, 7790)
	anna, pa := f.join("Anna", 100, 100)
	cora, pc := f.join("Cora", 102, 100)
	f.hand(anna, pa, it.legs)
	f.hand(cora, pc, it.legs)
	got := f.talk(anna, f.moogie)
	got = f.pick(anna, boxID(got), 4)
	got = f.pick(anna, boxID(got), 1)
	if !hasMsg(got, "chat:"+fmt.Sprintf(entity.MetalLineRisesN, "Anna", entity.MetalMoogieV3Name)) ||
		!hasMsg(got, fmt.Sprintf("op:%04X", uint16(protocol.OpcodeRemoveInvItem))) {
		t.Errorf("V3's pick: %q", got)
	}
	c := f.standingNamed(entity.MetalMoogieV3Name)
	if c == nil || !c.DuelSpawn() {
		t.Fatal("no V3 standing")
	}
	if f.count(pa, it.legs) != 0 {
		t.Error("Anna's legs not taken once V3 stood")
	}
	got = f.talk(cora, f.moogie)
	got = f.pick(cora, boxID(got), 4)
	got = f.pick(cora, boxID(got), 1)
	if len(got) != 1 || got[0] != "chat:"+fmt.Sprintf(entity.MetalLineBusy, "Anna") || f.count(pc, it.legs) != 1 {
		t.Errorf("Cora's V3 while Anna duels: %q, legs kept %d", got, f.count(pc, it.legs))
	}
	if f.stack(pc, it.snow) != 0 {
		t.Error("Cora given snowballs for a refused duel")
	}
	// no V3 on the server: the reward and No alone
	v3.Name = "Someone Else"
	dora, pd := f.join("Dora", 104, 100)
	f.hand(dora, pd, it.legs)
	got = f.talk(dora, f.moogie)
	got = f.pick(dora, boxID(got), 4)
	if len(got) != 1 || strings.Contains(got[0], "Summon Metal Moogie V3") || !strings.HasSuffix(got[0], "|I'll take the snowballs|No") {
		t.Errorf("the legs' question with no V3: %q", got)
	}
}

// The torso and the arms give their potions; the head asks nothing: the stone at once and his words. Every
// part held with the tribute: the hand-ins do not fit his box beside the rest (CLIENT's six), so one answer
// asks a box of them all.
func TestMetalChainTorsoArmsHeadAndTheTrophyBox(t *testing.T) {
	f := newMetalFixture(t)
	it := f.chainItems()
	anna, pa := f.join("Anna", 100, 100)
	f.hand(anna, pa, it.head)
	got := f.talk(anna, f.moogie)
	got = f.pick(anna, boxID(got), 4)
	if !hasMsg(got, "chat:"+fmt.Sprintf(moogieParts[3].thanks, "Anna")) || f.count(pa, it.stone) != 1 || f.count(pa, it.head) != 0 {
		t.Errorf("the head: %q, stone %d, head %d", got, f.count(pa, it.stone), f.count(pa, it.head))
	}
	for _, c := range []struct {
		part, reward int
		i            int
	}{{it.torso, it.insight, 1}, {it.arms, it.deputy, 2}} {
		f.hand(anna, pa, c.part)
		got := f.talk(anna, f.moogie)
		got = f.pick(anna, boxID(got), 4)
		if len(got) != 1 || !strings.Contains(got[0], moogieParts[c.i].question) {
			t.Fatalf("%s's question: %q", moogieParts[c.i].item, got)
		}
		got = f.pick(anna, boxID(got), 0)
		if f.count(pa, c.part) != 0 || f.count(pa, c.reward) != 1 || !hasMsg(got, "chat:"+fmt.Sprintf(moogieFairTrade, "Anna")) {
			t.Errorf("%s for its reward: %q, part %d, reward %d", moogieParts[c.i].item, got, f.count(pa, c.part), f.count(pa, c.reward))
		}
	}
	// the tribute and a part: one answer for them
	f.give(anna, pa)
	f.hand(anna, pa, it.legs)
	got = f.talk(anna, f.moogie)
	want := "|" + strings.Join([]string{moogieClearOpt, moogieHealOpt, moogieResetOpt, moogieProveOpt, moogieTrophiesOpt, moogieNoOpt}, "|")
	if len(got) != 1 || !strings.HasSuffix(got[0], want) {
		t.Fatalf("tribute and legs held: %q, want %q", got, want)
	}
	got = f.pick(anna, boxID(got), 4)
	want = moogieTrophiesAsk + "|" + strings.Join([]string{moogieTributeOpt, moogieParts[0].answer, "No"}, "|")
	if len(got) != 1 || !strings.HasSuffix(got[0], want) {
		t.Fatalf("the trophies' box: %q, want %q", got, want)
	}
	got = f.pick(anna, boxID(got), 1)
	if len(got) != 1 || !strings.Contains(got[0], moogieParts[0].question) {
		t.Errorf("the legs from the trophies' box: %q", got)
	}
}

// His questions and answers fit a box (questAskChars, questAnswerChars).
func TestMetalChainWordsFitTheBox(t *testing.T) {
	for _, q := range []string{moogieTrophiesAsk} {
		if n := len([]rune(q)); n > questAskChars {
			t.Errorf("%q is %d characters", q, n)
		}
	}
	for _, p := range moogieParts {
		if n := len([]rune(p.question)); n > questAskChars {
			t.Errorf("%q is %d characters, a box shows %d", p.question, n, questAskChars)
		}
		for _, o := range []string{p.answer, p.rewardOpt, p.nextOpt, moogieTrophiesOpt} {
			if n := len([]rune(o)); n > questAnswerChars {
				t.Errorf("answer %q is %d characters", o, n)
			}
		}
	}
}
