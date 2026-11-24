package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/server/internal/shardchat"
)

// A receipt from an absent target is also a fence: the peer has finished
// delivering all earlier shouts on this connection.
func fenceShardChat(t *testing.T, b *shardchat.Bridge) {
	t.Helper()
	probe := shardchat.Message{Channel: "tell", From: "Probe", To: "Unoccupied", Text: "ready"}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := b.Send(probe); err == nil {
			return
		} else if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}
}

func linkChatWorlds(t *testing.T, sandbox, vanilla *chatWorld) {
	t.Helper()
	sandbox.h.shards = shardchat.New(shardchat.Sandbox, sandbox.h.receiveShardChat)
	vanilla.h.shards = shardchat.New(shardchat.Vanilla, vanilla.h.receiveShardChat)
	ctx, cancel := context.WithCancel(context.Background())
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err == nil {
			_ = vanilla.h.shards.Serve(ctx, c)
		}
	}))
	t.Cleanup(target.Close)
	t.Cleanup(cancel)
	c, _, err := websocket.Dial(ctx, target.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	go sandbox.h.shards.Serve(ctx, c)
	fenceShardChat(t, sandbox.h.shards)
}

// Check every queued chat packet (including the reply opcode), then consume it.
func wantShardLines(t *testing.T, s *session.Session, want ...line) {
	t.Helper()
	if got := heard(t, s); !slices.Equal(got, want) {
		t.Fatalf("chat packets = %+v, want %+v", got, want)
	}
	drain(s)
}

func tellPacket(text string) line {
	return line{op: protocol.OpcodeColoredChat, text: text, rgb: [3]byte{tellR, tellG, tellB}}
}

func TestShardChatAddressesAndReply(t *testing.T) {
	for _, tc := range []struct{ home, peer, homeLong, peerLong string }{
		{"s", "v", "sandbox", "vanilla"},
		{"v", "s", "vanilla", "sandbox"},
	} {
		t.Run(tc.home, func(t *testing.T) {
			sandbox, vanilla := newChatWorld(t), newChatWorld(t)
			linkChatWorlds(t, sandbox, vanilla)
			local, remote := sandbox, vanilla
			if tc.home == "v" {
				local, remote = vanilla, sandbox
			}
			speaker, _ := local.join(1, "Speaker", 0, 0, 0)
			localTwin, _ := local.join(2, "Twin", 0, 0, 0)
			otherSpeaker, _ := remote.join(1, "Speaker", 0, 0, 0)
			remoteTwin, _ := remote.join(2, "Twin", 0, 0, 1)
			remoteOnly, _ := remote.join(3, "Visitor", 0, 0, 0)
			sender := tc.home + ".Speaker"
			target := tc.peer + ".Twin"

			for _, address := range []string{"twin", tc.home + ".twin", tc.homeLong + ".twin"} {
				local.say(speaker, "/tell "+address+" local")
				wantShardLines(t, localTwin, line{op: protocol.OpcodeReplyName, text: "Speaker"}, tellPacket("Speaker tells you, 'local'"))
				wantShardLines(t, speaker, tellPacket("You told Twin, 'local'"))
				wantShardLines(t, remoteTwin)
			}
			for _, address := range []string{strings.ToUpper(tc.peerLong) + ".tWiN", tc.peer + ".twin"} {
				// "duel" is normally a PS2 command. In a qualified tell it is the message.
				local.say(speaker, "/tell "+address+" duel")
				reply := heard(t, remoteTwin)
				wantShardLines(t, remoteTwin, line{op: protocol.OpcodeReplyName, text: sender}, tellPacket(sender+" tells you, 'duel'"))
				wantShardLines(t, speaker, tellPacket("You told "+target+", 'duel'"))
				// Both clients turn /r and /reply into /t plus this exact opcode's name.
				remote.say(remoteTwin, "/t "+reply[0].text+" back")
				wantShardLines(t, speaker, line{op: protocol.OpcodeReplyName, text: target}, tellPacket(target+" tells you, 'back'"))
				wantShardLines(t, remoteTwin, tellPacket("You told "+sender+", 'back'"))
				wantShardLines(t, otherSpeaker)
				wantShardLines(t, localTwin)
			}
			local.say(speaker, "/tell Visitor secret")
			wantShardLines(t, speaker, line{op: protocol.OpcodeErrorMessage, text: "Visitor is not connected."})
			wantShardLines(t, remoteOnly)
			remoteTwin.SetCharacterInWorld(false)
			local.say(speaker, "/tell "+target+" private")
			wantShardLines(t, speaker, line{op: protocol.OpcodeErrorMessage, text: target + " is not connected."})
			wantShardLines(t, localTwin)
			wantShardLines(t, remoteTwin)
		})
	}
}

func TestShardChatShoutsAndSilence(t *testing.T) {
	sandbox, vanilla := newChatWorld(t), newChatWorld(t)
	linkChatWorlds(t, sandbox, vanilla)
	s, sp := sandbox.join(1, "Twin", 0, 0, 0)
	v, _ := vanilla.join(1, "Twin", 0, 0, 0)
	s2, _ := sandbox.join(2, "Other", 2000, 2000, 1)
	v2, _ := vanilla.join(2, "Other", 2000, 2000, 1)
	rgb := [3]byte{0x3f, 0xff, 0x3f}
	vanilla.send(v, protocol.OpcodeClientShout, append([]byte{2}, wide("first", 0)...))
	fenceShardChat(t, vanilla.h.shards)
	sandbox.mode(s, ChatModeShout)
	sandbox.say(s, "second")
	fenceShardChat(t, sandbox.h.shards)
	for _, session := range []*session.Session{s, s2} {
		wantShardLines(t, session, line{op: protocol.OpcodeColoredChat, text: "v.Twin shouts: first", rgb: rgb},
			line{op: protocol.OpcodeColoredChat, text: "Twin shouts: second", rgb: rgb})
	}
	for _, session := range []*session.Session{v, v2} {
		wantShardLines(t, session, line{op: protocol.OpcodeColoredChat, text: "Twin shouts: first", rgb: rgb},
			line{op: protocol.OpcodeColoredChat, text: "s.Twin shouts: second", rgb: rgb})
	}
	sandbox.h.silenceChar(sp.CharServerID)
	sandbox.say(s, "silenced shout")
	sandbox.say(s, "/tell v.Twin silenced tell")
	fenceShardChat(t, sandbox.h.shards)
	muted := line{op: protocol.OpcodeErrorMessage, text: "You have been silenced by a GM."}
	wantShardLines(t, s, muted, muted)
	wantShardLines(t, v)
	wantShardLines(t, v2)
	wantShardLines(t, s2)
}

func TestShardChatPeerDown(t *testing.T) {
	c := newChatWorld(t)
	c.h.shards = shardchat.New(shardchat.Vanilla, c.h.receiveShardChat)
	a, _ := c.join(1, "Speaker", 0, 0, 0)
	b, _ := c.join(2, "Twin", 0, 0, 0)
	c.say(a, "/tell sandbox.Twin remote")
	wantShardLines(t, a, line{op: protocol.OpcodeErrorMessage, text: "Could not confirm delivery to s.Twin."})
	wantShardLines(t, b)
	c.say(a, "/tell vanilla.Twin local")
	wantShardLines(t, b, line{op: protocol.OpcodeReplyName, text: "Speaker"}, tellPacket("Speaker tells you, 'local'"))
	wantShardLines(t, a, tellPacket("You told Twin, 'local'"))
	c.mode(a, ChatModeShout)
	c.say(a, "anyone here?")
	wantShardLines(t, b, line{op: protocol.OpcodeColoredChat, text: "Speaker shouts: anyone here?", rgb: [3]byte{0x3f, 0xff, 0x3f}})
}

func TestShardChatAwayAndReplyTargetChanges(t *testing.T) {
	sandbox, vanilla := newChatWorld(t), newChatWorld(t)
	linkChatWorlds(t, sandbox, vanilla)
	const name = "Abcdefghijklmno" // the longest creatable name
	remote, _ := sandbox.join(1, name, 0, 0, 0)
	local, _ := vanilla.join(1, name, 0, 0, 0)
	recipient, _ := vanilla.join(2, "Twin", 0, 0, 0)
	vanilla.say(recipient, "/afk back soon")
	drain(recipient)
	sandbox.say(remote, "/tell v.Twin hello")
	wantShardLines(t, recipient, line{op: protocol.OpcodeReplyName, text: "s." + name}, tellPacket("s."+name+" tells you, 'hello'"))
	wantShardLines(t, remote, tellPacket("You told v.Twin, 'hello'"), tellPacket("v.Twin is away: back soon"))
	sandbox.say(remote, "/tell vanilla.Twin again")
	wantShardLines(t, remote, tellPacket("You told v.Twin, 'again'"))
	drain(recipient)
	vanilla.say(local, "/tell Twin here")
	wantShardLines(t, recipient, line{op: protocol.OpcodeReplyName, text: name}, tellPacket(name+" tells you, 'here'"))
	wantShardLines(t, local, tellPacket("You told Twin, 'here'"), tellPacket("Twin is away: back soon"))
}

func TestShardChatItemLinks(t *testing.T) {
	sandbox, vanilla := newLinkWorld(t), newLinkWorld(t)
	linkChatWorlds(t, sandbox.chatWorld, vanilla.chatWorld)
	s, sp := sandbox.join(1, "Sender", 0, 0, 0)
	v, _ := vanilla.join(1, "Viewer", 0, 0, 0)
	s.SetItemLinks(true)
	v.SetItemLinks(true)
	const sword = "Rusty Sword"
	item := sandbox.own(sp.CharServerID, sandbox.item(sword, 1, 50, 5), 1, 37, 0)
	sandbox.lists(s, sp.CharServerID)
	row, err := sandbox.inv.GetCharItem(sp.CharServerID, item)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		route byte
		typed string
		want  string
	}{
		{itemLinkRouteShout, "look Rusty Sword", "s.Sender shouts: look Rusty Sword"},
		{itemLinkRouteSay, "/t vanilla.Viewer my Rusty Sword", "s.Sender tells you, 'my Rusty Sword'"},
	} {
		sandbox.linkSay(s, tc.route, noTarget, tc.typed, linkOn(tc.typed, sword, 0, 0))
		fenceShardChat(t, sandbox.h.shards)
		spans, lines := linksTo(t, v)
		if len(spans) != 1 || !slices.Equal(lines, []string{tc.want}) {
			t.Fatalf("item spans %+v on %q, want a link on %q", spans, lines, tc.want)
		}
		l := spans[0]
		if tc.want[l.at:l.at+l.n] != sword {
			t.Fatalf("wrong item span %+v", l)
		}
		if status, data := vanilla.ask(v, l.id); status != 1 || !bytes.Equal(data, recordOf(row, l.id)) {
			t.Fatalf("remote inspect did not return the sender's item (status %d)", status)
		}
	}
}
