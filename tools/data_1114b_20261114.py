#!/usr/bin/env python3
"""Four of the owner's asks of 2026-11-14, one tool (--only drakes,necklace,earthpets,heirloom; default all).

drakes  "im seeing emerald drake in Isle of dread but should not be": every emerald drake row on the Isle of Dread
        (world 0, x 23800-26200, z 2000-4200: the island's creatures stand at x 24179-25878, z 2225-3825, nothing
        else of ours there) is deleted, with any npcrespawn / npcwander / npcpaths / npcstands / npcrare /
        campspawns / npcitem row of it (none today: wildspread 2026-11-01 added them as plain rows; its own
        wildspread_adds* history rows are left, as a GM's in-game /delete leaves them). Their loot is by name
        (npcloot), shared with the drakes elsewhere in Tunaria: untouched. Live: 31 rows; Heirloom: 1 left (the GM
        forestwood /deleted 30 drakes and 36 dire worgs there in the game, 01:59-07:08 UTC). The dire worgs stay
        (the owner named only the drakes): their count and places are reported.

necklace "the diamond necklace for the lvl 25 quest should not drop from mobs, only bought from the quest mob":
        report only. No quest of either shard takes or gives a diamond necklace; the one "diamond necklace" record
        (7962, level 50) is in no loot table, pool, NPC's inventory or merchant's stock, so nothing drops it now.

earthpets "lets make mag earth pets tank better than necro pet of the same tier": a summoned pet's hit points are
        its petstats hp x balancelevel.pet_hp_pct (owner's level; the same for both classes) x
        balanceclasslevel.pet_hp_pct (owner's class and level). A creature's melee hit on a workbook pet is its
        roll less the row's AC and AGI by archetype (entity/petstats.go sheetHitOn), and every earth and
        necromancer pet's AC is so far above a creature's top of its owner's level that every hit lands at the
        creature's floor, (level+1)/2, for both: AC changes nothing, hit points decide. A necromancer's pet heals
        itself with its lifetaps (Drain HP by N, + the INT share): one minute's taps are counted to it.
        For each earth tier (its summoning's level to the next earth tier's, 60 the last), against the
        necromancer's best pet the owner has at each of those levels (by those effective hit points):
          - petstats.hp raised so the earth pet has 18% more effective hit points than the necromancer's at
            EVERY owner level of its range (never lowered). The class table is not touched (every magician pet
            shares it); since it is jagged (fit per level to the magician's best pet), the margin at some levels
            of a range is larger than 18%.
          - wpn1_dmg / wpn2_dmg lowered to the least the necromancer's compared pets have in its range where the
            earth pet's is higher (never raised): it stays the tank, not the bigger hitter.
        The Earth Elementaling (632, owner levels 1-15) only with --elementaling: to clear the necromancer's pets
        at 8-15 its sheet would need ~13x its hit points, which at owner levels 1-7 makes it 2-12x the
        necromancer's (the magician's class row is fit to its water, fire and air pets there).

heirloom "lets make the scimitar that zerkziee has a heirloom weapon for rangers" (Heirloom shard only; Live has no
        heirlooms): a new record, Heirloom Ykesha, copied from the ranger's Heirloom Scimitar of Fury (714027: no
        trade, worth nothing, level 1, ranger only, its slot) with the look Zerkziee's scimitar has (714385: the
        Triad Scimitar's model 0xD03942DE and icon), its heirlooms row (ranger, main) with epic 996023, Heroic
        Ykesha: from level 60 the wearer's view is Heroic Ykesha's figures and its proc (an heirloom's proc is its
        epic's, by the epic's name and description: "Procs 160 damage"), below it the best ranger main hand of the
        wearer's level as every heirloom. The name keeps Zerkziee's Ykesha's identity; its own description names
        no proc, so below 60 it procs nothing (as every heirloom). Sold by every Heirloom Outfitter who sells
        714027 (76), at the end of its list. Zerkziee keeps his 714385.

Read at the server's start (npcs, petstats, itempattern, heirlooms, npcitem): a restart. A pet out at the time keeps
its numbers until summoned again. Report by default; --apply in one transaction, a gm_audit line a row, logged in
data_1114b_log; a second --apply finds nothing to do; --undo puts back exactly what was there, a row changed since
left and listed.

    python3 -E -s tools/data_1114b_20261114.py --db eqoa.db [--apply [--elementaling] | --undo] [--only a,b]
"""
import argparse
import datetime
import json
import math
import re
import sqlite3
import sys

LOG = "data_1114b_log"
WHO, VIA = "dab", "tool"
DETAIL = {
    "drakes": "the owner (2026-11-14): no emerald drakes on the Isle of Dread: tools/data_1114b_20261114.py",
    "earthpets": "the owner (2026-11-14): magician earth pets tank better than the necromancer's pet of the tier: "
                 "tools/data_1114b_20261114.py",
    "heirloom": "the owner (2026-11-14): Zerkziee's scimitar a ranger heirloom (Heirloom Ykesha): tools/data_1114b_20261114.py",
}
PARTS = ("drakes", "necklace", "earthpets", "heirloom")

IOD = (0, 23800, 26200, 2000, 4200)  # world, x, z
DRAKE = "emerald drake"
NPC_REFS = [("npcrespawn", "npc_row"), ("npcwander", "npcid"), ("npcpaths", "npcid"), ("npcstands", "npc_row"),
            ("npcrare", "npc_row"), ("campspawns", "npcid"), ("npcitem", "npcid")]

MAGE, NECRO = 10, 11
MARGIN = 1.18
ELEMENTALING = 632

HEIR_BASE, HEIR_LOOK, HEIR_EPIC = 714027, 714385, 996023
HEIR_NAME = "Heirloom Ykesha"
HEIR_DESC = ("Zerkziee's scimitar, in the Triad Scimitar's look. Heirloom gear for testing: always the best of its kind "
             "your class can wield at your level, and from level 60 Heroic Ykesha itself.")
TRIAD_MODEL = -801553698  # 0xD03942DE

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    part   TEXT NOT NULL,
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,
    before TEXT,
    after  TEXT,
    audit  INTEGER NOT NULL DEFAULT 0
)"""


class Refused(Exception):
    pass


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False, sort_keys=True)


def cols(db, t):
    return [r[1] for r in db.execute(f'PRAGMA table_info("{t}")')]


def rowdict(db, t, where, args):
    c = cols(db, t)
    rs = db.execute(f'SELECT {", ".join(chr(34) + x + chr(34) for x in c)} FROM "{t}" WHERE {where}', args).fetchall()
    return [dict(zip(c, r)) for r in rs]


def audit(db, part, action, tbl, key, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, compact(key), compact(before or {}), compact(after or {}),
                      DETAIL[part]))
    return cur.lastrowid


def logrow(db, part, tbl, key, before, after, a):
    db.execute(f"INSERT INTO {LOG} (part, tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?, ?)",
               (part, tbl, compact(key), None if before is None else compact(before), None if after is None else compact(after), a))


# ---------------------------------------------------------------- drakes

def in_iod(where_prefix=""):
    w, x0, x1, z0, z1 = IOD
    return (f"{where_prefix}world = {w} AND {where_prefix}x_coord BETWEEN {x0} AND {x1} "
            f"AND {where_prefix}z_coord BETWEEN {z0} AND {z1}")


def plan_drakes(db, lines):
    rows = rowdict(db, "npcs", f"npc_name = ? AND {in_iod()}", (DRAKE,))
    changes = []
    for r in rows:
        refs = []
        for t, c in NPC_REFS:
            if table(db, t) and c in cols(db, t):
                for rr in rowdict(db, t, f'"{c}" = ?', (r["id"],)):
                    refs.append((t, c, rr))
        changes.append((r, refs))
    nref = sum(len(x[1]) for x in changes)
    lines.append(f"drakes: {len(rows)} emerald drake rows on the Isle of Dread to delete, {nref} rows naming them")
    for r, refs in changes:
        lines.append(f"  npcs {r['id']} L{r['npc_level']} ({r['x_coord']:.0f}, {r['z_coord']:.0f})" +
                     "".join(f"; {t} row" for t, _, _ in refs))
    worgs = db.execute(f"SELECT id, npc_level, x_coord, z_coord FROM npcs WHERE npc_name = 'dire worg' AND {in_iod()} "
                       "ORDER BY x_coord").fetchall()
    lines.append(f"drakes: dire worgs on the island, left: {len(worgs)}" +
                 (" at " + ", ".join(f"{i} L{lv} ({x:.0f},{z:.0f})" for i, lv, x, z in worgs) if worgs else ""))
    return changes


def apply_drakes(db, changes):
    n = 0
    for r, refs in changes:
        for t, c, rr in refs:
            where = " AND ".join(f'"{k}" IS ?' for k in rr)
            if db.execute(f'DELETE FROM "{t}" WHERE {where}', tuple(rr.values())).rowcount != 1:
                raise Refused(f"{t} row of npcs {r['id']} changed under the delete")
            logrow(db, "drakes", t, {c: r["id"]}, rr, None, audit(db, "drakes", "delete", t, {c: r["id"]}, rr, None))
            n += 1
        if db.execute("DELETE FROM npcs WHERE id = ? AND npc_name = ?", (r["id"], DRAKE)).rowcount != 1:
            raise Refused(f"npcs {r['id']} changed under the delete")
        logrow(db, "drakes", "npcs", {"id": r["id"]}, r, None, audit(db, "drakes", "delete", "npcs", {"id": r["id"]}, r, None))
        n += 1
    return n


# ---------------------------------------------------------------- necklace

def plan_necklace(db, lines):
    ps = db.execute("SELECT patternid, levelreq FROM itempattern WHERE lower(itemname) = 'diamond necklace'").fetchall()
    for pid, lv in ps:
        srcs = []
        for t, c in (("loottables", "patternid"), ("lootpool", "patternid"), ("npcitem", "patternid")):
            if table(db, t):
                k = db.execute(f"SELECT COUNT(*) FROM {t} WHERE {c} = ?", (pid,)).fetchone()[0]
                if k:
                    srcs.append(f"{t} {k}")
        qs = db.execute("SELECT DISTINCT questid FROM queststeps WHERE (',' || replace(items || ',' || rewarditems || ',' || "
                        "events || choices || byclass, ' ', '') || ',') GLOB ?", (f"*[^0-9]{pid}[^0-9]*",)).fetchall()
        lines.append(f"necklace: diamond necklace {pid} (level {lv}): drop/sale sources: {', '.join(srcs) or 'none'}; "
                     f"quests naming it: {', '.join(str(x[0]) for x in qs) or 'none'} (nothing to change)")
    if not ps:
        lines.append("necklace: no diamond necklace record")
    return []


# ---------------------------------------------------------------- earth pets

ARCH = {"tank": (0.305, 0.0), "melee": (0.227, 15.1), "priest": (0.18, 23.4), "caster": (0.133, 31.7)}


def interp(rows, level, default=100.0):
    if not rows:
        return default
    for i, (lv, v) in enumerate(rows):
        if lv >= level:
            if lv == level or i == 0:
                return v
            a = rows[i - 1]
            return a[1] + (v - a[1]) * (level - a[0]) / (lv - a[0])
    return rows[-1][1]


def mean_hit(level, ac, agi, typ):
    """A creature of the level's mean melee hit on a workbook pet (entity/defence.go creatureRoll, petstats.go sheetHitOn)."""
    lf = float(max(level, 1))
    top = 182.6 * (lf / 45) ** 1.52
    width = int(math.floor(138 * (lf / 45) ** 0.81 + 0.5))
    slope = min(max(0.1256 - 0.00119 * (lf - 45), 0.08), 0.2)
    floor = (level + 1) // 2
    s, off = ARCH.get(typ, ARCH["tank"])
    t = int(math.floor(top - slope * ac - s * agi + off + 0.5))
    return sum(max(h, floor, 1) for h in range(t - width, t + 1)) / (width + 1)


def heal_minute(db, abilities, intel):
    h = 0.0
    for a in json.loads(abilities or "[]"):
        if a.get("kind") != "lifetap" or not a.get("spell_id"):
            continue
        r = db.execute("SELECT line FROM spelleffect WHERE spellid = ?", (a["spell_id"],)).fetchone()
        m = re.search(r"Drain HP by (\d+)", r[0]) if r else None
        if not m:
            continue
        amt = float(m.group(1))
        m2 = re.search(r"Increase (\d+)% based on INT", r[0])
        if m2:
            amt += int(m2.group(1)) / 100 * intel
        h += amt * max(1, int(60 // float(a.get("every") or 60)))
    return h


def class_pets(db, cls, tank_only):
    q = ("SELECT p.spellid, p.name, sp.abilitylvl, p.type, p.hp, p.ac, p.agi, p.wpn1_dmg, p.wpn2_dmg, p.abilities, "
         "p.intelligence FROM petstats p JOIN classspells c ON c.spellid = p.spellid "
         "JOIN spellpattern sp ON sp.spellid = p.spellid WHERE c.class = ? AND p.type <> 'non-combat' AND p.name <> 'Construct'")
    out = []
    for r in db.execute(q, (cls,)):
        sid, name, al, typ, hp, ac, agi, w1, w2, ab, intel = r
        if tank_only and not (typ == "tank" and db.execute("SELECT element FROM petstats WHERE spellid = ?", (sid,)).fetchone()[0] == "earth"):
            continue
        out.append(dict(sid=sid, name=name, al=al, typ=typ, hp=hp, ac=ac, agi=agi, w1=w1, w2=w2,
                        heal=heal_minute(db, ab, intel)))
    return sorted(out, key=lambda p: (p["al"], p["sid"]))


def pet_ehp(p, hp, level, bl, cl):
    mx = hp * interp(bl, level) / 100 * interp(cl, level) / 100
    return (mx + p["heal"]) / mean_hit(level, p["ac"], p["agi"], p["typ"]), mx


def earth_table(db, hp_of=None):
    bl = db.execute("SELECT level, pet_hp_pct FROM balancelevel ORDER BY level").fetchall()
    cls = {c: db.execute("SELECT level, pet_hp_pct FROM balanceclasslevel WHERE class = ? ORDER BY level", (c,)).fetchall()
           for c in (MAGE, NECRO)}
    earth, necro = class_pets(db, MAGE, True), class_pets(db, NECRO, False)
    tiers = []
    for i, e in enumerate(earth):
        nxt = next((x["al"] for x in earth[i + 1:] if x["al"] > e["al"]), 61)
        tiers.append((e, range(max(e["al"], 1), nxt)))
    rows = []
    for e, rng in tiers:
        for lv in rng:
            have = [n for n in necro if n["al"] <= lv]
            if not have:
                continue
            nb = max(have, key=lambda n: (pet_ehp(n, n["hp"], lv, bl, cls[NECRO])[0], n["sid"]))
            n_ehp, n_hp = pet_ehp(nb, nb["hp"], lv, bl, cls[NECRO])
            ehp_hp = (hp_of or {}).get(e["sid"], e["hp"])
            e_ehp, e_hp = pet_ehp(e, ehp_hp, lv, bl, cls[MAGE])
            # the sheet hp that gives MARGIN x the necromancer's
            per = interp(bl, lv) / 100 * interp(cls[MAGE], lv) / 100
            need = math.ceil((MARGIN * n_ehp * mean_hit(lv, e["ac"], e["agi"], e["typ"]) - e["heal"]) / per)
            rows.append(dict(level=lv, e=e, n=nb, e_hp=e_hp, e_ehp=e_ehp, n_hp=n_hp, n_ehp=n_ehp, need=need))
    return tiers, rows


def plan_earthpets(db, lines, elementaling):
    tiers, rows = earth_table(db)
    changes = []
    for e, rng in tiers:
        rs = [r for r in rows if r["e"]["sid"] == e["sid"]]
        if not rs:
            continue
        if e["sid"] == ELEMENTALING and not elementaling:
            lines.append(f"earthpets: {e['name']} {e['sid']} (owner {rng.start}-{rng.stop - 1}) left (--elementaling): "
                         f"its sheet would need {max(r['need'] for r in rs)} hit points, now {e['hp']}")
            continue
        need = max(r["need"] for r in rs)
        new = {}
        if need > e["hp"]:
            new["hp"] = need
        w1cap = min(r["n"]["w1"] for r in rs)
        if e["w1"] > w1cap:
            new["wpn1_dmg"] = w1cap
        if e["w2"] > 0:
            w2s = [r["n"]["w2"] for r in rs if r["n"]["w2"] > 0]
            w2cap = min(w2s) if w2s else 0
            if w2s and e["w2"] > w2cap:
                new["wpn2_dmg"] = w2cap
        if new:
            before = {k: {"hp": e["hp"], "wpn1_dmg": e["w1"], "wpn2_dmg": e["w2"]}[k] for k in new}
            changes.append((e, before, new))
            lines.append(f"earthpets: {e['name']} {e['sid']} (owner {rng.start}-{rng.stop - 1}): " +
                         ", ".join(f"{k} {before[k]} -> {v}" for k, v in new.items()))
        else:
            lines.append(f"earthpets: {e['name']} {e['sid']} (owner {rng.start}-{rng.stop - 1}): already the better tank")
    return changes


def earth_report(db, lines, title):
    _, rows = earth_table(db)
    lines.append(f"earthpets {title}: owner level | earth pet hp, eff. | necromancer's pet hp+taps, eff. | ratio")
    for r in rows:
        lines.append(f"  {r['level']:2d} | {r['e']['name']} {r['e']['sid']} {r['e_hp']:.0f}, {r['e_ehp']:.0f} | "
                     f"{r['n']['name']} {r['n']['sid']} {r['n_hp']:.0f}+{r['n']['heal']:.0f}, {r['n_ehp']:.0f} | "
                     f"{r['e_ehp'] / r['n_ehp']:.2f}")


def apply_earthpets(db, changes):
    n = 0
    for e, before, new in changes:
        sets = ", ".join(f"{k} = ?" for k in new)
        conds = " AND ".join(f"{k} = ?" for k in before)
        if db.execute(f"UPDATE petstats SET {sets} WHERE spellid = ? AND {conds}",
                      (*new.values(), e["sid"], *before.values())).rowcount != 1:
            raise Refused(f"petstats {e['sid']} changed under the update")
        key = {"spellid": e["sid"]}
        logrow(db, "earthpets", "petstats", key, before, new, audit(db, "earthpets", "update", "petstats", key, before, new))
        n += 1
    return n


# ---------------------------------------------------------------- heirloom

def plan_heirloom(db, lines):
    if not table(db, "heirlooms") or db.execute("SELECT COUNT(*) FROM heirlooms").fetchone()[0] == 0:
        lines.append("heirloom: this shard has no heirlooms (Live): nothing")
        return None
    have = db.execute("SELECT patternid FROM itempattern WHERE itemname = ?", (HEIR_NAME,)).fetchall()
    if have:
        pid = have[0][0]
        if db.execute("SELECT 1 FROM heirlooms WHERE patternid = ?", (pid,)).fetchone():
            lines.append(f"heirloom: {HEIR_NAME} {pid} already")
            return None
        raise Refused(f"an itempattern named {HEIR_NAME} ({pid}) that is no heirloom")
    base = rowdict(db, "itempattern", "patternid = ?", (HEIR_BASE,))
    look = rowdict(db, "itempattern", "patternid = ?", (HEIR_LOOK,))
    epic = db.execute("SELECT itemname, levelreq FROM itempattern WHERE patternid = ?", (HEIR_EPIC,)).fetchone()
    hb = db.execute("SELECT class, role, epic FROM heirlooms WHERE patternid = ?", (HEIR_BASE,)).fetchone()
    if not base or base[0]["itemname"] != "Heirloom Scimitar of Fury" or hb != (1, "main", 701006):
        raise Refused(f"the ranger's Heirloom Scimitar of Fury {HEIR_BASE} not as found: {base and base[0]['itemname']} {hb}")
    if not look or look[0]["itemname"] != "Heroic Ykesha" or look[0]["model"] != TRIAD_MODEL:
        raise Refused(f"Zerkziee's scimitar {HEIR_LOOK} not as found")
    if epic != ("Heroic Ykesha", 60):
        raise Refused(f"Heroic Ykesha {HEIR_EPIC} not as found: {epic}")
    pid = db.execute("SELECT MAX(patternid) FROM itempattern WHERE patternid BETWEEN 714000 AND 714999").fetchone()[0] + 1
    if pid > 714999 or db.execute("SELECT 1 FROM itempattern WHERE patternid = ?", (pid,)).fetchone():
        raise Refused(f"no free heirloom id ({pid})")
    rec = dict(base[0])
    rec.update(patternid=pid, itemname=HEIR_NAME, itemdesc=HEIR_DESC, model=look[0]["model"], itemicon=look[0]["itemicon"],
               color=look[0]["color"])
    sellers = db.execute("SELECT npcid, stackleft, remainHP, remaincharge, equiploc, location FROM npcitem WHERE patternid = ? "
                         "ORDER BY npcid", (HEIR_BASE,)).fetchall()
    stock = []
    for npcid, sl, rh, rc, eq, loc in sellers:
        last = db.execute("SELECT MAX(listnumber) FROM npcitem WHERE npcid = ?", (npcid,)).fetchone()[0] or 0
        stock.append(dict(npcid=npcid, stackleft=sl, remainHP=rh, remaincharge=rc, patternid=pid, equiploc=eq, location=loc,
                          listnumber=last + 1))
    lines.append(f"heirloom: {HEIR_NAME} {pid} (copy of {HEIR_BASE}, look of {HEIR_LOOK}, classuse {rec['classuse']}), "
                 f"heirlooms (1, main, epic {HEIR_EPIC} Heroic Ykesha), sold by {len(stock)} Heirloom Outfitters")
    return rec, stock


def apply_heirloom(db, plan):
    rec, stock = plan
    c = list(rec)
    db.execute(f'INSERT INTO itempattern ({", ".join(chr(34) + x + chr(34) for x in c)}) VALUES ({", ".join("?" * len(c))})',
               tuple(rec.values()))
    key = {"patternid": rec["patternid"]}
    logrow(db, "heirloom", "itempattern", key, None, rec, audit(db, "heirloom", "insert", "itempattern", key, None, rec))
    h = {"patternid": rec["patternid"], "class": 1, "role": "main", "epic": HEIR_EPIC}
    db.execute("INSERT INTO heirlooms (patternid, class, role, epic) VALUES (?, ?, ?, ?)", tuple(h.values()))
    logrow(db, "heirloom", "heirlooms", key, None, h, audit(db, "heirloom", "insert", "heirlooms", key, None, h))
    n = 2
    for s in stock:
        cur = db.execute("INSERT INTO npcitem (npcid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber) "
                         "VALUES (?, ?, ?, ?, ?, ?, ?, ?)", tuple(s.values()))
        row = dict(itemid=cur.lastrowid, **s)
        k = {"itemid": cur.lastrowid}
        logrow(db, "heirloom", "npcitem", k, None, row, audit(db, "heirloom", "insert", "npcitem", k, None, row))
        n += 1
    return n


# ---------------------------------------------------------------- undo

def undo(db, only):
    if not table(db, LOG):
        return 0, []
    back, left = 0, []
    for seq, part, tbl, key, before, after in db.execute(
            f"SELECT seq, part, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        if part not in only:
            continue
        key = json.loads(key)
        before = None if before is None else json.loads(before)
        after = None if after is None else json.loads(after)
        ok = False
        if after is None:  # a delete: put it back, where nothing took its key since
            c = list(before)
            try:
                db.execute(f'INSERT INTO "{tbl}" ({", ".join(chr(34) + x + chr(34) for x in c)}) VALUES ({", ".join("?" * len(c))})',
                           tuple(before.values()))
                ok = True
            except sqlite3.IntegrityError:
                ok = False
        elif before is None:  # an insert: take it out if as written (a record only when nothing holds it)
            held = 0
            if tbl == "itempattern":
                for t in ("charinventory", "npcitem", "auction_lots"):
                    if table(db, t) and "patternid" in cols(db, t):
                        held += db.execute(f"SELECT COUNT(*) FROM {t} WHERE patternid = ?", (key["patternid"],)).fetchone()[0]
            if not held:
                where = " AND ".join(f'"{k}" IS ?' for k in after)
                ok = db.execute(f'DELETE FROM "{tbl}" WHERE {where}', tuple(after.values())).rowcount == 1
        else:  # an update
            sets = ", ".join(f"{k} = ?" for k in before)
            conds = " AND ".join(f"{k} = ?" for k in list(key) + list(after))
            ok = db.execute(f'UPDATE "{tbl}" SET {sets} WHERE {conds}',
                            (*before.values(), *key.values(), *after.values())).rowcount == 1
        if not ok:
            left.append(f"{tbl} {key}")
            continue
        audit(db, part, "insert" if after is None else "delete" if before is None else "update", tbl, key, after, before)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if db.execute(f"SELECT COUNT(*) FROM {LOG}").fetchone()[0] == 0:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    ap.add_argument("--elementaling", action="store_true", help="the Earth Elementaling (632) too (see above)")
    ap.add_argument("--only", default=",".join(PARTS))
    ap.add_argument("--table", action="store_true", help="print the earth/necromancer table (before, and after an --apply)")
    a = ap.parse_args()
    only = [p.strip() for p in a.only.split(",") if p.strip()]
    if any(p not in PARTS for p in only):
        sys.exit(f"--only: {only}; parts are {PARTS}")
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    lines = []
    try:
        db.execute("BEGIN IMMEDIATE")
        if a.undo:
            back, left = undo(db, only)
            db.execute("COMMIT")
            print(f"undo: {back} rows put back" + ("; left (changed since): " + ", ".join(left) if left else ""))
            return
        plans = {}
        if "drakes" in only:
            plans["drakes"] = plan_drakes(db, lines)
        if "necklace" in only:
            plan_necklace(db, lines)
        if "earthpets" in only:
            if a.table:
                earth_report(db, lines, "now")
            plans["earthpets"] = plan_earthpets(db, lines, a.elementaling)
        if "heirloom" in only:
            plans["heirloom"] = plan_heirloom(db, lines)
        print("\n".join(lines))
        if not a.apply:
            db.execute("ROLLBACK")
            print("report only (--apply to write)")
            return
        db.execute(LOG_SCHEMA)
        n = 0
        if plans.get("drakes"):
            n += apply_drakes(db, plans["drakes"])
        if plans.get("earthpets"):
            n += apply_earthpets(db, plans["earthpets"])
        if plans.get("heirloom"):
            n += apply_heirloom(db, plans["heirloom"])
        if n == 0 and db.execute(f"SELECT COUNT(*) FROM {LOG}").fetchone()[0] == 0:
            db.execute(f"DROP TABLE {LOG}")
        if a.table and "earthpets" in only:
            out = []
            earth_report(db, out, "after")
            print("\n".join(out))
        db.execute("COMMIT")
        print(f"applied: {n} rows written" if n else "nothing to do")
    except Refused as e:
        db.execute("ROLLBACK")
        sys.exit(f"REFUSED, nothing written: {e}")
    except Exception:
        db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    main()
