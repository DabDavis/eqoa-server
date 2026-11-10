package handlers

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// Buffer on a real server's data (EQOA_REAL_DB, a database the petition 1390 data step was applied to; a copy is
// opened): his five records read as he lands them, every one with its visual and icon, and talked to by a level 60
// cleric they land a second later for five hours of play: Order's 650 AC and the GM Armor's 500 both counted, every
// resist 85 and a tenth of the cleric's WIS, Avatar's, Order's and Pure Heart's maximums. Then the GM Armor stacks
// with the self AC line (Steel Skin, stack 2) and stays through a druid's Rockskin (Order's stack type: nested under
// Order, its own maximum power counted beside it). Skipped without a database, or one without the GM Armor.
func TestRealBufferSpells(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "buffer.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := database.NewSpellbookStore(db)
	spells, missing := bufferSpells(store)
	if slices.Contains(missing, "GM Armor") {
		t.Skip("no GM Armor record: the petition 1390 data step is not applied to this database")
	}
	if len(missing) > 0 || len(spells) != len(BufferSpells) {
		t.Fatalf("his spells: %d read, missing %v", len(spells), missing)
	}
	for i, s := range spells {
		if s.SpellID != BufferSpells[i].ID || s.Name != BufferSpells[i].Name || s.Def.Duration <= 0 || s.FX == 0 || s.Icon == 0 {
			t.Errorf("spell %d: %d %q, lasting %v, visual %#x, icon %#x", i, s.SpellID, s.Name, s.Def.Duration, s.FX, s.Icon)
		}
	}

	r := newPortalRig(t)
	p := r.pa
	p.Level, p.Class, p.Wis, p.HP, p.MaxHP, p.Power, p.MaxPower = 60, 9, 200, 3000, 3000, 2000, 2000
	buf := &entity.NPC{ServerID: 7790, Name: entity.BufferName, X: 5001, Y: 50, Z: 5000, Level: 60, HP: 5000, MaxHP: 5000, NPCType: 0x80}
	r.gw.NPCs[buf.ServerID] = buf
	r.ih.buffer = NewBuffer(r.gw, store)
	r.ih.buffer.now = func() time.Time { return r.now }
	r.talk(r.anna, buf)
	if got := r.msgs(r.anna); countMsgs(got, "op:00B1") != 5 {
		t.Fatalf("his talk: %q", got)
	}
	r.gw.Tick(r.now.Add(time.Second))
	if len(p.Effects) != 5 {
		t.Fatalf("landed: %d effects", len(p.Effects))
	}
	for _, e := range p.Effects {
		if want := r.now.Add(time.Second + entity.BufferLasts); !e.Until.Equal(want) {
			t.Errorf("%s until %v, want %v", e.Name, e.Until, want)
		}
	}
	m := p.Mods
	if m.AC != 650+500 || m.FR != 85+20 || m.AR != 85+20 || m.Str != 150+20 || m.MaxHP != 1500+20+650+1200 || m.MaxPower != 1200 {
		t.Errorf("his five on a WIS 200 cleric: %+v", m)
	}
	t.Logf("his five: AC +%d, max HP +%d, max power +%d, STR STA AGI DEX +%d, resists +%d; lines %q", m.AC, m.MaxHP,
		m.MaxPower, m.Str, m.FR, func() []string {
			var out []string
			for _, e := range p.Effects {
				out = append(out, e.Line)
			}
			return out
		}())

	land := func(name string) {
		t.Helper()
		id, ok, err := store.SpellIDByName(name)
		if err != nil || !ok {
			t.Fatalf("%s: %v %v", name, ok, err)
		}
		sp, err := store.SpellPattern(id)
		if err != nil {
			t.Fatal(err)
		}
		row, _, err := store.SpellEffectRow(id)
		if err != nil {
			t.Fatal(err)
		}
		r.gw.LandOnSelf(r.anna, entity.SpellOfRow(sp, 0, row.Line, row.Effects, row.FX), r.now.Add(2*time.Second))
	}
	before := p.Mods.AC
	land("Steel Skin")
	if p.Mods.AC <= before || !hasEffect(p, BufferGMArmorID) || !hasEffect(p, 700805) {
		t.Errorf("Steel Skin beside his: AC +%d (was +%d), GM Armor %v, Order %v", p.Mods.AC, before, hasEffect(p, BufferGMArmorID),
			hasEffect(p, 700805))
	}
	ac := p.Mods.AC
	land("Rockskin")
	if !hasEffect(p, BufferGMArmorID) || !hasEffect(p, 700805) || p.Mods.AC != ac {
		t.Errorf("Rockskin under Order: AC +%d (was +%d), GM Armor %v, Order %v", p.Mods.AC, ac, hasEffect(p, BufferGMArmorID),
			hasEffect(p, 700805))
	}
}

func hasEffect(p *entity.Player, id int) bool {
	for _, e := range p.Effects {
		if e.SpellID == id {
			return true
		}
	}
	return false
}
