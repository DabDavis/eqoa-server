package handlers

import (
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// The Vallon Zek raid as tools/vallon_zek_20261118.py writes it (its --dump: Ronson's t222), played on the
// triggers: the rune's corpse raises the portal by the campfire; Vallon spoken to by anyone answers, then comes
// hostile with his guards; his death is fifteen minutes' loot, the isle cleared and two hours' rest; his ninety
// minutes run out, or the isle stands empty ten, and the trigger rests fifteen. The shard's gate is
// TestPortalShardSendsItsUser; the tool on a real database TestRealVallonZek.

// vzSet writes the tool's set into the test world.
func vzSet(w *trigWorld) {
	w.t.Helper()
	py, err := exec.LookPath("python3")
	if err != nil {
		w.t.Skip("no python3 to dump the set")
	}
	out, err := exec.Command(py, "-I", filepath.Join("..", "..", "tools", "vallon_zek_20261118.py"), "--dump").Output()
	if err != nil {
		w.t.Fatal(err)
	}
	zpSetData(w, out)
}

// A corpse that rolled the rune raises the Portal of Zek where its row stands (held out of the world), talked to
// only, for an hour; a second rune while it stands raises no second one; a corpse of anything else raises none.
func TestVallonZekPortalRisesWithTheRune(t *testing.T) {
	w := newTrigWorld(t)
	vzSet(w)
	const rune = 701521
	if _, err := w.db.Exec(`INSERT INTO lootdroptrigger (patternid, trigger) VALUES (?, 'Portal of Zek')`, rune); err != nil {
		t.Fatal(err)
	}
	cs, _ := w.join(11, "Camper", 15310, 18905, 0)
	h := &NPCWindowHandler{inv: database.NewInventoryStore(w.db)}
	scout := &entity.NPC{ServerID: 77001, Name: "Zek scout", Level: 60, World: 0, X: 15330, Y: 61, Z: 18860, Dead: true}
	drop := func(items []database.CharInventoryEntry) {
		if fired := h.fireDropTriggers(scout, items); fired != nil {
			fired.Wait()
		}
	}
	drop([]database.CharInventoryEntry{{Pattern: database.ItemPatternRecord{PatternID: 12}}})
	if n := w.alive("Portal of Zek"); n != 0 {
		t.Fatalf("a corpse of no rune raised %d portals", n)
	}
	drop([]database.CharInventoryEntry{{Pattern: database.ItemPatternRecord{PatternID: 12}},
		{Pattern: database.ItemPatternRecord{PatternID: rune}}})
	if w.alive("Portal of Zek") != 1 {
		t.Fatalf("the rune raised %d portals, want 1", w.alive("Portal of Zek"))
	}
	p := w.first("Portal of Zek")
	if p.World != 0 || p.X < 15290 || p.X > 15310 || p.Z < 18910 || p.Z > 18930 || p.NPCType&0x80 == 0 {
		t.Errorf("the portal: world %d (%.0f, %.0f), type %#x: want world 0 at its row, talked to only", p.World, p.X, p.Z, p.NPCType)
	}
	if got := strings.Join(yellow(t, cs), "|"); !strings.Contains(got, "A dark portal tears open") {
		t.Errorf("the camp was told %q", got)
	}
	drop([]database.CharInventoryEntry{{Pattern: database.ItemPatternRecord{PatternID: rune}}})
	if n := w.alive("Portal of Zek"); n != 1 {
		t.Fatalf("a second rune: %d portals", n)
	}
	w.at(61 * time.Minute)
	if n := w.alive("Portal of Zek"); n != 0 {
		t.Fatalf("%d portals outstay their hour", n)
	}
	drop([]database.CharInventoryEntry{{Pattern: database.ItemPatternRecord{PatternID: rune}}})
	if w.alive("Portal of Zek") != 1 {
		t.Fatal("a rune after the portal went raised none")
	}
}

// vallonTalk talks to Vallon's own row and checks he comes, with his guards, four seconds later.
func vallonTalk(t *testing.T, w *trigWorld, rs *session.Session, rp *entity.Player, at time.Duration) *entity.NPC {
	t.Helper()
	v := w.standing("Vallon Zek")
	w.at(at)
	if !w.e.Talk(rs, rp, v) {
		t.Fatal("Vallon's talk started nothing")
	}
	if n := w.alive("Vallon Zek"); n != 0 {
		t.Fatalf("Vallon came before his words were done: %d", n)
	}
	w.at(at + 4*time.Second)
	if w.alive("Vallon Zek") != 1 || w.alive("Guard of Vallon") != 3 {
		t.Fatalf("Vallon %d, guards %d: want 1 and 3", w.alive("Vallon Zek"), w.alive("Guard of Vallon"))
	}
	if v.HiddenUntil.IsZero() {
		t.Fatal("Vallon's own row stands beside his copy")
	}
	return v
}

// Anyone spoken to him (no quest asked) brings him; his death: the guards gone, his words, fifteen minutes to
// loot, the isle cleared, the trigger free fifteen minutes later; his row hidden from sixteen minutes after the
// kill for two hours.
func TestVallonZekSpokenToWonAndRested(t *testing.T) {
	w := newTrigWorld(t)
	vzSet(w)
	rs, rp := w.join(10, "Raider", 4372, 7110, 5)
	v := vallonTalk(t, w, rs, rp, 0)
	if w.e.Talk(rs, rp, v) && w.alive("Vallon Zek") != 1 {
		t.Fatal("a second talk brought a second Vallon")
	}
	w.clear()
	c := w.first("Vallon Zek")
	w.at(10 * time.Minute)
	w.kill(c)
	w.e.Killed(entity.KillCredit{NPCName: c.Name, NPCID: c.ServerID})
	w.at(10*time.Minute + time.Second)
	if n := w.alive("Guard of Vallon"); n != 0 {
		t.Fatalf("%d guards outlive Vallon", n)
	}
	if got := strings.Join(yellow(t, rs), "|"); !strings.Contains(got, "You have won the battle, but not the war!") {
		t.Fatalf("no word of his fall: %q", got)
	}
	w.at(10*time.Minute + 901*time.Second)
	if x, z, world, ok := teleported(rs); !ok || world != 5 || x < 4890 || x > 4910 || z < 7040 || z > 7060 {
		t.Fatalf("the raid was not sent to the port-in: %v (%.0f, %.0f) world %d", ok, x, z, world)
	}
	if !w.e.BusyNamed("Vallon Zek") {
		t.Fatal("the trigger is free at once after the loot")
	}
	w.at(10*time.Minute + 961*time.Second)
	if !v.HiddenUntil.After(w.now.Add(110 * time.Minute)) {
		t.Fatalf("his rest: hidden until %v (now %v), want two hours", v.HiddenUntil, w.now)
	}
	w.at(10*time.Minute + 31*time.Minute)
	if w.e.BusyNamed("Vallon Zek") {
		t.Fatal("the trigger still runs half an hour after the kill")
	}
	if !v.HiddenUntil.After(w.now) {
		t.Fatal("Vallon stands to be spoken to half an hour after his death")
	}
	w.at(10*time.Minute + 16*time.Minute + 121*time.Minute)
	if v.HiddenUntil.After(w.now) {
		t.Fatalf("still hidden %v after his rest", v.HiddenUntil.Sub(w.now))
	}
	vallonTalk(t, w, rs, rp, 10*time.Minute+16*time.Minute+122*time.Minute)
}

// His ninety minutes run out: he and his guards go, the isle is cleared, his row stands again, and for
// fifteen minutes no talk brings him; then one does.
func TestVallonZekTimesOutAndRests(t *testing.T) {
	w := newTrigWorld(t)
	vzSet(w)
	rs, rp := w.join(10, "Raider", 4372, 7110, 5)
	v := vallonTalk(t, w, rs, rp, 0)
	w.at(4*time.Second + 5400*time.Second + time.Second)
	if w.alive("Vallon Zek") != 0 || w.alive("Guard of Vallon") != 0 {
		t.Fatalf("after ninety minutes: Vallon %d, guards %d", w.alive("Vallon Zek"), w.alive("Guard of Vallon"))
	}
	if _, _, world, ok := teleported(rs); !ok || world != 5 {
		t.Fatal("the raid was not sent off the isle")
	}
	if v.HiddenUntil.After(w.now) {
		t.Fatal("his row is still hidden after the run")
	}
	w.at(4*time.Second + 5400*time.Second + 10*time.Minute)
	if w.e.Talk(rs, rp, v) || w.alive("Vallon Zek") != 0 {
		t.Fatal("a talk in the rest brought him")
	}
	vallonTalk(t, w, rs, rp, 4*time.Second+5400*time.Second+16*time.Minute)
}

// The raid wiped and gone: ten minutes with no one on the isle end the run, fifteen more its rest.
func TestVallonZekEmptyIsleEnds(t *testing.T) {
	w := newTrigWorld(t)
	vzSet(w)
	rs, rp := w.join(10, "Raider", 4372, 7110, 5)
	vallonTalk(t, w, rs, rp, 0)
	rp.World = 0
	w.at(5 * time.Second)
	w.at(5*time.Second + 601*time.Second)
	if w.alive("Vallon Zek") != 0 {
		t.Fatal("Vallon stays on an empty isle")
	}
	w.at(5*time.Second + 601*time.Second + 15*time.Minute + time.Second)
	if w.e.BusyNamed("Vallon Zek") {
		t.Fatal("the trigger still rests")
	}
}

// A Portal Shard of Zek (itemspell, spellgates): its Use casts Zek's Portal, no cast time, no power, which sends
// its user alone to Vallon's isle, and takes one shard off the stack of twenty.
func TestPortalShardSendsItsUser(t *testing.T) {
	f := newScribeFixture(t)
	w := entity.NewWorld()
	w.SessionManager = session.NewManager()
	f.sess = w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(203, 0, 113, 77), Port: 5077}, 77, 77, 77, 1, true)
	f.sess.SetCharacterInWorld(true)
	f.player.HP, f.player.MaxHP, f.player.Level, f.player.Power, f.player.MaxPower = 1000, 1000, 60, 0, 5000
	f.player.X, f.player.Z = 15300, 18900
	w.Players[f.player.ServerID] = f.player
	f.sess.SetECSEntity(f.player)
	f.h.gameWorld = w
	const gate = 760380
	if _, err := f.db.Exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power,
		iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		VALUES (?, 1, 0, 0, 16544, 0, 0, 0, 0, 0, 0, -1, 'Zek''s Portal', '')`, gate); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO spelleffect (spellid, line, fxdict, source, effects)
		VALUES (?, 'Self, Teleports the caster to Vallon Zek''s training grounds', 0, 'test', '')`, gate); err != nil {
		t.Fatal(err)
	}
	w.SetSpellGates(map[string]database.SpellGate{"zek's portal": {Spell: "zek's portal", Kind: database.GateSelf,
		Place: "Vallon Zek's training grounds", World: 5, X: 4377.8, Y: 253.5, Z: 7052.8}})
	var sent []entity.GateTravel
	w.OnTravel(func(tr entity.GateTravel) { sent = append(sent, tr) })
	shard := f.item("Portal Shard of Zek", false, 0x7FFF, 1, 20)
	if _, err := f.db.Exec(`INSERT INTO itemeffect (patternid, line, name, charges, source) VALUES (?, 'Self, Teleports the caster to Vallon Zek''s training grounds', 'Zek''s Portal', 1, 'test')`, shard); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO itemspell (patternid, spellid, rest_s) VALUES (?, ?, 0)`, shard, gate); err != nil {
		t.Fatal(err)
	}
	f.own(shard, 20)
	f.loadList()
	f.use(0)
	if n := f.stackOf("Portal Shard of Zek"); n != 19 {
		t.Fatalf("after a Use: %d shards, want 19", n)
	}
	now := time.Now()
	for i := 1; i <= 40 && len(sent) == 0; i++ {
		w.Tick(now.Add(time.Duration(i) * 100 * time.Millisecond))
	}
	if len(sent) != 1 {
		t.Fatalf("the shard sent %d travellers, want its user", len(sent))
	}
	if tr := sent[0]; tr.P != f.player || tr.To.World != 5 || tr.To.X != 4377.8 || tr.To.Z != 7052.8 {
		t.Errorf("the shard's travel %+v", tr)
	}
}

// A creature's held drop is its name's npcheld record for that hand (No Trade as it is) where it has one, the
// catalog's of its look otherwise; nothing for a hand it holds nothing in. The catalog's here is a rare record (a
// use: the fixture's items' procanim) far below level 68, so the drops roll rare in a window wide enough for it;
// the named record drops whatever the roll (the owner, 2026-11-21: the 1 in 500 is the look's pick alone).
func TestNPCHeldNamesItsDrop(t *testing.T) {
	f := newScribeFixture(t)
	const look = 0x4B4F6A2A
	plain := f.item("Sword of Dreams", false, 0x7FFF, 60, 1)
	named := f.item("Sapphyra's Sword", false, 0x7FFF, 60, 1)
	for _, id := range []int{plain, named} {
		if _, err := f.db.Exec(`UPDATE itempattern SET model = ?, equipslot = 12 WHERE patternid = ?`, look, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.db.Exec(`UPDATE itempattern SET trade = 1 WHERE patternid = ?`, named); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO npcheld (npc_name, hand, patternid) VALUES ('vallon zek', 0, ?)`, named); err != nil {
		t.Fatal(err)
	}
	d, err := f.inv.HeldDrops()
	if err != nil {
		t.Fatal(err)
	}
	d.LevelBelow = 100
	SetHeldDrops(d)
	t.Cleanup(func() { SetHeldDrops(nil) })
	drops := func(name string, primary, shield int) []string {
		var out []string
		for _, e := range heldDropItems(&entity.NPC{Name: name, Level: 68, PrimaryWeapon: primary, Shield: shield}, func() float64 { return 0 }) {
			out = append(out, e.Pattern.ItemName)
		}
		return out
	}
	if got := strings.Join(drops("Vallon Zek", look, 0), ","); got != "Sapphyra's Sword" {
		t.Errorf("Vallon drops %q, want Sapphyra's Sword", got)
	}
	if got := strings.Join(drops("Zek trainer", look, 0), ","); got != "Sword of Dreams" {
		t.Errorf("another of the look drops %q, want the catalog's tradeable record", got)
	}
	if got := drops("Vallon Zek", 0, 0); len(got) != 0 {
		t.Errorf("empty-handed, Vallon drops %v", got)
	}
}
