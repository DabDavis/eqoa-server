package handlers

import (
	"fmt"
	"math"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Mr. Moogie's challenge and his favor through his talk (the GM Jibrish's petitions 1379 and 1380,
// by Jibcro): a rig of a database holding the Moogie Tribute (no trade, lore) and the Duelist's Favor
// ("Self, Increase Size by 50%, 60min"), Mr. Moogie beside the players, his metal twin's spawn-only
// original 30 off with the tribute as its npcitem row.

const (
	metalOrigNPCID  = 9701
	metalFavorSpell = 9501
)

type metalFixture struct {
	t       *testing.T
	db      *database.SQLiteDB
	w       *entity.World
	mgr     *session.Manager
	ih      *InteractHandler
	windows *NPCWindowHandler
	inv     *database.InventoryStore
	moogie  *entity.NPC
	orig    *entity.NPC
	tribute int
	next    int
}

func newMetalFixture(t *testing.T) *metalFixture {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/metal.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	f := &metalFixture{t: t, db: db, inv: database.NewInventoryStore(db)}
	res, err := db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, 123, 0, -1, 0, 1, 1, 0, 0, 0, 0, 1, 1, 0, 0, 32767, 1023, 0, 1, 0, 0, 'Moogie Tribute', '', 0, 0)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	f.tribute = int(id)
	f.exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast,
		eqprequire, spellname, spelldesc) VALUES (?, 1, 0, 0, 0, 0, 0, 0, 456, 0, 0, 0, ?, '')`, metalFavorSpell, entity.DuelistsFavorName)
	f.exec(`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (?, 'Self, Increase Size by 50%, 60min', 0, 'test')`, metalFavorSpell)
	f.exec(`INSERT INTO npcitem (npcid, stackleft, remainHP, patternid, listnumber) VALUES (?, 1, 0, ?, 0)`, metalOrigNPCID, f.tribute)
	f.w = entity.NewWorld()
	f.mgr = session.NewManager()
	f.w.SessionManager = f.mgr
	f.moogie = &entity.NPC{ServerID: 7777, Name: entity.MoogieName, X: 101, Y: 54, Z: 100, Level: 80, HP: 1000, MaxHP: 1000, NPCType: 0x80}
	f.orig = &entity.NPC{ServerID: 7778, Name: entity.MetalMoogieName, NPCID: metalOrigNPCID, X: 101, Y: 54, Z: 130, Level: 60,
		HP: 49968, MaxHP: 49968, SpawnOnly: true}
	for _, n := range []*entity.NPC{f.moogie, f.orig} {
		n.SpawnX, n.SpawnY, n.SpawnZ = n.X, n.Y, n.Z
		f.w.NPCs[n.ServerID] = n
		f.w.SpatialIndex.Insert(n.ServerID, n.X, n.Z)
	}
	f.ih = NewInteractHandler(f.mgr)
	f.ih.SetGameWorld(f.w)
	f.ih.favor = NewMoogieFavor(f.inv, database.NewSpellbookStore(db))
	f.windows = NewNPCWindowHandler(f.mgr, f.inv)
	f.windows.SetGameWorld(f.w)
	return f
}

func (f *metalFixture) exec(q string, args ...any) {
	f.t.Helper()
	if _, err := f.db.Exec(q, args...); err != nil {
		f.t.Fatalf("%s: %v", q, err)
	}
}

// join puts a character in the world at (x, z), its pack read from the database.
func (f *metalFixture) join(name string, x, z float32) (*session.Session, *entity.Player) {
	f.t.Helper()
	f.next++
	rec := &database.CharacterRecord{CharName: name, AccountID: 1, Level: 60}
	if err := database.NewSQLiteCharacterStore(f.db).Create(rec); err != nil {
		f.t.Fatal(err)
	}
	sess := f.mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 8, byte(f.next)), Port: 7100 + f.next}, uint32(f.next), 1, 1, 2, true)
	p := &entity.Player{ServerID: entity.EntityID(2000 + f.next), CharServerID: rec.ServerID, Name: name, X: x, Y: 54, Z: z, Level: 60,
		HP: 5000, MaxHP: 5000, Size: entity.PlayerSize, Tunar: 100}
	f.w.Players[p.ServerID] = p
	f.w.SpatialIndex.Insert(p.ServerID, x, z)
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	sess.SetPlayerObjectID(0x15000000 + uint32(p.ServerID))
	f.loadPack(sess, p)
	return sess, p
}

// loadPack is the session's copy of the pack, as its MemoryDump makes it.
func (f *metalFixture) loadPack(sess *session.Session, p *entity.Player) {
	f.t.Helper()
	rows, err := f.inv.GetCharacterItems(p.CharServerID)
	if err != nil {
		f.t.Fatal(err)
	}
	var pack []database.CharInventoryEntry
	for _, r := range rows {
		if r.Location == 0 {
			pack = append(pack, r)
		}
	}
	sess.SetInventoryList(newInvList(pack))
}

// give puts a Moogie Tribute in the player's pack.
func (f *metalFixture) give(sess *session.Session, p *entity.Player) {
	f.t.Helper()
	f.exec(`INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
		VALUES (?, 1, 0, 0, ?, -1, 0, 0)`, p.CharServerID, f.tribute)
	f.loadPack(sess, p)
}

// held is how many tributes the player's rows hold.
func (f *metalFixture) held(p *entity.Player) int {
	f.t.Helper()
	var n int
	if err := f.db.QueryRow(`SELECT count(*) FROM charinventory WHERE serverid = ? AND patternid = ?`, p.CharServerID, f.tribute).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

// see puts an entity on the session's first free slot; its object id.
func (f *metalFixture) see(sess *session.Session, id entity.EntityID) uint32 {
	vs := entity.GetOrCreateVisibility(sess)
	for i := range vs.ChannelEntity {
		if vs.ChannelEntity[i] == id {
			return vs.ChannelObjectID[i]
		}
		if vs.ChannelEntity[i] == 0 {
			vs.ChannelEntity[i], vs.ChannelObjectID[i] = id, 0x00300000+uint32(id)
			return vs.ChannelObjectID[i]
		}
	}
	f.t.Fatal("no slot")
	return 0
}

// talk is the player's 0x0004 to n, and what it was sent.
func (f *metalFixture) talk(sess *session.Session, n *entity.NPC) []string {
	f.t.Helper()
	metalMsgs(sess)
	if err := f.ih.HandleInteract(sess, incoming(protocol.OpcodeInteract, u32b(f.see(sess, n.ServerID)))); err != nil {
		f.t.Fatal(err)
	}
	return metalMsgs(sess)
}

// pick is the player's 0x0035 answer, and what it was sent.
func (f *metalFixture) pick(sess *session.Session, box uint32, option byte) []string {
	f.t.Helper()
	if err := f.ih.HandleDialogueOption(sess, incoming(protocol.OpcodeDialogueBoxOption, append(u32b(box), option))); err != nil {
		f.t.Fatal(err)
	}
	return metalMsgs(sess)
}

// standing is the Metal Moogie standing (a copy, not the original), nil none.
func (f *metalFixture) standing() *entity.NPC {
	for _, n := range f.w.NPCs {
		if n.Name == entity.MetalMoogieName && !n.SpawnOnly && !n.Dead {
			return n
		}
	}
	return nil
}

// metalMsgs is portalRig's decoding of what s was sent.
func metalMsgs(s *session.Session) []string {
	var r *portalRig
	return r.msgs(s)
}

// Mr. Moogie's box keeps his own answers and adds Jibcro's "I'd like to prove my worth" (while his
// metal twin's original is on the server; the control: without it, no such answer). Picked, Mr.
// Moogie says so and a Metal Moogie stands; Cora's ask while he stands is refused with whom he duels;
// Anna may not have Mr. Moogie's help while she duels, and a talk to her Metal Moogie from Cora is
// answered in a line.
func TestMrMoogieOffersToProveWorth(t *testing.T) {
	f := newMetalFixture(t)
	anna, _ := f.join("Anna", 100, 100)
	cora, pc := f.join("Cora", 102, 100)
	got := f.talk(anna, f.moogie)
	want := "|" + strings.Join([]string{moogieClearOpt, moogieHealOpt, moogieResetOpt, moogieProveOpt, moogieNoOpt}, "|")
	if len(got) != 1 || !strings.HasPrefix(got[0], "ask#") || !strings.HasSuffix(got[0], want) {
		t.Fatalf("his talk: %q, want the answers %q", got, want)
	}
	got = f.pick(anna, boxID(got), 3)
	if len(got) != 1 || got[0] != "chat:"+fmt.Sprintf(entity.MetalLineRises, "Anna") {
		t.Errorf("Anna's pick: %q", got)
	}
	c := f.standing()
	if c == nil || c.CurrentEnemy != 0 || !c.DuelSpawn() { // he goes for her at the countdown's FIGHT (petition 1623)
		t.Fatalf("no Metal Moogie standing for Anna: %+v", c)
	}
	got = f.talk(cora, f.moogie)
	got = f.pick(cora, boxID(got), 3)
	if len(got) != 1 || got[0] != "chat:"+fmt.Sprintf(entity.MetalLineBusy, "Anna") {
		t.Errorf("Cora's ask while Anna duels: %q", got)
	}
	if got := f.talk(anna, f.moogie); len(got) != 1 || got[0] != "colour:"+entity.MetalLineDueling {
		t.Errorf("Anna's talk mid-duel: %q", got)
	}
	pc.Z = c.Z - 3 // beside him
	if got := f.talk(cora, c); len(got) != 1 || got[0] != "colour:"+fmt.Sprintf(entity.MetalLineTalk, "Anna") {
		t.Errorf("Cora's talk to Anna's Metal Moogie: %q", got)
	}
	// the control: no original, no such answer
	pc.Z = 100
	f.orig.SpawnOnly = false
	f.orig.Name = "Someone Else"
	got = f.talk(cora, f.moogie)
	want = "|" + strings.Join([]string{moogieClearOpt, moogieHealOpt, moogieResetOpt, moogieNoOpt}, "|")
	if len(got) != 1 || !strings.HasSuffix(got[0], want) || strings.Contains(got[0], moogieProveOpt) {
		t.Errorf("his talk with no metal twin: %q", got)
	}
	for _, o := range []string{moogieProveOpt, moogieTributeOpt} {
		if n := len([]rune(o)); n > questAnswerChars {
			t.Errorf("answer %q is %d characters, a box shows %d", o, n, questAnswerChars)
		}
	}
}

// A Moogie Tribute handed to Mr. Moogie: his box offers the hand-in to one who holds one (the control:
// none held, no such answer); picked, the tribute leaves the pack (its row, and the client's list:
// 0x003C) and his Duelist's Favor lands, its giver half again as big, and he says so. With no favor on
// the server, no hand-in is offered and the tribute stays.
func TestAMoogieTributeForMrMoogiesFavor(t *testing.T) {
	f := newMetalFixture(t)
	anna, pa := f.join("Anna", 100, 100)
	got := f.talk(anna, f.moogie)
	if len(got) != 1 || strings.Contains(got[0], moogieTributeOpt) {
		t.Fatalf("his talk with no tribute held: %q", got)
	}
	f.give(anna, pa)
	got = f.talk(anna, f.moogie)
	want := "|" + strings.Join([]string{moogieClearOpt, moogieHealOpt, moogieResetOpt, moogieProveOpt, moogieTributeOpt, moogieNoOpt}, "|")
	if len(got) != 1 || !strings.HasSuffix(got[0], want) {
		t.Fatalf("his talk with a tribute held: %q, want the answers %q", got, want)
	}
	got = f.pick(anna, boxID(got), 4)
	if !hasMsg(got, fmt.Sprintf("op:%04X", uint16(protocol.OpcodeRemoveInvItem))) || !hasMsg(got, "chat:"+fmt.Sprintf(moogieFavored, "Anna")) {
		t.Errorf("the hand-in sent %q", got)
	}
	if n := f.held(pa); n != 0 {
		t.Errorf("%d tributes left in Anna's rows", n)
	}
	if l := sessionInvList(anna); l.held(f.tribute) != 0 {
		t.Errorf("the client's list still holds the tribute")
	}
	if names := f.w.PlayerEffects(anna); len(names) != 1 || !strings.Contains(fmt.Sprint(names), entity.DuelistsFavorName) {
		t.Errorf("effects after the hand-in: %v", names)
	}
	if _, size := pa.Body(); math.Abs(float64(size-1.5*entity.PlayerSize)) > 1e-6 {
		t.Errorf("Anna's size with the favor: %v", size)
	}
	if got := f.talk(anna, f.moogie); len(got) != 1 || strings.Contains(got[0], moogieTributeOpt) {
		t.Errorf("his talk after the hand-in: %q", got)
	}
	// no favor on the server: no hand-in, the tribute kept
	f.exec(`DELETE FROM spellpattern WHERE spellid = ?`, metalFavorSpell)
	f.give(anna, pa)
	if got := f.talk(anna, f.moogie); len(got) != 1 || strings.Contains(got[0], moogieTributeOpt) {
		t.Errorf("his talk with no favor to give: %q", got)
	}
	if n := f.held(pa); n != 1 {
		t.Errorf("%d tributes held with no favor to give, want 1", n)
	}
}

// His corpse is his challenger's alone: Cora's opening is refused (not lootable, a line saying
// whose), Anna's opens on its single Moogie Tribute (his npcitem row) and nothing else: no coins, no
// loot table's or category's roll.
func TestMetalMoogiesCorpseIsHisChallengers(t *testing.T) {
	f := newMetalFixture(t)
	anna, pa := f.join("Anna", 100, 100)
	cora, _ := f.join("Cora", 102, 100)
	if line, ok := f.w.MetalMoogieChallenge(anna, f.moogie.ServerID, time.Now()); !ok {
		t.Fatalf("no challenge: %q", line)
	}
	c := f.standing()
	c.Dead, c.DeathTime = true, time.Now()
	metalMsgs(anna)
	metalMsgs(cora)
	if !f.windows.OpenCorpse(cora, c) {
		t.Fatal("Cora's opening not taken as a corpse's")
	}
	got := metalMsgs(cora)
	if !hasMsg(got, "colour:"+fmt.Sprintf(entity.MetalLineLoot, "Anna")) || !hasMsg(got, fmt.Sprintf("op:%04X", uint16(protocol.OpcodeLootOptions))) {
		t.Errorf("Cora at Anna's corpse: %q", got)
	}
	tunar := pa.Tunar
	if !f.windows.OpenCorpse(anna, c) {
		t.Fatal("Anna's opening not taken as a corpse's")
	}
	var loot []byte
	for _, m := range anna.Queue().GetPending() {
		if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(uint16(d[0])|uint16(d[1])<<8) == protocol.OpcodeLoot {
			loot = append([]byte(nil), d...)
		}
	}
	if loot == nil {
		t.Fatal("Anna's corpse sent no loot window")
	}
	if n := lootCount(t, loot); n != 1 {
		t.Errorf("his corpse holds %d records, want the one tribute", n)
	}
	if pa.Tunar != tunar {
		t.Errorf("his corpse gave coins: %d -> %d", tunar, pa.Tunar)
	}
}
