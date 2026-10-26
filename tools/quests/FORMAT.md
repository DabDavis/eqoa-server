# Quest content files

`tools/quests/data/<city>.json` holds a starting city's class quests, written from EQOA-Data's
guides (`Quests/1-20/<city>/<class>.json`, github.com/Jadiction/EQOA-Data: prose in Markdown, one
`# Level N` section a quest or a few). `tools/import_quests.py` writes them into `quests` and
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
| `key` | stable and unique: the guide's key and its section, `1-20/freeport/warrior/L3`; a second quest in one section `.../L20b` |
| `name` | the quest's name, the guide's where it gives one, else a short one of ours |
| `giver` | the NPC that offers it, as `npcs.name` spells it |
| `classes` | class ids (WAR 0 ... ALC 14) the guide is for; `races`: race ids, empty for any |
| `minlevel`, `maxlevel` | the section's level; `maxlevel` 60 unless the guide says otherwise |
| `requires` | keys of the quests to have finished first: a class's chain, each section after the one before |
| `offer` | the giver's words offering it (the question box's paragraph), in the giver's voice, from the guide's facts |
| `accept`, `decline` | the two answers; `declined` the giver's reply to a refusal; `started` its words once taken |
| `steps` | Step, in order |
| `events` | Event, on the quest itself (`accept`, `finish`) |
| `unresolved` | every name the file needs that the database lacks (an NPC, a creature, an item, a spell) |
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
| `rewards` | `{"xp": n, "tunar": n, "items": [names], "spells": [names], "factions": [["<faction>", n], ...]}` given when the step is done; the guide's figures (usually on the last step) |
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
an earlier one wrote, except a quest a GM has changed since. A quest is a draft when a name it uses is
not in the database, when its own `unresolved` lists anything, or when a quest it requires is missing
or a draft. Stop the server to apply, start it after.
