#!/usr/bin/env python3
"""Side quests of levels 23-53 against the eqoa.forumotion.com walkthroughs (the owner, 2026-11-21: t105 Hodstock 23,
t106 Temby 25, t107 Sarek's Tower 27, t109 Muniel's Tea Garden 33, t110 Highpass Hold 35, t111 Oasis 37, t112 Eternal
Desert 43, t113 Castle Lightwolf 45, t114 Gramash Ruins 47, t115 Dead Hills 53; more threads may follow: one more
entry in FIXES or REQUIRES each).

Audited, every step, NPC, price, standing and class reward of the EQOA-Data 23-47/23 ... 47 quests (100759-100766,
100772-100792) and 53-59/53/p1 (101020). Both shards held the same rows on 2026-11-21.

FIVE DIFFERENCES, fixed here (steps rewritten whole; rewards, items and NPCs are the quest's own, nothing made):
  1. The Remains of Manger MacGray (Hodstock 23 part 3): the guide's "Ice chest gives you your final rewards". Ours
     had the chest hand over the remains and a sixth step, the remains back to Feria Hurkert, carry the experience and
     the class items. The chest step now carries that step's rewards as they are and no longer gives the remains
     (nothing else asks for them; the record is kept); the sixth step goes.
  2. Yismay's Tea (Muniel's Tea Garden 33): the guide's "Captain Ogofa sends you to Captain Skorn (one tent over) who
     sells you the tinsy tea leaves for 1625 tunar". Ours took the 1,625 at Captain Hogofa and Skorn gave the leaves
     for nothing: Hogofa is now spoken to, Skorn takes the 1,625.
  3. The Seal of Deiphobus (Highpass Hold 35, Ambrose Castor): the guide's Iulias Maphesti sends you to Caelduiel,
     "return to Iulias", then he sends you to Lelalia in Temby, "Return to Iulias", then back to Ambrose. Ours sent
     both at once and paid Lelaia before the charm went to Iulias. Now: Caelduiel's charm to Iulias, then Lelaia's
     seal (2,325), then the seal shown to Iulias, then Ambrose (one step more).
  4. Fender's Stone (Highpass Hold 35): the guide's "Fender sends you to Smithy Palin, on the far end of the HP keep,
     for your reward ... Smithy Palin wants 2250 tunar, he then sends you to Tailor Portia ... Tailor Portia rewards
     you!". Ours gave the experience at Palin and nothing at Portia: Portia now gives it.
  5. The Thulite Anvil (Gramash Ruins 47 part 3): the guide's "the quests, which has to be done in order"; ours
     required part 1 only. It now requires parts 1 and 2 (Margun Leafstrider).

LEFT AS OURS (matches, the guide is garbled or silent, or names something we have no record of):
  - Spellings: the guide's Feria Huckert, Apprentice Buerma, Razuam Marus, Captain Ogofa, Father Manstien, Dametrios,
    Phorriun, Lelalia Shavererll, Captain Ravensite, Svenle, Enrique Reynold, Elmo Haggar are our NPC rows Feria
    Hurkert, Apprentice Buema, Razuan Marus, Captain Hogofa, Father Manstein, Demetrios Velspane, Phoborium, Lelaia
    Shavrell, Captain Ravenhite, Svente Dormio, Enrique Revnold, Elmo Hagger; the quests already name the rows.
  - Creatures with no record: the guide's Redheaded Bandit (ours: any arctic bandit's 25% bandana) and Blacktongue
    Ravager (any Blacktongue orc's 25% lantern).
  - The guide's "Featherlite" helm, tunic and gloves (Hodstock shaman): no records; ours Featherweave, same parts. The
    cleric's list names the Goldlight Cap twice, so which part gives the shield is garbled: ours coif + Weaken Death,
    shield, boots. Every other class's Hodstock rewards match item for item, ability for ability; Dead Hills' Etheric
    tunic, no-dachi, shield, kama and Astral Displacement match class for class.
  - Prices all match (1,200 x3, 790, 1,200, 782, 1,450, 1,625 x3, 2,325 x4, 2,250, 2,556, 2,550, 2,652 x3, 2,900,
    2,923, 2,875, 3,000 x3, 7,000); so do Feria's and Castle Lightwolf's 150 (both raisable) and every experience the
    guides give (1,785,000, 2,679,818, 3,768,632, 4,140,484, 6,722,242, 7,083,126, 45,000,000).
  - The red diamond (Sarek 2): the guide's second seller, Tsekar in the Green Rift, is not a seller here: a give step
    names one NPC (an alias counts for kill and loot only), so that is code, not data.
  - Guide silent: the gates the guides mention without a figure (Temby's, Min Gyrind's gypsies, the Black Swan Inn's
    Bandit Hills bandits), every unnamed item ("an item", "a class specific ring"), the experience a guide gives no
    figure for.

Everything goes by NAME: a quest by its source, name and giver, a step by its kind and NPC, an item by its itemname. A
quest not found once, a step not as written, a character standing on a step that changes, or a table
(queststeptargets, questdoors, questwalls) naming one, and nothing is written.

Report only by default; --apply writes, a gm_audit line a quest (its steps before and after), the old steps and notes
and requires kept in side_quest_guides_1121_log; --undo puts back each quest still as this tool left it. A second --apply finds
nothing to do.

WHEN IT TAKES EFFECT: the quests are read again at a GM panel quest edit or reload, else at the server's next start.

    tools/side_quest_guides_20261121.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "side_quest_guides_1121_log"
TOOL = "tools/side_quest_guides_20261121.py"
DETAIL = "side quests against the forum walkthroughs (levels 23-53): " + TOOL
ACCOUNT, WHO, VIA = 1, "dab", "tool"
REWARDS = ("xp", "rewardtunar", "rewarditems", "factions", "spells", "byclass")
EMPTY = {"xp": 0, "rewardtunar": 0, "rewarditems": "", "factions": "", "spells": "", "byclass": ""}


class Refused(Exception):
    pass


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def fold(s):
    return " ".join((s or "").split()).lower()


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    cols = [c[0] for c in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def item_names(db, items):
    """The item names an "id*count, ..." list holds, in order (an id with no record: '#id')."""
    out = []
    for part in str(items or "").split(","):
        part = part.strip()
        if not part:
            continue
        pid = int(part.split("*")[0])
        r = db.execute("SELECT itemname FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
        out.append(r[0] if r else f"#{pid}")
    return out


def events_of(s):
    try:
        ev = json.loads(s) if s else []
    except ValueError:
        raise Refused(f"events not JSON: {s!r}")
    return ev if isinstance(ev, list) else []


def gives(db, st, name):
    """The step's events that give the item named name."""
    return [e for e in events_of(st["events"]) if isinstance(e, dict) and e.get("do") == "give"
            and item_names(db, e.get("items", "")) == [name]]


def has_rewards(st):
    return any((st[k] or EMPTY[k]) != EMPTY[k] for k in REWARDS)


def expect(db, steps, n, kind, target, items=None, tunar=None):
    """Step n, refused unless it is kind at target (with those item names, that price)."""
    st = next((s for s in steps if s["step"] == n), None)
    if st is None:
        raise Refused(f"no step {n}")
    got = (fold(st["kind"]), fold(st["target"]))
    if got != (kind, fold(target)):
        raise Refused(f"step {n} is {got[0]} {st['target']!r}, not {kind} {target!r}")
    if items is not None and item_names(db, st["items"]) != items:
        raise Refused(f"step {n} items {item_names(db, st['items'])}, not {items}")
    if tunar is not None and st["tunar"] != tunar:
        raise Refused(f"step {n} asks {st['tunar']} tunar, not {tunar}")
    return st


def renumber(steps):
    out = []
    for i, s in enumerate(steps, 1):
        out.append(dict(s, step=i))
    return out


# --- the fixes: each takes the quest's steps; None when already as the guide, else the new steps -----------------

def manger_macgray(db, steps):
    last = steps[-1]
    if fold(last["target"]) == "frosty chest":
        if has_rewards(last):
            return None
        raise Refused("the frosty chest is the last step, with no rewards")
    back = expect(db, steps, len(steps), "give", "Feria Hurkert", ["Remains of Manger MacGray"])
    chest = expect(db, steps, len(steps) - 1, "give", "frosty chest", ["Ice Key"])
    if has_rewards(chest) or chest["choices"]:
        raise Refused("the frosty chest step has rewards or choices of its own")
    g = gives(db, chest, "Remains of Manger MacGray")
    if len(g) != 1:
        raise Refused(f"{len(g)} events of the chest give the remains, not one")
    new = dict(chest, **{k: back[k] for k in REWARDS})
    keep = events_of(chest["events"])
    keep.remove(g[0])
    new["events"] = go_json(keep) if keep else ""
    return steps[:-2] + [new]


def yismay_leaves(db, steps):
    s5 = next((s for s in steps if s["step"] == 5), None)
    if s5 is not None and fold(s5["target"]) == "captain skorn" and fold(s5["kind"]) == "give" and s5["tunar"] == 1625:
        return None
    hog = expect(db, steps, 4, "give", "Captain Hogofa", [], 1625)
    skorn = expect(db, steps, 5, "talk", "Captain Skorn", [], 0)
    if len(gives(db, skorn, "Tinsy Tea Leaves")) != 1:
        raise Refused("Captain Skorn's step does not give the tinsy tea leaves once")
    ask = expect(db, steps, 3, "give", "Teamaster Yismay")
    old_says = "Take him 1,625 tunar."
    if old_says not in ask["says"]:
        raise Refused(f"step 3 says no {old_says!r}")
    new = [dict(s) for s in steps]
    new[2]["says"] = ask["says"].replace(old_says, "Take 1,625 tunar with you.")
    new[3].update(kind="talk", tunar=0,
                  journal="Find Captain Hogofa at the gypsy camp up the road north of Muniel's Tea Garden.",
                  says="Tinsy tea leaves? Skorn, in the next tent, sells them. Tell him I sent you.")
    new[4].update(kind="give", tunar=hog["tunar"], notyet=hog["notyet"] or "Sixteen hundred and twenty-five tunar.",
                  journal="Buy the tinsy tea leaves from Captain Skorn in the next tent (1,625 tunar).")
    return new


def seal_of_deiphobus(db, steps):
    s5 = next((s for s in steps if s["step"] == 5), None)
    if s5 is not None and fold(s5["target"]) == "iulias maphesti":
        return None
    iulias = expect(db, steps, 3, "talk", "Iulias Maphesti")
    expect(db, steps, 4, "loot", "Caelduiel the Sacred", ["Caelduiel's Charm"])
    lelaia = expect(db, steps, 5, "give", "Lelaia Shavrell", [], 2325)
    charm = expect(db, steps, 6, "give", "Iulias Maphesti", ["Caelduiel's Charm"])
    expect(db, steps, 7, "talk", "Ambrose Castor")
    seal = gives(db, lelaia, "Lelaia's Seal")
    if len(seal) != 1:
        raise Refused("Lelaia's step does not give her seal once")
    seal_id = str(seal[0]["items"]).strip()
    a = dict(iulias, says="Kill Caelduiel the Sacred at the Al'Farek ruins, south of the great Nasehir village, west "
                          "to the blue stones, and bring me his charm.")
    b = dict(charm, says="Good. Now Lelaia Shavrell in Temby has a seal for us. Pay her for it and bring it to me.")
    c = dict(lelaia)
    d = dict(charm, kind="show", items=seal_id, tunar=0, says="Lelaia's seal. Keep it: Ambrose will want it. Go back "
             "to him.", journal="Show Lelaia's seal to Iulias Maphesti in Freeport.", notyet="", waiting="",
             events="", **EMPTY)
    return renumber(steps[:2] + [a, steps[3], b, c, d] + steps[6:])


def fenders_stone(db, steps):
    palin = expect(db, steps, 5, "give", "Smithy Palin", [], 2250)
    portia = expect(db, steps, 6, "talk", "Tailor Portia")
    if len(steps) != 6:
        raise Refused(f"{len(steps)} steps, not 6")
    if has_rewards(portia) and not has_rewards(palin):
        return None
    if has_rewards(portia) or not has_rewards(palin):
        raise Refused("neither Smithy Palin alone nor Tailor Portia alone has the rewards")
    return steps[:4] + [dict(palin, **EMPTY), dict(portia, **{k: palin[k] for k in REWARDS})]


# (source, quest, giver, fix, the guide's thread and words)
FIXES = [
    ("EQOA-Data 23-47/23/p3", "The Remains of Manger MacGray", "Feria Hurkert", manger_macgray,
     "t105 Hodstock 23: \"Ice chest gives you your final rewards\": the rewards at the frosty chest, the return of the"
     " remains to Feria dropped"),
    ("EQOA-Data 23-47/33/teagarden", "Yismay's Tea", "Teamaster Yismay", yismay_leaves,
     "t109 Muniel's Tea Garden 33: Captain Skorn \"sells you the tinsy tea leaves for 1625 tunar\": the price at Skorn,"
     " not Hogofa"),
    ("EQOA-Data 23-47/35/ambrose", "The Seal of Deiphobus", "Ambrose Castor", seal_of_deiphobus,
     "t110 Highpass Hold 35: Caelduiel's charm back to Iulias, then Lelaia's seal, then \"Return to Iulias\", then"
     " Ambrose"),
    ("EQOA-Data 23-47/35/fender", "Fender's Stone", "Fender Crow", fenders_stone,
     "t110 Highpass Hold 35: \"Tailor Portia rewards you!\": the experience at Portia, not Smithy Palin"),
]

# A quest that must wait for others: (source, quest, giver, [(source, quest) it requires, all of them], the guide)
REQUIRES = [
    ("EQOA-Data 23-47/47/p3", "The Thulite Anvil", "Grimm Brellhand",
     [("EQOA-Data 23-47/47/p1", "Ore for the Forge"), ("EQOA-Data 23-47/47/p2", "Margun Leafstrider")],
     "t114 Gramash Ruins 47: \"the quests, which has to be done in order\": part 3 after part 2, not only part 1"),
]


def find_quest(db, source, name, giver):
    qs = rows(db, "SELECT * FROM quests WHERE name = ? AND (source = ? OR source LIKE ?)", (name, source, source + " %"))
    qs = [q for q in qs if fold(q["giver"]) == fold(giver)]
    if len(qs) != 1:
        raise Refused(f"{len(qs)} quests named {name!r} from {source!r} given by {giver!r}, not one")
    return qs[0]


def quest_steps(db, qid):
    return rows(db, "SELECT * FROM queststeps WHERE questid = ? ORDER BY step", (qid,))


def first_changed(old, new):
    for i in range(max(len(old), len(new))):
        if i >= len(old) or i >= len(new) or old[i] != new[i]:
            return i + 1
    return None


def note_for(why):
    return f" Against the guide 2026-11-21 ({TOOL}): {why}."


def find_by_source(db, source, name):
    qs = rows(db, "SELECT questid FROM quests WHERE name = ? AND (source = ? OR source LIKE ?)", (name, source, source + " %"))
    if len(qs) != 1:
        raise Refused(f"{len(qs)} quests named {name!r} from {source!r}, not one")
    return qs[0]["questid"]


def requires_ids(s):
    return [int(x) for x in str(s or "").replace(",", " ").split()]


def plan(db):
    """[(quest, old steps, new steps, first step changed, note, new requires or None)] still to do; [names] done."""
    todo, done = [], []
    for source, name, giver, needs, why in REQUIRES:
        q = find_quest(db, source, name, giver)
        want = [find_by_source(db, s, n) for s, n in needs]
        have = requires_ids(q["requires"])
        if sorted(have) == sorted(want):
            done.append(f"{q['questid']} {name}")
            continue
        if not set(have) <= set(want):
            raise Refused(f"quest {q['questid']} {name!r} requires {q['requires']!r}, not a part of {want}")
        steps = quest_steps(db, q["questid"])
        todo.append((q, steps, steps, None, note_for(why), ", ".join(str(i) for i in want)))
    for source, name, giver, fix, why in FIXES:
        q = find_quest(db, source, name, giver)
        old = quest_steps(db, q["questid"])
        try:
            new = fix(db, old)
        except Refused as e:
            raise Refused(f"quest {q['questid']} {name!r}: {e}")
        if new is None:
            done.append(f"{q['questid']} {name}")
            continue
        first = first_changed(old, new)
        qid = q["questid"]
        stand = db.execute("SELECT count(*) FROM charquests WHERE questid = ? AND done = 0 AND step >= ?",
                           (qid, first)).fetchone()[0]
        if stand:
            raise Refused(f"quest {qid} {name!r}: {stand} characters stand on step {first} or later (steps that "
                          f"change); run again once they are past it")
        for t in ("queststeptargets", "questdoors", "questwalls"):
            if db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is None:
                continue
            n = db.execute(f"SELECT count(*) FROM {t} WHERE questid = ? AND step >= ?", (qid, first)).fetchone()[0]
            if n:
                raise Refused(f"quest {qid} {name!r}: {n} {t} rows name step {first} or later")
        todo.append((q, old, new, first, note_for(why), None))
    return todo, done


def audit(db, tbl, key, before, after, detail=DETAIL):
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', ?, ?, ?, ?, ?)",
               (now(), ACCOUNT, WHO, VIA, tbl, go_json(key), go_json(before), go_json(after), detail))


def put_steps(db, qid, steps):
    db.execute("DELETE FROM queststeps WHERE questid = ?", (qid,))
    for s in steps:
        cols = list(s)
        db.execute(f"INSERT INTO queststeps ({', '.join(cols)}) VALUES ({', '.join('?' for _ in cols)})",
                   [s[c] for c in cols])


def apply(db, todo):
    db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (questid INTEGER PRIMARY KEY, oldsteps TEXT NOT NULL, "
               "newsteps TEXT NOT NULL, oldnotes TEXT NOT NULL, newnotes TEXT NOT NULL, oldrequires TEXT NOT NULL, "
               "newrequires TEXT NOT NULL, at TEXT NOT NULL)")
    for q, old, new, first, note, req in todo:
        qid = q["questid"]
        notes, oldreq = q["notes"] or "", q["requires"] or ""
        newnotes = notes if note.strip() in notes else notes + note
        newreq = oldreq if req is None else req
        if new != old:
            put_steps(db, qid, new)
            audit(db, "queststeps", {"questid": qid}, {"steps": old}, {"steps": new})
        db.execute("UPDATE quests SET notes = ?, requires = ? WHERE questid = ?", (newnotes, newreq, qid))
        db.execute(f"INSERT OR REPLACE INTO {LOG} (questid, oldsteps, newsteps, oldnotes, newnotes, oldrequires, "
                   "newrequires, at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                   (qid, go_json(old), go_json(new), notes, newnotes, oldreq, newreq, now()))
        audit(db, "quests", {"questid": qid}, {"notes": notes, "requires": oldreq},
              {"notes": newnotes, "requires": newreq})
    return len(todo)


def undo(db):
    if db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (LOG,)).fetchone() is None:
        return 0, 0
    back, left = 0, 0
    for qid, olds, news, oldnotes, newnotes, oldreq, newreq in db.execute(
            f"SELECT questid, oldsteps, newsteps, oldnotes, newnotes, oldrequires, newrequires FROM {LOG} "
            "ORDER BY questid").fetchall():
        cur = quest_steps(db, qid)
        qrow = db.execute("SELECT notes, requires FROM quests WHERE questid = ?", (qid,)).fetchone()
        if go_json(cur) != news or qrow is None or tuple(qrow) != (newnotes, newreq):
            print(f"  quest {qid}: changed since, left")
            left += 1
            continue
        if olds != news:
            old = json.loads(olds)
            put_steps(db, qid, old)
            audit(db, "queststeps", {"questid": qid}, {"steps": json.loads(news)}, {"steps": old}, "undo: " + DETAIL)
        db.execute("UPDATE quests SET notes = ?, requires = ? WHERE questid = ?", (oldnotes, oldreq, qid))
        audit(db, "quests", {"questid": qid}, {"notes": newnotes, "requires": newreq},
              {"notes": oldnotes, "requires": oldreq}, "undo: " + DETAIL)
        db.execute(f"DELETE FROM {LOG} WHERE questid = ?", (qid,))
        back += 1
    return back, left


def describe(db, s):
    bits = [f"{s['step']}. {s['kind']} {s['target'] or '(place)'}"]
    if s["items"]:
        bits.append("items " + ", ".join(item_names(db, s["items"])))
    if s["tunar"]:
        bits.append(f"{s['tunar']} tunar")
    if s["xp"]:
        bits.append(f"xp {s['xp']}")
    if s["byclass"]:
        bits.append(f"byclass {len(json.loads(s['byclass']))} classes")
    for e in events_of(s["events"]):
        if isinstance(e, dict) and e.get("do") == "give":
            bits.append("gives " + ", ".join(item_names(db, e.get("items", ""))))
    return "; ".join(bits)


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, timeout=5)
    try:
        if a.undo:
            back, left = undo(db)
            db.commit()
            print(f"undo: {back} quests put back, {left} left as changed since")
            return 0
        try:
            todo, done = plan(db)
        except Refused as e:
            print(f"REFUSED, nothing written: {e}")
            return 2
        for q, old, new, first, note, req in todo:
            if req is not None:
                print(f"  {q['questid']} {q['name']}: requires {q['requires']!r} -> {req!r}.{note}")
                continue
            print(f"  {q['questid']} {q['name']} (from step {first}):{note}")
            for s in old[first - 1:]:
                print(f"      was {describe(db, s)}")
            for s in new[first - 1:]:
                print(f"      now {describe(db, s)}")
        for d in done:
            print(f"  {d}: already as the guide")
        print(f"{len(todo)} quests to change, {len(done)} already as the guide")
        if a.apply:
            n = apply(db, todo)
            db.commit()
            print(f"applied: {n} quests changed")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
