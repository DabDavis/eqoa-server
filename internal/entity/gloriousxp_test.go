package entity

import (
	"testing"
	"time"
)

func TestGloriousExperienceAwardsExactlyFifteenTimesBase(t *testing.T) {
	k := newKillWorld(t, 30, 30)
	user, target := k.players[0], k.players[1]
	userSession, targetSession := k.sess[0], k.sess[1]
	userSession.SetPlayerObjectID(4001)
	const targetID = 4002
	visible := GetOrCreateVisibility(userSession)
	visible.ChannelEntity[0], visible.ChannelObjectID[0] = target.ServerID, targetID
	start := time.Now()
	potion := NewPotionEffect(42, "Glorious Experience Test", "Target, Glorious Experience, 1hr", "", 123, "", 0, 0)
	use := func(now time.Time) {
		t.Helper()
		if refusal, err := k.w.UsePotion(userSession, potion, targetID, now); refusal != "" || err != nil {
			t.Fatalf("use refused %q, error %v", refusal, err)
		}
	}
	award := func(want int) {
		t.Helper()
		before := earned(target, 0)
		k.w.AwardXP(targetSession, target, 100)
		if got := earned(target, 0) - before; got != want {
			t.Fatalf("100 base XP paid %d, want %d", got, want)
		}
	}

	use(start)
	award(1500)
	k.w.AwardXP(userSession, user, 100)
	if got := earned(user, 0); got != 100 || user.hasEffect(gloriousXPEffectID) {
		t.Fatalf("potion user earned %d, want the unboosted 100", got)
	}
	k.w.SetBoon(Boon{Percent: 600, Duration: 4 * time.Hour})
	k.w.GiveBoon(targetSession, "Freeport", start)
	k.w.GiveRewardXP(targetSession, "GM", start)
	award(1500)
	if craft, ok := k.w.CraftView(targetSession); !ok || craft.XPPct != 1500 {
		t.Fatalf("crafting XP percentage %d, want 1500", craft.XPPct)
	}
	use(start.Add(10 * time.Minute))
	award(1500)

	rows, _ := k.w.SavedEffects(target.ServerID, start.Add(20*time.Minute))
	doses := 0
	for _, row := range rows {
		if row.SpellID == gloriousXPEffectID {
			doses++
			if row.LeftMS != (50*time.Minute).Milliseconds() || row.Name != potion.Name {
				t.Fatalf("saved potion %q with %d ms", row.Name, row.LeftMS)
			}
		}
	}
	if doses != 1 {
		t.Fatalf("saved %d doses after renewal, want one", doses)
	}
	afterLogin := newKillWorld(t, 30)
	afterLogin.w.SetBoon(k.w.Boon())
	login := start.Add(24 * time.Hour)
	afterLogin.w.RestoreEffects(afterLogin.sess[0], rows, login)
	afterLogin.w.AwardXP(afterLogin.sess[0], afterLogin.players[0], 100)
	if got := earned(afterLogin.players[0], 0); got != 1500 {
		t.Fatalf("after login 100 base XP paid %d", got)
	}
	k.w.mu.Lock()
	k.w.clearSpellsLocked(target)
	k.w.mu.Unlock()
	award(1500)
	k.w.mu.Lock()
	k.w.effectSystem(start.Add(70 * time.Minute))
	k.w.mu.Unlock()
	award(600)
}

func TestGloriousExperienceRefusesIneligibleTargets(t *testing.T) {
	r := newAreaRig(t)
	potion := NewPotionEffect(42, "Glorious Experience Test", "Target, Glorious Experience, 1hr", "", 123, "", 0, 0)
	dead := r.player(3001, "Adventurer", 1002, 1000)
	dead.HP = 0
	pet := r.petOf(r.p, 3002, 1003, 1000)
	far := r.player(3003, "Traveller", 1000+PotionRange+5, 1000)
	for _, id := range []uint32{0, r.objectID(dead.ServerID), r.objectID(pet.ServerID), r.objectID(far.ServerID)} {
		if refusal, err := r.w.UsePotion(r.sess, potion, id, r.t0); refusal == "" || err != nil {
			t.Fatalf("target %d: refusal %q, error %v", id, refusal, err)
		}
	}
}
