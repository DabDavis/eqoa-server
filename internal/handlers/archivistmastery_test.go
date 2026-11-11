package handlers

import (
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// The Spell Archivist leaves the masteries' spells to them (petition 1381,
// archivistmastery.go), against a database, the quests running and the
// mastery window's handler over the same books (the server's wiring: the
// character store, the spell books). A necromancer of 60 beside him, made up,
// the names and ids the live server's: Gloom Pact a merchant's scroll, and
// Gloom Covenant one too, which Deadly Covenant 1 gives in its place; Flesh
// Mastery a class quest's, and Flesh Mastery + another's (the live data's
// wrong record before petfix172), which Teachings of Morthalis 1 gives for
// the plain one it needs; Bone Ward a merchant's scroll a rank only adds (as
// every class's Hide mastery adds the rogue's Hide).

const (
	mGloomPact     = 882
	mGloomCovenant = 900881
	mFlesh         = 900913
	mFleshPlus     = 2282
	mBoneWard      = 6301 // made up
	mNecromancer   = 11
	mRankCovenant  = 6351
	mRankMorthalis = 6381
	mRankBoneWard  = 6390 // made up
)

// newMasteryRig is the necromancer of 60, the ranks enabled or not.
func newMasteryRig(t *testing.T, ranks bool) *archivistFixture {
	t.Helper()
	f := newArchivistFixture(t, nil)
	f.p.Class, f.p.Level = mNecromancer, 60
	for _, s := range []struct {
		id    int
		name  string
		level int
	}{
		{1623, "Return Home", 1}, {mGloomPact, "Gloom Pact", 4}, {mGloomCovenant, "Gloom Covenant", 4},
		{mBoneWard, "Bone Ward", 10}, {mFlesh, "Flesh Mastery", 23}, {mFleshPlus, "Flesh Mastery +", 23},
	} {
		f.spell(s.id, s.name, s.level, "")
	}
	for _, id := range []int{mGloomPact, mGloomCovenant, mBoneWard, mFlesh, mFleshPlus} {
		f.classSpell(mNecromancer, id, "dump")
	}
	f.shop(SpellArchivistName, 0x81, archivistShop)
	nec := 1 << mNecromancer
	f.shop("Scroll Merchant", 0x81, 41, f.scroll("Gloom Pact", nec, 4), f.scroll("Gloom Covenant", nec, 4), f.scroll("Bone Ward", nec, 10))
	f.sideQuest(200101, "23-47/23/p1", true, "", 23, "", "", fmt.Sprintf(`{"11":{"spells":"%d"}}`, mFlesh), "")
	f.sideQuest(200102, "23-47/33/teagarden", true, "", 33, "", "", fmt.Sprintf(`{"11":{"spells":"%d"}}`, mFleshPlus), "")
	on := 0
	if ranks {
		on = 1
	}
	for _, r := range []struct {
		id                    int
		name, mastery         string
		adds, addIDs          string
		removes, removeIDs    string
		requires, requiresIDs string
	}{
		{mRankCovenant, "Deadly Covenant 1", "Deadly Covenant", "Gloom Covenant@4", fmt.Sprint(mGloomCovenant),
			"Gloom Pact@4", fmt.Sprint(mGloomPact), "Gloom Pact@4", fmt.Sprint(mGloomPact)},
		{mRankMorthalis, "Teachings of Morthalis 1", "Teachings of Morthalis", "Flesh Mastery +@23", fmt.Sprint(mFleshPlus),
			"Flesh Mastery@23", fmt.Sprint(mFlesh), "Flesh Mastery@23", fmt.Sprint(mFlesh)},
		{mRankBoneWard, "Bone Ward", "Bone Ward", "Bone Ward@10", fmt.Sprint(mBoneWard), "", "", "", ""},
	} {
		if _, err := f.db.Exec(`INSERT INTO cmcatalog (id, category, classes, races, mastery, rank, name, cost, level, adds,
			add_spells, removes, remove_spells, requires, require_spells, enabled, source)
			VALUES (?, 3, ?, 2047, ?, 1, ?, 25, 20, ?, ?, ?, ?, ?, ?, ?, 'test')`, r.id, nec, r.mastery, r.name, r.adds, r.addIDs,
			r.removes, r.removeIDs, r.requires, r.requiresIDs, on); err != nil {
			t.Fatal(err)
		}
	}
	qe, err := NewQuestEngine(f.db, f.w, f.mgr, f.inv, f.spells)
	if err != nil {
		t.Fatal(err)
	}
	f.a.quests = func() *QuestEngine { return qe }
	f.know(1623, 0, 0)
	return f
}

// buyRank sends 0x1401's purchase of a rank (classmastery_test.go's cmTest.send)
// and returns the red lines.
func (f *archivistFixture) buyRank(h *ClassMasteryHandler, id int) []string {
	f.t.Helper()
	f.sess.Queue().GetPending()
	req := make([]byte, 9)
	req[0] = cmBuy
	binary.LittleEndian.PutUint32(req[5:], uint32(int32(id)))
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeClassMastery)
	msg.SetData(req)
	if err := h.HandleClassMastery(f.sess, msg); err != nil {
		f.t.Fatal(err)
	}
	var red []string
	for _, m := range f.sess.Queue().PeekPending() {
		d := m.Data()
		if protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeErrorMessage {
			r := &cmReader{b: d[2:]}
			red = append(red, r.wide(1024))
		}
	}
	return red
}

// "Grant basic spells" writes Gloom Pact, Bone Ward and Flesh Mastery and
// neither mastery's own: not Gloom Covenant though a merchant sells it (the
// rank's way up from Gloom Pact), not Flesh Mastery + though a quest teaches
// it. The controls: with the ranks disabled he writes both (the rule is the
// ranks'); and buying each rank through the mastery window still gives its
// spell for the one it replaces (Gloom Covenant for Gloom Pact, Flesh
// Mastery + for the Flesh Mastery he wrote), and a grant after it puts
// neither replaced spell back.
func TestTheArchivistLeavesTheMasteriesSpells(t *testing.T) {
	f := newMasteryRig(t, true)
	f.wantMsgs("the grant", f.grant(), "add:882@1 flag 0", "add:6301@2 flag 0", "add:900913@3 flag 0",
		"box:Spell Archivist: "+fmt.Sprintf(archivistGranted, spellCount(3), 60))
	f.wantMsgs("a second grant", f.grant(), "box:Spell Archivist: "+fmt.Sprintf(archivistHasAll, 60))

	g := newMasteryRig(t, false)
	g.wantMsgs("the grant with no rank enabled", g.grant(), "add:900881@1 flag 0", "add:882@2 flag 0", "add:6301@3 flag 0",
		"add:900913@4 flag 0", "add:2282@5 flag 0", "box:Spell Archivist: "+fmt.Sprintf(archivistGranted, spellCount(5), 60))

	rows, err := f.db.CMCatalog()
	if err != nil {
		t.Fatal(err)
	}
	f.w.SetCMCatalog(rows)
	h := NewClassMasteryHandler(f.mgr)
	h.SetGameWorld(f.w)
	h.SetStore(database.NewSQLiteCharacterStore(f.db))
	h.SetSpellbook(f.spells)
	f.p.Mastery.Points = 100
	for _, id := range []int{mRankCovenant, mRankMorthalis} {
		if red := f.buyRank(h, id); len(red) != 0 {
			t.Fatalf("buying %d: %q", id, red)
		}
	}
	has := map[string]bool{}
	for _, r := range f.book() {
		has[strings.SplitN(r, "@", 2)[0]] = true
	}
	for id, want := range map[int]bool{mGloomCovenant: true, mFleshPlus: true, mGloomPact: false, mFlesh: false, mBoneWard: true} {
		if has[fmt.Sprint(id)] != want {
			t.Errorf("after the purchases the book %v: %d held %v, want %v", f.book(), id, has[fmt.Sprint(id)], want)
		}
	}
	// nor does a grant put back what they took out
	f.wantMsgs("a grant after the purchases", f.grant(), "box:Spell Archivist: "+fmt.Sprintf(archivistHasAll, 60))
}

// A wizard's familiars are granted whatever masteries the wizard owns
// (petition 1592, Kapow: "no familiars on wizy when requesting spells from
// spell dude"): the live server's Guardian Familiar 1 takes Lesser Familiar
// out for Lesser Guardian, another pet beside it, and a grant after its
// purchase put Lesser Familiar back no more; it does now, beside the
// Guardian, and a second grant writes nothing. Lesser Guardian stays the
// mastery's: the grant never writes it. The ids and names are the live
// server's.
func TestTheArchivistGrantsFamiliarsPastGuardianFamiliar(t *testing.T) {
	const (
		minorFamiliar, lesserFamiliar, lesserGuardian = 700110, 700200, 700893
		rankGuardian                                  = 6941
	)
	f := newArchivistFixture(t, nil)
	f.p.Class, f.p.Level = archivistWizard, 60
	for _, s := range []struct {
		id    int
		name  string
		level int
	}{
		{1623, "Return Home", 1}, {minorFamiliar, "Minor Familiar", 12}, {lesserFamiliar, "Lesser Familiar", 20},
		{lesserGuardian, "Lesser Guardian", 20},
	} {
		f.spell(s.id, s.name, s.level, "")
	}
	f.classSpell(archivistWizard, minorFamiliar, "eqoa-data")
	f.classSpell(archivistWizard, lesserFamiliar, "eqoa-data")
	f.shop(SpellArchivistName, 0x81, archivistShop)
	wiz := 1 << archivistWizard
	f.shop("Scroll Merchant", 0x81, 41, f.scroll("Minor Familiar", wiz, 12), f.scroll("Lesser Familiar", wiz, 20))
	if _, err := f.db.Exec(`INSERT INTO cmcatalog (id, category, classes, races, mastery, rank, name, cost, level, adds,
		add_spells, removes, remove_spells, requires, require_spells, enabled, source)
		VALUES (?, 3, ?, 2047, 'Guardian Familiar', 1, 'Guardian Familiar 1', 25, 20, 'Lesser Guardian@20', ?,
		'Lesser Familiar@20', ?, 'Lesser Familiar@20', ?, 1, 'test')`, rankGuardian, wiz, fmt.Sprint(lesserGuardian),
		fmt.Sprint(lesserFamiliar), fmt.Sprint(lesserFamiliar)); err != nil {
		t.Fatal(err)
	}
	f.know(1623, 0, 0)
	f.wantMsgs("the grant", f.grant(), "add:700110@1 flag 0", "add:700200@2 flag 0",
		"box:Spell Archivist: "+fmt.Sprintf(archivistGranted, spellCount(2), 60))

	rows, err := f.db.CMCatalog()
	if err != nil {
		t.Fatal(err)
	}
	f.w.SetCMCatalog(rows)
	h := NewClassMasteryHandler(f.mgr)
	h.SetGameWorld(f.w)
	h.SetStore(database.NewSQLiteCharacterStore(f.db))
	h.SetSpellbook(f.spells)
	f.p.Mastery.Points = 100
	if red := f.buyRank(h, rankGuardian); len(red) != 0 {
		t.Fatalf("buying Guardian Familiar 1: %q", red)
	}
	if b := strings.Join(f.book(), " "); strings.Contains(b, "700200@") || !strings.Contains(b, "700893@") {
		t.Fatalf("after the purchase the book %s: want Lesser Guardian for Lesser Familiar", b)
	}
	got := f.grant()
	if len(got) != 2 || !strings.HasPrefix(got[0], "add:700200@") ||
		got[1] != "box:Spell Archivist: "+fmt.Sprintf(archivistGranted, spellCount(1), 60) {
		t.Fatalf("the grant after the purchase: %q, want Lesser Familiar back", got)
	}
	if b := strings.Join(f.book(), " "); !strings.Contains(b, "700200@") || !strings.Contains(b, "700893@") {
		t.Errorf("the book %s: want Lesser Familiar beside Lesser Guardian", b)
	}
	f.wantMsgs("a second grant", f.grant(), "box:Spell Archivist: "+fmt.Sprintf(archivistHasAll, 60))
}
