#!/usr/bin/env python3
"""The live game's charm rules for the creatures it names, from eqoa.fandom.com (the user, 2026-11-02: charm-pets.md,
the wiki's "Charmable Mobs" page, with "https://eqoa.fandom.com/wiki/Category:Mobs"; the user picked "Pages' names +
bosses" and "Charm templates per kind"). Writes table npccharm (entity/charm.go, database/npccharm.go) and the class
templates its rows name (classtemplates, entity/pettemplate.go).

Its sources, all of them the wiki's (claude-notes/charm-20261102.md has the text):
  - "Charmable Mobs" (2010-03-10; Kaleif's list and KriztianBBB's): which creatures were charmed and what each made as a
    pet, which were "DIFFICULT TO UNCHARMABLE";
  - "Charming" (2010-03-08): the rhinos of 48-54 and the snapping tortoises uncharmable; a 60 druid's pets;
  - "Category:Raid mobs": Bertoxxulous, Cazic Thule, Lord Nagafen, Noroxus, the Quintessence of Veeshan, Toxxulia
    (Kaleif's "Almost all named raid mobs (i.e. dragons, Parathior, etc.) are lvl 55-70+ and uncharmable").

What it writes:
  - npccharm, charmable 0 (refused at the cast's start, "Your target cannot be charmed."): UNCHARMABLE's names, the
    raid mobs (most above any charm's cap here, named so a GM's level edit keeps them so), the dragons and Parathior
    (52 here: Allure's 53 took it), the Were-Hunters, the raging rhinos, the snapping tortoises;
  - npccharm, resist_pct (its chance to resist a charm that lands; the server's default 10): Ambassador Vu'Malik 100
    ("charm will not stick and he will nuke as if taunted": resisted, he comes at the caster), Hand of Thule 85 ("Very
    low success rate"), Ebony Recluse 60 ("Low success rate"), kappa drudges 40 ("Tough Charms"). The figures are
    ours; the words are the page's;
  - classtemplates, one "charm <kind>" row a kind (class -1: the creature keeps its own class, swing and hit points,
    "current existing stats"), its abilities read off the page's words (TEMPLATES below), and npccharm, template, for
    each creature name of the kind found in npcs. Each ability casts the class's spell of its kind at or below the
    pet's level (spells_of), as the summoned pets' templates do; a charmed pet's casts cost it their spells' power (28
    a level, back a 50th a tick and with what its master gives it), but the Nasehir wizards' ("chain cast, never run
    OOM"). The damage percents, clocks and crits' figures are ours (a crit's from the page's "700+", "3k+").

Never a row npccharm or classtemplates already has (a GM's, or this tool's before: said, left as it is). Every row
written (source charm-seed) is logged in npc_charm_seed_log (the table, the row's key, the row after, why); --undo
deletes each still as written (one a GM changed since is kept, and said), a classtemplates row only once no npccharm
row names it, and drops the log; a second --apply adds only what is still to do. Both copy the database first
(<db>.bak-<when>-pre-charm) unless --no-backup. npccharm is made when missing (as a server of 2026-11-02 makes it). Report
only by default.

The server reads npccharm at its start and again after a GM's edit of it in the panel, classtemplates at its start and
after a template edit; the templates' "free" field and the area and special abilities a charmed pet casts need a server
of 2026-11-02 or later (an older one refuses a "free" template as a bad row): apply with that server stopped, before it
starts (a swap's before.sh).

    tools/seed_npc_charm.py [--db eqoa.db] [--apply | --undo] [--no-backup] [--list]
"""
import argparse
import datetime
import json
import os
import re
import shutil
import sqlite3
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
SOURCE = "charm-seed"
LOG = "npc_charm_seed_log"
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY,
    tbl     TEXT NOT NULL,   -- npccharm | classtemplates
    place   TEXT NOT NULL,   -- the row's name
    after   TEXT NOT NULL,   -- the whole row as written, JSON
    why     TEXT NOT NULL,
    applied TEXT NOT NULL
)"""
# database/npccharm.go's schema
NPCCHARM_SCHEMA = """CREATE TABLE IF NOT EXISTS npccharm (
    name       TEXT PRIMARY KEY,
    charmable  INTEGER NOT NULL DEFAULT 1,
    resist_pct INTEGER NOT NULL DEFAULT 0,
    template   TEXT NOT NULL DEFAULT '',
    note       TEXT NOT NULL DEFAULT '',
    source     TEXT NOT NULL DEFAULT 'gm'
)"""
WHY = "the wiki's Charmable Mobs and Charming pages (the user, 2026-11-02; tools/seed_npc_charm.py)"

# classes (CLAUDE.md's Protocol Constants)
WAR, RAN, PAL, SK, MNK, BRD, RGE, DRD, SHA, CLR, MAG, NEC, ENC, WIZ, ALC = range(15)

# Uncharmable: (name pattern, the words that make it so). A pattern is a regular expression over the name as
# npcclass keys it, whole.
UNCHARMABLE = [
    (r"werehunter|were-hunter", "Charmable Mobs: \"Were-Hunter NPCs ~ (Zone: Various large cities) Uncharmable.\""),
    (r"raging rhinoceros|raging rhino", "Charmable Mobs: \"Raging Rhino ~ (Zone: The Hunt) Uncharmable.\"; Charming: \"the rhinos "
     "lvl 48-54 are uncharmable\""),
    (r"snapping tortoise", "Charming: \"The snapping tortises are uncharmable to my knowledge.\""),
    (r"lady vox|enraged lady vox", "Charmable Mobs: \"Almost all named raid mobs (i.e. dragons, Parathior, etc.) are lvl 55-70+ and "
     "uncharmable\"; Lady Vox is a raid mob (the wiki's page)"),
    (r"parathior", "Charmable Mobs: \"Almost all named raid mobs (i.e. dragons, Parathior, etc.) are lvl 55-70+ and uncharmable\""),
    (r"bertoxxulous the plaguebringer|bertoxxulous", "the wiki's Category:Raid mobs (Bertoxxulous)"),
    (r"cazic thule|cazic-thule|cazicthule", "the wiki's Category:Raid mobs (Cazic Thule)"),
    (r"lord nagafen", "the wiki's Category:Raid mobs (Lord Nagafen)"),
    (r"noroxus", "the wiki's Category:Raid mobs (Noroxus)"),
    (r"quintessence of veeshan", "the wiki's Category:Raid mobs (Quintessence of Veeshan)"),
    (r"toxxulia", "the wiki's Category:Raid mobs (Toxxulia)"),
]

# A resist of their own: (name pattern, percent, the words).
RESISTS = [
    (r"ambassador vu.malik", 100, "Charmable Mobs: \"Uncharmable - no pop-up(!!), but charm will not stick and he will nuke as if "
     "taunted.\""),
    (r"hand of thule", 85, "Charmable Mobs: \"Very low success rate, not worth charming for useful purposes.\""),
    (r"ebony recluse", 60, "Charmable Mobs: \"Very high AC/HP, melee dmg, also poison attack. It's a spider. Low success rate.\""),
    (r"kappa drudge", 40, "Charmable Mobs: \"Kappa Drudge = Tough Charms\""),
]


def nuke(every, first, spells_of, pct, element=None, max_recast=30, max_cast=5, free=False):
    a = {"kind": "nuke", "every": every, "first": first, "spells_of": spells_of, "damage_pct": pct,
         "max_recast": max_recast, "max_cast": max_cast}
    if element:
        a["element"] = element
    if free:
        a["free"] = True
    return a


def aoe(every, first, spells_of, pct, max_recast=30, max_cast=5, free=False):
    a = {"kind": "aoe", "every": every, "first": first, "spells_of": spells_of, "damage_pct": pct,
         "max_recast": max_recast, "max_cast": max_cast}
    if free:
        a["free"] = True
    return a


def chained_nuke(spells_of, pct, free=False):
    a = {"kind": "nuke", "chain": True, "spells_of": spells_of, "damage_pct": pct, "max_recast": 10, "max_cast": 5}
    if free:
        a["free"] = True
    return a


# The charm templates: name -> (the kinds' name patterns, the template's fields, the page's words). The damage percents
# are a pet's share of a player's spell (the summoned pets': 25-35%); the crits' a level from the page's figures.
NUKE_PCT = 50
TEMPLATES = {
    "charm tree": (
        [r"hollow", r"hollow tyrant", r"withered", r"treant", r"weary treant", r"thornless treant", r"forest keeper"],
        {"abilities": [
            {"kind": "buff", "every": 30, "first": 1, "spells_of": DRD, "stat": "ds", "free": True},
            {"kind": "buff", "every": 30, "first": 3, "spells_of": DRD, "stat": "ac", "free": True},
            nuke(12, 4, DRD, NUKE_PCT, "fire"),
            nuke(12, 10, DRD, NUKE_PCT, "lightning"),
            {"kind": "heal", "every": 15, "first": 1, "below": 40, "spells_of": CLR},
        ]},
        "Hollow: \"Immune to Arcane, Damage shield, Lightning/Fire DD, Self-Heals, Self AC buff, SoW\"; \"All the Trees, be it "
        "Withered, Hollow, Treant, etc have the ability to tank very well ... a 'life tap' ... to heal themselves when they're "
        "low on health ... try giving it some power\" (a druid's shield and skin, its own and free; a druid's fire and "
        "lightning nukes; a heal below 40%, a cleric's: the druid's book has no heal of one cast)"),
    "charm wisp": (
        [r"shadow wisp", r"shadowisp", r"shadowwisp", r"dark wisp", r"darkwisp", r"malign wisp", r"swamp wisp", r"crag wisp",
         r"will o'wisp", r"willowisp", r"wisp", r"corroded wisp", r"wandering wisp"],
        {"abilities": [aoe(15, 2, WIZ, 35, max_recast=10), chained_nuke(WIZ, 35)]},
        "\"at lvl 45 I can charm the Shadowisps ... IMO the Rhino/wisp is the best pet\"; the wisps fight as wizards here "
        "(the roamers' seed): a wizard's area spell and a single target's after it"),
    "charm nasehir wizard": (
        [r"nasehir wizard"],
        {"abilities": [aoe(15, 2, WIZ, 35, max_recast=10, free=True), chained_nuke(WIZ, 35, free=True)]},
        "\"The BEST pet IMO in this area is the Nasehir Wizard ... These wizards are like having a 5th group member, they "
        "chain cast, never run OOM\" (free: no power spent)"),
    "charm griffon": (
        [r"plague griffon", r"griffenne"],
        {"special": "Critical Hit", "special_damage": 14, "special_every": 30, "special_first": 8, "special_jitter": 5,
         "abilities": [nuke(15, 5, WIZ, NUKE_PCT)]},
        "Plague Griffon: \"Immune to Disease. Quad hit, 700+ crit, 1400+ nuke.\"; Buccs: \"the best DD mobs in the area are the "
        "Griffienne's\" (a crit of 14 a level: 700 at 50)"),
    "charm rhino": (
        [r"rampaging rhinoceros", r"charging rhinoceros"],
        {"special": "Critical Hit", "special_damage": 60, "special_every": 30, "special_first": 10, "special_jitter": 5,
         "abilities": []},
        "Rampaging/Charging Rhino: \"3k+ critical dmg attack, ok melee dmg, high hp.\" (60 a level: 3,000 at 50)"),
    "charm brute": (
        [r"superior brute"],
        {"special": "Critical Hit", "special_damage": 20, "special_every": 25, "special_first": 8, "special_jitter": 5,
         "abilities": []},
        "Superior Brute: \"Critical hits, good melee dmg, Rampage (AOE melee) at high levels\" (the crits; no area melee)"),
    "charm shark": (
        [r"blackwater shark"],
        {"abilities": [{"kind": "heal", "every": 6, "first": 6, "heal_pct": 3}]},
        "Blackwater Shark: \"One of the best tanking mobs in Odus. Decent melee. Super High HoT.\" (3% of its most a 6 s tick)"),
    "charm knight": (
        [r"partisan of paineel", r"partisan [a-z]+"],
        {"abilities": [{"kind": "lifetap", "every": 15, "first": 3, "spells_of": SK, "damage_pct": NUKE_PCT, "max_recast": 90}]},
        "Partisans: \"Shadowknight guards. Immune to Arcane, quad hit, self-heal.\" (a shadow knight's tap)"),
    "charm dead rider": (
        [r"dead rider"],
        {"abilities": [nuke(15, 4, NEC, NUKE_PCT, "disease")]},
        "Dead rider: \"Very good tank, good melee dmg, good resists, Disease DD, major debuff\" (the disease nuke; no debuff)"),
    "charm breath stealer": (
        [r"breath stealer"],
        {"abilities": [{"kind": "lifetap", "every": 12, "first": 3, "spells_of": NEC, "damage_pct": NUKE_PCT, "max_recast": 30},
                       nuke(15, 8, ENC, NUKE_PCT, "arcane")]},
        "Breath Stealer: \"AR based AoE lifetap of 1200 DMG ... It also quads and nukes.\" (a tap and an arcane nuke, one target)"),
    "charm wicker man": (
        [r"wicker man"],
        {"abilities": [nuke(12, 4, ENC, 75, "arcane")]},
        "Wicker Man: \"Powerful Arcane DD\""),
    "charm centaur": (
        [r"centaur druid"],
        {"abilities": [nuke(12, 3, DRD, NUKE_PCT)]},
        "Cyclops: \"the best pets I've seen have been the Casting Centaurs ... relatively low level (low 30's) but they do great "
        "dmg with their nukes compared to what our store bought pets do\""),
    "charm root": (
        [r"anaconda", r"young anaconda", r"giant anaconda", r"ancient basilisk"],
        {"abilities": [{"kind": "root", "every": 20, "first": 2, "spells_of": DRD}]},
        "Charming: \"I really like anacondas and young anacondas because they cast unbreakable root ... Same goes for Basilisks, "
        "like ancient basilisks in Envar and Geo Cit.\" (a druid's root; it breaks at a hit as every root here)"),
    "charm faun": (
        [r"acolyte faun", r"faun maiden", r"sylvan faun"],
        {"abilities": [{"kind": "heal", "every": 12, "first": 1, "below": 50, "spells_of": CLR}, nuke(12, 4, DRD, NUKE_PCT)]},
        "Charming: \"Oh and Fauns are great casters that heal themselves.\" (a cleric's heal below 50%, a druid's nuke)"),
    "charm eagle": (
        [r"great eagle"],
        {"abilities": [aoe(20, 4, WIZ, 35)]},
        "Charming: \"Great Eagles in Spirittalkers are great but they AoE so be careful.\""),
}
TEMPLATE_DEFAULTS = {"class": -1, "hp_pct": 100, "taken_pct": 100, "melee_pct": 100, "special": "", "special_chance": 0,
                     "special_damage": 0.0, "special_fx": 0, "special_every": 0.0, "special_first": 0.0, "special_jitter": 0.0,
                     "special_from": 0}


def key(name):
    """database.NPCClassKey: lower case, trimmed, no trailing $."""
    return name.strip().lower().removesuffix("$").strip()


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def row_of(db, tbl, name):
    if not has_table(db, tbl):
        return None
    cur = db.execute(f"SELECT * FROM {tbl} WHERE name = ?", (name,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def creature_names(db):
    """every NPC name (npcclass's key) with its spawns' levels, for the names the patterns take"""
    names = {}
    for n, lv in db.execute("SELECT npc_name, npc_level FROM npcs"):
        if n is None:
            continue
        k = key(n)
        lo, hi = names.get(k, (lv, lv))
        names[k] = (min(lo, lv), max(hi, lv))
    return names


def matching(names, pattern):
    rx = re.compile(f"(?:{pattern})")
    return sorted(n for n in names if rx.fullmatch(n))


def template_row(name, fields, words):
    row = dict(TEMPLATE_DEFAULTS)
    row.update({k: v for k, v in fields.items() if k != "abilities"})
    row["name"] = name
    row["abilities"] = json.dumps(fields.get("abilities", []), separators=(",", ":"))
    row["note"] = f"a charmed pet's (the wiki's Charmable Mobs/Charming pages): {words}; tools/seed_npc_charm.py"
    row["source"] = SOURCE
    return row


def plan(db):
    """[(table, row, why)] to add, and what was found or left: [(kind, name, levels, verdict)]"""
    for t in ("npcs", "classtemplates"):
        if not has_table(db, t):
            sys.exit(f"no table {t}: a server of 2026-10-29 or later makes it")
    names = creature_names(db)
    adds, seen, found = [], set(), []

    def charm_row(n, charmable, resist, template, note, kind):
        if n in seen:
            return
        seen.add(n)
        lv = names[n]
        if row_of(db, "npccharm", n) is not None:
            found.append((kind, n, lv, "npccharm has a row: left as it is"))
            return
        adds.append(("npccharm", {"name": n, "charmable": charmable, "resist_pct": resist, "template": template,
                                  "note": note, "source": SOURCE}))
        found.append((kind, n, lv, "written"))

    for pattern, words in UNCHARMABLE:
        hit = matching(names, pattern)
        if not hit:
            found.append(("uncharmable", pattern, None, "no NPC of the name here"))
        for n in hit:
            charm_row(n, 0, 0, "", f"uncharmable: {words}", "uncharmable")
    for pattern, pct, words in RESISTS:
        hit = matching(names, pattern)
        if not hit:
            found.append(("resist", pattern, None, "no NPC of the name here"))
        for n in hit:
            charm_row(n, 1, pct, "", f"resists {pct}%: {words}", "resist")
    for tname, (patterns, fields, words) in TEMPLATES.items():
        hits = sorted({n for p in patterns for n in matching(names, p)})
        if not hits:
            found.append(("template", tname, None, "no NPC of its kinds here: not written"))
            continue
        if row_of(db, "classtemplates", tname) is None:
            adds.append(("classtemplates", template_row(tname, fields, words)))
        else:
            found.append(("template", tname, None, "classtemplates has the row: left as it is"))
        for n in hits:
            charm_row(n, 1, 0, tname, f"charmed, fights by {tname!r}: {words}", tname)
    return adds, found


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst, n = f"{path}.bak-{when}-pre-charm", 1
    while os.path.exists(dst):
        n += 1
        dst = f"{path}.bak-{when}-{n}-pre-charm"
    shutil.copy2(path, dst)
    return dst


def apply(db, adds):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(NPCCHARM_SCHEMA)
        db.execute(LOG_SCHEMA)
        for tbl, row in adds:
            cols = list(row)
            db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])
            db.execute(f"INSERT INTO {LOG} (tbl, place, after, why, applied) VALUES (?, ?, ?, ?, ?)",
                       (tbl, row["name"], json.dumps(row_of(db, tbl, row["name"]), sort_keys=True), WHY, when))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db):
    if not has_table(db, LOG):
        print("nothing logged: nothing to undo")
        return
    logged = db.execute(f"SELECT seq, tbl, place, after FROM {LOG} ORDER BY seq DESC").fetchall()
    back, kept = 0, 0
    db.execute("BEGIN IMMEDIATE")
    try:
        # npccharm first (newest first), then the templates no row names any more
        for seq, tbl, place, after in sorted(logged, key=lambda r: (r[1] != "npccharm", -r[0])):
            if tbl not in ("npccharm", "classtemplates"):
                raise SystemExit(f"{LOG} names table {tbl!r}, which this tool never writes")
            now = row_of(db, tbl, place)
            if now is None:
                continue
            if now != json.loads(after):
                print(f"  {tbl} {place!r}: changed since: left as it is")
                kept += 1
                continue
            if tbl == "classtemplates" and has_table(db, "npccharm") and \
                    db.execute("SELECT count(*) FROM npccharm WHERE template = ?", (place,)).fetchone()[0]:
                print(f"  classtemplates {place!r}: npccharm rows still name it: left as it is")
                kept += 1
                continue
            db.execute(f"DELETE FROM {tbl} WHERE name = ?", (place,))
            back += 1
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"took out {back} of {len(logged)} rows ({kept} left); the server drops them at its next start (npccharm: or an edit "
          "of it in the panel)")


def report(adds, found, show):
    by = {}
    for kind, name, lv, verdict in found:
        by.setdefault(kind, []).append((name, lv, verdict))
    for kind in sorted(by):
        rows = by[kind]
        written = [r for r in rows if r[2] == "written"]
        print(f"{kind}: {len(written)} names written, {len(rows) - len(written)} not")
        for name, lv, verdict in rows:
            if show or verdict != "written":
                levels = f" (level {lv[0]}-{lv[1]})" if lv else ""
                print(f"    {name}{levels}: {verdict}")
    tmpl = [r for t, r in adds if t == "classtemplates"]
    print(f"{len(adds)} rows to add: {len(adds) - len(tmpl)} npccharm, {len(tmpl)} classtemplates "
          f"({', '.join(r['name'] for r in tmpl) or 'none'})")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    ap.add_argument("--list", action="store_true", help="name every row written")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the rows (logged)")
    g.add_argument("--undo", action="store_true", help="take out what --apply wrote")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not a.no_backup and has_table(db, LOG):
            print(f"copy: {backup(a.db)}")
        undo(db)
        return
    adds, found = plan(db)
    report(adds, found, a.list)
    if not a.apply or not adds:
        if not a.apply:
            print("report only: --apply to write")
        return
    if not a.no_backup:
        print(f"copy: {backup(a.db)}")
    apply(db, adds)
    print(f"added {len(adds)} rows: the server reads them at its next start (npccharm: or an edit of it in the panel)")


if __name__ == "__main__":
    main()
