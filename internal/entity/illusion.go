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
	FormEnd            // "Remove Illusion", "True Form": the caster's own body back
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
// the same and are no longer counted unread.
func (d *SpellDef) readIllusion(name, desc string) {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.TrimSpace(strings.TrimSuffix(n, "+"))
	ds := strings.ToLower(desc)
	switch {
	case strings.HasPrefix(n, "illusion:"):
		d.Form, d.FormKind = strings.TrimSpace(strings.TrimPrefix(n, "illusion:")), FormSelf
	case strings.HasPrefix(n, "glamour:"):
		d.Form, d.FormKind = strings.TrimSpace(strings.TrimPrefix(n, "glamour:")), FormGroup
	case n == "remove illusion" || n == "true form":
		d.FormKind = FormEnd
	case strings.HasSuffix(n, " form") && !strings.Contains(ds, "without changing") &&
		(strings.Contains(ds, "assume the form") || strings.Contains(ds, "illusion")):
		d.Form, d.FormKind = strings.TrimSpace(strings.TrimSuffix(n, " form")), FormSelf
	default:
		return
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
		d.Effects = append(d.Effects, SpellEffect{Kind: EffectEndIllusion})
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
// race's for the bearer's sex, at the bearer's own scale (size 0), or a
// creature's at its size. ok is false when this server has none.
func (w *World) illusionBodyLocked(form string, bearer *Player) (model uint32, size float32, ok bool) {
	if race, isRace := illusionRaces[form]; isRace {
		m := w.raceModels[raceSex{race, bearer.Sex}]
		return m, 0, m != 0
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
