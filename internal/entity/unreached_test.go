package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Mr. Moogie's unreached moment (the GM Jibwiz's petition 1916): with no melee
// hit landed for 60 s, every 6 s 20,000 pure on each player of his hate list
// that has been on it 60 s or more; one on it a shorter time is spared, and a
// hit landed puts it off again for 60 s.
func TestAnUnreachedBossStrikesItsList(t *testing.T) {
	m := database.TemplateAbility{Kind: database.AbilityUnreached, Every: 6, First: 6, Damage: 20000}
	if err := database.CheckTemplateAbilities([]database.TemplateAbility{m}); err != nil {
		t.Fatal(err)
	}
	if err := database.CheckTemplateAbilities([]database.TemplateAbility{{Kind: database.AbilityNuke, Unhit: 60}}); err == nil {
		t.Fatal("a nuke with unhit passed")
	}
	f := newAggroFix(t, 60)
	f.p.HP, f.p.MaxHP = 30000, 30000
	late := &Player{ServerID: 1002, CharServerID: 78, Name: "Late", Level: 60, HP: 30000, MaxHP: 30000, X: 1005, Y: 10, Z: 1000}
	f.w.Players[late.ServerID] = late
	boss := f.npc(3100, "mr moogie", 80, creationBody, 1010, 1000)
	boss.HP, boss.MaxHP = 3000000, 3000000
	boss.tmpl = &ClassTemplate{moments: []database.TemplateAbility{m}}
	boss.CurrentEnemy = f.p.ServerID
	boss.Threats = map[EntityID]int64{f.p.ServerID: 10}
	now := f.now
	fight := boss.fight()
	at := func(d time.Duration) { f.w.bossMomentsLocked(boss, fight, now.Add(d)) }
	at(0)
	at(30 * time.Second)
	boss.Threats[late.ServerID] = 5 // on his list 30 s in
	for d := 36 * time.Second; d <= 54*time.Second; d += 6 * time.Second {
		at(d)
	}
	if f.p.HP != 30000 || late.HP != 30000 {
		t.Fatalf("before 60 s: %d, %d", f.p.HP, late.HP)
	}
	at(60 * time.Second)
	if f.p.HP != 10000 || late.HP != 30000 {
		t.Fatalf("at 60 s: the first %d (want 10000), the late one %d (want 30000, on the list 30 s)", f.p.HP, late.HP)
	}
	// a hit lands on someone: no strike for 60 s more
	f.p.HP = 30000
	boss.meleeLanded(300, now.Add(62*time.Second))
	at(66 * time.Second)
	at(96 * time.Second)
	if f.p.HP != 30000 || late.HP != 30000 {
		t.Fatalf("after a hit landed: %d, %d", f.p.HP, late.HP)
	}
	at(122 * time.Second)
	if f.p.HP != 10000 || late.HP != 10000 {
		t.Fatalf("60 s after the hit: %d, %d (want 10000 each)", f.p.HP, late.HP)
	}
	// a miss is no hit
	boss.meleeLanded(0, now.Add(123*time.Second))
	f.p.HP = 30000
	at(128 * time.Second)
	if f.p.HP != 10000 {
		t.Fatalf("a miss put it off: %d", f.p.HP)
	}
}
