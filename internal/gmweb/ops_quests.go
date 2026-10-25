package gmweb

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/handlers"
	"github.com/eqoa/server/internal/session"
)

// The quests (handlers/quests.go; tables quests, queststeps, charquests): the
// Quests page's operations, an AI agent's too. A quest and its steps are saved
// together, as row changes (each logged and revertible); a character's quest
// is set through the engine, so its journal follows, and logged as the
// charquests row before and after.

// questCols are a quest row's columns the editor sends, in the table's order.
var questCols = []string{"name", "enabled", "giver", "minlevel", "maxlevel", "classes", "races", "requires",
	"repeatable", "offer", "accept", "decline", "declined", "started", "source", "notes"}

// stepCols are a step row's columns the editor sends.
var stepCols = []string{"kind", "target", "npcid", "items", "tunar", "count", "world", "x", "y", "z", "radius",
	"journal", "answer", "says", "waiting", "notyet", "reminder", "xp", "rewardtunar", "rewarditems", "factions", "spells"}

func (s *Server) registerQuestOps() {
	questSchema := obj(map[string]any{
		"questid":    integer("its id; none or 0 makes a new quest"),
		"name":       str("what GMs call it (players see the journal lines)"),
		"enabled":    integer("1: offered in the game; 0: a draft"),
		"giver":      str("the NPC who offers it, by its name as the game shows it"),
		"minlevel":   integer("the lowest level that may take it"),
		"maxlevel":   integer("the highest (0: any)"),
		"classes":    str("class ids that may take it, comma separated; empty: any (0 Warrior ... 14 Alchemist)"),
		"races":      str("race ids, comma separated; empty: any"),
		"requires":   str("quest ids finished first, comma separated"),
		"repeatable": integer("1: may be taken again once finished"),
		"offer":      str("the giver's words offering it: a paragraph (lines between blank lines) a question, a line a box, each paragraph's last line the question"),
		"accept":     str("the player's answers that take it, a line a question (the last one gives the quest); empty: taken as soon as the giver is talked to"),
		"decline":    str("the player's answer that refuses it, offered with each answer; empty: none"),
		"declined":   str("the giver's reply to a refusal, said as written (no name put before it)"),
		"started":    str("the giver's words once it is taken"),
		"source":     str("where it came from"),
		"notes":      str("for GMs"),
	})
	stepSchema := obj(map[string]any{
		"kind":        strEnum("talk: talk to target; show: talk to target holding items (kept); give: hand target items and tunar (taken); kill: kill count of target; reach: come within radius of x, z in world", database.StepKinds...),
		"target":      str("talk/give: the NPC's name; kill: the creature's name"),
		"npcid":       integer("only that spawn (npcs.id); 0: any of the name"),
		"items":       str("show/give: items held (show) or handed in (give), patternid*count, comma separated"),
		"tunar":       integer("show/give: tunar held or handed in"),
		"count":       integer("kill: how many"),
		"world":       integer("reach: the world"),
		"x":           number("reach: x"),
		"y":           number("reach: y"),
		"z":           number("reach: z"),
		"radius":      number("reach: within (default 15)"),
		"journal":     str("the journal's line while this is the step to do (at most 127 characters)"),
		"answer":      str("talk/show/give: the player's answer that does it (its first line; any further lines are answers that change nothing, as Sandstorm's \"Farewell\"); empty: done on talking"),
		"notyet":      str("show/give: the player's answer(s) offered while the items are not held (Sandstorm's \"No, I am lost.\"); empty: the reminder at once"),
		"reminder":    str("show/give: the NPC's words while the items are not held"),
		"says":        str("the NPC's words when it is done: a line a box"),
		"waiting":     str("the NPC's words before it can be done; with an answer, the last line asks"),
		"xp":          integer("reward: experience"),
		"rewardtunar": integer("reward: tunar"),
		"rewarditems": str("reward: items, patternid*count, comma separated"),
		"factions":    str("reward: faction changes, Faction Name:+10, comma separated"),
		"spells":      str("reward: spell ids, comma separated"),
	})
	s.addOp(&Op{
		Name: "quest_list", Title: "List quests", Scope: scopeRead, Core: true,
		Description: "Every quest (drafts too) with its giver, levels, step count and how many characters have it under " +
			"way or finished. search narrows by name, giver or notes.",
		Input: obj(map[string]any{"search": str("part of a name, giver or note")}),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ Search string }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.questList(req.Search)
		},
	})
	s.addOp(&Op{
		Name: "quest_get", Title: "Get a quest", Scope: scopeRead, Core: true,
		Description: "One quest with its steps in order, what is wrong with it (problems: unknown NPCs or items, " +
			"a step no one can do), and who has it.",
		Input: obj(map[string]any{"quest": integer("the quest's id")}, "quest"),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ Quest int64 }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.questGet(req.Quest)
		},
	})
	s.addOp(&Op{
		Name: "quest_save", Title: "Save a quest", Scope: scopeWrite, Core: true,
		Description: "Make or change a quest and its steps in one go: quest is its row (no questid: a new one), steps " +
			"its steps in order (numbered 1, 2, ... as given; steps past the last are deleted). Each row changed is " +
			"logged and revertible. The game uses it at once. The answer lists its problems.",
		Input: obj(map[string]any{
			"quest": questSchema,
			"steps": arr(stepSchema, "the steps in order"),
		}, "quest", "steps"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Quest map[string]any
				Steps []map[string]any
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.questSave(c.a, req.Quest, req.Steps)
		},
	})
	s.addOp(&Op{
		Name: "quest_delete", Title: "Delete a quest", Scope: scopeWrite, Destructive: true,
		Description: "Delete a quest, its steps and every character's progress in it (each row logged and revertible). " +
			"dryRun says what would go.",
		Input: obj(map[string]any{"quest": integer("the quest's id"), "dryRun": boolean("only say what would go")}, "quest"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Quest  int64
				DryRun bool
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.questDelete(c.a, req.Quest, req.DryRun)
		},
	})
	s.addOp(&Op{
		Name: "character_quests", Title: "A character's quests", Scope: scopeRead, Core: true,
		Description: "A character's quests: those under way in journal order (with the step to do and its journal " +
			"line), then those finished.",
		Input: obj(map[string]any{"character": idOrName("the character, by name or serverid")}, "character"),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ Character any }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.characterQuests(req.Character)
		},
	})
	s.addOp(&Op{
		Name: "character_quest_set", Title: "Set a character's quest", Scope: scopeWrite, Core: true,
		Description: "Change a character's quest, its journal kept in step (at once in the world): give (under way at " +
			"step, its first when none), step (the step to do), complete (finished, no reward), giveup (ended " +
			"unfinished, as the player's own delete) or reset (forgotten: it may be taken again). Logged and revertible.",
		Input: obj(map[string]any{
			"character": idOrName("the character, by name or serverid"),
			"quest":     integer("the quest's id"),
			"action":    strEnum("what to do", handlers.QuestActions...),
			"step":      integer("give/step: the step"),
		}, "character", "quest", "action"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Character any
				Quest     int64
				Action    string
				Step      int
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.characterQuestSet(c.a, req.Character, int(req.Quest), req.Action, req.Step)
		},
	})
	s.addOp(&Op{
		Name: "quest_try", Title: "Try a quest", Scope: scopeWrite, Core: true,
		Description: "Give a quest (a draft too) to your own character in the world, to play it through: talk to its " +
			"first step's NPC. Logged and revertible.",
		Input: obj(map[string]any{"quest": integer("the quest's id")}, "quest"),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ Quest int64 }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.questTry(c.a, int(req.Quest))
		},
	})
	s.addOp(&Op{
		Name: "quest_import_eqoadata", Title: "Import EQOA-Data quests as drafts", Scope: scopeWrite,
		Description: "Make DRAFT quests (off until a GM turns them on) out of EQOA-Data's quest guides " +
			"(github.com/Jadiction/EQOA-Data, Quests/**/*.json): one a guide's \"# Level N\" section, else one a file; the " +
			"giver, level, class, experience and reward items where they resolve by name to this server's NPCs and " +
			"items, one talk step to the giver, the guide's text in its notes. A quest made from a section before is " +
			"skipped. files are {path, text}; dryRun lists what it would make. Each row is logged and revertible.",
		Input: obj(map[string]any{
			"files": arr(obj(map[string]any{
				"path": str("the file's path within EQOA-Data, e.g. Quests/1-20/freeport/magician.json"),
				"text": str("the file's JSON"),
			}, "path", "text"), "the guides"),
			"dryRun": boolean("only list what it would make"),
		}, "files"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Files  []eqoaDataFile
				DryRun bool
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.questImport(c.a, req.Files, req.DryRun)
		},
	})
}

// quests is the running engine, or an error saying it is not.
func quests() (*handlers.QuestEngine, error) {
	q := handlers.Quests()
	if q == nil {
		return nil, errors.New("the quests are not running in this server")
	}
	return q, nil
}

func (s *Server) questList(search string) (any, error) {
	q := `SELECT q.questid, q.name, q.enabled, q.giver, q.minlevel, q.maxlevel, q.source,
		(SELECT COUNT(*) FROM queststeps st WHERE st.questid = q.questid) AS steps,
		(SELECT COUNT(*) FROM charquests c WHERE c.questid = q.questid AND c.step > 0) AS underway,
		(SELECT COUNT(*) FROM charquests c WHERE c.questid = q.questid AND c.done > 0) AS finished
		FROM quests q`
	var args []any
	if search = strings.TrimSpace(search); search != "" {
		like := "%" + likeEscape(search) + "%"
		q += ` WHERE q.name LIKE ? ESCAPE '\' OR q.giver LIKE ? ESCAPE '\' OR q.notes LIKE ? ESCAPE '\'`
		args = append(args, like, like, like)
		if id, err := parseInt("quest", search); err == nil {
			q += ` OR q.questid = ?`
			args = append(args, id)
		}
	}
	rows, err := s.rowsOf(q+` ORDER BY q.questid`, args...)
	if err != nil {
		return nil, err
	}
	return map[string]any{"quests": rows, "total": len(rows), "kinds": database.StepKinds}, nil
}

func (s *Server) questGet(id int64) (any, error) {
	quest, err := s.rowsOf(`SELECT * FROM quests WHERE questid = ?`, id)
	if err != nil {
		return nil, err
	}
	if len(quest) == 0 {
		return nil, notFound("no quest %d", id)
	}
	steps, err := s.rowsOf(`SELECT * FROM queststeps WHERE questid = ? ORDER BY step`, id)
	if err != nil {
		return nil, err
	}
	who, err := s.rowsOf(`SELECT c.serverid, ch.charName, c.step, c.count, c.done, c.updated FROM charquests c
		LEFT JOIN characters ch ON ch.serverid = c.serverid WHERE c.questid = ? ORDER BY c.updated DESC LIMIT 50`, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{"quest": quest[0], "steps": steps, "problems": s.questProblems(quest[0], steps), "characters": who}, nil
}

// questProblems is what keeps a quest from being done as written: names and
// ids this server does not have, a step no one can do, words the client cuts.
func (s *Server) questProblems(q map[string]any, steps []map[string]any) []string {
	text := func(r map[string]any, col string) string { v, _ := r[col].(string); return strings.TrimSpace(v) }
	num := func(r map[string]any, col string) float64 { v, _ := asFloat(r[col]); return v }
	var out []string
	add := func(format string, a ...any) { out = append(out, fmt.Sprintf(format, a...)) }
	if text(q, "name") == "" {
		add("it has no name")
	}
	if g := text(q, "giver"); g == "" {
		add("it has no giver: no one offers it (a GM can still give it)")
	} else {
		s.npcProblems(g, "the giver", add)
	}
	if lo, hi := num(q, "minlevel"), num(q, "maxlevel"); hi > 0 && lo > hi {
		add("its levels run backwards: %v to %v", lo, hi)
	}
	if text(q, "accept") == "" && text(q, "decline") != "" {
		add("it has an answer refusing it but none taking it: it is taken when talked to, and the refusal never shows")
	}
	if utf8.RuneCountInString(text(q, "accept")) > 63 || utf8.RuneCountInString(text(q, "decline")) > 63 {
		add("an answer is longer than 63 characters: the client cuts it")
	}
	self, _ := asFloat(q["questid"])
	for _, r := range database.ParseIntList(text(q, "requires")) {
		if float64(r) == self {
			add("it requires itself: no one can take it")
			continue
		}
		var n int
		s.d.DB.QueryRow(`SELECT COUNT(*) FROM quests WHERE questid = ?`, r).Scan(&n)
		if n == 0 {
			add("it requires quest %d, which does not exist", r)
		}
	}
	if len(steps) == 0 {
		add("it has no steps: it cannot be taken")
	}
	if strings.HasPrefix(text(q, "source"), "EQOA-Data") && len(steps) == 1 &&
		strings.EqualFold(text(steps[0], "kind"), database.StepTalk) && strings.EqualFold(text(steps[0], "target"), text(q, "giver")) {
		add("an imported draft: its steps are still the one talk to its giver; write them from its notes before turning it on")
	}
	for _, st := range steps {
		n := int(num(st, "step"))
		where := fmt.Sprintf("step %d", n)
		kind := strings.ToLower(text(st, "kind"))
		switch kind {
		case database.StepTalk, database.StepShow, database.StepGive:
			if t := text(st, "target"); t == "" {
				add("%s (%s) names no NPC", where, kind)
			} else {
				s.npcProblems(t, where+"'s NPC", add)
			}
			if kind != database.StepTalk {
				items := database.ParseItemCounts(text(st, "items"))
				if len(items) == 0 && num(st, "tunar") <= 0 {
					add("%s wants nothing held (no items, no tunar)", where)
				}
				s.itemProblems(items, where+" wants", add)
			}
		case database.StepKill:
			if t := text(st, "target"); t == "" {
				add("%s names no creature", where)
			} else {
				var n int
				s.d.DB.QueryRow(`SELECT COUNT(*) FROM npcs WHERE LOWER(npc_name) = LOWER(?)`, t).Scan(&n)
				if n == 0 {
					add("%s: no creature named %q spawns here, so it cannot be done", where, t)
				}
			}
			if num(st, "count") < 1 {
				add("%s kills fewer than one", where)
			}
		case database.StepReach:
			if num(st, "x") == 0 && num(st, "z") == 0 {
				add("%s's place is 0, 0", where)
			}
		default:
			add("%s: no kind %q (talk, show, give, kill or reach)", where, kind)
		}
		if text(st, "journal") == "" {
			add("%s has no journal line (the quest's name is shown)", where)
		} else if utf8.RuneCountInString(text(st, "journal")) > 127 {
			add("%s's journal line is longer than 127 characters: the client cuts it", where)
		}
		if utf8.RuneCountInString(text(st, "answer")) > 63 {
			add("%s's answer is longer than 63 characters: the client cuts it", where)
		}
		s.itemProblems(database.ParseItemCounts(text(st, "rewarditems")), where+" gives", add)
		for _, sp := range database.ParseIntList(text(st, "spells")) {
			var n int
			s.d.DB.QueryRow(`SELECT COUNT(*) FROM spellpattern WHERE spellid = ?`, sp).Scan(&n)
			if n == 0 {
				add("%s gives spell %d, which does not exist", where, sp)
			}
		}
		for _, f := range database.ParseFactionDeltas(text(st, "factions")) {
			var n int
			s.d.DB.QueryRow(`SELECT COUNT(*) FROM factions WHERE faction = ?`, f.Faction).Scan(&n)
			if n == 0 {
				add("%s moves faction %q, which this server does not have", where, f.Faction)
			}
		}
	}
	return out
}

// npcProblems says what is wrong with an NPC a quest names: none of the name,
// or one that opens a window (the client never talks to a merchant, banker
// or blacksmith: InteractWithTarget sends their windows' requests instead).
func (s *Server) npcProblems(name, what string, add func(string, ...any)) {
	rows, err := s.rowsOf(`SELECT npc_type FROM npcs WHERE LOWER(npc_name) = LOWER(?) LIMIT 20`, name)
	if err != nil {
		return
	}
	if len(rows) == 0 {
		add("%s: no NPC is named %q", what, name)
		return
	}
	for _, r := range rows {
		if t, _ := asFloat(r["npc_type"]); int64(t)&0x0B != 0 {
			add("%s %q opens a window (merchant, bank or blacksmith): the game never talks to it", what, name)
			return
		}
	}
}

// itemProblems says which items do not exist.
func (s *Server) itemProblems(items []database.ItemCount, what string, add func(string, ...any)) {
	for _, it := range items {
		var n int
		s.d.DB.QueryRow(`SELECT COUNT(*) FROM itempattern WHERE patternid = ?`, it.PatternID).Scan(&n)
		if n == 0 {
			add("%s item %d, which does not exist", what, it.PatternID)
		}
	}
}

func (s *Server) questSave(a *actor, quest map[string]any, steps []map[string]any) (any, error) {
	qt, err := s.table("quests")
	if err != nil {
		return nil, err
	}
	st, err := s.table("queststeps")
	if err != nil {
		return nil, err
	}
	pick := func(src map[string]any, cols []string) map[string]any {
		out := map[string]any{}
		for _, c := range cols {
			if v, ok := src[c]; ok {
				out[c] = v
			}
		}
		return out
	}
	vals := pick(quest, questCols)
	if n, ok := vals["name"].(string); !ok || strings.TrimSpace(n) == "" {
		return nil, badRequest("a quest needs a name")
	}
	for i, step := range steps {
		k, _ := step["kind"].(string)
		if !containsStr(database.StepKinds, strings.ToLower(strings.TrimSpace(k))) {
			return nil, badRequest("step %d: kind is talk, show, give, kill or reach", i+1)
		}
	}
	id, _ := asFloat(quest["questid"])
	var changes []*change
	if id <= 0 {
		ch, err := s.insertRow(a, qt, vals, writeOpts{})
		if err != nil {
			return nil, err
		}
		changes = append(changes, ch)
		id, _ = asFloat(ch.Key["questid"])
	} else {
		cur, err := getRow(s.d.DB, qt, map[string]any{"questid": int64(id)})
		if err != nil {
			return nil, notFound("no quest %d", int64(id))
		}
		diff := map[string]any{}
		for k, v := range vals {
			if !sameValue(v, cur[k]) {
				diff[k] = v
			}
		}
		if len(diff) > 0 {
			ch, err := s.updateRow(a, qt, map[string]any{"questid": int64(id)}, diff, nil, writeOpts{})
			if err != nil {
				return nil, err
			}
			changes = append(changes, ch)
		}
	}
	old, err := s.rowsOf(`SELECT * FROM queststeps WHERE questid = ? ORDER BY step`, int64(id))
	if err != nil {
		return nil, err
	}
	byStep := map[int64]map[string]any{}
	for _, r := range old {
		n, _ := asFloat(r["step"])
		byStep[int64(n)] = r
	}
	for i, step := range steps {
		n := int64(i + 1)
		v := pick(step, stepCols)
		if k, ok := v["kind"].(string); ok {
			v["kind"] = strings.ToLower(strings.TrimSpace(k))
		}
		key := map[string]any{"questid": int64(id), "step": n}
		if cur, ok := byStep[n]; ok {
			diff := map[string]any{}
			for k, val := range v {
				if !sameValue(val, cur[k]) {
					diff[k] = val
				}
			}
			if len(diff) > 0 {
				ch, err := s.updateRow(a, st, key, diff, nil, writeOpts{})
				if err != nil {
					return nil, fmt.Errorf("step %d: %w", n, err)
				}
				changes = append(changes, ch)
			}
			continue
		}
		v["questid"], v["step"] = int64(id), n
		ch, err := s.insertRow(a, st, v, writeOpts{})
		if err != nil {
			return nil, fmt.Errorf("step %d: %w", n, err)
		}
		changes = append(changes, ch)
	}
	for n := range byStep {
		if n > int64(len(steps)) {
			ch, err := s.deleteRow(a, st, map[string]any{"questid": int64(id), "step": n}, writeOpts{})
			if err != nil {
				return nil, fmt.Errorf("step %d: %w", n, err)
			}
			changes = append(changes, ch)
		}
	}
	out, err := s.questGet(int64(id))
	if err != nil {
		return nil, err
	}
	m := out.(map[string]any)
	m["changes"] = len(changes)
	live := "nothing changed"
	if len(changes) > 0 {
		live = changes[len(changes)-1].Live
	}
	m["live"] = live
	return m, nil
}

func containsStr(v []string, s string) bool {
	for _, x := range v {
		if x == s {
			return true
		}
	}
	return false
}

func (s *Server) questDelete(a *actor, id int64, dry bool) (any, error) {
	qt, err := s.table("quests")
	if err != nil {
		return nil, err
	}
	if _, err := getRow(s.d.DB, qt, map[string]any{"questid": id}); err != nil {
		return nil, notFound("no quest %d", id)
	}
	var steps, chars int64
	s.d.DB.QueryRow(`SELECT COUNT(*) FROM queststeps WHERE questid = ?`, id).Scan(&steps)
	s.d.DB.QueryRow(`SELECT COUNT(*) FROM charquests WHERE questid = ?`, id).Scan(&chars)
	if dry {
		return map[string]any{"dryRun": true, "steps": steps, "characters": chars}, nil
	}
	ct, _ := s.table("charquests")
	st, _ := s.table("queststeps")
	cq, err := s.rowsOf(`SELECT serverid FROM charquests WHERE questid = ?`, id)
	if err != nil {
		return nil, err
	}
	for _, r := range cq {
		sid, _ := asFloat(r["serverid"])
		if _, err := s.deleteRow(a, ct, map[string]any{"serverid": int64(sid), "questid": id}, writeOpts{}); err != nil {
			return nil, err
		}
	}
	sr, err := s.rowsOf(`SELECT step FROM queststeps WHERE questid = ?`, id)
	if err != nil {
		return nil, err
	}
	for _, r := range sr {
		n, _ := asFloat(r["step"])
		if _, err := s.deleteRow(a, st, map[string]any{"questid": id, "step": int64(n)}, writeOpts{}); err != nil {
			return nil, err
		}
	}
	ch, err := s.deleteRow(a, qt, map[string]any{"questid": id}, writeOpts{})
	if err != nil {
		return nil, err
	}
	return map[string]any{"deleted": id, "steps": steps, "characters": chars, "live": ch.Live}, nil
}

func (s *Server) characterQuests(who any) (any, error) {
	id, name, err := s.characterRef(who)
	if err != nil {
		return nil, err
	}
	q, err := quests()
	if err != nil {
		return nil, err
	}
	_, online := s.onlineCharacter(uint32(id))
	return map[string]any{"serverid": id, "character": name, "online": online, "quests": q.CharacterQuests(uint32(id)),
		"actions": handlers.QuestActions}, nil
}

// charQuestRow is a character's charquests row, or nil.
func (s *Server) charQuestRow(serverID int64, questID int) map[string]any {
	t, err := s.table("charquests")
	if err != nil {
		return nil
	}
	row, err := getRow(s.d.DB, t, map[string]any{"serverid": serverID, "questid": int64(questID)})
	if err != nil {
		return nil
	}
	delete(row, "__key")
	return row
}

// auditQuestRow logs a character's quest row before and after an engine
// change, as the row change it was (revertible).
func (s *Server) auditQuestRow(a *actor, serverID int64, questID int, before, after map[string]any, detail string) {
	action := "update"
	switch {
	case before == nil && after == nil:
		return
	case before == nil:
		action = "insert"
	case after == nil:
		action = "delete"
	}
	s.auditTx(s.d.DB, a, action, "charquests", map[string]any{"serverid": serverID, "questid": int64(questID)}, before, after, detail)
}

func (s *Server) characterQuestSet(a *actor, who any, questID int, action string, step int) (any, error) {
	id, name, err := s.characterRef(who)
	if err != nil {
		return nil, err
	}
	if err := s.guardCharacter(s.d.DB, a, id); err != nil {
		return nil, err
	}
	q, err := quests()
	if err != nil {
		return nil, err
	}
	before := s.charQuestRow(id, questID)
	msg, err := q.SetCharacterQuest(uint32(id), questID, action, step)
	if err != nil {
		return nil, badRequest("%v", err)
	}
	after := s.charQuestRow(id, questID)
	s.auditQuestRow(a, id, questID, before, after, fmt.Sprintf("%s: quest %d %s (%s)", name, questID, action, msg))
	return map[string]any{"message": fmt.Sprintf("%s: %s", name, msg), "quests": q.CharacterQuests(uint32(id))}, nil
}

// gmSession is the actor's own character in the world, or nil.
func (s *Server) gmSession(a *actor) *session.Session {
	if s.d.Sessions == nil || s.d.World == nil {
		return nil
	}
	var found *session.Session
	s.d.Sessions.ForEach(func(sess *session.Session) bool {
		if sess.AccountID() != a.AccountID || !sess.CharacterInWorld() || sess.PendingTermination() {
			return true
		}
		if _, replaced := sess.ReplacedFor(); replaced {
			return true
		}
		if p := s.d.World.SessionPlayer(sess); p != nil && s.d.World.IsLivePlayer(p) {
			found = sess
			return false
		}
		return true
	})
	return found
}

func (s *Server) questTry(a *actor, questID int) (any, error) {
	q, err := quests()
	if err != nil {
		return nil, err
	}
	sess := s.gmSession(a)
	if sess == nil {
		return nil, badRequest("you have no character in the world: log in to the game, then try it")
	}
	p := s.d.World.SessionPlayer(sess)
	id := int64(p.CharServerID)
	before := s.charQuestRow(id, questID)
	msg, err := q.GiveToSession(sess, questID)
	if err != nil {
		return nil, badRequest("%v", err)
	}
	after := s.charQuestRow(id, questID)
	s.auditQuestRow(a, id, questID, before, after, fmt.Sprintf("%s tries quest %d", p.Name, questID))
	return map[string]any{"message": fmt.Sprintf("%s: %s", p.Name, msg), "character": p.Name}, nil
}

// liveQuests is a quests or queststeps row change applied to the running game.
func (s *Server) liveQuests() string {
	q := handlers.Quests()
	if q == nil {
		return "the quests are not running in this server"
	}
	return q.ReloadQuests()
}

// liveCharQuest is a charquests row change applied where the character stands.
func (s *Server) liveCharQuest(before, after map[string]any) string {
	q := handlers.Quests()
	if q == nil {
		return "at the next start"
	}
	row := after
	if row == nil {
		row = before
	}
	id, ok := asFloat(row["serverid"])
	if !ok {
		return "at the character's next login"
	}
	return q.ReloadCharacter(uint32(id))
}
