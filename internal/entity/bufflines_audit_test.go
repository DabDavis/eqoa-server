package entity

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// TestBuffLinesAudit (2026-11-14, the owner: "check the lines across every
// class") reads every beneficial spell of a database as the server casts it
// (SpellDefFrom, its recast group), puts each on a bearer as a level 60
// caster's cast would (lastingParts) and writes, as JSON, every spell's line
// and rank and every pair of them the engine holds to one line: the one
// refused under the other (lineVerdict) or taken off by it (lineDrops).
// tools/buff_lines_20261114.py's audit reads it. Run only with
// BUFFLINES_DB=<a copy of a database> BUFFLINES_OUT=<json file>.
func TestBuffLinesAudit(t *testing.T) {
	src, out := os.Getenv("BUFFLINES_DB"), os.Getenv("BUFFLINES_OUT")
	if src == "" || out == "" {
		t.Skip("BUFFLINES_DB and BUFFLINES_OUT not set")
	}
	cp := filepath.Join(t.TempDir(), "eqoa.db")
	in, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cp, in, 0o600); err != nil {
		t.Fatal(err)
	}
	log.SetOutput(io.Discard)
	db, err := database.NewSQLite(cp)
	log.SetOutput(os.Stderr)
	if err != nil {
		t.Fatal(err)
	}
	type owner struct {
		Class  int    `json:"class"`
		Source string `json:"source"`
	}
	owners := map[int][]owner{}
	rows, err := db.Query(`SELECT class, spellid, source FROM classspells`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var o owner
		var id int
		if err := rows.Scan(&o.Class, &id, &o.Source); err != nil {
			t.Fatal(err)
		}
		owners[id] = append(owners[id], o)
	}
	rows.Close()
	store := database.NewSpellbookStore(db)
	rows, err = db.Query(`SELECT spellid FROM spellpattern ORDER BY spellid`)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int
	for rows.Next() {
		var id int
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	type spellOut struct {
		ID      int            `json:"id"`
		Name    string         `json:"name"`
		Level   int            `json:"level"`
		Scope   int            `json:"scope"`
		Stack   int            `json:"stack"`
		Rank    int            `json:"rank"`
		Line    string         `json:"line"`
		Mods    map[string]int `json:"mods"`
		Speed   int            `json:"speed"`
		Invis   bool           `json:"invis"`
		Proc    int            `json:"proc"`
		Recast  string         `json:"recast_group"`
		Owners  []owner        `json:"owners"`
		Workbk  bool           `json:"workbook"`
		Harmful bool           `json:"harmful"`
	}
	var spells []spellOut
	var effs []*ActiveEffect
	caster := &Player{Level: 60}
	now := time.Now()
	for _, id := range ids {
		sp, err := store.SpellPattern(id)
		if err != nil || sp == nil {
			continue
		}
		row, _, _ := store.SpellEffectRow(id)
		def := SpellDefFrom(sp.SpellName, row.Line, sp.SpellDesc, row.Effects)
		if def.Harmful {
			continue
		}
		cs := &castState{spell: CastSpell{SpellID: sp.SpellID, Level: sp.AbilityLvl, Scope: sp.Scope, Name: sp.SpellName, Def: def}}
		if grp, _, err := store.RecastGroup(id); err == nil {
			cs.spell.RecastGroup = grp
		}
		ae := func() (ae *ActiveEffect) {
			defer func() {
				if recover() != nil {
					ae = nil
				}
			}()
			return lastingParts(cs, caster, now)
		}()
		if ae == nil {
			continue
		}
		ae.Line, ae.Rank = EffectLine(&cs.spell), lineRank(&cs.spell)
		if ae.Line == "" {
			continue
		}
		m := map[string]int{}
		for st := Stat(1); int(st) < len(statLineNames)-1; st++ {
			if v := ae.Mods.get(st); v != 0 {
				m[statLineNames[st]] = v
			}
		}
		spells = append(spells, spellOut{ID: id, Name: sp.SpellName, Level: sp.AbilityLvl, Scope: sp.Scope, Stack: def.Stack,
			Rank: ae.Rank, Line: ae.Line, Mods: m, Speed: speedPct(ae.Speed, ae.Rules.SowPct), Invis: ae.Invis,
			Proc: ae.Proc + ae.ProcMax, Recast: cs.spell.RecastGroup, Owners: owners[id], Workbk: def.Workbook})
		effs = append(effs, ae)
	}
	type pairOut struct {
		A, B int    // spell ids (indexes' ids)
		Kind string // "refuses": A on refuses B; "drops": B landing takes A off
	}
	var pairs []pairOut
	for i, a := range effs {
		for j, b := range effs {
			if i == j || a.SpellID == b.SpellID {
				continue
			}
			if lineVerdict([]*ActiveEffect{a}, b) != nil {
				pairs = append(pairs, pairOut{a.SpellID, b.SpellID, "refuses"})
			}
			if slices.Contains(lineDrops([]*ActiveEffect{a}, b), a.SpellID) {
				pairs = append(pairs, pairOut{a.SpellID, b.SpellID, "drops"})
			}
		}
	}
	buf, _ := json.Marshal(map[string]any{"spells": spells, "pairs": pairs})
	if err := os.WriteFile(out, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d beneficial spells with a line, %d pair relations", len(spells), len(pairs))
}
