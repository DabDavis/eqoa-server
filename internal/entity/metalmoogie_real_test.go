package entity

import (
	"math"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// Mr. Moogie's challenge on a real server's database with its data step applied (EQOA_REAL_DB names
// it; a copy is read; the step is the lane's metalmoogie_20261109.py, petitions 1379 and 1380): his
// metal twin's row stands out of the world, his template reads with no problem, a challenge from
// beside Mr. Moogie copies him with a GM's 50k hit points, and under the database's own balance his
// hits on a level 60 warrior of AC 2000 and AGI 400 land near 300, four a round; the Duelist's Favor
// reads as half again as big for an hour, the Moogie Tribute is NO TRADE and LORE, and his corpse holds
// one. Skipped without a database, or with one the step was not applied to.
func TestRealMetalMoogie(t *testing.T) {
	if os.Getenv("EQOA_REAL_DB") == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	data, err := os.ReadFile(os.Getenv("EQOA_REAL_DB"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "metal.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM npcs WHERE npc_name = ?`, MetalMoogieName).Scan(&n); err != nil || n == 0 {
		t.Skipf("no %s in this database (the data step not applied): %v", MetalMoogieName, err)
	}
	w := NewWorld()
	w.SessionManager = session.NewManager()
	w.LoadNPCs(database.NewNPCStore(db))
	if _, _, err := w.LoadSpawnOnly(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	for _, p := range w.ClassTemplateProblems() {
		if strings.Contains(p, "metal moogie") {
			t.Errorf("his template's problem: %s", p)
		}
	}
	if _, _, err := w.LoadBalance(db); err != nil {
		t.Fatal(err)
	}
	var moogie *NPC
	for _, npc := range w.NPCs {
		if npc.Name == MoogieName && !npc.Temporary {
			moogie = npc
		}
	}
	if moogie == nil {
		t.Fatal("no Mr. Moogie")
	}
	if !w.MetalMoogieHere() {
		t.Fatal("no spawn-only Metal Moogie")
	}
	sess := w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 7, 1), Port: 7001}, 1, 1, 1, 2, true)
	p := &Player{ServerID: 999001, CharServerID: 999001, Name: "Tester", Class: 0, Level: 60, HP: 5000, MaxHP: 5000,
		X: moogie.X + 3, Y: moogie.Y, Z: moogie.Z, World: moogie.World, Size: PlayerSize, GMTankProfile: 2}
	w.Players[p.ServerID] = p
	w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	line, ok := w.MetalMoogieChallenge(sess, moogie.ServerID, time.Now())
	if !ok {
		t.Fatalf("no challenge: %q", line)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	c := w.NPCs[w.metal.npc]
	if c.MaxHP < 49000 || c.MaxHP > 51000 || c.Level != 60 || c.TemplateName() != "metal moogie" {
		t.Errorf("he rose with %d hit points at level %d on %q", c.MaxHP, c.Level, c.TemplateName())
	}
	if d := dist3(c.X, c.Y, c.Z, moogie.X, moogie.Y, moogie.Z); d > MetalRiseNear {
		t.Errorf("he rose %.1f from Mr. Moogie", d)
	}
	if _, _, _, hits := npcRound(c); len(hits) != 4 {
		t.Errorf("%d hits a round", len(hits))
	}
	sum, lo, hi := 0, math.MaxInt, 0
	const rolls = 1000
	for i := 0; i < rolls; i++ {
		k := i
		hit := w.npcHitPlayerLocked(c, p, npcMeleeOn(c, p, func(m int) int { return k * m / rolls }))
		sum += hit
		lo, hi = min(lo, hit), max(hi, hit)
	}
	if mean := float64(sum) / rolls; mean < 270 || mean > 330 {
		t.Errorf("his hits on the 2,000 AC 400 AGI warrior: mean %.1f (%d-%d), want about 300", mean, lo, hi)
	} else {
		t.Logf("%d hit points; on the 2,000 AC 400 AGI level 60 warrior %.1f a hit (%d-%d), four a round", c.MaxHP, mean, lo, hi)
	}
	t.Logf("%s", w.templateLineLocked(c, time.Now()))

	var line2 string
	var fx int64
	if err := db.QueryRow(`SELECT e.line, e.fxdict FROM spellpattern p JOIN spelleffect e ON e.spellid = p.spellid WHERE p.spellname = ?`,
		DuelistsFavorName).Scan(&line2, &fx); err != nil {
		t.Fatalf("the favor's record: %v", err)
	}
	if d := ParseSpellNamed(DuelistsFavorName, line2, ""); len(d.Effects) != 1 || d.Effects[0].Kind != EffectSize || d.Effects[0].Amount != 50 ||
		d.Duration != time.Hour {
		t.Errorf("the favor reads %+v", d)
	}
	var trade, lore, held int
	if err := db.QueryRow(`SELECT trade, lore FROM itempattern WHERE itemname = ?`, MoogieTributeName).Scan(&trade, &lore); err != nil || trade != 1 || lore != 1 {
		t.Errorf("the tribute: trade %d, lore %d, %v", trade, lore, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM npcitem i JOIN itempattern p ON p.patternid = i.patternid WHERE i.npcid = ? AND p.itemname = ?`,
		c.NPCID, MoogieTributeName).Scan(&held); err != nil || held != 1 {
		t.Errorf("%d tributes on his corpse (%v), want one", held, err)
	}
}

// The Metal Moogie chain on a real server's database with tools/moogie_chain_688_20261115.py applied
// (EQOA_REAL_DB; a copy is read; work board #688): the chain's templates read with no problem, V3 to V5 and
// the gearling are spawn-only, V5 rises from beside Mr. Moogie on his template with V2's hit points, his
// gearlings below 25% at 5,040 on a fixed 25 a hit with the root pinned to Root's look, each version's corpse
// holds its part (V2's no tribute), the three uses read as the code's, and the Winding Stone has its familiar.
// Skipped without a database, or one the tool was not applied to.
func TestRealMetalMoogieChain(t *testing.T) {
	if os.Getenv("EQOA_REAL_DB") == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	data, err := os.ReadFile(os.Getenv("EQOA_REAL_DB"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "chain.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM npcs WHERE npc_name = ?`, MetalMoogieV5Name).Scan(&n); err != nil || n == 0 {
		t.Skipf("no %s in this database (the tool not applied): %v", MetalMoogieV5Name, err)
	}
	_, bad, err := db.ClassTemplates()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bad {
		if strings.Contains(b, "moogie") {
			t.Errorf("a chain template left out: %s", b)
		}
	}
	w := NewWorld()
	w.SessionManager = session.NewManager()
	w.LoadNPCs(database.NewNPCStore(db))
	if _, _, err := w.LoadSpawnOnly(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	for _, p := range w.ClassTemplateProblems() {
		if strings.Contains(p, "moogie") {
			t.Errorf("a template's problem: %s", p)
		}
	}
	if _, _, err := w.LoadBalance(db); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{MetalMoogieV2Name, MetalMoogieV3Name, MetalMoogieV4Name, MetalMoogieV5Name} {
		if !w.MetalMoogieVersionHere(v) {
			t.Errorf("%s not spawn-only here", v)
		}
	}
	var moogie *NPC
	for _, npc := range w.NPCs {
		if npc.Name == MoogieName && !npc.Temporary {
			moogie = npc
		}
	}
	if moogie == nil {
		t.Fatal("no Mr. Moogie")
	}
	sess := w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 7, 2), Port: 7002}, 1, 1, 1, 2, true)
	p := &Player{ServerID: 999002, CharServerID: 999002, Name: "Tester", Class: 0, Level: 60, HP: 5000, MaxHP: 5000,
		X: moogie.X + 3, Y: moogie.Y, Z: moogie.Z, World: moogie.World, Size: PlayerSize}
	w.Players[p.ServerID] = p
	w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	line, ok := w.MetalMoogieVersionChallenge(sess, moogie.ServerID, MetalMoogieV5Name, time.Now())
	if !ok {
		t.Fatalf("no V5: %q", line)
	}
	w.mu.Lock()
	c := w.NPCs[w.metal.npc]
	v2 := w.npcByNameLocked(MetalMoogieV2Name)
	if c.TemplateName() != "metal moogie v5" || c.MaxHP < 54000 || c.MaxHP > 56000 || c.Size != v2.Size || c.Level != 60 {
		t.Errorf("V5 rose: template %q, %d hit points, size %v (V2's %v), level %d", c.TemplateName(), c.MaxHP, c.Size, v2.Size, c.Level)
	}
	t.Logf("%s", w.templateLineLocked(c, time.Now()))
	w.metal.c.held = false
	c.CurrentEnemy = p.ServerID
	c.HP = c.MaxHP / 5
	w.templatePhasesLocked(c, time.Now())
	var gs []*NPC
	for _, a := range w.adds {
		if a.add.by == c {
			gs = append(gs, a)
		}
	}
	if len(gs) != 2 {
		t.Fatalf("%d gearlings", len(gs))
	}
	g := gs[0]
	if g.MaxHP != 5040 || g.Size != 4.5 || g.meleeFixed() != 25 || g.challenge != c.challenge {
		t.Errorf("a gearling: %d hit points, size %v, fixed %d", g.MaxHP, g.Size, g.meleeFixed())
	}
	var root *templateAbility
	for i := range g.tmpl.abilities {
		if g.tmpl.abilities[i].Kind == "root" {
			root = &g.tmpl.abilities[i]
		}
	}
	if root == nil || root.pinned == nil || root.pinned.SpellID != 460 || root.pinned.visual() != 3479902439 {
		t.Errorf("the gearling's root: %+v", root)
	}
	t.Logf("%s", w.templateLineLocked(g, time.Now()))
	w.mu.Unlock()
	for who, part := range map[string]string{MetalMoogieV2Name: MetalLegsName, MetalMoogieV3Name: MetalTorsoName,
		MetalMoogieV4Name: MetalArmsName, MetalMoogieV5Name: MetalHeadName} {
		var parts, others int
		if err := db.QueryRow(`SELECT count(CASE WHEN p.itemname = ? THEN 1 END), count(CASE WHEN p.itemname != ? THEN 1 END)
			FROM npcitem i JOIN itempattern p ON p.patternid = i.patternid JOIN npcs n ON n.id = i.npcid WHERE n.npc_name = ?`,
			part, part, who).Scan(&parts, &others); err != nil || parts != 1 || others != 0 {
			t.Errorf("%s's corpse: %d %s, %d others (%v)", who, parts, part, others, err)
		}
	}
	effects, err := database.NewInventoryStore(db).ItemEffects()
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	for _, r := range effects {
		pe := NewPotionEffect(r.PatternID, r.ItemName, r.Line, r.Name, r.Icon, r.Breaks, r.Wear, r.Area)
		if pe.Special != "" {
			found[r.ItemName] = pe.Special
			if r.ItemName == "Snowball" && (pe.SpecialFX != SnowballFX || r.Cooldown != 1) {
				t.Errorf("the snowball's row: fx %#x, cooldown %v", pe.SpecialFX, r.Cooldown)
			}
			if r.ItemName == "Deputy's Draught" && (pe.SpecialSpell != DeputySpellID || pe.SpecialRadius != 30 || r.Charges != 1) {
				t.Errorf("the draught's row: %+v", r)
			}
		}
	}
	if found["Snowball"] != PotionSnowball || found["Potion of Moogie's Insight"] != PotionInsight || found["Deputy's Draught"] != PotionDeputy {
		t.Errorf("the uses read: %v", found)
	}
	var fam string
	var heal int
	var size float64
	if err := db.QueryRow(`SELECT f.name, f.heal, f.size FROM familiars f JOIN itempattern p ON p.patternid = f.patternid
		WHERE p.itemname = 'Moogie Winding Stone'`).Scan(&fam, &heal, &size); err != nil || fam != "Mini Metal Moogie" || heal != 0 || size != 2 {
		t.Errorf("the stone's familiar: %q heal %d size %v (%v)", fam, heal, size, err)
	}
}
