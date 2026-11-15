package entity

import (
	"math/bits"
	"sort"
	"strings"
)

// mercmaster.go — a merc's Master Class (the owner, 2026-11-15: "if 55+
// choose the merc's master class tag (which comes with the master class
// abilities)", then "tinker is racial master class, so account for archetype
// master class, racial, etc. magician mercs can be aero, pyro, hydro, geo
// mancers also"; Hogknight's ask: "is it possible to give the gnome mercs a
// tinker pet option?").
//
// A merc of MercMasterLevel or more may hold one Master Class (cmcatalog
// category 4, CMCategoryMaster), any a player of its class and race is
// offered (CMRank.OfferedTo: the classes and races bit sets, as the mastery
// window lists them), grouped as the menu asks them (MercMasterKind):
//
//   - its class's own: a rank of one class (the magician's Aeromancer,
//     Pyromancer, Hydromancer, Geomancer and Sage; the necromancer's Lich);
//   - its race's: a rank for some races only (racialRank: the gnome's
//     Tinker, the barbarian's Berserker, the halfling's Deputy);
//   - an archetype's: a rank of several classes, every race's (the
//     were-hunters').
//
// It holds the Master Class as a player who bought every rank of it would:
// each rank up to its level (the first at 55, the abilities at 60), what
// they add to its stats (CMRank.Effects on its body, refreshCMLocked: a
// player's figures) and its book changed as a purchase changes a player's
// (CMAddBook: Exorcise's lines swapped, the Sensei's Healing Touch line in
// place of the Mend line). What a player must have before the purchase that
// a merc never has (other masteries' ranks bought, base stats trained up, a
// quest done) is not asked of it: it is hired at its level with the Master
// Class. An ability a rank needs in its book (Slayer's Curse of Bertoxxulous
// needs Hand of Bertoxxulous) is asked, since it is what the rank replaces;
// a rank that is another look of a summoning it already has (Tinker's
// Construct (Rat) and (Drachnid)) is left out (handlers' mercmaster.go).
//
// A Master Class's summoning (Tinker's Construct, the Death Knight's
// Servant, the Animator's Grand Animation) is the merc's pet before its
// class's book's (mercPetSpell): one chose the Master Class for it. A kind
// its hirer picked (/merc pet) still wins. A racial ability's summoning (the
// gnome's Invention from 6, the troll's Swamp Friends, the Mermaid's
// Aquatic Birthright: MercKit.RacialPets) is summoned only by a merc whose
// book has no other: a gnome warrior's Invention, not a gnome magician's.

// MercMasterLevel is the least level a merc takes a Master Class at: the
// Master Classes' first ranks'.
const MercMasterLevel = 55

// The kinds of a Master Class, as the menu groups them.
const (
	MercMasterOfClass     = "class"
	MercMasterOfRace      = "racial"
	MercMasterOfArchetype = "archetype"
)

// MercMasterClass is a Master Class a merc may hold: its name (cmcatalog's
// mastery), its kind and its ranks for the merc's class and race, by rank.
type MercMasterClass struct {
	Name  string
	Kind  string
	Ranks []*CMRank
}

// MercMasterKind is a Master Class rank's kind (the file's doc).
func MercMasterKind(r *CMRank) string {
	switch {
	case r.racialRank():
		return MercMasterOfRace
	case bits.OnesCount(uint(r.Classes)) == 1:
		return MercMasterOfClass
	}
	return MercMasterOfArchetype
}

// mercMasterKindOrder is the menu's order of the kinds.
var mercMasterKindOrder = map[string]int{MercMasterOfClass: 0, MercMasterOfRace: 1, MercMasterOfArchetype: 2}

// MercMasterClasses is every Master Class a merc of the class and race may
// hold, by kind (its class's, its race's, an archetype's) then name.
func (w *World) MercMasterClasses(class, race int) []MercMasterClass {
	w.mu.RLock()
	cat := w.cms
	w.mu.RUnlock()
	if cat == nil {
		return nil
	}
	at := map[string]int{}
	var out []MercMasterClass
	for _, r := range cat.order {
		if r.Category != CMCategoryMaster || !r.OfferedTo(class, race) {
			continue
		}
		name := r.Mastery
		if name == "" {
			name = r.Name
		}
		i, ok := at[strings.ToLower(name)]
		if !ok {
			i = len(out)
			at[strings.ToLower(name)] = i
			out = append(out, MercMasterClass{Name: name, Kind: MercMasterKind(r)})
		}
		if r.Parent == 0 {
			out[i].Kind = MercMasterKind(r) // its first rank's
		}
		out[i].Ranks = append(out[i].Ranks, r)
	}
	for i := range out {
		sort.SliceStable(out[i].Ranks, func(a, b int) bool {
			ra, rb := out[i].Ranks[a], out[i].Ranks[b]
			return ra.Rank < rb.Rank || ra.Rank == rb.Rank && ra.ID < rb.ID
		})
	}
	sort.SliceStable(out, func(a, b int) bool {
		ka, kb := mercMasterKindOrder[out[a].Kind], mercMasterKindOrder[out[b].Kind]
		return ka < kb || ka == kb && out[a].Name < out[b].Name
	})
	return out
}

// MercMasterNamed is the Master Class of the list a word names: its name
// (any case), else the only one whose name begins with it.
func MercMasterNamed(list []MercMasterClass, word string) (MercMasterClass, bool) {
	word = strings.ToLower(strings.TrimSpace(word))
	if word == "" {
		return MercMasterClass{}, false
	}
	var found []MercMasterClass
	for _, mc := range list {
		n := strings.ToLower(mc.Name)
		if n == word {
			return mc, true
		}
		if strings.HasPrefix(n, word) {
			found = append(found, mc)
		}
	}
	if len(found) != 1 {
		return MercMasterClass{}, false
	}
	return found[0], true
}

// MercRacialRanks is the racial abilities (cmcatalog category 1) a player
// of the class and race buys by the level that give a summoning: the
// handlers keep those whose spells summon (MercKit.RacialPets).
func (w *World) MercRacialRanks(class, race, level int) []*CMRank {
	w.mu.RLock()
	cat := w.cms
	w.mu.RUnlock()
	if cat == nil {
		return nil
	}
	var out []*CMRank
	for _, r := range cat.order {
		if r.Category == 1 && r.Level <= level && len(r.Adds) > 0 && r.OfferedTo(class, race) {
			out = append(out, r)
		}
	}
	return out
}

// mercMasterBody puts the kit's Master Class's ranks on its body: what they
// add to its stats, as a player's ranks (refreshCMLocked).
func mercMasterBody(p *Player, k *MercKit) {
	if len(k.MasterRanks) == 0 {
		return
	}
	cat := &cmCatalog{byID: make(map[int]*CMRank, len(k.MasterRanks))}
	p.CMs = make(map[int]int, len(k.MasterRanks))
	for _, r := range k.MasterRanks {
		cat.byID[r.ID] = r
		p.CMs[r.ID] = r.Cost
	}
	p.refreshCMLocked(cat)
}

// MercTitle is what a target window shows of the NPC in place of its class
// when it is a merc with a Master Class (as a player's: TargetInfoLines), ""
// otherwise.
func (w *World) MercTitle(n *NPC) string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if n == nil || n.merc == nil {
		return ""
	}
	return n.merc.kit.Master
}
