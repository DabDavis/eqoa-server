# eqoa-server

A server for **EverQuest Online Adventures: Frontiers**, in Go. The retail PS2 disc (booted
through EQOABOOT, unmodified), PCSX2, and the native
[eqoa-rust-client-vanilla](https://github.com/DabDavis/eqoa-rust-client-vanilla) (Linux, Windows,
macOS and a browser page) all log in, create characters and play on it.

Its wire formats are checked against the PS2 client itself: the
[matching decompilation](https://github.com/DabDavis/eqoa-decomp) of the game's network
library and client module, and packet captures of the retail client.

A fan project, not affiliated with Sony Online Entertainment or Daybreak. No disc files or
world files are included; the game content comes as a separate database (below).

## What it runs

One binary, `cmd/gameserver`:

| Port | Protocol | What |
|---|---|---|
| 9735 | TCP | Auth: the login request, DES-encrypted password, bcrypt-checked accounts |
| 10070 | UDP | The game: DRDP (reliable messages, state channels, XOR deltas), world entry, movement, combat, NPCs, quests, equipment, chat, groups, guilds, factions, masteries, tradeskills, duels |
| 7000 | HTTP | The patch server: the VerantPatcher manifest the PS2 downloads its patches from |
| 8080 | HTTP | Optional account page (and the community pages' routes) |
| 7003 | UDP | Optional: the PS2 hard-disk setup's world delivery |
| (gm.port) | HTTP | Optional GM panel, behind a TLS proxy: a web page and MCP/OAuth/OpenAPI for AI agents |

## Installing a server

### 1. Build

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

### 2. A database

The server makes its schema on first start, but **the game content** (NPC spawns and their
paths, quests, items, spells, merchants, loot, factions, masteries, recipes, the balance) is
data, not code. Two ways to get it:

- **The open database** (`eqoa-open-<date>.db.gz`, from this repository's
  [Releases](https://github.com/DabDavis/eqoa-server/releases)): everything a fresh server needs,
  with no accounts, characters or anything a player did. Unpack it as `eqoa.db` beside the
  binary:

  ```bash
  gunzip -c eqoa-open-20261108.db.gz > eqoa.db
  sha256sum eqoa.db    # against the release notes
  ```

  Every import, seed and fix this repository's tools made is already in it, so do not run their
  `--apply` against it (their logs, which let `--undo` work, are not included).

- **Build your own** from data you have: `cmd/importdump`, `cmd/extractdb`, `cmd/sqlimport`,
  `tools/import_ff_npcs.py`, `tools/import_prh_content.py` (Return Home's static tables), the
  harvest importer in `tools/` (what `eqoa harvest` collected off a server: its spell vendors'
  catalogs, their items, the ability records) and `tools/import_spell_effects.py` (what each
  spell does, from the EQOA-Data project's spell lists); `sql/` holds a few NPC set-ups.

### 3. Files from your own disc (optional, recommended)

The server never ships game files; these are made from your own *Frontiers* disc image
(SLUS-20744) and read from `worlds/` beside the binary:

```bash
go build -o groundbake ./cmd/groundbake && ./groundbake -iso frontiers.iso -out worlds/ground
go build -o modelbake ./cmd/modelbake && ./modelbake -iso frontiers.iso -out worlds/models.txt
```

- `worlds/ground/` (about 100 MB): the terrain, the walls of what stands on it, and where the
  water is, for every world. Without it creatures keep their spawn heights, walk through walls
  and into water, and players' own moves are checked for speed alone. The bake reads Tunaria's
  997 MB world file whole: run it on a machine with about 2 GB free and copy the folder over.
- `worlds/models.txt`: each body's size, so a big creature is reached from farther away.
  Without it every creature is a man's size.
- Optional, `go build -o groundmap ./cmd/groundmap && ./groundmap worlds/ground` (about 650 MB of
  files, a few seconds, no disc): each server then maps the collision read-only instead of decoding
  zones onto its heap, up to 650 MB less of it, and any servers on one machine share one copy.

### 4. Configure

```bash
cp config.yaml.example config.yaml
```

The settings that matter first:

- `server.server_ip`: **the address players' clients connect to** (it goes into the server list
  and the patch manifest), so it must be reachable from them; never `localhost`. A dotted quad
  for PS2 world downloads.
- `auth.allow_auto_register`: `true` lets an unknown account name register itself with the
  password it first logs in with; `false` admits only accounts already in the database or
  listed under `auth.accounts`.
- `auth.accounts`: accounts made at start if missing (change the example's password).
- `auth.one_login`: `false` allows several windows on an account (one character a window).

Every other setting is explained in the comments of `config.yaml.example`.

### 5. Run it

```bash
./gameserver          # from its own folder: it finds eqoa.db, patches/ and worlds/ there
```

Open the ports above in your firewall (9735/tcp and 10070/udp at least). As a service
(`/etc/systemd/system/eqoa-game.service`):

```ini
[Unit]
Description=EQOA Game Server
After=network.target

[Service]
Type=simple
User=eqoa
WorkingDirectory=/srv/eqoa-server
ExecStart=/srv/eqoa-server/gameserver
Restart=on-failure
RestartSec=5
StandardOutput=append:/srv/eqoa-server/gameserver.log
StandardError=append:/srv/eqoa-server/gameserver.log

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable --now eqoa-game
journalctl -u eqoa-game     # or tail gameserver.log
```

A stop (`systemctl stop`, a restart) saves every character in the world; set
`world.restart_notice` to warn them first.

### 6. Connecting

- **eqoa-rust-client-vanilla**: on the title screen, **Network Setup** takes the server's address;
  then the pregame runs as on a PS2 (Station's login, where an account can be made when the
  server allows it, the server list, character select). `eqoa boot <server>` starts with that
  server. A browser build of it reaches a server through `cmd/wsrelay` (WebSockets to the game's
  UDP and TCP ports).
- **A PS2 or PCSX2** plays the retail disc through EQOABOOT.ELF, a loader built outside this
  repository that patches the login address in memory. Given its template
  (`players/EQOABOOT.template.ELF` beside the server), the server writes `players/EQOABOOT.ELF`
  aimed at `server_ip` at each start. The patch server offers whatever you put in
  `patches/serve/` to the memory card.

## Running a server

### Game masters

An account with `accountinfo.acctlevel` 1 or more is a GM: the in-game `#` commands (`#goto`,
`#summon`, `#spawn`, `#template`, `#stats`, `#ticks`, ...; a client that forwards `/commands`
takes them as `/goto` too) and the GM panel. Level 2 (`gm.admin_level`) adds the admin tools.

```bash
sqlite3 eqoa.db "UPDATE accountinfo SET acctlevel = 2 WHERE username = 'yourname'"
```

The level is read at every command and request, so a change counts at once.

### The GM panel

A web page for GMs that edits every table, with the running world kept in step (an NPC edited
is changed where it stands, a quest saved is reloaded), every change audited and revertible,
plus the world map with NPC paths, quests, triggers (scripted events), petitions, the balance
tables and a spell-visual preview. The map and the 3D models are drawn in the browser from the
GM's own disc image; the server holds no game files.

1. In `config.yaml`:

   ```yaml
   gm:
     port: 8081
     bind: "127.0.0.1"
     public_url: "https://gm.example.org"
     require_totp: true
   ```

2. Put it on **a host name of its own** behind TLS (then no other page shares its origin, and
   its session is a `__Host-` cookie). nginx, with a certificate from Let's Encrypt:

   ```nginx
   server {
       listen 443 ssl;
       http2 on;
       server_name gm.example.org;
       ssl_certificate     /etc/letsencrypt/live/gm.example.org/fullchain.pem;
       ssl_certificate_key /etc/letsencrypt/live/gm.example.org/privkey.pem;
       location / {
           proxy_pass http://127.0.0.1:8081;
           proxy_http_version 1.1;
           proxy_set_header Host $host;
           proxy_set_header X-Real-IP $remote_addr;
           proxy_set_header X-Forwarded-For $remote_addr;
           proxy_set_header X-Forwarded-Proto $scheme;
           proxy_read_timeout 120s;
           client_max_body_size 4m;
       }
   }
   ```

3. Sign in with a GM account's game name and password, and enrol an authenticator app on the
   Security page: the admin tools (raw SQL, settings, backups, a restart, account levels) need a
   sign-in with a code, and `require_totp` asks it of every GM.

**AI agents** (ChatGPT, Claude, Codex, any MCP client) work as a GM: through OAuth at
`<public_url>/mcp`, or with a token made in the panel and the OpenAPI description at
`<public_url>/api/openapi.json` (a custom GPT's actions). `guide.md` is what an agent reads
first. An admin can hand an agent a sanitised copy of the database for testing (Settings:
download snapshot; accounts' secrets and personal fields taken out).

### Deploying without kicking players

A data change needs no restart (the server reads its tables as it goes; the balance at once on
a save of a Balance page). A new binary does, and a restart ends every session, so
`tools/swap_when_empty.sh` swaps a staged binary in only when nobody has been connected for two
minutes:

```cron
* * * * * EQOA_UNIT=eqoa-game EQOA_DIR=/srv/eqoa-server /srv/eqoa-server/tools/swap_when_empty.sh >> $HOME/eqoa-swap/swap.log 2>&1
```

It runs as the server's user, which needs passwordless `sudo` for `systemctl stop|start` of the
unit and to read `/proc/net/nf_conntrack` (how it sees connected players).

Stage it as `~/eqoa-swap/pending/gameserver` with its `sha256` beside it (and an optional
`before.sh` for a data step to run with the server stopped); the script's header has the
details. It backs up the binary and the database first and rolls back a binary that does not
come up.

### The work board

When several people (or their agents) work on one server, the panel's Work board page says
who is changing what: entries naming the tables and files they touch, with notes and results.
`cmd/workboard` is the same board from a shell on the server.

### Tuning

`cmd/balancesim` fights simulated fights on the server's own systems against a copy of the
database, to see what a balance table change does before it goes in. The balance tables,
their targets and the tunings so far are in `CLAUDE.md`.

## Layout

```
cmd/gameserver/      the server (game + auth + patch + account + GM panel)
cmd/groundbake/      the ground, walls and water files, from your disc
cmd/groundmap/       the collision's maps, one copy in memory however many servers (optional)
cmd/modelbake/       the bodies' sizes, from your disc
cmd/balancesim/      the fight simulator
cmd/workboard/       the work board from a shell
cmd/wsrelay/         WebSockets to the game's sockets, for a browser client
cmd/…                importers and wire-format inspection tools
internal/network/    DRDP over UDP, the TCP listeners, the patch server
internal/session/    sessions, state channels, XOR deltas
internal/handlers/   opcode handlers and senders
internal/entity/     players, NPCs, the world and its tick systems
internal/database/   SQLite persistence
internal/auth/       account and login checks
internal/gmweb/      the GM panel (web page, MCP + OAuth + OpenAPI for AI agents)
tools/               data tools (imports, seeds, fixes; each --apply/--undo, logged)
```

`CLAUDE.md` is the working notes: the design, the protocol details and why each odd-looking
rule is there.

## Sharing back

Running a server from this code, or a fork of it? Fixes and improvements sent back here help
every other server too: a pull request, or an issue saying what you changed, is welcome.
Contributions are taken under the same licence.

## License

GNU Affero General Public License, version 3 or (at your option) any later version
(`LICENSE`), for this repository's own work: the server, its tools and its notes. If you run
a changed copy for players, it has to offer them its source (the licence's section 13), and
if you hand out a changed copy, its source goes with it.

Commits up to `a4ce54a` (2026-11-07) were released under the MIT licence and stay available
under it at those commits; `NOTICE` keeps its notice for the code they hold.

The licence grants no rights in *EverQuest Online Adventures* itself; the game, its binaries and
its data belong to their owners. No disc file or game binary is distributed here; the open
database holds the names, numbers and texts of the game's content as fan projects have
recorded them, and grants no rights in those either.
