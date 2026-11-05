#!/usr/bin/env python3
"""Two records of the were lines set right (the user, 2026-11-05: "we also need to make sure were
infection works and that the were creatures are in the correct zones with decent population").

- The gatorweres' body. The six round The Bone Crusher in Dinbak (tools/seed_quest_npcs.py set qc,
  "the server has none") were given an alligator's body (1041154017, a raging alligator's). The
  Gatorwere form the gator line's infection teaches shows 740132644 (EQOAEmu's workbook sprite
  0x24871D2C read the other way round, entity/spelljson.go's spriteBody), and every gatorwere and
  weregator Sandstorm shows wears that body at scale 1.1 (the Odus census tools/seed_sandstorm_census.py
  seeded, census-odus.jsonl: Elder Gatorwere, Elder Weregator, Ancient Gatorwere, Ancient Weregator,
  every sighting).
  Each npcs row named gatorwere (any case) on the alligator's body that a seed wrote
  (quest_npc_seed_rows) is given that body and scale; a row no seed wrote (a GM's) is left, said.
- The Wolfwere form (901824, tools/import_eqoaemu_spells.py's from the workbook, "made") lowers INT
  by 16 twice, in its line and its effects; the guide's table (EQOA-Data Quests/Weres/wolfwere.json,
  Form Stats: Wolfwere STR 45, STA 45, INT -16) once, as the other four first forms' records do
  (Bearwere, Lionwere, Gatorwere: the stats, then INT -16 resistable). The copy without the resist
  is taken out of spelleffect.effects and the line. (tools/quests/data/weres.json's wolf infection
  teaches this form now, not Werewolf Form, an older record that changes no stat:
  tools/import_quests.py writes that.)

Every value changed is logged in were_lines_fix_log (table, row, column, old, new); --undo puts back
each one still as this left it (one changed since is left and said) and drops the log; a second
--apply is refused while the log is there. Report only by default. The server reads npcs at its
start: apply with it stopped (a swap's before.sh), or restart after; a spell's effects are read at
each cast.

    tools/fix_were_lines.py [--db eqoa.db] [--apply | --undo]
"""

import argparse
import json
import shutil
import sqlite3
import time

ALLIGATOR = 1041154017   # a raging alligator's body: the qc seed's gatorweres
WEREGATOR = 740132644    # the Gatorwere form's and Sandstorm's gatorweres' body
WEREGATOR_SIZE = 1.1     # Sandstorm's scale for every one of them
WOLFWERE = 901824        # the Wolfwere form

LOG = "were_lines_fix_log"
LOG_DDL = f"""CREATE TABLE {LOG} (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl TEXT NOT NULL,
    rid INTEGER NOT NULL,
    col TEXT NOT NULL,
    old TEXT,
    new TEXT,
    why TEXT NOT NULL DEFAULT '',
    applied TEXT NOT NULL
)"""


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def gator_changes(db):
    """The gatorweres to give the weregator's body: (changes, the rows left)."""
    seeded = set()
    if has_table(db, "quest_npc_seed_rows"):
        seeded = {r[0] for r in db.execute("SELECT npc_row FROM quest_npc_seed_rows")}
    out, left = [], []
    for rid, name, model, size in db.execute(
            "SELECT id, npc_name, modelid, size FROM npcs WHERE lower(trim(npc_name)) = 'gatorwere' AND modelid = ? ORDER BY id",
            (ALLIGATOR,)):
        if rid not in seeded:
            left.append((rid, name))
            continue
        why = "the Gatorwere form's body and Sandstorm's gatorweres' (740132644, scale 1.1)"
        out.append(("npcs", rid, "modelid", model, WEREGATOR, why))
        if size != WEREGATOR_SIZE:
            out.append(("npcs", rid, "size", size, WEREGATOR_SIZE, why))
    return out, left


def wolfwere_changes(db):
    """The Wolfwere form's INT -16 once: the changes to its spelleffect row."""
    row = db.execute("SELECT line, effects FROM spelleffect WHERE spellid = ?", (WOLFWERE,)).fetchone()
    if row is None:
        return []
    line, effects = row
    try:
        d = json.loads(effects)
    except ValueError:
        return []
    es = d.get("effects", [])
    ints = [i for i, e in enumerate(es) if e.get("kind") == "stat" and e.get("stat") == "int" and e.get("amount") == -16]
    if len(ints) != 2:
        return []
    plain = [i for i in ints if not es[i].get("resist")]
    if len(plain) != 1:
        return []
    d["effects"] = [e for i, e in enumerate(es) if i != plain[0]]
    new_effects = json.dumps(d, separators=(",", ":"))
    why = "the guide's Form Stats: Wolfwere INT -16, once (EQOA-Data Quests/Weres/wolfwere.json)"
    out = [("spelleffect", WOLFWERE, "effects", effects, new_effects, why)]
    twice = ", Decrease INT by 16, Decrease INT by 16"
    if twice in line:
        out.append(("spelleffect", WOLFWERE, "line", line, line.replace(twice, ", Decrease INT by 16"), why))
    return out


KEY = {"npcs": "id", "spelleffect": "spellid"}


def apply(db, changes):
    db.execute(LOG_DDL)
    now = time.strftime("%Y-%m-%d %H:%M:%S", time.gmtime())
    n = 0
    for tbl, rid, col, old, new, why in changes:
        cur = db.execute(f"UPDATE {tbl} SET {col} = ? WHERE {KEY[tbl]} = ? AND {col} IS ?", (new, rid, old))
        if cur.rowcount:
            db.execute(f"INSERT INTO {LOG} (tbl, rid, col, old, new, why, applied) VALUES (?, ?, ?, ?, ?, ?, ?)",
                       (tbl, rid, col, json.dumps(old), json.dumps(new), why, now))
            n += 1
    return n


def undo(db):
    back, kept = 0, []
    for seq, tbl, rid, col, old, new in db.execute(f"SELECT seq, tbl, rid, col, old, new FROM {LOG} ORDER BY seq DESC").fetchall():
        cur = db.execute(f"UPDATE {tbl} SET {col} = ? WHERE {KEY[tbl]} = ? AND {col} IS ?", (json.loads(old), rid, json.loads(new)))
        if cur.rowcount:
            back += 1
        else:
            kept.append(f"{tbl} {rid} {col}")
    db.execute(f"DROP TABLE {LOG}")
    return back, kept


def short(v):
    s = str(v)
    return s if len(s) <= 90 else s[:87] + "..."


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default="eqoa.db")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the changes (a copy of the database first)")
    g.add_argument("--undo", action="store_true", help="put back every value logged")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    a = ap.parse_args()
    writing = a.apply or a.undo
    if writing and not a.no_backup:
        dst = f"{a.db}.bak-{time.strftime('%Y%m%d-%H%M%S')}-were-lines"
        shutil.copy2(a.db, dst)
        print("backup:", dst)
    db = sqlite3.connect(a.db if writing else f"file:{a.db}?mode=ro", isolation_level=None, uri=not writing)
    db.execute("PRAGMA busy_timeout = 10000")
    if a.undo:
        if not has_table(db, LOG):
            print("nothing logged: nothing to undo")
            return
        db.execute("BEGIN IMMEDIATE")
        back, kept = undo(db)
        db.execute("COMMIT")
        print(f"put back {back} values" + (f"; left {len(kept)} changed since: {', '.join(kept)}" if kept else ""))
        print("restart the server for the world to have them")
        return
    if has_table(db, LOG):
        raise SystemExit(f"{LOG} is in this database: applied already (--undo first)")
    gator, left = gator_changes(db)
    wolf = wolfwere_changes(db)
    for tbl, rid, col, old, new, why in gator + wolf:
        print(f"{tbl} {rid} {col}: {short(old)} -> {short(new)}")
    for rid, name in left:
        print(f"npcs {rid} {name!r}: on the alligator's body but no seed's row: left (a GM's)")
    rows = len({rid for _, rid, *_ in gator})
    print(f"{rows} gatorweres given the weregator's body ({len(gator)} values), {len(left)} left; "
          f"the Wolfwere form: {len(wolf)} values")
    if not a.apply:
        print("report only: --apply to write")
        return
    db.execute("BEGIN IMMEDIATE")
    n = apply(db, gator + wolf)
    db.execute("COMMIT")
    print(f"wrote {n} values; restart the server for the world to have them")


if __name__ == "__main__":
    main()
