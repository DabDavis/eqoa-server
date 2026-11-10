package handlers

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// mobInfoSwitch turns world.mob_info on for one test, off again after it.
func mobInfoSwitch(t *testing.T) {
	t.Helper()
	SetMobInfo(true)
	t.Cleanup(func() { SetMobInfo(false) })
}

// taken is every message queued for the talk world's session, taken: each as
// its bytes from the opcode on.
func (tw *talkWorld) taken() [][]byte {
	var out [][]byte
	for _, m := range tw.s.Queue().GetPending() {
		out = append(out, m.Data())
	}
	return out
}

// mobBoxes is each message's box text (mobBox), for a failure's line.
func mobBoxes(msgs [][]byte) []string {
	var out []string
	for _, d := range msgs {
		out = append(out, mobBox(d))
	}
	return out
}

// mobBox is the text of a dialogue box (0x0046), or "" for anything else.
func mobBox(d []byte) string {
	if len(d) < 10 || protocol.GameOpcode(binary.LittleEndian.Uint16(d)) != protocol.OpcodeDialogueBox {
		return ""
	}
	return wideText(d[6:])
}

// The petition's ask (1459): talked to, a monster with nothing to say shows its
// name and level and nothing else. A PS2 (no answer to the offer) is sent one
// dialogue box of the two lines; a client that answered the offer one
// OpcodeMobInfo and no box, the id the talk named, the level and the name. The
// level is the one the con colour reads: the target's 0x0010 for the same
// monster is green to this level 30 player, as level 3 is. The control: with
// the switch off the same talk answers nothing, as before.
func TestAMonsterShowsItsNameAndLevel(t *testing.T) {
	tw := newTalkWorld(t, 25403, 15725)
	snake := tw.add(1, &entity.NPC{ServerID: 701, Name: "moss snake", Level: 3, MaxHP: 40, HP: 40,
		SpawnX: 25404, SpawnY: 54, SpawnZ: 15726})

	tw.talk(snake)
	if got := tw.taken(); len(got) != 0 {
		t.Fatalf("the control, the switch off: the talk answered %d messages", len(got))
	}

	mobInfoSwitch(t)
	tw.talk(snake)
	got := tw.taken()
	if len(got) != 1 || mobBox(got[0]) != "moss snake\nLevel 3" {
		t.Fatalf("a PS2's talk: %q, want one box \"moss snake\\nLevel 3\"", mobBoxes(got))
	}

	tw.s.SetMobInfo(true) // vanilla answered the offer
	tw.talk(snake)
	got = tw.taken()
	want := buildMobInfo(snake, "moss snake", 3)
	if len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("an answering client's talk: %x, want one %x", got, want)
	}
	if id, level, name := binary.LittleEndian.Uint32(got[0][3:]), got[0][7], string(got[0][9:]); id != snake || level != 3 || name != "moss snake" {
		t.Fatalf("read back: id %d level %d name %q", id, level, name)
	}

	// the con colour's level is the one shown: green, as 3 is to a 30
	th := NewTargetHandler(tw.mgr)
	th.SetGameWorld(tw.w)
	b := make([]byte, 8)
	binary.LittleEndian.PutUint32(b, 77)
	binary.LittleEndian.PutUint32(b[4:], snake)
	if err := th.HandleTarget(tw.s, incoming(protocol.OpcodeClassicTargetInfo, b)); err != nil {
		t.Fatal(err)
	}
	var con byte = 255
	for _, d := range tw.taken() {
		if protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeClassicTargetInfo {
			con = d[3]
		}
	}
	if con != entity.ConColor(tw.p.Level, int(got[0][7])) || con == entity.ConColor(tw.p.Level, tw.p.Level) {
		t.Fatalf("the target's con %d is not the shown level's %d", con, entity.ConColor(tw.p.Level, int(got[0][7])))
	}
}

// Whatever made a talk do something keeps doing it with the switch on: a sign's
// words, a town's person's line (neither has the NPC bit), and nothing at all
// from an NPC that has the bit (no monster: no one may attack it) or from a dead
// monster. The control: a monster beside them, with nothing to say, shows its
// name and level.
func TestMobInfoLeavesEveryOtherTalkAsItWas(t *testing.T) {
	mobInfoSwitch(t)
	tw := newTalkWorld(t, 25403, 15725)
	const human = 0x70D898C6
	tw.w.SetPlayerModels(map[uint32]bool{human: true})
	tw.w.SetNPCFactions([]database.NPCFactionRow{{Name: "vahji shandun", World: -1, Faction: "Citizens of Freeport"}})
	tw.h.SetDialogLines(func() ([]database.NPCDialogLine, error) {
		return []database.NPCDialogLine{{ID: 1, Town: "Freeport", Line: "Gold talks in %t, %p.", Race: -1, Weight: 1}}, nil
	})
	SetNPCText(map[string]string{"Sign Post": "Freeport: east"})
	defer SetNPCText(nil)

	sign := tw.add(1, &entity.NPC{ServerID: 711, Name: "Sign Post", Level: 1, SpawnX: 25404, SpawnY: 54, SpawnZ: 15726})
	vahji := tw.add(2, &entity.NPC{ServerID: 712, Name: "Vahji Shandun", Level: 21, ModelID: human, SpawnX: 25403.2, SpawnY: 54.1, SpawnZ: 15725.3})
	clerk := tw.add(3, &entity.NPC{ServerID: 713, Name: "Freeport clerk", Level: 20, NPCType: 0x80, SpawnX: 25402, SpawnY: 54, SpawnZ: 15724})
	dead := tw.add(4, &entity.NPC{ServerID: 714, Name: "decaying skeleton", Level: 4, Dead: true, SpawnX: 25405, SpawnY: 54, SpawnZ: 15725})
	bat := tw.add(5, &entity.NPC{ServerID: 715, Name: "fruit bat", Level: 2, SpawnX: 25403, SpawnY: 54, SpawnZ: 15727})

	for _, c := range []struct {
		id   uint32
		who  string
		want []string // the dialogue boxes, none for nothing
	}{
		{sign, "the sign", []string{"Freeport: east"}},
		{vahji, "the town's person", []string{"Vahji Shandun: Gold talks in Freeport, Talker."}},
		{clerk, "an NPC with the NPC bit", nil},
		{dead, "a dead monster", nil},
		{bat, "the control, a silent monster", []string{"fruit bat\nLevel 2"}},
	} {
		tw.talk(c.id)
		var boxes []string
		for _, d := range tw.taken() {
			if protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeMobInfo {
				t.Errorf("%s: an OpcodeMobInfo to a client that never answered the offer", c.who)
			}
			if s := mobBox(d); s != "" {
				boxes = append(boxes, s)
			} else {
				t.Errorf("%s: a message %x besides the boxes", c.who, d[:2])
			}
		}
		if len(boxes) != len(c.want) || (len(boxes) > 0 && boxes[0] != c.want[0]) {
			t.Errorf("%s answered %q, want %q", c.who, boxes, c.want)
		}
	}
}

// Out of the talk's reach, where a talk does nothing, a monster is looked at:
// its name and level, as CLIENT's details box shows a player at any distance.
// The controls: the switch off, nothing (as before); an NPC with the NPC bit
// out of reach, nothing.
func TestAMonsterOutOfReachShowsItsNameAndLevel(t *testing.T) {
	tw := newTalkWorld(t, 25403, 15725)
	far := tw.add(1, &entity.NPC{ServerID: 721, Name: "Deathfist pawn", Level: 7, SpawnX: 25433, SpawnY: 54, SpawnZ: 15725})
	farClerk := tw.add(2, &entity.NPC{ServerID: 722, Name: "Tower clerk", Level: 7, NPCType: 0x80, SpawnX: 25403, SpawnY: 54, SpawnZ: 15755})
	if entity.InRange(tw.p, tw.w.NPCs[721], entity.InteractRange) {
		t.Fatal("the pawn stands in the talk's reach")
	}
	tw.talk(far)
	if got := tw.taken(); len(got) != 0 {
		t.Fatalf("the control, the switch off: %d messages", len(got))
	}
	mobInfoSwitch(t)
	tw.talk(far)
	if got := tw.taken(); len(got) != 1 || mobBox(got[0]) != "Deathfist pawn\nLevel 7" {
		t.Fatalf("out of reach: %q", mobBoxes(got))
	}
	tw.talk(farClerk)
	if got := tw.taken(); len(got) != 0 {
		t.Fatalf("the control, an NPC with the NPC bit out of reach: %d messages", len(got))
	}
}

// The name as players see it: a source's trailing '$' off, only what a PS2 can
// type (a '|' would be a button's glyph in its text box), cut to 63; in the box
// a '%' doubled for CLIENT's text expander, which shows it as one (the message
// carries it as it is: the details box draws a name as it is); a level past a
// byte the byte's most on the wire, the number itself in the box.
func TestMobInfoTheNameAsShown(t *testing.T) {
	mobInfoSwitch(t)
	tw := newTalkWorld(t, 25403, 15725)
	long := "Pestiferous Spider Queen of the Deep Webs Under the Ruins of Old Highpass Hold"
	odd := tw.add(1, &entity.NPC{ServerID: 731, Name: " Maeyni Do`Traelix|+ 5% $", Level: 300, SpawnX: 25404, SpawnY: 54, SpawnZ: 15726})
	queen := tw.add(2, &entity.NPC{ServerID: 732, Name: long, Level: 55, SpawnX: 25404, SpawnY: 54, SpawnZ: 15724})
	tw.talk(odd)
	if got := tw.taken(); len(got) != 1 || mobBox(got[0]) != "Maeyni Do`Traelix+ 5%%\nLevel 300" {
		t.Fatalf("the box: %q", mobBoxes(got))
	}
	tw.s.SetMobInfo(true)
	tw.talk(odd)
	if got := tw.taken(); len(got) != 1 || got[0][7] != 255 || string(got[0][9:]) != "Maeyni Do`Traelix+ 5%" {
		t.Fatalf("the message: %x", got)
	}
	tw.talk(queen)
	got := tw.taken()
	if len(got) != 1 || int(got[0][8]) != 63 || string(got[0][9:]) != long[:63] {
		t.Fatalf("a long name: %x", got)
	}
}

// OpcodeMobInfo and its offer on the wire, as vanilla's test reads them
// (net::session's mob_info_offer_answer_and_show): the offer a u8 1; the
// message the layout, the id, the level, the name's count and bytes.
func TestMobInfoOnTheWire(t *testing.T) {
	if got, want := buildMobInfoOffer(), []byte{0x4C, 0x7E, 1}; !bytes.Equal(got, want) {
		t.Errorf("the offer %x, want %x", got, want)
	}
	want := []byte{0x4E, 0x7E, 1, 0xA1, 0x86, 0x01, 0x00, 3, 10, 'm', 'o', 's', 's', ' ', 's', 'n', 'a', 'k', 'e'}
	if got := buildMobInfo(0x000186A1, "moss snake", 3); !bytes.Equal(got, want) {
		t.Errorf("the message %x, want %x", got, want)
	}
}

// The offer goes at the world entry only where world.mob_info is on, once (the
// control: off, none); a client's answer is its session's (a u8 1 the message,
// 0 the box, a u32's by its first byte, an empty one nothing), through the
// registry too; a session that never answered takes the box.
func TestMobInfoOfferAndAnswer(t *testing.T) {
	h := &CharacterHandler{}
	in := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCharacterInWorld)
	offersOf := func(on bool) [][]byte {
		SetMobInfo(on)
		defer SetMobInfo(false)
		sess := entitySlotsSession()
		for range 2 {
			if err := h.HandleCharacterInWorld(sess, in); err != nil {
				t.Fatal(err)
			}
		}
		var out [][]byte
		for _, m := range sess.Queue().PeekPending() {
			if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeMobInfoOffer {
				out = append(out, d)
			}
		}
		return out
	}
	if got := offersOf(false); len(got) != 0 {
		t.Fatalf("the control, off: offered %x", got)
	}
	if got, want := offersOf(true), []byte{0x4C, 0x7E, 1}; len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("on: offered %x, want one %x", got, want)
	}

	sess := entitySlotsSession()
	if sess.MobInfo() {
		t.Fatal("a session that never answered takes the message")
	}
	mh := NewMobInfoHandler()
	for _, c := range []struct {
		data []byte
		want bool
	}{{[]byte{1}, true}, {nil, true}, {[]byte{0}, false}, {[]byte{1, 0, 0, 0}, true}, {[]byte{0, 0, 0, 0}, false}} {
		if err := mh.HandleMobInfoAnswer(sess, incoming(protocol.OpcodeMobInfoAnswer, c.data)); err != nil {
			t.Fatal(err)
		}
		if sess.MobInfo() != c.want {
			t.Fatalf("after %x: %v, want %v", c.data, sess.MobInfo(), c.want)
		}
	}
	r := NewRegistry()
	mh.RegisterHandlers(r)
	if err := r.Handle(sess, incoming(protocol.OpcodeMobInfoAnswer, []byte{1})); err != nil || !sess.MobInfo() {
		t.Fatalf("through the registry: %v, %v", err, sess.MobInfo())
	}
}
