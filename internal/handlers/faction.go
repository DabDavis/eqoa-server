package handlers

import (
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// FactionHandler handles faction-related messages.
type FactionHandler struct {
	sessionManager *session.Manager
	gameWorld      *entity.World
}

// NewFactionHandler creates a new faction handler.
func NewFactionHandler(sessionMgr *session.Manager) *FactionHandler {
	return &FactionHandler{sessionManager: sessionMgr}
}

// SetGameWorld sets the game world reference.
func (h *FactionHandler) SetGameWorld(gw *entity.World) {
	h.gameWorld = gw
}

// RegisterHandlers registers faction handlers with the registry.
func (h *FactionHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeClientFaction, h.HandleClientFaction)
	r.Register(protocol.OpcodeFactionMessages, h.HandleFactionMessages)
	r.Register(protocol.OpcodeLootMessages, h.HandleLootMessages)
}

// HandleFactionMessages handles the faction message toggle (0x00C9).
// Client sends this on world entry: byte(state) + byte(0). state: 0=show, 1=hide.
func (h *FactionHandler) HandleFactionMessages(sess *session.Session, msg *protocol.Message) error {
	log.Printf("FactionMessages from %s: %x", sess.RemoteAddr(), msg.Data())
	return nil
}

// HandleLootMessages handles the loot message toggle (0x00CA).
// Client sends this on world entry: byte(state) + byte(0). state: 0=show, 1=hide.
func (h *FactionHandler) HandleLootMessages(sess *session.Session, msg *protocol.Message) error {
	log.Printf("LootMessages from %s: %x", sess.RemoteAddr(), msg.Data())
	return nil
}

// HandleClientFaction handles a faction page request (0x0AC4).
// Wire format in: byte(0x66) + zigzag(pageID) + byte(pageIndex)
// Wire format out: byte(0x77) + zigzag(pageID) + byte(pageIndex) + byte(0x44) +
//
//	zigzag(count) + [int32(nameLen) + utf16(name) + zigzag(standing)] × count
//
// pageID is a session-specific counter the client increments per request.
// pageIndex identifies which faction page (0, 2, 4, ... 66 for 34 pages).
// Server echoes back the client's pageID and pageIndex.
func (h *FactionHandler) HandleClientFaction(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 3 {
		return nil
	}

	reader := buffer.NewReader(data)

	// Read request: byte(0x66) + zigzag(pageID) + byte(pageIndex)
	marker, _ := reader.ReadByte()
	if marker != 0x66 {
		log.Printf("ClientFaction: unexpected marker 0x%02X", marker)
		return nil
	}

	pageID, _ := reader.Read7BitEncodedInt64()
	pageIndex, _ := reader.ReadByte()

	log.Printf("ClientFaction from %s: pageID=%d pageIndex=%d", sess.RemoteAddr(), pageID, pageIndex)

	// Look up by pageIndex (not pageID, which is session-specific)
	page, ok := factionPages[int(pageIndex)]
	if !ok || len(page) == 0 {
		return nil // No factions on this page — don't respond (matches eqoa.live)
	}

	// Build response: opcode(2) + byte(0x77) + zigzag(pageID) + byte(pageIndex) + byte(0x44) + zigzag(count) + entries
	resp := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeFactionPage)
	buf := make([]byte, 8192)
	w := buffer.NewWriter(buf)

	// Opcode
	w.WriteUint16(uint16(protocol.OpcodeFactionPage))

	// Response header — echo back client's pageID and pageIndex
	w.WriteByte(0x77)
	w.Write7BitEncodedInt64(pageID)
	w.WriteByte(pageIndex)
	w.WriteByte(0x44) // constant flag

	// Count of factions in this page
	w.Write7BitEncodedInt64(int64(len(page)))

	// Faction entries
	for _, f := range page {
		// int32 name length (chars, not bytes)
		nameRunes := []rune(f.Name)
		w.WriteInt32(int32(len(nameRunes)))
		// UTF-16LE name
		w.WriteStringUnicodeRaw(f.Name)
		// Standing as zigzag
		w.Write7BitEncodedInt64(int64(f.Standing))
	}

	resp.SetData(buf[:w.Position()])
	sess.Queue().Add(resp)
	return nil
}

// FactionEntry represents a single faction with its default standing.
type FactionEntry struct {
	Name     string
	Standing int
}

// factionPages maps pageIndex → list of faction entries.
// PageIndex goes 0, 2, 4, ... 66 (34 pages, 10 factions each except last).
// Client sends pageID (session-specific counter) + pageIndex; server echoes both back.
// Data reverse-engineered from eqoa.live pcap (2026-02-13). 331 factions total.
var factionPages = map[int][]FactionEntry{
	0: {
		{"Citizens of Klick`Anon", 300},
		{"Klick`Anon Watch", 300},
		{"Academy of Arcane Science", 1},
		{"Actour's Guild", 1},
		{"Allies of Kithrin", 1},
		{"Bearweres", 1},
		{"Blacktoe Ogres", 1},
		{"Brokenskull Rock", 1},
		{"Chosen of Gunthak", 1},
		{"Church of the Holy Pearl", 1},
	},
	2: {
		{"Citizens of Baga", 1},
		{"Citizens of Bastable", 1},
		{"Citizens of Blackwater", 1},
		{"Citizens of Blakedown", 1},
		{"Citizens of Bogman", 1},
		{"Citizens of Brog Fens SW", 1},
		{"Citizens of Dark Solace", 1},
		{"Citizens of Darvar Manor", 1},
		{"Citizens of Ferran's Hope ", 1},
		{"Citizens of Fort Seriak", 1},
	},
	4: {
		{"Citizens of Freeport", 1},
		{"Citizens of Grobb", 1},
		{"Citizens of Hazinak", 1},
		{"Citizens of Highpass", 1},
		{"Citizens of Honjour", 1},
		{"Citizens of Kelinar", 1},
		{"Citizens of Kerplunk Outpost", 1},
		{"Citizens of Morthalis", 1},
		{"Citizens of Muniels Tea Garden", 1},
		{"Citizens of Murnf", 1},
	},
	6: {
		{"Citizens of Neriak", 1},
		{"Citizens of Oasis", 1},
		{"Citizens of Oggok", 1},
		{"Citizens of Paineel", 1},
		{"Citizens of Wyndhaven", 1},
		{"Citizens of Zentar's Keep", 1},
		{"Craknek Warriors", 1},
		{"Cult of the Dragon", 1},
		{"Da Bashers", 1},
		{"Defenders of Highpass", 1},
	},
	8: {
		{"Disciples of Plague", 1},
		{"Dumak Olivarn", 1},
		{"Escaped Kappa", 1},
		{"Explorers Guild 135", 1},
		{"Feria Hurkert", 1},
		{"Followers of Mog", 1},
		{"Fort Seriak", 1},
		{"Gatorweres", 1},
		{"Gypsies of Tunaria", 1},
		{"Gypsy Werehunters", 1},
	},
	10: {
		{"House D`Narus", 1},
		{"House Do`Vexis", 1},
		{"House Gal`Saris", 1},
		{"House J`Narus", 1},
		{"House U`Dedne", 1},
		{"Institute of Astral Cognizance", 1},
		{"Jared's Tower", 1},
		{"Klick`Anon Clockworks", 1},
		{"Light of Stoneclaw", 1},
		{"Lionweres", 1},
	},
	12: {
		{"Lodge of the Dead", 1},
		{"Marius and Darius", 1},
		{"Mechanamagical College", 1},
		{"Merchant House Nothard", 1},
		{"Merchant House Slaerin", 1},
		{"Miners Guild 189", 1},
		{"Mog Thugs", 1},
		{"Monks of the Endless Tower", 1},
		{"Muniel's Tea Garden", 1},
		{"Necrological Society", 1},
	},
	14: {
		{"Necromancers of Rosethorn Spire", 1},
		{"Night Reapers", 1},
		{"Outlaws of Grobb", 1},
		{"Penalty Notes", 1},
		{"Pwinn of Gal`Saris", 1},
		{"Radavex Planeswalker", 1},
		{"Raiders of Takish`Hiz", 1},
		{"Ratweres", 1},
		{"Razgut", 1},
		{"Royal Players", 1},
	},
	16: {
		{"Saerk", 1},
		{"Seisen Monks", 1},
		{"Servants of the Hidden", 1},
		{"Servants of the Sickle", 1},
		{"Shadowknights of Nightkeep", 1},
		{"Silken Gauntlet", 1},
		{"Sphynxs of Rathe", 1},
		{"Spiritwalkers", 1},
		{"Stone Watchers", 1},
		{"Sunmanes", 1},
	},
	18: {
		{"Talamirs Regiment", 1},
		{"Temple of Brell", 1},
		{"Temple of Greenblood", 1},
		{"The Bone Crusher", 1},
		{"The Church of Innoruuk", 1},
		{"The Dark Ones", 1},
		{"The Ebon Mask", 1},
		{"The Freeport Militia", 1},
		{"The Hand of Quellious", 1},
		{"The Hidden", 1},
	},
	20: {
		{"The Highpass Guard", 1},
		{"The Houseless Rebelion", 1},
		{"The Indigo Brotherhood", 1},
		{"The Junk Mongers", 1},
		{"The Last Inn", 1},
		{"The Last Smith", 1},
		{"The Partisans of Morthalis", 1},
		{"The Royal Guard", 1},
		{"The Spiteful Shield", 1},
		{"The Spurned", 1},
	},
	22: {
		{"The Storyteller", 1},
		{"Topi Di Ombra", 1},
		{"Vampire Hunters", 1},
		{"Villagers of Sonderal", 1},
		{"Villagers of Waterspout", 1},
		{"Wards of Dawn", 1},
		{"Wolfweres", 1},
		{"Zentar Reese", 1},
		{"Befallen", 0},
		{"Citizens of Dshinn", 0},
	},
	24: {
		{"City Warden", 0},
		{"Followers of Veeshan", 0},
		{"Goblin Prisoners", 0},
		{"Granite Golem", 0},
		{"Kerplunk (unused)", 0},
		{"Mithaniel Marr's Crusaders", 0},
		{"People of Jextalan", 0},
		{"Soldiers of Rallos Zek", 0},
		{"Tendora-Buki Tribe", 0},
		{"Test Faction", 0},
	},
	26: {
		{"The Crescent Covenant", 0},
		{"The Merka Huntsmen", 0},
		{"The Merka Tribe", 0},
		{"The Rainfeather Tribe", 0},
		{"The Spirits of the Forest", 0},
		{"The Taksehir Nation", 0},
		{"Tunarian Gypsies", 0},
		{"Werehunters of Tunaria", 0},
		{"Bloodsabers", -1},
		{"Citizens of Melton", -1},
	},
	28: {
		{"Verthok the Executioner", -1},
		{"Citizens of Hagley", -99},
		{"Citizens of Graumord", -150},
		{"Black Swan Inn", -200},
		{"Werewolves of Lightwolf", -200},
		{"Pickclaw Goblins", -250},
		{"Citizens of Highbourne", -300},
		{"Crusaders of Marr's Fist", -300},
		{"Library of Highbourne", -300},
		{"Priests of Life", -300},
	},
	30: {
		{"Slaves of Paineel", -300},
		{"Smugglers of Temby", -300},
		{"The Highbourne Senate", -300},
		{"The Peacekeepers", -300},
		{"Denizens of the Plane of Sky", -349},
		{"Kejek Village", -349},
		{"Welahi-Nai Tribe", -349},
		{"Citizens of Diren Hold", -400},
		{"Desert Bandits", -400},
		{"Guards of Diren Hold", -400},
	},
	32: {
		{"The Pusling", -400},
		{"Verthok's Followers", -450},
		{"Aviaks of Tunaria", -500},
		{"Aviaks of the Karanas", -500},
		{"Bandits of Tunaria", -500},
		{"Barbarians of the Anu Tribe", -500},
		{"Black Ants", -500},
		{"Blackburrow Gnolls", -500},
		{"Blackscourge Pirates", -500},
		{"Blacktongue Orcs", -500},
	},
	34: {
		{"Bloodfoot Bandits", -500},
		{"Bluefeather's Brood", -500},
		{"Brontar's Bandits", -500},
		{"Cawtou Aviaks", -500},
		{"Centaurs of the Plains", -500},
		{"Chaos Minions of Takish`Hiz", -500},
		{"Chiktar Hive", -500},
		{"Citizens of Caer Sloth", -500},
		{"Clan Ankextet", -500},
		{"Clan Kolbok Kobolds", -500},
	},
	36: {
		{"Clan Runnyeye", -500},
		{"Clan Snowfist", -500},
		{"Clockwork Town", -500},
		{"Coalescence of Evil", -500},
		{"Coldpaw", -500},
		{"Cracktooth Orcs", -500},
		{"Crethley Manor", -500},
		{"Darkwood Goblins", -500},
		{"Deathfist Hordes", -500},
		{"Deathfist Mercenaries", -500},
	},
	38: {
		{"Deepscarabs", -500},
		{"Desert Dragon Skahyir", -500},
		{"Double Dagger Rats", -500},
		{"Fire Giants", -500},
		{"Fire Pit Kobolds", -500},
		{"Followers of Quintashi", -500},
		{"Frosteye Cohort", -500},
		{"Fungusmen of Guk", -500},
		{"Gate Guards of Urglunt", -500},
		{"Ghosts of SouthWatch", -500},
	},
	40: {
		{"Gnoll Raiders", -500},
		{"Gnolls of Tunaria", -500},
		{"Goldfeather Aviaks", -500},
		{"Grimrik's Outcasts", -500},
		{"Hatebone Orcs", -500},
		{"Haz'Kar", -500},
		{"Hazinak Temple", -500},
		{"Hill Giants", -500},
		{"Hobgoblins of Rathe Mtns.", -500},
		{"Ice Giants of Everfrost", -500},
	},
	42: {
		{"Inhabitants of the Plane of Sky", -500},
		{"Jaeger Mansion", -500},
		{"Jareth Roanir", -500},
		{"Kerreth Village", -500},
		{"Khelmarak", -500},
		{"Kikarnta Kobolds", -500},
		{"Lady Vox", -500},
		{"Lavastorm Goblins", -500},
		{"Lizardmen of Tunaria", -500},
		{"Lord Nagafen", -500},
	},
	44: {
		{"Lost Guards", -500},
		{"Lurking Bandits", -500},
		{"Madman", -500},
		{"Minions of Bertoxxulous", -500},
		{"Minions of Vox", -500},
		{"Minotaurs of Tunaria", -500},
		{"Mossmouth Goblins", -500},
		{"Mountain Feather", -500},
		{"Nasehir Cutthroats", -500},
		{"Orcs of Tunaria", -500},
	},
	46: {
		{"Plankwalker's Pirates", -500},
		{"Qeynos Prisoners", -500},
		{"Ragel Aviaks", -500},
		{"Red Ants", -500},
		{"Red Ear Gnolls", -500},
		{"Rustel Minotaurs", -500},
		{"Sand Giants of Ro", -500},
		{"Sarek the Betrayer", -500},
		{"Serpent's Den", -500},
		{"Shadows of Stoneclaw", -500},
	},
	48: {
		{"Siliskor", -500},
		{"Slesher Gnolls", -500},
		{"SlithTar", -500},
		{"Solusek Goblins", -500},
		{"Sylhilthis", -500},
		{"Sylhilthis' Chosen Few", -500},
		{"Sylhilthis' Slaves", -500},
		{"Tae Ew Lizards", -500},
		{"Taereg of Tunaria", -500},
		{"Tarlin's Bandits", -500},
	},
	50: {
		{"The Barrow Cult", -500},
		{"The Eye", -500},
		{"The Freezeblood Goblins", -500},
		{"The Frogloks of Guk", -500},
		{"The Werebear Witchking", -500},
		{"Tomb City of Envar", -500},
		{"Towers of Noctix", -500},
		{"Toxxulia's Brood", -500},
		{"Tubrenzalan", -500},
		{"Undead Frogloks", -500},
	},
	52: {
		{"Undead Frogloks of Guk", -500},
		{"Undead of Paineel", -500},
		{"Verghest Felstar", -500},
		{"Worshippers of Siliskor", -500},
		{"Followers of the Crippled Lion", -600},
		{"Citizens of Castle Lightwolf", -800},
		{"Citizens of Bobble-by-Water", -1000},
		{"Citizens of Moradhim", -1000},
		{"Citizens of Rivervale", -1000},
		{"Denizens of Arcadin", -1000},
	},
	54: {
		{"Denizens of Dread", -1000},
		{"Fort Alliance", -1000},
		{"The Depthsworn Court", -1000},
		{"The Doomseekers of Moradhim", -1000},
		{"The Fey", -1000},
		{"The Indigo Pirates", -1000},
		{"The Wandering Frogloks", -1000},
		{"Treants of the Unkempt Wood", -1000},
		{"Unkempt Defenders", -1000},
		{"Binshore", -2000},
	},
	56: {
		{"Chiasta Citizens", -2000},
		{"Citizens of Farstone", -2000},
		{"Citizens of Fayspires", -2000},
		{"Citizens of Forkwatch", -2000},
		{"Citizens of Gerntar", -2000},
		{"Citizens of Halas", -2000},
		{"Citizens of Hodstock", -2000},
		{"Citizens of Jethro Pike", -2000},
		{"Citizens of Loreth Kith", -2000},
		{"Citizens of Mariel", -2000},
	},
	58: {
		{"Citizens of Merry-by-water", -2000},
		{"Citizens of Qeynos", -2000},
		{"Citizens of Stormhaven", -2000},
		{"Citizens of Surefall Glade", -2000},
		{"Citizens of Tethelin", -2000},
		{"Citizens of Wymondham", -2000},
		{"College of High Magic", -2000},
		{"Crusaders of Mithaniel Marr", -2000},
		{"Defenders of Erollisi Marr", -2000},
		{"Elves of Tak`Xiv", -2000},
	},
	60: {
		{"Eyes of the Tribunal", -2000},
		{"Farmers of Hodstock", -2000},
		{"Followers of Rodcet Nife", -2000},
		{"Guardians of the Grove", -2000},
		{"Keepers of the Glade", -2000},
		{"Last Defenders of Feydwell", -2000},
		{"Loreth Kith", -2000},
		{"Miners Guild 231", -2000},
		{"Order of the Silent Fist", -2000},
		{"Protectors of the Pine", -2000},
	},
	62: {
		{"Proudpine Outpost", -2000},
		{"Qeynos Troupe", -2000},
		{"Remnant forces of Takish`Hiz", -2000},
		{"Scouts of Tunare", -2000},
		{"Seers of the Tribunal", -2000},
		{"Songweavers of Tunare", -2000},
		{"Stormhaven Guard", -2000},
		{"Temple of Bristlebane", -2000},
		{"The Anagogical Order", -2000},
		{"The Church of Below", -2000},
	},
	64: {
		{"The Deep Pockets", -2000},
		{"The Emerald Guardians", -2000},
		{"The Farstone Guard", -2000},
		{"The Guardians of the Vale", -2000},
		{"The Heavy Purse", -2000},
		{"The Paladins of Tunare", -2000},
		{"The Qeynos Guard", -2000},
		{"The Stoneguard", -2000},
		{"The Stormreapers", -2000},
		{"The Thinhairs", -2000},
	},
	66: {
		{"The Wolves of the North", -2000},
	},
}
