package charmove

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/eqoa/server/internal/database"
)

// QuestDone is a quest a plan marks done (a transfer's path, epic, were line,
// flags; the user, 2026-11-05: "add path, were or vamp, flags completed,
// quests"): its charquests row finished (step 0, done at least once) with the
// picks given, and the spells its steps give the character's class, and the
// picked options', into the book. No experience, tunar, faction or item: the
// plan's items bring the gear.
type QuestDone struct {
	Quest int `json:"quest"`
	// Picks are the options taken at its choices, "step:option, ..." from 0
	// (database.FormatPicks); "" keeps the row's own.
	Picks string `json:"picks,omitempty"`
}

// checkQuests is Check's part for the quests: each once, the picks readable.
func checkQuests(qs []QuestDone) error {
	seen := map[int]bool{}
	for _, q := range qs {
		if q.Quest <= 0 || seen[q.Quest] {
			return fmt.Errorf("a quest done names its quest once (%d)", q.Quest)
		}
		seen[q.Quest] = true
		parts := 0
		for _, f := range strings.Split(q.Picks, ",") {
			if strings.TrimSpace(f) != "" {
				parts++
			}
		}
		if parts != len(database.ParsePicks(q.Picks)) {
			return fmt.Errorf("quest %d: picks %q are \"step:option, ...\"", q.Quest, q.Picks)
		}
	}
	return nil
}

// questsDone marks each quest done and gives its spells.
func (m *migration) questsDone(qs []QuestDone) error {
	if len(qs) == 0 {
		return nil
	}
	have := map[int]bool{}
	rows, err := m.tx.Query(`SELECT spellid FROM spells WHERE serverid = ?`, m.ch.serverID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		have[id] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	var next int
	if err := m.tx.QueryRow(`SELECT COALESCE(MAX(addedorder) + 1, 0) FROM spells WHERE serverid = ? AND addedorder < 252`,
		m.ch.serverID).Scan(&next); err != nil {
		return err
	}
	var given []string
	for _, q := range qs {
		var name string
		if err := m.tx.QueryRow(`SELECT name FROM quests WHERE questid = ?`, q.Quest).Scan(&name); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("quest %d: no such quest", q.Quest)
			}
			return err
		}
		picks := database.ParsePicks(q.Picks)
		spells, picked, err := m.questSpells(q.Quest, picks)
		if err != nil {
			return err
		}
		var before any
		var b struct {
			step, count, done, slot int
			updated, picks          string
		}
		err = m.tx.QueryRow(`SELECT step, count, done, slot, updated, picks FROM charquests WHERE serverid = ? AND questid = ?`,
			m.ch.serverID, q.Quest).Scan(&b.step, &b.count, &b.done, &b.slot, &b.updated, &b.picks)
		switch {
		case errors.Is(err, sql.ErrNoRows):
		case err != nil:
			return err
		default:
			before = map[string]any{"step": b.step, "count": b.count, "done": b.done, "slot": b.slot, "updated": b.updated, "picks": b.picks}
		}
		done, keep := 1, database.FormatPicks(picks)
		was := "not taken"
		if before != nil {
			done = max(b.done, 1)
			if keep == "" {
				keep = b.picks
			}
			if b.step == 0 {
				was = fmt.Sprintf("done %d times", b.done)
			} else {
				was = fmt.Sprintf("at step %d", b.step)
			}
		}
		after := map[string]any{"step": 0, "count": 0, "done": done, "slot": b.slot, "updated": m.stamp, "picks": keep}
		if m.apply {
			if _, err := m.tx.Exec(`INSERT INTO charquests (serverid, questid, step, count, done, slot, updated, picks)
				VALUES (?, ?, 0, 0, ?, ?, ?, ?)
				ON CONFLICT(serverid, questid) DO UPDATE SET step = 0, count = 0, done = excluded.done, slot = excluded.slot,
					updated = excluded.updated, picks = excluded.picks`,
				m.ch.serverID, q.Quest, done, b.slot, m.stamp, keep); err != nil {
				return err
			}
		}
		what := ""
		if len(picked) > 0 {
			what = ", picking " + strings.Join(picked, " and ")
		}
		m.say("Quest %d %q done%s (was %s)", q.Quest, name, what, was)
		if err := m.note("quest", "charquests", int64(q.Quest), before, after, name); err != nil {
			return err
		}
		for _, id := range spells {
			if have[id] {
				continue
			}
			var sname string
			if err := m.tx.QueryRow(`SELECT spellname FROM spellpattern WHERE spellid = ?`, id).Scan(&sname); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					m.say("  quest %d's spell %d: no such spell, left out", q.Quest, id)
					continue
				}
				return err
			}
			if next >= 252 {
				m.say("  the book is full: %q left out", sname)
				continue
			}
			have[id] = true
			var rowID int64 = -1
			if m.apply {
				res, err := m.tx.Exec(`INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide)
					VALUES (?, ?, ?, 0, -1, 0, -1)`, m.ch.serverID, id, next)
				if err != nil {
					return err
				}
				if rowID, err = res.LastInsertId(); err != nil {
					return err
				}
			}
			if err := m.note("spell", "spells", rowID, nil, map[string]any{"spellid": id, "addedorder": next}, sname); err != nil {
				return err
			}
			given = append(given, sname)
			next++
		}
	}
	if len(given) > 0 {
		m.say("Spells of the quests into the book: %s", strings.Join(given, ", "))
	}
	return nil
}

// questSpells is the spells a quest's steps give the character's class, the
// picked options' among them, and the picks as words; a pick of a step with
// no such option refuses the plan.
func (m *migration) questSpells(quest int, picks map[int]int) ([]int, []string, error) {
	rows, err := m.tx.Query(`SELECT step, spells, byclass, choices FROM queststeps WHERE questid = ? ORDER BY step`, quest)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var spells []int
	var picked []string
	found := map[int]bool{}
	for rows.Next() {
		var step int
		var own, byClass, choices string
		if err := rows.Scan(&step, &own, &byClass, &choices); err != nil {
			return nil, nil, err
		}
		spells = append(spells, database.ParseIntList(own)...)
		if bc, err := database.ParseClassRewards(byClass); err == nil {
			spells = append(spells, bc[m.ch.class].Spells...)
		}
		opt, ok := picks[step]
		if !ok {
			continue
		}
		ch, err := database.ParseStepChoices(choices)
		if err != nil || opt >= len(ch.Options) || ch.Options[opt].Stay {
			return nil, nil, fmt.Errorf("quest %d: step %d has no option %d to pick", quest, step, opt)
		}
		found[step] = true
		spells = append(spells, ch.Options[opt].Reward.Spells...)
		picked = append(picked, fmt.Sprintf("%q", ch.Options[opt].Answer))
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	for step := range picks {
		if !found[step] {
			return nil, nil, fmt.Errorf("quest %d has no step %d with a choice", quest, step)
		}
	}
	return spells, picked, nil
}

// makeVampire makes the character a vampire, unless it is one.
func (m *migration) makeVampire() error {
	var n int
	if err := m.tx.QueryRow(`SELECT COUNT(*) FROM charvampire WHERE serverid = ?`, m.ch.serverID).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		m.say("Vampire: one already")
		return nil
	}
	note := "brought over by run " + m.run
	if m.apply {
		if _, err := m.tx.Exec(`INSERT INTO charvampire (serverid, since, note, source) VALUES (?, ?, ?, 'transfer')`,
			m.ch.serverID, m.stamp, note); err != nil {
			return err
		}
	}
	m.say("Vampire: made one (the sun burns it by day)")
	return m.note("vampire", "charvampire", int64(m.ch.serverID), nil, map[string]any{"since": m.stamp, "note": note, "source": "transfer"},
		"a vampire")
}

// undoQuest puts a quest's row back as it was before the run (none: deleted),
// unless it changed since.
func undoQuest(tx *sql.Tx, r logRow, before, after map[string]any, changed func(string) bool) (bool, error) {
	var step, done int
	var picks string
	err := tx.QueryRow(`SELECT step, done, picks FROM charquests WHERE serverid = ? AND questid = ?`, r.serverID, r.rowID).
		Scan(&step, &done, &picks)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if changed("its row deleted") {
			return false, nil
		}
	case err != nil:
		return false, err
	default:
		if (fmt.Sprint(after["step"]) != fmt.Sprint(step) || fmt.Sprint(after["done"]) != fmt.Sprint(done) ||
			fmt.Sprint(after["picks"]) != picks) && changed(fmt.Sprintf("step %d, done %d, picks %q", step, done, picks)) {
			return false, nil
		}
	}
	if before == nil {
		_, err := tx.Exec(`DELETE FROM charquests WHERE serverid = ? AND questid = ?`, r.serverID, r.rowID)
		return err == nil, err
	}
	_, err = tx.Exec(`INSERT INTO charquests (serverid, questid, step, count, done, slot, updated, picks) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(serverid, questid) DO UPDATE SET step = excluded.step, count = excluded.count, done = excluded.done,
			slot = excluded.slot, updated = excluded.updated, picks = excluded.picks`,
		r.serverID, r.rowID, before["step"], before["count"], before["done"], before["slot"], before["updated"], before["picks"])
	return err == nil, err
}

// undoVampire takes back a vampire the run made.
func undoVampire(tx *sql.Tx, r logRow) (bool, error) {
	_, err := tx.Exec(`DELETE FROM charvampire WHERE serverid = ? AND source = 'transfer'`, r.serverID)
	return err == nil, err
}
