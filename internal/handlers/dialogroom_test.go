package handlers

import (
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The dialog room (dialogroom.go): the offer, the answer and its clamp, and the
// boxes of the coachmen, the Testing Heroes Vendor and the Equipment Dyer at
// CLIENT's six (as they were before it) and at twelve (vanilla's).

// The offer: OpcodeDialogRoom and the server's most, a u32, nothing else.
func TestDialogRoomOffer(t *testing.T) {
	got := buildDialogRoomOffer(session.MaxDialogRoom)
	want := binary.LittleEndian.AppendUint16(nil, uint16(protocol.OpcodeDialogRoom))
	want = binary.LittleEndian.AppendUint32(want, 12)
	if string(got) != string(want) || protocol.OpcodeDialogRoom != 0x7E71 {
		t.Fatalf("offer % X, want % X", got, want)
	}
	if protocol.OpcodeDialogRoom.String() != "DialogRoom" {
		t.Errorf("named %q", protocol.OpcodeDialogRoom.String())
	}
}

// The answer: a session never answered (a PS2) keeps six; an answer is held
// within six and the server's most; a short one changes nothing.
func TestDialogRoomAnswerClamps(t *testing.T) {
	c := newChatWorld(t)
	s, _ := c.join(1, "Anna", 5000, 5000, 0)
	if dialogRoom(s) != 6 || dialogRoom(nil) != 6 {
		t.Fatalf("never answered: %d", dialogRoom(s))
	}
	h := NewDialogRoomHandler()
	answer := func(b []byte) int {
		t.Helper()
		if err := h.HandleDialogRoom(s, incoming(protocol.OpcodeDialogRoom, b)); err != nil {
			t.Fatal(err)
		}
		return dialogRoom(s)
	}
	for _, c := range []struct{ asked, want int }{{12, 12}, {9, 9}, {40, 12}, {3, 6}, {0, 6}, {0xFFFFFFFF, 12}, {6, 6}} {
		if got := answer(u32b(uint32(c.asked))); got != c.want {
			t.Errorf("asked %d: room %d, want %d", c.asked, got, c.want)
		}
	}
	answer(u32b(12))
	if got := answer([]byte{9, 0}); got != 12 {
		t.Errorf("a short answer changed the room to %d", got)
	}
	// kept over a restart's keep
	k := s.Keep()
	if k.DialogRoom != 12 {
		t.Errorf("kept %d", k.DialogRoom)
	}
	if session.ClampDialogRoom(20, 9) != 9 || session.ClampDialogRoom(20, 40) != 12 || session.ClampDialogRoom(1, 12) != 6 {
		t.Error("ClampDialogRoom")
	}
}

// A box is cut to its client's room: six for a PS2, twelve for a client of
// twelve.
func TestQuestPopupCutToTheRoom(t *testing.T) {
	c := newChatWorld(t)
	s, _ := c.join(1, "Anna", 5000, 5000, 0)
	s.Queue().GetPending()
	opts := make([]string, 20)
	for i := range opts {
		opts[i] = fmt.Sprint(i)
	}
	count := func() int {
		ms := s.Queue().GetPending()
		d := ms[len(ms)-1].Data()
		// opcode, id, the question's u32 count and one character
		return int(d[2+4+4+2])
	}
	SendQuestPopupMulti(s, 1, "q", opts)
	if n := count(); n != 6 {
		t.Errorf("a PS2's box: %d answers", n)
	}
	s.SetDialogRoom(12)
	SendQuestPopupMulti(s, 1, "q", opts)
	if n := count(); n != 12 {
		t.Errorf("a client of twelve: %d answers", n)
	}
}

// oldCoachPage is the coach's page as it was before the room (five a page,
// More and Back after): the boxes a PS2 must still be sent.
func oldCoachPage(n, page int) (from, to int, more, back bool) {
	from, to = page*5, min(page*5+5, n)
	return from, to, to < n, page > 0
}

// navPage: no box over its room, every item on one page, in order, More and
// Back where they lead; at six the pages of every list of up to ten are the
// coach's as they were (a longer one's middle pages had held five, More and
// Back: seven answers, past CLIENT's six).
func TestNavPage(t *testing.T) {
	for _, room := range []int{6, 9, 12} {
		for n := 0; n <= 40; n++ {
			next := 0
			for page := 0; ; page++ {
				from, to, more, back := navPage(n, page, room)
				box := to - from
				if more {
					box++
				}
				if back {
					box++
				}
				if box > room || from != next || (page > 0) != back {
					t.Fatalf("room %d, %d items, page %d: [%d,%d) more %v back %v", room, n, page, from, to, more, back)
				}
				if room == 6 && n <= 10 {
					if of, ot, om, ob := oldCoachPage(n, page); of != from || ot != to || om != more || ob != back {
						t.Fatalf("%d items, page %d: [%d,%d) %v %v, was [%d,%d) %v %v", n, page, from, to, more, back, of, ot, om, ob)
					}
				}
				next = to
				if !more {
					break
				}
			}
			if next != n {
				t.Fatalf("room %d, %d items: the pages ended at %d", room, n, next)
			}
		}
	}
}

// coachDests are n stops, "Stop 0"...
func coachDests(n int) []entity.CoachDest {
	out := make([]entity.CoachDest, n)
	for i := range out {
		out[i] = entity.CoachDest{Name: fmt.Sprintf("Stop %d", i), X: 5000 + float32(i), Y: 50, Z: 5000, World: 0}
	}
	return out
}

func stops(from, to int) []string {
	var out []string
	for i := from; i < to; i++ {
		out = append(out, fmt.Sprintf("Stop %d", i))
	}
	return out
}

// A coachman's stops, through the 0x0035s a client sends: at six the boxes a
// PS2 was always sent (five and More, the rest and Back); at twelve eleven
// and More, then the rest and Back; More, Back and a stop's pick each mapped
// to the page the box showed.
func TestCoachPagesByTheRoom(t *testing.T) {
	const greet = "Greetings, traveler! Where would you like to go?"
	for _, c := range []struct {
		room, n int
		boxes   [][]string // each page's answers, More... and Back
	}{
		{6, 5, [][]string{stops(0, 5)}},
		{6, 8, [][]string{append(stops(0, 5), "More..."), append(stops(5, 8), "Back")}},
		{6, 10, [][]string{append(stops(0, 5), "More..."), append(stops(5, 10), "Back")}},
		{6, 13, [][]string{append(stops(0, 5), "More..."), append(stops(5, 9), "More...", "Back"), append(stops(9, 13), "Back")}},
		{12, 8, [][]string{stops(0, 8)}},
		{12, 11, [][]string{stops(0, 11)}},
		{12, 15, [][]string{append(stops(0, 11), "More..."), append(stops(11, 15), "Back")}},
		{12, 30, [][]string{append(stops(0, 11), "More..."), append(stops(11, 21), "More...", "Back"), append(stops(21, 30), "Back")}},
	} {
		t.Run(fmt.Sprintf("room%d_%dstops", c.room, c.n), func(t *testing.T) {
			r := newPortalRig(t)
			if c.room != 6 {
				r.anna.SetDialogRoom(c.room)
			}
			r.pa.PendingCoachDests, r.pa.CoachPage = coachDests(c.n), 0
			r.ih.sendCoachPage(r.anna, r.pa)
			for page, want := range c.boxes {
				r.want("Anna", r.msgs(r.anna), "ask#0:"+greet+"|"+strings.Join(want, "|"))
				if page == len(c.boxes)-1 {
					break
				}
				r.pick(r.anna, 0, byte(len(want)-1-boolInt(page > 0))) // More...
			}
			// Back from the last page, then a pick on the one before it
			if len(c.boxes) > 1 {
				last := c.boxes[len(c.boxes)-1]
				r.pick(r.anna, 0, byte(len(last)-1)) // Back
				before := c.boxes[len(c.boxes)-2]
				r.want("Anna", r.msgs(r.anna), "ask#0:"+greet+"|"+strings.Join(before, "|"))
				r.pick(r.anna, 0, 1)
				if got := r.msgs(r.anna); len(r.pa.PendingCoachDests) != 0 || !hasMsg(got, "teleport") {
					t.Fatalf("a pick on the page before the last: %q, %d left", got, len(r.pa.PendingCoachDests))
				}
				if want := before[1]; r.pa.X != coachDests(c.n)[stopIndex(want)].X {
					t.Errorf("picked %s, landed at x %v", want, r.pa.X)
				}
			}
		})
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func stopIndex(label string) int {
	var n int
	fmt.Sscanf(label, "Stop %d", &n)
	return n
}

// The Testing Heroes Vendor's list of thirteen wares: at six five a page and
// More... (as ever); at twelve eleven and More..., then the last two and
// More..., each pick the ware its box showed.
func TestHeroesPagesByTheRoom(t *testing.T) {
	answers := func(got []string) []string {
		for _, m := range got {
			if rest, ok := strings.CutPrefix(m, "ask#"); ok {
				_, q, _ := strings.Cut(rest, ":")
				return strings.Split(q, "|")[1:]
			}
		}
		return nil
	}
	for _, room := range []int{6, 12} {
		t.Run(fmt.Sprint("room", room), func(t *testing.T) {
			r := newHeroesRig(t)
			for i := 0; i < 8; i++ {
				res, err := r.db.Exec(`INSERT INTO itempattern
					(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
					 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
					 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
					VALUES (0, 0, 209763529, 0, -1, 0, 0, 1, 0, 0, 0, 0, 1, 1, 0, 0, 0, 0, 2, 0, 0, 0, ?, 'Made up.', 0, 0)`,
					fmt.Sprintf("Ware %d", i))
				if err != nil {
					t.Fatal(err)
				}
				id, _ := res.LastInsertId()
				if _, err := r.db.Exec(`INSERT INTO heroes_stock (patternid, cost, says, source) VALUES (?, ?, '', 'testing-heroes')`, id, 2000+i); err != nil {
					t.Fatal(err)
				}
			}
			if room != 6 {
				r.anna.SetDialogRoom(room)
			}
			got, box := r.visit()
			first := answers(got)
			per := room - 1
			if len(first) != room || first[per] != heroesMore {
				t.Fatalf("the first box: %d answers %q", len(first), first)
			}
			if room == 6 && strings.Join(first[:5], "|") != heroesList {
				t.Errorf("a PS2's first box %q, want the five as ever", first)
			}
			got, next := r.choose(box, byte(per)) // More...
			second := answers(got)
			n := min(13-per, per)
			if len(second) != n+1 || second[n] != heroesMore {
				t.Fatalf("the second box: %q", second)
			}
			r.choose(next, 0) // the second box's first ware
			name, _, _ := strings.Cut(second[0], ":")
			if len(r.shown) != 1 || !strings.Contains(r.shown[0], name) {
				t.Errorf("picked %q, shown %q", second[0], r.shown)
			}
		})
	}
}

// The Equipment Dyer's first question: at six as ever; at twelve the armour's
// pieces and the robe and shield in it, each answer dyeing what it names, a
// category that does not fit (the trim, the weapons) still its own question.
func TestDyerFirstQuestionByTheRoom(t *testing.T) {
	six := choiceLabels(dyerMenuIn(6))
	if strings.Join(six, "|") != strings.Join(choiceLabels(dyerMenu()), "|") || len(six) != 6 {
		t.Fatalf("six: %q", six)
	}
	twelve := dyerMenuIn(12)
	want := "All armor|Chest|Bracers|Gloves|Legs|Boots|Helm|Armor trim|Robe|Shield|Weapons|Remove all dyes"
	if got := strings.Join(choiceLabels(twelve), "|"); got != want {
		t.Fatalf("twelve: %q\nwant      %q", got, want)
	}
	if twelve[1].slots[0] != dyerArmorPiece.sub[0].slots[0] || twelve[7].sub == nil || twelve[10].sub == nil || twelve[11].slots != nil {
		t.Errorf("twelve's answers do not do what they name: %+v", twelve)
	}
	for _, room := range []int{0, 6, 7, 9, 10, 11, 12, 20} {
		if n := len(dyerMenuIn(room)); n > max(room, 6) {
			t.Errorf("room %d: %d answers", room, n)
		}
	}
}

// The dyer through its boxes: at six the first question is the six and its
// second answer, "One armor piece", asks which; at twelve the second answer,
// the chest, goes straight to the colours, and the eighth, the trim, still
// asks which, its pick mapped in the twelve's menu; a dye lands.
func TestDyerAnswersByTheRoom(t *testing.T) {
	for _, room := range []int{6, 12} {
		t.Run(fmt.Sprint("room", room), func(t *testing.T) {
			tw := newTalkWorld(t, 100, 100)
			db, err := database.NewSQLite(t.TempDir() + "/dyer.db")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { db.Close() })
			rec := &database.CharacterRecord{CharName: "Talker", AccountID: 1, Level: 10}
			if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
				t.Fatal(err)
			}
			tw.p.CharServerID = rec.ServerID
			if room != 6 {
				tw.s.SetDialogRoom(room)
			}
			d := NewDyer(tw.w, db)
			dyer := &entity.NPC{ServerID: 3101, Name: EquipmentDyerName, NPCType: 0x80}
			questMsgs(tw.s)
			pick := func(msgs []string, n byte) []string {
				t.Helper()
				if id := boxID(msgs); id == 0 || !d.Answer(tw.s, tw.p, id, n) {
					t.Fatalf("no dyer's question in %q", msgs)
				}
				return questMsgs(tw.s)
			}
			last := func(msgs []string) string { return msgs[len(msgs)-1] }
			d.Talk(tw.s, tw.p, dyer)
			first := questMsgs(tw.s)
			if room == 6 {
				if !strings.HasSuffix(last(first), ":What shall I dye?|All armor|One armor piece|Armor trim|Robe and shield|Weapons|Remove all dyes") {
					t.Fatalf("six: %q", first)
				}
				if got := pick(first, 1); !strings.HasSuffix(last(got), ":Which one?|Chest|Bracers|Gloves|Legs|Boots|Helm") {
					t.Fatalf("six, One armor piece: %q", got)
				}
				return
			}
			if !strings.HasSuffix(last(first), ":What shall I dye?|All armor|Chest|Bracers|Gloves|Legs|Boots|Helm|Armor trim|Robe|Shield|Weapons|Remove all dyes") {
				t.Fatalf("twelve: %q", first)
			}
			got := pick(first, 1) // Chest
			if !strings.Contains(last(got), ":Which colors for chest?|Whites, greys and blacks|") {
				t.Fatalf("twelve, Chest: %q", got)
			}
			got = pick(pick(got, 1), 0) // Reds and pinks, Red
			if c, _ := db.CharDyes(rec.ServerID); c[database.DyeChest] != 0xD10000FF {
				t.Errorf("chest red: %08X", c[database.DyeChest])
			}
			d.Talk(tw.s, tw.p, dyer)
			got = pick(questMsgs(tw.s), 7) // Armor trim
			if !strings.HasSuffix(last(got), ":Which one?|All armor trim|Chest or robe top trim|Bracers and gloves trim|Legs or robe skirt trim|Boots trim|Helm trim") {
				t.Fatalf("twelve, Armor trim: %q", got)
			}
			if got = pick(got, 1); !strings.Contains(last(got), ":Which colors for chest or robe top trim?|") {
				t.Fatalf("twelve, the trim's chest: %q", got)
			}
			d.Talk(tw.s, tw.p, dyer)
			if got = pick(questMsgs(tw.s), 10); !strings.HasSuffix(last(got), ":Which one?|Both weapons|Primary weapon|Secondary weapon") {
				t.Fatalf("twelve, Weapons: %q", got)
			}
		})
	}
}
