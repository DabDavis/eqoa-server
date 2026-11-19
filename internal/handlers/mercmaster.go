package handlers

import (
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// mercmaster.go — a merc's Master Class and its race's pets (the owner,
// 2026-11-15: "yes build the gnome tinker pet for mercs. actually this can be
// a bigger udpate to mercs, if 55+ choose the merc's master class tag (which
// comes with the master class abilities)", then "tinker is racial master
// class, so account for archetype master class, racial, etc. magician mercs
// can be aero, pyro, hydro, geo mancers also"; a player's ask: "is it
// possible to give the gnome mercs a tinker pet option?"). The rules are
// entity/mercmaster.go's.
//
//   - /merc [name] master: the Master Class it holds and those it may take,
//     by kind; /merc [name] master <name> gives it one (a part of the name
//     will do when only one begins so), in place of any it held; /merc
//     [name] master none takes it away. Its owner (so it) of level 55 or more
//     alone: refused below, said why. Kept in its row (charmerc.master), so
//     at every login and through a keep restart (its kit is made from the
//     row), and taken at once by a merc out (rekit: its body, its book, its
//     pet). A row whose Master Class its class and race are no longer
//     offered (a GM's edit of cmcatalog) is a merc with none until another is
//     given.
//   - its menu's Master (level 55 or more): the kinds it may take (its
//     class's, its race's, an archetype's) and None, then a kind's names a
//     page at a time (the client's dialog room less one, More between them).
//   - its race's racial abilities that summon (the gnome's Invention from 6,
//     the troll's Swamp Friends, the Mermaid's Aquatic Birthright) are in its
//     book at the levels a player buys them, its pet only when nothing else
//     of its book summons (entity's mercPetSpell): no other racial ability
//     is given (a merc takes no mastery but these, as before).
//
// The target window shows the Master Class in place of its class, as a
// player's (MercTitle, target.go), and /merc says it.

// The Master Class's lines (ours). A question is 127 characters at most.
const (
	mercMasterLow     = "%s must be level %d to hold a Master Class (it is %d)."
	mercMasterNone    = "No Master Class is offered to a %s %s."
	mercMasterHolds   = "%s holds the Master Class of %s."
	mercMasterHoldsNo = "%s holds no Master Class."
	mercMasterMay     = "A %s %s may take: %s. /merc master <name> gives one, /merc master none takes it away."
	mercMasterSet     = "%s is now a %s%s."
	mercMasterGone    = "%s no longer holds the Master Class of %s."
	mercMasterWhich   = "A %s %s cannot take %q. It may take: %s."
	mercMasterAsk     = "%s's Master Class: %s. Which kind?"
	mercMasterListAsk = "%s Master Classes for %s (%d of %d):"
)

// mercMasterKindNames are the kinds' words in the lines and the menu.
var mercMasterKindNames = map[string]string{
	entity.MercMasterOfClass:     "Class",
	entity.MercMasterOfRace:      "Racial",
	entity.MercMasterOfArchetype: "Archetype",
}

// mercMasterNoneWords take a merc's Master Class away.
var mercMasterNoneWords = map[string]bool{"none": true, "off": true, "clear": true, "remove": true}

// mercMasterSaid is the list's names by kind, for a line.
func mercMasterSaid(list []entity.MercMasterClass) string {
	var parts []string
	for _, kind := range []string{entity.MercMasterOfClass, entity.MercMasterOfRace, entity.MercMasterOfArchetype} {
		var names []string
		for _, mc := range list {
			if mc.Kind == kind {
				names = append(names, mc.Name)
			}
		}
		if len(names) > 0 {
			parts = append(parts, mercMasterKindNames[kind]+" "+strings.Join(names, ", "))
		}
	}
	return strings.Join(parts, "; ")
}

// mercMasterOf is the row's Master Class as its class and race may hold it,
// false for none (or one no longer offered).
func (m *Mercs) mercMasterOf(row database.MercRow) (entity.MercMasterClass, bool) {
	if row.Master == "" {
		return entity.MercMasterClass{}, false
	}
	for _, mc := range m.gw.MercMasterClasses(row.Class, row.Race) {
		if strings.EqualFold(mc.Name, row.Master) {
			return mc, true
		}
	}
	return entity.MercMasterClass{}, false
}

// mercSummons is a summoning a merc casts for its pet (entity's mercPetSpell).
func mercSummons(s *entity.CastSpell) bool {
	return s.Def.Summons() && !s.Def.Charms() && s.Familiar == nil && s.Travel == nil
}

// mercMasterKit puts the row's race's pets and its Master Class on the kit at
// its level (the file's doc): its book changed as the ranks change a
// player's, the ranks on its body, the summonings they give noted.
func (m *Mercs) mercMasterKit(row database.MercRow, k *entity.MercKit) {
	if m.db == nil || m.gw == nil {
		return
	}
	// the race's racial abilities that summon, every spell of the rank a
	// summoning
	var racial []*entity.CMRank
	for _, r := range m.gw.MercRacialRanks(row.Class, row.Race, k.Level) {
		spells := m.rankSpells(r.Adds)
		if len(spells) == 0 {
			continue
		}
		all := true
		for _, s := range spells {
			all = all && mercSummons(&s)
		}
		if all {
			racial = append(racial, r)
		}
	}
	if len(racial) > 0 {
		give := m.addRanks(k, racial)
		k.RacialPets = give
	}
	mc, ok := m.mercMasterOf(row)
	if !ok || k.Level < entity.MercMasterLevel {
		return
	}
	have := map[int]bool{}
	summons := map[int]bool{}
	for _, s := range k.Spells {
		have[s.SpellID] = true
		if mercSummons(&s) {
			summons[s.SpellID] = true
		}
	}
	held := func(a entity.CMAbility) bool {
		if have[a.SpellID] {
			return true
		}
		return slices.ContainsFunc(a.Alts, func(id int) bool { return have[id] })
	}
	var ranks []*entity.CMRank
	for _, r := range mc.Ranks {
		if r.Level > k.Level {
			continue
		}
		adds := m.rankSpells(r.Adds)
		addsSummon := slices.ContainsFunc(adds, func(s entity.CastSpell) bool { return mercSummons(&s) })
		need, look := true, false
		for _, a := range r.Requires {
			if a.SpellID > 0 && !held(a) {
				need = false
			}
			look = look || a.SpellID > 0 && summons[a.SpellID] && addsSummon
		}
		if !need || look {
			continue // what it replaces it lacks, or another look of a pet it has (Construct (Rat))
		}
		ranks = append(ranks, r)
		give, take := r.BookChange()
		for _, a := range give {
			have[a.SpellID] = a.SpellID > 0
		}
		for _, a := range take {
			delete(have, a.SpellID)
		}
		for _, s := range adds {
			if mercSummons(&s) {
				summons[s.SpellID] = true
			}
		}
	}
	m.addRanks(k, ranks)
	gives := map[int]bool{} // every spell its ranks give, in its book before or not
	for _, r := range ranks {
		g, _ := r.BookChange()
		for _, a := range g {
			gives[a.SpellID] = a.SpellID > 0
		}
	}
	for i := range k.Spells {
		s := &k.Spells[i]
		if !gives[s.SpellID] {
			continue
		}
		if mercSummons(s) {
			k.MasterPets = append(k.MasterPets, s.SpellID)
		}
		if s.Def.Harmful && s.Scope == 1 && s.Area.Centre == entity.AreaOnTarget {
			s.MercSplash = true // its splash on what fights its team alone (entity's mercSplashLocked)
		}
	}
	k.Master, k.MasterRanks = mc.Name, ranks
}

// rankSpells is the spells of a rank's abilities, those found.
func (m *Mercs) rankSpells(list []entity.CMAbility) []entity.CastSpell {
	var ids []int
	for _, a := range list {
		if a.SpellID > 0 {
			ids = append(ids, a.SpellID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return entity.CastSpellsByID(m.db, ids)
}

// addRanks changes the kit's book as buying the ranks changes a player's
// (entity.CMAddBook): what they replace out, what they give in (once). It
// is the ids given.
func (m *Mercs) addRanks(k *entity.MercKit, ranks []*entity.CMRank) (given []int) {
	give, take := entity.CMAddBook(ranks)
	out := map[int]bool{}
	for _, a := range take {
		if a.SpellID > 0 {
			out[a.SpellID] = true
		}
	}
	book := k.Spells[:0:0]
	have := map[int]bool{}
	for _, s := range k.Spells {
		if !out[s.SpellID] {
			book = append(book, s)
			have[s.SpellID] = true
		}
	}
	added := m.rankSpells(give)
	for _, s := range added {
		if have[s.SpellID] {
			continue
		}
		have[s.SpellID] = true
		s.DisplayIndex = len(book) + 1
		book = append(book, s)
		given = append(given, s.SpellID)
	}
	k.Spells = book
	return given
}

// master is /merc [name] master [name|none] (the file's doc).
func (m *Mercs) master(sess *session.Session, p *entity.Player, slot int, word string) {
	if m.db == nil {
		SendErrorMessage(sess, mercNoneLine)
		return
	}
	row, ok, err := m.db.Merc(p.CharServerID, slot)
	if err != nil || !ok {
		SendClientMessage(sess, mercNoneLine)
		return
	}
	race, class := entity.RaceName(row.Race), entity.ClassName(row.Class)
	list := m.gw.MercMasterClasses(row.Class, row.Race)
	word = strings.TrimSpace(word)
	if word == "" {
		if mc, ok := m.mercMasterOf(row); ok {
			line := fmt.Sprintf(mercMasterHolds, row.Name, mc.Name)
			if p.Level < entity.MercMasterLevel {
				line += fmt.Sprintf(" It comes into it at level %d.", entity.MercMasterLevel)
			}
			SendClientMessage(sess, line)
		} else {
			SendClientMessage(sess, fmt.Sprintf(mercMasterHoldsNo, row.Name))
		}
		if len(list) == 0 {
			SendClientMessage(sess, fmt.Sprintf(mercMasterNone, race, class))
			return
		}
		SendClientMessage(sess, fmt.Sprintf(mercMasterMay, race, class, mercMasterSaid(list)))
		return
	}
	if mercMasterNoneWords[strings.ToLower(word)] {
		if row.Master == "" {
			SendClientMessage(sess, fmt.Sprintf(mercMasterHoldsNo, row.Name))
			return
		}
		m.setMaster(sess, p, row, "")
		SendClientMessage(sess, fmt.Sprintf(mercMasterGone, row.Name, row.Master))
		return
	}
	if p.Level < entity.MercMasterLevel {
		SendErrorMessage(sess, fmt.Sprintf(mercMasterLow, row.Name, entity.MercMasterLevel, p.Level))
		return
	}
	if len(list) == 0 {
		SendErrorMessage(sess, fmt.Sprintf(mercMasterNone, race, class))
		return
	}
	mc, ok := entity.MercMasterNamed(list, word)
	if !ok {
		SendErrorMessage(sess, fmt.Sprintf(mercMasterWhich, race, class, word, mercMasterSaid(list)))
		return
	}
	if !m.setMaster(sess, p, row, mc.Name) {
		return
	}
	var got []string // the abilities it holds now: its kit's ranks
	row.Master = mc.Name
	if k, err := m.kit(row, p.Level); err == nil {
		for _, r := range k.MasterRanks {
			if r.Parent != 0 {
				got = append(got, r.Name)
			}
		}
	}
	more := ""
	if len(got) > 0 {
		more = ": " + strings.Join(got, ", ")
	} else if mcAt := mercMasterNextLevel(mc, p.Level); mcAt > 0 {
		more = fmt.Sprintf(", its abilities at level %d", mcAt)
	}
	SendClientMessage(sess, fmt.Sprintf(mercMasterSet, row.Name, mc.Name, more))
}

// mercMasterNextLevel is the least level past level of the Master Class's
// ranks, 0 none.
func mercMasterNextLevel(mc entity.MercMasterClass, level int) int {
	next := 0
	for _, r := range mc.Ranks {
		if r.Level > level && (next == 0 || r.Level < next) {
			next = r.Level
		}
	}
	return next
}

// setMaster keeps the row's Master Class and gives the merc out its kit:
// false when it could not be kept (said).
func (m *Mercs) setMaster(sess *session.Session, p *entity.Player, row database.MercRow, name string) bool {
	if err := m.db.SetMercMaster(p.CharServerID, row.Slot, name); err != nil {
		log.Printf("Merc: %s's %q's Master Class not kept: %v", p.Name, row.Name, err)
		SendErrorMessage(sess, mercKitFailed)
		return false
	}
	m.rekit(p.CharServerID, p.Level) // the merc out takes it now
	log.Printf("Merc: %s's %q's Master Class: %q (was %q)", p.Name, row.Name, name, row.Master)
	return true
}

// menuMaster asks the merc's Master Class's kinds (its menu's Master).
func (m *Mercs) menuMaster(sess *session.Session, p *entity.Player, slot int, info entity.MercInfo) {
	row, ok, _ := m.db.Merc(p.CharServerID, slot)
	if !ok {
		return
	}
	list := m.gw.MercMasterClasses(row.Class, row.Race)
	var keys, answers []string
	for _, kind := range []string{entity.MercMasterOfClass, entity.MercMasterOfRace, entity.MercMasterOfArchetype} {
		n := 0
		for _, mc := range list {
			if mc.Kind == kind {
				n++
			}
		}
		if n > 0 {
			keys, answers = append(keys, kind), append(answers, fmt.Sprintf("%s (%d)", mercMasterKindNames[kind], n))
		}
	}
	held := "none"
	if mc, ok := m.mercMasterOf(row); ok {
		held = mc.Name
		keys, answers = append(keys, "none"), append(answers, "None")
	}
	if len(keys) == 0 {
		mercSay(sess, fmt.Sprintf(mercMasterNone, entity.RaceName(row.Race), entity.ClassName(row.Class)))
		return
	}
	m.mu.Lock()
	m.askLocked(sess, p, mercAsk{stage: mercStageMaster, slot: slot, keys: keys}, fmt.Sprintf(mercMasterAsk, info.Name, held), answers)
	m.mu.Unlock()
}

// menuMasterList asks a page of a kind's Master Classes, the one held marked.
func (m *Mercs) menuMasterList(sess *session.Session, p *entity.Player, slot int, info entity.MercInfo, kind string, page int) {
	row, ok, _ := m.db.Merc(p.CharServerID, slot)
	if !ok {
		return
	}
	var names []string
	for _, mc := range m.gw.MercMasterClasses(row.Class, row.Race) {
		if mc.Kind == kind {
			names = append(names, mc.Name)
		}
	}
	if len(names) == 0 {
		return
	}
	per := dialogRoom(sess) - 1
	pages := (len(names) + per - 1) / per
	if pages == 1 {
		per = len(names)
	}
	page = max(0, min(page, pages-1))
	shown := names[page*per : min((page+1)*per, len(names))]
	var answers []string
	for _, n := range shown {
		if strings.EqualFold(n, row.Master) {
			n += " (now)"
		}
		answers = append(answers, n)
	}
	keys := slices.Clone(shown)
	if pages > 1 {
		answers, keys = append(answers, "More"), append(keys, "more")
	}
	m.mu.Lock()
	m.askLocked(sess, p, mercAsk{stage: mercStageMasterList, slot: slot, page: page, per: per, keys: keys, kind: kind},
		fmt.Sprintf(mercMasterListAsk, mercMasterKindNames[kind], info.Name, page+1, pages), answers)
	m.mu.Unlock()
}

// masterAnswer is a choice in a Master Class box (menuAnswer's).
func (m *Mercs) masterAnswer(sess *session.Session, p *entity.Player, a mercAsk, info entity.MercInfo, n int) {
	if n >= len(a.keys) {
		return
	}
	order := func(words string) { m.Command(sess, p, fmt.Sprintf("%d %s", a.slot, words)) }
	key := a.keys[n]
	switch a.stage {
	case mercStageMaster:
		if key == "none" {
			order("master none")
			return
		}
		m.menuMasterList(sess, p, a.slot, info, key, 0)
	case mercStageMasterList:
		if key == "more" {
			next := a.page + 1
			if next*a.perBox() >= m.masterCount(p, a.slot, a.kind) {
				next = 0
			}
			m.menuMasterList(sess, p, a.slot, info, a.kind, next)
			return
		}
		order("master " + key)
	}
}

// masterCount is how many Master Classes of the kind the merc of the slot
// may take.
func (m *Mercs) masterCount(p *entity.Player, slot int, kind string) int {
	row, ok, _ := m.db.Merc(p.CharServerID, slot)
	if !ok {
		return 0
	}
	n := 0
	for _, mc := range m.gw.MercMasterClasses(row.Class, row.Race) {
		if mc.Kind == kind {
			n++
		}
	}
	return n
}
