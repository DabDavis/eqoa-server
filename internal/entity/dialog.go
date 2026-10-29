package entity

import (
	"math/rand/v2"
	"regexp"
	"strings"

	"github.com/eqoa/server/internal/database"
)

// Town dialog: what a town's people and the guards of Norrath say when talked
// to and nothing else answers (the user, 2026-10-29: "add dialog to all town
// NPCs and Guards throughout Norrath, but not wildlife of course"; the lines
// are ours, table npcdialog, database/npcdialog.go).
//
// Who speaks (dialogSpeakerLocked): an NPC of a town's faction (npcfaction: a
// town's Citizens, its guards' faction, Neriak's houses), or a guard of no
// town by a name of its own ("Guard Miller") or a militia's (a camp's
// "Siloth knight" says nothing); on a body of a players' race
// (charactermodel) or with the NPC bit, alive, and not wildlife, undead, a
// pet, a charmed creature or a training dummy: a gnoll's guard or a
// skeleton's says nothing.
//
// What it says (TownLine): a line of npcdialog whose selectors fit (an empty
// one fits anyone): the speaker's town, its role by its name (NPCRole), its
// name, the mood (DialogMood: the listener's standing with its faction, as
// the target box's face shows it) and the listener's race. Each fitting line
// counts its weight, times 3 for its town, 2 for its role, 6 for its name, 2
// for its race and 2 for its mood, so a town's own words are heard more than
// the generic ones; one at random, never the one this speaker said to this
// listener last when there is another.

// The moods a line may be for (npcdialog.mood).
const (
	MoodNeutral  = "neutral"
	MoodFriendly = "friendly"
	MoodWarm     = "warm"
	MoodWary     = "wary"
	MoodHostile  = "hostile"
)

// The roles a line may be for (npcdialog.role), by the speaker's name.
const (
	RoleGuard   = "guard"
	RoleTrader  = "trader"
	RolePriest  = "priest"
	RoleScholar = "scholar"
	RoleWorker  = "worker"
	RoleNoble   = "noble"
	RoleCitizen = "citizen"
)

// roleWords names a role by a word of the speaker's name, the first role
// that has one (a guard's words are seed_npc_social.py's GUARD_WORDS and the
// towns' own: Oggok's bashers, Freeport's bouncers and dragoons).
var roleWords = []struct {
	role  string
	words map[string]bool
}{
	{RoleGuard, setOf("guard", "guards", "guardsman", "guardian", "sentry", "sentinel", "watchman", "warden", "deputy",
		"lieutenant", "sergeant", "soldier", "corporal", "protector", "marshal", "marshall", "commander", "captain",
		"militia", "defender", "trooper", "force", "footman", "dragoon", "crossbowman", "bouncer", "basher", "watch",
		"legionnaire", "patrol", "patrolman", "knight", "crusader", "scout")},
	{RoleTrader, setOf("merchant", "smithy", "tailor", "bowyer", "grocer", "armorer", "banker", "jeweler",
		"weaponsmith", "chemist", "seamster", "shieldsmith", "blacksmith", "tacklemaster", "trader", "vendor",
		"peddler", "barkeep", "bartender", "innkeeper", "brewer", "cook", "baker", "porter", "keeper", "fletcher")},
	{RolePriest, setOf("acolyte", "priest", "priestess", "cleric", "initiate", "deacon", "bishop", "oracle", "seer",
		"shaman", "mystic", "monk", "abbot", "templar", "disciple", "zealot", "denouncer", "doomseeker")},
	{RoleScholar, setOf("historian", "scribe", "sage", "scholar", "librarian", "advisor", "archivist", "loremaster",
		"teacher", "tutor", "wizard", "magus", "mage", "researcher", "astronomer", "alchemist")},
	{RoleWorker, setOf("farmer", "miner", "foreman", "fisherman", "fisher", "laborer", "labourer", "hunter", "trapper",
		"digger", "woodcutter", "lumberjack", "shepherd", "herder", "rancher", "dockhand", "sailor", "deckhand",
		"stablehand", "groom", "worker", "carpenter", "mason", "excavator", "prospector", "assistant", "apprentice")},
	{RoleNoble, setOf("sir", "lord", "lady", "baron", "baroness", "duke", "duchess", "count", "countess", "senator",
		"councilor", "councillor", "magistrate", "noble", "noblewoman", "nobleman", "prince", "princess", "king",
		"queen", "regent", "chancellor", "steward", "elder")},
}

func setOf(ws ...string) map[string]bool {
	m := make(map[string]bool, len(ws))
	for _, w := range ws {
		m[w] = true
	}
	return m
}

var nameWord = regexp.MustCompile(`[a-z]+`)

// NPCRole is the role a speaker's name gives it, else a citizen's.
func NPCRole(name string) string {
	words := nameWord.FindAllString(strings.ToLower(name), -1)
	for _, r := range roleWords {
		for _, w := range words {
			if r.words[w] {
				return r.role
			}
		}
	}
	return RoleCitizen
}

// townFactions are the factions of a town's people that name no town in
// "Citizens of": its guards', Neriak's houses, the merchant houses.
var townFactions = map[string]string{
	"The Freeport Militia":   "Freeport",
	"The Qeynos Guard":       "Qeynos",
	"The Highpass Guard":     "Highpass",
	"Klick`Anon Watch":       "Klick`Anon",
	"House U`Dedne":          "Neriak",
	"House J`Narus":          "Neriak",
	"House Do`Vexis":         "Neriak",
	"House Gal`Saris":        "Neriak",
	"Merchant House Nothard": "Freeport",
}

// DialogTown is the town a faction's NPCs are of, "" none: "Citizens of X"
// is X's, the others townFactions'.
func DialogTown(faction string) string {
	if t, ok := strings.CutPrefix(faction, "Citizens of "); ok {
		return t
	}
	return townFactions[faction]
}

// townNames are how a town's name is said, where its faction spells it
// otherwise.
var townNames = map[string]string{
	"Klick`Anon":         "Klick'Anon",
	"Muniels Tea Garden": "Muniel's Tea Garden",
}

// SetModelSexes names the players' race bodies' sexes (charactermodel: 0
// male, 1 female), the speaker's sex a line of town dialog may ask for.
func (w *World) SetModelSexes(sexes map[uint32]int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.modelSex = sexes
}

// dialogSexLocked is npc's sex as a line names it, "male" or "female" by its
// body, "" when no players' race body is it. Must be called with the World's
// lock held.
func (w *World) dialogSexLocked(npc *NPC) string {
	switch sex, ok := w.modelSex[uint32(npc.ModelID)]; {
	case !ok:
		return ""
	case sex == 1:
		return "female"
	default:
		return "male"
	}
}

// dialogSpeakerLocked reports whether npc says town dialog, and its town (""
// none: a guard on a road) and role. Must be called with the World's lock
// held.
func (w *World) dialogSpeakerLocked(npc *NPC) (town, role string, ok bool) {
	if npc.Dead || npc.Owner != 0 || npc.Charm != nil || npc.Dummy != NotDummy {
		return "", "", false
	}
	if !w.playerModels[uint32(npc.ModelID)] && npc.NPCType&0x80 == 0 {
		return "", "", false // a creature's body: a gnoll's guard says nothing
	}
	if w.undeadLocked(npc) {
		return "", "", false
	}
	if _, wild := w.wildlifeLocked(npc); wild {
		return "", "", false
	}
	town = DialogTown(w.npcFactionLocked(npc))
	role = NPCRole(npc.Name)
	if town == "" && (role != RoleGuard || !civilGuardName(npc.Name)) {
		return "", "", false
	}
	return town, role, true
}

// guardTitle is a guard's title before a name of its own ("Guard Miller",
// "Deputy Brady"): a guard of no town speaks only so named or of a militia,
// as the hostile camps' generic spawns on human bodies are named for their
// kind (54 "Siloth knight", 49 "Jextalan guard" on eqoaclassic) and some
// carry the NPC bit (a "hobgoblin scout").
var guardTitle = regexp.MustCompile(`^(?:Guard|Guardsman|Deputy|Captain|Sergeant|Lieutenant|Sentry|Sentinel|Watchman|` +
	`Warden|Marshal|Marshall|Commander|Corporal|Trooper|Footman|Protector|Defender|Soldier)\s+[A-Z]`)

func civilGuardName(name string) bool {
	name = strings.TrimSpace(name)
	return guardTitle.MatchString(name) || strings.Contains(strings.ToLower(name), "militia")
}

// dialogMoodLocked is how npc feels about p: hostile when it attacks p on
// sight or fights it, else by p's standing with its faction (the target
// box's bands: Aggressive and Dubious hostile, Cautious wary, Neutral,
// Friendly, Warm and Allied warm), neutral with no faction. Must be called
// with the World's lock held.
func (w *World) dialogMoodLocked(p *Player, npc *NPC) string {
	if npc.CurrentEnemy == p.ServerID || w.threatensLocked(npc, p) {
		return MoodHostile
	}
	f := w.npcFactionLocked(npc)
	if f == "" || w.aggro.factionOff {
		return MoodNeutral
	}
	switch StandingDisposition(w.standingLocked(p, f)) {
	case DispAggressive, DispDubious:
		return MoodHostile
	case DispCautious:
		return MoodWary
	case DispFriendly:
		return MoodFriendly
	case DispWarm, DispAllied:
		return MoodWarm
	}
	return MoodNeutral
}

// DialogMoodFits reports whether a line of mood lm (npcdialog.mood) is said
// in mood m: "" in neutral, friendly or warm; else m is in its comma list.
func DialogMoodFits(lm, m string) bool {
	if strings.TrimSpace(lm) == "" {
		return m == MoodNeutral || m == MoodFriendly || m == MoodWarm
	}
	for _, x := range strings.Split(lm, ",") {
		if strings.EqualFold(strings.TrimSpace(x), m) {
			return true
		}
	}
	return false
}

// DialogPick is one line that fits and how often it is said.
type DialogPick struct {
	Line   database.NPCDialogLine
	Weight int
}

// DialogCandidates are the lines that fit a speaker of the town, role and
// name speaking in the mood to a listener of the race, each with its weight
// (the file's comment).
func DialogCandidates(lines []database.NPCDialogLine, town, role, name, mood, sex string, race int) []DialogPick {
	key := database.NPCClassKey(name)
	var out []DialogPick
	for _, l := range lines {
		if l.Line == "" || l.Weight <= 0 || (town == "" && strings.Contains(l.Line, "%t")) {
			continue // a line naming the town, from a speaker of none (a guard on a road)
		}
		if l.Town != "" && !strings.EqualFold(l.Town, town) {
			continue
		}
		if l.Role != "" && !strings.EqualFold(l.Role, role) {
			continue
		}
		if l.Name != "" && database.NPCClassKey(l.Name) != key {
			continue
		}
		if l.Race >= 0 && l.Race != race {
			continue
		}
		if l.Sex != "" && !strings.EqualFold(l.Sex, sex) {
			continue // a line of a husband or a wife, said by the one it fits (a speaker of no known sex says none)
		}
		if !DialogMoodFits(l.Mood, mood) {
			continue
		}
		wt := l.Weight
		if l.Town != "" {
			wt *= 3
		}
		if l.Role != "" {
			wt *= 2
		}
		if l.Name != "" {
			wt *= 6
		}
		if l.Race >= 0 {
			wt *= 2
		}
		if l.Sex != "" {
			wt *= 2
		}
		if strings.TrimSpace(l.Mood) != "" {
			wt *= 2
		}
		out = append(out, DialogPick{l, wt})
	}
	return out
}

// TownLine is what npc says to p (the file's comment): its name as it is
// shown, the line with its words filled in and the line's id, ok false when
// it says nothing. last is the id of the line it said to p before, not said
// again when another fits.
func (w *World) TownLine(p *Player, npc *NPC, lines []database.NPCDialogLine, last int) (speaker, line string, id int, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	town, role, ok := w.dialogSpeakerLocked(npc)
	if !ok {
		return "", "", 0, false
	}
	mood := w.dialogMoodLocked(p, npc)
	picks := DialogCandidates(lines, town, role, npc.Name, mood, w.dialogSexLocked(npc), p.Race)
	if len(picks) > 1 && last != 0 {
		kept := picks[:0:0]
		for _, c := range picks {
			if c.Line.ID != last {
				kept = append(kept, c)
			}
		}
		if len(kept) > 0 {
			picks = kept
		}
	}
	total := 0
	for _, c := range picks {
		total += c.Weight
	}
	if total == 0 {
		return "", "", 0, false
	}
	n := rand.IntN(total)
	var chosen database.NPCDialogLine
	for _, c := range picks {
		if n < c.Weight {
			chosen = c.Line
			break
		}
		n -= c.Weight
	}
	speaker = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(npc.Name), "$"))
	return speaker, DialogSay(chosen.Line, p, speaker, town), chosen.ID, true
}

// DialogSay fills a line's words in: %p the listener's name, %r its race,
// %c its class, %t the town, %n the speaker.
func DialogSay(line string, p *Player, speaker, town string) string {
	shown := town
	if t, ok := townNames[town]; ok {
		shown = t
	}
	return strings.NewReplacer(
		"%p", p.Name,
		"%r", strings.ToLower(RaceName(p.Race)),
		"%c", strings.ToLower(ClassName(p.Class)),
		"%t", shown,
		"%n", speaker,
	).Replace(line)
}
