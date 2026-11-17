package entity

import (
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// charmcon.go — a charm by consider colour: the Toy Shoppe's Puppet Strings
// (the owner, 2026-11-17: "charms the target if light blue or lower", then
// "the charm has 10 charges then disappears"; tools/puppet_strings_20261117.py
// makes the record, its itemeffect line "Target, Charm up to light blue, No
// Recharge, 10min", 10 charges and a 60 s cooldown). Not the Cazic Thule drop
// of the same name (713085, "Charm from level 41 up to level 52": charm.go's
// level rules, its resist roll), which this leaves as it was.
//
// An item's use whose charm line says "up to <colour>" (SpellDef.CharmByCon)
// takes its target by how it cons to its user (ConColor, the colour the
// client shows), not by level:
//
//   - refused first, red, no charge spent and no cooldown begun, with any pet
//     of the user's out, summoned or charmed or a stacking charm's animal
//     beside it ("You already have a pet."): the strings take the one pet
//     slot, and never send a pet away;
//   - then at what no charm may take ("Your target cannot be charmed."): a
//     player (castTargetsLocked refuses one first: "Target is friendly."),
//     anything not attackable, a pet or a merc (castTargetsLocked again), a
//     name npccharm makes uncharmable or gives a resist of 100, a raid boss
//     (raidBossLocked: the loot's boss tables, a template of RaidBossHPPct), a
//     placeholder standing as its rare (npcrare), a quest step's spawn, a GM
//     trigger's, a training dummy, a monkey, a Metal Moogie's challenge
//     (castTargetsLocked);
//   - then at a creature that cons above CharmCon ("This creature is too
//     strong for the strings."): with light blue, a dark blue, white, yellow
//     or red one;
//   - else it is taken, always (no resist roll: the owner's "charms the
//     target"), as a charm spell's landing takes it (charmLocked: the pet bar,
//     the follow, the stances, npccharm's template), for the line's time
//     (10 minutes, no CHA share) or until it is let go by the charm's usual
//     rules (dismissed, its master dead or gone, out of reach: charm.go), and
//     a charge is spent (handlers/potion.go: the last takes the item, "Your
//     Puppet Strings snap.").

// charmTooStrong refuses a creature above a charm by colour's cap.
const charmTooStrong = "This creature is too strong for the strings."

// conCharmRefusalLocked is why a charm by colour (the file's doc) may not take
// npc for p ("" when it may). Must be called with the lock held.
func (w *World) conCharmRefusalLocked(p *Player, npc *NPC, spell *CastSpell) string {
	if w.petStateLocked(p) != nil || w.sidePetOfLocked(p) {
		return charmHavePet
	}
	if npc == nil || npc.Dead || npc.Owner != 0 || npc.Dummy != NotDummy || npc.isMonkey() || npc.isMerc() ||
		npc.isFamiliar() || npc.QuestSpawn || npc.EventAggro || w.raidBossLocked(npc) || w.rareStandingLocked(npc) {
		return charmCannot
	}
	if r, ok := w.charmRuleLocked(npc); ok && (!r.Charmable || r.ResistPct >= 100) {
		return charmCannot
	}
	if ConColor(p.Level, npc.Level) < spell.Def.CharmCon {
		return charmTooStrong
	}
	return ""
}

// sidePetOfLocked reports a stacking charm's animal of p's out beside its pet
// (stackcharm.go). Must be called with the lock held.
func (w *World) sidePetOfLocked(p *Player) bool {
	ps := w.sidePets[p.ServerID]
	return ps != nil && ps.owner == p && !ps.npc.Dead
}

// rareStandingLocked reports a placeholder standing as its rare (npcrare.go).
// Must be called with the lock held.
func (w *World) rareStandingLocked(npc *NPC) bool {
	r := w.rares[npc.ServerID]
	return r != nil && r.rare != nil && database.NPCClassKey(npc.Name) == database.NPCClassKey(r.rare.rec.NPCName)
}

// conCharmLocked is a charm by colour's use on npc, its checks passed: taken,
// the user its master. Must be called with the lock held.
func (w *World) conCharmLocked(p *Player, sess *session.Session, npc *NPC, spell *CastSpell, now time.Time) {
	w.broadcastCastLocked(p, npc.ServerID, 0, spell.visual())
	w.charmLocked(p, sess, npc, spell, now)
}
