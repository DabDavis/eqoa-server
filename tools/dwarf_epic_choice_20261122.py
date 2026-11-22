#!/usr/bin/env python3
"""Dwarves choose which level 60 epic to run, the good side's or the evil side's, for their class (the owner,
2026-11-22, from a player's suggestion: a dwarf shadow knight "having the choice of which epic to run would be
interesting").

The level 60 epics are EQOA-Data's "60/<good|evil>-<tank|melee|caster|healer>/" chains (tools/quests/data/60.json):
a chain's "The Recruiter" quests (start-gN, one talk to the chain's giver, each of some classes and races) are what
its first quest (ep1-1) requires, one of them; its ep quests carry the chain's classes and races; Ediopolus's
"The <Class>'s Epic Reward" (reward-<class>) requires the chain's last quest. An archetype is its classes on either
side: tank WAR PAL SK, melee RAN MNK BRD RGE, healer DRD SHA CL, caster MAG NEC ENC WIZ ALC.

What a dwarf (race 4) may run is read off the recruiters: a class has a side when a recruiter of that side takes the
class and a dwarf. No class has both (refused if one does). The rest goes through ONE new recruiter quest a side,
for dwarves alone (races "4"), so nothing changes for any other race:

recruiter  a side with archetype classes no dwarf can start: a new "The Recruiter" (a copy of the side's first, its
           step and giver), classes those, races 4; ep1-1's requires gains it as one more alternative.
races      race 4 on every ep quest of a chain that lacks it.
classes    a class of the archetype missing from a chain (SK on the good tank chain, PAL on the evil one; RAN, MNK,
           BRD on the evil melee; DRD on the evil healer; NEC on the good caster) is added to its ep quests (only a
           dwarf reaches ep1-1 by it: every other recruiter of the side refuses the class), and its class's reward
           quest, on the other side, requires that chain's last quest as an alternative (no new reward quest: the
           weapon is the class's own; Dabrish's exchanges read the same quest).
exclusion  one epic, never both: a dwarf's new recruiter of a side excludes the OTHER side's ep1-1 (and the other
           side's new recruiter), and that ep1-1 excludes it (quests.excludes: not offered nor taken by one who has
           finished or holds any of them, handlers/quests.go mayTakeAtLocked). A dwarf who took the side it always had
           cannot take the new recruiter; one who took the new recruiter cannot start the side it always had. Only a
           dwarf can finish the new recruiters, so the excludes bind no one else (a human warrior, who can run both
           chains today, still can).

Every quest is found by its source and NAME, never by id, and must be as read when this was made (names, classes,
the disc races besides the dwarf, ep1-1's requires and excludes, the rewards' requires): anything else is refused
whole. New quest ids are the first free from 109800. Gates not changed, reported: factionreq (none on these chains),
and the givers' factions against a dwarf's starting standings (KOS at -350 and below attacks on sight).

Every row written is a gm_audit line (via "tool") and a row of dwarf_epic_choice_20261122_log; --undo puts each back as
it was (a row changed since, or a new recruiter a character holds, is left and said) and drops the log. A report by
default; --apply writes once (a second finds the log and writes nothing); --apply and --undo copy the database first
(<db>.bak-<when>-pre-dwarfepic) unless --no-backup.

WHEN IT TAKES EFFECT: the quests are read again at a GM panel quest edit or reload (handlers.ReloadQuests), else at
the server's next start. No restart is needed.

    python3 -E -s tools/dwarf_epic_choice_20261122.py --db eqoa.db --shard heirloom             report
    python3 -E -s tools/dwarf_epic_choice_20261122.py --db eqoa.db --shard heirloom --apply     write it
    python3 -E -s tools/dwarf_epic_choice_20261122.py --db eqoa.db --shard heirloom --undo      take it back
"""
import argparse
import datetime
import json
import shutil
import sqlite3
import sys

LOG = "dwarf_epic_choice_20261122_log"
WHO, VIA = "claude", "tool"
SOURCE = "ours: tools/dwarf_epic_choice_20261122.py"
DETAIL = ("the owner, 2026-11-22: dwarves choose which level 60 epic to run, the good side's or the evil side's: "
          "tools/dwarf_epic_choice_20261122.py")
DWARF = 4
FIRST_NEW_ID = 109800
FIRST_CUSTOM_RACE = 11  # entity.FirstCustomRace: ids from here are server races, left as they are
KOS_AT = -350           # entity.FactionKOSAt

CLASS_NAMES = ["warrior", "ranger", "paladin", "shadowknight", "monk", "bard", "rogue", "druid", "shaman", "cleric",
               "magician", "necromancer", "enchanter", "wizard", "alchemist"]
ABBR = ["WAR", "RAN", "PAL", "SK", "MNK", "BRD", "RGE", "DRD", "SHA", "CL", "MAG", "NEC", "ENC", "WIZ", "ALC"]
ARCHETYPES = {"tank": (0, 2, 3), "melee": (1, 4, 5, 6), "healer": (7, 8, 9), "caster": (10, 11, 12, 13, 14)}
SIDES = ("good", "evil")

# As read on both shards, 2026-11-22 (Heirloom 05:26 and Live 05:28 UTC pre-swap backups): ep1-1's name, the ep
# quests' classes, their disc races (ids under 11) besides the dwarf, and the name of the last quest the rewards require.
CHAINS = {
    "good-tank": ("Crystals and Warbeads", "0, 2", {0, 1, 3, 6, 7, 8}, "Synder of the Indigo"),
    "evil-tank": ("Wands and Branches", "0, 3", {0, 2, 5, 8, 9}, "Quil'Eyne's Journal"),
    "good-melee": ("Goo and Warbeads", "1, 4, 5, 6", {0, 1, 3, 6, 7}, "The Vampires of Greyvax"),
    "evil-melee": ("Crystals and Branches", "6", {0, 2, 3}, "Ahriaana the Ultimate"),
    "good-healer": ("Wands and Warbeads", "7, 8, 9", {0, 1, 3, 6, 7, 8}, "The Avatar of Hate"),
    "evil-healer": ("Wands and Branches", "8, 9", {0, 2, 5, 9}, "Synder of the Indigo"),
    "good-caster": ("Goo and Warbeads", "10, 12, 13, 14", {0, 1, 3, 8}, "Ahriaana the Ultimate"),
    "evil-caster": ("Crystals and Branches", "10, 11, 12, 13, 14", {0, 2, 3, 8, 9}, "The Avatar of Love"),
}
RECRUITER = "The Recruiter"
LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    ref    TEXT NOT NULL,
    before TEXT,
    after  TEXT,
    audit  INTEGER,
    at     TEXT NOT NULL
)"""
AUDIT_DDL = """CREATE TABLE IF NOT EXISTS gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER,
    username TEXT, via TEXT, ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT)"""
KEYS = {"quests": ("questid",), "queststeps": ("questid", "step"), "queststeptargets": ("questid", "step", "target"),
        "questunlocks": ("questid", "npc", "dest")}


class Refused(Exception):
    pass


def go_json(m):
    """A row as Go's json.Marshal writes a map (keys sorted, <>& escaped), as the panel's audit lines are."""
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    names = [c[0] for c in cur.description]
    return [dict(zip(names, r)) for r in cur.fetchall()]


def has_table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def ints(s):
    """database.ParseIntList: ids separated by commas (spaces ignored)."""
    return [int(x) for x in str(s or "").replace(" ", "").split(",") if x != ""]


def int_list(v):
    return ", ".join(str(x) for x in sorted(set(v)))


def chain_key(source):
    """'EQOA-Data 60/good-tank/ep1-1 (tools/import_quests.py)' -> ('good-tank', 'ep1-1')."""
    parts = source.split(" ", 2)
    if len(parts) < 2 or not parts[1].startswith("60/"):
        return None
    p = parts[1].split("/")
    return (p[1], p[2]) if len(p) == 3 else None


class Plan:
    def __init__(self, db):
        self.db = db
        self.changes = []  # (tbl, ref, before or None, after)
        self.lines = []
        self.gates = []

    # ---- reading ---------------------------------------------------------------------------------------------------

    def read(self):
        db = self.db
        self.chains = {c: {"ep": [], "start": [], "reward": {}} for c in CHAINS}
        for q in rows(db, "SELECT * FROM quests WHERE source LIKE 'EQOA-Data 60/%' ORDER BY questid"):
            k = chain_key(q["source"])
            if not k or k[0] not in CHAINS:
                continue
            ch, part = self.chains[k[0]], k[1]
            if part.startswith("ep"):
                ch["ep"].append(q)
            elif part.startswith("start-"):
                ch["start"].append(q)
            elif part.startswith("reward-"):
                cname = part[len("reward-"):]
                if cname not in CLASS_NAMES:
                    raise Refused(f"{k[0]}: a reward of no class {cname!r}")
                ch["reward"][CLASS_NAMES.index(cname)] = q
        for name, (first, classes, disc, last_name) in CHAINS.items():
            ch = self.chains[name]
            eps = {chain_key(q["source"])[1]: q for q in ch["ep"]}
            if "ep1-1" not in eps or eps["ep1-1"]["name"] != first:
                raise Refused(f"{name}: no ep1-1 named {first!r}")
            ch["first"] = eps["ep1-1"]
            for q in ch["ep"]:
                if q["classes"] != classes:
                    raise Refused(f"{name}: quest {q['questid']} {q['name']!r} classes {q['classes']!r}, read {classes!r}")
                have = {r for r in ints(q["races"]) if r < FIRST_CUSTOM_RACE} - {DWARF}
                if have != disc:
                    raise Refused(f"{name}: quest {q['questid']} {q['name']!r} disc races {sorted(have)}, read {sorted(disc)}")
            if not ch["start"] or any(q["name"] != RECRUITER for q in ch["start"]):
                raise Refused(f"{name}: its start quests are not all {RECRUITER!r}")
            if not ch["reward"]:
                raise Refused(f"{name}: no reward quests")
            req = {q["requires"].strip() for q in ch["reward"].values()}
            if len(req) != 1 or not req.pop().isdigit():
                raise Refused(f"{name}: its rewards do not all require one quest")
            last_id = int(ch["reward"][min(ch["reward"])]["requires"])
            last = next((q for q in ch["ep"] if q["questid"] == last_id), None)
            if last is None or last["name"] != last_name:
                raise Refused(f"{name}: its rewards require {last_id}, not the ep quest {last_name!r}")
            ch["last"] = last
            start_ids = sorted(q["questid"] for q in ch["start"])
            got = sorted(int(x) for x in ch["first"]["requires"].replace(" ", "").split("|") if x)
            if "," in ch["first"]["requires"] or got != start_ids:
                raise Refused(f"{name}: ep1-1 requires {ch['first']['requires']!r}, not one of its recruiters {start_ids}")
            if ch["first"]["excludes"].strip():
                raise Refused(f"{name}: ep1-1 excludes {ch['first']['excludes']!r} already")

    def dwarf_side(self, chain, cls):
        """Whether a recruiter of the chain takes a dwarf of the class (races empty is every race)."""
        for q in self.chains[chain]["start"]:
            cs, rs = ints(q["classes"]), ints(q["races"])
            if (not cs or cls in cs) and (not rs or DWARF in rs):
                return True
        return False

    # ---- planning --------------------------------------------------------------------------------------------------

    def plan(self):
        db = self.db
        self.read()
        taken = {r[0] for r in db.execute("SELECT questid FROM quests WHERE questid >= ?", (FIRST_NEW_ID,))}
        next_id = FIRST_NEW_ID
        new = {}  # chain -> (questid, classes)
        for arch, classes in ARCHETYPES.items():
            for c in classes:
                sides = [s for s in SIDES if self.dwarf_side(f"{s}-{arch}", c)]
                if len(sides) > 1:
                    raise Refused(f"a dwarf {ABBR[c]} can start both {arch} chains already")
            for side in SIDES:
                chain = f"{side}-{arch}"
                missing = [c for c in classes if not self.dwarf_side(chain, c)]
                if not missing:
                    continue
                while next_id in taken:
                    next_id += 1
                new[chain] = (next_id, missing)
                taken.add(next_id)
        self.new = new
        for arch, classes in ARCHETYPES.items():
            for side in SIDES:
                self.plan_chain(side, arch, classes)
        self.plan_gates()

    def plan_chain(self, side, arch, classes):
        chain = f"{side}-{arch}"
        other = f"{'evil' if side == 'good' else 'good'}-{arch}"
        ch, och = self.chains[chain], self.chains[other]
        homes = [ABBR[c] for c in classes if self.dwarf_side(chain, c)]
        self.lines.append(f"{chain}: a dwarf {'/'.join(homes) or 'of no class'} could start it already")
        # race 4 on its ep quests
        lacking = [q for q in ch["ep"] if ints(q["races"]) and DWARF not in ints(q["races"])]
        for q in lacking:
            self.change_quest(q, races=int_list(ints(q["races"]) + [DWARF]))
        if lacking:
            self.lines.append(f"  race 4 added to its {len(lacking)} ep quests")
        # classes of the archetype the chain lacks
        cross = [c for c in classes if c not in ints(ch["ep"][0]["classes"])]
        if cross:
            for q in ch["ep"]:
                self.change_quest(q, classes=int_list(ints(q["classes"]) + cross))
            self.lines.append(f"  classes {'/'.join(ABBR[c] for c in cross)} added to its {len(ch['ep'])} ep quests")
            for c in cross:
                rq = och["reward"].get(c)
                if rq is None:
                    raise Refused(f"{chain}: a {ABBR[c]} has no reward quest on the {other} side either")
                if rq["requires"].strip() != str(och["last"]["questid"]):
                    raise Refused(f"{other}: reward {rq['questid']} requires {rq['requires']!r}")
                req = f"{och['last']['questid']}|{ch['last']['questid']}"
                self.change_quest(rq, requires=req)
                self.lines.append(f"  {rq['questid']} {rq['name']!r} ({other}) also after {ch['last']['questid']} "
                                  f"{ch['last']['name']!r}: requires {req}")
        # the dwarves' recruiter, its exclusion
        mine, theirs = self.new.get(chain), self.new.get(other)
        first = ch["first"]
        if mine:
            nid, missing = mine
            tmpl = min(ch["start"], key=lambda q: q["questid"])
            ex = [och["first"]["questid"]] + ([theirs[0]] if theirs else [])
            row = dict(tmpl, questid=nid, classes=int_list(missing), races=str(DWARF), excludes=int_list(ex),
                       source=f"{SOURCE} (60/{chain}/start-dwarf, a copy of {tmpl['questid']})",
                       notes=(f"A dwarf's recruiter for the {chain} epic (the owner, 2026-11-22: dwarves choose which "
                              f"level 60 epic to run). Excludes the {other} chain's first quest: one epic, never both."))
            self.changes.append(("quests", {"questid": nid}, None, row))
            for st in rows(self.db, "SELECT * FROM queststeps WHERE questid = ? ORDER BY step", (tmpl["questid"],)):
                self.changes.append(("queststeps", {"questid": nid, "step": st["step"]}, None, dict(st, questid=nid)))
            for t in ("queststeptargets", "questunlocks"):
                if has_table(self.db, t):
                    for r in rows(self.db, f"SELECT * FROM {t} WHERE questid = ?", (tmpl["questid"],)):
                        self.changes.append((t, {k: (nid if k == "questid" else r[k]) for k in KEYS[t]}, None,
                                             dict(r, questid=nid)))
            req = first["requires"].strip() + f"|{nid}"
            self.change_quest(first, requires=req)
            self.lines.append(f"  new {nid} {RECRUITER!r} at {tmpl['giver']!r} (a copy of {tmpl['questid']}): "
                              f"{'/'.join(ABBR[c] for c in missing)}, dwarves only, excludes {int_list(ex)}; "
                              f"{first['questid']} {first['name']!r} requires {req}")
        if theirs:
            self.change_quest(first, excludes=str(theirs[0]))
            self.lines.append(f"  {first['questid']} {first['name']!r} excludes {theirs[0]} (the {other} dwarves' recruiter)")

    def change_quest(self, q, **cols):
        """One quests row's change, merged with any planned before it."""
        for i, (tbl, ref, before, after) in enumerate(self.changes):
            if tbl == "quests" and ref["questid"] == q["questid"] and before is not None:
                self.changes[i] = (tbl, ref, before, dict(after, **cols))
                return
        self.changes.append(("quests", {"questid": q["questid"]}, dict(q), dict(q, **cols)))

    def plan_gates(self):
        """What the quests themselves gate (factionreq), and the givers' factions against a dwarf's start."""
        db = self.db
        start = {}
        for r in rows(db, "SELECT race, class, humtype, faction, standing FROM factionstart WHERE race IN (-1, ?)", (DWARF,)):
            start.setdefault(r["faction"], []).append(r)
        base = {r["name"]: r["standing"] for r in rows(db, "SELECT name, standing FROM factions")}

        def standing(faction, cls):
            best = None
            for r in start.get(faction, []):
                if r["class"] not in (-1, cls) or r["humtype"] not in (-1, 0):
                    continue
                spec = (r["race"] >= 0) + (r["class"] >= 0)
                if best is None or spec > best[0]:
                    best = (spec, r["standing"])
            return best[1] if best else base.get(faction, 0)

        nf = {}
        for r in rows(db, "SELECT lower(trim(name)) AS n, faction FROM npcfaction"):
            nf.setdefault(r["n"], set()).add(r["faction"])
        for arch, classes in ARCHETYPES.items():
            for side in SIDES:
                chain = f"{side}-{arch}"
                ch = self.chains[chain]
                for q in ch["ep"] + ch["start"]:
                    if q["factionreq"].strip():
                        self.gates.append(f"{chain}: {q['questid']} {q['name']!r} factionreq {q['factionreq']!r}")
                ids = [q["questid"] for q in ch["ep"]]
                names = {q["giver"].strip().lower() for q in ch["ep"]}
                names |= {r[0].strip().lower() for r in db.execute(
                    f"SELECT DISTINCT target FROM queststeps WHERE questid IN ({','.join('?' * len(ids))})", ids)}
                for n in sorted(names):
                    for f in sorted(nf.get(n, ())):
                        kos = [ABBR[c] for c in classes if standing(f, c) <= KOS_AT]
                        if kos:
                            self.gates.append(f"{chain}: {n} is {f}, KOS to a new dwarf {'/'.join(kos)} "
                                              f"({', '.join(str(standing(f, c)) for c in classes if ABBR[c] in kos)})")

    # ---- writing ---------------------------------------------------------------------------------------------------

    def apply(self, shard):
        db = self.db
        db.execute(LOG_DDL)
        db.execute(AUDIT_DDL)
        at = now()
        n = 0
        for tbl, ref, before, after in self.changes:
            if before is None:
                cols = list(after)
                db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                           [after[c] for c in cols])
                action = "insert"
            else:
                diff = {k: v for k, v in after.items() if before.get(k) != v}
                db.execute(f"UPDATE {tbl} SET {', '.join(f'{k} = ?' for k in diff)} WHERE "
                           + " AND ".join(f"{k} = ?" for k in KEYS[tbl]), list(diff.values()) + [ref[k] for k in KEYS[tbl]])
                action = "update"
            aid = audit(db, at, action, tbl, ref, before, after, f"{DETAIL} ({shard})")
            db.execute(f"INSERT INTO {LOG} (tbl, ref, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?)",
                       (tbl, go_json(ref), None if before is None else go_json(before), go_json(after), aid, at))
            n += 1
        return n


def audit(db, at, action, tbl, ref, before, after, detail):
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, 0, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, WHO, VIA, action, tbl, go_json(ref), None if before is None else go_json(before),
                      None if after is None else go_json(after), detail))
    return cur.lastrowid


def row_now(db, tbl, ref):
    r = rows(db, f"SELECT * FROM {tbl} WHERE " + " AND ".join(f"{k} = ?" for k in KEYS[tbl]),
             [ref[k] for k in KEYS[tbl]])
    return r[0] if r else None


def undo(db, shard):
    """Each logged row put back while it is as this tool left it; (back, left)."""
    if not has_table(db, LOG):
        return 0, 0
    db.execute(AUDIT_DDL)
    at = now()
    back = left = 0
    new_ids = set()
    logged = rows(db, f"SELECT * FROM {LOG} ORDER BY seq DESC")
    for r in logged:
        if r["tbl"] == "quests" and r["before"] is None:
            new_ids.add(json.loads(r["ref"])["questid"])
    held = {q: n for q, n in db.execute(
        f"SELECT questid, count(*) FROM charquests WHERE questid IN ({','.join('?' * len(new_ids))}) GROUP BY questid",
        sorted(new_ids))} if new_ids and has_table(db, "charquests") else {}
    for r in logged:
        tbl, ref = r["tbl"], json.loads(r["ref"])
        before = None if r["before"] is None else json.loads(r["before"])
        after = json.loads(r["after"])
        cur = row_now(db, tbl, ref)
        qid = ref.get("questid")
        if before is None and qid in held:
            print(f"  left {tbl} {ref}: {held[qid]} characters hold quest {qid}")
            left += 1
            continue
        if cur is None or any(cur.get(k) != v for k, v in after.items()):
            print(f"  left {tbl} {ref}: changed since")
            left += 1
            continue
        if before is None:
            db.execute(f"DELETE FROM {tbl} WHERE " + " AND ".join(f"{k} = ?" for k in KEYS[tbl]),
                       [ref[k] for k in KEYS[tbl]])
            audit(db, at, "delete", tbl, ref, after, None, f"{DETAIL} ({shard}): undone")
        else:
            diff = {k: v for k, v in before.items() if after.get(k) != v}
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{k} = ?' for k in diff)} WHERE "
                       + " AND ".join(f"{k} = ?" for k in KEYS[tbl]), list(diff.values()) + [ref[k] for k in KEYS[tbl]])
            audit(db, at, "update", tbl, ref, after, before, f"{DETAIL} ({shard}): undone")
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (r["seq"],))
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def backup(path):
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-dwarfepic"
    src = sqlite3.connect(path, timeout=5)
    out = sqlite3.connect(dst)
    with out:
        src.backup(out)
    out.close()
    src.close()
    return dst


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--shard", required=True, choices=("heirloom", "live"))
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    ap.add_argument("--no-backup", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, timeout=5)
    try:
        if a.undo:
            if not has_table(db, LOG):
                print("nothing to undo: no log")
                return 0
            if not a.no_backup:
                print(f"backup: {backup(a.db)}")
            back, left = undo(db, a.shard)
            db.commit()
            print(f"undo: {back} rows put back, {left} left")
            return 0
        if has_table(db, LOG):
            n = db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]
            print(f"applied already ({n} rows in {LOG}): nothing to do")
            return 0
        p = Plan(db)
        try:
            p.plan()
        except Refused as e:
            print(f"REFUSED, nothing written: {e}")
            return 2
        for line in p.lines:
            print(line)
        print("gates (not changed):")
        for line in p.gates or ["  none"]:
            print(f"  {line}")
        ins = sum(1 for c in p.changes if c[2] is None)
        print(f"{len(p.changes)} rows: {ins} new, {len(p.changes) - ins} changed")
        if a.apply:
            if not a.no_backup:
                print(f"backup: {backup(a.db)}")
            n = p.apply(a.shard)
            db.commit()
            print(f"applied: {n} rows; live at a GM panel quest reload or the next start")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
