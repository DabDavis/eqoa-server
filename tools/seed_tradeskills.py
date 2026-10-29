#!/usr/bin/env python3
"""Tradeskills' content (the user, 2026-10-29: "lets add tradeskills"; four
crafts were live, built Armorcrafting first): a craft's components, tools,
products and recipes, its trainers' starter kit and its suppliers' wares, put
in this server's tables from the public record. The engine that reads them is
internal/handlers/craft.go (claude-notes/tradeskill-protocol-20261029.md); the
record, claude-notes/tradeskill-sources-20261029.md.

**The record.**
- The recipes are eqoatrade.com's grids (2003-04, archived; fetched into a
  folder of the operator's own by tools/fetch_tradeskill_sources.py, never
  kept here): a page a piece, a cell a trade level and a modifier, holding the
  product's AC and stats or N/C (no combine). For armor 2,329 cells: Prima's
  Frontiers guide's own counts (120 mundane and 1,800 magical pieces, 150
  heater, 130 kite and 110 tower shields). A magical cell's AC is the mundane
  one, and a modifier's stats depend only on the trade level (every cell
  agrees), which is Prima's own rule (p. 441's stat bonuses by metal): so the
  two shields Prima lists that the grids lack, the targe and the tear (p. 441's
  AC rows), are made by it too.
- A recipe is the cell's trade level's ore, the piece's mold and, unless it
  is mundane, the cell's modifier (a gem or alloy), with the craft's tool held;
  a failure loses the ore and the mold and keeps the modifier (eqoatrade's
  tutorial). Its book tab is its metal (CLIENT's own list, spec 3.2), its level
  the trade level, its rarity its rarest part's (Prima).
- The components are Sandstorm's own records, read off its merchants in the
  harvests (--harvests; `eqoa harvest`'s JSON, never kept here): ten armorsmith
  hammers, a tier a metal ("Novice ... suitable for forging crude iron" to
  "Grandmaster ... seralite"), seventeen molds ("Plate Breastplate Mold",
  "Chain Coif Mold", five shields') and ten ores ("Crude Iron Ore" ... "Seralite
  Ore"), under Sandstorm's ids where this server has them free.
- Prima's rules (p. 440-441): a product's level is 5 x trade level - 2; its
  armor type chain or banded to trade level 5 and scale or plate from 6 (CLIENT's
  armor sets 3, 5, 7, 4, as the live records carry them), its tint the trade
  level's; the metals' bonus durability and hit points.
- Names as the live server's own records spell them: "<modifier> <metal>
  <piece>" ("heartsblood seralite breastplate", item level 48), mundane "<metal>
  <piece>".
- The trainers and suppliers are eqoatrade's vendor list: in twelve cities a
  "Smithy" (the tutorial and starter kit: tools/quests/data/tradeskills.json)
  and the merchants of hammers, molds and ore, whose stand-in "general goods"
  (tools/stock_merchants.py's) give way to the craft's wares.

**Jewelry** (--craft jewel): Prima's Frontiers guide's gem tables (pp. 442-443,
out of its layout text: each gem's rarity, least trade level and columns, read
by the rotated labels' places, and its cells by metal), a recipe a cell and a
mold (a bar, the mold and the gem, the gem kept on a failure), the products
"<metal> <gem> <piece>" at the same level rule (no metal's hit points; Prima's
bonus durability); the eight gems SOE changed on 5 Dec 2003 by the live
records' figures (GEM_ANCHORS); Sandstorm's own kits, wax molds and bars; the
gems this server lacks made as Sandstorm's are, a drop table "craft gems";
eqoatrade's starter (a glass trinket, a cracked glass earring) under Special.
A live record is the product only with the product's stats too.

A product this server has the live record of (its name, level and slot: the
heartsblood and mindflow sets, "heartsblood seralite breastplate") is that
record; one of the name that is another item (a found "Steel Chain Tunic" of
level 30) keeps its name, and the crafted one is made beside it.

**Ours** (no record has them): the products' prices (three of the ore, the
mold, and the modifier's worth by rarity), icons (the commonest of this
server's items of the slot and armor set) and hit points beyond the metal's
bonus (the live records fit 200 x level + the bonus and 50 + the bonus
durability), the twelve modifiers this server lacked (prices by rarity), the
starter kit's emblem, scroll and bracers, the recipes' difficulty and XP, and
the modifiers' drop table (eqoatrade: "Random", any creature; our chance, and
levels by rarity), bound to every creature by the loot row "*".

Every row written and every value changed is logged in tradeskill_seed;
--undo puts back each one still as written; a second --apply is refused while
the log is there. Report only by default. The server reads recipes, stock and
loot as it goes; new items reach a client at its next login.

    tools/seed_tradeskills.py [--db eqoa.db] --sources <folder> --harvests <dir or file> ... [--craft armor|jewel] [--apply | --undo]
"""
import argparse
import collections
import datetime
import glob
import html
import json
import os
import re
import shutil
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from import_sandstorm_harvest import item_row  # noqa: E402

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
SOURCE = "tradeskill-seed"
ARMOR = 1  # the skill: CLIENT's craft slot

# Prima p. 440, "Metals": the order is CLIENT's own component list, so the index is the book's tab
METALS = ["Crude Iron", "Iron", "Bronze", "Brass", "Steel", "Combine Steel", "Adamantite", "Mithril", "Brellium", "Seralite"]
BONUS_DUR = [1, 3, 7, 12, 15, 18, 20, 22, 26, 30]
BONUS_HP = [50, 100, 200, 400, 800, 1200, 1800, 2200, 2500, 2800]
# p. 441, "Metal Armor Type and Color by Mold and Trade Level": Tan, None, Orange, Orange, Steel, Sky,
# Wine, Blue, Brown, Black as CLIENT's $Tints holds them (no black: dark gray; none: white, 0)
TINT = [11, 0, 6, 6, 10, 2, 13, 4, 3, 1]
# p. 441, "Mundane Crafted Shield AC": the two shields the grids lack (None: none at that trade level)
EXTRA_SHIELD_AC = {
    "targe": [10, 29, 45, 68, 82, 101, 119, 138, 158, 175],
    "tear": [None, None, None, None, 118, 147, 173, 201, 229, 256],
}

# a piece: (equip slot, kind, its word in a product's name at trade levels 1-5 and 6-10, its mold)
PIECES = {
    "Chain/Coif.htm": (1, "chain", ("chain coif", "scale coif"), "Chain Coif Mold"),
    "Chain/Tunic.htm": (5, "chain", ("chain tunic", "scale tunic"), "Chain Tunic Mold"),
    "Chain/Bracers.htm": (7, "chain", ("chain bracers", "scale bracers"), "Chain Bracers Mold"),
    "Chain/Gloves.htm": (19, "chain", ("chain gloves", "scale gloves"), "Chain Gloves Mold"),
    "Chain/Legs.htm": (10, "chain", ("chain leggings", "scale leggings"), "Chain Leggings Mold"),
    "Chain/Boots.htm": (11, "chain", ("chain boots", "scale boots"), "Chain Boots Mold"),
    "Plate/Helm.htm": (1, "plate", ("helm", "helm"), "Plate Helm Mold"),
    "Plate/BP.htm": (5, "plate", ("breastplate", "breastplate"), "Plate Breastplate Mold"),
    "Plate/Bracers.htm": (7, "plate", ("banded bracers", "plate bracers"), "Plate Bracers Mold"),
    "Plate/Gaunts.htm": (19, "plate", ("gauntlets", "gauntlets"), "Plate Gauntlets Mold"),
    "Plate/Greaves.htm": (10, "plate", ("greaves", "greaves"), "Plate Greaves Mold"),
    "Plate/Boots.htm": (11, "plate", ("banded boots", "plate boots"), "Plate Boots Mold"),
    "Shield/Heater.htm": (13, "heater", ("heater shield", "heater shield"), "Heater Shield Mold"),
    "Shield/Kite.htm": (13, "kite", ("kite shield", "kite shield"), "Kite Shield Mold"),
    "Shield/Tower.htm": (13, "tower", ("tower shield", "tower shield"), "Tower Shield Mold"),
}
EXTRA_SHIELDS = {
    "targe": (13, "targe", ("targe shield", "targe shield"), "Targe Shield Mold"),
    "tear": (13, "tear", ("tear shield", "tear shield"), "Tear Shield Mold"),
}
# CLIENT's armor sets ($Armor) at trade levels 1-5 and 6-10: chain then scale, banded then plate
ARMOR_SET = {"chain": (3, 7), "plate": (5, 4)}
# who wears it: the live records' chain and plate masks; a shield by its armor class (Prima: the heater
# and the targe leather, the kite and the tear chain, the tower plate)
CLASSES = {"chain": 879, "plate": 525, "heater": 1007, "targe": 1007, "kite": 879, "tear": 879, "tower": 525}
ANY_CLASS, ANY_RACE = 32767, 32767
SHIELD_WORD = {"heater": "heater", "kite": "kite", "tower": "tower", "targe": "targe", "tear": "kite"}

# the grid's modifiers and their rarity (Prima p. 441: 0 common, 1 uncommon, 2 rare, 3 ultra rare)
MODIFIERS = {
    "Sinhalite": 0, "Rhodizite": 0, "Pectolite": 0, "Zoisite": 0, "Apatite": 0, "Leucite": 0, "Idocrase": 0,
    "Rubellite": 0, "Indicolite": 0, "Dravite": 0, "Achroite": 0, "Amblygonite": 0, "Schorl": 0,
    "Titan Alloy": 1, "Mana Infused Alloy": 1, "Marr Blessed Alloy": 1, "Lazulite": 1, "Enstatite": 1, "Takish Ivory": 1,
    "Nephrite": 2, "Heartsblood Alloy": 3, "Mindflow Crystals": 3,
}
MUNDANE = "No Gem/Alloy"
# a cell's stat word -> itempattern column
STAT_COL = {"AC": "AC", "Str": "str", "Sta": "sta", "Agi": "agi", "Dex": "dex", "Wis": "wis", "Int": "intelligence",
            "Cha": "cha", "Hp": "HPMAX", "Pow": "POWMAX", "FR": "FR", "CR": "CR", "LR": "LR", "AR": "AR", "PR": "PR",
            "DR": "DR", "HoT": "HoT", "PoT": "PoT"}
STAT_COLS = sorted(set(STAT_COL.values()))
STAND_IN_ICONS = {848266163, -570739083}  # the old dump's (tools/fix_dump_weapons.py)

# ours: prices by rarity (a modifier this server lacked; a product's share for its modifier)
MOD_PRICE = [20, 100, 400, 1500]
MOD_WORTH = [50, 200, 600, 2000]
LOOT_TABLE = "craft modifiers"
LOOT_WEIGHT = [30, 9, 3, 1]          # ours: by rarity
LOOT_LEVELS = [(1, 60), (10, 60), (20, 60), (30, 60)]
LOOT_CHANCE = 3                      # ours: percent of every creature's corpses

# the starter (eqoatrade's tutorial: the trainer's kit; the recipe "under Crude Iron")
HAMMERS = ["Novice", "Initiate", "Assistant", "Apprentice", "Adept", "Journeyman", "Craftsman", "Artisan", "Master",
           "Grandmaster"]
STARTER_HAMMER = "Novice Armorsmith Hammer"
STARTER_EMBLEM = "Blacksmith's Emblem"
STARTER_SCROLL = "Blacksmith's Bracers Recipe"
STARTER_PRODUCT = "blacksmith's bracers"
STARTER_MOLD = "Chain Bracers Mold"

CRAFT_DDL = """CREATE TABLE IF NOT EXISTS craftrecipes (
    id          INTEGER PRIMARY KEY,
    skill       INTEGER NOT NULL,
    comp        INTEGER NOT NULL,
    level       INTEGER NOT NULL,
    rarity      INTEGER NOT NULL,
    name        TEXT NOT NULL,
    product     INTEGER NOT NULL,
    yield       INTEGER NOT NULL DEFAULT 1,
    ingredients TEXT NOT NULL,
    difficulty  INTEGER NOT NULL,
    xp          INTEGER NOT NULL,
    discover    INTEGER NOT NULL DEFAULT 1,
    scroll      INTEGER NOT NULL DEFAULT 0,
    description TEXT NOT NULL DEFAULT '',
    source      TEXT NOT NULL DEFAULT 'gm'
)"""
LOG_DDL = """CREATE TABLE IF NOT EXISTS tradeskill_seed (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    craft  TEXT NOT NULL,   -- armor | jewel
    tbl    TEXT NOT NULL,   -- itempattern | craftrecipes | npcitem | npcs | loottables | npcloot
    key    TEXT NOT NULL,   -- the row's key (npcloot: npc name and table name, JSON)
    action TEXT NOT NULL,   -- inserted | updated | deleted
    old    TEXT,            -- the row before (updated, deleted), JSON
    new    TEXT             -- the row written (inserted, updated), JSON
)"""
KEYS = {"itempattern": "patternid", "craftrecipes": "id", "npcitem": "itemid", "npcs": "id", "loottables": "id"}


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def norm(s):
    return re.sub(r"\s+", " ", (s or "").replace("`", "'")).strip().lower()


# ---- the grids


def cells_of(row_html):
    out = []
    for c in re.findall(r"<t[dh][^>]*>(.*?)</t[dh]>", row_html, flags=re.S | re.I):
        c = re.sub(r"<br\s*/?>", " ", c, flags=re.I)
        c = html.unescape(re.sub(r"<[^>]+>", " ", c))
        out.append(re.sub(r"\s+", " ", c).strip())
    return out


def read_grid(path):
    """A grid page's cells: {(trade level, modifier): cell text}."""
    with open(path, "rb") as f:
        t = f.read().decode("latin-1")
    out = {}
    levels = None
    for tr in re.findall(r"<tr.*?</tr>", t, flags=re.S | re.I):
        row = [c for c in cells_of(tr)]
        if not any(row):
            continue
        if row[0].lower() == "gem":
            levels = [int(re.search(r"\d+", c).group()) for c in row[1:] if re.search(r"\d+", c)]
            continue
        if levels is None or len(row) < 2 or row[0] not in MODIFIERS and row[0] != MUNDANE:
            continue
        for lvl, c in zip(levels, row[1:]):
            out[(lvl, row[0])] = c
    return out


def stats_of(cell):
    """A cell's AC and stats as itempattern columns, or None for N/C."""
    if not cell or cell.upper().startswith("N/C"):
        return None
    out = {}
    for part in cell.split(","):
        m = re.fullmatch(r"\s*(\d+)\s+([A-Za-z]+)\s*", part)
        if not m or m.group(2) not in STAT_COL:
            raise ValueError(f"a cell's stat not read: {cell!r}")
        out[STAT_COL[m.group(2)]] = int(m.group(1))
    return out


def read_grids(folder):
    """{piece key: {(trade level, modifier): stats}} off the grid pages, and
    the modifier stats by (trade level, modifier) every piece agrees on."""
    grids, bonus, problems = {}, {}, []
    for key in PIECES:
        path = os.path.join(folder, "eqoatrade", "Recipes", "Armor", "Original", key)
        if not os.path.exists(path):
            sys.exit(f"{path}: missing (tools/fetch_tradeskill_sources.py --out {folder})")
        cells = {}
        for (lvl, mod), c in read_grid(path).items():
            s = stats_of(c)
            if s is None:
                continue
            cells[(lvl, mod)] = s
        grids[key] = cells
        for (lvl, mod), s in cells.items():
            if mod == MUNDANE:
                continue
            extra = {k: v for k, v in s.items() if k != "AC"}
            if s.get("AC") != cells.get((lvl, MUNDANE), {}).get("AC"):
                problems.append(f"{key} trade level {lvl} {mod}: AC {s.get('AC')} is not the mundane one's")
            if bonus.setdefault((lvl, mod), extra) != extra:
                problems.append(f"{key} trade level {lvl} {mod}: {extra} where another piece has {bonus[(lvl, mod)]}")
    return grids, bonus, problems


VENDOR_PAGES = {
    "armor": ("Armor.htm", {"tutorial": "trainer", "hammers": "hammers", "molds and patterns": "molds", "metal blocks": "metal"}),
    "jewel": ("Jewelry.htm", {"tutorial": "trainer", "kits": "hammers", "molds, bars, glass": "molds"}),
}


def read_vendors(folder, craft="armor"):
    """eqoatrade's vendor page: {city: {"trainer", "hammers" (the tools), "molds", "metal": name}}."""
    page, want = VENDOR_PAGES[craft]
    path = os.path.join(folder, "eqoatrade", "Vendors", page)
    with open(path, "rb") as f:
        t = f.read().decode("latin-1")
    t = re.sub(r"(?is)<(script|style).*?</\1>", " ", t)
    t = re.sub(r"(?i)<br\s*/?>|</(p|tr|li|h\d|div)>", "\n", t)
    t = html.unescape(re.sub(r"<[^>]+>", " ", t))
    lines = [re.sub(r"\s+", " ", ln).strip() for ln in t.splitlines()]
    out, city = {}, None
    roles = "|".join(re.escape(k) for k in sorted(want, key=len, reverse=True))
    for ln in lines:
        m = re.match(rf"^({roles}) - (.+)$", ln, flags=re.I)
        if not m:
            # a city's line: "<city> - <where in it>" (Armor's "Near ..."/"By ...", Jewelry's "Outside of ...")
            c = re.match(r"^([A-Z][A-Za-z' ]+?) - \S", ln)
            if c:
                city = c.group(1).strip()
                out[city] = {}
            continue
        if city:
            name = re.sub(r"\(.*", "", m.group(2)).strip()
            name = re.sub(r"^Mecha?n?t?\b|^Merchan\b", "Merchant", name)
            if "?" in name or name.lower().startswith("couldn"):
                continue
            out[city][want[m.group(1).lower()]] = name
    return out


# ---- the database


def item_by_name(db, name):
    """The pattern of the name (any case): the one a price or a component flag
    marks, else the lowest id."""
    rows = db.execute("SELECT patternid, itemcost, craft FROM itempattern WHERE lower(itemname) = ? "
                      "ORDER BY (itemcost > 0) DESC, (craft > 0) DESC, patternid", (norm(name),)).fetchall()
    return rows[0][0] if rows else None


def crafted_record(db, name, level, slot):
    """This server's record of a product, made by a live craftsman: the name,
    the product's level and its slot (a found or bought "Steel Chain Tunic" of
    level 30 is another item than the crafted one of 23)."""
    r = db.execute("SELECT patternid FROM itempattern WHERE lower(itemname) = ? AND levelreq = ? AND equipslot = ? "
                   "ORDER BY patternid LIMIT 1", (norm(name), level, slot)).fetchone()
    return r[0] if r else None


def commonest(db, sql, args=()):
    r = db.execute(f"SELECT x, count(*) FROM ({sql}) WHERE x IS NOT NULL AND x != 0 AND x NOT IN "
                   f"({', '.join(str(i) for i in STAND_IN_ICONS)}) GROUP BY x ORDER BY count(*) DESC, x LIMIT 1", args).fetchone()
    return r[0] if r else None


def harvest_components(harvests, want=None):
    """Sandstorm's own records of a craft's tools, molds and metals (want: the
    names; Armor's by default), by name, off every harvest file's merchants (a
    file of another kind is passed over)."""
    files = []
    for p in harvests:
        files += sorted(glob.glob(os.path.join(p, "*.json"))) if os.path.isdir(p) else [p]
    if want is None:
        want = {f"{h} Armorsmith Hammer" for h in HAMMERS} | {m for _, _, _, m in PIECES.values()} | \
               {m for _, _, _, m in EXTRA_SHIELDS.values()} | {f"{m} Ore" for m in METALS}
    got = {}
    for f in files:
        try:
            with open(f) as fh:
                d = json.load(fh)
        except (OSError, ValueError):
            continue
        for m in d.get("merchants", []) if isinstance(d, dict) else []:
            for w in m.get("wares") or []:
                it = w.get("item", w)
                if it.get("name") in want and it["name"] not in got:
                    row, _ = item_row(it)
                    got[it["name"]] = row
    return got, sorted(want - set(got))


def plan(db, folder, harvests):
    """What --apply writes: items, recipes, stock, loot; and what was left."""
    grids, bonus, problems = read_grids(folder)
    vendors = read_vendors(folder)
    comps, missing = harvest_components(harvests)
    if missing:
        problems.append(f"the harvests lack Sandstorm's records of: {', '.join(missing)}")
    cols = [r[1] for r in db.execute("PRAGMA table_info(itempattern)")]
    taken = {r[0]: r[1] for r in db.execute("SELECT patternid, itemname FROM itempattern")}

    items = []  # (patternid or None, row): rows to insert
    ids = {}    # name -> patternid (existing, or planned with a fixed id; None until inserted)
    for name, row in sorted(comps.items()):
        have = item_by_name(db, name)
        if have is not None:
            ids[name] = have
            continue
        pid = row["patternid"]
        if pid in taken:
            pid = None  # Sandstorm's id holds another item here: a new one
        r = {c: v for c, v in row.items() if c in cols}
        r.pop("patternid", None)
        r.setdefault("model", 0)  # a harvested record has no model or tint: a component shows nothing
        r.setdefault("color", 0)
        items.append((pid, r))
        ids[name] = pid

    gem_icon = commonest(db, "SELECT itemicon AS x FROM itempattern WHERE craft > 0 AND equipslot = -1 AND "
                             "lower(itemname) IN (" + ",".join("?" * len(MODIFIERS)) + ") AND lower(itemname) NOT LIKE '% alloy'",
                         [n.lower() for n in MODIFIERS])
    alloy_icon = commonest(db, "SELECT itemicon AS x FROM itempattern WHERE craft > 0 AND lower(itemname) LIKE '% alloy'")
    for name, rarity in MODIFIERS.items():
        have = item_by_name(db, name)
        if have is not None:
            ids[name] = have
            continue
        icon = alloy_icon if name.endswith("Alloy") or name.endswith("Crystals") else gem_icon
        items.append((None, base_item(name, icon or 0, MOD_PRICE[rarity], 0, -1, maxstack=50, craft=1,
                                      desc=f"A {['common', 'uncommon', 'rare', 'ultra rare'][rarity]} {'alloy' if 'Alloy' in name else 'gem'} an armorsmith works into his armor.")))
        ids[name] = None

    hammer_icon = commonest(db, "SELECT itemicon AS x FROM itempattern WHERE lower(itemname) LIKE '%smithing hammer%'")
    emblem_icon = commonest(db, "SELECT itemicon AS x FROM itempattern WHERE lower(itemname) LIKE '%emblem%'") or hammer_icon
    scroll_icon = commonest(db, "SELECT itemicon AS x FROM itempattern WHERE lower(itemname) LIKE '%recipe%'") or hammer_icon
    for name, icon, price, desc, craft in [
        (STARTER_EMBLEM, emblem_icon, 5, "A smith's emblem, worked into a first piece of armor.", 1),
        (STARTER_SCROLL, scroll_icon, 10, "A recipe for blacksmith's bracers. Use it to scribe the recipe.", 0),
    ]:
        if item_by_name(db, name) is None:
            items.append((None, base_item(name, icon or 0, price, 0, -1, maxstack=1 if craft == 0 else 20, craft=craft, desc=desc)))
            ids[name] = None
        else:
            ids[name] = item_by_name(db, name)

    # the products
    icon_cache = {}

    def product_icon(slot, aset):
        if (slot, aset) not in icon_cache:
            icon_cache[(slot, aset)] = (
                commonest(db, "SELECT itemicon AS x FROM itempattern WHERE equipslot = ? AND model = ?", (slot, aset))
                or commonest(db, "SELECT itemicon AS x FROM itempattern WHERE equipslot = ?", (slot,)) or 0)
        return icon_cache[(slot, aset)]

    shield_model = {}
    for kind, word in SHIELD_WORD.items():
        shield_model[kind] = commonest(db, "SELECT model AS x FROM itempattern WHERE equipslot = 13 AND lower(itemname) LIKE ?",
                                       (f"%{word}%",)) or 0

    def shield_icon(model):
        return commonest(db, "SELECT itemicon AS x FROM itempattern WHERE equipslot = 13 AND model = ?", (model,)) or \
            product_icon(13, 0)

    products = []  # (piece key, trade level, modifier or None, name, row, rarity, mold name)
    pieces = dict(PIECES)
    pieces.update({f"extra:{k}": v for k, v in EXTRA_SHIELDS.items()})
    for key, (slot, kind, words, mold) in pieces.items():
        if key.startswith("extra:"):
            ac = EXTRA_SHIELD_AC[kind]
            cells = {}
            for lvl in range(1, 11):
                if ac[lvl - 1] is None:
                    continue
                cells[(lvl, MUNDANE)] = {"AC": ac[lvl - 1]}
                for (bl, mod), extra in bonus.items():
                    if bl == lvl:
                        cells[(lvl, mod)] = dict(extra, AC=ac[lvl - 1])
        else:
            cells = grids[key]
        for (lvl, mod), stats in sorted(cells.items()):
            metal = METALS[lvl - 1]
            word = words[0 if lvl <= 5 else 1]
            modw = "" if mod == MUNDANE else re.sub(r" (Alloy|Crystals)$", "", mod).lower() + " "
            name = f"{modw}{metal.lower()} {word}"
            level = 5 * lvl - 2
            rarity = 0 if mod == MUNDANE else MODIFIERS[mod]
            if kind in ARMOR_SET:
                model, icon = ARMOR_SET[kind][0 if lvl <= 5 else 1], None
                icon = product_icon(slot, model)
            else:
                model = shield_model[kind]
                icon = shield_icon(model)
            ore_price = (comps.get(f"{metal} Ore") or {}).get("itemcost") or 10 * lvl * lvl
            price = 3 * ore_price + 10 + (0 if mod == MUNDANE else MOD_WORTH[rarity])
            what = "A shield" if slot == 13 else "Armor"
            row = base_item(name, icon, price, level, slot, maxstack=1, craft=0, classes=CLASSES[kind],
                            desc=f"{what} forged of {metal.lower()}.", model=model, color=TINT[lvl - 1],
                            maxhp=200 * level + BONUS_HP[lvl - 1], duration=50 + BONUS_DUR[lvl - 1])
            for c, v in stats.items():
                row[c] = v
            ings = [(f"{metal} Ore", 1, False), (mold, 1, False)] + ([] if mod == MUNDANE else [(mod, 1, True)])
            products.append({"name": name, "row": row, "have": crafted_record(db, name, level, slot), "comp": lvl - 1,
                             "level": lvl, "rarity": rarity, "ings": ings, "sort": (lvl, rarity, name), "kind": kind,
                             "magical": mod != MUNDANE})
    # the starter: crude iron, the chain bracers' mold and the emblem (eqoatrade's tutorial: level 1, AC 1)
    starter = base_item(STARTER_PRODUCT, product_icon(7, 3), 2, 1, 7, maxstack=1, craft=0, classes=ANY_CLASS,
                        desc="Bracers a smith's apprentice makes first.", model=3, color=TINT[0], maxhp=200 + BONUS_HP[0],
                        duration=50 + BONUS_DUR[0])
    starter["AC"] = 1
    products.append({"name": STARTER_PRODUCT, "row": starter, "have": None, "comp": 0, "level": 1, "rarity": 0,
                     "ings": [(f"{METALS[0]} Ore", 1, False), (STARTER_MOLD, 1, False), (STARTER_EMBLEM, 1, False)],
                     "sort": (99,), "kind": "starter", "magical": False, "scroll": STARTER_SCROLL, "discover": 0,
                     "difficulty": 1, "xp": 10, "description": "The first piece of armor a smith's apprentice makes."})
    names = collections.Counter(p["name"] for p in products)
    for n, k in names.items():
        if k > 1:
            problems.append(f"{k} products named {n!r}")

    # the suppliers: each city's three merchants, and the trainer (a quest gives the kit)
    stock, left = [], []
    hammers = [f"{h} Armorsmith Hammer" for h in HAMMERS]
    molds = [m for _, _, _, m in PIECES.values()] + [m for _, _, _, m in EXTRA_SHIELDS.values()]
    ores = [f"{m} Ore" for m in METALS]
    for city, v in sorted(vendors.items()):
        for role, wares in (("hammers", hammers), ("molds", molds), ("metal", ores)):
            name = v.get(role)
            if not name:
                left.append(f"{city}: no {role} seller on eqoatrade's list")
                continue
            r = db.execute("SELECT id, npc_id, npc_type FROM npcs WHERE lower(replace(npc_name, '`', '''')) = ? ORDER BY id",
                           (norm(name),)).fetchall()
            if not r:
                left.append(f"{city}: {name} ({role}) is no NPC here")
                continue
            if len(r) > 1:
                left.append(f"{city}: {len(r)} NPCs named {name}: the first stocked")
            stock.append((city, role, name, r[0], wares))
    loot = {"table": LOOT_TABLE, "mods": list(MODIFIERS.items()),
            "note": "the armorsmiths' gems and alloys: any creature (eqoatrade: random)"}
    return {"skill": ARMOR, "cells": sum(len(v) for v in grids.values()), "vendors": vendors, "comps": comps, "items": items,
            "ids": ids, "products": products, "stock": stock, "left": left, "problems": problems, "loot": loot}


def base_item(name, icon, price, level, slot, maxstack, craft, classes=ANY_CLASS, desc="", model=0, color=0,
              maxhp=0, duration=50):
    return {"itemcost": price, "unk1": 0, "itemicon": icon, "unk2": 0, "equipslot": slot, "unk3": 0, "trade": 0, "rent": 0,
            "unk4": 0, "attacktype": 0, "weapondamage": 0, "unk5": 0, "levelreq": level, "maxstack": maxstack,
            "maxhp": maxhp, "duration": duration, "classuse": classes, "raceuse": ANY_RACE, "procanim": 0, "lore": 0,
            "unk6": 0, "craft": craft, "itemname": name, "itemdesc": desc, "model": model, "color": color}


CRAFT = {"name": "armor"}  # the craft being written: what log() files each change under


def log(db, tbl, key, action, old=None, new=None):
    db.execute("INSERT INTO tradeskill_seed (craft, tbl, key, action, old, new) VALUES (?, ?, ?, ?, ?, ?)",
               (CRAFT["name"], tbl, json.dumps(key) if isinstance(key, (list, tuple)) else str(key), action,
                None if old is None else json.dumps(old, sort_keys=True), None if new is None else json.dumps(new, sort_keys=True)))


def row_of(db, tbl, key):
    cols = [r[1] for r in db.execute(f"PRAGMA table_info({tbl})")]
    r = db.execute(f"SELECT * FROM {tbl} WHERE {KEYS[tbl]} = ?", (key,)).fetchone()
    return None if r is None else dict(zip(cols, r))


def insert(db, tbl, row, key=None):
    cols = list(row)
    if key is not None:
        cols = [KEYS[tbl]] + cols
        vals = [key] + [row[c] for c in row]
    else:
        vals = [row[c] for c in row]
    cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", vals)
    k = key if key is not None else cur.lastrowid
    log(db, tbl, k, "inserted", new=row_of(db, tbl, k))
    return k


def apply(db, p):
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_DDL)
        db.execute(CRAFT_DDL)
        ids = dict(p["ids"])
        # Sandstorm's ids first: a new item takes the highest id + 1, which must not be one of theirs
        for pid, row in sorted(p["items"], key=lambda x: x[0] is None):
            ids[row["itemname"]] = insert(db, "itempattern", row, pid)
        # the recipes, in the book's order: by metal, then rarity and name
        nxt = (db.execute("SELECT max(id) FROM craftrecipes").fetchone()[0] or 0) + 1
        recipes = made = 0
        for pr in sorted(p["products"], key=lambda x: x["sort"]):
            if pr["have"] is not None:
                pid = pr["have"]
            else:
                pid = insert(db, "itempattern", pr["row"])
                made += 1
            lvl, rarity = pr["level"], pr["rarity"]
            insert(db, "craftrecipes", {
                "skill": p["skill"], "comp": pr["comp"], "level": lvl, "rarity": rarity, "name": pr["name"], "product": pid,
                "yield": 1, "ingredients": json.dumps([{"item": ids[n], "count": c, "keep": k} for n, c, k in pr["ings"]]),
                "difficulty": pr.get("difficulty", 10 * lvl + 2 * rarity), "xp": pr.get("xp", 25 * lvl * (1 + rarity)),
                "discover": pr.get("discover", 1), "scroll": ids[pr["scroll"]] if pr.get("scroll") else 0,
                "description": pr.get("description", ""), "source": SOURCE}, nxt)
            nxt += 1
            recipes += 1
        # the suppliers
        stand_in = set()
        if has_table(db, "merchant_stock"):
            stand_in = {k for (k,) in db.execute("SELECT key FROM merchant_stock WHERE what = 'catalog' AND note = 'general goods'")}
        wares_n = removed = 0
        for city, role, name, (npc, npc_id, typ), wares in p["stock"]:
            now = row_of(db, "npcs", npc)
            if not (now["npc_type"] or 0) & 0x01:
                db.execute("UPDATE npcs SET npc_type = ? WHERE id = ?", ((now["npc_type"] or 0) | 0x01, npc))
                log(db, "npcs", npc, "updated", old={"npc_type": now["npc_type"]}, new={"npc_type": (now["npc_type"] or 0) | 0x01})
            if not now["npc_id"]:
                db.execute("UPDATE npcs SET npc_id = ? WHERE id = ?", (npc, npc))
                log(db, "npcs", npc, "updated", old={"npc_id": now["npc_id"]}, new={"npc_id": npc})
            catalog = now["npc_id"] or npc
            for item_id, in db.execute("SELECT itemid FROM npcitem WHERE npcid = ? ORDER BY itemid", (catalog,)).fetchall():
                if item_id in stand_in:
                    old = row_of(db, "npcitem", item_id)
                    db.execute("DELETE FROM npcitem WHERE itemid = ?", (item_id,))
                    log(db, "npcitem", item_id, "deleted", old=old)
                    removed += 1
            have = {r[0] for r in db.execute("SELECT patternid FROM npcitem WHERE npcid = ?", (catalog,))}
            n = (db.execute("SELECT max(listnumber) FROM npcitem WHERE npcid = ?", (catalog,)).fetchone()[0] or -1) + 1
            for w in wares:
                if ids[w] in have:
                    continue
                insert(db, "npcitem", {"npcid": catalog, "stackleft": 1, "remainHP": 0, "remaincharge": 0, "patternid": ids[w],
                                       "equiploc": -1, "location": 1, "listnumber": n})
                have.add(ids[w])
                n += 1
                wares_n += 1
        # the modifiers' drops, on every creature
        loot = 0
        lt = p["loot"]
        for mod, rarity in lt["mods"]:
            lo, hi = LOOT_LEVELS[rarity]
            insert(db, "loottables", {"tablename": lt["table"], "patternid": ids[mod], "weight": LOOT_WEIGHT[rarity],
                                      "minlevel": lo, "maxlevel": hi, "source": SOURCE})
            loot += 1
        if db.execute("SELECT 1 FROM npcloot WHERE npc_name = '*' AND tablename = ?", (lt["table"],)).fetchone() is None:
            db.execute("INSERT INTO npcloot (npc_name, tablename, chance, rolls, note, source) VALUES ('*', ?, ?, 1, ?, ?)",
                       (lt["table"], LOOT_CHANCE, lt["note"], SOURCE))
            log(db, "npcloot", ["*", lt["table"]], "inserted", new={"chance": LOOT_CHANCE, "rolls": 1})
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return {"items": len(p["items"]) + made, "recipes": recipes, "wares": wares_n, "stand-ins removed": removed,
            "loot rows": loot}


def logged(db, craft):
    return has_table(db, "tradeskill_seed") and db.execute(
        "SELECT count(*) FROM tradeskill_seed WHERE craft = ?", (craft,)).fetchone()[0] > 0


def undo(db, craft):
    if not logged(db, craft):
        print(f"nothing of {craft} logged: nothing to undo")
        return
    db.execute("BEGIN IMMEDIATE")
    try:
        back = kept = 0
        for seq, tbl, key, action, old, new in db.execute(
                "SELECT seq, tbl, key, action, old, new FROM tradeskill_seed WHERE craft = ? ORDER BY seq DESC", (craft,)).fetchall():
            old = json.loads(old) if old else None
            new = json.loads(new) if new else None
            if tbl == "npcloot":
                name, table = json.loads(key)
                r = db.execute("SELECT chance, rolls FROM npcloot WHERE npc_name = ? AND tablename = ?", (name, table)).fetchone()
                if r is not None and {"chance": r[0], "rolls": r[1]} == new:
                    db.execute("DELETE FROM npcloot WHERE npc_name = ? AND tablename = ?", (name, table))
                    back += 1
                else:
                    kept += 1
                continue
            k = int(key)
            now = row_of(db, tbl, k)
            if action == "inserted":
                if now == new:
                    db.execute(f"DELETE FROM {tbl} WHERE {KEYS[tbl]} = ?", (k,))
                    back += 1
                else:
                    kept += 1
            elif action == "updated":
                if now is not None and all(now.get(c) == v for c, v in new.items()):
                    db.execute(f"UPDATE {tbl} SET {', '.join(c + ' = ?' for c in old)} WHERE {KEYS[tbl]} = ?", [*old.values(), k])
                    back += 1
                else:
                    kept += 1
            elif action == "deleted":
                if now is None:
                    cols = list(old)
                    db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [old[c] for c in cols])
                    back += 1
                else:
                    kept += 1
        db.execute("DELETE FROM tradeskill_seed WHERE craft = ?", (craft,))
        if db.execute("SELECT count(*) FROM tradeskill_seed").fetchone()[0] == 0:
            db.execute("DROP TABLE tradeskill_seed")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"took back {back} changes; {kept} changed since left as they are")


# ---- Jewelcrafting


JEWEL = 3  # the skill: CLIENT's craft slot
# Prima p. 443, "Metals": the order is CLIENT's Jewel list (spec 3.2), its Special tab after them
JEWEL_METALS = ["Iron", "Bronze", "Steel", "Copper", "Silver", "Electrum", "Gold", "Platinum", "Adamantite", "Mithril"]
JEWEL_BONUS_DUR = [0, 2, 4, 6, 8, 10, 12, 15, 18, 20]
JEWEL_SPECIAL = 10
# Sandstorm's ten kits, a tier a metal (Freeport's Merchant Gillinen), and its four wax molds and ten bars (Merchant
# Rothien): the tool held while working, worn in Held as a hammer is (wear slot 22 + 3)
KITS = ["Novice", "Initiate", "Assistant", "Apprentice", "Adept", "Journeyman", "Expert", "Artisan", "Master", "Grandmaster"]
# p. 443, "Casting Molds": a mold's piece and its equip slot (Ring, Ear, Neck, one Forearm)
JEWEL_MOLDS = {"Ring Mold": ("ring", 8), "Earring Mold": ("earring", 3), "Pendant Mold": ("pendant", 4),
               "Bracelet Mold": ("bracelet", 6)}
# p. 442, "Gem Properties": its columns left to right, as their rotated labels stand (Lightning before Cold; Sandstorm's
# Turquoise is "cold resistance" and its Topaz "lightning and arcane", as these columns read)
GEM_LABELS = ["Str", "Sta", "Agi", "Dex", "Wis", "Int", "Chr", "HP Max", "PWR Max", "HoT", "PoT", "AC", "Fire", "Lightening",
              "Cold", "Arcane", "Poison", "Disease"]
GEM_COLS = ["str", "sta", "agi", "dex", "wis", "intelligence", "cha", "HPMAX", "POWMAX", "HoT", "PoT", "AC", "FR", "LR", "CR",
            "AR", "PR", "DR"]
GEM_RARITY = {"common": 0, "uncommon": 1, "rare": 2, "ultra-rare": 3}
# a gem's item where this server's record is named otherwise than the table (Sandstorm's "Black Bloodstone", "These gems
# are often sought after by jewelers ... stamina"), and its word in a product's name, as the live records have them
# ("electrum bloodstone earring", "mithril rose quartz earring")
GEM_ITEM = {"Bloodstone": "Black Bloodstone"}
GEM_WORD = {"Bloodstone": "bloodstone", "Star Rose Quartz": "rose quartz"}
# The eight gems SOE changed on 5 Dec 2003: "we have removed these [HoT/PoT] effects and changed the stats of the
# jewelry crafting products made from: Ruby, Emerald, Sapphire, Star Ruby, Fire Emerald, Black Sapphire, Diamond, and
# Blue Diamond" (Allakhazam story 2762); Prima, a year later, prints them as before. Their stats as the live game's own
# records have them at one metal (in this server's items, out of Hagley's and Return Home's captures; EQOA-Data's
# gems.json gives the same kinds), the other metals along Prima's curve for that kind of stat (an attribute's or a
# resist's: Amber's row; hit points' or power's: Tourmaline's; AC: Pyrite's).
GEM_ANCHORS = {
    "Ruby": (9, {"HPMAX": 129}),                                                # adamantite ruby bracelet
    "Star Ruby": (10, {"str": 24, "HPMAX": 92, "PR": 24}),                      # mithril star ruby ring
    "Black Sapphire": (10, {"sta": 25, "HPMAX": 84, "POWMAX": 21, "AR": 25}),   # mithril black sapphire earring
    "Diamond": (10, {"cha": 24, "HPMAX": 84, "AC": 56}),                        # mithril diamond earring, pendant
}
# ours, for the four no record shows: a twin's stats where Prima's rows pair them; Sapphire ("hp, pow major") the
# Emerald's power with the plain hit points the diamond's and the black sapphire's records share; Blue Diamond Prima's
# cells, which already carry hit points, power and AC
GEM_TWINS = {"Emerald": ("Ruby", {"HPMAX": "POWMAX"}),
             "Fire Emerald": ("Star Ruby", {"str": "dex", "PR": "FR", "HPMAX": "POWMAX"})}
CHANGED_GEMS = ("Ruby", "Emerald", "Sapphire", "Star Ruby", "Fire Emerald", "Black Sapphire", "Diamond", "Blue Diamond")
GEM_PRICE = [20, 60, 270, 800]       # the gems Sandstorm prices (20, 60, 270); the ultra rare ours
JEWEL_LOOT_TABLE = "craft gems"
# the starter (eqoatrade's tutorial: the jeweler's kit, "a glass trinket recipe, 1 piece of glass, 1 piece of cracked
# glass, an earring mold, a pendant mold, and 1 iron bar"; the trinket by the recipe, the cracked glass earring by
# experiment, both under Special; their stats ours)
STARTER_KIT = "Novice Jewelers Kit"
GLASS, CRACKED = "Piece of Glass", "Piece of Cracked Glass"
TRINKET_SCROLL, TRINKET, CRACKED_EARRING = "Glass Trinket Recipe", "glass trinket", "cracked glass earring"
STAT_WORDS = {"str": "strength", "sta": "stamina", "agi": "agility", "dex": "dexterity", "wis": "wisdom",
              "intelligence": "intelligence", "cha": "charisma", "HPMAX": "vitality", "POWMAX": "power", "HoT": "health",
              "PoT": "power regeneration", "AC": "armor class", "FR": "fire", "LR": "lightning", "CR": "cold", "AR": "arcane",
              "PR": "poison", "DR": "disease"}
RESISTS = ("FR", "LR", "CR", "AR", "PR", "DR")
# Prima's city tables where eqoatrade's list names an NPC this server lacks: Neriak's "Jeweler K`Trath, Jewelry Making
# Tools" (eqoatrade: "Helenth K'Trath"). Surefall Glade has no kit seller in either.
JEWEL_SELLERS = {"Neriak": {"hammers": "Jeweler K`Trath"}}


def read_prima_jewel(folder):
    """Prima's Frontiers guide, pp. 442-443, out of its layout text (pdftotext
    -layout, tools/fetch_tradeskill_sources.py --craft jewel): each gem's
    rarity, its least trade level and the columns its marks stand under (Gem
    Properties, by the rotated labels' places), and its ten cells by metal (Gem
    & Metal Combinations, by the header's places; a cell's lines joined)."""
    path = os.path.join(folder, "prima", "prima-frontiers-layout.txt")
    with open(path, encoding="utf-8") as f:
        lines = f.read().split("\n")
    start = next(i for i, ln in enumerate(lines) if ln.strip() == "Jewel Crafting")
    end = next(i for i in range(start, len(lines)) if lines[i].strip() == "Tailoring")
    sec = lines[start:end]
    row = re.compile(r"^([A-Z][A-Za-z' ]+?)\s{2,}(Common|Uncommon|Rare|Ultra-Rare)\s+(\d+)\s+(.*)$")
    props, problems = {}, []
    i = 0
    while i < len(sec):
        if not sec[i].strip().startswith("Gem Properties"):
            i += 1
            continue
        xs = {}
        i += 1
        while i < len(sec) and not sec[i].startswith("Gem Name"):
            for lab in GEM_LABELS:
                m = re.search(r"(?<![A-Za-z])" + re.escape(lab) + r"(?![A-Za-z])", sec[i])
                if m:
                    xs.setdefault(lab, m.start() + len(lab) / 2)
            i += 1
        if len(xs) != len(GEM_LABELS):
            problems.append(f"Gem Properties: no label {sorted(set(GEM_LABELS) - set(xs))}")
            continue
        i += 1
        while i < len(sec):
            m = row.match(sec[i])
            if not m:
                break
            marks = [k.start() for k in re.finditer("[—X]", sec[i])][-len(GEM_LABELS):]
            cols = sorted({min(xs, key=lambda lab, x=x: abs(xs[lab] - x)) for x in marks if sec[i][x] == "X"},
                          key=GEM_LABELS.index)
            props[m.group(1)] = (GEM_RARITY[m.group(2).lower()], int(m.group(3)), [GEM_COLS[GEM_LABELS.index(c)] for c in cols])
            i += 1
    cells = {}
    i = 0
    while i < len(sec):
        if not sec[i].strip().startswith("Gem & Metal Combinations"):
            i += 1
            continue
        hdr = sec[i + 1]
        mx = [hdr.index(m) + len(m) / 2 for m in JEWEL_METALS]
        i += 2
        name = None
        while i < len(sec):
            ln = sec[i]
            if ln.strip().startswith("Gem & Metal Combinations"):
                break  # the table's next page (pdftotext puts a form feed before it too)
            m = re.match(r"^([A-Z][A-Za-z' ]+?)\s{2,}(\S.*)$", ln)
            if m and m.group(1) in props:
                name = m.group(1)
                cells[name] = [""] * 10
                body = " " * m.start(2) + ln[m.start(2):]
            elif ln.strip() and not ln.startswith(" "):
                break
            elif name and "+" in ln:
                body = ln
            else:
                i += 1
                continue
            for t in re.finditer(r"\S+", body):
                c = min(range(10), key=lambda k: abs(mx[k] - (t.start() + len(t.group()) / 2)))
                cells[name][c] += t.group()
            i += 1
    for g in props:
        if g not in cells:
            problems.append(f"no Gem & Metal row for {g}")
    return props, cells, problems


def gem_stats(props, cells, problems):
    """{gem: {trade level: {column: value}}}: Prima's cells as printed, but for
    the eight gems changed in 2003 (GEM_ANCHORS's comment)."""
    out = {}
    for gem, (_, _, cols) in props.items():
        if gem in CHANGED_GEMS:
            continue
        per = {}
        for t, c in enumerate(cells.get(gem, []), 1):
            if not c or c == "N\\A":
                continue
            vals = [int(v) for v in re.findall(r"\+(\d+)", c)]
            if len(vals) != len(cols):
                problems.append(f"{gem} at trade level {t}: {c!r} for {cols}")
                continue
            per[t] = dict(zip(cols, vals))
        out[gem] = per

    def curve(gem):
        return [int(c.lstrip("+")) for c in cells[gem]]
    curves = {"attr": curve("Amber"), "hp": curve("Tourmaline"), "ac": curve("Pyrite")}

    def along(col, at, value, t):
        c = curves["hp" if col in ("HPMAX", "POWMAX") else "ac" if col == "AC" else "attr"]
        return int(value * c[t - 1] / c[at - 1] + 0.5)
    anchors = dict(GEM_ANCHORS)
    for twin, (src, rename) in GEM_TWINS.items():
        at, vals = anchors[src]
        anchors[twin] = (at, {rename.get(k, k): v for k, v in vals.items()})
    at, vals = anchors["Emerald"]
    anchors["Sapphire"] = (10, {"HPMAX": GEM_ANCHORS["Diamond"][1]["HPMAX"], "POWMAX": along("POWMAX", at, vals["POWMAX"], 10)})
    for gem, (at, vals) in anchors.items():
        out[gem] = {t: {c: along(c, at, v, t) for c, v in vals.items()} for t in range(props[gem][1], 11)}
    label = {"HP": "HPMAX", "PM": "POWMAX", "Pwr": "POWMAX", "AC": "AC"}
    per = {}
    for t, c in enumerate(cells.get("Blue Diamond", []), 1):
        if c and c != "N\\A":
            per[t] = {label[lab]: int(v) for v, lab in re.findall(r"\+(\d+)([A-Za-z]+)", c)}
    out["Blue Diamond"] = per
    return out


def gem_desc(cols):
    """A gem's line, as Sandstorm's gems word theirs."""
    res = [STAT_WORDS[c] for c in cols if c in RESISTS]
    other = [STAT_WORDS[c] for c in cols if c not in RESISTS]
    if res:
        other.append(f"{' and '.join(res)} resistance{'s' if len(res) > 1 else ''}")
    words = other[0] if len(other) == 1 else ", ".join(other[:-1]) + " and " + other[-1]
    return f"These gems are often sought after by jewelers for their seemingly magical ability to enhance {words}."


def crafted_jewel(db, name, level, slot, row):
    """This server's record of a jewelry product, made by a live craftsman:
    the name (the live records' names stop at 32 letters), level and slot, and
    its hit points, durability and stats all the product's, so a found "Silver
    Pearl Earring" of another make, or a record without its stats, is another
    item."""
    names = sorted({name.lower(), name.lower()[:32]})
    cols = ["patternid", "maxhp", "duration"] + STAT_COLS
    for r in db.execute(f"SELECT {', '.join(cols)} FROM itempattern WHERE lower(itemname) IN ({','.join('?' * len(names))}) "
                        "AND levelreq = ? AND equipslot = ? ORDER BY patternid", (*names, level, slot)):
        rec = dict(zip(cols, r))
        if rec["maxhp"] == row["maxhp"] and rec["duration"] == row["duration"] and \
                all((rec[c] or 0) == row.get(c, 0) for c in STAT_COLS):
            return rec["patternid"]
    return None


def npc_named(db, name):
    return db.execute("SELECT id, npc_id, npc_type FROM npcs WHERE lower(replace(npc_name, '`', \"'\")) = ? ORDER BY id",
                      (norm(name),)).fetchall()


def plan_jewel(db, folder, harvests):
    """Jewelcrafting's items, recipes, stock and drops (the module's doc)."""
    problems = []
    props, cells, more = read_prima_jewel(folder)
    problems += more
    stats = gem_stats(props, cells, problems)
    vendors = read_vendors(folder, "jewel")
    want = {f"{k} Jewelers Kit" for k in KITS} | set(JEWEL_MOLDS) | {f"{m} Bar" for m in JEWEL_METALS}
    comps, missing = harvest_components(harvests, want)
    if missing:
        problems.append(f"the harvests lack Sandstorm's records of: {', '.join(missing)}")
    cols = [r[1] for r in db.execute("PRAGMA table_info(itempattern)")]
    taken = {r[0] for r in db.execute("SELECT patternid FROM itempattern")}
    items, ids = [], {}
    for name, row in sorted(comps.items()):
        have = item_by_name(db, name)
        if have is not None:
            ids[name] = have
            continue
        pid = row["patternid"] if row["patternid"] not in taken else None
        r = {c: v for c, v in row.items() if c in cols}
        r.pop("patternid", None)
        r.setdefault("model", 0)
        r.setdefault("color", 0)
        items.append((pid, r))
        ids[name] = pid

    # the gems: this server's own where it has them, else made as Sandstorm's are
    gem_rows = db.execute("SELECT itemicon, itemcost FROM itempattern WHERE craft > 0 AND equipslot = -1 AND "
                          "itemdesc LIKE '%sought after by jewelers%'").fetchall()
    for gem, (rarity, _, _) in props.items():
        name = GEM_ITEM.get(gem, gem)
        have = item_by_name(db, name)
        if have is not None:
            ids[name] = have
            continue
        price = GEM_PRICE[rarity]
        icons = collections.Counter(i for i, c in gem_rows if c == min(price, GEM_PRICE[2]))
        icon = sorted(icons.items(), key=lambda x: (-x[1], x[0]))[0][0] if icons else 0
        items.append((None, base_item(name, icon, price, 0, -1, maxstack=50, craft=1,
                                      desc=gem_desc(sorted(stats[gem][max(stats[gem])], key=GEM_COLS.index)))))
        ids[name] = None
    scroll_icon = commonest(db, "SELECT itemicon AS x FROM itempattern WHERE lower(itemname) LIKE '%recipe%'")
    glass_icon = commonest(db, "SELECT itemicon AS x FROM itempattern WHERE itemdesc LIKE '%sought after by jewelers%'")
    for name, icon, price, desc, craft, stack in [
        (TRINKET_SCROLL, scroll_icon, 10, "A recipe for a glass trinket. Use it to scribe the recipe.", 0, 1),
        (GLASS, glass_icon, 2, "A piece of clear glass, for a jeweler's first work.", 1, 50),
        (CRACKED, glass_icon, 1, "A piece of glass with a crack through it.", 1, 50),
    ]:
        have = item_by_name(db, name)
        if have is None:
            items.append((None, base_item(name, icon or 0, price, 0, -1, maxstack=stack, craft=craft, desc=desc)))
        ids[name] = have

    # the products: a gem, a metal and a mold, each piece's icon the live records'
    metal_re = "|".join(m.lower() for m in JEWEL_METALS)
    live = collections.defaultdict(collections.Counter)
    for name, slot, icon, hp in db.execute("SELECT itemname, equipslot, itemicon, maxhp FROM itempattern "
                                           "WHERE equipslot IN (3, 4, 6, 8)"):
        if hp and re.match(rf"^({metal_re}) .+ (ring|earring|pendant|bracelet|bracele)$", (name or "").lower()):
            live[slot][icon] += 1

    def piece_icon(slot):
        if live[slot]:
            return sorted(live[slot].items(), key=lambda x: (-x[1], x[0]))[0][0]
        return commonest(db, "SELECT itemicon AS x FROM itempattern WHERE equipslot = ?", (slot,)) or 0
    products, cells_n = [], 0
    for gem, (rarity, _, _) in props.items():
        for t, st in sorted(stats[gem].items()):
            cells_n += 1
            metal = JEWEL_METALS[t - 1]
            level = 5 * t - 2
            bar_price = (comps.get(f"{metal} Bar") or {}).get("itemcost") or 10 * t * t
            for mold, (piece, slot) in JEWEL_MOLDS.items():
                name = f"{metal.lower()} {GEM_WORD.get(gem, gem.lower())} {piece}"
                row = base_item(name, piece_icon(slot), 3 * bar_price + 10 + MOD_WORTH[rarity], level, slot, maxstack=1,
                                craft=0, desc=f"A{'n' if piece[0] in 'ae' else ''} {piece} cast of {metal.lower()}, set with "
                                              f"{'an' if gem[0] in 'AEIOU' else 'a'} {gem.lower()}.",
                                maxhp=200 * level, duration=50 + JEWEL_BONUS_DUR[t - 1])
                row.update(st)
                ings = [(f"{metal} Bar", 1, False), (mold, 1, False), (GEM_ITEM.get(gem, gem), 1, True)]
                products.append({"name": name, "row": row, "have": crafted_jewel(db, name, level, slot, row), "comp": t - 1,
                                 "level": t, "rarity": rarity, "ings": ings, "sort": (t, rarity, name), "kind": piece,
                                 "magical": True})
    for name, slot, stat, ings, extra in [
        (TRINKET, 4, "cha", [(GLASS, 1, True), ("Pendant Mold", 1, False)],
         {"scroll": TRINKET_SCROLL, "discover": 0, "description": "The first piece a jeweler's apprentice makes."}),
        (CRACKED_EARRING, 3, "intelligence", [(CRACKED, 1, True), ("Earring Mold", 1, False), ("Iron Bar", 1, False)],
         {"discover": 1, "description": "An apprentice's earring of cracked glass."}),
    ]:
        row = base_item(name, piece_icon(slot), 2, 1, slot, maxstack=1, craft=0, desc=f"A jeweler's first {name}.",
                        maxhp=200, duration=50)
        row[stat] = 1
        products.append(dict({"name": name, "row": row, "have": None, "comp": JEWEL_SPECIAL, "level": 1, "rarity": 0,
                              "ings": ings, "sort": (99, name), "kind": "starter", "magical": False, "difficulty": 1,
                              "xp": 10}, **extra))
    names = collections.Counter(p["name"] for p in products)
    problems += [f"{k} products named {n!r}" for n, k in names.items() if k > 1]

    stock, left = [], []
    kits = [f"{k} Jewelers Kit" for k in KITS]
    rest = list(JEWEL_MOLDS) + [f"{m} Bar" for m in JEWEL_METALS] + [GLASS, CRACKED]
    for city, v in sorted(vendors.items()):
        v = dict(v, **JEWEL_SELLERS.get(city, {}))
        for role, wares, what in (("hammers", kits, "kits"), ("molds", rest, "molds, bars and glass")):
            name = v.get(role)
            if not name:
                left.append(f"{city}: no {what} seller on eqoatrade's list")
                continue
            r = npc_named(db, name)
            if not r:
                left.append(f"{city}: {name} ({what}) is no NPC here")
                continue
            if len(r) > 1:
                left.append(f"{city}: {len(r)} NPCs named {name}: the first stocked")
            stock.append((city, role, name, r[0], wares))
    loot = {"table": JEWEL_LOOT_TABLE, "mods": [(GEM_ITEM.get(g, g), props[g][0]) for g in props],
            "note": "the jewelers' gems: any creature (eqoatrade: random)"}
    return {"skill": JEWEL, "cells": cells_n, "vendors": vendors, "comps": comps, "items": items, "ids": ids,
            "products": products, "stock": stock, "left": left, "problems": problems, "loot": loot}


PLANS = {"armor": plan, "jewel": plan_jewel}


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-tradeskills"
    shutil.copy2(path, dst)
    return dst


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--sources", help="tools/fetch_tradeskill_sources.py's folder")
    ap.add_argument("--harvests", nargs="*", default=[], help="eqoa harvest JSON files, or folders of them")
    ap.add_argument("--craft", default="armor", choices=sorted(PLANS))
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    CRAFT["name"] = a.craft
    if a.undo:
        if not a.no_backup and logged(db, a.craft):
            print(f"copy: {backup(a.db)}")
        undo(db, a.craft)
        return
    if not a.sources or not a.harvests:
        sys.exit("give --sources and --harvests")
    if a.apply and logged(db, a.craft):
        sys.exit(f"tradeskill_seed holds {a.craft}: applied already (--undo --craft {a.craft} first)")
    p = PLANS[a.craft](db, a.sources, a.harvests)
    per = collections.Counter(x["kind"] for x in p["products"])
    mods = collections.Counter("magical" if x["magical"] else "mundane" for x in p["products"])
    print(f"components from Sandstorm's records: {len(p['comps'])}; items to make: {len(p['items'])} "
          f"({sum(1 for pid, _ in p['items'] if pid)} under Sandstorm's ids)")
    print(f"products and recipes: {len(p['products'])} ({dict(mods)}; by kind {dict(per)})")
    for x in p["products"]:
        if x["have"] is not None:
            print(f"  {x['name']!r} is here already ({x['have']}): its recipe makes that record")
    print(f"cells read: {p['cells']}" + (" (Prima's count: 2,329)" if a.craft == "armor" else ""))
    for city, role, name, (npc, _, _), wares in p["stock"]:
        print(f"  {city}: {name} ({npc}) sells the {len(wares)} {role}")
    for ln in p["left"]:
        print(f"left: {ln}")
    for ln in p["problems"]:
        print(f"problem: {ln}")
    if p["problems"] and a.apply:
        sys.exit("problems: nothing written")
    if not a.apply:
        print("report only: --apply to write")
        return
    if not a.no_backup:
        print(f"copy: {backup(a.db)}")
    done = apply(db, p)
    print("wrote " + ", ".join(f"{v} {k}" for k, v in done.items()))


if __name__ == "__main__":
    main()
