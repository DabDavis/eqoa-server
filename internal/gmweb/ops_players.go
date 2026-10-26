package gmweb

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/eqoa/server/internal/auth"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/handlers"
)

// Characters and accounts: what a GM does to a player's character (level,
// purse, items, spells) and to an account (make one, its password, a ban, its
// GM level), live where the character is in the world.

func (s *Server) registerPlayerOps() {
	s.addOp(&Op{
		Name: "set_character", Title: "Set a character's level, money or health", Scope: scopeWrite, Core: true,
		Description: "Set a character's level (1-60), tunar and bank tunar, saved and, for one in the world, applied " +
			"where it stands at once (its client is told). heal brings an online character's health and power to " +
			"full. Leave out what should not change.",
		Input: obj(map[string]any{
			"character":  idOrName("the character, by name or serverid"),
			"level":      integer("1 to 60"),
			"tunar":      integer("money carried"),
			"bank_tunar": integer("money in the bank"),
			"heal":       boolean("an online character: health and power to full"),
		}, "character"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Character any
				Level     *int
				Tunar     *int64
				BankTunar *int64 `json:"bank_tunar"`
				Heal      bool
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			id, name, err := c.s.characterRef(req.Character)
			if err != nil {
				return nil, err
			}
			vals := map[string]any{}
			if req.Level != nil {
				if *req.Level < 1 || *req.Level > 60 {
					return nil, badRequest("level is 1 to 60")
				}
				vals["level"] = int64(*req.Level)
			}
			if req.Tunar != nil {
				if *req.Tunar < 0 {
					return nil, badRequest("tunar cannot be negative")
				}
				vals["tunar"] = *req.Tunar
			}
			if req.BankTunar != nil {
				if *req.BankTunar < 0 {
					return nil, badRequest("bank_tunar cannot be negative")
				}
				vals["bankTunar"] = *req.BankTunar
			}
			out := map[string]any{"character": name}
			if len(vals) > 0 {
				t, err := c.s.table("characters")
				if err != nil {
					return nil, err
				}
				ch, err := c.s.updateRow(c.a, t, map[string]any{"serverid": id}, vals, nil, writeOpts{})
				if err != nil {
					return nil, err
				}
				out["change"] = ch
			}
			if req.Heal {
				if c.s.d.World != nil && c.s.d.World.HealPlayer(uint32(id)) {
					out["healed"] = true
					c.s.audit(c.a, "live", "", nil, nil, nil, fmt.Sprintf("healed %s", name))
				} else {
					out["healed"] = false
					out["healNote"] = name + " is not in the world, or is dead"
				}
			}
			return out, nil
		},
	})
	s.addOp(&Op{
		Name: "give_item", Title: "Give an item", Scope: scopeWrite, Core: true,
		Description: "Give a character items, new and at full condition: one in the world gets them in its " +
			"inventory at once (stacking as a purchase does), one offline finds them at its next login. location " +
			"bank puts them in the bank instead (seen at the next login). Name the item by its patternid or its " +
			"exact name (search finds names).",
		Input: obj(map[string]any{
			"character": idOrName("the character, by name or serverid"),
			"item":      idOrName("the item: its patternid or exact name"),
			"count":     integer("how many (default 1, at most 1000)"),
			"location":  strEnum("where (default inventory)", "inventory", "bank"),
		}, "character", "item"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Character any
				Item      any
				Count     int
				Location  string
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.giveItem(c.a, req.Character, req.Item, req.Count, req.Location)
		},
	})
	s.addOp(&Op{
		Name: "grant_spell", Title: "Give a spell", Scope: scopeWrite, Core: true,
		Description: "Put a spell in a character's spell book (by spellid or exact name), or every spell of its " +
			"class with spell \"all\" (the classspells table; were forms and a were's abilities are no class's and " +
			"come only by their own id). One in the world gets it at once, as scribing a " +
			"scroll gives it; one offline at its next login. A spell it knows already is skipped.",
		Input: obj(map[string]any{
			"character": idOrName("the character, by name or serverid"),
			"spell":     idOrName("the spell: its spellid, its exact name, or \"all\" for its class's"),
		}, "character", "spell"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Character any
				Spell     any
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.grantSpell(c.a, req.Character, req.Spell)
		},
	})
}

// itemRef is an item by patternid or exact name.
func (s *Server) itemRef(v any) (database.ItemPatternRecord, error) {
	if n, ok := v.(json.Number); ok {
		v = string(n)
	}
	text := strings.TrimSpace(fmt.Sprint(v))
	if v == nil || text == "" {
		return database.ItemPatternRecord{}, badRequest("name an item (patternid or exact name)")
	}
	store := database.NewItemPatternStore(s.d.DB)
	if id, err := strconv.Atoi(text); err == nil {
		rec, err := store.GetItemPattern(id)
		if err != nil {
			return database.ItemPatternRecord{}, notFound("no item %d", id)
		}
		return *rec, nil
	}
	rows, err := s.rowsOf(`SELECT patternid, itemname FROM itempattern WHERE LOWER(itemname) = LOWER(?) ORDER BY patternid LIMIT 20`, text)
	if err != nil {
		return database.ItemPatternRecord{}, err
	}
	switch len(rows) {
	case 0:
		sug, _ := s.rowsOf(`SELECT patternid, itemname FROM itempattern WHERE itemname LIKE ? ESCAPE '\' ORDER BY itemname LIMIT 8`,
			"%"+likeEscape(text)+"%")
		return database.ItemPatternRecord{}, notFound("no item named %q%s", text, suggest(sug, "patternid", "itemname"))
	case 1:
		id, _ := asFloat(rows[0]["patternid"])
		rec, err := store.GetItemPattern(int(id))
		if err != nil {
			return database.ItemPatternRecord{}, err
		}
		return *rec, nil
	}
	return database.ItemPatternRecord{}, badRequest("%d items are named %q; give the patternid%s", len(rows), text,
		suggest(rows, "patternid", "itemname"))
}

// suggest lists rows as " (did you mean: name #id, ...)".
func suggest(rows []map[string]any, idCol, nameCol string) string {
	if len(rows) == 0 {
		return ""
	}
	var parts []string
	for _, r := range rows {
		parts = append(parts, fmt.Sprintf("%v #%v", r[nameCol], r[idCol]))
	}
	return " (did you mean: " + strings.Join(parts, ", ") + ")"
}

func (s *Server) giveItem(a *actor, who, what any, count int, location string) (any, error) {
	id, name, err := s.characterRef(who)
	if err != nil {
		return nil, err
	}
	if err := s.guardCharacter(s.d.DB, a, id); err != nil {
		return nil, err
	}
	item, err := s.itemRef(what)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		count = 1
	}
	if count < 1 || count > 1000 {
		return nil, badRequest("count is 1 to 1000")
	}
	loc := 0
	switch location {
	case "", "inventory":
	case "bank":
		loc = 1
	default:
		return nil, badRequest("location is inventory or bank")
	}
	t, err := s.table("charinventory")
	if err != nil {
		return nil, err
	}

	if sess := s.sessionFor(uint32(id)); sess != nil && loc == 0 {
		// The live path writes the rows itself (as a sale does); the log gets
		// each row it made or changed.
		before, _ := s.rowsOf(`SELECT * FROM charinventory WHERE serverid = ? AND patternid = ?`, id, item.PatternID)
		given, err := handlers.GiveItemNow(sess, s.d.World, database.NewInventoryStore(s.d.DB), item, count)
		if err != nil && given == 0 {
			return nil, err
		}
		after, _ := s.rowsOf(`SELECT * FROM charinventory WHERE serverid = ? AND patternid = ?`, id, item.PatternID)
		prev := map[string]map[string]any{}
		for _, r := range before {
			prev[fmt.Sprint(r["itemid"])] = r
		}
		for _, r := range after {
			k := fmt.Sprint(r["itemid"])
			key := map[string]any{"itemid": r["itemid"]}
			if b, ok := prev[k]; !ok {
				s.auditTx(s.d.DB, a, "insert", t.Name, key, nil, r, fmt.Sprintf("gave %s %s (live)", name, item.ItemName))
			} else if len(movedColumns(b, r)) > 0 {
				s.auditTx(s.d.DB, a, "update", t.Name, key, b, r, fmt.Sprintf("gave %s %s (live)", name, item.ItemName))
			}
		}
		msg := fmt.Sprintf("%s got %d × %s in their inventory now", name, given, item.ItemName)
		if given < count {
			msg += fmt.Sprintf(" (%d did not fit: the stack or the list is full)", count-given)
		}
		log.Printf("GM %s (%s): %s", a.Username, a.Via, msg)
		return map[string]any{"given": given, "online": true, "message": msg}, nil
	}

	// offline, or the bank: new rows at the end of the list
	stacks := []int{count}
	if item.MaxStack < 2 {
		stacks = make([]int, count)
		for i := range stacks {
			stacks[i] = 1
		}
	} else if count > item.MaxStack {
		stacks = nil
		for left := count; left > 0; left -= item.MaxStack {
			stacks = append(stacks, min(left, item.MaxStack))
		}
	}
	var changes []*change
	for _, n := range stacks {
		var next int64
		s.d.DB.QueryRow(`SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = ?`, id, loc).Scan(&next)
		ch, err := s.insertRow(a, t, map[string]any{
			"serverid": id, "stackleft": int64(n), "remainHP": int64(item.MaxHP), "remaincharge": int64(0),
			"patternid": int64(item.PatternID), "equiploc": int64(-1), "location": int64(loc), "listnumber": next,
		}, writeOpts{})
		if err != nil {
			return nil, err
		}
		changes = append(changes, ch)
	}
	where := "inventory"
	if loc == 1 {
		where = "bank"
	}
	msg := fmt.Sprintf("%s gets %d × %s in their %s", name, count, item.ItemName, where)
	if _, on := s.onlineCharacter(uint32(id)); on {
		msg += " at their next login (they are online; the bank is sent at login)"
	} else {
		msg += " at their next login"
	}
	return map[string]any{"given": count, "online": false, "message": msg, "rows": len(changes)}, nil
}

// spellRef is a spell by spellid or exact name.
func (s *Server) spellRef(v any) (int64, string, error) {
	if n, ok := v.(json.Number); ok {
		v = string(n)
	}
	text := strings.TrimSpace(fmt.Sprint(v))
	if id, err := strconv.ParseInt(text, 10, 64); err == nil {
		var name string
		if err := s.d.DB.QueryRow(`SELECT spellname FROM spellpattern WHERE spellid = ?`, id).Scan(&name); err != nil {
			return 0, "", notFound("no spell %d", id)
		}
		return id, name, nil
	}
	rows, err := s.rowsOf(`SELECT spellid, spellname FROM spellpattern WHERE LOWER(spellname) = LOWER(?) ORDER BY spellid LIMIT 20`, text)
	if err != nil {
		return 0, "", err
	}
	switch len(rows) {
	case 0:
		sug, _ := s.rowsOf(`SELECT spellid, spellname FROM spellpattern WHERE spellname LIKE ? ESCAPE '\' ORDER BY spellname LIMIT 8`,
			"%"+likeEscape(text)+"%")
		return 0, "", notFound("no spell named %q%s", text, suggest(sug, "spellid", "spellname"))
	case 1:
		id, _ := asFloat(rows[0]["spellid"])
		return int64(id), fmt.Sprint(rows[0]["spellname"]), nil
	}
	return 0, "", badRequest("%d spells are named %q; give the spellid%s", len(rows), text, suggest(rows, "spellid", "spellname"))
}

func (s *Server) grantSpell(a *actor, who, what any) (any, error) {
	id, name, err := s.characterRef(who)
	if err != nil {
		return nil, err
	}
	if err := s.guardCharacter(s.d.DB, a, id); err != nil {
		return nil, err
	}
	var class int
	if err := s.d.DB.QueryRow(`SELECT tclass FROM characters WHERE serverid = ?`, id).Scan(&class); err != nil {
		return nil, err
	}
	var ids []int64
	if strings.EqualFold(strings.TrimSpace(fmt.Sprint(what)), "all") {
		rows, err := s.rowsOf(`SELECT c.spellid, p.spellname, COALESCE(p.spelldesc, '') AS spelldesc,
			COALESCE(c.source, '') AS source FROM classspells c
			JOIN spellpattern p ON p.spellid = c.spellid
			WHERE c.class = ? ORDER BY p.abilitylvl, p.spellname, c.spellid`, class)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			// a were form is no class's: a were quest's and a mastery's
			// (database.IsWereAbility); granted by its id alone
			if database.IsWereAbility(fmt.Sprint(r["spellname"]), fmt.Sprint(r["spelldesc"])) {
				continue
			}
			// nor a mastery's, an epic's or a master class's (database.Granted):
			// no level gives those; granted by id alone
			if !database.Granted(fmt.Sprint(r["source"])) {
				continue
			}
			v, _ := asFloat(r["spellid"])
			ids = append(ids, int64(v))
		}
		if len(ids) == 0 {
			return nil, notFound("the classspells table has no spells for a %s", className(class))
		}
	} else {
		sid, _, err := s.spellRef(what)
		if err != nil {
			return nil, err
		}
		ids = []int64{sid}
	}
	t, err := s.table("spells")
	if err != nil {
		return nil, err
	}
	known := map[int64]bool{}
	next := int64(0)
	book, err := s.rowsOf(`SELECT spellid, addedorder FROM spells WHERE serverid = ?`, id)
	if err != nil {
		return nil, err
	}
	for _, r := range book {
		sid, _ := asFloat(r["spellid"])
		ord, _ := asFloat(r["addedorder"])
		known[int64(sid)] = true
		if ord < 252 && int64(ord) >= next {
			next = int64(ord) + 1
		}
	}
	sess := s.sessionFor(uint32(id))
	var added []string
	skipped := 0
	full := false
	for _, sid := range ids {
		if known[sid] {
			skipped++
			continue
		}
		if next >= 252 {
			full = true
			break
		}
		if _, err := s.insertRow(a, t, map[string]any{
			"serverid": id, "spellid": sid, "addedorder": next, "onHotBar": int64(0), "whereonBar": int64(-1),
			"unk1": int64(0), "showhide": int64(-1),
		}, writeOpts{}); err != nil {
			return nil, err
		}
		known[sid] = true
		next++
		var sname string
		s.d.DB.QueryRow(`SELECT spellname FROM spellpattern WHERE spellid = ?`, sid).Scan(&sname)
		added = append(added, sname)
		if sess != nil {
			s.sendSpellLive(uint32(id), sid)
		}
	}
	msg := fmt.Sprintf("%s learned %d spell(s)", name, len(added))
	if len(added) > 0 && len(added) <= 10 {
		msg += ": " + strings.Join(added, ", ")
	}
	if skipped > 0 {
		msg += fmt.Sprintf("; %d known already", skipped)
	}
	if full {
		msg += "; the book is full (252)"
	}
	if sess != nil {
		msg += " (in their book now)"
	} else {
		msg += " (at their next login)"
	}
	return map[string]any{"added": added, "skipped": skipped, "message": msg}, nil
}

// sendSpellLive tells an online client of a spell just put in its book, as a
// scribed scroll's is (0x00B3, announced).
func (s *Server) sendSpellLive(serverID uint32, spellID int64) {
	book, err := database.NewSpellbookStore(s.d.DB).GetCharacterSpells(serverID)
	if err != nil {
		return
	}
	for i := range book {
		if int64(book[i].Pattern.SpellID) == spellID {
			if ss := s.sessionFor(serverID); ss != nil {
				handlers.SendAbilityAdd(ss, 0, true, &book[i])
			}
			return
		}
	}
}

func (s *Server) registerAccountOps() {
	s.addOp(&Op{
		Name: "create_account", Title: "Make an account", Scope: scopeWrite,
		Description: "Make a game account with a password (hashed as the game's are). level above 0 makes a GM, " +
			"which only an admin may, and never above their own level.",
		Input: obj(map[string]any{
			"username": str("3 to 16 letters, digits or _"),
			"password": str("4 characters or more"),
			"level":    integer("acctlevel: 0 a player (default), 1+ a GM"),
		}, "username", "password"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Username string
				Password string
				Level    int
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			user := strings.TrimSpace(req.Username)
			if len(user) < 3 || len(user) > 16 {
				return nil, badRequest("a name is 3 to 16 characters")
			}
			for _, r := range user {
				if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_') {
					return nil, badRequest("a name is letters, digits and _")
				}
			}
			if len(req.Password) < 4 {
				return nil, badRequest("a password is 4 characters or more")
			}
			if req.Level != 0 {
				if !c.a.may(scopeAdmin) {
					return nil, forbidden("only an admin makes a GM account")
				}
				if req.Level < 0 || req.Level > c.a.Level {
					return nil, forbidden("level is 0 to your own (%d)", c.a.Level)
				}
			}
			if _, err := c.s.accountByName(user); err == nil {
				return nil, badRequest("the name %q is taken", user)
			}
			hash, err := auth.HashPassword(req.Password, c.s.d.Config.Auth.BCryptCost)
			if err != nil {
				return nil, err
			}
			t, err := c.s.table("accountinfo")
			if err != nil {
				return nil, err
			}
			tx, err := c.s.d.DB.Begin()
			if err != nil {
				return nil, err
			}
			defer tx.Rollback()
			res, err := tx.Exec(`INSERT INTO accountinfo (username, pwhash, acctstatus, acctlevel, acctcreation)
				VALUES (?, ?, 1, ?, CURRENT_TIMESTAMP)`, user, hash, req.Level)
			if err != nil {
				return nil, sqlError(err)
			}
			id, _ := res.LastInsertId()
			row, err := getRow(tx, t, map[string]any{"accountid": id})
			if err != nil {
				return nil, err
			}
			delete(row, "__key")
			aid, err := c.s.auditTx(tx, c.a, "insert", t.Name, map[string]any{"accountid": id}, nil, row,
				fmt.Sprintf("made account %s (level %d)", user, req.Level))
			if err != nil {
				return nil, err
			}
			if err := tx.Commit(); err != nil {
				return nil, err
			}
			log.Printf("GM %s (%s): made account %q (level %d)", c.a.Username, c.a.Via, user, req.Level)
			return map[string]any{"accountid": id, "username": user, "level": req.Level, "audit": aid}, nil
		},
	})
	s.addOp(&Op{
		Name: "set_account_password", Title: "Set an account's password", Scope: scopeWrite, Destructive: true,
		Description: "Give an account a new password (hashed as the game's are). Its sessions in this panel end and " +
			"its API tokens and connected agents are revoked. An account at or above your level is changed only by " +
			"itself or a higher one.",
		Input: obj(map[string]any{
			"account":  idOrName("the account, by name or accountid"),
			"password": str("4 characters or more"),
		}, "account", "password"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Account  any
				Password string
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			acc, err := c.s.accountRef(req.Account)
			if err != nil {
				return nil, err
			}
			if acc.ID != c.a.AccountID && acc.Level >= c.a.Level {
				return nil, forbidden("%s is at or above your level", acc.Username)
			}
			if len(req.Password) < 4 {
				return nil, badRequest("a password is 4 characters or more")
			}
			hash, err := auth.HashPassword(req.Password, c.s.d.Config.Auth.BCryptCost)
			if err != nil {
				return nil, err
			}
			if _, err := c.s.d.DB.Exec(`UPDATE accountinfo SET pwhash = ? WHERE accountid = ?`, hash, acc.ID); err != nil {
				return nil, err
			}
			keep := ""
			if c.a.sess != nil && acc.ID == c.a.AccountID {
				keep = c.a.sess.Token
			}
			ended := c.s.sessions.endAccount(acc.ID, keep)
			// a token is as good as the password it was made with
			revoked := c.s.revokeAccountTokens(acc.ID)
			c.s.audit(c.a, "account", "accountinfo", map[string]any{"accountid": acc.ID}, nil, nil,
				fmt.Sprintf("set the password of %s (%d token(s) revoked)", acc.Username, revoked))
			return map[string]any{"account": acc.Username, "panelSessionsEnded": ended, "tokensRevoked": revoked}, nil
		},
	})
	s.addOp(&Op{
		Name: "ban_account", Title: "Ban or unban an account", Scope: scopeWrite, Destructive: true, Core: true,
		Description: "Ban an account (its game sessions are ejected and its logins refused) or lift a ban. The " +
			"reason goes in the audit log. An account at or above your level cannot be banned by you.",
		Input: obj(map[string]any{
			"account": idOrName("the account, by name or accountid"),
			"banned":  boolean("true to ban, false to lift a ban"),
			"reason":  str("why (kept in the audit log)"),
		}, "account", "banned"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Account any
				Banned  bool
				Reason  string
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			acc, err := c.s.accountRef(req.Account)
			if err != nil {
				return nil, err
			}
			if acc.ID == c.a.AccountID {
				return nil, badRequest("you cannot ban yourself")
			}
			status := int64(1)
			if req.Banned {
				status = -1
			}
			t, err := c.s.table("accountinfo")
			if err != nil {
				return nil, err
			}
			ch, err := c.s.updateRow(c.a, t, map[string]any{"accountid": acc.ID}, map[string]any{"acctstatus": status}, nil, writeOpts{})
			if err != nil {
				return nil, err
			}
			if req.Reason != "" && ch.Audit != 0 {
				c.s.d.DB.Exec(`UPDATE gm_audit SET detail = ? WHERE id = ?`, "reason: "+req.Reason, ch.Audit)
			}
			return map[string]any{"account": acc.Username, "banned": req.Banned, "change": ch}, nil
		},
	})
	s.addOp(&Op{
		Name: "set_account_level", Title: "Set an account's GM level", Scope: scopeAdmin, Destructive: true,
		Description: "Set an account's acctlevel: 0 a player, gm level (1) a GM, admin level (2) an admin. An admin " +
			"only; never your own, never above your own level, never an account at or above it.",
		Input: obj(map[string]any{
			"account": idOrName("the account, by name or accountid"),
			"level":   integer("the new acctlevel"),
		}, "account", "level"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Account any
				Level   int
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			acc, err := c.s.accountRef(req.Account)
			if err != nil {
				return nil, err
			}
			switch {
			case acc.ID == c.a.AccountID:
				return nil, forbidden("not your own level")
			case acc.Level >= c.a.Level:
				return nil, forbidden("%s is at or above your level", acc.Username)
			case req.Level < 0 || req.Level > c.a.Level:
				return nil, forbidden("level is 0 to your own (%d)", c.a.Level)
			}
			t, err := c.s.table("accountinfo")
			if err != nil {
				return nil, err
			}
			// updateRow holds the change to the same rules (guardLevel), and a
			// GM no more has their sessions and tokens ended (liveAccount)
			ch, err := c.s.updateRow(c.a, t, map[string]any{"accountid": acc.ID}, map[string]any{"acctlevel": int64(req.Level)},
				nil, writeOpts{Raw: true})
			if err != nil {
				return nil, err
			}
			return map[string]any{"account": acc.Username, "level": req.Level, "change": ch}, nil
		},
	})
}
