package handlers

import (
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// rockfreeport.go — the Rock of Freeport (petition 1338, the GM Jibrish,
// 2026-11-09: "Add a stone every character in game gets by default on the
// test server (heirloom). This stone is called "Rock of Freeport" and on use
// it teleports the caster in front of Dabrish in freeport. This is unlimited
// uses. The teleport should have a 2 second cast time, using return homes
// spell animation. infinite use").
//
// world.rock_of_freeport (SetRockOfFreeport: off unless set; the Heirloom Test
// Server sets it, the Live Server does not) turns all of it on. The server
// makes the record at its start when none is so named (EnsureRockOfFreeport:
// no data step, and a GM's edit of it stands). A character holding none is
// given one at its login, before its items are read, in its pack
// (giveRockOfFreeport); one whose pack is full is told so once in the world
// (tellRockOwed) and the next login tries again, so a new character has its
// stone from its first login and needs no starting item. The stone is the gem
// type (procanim 7, as the fang and the portal: the item menu's Use and the
// HUD's gem quick slot), NO TRADE, LORE (one each), worth 0, never spent.
//
// Its Use is a cast of RockCastTime through the World's own casting
// (entity.CastSpell.Travel): the CastingCombat of the caster at itself in
// Return Home's visual (its spelleffect row's, else ReturnHomeVisual), to
// everyone who sees it; no lockout and no 0x00CD, since it is no ability of
// the book; landing as every cast lands, castLand after its cast time, with
// the caster sent where the Summoning Portal's way home lands, a few steps in
// front of Dabrish wherever he stands (freeportSpot, summonportal.go). As
// every cast here it goes on through a move or a hit (CLIENT has no cancel,
// and the server stops no cast for either), death ends it, a use within
// another cast's time is let go, and so is a second stone while the first is
// on its way. With the switch off a stone held does nothing but say so.
//
// Ours, all of it but Return Home's visual: the live game's records hold no
// such item (Sandstorm's travel stones, whose icon it wears, "instantly
// teleport").

// The stone's record's name, its cast and its record (ours, but for the icon).
const (
	RockOfFreeportName = "Rock of Freeport"
	// RockCastTime is the stone's cast: the petition's "2 second cast time"
	RockCastTime = 2 * time.Second
	// rockCastID is the cast's id: no spell's (theirs are positive) and no
	// effect's (it puts none on: food -1001, drink -1002, the boon -1003, the
	// boards' wolf -1004)
	rockCastID = -1005
	// ReturnHomeVisual is Return Home's 0x00B1 visual, the spelleffect fxdict
	// of all three of its records on eqoaclassic (1542, 1623, 2300; 2026-11-09):
	// the stone's when no row gives one
	ReturnHomeVisual uint32 = 0x8FB28233
	// rockRecordID is where the record is made, the next of the kept items'
	// ids (the lamp 715000, the fang 715010, the portal 715020, the Stone of
	// Guenhwyvar 715030; 715040-715042 are Drizzt's gear), or the first free
	// one after it
	rockRecordID = 715050
	// rockIcon is ITEMICON.ESF's blue rune stone, Sandstorm's travel stones'
	// ("Freeport Travel Stone", "Zentar's Keep Travel Stone") and the Grobb
	// Teleportation Rune's
	rockIcon = 414194432
)

// The stone's words (ours).
const (
	rockDesc     = "A smooth stone from the streets of Freeport. Use it and after a short cast you stand in Freeport, before Dabrish. It never wears out."
	rockCasting  = "The Rock of Freeport warms in your hand."
	rockNoPlace  = "The Rock of Freeport cannot find Freeport."
	rockQuiet    = "The Rock of Freeport is quiet here."
	rockOwedLine = "Your pack is full, so your Rock of Freeport could not be given. Make room and it is yours at your next login."
)

// rockOn is world.rock_of_freeport: off unless set.
var rockOn atomic.Bool

// SetRockOfFreeport turns the Rock of Freeport on or off: its record at the
// start, the gift at a login and the stone's Use.
func SetRockOfFreeport(on bool) { rockOn.Store(on) }

// RockOfFreeportOn reports world.rock_of_freeport.
func RockOfFreeportOn() bool { return rockOn.Load() }

// IsRockOfFreeport is whether an item record is the stone: worn nowhere and so
// named.
func IsRockOfFreeport(p *database.ItemPatternRecord) bool {
	return p != nil && p.EquipSlot == -1 && strings.EqualFold(strings.TrimSpace(p.ItemName), RockOfFreeportName)
}

// rockRecord is the stone's record as the server makes it.
func rockRecord() database.ItemPatternRecord {
	return database.ItemPatternRecord{ItemName: RockOfFreeportName, ItemDesc: rockDesc, ItemIcon: rockIcon,
		EquipSlot: -1, Trade: 1, Rent: 1, LevelReq: 1, MaxStack: 1, Duration: 80, ClassUse: 32767, RaceUse: 32767,
		ProcAnim: 7, Lore: 1, Color: 0xFFFFFFFF}
}

// EnsureRockOfFreeport makes the stone's record at the server's start when no
// record is so named (database.EnsureItemRecord, from rockRecordID), and
// reports its id and whether it was made now. Called with the switch on.
func EnsureRockOfFreeport(db *database.SQLiteDB) (int, bool, error) {
	return database.EnsureItemRecord(db.DB, rockRecord(), rockRecordID)
}

// rockOwed is the characters a login could not give the stone (a full pack),
// until each is in the world to be told (tellRockOwed).
var (
	rockOwedMu sync.Mutex
	rockOwed   = map[uint32]bool{}
)

// giveRockOfFreeport gives a character holding none the stone at its login,
// before its items are read: in its pack while that holds fewer than
// invListCap records (worn ones among them). A full pack is noted for a line
// once it is in the world, and the next login tries again. Nothing with the
// switch off or without the record.
func (h *CharacterHandler) giveRockOfFreeport(serverID uint32) {
	if !rockOn.Load() || h.store == nil || h.inventoryStore == nil {
		return
	}
	db := h.store.DB()
	id := keptItemID(db, RockOfFreeportName, "Rock")
	if id == 0 {
		return
	}
	var held, pack int
	if err := db.QueryRow(`SELECT COALESCE(SUM(patternid = ?), 0), COALESCE(SUM(location = 0), 0) FROM charinventory WHERE serverid = ?`,
		id, serverID).Scan(&held, &pack); err != nil {
		log.Printf("Rock: character %d's items: %v", serverID, err)
		return
	}
	if held > 0 {
		return
	}
	if pack >= invListCap {
		log.Printf("Rock: character %d's pack is full: no stone this login (told in the world)", serverID)
		rockOwedMu.Lock()
		rockOwed[serverID] = true
		rockOwedMu.Unlock()
		return
	}
	var hp int
	_ = db.QueryRow(`SELECT maxhp FROM itempattern WHERE patternid = ?`, id).Scan(&hp)
	if _, err := h.inventoryStore.AddCharItem(serverID, id, 1, hp, 0, 0); err != nil {
		log.Printf("Rock: character %d's stone: %v", serverID, err)
		return
	}
	log.Printf("Rock: character %d given its Rock of Freeport (record %d)", serverID, id)
}

// tellRockOwed tells a player entering the world, once, that its login had no
// room for the stone.
func tellRockOwed(sess *session.Session, p *entity.Player) {
	rockOwedMu.Lock()
	owed := rockOwed[p.CharServerID]
	delete(rockOwed, p.CharServerID)
	rockOwedMu.Unlock()
	if owed {
		SendColoredChat(sess, rockOwedLine, 255, 255, 0)
	}
}

// rockCast is the stone's cast: RockCastTime on its caster alone, in the
// visual fx, sending it to `to` as it lands.
func rockCast(fx uint32, to entity.GateSpot) entity.CastSpell {
	return entity.CastSpell{SpellID: rockCastID, Name: RockOfFreeportName, CastTime: RockCastTime, FX: fx, Travel: &to}
}

// returnHomeVisual is Return Home's 0x00B1 visual: its spelleffect row's,
// else ReturnHomeVisual.
func (h *EquipHandler) returnHomeVisual() uint32 {
	if h.spells == nil {
		return ReturnHomeVisual
	}
	row, ok, err := h.spells.SpellEffectRow(database.ReturnHomeSpell)
	if err != nil {
		log.Printf("Rock: Return Home's visual: %v", err)
	}
	if !ok || row.FX == 0 {
		return ReturnHomeVisual
	}
	return row.FX
}

// useRockOfFreeport is the stone's Use: with the switch off its line alone;
// else the place in front of Dabrish, refused in red without one, and the
// stone's cast, which sends the player there as it lands, said in a line. The
// dead are told nothing, and a use StartCast would let go (within another
// cast, a stone's trip on its way) is answered as a cast's is, not at all,
// and asked first (TravelBusy), so a press held looks for no place: finding
// Dabrish walks every NPC.
func (h *EquipHandler) useRockOfFreeport(sess *session.Session, player *entity.Player) {
	if h.gameWorld == nil || player == nil {
		return
	}
	now := time.Now()
	if h.gameWorld.TravelBusy(sess, now) {
		return
	}
	if !rockOn.Load() {
		SendErrorMessage(sess, rockQuiet)
		return
	}
	to, err := freeportSpot(h.gameWorld)
	if err != nil {
		log.Printf("Rock: %s: %v", player.Name, err)
		SendErrorMessage(sess, rockNoPlace)
		return
	}
	if h.gameWorld.StartCast(sess, rockCast(h.returnHomeVisual(), to), 0xFFFFFFFF, now) {
		SendClientMessage(sess, rockCasting)
	}
}
