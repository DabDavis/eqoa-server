package handlers

// keep.go — the handlers' part of a session carried through a restart of the
// server process (claude-notes/session-keep-restart-20261106.md;
// session/keep.go, entity/keep.go and network/udp/keep.go have the rest).
//
// A character comes back as a login builds it (enterFromRecord), from the row
// the shutdown's save wrote; what the handlers keep of a session beside it is
// carried here: the server's copies of the client's item lists (which record
// each key the client sends names, until the dump below sends them anew), the
// auction's live forms and the quest marks and tracker the client asked for.
// Then, once the client has reported where it stands, it is sent a
// MemoryDump there, as a respawn sends one (CLIENT's Msg000D brings the
// client's copy of the character in line with this process's: its lists, its
// journal, its abilities; it keeps its scene and its world), with the run
// speed its effects give, its lockouts and, for a GM, its channel. A character
// that was dead respawns, as it would have.

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"sync/atomic"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// keepChars is the character handler RegisterDefaultHandlers made, which a
// resume builds characters and sends dumps through; keepWindows its NPC window
// handler, which holds what the corpses hold.
var (
	keepChars   atomic.Pointer[CharacterHandler]
	keepWindows atomic.Pointer[NPCWindowHandler]
)

// KeptExtras is what a restart carries of the handlers' own state for a
// session.
type KeptExtras struct {
	Inventory     *KeptList `json:"inventory,omitempty"`
	Bank          *KeptList `json:"bank,omitempty"`
	AuctionForm   bool      `json:"auction_form,omitempty"`
	QuestMarks    bool      `json:"quest_marks,omitempty"`
	QuestProgress bool      `json:"quest_progress,omitempty"`
	// QuestHeadMarks is whether the client asked for the marks over the
	// NPCs' heads (questheadmarks.go)
	QuestHeadMarks bool `json:"quest_head_marks,omitempty"`
	// LootOpen: its client has a corpse's window open (ResumeCharacter closes it)
	LootOpen bool `json:"loot_open,omitempty"`
}

// KeptList is an item list's copy (itemlist.go's invList); Cap the records
// the client's list holds (0, or a file from before it: CLIENT's 40).
type KeptList struct {
	Recs []KeptListRec `json:"recs"`
	Next uint32        `json:"next"`
	Cap  int           `json:"cap,omitempty"`
	// Kept: the items of the rows kept beside the list (invList.kept)
	Kept []int `json:"kept,omitempty"`
}

// KeptListRec is one of a KeptList's records.
type KeptListRec struct {
	Key     uint32 `json:"key"`
	Item    int    `json:"item"`
	Pattern int    `json:"pattern"`
	Qty     int    `json:"qty"`
}

func keptList(l *invList) *KeptList {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	k := &KeptList{Next: l.next, Recs: make([]KeptListRec, 0, len(l.recs)), Cap: l.cap}
	for _, r := range l.recs {
		k.Recs = append(k.Recs, KeptListRec{Key: r.key, Item: r.itemID, Pattern: r.patternID, Qty: r.qty})
	}
	for id := range l.kept {
		k.Kept = append(k.Kept, id)
	}
	sort.Ints(k.Kept)
	return k
}

func (k *KeptList) list() *invList {
	if k == nil {
		return nil
	}
	l := &invList{next: k.Next, recs: make([]invRec, 0, len(k.Recs)), cap: k.Cap}
	for _, r := range k.Recs {
		l.recs = append(l.recs, invRec{key: r.Key, itemID: r.Item, patternID: r.Pattern, qty: r.Qty})
	}
	if len(k.Kept) > 0 {
		l.kept = make(map[int]bool, len(k.Kept))
		for _, id := range k.Kept {
			l.kept[id] = true
		}
	}
	return l
}

// KeepExtras is what the handlers keep of sess; p is its player (nil before
// one).
func KeepExtras(sess *session.Session, p *entity.Player) KeptExtras {
	k := KeptExtras{Inventory: keptList(sessionInvList(sess)), Bank: keptList(sessionBankList(sess))}
	if h := keepChars.Load(); h != nil && h.auction != nil {
		h.auction.formMu.Lock()
		k.AuctionForm = h.auction.form[sess]
		h.auction.formMu.Unlock()
	}
	if q := Quests(); q != nil && p != nil {
		k.QuestMarks, k.QuestProgress = q.keptFlags(p)
		k.QuestHeadMarks = q.headMarksOn(p)
	}
	if h := keepWindows.Load(); h != nil {
		ls := h.loots()
		ls.mu.Lock()
		_, k.LootOpen = ls.open[sess]
		ls.mu.Unlock()
	}
	return k
}

// keptFlags is whether p's client asked for the quest marks and the tracker.
func (q *QuestEngine) keptFlags(p *entity.Player) (marks, progress bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	c := q.namedLocked(p.CharServerID, p.Name)
	return c.marksOn, c.progressOn
}

// KeptCorpse is what a corpse holds as a restart carries it, for the creature the
// World carries dead (entity.KeptNPC). A corpse no one has opened has nothing to
// keep: its contents are rolled when it first is, in whichever process. One that
// has been must be kept, even with nothing left on it: forgotten, its next opener
// would roll it all again, coins too (they went to the first opener: a corpse
// here holds none). A loot step's drops lie on it from the kill, opened or not.
// A window open on one is closed at the resume: the dump tells its client it holds
// no corpse (CLIENT's HaveCorpse, Msg000D) and the window would stand there saying
// "Empty" over a corpse that is not; the client opens it again.
type KeptCorpse struct {
	ID     uint32                                   `json:"id"`
	Died   time.Time                                `json:"died"`
	Opened bool                                     `json:"opened,omitempty"`
	Items  []database.CharInventoryEntry            `json:"items,omitempty"`
	Drops  map[uint32][]database.CharInventoryEntry `json:"drops,omitempty"`
	// Raid is its raid's hold (raidloot.go: keepRaid), nil for none
	Raid *KeptRaidCorpse `json:"raid,omitempty"`
	// Right is its kill group (lootrights.go), nil for none: the corpse anyone's.
	// Without it every corpse lay open to all after a restart, a stranger's
	// first opening taking the coins and the items (the owner, 2026-11-12).
	Right *KeptRight `json:"right,omitempty"`
}

// KeptRight is a corpse's kill group: the characters the kill counted for and
// the killer's group (kept by its id: RestoreGroups forms each again under it).
type KeptRight struct {
	Chars []uint32 `json:"chars"`
	Group uint32   `json:"group,omitempty"`
}

// keptRight is the kill group h holds for the corpse of the death died, nil
// for none.
func (h *NPCWindowHandler) keptRight(id entity.EntityID, died time.Time) *KeptRight {
	v, ok := h.killRights.Load(id)
	if !ok {
		return nil
	}
	r := v.(lootRight)
	if !r.died.Equal(died) {
		return nil
	}
	k := &KeptRight{Group: r.group}
	for c := range r.chars {
		k.Chars = append(k.Chars, c)
	}
	sort.Slice(k.Chars, func(i, j int) bool { return k.Chars[i] < k.Chars[j] })
	return k
}

// KeepCorpses is what the corpses that lie now hold (KeptCorpse).
func KeepCorpses() []KeptCorpse {
	h := keepWindows.Load()
	if h == nil || h.corpseLies == nil {
		return nil
	}
	ls := h.loots()
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ids := make(map[entity.EntityID]bool, len(ls.corpses)+len(ls.drops))
	for id := range ls.corpses {
		ids[id] = true
	}
	for id := range ls.drops {
		ids[id] = true
	}
	h.killRights.Range(func(key, _ any) bool {
		ids[key.(entity.EntityID)] = true // a corpse no one opened is still its kill group's
		return true
	})
	var out []KeptCorpse
	for id := range ids {
		died, lies := h.corpseLies(id)
		if !lies {
			continue
		}
		k := KeptCorpse{ID: uint32(id), Died: died}
		if c := ls.corpses[id]; c != nil && c.died.Equal(died) {
			k.Opened, k.Items = true, c.items
			k.Raid = keepRaid(ls, c, h.raidClock())
		}
		if d := ls.drops[id]; d != nil && d.died.Equal(died) {
			k.Drops = d.items
		}
		k.Right = h.keptRight(id, died)
		if k.Opened || len(k.Drops) > 0 || k.Right != nil {
			out = append(out, k)
		}
	}
	return out
}

// RestoreCorpses puts back what the corpses hold, once the World has its dead
// again, and reports how many it did: one whose creature does not lie dead of the
// same death (its row gone, or a data step made it another) is passed over.
func RestoreCorpses(kept []KeptCorpse) int {
	return RestoreCorpsesAs(kept, nil)
}

// RestoreCorpsesAs is RestoreCorpses with the copies' corpses the World made
// again under new ids (entity.RestoreCopies: old id to new; an id not in it
// is its own).
func RestoreCorpsesAs(kept []KeptCorpse, ids map[entity.EntityID]entity.EntityID) int {
	h := keepWindows.Load()
	if h == nil || h.corpseLies == nil {
		return 0
	}
	ls := h.loots()
	ls.mu.Lock()
	defer ls.mu.Unlock()
	n := 0
	for _, k := range kept {
		id := entity.EntityID(k.ID)
		if to, ok := ids[id]; ok {
			id = to
		}
		died, lies := h.corpseLies(id)
		if !lies || !died.Equal(k.Died) {
			continue
		}
		if k.Opened {
			c := &corpse{died: died, items: k.Items}
			ls.corpses[id] = c
			if restoreRaid(ls, id, c, k.Raid, h.raidClock()) {
				h.startRaidWorker() // its rolls and holds end on time
			}
		}
		if len(k.Drops) > 0 {
			ls.drops[id] = &questDrops{died: died, items: k.Drops}
		}
		if k.Right != nil {
			r := lootRight{died: died, chars: make(map[uint32]bool, len(k.Right.Chars)), group: k.Right.Group}
			for _, c := range k.Right.Chars {
				r.chars[c] = true
			}
			h.killRights.Store(id, r)
		}
		n++
	}
	return n
}

// ResumeCharacter builds the character of a session carried through a restart
// from its row, as a login does, and gives it what was kept: the handlers' and
// the World's (entity.KeptPlayer). Its client is in the world already; its
// dump comes later (ResumeDump). The session must name the character of its
// own account.
func ResumeCharacter(sess *session.Session, ex KeptExtras, kp entity.KeptPlayer, now time.Time) (*entity.Player, error) {
	h := keepChars.Load()
	if h == nil || h.gameWorld == nil || h.store == nil {
		return nil, errors.New("no character handler")
	}
	record, err := h.store.GetByServerID(sess.CharacterServerID())
	if err != nil {
		return nil, fmt.Errorf("character %d: %w", sess.CharacterServerID(), err)
	}
	if acct := sess.AccountID(); acct == 0 || record.AccountID != acct {
		return nil, fmt.Errorf("character %d ('%s') is account %d's, the session's is %d", record.ServerID, record.CharName, record.AccountID, acct)
	}
	if record.CharName != kp.Name && kp.Name != "" {
		return nil, fmt.Errorf("character %d is '%s' now, it was '%s'", record.ServerID, record.CharName, kp.Name)
	}
	player := h.enterFromRecord(sess, record)
	if player == nil {
		return nil, fmt.Errorf("character %d: no player made", record.ServerID)
	}
	h.gameWorld.RestoreKeptPlayer(sess, kp, now)
	if kp.Pet != nil {
		h.restorePet(sess, *kp.Pet, now) // its pet, summoned or charmed (entity/keeppet.go)
	}
	for _, km := range entity.KeptMercsOf(&kp) {
		mercsH.Load().Restore(sess, player, km, now) // its mercenaries, out and on standby (merc.go, entity/keepmerc.go)
	}
	if kp.Familiar != nil {
		familiarsH.Load().Restore(sess, *kp.Familiar, now) // its familiar (familiar.go)
	}
	resumeRecord(player.CharServerID) // its target window's record lines (combatrecord.go)
	if kp.Jumps && !h.gameWorld.JumpsAllowed() {
		// offered the jump before a restart into a server without them (jump.go)
		SendJumpsOff(sess)
	}
	if kp.RaceLabels {
		// its client keeps its list; a data step in the hold may have changed it
		h.gameWorld.RefreshRaceLabels(h.store.DB())
	}
	// and its race looks, the same (raceoptions.go)
	h.gameWorld.RefreshRaceLooks(h.store.DB(), sess)
	// and the NPCs' body tints, to a client that asked (npctint.go)
	h.gameWorld.RefreshNPCTints(sess)
	// the lists the client has, until its dump sends them anew
	pack, bank := ex.Inventory.list(), ex.Bank.list()
	if pack != nil {
		pack.bank = bank // a Lore item in the bank is one held (itemlist.go's loreRoom)
	}
	sess.SetInventoryList(pack)
	sess.SetBankList(bank)
	if ex.AuctionForm && h.auction != nil {
		h.auction.formMu.Lock()
		h.auction.form[sess] = true
		h.auction.formMu.Unlock()
	}
	if q := Quests(); q != nil {
		if ex.QuestMarks {
			q.SetMarkers(sess, player, true)
		}
		if ex.QuestProgress {
			q.SetProgress(sess, player, true)
		}
		if ex.QuestHeadMarks {
			q.resumeHeadMarks(player)
		}
	}
	if ex.LootOpen {
		queueReliable(sess, protocol.OpcodeClientCloseLoot, closeLootPayload())
	}
	return player, nil
}

// ResumeLine is what a player carried through a restart is told once back.
const ResumeLine = "The server was updated. You stayed in the world."

// ResumeDump sends a resumed session's client its MemoryDump where its
// character stands now (respawned home if it was dead when the restart came),
// with what the dump leaves out: the run speed its effects give, its lockouts
// and, for a GM's client, the GM channel and its flight; and the line that says
// what happened. It reports whether the session has a character.
func ResumeDump(sess *session.Session, dead bool) bool {
	h := keepChars.Load()
	if h == nil || h.gameWorld == nil {
		return false
	}
	gw := h.gameWorld
	p := gw.SessionPlayer(sess)
	if p == nil {
		return false
	}
	if dead {
		// what the respawn it was waiting for does: home, whole, a dump
		h.respawnPlayer(sess, p)
	} else {
		gw.SyncSelfChannels(sess)
		snap, ok := gw.PlayerSnapshot(p.ServerID)
		if !ok {
			return false
		}
		h.queueMemoryDump(sess, &snap)
	}
	h.SendPlayerSpeed(sess, gw.PlayerSpeed(sess, entity.RunSpeed))
	// its client may swing on through the restart with no 0x000F: the hands'
	// swing for that (entity/attack.go pressUnsentLocked)
	handsChanged(gw, h.inventoryStore, sess, p)
	if dead {
		gw.SendPvPStill(sess) // its lockouts went with the respawn's dump
	} else if n := gw.SendTakenOver(sess, time.Now()); n > 0 {
		log.Printf("Keep: '%s' told its %d lockouts", p.Name, n)
	}
	if h.gmChannel != nil {
		h.gmChannel(sess)
	}
	// and a GM in the air is still in the air: the dump stood it where it is
	// and said nothing of flight (fly.go)
	if on, _ := gw.Flying(sess); on {
		SendFlying(sess, true)
	}
	SendClientMessage(sess, ResumeLine)
	return true
}

// restorePet gives the resumed player back the pet the restart carried
// (entity/keeppet.go, petition 1363): a summoned one by its spell's record as
// the database has it now, a charmed one by its creature.
func (h *CharacterHandler) restorePet(sess *session.Session, k entity.KeptPet, now time.Time) {
	var spell *entity.CastSpell
	if k.Spell != 0 && h.spellbookStore != nil {
		sp, err := h.spellbookStore.SpellPattern(k.Spell)
		row, _, err2 := h.spellbookStore.SpellEffectRow(k.Spell)
		if err == nil && err2 == nil && sp != nil {
			s := castSpellRow(&database.CharSpellEntry{Pattern: *sp}, row)
			spell = &s
		} else {
			log.Printf("Keep: the pet %q's spell %d: %v %v", k.Name, k.Spell, err, err2)
		}
	}
	h.gameWorld.RestorePet(sess, k, spell, now)
}
