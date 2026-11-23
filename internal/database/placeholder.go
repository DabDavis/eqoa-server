package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// A GM's or a sub GM's /ph (handlers/placeholder.go; a GM, Heirloom petition
// 2741: "/placeholder which means they can bind multiple mobs to a spawn
// point ... set the percentage of 100 odds that each one pops"): an npcs row's
// spawn point bound to pop one of up to MaxPlaceholderKinds kinds, each at its
// odds, each time it comes back from a death. It is the row's campspawns row
// (gmcamps.go): camp 0 for a row of no camp, a camp's spawn's own row keeping
// its camp; kinds the names, weights their odds (percent, adding to 100; ""
// for even odds). Every write has its gm_audit line in its transaction, as the
// GM panel's own, so the panel lists and reverts it.

// MaxPlaceholderKinds is the most kinds a /ph binds to one spawn point.
const MaxPlaceholderKinds = 5

// CampKindWeights is a weights column's numbers, one a kind of n: nil (even
// odds) when it is empty, or is not n whole numbers above 0 (a row edited by
// hand that no longer fits its kinds).
func CampKindWeights(s string, n int) []int {
	if strings.TrimSpace(s) == "" || n == 0 {
		return nil
	}
	parts := strings.Split(s, ",")
	if len(parts) != n {
		return nil
	}
	out := make([]int, n)
	for i, p := range parts {
		v, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || v <= 0 {
			return nil
		}
		out[i] = v
	}
	return out
}

// CampWeightsText is weights as a weights column keeps them ("" for none).
func CampWeightsText(weights []int) string {
	parts := make([]string, len(weights))
	for i, v := range weights {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, ", ")
}

// CampSpawnOf is the campspawns row of an npcs row; false when it has none.
func (db *SQLiteDB) CampSpawnOf(npcID int) (CampSpawnRow, bool, error) {
	r := CampSpawnRow{NPCID: npcID}
	var kinds, weights string
	err := db.QueryRow(`SELECT camp, kinds, weights FROM campspawns WHERE npcid = ?`, npcID).Scan(&r.Camp, &kinds, &weights)
	if errors.Is(err, sql.ErrNoRows) {
		return r, false, nil
	}
	if err != nil {
		return r, false, err
	}
	r.Kinds = CampKindNames(kinds)
	r.Weights = CampKindWeights(weights, len(r.Kinds))
	return r, true, nil
}

// SetPlaceholderAudited binds an npcs row's spawn point to kinds at weights
// (nil: even odds): its campspawns row made (camp 0) or its kinds and weights
// changed (a camp's spawn keeps its camp), with its audit line. It reports the
// row's change: insert, update or unchanged.
func (db *SQLiteDB) SetPlaceholderAudited(npcID int, kinds []string, weights []int, who GMEdit, detail string) (string, error) {
	if len(kinds) == 0 || len(kinds) > MaxPlaceholderKinds {
		return "", fmt.Errorf("a placeholder pops 1 to %d kinds, not %d", MaxPlaceholderKinds, len(kinds))
	}
	if weights != nil && len(weights) != len(kinds) {
		return "", fmt.Errorf("%d weights for %d kinds", len(weights), len(kinds))
	}
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return "", err
	}
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	const sel = `SELECT * FROM campspawns WHERE npcid = ?`
	before, err := fullRow(tx, sel, npcID)
	if err != nil {
		return "", err
	}
	k, wt := CampKindsText(kinds), CampWeightsText(weights)
	action := "insert"
	if before == nil {
		_, err = tx.Exec(`INSERT INTO campspawns (npcid, camp, kinds, weights) VALUES (?, 0, ?, ?)`, npcID, k, wt)
	} else {
		action = "update"
		_, err = tx.Exec(`UPDATE campspawns SET kinds = ?, weights = ? WHERE npcid = ?`, k, wt, npcID)
	}
	if err != nil {
		return "", err
	}
	after, err := fullRow(tx, sel, npcID)
	if err != nil {
		return "", err
	}
	if before != nil && fmt.Sprint(before) == fmt.Sprint(after) {
		return "unchanged", nil
	}
	if _, err := auditRowChange(tx, who, action, "campspawns", map[string]any{"npcid": int64(npcID)}, before, after, detail); err != nil {
		return "", err
	}
	return action, tx.Commit()
}

// ClearPlaceholderAudited takes a /ph's binding off an npcs row, audited: a
// placeholder's campspawns row (camp 0) deleted, a camp's spawn's kinds set to
// its own name alone (own), even odds, its camp kept. It reports "delete",
// "update", or "" when the row had no binding to take.
func (db *SQLiteDB) ClearPlaceholderAudited(npcID int, own string, who GMEdit, detail string) (string, error) {
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return "", err
	}
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	var camp int64
	err = tx.QueryRow(`SELECT camp FROM campspawns WHERE npcid = ?`, npcID).Scan(&camp)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if camp == 0 {
		if _, err := auditedDelete(tx, who, "campspawns", "npcid", int64(npcID), detail); err != nil {
			return "", err
		}
		return "delete", tx.Commit()
	}
	const sel = `SELECT * FROM campspawns WHERE npcid = ?`
	before, err := fullRow(tx, sel, npcID)
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(`UPDATE campspawns SET kinds = ?, weights = '' WHERE npcid = ?`, own, npcID); err != nil {
		return "", err
	}
	after, err := fullRow(tx, sel, npcID)
	if err != nil {
		return "", err
	}
	if fmt.Sprint(before) == fmt.Sprint(after) {
		return "", nil
	}
	if _, err := auditRowChange(tx, who, "update", "campspawns", map[string]any{"npcid": int64(npcID)}, before, after, detail); err != nil {
		return "", err
	}
	return "update", tx.Commit()
}
