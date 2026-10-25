package handlers

import (
	"encoding/binary"
	"math"
	"net"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The worlds a client loads, by what its DiscVersion said: a Frontiers disc's
// six (0x12: a retail PS2, the vanilla client, the page; and a client that
// has said nothing yet), the whole table for a PS2 running the July S2 (0x13,
// S2's marker on its world session, our hard-disk loader after it), or S2's
// eight for Sandstorm's live S2 (0x15). No session is the disc's six.
func TestLoadableWorldsByDiscVersion(t *testing.T) {
	c := newChatWorld(t)
	s, _ := c.join(1, "Tester", 25000, 15000, 0)
	cases := []struct {
		version byte
		max     int
	}{{0, discWorlds}, {protocol.GameVersionEQOAVanilla, discWorlds}, {protocol.GameVersionSandstormS2, WorldCount}, {protocol.GameVersionSandstormS2Live, s2Worlds}}
	for _, k := range cases {
		s.SetDiscVersion(k.version)
		for w := 0; w < 256; w++ {
			if got, want := CanLoadWorld(s, uint8(w)), w < k.max; got != want {
				t.Errorf("version %#x, world %d: %v, want %v", k.version, w, got, want)
			}
		}
	}
	if CanLoadWorld(nil, 6) || !CanLoadWorld(nil, 5) {
		t.Error("no session must be the disc's six")
	}
}

// gukNPCs is a world's people for the namesake tests: three of Guk's
// frogloks standing together in Tunaria, two NPCs whose names hold "guk"
// inside a word and who stand elsewhere (made-up names), two guards along a road, and the
// August test gatekeeper in world 8 (its label was "Guk").
func gukNPCs(w *entity.World) {
	for id, n := range map[entity.EntityID]*entity.NPC{
		700: {Name: "froglok guk wizard", World: 0, SpawnX: 24610, SpawnY: -30, SpawnZ: 28980},
		701: {Name: "froglok guk shaman", World: 0, SpawnX: 24630, SpawnY: -33, SpawnZ: 28955},
		702: {Name: "froglok guk warrior", World: 0, SpawnX: 24630, SpawnY: -33, SpawnZ: 28962},
		703: {Name: "Old Bogguk", World: 0, SpawnX: 25565, SpawnY: 63.8, SpawnZ: 31495},
		704: {Name: "Snarguk", World: 0, SpawnX: 15512, SpawnY: 54.1, SpawnZ: 10403},
		705: {Name: "Guard Alben", World: 0, SpawnX: 25000, SpawnY: 1, SpawnZ: 15000},
		706: {Name: "Guard Dahvel", World: 0, SpawnX: 18000, SpawnY: 1, SpawnZ: 11000},
		707: {Name: "Gatekeeper Krup", World: 8, SpawnX: 1125, SpawnY: -62.2, SpawnZ: 1143.7},
		708: {Name: "kobold guk sentry", World: 8, SpawnX: 1130, SpawnY: -62.2, SpawnZ: 1150},
	} {
		n.ServerID = id
		n.X, n.Y, n.Z = n.SpawnX, n.SpawnY, n.SpawnZ
		w.NPCs[id] = n
	}
}

// "guk" is where Guk's frogloks stand, each a word of their names: the spot
// of the one nearest their middle. A name's inner letters do not count
// (Old Bogguk, Snarguk), guards along a road are not one place, and a
// world the client cannot load is not offered. The control: with a plain
// substring Bogguk and Snarguk join, standing elsewhere, and there is no place.
func TestNamesakePlace(t *testing.T) {
	w := entity.NewWorld()
	gukNPCs(w)
	disc := DiscWorld
	p, n, ok := NamesakePlace(w, "guk", disc)
	if !ok || n != 3 || p.NPC != "froglok guk warrior" || p.World != 0 || p.X != 24630 || p.Z != 28962 {
		t.Fatalf("guk: %+v, %d, %v; want the warrior's spot of 3 in Tunaria", p, n, ok)
	}
	if _, n, ok := NamesakePlace(w, "guard", disc); ok || n != 2 {
		t.Errorf("guard: %d, %v; want 2 that stand apart, no place", n, ok)
	}
	if p, n, ok := NamesakePlace(w, "bogguk", disc); !ok || n != 1 || p.NPC != "Old Bogguk" {
		t.Errorf("bogguk: %+v, %d, %v; want Old Bogguk", p, n, ok)
	}
	if _, _, ok := NamesakePlace(w, "nobody", disc); ok {
		t.Error("a name nobody has found a place")
	}
	// world 8's guk sentry counts only for a game that loads it, and then
	// "guk" names people in two worlds: no one place
	if _, n, ok := NamesakePlace(w, "kobold", disc); ok || n != 0 {
		t.Errorf("kobold for the disc's six: %d, %v; want none (world 8)", n, ok)
	}
	if p, _, ok := NamesakePlace(w, "kobold", nil); !ok || p.World != 8 {
		t.Errorf("kobold for the whole table: %+v %v", p, ok)
	}
	if _, n, ok := NamesakePlace(w, "guk", nil); ok || n != 4 {
		t.Errorf("guk for the whole table: %d, %v; want 4 in two worlds, no place", n, ok)
	}

	var sub []entity.Place
	for _, s := range w.NPCsNamed(func(name string) bool { return strings.Contains(strings.ToLower(name), "guk") }) {
		if s.World == 0 {
			sub = append(sub, s)
		}
	}
	if len(sub) != 5 {
		t.Fatalf("the control: a substring finds %d in Tunaria, want 5", len(sub))
	}
	var mx, mz float64
	for _, s := range sub {
		mx += float64(s.X) / float64(len(sub))
		mz += float64(s.Z) / float64(len(sub))
	}
	apart := false
	for _, s := range sub {
		if math.Hypot(float64(s.X)-mx, float64(s.Z)-mz) > namesakeSpan {
			apart = true
		}
	}
	if !apart {
		t.Error("the control: the substring's five should stand apart")
	}
}

// A GM's /goto guk lands in Guk, where its frogloks stand in Tunaria; before,
// the August gatekeeper's label "Guk" sent it to world 8, which a game with
// the disc's six cannot load (F3 20261025-000749). A GM whose game loads only
// the disc's six is refused world 8 by its label, by coordinates and by an NPC
// there, and nothing moves; a patched PS2's GM goes. The control: /goto guard
// lists the guards along the road rather than landing among them.
func TestGotoGukGoesToGuk(t *testing.T) {
	c, gs, gp, _, _ := gmWorld(t)
	gukNPCs(c.gw)
	c.say(gs, "/goto guk")
	if x, z, w, ok := teleported(gs); !ok || x != 24630 || z != 28962 || w != 0 || gp.World != 0 {
		t.Fatalf("/goto guk: (%v, %v) world %d, %v", x, z, w, ok)
	}
	if got := texts(heard(t, gs)); !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, "froglok guk warrior, where the 3 NPCs") }) {
		t.Fatalf("/goto guk said %q", got)
	}

	refused := func(line string) {
		t.Helper()
		gs.Queue().GetPending()
		c.say(gs, line)
		if _, _, _, ok := teleported(gs); ok {
			t.Fatalf("%s moved a GM whose game has the disc's six", line)
		}
		if gp.World != 0 {
			t.Fatalf("%s filed the GM under world %d", line, gp.World)
		}
	}
	refused("/goto world 8")
	refused("/goto 1125 -62 1143 8")
	refused("/goto gatekeeper krup")
	refused("#tele world9")
	if got := texts(heard(t, gs)); !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, "cannot load world9 (world 9)") }) {
		t.Errorf("#tele world9 said %v", got)
	}

	gs.Queue().GetPending()
	c.say(gs, "/goto guard")
	if _, _, _, ok := teleported(gs); ok {
		t.Fatal("the control: /goto guard landed among guards along a road")
	}
	if got := texts(heard(t, gs)); !slices.Contains(got, `[GM] 2 NPCs match "guard"; which one?`) {
		t.Fatalf("the control: /goto guard said %q", got)
	}

	gs.SetDiscVersion(protocol.GameVersionSandstormS2)
	gs.Queue().GetPending()
	c.say(gs, "/goto world 8")
	if x, z, w, ok := teleported(gs); !ok || w != 8 || x != 1125 || z != 1143.7 || gp.World != 8 {
		t.Fatalf("a patched PS2's /goto world 8: (%v, %v) world %d, %v", x, z, w, ok)
	}
}

// optionBox is the options of the option box (0x0034) queued for s, if one is.
func optionBox(t *testing.T, s *session.Session) []string {
	t.Helper()
	for _, m := range s.Queue().GetPending() {
		d := m.Data()
		if len(d) < 6 || protocol.GameOpcode(binary.LittleEndian.Uint16(d)) != protocol.OpcodeQuestPopupMulti {
			continue
		}
		p := 6
		str := func() string {
			n := int(binary.LittleEndian.Uint32(d[p:]))
			u := make([]uint16, n)
			for i := range u {
				u[i] = binary.LittleEndian.Uint16(d[p+4+2*i:])
			}
			p += 4 + 2*n
			return string(utf16.Decode(u))
		}
		str() // the greeting
		n := int(d[p])
		p++
		var out []string
		for range n {
			out = append(out, str())
		}
		return out
	}
	t.Fatal("no option box")
	return nil
}

// Harbormaster Thand's menu, through the interact handler, offers a game with
// the disc's six only Odus (and none of the patched consoles' worlds 7, 8 and
// 9, picked once from a page: F3 20261025-000749); a patched PS2's session
// is offered the whole table, which is the control.
func TestTheHarbormasterOffersWhatTheGameLoads(t *testing.T) {
	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	for id, n := range map[entity.EntityID]*entity.NPC{
		800: {Name: "Harbormaster Thand", World: 0, SpawnX: 25377.7, SpawnY: 54.1, SpawnZ: 15741.5},
		801: {Name: "Gatekeeper Sulon", World: 2, SpawnX: 7338, SpawnY: 54.2, SpawnZ: 11124},
		802: {Name: "Gatekeeper Rilis", World: 7, SpawnX: 4562, SpawnY: -12.3, SpawnZ: 3663.8},
		803: {Name: "Gatekeeper Krup", World: 8, SpawnX: 1125, SpawnY: -62.2, SpawnZ: 1143.7},
		804: {Name: "Harbormaster Machado", World: 9, SpawnX: 13000, SpawnY: 66.5, SpawnZ: 23000},
	} {
		n.ServerID = id
		n.X, n.Y, n.Z = n.SpawnX, n.SpawnY, n.SpawnZ
		w.NPCs[id] = n
	}
	h := NewInteractHandler(mgr)
	h.SetGameWorld(w)
	menu := func(port byte, version byte) []string {
		s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, port), Port: 7000 + int(port)}, uint32(port), 1, 1, 2, true)
		p := &entity.Player{ServerID: entity.EntityID(1000 + int(port)), Name: "Traveller", X: 25377.7, Y: 54.1, Z: 15741.5}
		w.Players[p.ServerID] = p
		s.SetECSEntity(p)
		s.SetCharacterInWorld(true)
		s.SetDiscVersion(version)
		vs := entity.GetOrCreateVisibility(s)
		vs.ChannelEntity[1], vs.ChannelObjectID[1] = 800, 100002
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeInteract)
		msg.SetData([]byte{0xA2, 0x86, 0x01, 0x00})
		if err := h.HandleInteract(s, msg); err != nil {
			t.Fatal(err)
		}
		return optionBox(t, s)
	}
	if got := menu(1, protocol.GameVersionEQOAVanilla); !slices.Equal(got, []string{"Odus"}) {
		t.Errorf("a disc's game is offered %q, want just Odus", got)
	}
	if got := menu(2, protocol.GameVersionSandstormS2); len(got) != 4 || !slices.Contains(got, "World 8 (HDD)") {
		t.Errorf("the control: a patched PS2 is offered %q, want Odus and worlds 7, 8 and 9", got)
	}
}
