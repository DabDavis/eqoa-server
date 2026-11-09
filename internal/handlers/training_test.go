package handlers

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// The trainer's own rules (VIWndTrainer::Adjust, VIClient::TrainStats): more
// points than are left, nothing, or a stat lowered are not paid at all; a
// stat's cap is no refusal (FitTraining holds it there, below).
func TestTrainRefusal(t *testing.T) {
	cases := []struct {
		adds   [7]int64
		unused int
		paid   bool
	}{
		{[7]int64{0, 3, 0, 0, 0, 0, 0}, 347, true},
		{[7]int64{0, 206, 0, 0, 0, 0, 0}, 347, true}, // past a cap: FitTraining's
		{[7]int64{10, 0, 0, 0, 0, 0, 0}, 9, false},   // more than are left
		{[7]int64{}, 9, false},                       // nothing
		{[7]int64{5, -2, 0, 0, 0, 0, 0}, 9, false},   // a stat lowered
		{[7]int64{1, 1, 1, 1, 1, 1, 1}, 7, true},
	}
	for i, c := range cases {
		why := trainRefusal(c.adds, c.unused)
		if (why == "") != c.paid {
			t.Errorf("case %d (%v, %d left): refusal %q, paid wanted %v", i, c.adds, c.unused, why, c.paid)
		}
	}
}

// Where a stat's base stops (entity.FitTraining): its cap, the level's (400
// from level 46, 350 below) and what the masteries bought raise it by. Points
// into a stat whose shown value items hold at the full cap are taken (into its
// base, as a tester's level-60 Stamina took 115, F3 20261025-214427); points
// past the base's cap are held back and stay unspent. The control: the same
// requests at the level's cap alone are held where SOE's trainer held them.
func TestTrainingStopsAtEachStatsCap(t *testing.T) {
	base := [7]int{60, 195, 70, 70, 55, 85, 50}
	at := func(level int, raises [7]int) [7]int {
		p := &entity.Player{Level: level}
		for i, r := range raises {
			p.CMEffects[7+i] = r
		}
		return p.StatCaps()
	}
	none := [7]int{}
	brawn := [7]int{125} // Brawn 1-5: STR's cap 525
	for i, c := range []struct {
		caps      [7]int
		adds, fit [7]int64
	}{
		{at(60, none), [7]int64{0, 3}, [7]int64{0, 3}},     // a capped Stamina's base still takes them
		{at(60, none), [7]int64{0, 205}, [7]int64{0, 205}}, // to the cap exactly (195 + 205 = 400)
		{at(60, none), [7]int64{0, 206}, [7]int64{0, 205}},
		{at(45, none), [7]int64{0, 156}, [7]int64{0, 155}}, // 350 below level 46
		{at(60, none), [7]int64{341}, [7]int64{340}},       // STR at 400 and no further
		{at(60, brawn), [7]int64{341}, [7]int64{341}},      // Brawn's raise: past 400
		{at(60, brawn), [7]int64{465, 206}, [7]int64{465, 205}},
		{at(60, brawn), [7]int64{466}, [7]int64{465}}, // and no further than 525
	} {
		fit, held := entity.FitTraining(base, c.caps, c.adds)
		if fit != c.fit {
			t.Errorf("case %d (caps %v, %v): fit %v, want %v", i, c.caps, c.adds, fit, c.fit)
		}
		for s := range held {
			if held[s] != c.adds[s]-c.fit[s] {
				t.Errorf("case %d: stat %d held back %d, want %d", i, s, held[s], c.adds[s]-c.fit[s])
			}
		}
	}
	// a base already past its cap (a raise removed) takes none and loses none
	if fit, held := entity.FitTraining([7]int{450}, at(60, none), [7]int64{5}); fit[0] != 0 || held[0] != 5 {
		t.Errorf("a base past its cap: fit %v, held %v", fit, held)
	}
}

// A training request through the handler, a level 60 warrior trained to the
// level's cap in STR: a point more is held back, said in red, and nothing
// spent (0x001D 0); with Brawn 1-5's raise held (cmcatalog's STRMax 15 to 35,
// 125 in all) the same request is paid and the base passes 400, up to 525,
// the rest of a request held back and kept. The control: STA, which no rank
// raises, still stops at 400.
func TestTrainingPastTheOldCapWithACapMastery(t *testing.T) {
	c := newChatWorld(t)
	sess, p := c.join(7, "Trainer", 25000, 15000, 0)
	p.Class, p.Level = 0, 60
	p.BaseStr, p.BaseSta, p.BaseAgi, p.BaseDex, p.BaseWis, p.BaseInt, p.BaseCha = 400, 395, 90, 90, 80, 80, 70
	p.UnusedTP = 200
	p.RecalcStats(true)
	h := NewTrainingHandler(c.sm, nil)
	h.SetGameWorld(c.gw)
	train := func(adds [7]int64) (spent int32, red []string) {
		t.Helper()
		sess.Queue().GetPending()
		var b []byte
		for _, a := range adds {
			b = appendZigzag(b, a)
		}
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeUpdateTrainingPoints)
		msg.SetData(b)
		if err := h.HandleUpdateTrainingPoints(sess, msg); err != nil {
			t.Fatal(err)
		}
		spent = -1
		for _, m := range sess.Queue().PeekPending() {
			d := m.Data()
			switch protocol.GameOpcode(binary.LittleEndian.Uint16(d)) {
			case protocol.OpcodeUpdateTrainingPoints:
				spent = int32(zigzagAt(d[2:]))
			case protocol.OpcodeErrorMessage:
				r := &cmReader{b: d[2:]}
				red = append(red, r.wide(1024))
			}
		}
		return spent, red
	}

	if spent, red := train([7]int64{1}); spent != 0 || p.BaseStr != 400 || p.UnusedTP != 200 || len(red) != 1 || !strings.Contains(red[0], "STR 400") {
		t.Fatalf("STR 400 + 1 with no raise: spent %d, STR %d, %d left, red %v; want 0, 400, 200 and a line", spent, p.BaseStr, p.UnusedTP, red)
	}

	p.CMEffects[7] = 15 + 20 + 25 + 30 + 35
	p.RecalcStats(false)
	if spent, red := train([7]int64{100}); spent != 100 || p.BaseStr != 500 || p.UnusedTP != 100 || len(red) != 0 {
		t.Fatalf("STR 400 + 100 with Brawn 1-5: spent %d, STR %d, %d left, red %v; want 100, 500, 100", spent, p.BaseStr, p.UnusedTP, red)
	}
	if got := p.StatNow(entity.StatStr); got != 500 {
		t.Errorf("STR now %d, want 500 (past the old 400)", got)
	}
	spent, red := train([7]int64{30, 10})
	if spent != 30 || p.BaseStr != 525 || p.BaseSta != 400 || p.UnusedTP != 70 {
		t.Errorf("STR +30 and STA +10: spent %d, STR %d, STA %d, %d left; want 30 (25 and 5), 525, 400, 70", spent, p.BaseStr, p.BaseSta, p.UnusedTP)
	}
	if len(red) != 1 || !strings.Contains(red[0], "STR 525") || !strings.Contains(red[0], "STA 400") || !strings.Contains(red[0], "10 training points") {
		t.Errorf("the held-back line: %v", red)
	}
}

// appendZigzag is Write7BitEncodedInt64's zigzag varint.
func appendZigzag(b []byte, v int64) []byte {
	u := uint64(v<<1) ^ uint64(v>>63)
	for u >= 0x80 {
		b = append(b, byte(u)|0x80)
		u >>= 7
	}
	return append(b, byte(u))
}

// zigzagAt reads one zigzag varint.
func zigzagAt(b []byte) int64 {
	var u uint64
	for i, sh := 0, 0; i < len(b); i, sh = i+1, sh+7 {
		u |= uint64(b[i]&0x7F) << sh
		if b[i]&0x80 == 0 {
			break
		}
	}
	return int64(u>>1) ^ -int64(u&1)
}
