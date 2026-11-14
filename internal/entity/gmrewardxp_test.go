package entity

import (
	"net"
	"slices"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// rewardXPOn is the GM's reward on p, if it is.
func rewardXPOf(p *Player) (n int, e *ActiveEffect) {
	for _, ae := range p.Effects {
		if ae.SpellID == RewardXPEffectID {
			if n++; e == nil {
				e = ae
			}
		}
	}
	return n, e
}

// Petition 5033: a GM's reward makes an award 10% more, a kill's and a quest's, its line saying so; on top of a boon
// the two multiply (2 x 1.1). It shows in the effects window with its icon. The control: a player without it earns
// the plain award.
func TestARewardOfExperienceIsTenPercentMore(t *testing.T) {
	k := newKillWorld(t, 30, 30)
	with, without := k.players[0], k.players[1]
	t0 := time.Now()
	name, renewed, _, ok := k.w.GiveRewardXP(k.sess[0], "Jibrish", t0)
	if !ok || renewed || name != with.Name {
		t.Fatalf("given: %q %v %v", name, renewed, ok)
	}
	if n, e := rewardXPOf(with); n != 1 || !e.Until.Equal(t0.Add(30*time.Minute)) || e.Name != RewardXPName ||
		!slices.Contains(with.Buffs, BuffSlot{Icon: RewardXPIcon, Name: RewardXPName}) {
		t.Fatalf("on: %d %+v, slots %+v", n, e, with.Buffs)
	}
	sentOf(k.sess[0])
	k.w.AwardXP(k.sess[0], with, 1000)
	k.w.AwardXP(k.sess[1], without, 1000)
	if earned(with, 0) != 1100 || earned(without, 0) != 1000 {
		t.Fatalf("a quest's 1000: %d with the reward, %d without", earned(with, 0), earned(without, 0))
	}
	if lines, _ := sentOf(k.sess[0]); !slices.Contains(lines, "You receive 1100 XP (reward +10%).") {
		t.Errorf("the line: %q", lines)
	}

	npc := &NPC{ServerID: 5201, Name: "Froglok", Level: 30, X: 1000, Z: 1000}
	b1, b2 := earned(with, 0), earned(without, 0)
	k.w.awardKillXPLocked(k.sess[0], with, npc)
	share := groupExp(30, 30, []int{30, 30})
	if got, ctl := earned(with, 0)-b1, earned(without, 0)-b2; ctl != share || got != share*110/100 {
		t.Errorf("a kill: %d with, %d without; want %d and %d", got, ctl, share*110/100, share)
	}

	k.w.GiveBoon(k.sess[0], "Freeport", t0)
	sentOf(k.sess[0])
	b1 = earned(with, 0)
	k.w.AwardXP(k.sess[0], with, 1000)
	if got := earned(with, 0) - b1; got != 2200 {
		t.Errorf("with a boon as well: %d, want 2200", got)
	}
	if lines, _ := sentOf(k.sess[0]); !slices.Contains(lines, "You receive 2200 XP (boon x2, reward +10%).") {
		t.Errorf("the line with both: %q", lines)
	}
}

// A second reward puts it back to its whole 30 minutes, never two of it nor 20% (it does not stack).
func TestARewardOfExperienceRenewsAndDoesNotStack(t *testing.T) {
	k := newKillWorld(t, 20)
	p := k.players[0]
	t0 := time.Now()
	k.w.GiveRewardXP(k.sess[0], "Jibrish", t0)
	_, renewed, left, ok := k.w.GiveRewardXP(k.sess[0], "Jibrish", t0.Add(20*time.Minute))
	if !ok || !renewed || left != 10*time.Minute {
		t.Fatalf("again: renewed %v, left %v, %v", renewed, left, ok)
	}
	if n, e := rewardXPOf(p); n != 1 || !e.Until.Equal(t0.Add(50*time.Minute)) || len(p.Buffs) != 1 {
		t.Fatalf("after the second: %d, %+v, slots %+v", n, e, p.Buffs)
	}
	k.w.AwardXP(k.sess[0], p, 1000)
	if earned(p, 0) != 1100 {
		t.Errorf("after two rewards: %d, want 1100", earned(p, 0))
	}
}

// It ends at its 30 minutes, its bearer told, and the next award is the plain one; until then it is on. A death
// leaves it on, as a boon's.
func TestARewardOfExperienceEndsAtThirtyMinutes(t *testing.T) {
	k := newKillWorld(t, 20)
	p := k.players[0]
	t0 := time.Now()
	k.w.GiveRewardXP(k.sess[0], "Jibrish", t0)
	k.w.mu.Lock()
	k.w.clearSpellsLocked(p) // a death
	k.w.effectSystem(t0.Add(30*time.Minute - time.Second))
	k.w.mu.Unlock()
	if n, _ := rewardXPOf(p); n != 1 {
		t.Fatal("gone before its time, or at a death")
	}
	k.sess[0].Queue().GetPending()
	k.w.mu.Lock()
	k.w.effectSystem(t0.Add(30 * time.Minute))
	k.w.mu.Unlock()
	if n, _ := rewardXPOf(p); n != 0 || len(p.Buffs) != 0 {
		t.Fatalf("at its time: %d, slots %+v", n, p.Buffs)
	}
	if got := redLines(k.sess[0]); !slices.Contains(got, RewardXPEndLine) {
		t.Errorf("the end's line: %q", got)
	}
	k.w.AwardXP(k.sess[0], p, 1000)
	if earned(p, 0) != 1000 {
		t.Errorf("after it: %d, want 1000", earned(p, 0))
	}
}

// It is kept over a logout (and so a keep restart, which saves the effects as a logout does) with what is left of it,
// its time running again at the login, and gives its 10% there. The control: the new player before the restore earns
// the plain award.
func TestARewardOfExperienceIsKeptOverALogout(t *testing.T) {
	k := newKillWorld(t, 20)
	t0 := time.Now()
	k.w.GiveRewardXP(k.sess[0], "Jibrish", t0)
	rows, ok := k.w.SavedEffects(k.players[0].ServerID, t0.Add(10*time.Minute))
	if !ok || len(rows) != 1 || rows[0].SpellID != RewardXPEffectID || rows[0].LeftMS != (20*time.Minute).Milliseconds() ||
		rows[0].Name != RewardXPName || rows[0].Icon != RewardXPIcon {
		t.Fatalf("kept: %+v %v", rows, ok)
	}
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 2, 9), Port: 7309}, 1, 1, 1, 2, true)
	p := &Player{ServerID: 2009, Name: "Sharer", Level: 20, HP: 100, MaxHP: 100}
	w.Players[p.ServerID] = p
	s.SetECSEntity(p)
	s.SetCharacterInWorld(true)
	w.AwardXP(s, p, 1000)
	if earned(p, 0) != 1000 {
		t.Fatalf("the control: %d", earned(p, 0))
	}
	login := t0.Add(5 * time.Hour)
	if n := w.RestoreEffects(s, rows, login); n != 1 {
		t.Fatalf("restored %d", n)
	}
	if n, e := rewardXPOf(p); n != 1 || !e.Until.Equal(login.Add(20*time.Minute)) {
		t.Fatalf("after the login: %d, %+v", n, e)
	}
	if left := w.RewardXPLeft(s, login); left != 20*time.Minute {
		t.Errorf("left: %v", left)
	}
	w.AwardXP(s, p, 1000)
	if got := earned(p, 0); got != 2100 {
		t.Errorf("after the login: %d, want %d", got, 1000+1100)
	}

}
