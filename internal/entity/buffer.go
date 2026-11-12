package entity

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// buffer.go — Buffer, who buffs whoever talks to him (the GM Jibrish's petition 1390, Heirloom: "Add a vendor
// close but north of the announcement board in FP named "Buffer". When spoken to, buffer gives you avatar, order,
// pure heart, the cleric + all resists buffs and a stackable GM AC that is +500ac buff. They last for 5 hours. It
// casts it after 1 second."). handlers/buffer.go reads which spells (their records, at each talk) and asks here.
//
// A talk begins his cast at the speaker: each spell's 0x00B1 from him at it, BufferCastTime long, in the spell's
// own visual, to everyone who sees him (CLIENT plays each on him for the cast time, then on the speaker, as any
// creature's cast; he does not turn, so he keeps facing the plaza). BufferCastTime later bufferSystem lands every
// one on the speaker as LandOnSelf lands a board's gift (as if it had cast them on itself: a spell's share of its
// caster's WIS is the speaker's), each for BufferLasts of play: the spell's own duration is its record's for anyone
// else's cast (30 minutes for most), only his casts last five hours. Each goes on as its spell's effect, its icon
// and name on the effects channel, under every rule of its line (buffline.go: a stronger one of its line on is not
// taken off, its bearer told), and is kept over a logout and ended by a death, a strip or Mr. Moogie's "Clear my
// buffs" as any buff is. The speaker alone: its group, its pet and those round it get none.
//
// A second talk with them on lands them again, each in its own old one's place at its whole five hours (a recast's
// rule, addEffectLocked); a talk while his cast at the speaker is on its way is let go. He will not cast on the
// dead, on one flagged for PvP or in a duel (Mr. Moogie's rule, moogie.go: Avatar heals as it lands), and a cast
// finds nobody when the speaker has died, left the world or gone past bufferReach of him by its landing. Not kept
// over a restart (a cast on its way is not). The words are ours.

// BufferName is his name: an NPC of it is Buffer.
const BufferName = "Bruce Buffer"

// IsBuffer is whether an NPC of the name is Buffer.
func IsBuffer(name string) bool {
	n := strings.TrimSpace(name)
	// "Buffer" was his name until 2026-11-12 (the owner: "Bruce Buffer"); a row not yet renamed still is he
	return strings.EqualFold(n, BufferName) || strings.EqualFold(n, "Buffer")
}

const (
	// BufferCastTime is how long his cast takes: Jibrish's "It casts it after 1 second".
	BufferCastTime = time.Second
	// BufferLasts is how long what he casts lasts, of play: Jibrish's "They last for 5 hours".
	BufferLasts = 5 * time.Hour
	// bufferReach is how near him the speaker must still be when his cast lands (a talk's reach is
	// InteractRange; a second's walk takes it no farther than this)
	bufferReach float32 = 30
)

// His words (ours).
const (
	BufferLineBegins = "Bruce Buffer begins to cast his blessings on you."
	BufferLineDone   = "Bruce Buffer says, 'There you are, %s: %s, for five hours of play.'"
	BufferLineHad    = "Bruce Buffer says, 'You already carry stronger blessings than mine, %s.'"
	BufferLineDead   = "Buffer cannot help you now."
	BufferLinePvP    = "Buffer will not buff you while you are flagged for PvP."
	BufferLineDuel   = "Buffer will not buff you while you are in a duel."
	BufferLineNone   = "Bruce Buffer says, 'I have no blessings to give today.'"
)

// bufferCast is one of his casts on its way: whom it is at (the player as it was: a new login under the same id
// is not it), which Buffer, the spells as he lands them and when.
type bufferCast struct {
	npc    EntityID
	player EntityID
	who    *Player
	spells []CastSpell
	lands  time.Time
}

// StartBuffer begins Buffer's cast at the session's player (the file's doc): spells, in order, each lasting
// BufferLasts where it lasts at all. It returns the line to tell the player and whether the cast began; a refusal's
// line is said in red, and nothing is said (an empty line) when one of his casts at the player is on its way, he is
// gone, or there is nothing to cast.
func (w *World) StartBuffer(sess *session.Session, buffer EntityID, spells []CastSpell, now time.Time) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	npc := w.NPCs[buffer]
	switch {
	case p == nil || p.IsDead() || p.respawnPending():
		return BufferLineDead, false
	case npc == nil || npc.Dead || len(spells) == 0:
		return "", false
	case p.pvp.on:
		return BufferLinePvP, false
	case w.duelOnLocked(p.ServerID) != nil || w.metalDuelOfLocked(p.ServerID):
		return BufferLineDuel, false
	}
	for _, c := range w.buffers {
		if c.player == p.ServerID && c.who == p {
			log.Printf("Buffer: %s talks to %s while his cast is on its way: let go", p.Name, npc.Name)
			return "", false
		}
	}
	c := &bufferCast{npc: buffer, player: p.ServerID, who: p, lands: now.Add(BufferCastTime)}
	ms := uint32(BufferCastTime / time.Millisecond)
	names := make([]string, 0, len(spells))
	for _, s := range spells {
		if s.Def.Duration > 0 {
			s.Def.Duration = BufferLasts // his casts alone: the record keeps its own
		}
		c.spells = append(c.spells, s)
		names = append(names, s.Name)
		w.broadcastNPCCastLocked(npc, p.ServerID, ms, s.visual())
	}
	w.buffers = append(w.buffers, c)
	log.Printf("Buffer: %s begins %s on %s: lands in %v, each for %v", npc.Name, strings.Join(names, ", "), p.Name,
		BufferCastTime, BufferLasts)
	return BufferLineBegins, true
}

// bufferSystem lands every cast of Buffer's whose time has come. Every tick; must be called with the lock held.
func (w *World) bufferSystem(now time.Time) {
	if len(w.buffers) == 0 {
		return
	}
	keep := w.buffers[:0]
	for _, c := range w.buffers {
		if now.Before(c.lands) {
			keep = append(keep, c)
			continue
		}
		w.bufferLandLocked(c, now)
	}
	for i := len(keep); i < len(w.buffers); i++ {
		w.buffers[i] = nil
	}
	w.buffers = keep
}

// bufferLandLocked lands one of his casts on its speaker (the file's doc), and tells it which took. Must be called
// with the lock held.
func (w *World) bufferLandLocked(c *bufferCast, now time.Time) {
	p := w.Players[c.player]
	if p == nil || p != c.who || p.IsDead() || p.respawnPending() {
		log.Printf("Buffer: a cast at player %d finds nobody (gone or dead)", c.player)
		return
	}
	npc := w.NPCs[c.npc]
	if npc == nil || npc.Dead || npc.World != p.World || DistanceBetween(npc, p) > bufferReach {
		log.Printf("Buffer: his cast at %s finds nobody: gone from him", p.Name)
		return
	}
	sess := w.findSessionByEntityIDLocked(p.ServerID)
	// a renewal takes each old one off before its new one goes on (addEffectLocked), and the hit points and power are
	// held within the maximum that leaves for the moment: so a spell that heals as it lands (Avatar) goes on last, its
	// heal after every maximum is back, and his casts, which only give, leave neither below what the speaker had (a
	// renewal at full health stays full)
	hp, power := p.HP, p.Power
	order := make([]int, 0, len(c.spells))
	for _, last := range []bool{false, true} {
		for i := range c.spells {
			if healsAsItLands(&c.spells[i]) == last {
				order = append(order, i)
			}
		}
	}
	took := make([]bool, len(c.spells))
	for _, i := range order {
		s := c.spells[i]
		s.figures = figuresGift // his, not the speaker's: told untagged (numbertags.go)
		w.landOnPlayerLocked(&castState{spell: s, caster: p.ServerID, targets: []EntityID{p.ServerID}}, p, sess, p, now)
		if s.Def.Duration <= 0 {
			took[i] = true // nothing of it lasts: what it does, it did
			continue
		}
		for _, e := range p.Effects {
			if e.SpellID == s.SpellID && !e.Until.Before(now.Add(s.Def.Duration)) {
				took[i] = true
				break
			}
		}
	}
	if p.HP < hp || p.Power < power {
		p.HP, p.Power = max(p.HP, min(hp, p.MaxHPNow())), max(p.Power, min(power, p.MaxPowerNow()))
		p.StatsDirty = true
	}
	var names []string // in his order, whatever order they went on in
	for i, s := range c.spells {
		if took[i] {
			names = append(names, s.Name)
		}
	}
	log.Printf("Buffer: %s's cast lands on %s: %d of %d took (%s), until %s", npc.Name, p.Name, len(names), len(c.spells),
		strings.Join(names, ", "), now.Add(BufferLasts).UTC().Format("15:04:05"))
	if sess == nil {
		return
	}
	if len(names) == 0 {
		w.sendChatToSession(sess, fmt.Sprintf(BufferLineHad, p.Name))
		return
	}
	w.sendChatToSession(sess, fmt.Sprintf(BufferLineDone, p.Name, andList(names)))
}

// healsAsItLands reports a spell with a heal of its bearer as it lands (Avatar's "Increase Hitpoints by 1500").
func healsAsItLands(s *CastSpell) bool {
	for _, e := range s.Def.Effects {
		if e.Kind == EffectHP && e.Amount > 0 && !e.Self {
			return true
		}
	}
	return false
}

// andList lists names as a sentence does: "A", "A and B", "A, B and C".
func andList(names []string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}
