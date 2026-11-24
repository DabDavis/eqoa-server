package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// placeholder.go — /ph, also /placeholder (a GM, Heirloom petition 2741:
// "testers and gm's need a command to /ph. this is /placeholder which means
// they can bind multiple mobs to a spawn point ... /ph deathfist reaver90,
// deathfist prophet10 ... set the percentage of 100 odds that each one pops.
// This needs to be able to set up to 5 different monster"). A GM's (acctlevel
// 1 or more) and a sub GM's, the tester drawn [Test] (devcmds.go's
// subGMCommands); as /ph, #ph, or a PS2's "/t ph ...":
//
//   - /ph <name>[odds], <name>[odds], ...: the target, an NPC of a row (not a
//     player, a pet, a mercenary, a spawn of no row: npcRowTarget), pops one
//     of 1 to 5 kinds each time it comes back from a death, at the odds given:
//     a number glued to the name or after it ("deathfist reaver90",
//     "deathfist reaver 90"), on every name or on none (even odds), each 1 to
//     100, adding to 100 exactly. Names are NPC names as they stand in the
//     world, any case; a training dummy and a spawn-only original are refused
//     as /camp refuses them; the target's own kind may be one. It is the row's
//     campspawns row (database/placeholder.go: camp 0, or a camp's spawn's own
//     with its camp kept), read into the world at once (entity/campspawn.go),
//     so the next respawn pops by it. Alternate records stay with the binding
//     when the last fixed spawn of a kind is deleted. A placeholder stands or wanders as the
//     kind it popped; a keep restart keeps the kind it stands as.
//   - /ph alone: the target's kinds and odds.
//   - /ph clear: the binding off; it comes back as its own row's kind at its
//     next respawn (a camp's spawn pops its own kind alone).
//
// Every change has its gm_audit line (via "game"), so the GM panel lists and
// reverts it.

// phUsage is /ph's usage line, with its example.
const phUsage = "Usage: /ph <name>[odds], up to 5, odds adding to 100 - e.g. /ph deathfist reaver90, deathfist prophet10"

// phHelp is /help's line for /ph, to a sub GM (a GM's is in #help).
const phHelp = "  /ph <name>[odds], ... - your target's spawn point pops 1-5 kinds, odds adding to 100 (/ph deathfist reaver90, deathfist prophet10); /ph alone shows them, /ph clear; /t ph on a PS2"

// phMore is /ph's other forms.
const phMore = "Target the creature first. No odds = even odds. /ph alone shows its kinds; /ph clear takes them off."

// phKind is one kind of a /ph: its name as the NPCs spell it, and its odds (0
// when none were given).
type phKind struct {
	Name   string
	Weight int
}

// phParse reads /ph's list (the text after the command) into its kinds, by
// lookup (a name in any case to the NPCs' spelling; false for none), or says
// in one line what is wrong with it, with close names for an unknown one by
// suggest (nil: none).
func phParse(arg string, lookup func(string) (string, bool), suggest func(string) []string) ([]phKind, string) {
	var entries []string
	for _, e := range strings.Split(arg, ",") {
		if e = strings.Join(strings.Fields(e), " "); e != "" {
			entries = append(entries, e)
		}
	}
	if len(entries) == 0 {
		return nil, "No names given."
	}
	if len(entries) > database.MaxPlaceholderKinds {
		return nil, fmt.Sprintf("%d names given: a spawn point pops at most %d.", len(entries), database.MaxPlaceholderKinds)
	}
	var kinds []phKind
	var unknown []string
	for _, e := range entries {
		k, bad, ok := phEntry(e, lookup)
		if bad != "" {
			return nil, bad
		}
		if !ok {
			u := fmt.Sprintf("%q", k.Name)
			if suggest != nil {
				if s := suggest(k.Name); len(s) > 0 {
					u += " (did you mean " + orList(s) + "?)"
				}
			}
			unknown = append(unknown, u)
			continue
		}
		kinds = append(kinds, k)
	}
	if len(unknown) > 0 {
		return nil, fmt.Sprintf("Unknown %s %s: no NPC is called that.", plural(len(unknown), "name", "names"), strings.Join(unknown, ", "))
	}
	seen := map[string]bool{}
	given, sum := 0, 0
	for _, k := range kinds {
		key := strings.ToLower(k.Name)
		if seen[key] {
			return nil, fmt.Sprintf("%s is named twice: name each creature once.", k.Name)
		}
		seen[key] = true
		if k.Weight > 0 {
			given++
			sum += k.Weight
		}
	}
	switch {
	case given > 0 && given < len(kinds):
		var none []string
		for _, k := range kinds {
			if k.Weight == 0 {
				none = append(none, k.Name)
			}
		}
		return nil, fmt.Sprintf("Odds on %d of %d names (none on %s): give odds to every name, or to none for even odds.",
			given, len(kinds), orList(none))
	case given > 0 && sum != 100:
		return nil, fmt.Sprintf("The odds add to %d, not 100.", sum)
	}
	return kinds, ""
}

// phEntry reads one name of a /ph's list: the whole of it a name, else a name
// and its odds after it (glued or after a space; a % after them may be). It
// gives the kind, what is wrong with its odds (""), and false when no name
// was found (the kind's name then the one it read, for the GM to see).
func phEntry(e string, lookup func(string) (string, bool)) (phKind, string, bool) {
	e = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(e), "%"))
	if name, ok := lookup(e); ok {
		return phKind{Name: name}, "", true
	}
	digits := len(e)
	for digits > 0 && unicode.IsDigit(rune(e[digits-1])) {
		digits--
	}
	run := e[digits:] // the digits at the end
	if run == "" {
		return phKind{Name: e}, "", false
	}
	// the shortest odds whose name is a name: "RE PC Fire 1250" is "RE PC
	// Fire 12" at 50
	for k := 1; k <= len(run); k++ {
		name := strings.TrimSpace(e[:len(e)-k])
		got, ok := lookup(name)
		if !ok {
			continue
		}
		v, err := strconv.Atoi(e[len(e)-k:])
		if err != nil || v < 1 || v > 100 {
			return phKind{}, fmt.Sprintf("The odds of %s are %s: each must be 1 to 100.", got, e[len(e)-k:]), false
		}
		return phKind{Name: got, Weight: v}, "", true
	}
	return phKind{Name: strings.TrimSpace(e[:digits])}, "", false
}

// phNames is the lookup and the suggestions /ph reads names by: the NPCs of
// the world, any case.
func phNames(names []string) (func(string) (string, bool), func(string) []string) {
	byLower := make(map[string]string, len(names))
	for _, n := range names {
		l := strings.ToLower(n)
		if _, ok := byLower[l]; !ok {
			byLower[l] = n
		}
	}
	lookup := func(q string) (string, bool) {
		n, ok := byLower[strings.ToLower(strings.TrimSpace(q))]
		return n, ok
	}
	suggest := func(q string) []string {
		q = strings.ToLower(q)
		type near struct {
			name string
			d    int
		}
		var close []near
		for l, n := range byLower {
			if abs(len(l)-len(q)) > 2 {
				continue
			}
			if d := editDistance(l, q); d <= 2 {
				close = append(close, near{n, d})
			}
		}
		sort.Slice(close, func(i, j int) bool {
			if close[i].d != close[j].d {
				return close[i].d < close[j].d
			}
			return close[i].name < close[j].name
		})
		var out []string
		for _, c := range close {
			out = append(out, c.name)
		}
		if len(out) == 0 {
			out = searchNames(names, q)
		}
		if len(out) > 3 {
			out = out[:3]
		}
		return out
	}
	return lookup, suggest
}

// abs is |v|.
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// editDistance is the Levenshtein distance of a and b, by bytes.
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// phOdds is kinds at their odds as the GM reads them: "a 90%, b 10%", even
// odds each its share.
func phOdds(kinds []string, weights []int) string {
	parts := make([]string, len(kinds))
	for i, k := range kinds {
		if weights != nil && len(weights) == len(kinds) {
			parts[i] = fmt.Sprintf("%s %d%%", k, weights[i])
		} else {
			parts[i] = fmt.Sprintf("%s %g%%", k, math.Round(1000/float64(len(kinds)))/10)
		}
	}
	return strings.Join(parts, ", ")
}

// phCommand is /ph [<name>[odds], ... | clear] (the file's doc).
func (h *ChatHandler) phCommand(sess *session.Session, player *entity.Player, args []string) {
	if h.camps.db == nil {
		h.gmReply(sess, "/ph: no database here")
		return
	}
	arg := strings.TrimSpace(strings.Join(args, " "))
	if strings.EqualFold(arg, "help") || strings.EqualFold(arg, "?") {
		h.gmReply(sess, phUsage)
		h.gmReply(sess, phMore)
		return
	}
	// the names first: a list that is wrong is said so before the target
	var kinds []phKind
	if arg != "" && !strings.EqualFold(arg, "clear") {
		lookup, suggest := phNames(h.gameWorld.PlaceholderNames())
		var bad string
		if kinds, bad = phParse(arg, lookup, suggest); bad != "" {
			h.gmReply(sess, "/ph: "+bad)
			h.gmReply(sess, phUsage)
			return
		}
		for _, k := range kinds {
			if why := h.gameWorld.CampKindRefused(k.Name); why != "" {
				h.gmReply(sess, fmt.Sprintf("/ph: %s %s: nothing changed.", k.Name, why))
				return
			}
		}
	}
	if player.TargetObjectID == 0 || player.TargetObjectID == 0xFFFFFFFF {
		h.gmReply(sess, "/ph: target the creature whose spawn point it is first.")
		h.gmReply(sess, phUsage)
		return
	}
	id, rec, ok := h.npcRowTarget(sess, player, "/ph")
	if !ok {
		return
	}
	what := fmt.Sprintf("%s (row %d)", rec.NPCName, id)
	cur, has, err := h.camps.db.CampSpawnOf(int(id))
	if err != nil {
		h.gmReply(sess, "/ph: "+err.Error())
		return
	}
	campNote := func() string {
		if has && cur.Camp != 0 {
			return fmt.Sprintf(" (a spawn of camp %d)", cur.Camp)
		}
		return ""
	}

	switch {
	case arg == "":
		if !has || len(cur.Kinds) == 0 {
			h.gmReply(sess, fmt.Sprintf("%s pops only itself: it is no placeholder. %s", what, phUsage))
			return
		}
		h.gmReply(sess, fmt.Sprintf("%s%s pops %s each time it comes back. /ph clear takes it off.", what, campNote(), phOdds(cur.Kinds, cur.Weights)))
		return

	case strings.EqualFold(arg, "clear"):
		detail := "/ph clear in the game: " + what
		done, err := h.camps.db.ClearPlaceholderAudited(int(id), rec.NPCName, gmEditOf(sess), detail)
		if err != nil {
			h.gmReply(sess, "/ph clear: nothing changed: "+err.Error())
			return
		}
		if done == "" {
			h.gmReply(sess, fmt.Sprintf("%s is no placeholder: nothing to clear.", what))
			return
		}
		if _, err := h.gameWorld.LoadCampSpawns(h.camps.db); err != nil {
			log.Printf("GM: /ph clear %d: the camps not read again: %v", id, err)
		}
		if done == "update" {
			h.gmReply(sess, fmt.Sprintf("%s%s pops only its own kind now, from its next respawn.", what, campNote()))
		} else {
			h.gmReply(sess, fmt.Sprintf("%s is no placeholder now: it comes back as itself from its next respawn.", what))
		}
		log.Printf("GM: %s took the placeholder off NPC %d (%s)", player.Name, id, rec.NPCName)
		return
	}

	names := make([]string, len(kinds))
	var weights []int
	for i, k := range kinds {
		names[i] = k.Name
		if k.Weight > 0 {
			weights = append(weights, k.Weight)
		}
	}
	alternates, err := h.phTemplates(rec, cur, names)
	if err != nil {
		h.gmReply(sess, fmt.Sprintf("/ph: %v: nothing changed.", err))
		return
	}
	odds := phOdds(names, weights)
	detail := fmt.Sprintf("/ph in the game: %s pops %s", what, odds)
	action, err := h.camps.db.SetPlaceholderAudited(int(id), names, weights, gmEditOf(sess), detail, alternates...)
	if err != nil {
		h.gmReply(sess, "/ph: nothing changed: "+err.Error())
		return
	}
	if action == "unchanged" {
		h.gmReply(sess, fmt.Sprintf("%s pops %s already.", what, odds))
		return
	}
	if _, err := h.gameWorld.LoadCampSpawns(h.camps.db); err != nil {
		h.gmReply(sess, fmt.Sprintf("/ph: kept, but not in the world yet (at the next start): %v", err))
		return
	}
	h.gmReply(sess, fmt.Sprintf("%s%s is a placeholder now: it pops %s, from its next respawn. /ph clear takes it off.", what, campNote(), odds))
	log.Printf("GM: %s made NPC %d (%s) a placeholder popping %s", player.Name, id, rec.NPCName, odds)
}

// phTemplates captures alternate NPC records before acknowledging the bind.
// Editing the odds on an existing point can reuse its last saved record.
func (h *ChatHandler) phTemplates(spot *database.NPCRecord, current database.CampSpawnRow, names []string) ([]database.NPCRecord, error) {
	store := database.NewNPCStore(h.camps.db)
	own := database.NPCClassKey(spot.NPCName)
	var alternates []database.NPCRecord
	for _, name := range names {
		key := database.NPCClassKey(name)
		if key == own {
			continue
		}
		if id, ok := h.gameWorld.CampTemplate(name, uint8(spot.World), spot.XCoord, spot.ZCoord); ok {
			source, err := store.GetNPC(int(id))
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
			if source != nil && database.NPCClassKey(source.NPCName) == key {
				alternates = append(alternates, *source)
				continue
			}
		}
		saved, ok := current.Templates[key]
		if !ok || database.NPCClassKey(saved.NPCName) != key {
			return nil, fmt.Errorf("no NPC row is called %q (a GM's spawn has none)", name)
		}
		alternates = append(alternates, saved)
	}
	return alternates, nil
}
