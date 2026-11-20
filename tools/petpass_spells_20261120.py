#!/usr/bin/env python3
"""The spells lane of the petition pass of 2026-11-20: the data its petitions need (the code's half is on the same
branch). Seven parts, each for the shards named, each row read back before it is changed (refused, nothing written,
when a row is not as this tool found it):

  necro      (both) the necromancer's level 60 pet line. Players' petitions: "the base lvl60 nec pet labeled SWARMING
             DEATH is summoning RED SWARMING DEATH and RSD should be the pet you are granted when you unlock the last
             pet weapon CM"; "Unholy Sacrament 4 should be a higher level/better Swarming Death, not another Swarming
             Bones". Swarming Death (700823) summoned a pet named Red Swarming Death (the workbook's pet 0055): now named
             Swarming Death (its body the one the disc's Swarming Death creatures wear, as before). Unholy Sacrament 4
             (cmcatalog 6368) gave Bone Swarming Death (60) (901004), the bone swarm, which Bone Collection (6371, the
             first Pet Graphic rank) gives too: it now gives Swarming Death (60) (906301, made here: 901004's level 55
             sheet and kit with the Swarming Death's body, size and name), and Bone Collection replaces that with the
             bone swarm, then Ghoulish, Haunting and Crimson (Red Swarming Death (60), 906300) as before. A book that
             owns Unholy Sacrament 4 and no Bone Collection holding 901004 has it swapped for 906301 (its row, so its hot
             bar slot, kept).
  magepets   (both) a magician's petition: "lvl 60 Pet sums from CMs are lvl 50 . i remember them being 53". The three
             level 60 masteries' elementals (Summon Badi, Djinn and Ghilan (60): 900742, 900743, 900745) level 50 -> 53.
  searing    (both) a player's petition: Searing Aura (700813), the magician's level 60 damage shield, wore the fire
             nukes' icon (829958967); it takes the icon of its line (Smoldering .. Incinerating Aura, -960607353).
  celestial  (live)  a player's petition: no merchant sold the cleric's level 60 Celestial Healing. Its scroll (8778, the
             cleric's and the shaman's; on the Heirloom shard the Spell Archivists sell it) is stocked by every
             merchant selling the cleric's or the shaman's level 60 scrolls, at the level 60 scrolls' price (8632; it
             was 0).
  archivist  (both; the archivists' stock heirloom alone) a level 60 enchanter's petitions: the Spell Archivist gave no
             Stormlord's Touch, the Plane of Disease spell, nor Chaotic Rune (55). Stormlord's Touch (701262) is the
             enchanter's reward of the Plane of Disease's spell quest as its siblings (Hymn of Nife, Lightning Storm
             ...) are, but its classspells row said "master Animator" (no Animator rank gives it), so the grant passed
             it by, and no archivist sold its scroll (960243) as they sell its siblings': the row says eqoa-data as
             theirs, and every Spell Archivist sells the scroll. Chaotic Rune's scroll (700052) wore another art than
             the scrolls' (414194432 for -607649328), so it was no scroll: not to be scribed (bought, it did nothing),
             not granted; it takes the scrolls' art, and every race as its siblings (raceuse 1007 -> 1023).
  returnhome (both) a player's petition: Return Home "within 10-15 seconds"; the owner said do them all. Every record of
             it (1542, 1623 60 s; 2300 30 s) casts in 15 s.
  rangerbow  (both) a ranger's petition: "all ranger bow shots and cmed versions should be range 35 not 30"; the owner
             said do it as asked. The ranger's fourteen arrows and shots (Flame .. Lightning Arrow and Shot) and True
             Shot (35 on the Heirloom shard already): range 30 (16880) -> 35 (16908, the float's high half).

Report only by default; --apply in one transaction, a gm_audit line a row, log petpass_spells_1120_log; --undo exact (a
row changed since is refused and said, nothing undone). --part limits it to some parts. What reads what when: spell
records, petstats, pettemplate and cmcatalog are read at the server's start (a keep restart); a book (spells) and the
client's spell records at the next login; a merchant's wares at each look, the Spell Archivist's within 10 minutes; an
item record at the server's start.

    tools/petpass_spells_20261120.py --db eqoa.db --shard heirloom|live [--part necro,...] [--apply | --undo]
"""
import argparse
import json
import sqlite3
import sys
import time

LOG = "petpass_spells_1120_log"
DETAIL = "the spells lane of the petition pass of 2026-11-20: tools/petpass_spells_20261120.py"
PARTS = ("necro", "magepets", "searing", "celestial", "archivist", "returnhome", "rangerbow")
KEYS = {"petstats": ("spellid",), "spellpattern": ("spellid",), "spelleffect": ("spellid",),
        "classspells": ("class", "spellid"), "pettemplate": ("spell", "weapon"), "cmcatalog": ("id",),
        "spells": ("id",), "itempattern": ("patternid",), "npcitem": ("itemid",)}

SWARMING, BONE, RED, NEWSD = 700823, 901004, 906300, 906301
SD_BODY, SD_SIZE = 1851657770, 1.3
SACRAMENT4, BONECOLL = 6368, 6371
NECRO = 11
MAGE_PETS = (900742, 900743, 900745)
SEARING, NUKE_ICON, AURA_ICON = 700813, 829958967, -960607353
CELESTIAL, L60_PRICE = 8778, 8632
SCROLL_ICON = -607649328
STORMLORD, STORMLORD_SCROLL, POD_SIBLING = 701262, 960243, 960244
CHAOTIC_SCROLL, CHAOTIC_ICON = 700052, 414194432
RETURN_HOME = {1542: 60, 1623: 60, 2300: 30}
RETURN_HOME_CAST = 15
RANGE30, RANGE35 = 16880, 16908
RANGER_SHOTS = (700063, 700849, 700146, 700881, 700248, 700908, 700373, 700959, 700512, 701013, 700729, 701084,
                700795, 701170, 701247)


class Refused(Exception):
    pass


def dumps(v):
    return json.dumps(v, sort_keys=True, separators=(",", ":"), ensure_ascii=False)


def row(db, tbl, key):
    cols = KEYS[tbl]
    r = db.execute(f"SELECT * FROM {tbl} WHERE " + " AND ".join(f"{c} = ?" for c in cols), key).fetchone()
    return None if r is None else dict(r)


class Plan:
    """The changes a run makes: (part, tbl, key, before, after), before None for an insert."""

    def __init__(self, db):
        self.db = db
        self.changes = []

    def update(self, part, tbl, key, want, new):
        key = tuple(key)
        r = row(self.db, tbl, key)
        if r is None:
            raise Refused(f"{part}: {tbl} {key} is missing")
        if all(r[c] == v for c, v in new.items()):
            return  # already so
        for c, v in want.items():
            if r[c] != v:
                raise Refused(f"{part}: {tbl} {key}.{c} is {r[c]!r}, not {v!r} as this tool found it")
        after = dict(r)
        after.update(new)
        self.changes.append((part, tbl, key, r, after))

    def insert(self, part, tbl, new):
        key = tuple(new[c] for c in KEYS[tbl]) if tbl != "npcitem" else None
        if key is not None:
            r = row(self.db, tbl, key)
            if r is not None:
                if all(r.get(c) == v for c, v in new.items()):
                    return
                raise Refused(f"{part}: {tbl} {key} is there already, not as this tool makes it")
        self.changes.append((part, tbl, key, None, new))


def copy_of(db, tbl, key, **new):
    r = row(db, tbl, key)
    if r is None:
        raise Refused(f"{tbl} {key}, the row to copy, is missing")
    r.update(new)
    return r


def plan_necro(p):
    db = p.db
    p.update("necro", "petstats", (SWARMING,), {"name": "Red Swarming Death", "model": SD_BODY},
             {"name": "Swarming Death"})
    p.insert("necro", "spellpattern", copy_of(db, "spellpattern", (BONE,), spellid=NEWSD,
             spellname="Swarming Death (60)", spelldesc="CM: Summons a Swarming Death to aid you."))
    p.insert("necro", "spelleffect", copy_of(db, "spelleffect", (BONE,), spellid=NEWSD,
             source="Unholy Sacrament 4's Swarming Death (petition pass 2026-11-20): Bone Swarming Death (60)'s row"))
    p.insert("necro", "classspells", {"class": NECRO, "spellid": NEWSD, "source": "cm (petition pass 2026-11-20)"})
    p.insert("necro", "petstats", copy_of(db, "petstats", (BONE,), spellid=NEWSD, name="Swarming Death",
             model=SD_BODY, size=SD_SIZE,
             note="Unholy Sacrament 4's Swarming Death (petition pass 2026-11-20): Bone Swarming Death (60)'s sheet "
                  "(901004, the level 55 numbers and kit) with the Swarming Death's body, size and name (700823)",
             source="petition pass 2026-11-20"))
    p.insert("necro", "pettemplate", copy_of(db, "pettemplate", ("bone swarming death (60)", 0),
             spell="swarming death (60)", note="a necromancer's pet, Unholy Sacrament 4's Swarming Death (petition "
             "pass 2026-11-20): Bone Swarming Death (60)'s row, the Swarming Deaths' own template",
             source="petition pass 2026-11-20"))
    p.update("necro", "cmcatalog", (SACRAMENT4,), {"adds": "Bone Swarming Death (60)@60", "add_spells": str(BONE)},
             {"adds": "Swarming Death (60)@60", "add_spells": str(NEWSD)})
    p.update("necro", "cmcatalog", (BONECOLL,), {"removes": "", "remove_spells": "", "add_spells": str(BONE)},
             {"removes": "Swarming Death (60)@60", "remove_spells": str(NEWSD)})
    # the books: owning Unholy Sacrament 4 and no Bone Collection, holding the bone swarm (901004)
    for (rid,) in db.execute("""SELECT s.id FROM spells s WHERE s.spellid = ?
            AND EXISTS (SELECT 1 FROM charcms c WHERE c.serverid = s.serverid AND c.cmid = ?)
            AND NOT EXISTS (SELECT 1 FROM charcms c WHERE c.serverid = s.serverid AND c.cmid = ?)
            AND NOT EXISTS (SELECT 1 FROM spells t WHERE t.serverid = s.serverid AND t.spellid = ?)
            ORDER BY s.id""", (BONE, SACRAMENT4, BONECOLL, NEWSD)):
        p.update("necro", "spells", (rid,), {"spellid": BONE}, {"spellid": NEWSD})


def plan_magepets(p):
    for sid in MAGE_PETS:
        p.update("magepets", "petstats", (sid,), {"level": 50}, {"level": 53})


def plan_searing(p):
    p.update("searing", "spellpattern", (SEARING,), {"spellname": "Searing Aura", "icon": NUKE_ICON},
             {"icon": AURA_ICON})


def merchants_selling(db, classbit, level):
    return [r[0] for r in db.execute("""SELECT DISTINCT n.npcid FROM npcitem n
            JOIN (SELECT DISTINCT npc_id FROM npcs WHERE (npc_type & 1) = 1) m ON m.npc_id = n.npcid
            JOIN itempattern p ON p.patternid = n.patternid AND p.itemicon = ? AND p.equipslot = -1
            WHERE (p.classuse & ?) != 0 AND p.classuse < 32767 AND p.levelreq = ? ORDER BY n.npcid""",
                                     (SCROLL_ICON, classbit, level))]


def stock(p, part, npcids, pattern):
    for npc in npcids:
        if p.db.execute("SELECT 1 FROM npcitem WHERE npcid = ? AND patternid = ?", (npc, pattern)).fetchone():
            continue
        last = p.db.execute("SELECT COALESCE(max(listnumber), -1) FROM npcitem WHERE npcid = ?", (npc,)).fetchone()[0]
        p.insert(part, "npcitem", {"npcid": npc, "stackleft": 1, "remainHP": 0, "remaincharge": 0,
                                   "patternid": pattern, "equiploc": -1, "location": 1, "listnumber": last + 1})


def plan_celestial(p):
    p.update("celestial", "itempattern", (CELESTIAL,), {"itemname": "Celestial Healing", "itemcost": 0},
             {"itemcost": L60_PRICE})
    sellers = sorted(set(merchants_selling(p.db, 1 << 9, 60)) | set(merchants_selling(p.db, 1 << 8, 60)))
    if not sellers:
        raise Refused("celestial: no merchant sells the cleric's or the shaman's level 60 scrolls")
    stock(p, "celestial", sellers, CELESTIAL)


def plan_archivist(p, shard):
    p.update("archivist", "classspells", (12, STORMLORD), {"source": "master Animator"}, {"source": "eqoa-data"})
    p.update("archivist", "itempattern", (CHAOTIC_SCROLL,), {"itemname": "Chaotic Rune", "itemicon": CHAOTIC_ICON,
             "raceuse": 1007}, {"itemicon": SCROLL_ICON, "raceuse": 1023})
    if shard == "heirloom":
        archivists = [r[0] for r in p.db.execute("""SELECT DISTINCT n.npcid FROM npcitem n JOIN npcs m
                ON m.npc_id = n.npcid AND m.npc_name = 'Spell Archivist' AND (m.npc_type & 1) = 1
                WHERE n.patternid = ? ORDER BY n.npcid""", (POD_SIBLING,))]
        if not archivists:
            raise Refused("archivist: no Spell Archivist sells Lightning Storm's scroll (960244)")
        stock(p, "archivist", archivists, STORMLORD_SCROLL)


def plan_returnhome(p):
    for sid, cast in RETURN_HOME.items():
        p.update("returnhome", "spellpattern", (sid,), {"spellname": "Return Home", "casttime": cast},
                 {"casttime": RETURN_HOME_CAST})


def plan_rangerbow(p):
    for sid in RANGER_SHOTS:
        r = row(p.db, "spellpattern", (sid,))
        if r is not None and r["range"] == RANGE35:
            continue
        p.update("rangerbow", "spellpattern", (sid,), {"range": RANGE30}, {"range": RANGE35})


def ensure_log(db):
    db.execute(f"""CREATE TABLE IF NOT EXISTS {LOG} (
        id     INTEGER PRIMARY KEY AUTOINCREMENT,
        part   TEXT NOT NULL,
        tbl    TEXT NOT NULL,
        rowkey TEXT NOT NULL,      -- the row's key (npcitem: its itemid, given at the insert)
        before TEXT,               -- the row before (NULL: inserted)
        after  TEXT NOT NULL,      -- the row after
        audit  INTEGER NOT NULL,   -- the gm_audit line written
        undone INTEGER NOT NULL DEFAULT 0
    )""")


def audit(db, who, action, tbl, key, before, after, detail=DETAIL):
    cur = db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after,
            detail) VALUES (?, 1, ?, 'tool', '', ?, ?, ?, ?, ?, ?)""",
                     (time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), who, action, tbl, key,
                      None if before is None else dumps(before), None if after is None else dumps(after), detail))
    return cur.lastrowid


def apply(db, changes, who):
    ensure_log(db)
    for part, tbl, key, before, after in changes:
        if before is None:
            cols = list(after)
            cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                             [after[c] for c in cols])
            if tbl == "npcitem":
                after = dict(after, itemid=cur.lastrowid)
                key = (cur.lastrowid,)
            action = "insert"
        else:
            sets = [c for c in after if after[c] != before[c]]
            db.execute(f"UPDATE {tbl} SET " + ", ".join(f"{c} = ?" for c in sets) + " WHERE " +
                       " AND ".join(f"{c} = ?" for c in KEYS[tbl]), [after[c] for c in sets] + list(key))
            action = "update"
        k = dumps(dict(zip(KEYS[tbl], key)))
        a = audit(db, who, action, tbl, k, before, after)
        db.execute(f"INSERT INTO {LOG} (part, tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?, ?)",
                   (part, tbl, k, None if before is None else dumps(before), dumps(after), a))


def undo(db, parts, who):
    ensure_log(db)
    rows = db.execute(f"SELECT id, part, tbl, rowkey, before, after FROM {LOG} WHERE undone = 0 ORDER BY id DESC"
                      ).fetchall()
    rows = [r for r in rows if r["part"] in parts]
    for r in rows:
        tbl, key = r["tbl"], json.loads(r["rowkey"])
        now = row(db, tbl, tuple(key[c] for c in KEYS[tbl]))
        after = json.loads(r["after"])
        if now is None or any(now.get(c) != v for c, v in after.items()):
            raise Refused(f"undo: {tbl} {key} changed since this tool wrote it: {now}")
    for r in rows:
        tbl, key = r["tbl"], json.loads(r["rowkey"])
        where = " AND ".join(f"{c} = ?" for c in KEYS[tbl])
        vals = [key[c] for c in KEYS[tbl]]
        after = json.loads(r["after"])
        if r["before"] is None:
            db.execute(f"DELETE FROM {tbl} WHERE {where}", vals)
            a = audit(db, who, "delete", tbl, r["rowkey"], after, None, DETAIL + " (undo)")
        else:
            before = json.loads(r["before"])
            sets = [c for c in before if before[c] != after.get(c)]
            db.execute(f"UPDATE {tbl} SET " + ", ".join(f"{c} = ?" for c in sets) + f" WHERE {where}",
                       [before[c] for c in sets] + vals)
            a = audit(db, who, "update", tbl, r["rowkey"], after, before, DETAIL + " (undo)")
        db.execute(f"UPDATE {LOG} SET undone = ? WHERE id = ?", (a, r["id"]))
    return len(rows)


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    ap.add_argument("--shard", required=True, choices=("heirloom", "live"))
    ap.add_argument("--part", default=",".join(PARTS))
    ap.add_argument("--who", default="tool", help="gm_audit's username")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    parts = [x.strip() for x in a.part.split(",") if x.strip()]
    for x in parts:
        if x not in PARTS:
            sys.exit(f"unknown part {x!r}: {', '.join(PARTS)}")
    if a.shard == "heirloom" and "celestial" in parts and a.part != ",".join(PARTS):
        sys.exit("celestial is the live shard's alone")
    if a.shard == "heirloom":
        parts = [x for x in parts if x != "celestial"]
    db = sqlite3.connect(a.db)
    db.row_factory = sqlite3.Row
    try:
        with db:
            if a.undo:
                n = undo(db, parts, a.who)
                print(f"undone: {n} rows ({', '.join(parts)})")
                return
            p = Plan(db)
            for x in parts:
                if x == "archivist":
                    plan_archivist(p, a.shard)
                else:
                    globals()["plan_" + x](p)
            counts = {}
            for part, tbl, key, before, after in p.changes:
                counts.setdefault(part, {}).setdefault((tbl, "insert" if before is None else "update"), 0)
                counts[part][(tbl, "insert" if before is None else "update")] += 1
                if before is None:
                    print(f"  {part}: insert {tbl} {key or ''} {dumps({k: after[k] for k in list(after)[:6]})}")
                else:
                    diff = {c: [before[c], after[c]] for c in after if after[c] != before[c]}
                    print(f"  {part}: update {tbl} {key} {dumps(diff)}")
            for part in parts:
                c = counts.get(part, {})
                print(f"{part}: " + (", ".join(f"{n} {op} {t}" for (t, op), n in sorted(c.items())) or "nothing to do"))
            if a.apply:
                apply(db, p.changes, a.who)
                print(f"applied: {len(p.changes)} rows, log {LOG}")
            else:
                print("report only (--apply to write)")
    except Refused as e:
        sys.exit(f"refused, nothing written: {e}")


if __name__ == "__main__":
    main()
