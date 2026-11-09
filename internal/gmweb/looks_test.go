package gmweb

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os/exec"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The look columns are the ones measured (looks.go): each column a picture,
// colour or palette kind, and the 32-bit ones a form to be written in, the
// one the code that reads the column needs.
func TestLookColumnsAreTheOnesMeasured(t *testing.T) {
	f := newFixture(t)
	want := map[string][2]string{
		"itempattern.itemicon":         {lookItemIcon, storeInt32},
		"itempattern.model":            {lookItemLook, storeInt32},
		"itempattern.color":            {lookItemColor, storeUint32},
		"spellpattern.icon":            {lookUIIcon, storeInt32},
		"spellpattern.iconColor":       {lookUIBack, storeInt32},
		"spelleffect.fxdict":           {lookVisual, storeUint32},
		"spellfx_suggestions.fxdict":   {lookVisual, storeUint32},
		"npcs.modelid":                 {lookChar, storeUint32},
		"npcs.primary_weapon":          {lookItem, storeInt32},
		"npcs.shield":                  {lookItem, storeInt32},
		"npcs.torso":                   {lookArmorChest, ""},
		"npcs.head":                    {lookArmorHelm, ""},
		"npcs.unk_o":                   {lookTint, ""},
		"npcs.robe_style":              {lookRobe, ""},
		"npcs.hair_color":              {lookHair, ""},
		"characters.modelid":           {lookChar, storeInt32},
		"chardyes.color":               {lookRGBA, storeUint32},
		"charlook.value":               {lookRaceLook, storeInt32},
		"itemeffect.icon":              {lookUIIcon, storeUint32},
		"itemeffect.body":              {lookChar, storeUint32},
		"racedefs.body_male":           {lookChar, storeInt32},
		"classtemplates.special_fx":    {lookVisual, storeUint32},
		"petstats.primary_model":       {lookItem, storeUint32},
		"charactermodel.modelid":       {lookChar, storeUint32},
		"charbuffs.icon":               {lookUIIcon, storeUint32},
		"spellpattern.spellname":       {"", ""},
		"itempattern.weapondamage":     {"", ""},
		"npcs.hair_length":             {"", ""},
		"characters.haircolor":         {lookHair, ""},
		"itempattern.patternid":        {"", ""},
		"chardyes.slot":                {"", ""},
		"petstats.model":               {lookChar, storeUint32},
		"npcs.secondary_weapon":        {lookItem, storeInt32},
		"racedefs.body_female":         {lookChar, storeInt32},
		"petstats.secondary_model":     {lookItem, storeUint32},
		"npcs.legs":                    {lookArmorLegs, ""},
		"npcs.feet":                    {lookArmorBoots, ""},
		"npcs.gloves":                  {lookArmorGloves, ""},
		"npcs.forearm":                 {lookArmorBracer, ""},
		"spellpattern.abilitylvl":      {"", ""},
		"itempattern.equipslot":        {"", ""},
		"characters.faceoption":        {"", ""},
		"npcs.face":                    {"", ""},
		"chardyes.serverid":            {"", ""},
		"classtemplates.special_every": {"", ""},
	}
	for key, w := range want {
		table, col, _ := strings.Cut(key, ".")
		tb, err := f.s.table(table)
		if err != nil {
			t.Errorf("%s: %v", key, err)
			continue
		}
		c := tb.col(col)
		if c == nil {
			t.Errorf("%s: no such column", key)
			continue
		}
		if c.Look != w[0] || c.Store != w[1] {
			t.Errorf("%s: look %q store %q, want %q %q", key, c.Look, c.Store, w[0], w[1])
		}
	}
	// every look the catalog names is one the page draws, and every 32-bit
	// column with a look says which form it is written in
	page, err := fs.ReadFile(staticFS, "static/lib/look.js")
	if err != nil {
		t.Fatal(err)
	}
	for table, n := range catalog {
		for col, cn := range n.Cols {
			if cn.Look == "" {
				if cn.Store != "" {
					t.Errorf("%s.%s: a store with no look", table, col)
				}
				continue
			}
			kind := cn.Look
			if strings.HasPrefix(kind, "armor:") {
				kind = "armor:"
			}
			if !strings.Contains(string(page), "'"+kind+"'") {
				t.Errorf("%s.%s: look %q, which lib/look.js does not draw", table, col, cn.Look)
			}
			if cn.Format == hex32 && cn.Store != storeInt32 && cn.Store != storeUint32 {
				t.Errorf("%s.%s: a 32-bit look with store %q", table, col, cn.Store)
			}
		}
	}
	// the control: a column the catalog leaves alone carries no look
	if c, _ := f.s.table("accountinfo"); c.col("username").Look != "" {
		t.Errorf("accountinfo.username has a look")
	}
}

// describe_table hands the palettes a table's looks draw: the disc tints by
// name with their colours, and the dyers' colours beside a colour.
func TestDescribeHandsTheLooksPalettes(t *testing.T) {
	f := newFixture(t)
	gm := f.browser()
	gm.login("gm")
	get := func(table string) map[string]any {
		code, out := gm.op("describe_table", map[string]any{"table": table})
		if code != 200 {
			t.Fatalf("describe %s: %d %v", table, code, out)
		}
		return out
	}
	items := get("itempattern")
	pal, ok := items["palettes"].(map[string]any)
	if !ok {
		t.Fatalf("itempattern: no palettes: %v", items["palettes"])
	}
	tints, _ := pal["tints"].([]any)
	if len(tints) != len(database.DiscTints) {
		t.Fatalf("tints: %d", len(tints))
	}
	for i, x := range tints {
		e := x.(map[string]any)
		want := database.FormatDye(database.RGBA(database.DiscTints[i]))
		if e["rgba"] != want || e["value"] != float64(i) || e["label"] != tintNames[i] {
			t.Errorf("tint %d: %v, want %s %s", i, e, want, tintNames[i])
		}
	}
	if e := tints[13].(map[string]any); e["rgba"] != "96003CFF" || e["label"] != "Burgundy" {
		t.Errorf("tint 13: %v", e)
	}
	dyes, _ := pal["dyes"].([]any)
	if len(dyes) != len(database.DyeFamilies) {
		t.Errorf("dye families: %d", len(dyes))
	}
	if fam := dyes[5].(map[string]any); !strings.Contains(fmt.Sprint(fam["colours"]), "400080FF") {
		t.Errorf("the purples: %v", fam)
	}
	// a tint index alone: the tints, no dyes
	npcs := get("npcs")
	if p, _ := npcs["palettes"].(map[string]any); p["tints"] == nil || p["dyes"] != nil {
		t.Errorf("npcs' palettes: %v", npcs["palettes"])
	}
	// the control: a table with no colour has none
	if get("accountinfo")["palettes"] != nil {
		t.Errorf("accountinfo has palettes")
	}
	// a column's look and store reach the page
	for _, x := range items["columns"].([]any) {
		c := x.(map[string]any)
		if c["name"] == "itemicon" && (c["look"] != lookItemIcon || c["store"] != storeInt32) {
			t.Errorf("itemicon: %v", c)
		}
	}
}

// look_names tells what wears each id, one id whichever way a table keeps it,
// the commonest names first; a tester's from its own tables alone.
func TestLookNamesTellsWhatWearsEachID(t *testing.T) {
	f := newFixture(t)
	f.newTester()
	// the same icon kept signed on one item and unsigned on another, a third
	// item on its own, and NPCs holding the sword's model
	mustExec(t, f.db, `UPDATE itempattern SET itemicon = -1269925226, model = -1817771540, equipslot = 12 WHERE patternid = 10`)
	mustExec(t, f.db, `UPDATE itempattern SET itemicon = 3025042070 WHERE patternid = 11`)
	mustExec(t, f.db, `UPDATE npcs SET primary_weapon = 2477195756 WHERE id IN (500, 501)`)
	mustExec(t, f.db, `INSERT INTO itemeffect (patternid, line, name, icon, body) VALUES (11, 'Self, Increase HP by 5', 'Bread', 4, 1893243078)`)
	gm := f.browser()
	gm.login("gm")
	names := func(c *client, kind string) map[uint32]lookUse {
		t.Helper()
		code, out := c.op("look_names", map[string]any{"kind": kind})
		if code != 200 {
			t.Fatalf("look_names %s: %d %v", kind, code, out)
		}
		b, _ := json.Marshal(out["ids"])
		var list []lookUse
		json.Unmarshal(b, &list)
		m := map[uint32]lookUse{}
		for _, u := range list {
			m[u.ID] = u
		}
		return m
	}
	icons := names(gm, "itemicon")
	u := icons[0xB44E7A96]
	if u.Rows != 2 || u.Hex != "0xB44E7A96" || strings.Join(u.Names, "|") != "Bread|Rusty Sword" {
		t.Errorf("the icon kept both ways: %+v (all: %v)", u, icons)
	}
	// the sword's model: the item, and the NPCs holding it, the commonest first
	models := names(gm, "item")
	if m := models[0x93A701EC]; m.Rows != 3 || len(m.Names) != 3 || m.Names[0] != "Guard Dahvel (held)" && m.Names[0] != "Merchant Shadehand (held)" {
		t.Errorf("the sword's model: %+v", m)
	}
	bodies := names(gm, "char")
	if b := bodies[1893243078]; b.Rows != 4 || !strings.Contains(strings.Join(b.Names, "|"), "Guard Dahvel") {
		t.Errorf("the guards' body: %+v", b)
	}
	// a tester: its own tables (items, their effects), never an NPC's name
	tess := f.browser()
	tess.login("tess")
	if b := names(tess, "char")[1893243078]; b.Rows != 1 || strings.Join(b.Names, "|") != "Bread" {
		t.Errorf("a tester's bodies: %+v", b)
	}
	if m := names(tess, "item")[0x93A701EC]; m.Rows != 1 || strings.Join(m.Names, "|") != "Rusty Sword" {
		t.Errorf("a tester's models: %+v", m)
	}
	// a storyteller (who reads the NPCs, not the items): the NPCs' bodies, no item
	f.newStoryteller()
	sage := f.browser()
	sage.login("sage")
	if b := names(sage, "char")[1893243078]; b.Rows != 3 || strings.Contains(strings.Join(b.Names, "|"), "Bread") {
		t.Errorf("a storyteller's bodies: %+v", b)
	}
	if n := len(names(sage, "itemicon")); n != 0 {
		t.Errorf("a storyteller was told %d item icons", n)
	}
	// the control: an unknown kind is refused
	if code, out := gm.op("look_names", map[string]any{"kind": "faces"}); code != 400 {
		t.Errorf("an unknown kind: %d %v", code, out)
	}
}

// The page in a real browser, when one is installed, with no disc: a look
// field takes an id in hex or decimal and writes the column's own form (an
// item's icon signed, its model signed, its colour unsigned), a colour by
// a dyer's swatch, an NPC's armour by its select and its tint by a swatch;
// each save is the audited row edit; the page's tint for a colour is the
// server's (entity.ArmorTint).
func TestTheLookFieldsWriteTheColumnsForm(t *testing.T) {
	var browser string
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"} {
		if p, err := exec.LookPath(name); err == nil {
			browser = p
			break
		}
	}
	if browser == "" {
		t.Skip("no Chromium to load the page in")
	}
	f := newFixture(t)
	colours := []uint32{0x101010FF, 0xD10000FF, 0x400080FF, 0x00008CFF, 0x808080FF, 0xFAAF32FF, 0x5B6B3CFF, 0x00C8B4FF, 0xEBCDA5FF}
	var want []int
	for _, c := range colours {
		want = append(want, int(entity.ArmorTint(c)))
	}
	cj, _ := json.Marshal(colours)
	wj, _ := json.Marshal(want)
	script := `
    const L = await import('/static/lib/look.js');
    same([L.parseId('0xB44E7A96'), L.parseId('3025042070'), L.parseId('-1269925226'), L.parseId(' 0x1a '), Number.isNaN(L.parseId('0x1FFFFFFFF')), Number.isNaN(L.parseId('sword'))],
      [3025042070, 3025042070, -1269925226, 26, true, true]);
    same([L.stored({store: 'int32'}, 3025042070), L.stored({store: 'uint32'}, -1269925226), L.hex8(-1269925226)], [-1269925226, 3025042070, '0xB44E7A96']);
    same([L.parseColour('#ff0000'), L.parseColour('400080FF'), L.parseColour('0x00008CFF'), Number.isNaN(L.parseColour('red'))], [0xFF0000FF, 0x400080FF, 0x00008CFF, true]);
    const meta = await (await import('/static/lib/meta.js')).describe('itempattern');
    same(` + string(cj) + `.map((c) => L.nearestTint(c, meta.palettes.tints)), ` + string(wj) + `);
    // an item: its icon in hex, its model in decimal, a dyer's purple
    await open('#/items/10', 'Item 10');
    const set = (label, v) => { const i = field('input', label)[0]; if (!i) throw new Error('no field ' + label); i.value = v; i.dispatchEvent(new Event('input')); i.dispatchEvent(new Event('blur')); };
    set('Icon', '0xB44E7A96');
    set('Model', '3025042070');
    if (field('input', 'Model')[0].value !== '0xB44E7A96') throw new Error('the decimal model was not shown in hex: ' + field('input', 'Model')[0].value);
    [...document.querySelectorAll('main button')].find((b) => b.textContent.trim() === 'Colour').click();
    await wait(() => document.querySelector('main button[aria-label="Purple (400080FF)"]'), 'the dyers\' colours');
    document.querySelector('main button[aria-label="Purple (400080FF)"]').click();
    if (!text().includes('drawn as tint')) throw new Error('no PS2 tint for the colour: ' + text().slice(0, 300));
    await wait(() => buttons().includes('Save'), 'Save');
    [...document.querySelectorAll('main button')].find((b) => b.textContent.trim() === 'Save').click();
    await wait(() => document.body.textContent.includes('Saved'), 'Saved');
    // an NPC: chest set 3 by its select, tint 6 by its swatch, a robe
    await open('#/npcs/500', 'NPC 500');
    const chest = field('select', 'Chest (by name)')[0];
    chest.value = '3'; chest.dispatchEvent(new Event('change'));
    document.querySelector('main button[aria-label="Tint 6: Gold"]').click();
    const robe = field('select', 'Robe (by name)')[0];
    robe.value = '2'; robe.dispatchEvent(new Event('change'));
    await wait(() => buttons().includes('Save'), 'Save');
    [...document.querySelectorAll('main button')].find((b) => b.textContent.trim() === 'Save').click();
    await wait(() => document.body.textContent.includes('Saved'), 'Saved');
`
	browserPage(t, f, browser, "gm", "", script)
	var icon, model, color int64
	if err := f.db.QueryRow(`SELECT itemicon, model, color FROM itempattern WHERE patternid = 10`).Scan(&icon, &model, &color); err != nil {
		t.Fatal(err)
	}
	if icon != -1269925226 || model != -1269925226 || color != 0x400080FF {
		t.Errorf("item 10: icon %d model %d colour %#x, want -1269925226 twice and 0x400080FF", icon, model, color)
	}
	var torso, tint int
	var robe string
	if err := f.db.QueryRow(`SELECT torso, unk_o, robe_style FROM npcs WHERE id = 500`).Scan(&torso, &tint, &robe); err != nil {
		t.Fatal(err)
	}
	if torso != 3 || tint != 6 || robe != "2" {
		t.Errorf("NPC 500: chest %d tint %d robe %q, want 3, 6, \"2\"", torso, tint, robe)
	}
	// each save is the audited row edit, revertible
	var n int
	f.db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl IN ('itempattern', 'npcs') AND action = 'update'`).Scan(&n)
	if n != 2 {
		t.Errorf("audit lines: %d, want 2", n)
	}
}
