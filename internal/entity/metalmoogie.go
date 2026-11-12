package entity

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// metalmoogie.go — Metal Moogie, Mr. Moogie's challenge (the GM Jibrish's petition 1379, by Jibcro:
// "Add another dialogue option to mr. moogie ... "I'd like to prove my worth" / If someone clicks
// this it summons a metallic version of mr. moogie called Metal Moogie ... Moogie will only spawn one
// of these at a time. It locks the player into a duel with metal moogie. No other players can hit it
// except the player who challenged, like a duel. If they defeat it they can loot a single "Moogie
// Tribute" which can be turned into to Mr. Moogie to receive a buff called Duelists Favor"; and 1380:
// "If Metal Moogie defeats the player in a duel, he should despawn and mr. moogie shout call the
// player pathetic in /say"). The talk and the tribute's turn-in are handlers' moogie.go; his figures
// (50k hit points, quads for about 300 on a 2,000 AC 400 AGI warrior, a 750 kick every 6 s, a 750
// unresistable lifetap every 30 s, a 25% 500 lightning proc) are his class template's, data.
//
// Metal Moogie is a copy of the spawn-only NPC of his name (spawnonly.go: the original stands in no
// one's world), as a GM trigger's spawn is: a Temporary NPC, his template's with it. One stands at a
// time, on the whole server. He rises at his original's spot when that is within MetalRiseNear of the
// Mr. Moogie asked (the data puts it beside him), else MetalRiseOut from Mr. Moogie toward the
// challenger, on the ground's floor there, facing the challenger.
//
// The fight starts after a countdown (the GM Jibrish's petition 1623: "when moogie spawns, he should be
// immune for 5 seconds. A count down should start in the chat saying "Fight starts in 5" "4" etc. On
// "FIGHT" metal moogie should attack the challenger with the same duel mechanics"): for MetalCountdown
// seconds from his rising he is held (npcChallenge.held): no one, the challenger and its pet included,
// may hurt him (a swing is not engaged, a harmful spell refused, an area passes him by, a pet will not
// go for him; the challenger is told MetalLineWait, anyone else his duel is another's), he holds no hate,
// attacks no one and notices no one. The challenger alone is told the count in chat, one line a second
// on the world's tick (metalMoogieSystem): MetalLineCount, then MetalLineCountN down to 1, then
// MetalLineFight, at which he goes for the challenger and every rule below is the duel's as before. The
// countdown is the duel's: a challenger who leaves, flees or is brought to nothing during it ends it as
// below (a restart ends it as it ends any duel: he is a Temporary NPC, not kept).
//
// The duel is the challenger's and his alone, as a duel's two are each other's (duel.go):
//   - only the challenger, and the challenger's own pet, may hurt him: anyone else's melee is not
//     engaged (attack.go), a harmful spell at him is refused and an area spell passes him by (cast.go,
//     area.go), another's pet will not go for him (pet.go), each told so in a line (at most one every
//     MetalTellEvery); no guard, social helper or trigger takes him on, and he helps no one;
//   - he fights the challenger alone: hate from anyone else is no hate on him (addThreatLocked), he
//     attacks no one on sight, a pet's taunt does not turn him, and a challenger he has let go of
//     (a feigned death, an invisibility) is taken up again at once;
//   - every other client is shown him with the NPC bit (challengeViewLocked: CLIENT will not swing at
//     him, and a click talks to him, answered in a line), the challenger as a creature to fight;
//   - the challenger may not have Mr. Moogie's help while it lasts (moogie.go's MoogieBusy). Others'
//     heals and buffs on the challenger land, as in a duel.
//
// It ends:
//   - at his death: the challenger has won, and his corpse lies its time with a single Moogie
//     Tribute on it (his npcitem row), the challenger's alone to loot (ChallengeLooter); no
//     experience, coins, faction or other loot;
//   - at the challenger's defeat, a hit it could not take: as a duel's loser it is left at 1 HP (no
//     death, no debt), he leaves the world at once and Mr. Moogie calls it pathetic in /say, heard
//     within MetalSayHeard of him (petition 1380);
//   - when the challenger flees: another world, or past MetalArenaRadius of the arena's centre
//     (halfway between where he rose and where the challenger stood); after MetalMaxTime; at the
//     challenger's logout or a new login: he leaves the world, the challenger told why.
//
// Metal Moogie V2 (the GM Jibrish's petition 1441: "if a player has a token from defeating metal moogie
// that can instead choose to use it to summon Metal Moogie V2 who is 20% bigger and has all his stats
// and damaged tuned upward by 10%") is the same challenge of another original, the spawn-only NPC of his
// name, asked of Mr. Moogie with a Moogie Tribute (handlers' moogie.go takes it once he stands): every
// rule above is his, one duel of the two at a time on the whole server, his lines Metal Moogie's with his
// name (duelLine). His size and figures are his row's and template's, data (Metal Moogie's at 1.2 and
// 1.1, and a ranged attack of his own: petition 1442); his corpse gives his own items as Metal Moogie's
// does (the data's: one Moogie Tribute, as his).

// MetalMoogieName is his name: the spawn-only NPC of it is the original copied; MetalMoogieV2Name his
// remade twin's (petition 1441).
const (
	MetalMoogieName   = "Metal Moogie"
	MetalMoogieV2Name = "Metal Moogie V2"
)

// The challenge's numbers (ours).
const (
	// MetalArenaRadius is how far from the arena's centre the challenger may go (DuelArenaRadius).
	MetalArenaRadius float32 = 60
	// MetalMaxTime ends a duel that long with no winner.
	MetalMaxTime = 15 * time.Minute
	// MetalRiseNear: he rises at his original's spot when it is this near the Mr. Moogie asked.
	MetalRiseNear float32 = 60
	// MetalRiseOut is how far from Mr. Moogie toward the challenger he rises otherwise.
	MetalRiseOut float32 = 25
	// MetalTellEvery is the most often a player is told he is another's to fight.
	MetalTellEvery = 4 * time.Second
	// MetalSayHeard is who hears Mr. Moogie's /say (chat's say radius).
	MetalSayHeard float32 = 200
	// metalSpareFor is how long after a lost duel the challenger is held at 1 HP.
	metalSpareFor = time.Second
	// metalRiseFloor is how far from his spot's height the ground's floor may be for him to stand on it.
	metalRiseFloor float32 = 20
	// MetalCountdown is the count from his rising to the fight, in seconds (petition 1623), a line each.
	MetalCountdown = 5
	// metalCountStep is the time between the countdown's lines.
	metalCountStep = time.Second
)

// The challenge's words (ours), Mr. Moogie's in his voice ("I am king of this jungle").
const (
	MetalLineRises   = "Mr. Moogie says, 'So, %s, you would prove your worth? Face my metal twin, and you alone!'"
	MetalLineRisesV2 = "Mr. Moogie says, 'Your tribute buys a greater trial, %s: face my metal twin remade, and you alone!'"
	MetalLineBusy    = "Mr. Moogie says, 'My metal twin is already dueling %s. One challenger at a time.'"
	MetalLineMine    = "Mr. Moogie says, 'You are already dueling my metal twin. Finish what you started.'"
	MetalLineAbsent  = "Mr. Moogie says, 'My metal twin is not here today.'"
	MetalLineDueling = "Mr. Moogie will not help you while you duel his metal twin."
	MetalLineNotYour = "Metal Moogie duels %s alone."
	MetalLinePet     = "Metal Moogie duels %s alone: your pet will not attack him."
	MetalLineCharm   = "Metal Moogie cannot be charmed."
	MetalLineTalk    = "Metal Moogie is dueling %s and pays you no mind."
	MetalLineWon     = "Metal Moogie falls! Take his Moogie Tribute to Mr. Moogie."
	MetalLineLost    = "Metal Moogie has defeated you."
	MetalLineFled    = "You fled the duel. Metal Moogie returns to Mr. Moogie's side."
	MetalLineTime    = "The duel has gone on too long. Metal Moogie returns to Mr. Moogie's side."
	MetalLineLoot    = "Metal Moogie's tribute is %s's alone."
	// the countdown (petition 1623, Jibrish's words): the first line, each second after it, the start
	MetalLineCount  = "Fight starts in %d"
	MetalLineCountN = "%d"
	MetalLineFight  = "FIGHT!"
	MetalLineWait   = "The fight has not begun. Wait for FIGHT!"
	// MetalSayPathetic is Mr. Moogie's /say at a challenger's defeat (petition 1380)
	MetalSayPathetic = "Pathetic, %s! My metal twin barely warmed its gears. Come back when you are worthy of the king of this jungle."
)

// npcChallenge is who a Metal Moogie duels: the challenger as it was asked (a new login under the same
// id is not it), its character and its name.
type npcChallenge struct {
	player EntityID
	who    *Player
	char   uint32
	name   string
	// held is the countdown before the fight (petition 1623): no one may hurt him, he hurts no one
	held bool
}

// metalDuel is the Metal Moogie standing: his NPC and its name, the Mr. Moogie who sent him, the arena's
// centre and world, and when it began.
type metalDuel struct {
	npc     EntityID
	name    string
	moogie  EntityID
	c       *npcChallenge
	world   uint8
	x, y, z float32
	began   time.Time
	// count is the countdown's next number to say (MetalCountdown down to 1, 0 for FIGHT), at next
	count int
	next  time.Time
}

// metalOriginalLocked is the spawn-only NPC of his name, nil when the server has none. Must be called
// with the lock held.
func (w *World) metalOriginalLocked() *NPC {
	return w.metalOriginalOfLocked(MetalMoogieName)
}

// metalOriginalOfLocked is the spawn-only NPC of the name (Metal Moogie's or V2's), nil for none. Must be
// called with the lock held.
func (w *World) metalOriginalOfLocked(name string) *NPC {
	t := w.npcByNameLocked(name)
	if t == nil || !t.SpawnOnly {
		return nil
	}
	return t
}

// MetalMoogieHere reports whether Mr. Moogie can send his metal twin: the spawn-only original is there.
func (w *World) MetalMoogieHere() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.metalOriginalLocked() != nil
}

// MetalMoogieV2Here reports whether Mr. Moogie can send his metal twin remade for a tribute: the
// spawn-only original of Metal Moogie V2 is there (petition 1441).
func (w *World) MetalMoogieV2Here() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.metalOriginalOfLocked(MetalMoogieV2Name) != nil
}

// duelLine is one of the challenge's lines that names Metal Moogie, naming npc instead when it is another
// (Metal Moogie V2: "Metal Moogie V2 duels Anna alone."); its first "Metal Moogie" alone, so Mr. Moogie
// stays himself.
func duelLine(line string, npc *NPC) string {
	if npc == nil || strings.EqualFold(npc.Name, MetalMoogieName) {
		return line
	}
	return strings.Replace(line, MetalMoogieName, npc.Name, 1)
}

// line is duelLine for the duel standing, by its NPC's name.
func (d *metalDuel) line(line string) string {
	if d == nil || d.name == "" || strings.EqualFold(d.name, MetalMoogieName) {
		return line
	}
	return strings.Replace(line, MetalMoogieName, d.name, 1)
}

// MetalMoogieChallenge is sess's player asking Mr. Moogie (moogie, his NPC) to prove its worth: a Metal
// Moogie rises and goes for it, the duel the file's doc's. It returns Mr. Moogie's answer, and whether
// one rose.
func (w *World) MetalMoogieChallenge(sess *session.Session, moogie EntityID, now time.Time) (string, bool) {
	return w.metalChallenge(sess, moogie, MetalMoogieName, MetalLineRises, now)
}

// MetalMoogieV2Challenge is MetalMoogieChallenge of Metal Moogie V2, for a tribute the caller takes once
// he stands (petition 1441, handlers' moogie.go).
func (w *World) MetalMoogieV2Challenge(sess *session.Session, moogie EntityID, now time.Time) (string, bool) {
	return w.metalChallenge(sess, moogie, MetalMoogieV2Name, MetalLineRisesV2, now)
}

// metalChallenge is a challenge of the spawn-only original of the name, risen with that line.
func (w *World) metalChallenge(sess *session.Session, moogie EntityID, name, rises string, now time.Time) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.IsDead() || p.respawnPending() {
		return MoogieLineDead, false
	}
	if d := w.metal; d != nil {
		if d.c.player == p.ServerID {
			return MetalLineMine, false
		}
		return fmt.Sprintf(MetalLineBusy, d.c.name), false
	}
	m := w.NPCs[moogie]
	t := w.metalOriginalOfLocked(name)
	if m == nil || m.Dead || t == nil {
		return MetalLineAbsent, false
	}
	x, y, z := t.SpawnX, t.SpawnY, t.SpawnZ
	if t.World != m.World || dist3(x, y, z, m.X, m.Y, m.Z) > MetalRiseNear {
		dx, dz := p.X-m.X, p.Z-m.Z
		if l := float32(math.Hypot(float64(dx), float64(dz))); l > 0.01 {
			dx, dz = dx/l, dz/l
		} else {
			dx, dz = float32(math.Sin(float64(m.Facing))), float32(math.Cos(float64(m.Facing)))
		}
		x, y, z = m.X+dx*MetalRiseOut, m.Y, m.Z+dz*MetalRiseOut
	}
	if w.ground != nil {
		if fy, ok := w.ground.Floor(m.World, x, y+3, z); ok && fy-y <= metalRiseFloor && y-fy <= metalRiseFloor {
			y = fy
		}
	}
	facing := headingTo(p.X-x, p.Z-z)
	c := w.spawnCopyLocked(t, x, y, z, facing, m.World)
	ch := &npcChallenge{player: p.ServerID, who: p, char: p.CharServerID, name: p.Name, held: true}
	c.challenge = ch
	c.NPCType &^= 0x80 // the challenger's to fight
	c.EventAggro = false
	// no hate yet: he goes for the challenger at the countdown's FIGHT (metalMoogieSystem, petition 1623)
	w.metal = &metalDuel{npc: c.ServerID, name: c.Name, moogie: m.ServerID, c: ch, world: m.World,
		x: (x + p.X) / 2, y: (y + p.Y) / 2, z: (z + p.Z) / 2, began: now, count: MetalCountdown, next: now}
	log.Printf("Metal Moogie: %s challenges Mr. Moogie: %s (%d, level %d, %d HP, size %.2f, template %q) rises at (%.1f, %.1f, %.1f)",
		p.Name, c.Name, c.ServerID, c.Level, c.MaxHP, c.Size, c.TemplateName(), x, y, z)
	return fmt.Sprintf(rises, p.Name), true
}

// EndMetalDuelOf ends the duel of sess's player now, its Metal Moogie out of the world (a tribute that
// could not be taken: handlers' moogie.go). It reports whether one was on.
func (w *World) EndMetalDuelOf(sess *session.Session, how string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || !w.metalDuelOfLocked(p.ServerID) {
		return false
	}
	w.endMetalLocked(how, time.Now())
	return true
}

// MetalDuelOf reports whether the session's player is dueling a Metal Moogie now.
func (w *World) MetalDuelOf(sess *session.Session) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	return p != nil && w.metalDuelOfLocked(p.ServerID)
}

// metalDuelOfLocked reports whether the player id duels the Metal Moogie standing. Must be called
// with the lock held.
func (w *World) metalDuelOfLocked(id EntityID) bool {
	return w.metal != nil && w.metal.c.player == id
}

// metalSparesLocked reports whether an area of player id's (or its pet's) must leave npc out: Mr.
// Moogie, while id duels his Metal Moogie (the GM Jibrish's petition 1619: a challenger's pet splashed
// Mr. Moogie, standing beside his twin, and he killed it). Must be called with the lock held.
func (w *World) metalSparesLocked(npc *NPC, id EntityID) bool {
	return npc != nil && w.metalDuelOfLocked(id) && strings.EqualFold(strings.TrimSpace(npc.Name), MoogieName)
}

// challengeBars reports whether npc is a Metal Moogie that player id may not hurt: another's to fight, or
// anyone's during the countdown (petition 1623); id a player, or the owner of the pet that would.
func challengeBars(npc *NPC, id EntityID) bool {
	if siegeBars(npc, id) {
		return true // a siege's King or gate: its attackers' alone, once live (siege.go)
	}
	return npc != nil && npc.challenge != nil && (npc.challenge.held || npc.challenge.player != id)
}

// duelHeld reports whether n is a Metal Moogie in his countdown (petition 1623): nothing hurts him.
func (n *NPC) duelHeld() bool { return n != nil && n.challenge != nil && n.challenge.held }

// challengeRefusal is the line for one barred from a Metal Moogie (challengeBars), "" for none: the
// challenger during the countdown is told to wait.
func challengeRefusal(npc *NPC, id EntityID) string {
	if line := siegeRefusal(npc, id); line != "" {
		return line // a siege's King or gate (siege.go)
	}
	if !challengeBars(npc, id) {
		return ""
	}
	if npc.challenge.player == id {
		return MetalLineWait
	}
	return fmt.Sprintf(duelLine(MetalLineNotYour, npc), npc.challenge.name)
}

// tellChallengeLocked tells p, barred from npc, so in a line: at most one every MetalTellEvery. Must be
// called with the lock held.
func (w *World) tellChallengeLocked(sess *session.Session, p *Player, line string, now time.Time) {
	if sess == nil || p == nil || line == "" {
		return
	}
	if w.metalTold == nil {
		w.metalTold = map[EntityID]time.Time{}
	}
	if last, ok := w.metalTold[p.ServerID]; ok && now.Sub(last) < MetalTellEvery {
		return
	}
	w.metalTold[p.ServerID] = now
	w.sendColoredLocked(sess, line, duelRed)
}

// challengeViewLocked shows every client but the challenger's a Metal Moogie with the NPC bit (the
// wire's byte 146 0x80, byte 143 0, as an NPC no one may attack is sent): CLIENT swings at no such
// creature, and a click talks to it. Must be called with the lock held.
func (w *World) challengeViewLocked(sess *session.Session, npc *NPC, buf []byte) {
	if npc.siege != nil {
		w.siegeViewLocked(w.sessionPlayerLocked(sess), npc, buf) // a siege's, its attackers' alone (siege.go)
		return
	}
	if npc.challenge == nil || npc.Dead || len(buf) <= 146 {
		return
	}
	if viewer := w.sessionPlayerLocked(sess); viewer != nil && viewer.ServerID == npc.challenge.player {
		return
	}
	buf[146] |= 0x80
	buf[143] = 0
}

// MetalMoogieTalk is a talk to npc: the line when it is a Metal Moogie standing, ok false otherwise.
func (w *World) MetalMoogieTalk(npc *NPC) (string, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if npc == nil || npc.Dead || npc.challenge == nil {
		return "", false
	}
	return fmt.Sprintf(duelLine(MetalLineTalk, npc), npc.challenge.name), true
}

// ChallengeLooter reports a Metal Moogie's corpse: whether npc is one, whether the session's character
// is its challenger (the only one who may loot it), and the line for anyone else.
func (w *World) ChallengeLooter(sess *session.Session, npc *NPC) (duel, mine bool, line string) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if npc == nil || npc.challenge == nil {
		return false, false, ""
	}
	p := w.sessionPlayerLocked(sess)
	if p != nil && p.CharServerID == npc.challenge.char {
		return true, true, ""
	}
	return true, false, fmt.Sprintf(duelLine(MetalLineLoot, npc), npc.challenge.name)
}

// DuelSpawn reports a Metal Moogie (or his corpse): a duel's, whose corpse gives his own items alone (no
// coins, no loot tables of every creature's, no category rolls).
func (n *NPC) DuelSpawn() bool { return n != nil && (n.challenge != nil || n.siege != nil) } // a siege's gives nothing (siege.go)

// challengeDefeatLocked is p brought to nothing (playerDownLocked) while it duels a Metal Moogie, or
// held at 1 HP after one: a duel's loser, left at 1 HP, the duel lost (petition 1380: he leaves the
// world, and Mr. Moogie calls p pathetic in /say). It reports whether it was. Must be called with the
// lock held.
func (w *World) challengeDefeatLocked(sess *session.Session, p *Player, now time.Time) bool {
	if now.Before(p.metalSpared) {
		p.HP = max(p.HP, 1)
		p.StatsDirty = true
		return true
	}
	d := w.metal
	if d == nil || d.c.player != p.ServerID || d.c.who != p {
		return false
	}
	p.HP = 1
	p.StatsDirty = true
	p.metalSpared = now.Add(metalSpareFor)
	if sess != nil {
		w.sendColoredLocked(sess, d.line(MetalLineLost), duelOrange)
	}
	w.endMetalLocked("defeated", now)
	w.moogieSayLocked(d, fmt.Sprintf(MetalSayPathetic, p.Name))
	return true
}

// moogieSayLocked is Mr. Moogie's line in /say ("Mr. Moogie: text", as a say is sent), to every player
// within MetalSayHeard of him in his world: the Mr. Moogie who sent the duel's Metal Moogie, or, gone,
// where the duel was. Must be called with the lock held.
func (w *World) moogieSayLocked(d *metalDuel, text string) {
	world, x, z := d.world, d.x, d.z
	if m := w.NPCs[d.moogie]; m != nil {
		world, x, z = m.World, m.X, m.Z
	}
	n := w.sayNearLocked(MoogieName, world, x, z, text) // abilitytimes.go
	log.Printf("Metal Moogie: %q to %d players", MoogieName+": "+text, n)
}

// endMetalLocked ends the duel: Metal Moogie leaves the world unless he lies dead (his corpse lies its
// time). how says why, for the log. Must be called with the lock held.
func (w *World) endMetalLocked(how string, now time.Time) {
	d := w.metal
	if d == nil {
		return
	}
	w.metal = nil
	if npc := w.NPCs[d.npc]; npc != nil && npc.challenge == d.c && !npc.Dead {
		w.removeNPCLocked(npc)
	}
	log.Printf("Metal Moogie: %s's duel with %s ends (%s) after %v", d.c.name, d.name, how, now.Sub(d.began).Round(time.Second))
}

// metalMoogieSystem keeps the Metal Moogie standing to his duel: it ends at his death (won), the
// challenger gone (a logout, a new login), fled (another world, past MetalArenaRadius) or after
// MetalMaxTime; he takes the challenger up again whenever he let it go. A defeat is
// challengeDefeatLocked's, at the hit. Every tick; must be called with the lock held.
func (w *World) metalMoogieSystem(now time.Time) {
	d := w.metal
	if d == nil {
		return
	}
	npc := w.NPCs[d.npc]
	if npc == nil || npc.challenge != d.c {
		w.endMetalLocked("gone", now) // a GM's #despawn
		return
	}
	p := w.Players[d.c.player]
	if p == nil || p != d.c.who {
		w.endMetalLocked("the challenger left", now)
		return
	}
	sess := w.findSessionByEntityIDLocked(p.ServerID)
	if npc.Dead {
		w.endMetalLocked("won", now)
		if sess != nil {
			w.sendColoredLocked(sess, d.line(MetalLineWon), duelOrange)
		}
		return
	}
	if p.World != d.world || dist3(p.X, p.Y, p.Z, d.x, d.y, d.z) > MetalArenaRadius {
		w.endMetalLocked("fled", now)
		if sess != nil {
			w.sendColoredLocked(sess, d.line(MetalLineFled), duelOrange)
		}
		return
	}
	if d.c.held {
		w.metalCountLocked(d, npc, p, sess, now)
		return
	}
	if now.Sub(d.began) >= MetalMaxTime {
		w.endMetalLocked("time", now)
		if sess != nil {
			w.sendColoredLocked(sess, d.line(MetalLineTime), duelOrange)
		}
		return
	}
	if npc.CurrentEnemy != p.ServerID && !p.IsDead() {
		w.addThreatLocked(npc, p.ServerID, 1) // let go (a feigned death, an invisibility): taken up again
		npc.CurrentEnemy = p.ServerID
	}
}

// metalCountLocked is the countdown's tick (petition 1623): its next line to the challenger when its
// second has come, "Fight starts in 5", "4" .. "1" a second apart, then "FIGHT!", at which the hold ends
// and he goes for the challenger. Must be called with the lock held.
func (w *World) metalCountLocked(d *metalDuel, npc *NPC, p *Player, sess *session.Session, now time.Time) {
	if now.Before(d.next) {
		return
	}
	line := MetalLineFight
	switch {
	case d.count == MetalCountdown:
		line = fmt.Sprintf(MetalLineCount, d.count)
	case d.count > 0:
		line = fmt.Sprintf(MetalLineCountN, d.count)
	}
	if sess != nil {
		w.sendColoredLocked(sess, line, duelOrange)
	}
	if d.count > 0 {
		d.count--
		d.next = d.next.Add(metalCountStep)
		if !d.next.After(now) {
			d.next = now.Add(metalCountStep) // a late tick: a second from this one, never two lines at once
		}
		return
	}
	d.c.held = false
	npc.Threats = nil
	w.addThreatLocked(npc, p.ServerID, 1) // he goes for the challenger now, the duel's rules as before
	npc.CurrentEnemy = p.ServerID
	log.Printf("Metal Moogie: %s's duel with %s begins (FIGHT, %v after he rose)", d.c.name, d.name, now.Sub(d.began).Round(time.Millisecond))
}

// metalLeavingLocked is p leaving the world: its duel ends at once, before the id is another's. Must be
// called with the lock held.
func (w *World) metalLeavingLocked(p *Player) {
	if d := w.metal; d != nil && d.c.player == p.ServerID && d.c.who == p {
		w.endMetalLocked("the challenger left", time.Now())
	}
	delete(w.metalTold, p.ServerID)
}

// DuelistsFavorName is the spell a Moogie Tribute handed to Mr. Moogie lands on its giver (spellpattern,
// spelleffect: "Self, Increase Size by 50%, 60min"; the data's, by this name), and MoogieTributeName the
// item (itempattern: no trade, lore).
const (
	DuelistsFavorName = "Duelist's Favor"
	MoogieTributeName = "Moogie Tribute"
)

// GiveDuelistsFavor lands spell, Mr. Moogie's favor, on the session's player as LandOnSelf does (its
// visual to those round it, its effect for its time), kept (Rules.Kept: it ends only at its time or a
// death, as Jibcro's "lasts for 1 hour or until death"). False with no living player, or when the spell
// leaves nothing lasting on it.
func (w *World) GiveDuelistsFavor(sess *session.Session, spell CastSpell, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.IsDead() || p.respawnPending() {
		return false
	}
	w.broadcastCastLocked(p, p.ServerID, 0, spell.visual())
	spell.figures = figuresGift // Mr. Moogie's, none of the player's: told untagged (numbertags.go)
	w.landOnPlayerLocked(&castState{spell: spell, caster: p.ServerID, targets: []EntityID{p.ServerID}}, p, sess, p, now)
	for _, ae := range p.Effects {
		if ae.SpellID == spell.SpellID {
			ae.Rules.Kept = true
			model, size := p.Body()
			log.Printf("Metal Moogie: %s has Mr. Moogie's %q until %s: shown at %.2f (body %#08x)", p.Name, spell.Name,
				ae.Until.UTC().Format("15:04:05"), size, model)
			return true
		}
	}
	log.Printf("Metal Moogie: %q left nothing on %s", spell.Name, p.Name)
	return false
}

// metalWord is the duel in a line for #stats: whom a Metal Moogie duels, "" for any other NPC.
func metalWord(npc *NPC) string {
	if npc == nil || npc.challenge == nil {
		return ""
	}
	return fmt.Sprintf("Duels %s alone (Mr. Moogie's challenge): only %s and %s's pet may hurt it", npc.challenge.name,
		npc.challenge.name, npc.challenge.name)
}
