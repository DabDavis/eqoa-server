package gmweb

import (
	"database/sql"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/eqoa/server/internal/database"
)

// ops_instanceaura.go — an aura and a tag on ONE copy of a held item (table
// instanceaura, database/instanceaura.go): instance_aura reads what a
// character (or its mercenary) holds in a hand and the copy's row;
// set_instance_aura writes it, audited (a gm_audit row change, revertible as
// any). The copy is picked by character and hand, or by its itemid.

// instanceAuraIn is one of a copy's auras as the operations take it.
type instanceAuraIn struct {
	Kind   *int   `json:"kind"`
	Colour string `json:"colour"`
	Source string `json:"source"`
	Along  *int   `json:"along"`
}

// aura reads it over was (a field left out keeps was's).
func (in *instanceAuraIn) aura(was database.ItemAura) (database.ItemAura, error) {
	a := was
	if in == nil {
		return a, nil
	}
	if in.Kind != nil {
		if *in.Kind != 0 && *in.Kind != database.AuraNone && !database.HandAuraKind(int64(*in.Kind)) {
			return a, badRequest("a held item's aura kind is 0 none, 3 the particles, 7 an epic's look or 9 an energy skin")
		}
		a.Kind = uint8(*in.Kind)
	}
	hex := func(s, what string) (uint32, error) {
		s = strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "0x"), "#")
		v, err := strconv.ParseUint(s, 16, 32)
		if err != nil {
			return 0, badRequest("%s is 32-bit hex (000000FF, 5FD4AE7C)", what)
		}
		return uint32(v), nil
	}
	var err error
	if in.Colour != "" {
		if a.Color, err = hex(in.Colour, "colour"); err != nil {
			return a, err
		}
	}
	if in.Source != "" {
		if a.Source, err = hex(in.Source, "source"); err != nil {
			return a, err
		}
	}
	if in.Along != nil {
		if *in.Along < 0 || *in.Along > database.AuraAlongMost {
			return a, badRequest("along is 0-%d, percent of the item's length from the hand", database.AuraAlongMost)
		}
		a.Nodes = uint8(*in.Along)
	}
	return a, nil
}

// instanceAuraPick is how the operations name a copy.
type instanceAuraPick struct {
	ItemID    int    `json:"itemid"`
	Character int    `json:"character"`
	Name      string `json:"name"`
	Merc      int    `json:"merc"`
	Hand      string `json:"hand"`
}

var instanceAuraPickProps = map[string]any{
	"itemid":    integer("the copy: Character items' Item row (negative: a mercenary's record, -charmercitem.id); or give character/name and hand"),
	"character": integer("the character's server_id"),
	"name":      str("the character's name (in place of character)"),
	"merc":      integer("0 the character itself (the default), else its mercenary of that hire slot"),
	"hand":      strEnum("the hand whose held copy: mainhand or offhand (a shield held there too)", "mainhand", "offhand"),
}

// pick finds the copy: its key, and words for it.
func (s *Server) pickInstanceCopy(p instanceAuraPick) (int, string, error) {
	db := s.d.DB
	if db == nil {
		return 0, "", badRequest("no database")
	}
	if p.ItemID != 0 {
		return p.ItemID, fmt.Sprintf("item copy %d", p.ItemID), nil
	}
	sid := p.Character
	if sid == 0 && p.Name != "" {
		if err := db.QueryRow(`SELECT serverid FROM characters WHERE charName = ? COLLATE NOCASE`, p.Name).Scan(&sid); err == sql.ErrNoRows {
			return 0, "", notFound("no character named %q", p.Name)
		} else if err != nil {
			return 0, "", err
		}
	}
	if sid <= 0 {
		return 0, "", badRequest("give itemid, or character (or name) and hand")
	}
	hand := database.HandMain
	switch p.Hand {
	case "mainhand", "":
	case "offhand":
		hand = database.HandOff
	default:
		return 0, "", badRequest("hand is mainhand or offhand")
	}
	id, pat, name, err := db.HeldCopy(uint32(sid), p.Merc, hand)
	if err != nil {
		return 0, "", err
	}
	whose := fmt.Sprintf("character %d", sid)
	if p.Merc > 0 {
		whose += fmt.Sprintf("'s mercenary %d", p.Merc)
	}
	if id == 0 {
		return 0, "", notFound("%s holds no copy of its own in the %s (nothing, or a mercenary's lent heirloom)", whose, map[int]string{0: "main hand", 1: "off hand"}[hand])
	}
	return id, fmt.Sprintf("%s's %s copy %d (%s, record %d)", whose, map[int]string{0: "main hand", 1: "off hand"}[hand], id, name, pat), nil
}

func instanceAuraOut(a database.InstanceAura, has bool, what string) map[string]any {
	out := map[string]any{"itemid": a.ItemID, "copy": what, "has_row": has}
	if has {
		row := func(x database.ItemAura) map[string]any {
			return map[string]any{"kind": x.Kind, "colour": fmt.Sprintf("%08X", x.Color), "source": fmt.Sprintf("%08X", x.Source), "along": x.Nodes}
		}
		out["tag"], out["first"], out["second"], out["note"], out["source_tool"] = a.Tag, row(a.First), row(a.Second), a.Note, a.Source
	}
	return out
}

func (s *Server) registerInstanceAuraOps() {
	s.addOp(&Op{
		Name: "instance_aura", Title: "A held item copy's own aura", Scope: scopeRead,
		Description: "The copy of an item a character (or its mercenary) holds in a hand, and that copy's own aura and tag (table instanceaura), " +
			"by character (or name) and hand, or by the copy's itemid.",
		Input: obj(instanceAuraPickProps),
		Handler: func(c *opCtx) (any, error) {
			var p instanceAuraPick
			if err := c.decode(&p); err != nil {
				return nil, err
			}
			id, what, err := c.s.pickInstanceCopy(p)
			if err != nil {
				return nil, err
			}
			a, has, err := c.s.d.DB.InstanceAuraOf(id)
			if err != nil {
				return nil, err
			}
			a.ItemID = id
			return instanceAuraOut(a, has, what), nil
		},
	})
	props := map[string]any{}
	for k, v := range instanceAuraPickProps {
		props[k] = v
	}
	auraProps := obj(map[string]any{
		"kind":   integer("0 none, 3 the particles, 7 an epic's look, 9 an energy skin"),
		"colour": str("0xRRGGBBAA hex, 0 the source's own colours; 000000FF a dark aura"),
		"source": str("the source DictID in hex (Sickle of Avhi Escron 5FD4AE7C, the freezing 6C151A60, the twinkle 1D875ECD); 0 the Sword of Virtue's"),
		"along":  integer("where along the item, 1-100 percent from the hand; 0 the source's own place"),
	})
	props["tag"] = str("the tag shown as \"Aura: <tag>\" with the item on every client, a PS2 too (24 letters, digits, spaces, ' or -); \"\" none; left out: kept")
	props["first"] = auraProps
	props["second"] = auraProps
	props["clear"] = boolean("true: the copy's row gone (tag and both auras)")
	props["note"] = str("why")
	s.addOp(&Op{
		Name: "set_instance_aura", Title: "Set a held item copy's own aura and tag", Scope: scopeWrite, Destructive: true,
		Description: "An aura on ONE copy of a held item (table instanceaura), shown on whoever holds that copy and on no other copy of the item, " +
			"carried through trades, the bank, the auction house and mercenaries; and its tag, \"Aura: <tag>\" at the head of its description on every " +
			"client. Pick the copy by character (or name) and hand, or by itemid. first and second are its two auras (a field left out keeps the " +
			"copy's); clear removes the row. A hand's order: the character's own (charaura 9/10) first, then this, then the item record's (itemaura). " +
			"Audited; at once in the world.",
		Input: obj(props),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				instanceAuraPick
				Tag    *string         `json:"tag"`
				First  *instanceAuraIn `json:"first"`
				Second *instanceAuraIn `json:"second"`
				Clear  bool            `json:"clear"`
				Note   *string         `json:"note"`
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			id, what, err := c.s.pickInstanceCopy(req.instanceAuraPick)
			if err != nil {
				return nil, err
			}
			db := c.s.d.DB
			row, _, err := db.InstanceAuraOf(id)
			if err != nil {
				return nil, err
			}
			row.ItemID = id
			if req.Clear {
				row = database.InstanceAura{ItemID: id}
			} else {
				if req.Tag != nil {
					if !database.ValidInstanceTag(*req.Tag) {
						return nil, badRequest("a tag is at most %d letters, digits, spaces, ' or -", database.InstanceTagMost)
					}
					row.Tag = *req.Tag
				}
				if row.First, err = req.First.aura(row.First); err != nil {
					return nil, err
				}
				if row.Second, err = req.Second.aura(row.Second); err != nil {
					return nil, err
				}
				if req.Note != nil {
					row.Note = *req.Note
				}
			}
			row.Source = "gm"
			if err := db.SetInstanceAuraAudited(row, workWho(c.a), "the GM panel: set_instance_aura on "+what); err != nil {
				return nil, badRequest("%v", err)
			}
			live := c.s.liveItemAuras()
			after, has, err := db.InstanceAuraOf(id)
			if err != nil {
				return nil, err
			}
			after.ItemID = id
			out := instanceAuraOut(after, has, what)
			out["live"] = live
			return out, nil
		},
	})
}

// checkInstanceAuraRow holds an instanceaura row edited in the panel to what
// the client draws and shows (database.CheckInstanceAura's rules, on the
// columns given).
func checkInstanceAuraRow(row map[string]any) error {
	if v, ok := row["itemid"]; ok {
		if f, fok := asFloat(v); !fok || f != math.Trunc(f) || f == 0 {
			return badRequest("itemid is the copy: Character items' Item row, or a mercenary's record negated")
		}
	}
	if v, ok := row["tag"]; ok {
		t, _ := v.(string)
		if !database.ValidInstanceTag(t) {
			return badRequest("tag is at most %d letters, digits, spaces, ' or -", database.InstanceTagMost)
		}
	}
	for _, col := range []string{"color", "source", "color2", "source2"} {
		if v, has := row[col]; has {
			if f, fok := asFloat(v); !fok || f != math.Trunc(f) || !database.ValidNPCTint(int64(f)) {
				return badRequest("%s is a 32-bit value (0xRRGGBBAA for a colour, a DictID for a source)", col)
			}
		}
	}
	for _, col := range []string{"kind", "kind2"} {
		if v, has := row[col]; has {
			f, fok := asFloat(v)
			if !fok || f != math.Trunc(f) || f != 0 && f != database.AuraNone && !database.HandAuraKind(int64(f)) {
				return badRequest("%s is a held item's: 0 none, 3 the particles, 7 an epic's look, 9 an energy skin", col)
			}
		}
	}
	for _, col := range []string{"nodes", "nodes2"} {
		if v, has := row[col]; has {
			if f, fok := asFloat(v); !fok || f != math.Trunc(f) || f < 0 || f > database.AuraAlongMost {
				return badRequest("%s is where along the held item, 0-%d", col, database.AuraAlongMost)
			}
		}
	}
	return nil
}
