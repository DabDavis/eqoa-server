package entity

import "strings"

const (
	drunkUncleProtection = 1000813
	yellingAtCousins     = 1000814
)

func isDrunkUncleProtection(spellID int, name string) bool {
	return spellID == PotionEffectID(drunkUncleProtection) && strings.EqualFold(name, "Drunk uncle protection")
}

// IsYellingAtCousins identifies the Thanksgiving charm among ordinary charged items.
func (pe *PotionEffect) IsYellingAtCousins() bool {
	return pe.PatternID == yellingAtCousins && strings.EqualFold(pe.ItemName, "Yelling at cousins") && pe.Def.Charms()
}

// thanksgivingCharmRefusalLocked checks permanent immunity and the normal
// level cap. Eligible creatures skip the ordinary charm resist roll.
func (w *World) thanksgivingCharmRefusalLocked(npc *NPC, spell *CastSpell) string {
	if npc.Dead || npc.Owner != 0 || npc.Dummy != NotDummy {
		return charmCannot
	}
	// never a raid boss, a quest or event spawn, a rare standing, a familiar or the barrel's monkeys:
	// the same creatures the Puppet Strings refuse (charmcon.go), so a sure charm takes no boss
	if npc.isMonkey() || npc.isMerc() || npc.isFamiliar() || npc.QuestSpawn || npc.EventAggro || w.raidBossLocked(npc) || w.rareStandingLocked(npc) {
		return charmCannot
	}
	if rule, ok := w.charmRuleLocked(npc); ok && (!rule.Charmable || rule.ResistPct >= 100) {
		return charmCannot
	}
	return w.charmStartLocked(spell, []EntityID{npc.ServerID})
}
