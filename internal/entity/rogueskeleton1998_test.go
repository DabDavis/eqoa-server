package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Skeletal Mask as tools/rogue_skeleton_1998_20261113.py writes it (the
// halfling rogue Hoofbite's ticket 1998: "Request to add skeleton diguise
// spell for rogue"): its record in Brute's Mask's frame (a 5 s cast, self,
// scope 0) and its effects, the illusion of the workbook's sprite 0x818AF96A,
// the skeleton NPCs' CSprite 0x6AF98A81 read the other way round.

const skeletonBody uint32 = 0x6AF98A81 // npcs.modelid 1794738817

const skeletalMaskJSON = `{"v":1,"scope":"self","target_type":"Self","harmful":false,"duration_s":600,"stack":2,` +
	`"level":60,"min_level":1,"req_equipment":"Any","sheet_type":"Alter Appearance","arch_type":"Illusion",` +
	`"effects":[{"type":"Alter Appearance","kind":"illusion","sprite":"0x818AF96A"}]}`

func skeletalMask() CastSpell {
	sp := &database.SpellPatternRecord{SpellID: 907237, AbilityLvl: 60, Range: 16544, CastTime: 5, Power: 80, Scope: 0,
		Recast: 1, SpellName: "Skeletal Mask", SpellDesc: "Self, Skeleton Mask, 10min", Icon: 1507275072, IconColor: -1809115846}
	return SpellOfRow(sp, 5, "Self, Gives Form, 10min", skeletalMaskJSON, 2021015237)
}

// halflingRogue is illusionRig's player made a halfling rogue of a
// halfling's own body at 1.0 (no racesize row for halflings on either shard).
func halflingRogue(t *testing.T) *caster {
	t.Helper()
	c := illusionRig(t)
	c.p.Class, c.p.Race, c.p.Sex, c.p.ModelID, c.p.Size, c.p.Level = 6, 7, 0, int64(0xA3DB3FE2), 1, 60
	return c
}

// Cast at a creature or at no one, it shows the halfling as the skeleton, at
// a player's 1.0 (a creature's body by sprite takes its bearer's scale), on
// her effects with its pink man; it lasts ten minutes and then she is herself.
func TestTicket1998SkeletalMaskShowsTheHalflingAsASkeleton(t *testing.T) {
	c := halflingRogue(t)
	s := skeletalMask()
	if s.Def.FormModel != skeletonBody || s.Def.Duration != 10*time.Minute || s.Def.Target != TargetSelf {
		t.Fatalf("read as body %#x for %v, target %v; want %#x, 10m, self", s.Def.FormModel, s.Def.Duration, s.Def.Target, skeletonBody)
	}
	c.land(t, s, c.npcO, 0)
	if m, sz := channelBody(c.p); m != skeletonBody || sz != 1 {
		t.Fatalf("the halfling under it: model %#x scale %v; want the skeleton %#x at 1", m, sz, skeletonBody)
	}
	if c.npc.Effects != nil {
		t.Error("the disguise landed on the creature")
	}
	if len(c.p.Buffs) != 1 || c.p.Buffs[0].Name != "Skeletal Mask" {
		t.Errorf("her effects: %+v", c.p.Buffs)
	}
	c.tick(5*time.Second + castLand + 10*time.Minute + time.Second)
	if m, _ := channelBody(c.p); m != 0xA3DB3FE2 {
		t.Errorf("after its ten minutes: model %#x, want her own", m)
	}
}

// It ends as the rogue's other masks do: at Remove Mask (a cure of "mask":
// every illusion) and at another illusion, which takes its place.
func TestTicket1998SkeletalMaskEndsAsTheMasksDo(t *testing.T) {
	c := halflingRogue(t)
	c.land(t, skeletalMask(), 0xFFFFFFFF, 0)
	remove := wbSpell(700836, 1, 0, "Remove Mask", `{"v":1,"scope":"self","target_type":"Self","harmful":false,"stack":0,`+
		`"level":1,"min_level":1,"req_equipment":"Any","sheet_type":"Remove Ability","effects":[{"type":"Remove Ability","kind":"cure","removes":"ID?"}]}`)
	c.land(t, remove, 0xFFFFFFFF, 20*time.Second)
	if m, _ := channelBody(c.p); m != 0xA3DB3FE2 || len(c.p.Buffs) != 0 {
		t.Fatalf("after Remove Mask: model %#x, effects %+v; want her own and none", m, c.p.Buffs)
	}

	c.land(t, skeletalMask(), 0xFFFFFFFF, 40*time.Second)
	c.land(t, troll(), 0xFFFFFFFF, 60*time.Second)
	if m, _ := channelBody(c.p); m != trollMale {
		t.Fatalf("an illusion after it: model %#x, want the troll's", m)
	}
	for _, e := range c.p.Effects {
		if e.SpellID == 907237 {
			t.Error("Skeletal Mask still on under the troll")
		}
	}
}
