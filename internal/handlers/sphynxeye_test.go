package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// Live petition 5514: the Eye of the Sphynx as tools/sky_eye_20261123.py makes it (one a copy, an
// itemeffect of no charges, its Use a self gate to the Plane of Sky): a Use begins the gate's cast, and
// the Eye stays in the pack, a Use after a Use, as the guide's "carried each time" has it; a stacking
// record (the Eye as it was, maxstack 50) would have given one off its stack.
func TestTheEyeOfTheSphynxIsKept(t *testing.T) {
	f := newScribeFixture(t)
	w := entity.NewWorld()
	w.SessionManager = session.NewManager()
	f.player.HP, f.player.MaxHP, f.player.Level, f.player.Power, f.player.MaxPower = 1000, 1000, 60, 10, 5000
	w.Players[f.player.ServerID] = f.player
	f.sess.SetECSEntity(f.player)
	f.h.gameWorld = w
	const gate = 760400
	if _, err := f.db.Exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power,
		iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		VALUES (?, 1, 0, 0, 16544, 0, 0, 0, 0, 0, 0, -1, 'Eye of the Sphynx', '')`, gate); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO spelleffect (spellid, line, fxdict, source, effects) VALUES (?, ?, 0, 'test', ?)`, gate,
		"Self, Teleports the caster to the Plane of Sky, by Sartheret",
		`{"v":1,"scope":"self","target_type":"Self","harmful":false,"stack":0,"level":1,"min_level":1,"req_equipment":"Any","sheet_type":"Teleport","arch_type":"Teleport","effects":[{"id":"086D","type":"Teleport","kind":"teleport","location":"Plane of Sky,4834.8,250.06,5524.7"}]}`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO spellgates (spell, kind, place, world, x, y, z, radius, note, source)
		VALUES ('eye of the sphynx', 'self', 'the Plane of Sky', 4, 4834.8, 250.06, 5524.7, 0, '', 'test')`); err != nil {
		t.Fatal(err)
	}
	if n, err := w.LoadSpellGates(f.db); err != nil || n == 0 {
		t.Fatalf("spell gates: %d %v", n, err)
	}
	eye := f.item("Eye of the Sphynx", false, 0x7FFF, 1, 1)
	if _, err := f.db.Exec(`INSERT INTO itemeffect (patternid, line, name, charges, source) VALUES (?, 'Self, Teleports the caster to the Plane of Sky', 'Eye of the Sphynx', 0, 'test')`, eye); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO itemspell (patternid, spellid, rest_s) VALUES (?, ?, 0)`, eye, gate); err != nil {
		t.Fatal(err)
	}
	f.own(eye, 1)
	f.loadList()
	held := func() (int, uint32) {
		t.Helper()
		n, key := 0, uint32(0)
		for i, r := range f.rowsOf() {
			if r.Pattern.PatternID == eye {
				n += r.StackLeft
				key = uint32(i)
			}
		}
		return n, key
	}
	for use := 1; use <= 3; use++ {
		n, key := held()
		if n != 1 {
			t.Fatalf("before use %d: %d Eyes held, want 1", use, n)
		}
		sent := f.use(key)
		if f.player.Casting == nil {
			t.Fatalf("use %d: no gate cast (%s)", use, ops(sent))
		}
		f.player.Casting = nil
	}
	if n, _ := held(); n != 1 {
		t.Errorf("after three uses: %d Eyes held, want 1", n)
	}
}
