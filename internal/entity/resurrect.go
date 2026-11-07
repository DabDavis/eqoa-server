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
//   - a death at any level (playerDiedLocked, deathdebt.go) keeps what it added
//     to the debt, and where it happened, as the character's chance of a rez
//     (Player.Rez). A later death takes its place and ends any offer of the
//     earlier's way back, so the earlier one's debt and place are past recovering
//     (petition 1130, the GM Jibrish: "One rez per death, if they die again they
//     lose the ability to get rez'd from the previous death. Only the latest rez
//     death location is saved");
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
//   - the way back to the body is the target's to take (CLIENT's prompt, 0x00D3's
//     resurrect form; petitions 1051, 1130): its Yes goes there, its No stays where
//     it is, and the share comes off the debt whichever it answers. A rez has no
//     level to it (petition 1130: "including level 60 res on level 1");
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
	// Placed and the place are where the death was (petition 1051): a rez in
	// the chance's time offers its target the way back there, debt or none.
	// Kept with the chance over a logout and a restart (table charrez; petition
	// 1130, the GM Jibrish: "Only the latest rez death location is saved").
	Placed  bool
	World   uint8
	X, Y, Z float32
}

// live reports whether the chance still counts at now: a debt to give back
// or a place to go back to.
func (c RezChance) live(now time.Time) bool {
	return (c.Debt > 0 || c.Placed) && now.Before(c.Until)
}

// returns reports whether a rez still offers the way back to the body.
func (c RezChance) returns(now time.Time) bool {
	return c.Placed && now.Before(c.Until)
}

// RezReturn is a rez's offer of the way back to where its target died,
// waiting for the target's answer to CLIENT's prompt (0x00D3's resurrect
// form; its Yes is 0x00D4 1) until Until.
type RezReturn struct {
	World   uint8
	X, Y, Z float32
	Until   time.Time
}

// RezOfferFor is how long a rez's offer waits for its answer (ours).
const RezOfferFor = 2 * time.Minute

// RezOfferFunc shows a player a rez's offer: CLIENT's prompt "<caster> wishes
// to resurrect you" (handlers: SendSummonPrompt). It runs under the World's
// lock and must not call back into the World.
type RezOfferFunc func(sess *session.Session, caster string)

// OnRezOffer sets the hook that shows a rez's offer.
func (w *World) OnRezOffer(fn RezOfferFunc) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.rezOffer = fn
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

// noteRezChance keeps what a death added to the debt, and where it happened, as
// the character's chance of a rez: the latest death's alone, so an offer still
// waiting on the earlier one's way back ends with it (playerDiedLocked).
func (p *Player) noteRezChance(debt int, now time.Time) {
	p.Rez = RezChance{Debt: max(debt, 0), Until: now.Add(RezWindow), Placed: true, World: p.World, X: p.X, Y: p.Y, Z: p.Z}
	p.RezReturn = RezReturn{}
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

// rezNamesAnotherLocked reports a rez's own targets naming a player other than
// its caster. The workbook gives nearly every rez a group's scope at an endless
// radius where its record and its line say a target, and an area of a group's
// starts from its caster (areaStartLocked), so until 2026-11-06 every such rez cast
// at someone landed on its caster, and on the group's others with no rez in it:
// no debt came off anyone (petition 1128: "rez does not work i think it my be self
// casting?"). Cast at someone, it is that one's alone. Must be called with the
// lock held.
func (w *World) rezNamesAnotherLocked(targets []EntityID, p *Player) bool {
	for _, id := range targets {
		if id != p.ServerID && w.Players[id] != nil {
			return true
		}
	}
	return false
}

// rezGroupNearLocked is the others of a group's rez cast at its caster, or a self
// spell's of a group's scope: the members of its group within r in its world, dead
// or alive, with something for it to do: a share of debt, or a way back to the body.
// Must be called with the lock held.
func (w *World) rezGroupNearLocked(p *Player, pct int, r float32, now time.Time) []areaNear {
	var out []areaNear
	for _, m := range w.groupMembersLocked(p) {
		if m == p || m.World != p.World || (rezAmount(m, pct, now) <= 0 && !m.Rez.returns(now)) {
			continue
		}
		if d := dist3(m.X, m.Y, m.Z, p.X, p.Y, p.Z); d <= r {
			out = append(out, areaNear{m.ServerID, d})
		}
	}
	return out
}

// rezStartLocked checks a rez as it begins (StartCast): when no target has
// anything to recover, nor the caster's group for a group's rez cast at its
// caster, the refusal's line, and a fizzle (the lists' "the spell fizzles
// otherwise"). A pet is no player: it has nothing to recover. Must be called with
// the lock held.
func (w *World) rezStartLocked(p *Player, spell *CastSpell, targets []EntityID, now time.Time) (refusal string, fizzle bool) {
	d := &spell.Def
	pct := d.Resurrects()
	if a := &spell.Area; a.Centre != AreaNone && (spell.Scope == 2 || d.Target == TargetGroup) && !w.rezNamesAnotherLocked(targets, p) &&
		len(w.rezGroupNearLocked(p, pct, a.radiusFor(false), now)) > 0 {
		return "", false
	}
	name := ""
	for _, id := range targets {
		if t := w.Players[id]; t != nil {
			if rezAmount(t, pct, now) > 0 || t.Rez.returns(now) {
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
	if t.Rez.returns(now) {
		w.offerRezReturnLocked(caster, t, now) // the way back to the body, debt or none
	}
	if n <= 0 {
		returned := t.Rez.returns(now)
		t.Rez = RezChance{}
		if returned {
			return
		}
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
	c := p.Rez
	return database.RezRow{Debt: c.Debt, LeftMS: c.Until.Sub(now).Milliseconds(), Placed: c.Placed, World: c.World, X: c.X, Y: c.Y, Z: c.Z}, true
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
	if (row.Debt > 0 || row.Placed) && row.LeftMS > 0 {
		p.Rez = RezChance{Debt: row.Debt, Until: now.Add(time.Duration(row.LeftMS) * time.Millisecond),
			Placed: row.Placed, World: row.World, X: row.X, Y: row.Y, Z: row.Z}
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

// offerRezReturnLocked offers a rez's target the way back to where it died
// (petition 1051, Jibheals: "Even if the character has no debt to clear, they
// should still get the popup to get back to where they last died IF eligible
// to rez"): CLIENT's resurrect prompt (OnRezOffer's hook), its answer waited
// for RezOfferFor. Must be called with the lock held.
func (w *World) offerRezReturnLocked(caster, t *Player, now time.Time) {
	c := t.Rez
	t.RezReturn = RezReturn{World: c.World, X: c.X, Y: c.Y, Z: c.Z, Until: now.Add(RezOfferFor)}
	if w.rezOffer != nil {
		if ts := w.findSessionByEntityIDLocked(t.ServerID); ts != nil {
			w.rezOffer(ts, caster.Name)
		}
	}
	log.Printf("Rez: %s offers %s the way back to (%.1f, %.1f, %.1f) world %d", caster.Name, t.Name, c.X, c.Y, c.Z, c.World)
}

// DeathPlaceLive reports whether the session's player, alive, may still go back
// to where it last died: a rez's chance with its place left.
func (w *World) DeathPlaceLive(sess *session.Session, now time.Time) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	return p != nil && !p.IsDead() && !p.respawnPending() && p.Rez.returns(now)
}

// TakeDeathPlace is the way back a town's board or sign gives (petition 1218,
// the GM Jibrish: "a 0% XP resurrect that sends them back to the point they
// died at", "so if you die and ... no clerics on you can still get back"; the
// words handlers/boardinvis.go's): where the player died, one trip, the place
// spent as a rez's offer spends it. No experience comes back and the debt is
// as it was, so a cleric's rez in the chance's time still gives its share. ok
// is false with no place, or the player dead.
func (w *World) TakeDeathPlace(sess *session.Session, now time.Time) (world uint8, x, y, z float32, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.IsDead() || p.respawnPending() || !p.Rez.returns(now) {
		return 0, 0, 0, 0, false
	}
	c := p.Rez
	p.Rez.Placed = false
	p.RezReturn = RezReturn{}
	log.Printf("Rez: %s takes a board's way back to (%.1f, %.1f, %.1f) world %d (debt %d, no share given)", p.Name, c.X, c.Y, c.Z, c.World, p.Debt)
	return c.World, c.X, c.Y, c.Z, true
}

// TakeRezReturn is the session's player's answer to a rez's offer: the offer
// taken (on a No too), and on a Yes with one in its time the place it leads
// back to, the player brought back to life there first when it lies dead
// (hit points and power whole, as a respawn's). ok is false with no offer.
func (w *World) TakeRezReturn(sess *session.Session, yes bool, now time.Time) (world uint8, x, y, z float32, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return 0, 0, 0, 0, false
	}
	r := p.RezReturn
	p.RezReturn = RezReturn{}
	if !yes || r.Until.IsZero() || !now.Before(r.Until) {
		return 0, 0, 0, 0, false
	}
	if p.respawnPending() {
		p.DiedAt = time.Time{}
		p.HP, p.Power = p.MaxHP, p.MaxPower
		p.Breath = BreathFull
		p.DrowningCounter = 0
		p.StatsDirty = true
	}
	log.Printf("Rez: %s goes back to (%.1f, %.1f, %.1f) world %d", p.Name, r.X, r.Y, r.Z, r.World)
	return r.World, r.X, r.Y, r.Z, true
}
