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

A line holds: `t`, `path` (the hit's), `formula` (the one that ran),
`builtin` (what landed), `value` (the formula's figure), `raw`, `mitigated`
(what a mitigation took off before the built-in figure: a creature's melee, a
PvP melee hit), `builtin_pre`, `formula_final_preview` and `stage` (the
comparison stages, below), `draws` (the formula's random draws), `fail` (a
run's failure, part and kind; `value` is then `builtin`), `weapon_type` (the
attacker's weapon's attack type), `d_resist` (the defender's resist the hit
is against), `in` (the inputs the formula reads, and always the sides' kinds,
names, families, races, classes, levels, mods (the level-based ones, the
effects' and the masteries'), npc rows, the defender's AC and AGI and
`d_dummy`, the attacker's STR and DEX, the weapon's damage, delay, hands and
type, the ranged slot's damage and the shot's arrows, the flags, the spell,
the resist, `base_pre`, `world` and `instance`), and `tags`: the value of
every step whose name begins with `shadow_` (a number or a string), the
file's own way to record, say, which of its records a hit matched. Never a
formula's text.

### The comparison stages

Every path's figure goes, in order: the roll (`raw`), the balance's shares
(and an add's share of its creature), then, on two paths, a **stage** of
steps, and only then the formula's hook. In **live mode a formula's result
replaces the figure at the hook, after the stage**: the stage is not run on
it again, and what the formula returns is what lands (before the boss-hold
and the dummy, as for every path). So in live mode a formula's `base` is the
figure after the stage, and `base_pre` the figure before it.

| Path | Its stage, in order |
|------|---------------------|
| `npc_melee` (`systems.go` `npcAttackPlayer`) | the player's mitigation (`workbook.go` `mitigateLocked`: the strongest mitigation with any left takes its percent off, a capped one no more than it has left, spending that much and breaking when spent) |
| `pvp_melee` (`duel.go` `duelHurtAtLocked`) | the defender's mitigation, as above; then, in a PvP fight (not a duel), a wounded flag carrier's added share (`ctfflag.go`) |
| other `pvp_*` | in a PvP fight, the flag carrier's added share |
| every other path | none |

Each shadow line then holds three figures of the hit:

- `builtin_pre`: the built-in figure **before the stage** (after the
  balance's shares), `builtin` on a path with none. For `npc_melee` it is
  `builtin + mitigated`.
- `builtin`: the built-in figure **after the stage**: what landed, and the
  figure a live formula's result takes the place of (`base`).
- `formula_final_preview`: the formula's figure (`value`) put **through the
  same stage**, as the built-in figure went through it, computed from the
  state as it stood before the hit: the mitigation's percent and what a
  capped one had left before this hit spent any, the carrier's wounds. It is
  a preview with no side effect: it spends no mitigation, breaks none, logs
  nothing, sets off no proc and draws nothing. On a path with no stage it is
  `value`.
- `stage`: the stage's steps: `mitigation`, `ctf`, `mitigation+ctf`, `none`
  (a stage that met no mitigation and no wounds), left out on a path with no
  stage.

So a formula written for the figure before mitigation (reading `base_pre`)
is compared against the built-in by `formula_final_preview` against
`builtin`, and one written for the landed figure (reading `base`) by `value`
against `builtin`. The figures that land are unchanged by shadow mode, and
live mode is as it was: its result is the landed figure.

### The draws in shadow mode

A shadowed formula draws from a random source of its own, seeded at start
and apart from the game's (math/rand's global source, which the melee rolls,
procs and every other gameplay roll draw from), so switching shadow mode on
changes no gameplay roll's sequence: with the game's seed fixed the game's
draws come out the same with shadow on or off (a test holds it). The
sampling of `formula_shadow_sample` has its own source too.
`entity.SeedShadowRand(seed)` fixes the shadow source's seed (a test's, or
two shadow runs to compare alike). **Live mode draws from the game's
source**, as before: there the formula's figure lands and is part of the
game, as the built-in roll it replaces is, and an older file gives the same
figures from the same seed it did.

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
61+), and the largest differences. Figures only: no formula's text, no tags.

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

- `player_melee`: `entity/attack.go` `playerHitLocked`, after the balance's
  shares (`meleeDealtLocked`).
- `player_spell`, `player_dot`, `player_proc`, `player_shield`:
  `entity/cast.go` `spellHitHateLocked`, the one place a player's spell, tick,
  proc, damage mark and damage shield land on a creature, after `dealtLocked`.
- `pvp_*`: `entity/duel.go` `duelHurtLocked`, after the duel's or PvP's share,
  a melee hit's mitigation and a flag carrier's bonus (`raw` is the figure
  before the share).
- `npc_melee`: `entity/systems.go` `npcAttackPlayer`, after the balance and a
  mitigation. `npc_spell` and `npc_dot`: `entity/classtemplate.go`
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

YAML, two sections, both optional:

```yaml
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
| `mode` | `"melee"`, `"ranged"`, `"spell"`, `"dot"`, `"shield"`, `"proc"` or `"special"` |
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
| `a_cm_offmod`, `a_cm_defmod` | the OFF MOD and DEF MOD a player's bought masteries give (the sum of its ranks' `offmod`/`defmod`, `CMEffects`); 0 for a creature |
| `a_npc_row` | a creature's `npcs` row id (a copy's, a spawn's, the row it was copied from; 0 for a copy of none and for a player) |
| `a_eff_level` | its combat level: `a_level + a_offmod` |
| `a_hp`, `a_maxhp`, `a_power`, `a_maxpower` | its hit points and power now, and their maxima |
| `a_str`, `a_sta`, `a_agi`, `a_dex`, `a_wis`, `a_int`, `a_cha`, `a_ac` | its stats as the client shows them |
| `weapon_dmg` | the attacker's weapon's damage (a monk's bare hands' too; 0 for a creature) |
| `weapon_cond` | its condition, 0 to 1 |
| `weapon_delay` | its swing's delay in seconds |
| `two_handed` | 1 for a two-handed weapon |
| `weapon_type` | the attacker's weapon's attack type (the one `weapon_dmg` is; the line's `weapon_type`), 0 for none and for a creature |
| `ranged_weapon_dmg`, `ranged_weapon_type` | the item in the attacker's ranged slot's damage and attack type, 0 for none, whatever the hands hold. A bow's shot in the built-in rolls on `weapon_dmg`, which is the hands' weapon's when one is held and the ranged slot's only when none is |
| `projectiles` | a bow's shot's arrows: 1, or 2 at the fourth ranged swing (each rolled and taken at its share, summed in `raw`); 0 for a swing that is no shot |
| `d_kind` | the defender: `"player"`, `"creature"` or `"pet"` |
| `d_name`, `d_family`, `d_race`, `d_class`, `d_class_name`, `d_level` | as the attacker's |
| `d_offmod`, `d_defmod` | its OFF and DEF mods (as `a_offmod`) |
| `d_effect_offmod`, `d_effect_defmod`, `d_cm_offmod`, `d_cm_defmod`, `d_npc_row` | as the attacker's |
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
`a_/d_cm_defmod`, `a_/d_npc_row`, `weapon_type`, `ranged_weapon_dmg`,
`ranged_weapon_type`, `projectiles`) may be a step's name in a file: from that
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
