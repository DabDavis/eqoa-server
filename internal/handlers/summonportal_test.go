package handlers

import (
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The Summoning Portal (summonportal.go) driven through the handlers a client
// reaches: 0x003E, 0x0004, 0x0035, 0x000E and a typed line's three ways. Anna
// and Cora stand together in Tunaria, Bert far off, Dora in Odus; nobody is in
// a group. The people and places are made up.

type portalRig struct {
	*chatWorld
	t   *testing.T
	g   *Portal
	ih  *InteractHandler
	gh  *GuildHandler
	now time.Time

	anna, bert, cora, dora *session.Session
	pa, pb, pc, pd         *entity.Player
}

func newPortalRig(t *testing.T) *portalRig {
	t.Helper()
	c := newChatWorld(t)
	r := &portalRig{chatWorld: c, t: t, now: time.Unix(1_800_000_000, 0)}
	r.ih = NewInteractHandler(c.sm)
	r.ih.SetGameWorld(c.gw)
	r.gh = NewGuildHandler(c.sm)
	r.gh.SetGameWorld(c.gw)
	r.g = NewPortal(c.gw)
	r.g.now = func() time.Time { return r.now }
	c.h.portal, r.ih.portal, r.gh.portal = r.g, r.g, r.g
	// the hooks as RegisterDefaultHandlers sets them
	ch := &CharacterHandler{sessionManager: c.sm, gameWorld: c.gw}
	c.gw.OnTravel(ch.travelPlayer)
	c.gw.SetLoadableWorlds(LoadableWorlds)
	mk := func(id int, name string, x, z float32, w uint8) (*session.Session, *entity.Player) {
		s, p := c.join(id, name, x, z, w)
		p.HP, p.MaxHP = 100, 100
		return s, p
	}
	r.anna, r.pa = mk(1, "Anna", 5000, 5000, 0)
	r.cora, r.pc = mk(2, "Cora", 5001, 5001, 0)
	r.bert, r.pb = mk(3, "Bert", 20000, 20000, 0)
	r.dora, r.pd = mk(4, "Dora", 6000, 6000, 2)
	r.drain()
	return r
}

func (r *portalRig) drain() {
	for _, s := range []*session.Session{r.anna, r.bert, r.cora, r.dora} {
		s.Queue().GetPending()
	}
}

// open is the item used by Anna.
func (r *portalRig) open() *entity.NPC {
	r.t.Helper()
	r.g.Open(r.anna, r.pa)
	var n *entity.NPC
	for _, x := range r.gw.NPCs {
		if x.PortalOf == r.pa.ServerID {
			n = x
		}
	}
	if n == nil {
		r.t.Fatal("no portal")
	}
	r.drain()
	return n
}

// talk is the player's 0x0004 to the portal, known to its session on a slot.
func (r *portalRig) talk(s *session.Session, n *entity.NPC) {
	r.t.Helper()
	vs := entity.GetOrCreateVisibility(s)
	const id = uint32(100001)
	vs.ChannelEntity[0], vs.ChannelObjectID[0] = n.ServerID, id
	if err := r.ih.HandleInteract(s, incoming(protocol.OpcodeInteract, u32b(id))); err != nil {
		r.t.Fatal(err)
	}
}

// pick is the player's 0x0035.
func (r *portalRig) pick(s *session.Session, box uint32, option byte) {
	r.t.Helper()
	if err := r.ih.HandleDialogueOption(s, incoming(protocol.OpcodeDialogueBoxOption, append(u32b(box), option))); err != nil {
		r.t.Fatal(err)
	}
}

// ask talks to the portal and answers the box, leaving s's next line awaited;
// it returns nothing said (the queues are drained).
func (r *portalRig) ask(s *session.Session, n *entity.NPC) {
	r.t.Helper()
	r.drain()
	r.talk(s, n)
	box := boxID(r.msgs(s))
	if !IsPortalBox(box) {
		r.t.Fatalf("the portal's box: %08X", box)
	}
	r.now = r.now.Add(2 * time.Second)
	r.pick(s, box, 0)
	r.now = r.now.Add(2 * time.Second)
	r.drain()
}

// msgs is what s was sent and took: "box:", "ask#id:text|a|b", "chat:" white,
// "red:", "colour:", "teleport" and "op:XXXX".
func (r *portalRig) msgs(s *session.Session) []string {
	var out []string
	for _, m := range s.Queue().GetPending() {
		d := m.Data()
		if len(d) < 2 {
			continue
		}
		op := protocol.GameOpcode(binary.LittleEndian.Uint16(d))
		b := buffer.NewReader(d[2:])
		wide := func() string {
			n, _ := b.ReadUint32()
			var sb strings.Builder
			for i := uint32(0); i < n; i++ {
				u, _ := b.ReadUint16()
				sb.WriteRune(rune(u))
			}
			return sb.String()
		}
		switch op {
		case protocol.OpcodeQuestPopupMulti:
			id, _ := b.ReadUint32()
			text := wide()
			c, _ := b.ReadByte()
			opts := []string{}
			for i := 0; i < int(c); i++ {
				opts = append(opts, wide())
			}
			out = append(out, fmt.Sprintf("ask#%d:%s|%s", id, text, strings.Join(opts, "|")))
		case protocol.OpcodeDialogueBox:
			b.ReadUint32()
			out = append(out, "box:"+wide())
		case protocol.OpcodeClientMessage:
			out = append(out, "chat:"+wide())
		case protocol.OpcodeErrorMessage:
			out = append(out, "red:"+wide())
		case protocol.OpcodeColoredChat:
			out = append(out, "colour:"+wide())
		case protocol.OpcodeTeleport:
			out = append(out, "teleport")
		default:
			out = append(out, fmt.Sprintf("op:%04X", uint16(op)))
		}
	}
	return out
}

func (r *portalRig) want(who string, got []string, wants ...string) {
	r.t.Helper()
	if len(got) != len(wants) {
		r.t.Errorf("%s was sent %q, want %q", who, got, wants)
		return
	}
	for i, w := range wants {
		if !hasMsg(got[i:i+1], w) {
			r.t.Errorf("%s was sent %q, want %q", who, got, wants)
			return
		}
	}
}

// tick lands whatever a yes sent on its way.
func (r *portalRig) tick() {
	r.t.Helper()
	r.gw.Tick(r.now)
}

// The whole of it through the handlers: the item's portal, the box, the name in
// chat (any case) heard by no one, the accept box, a yes bringing the target to
// where the portal's owner stood, the caller told; then the next line is chat
// again. The controls: the target stays until it answers, and a bystander in say
// range hears the line after the name.
func TestAPortalCallEndToEnd(t *testing.T) {
	r := newPortalRig(t)
	n := r.open()

	r.talk(r.anna, n)
	got := r.msgs(r.anna)
	box := boxID(got)
	if !IsPortalBox(box) || box&0xFF000000 != 0x5A000000 {
		t.Fatalf("the box's id %08X", box)
	}
	r.want("Anna", got, "ask#*")
	if !strings.Contains(got[0], ":"+portalQuestion+"|"+portalWillType) {
		t.Fatalf("the box: %q", got)
	}
	if !strings.Contains(portalQuestion, "type the player's name") || !strings.Contains(portalQuestion, "press Enter") {
		t.Errorf("the box does not say what to do: %q", portalQuestion)
	}
	r.now = r.now.Add(2 * time.Second)
	r.pick(r.anna, box, 0)
	r.want("Anna", r.msgs(r.anna), "chat:"+portalListens)

	r.say(r.anna, "bERt")
	r.want("Cora, a bystander", r.msgs(r.cora))
	r.want("Anna", r.msgs(r.anna), "chat:You call Bert through the portal.")
	got = r.msgs(r.bert)
	r.want("Bert", got, "ask#*")
	if !strings.HasSuffix(got[0], ":Anna calls you through a summoning portal. Go?|Yes|No") {
		t.Fatalf("the call's box: %q", got)
	}
	call := boxID(got)
	if !IsPortalBox(call) {
		t.Fatalf("the call's box id %08X", call)
	}
	r.tick()
	if snap, _ := r.gw.PlayerSnapshot(r.pb.ServerID); snap.X != 20000 {
		t.Fatalf("Bert moved before answering: x %.0f", snap.X)
	}
	r.drain()

	r.now = r.now.Add(4 * time.Second)
	r.pick(r.bert, call, 0)
	r.tick()
	if snap, _ := r.gw.PlayerSnapshot(r.pb.ServerID); snap.World != 0 || snap.X != 5000 || snap.Z != 5000 || snap.Y != r.pa.Y {
		t.Errorf("Bert stands at world %d (%.0f, %.1f, %.0f), want where Anna stood", snap.World, snap.X, snap.Y, snap.Z)
	}
	x, z, world, ok := teleported(r.bert)
	if !ok || x != 5000 || z != 5000 || world != 0 {
		t.Errorf("Bert's client was told (%.0f, %.0f) world %d (%v)", x, z, world, ok)
	}
	r.want("Bert", r.msgs(r.bert), "chat:"+portalThrough, "teleport")
	r.want("Anna", r.msgs(r.anna), "chat:Bert answers your call.")

	// no call waits now, and Anna's next line is the room's
	r.say(r.anna, "Thanks for coming")
	r.want("Cora", r.msgs(r.cora), "chat:Anna: Thanks for coming")
	r.want("Bert, beside her now", r.msgs(r.bert), "chat:Anna: Thanks for coming")
}

// Across worlds, no group: the World's world and place change, the client is
// told the new world and its state channels held for the load, as a GM's summon
// does. The control: its world before the answer. Anyone may use the portal, not
// its owner alone.
func TestAPortalCallAcrossWorlds(t *testing.T) {
	r := newPortalRig(t)
	n := r.open()
	r.ask(r.cora, n) // not the owner
	r.say(r.cora, "Dora")
	r.want("Anna", r.msgs(r.anna))
	r.want("Cora", r.msgs(r.cora), "chat:You call Dora through the portal.")
	call := boxID(r.msgs(r.dora))
	if call == 0 {
		t.Fatal("no call to Dora")
	}
	if snap, _ := r.gw.PlayerSnapshot(r.pd.ServerID); snap.World != 2 {
		t.Fatalf("Dora in world %d before she answers", snap.World)
	}
	r.drain()
	r.now = r.now.Add(4 * time.Second)
	r.pick(r.dora, call, 0)
	r.tick()
	snap, _ := r.gw.PlayerSnapshot(r.pd.ServerID)
	if snap.World != 0 || snap.X != 5000 || snap.Z != 5000 {
		t.Errorf("Dora in world %d (%.0f, %.0f), want Tunaria, where Anna stood", snap.World, snap.X, snap.Z)
	}
	if _, _, world, ok := teleported(r.dora); !ok || world != 0 {
		t.Errorf("Dora's client told world %d (%v)", world, ok)
	}
	if !r.dora.StateChannelsHeld() {
		t.Error("Dora's state channels not held for the world's load")
	}
}

// A no tells the caller and moves no one; so does the box closed by hand. Either
// way the target is free to be called again. The control is the yes above.
func TestAPortalCallDeclined(t *testing.T) {
	for _, c := range []struct {
		name   string
		option byte
	}{{"No", 1}, {"the box closed", 255}} {
		t.Run(c.name, func(t *testing.T) {
			r := newPortalRig(t)
			n := r.open()
			r.ask(r.anna, n)
			r.say(r.anna, "Bert")
			call := boxID(r.msgs(r.bert))
			r.drain()
			r.now = r.now.Add(5 * time.Second)
			r.pick(r.bert, call, c.option)
			r.tick()
			r.want("Anna", r.msgs(r.anna), "chat:Bert declines your call.")
			r.want("Bert", r.msgs(r.bert))
			if snap, _ := r.gw.PlayerSnapshot(r.pb.ServerID); snap.X != 20000 {
				t.Errorf("Bert moved: x %.0f", snap.X)
			}
			// the old box answered again does nothing but say so
			r.pick(r.bert, call, 0)
			r.want("Bert", r.msgs(r.bert), "chat:"+portalPassed)
			r.tick()
			if snap, _ := r.gw.PlayerSnapshot(r.pb.ServerID); snap.X != 20000 {
				t.Errorf("Bert moved by an old box: x %.0f", snap.X)
			}
			// called again
			r.ask(r.anna, n)
			r.say(r.anna, "Bert")
			if boxID(r.msgs(r.bert)) == 0 {
				t.Error("no second call")
			}
		})
	}
}

// CLIENT's own decline (Msg0034, 255 at once: a menu was up) is no answer: the
// caller hears nothing, the target is told once who calls, the box goes out
// again until it shows, and then its yes counts. The control: the same 255 later
// is the box closed by hand (TestAPortalCallDeclined).
func TestAPortalCallDeclinedByTheClient(t *testing.T) {
	r := newPortalRig(t)
	n := r.open()
	r.ask(r.anna, n)
	r.say(r.anna, "Bert")
	call := boxID(r.msgs(r.bert))
	r.drain()

	r.now = r.now.Add(100 * time.Millisecond)
	r.pick(r.bert, call, 255)
	r.want("Anna", r.msgs(r.anna))
	r.want("Bert", r.msgs(r.bert), "chat:Anna calls you through a summoning portal. Close your menu to answer.")

	r.g.sweep() // too soon
	r.want("Bert", r.msgs(r.bert))
	r.now = r.now.Add(portalAskAgain)
	r.g.sweep()
	got := r.msgs(r.bert)
	if len(got) != 1 || boxID(got) != call {
		t.Fatalf("asked again: %q", got)
	}
	r.g.sweep() // once a decline
	r.want("Bert", r.msgs(r.bert))

	r.now = r.now.Add(100 * time.Millisecond)
	r.pick(r.bert, call, 255) // the menu still up: no second line
	r.want("Bert", r.msgs(r.bert))
	r.now = r.now.Add(portalAskAgain)
	r.g.sweep()
	r.drain()

	r.now = r.now.Add(2 * time.Second)
	r.pick(r.bert, call, 0)
	r.tick()
	if snap, _ := r.gw.PlayerSnapshot(r.pb.ServerID); snap.X != 5000 {
		t.Errorf("Bert's yes after the declines: x %.0f", snap.X)
	}
	r.want("Anna", r.msgs(r.anna), "chat:Bert answers your call.")
}

// The name's box: the answer and CLIENT's own decline leave the name awaited, a
// close by hand ends it, so the next line is chat. The controls are each other.
func TestAPortalNameBox(t *testing.T) {
	r := newPortalRig(t)
	n := r.open()

	// declined by CLIENT at once: told in a line, and the name still taken
	r.talk(r.anna, n)
	box := boxID(r.msgs(r.anna))
	r.now = r.now.Add(100 * time.Millisecond)
	r.pick(r.anna, box, 255)
	r.want("Anna", r.msgs(r.anna), "chat:"+portalListens)
	r.say(r.anna, "Bert")
	r.want("Cora", r.msgs(r.cora))
	if boxID(r.msgs(r.bert)) == 0 {
		t.Error("the name after CLIENT's decline not taken")
	}
	r.drain()

	// closed by hand: the wait ends, and a line is chat
	r.now = r.now.Add(PortalCallWait + time.Second) // Bert's call lapses too
	r.g.sweep()
	r.drain()
	r.talk(r.anna, n)
	box = boxID(r.msgs(r.anna))
	r.now = r.now.Add(5 * time.Second)
	r.pick(r.anna, box, 255)
	r.want("Anna", r.msgs(r.anna))
	r.say(r.anna, "Bert is late")
	r.want("Cora", r.msgs(r.cora), "chat:Anna: Bert is late")
	r.want("Bert", r.msgs(r.bert))
	r.drain()

	// a second talk while the first box is up: CLIENT declines the second, and
	// the first box's answer still counts
	r.talk(r.anna, n)
	first := boxID(r.msgs(r.anna))
	r.now = r.now.Add(100 * time.Millisecond)
	r.talk(r.anna, n)
	second := boxID(r.msgs(r.anna))
	if first != second {
		t.Fatalf("a second talk's box %08X, the first's %08X", second, first)
	}
	r.pick(r.anna, second, 255)
	r.drain()
	r.now = r.now.Add(5 * time.Second)
	r.pick(r.anna, first, 0)
	r.want("Anna", r.msgs(r.anna), "chat:"+portalListens)
	r.say(r.anna, "Bert")
	if boxID(r.msgs(r.bert)) == 0 {
		t.Error("the name after a double talk not taken")
	}
	r.drain()

	// an old name box answered with no wait says so
	r.pick(r.anna, first, 0)
	r.want("Anna", r.msgs(r.anna), "chat:"+portalDeaf)
}

// The refusals: each a red line to the caller alone, no box to anyone, and none
// counts against the rate. The control is the good call at the end.
func TestAPortalRefusals(t *testing.T) {
	r := newPortalRig(t)
	n := r.open()
	refused := func(name, want string) {
		t.Helper()
		r.ask(r.anna, n)
		r.say(r.anna, name)
		r.want(name+": Anna", r.msgs(r.anna), "red:"+want)
		for who, s := range map[string]*session.Session{"Bert": r.bert, "Cora": r.cora, "Dora": r.dora} {
			r.want(name+": "+who, r.msgs(s))
		}
		if len(r.g.calls) != 0 {
			t.Fatalf("%s: a call stands: %v", name, r.g.calls)
		}
	}
	refused("Nobody", `No player named "Nobody" is online.`)
	refused("  nobody  ", `No player named "nobody" is online.`)
	refused("anna", "You cannot call yourself.")
	r.pb.HP = 0
	refused("Bert", "Bert is dead and cannot answer a call.")
	r.pb.HP = 100
	// a world the target's game does not have: Anna in S2's world 7, Bert on the disc's six
	r.gw.UpdatePosition(r.pa.ServerID, 5000, 50, 5000, 0)
	r.anna.SetDiscVersion(protocol.GameVersionSandstormS2Live)
	r.pa.World = 7
	far := r.open()
	refused2 := func(name, want string) {
		t.Helper()
		r.ask(r.anna, far)
		r.say(r.anna, name)
		r.want(name+": Anna", r.msgs(r.anna), "red:"+want)
		r.want(name+": Bert", r.msgs(r.bert))
	}
	refused2("Bert", "Bert's game cannot load this world.")
	r.pa.World = 0
	n = r.open()
	// no refusal counted: the good call goes at once
	r.ask(r.anna, n)
	r.say(r.anna, "Bert")
	r.want("Anna", r.msgs(r.anna), "chat:You call Bert through the portal.")
	if boxID(r.msgs(r.bert)) == 0 {
		t.Error("the good call after the refusals got no box")
	}
}

// A target holding an unanswered call: the second caller is told, and the target
// holds one box. A caller may not call again within PortalCallEvery; past it, it
// may. The controls: the first call's box, the call past the limit.
func TestAPortalBusyTargetAndRateLimit(t *testing.T) {
	r := newPortalRig(t)
	n := r.open()
	r.ask(r.anna, n)
	r.say(r.anna, "Bert")
	first := boxID(r.msgs(r.bert))
	if first == 0 {
		t.Fatal("no first call")
	}
	r.ask(r.cora, n)
	r.say(r.cora, "bert")
	r.want("Cora", r.msgs(r.cora), "red:Bert is already answering a call. Try again in a moment.")
	r.want("Bert", r.msgs(r.bert))
	r.drain()

	// Anna's next call, within the limit (ask's own time is what passes)
	r.g.called[r.pa.ServerID] = r.now
	r.ask(r.anna, n)
	r.g.called[r.pa.ServerID] = r.now
	r.say(r.anna, "Dora")
	r.want("Anna", r.msgs(r.anna), "red:"+portalGathering)
	r.want("Dora", r.msgs(r.dora))
	r.ask(r.anna, n)
	r.now = r.now.Add(PortalCallEvery)
	r.say(r.anna, "Dora")
	r.want("Anna", r.msgs(r.anna), "chat:You call Dora through the portal.")
	if boxID(r.msgs(r.dora)) == 0 {
		t.Error("the call past the limit got no box")
	}
	// the mark of the old box still answers the old call: Bert's is live
	r.drain()
	r.now = r.now.Add(time.Second)
	r.pick(r.bert, first, 1)
	r.want("Anna", r.msgs(r.anna), "chat:Bert declines your call.")
}

// A /command or a #command is a command while a name is awaited, and the name
// is still awaited after it: a tell, a roll, an unknown /word, a non-GM's #word.
// /g, /s, /gu and /say reach the server as the mode, the bare text and the mode
// back (CLIENT's ChatLine), and go where they said. The control: the plain line
// after them is the name.
func TestAPortalNameLeavesCommandsAlone(t *testing.T) {
	r := newPortalRig(t)
	n := r.open()
	r.ask(r.anna, n)

	r.say(r.anna, "/t Bert hello")
	got := r.msgs(r.bert)
	if !hasMsg(got, "colour:Anna tells you, 'hello'") {
		t.Errorf("the tell: %q", got)
	}
	if !hasMsg(r.msgs(r.anna), "colour:You told Bert, 'hello'") {
		t.Error("the tell's copy to its sender")
	}

	r.say(r.anna, "#roll")
	if got := r.msgs(r.cora); len(got) != 1 || !strings.Contains(got[0], "Anna rolls") {
		t.Errorf("#roll: Cora heard %q", got)
	}
	r.drain()
	r.say(r.anna, "/roll")
	if got := r.msgs(r.cora); len(got) != 1 || !strings.Contains(got[0], "Anna rolls") {
		t.Errorf("/roll: Cora heard %q", got)
	}
	r.drain()

	r.say(r.anna, "/nosuchword")
	r.want("Anna", r.msgs(r.anna), `red:Unrecognized command "/nosuchword".`)
	r.say(r.anna, "#goto freeport")
	r.want("Anna", r.msgs(r.anna), `red:Unrecognized command "#goto".`) // a non-GM's
	r.want("Cora", r.msgs(r.cora))
	r.drain()

	// /g hi from a player in say mode, in a group
	r.group(t, r.anna, r.pa, r.cora, r.pc)
	r.drain()
	r.mode(r.anna, ChatModeGroup)
	r.say(r.anna, "hi team")
	r.mode(r.anna, ChatModeSay)
	if ls := heard(t, r.cora); len(ls) != 1 || ls[0].text != "Anna tells the group: hi team" {
		t.Errorf("/g: Cora heard %q", texts(ls))
	}
	r.drain()

	// /s hi
	r.mode(r.anna, ChatModeShout)
	r.send(r.anna, protocol.OpcodeClientShout, append([]byte{2}, wide("hi all", 0xFFFFFFFF)...))
	r.mode(r.anna, ChatModeSay)
	if got := r.msgs(r.bert); len(got) != 1 || got[0] != "colour:Anna shouts: hi all" {
		t.Errorf("/s: Bert heard %q", got)
	}
	r.drain()

	// /say hi, in say mode: the mode, the text, the mode
	r.mode(r.anna, ChatModeSay)
	r.say(r.anna, "hi room")
	r.mode(r.anna, ChatModeSay)
	r.want("Cora", r.msgs(r.cora), "chat:Anna: hi room")
	r.drain()

	// and the name, typed a moment later
	r.now = r.now.Add(3 * time.Second)
	r.say(r.anna, "Bert")
	r.want("Cora", r.msgs(r.cora))
	r.want("Anna", r.msgs(r.anna), "chat:You call Bert through the portal.")
	if boxID(r.msgs(r.bert)) == 0 {
		t.Error("the name after the commands not taken")
	}
}

// A player in shout or guild mode types the name on those opcodes (CLIENT's
// ChatLine: 0x0C01 and 0x0578): taken there, shouted to no one, filed with no
// guild. The controls: the same line with nothing awaited is shouted, or refused
// for a guild it lacks.
func TestAPortalNameInShoutAndGuildMode(t *testing.T) {
	r := newPortalRig(t)
	n := r.open()
	shout := func(text string) {
		t.Helper()
		r.send(r.anna, protocol.OpcodeClientShout, append([]byte{2}, wide(text, 0xFFFFFFFF)...))
	}
	guild := func(text string) {
		t.Helper()
		if err := r.gh.HandleGuildCommand(r.anna, guildCmd(guildCmdChat, wideb(text))); err != nil {
			t.Fatal(err)
		}
	}

	shout("Bert")
	if got := r.msgs(r.cora); len(got) != 1 || got[0] != "colour:Anna shouts: Bert" {
		t.Fatalf("the control: a shout with nothing awaited: %q", got)
	}
	r.want("Bert", r.msgs(r.bert), "colour:Anna shouts: Bert")
	r.drain()

	r.mode(r.anna, ChatModeShout)
	r.now = r.now.Add(5 * time.Second)
	r.ask(r.anna, n)
	shout("Bert")
	r.want("Cora", r.msgs(r.cora))
	r.want("Anna", r.msgs(r.anna), "chat:You call Bert through the portal.")
	first := boxID(r.msgs(r.bert))
	if first == 0 {
		t.Fatal("the name in shout mode got no box")
	}
	r.pick(r.bert, first, 1)
	r.drain()

	guild("Bert")
	if got := r.msgs(r.anna); len(got) != 1 || got[0] != fmt.Sprintf("op:%04X", uint16(protocol.OpcodeGuildCommandFailed)) {
		t.Fatalf("the control: a guild line with nothing awaited and no guild: %q", got)
	}
	r.drain()

	r.mode(r.anna, ChatModeGuild)
	r.now = r.now.Add(PortalCallEvery)
	r.ask(r.anna, n)
	guild("Bert")
	r.want("Anna", r.msgs(r.anna), "chat:You call Bert through the portal.")
	if boxID(r.msgs(r.bert)) == 0 {
		t.Error("the name in guild mode got no box")
	}
}

// With nothing awaited a line is chat, untouched, whatever it says: a player's
// own name calls no one. The control for "untouched" is that everyone in range
// hears it; the rig holds no call afterwards.
func TestChatPassesAPortalWithNothingAwaited(t *testing.T) {
	r := newPortalRig(t)
	r.open() // a portal stands, unspoken to
	r.say(r.anna, "Bert")
	r.want("Cora", r.msgs(r.cora), "chat:Anna: Bert")
	r.want("Anna", r.msgs(r.anna), "chat:Anna: Bert")
	r.want("Bert", r.msgs(r.bert))
	r.say(r.cora, "hello there")
	r.want("Anna", r.msgs(r.anna), "chat:Cora: hello there")
	if len(r.g.calls)+len(r.g.waits) != 0 {
		t.Errorf("calls %d, waits %d", len(r.g.calls), len(r.g.waits))
	}
	// and a chat handler with no portal at all
	c := newChatWorld(t)
	s, _ := c.join(9, "Lone", 5000, 5000, 0)
	c.say(s, "hello")
	if ls := heard(t, s); len(ls) != 1 || ls[0].text != "Lone: hello" {
		t.Errorf("no portal: %q", texts(ls))
	}
}

// The wait for a name ends at its minute, when the portal closes or when the
// player walks off, each said by the sweep; the next line is then chat. The
// controls: a moment before its minute the name is taken.
func TestAPortalWaitEnds(t *testing.T) {
	t.Run("a moment before its minute", func(t *testing.T) {
		r := newPortalRig(t)
		n := r.open()
		r.ask(r.anna, n)
		r.now = r.now.Add(PortalNameWait - 3*time.Second)
		r.g.sweep()
		r.want("Anna", r.msgs(r.anna))
		r.say(r.anna, "Bert")
		if boxID(r.msgs(r.bert)) == 0 {
			t.Error("not taken")
		}
	})
	t.Run("its minute", func(t *testing.T) {
		r := newPortalRig(t)
		n := r.open()
		r.ask(r.anna, n)
		r.now = r.now.Add(PortalNameWait + time.Second)
		r.g.sweep()
		r.want("Anna", r.msgs(r.anna), "chat:"+portalDeaf)
		r.say(r.anna, "Bert")
		r.want("Cora", r.msgs(r.cora), "chat:Anna: Bert")
		r.want("Bert", r.msgs(r.bert))
	})
	t.Run("its minute with no sweep yet", func(t *testing.T) {
		r := newPortalRig(t)
		n := r.open()
		r.ask(r.anna, n)
		r.now = r.now.Add(PortalNameWait + time.Second)
		r.say(r.anna, "Bert")
		r.want("Cora", r.msgs(r.cora), "chat:Anna: Bert")
		r.want("Bert", r.msgs(r.bert))
	})
	t.Run("the portal closed", func(t *testing.T) {
		r := newPortalRig(t)
		n := r.open()
		r.ask(r.cora, n)
		r.open() // its owner uses the item again: another portal, this one gone
		r.g.sweep()
		r.want("Cora", r.msgs(r.cora), "chat:"+portalDeaf)
		r.say(r.cora, "Bert")
		r.want("Anna", r.msgs(r.anna), "chat:Cora: Bert")
		r.want("Bert", r.msgs(r.bert))
	})
	t.Run("walked off", func(t *testing.T) {
		r := newPortalRig(t)
		n := r.open()
		r.ask(r.cora, n)
		r.gw.UpdatePosition(r.pc.ServerID, 5000, 50, 5000+entity.PortalReach+10, 0)
		r.say(r.cora, "Bert")
		r.want("Anna", r.msgs(r.anna), "chat:Cora: Bert")
		r.want("Bert", r.msgs(r.bert))
	})
}

// A call ends at its minute, when the portal closes or when either side leaves,
// each told what is left of them, and a late yes moves no one. The control: a
// yes in time (TestAPortalCallEndToEnd).
func TestAPortalCallEnds(t *testing.T) {
	stays := func(r *portalRig, call uint32) {
		t.Helper()
		r.pick(r.bert, call, 0)
		r.tick()
		if snap, _ := r.gw.PlayerSnapshot(r.pb.ServerID); snap.X != 20000 {
			t.Errorf("Bert moved: x %.0f", snap.X)
		}
		r.drain()
	}
	begin := func(t *testing.T) (*portalRig, uint32) {
		r := newPortalRig(t)
		n := r.open()
		r.ask(r.cora, n)
		r.say(r.cora, "Bert")
		call := boxID(r.msgs(r.bert))
		if call == 0 {
			t.Fatal("no call")
		}
		r.drain()
		return r, call
	}
	t.Run("its minute", func(t *testing.T) {
		r, call := begin(t)
		r.now = r.now.Add(PortalCallWait - time.Second)
		r.g.sweep()
		r.want("Cora", r.msgs(r.cora))
		r.now = r.now.Add(2 * time.Second)
		r.g.sweep()
		r.want("Cora", r.msgs(r.cora), "chat:Bert does not answer your call.")
		r.want("Bert", r.msgs(r.bert), "chat:Cora's call has passed.")
		stays(r, call)
	})
	t.Run("the portal closed", func(t *testing.T) {
		r, call := begin(t)
		r.now = r.now.Add(time.Second)
		r.open() // Anna's second use
		r.g.sweep()
		r.want("Cora", r.msgs(r.cora), "chat:"+portalClosed)
		r.want("Bert", r.msgs(r.bert), "chat:"+portalClosed)
		stays(r, call)
	})
	t.Run("the portal closed with no sweep yet", func(t *testing.T) {
		r, call := begin(t)
		r.now = r.now.Add(5 * time.Second)
		r.open()
		r.pick(r.bert, call, 0)
		r.want("Bert", r.msgs(r.bert), "red:"+portalClosed)
		r.want("Cora", r.msgs(r.cora), "red:"+portalClosed)
		r.tick()
		if snap, _ := r.gw.PlayerSnapshot(r.pb.ServerID); snap.X != 20000 {
			t.Errorf("Bert moved: x %.0f", snap.X)
		}
	})
	t.Run("the caller left", func(t *testing.T) {
		r, call := begin(t)
		r.gw.RemovePlayerBySession(r.cora)
		r.g.sweep()
		r.want("Bert", r.msgs(r.bert), "chat:Cora's call has ended.")
		stays(r, call)
	})
	t.Run("the target left", func(t *testing.T) {
		r, _ := begin(t)
		r.gw.RemovePlayerBySession(r.bert)
		r.g.sweep()
		r.want("Cora", r.msgs(r.cora), "chat:Bert left before answering your call.")
		if len(r.g.calls) != 0 {
			t.Errorf("%d calls stand", len(r.g.calls))
		}
	})
	t.Run("the target died", func(t *testing.T) {
		r, call := begin(t)
		r.pb.HP = 0
		r.now = r.now.Add(3 * time.Second)
		r.pick(r.bert, call, 0)
		r.tick()
		r.want("Bert", r.msgs(r.bert), "red:"+portalYouDead)
		r.want("Cora", r.msgs(r.cora), "red:Bert cannot come.")
		if snap, _ := r.gw.PlayerSnapshot(r.pb.ServerID); snap.X != 20000 {
			t.Errorf("Bert moved: x %.0f", snap.X)
		}
	})
	t.Run("no place to land", func(t *testing.T) {
		r := newPortalRig(t)
		r.gw.UpdatePosition(r.pa.ServerID, 5000, entity.WorldFloorY-1, 5000, 0) // Anna falling under the world
		n := r.open()
		r.ask(r.anna, n)
		r.say(r.anna, "Bert")
		call := boxID(r.msgs(r.bert))
		if call == 0 {
			t.Fatal("no call")
		}
		r.drain()
		r.now = r.now.Add(3 * time.Second)
		r.pick(r.bert, call, 0)
		r.tick()
		r.want("Bert", r.msgs(r.bert), "red:"+portalClosed)
		r.want("Anna", r.msgs(r.anna), "red:Bert cannot come.")
		if snap, _ := r.gw.PlayerSnapshot(r.pb.ServerID); snap.X != 20000 {
			t.Errorf("Bert moved: x %.0f", snap.X)
		}
	})
}

// A coach list open on the target's screen is not what its yes answers: the box
// goes by its id. The control: a pick with no box mark still picks from the list.
func TestAPortalBoxIsNotACoachPick(t *testing.T) {
	r := newPortalRig(t)
	n := r.open()
	r.ask(r.anna, n)
	r.say(r.anna, "Bert")
	call := boxID(r.msgs(r.bert))
	if !IsPortalBox(call) || IsGenieBox(call) || IsTriggerBox(call) || IsDyerBox(call) {
		t.Fatalf("box %08X", call)
	}
	r.pb.PendingCoachDests = []entity.CoachDest{{Name: "Somewhere", World: 0, X: 1, Y: 2, Z: 3}}
	r.drain()
	r.now = r.now.Add(4 * time.Second)
	r.pick(r.bert, call, 1)
	if len(r.pb.PendingCoachDests) != 1 {
		t.Error("the coach list was answered")
	}
	r.want("Anna", r.msgs(r.anna), "chat:Bert declines your call.")
}

// With PortalAsksFirst off (the owner's one line) a named player comes at once:
// no box. The rule as shipped asks.
func TestAPortalStraightAway(t *testing.T) {
	if !PortalAsksFirst {
		t.Error("shipped without the accept box: the owner was told it asks")
	}
	r := newPortalRig(t)
	r.g.askFirst = false
	n := r.open()
	r.ask(r.anna, n)
	r.say(r.anna, "bert")
	got := r.msgs(r.bert)
	if boxID(got) != 0 {
		t.Errorf("a box when told not to ask: %q", got)
	}
	r.want("Bert", got, "chat:"+portalThrough)
	r.tick()
	if snap, _ := r.gw.PlayerSnapshot(r.pb.ServerID); snap.X != 5000 {
		t.Errorf("Bert not brought: x %.0f", snap.X)
	}
}

// The item from the item menu and from a quick slot (0x003E, either way): its
// portal, its line, the item kept; on the gem quick slot as the fang is. The
// controls: another gem opens nothing, and the dead open nothing.
func TestUsingTheItemOpensThePortal(t *testing.T) {
	f := newScribeFixture(t)
	w := entity.NewWorld()
	w.SessionManager = session.NewManager()
	f.player.HP, f.player.MaxHP = 100, 100
	w.Players[f.player.ServerID] = f.player
	f.sess.SetECSEntity(f.player)
	f.h.portal = NewPortal(w)
	insert := func(name string, procanim int) int {
		res, err := f.db.Exec(`INSERT INTO itempattern (itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
			VALUES (0, 0, 0, 0, -1, 0, 1, 1, 0, 0, 0, 0, 1, 1, 0, 80, 32767, 32767, ?, 0, 0, 0, ?, '', 0, 0)`, procanim, name)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return int(id)
	}
	stone := insert("Spitestone", 7)
	portal := insert(SummoningPortalName, 7)
	f.own(stone, 1)
	f.own(portal, 1)
	f.loadList()
	count := func() (n int, id entity.EntityID) {
		for _, npc := range w.NPCs {
			if npc.PortalOf == f.player.ServerID {
				n++
				id = npc.ServerID
			}
		}
		return n, id
	}
	key := func(pattern int) uint32 {
		t.Helper()
		rows, err := f.inv.GetCharacterItems(f.sid)
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			if rows[i].Pattern.PatternID == pattern {
				return uint32(i)
			}
		}
		t.Fatalf("no item %d", pattern)
		return 0
	}
	if f.use(key(stone)); func() bool { n, _ := count(); return n != 0 }() {
		t.Fatal("another gem opened a portal")
	}
	sent := f.use(key(portal))
	n, first := count()
	if n != 1 || f.stackOf(SummoningPortalName) != 1 {
		t.Fatalf("the menu's Use: %d portals, %d items left", n, f.stackOf(SummoningPortalName))
	}
	if len(sent) != 1 || opOf(sent[0]) != protocol.OpcodeClientMessage {
		t.Errorf("the menu's Use sent %s, want its line alone", ops(sent))
	}
	// a quick slot's press: UseItem(i, 1), on the third slot a gem takes
	rows, _ := f.inv.GetCharacterItems(f.sid)
	if !quickSlotFits(&rows[key(portal)], 2) {
		t.Error("a gem not on the gem quick slot")
	}
	quick := build(protocol.OpcodeInteractItem, 16, func(w *buffer.Writer) {
		w.WriteUint32(key(portal))
		w.WriteUint32(0xFFFFFFFF)
		w.WriteByte(1)
	})[2:]
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeInteractItem)
	msg.SetData(quick)
	if err := f.h.HandleInteractItem(f.sess, msg); err != nil {
		t.Fatal(err)
	}
	n, second := count()
	if n != 1 || second == first || f.stackOf(SummoningPortalName) != 1 {
		t.Fatalf("the quick slot's: %d portals (%d, was %d), %d items left", n, second, first, f.stackOf(SummoningPortalName))
	}
	if IsSummoningPortal(&database.ItemPatternRecord{EquipSlot: 18, ItemName: SummoningPortalName}) ||
		!IsSummoningPortal(&database.ItemPatternRecord{EquipSlot: -1, ItemName: "summoning portal "}) {
		t.Error("IsSummoningPortal")
	}
	// the dead open nothing and hear nothing
	f.sess.Queue().GetPending()
	f.player.HP = 0
	if sent := f.use(key(portal)); len(sent) != 0 {
		t.Errorf("the dead's Use sent %s", ops(sent))
	}
	if n, id := count(); n != 1 || id != second {
		t.Errorf("the dead's Use: %d portals", n)
	}
}

// The item at the login: a character holding none gets one, never a second, and
// none without the record (the control). The rest is the lamp's own giveKeptItem.
func TestALoginGivesTheSummoningPortal(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/portal.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	rec := &database.CharacterRecord{CharName: "Holder", AccountID: 1}
	if err := store.Create(rec); err != nil {
		t.Fatal(err)
	}
	h := NewCharacterHandler(store, nil, database.NewInventoryStore(db), database.NewSpellbookStore(db))
	count := func() int {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM charinventory c JOIN itempattern p ON p.patternid = c.patternid
			WHERE c.serverid = ? AND p.itemname = ?`, rec.ServerID, SummoningPortalName).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	h.giveSummoningPortal(rec.ServerID)
	if count() != 0 {
		t.Fatal("an item with no record")
	}
	if _, err := db.Exec(`INSERT INTO itempattern (itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, 0, 0, -1, 0, 1, 1, 0, 0, 0, 0, 1, 1, 0, 80, 32767, 32767, 7, 0, 0, 0, ?, '', 0, 0)`, SummoningPortalName); err != nil {
		t.Fatal(err)
	}
	h.giveSummoningPortal(rec.ServerID)
	h.giveSummoningPortal(rec.ServerID)
	if count() != 1 {
		t.Fatalf("%d items, want one", count())
	}
}
