#!/usr/bin/env python3
"""Every city's level 20 class quest gives the guides' choice of two paths, each with its weapon (petitions 5071 and
5072, Smoke, Live, 2026-11-14: "20 sk freeport quest only gave me bloodwasp but no weapon for quest", "20 epic quest sk
and necro only gave bloodwasp and bloodgale for necro no choice of other path").

The sweep (all 77 level 20 quests of the 13 cities, both shards): Freeport's ten, Grobb's three and Oggok's four gave
one path (below); every other city's already asked a pick (scrolls) but gave no weapon. Each pick's paths get the
class's level 20 weapon of that path, as the guides give them ("Power Dance ... and a Slight Rapier", "Bellow ... and
a one-handed runework axe", "a mace and the Student's Knack spell"): the guides name each city's weapon its own way;
ours is the class's one level 20 pair (Freeport's, below, the only such records), the same everywhere. Paladins,
wizards, monks, druids and rangers have no level 20 quest weapon here (their guides name none, or a weapon with no
record: the Tethelin ranger's Frozen Dawn scimitar, the Klik'Anon wizard's two-handed staff): their picks are left.
A cleric's or paladin's third path (Elemental Ward, Clay Barrier: ours) gets no weapon.

Every class's level 20 quest ends at its guildmaster saying "Choose your path." The guides (eqoa.forumotion.com:
Freeport t10-t19, Grobb t21-t23, Oggok t63-t66) give a pick of two spells, each with a weapon ("Bellow ... comes with
an ensorcelled greataxe. Pillar of Might ... comes with an ensorcelled longsword"); the import gave one spell (and in
Freeport the first path's weapon): no pick. Neriak's were made picks today (tools/darkelf_quests_20261114.py), as the
other cities' are. Each quest's reward step (its last) gets a choice (queststeps.choices) of the two paths, each its
spell and its weapon, the step's own spell and item cleared:
  Freeport  warrior    Bellow + ensorcelled greataxe / Pillar of Might + ensorcelled longsword
            alchemist  Student's Knack + alchemized club / Klik'Anon Cocktail + alchemical tome (NEW, "a book")
            bard       Power Dance + magic foil / Sweeping Combat + magic sabre
            cleric     Disease Ward + magic staff / Field Dress + enchanted mace
            magician   Frozen Mark + magical staff / Lava Stone + Mystical Tome
            rogue      Avoidance + magic dagger / Minor Wound + magic rapier
            enchanter  Spacious Mind + magic scepter / Power Boon + magic book
            necromancer Blood Gale + magic sickle / Power Gale + magic totem
            shadowknight Shadow Tower + Axe of Defiance / Bloodwasp + magic partisan (Heirloom's pick since 2026-11-13,
                       now Live's too: the guide names the spells, not the weapons; these are the two SK level 20 weapons)
            wizard     Blazing Clash / Mental Focus (the guide gives no weapon)
  Grobb     shadowknight, warrior: as Freeport's (the guides name the spells alone; the weapons as the other cities')
            shaman     Decaying Wind + magic greatclub (NEW) / Feral Spirit + magic spear (NEW) (the guide: "Ferel Spirit")
  Oggok     necromancer Blood Gale + magic sickle / Power Gale + magic totem (the guide's dagger and staff: Freeport's
                       necromancer pair, the only level 20 necromancer weapons)
            shadowknight as Freeport's ("Choose your path")
            shaman     Decaying Wind + magic greatclub / Spiritual Guide + magic spear (the guide: "Spell gives you
                       decaying wind and a 2 handed club, pet path gives you spiritual guide and a 1 handed spear")
            warrior    Bellow + ensorcelled greataxe / Pillar of Might + ensorcelled longsword ("an axe", "a sword")
New items (none existed; each a copy of the comparable level 20 class weapon, its class and look its own):
  "magic greatclub" (shaman, two-handed blunt): magic staff 8414's row (the cleric's two-handed level 20 weapon:
    damage 267, wis 22, cha 10, hp 9), the look of the Moorblood Great Club (model -553431974, its icon);
  "magic spear" (shaman, one-handed pierce): magic dagger 8550's damage 230 and slot, wis 13 sta 12 for its stats,
    the look of the Spiritrune Spear (model -6891105, its icon);
  "alchemical tome" (alchemist, held in the off hand): magic book 8428's row (the enchanter's book), the alchemized
    club's wis 15, int 15, power 40.
Each at the next patternid of its shard (the shards' item ids differ).

Who finished one already (charquests done), as a quest gives an item (the pack's next place, remainHP the record's
maxhp; a full pack, 100 with what is worn (pack_slots), is reported for a GM, not given; an item held is not given
twice): one who picked (charquests.picks) gets their path's weapon; one whose quest gave one path (Freeport, Grobb,
Oggok, and Neriak's four that did until today: no pick recorded) gets that path's weapon when it gave none and the
other path's scroll, so the pick is theirs to make. Live: Smoke (Freeport shadowknight: Bloodwasp) the magic partisan
and the Shadow Tower scroll; Smokey (Freeport necromancer: Blood Gale and the magic sickle) the Power Gale scroll;
Battery and Areo their picks' weapons (Fayspires: magic book, Mystical Tome).

The quests load at the server's start and the server holds a player's pack while they play: run it in a keep restart's
before.sh. Refused, nothing written, when a step is not as the import left it. Report only by default; --apply in one
transaction, a gm_audit line a row, log l20_rewards_1114_log; a second --apply does nothing; --undo exact (an item
given is taken back only while it is where it was put; a created item record is removed only while nothing holds it).

    python3 -E -s tools/l20_rewards_20261114.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "l20_rewards_1114_log"
WHO, VIA = "dab", "tool"
DETAIL = ("petition 5071 (Smoke): the level 20 quests of Freeport, Grobb and Oggok give the guides' pick of two paths, "
          "each with its weapon: tools/l20_rewards_20261114.py")
INV_CAP = 100  # the pack, worn items among them (config pack_slots 100 on both shards; a PS2 is shown 40)
ASK = "Which do you take?"
SAYS = "Then it is yours."

# spells
BELLOW, PILLAR = 1588, 1534
KNACK, COCKTAIL = 700223, 700195
DANCE, SWEEP = 164, 700225
DWARD, FDRESS = 700184, 700187
FMARK, LSTONE = 664, 700197
AVOID, WOUND = 700167, 700205
SMIND, PBOON = 700215, 700207
BGALE, PGALE = 700170, 700208
STOWER, BWASP = 700212, 1436
BCLASH, MFOCUS = 700169, 700203
DWIND, FSPIRIT, SGUIDE = 700181, 700186, 700217
# weapons (the same patternid on both shards: checked by name)
WEAPONS = {8575: "ensorcelled greataxe", 8576: "ensorcelled longsword", 8402: "alchemized club", 8371: "magic foil",
           8372: "magic sabre", 8414: "magic staff", 8413: "enchanted mace", 8453: "magical staff", 8454: "Mystical Tome",
           8550: "magic dagger", 8551: "magic rapier", 8427: "magic scepter", 8428: "magic book", 8530: "magic sickle",
           8531: "magic totem", 5501: "Axe of Defiance", 8562: "magic partisan"}
# new items: name -> (the row copied, the columns set)
NEW_ITEMS = {
    "magic greatclub": (8414, {"classuse": 256, "raceuse": 1023, "attacktype": 4, "equipslot": 15, "model": -553431974,
                               "itemicon": 469503945, "itemdesc": "A greatclub glowing with magic."}),
    "magic spear": (8550, {"classuse": 256, "raceuse": 1023, "attacktype": 5, "equipslot": 12, "weapondamage": 230,
                           "unk2": 0, "unk3": 0, "unk4": 0, "unk5": 0, "model": -6891105, "itemicon": 109831808, "agi": 0, "wis": 13, "sta": 12,
                           "lore": 1, "trade": 1, "maxhp": 3000, "itemdesc": "A spear glowing with magic."}),
    "alchemical tome": (8428, {"classuse": 16384, "raceuse": 271, "cha": 0, "intelligence": 15, "wis": 15,
                               "POWMAX": 40, "itemdesc": "A tome of alchemical lore."}),
}
SK = ((STOWER, 5501), (BWASP, 8562))
# quest -> (the path its step gave: spell, the item it gave or None), the two paths (spell, weapon); a weapon a
# NEW_ITEMS name or a patternid or None
QUESTS = {
    100070: ((BELLOW, 8575), ((BELLOW, 8575), (PILLAR, 8576))),
    100080: ((KNACK, 8402), ((KNACK, 8402), (COCKTAIL, "alchemical tome"))),
    100090: ((DANCE, 8371), ((DANCE, 8371), (SWEEP, 8372))),
    100100: ((DWARD, 8414), ((DWARD, 8414), (FDRESS, 8413))),
    100110: ((FMARK, 8453), ((FMARK, 8453), (LSTONE, 8454))),
    100120: ((AVOID, 8550), ((AVOID, 8550), (WOUND, 8551))),
    100130: ((SMIND, 8427), ((SMIND, 8427), (PBOON, 8428))),
    100140: ((BGALE, 8530), ((BGALE, 8530), (PGALE, 8531))),
    100150: ((BWASP, None), SK),
    100160: ((BCLASH, None), ((BCLASH, None), (MFOCUS, None))),
    100170: ((BWASP, None), SK),
    100180: ((DWIND, None), ((DWIND, "magic greatclub"), (FSPIRIT, "magic spear"))),
    100190: ((BELLOW, None), ((BELLOW, 8575), (PILLAR, 8576))),
    100516: ((BGALE, None), ((BGALE, 8530), (PGALE, 8531))),
    100526: ((BWASP, None), SK),
    100536: ((DWIND, None), ((DWIND, "magic greatclub"), (SGUIDE, "magic spear"))),
    100546: ((BELLOW, None), ((BELLOW, 8575), (PILLAR, 8576))),
}
ROW = "*row*"

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,
    keycol  TEXT NOT NULL,
    keyval  TEXT NOT NULL,
    col     TEXT NOT NULL,
    before  TEXT,
    after   TEXT,
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


def rowdict(db, sql, args):
    cur = db.execute(sql, args)
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def audit(db, tbl, keycol, keyval, col, before, after):
    action = "update" if col != ROW else ("delete" if after is None else "insert")
    if col == ROW:
        before, after = ({} if before is None else before), ({} if after is None else after)
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    rowkey = dict(zip(keycol.split(","), (int(v) for v in str(keyval).split(","))))
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, compact(rowkey),
                      compact(before if col == ROW else {col: before}), compact(after if col == ROW else {col: after}), DETAIL))
    return cur.lastrowid


def spellname(db, sid):
    r = db.execute("SELECT spellname FROM spellpattern WHERE spellid = ?", (sid,)).fetchone()
    return r and r[0]


def choices_json(paths, ids, names):
    opts = []
    for spell, weapon in paths:
        o = {"answer": names[spell] if weapon is None else f"{names[spell]} and the {names[('w', weapon)]}", "says": SAYS}
        if weapon is not None:
            o["items"] = str(ids[weapon])
        o["spells"] = str(spell)
        opts.append(o)
    return compact({"ask": ASK, "options": opts})


WEAPON_BY_SPELL = {"Bellow": 8575, "Pillar of Might": 8576, "Student's Knack": 8402, "Klik'Anon Cocktail": "alchemical tome",
                   "Power Dance": 8371, "Sweeping Combat": 8372, "Disease Ward": 8414, "Field Dress": 8413,
                   "Frozen Mark": 8453, "Lava Stone": 8454, "Avoidance": 8550, "Minor Wound": 8551, "Spacious Mind": 8427,
                   "Power Boon": 8428, "Blood Gale": 8530, "Power Gale": 8531, "Shadow Tower": 5501, "Bloodwasp": 8562,
                   "Decaying Wind": "magic greatclub", "Feral Spirit": "magic spear", "Spiritual Guide": "magic spear"}
# Neriak's level 20 quests that gave one path until tools/darkelf_quests_20261114.py (2026-11-14): the path given
NERIAK_WAS_SINGLE = {100446: SMIND, 100466: BGALE, 100476: AVOID, 100486: BWASP}
CLASSBIT = {"warrior": 1, "ranger": 2, "paladin": 4, "shadowknight": 8, "monk": 16, "bard": 32, "rogue": 64, "druid": 128,
            "shaman": 256, "cleric": 512, "magician": 1024, "necromancer": 2048, "enchanter": 4096, "wizard": 8192,
            "alchemist": 16384}


def option_spell(o):
    return o["answer"].split(" and the ")[0]


def scroll_of(db, name, cls):
    r = db.execute("SELECT patternid FROM itempattern WHERE itemname = ? AND equipslot NOT IN (12, 13, 14, 15, 18) "
                   "AND (classuse & ?) != 0 ORDER BY patternid", (name, CLASSBIT[cls])).fetchone()
    return r and r[0]


def plan(db):
    changes, lines, refusals = [], [], []
    names, ids = {}, {}
    for pid, n in WEAPONS.items():
        r = db.execute("SELECT itemname FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
        if r is None or r[0] != n:
            refusals.append(f"itempattern {pid}: {r and r[0]!r}, want {n!r}")
        ids[pid], names[("w", pid)] = pid, n
    nid = db.execute("SELECT MAX(patternid) FROM itempattern").fetchone()[0] + 1
    for name, (src, cols) in NEW_ITEMS.items():
        have = db.execute("SELECT patternid FROM itempattern WHERE itemname = ? AND classuse = ?", (name, cols["classuse"])).fetchone()
        names[("w", name)] = name
        if have:
            ids[name] = have[0]
            lines.append(f"item {name!r} is there already ({have[0]})")
            continue
        row = rowdict(db, "SELECT * FROM itempattern WHERE patternid = ?", (src,))
        if row is None:
            refusals.append(f"itempattern {src} (the copy for {name}) is missing")
            continue
        row.update(cols, patternid=nid, itemname=name)
        ids[name] = nid
        changes.append(("itempattern", "patternid", nid, ROW, None, row))
        lines.append(f"item {nid} {name!r}: a copy of {src} ({row['itemdesc']})")
        nid += 1
    for qid, ((given, gave_item), paths) in QUESTS.items():
        for spell, _ in paths:
            names[spell] = spellname(db, spell)
            if not names[spell]:
                refusals.append(f"spell {spell} missing")
    if refusals:
        return changes, lines, refusals, ids
    gives = {}  # serverid -> [(patternid, why)]

    def grant(sid, cname, pid, what):
        if db.execute("SELECT 1 FROM charinventory WHERE serverid = ? AND patternid = ?", (sid, pid)).fetchone() or \
                any(p == pid for p, _ in gives.get(sid, [])):
            lines.append(f"  {cname} ({sid}) holds {what} already")
            return
        gives.setdefault(sid, []).append((pid, f"{cname}: {what}"))

    quests = db.execute("SELECT questid, source FROM quests WHERE source LIKE 'EQOA-Data 1-20/%/L20%' ORDER BY questid").fetchall()
    for qid, src in quests:
        city, cls = src.split("/")[1:3]
        st = db.execute("SELECT MAX(step) FROM queststeps WHERE questid = ?", (qid,)).fetchone()[0]
        r = rowdict(db, "SELECT kind, spells, rewarditems, choices FROM queststeps WHERE questid = ? AND step = ?", (qid, st))
        key = f"{qid},{st}"
        finishers = db.execute("SELECT c.serverid, h.charName, c.picks FROM charquests c JOIN characters h ON h.serverid = "
                               "c.serverid WHERE c.questid = ? AND c.done >= 1 ORDER BY c.serverid", (qid,)).fetchall()
        if qid in QUESTS:
            (given, gave_item), paths = QUESTS[qid]
            want = choices_json(paths, ids, names)
            def paths_of(c):
                return [(o.get("spells", ""), o.get("items", "")) for o in json.loads(c)["options"]]
            if r["choices"] and sorted(paths_of(r["choices"])) == sorted(paths_of(want)) and r["spells"] == "" and \
                    r["rewarditems"] == "":
                lines.append(f"{city} {cls} {qid} step {st}: the pick is there already")
            elif r["kind"] not in ("talk", "give", "show") or r["choices"] != "" or r["spells"] != str(given) or \
                    r["rewarditems"] != ("" if gave_item is None else str(gave_item)):
                refusals.append(f"quest {qid} step {st}: {r}, want spells {given} items {gave_item} and no choices")
                continue
            else:
                changes.append(("queststeps", "questid,step", key, "choices", "", want))
                changes.append(("queststeps", "questid,step", key, "spells", r["spells"], ""))
                if r["rewarditems"]:
                    changes.append(("queststeps", "questid,step", key, "rewarditems", r["rewarditems"], ""))
                lines.append(f"{city} {cls} {qid} step {st}: was {names[given]} alone"
                             f"{'' if gave_item is None else ' + the ' + names[('w', gave_item)]}; now "
                             + " / ".join(o["answer"] for o in json.loads(want)["options"]))
            for sid, cname, picks in finishers:
                if picks:
                    continue  # chose from Heirloom's pick (100150)
                path = next(w for s_, w in paths if s_ == given)
                if gave_item is None and path is not None:
                    grant(sid, cname, ids[path], f"the {names[('w', path)]} (the {names[given]} path's weapon)")
                for s_, _ in paths:
                    if s_ != given:
                        sc = scroll_of(db, names[s_], cls)
                        if sc:
                            grant(sid, cname, sc, f"the {names[s_]} scroll (the path not offered)")
                        else:
                            lines.append(f"  {cname}: no {names[s_]} scroll for a {cls}: not given")
            continue
        if not r["choices"]:
            lines.append(f"{city} {cls} {qid}: no pick and not in the plan: left")
            continue
        ch = json.loads(r["choices"])
        added = []
        for o in ch["options"]:
            w = WEAPON_BY_SPELL.get(option_spell(o))
            if w is None:
                continue
            have = [x.strip() for x in o.get("items", "").split(",") if x.strip()]
            if str(ids[w]) in [x.split("*")[0] for x in have]:
                continue
            o["items"] = ", ".join(have + [str(ids[w])])
            added.append(f"{option_spell(o)} + the {names[('w', w)]}")
        if added:
            new = compact(ch)
            changes.append(("queststeps", "questid,step", key, "choices", r["choices"], new))
            lines.append(f"{city} {cls} {qid} step {st}: a pick already; its weapons added: " + "; ".join(added))
        elif any(WEAPON_BY_SPELL.get(option_spell(o)) for o in ch["options"]):
            lines.append(f"{city} {cls} {qid} step {st}: pick and weapons there already")
        else:
            lines.append(f"{city} {cls} {qid} step {st}: a pick, no weapon for the class (as the guides we have)")
        for sid, cname, picks in finishers:
            if picks:
                try:
                    i = int(picks.split(",")[-1].split(":")[1])
                    o = ch["options"][i]
                except (ValueError, IndexError):
                    lines.append(f"  {cname} ({sid}): picks {picks!r} not read")
                    continue
                w = WEAPON_BY_SPELL.get(option_spell(o))
                if w is not None:
                    grant(sid, cname, ids[w], f"the {names[('w', w)]} (their pick, {option_spell(o)})")
            elif qid in NERIAK_WAS_SINGLE:
                given = spellname(db, NERIAK_WAS_SINGLE[qid])
                w = WEAPON_BY_SPELL.get(given)
                if w is not None:
                    grant(sid, cname, ids[w], f"the {names[('w', w)]} (the {given} path's weapon)")
                for o in ch["options"]:
                    if option_spell(o) != given:
                        sc = scroll_of(db, option_spell(o), cls)
                        if sc:
                            grant(sid, cname, sc, f"the {option_spell(o)} scroll (the path not offered)")
            else:
                lines.append(f"  {cname} ({sid}) finished it with no pick recorded: left")
    for sid, items in gives.items():
        held = db.execute("SELECT count(*) FROM charinventory WHERE serverid = ? AND location = 0", (sid,)).fetchone()[0]
        for pid, why in items:
            if held >= INV_CAP:
                lines.append(f"  {why}: pack full ({held}): NOT given, a GM gives it")
                continue
            held += 1
            changes.append(("give", "serverid", sid, "patternid", None, pid))
            lines.append(f"  given: {why} ({pid})")
    return changes, lines, refusals, ids


def write(db, tbl, keycol, keyval, col, before, after):
    cols, vals = keycol.split(","), [int(v) for v in str(keyval).split(",")]
    where = " AND ".join(f"{c} = ?" for c in cols)
    if col == ROW:
        now = rowdict(db, f"SELECT * FROM {tbl} WHERE {where}", vals)
        if after is None:
            if now != before:
                return False
            if tbl == "itempattern" and db.execute("SELECT 1 FROM charinventory WHERE patternid = ?", vals).fetchone():
                return False  # someone holds it now
            return db.execute(f"DELETE FROM {tbl} WHERE {where}", vals).rowcount == 1
        if now is not None:
            return False
        names = list(after)
        db.execute(f"INSERT INTO {tbl} ({', '.join(names)}) VALUES ({', '.join('?' * len(names))})", [after[n] for n in names])
        return True
    return db.execute(f"UPDATE {tbl} SET {col} = ? WHERE {where} AND {col} IS ?", (after, *vals, before)).rowcount == 1


def give(db, sid, pid):
    cur = db.execute(
        "INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber) "
        "VALUES (?, 1, (SELECT COALESCE(maxhp, 0) FROM itempattern WHERE patternid = ?), 0, ?, -1, 0, "
        "(SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = 0))",
        (sid, pid, pid, sid))
    return rowdict(db, "SELECT * FROM charinventory WHERE itemid = ?", (cur.lastrowid,))


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for _, tbl, keycol, keyval, col, before, after, _a in db.execute(f"SELECT * FROM {LOG} ORDER BY seq DESC").fetchall():
        before, after = json.loads(before), json.loads(after)
        if tbl == "charinventory":
            now = rowdict(db, "SELECT * FROM charinventory WHERE itemid = ?", (int(keyval),))
            if now is None or now["serverid"] != after["serverid"] or now["patternid"] != after["patternid"] or \
                    now["equiploc"] != -1 or now["location"] != after["location"]:
                print(f"left: charinventory {keyval} (moved, worn or gone since)")
                left += 1
                continue
            db.execute("DELETE FROM charinventory WHERE itemid = ?", (int(keyval),))
            audit(db, "charinventory", "itemid", keyval, ROW, now, None)
            back += 1
            continue
        if not write(db, tbl, keycol, keyval, col, after, before):
            print(f"left: {tbl} {keyval} {col} changed since")
            left += 1
            continue
        audit(db, tbl, keycol, keyval, col, after, before)
        back += 1
    if not left:
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
        if a.undo:
            db.execute("BEGIN IMMEDIATE")
            try:
                back, left = undo(db)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            print(f"put back {back} rows, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if table(db, LOG):
            print(f"applied already ({LOG} is there): --undo first")
            return 0
        changes, lines, refusals, _ = plan(db)
        for line in lines:
            print(line)
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not changes:
            print("nothing to do")
            return 0
        if not a.apply:
            print(f"report only: {len(changes)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            db.execute(LOG_SCHEMA)
            for tbl, keycol, keyval, col, before, after in changes:
                if tbl == "give":
                    row = give(db, keyval, after)
                    tbl, keycol, keyval, col, before, after = "charinventory", "itemid", row["itemid"], ROW, None, row
                elif not write(db, tbl, keycol, keyval, col, before, after):
                    raise RuntimeError(f"{tbl} {keyval} changed under the update")
                a_id = audit(db, tbl, keycol, keyval, col, before, after)
                db.execute(f"INSERT INTO {LOG} (tbl, keycol, keyval, col, before, after, audit) VALUES (?, ?, ?, ?, ?, ?, ?)",
                           (tbl, keycol, str(keyval), col, json.dumps(before), json.dumps(after), a_id))
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {len(changes)} rows, log {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
