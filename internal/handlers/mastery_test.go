package handlers

import (
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// masteryPoints is the unspent and spent points of the last 0x1402 sub-op 3
// queued for s.
func masteryPoints(s *session.Session) (unspent, spent int, ok bool) {
	for _, m := range s.Queue().PeekPending() {
		d := m.Data()
		if len(d) == 11 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeClassMasteryServer && d[2] == 3 {
			unspent, spent, ok = int(binary.LittleEndian.Uint32(d[3:])), int(binary.LittleEndian.Uint32(d[7:])), true
		}
	}
	return
}

// #grantcm is a GM's, gives what jib asked (never past 1500 spent and
// unspent together), keeps it and tells the client as a gain is told. The
// control: a player's #grantcm gives nothing.
func TestGrantCMIsAGMsAndCapped(t *testing.T) {
	c, gs, gp, ps, pp := gmWorld(t)
	kept := &keptMastery{}
	c.h.SetMasteryStore(kept)

	c.say(ps, "#grantcm 500")
	if pp.Mastery.Points != 0 {
		t.Fatalf("the control: a player's #grantcm gave %d", pp.Mastery.Points)
	}

	gp.Mastery = entity.Mastery{Points: 50, Spent: 50}
	c.say(gs, "#grantcm 100")
	if gp.Mastery.Points != 150 || kept.rows[gp.CharServerID].Points != 150 {
		t.Fatalf("50/50 + 100: %d unspent, kept %d; want 150", gp.Mastery.Points, kept.rows[gp.CharServerID].Points)
	}
	if u, s, ok := masteryPoints(gs); !ok || u != 150 || s != 50 {
		t.Errorf("told %d/%d (%v), want 150/50", u, s, ok)
	}
	gs.Queue().GetPending()
	c.say(gs, "/grantcm 1500")
	if gp.Mastery.Points != 1450 {
		t.Fatalf("150/50 + 1500: %d unspent, want 1450 (1500 in all)", gp.Mastery.Points)
	}
	if got := texts(heard(t, gs)); len(got) == 0 || !strings.Contains(got[len(got)-1], "Granted 1300 of 1500") {
		t.Errorf("the GM was told %q", got)
	}
	gs.Queue().GetPending()
	c.say(gs, "#grantcm 10")
	if gp.Mastery.Points != 1450 {
		t.Fatalf("at the most: %d unspent", gp.Mastery.Points)
	}
	if _, _, ok := masteryPoints(gs); ok {
		t.Error("nothing given, yet a gain was told")
	}
}

// The MemoryDump's m_p44 is the level's stat cap before masteries, SOE's
// (350 at 45, 400 at 60), read where VINetCharacter reads it. The control:
// the server's old flat 475 at both.
func TestTheDumpCarriesSOEsStatCap(t *testing.T) {
	for _, c := range []struct{ level, want int }{{45, 350}, {60, 400}} {
		buf := make([]byte, 256)
		w := buffer.NewWriter(buf)
		writeDumpRecord(w, &entity.Player{Name: "Capped", Level: c.level, Breath: 255})
		r := buffer.NewReader(buf[:w.Position()])
		z := func() int64 { v, _ := r.Read7BitEncodedInt64(); return v }
		z()
		n, _ := r.ReadInt32()
		r.Advance(int(n))
		for i := 0; i < 5; i++ { // class, race, level, experience, m_p2C
			z()
		}
		r.ReadByte()
		for i := 0; i < 3; i++ { // purse, bank, training points
			z()
		}
		if got := z(); got != int64(c.want) || got == 475 {
			t.Errorf("level %d: m_p44 %d, want %d", c.level, got, c.want)
		}
		if r.Remaining() != 0 {
			t.Errorf("level %d: %d bytes past m_p44", c.level, r.Remaining())
		}
	}
}

// keptTP is a progress store that keeps levels and training points.
type keptTP struct{ unused, total int }

func (k *keptTP) SetProgress(uint32, int, int) error { return nil }
func (k *keptTP) SetTrainingPoints(_ uint32, unused, total int) error {
	k.unused, k.total = unused, total
	return nil
}

// #setlevel gives the training points the level is due and the character
// has not had, tells them (0x0036) and keeps them; set lower it keeps what
// it has. The control: a lower level sends no 0x0036.
func TestSetLevelGivesTheLevelsTrainingPoints(t *testing.T) {
	c, gs, gp, _, _ := gmWorld(t)
	kept := &keptTP{}
	c.h.SetStores(kept, nil)
	told := func() (int, bool) {
		for _, m := range gs.Queue().PeekPending() {
			d := m.Data()
			if protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeTrainingPoints {
				v, n := binary.Varint(d[3:])
				return int(v), n > 0
			}
		}
		return 0, false
	}
	c.say(gs, "#setlevel 30")
	want := entity.TrainingPointsDue(30) - entity.CreationTrainingPoints
	if v, ok := told(); !ok || v != want || gp.UnusedTP != want || kept.unused != want || kept.total != entity.TrainingPointsDue(30) {
		t.Fatalf("level 30: told %d (%v), %d unspent, kept %d/%d; want %d", v, ok, gp.UnusedTP, kept.unused, kept.total, want)
	}
	gs.Queue().GetPending()
	c.say(gs, "#setlevel 10")
	if _, ok := told(); ok || gp.UnusedTP != want {
		t.Fatalf("the control: set lower, told again or changed (%d)", gp.UnusedTP)
	}
	gs.Queue().GetPending()
	c.say(gs, "#setlevel 31")
	if v, _ := told(); v != want+9 {
		t.Errorf("on to 31: told %d, want %d (30's 9 once)", v, want+9)
	}
}

// #cm lists what a character bought and #cmreset takes it all back with its
// points, its abilities taken out and the caps back at the level's; both are
// a GM's. The control: a player's #cmreset resets nothing.
func TestCMCommandsAreAGMs(t *testing.T) {
	c, gs, gp, ps, pp := gmWorld(t)
	c.gw.SetCMCatalog(testCatalog())
	cm := NewClassMasteryHandler(c.sm)
	cm.SetGameWorld(c.gw)
	book := &fakeBook{spells: map[int]string{500: "Taunt"}}
	kept := &keptMastery{book: book}
	cm.SetStore(kept)
	cm.SetSpellbook(book)
	c.h.SetClassMastery(cm)
	buy := func(s *session.Session, id int) {
		req := make([]byte, 9)
		req[0] = cmBuy
		binary.LittleEndian.PutUint32(req[5:], uint32(id))
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeClassMastery)
		msg.SetData(req)
		if err := cm.HandleClassMastery(s, msg); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []*entity.Player{gp, pp} {
		p.Class, p.Level, p.BaseStr = 0, 60, 130
		p.Mastery = entity.Mastery{Points: 100}
		p.RecalcStats(true)
	}
	buy(gs, 1)
	buy(gs, 10)
	buy(ps, 1)
	if gp.Mastery.Points != 70 || gp.StatCapOf(entity.StatStr) != 415 || len(book.added) != 1 {
		t.Fatalf("bought: %+v, STR cap %d, abilities added %v", gp.Mastery, gp.StatCapOf(entity.StatStr), book.added)
	}

	c.say(ps, "#cmreset")
	if pp.Mastery.Points != 90 || len(pp.CMs) != 1 {
		t.Fatalf("the control: a player's #cmreset changed %+v, %v", pp.Mastery, pp.CMs)
	}

	gs.Queue().GetPending()
	c.say(gs, "#cm")
	got := strings.Join(texts(heard(t, gs)), "\n")
	if !strings.Contains(got, "Gm: 2 masteries") || !strings.Contains(got, "Brawn 1 (general, 10)") ||
		!strings.Contains(got, "Battle Cry 1 (class, 20)") || !strings.Contains(got, "STR cap +15") {
		t.Errorf("#cm said:\n%s", got)
	}

	gs.Queue().GetPending()
	c.say(gs, "#cmreset")
	if gp.Mastery.Points != 100 || gp.Mastery.Spent != 0 || len(gp.CMs) != 0 || gp.StatCapOf(entity.StatStr) != 400 {
		t.Errorf("after #cmreset: %+v, %v, STR cap %d", gp.Mastery, gp.CMs, gp.StatCapOf(entity.StatStr))
	}
	if len(kept.owned[gp.CharServerID]) != 0 || kept.rows[gp.CharServerID].Points != 100 {
		t.Errorf("not kept: %v, %+v", kept.owned[gp.CharServerID], kept.rows[gp.CharServerID])
	}
	// Battle Cry 1 took Taunt (500) and gave 501: the reset takes 501 and
	// gives Taunt back
	if fmt.Sprint(kept.removedSpells) != "[500 501]" || fmt.Sprint(book.added) != "[501 500]" {
		t.Errorf("abilities: taken out %v, added %v; want [500 501] and [501 500]", kept.removedSpells, book.added)
	}
	removed, points := 0, false
	for _, m := range gs.Queue().PeekPending() {
		d := m.Data()
		if protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeClassMasteryServer {
			switch d[2] {
			case cmRespRemoved:
				removed++
			case cmRespPoints:
				points = true
			}
		}
	}
	if removed != 2 || !points {
		t.Errorf("told the client %d removals and points %v; want 2 and the points", removed, points)
	}
}
