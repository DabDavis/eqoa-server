package entity

import (
	"fmt"
	"log"
	"math/rand"
	"sort"
	"time"

	"github.com/eqoa/server/internal/database"
)

// pettap.go — a necromancer's pet's life tap (F3 20261025-215804, jib:
// "Necromancer pets starting at the level 20 pet and up should have a level
// appropriate life tap that they do periodically, approximately a 30 second
// recast. They should use it when its up while still meleeing. So if its a
// level 20 pet, and necromancers get a lifetap at level 20, it should be
// using that spell once every 30ish seconds, including the small heal.").
//
// The taps are the necromancer's own spells, read out of classspells,
// spellpattern and spelleffect at the start and after a GM's edit of any of
// them (LoadPetTaps): every single-target spell whose line takes hit points
// for its caster (lifetap.go). A pet a necromancer summoned (a creature it
// charmed is not one) of level petTapLevel or more casts the highest of them
// at or below its level whenever it is up and the pet stands in reach of
// what it fights: the spell's visual from the pet (0x00B1, no cast time),
// the line's hit on its target as one of the pet's own (its owner's figure,
// hate, and a kill's XP and faction hits, as the pet's melee), and as much
// back on the pet's hit points, told to the owner over the pet. Then not
// again for petTapEvery. The line's share of its caster's stat is left out:
// a pet has none.
//
// The 30 seconds and the first pet that taps (the level 20 one, Wilting
// Flesh's zombie) are jib's; the rest is the server's own: no recording has a
// pet casting (Sandstorm's one recorded pet, a Fire Elemental, cast
// nothing). A level 20 pet's tap is Siphon Life, the necromancer's tap of
// level 16 (EQOA-Data's lists have none at 20).

const (
	petTapLevel      = 20
	petTapEvery      = 30 * time.Second
	classNecromancer = 11
)

// PetTap is a life tap a necromancer's pet casts: its spell's level and
// name, the line's hit, and its visual.
type PetTap struct {
	Level int
	Name  string
	Hit   SpellEffect // the hit (Amount < 0), taken for its caster
	FX    uint32
}

// petTapsOf is the taps among a class's spells, by level.
func petTapsOf(lines []database.ClassSpellLine) []PetTap {
	var taps []PetTap
	seen := map[string]bool{}
	for _, l := range lines {
		if l.Scope != 1 {
			continue
		}
		d := ParseSpellNamed(l.Name, l.Line, l.Desc)
		if d.Target != TargetSingle {
			continue
		}
		for _, e := range d.Effects {
			if e.Kind != EffectHP || e.Amount >= 0 || e.Drain != DrainCaster || e.Self {
				continue
			}
			key := fmt.Sprintf("%d %s", l.Level, l.Name)
			if seen[key] {
				break
			}
			seen[key] = true
			fx := l.FX
			if fx == 0 {
				fx, _ = StandInVisual(d)
			}
			taps = append(taps, PetTap{Level: l.Level, Name: l.Name, Hit: e, FX: fx})
			break
		}
	}
	sort.SliceStable(taps, func(i, j int) bool { return taps[i].Level < taps[j].Level })
	return taps
}

// LoadPetTaps reads the necromancer's taps into the world, at the start and
// after a GM's edit of a spell. Call it without the database held. It
// reports how many there are.
func (w *World) LoadPetTaps(db *database.SQLiteDB) (int, error) {
	lines, err := db.ClassSpellLines(classNecromancer)
	if err != nil {
		return 0, fmt.Errorf("the necromancer's spells: %w", err)
	}
	taps := petTapsOf(lines)
	w.mu.Lock()
	w.petTaps = taps
	w.mu.Unlock()
	return len(taps), nil
}

// PetTapSummary says which taps the pets cast, for the log.
func (w *World) PetTapSummary() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if len(w.petTaps) == 0 {
		return "no necromancer's life tap: its pets cast none"
	}
	s := ""
	for i, t := range w.petTaps {
		if i > 0 {
			s += ", "
		}
		s += fmt.Sprintf("%s (%d, %d)", t.Name, t.Level, -t.Hit.Amount)
	}
	return fmt.Sprintf("a necromancer's pet of level %d+ taps every %v: %s", petTapLevel, petTapEvery, s)
}

// petTapForLocked is the tap a pet of that level casts, or nil.
func (w *World) petTapForLocked(level int) *PetTap {
	var best *PetTap
	for i := range w.petTaps {
		if w.petTaps[i].Level <= level {
			best = &w.petTaps[i]
		}
	}
	return best
}

// petTapIntn is the taps' dice (a test's own).
var petTapIntn = rand.Intn

// petTapLocked casts the pet's tap on its target when the pet taps and the
// tap is up. It reports a kill.
func (w *World) petTapLocked(ps *petState, t *NPC, now time.Time) bool {
	pet := ps.npc
	if pet.Charm != nil || ps.owner.Class != classNecromancer || pet.Level < petTapLevel || now.Before(ps.tapNext) {
		return false
	}
	tap := w.petTapForLocked(pet.Level)
	if tap == nil {
		return false
	}
	ps.tapNext = now.Add(petTapEvery)
	n := -tap.Hit.AmountMax
	if tap.Hit.AmountMax > tap.Hit.Amount {
		n += petTapIntn(tap.Hit.AmountMax - tap.Hit.Amount + 1)
	}
	w.broadcastNPCSwingLocked(pet, t.ServerID, 0, tap.FX)
	log.Printf("Pet: %s's %q casts %s on %s for %d", ps.owner.Name, pet.Name, tap.Name, t.Name, n)
	killed := w.petDamageLocked(ps, t, n, now)
	if pet.HP < pet.MaxHP {
		pet.HP = min(pet.HP+n, pet.MaxHP)
		if sess := w.findSessionByEntityIDLocked(ps.owner.ServerID); sess != nil {
			if id := w.objectIDForEntityLocked(sess, pet.ServerID); id != 0 {
				w.sendDamageIndication(sess, sess.PlayerObjectID(), n, id)
			}
		}
	}
	return killed
}
