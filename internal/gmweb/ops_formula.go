package gmweb

// The damage formula's mode on each shard, its switch, and the shadow log's
// summary (internal/formulamode, entity/dmgshadow.go; the owner, 2026-11-14).
// None shows a formula's text: the modes, the counts of formulas loaded, and
// figures of the shadow log alone.

import (
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

func (s *Server) registerFormulaOps() {
	s.addOp(&Op{
		Name: "formula_modes", Title: "Damage formula mode of each shard", Scope: scopeRead,
		Description: "Each shard's damage formula mode: default (the built-in damage), jibs (live: the operator's formula " +
			"file applied) or shadow (the built-in applied, the formula's figure recorded beside it in a private file), " +
			"who set it and when, and how many formulas its server has loaded. Never a formula's text.",
		Handler: func(c *opCtx) (any, error) {
			if c.s.d.Formula == nil {
				return nil, badRequest("this server has no damage formula switch")
			}
			return map[string]any{"shards": c.s.d.Formula.Status()}, nil
		},
	})
	s.addOp(&Op{
		Name: "set_formula_mode", Title: "Switch a shard's damage formula mode", Scope: scopeAdmin,
		Description: "Switch a shard (this one or another this server reaches: combat.formula_peers) to mode " +
			"\"default\" (the built-in damage), \"jibs\" (live: the operator's formula file applied) or \"shadow\" " +
			"(the built-in applied, the formula recorded beside it). At once here, within seconds on another shard; " +
			"kept over a restart; audited and told to the GMs in the game, as /formula is. A shard with no formula " +
			"loaded refuses jibs and shadow. An admin only.",
		Input: obj(map[string]any{"shard": str("the shard's name (formula_modes lists them); empty: this one"),
			"mode": str("default, jibs or shadow")}, "mode"),
		Handler: func(c *opCtx) (any, error) {
			if c.s.d.Formula == nil {
				return nil, badRequest("this server has no damage formula switch")
			}
			var req struct{ Shard, Mode string }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			var m entity.FormulaMode
			switch strings.ToLower(strings.TrimSpace(req.Mode)) {
			case "jibs", "live":
				m = entity.FormulaLive
			case "default", "off", "builtin":
				m = entity.FormulaOff
			case "shadow":
				m = entity.FormulaShadow
			default:
				return nil, badRequest("mode is default, jibs or shadow")
			}
			line, err := c.s.d.Formula.Set(req.Shard, m, database.GMEdit{AccountID: c.a.AccountID, Username: c.a.Username, Via: c.a.Via, IP: c.a.IP})
			if err != nil {
				return nil, badRequest("%v", err)
			}
			return map[string]any{"message": line, "shards": c.s.d.Formula.Status()}, nil
		},
	})
	s.addOp(&Op{
		Name: "formula_shadow_summary", Title: "Shadow mode: the formula against the built-in damage", Scope: scopeAdmin,
		Description: "Summarise this shard's shadow log (combat.formula_shadow_log): how many hits, how many the formula " +
			"modelled, and the mean, median and 90th percentile of the built-in and the formula's figures with their ratio, " +
			"overall, by path, and by path / attacker kind / attacker level band; the largest differences. Figures only, " +
			"never a formula's text. An admin only.",
		Input: obj(map[string]any{"path": str("one path only (npc_melee, player_melee, ...); empty: all"),
			"hours": integer("only the last this many hours; 0: all"),
			"top":   integer("how many of the largest differences (default 10, at most 100)")}),
		Handler: func(c *opCtx) (any, error) {
			if c.s.d.World == nil {
				return nil, badRequest("no world is running")
			}
			path, written, dropped := c.s.d.World.DamageFormulaShadowLog()
			if path == "" {
				return nil, badRequest("no shadow log is configured here (combat.formula_shadow_log)")
			}
			if err := c.s.d.World.FlushDamageFormulaShadow(); err != nil {
				return nil, badRequest("%v", err)
			}
			var req struct {
				Path  string
				Hours int
				Top   int
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			o := entity.ShadowSummaryOptions{Path: strings.TrimSpace(req.Path), Top: min(max(req.Top, 0), 100)}
			if req.Hours > 0 {
				o.Since = time.Now().Add(-time.Duration(req.Hours) * time.Hour)
			}
			sum, err := entity.SummarizeShadowLog(path, o)
			if err != nil {
				return nil, badRequest("%v", err)
			}
			return map[string]any{"mode": c.s.d.World.DamageFormulaMode().String(), "written_since_start": written,
				"dropped_since_start": dropped, "summary": sum}, nil
		},
	})
}
