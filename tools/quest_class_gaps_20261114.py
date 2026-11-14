#!/usr/bin/env python3
"""The quests' class gaps against their EQOA-Data guides, filled (the owner, 2026-11-14: "yes find the gaps").

The plan is tools/quest_class_gaps_20261114.json, written by tools/quest_class_audit_20261114.py from the guides
(external/EQOA-Data/Quests) and the import's notes; its report says what each gap is. This tool needs no guide: it
resolves the plan against the database it is given, so each shard gets its own item ids.

  the gate     a quest whose guide's "Class:" line allows a class its quests.classes leaves out gets the class added,
               once the class has a reward on each of the quest's class-reward steps (Grobb's Froglok Liver: the bard
               and the alchemist; the import had limited it to the classes its guide lists rewards for).
  guide-named  a class the guide names a reward for that its step (queststeps.byclass) gives nothing gets the guide's
               item: itempattern by name (case and apostrophes aside, a plural's s), its class mask letting the class
               wear it, its level within 3 of the quest's. When no item is that one, the step being one that gives
               each class a piece of its level's class set, the class gets an unused piece of its own set instead.
  guide-silent a class the guide allows and names no reward for, on a step that gives other classes pieces of their
               level's class sets, gets a piece of its own set: the class-only wearable items of the quest's level
               whose name carries the class's set word (Chromatic, of the Disciple, of Malice, Blue Aura, ...), one no
               quest of the same level gives the class already (two records of one name and slot are one piece), the
               lowest id first. A class whose set has none left
               is reported and left.
  Rewards a step gives already are never changed, the guide's or not. A step whose classes' rewards are shared
  (archetype) items or abilities is not a set step: its gaps are reported, not filled.

Characters who finished a quest before are not given anything (listed by class). The quests load at the server's
start: run it in a keep restart's before.sh. Report only by default; --apply in one transaction, a gm_audit line a row,
log quest_class_gaps_1114_log; --undo exact (a row changed since is left and said).

    tools/quest_class_gaps_20261114.py --db eqoa.db [--plan PLAN] [--apply | --undo]
"""
import argparse
import datetime
import json
import os
import re
import sqlite3
import sys

LOG = "quest_class_gaps_1114_log"
DETAIL = "quest class gaps against the EQOA-Data guides (owner 2026-11-14): tools/quest_class_gaps_20261114.py"
WHO, VIA = "claude", "tool"
PLAN = os.path.join(os.path.dirname(os.path.abspath(__file__)), "quest_class_gaps_20261114.json")
NAMES = ["WAR", "RAN", "PAL", "SK", "MNK", "BRD", "RGE", "DRD", "SHA", "CLR", "MAG", "NEC", "ENC", "WIZ", "ALC"]
LEVEL_SLACK = 3
SLOT_WORDS = {"of", "the", "a", "and", "tunic", "robe", "helm", "gloves", "boots", "bracers", "bracer", "belt", "sash",
              "leggings", "greaves", "legplates", "pendant", "amulet", "ring", "dagger", "blade", "sword", "wand",
              "shield", "tome", "circlet", "slippers", "insignia", "knuckles", "fist", "chestguard", "tabbard",
              "tabard", "spear", "crook", "ahlspiess", "axe", "foil", "gauntlets", "coif", "crown", "tunic", "pants",
              "bracelet", "earring", "skullcap", "turban", "legwraps", "shamshir", "breastplate", "staff", "boots"}
NOTE = " Class gaps filled 2026-11-14 against the guide (tools/quest_class_gaps_20261114.py): {}."

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rid    TEXT NOT NULL,
    col    TEXT NOT NULL,
    old    TEXT,
    new    TEXT,
    audit  INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False, sort_keys=True)


def fold(s):
    s = str(s or "").lower().replace("’", "'").replace("`", "'").replace("'", "")
    return " ".join(re.sub(r"[^\w\s-]", " ", s).split())


def ids(s):
    return [int(x) for x in str(s or "").split(",") if x.strip().isdigit()]


def audit(db, action, tbl, key, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, compact(key), compact(before or {}),
                      compact(after or {}), DETAIL))
    return cur.lastrowid


class Items:
    def __init__(self, db):
        self.rows = {}
        self.by_name = {}
        for pid, name, cu, lvl, slot in db.execute(
                "SELECT patternid, itemname, classuse, levelreq, equipslot FROM itempattern ORDER BY patternid"):
            self.rows[pid] = (name, cu, lvl, slot)
            self.by_name.setdefault(fold(name), []).append(pid)
        self._sets = {}

    def named(self, name, c, level):
        """The item of the guide's name the class may wear, of the quest's level (or None, why)."""
        k = fold(name)
        cands = self.by_name.get(k) or self.by_name.get(k[:-1] if k.endswith("s") else k + "s") or []
        ok = [p for p in cands if (self.rows[p][1] >> c) & 1 and abs(self.rows[p][2] - level) <= LEVEL_SLACK]
        if ok:
            return ok[0], "the guide's"
        if cands:
            n, cu, lvl, _ = self.rows[cands[0]]
            return None, f"only {n!r} ({cands[0]}: level {lvl}, classuse {cu})"
        return None, "no item of the name"

    def piece(self, pid):
        """An item as a piece: its slot and its name's words less "of"/"the" ("Ring of Shadow" and "Ring of the
        Shadow", two records of one ring, are one piece)."""
        r = self.rows.get(pid)
        if r is None:
            return (None, pid)
        return (r[3], tuple(sorted(set(fold(r[0]).split()) - {"of", "the", "a"})))

    def own(self, pid, c, level):
        """A piece of a class's own set of that level."""
        r = self.rows.get(pid)
        return r is not None and r[1] == 1 << c and r[2] == level and r[3] >= 0

    def set_of(self, c, level):
        """[patternid] of the class's set of a level: its class-only wearable items there carrying its set word."""
        if (c, level) in self._sets:
            return self._sets[(c, level)]
        cands = [p for p, (n, cu, lvl, slot) in self.rows.items()
                 if cu == 1 << c and lvl == level and slot >= 0 and re.search(r"[A-Z]", n)]
        words = {}
        for p in cands:
            for w in set(fold(self.rows[p][0]).split()) - SLOT_WORDS:
                words[w] = words.get(w, 0) + 1
        out = cands
        if words:
            top = max(words.items(), key=lambda kv: (kv[1], kv[0]))
            if top[1] >= 2:
                out = [p for p in cands if top[0] in fold(self.rows[p][0]).split()]
        self._sets[(c, level)] = sorted(out)
        return self._sets[(c, level)]


def given_at_level(db, level):
    """{class: {patternid}} the quests of a level give each class (their steps' byclass items and rewarditems)."""
    out = {c: set() for c in range(15)}
    for bc, ri in db.execute("SELECT s.byclass, s.rewarditems FROM queststeps s JOIN quests q USING (questid) "
                             "WHERE q.minlevel = ?", (level,)):
        common = set(ids(ri))
        for c in range(15):
            out[c] |= common
        if bc:
            for k, e in json.loads(bc).items():
                if str(k).isdigit() and isinstance(e, dict):
                    out[int(k)] |= set(ids(e.get("items")))
    return out


def has_reward(entry):
    return bool(str((entry or {}).get("items") or "").strip() or str((entry or {}).get("spells") or "").strip())


def plan(db, quests):
    """(changes, lines, refusals) for the plan's quests against this database."""
    changes, lines, refusals = [], [], []
    items = Items(db)
    used = {}
    for pq in sorted(quests, key=lambda x: x["questid"]):
        qid = pq["questid"]
        row = db.execute("SELECT name, classes, minlevel, notes FROM quests WHERE questid = ?", (qid,)).fetchone()
        if row is None:
            refusals.append(f"quest {qid} ({pq['name']}) is not here")
            continue
        name, classes, level, notes = row
        if name != pq["name"]:
            refusals.append(f"quest {qid} is {name!r}, not {pq['name']!r}: look first")
            continue
        if level not in used:
            used[level] = given_at_level(db, level)
        said = []
        new_bc = {}
        for ps in pq["steps"]:
            st = db.execute("SELECT byclass FROM queststeps WHERE questid = ? AND step = ?", (qid, ps["step"])).fetchone()
            if st is None or not st[0]:
                lines.append(f"{qid} {name} step {ps['step']}: no class rewards on this shard (its data differ): skipped")
                continue
            old = st[0]
            bc = json.loads(old)
            set_step = any(items.own(p, int(k), level) for k, e in bc.items() if str(k).isdigit()
                           for p in ids((e or {}).get("items")))
            for cs, f in sorted(ps["fill"].items(), key=lambda kv: int(kv[0])):
                c = int(cs)
                if has_reward(bc.get(cs)):
                    continue
                pick, why = None, None
                for n in f.get("guide") or []:
                    pick, why = items.named(n, c, level)
                    if pick:
                        why = f"the guide's {n}"
                        break
                if pick is None:
                    if not set_step:
                        lines.append(f"{qid} {name} step {ps['step']}, {NAMES[c]}: left (" +
                                     (f"the guide's {', '.join(f['guide'])}: {why}; " if f.get("guide") else "") +
                                     "the step gives no class-set pieces to fill from)")
                        continue
                    held = {items.piece(p) for p in used[level][c]}
                    free = [p for p in items.set_of(c, level) if items.piece(p) not in held]
                    if not free:
                        lines.append(f"{qid} {name} step {ps['step']}, {NAMES[c]}: left (no piece of its level {level} "
                                     f"set the level's quests do not give it already)")
                        continue
                    pick = free[0]
                    why = (f"its set's piece for the guide's {', '.join(f['guide'])} ({why})" if f.get("guide")
                           else "its set's piece (the guide names none)")
                used[level][c].add(pick)
                e = dict(bc.get(cs) or {})
                e["items"] = str(pick)
                bc[cs] = e
                lines.append(f"{qid} {name} step {ps['step']}, {NAMES[c]}: {items.rows[pick][0]} ({pick}), {why}")
                said.append(f"{NAMES[c]} {items.rows[pick][0]}")
            new = compact(bc)
            if new != compact(json.loads(old)):
                changes.append(("queststeps", f"{qid},{ps['step']}", "byclass", old, new))
                new_bc[ps["step"]] = bc
        # the gate, once each class-reward step gives the class something
        if pq["widen"] and classes.strip():
            have = set(ids(classes))
            add = []
            for c in pq["widen"]:
                if c in have:
                    continue
                ok = True
                for s, bcs in db.execute("SELECT step, byclass FROM queststeps WHERE questid = ? AND byclass != ''", (qid,)):
                    bc = new_bc.get(s) or json.loads(bcs)
                    if not has_reward(bc.get(str(c))):
                        ok = False
                if ok:
                    add.append(c)
                else:
                    lines.append(f"{qid} {name}: gate not widened to {NAMES[c]} (no reward for it on a class-reward step)")
            if add:
                ncl = ", ".join(str(c) for c in sorted(have | set(add)))
                changes.append(("quests", str(qid), "classes", classes, ncl))
                lines.append(f"{qid} {name}: gate widened to {', '.join(NAMES[c] for c in add)} ({classes!r} -> {ncl!r})")
                said.append("open to " + ", ".join(NAMES[c] for c in add))
        if said:
            note = NOTE.format("; ".join(said))
            if note.strip() not in notes:
                changes.append(("quests", str(qid), "notes", notes, notes + note))
            fin = db.execute("SELECT h.tclass, count(*) FROM charquests c JOIN characters h ON h.serverid = c.serverid "
                             "WHERE c.questid = ? AND c.done >= 1 GROUP BY h.tclass", (qid,)).fetchall()
            got = {NAMES[k]: n for k, n in fin if any(s.startswith(NAMES[k] + " ") for s in said)}
            if got:
                lines.append(f"{qid} {name}: finished before by {got} (not given: a GM may)")
    return changes, lines, refusals


def apply(db, changes):
    db.execute(LOG_SCHEMA)
    for tbl, rid, col, old, new in changes:
        if tbl == "queststeps":
            q, s = (int(x) for x in rid.split(","))
            if db.execute(f"UPDATE queststeps SET {col} = ? WHERE questid = ? AND step = ? AND {col} = ?",
                          (new, q, s, old)).rowcount != 1:
                raise RuntimeError(f"queststeps {rid} changed under the update")
            key = {"questid": q, "step": s}
        else:
            if db.execute(f"UPDATE quests SET {col} = ? WHERE questid = ? AND {col} = ?", (new, int(rid), old)).rowcount != 1:
                raise RuntimeError(f"quests {rid} changed under the update")
            key = {"questid": int(rid)}
        a = audit(db, "update", tbl, key, {col: old}, {col: new})
        db.execute(f"INSERT INTO {LOG} (tbl, rid, col, old, new, audit) VALUES (?, ?, ?, ?, ?, ?)",
                   (tbl, rid, col, json.dumps(old), json.dumps(new), a))


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, tbl, rid, col, old, new in db.execute(f"SELECT seq, tbl, rid, col, old, new FROM {LOG} ORDER BY seq DESC").fetchall():
        old, new = json.loads(old), json.loads(new)
        if tbl == "queststeps":
            q, s = (int(x) for x in rid.split(","))
            ok = db.execute(f"UPDATE queststeps SET {col} = ? WHERE questid = ? AND step = ? AND {col} = ?",
                            (old, q, s, new)).rowcount == 1
            key = {"questid": q, "step": s}
        else:
            ok = db.execute(f"UPDATE quests SET {col} = ? WHERE questid = ? AND {col} = ?", (old, int(rid), new)).rowcount == 1
            key = {"questid": int(rid)}
        if not ok:
            print(f"  left: {tbl} {rid} {col} changed since")
            left += 1
            continue
        audit(db, "update", tbl, key, {col: new}, {col: old})
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if left == 0:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    ap.add_argument("--plan", default=PLAN)
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
        with open(a.plan) as f:
            quests = json.load(f)["quests"]
        changes, lines, refusals = plan(db, quests)
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
        if table(db, LOG):
            db.execute("ROLLBACK")
            print(f"applied already ({LOG} is there) and {len(changes)} rows differ since: --undo first")
            return 1
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
