#!/usr/bin/env python3
"""The quest reward items with no class mask, given the class mask of the class quests that reward them
(the user, 2026-10-30: "Yes, the quest's class").

The old dump's records carry no class mask (itempattern.classuse 0) on 2,189 wearable items. The
server lets any class wear one (handlers/equip.go cannotWear: a mask of 0 is not checked) and CLIENT
reads 0 as no class (its CanUse marks the item unusable, though it sends the equip all the same). 41
of them are quest rewards, 26 in the city class quests: Freeport's warrior level-10 Sturdy Two-Handed
Sword, Qeynos's wizard level-7 Arcane Apprentice's Boots, Halas's Warrior's Tunic.

An item counts when it is wearable (a wear slot, not -1 or 0), its mask is 0, and a quest step gives
it as its reward (the 41 the user was asked about: 32 items). With --all-rewards, also one only a
step's rewards by class, a choice option or a give event gives (155 more, most of them the 23-47 ability
quests' class items; asked separately). Its new mask is every class of all the quests that give it, by
any of those (a quest's classes column; a reward by class its class), so no class a quest gives it to is
left out. One a quest for every class gives (no classes, or all fifteen) is reported and left. Its other uses are listed beside it: merchants and corpses (npcitem),
loot tables, start items, default inventories, recipes, auction lots, gear sets, and the characters who
hold it; a holder of another class keeps what it holds, but may not put it on afresh.

Every change is logged in class_mask_fix_log; --undo puts back each mask still as this left it. One
transaction. A report by default (--tsv FILE the list). No restart: the server reads an item record at
each use (cannotWear), so the masks hold at once; a client reads item records at its login, so a player
sees the new mask (the item no longer marked unusable for its class) at the next one.

    tools/fix_reward_class_masks.py [--db eqoa.db] [--apply | --undo] [--all-rewards] [--tsv FILE]
"""

import argparse
import collections
import csv
import json
import re
import sqlite3
import sys

CLASSES = ["warrior", "ranger", "paladin", "shadowknight", "monk", "bard", "rogue", "druid", "shaman",
           "cleric", "magician", "necromancer", "enchanter", "wizard", "alchemist"]
ABBR = ["WAR", "RAN", "PAL", "SK", "MNK", "BRD", "RGE", "DRD", "SHA", "CL", "MAG", "NEC", "ENC", "WIZ", "ALC"]
EVERY = set(range(15))

LOG = "class_mask_fix_log"
LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    rid INTEGER NOT NULL,
    old INTEGER NOT NULL,
    new INTEGER NOT NULL
)"""


def ids(s):
    """A rewards column's ids ("8554, 706000*2")."""
    return [int(m.group(1)) for m in re.finditer(r"(?<![\d*])(\d+)(?:\*\d+)?", str(s or ""))]


def class_list(s):
    """A quests.classes value as class ids; none means every class."""
    out = {int(x) for x in str(s or "").replace(" ", "").split(",") if x.isdigit() and int(x) < 15}
    return out or set(EVERY)


def has_table(db, t):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone()[0] > 0


def gives(db):
    """item -> [(questid, classes, where)]: every quest reward of every item."""
    keys = dict(db.execute("SELECT questid, key FROM quest_import_quests")) if has_table(db, "quest_import_quests") else {}
    quests = {qid: (class_list(cl), en, name, keys.get(qid, "")) for qid, cl, en, name in
              db.execute("SELECT questid, classes, enabled, name FROM quests")}
    out = collections.defaultdict(list)

    def events(evs, qid, classes, where):
        try:
            lst = json.loads(evs) if evs else []
        except ValueError:
            return
        for e in lst if isinstance(lst, list) else []:
            if isinstance(e, dict) and e.get("do") == "give":
                for p in ids(e.get("items")):
                    out[p].append((qid, classes, f"{where} give event"))

    for qid, step, ri, bc, ch, ev in db.execute(
            "SELECT questid, step, rewarditems, byclass, choices, events FROM queststeps ORDER BY questid, step"):
        if qid not in quests:
            continue
        classes = quests[qid][0]
        for p in ids(ri):
            out[p].append((qid, classes, f"step {step}"))
        for k, e in (json.loads(bc) if bc else {}).items():
            for p in ids((e or {}).get("items")):
                out[p].append((qid, {int(k)}, f"step {step}, by class"))
        for o in (json.loads(ch).get("options") or []) if ch else []:
            for p in ids(o.get("items")):
                out[p].append((qid, classes, f"step {step}, a choice"))
        events(ev, qid, classes, f"step {step}")
    for qid, ev in db.execute("SELECT questid, events FROM quests"):
        events(ev, qid, quests[qid][0], "quest")
    return out, quests


OTHER_USES = [  # (what, query giving one row per use)
    ("sold by or on the corpse of", "SELECT c.npc_name FROM npcitem n JOIN npcs c ON c.id = n.npcid WHERE n.patternid = ?"),
    ("in loot table", "SELECT tablename FROM loottables WHERE patternid = ?"),
    ("a start item of", "SELECT 'race ' || race || ' class ' || class FROM startitems WHERE patternid = ?"),
    ("in the default inventory of class", "SELECT tclass FROM defaultcharinventory WHERE patternid = ?"),
    ("made by recipe", "SELECT name FROM craftrecipes WHERE product = ?"),
    ("in auction lot", "SELECT lot FROM auction_lots WHERE patternid = ?"),
    ("in a gear set of character", "SELECT serverid FROM gearsets WHERE mainpattern = ? OR offpattern = ?"),
]


def other_uses(db, pid):
    out = []
    for what, sql in OTHER_USES:
        table = re.search(r"FROM (\w+)", sql).group(1)
        if not has_table(db, table):
            continue  # a database without it (a test's)
        rows = db.execute(sql, (pid,) * sql.count("?")).fetchall()
        if rows:
            names = sorted({str(r[0]) for r in rows})
            out.append(f"{what} {', '.join(names[:6])}{' and ' + str(len(names) - 6) + ' more' if len(names) > 6 else ''}")
    return out


def step_reward(where):
    """A source that is a step's own reward items (the scope the user approved first)."""
    return re.fullmatch(r"step \d+", where) is not None


def plan(db, all_rewards=False):
    """(changes, left, outside): changes [(item, new mask, sources, uses, holders of other classes)],
    left the items reported and not changed, with why; outside the items only a reward by class, a
    choice or a give event gives (changed with all_rewards). An item's mask counts every quest that
    gives it, in either scope, so no class a quest gives it to is left out."""
    rewards, quests = gives(db)
    changes, left, outside = [], [], []
    for pid in sorted(rewards):
        r = db.execute("SELECT itemname, equipslot, levelreq, classuse FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
        if r is None:
            continue
        name, slot, level, mask = r
        if mask != 0 or slot in (-1, 0, None):
            continue
        srcs = rewards[pid]
        if not all_rewards and not any(step_reward(w) for _, _, w in srcs):
            outside.append(pid)
            continue
        classes = set()
        every = []
        for qid, cl, where in srcs:
            if cl >= EVERY:
                every.append(qid)
            classes |= cl
        uses = other_uses(db, pid)
        src_text = "; ".join(f"{quests[q][3] or q}#{q}{'' if quests[q][1] else ' (a draft)'} ({where}: "
                             f"{'every class' if cl >= EVERY else '/'.join(ABBR[c] for c in sorted(cl))})" for q, cl, where in srcs)
        item = dict(pid=pid, name=name, slot=slot, level=level, sources=src_text, uses=uses)
        if every:
            left.append(dict(item, why=f"a quest for every class gives it ({', '.join(map(str, sorted(set(every))))})"))
            continue
        new = sum(1 << c for c in classes)
        holders = db.execute("SELECT h.tclass, count(*) FROM charinventory i JOIN characters h ON h.serverid = i.serverid "
                             "WHERE i.patternid = ? GROUP BY h.tclass", (pid,)).fetchall()
        others = {CLASSES[c] if 0 <= c < 15 else str(c): n for c, n in holders if not new >> c & 1}
        changes.append(dict(item, new=new, holders_other=others))
    return changes, left, outside


def apply(db, changes):
    db.execute(LOG_DDL)
    n = 0
    for c in changes:
        cur = db.execute("UPDATE itempattern SET classuse = ? WHERE patternid = ? AND classuse = 0", (c["new"], c["pid"]))
        if cur.rowcount:
            db.execute(f"INSERT INTO {LOG} (rid, old, new) VALUES (?, 0, ?)", (c["pid"], c["new"]))
            n += 1
    return n


def undo(db):
    if not has_table(db, LOG):
        print("nothing logged: nothing to undo")
        return
    back = kept = 0
    for rid, old, new in db.execute(f"SELECT rid, old, new FROM {LOG} ORDER BY seq DESC").fetchall():
        cur = db.execute("UPDATE itempattern SET classuse = ? WHERE patternid = ? AND classuse = ?", (old, rid, new))
        back, kept = (back + 1, kept) if cur.rowcount else (back, kept + 1)
    db.execute(f"DROP TABLE {LOG}")
    print(f"put back {back}; left {kept} changed since")


def mask_names(mask):
    return "/".join(ABBR[c] for c in range(15) if mask >> c & 1)


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default="eqoa.db")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    ap.add_argument("--tsv", help="write the list here")
    ap.add_argument("--all-rewards", action="store_true",
                    help="also the items only a reward by class, a choice or a give event gives")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 10000")
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        try:
            undo(db)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        return
    if a.apply and has_table(db, LOG) and db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]:
        sys.exit(f"{LOG} holds an earlier apply: --undo it first")
    changes, left, outside = plan(db, all_rewards=a.all_rewards)
    for c in changes:
        print(f"{c['pid']} {c['name']!r} (slot {c['slot']}, level {c['level']}): classes {mask_names(c['new'])} ({c['new']})"
              f" | given by {c['sources']}" + (f" | also {'; '.join(c['uses'])}" if c["uses"] else "")
              + (f" | HELD by {c['holders_other']} of another class" if c["holders_other"] else ""))
    for x in left:
        print(f"LEFT {x['pid']} {x['name']!r}: {x['why']} | given by {x['sources']}"
              + (f" | also {'; '.join(x['uses'])}" if x["uses"] else ""))
    print(f"{len(changes)} items to give a class mask, {len(left)} left"
          + (f"; {len(outside)} more only a reward by class, a choice or a give event gives (--all-rewards)" if outside else ""))
    if a.tsv:
        with open(a.tsv, "w", newline="") as out:
            w = csv.writer(out, delimiter="\t", lineterminator="\n")
            w.writerow(["patternid", "item", "slot", "level", "new classes", "new mask", "given by", "other uses",
                        "held by another class", "left because"])
            for c in changes:
                w.writerow([c["pid"], c["name"], c["slot"], c["level"], mask_names(c["new"]), c["new"], c["sources"],
                            "; ".join(c["uses"]), ", ".join(f"{k} {v}" for k, v in c["holders_other"].items()), ""])
            for x in left:
                w.writerow([x["pid"], x["name"], x["slot"], x["level"], "", "", x["sources"], "; ".join(x["uses"]), "", x["why"]])
    if not a.apply:
        print("report only: --apply to write")
        return
    db.execute("BEGIN IMMEDIATE")
    try:
        n = apply(db, changes)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"gave {n} items their quests' class mask; no restart (clients see it at their next login)")


if __name__ == "__main__":
    main()
