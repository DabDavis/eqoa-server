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

And the user's choices of the same day, after the check (asked, each answered):
- Zaren, the wolf line's named kill, at level 35, not 44: the lines open at 30 ("Zaren to 35, keep
  the 43s"; in the live game he scaled to whoever called him).
- The six line givers given the NPC bit (npc_type | 0x80: talked to, never attacked): Levandius,
  Dorandas, Edward Duskrender, Corindas, Yindrius and The Bone Crusher ("Yes, unattackable").
- A kill of a line's were raises its society by 10 ("Add +10 a kill"; the live game's kills raised
  it, by no figure on record): factionhits rows, source were-kills, for the tail weres and the
  other spelling of each (werewolf, werebear).
- The wolf and bear lines' tails drop from the other spelling too ("The other spelling too": the
  same beasts): the p2 loot step takes werewolf, the bear's werebear (queststeptargets rows; the
  step's own target stays).

Every value changed is logged in were_lines_fix_log (table, row, column, old, new; a row added is
logged with its column "*insert*" and its values); --undo puts back each one still as this left it
(one changed since is left and said), takes out each row it added and drops the log; a second
--apply is refused while the log is there. Report only by default. The server reads npcs, quests
and factionhits at its start: apply with it stopped (a swap's before.sh), or restart after; a
spell's effects are read at each cast. tools/import_quests.py, re-run over a quest these rows
were added to, deletes them (it rewrites the quest): run this after it.

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

ZAREN_LEVEL = 35         # the user's choice: the line opens at 30
NPC_BIT = 0x80           # npc_type's low byte (CLIENT's flags): talked to, never attacked
GIVERS = ["Levandius", "Dorandas", "Edward Duskrender", "Corindas", "Yindrius", "The Bone Crusher"]
KILL_DELTA = 10          # the user's choice: a kill of a line's were, its society +10
KILL_SOURCE = "were-kills"
KILL_HITS = {            # a were (npcs' name, any case) -> its line's society (the p2 hand-in's faction)
    "wolfwere": "Night Reapers", "werewolf": "Night Reapers",
    "bearwere": "Wards of Dawn", "werebear": "Wards of Dawn",
    "wererat": "Topi Di Ombra",
    "lionwere": "Sunmanes",
    "gatorwere": "The Bone Crusher",
}
TAIL_ALSO = {            # a tail quest's import key -> (its loot step's creature, the other spelling it takes too)
    "Weres/wolfwere/p2": ("wolfwere", "werewolf"),
    "Weres/bearwere/p2": ("bearwere", "werebear"),
}

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


def zaren_changes(db):
    """Zaren at the user's level 35 (one row, as the seed has him)."""
    out = []
    for rid, level in db.execute("SELECT id, npc_level FROM npcs WHERE lower(trim(npc_name)) = 'zaren' ORDER BY id"):
        if level != ZAREN_LEVEL:
            out.append(("npcs", rid, "npc_level", level, ZAREN_LEVEL, "the user's choice: Zaren at 35, the line opens at 30"))
    return out


def giver_changes(db):
    """The line givers with the NPC bit, each row of their names that lacks it."""
    out = []
    for name in GIVERS:
        for rid, t in db.execute("SELECT id, npc_type FROM npcs WHERE lower(trim(npc_name)) = lower(?) ORDER BY id", (name,)):
            if not t & NPC_BIT:
                out.append(("npcs", rid, "npc_type", t, t | NPC_BIT, f"the user's choice: {name}, a line's giver, never attacked"))
    return out


def kill_inserts(db):
    """factionhits rows for the weres' kills, each that no row of its name and faction holds yet:
    (inserts, the names and factions left as a row of theirs has them)."""
    out, left = [], []
    for npc, faction in KILL_HITS.items():
        have = db.execute("SELECT delta, source FROM factionhits WHERE lower(trim(npc_name)) = ? AND faction = ?",
                          (npc, faction)).fetchone()
        if have:
            left.append(f"{npc} / {faction}: {have[0]:+d} ({have[1]})")
            continue
        out.append(("factionhits", {"npc_name": npc, "faction": faction, "delta": KILL_DELTA, "source": KILL_SOURCE},
                    f"the user's choice: a kill of a {npc}, {faction} +{KILL_DELTA}"))
    return out, left


def tail_inserts(db):
    """queststeptargets rows: the tail quests' loot steps take the other spelling."""
    out = []
    for key, (own, also) in TAIL_ALSO.items():
        row = db.execute("SELECT questid FROM quests WHERE source LIKE ?", (f"EQOA-Data {key} %",)).fetchone()
        if row is None:
            continue
        qid = row[0]
        step = db.execute("SELECT step FROM queststeps WHERE questid = ? AND kind = 'loot' AND lower(trim(target)) = ?",
                          (qid, own)).fetchone()
        if step is None:
            continue
        if db.execute("SELECT count(*) FROM queststeptargets WHERE questid = ? AND step = ? AND lower(trim(target)) = ?",
                      (qid, step[0], also)).fetchone()[0]:
            continue
        out.append(("queststeptargets", {"questid": qid, "step": step[0], "target": also},
                    f"the user's choice: {also}, the other spelling, drops the {own} tail too"))
    return out


KEY = {"npcs": "id", "spelleffect": "spellid"}


def apply(db, changes, inserts=()):
    db.execute(LOG_DDL)
    now = time.strftime("%Y-%m-%d %H:%M:%S", time.gmtime())
    n = 0
    for tbl, rid, col, old, new, why in changes:
        cur = db.execute(f"UPDATE {tbl} SET {col} = ? WHERE {KEY[tbl]} = ? AND {col} IS ?", (new, rid, old))
        if cur.rowcount:
            db.execute(f"INSERT INTO {LOG} (tbl, rid, col, old, new, why, applied) VALUES (?, ?, ?, ?, ?, ?, ?)",
                       (tbl, rid, col, json.dumps(old), json.dumps(new), why, now))
            n += 1
    for tbl, vals, why in inserts:
        cols = list(vals)
        cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [vals[c] for c in cols])
        db.execute(f"INSERT INTO {LOG} (tbl, rid, col, old, new, why, applied) VALUES (?, ?, '*insert*', NULL, ?, ?, ?)",
                   (tbl, cur.lastrowid, json.dumps(vals, sort_keys=True), why, now))
        n += 1
    return n


def undo(db):
    back, kept = 0, []
    for seq, tbl, rid, col, old, new in db.execute(f"SELECT seq, tbl, rid, col, old, new FROM {LOG} ORDER BY seq DESC").fetchall():
        if col == "*insert*":
            vals = json.loads(new)
            where = " AND ".join(f"{c} IS ?" for c in vals)
            cur = db.execute(f"DELETE FROM {tbl} WHERE rowid = ? AND {where}", [rid] + [vals[c] for c in vals])
            if cur.rowcount:
                back += 1
            else:
                kept.append(f"{tbl} row {rid} (changed or gone since)")
            continue
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
    zaren = zaren_changes(db)
    givers = giver_changes(db)
    kills, kills_left = kill_inserts(db)
    tails = tail_inserts(db)
    for tbl, rid, col, old, new, why in gator + wolf + zaren + givers:
        print(f"{tbl} {rid} {col}: {short(old)} -> {short(new)}")
    for tbl, vals, why in kills + tails:
        print(f"{tbl} + {vals}")
    for rid, name in left:
        print(f"npcs {rid} {name!r}: on the alligator's body but no seed's row: left (a GM's)")
    for s in kills_left:
        print(f"factionhits {s}: a row of its own is there: left")
    rows = len({rid for _, rid, *_ in gator})
    print(f"{rows} gatorweres given the weregator's body ({len(gator)} values), {len(left)} left; "
          f"the Wolfwere form: {len(wolf)} values; Zaren: {len(zaren)}; givers given the NPC bit: {len(givers)}; "
          f"kill hits: {len(kills)} ({len(kills_left)} left); tail creatures: {len(tails)}")
    if not a.apply:
        print("report only: --apply to write")
        return
    db.execute("BEGIN IMMEDIATE")
    n = apply(db, gator + wolf + zaren + givers, kills + tails)
    db.execute("COMMIT")
    print(f"wrote {n} values; restart the server for the world to have them")


if __name__ == "__main__":
    main()
