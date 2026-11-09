package entity

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
)

// strip.go — a templated creature's strip (database.AbilityStrip; the GM Jibrish's petition
// 1368: "Give mr moogie a caster debuff on his target every 30 seconds that strips any and all
// buffs, including immunity to damage / god mode"). It is cast as a template's other abilities
// are (classtemplate.go): on its clock, at what the creature fights in its reach (a player, or a
// player's pet), its 0x00B1 from the creature to everyone who sees it, landing castLand after its
// cast time on a target still there. It casts no spell of a book: its name is its row's
// (database.DefaultStripName for none), with no cast time and a harmful spell's stand-in visual;
// or it pins a spell, whose record's name, cast time and visual it takes, and nothing else of it
// (the spell's own effects do not land). At its landing:
//
//   - on a player, every beneficial effect ends (beneficialEffect), each as its time running out
//     ends it (removeEffectLocked: off the effects channel, the stats summed again, a new run
//     speed told, an illusion's body, invisibility and a power ceiling going with theirs, a heal
//     over time with no last tick): its buffs, damage shields and defensive procs, heals and power
//     over time, invulnerabilities and mitigations (Divine Intervention, Eldrich Rune, the
//     paladin's barriers), speeds, forms and illusions, stances, a meal's and a drink's, a
//     potion's, and a buff with a price (a barrier's hold, a form's tick) with its price;
//   - what harms it stays: a harmful spell's effect (a creature's debuff and damage over time, a
//     duel's or a PvP foe's) and a price that gives nothing (a pact's paid on its caster); and so
//     does the towns' Boon, a blessing for playing (boon.go), as Mr. Moogie's own "Clear my
//     buffs" keeps it (moogie.go);
//   - a GM's #testmode, the server's god mode (gmchannel.go: no death, a killing hit leaving 1 HP),
//     goes off, logged; #tankprofile (a stand-in for worn armour) and the TEST menu's settings stay;
//   - it is told in a line what went (or that there was nothing), the god mode in a line of its own.
//
// On a pet its beneficial effects end as at their time (petEffectsLocked), its owner told. No
// resist takes it, and it is no hit: nothing that ends at a hit ends for it. The words and the
// default name are ours.

// stripListMost is the most characters of names a strip's line lists; the rest are counted, so a
// line stays under 200 characters.
const stripListMost = 120

// stripSpell is a strip's cast when it pins no spell: its name, no cast time, a harmful spell's
// stand-in visual (StandInVisual).
func stripSpell(a database.TemplateAbility) *CastSpell {
	name := a.Name
	if name == "" {
		name = database.DefaultStripName
	}
	return &CastSpell{Name: name, Scope: 1, Def: SpellDef{Harmful: true, Target: TargetSingle}}
}

// beneficialEffect reports an effect its bearer is the better for, which a strip takes off: one
// that gives anything (gives), with its price if it has one (a pact's stat, a form's tick, a
// barrier's hold, the Eye line's power ceiling go with it, where harmfulEffect alone would keep
// them), or that does nothing the server reads (a buff's icon alone). Never a harmful spell's
// (Rules.Harm: a creature's debuff), a duel's or a PvP foe's (duelFoe), a creature's damage over
// time (npcFoe), a price that gives nothing (a pact's paid on its caster, apart from what it
// gave), or the towns' Boon.
func beneficialEffect(ae *ActiveEffect) bool {
	switch {
	case ae.Rules.Harm || ae.duelFoe != 0 || ae.npcFoe != 0 || ae.SpellID == BoonEffectID:
		return false
	case ae.gives():
		return true
	}
	return !ae.costs()
}

// gives reports whether an effect gives its bearer anything: a stat or a maximum raised, a heal
// or power over time, a run speed, invisibility or hiding, a body (an illusion, a form), a damage
// shield or a defensive proc, a proc, a meal's or a drink's rule, a mitigation or an
// invulnerability, enduring breath, a proc's heal, a disguise.
func (ae *ActiveEffect) gives() bool {
	r := &ae.Rules
	return ae.Mods.gives() || ae.HPTick > 0 || ae.PowerTick > 0 || ae.Speed || ae.Invis || ae.Form != 0 ||
		ae.Shield != nil || ae.Proc != 0 || ae.ProcMax != 0 || len(ae.ProcSpells) > 0 || ae.Food != nil ||
		r.MitigatePct > 0 || r.Breath || r.Hide || r.ProcHeal != 0 || r.Disguise
}

// costs reports whether an effect costs its bearer anything: a stat taken, a tick that takes hit
// points or power, a hold, a slow, a power ceiling.
func (ae *ActiveEffect) costs() bool {
	return ae.Mods.harmful() || ae.HPTick < 0 || ae.PowerTick < 0 || ae.Root || ae.Snare || ae.PowerCapped ||
		ae.Rules.SpeedPct < 0
}

// gives is whether the changes raise anything.
func (m StatMods) gives() bool {
	for _, v := range []int{m.Str, m.Sta, m.Agi, m.Dex, m.Wis, m.Int, m.Cha, m.AC, m.MaxHP, m.MaxPower,
		m.FR, m.CR, m.PR, m.DR, m.LR, m.AR, m.HoT, m.PoT, m.Melee, m.Pierce, m.Blunt, m.Slash, m.HPFactor, m.OffMod, m.DefMod} {
		if v > 0 {
			return true
		}
	}
	return false
}

// stripPlayerLocked ends every beneficial effect on p and turns a GM's #testmode off. It reports
// the names of the effects that went (one already run out is not named, an unnamed one is "an
// effect") and whether the god mode was on. Must be called with the lock held.
func (w *World) stripPlayerLocked(p *Player, now time.Time) (names []string, god bool) {
	for _, ae := range append([]*ActiveEffect(nil), p.Effects...) {
		if !beneficialEffect(ae) || !w.removeEffectLocked(p, ae.SpellID) {
			continue
		}
		if now.Before(ae.Until) {
			names = append(names, stripWord(ae))
		}
	}
	if p.GMTestMode {
		p.GMTestMode, god = false, true
	}
	p.StatsDirty = true
	return names, god
}

// stripPetLocked ends every beneficial effect on a pet as its time running out ends it
// (petEffectsLocked: off its effects, its maximum hit points taken back), and reports the names
// of those that went. Must be called with the lock held.
func (w *World) stripPetLocked(pet *NPC, now time.Time) (names []string) {
	keep := make([]*ActiveEffect, 0, len(pet.Effects))
	for _, ae := range pet.Effects {
		if !beneficialEffect(ae) {
			keep = append(keep, ae)
			continue
		}
		if now.Before(ae.Until) {
			names = append(names, stripWord(ae))
		}
	}
	pet.Effects = keep
	pet.syncEffectMaxHP()
	return names
}

// templateStripLocked lands a templated NPC's strip (the file's doc) on what it fights: a living
// player in its world, else a living pet of a player's there. Must be called with the lock held.
func (w *World) templateStripLocked(c *npcCast, now time.Time) {
	npc, name := c.npc, c.spell.Name
	if p := w.Players[c.target]; p != nil {
		if p.IsDead() || p.respawnPending() || p.World != npc.World {
			return
		}
		names, god := w.stripPlayerLocked(p, now)
		p.LastCombatTime = now
		if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
			w.sendChatToSession(sess, stripLine(npc.Name, name, "your", "you", names))
			if god {
				w.sendChatToSession(sess, fmt.Sprintf("%s's %s strips your god mode: #testmode is off.", npc.Name, name))
			}
		}
		stripLog(npc.Name, name, p.Name, names)
		if god {
			log.Printf("Template: %s's %q turns %s's god mode off (#testmode)", npc.Name, name, p.Name)
		}
		return
	}
	pet := w.NPCs[c.target]
	if pet == nil || pet.Owner == 0 || pet.Dead || pet.World != npc.World {
		return
	}
	names := w.stripPetLocked(pet, now)
	pet.LastCombatTime = now
	if owner := w.Players[pet.Owner]; owner != nil {
		owner.StatsDirty = true // its pet's effects are in its effects window (effects.go)
		if sess := w.findSessionByEntityIDLocked(owner.ServerID); sess != nil {
			w.sendChatToSession(sess, stripLine(npc.Name, name, "your pet's", "your pet", names))
		}
	}
	stripLog(npc.Name, name, fmt.Sprintf("the pet %q", pet.Name), names)
}

// stripLog says a strip's landing in the server's log: every name that went, or none.
func stripLog(npc, spell, whom string, names []string) {
	if len(names) == 0 {
		log.Printf("Template: %s's %q finds no buffs on %s", npc, spell, whom)
		return
	}
	log.Printf("Template: %s's %q strips %s of %d buffs (%s)", npc, spell, whom, len(names), strings.Join(names, ", "))
}

// stripWord is an effect's name in a strip's line.
func stripWord(ae *ActiveEffect) string {
	if ae.Name != "" {
		return ae.Name
	}
	return "an effect"
}

// stripLine is what a strip's bearer (or its pet's owner) is told: whose buffs went, by name, or
// that it found none on whom.
func stripLine(npc, spell, whose, whom string, names []string) string {
	if len(names) == 0 {
		return fmt.Sprintf("%s's %s finds no buffs on %s.", npc, spell, whom)
	}
	return fmt.Sprintf("%s's %s strips %s buffs: %s.", npc, spell, whose, stripList(names))
}

// stripList is names joined for a line, those past stripListMost characters counted ("and 3
// more").
func stripList(names []string) string {
	var b strings.Builder
	for i, n := range names {
		if i > 0 && b.Len()+2+len(n) > stripListMost {
			fmt.Fprintf(&b, " and %d more", len(names)-i)
			break
		}
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(n)
	}
	return b.String()
}

// stripStatsWords is #stats' words for a template's strip (templateLineLocked): its name and
// cast, its clock, and what it does.
func stripStatsWords(a *templateAbility, when string) string {
	if a.pinned == nil {
		return fmt.Sprintf("strip nothing (%s) %s", pinWords(a.TemplateAbility), when)
	}
	return fmt.Sprintf("strip %s (%v cast) %s: every buff off what it fights, and a GM's #testmode", a.pinned.Name, a.pinned.CastTime, when)
}
