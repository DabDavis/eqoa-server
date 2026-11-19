package gmweb

import (
	"fmt"
	"strings"
)

// [OG] characters (database/ogchar.go; the owner, 2026-11-19: "lets make [OG]
// status a char tag like [HC] the account status is confusing new players"): a
// player chooses it once, while the character is new; a GM changes it here,
// one character at a time, audited and revertible. It replaces set_account_og
// (the account's flag, which nothing reads for play any more). Each shard's
// characters are its own, so this is never a shared-accounts refusal.

func (s *Server) registerOGCharOps() {
	s.addOp(&Op{
		Name: "set_character_og", Title: "Make a character [OG] or not", Scope: scopeWrite, Destructive: true,
		Description: "Set whether a character is an [OG] one (characters.og): on a shard with world.og_accounts (the " +
			"Vanilla Test server) its sessions have the game's own UI (no quality-of-life additions, no map, minimap " +
			"or item links, no mouse or touch in vanilla) and its name is drawn \"[OG] Name\". A player chooses it " +
			"once, while the character is new, and cannot change it: this is the GM's way. Its choice is marked made " +
			"(og_chosen), so the game does not ask again. The reason is required and kept in the audit log; revert " +
			"undoes the row change. It counts at the character's next world entry.",
		Input: obj(map[string]any{
			"character": idOrName("the character, by name or serverid"),
			"og":        boolean("true for an [OG] character, false for a plain one"),
			"reason":    str("why (kept in the audit log)"),
		}, "character", "og", "reason"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Character any
				OG        bool
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
			t, err := c.s.table("characters")
			if err != nil {
				return nil, err
			}
			v := int64(0)
			if req.OG {
				v = 1
			}
			ch, err := c.s.updateRow(c.a, t, map[string]any{"serverid": id}, map[string]any{"og": v, "og_chosen": int64(1)}, nil, writeOpts{Raw: true})
			if err != nil {
				return nil, err
			}
			if ch.Audit != 0 {
				c.s.d.DB.Exec(`UPDATE gm_audit SET detail = ? WHERE id = ?`,
					fmt.Sprintf("%s [OG] %v: %s", name, req.OG, strings.TrimSpace(req.Reason)), ch.Audit)
			}
			return map[string]any{"character": name, "og": req.OG, "change": ch}, nil
		},
	})
}
