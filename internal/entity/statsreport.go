package entity

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// statsreport.go — #stats (F3 20261026-014422, jib: "For testing purposes we
// need a gm /command to display extended states - such as PoT, HoT, def mods,
// off mods and any other not normally displayed stats, in the chat log"):
// what the server works out about a player or a creature and the game does
// not show, a chat line each. The numbers are the ones the server rolls and
// ticks on (regen.go, offence.go, defence.go, mastery.go, rounds.go); the old
// OFF/DEF MOD formulas (stats.go) are shown as what they are, fitted
// placeholders no roll reads.

var (
	raceNames  = []string{"Human", "Elf", "Dark Elf", "Gnome", "Dwarf", "Troll", "Barbarian", "Halfling", "Erudite", "Ogre", "Froglok"}
	classNames = []string{"Warrior", "Ranger", "Paladin", "Shadow Knight", "Monk", "Bard", "Rogue", "Druid",
		"Shaman", "Cleric", "Magician", "Necromancer", "Enchanter", "Wizard", "Alchemist"}
	archNames = [...]string{archTank: "tank", archMelee: "melee", archPriest: "priest", archCaster: "caster"}
)

func nameIn(names []string, i int) string {
	if i >= 0 && i < len(names) {
		return names[i]
	}
	return fmt.Sprintf("#%d", i)
}

// StatsReport is #stats: the lines on the GM's target when it is a player or
// a creature in the GM's view, else on the GM.
func (w *World) StatsReport(sess *session.Session, objectID uint32, now time.Time) []string {
	w.mu.Lock() // hostileLocked keeps what it worked out
	defer w.mu.Unlock()
	gm := w.sessionPlayerLocked(sess)
	if gm == nil {
		return nil
	}
	if objectID != 0 && objectID != 0xFFFFFFFF {
		if ent, ok := w.findEntityByObjectIDLocked(sess, objectID); ok {
			switch t := ent.(type) {
			case *Player:
				return w.playerStatsLocked(t, now)
			case *NPC:
				return w.npcStatsLocked(t, gm, now)
			}
		}
	}
	return w.playerStatsLocked(gm, now)
}

// playerStatsLocked is a player's lines. Must be called with the lock held.
func (w *World) playerStatsLocked(p *Player, now time.Time) []string {
	a := archetypeOf(p.Class)
	out := []string{fmt.Sprintf("%s: level %d %s %s (defends as a %s), HP %d/%d, power %d/%d",
		p.Name, p.Level, nameIn(raceNames, p.Race), nameIn(classNames, p.Class), archNames[a],
		p.HP, p.MaxHPNow(), p.Power, p.MaxPowerNow())}

	c := &p.CMEffects
	hp := p.maxHPFor(p.StatNow(StatSta))
	out = append(out, fmt.Sprintf("Max HP %d: %d from the level and STA (HP factor %d), %+d items, %+d masteries, %+d spells",
		p.MaxHPNow(), hp, hpFactor(p.Class)+c[cmSlotHPFactor], p.Gear.MaxHP, c[cmSlotHP], p.MaxHPNow()-hp-p.Gear.MaxHP-c[cmSlotHP]))
	pw := CalculateMaxPower(p.Level, p.Class, p.StatNow(StatStr), p.StatNow(StatSta), p.StatNow(StatAgi),
		p.StatNow(StatDex), p.StatNow(StatWis), p.StatNow(StatInt), p.StatNow(StatCha))
	out = append(out, fmt.Sprintf("Max power %d: %d from the level and stats, %+d items, %+d masteries, %+d spells",
		p.MaxPowerNow(), pw, p.Gear.MaxPower, c[cmSlotPower], p.MaxPowerNow()-pw-p.Gear.MaxPower-c[cmSlotPower]))
	out = append(out, fmt.Sprintf("HoT %d every 6 s: %d base (max HP/50), %+d items (%d worn), %+d meals and spells, %+d masteries",
		p.HoTNow(), p.HoTBase(), ItemCredit(p.Gear.HoTItems), len(p.Gear.HoTItems), p.Mods.HoT, p.CMEffects[cmSlotHoT]))
	out = append(out, fmt.Sprintf("PoT %d every 6 s: %d base (max power/50), %+d items (%d worn), %+d drinks and spells, %+d masteries",
		p.PoTNow(), p.PoTBase(), ItemCredit(p.Gear.PoTItems), len(p.Gear.PoTItems), p.Mods.PoT, p.CMEffects[cmSlotPoT]))

	for i, s := range []struct {
		name            string
		st              Stat
		base, gear, mod int
	}{
		{"STR", StatStr, p.BaseStr, p.Gear.Str, p.Mods.Str},
		{"STA", StatSta, p.BaseSta, p.Gear.Sta, p.Mods.Sta},
		{"AGI", StatAgi, p.BaseAgi, p.Gear.Agi, p.Mods.Agi},
		{"DEX", StatDex, p.BaseDex, p.Gear.Dex, p.Mods.Dex},
		{"WIS", StatWis, p.BaseWis, p.Gear.Wis, p.Mods.Wis},
		{"INT", StatInt, p.BaseInt, p.Gear.Int, p.Mods.Int},
		{"CHA", StatCha, p.BaseCha, p.Gear.Cha, p.Mods.Cha},
	} {
		limit := p.StatCapOf(s.st)
		cm := p.CMEffects[i]
		line := fmt.Sprintf("%s %d = base %d %+d items %+d spells %+d masteries", s.name, p.StatNow(s.st), s.base, s.gear, s.mod, cm)
		if total := s.base + s.gear + s.mod + cm; total > limit {
			line += fmt.Sprintf(" (%d past the cap of %d)", total-limit, limit)
		} else {
			line += fmt.Sprintf(" (cap %d)", limit)
		}
		out = append(out, line)
	}
	out = append(out, fmt.Sprintf("AC %d = %d items %+d spells %+d masteries; resists fire %d, cold %d, lightning %d, arcane %d, poison %d, disease %d",
		p.StatNow(StatAC), p.Gear.AC, p.Mods.AC, p.CMEffects[cmSlotAC], p.StatNow(StatFR), p.StatNow(StatCR), p.StatNow(StatLR),
		p.StatNow(StatAR), p.StatNow(StatPR), p.StatNow(StatDR)))

	foe := &NPC{Level: p.Level}
	floor, low, high := playerMeleeRoll(p, foe)
	hands := "one-handed"
	if p.Gear.WeaponTwoHanded {
		hands = "two-handed"
	}
	out = append(out, fmt.Sprintf("Melee: weapon DMG %d at %.0f%% (%s); on a level %d creature %d-%d a hit, never under %d",
		p.Gear.WeaponDamage, p.Gear.WeaponCond*100, hands, p.Level, max(low, floor), max(high, floor), floor))
	if p.AttackTarget != 0 && p.Swing.Delay > 0 {
		s := p.Swing
		line := fmt.Sprintf("Swing: %s every %.1f s, %d hits a round, reach %.1f", swingName(s.Kind), s.Delay.Seconds(), len(swingHits(s.Kind)), s.Reach)
		if s.Potion != 0 {
			line += fmt.Sprintf(", throwing potions (%d left)", s.PotionLeft)
		}
		out = append(out, line)
	} else {
		out = append(out, "Swing: worked out from the worn gear when a fight begins")
	}
	r := creatureRoll(p.Level)
	lo, hi := hitRangeOn(r, p)
	out = append(out, fmt.Sprintf("Defence: a level %d creature hits for %d-%d (AC %d x %.4f and AGI %d x %.3f off its top, a %s's %+.1f)",
		p.Level, lo, hi, p.StatNow(StatAC), r.ACSlope, p.StatNow(StatAgi), agiSlope[a], archNames[a], archOffset[a]))
	out = append(out, fmt.Sprintf("Off/def mod by the old fitted formulas (no roll uses them): OFF %d, DEF %d", p.OffMod, p.DefMod))
	out = append(out, fmt.Sprintf("Move rate %d (run speed %.2f); training points %d unspent of %d; masteries: %s, %d bought (#cm lists them)",
		p.MoveRate(), p.SpeedNow(), p.UnusedTP, p.TotalTP, MasteryLine(p.Mastery), len(p.CMs)))
	out = append(out, "Effects: "+effectsLine(p.Effects, now))
	return out
}

// npcStatsLocked is a creature's lines, the viewer's (the GM's) fight with
// it among them. Must be called with the lock held.
func (w *World) npcStatsLocked(npc *NPC, viewer *Player, now time.Time) []string {
	class := npc.Class
	if class < 0 || class > 14 {
		class = 0
	}
	out := []string{fmt.Sprintf("%s: level %d %s NPC (row %d), HP %d/%d, size %.2f, body 0x%08X",
		npc.Name, npc.Level, nameIn(classNames, class), npc.NPCID, npc.HP, npc.MaxHP, npc.Size, uint32(npc.ModelID))}
	switch {
	case npc.Owner != 0:
		owner := "someone"
		if o := w.Players[npc.Owner]; o != nil {
			owner = o.Name
		}
		out = append(out, "A pet of "+owner)
	case npc.Dummy != NotDummy:
		out = append(out, "A training dummy")
	}

	length, delay, _, hits := npcRound(npc)
	r := npcMeleeRoll(npc)
	top0 := int(math.Round(r.Top))
	out = append(out, fmt.Sprintf("Melee: %s, a round every %.1f s (its 0x00B1 says %.1f), %d hits; a hit %d-%d on a tank of no AC or AGI, never under %d",
		swingName(npcSwingKind(npc)), length.Seconds(), delay.Seconds(), len(hits), max(top0-r.Width, r.Floor, 1), max(top0, r.Floor, 1), r.Floor))
	if viewer != nil {
		lo, hi := hitRangeOn(r, viewer)
		floor, low, high := playerMeleeRoll(viewer, npc)
		out = append(out, fmt.Sprintf("On %s: %d-%d a hit (AC x %.4f off its top); %s's melee on it %d-%d, never under %d (its defence %.1f)",
			viewer.Name, lo, hi, r.ACSlope, viewer.Name, max(low, floor), max(high, floor), floor, targetDefence(npc)))
	}
	extra := npcReachExtra(npc)
	out = append(out, fmt.Sprintf("Reach %.1f%+.0f; attacks on sight: %s (within %.0f); leash %.0f, roams within %.0f",
		meleeReach, extra, yesNo(w.hostileLocked(npc)), npc.AggroRadius, npc.LeashRadius, npc.RoamRadius))
	helps := "no one"
	if rule, ok := w.npcSocial.rules[npc.Name]; ok && rule.Social {
		radius := rule.Radius
		if radius <= 0 {
			radius = DefaultSocialRadius
		}
		helps = fmt.Sprintf("%s within %.0f", w.socialFactionLocked(npc), radius)
	}
	out = append(out, fmt.Sprintf("Undead: %s; helps %s; heals out of a fight 15%% a tick from the third (out %d ticks)",
		yesNo(w.undeadLocked(npc)), helps, npc.calmTicks))
	if len(npc.Threats) == 0 {
		out = append(out, "Hate: none")
	} else {
		type foe struct {
			name string
			hate int64
		}
		var foes []foe
		for id, h := range npc.Threats {
			name := fmt.Sprintf("#%d", id)
			if p := w.Players[id]; p != nil {
				name = p.Name
			} else if n := w.NPCs[id]; n != nil {
				name = n.Name
			}
			if id == npc.CurrentEnemy {
				name += " (fighting)"
			}
			foes = append(foes, foe{name, h})
		}
		sort.Slice(foes, func(i, j int) bool { return foes[i].hate > foes[j].hate })
		var parts []string
		for i, f := range foes {
			if i == 5 {
				parts = append(parts, fmt.Sprintf("and %d more", len(foes)-5))
				break
			}
			parts = append(parts, fmt.Sprintf("%s %d", f.name, f.hate))
		}
		out = append(out, "Hate: "+strings.Join(parts, ", "))
	}
	out = append(out, "Effects: "+effectsLine(npc.Effects, now))
	return out
}

// hitRangeOn is a creature roll's hits on the player: lowest and highest.
func hitRangeOn(r meleeRoll, p *Player) (lo, hi int) {
	a := archetypeOf(p.Class)
	top := int(math.Round(r.Top - r.ACSlope*float64(p.StatNow(StatAC)) -
		agiSlope[a]*float64(p.StatNow(StatAgi)) + archOffset[a]))
	return max(top-r.Width, r.Floor, 1), max(top, r.Floor, 1)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// swingName is a swing kind as its effect's name says it ("Attack4",
// "Range1"), "none" for no swing.
func swingName(kind int) string {
	if n := SwingEffectName(kind); n != "" {
		return fmt.Sprintf("%s (kind %d)", n, kind)
	}
	return "none"
}

// effectsLine is the effects on someone, each with its time left and what it
// does each tick.
func effectsLine(effects []*ActiveEffect, now time.Time) string {
	if len(effects) == 0 {
		return "none"
	}
	var parts []string
	for _, e := range effects {
		s := e.Name
		if s == "" {
			s = fmt.Sprintf("spell %d", e.SpellID)
		}
		if e.HPTick != 0 {
			s += fmt.Sprintf(" %+d HP/tick", e.HPTick)
		}
		if e.PowerTick != 0 {
			s += fmt.Sprintf(" %+d power/tick", e.PowerTick)
		}
		if e.Until.IsZero() {
			s += " (until taken off)"
		} else if left := e.Until.Sub(now); left > 0 {
			s += " (" + left.Round(time.Second).String() + ")"
		} else {
			s += " (ending)"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ", ")
}
