# Damage formulas from a file the operator keeps

The server can take the damage of each kind of combat hit from formulas in a
file on the server's own machine, outside this repository. The repository
carries only the support: the language, the file format, the hooks in every
damage path, and the reload. With no file configured, every figure is the
built-in one (`internal/entity/dmgformula.go`, `internal/formula`).

## Turning it on

```yaml
# config.yaml
combat:
  formula_file: /home/sdg/eqoa-server/private/damage.formula
```

- Leave the key out, or set it to `""`, and the server behaves as it always has.
- Keep the file readable by the server's user alone (`chmod 600`). The
  repository's `.gitignore` refuses `/private/` and `*.formula`, so a copy
  dropped in a checkout is not committed by accident.
- The server reads the file at start. **It reloads by itself within a second
  of the file changing** (its modification time or size), and the GM panel's
  `reload_formulas` operation (admin only) reloads it at once and answers with
  the names now in use.
- A load logs `Damage formulas loaded: N (names), K table(s)`, the formulas'
  names only.

## The mode: off, live, shadow

```yaml
combat:
  formula_file: /home/sdg/eqoa-server/private/damage.formula
  formula_mode: shadow            # "" or off | live | shadow
  formula_shadow_log: /home/sdg/eqoa-server/private/formula-shadow.jsonl
  formula_shadow_sample: 1        # the share of hits recorded (unset: all)
  formula_shadow_max_mb: 64       # rotated to <file>.1 at this size
  formula_shard: heirloom         # this shard's name in /formula
  formula_peers:                  # other shards' databases, switchable from here
    live: /home/sdg/eqoa-live/eqoa.db
```

- **off** (`""`, `off`): the file is read (so `reload_formulas` names what it
  holds) and nothing of it lands: the built-in damage everywhere.
- **live**: a formula's result replaces the built-in figure of its path.
- **shadow**: on every hit of a path a formula covers, the formula runs as live
  would run it and its figure is **not** applied: the built-in figure lands,
  unchanged. The pair is written, one JSON line a hit, to
  `formula_shadow_log`, created mode 600 in a directory of mode 700, outside
  the repository and outside the server's log. The tick does no I/O: a record
  is handed to a queue (8,192; a full queue drops the record and counts it) and
  a goroutine of its own writes it, buffered, flushed every second, rotating
  by size. Every path a formula covers is shadowed: a file defining
  `npc_melee` alone shadows creatures' melee on players alone.

A line holds:

- `t`; `path` (the hit's); `formula` (the one that ran); `file_hash` (the
  formula file's SHA-256, its first 16 hex digits: which load ran) and
  `file_version` (the file's own `version:`, left out when it has none);
  `sample` (the share of hits recorded) and `since` (when this sampling began,
  the shadow log being set: the window is `since` to `t`).
- The figures (the comparison stages, below): `builtin` (what landed),
  `value` (the formula's figure), `builtin_pre`, `formula_final_preview`,
  `formula_at`, `builtin_before_effects`, `builtin_after_effects`,
  `formula_before_effects`, `formula_after_effects`, `stage`, `raw`,
  `mitigated` (what a mitigation took off the built-in figure: a creature's
  melee, a PvP melee hit).
- `draws` (the formula's random draws); `fail` (a run's failure, part and
  kind) and `fallback` (the failure's kind alone: `negative`, `not a number
  (NaN)`, `infinite`, a missing field ...; the built-in figure then stood for
  the formula's, so `value` is the built-in's at the formula's place).
- `attack` (the attack's kind: `melee`, `ranged` for a bow's shot, `spell`,
  `dot`, `shield`, `proc`, `special`), `arrow` (which arrow of a shot this run
  is, 1 or 2; left out for a run on the whole shot or no shot) and `arrows`
  (the shot's arrows); `weapon_type` (the input of that name: the attack's
  weapon's type, -1 unknown) and `weapon_mismatch` (a ranged weapon's type on
  a hit that is no shot); `d_resist` (the defender's resist the hit is
  against).
- `in`: the inputs the formula reads, and always the sides' kinds, names,
  families, races, classes, levels, mods (the level-based ones, the effects',
  the masteries' and their ranks), npc rows, the defender's AC and AGI and
  `d_dummy`, the attacker's STR and DEX, the weapon's damage, delay, hands,
  type, kind and the defender's resist of that kind, the ranged slot's damage,
  the shot's arrows and the arrow, the flags, `mode`, the spell, the resist,
  `base_pre`, `world` and `instance`.
- `tags`: the value of every step whose name begins with `shadow_` (a number
  or a string), the file's own way to record, say, which of its records a hit
  matched.

Never a formula's text.

### The comparison stages

Every path's figure goes, in order: the roll (`raw`), the balance's shares
(and an add's share of its creature), then, on two paths, a **stage** of
steps (the hit's *effects*), with the formula standing before the stage on
one path and after it on the other:

| Path | Its stage, in order | The formula stands |
|------|---------------------|--------------------|
| `npc_melee` (`systems.go` `npcAttackPlayer`) | the player's mitigation (`workbook.go` `mitigateSnapLocked`: the strongest mitigation with any left takes its percent off, a capped one no more than it has left, spending that much and breaking when spent; an invulnerability is a mitigation of 100%) | **before** the stage (`formula_at: before_effects`) |
| `pvp_melee` (`duel.go` `duelHurtAtLocked`) | the duel's or PvP's share comes first (with the balance's), then the defender's mitigation, as above; then, in a PvP fight (not a duel), a wounded flag carrier's added share (`ctfflag.go`) | **after** the stage (`after_effects`) |
| other `pvp_*` | the share, then in a PvP fight the flag carrier's added share | after |
| every other path | none | after (at the end) |

- **`npc_melee`, live**: the formula runs on the figure before the
  mitigation (`base` = `base_pre` = the balance-shared figure), its result
  takes that figure's place, and then the mitigation's whole lifecycle runs
  on it, once, as it runs on the built-in figure: its share off, a capped
  pool spent by what it took, the mitigation broken when spent. What is left
  lands. (Until 2026-11-15 the formula ran after the mitigation on the
  landed figure; a file meant for that keeps its figure only where no
  mitigation is up.)
- **Every other path, live**: a formula's result replaces the figure at the
  end, after the stage: the stage is not run on it again. Its `base` is the
  figure after the stage, `base_pre` the figure before it.
- **Shadow**, every path: the built-in lands, and spends what it spends,
  unchanged. The formula runs where it would run live, with the same `base`.

Each shadow line holds the comparable figures:

- `builtin_before_effects` (also `builtin_pre`): the built-in figure before
  the stage, after the balance's shares (and a duel's share); the landed
  figure on a path with no stage. For `npc_melee` it is `builtin + mitigated`.
- `builtin_after_effects` (also `builtin`): the built-in figure after the
  stage: what landed.
- `formula_before_effects` (`npc_melee` alone; left out where the formula
  stands after the effects): the formula's figure, `value`.
- `formula_after_effects` (also `formula_final_preview`): the formula's figure
  as it would land. For `npc_melee` it is `value` put through the mitigation
  **as it stood before this hit** (its percent, and what a capped one had
  left before the built-in's hit spent any): a preview with no side effect,
  spending no pool, breaking no mitigation, logging nothing, setting off no
  proc and drawing nothing. Elsewhere it is `value`.
- `stage`: the stage's steps: `mitigation`, `ctf`, `mitigation+ctf`, `none`
  (a stage that met no mitigation and no wounds), left out on a path with no
  stage.

Compare `formula_after_effects` with `builtin_after_effects` (the summary
does), and `formula_before_effects` with `builtin_before_effects`.

A later step put after the duel's share and the mitigation, before the
formula's hook (say a flat share off every incoming hit), would on
`pvp_melee` be inside the stage: part of `builtin_after_effects`, and a
`pvp_*` formula's result would replace a figure that already had it. On
`npc_melee` it would sit after `formulaBegin` if put after the mitigation:
it would then need its own pure preview in `dmgStage.preview` (and its
capture in `formulaFinish`) for `formula_after_effects` to include it; until
then the preview stops at the mitigation (and a carrier's wounds).

### The draws in shadow mode

A shadowed formula draws from a random source of its own, seeded at start
and apart from the game's (math/rand's global source, which the melee rolls,
procs and every other gameplay roll draw from), so switching shadow mode on
changes no gameplay roll's sequence: with the game's seed fixed the game's
draws come out the same with shadow on or off, with formulas that fail (and
fall back) and with shadow runs from several goroutines at once (tests hold
it, under `-race` too). The sampling of `formula_shadow_sample` has its own
source too. `entity.SeedShadowRand(seed)` fixes the shadow source's seed (a
test's, or two shadow runs to compare alike). **Live mode draws from the
game's source**, as before: there the formula's figure lands and is part of
the game, as the built-in roll it replaces is.

The mode is kept in each shard's database (table `formula_setting`):
`formula_mode` only seeds it, and a GM's switch overwrites it and outlives a
restart.

- In the game, `/formula` (or `#formula`) shows each shard's mode to any GM;
  `/formula jibs <shard>` (live), `/formula default <shard>` (off) and
  `/formula shadow <shard>` switch it, an admin's (`gm.admin_level`). Left
  out, the shard is this one. A shard with no formula loaded refuses `jibs`
  and `shadow` ("no formula loaded"), one with no shadow log `shadow`.
- On the GM panel (Settings, Damage formula): `formula_modes` (any GM),
  `set_formula_mode` (an admin).
- A switch of this shard takes effect at once; of another shard
  (`formula_peers`), its database's row is written with a line of its
  `formula_mode_inbox`, and its server takes it up within 3 seconds and tells
  its GMs. Each server writes beside its row how many formulas it has loaded
  and whether it has a shadow log, which is what the other shard's GM is
  refused by. A shard whose database is not named in `formula_peers` is
  refused with a line saying so.
- Every switch has its `gm_audit` line in the database of the shard switched
  (and a line in the switching shard's own), and a `[Formula]` line on the GM
  channel of both.

`formula_shadow_summary` (the panel, an admin's) reads the file and the one
rotated before it: hits, how many the formula modelled (it ran and drew, or
gave a figure other than the built-in), failures, the mean, median and 90th
percentile of the built-in and the formula's figures and their ratio,
overall, by path, and by path / attacker kind / attacker level band (1-10 ...
61+), and the largest differences. Each group counts its `fallbacks` (lines
whose built-in figure stood for the formula's: a failed run, or a shot left
on the built-in), and `fallback_reasons` counts them by path and reason
(`player_melee / ranged_builtin`, `npc_melee / negative`). The figures compared are each line's as
they would land (`builtin_after_effects`, `formula_after_effects`; a line
written before those, `builtin` and `value`). With `tag` (a `shadow_` step's
name, with or without the prefix) it also groups the hits by path and that
step's value (`by_tag`: `npc_melee / band=high`; a hit without it under
`band=(none)`), so the populations a formula tells apart are summed apart.
Figures only: no formula's text, and no tag but the one asked for, whose
values then show.

The per-character figures the inputs read (the bow's damage and type, the
masteries' OFF/DEF points and rank counts) are never kept through a
session-keeping restart: a resumed character is built from its rows as a
login builds it (`handlers/keep.go` `ResumeCharacter` through
`enterFromRecord`: its masteries, then its worn items), so a keep state
written before they existed resumes with them filled (a test holds both
paths).

## What is safe about it

- **The text never leaves the file.** No formula, step, table key or table
  value is logged, sent to a client, written to the GM audit, shown in the
  panel or put in an error. A mistake is reported by line and column, the
  formula's name and its part (`step 2`, `result`, `min`, `max`), and the kind
  of mistake ("unknown name", "expected \")\"", "a field the record lacks").
- **A file with any mistake is refused whole**, and the formulas already in use
  stay (at start: the built-in figures). A file that is removed or cannot be
  read leaves the built-in figures.
- **A formula that fails at a hit falls back to the built-in figure for that
  hit**: a result that is not a number, NaN, infinite or negative, a field a
  record lacks, a value of the wrong kind, a draw that failed. Such a failure
  is logged at most once a minute per formula, with how many were kept quiet.
- **It cannot hang or touch anything.** The language has no loops, recursion,
  assignment, I/O or calls outside itself. An expression is at most 8,192
  bytes, 64 deep and 2,000 parts; a formula has at most 64 steps; the file is
  at most 4 MB with at most 500,000 table values. The one sampler with a
  rejection loop (the Gamma draws inside `beta`) gives up after 64 tries.
- A result is rounded to a whole number and held at most 1,000,000,000.

## Cost

Each formula is compiled once, at load. With no file, a hit costs one atomic
load more than before. With a formula, a hit reads only the inputs that
formula uses. Measured on the development machine (an i7-3770, shared with
other work, so figures are rough): a few arithmetic operations about 0.4 µs; a
formula with two table lookups through a fallback chain, a weighted pick over
a record's slots and a `beta` draw about 1.5 µs; no allocation per hit.
`go test ./internal/formula -bench .` and
`go test ./internal/entity -run XXX -bench DamageFormula` measure it again.

## The formulas: one for each damage path

A formula's name is `<source>_<kind>`. Each is optional. A source's name alone
(`player`, `npc`, ...) covers every kind of that source that has no formula of
its own.

| Source | Who hits whom |
|--------|---------------|
| `player` | a player on a creature |
| `pvp` | a player, or its pet, on a player: a duel or two flagged players |
| `npc` | a creature on a player |
| `pet` | a player's pet on a creature |
| `merc` | a player's mercenary on a creature |

| Kind | What made the hit |
|------|-------------------|
| `melee` | an auto-attack swing (a bow's shot too: `ranged` is 1) |
| `spell` | a spell's direct damage |
| `dot` | a damage over time's tick |
| `shield` | a damage shield answering a melee hit |
| `proc` | a proc (a weapon's, a worn item's, an effect's, a mark's) |
| `special` | a creature template's special or kick (`npc_special` only) |

So the names are `player_melee`, `player_spell`, `player_dot`,
`player_shield`, `player_proc`, `pvp_melee`, `pvp_spell`, `pvp_dot`,
`pvp_shield`, `pvp_proc`, `npc_melee`, `npc_spell`, `npc_dot`, `npc_shield`,
`npc_proc`, `npc_special`, `pet_melee`, `pet_spell`, `pet_dot`, `pet_shield`,
`pet_proc`, `merc_melee`, `merc_spell`, `merc_dot`, `merc_shield`,
`merc_proc`, and the sources `player`, `pvp`, `npc`, `pet`, `merc`. The kind
of a hit is the one its damage number is tagged with for the client.

Where each takes effect:

- `player_melee`: `entity/attack.go` `playerHitLocked`
  (`playerShotFormulaLocked`), after the balance's shares
  (`meleeDealtLocked`). A bow's shot (the branch `playerShotParts` takes: a
  ranged swing, no thrown potion) runs it once on the whole shot, as it
  always has; a file that says `options: ranged: arrow` has it run **once
  for each arrow**: the arrows are rolled as before (each its ranged roll at its share), the
  balance's share is taken once on their sum as before, and arrow i's run
  gets that figure's share by its roll (`base`: the shares sum to the
  built-in figure, the last arrow taking the rounding) and its own roll
  (`raw`), with `arrow` = i and `projectiles` = the count. The runs' figures
  are summed into the swing's one hit: one figure to the client, one threat,
  the swing's procs, its power, a kill, as before. The formula set and the
  mode are read once for the swing: a reload or a switch between its arrows
  does not reach it (every arrow runs on, and is logged with, the same
  file).
- `player_spell`, `player_dot`, `player_proc`, `player_shield`:
  `entity/cast.go` `spellHitHateLocked`, the one place a player's spell, tick,
  proc, damage mark and damage shield land on a creature, after `dealtLocked`.
- `pvp_*`: `entity/duel.go` `duelHurtLocked`, after the duel's or PvP's share,
  a melee hit's mitigation and a flag carrier's bonus (`raw` is the figure
  before the share).
- `npc_melee`: `entity/systems.go` `npcAttackPlayer`, after the balance and
  **before** the mitigation, which then runs on the formula's figure
  ("The comparison stages"). `npc_spell` and `npc_dot`: `entity/classtemplate.go`
  (`templateHitLocked`, `npcTickLocked`), after the resist. `npc_special`:
  `templateStrikeLocked`. `npc_shield`: `entity/creatureshield.go`.
  `npc_proc`: a template's proc (`creatureshield.go`), its ability procs and
  veils (`creatureproc.go`).
- `pet_*`: `entity/pet.go` `petLandHitLocked`, after `petDealtLocked`.
- `merc_*`: `entity/mercfight.go` `mercDamageLocked` (its figure arrives with
  the balance's shares already taken).

What a formula does not replace: a creature held at 1 hit point by its boss
moment still takes nothing (`deathhold.go`); a training dummy still takes
nothing (it is told the figure); and damage that is no combat hit of these
paths (hazards, drowning, a boss moment's scripted blast, a guard's or a
charmed creature's hit on another creature, a creature's hit on a pet or a
mercenary, a hit on a pet in a duel).

## The file

YAML, four sections, all optional:

```yaml
version: 2026-11-15a       # optional: one line, at most 64 characters; logged
                           #   at load and in each shadow line (file_version)
options:                   # optional
  ranged: swing            # swing (default) | arrow | builtin
tables:
  <table>:                 # lower-case letters, digits, _
    <key>:                 # a record, under any text key
      <field>: 1.5         # a number, a string, true/false (1/0),
      <field>: some text   #   a record, or a list
      slots:
        - {weight: 3, x: 1}
        - {weight: 1, x: 2}
formulas:
  <name>: <expression>     # a result alone
  <name>:
    steps:                 # optional, run in order
      - <step>: <expression>
      - <step>: <expression>   # may read the steps above it
    result: <expression>
    min: <expression>      # optional clamps of the result
    max: <expression>
```

An expression may be written on several lines with YAML's `|`, and `#` starts
a comment to the end of its line inside one.

`options: ranged:` is what a bow's shot takes of the formulas:

- `swing` (the default, and what a file without the option gets): once on
  the whole shot (`arrow` 0, `raw` the arrows' sum, `base` the shot's
  figure), as before 2026-11-15, so an older file lands what it did.
- `arrow`: `player_melee` runs once for each arrow (above); any other path's
  shot (`pvp_melee`, `merc_melee`) once on the whole shot.
- `builtin`: every bow's shot (a player's, PvP's, a merc's) keeps the
  built-in figure; no formula runs on it. In shadow mode each such shot still
  writes its line, with `fallback: ranged_builtin`, `attack: ranged`, the
  built-in figures (`value` the built-in's), no draws and the inputs every
  line carries, so each shot is a counted fallback in the summary.

An option the server does not know, or a word it does not take, refuses the
file (by line and the option's name). A load logs the file's hash (the first
16 hex digits of its SHA-256) and its version.

## The language

Values are numbers, strings (`"text"`), records and lists.

- Arithmetic `+ - * / %`, power `^` (right-associative; `-2^2` is `-4`).
- Comparisons `== !=` (numbers or strings) and `< <= > >=` (numbers), 1 or 0.
- `&&`, `||`, `!` (a nonzero number is true), and `c ? a : b`.
- A record's field: `rec.name` (a run error when it lacks it).
- `abs floor ceil round trunc sqrt log log10 exp sign`, `pow(a, b)`,
  `min(...)`, `max(...)`, `clamp(x, lo, hi)`, `if(c, a, b)`.
- `lookup("table", key1, key2, ...)`: the table's record under the first key
  it has: say the exact name, then a family, then `"default"`. When it has
  none, an empty record, whose `field` reads give their defaults. The table
  is named by a string written in the formula, checked at load.
- `field(rec, "name", default)`: a field, or the default when the record
  lacks it. `has(rec, "name")`: 1 or 0. `len(list)`, `at(list, i)` (from 0).
- Random draws, from the server's own random source (in shadow mode, a
  source of the shadow runs' own: "The draws in shadow mode"):
  `uniform()` in [0, 1), `uniform(lo, hi)`, `normal(mu, sigma)`,
  `beta(alpha, beta)` (X/(X+Y) of two Gamma draws by Marsaglia and Tsang's
  method), `pick(list)` (one item of a list, chosen by one uniform draw with
  the chance of its `weight` field, or `pick(list, "field")` for another;
  a number item is its own weight), and `pick_index(w0, w1, ...)` (an index
  from 0).

### Checking a formula against fixed draws

`formula.Forced{Draws: [...]}` is a random source that replays a vector: each
draw a formula makes (`uniform`, `normal`, `beta`, the draw inside `pick` and
`pick_index`) returns the next value as its result. A vector that runs out
fails the run rather than inventing draws. The entity tests set it with the
package variable `formulaRand`; a port of a formula can be checked against
vectors of (inputs, draws, result) this way.

## The inputs

Every formula may read these. A side that is a creature has no stats of its
own: its `_str` ... `_ar` are what its effects add.

| Input | Meaning |
|-------|---------|
| `base` | the built-in figure of this hit, what would land without a formula |
| `base_pre` | the built-in figure before the path's stage (a mitigation, a flag carrier's wounds: "The comparison stages"); `base` on a path with none |
| `raw` | the path's figure before the balance's shares (the weapon roll, the spell's amount after resists; `base` where the path has none) |
| `kind` | 1 melee, 2 spell, 3 dot, 4 shield, 5 proc, 6 special |
| `mode` | the attack's kind: `"melee"`, `"ranged"` (a bow's shot), `"spell"`, `"dot"`, `"shield"`, `"proc"` or `"special"` |
| `pvp` | 1 for a `pvp_*` hit |
| `duel` | 1 in a duel (0 in a fight of two flagged players) |
| `ranged` | 1 for a bow's shot |
| `by_pet` | 1 for a pet's hit in PvP |
| `spell_id` | the spell's id (0 for none) |
| `spell_level` | the spell's level (the ability's; 0 for none or unknown) |
| `resist` | the resist the hit is against: 0 none, 11 FR, 12 CR, 13 PR, 14 DR, 15 LR, 16 AR |
| `con` | the defender's level less the attacker's |
| `world`, `instance` | the world index the hit is in and the instanced world's copy (0 for none): the defender's (a player's before a creature's), else the attacker's |
| `a_kind` | the attacker: `"player"`, `"creature"`, `"pet"` or `"merc"` |
| `a_name` | its name (a merc's its NPC's) |
| `a_family` | a creature's family key: the class template it fights by (`""` for none and for a player) |
| `a_race` | a player's race's name (`"Human"` ...), a creature's race as its row has it |
| `a_class` | its class id (WAR=0 ... ALC=14) |
| `a_class_name` | its class's name (`"Warrior"` ...) |
| `a_level` | its level |
| `a_offmod`, `a_defmod` | its OFF and DEF mods: a player's from its effects; a creature's its level-based estimate (its row's level through `NPCOffMod`/`NPCDefMod`; a pet its sheet's) |
| `a_effect_offmod`, `a_effect_defmod` | its OFF and DEF mods from its effects alone: a player's (as `a_offmod`), a creature's what its effects add |
| `a_cm_offmod`, `a_cm_defmod` | the OFF MOD and DEF MOD points a player's bought masteries give: the sum over its owned ranks of each rank's `cmcatalog` effect `offmod` / `defmod` (`CMEffects` OFF MOD and DEF MOD rows, what the mastery window shows). One unit is one point of OFF (DEF) MOD as the window shows it; a rank gives what its row says, so points equal ranks only where each rank's row gives 1. Not in `a_offmod` (a player's effects') nor in the level estimates; 0 for a creature |
| `a_cm_off_ranks`, `a_cm_def_ranks` | how many of its owned ranks give any OFF MOD (DEF MOD): a count of ranks, whatever each gives; 0 for a creature |
| `a_npc_row` | a creature's `npcs` row id (a copy's, a spawn's, the row it was copied from; 0 for a copy of none and for a player) |
| `a_eff_level` | its combat level: `a_level + a_offmod` |
| `a_hp`, `a_maxhp`, `a_power`, `a_maxpower` | its hit points and power now, and their maxima |
| `a_str`, `a_sta`, `a_agi`, `a_dex`, `a_wis`, `a_int`, `a_cha`, `a_ac` | its stats as the client shows them |
| `weapon_dmg` | the attacker's weapon's damage (a monk's bare hands' too; 0 for a creature) |
| `weapon_cond` | its condition, 0 to 1 |
| `weapon_delay` | its swing's delay in seconds |
| `two_handed` | 1 for a two-handed weapon |
| `weapon_type` | the attack type of the weapon the attack's branch used: a bow's shot the ranged slot's item's (the hand's when it has none); any other hit the hand's whose hit it is, as the built-in routes the hit's resist (`handType`: the off hand's every second hit of a round, a ranged weapon's when it is the one that swings). A melee hit whose type is a ranged weapon's (7 a bow, 9 a crossbow, 10 thrown) is **-1** (unknown). 0 for no player attacker (a creature, a pet). The line's `weapon_type` too |
| `weapon_mismatch` | 1 for that melee hit with a ranged weapon's type, else 0 (the line's `weapon_mismatch`) |
| `weapon_kind` | the typed melee resist `weapon_type` routes to: `"slash"` (types 1, 2), `"pierce"` (5, 6, 7, 9, 10, and every bow's shot), `"blunt"` (the rest, and a player's bare hands); `""` for -1 and no player attacker |
| `ranged_weapon_dmg`, `ranged_weapon_type` | the item in the attacker's ranged slot's damage and attack type, 0 for none, whatever the hands hold. A bow's shot in the built-in rolls on `weapon_dmg`, which is the hands' weapon's when one is held and the ranged slot's only when none is |
| `projectiles` | a bow's shot's arrows: 1, or 2 at the fourth ranged swing; 0 for a swing that is no shot |
| `arrow` | which arrow of the shot this run is (1, 2) under `ranged: arrow`; 0 for a run on the whole shot and for no shot |
| `d_kind` | the defender: `"player"`, `"creature"` or `"pet"` |
| `d_name`, `d_family`, `d_race`, `d_class`, `d_class_name`, `d_level` | as the attacker's |
| `d_offmod`, `d_defmod` | its OFF and DEF mods (as `a_offmod`) |
| `d_effect_offmod`, `d_effect_defmod`, `d_cm_offmod`, `d_cm_defmod`, `d_cm_off_ranks`, `d_cm_def_ranks`, `d_npc_row` | as the attacker's |
| `d_kind_resist` | the defender's typed melee resist of `weapon_kind`: what its effects give it of slash, pierce or blunt (a debuff's negative), a player's and a creature's alike; 0 for no kind |
| `d_kind_debuff_pct` | a creature's strongest debuff of that kind, in percent more taken (the built-in's `npcMeleeDebuffPct`, with the Rakish line's one-handed-pierce rule on `weapon_type`); 0 for a player and no kind |
| `d_kind_taken_pct` | a creature's class template's share taken of that kind (`kindTaken`), 100 for none |
| `d_eff_level` | its combat level: `d_level + d_defmod` |
| `d_hp`, `d_maxhp`, `d_power`, `d_maxpower` | as the attacker's |
| `d_defence` | a creature's defence figure in the built-in melee roll (0 for a player) |
| `d_dummy` | 0, or the kind of training dummy |
| `d_str`, `d_sta`, `d_agi`, `d_dex`, `d_wis`, `d_int`, `d_cha`, `d_ac` | its stats |
| `d_fr`, `d_cr`, `d_pr`, `d_dr`, `d_lr`, `d_ar` | its resists |
| `owner_level`, `owner_class`, `owner_class_name` | a pet's or a merc's owner's (0, -1, `""` for none) |

The built-in has no critical hits; a formula that wants them draws its own.

### Older files and newer inputs

The inputs added on 2026-11-15 (`base_pre`, `world`, `instance`,
`a_/d_effect_offmod`, `a_/d_effect_defmod`, `a_/d_cm_offmod`,
`a_/d_cm_defmod`, `a_/d_cm_off_ranks`, `a_/d_cm_def_ranks`, `a_/d_npc_row`,
`weapon_type`, `weapon_mismatch`, `weapon_kind`, `d_kind_resist`,
`d_kind_debuff_pct`, `d_kind_taken_pct`, `ranged_weapon_dmg`,
`ranged_weapon_type`, `projectiles`, `arrow`) may be a step's name in a file: from that
step on the name is the step's (the steps before it read the input), so a
file written before them, with a step of one of those names, loads and gives
the figures it did. A step named as any older input is refused as before.

## A made-up example

Nothing here is a real formula; it only shows the parts.

```yaml
tables:
  profiles:
    Warrior: {scale: 1.2, slots: [{weight: 3, add: 0}, {weight: 1, add: 25}]}
    default: {scale: 1.0, slots: [{weight: 1, add: 0}]}
  creatures:
    Example Golem: {family: golems, extra: 10}
    golems:        {anchor: 20, extra: 5}
    default:       {anchor: 1}
formulas:
  player_melee:
    steps:
      - prof: lookup("profiles", a_class_name, "default")
      - slot: pick(prof.slots)
      - mob: lookup("creatures", d_name, d_family, "default")
      - swing: base * prof.scale * (0.9 + 0.2 * uniform())
    result: swing + slot.add + field(mob, "extra", 0)
    min: 1
    max: base * 3
  npc: base * 2
```

`internal/formula/testdata/toy.yaml` and
`internal/entity/testdata/toy-damage.yaml` are the tests' toy files.
