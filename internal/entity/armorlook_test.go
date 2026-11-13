package entity

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// ornatePlayer wears set 9 on chest, legs and boots and set 4 on its bracer.
func ornatePlayer() *Player {
	p := dyedPlayer()
	p.Chest, p.Bracer, p.Gloves, p.Legs, p.Boots, p.Helm = OrnateSet, 4, 3, OrnateSet, OrnateSet, 4
	return p
}

// Set 9 (the ornate plate off CHAR.ESF) reaches only a client that draws this
// server's armour; a PS2 (whose SetArmorSlot clamps 9 to none, the bare body),
// a channel with no session, and a client of the live look that did not ask
// are sent set 4 in its place, every other set as it was. The serializer's
// own bytes keep 9 (the control: what the world holds is the item's set).
func TestSetNineIsPlainPlateToAClientWithoutIt(t *testing.T) {
	p := ornatePlayer()
	disc := SerializePlayerUpdate(100001, p)
	if got := disc[ChannelArmor : ChannelArmor+6]; !bytes.Equal(got, []byte{9, 4, 3, 9, 9, 4}) {
		t.Fatalf("the serializer's sets: %v", got)
	}
	want := []byte{4, 4, 3, 4, 4, 4}
	ps2 := lookSession(t, 1)
	for name, got := range map[string][]byte{"a PS2": ForViewer(ps2, disc, &p.Dyes), "no session": ForViewer(nil, disc, &p.Dyes)} {
		if len(got) != EntityStateSize || !bytes.Equal(got[ChannelArmor:ChannelArmor+6], want) {
			t.Fatalf("%s: %d bytes, sets %v", name, len(got), got[ChannelArmor:ChannelArmor+6])
		}
		if !bytes.Equal(got[:ChannelArmor], disc[:ChannelArmor]) || !bytes.Equal(got[ChannelArmor+6:], disc[ChannelArmor+6:]) {
			t.Fatalf("%s: more changed than the sets", name)
		}
	}
	if disc[ChannelArmor] != OrnateSet {
		t.Fatal("ForViewer wrote over the serializer's buffer")
	}
	live := lookSession(t, 2)
	live.SetLiveLook(true)
	got := ForViewer(live, disc, &p.Dyes)
	if len(got) != LiveEntityStateSize || !bytes.Equal(got[ChannelArmor:ChannelArmor+6], want) {
		t.Fatalf("the live look without this server's armour: %d bytes, sets %v", len(got), got[ChannelArmor:ChannelArmor+6])
	}
	live.SetArmorLook(true)
	got = ForViewer(live, disc, &p.Dyes)
	if len(got) != ArmorStateSize || !bytes.Equal(got[ChannelArmor:ChannelArmor+6], []byte{9, 4, 3, 9, 9, 4}) {
		t.Fatalf("this server's armour: %d bytes, sets %v", len(got), got[ChannelArmor:ChannelArmor+6])
	}
	// no live look, no armour look: a PS2 never asks for it alone
	ps2.SetArmorLook(true)
	if got := ForViewer(ps2, disc, &p.Dyes); len(got) != EntityStateSize || got[ChannelArmor] != PlainPlateSet {
		t.Fatalf("the armour look without the live look: %d bytes, chest %d", len(got), got[ChannelArmor])
	}
	// an NPC's set 9 the same
	n := &NPC{Name: "an ornate guard", Torso: OrnateSet}
	nd := SerializeNPCUpdate(5, n)
	if nd[ChannelArmor] != OrnateSet || ForViewer(lookSession(t, 3), nd, nil)[ChannelArmor] != PlainPlateSet {
		t.Fatal("an NPC's set 9 to a PS2")
	}
}

// To a client that draws this server's armour the live channel is followed by
// the six trim colours (chest .. helm, red first, 0 none): read as the live
// CLIENT reads the 201 bytes (readCreature), every field is LiveChannel's; the
// trims are the chardyes trim slots. The control: no trim dyes, six zeros.
func TestTheTrimsFollowTheLiveChannel(t *testing.T) {
	p := ornatePlayer()
	p.Dyes[database.DyeChest] = 0x808080FF
	p.Dyes[database.DyeChestTrim] = 0xD10000FF
	p.Dyes[database.DyeBootsTrim] = 0x2050E0FF
	disc := SerializePlayerUpdate(100001, p)
	got := ArmorChannel(disc, &p.Dyes)
	if len(got) != ArmorStateSize || ArmorStateSize != 225 {
		t.Fatalf("%d bytes", len(got))
	}
	if !bytes.Equal(got[:LiveEntityStateSize], LiveChannel(disc, &p.Dyes)) {
		t.Fatal("the first 201 bytes are not the live channel")
	}
	c := readCreature(got, true)
	if c.read != LiveEntityStateSize || c.armorCol[0] != 0x808080FF {
		t.Fatalf("the live CLIENT reads %d bytes, chest %08x", c.read, c.armorCol[0])
	}
	trim := func(b []byte, i int) uint32 { return binary.BigEndian.Uint32(b[LiveEntityStateSize+4*i:]) }
	for i, want := range []uint32{0xD10000FF, 0, 0, 0, 0x2050E0FF, 0} {
		if trim(got, i) != want {
			t.Fatalf("trim %d: %08x, want %08x", i, trim(got, i), want)
		}
	}
	plain := dyedPlayer()
	g := ArmorChannel(SerializePlayerUpdate(1, plain), &plain.Dyes)
	if !bytes.Equal(g[LiveEntityStateSize:], make([]byte, ArmorTrimSize)) {
		t.Fatalf("no trim dyes: %x", g[LiveEntityStateSize:])
	}
	// the record: LiveGear's 71, then the same six
	gear := make([]byte, DiscGearSize)
	gear[20] = OrnateSet
	ag := ArmorGear(gear, &p.Dyes)
	if len(ag) != ArmorGearSize || !bytes.Equal(ag[:LiveGearSize], LiveGear(gear, &p.Dyes)) || trim2(ag, 0) != 0xD10000FF || trim2(ag, 4) != 0x2050E0FF {
		t.Fatalf("the record: %d bytes %x", len(ag), ag[LiveGearSize:])
	}
}

func trim2(b []byte, i int) uint32 { return binary.BigEndian.Uint32(b[LiveGearSize+4*i:]) }

// Through the world, as TestTheWorldSendsEachClientItsLook: a player in set 9
// is set 4 on the PS2's channel and on an older vanilla's (the live look
// without this server's armour), and set 9 with its trims on the channel of a
// client that draws this server's armour; a trim dye set where it stands
// reaches that client at the next tick and changes nothing on the PS2's.
func TestTheWorldSendsSetNineAndTrimsToTheirClientsAlone(t *testing.T) {
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	add := func(n byte, x float32, liveLook, armor bool) (*session.Session, *Player) {
		sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 2, n), Port: 7200 + int(n)}, uint32(n), uint32(n), 1, 2, true)
		sess.SetCharacterInWorld(true)
		sess.SetPlayerObjectID(uint32(n))
		sess.SetLiveLook(liveLook)
		sess.SetArmorLook(armor)
		p := dyedPlayer()
		p.ServerID, p.CharServerID, p.X = EntityID(n), uint32(n), x
		p.Name = string(rune('A'+n)) + "name"
		w.Players[p.ServerID] = p
		w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
		sess.SetECSEntity(p)
		return sess, p
	}
	ps2, _ := add(1, 100, false, false)
	van, vp := add(2, 102, true, true)
	old, _ := add(3, 104, true, false)
	vp.Chest, vp.Legs = OrnateSet, OrnateSet
	w.TickCount = 0
	w.visibilitySystem()
	w.stateSyncSystem()
	w.entityChannelSyncSystem()
	view := func(viewer *session.Session, id EntityID) []byte {
		vs := GetOrCreateVisibility(viewer)
		for i, e := range vs.ChannelEntity {
			if e == id {
				return viewer.Connection().GetServerObject(i + 1).GetData()
			}
		}
		t.Fatalf("%v not in view", id)
		return nil
	}
	if got := van.Connection().GetServerObject(0).GetData(); len(got) != ArmorStateSize || got[ChannelArmor] != OrnateSet {
		t.Fatalf("its own channel: %d bytes, chest %d", len(got), got[ChannelArmor])
	}
	if got := view(ps2, 2); len(got) != EntityStateSize || got[ChannelArmor] != PlainPlateSet {
		t.Fatalf("the PS2's view: %d bytes, chest %d", len(got), got[ChannelArmor])
	}
	if got := view(old, 2); len(got) != LiveEntityStateSize || got[ChannelArmor] != PlainPlateSet {
		t.Fatalf("an older client's view: %d bytes, chest %d", len(got), got[ChannelArmor])
	}
	ps2Before := append([]byte(nil), view(ps2, 2)...)
	d := vp.Dyes
	d[database.DyeLegsTrim] = 0x00A86BFF
	if _, ok := w.SetDyes(2, d); !ok {
		t.Fatal("SetDyes")
	}
	w.stateSyncSystem()
	w.entityChannelSyncSystem()
	if got := binary.BigEndian.Uint32(van.Connection().GetServerObject(0).GetData()[LiveEntityStateSize+4*3:]); got != 0x00A86BFF {
		t.Fatalf("its own legs' trim: %08x", got)
	}
	if !bytes.Equal(view(ps2, 2), ps2Before) {
		t.Fatal("the PS2's view changed with a trim dye")
	}
}
