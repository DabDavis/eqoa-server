package handlers

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// raidloot.go — a raid's loot rules (the GM Jibrish's petitions 1448 and 1451, 2026-11-10;
// entity/raidrules.go keeps a raid's rule and entity/raidcmds.go its commands).
//
// FREE FOR ALL, every group's way and a raid's until its leader picks another: anyone
// loots anything (loot.go).
//
// NEED BEFORE GREED. A raid's kill (the kill credited to one of its members while its
// rule is this: entity.KillCredit.Raid) is rolled at its death rather than at its first
// opening, for the highest level among its seats (loot.go's lootLevelReach), and each item
// the rules cover (raidCovers: never a stackable or vendor trash; a no trade item, an item
// with stats, or anything else a raid boss drops) is taken off it and offered to its seats
// (the raid's members within KillShareRange of the corpse and those kicked from it during
// the fight, entity's RaidKill): Need (a seat the equip rules let wear or use it:
// cannotWear, scrollRefusal), Greed (any seat), or Pass, for raidRollFor. A choice of Need
// or Greed rolls 0 to 100 at once and is said to the raid; when every seat has chosen, or
// the time is up (no choice is a pass), the highest Need wins, else the highest Greed, a
// tie rolling again between the tied. No roll: the item lies on the corpse for anyone. The
// winner's item goes into its pack at once when there is room; else it waits on the
// corpse for the winner alone for raidHoldFor, and the winner's opening the corpse then
// puts its won items in its pack (or says the pack is full) and opens no window, so no
// one sees them; past that it is anyone's. The rest of the corpse is anyone's throughout.
//
// MASTER LOOTER. A raid's kill holding an item the rules cover is its leader's alone for
// raidHoldFor: anyone else is told so (0x0085 3, word 675: "That corpse belongs to another
// player or group"). The leader takes as anyone does, gives an item to a member or one
// kicked in the fight (Assign: into that one's pack, or a refusal when it is full), or
// lets the whole corpse go to anyone (Abandon); past the time it is anyone's. A kill with
// nothing the rules cover is free for all.
//
// A corpse the rules hold lies until they are done, then decays as any does from then
// (entity's HoldCorpse).
//
// RAID BOSS (ours): a creature whose class template gives it raidBossHPPct or more of its
// level's hit points (every tuned raid boss is 1,008% to 10,417%), whose loot categories'
// profile is "boss" (lootcreature: a GM gives any creature one on the panel), or one of
// whose loot tables is a "boss ..." one (database.RaidBoss).
//
// A client that answered the raid offer (protocol.OpcodeRaidAnswer) is offered its rolls as
// windows (0x7E45, answered by 0x7E46) and its master looter's list comes as 0x7E47 (and is
// answered by 0x7E48 3 and 4); any other client, a PS2, is told the same in chat lines and
// answers by typing "/t need [n]", "/t greed [n]", "/t pass [n]" (n the roll's number: with
// none, the oldest it has not answered), "/t assign <number> <name>" and "/t abandon"
// (raidcmds.go routes them). Every line is in the raid's orange.

const (
	// raidRollFor is a roll's base time (petition 1448: "People have 120 seconds to choose";
	// the GM Jibwiz's petition 1939, 2026-11-13: "The base time for the rolls is too short. Add
	// 5 seconds to it").
	raidRollFor = 125 * time.Second
	// raidRollEach is the time every open roll of a raid gains for each other one rolled at the
	// same time (petition 1939: "Add an additional amount of time to ALL items for each one that
	// is being rolled at the same time. Eg if its 60 seconds for one, its 70 for both of them for
	// 2 80 for 3 etc."): a kill's three items roll for 125 + 2 x 10 seconds each; a second kill's
	// items while those are open add their count's 10 s to each of those too (rollTimesLocked).
	raidRollEach = 10 * time.Second
	// raidHoldFor is a winner's time to take its item off the corpse, and a master
	// looter's hold (petition 1448: "a period of 5 minutes"; ours for the master looter).
	raidHoldFor = 5 * time.Minute
	// raidRollTop is a roll's top: 0 to 100 (petition 1448).
	raidRollTop = 100
	// raidBossHPPct is the class template hit points from which a creature is a raid boss
	// (the World's, which credits a raid boss's kill by the same rule: entity/heroes.go).
	raidBossHPPct = entity.RaidBossHPPct
	// raidRollIDs is the most roll numbers: they go round, 1 to 999.
	raidRollIDs = 999
)

// rollChoice is a seat's choice.
type rollChoice byte

const (
	rollNone  rollChoice = 0
	rollNeed  rollChoice = 1
	rollGreed rollChoice = 2
	rollPass  rollChoice = 3
)

func (c rollChoice) String() string {
	switch c {
	case rollNeed:
		return "Need"
	case rollGreed:
		return "Greed"
	case rollPass:
		return "Pass"
	}
	return "nothing"
}

// rollSeat is one seat of a roll. Its session is found by its character at each
// message (a takeover or a restart makes another).
type rollSeat struct {
	entity.RaidSeat
	mayNeed bool
	choice  rollChoice
	roll    int
	closed  bool // its window or its lines are done with
}

// lootRoll is one item's roll.
type lootRoll struct {
	id     int
	corpse entity.EntityID
	died   time.Time
	group  uint32
	item   *heldItem
	ends   time.Time
	seats  []*rollSeat
}

// heldItem is an item a raid's rules hold off the corpse: rolled on (roll), or won and
// waiting for its winner until until.
type heldItem struct {
	rec        database.CharInventoryEntry
	roll       *lootRoll
	winner     uint32 // characters.serverid
	winnerName string
	until      time.Time
}

// raidCorpse is a corpse's raid hold (corpse.raid).
type raidCorpse struct {
	rule  entity.LootRule
	group uint32
	from  string // the creature's name, for the lines
	// the master looter (LootMaster): its character, 0 once let go; its name; its time
	master     uint32
	masterName string
	until      time.Time
	// members are whom the master looter may give to, as the kill had them
	members []entity.RaidSeat
	held    []*heldItem
}

// raidHooks are the World's, for the rules (setRaidHooks); tests stand in for them.
type raidHooks struct {
	// seat is a character's session and player in the world, or nils
	seat func(char uint32) (*session.Session, *entity.Player)
	// sessions are a raid's members' sessions now
	sessions func(group uint32) []*session.Session
	// windows is whether a session's client shows the raid's windows
	windows func(*session.Session) bool
	// boss is whether the tables make a creature of that name a raid boss
	boss func(name string) bool
	// hold keeps a corpse lying until then (World.HoldCorpse)
	hold func(id entity.EntityID, died, until time.Time) bool
	// npc is the NPC of that id, or nil
	npc func(id entity.EntityID) *entity.NPC
	// seen is the id recv's client knows a player by
	seen func(recv *session.Session, p *entity.Player) uint32
	// seenBy resolves an id sender's client gave to a player
	seenBy func(sender *session.Session, id uint32) *entity.Player
	// decay is the world's corpse decay
	decay func() time.Duration
	now   func() time.Time
	intn  func(int) int
}

// raidState is the handler's raid loot beyond the corpses: the kills heard and not yet
// rolled (by creature), the worker's wake, and which corpse each master looter was last
// listed (for a PS2's typed assign, its window closed).
type raidState struct {
	kills  sync.Map // entity.EntityID -> entity.KillCredit
	wake   chan struct{}
	once   sync.Once
	listed map[uint32]entity.EntityID
}

// setRaidHooks gives the rules the World (SetGameWorld), hears its raids' kills and
// starts the worker that rolls them and ends what is due.
func (h *NPCWindowHandler) setRaidHooks(gw *entity.World) {
	h.raid = raidHooks{
		seat: func(char uint32) (*session.Session, *entity.Player) {
			s := SessionOfCharacter(gw, h.sessionManager, char)
			return s, gw.SessionPlayer(s)
		},
		sessions: gw.RaidSessions,
		windows:  func(s *session.Session) bool { return gw.RaidWindows(gw.SessionPlayer(s)) },
		hold:     gw.HoldCorpse,
		npc:      gw.NPCByID,
		seen:     gw.IDSeenBy,
		seenBy:   gw.PlayerSeenBy,
		decay: func() time.Duration {
			d, _ := gw.CorpseTimes()
			return d
		},
	}
	gw.OnKill(h.heardRaidKill)
}

// startRaidWorker starts, once, the worker that rolls the raid kills heard and ends what
// is due: at the first raid kill heard, or a raid hold a restart brought back.
func (h *NPCWindowHandler) startRaidWorker() {
	h.raids.once.Do(func() {
		h.raids.wake = make(chan struct{}, 1)
		go h.raidWorker(h.raids.wake)
	})
}

func (h *NPCWindowHandler) raidClock() time.Time {
	if h.raid.now != nil {
		return h.raid.now()
	}
	return time.Now()
}

func (h *NPCWindowHandler) raidIntn(n int) int {
	if h.raid.intn != nil {
		return h.raid.intn(n)
	}
	return rand.Intn(n)
}

func (h *NPCWindowHandler) raidSeatOf(char uint32) (*session.Session, *entity.Player) {
	if h.raid.seat == nil || char == 0 {
		return nil, nil
	}
	return h.raid.seat(char)
}

func (h *NPCWindowHandler) raidWindowsOf(sess *session.Session) bool {
	return sess != nil && h.raid.windows != nil && h.raid.windows(sess)
}

// raidBoss is the raid boss rule (the file's doc) for a creature of that name and
// template.
func (h *NPCWindowHandler) raidBoss(name string, templateHP int) bool {
	if templateHP >= raidBossHPPct {
		return true
	}
	if h.raid.boss != nil {
		return h.raid.boss(name)
	}
	if h.inv == nil {
		return false
	}
	boss, err := h.inv.RaidBoss(name)
	if err != nil {
		log.Printf("Raid loot: is %q a raid boss: %v", name, err)
	}
	return boss
}

// holdCorpseFor keeps a corpse lying until end and then its decay (the file's doc).
func (h *NPCWindowHandler) holdCorpseFor(id entity.EntityID, died, end time.Time) {
	if h.raid.hold == nil {
		return
	}
	decay := entity.DefaultCorpseDecay
	if h.raid.decay != nil {
		decay = h.raid.decay()
	}
	h.raid.hold(id, died, end.Add(decay))
}

// itemHasStats is whether an item gives anything when worn or held: a stat, hit points,
// power, AC, a resist, regeneration, a weapon's damage or proc.
func itemHasStats(p *database.ItemPatternRecord) bool {
	for _, v := range []*int{p.Str, p.Sta, p.Agi, p.Wis, p.Dex, p.Cha, p.Intelligence, p.HPMAX, p.POWMAX, p.PoT, p.HoT,
		p.AC, p.PR, p.DR, p.FR, p.CR, p.LR, p.AR, p.WeaponProc} {
		if v != nil && *v != 0 {
			return true
		}
	}
	return p.WeaponDamage > 0
}

// vendorTrash is an item worth its price alone: no stats, nothing to wear, no scroll,
// not Lore, not No Trade, and a price to sell it for. An item with no price is a quest
// piece or a key (the owner, 2026-11-13: the Ascended Ring of Veeshan, off the
// Quintessence of Veeshan, went to whoever looted first: "special items like that
// should" use need before greed).
func vendorTrash(p *database.ItemPatternRecord) bool {
	return !itemHasStats(p) && p.EquipSlot < 1 && !isScroll(p) && p.Lore == 0 && p.Trade == 0 && p.ItemCost > 0
}

// raidCovers is whether a raid's rules hold an item (petition 1448: "raid bosses, no trade
// items and items with stats, not stack items or vendor trash").
func raidCovers(p *database.ItemPatternRecord, boss bool) bool {
	if p.MaxStack > 1 {
		return false
	}
	if p.Trade > 0 || itemHasStats(p) {
		return true
	}
	return boss && !vendorTrash(p)
}

// mayNeed is whether a seat may roll Need on an item: the equip rules let it wear or use
// it (petition 1448: "only people who can equip or use the item can roll on it").
func mayNeed(s entity.RaidSeat, p *database.ItemPatternRecord) bool {
	if isScroll(p) {
		return scrollRefusal(p.ClassUse, p.RaceUse, p.LevelReq, s.Class, s.Race, s.Level) == ""
	}
	return cannotWear(&entity.Player{Class: s.Class, Race: s.Race, Level: s.Level}, p) == ""
}

// heardRaidKill takes a kill from the World (its lock held: never wait here): a raid's is
// kept to be rolled, by the worker or the corpse's first opening, whichever is first.
func (h *NPCWindowHandler) heardRaidKill(k entity.KillCredit) {
	if k.Raid == nil {
		return
	}
	h.raids.kills.Store(k.NPCID, k)
	h.startRaidWorker()
	select {
	case h.raids.wake <- struct{}{}:
	default:
	}
}

// raidWorker rolls the raid kills heard and ends, once a second, what is due.
func (h *NPCWindowHandler) raidWorker(wake <-chan struct{}) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-wake:
		case <-tick.C:
			h.RaidTick(h.raidClock())
		}
		h.takeRaidKills()
	}
}

// takeRaidKills rolls every raid kill heard and not yet rolled.
func (h *NPCWindowHandler) takeRaidKills() {
	h.raids.kills.Range(func(key, _ any) bool {
		v, ok := h.raids.kills.LoadAndDelete(key)
		if !ok {
			return true
		}
		k := v.(entity.KillCredit)
		var npc *entity.NPC
		if h.raid.npc != nil {
			npc = h.raid.npc(k.NPCID)
		}
		if npc == nil || !npc.Dead || !npc.DeathTime.Equal(k.Died) {
			return true // gone, or up again
		}
		ls := h.loots()
		ls.mu.Lock()
		h.startRaidLootLocked(ls, npc, k)
		ls.mu.Unlock()
		return true
	})
}

// takeRaidKillLocked rolls npc's death died as its raid's when it was one and is not
// rolled yet: OpenCorpse's first look, so an opening before the worker cannot take it
// for free. Must be called with ls.mu held.
func (h *NPCWindowHandler) takeRaidKillLocked(ls *lootState, npc *entity.NPC, died time.Time) {
	v, ok := h.raids.kills.Load(npc.ServerID)
	if !ok {
		return
	}
	k := v.(entity.KillCredit)
	if !k.Died.Equal(died) {
		return
	}
	h.raids.kills.Delete(npc.ServerID)
	h.startRaidLootLocked(ls, npc, k)
}

// startRaidLootLocked rolls a raid kill's corpse and puts its rule on it (the file's doc).
// Must be called with ls.mu held.
func (h *NPCWindowHandler) startRaidLootLocked(ls *lootState, npc *entity.NPC, k entity.KillCredit) {
	if c := ls.corpses[npc.ServerID]; c != nil && c.died.Equal(k.Died) {
		return // opened already
	}
	if npc.Owner != 0 || npc.Charm != nil || npc.Dummy != entity.NotDummy {
		return // no one's to loot, or nothing to (loot.go)
	}
	r := k.Raid
	level := 0
	for _, s := range r.Seats {
		level = max(level, s.Level)
	}
	c := h.rollCorpse(npc, k.Died, level)
	ls.corpses[npc.ServerID] = c
	boss := h.raidBoss(npc.Name, r.TemplateHPPct)
	var covered []linkedItem
	for i := range c.items {
		if raidCovers(&c.items[i].Pattern, boss) {
			covered = append(covered, lootItem(c.items[i], 0))
		}
	}
	coveredList := linkedItems{covered, ", "}
	log.Printf("Raid loot: group %d's kill of %q (boss %v, %s): %d items, %d the rules hold", r.Group, npc.Name, boss, r.Rule, len(c.items), len(covered))
	if len(covered) == 0 {
		return // a corpse like any other
	}
	now := h.raidClock()
	rc := &raidCorpse{rule: r.Rule, group: r.Group, from: npc.Name, members: r.Members}
	c.raid = rc
	if r.Rule == entity.LootMaster {
		rc.master, rc.masterName, rc.until = r.Leader.ServerID, r.Leader.Name, now.Add(raidHoldFor)
		h.holdCorpseFor(npc.ServerID, k.Died, rc.until)
		h.raidSayLocked(rc, nil, h.linkedf(npc.Name, "%s's loot (%s) is the master looter's: %s's alone for %s.",
			npc.Name, coveredList, rc.masterName, spanWords(raidHoldFor)))
		if s, _ := h.raidSeatOf(rc.master); s != nil {
			how := "Open it to take, assign or abandon its loot."
			if !h.raidWindowsOf(s) {
				how = "Open it to see its items by number; then /t assign <number> <name>, or /t abandon to let anyone loot it."
			}
			SendColoredChat(s, "You are the master looter of "+npc.Name+"'s corpse. "+how, raidR, raidG, raidB)
		}
		return
	}
	keep := c.items[:0:0]
	var rolls []*lootRoll
	for _, it := range c.items {
		if !raidCovers(&it.Pattern, boss) {
			keep = append(keep, it)
			continue
		}
		hi := &heldItem{rec: it}
		roll := h.newRollLocked(ls, npc, k.Died, rc, hi, r.Seats, now)
		if roll == nil {
			keep = append(keep, it) // no number free: anyone's
			continue
		}
		hi.roll = roll
		rc.held = append(rc.held, hi)
		rolls = append(rolls, roll)
	}
	c.items = keep
	length := h.rollTimesLocked(ls, rc.group, rolls, now)
	h.holdCorpseFor(npc.ServerID, k.Died, now.Add(length+raidHoldFor))
	h.raidSayLocked(rc, nil, h.linkedf(npc.Name, "Need before greed on %s's loot: %s. Need, Greed or Pass within %d seconds.",
		npc.Name, coveredList, int(length/time.Second)))
	for _, roll := range rolls {
		h.offerRollLocked(roll, now)
	}
}

// rollTimesLocked sets the time of a raid's new rolls (raidRollEach's doc): each ends raidRollFor
// after now and raidRollEach more for every other roll of the group's open with it, the new ones
// among them; each roll of the group's already open gains raidRollEach for each new one, its
// corpse held as long, and its seats still to choose are offered it again with its new time (a
// window takes the new seconds in place of the old: vanilla's rollwnd). It returns the new rolls'
// time. Must be called with ls.mu held.
func (h *NPCWindowHandler) rollTimesLocked(ls *lootState, group uint32, rolls []*lootRoll, now time.Time) time.Duration {
	isNew := make(map[*lootRoll]bool, len(rolls))
	for _, r := range rolls {
		isNew[r] = true
	}
	var older []*lootRoll
	for _, r := range ls.rolls {
		if r.group == group && !isNew[r] && now.Before(r.ends) {
			older = append(older, r)
		}
	}
	sort.Slice(older, func(i, j int) bool { return older[i].id < older[j].id })
	length := raidRollFor + raidRollEach*time.Duration(len(older)+len(rolls)-1)
	for _, r := range rolls {
		r.ends = now.Add(length)
	}
	if len(rolls) == 0 {
		return length
	}
	more := raidRollEach * time.Duration(len(rolls))
	for _, r := range older {
		r.ends = r.ends.Add(more)
		h.holdCorpseFor(r.corpse, r.died, r.ends.Add(raidHoldFor))
		for _, s := range r.seats {
			if s.choice != rollNone || s.closed {
				continue
			}
			sess, _ := h.raidSeatOf(s.ServerID)
			switch {
			case sess == nil:
			case h.raidWindowsOf(sess):
				queueReliable(sess, protocol.OpcodeRaidRoll, rollPayload(r, s, now, readsCapes(sess)))
			default:
				SendColoredChat(sess, fmt.Sprintf("Roll %d: %d seconds now to choose.", r.id, rollSecondsLeft(r, now)), raidR, raidG, raidB)
			}
		}
	}
	if len(older) > 0 {
		log.Printf("Raid loot: group %d's %d open rolls gain %s for %d new ones", group, len(older), more, len(rolls))
	}
	return length
}

// spanWords is a time in the lines' words: "5 minutes", "90 seconds".
func spanWords(d time.Duration) string {
	if d >= 2*time.Minute {
		return fmt.Sprintf("%d minutes", int(math.Ceil(d.Minutes())))
	}
	return fmt.Sprintf("%d seconds", int(math.Ceil(d.Seconds())))
}

// newRollLocked is a roll on hi for the seats, under the next free number (nil when all
// raidRollIDs are taken). Must be called with ls.mu held.
func (h *NPCWindowHandler) newRollLocked(ls *lootState, npc *entity.NPC, died time.Time, rc *raidCorpse, hi *heldItem,
	seats []entity.RaidSeat, now time.Time) *lootRoll {
	for tries := 0; tries < raidRollIDs; tries++ {
		ls.nextRoll = ls.nextRoll%raidRollIDs + 1
		if ls.rolls[ls.nextRoll] == nil {
			r := &lootRoll{id: ls.nextRoll, corpse: npc.ServerID, died: died, group: rc.group, item: hi, ends: now.Add(raidRollFor)} // rollTimesLocked sets its end
			for _, s := range seats {
				r.seats = append(r.seats, &rollSeat{RaidSeat: s, mayNeed: mayNeed(s, &hi.rec.Pattern)})
			}
			ls.rolls[r.id] = r
			return r
		}
	}
	return nil
}

// raidR, raidG, raidB are the raid's lines' colour: its chat's orange (raidChatRGB).
const raidR, raidG, raidB = 255, 140, 0

// rollPayload is a roll's 0x7E45 to a seat (protocol.OpcodeRaidRoll): open, its seconds
// left, may it Need, the item; or closed (0 seconds, nothing after).
func rollPayload(r *lootRoll, s *rollSeat, now time.Time, capes bool) []byte {
	return build(protocol.OpcodeRaidRoll, 2048, func(w *buffer.Writer) {
		w.WriteUint32(uint32(r.id))
		left := 0
		if !s.closed {
			left = max(1, int(math.Ceil(r.ends.Sub(now).Seconds())))
		}
		w.WriteUint16(uint16(min(left, math.MaxUint16)))
		if left == 0 {
			return
		}
		var flags byte
		if s.mayNeed {
			flags |= 1
		}
		w.WriteByte(flags)
		writeSingleItem(w, &r.item.rec, r.id, capes)
	})
}

// offerRollLocked offers a roll to each of its seats: a window, or the lines. Must be
// called with ls.mu held.
func (h *NPCWindowHandler) offerRollLocked(r *lootRoll, now time.Time) {
	for _, s := range r.seats {
		sess, _ := h.raidSeatOf(s.ServerID)
		if sess == nil {
			continue
		}
		if h.raidWindowsOf(sess) {
			queueReliable(sess, protocol.OpcodeRaidRoll, rollPayload(r, s, now, readsCapes(sess)))
			continue
		}
		h.rollOfferLinked(r, s, now).colored(sess, raidR, raidG, raidB)
	}
}

// rollSecondsLeft is a roll's whole seconds left at now (at least 1).
func rollSecondsLeft(r *lootRoll, now time.Time) int {
	return max(1, int(math.Ceil(r.ends.Sub(now).Seconds())))
}

// rollOfferLine is a roll offered to a seat whose client shows no windows.
func rollOfferLine(r *lootRoll, s *rollSeat, now time.Time) string {
	item, secs := r.item.rec.Pattern.ItemName, rollSecondsLeft(r, now)
	if s.mayNeed {
		return fmt.Sprintf("Roll %d: %s. /t need %d, /t greed %d or /t pass %d within %d seconds.", r.id, item, r.id, r.id, r.id, secs)
	}
	return fmt.Sprintf("Roll %d: %s (you cannot use it). /t greed %d or /t pass %d within %d seconds.", r.id, item, r.id, r.id, secs)
}

// rollOfferLinked is rollOfferLine with the item linked (lootlinks.go).
func (h *NPCWindowHandler) rollOfferLinked(r *lootRoll, s *rollSeat, now time.Time) *serverLine {
	item, secs := lootItem(r.item.rec, 0), rollSecondsLeft(r, now)
	if s.mayNeed {
		return h.linkedf(s.Name, "Roll %d: %s. /t need %d, /t greed %d or /t pass %d within %d seconds.", r.id, item, r.id, r.id, r.id, secs)
	}
	return h.linkedf(s.Name, "Roll %d: %s (you cannot use it). /t greed %d or /t pass %d within %d seconds.", r.id, item, r.id, r.id, secs)
}

// closeRollFor closes a seat's window on a roll (a seat with no windows has nothing to
// close). Must be called with ls.mu held.
func (h *NPCWindowHandler) closeRollFor(r *lootRoll, s *rollSeat, now time.Time) {
	s.closed = true
	if sess, _ := h.raidSeatOf(s.ServerID); sess != nil && h.raidWindowsOf(sess) {
		queueReliable(sess, protocol.OpcodeRaidRoll, rollPayload(r, s, now, false))
	}
}

// raidSayLocked tells a raid's members and the given seats (its claimants) a line in the
// raid's orange. Must be called with ls.mu held.
func (h *NPCWindowHandler) raidSayLocked(rc *raidCorpse, seats []*rollSeat, line *serverLine) {
	told := map[*session.Session]bool{}
	say := func(s *session.Session) {
		if s != nil && !told[s] {
			told[s] = true
			line.colored(s, raidR, raidG, raidB)
		}
	}
	if h.raid.sessions != nil {
		for _, s := range h.raid.sessions(rc.group) {
			say(s)
		}
	}
	for _, m := range rc.members {
		if m.Claim {
			s, _ := h.raidSeatOf(m.ServerID)
			say(s)
		}
	}
	for _, st := range seats {
		s, _ := h.raidSeatOf(st.ServerID)
		say(s)
	}
	log.Printf("Raid loot: group %d: %s", rc.group, line.text)
}

// corpseRaidLocked is the raid hold of a roll's corpse, if it still lies. Must be called
// with ls.mu held.
func (ls *lootState) corpseRaidLocked(r *lootRoll) (*corpse, *raidCorpse) {
	c := ls.corpses[r.corpse]
	if c == nil || !c.died.Equal(r.died) || c.raid == nil {
		return nil, nil
	}
	return c, c.raid
}

// rollOfLocked is the roll id of character char's seat, or with id 0 the oldest roll it
// has not chosen on. Must be called with ls.mu held.
func (ls *lootState) rollOfLocked(id int, char uint32) (*lootRoll, *rollSeat) {
	if id != 0 {
		if r := ls.rolls[id]; r != nil {
			for _, s := range r.seats {
				if s.ServerID == char {
					return r, s
				}
			}
		}
		return nil, nil
	}
	var best *lootRoll
	var seat *rollSeat
	for _, r := range ls.rolls {
		for _, s := range r.seats {
			if s.ServerID == char && s.choice == rollNone && (best == nil || r.ends.Before(best.ends) || r.ends.Equal(best.ends) && r.id < best.id) {
				best, seat = r, s
			}
		}
	}
	return best, seat
}

// winTake is a won item put in a pack, for the quests after the lock.
type winTake struct {
	sess *session.Session
	p    *entity.Player
	take lootTake
}

// tellWinsQuests tells the quests the won items taken. Called with no loot lock held.
func tellWinsQuests(won []winTake) {
	for _, w := range won {
		tellQuestsLooted(w.sess, w.p, []lootTake{w.take})
	}
}

// RaidRollChoice is a seat's choice on a roll: its client's 0x7E46, or its typed /need,
// /greed or /pass (id 0: the oldest roll it has not chosen on).
func (h *NPCWindowHandler) RaidRollChoice(sess *session.Session, id int, choice rollChoice) {
	var won []winTake
	defer func() { tellWinsQuests(won) }() // after the lock
	ls := h.loots()
	ls.mu.Lock()
	defer ls.mu.Unlock()
	won = h.rollChoiceLocked(ls, sess, id, choice, h.raidClock())
}

func (h *NPCWindowHandler) rollChoiceLocked(ls *lootState, sess *session.Session, id int, choice rollChoice, now time.Time) []winTake {
	if choice < rollNeed || choice > rollPass {
		return nil
	}
	char := h.charOf(sess)
	r, s := ls.rollOfLocked(id, char)
	switch {
	case r == nil && id == 0:
		SendColoredChat(sess, "You have no loot roll to answer.", 255, 63, 63)
		return nil
	case r == nil:
		SendColoredChat(sess, fmt.Sprintf("Roll %d is over, or not yours.", id), 255, 63, 63)
		return nil
	case s.choice != rollNone:
		SendColoredChat(sess, fmt.Sprintf("You chose %s on roll %d already.", s.choice, r.id), 255, 63, 63)
		return nil
	case choice == rollNeed && !s.mayNeed:
		SendColoredChat(sess, fmt.Sprintf("You cannot use %s: Greed or Pass.", r.item.rec.Pattern.ItemName), 255, 63, 63)
		return nil
	}
	_, rc := ls.corpseRaidLocked(r)
	item := r.item.rec.Pattern.ItemName
	linked := lootItem(r.item.rec, 0)
	s.choice = choice
	if choice == rollPass {
		h.linkedf(s.Name, "You pass on %s.", linked).colored(sess, raidR, raidG, raidB)
	} else {
		s.roll = h.raidIntn(raidRollTop + 1)
		if rc != nil {
			h.raidSayLocked(rc, r.seats, h.linkedf(s.Name, "%s rolls %s on %s: %d.", s.Name, choice, linked, s.roll))
		}
	}
	log.Printf("Raid loot: %s chooses %s on roll %d (%s): %d", s.Name, choice, r.id, item, s.roll)
	h.closeRollFor(r, s, now)
	for _, o := range r.seats {
		if o.choice == rollNone {
			return nil
		}
	}
	return h.endRollLocked(ls, r, now)
}

// decideLocked is a roll's winner (the file's doc), a tie rolled again and said; nil when
// no one rolled. Must be called with ls.mu held.
func (h *NPCWindowHandler) decideLocked(r *lootRoll, rc *raidCorpse) *rollSeat {
	for _, tier := range []rollChoice{rollNeed, rollGreed} {
		var top []*rollSeat
		best := -1
		for _, s := range r.seats {
			switch {
			case s.choice != tier:
			case s.roll > best:
				best, top = s.roll, []*rollSeat{s}
			case s.roll == best:
				top = append(top, s)
			}
		}
		for len(top) > 1 {
			var names, parts []string
			was := best
			next, best := []*rollSeat(nil), -1
			for _, s := range top {
				s.roll = h.raidIntn(raidRollTop + 1)
				names = append(names, s.Name)
				parts = append(parts, fmt.Sprintf("%s %d", s.Name, s.roll))
				switch {
				case s.roll > best:
					best, next = s.roll, []*rollSeat{s}
				case s.roll == best:
					next = append(next, s)
				}
			}
			if rc != nil {
				h.raidSayLocked(rc, r.seats, h.linkedf(rc.from, "A tie at %s %d on %s between %s: again, %s.", tier, was,
					lootItem(r.item.rec, 0), strings.Join(names, " and "), strings.Join(parts, ", ")))
			}
			top = next
		}
		if len(top) == 1 {
			return top[0]
		}
	}
	return nil
}

// endRollLocked decides a roll and gives its item (the file's doc). Must be called with
// ls.mu held.
func (h *NPCWindowHandler) endRollLocked(ls *lootState, r *lootRoll, now time.Time) []winTake {
	delete(ls.rolls, r.id)
	for _, s := range r.seats {
		if !s.closed {
			h.closeRollFor(r, s, now)
		}
	}
	hi := r.item
	hi.roll = nil
	c, rc := ls.corpseRaidLocked(r)
	if c == nil {
		return nil // the corpse went with it
	}
	item := lootItem(hi.rec, 0)
	win := h.decideLocked(r, rc)
	if win == nil {
		rc.drop(hi)
		c.items = append(c.items, hi.rec)
		h.raidSayLocked(rc, r.seats, h.linkedf(rc.from, "No one rolled on %s: it lies on %s's corpse for anyone.", item, rc.from))
		return nil
	}
	hi.winner, hi.winnerName, hi.until = win.ServerID, win.Name, now.Add(raidHoldFor)
	h.raidSayLocked(rc, r.seats, h.linkedf(win.Name, "%s wins %s with %s %d.", win.Name, item, win.choice, win.roll))
	took, why := h.giveWonLocked(c, hi)
	if took != nil {
		return []winTake{*took}
	}
	h.holdCorpseFor(r.corpse, r.died, hi.until)
	if s, _ := h.raidSeatOf(win.ServerID); s != nil {
		h.wonWaitsLinked(win.Name, hi.rec, rc.from, why, raidHoldFor).colored(s, raidR, raidG, raidB)
	}
	h.raidSayLocked(rc, nil, h.linkedf(win.Name, "%s waits on %s's corpse for %s for %s.", item, rc.from, win.Name, spanWords(raidHoldFor)))
	return nil
}

// wonWaitsLine is a winner's line for an item it could not take now.
func wonWaitsLine(item, from, why string, left time.Duration) string {
	if why == "lore" {
		return fmt.Sprintf("You won %s, but you hold one already (Lore). It waits on %s's corpse for you for %s.", item, from, spanWords(left))
	}
	return fmt.Sprintf("You won %s, but your pack is full. Make room and open %s's corpse within %s to take it.", item, from, spanWords(left))
}

// wonWaitsLinked is wonWaitsLine with the item linked (lootlinks.go).
func (h *NPCWindowHandler) wonWaitsLinked(who string, rec database.CharInventoryEntry, from, why string, left time.Duration) *serverLine {
	if why == "lore" {
		return h.linkedf(who, "You won %s, but you hold one already (Lore). It waits on %s's corpse for you for %s.", lootItem(rec, 0), from, spanWords(left))
	}
	return h.linkedf(who, "You won %s, but your pack is full. Make room and open %s's corpse within %s to take it.", lootItem(rec, 0), from, spanWords(left))
}

// drop takes hi out of the hold.
func (rc *raidCorpse) drop(hi *heldItem) {
	for i, x := range rc.held {
		if x == hi {
			rc.held = append(rc.held[:i], rc.held[i+1:]...)
			return
		}
	}
}

// giveWonLocked puts a won item in its winner's pack when the winner is in the world and
// has room: the take, or nil and why not ("away", "full", "lore"). Must be called with
// ls.mu held.
func (h *NPCWindowHandler) giveWonLocked(c *corpse, hi *heldItem) (*winTake, string) {
	sess, p := h.raidSeatOf(hi.winner)
	inv := sessionInvList(sess)
	if sess == nil || p == nil || inv == nil || h.inv == nil {
		return nil, "away"
	}
	rec := hi.rec
	room := inv.wouldAdd(rec.Pattern.PatternID, rec.StackLeft, rec.Pattern.MaxStack)
	took := inv.loreRoom(&rec.Pattern, room)
	switch {
	case took == 0 && room > 0:
		return nil, "lore"
	case took < rec.StackLeft:
		return nil, "full"
	}
	out, key, err := h.giveItem(p, inv, rec, took, 0)
	if err != nil {
		log.Printf("Raid loot: %s's %q: %v", p.Name, rec.Pattern.ItemName, err)
		return nil, "full"
	}
	queueReliable(sess, protocol.OpcodeAddInvItem, addInvItemPayload(out, key, readsCapes(sess)))
	h.tellLooted(sess, p.Name, rec, took)
	c.raid.drop(hi)
	log.Printf("Raid loot: %s takes %q (won)", p.Name, rec.Pattern.ItemName)
	return &winTake{sess: sess, p: p, take: lootTake{rec.Pattern.PatternID, took}}, ""
}

// raidOpenLocked is a raid's rules at sess's opening of npc's corpse c, before any window:
// its won items into the opener's pack (true: no window opens, petition 1448's "This
// should not actually open the corpse loot window"), a master looter's corpse refused to
// anyone else, or one holding nothing for the opener but what is rolled on or waits for
// others refused. False: it opens as any corpse does. Must be called with ls.mu held.
func (h *NPCWindowHandler) raidOpenLocked(sess *session.Session, ls *lootState, npc *entity.NPC, c *corpse) (bool, []winTake) {
	rc := c.raid
	if rc == nil {
		return false, nil
	}
	now := h.raidClock()
	char := h.charOf(sess)
	var mine []*heldItem
	for _, hi := range rc.held {
		if hi.roll == nil && hi.winner == char && char != 0 && now.Before(hi.until) {
			mine = append(mine, hi)
		}
	}
	if len(mine) > 0 {
		var won []winTake
		for _, hi := range mine {
			if took, why := h.giveWonLocked(c, hi); took != nil {
				won = append(won, *took)
			} else if why == "lore" {
				SendClientMessage(sess, fmt.Sprintf(loreLine, hi.rec.Pattern.ItemName))
			} else {
				SendClientMessage(sess, lootNoRoom)
				h.wonWaitsLinked(sessName(h, sess), hi.rec, rc.from, why, hi.until.Sub(now)).colored(sess, raidR, raidG, raidB)
			}
		}
		log.Printf("Raid loot: %s claims %d won items on %q's corpse, %d taken", sessName(h, sess), len(mine), npc.Name, len(won))
		return true, won
	}
	if rc.master != 0 && rc.master != char && now.Before(rc.until) {
		queueReliable(sess, protocol.OpcodeLootOptions, buildLootOption(lootAnotherOwner))
		SendColoredChat(sess, fmt.Sprintf("%s's loot is %s's, the raid's master looter, for %s more.",
			rc.from, rc.masterName, spanWords(rc.until.Sub(now))), 255, 63, 63)
		return true, nil
	}
	if len(c.items) == 0 && len(ls.dropsLocked(npc.ServerID, c, char)) == 0 && len(rc.held) > 0 {
		queueReliable(sess, protocol.OpcodeLootOptions, buildLootOption(lootAnotherOwner))
		SendColoredChat(sess, fmt.Sprintf("%s's loot is being rolled on, or waits for its winners.", rc.from), 255, 63, 63)
		return true, nil
	}
	return false, nil
}

// raidOpenedLocked tells a master looter who opened its corpse what it may do: its
// client's list (0x7E47), or the items by number in chat. Must be called with ls.mu held.
func (h *NPCWindowHandler) raidOpenedLocked(sess *session.Session, ls *lootState, npc *entity.NPC, c *corpse) {
	rc := c.raid
	now := h.raidClock()
	char := h.charOf(sess)
	if rc == nil || rc.master == 0 || rc.master != char || !now.Before(rc.until) {
		return
	}
	if ls.listed == nil {
		ls.listed = map[uint32]entity.EntityID{}
	}
	ls.listed[char] = npc.ServerID
	if h.raidWindowsOf(sess) {
		queueReliable(sess, protocol.OpcodeRaidMasterLoot, h.masterLootPayload(sess, rc, now))
		return
	}
	h.listMasterLootLocked(sess, c, now)
}

// masterListLocked sends a master looter's client its list again after a resent corpse
// (resendCorpse): vanilla forgets it at each 0x0016, as CLIENT forgets the corpse, so it
// follows every 0x0018 of the corpse to its master looter. A client with no windows has
// its numbers at the opening and after each assign. Must be called with ls.mu held.
func (h *NPCWindowHandler) masterListLocked(sess *session.Session, c *corpse) {
	rc := c.raid
	now := h.raidClock()
	if rc == nil || rc.master == 0 || rc.master != h.charOf(sess) || !now.Before(rc.until) || !h.raidWindowsOf(sess) {
		return
	}
	queueReliable(sess, protocol.OpcodeRaidMasterLoot, h.masterLootPayload(sess, rc, now))
}

// listMasterLootLocked tells a master looter with no windows its corpse's items by
// number, and how to give them. Must be called with ls.mu held.
func (h *NPCWindowHandler) listMasterLootLocked(sess *session.Session, c *corpse, now time.Time) {
	if len(c.items) == 0 {
		SendColoredChat(sess, fmt.Sprintf("Master looter: nothing is left on %s's corpse.", c.raid.from), raidR, raidG, raidB)
		return
	}
	format := "Master looter, %s left: "
	args := []any{spanWords(c.raid.until.Sub(now))}
	for i, it := range c.items {
		if i > 0 {
			format += ", "
		}
		if it.StackLeft > 1 { // lootStackName's
			format += "%d %d %s"
			args = append(args, i+1, it.StackLeft, lootItem(it, 0))
		} else {
			format += "%d %s"
			args = append(args, i+1, lootItem(it, 0))
		}
	}
	h.linkedf(c.raid.from, format+".", args...).colored(sess, raidR, raidG, raidB)
	SendColoredChat(sess, "Take as usual, or /t assign <number> <name>, or /t abandon to let anyone loot it all.", raidR, raidG, raidB)
}

// lootStackName is a record's name with its count when more than one.
func lootStackName(e database.CharInventoryEntry) string {
	if e.StackLeft > 1 {
		return fmt.Sprintf("%d %s", e.StackLeft, e.Pattern.ItemName)
	}
	return e.Pattern.ItemName
}

// lootStackNamed is linkedf(format, a, lootStackName(rec), b) with the item linked: the
// count, when more than one, before it and outside the link.
func (h *NPCWindowHandler) lootStackNamed(who, format, a string, rec database.CharInventoryEntry, b string) *serverLine {
	if rec.StackLeft > 1 {
		return h.linkedf(who, format, a, linkedCount{rec.StackLeft, lootItem(rec, 0)}, b)
	}
	return h.linkedf(who, format, a, lootItem(rec, 0), b)
}

// masterCandidate is one the master looter may give to.
type masterCandidate struct {
	char uint32
	name string
	p    *entity.Player
	sess *session.Session
}

// masterCandidatesLocked is everyone a master looter may give to, in the world now: the
// raid's members at the kill and since, and those kicked in the fight. Must be called
// with ls.mu held.
func (h *NPCWindowHandler) masterCandidatesLocked(rc *raidCorpse) []masterCandidate {
	var out []masterCandidate
	seen := map[uint32]bool{}
	add := func(char uint32) {
		if char == 0 || seen[char] {
			return
		}
		seen[char] = true
		if s, p := h.raidSeatOf(char); s != nil && p != nil {
			out = append(out, masterCandidate{char: char, name: p.Name, p: p, sess: s})
		}
	}
	add(rc.master)
	for _, m := range rc.members {
		add(m.ServerID)
	}
	if h.raid.sessions != nil && h.playerOf != nil {
		for _, s := range h.raid.sessions(rc.group) {
			if p := h.playerOf(s); p != nil {
				add(p.CharServerID)
			}
		}
	}
	return out
}

// masterLootPayload is 0x7E47 to the master looter (protocol.OpcodeRaidMasterLoot): its
// time left and whom it may give to, by the ids its client knows them by.
func (h *NPCWindowHandler) masterLootPayload(sess *session.Session, rc *raidCorpse, now time.Time) []byte {
	cands := h.masterCandidatesLocked(rc)
	if len(cands) > 255 {
		cands = cands[:255]
	}
	return build(protocol.OpcodeRaidMasterLoot, 16+len(cands)*28, func(w *buffer.Writer) {
		w.WriteByte(1)
		w.WriteUint16(uint16(min(math.MaxUint16, int(math.Ceil(rc.until.Sub(now).Seconds())))))
		w.WriteByte(byte(len(cands)))
		for _, m := range cands {
			id := uint32(0)
			if h.raid.seen != nil {
				id = h.raid.seen(sess, m.p)
			}
			w.WriteUint32(id)
			name := []byte(m.name)
			if len(name) > 23 {
				name = name[:23]
			}
			w.WriteByte(byte(len(name)))
			w.WriteBytes(name)
		}
	})
}

// masterLootOffPayload is 0x7E47's end of a hold: a u8, 0.
func masterLootOffPayload() []byte {
	return build(protocol.OpcodeRaidMasterLoot, 3, func(w *buffer.Writer) { w.WriteByte(0) })
}

// masterCorpseLocked is the corpse sess's player holds as master looter: the one open,
// else the one it was last listed (a PS2 types with its window closed). Must be called
// with ls.mu held.
func (h *NPCWindowHandler) masterCorpseLocked(sess *session.Session, ls *lootState, now time.Time) (entity.EntityID, *corpse) {
	char := h.charOf(sess)
	held := func(id entity.EntityID) *corpse {
		c := ls.corpses[id]
		if c == nil || c.raid == nil || c.raid.master != char || char == 0 || !now.Before(c.raid.until) {
			return nil
		}
		if h.corpseLies != nil {
			if died, lies := h.corpseLies(id); !lies || !died.Equal(c.died) {
				return nil
			}
		}
		return c
	}
	if id, ok := ls.open[sess]; ok {
		if c := held(id); c != nil {
			return id, c
		}
	}
	if id, ok := ls.listed[char]; ok {
		if c := held(id); c != nil {
			return id, c
		}
	}
	return 0, nil
}

// MasterAssign is a master looter's Assign: the corpse's record at key (its window's, or
// a PS2's number less one) to the player its client knows by id, or named. The item goes
// into that one's pack, or stays when it has no room.
func (h *NPCWindowHandler) MasterAssign(sess *session.Session, key int, id uint32, name string) {
	var won []winTake
	defer func() { tellWinsQuests(won) }() // after the lock
	ls := h.loots()
	ls.mu.Lock()
	defer ls.mu.Unlock()
	now := h.raidClock()
	cid, c := h.masterCorpseLocked(sess, ls, now)
	if c == nil {
		SendColoredChat(sess, "You hold no corpse as master looter.", 255, 63, 63)
		return
	}
	rc := c.raid
	if key < 0 || key >= len(c.items) {
		SendColoredChat(sess, fmt.Sprintf("There is no item %d on %s's corpse.", key+1, rc.from), 255, 63, 63)
		return
	}
	var to *masterCandidate
	cands := h.masterCandidatesLocked(rc)
	var want *entity.Player
	if name == "" && h.raid.seenBy != nil {
		want = h.raid.seenBy(sess, id)
	}
	for i := range cands {
		if name != "" && strings.EqualFold(cands[i].name, strings.TrimSpace(name)) || want != nil && cands[i].char == want.CharServerID {
			to = &cands[i]
			break
		}
	}
	if to == nil {
		who := name
		if who == "" {
			who = "That player"
		}
		SendColoredChat(sess, fmt.Sprintf("%s is not in your raid, or not in the world.", who), 255, 63, 63)
		return
	}
	rec := c.items[key]
	inv := sessionInvList(to.sess)
	if inv == nil || h.inv == nil {
		SendColoredChat(sess, fmt.Sprintf("%s cannot take items now.", to.name), 255, 63, 63)
		return
	}
	room := inv.wouldAdd(rec.Pattern.PatternID, rec.StackLeft, rec.Pattern.MaxStack)
	took := inv.loreRoom(&rec.Pattern, room)
	switch {
	case took == 0 && room > 0:
		SendColoredChat(sess, fmt.Sprintf("%s holds a %s already (Lore).", to.name, rec.Pattern.ItemName), 255, 63, 63)
		return
	case took < rec.StackLeft:
		SendColoredChat(sess, fmt.Sprintf("%s has no room for %s.", to.name, lootStackName(rec)), 255, 63, 63)
		return
	}
	out, k, err := h.giveItem(to.p, inv, rec, took, 0)
	if err != nil {
		log.Printf("Raid loot: assigning %q to %s: %v", rec.Pattern.ItemName, to.name, err)
		return
	}
	queueReliable(to.sess, protocol.OpcodeAddInvItem, addInvItemPayload(out, k, readsCapes(to.sess)))
	h.tellLooted(to.sess, to.name, rec, took)
	ls.tookLocked(cid, c, h.charOf(sess), key, took)
	won = append(won, winTake{sess: to.sess, p: to.p, take: lootTake{rec.Pattern.PatternID, took}})
	h.raidSayLocked(rc, nil, h.lootStackNamed(to.name, "%s assigns %s to %s.", rc.masterName, rec, to.name))
	if open, ok := ls.open[sess]; ok && open == cid {
		h.resendCorpse(sess, ls, c) // and its 0x7E47 (masterListLocked)
		return
	}
	h.listMasterLootLocked(sess, c, now)
}

// MasterAbandon is a master looter's Abandon: the corpse anyone's, everything on it.
func (h *NPCWindowHandler) MasterAbandon(sess *session.Session) {
	ls := h.loots()
	ls.mu.Lock()
	defer ls.mu.Unlock()
	now := h.raidClock()
	_, c := h.masterCorpseLocked(sess, ls, now)
	if c == nil {
		SendColoredChat(sess, "You hold no corpse as master looter.", 255, 63, 63)
		return
	}
	rc := c.raid
	rc.master = 0
	if h.raidWindowsOf(sess) {
		queueReliable(sess, protocol.OpcodeRaidMasterLoot, masterLootOffPayload())
	}
	h.raidSayLocked(rc, nil, plainLine(fmt.Sprintf("%s abandons %s's loot: anyone may loot anything on it.", rc.masterName, rc.from)))
}

// RaidTick ends the rolls whose time is up, lets the won items whose winners did not take
// them and the master looters' holds that are over go to anyone. Once a second (the
// worker).
func (h *NPCWindowHandler) RaidTick(now time.Time) {
	var won []winTake
	defer func() { tellWinsQuests(won) }() // after the lock
	ls := h.loots()
	ls.mu.Lock()
	defer ls.mu.Unlock()
	var due []*lootRoll
	for _, r := range ls.rolls {
		if !now.Before(r.ends) {
			due = append(due, r)
		}
	}
	sort.Slice(due, func(i, j int) bool { return due[i].id < due[j].id })
	for _, r := range due {
		won = append(won, h.endRollLocked(ls, r, now)...)
	}
	for _, c := range ls.corpses {
		rc := c.raid
		if rc == nil {
			continue
		}
		if rc.master != 0 && !now.Before(rc.until) {
			if s, _ := h.raidSeatOf(rc.master); s != nil && h.raidWindowsOf(s) {
				queueReliable(s, protocol.OpcodeRaidMasterLoot, masterLootOffPayload())
			}
			rc.master = 0
			h.raidSayLocked(rc, nil, plainLine(fmt.Sprintf("The master looter's hold on %s's corpse is over: anyone may loot it.", rc.from)))
		}
		for _, hi := range append([]*heldItem(nil), rc.held...) {
			if hi.roll != nil || now.Before(hi.until) {
				continue
			}
			rc.drop(hi)
			c.items = append(c.items, hi.rec)
			if s, _ := h.raidSeatOf(hi.winner); s != nil {
				h.linkedf(hi.winnerName, "Your claim on %s is over: it lies on %s's corpse for anyone.", lootItem(hi.rec, 0), rc.from).colored(s, raidR, raidG, raidB)
			}
			log.Printf("Raid loot: %s's claim on %q (%s's corpse) is over", hi.winnerName, hi.rec.Pattern.ItemName, rc.from)
		}
	}
}

// raidCorpseGoneLocked ends the rolls on a corpse that went (corpseGoneLocked). Must be
// called with ls.mu held.
func (h *NPCWindowHandler) raidCorpseGoneLocked(ls *lootState, id entity.EntityID) {
	now := h.raidClock()
	for rid, r := range ls.rolls {
		if r.corpse != id {
			continue
		}
		delete(ls.rolls, rid)
		for _, s := range r.seats {
			if !s.closed {
				h.closeRollFor(r, s, now)
			}
		}
	}
}

// HasRoll is whether sess's player has a roll to answer: roll n's seat, or with n 0 any
// roll it has not chosen on (a PS2's "/t need" is the command only then: raidcmds.go).
func (h *NPCWindowHandler) HasRoll(sess *session.Session, n int) bool {
	if n < 0 {
		return false
	}
	char := h.charOf(sess)
	ls := h.loots()
	ls.mu.Lock()
	defer ls.mu.Unlock()
	r, _ := ls.rollOfLocked(n, char)
	return r != nil && char != 0
}

// HoldsMasterCorpse is whether sess's player holds a corpse as master looter.
func (h *NPCWindowHandler) HoldsMasterCorpse(sess *session.Session) bool {
	ls := h.loots()
	ls.mu.Lock()
	defer ls.mu.Unlock()
	char := h.charOf(sess)
	now := h.raidClock()
	if _, c := h.masterCorpseLocked(sess, ls, now); c != nil {
		return true
	}
	for _, c := range ls.corpses {
		if c.raid != nil && c.raid.master == char && char != 0 && now.Before(c.raid.until) {
			return true
		}
	}
	return false
}

// KeptRaidCorpse is a corpse's raid hold as a restart carries it (keep.go's KeptCorpse):
// its rolls going on with their seats' choices, its items waiting for their winners and
// its master looter's hold, each time as what was left of it.
type KeptRaidCorpse struct {
	Rule       entity.LootRule `json:"rule"`
	Group      uint32          `json:"group"`
	From       string          `json:"from"`
	Master     uint32          `json:"master,omitempty"`
	MasterName string          `json:"master_name,omitempty"`
	MasterLeft int64           `json:"master_left_ms,omitempty"`
	Members    []KeptRaidSeat  `json:"members,omitempty"`
	Held       []KeptHeldItem  `json:"held,omitempty"`
}

// KeptRaidSeat is one seat, by its character.
type KeptRaidSeat struct {
	Char    uint32 `json:"char"`
	Name    string `json:"name"`
	Class   int    `json:"class"`
	Race    int    `json:"race"`
	Level   int    `json:"level"`
	Claim   bool   `json:"claim,omitempty"`
	MayNeed bool   `json:"may_need,omitempty"`
	Choice  byte   `json:"choice,omitempty"`
	Roll    int    `json:"roll,omitempty"`
	Closed  bool   `json:"closed,omitempty"`
}

// KeptHeldItem is an item held off the corpse: rolled on (its roll's number, time and
// seats), or won (its winner and time).
type KeptHeldItem struct {
	Item       database.CharInventoryEntry `json:"item"`
	RollID     int                         `json:"roll_id,omitempty"`
	RollLeft   int64                       `json:"roll_left_ms,omitempty"`
	Seats      []KeptRaidSeat              `json:"seats,omitempty"`
	Winner     uint32                      `json:"winner,omitempty"`
	WinnerName string                      `json:"winner_name,omitempty"`
	Left       int64                       `json:"left_ms,omitempty"`
}

func keptSeat(s entity.RaidSeat) KeptRaidSeat {
	return KeptRaidSeat{Char: s.ServerID, Name: s.Name, Class: s.Class, Race: s.Race, Level: s.Level, Claim: s.Claim}
}

func (k KeptRaidSeat) seat() entity.RaidSeat {
	return entity.RaidSeat{ServerID: k.Char, Name: k.Name, Class: k.Class, Race: k.Race, Level: k.Level, Claim: k.Claim}
}

// keepRaid is c's raid hold to keep, nil for none. Must be called with ls.mu held.
func keepRaid(ls *lootState, c *corpse, now time.Time) *KeptRaidCorpse {
	rc := c.raid
	if rc == nil {
		return nil
	}
	k := &KeptRaidCorpse{Rule: rc.rule, Group: rc.group, From: rc.from, Master: rc.master, MasterName: rc.masterName}
	if rc.master != 0 {
		k.MasterLeft = max(0, rc.until.Sub(now).Milliseconds())
	}
	for _, m := range rc.members {
		k.Members = append(k.Members, keptSeat(m))
	}
	for _, hi := range rc.held {
		kh := KeptHeldItem{Item: hi.rec, Winner: hi.winner, WinnerName: hi.winnerName}
		if r := hi.roll; r != nil {
			kh.RollID, kh.RollLeft = r.id, max(0, r.ends.Sub(now).Milliseconds())
			for _, s := range r.seats {
				ks := keptSeat(s.RaidSeat)
				ks.MayNeed, ks.Choice, ks.Roll, ks.Closed = s.mayNeed, byte(s.choice), s.roll, s.closed
				kh.Seats = append(kh.Seats, ks)
			}
		} else {
			kh.Left = max(0, hi.until.Sub(now).Milliseconds())
		}
		k.Held = append(k.Held, kh)
	}
	return k
}

// restoreRaid puts a kept raid hold back on corpse c of creature id, its rolls under their
// numbers, and reports whether there was one. Must be called with ls.mu held.
func restoreRaid(ls *lootState, id entity.EntityID, c *corpse, k *KeptRaidCorpse, now time.Time) bool {
	if k == nil {
		return false
	}
	rc := &raidCorpse{rule: k.Rule, group: k.Group, from: k.From, master: k.Master, masterName: k.MasterName,
		until: now.Add(time.Duration(k.MasterLeft) * time.Millisecond)}
	for _, m := range k.Members {
		rc.members = append(rc.members, m.seat())
	}
	c.raid = rc
	for _, kh := range k.Held {
		hi := &heldItem{rec: kh.Item, winner: kh.Winner, winnerName: kh.WinnerName,
			until: now.Add(time.Duration(kh.Left) * time.Millisecond)}
		if kh.RollID != 0 && ls.rolls[kh.RollID] == nil {
			r := &lootRoll{id: kh.RollID, corpse: id, died: c.died, group: k.Group, item: hi,
				ends: now.Add(time.Duration(kh.RollLeft) * time.Millisecond)}
			for _, ks := range kh.Seats {
				r.seats = append(r.seats, &rollSeat{RaidSeat: ks.seat(), mayNeed: ks.MayNeed, choice: rollChoice(ks.Choice), roll: ks.Roll, closed: ks.Closed})
			}
			hi.roll = r
			ls.rolls[r.id] = r
			ls.nextRoll = max(ls.nextRoll, r.id)
		} else if kh.RollID != 0 {
			c.items = append(c.items, kh.Item) // its number taken: anyone's
			continue
		}
		rc.held = append(rc.held, hi)
	}
	return true
}
