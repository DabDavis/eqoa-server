# The homelands' starter quests: one spec a race

The owner, 2026-11-10: "lets polish the new playable races starting areas and quests. the first quests should be a
piece of equipment(not wep), 2nd quest spiritmaster and coachman. 3rd kill a mob, etc. follow the same template as
the other main race quests." And of travel: "the coach will connect to freeport and the other homeland areas."

Every starting city's class chain opens the same way (`tools/quests/data/<city>.json`). A race's homeland (a zone of
the Homelands, world 10: `tools/worlds/homelands.json`) now opens that way too, once for all fifteen classes, and
its gatekeeper's flight to the mainland comes after:

| quest | level | experience | what it is | what it gives |
|-------|-------|-----------|------------|---------------|
| `L1` | 1 | 430 | the giver sends the newcomer to a merchant of the home for the race's own piece of equipment; it is shown to the giver | the piece (kept) |
| `L2` | 2 | 2,200 | the spiritmaster binds; the coachman signs his ledger; back to the giver | |
| `L3` | 3 | 6,900 | 2-3 drops off a creature of the home (one kill in two drops one), handed in | the class's level 3 quest ability (Kick, Sneak, Minor Blessing ...) |
| `L4` | 4 | 17,000 | "new gear": materials one or two merchants hand over, and a creature's drop, handed in | the class's level 4 weapon (a robe or gloves for the cloth classes) |
| `L5` | 5 | 36,500 | a named foe's drop (it always drops), or a count of a tougher kind | the class's level 5 quest ability (Taunt, Shadowstrike, Holy Shock ...) |
| `flight` | after all five | none | the gatekeeper sends the character to its class's city on the mainland and binds it there: the passage the five quests earn (word it so; it is not the only road, a coach runs between the home and Freeport) | |

The owner, 2026-11-11: "let's add up to lvl 20 path quests for them all. you can add a new zone for the higher level
mobs/quest objectives and that zone can be shared by all the homeland races, so players can group together." Past the
flight the chain goes on at the cities' levels, given by the home's own people and done on **the Homelands' shared
ground, the Goldfeather Eyrie** (world 10's 22nd zone: below), which every home's coach (and Freeport's) runs to:

| quest | level | experience | what it is (the cities' shape) | what it gives |
|-------|-------|-----------|------------|---------------|
| `L7` | 7 | 157,474 | two tasks of the hunt (Halas's Three Tasks): one or two kinds' drops, handed in together | the class's level 7 ability (Furious Defense, Acrobatics ...) |
| `L10` | 10 | 556,753 | a new weapon: what a camp's quartermaster hands over (0-2) and a creature's drop | the class's level 10 weapon |
| `L13` | 13 | 550,698 | a named foe brought down (a kill), or its drop brought back | the class's level 13 ability |
| `L15` | 15 | 883,791 | a named foe's spoil (it always drops), carried to whoever wants it, then back | the class's level 15 ability (with the ranger's tunic) |
| `L20` | 20 | 2,814,929 | the road: 1-3 stops, a hunt whose drops someone on it wants, 0-2 stops more, the last word at the giver | **the path pick**: "Which do you take?", the options the class's own city asks (Bellow or Pillar of Might ...) |

Each finishes the character's own city class quest of its level too (`classquest`), as L1-L5 do, so the guildmaster on
the mainland goes on from there and the level 30 path quests (which ask a city's level 20) chain. Both ways: a homeland
quest **stands finished** when the city class quest it stands for is (a character that did Halas's level 7 after its
flight is not offered the home's, nor paid twice, and the home's level 10 is offered); a class its race's cities train
none of (an Aviak druid) finishes the class's quests of the level in every city (`handlers/questevents.go`). `L20` is one
quest a class (`homelands/<race>/L20/<class>`, `classes: [c]`), each with its class's options (`class-rewards.json`'s
`L20.choices`: the same in every city of the class; the cleric and the paladin have a third, a spell). Each level is
optional in a spec, but none without the one before it.

`tools/quests/gen_homelands.py` writes `tools/quests/data/homelands.json` from the specs here. **The template is the
generator's**: keys, levels, the chain, the experience, the steps' kinds, drop chances, rewards by class
(`class-rewards.json`, one table for every home), the flight's teleport and bind, whose the quests are (the home's
own race alone, by its `racedefs` id: a coach brings anyone into any home) and, on each of L1-L5, the `classquest`
event: finishing a homeland's quest of a level finishes the character's own city class quest of that level too
(silently, no reward: `handlers/questevents.go`), so its guildmaster on the mainland goes on at level 7 and does not
offer the tunic again. **A spec holds only what is the home's own**: names, words, items, creatures.

A race with no spec keeps its legacy three quests (`legacy.json`); a spec, once there, is taken instead, so adding a
home touches no shared quest file. `aviak.json` is the worked example: copy it.

## Adding a home, in short

1. Stand its people and creatures in its home of `tools/worlds/homelands.json` ("The home's people", below).
2. Write `tools/quests/homelands/<race>.json` (the race keys are `gen_homelands.py`'s `RACES`).
3. `python3 tools/quests/gen_homelands.py` (it refuses a spec it cannot build, and says why).
4. `python3 tools/quests/gen_homelands_test.py` and `python3 tools/seed_race_homes_test.py`: the template held, every
   name of the spec standing in the home in its role.
5. On a COPY of a database: `tools/seed_race_homes.py --undo` (when the homes are seeded in it), `--apply`, then
   `tools/quests/check_quests.py --db copy tools/quests/data/homelands.json` (0 errors), `tools/import_quests.py --db
   copy --apply tools/quests/data/homelands.json`, and `EQOA_REAL_DB=copy python3 tools/quests/gen_homelands_test.py`.
6. Play it (a private server on the copy, the real client), and go down the checklist below with shots.

`tools/quests/data/homelands.json` is generated: never edit it (nor run `tools/quests/fix_names.py --write` on it:
respell a name in the spec). When two branches both wrote it, take either side and run the generator again.

## The spec

Texts are a string or a list of lines. **A line of an NPC's is a box**; the client shows 217 characters with the
speaker's name and `": "` before it (so "Huntmaster Arrok: " leaves 199), a journal line 127, an answer 63. An offer
is at least two lines, the last one the question the two answers answer (127 characters, ending in `?`): the lines
before it are said in boxes, then the question is asked. `%p` is the player's name. The generator refuses what the
client would cut.

```
race, home, city      the key ("aviak"), the home's zone and the city the flight goes to (for the notes)
note                  optional: who is who, for the next reader

L1  name, giver, offer, accept, decline, declined, started      (every quest has these)
    merchant          who hands the piece over (a merchant of the home: give it a `stock`)
    piece             {name, description, like, set}: the gift. `like` is a record of the SLOT and look you want (never
                      a weapon); `set` its figures as the cities' level 1 gifts': AC up to 5, or HPMAX or POWMAX up
                      to 5 (4 armour is the cities' usual; 5 hit points or power for a trinket). The generator makes
                      it every class's, every race's, level 1, NO TRADE, free. All-class bases: Tattered Cap / Tunic /
                      Bracers / Pants / Boots / Gloves, Dun Cloth Belt, glass trinket (neck), Chipped Ring, Carved
                      Bracelet, Shell Earring. Prefer a slot that needs no art on the race's body (bracer, neck, ring,
                      belt, earring, bracelet) unless the body shows it.
    fetch             {journal, says}: the talk at the merchant (its words as it hands the piece over)
    show              {journal, says}: the piece shown to the giver (worn or carried: both count)

L2  spiritmaster, coachman    "Spiritmaster ..." and "Coachman ..." / "Coachwoman ..." of the home (the server knows
                      both by those words)
    bind              {journal, says}: the spiritmaster. The server asks "bind me here?" first; Yes does the step
    ledger            {journal, says}: the coachman. Talking signs his ledger (the server says "I'll add you to my
                      ledger now."); his words say where his coach runs: Freeport and the other homes
    back              {journal, says}: the giver

L3  target            the creature (a name of its own: see below), count 2 or 3
    drop              {name, description, like}: made NO TRADE; `like` a junk record whose icon fits (Rat Tail Tip,
                      cracked snake fang, ruined bat wing, small spider eye ...)
    hunt              {journal}
    give              {journal, says}: the giver, as it teaches the class's ability ("Now learn this.")

L4  materials         one or two {merchant, items, journal, says}: each merchant hands over one or two items when
                      talked to. An item is a record's name ("leather strip", "wooden shaft", "iron ore" exist) or
                      one to make ({name, description, like})
    target, count (1 or 2), drop, hunt, give       as L3; the hand-in is every material and the drop

L5  kind              "loot" (default): target, count (1-3), drop, hunt, give; chance 1 unless given (a named foe
                      always drops). "kill": target, count (1-5), hunt, back {journal, says}
flight  name, giver ("Gatekeeper ..."), offer, accept, decline, declined, started, journal

L7  hunts             one or two {target, count (1-3), drop, hunt {journal}}: kinds of the shared ground; give {journal, says}
L10 materials         zero to two {merchant, items, journal, says} (as L4; the Eyrie's Quartermaster Dunmore hands ore)
    target, count (1-3), drop, hunt, give           as L4
L13 kind              "kill" (default): target (a named foe, or a kind), count (1-8), hunt, back {journal, says};
                      "loot": target, count (1-3), drop, hunt, give (chance 1 unless given)
L15 target, count (1-3), drop, hunt                  a named foe's drop (chance 1 unless given)
    carry             optional {target, journal, says}: the drop handed to someone (Warden Halvek), then
    back              {journal, says} at the giver; with no carry, give {journal, says} at the giver
L20 road              one to three stops {target, journal, says} (talks)
    target, count (1-3), drop, hunt                  a hunt (one kill in two drops one)
    handin            {target (default the last stop), journal, says}: the drops handed in
    after             zero to two stops more
    final             {target (default the giver), journal, says}: the last word, where the path is picked
    picked            optional: the giver's words when a path is picked ("Then it is yours.")
```

`give`, `show`, `back`, `fetch`, `bind`, `ledger` may add `notyet` (the player's answer while items lack),
`waiting` and `reminder` (FORMAT.md).

## The shared ground: the Goldfeather Eyrie

Tunaria's zone 27 (cell 7,3), a snowfield under the Goldfeather aviaks' eyrie, is world 10's 22nd zone: no race's home
(`"stable": true` in the recipe puts its coachwoman on the homes' coach network: every home's coach and Freeport's run
there, hers to them) and its people and creatures are its zone's `shared` block, seeded by `tools/seed_race_homes.py` as a
home's are (plus `templates`, the creatures' combat templates, `roamer`, and `loot`, their loot region, `arctic`).

- **The outpost** on the frozen river's south-west reach, where the coach sets a rider down (14470, 66.5, 7500): Coachwoman
  Brenna, **Warden Halvek** (a hill giant: L15 carries and L20 roads end with him), **Quartermaster Dunmore** (hands over L10
  materials; sells Merchant Mallie's goods), Spiritmaster Oswin (a group binds here), three sentries. **Scout Feyla** stands
  on the river's north-east reach, watching the eyrie (an L20 road's stop).
- **The creatures**, by the level they serve (every home's chain shares them; each six spawns or more, at the zone's own
  creatures' spots; from the outpost):

| level | kind | spawns | where (from the outpost) | rule |
|-------|------|--------|-------|------|
| 6-7 | drift rat | 9 | the snow east | peaceful |
| 7-9 | snowfield badger | 10 | the snow east | peaceful |
| 9-11 | rime spider | 7 | north-east | peaceful |
| 10-12 | pale owl | 10 | north | peaceful |
| 12-14 | frostpaw cub | 12 | north, past the river | peaceful |
| 14 | **Old Frostjaw** (named) | 1 | the slopes north-east | hostile |
| 15-17 | rime elemental | 10 | the ice caves north-east (under the hills) | hostile |
| 17 | **Rimeheart** (named) | 1 | the ice caves north-east | hostile |
| 18-20 | Goldfeather raider | 10 | round the eyrie, east (its plateau) | hostile |

- **Its drops are one set for every home**: copy the Aviak's `drop` blocks (`drift rat fur`, `snowfield badger pelt`,
  `rime spider silk`, `shard of the Rimeheart`, `Goldfeather war-talon`) as they are; an item two homes make must be made
  the same (the generator refuses two makings of one name). A home may add its own for a kind it hunts differently.
- **Directions** in a journal line about the shared ground are from the outpost (the test measures them from Coachwoman
  Brenna's spot); about the home, from its start, as before.

## The home's people (`tools/worlds/homelands.json`, `tools/seed_race_homes.py`)

A home's block has `start`, `npcs`, `peaceful`, `hostile`, `lines`. Each of `npcs`: `name`, `like` (an npcs row's
id or name whose look, gear and body it copies) or `body` (a CHAR.ESF DictID worn instead, gear off), `level`,
`role` (`talker` or `foe`), `at` [x, y, z], `toward` [x, z] or `facing` (0-255), `size`, `stock` (a seller of
another world whose wares it sells too: that makes it a merchant), `optional`, `note`. Run `tools/seed_race_homes.py
--format` after editing: the file keeps one layout, so branches merge.

Who stands in every home:

- **the giver** of L1 and L2 (the elder), the start 7 or so in front of it and facing it;
- **a second giver** for L3-L5 where it reads better (the cities use one: a captain beside the guildmaster), a few
  steps from the first;
- **an outfitter**, a merchant: it hands over the L1 piece and the L4 materials. Give it a `stock` of a city
  merchant that fits (a tailor's cloth, a smith's ore): CLIENT asks a merchant for its window and nothing else, the
  quests answer that request first, and the window must then sell something;
- **the grocer** (there already), who may hand over an L4 material too;
- **the spiritmaster** and **the gatekeeper** (there already), **one coachman** (new);
- **`Spell Archivist`**, by that exact name, with `"stock": "Spell Archivist", "optional": true`: the Heirloom
  server's spell seller and "Grant basic spells" (`handlers/archivist.go` knows him by the name), so a caster has
  its level 1-5 spells at home; left out, and said, on a database that has none to copy;
- **creatures**: the L3 kind (6-8 spawns, level 2-3), the L4 kind (about 6, level 3-4), the L5 foe (level 5; with a
  kind, level 4-6 and five or more spawns). The old starter critters may be the L3 kind at level 2-3.

Rules the tools hold you to:

- **A creature's name is its home's alone.** `npcaggro` and a quest's kill go by name: a name NPCs of another world
  bear is refused. ("scrub rat", not "rat".)
- **Every creature is `peaceful` or `hostile`** by name. Wildlife of level 5 and under attacks no one unless a rule
  says so, and anything else with a creature's body attacks on sight: say which. Critters peaceful; the L5 foe
  hostile. A name that has an `npcaggro` row already (a GM's) keeps it, and the tool says so.
- A hunt's kind (L3, L4) has six spawns or more; a merchant that hands things over has a `stock`; one coachman, one
  spiritmaster, one gatekeeper a home, each name unique among the homes.

**Places.** Stand everyone on a spot the disc's own NPCs of that zone stand on: the `npcs` rows of worlds 0 and 2
inside the home's cell have floor-true heights (`SELECT id, npc_name, x_coord, y_coord, z_coord FROM npcs WHERE world
= 0 AND x_coord >= 8000 AND x_coord < 10000 AND z_coord >= 20000 AND z_coord < 22000` for Aviak Village). **+x is
east and +z is SOUTH: north is toward smaller z** (Halas, in the north, is at z 4,289; Grobb, in the south, at
31,410; the client's map draws north, -z, up). A height taken from anywhere else floats or sinks. Village people on the village's floor near the start;
creatures at the zone's own creature spots, the L3 kind nearest.

**Bodies by sight, never by name.** A `like` row copies a look that is known; a `body` is chosen from a rendering of
it (`eqoa shot CHAR.ESF <index> out.png` draws any of the 584 bodies), and a small or large `size` says what it is
("the huge wasp's body, half its size").

**Coaches.** A home's coachman is a real stable (`entity/coachhomes.go`): his town is the home's zone name as the
recipe has it, his coach runs to Freeport and to every other home, and Freeport's runs to every home. The house's
rules hold: talking to him signs his ledger; a ride is sold only to a town whose ledger the character has signed
(any coachman on the network offers to sign every ledger at once); a home is offered only to a client that holds the
Homelands. **A rider lands on the coachman's own spot**: stand him on open floor with room around him, away from
anything that attacks, and not where an arrival would block a doorway. He opens his menu when talked to, so he says
no `lines`.

**`lines`** (`npcdialog` by name: what one says when nothing else answers): the gatekeeper's before the five are
done, the sentinels', the givers' afterwards. `%p`, `%r`, `%c` are the listener's name, race and class.

## Adding L7-L20 to a home (a content lane's guide)

The Aviak's (`aviak.json`'s `L7` .. `L20`) is the worked example. For a race:

1. **Givers.** L7-L15 from the home's second giver (its huntmaster, warden, captain: whoever gives L3-L5), L20 from the
   first (the elder, who gave L1-L2): the cities' captain and elder. No new person is needed in the home.
2. **Objectives on the shared ground**, its people and kinds (the table above), each level's on the kinds of its level:
   L7 drift rats and/or snowfield badgers, L10 Dunmore's ore (or no material) and rime spiders or pale owls, L13 Old
   Frostjaw (or a count of frostpaw cubs), L15 the Rimeheart (carried to Warden Halvek), L20 a road through Halvek and
   Scout Feyla to the Goldfeather raiders. A home may hunt in its own zone instead (its own creatures, placed in its
   `home` block, six spawns or more, a level's worth), but the shared ground is where the homes meet: keep at least
   L13-L20 there.
3. **Words** in the race's voice, as its L1-L5: the giver sends the character by its home's coach ("ride Harrek's coach to
   the Eyrie"), says why (the race's own reason), and the journal lines name where from the outpost. The shared people
   are everyone's: their `says` lines answer this race's errand (each step carries its own words).
4. `python3 tools/quests/gen_homelands.py`, `python3 tools/quests/gen_homelands_test.py`, then on a seeded copy
   `check_quests.py`, `import_quests.py --apply`, `EQOA_REAL_DB=copy python3 tools/quests/gen_homelands_test.py`.
5. Play L7 and the L20 pick on a rig (a GM's `#setlevel`, `/goto` and the coach are fine for the rest), and the
   checklist below for the new steps.

## The checklist, per home (shots of each)

On a private server on a seeded copy, with the real client, as a NEW character of the race:

1. **The first view.** At login the giver is in front of the character, a few steps off, facing it; no creature
   within 60; nothing blocks the way to the outfitter.
2. **Footing and facing.** Every person and creature stands on its floor (nothing floating, nothing sunk to the
   knees), outside the art (not in a trunk, a wall, a fire), and faces where a visitor comes from. Walk round each.
3. **The chain, start to end**, on one class with a city chain and one without (the wire race's cities:
   `tools/quests/data/<city>.json`): each offer's boxes and question, each journal line, each step's words; the
   piece in the pack, wearable, and shown while worn; the bind asked and done; the ledger signed; the L3 drops
   dropping and the tracker counting; the L4 hand-overs and hand-in; the L5 foe attacking first; each reward (the
   ability in the book, the weapon in the pack); the gatekeeper refusing before L5 with its own lines and flying
   after; on the mainland the class's guildmaster offering its level 7 quest (not level 1).
4. **The hunts' ground.** Each kill target is reachable on foot from the village without crossing anything that
   attacks a level 3, and the L5 foe stands alone (nothing else joins its fight).
5. **What a new player leans on**: the client's map (M) and minimap show the home; the quest marks stand over each
   step's NPC (giver, merchant, spiritmaster, coachman, gatekeeper) and the hunt's kind; the quest tracker counts;
   the welcome guide's boxes come at the first login. If one is blank or wrong in the Homelands, write down exactly
   what, with the shot: it is the client's to fix, not the content's.
6. **The stable.** The coachman's menu: his town's ledger signed on the first talk, Freeport and the other homes
   offered once their ledgers are signed, a ride out and back landing on open floor beside each coachman.
7. **A visitor.** A character of another race at the givers: no quest of the home offered; the merchants, the
   spiritmaster and the Spell Archivist serve it.
8. **Words.** In EQOA's voice: short, plain, the race's own manner; no line the client cuts (the generator checks);
   directions that are true: check every direction against the client's compass, not the numbers alone (the test
   holds a journal line's north, south, east and west to where its step's NPC or creatures stand from the start, and
   catches a word turned round; it cannot tell a path from a hill); each name spelled one way everywhere.

## Commands

    python3 tools/quests/gen_homelands.py --list           each race: its spec, or its legacy three
    python3 tools/quests/gen_homelands.py                  write tools/quests/data/homelands.json
    python3 tools/quests/gen_homelands.py --check          is the file what the specs give?
    python3 tools/seed_race_homes.py --format              the recipe file in its own layout again
    python3 tools/seed_race_homes.py --db copy.db          what a seeding would write (and what it leaves out)
    python3 tools/seed_race_homes.py --db copy.db --undo && python3 tools/seed_race_homes.py --db copy.db --apply
    python3 tools/quests/check_quests.py --db copy.db tools/quests/data/homelands.json
    python3 tools/import_quests.py --db copy.db --apply tools/quests/data/homelands.json

**Over a seeding that is there** (the live server's), the server stopped, in this order: `seed_race_homes.py --undo`,
`--apply` (the new recipe), `import_quests.py --apply tools/quests/data/homelands.json`, `retire_quests.py --apply
tools/quests/data/homelands.json` (the old welcome and hunt go, with the lines and marks characters had in them),
`homes_flown.py --apply` (one who had flown under the old two keeps its passage: its home's five are marked finished).
Never `import_quests.py --undo`: it takes back every import of the game.

An item the importer has made is not made again: a piece's `set` or a drop's `like` changed after an import needs a
fresh copy of the database (or the record mended by hand). The server reads people and quests at its start.
