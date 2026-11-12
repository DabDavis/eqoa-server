package entity

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// testerProLine is the Tester Pro's itemeffect line as
// tools/tester_pro_20261112.py writes it (petition 1725).
const testerProLine = "Self, Run Speed 2x Spirit of the Wolf, Set Maximum HP to 1, Set All Stats to 1, Unlimited Uses, 30min"

func testerPro() PotionEffect {
	return NewPotionEffect(965006, "Tester Pro", testerProLine, "Tester Pro", 638503769, "", 0, 0)
}

// The line reads whole: a self buff of 30 minutes, a speed of twice the
// movement buff's, a ceiling of 1 on the hit points, every stat at 1, used
// without end, nothing unread, not harmful.
func TestTesterProLineReads(t *testing.T) {
	pe := testerPro()
	d := pe.Def
	if pe.Thrown() || d.Duration != 30*time.Minute || d.Harmful || !d.Unlimited || len(d.Unread) != 0 || len(d.Effects) != 3 {
		t.Fatalf("read as thrown %v, %v, harmful %v, unlimited %v, unread %q, effects %+v", pe.Thrown(), d.Duration, d.Harmful,
			d.Unlimited, d.Unread, d.Effects)
	}
	e := d.Effects
	if e[0].Kind != EffectSpeed || e[0].SowPct != 200 || e[1].Kind != EffectHPCap || e[1].Amount != 1 ||
		e[2].Kind != EffectStatPin || e[2].Amount != 1 {
		t.Fatalf("effects %+v", e)
	}
	for _, c := range []struct {
		line string
		want EffectKind
		n    int
	}{
		{"Self, Caps HP at 50, 1min", EffectHPCap, 50},
		{"Self, Set Max HP to 10, 1min", EffectHPCap, 10},
		{"Self, Movement Speed 1.5x SoW, 1min", EffectSpeed, 150},
		{"Self, Set all of their stats to 5, 1min", EffectStatPin, 5},
	} {
		d := ParseSpellText(c.line)
		if len(d.Effects) != 1 || d.Effects[0].Kind != c.want || (c.want == EffectSpeed && d.Effects[0].SowPct != c.n) ||
			(c.want != EffectSpeed && d.Effects[0].Amount != c.n) {
			t.Errorf("%q: %+v, unread %q", c.line, d.Effects, d.Unread)
		}
	}
	// the controls: a raise of the maximum is a raise, "Set STR to 500" a stat
	// set, the movement buff's own speed of no multiple, and a potion without
	// the words is spent
	if d := ParseSpellText("Self, Increase Max HP by 50, 1min"); len(d.Effects) != 1 || d.Effects[0].Kind != EffectStat || d.Effects[0].Stat != StatMaxHP {
		t.Errorf("a raise of the maximum: %+v", d.Effects)
	}
	if d := ParseSpellText("Self, Set STR to 500, 1min"); len(d.Effects) != 1 || d.Effects[0].Kind != EffectStat || !d.Effects[0].Set {
		t.Errorf("a stat set: %+v", d.Effects)
	}
	if d := ParseSpellText("Self, Sow, 30min"); len(d.Effects) != 1 || d.Effects[0].Kind != EffectSpeed || d.Effects[0].SowPct != 0 || d.Unlimited {
		t.Errorf("the movement buff: %+v", d.Effects)
	}
	if BuffLine(&CastSpell{Def: testerPro().Def}) == BuffLine(&CastSpell{Def: ParseSpellText("Self, Sow, 30min")}) {
		t.Error("the Tester Pro shares the movement buff's line")
	}
}

// speedsSent are the run speeds told the client (0x00F8), in order.
func speedsSent(out [][]byte) []float32 {
	var s []float32
	for _, m := range out {
		if len(m) >= 6 && binary.LittleEndian.Uint16(m) == uint16(protocol.OpcodePlayerSpeed) {
			s = append(s, math.Float32frombits(binary.LittleEndian.Uint32(m[2:])))
		}
	}
	return s
}

// Used: the run speed twice Spirit of the Wolf's (told by 0x00F8), the most
// hit points 1 and the hit points with them, every stat 1 whatever is worn
// or cast on it, the stat channel saying so; used again it renews its 30
// minutes, one effect; kept over a logout; at its end the speed, the most hit
// points and every stat are as they were, the hit points where it left them.
// The control: Spirit of the Wolf alone is 1.3 times the run.
func TestTesterProTradesEverythingForSpeed(t *testing.T) {
	c := newCaster(t)
	p := c.p
	sow := p.Speed * speedBuff
	ownMax, ownPower := p.MaxHPNow(), p.MaxPowerNow()
	own := [7]int{}
	for i := range own {
		own[i] = p.StatNow(StatStr + Stat(i))
	}

	if r := c.use(t, testerPro(), 0xFFFFFFFF, 0); r != "" {
		t.Fatalf("refused: %q", r)
	}
	if got := speedsSent(c.sent()); len(got) != 1 || got[0] != sow*2 {
		t.Fatalf("speeds told %v, want [%v] (twice the movement buff's %v)", got, sow*2, sow)
	}
	if p.SpeedNow() != sow*2 || p.MaxHPNow() != 1 || p.HP != 1 {
		t.Fatalf("under it: speed %v, HP %d/%d", p.SpeedNow(), p.HP, p.MaxHPNow())
	}
	for i := range own {
		if n := p.StatNow(StatStr + Stat(i)); n != 1 {
			t.Fatalf("stat %d is %d, want 1", i, n)
		}
	}
	buf := SerializeStatUpdate(p)
	for off := 0; off < 28; off += 4 {
		if n := int32(binary.LittleEndian.Uint32(buf[off:])); n != 1 {
			t.Errorf("stat channel offset %d: %d, want 1", off, n)
		}
	}
	if hp, top := binary.LittleEndian.Uint32(buf[28:]), binary.LittleEndian.Uint32(buf[32:]); hp != 1 || top != 1 {
		t.Errorf("stat channel HP %d/%d, want 1/1", hp, top)
	}

	// a stat raised under it (gear, a buff) stays 1; a heal stays at 1
	p.Str += 40
	c.w.mu.Lock()
	c.w.healLocked(p, c.sess, p, 300, 0)
	c.w.mu.Unlock()
	if p.StatNow(StatStr) != 1 || p.HP != 1 {
		t.Fatalf("raised under it: STR %d, HP %d", p.StatNow(StatStr), p.HP)
	}
	p.Str -= 40
	// a movement buff on top takes nothing off it, and its speed stays the faster
	if r := c.use(t, NewPotionEffect(1015, "Alacrity Potion", "Self, Sow, 30min", "", 638503769, "", 0, 0), 0xFFFFFFFF, time.Second); r != "" {
		t.Fatalf("Alacrity refused: %q", r)
	}
	if p.SpeedNow() != sow*2 || len(p.Effects) != 2 {
		t.Fatalf("with Alacrity: speed %v, %d effects", p.SpeedNow(), len(p.Effects))
	}
	c.w.mu.Lock()
	c.w.removeEffectLocked(p, PotionEffectID(1015))
	c.w.mu.Unlock()

	// used again: one effect, its 30 minutes renewed
	c.use(t, testerPro(), 0xFFFFFFFF, 10*time.Minute)
	if len(p.Effects) != 1 || p.Effects[0].Until != c.at(40*time.Minute) {
		t.Fatalf("used again: %d effects, until %v; want one, until %v", len(p.Effects), p.Effects[0].Until, c.at(40*time.Minute))
	}

	// kept over a logout, all of it
	rows := savedEffects(p, c.at(10*time.Minute))
	p.Effects, p.Buffs, p.HP = nil, nil, 1
	c.w.mu.Lock()
	c.w.effectsChangedLocked(p, p.SpeedNow())
	c.w.mu.Unlock()
	if n := c.w.RestoreEffects(c.sess, rows, c.at(10*time.Minute)); n != 1 || p.SpeedNow() != sow*2 || p.MaxHPNow() != 1 || p.StatNow(StatWis) != 1 {
		t.Fatalf("restored %d: speed %v, max HP %d, WIS %d", n, p.SpeedNow(), p.MaxHPNow(), p.StatNow(StatWis))
	}
	p.Effects, p.Buffs = nil, nil
	c.use(t, testerPro(), 0xFFFFFFFF, 10*time.Minute)
	c.sent()

	// its end: everything as it was, the hit points where it left them
	c.potionTick(40*time.Minute + time.Second)
	if len(p.Effects) != 0 {
		t.Fatalf("%d effects past its time", len(p.Effects))
	}
	if got := speedsSent(c.sent()); len(got) != 1 || got[0] != p.Speed {
		t.Fatalf("speeds told at its end %v, want [%v]", got, p.Speed)
	}
	if p.MaxHPNow() != ownMax || p.MaxPowerNow() != ownPower || p.HP != 1 {
		t.Fatalf("after: HP %d/%d (want 1/%d), max power %d (want %d)", p.HP, p.MaxHPNow(), ownMax, p.MaxPowerNow(), ownPower)
	}
	for i := range own {
		if n := p.StatNow(StatStr + Stat(i)); n != own[i] {
			t.Fatalf("after: stat %d is %d, want %d", i, n, own[i])
		}
	}

	// the control: Spirit of the Wolf alone
	c.use(t, NewPotionEffect(1015, "Alacrity Potion", "Self, Sow, 30min", "", 638503769, "", 0, 0), 0xFFFFFFFF, time.Hour)
	if p.SpeedNow() != sow {
		t.Fatalf("Alacrity alone: %v, want %v", p.SpeedNow(), sow)
	}
}

// The movement check judges a report by the speed the server gave: twice
// Spirit of the Wolf's under the Tester Pro.
func TestTesterProSpeedIsTheMoveChecksSpeed(t *testing.T) {
	c := newCaster(t)
	c.use(t, testerPro(), 0xFFFFFFFF, 0)
	var st moveState
	if s := st.speed(c.p, c.at(0)); s != c.p.Speed*speedBuff*2 {
		t.Fatalf("the check's speed %v, want %v", s, c.p.Speed*speedBuff*2)
	}
}
