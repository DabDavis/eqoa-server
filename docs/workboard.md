# The work board: next action, waiting on, basis

The work board (`workboard` table; the GM panel's Work board page, `work_board` / `post_work` over MCP and
OpenAPI, `cmd/workboard` on the box) says who is changing what. Since 2026-11-24 (jib's board #320-#324, both owners'
approval) each entry also says what happens next and why its change was made. Four columns on the entry's own
row, nothing else stored:

| Field (JSON and column) | CLI flag | What |
|---|---|---|
| `next` | `-next` | the one next action, exact (a command, an id), one line, at most 500 characters |
| `waiting_on` | `-waiting` (or `-waiting-on`) | who or what it waits on: a person, a restart, a login; at most 100 |
| `basis` | `-basis` | why: `era`, `decomp`, `shared`, `owner`, `estimate`, `test`, or empty |
| `basis_ref` | `-basis-ref` | a short source or reference, at most 120: `Prima p.212`, `decomp viswing.cpp`, `owner 2026-11-24`, `petition 5590` |

With `owner`, `status` and `version` (as before) these are what a view of who acts next is computed from
("needs Jibrish", "waiting on Dab"): the board is the record, a view of it is never a second copy. A handoff
checks the entry's `version` at read time; when they disagree, the board wins.

`""` clears a field. Marking an entry done clears `next` and `waiting_on` unless the same change gives them.
Clearing `basis` clears `basis_ref`. Every change is a version and a `gm_audit` line, as any board edit.

The bases:

- `era`: a source from the live game's time (Prima, SOE's records, Allakhazam, a period guide, a capture), cited in
  `basis_ref`. A player's memory alone is not one.
- `decomp`: the decomp or CLIENT's own handling.
- `shared`: a defect in code or data both shards share; the defect is its evidence.
- `owner`: the owner's decision (`basis_ref`: `owner 2026-11-24`).
- `estimate`: a fitted or guessed value, to re-check when evidence turns up.
- `test`: operational, or a tester's grant: outside the accuracy record.

## The resume convention (multi-step work in progress)

- **Last verified**: a note that begins `Verified` or names an audit or receipt id
  (`Verified: npcs 121689 renamed, read back (audit 163600)`). It is not stored twice: `-json` derives
  `last_verified` (the newest such note) from the notes.
- **Next**: `next`, the exact next step. Once the last step left is the final check, `next` is it
  (`Final check: read the item ids back after the next login`); a final check that must be written earlier goes in
  a note. A readback compares ids against what was meant, not "the item is there".
- **Final check**: in `next` or a note, as above.
- **Waiting on**: `waiting_on`.

Reuse ids rather than text: a note or `basis_ref` names `audit 163600`, `petition 5590`, `board 2123`, never a copy
of what that record says.

## Commands

```
./workboard [-db ~/eqoa-live/eqoa.db] list -json                 every entry with every field
./workboard show 2123 -json                                       one entry, its history (gm_audit) and overlaps
./workboard list -basis owner                                     entries of one basis (none: those without)
./workboard set 2123 -next "rehearse #310 on VACUUM INTO copies" -waiting dab -basis test
./workboard note 1959 -next "Final check: readback after the next login" "Verified: renamed (audit 163600)"
./workboard add -title "..." -touches "..." -status in_progress -basis shared -basis-ref "petition 5590"
./workboard done 2123 -result "one line"
```

Over the panel: `post_work {"id": 2123, "next": "...", "waiting_on": "dab", "basis": "era", "basis_ref": "Prima p.212"}`;
`work_board {"basis": "era"}`.

## Direct changes carry their basis in the audit line

A change that is not a board entry's own says why in its `gm_audit` line's `detail`, which is text, so as a tag at
its start: `[basis:shared petition 5590] the reason` (`database.BasisTag`). No new column.

- **GM panel row edits** (`insert_row`, `clone_row`, `update_row`, `delete_row`, `update_rows`, `delete_rows`,
  `sql_execute`) take optional `reason`, `basis` and `basis_ref`; a bad basis is refused before anything is written.
- **Data tools on `tools/tix4_rows.py`** (most `petpass_*`) take `--basis` and `--basis-ref`, or `basis` /
  `basis_ref` in their `SPEC`. A tool that writes `gm_audit` itself starts its detail with the same tag.
- **Releases** (`deployNNN.sh`): the board entry the script posts carries it,
  `./workboard add ... -basis shared` (or `owner`, `era` ...).
- When the detail already names the petition, `[basis:shared]` alone is enough; when the change belongs to a board
  entry with a basis, the reason names the entry (`board 2123`) instead of repeating it.

## Finding the basis of a Live change

- **From a work id**: `./workboard -db ~/eqoa-live/eqoa.db show <id>` (`basis`, `basis_ref`; its history lists the
  audit ids).
- **From an audit id**: `audit_log {"search": "..."}` or the panel's audit page: the detail begins `[basis:...]`, or
  names `board N` (then the entry's basis) or a petition.
- **From a row**: the panel's history of the row (`audit_log {"table": "npcs", "row_id": 121689}`) lists its lines,
  each with its tag. `audit_log {"search": "[basis:estimate"}` lists every change of one basis.

A change with neither a tag nor an entry has no recorded basis: that is the gap to close, not a fallback.
