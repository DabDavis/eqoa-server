package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The season's currency (seasonrewards.go): earned while the season is on, by a
// kill's drop (named creatures more, green ones none), the gift NPC once a day
// and each full hour played; nothing while it is off.

type rewardsFix struct {
	*trigWorld
	r       *SeasonRewards
	dropped []uint32 // characters a token dropped for
}

func newRewardsFix(t *testing.T) *rewardsFix {
	w := newTrigWorld(t)
	for _, q := range []string{
		`INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
			VALUES (716900, 1, 0, 140265582, 0, -1, 0, 0, 1, 0, 0, 0, 0, 0, 999, 0, 0, 32767, 1023, 0, 0, 0, 0, 'Holly Token', '', 0, 0)`,
		`INSERT INTO seasonrewards (season, item, drop_pct, named_pct, gift_npc, gift_count, hourly) VALUES ('Christmas', 716900, 4, 25, 'Holiday Elf', 3, 1)`,
	} {
		if _, err := w.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	f := &rewardsFix{trigWorld: w}
	f.r = NewSeasonRewards(w.db, w.gw, w.sm, nil)
	f.r.drop = func(_ entity.EntityID, _ time.Time, serverID uint32, rec database.CharInventoryEntry) {
		if rec.Pattern.PatternID == 716900 {
			f.dropped = append(f.dropped, serverID)
		}
	}
	f.r.now = func() time.Time { return w.now }
	return f
}

func (f *rewardsFix) season(on bool) {
	if _, err := f.db.SetSeason("Christmas", on, "test"); err != nil {
		f.t.Fatal(err)
	}
	f.r.Refresh()
}

func (f *rewardsFix) kill(name string, level int, roll float64) {
	f.r.roll = func() float64 { return roll }
	f.r.Killed(entity.KillCredit{NPCName: name, NPCID: 900, Level: level, Died: f.now,
		Players: []entity.KillPlayer{{Session: f.gs, ServerID: f.gp.CharServerID, Name: f.gp.Name}}})
}

func TestSeasonDropsByConAndName(t *testing.T) {
	f := newRewardsFix(t)
	lvl := f.gp.Level
	f.kill("large rat", lvl, 0.01)
	if len(f.dropped) != 0 {
		t.Fatalf("season off: dropped %v", f.dropped)
	}
	f.season(true)
	f.kill("large rat", lvl, 0.039) // under 4%
	f.kill("large rat", lvl, 0.041) // over
	f.kill("Lord Rat", lvl, 0.20)   // a named creature: under 25%
	if len(f.dropped) != 2 {
		t.Fatalf("season on: %d drops, want 2 (a rat under 4%%, a named one under 25%%)", len(f.dropped))
	}
	f.dropped = nil
	f.gp.Level = 50
	f.kill("large rat", 1, 0.0) // green to a level 50
	if len(f.dropped) != 0 {
		t.Fatalf("a green creature dropped %v", f.dropped)
	}
	f.season(false)
	f.gp.Level = lvl
	f.kill("Lord Rat", lvl, 0.0)
	if len(f.dropped) != 0 {
		t.Fatalf("season off again: dropped %v", f.dropped)
	}
	if !Named("Lord Nagafen") || Named("a decaying skeleton") || Named("") {
		t.Errorf("Named")
	}
}

func TestSeasonGiftOnceADay(t *testing.T) {
	f := newRewardsFix(t)
	elf := &entity.NPC{ServerID: 702, Name: "Holiday Elf"}
	gifts := func() int {
		var n int
		f.db.QueryRow(`SELECT count(*) FROM seasongifts`).Scan(&n)
		return n
	}
	f.r.Talk(f.gs, f.gp, elf)
	if gifts() != 0 {
		t.Fatalf("season off: a gift kept")
	}
	f.season(true)
	f.clear()
	f.r.Talk(f.gs, f.gp, elf)
	if gifts() != 1 {
		t.Fatalf("season on: no gift kept")
	}
	f.clear()
	f.r.Talk(f.gs, f.gp, elf)
	said := false
	for _, l := range heard(t, f.gs) {
		said = said || strings.Contains(l.text, "Come back tomorrow")
	}
	if !said {
		t.Errorf("the same day again: not told to come back tomorrow")
	}
	f.now = f.now.Add(24 * time.Hour)
	if first, _ := f.db.TakeSeasonGift("Christmas", f.gp.CharServerID, f.now.UTC().Format("2006-01-02")); !first {
		t.Errorf("the next day: no gift")
	}
	f.r.Talk(f.gs, f.gp, &entity.NPC{ServerID: 703, Name: "Orc Pawn"})
}

func TestSeasonPlaytimeHourly(t *testing.T) {
	f := newRewardsFix(t)
	f.r.Tick(30 * time.Minute)
	if got, _ := f.db.SeasonPlay("Christmas"); len(got) != 0 {
		t.Fatalf("season off: playtime counted %v", got)
	}
	f.season(true)
	f.r.Tick(50 * time.Minute)
	got, _ := f.db.SeasonPlay("Christmas")
	if got[f.gp.CharServerID] != 3000 {
		t.Fatalf("50 minutes: %v", got)
	}
	f.r.Tick(20 * time.Minute) // past the hour: given, 10 minutes kept
	got, _ = f.db.SeasonPlay("Christmas")
	if got[f.gp.CharServerID] != 600 {
		t.Fatalf("70 minutes: %v, want 600 left toward the next hour", got)
	}
}
