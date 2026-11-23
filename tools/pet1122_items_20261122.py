#!/usr/bin/env python3
"""Players' petitions of 2026-11-22, the data half (Heirloom 2679, 2696, 2685, 2682, 2695, 2689, 2698; Live 5436, 5438,
5439, 5440, 5491). Every row is found by name on each shard (item and spell ids can differ) and changed only when it
reads as this tool expects; anything else refuses the whole run.

  2679  Sneak (the ranger's level 1 self stealth) was replaced by Toxophilite in the live game: off the rangers' list
        (classspells class 1), the merchant's ranger Sneak scroll off his list (npcitem), and off every ranger's book
        (spells). Toxophilite's icon: the bow and arrow (UI 0x7C84475E) for the forage cornucopia it had.
  2696  Stalk (ranger 16) was replaced by Constitution (SOE's notes of 2006-04): the same three.
  2685  Forage Berries makes three (its effects' create_item count, 1). Its foraged berries (and Red Burn Berries, the
  5436  same art) wore the bread loaf's item icon: the disc's red berries (ITEMICON -1023414165). The spell's own icon
        stays the cornucopia: UI.CSF has no berries (every forage spell wears it).
  2682  The hunt-and-gather trophies that stack to 99 (Giant Blue Pearl Ring, Cold Iron Nosering, collars, ear cuffs,
        pendants, rings, the Nixie Cap) were wearable records: not worn (equipslot -1), so they sit in Misc.
  2695  Quest loot of the imported quests (ids from 700000) that does nothing when used (no itemeffect, itemspell or
        foodeffect row, no HoT or PoT) was use type 2, which CLIENT files under Magic (the spikeshell crab leg): type 0,
        Misc. Records an open work-board entry holds (HELD) are left, the crab leg among them (entry 1996's).
  2698  The shadow knight's cleave line (Paincleave, Sorrowcleave, Bloodcleave, Doomcleave, Deathcleave) is the Soul
        Cleave mastery's area strike (EQOA-Data's SK Exemplar guide: "defile the souls of all enemies around the
        Shadowknight ... Life tap applies only to the primary target; AoE portion deals damage only"): scope target_aoe,
        radius 10, so its second hit (hp2) lands round the target, the drain and the first hit on the target alone.
  5491  The monk's level 60 epics were both PRIMARY (12): Claws of Mayhem (and its Ascended copy) SECONDARY (14, either
        hand) so both are worn; the ranger's Blade of Nature's Purity beside Blade of Nature's Innocence the same.
  5438  Crusader's Aura shared the paladin wards' stack type (2, offensive procs): a stack type of its own (its id).
  5439  Dirge of Defense raised DEX where its record and description say AGI: AGI.
  5440  (Live only) the alchemist's draughts stacked to 2000 (Infused 9999): 50. Heirloom's are left: 19 Heirloom stacks
        of Infused Draught hold more than 50 (a GM's grants), the owner's call.

  2703  Wakizashi of Toxicity (and its copies) wears the katana's model and icon (Katana of Toxicity's).

  2704  Longbow of Toxicity (and its copies) wears the longbows' model (Centaur Longbow's), not the shortbows'; Chosen's
        Rapier (and its copies) the rapiers' (Dainty Rapier's), not the dirk's: the rest of the Toxicity and Chosen's
        weapons wear their kind's.

  5499  (both shards) The thrown potions and stones had no hit points, which the inventory shows as broken (an X,
        condition 0): 200, as Freezing Potion and Freeport Cocktail have, on the records and every copy held at 0.
  2706  U'Dedne's Chalice wore the spell scroll's icon and words: the chalices' icon, a chalice's description.

The owner's decisions of 2026-11-23 (over his 2026-11-18 rule keeping epic models off non-epic items for the first
two, and over Hammer of Piety's Judgement look put back on 2026-11-19, petition 2444, for the third):
  2699  Chosen's Baton (and Ascended) take Glory of Elddar's model and icon (the enchanter's 49 staff epic).
  2701  Chosen's Zwiehander (and Ascended) take Sapphyra's Sword's model and icon.
  2700  Hammer of Piety, Hammer of Ethereal Conquest and Hammer of Mystical Enlightenment (and their Ascended copies)
        take Hammer of Divinity's model and icon (the plain warhammer), off Judgement's.
  2710  Warmonger's Club (and Ascended) take the shaman epic Cudgel of the Spirit Guide's model and icon (approved).
--looks is accepted and changes nothing (they are in every run since the owner's yes).

What takes effect when: spelleffect is read at every cast; item records at each request (a held copy shows its new
record at its holder's next login); classspells and the merchant's list within the archivist's 10 minutes; a book's
row at once (the client's book at the next login); spellpattern's icon at the next login.

Dry run by default. --apply writes in one transaction, a gm_audit line a row, every row's before and after in
pet1122_items_log; a second --apply writes nothing. --undo puts back what the log holds, latest first (a row changed
since is left and said). --panel posts the same changes to the GM panel (update_row with expect, delete_row; each
audited and revertible there) for the shard instead of writing the file: --db is then a copy read for the plan.

    python3 -E -s tools/pet1122_items_20261122.py --db eqoa.db --shard heirloom|live [--looks] [--apply | --undo]
    python3 -E -s tools/pet1122_items_20261122.py --db copy.db --shard heirloom [--looks] --panel [--post]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys
import urllib.request

LOG = "pet1122_items_log"
WHO, ACCOUNT, VIA = "dab", 1, "tool"
TOOL = "tools/pet1122_items_20261122.py"
DETAIL = f"Petitions of 2026-11-22 (items and spells): {TOOL}"

RANGER = 1
BOW_ICON, FORAGE_ICON = 2089043806, -295381203
BREAD_ICON, BERRIES_ICON = 1511586099, -1023414165
TROPHIES = ("Black Gnoll Ring", "Blood Paw Collar", "Cold Iron Nosering", "Dark Skull Ring", "Giant Blue Pearl Ring",
            "Gnoll Ear Cuff", "Nixie Cap", "Platinum Gnoll Pendant", "Serpent Fang Ring", "Witchking Collar")
CLEAVES = ("Paincleave", "Sorrowcleave", "Bloodcleave", "Doomcleave", "Deathcleave")
CLEAVE_RADIUS = 10
SECONDARY = ("Claws of Mayhem", "Ascended Claws of Mayhem", "Blade of Nature's Purity")
DRAUGHTS = ("Watery Draught", "Weak Draught", "Draught", "Bitter Draught", "Stout Draught", "Infused Draught")
DRAUGHT_STACK = 50
LOOKS = [  # (petition, item, the look's record)
    (2699, "Chosen's Baton", "Glory of Elddar"),
    (2699, "Ascended Chosen's Baton", "Glory of Elddar"),
    (2701, "Chosen's Zwiehander", "Sapphyra's Sword"),
    (2701, "Ascended Chosen's Zwiehander", "Sapphyra's Sword"),
    (2700, "Hammer of Piety", "Hammer of Divinity"),
    (2700, "Hammer of Ethereal Conquest", "Hammer of Divinity"),
    (2700, "Ascended Hammer of Ethereal Conquest", "Hammer of Divinity"),
    (2700, "Hammer of Mystical Enlightenment", "Hammer of Divinity"),
    (2700, "Ascended Hammer of Mystical Enlightenment", "Hammer of Divinity"),
    (2710, "Warmonger's Club", "Cudgel of the Spirit Guide"),
    (2710, "Ascended Warmonger's Club", "Cudgel of the Spirit Guide"),
]
QUEST_ITEMS_FROM = 700000
THROWN_HP = 200
CHALICE_DESC = "A chalice of the House of U'Dedne, carried off by Valraen V`Ghen."
# item records open work-board entries hold (2026-11-22: 1981, 1995, 1996 and others' quest rebuilds and Live mirrors,
# which compare those rows to what they read): the Misc sweep leaves them, the spikeshell crab leg (1996's) among them
HELD = {700294, 700298, 700307, 700318, 700322, 700323, 700330, 700332, 700336, 700340, 700345, 700347, 700352, 700367,
        700370, 700377, 700380, 700386, 700451, 700452, 700554, 700608, 707104, 707113, 709200, 715070}

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,     -- its key (JSON)
    before TEXT NOT NULL,     -- the whole row before (JSON)
    after  TEXT,              -- the whole row after (JSON; NULL: deleted)
    audit  INTEGER NOT NULL,
    at     TEXT NOT NULL
)"""

KEYS = {"classspells": ("class", "spellid"), "npcitem": ("itemid",), "spells": ("id",), "spellpattern": ("spellid",),
        "spelleffect": ("spellid",), "itempattern": ("patternid",), "charinventory": ("itemid",)}


class Refused(Exception):
    pass


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def dumps(v):
    return json.dumps(v, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    names = [d[0] for d in cur.description]
    return [dict(zip(names, r)) for r in cur.fetchall()]


def one(db, tbl, key):
    w = " AND ".join(f'"{c}" = ?' for c in key)
    r = rows(db, f'SELECT * FROM "{tbl}" WHERE {w}', tuple(key.values()))
    return r[0] if r else None


class Plan:
    def __init__(self, db, shard, looks):
        self.db, self.shard, self.looks = db, shard, looks
        self.ops = []  # (tbl, key, before row, values or None to delete, why)
        self.lines = []
        self.seen = set()

    def say(self, s):
        self.lines.append(f"  {s}")

    def keyof(self, tbl, row):
        return {c: row[c] for c in KEYS[tbl]}

    def update(self, tbl, row, values, why):
        key = self.keyof(tbl, row)
        values = {c: v for c, v in values.items() if row[c] != v}
        if dumps([tbl, key]) in self.seen:
            # a second change of the row (U'Dedne's Chalice: the Misc sweep, then its icon): one update, its columns
            # each changed once
            for i, (t, k, b, v, w) in enumerate(self.ops):
                if t == tbl and k == key and v is not None and not set(v) & set(values):
                    self.ops[i] = (t, k, b, {**v, **values}, f"{w}; {why}")
                    self.say(f"{why}: {tbl} {dumps(key)} " + ", ".join(f"{c} {short(row[c])} -> {short(x)}"
                                                                    for c, x in values.items()))
                    return
            raise Refused(f"{tbl} {key} changed twice")
        self.seen.add(dumps([tbl, key]))
        if not values:
            self.say(f"{why}: {tbl} {dumps(key)} already so")
            return
        self.ops.append((tbl, key, row, values, why))
        self.say(f"{why}: {tbl} {dumps(key)} " + ", ".join(f"{c} {short(row[c])} -> {short(v)}" for c, v in values.items()))

    def delete(self, tbl, row, why):
        key = self.keyof(tbl, row)
        self.seen.add(dumps([tbl, key]))
        self.ops.append((tbl, key, row, None, why))
        self.say(f"{why}: delete {tbl} {dumps(key)}")


def short(v, n=90):
    s = v if isinstance(v, str) else repr(v)
    return s if len(s) <= n else s[:n] + "..."


def item(db, name, **want):
    """The one item record of the name (exact), refusing none or several."""
    rs = rows(db, "SELECT * FROM itempattern WHERE itemname = ?", (name,))
    if len(rs) != 1:
        raise Refused(f"item {name!r}: {len(rs)} records, not one ({[r['patternid'] for r in rs]})")
    r = rs[0]
    for c, v in want.items():
        if r[c] != v and not (isinstance(v, tuple) and r[c] in v):
            raise Refused(f"item {name!r} ({r['patternid']}): {c} {r[c]!r}, not {v!r}")
    return r


def items(db, name):
    return rows(db, "SELECT * FROM itempattern WHERE itemname = ?", (name,))


def ranger_spell(db, name, level):
    rs = rows(db, "SELECT p.* FROM spellpattern p JOIN classspells c ON c.spellid = p.spellid "
                  "WHERE c.class = ? AND p.spellname = ? AND p.abilitylvl = ?", (RANGER, name, level))
    return rs


def spell(db, name, **want):
    rs = rows(db, "SELECT * FROM spellpattern WHERE spellname = ?", (name,))
    if len(rs) != 1:
        raise Refused(f"spell {name!r}: {len(rs)} records, not one ({[r['spellid'] for r in rs]})")
    for c, v in want.items():
        if rs[0][c] != v:
            raise Refused(f"spell {name!r}: {c} {rs[0][c]!r}, not {v!r}")
    return rs[0]


def effect(db, spellid):
    r = one(db, "spelleffect", {"spellid": spellid})
    if r is None or not r["effects"]:
        raise Refused(f"spelleffect {spellid}: no workbook effects")
    return r, json.loads(r["effects"])


def jsc(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


def build(p):
    db = p.db
    for t in KEYS:
        if not table(db, t):
            raise Refused(f"no table {t}")

    # 2679, 2696: Sneak and Stalk off the rangers
    for pet, name, level in ((2679, "Sneak", 1), (2696, "Stalk", 16)):
        why = f"{pet} {name}"
        sp = ranger_spell(db, name, level)
        if len(sp) > 1:
            raise Refused(f"{name}: {len(sp)} ranger rows")
        if not sp:
            p.say(f"{why}: no ranger row (done)")
        else:
            sid = sp[0]["spellid"]
            others = db.execute("SELECT group_concat(class) FROM classspells WHERE spellid = ? AND class != ?",
                                (sid, RANGER)).fetchone()[0]
            if others:
                raise Refused(f"{name} {sid}: other classes have it too ({others})")
            p.delete("classspells", one(db, "classspells", {"class": RANGER, "spellid": sid}), why)
            for r in rows(db, "SELECT s.* FROM spells s JOIN characters c ON c.serverid = s.serverid "
                              "WHERE s.spellid = ? AND c.tclass = ? ORDER BY s.id", (sid, RANGER)):
                p.delete("spells", r, f"{why} (a ranger's book)")
        for sc in items(db, name):
            if sc["equipslot"] != -1 or sc["classuse"] != 1 << RANGER:
                continue  # the scroll of the ranger's alone
            for r in rows(db, "SELECT * FROM npcitem WHERE patternid = ? ORDER BY itemid", (sc["patternid"],)):
                p.delete("npcitem", r, f"{why} (the scroll {sc['patternid']} on merchant {r['npcid']}'s list)")
    tox = spell(db, "Toxophilite", abilitylvl=1)
    if tox["icon"] not in (FORAGE_ICON, BOW_ICON):
        raise Refused(f"Toxophilite's icon {tox['icon']}, not the forage cornucopia's")
    p.update("spellpattern", tox, {"icon": BOW_ICON}, "2679 Toxophilite's icon (the bow and arrow)")

    # 2685 / 5436: Forage Berries makes three; the berries' icon
    fb = spell(db, "Forage Berries", abilitylvl=5)
    row, ef = effect(db, fb["spellid"])
    made = [e for e in ef.get("effects", []) if e.get("kind") == "create_item"]
    if len(made) != 1 or (made[0].get("item") or {}).get("name") != "foraged berries" or made[0]["item"].get("count") not in (1, 3):
        raise Refused(f"Forage Berries' effects: {row['effects'][:200]}")
    made[0]["item"]["count"] = 3
    p.update("spelleffect", row, {"effects": jsc(ef)}, "2685 Forage Berries makes 3")
    for name in ("foraged berries", "Red Burn Berries"):
        r = item(db, name, itemicon=(BREAD_ICON, BERRIES_ICON))
        p.update("itempattern", r, {"itemicon": BERRIES_ICON}, f"2685 {name}'s icon (berries)")

    # 2682: the hunt-and-gather trophies are not worn
    for name in TROPHIES:
        r = item(db, name, maxstack=99)
        if r["equipslot"] not in (-1, 1, 3, 4, 8):
            raise Refused(f"{name}: equipslot {r['equipslot']}")
        worn = db.execute("SELECT count(*) FROM charinventory WHERE patternid = ? AND equiploc != -1",
                          (r["patternid"],)).fetchone()[0]
        if worn and r["equipslot"] != -1:
            raise Refused(f"{name}: {worn} worn now")
        p.update("itempattern", r, {"equipslot": -1}, f"2682 {name} not worn (a trophy)")

    # 2695: quest loot that does nothing is Misc
    for r in rows(db, f"""SELECT p.* FROM itempattern p WHERE p.procanim = 2 AND p.equipslot = -1 AND p.patternid >= ?
            AND p.patternid IN (SELECT CAST(trim(items) AS INTEGER) FROM queststeps WHERE kind = 'loot')
            AND COALESCE(p.HoT, 0) = 0 AND COALESCE(p.PoT, 0) = 0
            AND NOT EXISTS (SELECT 1 FROM itemeffect e WHERE e.patternid = p.patternid)
            AND NOT EXISTS (SELECT 1 FROM itemspell s WHERE s.patternid = p.patternid)
            {"AND NOT EXISTS (SELECT 1 FROM foodeffect f WHERE f.patternid = p.patternid)" if table(db, "foodeffect") else ""}
            ORDER BY p.patternid""", (QUEST_ITEMS_FROM,)):
        if r["patternid"] in HELD:
            p.say(f"2695 {r['itemname']} ({r['patternid']}): held by an open work-board entry, left")
            continue
        p.update("itempattern", r, {"procanim": 0}, f"2695 {r['itemname']} (quest loot) under Misc")

    # 2698: the cleave line's area
    for name in CLEAVES:
        sp = spell(db, name)
        row, ef = effect(db, sp["spellid"])
        if ef.get("scope") not in ("target", "target_aoe") or not any(e.get("kind") == "hp2" for e in ef["effects"]):
            raise Refused(f"{name}: scope {ef.get('scope')!r}, effects {row['effects'][:200]}")
        ef["scope"] = "target_aoe"
        ef["radius"] = CLEAVE_RADIUS
        p.update("spelleffect", row, {"effects": jsc(ef)}, f"2698 {name} an area strike (radius {CLEAVE_RADIUS})")

    # 5491: one of each pair either hand
    for name in SECONDARY:
        r = item(db, name, equipslot=(12, 14))
        p.update("itempattern", r, {"equipslot": 14}, f"5491 {name} SECONDARY (either hand)")

    # 5438: Crusader's Aura a stack type of its own
    ca = spell(db, "Crusader's Aura", abilitylvl=60)
    row, ef = effect(db, ca["spellid"])
    if ef.get("stack") not in (2, ca["spellid"]):
        raise Refused(f"Crusader's Aura: stack {ef.get('stack')!r}")
    ef["stack"] = ca["spellid"]
    p.update("spelleffect", row, {"effects": jsc(ef)}, "5438 Crusader's Aura its own stack type")

    # 5439: Dirge of Defense raises AGI
    dd = spell(db, "Dirge of Defense", abilitylvl=12)
    row, ef = effect(db, dd["spellid"])
    st = [e for e in ef["effects"] if e.get("kind") == "stat" and e.get("stat") in ("dex", "agi")]
    if len(st) != 1:
        raise Refused(f"Dirge of Defense: {row['effects'][:200]}")
    st[0]["stat"] = "agi"
    p.update("spelleffect", row, {"effects": jsc(ef), "line": row["line"].replace("Increase DEX by", "Increase AGI by")},
             "5439 Dirge of Defense AGI")

    # 5440: draughts stack to 50 (Live)
    if p.shard == "live":
        for name in DRAUGHTS:
            r = item(db, name, maxstack=(2000, 9999, DRAUGHT_STACK))
            p.update("itempattern", r, {"maxstack": DRAUGHT_STACK}, f"5440 {name} stacks to {DRAUGHT_STACK}")

    # 2703: the wakizashi of toxicity wears the katana (a small katana: no record has a shorter one; its sabre's look
    # and the warrior's epic greatsword icon were not a wakizashi's)
    kat = {(r["model"], r["itemicon"]) for r in rows(db, "SELECT model, itemicon FROM itempattern WHERE itemname IN "
                                                         "('Katana of Toxicity', 'Katana of Severing')")}
    if len(kat) != 1:
        raise Refused(f"the katana's look: {kat}")
    (km, ki), = kat
    for r in rows(db, "SELECT * FROM itempattern WHERE lower(itemname) IN ('wakizashi of toxicity', "
                      "'ascended wakizashi of toxicity') ORDER BY patternid"):
        p.update("itempattern", r, {"model": km, "itemicon": ki}, f"2703 {r['itemname']} wears the katana")

    # 2704: the longbow of toxicity wore the shortbows' model (Crude Shortbow's): the longbows' commonest (crude
    # longbow, Centaur Longbow, Quicksilver longbow); the sweep of its set and the Chosen's: Chosen's Rapier wore the
    # dirk's, the rapiers' own (Stout Rapier, Dainty Rapier)
    for names, like, why in ((("longbow of toxicity", "ascended longbow of toxicity"), "Centaur Longbow", "2704"),
                             (("chosen's rapier", "ascended chosen's rapier"), "Dainty Rapier", "2704 sweep")):
        src = item(db, like)
        for r in rows(db, f"SELECT * FROM itempattern WHERE lower(itemname) IN ({', '.join('?' * len(names))}) "
                          "ORDER BY patternid", names):
            p.update("itempattern", r, {"model": src["model"], "itemicon": src["itemicon"]},
                     f"{why} {r['itemname']} wears {like}'s look")

    # 2706/2707: U'Dedne's Chalice (the quest The U'Dedne Chalice) wore the spell scroll's art and words: the
    # chalices' icon (chalice of the fallen, stone chalice, brass chalice) and a chalice's words
    ch = item(db, "U'Dedne's Chalice", equipslot=-1)
    cup = {r["itemicon"] for r in rows(db, "SELECT itemicon FROM itempattern WHERE itemname IN "
                                           "('chalice of the fallen', 'stone chalice', 'brass chalice')")}
    if len(cup) != 1:
        raise Refused(f"the chalices' icon: {cup}")
    p.update("itempattern", ch, {"itemicon": cup.pop(), "itemdesc": CHALICE_DESC}, "2706 U'Dedne's Chalice a chalice")

    # 5499: the thrown potions and stones (worn in the ranged slot, attack type 10) had no hit points: CLIENT's
    # Equipment tab flags a worn kind with none as broken (an X, condition 0). The hit points of the two that had
    # theirs (Freezing Potion, Freeport Cocktail: 200), and each copy held at 0 given them (a new one is made with
    # its record's: GiveItemNow; a stack added to keeps its own)
    full = {r["maxhp"] for r in rows(db, "SELECT maxhp FROM itempattern WHERE itemname IN "
                                         "('Freezing Potion', 'Freeport Cocktail')")}
    if full != {THROWN_HP}:
        raise Refused(f"the thrown potions' hit points: {full}")
    for r in rows(db, "SELECT * FROM itempattern WHERE equipslot = 16 AND attacktype = 10 AND maxhp <= 0 "
                      "ORDER BY patternid"):
        p.update("itempattern", r, {"maxhp": THROWN_HP}, f"5499 {r['itemname']} {THROWN_HP} hit points")
        for h in rows(db, "SELECT * FROM charinventory WHERE patternid = ? AND remainHP <= 0 ORDER BY itemid",
                      (r["patternid"],)):
            p.update("charinventory", h, {"remainHP": THROWN_HP}, f"5499 {r['itemname']} held (serverid {h['serverid']})")

    # 2699, 2700, 2701: looks, the owner's yes of 2026-11-23
    if True:
        for pet, name, look in LOOKS:
            got = items(db, name)
            if not got:
                p.say(f"{pet} {name}: no record on this shard")
                continue
            src = [r for r in items(db, look) if r["equipslot"] in (12, 14, 15, 18)]
            if len({(r["model"], r["itemicon"]) for r in src}) != 1:
                raise Refused(f"{look}: {len(src)} hand records, not one look")
            for r in got:
                p.update("itempattern", r, {"model": src[0]["model"], "itemicon": src[0]["itemicon"]},
                         f"{pet} {name} {r['patternid']} wears {look}'s look")


# ---------------------------------------------------------------------------------------------- writing

def audit(db, action, tbl, key, before, after):
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (now(), ACCOUNT, WHO, VIA, action, tbl, dumps(key), None if before is None else dumps(before),
                      None if after is None else dumps(after), DETAIL))
    return cur.lastrowid


def apply(db, p):
    db.execute(LOG_DDL)
    at = now()
    for tbl, key, before, values, _ in p.ops:
        w = " AND ".join(f'"{c}" = ?' for c in key)
        if values is None:
            db.execute(f'DELETE FROM "{tbl}" WHERE {w}', tuple(key.values()))
            after = None
            action = "delete"
        else:
            db.execute(f'UPDATE "{tbl}" SET {", ".join(chr(34) + c + chr(34) + " = ?" for c in values)} WHERE {w}',
                       (*values.values(), *key.values()))
            after = one(db, tbl, key)
            action = "update"
        a = audit(db, action, tbl, key, before, after)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?)",
                   (tbl, dumps(key), dumps(before), None if after is None else dumps(after), a, at))
    return len(p.ops)


def undo(db):
    if not table(db, LOG):
        return 0, []
    back, left = 0, []
    for seq, tbl, rowkey, before, after in db.execute(
            f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key, b = json.loads(rowkey), json.loads(before)
        have = one(db, tbl, key)
        if (after is None and have is not None) or (after is not None and (have is None or dumps(have) != after)):
            left.append(f"{tbl} {rowkey}: changed since, left as it is")
            continue
        if after is None:
            cs = list(b)
            db.execute(f'INSERT INTO "{tbl}" ({", ".join(chr(34) + c + chr(34) for c in cs)}) '
                       f'VALUES ({", ".join("?" * len(cs))})', [b[c] for c in cs])
            action = "insert"
        else:
            w = " AND ".join(f'"{c}" = ?' for c in key)
            db.execute(f'UPDATE "{tbl}" SET {", ".join(chr(34) + c + chr(34) + " = ?" for c in b)} WHERE {w}',
                       (*b.values(), *key.values()))
            action = "update"
        audit(db, action, tbl, key, have, b)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if not db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


# ---------------------------------------------------------------------------------------------- the panel

def panel_requests(p):
    """Each change as a GM panel op: (op, body)."""
    out = []
    for tbl, key, before, values, _ in p.ops:
        body = {"table": tbl, "key": key, "shard": p.shard}
        if values is None:
            out.append(("delete_row", body))
        else:
            body.update(values=values, expect={c: before[c] for c in values})
            out.append(("update_row", body))
    return out


def post(base, token, op, body):
    req = urllib.request.Request(f"{base}/api/op/{op}", data=json.dumps(body).encode(), method="POST",
                                 headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            return r.status, r.read().decode()
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--shard", required=True, choices=("heirloom", "live"))
    ap.add_argument("--looks", action="store_true", help="no change: the looks are in every run (the owner's yes, 2026-11-23)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    g.add_argument("--panel", action="store_true", help="the changes as GM panel ops (with --post, sent)")
    ap.add_argument("--post", action="store_true")
    ap.add_argument("--base", default="https://gm.eqoaclassic.com")
    ap.add_argument("--token", default=os.path.expanduser("~/.config/eqoa-gm-token"))
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("PRAGMA busy_timeout = 5000")
    db.execute("BEGIN IMMEDIATE" if (a.apply or a.undo) else "BEGIN")
    try:
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"undone: {back} rows put back" + "".join(f"\n  left: {w}" for w in left))
            return 0
        p = Plan(db, a.shard, a.looks)
        try:
            build(p)
        except Refused as e:
            db.execute("ROLLBACK")
            print("\n".join(p.lines))
            print(f"REFUSED, nothing written: {e}")
            return 1
        print(f"{a.shard}:")
        print("\n".join(p.lines))
        counts = {}
        for tbl, _, _, v, _ in p.ops:
            k = f"{'delete' if v is None else 'update'} {tbl}"
            counts[k] = counts.get(k, 0) + 1
        print("totals: " + (", ".join(f"{k} {v}" for k, v in sorted(counts.items())) or "nothing"))
        if a.panel:
            db.execute("ROLLBACK")
            reqs = panel_requests(p)
            if not a.post:
                for op, body in reqs:
                    print(op, json.dumps(body)[:300])
                print("panel: nothing sent (--post to send)")
                return 0
            token = open(a.token).read().strip()
            bad = 0
            for op, body in reqs:
                code, text = post(a.base, token, op, body)
                if code != 200:
                    bad += 1
                    print(f"  {op} {body['table']} {dumps(body['key'])}: {code} {text[:300]}")
            print(f"panel: {len(reqs) - bad} of {len(reqs)} changes made")
            return 1 if bad else 0
        if not a.apply:
            db.execute("ROLLBACK")
            print("dry run: nothing written (--apply to write)")
            return 0
        if not p.ops:
            db.execute("ROLLBACK")
            print("nothing to write: all in place")
            return 0
        n = apply(db, p)
        db.execute("COMMIT")
        print(f"applied: {n} rows, a gm_audit line each, logged in {LOG}")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
