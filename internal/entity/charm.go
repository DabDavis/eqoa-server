package entity

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// charm.go — the charms: the enchanter's (the Entrancing lines and their
// short versions, Binding Grasp), the bard's Entrancing Song, the druid's
// animal charms and the necromancer's undead ones: a creature made the
// caster's pet (F3 20261025-164922, a GM: "It should check if you currently
// have a pet. If you do, it should block your spellcast and do the fizzle
// spell animation briefly. If you do not, it should check the mobs level
// against the charms maximum level ... When charm hits, it should roll a
// resist check against the mob. Generally charm has a high success rate.
// When it succeeds ... the monster should become your pet with current
// existing stats. All pet stances should work as normal, pet attack ... When
// the charm pet dies, it should not drop its loot.").
//
// What the reference server sent (harvests/sandstorm-spells-20261021/pets/, an
// enchanter of 49's Entrancing Captivation; claude-notes/pets-charm-client-
// 20261021.md): on a charm, 0x007E with the creature's OWN id and 0x1003
// putting its name on the non-collide list, as a summoned pet's; the
// creature keeps its channel, its byte 142 turning 3 (player-side), 143 0
// and its npc flags 0x80 (the owner cannot attack it); it follows.
//
// The live game's rules, from eqoa.fandom.com (2026-11-02, the user's
// charm-pets.md and "Category:Mobs"; claude-notes/charm-20261102.md): its
// Charmable Mobs page (Kaleif's and KriztianBBB's lists), its Charming page,
// its List of Enchanter Spells and SOE's own Class Balancing notes of 2010.
//
//   - refused at its start, no power taken, no lockout: with a charmed
//     creature out, before its target is looked at, "You already have a
//     pet." in red and nothing else, as the reference server refuses any cast (one red
//     line: claude-notes/casting-sandstorm-20261021.md), the words the game's
//     own "You do not have a pet." (WORDS.TXT 1021) turned round (until F3
//     20261029-082049, a GM: "Charm line should not be castable while you
//     currently have a pet", it also showed a brief cast). A summoned pet out
//     is no refusal since petition 1348 ("when tring to charm a
//     pet with binding grasp, if i alrdy got swarming blades out tanking it
//     to charm, i get red message saying already got a pet"): the charm is
//     cast while the summoned pet holds the creature, and when it takes hold
//     the summoned pet goes (charmLandLocked: 0x007F, its non-collide entry
//     off, charmPetLeaves), so there is never a spell pet and a charmed one at
//     once (petition 1351: "Dont make it so they can summon their
//     spell pet and charm at the same time"); a charm resisted leaves it
//     there. Refused too at a
//     creature of a name npccharm makes uncharmable, "Your target cannot be
//     charmed." (ours: the live game showed a pop-up whose words no record
//     keeps; the Charmable Mobs page: Were-Hunter NPCs, raging rhinos, the
//     named raid mobs); at one above the charm's level, "Your target is too
//     high a level to charm."
//   - the charm's level (charmMaxLevel): SOE's notes ("Allure and Entrancing
//     Allure now charm up to 53."), else its words ("Charms up to level 58
//     mobs", Binding Grasp; the bard's "Charm up to level 54"), else two
//     below its own level, and three below for a druid's animal charm: the
//     wiki's "any creature that conned Green, Light Blue, or Dark Blue at
//     that spell's level was charmable" (dark blue is one or two below,
//     concolor.go), and SOE's two caps both sit at its lower edge (Allure, level
//     55: 53; Binding Grasp, level 60: 58); Allakhazam's comments put the
//     druid's at light blue (three below).
//   - at its landing a resist: training dummies always; else the creature's
//     own chance (npccharm's resist_pct: Ebony Recluse's "Low success rate",
//     Hand of Thule's "Very low", Vu'Malik's charm that "will not stick")
//     or charmResistBase, the arcane resist debuffs on it (the Paralytic
//     line, enchanter.go) taken off, never under charmResistFloor but where
//     it is 100. Undead are charmed as anything else (the Charmable Mobs
//     page's enchanters charm spectres, spirits, lost souls, ghouls and dead
//     riders; until 2026-11-02 every charm but the necromancer's failed on
//     them, from one charm the reference server saw resisted on a Rotting Flesh). The
//     necromancer's undead charms (Charm, Command, Control, Coerce and
//     Dominate Undead) are still refused at anything not undead (undead.go),
//     the druid's at anything not an animal (workbookmore.go). Resisted, it
//     fights the caster.
//   - taken, the creature is the caster's pet (pet.go) as it is: its level,
//     hit points and swing; it follows, takes the stances and attack, runs
//     to its master after any trip (the run, below). Its npccharm row's
//     template (classtemplates) is what it casts as a pet (pettemplate.go:
//     the Charmable Mobs page's nukes, heals, damage shields, taps and
//     crits); its spells cost it power (its pet's power, 28 a level), which
//     comes back as a player's does and with what its master gives it (the
//     page: "If you notice the tree not casting as much as usual, try
//     giving it some power").
//   - it holds for the line's time plus its CHA share of its caster's CHA in
//     seconds ("Increase 25% based on CHA": the wiki's "25% of your CHA is
//     added to the duration of the spell, meaning that 600 CHA would add an
//     extra 150 seconds"), and it is let go then, when it is dismissed (both
//     turn it on its master: "when the spell breaks the charmed NPC will
//     invariably attack the enchanter"), when its master dies, logs out or
//     summons a pet.
//   - the grace (charmFar): a creature let go past the zones next to the one
//     it spawned in (the 8 round it, the diagonals counted: the Charming
//     page's "one zone grace ... also counts as NE NW SE or SW of the zone
//     the mob is charmed in"), or in another world, goes home at once and
//     fights no one ("If a charm expires on an NPC that has been taken a
//     significant distance away from their spawn location, the NPC will
//     despawn immediately"). A zone is the world's 2000-unit square (CLIENT's
//     zone-name grid, the ground files' zones). The grace decides only where
//     a released creature ends up; it breaks no charm. One that dies as a pet
//     leaves a corpse no one may loot (OpenCorpse) and comes back itself.
//   - the check (charmOutOfReach; a GM, 2026-11-16, petition 2233: "its
//     an absolute distance check from player location to pet"): every tick,
//     whatever it is doing, a charmed creature farther than charmRunRange
//     (4,920: the Isle of Dread's middle to Klik'Anon's, plus 10%) from its
//     master, the player, or in another world, is let go there with "Your
//     charm spell has worn off." (petSystem; the Kerran's side animal and a
//     merc's charmed pet the same, measured from the player). On foot it
//     follows, so the check never trips.
//   - the run (a GM: "It should run to the enchanter at max sow speed in
//     a straight line and then when it hits a zone wall run through it"):
//     no trip breaks or carries a charm, a coach, a harbor's boat, a
//     waysender, Return Home, a gate or a rez alike. Inside charmRunRange,
//     a charmed pet more than petLost from its master runs to it
//     (charmRunStepLocked): straight, at Spirit of the Wolf's pace
//     (charmRunSpeed), through walls and terrain, on the ground's top at
//     each step, taking no target and not stopping when hit, until it is
//     petCatchUp off; then it follows. Its master travelling again past
//     charmRunRange from it lets it go then. A summoned pet still comes
//     along at once.

// charmHold is what a charm changed on a creature, to put back.
type charmHold struct {
	npcType int
	faction byte
	world   uint8
	until   time.Time // zero: until it is let go otherwise
	spell   string
	resists *ClassTemplate // wild elemental defenses retained for this charm
}

const (
	// charmNPCType is a charmed creature's npc flags: the reference server's charmed
	// ant's 0x80 (0x8000 on the wire, statesync's swap).
	charmNPCType = 0x80
	// charmFizzle is the brief cast a refusal that fizzles shows: an
	// undead-only spell at the living (undead.go), a rez with nothing to give
	// back (resurrect.go). A charm refused shows none.
	charmFizzle = 600 * time.Millisecond
	// charmResistBase is a creature's chance in percent to resist a charm
	// (ours: a GM's "Generally charm has a high success rate"; the Charming
	// page's 46/47 rhinos "ALWAYS ... on the first try ... Even with base
	// cha"), charmResistFloor the least the arcane debuffs take it to.
	charmResistBase  = 10
	charmResistFloor = 2
	// charmCapBelow is how far under its own level a charm's cap is, and
	// animalCharmCapBelow a druid's animal charm's (the file's doc).
	charmCapBelow       = 2
	animalCharmCapBelow = 3
	// charmZone is a zone's side: the world's squares (the grace, charmFar).
	charmZone = 2000
	// charmRunRange is the farthest a charmed creature may be from its master
	// (the player) and keep its charm, checked every tick: the Isle of Dread
	// zone's middle (25000, 3000) to Klik'Anon's (23000, 7000), 4472.1, plus
	// 10% (2026-11-16), rounded up.
	charmRunRange float32 = 4920
)

// charmRunSpeed is a charmed creature's pace when it runs to its master:
// Spirit of the Wolf's, the movement rate's most (100).
var charmRunSpeed = RunSpeed * speedBuff // 11.483

// soeCharmCaps are the caps SOE's own notes give by the spell's name (lower
// case): the Class Balancing page of 2010's "Allure and Entrancing Allure now
// charm up to 53." (the wiki's List of Enchanter Spells too: Allure, 53),
// over Entrancing Allure's older words ("Charms up to level 54 mobs").
var soeCharmCaps = map[string]int{"allure": 53, "entrancing allure": 53}

const (
	charmHavePet  = "You already have a pet."
	charmCannot   = "Your target cannot be charmed." // ours: the live game's pop-up is not recorded
	charmTooHigh  = "Your target is too high a level to charm."
	charmTooLow   = "Your target is too low a level for this charm." // ours: a charm with a lowest level (CharmMin)
	charmResisted = "Attempt resisted!"                              // the reference server's
	charmWornOff  = "Your charm spell has worn off."
	// charmPetLeaves follows a summoned pet's name when it goes as a charm
	// takes hold (petition 1348; ours)
	charmPetLeaves = " departs as your charm takes hold."
)

// charmIntn is the charm's dice (a test's own).
var charmIntn = rand.Intn

// charmMaxLevel is the highest level the charm takes (the file's doc): SOE's
// notes, else its words, else its level less charmCapBelow (a druid's
// animal charm animalCharmCapBelow), at least 1.
func charmMaxLevel(spell *CastSpell) int {
	if n, ok := soeCharmCaps[strings.ToLower(strings.TrimSpace(spell.Name))]; ok {
		return n
	}
	if spell.Def.CharmMax > 0 {
		return spell.Def.CharmMax
	}
	below := charmCapBelow
	if spell.Def.TargetKind == "animal" {
		below = animalCharmCapBelow
	}
	return max(spell.Level-below, 1)
}

// charmEffect is the spell's charm effect (its CHA share).
func charmEffect(d *SpellDef) SpellEffect {
	for _, e := range d.Effects {
		if e.Kind == EffectCharm {
			return e
		}
	}
	return SpellEffect{Kind: EffectCharm}
}

// charmDuration is how long the charm holds on a caster with that CHA: its
// line's time plus its CHA share of the CHA in seconds (25%: a quarter of a
// second a point); 0 for a line that gives none (until it is let go).
func charmDuration(spell *CastSpell, cha int) time.Duration {
	d := spell.Def.Duration
	if d <= 0 {
		return 0
	}
	if e := charmEffect(&spell.Def); e.Scale == StatCha && e.ScalePct > 0 {
		d += time.Duration(e.ScalePct*max(cha, 0)) * time.Second / 100
	}
	return d
}

// charmResistLocked is the chance in percent that the creature resists a
// charm now: its npccharm row's, else charmResistBase, less the arcane
// resist debuffs on it, never under charmResistFloor; 100 (a charm that
// never holds on it) whatever is on it. Must be called with the lock held.
func (w *World) charmResistLocked(npc *NPC, now time.Time) int {
	pct := charmResistBase
	if r, ok := w.charmRuleLocked(npc); ok && r.ResistPct > 0 {
		pct = r.ResistPct
	}
	if pct >= 100 {
		return 100
	}
	return max(pct-npcResistOffPct(npc, now), charmResistFloor)
}

// charmPetRefusalLocked is the line refusing a charm its caster casts with a
// charmed creature out ("" for any other spell, no pet, or a summoned one,
// which goes when the charm takes hold: petition 1348, charmLandLocked; nor
// for a stacking charm, or a charmed pet one took: stackcharm.go): the first
// of a cast's checks, so whatever is targeted the charm is not cast (F3
// 20261029-082049). Must be called with the lock held.
func (w *World) charmPetRefusalLocked(p *Player, spell *CastSpell) string {
	if w.charmBlockedLocked(p, spell) {
		return charmHavePet
	}
	return ""
}

// charmStartLocked checks a charm as it begins, its target found (the pet
// is charmPetRefusalLocked's): the refusal's line, "" for none; no charm's
// refusal fizzles. Must be called with the lock held.
func (w *World) charmStartLocked(spell *CastSpell, targets []EntityID) string {
	for _, id := range targets {
		npc := w.NPCs[id]
		if npc == nil {
			continue
		}
		if r, ok := w.charmRuleLocked(npc); ok && !r.Charmable {
			return charmCannot
		}
		if npc.Level > charmMaxLevel(spell) {
			return charmTooHigh
		}
		if spell.Def.CharmMin > 0 && npc.Level < spell.Def.CharmMin {
			return charmTooLow
		}
	}
	return ""
}

// charmLandLocked is a charm landing on its target: resisted (the line, and
// the creature at the caster) or taken. Must be called with the lock held.
func (w *World) charmLandLocked(cs *castState, caster *Player, sess *session.Session, npc *NPC, now time.Time) {
	if npc.Dead || npc.Owner != 0 {
		return
	}
	if w.charmBlockedLocked(caster, &cs.spell) {
		// a charmed one came while it was cast
		if sess != nil {
			w.sendColoredLocked(sess, charmHavePet, castRefusalRGB)
		}
		return
	}
	// undead are charmed as anything else; the necromancer's undead line
	// takes nothing else (undead.go: refused at its start). An arcane resist
	// debuff on it (the Paralytic line) lowers the chance (enchanter.go)
	resist := w.charmResistLocked(npc, now)
	if npc.Dummy != NotDummy || charmIntn(100) < resist {
		if sess != nil {
			w.sendColoredLocked(sess, charmResisted, castRefusalRGB)
		}
		caster.LastCombatTime = now
		// hate enough to make the caster its enemy (none counts as none),
		// as the reference server's resisting Rotting Flesh came at its caster
		w.updateAggro(npc, caster.ServerID, 1)
		log.Printf("Charm: %s's %q resisted by %s (%d%%)", caster.Name, cs.spell.Name, npc.Name, resist)
		return
	}
	if w.stackingCharmLocked(&cs.spell) {
		w.stackCharmLandLocked(caster, sess, npc, &cs.spell, now) // beside the pet out (stackcharm.go)
		return
	}
	if old := w.pets[caster.ServerID]; old != nil && old.npc == npc {
		// its own pet, its charm gone from under it (a GM's NPC reload remade
		// it from its row): taken again, not killed as a summoned pet goes and
		// then charmed dead (Heirloom petition 2555)
		delete(w.pets, caster.ServerID)
	} else if old != nil && w.sideStepLocked(old, caster, sess) {
		// a stacking charm's animal stays, beside the new one (stackcharm.go)
	} else if old != nil {
		// one pet at a time (petition 1351): the summoned pet that held the
		// creature goes as the charm takes hold (petition 1348), its 0x007F
		// before the creature's 0x007E so the bar takes the new name; one
		// that died lies its corpse's time, its owner told nothing more; an
		// earlier login's goes untold, as petSystem lets it go
		tell, line := sess, ""
		switch {
		case old.owner != caster:
			tell = nil
		case !old.npc.Dead:
			line = strings.TrimSpace(old.npc.Name) + charmPetLeaves
			log.Printf("Charm: %s's %q takes the place of its %q", caster.Name, cs.spell.Name, old.npc.Name)
		}
		w.dismissPetLocked(old, tell, line, now)
	}
	if npc.Dead {
		return // a charm never takes hold of a dead creature (petition 2555)
	}
	w.charmLocked(caster, sess, npc, &cs.spell, now)
}

// charmLocked makes the creature the caster's pet, as it is, fighting by its
// npccharm row's template (pettemplate.go) when the world has it. Must be
// called with the lock held.
func (w *World) charmLocked(caster *Player, sess *session.Session, npc *NPC, spell *CastSpell, now time.Time) {
	w.holdCharmLocked(caster, sess, npc, newCharmHold(caster, npc, spell, now), now)
}

// holdCharmLocked makes the creature the caster's pet under hold: a charm's
// landing (charmLocked) or one a restart carried (keeppet.go). Must be called
// with the lock held.
func (w *World) holdCharmLocked(caster *Player, sess *session.Session, npc *NPC, hold *charmHold, now time.Time) {
	w.takeCharmedLocked(caster, npc, hold)
	if w.pets == nil {
		w.pets = make(map[EntityID]*petState)
	}
	ps := &petState{owner: caster, npc: npc, stance: petDefensive, last: now}
	ps.tmpl = w.charmTemplateLocked(npc)
	w.pets[caster.ServerID] = ps
	w.duelNewPetLocked(caster.ServerID) // in its duel again, if the old one yielded (duel.go)
	if sess != nil {
		w.announcePetLocked(ps, sess)
	}
	lasts := "until it is let go"
	if !hold.until.IsZero() {
		lasts = "for " + hold.until.Sub(now).String()
	}
	fights := "as itself"
	if ps.tmpl != nil {
		fights = "by template " + ps.tmpl.tmpl.Name
	}
	log.Printf("Charm: %s's %q takes %s (level %d, %d/%d HP, power %d), %s, %s", caster.Name, hold.spell, npc.Name, npc.Level,
		npc.HP, npc.MaxHP, npc.Power, lasts, fights)
}

// takeCharmedLocked is what a charm changes on the creature it takes for
// owner (a player's pet, or a merc's: mercharm.go): its hold, its owner, its
// flags, a pet's power, out of every fight and its roaming. Must be called
// with the lock held.
func (w *World) takeCharmedLocked(owner *Player, npc *NPC, hold *charmHold) {
	npc.Charm = hold
	hold.resists = w.charmResistsLocked(npc)
	npc.Owner = owner.ServerID
	npc.NPCType = charmNPCType
	petPowerLocked(npc) // a pet's power, as a summoned one's (npcregen.go)
	npc.Threats, npc.CurrentEnemy = nil, 0
	npc.endRound()
	npc.endTemplateFight() // its enemy template's clocks (classtemplate.go)
	w.wakeNPCLocked(npc)   // a pet's regeneration (npcsets.go)
	npc.RoundTarget = 0
	npc.stopMoving()
	npc.roam = roamState{}
	if owner.AttackTarget == npc.ServerID {
		owner.disengage()
	}
}

// Ebony Recluse keeps its wild elemental resists while charmed. Its attacks
// and hit points continue to follow the ordinary charm rules.
func (w *World) charmResistsLocked(npc *NPC) *ClassTemplate {
	if database.NPCClassKey(npc.Name) == "ebony recluse" {
		return w.namedTemplateLocked(npc)
	}
	return nil
}

func (n *NPC) charmResists() *ClassTemplate {
	if n == nil || n.Charm == nil {
		return nil
	}
	return n.Charm.resists
}

// charmTemplateLocked is the pet template a charmed creature fights by: its
// npccharm row's class template as a pet's row (its own swing and class kept:
// "current existing stats"), nil for none. Must be called with the lock held.
func (w *World) charmTemplateLocked(npc *NPC) *petTemplate {
	r, ok := w.charmRuleLocked(npc)
	if !ok || r.Template == "" {
		return nil
	}
	t := w.templates.byName[r.Template]
	if t == nil {
		return nil
	}
	return &petTemplate{PetTemplate: database.PetTemplate{Template: t.Name, SwingClass: -1, Swing: -1}, tmpl: t}
}

// charmFar reports whether a charmed creature of its own world home, at x, z
// in world, is past its grace: in another world, or beyond the zones next to
// the one it spawned in (the file's doc).
func charmFar(npc *NPC, home, world uint8, x, z float32) bool {
	if world != home {
		return true
	}
	zone := func(v float32) int { return int(math.Floor(float64(v) / charmZone)) }
	dx, dz := zone(x)-zone(npc.SpawnX), zone(z)-zone(npc.SpawnZ)
	return dx > 1 || dx < -1 || dz > 1 || dz < -1
}

// releaseCharmLocked lets a charmed pet go: off its master's pet bar
// (0x007F, its non-collide entry off) with the line, the creature its own
// again (unCharmLocked), sent home if it is past its grace (charmFar), and,
// hostile and still near, turned on its master. A dead one is let go at once,
// its corpse a creature's of nothing (no one's to loot) that comes back home
// (charmDied, charmHome). sess is the
// master's, or nil. Must be called with the lock held.
func (w *World) releaseCharmLocked(ps *petState, sess *session.Session, text string, hostile bool) {
	npc, owner := ps.npc, ps.owner
	if w.pets[owner.ServerID] == ps {
		delete(w.pets, owner.ServerID)
	}
	if w.sidePets[owner.ServerID] == ps {
		delete(w.sidePets, owner.ServerID) // a stacking charm's animal (stackcharm.go)
	}
	ps.target = 0
	npc.endRound()
	npc.RoundTarget = 0
	npc.stopMoving()
	if sess != nil {
		if !ps.side {
			sendClearPet(sess) // the animal beside the pet is not on the bar (stackcharm.go)
		}
		if ps.noCollide {
			sendNoCollideRemove(sess, ps.noCollideID())
			ps.noCollide = false
		}
		if text != "" {
			w.sendChatToSession(sess, text)
		}
	}
	if npc.Dead {
		// its death ends the charm at once (the owner, 2026-11-14: "a GM has
		// charmed 2 dead crocodiles and they stayed charmed"): until then a
		// dead one kept its hold, its owner and the charmed flags until it came
		// back, so its corpse lay as its master's charmed pet on every client.
		// Now it is a creature's corpse again, of nothing still (its master's
		// kill: no loot, CorpseOfNothing), and comes back on its own spawn in
		// its own world (respawnNPCLocked: charmHome).
		home := w.unCharmLocked(npc)
		npc.charmDied, npc.charmHome = true, home
		log.Printf("Charm: %s's charmed %q died: the charm ends (its corpse a creature's, of nothing)", owner.Name, npc.Name)
		return
	}
	home := w.unCharmLocked(npc)
	// it keeps what was cast on it until each runs out (the user,
	// 2026-11-01: "is charm breaks/release the mob retains all buffs until
	// they expire"): its maximum's raise (pet.go) and its heals over time,
	// on the creatures' effects from now on (effectSystem), and in its fights
	// as every creature's effects count ("make the released mob's buffs
	// count in its fights too": creatureMods, pet.go)
	if len(npc.Effects) > 0 {
		if w.affected == nil {
			w.affected = make(map[EntityID]*NPC)
		}
		w.affected[npc.ServerID] = npc
	}
	popped := charmFar(npc, home, npc.World, npc.X, npc.Z)
	if popped {
		// past its grace: home at once, as it was found (the file's doc)
		oldX, oldZ := npc.X, npc.Z
		npc.World = home
		npc.X, npc.Y, npc.Z, npc.Facing = npc.SpawnX, npc.SpawnY, npc.SpawnZ, npc.SpawnFacing
		w.SpatialIndex.Update(npc.ServerID, oldX, oldZ, npc.X, npc.Z)
		hostile = false
	}
	if hostile && !owner.IsDead() && w.Players[owner.ServerID] == owner {
		w.updateAggro(npc, owner.ServerID, 1)
	}
	log.Printf("Charm: %s's %q goes free (hostile %v, sent home %v)", owner.Name, npc.Name, hostile, popped)
}

// charmOutOfReach reports a charmed pet past its master's reach (the file's
// doc): the player in another world than the pet, or farther than
// charmRunRange.
func charmOutOfReach(pet *NPC, p *Player) bool {
	return p.World != pet.World || distXZ(pet.X, pet.Z, p.X, p.Z) > charmRunRange
}

// charmRunStepLocked is one step of a charmed creature's run to its master
// (the file's doc): straight toward (x, z) at charmRunSpeed, never past it,
// through walls, quest walls, water's edges and terrain alike (no route, no
// stepNPCToward refusals), on the top of the terrain at its place (over the
// sea, the bed), else at y (its master's). Must be called with the lock held.
func (w *World) charmRunStepLocked(npc *NPC, x, y, z, dt float32) {
	dx, dz := x-npc.X, z-npc.Z
	d := float32(math.Sqrt(float64(dx*dx + dz*dz)))
	if d <= 0 {
		return
	}
	ux, uz := dx/d, dz/d
	step := min(charmRunSpeed*dt, d)
	oldX, oldZ := npc.X, npc.Z
	npc.X, npc.Z = npc.X+ux*step, npc.Z+uz*step
	if step == d {
		npc.X, npc.Z = x, z
	}
	npc.Y = y
	if w.ground != nil {
		if fy, ok := w.ground.Floor(npc.World, npc.X, 1e6, npc.Z); ok {
			npc.Y = fy
		}
	}
	npc.Facing = headingTo(dx, dz)
	npc.VelX, npc.VelY, npc.VelZ = ux*charmRunSpeed, 0, uz*charmRunSpeed
	npc.Loco = LocoRun
	if w.SpatialIndex != nil {
		w.SpatialIndex.Update(npc.ServerID, oldX, oldZ, npc.X, npc.Z)
	}
}

// charmRunTurnLocked is a running charmed pet's turn (ps.running: the file's
// doc), toward its master at (x, y, z): no target, no round, one step of its
// run, and its run over once it is petCatchUp off (its follow from the next
// tick). It reports whether the turn was the run's. Must be called with the
// lock held.
func (w *World) charmRunTurnLocked(ps *petState, x, y, z, dt float32) bool {
	if !ps.running {
		return false
	}
	pet := ps.npc
	ps.target = 0
	pet.endRound()
	w.charmRunStepLocked(pet, x, y, z, dt)
	if distXZ(pet.X, pet.Z, x, z) <= petCatchUp {
		ps.running = false
	}
	return true
}

// unCharmLocked puts back what a charm changed on the creature and returns
// its own world (its present one when no charm held it), its enemy template
// as its name has it now (a GM's reload while it was a pet left it none:
// classtemplate.go).
func (w *World) unCharmLocked(npc *NPC) uint8 {
	h := npc.Charm
	if h == nil {
		return npc.World
	}
	npc.Owner = 0
	npc.NPCType = h.npcType
	npc.Faction = h.faction
	npc.Charm = nil
	w.wakeNPCLocked(npc)           // a creature again: its fights and its regeneration (npcsets.go)
	npc.Power, npc.MaxPower = 0, 0 // a creature again: no power (npcregen.go)
	npc.endTemplateFight()         // the pet template's clocks (pettemplate.go)
	w.applyTemplateLocked(npc)
	// its hit points as they stand, never over its own maximum (its
	// template's and its kept effects': applyTemplateLocked, syncEffectMaxHP)
	if npc.MaxHP > 0 && npc.HP > npc.MaxHP {
		npc.HP = npc.MaxHP
	}
	return h.world
}

// SetCharmRules sets the GMs' charm rules (table npccharm), keyed as
// database.NPCClassKey keys a name.
func (w *World) SetCharmRules(rules map[string]database.CharmRule) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.charmRules = rules
}

// LoadCharmRules reads table npccharm into the world: at the start and after
// a GM's edit. Call it without the database held. It reports how many names
// it read.
func (w *World) LoadCharmRules(db *database.SQLiteDB) (int, error) {
	rules, err := db.NPCCharmRules()
	if err != nil {
		return 0, fmt.Errorf("npccharm: %w", err)
	}
	w.SetCharmRules(rules)
	return len(rules), nil
}

// charmRuleLocked is the creature's npccharm row, by its name. Must be called
// with the lock held.
func (w *World) charmRuleLocked(npc *NPC) (database.CharmRule, bool) {
	r, ok := w.charmRules[database.NPCClassKey(npc.Name)]
	return r, ok
}

// CharmSummary says what the charm rules hold, for the log at the start (and
// what of them the world cannot use: a template it lacks).
func (w *World) CharmSummary() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	no, resist, tmpl := 0, 0, 0
	var missing []string
	for name, r := range w.charmRules {
		if !r.Charmable {
			no++
		}
		if r.ResistPct > 0 {
			resist++
		}
		if r.Template != "" {
			tmpl++
			if w.templates.byName[r.Template] == nil {
				missing = append(missing, name+" ("+r.Template+")")
			}
		}
	}
	line := fmt.Sprintf("charm: %d names in npccharm (%d uncharmable, %d with a resist of their own, %d fighting by a charm template)",
		len(w.charmRules), no, resist, tmpl)
	if len(missing) > 0 {
		sort.Strings(missing)
		line += fmt.Sprintf("; templates not found, so those fight as themselves: %s", strings.Join(missing, ", "))
	}
	return line
}
