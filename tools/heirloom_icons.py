#!/usr/bin/env python3
"""The heirloom weapons' icons told apart (the user, 2026-11-06: "the new heirloom weapons have all the same icon for
some"). Many level 60 epics share one icon (the bard's blades, the ranger's axes and blades, the monk's claws, the
knight's sword, the druid's Cosmos pair: the epics' blade, 445518418), and so did their heirlooms (tools/heirloom_epics.py
copies an epic's look): 34 records of 14 names on one icon, 22 staves of 6 on another.

Each icon shared by heirloom weapons of different names stays with one of them (KEEP: the weapon it belongs to most, the
user's own picks kept: the Scimitar of Fury's green blades, the glowing hammer's three), and each other name takes the
icon of real weapons of its own kind (an axe's for an axe, a claw's for claws; its kind by its epic's name, else its
own, else its attack type), the highest-level one of that kind no heirloom wears, each name its own. Every record of a
name (each class's copy) takes the same icon. The models are not changed. Data only: a client sees an icon at its next
login. Logged in heirloom_icons_log; --undo puts each icon back.

    tools/heirloom_icons.py [--db eqoa.db] [--apply | --undo]
"""

import argparse
import datetime
import os
import re
import sqlite3

LOG = "heirloom_icons_log"
KEEP = {
    445518418: ["Heirloom Sword of the Contender"],
    -597854559: ["Heirloom Staff of the Summoner"],
    -1405600909: ["Heirloom Mystical Hammer"],
    -2055321559: ["Heirloom Staff of Divinity"],
    -439584577: ["Heirloom Ethereal Hammer", "Heirloom Hammer of Absolution", "Heirloom Mace of Repentence"],
    -921006351: ["Heirloom Sword of the Challenger"],
    913980370: ["Heirloom Virtus Ferrum"],
    621242233: ["Heirloom Scimitar of Fury"],
    -899623943: ["Heirloom Oak Staff of Eilerawdl"],
}
KINDS = [  # first match wins: the narrow kinds before the broad
    ("greatsword", r"\b(greatsword|great sword|claymore|beheader|flamberge|dreams|challenger)\b"),
    ("scimitar", r"\b(scimitar|falchion|shamshir)\b"),
    ("sickle", r"\b(sickle|scythe)\b"),
    ("claws", r"\b(claws?|knuckles?|talons?)\b"),
    ("dagger", r"\b(dagger|dirk|stiletto|knife|kris|tanto|blacktongue|heartsbane)\b"),
    ("axe", r"\b(axe|hatchet|greataxe|cleaver|foereaver)\b"),
    ("spear", r"\b(spear|pike|lance|trident|halberd|polearm|glaive|naginata|downfall)\b"),
    ("hammer", r"\b(hammer|warhammer|maul|mallet|judgement)\b"),
    ("mace", r"\b(mace|morning ?star|flail|club|cudgel|bludgeon|bedlam)\b"),
    ("staff", r"\b(staff|stave|rod|crosier|wand|scepter|cane|path)\b"),
    ("sword", r"\b(sword|blade|longsword|rapier|sabre|saber|ballad|katana|gladius)\b"),
]
OWN_KIND = {"Heirloom Sunsilver Blade": "staff"}  # a staff's model under a blade's name: its icon the staff's


def kind_of(*names):
    for n in names:
        for k, rx in KINDS:
            if n and re.search(rx, n, re.I):
                return k
    return None


def plan(db):
    if db.execute(f"SELECT COUNT(*) FROM {LOG}").fetchone()[0]:
        raise SystemExit("the icons are changed in this database already: --undo first")
    rows = db.execute("""SELECT p.patternid, p.itemname, p.itemicon, COALESCE(e.itemname, '') FROM heirlooms h
        JOIN itempattern p ON p.patternid = h.patternid LEFT JOIN itempattern e ON e.patternid = h.epic
        WHERE h.role IN ('main', 'twohand', 'ranged')""").fetchall()
    by_icon = {}
    for pid, name, icon, epic in rows:
        by_icon.setdefault(icon, {}).setdefault(name, []).append((pid, epic))
    used = {icon for icon in by_icon}
    heir = {r[0] for r in db.execute("SELECT patternid FROM heirlooms")}
    cand = {}
    for pid, name, icon, lv in db.execute("SELECT patternid, itemname, itemicon, levelreq FROM itempattern "
                                          "WHERE equipslot IN (12, 13, 14, 15) AND weapondamage > 0"):
        if pid in heir:
            continue
        k = kind_of(name)
        if k:
            e = cand.setdefault(k, {}).setdefault(icon, [0, 0])
            e[0], e[1] = max(e[0], lv or 0), e[1] + 1
    ranked = {k: [i for i, _ in sorted(v.items(), key=lambda kv: (-kv[1][0], -kv[1][1], kv[0]))] for k, v in cand.items()}
    changes = []
    for icon, names in sorted(by_icon.items()):
        if len(names) < 2:
            continue
        keep = KEEP.get(icon, [])
        if not keep:
            raise SystemExit(f"icon {icon} is shared by {sorted(names)} with no keeper named")
        for name in sorted(names):
            if name in keep:
                continue
            epic = names[name][0][1]
            k = OWN_KIND.get(name) or kind_of(epic, name)
            free = [i for i in ranked.get(k, []) if i not in used]
            if not free:
                raise SystemExit(f"no free {k} icon for {name}")
            new = free[0]
            used.add(new)
            for pid, _ in names[name]:
                changes.append((pid, name, icon, new, k))
    return changes


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    if not os.path.isfile(a.db):
        raise SystemExit(f"no database at {a.db}")
    db = sqlite3.connect(a.db, isolation_level=None, timeout=10)
    db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (patternid INTEGER PRIMARY KEY, old INTEGER NOT NULL, run TEXT NOT NULL)")
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        n = db.execute(f"UPDATE itempattern SET itemicon = (SELECT old FROM {LOG} l WHERE l.patternid = itempattern.patternid) "
                       f"WHERE patternid IN (SELECT patternid FROM {LOG})").rowcount
        db.execute(f"DELETE FROM {LOG}")
        db.execute("COMMIT")
        print(f"put back {n} icons")
        return
    changes = plan(db)
    seen = set()
    for pid, name, old, new, k in changes:
        if name not in seen:
            seen.add(name)
            print(f"  {name}: {old} -> {new} (a {k}'s)")
    print(f"{len(changes)} records of {len(seen)} names")
    if not a.apply:
        return
    run = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
    db.execute("BEGIN IMMEDIATE")
    try:
        for pid, _, old, new, _ in changes:
            db.execute(f"INSERT INTO {LOG} (patternid, old, run) VALUES (?, ?, ?)", (pid, old, run))
            db.execute("UPDATE itempattern SET itemicon = ? WHERE patternid = ?", (new, pid))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"wrote it (run {run}): a client sees the icons at its next login")


if __name__ == "__main__":
    main()
