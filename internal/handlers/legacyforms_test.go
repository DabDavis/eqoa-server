package handlers

import (
	"net"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// fakeLegacy is a legacyStore of rows given.
type fakeLegacy struct {
	forms    map[int][]database.LegacyForm
	patterns map[int]*database.SpellPatternRecord
}

func (f fakeLegacy) LegacyFormsOf(id int) ([]database.LegacyForm, error) { return f.forms[id], nil }
func (f fakeLegacy) SpellPattern(id int) (*database.SpellPatternRecord, error) {
	return f.patterns[id], nil
}

// The two seeded forms' workbook effects (tools/legacy_forms_20261126.py, its
// output): Fervor's 2004 weapon proc and Sacred Heart's.
const (
	fervor2004Effects = `{"v":1,"scope":"self","target_type":"Self","harmful":false,"duration_s":300,"stack":2,"level":4,"min_level":1,"req_equipment":"Any","sheet_type":"Offensive Proc","effects":[{"type":"Offensive Proc","kind":"proc","procs":[{"type":"Dmg/Heal","kind":"hp","amount":-8,"on":"offense","scope":"target","target_type":"Enemy"}]}]}`
	heart2004Effects  = `{"v":1,"scope":"self","target_type":"Self","harmful":false,"duration_s":300,"stack":2,"level":51,"min_level":30,"req_equipment":"Any","sheet_type":"Offensive Proc","effects":[{"type":"Offensive Proc","kind":"proc","procs":[{"type":"Dmg/Heal","kind":"hp","amount":5,"scale":"cha","scale_pct":150,"on":"offense","scope":"self","target_type":"Self"}]}]}`
)

// Heirloom petition 3184: buying a legacy form's rank makes a cast of the
// ability the legacy spell's (its own record: name, power, recast), from the
// ability's own place in the book and bar; removing the rank, or a reset that
// refunds it, gives the present form back at the next cast. The quality-of-life
// lock (an [OG] character's session) casts the present form. The control: a
// player without the rank casts the present form.
func TestALegacyFormsRankSwapsTheCast(t *testing.T) {
	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 7009}, 9, 1, 1, 2, true)
	p := &entity.Player{ServerID: 1009, CharServerID: 77, Name: "Kalamer", Class: 2, Level: 51, Mastery: entity.Mastery{Points: 3}}
	w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	w.SetCMCatalog([]database.CMRankRow{{ID: 31842, Category: 3, Classes: 1 << 2, Races: 2047, Mastery: "Legacy Sacred Heart",
		Rank: 1, Name: "Legacy Sacred Heart", Cost: 1, Level: 51, Enabled: true}})
	store := fakeLegacy{
		forms: map[int][]database.LegacyForm{701066: {{SpellID: 701066, LegacySpellID: 761842, MasteryID: 31842}}},
		patterns: map[int]*database.SpellPatternRecord{761842: {SpellID: 761842, AbilityLvl: 51, Power: 0, Recast: 3600,
			SpellName: "Sacred Heart (2004)"}},
	}
	book := &database.CharSpellEntry{AddedOrder: 12, OnHotBar: 1, WhereOnHotBar: 3,
		Pattern: database.SpellPatternRecord{SpellID: 701066, AbilityLvl: 51, Power: 500, Recast: 900, SpellName: "Sacred Heart"}}
	cast := func() *database.CharSpellEntry { return legacyCast(store, w.OwnsCMAwake, sess, p, book) }

	if got := cast(); got != book {
		t.Fatalf("the control, no rank: cast %+v", got.Pattern)
	}
	if res := w.BuyCM(sess, 31842, entity.CMKnows{}); res.Refusal != "" {
		t.Fatalf("buying: %s", res.Refusal)
	}
	got := cast()
	if got.Pattern.SpellID != 761842 || got.Pattern.SpellName != "Sacred Heart (2004)" || got.Pattern.Recast != 3600 ||
		got.AddedOrder != 12 || got.WhereOnHotBar != 3 {
		t.Fatalf("bought: cast %+v (order %d, bar %d), want Sacred Heart (2004) from the ability's place", got.Pattern, got.AddedOrder, got.WhereOnHotBar)
	}
	if book.Pattern.SpellID != 701066 {
		t.Fatal("the book's own entry was changed")
	}

	sess.SetOG(true)
	if got := cast(); got != book {
		t.Fatalf("an [OG] character's session: cast %q, want the present form", got.Pattern.SpellName)
	}
	sess.SetOG(false)

	if res := w.RemoveCM(sess, 31842); res.Refusal != "" {
		t.Fatalf("removing: %s", res.Refusal)
	}
	if got := cast(); got != book {
		t.Fatalf("removed: cast %q, want the present form", got.Pattern.SpellName)
	}
	p.Mastery.Points = 1
	if res := w.BuyCM(sess, 31842, entity.CMKnows{}); res.Refusal != "" {
		t.Fatalf("buying again: %s", res.Refusal)
	}
	if got := cast(); got.Pattern.SpellID != 761842 {
		t.Fatal("bought again: not the legacy form")
	}
	if res := w.ResetCMs(sess); res.Paid != 1 {
		t.Fatalf("the reset refunded %d, want 1", res.Paid)
	}
	if got := cast(); got != book {
		t.Fatalf("refunded: cast %q, want the present form", got.Pattern.SpellName)
	}
}

// The seeded forms do what their records say (the tool's effects, read as a
// cast reads them): Fervor's a weapon proc of 8 on the target, Sacred Heart's a
// weapon proc healing its bearer 5 + 150% CHA, both on its own hits, 5 minutes.
func TestTheSeededLegacyFormsRead(t *testing.T) {
	d := entity.SpellDefFrom("Fervor (2004)", "", "", fervor2004Effects)
	if len(d.Effects) != 1 || d.Effects[0].Kind != entity.EffectProc || d.Effects[0].Amount != 8 {
		t.Fatalf("Fervor (2004): %+v", d.Effects)
	}
	d = entity.SpellDefFrom("Sacred Heart (2004)", "", "", heart2004Effects)
	if len(d.Effects) != 1 {
		t.Fatalf("Sacred Heart (2004): %+v", d.Effects)
	}
	e := d.Effects[0]
	if e.Kind != entity.EffectProcHeal || e.Defense || e.Amount != 5 || e.Scale != entity.StatCha || e.ScalePct != 150 {
		t.Fatalf("Sacred Heart (2004): %+v, want an offensive proc heal of 5 + 150%% CHA", e)
	}
}

// legacyforms in the database: the ability's enabled forms, and whether a rank
// gives one (an older form's rank is not sold where the quality-of-life lock
// holds). The control: a disabled row is none.
func TestLegacyFormsInTheDatabase(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/legacy.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO legacyforms (spellid, legacy_spellid, mastery_id, note) VALUES (700032, 761841, 31841, 'n'),
		(700032, 761849, 31849, 'off')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE legacyforms SET enabled = 0 WHERE mastery_id = 31849`); err != nil {
		t.Fatal(err)
	}
	s := database.NewSpellbookStore(db)
	forms, err := s.LegacyFormsOf(700032)
	if err != nil || len(forms) != 1 || forms[0] != (database.LegacyForm{SpellID: 700032, LegacySpellID: 761841, MasteryID: 31841, Note: "n"}) {
		t.Fatalf("forms %+v %v", forms, err)
	}
	if is, err := s.IsLegacyMastery(31841); !is || err != nil {
		t.Fatalf("31841: %v %v", is, err)
	}
	if is, _ := s.IsLegacyMastery(31849); is {
		t.Fatal("the control: a disabled row's rank")
	}
	h := &ClassMasteryHandler{legacyMastery: s.IsLegacyMastery}
	sess := testSession()
	if why := h.legacyRefusal(sess, 31841); why != "" {
		t.Fatalf("no lock: %q", why)
	}
	sess.SetOG(true)
	if why := h.legacyRefusal(sess, 31841); why != legacyRefusalLine {
		t.Fatalf("an [OG] character's: %q", why)
	}
	if why := h.legacyRefusal(sess, 3671); why != "" {
		t.Fatalf("another rank under the lock: %q", why)
	}
}
