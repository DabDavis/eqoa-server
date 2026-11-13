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
- `pvp_*`: `entity/duel.go` `duelHurtLocked`, after the duel's or PvP's share
  and a flag carrier's bonus (`raw` is the figure before the share).
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
- Random draws, from the server's own random source:
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
| `a_kind` | the attacker: `"player"`, `"creature"`, `"pet"` or `"merc"` |
| `a_name` | its name (a merc's its NPC's) |
| `a_family` | a creature's family key: the class template it fights by (`""` for none and for a player) |
| `a_race` | a player's race's name (`"Human"` ...), a creature's race as its row has it |
| `a_class` | its class id (WAR=0 ... ALC=14) |
| `a_class_name` | its class's name (`"Warrior"` ...) |
| `a_level` | its level |
| `a_offmod`, `a_defmod` | its OFF and DEF mods |
| `a_eff_level` | its combat level: `a_level + a_offmod` |
| `a_hp`, `a_maxhp`, `a_power`, `a_maxpower` | its hit points and power now, and their maxima |
| `a_str`, `a_sta`, `a_agi`, `a_dex`, `a_wis`, `a_int`, `a_cha`, `a_ac` | its stats as the client shows them |
| `weapon_dmg` | the attacker's weapon's damage (a monk's bare hands' too; 0 for a creature) |
| `weapon_cond` | its condition, 0 to 1 |
| `weapon_delay` | its swing's delay in seconds |
| `two_handed` | 1 for a two-handed weapon |
| `d_kind` | the defender: `"player"`, `"creature"` or `"pet"` |
| `d_name`, `d_family`, `d_race`, `d_class`, `d_class_name`, `d_level` | as the attacker's |
| `d_offmod`, `d_defmod` | its OFF and DEF mods |
| `d_eff_level` | its combat level: `d_level + d_defmod` |
| `d_hp`, `d_maxhp`, `d_power`, `d_maxpower` | as the attacker's |
| `d_defence` | a creature's defence figure in the built-in melee roll (0 for a player) |
| `d_dummy` | 0, or the kind of training dummy |
| `d_str`, `d_sta`, `d_agi`, `d_dex`, `d_wis`, `d_int`, `d_cha`, `d_ac` | its stats |
| `d_fr`, `d_cr`, `d_pr`, `d_dr`, `d_lr`, `d_ar` | its resists |
| `owner_level`, `owner_class`, `owner_class_name` | a pet's or a merc's owner's (0, -1, `""` for none) |

The built-in has no critical hits; a formula that wants them draws its own.

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
