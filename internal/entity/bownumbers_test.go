package entity

import (
	"encoding/binary"
	"testing"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// shotNumbers is the 0x00DB amounts queued on sess after the first skip
// messages, as the client reads them (zigzag, negative for damage dealt).
func shotNumbers(sess *session.Session, skip int) (amounts []int, queued int) {
	pending := sess.Queue().PeekPending()
	for _, m := range pending[skip:] {
		d := m.Data()
		if len(d) < 2 || protocol.GameOpcode(binary.LittleEndian.Uint16(d)) != protocol.OpcodeDamageIndication {
			continue
		}
		r := &vims{b: d[2:]}
		r.u32()
		amounts = append(amounts, int(r.signed()))
	}
	return amounts, len(pending)
}

// A Range4 shot's two arrows are told as two numbers (Live tickets 5586/5587:
// two arrows seen, one number, read as a miss), each the arrow's share of
// the round's damage by its own roll, summing to it; the creature takes the
// same one hit as before, and its threat is that hit's. A one-arrow shot and
// a melee swing stay one number. Negative control: the numbers are checked
// against arrowShare on the arrows' own rolls, so a split by halves or a
// second number of the whole would fail.
func TestABowShotsArrowsAreTwoNumbers(t *testing.T) {
	sr := newShotRig(t)
	fixDice(t)
	dice := func() func(int) int {
		k := 0 // the top, then the middle, in turn: two arrows of different rolls
		return func(n int) int {
			k++
			if k%2 == 1 {
				return n - 1
			}
			return n / 2
		}
	}
	g, p, w := sr.guard, sr.p, sr.r.w
	p.Level, p.Str, p.Dex, p.Wis = 60, 200, 400, 250 // figures big enough that the arrows' rolls differ
	p.Gear.RangedDamage, p.Gear.WeaponDamage = 700, 300
	shot := func(kind, hp int) (nums []int, dealt, threat int, killed bool) {
		oldIntn := meleeIntn
		meleeIntn = dice()
		defer func() { meleeIntn = oldIntn }()
		p.Swing.Kind = kind
		p.Power, p.MaxPower = 10_000, 10_000
		g.HP, g.MaxHP, g.Dead, g.Threats = hp, 1_000_000, false, nil
		_, skip := shotNumbers(sr.sess, 0)
		w.mu.Lock()
		killed = w.playerHitLocked(sr.sess, p, g, sr.r.objectID(g.ServerID))
		w.mu.Unlock()
		nums, _ = shotNumbers(sr.sess, skip)
		return nums, hp - g.HP, int(g.Threats[p.ServerID]), killed
	}
	sum := func(ns []int) (s int) {
		for _, n := range ns {
			s -= n
		}
		return s
	}

	nums, dealt, threat, _ := shot(swingRange4, 1_000_000)
	_, parts, arrows := playerShotParts(p, g, dice()) // the same rolls, in the same order
	raw := parts[0] + parts[1]
	if arrows != 2 || parts[0] == parts[1] {
		t.Fatalf("the rig's arrows: %d, parts %v (want two different rolls)", arrows, parts)
	}
	if len(nums) != 2 || sum(nums) != dealt || dealt <= 0 {
		t.Fatalf("Range4: numbers %v (sum %d), dealt %d; want two summing to it", nums, sum(nums), dealt)
	}
	for i := range 2 {
		if want := arrowShare(dealt, raw, parts, 2, i); -nums[i] != want {
			t.Errorf("arrow %d's number %d, want its share %d of %d by rolls %v", i+1, -nums[i], want, dealt, parts)
		}
	}
	if threat != dealt {
		t.Errorf("threat %d, want the round's one hit %d", threat, dealt)
	}

	// one arrow, and a melee swing: one number, the whole hit, as before
	for _, kind := range []int{swingRange1, 5} {
		nums, dealt, _, _ := shot(kind, 1_000_000)
		if len(nums) != 1 || -nums[0] != dealt || dealt <= 0 {
			t.Errorf("swing kind %d: numbers %v, dealt %d; want one of it", kind, nums, dealt)
		}
	}

	// a kill on the first arrow's share alone: still both numbers, their sum
	// the hit as told before (the creature's HP floors at 0)
	full, _, _, _ := shot(swingRange4, 1_000_000)
	nums, _, _, killed := shot(swingRange4, 1)
	if !killed || !g.Dead || len(nums) != 2 || sum(nums) != sum(full) {
		t.Errorf("a kill by the first arrow: killed %v, dead %v, numbers %v (sum %d), want two summing to %d", killed, g.Dead, nums, sum(nums), sum(full))
	}
}
