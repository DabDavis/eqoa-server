package handlers

import (
	"database/sql"
	"io"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/ground"
	"github.com/eqoa/server/internal/session"
)

// TestRealVallonZek runs tools/vallon_zek_20261118.py on a copy of a server database (EQOA_REAL_DB) and holds
// what it wrote to the server: every Zek scout rolls the rune at 10%, whose roll fires the Portal of Zek; the
// portal, Vallon's guards and the Rage of Zek are held out of the world, the scouts are level 60 and white-faced,
// Vallon wears plate and no helm; quest "Vallon Zek" is the rune's hand-in for twenty shards; a shard casts Zek's
// Portal, a self gate to the isle (with EQOA_GROUND_DIR, cast by a player at the campfire, it lands him on the
// isle's ground near its near side); Vallon's template summons two Rages of Zek at 15% and reads with no problem;
// the four triggers read with no problem; a second run writes nothing; --undo leaves every table as it was. It
// says what a held Sapphyra's Sword's look would drop. Run it on a copy of each shard before applying the tool.
func TestRealVallonZek(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	path := filepath.Join(t.TempDir(), "real.db")
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	in.Close()
	out.Close()
	db0, err := database.NewSQLite(path) // the server's own tables first (lootdroptrigger), as a start makes them
	if err != nil {
		t.Fatal(err)
	}
	db0.Close()
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	before := tableDigest(t, raw)
	raw.Close()
	t.Logf("apply:\n%s", vzTool(t, path, "--apply", "--no-backup"))
	if again := vzTool(t, path, "--apply", "--no-backup"); !strings.Contains(again, "nothing to do") {
		t.Errorf("a second run: %s", again)
	}

	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	inv := database.NewInventoryStore(db)
	id := func(name string) int {
		var pid int
		if err := db.QueryRow(`SELECT patternid FROM itempattern WHERE itemname = ? ORDER BY patternid LIMIT 1`, name).Scan(&pid); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return pid
	}
	rune, shard := id("Portal Rune of Zek"), id("Portal Shard of Zek")
	rolls, err := inv.NPCLootRolls("Zek scout", 60)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range rolls {
		if r.Table == "zek portal rune" {
			found = r.Chance == 10 && r.Rolls == 1 && len(r.Choices) == 1 && r.Choices[0].Item.Pattern.PatternID == rune
		}
	}
	if !found {
		t.Errorf("a Zek scout's rolls: %+v", rolls)
	}
	if trig, err := inv.LootDropTriggers([]int{12, rune}); err != nil || len(trig) != 1 || trig[0] != "Portal of Zek" {
		t.Errorf("the rune fires %v (%v)", trig, err)
	}
	var rent, trade, stack, lore int
	db.QueryRow(`SELECT rent, trade, maxstack, lore FROM itempattern WHERE patternid = ?`, shard).Scan(&rent, &trade, &stack, &lore)
	if rent != 0 || trade != 0 || stack != 20 || lore != 0 {
		t.Errorf("the shard: rent %d trade %d stack %d lore %d: want NO RENT, tradeable, 20, not LORE", rent, trade, stack, lore)
	}
	db.QueryRow(`SELECT rent FROM itempattern WHERE patternid = ?`, rune).Scan(&rent)
	if rent != 0 {
		t.Errorf("the rune's rent %d: want NO RENT", rent)
	}
	if is, ok, err := inv.ItemSpell(shard); err != nil || !ok || is.SpellID != 760380 {
		t.Errorf("the shard's spell: %+v %v %v", is, ok, err)
	}

	// the quest
	qs, err := database.NewQuestStore(db).LoadQuests()
	if err != nil {
		t.Fatal(err)
	}
	var q *database.Quest
	for i := range qs {
		if qs[i].Name == "Vallon Zek" && qs[i].Giver == "Portal of Zek" {
			q = &qs[i]
		}
	}
	if q == nil {
		t.Fatal("no quest Vallon Zek at the Portal of Zek")
	}
	if len(q.Steps) != 1 || q.Steps[0].Kind != database.StepGive || q.Steps[0].Target != "Portal of Zek" ||
		len(q.Steps[0].Items) != 1 || q.Steps[0].Items[0].PatternID != rune || len(q.Steps[0].RewardItems) != 1 ||
		q.Steps[0].RewardItems[0].PatternID != shard || q.Steps[0].RewardItems[0].Count != 20 {
		t.Errorf("the quest's steps: %+v", q.Steps)
	}

	// the triggers
	trigs, err := db.GMTriggers()
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tr := range trigs {
		switch tr.Name {
		case "Vallon Zek", "Guards of Vallon", "Portal of Zek", "Vallon Zek's Rest":
			names[tr.Name] = true
			if p := database.TriggerProblems(tr); len(p) > 0 {
				t.Errorf("trigger %q: %v", tr.Name, p)
			}
		}
	}
	if len(names) != 4 {
		t.Errorf("triggers: %v", names)
	}
	deaths, err := db.NPCDeathTriggers()
	if err != nil {
		t.Fatal(err)
	}
	if d := deaths["vallon zek"]; len(d) != 1 || d[0].Trigger != "Vallon Zek's Rest" || d[0].Chance != 100 {
		t.Errorf("Vallon's death fires %+v", d)
	}

	// the world's reading
	w := entity.NewWorld()
	w.SessionManager = session.NewManager()
	w.LoadNPCs(database.NewNPCStore(db))
	if _, _, err := w.LoadSpawnOnly(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	for _, p := range w.ClassTemplateProblems() {
		if strings.Contains(strings.ToLower(p), "vallon") || strings.Contains(p, "Rage of Zek") {
			t.Errorf("template: %s", p)
		}
	}
	var tmpl string
	db.QueryRow(`SELECT abilities FROM classtemplates WHERE name = 'vallon zek'`).Scan(&tmpl)
	if !strings.Contains(tmpl, `"kind":"summon"`) || !strings.Contains(tmpl, `"npc":"Rage of Zek"`) || !strings.Contains(tmpl, `"share":15`) {
		t.Errorf("Vallon's template: %s", tmpl)
	}
	for _, n := range w.NPCs {
		switch {
		case n.Name == "Portal of Zek", n.Name == "Rage of Zek", n.Name == "Guard of Vallon":
			if !n.SpawnOnly {
				t.Errorf("%s (%d) stands in the world", n.Name, n.ServerID)
			}
		case strings.EqualFold(n.Name, "Zek scout") && n.World == 0:
			if n.Level != 60 {
				t.Errorf("a Zek scout of level %d", n.Level)
			}
		}
	}
	var torso, feet, head int
	db.QueryRow(`SELECT torso, feet, head FROM npcs WHERE npc_name = 'Vallon Zek' AND world = 5`).Scan(&torso, &feet, &head)
	if torso != 4 || feet != 4 || head != 0 {
		t.Errorf("Vallon wears torso %d feet %d head %d: want plate, no helm", torso, feet, head)
	}
	if n, err := w.LoadSpellGates(db); err != nil || n == 0 {
		t.Fatalf("spell gates: %d %v", n, err)
	}
	g, ok := w.SpellGate("Zek's Portal")
	if !ok || g.Kind != database.GateSelf || g.World != 5 {
		t.Fatalf("Zek's Portal's gate: %+v %v", g, ok)
	}
	// Sapphyra's Sword: in his hand, his corpse's held drop (npcheld) the very record, out of his table
	sword := id("Sapphyra's Sword")
	var model, held int64
	db.QueryRow(`SELECT model FROM itempattern WHERE patternid = ?`, sword).Scan(&model)
	db.QueryRow(`SELECT primary_weapon FROM npcs WHERE npc_name = 'Vallon Zek' AND world = 5`).Scan(&held)
	if held != model || model == 0 {
		t.Errorf("Vallon holds %d, the sword's look is %d", held, model)
	}
	hd, err := inv.HeldDrops()
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := hd.Named("Vallon Zek", database.NPCHeldPrimary); !ok || e.Pattern.PatternID != sword {
		t.Errorf("his held drop: %+v %v, want Sapphyra's Sword (%d)", e.Pattern.ItemName, ok, sword)
	}
	if e, ok := hd.Pick(uint32(model), false, 68, "Vallon Zek"); ok {
		t.Logf("the look alone would drop %q (%d): npcheld names the sword", e.Pattern.ItemName, e.Pattern.PatternID)
	}
	vr, err := inv.NPCLootRolls("Vallon Zek", 68)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range vr {
		if r.Table != "vallon zek" {
			continue
		}
		if len(r.Choices) != 11 || r.Rolls != 3 {
			t.Errorf("his table: %d items, %d rolls; want 11 and 3", len(r.Choices), r.Rolls)
		}
		for _, c := range r.Choices {
			if c.Item.Pattern.PatternID == sword {
				t.Error("the sword is still in his table")
			}
		}
	}

	// the shard's gate on the ground, cast at the campfire
	if dir := os.Getenv("EQOA_GROUND_DIR"); dir != "" {
		gr, err := ground.Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer gr.Close()
		w.SetGround(gr)
		sess := w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(203, 0, 113, 9), Port: 5009}, 9, 9, 9, 1, true)
		p := w.CreatePlayer(sess, &database.CharacterRecord{ServerID: 9, CharName: "Raider", X: 15310, Y: 60, Z: 18890, Level: 60})
		if p == nil {
			t.Fatal("no player")
		}
		sess.SetCharacterInWorld(true)
		p.HP, p.MaxHP = 5000, 5000
		var sent []entity.GateTravel
		w.OnTravel(func(tr entity.GateTravel) { sent = append(sent, tr) })
		spells := database.NewSpellbookStore(db)
		sp, err := spells.SpellPattern(760380)
		if err != nil {
			t.Fatal(err)
		}
		fx, _, err := spells.SpellEffectRow(760380)
		if err != nil {
			t.Fatal(err)
		}
		cs := castSpellRow(&database.CharSpellEntry{Pattern: *sp}, fx)
		cs.PowerCost, cs.ItemCast = 0, true
		now := time.Now()
		if !w.StartCast(sess, cs, 0xFFFFFFFF, now) {
			t.Fatal("the shard's cast did not begin")
		}
		for i := 1; i <= 40 && len(sent) == 0; i++ {
			w.Tick(now.Add(time.Duration(i) * 100 * time.Millisecond))
		}
		if len(sent) != 1 {
			t.Fatalf("the gate sent %d", len(sent))
		}
		to := sent[0].To
		d := math.Hypot(float64(to.X-4377.8), float64(to.Z-7052.8))
		t.Logf("the shard lands Raider at (%.1f, %.1f, %.1f) world %d, %.1f from the place, %.1f from Vallon's row",
			to.X, to.Y, to.Z, to.World, d, math.Hypot(float64(to.X-4372), float64(to.Z-7105)))
		if to.World != 5 || d > 40 || math.Abs(float64(to.Y-253.5)) > 15 {
			t.Errorf("the shard's landing (%.1f, %.1f, %.1f) world %d is off the isle's near side", to.X, to.Y, to.Z, to.World)
		}
	}
	db.Close()

	t.Logf("undo:\n%s", vzTool(t, path, "--undo", "--no-backup"))
	raw, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	after := tableDigest(t, raw)
	for tbl, h := range before {
		if after[tbl] != h && tbl != "gm_audit" && tbl != "sqlite_sequence" {
			t.Errorf("table %s differs after --undo", tbl)
		}
	}
	for tbl := range after {
		if _, ok := before[tbl]; !ok {
			t.Errorf("table %s left after --undo", tbl)
		}
	}
}

// vzTool runs tools/vallon_zek_20261118.py on the database.
func vzTool(t *testing.T, path string, args ...string) string {
	t.Helper()
	cmd := exec.Command("python3", append([]string{"-E", "-s", "../../tools/vallon_zek_20261118.py", "--db", path}, args...)...)
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the tool %v: %v\n%s", args, err, b)
	}
	return string(b)
}
