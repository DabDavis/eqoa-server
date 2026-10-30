package entity

import (
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// area.go — area spells: a spell that lands on everything within a radius of
// its target or of its caster, up to a cap (jib's review, F3 pass 19). Until
// now the wizard's storms and maelstroms, the magician's storms, the warrior's
// shouts and the cleric's Undead line landed on their one target.
//
// Which spells, read from their lines as EQOA-Data's lists word them and
// spelleffect keeps them (ReadSpellArea), and where a line says nothing, by
// name:
//
//   - "Target, Decrease Hitpoints by 210 aoe Decrease Hitpoints by 90": the
//     target takes the first figure, everything round it the one after the
//     area word (the storms and maelstroms, the Undead line), also worded
//     "Aoe by 8", "AOE by Decrease Hitpoints by 16", "aoe 390" and "AE 856".
//     A scale the line gives one share and not the other is the spell's, so
//     both take it (the maelstroms' DEX after the area's figure, the furies'
//     before it); a share with its own keeps it ("Decrease HP by 160 ;
//     Increase 30% based on WIS, AOE Decrease HP by 88 ; Increase 15% based
//     on WIS": until now the target's 30% was read as the rest's 15%).
//   - a bare "AOE" ("Target, Increase Hate by 100 ; Increase 10% based on
//     STR, AOE", the shouts): the whole spell on everything in the area.
//   - a group's radius ("Group, buff Hp by 63 Modified 25% based on CHA,
//     Radius 15", "15rad", Group Valor's "30rad"): the cleric's group heals,
//     "a healing wave that washes over the caster's group" (Wave of Health's
//     own description), and by name the ranks whose lines give none (Tide and
//     Wave of Health and of Glory). A radius on a line of one target is not an
//     area (Valor's "30rad", beside Group Valor's).
//   - "PB AE", "point blank", "around the caster", "around the target": no
//     list has them; a GM's words.
//   - Lightning Maelstrom, whose line names no area: every other maelstrom is
//     a storm's figures with a DEX scale (Fire Storm's and Fire Maelstrom's 82
//     and 36 ... Pillar of Flame's and Fiery Maelstrom's 764 and 326), and the
//     wizard's spell of its 846 is Lightning Column, "846 aoe 360", so its
//     others take 360 (ours, by that pattern).
//
// Where the area lies: round its caster for a group's spell, a Self line, the
// PB words and a taunt (a shout, by its name, or a line of hate alone:
// EQOA-Data's masteries list calls Hero Shout, whose ranks turn the warrior's
// taunts into the shouts, an "AoE taunt"); round its target otherwise.
//
// Who is in it is chosen as the cast begins, as its target is, for the cast's
// 0x00B1 names them; each takes its share at the landing if it is still
// there, alive and, a creature, one anyone may attack (cast.go tests no range
// at the landing for the target either):
//
//   - a harmful spell's are creatures: never a player, nor a pet or a charmed
//     creature, nor an NPC with the NPC bit (0x80: no one may attack it), nor
//     one across the water's edge from the caster (water.go), nor for a spell
//     for undead alone anything living (undead.go). Each is the caster's hit
//     on it, as a single target's is: its own roll, its hate, its call to the
//     helpers of its faction (socialaggro.go), its kill;
//   - a group's are the caster's group alive in its world and their pets, the
//     caster its own target;
//   - any other spell that helps (no list has one: a GM's row) lands on every
//     player and player's pet in it.
//
// The nearest come first, the cast's own target counting, up to the cap.
//
// What players see: the cast's one 0x00B1, as before, now naming its others in
// the count and the ids CLIENT's CastingCombat reads; it hands them to the
// visual as its secondary targets (VISpellEffectSystem::Play), each playing
// the effect's Area event list (VISpellEventDriver). A 0x00B1 names seven at
// most: CLIENT copies them into int actors[8] and ends the list with -1. Each
// hit's figure is its own 0x00DB, as a single target's.
//
// Ours, not measured (no capture holds an area spell, and no list gives an
// area's radius but the group heals'): the radii, the cap, choosing the others
// as the cast begins, Lightning Maelstrom's 360 and the group heals by name.
// Rampage, which EQOA-Data's masteries list calls a "High-damage AoE taunt", is
// left a spell of one target: its line says nothing of an area, and its
// record's recast is 3 s. A GM sets any spell's centre, radius and cap (table
// spellarea, the GM panel's "Spell areas": AreaRule).

// The radii and the cap: ours.
const (
	// AreaRadius is how far round its target an area spell reaches, and a
	// group's or a heal's round its caster when its line gives none: the
	// cleric's group heals' "Radius 15", the one radius the lists give.
	AreaRadius float32 = 15
	// ShoutRadius is how far round its caster a harmful spell reaches: the
	// fight round the caster and those beside it, not the next camp (a
	// shout's hate starts a fight with all it reaches).
	ShoutRadius float32 = 10
	// AreaMaxTargets is the most an area spell lands on, its own target among
	// them: the target and the seven others a 0x00B1 can name.
	AreaMaxTargets = 8
	// areaShown is the most others a 0x00B1 names: CLIENT's CastingCombat
	// copies them into int actors[8] and ends the list with -1, so an eighth
	// would write past it.
	areaShown = 7
	// areaRadiusMost and areaMaxMost bound a GM's row.
	areaRadiusMost float32 = 100
	areaMaxMost            = 64
)

// AreaCentre is where an area spell's area lies.
type AreaCentre int

const (
	AreaNone     AreaCentre = iota // a spell of one target
	AreaOnTarget                   // round its target ("Target AE")
	AreaOnCaster                   // round its caster ("PB AE", a taunt, a group's)
)

// SpellArea is an area spell's area: its line's reading (ReadSpellArea) and a
// GM's row over it (With).
type SpellArea struct {
	Centre AreaCentre
	Radius float32 // its line's or its row's; 0: its kind's (radiusFor)
	Max    int     // its row's cap; 0: AreaMaxTargets
	// Effects is what its others take; nil: the spell's own
	Effects []SpellEffect
	// Why is what made it one, for the log
	Why string
}

// AreaRule is a GM's row of table spellarea: Centre AreaRuleLine as the
// spell's line reads, AreaRuleTarget round its target, AreaRuleCaster round
// its caster, AreaRuleNone not an area spell; Radius and Max 0 as its line or
// its kind has them.
type AreaRule struct {
	Centre int
	Radius float32
	Max    int
}

// The centres a spellarea row names.
const (
	AreaRuleNone   = -1
	AreaRuleLine   = 0
	AreaRuleTarget = 1
	AreaRuleCaster = 2
)

// With is the area as a GM's row has it.
func (a SpellArea) With(r AreaRule) SpellArea {
	switch r.Centre {
	case AreaRuleNone:
		return SpellArea{}
	case AreaRuleTarget:
		a.Centre = AreaOnTarget
	case AreaRuleCaster:
		a.Centre = AreaOnCaster
	}
	if a.Centre == AreaNone {
		return a
	}
	if r.Radius > 0 {
		a.Radius = min(r.Radius, areaRadiusMost)
	}
	if r.Max > 0 {
		a.Max = min(r.Max, areaMaxMost)
	}
	a.Why = strings.TrimSpace(a.Why + " (a GM's row)")
	return a
}

// radiusFor is how far the area reaches: its own radius, else its kind's.
func (a SpellArea) radiusFor(harmful bool) float32 {
	switch {
	case a.Radius > 0:
		return a.Radius
	case harmful && a.Centre == AreaOnCaster:
		return ShoutRadius
	}
	return AreaRadius
}

// most is the most it lands on, its own target among them.
func (a SpellArea) most() int {
	if a.Max > 0 {
		return a.Max
	}
	return AreaMaxTargets
}

var (
	// reAreaWord is the area word, where the target's share ends: "aoe",
	// "ae" (Thunderous Fury's "AE 856"), a GM's "PB AE"; not "group/aoe" or
	// "(aoe)" in an epic's notes
	reAreaWord = regexp.MustCompile(`(^|[\s,;])(?:pb\s*)?(?:aoe|ae)($|[\s,;])`)
	// reTypeNote is a note on an area's kind after its figure: "Decrease
	// Hitpoints by 90 - Lightning Based AoE"
	reTypeNote = regexp.MustCompile(`\s+-\s+[a-z]+ based aoe\b`)
	// reHasScale is a scale in a share's words
	reHasScale = regexp.MustCompile(`\d+\s*%\s*(?:based on|mod by|modified by|by)\s+[a-z]`)
	// reRadius is a radius in a line: "Radius 15", "15rad", "30rad"
	reRadius = regexp.MustCompile(`\bradius\s*(\d+(?:\.\d+)?)\b|\b(\d+(?:\.\d+)?)\s*rad(?:ius)?\b`)
	// reOnCaster and reOnTarget are a GM's words for where the area lies
	reOnCaster = regexp.MustCompile(`\bpb\s*(?:aoe|ae)\b|\bpoint[- ]blank\b|\baround (?:the )?(?:caster|you|self)\b`)
	reOnTarget = regexp.MustCompile(`\btarget\s+(?:aoe|ae)\b|\baround (?:the )?(?:target|it)\b`)
	// reShout is a shout: its caster's own voice
	reShout = regexp.MustCompile(`\bshouts?\b`)
	// reAreaFigure is an area's figure alone: "by 8", "856", "by 50 to 60"
	reAreaFigure = regexp.MustCompile(`^(?:by\s+)?(\d+)(?:\s+to\s+(\d+))?$`)
)

// areaNames are the area spells whose lines say nothing of it, by name (the
// file's doc): a group's, round its caster, or a harmful one round its
// target, and what its others take in an area's words ("": the spell's own).
var areaNames = map[string]struct {
	group bool
	words string
}{
	"lightning maelstrom": {words: "decrease hitpoints by 360"},
	"tide of health":      {group: true},
	"tide of glory":       {group: true},
	"wave of health":      {group: true},
	"wave of glory":       {group: true},
}

// ReadSpellArea is a spell's area as its line (or where the line says
// nothing, its name) gives it, the file's doc; AreaNone for a spell of one
// target. d is the line read (ParseSpellNamed).
func ReadSpellArea(name, line string, d SpellDef) SpellArea {
	if strings.TrimSpace(line) == "" {
		line = DefaultSpellLine(name)
	}
	// nothing to spread (an epic's notes), or a pet, a charm, an illusion
	if len(d.Effects) == 0 || d.Summons() || d.Charms() || d.FormKind != FormNone {
		return SpellArea{}
	}
	text := reTypeNote.ReplaceAllString(strings.ToLower(strings.TrimSpace(line)), "")
	n := strings.ToLower(strings.TrimSpace(name))
	named, byName := areaNames[n]
	words, worded := areaWords(text)
	if d.Target == TargetGroup {
		switch m := reRadius.FindStringSubmatch(text); {
		case m != nil:
			return SpellArea{Centre: AreaOnCaster, Radius: radiusOf(m), Why: "a group's " + strings.TrimSpace(m[0])}
		case worded:
			return SpellArea{Centre: AreaOnCaster, Effects: areaShare(words, d), Why: "a group's area"}
		case byName && named.group:
			return SpellArea{Centre: AreaOnCaster, Why: "a group heal, by its name"}
		}
		return SpellArea{}
	}
	switch {
	case worded || reOnCaster.MatchString(text) || reOnTarget.MatchString(text):
		return SpellArea{Centre: areaCentre(n, text, d), Effects: areaShare(words, d), Why: "its line"}
	case byName && !named.group && d.Harmful:
		return SpellArea{Centre: AreaOnTarget, Effects: areaShare(named.words, d), Why: "its name"}
	}
	return SpellArea{}
}

// radiusOf is a reRadius match's figure.
func radiusOf(m []string) float32 {
	s := m[1]
	if s == "" {
		s = m[2]
	}
	r, _ := strconv.ParseFloat(s, 32)
	return float32(r)
}

// areaCentre is where a spell's area lies (the file's doc).
func areaCentre(name, text string, d SpellDef) AreaCentre {
	switch {
	case reOnCaster.MatchString(text):
		return AreaOnCaster
	case reOnTarget.MatchString(text):
		return AreaOnTarget
	case d.Target == TargetSelf || reShout.MatchString(name) || hateAlone(d):
		return AreaOnCaster
	}
	return AreaOnTarget
}

// hateAlone reports a line of hate alone: a taunt.
func hateAlone(d SpellDef) bool {
	for _, e := range d.Effects {
		if e.Kind != EffectHate {
			return false
		}
	}
	return len(d.Effects) > 0
}

// areaWords is what a line says after its area word, to the end of that
// clause ("decrease hitpoints by 36 ; increase 30% based on dex", "by 8",
// "856"; "" for a bare AOE), and whether it has an area word. text is lower
// case.
func areaWords(text string) (string, bool) {
	loc := reAreaWord.FindStringSubmatchIndex(text)
	if loc == nil {
		return "", false
	}
	rest := text[loc[4]:]
	if i := strings.IndexByte(rest, ','); i >= 0 {
		rest = rest[:i]
	}
	return strings.Trim(rest, " ;."), true
}

// targetsShare is a line as its target's share reads (ParseSpellText), its
// area's words taken out: "target, decrease hitpoints by 210 aoe decrease
// hitpoints by 90 ; increase 30% based on dex" is "target, decrease hitpoints
// by 210 ; increase 30% based on dex". A scale after the area's figure is the
// spell's, the target's too, unless the target's share has its own (the
// file's doc). text is lower case.
func targetsShare(text string) string {
	text = reTypeNote.ReplaceAllString(text, "")
	loc := reAreaWord.FindStringSubmatchIndex(text)
	if loc == nil {
		return text
	}
	before := strings.TrimRight(text[:loc[3]], " ,;")
	clause, after := text[loc[4]:], ""
	if i := strings.IndexByte(clause, ','); i >= 0 {
		clause, after = clause[:i], clause[i:]
	}
	own := before
	if i := strings.LastIndexByte(own, ','); i >= 0 {
		own = own[i+1:]
	}
	if i := strings.IndexByte(clause, ';'); i >= 0 && !reHasScale.MatchString(own) {
		before += " " + strings.TrimSpace(clause[i:])
	}
	return before + after
}

// areaShare is what an area's others take, from its words (areaWords): nil
// for the spell's own; a figure alone ("by 8", "AE 856") is the target's hit
// at that figure. A scale its words leave out is the target's hit's (the
// file's doc).
func areaShare(words string, d SpellDef) []SpellEffect {
	words = strings.TrimSpace(words)
	if words == "" {
		return nil
	}
	// the target's hit: its first effect with a figure that is not its
	// caster's own
	hit := -1
	for i, e := range d.Effects {
		if !e.Self && e.Amount != 0 {
			hit = i
			break
		}
	}
	if m := reAreaFigure.FindStringSubmatch(words); m != nil {
		if hit < 0 {
			return nil
		}
		e := d.Effects[hit]
		lo := atoi(m[1])
		hi := max(atoi(m[2]), lo)
		if e.Amount < 0 {
			e.Amount, e.AmountMax = -hi, -lo
		} else {
			e.Amount, e.AmountMax = lo, hi
		}
		return []SpellEffect{e}
	}
	// "by Decrease Hitpoints by 16" (Minor Energy Storm)
	share := ParseSpellText("target, " + strings.TrimPrefix(words, "by ")).Effects
	if len(share) == 0 {
		return nil
	}
	if hit >= 0 && d.Effects[hit].Scale != StatNone {
		for i := range share {
			if share[i].Scale == StatNone && share[i].Kind == d.Effects[hit].Kind {
				share[i].Scale, share[i].ScalePct = d.Effects[hit].Scale, d.Effects[hit].ScalePct
			}
		}
	}
	return share
}

// areaNear is one of an area's others and how far it stands from its centre.
type areaNear struct {
	id   EntityID
	dist float32
}

// areaStartLocked chooses an area spell's others as its cast begins
// (cs.others, the nearest first, the cast's own targets counting to the cap),
// for its 0x00B1 names them. A group's are its members and their pets within
// its radius of the caster, the caster its own target; a harmful spell lands
// on no player, so one cast round its caster with no target (a shout with no
// target word) has none of its own. Must be called with the lock held.
func (w *World) areaStartLocked(p *Player, cs *castState, now time.Time) {
	a := &cs.spell.Area
	if a.Centre == AreaNone || w.SpatialIndex == nil {
		return
	}
	d := &cs.spell.Def
	r := a.radiusFor(d.Harmful)
	var near []areaNear
	centre := "caster"
	// a group's by its record's scope or its line's word
	if (cs.spell.Scope == 2 || d.Target == TargetGroup) && !d.Harmful {
		cs.targets = []EntityID{p.ServerID}
		near = w.groupNearLocked(p, r)
	} else {
		if d.Harmful {
			var own []EntityID
			for _, id := range cs.targets {
				if _, player := w.Players[id]; !player {
					own = append(own, id)
				}
			}
			cs.targets = own
		}
		cx, cy, cz := p.X, p.Y, p.Z
		if a.Centre == AreaOnTarget && len(cs.targets) > 0 {
			centre = "target"
			if t := w.Players[cs.targets[0]]; t != nil {
				cx, cy, cz = t.X, t.Y, t.Z
			} else if npc := w.NPCs[cs.targets[0]]; npc != nil {
				cx, cy, cz = npc.X, npc.Y, npc.Z
			}
		}
		if d.Harmful {
			near = w.areaCreaturesLocked(p, cs, cx, cy, cz, r, now)
		} else {
			near = w.areaFriendsLocked(p, cs, cx, cy, cz, r)
		}
	}
	sort.SliceStable(near, func(i, j int) bool { return near[i].dist < near[j].dist })
	found := len(near)
	if room := max(a.most()-len(cs.targets), 0); len(near) > room {
		near = near[:room]
	}
	cs.others = make([]EntityID, 0, len(near))
	for _, n := range near {
		cs.others = append(cs.others, n.id)
	}
	log.Printf("Cast: %s's %q is an area spell (%s) round its %s within %.0f: %d more of %d found, %d at most in all",
		p.Name, cs.spell.Name, a.Why, centre, r, len(cs.others), found, a.most())
}

// areaCreaturesLocked is the creatures a harmful area's others may be (the
// file's doc) within r of (cx, cy, cz), the cast's own targets left out. Must
// be called with the lock held.
func (w *World) areaCreaturesLocked(p *Player, cs *castState, cx, cy, cz, r float32, now time.Time) []areaNear {
	undead := cs.spell.Def.undeadOnly()
	var out []areaNear
	for _, id := range w.SpatialIndex.QueryRadius(cx, cz, r) {
		npc := w.NPCs[id]
		if npc == nil || areaHas(cs.targets, id) || npc.Dead || npc.Owner != 0 || npc.NPCType&0x80 != 0 ||
			npc.World != p.World || npc.hiddenAt(now) {
			continue
		}
		if w.apartLocked(p, npc) || undead && !w.undeadLocked(npc) { // water.go, undead.go
			continue
		}
		if dd := dist3(npc.X, npc.Y, npc.Z, cx, cy, cz); dd <= r {
			out = append(out, areaNear{id, dd})
		}
	}
	return out
}

// areaFriendsLocked is the players and players' pets a helping area's others
// may be within r of (cx, cy, cz), the cast's own targets left out. Must be
// called with the lock held.
func (w *World) areaFriendsLocked(p *Player, cs *castState, cx, cy, cz, r float32) []areaNear {
	var out []areaNear
	for _, id := range w.SpatialIndex.QueryRadius(cx, cz, r) {
		if areaHas(cs.targets, id) {
			continue
		}
		if t := w.Players[id]; t != nil {
			if !t.IsDead() && t.World == p.World {
				if dd := dist3(t.X, t.Y, t.Z, cx, cy, cz); dd <= r {
					out = append(out, areaNear{id, dd})
				}
			}
			continue
		}
		if pet := w.NPCs[id]; pet != nil && pet.Owner != 0 && w.Players[pet.Owner] != nil && !pet.Dead && pet.World == p.World {
			if dd := dist3(pet.X, pet.Y, pet.Z, cx, cy, cz); dd <= r {
				out = append(out, areaNear{id, dd})
			}
		}
	}
	return out
}

// groupNearLocked is a group's area beside its caster: each member alive in
// its world within r of it, and each member's pet (its own too). Must be
// called with the lock held.
func (w *World) groupNearLocked(p *Player, r float32) []areaNear {
	members := w.groupMembersLocked(p)
	if len(members) == 0 {
		members = []*Player{p}
	}
	var out []areaNear
	add := func(id EntityID, world uint8, x, y, z float32) {
		if dd := dist3(x, y, z, p.X, p.Y, p.Z); world == p.World && dd <= r {
			out = append(out, areaNear{id, dd})
		}
	}
	for _, m := range members {
		if m != p && !m.IsDead() {
			add(m.ServerID, m.World, m.X, m.Y, m.Z)
		}
		if pet := w.petOfLocked(m); pet != nil {
			add(pet.ServerID, pet.World, pet.X, pet.Y, pet.Z)
		}
	}
	return out
}

func areaHas(ids []EntityID, id EntityID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// landOthersLocked lands an area spell on its others, each its share, after
// its own targets (cast.go's landLocked). Each must still be there to take it:
// alive, and a creature one anyone may still attack (not charmed since). Must
// be called with the lock held.
func (w *World) landOthersLocked(cs *castState, caster *Player, casterSess *session.Session, now time.Time) {
	if len(cs.others) == 0 || caster.IsDead() {
		return
	}
	share := cs
	if e := cs.spell.Area.Effects; e != nil {
		c := *cs
		c.spell.Def.Effects = e
		share = &c
	}
	harmful := cs.spell.Def.Harmful
	for _, id := range cs.others {
		if t := w.Players[id]; t != nil {
			if !harmful && !t.IsDead() {
				w.landOnPlayerLocked(share, caster, casterSess, t, now)
			}
			continue
		}
		npc := w.NPCs[id]
		if npc == nil || npc.Dead {
			continue
		}
		switch {
		case npc.Owner != 0:
			if !harmful {
				w.landOnPetLocked(share, caster, casterSess, npc, now) // pet.go
			}
		case harmful && npc.NPCType&0x80 == 0:
			w.landOnNPCLocked(share, caster, casterSess, npc, now)
		}
	}
}

// areaSeenLocked is an area's others as session s knows them, for its 0x00B1:
// those it sees, the nearest first, areaShown at most. Must be called with the
// lock held.
func (w *World) areaSeenLocked(s *session.Session, others []EntityID) []uint32 {
	var out []uint32
	for _, id := range others {
		if len(out) == areaShown {
			break
		}
		if o := w.objectIDSeenByLocked(s, id); o != 0 {
			out = append(out, o)
		}
	}
	return out
}
