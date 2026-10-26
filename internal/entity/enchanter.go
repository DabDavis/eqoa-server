package entity

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// enchanter.go — the enchanter's lines that the rest of the spell code read
// but did not act on, made to work (the user, 2026-10-26: "the enchanter
// fixes, but in our own way"). The sources are public and this server's
// own: EQOA-Data's spell lists (the lines, as spelleffect holds them) and
// its masteries list (Information/databases/cms/enchanter.json: Anarchic
// Reign "Group melee-proc buff", Paralytic Enchantment "AR Debuff", Emphathy
// "Group/AoE PoT buff", Power Burst "Targetable 10k power refill"). What no
// source gives is ours, and said so where it is set.
//
//   - The proc lines go by their names (procFamily): the Chaotic procs (Flux,
//     Mark, Glyph, Rune), the Anarchic ones (the same four, for the group) and
//     the Animator's runes (Stormlord's Rune, Rune of Animation) are weapon
//     procs whatever their record's description says. The lists word every
//     one as a hit with a duration ("Group, Decrease Hitpoints by 64, 5min"),
//     and only the dump records of Chaotic Flux, Mark and Glyph have a
//     description naming a proc (ParseSpell's test), so Chaotic Rune's
//     imported record hit for 256 as a nuke and the Anarchic line put an
//     empty icon on the group. The Chaotic nukes (Feedback, Shock) last
//     nothing and stay nukes. A proc line's scale ("; Increase 10% based on
//     DEX", Stormlord's Rune's) is its caster's, taken at its landing as
//     every line's scale is (lasting).
//   - A group proc reaches the members' pets too (groupProcPetsLocked): the
//     masteries list's "group melee-proc buff", and pets swing.
//   - The clarity line (clarityFamily: Breeze, Intuition, Clarity, Lucidity,
//     Tranquility, Empathy and the group's Empathic ...) is one at a time on a
//     player, the most power a tick winning, as heals over time are (hot.go):
//     a weaker one does nothing, a stronger or equal one replaces the other
//     and keeps its tick (no tick at once). Mana Flow, the Power Flow, Tide,
//     Wave and Flood bursts and the bards' songs are not of the line and add
//     to it.
//   - An arcane resist debuff on a creature (the Paralytic line, "Decrease
//     Resist Arcane by 25, 15min") lowers the chance it resists what this
//     server rolls an arcane resist for: a proc (proc.go) and a charm
//     (charm.go). By resist.go's own scale (ResistShare: 75% at 315), N AR
//     off takes that share of a hundred off the creature's chance in percent
//     (25 AR: 6 points). The strongest one counts: the five Paralytic ranks
//     are one line, and cast one over another they do not add up (ours, as
//     the clarity line). Nothing else a player casts at a creature is
//     resisted on this server yet.

// procFamilies are the names (lower case, a prefix) of the lines that are
// weapon procs (see the file's comment).
var procFamilies = []string{
	"chaotic flux", "chaotic mark", "chaotic glyph", "chaotic rune",
	"anarchic ", "stormlord's ", "rune of ",
}

// procFamily reports whether a spell of this name is a weapon proc.
func procFamily(name string) bool {
	n := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(name), "+")))
	for _, f := range procFamilies {
		if strings.HasPrefix(n, f) {
			return true
		}
	}
	return false
}

// readEnchanterProc makes a proc of a proc family's line (a hit that lasts),
// when its description did not already.
func (d *SpellDef) readEnchanterProc(name string) {
	if d.hasProc() || d.Duration <= 0 || !procFamily(name) {
		return
	}
	*d = d.asProc()
}

// hasProc reports whether the spell is a weapon proc.
func (d *SpellDef) hasProc() bool {
	for _, e := range d.Effects {
		if e.Kind == EffectProc {
			return true
		}
	}
	return false
}

// procBonus is what a proc's scale adds from its caster (0 for a line
// without one: the Chaotic lines carry no mod).
func procBonus(e SpellEffect, caster *Player) int {
	if e.Scale == StatNone || e.ScalePct == 0 || caster == nil {
		return 0
	}
	return caster.StatNow(e.Scale) * e.ScalePct / 100
}

// groupProcPetsLocked adds to a group spell's targets each member's pet in
// its range and world (the caster's own among them). Must be called with the
// lock held.
func (w *World) groupProcPetsLocked(caster *Player, ids []EntityID, r float32) []EntityID {
	for _, id := range append([]EntityID(nil), ids...) {
		m := w.Players[id]
		if m == nil {
			continue
		}
		pet := w.petOfLocked(m)
		if pet == nil || pet.Dead || pet.World != caster.World || outOfRange(caster, pet.X, pet.Y, pet.Z, r) {
			continue
		}
		ids = append(ids, pet.ServerID)
	}
	return ids
}

// clarityFamily reports whether a spell of this name is of the clarity line.
func clarityFamily(name string) bool {
	n := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(name), "+")))
	switch n {
	case "breeze", "intuition", "clarity", "lucidity", "tranquility", "empathy":
		return true
	}
	return strings.HasPrefix(n, "empathic ")
}

// clarityWeaker is what the caster is told when its clarity does not take
// (ours).
const clarityWeaker = "%s already has a stronger power over time."

// placeClarityLocked sets a clarity line's effect by the rules above and
// reports whether it goes on at all. The one it replaces comes off here.
// Must be called with the lock held.
func (w *World) placeClarityLocked(caster *Player, casterSess *session.Session, t *Player, ae *ActiveEffect, now time.Time) bool {
	if ae.PowerTick <= 0 || !clarityFamily(ae.Name) {
		return true
	}
	var old *ActiveEffect
	for _, e := range t.Effects {
		if e.SpellID != ae.SpellID && e.PowerTick > 0 && clarityFamily(e.Name) && now.Before(e.Until) {
			old = e
			break
		}
	}
	if old == nil {
		return true
	}
	if ae.PowerTick < old.PowerTick {
		log.Printf("Cast: %s's %q on %s does nothing: %q gives more", caster.Name, ae.Name, t.Name, old.Name)
		if casterSess != nil {
			w.sendColoredLocked(casterSess, fmt.Sprintf(clarityWeaker, t.Name), castRefusalRGB)
		}
		return false
	}
	// no tick at once: the next is the old one's, or the next regular tick
	// when the old one's is due (as placeHoTLocked)
	next := old.NextTick
	if !next.After(now) {
		next = w.nextRegularTick(now, now)
	}
	ae.NextTick = next
	w.removeEffectLocked(t, old.SpellID)
	return true
}

// npcResistOffPct is how many points the arcane resist debuffs on a creature
// take off its chance to resist, in percent (see the file's comment): the
// strongest one's.
func npcResistOffPct(npc *NPC, now time.Time) int {
	if npc == nil {
		return 0
	}
	off := 0
	for _, e := range npc.Effects {
		if now.Before(e.Until) {
			off = max(off, -e.Mods.AR)
		}
	}
	if off == 0 {
		return 0
	}
	return int(math.Round(ResistShare(off) * 100))
}
