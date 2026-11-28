package entity

import "testing"

// The owner, 2026-11-28: "elder, faithful, bard heals should stack", then "savage heal should stack also with other
// HoT". Faithful Healing, Savage Healing and the bard's heal songs each tick in a slot of their own, beside the heals
// over time of their scopes' slots (a cleric's Celestial Healing's 1, a self heal's 2, a group's 3), the bard's songs one
// slot among themselves; Tribal Ward's hit-set-off heal has its proc's own slot. The controls: Celestial Healing, a self
// heal and a group heal keep their scopes' slots, and the slots here are all apart.
func TestElderFaithfulBardAndSavageHealsHaveSlotsOfTheirOwn(t *testing.T) {
	wb := func(id, stack, scope int) *CastSpell {
		return &CastSpell{SpellID: id, Scope: scope, Def: SpellDef{Workbook: true, Stack: stack}}
	}
	faithful, celestial := hotSlot(wb(701138, 1, 1)), hotSlot(wb(700771, 1, 1))
	savage, selfHeal := hotSlot(&CastSpell{SpellID: 700643, Scope: 0}), hotSlot(&CastSpell{SpellID: 701066, Scope: 0})
	hymnNife, celebNife, hymnVigor := hotSlot(wb(700788, 3, 2)), hotSlot(wb(701117, 3, 2)), hotSlot(wb(700034, 3, 2))
	hearth := hotSlot(wb(760132, 3, 2))
	if celestial != 1 || selfHeal != 2 || hearth != 3 {
		t.Fatalf("the scopes' slots: Celestial %d, a self heal %d, Hearth Blessing %d; want 1, 2, 3", celestial, selfHeal, hearth)
	}
	if faithful == celestial || savage == selfHeal || hymnNife == hearth {
		t.Errorf("a slot shared: Faithful %d (Celestial %d), Savage %d (self %d), Hymn of Nife %d (Hearth %d)",
			faithful, celestial, savage, selfHeal, hymnNife, hearth)
	}
	if hymnNife != celebNife || hymnNife != hymnVigor {
		t.Errorf("the bard's songs apart: Hymn of Nife %d, Celebration of Nife %d, Hymn of Vigor %d", hymnNife, celebNife, hymnVigor)
	}
	tribal := hotSlot(&CastSpell{SpellID: procSpellID(701246, 0), Def: SpellDef{Workbook: true, Stack: procHoTSlot(procSpellID(701246, 0))}})
	seen := map[int]string{1: "single", 2: "self", 3: "group", 4: "illusion"}
	for name, slot := range map[string]int{"Faithful": faithful, "Savage": savage, "bard": hymnNife, "Tribal Ward": tribal} {
		if prev, ok := seen[slot]; ok {
			t.Errorf("%s's slot %d is %s's too", name, slot, prev)
		}
		seen[slot] = name
	}
}
