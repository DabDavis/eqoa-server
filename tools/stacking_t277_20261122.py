#!/usr/bin/env python3
"""The players' Ability Stacking Guide's eight exceptions, checked and fixed (the owner, 2026-11-22: "check all 8 and
fix them"; the guide: eqoa.forumotion.com t277, "Exceptions to the rule").

A workbook spell's line is its stack type, its bearer and its stats (internal/entity/buffline.go): two buffs of one
stack type that share a stat are one line, and only the better counts. Since 2026-11-14 (tools/buff_lines_20261114.py)
each class's buff family is a stack type of its own, so different classes' buffs stack. What the guide's exceptions
say, and what this writes (spelleffect <id>: the effects' "stack", nothing else):

  1. Cleric and druid AC/HP lines: "This is to make sure they don't stack with buffs from the same line, but do stack
     with the other class's buffs". Ours stacked a line's single and group forms: the cleric's Courage to Order were
     stack 237 and Group Courage to United Faith 700189; the druid's Woodskin to Natureskin and Tunare's Blessing (and
     the plain Vitae Blessing, a single target's line) 1, and Group Woodskin to Group Blessing and Vitae's Blessing
     700834. Now Courage to Order 237 -> 700189 (the guide: all the cleric's "group"), the druid's group/AoE forms
     700834 -> 1 (the guide: all the druid's "target"; 1 so that the plain Vitae Blessing, which goes by its scope's
     stack type, is still one line with them).
  2. Alchemist and shaman stat lines: the same. The alchemist's Clever Thought line (700002) and Focused Intent line
     (700065) -> 700833, their group forms' (the two lines share no stat, so they stay two lines); the shaman's
     Finesse line (700062) and Tribal Toughness line (1) -> 700149, their group forms' and Tribal Spirit's. LEFT:
     Concentration (DEX/WIS) and Inspiration (AGI/INT), the CMs' level 60 AoEs, keep stack types of their own: the
     records give the single and group lines AGI/WIS and DEX/INT (their descriptions and SOE's 2006 notes say AGI/INT
     and DEX/WIS), so each shares one stat with each line and would take both off (the 2026-11-14 tool's finding).
  3. Enchanter PoT: the single forms (stack 1) and the Empathy CM's group forms (3) already stack (no change).
  4. Form of the Master and Wolf's Instinct: already stack with self, target and group buffs (no change).
  5. Werehunter buffs, "for tanks and melees, category 4 (form). For casters and priests, category 2 (self)": Blessing
     of Valentina (the tanks', self 2) -> 4 and Troka's Blessing (the melees', none 0) -> 4; Blessing of Yazmina and
     Blessing of Luludja (casters', priests') stay 2.
  6. Short tank AC buffs, "the 12-24 second AC buffs included in the various taunt paths ... effective in the first or
     target category": the taunts' AC on their caster had no stack type (0), so no line, and two taunts' AC added up.
     Now 1: one line among themselves (their AC is the caster's own, "+AC@self", so no other class's target buff's),
     still stacking with self and group AC.
  7. Necromancer DoTs: no spell named, and ours lets every DoT on a creature tick: nothing written (the report).
  8. Lycan buffs, racial CMs and MCs, some MCAs, "yet to be altered" (they stack with everything, as Pure Heart, Nine
     Lives, Barkskin, Deathless, Living Stone, Leadership, Guided Meditation and Catnip already do): the were
     abilities the were quests give and the racial master classes' buffs, each a stack type of its own (its id).

Spells are found by name on each shard (ids differ between shards); a name with no row, more than one candidate row,
or a stack type not as read is refused, nothing written. Report only by default; --apply in one transaction, a
gm_audit line a row, log stacking_t277_1122_log; --undo exact (a row changed since is left and said). spelleffect is
read at every cast: no restart, no reload (an effect already on a player keeps the line it landed with until it ends).

    python3 -E -s tools/stacking_t277_20261122.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "stacking_t277_1122_log"
WHO, VIA = "dab", "tool"
DETAIL = "Ability Stacking Guide's exceptions (the owner: check all 8 and fix them): tools/stacking_t277_20261122.py"
OWN = "own"  # a stack type of the spell's own (its id on that shard)

# (exception, name, stack now, stack after)
PLAN = (
    [("1 cleric", n, 237, 700189) for n in ("Courage", "Center", "Daring", "Bravery", "Valor", "Resolution", "Order")]
    + [("1 druid", n, 700834, 1) for n in ("Group Woodskin", "Group Rockskin", "Group Steelskin", "Group Diamondskin",
                                          "Group Natureskin", "Group Blessing", "Vitae's Blessing")]
    + [("2 alchemist", n, 700002, 700833) for n in ("Clever Thought", "Creative Thinking", "Ingenious Thought",
                                                    "Inspired Thinking", "Prodigious Thought")]
    + [("2 alchemist", n, 700065, 700833) for n in ("Focused Intent", "Honed Intent", "Applied Intent", "Direct Intent",
                                                    "Cunning Intent")]
    + [("2 shaman", n, 700062, 700149) for n in ("Finesse", "Deftness", "Accuracy", "Precision", "Exactness", "Acumen")]
    + [("2 shaman", n, 1, 700149) for n in ("Tribal Toughness", "Tribal Brawn", "Tribal Strength", "Tribal Force",
                                            "Tribal Might", "Tribal Thew")]
    + [("5 werehunter", "Blessing of Valentina", 2, 4), ("5 werehunter", "Troka's Blessing", 0, 4)]
    + [("6 taunt AC", n, 0, 1) for n in (
        "Pillar of Might", "Pillar of Strength", "Pillar of Force", "Pillar of Deeds", "Pillar of Heroics",
        "Shadow Tower", "Umbral Tower", "Grim Tower", "Doom Tower", "Cursed Tower",
        "Shining Bastion", "Gleaming Bastion", "Hallowed Bastion", "Sacred Bastion", "Blessed Bastion",
        "Goading Ridicule", "Ozmo's Careful Teasing", "Ozmo's Echoing Mockery", "Ozmo's Infuriating Jeer",
        "Ozmo's Maternal Asperation", "Ozmo's Unrelenting Ridicule", "Ozmo's Goading Ridicule",
        "Ascended Blessed Bastion", "Ascended Doom Tower", "Ascended Goading Ridicule",
        "Ascended Ozmo's Careful Teasing", "Ascended Ozmo's Echoing Mockery", "Ascended Ozmo's Infuriating Jeer",
        "Ascended Ozmo's Maternal Asperation", "Ascended Ozmo's Unrelenting Ridicule")]
    + [("8 lycan", n, s, OWN) for n, s in (
        ("Growl of Battle", 3), ("Rage of Drinal", 2), ("Cave Bear Strength", 2), ("Bear Nobility", 2),
        ("Defense of the Swarm", 3), ("Swarm Mentality", 3), ("Cunning Subterfuge", 2), ("Agility of Kehrahn", 2),
        ("Lion's Nobility", 2), ("Reptilian Frenzy", 2), ("Reptilian Gaze", 2))]
    + [("8 racial MC", n, s, OWN) for n, s in (
        ("Enrage", 2), ("Battle Regeneration", 2), ("Regrowth", 2), ("Marr's Blessing", 3), ("Updraft", 2),
        ("Howl of the Pack", 3), ("War Cry", 3), ("Tunnel Dash", 2))]
)

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    keycol TEXT NOT NULL,
    keyval TEXT NOT NULL,
    col    TEXT NOT NULL,
    before TEXT NOT NULL,
    after  TEXT NOT NULL,
    audit  INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


def audit(db, tbl, keycol, keyval, col, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, tbl, compact({keycol: keyval}),
                      compact({col: before}), compact({col: after}), DETAIL))
    return cur.lastrowid


def effects_of(text):
    """The effects object, or None when it is not a workbook row in its compact form."""
    try:
        eff = json.loads(text)
    except ValueError:
        return None
    if not isinstance(eff, dict) or compact(eff) != text:
        return None
    return eff


def plan(db):
    """(changes, lines, refusals)."""
    changes, lines, refusals = [], [], []
    for ex, name, old, new in PLAN:
        rows = db.execute("SELECT p.spellid, e.effects FROM spellpattern p JOIN spelleffect e USING (spellid) "
                          "WHERE p.spellname = ?", (name,)).fetchall()
        cands = []
        for sid, text in rows:
            eff = effects_of(text)
            if eff is None:
                continue
            want = sid if new == OWN else new
            if eff.get("stack", 0) in (old, want):
                cands.append((sid, text, eff, want))
        if len(cands) != 1:
            refusals.append(f"{ex}: {name!r}: {len(cands)} rows of stack {old} or after (rows by the name: "
                            f"{[r[0] for r in rows]})")
            continue
        sid, text, eff, want = cands[0]
        if eff.get("stack", 0) == want:
            lines.append(f"  {ex:<13} {sid} {name}: stack {want} already")
            continue
        eff["stack"] = want
        changes.append(("spelleffect", "spellid", sid, "effects", text, compact(eff)))
        lines.append(f"  {ex:<13} {sid} {name}: stack {old} -> {want}")
    return changes, lines, refusals


def write(db, tbl, keycol, keyval, col, before, after):
    cur = db.execute(f"UPDATE {tbl} SET {col} = ? WHERE {keycol} = ? AND {col} = ?", (after, keyval, before))
    return cur.rowcount == 1


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, tbl, keycol, keyval, col, before, after, _ in db.execute(f"SELECT * FROM {LOG} ORDER BY seq DESC").fetchall():
        if not write(db, tbl, keycol, int(keyval), col, after, before):
            print(f"left: {tbl} {keyval} changed since")
            left += 1
            continue
        audit(db, tbl, keycol, int(keyval), col, after, before)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        if a.undo:
            db.execute("BEGIN IMMEDIATE")
            try:
                back, left = undo(db)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            print(f"put back {back} rows, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if table(db, LOG):
            print(f"applied already ({LOG} is there): --undo first")
            return 0
        changes, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not changes:
            print("nothing to do")
            return 0
        if not a.apply:
            print(f"report only: {len(changes)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            db.execute(LOG_SCHEMA)
            for tbl, keycol, keyval, col, before, after in changes:
                if not write(db, tbl, keycol, keyval, col, before, after):
                    raise RuntimeError(f"{tbl} {keyval} changed under the update")
                a_id = audit(db, tbl, keycol, keyval, col, before, after)
                db.execute(f"INSERT INTO {LOG} (tbl, keycol, keyval, col, before, after, audit) VALUES (?, ?, ?, ?, ?, ?, ?)",
                           (tbl, keycol, str(keyval), col, before, after, a_id))
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {len(changes)} rows, log {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
