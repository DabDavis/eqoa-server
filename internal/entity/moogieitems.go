package entity

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// moogieitems.go — the Metal Moogie chain's rewards that do something when used (work board #688, the GM
// Jibrish's): the Snowball, the Potion of Moogie's Insight and the Deputy's Draught. Each is an itemeffect row
// whose line names it (NewPotionEffect reads the words, readMoogieUse): handlers/potion.go's Use takes it as
// any potion's (its stack's 0x003C, a charge, its row's cooldown), and UsePotion does it here.
//
//   - "Target, Snowball 0x15C66F59": thrown at another player, alive, in the thrower's world and within
//     SnowballRange; anything else is refused in red (SnowballRefusal), nothing used. A 0x00B1 from the thrower
//     to the target, cast 0, the line's visual (Frost Shot's projectile unless a GM writes another), then 1 off
//     the target's hit points, told in its 0x00DB, never below 1. No PvP act: no flag, no hate, no duel and no
//     Mr. Moogie lock; it breaks no invisibility and works in towns. Each told a line.
//     It is a thrown weapon too (petition 2204, Hoofbite, Heirloom: "Moogie snowballs showing as stackable
//     trash item and cant be equipped to throw at other players"; tools/snowball_throw_20261116.py): its
//     record is CLIENT's usable type 2 (procanim; at 0 with no wear slot VIInfoText::IsMisc filed it with the
//     loot, no Use in any menu, and CLIENT never sent one: no line of it in the log), wear 16 (the throwing
//     knives' ranged slot), attack type 10 and reach SnowballRange. So the Start menu lists it twice: under
//     Equipment (Equip) and under the usable items (Use, Prepare onto quick slot 1 or 2). CLIENT sends both
//     as the same 0x003E with its target (UseItem(i, 0)); a quick slot's is UseItem(i, 1). The handler
//     (handlers/potion.go usesPotion) throws one at another player targeted from either, and from a
//     quick slot at anything (refused here when it is no player); from the menu with no other player
//     targeted it is worn or taken off. Worn, CLIENT's ranged auto-attack throws it only where
//     CLIENT swings at a player at all (TargetInRange refuses player against player, channel byte 142 2
//     or 3): a duel's opponent and a PvP foe, each round a snowball's hit (thrownSnowballAtFoeLocked),
//     no duel or PvP harm, one off the worn stack. At a creature it is refused in red and the fight let
//     go (snowballAtNPCLocked): the snowball is for players.
//   - "Self, Moogie's Insight": an effect of its own (InsightEffectID, the Boon's icon, named InsightName) for
//     InsightLasts of play, kept as the Boon is (a logout, a death, Mr. Moogie's clear and a strip leave it;
//     a GM's #clearbuffs ends it); a second drunk puts it back to its whole hour. Every experience award (a
//     kill's, a quest's: xpPctLocked) is the Boon's percent plus InsightPct while it is on: 225% with the
//     Boon's 200, 125% without.
//   - "Self, Deputy's Draught 901843, Radius 30": the spell of the id (Halfling Heroism unless a GM writes
//     another; the handler reads its record into PotionEffect.Grant) lands on every player within the radius
//     of the drinker, the drinker too, in its world, alive and not loading a world, grouped or not, flagged or
//     not, as if cast on it, with no consent asked and none refused for its level; its visual on each. Each is
//     told DeputyLine, the drinker how many it caught.

// The uses' kinds (PotionEffect.Special).
const (
	PotionSnowball = "snowball"
	PotionInsight  = "insight"
	PotionDeputy   = "deputy"
)

// The Snowball's numbers and words (the GM Jibrish's).
const (
	SnowballRange     float32 = 30
	SnowballFX        uint32  = 0x15C66F59 // Frost Shot's projectile
	SnowballRefusal           = "You can only throw a snowball at another player."
	snowballHitLine           = "%s hits you with a snowball!"
	snowballThrowLine         = "You hit %s with a snowball!"
	InsightEffectID           = -1012 // beside the Boon's -1003 (-1004 is the boards' wolf: boardwolf.go)
	InsightName               = "Moogie's Insight"
	InsightIcon       uint32  = 0xDA6F7DE9
	InsightPct                = 25
	InsightLasts              = time.Hour
	InsightLine               = "You feel Mr. Moogie's insight: +25% experience for one hour."
	insightTag                = "insight +25%"
	DeputySpellID             = 901843 // Halfling Heroism
	DeputyRange       float32 = 30
	DeputyLine                = "%s's Deputy's Draught makes a halfling of you!"
	deputyDrinkerLine         = "Your Deputy's Draught makes a halfling of %d %s."
	deputyFlat                = "The draught has gone flat."
)

var (
	reSnowball = regexp.MustCompile(`(?i)^\s*target\s*,\s*snowball\b(?:[\s,]+(?:visual\s+)?(0x[0-9a-f]+|\d+))?\s*$`)
	reInsight  = regexp.MustCompile(`(?i)^\s*self\s*,\s*moogie'?s\s+insight\b`)
	reDeputy   = regexp.MustCompile(`(?i)^\s*self\s*,\s*deputy'?s\s+draught\b(?:\s+(\d+))?(?:\s*,\s*radius\s+(\d+(?:\.\d+)?))?\s*$`)
)

// readMoogieUse reads an itemeffect line that names one of the file's uses into pe. Its other words are
// read as ever (a line of none of them is a potion's).
func (pe *PotionEffect) readMoogieUse(line string) {
	switch {
	case reSnowball.MatchString(line):
		m := reSnowball.FindStringSubmatch(line)
		pe.Special, pe.SpecialFX = PotionSnowball, SnowballFX
		if m[1] != "" {
			if v, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(m[1]), "0x"), base(m[1]), 32); err == nil {
				pe.SpecialFX = uint32(v)
			}
		}
	case reInsight.MatchString(line):
		pe.Special = PotionInsight
	case reDeputy.MatchString(line):
		m := reDeputy.FindStringSubmatch(line)
		pe.Special, pe.SpecialSpell, pe.SpecialRadius = PotionDeputy, DeputySpellID, DeputyRange
		if m[1] != "" {
			pe.SpecialSpell, _ = strconv.Atoi(m[1])
		}
		if m[2] != "" {
			if r, err := strconv.ParseFloat(m[2], 32); err == nil && r > 0 {
				pe.SpecialRadius = float32(r)
			}
		}
	}
}

func base(n string) int {
	if strings.HasPrefix(strings.ToLower(n), "0x") {
		return 16
	}
	return 10
}

// moogieUseLocked does the file's use of pe by p (pe.Special set): the refusal ("" when it was used). Must be
// called with the lock held.
func (w *World) moogieUseLocked(sess *session.Session, p *Player, pe *PotionEffect, targetObjectID uint32, now time.Time) string {
	switch pe.Special {
	case PotionSnowball:
		return w.throwSnowballLocked(sess, p, pe, targetObjectID, now)
	case PotionInsight:
		w.drinkInsightLocked(sess, p, now)
	case PotionDeputy:
		if r := w.drinkDeputyLocked(sess, p, pe, now); r != "" {
			return r
		}
	}
	w.potionUsedLocked(p, pe, now)
	return ""
}

// throwSnowballLocked is a snowball thrown by p at the player targeted (the file's doc). Must be called with
// the lock held.
func (w *World) throwSnowballLocked(sess *session.Session, p *Player, pe *PotionEffect, targetObjectID uint32, now time.Time) string {
	e := w.entitySeenByLocked(sess, targetObjectID)
	t, ok := e.(*Player)
	if !ok || t == nil || t == p || t.IsDead() || t.respawnPending() || !playersShare(p, t) ||
		dist3(p.X, p.Y, p.Z, t.X, t.Y, t.Z) > SnowballRange {
		return SnowballRefusal
	}
	w.potionUsedLocked(p, pe, now)
	w.snowballHitLocked(sess, p, t, pe.SpecialFX)
	return ""
}

// snowballHitLocked is a snowball from p landing on t (the file's doc): its visual, 1 off t's hit points
// (never below 1), each told. Must be called with the lock held.
func (w *World) snowballHitLocked(sess *session.Session, p, t *Player, fx uint32) {
	w.broadcastCastLocked(p, t.ServerID, 0, fx)
	took := 0
	if t.HP > 1 {
		took = 1
		t.HP--
		t.StatsDirty = true
	}
	ts := w.findSessionByEntityIDLocked(t.ServerID)
	if ts != nil {
		if cid := w.objectIDSeenByLocked(ts, p.ServerID); cid != 0 {
			w.sendDamageIndication(ts, cid, -took, ts.PlayerObjectID(), 0)
		}
		w.sendChatToSession(ts, fmt.Sprintf(snowballHitLine, p.Name))
	}
	if sess != nil {
		if tid := w.objectIDSeenByLocked(sess, t.ServerID); tid != 0 {
			w.sendDamageIndication(sess, sess.PlayerObjectID(), -took, tid, 0)
		}
		w.sendChatToSession(sess, fmt.Sprintf(snowballThrowLine, t.Name))
	}
	log.Printf("Snowball: %s hits %s (HP %d/%d, %d taken)", p.Name, t.Name, t.HP, t.MaxHPNow(), took)
}

// snowballToldGap is how often a worn snowball's refusal at a creature is said.
const snowballToldGap = 2 * time.Second

// wornSnowballLocked is the snowball a swing throws (Swing.Potion a snowball's record), nil for any other
// swing. Must be called with the lock held.
func (w *World) wornSnowballLocked(s Swing) *PotionEffect {
	if s.Potion == 0 {
		return nil
	}
	if pe := w.potions.byPattern[s.Potion]; pe != nil && pe.Special == PotionSnowball {
		return pe
	}
	return nil
}

// snowballAtNPCLocked refuses a worn snowball's throw at a creature (the file's doc): said in red, at most
// every snowballToldGap. Must be called with the lock held.
func (w *World) snowballAtNPCLocked(sess *session.Session, p *Player, now time.Time) {
	if sess == nil || !p.snowballToldAt.IsZero() && now.Sub(p.snowballToldAt) < snowballToldGap {
		return
	}
	p.snowballToldAt = now
	w.sendColoredLocked(sess, SnowballRefusal, castRefusalRGB)
	log.Printf("Snowball: %s's worn snowball at a creature: refused", p.Name)
}

// thrownSnowballAtFoeLocked is a duel's or a PvP fight's round with a worn snowball (the file's doc): a
// snowball's hit on t and nothing of the fight's harm, one off the worn stack, the fight let go with the
// last. It reports whether the fight ended. Must be called with the lock held.
func (w *World) thrownSnowballAtFoeLocked(p, t *Player, pe *PotionEffect, now time.Time) bool {
	sess := w.findSessionByEntityIDLocked(p.ServerID)
	if t.IsDead() || !playersShare(p, t) {
		return true
	}
	w.snowballHitLocked(sess, p, t, pe.SpecialFX)
	p.Swing.PotionLeft--
	if fn := w.potions.onThrown; fn != nil && sess != nil {
		go fn(sess, p.CharServerID, pe.PatternID)
	}
	if p.Swing.PotionLeft <= 0 {
		log.Printf("Snowball: %s threw the last %q", p.Name, pe.ItemName)
		p.Swing.Potion = 0
		return true
	}
	return false
}

// TargetsOtherPlayer is whether the object the session names is another player (a snowball's mark:
// handlers/potion.go usesPotion).
func (w *World) TargetsOtherPlayer(sess *session.Session, objectID uint32) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	t, ok := w.entitySeenByLocked(sess, objectID).(*Player)
	return ok && t != nil && p != nil && t != p
}

// playersShare reports two players in one world and, in an instanced one, one instance.
func playersShare(a, b *Player) bool {
	return a.World == b.World && (!Instanced(a.World) || a.instanceKey() == b.instanceKey())
}

// drinkInsightLocked puts Moogie's Insight on p for its whole hour, in place of one on it (the file's doc).
// Must be called with the lock held.
func (w *World) drinkInsightLocked(sess *session.Session, p *Player, now time.Time) {
	renewed := p.hasEffect(InsightEffectID)
	w.addEffectLocked(p, &ActiveEffect{SpellID: InsightEffectID, Name: InsightName, Icon: InsightIcon, Caster: p.ServerID,
		Until: now.Add(InsightLasts)}, now)
	if !renewed {
		w.broadcastCastLocked(p, p.ServerID, 0, fxBuff)
	}
	if sess != nil {
		w.sendChatToSession(sess, InsightLine)
	}
	log.Printf("Insight: %s has %s for %v (renewed %v)", p.Name, InsightName, InsightLasts, renewed)
}

// drinkDeputyLocked lands the draught's spell on every player near p (the file's doc): the refusal, "" when
// it was drunk. Must be called with the lock held.
func (w *World) drinkDeputyLocked(sess *session.Session, p *Player, pe *PotionEffect, now time.Time) string {
	if pe.Grant == nil {
		return deputyFlat // the handler found no record of its spell
	}
	spell := *pe.Grant
	spell.Def.MinLevel = 0      // none refused for its level: everyone near "like it or not"
	spell.figures = figuresGift // none of the drinker's figures: untagged (numbertags.go)
	var caught []string
	for _, t := range w.Players {
		if t.IsDead() || t.respawnPending() || !playersShare(p, t) || dist3(p.X, p.Y, p.Z, t.X, t.Y, t.Z) > pe.SpecialRadius ||
			w.travelDueLocked(t) {
			continue
		}
		ts := w.findSessionByEntityIDLocked(t.ServerID)
		if ts == nil || ts.StateChannelsHeld() {
			continue // loading a world, or no client to land on
		}
		w.broadcastCastLocked(p, t.ServerID, 0, spell.visual())
		w.landOnPlayerLocked(&castState{spell: spell, caster: p.ServerID, targets: []EntityID{t.ServerID}}, p, sess, t, now)
		w.sendChatToSession(ts, fmt.Sprintf(DeputyLine, p.Name))
		caught = append(caught, t.Name)
	}
	if sess != nil {
		who := "player"
		if len(caught) != 1 {
			who = "players"
		}
		w.sendChatToSession(sess, fmt.Sprintf(deputyDrinkerLine, len(caught), who))
	}
	log.Printf("Deputy: %s's %q lands %q on %d: %s", p.Name, pe.ItemName, spell.Name, len(caught), strings.Join(caught, ", "))
	return ""
}

// insightPct is Moogie's Insight's share added to an award's percent, 0 without it. Must be called with
// the lock held.
func insightPct(p *Player) int {
	if p.hasEffect(InsightEffectID) {
		return InsightPct
	}
	return 0
}
