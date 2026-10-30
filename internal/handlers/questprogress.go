package handlers

import (
	"encoding/binary"
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The quest tracker for a client's HUD (protocol.OpcodeQuestProgress, the
// user: "add quest progress hud item similar to WoW ... like if quest says
// obtain 3 bat eyes, have 0/3"). Offered at world entry beside the marks (a
// u32, 1); a client that draws it answers 1 to have it and 0 to stop, and
// from then on is sent the whole tracker (OpcodeQuestProgressData) whenever
// it changes: each quest of the journal (at most eight, in its order), its
// name and what its step asks, counted:
//
//   - a show or give step: each item held of those it wants (the session's
//     own copy of the client's list, never the database) and the tunar, then
//     the NPC to bring them to;
//   - a kill step: the kills done of those needed (its creatures named, "raven
//     or magpie"); a loot step: the drops taken of those needed;
//   - a talk step: the NPC to speak with; a reach step: the journal's line.
//
// Worked out from the engine's state each second for the clients that asked,
// and at once when a quest is taken, counted, advanced, finished or given up
// (saveLocked's nudge); a tracker goes out only when it differs from the last.

// The bounds of a tracker: objectives a quest, a text's bytes, the body's.
const (
	maxProgressGoals = 6
	maxProgressText  = 64
	maxProgressBytes = 3000
)

// questProgress is one quest on the tracker.
type questProgress struct {
	name  string
	goals []progressGoal
}

// progressGoal is one objective: its words and its count (need 0: nothing
// to count, a thing to do).
type progressGoal struct {
	text       string
	have, need int
}

// buildQuestProgressOffer is OpcodeQuestProgress from the server: a u32, 1.
func buildQuestProgressOffer() []byte {
	return build(protocol.OpcodeQuestProgress, 6, func(w *buffer.Writer) {
		w.WriteUint32(1)
	})
}

// SendQuestProgressOffer offers the session's client the quest tracker.
func SendQuestProgressOffer(sess *session.Session) {
	queueReliable(sess, protocol.OpcodeQuestProgress, buildQuestProgressOffer())
}

// QuestProgressHandler takes a client's OpcodeQuestProgress: whether it
// wants the tracker.
type QuestProgressHandler struct {
	world *entity.World
}

// NewQuestProgressHandler creates the quest tracker's handler.
func NewQuestProgressHandler(w *entity.World) *QuestProgressHandler {
	return &QuestProgressHandler{world: w}
}

// RegisterHandlers registers the quest tracker's handler.
func (h *QuestProgressHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeQuestProgress, h.HandleQuestProgress)
}

// HandleQuestProgress makes the client's answer its character's: a u32,
// nonzero to be sent the tracker, 0 to stop. Nothing is answered.
func (h *QuestProgressHandler) HandleQuestProgress(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 || h.world == nil {
		log.Printf("QuestProgress: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	on := binary.LittleEndian.Uint32(data[:4]) != 0
	p := h.world.SessionPlayer(sess)
	q := Quests()
	if p == nil || q == nil {
		log.Printf("QuestProgress: %s has no player in the world, or no quests run", sess.RemoteAddr())
		return nil
	}
	q.SetProgress(sess, p, on)
	log.Printf("QuestProgress: %s %s", p.Name, map[bool]string{true: "draws it", false: "stopped"}[on])
	return nil
}

// SetProgress makes the character's client have the tracker (on) or stop
// (off); on, it goes out at once whatever was sent before.
func (q *QuestEngine) SetProgress(sess *session.Session, p *entity.Player, on bool) {
	if q == nil || p == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	c := q.namedLocked(p.CharServerID, p.Name)
	c.progressOn, c.progressSent = on, ""
	if on {
		q.sendProgressLocked(sess, p, c)
	}
}

// syncProgress sends each client that asked its tracker when it changed.
func (q *QuestEngine) syncProgress() {
	if q.sessionOf == nil || q.playerOf == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for id, c := range q.chars {
		if !c.progressOn {
			continue
		}
		sess := q.sessionOf(id)
		if sess == nil {
			continue
		}
		q.sendProgressLocked(sess, q.playerOf(sess), c)
	}
}

// sendProgressLocked sends the character's tracker when it differs from the
// last one sent. Must be called with q.mu held.
func (q *QuestEngine) sendProgressLocked(sess *session.Session, p *entity.Player, c *questChar) {
	if sess == nil || p == nil {
		return
	}
	body := encodeQuestProgress(q.progressLocked(sessionInvList(sess), p, c))
	if string(body) == c.progressSent {
		return
	}
	c.progressSent = string(body)
	queueReliable(sess, protocol.OpcodeQuestProgressData, append(binary.LittleEndian.AppendUint16(nil, uint16(protocol.OpcodeQuestProgressData)), body...))
}

// progressLocked is the character's tracker: the journal's quests, each with
// its step's objectives, the items counted in list l (the session's). Must
// be called with q.mu held.
func (q *QuestEngine) progressLocked(l *invList, p *entity.Player, c *questChar) []questProgress {
	name := func(id int) string {
		if n := q.names[id]; n != "" {
			return n
		}
		return fmt.Sprintf("item %d", id)
	}
	var out []questProgress
	for _, cq := range c.active() {
		if len(out) == questJournalLines {
			break
		}
		qu := q.cat.byID[cq.QuestID]
		if qu == nil {
			continue
		}
		qp := questProgress{name: qu.Name}
		if strings.TrimSpace(qp.name) == "" {
			qp.name = fmt.Sprintf("Quest %d", qu.ID)
		}
		st := qu.Step(cq.Step)
		var target string
		if st != nil {
			target = strings.TrimSpace(c.say(st.Target))
		}
		switch {
		case st == nil:
		case st.Kind == database.StepTalk:
			qp.goals = append(qp.goals, progressGoal{text: "Speak with " + target})
		case st.Kind == database.StepShow || st.Kind == database.StepGive:
			for _, it := range st.Items {
				qp.goals = append(qp.goals, progressGoal{text: name(it.PatternID), have: l.held(it.PatternID), need: max(it.Count, 1)})
			}
			if st.Tunar > 0 {
				qp.goals = append(qp.goals, progressGoal{text: "Tunar", have: p.Tunar, need: st.Tunar})
			}
			verb := "Give to "
			if st.Kind == database.StepShow {
				verb = "Show to "
			}
			qp.goals = append(qp.goals, progressGoal{text: verb + target})
		case st.Kind == database.StepKill:
			qp.goals = append(qp.goals, progressGoal{text: strings.TrimSpace(c.say(st.TargetWords())), have: cq.Count, need: max(st.Count, 1)})
		case st.Kind == database.StepLoot:
			if id, need, ok := st.LootItem(); ok {
				qp.goals = append(qp.goals, progressGoal{text: name(id), have: cq.Count, need: need})
			}
		default:
			// a reach step (and any other): what the journal says to do
			if line := strings.TrimSpace(c.say(st.Journal)); line != "" {
				qp.goals = append(qp.goals, progressGoal{text: line})
			}
		}
		if len(qp.goals) > maxProgressGoals {
			qp.goals = qp.goals[:maxProgressGoals]
		}
		out = append(out, qp)
	}
	return out
}

// putProgressText writes a u8 byte count and the text, clipped to
// maxProgressText.
func putProgressText(b []byte, s string) []byte {
	s = clip(s, maxProgressText)
	return append(append(b, byte(len(s))), s...)
}

// encodeQuestProgress is OpcodeQuestProgressData's body: a u8 count, then
// each quest's name and a u8 count of objectives, each its text, u16 had and
// u16 needed (a count past 65535 shown as that). Quests past
// maxProgressBytes are left out.
func encodeQuestProgress(qs []questProgress) []byte {
	out := []byte{0}
	n := 0
	u16 := func(v int) uint16 { return uint16(max(0, min(0xFFFF, v))) }
	for _, qp := range qs {
		b := putProgressText(nil, qp.name)
		b = append(b, byte(len(qp.goals)))
		for _, g := range qp.goals {
			b = putProgressText(b, g.text)
			b = binary.LittleEndian.AppendUint16(b, u16(g.have))
			b = binary.LittleEndian.AppendUint16(b, u16(g.need))
		}
		if len(out)+len(b) > maxProgressBytes || n == 255 {
			break
		}
		out = append(out, b...)
		n++
	}
	out[0] = byte(n)
	return out
}
