package gmweb

import (
	"fmt"

	"github.com/eqoa/server/internal/database"
)

// The game's balance (entity/balance.go): tables balancelevel and balancecon,
// read again whole after a change, every NPC's hit points made those of its
// level at once (each keeping its share of them).

// liveBalance reads both tables into the world (the change is committed: the
// database is free).
func (s *Server) liveBalance() string {
	if s.d.World == nil || s.d.DB == nil {
		return "no world is running: at the next start"
	}
	levels, cons, err := s.d.World.LoadBalance(s.d.DB)
	if err != nil {
		return "not applied to the world: " + err.Error()
	}
	return fmt.Sprintf("at once (%d level rows, %d con rows): every NPC's hit points are its level's now, "+
		"each keeping its share of them, and the next hit of every fight uses it, the next combat art its cooldown; "+
		"a summoned pet's at its next summoning", levels, cons)
}

// checkBalanceRow holds a balancelevel or balancecon row to its bounds
// (database.BalanceProblems).
func checkBalanceRow(table string, row map[string]any) error {
	key := "level"
	cols := []string{"hp_pct", "damage_pct", "pet_hp_pct", "pet_damage_pct"}
	if table == "balancelevel" {
		if v, present := row["con_pct"]; present && v != nil {
			p, ok := asFloat(v)
			if !ok {
				return badRequest("%s: con_pct is a number", table)
			}
			if why := database.BalanceConPctProblem(p); why != "" {
				return badRequest("%s: %s", table, why)
			}
		}
		if v, present := row["art_cooldown_ms"]; present && v != nil {
			ms, ok := asFloat(v)
			if !ok {
				return badRequest("%s: art_cooldown_ms is a number", table)
			}
			if why := database.BalanceMSProblem("art_cooldown_ms", ms); why != "" {
				return badRequest("%s: %s", table, why)
			}
		}
	}
	if table == "balancecon" {
		key, cols = "diff", []string{"damage_pct", "taken_pct"}
	}
	k, ok := asFloat(row[key])
	if !ok {
		return badRequest("%s: %s is a whole number", table, key)
	}
	var pcts []float64
	for _, c := range cols {
		if v, present := row[c]; present && v != nil {
			p, ok := asFloat(v)
			if !ok {
				return badRequest("%s: %s is a number", table, c)
			}
			pcts = append(pcts, p)
		}
	}
	if why := database.BalanceProblems(table, int(k), pcts...); why != "" {
		return badRequest("%s: %s", table, why)
	}
	return nil
}
