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

// homes.go — the players' homes (entity/homes.go has the instances and who may stand where; the owner,
// 2026-11-11: "lets add player homes, will be in a new world and instanced per player. players can invite players to
// their home. their home is a very small zone with a yard around and a house thats it. add a bulletin board in
// freeport near dabrish that says Real Estate Listings").
//
// The homes' world is a server world (serverworlds.go; world.homes.world, slot 13, tools/worlds/homesteads.json):
// one disc zone with a house, its built recipe's "home" block the places (the door a player lands at, the yard's
// board, the yard's edge). Every character has a home there, free: its own instance of the world.
//
// The Real Estate Listings board (any NPC of the name; tools/place_homes_board.py stands one near Dabrish and
// its twin in the yard) asks a box under mark 0x68000000: go to my home, visit a friend's home (the homes the
// player was invited to, and its group's members'), and in a home the way out (back where the player came from).
// /home (and #home; a PS2's "/t home ...") does the same by words: /home goes home, /home leave goes out,
// /home invite <name>, /home uninvite <name>, /home kick <name>, /home list. The invites are kept in table
// charhomeguests, the stays in charhomestay (database/homes.go).
//
// Only a client that holds the homes' world goes in (vanilla, natively or in a page, once it built it and said
// so: 0x7E31's answer): a PS2 and an older vanilla are told why and not moved (homesReach).

// homesBoxMark marks the board's questions' ids (boxmarks_test.go keeps every feature's mark its own).
const homesBoxMark = 0x68000000

// The board's words (ours).
const (
	homesAskOut     = "Real Estate Listings: 'A cottage and a yard of your own in %s, yours for nothing. Where would you go?'"
	homesAskIn      = "Real Estate Listings: 'You stand in %s's home. Where would you go?'"
	homesAskOwn     = "Real Estate Listings: 'Welcome home. Where would you go?'"
	homesAskFriends = "Real Estate Listings: 'The homes whose doors are open to you:'"
	homesAnswerHome = "Go to my home"
	homesAnswerVis  = "Visit a friend's home"
	homesAnswerOut  = "Return to %s"
	homesAnswerBye  = "Leave"
	homesAnswerMore = "More homes"
	homesClosed     = "Real Estate Listings: 'There are no homes on this server.'"
	homesNoWorld    = "Real Estate Listings: 'Your game cannot see the Homesteads, so you cannot go there. It needs a client that builds the server's worlds (eqoa-rust-client-vanilla).'"
	homesNoFriends  = "Real Estate Listings: 'No one has opened their door to you. A friend can invite you with /home invite and your name.'"
	homesTooFar     = "Come back to the board to answer."
	homesReach      = 30
	homesHelp       = "/home goes to your home, /home leave goes back out, /home invite <name> and /home uninvite <name> open and shut your door to a friend, /home kick <name> shows a guest out, /home list names your guests. Or read the Real Estate Listings beside Dabrish."
)

// homesDo is what a board's answer does.
type homesDo int

const (
	homesDoHome homesDo = iota
	homesDoVisit
	homesDoOut
	homesDoBye
	homesDoMore
	homesDoFriend
)

// homesAnswer is one answer of a box: what it does, and the friend's home it names.
type homesAnswer struct {
	do    homesDo
	host  uint32
	page  int
	label string
}

type homesAsk struct {
	box     uint32
	answers []homesAnswer
	npc     entity.EntityID
}

// Homes is the board's questions and the homes' rows.
type Homes struct {
	gw  *entity.World
	db  *database.SQLiteDB
	now func() time.Time

	mu    sync.Mutex
	seq   uint32
	asked map[uint32]homesAsk
}

// homesH is the homes' handler (SetupHomes).
var homesH atomic.Pointer[Homes]

// homesReaches is whether sess's client can be sent into the homes' world: its game loads the slot and it said it
// holds the server's world there (as the arena's, ctfReaches).
func homesReaches(sess *session.Session, world uint8) bool {
	return sess != nil && CanLoadWorld(sess, world) && sess.HoldsServerWorld(world)
}

// IsHomesBoard is whether an NPC of the name is the board.
func IsHomesBoard(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), entity.HomesBoardName)
}

// homeJSON is a recipe's "home" block (tools/worlds/homesteads.json).
type homeJSON struct {
	Name  string    `json:"name"`
	Door  arenaSpot `json:"door"`
	Board arenaSpot `json:"board"`
	Yard  struct {
		X float32 `json:"x"`
		Z float32 `json:"z"`
		R float32 `json:"r"`
	} `json:"yard"`
}

// ReadHome is a built world's homes' places (its recipe's "home" block), checked.
func ReadHome(b worlds.Built) (entity.HomesLayout, error) {
	var lay entity.HomesLayout
	if len(b.Home) == 0 {
		return lay, fmt.Errorf("world %d %q has no home in its recipe", b.World, b.Name)
	}
	var h homeJSON
	if err := json.Unmarshal(b.Home, &h); err != nil {
		return lay, fmt.Errorf("world %d's home: %w", b.World, err)
	}
	lay.Name = h.Name
	if lay.Name == "" {
		lay.Name = b.Name
	}
	d, bd := h.Door.spot(), h.Board.spot()
	lay.Door = entity.HomeSpot{X: d.X, Y: d.Y, Z: d.Z, Facing: d.Facing}
	lay.Board = entity.HomeSpot{X: bd.X, Y: bd.Y, Z: bd.Z, Facing: bd.Facing}
	lay.YardX, lay.YardZ, lay.YardRadius = h.Yard.X, h.Yard.Z, h.Yard.R
	if d.X == 0 && d.Z == 0 {
		return lay, fmt.Errorf("world %d's home: its door is at 0, 0", b.World)
	}
	if lay.YardRadius <= 0 {
		return lay, fmt.Errorf("world %d's home: a yard of radius %.1f", b.World, lay.YardRadius)
	}
	if !lay.InYard(d.X, d.Z) || (bd.X != 0 || bd.Z != 0) && !lay.InYard(bd.X, bd.Z) {
		return lay, fmt.Errorf("world %d's home: its door or board is outside its yard", b.World)
	}
	return lay, nil
}

// SetupHomes turns world.homes on at the start: its world must be one of the server's own built worlds with a home
// (LoadServerWorlds first), and the invites and stays are read from the database. Before the kept sessions are put
// back (a login inside a home asks where it stood). It reports whether the homes are on.
func SetupHomes(gw *entity.World, c config.HomesConfig, db *database.SQLiteDB) bool {
	world := uint8(c.World)
	if world == 0 {
		world = DefaultHomesWorld
	}
	h := &Homes{gw: gw, db: db, now: time.Now, asked: map[uint32]homesAsk{}}
	homesH.Store(h)
	if gw == nil {
		return false
	}
	if !c.Enabled {
		gw.SetHomes(false, world, entity.HomesLayout{}, nil, nil, nil)
		return false
	}
	b, ok := ServerWorld(world)
	if !ok {
		log.Printf("WARNING: world.homes: world %d is none of the server's own built worlds (world.server_worlds, cmd/worldbuild); the homes are closed", world)
		gw.SetHomes(false, world, entity.HomesLayout{}, nil, nil, nil)
		return false
	}
	lay, err := ReadHome(b)
	if err != nil {
		log.Printf("WARNING: world.homes: %v; the homes are closed", err)
		gw.SetHomes(false, world, entity.HomesLayout{}, nil, nil, nil)
		return false
	}
	names := map[uint32]string{}
	guests := map[uint32][]entity.HomeGuest{}
	stays := map[uint32]entity.HomeStay{}
	if db != nil {
		rows, err := db.HomeGuestRows()
		if err != nil {
			log.Printf("WARNING: world.homes: the invites: %v", err)
		}
		for _, r := range rows {
			names[r.Owner] = r.OwnerName
			guests[r.Owner] = append(guests[r.Owner], entity.HomeGuest{ID: r.Guest, Name: r.GuestName})
		}
		srows, err := db.HomeStayRows()
		if err != nil {
			log.Printf("WARNING: world.homes: the stays: %v", err)
		}
		for _, r := range srows {
			stays[r.Char] = entity.HomeStay{Host: r.Host, FromFacing: r.FromFacing,
				From: entity.GateSpot{World: uint8(r.FromWorld), X: r.FromX, Y: r.FromY, Z: r.FromZ, Place: "where you were"}}
		}
	}
	gw.SetHomes(true, world, lay, names, guests, stays)
	log.Printf("Homes: %s in world %d: the door at (%.1f, %.1f, %.1f), a yard of %.0f round (%.1f, %.1f); %d homes with guests, %d stays",
		lay.Name, world, lay.Door.X, lay.Door.Y, lay.Door.Z, lay.YardRadius, lay.YardX, lay.YardZ, len(guests), len(stays))
	return true
}

// DefaultHomesWorld is the homes' world's slot when world.homes.world is unset.
const DefaultHomesWorld = 13

// homesTell sends lines in the homes' colour.
func homesTell(sess *session.Session, lines ...string) {
	for _, l := range lines {
		if l != "" {
			SendColoredChat(sess, l, 150, 220, 150)
		}
	}
}

// homesTalk is a talk to npc: the board's question when npc is the board, else false.
func (h *InteractHandler) homesTalk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if npc == nil || npc.Dead || !IsHomesBoard(npc.Name) {
		return false
	}
	forgetDialogs(p)
	hs := homesH.Load()
	gw := h.gameWorld
	on, world := false, uint8(0)
	if gw != nil {
		on, world = gw.HomesOn()
	}
	switch {
	case hs == nil || !on:
		teleSay(sess, homesClosed)
	case !homesReaches(sess, world):
		teleSay(sess, homesNoWorld)
		log.Printf("Homes: %s at the board: its game cannot load world %d", p.Name, world)
	default:
		hs.askRoot(sess, p, npc.ServerID)
	}
	return true
}

// askRoot is the board's first box.
func (hs *Homes) askRoot(sess *session.Session, p *entity.Player, npc entity.EntityID) {
	_, world := hs.gw.HomesOn()
	name := hs.gw.HomesName()
	question := fmt.Sprintf(homesAskOut, name)
	var answers []homesAnswer
	if p.World == world {
		host := p.InstanceKey()
		if host == p.CharServerID {
			question = homesAskOwn
		} else {
			question = fmt.Sprintf(homesAskIn, hs.gw.HomeHostName(host))
		}
		answers = append(answers, homesAnswer{do: homesDoOut, label: fmt.Sprintf(homesAnswerOut, hs.wayOutName(p))})
		if host != p.CharServerID {
			answers = append(answers, homesAnswer{do: homesDoHome, label: homesAnswerHome})
		}
	} else {
		answers = append(answers, homesAnswer{do: homesDoHome, label: homesAnswerHome})
	}
	answers = append(answers, homesAnswer{do: homesDoVisit, label: homesAnswerVis}, homesAnswer{do: homesDoBye, label: homesAnswerBye})
	hs.ask(sess, p, question, answers, npc)
}

// wayOutName is where the way out goes, as an answer names it.
func (hs *Homes) wayOutName(p *entity.Player) string {
	if s, ok := hs.gw.HomeStayOf(p.CharServerID); ok && (s.From.X != 0 || s.From.Z != 0) {
		if n := worldName(s.From.World); n != "" {
			return n
		}
	}
	return "Freeport"
}

// askFriends is the box of the friends' homes, page by page (five a box and "More homes").
func (hs *Homes) askFriends(sess *session.Session, p *entity.Player, npc entity.EntityID, page int) {
	open := hs.gw.HomesOpenTo(sess)
	if len(open) == 0 {
		teleSay(sess, homesNoFriends)
		return
	}
	const per = maxDialogOptions - 1
	if page*per >= len(open) {
		page = 0
	}
	var answers []homesAnswer
	for _, c := range open[page*per : min(len(open), (page+1)*per)] {
		answers = append(answers, homesAnswer{do: homesDoFriend, host: c.Host, label: c.Name + "'s home"})
	}
	if len(open) > (page+1)*per || page > 0 {
		answers = append(answers, homesAnswer{do: homesDoMore, page: page + 1, label: homesAnswerMore})
	}
	hs.ask(sess, p, homesAskFriends, answers, npc)
}

// ask opens a box of the answers for p.
func (hs *Homes) ask(sess *session.Session, p *entity.Player, question string, answers []homesAnswer, npc entity.EntityID) {
	hs.mu.Lock()
	hs.seq++
	id := uint32(homesBoxMark) | (hs.seq & 0xFFFFFF)
	hs.asked[p.CharServerID] = homesAsk{box: id, answers: answers, npc: npc}
	hs.mu.Unlock()
	words := make([]string, 0, len(answers))
	for _, a := range answers {
		words = append(words, clipRunes(a.label, questAnswerChars))
	}
	SendQuestPopupMulti(sess, id, clipRunes(question, questAskChars), words)
}

// answerHomes is HandleDialogueOption's question for the board's box. It reports whether the box was its.
func (h *InteractHandler) answerHomes(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if box&0xFF000000 != homesBoxMark {
		return false
	}
	hs := homesH.Load()
	if option == 255 || hs == nil {
		return true
	}
	hs.mu.Lock()
	asked, ok := hs.asked[p.CharServerID]
	if ok && asked.box == box {
		delete(hs.asked, p.CharServerID)
	}
	hs.mu.Unlock()
	gw := hs.gw
	if !ok || asked.box != box || int(option) >= len(asked.answers) || gw == nil {
		return true
	}
	if w, x, _, z, here := gw.NPCSpot(asked.npc); !here || w != p.World || (x-p.X)*(x-p.X)+(z-p.Z)*(z-p.Z) > homesReach*homesReach {
		teleSay(sess, homesTooFar)
		return true
	}
	a := asked.answers[option]
	switch a.do {
	case homesDoHome:
		hs.goHome(sess, p, p.CharServerID)
	case homesDoFriend:
		hs.goHome(sess, p, a.host)
	case homesDoVisit:
		hs.askFriends(sess, p, asked.npc, 0)
	case homesDoMore:
		hs.askFriends(sess, p, asked.npc, a.page)
	case homesDoOut:
		hs.leave(sess)
	}
	return true
}

// goHome sends the session's player into host's home, its stay kept.
func (hs *Homes) goHome(sess *session.Session, p *entity.Player, host uint32) {
	on, world := hs.gw.HomesOn()
	switch {
	case !on:
		homesTell(sess, homesClosed)
		return
	case !homesReaches(sess, world):
		homesTell(sess, homesNoWorld)
		log.Printf("Homes: %s's game cannot load world %d: not moved", p.Name, world)
		return
	}
	now := hs.now()
	stay, why, ok := hs.gw.HomeEnter(sess, host, now)
	if !ok {
		homesTell(sess, why)
		return
	}
	if hs.db != nil {
		row := database.HomeStayRow{Char: p.CharServerID, Host: stay.Host, FromWorld: int(stay.From.World), FromX: stay.From.X,
			FromY: stay.From.Y, FromZ: stay.From.Z, FromFacing: stay.FromFacing}
		if err := hs.db.SetHomeStay(row, now); err != nil {
			log.Printf("Homes: %s's stay not kept: %v", p.Name, err)
		}
	}
}

// leave sends the session's player out of the homes.
func (hs *Homes) leave(sess *session.Session) {
	if why, ok := hs.gw.HomeLeave(sess); !ok {
		homesTell(sess, why)
	}
}

// homeCommand is /home [leave|invite <name>|uninvite <name>|kick <name>|list|help].
func homeCommand(gw *entity.World, sess *session.Session, rest string) {
	hs := homesH.Load()
	if gw == nil || hs == nil {
		return
	}
	p := gw.SessionPlayer(sess)
	if p == nil {
		return
	}
	if on, _ := gw.HomesOn(); !on {
		homesTell(sess, "There are no homes on this server.")
		return
	}
	word, arg, _ := strings.Cut(strings.TrimSpace(rest), " ")
	arg = strings.TrimSpace(arg)
	switch strings.ToLower(word) {
	case "":
		hs.goHome(sess, p, p.CharServerID)
	case "leave", "out":
		hs.leave(sess)
	case "invite":
		hs.invite(sess, p, arg)
	case "uninvite":
		hs.uninvite(sess, p, arg)
	case "kick":
		if arg == "" {
			homesTell(sess, homesHelp)
			return
		}
		if name, why := gw.HomeKick(sess, arg); name != "" {
			homesTell(sess, fmt.Sprintf("You show %s out of your home.", name))
		} else {
			homesTell(sess, why)
		}
	case "list":
		gs := gw.HomeGuests(sess)
		if len(gs) == 0 {
			homesTell(sess, "No one is invited to your home. /home invite <name> opens your door to a friend.")
			return
		}
		names := make([]string, len(gs))
		for i, g := range gs {
			names[i] = g.Name
		}
		homesTell(sess, fmt.Sprintf("Invited to your home (%d): %s.", len(gs), strings.Join(names, ", ")))
	default:
		homesTell(sess, homesHelp)
	}
}

// charByName is a character's id and name as written, by a name in any case; ok false when there is none.
func (hs *Homes) charByName(name string) (uint32, string, bool) {
	if hs.db == nil || name == "" {
		return 0, "", false
	}
	var id uint32
	var n string
	err := hs.db.QueryRow(`SELECT serverid, charName FROM characters WHERE LOWER(charName) = LOWER(?)`, name).Scan(&id, &n)
	return id, n, err == nil
}

// invite opens p's home to the character named name.
func (hs *Homes) invite(sess *session.Session, p *entity.Player, name string) {
	if name == "" {
		homesTell(sess, homesHelp)
		return
	}
	id, n, ok := hs.charByName(name)
	switch {
	case !ok:
		homesTell(sess, fmt.Sprintf("There is no one named %s.", name))
		return
	case id == p.CharServerID:
		homesTell(sess, "Your door is always open to you.")
		return
	}
	if !hs.gw.HomeInvite(p.CharServerID, p.Name, entity.HomeGuest{ID: id, Name: n}) {
		homesTell(sess, fmt.Sprintf("%s is already invited to your home.", n))
		return
	}
	if err := hs.db.AddHomeGuest(database.HomeGuestRow{Owner: p.CharServerID, Guest: id, OwnerName: p.Name, GuestName: n}, hs.now()); err != nil {
		log.Printf("Homes: %s's invite of %s not kept: %v", p.Name, n, err)
	}
	homesTell(sess, fmt.Sprintf("You invite %s to your home. They can visit it from the Real Estate Listings.", n))
	if gp := hs.gw.PlayerByName(n); gp != nil {
		if gs := hs.gw.FindSessionByEntityID(gp.ServerID); gs != nil {
			homesTell(gs, fmt.Sprintf("%s invites you to their home: visit it from the Real Estate Listings beside Dabrish.", p.Name))
		}
	}
	log.Printf("Homes: %s invites %s", p.Name, n)
}

// uninvite shuts p's home to the character named name, who goes out if it stands there.
func (hs *Homes) uninvite(sess *session.Session, p *entity.Player, name string) {
	if name == "" {
		homesTell(sess, homesHelp)
		return
	}
	id, n, ok := hs.charByName(name)
	if !ok {
		homesTell(sess, fmt.Sprintf("There is no one named %s.", name))
		return
	}
	was, sent := hs.gw.HomeUninvite(p.CharServerID, id)
	if hs.db != nil {
		if _, err := hs.db.RemoveHomeGuest(p.CharServerID, id); err != nil {
			log.Printf("Homes: %s's invite of %s not taken back: %v", p.Name, n, err)
		}
	}
	switch {
	case !was && !sent:
		homesTell(sess, fmt.Sprintf("%s was not invited to your home.", n))
	case sent:
		homesTell(sess, fmt.Sprintf("You take back %s's invitation, and they step out of your home.", n))
	default:
		homesTell(sess, fmt.Sprintf("You take back %s's invitation.", n))
	}
	log.Printf("Homes: %s takes back %s's invite (sent out: %v)", p.Name, n, sent)
}

// homeLogin starts a character saved in the homes' world: in the home it stood in while it may (entity.HomeLogin;
// the client must hold the world), else where it came from. It reports whether it decided.
func homeLogin(gw *entity.World, record *database.CharacterRecord, held func(world uint8) bool) bool {
	if gw == nil {
		return false
	}
	home, resume, host, out := gw.HomeLogin(record.ServerID, record.World)
	if !home {
		return false
	}
	if resume && held != nil && held(uint8(record.World)) {
		if _, err := ValidateTeleport(record.X, record.Y, record.Z, byte(record.World)); err == nil {
			log.Printf("loginSpawn: '%s' resumes in %s's home (instance %d) at (%.1f, %.1f, %.1f) world=%d",
				record.CharName, gw.HomeHostName(host), host, record.X, record.Y, record.Z, record.World)
			return true
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
	log.Printf("loginSpawn: '%s' was saved in a home it may not stand in now (or its game does not hold the homes): starts where it came from (%.1f, %.1f, %.1f) world=%d",
		record.CharName, out.X, out.Y, out.Z, out.World)
	record.X, record.Y, record.Z, record.World = out.X, out.Y, out.Z, int(out.World)
	return true
}
