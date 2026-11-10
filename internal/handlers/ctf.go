package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/server/internal/worlds"
)

// ctf.go — capture the flag's herald, its commands and its arena's places (entity/ctf.go has the rules; the
// owner, 2026-11-10: "players will join from PvP Events NPC next to Dabrish", then "for the ctf we are making it
// like warsong gulch from WoW", its rules alone). The arena is a server world (serverworlds.go): world.ctf names
// its slot, which must be in world.server_worlds, and its built recipe's "arena" block (tools/worlds/arena.json)
// is its places: each team's banner stand and camp, the power-ups, the field's edge, the bodies it shows.
//
// The herald is any NPC named "PvP Events" (entity.CTFHeraldName; tools/place_ctf_herald.py stands one beside
// Dabrish), talked to before quests as Dabrish and the Teleporter: a box of answers under mark 0x5F000000 (beside
// the Teleporter's 0x5E). /ctf (and #ctf, and "/t ctf" from a PS2, which keeps an unknown /command to itself) says
// how things stand; "/ctf join" and "/ctf leave" sign up and leave, the queue or a match. The words are ours.

// ctfBoxMark marks the herald's questions' ids.
const ctfBoxMark = 0x5F000000

// IsCTFHerald is whether an NPC of the name is the herald.
func IsCTFHerald(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), entity.CTFHeraldName)
}

// The herald's words (ours).
const (
	ctfHeraldAsk      = "PvP Events: 'Two companies fight over each other's banners in %s. Will you take up arms?'"
	ctfHeraldAskIn    = "PvP Events: 'Your name is on my list. %s'"
	ctfAnswerJoin     = "Sign me up"
	ctfAnswerLeave    = "Take my name off the list"
	ctfAnswerRules    = "How is it fought?"
	ctfAnswerStatus   = "How do things stand?"
	ctfHeraldTooFar   = "Come back to me to answer: my list is here."
	ctfHeraldClosed   = "PvP Events: 'The arena is closed on this server.'"
	ctfHeraldNoArena  = "PvP Events: 'Your game cannot see the arena, so I cannot send you there. It needs a client that builds the server's worlds (eqoa-rust-client-vanilla).'"
	ctfHeraldInMatch  = "PvP Events: 'You are already fighting in the arena.'"
	ctfHeraldReach    = 30
	ctfRulesHeading   = "Capture the banner, the rules:"
	ctfCommandHelp    = "/ctf: how the arena stands. /ctf join signs you up (or talk to the PvP Events herald beside Dabrish), /ctf leave takes you off the list or out of a match, /ctf rules says how it is fought."
)

// ctfRules are the rules as the herald and "/ctf rules" tell them, the numbers the server's (set).
func ctfRules(set entity.CTFSettings, lay entity.CTFLayout) []string {
	a, b := lay.Teams[0], lay.Teams[1]
	mins := func(d time.Duration) string {
		n := int(d.Round(time.Minute) / time.Minute)
		if n == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", n)
	}
	secs := func(d time.Duration) string { return fmt.Sprintf("%d seconds", int(d.Round(time.Second)/time.Second)) }
	lines := []string{
		ctfRulesHeading,
		fmt.Sprintf("%s and %s each guard a banner on a stand at their camp, at the two ends of %s.", upFirst(a.Name), b.Name, lay.Name),
		"Take the other company's banner from its stand and carry it to your own stand to capture it: it counts only while your own banner stands there.",
		fmt.Sprintf("A carrier who falls drops the banner where it fell. Touch your own fallen banner to send it home; a foe can take it up again. One left lying goes home by itself after %s.", secs(set.FlagReturn)),
		fmt.Sprintf("First to %d captures wins. After %s the higher score wins, and a tie is a draw.", set.ScoreLimit, mins(set.TimeLimit)),
		fmt.Sprintf("The fallen rise together at their camp every %s. A death in the arena costs nothing.", secs(set.WaveEvery)),
	}
	if len(lay.PowerUps) > 0 {
		lines = append(lines, fmt.Sprintf("Shrines in the field give speed for %s or mend your wounds, and come back a while after they are used.", secs(set.SpeedFor)))
	}
	if set.WoundAfter > 0 && set.WoundStep > 0 {
		lines = append(lines, fmt.Sprintf("A carrier who holds a banner %s takes %d%% more from every hit, and more each further minute.", mins(set.WoundAfter), set.WoundStep))
	}
	lines = append(lines, fmt.Sprintf("Teams of up to %d; a match begins when %d a side have signed up. /ctf shows the match, /ctf leave takes you out.", set.TeamSize, set.MinPerTeam))
	return lines
}

// ctfDo is what a herald's answer does.
type ctfDo int

const (
	ctfDoJoin ctfDo = iota
	ctfDoLeave
	ctfDoRules
	ctfDoStatus
)

// ctfAsk is a question the herald asked: its box, its answers and the herald's id.
type ctfAsk struct {
	box     uint32
	answers []ctfDo
	npc     entity.EntityID
}

// CTFHerald keeps the questions the herald asked, by character.
type CTFHerald struct {
	mu    sync.Mutex
	seq   uint32
	asked map[uint32]ctfAsk
}

// ctfTalk is a talk to npc: the herald's question when npc is the herald, a touch when it is a banner or a
// power-up of the arena's match (entity.CTFTouch), else false.
func (h *InteractHandler) ctfTalk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	gw := h.gameWorld
	if npc == nil || npc.Dead || gw == nil {
		return false
	}
	if gw.CTFTouch(p.ServerID, npc.ServerID, time.Now()) {
		return true
	}
	if !IsCTFHerald(npc.Name) {
		return false
	}
	forgetDialogs(p)
	on, world := gw.CTFOn()
	switch {
	case !on:
		teleSay(sess, ctfHeraldClosed)
		return true
	case gw.CTFInMatch(sess):
		teleSay(sess, ctfHeraldInMatch)
		return true
	case !CanLoadWorld(sess, world):
		teleSay(sess, ctfHeraldNoArena)
		log.Printf("CTF: %s at the herald: its game cannot load world %d", p.Name, world)
		return true
	}
	ask := fmt.Sprintf(ctfHeraldAsk, ctfArenaName(gw))
	answers := []ctfDo{ctfDoJoin, ctfDoRules, ctfDoStatus}
	if gw.CTFQueued(sess) {
		ask = fmt.Sprintf(ctfHeraldAskIn, strings.Join(gw.CTFStatus(sess, time.Now()), " "))
		answers = []ctfDo{ctfDoLeave, ctfDoRules, ctfDoStatus}
	}
	h.ctf.ask(sess, p, clipRunes(ask, questAskChars), answers, npc.ServerID)
	return true
}

// ctfArenaName is the arena's name, for the herald.
func ctfArenaName(gw *entity.World) string {
	if n := gw.CTFArenaName(); n != "" {
		return n
	}
	return "the arena"
}

// ask opens a box of the answers for p.
func (c *CTFHerald) ask(sess *session.Session, p *entity.Player, question string, answers []ctfDo, npc entity.EntityID) {
	c.mu.Lock()
	c.seq++
	id := ctfBoxMark | (c.seq & 0xFFFFFF)
	if c.asked == nil {
		c.asked = map[uint32]ctfAsk{}
	}
	c.asked[p.CharServerID] = ctfAsk{box: id, answers: answers, npc: npc}
	c.mu.Unlock()
	words := make([]string, 0, len(answers))
	for _, a := range answers {
		switch a {
		case ctfDoJoin:
			words = append(words, ctfAnswerJoin)
		case ctfDoLeave:
			words = append(words, ctfAnswerLeave)
		case ctfDoRules:
			words = append(words, ctfAnswerRules)
		case ctfDoStatus:
			words = append(words, ctfAnswerStatus)
		}
	}
	SendQuestPopupMulti(sess, id, question, words)
}

// answerCTF is HandleDialogueOption's question for the herald's box. It reports whether the box was its (an old
// one, or a close, does nothing).
func (h *InteractHandler) answerCTF(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if box&0xFF000000 != ctfBoxMark {
		return false
	}
	if option == 255 {
		return true
	}
	c := &h.ctf
	c.mu.Lock()
	asked, ok := c.asked[p.CharServerID]
	if ok && asked.box == box {
		delete(c.asked, p.CharServerID)
	}
	c.mu.Unlock()
	gw := h.gameWorld
	if !ok || asked.box != box || int(option) >= len(asked.answers) || gw == nil {
		return true
	}
	if w, x, _, z, here := gw.NPCSpot(asked.npc); !here || w != p.World || (x-p.X)*(x-p.X)+(z-p.Z)*(z-p.Z) > ctfHeraldReach*ctfHeraldReach {
		teleSay(sess, ctfHeraldTooFar)
		return true
	}
	now := time.Now()
	switch asked.answers[option] {
	case ctfDoJoin:
		ctfTell(sess, gw.CTFJoin(sess, now)...)
	case ctfDoLeave:
		ctfTell(sess, gw.CTFLeave(sess, now))
	case ctfDoRules:
		ctfTell(sess, ctfRulesLines...)
	case ctfDoStatus:
		ctfTell(sess, gw.CTFStatus(sess, now)...)
	}
	return true
}

// ctfTell sends lines in the herald's colour.
func ctfTell(sess *session.Session, lines ...string) {
	for _, l := range lines {
		if l != "" {
			SendColoredChat(sess, l, 120, 200, 255)
		}
	}
}

// ctfCommand is /ctf [join|leave|rules]: the arena's state, a sign-up, a leave, the rules.
func ctfCommand(gw *entity.World, sess *session.Session, rest string) {
	if gw == nil {
		return
	}
	now := time.Now()
	switch strings.ToLower(strings.TrimSpace(rest)) {
	case "":
		ctfTell(sess, gw.CTFStatus(sess, now)...)
	case "join":
		_, world := gw.CTFOn()
		if on, _ := gw.CTFOn(); on && !CanLoadWorld(sess, world) {
			ctfTell(sess, ctfHeraldNoArena)
			return
		}
		ctfTell(sess, gw.CTFJoin(sess, now)...)
	case "leave":
		ctfTell(sess, gw.CTFLeave(sess, now))
	case "rules":
		ctfTell(sess, ctfRulesLines...)
	default:
		ctfTell(sess, ctfCommandHelp)
	}
}

// arenaSpot is a place in the recipe: x, y, z and a facing in degrees (0 faces +z, 90 +x), or "toward" a point.
type arenaSpot struct {
	X      float32    `json:"x"`
	Y      float32    `json:"y"`
	Z      float32    `json:"z"`
	Facing *float32   `json:"facing,omitempty"`
	Toward [2]float32 `json:"toward,omitempty"`
}

func (s arenaSpot) spot() entity.CTFSpot {
	out := entity.CTFSpot{X: s.X, Y: s.Y, Z: s.Z}
	switch {
	case s.Facing != nil:
		out.Facing = float32(float64(*s.Facing) * math.Pi / 180)
	case s.Toward != [2]float32{}:
		out.Facing = heading(s.X, s.Z, s.Toward[0], s.Toward[1])
	}
	return out
}

// dictID is a DictID in the recipe: "0x9a1dba92" or a number.
type dictID uint32

func (d *dictID) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		var n uint32
		if err := json.Unmarshal(b, &n); err != nil {
			return err
		}
		*d = dictID(n)
		return nil
	}
	n, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(s), "0x"), 16, 32)
	if err != nil {
		return fmt.Errorf("DictID %q: %w", s, err)
	}
	*d = dictID(n)
	return nil
}

// arenaJSON is a recipe's "arena" block (tools/worlds/arena.json).
type arenaJSON struct {
	Game      string  `json:"game"`
	Name      string  `json:"name"`
	FlagBody  dictID  `json:"flag_body"`
	FlagHand  dictID  `json:"flag_hand"`
	FlagSize  float32 `json:"flag_size"`
	SpeedBody dictID  `json:"speed_body"`
	HealBody  dictID  `json:"heal_body"`
	Teams     []struct {
		Name  string    `json:"name"`
		Short string    `json:"short"`
		Flag  string    `json:"flag"`
		Body  dictID    `json:"flag_body"`
		Size  float32   `json:"flag_size"`
		Stand arenaSpot `json:"stand"`
		Camp  arenaSpot `json:"camp"`
	} `json:"teams"`
	PowerUps []struct {
		Kind string    `json:"kind"`
		Name string    `json:"name"`
		At   arenaSpot `json:"at"`
	} `json:"powerups"`
	Field [][2]float32 `json:"field"`
}

// ReadArena is a built world's capture the flag arena (its recipe's "arena" block), checked.
func ReadArena(b worlds.Built) (entity.CTFLayout, error) {
	var lay entity.CTFLayout
	if len(b.Arena) == 0 {
		return lay, fmt.Errorf("world %d %q has no arena in its recipe", b.World, b.Name)
	}
	var a arenaJSON
	if err := json.Unmarshal(b.Arena, &a); err != nil {
		return lay, fmt.Errorf("world %d's arena: %w", b.World, err)
	}
	if a.Game != "" && a.Game != "ctf" {
		return lay, fmt.Errorf("world %d's arena is for %q, not capture the flag", b.World, a.Game)
	}
	if len(a.Teams) != 2 {
		return lay, fmt.Errorf("world %d's arena has %d teams: capture the flag has 2", b.World, len(a.Teams))
	}
	lay.Name = a.Name
	if lay.Name == "" {
		lay.Name = b.Name
	}
	lay.FlagBody, lay.FlagHand, lay.FlagSize = uint32(a.FlagBody), uint32(a.FlagHand), a.FlagSize
	lay.SpeedBody, lay.HealBody = uint32(a.SpeedBody), uint32(a.HealBody)
	for i, t := range a.Teams {
		if t.Name == "" || t.Short == "" || t.Flag == "" {
			return lay, fmt.Errorf("world %d's arena: team %d needs a name, a short name and a banner's name", b.World, i)
		}
		if t.Body == 0 && lay.FlagBody == 0 {
			return lay, fmt.Errorf("world %d's arena: team %d's banner has no body (flag_body)", b.World, i)
		}
		lay.Teams[i] = entity.CTFTeamLayout{Name: t.Name, Short: t.Short, Flag: t.Flag, Stand: t.Stand.spot(), Camp: t.Camp.spot(),
			Body: uint32(t.Body), Size: t.Size}
		if t.Stand.X == 0 && t.Stand.Z == 0 || t.Camp.X == 0 && t.Camp.Z == 0 {
			return lay, fmt.Errorf("world %d's arena: team %d's stand or camp is at 0, 0", b.World, i)
		}
	}
	for _, u := range a.PowerUps {
		if u.Kind != entity.CTFSpeed && u.Kind != entity.CTFHeal {
			return lay, fmt.Errorf("world %d's arena: a power-up of kind %q (speed or heal)", b.World, u.Kind)
		}
		lay.PowerUps = append(lay.PowerUps, entity.CTFPowerUpLayout{Kind: u.Kind, Name: u.Name, At: u.At.spot()})
	}
	if len(a.Field) > 0 && len(a.Field) < 3 {
		return lay, fmt.Errorf("world %d's arena: a field of %d points", b.World, len(a.Field))
	}
	lay.Field = a.Field
	return lay, nil
}

// CTFSettings are world.ctf's numbers, each unset one its default.
func CTFSettings(c config.CTFConfig) entity.CTFSettings {
	s := entity.DefaultCTFSettings()
	if c.World > 0 {
		s.World = uint8(c.World)
	}
	pick := func(v int, into *int) {
		if v > 0 {
			*into = v
		}
	}
	dur := func(v time.Duration, into *time.Duration) {
		if v > 0 {
			*into = v
		}
	}
	pick(c.TeamSize, &s.TeamSize)
	pick(c.MinPerTeam, &s.MinPerTeam)
	pick(c.ScoreLimit, &s.ScoreLimit)
	pick(c.WoundStep, &s.WoundStep)
	pick(c.WoundMax, &s.WoundMax)
	pick(c.MinLevel, &s.MinLevel)
	dur(c.TimeLimit, &s.TimeLimit)
	dur(c.WaveEvery, &s.WaveEvery)
	dur(c.WaveMinDead, &s.WaveMinDead)
	dur(c.Countdown, &s.Countdown)
	dur(c.Prep, &s.Prep)
	dur(c.EndHold, &s.EndHold)
	dur(c.FlagReturn, &s.FlagReturn)
	dur(c.PowerUpEvery, &s.PowerUpEvery)
	dur(c.SpeedFor, &s.SpeedFor)
	switch {
	case c.WoundAfter < 0:
		s.WoundAfter = 0
	case c.WoundAfter > 0:
		s.WoundAfter = c.WoundAfter
	}
	if c.TouchReach > 0 {
		s.TouchReach = c.TouchReach
	}
	s.MinPerTeam = min(s.MinPerTeam, s.TeamSize)
	return s
}

// SetupCTF turns world.ctf on at the start: its world must be one of the server's own built worlds with an arena
// (LoadServerWorlds first). It says what it did, and reports whether the arena is on.
func SetupCTF(gw *entity.World, c config.CTFConfig) bool {
	set := CTFSettings(c)
	if !c.Enabled {
		gw.SetCTF(false, set, entity.CTFLayout{})
		return false
	}
	b, ok := ServerWorld(set.World)
	if !ok {
		log.Printf("WARNING: world.ctf: world %d is none of the server's own built worlds (world.server_worlds, cmd/worldbuild); the arena is closed", set.World)
		gw.SetCTF(false, set, entity.CTFLayout{})
		return false
	}
	lay, err := ReadArena(b)
	if err != nil {
		log.Printf("WARNING: world.ctf: %v; the arena is closed", err)
		gw.SetCTF(false, set, entity.CTFLayout{})
		return false
	}
	gw.SetCTF(true, set, lay)
	ctfRulesLines = ctfRules(set, lay)
	log.Printf("CTF: %s in world %d: %s against %s, teams of up to %d (start at %d a side), %d captures or %v, waves every %v, %d power-ups",
		lay.Name, set.World, lay.Teams[0].Short, lay.Teams[1].Short, set.TeamSize, set.MinPerTeam, set.ScoreLimit, set.TimeLimit,
		set.WaveEvery, len(lay.PowerUps))
	return true
}

// ctfRulesLines are the rules as told, made at the start (SetupCTF).
var ctfRulesLines []string

// ctfRing is a respawn's place round its camp, by the player's id: a wave's risers spread over a ring
// (entity.GateRingSpot) rather than one on another.
func ctfRing(id entity.EntityID) int {
	return 1 + int(id%8)
}
