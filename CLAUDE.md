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
| `visibility.go` | 23-channel visibility system (channels 1-23: a PS2 reads object channels 0x00-0x17 only, 0 is the player), stable ObjectID assignment per session; an entity leaves by its own id and type with the flag cleared (`MarkRemoved`), never 162 zero bytes (type 0 asserts the PS2) |
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
target), `#monstersearch`, `#nearby`. Until 2026-10-21 every player was a GM. Make one:
`sqlite3 eqoa.db "UPDATE accountinfo SET acctlevel = 1 WHERE username = '<name>'"`.

### Data Flow (World Entry)

1. `HandleSelectedChar` → `loginSpawn(record)` → `gw.CreatePlayer(sess, record)` — creates Player struct
2. `SendMemoryDump` — reads player fields directly from Player struct
3. `initPlayerStateChannel` → `SerializePlayerUpdate` — 201-byte channel 0
4. `initPlayerStatChannel` → `SerializeStatUpdate` — 236-byte channel 0x42
5. `stateSyncSystem()` — updates channel 0 every tick
6. `statChannelSyncSystem()` — updates stat channel every 20 ticks
7. `visibilitySystem()` — assigns nearby entities to channels 1-23 every 5 ticks

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
| `aiSystem` | Every 30 ticks (3s) | NPC aggro detection, melee combat, leash |
| `attackSystem` | Every tick | Players' auto-attack (`attack.go`): a 0x000F engages, swings land one swing delay apart (CLIENT's GetSwing table) in CLIENT's reach while 0x40 byte 36 is up or the NPC fights back |
| `respawnSystem` | Every tick | Dead NPC respawn after RespawnTime |
| `playerRespawnSystem` | Every tick | A dead player (`respawn.go`: 0x005E once, off every threat list, no regen) is restored 15 s after its death (Sandstorm's 15.6 s in `pcaps/breath_capture.pcap`) and sent a MemoryDump at the login spawn: the dump, not 0x00B2, is what clears CLIENT's dead flag (Msg000D's ClearPC1B0) |
| `regenSystem` | Every 20 ticks (2s) | 1% HP/Power regen (skipped if in combat <10s) |
| `stateSyncSystem` | Every tick | Serialize player state to channel 0 |
| `statChannelSyncSystem` | Every 20 ticks | Serialize stats to channel 0x42 |
| `visibilitySystem` | Every 5 ticks | Assign nearby entities to channels 1-23 |

## Current Status

### Working
- Full login flow: auth → patch → server list → character select → world entry
- Character creation with SQLite persistence
- NPC loading from packets.db, visibility system, state channel delivery
- Chat system (say + shout)
- Targeting (select NPCs/players, con colors, faction faces)
- Basic combat (attack command, damage, death; a player respawns at the login spawn 15 s after dying)
- NPC AI (aggro, melee, leash, respawn)
- HP/Power regen system
- Hotkey menu (L2 D-pad) — 25 default hotkeys, fragmented MemoryDump delivery
- Spell scrolls: the item menu's Use scribes one (`scribe.go`: a scroll is the scroll's art plus a spell of its name; Sandstorm's 0x003C + 0x00B3 and its refusal lines, recorded); `tools/import_sandstorm_harvest.py` stocks the spell vendors
- Casting (0x0032, `entity/cast.go`): Sandstorm's start (0x00CD lockout = cast + recast + 300 ms, then 0x00B1), landing 1.4 s after the cast time, refusal lines and silences, all recorded; what a spell does is read from its EQOA-Data line in `spelleffect` (`entity/spelltext.go`, filled by `tools/import_spell_effects.py`): hits, heals, ticks, stat buffs on the stat and effects channels, speed, invisibility, Return Home. Not yet: pets, charm, cures, resurrection, procs, damage shields
- For testing, `world.start_level` (a new character's level; a client's creation message never picks it) and `world.start_all_spells` (every spell of its class, from `classspells`, filled by `tools/import_class_spells.py` out of Sandstorm's dumps, its vendors' scrolls and EQOA-Data's class lists), both creation paths (`database/newchar.go`)
- Spell effects kept over a logout (`entity/buffsave.go`, table `charbuffs`), their time running only in the world, as Sandstorm's: saved at a logout, a re-login's takeover and for everyone at a shutdown (`Listener.SaveAll`, positions too), put back at the login (the world entry's 0x00F8 the buffed speed)

### Key Server Protocol Details

**State buffer format**: Hybrid — C# 201-byte size, Java (eqoa.live) field offsets from byte 99+.

**Hotkey wire format** (eqoa.live pcap, NOT C# reference):
- `count(zigzag) + foreach: direction(zigzag) + 4×(byte(slotID) + WriteStringUnicode(label) + WriteStringUnicode(msg))`
- No marker bytes. Empty strings = `int32(0)`. MemoryDump ~4800 bytes, auto-fragmented into 4×0xFA + 1×0xFB.

**MemoryDump format** (Sandstorm differs from C#): No weapon hotbar section between inventory and bank. Bank items immediately follow inventory. 4 transition bytes between bank and spells.

**Segment header size**: Placeholder varint written first, patched with actual body size after all messages written (`PatchSegmentHeaderSize`). Prevents body size mismatch when fragments exceed MTU.

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

Test accounts come from `auth.accounts` in `config.yaml`, created at startup if
absent with the password hashed. Existing accounts are never overwritten.
