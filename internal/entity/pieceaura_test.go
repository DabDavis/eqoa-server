package entity

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// The pieces' auras (pieceaura.go; the owner, 2026-11-13: "even pieces of
// armor like bracers only if that piece has aura ... and they can have their
// own aura color each").

// auraBytes is one piece's aura as the channel carries it.
func auraBytes(a database.ItemAura) []byte {
	b := binary.BigEndian.AppendUint32(nil, a.Color)
	b = append(b, a.Kind)
	b = binary.LittleEndian.AppendUint32(b, a.Source)
	return append(b, a.Nodes)
}

// To a client that asked for them, the channel is the 225 bytes of this
// server's armour then seven pieces' auras, 10 bytes each, in the order
// chest, bracer, gloves, legs, boots, helm, robe (295 in all): each worn
// piece's item's, the robe's included; a piece whose item has none, a piece
// not worn, and a hidden piece, zeros. The first 225 are ArmorChannel's.
func TestThePiecesAurasFollowTheTrims(t *testing.T) {
	if AuraStateSize != 295 {
		t.Fatalf("AuraStateSize %d", AuraStateSize)
	}
	w := NewWorld()
	chest := database.ItemAura{PatternID: 500, Color: 0x3060FFFF}
	bracer := database.ItemAura{PatternID: 501, Color: 0xFF2020FF, Kind: database.AuraBoth, Source: 0x88EC092D, Nodes: 3}
	robe := database.ItemAura{PatternID: 502, Color: 0x20FF20FF, Kind: database.AuraShell}
	helm := database.ItemAura{PatternID: 503, Kind: database.AuraParticles, Source: 0x12345678, Nodes: 2}
	w.SetItemAuras(map[int]database.ItemAura{500: chest, 501: bracer, 502: robe, 503: helm})
	p := ornatePlayer()
	p.SetWornPiece(AuraChest, 500)
	p.SetWornPiece(AuraBracer, 501)
	p.SetWornPiece(AuraLegs, 999) // an item with no aura
	p.SetWornPiece(AuraRobe, 502)
	p.SetWornPiece(AuraHelm, 503)
	p.HiddenParts = HideHelm // the helm hidden: no aura
	a := w.PieceAurasOf(p)
	if a == nil {
		t.Fatal("no auras")
	}
	disc := SerializePlayerUpdate(100001, p)
	got := AuraChannel(disc, &p.Dyes, a)
	if len(got) != AuraStateSize || !bytes.Equal(got[:ArmorStateSize], ArmorChannel(disc, &p.Dyes)) {
		t.Fatalf("%d bytes; the first 225 not ArmorChannel's", len(got))
	}
	var want []byte
	for _, x := range []database.ItemAura{chest, bracer, {}, {}, {}, {}, robe} {
		want = append(want, auraBytes(x)...)
	}
	if !bytes.Equal(got[ArmorStateSize:], want) {
		t.Fatalf("the auras:\n% x\nwant\n% x", got[ArmorStateSize:], want)
	}
	// the robe's bytes, spelled out: 20 ff 20 ff, kind 2, source 0, nodes 0
	if r := got[ArmorStateSize+6*PieceAuraSize:]; !bytes.Equal(r, []byte{0x20, 0xFF, 0x20, 0xFF, 2, 0, 0, 0, 0, 0}) {
		t.Fatalf("the robe: % x", r)
	}
	// the bracer's source, little-endian
	if b := got[ArmorStateSize+PieceAuraSize:][5:9]; !bytes.Equal(b, []byte{0x2D, 0x09, 0xEC, 0x88}) {
		t.Fatalf("the bracer's source: % x", b)
	}
	// the helm shown again: its particles
	p.HiddenParts = 0
	if a := w.PieceAurasOf(p); a == nil || a[AuraHelm] != helm {
		t.Fatalf("the helm unhidden: %+v", a)
	}
	// none worn, or no rows: nil, which is seven zeros
	if w.PieceAurasOf(dyedPlayer()) != nil || NewWorld().PieceAurasOf(p) != nil {
		t.Fatal("auras with nothing worn, or no rows")
	}
	if g := AuraChannel(disc, &p.Dyes, nil); !bytes.Equal(g[ArmorStateSize:], make([]byte, 70)) {
		t.Fatal("nil auras are not zeros")
	}
}

// Who is sent what: the 295 bytes go only to a session that asked for the
// pieces' auras with this server's armour in the live look; one with the
// armour alone gets ArmorChannel's 225, one with the live look alone 201, a
// PS2 162, exactly as ForViewer sends them (the control: no change for any
// client that did not ask). The bit without the armour, or without the live
// look, is nothing.
func TestThePiecesAurasGoOnlyToTheClientsThatAsk(t *testing.T) {
	p := ornatePlayer()
	disc := SerializePlayerUpdate(100001, p)
	a := &PieceAuras{AuraChest: {Color: 0x3060FFFF}}
	for i, c := range []struct {
		live, armor, auras bool
		want               int
	}{
		{false, false, false, EntityStateSize},
		{false, false, true, EntityStateSize},
		{true, false, false, LiveEntityStateSize},
		{true, false, true, LiveEntityStateSize},
		{true, true, false, ArmorStateSize},
		{true, true, true, AuraStateSize},
	} {
		s := lookSession(t, byte(20+i))
		s.SetLiveLook(c.live)
		s.SetArmorLook(c.armor)
		s.SetPieceAuras(c.auras)
		got := ForViewerAuras(s, disc, &p.Dyes, a)
		if len(got) != c.want {
			t.Fatalf("%+v: %d bytes, want %d", c, len(got), c.want)
		}
		if c.want != AuraStateSize && !bytes.Equal(got, ForViewer(s, disc, &p.Dyes)) {
			t.Fatalf("%+v: not ForViewer's bytes", c)
		}
	}
	if got := ForViewerAuras(nil, disc, &p.Dyes, a); !bytes.Equal(got, ForViewer(nil, disc, &p.Dyes)) {
		t.Fatal("no session")
	}
}

// Through the world: a player in an aura'd bracer is 295 bytes on the channel
// of a client that draws auras (its own channel 0 and another's view), the
// bracer's aura in its place, every NPC on it 295 with zeros (a mercenary
// in an aura'd bracer its aura); the PS2's
// view and an armour-only client's are as they were. A GM's edit of the rows
// (SetItemAuras) reaches the next tick; the channel's XOR delta then names a
// message of the new length, and a format change (the client stops asking)
// shrinks the buffer to 225 again.
func TestTheWorldSendsThePiecesAuras(t *testing.T) {
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	w.SetItemAuras(map[int]database.ItemAura{501: {PatternID: 501, Color: 0xFF2020FF, Kind: database.AuraShell}})
	add := func(n byte, x float32, live, armor, auras bool) (*session.Session, *Player) {
		sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 3, n), Port: 7300 + int(n)}, uint32(n), uint32(n), 1, 2, true)
		sess.SetCharacterInWorld(true)
		sess.SetPlayerObjectID(uint32(n))
		sess.SetLiveLook(live)
		sess.SetArmorLook(armor)
		sess.SetPieceAuras(auras)
		p := dyedPlayer()
		p.ServerID, p.CharServerID, p.X = EntityID(n), uint32(n), x
		p.Name = string(rune('A'+n)) + "name"
		w.Players[p.ServerID] = p
		w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
		sess.SetECSEntity(p)
		return sess, p
	}
	ps2, _ := add(1, 100, false, false, false)
	van, vp := add(2, 102, true, true, true)
	arm, _ := add(3, 104, true, true, false)
	_, op := add(4, 106, true, true, true)
	op.SetWornPiece(AuraBracer, 501)
	vp.SetWornPiece(AuraBracer, 501)
	n := &NPC{ServerID: 50, Name: "a wolf", X: 103, Y: 20, Z: 300, HP: 10, MaxHP: 10, Size: 1}
	w.NPCs[n.ServerID] = n
	w.SpatialIndex.Insert(n.ServerID, n.X, n.Z)
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
	bracer := []byte{0xFF, 0x20, 0x20, 0xFF, database.AuraShell, 0, 0, 0, 0, 0}
	own := van.Connection().GetServerObject(0).GetData()
	if len(own) != AuraStateSize || !bytes.Equal(own[ArmorStateSize+PieceAuraSize:ArmorStateSize+2*PieceAuraSize], bracer) {
		t.Fatalf("its own channel: %d bytes % x", len(own), own[min(len(own), ArmorStateSize):])
	}
	if got := view(van, 4); len(got) != AuraStateSize || !bytes.Equal(got[ArmorStateSize+PieceAuraSize:ArmorStateSize+2*PieceAuraSize], bracer) {
		t.Fatalf("another's bracer: %d bytes", len(got))
	}
	if got := view(van, 50); len(got) != AuraStateSize || !bytes.Equal(got[ArmorStateSize:], make([]byte, 70)) {
		t.Fatalf("an NPC: %d bytes", len(got))
	}
	// a mercenary, an NPC dressed in item records (MercLook.WornPieces): its
	// bracer's aura as a player's
	n.WornPieces[AuraBracer], n.WornMask = 501, 1<<AuraBracer
	w.entityChannelSyncSystem()
	if got := view(van, 50); !bytes.Equal(got[ArmorStateSize+PieceAuraSize:ArmorStateSize+2*PieceAuraSize], bracer) {
		t.Fatalf("a mercenary's bracer: % x", got[ArmorStateSize:])
	}
	if got := view(ps2, 4); len(got) != EntityStateSize {
		t.Fatalf("the PS2's view: %d bytes", len(got))
	}
	if got := view(arm, 4); len(got) != ArmorStateSize {
		t.Fatalf("an armour-only client's view: %d bytes", len(got))
	}
	// a GM's edit: the bracer blue, at the next tick
	w.SetItemAuras(map[int]database.ItemAura{501: {PatternID: 501, Color: 0x3060FFFF, Kind: database.AuraShell}})
	w.stateSyncSystem()
	w.entityChannelSyncSystem()
	if got := view(van, 4); !bytes.Equal(got[ArmorStateSize+PieceAuraSize:ArmorStateSize+PieceAuraSize+4], []byte{0x30, 0x60, 0xFF, 0xFF}) {
		t.Fatalf("the edit: % x", got[ArmorStateSize+PieceAuraSize:ArmorStateSize+PieceAuraSize+4])
	}
	// a format change: the client stops asking; the channel takes the 225
	van.SetPieceAuras(false)
	w.stateSyncSystem()
	if got := van.Connection().GetServerObject(0).GetData(); len(got) != ArmorStateSize {
		t.Fatalf("after the format change: %d bytes", len(got))
	}
}

// The body's aura (charaura slot 7; the owner, 2026-11-13: "a wholebody just
// applies it to body and underclothes, armor will cover it up"): to a session
// that asked for it too (bit 4) the channel is 305 bytes, the 295 then the
// body's ten; a session that asked for the pieces alone gets the 295 bytes
// exactly as before (vanilla a95b250's), the body left out. A character's own
// piece aura wins over its item's, and only that character's: another wearer
// of the item shows the item's. A bare character shows its body aura, with
// nothing worn; without charaura rows, as before.
func TestTheBodysAuraAndACharactersOwn(t *testing.T) {
	if BodyAuraStateSize != 305 {
		t.Fatalf("BodyAuraStateSize %d", BodyAuraStateSize)
	}
	w := NewWorld()
	item := database.ItemAura{PatternID: 500, Color: 0xFFC040FF, Kind: database.AuraTrim}
	w.SetItemAuras(map[int]database.ItemAura{500: item})
	mine := database.ItemAura{Color: 0x3060FFFF, Kind: database.AuraParticles}
	body := database.ItemAura{Color: 0x000000FF, Kind: database.AuraShell}
	var own [database.CharAuraSlots]database.ItemAura
	own[AuraChest], own[AuraBody] = mine, body
	w.SetCharAuras(map[database.CharAuraKey][database.CharAuraSlots]database.ItemAura{{CharID: 7}: own})
	a, b := ornatePlayer(), ornatePlayer()
	a.CharServerID, b.CharServerID = 7, 8
	a.SetWornPiece(AuraChest, 500)
	b.SetWornPiece(AuraChest, 500)
	pa, pb := w.PieceAurasOf(a), w.PieceAurasOf(b)
	if pa == nil || pa[AuraChest] != mine || pa[AuraBody] != body {
		t.Fatalf("the character's own: %+v", pa)
	}
	if pb == nil || pb[AuraChest] != item || pb[AuraBody] != (database.ItemAura{}) {
		t.Fatalf("another wearer of the item: %+v", pb)
	}
	bare := dyedPlayer()
	bare.CharServerID = 7
	if p := w.PieceAurasOf(bare); p == nil || p[AuraBody] != body || p[AuraChest] != (database.ItemAura{}) {
		t.Fatalf("bare: the body alone: %+v", p)
	}
	disc := SerializePlayerUpdate(100001, a)
	s := lookSession(t, 40)
	s.SetLiveLook(true)
	s.SetArmorLook(true)
	s.SetPieceAuras(true)
	old := ForViewerAuras(s, disc, &a.Dyes, pa)
	if len(old) != AuraStateSize || !bytes.Equal(old, AuraChannel(disc, &a.Dyes, pa)) {
		t.Fatalf("the pieces' clients: %d bytes", len(old))
	}
	// the 295 bytes as they were made before the body: seven auras, no more
	seven := append(ArmorChannel(disc, &a.Dyes), auraBytes(mine)...)
	seven = append(seven, make([]byte, 6*PieceAuraSize)...)
	if !bytes.Equal(old, seven) {
		t.Fatal("the 295 bytes changed")
	}
	s.SetBodyAura(true)
	got := ForViewerAuras(s, disc, &a.Dyes, pa)
	if len(got) != BodyAuraStateSize || !bytes.Equal(got[:AuraStateSize], old) || !bytes.Equal(got[AuraStateSize:], auraBytes(body)) {
		t.Fatalf("the body's clients: %d bytes, tail % x", len(got), got[AuraStateSize:])
	}
	s.SetPieceAuras(false)
	if s.BodyAura() || len(ForViewerAuras(s, disc, &a.Dyes, pa)) != ArmorStateSize {
		t.Fatal("the body bit without the pieces' counts")
	}
}

// The cape's aura (charaura slot 8, or the worn cape item's itemaura; the
// owner, 2026-11-14: "also cannot apply aura to capes yet"): to a session that
// asked for it too (bit 5) the channel is 315 bytes, the 305 then the cape's
// ten; a session that asked for the body's alone gets the 305 bytes exactly
// as before (vanilla 481ce51's), the cape left out, and the indices before it
// stay. A worn cape shows its item's aura, the character's own over it; a
// hidden cape and no cape show none, whatever charaura says.
func TestTheCapesAura(t *testing.T) {
	if CapeAuraStateSize != 315 || AuraBody != 7 || AuraCape != 8 || database.CharAuraCape != AuraCape || database.CharAuraSlots != AuraSlots {
		t.Fatalf("sizes: %d, body %d cape %d", CapeAuraStateSize, AuraBody, AuraCape)
	}
	w := NewWorld()
	item := database.ItemAura{PatternID: 717001, Color: 0x3060FFFF, Kind: database.AuraShell}
	w.SetItemAuras(map[int]database.ItemAura{717001: item})
	a, b := ornatePlayer(), ornatePlayer()
	a.CharServerID, b.CharServerID = 7, 8
	mine := database.ItemAura{Color: 0xFF2020FF, Kind: database.AuraTrim}
	body := database.ItemAura{Color: 0x000000FF, Kind: database.AuraShell}
	var own [database.CharAuraSlots]database.ItemAura
	own[AuraCape], own[AuraBody] = mine, body
	w.SetCharAuras(map[database.CharAuraKey][database.CharAuraSlots]database.ItemAura{{CharID: 7}: own})
	if p := w.PieceAurasOf(a); p == nil || p[AuraCape] != (database.ItemAura{}) || p[AuraBody] != body {
		t.Fatalf("no cape worn: its charaura row shows nothing: %+v", p)
	}
	a.SetWornPiece(AuraCape, 717001)
	b.SetWornPiece(AuraCape, 717001)
	a.SetWornPiece(AuraBody, 717001) // no piece: ignored
	if a.WornMask&(1<<AuraBody) != 0 {
		t.Fatal("the body recorded as a worn piece")
	}
	pa, pb := w.PieceAurasOf(a), w.PieceAurasOf(b)
	if pa == nil || pa[AuraCape] != mine {
		t.Fatalf("the character's own cape aura: %+v", pa)
	}
	if pb == nil || pb[AuraCape] != item {
		t.Fatalf("the cape item's own: %+v", pb)
	}
	b.HiddenParts = HideCape
	if w.PieceAurasOf(b) != nil {
		t.Fatal("a hidden cape shows its aura")
	}
	b.HiddenParts = HideRobe // another part hidden: the cape still shows
	if p := w.PieceAurasOf(b); p == nil || p[AuraCape] != item {
		t.Fatalf("the robe hidden hid the cape: %+v", p)
	}
	disc := SerializePlayerUpdate(100001, a)
	s := lookSession(t, 41)
	s.SetLiveLook(true)
	s.SetArmorLook(true)
	s.SetPieceAuras(true)
	s.SetBodyAura(true)
	old := ForViewerAuras(s, disc, &a.Dyes, pa)
	// the 305 bytes as they were made before the cape: eight auras, no more
	eight := append(ArmorChannel(disc, &a.Dyes), make([]byte, 7*PieceAuraSize)...)
	eight = append(eight, auraBytes(body)...)
	if len(old) != BodyAuraStateSize || !bytes.Equal(old, eight) {
		t.Fatalf("the body's clients: %d bytes, changed", len(old))
	}
	s.SetCapeAura(true)
	got := ForViewerAuras(s, disc, &a.Dyes, pa)
	if len(got) != CapeAuraStateSize || !bytes.Equal(got[:BodyAuraStateSize], old) || !bytes.Equal(got[BodyAuraStateSize:], auraBytes(mine)) {
		t.Fatalf("the cape's clients: %d bytes, tail % x", len(got), got[BodyAuraStateSize:])
	}
	if !bytes.Equal(got, CapeAuraChannel(disc, &a.Dyes, pa)) {
		t.Fatal("ForViewerAuras is not CapeAuraChannel")
	}
	if n := ForViewerAuras(s, disc, nil, nil); len(n) != CapeAuraStateSize || !bytes.Equal(n[BodyAuraStateSize:], make([]byte, PieceAuraSize)) {
		t.Fatal("no auras: the 315 bytes with the cape's none")
	}
	s.SetBodyAura(false)
	if s.CapeAura() || len(ForViewerAuras(s, disc, &a.Dyes, pa)) != AuraStateSize {
		t.Fatal("the cape bit without the body's counts")
	}
	if PS2 := ForViewerAuras(nil, disc, &a.Dyes, pa); len(PS2) != EntityStateSize {
		t.Fatalf("a PS2: %d bytes", len(PS2))
	}
}
