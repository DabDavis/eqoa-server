package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/eqoa/server/internal/auth"
	"github.com/eqoa/server/internal/bootelf"
	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/gmweb"
	"github.com/eqoa/server/internal/handlers"
	"github.com/eqoa/server/internal/network/tcp"
	"github.com/eqoa/server/internal/network/udp"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/server/internal/worlds"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "config.yaml", "Path to config file")
	flag.Parse()

	log.Println("EQOA Game Server starting...")

	// Load configuration
	cfg, err := config.LoadOrDefault(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize SQLite database
	db, err := database.NewSQLite(cfg.Server.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()
	log.Printf("SQLite database initialized (%s)", cfg.Server.DBPath)

	// Create database stores
	charStore := database.NewSQLiteCharacterStore(db)
	// What a new character starts with (for testing: world.start_level and
	// world.start_all_spells); the account page's store is given the same
	newRules := database.NewCharacterRules{Level: cfg.World.StartLevel, AllSpells: cfg.World.StartAllSpells}
	charStore.SetNewCharacterRules(newRules)
	if cfg.World.StartLevel > 1 || cfg.World.StartAllSpells {
		log.Printf("New characters: level %d, all their class's spells: %v", max(1, min(60, cfg.World.StartLevel)), cfg.World.StartAllSpells)
	}
	accountStore := database.NewSQLiteAccountStore(db)
	inventoryStore := database.NewInventoryStore(db)
	spellbookStore := database.NewSpellbookStore(db)

	// Initialize game world
	gameWorld := entity.NewWorld()

	// Load NPCs from eqoa.db
	npcStore := database.NewNPCStore(db)
	gameWorld.LoadNPCs(npcStore)

	log.Printf("Entity world: %d players, %d NPCs", gameWorld.PlayerCount(), gameWorld.NPCCount())

	// Initialize session manager
	sessionMgr := session.NewManager()
	sessionMgr.SetOnReplaced(handlers.SendLoggedInElsewhere)
	sessionMgr.SetTimeouts(cfg.World.SessionTimeoutPre, cfg.World.SessionTimeoutGame)

	// Start session cleanup routine
	stopCleanup := sessionMgr.StartCleanupRoutine(cfg.World.SessionTimeoutPre)
	defer stopCleanup()

	// Initialize handler registry
	handlerRegistry := handlers.NewRegistry()
	handlers.RegisterDefaultHandlers(handlerRegistry, charStore, accountStore, sessionMgr, cfg.Server.ServerIP, gameWorld, inventoryStore, spellbookStore)

	// Create UDP handler registry and register handlers.
	// Fallback to handlerRegistry for opcodes not explicitly bridged.
	udpHandlers := udp.NewHandlerRegistry()
	udpHandlers.SetFallback(handlerRegistry)

	// Register handlers with the UDP registry
	udpHandlers.Register(protocol.OpcodeAuthenticate, func(sess *session.Session, msg *protocol.Message) {
		handlerRegistry.Handle(sess, msg)
	})
	udpHandlers.Register(protocol.OpcodeAuthenticate2, func(sess *session.Session, msg *protocol.Message) {
		handlerRegistry.Handle(sess, msg)
	})
	udpHandlers.Register(protocol.OpcodeDiscVersion, func(sess *session.Session, msg *protocol.Message) {
		handlerRegistry.Handle(sess, msg)
	})
	udpHandlers.Register(protocol.OpcodeCreateCharacter, func(sess *session.Session, msg *protocol.Message) {
		handlerRegistry.Handle(sess, msg)
	})
	udpHandlers.Register(protocol.OpcodeSelectedChar, func(sess *session.Session, msg *protocol.Message) {
		handlerRegistry.Handle(sess, msg)
	})
	udpHandlers.Register(protocol.OpcodeRandomName, func(sess *session.Session, msg *protocol.Message) {
		handlerRegistry.Handle(sess, msg)
	})
	udpHandlers.Register(protocol.OpcodeCharacterInWorld, func(sess *session.Session, msg *protocol.Message) {
		handlerRegistry.Handle(sess, msg)
	})
	udpHandlers.Register(protocol.OpcodeClientSayChat, func(sess *session.Session, msg *protocol.Message) {
		handlerRegistry.Handle(sess, msg)
	})
	udpHandlers.Register(protocol.OpcodeClientShout, func(sess *session.Session, msg *protocol.Message) {
		handlerRegistry.Handle(sess, msg)
	})
	udpHandlers.Register(protocol.OpcodeChangeChatMode, func(sess *session.Session, msg *protocol.Message) {
		handlerRegistry.Handle(sess, msg)
	})
	udpHandlers.Register(protocol.OpcodeTarget, func(sess *session.Session, msg *protocol.Message) {
		handlerRegistry.Handle(sess, msg)
	})
	udpHandlers.Register(protocol.OpcodeClassicTargetInfo, func(sess *session.Session, msg *protocol.Message) {
		handlerRegistry.Handle(sess, msg)
	})
	udpHandlers.Register(protocol.OpcodeAttack, func(sess *session.Session, msg *protocol.Message) {
		handlerRegistry.Handle(sess, msg)
	})
	udpHandlers.Register(protocol.OpcodeInteract, func(sess *session.Session, msg *protocol.Message) {
		handlerRegistry.Handle(sess, msg)
	})
	udpHandlers.Register(protocol.OpcodeDialogueBoxOption, func(sess *session.Session, msg *protocol.Message) {
		handlerRegistry.Handle(sess, msg)
	})
	udpHandlers.Register(protocol.OpcodeWhoList, func(sess *session.Session, msg *protocol.Message) {
		handlerRegistry.Handle(sess, msg)
	})
	udpHandlers.Register(protocol.OpcodeUpdateTrainingPoints, func(sess *session.Session, msg *protocol.Message) {
		handlerRegistry.Handle(sess, msg)
	})
	udpHandlers.Register(protocol.OpcodeClientFaction, func(sess *session.Session, msg *protocol.Message) {
		handlerRegistry.Handle(sess, msg)
	})
	udpHandlers.Register(protocol.OpcodeFactionMessages, func(sess *session.Session, msg *protocol.Message) {
		handlerRegistry.Handle(sess, msg)
	})
	udpHandlers.Register(protocol.OpcodeLootMessages, func(sess *session.Session, msg *protocol.Message) {
		handlerRegistry.Handle(sess, msg)
	})

	// Seed the auth account table from config before the listener starts.
	tcp.SeedAccounts(accountStore, cfg.Auth.Accounts, cfg.Auth.BCryptCost)

	handlers.SetNPCText(cfg.World.NPCText)
	handlers.SetMaxEntitySlots(cfg.World.MaxEntitySlots)
	handlers.SetMaxViewDistance(cfg.World.MaxViewDistance)
	log.Printf("Entity view: up to %d within %d for a client that keeps more than 24 within 300",
		handlers.MaxEntitySlots(), handlers.MaxViewDistance())
	if len(cfg.World.NPCText) > 0 {
		log.Printf("NPC text: %d NPCs say their own words when talked to", len(cfg.World.NPCText))
	}

	// Tickets link the two halves of the handshake: the TCP auth server grants
	// one when a password checks out, and the UDP handlers require one. Without
	// that link the UDP path would still accept any username on its own.
	loginTickets := auth.NewTicketStore(cfg.Auth.TicketTTL)
	if cfg.Auth.RequireTCPLogin {
		handlerRegistry.SetLoginTickets(loginTickets)
	} else {
		log.Printf("WARNING: auth.require_tcp_login is off — the UDP handshake will " +
			"accept any username without a login. Do not expose this server.")
	}

	// Initialize and start auth server (TCP port 9735)
	authHandler := tcp.NewAuthServer(cfg, accountStore, loginTickets)
	// The operators' addresses and the server's own (the web relay's source,
	// every browser player's when the relay cannot name the page): one login
	// at a time does not apply to them by address, nor does the login limit.
	exempt := append([]string{cfg.Server.ServerIP}, cfg.Auth.OneLoginExempt...)
	if cfg.Auth.OneLogin {
		if err := authHandler.SetOneLogin(sessionMgr.LiveLogin, exempt); err != nil {
			log.Fatalf("auth.one_login_exempt: %v", err)
		}
		log.Printf("Auth: one login at a time per account, and per address except %v", exempt)
	}
	if cfg.Auth.RelayLookup != "" {
		addrs := []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}
		names := append([]string{cfg.Server.ServerIP}, cfg.Auth.RelayAddrs...)
		for _, a := range names {
			if ip := net.ParseIP(a); ip != nil {
				addrs = append(addrs, ip)
			}
		}
		authHandler.SetRelay(addrs, tcp.RelayLookup(cfg.Auth.RelayLookup))
		log.Printf("Auth: relayed players (from %v) named by %s", names, cfg.Auth.RelayLookup)
	}
	// One limit on guessing a password for every place one is checked: the
	// game's login here, the account pages and the GM panel (handed below).
	loginLimiter := auth.NewLimiter()
	if err := authHandler.SetLimiter(loginLimiter, exempt); err != nil {
		log.Fatalf("auth: %v", err)
	}
	log.Printf("Auth: at most %d failed logins an account and %d an address every %v (addresses %v not counted)",
		auth.FailsPerName, auth.FailsPerAddr, auth.FailWindow, exempt)
	authListener := tcp.NewListener(cfg, authHandler)
	if err := authListener.Start(); err != nil {
		log.Fatalf("Failed to start auth server: %v", err)
	}
	defer authListener.Stop()

	// Initialize and start patch server
	patchServer := tcp.NewPatchServer(cfg)
	if err := patchServer.Start(); err != nil {
		log.Fatalf("Failed to start patch server: %v", err)
	}
	defer patchServer.Stop()

	// The Freeport technician talks to the SD payload with control strings the
	// payload's display hook is meant to swallow. Between the two SD passes there
	// is no payload to swallow them and the client draws them as question marks,
	// so the technician asks the patch server whether this console is in that
	// window and explains instead. The patcher is the only thing that knows.
	handlers.ConsoleMidSDRefresh = patchServer.ConsoleMidSDRefresh

	// World ESF delivery to the in-game HDD stager. Separate from the patch
	// server's HTTP route because the consumer is different: by the time our SD
	// payload runs, the patcher module that owns the HTTP client has been
	// unloaded by CLIENT, so the stager speaks this datagram protocol instead.
	if cfg.Patch.WorldsUDPPort > 0 {
		worldUDP := udp.NewWorldServer(cfg.Patch.WorldsDir, cfg.Patch.WorldsUDPPort)
		// The same socket carries the payload's ANSWERS: what EQPREP did when
		// the Freeport technician was asked to prepare a hard disk. The
		// datagram identifies the console and nothing else, so the handlers
		// match it against the request they recorded when they sent the verb —
		// the console's address has to agree, but it is the recorded ACCOUNT
		// that decides who is told.
		worldUDP.OnStatus = func(ip string, rep udp.StatusReport) {
			log.Printf("HDD: %s reported verb=%d result=%#08x id=%d — %s",
				ip, rep.Verb, rep.Result, rep.ID, rep.Describe())
			handlers.ReportHDDStatus(sessionMgr, gameWorld, handlers.HDDReport{
				IP:       ip,
				Verb:     rune(rep.Verb),
				Token:    rep.Token,
				Describe: rep.Describe(),
			})
		}
		if err := worldUDP.Start(); err != nil {
			log.Fatalf("Failed to start world UDP server: %v", err)
		}
		defer worldUDP.Stop()
	}

	// Initialize and start account registration server
	accountServer := tcp.NewAccountServer(db, cfg.Server.AccountPort, cfg.Auth.BCryptCost)
	accountServer.SetNewCharacterRules(newRules)

	// The account pages can move a character on the logged-in account. Where the
	// destinations ARE depends on where their defining NPCs spawned, and moving a
	// character who is logged in has to happen in the world rather than in the
	// database, so both answers come from the game side rather than being
	// duplicated over there.
	accountServer.Destinations = func() []tcp.TeleportDest {
		// The same list Harbormaster Thand offers, plus Freeport itself — a
		// Harbormaster omits its own stop because a player standing at one
		// cannot usefully travel to it, which does not apply to a web page.
		src := handlers.WarpDestinations(gameWorld)
		out := make([]tcp.TeleportDest, 0, len(src))
		for _, d := range src {
			out = append(out, tcp.TeleportDest{Name: d.Name, Label: d.Label, World: d.World})
		}
		return out
	}
	accountServer.WorldNames = handlers.WorldNames
	accountServer.ResolveDest = func(name string) (float32, float32, float32, byte, string, bool) {
		return handlers.ResolveTeleDest(gameWorld, name)
	}
	accountServer.ValidatePlace = handlers.ValidateTeleport
	accountServer.TeleportLive = func(serverID uint32, x, y, z float32, world byte) bool {
		return handlers.TeleportOnlineCharacter(gameWorld, sessionMgr,
			entity.EntityID(serverID), x, y, z, world)
	}
	accountServer.SendOnly = handlers.IsSendOnlyWorld
	accountServer.EndSessions = sessionMgr.EndAccount
	accountServer.Limiter = loginLimiter

	if err := accountServer.Start(); err != nil {
		log.Fatalf("Failed to start account server: %v", err)
	}
	defer accountServer.Stop()

	// Wire session manager into world for systems
	gameWorld.SessionManager = sessionMgr

	// Initialize UDP listener with game world
	udpListener := udp.NewListener(cfg, sessionMgr, udpHandlers, gameWorld)
	udpListener.SetCharStore(charStore)

	// The GM panel (internal/gmweb): a web page for GMs and the same tools
	// for an AI agent (MCP, OpenAPI), off unless gm.port is set. It edits the
	// database and the running world, so it is handed both, and the hooks
	// that change what main owns: the new-character rules (both creation
	// paths hold a copy), auto-registration, and a restart.
	if cfg.GM.Port != 0 {
		gm, err := gmweb.New(gmweb.Deps{
			Config: cfg, ConfigPath: *configPath, DB: db, World: gameWorld, Sessions: sessionMgr,
			Characters: charStore, Accounts: accountStore, NPCs: npcStore,
			ApplyNewCharacterRules: func(r database.NewCharacterRules) {
				charStore.SetNewCharacterRules(r)
				accountServer.SetNewCharacterRules(r)
			},
			SetAutoRegister: authHandler.SetAllowAutoRegister,
			AutoRegister:    authHandler.AllowAutoRegister,
			Limiter:         loginLimiter,
			Restart: func() error {
				// Everyone saved as at a shutdown, then this process becomes
				// the binary on disk again (a new build, if one was copied in):
				// every socket closes on exec and the new image opens them.
				udpListener.SaveAll()
				exe, err := os.Executable()
				if err != nil {
					return err
				}
				exe = strings.TrimSuffix(exe, " (deleted)")
				log.Printf("GM panel: restarting %s", exe)
				return syscall.Exec(exe, os.Args, os.Environ())
			},
		})
		if err != nil {
			log.Fatalf("GM panel: %v", err)
		}
		if err := gm.Start(); err != nil {
			log.Fatalf("GM panel: %v", err)
		}
		defer gm.Stop()
	}

	// Start UDP listener
	if err := udpListener.Start(); err != nil {
		log.Fatalf("Failed to start UDP listener: %v", err)
	}
	defer udpListener.Stop()

	// Start session tick routine for keepalives and state updates
	stopTick := udpListener.StartTickRoutine(cfg.World.TickRate)
	defer stopTick()

	log.Printf("Auth server ready on port %d", cfg.Server.AuthPort)
	log.Printf("Game server ready on port %d", cfg.Server.GamePort)
	log.Printf("Patch server ready on port %d", cfg.Server.PatchPort)
	log.Printf("Account registration server ready on port %d", cfg.Server.AccountPort)
	// Which of the client's own world slots this server can actually supply.
	// Printed because a missing world ESF otherwise surfaces three layers away,
	// as a console that fails to open a file nobody said was absent.
	//
	// Every slot listed is reachable on every boot: the SD's stager fetches
	// whichever "hdd:" world the client opens, so dropping worldN.ESF in here
	// is the whole of adding a world.
	if avail := worlds.Available(cfg.Patch.WorldsDir, 8, handlers.WorldCount); len(avail) > 0 {
		for slot := 8; slot < handlers.WorldCount; slot++ {
			if name, ok := avail[slot]; ok {
				log.Printf("World slot %d: serving %s", slot, name)
			}
		}
	} else {
		log.Printf("World slots 8-%d: no worldN.ESF in %q — the disc worlds still "+
			"work, but nothing can be streamed off the hard disk",
			handlers.WorldCount-1, worlds.Dir(cfg.Patch.WorldsDir))
	}

	// The loader a tester runs to play the retail disc against THIS server.
	//
	// It is the one file handed to players that cannot be the same bytes for
	// everybody: it patches the login address into SLUS before the game starts,
	// and the retail disc hardcodes that address where no card file redirects
	// it. So the distribution carries a template and the server aims it, at the
	// same server_ip that renders eqahosts.txt and the patch manifest — an
	// operator cannot hand out a loader that disagrees with the server it came
	// from, which is precisely the failure a prebuilt copy would ship.
	//
	// Nothing here when there is no players/ folder: that is a build from
	// source rather than an unpacked archive, and it has nowhere to put one.
	if p, err := bootelf.Stage("players", cfg.Server.ServerIP); err != nil {
		log.Printf("WARNING: could not aim the boot loader at %s (%v). "+
			"players/EQOABOOT.ELF is stale or absent — do NOT hand out the "+
			"template, it carries whichever address it was built with",
			cfg.Server.ServerIP, err)
	} else if p != "" {
		log.Printf("Testers' boot loader: %s aimed at %s", p, cfg.Server.ServerIP)
	}

	log.Printf("World tick rate: %v", cfg.World.TickRate)
	log.Printf("Session pre-game timeout: %v", cfg.World.SessionTimeoutPre)
	log.Printf("Session in-game timeout: %v", cfg.World.SessionTimeoutGame)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Println("Shutdown signal received, stopping server...")
	// Everyone in the world is saved as at a logout: their positions and
	// their spell effects.
	udpListener.SaveAll()

	log.Println("Server stopped")
}
