package handlers

import (
	"encoding/binary"
	"fmt"
	"net"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The vampires' line in the handlers (entity/vampire.go): the blood a cast
// costs, the bats' infection, the coffins.

// bloodCaster is a level 30 character in the world with Sanctuary's record
// (cast 3, a self spell costing two Units of Blood) on bar slot 6, a plain
// self spell on slot 7, and blood stacks of the sizes given, its session's
// list holding them.
type bloodCaster struct {
	t     *testing.T
	db    *database.SQLiteDB
	inv   *database.InventoryStore
	w     *entity.World
	s     *session.Session
	p     *entity.Player
	r     *Registry
	blood int
	sid   uint32
}

func newBloodCaster(t *testing.T, stacks ...int) *bloodCaster {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/blood.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	rec := &database.CharacterRecord{CharName: "Afkqq", AccountID: 1}
	if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		VALUES (1823, 30, 0, 0, 16528, 3, 0, -1809115846, 2036326070, 0, 3600, -1, 'Sanctuary', 'Vampire Teleport to The Sanctuary.')`)
	exec(`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (1823, 'Self, Increase AC by 1, 1min, Reagent Unit of Blood x2', 0, 'test')`)
	exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		VALUES (900, 1, 0, 0, 16528, 1, 0, -1809115846, 2036326070, 0, 1, -1, 'Plain', '')`)
	exec(`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (900, 'Self, Increase AC by 1, 1min', 0, 'test')`)
	for _, r := range [][2]int{{1823, 6}, {900, 7}} {
		exec(`INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide) VALUES (?, ?, ?, 1, ?, 0, -1)`,
			rec.ServerID, r[0], r[1], r[1])
	}
	res, err := db.Exec(`INSERT INTO itempattern (itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, 1778128471, 0, -1, 0, 1, 0, 0, 0, 0, 0, 1, 30, 0, 50, 32767, 1023, 0, 0, 0, 0, 'Unit of Blood', '', 0, 0)`)
	if err != nil {
		t.Fatal(err)
	}
	blood, _ := res.LastInsertId()
	inv := database.NewInventoryStore(db)
	for _, n := range stacks {
		if _, err := inv.AddCharItem(rec.ServerID, int(blood), n, 0, 0, 0); err != nil {
			t.Fatal(err)
		}
	}

	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 7001}, 1, 1, 1, 2, true)
	p := &entity.Player{ServerID: 1001, CharServerID: rec.ServerID, Name: "Afkqq", Class: 9, Level: 30, HP: 900, MaxHP: 900,
		Power: 500, MaxPower: 500, X: 100, Y: 10, Z: 100, Vampire: true}
	w.Players[p.ServerID] = p
	s.SetECSEntity(p)
	s.SetCharacterInWorld(true)
	s.SetPlayerObjectID(0x284AEFD9)
	items, err := inv.GetCharacterItems(rec.ServerID)
	if err != nil {
		t.Fatal(err)
	}
	s.SetInventoryList(newInvList(items))

	h := NewSpellbookHandler(w, database.NewSpellbookStore(db))
	h.SetInventory(inv)
	r := NewRegistry()
	h.RegisterHandlers(r)
	return &bloodCaster{t: t, db: db, inv: inv, w: w, s: s, p: p, r: r, blood: int(blood), sid: rec.ServerID}
}

// use is the player's 0x0032 for the bar slot, at itself; the messages it
// was sent, in words: "cast" (0x00B1), "take:<n>" (0x003C), "red:<text>"
// (0x0A7B), else the opcode.
func (c *bloodCaster) use(slot byte) []string {
	c.t.Helper()
	payload := build(protocol.OpcodeUseSkill, 16, func(wr *buffer.Writer) {
		wr.WriteUint32(0)
		wr.WriteByte(slot)
		wr.WriteUint32(0x284AEFD9)
	})[2:]
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeUseSkill)
	msg.SetData(payload)
	fn, ok := c.r.Get(protocol.OpcodeUseSkill)
	if !ok {
		c.t.Fatal("no handler for 0x0032")
	}
	if err := fn(c.s, msg); err != nil {
		c.t.Fatal(err)
	}
	var out []string
	for _, m := range c.s.Queue().GetPending() {
		d := m.Data()
		r := buffer.NewReader(d[2:])
		switch op := protocol.GameOpcode(binary.LittleEndian.Uint16(d)); op {
		case protocol.OpcodeCastingCombat:
			out = append(out, "cast")
		case protocol.OpcodeRemoveInvItem:
			r.ReadByte()
			r.ReadByte()
			n, _ := r.Read7BitEncodedInt64()
			out = append(out, fmt.Sprintf("take:%d", n))
		case protocol.OpcodeColoredChat:
			n, _ := r.ReadUint32()
			u := make([]uint16, n)
			for i := range u {
				u[i], _ = r.ReadUint16()
			}
			out = append(out, "red:"+string(utf16.Decode(u)))
		default:
			out = append(out, fmt.Sprintf("op:%04X", uint16(op)))
		}
	}
	return out
}

// held is the blood the character carries, and the session's list's.
func (c *bloodCaster) held() (db, list int) {
	c.t.Helper()
	c.db.QueryRow(`SELECT COALESCE(SUM(stackleft), 0) FROM charinventory WHERE serverid = ? AND patternid = ?`, c.sid, c.blood).Scan(&db)
	for _, r := range sessionInvList(c.s).recs {
		if r.patternID == c.blood {
			list += r.qty
		}
	}
	return db, list
}

// A cast that costs blood begins only while the vampire carries enough, and
// takes it as it begins: two of 1, 2 and 2, from the first stacks first,
// told by 0x003C. A cast the World lets go for its own reasons (Sanctuary
// again at once, the first still casting) takes none. With one left it is
// refused in red and takes nothing. The control: a spell that costs nothing
// begins with no blood at all.
func TestACastThatCostsBloodTakesIt(t *testing.T) {
	c := newBloodCaster(t, 1, 2, 2)
	m := c.use(6)
	if !slices.Contains(m, "cast") {
		t.Fatalf("with five, Sanctuary did not begin: %q", m)
	}
	if n := strings.Count(strings.Join(m, " "), "take:1"); n != 2 {
		t.Errorf("two from stacks of 1, 2 and 2: %q, want two 0x003C of one", m)
	}
	if db, list := c.held(); db != 3 || list != 3 {
		t.Fatalf("after the cast: %d held, %d on the list; want 3", db, list)
	}
	m = c.use(6) // still casting: the World lets it go unanswered
	if db, _ := c.held(); db != 3 || len(m) != 0 {
		t.Errorf("a cast the World let go took blood (%d left) or said %q", db, m)
	}
	c.p.Lockouts, c.p.Casting = nil, nil
	if m = c.use(6); !slices.Contains(m, "cast") {
		t.Fatalf("with three, the second Sanctuary did not begin: %q", m)
	}
	if db, list := c.held(); db != 1 || list != 1 {
		t.Fatalf("after the second cast: %d held, %d on the list; want 1", db, list)
	}
	c.p.Lockouts, c.p.Casting = nil, nil
	m = c.use(6)
	if !slices.Equal(m, []string{"red:Sanctuary needs 2 Units of Blood; you have 1."}) {
		t.Errorf("with one: %q, want the refusal alone", m)
	}
	if db, list := c.held(); db != 1 || list != 1 {
		t.Errorf("a refused cast took blood: %d held, %d listed", db, list)
	}

	k := newBloodCaster(t)
	m = k.use(7)
	if !slices.Contains(m, "cast") {
		t.Errorf("the control: a spell costing nothing did not begin with no blood: %q", m)
	}
}

// The bats' kill step's vampire event makes the character a vampire: a
// charvampire row of source quest, and the player in the world burning by
// day at once. The control: the same quest without the event leaves the
// character as it was.
func TestTheBatsMakeAVampire(t *testing.T) {
	for _, withEvent := range []bool{true, false} {
		f := newQuestFixture(t)
		events := ""
		if withEvent {
			events = `[{"on":"done","do":"vampire"}]`
		}
		f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (95, 'Razeal Has Won', 1, 'Malsis')`)
		f.exec(`INSERT INTO queststeps (questid, step, kind, target, count, journal, events) VALUES (95, 1, 'kill', 'Desmodus bat', 2, 'Hunt desmodus bats.', ?)`, events)
		f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (95, 2, 'talk', 'Grindal', 'Have Grindal look you over.')`)
		f.qe.Reload()
		if _, err := f.qe.SetCharacterQuest(f.sid, 95, QuestGive, 0); err != nil {
			t.Fatal(err)
		}
		f.killFor("Desmodus bat", 0)
		if v, _ := f.db.IsVampire(f.sid); v || f.tw.p.Vampire {
			t.Fatalf("one bat of two made a vampire")
		}
		f.killFor("desmodus bat", 0)
		if s, _ := f.progress(95); s != 2 {
			t.Fatalf("two bats killed, the step is %d", s)
		}
		v, _ := f.db.IsVampire(f.sid)
		var source string
		f.db.QueryRow(`SELECT source FROM charvampire WHERE serverid = ?`, f.sid).Scan(&source)
		if withEvent && (!v || source != "quest" || !f.tw.p.Vampire) {
			t.Errorf("the bats: kept %v (source %q), in the world %v; want a vampire of the quest's", v, source, f.tw.p.Vampire)
		}
		if !withEvent && (v || f.tw.p.Vampire) {
			t.Errorf("the control: no event, and the character is a vampire (kept %v, in the world %v)", v, f.tw.p.Vampire)
		}
	}
}

// A coffin opens for a vampire alone: Freeport's takes it to the Sanctuary,
// where Sandstorm's spell lands, exactly; the Sanctuary's lists the coffins
// back. To a mortal a coffin is a coffin: no trip, no word of one.
func TestACoffinOpensForVampiresAlone(t *testing.T) {
	tw := newTalkWorld(t, 25516.8, 15532)
	coffin := tw.add(1, &entity.NPC{ServerID: 417, Name: "coffin", NPCType: 384, SpawnX: 25516.8, SpawnY: 54.1, SpawnZ: 15529.8})

	tw.talk(coffin)
	for _, m := range questMsgs(tw.s) {
		if strings.HasPrefix(m, "box:") || strings.HasPrefix(m, "ask#") {
			t.Fatalf("a mortal at a coffin was told %q", m)
		}
	}
	tw.pick(coffin, 0)
	if _, _, _, moved := teleported(tw.s); moved {
		t.Fatal("a mortal went through a coffin")
	}

	tw.p.Vampire = true
	tw.talk(coffin)
	if got := optionBox(t, tw.s); !slices.Equal(got, []string{"Yes", "No"}) {
		t.Fatalf("a vampire was offered %q, want Yes and No", got)
	}
	tw.pick(coffin, 0)
	x, z, world, moved := teleported(tw.s)
	if !moved || world != 5 || x != sanctuaryLanding.X || z != sanctuaryLanding.Z {
		t.Errorf("the coffin sent the vampire to (%.2f, %.2f) world %d (%v), want the Sanctuary's landing", x, z, world, moved)
	}

	tw.s.Queue().GetPending()
	tw.p.World, tw.p.X, tw.p.Y, tw.p.Z = 5, sanctuaryLanding.X, sanctuaryLanding.Y, sanctuaryLanding.Z
	home := tw.add(2, &entity.NPC{ServerID: 9001, Name: "Sanctuary coffin", World: 5, NPCType: 384, SpawnX: 4323.72, SpawnY: 273.62, SpawnZ: 6183.48})
	dests, _, _ := wayDests(tw.w, tw.s, "Sanctuary coffin", tw.p)
	var names []string
	for _, d := range dests {
		names = append(names, d.Name)
	}
	if !slices.Equal(names, []string{"Freeport", "Collonridge Cemetery", "Solusek's Eye"}) {
		t.Errorf("the Sanctuary's coffin lists %q", names)
	}
	tw.talk(home)
	if len(tw.s.Queue().PeekPending()) == 0 {
		t.Error("the Sanctuary's coffin said nothing to a vampire")
	}
	tw.p.Vampire = false
	if dests, _, _ := wayDests(tw.w, tw.s, "Sanctuary coffin", tw.p); len(dests) != 0 {
		t.Errorf("the Sanctuary's coffin offers a mortal %d trips", len(dests))
	}
}
