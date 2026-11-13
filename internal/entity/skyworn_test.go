package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

func TestSkyWornBuffsRequireAscendedRecords(t *testing.T) {
	defer SetAscendedItems(AscendedItems())
	oldImports := importedItems.Load()
	t.Cleanup(func() { importedItems.Store(oldImports) })
	defer database.SetImportedShare(database.SetImportedShare(0.85))
	SetAscendedItems(map[int]int{980901: 716023, 980902: 716026})
	SetImportedItems(map[int]int{990901: 980901})
	for _, base := range []int{716023, 716026} {
		if _, ok := WornShieldOf(base, "Seraphic Planar item"); ok {
			t.Fatalf("base %d received an estimated Ascended effect", base)
		}
	}
	for _, tc := range []struct {
		id, amount int
		duration   time.Duration
	}{
		{980901, 110, time.Minute},
		{980902, 517, 5 * time.Minute},
		{990901, 94, time.Minute},
	} {
		s, ok := WornShieldOf(tc.id, "Ascended Seraphic Planar item")
		if !ok || s.Buff == nil {
			t.Fatalf("record %d has no buff", tc.id)
		}
		var amount int
		for _, effect := range s.Buff.Def.Effects {
			if effect.Kind == EffectStat {
				amount = effect.Amount
				break
			}
		}
		if amount != tc.amount || s.Buff.Def.Duration != tc.duration {
			t.Errorf("record %d amount %d duration %v", tc.id, amount, s.Buff.Def.Duration)
		}
	}
}

func TestSkyWornBuffsTriggerRefreshAndExpire(t *testing.T) {
	defer func(f func(int) int) { procIntn = f }(procIntn)
	for _, base := range []int{716023, 716026} {
		c, crab := casterAndCrab(t)
		crab.CurrentEnemy = 0
		originalHP, originalPower, originalCHA := c.p.MaxHPNow(), c.p.MaxPowerNow(), c.p.StatNow(StatCha)
		originalBody, _ := channelBody(c.p)
		buff := skyRaidWornBuff(base, false)
		c.p.Gear.Shields = []WornShield{{Buff: buff}}
		procIntn = func(int) int { return 10 }
		c.w.playerShieldsLocked(c.sess, c.p, crab, c.at(0))
		if len(c.p.Effects) != 0 {
			t.Fatal("a roll outside the ten percent chance procced")
		}
		procIntn = func(int) int { return 9 }
		c.w.playerShieldsLocked(c.sess, c.p, crab, c.at(time.Second))
		c.w.playerShieldsLocked(c.sess, c.p, crab, c.at(2*time.Second))
		if len(c.p.Effects) != 1 {
			t.Fatalf("record %d has %d effects after a refresh", base, len(c.p.Effects))
		}
		if base == 716023 && c.p.StatNow(StatCha) != originalCHA+110 {
			t.Fatalf("Bard buff CHA %d", c.p.StatNow(StatCha))
		}
		if base == 716026 {
			body, _ := channelBody(c.p)
			if c.p.MaxHPNow() != originalHP+517 || c.p.MaxPowerNow() != originalPower+264 || body != 0x94BA77CD {
				t.Fatalf("Enchanter buff HP %d power %d body %x", c.p.MaxHPNow(), c.p.MaxPowerNow(), body)
			}
		}
		c.tick(buff.Def.Duration + 3*time.Second)
		body, _ := channelBody(c.p)
		if len(c.p.Effects) != 0 || c.p.MaxHPNow() != originalHP || c.p.MaxPowerNow() != originalPower ||
			c.p.StatNow(StatCha) != originalCHA || body != originalBody {
			t.Fatalf("record %d did not expire cleanly", base)
		}
		c.p.HP = 0
		c.w.playerShieldsLocked(c.sess, c.p, crab, c.at(buff.Def.Duration+4*time.Second))
		if len(c.p.Effects) != 0 {
			t.Fatal("dead wearer received a proc")
		}
	}
}

func TestSkyWeaponProcEstimatesUseExistingParser(t *testing.T) {
	defer SetAscendedItems(AscendedItems())
	SetAscendedItems(map[int]int{980901: 960010, 980902: 716030})
	bow, ok := WeaponProcOf(980901, "Ascended Seraphic Planar Bow", "Procs a 12 second root, a 10% chance each hit.")
	if !ok || bow.Kind != ProcRoot || bow.rootFor() != 12*time.Second || bow.Chance != 10 {
		t.Fatalf("bow proc %+v, read %v", bow, ok)
	}
	dagger, ok := WeaponProcOf(980902, "Ascended Seraphic Planar Dagger", "Procs 200 arcane damage, a 10% chance each hit.")
	if !ok || dagger.Damage != 220 || dagger.Resist != StatAR || dagger.Chance != 10 {
		t.Fatalf("dagger proc %+v, read %v", dagger, ok)
	}
}
