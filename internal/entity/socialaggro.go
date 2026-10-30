package entity

import (
	"fmt"
	"log"
	"time"

	"github.com/eqoa/server/internal/database"
)

// NPCs coming to the help of their own (F3 20261025-215440, jib: "When a mob
// is attacked and it is on the same faction as another mob, IF it is social,
// it should assist the attacked monster. All guards of a town qualify for
// this, but not all mobs are social so we will have to specify as we go. The
// social radius should be about spell casting distance ... It should only
// trigger when you do damage or an aggressive act to the guard - so if you
// have existing agro on a monster on the same faction, run by another, the
// other wont assist unless they see you hit them.")
//
// When a player, or a player's pet, hits an NPC or does it another hostile act
// (updateAggro: a melee hit, a spell's damage or its landing, a hate clause, a
// resisted charm, each damage tick), every social NPC of the same faction
// standing within its social radius of it, in its world, joins the fight
// against the attacker as a hit of its own would start one (addThreatLocked:
// the chase, the leash and the way home are the fight's). Every hit calls them
// again, so one that walks up late joins at the next hit it is near; a player
// who only runs past them with a fight on is left alone.
//
// What is social, and which faction: table npcsocial, by NPC name (the GM
// panel's "Social NPCs", live): social 1 and its faction (any name: a faction
// of the factions table or a group of the GM's own). An attacked NPC is of its
// own row's faction and of every faction its kill lowers (factionhits), so only
// the NPCs that help need a row; one with none helps no one.
// tools/seed_npc_social.py writes the towns' guards, each town's under its
// guard faction (The Freeport Militia, The Highpass Guard, The Qeynos Guard)
// or, where the town has none, "Guards of <town>": guards help guards, not
// townsfolk.
//
// Chosen, as no capture holds a creature helping another:
//   - "sight" is a radius in the same world: the server holds no walls, only
//     the terrain's height, so a guard behind a house sees a hit too;
//   - the radius: 30 unless its row says, the range of 176 of the server's 402
//     spells (spellpattern.range's 0x41F0, 30.0) and AggroRadius's own;
//   - a helper already fighting, on its way home after a fight, dead, hidden,
//     a pet or a charmed creature, a dummy, or with the NPC bit (0x80: it
//     attacks no one, aggro.go) does not come; nor does anyone for a pet;
//   - a pet's hit sends them at its owner: a creature does not run after a pet
//     (npcFightPetLocked lets one 10 off go), so one sent at a pet 25 off would
//     drop it at once and help no one;
//   - their joining calls no one else: the next hit on one of them does;
//   - a GM's #aggro off counts for nothing here: the GM attacked.

// DefaultSocialRadius is how near an attacked NPC a social one must stand to
// come to its help when its row says nothing: a spell's usual range.
const DefaultSocialRadius float32 = 30

// SocialRule is a GM's word on the NPCs of one name (table npcsocial):
// whether they come to the help of their faction, the faction they are of,
// and how near (0: DefaultSocialRadius).
type SocialRule struct {
	Social  bool
	Faction string
	Radius  float32
}

// npcSocialState is the world's social rules by NPC name (database.NPCClassKey:
// any case, as npcaggro's) and the farthest any helper comes from.
type npcSocialState struct {
	rules map[string]SocialRule
	reach float32
}

// ruleFor is the rule for the NPCs of a name, in any case.
func (s *npcSocialState) ruleFor(name string) (SocialRule, bool) {
	r, ok := s.rules[database.NPCClassKey(name)]
	return r, ok
}

// socialFactionLocked is the faction an NPC helps: its row's, else the one
// its kill lowers most (factionhits; the first of equals); "" none. Must be
// called with the lock held.
func (w *World) socialFactionLocked(npc *NPC) string {
	if r, ok := w.npcSocial.ruleFor(npc.Name); ok && r.Faction != "" {
		return r.Faction
	}
	best, faction := 0, ""
	for _, h := range w.factions.hitsFor(npc.Name) {
		if h.Delta < best {
			best, faction = h.Delta, h.Faction
		}
	}
	return faction
}

// ofFactionLocked reports an NPC of the faction for the social pull: its
// row's, or any its kill lowers (factionhits). Must be called with the lock
// held.
func (w *World) ofFactionLocked(npc *NPC, faction string) bool {
	if r, ok := w.npcSocial.ruleFor(npc.Name); ok && r.Faction == faction {
		return true
	}
	for _, h := range w.factions.hitsFor(npc.Name) {
		if h.Delta < 0 && h.Faction == faction {
			return true
		}
	}
	return false
}

// socialEnemyLocked is whom the NPCs that help go for after a hit by
// attackerID: a player in the world, or a pet's owner (the file's doc); 0
// for anyone else. Must be called with the lock held.
func (w *World) socialEnemyLocked(attackerID EntityID) EntityID {
	if p, ok := w.Players[attackerID]; ok {
		if p.IsDead() {
			return 0
		}
		return attackerID
	}
	if pet, ok := w.NPCs[attackerID]; ok && pet.Owner != 0 {
		if p, ok := w.Players[pet.Owner]; ok && !p.IsDead() {
			return pet.Owner
		}
	}
	return 0
}

// helperLocked reports a social NPC free to come to victim's help: the
// file's doc's exclusions, its faction victim's, and victim within its
// radius. Must be called with the lock held.
func (w *World) helperLocked(helper, victim *NPC, now time.Time) bool {
	r, ok := w.npcSocial.ruleFor(helper.Name)
	if !ok || !r.Social || helper == victim {
		return false
	}
	if helper.Dead || helper.Owner != 0 || helper.Dummy != NotDummy || helper.NPCType&0x80 != 0 ||
		helper.World != victim.World || helper.CurrentEnemy != 0 || helper.chase.returning ||
		now.Before(helper.HiddenUntil) {
		return false
	}
	faction := w.socialFactionLocked(helper)
	if faction == "" || !w.ofFactionLocked(victim, faction) {
		return false
	}
	radius := r.Radius
	if radius <= 0 {
		radius = DefaultSocialRadius
	}
	dx, dy, dz := helper.X-victim.X, helper.Y-victim.Y, helper.Z-victim.Z
	return dx*dx+dy*dy+dz*dz <= radius*radius
}

// socialAssistLocked is a hit on victim by attackerID seen by the social NPCs
// of its faction near it: each free one joins the fight (the file's doc). Must
// be called with the lock held.
func (w *World) socialAssistLocked(victim *NPC, attackerID EntityID) {
	if len(w.npcSocial.rules) == 0 || w.SpatialIndex == nil || victim.Owner != 0 || victim.Dummy != NotDummy {
		return
	}
	enemy := w.socialEnemyLocked(attackerID)
	if enemy == 0 {
		return
	}
	now := time.Now()
	for _, id := range w.SpatialIndex.QueryRadius(victim.X, victim.Z, w.npcSocial.reach) {
		helper, ok := w.NPCs[id]
		if !ok || !w.helperLocked(helper, victim, now) {
			continue
		}
		w.addThreatLocked(helper, enemy, 1)
		log.Printf("Social: %s comes to the help of %s against %s (%.1f off, %s)",
			helper.Name, victim.Name, w.entityNameLocked(enemy),
			dist3(helper.X, helper.Y, helper.Z, victim.X, victim.Y, victim.Z), w.socialFactionLocked(helper))
	}
}

// entityNameLocked is a player's or an NPC's name, for the log.
func (w *World) entityNameLocked(id EntityID) string {
	if p, ok := w.Players[id]; ok {
		return p.Name
	}
	if n, ok := w.NPCs[id]; ok {
		return n.Name
	}
	return fmt.Sprintf("entity %d", id)
}

// SetSocialRules sets the GMs' rules by NPC name (table npcsocial): at the
// start and when a GM edits one. A rule is for its name in any case; of two
// spellings' rules the one that never helps wins, as npcaggro's peaceful one
// does (aggro.go).
func (w *World) SetSocialRules(rules map[string]SocialRule) {
	w.mu.Lock()
	defer w.mu.Unlock()
	keyed := make(map[string]SocialRule, len(rules))
	for name, r := range rules {
		k := database.NPCClassKey(name)
		if o, dup := keyed[k]; dup && !o.Social && r.Social {
			continue
		}
		keyed[k] = r
	}
	w.npcSocial.rules = keyed
	w.npcSocial.reach = DefaultSocialRadius
	for _, r := range keyed {
		if r.Social && r.Radius > w.npcSocial.reach {
			w.npcSocial.reach = r.Radius
		}
	}
}

// LoadSocialRules reads table npcsocial (database.NPCSocialRules) into the
// world: at the start and after a GM's edit. Call it without the database
// held. It reports how many NPC names help.
func (w *World) LoadSocialRules(db *database.SQLiteDB) (int, error) {
	rows, err := db.NPCSocialRules()
	if err != nil {
		return 0, fmt.Errorf("npcsocial: %w", err)
	}
	rules := make(map[string]SocialRule, len(rows))
	social := 0
	for _, r := range rows {
		rules[r.Name] = SocialRule{Social: r.Social, Faction: r.Faction, Radius: float32(r.Radius)}
		if r.Social {
			social++
		}
	}
	w.SetSocialRules(rules)
	return social, nil
}

// SocialRuleFor is the GMs' rule for the NPCs of a name, if there is one.
func (w *World) SocialRuleFor(name string) (SocialRule, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.npcSocial.ruleFor(name)
}

// SocialSummary is a line for the server's start: how many NPC names come to
// the help of their faction.
func (w *World) SocialSummary() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	n := 0
	for _, r := range w.npcSocial.rules {
		if r.Social {
			n++
		}
	}
	if n == 0 {
		return "no NPC comes to another's help (npcsocial has no social row)"
	}
	return fmt.Sprintf("%d NPC names come to the help of their faction within %.0f (npcsocial)", n, DefaultSocialRadius)
}
