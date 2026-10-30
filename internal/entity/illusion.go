package entity

import (
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// illusion.go — illusions: a spell that shows its bearer in another body.
//
// jib's F3 20261025-173245: "Illusion: <race>" turns the caster into that
// race's player body, whatever is targeted, its own features carried over
// (hair style 3 in pink stays hair style 3 in pink on the troll);
// "Glamour: <race>" does the same and adds its line's stats, cast on the
// caster or a member of its group; "Illusion: Clay Golem" and the like take
// that creature's body. They end at their duration, or with Remove Illusion
// (True Form for a druid), and are kept over a logout as every effect is
// (buffsave.go).
//
// What a body is on the wire: the entity channel's model id (bytes 28-31)
// and scale (36-39), nothing else. CLIENT dresses a player body from the
// body's own race and sex (VICSpriteCust: the face from $Race[race][face]
// [sex] of the CSprite), with the hair and face numbers the channel carries
// for the bearer, so a race's body needs only its model: the bearer's sex
// picks which of the race's two (charactermodel). A creature's body is the
// most common one this server's NPCs of that name wear, at their size. The
// lines' "Faction Change" is not done: nothing here makes a creature's
// regard depend on the body it sees.

// FormKind is whom an illusion spell lands on.
type FormKind int

const (
	FormNone  FormKind = iota
	FormSelf           // "Illusion: X", "X Form": the caster, whatever is targeted
	FormGroup          // "Glamour: X": the caster or a member of its group
	FormEnd            // "Remove Illusion", "True Form", "Revert Lich": the caster's own look back
)

// illusionDuration is how long an illusion lasts when its line gives no
// time (Illusion: Rat, Mouse Form, which have none): the lists' usual for
// illusions, ten minutes. Ours, not measured.
const illusionDuration = 10 * time.Minute

// castNotInGroup refuses a glamour at someone outside the caster's group, in
// the red of Sandstorm's other refusals. The words are ours: no recording
// has one.
const castNotInGroup = "Target is not in your group."

// illusionRaces are the player races an illusion's name may give, on the
// network enum (Human 0 ... Ogre 9), "delf" as the lists spell Dark Elf.
var illusionRaces = map[string]int{
	"human": 0, "elf": 1, "dark elf": 2, "delf": 2, "gnome": 3, "dwarf": 4,
	"troll": 5, "barbarian": 6, "halfling": 7, "erudite": 8, "ogre": 9,
}

// readIllusion reads an illusion out of a spell's name: "Illusion: Troll",
// "Glamour: Elf", "Mouse Form" (when its description assumes the form:
// "Assume the form of a mouse.", "CM: Illusion"; "grants the power of the
// beast without changing your form" does not), and the ends, "Remove
// Illusion" and "True Form". The lines' "Troll form" and "Form" clauses say
// the same and are no longer counted unread. A rogue's mask is an illusion
// of the race its line names ("Self, dark elf Mask, 10min": the user's
// ruling of 2026-10-29, "Rogue masks were illusions"), and Remove Mask ends
// the masks alone (Allakhazam's April 2004 record: it dispels the four
// masks); a mask naming no race (Mortal Mask's "Sprite Change Race") stays a
// look with no body.
func (d *SpellDef) readIllusion(name, desc string) {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.TrimSpace(strings.TrimSuffix(n, "+"))
	ds := strings.ToLower(desc)
	masks := false
	switch {
	case strings.HasPrefix(n, "illusion:"):
		d.Form, d.FormKind = strings.TrimSpace(strings.TrimPrefix(n, "illusion:")), FormSelf
	case strings.HasPrefix(n, "glamour:"):
		d.Form, d.FormKind = strings.TrimSpace(strings.TrimPrefix(n, "glamour:")), FormGroup
	case n == "remove illusion" || n == "true form" || n == "revert lich":
		// Revert Lich is the necromancer's own way out of Lich ("Self, Removes
		// Lich form"): no necromancer has Remove Illusion
		d.FormKind = FormEnd
	case strings.HasSuffix(n, " form") && !strings.Contains(ds, "without changing") &&
		(strings.Contains(ds, "assume the form") || strings.Contains(ds, "illusion")):
		d.Form, d.FormKind = strings.TrimSpace(strings.TrimSuffix(n, " form")), FormSelf
	case n == "remove mask":
		// its line lists the masks it ends ("Dispel Mask Effects; Mortal,
		// Deceiver's, ..."): the name says it all
		d.FormKind, masks, d.Unread = FormEnd, true, nil
	default:
		race := maskRace(d.Unread)
		if race == "" {
			return
		}
		d.Form, d.FormKind = race, FormSelf
	}
	d.Target = TargetSelf
	d.Harmful = false
	unread := d.Unread[:0]
	for _, u := range d.Unread {
		if u != "form" && !strings.HasSuffix(u, " form") && !strings.Contains(u, "natural form") {
			unread = append(unread, u)
		}
	}
	d.Unread = unread
	if d.FormKind == FormEnd {
		d.Effects = append(d.Effects, SpellEffect{Kind: EffectEndIllusion, Masks: masks})
		d.Duration = 0
		return
	}
	if d.Form == "" {
		d.FormKind = FormNone
		return
	}
	d.Effects = append(d.Effects, SpellEffect{Kind: EffectIllusion})
	if d.Duration <= 0 {
		d.Duration = illusionDuration
	}
}

// readAppearance marks a spell whose lasting part changes how its bearer
// looks, for Remove Illusion and True Form to end (the user's ruling of
// 2026-10-29, after the test lanes' report found True Form ended no druid or
// shaman form: it "should remove all illusions, glamours, any form changing
// appearance", and the stats that came with them). Every illusion and glamour
// is one, whether or not this server has its body; so is a line with a
// "Form" or "<x> form" clause (the forms: "Self, Worf form, Increase AGI by
// 29, ..."), "Sprite Change" (Lich, Mortal Mask), "<x> Mask" (the rogue's
// masks) or "Transform into ...". A clause that ends one ("Removes Lich
// form") or says it changes nothing ("does not change your form"), or a
// description that says so ("without changing your form"), is none. Such
// clauses are read now, no longer unread.
func (d *SpellDef) readAppearance(desc string) {
	if d.FormKind == FormEnd {
		return
	}
	if d.FormKind == FormSelf || d.FormKind == FormGroup {
		d.Appearance = true
	}
	if ds := strings.ToLower(desc); strings.Contains(ds, "without chang") || strings.Contains(ds, "not change") {
		return
	}
	var unread []string
	for _, u := range d.Unread {
		if appearanceClause(u) {
			d.Appearance = true
			continue
		}
		unread = append(unread, u)
	}
	d.Unread = unread
}

// appearanceClause reports whether an unread clause is one of readAppearance's.
func appearanceClause(u string) bool {
	if strings.Contains(u, "not change") || strings.Contains(u, "without chang") {
		return false
	}
	for _, w := range []string{"remove", "revert", "dispel", "cure"} {
		if strings.HasPrefix(u, w) {
			return false
		}
	}
	return u == "form" || strings.HasSuffix(u, " form") || strings.HasPrefix(u, "sprite change") ||
		strings.HasSuffix(u, " mask") || strings.HasPrefix(u, "transform into")
}

// maskRace is the player race a line's "<race> mask" clause names ("dark
// elf mask", "erudite mask"), "" when none does.
func maskRace(unread []string) string {
	for _, u := range unread {
		if r, ok := strings.CutSuffix(u, " mask"); ok {
			if _, isRace := illusionRaces[strings.TrimSpace(r)]; isRace {
				return strings.TrimSpace(r)
			}
		}
	}
	return ""
}

// endMasksLocked is Remove Mask's landing: every mask on t comes off (an
// effect of a spell named "<x> Mask"), its body with it, reporting whether
// one went.
func (w *World) endMasksLocked(t *Player) bool {
	gone := false
	for _, e := range append([]*ActiveEffect(nil), t.Effects...) {
		if strings.HasSuffix(strings.ToLower(strings.TrimSpace(e.Name)), " mask") {
			w.removeEffectLocked(t, e.SpellID)
			log.Printf("Cast: %q ends on %s: its mask is gone", e.Name, t.Name)
			gone = true
		}
	}
	return gone
}

// endAppearancesLocked is Remove Illusion's and True Form's landing: every
// effect that changes how t looks comes off, an illusion's body and what it
// gave with it, reporting whether one went.
func (w *World) endAppearancesLocked(t *Player) bool {
	gone := false
	for _, e := range append([]*ActiveEffect(nil), t.Effects...) {
		if e.Form != 0 || e.Appearance {
			w.removeEffectLocked(t, e.SpellID)
			log.Printf("Cast: %q ends on %s: its look is gone", e.Name, t.Name)
			gone = true
		}
	}
	return gone
}

// raceSex keys a players' race body: the race on the network enum, the sex
// (0 male, 1 female), as charactermodel does.
type raceSex struct{ race, sex int }

// SetRaceModels names each player race's two bodies (charactermodel's race,
// sex and model), the bodies a race's illusion shows.
func (w *World) SetRaceModels(models map[[2]int]uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.raceModels = make(map[raceSex]uint32, len(models))
	for k, m := range models {
		w.raceModels[raceSex{k[0], k[1]}] = m
	}
}

// illusionBodyLocked is the body an illusion of form shows bearer in: a
// race's for the bearer's sex, at that race's players' size (racesize.go), or
// a creature's at its size. ok is false when this server has none.
func (w *World) illusionBodyLocked(form string, bearer *Player) (model uint32, size float32, ok bool) {
	if race, isRace := illusionRaces[form]; isRace {
		m := w.raceModels[raceSex{race, bearer.Sex}]
		return m, w.raceSizeLocked(race), m != 0
	}
	words := strings.Fields(form)
	last := ""
	if len(words) > 1 {
		last = words[len(words)-1]
	}
	// the creature of the form's name, else of its last word ("feral rat":
	// a rat), the most common body among this server's own NPCs of it
	if t := w.petTemplateLocked(form, last, -1); t != nil && t.ModelID != 0 {
		return uint32(t.ModelID), t.Size, true
	}
	return 0, 0, false
}

// Body is the model and scale the player is shown with: the newest
// illusion's (its body, at the creature's size or the player's own), else
// its own.
func (p *Player) Body() (model uint32, size float32) {
	if p.GMForm != 0 { // a GM's Change Sprite (gmchannel.go)
		if p.GMFormSize > 0 {
			return p.GMForm, p.GMFormSize
		}
		return p.GMForm, p.Size
	}
	for i := len(p.Effects) - 1; i >= 0; i-- {
		if e := p.Effects[i]; e.Form != 0 {
			if e.FormSize > 0 {
				return e.Form, e.FormSize
			}
			return e.Form, p.Size
		}
	}
	return uint32(p.ModelID), p.Size
}

// endIllusionsLocked takes every illusion off a player but keep's (a spell
// id, 0 none), reporting whether one went.
func (w *World) endIllusionsLocked(t *Player, keep int) bool {
	gone := false
	for _, e := range append([]*ActiveEffect(nil), t.Effects...) {
		if e.Form != 0 && e.SpellID != keep {
			w.removeEffectLocked(t, e.SpellID)
			log.Printf("Cast: %q ends on %s: the illusion is gone", e.Name, t.Name)
			gone = true
		}
	}
	return gone
}

// landIllusionLocked gives an illusion's lasting effect its body and takes
// any other illusion off its bearer first (a new illusion replaces the
// old). An illusion of a body this server lacks lands as its stats alone.
func (w *World) landIllusionLocked(cs *castState, t *Player, ae *ActiveEffect) {
	d := &cs.spell.Def
	w.endIllusionsLocked(t, cs.spell.SpellID)
	model, size, ok := w.illusionBodyLocked(d.Form, t)
	if !ok {
		log.Printf("Cast: %q on %s: no body of %q on this server: no illusion", cs.spell.Name, t.Name, d.Form)
		return
	}
	ae.Form, ae.FormSize = model, size
	log.Printf("Cast: %q shows %s as %q (model %#x)", cs.spell.Name, t.Name, d.Form, model)
}

// glamourTargetsLocked is whom a glamour lands on: the caster when it names
// no one or itself, a member of the caster's group in its range, else no
// one, with the line refusing it.
func (w *World) glamourTargetsLocked(sess *session.Session, p *Player, spell *CastSpell, targetObjectID uint32) ([]EntityID, string) {
	if targetObjectID == 0 || targetObjectID == 0xFFFFFFFF || targetObjectID == sess.PlayerObjectID() {
		return []EntityID{p.ServerID}, ""
	}
	ent, ok := w.findEntityByObjectIDLocked(sess, targetObjectID)
	if !ok {
		return []EntityID{p.ServerID}, ""
	}
	t, isPlayer := ent.(*Player)
	if !isPlayer || t.IsDead() {
		return nil, castNotInGroup
	}
	if t == p {
		return []EntityID{p.ServerID}, ""
	}
	if p.GroupID == 0 || t.GroupID != p.GroupID {
		return nil, castNotInGroup
	}
	if t.World != p.World || outOfRange(p, t.X, t.Y, t.Z, spell.Range) {
		return nil, castOutOfRange
	}
	return []EntityID{t.ServerID}, ""
}
