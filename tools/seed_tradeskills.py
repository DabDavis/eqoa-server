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

**Weapons** (--craft weapon): Prima's Frontiers guide's Weaponcrafting tables
(pp. 446-447: the metals' bonus durability and hit points, the mundane damage
by hand, the stat and proc figures by trade level, a product a mold and trade
level), which eqoatrade's 1H and 2H grids are checked against (every filled
cell agrees: a difference is a problem and nothing is written); QuestMasters'
Weaponcrafting and gem pages (2006: id526, id459, id458) for who wields each
mold and the seventeen modifiers (Prima's fourteen starred ones, in no other
source, are left out). A recipe is an ore (Armor's: Sandstorm's), the mold and
from trade level 3 a modifier (kept on a failure); a gem gives its stat, an
alloy or crystal its proc in the words entity/weaponproc.go reads (a fiery,
freezing, cursed or withering weapon's hit, a malign, venomous, volatile or
ethereal one's damage over time, Prima's figure to twice it at Prima's chance;
a vampiric one's life tap, a siphoning one's mana tap, at trade levels 9 and
10). Names "<metal> <product>" and "<word> <metal> <product>" (eqoatrade's
"Makes Fiery xxx", "Cursed Adamantite Flamberge"); each product's look (model,
icon, colour) the commonest of this server's weapon records named as it is, a
mold's wear slot its products' looks' commonest. Sandstorm's hammers and
molds; eqoatrade's vendors (Prima's where eqoatrade's is no NPC here) and
starter (a dull knife by its recipe, a crude rusty mace by experiment, both
under Special); the modifiers drop from the loot categories' "modifiers".

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

    tools/seed_tradeskills.py [--db eqoa.db] --sources <folder> --harvests <dir or file> ... [--craft armor|jewel|weapon] [--apply | --undo]

In two steps, for a server that must not wait on the sources' reading (it takes ~25 s): --plan-out PLAN.json
against a copy of the target (decides everything, writes nothing), then --db <target> --plan-in PLAN.json --apply
(reads only the plan; refused unless the target is as the plan found it: the items it names or reuses, its fixed
ids, the merchants and their catalogs, the drop rows; one transaction, the same log, undo and audit line).
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
LOOT_CHANCE = 3                      # ours: percent of every creature's corpses (the modifiers)

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
    craft  TEXT NOT NULL,   -- armor | jewel | weapon
    tbl    TEXT NOT NULL,   -- itempattern | craftrecipes | npcitem | npcs | loottables | npcloot | lootpool | sqlite_sequence
    key    TEXT NOT NULL,   -- the row's key (npcloot: npc name and table name, JSON)
    action TEXT NOT NULL,   -- inserted | updated | deleted
    old    TEXT,            -- the row before (updated, deleted), JSON
    new    TEXT             -- the row written (inserted, updated), JSON
)"""
SEQ_TABLES = ["itempattern", "npcitem", "loottables", "lootpool"]  # the AUTOINCREMENT tables a seed writes
KEYS = {"itempattern": "patternid", "craftrecipes": "id", "npcitem": "itemid", "npcs": "id", "loottables": "id",
        "lootpool": "id"}


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
    "weapon": ("Weapon.htm", {"tutorial": "trainer", "hammers": "hammers", "molds": "molds", "metal blocks": "metal"}),
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
    loot = {"table": LOOT_TABLE, "chance": LOOT_CHANCE, "mods": list(MODIFIERS.items()),
            "note": "the armorsmiths' gems and alloys: any creature (eqoatrade: random)"}
    return {"skill": ARMOR, "cells": sum(len(v) for v in grids.values()), "vendors": vendors, "comps": comps, "items": items,
            "ids": ids, "products": products, "stock": stock, "left": left, "problems": problems, "loot": loot}


def base_item(name, icon, price, level, slot, maxstack, craft, classes=ANY_CLASS, desc="", model=0, color=0,
              maxhp=0, duration=50):
    # rent 1 is an ordinary item (CLIENT's m_rent: 0 is NO RENT, which the
    # bank and the auction refuse), as every live record of these kinds is.
    return {"itemcost": price, "unk1": 0, "itemicon": icon, "unk2": 0, "equipslot": slot, "unk3": 0, "trade": 0, "rent": 1,
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


def seq_of(db):
    """The AUTOINCREMENT counters of the tables a seed writes."""
    if not has_table(db, "sqlite_sequence"):
        return {}
    return dict(db.execute("SELECT name, seq FROM sqlite_sequence WHERE name IN (%s)" % ",".join("?" * len(SEQ_TABLES)),
                           SEQ_TABLES).fetchall())


def chunks(seq, n=500):
    seq = list(seq)
    for i in range(0, len(seq), n):
        yield seq[i:i + n]


def rows_by_key(db, tbl, keys):
    """Full rows of a table by their keys, a few queries for any number (row_of's dict, keyed)."""
    if not keys:
        return {}
    cols = [r[1] for r in db.execute(f"PRAGMA table_info({tbl})")]
    k = cols.index(KEYS[tbl])
    out = {}
    for part in chunks(keys):
        for r in db.execute(f"SELECT * FROM {tbl} WHERE {KEYS[tbl]} IN ({','.join('?' * len(part))})", part):
            out[r[k]] = dict(zip(cols, r))
    return out


def resolve(db, p):
    """Everything --apply will do, decided against the database as it is, as data (JSON-able): the items and
    products to make (a new one named by a ref, "@<name>", its id given when written), the recipes, the merchants'
    changes, the drops; and what of the database those decisions rest on ("assume", assumptions()). Reads only."""
    ref = {}  # item name -> an existing id, or "@name" for one this run makes
    for name, pid in p["ids"].items():
        ref[name] = pid
    items = []
    for pid, row in sorted(p["items"], key=lambda x: x[0] is None):  # Sandstorm's ids first (as one by one did)
        # one under Sandstorm's id is that id already (a catalog may list it: Sandstorm's own); else a ref
        ref[row["itemname"]] = pid or "@" + row["itemname"]
        items.append({"ref": ref[row["itemname"]], "fixed": pid, "row": row})
    for name, v in list(ref.items()):
        if v is None:
            raise SystemExit(f"{name}: no item and none planned")
    products, recipes = [], []
    for pr in sorted(p["products"], key=lambda x: x["sort"]):
        if pr["have"] is not None:
            pid = pr["have"]
        else:
            pid = "@@" + pr["name"]  # a product's own ref (a product may share a component's name: never here)
            products.append({"ref": pid, "row": pr["row"]})
        lvl, rarity = pr["level"], pr["rarity"]
        recipes.append({
            "skill": p["skill"], "comp": pr["comp"], "level": lvl, "rarity": rarity, "name": pr["name"], "product": pid,
            "yield": 1, "ingredients": [{"item": ref[n], "count": c, "keep": k} for n, c, k in pr["ings"]],
            "difficulty": pr.get("difficulty", 10 * lvl + 2 * rarity), "xp": pr.get("xp", 25 * lvl * (1 + rarity)),
            "discover": pr.get("discover", 1), "scroll": ref[pr["scroll"]] if pr.get("scroll") else 0,
            "description": pr.get("description", ""), "source": SOURCE})
    # the suppliers, each catalog as it stands and as earlier merchants of this run leave it
    stand_in = set()
    if has_table(db, "merchant_stock"):
        stand_in = {k for (k,) in db.execute("SELECT key FROM merchant_stock WHERE what = 'catalog' AND note = 'general goods'")}
    npc_rows = rows_by_key(db, "npcs", sorted({npc for _, _, _, (npc, _, _), _ in p["stock"]}))
    npc_now = {k: {"npc_type": v["npc_type"], "npc_id": v["npc_id"]} for k, v in npc_rows.items()}
    npc_upd, del_rows, adds, catalogs = [], [], [], {}
    for city, role, name, (npc, npc_id, typ), wares in p["stock"]:
        now = npc_now[npc]
        if not (now["npc_type"] or 0) & 0x01:
            npc_upd.append({"id": npc, "old": {"npc_type": now["npc_type"]}, "new": {"npc_type": (now["npc_type"] or 0) | 0x01}})
            now["npc_type"] = (now["npc_type"] or 0) | 0x01
        if not now["npc_id"]:
            npc_upd.append({"id": npc, "old": {"npc_id": now["npc_id"]}, "new": {"npc_id": npc}})
            now["npc_id"] = npc
        catalog = now["npc_id"] or npc
        if catalog not in catalogs:
            cols = [r[1] for r in db.execute("PRAGMA table_info(npcitem)")]
            catalogs[catalog] = [dict(zip(cols, r)) for r in db.execute("SELECT * FROM npcitem WHERE npcid = ? ORDER BY itemid", (catalog,))]
        rows = catalogs[catalog]
        for r in [r for r in rows if r.get("itemid") in stand_in]:
            del_rows.append(r)
            rows.remove(r)
        have = {r["patternid"] for r in rows}
        n = max([r["listnumber"] for r in rows if r["listnumber"] is not None], default=-1) + 1
        for w in wares:
            if ref[w] in have:
                continue
            row = {"npcid": catalog, "stackleft": 1, "remainHP": 0, "remaincharge": 0, "patternid": ref[w], "equiploc": -1,
                   "location": 1, "listnumber": n}
            adds.append(row)
            rows.append(dict(row, itemid=None))
            have.add(ref[w])
            n += 1
    pool, tables, npcloot = [], [], None
    lt = p["loot"]
    if lt.get("pool"):
        existing = set()
        known = [ref[m] for m, _ in lt["mods"] if isinstance(ref[m], int)]
        for part in chunks(known):
            existing |= {r[0] for r in db.execute("SELECT patternid FROM lootpool WHERE category = ? AND region = '*' AND "
                                                  f"npc_name = '' AND patternid IN ({','.join('?' * len(part))})", [lt["pool"], *part])}
        for mod, rarity in lt["mods"]:
            if ref[mod] in existing:
                continue
            lo, hi = LOOT_LEVELS[rarity]
            pool.append({"category": lt["pool"], "region": "*", "npc_name": "", "patternid": ref[mod],
                         "weight": LOOT_WEIGHT[rarity], "minlevel": lo, "maxlevel": hi, "source": SOURCE})
    else:
        for mod, rarity in lt["mods"]:
            lo, hi = LOOT_LEVELS[rarity]
            tables.append({"tablename": lt["table"], "patternid": ref[mod], "weight": LOOT_WEIGHT[rarity],
                           "minlevel": lo, "maxlevel": hi, "source": SOURCE})
        if db.execute("SELECT 1 FROM npcloot WHERE npc_name = '*' AND tablename = ?", (lt["table"],)).fetchone() is None:
            npcloot = {"tablename": lt["table"], "chance": lt["chance"], "note": lt["note"]}
    ops = {"version": 1, "craft": CRAFT["name"], "items": items, "products": products, "recipes": recipes,
           "npcs": npc_upd, "npcitem_del": del_rows, "npcitem_add": adds, "lootpool": pool, "loottables": tables,
           "npcloot": npcloot, "catalogs": sorted(catalogs), "pool": lt.get("pool"), "loot_table": lt.get("table"),
           "mod_ids": sorted({ref[m] for m, _ in lt["mods"] if isinstance(ref[m], int)}),
           "stock_npcs": sorted(npc_rows)}
    ops["assume"] = assumptions(db, ops)
    return ops


def assumptions(db, ops):
    """What of the database a plan's decisions rest on, read the same way when it is made and when it is applied:
    the existing items it refers to (id and name), every record named as an item or product it makes or reuses
    (whose presence decides make-or-reuse), its fixed ids, the merchants and their catalogs, the stand-ins among
    them, the drop rows of its modifiers. Equal, the plan is what a single --apply would do now."""
    names = sorted({x["row"]["itemname"].lower() for x in ops["items"]} | {r["name"].lower() for r in ops["recipes"]} |
                   {x["row"]["itemname"].lower() for x in ops["products"]})
    named = []
    for part in chunks(names):
        named += db.execute("SELECT patternid, lower(itemname), levelreq, equipslot FROM itempattern WHERE lower(itemname) IN "
                            f"({','.join('?' * len(part))})", part).fetchall()
    refs = sorted({i["item"] for r in ops["recipes"] for i in r["ingredients"] if isinstance(i["item"], int)} |
                  {r["product"] for r in ops["recipes"] if isinstance(r["product"], int)} |
                  {r["scroll"] for r in ops["recipes"] if isinstance(r["scroll"], int) and r["scroll"]} |
                  {a["patternid"] for a in ops["npcitem_add"] if isinstance(a["patternid"], int)} | set(ops["mod_ids"]))
    known = []
    for part in chunks(refs):
        known += db.execute(f"SELECT patternid, itemname FROM itempattern WHERE patternid IN ({','.join('?' * len(part))})",
                            part).fetchall()
    catalogs, mod_ids, pool = ops["catalogs"], ops["mod_ids"], ops["pool"]
    fixed = sorted(x["fixed"] for x in ops["items"] if x["fixed"])
    taken = []
    for part in chunks(fixed):
        taken += [r[0] for r in db.execute(f"SELECT patternid FROM itempattern WHERE patternid IN ({','.join('?' * len(part))})", part)]
    npcs = sorted(set(ops["stock_npcs"]) | set(catalogs))
    npc_state = [[k, v["npc_type"], v["npc_id"]] for k, v in sorted(rows_by_key(db, "npcs", npcs).items())]
    cat = []
    for part in chunks(catalogs):
        cat += db.execute(f"SELECT * FROM npcitem WHERE npcid IN ({','.join('?' * len(part))}) ORDER BY itemid", part).fetchall()
    stand = []
    if has_table(db, "merchant_stock") and cat:
        ids = {r[0] for r in cat}
        stand = sorted(k for (k,) in db.execute("SELECT key FROM merchant_stock WHERE what = 'catalog' AND note = 'general goods'")
                       if k in ids)
    drops = []
    if pool:
        for part in chunks(mod_ids):
            drops += db.execute(f"SELECT id, patternid FROM lootpool WHERE category = ? AND patternid IN ({','.join('?' * len(part))})",
                                [pool, *part]).fetchall()
    loot = db.execute("SELECT count(*) FROM npcloot WHERE npc_name = '*' AND tablename = ?", (ops["loot_table"],)).fetchone()[0] \
        if ops["loot_table"] and has_table(db, "npcloot") else 0
    return json.loads(json.dumps({"named": sorted(named), "known": sorted(known), "fixed_taken": taken, "npcs": npc_state,
                                  "catalogs": catalogs, "catalog_rows": cat, "stand_ins": stand, "drops": sorted(drops),
                                  "npcloot": loot}))


def write_ops(db, ops):
    """Write a resolved plan, in the transaction the caller holds: explicit ids (an AUTOINCREMENT's next, as one by
    one gave), every table's rows by executemany, then one log line a change (tradeskill_seed: the rows as written,
    read back a few hundred at a time) and the id counters. Returns the counts."""
    seqs = seq_of(db)

    def next_id(tbl, key, floor=0):
        a = db.execute(f"SELECT max({key}) FROM {tbl}").fetchone()[0] or 0
        return max(a, seqs.get(tbl) or 0, floor) + 1

    def put(tbl, rows):  # rows: dicts with their key; grouped by their columns for executemany
        groups = collections.defaultdict(list)
        for r in rows:
            groups[tuple(r)].append(r)
        for cols, rs in groups.items():
            db.executemany(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                           [[r[c] for c in cols] for r in rs])

    ids = {}
    fixed = [x for x in ops["items"] if x["fixed"]]
    nid = next_id("itempattern", "patternid", max([x["fixed"] for x in fixed], default=0))
    item_rows = []
    for x in ops["items"]:
        pid = x["fixed"]
        if not pid:
            pid, nid = nid, nid + 1
        ids[x["ref"]] = pid
        item_rows.append(dict({"patternid": pid}, **x["row"]))
    for x in ops["products"]:
        ids[x["ref"]], nid = nid, nid + 1
        item_rows.append(dict({"patternid": ids[x["ref"]]}, **x["row"]))

    def real(v):
        return ids[v] if isinstance(v, str) and v.startswith("@") else v
    put("itempattern", item_rows)
    rid = next_id("craftrecipes", "id")
    recipe_rows = []
    for r in ops["recipes"]:
        row = dict(r, id=rid, product=real(r["product"]), scroll=real(r["scroll"]),
                   ingredients=json.dumps([dict(i, item=real(i["item"])) for i in r["ingredients"]]))
        recipe_rows.append(row)
        rid += 1
    put("craftrecipes", recipe_rows)
    for u in ops["npcs"]:
        (col, v), = u["new"].items()
        db.execute(f"UPDATE npcs SET {col} = ? WHERE id = ?", (v, u["id"]))
    db.executemany("DELETE FROM npcitem WHERE itemid = ?", [(r["itemid"],) for r in ops["npcitem_del"]])
    iid = next_id("npcitem", "itemid")
    add_rows = []
    for a in ops["npcitem_add"]:
        add_rows.append(dict(a, itemid=iid, patternid=real(a["patternid"])))
        iid += 1
    put("npcitem", add_rows)
    pool_rows, table_rows = [], []
    if ops["lootpool"]:
        lid = next_id("lootpool", "id")
        for r in ops["lootpool"]:
            pool_rows.append(dict(r, id=lid, patternid=real(r["patternid"])))
            lid += 1
        put("lootpool", pool_rows)
    if ops["loottables"]:
        lid = next_id("loottables", "id")
        for r in ops["loottables"]:
            table_rows.append(dict(r, id=lid, patternid=real(r["patternid"])))
            lid += 1
        put("loottables", table_rows)
    if ops["npcloot"]:
        nl = ops["npcloot"]
        db.execute("INSERT INTO npcloot (npc_name, tablename, chance, rolls, note, source) VALUES ('*', ?, ?, 1, ?, ?)",
                   (nl["tablename"], nl["chance"], nl["note"], SOURCE))
    # the log, in the order one by one wrote it
    logs = []

    def j(v):
        return None if v is None else json.dumps(v, sort_keys=True)

    def inserted(tbl, rows):
        back = rows_by_key(db, tbl, [r[KEYS[tbl]] for r in rows])
        for r in rows:
            k = r[KEYS[tbl]]
            logs.append((CRAFT["name"], tbl, str(k), "inserted", None, j(back[k])))
    inserted("itempattern", item_rows[:len(ops["items"])])
    # a product and its recipe, recipe by recipe
    prod_back = rows_by_key(db, "itempattern", [r["patternid"] for r in item_rows[len(ops["items"]):]])
    rec_back = rows_by_key(db, "craftrecipes", [r["id"] for r in recipe_rows])
    made = set(prod_back)
    for r in recipe_rows:
        if r["product"] in made:
            logs.append((CRAFT["name"], "itempattern", str(r["product"]), "inserted", None, j(prod_back[r["product"]])))
            made.discard(r["product"])
        logs.append((CRAFT["name"], "craftrecipes", str(r["id"]), "inserted", None, j(rec_back[r["id"]])))
    for u in ops["npcs"]:
        logs.append((CRAFT["name"], "npcs", str(u["id"]), "updated", j(u["old"]), j(u["new"])))
    for r in ops["npcitem_del"]:
        logs.append((CRAFT["name"], "npcitem", str(r["itemid"]), "deleted", j(r), None))
    inserted("npcitem", add_rows)
    inserted("lootpool", pool_rows)
    inserted("loottables", table_rows)
    if ops["npcloot"]:
        logs.append((CRAFT["name"], "npcloot", json.dumps(["*", ops["npcloot"]["tablename"]]), "inserted", None,
                     j({"chance": ops["npcloot"]["chance"], "rolls": 1})))
    for name, seq in seq_of(db).items():
        if seqs.get(name) != seq:
            logs.append((CRAFT["name"], "sqlite_sequence", name, "updated", j({"seq": seqs.get(name)}), j({"seq": seq})))
    db.executemany("INSERT INTO tradeskill_seed (craft, tbl, key, action, old, new) VALUES (?, ?, ?, ?, ?, ?)", logs)
    counts = {"items": len(item_rows), "recipes": len(recipe_rows), "wares": len(add_rows),
              "stand-ins removed": len(ops["npcitem_del"]), "loot rows": len(pool_rows) + len(table_rows)}
    audit_line(db, "insert", f"tools/seed_tradeskills.py --craft {CRAFT['name']} --apply: {counts['items']} items, "
               f"{counts['recipes']} recipes, {counts['wares']} wares ({counts['stand-ins removed']} stand-ins out), "
               f"{counts['loot rows']} loot rows (each in tradeskill_seed)")
    return counts


def apply(db, p, ops=None):
    """Write a plan in one transaction: p (a single step: resolved here, inside it), or ops (--plan-in: a resolved
    plan, applied only when the database is still as it assumed)."""
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_DDL)
        db.execute(CRAFT_DDL)
        if ops is None:
            ops = resolve(db, p)
        else:
            if ops.get("version") != 1 or ops.get("craft") != CRAFT["name"]:
                raise SystemExit(f"the plan is not a --craft {CRAFT['name']} plan of this tool's")
            now = assumptions(db, ops)
            diff = [k for k in ops["assume"] if ops["assume"][k] != now.get(k)]
            if diff:
                raise SystemExit(f"the database is not as the plan found it ({', '.join(diff)}): nothing written; plan again")
        done = write_ops(db, ops)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return done


def audit_line(db, action, detail):
    """One gm_audit line for a whole run (tradeskill_seed holds each row it wrote)."""
    if not has_table(db, "gm_audit"):
        return
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, 0, 'dab', 'tool', '', ?, 'tradeskill_seed', ?, NULL, NULL, ?)",
               (at, action, json.dumps({"craft": CRAFT["name"]}), detail))


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
            if tbl == "sqlite_sequence":
                r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (key,)).fetchone()
                if r is not None and r[0] == new["seq"]:
                    if old["seq"] is None:
                        db.execute("DELETE FROM sqlite_sequence WHERE name = ?", (key,))
                    else:
                        db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", (old["seq"], key))
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
        first = db.execute("SELECT min(seq) FROM tradeskill_seed WHERE craft = ?", (craft,)).fetchone()[0]
        db.execute("DELETE FROM tradeskill_seed WHERE craft = ?", (craft,))
        if db.execute("SELECT count(*) FROM tradeskill_seed").fetchone()[0] == 0:
            db.execute("DROP TABLE tradeskill_seed")
        elif db.execute("SELECT count(*) FROM tradeskill_seed WHERE seq > ?", (first,)).fetchone()[0] == 0:
            # the craft's lines were the log's last: its counter back to before them
            db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = 'tradeskill_seed'", (first - 1,))
        audit_line(db, "delete", f"tools/seed_tradeskills.py --craft {craft} --undo: {back} changes taken back, {kept} left")
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
JEWEL_LOOT_CHANCE = 5                # ours: percent of every creature's corpses (the user, 2026-10-30: the gems' 3 to 5)
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
    loot = {"table": JEWEL_LOOT_TABLE, "chance": JEWEL_LOOT_CHANCE, "mods": [(GEM_ITEM.get(g, g), props[g][0]) for g in props],
            "note": "the jewelers' gems: any creature (eqoatrade: random)"}
    return {"skill": JEWEL, "cells": cells_n, "vendors": vendors, "comps": comps, "items": items, "ids": ids,
            "products": products, "stock": stock, "left": left, "problems": problems, "loot": loot}


# ---- Weaponcrafting


WEAPON = 0  # the skill: CLIENT's craft slot (Recipe Book: Weapons -> 0)
WEAPON_SPECIAL = 10
# Prima p. 446, "Metals" (Weaponcrafting): bonus durability and hit points by trade level; QuestMasters' Weaponcrafting
# page (id526) prints the same table. The metals and their order are Armor's (CLIENT's own list): Sandstorm's ores.
WEAPON_BONUS_DUR = [2, 5, 7, 10, 12, 15, 17, 20, 22, 25]
WEAPON_BONUS_HP = [50, 100, 200, 400, 800, 1200, 1800, 2000, 2400, 2600]
# p. 447, "Mundane Crafted Weapon Damage" (the damage itempattern.weapondamage holds: this server's merchant bronze
# claymore 162, steel greatsword 313); eqoatrade's 1H and 2H grids give the same to Adamantite (checked: read_weapon_grid)
WEAPON_DAMAGE = {1: [51, 82, 136, 190, 243, 295, 346, 400, 452, 505], 2: [59, 95, 159, 221, 284, 345, 407, 468, 531, 593]}
# p. 447, "Crafted Weapon Proc Stats": a stat gem's figure by trade level (the "w/ Stat" rows; eqoatrade's grids agree
# from Bronze to Adamantite, and give none below Bronze, as Prima's counts: modifiers from trade level 3)
WEAPON_STAT = {1: [1, 4, 7, 8, 10, 11, 11, 11, 12, 14], 2: [2, 6, 10, 14, 18, 20, 24, 26, 32, 35]}
# the "w/ DD", "w/ DoT" and "w/ Drain" rows from trade level 3: (figure, chance %); eqoatrade's grids print each DD and
# DoT cell's "Max" as twice Prima's figure (1H 20/40/40/50 against 10/20/20/25), so a proc rolls Prima's figure to twice it
WEAPON_DD = {1: [(10, 10), (20, 10), (20, 15), (25, 15), (25, 20), (30, 20), (30, 25), (40, 25)],
             2: [(10, 10), (20, 10), (25, 15), (33, 15), (33, 20), (40, 20), (45, 25), (55, 25)]}
WEAPON_DOT = {1: [(10, 10), (12, 10), (13, 15), (15, 15), (15, 20), (18, 20), (18, 25), (21, 25)],
              2: [(10, 10), (12, 10), (15, 15), (17, 15), (17, 20), (21, 20), (23, 25), (25, 25)]}
WEAPON_DRAIN = {1: {9: (10, 10), 10: (10, 15)}, 2: {9: (10, 20), 10: (10, 25)}}
WEAPON_MOD_FROM = 3  # the least trade level a modifier works at (Prima's counts; eqoatrade; QuestMasters id341)
# p. 446, "Weapon Molds" and "Metal Product Sprite by Mold and Trade Level": a mold's hands, its attack type (this
# server's records': 1 slash, 3 blunt, 5 pierce one-handed; 2 slash, 4 blunt, 6 pierce two-handed), and its product by
# trade level. Prima's "—" (no product printed) is the row's first product (ours), so every mold makes ten as Prima's
# count says (130 mundane weapons); its "Sacrificial" (cut to a word in the print) is a sacrificial dagger (ours). Who
# wields it: QuestMasters' Weaponcrafting page (id526) by mold.
WAR, RAN, PAL, SK, MNK, BRD, RGE, DRD, SHA, CL = range(10)
_MELEE = (WAR, SK, PAL, RAN, BRD, RGE)
_KNIGHTS = (WAR, SK, PAL)
_BLUNT = (WAR, SK, PAL, RAN, BRD, RGE, DRD, SHA, CL)
WEAPON_MOLDS = {
    "Knife Blade Mold": (1, 5, _MELEE, ["knife", "dirk", "dirk", "dagger", "dagger", "stiletto", "stiletto",
                                        "sacrificial dagger", "sacrificial dagger", "tanto"]),
    "Axe Head Mold": (1, 1, _MELEE, ["hand axe", "hand axe", "hatchet", "hatchet", "bearded axe", "bearded axe",
                                     "battle axe", "battle axe", "war axe", "war axe"]),
    "Two-Hand Axe Head Mold": (2, 2, _KNIGHTS, [None, None, "greataxe", "greataxe", "greataxe", "bearded greataxe",
                                                "bearded greataxe", "bearded greataxe", "great waraxe", "great waraxe"]),
    "Short Sword Blade Mold": (1, 1, _MELEE, ["battle knife", "battle knife", "short blade", "short blade", "small sword",
                                              "gladius", "gladius", "short sword", "short sword", "wakizashi"]),
    "Long Sword Blade Mold": (1, 1, _KNIGHTS, [None, None, "long blade", "long blade", "broad sword", "broad sword",
                                               "long sword", "long sword", "warsword", "katana"]),
    "Foil Mold": (1, 5, _MELEE, [None, None, "foil", "foil", "sabre", "sabre", "rapier", "rapier", "cutlass", "cutlass"]),
    "Two Hand Sword Mold": (2, 2, _KNIGHTS, ["bastard sword", "bastard sword", "claymore", "claymore", "greatsword",
                                             "greatsword", "flamberge", "flamberge", "zweihander", "no-dachi"]),
    "Polearm Head Mold": (2, 6, _KNIGHTS, ["longspear", "longspear", "longspear", "partizan", "partizan", "partizan",
                                           "spetum", "spetum", "spetum", "trident"]),
    "Poleaxe Head Mold": (2, 2, _KNIGHTS, ["glaive", "glaive", "guisarme", "guisarme", "halberd", "halberd",
                                           "bearded poleaxe", "bearded poleaxe", "pole axe", "naginata"]),
    "Hammer Mold": (1, 3, _BLUNT, ["smith hammer", "smith hammer", "smith hammer", "hammer", "hammer", "hammer",
                                   "military hammer", "military hammer", "mallet", "mallet"]),
    "Two Hand Hammer Mold": (2, 4, (WAR, SK, PAL, DRD, SHA, CL), [None, None, "warhammer", "warhammer", "warhammer",
                                                                  "battlehammer", "battlehammer", "battlehammer",
                                                                  "bladed warhammer", "bladed warhammer"]),
    "Mace Mold": (1, 3, _BLUNT, [None, "mace", "mace", "mace", "spiked mace", "spiked mace", "morning star",
                                 "morning star", "bladed mace", "bladed mace"]),
    # Prima: "By Product Sprite": the fists and knuckles blunt, the bladed knuckles and the katar pierce (ours)
    "Martial Mold": (1, 3, (WAR, SK, PAL, MNK), ["fist", "fist", "knuckles", "knuckles", "spiked knuckles",
                                                 "spiked knuckles", "bladed knuckles", "bladed knuckles", "katar", "katar"]),
}
MARTIAL_PIERCE = {"bladed knuckles", "katar"}
# the words this server's own weapon records are named by where a product's are not theirs, for its look (the
# product's word first): a record of the same kind lends its model and icon, never a model worked out by hand
LOOK_WORDS = {
    "sacrificial dagger": ["sacrificial", "dagger"], "bastard sword": ["bastardsword", "claymore"],
    "long blade": ["longsword", "broadsword"], "broad sword": ["broadsword"], "long sword": ["longsword"],
    "small sword": ["smallsword", "shortblade"], "short blade": ["shortblade"], "smith hammer": ["smithing hammer", "hammer"],
    "pole axe": ["poleaxe", "halberd"], "bearded poleaxe": ["poleaxe", "halberd"], "great waraxe": ["greataxe"],
    "bearded greataxe": ["bearded greataxe", "greataxe"], "spiked mace": ["morning star", "mace"],
    "bladed warhammer": ["warhammer"], "military hammer": ["military hammer", "hammer"], "battle knife": ["battle knife", "knife"],
    "war axe": ["war axe", "waraxe", "battle axe"], "battle axe": ["battle axe", "bearded axe"],
    "hand axe": ["hand axe", "handaxe", "hatchet"], "warsword": ["warsword", "longsword"],
}
# The modifiers (QuestMasters' "Gems By Each Craft" and Weaponcrafting pages, id459 and id526, eqoatrade's item list and
# Prima p. 446 agree on these seventeen; Prima's fourteen starred ones (the resist gems, Plague Dust, Viper Lead, Wisp
# Energy, Liquid Inferno, Nitrogen Alloy, Electrified Copper and the three banes) are in neither of the others and are
# left out): rarity (Prima's; QuestMasters' gem list id458 says rare for the last two), what each does, and the word a
# product's name takes (eqoatrade's "Makes Fiery xxx" ...; the gems' and the two drains' are ours: a gem's own name,
# as Armor's "sinhalite steel bracers", "vampiric" and "siphoning").
WEAPON_MODS = {
    "Bloodstone": (0, ("stat", "str"), "bloodstone"), "Citrine": (0, ("stat", "sta"), "citrine"),
    "Tsavorite": (0, ("stat", "agi"), "tsavorite"), "Ametrine": (0, ("stat", "dex"), "ametrine"),
    "Heliodor": (0, ("stat", "wis"), "heliodor"), "Moonstone": (0, ("stat", "intelligence"), "moonstone"),
    "Erollite": (0, ("stat", "cha"), "erollite"),
    "Magnesium Alloy": (1, ("dd", "fire"), "fiery"), "Dervish Ice Crystals": (1, ("dd", "cold"), "freezing"),
    "Ectoplasmic Crystals": (1, ("dd", "arcane"), "cursed"),
    # Prima "Arcane Power Damage", eqoatrade and QuestMasters "Non Damage Proc": a creature here has no power to take
    # (charm.go), so it lands as arcane damage (ours)
    "Intellect Spores": (1, ("dd", "arcane"), "withering"),
    "Malign Alloy": (2, ("dot", "disease"), "malign"), "Rogue Metal": (2, ("dot", "poison"), "venomous"),
    "Volatile Mana Crystals": (2, ("dot", "arcane"), "volatile"),
    "Ethereal Mists": (2, ("dot", "arcane"), "ethereal"),  # "Arcane Power DoT": as Intellect Spores
    "Negative Energy Stone": (3, ("drain", "mana tap"), "siphoning"),  # "Arcane Mana Tap Proc" (QuestMasters)
    "Viscous Vampire Blood": (3, ("drain", "lifetap"), "vampiric"),   # "Arcane Blood Tap Proc"
}
WEAPON_MOD_ITEM = {}  # a modifier's item where this server names it otherwise (none: kept for the rule)
STAT_WORD = {"str": "strength", "sta": "stamina", "agi": "agility", "dex": "dexterity", "wis": "wisdom",
             "intelligence": "insight", "cha": "charisma"}
WEAPON_STARTER_HAMMER = "Novice Weaponsmith Hammer"
# eqoatrade's tutorial: "An apprentice weaponsmith hammer, a dull knife recipe, 1 dented iron block, 1 rusted bronze
# block, 1 rusty iron block, 1 mace mold, and 1 knife blade mold"; the dull knife by the recipe ("under Special"),
# "rusty bronze block plus mace mold" a crude rusty mace by experiment. The dull knife's ingredients are not given: the
# rusty iron block and the knife blade mold (ours). The dented iron block's use was unknown to eqoatrade itself ("Does
# anyone know what the dented iron block is for?"): not made.
DULL_SCROLL, DULL_KNIFE, RUSTY_MACE = "Dull Knife Recipe", "dull knife", "crude rusty mace"
RUSTY_IRON, RUSTED_BRONZE = "Rusty Iron Block", "Rusted Bronze Block"
# Prima's city tables where eqoatrade's list names no NPC of this server's (or none): Neriak (eqoatrade: "Thesla
# V`Ryn"; molds "Hmm, spent about 10 minutes looking"; "Merchant Veikara or Mechant Zairella"), Klick'Anon (eqoatrade's
# "Lelil VII"), Grobb (eqoatrade's "Duliigh"), Rivervale (eqoatrade's "Bubba Whistetip"), Surefall Glade (Prima's mold
# seller, where eqoatrade's Merchant Ilander is no NPC: the Live Server's)
WEAPON_SELLERS = {"Neriak": {"hammers": "Weaponsmith Thesla", "molds": "Merchant Veikara", "metal": "Merchant Zairella"},
                  "Klick Anon": {"metal": "Merchant Lelii VII"}, "Grobb": {"molds": "Merchant Dulilgh"},
                  "Rivervale": {"hammers": "Bubba Whistletip"}}
WEAPON_FALLBACK = {"Surefall Glade": {"molds": "Merchant Kaelara"}}
WEAPON_POOL = "modifiers"  # the loot category the armorsmiths' modifiers drop from (lootpool, lootrate's own rate)


def read_weapon_grid(folder, problems):
    """eqoatrade's 1H and 2H grids (Recipes/Weapon/Original/{OneHand,TwoHand}.htm) against Prima's tables: each cell
    it fills ("136 / 7": damage / a stat gem's figure) must be Prima's; none of a modifier below trade level 3."""
    checked = 0
    for hands, page in ((1, "OneHand.htm"), (2, "TwoHand.htm")):
        path = os.path.join(folder, "eqoatrade", "Recipes", "Weapon", "Original", page)
        if not os.path.exists(path):
            problems.append(f"{path}: missing (tools/fetch_tradeskill_sources.py --craft weapon)")
            continue
        with open(path, "rb") as f:
            t = f.read().decode("latin-1")
        tls = None
        for tr in re.findall(r"<tr.*?</tr>", t, flags=re.S | re.I):
            row = cells_of(tr)
            if len(row) >= 7 and row[0].lower() == "gem":
                tls = {"CrIr": 1, "Ir": 2, "Brz": 3, "Brs": 4, "St": 5, "CoSt": 6, "Ad": 7, "Mi": 8, "Brl": 9, "Se": 10}
                tls = [tls[c] for c in row[2:7]]
                continue
            if not tls or len(row) < 7:
                continue
            mod, what = row[0], row[1]
            for tl, c in zip(tls, row[2:7]):
                m = re.fullmatch(r"(\d+)(?:\s*/\s*(\d+|\?))?", c)
                if not m:
                    continue
                checked += 1
                if int(m.group(1)) != WEAPON_DAMAGE[hands][tl - 1]:
                    problems.append(f"eqoatrade {page} {mod} TL{tl}: damage {m.group(1)}, Prima {WEAPON_DAMAGE[hands][tl - 1]}")
                if mod.startswith("No Gem"):
                    continue
                if tl < WEAPON_MOD_FROM:
                    problems.append(f"eqoatrade {page} {mod} TL{tl}: a modifier below trade level {WEAPON_MOD_FROM}")
                stat = re.search(r"Strength|Stamina|Agility|Dexterity|Wisdom|Intelligence|Charisma", what)
                if stat and m.group(2) not in (None, "?") and int(m.group(2)) != WEAPON_STAT[hands][tl - 1]:
                    problems.append(f"eqoatrade {page} {mod} TL{tl}: stat {m.group(2)}, Prima {WEAPON_STAT[hands][tl - 1]}")
    return checked


def weapon_look(db, sprite, hands, cache):
    """The look of a product: the commonest model (and that model's commonest icon, colour and wear slot) of this
    server's weapon records of the hands named by the product's word, else by its LOOK_WORDS."""
    key = (sprite, hands)
    if key in cache:
        return cache[key]
    slots = (12, 14) if hands == 1 else (15,)
    for w in [sprite] + LOOK_WORDS.get(sprite, []):
        where = (f"weapondamage > 0 AND equipslot IN ({', '.join(map(str, slots))}) AND model != 0 AND "
                 "(lower(itemname) LIKE ? OR lower(itemname) = ?)")
        args = (f"% {w}", w)
        model = commonest(db, f"SELECT model AS x FROM itempattern WHERE {where}", args)
        if model is None:
            continue
        icon = commonest(db, f"SELECT itemicon AS x FROM itempattern WHERE {where} AND model = ?", (*args, model))
        slot = db.execute(f"SELECT equipslot, count(*) FROM itempattern WHERE {where} AND model = ? GROUP BY equipslot "
                          "ORDER BY count(*) DESC, equipslot LIMIT 1", (*args, model)).fetchone()[0]
        color = db.execute(f"SELECT color, count(*) FROM itempattern WHERE {where} AND model = ? GROUP BY color "
                           "ORDER BY count(*) DESC, color LIMIT 1", (*args, model)).fetchone()[0]
        cache[key] = (model, icon or 0, slot, color, w)
        return cache[key]
    cache[key] = None
    return None


def class_mask(classes):
    m = 0
    for c in classes:
        m |= 1 << c
    return m


def weapon_proc_words(kind, element, hands, tl):
    """A product's proc in the words entity/weaponproc.go reads (WeaponProcFromText), or "" for none."""
    if kind == "dd":
        n, pct = WEAPON_DD[hands][tl - WEAPON_MOD_FROM]
        return f"Procs {n}-{2 * n} {element} damage, a {pct}% chance each hit."
    if kind == "dot":
        n, pct = WEAPON_DOT[hands][tl - WEAPON_MOD_FROM]
        return f"Procs a {n}-{2 * n} point {element} DoT, a {pct}% chance each hit."
    if kind == "drain":
        n, pct = WEAPON_DRAIN[hands][tl]
        return f"Procs a {n} point {element}, a {pct}% chance each hit."
    return ""


def plan_weapon(db, folder, harvests):
    """Weaponcrafting's items, recipes, stock and drops (the module's doc)."""
    problems = []
    cells = read_weapon_grid(folder, problems)
    vendors = read_vendors(folder, "weapon")
    hammers = [f"{h} Weaponsmith Hammer" for h in KITS]  # Sandstorm's: "Expert" where the armorsmiths' have "Craftsman"
    ores = [f"{m} Ore" for m in METALS]
    want = set(hammers) | set(WEAPON_MOLDS) | set(ores)
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

    # the modifiers: this server's own where it has them (Bloodstone, Ametrine ...), else made as its own are
    gem_icon = commonest(db, "SELECT itemicon AS x FROM itempattern WHERE craft > 0 AND equipslot = -1 AND "
                             "itemdesc LIKE '%crafters of weapons%'")
    alloy_icon = commonest(db, "SELECT itemicon AS x FROM itempattern WHERE craft > 0 AND lower(itemname) LIKE '% alloy'")
    rarities = ["common", "uncommon", "rare", "ultra rare"]
    for name, (rarity, (kind, what), word) in WEAPON_MODS.items():
        item = WEAPON_MOD_ITEM.get(name, name)
        have = item_by_name(db, item)
        if have is not None:
            ids[item] = have
            continue
        if kind == "stat":
            desc = (f"{name}s are often sought after by crafters of weapons for their ability to lend {STAT_WORD[what]} "
                    "to the weapon's wielder.")
            icon = gem_icon
        else:
            does = {"dd": f"{what} to its strikes", "dot": f"a lingering {what} to its strikes",
                    "drain": "a draining touch to its strikes"}[kind]
            desc = f"A{'n' if rarities[rarity][0] == 'u' else ''} {rarities[rarity]} {name.lower()} a weaponsmith works into a blade, lending {does}."
            icon = alloy_icon if re.search(r"Alloy|Metal|Crystals|Stone|Blood|Mists|Spores", name) else gem_icon
        items.append((None, base_item(item, icon or 0, MOD_PRICE[rarity], 0, -1, maxstack=50, craft=1, desc=desc)))
        ids[item] = None

    scroll_icon = commonest(db, "SELECT itemicon AS x FROM itempattern WHERE lower(itemname) LIKE '%recipe%'")
    ore_icon = (comps.get("Crude Iron Ore") or {}).get("itemicon") or commonest(
        db, "SELECT itemicon AS x FROM itempattern WHERE lower(itemname) LIKE '% ore' AND craft > 0")
    for name, icon, price, desc, craft, stack in [
        (DULL_SCROLL, scroll_icon, 10, "A recipe for a dull knife. Use it to scribe the recipe.", 0, 1),
        (RUSTY_IRON, ore_icon, 2, "A block of rusty iron, for a weaponsmith's first work.", 1, 20),
        (RUSTED_BRONZE, ore_icon, 2, "A block of rusted bronze, for a weaponsmith's first work.", 1, 20),
    ]:
        have = item_by_name(db, name)
        if have is None:
            items.append((None, base_item(name, icon or 0, price, 0, -1, maxstack=stack, craft=craft, desc=desc)))
        ids[name] = have

    looks, products = {}, []
    # a mold's wear slot, one for all its products: the one most of its products' looks are worn in (a one-hander's
    # primary hand, 12, or either hand, 14; a two-hander's 15)
    mold_slot = {}
    for mold, (hands, _, _, sprites) in WEAPON_MOLDS.items():
        got = collections.Counter(lk[2] for lk in (weapon_look(db, sp, hands, looks) for sp in sprites if sp) if lk)
        mold_slot[mold] = sorted(got.items(), key=lambda x: (-x[1], x[0]))[0][0] if got else (12 if hands == 1 else 15)

    def product(name, sprite, hands, attack, classes, level, tl, comp, damage, desc, rarity, ings, extra=None):
        look = weapon_look(db, sprite, hands, looks)
        if look is None:
            problems.append(f"no weapon record here looks like a {sprite} ({hands}-handed)")
            return
        model, icon, _, color, _ = look
        slot = mold_slot[ings[1][0]] if ings[1][0] in mold_slot else look[2]
        metal_tl = max(tl, 1)
        ore_price = (comps.get(f"{METALS[metal_tl - 1]} Ore") or {}).get("itemcost") or 10 * metal_tl * metal_tl
        row = base_item(name, icon, 3 * ore_price + 10 + (MOD_WORTH[rarity] if rarity is not None else 0), level, slot,
                        maxstack=1, craft=0, classes=class_mask(classes), desc=desc, model=model, color=color,
                        maxhp=200 * level + WEAPON_BONUS_HP[metal_tl - 1], duration=50 + WEAPON_BONUS_DUR[metal_tl - 1])
        row["attacktype"], row["weapondamage"] = attack, damage
        pr = {"name": name, "row": row, "have": None, "comp": comp, "level": tl, "rarity": rarity or 0, "ings": ings,
              "sort": (tl, rarity or 0, name), "kind": f"{hands}H", "magical": rarity is not None}
        r = db.execute("SELECT patternid FROM itempattern WHERE lower(itemname) = ? AND levelreq = ? AND equipslot = ? "
                       "AND weapondamage = ? ORDER BY patternid LIMIT 1", (name.lower(), level, slot, damage)).fetchone()
        pr["have"] = r[0] if r else None
        pr.update(extra or {})
        products.append(pr)

    for mold, (hands, attack, classes, sprites) in WEAPON_MOLDS.items():
        first = next(s for s in sprites if s)
        for tl in range(1, 11):
            sprite = sprites[tl - 1] or first
            at = 5 if mold == "Martial Mold" and sprite in MARTIAL_PIERCE else attack
            metal = METALS[tl - 1]
            level = 5 * tl - 2
            dmg = WEAPON_DAMAGE[hands][tl - 1]
            base = f"A {'two-handed ' if hands == 2 else ''}weapon forged of {metal.lower()}."
            ore = (f"{metal} Ore", 1, False)
            product(f"{metal.lower()} {sprite}", sprite, hands, at, classes, level, tl, tl - 1, dmg, base, None,
                    [ore, (mold, 1, False)])
            if tl < WEAPON_MOD_FROM:
                continue
            for mod, (rarity, (kind, what), word) in WEAPON_MODS.items():
                if kind == "drain" and tl not in WEAPON_DRAIN[hands]:
                    continue
                row_extra = {}
                desc = base
                if kind == "stat":
                    row_extra[what] = WEAPON_STAT[hands][tl - 1]
                else:
                    desc = f"{base} {weapon_proc_words(kind, what, hands, tl)}"
                name = f"{word} {metal.lower()} {sprite}"
                before = len(products)
                product(name, sprite, hands, at, classes, level, tl, tl - 1, dmg, desc, rarity,
                        [ore, (mold, 1, False), (WEAPON_MOD_ITEM.get(mod, mod), 1, True)])
                if len(products) > before:
                    products[-1]["row"].update(row_extra)
                    products[-1]["proc"] = kind
    # the starter, under Special (its figures ours: a crude iron one's look, under its damage)
    for name, sprite, attack, ings, extra in [
        (DULL_KNIFE, "knife", 5, [(RUSTY_IRON, 1, False), ("Knife Blade Mold", 1, False)],
         {"scroll": DULL_SCROLL, "discover": 0, "description": "The first weapon a weaponsmith's apprentice makes."}),
        (RUSTY_MACE, "mace", 3, [(RUSTED_BRONZE, 1, False), ("Mace Mold", 1, False)],
         {"discover": 1, "description": "An apprentice's mace of rusted bronze."}),
    ]:
        product(name, sprite, 1, attack, _BLUNT if sprite == "mace" else _MELEE, 1, 0, WEAPON_SPECIAL, 40,
                f"A weaponsmith's first {name}.", None, ings,
                dict(extra, level=1, sort=(99, name), kind="starter", magical=False, difficulty=1, xp=10, rarity=0))
    names = collections.Counter(p["name"] for p in products)
    problems += [f"{k} products named {n!r}" for n, k in names.items() if k > 1]

    stock, left = [], []
    molds = list(WEAPON_MOLDS) + [RUSTY_IRON, RUSTED_BRONZE]
    for city, v in sorted(vendors.items()):
        v = dict(v, **WEAPON_SELLERS.get(city, {}))
        for role, wares, what in (("hammers", hammers, "hammers"), ("molds", molds, "molds"), ("metal", ores, "ores")):
            name = v.get(role)
            r = npc_named(db, name) if name else []
            if not r and WEAPON_FALLBACK.get(city, {}).get(role):
                alt = WEAPON_FALLBACK[city][role]
                left.append(f"{city}: {name} ({what}) is no NPC here: Prima's {alt}")
                name, r = alt, npc_named(db, alt)
            if not name:
                left.append(f"{city}: no {what} seller on eqoatrade's list or Prima's")
                continue
            if not r:
                left.append(f"{city}: {name} ({what}) is no NPC here")
                continue
            if len(r) > 1:
                left.append(f"{city}: {len(r)} NPCs named {name}: the first stocked")
            stock.append((city, role, name, r[0], wares))
    loot = {"pool": WEAPON_POOL, "mods": [(WEAPON_MOD_ITEM.get(m, m), v[0]) for m, v in WEAPON_MODS.items()]}
    return {"skill": WEAPON, "cells": cells, "vendors": vendors, "comps": comps, "items": items, "ids": ids,
            "products": products, "stock": stock, "left": left, "problems": problems, "loot": loot}


PLANS = {"armor": plan, "jewel": plan_jewel, "weapon": plan_weapon}


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
    ap.add_argument("--plan-out", metavar="PLAN.json", help="decide everything (against this database, a copy of the "
                    "target's) and write the plan, nothing to the database")
    ap.add_argument("--plan-in", metavar="PLAN.json", help="with --apply: write a --plan-out plan (no sources read), "
                    "refused unless the database is as the plan found it")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    if a.plan_in and (not a.apply or a.plan_out):
        sys.exit("--plan-in goes with --apply (and not --plan-out)")
    if a.plan_out and a.apply:
        sys.exit("--plan-out writes a plan, not the database: apply it with --plan-in PLAN.json --apply")
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
    if a.plan_in:
        if logged(db, a.craft):
            sys.exit(f"tradeskill_seed holds {a.craft}: applied already (--undo --craft {a.craft} first)")
        with open(a.plan_in) as f:
            ops = json.load(f)
        if not a.no_backup:
            print(f"copy: {backup(a.db)}")
        done = apply(db, None, ops)
        print("wrote " + ", ".join(f"{v} {k}" for k, v in done.items()))
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
    if p["problems"] and (a.apply or a.plan_out):
        sys.exit("problems: nothing written")
    if a.plan_out:
        if logged(db, a.craft):
            sys.exit(f"tradeskill_seed holds {a.craft}: applied already (--undo --craft {a.craft} first)")
        ops = resolve(db, p)
        with open(a.plan_out, "w") as f:
            json.dump(ops, f)
        print(f"plan: {a.plan_out} ({len(ops['items']) + len(ops['products'])} items, {len(ops['recipes'])} recipes, "
              f"{len(ops['npcitem_add'])} wares, {len(ops['npcitem_del'])} stand-ins out); nothing written to {a.db}")
        return
    if not a.apply:
        print("report only: --apply to write")
        return
    if not a.no_backup:
        print(f"copy: {backup(a.db)}")
    done = apply(db, p)
    print("wrote " + ", ".join(f"{v} {k}" for k, v in done.items()))


if __name__ == "__main__":
    main()
