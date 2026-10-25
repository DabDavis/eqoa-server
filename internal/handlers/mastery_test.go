package handlers

import (
	"encoding/binary"
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
