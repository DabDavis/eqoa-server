package handlers

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// petrename.go: players name their mercs (/merc rename, every spelling) and
// their summoned pets (/pet rename), the names held to a character's rules,
// the name filter and the names the world holds; each rename asked logged.

// mercOut is the player's merc NPC of the name in the world, nil none.
func mercOut(r *portalRig, name string) *entity.NPC {
	for _, n := range r.gw.NPCs {
		if n.Owner == r.pa.ServerID && n.Name == name {
			return n
		}
	}
	return nil
}

// renameLog is petnamelog's lines for the character, oldest first, as
// "outcome kind asked: reason".
func renameLog(t *testing.T, db *database.SQLiteDB, serverID uint32) []string {
	t.Helper()
	got, err := db.PetRenameLog(serverID, 100)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for i := len(got) - 1; i >= 0; i-- {
		e := got[i]
		out = append(out, fmt.Sprintf("%s %s %s: %s", e.Outcome, e.Kind, e.Asked, e.Reason))
	}
	return out
}

// A merc renamed: in its row (audited under the player), where it stands
// (its NPC, drawn by the name on the channel every viewer reads), its owner
// told; it comes back by the name at the next login. Every spelling: the
// one hired, by its slot, by its name, "rename <old> <new>", a PS2's "/t merc".
func TestMercRename(t *testing.T) {
	r, m, db := newMercFixture(t)
	m.hire(r.anna, r.pa, 0, 0, 0)
	info, ok := r.gw.MercInfoOf(r.anna)
	if !ok {
		t.Fatal("no merc hired")
	}
	r.drain()
	r.say(r.anna, "/merc rename rennis")
	got := r.msgs(r.anna)
	if !hasMsg(got, "chat:"+fmt.Sprintf(mercRenamedLine, info.Name, "Rennis")) {
		t.Fatalf("/merc rename rennis: %q", got)
	}
	if now, _ := r.gw.MercInfoOf(r.anna); now.Name != "Rennis" {
		t.Errorf("its name where it stands: %q", now.Name)
	}
	n := mercOut(r, "Rennis")
	if n == nil {
		t.Fatal("no merc NPC named Rennis")
	}
	if plate := string(bytes.TrimRight(entity.SerializeNPCUpdate(1, n)[116:140], "\x00")); plate != "Rennis" {
		t.Errorf("its nameplate on the channel: %q", plate)
	}
	if row, _, _ := db.Merc(r.pa.CharServerID, 1); row.Name != "Rennis" {
		t.Errorf("its row: %q", row.Name)
	}
	var audits int
	if err := db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'charmerc' AND via = 'game' AND detail LIKE '/merc rename:%Rennis'`).Scan(&audits); err != nil || audits != 1 {
		t.Errorf("its audit lines: %d %v", audits, err)
	}
	// the logout and login: its row's name comes back
	r.gw.DismissMerc(r.anna, 1, time.Now())
	m.comeBack(r.anna, r.pa)
	if back, ok := r.gw.MercInfoOf(r.anna); !ok || back.Name != "Rennis" {
		t.Fatalf("at the next login: %+v %v", back, ok)
	}

	// the other spellings
	r.say(r.anna, "/t merc 1 rename Tovan")
	if now, _ := r.gw.MercInfoOf(r.anna); now.Name != "Tovan" {
		t.Fatalf("/t merc 1 rename Tovan: %q (%q)", now.Name, r.msgs(r.anna))
	}
	r.say(r.anna, "/t merc Tovan rename Ulric")
	if now, _ := r.gw.MercInfoOf(r.anna); now.Name != "Ulric" {
		t.Fatalf("/t merc Tovan rename Ulric: %q (%q)", now.Name, r.msgs(r.anna))
	}
	r.say(r.anna, "#merc rename Ulric Varian")
	if now, _ := r.gw.MercInfoOf(r.anna); now.Name != "Varian" {
		t.Fatalf("#merc rename Ulric Varian: %q (%q)", now.Name, r.msgs(r.anna))
	}
	if mercWords("Bob rename") || !mercWords("Varian rename Wyll") || !mercWords("rename Wyll") {
		t.Error("a tell's words taken for a rename, or a rename's not")
	}
	log := renameLog(t, db, r.pa.CharServerID)
	if len(log) != 4 || log[0] != "renamed merc rennis: " || log[3] != "renamed merc Varian: " {
		t.Errorf("the renames logged: %q", log)
	}
}

// Each refusal: its line, its logged outcome, and the merc's name unchanged.
// The name filter's (a refused word, a reserved or staff name, a GM's
// character's) are "filtered" and tell the player only that the name is not
// allowed; the rest are "refused" and say why.
func TestMercRenameRefusals(t *testing.T) {
	r, m, db := newMercFixture(t)
	m.hire(r.anna, r.pa, 0, 0, 0)
	m.hire(r.anna, r.pa, 13, 0, 0)
	all := r.gw.MercInfosOf(r.anna)
	if len(all) != 2 {
		t.Fatalf("hired %d", len(all))
	}
	first, second := all[0].Name, all[1].Name
	// what the world holds: another character's merc, a character, a GM's
	// character, a named individual
	if err := db.SetMerc(database.MercRow{ServerID: 9001, Slot: 1, Name: "Arthas", Class: 10, Race: 1, Stance: 2}); err != nil {
		t.Fatal(err)
	}
	insertCharacter(t, db, 9002, "Belion")
	insertCharacter(t, db, 9003, "Gmwarden")
	if _, err := db.Exec(`INSERT INTO accountinfo (accountid, username, acctlevel) VALUES (77, 'wardenacct', 2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE characters SET accountid = 77 WHERE serverid = 9003`); err != nil {
		t.Fatal(err)
	}
	r.gw.NPCs[7001] = &entity.NPC{ServerID: 7001, Name: "Dabrish", X: 1, Z: 1}
	for _, c := range []struct {
		asked, line, outcome string
	}{
		{"Re", fmt.Sprintf(renameLength, "Re"), database.PetRefused},
		{"Abcdefghijklmnop", fmt.Sprintf(renameLength, "Abcdefghijklmnop"), database.PetRefused},
		{"R3nnis", fmt.Sprintf(renameNotWord, "R3nnis"), database.PetRefused},
		{"Bigcum", renameFiltered, database.PetFiltered},
		{"Fuuuck", renameFiltered, database.PetFiltered},
		{"Admin", renameFiltered, database.PetFiltered},
		{"Guide", renameFiltered, database.PetFiltered},
		{"gmwarden", renameFiltered, database.PetFiltered},
		{"belion", fmt.Sprintf(renameCharacter, "Belion"), database.PetRefused},
		{"Anna", fmt.Sprintf(renameCharacter, "Anna"), database.PetRefused},
		{"ARTHAS", fmt.Sprintf(renameMercTaken, "Arthas"), database.PetRefused},
		{second, fmt.Sprintf(renameMercTaken, second), database.PetRefused},
		{"dabrish", fmt.Sprintf(renameIndividual, "Dabrish"), database.PetRefused},
		{strings.ToLower(first), fmt.Sprintf(renameSame, first), database.PetRefused},
	} {
		r.drain()
		r.say(r.anna, "/merc 1 rename "+c.asked)
		if got := r.msgs(r.anna); !hasMsg(got, "red:"+c.line) {
			t.Errorf("%q: told %q, want %q", c.asked, got, c.line)
		}
		if now := r.gw.MercInfosOf(r.anna)[0].Name; now != first {
			t.Fatalf("%q renamed it %q", c.asked, now)
		}
		log := renameLog(t, db, r.pa.CharServerID)
		if last := log[len(log)-1]; !strings.HasPrefix(last, c.outcome+" merc "+c.asked+": ") {
			t.Errorf("%q logged %q, want outcome %s", c.asked, last, c.outcome)
		}
	}
	// two words: not a name
	r.drain()
	r.say(r.anna, "/merc 1 rename Ren nis")
	if got := r.msgs(r.anna); !hasMsg(got, "red:"+fmt.Sprintf(renameNotWord, "Ren nis")) {
		t.Errorf("two words: %q", got)
	}
	// two hired and none named: which one
	r.drain()
	r.say(r.anna, "/merc rename Rennis")
	if got := r.msgs(r.anna); !hasMsg(got, "red:Name which mercenary: *") {
		t.Errorf("none named of two: %q", got)
	}
	if row, _, _ := db.Merc(r.pa.CharServerID, 1); row.Name != first {
		t.Errorf("its row renamed %q", row.Name)
	}
	// the filter's catches, as a GM finds them
	var filtered int
	if err := db.QueryRow(`SELECT COUNT(*) FROM petnamelog WHERE outcome = 'filtered' AND serverid = ?`, r.pa.CharServerID).Scan(&filtered); err != nil || filtered != 5 {
		t.Errorf("filtered lines: %d %v", filtered, err)
	}
}

// petRig is the merc fixture's player, a magician, with a Fire Elemental's
// body in the world for its summoning.
func petRenameRig(t *testing.T) (*portalRig, *Mercs, *database.SQLiteDB, entity.CastSpell) {
	r, m, db := newMercFixture(t)
	r.pa.Class, r.pa.Power, r.pa.MaxPower = 10, 2000, 2000
	r.gw.NPCs[9100] = &entity.NPC{ServerID: 9100, Name: "Fire Elemental", Level: 30, HP: 900, MaxHP: 900, ModelID: 0x8A416241,
		Size: 1, NPCType: 0x80, X: 9000, Z: 9000}
	spell := entity.CastSpell{SpellID: 717, Level: 30, Name: "Fire Elemental", Def: entity.ParseSpellText("Summons a Fire Elemental to aid you")}
	return r, m, db, spell
}

// /pet rename names the summoned pet at once (its NPC, its channel's name)
// and keeps the name for its spell, so it comes by it after a logout and a
// login (petnames, loaded at the login); a PS2's "/t pet rename"; the filter
// and the name rules as a merc's; no pet, no name; a tell to someone named
// Pet is a tell.
func TestPetRename(t *testing.T) {
	r, m, db, spell := petRenameRig(t)
	now := time.Unix(1_800_000_000, 0)
	m.now = func() time.Time { return now }
	r.drain()
	r.say(r.anna, "/pet rename Rennis")
	if got := r.msgs(r.anna); !hasMsg(got, "red:"+entity.PetRenameNoPet) {
		t.Errorf("no pet: %q", got)
	}
	if got := r.gw.SummonByItem(r.anna, spell, now, time.Time{}); got != entity.ItemSummoned {
		t.Fatalf("no pet: %v", got)
	}
	pet := r.gw.NPCs[r.gw.PetOf(r.pa)]
	r.drain()
	r.say(r.anna, "/pet rename rennis")
	if got := r.msgs(r.anna); !hasMsg(got, "chat:"+fmt.Sprintf(petRenamedLine, "Fire Elemental", "Rennis")) {
		t.Fatalf("/pet rename rennis: %q", got)
	}
	if pet.Name != "Rennis" {
		t.Errorf("the pet is %q", pet.Name)
	}
	if plate := string(bytes.TrimRight(entity.SerializeNPCUpdate(1, pet)[116:140], "\x00")); plate != "Rennis" {
		t.Errorf("its nameplate on the channel: %q", plate)
	}
	if names, err := db.PetNamesOf(r.pa.CharServerID); err != nil || names[717] != "Rennis" {
		t.Errorf("petnames: %v %v", names, err)
	}
	// the filter and the rules, by a PS2's spelling
	r.drain()
	r.say(r.anna, "/t pet rename Shitlord")
	if got := r.msgs(r.anna); !hasMsg(got, "red:"+renameFiltered) || pet.Name != "Rennis" {
		t.Errorf("the filter: %q, the pet %q", got, pet.Name)
	}
	insertCharacter(t, db, 9002, "Belion")
	r.drain()
	r.say(r.anna, "#pet rename Belion")
	if got := r.msgs(r.anna); !hasMsg(got, "red:"+fmt.Sprintf(renameCharacter, "Belion")) {
		t.Errorf("a character's name: %q", got)
	}
	r.drain()
	r.say(r.anna, "/pet")
	if got := r.msgs(r.anna); !hasMsg(got, "chat:"+petRenameUsage) {
		t.Errorf("/pet: %q", got)
	}
	if _, ok := petRenameLine("/t pet hello there"); ok {
		t.Error("a tell to Pet taken for /pet")
	}
	log := renameLog(t, db, r.pa.CharServerID)
	if len(log) != 3 || log[0] != "renamed pet rennis: " || !strings.HasPrefix(log[1], "filtered pet Shitlord: ") {
		t.Errorf("the renames logged: %q", log)
	}

	// a logout and a login: the player made anew, its names loaded, its
	// pet summoned again by its spell
	r.gw.PetCommand(r.anna, 5, 0, now) // Dismiss
	r.pa.PetNames = nil
	ch := &CharacterHandler{gameWorld: r.gw, store: database.NewSQLiteCharacterStore(db)}
	ch.loadPetNames(r.pa.CharServerID)
	now = now.Add(time.Minute)
	if got := r.gw.SummonByItem(r.anna, spell, now, time.Time{}); got != entity.ItemSummoned {
		t.Fatalf("no second pet: %v", got)
	}
	if again := r.gw.NPCs[r.gw.PetOf(r.pa)]; again == nil || again == pet || again.Name != "Rennis" {
		t.Errorf("summoned again after the login: %+v", again)
	}
	// the item's own pet out is still its pet, whatever its name
	if got := r.gw.SummonByItem(r.anna, spell, now, time.Time{}); got != entity.ItemPetOut {
		t.Errorf("the renamed pet out is not the item's: %v", got)
	}
}
