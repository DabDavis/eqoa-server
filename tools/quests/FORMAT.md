# Quest content files

`tools/quests/data/<city>.json` holds a starting city's class quests, written from EQOA-Data's
guides (`Quests/1-20/<city>/<class>.json`, github.com/Jadiction/EQOA-Data: prose in Markdown, one
`# Level N` section a quest or a few); `tools/quests/data/<category>.json` a later category's (17,
23-47, 30-40-49, 53-59, 60, hunt-and-gather, tank-exemplar, weres, misc, portal: one file a
category, its `city` the category). `tools/import_quests.py` writes them into `quests` and
`queststeps` (`--apply`, `--undo`, every row it writes logged), resolving every name against the
database it is given. A quest whose names do not all resolve is written as a draft (`enabled` 0)
with what is missing in its notes, so only whole quests are offered to players.

## A file

```json
{
  "city": "freeport",
  "quests": [ Quest, ... ],
  "items": [ Item, ... ]
}
```

## Quest

| field | what |
|-------|------|
| `key` | stable and unique, no spaces: the guide's key and its section, `1-20/freeport/warrior/L3`; a second quest in one section `.../L20b`; a later guide's `<EQOA-Data folder>/<guide file stem>/<part>` (`23-47/23/p1`, `30,40,49/group-1-good/p2`, `60/good-tank/ep2-3`), the GM panel filing it under its folder's category (`misc`, `30-40-49`, `tank-exemplar`) |
| `name` | the quest's name, the guide's where it gives one, else a short one of ours |
| `giver` | the NPC that offers it, as `npcs.name` spells it |
| `classes` | class ids (WAR 0 ... ALC 14) the guide is for; `races`: race ids, empty for any |
| `minlevel`, `maxlevel` | the section's level; `maxlevel` 60 unless the guide says otherwise |
| `requires` | keys of the quests to have finished first: a class's chain, each section after the one before |
| `factions_required` | optional: `[["<faction>", min], ["<faction>", min, max]]`, the standings it asks for (-2000 to 2000, `max` not below `min`), each faction as the `factions` table spells it. It is offered (and marked on a client's map) only while the player's standing with every one is at least `min` and, when given, at most `max`; once taken, its steps at its giver wait on them too (the giver says it will not deal with the player, a line of ours), while a step at another NPC, a kill, a loot or a reach goes on. A faction the database lacks makes the quest a draft |
| `repeatable` | optional, `true`: it may be taken again once finished, and its rewards are given again |
| `unlocks` | optional: `[{"npc": "<travel NPC as npcs spells it>", "to": "<the place>"}]`, the trips it opens: that waysender or portal (`internal/handlers/waysender.go`'s routes) offers its trip to `to` only to a player who has finished this quest, or another that unlocks the same trip, while one of them is in the game (a draft's bars no one; a trip only a quest opens, `QuestOnly` in `waysender.go` as the Portal guides' are, is offered to no one else even then, and a player who finished one of its quests keeps it). `to` is matched to a route's label by case, a leading "the" and its letters and digits ("the grobb" is Grobb, "Klick'Anon" is not "Klik'Anon"); one naming no route yet is said by the importer and the checker and written all the same, to open the trip once the route is wired |
| `offer` | the giver's words offering it (the question box's paragraph), in the giver's voice, from the guide's facts |
| `accept`, `decline` | the two answers; `declined` the giver's reply to a refusal; `started` its words once taken |
| `steps` | Step, in order |
| `events` | Event, on the quest itself (`accept`, `finish`) |
| `unresolved` | every name the file needs that the database lacks (an NPC, a creature, an item, a spell, a faction: a class's rewards' and `factions_required`'s too; an unlock's NPC and route are not, they only open a trip) |
| `notes` | what was simplified or left out, and why |

## Step

| field | what |
|-------|------|
| `kind` | `talk` (talk to `target`), `show` (show `items`, kept), `give` (hand over `items` and `tunar`), `kill` (`count` of `target`), `loot` (kill `target` until `count` of `items[0]` have been taken off their corpses during this step: each kill of one drops one with `chance` for a player on this step, onto that player's view of its corpse; what was held before the step does not count), `reach` (`world`, `x`, `y`, `z` within `radius`) |
| `target` | the NPC or creature, as `npcs.name` spells it |
| `items` | `[{"item": "<itempattern name>", "count": n}]` |
| `tunar`, `count`, `chance` | as `kind` needs them; `chance` 0-1 is ours (the guides give none), 0 every kill |
| `world`, `x`, `y`, `z`, `radius` | a `reach` step's place, game coordinates |
| `journal` | the journal's line for the step (what to do next), one short sentence |
| `says` | what `target` says when the step is done |
| `waiting`, `notyet`, `reminder` | optional: the NPC's words while the step waits, the answer while items lack, the giver's reminder |
| `answer` | optional: the player's answer that does a talk, show or give step (its first line; more lines are answers that change nothing); none: done on talking |
| `rewards` | `{"xp": n, "tunar": n, "items": [names], "spells": [names], "factions": [["<faction>", n], ...], "byclass": {...}}` given when the step is done; the guide's figures (usually on the last step) |
| `rewards.byclass` | optional: `{"<class id 0-14>": {"items": [names], "spells": [names], "xp": n, "tunar": n}}`, the guide's "Rewards by Class": given besides the step's own to a player of that class, each spell that class's own (the `classspells` row of the name), and named in that class's marks on the map |
| `choices` | optional, a talk, show or give step's: `[{"answer": "<the player's answer, 63 characters>", "says": "<the NPC's words when it is picked>", "stay": true, "rewards": {"items": [names], "spells": [names], "xp": n, "tunar": n}}]`, at most six (a guide's "choose your epic weapon"). Asked when the step is done at its NPC (after its `answer`, when it has one); the picked option's words are said and its rewards given besides the step's own (its spells the quest's class's own), the marks naming them "a choice of A or B"; an option with `stay` does not do the step (a wrong answer: its words said, the player may talk again) |
| `choose` | optional: the choice's question (none: "Which do you choose?", ours) |
| `events` | Event, on this step (`start`, `done`) |

## Item

A quest item a step needs that `itempattern` may lack (a belt that drops, a letter an NPC hands
over): `{"name", "description", "notrade": true, "like": "<an item whose look it takes>"}`.
The importer makes it when no item has the name.

## Event

`{"on": "accept" | "finish" | "start" | "done", "do": <action>, ...}`: what happens then, the
engine's (`handlers/quests.go`); its actions are listed there and in the GM panel's quest page.
`popup` (`text`) is a window the player closes; `say` (`npc`, `text`); `give` (`items`); `spawn`
(`npc`, a copy beside the player for the step, gone after; `"hostile": true` makes it one to kill:
attackable, and it attacks the player, which is how a step has a townsperson killed); `visual` (`fx`: a
DictID like `0x1DAEB5B6`, or a spell's name for that spell's visual; `seconds`); `teleport`
(`world`, `x`, `y`, `z`). One popup at a time: the client shows one question box at once.

## The import

`tools/import_quests.py [--db eqoa.db] [FILE ...]` reports what it would write (every file here by
default; `--verbose` says why each draft is one), `--apply` writes it, `--undo` takes it all back.
Quests keep their ids by their keys (`quest_import_quests`, from 100001), so a run again rewrites what
an earlier one wrote, except a quest a GM has changed since (its trips count: a GM's unlock row on it
is a change). A quest is a draft when a name it uses (an NPC, a creature, an item, a spell, a faction,
a class's reward) is not in the database, when its own `unresolved` lists anything, or when a quest it
requires is missing or a draft. Its `factions_required` go in `quests.factionreq`
(`Faction:min` or `Faction:min..max`), a step's `byclass` in `queststeps.byclass` (a JSON object by
class id, items and spells as ids), a step's `choices` in `queststeps.choices` (`{"ask", "options"}`,
items and spells as ids), its `unlocks` in table `questunlocks` (`--routes` names the
`waysender.go` they are matched to; this tree's by default). `--undo` takes the trips too. Stop the
server to apply, start it after.

    python3 tools/quests/check_quests.py --db eqoa.db tools/quests/data/misc.json
    python3 tools/import_quests.py --db eqoa.db tools/quests/data/misc.json            # the report
    python3 tools/import_quests.py --db eqoa.db --apply tools/quests/data/misc.json

## Keeping the files in step with a database

`tools/quests/fix_names.py --db eqoa.db [FILE ...]` (every file here by default) respells a name the
files use as the database spells it, where the one spelling agrees but for case, apostrophes and
backticks, hyphens, a leading "the" and plurals (`--rename [KEY:]OLD=NEW` for any other), sets each
quest's `unresolved` to what `check_quests.py` finds missing, and drops the notes' sentences that only
said a name now found was missing (and a chain's "It follows L5: L7 before it is a draft ...", its
`requires` put back on the quest right before it once that one is whole). A sentence that says more is
listed to be worded again by hand. It reports by default; `--write` writes. Run it after a seed adds
NPCs (`tools/seed_quest_npcs.py`), then `check_quests.py`. An unlock's travel NPC is respelled as an
NPC is, a faction (`factions_required`, a reward's) as the `factions` table spells it.

## The people and creatures a set of quests needs

`tools/quests/npcs/<set>.json` lists the NPCs and creatures a set of quest files needs and `npcs` lacks,
for `tools/seed_quest_npcs.py` to place (beside its built-in set `1-20`, the city guides'):

```json
{
  "set": "qa",
  "npcs": [ Spec, ... ],
  "attackable": [ "<an npcs name whose NPC bit a quest's kill clears>", ... ],
  "peaceful": [ "<an npcs name the quests talk to that attacks on sight>", ... ]
}
```

`attackable` names townsfolk a set's quests kill: their NPC bit (0x80) is cleared. `peaceful` names
creatures a set's quests talk to that attack on sight (a creature's body): each gets an `npcaggro` row
(aggressive 0, source `questnpcs:<set>`), the row the name had kept in the set's log and put back at
its undo (unless a GM changed it since); a name with a peaceful row already is left as it is.

| field | what |
|-------|------|
| `name` | as the quest files spell it |
| `role` | `foe` (what a quest kills: npc_type 0) or `talker` (whom it talks to: 128) |
| `body` | `["race", "<human, elf, darkelf, gnome, dwarf, troll, barbarian, halfling, erudite, ogre>", 0 male or 1 female]`, `["kind", "<a KIND key of the tool>"]` (bear, orc, spider ...) or `["npc", "<an npcs name whose body it copies>"]` |
| `place` | `["ff"]` (where FrontiersForge saw it; not a spot its placeholders share), `["poi", "tunaria", "rathe" or "odus", "<a point of EQOA-Data's map>"]` (the Rathe's points are the sheet's inset, south Tunaria's beside it in the same box Tunaria's: a point of the other region is refused), `["near", "<npc>", distance, world]` or `["at", world, x, z]`; any of them with a last `"low"` for a lower floor |
| `count` | how many spawns (1); `level` its level (none: the sighting's, the map's); `note` the guide's words for its place; `water` `land` (out of the water), `water` (a fish) or `both` |

Each file is a set of its own: `--apply --set qa` writes it, `--undo --set qa` takes exactly its rows
back out (and its `attackable` NPC bits back), whatever other sets are in; a report (no `--apply`)
plans every set not yet in. Run `fix_names.py` and `check_quests.py` after it, then the import:

    python3 tools/seed_quest_npcs.py --db eqoa.db --ground worlds/ground --set qa           # the report
    python3 tools/seed_quest_npcs.py --db eqoa.db --ground worlds/ground --apply --set qa
    python3 tools/quests/fix_names.py --db eqoa.db --write tools/quests/data/misc.json
