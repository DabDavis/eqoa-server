package entity

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// creatureRead is a creature's entity channel as a CLIENT reads it, field by
// field in its own order (VIClient::ChannelEntity's header, then
// VICreatureEntity::ReadUpdate): the disc's (decomp vicreatureentity.cpp,
// 0x00CC2DB0) or the live game's (eqoa-live-1500, Corsten's 0x013D2870).
type creatureRead struct {
	head      []byte // kind, id, type, VIEntity::ReadUpdate and the creature's walk state, items and armour sets: bytes 0-98 of both
	tints     []byte // the disc's six
	robeTint  byte   // the disc's
	itemCol   [3]uint32
	armorCol  [6]uint32
	variant   [6]byte
	robeCol   uint32
	hair      []byte // colour, length, style, face, robe
	tail      []byte // p294 .. p299: the 47 bytes after the robe (and its tint)
	magic     uint32
	read, all int
}

// readCreature reads b as the live game's CLIENT reads it when live, else as
// the disc's: a sequential read, as the code does it, not a table of offsets.
func readCreature(b []byte, live bool) creatureRead {
	pos := 0
	take := func(n int) []byte {
		v := b[pos : pos+n]
		pos += n
		return v
	}
	rgba := func() uint32 { return binary.BigEndian.Uint32(take(4)) } // four single-byte reads, r first
	var c creatureRead
	c.all = len(b)
	c.head = take(99) // header 6, VIEntity 34, the creature's 59 through m_armorSet
	if live {
		for i := range c.itemCol {
			c.itemCol[i] = rgba() // +0x296+4i
		}
		for i := 0; i < 6; i++ {
			c.armorCol[i] = rgba()    // +0x2A2+4i
			c.variant[i] = take(1)[0] // +0x2BE+i
		}
		c.robeCol = rgba() // +0x2BA
		c.hair = take(5)   // +0x288..+0x28C
	} else {
		c.tints = take(6) // +0x288
		c.hair = take(5)  // +0x28E..+0x292
		c.robeTint = take(1)[0]
	}
	c.tail = take(47) // p294 (4), p298, name 24, level, p255, p256, p257, race, class, npc flags 2, p25C 4, p260, p261, flags262, p263, p264, p299
	c.magic = binary.LittleEndian.Uint32(take(4))
	c.read = pos
	return c
}

func dyedPlayer() *Player {
	return &Player{
		ServerID: 7, Name: "Tinctoria", Level: 42, Race: 3, Class: 13, Sex: 1,
		HairColor: 1, HairLength: 2, HairStyle: 3, FaceOption: 2, Robe: 2, RobeWorn: true,
		Primary: 0x11223344, Secondary: 0x55667788, PrimaryType: 3,
		Chest: 4, Bracer: 4, Gloves: 3, Legs: 2, Boots: 1, Helm: 4,
		X: 100, Y: 20, Z: 300, Size: 1, MaxHP: 100, HP: 50,
	}
}

// The live channel is the live CLIENT's: read field by field as its
// ReadUpdate reads, it is 201 bytes ending in the magic, every field the
// disc's channel carries the same, the dyed slots their dyes and the undyed
// their tint's colour (a player's armour tint 0, white; its robe tint, the
// byte the disc's channel carries, Tints[1] for a woman), each held item
// white and every ArmorGroup variant 0. The control: the disc's reader finds
// the disc's channel whole, 162 bytes.
func TestLiveChannelIsTheLiveClients(t *testing.T) {
	p := dyedPlayer()
	p.Dyes[database.DyeChest] = 0xD10000FF
	p.Dyes[database.DyeBoots] = 0x5A5A78FF
	p.Dyes[database.DyePrimary] = 0x0000FFFF
	disc := SerializePlayerUpdate(100001, p)
	live := LiveChannel(disc, &p.Dyes)

	d := readCreature(disc, false)
	if d.read != EntityStateSize || d.all != EntityStateSize || d.magic != 0x71727374 {
		t.Fatalf("the control: the disc's reader read %d of %d, magic %#x", d.read, d.all, d.magic)
	}
	l := readCreature(live, true)
	if l.read != LiveEntityStateSize || l.all != LiveEntityStateSize || l.magic != 0x71727374 {
		t.Fatalf("the live reader read %d of %d bytes, magic %#x", l.read, l.all, l.magic)
	}
	if !bytes.Equal(l.head, d.head) || !bytes.Equal(l.hair, d.hair) || !bytes.Equal(l.tail, d.tail) {
		t.Fatalf("a field the disc's channel carries differs:\nhead %x\n     %x\nhair %x / %x\ntail %x\n     %x", l.head, d.head, l.hair, d.hair, l.tail, d.tail)
	}
	if name := string(bytes.TrimRight(l.tail[5:29], "\x00")); name != "Tinctoria" || l.tail[29] != 42 || l.tail[33] != 3 || l.tail[34] != 13 {
		t.Fatalf("name %q level %d race %d class %d", name, l.tail[29], l.tail[33], l.tail[34])
	}
	wantArmor := [6]uint32{0xD10000FF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0x5A5A78FF, 0xFFFFFFFF}
	if l.armorCol != wantArmor || l.variant != [6]byte{} {
		t.Fatalf("armour colours %08x variants %v, want %08x and none", l.armorCol, l.variant, wantArmor)
	}
	if l.itemCol != [3]uint32{0x0000FFFF, 0xFFFFFFFF, 0xFFFFFFFF} {
		t.Fatalf("held items' colours %08x", l.itemCol)
	}
	if d.robeTint != 1 || l.robeCol != database.RGBA(database.DiscTints[1]) {
		t.Fatalf("an undyed robe: the disc's tint %d, the live colour %08x (want the tint's, %08x)", d.robeTint, l.robeCol, database.RGBA(database.DiscTints[1]))
	}
	for i, tint := range d.tints {
		if p.Dyes[i] == 0 && l.armorCol[i] != TintRGBA(tint) {
			t.Fatalf("undyed slot %d: %08x for tint %d", i, l.armorCol[i], tint)
		}
	}
	p.Dyes[database.DyeRobe] = 0x008221FF
	if l = readCreature(LiveChannel(SerializePlayerUpdate(100001, p), &p.Dyes), true); l.robeCol != 0x008221FF {
		t.Fatalf("a dyed robe: %08x", l.robeCol)
	}
}

// The live channel at the offsets an independent writer of it puts them:
// Return Home's C# server (Entity.Update.cs, which writes the live game's
// 201 bytes, its Span one before ours): ChestColor 110 big-endian and each
// next slot 5 on, RobeColor 140, hair 144-146, face 147, robe 148, name 154,
// level 178, race 182, class 183, "trsq" 196.
func TestLiveChannelAtTheReferenceServersOffsets(t *testing.T) {
	p := dyedPlayer()
	for i := range 6 {
		p.Dyes[i] = 0x10203000 | uint32(i+1)
	}
	p.Dyes[database.DyeRobe] = 0xAABBCCDD
	b := LiveChannel(SerializePlayerUpdate(100001, p), &p.Dyes)
	at := func(cs int) []byte { return b[cs+1:] } // C# Span index -> ours
	for i := range 6 {
		if got := binary.BigEndian.Uint32(at(110 + 5*i)); got != p.Dyes[i] {
			t.Fatalf("armour slot %d at C# %d: %08x", i, 110+5*i, got)
		}
	}
	if binary.BigEndian.Uint32(at(140)) != 0xAABBCCDD {
		t.Fatalf("robe colour at C# 140: %x", at(140)[:4])
	}
	if !bytes.Equal(at(144)[:5], []byte{1, 2, 3, 2, 2}) {
		t.Fatalf("hair, face, robe at C# 144: %v", at(144)[:5])
	}
	if !bytes.HasPrefix(at(154), []byte("Tinctoria\x00")) || at(178)[0] != 42 || at(182)[0] != 3 || at(183)[0] != 13 {
		t.Fatalf("name, level, race, class at C# 154/178/182/183: %q %d %d %d", at(154)[:10], at(178)[0], at(182)[0], at(183)[0])
	}
	if string(at(196)[:4]) != "tsrq" {
		t.Fatalf("the magic at C# 196: %q", at(196)[:4])
	}
}

// An NPC has no dyes: its live channel carries its tints' colours (0, white)
// and a robe in its tint's.
func TestLiveChannelOfAnNPC(t *testing.T) {
	n := &NPC{ServerID: 9, Name: "Guard Serenda", Level: 35, ModelID: 0x70D898C6, Size: 1, HP: 1, MaxHP: 1,
		Torso: 6, Forearm: 6, NPCGloves: 6, NPCLegs: 6, Feet: 6, Head: 7, RobeStyle: "1"}
	l := readCreature(LiveChannel(SerializeNPCUpdate(100002, n), nil), true)
	for i, c := range l.armorCol {
		if c != 0xFFFFFFFF {
			t.Fatalf("slot %d: %08x", i, c)
		}
	}
	if l.robeCol != 0xFFFFFFFF || l.itemCol != [3]uint32{0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF} || l.hair[4] != 1 {
		t.Fatalf("robe %08x items %08x robe style %d", l.robeCol, l.itemCol, l.hair[4])
	}
}

// The live record (0x002C) as the live CLIENT's CharacterList (0x012CC018)
// reads the raw part: the robe, the held items, their attack types, the
// armour sets, six variants, then the colours; the disc's robe tint gone.
func TestLiveGear(t *testing.T) {
	disc := make([]byte, DiscGearSize)
	binary.LittleEndian.PutUint32(disc[0:], 2)           // robe 2
	disc[4] = 3                                          // its tint
	binary.LittleEndian.PutUint32(disc[5:], 0x11223344)  // primary
	binary.LittleEndian.PutUint32(disc[13:], 0x99AABBCC) // shield
	disc[17], disc[18] = 5, 1
	copy(disc[20:26], []byte{4, 0xFF, 3, 2, 1, 4})
	disc[26+3] = 6 // legs in tint 6
	var d Dyes
	d[database.DyeChest] = 0xA50501FF
	d[database.DyeShield] = 0xF03C3CFF
	g := LiveGear(disc, &d)
	if len(g) != LiveGearSize {
		t.Fatalf("%d bytes", len(g))
	}
	if binary.LittleEndian.Uint32(g[0:]) != 2 || binary.LittleEndian.Uint32(g[4:]) != 0x11223344 || binary.LittleEndian.Uint32(g[12:]) != 0x99AABBCC {
		t.Fatalf("robe and held items: %x", g[:16])
	}
	if !bytes.Equal(g[16:19], []byte{5, 1, 0}) || !bytes.Equal(g[19:25], []byte{4, 0xFF, 3, 2, 1, 4}) || !bytes.Equal(g[25:31], make([]byte, 6)) {
		t.Fatalf("attack types %v, sets %v, variants %v", g[16:19], g[19:25], g[25:31])
	}
	col := func(o int) uint32 { return binary.BigEndian.Uint32(g[o:]) }
	if col(31) != 0xFFFFFFFF || col(39) != 0xF03C3CFF {
		t.Fatalf("held items' colours %08x %08x", col(31), col(39))
	}
	if col(43) != 0xA50501FF || col(43+4) != 0xFFFFFFFF || col(43+12) != database.RGBA(database.DiscTints[6]) {
		t.Fatalf("armour colours %x", g[43:67])
	}
	if col(67) != database.RGBA(database.DiscTints[3]) {
		t.Fatalf("an undyed robe in its tint's colour: %08x", col(67))
	}
}

func lookSession(t *testing.T, n byte) *session.Session {
	t.Helper()
	sess := session.NewManager().CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, n), Port: 7000 + int(n)}, uint32(n), uint32(n), 1, 2, true)
	sess.SetCharacterInWorld(true)
	return sess
}

// A client of the disc's look (every PS2's, Sandstorm's) is handed the very
// buffer the serializer wrote, on a channel that stays 162 bytes; one of the
// live look the live game's 201, and back to 162 when it goes back: a
// channel takes the length of what it is given.
func TestTheLookFollowsTheClient(t *testing.T) {
	p := dyedPlayer()
	p.Dyes[database.DyeChest] = 0xD10000FF
	disc := SerializePlayerUpdate(100001, p)
	ps2 := lookSession(t, 1)
	if got := ForViewer(ps2, disc, &p.Dyes); &got[0] != &disc[0] {
		t.Fatal("a PS2's buffer is not the serializer's own")
	}
	if got := ForViewer(nil, disc, &p.Dyes); len(got) != EntityStateSize {
		t.Fatal("no session: not the disc's")
	}
	ch := session.NewStateUpdateData(EntityStateSize)
	SetEntityState(ch, ForViewer(ps2, disc, &p.Dyes))
	if ch.Len() != EntityStateSize || !bytes.Equal(ch.GetData(), disc) {
		t.Fatalf("a PS2's channel: %d bytes", ch.Len())
	}

	live := lookSession(t, 2)
	live.SetLiveLook(true)
	SetEntityState(ch, ForViewer(live, disc, &p.Dyes))
	if ch.Len() != LiveEntityStateSize || binary.BigEndian.Uint32(ch.GetData()[111:]) != 0xD10000FF {
		t.Fatalf("the live look: %d bytes, chest %x", ch.Len(), ch.GetData()[111:115])
	}
	live.SetLiveLook(false)
	SetEntityState(ch, ForViewer(live, disc, &p.Dyes))
	if ch.Len() != EntityStateSize || !bytes.Equal(ch.GetData(), disc) {
		t.Fatalf("back to the disc's: %d bytes", ch.Len())
	}
}

// Through the world: a tick puts the player's own channel in its client's
// look and every other player in view in each viewer's; a dye set where the
// player stands reaches a viewer of the live look at the next tick, never a
// PS2. The control: the PS2's channels are exactly the serializers' bytes.
func TestTheWorldSendsEachClientItsLook(t *testing.T) {
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	add := func(n byte, name string, x float32, liveLook bool) (*session.Session, *Player) {
		sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 1, n), Port: 7100 + int(n)}, uint32(n), uint32(n), 1, 2, true)
		sess.SetCharacterInWorld(true)
		sess.SetPlayerObjectID(uint32(n))
		sess.SetLiveLook(liveLook)
		p := dyedPlayer()
		p.ServerID, p.CharServerID, p.Name, p.X = EntityID(n), uint32(n), name, x
		w.Players[p.ServerID] = p
		w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
		sess.SetECSEntity(p)
		return sess, p
	}
	ps2, _ := add(1, "Plainjane", 100, false)
	van, vp := add(2, "Brightcoat", 102, true)
	w.TickCount = 0
	w.visibilitySystem()
	w.stateSyncSystem()
	w.entityChannelSyncSystem()

	own := func(s *session.Session) []byte { return s.Connection().GetServerObject(0).GetData() }
	other := func(s *session.Session) []byte { return s.Connection().GetServerObject(1).GetData() }
	if len(own(ps2)) != EntityStateSize || len(other(ps2)) != EntityStateSize {
		t.Fatalf("the PS2's channels: %d and %d bytes", len(own(ps2)), len(other(ps2)))
	}
	if len(own(van)) != LiveEntityStateSize || len(other(van)) != LiveEntityStateSize {
		t.Fatalf("the live look's channels: %d and %d bytes", len(own(van)), len(other(van)))
	}
	if _, ok := w.SetDyes(2, Dyes{database.DyeLegs: 0x400080FF}); !ok {
		t.Fatal("SetDyes: not in the world")
	}
	if vp.Dyes[database.DyeLegs] != 0x400080FF {
		t.Fatal("the dye is not on the player")
	}
	ps2Before := append([]byte(nil), other(ps2)...)
	w.stateSyncSystem()
	w.entityChannelSyncSystem()
	legs := func(b []byte) uint32 { return binary.BigEndian.Uint32(b[111+5*database.DyeLegs:]) }
	if legs(own(van)) != 0x400080FF {
		t.Fatalf("its own channel's legs: %08x", legs(own(van)))
	}
	if !bytes.Equal(other(ps2), ps2Before) {
		t.Fatal("the PS2's view of the dyed player changed")
	}
	// the PS2 sees Brightcoat exactly as the serializer writes it
	want := w.serializeEntityForChannel(ps2, 2, GetOrCreateVisibility(ps2).ChannelObjectID[0])
	if !bytes.Equal(other(ps2), want) || len(want) != EntityStateSize {
		t.Fatal("the PS2's channel is not the disc's serializer's")
	}
}
