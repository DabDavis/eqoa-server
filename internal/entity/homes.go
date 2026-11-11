package entity

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/eqoa/server/internal/session"
)

// homes.go — player homes (the owner, 2026-11-11: "lets add player homes, will be in a new world and instanced per
// player. players can invite players to their home. their home is a very small zone with a yard around and a house
// thats it. add a bulletin board in freeport near dabrish that says Real Estate Listings").
//
// The homes are one world of the server's own (handlers/serverworlds.go; tools/worlds/homesteads.json, slot 13:
// "the Homesteads"), built once, and every character has its own copy of it: an INSTANCE, keyed by its owner's
// character id. A player in an instanced world stands in one instance (Player.Instance) and meets only what stands
// in the same: the players of that instance, their pets, mercs and familiars (an NPC with an owner is in its
// owner's instance), and the world's own NPCs, which stand in every instance (NPC.Instance 0: the yard's board).
// Everything that asks whether two things share a place asks it through samePlace/npcSharesLocked: the view
// (visibility.go), a say and a /roll (handlers' inSayRange), an area's foes and friends and a group's area
// (area.go). Outside an instanced world an instance is nothing (instanceKey 0), so nothing else changes.
//
// Who may stand in a home: its owner, a character it invited (table charhomeguests, /home invite), and, while
// grouped with the owner (online), a member of the owner's group. A player in a home it may no longer stand in (an
// invite taken back, a kick, the group left) is sent out, to where it came from (homesSystem, once a second). The
// yard's edge is the world's own (a low fence with invisible collision boxes outside it, the recipe's extras): the
// server moves nobody back from it. Where a player
// came from (its last place outside the homes' world) is kept with its stay (table charhomestay) so the way out
// goes back there after a logout and a restart too; a login inside a home resumes there while the player may
// still stand in it (HomeLogin), else it starts where it came from.

// HomesBoardName is the board that sends players home: the Real Estate Listings in Freeport beside Dabrish, and
// its twin in every yard (tools/place_homes_board.py).
const HomesBoardName = "Real Estate Listings"

// homesCheckEvery is how often the homes' world is looked over (homesSystem).
const homesCheckEvery = time.Second

// homesQuietAfterMove is how long a player the server just moved is left alone by homesSystem (its client's reports
// from before the move come after it).
const homesQuietAfterMove = 3 * time.Second

// homesOut is how far in front of a board a player sent to it stands.
const homesOut = 4

// instancedWorlds are the worlds whose every player stands in an instance of its own (SetInstancedWorld).
var instancedWorlds [256]atomic.Bool

// SetInstancedWorld marks world as instanced (the homes' world, SetHomes).
func SetInstancedWorld(world uint8, on bool) { instancedWorlds[world].Store(on) }

// Instanced reports whether world is instanced.
func Instanced(world uint8) bool { return instancedWorlds[world].Load() }

// instanceKey is the instance p stands in: its Instance in an instanced world, else 0 (one place for all).
func (p *Player) instanceKey() uint32 {
	if p == nil || !Instanced(p.World) {
		return 0
	}
	return p.Instance
}

// InstanceKey is instanceKey, for the handlers.
func (p *Player) InstanceKey() uint32 { return p.instanceKey() }

// SamePlace reports whether two players share a world and, in an instanced world, its instance.
func SamePlace(a, b *Player) bool {
	return a != nil && b != nil && a.World == b.World && a.instanceKey() == b.instanceKey()
}

// npcOwnerLocked is the player an NPC belongs to (a pet, a merc, a merc's pet, a familiar, a genie, a portal), nil
// for one of the world's own. Must be called with the lock held.
func (w *World) npcOwnerLocked(n *NPC) *Player {
	switch {
	case n == nil:
		return nil
	case n.Owner != 0:
		if p := w.Players[n.Owner]; p != nil {
			return p
		}
		if m := w.NPCs[n.Owner]; m != nil && m.merc != nil { // a merc's pet names its merc
			return m.merc.owner
		}
	case n.merc != nil:
		return n.merc.owner
	case n.mercPet != nil:
		return n.mercPet.owner
	case n.familiar != nil:
		return n.familiar.owner
	case n.GenieOf != 0:
		return w.Players[n.GenieOf]
	case n.PortalOf != 0:
		return w.Players[n.PortalOf]
	}
	return nil
}

// npcInstanceLocked is the instance an NPC stands in: its owner's, or its own Instance (0: every instance of its
// world); 0 outside an instanced world. Must be called with the lock held.
func (w *World) npcInstanceLocked(n *NPC) uint32 {
	if n == nil || !Instanced(n.World) {
		return 0
	}
	if o := w.npcOwnerLocked(n); o != nil {
		return o.instanceKey()
	}
	return n.Instance
}

// npcSharesLocked reports whether n stands where p is: its world, and in an instanced world p's instance or every
// instance. Must be called with the lock held.
func (w *World) npcSharesLocked(n *NPC, p *Player) bool {
	if n == nil || p == nil || n.World != p.World {
		return false
	}
	if !Instanced(p.World) {
		return true
	}
	i := w.npcInstanceLocked(n)
	return i == 0 || i == p.instanceKey()
}

// npcsShareLocked reports whether two NPCs stand in one place (a pet's foe, a merc's): their world, and in an
// instanced world one instance (or either in every instance). Must be called with the lock held.
func (w *World) npcsShareLocked(a, b *NPC) bool {
	if a == nil || b == nil || a.World != b.World {
		return false
	}
	if !Instanced(a.World) {
		return true
	}
	ia, ib := w.npcInstanceLocked(a), w.npcInstanceLocked(b)
	return ia == 0 || ib == 0 || ia == ib
}

// HomeSpot is a place in the homes' world with a facing.
type HomeSpot struct {
	X, Y, Z, Facing float32
}

// HomesLayout is the homes' world's places (the recipe's "home" block: handlers.ReadHome).
type HomesLayout struct {
	Name string // "the Homesteads"
	// Door is where a player sent home lands, in the yard before the house's door, facing it.
	Door HomeSpot
	// Board is where the yard's board stands (tools/place_homes_board.py places it there).
	Board HomeSpot
	// The yard's edge is the world's own: a low fence with invisible collision boxes outside it (the recipe's
	// extras), which the game's walk and the server's ground both stop at; the server moves nobody back (the
	// owner, 2026-11-11: "natural barriers not the tele back to the house").
}

// HomeStay is where a character stands in the homes: whose home (Host, a character id) and where it came from
// (its last place outside the homes' world, the way out's landing).
type HomeStay struct {
	Host       uint32
	From       GateSpot
	FromFacing float32
}

// HomeGuest is an invite: a guest of a home, by id and name.
type HomeGuest struct {
	ID   uint32
	Name string
}

// homesState is the homes: on, the world, its places, the invites by host and the stays by character.
type homesState struct {
	on      bool
	world   uint8
	lay     HomesLayout
	guests  map[uint32]map[uint32]string // host -> guest -> the guest's name
	names   map[uint32]string            // a host's name, by id (an invite's)
	stays   map[uint32]HomeStay
	checkAt time.Time
	// movedAt is when the server last moved a player in the homes (homesSystem leaves it alone a while)
	movedAt map[EntityID]time.Time
}

// SetHomes turns the homes on (or off) in world, with its places, the invites (host -> guests) and the stays
// (character -> stay) the database holds. Before the kept sessions are put back (a login inside a home asks it).
func (w *World) SetHomes(on bool, world uint8, lay HomesLayout, hostNames map[uint32]string, guests map[uint32][]HomeGuest, stays map[uint32]HomeStay) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.homes.on && w.homes.world != world {
		SetInstancedWorld(w.homes.world, false)
	}
	w.homes = homesState{on: on, world: world, lay: lay, guests: map[uint32]map[uint32]string{}, names: map[uint32]string{},
		stays: map[uint32]HomeStay{}, movedAt: map[EntityID]time.Time{}}
	for id, n := range hostNames {
		w.homes.names[id] = n
	}
	for host, gs := range guests {
		for _, g := range gs {
			w.addGuestLocked(host, g)
		}
	}
	for id, s := range stays {
		w.homes.stays[id] = s
	}
	SetInstancedWorld(world, on)
}

// HomesOn reports whether the homes are on, and their world.
func (w *World) HomesOn() (bool, uint8) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.homes.on, w.homes.world
}

// HomesName is the homes' world's name ("" off).
func (w *World) HomesName() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.homes.lay.Name
}

// HomesDoor is the homes' door spot (where a player sent home lands).
func (w *World) HomesDoor() HomeSpot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.homes.lay.Door
}

func (w *World) addGuestLocked(host uint32, g HomeGuest) {
	if w.homes.guests[host] == nil {
		w.homes.guests[host] = map[uint32]string{}
	}
	w.homes.guests[host][g.ID] = g.Name
}

// homeAllowsLocked reports whether p may stand in host's home: its own, invited, or grouped with its owner. Must
// be called with the lock held.
func (w *World) homeAllowsLocked(host uint32, p *Player) bool {
	if p == nil || host == 0 {
		return false
	}
	if host == p.CharServerID {
		return true
	}
	if _, ok := w.homes.guests[host][p.CharServerID]; ok {
		return true
	}
	if p.GroupID != 0 {
		for _, m := range w.groupMembersLocked(p) {
			if m.CharServerID == host {
				return true
			}
		}
	}
	return false
}

// playerByCharLocked is the character's player in the world, nil when it is not. Must be called with the lock
// held.
func (w *World) playerByCharLocked(char uint32) *Player {
	for _, p := range w.Players {
		if p.CharServerID == char {
			return p
		}
	}
	return nil
}

// HomeHostName is the name of the home's owner (id), "" unknown.
func (w *World) HomeHostName(host uint32) string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.homeHostNameLocked(host)
}

func (w *World) homeHostNameLocked(host uint32) string {
	if p := w.playerByCharLocked(host); p != nil {
		return p.Name
	}
	return w.homes.names[host]
}

// HomeChoice is a home a player may go to: its owner's id and name.
type HomeChoice struct {
	Host uint32
	Name string
}

// HomesOpenTo is the homes other than its own the session's player may go to: those it was invited to, then its
// group's members' (online), each once, by name.
func (w *World) HomesOpenTo(sess *session.Session) []HomeChoice {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return nil
	}
	seen := map[uint32]bool{p.CharServerID: true}
	var out []HomeChoice
	for host, gs := range w.homes.guests {
		if _, ok := gs[p.CharServerID]; ok && !seen[host] {
			seen[host] = true
			out = append(out, HomeChoice{host, w.homeHostNameLocked(host)})
		}
	}
	for _, m := range w.groupMembersLocked(p) {
		if !seen[m.CharServerID] {
			seen[m.CharServerID] = true
			out = append(out, HomeChoice{m.CharServerID, m.Name})
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// HomeGuests is the guests of the session's player's home, by name.
func (w *World) HomeGuests(sess *session.Session) []HomeGuest {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return nil
	}
	var out []HomeGuest
	for id, n := range w.homes.guests[p.CharServerID] {
		out = append(out, HomeGuest{id, n})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// HomeInvite adds guest to host's guests (the handlers keep it in table charhomeguests first); hostName names the
// host for the guest's list. It reports false when the guest was already invited.
func (w *World) HomeInvite(host uint32, hostName string, guest HomeGuest) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.homes.guests[host][guest.ID]; ok {
		return false
	}
	w.homes.names[host] = hostName
	w.addGuestLocked(host, guest)
	return true
}

// HomeUninvite takes guest off host's guests and sends it out of host's home if it stands there (a group with
// the owner lets it in no more either: the owner meant it out). It reports whether it was a guest and whether it
// was sent out.
func (w *World) HomeUninvite(host, guest uint32) (was, sent bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.homes.guests[host][guest]; ok {
		was = true
		delete(w.homes.guests[host], guest)
		if len(w.homes.guests[host]) == 0 {
			delete(w.homes.guests, host)
		}
	}
	if g := w.playerByCharLocked(guest); g != nil && w.inHomeLocked(g) && g.instanceKey() == host {
		sent = w.homeSendOutLocked(g, fmt.Sprintf(homeLineUninvited, w.homeHostNameLocked(host)))
	}
	return was, sent
}

// HomeKick sends the player named name out of the session's player's home, when it stands there. It reports the
// player kicked's name ("" none) and why not.
func (w *World) HomeKick(sess *session.Session, name string) (string, string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return "", ""
	}
	for _, g := range w.Players {
		if !strings.EqualFold(g.Name, name) {
			continue
		}
		if g == p {
			return "", "You cannot show yourself out of your own home."
		}
		if !w.inHomeLocked(g) || g.instanceKey() != p.CharServerID {
			return "", fmt.Sprintf("%s is not in your home.", g.Name)
		}
		w.homeSendOutLocked(g, fmt.Sprintf(homeLineKicked, p.Name))
		return g.Name, ""
	}
	return "", fmt.Sprintf("No one named %s is in the world.", name)
}

// The homes' words (ours).
const (
	homeLineUninvited = "You are no longer welcome in %s's home, and you step out."
	homeLineKicked    = "%s shows you out of the home."
	homeLineGroupGone = "You are no longer in %s's group, and you step out of the home."
	homeLineWelcome   = "Welcome to %s's home."
	homeLineOwn       = "Welcome home."
)

// inHomeLocked reports whether p stands in the homes' world. Must be called with the lock held.
func (w *World) inHomeLocked(p *Player) bool {
	return w.homes.on && p != nil && p.World == w.homes.world
}

// HomeEnter sends the session's player into host's home (its own: host its own id): its instance set, its stay
// noted (where it came from: where it stands now outside the homes, or the stay it had inside), and a travel to
// the door. The caller has checked that its client holds the homes' world. It reports the stay to keep (table
// charhomestay) and, false, why not.
func (w *World) HomeEnter(sess *session.Session, host uint32, now time.Time) (HomeStay, string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	switch {
	case !w.homes.on:
		return HomeStay{}, "The homes are closed on this server.", false
	case p == nil || p.IsDead() || p.respawnPending():
		return HomeStay{}, "You cannot go there now.", false
	case !w.homeAllowsLocked(host, p):
		return HomeStay{}, fmt.Sprintf("%s has not invited you in.", w.homeHostNameLocked(host)), false
	}
	stay := HomeStay{Host: host}
	if w.inHomeLocked(p) {
		old := w.homes.stays[p.CharServerID]
		stay.From, stay.FromFacing = old.From, old.FromFacing
	} else {
		stay.From = GateSpot{World: p.World, X: p.X, Y: p.Y, Z: p.Z, Place: "where you were"}
		stay.FromFacing = p.Facing
	}
	w.homes.stays[p.CharServerID] = stay
	p.Instance = host
	d := w.homes.lay.Door
	w.homes.movedAt[p.ServerID] = now
	w.gates.due = append(w.gates.due, GateTravel{Sess: sess, P: p, To: GateSpot{World: w.homes.world, X: d.X, Y: d.Y, Z: d.Z,
		Place: w.homes.lay.Name}, Spell: "Real Estate Listings", Facing: d.Facing, HasFacing: true})
	line := homeLineOwn
	if host != p.CharServerID {
		line = fmt.Sprintf(homeLineWelcome, w.homeHostNameLocked(host))
	}
	w.sendColoredLocked(sess, line, homesColour)
	log.Printf("Homes: %s goes to %s's home (instance %d), from (%.1f, %.1f, %.1f) world=%d", p.Name,
		w.homeHostNameLocked(host), host, stay.From.X, stay.From.Y, stay.From.Z, stay.From.World)
	return stay, "", true
}

// homesColour is the homes' lines' colour.
var homesColour = [3]byte{150, 220, 150}

// HomeLeave sends the session's player out of the homes, to where it came from. It reports false (and why) when
// it is not in a home.
func (w *World) HomeLeave(sess *session.Session) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || !w.inHomeLocked(p) {
		return "You are not in a home.", false
	}
	if !w.homeSendOutLocked(p, "") {
		return "You cannot leave now.", false
	}
	return "", true
}

// homeSendOutLocked sends p out of the homes to where it came from (its stay's From; else in front of Freeport's
// board), told line when not "". Must be called with the lock held.
func (w *World) homeSendOutLocked(p *Player, line string) bool {
	sess := w.findSessionByEntityIDLocked(p.ServerID)
	if sess == nil {
		return false
	}
	to, facing, ok := w.homeWayOutLocked(p.CharServerID)
	if !ok {
		log.Printf("Homes: %s cannot be sent out: no place to go (no board outside the homes)", p.Name)
		return false
	}
	w.homes.movedAt[p.ServerID] = time.Now()
	w.gates.due = append(w.gates.due, GateTravel{Sess: sess, P: p, To: to, Spell: "the way out of the home",
		Facing: facing, HasFacing: true})
	if line != "" {
		w.sendColoredLocked(sess, line, homesColour)
	}
	log.Printf("Homes: %s goes out of the home to %s (%.1f, %.1f, %.1f) world=%d", p.Name, to.Place, to.X, to.Y, to.Z, to.World)
	return true
}

// homeWayOutLocked is where a character leaving the homes goes: where it came from (its stay), else in front of
// the board outside the homes. Must be called with the lock held.
func (w *World) homeWayOutLocked(char uint32) (GateSpot, float32, bool) {
	if s, ok := w.homes.stays[char]; ok && (s.From.X != 0 || s.From.Z != 0) && s.From.World != w.homes.world {
		return s.From, s.FromFacing, true
	}
	return w.homesBoardSpotLocked()
}

// homesBoardSpotLocked is the spot in front of the board outside the homes, facing away from it. Must be called
// with the lock held.
func (w *World) homesBoardSpotLocked() (GateSpot, float32, bool) {
	for _, npc := range w.NPCs {
		if npc.Temporary || npc.Dead || !strings.EqualFold(npc.Name, HomesBoardName) || npc.World == w.homes.world {
			continue
		}
		f := float64(npc.Facing)
		return GateSpot{World: npc.World, Y: npc.Y, Place: "in front of the " + HomesBoardName,
			X: npc.X + homesOut*float32(math.Sin(f)), Z: npc.Z + homesOut*float32(math.Cos(f))}, npc.Facing, true
	}
	return GateSpot{}, 0, false
}

// HomeLogin decides a login of the character saved in the homes' world: it resumes in the home it stood in (its
// stay's host) while it may still stand there (its own, or invited: a group is not kept over a logout), else it
// starts where it came from (out: the way out, whichever it does). home false: the character was not saved in the homes (nothing to decide).
func (w *World) HomeLogin(char uint32, world int) (home, resume bool, host uint32, out GateSpot) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.homes.on || world != int(w.homes.world) {
		return false, false, 0, GateSpot{}
	}
	s, ok := w.homes.stays[char]
	if !ok || s.Host == 0 {
		s = HomeStay{Host: char}
	}
	out, _, _ = w.homeWayOutLocked(char)
	_, invited := w.homes.guests[s.Host][char]
	if s.Host == char || invited {
		return true, true, s.Host, out
	}
	// not let in there now: if it starts in the homes after all (nowhere else to go), it is in its own
	s.Host = char
	w.homes.stays[char] = s
	return true, false, char, out
}

// homeInstanceOnLoadLocked is the instance a character created standing in the homes' world is in: its stay's
// host, else its own. Must be called with the lock held.
func (w *World) homeInstanceOnLoadLocked(char uint32, world uint8) uint32 {
	if !w.homes.on || world != w.homes.world {
		return 0
	}
	if s, ok := w.homes.stays[char]; ok && s.Host != 0 {
		return s.Host
	}
	return char
}

// homesSystem looks over the homes' world once a second: a player in a home it may not stand in goes out; one
// with no instance (a GM's move) is put in its own. Must be called
// with the lock held.
func (w *World) homesSystem(now time.Time) {
	if !w.homes.on || now.Before(w.homes.checkAt) {
		return
	}
	w.homes.checkAt = now.Add(homesCheckEvery)
	for id, at := range w.homes.movedAt {
		if now.Sub(at) > time.Minute {
			delete(w.homes.movedAt, id)
		}
	}
	for _, p := range w.Players {
		if p.World != w.homes.world || p.IsDead() || p.respawnPending() {
			continue
		}
		if now.Sub(w.homes.movedAt[p.ServerID]) < homesQuietAfterMove {
			continue
		}
		if p.Instance == 0 {
			p.Instance = p.CharServerID
			log.Printf("Homes: %s stood in the homes in no home: in its own", p.Name)
		}
		if host := p.Instance; !w.homeAllowsLocked(host, p) {
			line := fmt.Sprintf(homeLineUninvited, w.homeHostNameLocked(host))
			if _, invited := w.homes.guests[host][p.CharServerID]; !invited && w.playerByCharLocked(host) != nil {
				line = fmt.Sprintf(homeLineGroupGone, w.homeHostNameLocked(host))
			}
			w.homes.movedAt[p.ServerID] = now
			w.homeSendOutLocked(p, line)
			continue
		}
	}
}

// HomeStayOf is the character's stay (ok false: none).
func (w *World) HomeStayOf(char uint32) (HomeStay, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	s, ok := w.homes.stays[char]
	return s, ok
}

// instanceOfLocked is the instance the entity id stands in (a player's, an NPC's), 0 for none or every. Must be
// called with the lock held.
func (w *World) instanceOfLocked(id EntityID) uint32 {
	if p := w.Players[id]; p != nil {
		return p.instanceKey()
	}
	return w.npcInstanceLocked(w.NPCs[id])
}
