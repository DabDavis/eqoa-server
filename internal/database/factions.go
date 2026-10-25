package database

import "fmt"

// Factions: every faction, the standing a character starts with, a
// character's own standings and what a kill does to them (entity/faction.go).
//
//   - factions (name, standing): the factions and the starting standing. It is
//     filled from DefaultFactions when it is empty, so a GM's edit of it stays.
//   - charfactions (serverid, faction, standing): a character's standing with a
//     faction it has moved off the starting one, kept over a logout.
//   - factionhits (npc_name, faction, delta, source): killing an NPC of that
//     name adds delta to the killer's standing with that faction. The numbers
//     are this server's, not measured: no capture of a faction hit exists.
//     tools/seed_faction_hits.py writes the town rows (source "town-seed").

// FactionDefault is a faction and the standing a character starts with.
type FactionDefault struct {
	Name     string
	Standing int
}

// FactionHitRow is one row of factionhits: what killing NPCName does to the
// killer's standing with Faction.
type FactionHitRow struct {
	NPCName string
	Faction string
	Delta   int
}

// DefaultFactions are the factions and the standings an eqoa.live capture of
// 2026-02-13 sent for one character (a gnome: Klick`Anon 300, Qeynos -2000),
// in its order: standing high to low, then name. Every character starts with
// them; the server had no other standings before 2026-10-25.
var DefaultFactions = []FactionDefault{
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
	{"The Wolves of the North", -2000},
}

// seedFactions fills an empty factions table from DefaultFactions.
func (db *SQLiteDB) seedFactions() error {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM factions`).Scan(&n); err != nil || n > 0 {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for _, f := range DefaultFactions {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO factions (name, standing) VALUES (?, ?)`, f.Name, f.Standing); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// FactionDefaults is the factions table, in the window's order (standing high
// to low, then name).
func (db *SQLiteDB) FactionDefaults() ([]FactionDefault, error) {
	rows, err := db.Query(`SELECT name, standing FROM factions ORDER BY standing DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FactionDefault
	for rows.Next() {
		var f FactionDefault
		if err := rows.Scan(&f.Name, &f.Standing); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// FactionHits is the factionhits table.
func (db *SQLiteDB) FactionHits() ([]FactionHitRow, error) {
	rows, err := db.Query(`SELECT npc_name, faction, delta FROM factionhits ORDER BY npc_name, faction`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FactionHitRow
	for rows.Next() {
		var h FactionHitRow
		if err := rows.Scan(&h.NPCName, &h.Faction, &h.Delta); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// LoadFactions is a character's own standings (charfactions), by faction.
func (s *SQLiteCharacterStore) LoadFactions(serverID uint32) (map[string]int, error) {
	rows, err := s.db.Query(`SELECT faction, standing FROM charfactions WHERE serverid = ?`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var name string
		var v int
		if err := rows.Scan(&name, &v); err != nil {
			return nil, err
		}
		out[name] = v
	}
	return out, rows.Err()
}

// SaveFactions replaces a character's own standings with m.
func (s *SQLiteCharacterStore) SaveFactions(serverID uint32, m map[string]int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM charfactions WHERE serverid = ?`, serverID); err != nil {
		tx.Rollback()
		return err
	}
	for name, v := range m {
		if _, err := tx.Exec(`INSERT INTO charfactions (serverid, faction, standing) VALUES (?, ?, ?)`, serverID, name, v); err != nil {
			tx.Rollback()
			return fmt.Errorf("charfactions %d %q: %w", serverID, name, err)
		}
	}
	return tx.Commit()
}

// SetFaction keeps one of a character's standings (a GM's #faction).
func (s *SQLiteCharacterStore) SetFaction(serverID uint32, faction string, standing int) error {
	_, err := s.db.Exec(`INSERT INTO charfactions (serverid, faction, standing) VALUES (?, ?, ?)
		ON CONFLICT (serverid, faction) DO UPDATE SET standing = excluded.standing`, serverID, faction, standing)
	return err
}
