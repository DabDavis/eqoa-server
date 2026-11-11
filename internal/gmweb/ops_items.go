package gmweb

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// ops_items.go — item_reading: what an item record does as the server and
// CLIENT read it, in words: the coded columns by name, the weapon's proc (no
// column holds one: entity/weaponproc.go reads it from the description, or
// the epics' by name), a worn item's answer to a hit (entity/wornshield.go),
// what using it does (table itemeffect). The item editor shows it beside the
// form and asks again as a GM types, with the form's values.

// readingCols are the columns item_reading takes in place of the record's.
var readingCols = []string{"itemname", "itemdesc", "attacktype", "unk5", "equipslot", "procanim", "trade", "rent", "lore",
	"craft", "unk3", "weapondamage"}

func (s *Server) registerItemOps() {
	s.addOp(&Op{
		Name: "item_reading", Title: "What an item does, as the server reads it", Scope: scopeRead,
		Description: "One item record (itempattern) read as the server and CLIENT read it: its attack type's name, reach and melee " +
			"kind; its item type; the tags its info shows (NO TRADE, NO RENT, LORE); its weapon proc in plain words (no column " +
			"holds a proc: the server reads it from the Description, e.g. \"Procs a 100 point DD Spell.\", \"Procs 2000 slashing " +
			"damage, a 10% chance each hit.\", or an epic's by its name), with the chance and what resists it; a worn item's " +
			"effect on each melee hit its wearer takes (by its name); and what using it does (its itemeffect row). values " +
			"stand in for the record's own (" + strings.Join(readingCols, ", ") + "), to see what an edit would read as before " +
			"saving it; with no item, values alone are read.",
		Input: obj(map[string]any{
			"item":   integer("the record (itempattern.patternid); 0 or none: values alone"),
			"values": map[string]any{"type": "object", "description": "columns standing in for the record's (" + strings.Join(readingCols, ", ") + ")"},
		}),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Item   int
				Values map[string]any
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			row := map[string]any{}
			if req.Item > 0 {
				r, err := c.s.rowOf("itempattern", "patternid", req.Item)
				if err != nil {
					return nil, err
				}
				row = r
			}
			for _, k := range readingCols {
				if v, ok := req.Values[k]; ok {
					row[k] = v
				}
			}
			out := itemReading(req.Item, row)
			if req.Item > 0 {
				if eff, _ := c.s.rowsOf(`SELECT patternid, name, line, target_line, charges, cooldown, wear, breaks, area FROM itemeffect WHERE patternid = ?`, req.Item); len(eff) == 1 {
					out["use"] = eff[0]
				}
			}
			return out, nil
		},
	})
}

// rowInt is a row's column as a whole number (0 when not one).
func rowInt(row map[string]any, k string) int {
	switch v := row[k].(type) {
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0
		}
		return n
	case bool:
		if v {
			return 1
		}
		return 0
	}
	f, _ := asFloat(row[k])
	return int(f)
}

func rowText(row map[string]any, k string) string {
	if v, ok := row[k].(string); ok {
		return v
	}
	if row[k] == nil {
		return ""
	}
	return fmt.Sprint(row[k])
}

// itemReading is item_reading's answer for a row (the record's columns, or the
// values standing in for them).
func itemReading(id int, row map[string]any) map[string]any {
	name, desc := rowText(row, "itemname"), rowText(row, "itemdesc")
	at, slot, typ := rowInt(row, "attacktype"), rowInt(row, "equipslot"), rowInt(row, "procanim")
	out := map[string]any{"item": id, "name": name}

	ats := map[string]any{"value": at, "name": attackTypeName(at)}
	for _, a := range entity.AttackTypeWords {
		if a.Value == at {
			ats["word"] = a.Word
		}
	}
	if at != 0 {
		ats["reads"] = entity.AttackTypeReading(at)
	}
	out["attackType"] = ats
	if at >= 7 && at <= 10 {
		out["range"] = rowInt(row, "unk5")
	}
	out["wornIn"] = fmt.Sprintf("%s (%d)", enumLabel("wearslot", slot), slot)
	it := map[string]any{"value": typ, "name": fmt.Sprintf("unknown (%d): not a type CLIENT names; it shows the race, class and level lines as a type does", typ)}
	for _, t := range database.ItemTypeWords {
		if t.Value == typ {
			it["name"], it["help"] = fmt.Sprintf("%s (%d)", t.Name, typ), t.Help
		}
	}
	out["itemType"] = it

	// the tags CLIENT's info shows (VIInfoText: words 553-555)
	var tags []string
	if rowInt(row, "trade") > 0 {
		tags = append(tags, "NO TRADE")
	}
	if rowInt(row, "rent") == 0 {
		tags = append(tags, "NO RENT")
	}
	if rowInt(row, "lore") > 0 {
		tags = append(tags, "LORE")
	}
	out["tags"] = tags

	// the weapon's proc, read as handlers' wornGear reads a hand's
	if wp, ok := entity.WeaponProcOf(id, name, desc); ok {
		p := map[string]any{"text": wp.Explain(), "chance": wp.Chance, "resist": entity.StatName(wp.Resist)}
		if wp.Chance == 0 {
			p["chanceRule"] = entity.WeaponProcChanceRule()
		}
		switch {
		case !procHand(slot):
			p["note"] = "read only from an item worn in a hand or the ranged slot: worn in " + enumLabel("wearslot", slot) + " it never procs"
		case strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), "heirloom "):
			p["note"] = "an heirloom carries its epic's proc from the epic's level (handlers/heirloom.go), which this does not show"
		}
		out["proc"] = p
	} else if strings.Contains(strings.ToLower(desc), "proc") {
		out["procUnread"] = "its Description speaks of a proc the server does not read (no figure it knows: \"Procs a 100 point DD Spell.\" is read)"
	}
	if ws, ok := entity.WornShieldOf(id, name); ok {
		out["worn"] = ws.Explain()
	}
	return out
}

// procHand is whether an item worn in that wear slot is read for a proc:
// handlers' wornGear reads the main hand's (equip slots 14, 16), the off
// hand's (15) and the ranged slot's (18), which these wear slots go in
// (handlers' itemSlotToEquipSlot); a held item (19, 20) is not read.
func procHand(wear int) bool {
	switch wear {
	case 12, 14, 15, 16, 17:
		return true
	}
	return false
}
