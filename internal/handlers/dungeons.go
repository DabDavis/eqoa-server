package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/server/internal/worlds"
)

// dungeons.go — the instanced raid dungeons (entity/dungeons.go has the instances and who may stand where; the
// owner, 2026-11-12: "lets add a dungeon world. instanced versions of popular dungeons like Guk, Blackburrow,
// Runnyeye. they have the same mobs, but will be tuned for level 60 raids and raid gear").
//
// The dungeons' world is a server world (serverworlds.go; world.dungeons.world, slot 14, tools/worlds/dungeons.json):
// the disc's own Tunarian zones of each dungeon where they stand; its built recipe's "dungeons" block names each
// dungeon (a key, its name, the boxes its creatures stand in, its entrance). The world's npcs rows are the
// dungeons' creatures as templates (tools/dungeons_20261112.py), copied into each instance opened.
//
// The Dungeon Warden (any NPC of the name; the data tool stands one beside Dabrish and a twin at each dungeon's
// entrance, the way out) asks a box under mark 0x6B000000: each dungeon (entered in its party's instance, opened by
// a leader or a player alone), and inside the way out. /dungeon (#dungeon; a PS2's "/t dungeon ...") does it by
// words: /dungeon (the party's open instances), /dungeon enter <name>, /dungeon leave, /dungeon reset <name>.
//
// Only a client that holds the dungeons' world goes in (vanilla, once it built it and said so: 0x7E31's answer): a
// PS2 and an older vanilla are told why and not moved.

// dungeonsBoxMark marks the warden's questions' ids (boxmarks_test.go keeps every feature's mark its own).
const dungeonsBoxMark = 0x6B000000

// The warden's words (ours).
const (
	dungeonsAskOut   = "Tunaria's old dungeons, made for raids of level 60 heroes. Each party gets its own copy. Where to?"
	dungeonsAskIn    = "You stand in %s. Where would you go?"
	dungeonsAnsOpen  = "Enter %s"
	dungeonsAnsNew   = "Open %s"
	dungeonsAnsOut   = "Leave the dungeon"
	dungeonsClosed   = "The dungeons are sealed on this server."
	dungeonsNoWorld  = "Your game cannot see the dungeons, so you cannot go there. It needs a client that builds the server's worlds (eqoa-rust-client-vanilla)."
	dungeonsNotOpen  = "No instance of %s is open for your party, and only your group's leader may open one."
	dungeonsAskBoard = "Raids for level 60 heroes in Tunaria's old dungeons. Each party gets its own copy. Where to?"
	dungeonsTooFar   = "Come back to the Instanced Raids board or a Dungeon Warden to answer."
	dungeonsReach    = 30
	dungeonsHelp     = "/dungeon names your party's open dungeons; /dungeon enter <%s> goes in (a group's leader, or a player alone, opens its party's instance); /dungeon leave goes back out; /dungeon reset <name> closes the instance you opened while no one is inside. Or read the Instanced Raids board by Freeport's well."
)

type dungeonsDo int

const (
	dungeonsDoEnter dungeonsDo = iota
	dungeonsDoOut
	dungeonsDoMore
)

// dungeonsAnsMore is the answer that shows the next box of dungeons (a box holds maxDialogOptions answers).
const dungeonsAnsMore = "More dungeons"

type dungeonsAnswer struct {
	do    dungeonsDo
	key   string
	label string
	page  int
}

type dungeonsAsk struct {
	box     uint32
	answers []dungeonsAnswer
	npc     entity.EntityID
}

// Dungeons is the warden's questions.
type Dungeons struct {
	gw  *entity.World
	now func() time.Time

	mu    sync.Mutex
	seq   uint32
	asked map[uint32]dungeonsAsk
}

var dungeonsH atomic.Pointer[Dungeons]

// DefaultDungeonsWorld is the dungeons' world's slot when world.dungeons.world is unset.
const DefaultDungeonsWorld = 14

// dungeonsReaches is whether sess's client can be sent into the dungeons' world.
func dungeonsReaches(sess *session.Session, world uint8) bool {
	return sess != nil && CanLoadWorld(sess, world) && sess.HoldsServerWorld(world)
}

// dungeonJSON is one dungeon of a recipe's "dungeons" block.
type dungeonJSON struct {
	Key      string       `json:"key"`
	Name     string       `json:"name"`
	Boxes    [][4]float32 `json:"boxes"`
	Entrance arenaSpot    `json:"entrance"`
	Note     string       `json:"note,omitempty"`
}

// ReadDungeons is a built world's dungeons (its recipe's "dungeons" block), checked.
func ReadDungeons(b worlds.Built) ([]entity.DungeonDef, error) {
	if len(b.Dungeons) == 0 {
		return nil, fmt.Errorf("world %d %q has no dungeons in its recipe", b.World, b.Name)
	}
	var js []dungeonJSON
	if err := json.Unmarshal(b.Dungeons, &js); err != nil {
		return nil, fmt.Errorf("world %d's dungeons: %w", b.World, err)
	}
	var out []entity.DungeonDef
	seen := map[string]bool{}
	for _, j := range js {
		key := strings.ToLower(strings.TrimSpace(j.Key))
		if key == "" || seen[key] || j.Name == "" || len(j.Boxes) == 0 {
			return nil, fmt.Errorf("world %d's dungeon %q: a key of its own, a name and a box are needed", b.World, j.Key)
		}
		seen[key] = true
		d := entity.DungeonDef{Key: key, Name: j.Name}
		for _, bx := range j.Boxes {
			x0, z0, x1, z1 := min(bx[0], bx[2]), min(bx[1], bx[3]), max(bx[0], bx[2]), max(bx[1], bx[3])
			d.Boxes = append(d.Boxes, entity.DungeonBox{X0: x0, Z0: z0, X1: x1, Z1: z1})
		}
		s := j.Entrance.spot()
		if s.X == 0 && s.Z == 0 {
			return nil, fmt.Errorf("world %d's dungeon %q: its entrance is at 0, 0", b.World, j.Key)
		}
		d.Entrance = entity.HomeSpot{X: s.X, Y: s.Y, Z: s.Z, Facing: s.Facing}
		out = append(out, d)
	}
	return out, nil
}

// SetupDungeons turns world.dungeons on at the start: its world must be one of the server's own built worlds with
// dungeons (LoadServerWorlds first). Before the kept sessions are put back. It reports whether they are on.
func SetupDungeons(gw *entity.World, c config.DungeonsConfig, _ *database.SQLiteDB) bool {
	world := uint8(c.World)
	if world == 0 {
		world = DefaultDungeonsWorld
	}
	h := &Dungeons{gw: gw, now: time.Now, asked: map[uint32]dungeonsAsk{}}
	dungeonsH.Store(h)
	if gw == nil {
		return false
	}
	set := entity.DungeonSettings{CloseAfter: c.CloseAfter, MaxInstances: c.MaxInstances}
	if !c.Enabled {
		gw.SetDungeons(false, world, nil, set)
		return false
	}
	b, ok := ServerWorld(world)
	if !ok {
		log.Printf("WARNING: world.dungeons: world %d is none of the server's own built worlds (world.server_worlds, cmd/worldbuild); the dungeons are closed", world)
		gw.SetDungeons(false, world, nil, set)
		return false
	}
	defs, err := ReadDungeons(b)
	if err != nil {
		log.Printf("WARNING: world.dungeons: %v; the dungeons are closed", err)
		gw.SetDungeons(false, world, nil, set)
		return false
	}
	gw.SetDungeons(true, world, defs, set)
	return true
}

// dungeonsAs is a line's speaker: the warden's or the board's name (entity.DungeonRaidBoardName), as "Name: ".
func dungeonsAs(npc *entity.NPC) string {
	if npc != nil && entity.IsDungeonWarden(npc.Name) {
		return strings.TrimSpace(npc.Name) + ": "
	}
	return entity.DungeonWardenName + ": "
}

// dungeonsSay is the warden's (or the board's) words in a box.
func dungeonsSay(sess *session.Session, as, line string) {
	for _, part := range splitSay(as+line, questBoxChars) {
		SendDialogueBox(sess, 0, part)
	}
}

// dungeonsTell sends lines in the dungeons' colour.
func dungeonsTell(sess *session.Session, lines ...string) {
	for _, l := range lines {
		if l != "" {
			SendColoredChat(sess, l, 230, 190, 120)
		}
	}
}

// dungeonsTalk is a talk to npc: the warden's question when npc is a Dungeon Warden, else false.
func (h *InteractHandler) dungeonsTalk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if npc == nil || npc.Dead || !entity.IsDungeonWarden(npc.Name) {
		return false
	}
	forgetDialogs(p)
	ds := dungeonsH.Load()
	gw := h.gameWorld
	on, world := false, uint8(0)
	if gw != nil {
		on, world = gw.DungeonsOn()
	}
	switch {
	case ds == nil || !on:
		dungeonsSay(sess, dungeonsAs(npc), dungeonsClosed)
	case !dungeonsReaches(sess, world):
		dungeonsSay(sess, dungeonsAs(npc), dungeonsNoWorld)
		log.Printf("Dungeons: %s at the warden: its game cannot load world %d", p.Name, world)
	default:
		ds.askRoot(sess, p, npc, 0)
	}
	return true
}

// askRoot is the warden's box.
func (ds *Dungeons) askRoot(sess *session.Session, p *entity.Player, npc *entity.NPC, page int) {
	_, world := ds.gw.DungeonsOn()
	as := dungeonsAs(npc)
	question := dungeonsAskOut
	if entity.IsDungeonRaidBoard(npc.Name) {
		question = dungeonsAskBoard
	}
	var answers []dungeonsAnswer
	if p.World == world {
		question = fmt.Sprintf(dungeonsAskIn, "the dungeons")
		answers = append(answers, dungeonsAnswer{do: dungeonsDoOut, label: dungeonsAnsOut})
	}
	for _, c := range ds.gw.DungeonChoices(sess) {
		label := fmt.Sprintf(dungeonsAnsOpen, c.Name)
		if !c.Open {
			if !c.MayOpen {
				continue
			}
			label = fmt.Sprintf(dungeonsAnsNew, c.Name)
		}
		answers = append(answers, dungeonsAnswer{do: dungeonsDoEnter, key: c.Key, label: label})
	}
	if len(answers) == 0 {
		dungeonsSay(sess, as, "No dungeon is open for your party, and only your group's leader may open one.")
		return
	}
	ds.ask(sess, p, as+question, dungeonsPage(answers, page), npc.ServerID)
}

func (ds *Dungeons) ask(sess *session.Session, p *entity.Player, question string, answers []dungeonsAnswer, npc entity.EntityID) {
	ds.mu.Lock()
	ds.seq++
	id := uint32(dungeonsBoxMark) | (ds.seq & 0xFFFFFF)
	ds.asked[p.CharServerID] = dungeonsAsk{box: id, answers: answers, npc: npc}
	ds.mu.Unlock()
	words := make([]string, 0, len(answers))
	for _, a := range answers {
		words = append(words, clipRunes(a.label, questAnswerChars))
	}
	SendQuestPopupMulti(sess, id, clipRunes(question, questAskChars), words)
}

// answerDungeons is HandleDialogueOption's question for the warden's box. It reports whether the box was its.
func (h *InteractHandler) answerDungeons(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if box&0xFF000000 != dungeonsBoxMark {
		return false
	}
	ds := dungeonsH.Load()
	if option == 255 || ds == nil {
		return true
	}
	ds.mu.Lock()
	asked, ok := ds.asked[p.CharServerID]
	if ok && asked.box == box {
		delete(ds.asked, p.CharServerID)
	}
	ds.mu.Unlock()
	gw := ds.gw
	if !ok || asked.box != box || int(option) >= len(asked.answers) || gw == nil {
		return true
	}
	if w, x, _, z, here := gw.NPCSpot(asked.npc); !here || w != p.World || (x-p.X)*(x-p.X)+(z-p.Z)*(z-p.Z) > dungeonsReach*dungeonsReach {
		dungeonsTell(sess, dungeonsTooFar)
		return true
	}
	a := asked.answers[option]
	switch a.do {
	case dungeonsDoEnter:
		ds.enter(sess, p, a.key)
	case dungeonsDoOut:
		ds.leave(sess)
	case dungeonsDoMore:
		if npc := gw.NPCByID(asked.npc); npc != nil {
			ds.askRoot(sess, p, npc, a.page)
		}
	}
	return true
}

// dungeonsPage is the page'th box of the answers: maxDialogOptions a box, the last of a box that does not hold the rest
// "More dungeons" (to the next page, after the last back to the first).
func dungeonsPage(answers []dungeonsAnswer, page int) []dungeonsAnswer {
	if len(answers) <= maxDialogOptions {
		return answers
	}
	per := maxDialogOptions - 1
	pages := (len(answers) + per - 1) / per
	page %= pages
	out := append([]dungeonsAnswer(nil), answers[page*per:min(len(answers), (page+1)*per)]...)
	return append(out, dungeonsAnswer{do: dungeonsDoMore, page: (page + 1) % pages, label: dungeonsAnsMore})
}

// enter sends the session's player into its party's instance of the dungeon key.
func (ds *Dungeons) enter(sess *session.Session, p *entity.Player, key string) {
	on, world := ds.gw.DungeonsOn()
	switch {
	case !on:
		dungeonsTell(sess, dungeonsClosed)
		return
	case !dungeonsReaches(sess, world):
		dungeonsTell(sess, dungeonsNoWorld)
		log.Printf("Dungeons: %s's game cannot load world %d: not moved", p.Name, world)
		return
	}
	if why, ok := ds.gw.DungeonEnter(sess, key, ds.now()); !ok {
		dungeonsTell(sess, why)
	}
}

func (ds *Dungeons) leave(sess *session.Session) {
	if why, ok := ds.gw.DungeonLeave(sess); !ok {
		dungeonsTell(sess, why)
	}
}

// dungeonCommand is /dungeon [enter <name>|leave|reset <name>|help].
func dungeonCommand(gw *entity.World, sess *session.Session, rest string) {
	ds := dungeonsH.Load()
	if gw == nil || ds == nil {
		return
	}
	p := gw.SessionPlayer(sess)
	if p == nil {
		return
	}
	if on, _ := gw.DungeonsOn(); !on {
		dungeonsTell(sess, dungeonsClosed)
		return
	}
	word, arg, _ := strings.Cut(strings.TrimSpace(rest), " ")
	arg = strings.TrimSpace(arg)
	switch strings.ToLower(word) {
	case "", "list", "status":
		dungeonsTell(sess, gw.DungeonStatus(sess))
	case "enter", "go":
		if arg == "" {
			dungeonsTell(sess, dungeonsHelpLine(gw))
			return
		}
		ds.enter(sess, p, arg)
	case "leave", "out":
		ds.leave(sess)
	case "reset", "close":
		if arg == "" {
			dungeonsTell(sess, dungeonsHelpLine(gw))
			return
		}
		dungeonsTell(sess, gw.DungeonReset(sess, arg))
	default:
		dungeonsTell(sess, dungeonsHelpLine(gw))
	}
}

// dungeonsHelpLine is /dungeon's help, naming the world's dungeons by their keys (what /dungeon enter takes, as their
// names do).
func dungeonsHelpLine(gw *entity.World) string {
	var keys []string
	if gw != nil {
		for _, d := range gw.DungeonDefs() {
			keys = append(keys, d.Key)
		}
	}
	if len(keys) == 0 {
		keys = []string{"name"}
	}
	return fmt.Sprintf(dungeonsHelp, strings.Join(keys, "|"))
}

// dungeonLogin starts a character saved in the dungeons' world: in its party's open instance while there is one
// (its client must hold the world), else where it came from. It reports whether it decided.
func dungeonLogin(gw *entity.World, record *database.CharacterRecord, held func(world uint8) bool) bool {
	if gw == nil {
		return false
	}
	dungeon, resume, inst, out := gw.DungeonLogin(record.ServerID, record.World, record.X, record.Z)
	if !dungeon {
		return false
	}
	if resume && held != nil && held(uint8(record.World)) {
		// at its dungeon's entrance, never where it logged out: a place a player got stuck in (petition 1808: under
		// a rock of Blackburrow's north edge, the old landing) would hold it again, and a fight may stand there
		if e, ok := gw.DungeonEntrance(inst); ok {
			if _, err := ValidateTeleport(e.X, e.Y, e.Z, byte(record.World)); err == nil {
				log.Printf("loginSpawn: '%s' resumes in its party's dungeon instance %d, at its entrance (%.1f, %.1f, %.1f) world=%d (logged out at (%.1f, %.1f, %.1f))",
					record.CharName, inst, e.X, e.Y, e.Z, record.World, record.X, record.Y, record.Z)
				record.X, record.Y, record.Z, record.Facing = e.X, e.Y, e.Z, e.Facing
				return true
			}
		}
	}
	if out.X == 0 && out.Z == 0 {
		return false // the usual rule (beside Vaal)
	}
	if out.World >= discWorlds && (held == nil || !held(out.World)) {
		return false
	}
	if _, err := ValidateTeleport(out.X, out.Y, out.Z, out.World); err != nil {
		return false
	}
	log.Printf("loginSpawn: '%s' was saved in a dungeon with no instance open for it: starts at (%.1f, %.1f, %.1f) world=%d",
		record.CharName, out.X, out.Y, out.Z, out.World)
	record.X, record.Y, record.Z, record.World = out.X, out.Y, out.Z, int(out.World)
	return true
}
