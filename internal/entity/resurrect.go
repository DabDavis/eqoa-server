package entity

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// resurrect.go — the resurrection spells: a rez gives back its printed share
// of the experience debt its target's latest death added, once per death
// (jib's review, F3 pass 19).
//
// The lines are EQOA-Data's (Information/databases/spells), as spelleffect
// holds them: "Target, Ressurect Player: restore 20% experience debt" (the
// cleric's Revive to Reviviscence, the druid's Renew and Nature's Renewal,
// the necromancer's Reanimate), "Ressurect Player: restore 60% experience
// debt" (Reclamation, with no target word), "Self, Ressurect Player: restore
// 20% experience debt" (the paladin's Faith's Revival to Faith's Renewal,
// whose mastery EQOA-Data calls "Self-rez") and "Target, 80% REZ" (the
// Exorcist's Divine Revival). readResurrect reads any clause holding a
// resurrection's word and one percentage.
//
// What the live game did, as eqoa.fandom.com's lists of Cleric, Druid,
// Shaman and Necromancer Spells word it: a rez "may only be cast on a target
// who is in a group with you, and only if they have died recently (the spell
// fizzles otherwise). If it is succesful, they will be prompted to choose to
// be teleported back to the spot they were last killed, and a portion of
// their experience debt will be removed. XP debt can only be removed this way
// once after every death." And: "XP debt is removed when the spell is cast,
// regardless of whether or not the player chooses to return to their place
// of death." Here:
//
//   - a death that adds debt (playerDiedLocked, deathdebt.go) keeps what it
//     added as the character's chance of a rez (Player.Rez). A later death
//     takes its place, so the earlier one's debt is past recovering; a death
//     that adds nothing (below level 6) leaves no chance at all;
//   - a rez at a player, dead (the 15 s before its respawn) or released,
//     found and in range as castTargetsLocked finds any player, takes the
//     spell's share of that chance off the debt, at most what is still owed,
//     and spends the chance: 0x00D9 with the new debt and a line to the
//     target, a line to the caster. A "Self" line lands on its caster
//     whatever is targeted, as a self spell does;
//   - a target with nothing to recover is refused as the cast begins, as an
//     undead-only spell is (undead.go): a fizzle, a red line, no power, no
//     lockout. A rez that finds nothing when it lands (another rez landed
//     first, the chance's time ran out, the debt was paid) has spent its
//     power, as a heal on someone whole has, and the caster is told so. One
//     whose target died again before it landed gives back its share of that
//     newer death's debt, the latest.
//
// Ours, as no record says otherwise:
//
//   - a chance counts for RezWindow, 30 minutes of the character's time in
//     the world: the lists' "died recently" gives no figure. The time stops at
//     a logout, as a spell effect's does (buffsave.go), so a lost connection
//     just after a death does not lose it;
//   - a rez never moves anyone and never revives: a dead target still
//     respawns at its bind point 15 s after its death (respawn.go), with less
//     to pay. The live game's rez was a teleport back to the body that the
//     target had to accept (the client's own prompt is 0x00D3's resurrect
//     form); here there is nothing to accept, as a lower debt never hurts,
//     and reviving where the body lies would put a player back in the lava or
//     the deep water that may have killed it, beside the creature that did;
//   - the target need not be in the caster's group, as the user chose: any
//     player in range;
//   - the share is rounded to the nearest point of experience, at most 100%;
//   - the words of the lines told and of the refusal.

// RezWindow is how long after a death a rez still counts: 30 minutes of the
// character's time in the world (ours).
const RezWindow = 30 * time.Minute

// castNoRez is the refusal of a rez at a target with nothing to recover
// (ours), and what the caster is told when a rez lands on nothing.
const castNoRez = "%s has no experience to recover."

// RezChance is what a resurrection may still give back: the debt the
// character's latest death added, until when. The zero RezChance is none.
type RezChance struct {
	Debt  int
	Until time.Time
}

// live reports whether the chance still counts at now.
func (c RezChance) live(now time.Time) bool {
	return c.Debt > 0 && now.Before(c.Until)
}

// reRezWord is a resurrection's word in a clause, whole: the lists'
// "Ressurect" (sic), "resurrect", "resurrection", "rez", "rezz".
var reRezWord = regexp.MustCompile(`\b(?:res+ur+ect\w*|rezz?)\b`)

// rePercent is a percentage in a clause.
var rePercent = regexp.MustCompile(`(\d+)\s*%`)

// rezShareOf reads a clause as a resurrection: its word and its one
// percentage ("ressurect player: restore 20% experience debt", "80% rez").
func rezShareOf(clause string) (int, bool) {
	if !reRezWord.MatchString(clause) {
		return 0, false
	}
	m := rePercent.FindAllStringSubmatch(clause, -1)
	if len(m) != 1 {
		return 0, false
	}
	n, _ := strconv.Atoi(m[0][1])
	return min(n, 100), n > 0
}

// readResurrect makes a resurrection (EffectResurrect, its Amount the share
// in percent) of the clause the line's reader left unread (ParseSpellText).
func (d *SpellDef) readResurrect() {
	var rest []string
	for _, u := range d.Unread {
		if pct, ok := rezShareOf(u); ok && d.Resurrects() == 0 {
			d.Effects = append(d.Effects, SpellEffect{Kind: EffectResurrect, Amount: pct, AmountMax: pct})
			continue
		}
		rest = append(rest, u)
	}
	d.Unread = rest
}

// Resurrects is the share, in percent, of the latest death's debt the spell
// gives back; 0 for a spell that is no resurrection.
func (d *SpellDef) Resurrects() int {
	for _, e := range d.Effects {
		if e.Kind == EffectResurrect {
			return e.Amount
		}
	}
	return 0
}

// noteRezChance keeps what a death added to the debt as the character's
// chance of a rez: the latest death's alone, none for a death that added
// nothing (playerDiedLocked).
func (p *Player) noteRezChance(debt int, now time.Time) {
	if debt <= 0 {
		p.Rez = RezChance{}
		return
	}
	p.Rez = RezChance{Debt: debt, Until: now.Add(RezWindow)}
}

// rezAmount is what a rez of pct percent gives t back now: its share of the
// latest death's debt, at most what t still owes; 0 for nothing to recover.
func rezAmount(t *Player, pct int, now time.Time) int {
	if pct <= 0 || !t.Rez.live(now) {
		return 0
	}
	n := (t.Rez.Debt*min(pct, 100) + 50) / 100
	return max(0, min(n, t.Debt))
}

// rezStartLocked checks a rez as it begins (StartCast): when no target has
// anything to recover, the refusal's line, and a fizzle (the lists' "the
// spell fizzles otherwise"). A pet is no player: it has nothing to recover.
// Must be called with the lock held.
func (w *World) rezStartLocked(targets []EntityID, d *SpellDef, now time.Time) (refusal string, fizzle bool) {
	pct := d.Resurrects()
	name := ""
	for _, id := range targets {
		if t := w.Players[id]; t != nil {
			if rezAmount(t, pct, now) > 0 {
				return "", false
			}
			if name == "" {
				name = t.Name
			}
		} else if npc := w.NPCs[id]; npc != nil && name == "" {
			name = strings.TrimSpace(npc.Name)
		}
	}
	if name == "" {
		name = "Your target"
	}
	return fmt.Sprintf(castNoRez, name), true
}

// rezLandLocked is a rez landing on t, dead or alive: its share off the
// debt, the chance spent, both told. A spell that is no rez does nothing
// here. Must be called with the lock held.
func (w *World) rezLandLocked(cs *castState, caster *Player, casterSess *session.Session, t *Player, now time.Time) {
	pct := cs.spell.Def.Resurrects()
	if pct <= 0 {
		return
	}
	n := rezAmount(t, pct, now)
	if n <= 0 {
		log.Printf("Rez: %s's %q lands on %s with nothing to recover (chance %d, live %v; %d owed)",
			caster.Name, cs.spell.Name, t.Name, t.Rez.Debt, t.Rez.live(now), t.Debt)
		if casterSess != nil {
			w.sendColoredLocked(casterSess, fmt.Sprintf(castNoRez, t.Name), castRefusalRGB)
		}
		return
	}
	added := t.Rez.Debt
	t.Debt -= n
	t.Rez = RezChance{}
	left := fmt.Sprintf(" (%d left).", t.Debt)
	if t.Debt == 0 {
		left = ", and it is paid off."
	}
	if ts := w.findSessionByEntityIDLocked(t.ServerID); ts != nil {
		sendExpDebt(ts, t.Debt)
		if t == caster {
			w.sendChatToSession(ts, fmt.Sprintf("Your %s took %d off your experience debt%s", cs.spell.Name, n, left))
		} else {
			w.sendChatToSession(ts, fmt.Sprintf("%s's %s took %d off your experience debt%s", caster.Name, cs.spell.Name, n, left))
		}
	}
	if casterSess != nil && t != caster {
		w.sendChatToSession(casterSess, fmt.Sprintf("Your %s took %d off %s's experience debt.", cs.spell.Name, n, t.Name))
	}
	log.Printf("Rez: %s's %q (%d%%) takes %d off %s's debt, of the %d its last death added (%d owed now; dead %v)",
		caster.Name, cs.spell.Name, pct, n, t.Name, added, t.Debt, t.IsDead())
}

// SavedRez is a player's chance of a rez as table charrez keeps it, what is
// left of its time now (a zero row: none); ok is false when id is no player
// in the world.
func (w *World) SavedRez(id EntityID, now time.Time) (database.RezRow, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.Players[id]
	if p == nil {
		return database.RezRow{}, false
	}
	if !p.Rez.live(now) {
		return database.RezRow{}, true
	}
	return database.RezRow{Debt: p.Rez.Debt, LeftMS: p.Rez.Until.Sub(now).Milliseconds()}, true
}

// RestoreRez puts a kept chance of a rez back on the session's player at its
// login, its time running again from now. It reports whether one counts.
func (w *World) RestoreRez(sess *session.Session, row database.RezRow, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	p.Rez = RezChance{}
	if row.Debt > 0 && row.LeftMS > 0 {
		p.Rez = RezChance{Debt: row.Debt, Until: now.Add(time.Duration(row.LeftMS) * time.Millisecond)}
	}
	return p.Rez.live(now)
}

// RezOf is what a rez may still give back a share of (the debt the player's
// latest death added) and for how much longer; 0 for none (a GM's #debt).
func (w *World) RezOf(id EntityID, now time.Time) (debt int, left time.Duration, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.Players[id]
	if p == nil {
		return 0, 0, false
	}
	if !p.Rez.live(now) {
		return 0, 0, true
	}
	return p.Rez.Debt, p.Rez.Until.Sub(now), true
}
