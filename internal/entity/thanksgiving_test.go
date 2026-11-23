package entity

import (
	"os"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

func TestThanksgivingRaid(t *testing.T) {
	file := os.Getenv("EQOA_EVENT_SQL")
	if file == "" {
		t.Skip("requires the selected Heirloom content fixture")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(t.TempDir() + "/raid.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(string(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("UPDATE gmseasons SET active=1 WHERE name='Thanksgiving'"); err != nil {
		t.Fatal(err)
	}
	f := newAggroFix(t, 60)
	w := f.w
	w.LoadNPCs(database.NewNPCStore(db))
	if _, _, err = w.LoadBalance(db); err != nil {
		t.Fatal(err)
	}
	if _, err = w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	if _, _, err = w.LoadSpawnOnly(db); err != nil {
		t.Fatal(err)
	}
	from := func(name string, hostile bool) *NPC {
		t.Helper()
		id, _, ok := w.SpawnEventNPCFacing(name, 0, 1000, 10, 1000, 1002, 1000, 0, hostile)
		if !ok {
			t.Fatal("could not spawn " + name)
		}
		return w.NPCs[id]
	}
	king, santa, grinch := from("Turkey King", true), from("Santa Claus of Freeport", false), from("The Grinch", false)
	qov := w.NPCs[119599]
	if santa.tmpl == nil || santa.tmpl != grinch.tmpl || santa.MaxHP != grinch.MaxHP {
		t.Fatal("scene Santa lacks the Grinch's combat stats")
	}
	if qov == nil || qov.tmpl == nil || king.HP != 2000000 || king.MaxHP != king.HP || king.tmpl.MeleePct != qov.tmpl.MeleePct || king.tmpl.SpecialDamage != qov.tmpl.SpecialDamage || king.tmpl.Special != qov.tmpl.Special || len(king.tmpl.abilities) != 1 || len(king.tmpl.summons) != 9 {
		t.Fatalf("King's HP or combat template wrong (QoV %+v)", qov != nil)
	}
	a := king.tmpl.abilities[0]
	if a.pinned == nil || a.pinned.Name != "Turkey time" || a.every != time.Minute || a.pinned.Def.Duration != 10*time.Minute || a.pinned.Area.Radius <= 0 {
		t.Fatalf("wrong debuff %+v", a.TemplateAbility)
	}
	near := &Player{ServerID: 801, Name: "Near", World: 0, X: king.X + 8, Y: 10, Z: king.Z, HP: 10000, MaxHP: 10000}
	far := &Player{ServerID: 802, Name: "Far", World: 0, X: king.X + a.pinned.Area.Radius + 1, Y: 10, Z: king.Z, HP: 10000, MaxHP: 10000}
	w.Players[near.ServerID], w.Players[far.ServerID] = near, far
	for _, now := range []time.Time{f.now, f.now.Add(time.Minute)} {
		w.templateLandLocked(&npcCast{npc: king, tmpl: king.tmpl, spell: a.pinned, debuff: true, target: f.p.ServerID}, now)
		for _, target := range []*Player{near, f.p} {
			if len(target.Effects) != 1 {
				t.Fatalf("%s has %d debuffs", target.Name, len(target.Effects))
			}
			e := target.Effects[0]
			if e.Mods.Str != -200 || e.Mods.Dex != -200 || !e.Until.Equal(now.Add(10*time.Minute)) {
				t.Fatalf("wrong stats or expiry for %s: %+v", target.Name, e)
			}
		}
		if len(far.Effects) != 0 {
			t.Fatal("out-of-range player was debuffed")
		}
	}
	hurt := func(n int) { damage := w.npcTakesLocked(king, n, f.now); king.HP -= damage }
	hurt(199999)
	if len(w.adds) != 0 {
		t.Fatal("early adds")
	}
	hurt(1)
	if len(w.adds) != 5 {
		t.Fatal("exactly 90% did not produce five adds")
	}
	king.HP += 100
	hurt(100)
	if len(w.adds) != 5 {
		t.Fatal("healing retriggered a wave")
	}
	hurt(400000)
	if len(w.adds) != 15 {
		t.Fatal("hit crossing 80% and 70% lost a wave")
	}
	for i := 6; i >= 1; i-- {
		hurt(200000)
		if got := len(w.adds); got != 5*(10-i) {
			t.Fatalf("%d%% produced %d adds", i*10, got)
		}
	}
	hurt(200000)
	if len(w.adds) != 45 {
		t.Fatalf("death wave gave %d adds", len(w.adds))
	}
	_, _, _, kh := npcRound(king)
	if len(kh) != 4 {
		t.Fatalf("king hits %d times", len(kh))
	}
	for _, add := range w.adds {
		if add.tmpl == nil {
			t.Fatalf("Leftovers has no combat template: %v", w.ClassTemplateProblems())
		}
		_, _, _, ah := npcRound(add)
		if !add.leftover() || add.MaxHP != 1000 || add.HP != 1000 || len(ah) != 4 || add.shared(add.meleeOut(160))*2 != king.meleeOut(160) || len(add.tmpl.abilities) != 0 || len(add.tmpl.summons) != 0 {
			t.Fatalf("add is Leftovers %v, HP %d/%d, hits %d, melee %d vs King %d, abilities %d, summons %d", add.leftover(), add.HP, add.MaxHP, len(ah), add.shared(add.meleeOut(160)), king.meleeOut(160), len(add.tmpl.abilities), len(add.tmpl.summons))
		}
	}
	add := w.adds[0]
	f.p.HP, f.p.MaxHP = 9000, 9000
	w.tallyDamageLocked(add, near, 990)
	w.tallyDamageLocked(add, f.p, 10)
	w.killNPC(add)
	w.killCreditLocked(add, f.sess, f.p)
	w.killCreditLocked(add, f.sess, f.p)
	if f.p.HP != 7000 || near.HP != 10000 {
		t.Fatal("backlash must hit only the actual finisher once")
	}
	// A formed raid shares credit with its nearby members when the temporary boss dies.
	f.p.GroupID, near.GroupID = 17, 17
	near.CharServerID = 81
	w.groups = map[uint32]*Group{17: {ID: 17, flagged: true, Members: []EntityID{f.p.ServerID, near.ServerID}}}
	var credit KillCredit
	w.OnKill(func(k KillCredit) { credit = k })
	w.killNPC(king)
	w.creditKillLocked(f.sess, f.p, king)
	if credit.OriginalRow != turkeyKingRow || credit.NPCSpawn != 0 || len(credit.Players) != 2 || credit.Players[1].ServerID != near.CharServerID {
		t.Fatalf("raid kill credit %+v", credit)
	}
	t.Logf("King and adds quad, %d miniature adds across nine thresholds, two debuff casts refreshed both players", len(w.adds))
}

func TestThanksgivingRewardProc(t *testing.T) {
	r := newProcRig(t)
	wp, ok := WeaponProcOf(turkeySmasherPattern, "Turkey Smasher", "This procs.. something, but you're not sure what")
	if !ok || wp.Kind != ProcMysteryTravel || wp.Chance != 1 {
		t.Fatal("missing mystery proc")
	}
	r.friend.Gear.MainProc = &wp
	old := procIntn
	defer func() { procIntn = old }()
	count := 0
	for i := 0; i < 100; i++ {
		procIntn = func(n int) int { return i % n }
		r.w.weaponProcLocked(r.fsess, r.friend, r.npc, time.Now())
		if len(r.w.gates.due) > 0 {
			g := r.w.gates.due[0]
			if g.P != r.friend || g.Sess != r.fsess || g.To.World != 0 || g.To.X != 14607 || g.To.Z != 19045 || g.To.Y < 54 {
				t.Fatal("wrong traveler or landing")
			}
			count++
			r.w.gates.due = nil
		}
	}
	if count != 1 {
		t.Fatalf("teleport succeeded on %d of 100 distinct rolls", count)
	}
	procIntn = func(int) int { return 0 }
	r.npc.HP = 1
	if !r.hit() || len(r.w.gates.due) != 1 {
		t.Fatal("the club's killing blow did not proc")
	}
}
