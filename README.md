# eqoa-server

A server for **EverQuest Online Adventures: Frontiers**, in Go. The retail PS2 disc (booted
through EQOABOOT, unmodified) and the native [eqoa-client](https://github.com/DabDavis/eqoa-client-v1)
both log in, create characters and play on it.

Its wire formats are checked against the PS2 client itself: the
[matching decompilation](https://github.com/DabDavis/eqoa-decomp) of the game's network
library and client module, and packet captures of the retail client.

A fan project, not affiliated with Sony Online Entertainment or Daybreak. No game data is
included: no disc files, no world files, no databases.

## What it runs

One binary, `cmd/gameserver`:

| Port | Protocol | What |
|---|---|---|
| 9735 | TCP | Auth: the login request, DES-encrypted password, bcrypt-checked accounts |
| 10070 | UDP | The game: DRDP (reliable messages, 24 state channels, XOR deltas), world entry, movement, combat, NPCs, equipment, chat, guilds, factions, class masteries, training, the who list |
| 7000 | HTTP | The patch server: the VerantPatcher manifest the PS2 downloads its patches from |
| 8080 | HTTP | Optional account page |
| (gm.port) | HTTP | Optional GM panel, behind a TLS proxy: a web page and MCP/OAuth/OpenAPI for AI agents |

Beside the game itself it serves the PS2 hard-disk setup (an in-game technician, the disc
mirror and world delivery over UDP), which needs payloads built by separate projects.

## Building

Go 1.24. The server reads the game's byte streams with
[eqoa-esf-tools](https://github.com/DabDavis/eqoa-esf-tools)' `pkg/buffer`, through a `replace`
to a sibling checkout, so clone both side by side:

```bash
git clone https://github.com/DabDavis/eqoa-server
git clone -b client-v1 https://github.com/DabDavis/eqoa-esf-tools go-eqoa-esf-tools
cd eqoa-server
go build -o gameserver ./cmd/gameserver
go test ./...
```

## Running

```bash
cp config.yaml.example config.yaml   # set server_ip: the address the CLIENT can reach
./gameserver
```

`server_ip` goes into the server list and the patch manifest, so it must be reachable from
the PS2 or PCSX2 host; it is never `localhost`. The comments in `config.yaml.example`
explain every setting, including the test accounts created at startup.

The database schema is created on first start. **The game content is not**: NPC spawns,
items, spells and default characters come from a seed database that is not part of this
repository. `cmd/importdump`, `cmd/extractdb`, `cmd/sqlimport`,
`tools/import_ff_npcs.py`, `tools/import_prh_content.py` (Return Home's static tables) and
`tools/import_sandstorm_harvest.py` (what eqoa-rs's `eqoa harvest` collected off a server:
its spell vendors' catalogs, their items, the ability records) and
`tools/import_spell_effects.py` (what each spell does, from the EQOA-Data project's spell
lists, and the visuals recorded casts named) build one from data you have;
`sql/` holds a few NPC set-ups.

## Layout

```
cmd/gameserver/      the server (game + auth + patch + account)
cmd/…                importers and wire-format inspection tools
internal/network/    DRDP over UDP, the TCP listeners, the patch server
internal/session/    sessions, state channels, XOR deltas
internal/handlers/   opcode handlers and senders
internal/entity/     players, NPCs, the world and its tick systems
internal/database/   SQLite persistence
internal/auth/       account and login checks
internal/gmweb/      the GM panel (web page, MCP + OAuth + OpenAPI for AI agents)
```

## The GM panel

Set `gm.port` (and `gm.public_url`) to give accounts with `acctlevel` 1 or more a web page that
edits every table, with the running world kept in step (an NPC edited is changed where it
stands), every change audited and revertible. An AI agent (ChatGPT, Claude, Codex, any MCP
client) can sign in as a GM through OAuth at `<public_url>/mcp`, or use a GM's token with the
OpenAPI description at `<public_url>/api/openapi.json`. The map and the 3D models are drawn in
the browser from the GM's own disc image; the server holds no game data.

Put it on a host name of its own (`gm.example.org`) behind TLS: then no other page shares its
origin, and its session is a `__Host-` cookie. The admin's tools (raw SQL, settings, backups, a
restart, account levels) need an admin who signed in with an authenticator code, enrolled on
the panel's Security page; `gm.require_totp` asks every GM for one.

`CLAUDE.md` is the working notes: the design, the protocol details and why each odd-looking
rule is there.

## License

MIT (`LICENSE`), for this repository's own work: the server, its tools and its notes. It
grants no rights in *EverQuest Online Adventures* itself; the game, its binaries and its
data belong to their owners, and none of them are distributed here.
