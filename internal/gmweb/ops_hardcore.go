package gmweb

import (
	"fmt"
	"strings"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/fallen"
)

// [HC] Hardcore characters (handlers/hardcore.go; the owner, 2026-11-18): the
// shard's fallen, with everything their fall kept (the Fallen heroes page), and
// a GM's unfall for a mistake (a bug's or a lag's death): the character's
// fallen_at cleared, an audited row change revert undoes, its saved vitals let
// go so it enters whole where it fell. Nothing else brings one back.

func (s *Server) registerHardcoreOps() {
	s.addOp(&Op{
		Name: "fallen_list", Title: "Fallen Hardcore heroes", Scope: scopeRead,
		Description: "This shard's fallen Hardcore characters, the newest fall first: name, serverid, account, level, " +
			"class, race, Combat Mastery points, when (UTC), where, the cause and the killer, time played and time " +
			"from creation to the fall, as the fall kept them. limit (default 200, at most 2000).",
		Input: obj(map[string]any{"limit": integer("how many (default 200)")}),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ Limit int }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			if req.Limit <= 0 {
				req.Limit = 200
			}
			falls, err := c.s.d.DB.FallenList(min(req.Limit, 2000))
			if err != nil {
				return nil, err
			}
			rows := make([]map[string]any, 0, len(falls))
			for _, f := range falls {
				h := fallen.HeroOf(f, "")
				rows = append(rows, map[string]any{"serverid": f.ServerID, "name": f.Name, "accountid": f.AccountID,
					"level": f.Level, "class": h.Class, "race": h.Race, "cms": f.CMs,
					"fell": time.Unix(f.At, 0).UTC().Format("2006-01-02 15:04:05"), "place": f.Place, "cause": f.Cause,
					"killer": f.Killer, "world": f.World, "x": f.X, "y": f.Y, "z": f.Z,
					"played": fallen.Duration(f.Played), "lived": fallen.Duration(h.LivedDays)})
			}
			return map[string]any{"fallen": rows, "count": len(rows)}, nil
		},
	})
	s.addOp(&Op{
		Name: "hardcore_living", Title: "Living Hardcore leaderboard", Scope: scopeRead,
		Description: "This shard's living Hardcore characters ranked as the Hardcore Leaderboard in Freeport ranks them: " +
			"by level, then Combat Mastery points, then experience, then the longest alive. limit (default 100).",
		Input: obj(map[string]any{"limit": integer("how many (default 100)")}),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ Limit int }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			if req.Limit <= 0 {
				req.Limit = 100
			}
			heroes, err := c.s.d.DB.LivingHardcore(min(req.Limit, 2000))
			if err != nil {
				return nil, err
			}
			now := time.Now().Unix()
			rows := make([]map[string]any, 0, len(heroes))
			for i, h := range heroes {
				alive := int64(0)
				if h.Born > 0 {
					alive = now - h.Born
				}
				rows = append(rows, map[string]any{"rank": i + 1, "serverid": h.ServerID, "name": h.Name, "level": h.Level,
					"class": entity.ClassName(h.Class), "race": entity.RaceName(h.Race), "cms": h.CMs, "xp": h.TotalXP,
					"alive": fallen.Duration(alive), "played": fallen.Duration(h.Played)})
			}
			return map[string]any{"living": rows, "count": len(rows)}, nil
		},
	})
	s.addOp(&Op{
		Name: "unfall_character", Title: "Unfall a Hardcore character", Scope: scopeWrite, Destructive: true,
		Description: "Bring a fallen Hardcore character back, for a death that was a mistake (a bug, a lag spike, a " +
			"GM's slip): its fall cleared (characters.fallen_at 0; the rest of the fall's record kept for the audit), " +
			"its saved hit points let go so it enters whole where it fell, still Hardcore. Its player can select it " +
			"again at once. The reason is required and kept in the audit log; revert undoes the row change.",
		Input: obj(map[string]any{
			"character": idOrName("the character, by name or serverid"),
			"reason":    str("why (kept in the audit log): the bug, the lag, the mistake"),
		}, "character", "reason"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Character any
				Reason    string
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			if strings.TrimSpace(req.Reason) == "" {
				return nil, badRequest("say why: the reason is kept in the audit log")
			}
			id, name, err := c.s.characterRef(req.Character)
			if err != nil {
				return nil, err
			}
			st, err := c.s.d.DB.HardcoreOf(uint32(id))
			if err != nil {
				return nil, err
			}
			if !st.Fallen() {
				return nil, badRequest("%s has not fallen", name)
			}
			t, err := c.s.table("characters")
			if err != nil {
				return nil, err
			}
			ch, err := c.s.updateRow(c.a, t, map[string]any{"serverid": id}, map[string]any{"fallen_at": int64(0)}, nil, writeOpts{Raw: true})
			if err != nil {
				return nil, err
			}
			detail := fmt.Sprintf("unfall %s (fell %s, %s, %s): %s", name, time.Unix(st.Fall.At, 0).UTC().Format("2006-01-02 15:04"),
				st.Fall.Place, st.Fall.Killer, strings.TrimSpace(req.Reason))
			if ch.Audit != 0 {
				c.s.d.DB.Exec(`UPDATE gm_audit SET detail = ? WHERE id = ?`, detail, ch.Audit)
			}
			c.s.d.DB.Exec(`DELETE FROM charvitals WHERE serverid = ?`, id)
			return map[string]any{"character": name, "unfallen": true, "change": ch}, nil
		},
	})
}
