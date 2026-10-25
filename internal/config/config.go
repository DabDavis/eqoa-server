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
}

// ServerConfig holds network server settings.
type ServerConfig struct {
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
	// 0: 15; below 0, none wander). entity/corpse.go, entity/roam.go.
	CorpseDecay  time.Duration `yaml:"corpse_decay"`
	RespawnAfter time.Duration `yaml:"respawn_after"`
	RoamRadius   float32       `yaml:"roam_radius"`
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
	// game: the first window keeps playing, and multiboxing is stopped.
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
