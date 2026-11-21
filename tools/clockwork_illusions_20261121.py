#!/usr/bin/env python3
"""The Clockwork Invasion's illusions and its windmill (tickets 2603 and 2602), after
tools/clockwork_invasion_20261120.py built the chain without either.

SOURCE: Cryptic Souls' "Klick Clockworks Quest" by Morynn (external/questmasters/text/id357.txt; [CS]).

TICKET 2603, "Not getting the different illusion spell when completing each part". [CS] gives each of Elder
Mysidia's stages an illusion: 1 "Illusion of Clockwork Corrosion", 2 "Illusion of c. rat", 3 "Illusion of
c.Spider", 4 "Illusion of clockwork F (female) and Illusion of Clockwork M (male)", 5 "Illusion of clockwork
drachnid F (female) and Illusion of Clockwork drachnid M (male)"; the Corroded Giant "drops illusions", and its
"Forms available" are eight: G. Cube (gelatinous cube), Clockwork M, Clockwork F, C. Drachnid M, C. Drachnid F,
D. Corrosion (green jelly), C. Rat, C. Spider. Stage 1's "Clockwork Corrosion" is not in that list and the list's
"D. Corrosion" is no stage's: one form, the deadly corrosions stage 1 is fought with (green jelly's body), made
once here. Eight spells, then, each shaped exactly as Illusion: Clay Golem is (its spellpattern and spelleffect
rows: an "Illusion: <form>" of target scope, the workbook's illusion effect with its sprite), its body the
CHAR.ESF body a row of npcs of that creature wears (checked here: refused if no such row wears it), its sprite
that body's DictID byte-reversed (entity/spelljson.go spriteBody). Bodies found by sight off the disc (vanilla's
`eqoa shot CHAR.ESF <DictID>`):

    Illusion: Clockwork Corrosion        0x8348419F  deadly corrosion, green jelly (a green jelly blob)
    Illusion: Clockwork Rat              0x06024005  Model XIV (the clockwork rat)
    Illusion: Clockwork Spider           0x15BDF1BD  ancient clockwork, Model Series XXIII (the spiderbot)
    Illusion: Clockwork Female           0xFBABEB00  Guard Model, Patrol Droid XJ5 (the female clockwork)
    Illusion: Clockwork Male             0x9CF75E81  corroded clockwork, clockwork worker (the male clockwork)
    Illusion: Clockwork Drachnid Female  0x15764DA8  Deidre IV (a woman's torso on the spiderbot's legs)
    Illusion: Clockwork Drachnid Male    0x9D3DBD61  Model XJ3 Overdrone (a man's torso on them)
    Illusion: G. Cube                    0xD81E78E8  gelatinous cube

[CS]'s own "G. Cube" is kept for the cube: "Illusion: Gelatinous Cube" is an older spell and item of this server
(a scroll of that name would scribe either). Each stage's hand-in to Mysidia teaches its illusion(s) to every
class (queststeps.spells: handlers/quests.go teachLocked, as a scribe does); the giant's illusions are scrolls
(the illusion scrolls' shape, Illusion: Clay Golem's, usable by every class and race), one of the eight a kill
from a table of their own beside his armour's (his armour still one a kill). Levels ours: a stage's its reward
level (10-50), the cube 60; a scroll's price the illusion scrolls' of about its level.

TICKET 2602, "first part doesn't spawn on the hill by the windmill like the quest says". The windmill was a
guess (23580, 54.2, 6580: the plain by the east road). The disc's: TUNARIA.ESF places the windmill sprite
(0x353969CC, an HSprite: a tower and four turning sails) four times in the whole world, all in Klick`Anon's zone
31. The ramp out of the underground city rises from Mysidia's floor (y 32.7 at 23560, 6345) to the surface by
Watchman Roupe and Captain Xarta (y 54.2 at 23520, 6390): one comes up it facing south-west, so the left hand is
south-east, and [CS]'s "directly on the left side as you exit the ramp going up ... (s/e of entrance)" is the
windmill at (23620.2, 73.4, 6493.7), due south-east of the ramp's top (the others: one east-south-east up the
ridge at 23599, 90.8, 6417, two to the north-west). It stands on a terrace (floor 73.5) above the plain. Its
sails face its +z; its back (the portal [CS] names) is toward +x/-z by its yaw (-0.87): the spawns 7 back from
it, clear of the light well a few steps east. The reach step's place and the stage's spawns (its events, the
Corroded Saboteur's and Militant's anchor rows) move there.

Report only by default; --apply writes it in one transaction, every row with a gm_audit line (via "tool") and in
clockwork_1121_log; --undo puts back exactly what it wrote (a row changed since is left and said) and drops the
log; --applied lists it. Refused whole if a row is not as this tool expects. By NAMES, never ids.

WHEN IT TAKES EFFECT: the spells and scrolls at once (read as they are used); the quests, the anchors' places
and the giant's table at the next restart.

    python3 -E -s tools/clockwork_illusions_20261121.py --db eqoa.db [--apply | --applied | --undo]
"""
import argparse
import datetime
import json
import math
import sqlite3
import sys

LOG = "clockwork_1121_log"
WHO, VIA = "dab", "tool"
SRC = "clockwork1121"
CS = "Cryptic Souls' Klick Clockworks Quest (questmasters id357)"
DETAIL = f"the Clockwork Invasion's illusions and windmill (tickets 2603, 2602) as {CS}: tools/clockwork_illusions_20261121.py"

GIVER = "Elder Mysidia"
W = 0  # Tunaria

# 2602: the windmill (TUNARIA.ESF zone 31, sprite 0x353969CC) and its back
MILL_SPRITE = 0x353969CC
MILL_AT, MILL_YAW = (23620.2, 73.4, 6493.7), -0.87
FLOOR = 73.5  # its terrace (worlds/ground)
BACK = 7.0


def behind(at, yaw, d):
    """The point d behind a placed sprite whose front is its +z: SpritePlacement.Transform turns about Y by yaw
    (x' = x cos + z sin, z' = -x sin + z cos), so its local -z is (-sin, -cos)."""
    return (round(at[0] - math.sin(yaw) * d, 1), FLOOR, round(at[2] - math.cos(yaw) * d, 1))


WINDMILL = (MILL_AT[0], FLOOR, MILL_AT[2])
PORTAL = behind(MILL_AT, MILL_YAW, BACK)
OLD = (23580.0, 54.2, 6580.0)  # tools/clockwork_invasion_20261120.py's WINDMILL
Q1 = "The Clockwork Invasion: The Windmill"
ANCHORS = ("Corroded Saboteur", "Corroded Militant")

# 2603: (spell, level, body, a creature whose rows wear it, the Mysidia stage that teaches it or 0)
ILLUSIONS = [
    ("Illusion: Clockwork Corrosion", 10, 0x8348419F, "deadly corrosion", 1),
    ("Illusion: Clockwork Rat", 20, 0x06024005, "model xiv", 2),
    ("Illusion: Clockwork Spider", 30, 0x15BDF1BD, "ancient clockwork", 3),
    ("Illusion: Clockwork Female", 40, 0xFBABEB00, "patrol droid xj5", 4),
    ("Illusion: Clockwork Male", 40, 0x9CF75E81, "clockwork worker", 4),
    ("Illusion: Clockwork Drachnid Female", 50, 0x15764DA8, "deidre iv", 5),
    ("Illusion: Clockwork Drachnid Male", 50, 0x9D3DBD61, "model xj3 overdrone", 5),
    ("Illusion: G. Cube", 60, 0xD81E78E8, "gelatinous cube", 0),
]
STAGES = ["The Windmill", "The Rat Clones", "Klockizar Riddik", "The Drachnid", "The Warlord"]
TEMPLATE = "Illusion: Clay Golem"
SCROLL_ICON = -607649328  # handlers/scribe.go scrollIcon
ALL_CLASSES, ALL_RACES = 32767, 1023
PRICE = {10: 130, 20: 369, 30: 780, 40: 1380, 50: 2200, 60: 3086}  # the illusion scrolls' curve (L39 1311, L60 3086)
GIANT = "corroded giant"
GIANT_ILLUSIONS = "corroded giant illusions"


def sprite(model):
    """The workbook's sprite for a body: its DictID byte-reversed (entity/spelljson.go spriteBody)."""
    return "0x%08X" % int.from_bytes((model & 0xFFFFFFFF).to_bytes(4, "big"), "little")


def has(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def rows(db, tbl, where, args=()):
    cur = db.execute(f'SELECT * FROM "{tbl}" WHERE {where}', args)
    names = [d[0] for d in cur.description]
    return [dict(zip(names, r)) for r in cur.fetchall()]


def row(db, tbl, where, args=()):
    r = rows(db, tbl, where, args)
    return r[0] if r else None


def js(v):
    return json.dumps(v, separators=(",", ":")) if v is not None else None


def moved(events, to):
    """A step's events with every spawn at the old windmill put at the portal."""
    out = json.loads(events)
    n = 0
    for e in out:
        if e.get("do") == "spawn" and (e.get("x"), e.get("y"), e.get("z")) == OLD:
            e["x"], e["y"], e["z"] = to
            n += 1
    return json.dumps(out, separators=(",", ":")), n


class Plan:
    def __init__(self, db):
        self.bad = []
        b = self.bad
        sp = rows(db, "spellpattern", "spellname = ?", (TEMPLATE,))
        if len(sp) != 1:
            b.append(f"{len(sp)} spells {TEMPLATE!r}, want one")
        else:
            self.spell = sp[0]
            self.effect = row(db, "spelleffect", "spellid = ?", (sp[0]["spellid"],))
            try:
                fx = json.loads(self.effect["effects"])
                ill = [e for e in fx["effects"] if e.get("kind") == "illusion"]
                if fx.get("v") != 1 or len(ill) != 1 or fx.get("scope") != "target":
                    raise ValueError("not one illusion of target scope")
            except (TypeError, ValueError, KeyError) as e:
                b.append(f"{TEMPLATE}'s effects are not as expected: {e}")
        sc = rows(db, "itempattern", "itemname = ? AND itemicon = ? AND equipslot = -1", (TEMPLATE, SCROLL_ICON))
        if len(sc) != 1:
            b.append(f"{len(sc)} scrolls {TEMPLATE!r}, want one")
        else:
            self.scroll = sc[0]
        for name, lv, model, wearer, _ in ILLUSIONS:
            if rows(db, "spellpattern", "lower(trim(spellname)) = lower(?)", (name,)):
                b.append(f"a spell {name!r} is there already")
            if rows(db, "itempattern", "lower(trim(itemname)) = lower(?)", (name,)):
                b.append(f"an item {name!r} is there already")
            if not db.execute("SELECT 1 FROM npcs WHERE lower(trim(npc_name)) = ? AND (modelid & 4294967295) = ?",
                              (wearer, model)).fetchone():
                b.append(f"no npcs row of {wearer!r} wears {model:#010x}: {name}'s body unproven")
        self.give = {}
        for n, st in enumerate(STAGES, 1):
            q = rows(db, "quests", "name = ?", (f"The Clockwork Invasion: {st}",))
            if len(q) != 1:
                b.append(f"{len(q)} quests 'The Clockwork Invasion: {st}'")
                continue
            g = rows(db, "queststeps", "questid = ? AND kind = 'give' AND target = ?", (q[0]["questid"], GIVER))
            if len(g) != 1:
                b.append(f"stage {n}: {len(g)} hand-ins to {GIVER}")
            elif g[0]["spells"].strip():
                b.append(f"stage {n}'s hand-in teaches {g[0]['spells']!r} already")
            else:
                self.give[n] = g[0]
        q1 = rows(db, "quests", "name = ?", (Q1,))
        self.steps = []
        if len(q1) == 1:
            for s in rows(db, "queststeps", "questid = ? ORDER BY step", (q1[0]["questid"],)):
                if s["kind"] == "reach":
                    if (s["x"], s["y"], s["z"]) != OLD:
                        b.append(f"{Q1}'s reach step is at {(s['x'], s['y'], s['z'])}, not {OLD}")
                    self.steps.append(s)
                elif s["events"] and moved(s["events"], PORTAL)[1]:
                    self.steps.append(s)
            if not self.steps or sum(moved(s["events"], PORTAL)[1] for s in self.steps) != 3:
                b.append(f"{Q1}: not the reach step and three spawns at {OLD}")
        self.anchors = []
        for a in ANCHORS:
            r = rows(db, "npcs", "npc_name = ?", (a,))
            if len(r) != 1 or (r[0]["x_coord"], r[0]["y_coord"], r[0]["z_coord"]) != OLD:
                b.append(f"want one row of {a} at {OLD}")
            else:
                self.anchors.append(r[0])
        if not db.execute("SELECT 1 FROM npcloot WHERE npc_name = ? AND tablename = ?", (GIANT, GIANT)).fetchone():
            b.append(f"the {GIANT}'s own table is not there (run tools/clockwork_invasion_20261120.py first)")
        if db.execute("SELECT 1 FROM loottables WHERE tablename = ?", (GIANT_ILLUSIONS,)).fetchone() or \
                db.execute("SELECT 1 FROM npcloot WHERE tablename = ?", (GIANT_ILLUSIONS,)).fetchone():
            b.append(f"a table {GIANT_ILLUSIONS!r} is there already")


def report(db):
    for r in db.execute("SELECT s.spellid, s.spellname, s.abilitylvl, json_extract(e.effects, '$.effects[0].sprite') "
                        "FROM spellpattern s JOIN spelleffect e USING (spellid) WHERE e.source LIKE ? ORDER BY s.spellid",
                        (f"%{SRC}%",)):
        print(f"  spell {r}")
    for r in db.execute("SELECT q.name, s.step, s.kind, s.x, s.y, s.z, s.spells FROM queststeps s JOIN quests q USING "
                        "(questid) WHERE q.source = 'clockwork1120' AND (s.kind IN ('reach', 'give') AND s.questid IN "
                        "(SELECT questid FROM quests WHERE giver = ?)) ORDER BY s.questid, s.step", (GIVER,)):
        print(f"  step {r}")


def apply(db, p):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    db.execute(f"CREATE TABLE {LOG} (seq INTEGER PRIMARY KEY, tbl TEXT NOT NULL, rowkey TEXT NOT NULL, before TEXT, after TEXT)")
    n = [0]

    def done(tbl, key, before):
        where = " AND ".join(f"{c} = ?" for c in key)
        after = row(db, tbl, where, tuple(key.values()))
        action = "update" if before and after else "insert" if after else "delete"
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES (?, ?, ?, ?)", (tbl, js(key), js(before), js(after)))
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                   "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                   (at, acct[0] if acct else 0, WHO, VIA, action, tbl, js(key), js(before), js(after), DETAIL))
        n[0] += 1

    def insert(tbl, values, keycol):
        cur = db.execute(f'INSERT INTO "{tbl}" ({", ".join(values)}) VALUES ({", ".join("?" * len(values))})',
                         list(values.values()))
        key = {keycol: values[keycol] if keycol in values else cur.lastrowid}
        done(tbl, key, None)
        return key[keycol]

    def update(tbl, key, values):
        where = " AND ".join(f"{c} = ?" for c in key)
        before = row(db, tbl, where, tuple(key.values()))
        if all(before[c] == v for c, v in values.items()):
            return
        db.execute(f'UPDATE "{tbl}" SET {", ".join(f"{c} = ?" for c in values)} WHERE {where}',
                   list(values.values()) + list(key.values()))
        done(tbl, key, before)

    # 2603: the spells, as Illusion: Clay Golem; the scrolls, as its scroll
    fx = json.loads(p.effect["effects"])
    ill = next(e for e in fx["effects"] if e.get("kind") == "illusion")
    spells, scrolls = {}, {}
    for k, (name, lv, model, wearer, stage) in enumerate(ILLUSIONS, 1):
        r = dict(p.spell)
        r.pop("spellid")
        r.update(spellname=name, abilitylvl=lv)
        sid = insert("spellpattern", r, "spellid")
        e = dict(ill, id=f"CW{k:02d}", sprite=sprite(model))
        f = dict(fx, level=lv, min_level=lv, effects=[e])
        insert("spelleffect", {"spellid": sid, "line": p.effect["line"], "fxdict": p.effect["fxdict"],
                               "source": f"{CS}; {SRC}: the body {wearer}'s ({model:#010x}), shaped as {TEMPLATE}",
                               "effects": json.dumps(f, separators=(",", ":"))}, "spellid")
        spells[name] = sid
        s = dict(p.scroll)
        s.pop("patternid")
        s.update(itemname=name, itemdesc=f"This scroll will allow you to scribe {name}. Illusion only", levelreq=lv,
                 classuse=ALL_CLASSES, raceuse=ALL_RACES, itemcost=PRICE[lv])
        scrolls[name] = insert("itempattern", s, "patternid")

    # each Mysidia stage's hand-in teaches its illusions
    for stage, g in p.give.items():
        ids = [str(spells[name]) for name, _, _, _, st in ILLUSIONS if st == stage]
        update("queststeps", {"questid": g["questid"], "step": g["step"]}, {"spells": ", ".join(ids)})

    # the giant's illusions: one scroll a kill, a table of their own
    for name in scrolls:
        cur = db.execute("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) "
                         "VALUES (?, ?, 1, 1, 70, ?)", (GIANT_ILLUSIONS, scrolls[name], SRC))
        done("loottables", {"id": cur.lastrowid}, None)
    db.execute("INSERT INTO npcloot (npc_name, tablename, chance, rolls, note, source) VALUES (?, ?, 100, 1, ?, ?)",
               (GIANT, GIANT_ILLUSIONS, f"{CS}: 'He drops illusions': one of the eight forms' scrolls a kill", SRC))
    done("npcloot", {"npc_name": GIANT, "tablename": GIANT_ILLUSIONS}, None)

    # 2602: the windmill
    for s in p.steps:
        v = {}
        if s["kind"] == "reach":
            v.update(x=WINDMILL[0], y=WINDMILL[1], z=WINDMILL[2])
        if s["events"]:
            v["events"] = moved(s["events"], PORTAL)[0]
        update("queststeps", {"questid": s["questid"], "step": s["step"]}, v)
    for a in p.anchors:
        update("npcs", {"id": a["id"]}, {"x_coord": PORTAL[0], "y_coord": PORTAL[1], "z_coord": PORTAL[2]})
    return n[0]


def undo(db):
    n = 0
    for tbl, rowkey, before, after in db.execute(f"SELECT tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(rowkey)
        where = " AND ".join(f"{c} = ?" for c in key)
        now = row(db, tbl, where, tuple(key.values()))
        if js(now) != after:
            print(f"  {tbl} {rowkey}: changed since this tool wrote it, left as it is")
            continue
        if before is None:
            db.execute(f'DELETE FROM "{tbl}" WHERE {where}', tuple(key.values()))
        else:
            b = json.loads(before)
            db.execute(f'UPDATE "{tbl}" SET {", ".join(f"{c} = ?" for c in b)} WHERE {where}',
                       list(b.values()) + list(key.values()))
        n += 1
    db.execute(f"DROP TABLE {LOG}")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', ?, '{}', NULL, NULL, ?)",
               (datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"), acct[0] if acct else 0,
                WHO, VIA, LOG, f"undone, {n} rows put back: " + DETAIL))
    print(f"  {n} rows put back")


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--applied", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        if a.applied:
            if not has(db, LOG):
                print("not applied")
                return 1
            for tbl, key, before, after in db.execute(f"SELECT tbl, rowkey, before, after FROM {LOG} ORDER BY seq"):
                print(f"  {tbl} {key}: {'insert' if before is None else 'update'}")
            report(db)
            return 0
        if a.undo:
            if not has(db, LOG):
                print("nothing to undo")
                return 0
            db.execute("BEGIN IMMEDIATE")
            undo(db)
            db.execute("COMMIT")
            print("undone; the spells and scrolls at once, the rest at a restart")
            return 0
        if has(db, LOG):
            print(f"applied already ({LOG} is there): --undo to take it back")
            return 0
        p = Plan(db)
        if p.bad:
            print("refused, nothing done:\n  " + "\n  ".join(p.bad))
            return 1
        print(f"  would write: {len(ILLUSIONS)} illusion spells + scrolls as {TEMPLATE!r}; stages "
              f"{sorted(p.give)} teach theirs; table {GIANT_ILLUSIONS!r} for the giant; {Q1} at the windmill "
              f"{WINDMILL}, its spawns and {', '.join(ANCHORS)} at its back {PORTAL}")
        for name, lv, model, wearer, stage in ILLUSIONS:
            print(f"    {name:38} L{lv} body {model:#010x} ({wearer}) sprite {sprite(model)} "
                  f"{'stage %d' % stage if stage else 'the giant'}")
        if not a.apply:
            print("report only: --apply to write it")
            return 0
        db.execute("BEGIN IMMEDIATE")
        n = apply(db, p)
        db.execute("COMMIT")
        print(f"applied, {n} rows; the spells and scrolls at once, the quests, anchors and loot at the next restart")
        report(db)
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
