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

// The spells the test lanes' report of 2026-10-29 found broken, cast on a
// real server's records (EQOA_REAL_DB names its database, a copy of which is
// read) after tools/fix_tested_spells.py, each from its record and line as
// the cast handler builds it (handlers/cast.go's castSpell) and landed:
//   - B01-B04 and their kin: cast at a creature, the creature takes it (it
//     fights its caster) and the caster takes nothing;
//   - B05-B11: a pet for each summoning, the caster with none before;
//   - B12, B13: the evacuations land at Melton and Highpass, not the
//     caster's bind;
//   - and Poison Ward, Holy Aid and Bountiful Gather Power do what their
//     pages say.
//
// Skipped without a database, and on one the tool has not fixed: none is in
// this repository.
func TestRealTestedSpellsLand(t *testing.T) {
	if os.Getenv("EQOA_REAL_DB") == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	data, err := os.ReadFile(os.Getenv("EQOA_REAL_DB"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "tested.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var logged int
	if db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'tested_spells_fix_log'`).Scan(&logged); logged == 0 {
		t.Skip("tools/fix_tested_spells.py has not fixed this database")
	}
	w := realWorld(t)
	if _, err := w.LoadSpellGates(db); err != nil {
		t.Fatal(err)
	}
	var sent []GateTravel
	w.OnTravel(func(tr GateTravel) { sent = append(sent, tr) })
	store := database.NewSpellbookStore(db)
	byName := func(name string, class int) CastSpell {
		t.Helper()
		var id int
		if err := db.QueryRow(`SELECT p.spellid FROM spellpattern p JOIN classspells c ON c.spellid = p.spellid
			WHERE c.class = ? AND p.spellname = ?`, class, name).Scan(&id); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		sp, err := store.SpellPattern(id)
		if err != nil {
			t.Fatal(err)
		}
		line, fx, _, _ := store.SpellEffect(id)
		def := ParseSpellNamed(sp.SpellName, line, sp.SpellDesc)
		return CastSpell{SpellID: sp.SpellID, DisplayIndex: 1, Level: sp.AbilityLvl, Scope: sp.Scope, Name: sp.SpellName,
			Icon: uint32(sp.Icon), CastTime: time.Duration(sp.CastTime) * time.Second, Recast: time.Duration(sp.Recast) * time.Second,
			PowerCost: sp.Power, Range: math.Float32frombits(uint32(uint16(sp.Range)) << 16), Def: def, FX: fx,
			Area: ReadSpellArea(sp.SpellName, line, def)}
	}

	mgr := session.NewManager()
	w.SessionManager = mgr
	t0 := time.Unix(1_800_000_000, 0)
	port := 7300
	// caster is a level 60 of the class, alone in an empty corner of
	// Tunaria, with a creature beside it on its first channel
	caster := func(class int) (*Player, *session.Session, *NPC, uint32) {
		port++
		s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 2, 1), Port: port}, uint32(port), 1, 1, 2, true)
		p := &Player{ServerID: EntityID(90000 + port), Name: "Tester", Class: class, Level: 60, HP: 9000, MaxHP: 9000,
			Power: 9000, MaxPower: 9000, Speed: 8.833333, X: 100, Y: 50, Z: 100, World: 0}
		p.BaseStr, p.BaseSta, p.BaseDex, p.BaseCha = 200, 200, 200, 200
		w.mu.Lock()
		w.Players[p.ServerID] = p
		npc := &NPC{ServerID: EntityID(95000 + port), Name: "test creature", Level: 30, HP: 60000, MaxHP: 60000,
			ModelID: 0x55432169, Size: 1, X: 102, Y: 50, Z: 100, World: 0}
		w.NPCs[npc.ServerID] = npc
		w.mu.Unlock()
		s.SetECSEntity(p)
		s.SetCharacterInWorld(true)
		s.SetPlayerObjectID(0x10000000 + uint32(port))
		vs := GetOrCreateVisibility(s)
		vs.ChannelEntity[0], vs.ChannelObjectID[0] = npc.ServerID, 0x01000000+uint32(port)
		return p, s, npc, vs.ChannelObjectID[0]
	}
	at := t0
	land := func(s *session.Session, spell CastSpell, target uint32) bool {
		t.Helper()
		at = at.Add(10 * time.Minute) // past every lockout
		if !w.StartCast(s, spell, target, at) {
			return false
		}
		w.mu.Lock()
		w.castSystem(at.Add(spell.CastTime + castLand))
		w.mu.Unlock()
		w.runTravels()
		return true
	}
	hasEffect := func(effects []*ActiveEffect, id int) *ActiveEffect {
		for _, ae := range effects {
			if ae.SpellID == id {
				return ae
			}
		}
		return nil
	}

	// cast at a creature
	for _, c := range []struct {
		name  string
		class int
	}{
		{"Ember Strike", 4}, {"Pouncing Tiger", 4}, {"Annihilate", 13}, {"Arcane Bonds", 13},
		{"Plaguebringer's Rash", 11}, {"Shock of Poison", 11}, {"Lava Shock", 10}, {"Whirling Blades", 1},
		{"Inciting Touch", 3},
	} {
		p, s, npc, id := caster(c.class)
		spell := byName(c.name, c.class)
		if !land(s, spell, id) {
			t.Errorf("%s at a creature: refused", c.name)
			continue
		}
		w.mu.RLock()
		_, fights := npc.Threats[p.ServerID]
		mine := hasEffect(p.Effects, spell.SpellID)
		w.mu.RUnlock()
		if !fights || mine != nil || p.HP != p.MaxHP {
			t.Errorf("%s: the creature fights its caster %v, the caster's effect %v, its HP %d of %d",
				c.name, fights, mine != nil, p.HP, p.MaxHP)
		}
	}
	// the charms take the creature: charmed, or (resisted) fighting its caster
	for _, c := range []struct {
		name  string
		class int
	}{{"Beguile", 12}, {"Charm Animal", 7}} {
		p, s, npc, id := caster(c.class)
		npc.Level = 20 // within both charms' level
		if !land(s, byName(c.name, c.class), id) {
			t.Errorf("%s at a creature: refused", c.name)
			continue
		}
		w.mu.RLock()
		_, fights := npc.Threats[p.ServerID]
		w.mu.RUnlock()
		if w.PetOf(p) != npc.ServerID && !fights {
			t.Errorf("%s: the creature is neither charmed nor fighting its caster", c.name)
		}
	}

	// a pet for each summoning
	for _, c := range []struct {
		name  string
		class int
	}{
		{"Minor Familiar", 13}, {"Lesser Familiar", 13}, {"Arch Familiar", 13}, {"Lesser Guardian", 13},
		{"Guardian", 13}, {"Arch Guardian", 13}, {"Greater Guardian", 13}, {"Elder Guardian", 13},
		{"Wilting Flesh", 11}, {"Decaying Flesh", 11},
	} {
		p, s, _, _ := caster(c.class)
		if !land(s, byName(c.name, c.class), 0xFFFFFFFF) || w.PetOf(p) == 0 {
			t.Errorf("%s: no pet", c.name)
		}
	}

	// the evacuations land where their lines say
	for _, c := range []struct{ name, place string }{{"Melton Evacuation", "Melton"}, {"Highpass Evacuation", "Highpass"}} {
		_, s, _, _ := caster(13)
		sent = nil
		if !land(s, byName(c.name, 13), 0xFFFFFFFF) || len(sent) != 1 || sent[0].Home != 0 ||
			!strings.HasPrefix(sent[0].To.Place, c.place) {
			t.Errorf("%s: sent %+v", c.name, sent)
		}
	}

	// the rest by their pages
	p, s, _, _ := caster(9)
	ward := byName("Poison Ward", 9)
	if !land(s, ward, s.PlayerObjectID()) {
		t.Error("Poison Ward on oneself: refused")
	} else if ae := hasEffect(p.Effects, ward.SpellID); ae == nil || ae.Mods.PR != 100 || ae.Mods.DR != 100 ||
		ae.Until.Sub(at) < 29*time.Minute {
		t.Errorf("Poison Ward: %+v", ae)
	}
	p, s, _, _ = caster(2)
	aid := byName("Holy Aid", 2)
	if !land(s, aid, s.PlayerObjectID()) {
		t.Error("Holy Aid on oneself: refused")
	} else if ae := hasEffect(p.Effects, aid.SpellID); ae == nil || ae.HPTick <= 0 || ae.Until.Sub(at) < 20*time.Second {
		t.Errorf("Holy Aid: %+v", ae)
	}
	p, s, _, _ = caster(13)
	p.Power = 100
	bounty := byName("Bountiful Gather Power", 13)
	if !land(s, bounty, 0xFFFFFFFF) || p.Power != 100-bounty.PowerCost+120 || bounty.Recast != 90*time.Second {
		t.Errorf("Bountiful Gather Power: power %d, recast %v", p.Power, bounty.Recast)
	}
}
