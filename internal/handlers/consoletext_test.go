package handlers

import (
	"strings"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// consoleOrdinary is what a PS2's keyboard can type, which every relay must pass
// byte for byte. consoleHostile is what none can: the run of `|x` that kills a
// PS2's text box (a gram of 360 characters of it did, in the tracer), control
// characters, a line end and a tab, other scripts, a surrogate pair's worth.
const consoleOrdinary = `Hail Lord Nagafen! 50% (of 1,000): don't-stop/go? a=b; c+d*e @f_g ~h "i".`

var consoleHostile = strings.Repeat("|+", 180) + " hi\x01\x7f\n\tthereé中\U0001F600#&<|"

// heldToAConsole reports what in text a PS2's keyboard could not have typed.
func heldToAConsole(t *testing.T, what, text string, most int) {
	t.Helper()
	if text == "" {
		t.Errorf("%s: nothing came", what)
	}
	if len(text) > most {
		t.Errorf("%s: %d long, at most %d", what, len(text), most)
	}
	for _, c := range text {
		if !consoleTypable(c) {
			t.Errorf("%s: %q in %q", what, c, text)
			return
		}
	}
}

func TestConsoleText(t *testing.T) {
	if got := consoleLine(consoleOrdinary, chatLineMost); got != consoleOrdinary {
		t.Errorf("ordinary text changed: %q", got)
	}
	if got := consoleBlock("line one\nline two", gramTextMost); got != "line one\nline two" {
		t.Errorf("a block's line end: %q", got)
	}
	for in, want := range map[string]string{
		"|+|t||x|":               "+tx",
		"a\r\nb\nc\rd\te":        "a b c d e",
		"a$b&c<d>e[f]g":          "abcdefg",
		"Hail Maeyni Do`Traelix": "Hail Maeyni Do`Traelix",
		"Hail Zombie #22":        "Hail Zombie #22",
		"é中\U0001F600":           "",
		"||||":                   "",
		"a\x00b\x07c":            "abc",
	} {
		if got := consoleLine(in, chatLineMost); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	if got := consoleBlock("a\r\nb\rc\n", gramTextMost); got != "a\nb\nc\n" {
		t.Errorf("a block's line ends: %q", got)
	}
	if got := consoleLine(strings.Repeat("a", 200), chatLineMost); len(got) != chatLineMost {
		t.Errorf("a long line: %d", len(got))
	}
	// the cap counts what is kept, not what was thrown away
	if got := consoleLine(strings.Repeat("|", 500)+"abc", chatLineMost); got != "abc" {
		t.Errorf("past a run of bars: %q", got)
	}
	// each of a console's keys, and nothing beside them, out of all of ASCII
	var kept strings.Builder
	for c := rune(0); c < 0x80; c++ {
		if consoleTypable(c) {
			kept.WriteRune(c)
		}
	}
	if want := " !\"#%'()*+,-./0123456789:;=?@ABCDEFGHIJKLMNOPQRSTUVWXYZ_`abcdefghijklmnopqrstuvwxyz~"; kept.String() != want {
		t.Errorf("a console's keys:\n %s\nwant\n %s", kept.String(), want)
	}
}

// Every chat channel: what is heard by another of a hostile line is what a PS2's
// keyboard could type and no longer than its chat line, an ordinary line comes
// through byte for byte, and a line of nothing else is not relayed at all.
func TestChatRelaysHoldToAConsole(t *testing.T) {
	c := newChatWorld(t)
	as, pa := c.join(1, "Ann", 25000, 16000, 0)
	bs, pb := c.join(2, "Bob", 25010, 16000, 0)
	c.group(t, as, pa, bs, pb)
	pa.GuildID, pb.GuildID = 7, 7
	target := c.gw.IDSeenBy(as, pb)
	channels := []struct {
		name, prefix, suffix string
		send                 func(text string)
	}{
		{"say", "Ann: ", "", func(text string) { c.mode(as, ChatModeSay); c.say(as, text) }},
		{"shout", "Ann shouts: ", "", func(text string) {
			c.send(as, protocol.OpcodeClientShout, append([]byte{2}, wide(text, 0xFFFFFFFF)...))
		}},
		{"shout mode", "Ann shouts: ", "", func(text string) { c.mode(as, ChatModeShout); c.say(as, text) }},
		{"group", "Ann tells the group: ", "", func(text string) { c.mode(as, ChatModeGroup); c.say(as, text) }},
		{"guild", "Ann: ", "", func(text string) { c.mode(as, ChatModeGuild); c.say(as, text) }},
		{"tell", "Ann tells you, '", "'", func(text string) { c.mode(as, ChatModeSay); c.say(as, "/t Bob "+text) }},
		{"whisper", "Ann whispers, '", "'", func(text string) {
			c.mode(as, ChatModeWhisper)
			c.send(as, protocol.OpcodeClientSayChat, wide(text, target))
		}},
	}
	lastText := func() string {
		var got string
		for _, l := range heard(t, bs) {
			if strings.HasPrefix(l.text, "Ann") {
				got = l.text
			}
		}
		bs.Queue().GetPending()
		as.Queue().GetPending()
		return got
	}
	lastText()
	for _, ch := range channels {
		ch.send(consoleOrdinary)
		if got, want := lastText(), ch.prefix+consoleOrdinary+ch.suffix; got != want {
			t.Errorf("%s: an ordinary line came as %q, want %q", ch.name, got, want)
		}
		ch.send(consoleHostile)
		got := lastText()
		if !strings.HasPrefix(got, ch.prefix) || !strings.HasSuffix(got, ch.suffix) {
			t.Fatalf("%s: a hostile line came as %q", ch.name, got)
		}
		heldToAConsole(t, ch.name, got, len(ch.prefix)+chatLineMost+len(ch.suffix))
		ch.send("||||\x01é")
		if got := lastText(); got != "" {
			t.Errorf("%s: a line of nothing a console can type came as %q", ch.name, got)
		}
	}
}

// A GM's Snoop is a copy of one player's line to another.
func TestASnoopHoldsToAConsole(t *testing.T) {
	c := newChatWorld(t)
	as, pa := c.join(1, "Ann", 25000, 16000, 0)
	gs, pg := c.join(2, "Gil", 25010, 16000, 0)
	c.h.gm.snoops = map[entity.EntityID]map[entity.EntityID]bool{pa.ServerID: {pg.ServerID: true}}
	c.say(as, "a |+|+ b\u00e9")
	var got string
	for _, l := range heard(t, gs) {
		if strings.Contains(l.text, "Snoop:") {
			got = l.text
		}
	}
	if !strings.HasSuffix(got, "a ++ b") {
		t.Errorf("the GM saw %q", got)
	}
}

// Guild commands: the guild's chat, its name and its charter.
func TestGuildTextHoldsToAConsole(t *testing.T) {
	f := newGroupFixture(t, "Ann", "Bob")
	gh := NewGuildHandler(f.sm)
	gh.SetGameWorld(f.world)
	run := func(who string, m *protocol.Message) { gh.HandleGuildCommand(f.sess[who], m) }
	run("Ann", guildCmd(guildCmdCreate, wideb("Zi|+oné Rangers")))
	g := f.world.GuildOf(f.p["Ann"])
	if g == nil || g.Name != "Zi+on Rangers" {
		t.Fatalf("the guild: %+v", g)
	}
	id := f.seen("Bob", "Ann")
	run("Ann", guildCmd(guildCmdInvite, str8b("Bob"), wideb("Zion")))
	run("Bob", guildCmd(guildCmdJoin, u32b(id)))
	if f.world.GuildOf(f.p["Bob"]) == nil {
		t.Fatal("no join")
	}
	f.take("Bob")
	chatOf := func() string {
		r := open(t, lastOf(f.take("Bob"), protocol.OpcodeGuildInfo), protocol.OpcodeGuildInfo)
		r.u8()
		r.u32()
		return r.wide(512)
	}
	run("Ann", guildCmd(guildCmdChat, wideb(consoleOrdinary)))
	if got := chatOf(); got != "Ann: "+consoleOrdinary {
		t.Errorf("an ordinary guild line: %q", got)
	}
	run("Ann", guildCmd(guildCmdChat, wideb(consoleHostile)))
	heldToAConsole(t, "guild chat", chatOf(), len("Ann: ")+chatLineMost)

	run("Ann", guildCmd(guildCmdCharter, wideb("Be kind.\nBe |+braveé.")))
	if g.Charter != "Be kind.\nBe +brave." {
		t.Errorf("the charter: %q", g.Charter)
	}
	run("Ann", guildCmd(guildCmdCharter, wideb(consoleOrdinary)))
	if g.Charter != consoleOrdinary {
		t.Errorf("an ordinary charter: %q", g.Charter)
	}
}

// A gram: the hostile text of the tracer's crash reaches no reader's box, in
// either send's form, and an ordinary one, line ends and all, comes through.
func TestGramsHoldToAConsole(t *testing.T) {
	m, as, _, _ := mailWorld(t)
	send := func(data []byte) {
		t.Helper()
		if err := m.handleRequest(as, incoming(opcodeGramRequest, data)); err != nil {
			t.Fatal(err)
		}
	}
	box := func() *gram {
		t.Helper()
		b := m.boxes["bob"]
		if len(b) == 0 {
			t.Fatal("no gram")
		}
		g := b[len(b)-1]
		g.read = true
		return g
	}
	body := consoleOrdinary + "\nand a second line"
	send(liveSend([]string{"Bob"}, "Hail, Bob!", body))
	if g := box(); g.subject != "Hail, Bob!" || g.text != body {
		t.Errorf("an ordinary gram: %q %q", g.subject, g.text)
	}
	send(liveSend([]string{"Bob"}, consoleHostile, strings.Repeat("|+", 255)))
	g := box()
	heldToAConsole(t, "a gram's subject", g.subject, gramSubjectMost)
	heldToAConsole(t, "a gram's text", g.text, gramTextMost)
	if strings.Contains(g.text, "|") {
		t.Fatalf("the crashing gram: %q", g.text)
	}
	send(gramRequestBytes(1, true, func(w *buffer.Writer) {
		writeString8(w, "Bob", 24)
		writeWideString(w, "sé", 1024)
		writeWideString(w, strings.Repeat("|-", 200)+"end", 1024)
	}))
	if g := box(); g.text != strings.Repeat("-", 200)+"end" || g.subject != "s" {
		t.Errorf("the disc's form: %q %q", g.subject, g.text)
	}
}

// A petition's line to the GMs in the world is a line one player's text goes to
// another in. The petition itself keeps what was sent: the GMs read it in the panel.
func TestAPetitionsLineToGMsHoldsToAConsole(t *testing.T) {
	c, _, db, as, bs := bugWorld(t)
	bs.Queue().GetPending()
	chat(t, c, as, "/bug the bat |+|+ floatsé")
	got := sentText(t, bs)
	if !strings.Contains(got, "the bat ++ floats") || strings.Contains(got, "|") {
		t.Errorf("the GM was told %q", got)
	}
	p, ok, err := db.PetitionByID(1001)
	if err != nil || !ok || !strings.Contains(p.Text, "|+|+") {
		t.Errorf("the petition: %q %v %v", p.Text, ok, err)
	}
}
