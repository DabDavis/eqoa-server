package handlers

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The GM's channel against the chat world (devcmds_test.go's gmWorld: a GM
// on account 1, a player on account 2), each 0x0701 fed as the client
// writes it.

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// u16le is a wide text's units, as SendText13 writes them after the count.
func u16le(s string) string {
	var b strings.Builder
	for _, u := range utf16.Encode([]rune(s)) {
		b.WriteString(hex.EncodeToString([]byte{byte(u), byte(u >> 8)}))
	}
	return b.String()
}

// The bytes the vanilla client sent a test server (notes-gm.md's runs B, C
// and E), read as the decomp's senders wrote them. The controls: nothing, a
// sub-op no sender writes, a field cut short, a count past the data and one
// past gmTextMax are refused.
func TestGMRequestsAsTheClientWritesThem(t *testing.T) {
	for _, c := range []struct {
		name string
		hex  string
		want gmRequest
	}{
		{"BROADCAST", "0d 11000000" + u16le("Hello from the GM"), gmRequest{Sub: gmSubBroadcast, Text: "Hello from the GM"}},
		{"KILL", "10 a1860100", gmRequest{Sub: gmSubKill, A: 100001}},
		{"PACKET LOSS", "12 19 0000 0000", gmRequest{Sub: gmSubNetSim, Loss: 25}},
		{"LATENCY", "12 19 fa00 0000", gmRequest{Sub: gmSubNetSim, Loss: 25, Latency: 250}},
		{"SERVER LOAD", "13", gmRequest{Sub: gmSubServerLoad}},
		{"/create", "35 0e000000" + hex.EncodeToString([]byte("Sword of Truth")), gmRequest{Sub: gmSubCreate, Text: "Sword of Truth"}},
		{"Buff PC", "02 ffffffff 14000000", gmRequest{Sub: gmSubBuffPC, A: 0xFFFFFFFF, B: 20}},
		{"Goto Location", "06 10000000 30203235333630203534203135373030", gmRequest{Sub: gmSubGotoLocation, Text: "0 25360 54 15700"}},
		{"prompt Yes", "30", gmRequest{Sub: gmSubYes}},
		{"Change Name", "21 a1860100 05000000" + hex.EncodeToString([]byte("Bobby")), gmRequest{Sub: gmSubRename, A: 100001, Text: "Bobby"}},
		{"a Latin-1 name", "09 03000000 4ae973", gmRequest{Sub: gmSubKick, Text: "Jés"}},
	} {
		got, err := parseGMRequest(unhex(t, c.hex))
		if err != nil || got != c.want {
			t.Errorf("%s: %+v, %v; want %+v", c.name, got, err, c.want)
		}
	}
	for _, c := range []struct {
		name string
		hex  string
		want error
	}{
		{"nothing", "", errGMEmpty},
		{"sub-op 26", "1a 01000000", errGMUnknown},
		{"sub-op 0", "00", errGMUnknown},
		{"KILL cut short", "10 a186", errGMShort},
		{"a string's count past it", "06 10000000 3020", errGMShort},
		{"a string past gmTextMax", "06 01040000", errGMShort},
		{"PACKET LOSS cut short", "12 19 fa", errGMShort},
		{"Give Item's second int", "19 01000000", errGMShort},
	} {
		if _, err := parseGMRequest(unhex(t, c.hex)); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
}

// gmSent is every 0x0700 queued for s, its payload after the opcode.
func gmSent(s *session.Session) [][]byte {
	var out [][]byte
	for _, m := range s.Queue().PeekPending() {
		d := m.Data()
		if len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeAdminCode {
			out = append(out, d[2:])
		}
	}
	return out
}

// sentAll is every message of opcode op queued for s, its payload after the
// opcode.
func sentAll(s *session.Session, op protocol.GameOpcode) [][]byte {
	var out [][]byte
	for _, m := range s.Queue().PeekPending() {
		d := m.Data()
		if len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == op {
			out = append(out, d[2:])
		}
	}
	return out
}

// gm sends one 0x0701 from s, its payload as hex.
func (c *chatWorld) gm(s *session.Session, h string) {
	c.t.Helper()
	if err := c.h.HandleGMCommand(s, incoming(protocol.OpcodeGMCommand, unhex(c.t, h))); err != nil {
		c.t.Fatal(err)
	}
}

// gmText is a string field: u32 byte count, the bytes.
func gmText(s string) string {
	b := []byte(s)
	n := make([]byte, 4)
	binary.LittleEndian.PutUint32(n, uint32(len(b)))
	return hex.EncodeToString(n) + hex.EncodeToString(b)
}

// gmU32 is a u32 field.
func gmU32(v uint32) string {
	n := make([]byte, 4)
	binary.LittleEndian.PutUint32(n, v)
	return hex.EncodeToString(n)
}

// gmReplies is the [GM] lines s was sent, then forgotten.
func gmReplies(t *testing.T, s *session.Session) []string {
	t.Helper()
	var out []string
	for _, l := range heard(t, s) {
		out = append(out, l.text)
	}
	s.Queue().GetPending()
	return out
}

func hasLine(lines []string, part string) bool {
	for _, l := range lines {
		if strings.Contains(l, part) {
			return true
		}
	}
	return false
}

// The channel goes to a GM alone, at world entry: 0x0700 {35, zigzag 1},
// then the triggers' names offered (0x7E07, a u32 1). The control: the
// player is sent neither, nor anyone with no levels known.
func TestGMChannelIsAGMs(t *testing.T) {
	c, gs, _, ps, _ := gmWorld(t)
	c.h.SendGMChannel(gs)
	c.h.SendGMChannel(ps)
	if got := gmSent(gs); len(got) != 1 || hex.EncodeToString(got[0]) != "2302" {
		t.Fatalf("the GM's 0x0700: %x", got)
	}
	if got := sentAll(gs, protocol.OpcodeGMTriggers); len(got) != 1 || hex.EncodeToString(got[0]) != "01000000" {
		t.Fatalf("the GM's names offer: %x", got)
	}
	if got := gmSent(ps); len(got) != 0 {
		t.Fatalf("the player's 0x0700: %x", got)
	}
	if got := sentAll(ps, protocol.OpcodeGMTriggers); len(got) != 0 {
		t.Fatalf("the player's names offer: %x", got)
	}
	c.h.SetAccessLevels(nil)
	gs.Queue().GetPending()
	c.h.SendGMChannel(gs)
	if got := gmSent(gs); len(got) != 0 {
		t.Fatalf("with no levels: %x", got)
	}
}

// npcSeen puts an NPC into s's first visibility slot under object id 100001,
// as the visibility system would.
func (c *chatWorld) npcSeen(s *session.Session, npc *entity.NPC) uint32 {
	c.gw.NPCs[npc.ServerID] = npc
	vs := entity.GetOrCreateVisibility(s)
	vs.ChannelEntity[0], vs.ChannelObjectID[0] = npc.ServerID, 100001
	return 100001
}

// A command is a GM's: the player's KILL and HEAL do nothing, the GM's kill
// and heal; a second KILL says it is dead already; a trigger answers the GM
// alone (line 3 is slot 4: with no engine running, it says so), and a
// 0x0702 that is no trigger nobody.
func TestGMCommandsAreAGMsAloneOnTheChannel(t *testing.T) {
	c, gs, _, ps, _ := gmWorld(t)
	npc := &entity.NPC{ServerID: 900, Name: "Orc Pawn", HP: 10, MaxHP: 100, Level: 3}
	id := c.npcSeen(gs, npc)
	entity.GetOrCreateVisibility(ps).ChannelEntity[0] = npc.ServerID
	entity.GetOrCreateVisibility(ps).ChannelObjectID[0] = id

	c.gm(ps, "11"+gmU32(id))
	c.gm(ps, "10"+gmU32(id))
	if npc.HP != 10 || npc.Dead {
		t.Fatalf("a player's HEAL and KILL: hp %d dead %v", npc.HP, npc.Dead)
	}
	if got := gmReplies(t, ps); len(got) != 0 {
		t.Fatalf("a player was answered: %q", got)
	}
	c.gm(gs, "11"+gmU32(id))
	if npc.HP != 100 {
		t.Fatalf("the GM's HEAL: hp %d", npc.HP)
	}
	c.gm(gs, "10"+gmU32(id))
	if !npc.Dead {
		t.Fatal("the GM's KILL left it alive")
	}
	got := gmReplies(t, gs)
	if !hasLine(got, "Healed Orc Pawn") || !hasLine(got, "Killed Orc Pawn") {
		t.Fatalf("the GM's lines: %q", got)
	}
	c.gm(gs, "10"+gmU32(id))
	if got := gmReplies(t, gs); !hasLine(got, "dead already") {
		t.Fatalf("a second KILL: %q", got)
	}

	trig := func(s *session.Session, h string) {
		if err := c.h.HandleGMTrigger(s, incoming(protocol.OpcodeGMTrigger, unhex(t, h))); err != nil {
			t.Fatal(err)
		}
	}
	trig(gs, "6403")
	if got := gmReplies(t, gs); len(got) != 1 || !strings.Contains(got[0], "Trigger 4") {
		t.Fatalf("the GM's trigger: %q", got)
	}
	trig(ps, "6403")
	trig(gs, "6303")
	if got := append(gmReplies(t, ps), gmReplies(t, gs)...); len(got) != 0 {
		t.Fatalf("a player's trigger or a 99: %q", got)
	}
}

// KILL on a player is a death (respawn pending, the debt); HEAL on the GM
// itself brings it to full. The control: HEAL on a dead player does nothing.
func TestGMKillAndHealPlayers(t *testing.T) {
	c, gs, gp, _, pp := gmWorld(t)
	pp.Level = 20
	c.gm(gs, "10"+gmU32(0x7F000000|uint32(pp.ServerID)))
	if !pp.IsDead() || pp.Debt == 0 {
		t.Fatalf("KILL on a player: dead %v debt %d", pp.IsDead(), pp.Debt)
	}
	c.gm(gs, "11"+gmU32(0x7F000000|uint32(pp.ServerID)))
	if got := gmReplies(t, gs); !hasLine(got, "Player is dead") {
		t.Fatalf("HEAL on the dead: %q", got)
	}
	gp.HP, gp.MaxHP = 1, 80
	gs.SetPlayerObjectID(5555) // the id CLIENT's m_selfId holds
	c.gm(gs, "11"+gmU32(gs.PlayerObjectID()))
	if gp.HP != gp.MaxHPNow() {
		t.Fatalf("HEAL on the GM itself: %d of %d", gp.HP, gp.MaxHPNow())
	}
}

// Goto Location: "world x y z" teleports the GM, "x y z" in its own world.
// The control: two numbers or a word move nothing.
func TestGMGotoLocation(t *testing.T) {
	c, gs, _, _, _ := gmWorld(t)
	c.gm(gs, "06"+gmText("0 25360 54 15700"))
	if x, z, w, ok := teleported(gs); !ok || x != 25360 || z != 15700 || w != 0 {
		t.Fatalf("Goto Location: (%v, %v) world %d, %v", x, z, w, ok)
	}
	gs.Queue().GetPending()
	c.gm(gs, "06"+gmText("25370 54 15710"))
	if x, z, _, ok := teleported(gs); !ok || x != 25370 || z != 15710 {
		t.Fatalf("x y z: (%v, %v), %v", x, z, ok)
	}
	for _, bad := range []string{"25360 15700", "0 here 54 15700"} {
		gs.Queue().GetPending()
		c.gm(gs, "06"+gmText(bad))
		if _, _, _, ok := teleported(gs); ok {
			t.Fatalf("%q moved the GM", bad)
		}
	}
}

// Silence and Snoop: the silenced player's say reaches nobody and it is told
// so; Silence again lets it speak (the control: heard before and after). A
// snooping GM gets the player's lines as copies until End Snoop.
func TestGMSilenceAndSnoop(t *testing.T) {
	c, gs, _, ps, pp := gmWorld(t)
	far, _ := c.join(3, "Bystander", 25005, 15005, 0)
	c.say(ps, "before")
	if got := texts(heard(t, far)); len(got) != 1 || got[0] != "Player: before" {
		t.Fatalf("before the silence: %q", got)
	}
	far.Queue().GetPending()
	ps.Queue().GetPending()
	gs.Queue().GetPending()

	c.gm(gs, "0a"+gmText("player"))
	ps.Queue().GetPending()
	c.say(ps, "hush")
	if got := heard(t, far); len(got) != 0 {
		t.Fatalf("a silenced line was heard: %+v", got)
	}
	if got := heard(t, ps); len(got) != 1 || got[0].op != protocol.OpcodeErrorMessage {
		t.Fatalf("the silenced player was not told: %+v", got)
	}
	ps.Queue().GetPending()
	c.gm(gs, "0a"+gmText("Player"))
	c.say(ps, "after")
	if got := texts(heard(t, far)); len(got) != 1 || got[0] != "Player: after" {
		t.Fatalf("after Silence again: %q", got)
	}

	gs.Queue().GetPending()
	c.gm(gs, "22"+gmU32(0x7F000000|uint32(pp.ServerID)))
	gs.Queue().GetPending()
	c.say(ps, "secret")
	if got := gmReplies(t, gs); !hasLine(got, "Snoop: Player (say): secret") {
		t.Fatalf("the snoop's copy: %q", got)
	}
	c.gm(gs, "25")
	gs.Queue().GetPending()
	c.say(ps, "gone")
	if got := gmReplies(t, gs); hasLine(got, "Snoop:") {
		t.Fatalf("a copy after End Snoop: %q", got)
	}
}

// Freeze: the player's speed 0 (told by 0x00F8), then its own again. The
// control: a name no one has changes nothing.
func TestGMFreeze(t *testing.T) {
	c, gs, _, ps, pp := gmWorld(t)
	pp.Speed = 8.8
	speeds := func() []float32 {
		var out []float32
		for _, m := range ps.Queue().PeekPending() {
			d := m.Data()
			if len(d) >= 6 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodePlayerSpeed {
				out = append(out, f32(d[2:]))
			}
		}
		ps.Queue().GetPending()
		return out
	}
	c.gm(gs, "33"+gmText("Nobody"))
	if pp.Frozen || len(speeds()) != 0 {
		t.Fatal("a freeze of nobody froze the player")
	}
	c.gm(gs, "33"+gmText("Player"))
	if got := speeds(); !pp.Frozen || len(got) != 1 || got[0] != 0 {
		t.Fatalf("frozen: %v %v", pp.Frozen, got)
	}
	c.gm(gs, "33"+gmText("Player"))
	if got := speeds(); pp.Frozen || len(got) != 1 || got[0] != 8.8 {
		t.Fatalf("let go: %v %v", pp.Frozen, got)
	}
}

// Kill Spawner holds the NPC dead past its respawn time; Unkill brings it
// back now. The control: an NPC not held comes back on its own. Goto
// Spawner goes to the spawn point, by NPCID.
func TestGMSpawnerHold(t *testing.T) {
	c, gs, _, _, _ := gmWorld(t)
	held := &entity.NPC{ServerID: 901, NPCID: 77, Name: "Held", HP: 50, MaxHP: 50, SpawnX: 25100, SpawnZ: 15100}
	free := &entity.NPC{ServerID: 902, NPCID: 78, Name: "Free", HP: 50, MaxHP: 50}
	c.gw.NPCs[held.ServerID], c.gw.NPCs[free.ServerID] = held, free
	c.gm(gs, "17"+gmU32(77))
	if !held.Dead || !held.GMHeld {
		t.Fatalf("Kill Spawner: dead %v held %v", held.Dead, held.GMHeld)
	}
	c.gw.KillNPCNow(free.ServerID)
	past := time.Now().Add(time.Hour)
	held.DeathTime, free.DeathTime = past.Add(-50*time.Minute), past.Add(-50*time.Minute)
	held.CorpseGone, free.CorpseGone = past.Add(-40*time.Minute), past.Add(-40*time.Minute)
	c.gw.Tick(past)
	if !held.Dead || free.Dead {
		t.Fatalf("past the respawn: held dead %v, free dead %v", held.Dead, free.Dead)
	}
	c.gm(gs, "18"+gmU32(77))
	if held.Dead || held.GMHeld {
		t.Fatalf("Unkill Spawner: dead %v held %v", held.Dead, held.GMHeld)
	}
	gs.Queue().GetPending()
	c.gm(gs, "07"+gmU32(77))
	if x, z, _, ok := teleported(gs); !ok || x != 25100 || z != 15100 {
		t.Fatalf("Goto Spawner: (%v, %v) %v", x, z, ok)
	}
	gs.Queue().GetPending()
	c.gm(gs, "07"+gmU32(4242))
	if _, _, _, ok := teleported(gs); ok {
		t.Fatal("Goto Spawner of no spawner moved the GM")
	}
}

// Remove 1 Debt takes one death's at the player's level, Remove All Debt
// the rest; with no target, the GM's own.
func TestGMDebt(t *testing.T) {
	c, gs, gp, _, pp := gmWorld(t)
	pp.Level, pp.Debt = 20, 3*entity.DeathDebt(20)
	c.gm(gs, "26"+gmU32(0x7F000000|uint32(pp.ServerID)))
	if pp.Debt != 2*entity.DeathDebt(20) {
		t.Fatalf("Remove 1 Debt: %d", pp.Debt)
	}
	c.gm(gs, "27"+gmU32(0x7F000000|uint32(pp.ServerID)))
	if pp.Debt != 0 {
		t.Fatalf("Remove All Debt: %d", pp.Debt)
	}
	gp.Debt = 500
	c.gm(gs, "27"+gmU32(0xFFFFFFFF))
	if gp.Debt != 0 {
		t.Fatalf("no target, the GM's: %d", gp.Debt)
	}
}

// renames is a nameStore: a name another character has is taken.
type renames map[uint32]string

func (r renames) Rename(id uint32, name string) error {
	for other, n := range r {
		if other != id && strings.EqualFold(n, name) {
			return database.ErrNameTaken
		}
	}
	r[id] = name
	return nil
}

// Change Name: a free name is kept and the world takes it at once, the
// first letter up; the controls: a taken name, one too long for the client,
// one with a digit.
func TestGMRename(t *testing.T) {
	c, gs, _, _, pp := gmWorld(t)
	kept := renames{pp.CharServerID: "Player", 99: "Taken"}
	c.h.SetGMStores(nil, kept)
	target := gmU32(0x7F000000 | uint32(pp.ServerID))
	for _, bad := range []string{"taken", "Abcdefghijklm", "Bob2"} {
		c.gm(gs, "21"+target+gmText(bad))
		if pp.Name != "Player" || kept[pp.CharServerID] != "Player" {
			t.Fatalf("%q renamed it: %q / %q", bad, pp.Name, kept[pp.CharServerID])
		}
	}
	c.gm(gs, "21"+target+gmText("bOBBY"))
	if pp.Name != "Bobby" || kept[pp.CharServerID] != "Bobby" {
		t.Fatalf("a rename: %q / %q", pp.Name, kept[pp.CharServerID])
	}
}

// Give Item asks first: the item's name in 0x0700 50; No gives nothing, Yes
// gives one to the target, and an answer with nothing waiting nothing. Summon
// Item and /create give the GM at once; an item no row has, nothing.
func TestGMGiveItemAsksFirst(t *testing.T) {
	c, gs, gp, ps, pp := gmWorld(t)
	db, err := database.NewSQLite(t.TempDir() + "/gm.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	inv := database.NewInventoryStore(db)
	c.h.SetGMStores(inv, nil)
	res, err := db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (5, 0, 0, 0, -1, 0, 0, 1, 0, 0, 0, 0, 1, 20, 0, 0, 32767, 1023, 0, 0, 0, 0, 'Test Fish', '', 0, 0)`)
	if err != nil {
		t.Fatal(err)
	}
	fish, _ := res.LastInsertId()
	gs.SetInventoryList(newInvList(nil))
	ps.SetInventoryList(newInvList(nil))
	held := func(id uint32) int {
		items, err := inv.GetCharacterItems(id)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, e := range items {
			n += e.StackLeft
		}
		return n
	}

	target := gmU32(0x7F000000 | uint32(pp.ServerID))
	c.gm(gs, "19"+target+gmU32(uint32(fish)))
	got := gmSent(gs)
	if len(got) != 1 || got[0][0] != gmSubPrompt {
		t.Fatalf("Give Item's prompt: %x", got)
	}
	if name, _ := readWideLine(got[0][1:]); name != "Test Fish" {
		t.Fatalf("the prompt's name: %q", name)
	}
	if held(pp.CharServerID) != 0 {
		t.Fatal("given before the Yes")
	}
	c.gm(gs, "31")
	if held(pp.CharServerID) != 0 {
		t.Fatal("a No gave it")
	}
	c.gm(gs, "30")
	if held(pp.CharServerID) != 0 {
		t.Fatal("a Yes with nothing waiting gave it")
	}
	c.gm(gs, "19"+target+gmU32(uint32(fish)))
	c.gm(gs, "30")
	if held(pp.CharServerID) != 1 {
		t.Fatalf("a Yes: %d", held(pp.CharServerID))
	}
	if got := texts(heard(t, ps)); !hasLine(got, "A GM gave you Test Fish.") {
		t.Fatalf("the player was not told: %q", got)
	}

	c.gm(gs, "03"+gmU32(uint32(fish)))
	c.gm(gs, "35"+gmText("test fish 3"))
	if held(gp.CharServerID) != 4 {
		t.Fatalf("Summon Item and /create: %d", held(gp.CharServerID))
	}
	gs.Queue().GetPending()
	c.gm(gs, "03"+gmU32(4242))
	if got := gmReplies(t, gs); !hasLine(got, "No item 4242") || held(gp.CharServerID) != 4 {
		t.Fatalf("an item no row has: %q", got)
	}
}

// Toggle Invis hides the GM (Invisible, so the channel's byte 154 and
// aggro go with it) and shows it again; Change Sprite gives it a body and a
// scale, Revert Sprite its own. The control: a sprite that is no number
// changes nothing.
func TestGMInvisAndSprite(t *testing.T) {
	c, gs, gp, _, _ := gmWorld(t)
	c.gm(gs, "01")
	if !gp.Invisible() {
		t.Fatal("Toggle Invis: visible")
	}
	c.gm(gs, "01")
	if gp.Invisible() {
		t.Fatal("Toggle Invis again: hidden")
	}
	own, _ := gp.Body()
	c.gm(gs, "04"+gmText("lizard"))
	if m, _ := gp.Body(); m != own {
		t.Fatalf("a sprite that is no number: %08X", m)
	}
	c.gm(gs, "04"+gmText("0x70D898C6 1.5"))
	if m, s := gp.Body(); m != 0x70D898C6 || s != 1.5 {
		t.Fatalf("Change Sprite: %08X %v", m, s)
	}
	c.gm(gs, "05")
	if m, _ := gp.Body(); m != own {
		t.Fatalf("Revert Sprite: %08X", m)
	}
}
