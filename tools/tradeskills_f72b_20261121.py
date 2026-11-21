#!/usr/bin/env python3
"""Three of the forum Trade Skills audit's findings, approved by the owner (2026-11-21; the forum is eqoa.forumotion.com
f72, read whole for tools/tradeskills_f72_20261121.py):

1. MASTER CRAFT CLOTH AND LEATHER UNDER TAILORING. t395 ("Trade Skills General Questions") has Armor make "plate armor,
   chain armor and shields" and Tailoring "cloth armor, robes"; tools/master_craft_20261114.py put all 420 master-craft
   recipes under Armor Crafting (Tailoring was not live then). The 196 of cloth (Tweed, Vineweb Silk, Red Lined,
   Embroidered: Monk, Necromancer, Magician, Wizard, Enchanter) and leather (Pigskin, Shaded Leather, Trimmed Leather,
   Superior: Druid, Alchemist) become Tailoring's: craftrecipes.skill 1 -> 2 and comp 10 -> 18 (each list's Special
   tab: Armor's 10, Tailoring's 18, tools/seed_tradeskills.py TAILOR_SPECIAL). Nothing else in a recipe names its
   craft: its level (7-10), rarity, difficulty and XP are the same in either (seed_tradeskills.py's rules), the tool it
   needs is the craft's by wear slot (a Tailor's Kit of tier >= the level: Expert 31 for 7 to Grandmaster 46 for 10),
   and an Experiment matches by items alone (handlers/craft.go). A character's book (charrecipes) holds recipe ids and
   the book a skill shows is its recipes' by craftrecipes.skill, so a recipe known under Armor Crafting is in the
   Tailoring book after, at its own place; its orders are character-wide, so none collides. Making one needs the
   Tailoring skill at the recipe's level: a character who knows a moved recipe with Tailoring under its level is
   listed and the tool REFUSES, unless --raise-tailoring, which sets that character's Tailoring to the level (rank 0,
   no XP, logged and undone like the rest).

2. FIRE EMERALD AND STAR RUBY ARE RARE. t394 ("Gems, Gems, Gems") rates both Rare; Prima p. 442 (the seed's source)
   ultra rare. Each part that follows from a jewel gem's rarity becomes a rare gem's, read off the database's own
   rare jewel gems (Ruby, Diamond, Emerald, Sapphire), which must agree:
     - the gem's value: GEM_PRICE[rare] (800 -> 270);
     - its recipes (Jewel Crafting, the gem kept): rarity 3 -> 2, difficulty 10 x level + 2 x rarity (-2), XP
       25 x level x (1 + rarity) (x 3/4: 500 -> 375 at trade level 5, 1000 -> 750 at 10);
     - their products' value: 3 x the bar + 10 + MOD_WORTH[rarity] (2000 -> 600: 1,400 less each);
     - its "craft gems" loot table row (the seed's LOOT_WEIGHT / LOOT_LEVELS: weight 1 -> 3, from level 30 -> 20) and
       its "gems" loot pool row (tools' category seed: weight 12 -> 38, from level 30 -> 20), as the rare gems' rows;
     - its perpetual auction lot (tools/ah_materials_20261114.py: a rare material at 500 or its value if more): 800 ->
       500, only a perpetual lot, open, with no bid.
   Every one is checked as the rule had it before (a figure another tool or a GM changed is REFUSED, not guessed).

3. "l60 gems" ON THE BOSSES t393 NAMES. t393 ("Level 60 Gem Drops") names the level 60 gems' droppers; the table
   (Black Sapphire, Blue Diamond) is on four of them (King Cepheus, Queen Lacerta, Saftemoot, the haliai high
   priestess) at one chance and roll. Seven more that are here get it at the same chance and rolls: Shiran Harpy
   Queen, Jal'Raeth Patriarch, Jal'Raeth Bishop, High Priest Vorin and Tal'Thir (Envar), Overlord Xrag, and t393's
   "Overseer" of Geomancer's Citadel, which is Overseer Durekar here (the one level 60 overseer, beside Sarek the
   Betrayer's citadel). A boss not on the shard is left out and listed; npcloot keys a name as database.NPCClassKey.

Report by default; --apply in one transaction, a gm_audit line a row (as the panel's), logged in
tradeskills_f72b_1121_log; a second --apply finds nothing to do; --undo puts back exactly what was there (a row
changed since is left and listed). --skip PART (move, rare, drops; repeatable) leaves a part out.

WHEN IT TAKES EFFECT: no restart. Recipes are read again within 30 s; item values at each sale (a holder's window at
the next login); loot tables and pools at each death; auction lots at each search.

    python3 -E -s tools/tradeskills_f72b_20261121.py --db eqoa.db [--skip PART ...] [--raise-tailoring] [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "tradeskills_f72b_1121_log"
DETAIL = ("forum Trade Skills audit, the owner's approval (2026-11-21): master craft cloth and leather under Tailoring, "
          "Fire Emerald and Star Ruby rare, l60 gems on t393's bosses; tools/tradeskills_f72b_20261121.py")
WHO, VIA = "dab", "tool"
SOURCE = "f72-1121"
PARTS = ("move", "rare", "drops")

# 1: the master-craft source, its two crafts and Special tabs, the cloth and leather patterns and raw materials
# (tools/master_craft_20261114.py CHART, CHART_FIX applied)
MC_SOURCE = "master-craft-1114"
ARMOR, TAILOR, JEWEL = 1, 2, 3
ARMOR_SPECIAL, TAILOR_SPECIAL = 10, 18
CLOTH_PATTERNS = ["Tweed", "Vineweb Silk", "Red Lined", "Embroidered", "Pigskin", "Shaded Leather", "Trimmed Leather", "Superior"]
CLOTH_RAWS = ["Silky Thread", "Vineweb Silk Thread", "Blood Drenched Thread", "Silklined Swatch",
              "Intact Fur Hide", "Double Layered Hide", "Flakey Hide", "Brown Split Hide"]
MOVE_EXPECT = 196

# 2: the gems, their rare peers, the seed's rules (tools/seed_tradeskills.py GEM_PRICE, MOD_WORTH, difficulty, xp)
RARE_GEMS = ["Fire Emerald", "Star Ruby"]
RARE_PEERS = ["Ruby", "Diamond", "Emerald", "Sapphire"]
OLD_R, NEW_R = 3, 2
GEM_PRICE = [20, 60, 270, 800]
MOD_WORTH = [50, 200, 600, 2000]
GEM_TABLE, GEM_POOL = "craft gems", "gems"
AH_RARE = 500  # tools/ah_materials_20261114.py RARE
GEM_RECIPES = 24  # each: trade levels 5-10, four molds


def difficulty(level, rarity):
    return 10 * level + 2 * rarity


def xp(level, rarity):
    return 25 * level * (1 + rarity)


# 3: the table, the bosses it is on, the bosses t393 adds (names as npcs has them)
L60 = "l60 gems"
L60_HAVE = ["king cepheus", "queen lacerta", "saftemoot", "haliai high priestess"]
L60_ADD = ["Shiran Harpy Queen", "Jal'Raeth Patriarch", "Jal'Raeth Bishop", "High Priest Vorin", "Tal'Thir",
           "Overlord Xrag", "Overseer Durekar"]
L60_NOTE = "forum t393 Level 60 Gem Drops names it; chance and rolls the four's (tools/tradeskills_f72b_20261121.py)"

LOG_DDL = f"""CREATE TABLE {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    part   TEXT NOT NULL,
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,      -- the key (JSON)
    before TEXT,               -- the row before (JSON), NULL: inserted
    after  TEXT NOT NULL,      -- the row written (JSON)
    audit  INTEGER NOT NULL
)"""


def npc_key(name):
    """database.NPCClassKey: lower case, trimmed, no trailing $."""
    n = (name or "").strip().lower()
    return n[:-1].strip() if n.endswith("$") else n


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def columns(db, tbl):
    return {r[1] for r in db.execute(f"PRAGMA table_info({tbl})")}


def row(db, tbl, key):
    where = " AND ".join(f"{k} = ?" for k in key)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", tuple(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, tbl, key, action, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key), go_json(before), go_json(after), DETAIL))
    return cur.lastrowid


def put(db, tbl, key, want):
    """tbl's row of key made want (columns to set; None: deleted; a row not there: inserted); (before, after, audit)."""
    before = row(db, tbl, key)
    where = " AND ".join(f"{k} = ?" for k in key)
    if want is None:
        db.execute(f"DELETE FROM {tbl} WHERE {where}", tuple(key.values()))
        action = "delete"
    elif before is None:
        cols = dict(key, **want)
        db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", tuple(cols.values()))
        action = "insert"
    else:
        sets = {k: v for k, v in want.items() if k not in key}
        db.execute(f"UPDATE {tbl} SET {', '.join(f'{k} = ?' for k in sets)} WHERE {where}",
                   tuple(sets.values()) + tuple(key.values()))
        action = "update"
    after = row(db, tbl, key)
    return before, after, audit(db, tbl, key, action, before, after)


def one_item(db, name, refusals, slot=-1):
    found = db.execute("SELECT patternid FROM itempattern WHERE itemname = ? AND equipslot = ?", (name, slot)).fetchall()
    if len(found) != 1:
        refusals.append(f"{len(found)} component records named {name!r}, not one")
        return None
    return found[0][0]


def recipes_of(db):
    return [dict(id=i, skill=s, comp=c, level=lv, rarity=r, name=n, product=p, ings=json.loads(g), difficulty=d, xp=x, source=src)
            for i, s, c, lv, r, n, p, g, d, x, src in
            db.execute("SELECT id, skill, comp, level, rarity, name, product, ingredients, difficulty, xp, source FROM craftrecipes ORDER BY id")]


def holders(db, ids):
    if not ids or not table(db, "charinventory"):
        return 0, 0
    q = ",".join("?" * len(ids))
    return db.execute(f"SELECT count(*), count(DISTINCT serverid) FROM charinventory WHERE patternid IN ({q})", list(ids)).fetchone()


def knowers(db, ids):
    if not ids or not table(db, "charrecipes"):
        return []
    q = ",".join("?" * len(ids))
    return db.execute(f"SELECT charid, recipe FROM charrecipes WHERE recipe IN ({q}) ORDER BY charid, recipe", list(ids)).fetchall()


def plan_move(db, recipes, raise_tailoring, writes, lines, refusals):
    names = {p: n for p, n in db.execute("SELECT patternid, itemname FROM itempattern")}
    raws = {n.lower() for n in CLOTH_RAWS}
    pats = tuple(p.lower() + " " for p in CLOTH_PATTERNS)
    moved, done = [], []
    for r in recipes:
        if r["source"] != MC_SOURCE:
            continue
        inames = [(names.get(i.get("item")) or "").lower() for i in r["ings"]]
        if not (any(n in raws for n in inames) and any(n.startswith(pats) and n.endswith(" pattern") for n in inames)):
            continue
        if (r["skill"], r["comp"]) == (TAILOR, TAILOR_SPECIAL):
            done.append(r)
        elif (r["skill"], r["comp"]) == (ARMOR, ARMOR_SPECIAL):
            moved.append(r)
        else:
            refusals.append(f"recipe {r['id']} {r['name']!r}: skill {r['skill']} comp {r['comp']}, neither Armor's Special tab nor Tailoring's")
    if len(moved) + len(done) != MOVE_EXPECT:
        refusals.append(f"{len(moved) + len(done)} master-craft cloth and leather recipes here, not {MOVE_EXPECT}")
    lines.append(f"1. master craft cloth and leather: {len(moved)} recipes Armor Crafting -> Tailoring (skill {ARMOR} -> {TAILOR}, "
                 f"comp {ARMOR_SPECIAL} -> {TAILOR_SPECIAL}); {len(done)} there already")
    by_level = {}
    for r in moved:
        by_level[r["level"]] = by_level.get(r["level"], 0) + 1
        writes.append(("move", "craftrecipes", {"id": r["id"]}, {"skill": TAILOR, "comp": TAILOR_SPECIAL}))
    if by_level:
        lines.append("   by trade level: " + ", ".join(f"{lv}: {n}" for lv, n in sorted(by_level.items())))
    level_of = {r["id"]: r["level"] for r in moved + done}
    know = knowers(db, list(level_of))
    need = {}
    for charid, rid in know:
        need[charid] = max(need.get(charid, 0), level_of[rid])
    lines.append(f"   known by {len(need)} characters ({len(know)} book entries): they stay in each book, under Tailoring")
    for charid, lv in sorted(need.items()):
        cur = row(db, "charcraftskills", {"charid": charid, "skill": TAILOR}) if table(db, "charcraftskills") else None
        have = cur["level"] if cur else 1
        if have >= lv:
            lines.append(f"   character {charid}: Tailoring {have} >= {lv}, can make them")
            continue
        if not raise_tailoring:
            refusals.append(f"character {charid} knows a level {lv} moved recipe with Tailoring {have}: --raise-tailoring sets it to {lv}")
            continue
        lines.append(f"   character {charid}: Tailoring {have} -> {lv} (--raise-tailoring)")
        writes.append(("move", "charcraftskills", {"charid": charid, "skill": TAILOR}, {"level": lv, "rank": 0, "xp": 0}))


def plan_rare(db, recipes, writes, lines, refusals):
    # the rare peers' rule, which must agree
    peer_ids = [one_item(db, n, refusals) for n in RARE_PEERS]
    if None in peer_ids:
        return
    costs = {c for (c,) in db.execute(f"SELECT itemcost FROM itempattern WHERE patternid IN ({','.join('?' * len(peer_ids))})", peer_ids)}
    tab = {(w, lo) for w, lo in db.execute(f"SELECT weight, minlevel FROM loottables WHERE tablename = ? AND patternid IN "
                                           f"({','.join('?' * len(peer_ids))})", [GEM_TABLE] + peer_ids)}
    pool = {(w, lo) for w, lo in db.execute(f"SELECT weight, minlevel FROM lootpool WHERE category = ? AND patternid IN "
                                            f"({','.join('?' * len(peer_ids))})", [GEM_POOL] + peer_ids)}
    if costs != {GEM_PRICE[NEW_R]}:
        refusals.append(f"the rare gems' values are {sorted(costs)}, not {GEM_PRICE[NEW_R]}")
    if len(tab) != 1 or len(pool) != 1:
        refusals.append(f"the rare gems' {GEM_TABLE!r} rows {sorted(tab)} and {GEM_POOL!r} rows {sorted(pool)} do not agree")
        return
    (tw, tlo), (pw, plo) = tab.pop(), pool.pop()
    lines.append(f"2. Fire Emerald and Star Ruby rare (the rare gems: value {GEM_PRICE[NEW_R]}, {GEM_TABLE!r} weight {tw} from "
                 f"level {tlo}, {GEM_POOL!r} weight {pw} from level {plo})")
    bars = {}
    names = {p: (n, c) for p, n, c in db.execute("SELECT patternid, itemname, itemcost FROM itempattern")}
    perpetual = table(db, "auction_lots") and "perpetual" in columns(db, "auction_lots")
    for gem in RARE_GEMS:
        gid = one_item(db, gem, refusals)
        if gid is None:
            continue
        cost = names[gid][1]
        if cost == GEM_PRICE[OLD_R]:
            writes.append(("rare", "itempattern", {"patternid": gid}, {"itemcost": GEM_PRICE[NEW_R]}))
        elif cost != GEM_PRICE[NEW_R]:
            refusals.append(f"{gem}: value {cost}, neither {GEM_PRICE[OLD_R]} nor {GEM_PRICE[NEW_R]}")
        lines.append(f"   {gem} (item {gid}): value {cost} -> {GEM_PRICE[NEW_R]}")
        mine = [r for r in recipes if any(i.get("item") == gid and i.get("keep") for i in r["ings"])]
        if len(mine) != GEM_RECIPES or any(r["skill"] != JEWEL for r in mine):
            refusals.append(f"{gem}: {len(mine)} recipes take it ({sorted({r['skill'] for r in mine})} crafts), not {GEM_RECIPES} Jewel Crafting")
            continue
        n_rec = n_prod = 0
        for r in mine:
            lv = r["level"]
            bar = [names.get(i.get("item"), ("", 0)) for i in r["ings"] if not i.get("keep")]
            bar = [c for n, c in bar if n.endswith(" Bar")]
            if len(bar) != 1:
                refusals.append(f"recipe {r['id']} {r['name']!r}: {len(bar)} bars")
                continue
            old_p, new_p = 3 * bar[0] + 10 + MOD_WORTH[OLD_R], 3 * bar[0] + 10 + MOD_WORTH[NEW_R]
            have = (r["rarity"], r["difficulty"], r["xp"])
            if have == (OLD_R, difficulty(lv, OLD_R), xp(lv, OLD_R)):
                writes.append(("rare", "craftrecipes", {"id": r["id"]},
                               {"rarity": NEW_R, "difficulty": difficulty(lv, NEW_R), "xp": xp(lv, NEW_R)}))
                n_rec += 1
            elif have != (NEW_R, difficulty(lv, NEW_R), xp(lv, NEW_R)):
                refusals.append(f"recipe {r['id']} {r['name']!r}: rarity/difficulty/xp {have}, not the rule's at rarity {OLD_R} or {NEW_R}")
            p = names.get(r["product"])
            if p is None:
                refusals.append(f"recipe {r['id']}: product {r['product']} has no itempattern row")
            elif p[1] == old_p:
                writes.append(("rare", "itempattern", {"patternid": r["product"]}, {"itemcost": new_p}))
                n_prod += 1
                bars[lv] = (old_p, new_p)
            elif p[1] != new_p:
                refusals.append(f"{r['product']} {p[0]!r}: value {p[1]}, neither {old_p} nor {new_p}")
        lines.append(f"   {gem}: {n_rec} recipes rarity {OLD_R} -> {NEW_R}, difficulty -2, XP x3/4 "
                     f"({', '.join(f'TL{lv} {xp(lv, OLD_R)}->{xp(lv, NEW_R)}' for lv in (5, 10))}); {n_prod} products' value -"
                     f"{MOD_WORTH[OLD_R] - MOD_WORTH[NEW_R]}")
        for tbl, key_col, kind, (w, lo) in (("loottables", "tablename", GEM_TABLE, (tw, tlo)), ("lootpool", "category", GEM_POOL, (pw, plo))):
            rows_ = db.execute(f"SELECT id, weight, minlevel FROM {tbl} WHERE {key_col} = ? AND patternid = ?", (kind, gid)).fetchall()
            if len(rows_) != 1:
                refusals.append(f"{gem}: {len(rows_)} {tbl} rows in {kind!r}, not one")
                continue
            rid, cw, clo = rows_[0]
            if (cw, clo) != (w, lo):
                writes.append(("rare", tbl, {"id": rid}, {"weight": w, "minlevel": lo}))
            lines.append(f"   {gem}: {tbl} {kind!r} row {rid}: weight {cw} -> {w}, from level {clo} -> {lo}")
        if perpetual:
            lots = db.execute("SELECT lot, min_bid, buyout, state, bids, high_bidder FROM auction_lots WHERE patternid = ? AND perpetual = 1",
                              (gid,)).fetchall()
            for lot, mb, bo, state, bids, hb in lots:
                want = max(AH_RARE, GEM_PRICE[NEW_R])
                if (mb, bo) == (want, want):
                    continue
                if state != "open" or bids or hb or (mb, bo) != (max(AH_RARE, GEM_PRICE[OLD_R]),) * 2:
                    refusals.append(f"{gem}: perpetual lot {lot} {state}, {bids} bids, {mb}/{bo}: not as the materials tool left it")
                    continue
                writes.append(("rare", "auction_lots", {"lot": lot}, {"min_bid": want, "buyout": want}))
                lines.append(f"   {gem}: perpetual auction lot {lot}: {mb} -> {want}")
        n, chars = holders(db, [gid] + [r["product"] for r in mine])
        k = knowers(db, [r["id"] for r in mine])
        lines.append(f"   {gem}: held (gem and products) {n} items by {chars} characters; its recipes in {len({c for c, _ in k})} books")


def plan_drops(db, writes, lines, refusals):
    have = db.execute("SELECT npc_name, chance, rolls FROM npcloot WHERE tablename = ?", (L60,)).fetchall()
    base = {(c, r) for n, c, r in have if n in L60_HAVE}
    if len(base) != 1 or {n for n, _, _ in have if n in L60_HAVE} != set(L60_HAVE):
        refusals.append(f"{L60!r} on {sorted(n for n, _, _ in have)}: not the four at one chance and rolls")
        return
    chance, rolls = base.pop()
    if not db.execute("SELECT 1 FROM loottables WHERE tablename = ?", (L60,)).fetchone():
        refusals.append(f"no loot table {L60!r} here")
        return
    lines.append(f"3. {L60!r} (chance {chance}, rolls {rolls}) on t393's bosses")
    on = {n for n, _, _ in have}
    for name in L60_ADD:
        n = db.execute("SELECT count(*) FROM npcs WHERE npc_name = ?", (name,)).fetchone()[0]
        key = npc_key(name)
        if n == 0:
            lines.append(f"   {name}: not on this shard, left out")
            continue
        if key in on:
            lines.append(f"   {name}: has it already")
            continue
        writes.append(("drops", "npcloot", {"npc_name": key, "tablename": L60},
                       {"chance": chance, "rolls": rolls, "note": L60_NOTE, "source": SOURCE}))
        lines.append(f"   {name} ({n} spawn{'s' if n > 1 else ''}): + {L60!r}")


def plan(db, skip=(), raise_tailoring=False):
    """(writes [(part, tbl, key, want)], lines, refusals)."""
    writes, lines, refusals = [], [], []
    need = ["itempattern", "craftrecipes", "gm_audit"]
    if "rare" not in skip:
        need += ["loottables", "lootpool"]
    if "drops" not in skip:
        need += ["npcloot", "loottables", "npcs"]
    for t in dict.fromkeys(need):
        if not table(db, t):
            refusals.append(f"no {t} table here")
    bad = [s for s in skip if s not in PARTS]
    if bad:
        refusals.append(f"--skip takes {', '.join(PARTS)}, not {', '.join(bad)}")
    if refusals:
        return writes, lines, refusals
    recipes = recipes_of(db)
    if "move" not in skip:
        plan_move(db, recipes, raise_tailoring, writes, lines, refusals)
    if "rare" not in skip:
        plan_rare(db, recipes, writes, lines, refusals)
    if "drops" not in skip:
        plan_drops(db, writes, lines, refusals)
    for s in skip:
        lines.append(f"{s}: skipped (--skip)")
    return writes, lines, refusals


def apply(db, writes):
    db.execute(LOG_DDL)
    for part, tbl, key, want in writes:
        before, after, n = put(db, tbl, key, want)
        db.execute(f"INSERT INTO {LOG} (part, tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?, ?)",
                   (part, tbl, go_json(key), go_json(before), go_json(after), n))
    return len(writes)


def undo(db):
    back, left = 0, []
    if not table(db, LOG):
        return back, left
    for seq, tbl, rowkey, before, after in db.execute(f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(rowkey)
        if go_json(row(db, tbl, key)) != after:
            left.append(f"{tbl} {rowkey}: changed since, left as it is")
            continue
        b = json.loads(before) if before else None
        put(db, tbl, key, None if b is None else {k: v for k, v in b.items() if k not in key})
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--skip", action="append", default=[], metavar="PART", help=f"leave a part out: {', '.join(PARTS)} (repeatable)")
    ap.add_argument("--raise-tailoring", action="store_true",
                    help="raise the Tailoring of a character who knows a moved recipe above it to the recipe's level")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"undone: {back} rows back" + "".join(f"\n  left: {w}" for w in left))
            return 0 if not left else 1
        writes, lines, refusals = plan(db, set(a.skip), a.raise_tailoring)
        if refusals:
            db.execute("ROLLBACK")
            print("\n".join(lines))
            print("REFUSED, nothing written:" + "".join(f"\n  {r}" for r in refusals))
            return 1
        print("\n".join(lines))
        if not writes:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if table(db, LOG):
            db.execute("ROLLBACK")
            print(f"applied before and changed since ({LOG} is there): --undo first")
            return 1
        if not a.apply:
            db.execute("ROLLBACK")
            counts = {}
            for part, tbl, _, _ in writes:
                counts[f"{part} {tbl}"] = counts.get(f"{part} {tbl}", 0) + 1
            print(f"(report only: {len(writes)} rows: " + ", ".join(f"{k} {v}" for k, v in counts.items()) + "; --apply to write)")
            return 0
        n = apply(db, writes)
        db.execute("COMMIT")
        print(f"applied: {n} rows, in gm_audit and {LOG}")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
