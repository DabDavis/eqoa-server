package entity

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// tools/fix_moogie_tunare_20261110.py's rows read by this server: on a database made by the server's own schema,
// with Mr. Moogie's, Tunare's, Innoruuk's and Metal Moogie's rows as the brief found them, the tool's --apply leaves
// every template one the world can use (no problem named), Mr. Moogie's with his new parts, V2's with his attack from
// afar, Innoruuk's with Largesse of Plague's proc read off its spell and his veil; its --undo puts Mr. Moogie's back.
// It needs python3 (skipped without it).
func TestTheRaidGodsDataToolsRowsLoad(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("no python3")
	}
	tool, err := filepath.Abs("../../tools/fix_moogie_tunare_20261110.py")
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/gods.db"
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	exec1 := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	js := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	spell := func(id int, name, line, effects string) {
		exec1(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast,
			eqprequire, spellname, spelldesc) VALUES (?, 60, 0, 0, 16880, 0, 0, 0, 5, 1, 0, 0, ?, '')`, id, name)
		exec1(`INSERT INTO spelleffect (spellid, line, fxdict, source, effects) VALUES (?, ?, 0, 'test', ?)`, id, line, effects)
	}
	spell(700046, "Lay Hands", "Target, Increase Hitpoints by 1", "")
	spell(701115, "Test Thorns", "Target, Decrease Hitpoints by 900", "")
	spell(700744, "Test Withering", "Target, Decrease AC by 50, 2min", "")
	spell(700372, "Test Stormstrike", "Target, Decrease Hitpoints by 1200", "")
	spell(900486, "Diviner's Touch", "Target, Increase Hitpoints by 500", "")
	spell(905100, "Test Swarm", "Target, Decrease HP per tic by 300, 24sec", "")
	spell(905101, "Test Swarm Tap", "Target, Drain HP by 400 give self", "")
	spell(905102, "Test Swarm Bolt", "Target, Decrease Hitpoints by 800", "")
	spell(9100, "Clockwork Siphon", "Target, Drain HP by 750 give self, Unresistable", "")
	for _, id := range []int{905145, 905146, 905147, 905148} {
		spell(id, fmt.Sprintf("Test Hate %d", id), "Target, Decrease Hitpoints by 1000", "")
	}
	spell(700794, "Largesse of Plague", "", `{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"duration_s":3600,"effects":[`+
		`{"id":"0001","type":"Offensive Proc","kind":"proc","procs":[{"id":"0002","type":"Dmg/Heal","kind":"hp","amount":-650,"resist":"dr",`+
		`"on":"offense","scope":"target","target_type":"Enemy"}]}]}`)
	npc := `INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, modelid, size, npc_level, npc_type, npc_id, race)
		VALUES (?, ?, 0, ?, 54, 15756, ?, ?, ?, ?, ?, '0')`
	exec1(npc, 500, "Mr. Moogie", 25318.8, 1234, 2.0, 80, 128, 500)
	exec1(npc, 501, "Metal Moogie", 25290.0, 4567, 7.5, 60, 0, 501)
	exec1(npc, 502, "Innoruuk", 25000.0, 789, 3.0, 70, 0, 502)
	exec1(`INSERT INTO npcspawnonly (name, note, source) VALUES ('metal moogie', '', 'test')`)
	tmpl := `INSERT INTO classtemplates (name, class, hp_pct, melee_pct, kick_damage, kick_every, proc_chance, proc_damage, proc_name,
		proc_resist, abilities) VALUES (?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	moogieNow := `[{"kind":"strip","every":30,"first":30,"name":"Royal Decree"},{"kind":"heal","spell_id":700046,"heal_pct":100,"below":50,` +
		`"every":3600,"cooldown":3600,"say":"Thats an hour cool down, you'll pay for that","proc_chance":25,"proc_damage":1500,"proc_name":"Royal Fury"}]`
	exec1(tmpl, "mr moogie", 6000, 2500, 4000, 5.0, 0, 0, "", "", moogieNow)
	exec1(tmpl, "tunare", 5000, 500, 0, 0.0, 0, 0, "", "", `[{"kind":"nuke","spell_id":701115,"every":8,"first":2,"damage_pct":200},`+
		`{"kind":"debuff","spell_id":700744,"every":24,"first":5,"damage_pct":300},{"kind":"nuke","spell_id":700372,"every":30,"first":12,"damage_pct":300},`+
		`{"kind":"heal","spell_id":900486,"every":20,"below":50,"damage_pct":1000}]`)
	inno := []map[string]any{{"kind": "meleekind", "melee_kind": "slash", "taken_pct": 50}, {"kind": "meleekind", "melee_kind": "pierce", "taken_pct": 50}}
	for _, id := range []int{905145, 905146, 905147, 905148} {
		inno = append(inno, map[string]any{"kind": "nuke", "spell_id": id, "every": 20})
	}
	exec1(tmpl, "innoruuk", 2646, 1200, 0, 0.0, 10, 500, "Hate", "AR", js(inno))
	exec1(tmpl, "metal moogie", 347, 750, 750, 6.0, 25, 500, "Static Discharge", "LR", `[{"kind":"lifetap","spell_id":9100,"every":30,"first":30}]`)
	exec1(tmpl, "pet swarming death", 100, 100, 0, 0.0, 25, 250, "Swarming Sting", "AR", `[]`)
	exec1(`INSERT INTO petstats (spellid, name, abilities) VALUES (700823, 'Red Swarming Death', ?)`,
		`[{"cast":0.5,"every":24,"first":1,"kind":"dot","spell_id":905100},{"every":50,"first":3,"kind":"nuke","spell_id":905102},`+
			`{"every":30,"first":5,"kind":"lifetap","spell_id":905101}]`)

	run := func(args ...string) string {
		t.Helper()
		out, err := exec.Command(py, append([]string{tool, "--db", path}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("the tool %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	if out := run("--apply"); !strings.Contains(out, "applied: 10 rows") { // V2 has no npcitem row to copy here
		t.Fatalf("the tool's apply: %s", out)
	}
	w := NewWorld()
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	for _, p := range w.ClassTemplateProblems() {
		for _, name := range []string{"mr moogie", "tunare", "innoruuk", "metal moogie v2"} {
			if strings.Contains(p, fmt.Sprintf("%q", name)) {
				t.Errorf("a problem of the tool's: %s", p)
			}
		}
	}
	m, v2, in, tu := w.templates.byName["mr moogie"], w.templates.byName["metal moogie v2"], w.templates.byName["innoruuk"], w.templates.byName["tunare"]
	if m == nil || v2 == nil || in == nil || tu == nil {
		t.Fatalf("templates read: moogie %v, v2 %v, innoruuk %v, tunare %v", m != nil, v2 != nil, in != nil, tu != nil)
	}
	if m.hitHeal != 150 || len(m.prefer) != 1 || m.prefer[0] != "tunare" || len(m.summons) != 1 || m.summons[0].Share != 50 ||
		m.enrage == nil || m.enrage.Grow != 200 || len(m.abilities) != 3 {
		t.Errorf("Mr. Moogie's: hit heal %d, prefer %v, summons %+v, enrage %+v, %d cast abilities", m.hitHeal, m.prefer, m.summons, m.enrage, len(m.abilities))
	}
	if tap := m.abilities[2]; tap.Damage != 4000 || tap.pinned == nil || tap.pinned.Name != "Royal Siphon" {
		t.Errorf("his Royal Siphon: %+v", tap.TemplateAbility)
	}
	if !v2.farCasts || v2.HPPct != 382 || v2.MeleePct != 825 || v2.KickDamage != 825 {
		t.Errorf("V2's: from afar %v, hp %d, melee %d, kick %d", v2.farCasts, v2.HPPct, v2.MeleePct, v2.KickDamage)
	}
	if len(in.procs) != 1 || in.procs[0].least != 650 || in.procs[0].resist != StatDR || len(in.veils) != 1 || in.veils[0].pct != 1 ||
		in.ProcName != "Swarming Sting" || len(in.kindTaken) != 2 {
		t.Errorf("Innoruuk's: procs %+v, veils %+v, his proc %q, melee kinds %v", in.procs, in.veils, in.ProcName, in.kindTaken)
	}
	if a := tu.abilities[0]; a.HealPct != 100 || a.Below != 20 || a.Cooldown != 1800 || a.share == nil || a.share.Name != "Tunare's Renewal" {
		t.Errorf("Tunare's first: %+v", a.TemplateAbility)
	}
	run("--undo")
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	if m := w.templates.byName["mr moogie"]; m == nil || m.hitHeal != 0 || m.enrage != nil || len(m.abilities) != 2 || w.templates.byName["metal moogie v2"] != nil {
		t.Errorf("after the undo: Mr. Moogie's %+v, V2's %v", m, w.templates.byName["metal moogie v2"] != nil)
	}
}
