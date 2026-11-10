package gmweb

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/database"
)

// looks.go — what a look column's number IS, so the page can show it as a
// picture, a colour or a palette and offer a picker (static/lib/look.js), and
// an agent reading describe_table knows it too. Each was measured off the code
// that reads the column (2026-11-09; the user: "lets make the gm panel easier
// for players to change items, models, icons, etc. like hex and icons and
// model pictures, rgb sliders"):
//
//   - lookChar: a body, a CHAR.ESF CSprite's (or SimpleSprite's) DictID: npcs.modelid (442/442 live values,
//     claude-notes/gm-panel-20261022.md), characters.modelid, charactermodel, racedefs, petstats, itemeffect.body.
//   - lookItem: an ITEM.ESF model's DictID (GroupSprite, HSprite, SimpleSprite): what a hand holds on the state
//     channel (entity/statesync.go, bytes 67-78).
//   - lookItemIcon: an ITEMICON.ESF surface (each a 128x64 plate), itempattern.itemicon. It goes out zigzag in the
//     item record (handlers writeItemPattern) and CLIENT's VIMemStream::ReadSigned gathers 32 bits, so a value
//     kept unsigned past 2^31 loses its top bit on a PS2: kept signed (storeInt32); ScrollArt is matched exactly.
//   - lookUIIcon / lookUIBack: UI.CSF surfaces. A spell's icon is two (CLIENT's VIAbility m_iconBack then
//     m_iconFront, read in that order: spellpattern.iconColor then icon): the front a 32x32 picture, the back a
//     64x64 coloured gem it sits on. iconColor is no colour.
//   - lookItemLook: itempattern.model, read by the item's wear slot (handlers wornLook): an armour piece's set
//     (head, chest, forearms, legs, feet, hands: 0-9, 9 Sandstorm's), a robe's style (0-3), a cape's style
//     (capes.go), a hand's ITEM.ESF model.
//   - lookItemColor: itempattern.color: 0-14 a disc tint (entity.RobeLook: a robe is drawn in it; on armour
//     entity.ArmorColor leaves it undrawn), 0xFFFFFFFF plain, anything else 0xRRGGBBAA, drawn so by a client of
//     the live game's look and on a PS2 in the nearest tint (entity's nearestTint).
//   - lookRGBA: 0xRRGGBBAA, alpha never 0 (chardyes, database.ParseDyeColor).
//   - lookRaceLook: charlook.value: a colour (entity.Recolour: mode 1 a dye, 2 a tone, then hue, saturation,
//     lightness bytes) or a texture's DictID, by the row's kind.
//   - lookArmor*: an armour set on one slot of an NPC (bytes 93-98 of the channel), 0 none; the helm's is the
//     set's helm sprite.
//   - lookTint: a disc tint index, 0-14 (VICSpriteCust::Tints, database.DiscTints): npcs.unk_o.
//   - lookRobe: a robe style as npcs.robe_style's text: '0'-'3', or '255' or nothing for none.
//   - lookHair: which of the disc's eight hair textures (VICSpriteCust's $Hair): npcs.hair_color, characters.haircolor.
//   - lookVisual: a SPELLFX.CSF visual, 0x00B1's DictID; kept unsigned as SetSpellVisual writes it and the
//     community suggestions are matched exactly (database.DecideSpellFXSuggestions).
//
// The pictures themselves come off the GM's own disc in the browser (static/disc/): the server has no game data.
const (
	lookChar        = "char"
	lookItem        = "item"
	lookItemIcon    = "itemicon"
	lookUIIcon      = "uiicon"
	lookUIBack      = "uiback"
	lookItemLook    = "itemlook"
	lookItemColor   = "itemcolor"
	lookRGBA        = "rgba"
	lookRaceLook    = "racelook"
	lookArmorChest  = "armor:chest"
	lookArmorBracer = "armor:bracer"
	lookArmorGloves = "armor:gloves"
	lookArmorLegs   = "armor:legs"
	lookArmorBoots  = "armor:boots"
	lookArmorHelm   = "armor:helm"
	lookTint        = "tint"
	lookRobe        = "robe"
	lookHair        = "hair"
	lookVisual      = "visual"
)

const itemModelHelp = "by Worn in: an armour piece's set (head, chest, forearms, legs, feet, hands: 0 none, 1-8 the disc's, " +
	"9 Sandstorm's), a robe's style (0-3), a cape's style, a weapon's, shield's or held item's ITEM.ESF model DictID"

const itemColorHelp = "0-14 a disc tint (a robe is drawn in it; armour shows nothing of it), 0xFFFFFFFF plain, else " +
	"0xRRGGBBAA, drawn so by a client of the live game's look and on a PS2 in the nearest tint"

// tintNames are the disc tints' names (database.DiscTints, in order).
var tintNames = [len(database.DiscTints)]string{"White", "Dark grey", "Sky blue", "Brown", "Navy", "Green", "Gold",
	"Olive", "Red", "Ice blue", "Silver", "Tan", "Teal", "Burgundy", "Yellow"}

// paletteColour is a palette entry as describe_table hands it: its index (a
// tint's) or none, its name and its colour as RRGGBBAA (as #dye says one).
type paletteColour struct {
	Value *int   `json:"value,omitempty"`
	Label string `json:"label"`
	RGBA  string `json:"rgba"`
}

type paletteFamily struct {
	Label   string          `json:"label"`
	Colours []paletteColour `json:"colours"`
}

// lookPalettes are the palettes a table's look columns draw: the disc tints
// for a tint index and for the colours (whose PS2 drawing is the nearest
// tint), the Armor Dyers' colours for a colour.
func lookPalettes(t *Table) map[string]any {
	var tints, dyes bool
	for _, c := range t.Columns {
		switch c.Look {
		case lookTint:
			tints = true
		case lookItemColor, lookRGBA:
			tints, dyes = true, true
		}
	}
	if !tints {
		return nil
	}
	out := map[string]any{}
	list := make([]paletteColour, len(database.DiscTints))
	for i, c := range database.DiscTints {
		v := i
		list[i] = paletteColour{Value: &v, Label: tintNames[i], RGBA: database.FormatDye(database.RGBA(c))}
	}
	out["tints"] = list
	if dyes {
		var fams []paletteFamily
		for _, f := range database.DyeFamilies {
			pf := paletteFamily{Label: f.Label}
			for _, c := range f.Colours {
				pf.Colours = append(pf.Colours, paletteColour{Label: c.Label, RGBA: database.FormatDye(c.RGBA)})
			}
			fams = append(fams, pf)
		}
		out["dyes"] = fams
	}
	return out
}

// lookSource is a column a kind of look's ids are kept in, and what names a
// row holding one (an SQL expression over the table's columns).
type lookSource struct {
	table, col, name, where string
	// label, when set, names rows from columns the name expression returns
	// joined by "|" (charactermodel's race and sex)
	label func(parts []string) string
}

// lookSources are where look_names finds each kind's ids.
var lookSources = map[string][]lookSource{
	lookChar: {
		{table: "npcs", col: "modelid", name: "npc_name"},
		{table: "charactermodel", col: "modelid", name: "race || '|' || sex", label: raceSexLabel},
		{table: "racedefs", col: "body_male", name: "name || ' (male)'"},
		{table: "racedefs", col: "body_female", name: "name || ' (female)'"},
		{table: "petstats", col: "model", name: "name || ' (pet)'"},
		{table: "itemeffect", col: "body", name: "CASE WHEN name != '' THEN name ELSE 'item ' || patternid END"},
	},
	lookItem: {
		{table: "itempattern", col: "model", name: "itemname", where: "equipslot IN (12, 13, 14, 15, 16, 17, 18, 20)"},
		{table: "npcs", col: "primary_weapon", name: "npc_name || ' (held)'"},
		{table: "npcs", col: "secondary_weapon", name: "npc_name || ' (held)'"},
		{table: "npcs", col: "shield", name: "npc_name || ' (shield)'"},
	},
	lookItemIcon: {{table: "itempattern", col: "itemicon", name: "itemname"}},
	lookUIIcon: {
		{table: "spellpattern", col: "icon", name: "spellname"},
		{table: "itemeffect", col: "icon", name: "CASE WHEN name != '' THEN name ELSE 'item ' || patternid END"},
	},
	lookUIBack: {{table: "spellpattern", col: "iconColor", name: "spellname"}},
	lookVisual: {
		{table: "spelleffect", col: "fxdict", name: "COALESCE((SELECT spellname FROM spellpattern p WHERE p.spellid = spelleffect.spellid), 'spell ' || spellid)"},
		{table: "classtemplates", col: "special_fx", name: "'special of ' || name"},
	},
}

func raceSexLabel(parts []string) string {
	if len(parts) != 2 {
		return strings.Join(parts, " ")
	}
	race, sex := parts[0], parts[1]
	for _, e := range enums()["race"] {
		if fmt.Sprint(e.Value) == race {
			race = e.Label
		}
	}
	for _, e := range enums()["sex"] {
		if fmt.Sprint(e.Value) == sex {
			sex = strings.ToLower(e.Label)
		}
	}
	return race + " " + sex + " (player)"
}

// lookUse is one id look_names answers with.
type lookUse struct {
	ID    uint32   `json:"id"`
	Hex   string   `json:"hex"`
	Rows  int      `json:"rows"`
	Names []string `json:"names"`
}

// maxLookNames is how many names an id is answered with, the commonest first.
const maxLookNames = 6

func (s *Server) registerLookOps() {
	kinds := make([]string, 0, len(lookSources))
	for k := range lookSources {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	s.addOp(&Op{
		Name: "look_names", Title: "What wears each model, icon or visual", Scope: scopeRead,
		Description: "Every id a kind of look column holds, with how many rows hold it and up to six names of what " +
			"does, the commonest first, so a body, a model or an icon is told by what wears it: char (CHAR.ESF bodies: " +
			"NPCs, player races, custom races, pets, potions' bodies), item (ITEM.ESF models: weapons, shields and held " +
			"items, and the NPCs holding them), itemicon (ITEMICON.ESF: the items showing each), uiicon (UI.CSF icons: " +
			"spells, item effects), uiback (UI.CSF icon backs: spells), visual (SPELLFX visuals: spells, class " +
			"templates' specials). An id is its 32 bits unsigned (hex beside it), whichever way a table keeps it. A role " +
			"below a GM is answered from its own tables alone. An answer is kept a minute.",
		Input: obj(map[string]any{"kind": strEnum("the kind of look (describe_table names a column's as look)", kinds...)}, "kind"),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ Kind string }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			srcs, ok := lookSources[req.Kind]
			if !ok {
				return nil, badRequest("kind %q: one of %s", req.Kind, strings.Join(kinds, ", "))
			}
			return c.s.lookNames(c, srcs)
		},
	})
}

// lookCache keeps look_names' answers for lookCacheTTL: the NPCs' bodies are a
// read of every npcs row (60,000: half a second on the game's one database
// connection), and a page asks once a visit. A name a minute late is harmless.
type lookCache struct {
	mu sync.Mutex
	m  map[string]lookCacheEntry
}

type lookCacheEntry struct {
	at  time.Time
	out any
}

const lookCacheTTL = time.Minute

// lookNames is look_names' answer: each source read whole (one query each,
// every row taken before the next: the database has one connection), the
// counts and names per id put together after; kept a minute for the tables
// the asker reads.
func (s *Server) lookNames(c *opCtx, srcs []lookSource) (any, error) {
	var key strings.Builder
	for _, src := range srcs {
		if !c.a.limited() || c.a.readsTable(src.table) {
			key.WriteString(src.table + "." + src.col + "|")
		}
	}
	s.looks.mu.Lock()
	if e, ok := s.looks.m[key.String()]; ok && time.Since(e.at) < lookCacheTTL {
		s.looks.mu.Unlock()
		return e.out, nil
	}
	s.looks.mu.Unlock()
	out, err := s.readLookNames(c, srcs)
	if err == nil {
		s.looks.mu.Lock()
		if s.looks.m == nil {
			s.looks.m = map[string]lookCacheEntry{}
		}
		s.looks.m[key.String()] = lookCacheEntry{at: time.Now(), out: out}
		s.looks.mu.Unlock()
	}
	return out, err
}

func (s *Server) readLookNames(c *opCtx, srcs []lookSource) (any, error) {
	type tally struct {
		rows  int
		names map[string]int
	}
	by := map[uint32]*tally{}
	var read []string
	for _, src := range srcs {
		if c.a.limited() && !c.a.readsTable(src.table) {
			continue
		}
		t, err := s.table(src.table)
		if err != nil || t.col(src.col) == nil {
			continue // a table or a column this database lacks
		}
		where := src.col + " IS NOT NULL AND " + src.col + " != 0"
		if src.where != "" {
			where += " AND " + src.where
		}
		rs, err := s.d.DB.Query(`SELECT ` + src.col + ` & 4294967295, COALESCE(` + src.name + `, ''), COUNT(*) FROM ` + src.table +
			` WHERE ` + where + ` GROUP BY 1, 2`)
		if err != nil {
			return nil, err
		}
		type row struct {
			id   int64
			name string
			n    int
		}
		var rows []row
		for rs.Next() {
			var r row
			if err := rs.Scan(&r.id, &r.name, &r.n); err != nil {
				rs.Close()
				return nil, err
			}
			rows = append(rows, r)
		}
		err = rs.Err()
		rs.Close()
		if err != nil {
			return nil, err
		}
		read = append(read, src.table+"."+src.col)
		for _, r := range rows {
			id := uint32(r.id)
			tl := by[id]
			if tl == nil {
				tl = &tally{names: map[string]int{}}
				by[id] = tl
			}
			tl.rows += r.n
			name := strings.TrimSpace(r.name)
			if src.label != nil {
				name = src.label(strings.Split(name, "|"))
			}
			if name != "" {
				tl.names[name] += r.n
			}
		}
	}
	out := make([]lookUse, 0, len(by))
	for id, tl := range by {
		u := lookUse{ID: id, Hex: fmt.Sprintf("0x%08X", id), Rows: tl.rows, Names: []string{}}
		names := make([]string, 0, len(tl.names))
		for n := range tl.names {
			names = append(names, n)
		}
		sort.Slice(names, func(i, j int) bool {
			if tl.names[names[i]] != tl.names[names[j]] {
				return tl.names[names[i]] > tl.names[names[j]]
			}
			return names[i] < names[j]
		})
		if len(names) > maxLookNames {
			names = names[:maxLookNames]
		}
		u.Names = append(u.Names, names...)
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Rows != out[j].Rows {
			return out[i].Rows > out[j].Rows
		}
		return out[i].ID < out[j].ID
	})
	return map[string]any{"ids": out, "read": read}, nil
}
