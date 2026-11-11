#!/usr/bin/env python3
"""The content EQOA-Data's guides describe and the server lacked, as data: one set a
file of tools/guidegaps/ (sky-twins, rallos, razeal, oneoffs, pod-twins; Zaoran Plateau's
tallon, vallon, zp-armour, then zp-minions, and zp-records), each with its quest file in
tools/quests/data/ when it has one. The user, 2026-11-07: "lets build them all".

    tools/seed_guide_gaps.py --db eqoa.db --set sky-twins              report: what it would write
    tools/seed_guide_gaps.py --db eqoa.db --set sky-twins --apply
    tools/seed_guide_gaps.py --db eqoa.db --set sky-twins --undo

A set's file (JSON) holds whatever of these it needs:

  items      [{name, like, description, notrade, ..columns of itempattern..}]: a copy of
             the record named like, unless a record has the name
  spells     [{id, name, line, effects, fx, icon_like, scope, cast, recast, range, area}]
  templates  [{name, class, hp, level, taken, melee, crit, shield.., abilities, note}]: rows of
             classtemplates; hp and level make hp_pct (240 a level is what a level gives)
  npcs       [{name, world, x, y, z, facing, level, type, body, size, like, look, spawnonly,
             template, aggro, charm, undead, class, roam, count}]: a row of npcs each
  triggers   [{slot, name, scope, radius, world, x, y, z, steps, notes}]: rows of gmtriggers
  deaths     [{name, trigger, chance, note}]: rows of npcdeath
  loot       [{table, items: [name or [name, weight]], npcs: [[npc, chance, rolls]]}]
  step_events [{quest, step, target, events}]: events added to a step of a quest in the game
  dialog     [{name, line}]: an NPC's own lines, said when nothing else answers
  sql        [[statement, params]]: anything else, in the same transaction
  quests     "<file>": tools/quests/data/<file>, through import_quests.py's plan and write

--save-plan <file> on a copy and --plan <file> on the live database write the same rows in one
short transaction (a live server waits 5 s for a lock; a keep restart's hold is 12 s), each row
checked still to be what the plan found. --apply and --undo are one transaction each. Everything an apply changes is found by
comparing its tables before and after, and kept in guide_set_log, so --undo is exact:
the rows' old values, the counters' too, and the log's table dropped with its last set.
A quest's progress and a made item a character holds go with the undo (said).
Quests, NPCs, templates and triggers' death table are read at the server's start.
"""
import argparse
import json
import os
import re
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import import_quests  # noqa: E402

LOG = "guide_set_log"
# what a set may touch; found by comparing them whole, so nothing is left out of an undo
TABLES = ("itempattern", "spellpattern", "spelleffect", "spellarea", "classtemplates", "npctemplate", "npcs", "npcitem",
          "npcspawnonly", "npcaggro", "npccharm", "npcmove", "npcundead", "npcclass", "npcfaction", "npcsocial", "signtext",
          "loottables", "npcloot", "lootcreature", "gmtriggers", "npcdeath", "itemeffect", "npcrespawn", "npcdialog", "quests", "queststeps",
          "factions", "factionhits",
          "questunlocks", "queststeptargets", "quest_import_quests", "quest_import_items", "quest_import_meta", "sqlite_sequence")
BASE_HP = 240  # what a level gives a creature of no template: QoV 700,108 at 64 and 4558%


def key(name):
    return (name or "").strip().lower()


def has(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type='table' AND name=?", (t,)).fetchone() is not None


def snapshot(db):
    snap = {}
    for t in TABLES:
        if has(db, t):
            cols = [r[1] for r in db.execute(f'PRAGMA table_info("{t}")')]
            snap[t] = (cols, {r[0]: tuple(r[1:]) for r in db.execute(f'SELECT rowid, * FROM "{t}"')})
    return snap


def changes(before, after):
    """Every row that differs, as (table, op, rowid, old row JSON, new row JSON); a table made by the set: op 'made'."""
    out = []
    for t, (cols, rows) in after.items():
        if t not in before:
            out.append((t, "made", 0, "", json.dumps({"cols": cols})))
            for rid, row in rows.items():
                out.append((t, "added", rid, "", json.dumps(dict(zip(cols, row)))))
            continue
        old = before[t][1]
        for rid, row in rows.items():
            if rid not in old:
                out.append((t, "added", rid, "", json.dumps(dict(zip(cols, row)))))
            elif old[rid] != row:
                out.append((t, "changed", rid, json.dumps(dict(zip(cols, old[rid]))), json.dumps(dict(zip(cols, row)))))
        for rid, row in old.items():
            if rid not in rows:
                out.append((t, "gone", rid, json.dumps(dict(zip(cols, row))), ""))
    return out


def item_row(db, name):
    return db.execute("SELECT patternid FROM itempattern WHERE lower(trim(itemname)) = ?", (key(name),)).fetchone()


def item_id(db, name):
    r = item_row(db, name)
    if not r:
        raise SystemExit(f"no item {name!r}")
    return r[0]


STATS = ("str", "sta", "agi", "wis", "dex", "cha", "intelligence", "HPMAX", "POWMAX", "PoT", "HoT", "AC", "PR", "DR", "FR", "CR", "LR", "AR", "weapondamage")


def apply_items(db, spec, say):
    cols = [r[1] for r in db.execute("PRAGMA table_info(itempattern)")]
    for it in spec.get("items", []):
        if item_row(db, it["name"]):
            say(f"item {it['name']!r} is there: left")
            continue
        src = db.execute("SELECT * FROM itempattern WHERE lower(trim(itemname)) = ?", (key(it["like"]),)).fetchone()
        if not src:
            raise SystemExit(f"item {it['name']!r}: no record {it['like']!r} to copy")
        row = dict(zip(cols, src))
        row.pop("patternid")
        row["itemname"], row["itemdesc"] = it["name"], it.get("description", row["itemdesc"])
        if it.get("notrade"):
            row["trade"] = 1
        if "stats" in it:  # its figures alone, none of the copied record's
            for c in STATS:
                row[c] = 0
            row.update(it["stats"])
        for k, v in it.items():
            if k in row:
                row[k] = v
        names = list(row)
        db.execute(f'INSERT INTO itempattern ({",".join(names)}) VALUES ({",".join("?" * len(names))})', [row[n] for n in names])


def apply_spells(db, spec, say):
    for s in spec.get("spells", []):
        like = db.execute("SELECT icon, iconColor, range FROM spellpattern WHERE spellid = 760102").fetchone() or (0, 0, 16880)
        db.execute("""INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast,
            eqprequire, spellname, spelldesc) VALUES (?,?,0,0,?,?,0,?,?,?,?,-1,?,?)""",
                   (s["id"], s.get("level", 60), s.get("range", like[2]), s.get("cast", 0), like[1], like[0], s.get("scope", 1), s.get("recast", 0),
                    s["name"], s["line"]))
        db.execute("INSERT INTO spelleffect (spellid, line, fxdict, source, effects) VALUES (?,?,?,?,?)",
                   (s["id"], s["line"], s.get("fx", 0), f"guide gaps {spec['set']}", json.dumps(s["effects"], separators=(",", ":"))))
        if s.get("area"):
            db.execute("INSERT INTO spellarea (spellid, centre, radius, maxtargets, note) VALUES (?,?,?,0,?)",
                       (s["id"], s["area"].get("centre", 0), s["area"]["radius"], f"guide gaps {spec['set']}"))


def apply_templates(db, spec, say):
    for t in spec.get("templates", []):
        hp_pct = t.get("hp_pct") or round(t["hp"] * 100 / (BASE_HP * t["level"]))
        crit_level = t.get("crit", 0) / t["level"] if t.get("level") else 0
        row = {"name": key(t["name"]), "class": t.get("class", 0), "hp_pct": hp_pct, "taken_pct": t.get("taken", 100), "melee_pct": t.get("melee", 100),
               "special": "Critical Hit" if t.get("crit") else "", "special_damage": crit_level, "special_every": 30.0 if t.get("crit") else 0,
               "special_first": 15.0 if t.get("crit") else 0, "special_jitter": 5.0 if t.get("crit") else 0, "special_from": 10 if t.get("crit") else 0,
               "abilities": json.dumps(t.get("abilities", []), separators=(",", ":")), "note": t.get("note", ""), "source": f"guide gaps {spec['set']}",
               "shield_damage": t.get("shield_damage", 0), "shield_resist": t.get("shield_resist", ""), "proc_chance": t.get("proc_chance", 0),
               "proc_damage": t.get("proc_damage", 0), "proc_name": t.get("proc_name", ""), "proc_resist": t.get("proc_resist", ""),
               "resist_fail": t.get("resist_fail", 0), "resist_kind": t.get("resist_kind", ""), "resist_pct": t.get("resist_pct", 0)}
        names = list(row)
        quoted = ",".join(f'"{n}"' for n in names)
        db.execute(f'INSERT OR REPLACE INTO classtemplates ({quoted}) VALUES ({",".join("?" * len(names))})', [row[n] for n in names])


NPC_DEFAULTS = {"zone": 0, "unk_a": 0, "unk_b": 0, "dt_kill": None, "hp": 1, "hp_percent": 100, "unk_d": 0, "unk_f": 0, "unk_g": 0,
                "primary_weapon": 0, "secondary_weapon": 0, "shield": 0, "unk_h": 0, "torso": 0, "forearm": 0, "gloves": 0, "legs": 0, "feet": 0,
                "head": 0, "hair_color": 0, "hair_length": 0, "hair_style": 0, "face": 0, "robe_style": "0", "race": "", "unk_j": 0, "npc_id": 0,
                "unk_k": 0, "unk_i": "", "pd_icon": 0, "unk_m": 0, "unk_n": 0, "unk_o": 0}


def signed32(v):
    v &= 0xFFFFFFFF
    return v - (1 << 32) if v >= (1 << 31) else v


def apply_npcs(db, spec, say):
    source = f"guide gaps {spec['set']}"
    cols = [r[1] for r in db.execute("PRAGMA table_info(npcs)")]
    for n in spec.get("npcs", []):
        row = dict(NPC_DEFAULTS)
        if n.get("like"):
            src = db.execute("SELECT * FROM npcs WHERE lower(trim(npc_name)) = ? LIMIT 1", (key(n["like"]),)).fetchone()
            if not src:
                raise SystemExit(f"npc {n['name']!r}: no {n['like']!r} to copy")
            row.update(dict(zip(cols, src)))
            row.pop("id")
        row.update({"npc_name": n["name"], "x_coord": n["x"], "y_coord": n["y"], "z_coord": n["z"], "facing": n.get("facing", 0),
                    "world": n["world"], "npc_level": n.get("level", row.get("npc_level", 60)), "npc_type": n.get("type", row.get("npc_type", 0)),
                    "size": n.get("size", row.get("size", 1.0))})
        if n.get("body") is not None:
            row["modelid"] = signed32(int(n["body"], 16)) if isinstance(n["body"], str) else n["body"]
        row.update(n.get("look", {}))
        for i in range(n.get("count", 1)):
            names = list(row)
            cur = db.execute(f'INSERT INTO npcs ({",".join(names)}) VALUES ({",".join("?" * len(names))})', [row[c] for c in names])
            if not row.get("npc_id"):
                db.execute("UPDATE npcs SET npc_id = id WHERE id = ?", (cur.lastrowid,))
        k = key(n["name"])
        if n.get("spawnonly"):
            db.execute("INSERT OR IGNORE INTO npcspawnonly (name, note, source) VALUES (?,?,?)", (k, n.get("note", ""), source))
        if n.get("template"):
            db.execute("INSERT OR REPLACE INTO npctemplate (name, template, note, source) VALUES (?,?,?,?)", (k, key(n["template"]), n.get("note", ""), source))
        if "aggro" in n:
            a = n["aggro"]
            db.execute("INSERT OR REPLACE INTO npcaggro (npc_name, aggressive, radius, note, source) VALUES (?,?,?,?,?)",
                       (k, a.get("aggressive", 1), a.get("radius", 0), n.get("note", ""), source))
        if n.get("charm") is not None:
            db.execute("INSERT OR REPLACE INTO npccharm (name, charmable, resist_pct, template, note, source) VALUES (?,?,0,'',?,?)", (k, n["charm"], n.get("note", ""), source))
        if n.get("undead"):
            db.execute("INSERT OR REPLACE INTO npcundead (name, undead, note, source) VALUES (?,1,?,?)", (k, n.get("note", ""), source))
        if "class" in n:
            db.execute("INSERT OR REPLACE INTO npcclass (name, class, source) VALUES (?,?,?)", (k, n["class"], source))
        if "roam" in n:
            db.execute("INSERT OR REPLACE INTO npcmove (name, roam, radius, water, note, source) VALUES (?,?,?,'',?,?)", (k, n["roam"], n.get("roam_radius", 0), n.get("note", ""), source))
        if n.get("loot_region"):
            db.execute("INSERT OR REPLACE INTO lootcreature (npc_name, region, profile, source) VALUES (?,?,?,?)", (k, n["loot_region"], n.get("loot_profile", "boss"), source))
        if n.get("sign"):
            for rid, in db.execute("SELECT id FROM npcs WHERE npc_name = ?", (n["name"],)).fetchall():
                db.execute("INSERT OR REPLACE INTO signtext (npcid, kind, text, source, note) VALUES (?,?,?,?,'')", (rid, "sign", n["sign"], source))


def apply_triggers(db, spec, say):
    for t in spec.get("triggers", []):
        steps = json.dumps(t["steps"], separators=(",", ":"))
        db.execute("""INSERT OR REPLACE INTO gmtriggers (slot, name, enabled, scope, radius, world, x, y, z, steps, notes)
            VALUES (?,?,1,?,?,?,?,?,?,?,?)""", (t["slot"], t["name"], t.get("scope", "near"), t.get("radius", 0), t.get("world", -1),
                                                t.get("x", 0), t.get("y", 0), t.get("z", 0), steps, t.get("notes", "")))


NPCDEATH_DDL = """CREATE TABLE IF NOT EXISTS npcdeath (name TEXT NOT NULL, trigger TEXT NOT NULL, chance REAL NOT NULL DEFAULT 100,
            note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm', PRIMARY KEY (name, trigger))"""


def apply_deaths(db, spec, say):
    if spec.get("deaths"):
        db.execute(NPCDEATH_DDL)
    for d in spec.get("deaths", []):
        db.execute("INSERT OR REPLACE INTO npcdeath (name, trigger, chance, note, source) VALUES (?,?,?,?,?)",
                   (key(d["name"]), d["trigger"], d.get("chance", 100), d.get("note", ""), f"guide gaps {spec['set']}"))


def apply_loot(db, spec, say):
    source = f"guide gaps {spec['set']}"
    for l in spec.get("loot", []):
        missing = []
        for it in l["items"]:
            name, weight = (it, 1) if isinstance(it, str) else it
            r = item_row(db, name)
            if not r:
                missing.append(name)
                continue
            db.execute("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) VALUES (?,?,?,1,100,?)",
                       (l["table"], r[0], weight, source))
        if missing:
            say(f"loot {l['table']!r}: no record of {missing}")
        for npc, chance, rolls in l.get("npcs", []):
            db.execute("INSERT OR REPLACE INTO npcloot (npc_name, tablename, chance, rolls, note, source) VALUES (?,?,?,?,'',?)",
                       (key(npc), l["table"], chance, rolls, source))


def apply_dialog(db, spec, say):
    """An NPC's own lines (entity/dialog.go: a name with lines says only those)."""
    for d in spec.get("dialog", []):
        db.execute("INSERT INTO npcdialog (name, line, weight, source) VALUES (?,?,1,?)", (d["name"], d["line"], f"guide gaps {spec['set']}"))


def apply_step_events(db, spec, say):
    """Events added to a step of a quest already in the game: a line of the journal does not move, so a chain under way
    goes on. A give's items by name."""
    for se in spec.get("step_events", []):
        row = db.execute("""SELECT s.questid, s.events, s.target FROM queststeps s JOIN quest_import_quests i ON i.questid = s.questid
            WHERE i.key = ? AND s.step = ?""", (se["quest"], se["step"])).fetchone()
        if not row or key(row[2]) != key(se["target"]):
            say(f"step {se['step']} of {se['quest']} is not {se['target']!r}: left")
            continue
        evs = json.loads(row[1]) if row[1].strip() else []
        for e in se["events"]:
            e = dict(e)
            if "items" in e:
                e["items"] = ", ".join(f"{item_id(db, n)}*{c}" for n, c in e["items"])
            evs.append(e)
        db.execute("UPDATE queststeps SET events = ? WHERE questid = ? AND step = ?", (json.dumps(evs, separators=(",", ":")), row[0], se["step"]))


def apply_sql(db, spec, say):
    for stmt, params in spec.get("sql", []):
        db.execute(stmt, [item_id(db, p[5:]) if isinstance(p, str) and p.startswith("item:") else p for p in params])


def run(db, spec, say):
    apply_items(db, spec, say)
    apply_spells(db, spec, say)
    apply_templates(db, spec, say)
    apply_npcs(db, spec, say)
    apply_triggers(db, spec, say)
    apply_deaths(db, spec, say)
    apply_loot(db, spec, say)
    apply_dialog(db, spec, say)
    apply_step_events(db, spec, say)
    apply_sql(db, spec, say)
    if spec.get("quests"):
        path = os.path.join(HERE, "quests", "data", spec["quests"])
        files = import_quests.load_files([path])
        p = import_quests.plan(db, files)
        written, kept, made = import_quests.write(db, p, own=False)
        say(f"quests: wrote {written}, made {made} items, left {kept}")
        drafts = db.execute("SELECT q.name, q.notes FROM quests q WHERE q.source LIKE ? AND q.enabled = 0", ("%" + os.path.basename(path) + "%",)).fetchall()
        drafts += db.execute("SELECT q.name, q.notes FROM quests q JOIN quest_import_quests i ON i.questid = q.questid WHERE i.file LIKE ? AND q.enabled = 0", ("%" + os.path.basename(path),)).fetchall()
        for name, notes in drafts:
            say(f"DRAFT {name}: {notes[-300:]}")


def log_changes(db, name, ch):
    db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, setname TEXT NOT NULL, tbl TEXT NOT NULL, op TEXT NOT NULL, rid INTEGER NOT NULL, old TEXT NOT NULL)")
    for t, op, rid, old, _ in ch:
        db.execute(f"INSERT INTO {LOG} (setname, tbl, op, rid, old) VALUES (?,?,?,?,?)", (name, t, op, rid, old))


def apply(db, spec, do_commit, say, plan_path=None, extra=None):
    """extra(db, counts), when given, runs in the same transaction before its commit (a caller's gm_audit line)."""
    db.execute("BEGIN IMMEDIATE")
    try:
        if has(db, LOG) and db.execute(f"SELECT 1 FROM {LOG} WHERE setname = ?", (spec["set"],)).fetchone():
            raise SystemExit(f"set {spec['set']!r} is in: --undo it first")
        before = snapshot(db)
        run(db, spec, say)
        after = snapshot(db)
        ch = changes(before, after)
        log_changes(db, spec["set"], ch)
        counts = {}
        for t, op, _, _, _ in ch:
            counts[f"{t} {op}"] = counts.get(f"{t} {op}", 0) + 1
        say(json.dumps(counts, indent=0, sort_keys=True).replace("\n", " "))
        if plan_path:
            json.dump({"set": spec["set"], "changes": ch}, open(plan_path, "w"))
            say(f"plan written to {plan_path} ({len(ch)} rows); nothing written here")
            db.execute("ROLLBACK")
        elif do_commit:
            if extra:
                extra(db, counts)
            db.execute("COMMIT")
            say(f"applied {spec['set']}")
        else:
            db.execute("ROLLBACK")
            say("(a report: nothing written)")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def apply_plan(db, path, say):
    """A plan made on a copy (--save-plan), written in one short transaction: for a server with players on it, or a
    keep restart's hold. Every row must still be what the plan found: anything else refuses, nothing written."""
    p = json.load(open(path))
    name, ch = p["set"], p["changes"]
    db.execute("BEGIN IMMEDIATE")
    try:
        if has(db, LOG) and db.execute(f"SELECT 1 FROM {LOG} WHERE setname = ?", (name,)).fetchone():
            raise SystemExit(f"set {name!r} is in: --undo it first")
        for t, op, rid, old, new in ch:
            if op == "made":
                cols = json.loads(new)["cols"]
                if not has(db, t):
                    ddl = next(r[0] for r in [(NPCDEATH_DDL,)]) if t == "npcdeath" else None
                    if not ddl:
                        raise SystemExit(f"plan makes a table it cannot: {t}")
                    db.execute(ddl)
        for t, op, rid, old, new in ch:
            if op in ("added", "made"):
                continue
            row = db.execute(f'SELECT * FROM "{t}" WHERE rowid = ?', (rid,)).fetchone()
            cols = [r[1] for r in db.execute(f'PRAGMA table_info("{t}")')]
            if t != "sqlite_sequence" and (row is None or dict(zip(cols, row)) != json.loads(old)):
                raise SystemExit(f"{t} row {rid} has changed since the plan: plan again on a fresh copy")
        # what goes first: an INSERT OR REPLACE leaves a key both gone and added
        for t, op, rid, old, new in ch:
            if op == "gone":
                db.execute(f'DELETE FROM "{t}" WHERE rowid = ?', (rid,))
        for t, op, rid, old, new in ch:
            if op == "changed" and t != "sqlite_sequence":
                vals = json.loads(new)
                db.execute(f'UPDATE "{t}" SET ' + ",".join(f'"{c}" = ?' for c in vals) + " WHERE rowid = ?", list(vals.values()) + [rid])
        for t, op, rid, old, new in ch:
            if op == "added" and t != "sqlite_sequence":
                vals = json.loads(new)
                if db.execute(f'SELECT 1 FROM "{t}" WHERE rowid = ?', (rid,)).fetchone():
                    raise SystemExit(f"{t} row {rid} is taken since the plan: plan again on a fresh copy")
                db.execute(f'INSERT INTO "{t}" (rowid, ' + ",".join(f'"{c}"' for c in vals) + ") VALUES (?," + ",".join("?" * len(vals)) + ")", [rid] + list(vals.values()))
        # the counters: as the plan left them, in its own table (a row sqlite keeps by name, not rowid)
        for t, op, rid, old, new in ch:
            if t == "sqlite_sequence" and op in ("added", "changed"):
                v = json.loads(new)
                if db.execute("UPDATE sqlite_sequence SET seq = max(seq, ?) WHERE name = ?", (v["seq"], v["name"])).rowcount == 0:
                    db.execute("INSERT INTO sqlite_sequence (name, seq) VALUES (?, ?)", (v["name"], v["seq"]))
        log_changes(db, name, ch)
        db.execute("COMMIT")
        say(f"applied {name} from its plan: {len(ch)} rows")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db, name, say, extra=None):
    """extra(db, n), when given, runs in the same transaction before its commit."""
    if not has(db, LOG):
        raise SystemExit("nothing to undo")
    db.execute("BEGIN IMMEDIATE")
    try:
        rows = db.execute(f"SELECT seq, tbl, op, rid, old FROM {LOG} WHERE setname = ? ORDER BY seq DESC", (name,)).fetchall()
        if not rows:
            raise SystemExit(f"set {name!r} is not in")
        # what players took of it since: a made item held, a quest's progress
        made_items = [r for _, t, op, r, _ in rows if t == "itempattern" and op == "added"]
        made_quests = [r for _, t, op, r, _ in rows if t == "quests" and op == "added"]
        for rid in made_items:
            pid = db.execute("SELECT patternid FROM itempattern WHERE rowid = ?", (rid,)).fetchone()
            if pid:
                n = db.execute("DELETE FROM charinventory WHERE patternid = ?", (pid[0],)).rowcount
                if n:
                    say(f"took {n} of item {pid[0]} out of inventories")
        for rid in made_quests:
            qid = db.execute("SELECT questid FROM quests WHERE rowid = ?", (rid,)).fetchone()
            if qid and has(db, "charquests"):
                n = db.execute("DELETE FROM charquests WHERE questid = ?", (qid[0],)).rowcount
                if n:
                    say(f"took {n} characters' progress in quest {qid[0]}")
        # what was added first: an INSERT OR REPLACE leaves a key both added and gone
        for _, t, op, rid, old in rows:
            if op == "added":
                db.execute(f'DELETE FROM "{t}" WHERE rowid = ?', (rid,))
        for _, t, op, rid, old in rows:
            if op == "changed":
                vals = json.loads(old)
                if t == "sqlite_sequence" and has(db, vals["name"]):
                    # never under what another's rows have taken since
                    top = db.execute(f'SELECT max(rowid) FROM "{vals["name"]}"').fetchone()[0] or 0
                    vals["seq"] = max(vals["seq"], top)
                db.execute(f'UPDATE "{t}" SET ' + ",".join(f'"{c}" = ?' for c in vals) + " WHERE rowid = ?", list(vals.values()) + [rid])
            elif op == "gone":
                vals = json.loads(old)
                db.execute(f'INSERT INTO "{t}" (rowid, ' + ",".join(f'"{c}"' for c in vals) + ") VALUES (?," + ",".join("?" * len(vals)) + ")", [rid] + list(vals.values()))
        for _, t, op, _, _ in rows:
            if op == "made" and t != "sqlite_sequence":
                db.execute(f'DROP TABLE IF EXISTS "{t}"')
        db.execute(f"DELETE FROM {LOG} WHERE setname = ?", (name,))
        if not db.execute(f"SELECT 1 FROM {LOG}").fetchone():
            db.execute(f"DROP TABLE {LOG}")
        if extra:
            extra(db, len(rows))
        db.execute("COMMIT")
        say(f"undid {name}: {len(rows)} rows")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--set", required=True)
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    ap.add_argument("--save-plan", help="on a copy of the database: the rows it would change, to a file; nothing written")
    ap.add_argument("--plan", help="the rows of a --save-plan, written in one short transaction")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, timeout=5, isolation_level=None)
    say = lambda m: print(m)
    if a.undo:
        undo(db, a.set, say)
        return
    if a.plan:
        apply_plan(db, a.plan, say)
        return
    spec = json.load(open(os.path.join(HERE, "guidegaps", a.set + ".json")))
    spec["set"] = a.set
    apply(db, spec, a.apply, say, a.save_plan)


if __name__ == "__main__":
    main()
