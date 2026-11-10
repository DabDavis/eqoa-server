package handlers

import (
	"encoding/binary"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// statCapTold is the m_p44 the last 0x001C queued carries, -1 for none.
func (c *cmTest) statCapTold() int {
	v := -1
	for _, m := range c.sess.Queue().PeekPending() {
		d := m.Data()
		if protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodePlayerP44 && len(d) >= 6 {
			v = int(binary.LittleEndian.Uint32(d[2:]))
		}
	}
	return v
}

// trainerCapsTold is what the client makes of what it was told: CLIENT's
// GetP44Total(stat), m_p44 + m_bonus[stat], for each stat (the trainer's
// "Current" is held to it), and GetP44Total(-1), m_p44 alone (where the trainer
// stops a base).
func trainerCapsTold(p44 int, bonuses [7]int) (caps [7]int, stop int) {
	for i, b := range bonuses {
		caps[i] = p44 + b
	}
	return caps, p44
}

// dumpCapsOf reads the MemoryDump's m_p44 (the player record's last field)
// and the mastery block's seven m_bonus as CLIENT does (VINetCharacter's
// read, VIClientPart4EC58::ReadState).
func dumpCapsOf(t *testing.T, p *entity.Player) (int, [7]int) {
	t.Helper()
	rec := make([]byte, 512)
	w := buffer.NewWriter(rec)
	writeDumpRecord(w, p)
	r := &cmReader{b: rec[:w.Position()]}
	r.zz()       // id
	n := r.u32() // the name's length (raw)
	if r.need(int(n)) {
		r.o += int(n) // the name
	}
	for range 5 { // class, race, level, the bar's experience, m_p2C
		r.zz()
	}
	r.u8()        // breath
	for range 3 { // purse, bank, m_p40
		r.zz()
	}
	p44 := r.zz()
	if r.err != nil || r.o != len(r.b) {
		t.Fatalf("the dump's record does not read to its end: %v, %d of %d", r.err, r.o, len(r.b))
	}
	block := make([]byte, 512)
	bw := buffer.NewWriter(block)
	writeMasteryState(bw, p)
	br := &cmReader{b: block[:bw.Position()]}
	br.u32()
	for range 4 {
		br.zz()
	}
	br.u32() // the speed
	for range 29 {
		br.zz()
	}
	var bonuses [7]int
	for i := range bonuses {
		bonuses[i] = br.zz()
	}
	if br.err != nil || br.o != len(br.b) {
		t.Fatalf("the dump's mastery block does not read to its end: %v, %d of %d", br.err, br.o, len(br.b))
	}
	return p44, bonuses
}

// Petition 1375 (Beef: "CM line for warriors (maybe all classes) stat maxes
// dont work, not extending maximums"): a level 60 warrior, STR trained to 400
// and worn items adding 50 more, buys Brawn 1 (STRMax 15) and Godborn (every
// stat's 25). Each raises STR's cap and the client is told so: 0x001C's m_p44
// is the highest cap and each m_bonus its stat's cap less it, so CLIENT's
// GetP44Total(stat) is each stat's own cap and its trainer stops a base at the
// highest (the dump says the same at a login). STR then goes past the old 400
// by the items, by a spell's STR, and by training points (to 440, and no
// further); STA, raised by Godborn alone, stops at 425. The controls: before
// any rank the cap is 400 everywhere, the items held there, a point of
// training refused.
func TestACapMasteryLetsAStatPassTheOldCap(t *testing.T) {
	c := newCMTest(t, 0, 0, 60)
	p := c.p
	p.BaseStr, p.BaseSta = 400, 390
	p.UnusedTP = 100
	p.Gear = entity.Gear{Str: 50}
	p.RecalcStats(true)
	th := NewTrainingHandler(c.w.sm, nil)
	th.SetGameWorld(c.w.gw)
	train := func(adds [7]int64) int {
		t.Helper()
		c.sess.Queue().GetPending()
		var b []byte
		for _, a := range adds {
			b = appendZigzag(b, a)
		}
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeUpdateTrainingPoints)
		msg.SetData(b)
		if err := th.HandleUpdateTrainingPoints(c.sess, msg); err != nil {
			t.Fatal(err)
		}
		for _, m := range c.sess.Queue().PeekPending() {
			if d := m.Data(); protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeUpdateTrainingPoints {
				return int(zigzagAt(d[2:]))
			}
		}
		return -1
	}

	// the controls: nothing bought
	if got := p.StatNow(entity.StatStr); got != 400 {
		t.Fatalf("STR before any rank: %d, want 400 (the items' 50 held by the cap)", got)
	}
	if p44, bonuses := dumpCapsOf(t, p); p44 != 400 || bonuses != [7]int{} {
		t.Fatalf("the dump before any rank: m_p44 %d, bonuses %v; want 400 and none, as SOE's server sent", p44, bonuses)
	}
	if spent := train([7]int64{1}); spent != 0 || p.BaseStr != 400 {
		t.Fatalf("a point into STR 400 before any rank: spent %d, STR %d", spent, p.BaseStr)
	}

	buy := func(id int) [7]int {
		t.Helper()
		replies, red := c.send(cmBuy, id)
		if len(red) != 0 || len(replies) != 1 || replies[0][0] != cmRespBought {
			t.Fatalf("buying %d: % x, red %v", id, replies, red)
		}
		r := &cmReader{b: replies[0], o: 5}
		r.u32()
		r.u32()
		r.u32()
		st := r.state()
		if r.err != nil || r.o != len(r.b) {
			t.Fatalf("Recv05 does not read as CLIENT reads it: %v", r.err)
		}
		return st.bonuses
	}
	p.Mastery.Points = 300
	bonuses := buy(1) // Brawn 1: STR 1, STRMax 15
	caps, stop := trainerCapsTold(c.statCapTold(), bonuses)
	if caps != [7]int{415, 400, 400, 400, 400, 400, 400} || stop != 415 {
		t.Errorf("after Brawn 1 the client's caps %v, its trainer stopping at %d; want STR 415, the rest 400", caps, stop)
	}
	if got := p.StatNow(entity.StatStr); got != 415 {
		t.Errorf("STR with the items after Brawn 1: %d, want 415 (past the old 400)", got)
	}

	p.Mastery.Spent = 500 // Godborn's 500 spent
	bonuses = buy(3)      // Godborn: every cap 25
	caps, stop = trainerCapsTold(c.statCapTold(), bonuses)
	want := [7]int{440, 425, 425, 425, 425, 425, 425}
	if caps != want || stop != 440 || caps != p.StatCaps() {
		t.Errorf("after Godborn the client's caps %v (server %v), stopping at %d; want %v and 440", caps, p.StatCaps(), stop, want)
	}
	if p44, b := dumpCapsOf(t, p); p44 != 440 || b != bonuses {
		t.Errorf("the dump after both: m_p44 %d, bonuses %v; want 440, %v (as Recv05 told it)", p44, b, bonuses)
	}

	// a spell's STR past the old cap, the items off
	p.Gear = entity.Gear{}
	p.RecalcStats(false)
	p.Mods.Str = 20
	if got := p.StatNow(entity.StatStr); got != 421 {
		t.Errorf("STR 400 + Brawn's 1 + a spell's 20: %d, want 421", got)
	}
	p.Mods.Str = 0

	// training points past the old cap: STR to 440 and no further, STA to 425
	if spent := train([7]int64{50, 50}); spent != 75 || p.BaseStr != 440 || p.BaseSta != 425 || p.UnusedTP != 25 {
		t.Errorf("STR +50 and STA +50: spent %d, STR %d, STA %d, %d left; want 75 (40 and 35), 440, 425, 25", spent, p.BaseStr, p.BaseSta, p.UnusedTP)
	}
	if got := p.StatNow(entity.StatStr); got != 440 {
		t.Errorf("STR now %d, want its cap 440 (base 440, Brawn's 1 held)", got)
	}
}
