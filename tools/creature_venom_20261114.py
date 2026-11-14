#!/usr/bin/env python3
"""Creatures poison and infect as Prima's bestiary says (petitions 5030, Trulyblind, and 5032, Groceries, Live,
2026-11-14: "Ember Hornets, Ants, Snakes, the Things around the Mummys and even them, they do not poison you"; "the
Grave Rats and Skellys along coast From FP W do not give any poison or disease").

The mechanism is there (a class template's "dot" ability: entity/creaturedot.go, its ticks on a player through
templateDebuffLocked, tested by TestATemplatesDebuffCanPoison), but no ordinary creature had one: every wild kind fights
by "roamer" (tools/seed_roamers.py: SOE's basic creature, a crit every 30 s from level 10) and nothing else, so none
poisoned or infected anyone.

Prima's Frontiers guide (its bestiary, pp. 650-661: ~/.cache/eqoa-tradeskill-pages/prima-frontiers, the table
embedded below as PRIMA) names each kind's special abilities. 107 of its 803 rows poison or infect, in five grades
tied to level: Minor (6-15), Lesser (16-25), plain (24-33), Greater (34-43), Major (44-55) Poison or Infection
(spiders, snakes, scorpions, wasps, millipedes; mummies, zombies, rats, dread wolves). Its ember hornets, fire ants,
scarabs and sidewinders have "Critical Hits, Rampage" and no poison: not given one here (said in the report).

  classtemplates "roamer <grade> poison" and "roamer <grade> infection" (ten, new): the roamer row as it is, plus a
    damage over time every 24 s from 4 s into a fight, kept up (creaturedot.go):
      poison:    Toxic Bolt (700824, PR, 114 a tick, 24 s) at 8/13/35/55/90% (minor .. major: 9, 15, 40, 63, 103 a tick)
      infection: Rabid Infection (886, DR, 10, 24 s) 100%, Plague (700112, DR, 14, 24 s) 100%, Baga Rash (700295,
                 DR, 59, 24 s) 70% and 100%, Innothule Fever (970, DR, 158, 24 s) 70% (10, 14, 41, 59, 111 a tick)
    The figures are ours (Prima names a grade, no figure): about a third of a same-level creature's melee a round.
  npctemplate <name>: each kind of Prima's that poisons or infects whose name (letters only) is one of ours and fights
    by "roamer" now, to its grade's template; a name Prima lists at two grades takes the one its rows' middle level
    falls in; one whose rows stand below every level Prima gives the name its grade at (its level-5 spiders, Prima's
    "Young Spider 4-5 None") is left. A kind on any other template (a tuned one) is left and said.

Templates and npctemplate are read at the server's start, or at once by the GM panel's reload_npcs. Refused, nothing
written, when a template of these names is there not as this tool writes it or a spell is missing. Report only by
default; --apply in one transaction, a gm_audit line a row, log creature_venom_1114_log; --undo exact.

    tools/creature_venom_20261114.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import os
import re
import sqlite3
import sys

LOG = "creature_venom_1114_log"
DETAIL = ("petitions 5030/5032: creatures poison and infect as Prima's bestiary says: tools/creature_venom_20261114.py")
WHO, VIA = "claude", "tool"
SRC = "venom1114"
GRADES = ["minor", "lesser", "plain", "greater", "major"]
SPELLS = {
    "poison": [(700824, 8), (700824, 13), (700824, 35), (700824, 55), (700824, 90)],
    "infection": [(886, 100), (700112, 100), (700295, 70), (700295, 100), (970, 70)],
}
HERE = os.path.dirname(os.path.abspath(__file__))
PRIMA = os.path.join(HERE, "data", "prima_bestiary_venom.json")

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    name   TEXT NOT NULL,
    before TEXT,
    after  TEXT NOT NULL,
    audit  INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False, sort_keys=True, default=str)


def audit(db, action, tbl, key, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, compact(key), compact(before or {}), compact(after or {}), DETAIL))
    return cur.lastrowid


def row(db, tbl, name):
    cur = db.execute(f"SELECT * FROM {tbl} WHERE name = ?", (name,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def key(s):
    return re.sub(r"[^a-z]", "", s.lower())


def tname(kind, grade):
    return f"roamer {grade} {kind}"


def prima():
    """{letters-only name: [(kind, grade, lo, hi)]} of Prima's poisoning and infecting kinds."""
    out = {}
    for name, lvl, special in json.load(open(PRIMA)):
        lo, _, hi = lvl.partition("-")
        lo, hi = int(lo), int(hi or lo)
        for part in special.split(","):
            m = re.match(r"(Minor |Lesser |Greater |Major )?(Poison|Infection)( \d+-\d+)?$", part.strip())
            if m:
                out.setdefault(key(name), []).append((m.group(2).lower(), (m.group(1) or "plain").strip().lower(), lo, hi))
    return out


def plan(db):
    changes, lines, refusals = [], [], []
    roamer = row(db, "classtemplates", "roamer")
    if roamer is None:
        return changes, lines, ["no classtemplates 'roamer' here"]
    for kind, spells in SPELLS.items():
        for g, (sid, pct) in zip(GRADES, spells):
            if db.execute("SELECT 1 FROM spelleffect WHERE spellid = ?", (sid,)).fetchone() is None:
                refusals.append(f"spell {sid} is not here")
                continue
            name = tname(kind, g)
            want = dict(roamer, name=name, source=SRC,
                        abilities=json.dumps([{"kind": "dot", "spell_id": sid, "every": 24, "first": 4, "damage_pct": pct}],
                                             separators=(",", ":")),
                        note=f"Prima's {g} {kind} (its bestiary): the roamer with a damage over time (petitions 5030/5032, "
                             f"tools/creature_venom_20261114.py). The roamer's note: " + (roamer["note"] or ""))
            have = row(db, "classtemplates", name)
            if have is None:
                changes.append(("classtemplates", name, None, want))
            elif have.get("source") != SRC:
                refusals.append(f"classtemplates {name!r} is there, not this tool's")
    if refusals:
        return changes, lines, refusals
    pm = prima()
    ours = db.execute("SELECT name, template FROM npctemplate WHERE name NOT LIKE '#%'").fetchall()
    for name, tmpl in ours:
        entries = pm.get(key(name))
        if not entries:
            continue
        if tmpl != "roamer":
            lines.append(f"left: {name!r} fights by {tmpl!r} (Prima: {entries[0][1]} {entries[0][0]})")
            continue
        lv = sorted(l for (l,) in db.execute("SELECT npc_level FROM npcs WHERE lower(trim(npc_name)) = ?", (name.lower().strip(),)))
        mid = lv[len(lv) // 2] if lv else entries[0][2]
        if mid < min(e[2] for e in entries):
            lines.append(f"left: {name!r} (level {mid}): below Prima's poisoning or infecting ones of the name (level "
                         f"{min(e[2] for e in entries)}), whose younger ones have none")
            continue
        best = min(entries, key=lambda e: 0 if e[2] <= mid <= e[3] else min(abs(mid - e[2]), abs(mid - e[3])))
        new = tname(best[0], best[1])
        changes.append(("npctemplate", name, {"template": tmpl}, {"template": new}))
        lines.append(f"{name!r} ({len(lv)} rows, level {mid}): {new}")
    return changes, lines, refusals


def apply(db, changes):
    db.execute(LOG_SCHEMA)
    for tbl, name, before, after in changes:
        if before is None:
            cols = list(after)
            db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [after[c] for c in cols])
            a = audit(db, "insert", tbl, {"name": name}, None, after)
        else:
            if db.execute(f"UPDATE {tbl} SET template = ? WHERE name = ? AND template = ?",
                          (after["template"], name, before["template"])).rowcount != 1:
                raise RuntimeError(f"{tbl} {name} changed under the update")
            a = audit(db, "update", tbl, {"name": name}, before, after)
        db.execute(f"INSERT INTO {LOG} (tbl, name, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, name, None if before is None else compact(before), compact(after), a))


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, tbl, name, before, after in db.execute(f"SELECT seq, tbl, name, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        after = json.loads(after)
        now = row(db, tbl, name)
        if now is None or any(now.get(c) != v for c, v in after.items()):
            print(f"  left: {tbl} {name!r} changed since")
            left += 1
            continue
        if before is None:
            db.execute(f"DELETE FROM {tbl} WHERE name = ?", (name,))
            audit(db, "delete", tbl, {"name": name}, now, None)
        else:
            before = json.loads(before)
            db.execute(f"UPDATE {tbl} SET template = ? WHERE name = ?", (before["template"], name))
            audit(db, "update", tbl, {"name": name}, after, before)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if left == 0:
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
        db.execute("BEGIN IMMEDIATE")
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"put back {back}, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if table(db, LOG):
            db.execute("ROLLBACK")
            print(f"applied already ({LOG} is there): --undo first")
            return 0
        changes, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            db.execute("ROLLBACK")
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not changes:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"report only: {len(changes)} rows would be written (--apply writes them)")
            return 0
        apply(db, changes)
        db.execute("COMMIT")
        print(f"applied: {len(changes)} rows, log {LOG}")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
