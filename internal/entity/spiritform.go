package entity

import (
	"log"
	"regexp"
	"strconv"
	"time"
)

// spiritform.go — Spiritform: its bearer untouchable, its hit points held at
// one (the owner's ruling of 2026-10-29: "check for archives").
//
// Its line, EQOA-Data's as spelleffect holds it: "Self, Set Maximum HP to 0,
// Invulnerable, Form, 20sec", its description empty; until now its first two
// clauses were read nowhere, and the spell did nothing but its look. The
// game's own record, as Allakhazam's import of 2004-04-28 has it (archived):
// "Set Maximum HP to 0, Invulnerable, Sprite Change", Self Only, 20 seconds.
// The players' comments on that page: it is the effect of a stone from the
// Bone Sentinel (in "EG"), it makes its bearer invulnerable for some 12 to 15
// seconds with the hit points shown 1/1, and after it the hit points are 1
// with the normal maximum back. No recording of it exists.
//
// The words (a line's clause, any target word before it):
//   - "Set Maximum HP to N", "Set Max HP to N", "Set MaxHP to N" (or "Hit
//     Points", "Hitpoints", "Health"): a ceiling on the bearer's maximum hit
//     points while it lasts, as powercap.go's "Set MaxPower to N" is on its
//     power: the maximum at most N, the lowest of two such effects, the hit
//     points held under it (effectsChangedLocked), and the maximum its own
//     again at its end, the hit points where they were left (1 after
//     Spiritform), to regenerate. N of 0 reads as 1 (ours: the comments' 1/1;
//     a maximum of 0 would be a dead player), so the clause never kills. A
//     spell that lasts no time takes the hit points down to it at once, as a
//     power ceiling lasting no time does (no line has one);
//   - "Invulnerable", "Invulnerability": while it lasts its bearer, a player
//     or a pet, takes nothing of a creature's hit: a creature's melee
//     (npcAttackPlayer, the training and sparring dummies', a creature's on a
//     pet) and a creature's spell (the sparring dummy's strike, the one
//     creature spell here: no creature casts). The hit's 0x00DB is 0, and a
//     damage shield answers it as ever (dshield.go). It is looked at before a
//     ward (ward.go), which then has nothing to cut. Nothing else is stopped
//     (ours): drowning, a self line's or a pact's price (lifetap.go), a
//     potion's, a GM's KILL.
//
// Neither is harmful; both land as a buff on whom the line's target word and
// its record's scope say, and are kept over a logout (buffsave.go). A pet's
// maximum hit points are its own (no pet line sets one): the ceiling is a
// player's. Spiritform's "Form" is its look (illusion.go readAppearance): no
// body for it is known.

var (
	reHPCap        = regexp.MustCompile(`^(?:set\s+)?max(?:imum)?\s*(?:hp|hit\s*points|health)\s+to\s+(\d+)$`)
	reInvulnerable = regexp.MustCompile(`^invulnerab(?:le|ility)$`)
)

// spiritformPart reads a clause of the two: a ceiling on the maximum hit
// points (Amount, at least 1) or invulnerability.
func spiritformPart(part string) (SpellEffect, bool) {
	if reInvulnerable.MatchString(part) {
		return SpellEffect{Kind: EffectInvuln}, true
	}
	m := reHPCap.FindStringSubmatch(part)
	if m == nil {
		return SpellEffect{}, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return SpellEffect{}, false
	}
	n = max(n, 1)
	return SpellEffect{Kind: EffectHPCap, Amount: n, AmountMax: n}, true
}

// hpCap is the lowest ceiling the player's effects put on its maximum hit
// points (1 at least), and whether any does.
func (p *Player) hpCap() (int, bool) {
	top, capped := 0, false
	for _, e := range p.Effects {
		if e.HPCapped && (!capped || e.HPCap < top) {
			top, capped = e.HPCap, true
		}
	}
	return max(top, 1), capped
}

// invulnerableOf is the effect that makes its bearer invulnerable now, nil
// for none.
func invulnerableOf(effects []*ActiveEffect, now time.Time) *ActiveEffect {
	for _, e := range effects {
		if e.Invulnerable && now.Before(e.Until) {
			return e
		}
	}
	return nil
}

// playerInvulnerableLocked is a creature's hit of n on a player after the
// player's invulnerability: nothing under one, else n.
func (w *World) playerInvulnerableLocked(p *Player, npc *NPC, n int, now time.Time) int {
	if n <= 0 {
		return n
	}
	ae := invulnerableOf(p.Effects, now)
	if ae == nil {
		return n
	}
	log.Printf("Invulnerable: %s's %q takes nothing of %s's hit of %d", p.Name, ae.Name, npc.Name, n)
	return 0
}

// petInvulnerableLocked is a creature's hit of n on a pet after the pet's
// invulnerability, as a player's (playerInvulnerableLocked).
func (w *World) petInvulnerableLocked(pet, npc *NPC, n int, now time.Time) int {
	if n <= 0 {
		return n
	}
	ae := invulnerableOf(pet.Effects, now)
	if ae == nil {
		return n
	}
	log.Printf("Invulnerable: the pet %q's %q takes nothing of %s's hit of %d", pet.Name, ae.Name, npc.Name, n)
	return 0
}
