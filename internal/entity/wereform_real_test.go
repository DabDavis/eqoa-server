package entity

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// TestRealWereForms reads each spell the were lines' infections (and the
// forms bought after them) teach as the World casts it, on a real server's
// database (EQOA_REAL_DB; a copy is read): whom it lands on, the body it shows
// (the workbook's sprite read the other way round, else the most common body
// of the server's NPCs of the form's name: illusionBodyLocked) and the stats
// it changes. A form that shows no body, or none of the stats the guides'
// tables give the line's first form, is reported.
func TestRealWereForms(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	// a copy: the store makes its schema on open
	path := filepath.Join(t.TempDir(), "forms.db")
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	w := NewWorld()
	ns := database.NewNPCStore(db)
	if m, err := ns.RaceModels(); err == nil {
		w.SetRaceModels(m)
	}
	w.LoadNPCs(ns)
	// each line's infection (tools/quests/data/weres.json's p3, as imported)
	// teaches its first form: those spells, and the forms by the guides'
	// names beside them
	type form struct {
		line, name string
		first      bool // the line's infection teaches it
	}
	var forms []form
	for _, l := range []string{"wolfwere", "bearwere", "ratwere", "lionwere", "gatorwere"} {
		rows, err := db.Query(`SELECT p.spellname FROM quest_import_quests k JOIN queststeps s ON s.questid = k.questid
			JOIN spellpattern p ON (',' || replace(s.spells, ' ', '') || ',') LIKE ('%,' || p.spellid || ',%')
			WHERE k.key = ? AND s.spells != ''`, "Weres/"+l+"/p3")
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for rows.Next() {
			var name string
			rows.Scan(&name)
			forms = append(forms, form{l, name, true})
			n++
		}
		rows.Close()
		if n == 0 {
			t.Errorf("%s: the infection (Weres/%s/p3) teaches no spell", l, l)
		}
	}
	for _, name := range []string{"Werewolf Form", "Wolfwere", "Bearwere", "Ratwere", "Lionwere", "Gatorwere"} {
		forms = append(forms, form{"guide", name, false})
	}
	for _, f := range forms {
		var sp database.SpellPatternRecord
		var line, effects string
		err := db.QueryRow(`SELECT p.spellid, p.abilitylvl, p.unk2, p.unk3, p.range, p.casttime, p.power, p.scope, p.recast,
			p.eqprequire, p.spellname, p.spelldesc, COALESCE(e.line, ''), COALESCE(e.effects, '')
			FROM spellpattern p LEFT JOIN spelleffect e ON e.spellid = p.spellid WHERE p.spellname = ?`, f.name).
			Scan(&sp.SpellID, &sp.AbilityLvl, &sp.Unk2, &sp.Unk3, &sp.Range, &sp.CastTime, &sp.Power, &sp.Scope, &sp.Recast,
				&sp.EqpRequire, &sp.SpellName, &sp.SpellDesc, &line, &effects)
		if err != nil {
			t.Errorf("%s: %q: %v", f.line, f.name, err)
			continue
		}
		cs := SpellOfRow(&sp, 0, line, effects, 0)
		d := cs.Def
		model, size, ok := w.illusionBodyLocked(d.Form, 0)
		if d.FormModel != 0 {
			model, ok = d.FormModel, true
		}
		var stats []string
		for _, e := range d.Effects {
			if e.Kind == EffectStat {
				stats = append(stats, fmt.Sprintf("%v %+d", e.Stat, e.Amount))
			}
		}
		t.Logf("%s %q (%d): form %q kind %d, body %d (%#x) size %.2f found %v, lasts %v, recast %v, stats [%s], workbook effects %v",
			f.line, f.name, sp.SpellID, d.Form, d.FormKind, model, model, size, ok, d.Duration, cs.Recast,
			strings.Join(stats, ", "), effects != "")
		if f.first && (!ok || model == 0) {
			t.Errorf("%s: %q shows no body", f.line, f.name)
		}
		if f.first && len(stats) == 0 {
			t.Errorf("%s: %q changes no stat (the guides' first forms: two stats +45, INT -16)", f.line, f.name)
		}
		seen := map[string]bool{}
		for _, st := range stats {
			if f.first && seen[st] {
				t.Errorf("%s: %q changes %s twice (the guides' tables: once)", f.line, f.name, st)
			}
			seen[st] = true
		}
	}
}
