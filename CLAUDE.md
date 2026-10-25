# EQOA Go Game Server

UDP game server (port 10070), TCP auth server (port 9735), HTTP patch server (port 7000) for EQOA Frontiers.

See [root CLAUDE.md](../CLAUDE.md) for project overview, research notes, ISO reverse engineering, and shared infrastructure.

## This repository is PUBLIC

[github.com/DabDavis/eqoa-server](https://github.com/DabDavis/eqoa-server), MIT. Its history
starts at the 2026-10-16 import; the March 2026 private repository's history is the local
branch `private-main` and `../backups/go-eqoa-server-private-repo-20261016.bundle`.

- **Never commit** `config.yaml` (or a copy), a database or its backups, a log, a binary,
  `worlds/` or `patches/`. `.gitignore` covers them, and every pattern for a root binary is
  ANCHORED (`/gameserver`): the old unanchored `gameserver` also ignored `cmd/gameserver/`,
  so the server's own `main` was never committed until 2026-10-16.
- No credential, personal address or game data in code, tests or comments. Test vectors
  for the login crypt are synthetic (`authcrypt_test.go`).
- It builds beside a checkout of `DabDavis/eqoa-esf-tools` (branch `client-v1`) at
  `../go-eqoa-esf-tools`, for `pkg/buffer` (see `README.md`).

## Quick Commands

```bash
# Rebuild + restart (systemd service: eqoa-gameserver)
cd go-eqoa-server && go build -o gameserver ./cmd/gameserver && sudo systemctl restart eqoa-gameserver

# Status / logs
systemctl status eqoa-gameserver
journalctl -u eqoa-gameserver -f

# The auth server (TCP 9735) runs inside gameserver; there is no separate binary.

# Database
sqlite3 eqoa.db "SELECT id, server_id, name, model_id FROM characters"
```

## Package Map

```
cmd/gameserver/      UDP game (port 10070) + TCP auth (port 9735) + HTTP patch (port 7000)
cmd/wsrelay/         WebSocket relay for browser clients (wsrelay.py's protocol, host:port allow-list)
internal/network/    RDP protocol, packet assembly
internal/protocol/   Opcodes, message types, RLE encoding
internal/session/    Session management, state channels, XOR delta
internal/handlers/   Opcode handlers
internal/entity/     Player/NPC flat structs, World, systems
internal/database/   SQLite persistence
internal/gmweb/      the GM panel: web page, MCP + OAuth + OpenAPI for AI agents
```

## Architecture: Flat Struct Entity System (`internal/entity/`)

Two flat structs (`Player`, `NPC`) with a mutex-protected `World`. All entity data accessed via direct struct field reads — no map lookups, no ECS.

### Entity Package Files

| File | Purpose |
|------|---------|
| `types.go` | `EntityID` (uint32), `Entity` interface, Faction constants |
| `player.go` | `Player` struct — all player fields (identity, spatial, appearance, equipment, stats, combat, currency, XP) |
| `npc.go` | `NPC` struct — all NPC fields (identity, spatial, model, equipment, spawn, aggro, threats) |
| `world.go` | `World` struct with `sync.RWMutex`, maps of `Players`/`NPCs`, all mutating methods |
| `systems.go` | Tick loop + game systems (AI, respawn, regen, state sync, visibility) |
| `quadtree.go` | Spatial index (QuadTree) using `EntityID` |
| `statesync.go` | Binary serializers: `SerializePlayerUpdate` (201 bytes), `SerializeNPCUpdate` (201 bytes), `SerializeStatUpdate` (236 bytes) |
| `visibility.go` | The visibility system: the nearest entities on the session's entity slots, channels 1-23 (a PS2 reads object channels 0x00-0x17 only, 0 is the player) or as many as its client asked for (below), stable ObjectID assignment per session; an entity leaves by its own id and type with the flag cleared (`MarkRemoved`), never 162 zero bytes (type 0 asserts the PS2) |
| `xp.go` | `CharXPDict` level→XP table, XP/level-up logic |

### Concurrency Model

Handler goroutines acquire `world.mu.Lock()` for direct method calls. The tick goroutine acquires the same lock for its full duration (~1-5ms at 100ms tick rate). This gives ~95-99ms of handler availability per tick cycle.

```
handler → world.mu.Lock() → direct method call → return
```

### Key World Methods

| Method | Purpose |
|--------|---------|
| `CreatePlayer(sess, record) *Player` | Create player, add to world maps + spatial index |
| `RemovePlayer(id)` / `RemovePlayerBySession(sess)` | Remove player from world |
| `UpdatePosition(id, x, y, z, facing)` | Update player position + spatial index |
| `SetTarget(id, targetObjectID)` | Set player's target |
| `ProcessAttack(sess, targetObjectID, damage)` | Process attack damage |
| `FindEntityByObjectID(sess, objID) (Entity, bool)` | Look up entity by ObjectID |
| `SessionPlayer(sess) *Player` | Get player for session |
| `ForEachPlayer(fn)` | Iterate all players |

### A login resumes where the character logged out, else beside Archivist Vaal

`loginSpawn` (`internal/handlers/character.go`) keeps the record's saved position when
`resumable` takes it: it was ever saved (a new character holds the origin), it is in one
of the six worlds CLIENT's `gWorldInfo` names on the Frontiers disc (`discWorlds`: a world
an S2 or the hard disk adds may not be on the next console, or in a page), and
`ValidateTeleport` accepts it. Anything else starts at the `vaal` `#tele` destination, a
few units off the NPC; the anchor is the NPC row, so moving Archivist Vaal moves the spawn,
and `#tele vaal` and logging in cannot disagree. `savePlayerPosition` refuses to save a
fall, an incoherent world, or a death's place (the respawn would have moved the player).

Until 2026-10-20 every login started at Vaal, unconditionally, so a saved position could
never strand a session (under terrain, in a world not on that console's disk, at PNACH
coordinates); testers asked to come back where they left (F3 20261020-195202), and the
user chose resuming. When the spawn is Vaal's the record is **not** written back.

### GM commands are a GM's: `accountinfo.acctlevel` 1 or more

Every `#` command (`internal/handlers/chat.go`, `devcmds.go`) needs an account whose
`acctlevel` is 1 or more, read on each command (a database change counts without a new
login); anyone else is answered `Unrecognized command "#x".` and nothing is said aloud.
`#hddsetup` stays everyone's (a PS2 player's hard-disk setup, also the Freeport NPC's).
A GM may also spell any of them `/name`: eqoa-rs forwards a `/command` it does not know (as
Sandstorm's S2 does), a retail PS2 keeps an unknown one to itself. The dev console of F3
20261021-003556: `#goto x y z [world] | <town or NPC>` (`#teleport` too), `#gotoplayer`,
`#summon`, `#spawn <name>` (a temporary copy, gone after its corpse time; `#despawn` the
target), `#monstersearch`, `#nearby`; `#faction <faction> [standing]` (a GM's own standing). Until 2026-10-21 every player was a GM. Make one:
`sqlite3 eqoa.db "UPDATE accountinfo SET acctlevel = 1 WHERE username = '<name>'"`.
`#template` (`handlers/template.go`, jib's ask of 2026-10-25) makes the GM's own character a
60 of its class and race: level 60, every spell of its class (`classspells`), 1,000,000
tunar, and per slot the best item of `itempattern` it may wear (CLIENT's `CanUse`: class
and race bits, levelreq <= 60) by AC and stats, the hands by the DMG the melee rolls on,
worn; all of it written, then a MemoryDump where it stands. `#setlevel` and `#tunar` write
the row too, and a character's level and the experience into it (`totalXP`: every level
below plus this one's, `entity.TotalXP`) are kept by the logout save, the shutdown's and a
replacing login's: until then a level gained or set was gone at the next login.

A place may also be named by the people standing there: a word of the names of NPCs that
stand together in one world (`NamesakePlace`: `/goto guk` lands among Guk's "froglok guk ..."
in Tunaria, on the one nearest their middle; guards along a road are listed instead). The
zones' own names are CLIENT's (`ZoneName`) and are not in this repository.

**Only worlds the client loads** (`handlers/loadable.go`, F3 20261025-000749): a Frontiers
disc has six (a retail PS2, eqoa-rust-client-vanilla, the page: DiscVersion 0x12), only a
PS2 running a patch has more (the July S2's 0x13 on its world session, our hard-disk loader
riding on it: the whole table; Sandstorm's live S2 says 0x15: its eight). The harbormasters' and gatekeepers' menus, `#goto`/`#tele`/`#summon`, the GM panel's
list and teleport and the account pages offer and take only those (`LoadableWorlds(sess)`;
no session is the disc's six, and an offline character logs in only there): a client told
to load a world its table lacks stayed where it was while the server filed it under that
world. World 8 is labelled "World 8 (HDD)" (it was "Guk" while worlds came off the card).

### Data Flow (World Entry)

1. `HandleSelectedChar` → `loginSpawn(record)` → `gw.CreatePlayer(sess, record)` — creates Player struct
2. `SendMemoryDump` — reads player fields directly from Player struct
3. `initPlayerStateChannel` → `SerializePlayerUpdate` — 201-byte channel 0
4. `initPlayerStatChannel` → `SerializeStatUpdate` — 236-byte channel 0x42
5. `stateSyncSystem()` — updates channel 0 every tick
6. `statChannelSyncSystem()` — updates stat channel every 20 ticks
7. `visibilitySystem()` — assigns nearby entities to channels 1-23 (or the client's own number of slots) every 5 ticks

### Handler Pattern

```go
targetEntity, found := h.gameWorld.FindEntityByObjectID(sess, objectID)
switch t := targetEntity.(type) {
case *entity.Player:
    // access t.Name, t.Level, t.Race, t.Class directly
case *entity.NPC:
    // access t.Name, t.Level, t.Faction, t.NPCID directly
}
```

### Game Systems (run in `Tick()`)

| System | Interval | Purpose |
|--------|----------|---------|
| `aiSystem` | Every tick | NPC melee in Sandstorm's rounds (`rounds.go`): a round's swing (0x00B1, to all who see the NPC) then its hits at fixed times, the swing CLIENT's GetSwing for the NPC's class (the `npcclass` table, `tools/import_npc_classes.py`: Sandstorm's classes, else Return Home's) and level with its weapon and shield (a warrior: below Lv20 Attack1, one hit, 3.2 s; Lv20-34 Attack2, two in 4 s; Lv35+ Attack4, four in 6 s; Lv35+ with a shield Attack3, two in 4 s; a caster always Attack1); each hit the defence roll (`defence.go`); out of its reach (CLIENT's melee reach, 4.5 and a big creature's allowance) it runs at its enemy at the player's base run (8.83) and turns to it, its round's hits lost while it closes in, and past its leash (100 from where the fight began) or with its enemy 300 off or in another world it gives up and runs back there (`chase.go`, `homeSystem`; F3 20261025-165533: the AI dropped an enemy 10 off and never moved); the training dummies' fights too (`dummy.go`) |
| `attackSystem` | Every tick | Players' auto-attack in Sandstorm's rounds (`attack.go`, `rounds.go`): a 0x000F engages, the first round 0.25 s later, then one every swing delay (CLIENT's GetSwing on the class and gear: a caster's Attack1 one hit, an SK's or paladin's Attack3 two, a melee class's Attack2 two from Lv20 and Attack4 four from Lv35), the hits at the swing's times in CLIENT's reach, each Sandstorm's roll on the worn weapon, STR and DEX (`offence.go`); a round begins only with 0x40 byte 36 up, and the round in hand lands whole after the release |
| `corpseSystem` | Every tick | A creature's corpse and its return (`corpse.go`; F3 20261025-044229): a corpse lies `world.corpse_decay` (3m) unless its loot window closes with nothing left on it (an empty one opens as "Empty"; `handlers/loot.go` → `ClearCorpse`), then leaves every view; the creature is back on its spawn point `world.respawn_after` (60s) after its corpse went; a GM's /spawn leaves with its corpse |
| `roamSystem` | Every tick | Creatures wander round their spawn (`roam.go`, `npcmove.go`; F3 20261025-044257): within `world.roam_radius` (15; below 0 none) at Sandstorm's 5 units a second with the walk animation, resting 5-20 s; an NPC's velocity (bytes 40-42) and animation (58: 0 stand, 1 walk, 3 run) on its channel, CLIENT's creature path (`ExtrapolateMove`, `SetLocomotion`). A creature is a body that is not a player's race (`charactermodel`), no 0x80, not a dummy: guards, merchants and townsfolk stand, as Sandstorm's guards do. Only those within 1000 of a player move; none while fought, rooted or dead; a snare halves the pace. No ground on the server: the spawn's height, the client floors it within 1.5 |
| `petSystem` | Every tick | Summoned pets (`pet.go`): following (`npcmove.go`: an NPC's step with its velocity and locomotion on its channel), stances, rounds on their targets, their owners told their ids (0x007E), and taken away with their owners |
| `playerRespawnSystem` | Every tick | A dead player (`respawn.go`: 0x005E once, off every threat list, no regen) is restored 15 s after its death (Sandstorm's 15.6 s in `pcaps/breath_capture.pcap`) and sent a MemoryDump at the login spawn: the dump, not 0x00B2, is what clears CLIENT's dead flag (Msg000D's ClearPC1B0) |
| `regenSystem` | Every 20 ticks (2s) | 1% HP/Power regen (skipped if in combat <10s) |
| `stateSyncSystem` | Every tick | Serialize player state to channel 0 |
| `statChannelSyncSystem` | Every 20 ticks | Serialize stats to channel 0x42 |
| `visibilitySystem` | Every 5 ticks | Assign nearby entities to channels 1-23, or the slots (and distance) the client asked for |

## The GM panel (`internal/gmweb`)

A web page for GMs, and the same work for an AI agent, over the database and the running
world. **Off unless `gm.port` is set**; it listens on `gm.bind` (loopback) and is meant to be
reached through a TLS proxy on a host name of its own (`gm.<domain>` → `127.0.0.1:8081`), with
`gm.public_url` set to that address (the OAuth issuer, the cookie's path and `Secure`). Its own
host matters: a page of any other path on the same origin could read the session's CSRF token,
and on its own https host the session is a `__Host-` cookie no other host can set.

- **Who**: `acctlevel >= gm.min_level` (1) signs in with the game password (bcrypt) and a
  TOTP code once enrolled (`totp.go`); `gm.admin_level` (2) adds raw SQL, settings, backups,
  restart and other accounts' levels, **and those need a sign-in with an authenticator code**
  (`actor.Admin`: the session gave one or enrolled one; a token was made by such a sign-in,
  and the account still has it) — the game password travels in a weak cipher and is not
  enough for the whole server. The level is read again on EVERY request, so a demotion in the
  database is out at once. No account at or above your level, nor its characters or their
  rows, can be changed by you but your own (`guardAccountRow`, `guardOwner`), and a level
  change of any origin, a revert included, passes `guardLevel`. Sign-in attempts are counted
  per address (IPv6 by /64) and per name BEFORE the password is checked, in bounded maps, with
  at most four bcrypts at once (`session.go`, `auth.go`).
- **One registry of operations** (`ops*.go`): name, AI-readable description, JSON Schema,
  scope (read/write/admin), handler. The page calls `api/op/<name>`; MCP (`mcp.go`, JSON-RPC
  over plain HTTP POST, no stream) lists them as tools; `api/openapi.json` (`?core=1`, a
  shorter list for a custom GPT) describes them. A change to an operation is a change to all three.
- **AI agents** sign in as the GM: OAuth 2.1 (`oauth.go`: RFC 9728 resource metadata, RFC 8414,
  open dynamic registration, PKCE S256 only, refresh rotation, RFC 9207 `iss`, loopback
  redirects on any port per RFC 8252 7.3 — Codex needs that), or a personal bearer token
  made in the panel or by `POST api/auth/token` with name and password (a terminal, Codex over
  SSH). Tokens are kept as SHA-256 only (`gm_tokens`), scoped, revocable, rate limited
  (`gm.token_rate`, each call of an MCP batch counted), expire within a year, and go with the
  account's password. Registration is open, so the consent page names where a client returns
  to, and one not returning to ChatGPT, Claude or this computer is offered read only and needs
  the GM's tick; a bad authorize request is answered on the page, never redirected (an open
  redirector otherwise); a refresh token used twice revokes its grant. ChatGPT connectors take
  OAuth only (no static header); a custom GPT's actions take a token. `guide.md` is what an
  agent reads first.
- **Every change is audited** (`gm_audit`: who, through what, the row before and after, in the
  change's own transaction) and a row change can be reverted (`audit.go`).
- **Live** (`live.go`, `entity/gmlive.go`, `handlers/gmops.go`): an `npcs` row inserted, edited
  or deleted is applied to the World at once (the NPC is hidden from every view for
  `NPCRefreshHold` so each client re-creates it); `npcclass` re-classes NPCs by name;
  `defaultcharacter` invalidates both template caches; an online character's level, tunar,
  bank tunar and place are applied where it stands; a ban ejects (0x07A2) the account's
  sessions; give_item delivers into an online inventory as a sale does (0x003B); grant_spell
  as a scribe (0x00B3). What the game reads only at a login says so in the answer.
- **The catalog** (`catalog.go`) is what each table and column means (labels, enums, npc_type
  flags, class/race bit sets, references, hex DictIDs); a table it does not know is shown raw
  and is admin-only to write. `accountinfo.pwhash` is never read or written through rows.
- **The game disc, in the browser only** (`static/disc/`): the GM picks their own ISO; a worker
  reads it in place (ISO 9660 walk), draws NPC bodies (`npcs.modelid` = CHAR.ESF CSprite,
  442/442 live values), item models (ITEM.ESF) and icons (`itemicon` = ITEMICON.ESF Surface,
  337/337), and the map's zones from straight above (each world's zone table is in its last
  few KB, 0x8200 → 0x8210). CHAR/ITEM/ITEMICON.ESF are kept in OPFS and zone pictures in
  IndexedDB, so later visits need no disc. The server ships and stores no game data.

Traps: the database has ONE connection (`SetMaxOpenConns(1)`): never query while holding rows
or a `db.Conn`/transaction from the plain handle — it deadlocks the game (the SQL console once
did, through its audit write). A view must not `import` `app.js` (the page loads it with
`?v=<build>`, so another URL evaluates it twice): shared state is `lib/nav.js`. In a request
struct that embeds another, an outer field shadowing an embedded TAGGED one needs the same
`json` tag (Go matches the exact tag before the case-insensitive name).

Test locally against a copy of the database on spare ports (`gm.port` 18081, the rest moved),
and drive the page headless with `tools/gmdrive.py`; `go test ./internal/gmweb/`
covers sign-in, CSRF, limits, scopes, live NPC edits, revert, the guards, MCP and the whole
OAuth flow, and the official MCP Python SDK was run against it end to end.

## Current Status

### Working
- Full login flow: auth → patch → server list → character select → world entry
- Character creation with SQLite persistence
- NPC loading from packets.db, visibility system, state channel delivery
- Chat system (say + shout)
- Targeting (select NPCs/players, con colors, faction faces)
- Basic combat (attack command, damage, death; a player respawns at the login spawn 15 s after dying)
- NPC AI (aggro, melee in Sandstorm's rounds — `entity/rounds.go`, leash, respawn)
- Corpses as live EQOA's: lie 3 minutes unless emptied (an empty one opens "Empty"), emptied and closed they go at once, and the creature returns a minute after its corpse went (`entity/corpse.go`); creatures wander round their spawn, townsfolk stand (`entity/roam.go`)
- HP/Power regen system
- Hotkey menu (L2 D-pad) — 25 default hotkeys, fragmented MemoryDump delivery
- Spell scrolls: the item menu's Use scribes one (`scribe.go`: a scroll is the scroll's art plus a spell of its name; Sandstorm's 0x003C + 0x00B3 and its refusal lines, recorded); `tools/import_sandstorm_harvest.py` stocks the spell vendors
- Casting (0x0032, `entity/cast.go`): Sandstorm's start (0x00CD lockout = cast + recast + 300 ms, then 0x00B1), landing 1.4 s after the cast time, refusal lines and silences, all recorded; what a spell does is read from its EQOA-Data line in `spelleffect` (`entity/spelltext.go`, filled by `tools/import_spell_effects.py`): hits, heals, ticks, stat buffs on the stat and effects channels, speed, invisibility, Return Home, summoned pets (below). Not yet: charm, cures, resurrection, procs, damage shields
- Summoned pets (`entity/pet.go`, F3 20261025-044023; `claude-notes/pets-charm-client-20261021.md`): a line that summons ("Summons an Iron Golem to your defense", "Summons a pet") lands as a pet named as the spell, at the spell's level (at most the caster's), its body the most common of this server's NPCs of that name (else the line's creature, the name after a possessive, a class stand-in), a creature's HP and melee of its level. As Sandstorm's recorded Fire Elemental: 0x007E with its id (again after every MemoryDump), 0x1003 on the owner's non-collide list, byte 142 3, npc flags 0. It keeps the first entity slot, follows, comes along through a teleport or into another world, fights in rounds (its hits and kills the owner's; what it hits fights back), and takes 0x0041's stances (defensive a new pet's), dismiss, back off and attack, each answered in a line; scope-3 spells land on it; no player can harm it. It goes (0x007F, 0x1004) when dismissed, at its death or its owner's, and at a logout. Pets' damage is not measured (a creature's roll of its level less the target's defence)
- For testing, `world.start_level` (a new character's level; a client's creation message never picks it) and `world.start_all_spells` (every spell of its class, from `classspells`, filled by `tools/import_class_spells.py` out of Sandstorm's dumps, its vendors' scrolls and EQOA-Data's class lists), both creation paths (`database/newchar.go`)
- Training dummies as Sandstorm's (`entity/dummy.go`, measured; `claude-notes/training-dummies-20261021.md`): "Training Dummy …" and "Regular Dummy …" never die, fight one player in rounds, and a Training Dummy heals 300 after each hit
- A player's melee on an NPC as Sandstorm's (`entity/offence.go`, measured on 1,594 swings; `claude-notes/offence-sandstorm-20261022.md`): the roll from the worn weapon's DMG and condition (a broken one a fifth; `Gear.WeaponCond`, `WeaponTwoHanded`), STR raising the top and DEX the bottom, an archetype's offset, and the target's defence by its level; `offence_test.go` holds it to Sandstorm's swings. The attacker's own level was not measured (all L49-51); the default Khal Sword is DMG 999 and hits like it
- NPC melee on a player as Sandstorm's (`entity/defence.go`, measured on 2,660 dummy hits; `claude-notes/defence-sandstorm-20261022.md`): max(floor, a roll from top − width to top), the top falling with the player's AC (by the NPC's slope) and AGI (by the archetype's: tank 0.305, melee 0.227, caster 0.133) and raised by the archetype (melee +15, caster +32 over a tank); the measured dummies' own rolls, a creature's a provisional line through them (floor half its level). `defence_test.go` holds it to Sandstorm's hits (`testdata/sandstorm_defence_hits.txt`). Priests are set between melee and caster, unmeasured; NPC specials are not done
- The HUD's item quick slots and weapon sets, as SOE's live client has them (`handlers/gearsets.go`, tables `quickslots` and `gearsets`; `claude-notes/live-toolbars-gear-sets-20261024.md`): the item menu's Prepare (0x000A, answered with the list index; a potion on 0-1, a gem on 2-3, on a PS2 a weapon on 3, kept and sent in the MemoryDump), a weapon set (0x0021: the hands as worn, named; all four sent at world entry, which a PS2 skips) and taking one up (0x0022; 4 is bare hands); the quick slot's use is 0x003E with interaction 1, the item menu's use. Using a potion or a gem does nothing yet (no item effects)
- Spell effects kept over a logout (`entity/buffsave.go`, table `charbuffs`), their time running only in the world, as Sandstorm's: saved at a logout, a re-login's takeover and for everyone at a shutdown (`Listener.SaveAll`, positions too), put back at the login (the world entry's 0x00F8 the buffed speed)
- Factions (`entity/faction.go`, F3 20261025-044451): every faction's starting standing is table `factions` (filled from `database.DefaultFactions`, the eqoa.live capture's 331, when empty), a character's own are `charfactions` (kept as `charbuffs` are), and killing an NPC changes the KILLER's by the `factionhits` rows for its name (melee, spell or a GM's `#kill`; a `#spawn` copy counts as its original), each change a line in EverQuest's words unless 0x00C9 says Faction Messages off; -2000..2000; the window (0x0AC4 → 0x07E2) lists the character's own in the capture's order. **The hit numbers are ours, not measured.** `tools/seed_faction_hits.py` writes the town rows (source `town-seed`: a city's townsfolk -25 on its Citizens, its guards -50 on its guard factions; the cities off EQOA-Data's map, `--undo` deletes them); it needs the `factions` table, so run it after the server's first start with it and restart after. `#faction <faction> [standing]` shows or sets a GM's own

### Key Server Protocol Details

**State buffer format**: Hybrid — C# 201-byte size, Java (eqoa.live) field offsets from byte 99+.

**Hotkey wire format** (eqoa.live pcap, NOT C# reference):
- `count(zigzag) + foreach: direction(zigzag) + 4×(byte(slotID) + WriteStringUnicode(label) + WriteStringUnicode(msg))`
- No marker bytes. Empty strings = `int32(0)`. MemoryDump ~4800 bytes, auto-fragmented into 4×0xFA + 1×0xFB.

**MemoryDump format** (Sandstorm differs from C#): No weapon hotbar section between inventory and bank. Bank items immediately follow inventory. 4 transition bytes between bank and spells. (That section is SOE's live client's four weapon sets, 4 × zigzag main, zigzag off, string name; the Frontiers disc never reads it, so the sets go as 0x0021 after world entry instead.)

**Segment header size**: Placeholder varint written first, patched with actual body size after all messages written (`PatchSegmentHeaderSize`). Prevents body size mismatch when fragments exceed MTU.

**More than 24 NPCs and players in view, and farther, for a client that keeps them**
(2026-10-24; `session/entityslots.go`, `handlers/entityslots.go`). CLIENT keeps 24 entity
slots, the state channels 0x00-0x17; DRDP has 248 (0x00-0xF7), and CLIENT reads 0x40
(group), 0x42 (stats), 0x43 (effects) and 0x44-0x6B as other things and ignores the rest.
At the first 0x0014 the server offers its most, opcode **0x7E00** (`OpcodeEntityView`; not
retail: CLIENT's dispatcher has no default and nothing above 0x1402; S2's are
0x1500-0x1507), two u32s: the entity slots and the view distance. A client that keeps more
(eqoa-rust-client-vanilla's NPC/PC Display and NPC/PC View Distance) answers with its two,
again whenever they change, and the session fills that many from that far: slot n on
`session.EntityChannel(n)`, 24-63 on 0x18-0x3F and 64-203 on 0x6C-0xF7, so 204 at most
(`world.max_entity_slots`), and the visibility system's radius the connection's
`ViewDistance()`, at least the 300 every client has, at most 1000
(`world.max_view_distance`). `ConnectionData`'s object channels are by slot and grow when
asked; a state ack names a channel (`ServerObjectForChannel`). Such a session's tick sends a
datagram more for each 24 slots (`TickSession`, `statePacketsPerTick`), each with up to
`MaxStateUpdatesPerTick` entity channels further round the round robin. A session that
never asks (every PS2) is exactly as before: 24 slots within 300, one datagram a tick
(`TestPS2TickIsOneDatagram`).

**Effects channel (0x43) is as long as its count**: a u32 count and a 132-byte record an effect (icon DictID, name as a wide string of at most 31 characters), 4 bytes when there is none, as CLIENT's `Channel43` reads it and every value in the captures is (136 to 796 bytes). It is the one state channel that changes length (`StateUpdateData.SetSizedData`; DRDP XORs over the shorter of a message and its reference). `#effect <icon hex> [name]` / `#effect clear` sets them by hand: nothing else puts an effect on yet.

## Configuration

```yaml
# config.yaml
server:
  game_port: 10070
  auth_port: 9735
  patch_port: 7000
  bind_address: "0.0.0.0"
  server_ip: "203.0.113.10"   # your public address (a documentation one here)
```

`server_ip` used by: `internal/handlers/server.go` (server list), `internal/network/tcp/patchserver.go` (patch XML).

### The patch manifest is a FOLDER

`patches/serve/` is the manifest: every file in it is offered under its own name — the
name it gets on the memory card. Drop a file in to serve it, delete it to stop.
`patches/` beside it is the pool of rollbacks and build inputs, reachable with
`EQOA_SS_FILE` / `EQOA_S2_FILE`. Four things the folder cannot enforce alone, all
checked in code: `eqahosts.txt` is generated and refused from `serve/`; a `.AUT` is
refused (it makes the patcher DNAS-encrypt Sony's module onto the card); the six
**ELF-delivered** names are refused (below); and **removing a file from `serve/` does
not remove it from a card** — the patcher only acts on names the manifest carries, so
`serve/.delete` is how space is reclaimed. See [patches/README.md](patches/README.md).

**The `<Distribution>` says `UseCRC="True"`, as Sandstorm's does** (2026-10-16). Without
it the PS2's patcher judges a file by its SIZE alone, and a same-length change never
reaches a card. That was the silent "manifest GET, no file GET". The decomp of SETUP's
patcher shows `<Product Version>` is never read, so the version-bump comments in
`patchserver.go` are corrected in place. A file that SHRINKS still needs the Delete dance,
because the card keeps the old tail. See `claude-notes/go-client-patcher-20261016.md`.

### There is a SECOND manifest, and this server does not own it

`ATAD.IRX`, `HDD.IRX`, `PFS.IRX`, `EQPREP.IRX`, `EQSCENE.BIN` and `SD` are **not
served**. `EQOAHDD.ELF` installs them and is their only delivery; its manifest is
`eqscene/card/`. `elfDelivered` in `patchserver.go` refuses those names in `serve/`
(escape hatch `EQOA_SERVE_ELF_FILES=1`), because **one filename may not have two
writers**: the patcher re-downloads on SIZE, so two copies that differ but match in
length leave the card holding whichever landed first, forever, with every end reporting
success.

What stays here is the game patch — `SS`, `S2`, `ss-art.csf`, `station.pak`,
`WORDS.TXT` — plus generated `eqahosts.txt`. The chain is still `SS` → `S2` → `SD`, so
**`SD` is inert until the patcher has run**: nothing but `SS` stage 2 opens it. The tool
owns the hard-disk *content*; this server still owns the hard-disk *loader*.

Consequences elsewhere: `refreshOrder` (the per-console two-pass Delete dance) moved from
`SD` to `SS`, which inherits the same same-size-rebuild hazard; `ConsoleMidSDRefresh` now
asks about every managed file rather than naming `SD`; and `TestPayloadFilesAreServed`
checks the **union** of the two deliveries, failing a file delivered by neither *or* by
both.

## Authentication

Passwords ARE checked. The Login Request carries the password DES-CBC encrypted,
not hashed — key and IV are both `dfc2046b4983fd6b`, a fixed protocol constant
(see `claude-notes/auth-server-protocol.md`). The server decrypts it in
`internal/network/tcp/authcrypt.go`, reads to the first NUL, and compares against
a bcrypt hash in `accountinfo.pwhash`. Everything after that NUL is live EE
memory and differs between two logins of the same account — never treat it as
part of the credential.

Two things follow from the protocol that are easy to get wrong:

- A successful TCP login grants a **ticket** (`internal/auth/tickets.go`). The
  UDP `Authenticate`/`Authenticate2` handlers require one, because they read the
  username straight out of the packet — without the ticket a client could skip
  the auth server entirely and name any account, making the password check
  pointless. Governed by `auth.require_tcp_login`.
- Accounts with an empty `pwhash` predate password checking. The first login that
  supplies a password adopts it (when `auth.allow_auto_register` is on); after
  that the account is enforced normally.
- **Guessing is limited, everywhere a password is checked, by ONE `auth.Limiter`**
  (`internal/auth/limiter.go`, made in `main.go`): this login, the account pages'
  `/api/login` and `/api/logout` (public through nginx: the play page's "log the
  other window out"), and the GM panel. `auth.FailsPerName` (8) attempts an
  account and `auth.FailsPerAddr` (20) an address every 15 minutes, counted
  BEFORE the password is checked (a burst gets no more than a slow guesser), a
  right password taking its attempt back; at most four password checks at once.
  The address is the client's own: a relayed browser player's from wsrelay's
  lookup, which is set whenever `auth.relay_lookup` is. Not counted by address:
  loopback, `server_ip` and `auth.one_login_exempt` (an unnamed relayed player
  is counted by name only). A login that REGISTERS an account keeps its
  attempt, so an address registers no faster than it may guess. A refused
  login is STATION's failure with the reason in the text the browser shows.

Test accounts come from `auth.accounts` in `config.yaml`, created at startup if
absent with the password hashed. Existing accounts are never overwritten.
