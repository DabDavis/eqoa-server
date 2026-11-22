package gmweb

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/eqoa/server/internal/handlers"
)

// quest_definition_patch (board entry 1250): a reviewed content patch of quests and
// what names them, applied as ONE guarded change. The manifest gives every row it
// changes in full, as it must be now (before) and as it will be (after); one column
// moved since (drift), a quest anyone has (a charquests row of any state, or a
// character's memory), a row missing, or anything refused changes nothing at all.
// The rows are written in one transaction with an audit line each, and the quests
// are published to the game once, held out of play from before the first check to
// that publication (handlers/questdefgate.go: the lock order is there).
//
// It is an admin's tool (an authenticator sign-in). It applies no patch by itself.

// questPatchTables are the tables a quest's definition patch may change: the quest,
// its steps, and the NPC, rule rows and item record a rename names.
var questPatchTables = map[string]bool{
	"quests": true, "queststeps": true, "npcs": true, "npcclass": true, "npcfaction": true, "itempattern": true,
}

// questPatchRow is one row of a manifest.
type questPatchRow struct {
	Table  string         `json:"table"`
	Key    map[string]any `json:"key"`
	Before map[string]any `json:"before"`
	After  map[string]any `json:"after"`
}

// questPatchFault fails the patch at a write by its index (each row, then the
// commit), for the tests.
var questPatchFault func(i int) error

func (s *Server) registerQuestPatchOps() {
	s.addOp(&Op{
		Name: "quest_definition_patch", Title: "Quest definition patch (guarded)", Scope: scopeAdmin, Destructive: true,
		Description: "A reviewed content patch of quests applied as ONE guarded change: rows of quests, queststeps, npcs, " +
			"npcclass, npcfaction and itempattern, each given whole as it is now (before: every column) and as it will be " +
			"(after: every column, the key unchanged). Refused, changing nothing, when any row is missing or has moved from " +
			"its before (drift), when anyone has one of the quests (a charquests row of any state, finished included, or " +
			"a character's quests in memory), or on any error. The quests are those the quest and step rows name plus " +
			"quests; they are held out of play (no offer, accept, old answer, step or hand-in) from before the checks until " +
			"the game has read them again, once. dry_run checks everything and writes nothing. Every row is audited " +
			"before and after with detail (required: the review or board entry it applies).",
		Input: obj(map[string]any{
			"rows": arr(obj(map[string]any{
				"table":  str("quests, queststeps, npcs, npcclass, npcfaction or itempattern"),
				"key":    freeObject("the row's key (the table's key columns)"),
				"before": freeObject("the whole row as it must be now"),
				"after":  freeObject("the whole row as it will be"),
			}, "table", "key", "before", "after"), "the rows the patch changes, each once"),
			"quests":  arr(integer("a quest id"), "more quests to hold and check (a quest an item or NPC row belongs to)"),
			"detail":  str("why: the review or the board entry this applies (required)"),
			"dry_run": boolean("check everything, write nothing"),
		}, "rows", "detail"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Rows   []questPatchRow `json:"rows"`
				Quests []int           `json:"quests"`
				Detail string          `json:"detail"`
				DryRun bool            `json:"dry_run"`
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.questDefinitionPatch(c.a, req.Rows, req.Quests, req.Detail, req.DryRun)
		},
	})
}

// questPatchPlanned is a manifest row checked: its table and key read.
type questPatchPlanned struct {
	row   questPatchRow
	t     *Table
	key   map[string]any
	set   map[string]any // the columns that change, as SQL values
	label string
}

// questDefinitionPatch checks and applies a manifest (quest_definition_patch).
func (s *Server) questDefinitionPatch(a *actor, rows []questPatchRow, extra []int, detail string, dry bool) (any, error) {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return nil, badRequest("detail is required: the review or board entry this patch applies")
	}
	if len(rows) == 0 {
		return nil, badRequest("the manifest has no rows")
	}
	plans, ids, err := s.planQuestPatch(rows, extra)
	if err != nil {
		return nil, err
	}
	q, err := quests()
	if err != nil {
		return nil, err
	}
	commit := func() error {
		tx, err := s.d.DB.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		// every before exactly as it is, in the transaction that writes
		var drift []string
		for _, p := range plans {
			cur, err := getRow(tx, p.t, p.key)
			if err != nil {
				return badRequest("%s: %v", p.label, err)
			}
			delete(cur, "__key")
			for name, want := range p.row.Before {
				if !sameValue(want, cur[name]) {
					drift = append(drift, fmt.Sprintf("%s.%s is %v, not %v", p.label, name, cur[name], want))
				}
			}
		}
		if len(drift) > 0 {
			sort.Strings(drift)
			return badRequest("refused, nothing changed: the rows moved from the manifest's before (%s)", strings.Join(drift, "; "))
		}
		marks := make([]string, len(ids))
		args := make([]any, len(ids))
		for i, id := range ids {
			marks[i], args[i] = "?", id
		}
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM charquests WHERE questid IN (`+strings.Join(marks, ", ")+`)`, args...).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return badRequest("refused, nothing changed: %d character row(s) of quests %v (under way or finished); no character is changed by this tool", n, ids)
		}
		if dry {
			return errDryRun
		}
		for i, p := range plans {
			if questPatchFault != nil {
				if err := questPatchFault(i); err != nil {
					return err
				}
			}
			var sets []string
			var vals []any
			for _, col := range p.t.Columns {
				if v, ok := p.set[col.Name]; ok {
					sets = append(sets, quoteIdent(col.Name)+" = ?")
					vals = append(vals, v)
				}
			}
			w, kargs, err := keyWhere(p.t, p.key)
			if err != nil {
				return err
			}
			res, err := tx.Exec(`UPDATE `+quoteIdent(p.t.Name)+` SET `+strings.Join(sets, ", ")+` WHERE `+w, append(vals, kargs...)...)
			if err != nil {
				return sqlError(err)
			}
			if k, _ := res.RowsAffected(); k != 1 {
				return fmt.Errorf("%s: %d rows changed, want 1", p.label, k)
			}
			after, err := getRow(tx, p.t, p.key)
			if err != nil {
				return err
			}
			delete(after, "__key")
			for name, want := range p.row.After {
				if !sameValue(want, after[name]) {
					return badRequest("%s.%s reads %v after the write, not the manifest's %v", p.label, name, after[name], want)
				}
			}
			if _, err := s.auditTx(tx, a, "update", p.t.Name, p.key, p.row.Before, after, "quest definition patch: "+detail); err != nil {
				return err
			}
		}
		if questPatchFault != nil {
			if err := questPatchFault(len(plans)); err != nil {
				return err
			}
		}
		return tx.Commit()
	}
	gen, err := q.GuardedEdit(ids, commit)
	if errors.Is(err, errDryRun) {
		return map[string]any{"dry_run": true, "rows": len(plans), "quests": ids,
			"message": fmt.Sprintf("%d row(s) checked: as the manifest's before, no character has quests %v", len(plans), ids)}, nil
	}
	if errors.Is(err, handlers.ErrQuestInPlay) {
		return nil, badRequest("refused, nothing changed: %v", err)
	}
	if errors.Is(err, handlers.ErrQuestUnpublished) {
		return nil, &opError{500, err.Error()}
	}
	if err != nil {
		return nil, err
	}
	// the rows besides the quests' own, where they stand (an NPC renamed, its rule rows)
	var live []string
	for _, p := range plans {
		if p.t.Name == "quests" || p.t.Name == "queststeps" {
			continue
		}
		live = append(live, p.label+": "+s.applyLive(a, p.t, "update", p.row.Before, p.row.After))
	}
	return map[string]any{"rows": len(plans), "quests": ids, "generation": gen, "live": live,
		"message": fmt.Sprintf("%d row(s) changed in one transaction, each audited; quests %v published once (generation %d)",
			len(plans), ids, gen)}, nil
}

// errDryRun ends a dry run's transaction unwritten.
var errDryRun = errors.New("dry run")

// planQuestPatch checks a manifest's shape: its tables, whole rows, unchanged
// keys, each row once, something changing; and the quests it holds.
func (s *Server) planQuestPatch(rows []questPatchRow, extra []int) ([]questPatchPlanned, []int, error) {
	seen := map[string]bool{}
	idset := map[int]bool{}
	for _, id := range extra {
		idset[id] = true
	}
	var plans []questPatchPlanned
	for i, r := range rows {
		label := fmt.Sprintf("row %d (%s)", i+1, r.Table)
		if !questPatchTables[r.Table] {
			return nil, nil, badRequest("%s: a quest definition patch changes only quests, queststeps, npcs, npcclass, npcfaction and itempattern", label)
		}
		t, err := s.table(r.Table)
		if err != nil {
			return nil, nil, badRequest("%s: %v", label, err)
		}
		if t.RowID || len(t.Key) == 0 {
			return nil, nil, badRequest("%s: the table has no key of its own", label)
		}
		key := map[string]any{}
		for _, k := range t.Key {
			v, ok := r.Key[k]
			if !ok {
				return nil, nil, badRequest("%s: the key is %s; %q is missing", label, strings.Join(t.Key, " + "), k)
			}
			key[k] = v
			if !sameValue(v, r.Before[k]) || !sameValue(v, r.After[k]) {
				return nil, nil, badRequest("%s: the key's %s must be the same in key, before and after", label, k)
			}
		}
		if len(r.Key) != len(t.Key) {
			return nil, nil, badRequest("%s: the key names columns besides %s", label, strings.Join(t.Key, " + "))
		}
		label = fmt.Sprintf("%s %s", r.Table, keyText(key))
		if seen[label] {
			return nil, nil, badRequest("%s is in the manifest twice", label)
		}
		seen[label] = true
		for _, c := range t.Columns {
			if _, ok := r.Before[c.Name]; !ok {
				return nil, nil, badRequest("%s: before lacks %s (give the whole row)", label, c.Name)
			}
			if _, ok := r.After[c.Name]; !ok {
				return nil, nil, badRequest("%s: after lacks %s (give the whole row)", label, c.Name)
			}
		}
		if len(r.Before) != len(t.Columns) || len(r.After) != len(t.Columns) {
			return nil, nil, badRequest("%s: before and after name columns the table does not have", label)
		}
		set := map[string]any{}
		for _, c := range t.Columns {
			if sameValue(r.Before[c.Name], r.After[c.Name]) {
				continue
			}
			v, err := toSQL(t.col(c.Name), r.After[c.Name])
			if err != nil {
				return nil, nil, badRequest("%s.%s: %v", label, c.Name, err)
			}
			set[c.Name] = v
		}
		if len(set) == 0 {
			return nil, nil, badRequest("%s: before and after are the same", label)
		}
		if r.Table == "quests" || r.Table == "queststeps" {
			f, ok := asFloat(key["questid"])
			if !ok {
				return nil, nil, badRequest("%s: no quest id", label)
			}
			idset[int(f)] = true
		}
		plans = append(plans, questPatchPlanned{row: r, t: t, key: key, set: set, label: label})
	}
	if len(idset) == 0 {
		return nil, nil, badRequest("the patch names no quest: give quests the rows belong to")
	}
	ids := make([]int, 0, len(idset))
	for id := range idset {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return plans, ids, nil
}
