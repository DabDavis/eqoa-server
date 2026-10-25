package entity

import (
	"encoding/binary"
	"testing"
	"time"
)

// Worn items' HoT (PoT) as the Formulas page counts them: the highest in
// full, the second at 40% (its example: a second 25 PoT item gives 10),
// every further one 1. The control: the HoT page's 50% gives its own
// example's 32 for four 20 HoT items, 30 here.
func TestItemCreditIsTheFormulasPages(t *testing.T) {
	for _, c := range []struct {
		items []int
		want  int
	}{
		{nil, 0}, {[]int{7}, 7}, {[]int{25, 25}, 35}, {[]int{20, 20, 20, 20}, 30},
		{[]int{5, 30, 12}, 30 + 4 + 1}, {[]int{3, 3}, 3 + 1},
	} {
		if got := ItemCredit(c.items); got != c.want {
			t.Errorf("%v: %d, want %d", c.items, got, c.want)
		}
	}
	if half := 20 + 20*50/100 + 1 + 1; half != 32 || half == ItemCredit([]int{20, 20, 20, 20}) {
		t.Fatal("the control: the HoT page's 50% must give 32")
	}
}

// A tick every six seconds gives the HoT in hit points and the PoT in
// power, in a fight as out of one: Sandstorm's bard of 4201 hit points
// rose 89 a tick (84 + its items' 5), and its power 127 at 4149 (82 + 45),
// while being hit. Nothing between ticks. The controls: the old rule (1%
// every 2 s, none within 10 s of a fight) gave nothing here.
func TestRegenTicksEverySixSecondsInAFight(t *testing.T) {
	w := NewWorld()
	p := &Player{ServerID: 1, Name: "Bard", Level: 51, HP: 3000, MaxHP: 4201, Power: 2000, MaxPower: 4149}
	p.Gear.HoTItems = []int{5}
	p.Gear.PoTItems = []int{45}
	now := time.Unix(1_800_000_000, 0)
	p.LastCombatTime = now
	w.Players[p.ServerID] = p
	w.mu.Lock()
	defer w.mu.Unlock()
	w.TickCount = 30
	w.regenSystem(now)
	if p.HP != 3000 || p.Power != 2000 {
		t.Fatalf("between ticks: %d HP, %d power", p.HP, p.Power)
	}
	w.TickCount = 60
	w.regenSystem(now)
	if p.HP != 3089 || p.Power != 2127 {
		t.Errorf("a tick in a fight: %d HP, %d power; want 3089, 2127", p.HP, p.Power)
	}
	p.HP = 4190
	w.TickCount = 120
	w.regenSystem(now)
	if p.HP != 4201 {
		t.Errorf("past the maximum: %d", p.HP)
	}
	b := SerializeStatUpdate(p)
	at := func(o int) int { return int(int32(binary.LittleEndian.Uint32(b[o:]))) }
	if at(48) != 127 || at(52) != 89 || at(164) != 82 || at(168) != 84 {
		t.Errorf("stat channel PoT %d HoT %d, base %d %d; want 127 89, 82 84", at(48), at(52), at(164), at(168))
	}
	// a new character's innate rate: at least 1
	novice := &Player{Level: 1, HP: 10, MaxHP: 16, Power: 5, MaxPower: 41}
	novice.regenTick()
	if novice.HP != 11 || novice.Power != 6 {
		t.Errorf("level 1: %d HP, %d power; want 11, 6", novice.HP, novice.Power)
	}
}
