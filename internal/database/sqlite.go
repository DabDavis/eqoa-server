package database

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteDB wraps a SQLite database connection.
type SQLiteDB struct {
	*sql.DB
	path string
}

// NewSQLite creates or opens a SQLite database.
func NewSQLite(dbPath string) (*SQLiteDB, error) {
	// Create directory if it doesn't exist
	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}

	// Open database with WAL mode for better concurrency
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}

	// Configure connection pool
	db.SetMaxOpenConns(1) // SQLite handles concurrency differently
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0) // Connections don't expire

	// Verify connection
	if err := db.Ping(); err != nil {
		return nil, err
	}

	sqlite := &SQLiteDB{DB: db, path: dbPath}

	// Initialize schema
	if err := sqlite.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	log.Printf("SQLite database opened: %s", dbPath)
	return sqlite, nil
}

// initSchema creates the required tables if they don't exist.
// Schema matches C# EQOA_Master.sql exactly (column names, types, constraints).
func (db *SQLiteDB) initSchema() error {
	schema := `
	-- accountinfo (matches C# EQOA_Master.sql)
	CREATE TABLE IF NOT EXISTS accountinfo (
		accountid INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		pwhash BLOB,
		acctstatus INTEGER,
		acctlevel INTEGER,
		acctcreation DATETIME,
		lastlogin DATETIME,
		ipaddress TEXT,
		firstname TEXT,
		unknown1 TEXT,
		midinitial TEXT,
		lastname TEXT,
		unknown2 TEXT,
		countryAB TEXT,
		zip TEXT,
		birthday TEXT,
		birthyear TEXT,
		birthmon TEXT,
		sex TEXT,
		email TEXT,
		result INTEGER,
		subtime INTEGER,
		partime INTEGER,
		subfeatures INTEGER,
		gamefeatures INTEGER
	);

	-- charactermodel (matches C# exactly)
	CREATE TABLE IF NOT EXISTS charactermodel (
		sex INTEGER NOT NULL,
		modelid INTEGER NOT NULL PRIMARY KEY,
		race INTEGER NOT NULL
	);

	-- characters (matches C# exactly: serverid is PK, charName is UNIQUE)
	CREATE TABLE IF NOT EXISTS characters (
		charName TEXT UNIQUE NOT NULL,
		accountid INTEGER NOT NULL,
		serverid INTEGER PRIMARY KEY AUTOINCREMENT,
		modelid INTEGER NOT NULL,
		tclass INTEGER NOT NULL,
		race INTEGER NOT NULL,
		humType INTEGER NOT NULL,
		level INTEGER NOT NULL,
		haircolor INTEGER NOT NULL,
		hairlength INTEGER NOT NULL,
		hairstyle INTEGER NOT NULL,
		faceoption INTEGER NOT NULL,
		sex INTEGER NOT NULL,
		totalXP INTEGER NOT NULL DEFAULT 0,
		debt INTEGER NOT NULL DEFAULT 0,
		breath INTEGER NOT NULL DEFAULT 255,
		tunar INTEGER NOT NULL DEFAULT 0,
		bankTunar INTEGER NOT NULL DEFAULT 0,
		unusedTP INTEGER NOT NULL DEFAULT 0,
		totalTP INTEGER NOT NULL DEFAULT 0,
		Speed REAL NOT NULL DEFAULT 8.0,
		world INTEGER NOT NULL DEFAULT 0,
		x REAL NOT NULL DEFAULT 0,
		y REAL NOT NULL DEFAULT 0,
		z REAL NOT NULL DEFAULT 0,
		facing REAL NOT NULL DEFAULT 0,
		unknown REAL NOT NULL DEFAULT 0,
		tpstrength INTEGER NOT NULL DEFAULT 0,
		tpstamina INTEGER NOT NULL DEFAULT 0,
		tpagility INTEGER NOT NULL DEFAULT 0,
		tpdexterity INTEGER NOT NULL DEFAULT 0,
		tpwisdom INTEGER NOT NULL DEFAULT 0,
		tpintel INTEGER NOT NULL DEFAULT 0,
		tpcharisma INTEGER NOT NULL DEFAULT 0,
		currentHP INTEGER NOT NULL DEFAULT 100,
		currentPower INTEGER NOT NULL DEFAULT 100,
		unk42_1 INTEGER NOT NULL DEFAULT 0,
		ac INTEGER NOT NULL DEFAULT 0,
		unk42_2 INTEGER NOT NULL DEFAULT 0,
		unk42_3 INTEGER NOT NULL DEFAULT 0,
		unk42_4 INTEGER NOT NULL DEFAULT 0,
		unk42_5 INTEGER NOT NULL DEFAULT 0,
		unk42_6 INTEGER NOT NULL DEFAULT 0,
		unk42_7 INTEGER NOT NULL DEFAULT 0,
		unk42_8 INTEGER NOT NULL DEFAULT 0,
		poisonr INTEGER NOT NULL DEFAULT 0,
		diseaser INTEGER NOT NULL DEFAULT 0,
		firer INTEGER NOT NULL DEFAULT 0,
		coldr INTEGER NOT NULL DEFAULT 0,
		lightningr INTEGER NOT NULL DEFAULT 0,
		arcaner INTEGER NOT NULL DEFAULT 0,
		fishing INTEGER NOT NULL DEFAULT 0,
		unk42_9 INTEGER NOT NULL DEFAULT 0,
		unk42_10 INTEGER NOT NULL DEFAULT 0,
		unk42_11 INTEGER NOT NULL DEFAULT 0,
		player_flags TEXT NOT NULL DEFAULT '',
		completed_quests TEXT NOT NULL DEFAULT '',
		active_quests TEXT NOT NULL DEFAULT '',
		FOREIGN KEY (accountid) REFERENCES accountinfo(accountid)
	);

	-- charinventory
	CREATE TABLE IF NOT EXISTS charinventory (
		itemid INTEGER PRIMARY KEY AUTOINCREMENT,
		serverid INTEGER NOT NULL,
		stackleft INTEGER NOT NULL,
		remainHP INTEGER NOT NULL,
		remaincharge INTEGER NOT NULL,
		patternid INTEGER NOT NULL,
		equiploc INTEGER NOT NULL,
		location INTEGER NOT NULL,
		listnumber INTEGER NOT NULL,
		FOREIGN KEY (serverid) REFERENCES characters(serverid) ON DELETE CASCADE,
		FOREIGN KEY (patternid) REFERENCES itempattern(patternid)
	);

	-- defaultcharacter
	CREATE TABLE IF NOT EXISTS defaultcharacter (
		defaultCharacterID INTEGER PRIMARY KEY AUTOINCREMENT,
		tclass INTEGER NOT NULL,
		race INTEGER NOT NULL,
		humType INTEGER NOT NULL,
		level INTEGER NOT NULL,
		tunar INTEGER NOT NULL,
		bankTunar INTEGER NOT NULL,
		unusedTP INTEGER NOT NULL,
		totalTP INTEGER NOT NULL,
		x REAL NOT NULL,
		y REAL NOT NULL,
		z REAL,
		facing REAL NOT NULL,
		defaultSpeed REAL,
		world INTEGER,
		defaultStrength INTEGER,
		defaultStamina INTEGER,
		defaultAgility INTEGER,
		defaultDexterity INTEGER,
		defaultWisdom INTEGER,
		defaultIntelligence INTEGER,
		defaultCharisma INTEGER
	);

	-- defaultcharinventory
	CREATE TABLE IF NOT EXISTS defaultcharinventory (
		itemid INTEGER PRIMARY KEY AUTOINCREMENT,
		tclass TEXT NOT NULL,
		stackleft INTEGER NOT NULL,
		remainHP INTEGER NOT NULL,
		remaincharge INTEGER NOT NULL,
		patternid INTEGER NOT NULL,
		equiploc INTEGER NOT NULL,
		location INTEGER NOT NULL,
		listnumber INTEGER NOT NULL
	);

	-- defaultspells
	CREATE TABLE IF NOT EXISTS defaultspells (
		def_spell_id INTEGER PRIMARY KEY AUTOINCREMENT,
		tclass TEXT NOT NULL,
		spellid INTEGER NOT NULL,
		addedorder INTEGER NOT NULL,
		onHotBar INTEGER NOT NULL,
		whereonBar INTEGER NOT NULL,
		unk1 INTEGER NOT NULL,
		showhide INTEGER NOT NULL
	);

	-- hotkeys
	CREATE TABLE IF NOT EXISTS hotkeys (
		hotkeyid INTEGER PRIMARY KEY AUTOINCREMENT,
		serverid INTEGER NOT NULL,
		direction TEXT NOT NULL,
		Nlabel TEXT,
		Nmessage TEXT,
		Wlabel TEXT,
		Wmessage TEXT,
		Elabel TEXT,
		Emessage TEXT,
		Slabel TEXT,
		Smessage TEXT,
		FOREIGN KEY (serverid) REFERENCES characters(serverid) ON DELETE CASCADE
	);

	-- itempattern
	CREATE TABLE IF NOT EXISTS itempattern (
		patternid INTEGER PRIMARY KEY AUTOINCREMENT,
		itemcost INTEGER NOT NULL,
		unk1 INTEGER NOT NULL,
		itemicon INTEGER NOT NULL,
		unk2 INTEGER NOT NULL,
		equipslot INTEGER NOT NULL,
		unk3 INTEGER NOT NULL,
		trade INTEGER NOT NULL,
		rent INTEGER NOT NULL,
		unk4 INTEGER NOT NULL,
		attacktype INTEGER NOT NULL,
		weapondamage INTEGER NOT NULL,
		unk5 INTEGER NOT NULL,
		levelreq INTEGER NOT NULL,
		maxstack INTEGER NOT NULL,
		maxhp INTEGER NOT NULL,
		duration INTEGER NOT NULL,
		classuse INTEGER NOT NULL,
		raceuse INTEGER NOT NULL,
		procanim INTEGER NOT NULL,
		lore INTEGER NOT NULL,
		unk6 INTEGER NOT NULL,
		craft INTEGER NOT NULL,
		itemname TEXT NOT NULL,
		itemdesc TEXT NOT NULL,
		model INTEGER NOT NULL,
		color INTEGER NOT NULL,
		str INTEGER,
		sta INTEGER,
		agi INTEGER,
		wis INTEGER,
		dex INTEGER,
		cha INTEGER,
		intelligence INTEGER,
		HPMAX INTEGER,
		POWMAX INTEGER,
		PoT INTEGER,
		HoT INTEGER,
		AC INTEGER,
		PR INTEGER,
		DR INTEGER,
		FR INTEGER,
		CR INTEGER,
		LR INTEGER,
		AR INTEGER,
		weaponproc INTEGER,
		fish INTEGER
	);

	-- npcitem
	CREATE TABLE IF NOT EXISTS npcitem (
		itemid INTEGER PRIMARY KEY AUTOINCREMENT,
		npcid INTEGER NOT NULL,
		stackleft INTEGER NOT NULL DEFAULT 0,
		remainHP INTEGER NOT NULL DEFAULT 0,
		remaincharge INTEGER NOT NULL DEFAULT 0,
		patternid INTEGER NOT NULL,
		equiploc INTEGER NOT NULL DEFAULT 0,
		location INTEGER NOT NULL DEFAULT 1,
		listnumber INTEGER NOT NULL DEFAULT 0,
		FOREIGN KEY (npcid) REFERENCES npcs(id),
		FOREIGN KEY (patternid) REFERENCES itempattern(patternid) ON DELETE CASCADE
	);

	-- npcs
	CREATE TABLE IF NOT EXISTS npcs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		npc_name TEXT,
		zone INTEGER,
		unk_a INTEGER,
		unk_b INTEGER,
		x_coord REAL,
		y_coord REAL,
		z_coord REAL,
		facing INTEGER,
		world INTEGER,
		dt_kill DATE,
		hp INTEGER,
		hp_percent INTEGER,
		modelid INTEGER,
		size REAL,
		unk_d INTEGER,
		unk_f INTEGER,
		unk_g INTEGER,
		primary_weapon INTEGER,
		secondary_weapon INTEGER,
		shield INTEGER,
		unk_h INTEGER,
		torso INTEGER,
		forearm INTEGER,
		gloves INTEGER,
		legs INTEGER,
		feet INTEGER,
		head INTEGER,
		hair_color INTEGER,
		hair_length INTEGER,
		hair_style INTEGER,
		face INTEGER,
		robe_style TEXT,
		race TEXT,
		unk_j INTEGER,
		npc_level INTEGER,
		npc_type INTEGER,
		npc_id INTEGER,
		unk_k INTEGER,
		unk_i TEXT,
		pd_icon INTEGER,
		unk_m INTEGER,
		unk_n INTEGER,
		unk_o INTEGER
	);

	-- quests, queststeps, charquests: initQuests (quests.go), which first
	-- moves Return Home's quests and playerquest aside

	-- spellpattern
	CREATE TABLE IF NOT EXISTS spellpattern (
		spellid INTEGER PRIMARY KEY AUTOINCREMENT,
		abilitylvl INTEGER NOT NULL,
		unk2 INTEGER NOT NULL,
		unk3 INTEGER NOT NULL,
		range INTEGER NOT NULL,
		casttime INTEGER NOT NULL,
		power INTEGER NOT NULL,
		iconColor INTEGER NOT NULL,
		icon INTEGER NOT NULL,
		scope INTEGER NOT NULL,
		recast INTEGER NOT NULL,
		eqprequire INTEGER NOT NULL,
		spellname TEXT NOT NULL,
		spelldesc TEXT NOT NULL
	);

	-- spelleffect: what each spell does, as EQOA-Data's spell lists word
	-- it, and the visual CastingCombat names for it (0: the server's
	-- stand-in); tools/import_spell_effects.py fills it
	CREATE TABLE IF NOT EXISTS spelleffect (
		spellid INTEGER PRIMARY KEY,
		line    TEXT NOT NULL,
		fxdict  INTEGER NOT NULL,
		source  TEXT NOT NULL
	);

	-- charbuffs: a character's spell effects kept over a logout, what is
	-- left of each counted while it is in the world (entity/buffsave.go)
	CREATE TABLE IF NOT EXISTS charbuffs (
		serverid   INTEGER NOT NULL,
		seq        INTEGER NOT NULL,
		spellid    INTEGER NOT NULL,
		name       TEXT NOT NULL,
		icon       INTEGER NOT NULL,
		caster     INTEGER NOT NULL,
		left_ms    INTEGER NOT NULL,
		tick_ms    INTEGER NOT NULL,
		hp_tick    INTEGER NOT NULL,
		power_tick INTEGER NOT NULL,
		mods       TEXT NOT NULL,
		flags      INTEGER NOT NULL,
		PRIMARY KEY (serverid, seq)
	);

	-- classspells: which spells a class has, for a new character given
	-- them all (world.start_all_spells); tools/import_class_spells.py fills it;
	-- a source "eqoa-data cm|epic|mca|master <name>" is a spell no level gives,
	-- never handed out (grantsource.go)
	CREATE TABLE IF NOT EXISTS classspells (
		class   INTEGER NOT NULL,
		spellid INTEGER NOT NULL,
		source  TEXT NOT NULL,
		PRIMARY KEY (class, spellid)
	);

	-- npcclass: each NPC name's class (WAR=0 ... ALC=14), which picks its
	-- melee swing (entity/rounds.go); tools/import_npc_classes.py fills it,
	-- by NPCClassKey
	CREATE TABLE IF NOT EXISTS npcclass (
		name   TEXT PRIMARY KEY,
		class  INTEGER NOT NULL,
		source TEXT NOT NULL
	);

	-- spells (player spellbook)
	CREATE TABLE IF NOT EXISTS spells (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		serverid INTEGER NOT NULL,
		spellid INTEGER NOT NULL,
		addedorder INTEGER NOT NULL,
		onHotBar INTEGER NOT NULL,
		whereonBar INTEGER NOT NULL,
		unk1 INTEGER NOT NULL,
		showhide INTEGER NOT NULL,
		FOREIGN KEY (serverid) REFERENCES characters(serverid) ON DELETE CASCADE,
		FOREIGN KEY (spellid) REFERENCES spellpattern(spellid)
	);
	CREATE INDEX IF NOT EXISTS idx_spells_serverid ON spells(serverid);

	-- weaponhotbar: Return Home's schema, never used (it has no set number);
	-- the weapon sets are gearsets
	CREATE TABLE IF NOT EXISTS weaponhotbar (
		weaponsetID INTEGER PRIMARY KEY AUTOINCREMENT,
		serverid INTEGER NOT NULL,
		hotbarname TEXT NOT NULL,
		weaponID INTEGER NOT NULL,
		secondaryID INTEGER NOT NULL,
		FOREIGN KEY (serverid) REFERENCES characters(serverid) ON DELETE CASCADE
	);

	-- quickslots: the inventory item prepared on each of the HUD's four item
	-- quick slots (0x000A)
	CREATE TABLE IF NOT EXISTS quickslots (
		serverid INTEGER NOT NULL,
		pos INTEGER NOT NULL,
		itemid INTEGER NOT NULL,
		PRIMARY KEY (serverid, pos),
		FOREIGN KEY (serverid) REFERENCES characters(serverid) ON DELETE CASCADE
	);

	-- gearsets: the HUD's four weapon sets (0x0021, 0x0022), each the main
	-- hand's and the off hand's charinventory.itemid and patternid (-1 none)
	CREATE TABLE IF NOT EXISTS gearsets (
		serverid INTEGER NOT NULL,
		slot INTEGER NOT NULL,
		name TEXT NOT NULL,
		mainitem INTEGER NOT NULL,
		mainpattern INTEGER NOT NULL,
		offitem INTEGER NOT NULL,
		offpattern INTEGER NOT NULL,
		PRIMARY KEY (serverid, slot),
		FOREIGN KEY (serverid) REFERENCES characters(serverid) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS guilds (
		guild_id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		leader_id INTEGER NOT NULL,
		motd TEXT DEFAULT '',
		crest_id INTEGER DEFAULT 0,
		tax_rate INTEGER DEFAULT 0,
		level INTEGER DEFAULT 1,
		bank_gold INTEGER DEFAULT 0,
		xp INTEGER DEFAULT 0,
		hall_zone INTEGER DEFAULT -1,
		hall_instance INTEGER DEFAULT -1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (leader_id) REFERENCES characters(serverid)
	);

	CREATE TABLE IF NOT EXISTS guild_members (
		guild_id INTEGER NOT NULL,
		player_id INTEGER NOT NULL,
		rank INTEGER DEFAULT 7,
		joined_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (guild_id, player_id),
		FOREIGN KEY (guild_id) REFERENCES guilds(guild_id) ON DELETE CASCADE,
		FOREIGN KEY (player_id) REFERENCES characters(serverid) ON DELETE CASCADE
	);

	-- factions: every faction and the standing a character starts with;
	-- charfactions: a character's own standings; factionhits: what killing an
	-- NPC of a name does to the killer's (factions.go, entity/faction.go)
	CREATE TABLE IF NOT EXISTS factions (
		name     TEXT PRIMARY KEY,
		standing INTEGER NOT NULL
	);
	CREATE TABLE IF NOT EXISTS charfactions (
		serverid INTEGER NOT NULL,
		faction  TEXT NOT NULL,
		standing INTEGER NOT NULL,
		PRIMARY KEY (serverid, faction)
	);
	CREATE TABLE IF NOT EXISTS factionhits (
		id       INTEGER PRIMARY KEY AUTOINCREMENT,
		npc_name TEXT NOT NULL,
		faction  TEXT NOT NULL,
		delta    INTEGER NOT NULL,
		source   TEXT NOT NULL DEFAULT 'gm',
		UNIQUE (npc_name, faction)
	);
	CREATE INDEX IF NOT EXISTS idx_factionhits_npc ON factionhits(npc_name);

	-- charmastery: a character's class mastery points, unspent and spent, the
	-- share of its experience going to them and the experience gathered
	-- toward the next (mastery.go, entity/mastery.go)
	CREATE TABLE IF NOT EXISTS charmastery (
		serverid INTEGER PRIMARY KEY,
		points   INTEGER NOT NULL DEFAULT 0,
		spent    INTEGER NOT NULL DEFAULT 0,
		percent  INTEGER NOT NULL DEFAULT 0,
		xp       INTEGER NOT NULL DEFAULT 0
	);
	-- charbind: where a spiritmaster bound a character (Return Home and a
	-- death take it there); no row, the login spawn
	CREATE TABLE IF NOT EXISTS charbind (
		serverid     INTEGER PRIMARY KEY,
		world        INTEGER NOT NULL,
		x            REAL NOT NULL,
		y            REAL NOT NULL,
		z            REAL NOT NULL,
		facing       REAL NOT NULL DEFAULT 0,
		spiritmaster TEXT NOT NULL DEFAULT ''
	);

	-- npcpaths: a path a GM gave an NPC (npcpath.go, entity/path.go, the GM
	-- panel's map): npcid is its npcs row (so the path outlives its deaths and
	-- a restart), points a JSON list of {x, y, z[, pause]} in its world, mode
	-- loop | pingpong | once, pause the seconds it stands at each point (a
	-- point's own pause first), speed walk | run
	CREATE TABLE IF NOT EXISTS npcpaths (
		npcid  INTEGER PRIMARY KEY,
		world  INTEGER NOT NULL DEFAULT 0,
		mode   TEXT NOT NULL DEFAULT 'loop',
		pause  REAL NOT NULL DEFAULT 0,
		speed  TEXT NOT NULL DEFAULT 'walk',
		points TEXT NOT NULL DEFAULT '[]'
	);

	-- Create indexes
	CREATE INDEX IF NOT EXISTS idx_characters_account ON characters(accountid);
	CREATE INDEX IF NOT EXISTS idx_characters_name ON characters(charName);
	CREATE INDEX IF NOT EXISTS idx_charinventory_serverid ON charinventory(serverid);
	CREATE INDEX IF NOT EXISTS idx_npcs_zone ON npcs(zone);
	CREATE INDEX IF NOT EXISTS idx_npcs_world ON npcs(world);
	`

	_, err := db.Exec(schema)
	if err != nil {
		return err
	}
	if err := db.seedFactions(); err != nil {
		return fmt.Errorf("factions: %w", err)
	}
	if err := db.initQuests(); err != nil {
		return fmt.Errorf("quests: %w", err)
	}
	if err := db.ensureNPCAggro(); err != nil {
		return err
	}
	if err := db.ensureNPCUndead(); err != nil {
		return err
	}
	if err := db.ensureNPCSocial(); err != nil {
		return err
	}
	if err := db.ensureItemEffects(); err != nil {
		return err
	}
	if err := db.ensureSpellFXSuggestions(); err != nil {
		return err
	}
	if err := db.ensureMapMarks(); err != nil {
		return err
	}
	if err := db.ensureNPCMove(); err != nil {
		return err
	}
	if err := db.ensureLootTables(); err != nil {
		return err
	}
	if err := db.ensureCMCatalog(); err != nil {
		return err
	}

	log.Printf("SQLite schema initialized")
	return nil
}

// Close closes the database connection.
func (db *SQLiteDB) Close() error {
	if db.DB != nil {
		return db.DB.Close()
	}
	return nil
}

// SQLiteCharacterStore handles character persistence with SQLite.
type SQLiteCharacterStore struct {
	db *SQLiteDB

	// newRules is what CreateNew gives a new character (newchar.go)
	rulesMu  sync.RWMutex
	newRules NewCharacterRules
}

// NewSQLiteCharacterStore creates a new character store.
func NewSQLiteCharacterStore(db *SQLiteDB) *SQLiteCharacterStore {
	return &SQLiteCharacterStore{db: db}
}

// DB exposes the underlying handle so callers can build sibling stores off the
// same connection without threading another argument through their constructors.
func (s *SQLiteCharacterStore) DB() *SQLiteDB { return s.db }

// CharacterRecord represents a character in the database.
// Fields match C# characters table exactly.
type CharacterRecord struct {
	CharName        string
	AccountID       int
	ServerID        uint32 // PRIMARY KEY AUTOINCREMENT
	ModelID         int64  // bigint in C#
	TClass          int
	Race            int
	HumType         int
	Level           int
	HairColor       int
	HairLength      int
	HairStyle       int
	FaceOption      int
	Sex             int
	TotalXP         int
	Debt            int
	Breath          int
	Tunar           int
	BankTunar       int
	UnusedTP        int
	TotalTP         int
	Speed           float32
	World           int
	X, Y, Z         float32
	Facing          float32
	Unknown         float32
	TPStrength      int
	TPStamina       int
	TPAgility       int
	TPDexterity     int
	TPWisdom        int
	TPIntel         int
	TPCharisma      int
	CurrentHP       int
	CurrentPower    int
	Unk42_1         int
	AC              int
	Unk42_2         int
	Unk42_3         int
	Unk42_4         int
	Unk42_5         int
	Unk42_6         int
	Unk42_7         int
	Unk42_8         int
	PoisonR         int
	DiseaseR        int
	FireR           int
	ColdR           int
	LightningR      int
	ArcaneR         int
	Fishing         int
	Unk42_9         int
	Unk42_10        int
	Unk42_11        int
	PlayerFlags     string
	CompletedQuests string
	ActiveQuests    string
}

// charColumns is the list of character columns for SELECT queries.
const charColumns = `charName, accountid, serverid, modelid, tclass, race, humType, level,
	haircolor, hairlength, hairstyle, faceoption, sex,
	totalXP, debt, breath, tunar, bankTunar, unusedTP, totalTP,
	Speed, world, x, y, z, facing, unknown,
	tpstrength, tpstamina, tpagility, tpdexterity, tpwisdom, tpintel, tpcharisma,
	currentHP, currentPower,
	unk42_1, ac, unk42_2, unk42_3, unk42_4, unk42_5, unk42_6, unk42_7, unk42_8,
	poisonr, diseaser, firer, coldr, lightningr, arcaner,
	fishing, unk42_9, unk42_10, unk42_11,
	player_flags, completed_quests, active_quests`

// Create creates a new character. ServerID is auto-assigned by the database.
func (s *SQLiteCharacterStore) Create(c *CharacterRecord) error {
	result, err := s.db.Exec(`
		INSERT INTO characters (
			charName, accountid, modelid, tclass, race, humType, level,
			haircolor, hairlength, hairstyle, faceoption, sex,
			totalXP, debt, breath, tunar, bankTunar, unusedTP, totalTP,
			Speed, world, x, y, z, facing, unknown,
			tpstrength, tpstamina, tpagility, tpdexterity, tpwisdom, tpintel, tpcharisma,
			currentHP, currentPower,
			unk42_1, ac, unk42_2, unk42_3, unk42_4, unk42_5, unk42_6, unk42_7, unk42_8,
			poisonr, diseaser, firer, coldr, lightningr, arcaner,
			fishing, unk42_9, unk42_10, unk42_11,
			player_flags, completed_quests, active_quests
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		c.CharName, c.AccountID, c.ModelID, c.TClass, c.Race, c.HumType, c.Level,
		c.HairColor, c.HairLength, c.HairStyle, c.FaceOption, c.Sex,
		c.TotalXP, c.Debt, c.Breath, c.Tunar, c.BankTunar, c.UnusedTP, c.TotalTP,
		c.Speed, c.World, c.X, c.Y, c.Z, c.Facing, c.Unknown,
		c.TPStrength, c.TPStamina, c.TPAgility, c.TPDexterity, c.TPWisdom, c.TPIntel, c.TPCharisma,
		c.CurrentHP, c.CurrentPower,
		c.Unk42_1, c.AC, c.Unk42_2, c.Unk42_3, c.Unk42_4, c.Unk42_5, c.Unk42_6, c.Unk42_7, c.Unk42_8,
		c.PoisonR, c.DiseaseR, c.FireR, c.ColdR, c.LightningR, c.ArcaneR,
		c.Fishing, c.Unk42_9, c.Unk42_10, c.Unk42_11,
		c.PlayerFlags, c.CompletedQuests, c.ActiveQuests,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ErrCharacterExists
		}
		return err
	}

	// ServerID is auto-assigned by AUTOINCREMENT
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	c.ServerID = uint32(id)

	return nil
}

// GetByServerID retrieves a character by serverid (the PRIMARY KEY, sent by the client).
func (s *SQLiteCharacterStore) GetByServerID(serverID uint32) (*CharacterRecord, error) {
	row := s.db.QueryRow(`SELECT `+charColumns+` FROM characters WHERE serverid = ?`, serverID)
	return s.scanCharacter(row)
}

// GetByName retrieves a character by name (case-insensitive).
func (s *SQLiteCharacterStore) GetByName(name string) (*CharacterRecord, error) {
	row := s.db.QueryRow(`SELECT `+charColumns+` FROM characters WHERE LOWER(charName) = LOWER(?)`, name)
	return s.scanCharacter(row)
}

// GetByAccountID retrieves all characters for an account.
func (s *SQLiteCharacterStore) GetByAccountID(accountID int) ([]*CharacterRecord, error) {
	rows, err := s.db.Query(`SELECT `+charColumns+` FROM characters WHERE accountid = ? ORDER BY serverid`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chars []*CharacterRecord
	for rows.Next() {
		c, err := s.scanCharacterRows(rows)
		if err != nil {
			return nil, err
		}
		chars = append(chars, c)
	}
	return chars, rows.Err()
}

// GetAll retrieves all characters.
func (s *SQLiteCharacterStore) GetAll() ([]*CharacterRecord, error) {
	rows, err := s.db.Query(`SELECT ` + charColumns + ` FROM characters ORDER BY serverid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chars []*CharacterRecord
	for rows.Next() {
		c, err := s.scanCharacterRows(rows)
		if err != nil {
			return nil, err
		}
		chars = append(chars, c)
	}
	return chars, rows.Err()
}

// NameExists checks if a character name exists (case-insensitive).
func (s *SQLiteCharacterStore) NameExists(name string) (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM characters WHERE LOWER(charName) = LOWER(?)`, name).Scan(&count)
	return count > 0, err
}

// Update updates a character.
func (s *SQLiteCharacterStore) Update(c *CharacterRecord) error {
	_, err := s.db.Exec(`
		UPDATE characters SET
			level = ?, x = ?, y = ?, z = ?, facing = ?, world = ?, Speed = ?,
			currentHP = ?, currentPower = ?,
			tpstrength = ?, tpstamina = ?, tpagility = ?, tpdexterity = ?,
			tpwisdom = ?, tpintel = ?, tpcharisma = ?,
			totalXP = ?, debt = ?, tunar = ?, bankTunar = ?, unusedTP = ?, totalTP = ?,
			ac = ?, poisonr = ?, diseaser = ?, firer = ?, coldr = ?, lightningr = ?, arcaner = ?,
			fishing = ?, player_flags = ?, completed_quests = ?, active_quests = ?
		WHERE serverid = ?
	`,
		c.Level, c.X, c.Y, c.Z, c.Facing, c.World, c.Speed,
		c.CurrentHP, c.CurrentPower,
		c.TPStrength, c.TPStamina, c.TPAgility, c.TPDexterity,
		c.TPWisdom, c.TPIntel, c.TPCharisma,
		c.TotalXP, c.Debt, c.Tunar, c.BankTunar, c.UnusedTP, c.TotalTP,
		c.AC, c.PoisonR, c.DiseaseR, c.FireR, c.ColdR, c.LightningR, c.ArcaneR,
		c.Fishing, c.PlayerFlags, c.CompletedQuests, c.ActiveQuests,
		c.ServerID,
	)
	return err
}

// SetProgress stores a character's level and its kept total experience
// (totalXP: entity.TotalXP), and nothing else of the row: what a GM's level
// change writes at once, without the place the logout save keeps.
func (s *SQLiteCharacterStore) SetProgress(serverID uint32, level, totalXP int) error {
	res, err := s.db.Exec(`UPDATE characters SET level = ?, totalXP = ? WHERE serverid = ?`, level, totalXP, serverID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n != 1 {
		return ErrCharacterNotFound
	}
	return nil
}

// SetTrainingPoints stores a character's unspent training points and every
// one it has been given (entity/training.go), and nothing else of the row.
func (s *SQLiteCharacterStore) SetTrainingPoints(serverID uint32, unused, total int) error {
	res, err := s.db.Exec(`UPDATE characters SET unusedTP = ?, totalTP = ? WHERE serverid = ?`, unused, total, serverID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n != 1 {
		return ErrCharacterNotFound
	}
	return nil
}

// Delete deletes a character by serverid.
func (s *SQLiteCharacterStore) Delete(serverID uint32) error {
	result, err := s.db.Exec(`DELETE FROM characters WHERE serverid = ?`, serverID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrCharacterNotFound
	}
	// its bind point with it: a server id made again must not inherit one
	if _, err := s.db.Exec(`DELETE FROM charbind WHERE serverid = ?`, serverID); err != nil {
		return err
	}
	return nil
}

// Count returns the total number of characters.
func (s *SQLiteCharacterStore) Count() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM characters`).Scan(&count)
	return count, err
}

func (s *SQLiteCharacterStore) CountByAccount(accountID uint32) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM characters WHERE accountid = ?`, accountID).Scan(&count)
	return count, err
}

// scanCharacter scans a single row into a CharacterRecord.
func (s *SQLiteCharacterStore) scanCharacter(row *sql.Row) (*CharacterRecord, error) {
	var c CharacterRecord
	err := row.Scan(
		&c.CharName, &c.AccountID, &c.ServerID, &c.ModelID, &c.TClass, &c.Race, &c.HumType, &c.Level,
		&c.HairColor, &c.HairLength, &c.HairStyle, &c.FaceOption, &c.Sex,
		&c.TotalXP, &c.Debt, &c.Breath, &c.Tunar, &c.BankTunar, &c.UnusedTP, &c.TotalTP,
		&c.Speed, &c.World, &c.X, &c.Y, &c.Z, &c.Facing, &c.Unknown,
		&c.TPStrength, &c.TPStamina, &c.TPAgility, &c.TPDexterity, &c.TPWisdom, &c.TPIntel, &c.TPCharisma,
		&c.CurrentHP, &c.CurrentPower,
		&c.Unk42_1, &c.AC, &c.Unk42_2, &c.Unk42_3, &c.Unk42_4, &c.Unk42_5, &c.Unk42_6, &c.Unk42_7, &c.Unk42_8,
		&c.PoisonR, &c.DiseaseR, &c.FireR, &c.ColdR, &c.LightningR, &c.ArcaneR,
		&c.Fishing, &c.Unk42_9, &c.Unk42_10, &c.Unk42_11,
		&c.PlayerFlags, &c.CompletedQuests, &c.ActiveQuests,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCharacterNotFound
		}
		return nil, err
	}
	return &c, nil
}

// scanCharacterRows scans a rows result into a CharacterRecord.
func (s *SQLiteCharacterStore) scanCharacterRows(rows *sql.Rows) (*CharacterRecord, error) {
	var c CharacterRecord
	err := rows.Scan(
		&c.CharName, &c.AccountID, &c.ServerID, &c.ModelID, &c.TClass, &c.Race, &c.HumType, &c.Level,
		&c.HairColor, &c.HairLength, &c.HairStyle, &c.FaceOption, &c.Sex,
		&c.TotalXP, &c.Debt, &c.Breath, &c.Tunar, &c.BankTunar, &c.UnusedTP, &c.TotalTP,
		&c.Speed, &c.World, &c.X, &c.Y, &c.Z, &c.Facing, &c.Unknown,
		&c.TPStrength, &c.TPStamina, &c.TPAgility, &c.TPDexterity, &c.TPWisdom, &c.TPIntel, &c.TPCharisma,
		&c.CurrentHP, &c.CurrentPower,
		&c.Unk42_1, &c.AC, &c.Unk42_2, &c.Unk42_3, &c.Unk42_4, &c.Unk42_5, &c.Unk42_6, &c.Unk42_7, &c.Unk42_8,
		&c.PoisonR, &c.DiseaseR, &c.FireR, &c.ColdR, &c.LightningR, &c.ArcaneR,
		&c.Fishing, &c.Unk42_9, &c.Unk42_10, &c.Unk42_11,
		&c.PlayerFlags, &c.CompletedQuests, &c.ActiveQuests,
	)
	return &c, err
}

// AccountRecord represents an account in the database.
// Matches C# accountinfo table.
type AccountRecord struct {
	ID           int
	Username     string
	PasswordHash string
	Email        string
	CreatedAt    time.Time
	LastLogin    time.Time
	Banned       bool
	BanReason    string
	Activated    bool
	AccessLevel  int
}

// SQLiteAccountStore handles account persistence with SQLite.
type SQLiteAccountStore struct {
	db *SQLiteDB
}

// NewSQLiteAccountStore creates a new account store.
func NewSQLiteAccountStore(db *SQLiteDB) *SQLiteAccountStore {
	return &SQLiteAccountStore{db: db}
}

// GetByUsername retrieves an account by username (case-insensitive).
func (s *SQLiteAccountStore) GetByUsername(username string) (*AccountRecord, error) {
	// acctstatus carries account state in one column: > 0 active, 0 inactive,
	// < 0 banned. There is no separate ban column.
	//
	// This previously selected acctstatus straight into Banned while ALSO
	// deriving Activated from it, so every account Create makes -- which inserts
	// acctstatus = 1 -- read back as both activated and banned. Nothing checked
	// Banned at the time, so it stayed hidden until the login path started
	// honouring it and refused every account.
	row := s.db.QueryRow(`
		SELECT accountid, username, COALESCE(CAST(pwhash AS TEXT), ''), email, acctcreation, lastlogin,
		       CASE WHEN acctstatus < 0 THEN 1 ELSE 0 END,
		       '',
		       CASE WHEN acctstatus > 0 THEN 1 ELSE 0 END,
		       COALESCE(acctlevel, 0)
		FROM accountinfo WHERE LOWER(username) = LOWER(?)
	`, username)

	var a AccountRecord
	var lastLogin sql.NullTime
	var createdAt sql.NullTime
	var email, banReason sql.NullString
	err := row.Scan(&a.ID, &a.Username, &a.PasswordHash, &email, &createdAt, &lastLogin, &a.Banned, &banReason, &a.Activated, &a.AccessLevel)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAccountNotFound
		}
		return nil, err
	}
	if lastLogin.Valid {
		a.LastLogin = lastLogin.Time
	}
	if createdAt.Valid {
		a.CreatedAt = createdAt.Time
	}
	if email.Valid {
		a.Email = email.String
	}
	if banReason.Valid {
		a.BanReason = banReason.String
	}
	return &a, nil
}

// AccessLevel is the account's acctlevel: 0 a player, 1 or more a GM (the
// GM commands, handlers/devcmds.go). Read on each use, so a change in the
// database counts at once, without a new login.
func (s *SQLiteAccountStore) AccessLevel(accountID int) (int, error) {
	var level int
	err := s.db.QueryRow(`SELECT COALESCE(acctlevel, 0) FROM accountinfo WHERE accountid = ?`, accountID).Scan(&level)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrAccountNotFound
		}
		return 0, err
	}
	return level, nil
}

// Create creates a new account. Password should already be hashed if needed.
func (s *SQLiteAccountStore) Create(username, passwordHash string) (*AccountRecord, error) {
	result, err := s.db.Exec(`
		INSERT INTO accountinfo (username, pwhash, acctstatus, acctlevel, acctcreation) VALUES (?, ?, 1, 0, CURRENT_TIMESTAMP)
	`, username, passwordHash)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, errors.New("username already exists")
		}
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &AccountRecord{
		ID:        int(id),
		Username:  username,
		Activated: true,
	}, nil
}

// GetOrCreate gets an existing account or creates a new one.
func (s *SQLiteAccountStore) GetOrCreate(username, password string) (*AccountRecord, error) {
	// Try to get existing account
	account, err := s.GetByUsername(username)
	if err == nil {
		// Account exists - update last login
		s.db.Exec(`UPDATE accountinfo SET lastlogin = CURRENT_TIMESTAMP WHERE accountid = ?`, account.ID)
		return account, nil
	}
	if !errors.Is(err, ErrAccountNotFound) {
		return nil, err
	}

	// Create new account
	log.Printf("Creating new account: %s", username)
	return s.Create(username, password)
}

// UpdateLastLogin updates the last login time for an account.
func (s *SQLiteAccountStore) UpdateLastLogin(accountID int) error {
	_, err := s.db.Exec(`UPDATE accountinfo SET lastlogin = CURRENT_TIMESTAMP WHERE accountid = ?`, accountID)
	return err
}

// SetBanned bans or unbans an account. acctstatus is the single account-state
// column: negative is banned, positive is active.
func (s *SQLiteAccountStore) SetBanned(accountID int, banned bool) error {
	status := 1
	if banned {
		status = -1
	}
	_, err := s.db.Exec(`UPDATE accountinfo SET acctstatus = ? WHERE accountid = ?`, status, accountID)
	return err
}

// UpdatePasswordHash replaces the stored credential for an account. Used both
// for password changes and to upgrade a legacy plaintext password to bcrypt the
// first time its owner logs in.
func (s *SQLiteAccountStore) UpdatePasswordHash(accountID int, passwordHash string) error {
	_, err := s.db.Exec(`UPDATE accountinfo SET pwhash = ? WHERE accountid = ?`, passwordHash, accountID)
	return err
}

// CharacterModelStore provides lookups for the charactermodel table.
type CharacterModelStore struct {
	db *SQLiteDB
}

// NewCharacterModelStore creates a new character model store.
func NewCharacterModelStore(db *SQLiteDB) *CharacterModelStore {
	return &CharacterModelStore{db: db}
}

// GetModelID looks up the model ID for a given race and sex.
func (s *CharacterModelStore) GetModelID(race, sex int) (int64, error) {
	var modelID int64
	err := s.db.QueryRow(`SELECT modelid FROM charactermodel WHERE race = ? AND sex = ?`, race, sex).Scan(&modelID)
	if err != nil {
		return 0, err
	}
	return modelID, nil
}
