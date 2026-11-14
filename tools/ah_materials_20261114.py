#!/usr/bin/env python3
"""The crafting materials perpetually on sale at the auction house (the owner, 2026-11-14: "lets put all gems, alloys
etc. for crafting on the auction house perpetually on sale ... seller name ilovetesters"; then: both shards, common 100
tunar, rare 500).

One standing lot per material (database/auction_perpetual.go: an auction_lots row with perpetual = 1, seller 0, its
name "ilovetesters", one item, its starting bid and buyout the price, ending 2100-01-01): a buy at the price files the
items into the buyer's bank and leaves the lot up. The server shows and sells them only with
world.perpetual_auctions: true.

The materials are what the crafts' recipes consume (craftrecipes.ingredients, read on the database given, so each
shard's own recipes), less:
  - recipe products (a part made by another recipe: the Plane of Disease's Zombie Swatch);
  - the starter kits' first-work parts (Blacksmith's Emblem, Rusty Iron Block, Rusted Bronze Block, Piece of Glass,
    Piece of Cracked Glass: tools/seed_tradeskills.py's tutorial);
  - the Plane of Disease flag quest's parts (Stitched Zombie Skin, Sinew Cord);
  - iron ore, a quest item first (45 quest steps ask for it, 17 give it) and a part of 26 crude recipes;
  - anything No Trade, Lore or not rentable (CLIENT's CanSell: not auctionable).
Common or rare: a modifier (a gem, alloy, crystal: an ingredient the recipes mark keep) is the rarity of the recipes
it is in (Prima's: the product's rarity is its rarest part's; each modifier's recipes are of one rarity): Common and
Uncommon are common, Rare and Ultra Rare rare. A mundane part (ore, bar, mold) is common. The price is 100 tunar a
common, 500 a rare, raised to the item's value where that is more (merchants pay an item's full value,
handlers/merchant.go merchantRate 100: a lot under it would be tunar for nothing; and a player's own listing may not
start under it either).

auction_lots.perpetual is the server's (it adds it at its start): without it this refuses, so the rows are never
written under a build that would sweep them as orphans. Report only by default; --apply in one transaction, a gm_audit
line a row, log ah_materials_log; --undo exact. The server reads the lots per search: no restart for the rows.

    python3 -E -s tools/ah_materials_20261114.py --db eqoa.db [--apply | --undo]
"""
import datetime
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "ah_materials_log"
DETAIL = "The crafting materials perpetually on sale from ilovetesters: tools/ah_materials_20261114.py"
SELLER = "ilovetesters"
COMMON, RARE = 100, 500
ENDS = 4102444800  # database.AuctionPerpetualEnds, 2100-01-01
LISTED = int(datetime.datetime(2026, 11, 14, tzinfo=datetime.timezone.utc).timestamp())
RARITY = {0: "Common", 1: "Uncommon", 2: "Rare", 3: "Ultra Rare"}
# by name, any case: the shards' records of the later items have their own ids (Live made them after Heirloom)
LEFT_OUT = {
    "blacksmith's emblem": "starter kit", "rusty iron block": "starter kit", "rusted bronze block": "starter kit",
    "piece of glass": "starter kit", "piece of cracked glass": "starter kit",
    "stitched zombie skin": "PoD flag quest", "sinew cord": "PoD flag quest", "zombie swatch": "PoD flag quest",
    "iron ore": "a quest item (45 steps ask for it)",
}


def materials(db):
    """[(pattern row, rarity, price, why)], [(pattern, name, why left out)]."""
    products = {r[0] for r in db.execute("SELECT product FROM craftrecipes")}
    used = {}
    for rarity, ing in db.execute("SELECT rarity, ingredients FROM craftrecipes"):
        for i in json.loads(ing):
            u = used.setdefault(i["item"], {"keep": False, "rarities": set()})
            if i.get("keep"):
                u["keep"] = True
                u["rarities"].add(rarity)
    out, left = [], []
    for pid in sorted(used):
        p = tr.rows(db, "SELECT patternid, itemname, itemcost, maxstack, maxhp, trade, lore, rent FROM itempattern "
                        "WHERE patternid = ?", (pid,))
        if not p:
            left.append((pid, "?", "no item record"))
            continue
        p = p[0]
        u = used[pid]
        if p["itemname"].lower() in LEFT_OUT:
            left.append((pid, p["itemname"], LEFT_OUT[p["itemname"].lower()]))
        elif pid in products:
            left.append((pid, p["itemname"], "a recipe's product"))
        elif p["trade"] or p["lore"] or not p["rent"]:
            left.append((pid, p["itemname"], "not auctionable (No Trade, Lore or not rentable)"))
        else:
            rarity = max(u["rarities"]) if u["keep"] else 0
            base = RARE if rarity >= 2 else COMMON
            price = max(base, p["itemcost"] or 0)
            out.append((p, rarity if u["keep"] else None, price, base))
    return out, left


def row(p, price):
    return {"seller": 0, "seller_name": SELLER, "patternid": p["patternid"], "stack": 1, "remainhp": p["maxhp"] or 0,
            "charges": 0, "min_bid": price, "buyout": price, "fee": 0, "listed_at": LISTED, "ends_at": ENDS,
            "state": "open", "per_item": 1, "perpetual": 1}


def changes(db):
    if not tr.column(db, "auction_lots", "perpetual"):
        return [], [], ["auction_lots has no perpetual column: start the build with the standing lots "
                        "(database/auction_perpetual.go) on this database first; it adds the column"], False
    mats, left = materials(db)
    out, lines = [], []
    done = todo = 0
    counts = {"common": 0, "rare": 0}
    raised = []
    for p, rarity, price, base in mats:
        kind = "rare" if base == RARE else "common"
        counts[kind] += 1
        if price != base:
            raised.append(f"{p['itemname']} {price}")
        key = {"seller": 0, "perpetual": 1, "patternid": p["patternid"]}
        after = row(p, price)
        have = tr.rows(db, "SELECT * FROM auction_lots WHERE seller = 0 AND perpetual = 1 AND patternid = ?",
                       (p["patternid"],))
        if len(have) > 1:
            return [], [], [f"{len(have)} standing lots of {p['itemname']} ({p['patternid']}): one expected"], False
        if have and all(have[0][c] == v for c, v in after.items()):
            done += 1
            continue
        todo += 1
        out.append(("auction_lots", key, after))
        what = "mundane" if rarity is None else RARITY[rarity]
        lines.append(f"  {p['patternid']:>7} {p['itemname']:<26} {what:<10} {kind:<6} {price:>4} tunar"
                     + (" (its value)" if price != base else "") + (" (price changed)" if have else ""))
    lines.insert(0, f"{len(mats)} materials: {counts['common']} common ({COMMON}), {counts['rare']} rare ({RARE}); "
                    f"{todo} to put up, {done} up already")
    if raised:
        lines.append(f"raised to the item's value ({len(raised)}): " + ", ".join(raised))
    lines.append(f"left out ({len(left)}): " + "; ".join(f"{n} ({pid}): {why}" for pid, n, why in left))
    return out, lines, [], todo == 0 and done > 0


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("auction_lots", "craftrecipes", "itempattern"), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
