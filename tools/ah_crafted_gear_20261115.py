#!/usr/bin/env python3
"""Crafted gear perpetually on sale at the auction house, the Live shard only (petition 5010, Horizon: "Loot seems a
little few and far between ... maybe a possible vender until more crafters start playing"; the owner, 2026-11-15:
"crafted-gear in the auction house for melee, tank, caster, healer and jeweleries for not too expensive").

Standing lots as tools/ah_materials_20261114.py's (database/auction_perpetual.go: an auction_lots row with
perpetual = 1, seller 0 under the name "ilovetesters", one item, its starting bid and buyout the price, ending
2100-01-01): a buy at the price files the item into the buyer's bank and leaves the lot up. Shown and sold only with
world.perpetual_auctions: true.

What: real crafted gear, each item a product of a craftrecipes row (Weapon Crafting 0, Armor Crafting 1, Tailoring 2,
Jewel Crafting 3), in five level bands (the products' level requirements 13, 23, 33, 43, 48: the recipe tiers' own
steps), up to fifteen pieces a band:
  melee   a one-handed and a two-handed sword (Fiery: the uncommon fire proc), a STR chain or scale tunic (Sinhalite)
  tank    an HP breastplate (Titan), a STA shield (Rhodizite)
  caster  an INT robe (Leucite), cloth power gloves (Infused)
  healer  a WIS robe and a WIS chain or scale tunic (Apatite)
  jewelry a STR ring (Amber), a STA ring (Bloodstone), an INT pendant (Rose Quartz), a WIS pendant (Jasper), a DEX
          earring (Onyx), an HP/power bracelet (Jade)
The tiers name their shapes differently (a Foil, Sabre, Long Sword, Warsword, Katana; a Heater, Kite, Tower Shield):
each piece tries its names in order. A band whose recipes have no such modifier (Armor Crafting's level-13 tier has
only plain pieces) takes the plain piece of the same kind (the cheapest so named); a piece two roles share is listed
once. Never anything No Trade, Lore, not rentable or of no value (the old dump's 1199 Mithril Bloodstone Ring, worth
0: level 48 has no STA ring). 73 lots on Live as of 2026-11-15.

Price: the item's own value (itempattern.itemcost), at least 100 tunar. A merchant pays an item's full value
(handlers/merchant.go merchantRate 100), so a lot under it would be tunar for nothing; at it, a piece costs what a
merchant would give back for it: about 400-2,500 tunar a piece over the bands.

Report only by default (the dry run); --apply in one transaction, a gm_audit line a row, log ah_crafted_gear_log;
--undo exact. The server reads the lots per search: no restart.

    python3 -E -s tools/ah_crafted_gear_20261115.py --db ~/eqoa-live/eqoa.db [--apply | --undo]
"""
import datetime
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "ah_crafted_gear_log"
DETAIL = "Petition 5010 (Live, Horizon): crafted gear perpetually on sale from ilovetesters: tools/ah_crafted_gear_20261115.py"
SELLER = "ilovetesters"
FLOOR = 100
ENDS = 4102444800  # database.AuctionPerpetualEnds, 2100-01-01
LISTED = int(datetime.datetime(2026, 11, 15, tzinfo=datetime.timezone.utc).timestamp())
BANDS = (13, 23, 33, 43, 48)
WEAPON, ARMOR, TAILOR, JEWEL = 0, 1, 2, 3

# (role, what, recipe skill, wear slot, the name's ends (the tiers name their shapes differently: tried in order),
# the modifier the name starts with (None: the gem is in the end), the class mask it must have (None: any))
CLOTH = 32767  # Tailoring's cloth: every class (its leather is 17407)
PIECES = (
    ("melee", "one-handed sword", WEAPON, 12, (" Long Sword", " Foil", " Sabre", " Warsword", " Katana"), "Fiery", None),
    ("melee", "two-handed sword", WEAPON, 15, (" Flamberge", " Claymore", " Greatsword", " Zweihander", " No-dachi"),
     "Fiery", None),
    ("melee", "STR chain tunic", ARMOR, 5, (" Scale Tunic", " Chain Tunic"), "Sinhalite", None),
    ("tank", "HP breastplate", ARMOR, 5, (" Breastplate",), "Titan", None),
    ("tank", "STA shield", ARMOR, 13, (" Tower Shield", " Kite Shield", " Heater Shield"), "Rhodizite", None),
    ("caster", "INT robe", TAILOR, 2, (" Robe",), "Leucite", None),
    ("caster", "power gloves", TAILOR, 19, (" Gloves",), "Infused", CLOTH),
    ("healer", "WIS robe", TAILOR, 2, (" Robe",), "Apatite", None),
    ("healer", "WIS chain tunic", ARMOR, 5, (" Scale Tunic", " Chain Tunic"), "Apatite", None),
    ("jewelry", "STR ring", JEWEL, 8, (" Amber Ring",), None, None),
    ("jewelry", "STA ring", JEWEL, 8, (" Bloodstone Ring",), None, None),
    ("jewelry", "INT pendant", JEWEL, 4, (" Rose Quartz Pendant",), None, None),
    ("jewelry", "WIS pendant", JEWEL, 4, (" Jasper Pendant",), None, None),
    ("jewelry", "DEX earring", JEWEL, 3, (" Onyx Earring",), None, None),
    ("jewelry", "HP/power bracelet", JEWEL, 6, (" Jade Bracelet",), None, None),
)

COLS = "p.patternid, p.itemname, p.itemcost, p.maxhp, p.levelreq, p.classuse, p.trade, p.lore, p.rent, r.rarity"


def products(db, skill, slot, level, end, classuse):
    out = tr.rows(db, f"SELECT DISTINCT {COLS} FROM craftrecipes r JOIN itempattern p ON p.patternid = r.product "
                      "WHERE r.skill = ? AND p.equipslot = ? AND p.levelreq = ? AND p.itemname LIKE ? "
                      "ORDER BY p.itemcost, p.patternid", (skill, slot, level, "%" + end))
    # sold as any lot is: never No Trade, Lore or not rentable (CLIENT's CanSell); a record of no value is the old
    # dump's, not the recipe tier's (one: 1199 Mithril Bloodstone Ring, worth 0)
    out = [p for p in out if p["itemname"].endswith(end) and not (p["trade"] or p["lore"] or not p["rent"])
           and (p["itemcost"] or 0) > 0]
    if classuse is not None:
        out = [p for p in out if p["classuse"] == classuse]
    return out


def pick(db, skill, slot, level, ends, mod, classuse):
    """(pattern row, plain: True when the band had no modified piece), or (None, reason)."""
    found = [(end, products(db, skill, slot, level, end, classuse)) for end in ends]
    if not any(have for _, have in found):
        return None, "no such crafted piece at this level"
    if mod is None:
        return next(have[0] for _, have in found if have), False
    for _, have in found:
        for p in have:
            if p["itemname"].startswith(mod + " "):
                return p, False
    # the plain piece: the cheapest of the first shape the tier makes (no modifier adds to its value)
    return next(have[0] for _, have in found if have), True


def price(p):
    return max(FLOOR, p["itemcost"] or 0)


def row(p):
    v = price(p)
    return {"seller": 0, "seller_name": SELLER, "patternid": p["patternid"], "stack": 1, "remainhp": p["maxhp"] or 0,
            "charges": 0, "min_bid": v, "buyout": v, "fee": 0, "listed_at": LISTED, "ends_at": ENDS,
            "state": "open", "per_item": 1, "perpetual": 1}


def selection(db):
    """[(band, role, what, pattern, plain)], [missing lines]."""
    out, missing, seen = [], [], set()
    for band in BANDS:
        for role, what, skill, slot, ends, mod, classuse in PIECES:
            p, plain = pick(db, skill, slot, band, ends, mod, classuse)
            if p is None:
                missing.append(f"level {band} {role} {what}: {plain}")
                continue
            if p["patternid"] in seen:
                continue
            seen.add(p["patternid"])
            out.append((band, role, what, p, plain))
    return out, missing


def changes(db):
    if not tr.column(db, "auction_lots", "perpetual"):
        return [], [], ["auction_lots has no perpetual column: start the build with the standing lots "
                        "(database/auction_perpetual.go) on this database first; it adds the column"], False
    sel, missing = selection(db)
    out, lines, refusals = [], [], []
    done = todo = total = 0
    for band, role, what, p, plain in sel:
        key = {"seller": 0, "perpetual": 1, "patternid": p["patternid"]}
        after = row(p)
        total += after["buyout"]
        have = tr.rows(db, "SELECT * FROM auction_lots WHERE seller = 0 AND perpetual = 1 AND patternid = ?",
                       (p["patternid"],))
        if len(have) > 1:
            refusals.append(f"{len(have)} standing lots of {p['itemname']} ({p['patternid']}): one expected")
            continue
        if have and all(have[0][c] == v for c, v in after.items()):
            done += 1
            continue
        todo += 1
        out.append(("auction_lots", key, after))
        lines.append(f"  L{band:<3} {role:<8} {what:<18} {p['patternid']:>7} {p['itemname']:<42} {after['buyout']:>5} tunar"
                     + (" (plain: no modified piece at this level)" if plain else "")
                     + (" (price changed)" if have else ""))
    lines.insert(0, f"{len(sel)} crafted pieces in {len(BANDS)} bands ({', '.join(map(str, BANDS))}), priced at their "
                    f"value (at least {FLOOR}): {todo} to put up, {done} up already; one of each costs {total} tunar")
    if missing:
        lines.append(f"none to be had ({len(missing)}): " + "; ".join(missing))
    return out, lines, refusals, todo == 0 and done > 0


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("auction_lots", "craftrecipes", "itempattern"), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
