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
	"repeatable", "offer", "accept", "decline", "declined", "started", "source", "notes", "events"}

// stepCols are a step row's columns the editor sends.
var stepCols = []string{"kind", "target", "npcid", "items", "tunar", "count", "world", "x", "y", "z", "radius",
	"journal", "answer", "says", "waiting", "notyet", "reminder", "xp", "rewardtunar", "rewarditems", "factions", "spells",
	"chance", "events"}

// eventsHelp is what an events column holds, for the schemas.
const eventsHelp = `a JSON list of events, each {"on", "do", ...}: popup {text, answer} a window the player closes ` +
	`(one answer, "OK" when none); say {npc, text} an NPC's words; give {items: "patternid*count, ..."}; ` +
	`spawn {npc, hostile} a copy of that NPC before the player for the step (hostile: it may be attacked and attacks); ` +
	`visual {fx: "0x1DAEB5B6", seconds} a spell visual on the player; teleport {world, x, y, z}. ` +
	`text may hold %p, the player's name. Empty: none`

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
		"events":     str("what happens when it is taken (on accept) and finished (on finish): " + eventsHelp),
	})
	stepSchema := obj(map[string]any{
		"kind":        strEnum("talk: talk to target; show: talk to target holding items (kept); give: hand target items and tunar (taken); kill: kill count of target; loot: kill target until count of the item in items have been taken off their corpses during the step, each kill dropping one on the corpse with chance; reach: come within radius of x, z in world", database.StepKinds...),
		"target":      str("talk/give: the NPC's name; kill/loot: the creature's name"),
		"npcid":       integer("only that spawn (npcs.id); 0: any of the name"),
		"items":       str("show/give: items held (show) or handed in (give), patternid*count, comma separated; loot: the item that drops"),
		"tunar":       integer("show/give: tunar held or handed in"),
		"count":       integer("kill: how many; loot: how many of the item are to be taken off corpses during the step"),
		"chance":      number("loot: the chance (0 to 1) that a kill drops the item for a player on the step; 0: every kill"),
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
		"events":      str("what happens when it becomes the step to do (on start) and when it is done (on done): " + eventsHelp),
	})
	filterFields := map[string]any{
		"search":   str("part of a name, giver, note or import key, or a quest's id"),
		"status":   strEnum("in the game; draft (every draft); ready (a draft nothing keeps out now: every name it uses is here and every quest it requires is in the game or ready too); blocked (a draft that is not ready); empty: all", "", questInGame, "draft", questReady, "blocked"),
		"city":     str(`the city of its import key ("freeport", "neriak-nektulos"; quest_list's cities lists them); "none": quests of no city`),
		"class":    integer("a class id (0 Warrior ... 14 Alchemist): the quests for it, and those for any class"),
		"level":    integer("a level: the quests whose levels take it in"),
		"minLevel": integer("the quests whose levels reach this one or above"),
		"maxLevel": integer("the quests whose levels start at this one or below"),
		"giver":    str("part of the giver's name"),
		"missing":  str("a name drafts lack (quest_list's missing lists them): the quests that lack it"),
	}
	listFields := map[string]any{
		"limit":  integer("how many quests to return (default 300, at most 2000)"),
		"offset": integer("how many to skip first"),
	}
	for k, v := range filterFields {
		listFields[k] = v
	}
	s.addOp(&Op{
		Name: "quest_list", Title: "List quests", Scope: scopeRead, Core: true,
		Description: "Every quest (drafts too), as the filters narrow them, in id order: its status (in the game, ready: a draft " +
			"nothing keeps out now, or draft), why a draft is one (the names it lacks NOW, read off this server's NPCs, items, " +
			"spells and factions, and a draft quest it requires), its city (its import key's), classes, levels, giver, steps " +
			"and how many characters have it under way or finished. The answer also gives total (what the filters take), " +
			"counts by status, every city with its counts, and missing: the names the drafts taken lack, the most wanted " +
			"first (an NPC placed or an item made frees every quest that lacked only it).",
		Input: obj(listFields),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				questFilter
				Limit, Offset int
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.questList(req.questFilter, req.Limit, req.Offset)
		},
	})
	s.addOp(&Op{
		Name: "quest_set_enabled", Title: "Put quests in the game, or take them out", Scope: scopeWrite, Core: true,
		Description: "Turn quests on (in the game: their givers offer them) or off (drafts: offered to no one; a character " +
			"who has one under way goes on with it) in one go: the quests named, or every quest a filter (quest_list's) " +
			"takes. Turning on takes only ready drafts (see quest_list) unless force, and brings in the ready drafts they " +
			"require (a chain goes in together); a draft that is not ready is left out with its reasons. Each quest is its " +
			"own row change, logged and revertible. dryRun says what would change.",
		Input: obj(map[string]any{
			"quests":  arr(integer("a quest's id"), "the quests to change (else the filter's)"),
			"filter":  obj(filterFields),
			"enabled": boolean("true: in the game; false: drafts"),
			"dryRun":  boolean("only say what would change"),
			"force":   boolean("turn on drafts that are not ready too (their reasons are said)"),
		}, "enabled"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Quests  []int64
				Filter  *questFilter
				Enabled *bool
				DryRun  bool
				Force   bool
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			if req.Enabled == nil {
				return nil, badRequest("enabled is true or false")
			}
			return c.s.questSetEnabled(c.a, req.Quests, req.Filter, *req.Enabled, req.DryRun, req.Force)
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

func (s *Server) questList(f questFilter, limit, offset int) (any, error) {
	idx, err := s.loadQuestIndex()
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 300
	}
	limit = min(limit, 2000)
	all := idx.match(f)
	shown := all[min(max(offset, 0), len(all)):]
	shown = shown[:min(limit, len(shown))]
	missing := missingOf(all)
	if len(missing) > 50 {
		missing = missing[:50]
	}
	if shown == nil {
		shown = []*questRow{}
	}
	return map[string]any{"quests": shown, "total": len(all), "counts": statusCounts(all), "cities": idx.cities(),
		"missing": missing, "kinds": database.StepKinds}, nil
}

// questSetEnabled turns quests on or off, each its own audited row change,
// the game told once at the end.
func (s *Server) questSetEnabled(a *actor, ids []int64, f *questFilter, on, dry, force bool) (any, error) {
	if len(ids) == 0 && (f == nil || f.empty()) {
		return nil, badRequest("name the quests, or give a filter (quest_list's)")
	}
	qt, err := s.table("quests")
	if err != nil {
		return nil, err
	}
	idx, err := s.loadQuestIndex()
	if err != nil {
		return nil, err
	}
	type item struct {
		ID       int64  `json:"questid"`
		Name     string `json:"name"`
		Why      string `json:"why,omitempty"`
		Required bool   `json:"required,omitempty"` // brought in as a quest a chosen one requires
	}
	var targets []*questRow
	var skipped []item
	if len(ids) > 0 {
		for _, id := range ids {
			if r := idx.byID[id]; r != nil {
				targets = append(targets, r)
			} else {
				skipped = append(skipped, item{ID: id, Why: "no such quest"})
			}
		}
	} else {
		targets = idx.match(*f)
	}
	chosen := map[int64]bool{}
	required := map[int64]bool{}
	var order []*questRow
	add := func(r *questRow) {
		if !chosen[r.ID] {
			chosen[r.ID] = true
			order = append(order, r)
		}
	}
	for _, r := range targets {
		switch {
		case r.Enabled == on:
			// already so
		case !on, r.Status == questReady, force:
			add(r)
		default:
			skipped = append(skipped, item{ID: r.ID, Name: r.Name, Why: strings.Join(r.Why, "; ")})
		}
	}
	if on {
		// the drafts a chosen quest requires go in with it
		for i := 0; i < len(order); i++ {
			for _, req := range order[i].Requires {
				if q := idx.byID[req]; q != nil && !q.Enabled && !chosen[req] && (q.Status == questReady || force) {
					required[req] = true
					add(q)
				}
			}
		}
	}
	changed := []item{}
	for _, r := range order {
		ch, err := s.updateRow(a, qt, map[string]any{"questid": r.ID}, map[string]any{"enabled": boolInt(on)}, nil,
			writeOpts{Dry: dry, NoLive: true})
		if err != nil {
			return nil, fmt.Errorf("quest %d: %w", r.ID, err)
		}
		it := item{ID: r.ID, Name: r.Name, Required: required[r.ID]}
		if on && r.Status != questReady {
			it.Why = "forced: " + strings.Join(r.Why, "; ")
		}
		if ch.Action != "unchanged" {
			changed = append(changed, it)
		}
	}
	live := "nothing changed"
	switch {
	case dry:
		live = "a dry run: nothing changed"
	case len(changed) > 0:
		live = s.liveQuests()
	}
	if skipped == nil {
		skipped = []item{}
	}
	return map[string]any{"changed": changed, "skipped": skipped, "enabled": on, "dryRun": dry, "live": live}, nil
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
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
	out := map[string]any{"quest": quest[0], "steps": steps, "problems": s.questProblems(quest[0], steps), "characters": who}
	// where it stands among the rest: its status and why, the quests it
	// requires and those that require it
	if idx, err := s.loadQuestIndex(); err == nil {
		if r := idx.byID[id]; r != nil {
			out["status"], out["why"], out["city"], out["key"] = r.Status, r.Why, r.City, r.Key
			type link struct {
				ID     int64  `json:"questid"`
				Name   string `json:"name"`
				Status string `json:"status"`
			}
			before, after := []link{}, []link{}
			for _, req := range r.Requires {
				if q := idx.byID[req]; q != nil {
					before = append(before, link{q.ID, q.Name, q.Status})
				}
			}
			for _, q := range idx.rows {
				for _, req := range q.Requires {
					if req == id {
						after = append(after, link{q.ID, q.Name, q.Status})
						break
					}
				}
			}
			out["before"], out["after"] = before, after
		}
	}
	return out, nil
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
		s.npcProblems(g, "the giver", true, add)
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
	s.eventProblems(text(q, "events"), false, "the quest's", add)
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
				s.npcProblems(t, where+"'s NPC", false, add)
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
		case database.StepLoot:
			if t := text(st, "target"); t == "" {
				add("%s names no creature", where)
			} else {
				var n int
				s.d.DB.QueryRow(`SELECT COUNT(*) FROM npcs WHERE LOWER(npc_name) = LOWER(?)`, t).Scan(&n)
				if n == 0 {
					add("%s: no creature named %q spawns here, so nothing drops its item", where, t)
				}
			}
			items := database.ParseItemCounts(text(st, "items"))
			if len(items) == 0 {
				add("%s names no item to drop", where)
			}
			s.itemProblems(items, where+" drops", add)
			if c := num(st, "chance"); c < 0 || c > 1 {
				add("%s's chance %v is not between 0 and 1 (0 is every kill)", where, c)
			}
		case database.StepReach:
			if num(st, "x") == 0 && num(st, "z") == 0 {
				add("%s's place is 0, 0", where)
			}
		default:
			add("%s: no kind %q (talk, show, give, kill, loot or reach)", where, kind)
		}
		s.eventProblems(text(st, "events"), true, where+"'s", add)
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
			s.d.DB.QueryRow(`SELECT COUNT(*) FROM factions WHERE name = ?`, f.Faction).Scan(&n)
			if n == 0 {
				add("%s moves faction %q, which this server does not have", where, f.Faction)
			}
		}
	}
	return out
}

// npcProblems says what is wrong with an NPC a quest names: none of the name,
// or, for its giver (window), one that opens a window (the client never talks
// to a merchant, banker or blacksmith: InteractWithTarget asks for its window
// instead, so it offers nothing; a step at one is done when its window is
// asked for, handlers.QuestEngine.AtWindow).
func (s *Server) npcProblems(name, what string, window bool, add func(string, ...any)) {
	rows, err := s.rowsOf(`SELECT npc_type FROM npcs WHERE LOWER(npc_name) = LOWER(?) LIMIT 20`, name)
	if err != nil {
		return
	}
	if len(rows) == 0 {
		add("%s: no NPC is named %q", what, name)
		return
	}
	for _, r := range rows {
		if t, _ := asFloat(r["npc_type"]); window && int64(t)&0x0B != 0 {
			add("%s %q opens a window (merchant, bank or blacksmith): the game never talks to it, so it offers nothing", what, name)
			return
		}
	}
}

// eventProblems says what is wrong with an events column (a step's when step):
// its form (database.EventProblems), and the NPCs and items it names that
// this server lacks.
func (s *Server) eventProblems(events string, step bool, what string, add func(string, ...any)) {
	for _, p := range database.EventProblems(events, step) {
		add("%s %s", what, p)
	}
	evs, err := database.ParseQuestEvents(events)
	if err != nil {
		return
	}
	for i, e := range evs {
		where := fmt.Sprintf("%s event %d", what, i+1)
		switch e.Do {
		case database.EventSpawn:
			var n int
			s.d.DB.QueryRow(`SELECT COUNT(*) FROM npcs WHERE LOWER(npc_name) = LOWER(?)`, strings.TrimSpace(e.NPC)).Scan(&n)
			if n == 0 && strings.TrimSpace(e.NPC) != "" {
				add("%s spawns %q, which this server has no NPC of", where, e.NPC)
			}
		case database.EventGive:
			s.itemProblems(database.ParseItemCounts(e.Items), where+" gives", add)
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
			return nil, badRequest("step %d: kind is talk, show, give, kill, loot or reach", i+1)
		}
	}
	id, _ := asFloat(quest["questid"])
	var changes []*change
	if id <= 0 {
		ch, err := s.insertRow(a, qt, vals, writeOpts{NoLive: true})
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
			ch, err := s.updateRow(a, qt, map[string]any{"questid": int64(id)}, diff, nil, writeOpts{NoLive: true})
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
				ch, err := s.updateRow(a, st, key, diff, nil, writeOpts{NoLive: true})
				if err != nil {
					return nil, fmt.Errorf("step %d: %w", n, err)
				}
				changes = append(changes, ch)
			}
			continue
		}
		v["questid"], v["step"] = int64(id), n
		ch, err := s.insertRow(a, st, v, writeOpts{NoLive: true})
		if err != nil {
			return nil, fmt.Errorf("step %d: %w", n, err)
		}
		changes = append(changes, ch)
	}
	for n := range byStep {
		if n > int64(len(steps)) {
			ch, err := s.deleteRow(a, st, map[string]any{"questid": int64(id), "step": n}, writeOpts{NoLive: true})
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
		live = s.liveQuests() // once for the quest and all its steps
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
