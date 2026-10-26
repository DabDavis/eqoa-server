package entity

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// #stats (F3 20261026-014422): the lines on a player (HoT/PoT and their parts,
// a stat past its cap, the effects with their time) and on a creature in view
// (its round, its hits on the GM, the hate list in order); an unknown target
// is the GM's own.
func TestStatsReport(t *testing.T) {
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 7000}, 1, 1, 1, 2, true)
	p := &Player{ServerID: 1001, Name: "Tester", Class: 14, Race: 1, Level: 60, Speed: 8.833333,
		BaseStr: 60, BaseSta: 347, BaseAgi: 70, BaseDex: 70, BaseWis: 80, BaseInt: 90, BaseCha: 60,
		Gear: Gear{Sta: 280, AC: 230, HoTItems: []int{5, 3}, WeaponDamage: 30, WeaponCond: 1}}
	p.RecalcStats(true)
	now := time.Now()
	p.Mods.HoT = 4 // a meal
	p.Effects = []*ActiveEffect{
		{SpellID: 9000, Name: "Invisibility", Invis: true, Until: now.Add(90 * time.Second)},
		{SpellID: 9001, Name: "Healing", HPTick: 20, Until: now.Add(24 * time.Second)},
	}
	w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	guard := &NPC{ServerID: 3000, Name: "Guard Felgus", Level: 45, Class: 0, HP: 1800, MaxHP: 1800, Size: 1,
		ModelID: 0x70D898C6, PrimaryWeapon: 1, Threats: map[EntityID]int64{p.ServerID: 300, 3001: 900},
		CurrentEnemy: p.ServerID}
	pet := &NPC{ServerID: 3001, Name: "Fire Elemental", Level: 39}
	w.NPCs[guard.ServerID], w.NPCs[pet.ServerID] = guard, pet
	vs := GetOrCreateVisibility(sess)
	vs.ChannelEntity[0] = guard.ServerID
	vs.ChannelObjectID[0] = 0x0133E8A0

	has := func(lines []string, want string) bool {
		for _, l := range lines {
			if strings.Contains(l, want) {
				return true
			}
		}
		return false
	}

	mine := w.StatsReport(sess, 0, now)
	if len(mine) == 0 || !strings.HasPrefix(mine[0], "Tester: level 60 Elf Alchemist (defends as a caster)") {
		t.Fatalf("first line %q", mine)
	}
	limit := p.StatCapOf(StatSta)
	for _, want := range []string{
		fmt.Sprintf("HoT %d every 6 s: %d base (max HP/50), %+d items (2 worn), +4 meals and spells, +0 masteries", p.HoTNow(), p.HoTBase(), ItemCredit(p.Gear.HoTItems)),
		fmt.Sprintf("STA %d = base 347 +280 items +0 spells +0 masteries (%d past the cap of %d)", limit, 627-limit, limit),
		fmt.Sprintf("STR %d = base 60 +0 items +0 spells +0 masteries (cap %d)", p.StatNow(StatStr), p.StatCapOf(StatStr)),
		", 0 bought (#cm lists them)",
		"Invisibility (1m30s)", "Healing +20 HP/tick (24s)",
		"Off/def mod by the old fitted formulas (no roll uses them)",
	} {
		if !has(mine, want) {
			t.Errorf("no line with %q in\n%s", want, strings.Join(mine, "\n"))
		}
	}

	its := w.StatsReport(sess, 0x0133E8A0, now)
	if len(its) == 0 || !strings.HasPrefix(its[0], "Guard Felgus: level 45 Warrior NPC") {
		t.Fatalf("first line %q", its)
	}
	lo, hi := hitRangeOn(npcMeleeRoll(guard), p)
	for _, want := range []string{
		fmt.Sprintf("On Tester: %d-%d a hit", lo, hi),
		"Hate: Fire Elemental 900, Tester (fighting) 300",
	} {
		if !has(its, want) {
			t.Errorf("no line with %q in\n%s", want, strings.Join(its, "\n"))
		}
	}

	// control: a target the GM cannot see is the GM's own report
	if other := w.StatsReport(sess, 0x0BADBEEF, now); len(other) == 0 || !strings.HasPrefix(other[0], "Tester:") {
		t.Errorf("an unknown target gave %q", other)
	}
}
