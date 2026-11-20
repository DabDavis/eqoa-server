package gmweb

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// A sub GM's panel (the owner, 2026-11-20: "give sub gms the gm panel map also
// to move mobs"): a sub GM is a tester whose gm_testers row says subgm
// (tester.go), and beside the tester's tools it holds this role: the World map
// (every map operation that reads) and moving a creature there, which is the
// map's "Move it…": update_row on an npcs row, its place alone (subGMMoveCols).
// Nothing else of an NPC (its name, body, level, loot), no row added or taken
// away, no path, teleport, spawn or suggestion accepted: those stay a GM's.
// Every move is audited under the sub GM and reverted by it or a GM.

// roleSubGM is the sub GM's role, as a sign-in and server_status report it
// ("tester+subgm").
const roleSubGM = "subgm"

// subGMMoveCols are the npcs columns a sub GM writes: where the creature
// stands (its spawn) and which way it faces.
var subGMMoveCols = []string{"x_coord", "y_coord", "z_coord", "world", "facing"}

// subGMTableList is what a sub GM reads and changes: the NPCs' rows.
var subGMTableList = []string{"npcs"}

// subGMOps is the whole of what a sub GM may call beside a tester's. The map
// reads are the page's; update_row is the move, held to subGMRow.
var subGMOps = map[string]func(c *opCtx) error{
	"guide":           nil,
	"server_status":   nil, // limitedStatus
	"describe_table":  subGMNamesTable,
	"get_row":         subGMNamesTable,
	"map_markers":     nil,
	"map_layers":      nil,
	"map_marks":       nil,
	"map_quests":      nil,
	"map_camp":        nil,
	"map_suggestions": nil,
	"world_npcs":      nil,
	"npc_paths":       nil, // the map draws them; changing one is a GM's
	"npc_path":        nil,
	"update_row":      subGMMoves,
	"revert_change":   subGMRevert,
}

var subGMGrant = &grant{
	role:   roleSubGM,
	ops:    subGMOps,
	tables: subGMTableList,
	writes: func(t string) bool { return t == "npcs" },
	row:    subGMRow,
	mayRevert: func(a *actor, e *auditEntry) bool {
		return e.Table == "npcs" && subGMMayRevert(a, e) == nil
	},
	access: func(t *Table) tableAccess {
		if t.Name != "npcs" {
			return tableAccess{Write: writeNone}
		}
		return tableAccess{Write: roleSubGM, Only: subGMMoveCols}
	},
	refuse: notSubGMs,
	may:    "use the World map and move creatures on it",
	counts: map[string]string{"npcs": "npcs"},
	about: "A sub GM's tools on an EverQuest Online Adventures private server: the World map and moving a creature's " +
		"spawn (update_row on npcs: x_coord, y_coord, z_coord, world, facing alone). Every move is audited and can be reverted.",
	instructions: "You act for a sub GM: read the map (map_markers, map_layers, world_npcs) and move a creature with " +
		"update_row on table npcs, changing only x_coord, y_coord, z_coord, world and facing. Nothing else is yours.",
	guide: (*Server).subGMGuide,
}

func notSubGMs(what string) error {
	return refusedAs(what, what+" is not a sub GM's: a sub GM uses the World map and moves creatures on it "+
		"(their place alone), nothing else")
}

func subGMNamesTable(c *opCtx) error {
	var req struct{ Table string }
	if err := c.decode(&req); err != nil {
		return err
	}
	if !slices.Contains(subGMTableList, req.Table) {
		return notSubGMs("table " + strconv.Quote(req.Table))
	}
	return nil
}

// subGMMoves is update_row's rule for a sub GM: table npcs, its place alone.
// The row editor holds the change to subGMRow once more (roleRow).
func subGMMoves(c *opCtx) error {
	var req struct {
		Table           string
		Values, Changes map[string]any
	}
	if err := c.decode(&req); err != nil {
		return err
	}
	if req.Table != "npcs" {
		return notSubGMs("table " + strconv.Quote(req.Table))
	}
	if err := subGMRow(req.Table, "update", req.Values); err != nil {
		return err
	}
	return subGMRow(req.Table, "update", req.Changes)
}

// subGMRow: a change of an npcs row's place alone; no row added or taken away.
func subGMRow(table, action string, vals map[string]any) error {
	if table != "npcs" {
		return notSubGMs("changing table " + strconv.Quote(table))
	}
	if action != "update" {
		return forbidden("a sub GM adds no npcs row and takes none away, it moves one: a GM's")
	}
	for _, col := range colsOf(vals) {
		if !slices.Contains(subGMMoveCols, col) {
			return forbidden("npcs.%s is not a sub GM's: a sub GM moves a creature (%s), nothing else of it",
				col, strings.Join(subGMMoveCols, ", "))
		}
	}
	return nil
}

func subGMRevert(c *opCtx) error {
	var req struct{ ID int64 }
	if err := c.decode(&req); err != nil {
		return err
	}
	e, err := c.s.auditGet(req.ID)
	if err != nil {
		return notFound("%v", err)
	}
	if e.Table != "npcs" {
		return notSubGMs("reverting that entry")
	}
	return subGMMayRevert(c.a, e)
}

// subGMMayRevert: a move of a creature alone (its place's columns), its own or
// another sub GM's.
func subGMMayRevert(_ *actor, e *auditEntry) error {
	if e.Action != "update" {
		return forbidden("reverting that would add or take away a whole npcs row: a GM's")
	}
	for _, col := range changedColumns(e.Before, e.After) {
		if !slices.Contains(subGMMoveCols, col) {
			return forbidden("reverting that sets npcs.%s back, which is not a sub GM's: a GM's", col)
		}
	}
	return nil
}

func (s *Server) subGMGuide(_ *http.Request) string {
	return "# The sub GM's panel\n\n" +
		"A sub GM has the tester's item and spell tools and the **World map**. On the map, click a creature, " +
		"then **Move it…** and click where it should stand: its spawn moves there at once (its height is kept). " +
		"Only a creature's place and facing are a sub GM's (npcs: " + strings.Join(subGMMoveCols, ", ") + "); its " +
		"name, body, level, loot, paths and everything else stay a GM's. Every move is in the Activity log under " +
		"your name, and revert_change puts one back.\n"
}
