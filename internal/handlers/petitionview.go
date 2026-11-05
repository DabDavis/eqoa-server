package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"runtime/debug"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The server's view a petition is filed with (bugreport.go fileTicket): JSON
// in petitions.server_view, for the GMs alone (the GM panel's Petitions page,
// petition_get). It is what an F3 report's trace was, taken by the server, so
// a PS2's petition has it too: the moment, the build, the client as the server
// knows it (and its own note, when it sent one), the character, its target and
// what it was shown around it (entity.PetitionView), its quests under way, and
// the lines, boxes and damage figures it was shown in the last minutes
// (session.Transcript), chat said to it among them.

type serverView struct {
	Filed  string     `json:"filed"`
	Via    string     `json:"via"`
	Server viewServer `json:"server"`
	Client viewClient `json:"client"`
	entity.PetitionView
	Quests []QuestState `json:"quests,omitempty"`
	Lines  []viewLine   `json:"lines,omitempty"`
}

type viewServer struct {
	Build   string `json:"build"`
	Up      string `json:"up"`
	Players int    `json:"players"`
}

type viewClient struct {
	Disc      string `json:"disc"`
	F3        bool   `json:"f3"`
	MailForms bool   `json:"mailForms,omitempty"`
	LiveLook  bool   `json:"liveLook,omitempty"`
	Note      string `json:"note,omitempty"`
}

// viewLine is one thing the client was shown: Ago seconds before the filing.
type viewLine struct {
	Ago  float64 `json:"ago"`
	Kind string  `json:"kind"`
	Text string  `json:"text"`
}

var processStart = time.Now()

// serverBuild is the commit this server was built from (and "-dirty" when its
// tree had changes), else "unknown".
func serverBuild() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	rev, dirty := "", false
	for _, kv := range bi.Settings {
		switch kv.Key {
		case "vcs.revision":
			rev = kv.Value
		case "vcs.modified":
			dirty = kv.Value == "true"
		}
	}
	if rev == "" {
		return "unknown"
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	if dirty {
		rev += "-dirty"
	}
	return rev
}

// discName says what a DiscVersion's byte means (session.DiscVersion).
func discName(v byte) string {
	switch v {
	case 0:
		return "none sent"
	case 0x12:
		return "0x12, a Frontiers disc (a PS2, vanilla, the web page)"
	case 0x13:
		return "0x13, a PS2 with the July S2"
	case 0x15:
		return "0x15, a PS2 with Sandstorm's live S2"
	}
	return fmt.Sprintf("0x%02X", v)
}

// serverView is the view of sess's character now, as JSON ("" when it cannot
// be made: the petition is kept without it).
func (h *SupportHandler) serverView(sess *session.Session, me *entity.Player, via string, now time.Time) string {
	v := serverView{
		Filed: now.UTC().Format(time.RFC3339),
		Via:   via,
		Server: viewServer{Build: serverBuild(), Up: now.Sub(processStart).Round(time.Second).String(),
			Players: h.gameWorld.PlayerCount()},
		Client: viewClient{Disc: discName(sess.DiscVersion()), F3: sess.BugReports(), MailForms: sess.MailLiveForms(),
			LiveLook: sess.LiveLook(), Note: sess.TakeBugNote(now)},
	}
	if pv, ok := h.gameWorld.PetitionView(sess, now); ok {
		v.PetitionView = pv
	}
	if h.quests != nil {
		for _, q := range h.quests.CharacterQuests(me.CharServerID) {
			if q.Step > 0 {
				v.Quests = append(v.Quests, q)
			}
		}
	}
	v.Lines = h.transcriptLines(sess, sess.Transcript(now), now)
	b, err := json.Marshal(v)
	if err != nil {
		log.Printf("Support: %s's server view: %v", me.Name, err)
		return ""
	}
	return string(b)
}

// transcriptLines reads what the client was shown back into lines: the text
// of each line, box and question, each damage figure with who hit whom (as the
// server names their objects now: "you" for the player), each death.
func (h *SupportHandler) transcriptLines(sess *session.Session, shown []session.Shown, now time.Time) []viewLine {
	self := sess.PlayerObjectID()
	name := func(id uint32) string {
		switch id {
		case self:
			return "you"
		case 0xFFFFFFFF:
			return "the world"
		}
		if ent, ok := h.gameWorld.FindEntityByObjectID(sess, id); ok {
			switch e := ent.(type) {
			case *entity.Player:
				return e.Name
			case *entity.NPC:
				return e.Name
			}
		}
		return fmt.Sprintf("object %d", id)
	}
	out := make([]viewLine, 0, len(shown))
	for _, e := range shown {
		if len(e.Data) < 2 {
			continue
		}
		r := buffer.NewReader(e.Data[2:])
		l := viewLine{Ago: -now.Sub(e.At).Round(100 * time.Millisecond).Seconds()}
		switch e.Opcode {
		case protocol.OpcodeClientMessage:
			l.Kind = "line"
			l.Text, _ = readWide(r)
		case protocol.OpcodeColoredChat:
			text, _ := readWide(r)
			rgb, err := r.ReadBytes(3)
			if err == nil {
				l.Kind = fmt.Sprintf("line #%02x%02x%02x", rgb[0], rgb[1], rgb[2])
			} else {
				l.Kind = "line"
			}
			l.Text = text
		case protocol.OpcodeErrorMessage:
			l.Kind = "red line"
			l.Text, _ = readWide(r)
		case protocol.OpcodeDialogueBox:
			r.ReadUint32()
			l.Kind = "box"
			l.Text, _ = readWide(r)
		case protocol.OpcodeQuestPopupMulti:
			r.ReadUint32()
			q, _ := readWide(r)
			n, _ := r.ReadByte()
			text := q
			for i := 0; i < int(n); i++ {
				opt, ok := readWide(r)
				if !ok {
					break
				}
				text += " [" + opt + "]"
			}
			l.Kind, l.Text = "question", text
		case protocol.OpcodeDamageIndication:
			src, err1 := r.ReadUint32()
			amount, err2 := r.Read7BitEncodedInt64()
			dst, err3 := r.ReadUint32()
			if err1 != nil || err2 != nil || err3 != nil {
				continue
			}
			l.Kind = "damage"
			if amount >= 0 {
				l.Text = fmt.Sprintf("%s healed %s for %d", name(src), name(dst), amount)
			} else {
				l.Text = fmt.Sprintf("%s hit %s for %d", name(src), name(dst), -amount)
			}
		case protocol.OpcodeCharacterDied:
			l.Kind, l.Text = "death", "the player died"
		default:
			continue
		}
		out = append(out, l)
	}
	return out
}
