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
| `xp.go` | `CharXPDict` level→XP table (CLIENT's `ExpForLevel`, `killexp.go`), XP/level-up logic |
| `killexp.go` | What a kill is worth: the live server's experience model CLIENT carries unused (`GroupExp`/`KillExp`/`LevelExp`), shared with the group near |

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
`resumable` takes it: it was ever saved, it is in one
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

**A new character starts in its home city** (`database.CreateNew`, both creation paths; F3
20261025-221828): on `defaultcharacter`'s row for its race, class and city (a human's east 1
or west 2; the froglok a human of the east's), the live game's spots (Sandstorm's new
characters stand exactly on them), each a few steps from the class's first quest giver and
facing it (`tools/fix_start_spots.py` checked them against EQOA-Data's guides and moved four:
`--apply`/`--undo`, logged in `start_spot_log`); and it is bound there (`charbind`, by its
city's name) until a spiritmaster binds it, so its first login resumes there and Return Home
and a death take it home. A race with no row keeps the origin and starts at Vaal's.

### GM commands are a GM's: `accountinfo.acctlevel` 1 or more

Every `#` command (`internal/handlers/chat.go`, `devcmds.go`) needs an account whose
`acctlevel` is 1 or more, read on each command (a database change counts without a new
login); anyone else is answered `Unrecognized command "#x".` and nothing is said aloud.
`#hddsetup` stays everyone's (a PS2 player's hard-disk setup, also the Freeport NPC's).
A GM may also spell any of them `/name`: eqoa-rs forwards a `/command` it does not know (as
Sandstorm's S2 does), a retail PS2 keeps an unknown one to itself. The dev console of F3
20261021-003556: `#goto x y z [world] | <town or NPC>` (`#teleport` too), `#gotoplayer`,
`#summon`, `#spawn <name>` (a temporary copy, gone after its corpse time; `#despawn` the
target), `#monstersearch`, `#nearby`; `#faction <faction> [standing]` (a GM's own standing); `#aggro [on|off]` (whether creatures attack the GM on sight, `entity/aggro.go`; off until `#aggro on` or a restart); `#npcinfo` (the target's ids, model, size, level, spawn and gear in a box: what talking to an NPC with nothing to say showed everyone until F3 20261025-171528; such an NPC now answers nothing); `#stats` (F3 20261026-014422, jib's "a gm /command to display extended states": `entity/statsreport.go`, the target's or the GM's own in chat: max HP and power by their parts, HoT/PoT, each stat as base + items + spells against its cap, AC, resists, the melee and defence rolls, the old fitted OFF/DEF MOD said unused, move rate, training points, masteries, effects with their time; a creature's round, its roll on the GM and the GM's on it, reach, aggro, undead, who it helps, its hate list). **Spell visuals by hand** (`handlers/spellfx.go`, F3 pass 16, the user's ask): `#fx <visual> [seconds]` plays a visual (0x00B1's DictID, a 0xC200 record of SPELLFX.CSF, as `0x1DAEB5B6` or the eight digits) from the GM onto its target now (`World.PlayVisual`: a cast's 0x00B1 to all who see the GM, nothing else of a cast), `#fx <spell>` what a spell plays; `#spellfx <spell> [<visual> | 0]` says or sets `spelleffect.fxdict` (0: the stand-in of its kind, `entity.StandInVisual`), the change written to `gm_audit` in its own transaction as the panel writes a row edit (via `game`, `database.SetSpellVisual`), so the panel lists and reverts it. Until 2026-10-21 every player was a GM. Make one:
`sqlite3 eqoa.db "UPDATE accountinfo SET acctlevel = 1 WHERE username = '<name>'"`.
`#template` (`handlers/template.go`, jib's ask of 2026-10-25) makes the GM's own character a
60 of its class and race: level 60, every spell of its class (`classspells`; never a were form or a were's ability, `database.IsWereAbility`: the were quests are any class's and the forms past the first are masteries), 1,000,000
tunar, its mastery points to 1500 spent and unspent (the user's "let template also give 1500 CM points to the GM"), and per slot the best item of `itempattern` it may wear (CLIENT's `CanUse`: class
and race bits, levelreq <= 60) by AC and stats, the hands by the DMG the melee rolls on,
worn; all of it written, then a MemoryDump where it stands. `#setlevel` and `#tunar` write
the row too, and a character's level and the experience into it (`totalXP`: every level
below plus this one's, `entity.TotalXP`) are kept by the logout save, the shutdown's and a
replacing login's: until then a level gained or set was gone at the next login.

**The XP camps are `#tele` shortcuts** (`handlers/camps.go`, written by `tools/gen_camp_teles.py`;
the user's "add popular xp camps to the gm tele port like /tele dfc (deathfist citadel) /tele
hexbones"): EQOA-Data's XP-camp guide (its map's 69 camps with a note, `Map/zones.json`) placed
over the game by a fit of its map to the NPCs it names (Tunaria: the game is 2.13078 × the map
plus an offset, a median of 6.6 units off over 385 points; Odus has its own, looser, 164), and
anchored on the camp's commonest creature in the server's npcs at its densest cluster near that
point; 49 camps (`dfc`, `hexbones`, `kappa`, `tendora`, `harpies`, `xrag`, ...; aliases such as
`deathfist`, `shrieks`), each in `#tele`'s list by its creatures' levels here (not the guide's),
`#tele camps` with where each is. A camp's anchor is the creature of the anchor's name nearest
the spawn written in the table (within 300; never a GM's copy), else that spawn, and `#tele` lands
**beside** it, facing it (`World.CampLanding`, `entity/camplanding.go`): 45 to 150 out, the side away
from the camp's middle first, on the terrain within 12 of the terrain at the anchor (under it for a
camp on a keep's floor or a tree's platforms), under the open sky (no floor 8 above: never a tunnel
or a castle's hall), out of the water, no creature within 40 by where it stands or spawned (one with
15 at least where none has 40); a camp in a cave lands on the anchor (the Goblin Citadel, the Druk's
orc tunnel). A level 60 GM landing in the Deathfist Citadel's middle died in three seconds; beside it
a roamer still comes by. Without the ground files every camp lands on its anchor. `#goto <camp>` and
the GM panel's teleport list take them too. Run the tool again (it gofmts its output) after moving a
camp's creatures.

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
| `aiSystem` | Every tick | NPC melee in Sandstorm's rounds (`rounds.go`): a round's swing (0x00B1, to all who see the NPC) then its hits at fixed times, the swing CLIENT's GetSwing for the NPC's class (the `npcclass` table, `tools/import_npc_classes.py`: Sandstorm's classes, else Return Home's, whose npc_type is no class for its creatures, so `tools/fix_npc_classes.py [--apply|--undo]` gives each Return Home row the class its name says (a caster's word, whole and nearest the name's end: shaman, mystic, seer, augur, witch doctor, cleric, priest, bishop, druid, wizard, mage, magus, magician, summoner, wisp, elementalkin, necromancer, lich, warlock, defiler, enchanter, alchemist, chemist; paladin, shadow knight), else a warrior's, merchants and townsfolk too; Sandstorm's rows and a GM's left alone, logged in `npcclass_fix_log`; F3 20261026-042344: Lord Nagafen was a magician and single-hit) and level with its weapon and shield (a warrior: below Lv20 Attack1, one hit, 3.2 s; Lv20-34 Attack2, two in 4 s; Lv35+ Attack4, four in 6 s; Lv35+ with a shield Attack3, two in 4 s; a caster always Attack1); each hit the defence roll (`defence.go`); out of its reach (CLIENT's melee reach, 4.5, and a large creature's +3 or a huge one's +5: `reach.go`, below) it runs at its enemy at Sandstorm's chase pace (9.9 a second, measured: faster than the base run, 8.83) to half a step inside that reach, and turns to it, its round's hits lost while it closes in, and past its leash (100 from where the fight began) or with its enemy 300 off or in another world it gives up and runs back there (`chase.go`, `homeSystem`; F3 20261025-165533: the AI dropped an enemy 10 off and never moved); the training dummies' fights too (`dummy.go`) |
| `aggroSystem` | Every 5 ticks | Creatures attack on sight (`aggro.go`; the user's "aggro from mobs that are red faced and within level range"; `claude-notes/aggro-20261025.md`): a creature hostile by nature (no NPC bit, a body no player race has, `charactermodel`, spawned outside `towns.go`'s towns) attacks a player below three times its level within its `AggroRadius` (30) through a hit's own entry (`addThreatLocked`, which calls no helper: `socialAssistLocked` below), so the chase, leash and way home are the fight's; not an invisible player, a GM who typed `#aggro off`, one loading a world, a dead one; not a creature fighting or going home, a pet, a dummy. The target box's face is the same test per player (`World.TargetFace`, `handlers/target.go`): red exactly for what attacks you on sight or fights you. Measured on Sandstorm's 0x0010 replies (a L51 player saw L18-19 red, L17 white, all green); the ×3, the body/town rule and the radius are chosen. The loader's `FactionHostile` (`npc_type` bits) counts for nothing; with an EMPTY `charactermodel` (the local dev database) nothing attacks but by a GM rule. Table `npcaggro` (GM panel "Creature aggro", live) names exceptions by NPC name: `aggressive` 1/0, `radius`; `tools/seed_npc_aggro.py --eqoa-data ../external/EQOA-Data [--apply\|--undo]` seeds the players' pets Return Home's NPC rows hold (46 names on eqoaclassic) and the Cave Minnow peaceful. `world.aggro: false` turns it off |
| `socialAssistLocked` | Every hit | NPCs come to the help of their own (`socialaggro.go`; F3 20261025-215440, jib's ask): a player's or a pet's hit or hostile act on an NPC (`updateAggro`: melee, a spell's damage or landing, a hate clause, a resisted charm, each damage tick) calls every social NPC of its faction within its radius (30: the range of 176 of the 402 spells, and AggroRadius's own) in its world into the fight against the attacker (a pet's owner for a pet's hit: a creature does not run after a pet), as a hit of its own would start one (`addThreatLocked`). Table `npcsocial` (GM panel "Social NPCs", live) by NPC name: `social`, `faction` (any group name), `radius`; an attacked NPC is of its row's faction and of every faction its kill lowers (`factionhits`). A creature's own notice of a player (`aggroSystem` takes `addThreatLocked` alone) and a helper's joining call no one; a helper fighting, going home, dead, hidden, a pet or charmed, a dummy or with the NPC bit does not come. "Sight" is the radius: no walls. `tools/seed_npc_social.py --db eqoa.db [--apply\|--undo]` writes the towns' guards (factionhits' town-seed rows) under their town's guard faction or "Guards of <town>" |
| `npcRegenSystem` | Every 6 s | Creatures heal out of a fight (`npcregen.go`; F3 20261025-215107, jib's numbers): on the regen's clock an NPC with no enemy counts the ticks, the first two give nothing, every one after 15% of its maximum (at least 1) until whole; any threat (`addThreatLocked`) starts the count again, so one leashed heals on its way home. Not pets, charmed creatures, dummies or corpses. Until then a creature never healed |
| `attackSystem` | Every tick | Players' auto-attack in Sandstorm's rounds (`attack.go`, `rounds.go`): a 0x000F engages, the first round 0.25 s later, then one every swing delay (CLIENT's GetSwing on the class and gear: a caster's Attack1 one hit, an SK's or paladin's Attack3 two, a melee class's Attack2 two from Lv20 and Attack4 four from Lv35), the hits at the swing's times in CLIENT's reach, each Sandstorm's roll on the worn weapon, STR and DEX (`offence.go`); a round begins only with 0x40 byte 36 up, and the round in hand lands whole after the release |
| `corpseSystem` | Every tick | A creature's corpse and its return (`corpse.go`; F3 20261025-044229): its channel turns to a corpse at the death as Sandstorm's (byte 26, CLIENT's VIEntity +0x18, 3: selectable and the corpse bit; health 0), and the attack button on it is the game's loot (ToggleAutoAttack's 0x0015 → `handlers/loot.go` HandleLootCorpse, within InteractRange plus a large body's allowance, else "Out of range."; F3 20261026-042112: without the bit a creature the game shows as attackable could not be looted, InteractWithTarget sending nothing for one), talking to it (0x0004) too; its first opener takes its tunar at once, as Sandstorm's (0x0052 the purse, "You receive N tunar from the corpse.", `handlers/corpsecoins.go`; a roll from the creature's level to twice it, ours: the one recorded corpse, a level 2 Orc Pawn's, held 2; none on a pet's or a dummy's); a corpse lies `world.corpse_decay` (3m) unless its loot window closes with nothing left on it (an empty one opens as "Empty"; `handlers/loot.go` → `ClearCorpse`), then leaves every view; the creature is back on its spawn point `world.respawn_after` (60s) after its corpse went; a GM's /spawn leaves with its corpse |
| `roamSystem` | Every tick | Creatures wander round their spawn (`roam.go`, `npcmove.go`; F3 20261025-044257): within `world.roam_radius` (75 since F3 20261025-164404, was 15; below 0 none) at Sandstorm's 5 units a second with the walk animation, in legs of 5 to 35 in any direction, each followed by a rest of 3-25 s or, two in five, a pause of 0.5-2 s and another leg; never into or through a town (`towns.go`: a chain of 10 or more townsfolk each within 60 of the next, found in the server's own NPCs, and 40 round them; one that lives there may), so wildlife comes to a city's gates but not in; an NPC's velocity (bytes 40-42) and animation (58: 0 stand, 1 walk, 3 run) on its channel, CLIENT's creature path (`ExtrapolateMove`, `SetLocomotion`). **Only wildlife wanders unless `npcmove` says otherwise** (F3 20261026-042712, jib: "Not all mobs are wild life. Fire Giants are roaming currently when they should not be unless otherwise specified / designated as a roamer with a path"; `movement.go`'s `movementLocked`, `wildlife.go`): an animal by its name's head noun (its last word past a pack or hive word: Giant Rat, Wolf Pack Leader, red ant warrior, Scorchflies; not Fire Giant, Orc Pawn, serpent guardian), not undead (`npcundead`, the name), on a body that is not a player's race (`charactermodel`), no 0x80, not a dummy; anything else stands on its spawn until pulled and goes home after; an NPC players talk to (0x80) never wanders (a path walks it). Until then every creature not on a player's body wandered. The GM panel's NPC page says whether one wanders and why (`NPCState.Movement`). Only those within 1000 of a player move; none while fought, rooted or dead; a snare halves the pace. **The ground** (`internal/ground`, `entity/groundnpc.go`; F3 20261025-165427, creatures floating off hills): CLIENT sets a creature down only on a floor within 1.5 of the height it is sent (0x00CBCE40), so every NPC move (wander, chase, way home, a pet's) takes the highest TERRAIN floor at its place no more than 2 above it, for a creature that spawned within 1.5 below to 2.5 above the terrain (F3 20261026-021425: at 1.5 a crocodile recorded 1.68 above the sand wandered the beach at its spawn's height, floating); one that stands off the ground (a keep's floor, a bridge, a dock: the tail beyond 2.5) does not wander, keeps its spawn's height within 10 of its spawn in a chase or on its way home and takes the terrain beyond (`platformReach`); every wander leg must have a floor at every 2 units and no drop past 2.5 between two (`movement.go`'s `legOKLocked`: no cliff, no gap). **The water** (`entity/water.go`, F3 20261026-020316, jib's "Wildlife that is not aquatic should avoid naturally roaming into the water ... Same biome for damage"): the game's own, each world's BSP (0x8230) down to a zone leaf and its flags (0x3220's second word: 1 water, 2 slime, 4 lava, 8 mud), baked beside the ground by the same `cmd/groundbake` (`<world>.env`, 6 MB for the six worlds; `-only-env` writes those alone) and walked as CLIENT's FindLeaf (`ground.Env`); a body is in the water when half a unit above its feet is. A creature of the land never wanders into it, one of the water never out of it (by where it spawned, or npcmove's word); a fight takes a creature where its enemy is, but one of the water stops at its edge; a harmful spell across the edge is refused ("Your target is in the water." / "... out of the water.", our words), melee and throws across it are lost, both ways. Without the files nothing of it applies. **How the creatures of a name move**: table `npcmove` (GM panel "Creature movement", live; `movement.go`): `roam` -1 (the default; an older table's default of 1 is changed at the start, its rows kept) as its kind, 0 stands it on its spawn until pulled (F3 20261026-021730: the Hexbones), 1 wanders whatever its kind (a camp's roamers, on a player race's body too), `radius` its own, `water` land, both, water or empty (by its spawn); `tools/seed_npc_move.py [--eqoa-data ../external/EQOA-Data] [--apply|--undo]` writes creature names by whole words (the Hexbones and camp guards stand; crocodiles, caiman, crabs, tortoises both; fish water), roam 1 for the animals the words miss by the bodies they wear (a body 60% of whose creature names are wildlife's: spineback, widowmaker, razorgill) and for the camps EQOA-Data's map notes roaming (the Aviak Outpost's aviaks, Klik'Anon's pirate patrols), with wildlife.go's word lists (`TestSeedToolKeepsTheWildlifeLists`). The terrain (the zone rooms' collision) is baked from the operator's own disc by `cmd/groundbake -iso <image>` into `<worlds_dir>/ground/<world>.ground` (all six worlds 80 MB, 7 s; it needs ~2 GB, as the esf library reads Tunaria's 997 MB file whole, so bake on a big machine and copy the folder); the server reads a zone at a time (6 ms, at most 12; 32 kept, ~2.25 MB each) and without the folder keeps every height as before. **The bodies' sizes** (`entity/reach.go`, F3 pass 16): CLIENT's TargetInRange adds 3 to its 4.5 for a target whose npc flags carry 0x400 and 5 for 0x800, its flags' HIGH byte, the entity channel's byte 147, which nothing set: every creature, a dragon too, was reached at 4.5 from its centre and stood at 3.15 off. `cmd/modelbake -iso <image>` writes each CHAR.ESF body's edge (half the wider level side of its CSprite box) to `<worlds_dir>/models.txt` (568 bodies, a second), and a creature whose edge times its size is 1.5 past a man's (1.395) gets 0x400, 4 past it 0x800: the allowance whose gap to its edge is nearest a man's; no record of which creatures carried the bits on live exists. Without the file every creature is a man's size, as before. The database's `npc_type` never reaches those bits (its low byte is CLIENT's flags' low byte, 146; its high byte the class byte, 145) |
| `pathSystem` | Every tick | The paths GMs give NPCs (`path.go`, table `npcpaths`, the GM panel's map): waypoints walked in a loop, back and forth or once (then it stands at the last), at Sandstorm's walk (5) or the chase's run (8.83), the path's pause or a point's own at each; with the wander's step (`stepNPCToward`: on the ground, facing along the leg, velocity and walk/run on its channel), off the ground along the legs' heights. A pathed NPC never wanders (`wanders`) and walks with no player near; a fight, a dummy's fight or a charm takes it off its path and it takes it up at the end of the nearest leg; dead it starts again; its path taken away it walks home to its spawn (`paths.home`). Loaded at the start (`LoadNPCPaths`), set live by the panel (`SetNPCPath`/`ClearNPCPath`) |
| `petSystem` | Every tick | Summoned pets (`pet.go`): following (`npcmove.go`: an NPC's step with its velocity and locomotion on its channel), stances, rounds on their targets, their owners told their ids (0x007E), and taken away with their owners |
| `playerRespawnSystem` | Every tick | A dead player (`respawn.go`: 0x005E once, off every threat list, no regen) is restored 15 s after its death (Sandstorm's 15.6 s in `pcaps/breath_capture.pcap`) and sent a MemoryDump at its bind point (`charbind`, a spiritmaster's; never bound, the login spawn: `homeOf`): the dump, not 0x00B2, is what clears CLIENT's dead flag (Msg000D's ClearPC1B0). **A death leaves the live game's experience debt** (`deathdebt.go`; the user: "add xp debt on death like live"): its level's unrezzed debt (eqoa.fandom.com's Formulas chart, levels 6-54; SOE's Corsten state holds exactly five level 49 deaths of it), added to what is owed, told by 0x00D9 (the debt icon on the player's own target bar) and a line (ours); none below level 6, the chart's blanks (51, 53, 55-60) on its curve, ours; half of every award pays it (`mastery.go` splitXP); `#debt [amount]` shows or sets a GM's own. No resurrection yet (the live game's restored part of it) |
| `regenSystem` | Every 6 s | Health and power over time as the live game's (`regen.go`; eqoa.fandom.com's Formulas and HoT pages, F3 pass 14): HoT = max HP / 50 and PoT = max power / 50 (at least 1), plus worn items' HoT/PoT, the best in full, the second at `SecondItemShare` (40%, the Formulas page; the HoT page says 50%), each further one 1, plus a meal's HoT and a drink's PoT (`consumable.go`); every 6 s, in a fight or out of it (Sandstorm's capture: power ticked every 6.0-6.2 s through a fight); the stat channel carries PoT and HoT |
| `stateSyncSystem` | Every tick | Serialize player state to channel 0 |
| `statChannelSyncSystem` | Every 20 ticks | Serialize stats to channel 0x42 |
| `visibilitySystem` | Every 5 ticks | Assign nearby entities to channels 1-23, or the slots (and distance) the client asked for |
| `powerSyncSystem` | Every 5 ticks | Others' power (0x7E02) to a client that asked with 0x7E01: its group's members, its target and its target's target, players only, when the set changed (`powerbars.go`) |

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
  as a scribe (0x00B3); `quests`/`queststeps` rows are read again by the quest engine and `charquests` rows
  redraw an online character's journal. What the game reads only at a login says so in the answer.
- **Quests** (`ops_quests.go`, `questindex.go`, `questimport.go`, `static/views/quests.js`): `quest_list`, `quest_get` (with its
  problems, its status and the quests before and after it), `quest_save` (a quest and its steps as row changes, the
  engine reloaded once), `quest_delete`, `character_quests`, `character_quest_set` (through the engine, logged as the
  `charquests` row before and after, so revertible), `quest_try` (the GM's own character in the world),
  `quest_import_eqoadata` (one-step OFF drafts of any guide; a section `tools/import_quests.py` made whole is
  skipped). **Hundreds of imported quests** (F3 pass 18, the user's "update GM quest panel if needed since quests
  work now"; `questindex.go`): every quest's status is worked out from the database as it is NOW, one read of each
  table (`everyRow`, not `rowsOf`'s 1,000): **in the game**, **ready** (a draft nothing keeps out: its giver, step
  NPCs and creatures, items, spells and factions are here, its kill and loot creatures attackable, the names of the
  content's own `unresolved` note here, every quest it requires in the game or ready too) or **draft** with its
  reasons (`why`). `quest_list` filters by status (`blocked`: a draft not ready), city (the import key's; `none`),
  class, level or a range, giver, search, a name drafts lack (`missing`), with `limit`/`offset`, and answers the
  counts, every city's counts and `missing`, what the drafts taken lack, the most wanted first (Maryan 15, Brother
  Pendle 7...): placing that NPC frees every quest that lacked only it. `quest_set_enabled` turns the quests named,
  or a filter's, on or off, each its own audited row (revertible), the engine reloaded once: on takes ready drafts
  only (`force` for the rest, said in the answer) and brings in the ready drafts they require (a chain goes in
  together); `dryRun` first. The page: the filters in the address (`#/quests/f/status=draft&city=freeport`),
  status/city/classes/levels/giver/why columns, the lacked names as links, "Put the N ready in the game" and
  "Take the N in the game out" for what the filters show (a dry run's dialog first), and on a quest's page
  "Where it stands" with its chain and Put in / Take out. A GM's mended draft goes in from the panel at once; the
  importer's re-run (server stopped) is for new content files, and leaves a quest a GM has changed alone.
- **NPC paths** (`ops_paths.go`, `static/views/paths.js`): the World map's path editor (an NPC's Path…, or
  Path on the map on its page: `#/map/<world>/<npc>/path`): click to add points, drag to move, click a
  line to insert, Delete to remove; mode, pause, speed; Save, Revert, Remove path; the view fits the path,
  the NPC's place is polled and drawn as it walks. The ops `npc_paths`, `npc_path`, `set_npc_path` (a
  point's height left out is the ground's near the last point's) and `clear_npc_path` write one `npcpaths`
  row through the row editor, so each is audited and revertible; `livePath` applies it; `checkRow` holds
  the generic editor to the same checks. `tools/gmdrive.py` drives it (`mouse`/`key` steps).
- **The catalog** (`catalog.go`) is what each table and column means (labels, enums, npc_type
  flags, class/race bit sets, references, hex DictIDs); a table it does not know is shown raw
  and is admin-only to write. `accountinfo.pwhash` is never read or written through rows.
- **The game disc, in the browser only** (`static/disc/`): the GM picks their own ISO; a worker
  reads it in place (ISO 9660 walk), draws NPC bodies (`npcs.modelid` = CHAR.ESF CSprite,
  442/442 live values), item models (ITEM.ESF) and icons (`itemicon` = ITEMICON.ESF Surface,
  337/337), the map's zones from straight above (each world's zone table is in its last
  few KB, 0x8200 → 0x8210), and the spell visuals (DATA2/SPELLFX.CSF, a CSF inflated with
  `DecompressionStream`: 238 0xC200 records, each its DictID in 0xC210 and three event lists in
  0xC220; the only file on the disc with any, and every `fxdict` eqoaclassic recorded is one).
  CHAR/ITEM/ITEMICON.ESF and SPELLFX.CSF are kept in OPFS and zone pictures in IndexedDB, so
  later visits need no disc. The server ships and stores no game data.
- **Spell visuals** (`ops_visuals.go`, `views/visuals.js`; F3 pass 16): the page lists the disc's
  visuals with the spells using each; `play_visual` plays one (or a spell's) in the game from the
  GM's OWN character, logged in with the same account, onto its target (refused when it is not
  in the game), so a visual is seen before it is given; `set_spell_visual` gives a spell one through
  the audited row edit (a row inserted for a spell with none); `spell_visual`, `visual_uses` read.
  The game's `#fx`/`#spellfx` do the same; `gm_audit`'s statement is `database.GMAuditSchema`.
  **The preview** (F3 pass 17, the user's "a default human casting the spell fx ... so we can change
  without going in game", "for the castingcombat animation"): with `gm.preview_url` set, the page
  frames eqoa-rust-client-vanilla's `web/fxview.html` (the client's own `spells.rs` playing the
  0x00B1 a server sends, on two CSprites: `crates/eqoa/src/fxview.rs`), served with the play page
  from ITS host, never this server (no binaries here), and talks to it by postMessage only (origins
  checked both ways; the CSP's `frame-src` is that one origin, `'none'` without it). The disc worker
  hands it Blobs of the six files it reads (`previewFiles`: CHARCUST/CHARFACE.CSF, CHAR.ESF,
  SPELLFX.CSF, FX.CSF, ITEM.ESF; FX/CHARCUST/CHARFACE now kept in OPFS too). What a cast lands on
  (the user's "if offensive spell ... a second human or training dummy for the target"): a spell
  whose line is harmful (`spell_visual`'s `harmful`, `SpellDef.Harmful`) at a training dummy (body
  0x66262F1C at the Training Dummy rows' 0.25) before the caster, anything else on the caster; a bare
  visual at the dummy; the pane's select overrides (dummy, a second human, the caster). The preview's
  host must allow the frame: nginx `frame-ancestors https://gm.<domain>` on that file alone.
- **Community visuals** (`ops_suggest.go`, `views/community.js`; F3 pass 17, the user's "our community
  effort to help match the spell fx to the correct spells", "no sign in gates", "GMs accept in the
  panel"): players send from the public spell-visual page (eqoa-rust-client-vanilla's
  `web/spellfx.html`) which visual they think a spell plays, through the ACCOUNT server's two open
  routes (`internal/spellfx`, wired as `tcp.AccountServer.SpellFX`): `GET /api/spellfx/spells` (every
  class spell with its classes, line, harmful, target word, cast held to 30 s, and the visual it plays
  now, `standin` when that is its kind's stand-in, and `got` ("mastery", "epic", "master class") with `master` for a spell no level gives (`database.GotBy` of its rows' sources); one query, cached, rebuilt by
  `database.InvalidateSpellFX` (`SetSpellVisual`, a row edit of spellpattern/spelleffect/classspells)
  or after a minute) and `POST /api/spellfx/suggest` (`{spell, visual, name, note}`: a 32-bit hex
  DictID, a name of 32 characters and a note of 300 at most, plain text; an address, an IPv6 one by
  its /64, holds one live suggestion a spell, a new one replacing it, with no hourly cap (`spellfx.DefaultPerHour` 0; a player matching a class's whole list was stopped at 120); 100,000
  waiting is a full box). Table `spellfx_suggestions` keeps the sender only as a salted hash
  (`spellfx_meta`'s salt, made once and hidden from every page: with it an IPv4 hash walks back). The
  ops `visual_suggestions`, `accept_visual_suggestion` (set_spell_visual's audited row edit, so
  revertible; that visual's suggestions accepted, the spell's other waiting ones dismissed),
  `dismiss_visual_suggestion`, `clear_visual_suggestions` (a sender's rows by hash prefix); the page
  shows them beside the preview (`previewPane`, exported from `visuals.js`), player text as text.
  nginx proxies `/eqoa/account/api/spellfx/{spells,suggest}` (exact locations).

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
- Targeting (select NPCs/players, con colors as Sandstorm's, the face per player: red exactly for a creature that attacks that player on sight, `entity/aggro.go`)
- Basic combat (attack command, damage, death; a player respawns at its bind point, else the login spawn, 15 s after dying)
- NPC AI (aggro, melee in Sandstorm's rounds — `entity/rounds.go`, leash, respawn; NPCs helping their own and healing out of a fight — `entity/socialaggro.go`, `npcregen.go`)
- Corpses as live EQOA's: lie 3 minutes unless emptied (an empty one opens "Empty"), emptied and closed they go at once, and the creature returns a minute after its corpse went (`entity/corpse.go`); wildlife wanders round its spawn, every other creature and the townsfolk stand until pulled unless a GM's Creature movement row says otherwise (`entity/roam.go`, `wildlife.go`)
- **Loot tables** (`database/loottables.go`, `handlers/loot.go`'s `rollLoot`; F3 pass 18, the user's "if you see the loot tables for them add them"): a corpse holds its `npcitem` rows and what the tables bound to its NAME roll at its death (once a death, when first opened): table `loottables` (a row an item of a table: weight, the creature levels it drops for; GM panel "Loot tables") and `npcloot` (NPC name as npcclass keys it → table, a roll's chance in percent, rolls a kill; "Creature loot"), both read at every corpse, so live. `tools/seed_camp_loot.py --db eqoa.db --eqoa-data ../external/EQOA-Data [--apply|--undo]` writes EQOA-Data's own (its items' sources: 13 biome tables "arctic" ... "generic", 3,283 items matched by name to itempattern, each for creatures within 5 levels of its own, 55 and up to 70) and binds the 49 XP camps' creatures (`#tele camps`, 155 names) to their zone's biome table on EQOA-Data's map (a zone's biomes are its NW, NE, SW, SE quadrants, the order neighbouring zones agree on) at 25% a kill and to "generic" at 15%; Odus's zones have no biomes, so its camps get generic only; rathe mountains, red desert, deep jungle, wetlands, wastelands and the Khals' are written but bound to no one. **The chances, one roll a table and the level window are ours**; every row logged in `camp_loot_seed`, `--undo` deletes those no GM changed. No restart needed
- Wildlife where no import has any (F3 20261025-172939, -173551): `tools/seed_wildlife.py` turns FrontiersForge's sightings (`npcs_ff_import`) of generic creatures in Klick'Anon (levels 1-12) and the Rogue Clockworks (11-23) into a few spawns round each, copying a namesake row's body (a name with none is left out and listed), on the ground files' terrain, out of towns (`towns.go`'s, 120 off) and apart; a camp's creature round the camp; `--with-guide-species` adds the Klick'Anon guides' bats, rats, snakes and fireflies. Dry run by default, `--apply`, `--undo` (every row back), logged per row in `wildlife_seed_npcs`; stop the server to apply and start it after. The counts and spreads are ours
- HP/Power regen system
- **The quests' missing people and creatures** (`tools/seed_quest_npcs.py`, the user's "add the missing NPCs for the quests"): every NPC and creature the class quest files (`tools/quests/data/*.json`) needed and `npcs` lacked, placed where FrontiersForge saw it (`npcs_ff_import`), else at EQOA-Data's map point (Tunaria by a fit to the unique-named NPCs, a median 7 off, each point moved by the nearest misses the sightings show; the Rathe, an inset of the sheet, by Tunaria's scale anchored on Oggok), else beside an NPC the guide names the place by, on the ground files' floor nearest its neighbours' height; a body copied from an NPC of its race (charactermodel) or kind (its name's words), npc_type 0 for what a quest kills and 128 for whom it talks to, npc_id 0, a few spawns for a kind killed in numbers (SPECS, one entry a name, each with the guide's words for its place); the three kill targets that stood as townsfolk (Squire Janxt, Gamor, Furdle Nimdar) made attackable. Dry run by default, `--apply`, exact `--undo` (`quest_npc_seed_rows`, `quest_npc_seed_types`; a type a GM changed since is left); stop the server to apply. The quest files spell what it adds as the game does (backticks in Teir'dal names, "Dragoon V`Ghen") and list in `unresolved` only what is still missing with it in. Places, levels and bodies where no sighting exists are ours
- Breath and drowning (`systems.go` `breathSystem`): under liquid (the 0x40's UnderLiquid) Sandstorm's breath, eight steps of 32 every 6 s, on the stat channel's byte 232; then a tenth of the maximum health every 6 s until dead (F3 20261026-020342, jib's; Sandstorm's one recorded drowning, a level 1, lost 2 a step)
- Hotkey menu (L2 D-pad) — 25 default hotkeys, fragmented MemoryDump delivery
- Spell scrolls: the item menu's Use scribes one (`scribe.go`: a scroll is the scroll's art plus a spell of its name; Sandstorm's 0x003C + 0x00B3 and its refusal lines, recorded); `tools/import_sandstorm_harvest.py` stocks the spell vendors
- Food and drink (`handlers/consume.go`, `entity/consumable.go`, F3 20261025-172627, jib's account of the live game): the item menu's or a quick slot's Use on an unworn item described as a meal ("A hearty meal that will increase health regeneration.") or a drink ("A refreshing beverage that increases (aids in) power regeneration.") takes one (0x003C, as a scroll leaves) and adds the item's own HoT (a meal) or PoT (a drink), else its level, to the six-second regen for a minute (`StatMods.HoT`/`PoT`, so the stat channel's HoT and PoT too), shown on 0x43 by the item's name and a buff's 32x32 UI.CSF icon, a meal Regenerative Aura's (0x828563AF) and a drink Breeze's (0xA2774C47), never the item's own (`itemicon`, the inventory's 128x64 ITEMICON.ESF tile, which VIWndInfluence blits whole over its neighbours); a new meal replaces the last, a meal and a drink stay on together, and both are kept over a logout. Both end once auto-attack engages or the player lands a blow or a harmful spell or is hit (`consumableSystem`: `LastCombatTime` past the meal); refused in red below the item's level (Sandstorm's line) or with auto-attack on (ours). `tools/stock_grocers.py --prh <jib's export>` stocks the grocers: each sells Return Home's food line (13 meals, levels 2-49) or drink line (14 drinks, 2-54), a pair one of each, a grocer with none near both; prices 10 tunar a level and HoT/PoT the level where the item has none; `--undo` byte-exact. **The amounts, prices and the minute are ours, not measured**
- **Potions** (`entity/potion.go`, `handlers/potion.go`, table `itemeffect`, `tools/seed_potions.py`; F3 20261025-223151, -223841, -223959, jib's design): an alchemist's spell whose words summon an item ("SELF Summon Watery Draught", "Summons Unstable Potion X3": EQOA-Data's, as `spelleffect` holds them) gives the caster that many of it at its landing (`summonItemsLocked` → `summonItem` → `GiveItemNow`): the item by name, else by the spell's own name; the words' xN, else three of a potion (jib: "stacks of 3", F3 20261026-014336) or of anything stacking to three, one of anything else. An item with an `itemeffect` row is used, not worn: drunk (`Self`: its health taken at once, refused in red when that would kill; power, speed, invisibility, a resist, "Dispel Effect xN" harmful effects off, a gate) or thrown at the client's target (`Target`: castTargetsLocked's refusals, range `PotionRange` 20, landed as a spell's hit with its 0x00B1, and on every creature within its `area`); one leaves the stack by 0x003C. A thrown potion is a ranged weapon (F3 20261026-014336, jib: "equips in the range attack slot, like a bow, and fires as a range auto attack"): wear 16, a bow's (the rusty throwing knife's), the client's item type (`procanim`) 0 so no potion quick slot takes it, attack type 10, `unk5` the range; every Use wears it in the ranged slot, taking the hands off as a bow does (with a weapon in hand CLIENT's GetSwing swings melee), and worn it is each ranged auto-attack's throw (`Swing.Potion`, set by `swingFor` at every 0x000F; one off the worn stack each, the fight let go with the last); only a thrown potion worn nowhere (a GM's) is thrown by Use. The crude potions' effects last a minute (F3 20261026-014527). `tools/fix_potion_rules.py [--apply|--undo]` makes both changes on a database `seed_potions.py` filled before them (the potion quick slots holding a thrown one emptied). What a potion puts on ends early as its row says: `attack` (its bearer's auto-attack or harmful cast, `potionSystem`, or a throw), `damage` (`LastCombatTime` past its start), `wear` (% a six-second tick); its effect id is `-1000000 - patternid`, so a second replaces the first and the rules survive a logout. `tools/seed_potions.py [--apply|--undo]` writes the 45 potions of EQOA-Data's alchemist list from id 50001 (the Mixtures' Mix Potion lines, the cocktails' Fire Potion lines with a splash of 10, draughts, quickening and sublimation, tonics, correctives, restoratives, the returning potion; crude and faulty spells' potions NO TRADE) and the Klik'Anon Cocktail made a thrown weapon; restart after. **Ours, not measured**: the draughts' numbers (4 health a level for 15 power a level over 18 s), tonics past 30, the cures' counts, the splash, the range. `itempattern.trade` is CLIENT's `m_noTrade` (VIItem +0x1C): 1 is NO TRADE (the panel said "Tradeable"), and a trade refuses such an item (`trade.go`)
- **Invisibility ends on any action but moving, targeting and chat** (`handlers/invisactions.go`, `World.BreakInvis`; F3 20261026-014527, jib: "when using a potion while invis'd, the invis should break. Effectively any action other than movement, targeting and chat should break invis"): the Registry ends the sender's invisibility before each action message is handled (an item used, drunk, eaten, scribed, thrown, worn or taken off; a weapon set; talking to anyone and answering; merchants, the bank, blacksmiths, the auction, trades, looting; pet commands; emotes), as casting and attacking did already; the 0x40 channel, targeting, chat, the player's own windows, the group and the lists are not actions. A pet's invisibility still ends only when it fights
- Casting (0x0032, `entity/cast.go`): Sandstorm's start (0x00CD lockout = cast + recast + 300 ms, then 0x00B1), landing 1.4 s after the cast time, refusal lines and silences, all recorded; what a spell does is read from its EQOA-Data line in `spelleffect` (`entity/spelltext.go`, filled by `tools/import_spell_effects.py`): hits, heals, ticks, stat buffs on the stat and effects channels, speed, invisibility, Return Home, summoned pets and charm (below), procs (`entity/proc.go`). Not yet: cures, resurrection, damage shields. **What the lists leave out** (`entity/spellnotes.go`, F3 20261025-222444): a line raising stats with no time lasts 30 minutes (Courage's did nothing), the cleric's HP/AC line heals what it raises maximum HP by (by name, jib's rule), a spell raising max HP heals after the raise; radii, a buff's hate and an epic's power/cast/recast are `SpellDef.Ignored`; `TestEQOADataLinesRead` (`EQOA_DATA`) counts what is read of all 1,693 lines. **Heals over time** as jib has the live game's (`entity/hot.go`, F3 20261025-222354): the first tick at once unless one is on, the rest on the regen tick (`World.regenAt`; Sandstorm's six-second rhythm from the landing until a regen tick has run), a weaker over a stronger does nothing, a stronger takes the next tick with all its ticks (`ActiveEffect.Ticks`, kept over a logout). **`tools/import_eqoadata_spells.py`** makes the records of EQOA-Data's leveled class spells the server lacks (ids from 700001; icons by the list's names as UI.CSF file hashes or the records' agreement; cast, recast and power from another rank of the line, OURS), their lines, classspells and scrolls at the class's vendors in that band; `--undo` exact. **`tools/import_eqoadata_mastery_spells.py`** (the user's "add the missing level 50+ spells to the database so players can match them") makes the rest on top of it, with its own log: the masteries ("CM"), the level-60 epics and the master classes' ("MCA", "<master> MCA"), at every level (eqoaclassic: 429 records, 221 of them 50+), their lines, and classspells rows of source `eqoa-data cm|epic|mca|master <name>` (`database.GotBy`/`Granted`, `grantsource.go`): the community page lists them with `got`/`master`, and no class grant hands them out (`GrantClassSpells` for #template and start_all_spells, `grant_spell` "all", a pet's taps), as the were forms are not; `--retag` (off) would mark the class-spells importer's plain rows of such names too; `tools/import_class_spells.py` writes those sources itself
- **Life taps and pacts** (`entity/lifetap.go`, F3 20261025-215541, -215810, -214751, jib's asks): a drain ("Drain HP by N", "Decrease Drain HP", "... and transfer to caster", "give self") heals its caster by the damage dealt, the whole figure though it kills ("Drain Group HP": the caster's group in range); a stat or power drained is the caster's for the line's duration (an effect of the spell's name, its id negated). A clause "to Self", "Self Only", "Increase Self AC", "Increase Caster HP" lands on the caster once a cast, before the targets: a pact's price ("Increase Hitpoints by 25, Decrease Hitpoints by 50 to Self") less the share the caster's AR takes (`entity/resist.go`, below), and it can kill; a caster's own effect does not make a spell harmful, so a pact goes on a friend or oneself (price first, then heal). These words used to be cut off: every tap was a plain nuke, every pact a small heal. **A necromancer's pets tap** (`entity/pettap.go`, -215804): a summoned pet (not a charmed creature) of level 20+ casts the necromancer's best single-target tap at or below its level every 30 s while it fights in reach: the spell's visual from the pet, the hit as its own (the owner's figure, hate, XP), as much on the pet; the taps read from `classspells`/`spellpattern`/`spelleffect` at the start and after a GM edits any of them (`World.LoadPetTaps`). `tools/fix_taps_pacts.py [--apply|--undo]` gives each tap's and pact's row its own class's line at its level (the effect importer's name collisions gave the necromancer's Siphon Life L16 the L1 line, its Siphon Strength the shadow knight's) and makes Infernal Pact's 1820 the 1080 every other pact's 2x gives; run it after any spell import. The 30 s and level 20 are jib's; the rest is ours
- Summoned pets (`entity/pet.go`, F3 20261025-044023; `claude-notes/pets-charm-client-20261021.md`): a line that summons ("Summons an Iron Golem to your defense", "Summons a pet") lands as a pet named as the spell less a leading "Summon ", at the spell's level or the level its line names ("Summons a pet, level 30"; at most the caster's), its body the most common of this server's NPCs of that name (else the line's creature, the name after a possessive, a class stand-in), a creature's HP and melee of its level. As Sandstorm's recorded Fire Elemental: 0x007E with its id (again after every MemoryDump), 0x1003 on the owner's non-collide list, byte 142 3, npc flags 0. It keeps the first entity slot, follows, comes along through a teleport or into another world, fights in rounds (its hits and kills the owner's; what it hits fights back), and takes 0x0041's stances (defensive a new pet's), dismiss, back off and attack, each answered in a line; scope-3 spells land on it; no player can harm it. It goes (0x007F, 0x1004) when dismissed, at its death or its owner's, and at a logout. Pets' damage is not measured (a creature's roll of its level less the target's defence)
- An enchanter's pets swing once a round at any level, Grand Animation twice (`NPC.SwingFixed`, F3 20261025-164209); every other summoned pet swings as a warrior of its level (`newPet`: four hits a round from 35 with no shield; it took its template creature's `npcclass` row until F3 pass 18, the user's "at level 35 and above melee/war classes quad attack, and many pets/mobs are missing that": a shaman's Primal Spirit was a magician and single-hit at 60); a charmed creature keeps its own class. Charm (`entity/charm.go`, F3 20261025-164922): the Entrancing line makes the creature the caster's pet in place, as Sandstorm's recorded charm (0x007E with its own id, byte 142 3, 143 0, npc flags 0x80); refused with a pet already (a 0.6 s fizzle) or above the charm's level (its words' "up to level N", else the spell's own); resisted by undead (the tag below), dummies, else 10% less the line's CHA share, then it fights the caster; let go at the line's duration or on dismissal (hostile), at its master's death or logout (quietly); dead as a pet its corpse is no one's to loot and it returns itself. Procs (`entity/proc.go`, F3 20261025-165245): a hit whose ability description names a proc (Chaotic Flux, Mark, Glyph) is cast on a friend, player or pet, and its bearer's melee hits may do that damage, no mod: 3% + 1 per 20 INT (at most 25%; a pet as 3 INT a level), all resisted by 5% + 3 a level the creature is above the bearer. The chances are chosen, not measured
- **The enchanter's lines** (`entity/enchanter.go`, the user's "the enchanter fixes, but in our own way"; sources: EQOA-Data's lines and its masteries list, `Information/databases/cms/enchanter.json`): the procs go by their names (`procFamily`: Chaotic Flux/Mark/Glyph/Rune, the Anarchic four, Stormlord's Rune, Rune of Animation), whatever the record's description says (only the dump records of Chaotic Flux, Mark and Glyph name a proc: Chaotic Rune's imported record hit for 256 as a nuke, and the Anarchic line put an empty icon on the group); a proc line's scale is its caster's at its landing (Stormlord's Rune's DEX), or its bearer's at each proc when the line says so ("; Increase 20% based on bearer's STR"), held to a "max N" clause when it has one; a group proc lands on the members' pets too (the list's "Group melee-proc buff"); the clarity line (Breeze, Intuition, Clarity, Lucidity, Tranquility, Empathy and the Empathic ranks, the list's "Group/AoE PoT buff") is one at a time on a player, the most power a tick winning as a heal over time does (a weaker one does nothing, the caster told "<name> already has a stronger power over time."; a stronger one replaces it with no tick at once); an arcane resist debuff on a creature (Paralytic's "Decrease Resist Arcane by N", the list's "AR Debuff") takes ResistShare(N) of a hundred off its chance to resist a proc or a charm (25 AR: 6 points; the strongest one counts, the ranks not adding up; ours, by resist.go's scale); Earthen and Ferrous Golem wear a granite and an iron golem's body (`petBodyAliases`, ours). Mana Burst, Power Stream and Flux (instant power to a friend), Mana Flow and the Empathic ranks (group power over time) worked already. `tools/fix_enchanter_lines.py [--apply|--undo]` gives four rows their words: Entrancing Charm (no line: the list's "Entrancing Charms"), Mircyl's Animation (an empty one: "Mercyls Animation"'s), Anarchic Glyph's 15 read as Chaotic Glyph's 156 (ours), Rune of Animation's "Weapon proc" (no numbers in the list) as Stormlord's Rune's on INT (ours); it leaves a GM's line and any row another source wrote alone
- **Spells for undead alone** (`entity/undead.go`, F3 20261025-215937, -222603, -220159, jib's asks): a "Target undead" line (the cleric's and necromancer's Ward, Smite, Disrupt, Torment and Persecute Death, the shadow knight's Punish Death) and a charm whose name or description says undead (the necromancer's Charm, Command, Control and Coerce Undead, lines a plain "Charm") are refused at anything not undead with "This spell only works on the undead." in red (EverQuest's words: no recording of the live game's refusal exists), a 0.6 s fizzle, no power, no lockout; an undead charm takes undead on the charm's own roll. **Which creatures are undead** is table `npcundead` (name as `npcclass` keys it, `undead` 1/0, GM panel "Undead creatures", live), else a name's whole words (`reUndeadName`: skeleton, zombie, ghoul, mummy, wraith, spectre, ghost, rotting, decaying, soul, dead...; no longer "bone", which made the Hatebone orcs undead). `tools/seed_npc_undead.py [--apply|--undo]` seeds it from the names and from 22 CHAR.ESF bodies skeletons, zombies, mummies, ghouls and spectres wear (eqoaclassic: 259 names, 142 by name, 117 by body: the Hexbones, the returned Indigo pirates, haunted townsfolk); restart or edit a row after it
- For testing, `world.start_level` (a new character's level; a client's creation message never picks it) and `world.start_all_spells` (every spell of its class, from `classspells`, filled by `tools/import_class_spells.py` out of Sandstorm's dumps, its vendors' scrolls and EQOA-Data's class lists, weres left out), both creation paths (`database/newchar.go`). Without it a new character has what Sandstorm's fresh ones hold (`database.StartingAbilities`): one ability of its class, then Return Home; measured for ten classes, chosen for the bard, shaman, magician, necromancer and wizard
- Training dummies as Sandstorm's (`entity/dummy.go`, measured; `claude-notes/training-dummies-20261021.md`): "Training Dummy …" and "Regular Dummy …" never die, fight one player in rounds, and a Training Dummy heals 300 after each hit
- A **Sparring Dummy** (`entity/sparring.go`, `handlers/sparring.go`, `sql/sparring_dummy.sql`; F3 20261025-174503, the user's ask): talked to, it asks a level (10-60) and an element (fire, cold, lightning, arcane, poison, disease); struck, it fights in Attack4 rounds of that level (a creature's roll) and every second round strikes with its element, 10 a level spread ±20% less the share the player's resist takes (`entity/resist.go`), said in the chat with the share; it never kills (it stops at one hit point). **The strike's numbers are ours.**
- **A resist's share of a spell** (`entity/resist.go`, F3 20261026-015346, jib's rule): a resist counts up to `ResistCap` 315 and takes 75% of the damage there, in proportion below it (the line below the cap is ours); at the cap 2.5% of spells find it failed altogether (all of it lands, "your resist failed") and another 5% failed in part (it takes 45-74%, "your resist took only N%"). Where a resist cuts a spell: a sparring dummy's strike and a pact's price; creatures cast nothing at players yet, and a proc's resist is the creature's level chance (`proc.go`). Until then the share was resist/(resist+200), uncapped
- Travel and binding (F3 pass 15): **coach menus list towns** (`entity/coachtowns.go`: every coachman named for its town off EQOA-Data's map laid over Tunaria, a GM's new one by the town within 400); **waysenders** (`handlers/waysender.go`: the routes EQOA-Data's quest guides give, their fees where they give one, Waysender Brindel to Lavastorm's outpost for level 55 and up, "too weak" below; one route a Yes/No, several Sandstorm's "Where do you wish to go?"); **spiritmasters bind** (`handlers/spiritmaster.go`, table `charbind`: "Do you wish to set this as your bind point?", 3 in front of it, and its cast onto the player as Sandstorm's Zole's: 0x00B1, 3000 ms, visual 0x08F3B955, `World.NPCVisual`; Return Home and the death respawn go there through `homeOf`, else the login spawn). A new box replaces the open one (`forgetDialogs`). **The Plane of Disease** (F3 pass 17, "unable to travel to PoD"): the Secrets world's "Last Home" (world 5, zone 0); Necrologist Zizclank in the Rathe's Dead Hills (NW) (world 1) sends a level 50 to the portal players arrive at (5063, 3881), which sends them back, and the Whirling and Bloody Portals take a level 60 to the Queen's bowl and the Twins' valley and back (an NPC with trips in `wayRoutes` is a waysender whatever its name); `#tele pod` / `deadhills`. `tools/seed_pod.py [--apply|--undo]` places them and PoD's people from EQOA-Data's guides (91 rows, logged in `pod_seed_npcs`; npcundead/npcmove/npcaggro rules in `pod_seed_rules`); spots, levels and bodies are ours, the live flag quest and PoD's other quests are not in, the bosses are always up (no spawn by event) and there are no loot tables, so the PoD Drop spells drop nowhere.
- **ModelSize is Sandstorm's** (`entity/stats.go` `NPCSize`, `PlayerSize`): an NPC sends its row's own size and a player 1.0, whatever the race (the captures: every race's players 1.00, a gnome NPC 1.00 or 1.20, the rows' sizes exactly the wire's); a table of race factors (a gnome 0.45) drew gnome NPCs at under half size until F3 20261025-173632.
- A player's melee on an NPC as Sandstorm's (`entity/offence.go`, measured on 1,594 swings; `claude-notes/offence-sandstorm-20261022.md`): the roll from the worn weapon's DMG and condition (a broken one a fifth; `Gear.WeaponCond`, `WeaponTwoHanded`), STR raising the top and DEX the bottom, an archetype's offset, and the target's defence by its level; `offence_test.go` holds it to Sandstorm's swings. The attacker's own level was not measured (all L49-51); the default Khal Sword is DMG 999 and hits like it
- NPC melee on a player as Sandstorm's (`entity/defence.go`, measured on 2,660 dummy hits; `claude-notes/defence-sandstorm-20261022.md`): max(floor, a roll from top − width to top), the top falling with the player's AC (by the NPC's slope) and AGI (by the archetype's: tank 0.305, melee 0.227, caster 0.133) and raised by the archetype (melee +15, caster +32 over a tank); the measured dummies' own rolls, a creature's a provisional line through them (floor half its level). `defence_test.go` holds it to Sandstorm's hits (`testdata/sandstorm_defence_hits.txt`). Priests are set between melee and caster, unmeasured; NPC specials are not done
- The HUD's item quick slots and weapon sets, as SOE's live client has them (`handlers/gearsets.go`, tables `quickslots` and `gearsets`; `claude-notes/live-toolbars-gear-sets-20261024.md`): the item menu's Prepare (0x000A, answered with the list index; a potion on 0-1, a gem on 2-3, on a PS2 a weapon on 3, kept and sent in the MemoryDump), a weapon set (0x0021: the hands as worn, named; all four sent at world entry, which a PS2 skips) and taking one up (0x0022; 4 is bare hands); the quick slot's use is 0x003E with interaction 1, the item menu's use. A potion does what its `itemeffect` row says (below); a gem does nothing yet
- Spell effects kept over a logout (`entity/buffsave.go`, table `charbuffs`), their time running only in the world, as Sandstorm's: saved at a logout, a re-login's takeover and for everyone at a shutdown (`Listener.SaveAll`, positions too), put back at the login (the world entry's 0x00F8 the buffed speed)
- Stats, masteries and levels as the live game's formulas (F3 pass 14, jib's reports; `eqoa.fandom.com/wiki/Formulas` and its 2010 forum copy `eqoa.forumotion.com/t279`): **stat caps** are CLIENT's (`mastery.go`: a stat shows min(total, the dump's `m_p44` plus the mastery block's raises); 350 to level 45 and 400 from 46, read off SOE's retail save states at 45 and 60, plus the cap masteries the character bought (400 at 60 until one is); master classes' 600 and racial raises are not modelled); **HP** = (factor + STA/11) × level and **power** = (28 + P/10 + S/20) × level, both truncated (in integers), P and S from CLIENT's class table; **training points** 3 a level to 14, 5 to 29, 9 to 44, 14 to 60, given as a level is left, 0x0036, topped up once at a login for a character made or set at a level, spent through the trainer's 0x001D by its own rules (`trainRefusal`: no stat's base past `m_p44`; a refusal is answered 0x001D 0, so the window chooses again; a stat items hold at the full cap still takes points into its base); **mastery points**: the Mastery Points window's share (0x1401 sub-op 1, 0-100 by 1, table `charmastery`) splits each kill's experience below the level cap, and at 60 all of it goes to them whatever the share (jib's rules, F3 20261026-042328: `splitXP`); while an experience debt is owed (`characters.debt`, kept at a logout; 0x00D9 tells CLIENT what is left, its debt icon on the player's own target bar) half of every award pays it, the rest by those rules, but nothing makes a debt yet (no death penalty); point n costing round(125000 × 1.0061^(n-1)) (the wiki's table, all 151 values), `#grantcm <amount>` (GM, own character, spent + unspent at most 1500); masteries are bought in the game's Class Mastery window (`entity/cmbuy.go`, table `cmcatalog`, one row a rank): buyable once its level, points spent, base stats, the abilities it needs, the rank before it (`parent`) and any other mastery's ranks it names (`require_ranks`, their ids) are there, one master class at a time; a rank another one needs is not removed before it, each rank's figures adding to the ranks' below it (EQOA-Data's lists give each rank its own: the cap masteries' make the live 550 at 60 only so, and the gods' blessings so give 5, 15, 30, 50 and 75 of their resist), and a removal gives nothing back (jib, F3 20261026-043542: "permanent unless you delete them, which lowers your total cm's as you do that have to be re-earned"): the points leave the spent ones and are earned again; a GM's `#cmreset` returns them. **A kill's experience is the game's own** (`killexp.go`, F3 20261026-031118, jib: "We do not appear to be able to earn mastery points"): CLIENT carries the live server's model compiled in and never called (viexperience.cpp: `LevelExp` = (L+1)² × 40 × (1 + 0.02 L), 5% a level the creature is above or below the group's highest, 0.8 a member + 0.2, nothing below three quarters of the highest level, each member's share by level squared and at most three times its own level's worth), and Sandstorm gives exactly it (a level 2's solo kill of a level 2 Orc Pawn: 374); shared with the group's members in the world within `KillShareRange` (200, the quest kills' rule; the range is ours). Until then a kill was the creature's level × 100 (a level 60 got 6,300 for a level 63 where the model gives 425,820), so a mastery point took ~20 kills and leveling past 30 hundreds of times what the level table asks; the masteries' share now says Sandstorm's "You received N class mastery XP.". `CharXPDict` was the C# table, 4% above `ExpForLevel` from level 50 (its lower branch): now ExpForLevel itself. The **level table stays CLIENT's own** (`ExpForLevel`, what the bar divides by): the wiki's debt × 25 chart is far below it under level 30 (the user's choice). The **movement rate** (`cast.go` `MoveRate`): 20 running, 100 Spirit of the Wolf and the most, masteries between (`MoveRateBonus`), 20 the run speed 8.833333 and 100 Sandstorm's measured SoW (1.3 times), linear between
- **Quests** (`handlers/quests.go`, `questgm.go`, `database/quests.go`; the GM panel's Quests page; F3 pass 16, the user's "quest tools added to gm panel"): tables `quests` (the giver by NPC name, levels, classes, races, quests first, `enabled` 0 a draft), `queststeps` (kind talk, show, give, kill, loot, reach; the journal line, the NPC's words, the rewards; `chance`; `events`) and `charquests` (step, kills so far, times finished), which replace Return Home's never-filled `quests`/`playerquest` (`initQuests`: dropped when empty, renamed `*_returnhome` with rows). Talking to an NPC (0x0004) asks the quests first: a step to do at it, else a quest it offers the player may take; the giver's offer is a paragraph a question (0x0034, its id `0x51xxxxxx`, which 0x0035 echoes and no other box uses), answered by `accept`'s lines; then, **in Sandstorm's order as questrec16 recorded its first Freeport warrior quest**: 0x007C the journal line, "You have received a quest!", the giver's words (an NPC's lines named for it, its questions not); a step done: the NPC's words, 0x007D (and 0x007C for the next step's line), "You have finished a quest!" after the last, then experience (`World.AwardXP`), tunar, items (`GiveItemNow`), faction lines (`World.ApplyFactionHits`), spells (a scribe's 0x00B3). `show` keeps the items (Sandstorm's bracer), `give` takes them (0x003C). Kills: every kill a player is credited with passes `factionKillLocked`, which tells `World.OnKill` (`entity/questhooks.go`), shared with group members within 200 in the world; the engine takes them off a queue (never under the World's lock). Reach steps are looked at once a second. The journal: the MemoryDump's `m_p2BB0C` token and lines (at most eight), 0x007C/0x007D carrying a token that counts up; the player's own delete (0x007D in) gives the quest up (a finished count kept), ignored under an old token. A draft is offered to no one but goes on for a character who has it (`quest_try`). Also as recorded: a middle step announces "You have finished a quest!" and "You have received a quest!" after its line moves; a show or give step's question offers only `notyet` while the items lack (then `reminder`); an answer's further lines are dead ends ("Farewell"); a refusal's reply (`declined`) carries no name; `%p` is the player's name; a spiritmaster not yet bound to offers its bind before its quest step; the XP line is Sandstorm's "You receive N XP." (`awardXP`, kills too). **Not Sandstorm's**: its faction lines "11 faction with X (+10)" (ours EverQuest's words); its dialog ids count up per session and its journal token stays 0 (ours counts, so a delete under an old one is ignored); our kill steps say "Name: n of N." (Sandstorm's recorded quests had none). **Loot steps and events** (`handlers/questevents.go`, `questloot.go`; F3 pass 18, the user's "we need quests for all race/classes, and events so quests can trigger popups etc."): a `loot` step is a kill step whose kills drop its item (`items`' first, `count` of it to take): each kill of `target` credited to a player on the step, while it has taken fewer, drops one with `chance` (0: every kill; the roll is ours) onto THAT player's own view of the corpse, after the corpse's items (`KillCredit.NPCID`/`Died` name the corpse; `NPCWindowHandler.AddQuestDrop`), so no one else sees or takes it and the corpse goes only when its items and every drop are gone; what is taken off corpses while it is the step to do counts (`charquests.count`, as a kill step's kills; `QuestEngine.Looted`, told after the loot lock), items held from before do not, and the step is done at its count. **A step at a merchant, banker or blacksmith** (`QuestEngine.AtWindow`, F3 pass 18: 291 imported steps in 221 quests): CLIENT never talks to an NPC with a window (InteractWithTarget asks for its window: 0x004C, 0x124D, 0x0061), so such a talk, show or give step is done when its window is asked for, first, when it can be done now (its items and tunar held), the client's pending request cleared (0x00D7); else the window opens as ever; and a quest such an NPC gives is offered there first, once a session a quest (the user's "have shop NPCs offer the quest first": declined or left unanswered, the next request opens the window). `events` (a JSON list on `quests`, `on` accept or finish, and on `queststeps`, `on` start or done; `database.QuestEvent`) run in order after the engine's own words at that moment: `popup` (a window the player closes: Msg0034's box with one answer, "OK" unless named, the only server-opened box CLIENT waits on; text past its last 127 characters in 0x0046 boxes first; one at a time, as CLIENT shows one question box at once), `say` (an NPC's words, a box a line), `give` (`patternid*count`), `spawn` (a copy of an NPC four steps before the player for the step, `World.SpawnForQuest`; `hostile` makes it attackable, NPC bit 0x80 cleared, and sets it at the player; gone when the step is done, the quest given up, reset or completed by a GM, its character away, or after ten minutes), `visual` (a DictID played from the player on itself, `World.PlayVisualOn`), `teleport` (a travel NPC's warp, refused for a world the client cannot load). The GM panel's quest editor edits both (`quest_get`'s problems name a bad event, a spawn or give of what this server lacks, a loot step's missing creature or item, a chance past 0-1). **Content**: `tools/quests/data/<city>.json` (`tools/quests/FORMAT.md`) written from EQOA-Data's 1-20 city guides; `tools/import_quests.py [--apply\|--undo]` resolves every name (NPCs and creatures as the database spells them, items as a player reads them, a spell the quest's class's own, factions, a visual by DictID or a spell's), makes a file's quest items from their `like` item, writes a quest with anything missing (or needing a draft) as a draft with the misses in its notes, keeps ids by key from 100001 in `quest_import_quests` (a run again rewrites only what a GM has not changed since), and `--undo` is exact; stop the server to apply
- Factions (`entity/faction.go`, F3 20261025-044451): every faction's starting standing is table `factions` (filled from `database.DefaultFactions`, the eqoa.live capture's 331, when empty), a character's own are `charfactions` (kept as `charbuffs` are), and killing an NPC changes the KILLER's by the `factionhits` rows for its name (melee, spell or a GM's `#kill`; a `#spawn` copy counts as its original), each change a line in EverQuest's words unless 0x00C9 says Faction Messages off; -2000..2000; the window (0x0AC4 → 0x07E2) lists the character's own in the capture's order. **The hit numbers are ours, not measured.** `tools/seed_faction_hits.py` writes the town rows (source `town-seed`: a city's townsfolk -25 on its Citizens, its guards -50 on its guard factions; the cities off EQOA-Data's map, `--undo` deletes them); it needs the `factions` table, so run it after the server's first start with it and restart after. `#faction <faction> [standing]` shows or sets a GM's own

### Key Server Protocol Details

**A shutdown tells every client first** (`udp.Listener.NoticeShutdown`, before `SaveAll`; F3 20261026-063102: a restart left each game in a world that no longer answered, a pet running on along its last velocity): 0x07C0, CLIENT's Disconnect(17), "The server has been shutdown by a server administrator." (ConnectStatusString's word 1024; reason 16's 718 "We are updating the game" is the wrong-version login's), flushed and once more past the retransmit delay, so a restart costs about 0.7 s more and every game goes back to its login with the notice.

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

**Others' power for a client's bars** (2026-10-25; `entity/powerbars.go`,
`handlers/powerbars.go`, `claude-notes/mana-bars-20261025.md`). The game shows no one's
power but the player's own, so at the first 0x0014, beside 0x7E00's, the server offers
opcode **0x7E01** (`OpcodePowerBars`, a u32 1; not retail, ignored by a PS2). A client that
draws power bars (eqoa-rust-client-vanilla's Mana Bars) answers 1 (0 to stop), and from
then on `powerSyncSystem` (every 5 ticks, after `groupSyncSystem`) sends **0x7E02**
(`OpcodePowerBarsData`) whenever its set changed: a u8 count, then each a u32 id as that
client knows the player and a u8 power of 255, the whole set each time: each other group
member (the roster's id, `groupFarID`'s stand-in out of sight), the target and the target's
target (the ids the entity channel's bytes 59-62 carry). Players only: an NPC has no power.
A session that never asks (every PS2, a vanilla build from before it) is sent nothing.

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
- **A client with no ticket signs in by its token's password** (`auth.PasswordFromUDPToken`,
  `tcp.AuthServer.CheckUDPPassword`; F3 pass 17: after a restart, or more than `auth.ticket_ttl`
  after its Station login, a client back at the server list after a lost connection was refused
  and told nothing, and waited on "Connecting..." for good). Authenticate and Authenticate2 end
  with a 32-byte token, the password zero-filled and AES-256-ECB encrypted under a key STATION
  carries (`claude-notes/auth-server-protocol.md`), so the password is checked as a login is:
  the guessing limit, the stored hash, a ban; never an account made; one login at a time for
  the address (the account's own session is the player coming back, and its world entry
  replaces it). A refusal is sent as CLIENT shows one: 0x0729 "Bad Login/Password" (its server
  list then offers STATION's login), 0x00EC for another account playing from the address.
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
