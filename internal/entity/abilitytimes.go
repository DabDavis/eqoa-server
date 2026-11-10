package entity

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
)

// abilitytimes.go — what a creature's template ability carries past one cast (the GM Jibrish's
// petition 1394: "Give Mr. Moogie a 1 hour cool down complete heal at 50% health, and if he uses it he
// says "Thats an hour cool down, you'll pay for that". After Lh he also gets a proc buff on melee
// attacks, 25% chance to proc additional melee damage that does 1500."):
//
//   - a cooldown (database.TemplateAbility.Cooldown): the least time from one of its casts' start to
//     the next, kept on the creature (NPC.abilityReady), where an ability's clock (every, first) is its
//     fight's and begins again with each fight. A fight's end, a leash home, its death and its respawn
//     leave it running, and a restart that keeps the sessions carries it (keepnpc.go); a cold start and
//     a GM's remake of its row (an edit of it) begin it again. While it runs the ability waits, due,
//     as a heal waits above its share;
//   - a heal of its share (HealPct) on a creature, as on a pet: that percent of its most hit points,
//     100 its whole, no spell's figure. It pins a spell for its name, cast time and visual (Mr.
//     Moogie's: the paladin's Lay Hands), else it is named by Name (database.DefaultShareHealName) with
//     no cast time and the heal's stand-in visual;
//   - at its landing, its line in /say (Say: "Mr. Moogie: text", to every player within a say's reach of
//     the creature), and a melee proc for the rest of that fight (ProcChance, ProcDamage, ProcName),
//     rolled at each of its melee hits that lands on a player or a pet, fixed and unresisted, told as a
//     template's own proc is (creatureshield.go). The proc is the fight's (templateFight.proc): the
//     fight's end (no one left on its hate list, a leash home), its death, a restart and a GM's edit of
//     its template take it, and the next fight has none until the ability lands again. No client shows a
//     creature's buffs, so it is no effect on the creature: #stats says it is on.
//
// The numbers are a template's (Mr. Moogie's are petfix179's); the default names are ours.

// fightProc is a melee proc an ability's landing gave a creature for the rest of its fight.
type fightProc struct {
	chance, damage int
	name           string
}

// abilityKey is an ability as its creature's cooldown knows it: its kind and what it casts, so a GM's
// edit of the template's other abilities, or of their order, leaves its cooldown running.
func abilityKey(a *database.TemplateAbility) string {
	return a.Kind + "|" + strconv.Itoa(a.SpellID) + "|" + a.Spell + "|" + a.Name + "|" + strconv.Itoa(a.HealPct)
}

// coolingDown reports whether the ability's cooldown on n runs at now.
func (n *NPC) coolingDown(a *templateAbility, now time.Time) bool {
	return a.Cooldown > 0 && now.Before(n.abilityReady[abilityKey(&a.TemplateAbility)])
}

// startCooldown begins the ability's cooldown on n at now, its cast's start.
func (n *NPC) startCooldown(a *templateAbility, now time.Time) {
	if a.Cooldown <= 0 {
		return
	}
	if n.abilityReady == nil {
		n.abilityReady = map[string]time.Time{}
	}
	n.abilityReady[abilityKey(&a.TemplateAbility)] = now.Add(time.Duration(a.Cooldown * float64(time.Second)))
}

// pendingCooldowns is n's cooldowns still running at now, nil for none: what a restart carries.
func (n *NPC) pendingCooldowns(now time.Time) map[string]time.Time {
	var out map[string]time.Time
	for k, until := range n.abilityReady {
		if now.Before(until) {
			if out == nil {
				out = map[string]time.Time{}
			}
			out[k] = until
		}
	}
	return out
}

// fightProcNow is the melee proc an ability gave n for this fight, nil for none.
func (n *NPC) fightProcNow() *fightProc {
	if n.tfight == nil {
		return nil
	}
	return n.tfight.proc
}

// shareHealSpell is a creature's heal of its share when it pins no spell: its name, no cast time and
// the heal's stand-in visual.
func shareHealSpell(a database.TemplateAbility) *CastSpell {
	name := a.Name
	if name == "" {
		name = database.DefaultShareHealName
	}
	fx := uint32(fxHeal)
	if a.FX != 0 {
		fx = a.FX // its own look (the owner's Tunare's Renewal, a druid's heal's)
	}
	return &CastSpell{Name: name, Scope: 1, FX: fx}
}

// fixedHitSpell is a creature's fixed hit (Damage) when it pins no spell: its name
// (database.DefaultFixedHitName for none), no cast time, its own visual (FX) or a
// harmful spell's stand-in (petition 1439: Mr. Moogie's Royal Siphon; 1442: Metal
// Moogie V2's Gear Barrage).
func fixedHitSpell(a database.TemplateAbility) *CastSpell {
	name := a.Name
	if name == "" {
		name = database.DefaultFixedHitName
	}
	return &CastSpell{Name: name, Scope: 1, FX: a.FX, Def: SpellDef{Harmful: true, Target: TargetSingle}}
}

// templateLandedLocked is what a templated NPC's cast does past its landing (the file's doc): its line
// in /say, and the proc it gives the NPC for the rest of its fight while that fight is on (one that
// ended before the landing gives the next none). Must be called with the lock held.
func (w *World) templateLandedLocked(c *npcCast) {
	npc := c.npc
	if npc.Dead || c.tmpl == nil || c.ability < 0 || c.ability >= len(c.tmpl.abilities) {
		return
	}
	a := &c.tmpl.abilities[c.ability]
	if a.Say != "" && npc.add == nil { // an add's lines are its creature's (summon.go)
		n := w.sayNearLocked(npc.Name, npc.World, npc.X, npc.Z, a.Say)
		log.Printf("Template: %s says %q (its %q) to %d players", npc.Name, a.Say, c.spell.Name, n)
	}
	if a.ProcChance <= 0 || a.ProcDamage <= 0 {
		return
	}
	if f := npc.tfight; f != nil && f.on {
		name := a.ProcName
		if name == "" {
			name = database.DefaultFightProcName
		}
		f.proc = &fightProc{chance: a.ProcChance, damage: npc.shared(a.ProcDamage), name: name} // an add's at its share (summon.go)
		log.Printf("Template: %s's %q gives it %s for the rest of its fight: %d%% of its melee hits %d more",
			npc.Name, c.spell.Name, name, a.ProcChance, a.ProcDamage)
	}
}

// sayNearLocked is a speaker's line in /say ("Name: text", as a say is sent) to every player within a
// say's reach (MetalSayHeard) of (x, z) in its world, and how many heard it. Must be called with the
// lock held.
func (w *World) sayNearLocked(speaker string, world uint8, x, z float32, text string) int {
	line := speaker + ": " + text
	n := 0
	for _, p := range w.Players {
		if p.World != world || distXZ(p.X, p.Z, x, z) > MetalSayHeard {
			continue
		}
		if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
			w.sendChatToSession(sess, line)
			n++
		}
	}
	return n
}

// abilityTimesWords is #stats' words for what an ability carries past a cast (templateLineLocked): its
// cooldown and when it comes again, its line in /say, the proc its landing gives; "" for none.
func abilityTimesWords(npc *NPC, a *templateAbility, now time.Time) string {
	var b strings.Builder
	if a.Cooldown > 0 {
		cd := time.Duration(a.Cooldown * float64(time.Second))
		if left := npc.abilityReady[abilityKey(&a.TemplateAbility)].Sub(now); left > 0 {
			fmt.Fprintf(&b, ", once in %v across fights (again in %v)", cd, left.Round(time.Second))
		} else {
			fmt.Fprintf(&b, ", once in %v across fights (ready)", cd)
		}
	}
	if a.Say != "" {
		fmt.Fprintf(&b, ", saying %q", a.Say)
	}
	if a.ProcChance > 0 && a.ProcDamage > 0 {
		name := a.ProcName
		if name == "" {
			name = database.DefaultFightProcName
		}
		fmt.Fprintf(&b, ", then %s for the fight: %d%% of its melee hits %d more", name, a.ProcChance, a.ProcDamage)
	}
	return b.String()
}
