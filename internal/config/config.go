package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all server configuration.
type Config struct {
	Server ServerConfig `yaml:"server"`
	World  WorldConfig  `yaml:"world"`
	Patch  PatchConfig  `yaml:"patch"`
	Auth   AuthConfig   `yaml:"auth"`
	GM     GMConfig     `yaml:"gm"`

	// Shards is the server list every client is sent (0x07B3): each entry a
	// game server of its own, and CLIENT lets its player pick (the user,
	// 2026-11-07: "lets add another shard"). Unset: this server alone, as
	// "Marr's Fist", on server.game_port. Every process of a set of shards
	// carries the same list, so whichever a client asks shows the same.
	Shards []ShardConfig `yaml:"shards"`
}

// ShardConfig is a line of the server list.
type ShardConfig struct {
	// Name is what the list shows: 31 characters at most (CLIENT's m_name
	// holds 32 with its terminator). The list is shown sorted by name.
	Name string `yaml:"name"`

	// Port is the shard's game port (UDP), reached at IP.
	Port int `yaml:"port"`

	// Recommended makes it the one a client with no home shard in the list
	// starts on, marked Recommended (VIShardInfo::FirstValid).
	Recommended bool `yaml:"recommended"`

	// IP is its public address; unset: server.server_ip.
	IP string `yaml:"ip"`
}

// GMConfig is the GM panel (internal/gmweb): a web page, and the same work
// for an AI agent (MCP and OpenAPI), for accounts whose acctlevel is
// MinLevel or more. Off unless Port is set.
type GMConfig struct {
	// Port is its HTTP port; 0 turns it off.
	Port int `yaml:"port"`

	// Bind is the address it listens on. Loopback by default: it is meant to
	// be reached through a proxy that adds TLS (nginx), never in the clear.
	Bind string `yaml:"bind"`

	// PublicURL is where GMs and AI clients reach it, without a trailing
	// slash ("https://example.org/eqoa/gm"): the OAuth issuer, the cookie's
	// path, and whether the cookie is Secure. Empty: http on Bind:Port, as
	// for a panel used on the machine itself.
	PublicURL string `yaml:"public_url"`

	// MinLevel is the acctlevel that may sign in (the in-game GM commands'
	// 1); AdminLevel the one for the server's own settings, raw SQL,
	// backups, a restart and other accounts' levels.
	MinLevel   int `yaml:"min_level"`
	AdminLevel int `yaml:"admin_level"`

	// RequireTOTP makes every GM enrol an authenticator app before the
	// panel does anything for them.
	RequireTOTP bool `yaml:"require_totp"`

	// LogPath is the server's log the panel shows the end of; BackupDir
	// where its database backups go.
	LogPath   string `yaml:"log_path"`
	BackupDir string `yaml:"backup_dir"`

	// SessionIdle ends a signed-in browser session left alone this long.
	SessionIdle time.Duration `yaml:"session_idle"`

	// TokenRate is how many requests a minute one API token (an AI agent's)
	// may make.
	TokenRate int `yaml:"token_rate"`

	// PreviewURL is the spell-visual preview the Spell visuals page frames
	// (eqoa-rust-client-vanilla's web/fxview.html, served with its play
	// page: "https://example.org/eqoa/play/fxview.html"): a human casting a
	// visual as 0x00B1 plays it, off the GM's own disc. Empty: no preview.
	// Its host must allow the panel to frame it (frame-ancestors).
	PreviewURL string `yaml:"preview_url"`
}

// ServerConfig holds network server settings.
type ServerConfig struct {
	// GamePort is the game's UDP port. AuthPort (the Station login, TCP),
	// PatchPort (the patcher's HTTP) and AccountPort (the account pages):
	// 0 starts no such listener, for a shard another process logs its players
	// in for (auth.shared_db). A key left out is its default, not 0.
	GamePort    int `yaml:"game_port"`
	AuthPort    int `yaml:"auth_port"`
	PatchPort   int `yaml:"patch_port"`
	AccountPort int `yaml:"account_port"`

	BindAddress string `yaml:"bind_address"`

	// ServerIP is the address the CLIENT is told to connect to. It goes into
	// the server list and into the patch XML, so it must be reachable from the
	// PS2 / PCSX2 host — not "localhost", and not necessarily BindAddress.
	ServerIP string `yaml:"server_ip"`

	// DBPath is the SQLite database file.
	DBPath string `yaml:"db_path"`

	// ReadTimeout and WriteTimeout apply to the patch server's HTTP listener.
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`

	// GCPercent is the garbage collector's GOGC: how far the heap may grow past
	// what is live before a collection (unset or 0: Go's own, 100, the variable
	// GOGC saying otherwise). A server's live heap is nearly all its NPCs, there
	// from its start: at 100 it holds about as much again of nothing, at 25 a
	// quarter, for a collection every 30 s rather than 2 minutes with six players.
	GCPercent int `yaml:"gc_percent"`

	// NoHugePages keeps the kernel from backing this process with transparent
	// huge pages (prctl PR_SET_THP_DISABLE, Linux). With THP on "always", as
	// Debian ships it, khugepaged fills every 2 MB stretch the heap has touched
	// any of, undoing what the runtime gives back: 60 MB of an idle server of 150.
	NoHugePages bool `yaml:"no_huge_pages"`

	// PacketLog writes the packet path's debug lines: every packet received
	// and sent, its DRDP and state channel work (internal/debuglog). Off by
	// default: about 100 lines a second a player. cmd/movereplay reads the
	// "UDP raw hex" lines, so a log for it is written with this on; a GM's
	// #packetlog turns it on or off while the server runs.
	PacketLog bool `yaml:"packet_log"`
}

// WorldConfig holds world/zone settings.
type WorldConfig struct {
	TickRate           time.Duration `yaml:"tick_rate"`
	SessionTimeoutPre  time.Duration `yaml:"session_timeout_pre"`
	SessionTimeoutGame time.Duration `yaml:"session_timeout_game"`

	// NPCText is what an NPC says when talked to, by its name: a sign post or
	// a greeter, in place of the debug box every other NPC opens.
	NPCText map[string]string `yaml:"npc_text"`

	// StartLevel is the level a new character is created at (1 when unset);
	// the level a client's creation message carries is never taken.
	// StartAllSpells gives a new character every spell its class has
	// (classspells, tools/import_class_spells.py). Both are for testing.
	StartLevel     int  `yaml:"start_level"`
	StartAllSpells bool `yaml:"start_all_spells"`

	// MaxEntitySlots is the most NPCs and players (the player's own among
	// them) a client that keeps more than CLIENT's 24 may have in view
	// (handlers/entityslots.go): unset or 0 for every state channel the
	// protocol has (204), 24 for none past CLIENT's. MaxViewDistance is the
	// farthest such a client may have them looked for: unset or 0 for 1000,
	// 300 (every client's) for no farther. A PS2 always has 24 within 300.
	MaxEntitySlots  int `yaml:"max_entity_slots"`
	MaxViewDistance int `yaml:"max_view_distance"`

	// CorpseDecay is how long a creature's corpse lies when nothing empties
	// it (unset or 0: 3m); RespawnAfter how long after its corpse goes, so
	// decayed or emptied and closed, the creature comes back (unset or 0:
	// 60s). RoamRadius is how far from its spawn a creature wanders (unset or
	// 0: 70, Sandstorm's; below 0, none wander). entity/corpse.go, entity/roam.go.
	CorpseDecay  time.Duration `yaml:"corpse_decay"`
	RespawnAfter time.Duration `yaml:"respawn_after"`
	RoamRadius   float32       `yaml:"roam_radius"`

	// Aggro is whether creatures attack a player on sight (entity/aggro.go):
	// unset, they do; false, none attacks first.
	Aggro *bool `yaml:"aggro"`

	// FactionAggro is whether an NPC of a faction (table npcfaction) attacks
	// and shows its face by a player's standing with it (entity/aggro.go):
	// unset, it does; false, every NPC by the rules of no faction.
	FactionAggro *bool `yaml:"faction_aggro"`

	// The sun on a vampire (table charvampire, entity/vampire.go): it takes
	// VampireSunDamage (unset 50, Sandstorm's; 0 none) every
	// VampireSunInterval (unset or 0: 6s, the regeneration's tick) while its
	// client shows the sun up (6:00 to 18:00). VampireSunShade false burns it
	// under a roof too (unset: shade shelters it); VampireSunGMs false spares
	// a GM's vampire (unset: it burns as anyone's). The GM panel's settings
	// change all four while the server runs.
	VampireSunDamage   *int          `yaml:"vampire_sun_damage"`
	VampireSunInterval time.Duration `yaml:"vampire_sun_interval"`
	VampireSunShade    *bool         `yaml:"vampire_sun_shade"`
	VampireSunGMs      *bool         `yaml:"vampire_sun_gms"`

	// The towns' boon (entity/boon.go): talking to a town's announcement
	// board gives the player BoonXPPct percent of every experience award
	// (unset 200: double; 100, the boards give none) for BoonDuration of
	// play (unset or 0: 4h). The GM panel's settings change both while the
	// server runs.
	BoonXPPct    *int          `yaml:"boon_xp_pct"`
	BoonDuration time.Duration `yaml:"boon_duration"`

	// RestartNotice is the line every player in the world is sent when the
	// server stops (SIGINT or SIGTERM, or the GM panel's restart), in a GM
	// broadcast's colour, RestartNoticeWait (unset or 0: 5s) before the game
	// is told the server is going (0x07C0) and goes back to its login, so it
	// can be read. Unset: no line and no wait. Neither waits with nobody in
	// the world.
	RestartNotice     string        `yaml:"restart_notice"`
	RestartNoticeWait time.Duration `yaml:"restart_notice_wait"`

	// ReportNotice false leaves out the line a login is sent on how to report
	// a bug (#bug, /bug, F3 and Support: handlers/bugreport.go); unset: sent,
	// once a session, a few seconds after the player is in the world.
	ReportNotice *bool `yaml:"report_notice"`

	// Campfix false turns the players' camp reports off (/campfix, /mark,
	// /markadd and /markremove: handlers/campfix.go): every one of them then
	// answers that they are off, and a GM's own /mark (gmcamps.go) is as
	// ever. Unset: on.
	Campfix *bool `yaml:"campfix"`

	// MoveCheck is what the server does with a player's report of its own
	// place (its 0x40) that its moves cannot have reached: in the air with
	// nothing under it, under the ground, through a wall, faster than its
	// speed (entity/movecheck.go). "off" takes every report, as before;
	// "log" (unset) takes every report and logs the rest ("MOVECHECK"
	// lines); "enforce" leaves the player where it last was right and moves
	// the client back. #movecheck changes it while the server runs.
	MoveCheck string `yaml:"movecheck"`

	// GroundZones is how many zones of the ground files stay loaded at once
	// (and as many of the walls files'), the least lately used let go past
	// it: unset or 0, every zone once read (all six worlds: 652 MB at most,
	// 2.4 MB a zone on average). A small machine may cap it; players spread
	// over more zones than it holds make the server read zones over and over
	// (internal/ground's Ground.keep).
	GroundZones int `yaml:"ground_zones"`

	// PvP lets players flag themselves for PvP with /pvp (entity/pvp.go); unset
	// or false, /pvp says it is not available and nothing else changes.
	PvP bool `yaml:"pvp"`

	// Jump lets players jump (entity/jump.go, handlers/jump.go): a client that
	// can (vanilla) is offered it (protocol.OpcodeJump) and jumps about 0.9 up,
	// and the movement check takes a jump's arc from it. Unset or false: nothing
	// is offered, and every rise in the air is judged as before. A PS2 cannot
	// jump and is never offered it.
	Jump bool `yaml:"jump"`

	// Heirlooms false gives no character the kept items at its login: the
	// Genie's Lamp, the Heirloom Fang of the Wolf and the Summoning Portal
	// (handlers/genie.go, wolffang.go, summonportal.go). Unset: given, to a
	// character holding none, while their records exist. The records, the
	// Heirloom Outfitters and the start rows are data: a server without them
	// has none in its database as well.
	Heirlooms *bool `yaml:"heirlooms"`

	// OpenRaces offers at character creation, beside the game's own, the
	// froglok (race 10) and every race of table racedefs (entity/creation.go):
	// a client that can show them (vanilla) is told them before the world
	// (protocol.OpcodeCreationOffer), and the server takes them. AllClasses
	// offers every class to every race. Unset, both off: the game's races and
	// classes, as before, and a racedefs race is refused at creation. A PS2
	// creates as ever: its pages are the disc's.
	OpenRaces  bool `yaml:"open_races"`
	AllClasses bool `yaml:"all_classes"`

	// PackSlots and BankSlots are how many records the pack (worn items among
	// them, as CLIENT counts its 40) and the bank hold for a client that holds
	// more than CLIENT's 40 (vanilla), which is offered them after the first
	// character list (protocol.OpcodeItemLists, handlers/itemlists.go). Unset or
	// 40 or less: 40, nothing offered; at most 255. A PS2 always holds 40, and a
	// character with more is sent its first 40, the rest kept for it
	// (charinventory locations 2 and 3) until a client that holds them logs in.
	PackSlots int `yaml:"pack_slots"`
	BankSlots int `yaml:"bank_slots"`

	// Capes is an equip slot of this server's own, a cape's (entity/capes.go,
	// handlers/capes.go): items of wear slot 28 worn in equip slot 22, each
	// drawn in its record's style (model) by a client that draws capes
	// (vanilla), which is offered them after the first character list
	// (protocol.OpcodeCapes) and told who in its view wears which. Every other
	// client (a PS2) is sent a cape as a plain item, never worn. Unset or
	// false: no offer, and no cape can be worn.
	Capes bool `yaml:"capes"`

	// ImportedTag shows "(Imported)" after the name of gear a character
	// transfer brought in (its record a copy at world.imported_share: database/imported.go;
	// handlers/importedtag.go). Unset, off: the copies show their bases' names.
	ImportedTag bool `yaml:"imported_tag"`
	// ImportedShare is the share of its record's stats, procs and worn hit
	// effects that gear a transfer brought in keeps (0.85 = 85%), each copy
	// made its base again at this share at the start. Unset: 0.85.
	ImportedShare float64 `yaml:"imported_share"`

	// RockOfFreeport gives every character the Rock of Freeport
	// (handlers/rockfreeport.go): the server makes its record at its start
	// when none is so named, a character holding none is given one in its pack
	// at its login (told once in the world when the pack is full), and its Use
	// is a 2 s cast in Return Home's visual that sends its user a few steps in
	// front of Dabrish. Unset or false: none of it, and a stone held does
	// nothing. The Heirloom Test Server sets it.
	RockOfFreeport bool `yaml:"rock_of_freeport"`

	// TestInvis turns on /testinvis, every player's safe mode
	// (handlers/safemode.go, entity/safemode.go; petition 1424): typed, no
	// creature attacks the character on sight until it is typed again, and a
	// fight it starts works as any other. Unset or false: the command says it
	// is off, and a character left in safe mode leaves it at its next login.
	// The Heirloom Test Server sets it.
	TestInvis bool `yaml:"testinvis"`

	// Teleporter turns on the Teleporter (handlers/teleporter.go): an NPC of
	// that name (tools/place_teleporter.py stands one beside Dabrish) sends a
	// player to every raid's way in and beside every XP camp, a box at a time,
	// for nothing. Unset or false: it says its portals are closed. The
	// Heirloom Test Server sets it.
	Teleporter bool `yaml:"teleporter"`

	// Mercs turns on the mercenaries (handlers/merc.go, entity/merc.go): a
	// Mercenary Liaison (tools/place_merc_liaisons.py stands one beside each
	// city's announcements board and Dabrish) hires one for nothing, a player
	// of any class and race of its owner's level in heirlooms, a member of its
	// group, commanded by /merc; the hire is kept until dismissed. Unset or
	// false: a liaison says nothing and /merc says mercenaries are off. The
	// Heirloom Test Server sets it.
	Mercs bool `yaml:"mercs"`

	// Familiars turns on the familiars (handlers/familiar.go,
	// entity/familiar.go): an item of table familiars calls its familiar with a
	// cast (tools/seed_familiars.py: Healbot, sold by the Genie), and
	// /familiar dismiss sends it away. Unset or false: the item is quiet. The
	// Heirloom Test Server sets it.
	Familiars bool `yaml:"familiars"`

	// QuestMarksBelow is how many levels below the player's a quest's lowest
	// level may be for its giver's "!" on a client's map
	// (handlers/questmarks.go; a quest's name is marked once, at its nearest
	// giver, whatever this is). Unset or 0: 10, so a level 60 is shown the
	// quests for 50 and up (of the 150 or so it may take, three in five are for
	// below 50); a number, that many; -1, every quest the player may take,
	// however low.
	QuestMarksBelow int `yaml:"quest_marks_below"`
}

// PatchConfig selects which client patch payloads the patch server hands out.
//
// The corresponding EQOA_SS_FILE / EQOA_S2_FILE environment variables still
// override these, and deliberately take precedence: swapping a payload already
// requires a restart for the client's two-pass Delete dance, so a bisect should
// never also require a rebuild or a config edit.
//
// SDFile IS DEAD. SD is not served — EQOAHDD.ELF delivers it — so the field and
// EQOA_SD_FILE steer nothing. Setting either is LOGGED by payloadOverrides
// rather than ignored: a key left in a config.yaml written before the move would
// otherwise look like it was still choosing a payload. The field is kept so an
// existing config.yaml still parses.
type PatchConfig struct {
	// Dir holds the payloads. Relative paths resolve against the working
	// directory, then against the directory containing the executable.
	Dir string `yaml:"dir"`

	// Each must be a bare filename within Dir, not a path.
	//
	// EMPTY MEANS "the built-in default", which lives beside the code that
	// knows what each payload IS — defaultSSFile / defaultS2File /
	// defaultSDFile in internal/network/tcp/patchserver.go. DefaultConfig
	// deliberately leaves all three blank rather than repeating the names:
	// while it did repeat them, its copy of sd_file drifted to "SD", the
	// retired memory-card build, and any server that simply omitted the key
	// got the one payload with no hard-disk support at all — silently, since
	// a wrong-but-readable payload logs exactly like a right one.
	SSFile string `yaml:"ss_file"`
	S2File string `yaml:"s2_file"`
	SDFile string `yaml:"sd_file"`

	// WorldsDir holds world ESFs served over HTTP to the in-game HDD stager
	// (/worlds/...), NOT to the patcher. The patcher can only ever write to
	// mc0:/BASLUS-20744EQONLINE/, and it runs before pfs0: is mounted, so a
	// world can only reach the hard disk from inside the running game.
	//
	// Kept separate from Dir on purpose: everything in Dir is offered to the
	// memory card, where a 6 MB world would not fit and has no business being.
	WorldsDir string `yaml:"worlds_dir"`

	// WorldsUDPPort is where the in-game stager fetches world chunks. It is a
	// separate listener from the patch server's HTTP port because the consumer
	// is different: the HTTP route is driven by the patcher's own client, which
	// is unloaded by the time our payload exists, so the stager speaks a
	// datagram protocol of our own instead. 0 disables it.
	WorldsUDPPort int `yaml:"worlds_udp_port"`

	// News is what STATION's news box says after Play EQOA, on the way to the
	// server list, plain text ('\n' breaks a line); unset: "Welcome to Spire.".
	// Every client reads it there, so it is the place to say the list has changed.
	News string `yaml:"news"`
}

// AuthConfig holds authentication settings.
type AuthConfig struct {
	// Accounts are created in the database at startup if they do not already
	// exist, with their password hashed. An existing account is left alone, so
	// editing a password here does not reset one already in the database.
	Accounts []AccountSeed `yaml:"accounts"`

	// BCryptCost is the work factor for password hashing. Valid range is 4-31;
	// anything outside it falls back to bcrypt's default of 10.
	BCryptCost int `yaml:"bcrypt_cost"`

	// AllowAutoRegister makes an unknown username register itself on first
	// login, adopting the password it presented. Convenient for a private
	// server; it means anyone who can reach the auth port can claim any name
	// that is not already taken.
	//
	// It also governs accounts that exist with no password set at all — those
	// were created by the UDP path before password checking existed, and are
	// adopted the same way by the next login that supplies one.
	AllowAutoRegister bool `yaml:"allow_auto_register"`

	// RequireTCPLogin makes the UDP handlers refuse a username that has not
	// completed a TCP login recently. Without it, password checking is
	// decorative: the UDP path takes the username from the packet, so a client
	// can skip the auth server entirely and name any account.
	RequireTCPLogin bool `yaml:"require_tcp_login"`

	// TicketTTL is how long a completed TCP login stays valid for the UDP
	// handshake. It needs to cover a human sitting on the server-select and
	// character-select screens.
	TicketTTL time.Duration `yaml:"ticket_ttl"`

	// OneLogin refuses a login while the same account, or (for a client that
	// is not exempt) the same address, already has a live session in the
	// game: the first window keeps playing, and multiboxing is stopped. Off,
	// an account and an address may play several windows; a character is in
	// one window at a time either way (a second window of its account
	// choosing it takes it over, entity/takeover.go; another account's is
	// refused: HandleSelectedChar).
	OneLogin bool `yaml:"one_login"`

	// OneLoginExempt lists addresses (IPs or CIDR prefixes) the address rule
	// does not apply to: the operators' own, and the web relay's, through
	// which every browser player arrives (wsrelay limits those itself, by the
	// page's address). Loopback and server.server_ip are always exempt. The
	// account rule applies to everyone.
	OneLoginExempt []string `yaml:"one_login_exempt"`

	// RelayLookup is wsrelay's -lookup listener ("http://127.0.0.1:8792"):
	// asked which page is behind a relayed login or session, so the address
	// rule sees a browser player's own address. Empty: relayed players share
	// the relay's address, and it must then be exempt (server_ip is).
	RelayLookup string `yaml:"relay_lookup"`

	// RelayAddrs are the source addresses the relay reaches this server from
	// (default: loopback and server.server_ip).
	RelayAddrs []string `yaml:"relay_addrs"`

	// BrowserCharacters is the most of an account's characters in the game
	// through the web relay at once (0: no limit). A native client is not
	// counted. It needs relay_lookup, which says relayed players are named.
	BrowserCharacters int `yaml:"browser_characters"`

	// SharedDB is another shard's database, whose accountinfo is the accounts
	// of this one too (database.AccountMirror): this server's own table is a
	// copy of it, refreshed as a player signs in and every SharedRefresh
	// (unset or 0: 10 s), never written back. With it set a client signs in
	// here by the password its handshake's token carries, every time: no
	// login ticket comes from another process, and none is made here. Read
	// only; empty on the shard that owns the accounts.
	SharedDB      string        `yaml:"shared_db"`
	SharedRefresh time.Duration `yaml:"shared_refresh"`
}

// AccountSeed is a pre-registered account.
type AccountSeed struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			GamePort:     10070,
			AuthPort:     9735,
			PatchPort:    7000,
			AccountPort:  8080,
			BindAddress:  "0.0.0.0",
			ServerIP:     "127.0.0.1",
			DBPath:       "eqoa.db",
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		},
		World: WorldConfig{
			TickRate:           100 * time.Millisecond, // 10 Hz
			SessionTimeoutPre:  45 * time.Second,
			SessionTimeoutGame: 2 * time.Second,
		},
		Patch: PatchConfig{
			// The three payload names are left empty on purpose; see
			// PatchConfig. The patch server's own defaults answer for them.
			Dir:           "patches",
			WorldsDir:     "worlds",
			WorldsUDPPort: 7003,
		},
		GM: GMConfig{
			Bind:        "127.0.0.1",
			MinLevel:    1,
			AdminLevel:  2,
			LogPath:     "gameserver.log",
			BackupDir:   "backups",
			SessionIdle: 2 * time.Hour,
			TokenRate:   240,
		},
		Auth: AuthConfig{
			BCryptCost: 10,
			// Defaults suit a private server on a trusted LAN: names register
			// themselves, but a UDP client still has to have logged in.
			AllowAutoRegister: true,
			RequireTCPLogin:   true,
			TicketTTL:         15 * time.Minute,
			OneLogin:          true,
		},
	}
}

// Load reads configuration from a YAML file.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return cfg, nil
}

// LoadOrDefault loads config from path, or returns defaults if file doesn't exist.
func LoadOrDefault(path string) (*Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return DefaultConfig(), nil
	}
	return Load(path)
}

// Save writes the configuration to a YAML file.
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
