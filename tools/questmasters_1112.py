#!/usr/bin/env python3
"""QuestMasters' lists (2006-07, written by players of the live game; the owner's source 2026-11-12: "staff of
faykind needs updated"), the site's local copy external/questmasters/text/:

  id407 "Items That PROCs": each weapon this server has a record of, whose proc the list gives a figure for and the
    record's words did not, takes the list's proc in its description (entity/weaponproc.go reads a proc from a
    description: "Procs 150 lightning damage.", "Procs a 50 point poison DoT.", "500 DD offensive proc, works only on
    goblins."); Gorereaver's 400 becomes the list's 500 on goblins; Grrkill, "proc on goblins" with no figure, takes
    Gorereaver's (ours); the Staff of Faykind's words say its form as the code now has it (the form's +15 DEX and its
    380 cold + 70% of DEX: entity/weaponproc.go). The class lists the list gives widen a record that lacks a class
    the list names (the "...of Toxicity" weapons, the Gladius of the Serpent Gods and the Noroxus Spine Spear lacked
    the warrior; the Staff of infectious Dread the warrior and the alchemist); a list that names fewer classes than a
    record has ("shadowknight only") is not applied (said in the report). Every record of the name is changed (the
    base, its imported copies, an Ascended copy).
  id576 "Raid Mobs and their Resists": each raid mob this server has (by name) is strong in the resists the list
    names: 50 (the work boards' figure for a raid mob's strong resist), a starred one ("Skahyir- *LR*") 75; a
    template already stronger keeps its figure, a kind it has the list does not name is kept, and a kind the list
    adds to a template of another figure takes its own (resist_kind "AR:50": the code of this branch reads a kind's
    own percent). A template another creature the list does not name shares (roamer, se goblin named, pod twins)
    is copied for the listed one (its row as it is, the resists added, named after it) and npctemplate points the
    listed name at the copy. Not changed: what the GM Jibrish decided has no resists of its own (Contagion, board
    649; Impale, 651; Reaver of Graumord, 654), the Isle of Dread's Dread and Terror (their names are twelve other
    creatures' too: a name-keyed template would reach them; code ask 4), and the mobs this server has no NPC of.

Refused, nothing written, when a row is not as expected (a description not the one this tool was written against, a
proc in words already, a class mask or a template's resists other than expected). Report only by default; --apply in
one transaction, gm_audit lines, log questmasters_1112_log; a second --apply finds nothing to do; --undo exact (a row
changed since is left and said). Items are read at a character's login (its gear's procs and classes); templates at
the server's start or a GM panel template edit (reload_npcs). The resist_kind "LR:75" form needs this branch's code
(an older server refuses the template and the creature fights by none): deploy the code first.

    tools/questmasters_1112.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import hashlib
import json
import sqlite3
import sys

LOG = "questmasters_1112_log"
WHO, VIA = "dab", "tool"
DETAIL = "QuestMasters' lists (2006: id407 Items That PROCs, id576 Raid Mobs and their Resists): tools/questmasters_1112.py"
SOURCE = "questmasters1112"

# --- id407: weapons (lower-case record name -> what changes) ----------------------------------------------------
# add: a sentence added to the description on a line of its own; sha: the first 10 hex of the description's sha1 as
# this tool was written (refused otherwise); classuse: (before, after); replace: (old, new) inside the description.
ITEMS = {
    "bonecleaver": {"line": "Bonecleaver (War only): 200 CR proc, drops off Bone Sentinel in EG",
                    "add": "Procs 200 cold damage."},
    "biting wind scimitar": {"line": "Biting Wind Scimitar, 70 dd proc", "add": "Procs a 70 point DD spell."},
    "burnt rapier": {"line": "Burnt rapier, 80 fire proc", "add": "Procs 80 fire damage."},
    "burning point": {"line": "Burning Point, 150 FR based proc", "add": "Procs 150 fire damage."},
    "baton of curdling souls": {"line": "Curdling Spirits weapons ... 150 dd proc; club of curdling souls: 150 LR proc",
                                "add": "Procs 150 lightning damage."},
    "crippler's star": {"line": "Cripplers star lvl 46 100 proc", "add": "Procs 100 damage."},
    "deep scarab blade": {"line": "Deep Scarab Blade, 10 pr dot", "add": "Procs a 10 point poison DoT."},
    "deadened waraxe": {"line": "Deadened axe/dagger/hammer (34ish life tap proc)", "add": "Procs a 34 point lifetap."},
    "deadened hammer": {"line": "Deadened axe/dagger/hammer (34ish life tap proc)", "add": "Procs a 34 point lifetap."},
    "dawnfire, morning star of light": {"line": "DawnFire Morning Star of Light; 60 cl only- 185 ar nuke proc",
                                        "add": "Procs 185 damage."},
    "exploding fist": {"line": "Exploding Fist- lvl 40 monk knuckles. 17 str and 120 dd proc", "add": "Procs 120 damage."},
    "ebon mandible": {"line": "Ebon Mandible, lvl 50 1hs, some stats and 90 dd proc", "add": "Procs 90 damage."},
    "force of mind": {"line": "Force of Mind, lvl 50 monk glowy knuckles, 150 dd proc", "add": "Procs 150 damage."},
    "grimblade": {"line": "Grimblade, lvl 26 2h sword, 40 dd proc", "add": "Procs 40 damage."},
    "gorereaver": {"line": "gorereaver (Mnk, Mag, Nec, Enc, Wiz, and Alc): procs 500 on goblins",
                   "replace": ("400 DD offensive proc, works only on goblins.", "500 DD offensive proc, works only on goblins.")},
    "grrkill": {"line": "Grrkill - proc on goblins (no figure: Gorereaver's, ours)",
                "add": "500 DD offensive proc, works only on goblins."},
    "heartsplitter": {"line": "Heartsplitter, lvl 45, 20 hp drain proc (40 dd)", "add": "Procs a 20 point lifetap."},
    "imperial forged ranseur": {"line": "Imperial Forged (ALL) weapons proc 120. It's AR based, level 44.",
                                "add": "Procs 120 damage."},
    "mace of despoliation": {"line": "Mace of Despoliation, lvl 28 1hb mace ... 35ish dd proc", "add": "Procs 35 damage."},
    "rhino horn axe of lightning": {"line": "rhino horn axe of lightning (War, Ran, Pal, Sk, Brd, and Rge): 150 LR proc",
                                    "add": "Procs 150 lightning damage."},
    "red fire": {"line": "Red Fire; lvl 60, 500 fr proc, ct drop", "add": "Procs 500 fire damage."},
    "runed broadsword": {"line": "Runed Broadsword; Dmg 440 Proc Fr 99-100", "add": "Procs 99-100 fire damage."},
    "short sword of the ykesha": {"line": "Short Sword of the Ykesha- lv- 37 dam- 470 proc 100 direct damage.",
                                  "add": "Procs 100 damage."},
    "ykesha": {"line": "Ykesha, lvl 46 2hb column, 100 dd proc (ours: the Short Sword of the Ykesha's twin)",
               "add": "Procs 100 damage."},
    "springview knight greatsword": {"line": "Springview Knight Great Sword; Proc 167-168", "add": "Procs 167-168 damage."},
    "naginata of toxicity": {"line": "naginata of toxicity (War, Pal, and SK): 220 DR proc",
                             "add": "Procs 220 disease damage.", "classuse": (12, 13)},
    "no-dachi of toxicity": {"line": "no-dachi of toxicity (War, Pal, and SK): 220 DR proc",
                             "add": "Procs 220 disease damage.", "classuse": (12, 13)},
    "wakizashi of toxicity": {"line": "wakizashi of toxicity (War, Ran, Pal, SK, Brd, and Rge): 220 DR proc",
                              "add": "Procs 220 disease damage.", "classuse": (110, 111)},
    "tanto of toxicity": {"line": "tanto of toxicity (War, Ran, Pal, SK, Brd, and Rge): 220 DR proc",
                          "add": "Procs 220 disease damage.", "classuse": (110, 111)},
    "bladed mace of toxicity": {"line": "bladed mace of toxicity (War, Ran, Pal, SK, Mnk, Brd, Rge, Drd, Sha, and Cl): 220 DR proc",
                                "add": "Procs 220 disease damage.", "classuse": (1022, 1023)},
    "longbow of toxicity": {"line": "Toxicity Weapons- various weapons, including bow ... 220 dd proc",
                            "add": "Procs 220 disease damage."},
    "the rotting fist": {"line": "The Rotting Fist, lvl 60 1hb weapon, 120 DoT proc", "add": "Procs a 120 point DoT."},
    "thornback": {"line": "Thornback and Thornsting, 49 and 50 ranger only swords ... 100 and 70 dd procs",
                  "add": "Procs 100 damage."},
    "twilight": {"line": "Twilight is 150 proc AR resisted.", "add": "Procs 150 damage."},
    "whiptail broadsword": {"line": "Whiptail Broadsword has about a 50 poison proc.", "add": "Procs 50 poison damage."},
    "gladius of the serpent gods": {"line": "gladius of the serpent god (War, Ran, Pal, SK, Brd, and Rge): 50 PR DoT proc",
                                    "add": "Procs a 50 point poison DoT.", "classuse": (110, 111)},
    "noroxus spine spear": {"line": "noroxus spine spear (War, Ran, Pal, and SK): procs root (the code's, by name)",
                            "add": "Procs a root.", "classuse": (14, 15)},
    "song of the melospiza": {"line": "Song of Melospiza, lvl 40 caster weapon ... anti-aggro proc? (the code's, by name)",
                              "add": "Procs a calm: the creature's hate for its wielder halved."},
    "staff of infectious dread": {"line": "Staff of Infectious Dread ... Class: WAR RAN PAL SK MNK DRD SHA CL MAG NEC ENC WIZ ALC",
                                  "classuse": (16286, 32671)},
    "staff of faykind": {"line": "staff of faykind (All/All): procs a fairy form that gives you 15 Dex and gives you a "
                                 "380+70%Dex CR proc (the code's, by name)",
                         "replace": ("A staff of old fay make, light as a reed. Now and then it turns its wielder into a "
                                     "pixie for thirty seconds, renewed each time it does so again while they are one, and "
                                     "a pixie's blows strike for up to 800 more a round.",
                                     "A staff of old fay make. Now and then it turns its wielder into a pixie (+15 DEX) for "
                                     "thirty seconds, renewed when it does so again; a pixie's blows strike for 380 cold and "
                                     "70% of its DEX more, at most 800 a round.")},
}

# the list's class statements narrower than the record's: not applied, said
NARROWER = [
    ("Flamberge of Wounding", "lvl 49 shadowknight only", 111),
    ("Flamberge of Horror", "lvl 50 shadowknight only", 15),
    ("Thornback / Thornsting Longsword", "ranger only swords", 111),
    ("Splendor", "BP lvl 50 (Pal only)", 527),
    ("Desecrated Breastplate", "level 60 SK only", 525),
    ("Rune Crested Gloves", "Ran,Rge,Sha", 879),
]

# --- id576: raid mobs ------------------------------------------------------------------------------------------
LISTED, STARRED = 50, 75

# (page name, its resists as the page lists them ('*LR*' starred), the page's place, our NPC names)
MOBS = [
    ("Quintashi's Handmaiden", "LR CR AR", "vastly deep", ["quintashi's handmaiden"]),
    ("Dreadnaught", "AR CR", "anu village", ["dreadnaught"]),
    ("Skahyir", "*LR*", "box canyon", ["skahyir"]),
    ("Avatar of Fear", "*LR* AR PR DR", "cazic thule", ["avatar of fear"]),
    ("Hand of Thule", "*LR* AR PR DR", "cazic thule", ["hand of thule"]),
    ("Bone Sentinel", "AR DR", "eg", []),
    ("Overlord Xrag", "FR CR LR", "eg", ["overlord xrag"]),
    ("Siliskor", "PR FR", "eg", ["siliskor"]),
    ("Prophet Envar", "AR DR", "envar", []),
    ("Terrorantula", "PR", "eternal desert", ["terrorantula"]),
    ("Archon Avereth", "PR", "geo cit", []),
    ("Archon Vetash", "FR AR", "geo cit", ["archon vetash"]),
    ("Grundle", "AR CR", "geo cit", ["grundle"]),
    ("Sarek", "PR DR AR", "geo cit", ["sarek the betrayer"]),
    ("Bloodletter", "AR FR", "iod", ["bloodletter"]),
    ("Cazic Thule", "AR PR DR LR", "iod", ["cazicthule"]),
    ("Contagion", "AR PR", "iod", ["contagion"]),
    ("Dread", "AR LR", "iod", ["dread"]),
    ("Enraged Phantasm", "AR FR", "iod", ["enraged phantasm"]),
    ("Festering Jack", "AR FR", "iod", ["festering jack"]),
    ("Fright", "AR LR", "iod", ["fright"]),
    ("Impale", "LR", "iod", ["impale"]),
    ("Megaladon", "AR PR", "iod", ["megaladon"]),
    ("Qual'Tin", "AR LR PR DR", "iod", ["qual'tin"]),
    ("Reaver of Graumord", "AR LR", "iod", ["reaver of graumord"]),
    ("Terror", "AR LR PR DR", "iod", ["terror"]),
    ("The Watchman", "LR", "iod", ["the watchman", "the watchmen"]),
    ("Thoughtbleeder", "AR LR PR FR", "iod", ["thoughtbleeder"]),
    ("Zin the Dessicator", "AR", "iod", ["zin the desiccator"]),
    ("Kratas", "PR DR AR", "mila's reef", ["kratas"]),
    ("Bertoxxulous", "AR PR DR FR", "pod", ["bertoxxulous"]),
    ("Fleshripper", "DR AR PR", "pod", ["fleshripper"]),
    ("Noroxus", "PR DR AR FR", "pod", ["noroxus"]),
    ("Poxathixius", "PR DR", "pod", ["poxathixius"]),
    ("Queen Qy'tha", "PR DR AR LR", "pod", ["queen qy'tha"]),
    ("Urzathixius", "PR DR", "pod", ["uzrathixius"]),
    ("Lady Vox", "PR CR", "permafrost", ["lady vox", "enraged lady vox"]),
    ("Snowgrin", "CR", "permafrost", ["snowgrin"]),
    ("Aeyinar", "CR AR", "pos snow chain", ["enraged aeyinar"]),
    ("Chimera the Tyrant", "AR", "pos rain chain", []),
    ("King Ruhtra", "AR", "pos night chain", []),
    ("Rancorous", "AR PR", "pos sun chain", ["rancorous drake"]),
    ("Quintessence of Veeshan", "AR LR", "pos deso chain", ["quintessence of veeshan"]),
    ("Toxxulia", "AR DR", "south toxx", ["toxxulia"]),
    ("Arch Slater Grrdan", "FR", "se", ["arch slayer grrdan"]),
    ("Cave Hydra", "AR PR DR", "se", ["cave hydra"]),
    ("Chief Growlnar", "FR LR", "se", ["chief growlnar"]),
    ("Davoh the Dark", "PR AR", "se", ["davoh the dark"]),
    ("Duct Crawler Queen", "PR", "se", ["duct crawler queen"]),
    ("Efreeti Lord Djarn", "FR", "se", ["efreeti lord djarn"]),
    ("Elder Grryow", "FR LR", "se", ["elder grryow"]),
    ("Elder Frayer Grrva", "AR FR LR", "se", ["elder flayer grrva"]),
    ("Enrager Grrlok", "FR", "se", ["enrager grrlok"]),
    ("Gimha the Stunted", "FR", "se", ["gimha the stunted"]),
    ("Inquistor Zak", "FR LR", "se", ["inquisitor zak"]),
    ("Kindle", "AR FR LR", "se", ["kindle"]),
    ("King Daran", "AR", "se", ["king daran"]),
    ("King Grog", "FR AR", "se", ["king grog"]),
    ("Laknar", "FR", "se", ["laknar"]),
    ("Lava Enforcer", "FR", "se", ["lava enforcer"]),
    ("Lord Nagafen", "FR PR", "se", ["lord nagafen"]),
    ("Orim Flamestone", "CR LR FR", "se", ["orim flamestone"]),
    ("Raver Thoran", "FR AR", "se", ["reaver thoran"]),
    ("Singe", "FR", "se", ["singe"]),
    ("Wrag the Unclean", "AR DR", "se", ["wrag the unclean"]),
]

# not changed, said: the GM's decisions, and names a template by name would reach elsewhere
LEFT = {
    "contagion": "the GM Jibrish's board 649 (2026-11-10): no resists of its own ('t281 is the players' list: AR, PR')",
    "impale": "the GM Jibrish's board 651 (2026-11-10): no resists of its own ('t281 is the players' list: AR')",
    "reaver of graumord": "the GM Jibrish's board 654 (2026-11-10): no resists of his own ('t281 is the players' list: LR, AR')",
    "dread": "the name is 7 creatures' elsewhere (L33-55): a template by name would reach them (code ask 4)",
    "terror": "the name is 6 creatures' elsewhere (L41-55): a template by name would reach them (code ask 4)",
}

# templates another creature the list does not name shares: a copy for the listed one
SHARED = {"roamer", "se goblin named", "pod twins"}
# the copies' names (npc name -> template name) where one copy serves several listed names
COPY_NAME = {"poxathixius": "poxathixius and uzrathixius", "uzrathixius": "poxathixius and uzrathixius"}

# Tunaria's zone grid (claude-notes/tunaria-map.md: a zone a 2000 x 2000 cell from its corner), the names the list's
# places are, to compare
TUNARIA = {(8000, 4000): "Anu Village", (4000, 4000): "Permafrost", (20000, 20000): "Box Canyons",
           (20000, 22000): "Eternal Desert", (12000, 30000): "Milas Reef", (14000, 30000): "Cazic-Thule",
           (24000, 2000): "Isle of Dread", (22000, 22000): "Sycamore Joys Rest", (24000, 24000): "Elemental Towers",
           (22000, 24000): "Great Waste", (14000, 26000): "Elephant Graveyard", (14000, 22000): "Brog Fens",
           (22000, 12000): "Mu Lins Reach", (6000, 10000): "Twisted Tower", (18000, 28000): "Burial Mounds"}
WORLDS = {0: "Tunaria", 1: "Rathe", 2: "Odus", 3: "Lavastorm", 4: "Plane of Sky", 5: "Plane of Disease"}

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    op     TEXT NOT NULL,
    tbl    TEXT NOT NULL,
    keycol TEXT NOT NULL,
    keyval TEXT NOT NULL,
    col    TEXT NOT NULL,
    before TEXT,
    after  TEXT,
    audit  INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


def audit(db, action, tbl, keycol, keyval, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, compact({keycol: keyval}),
                      None if before is None else compact(before), None if after is None else compact(after), DETAIL))
    return cur.lastrowid


# --- planning: each change is (op, tbl, keycol, keyval, col, before, after); op update or insert ------------------

def plan_items(db):
    changes, lines, refusals = [], [], []
    for name, it in ITEMS.items():
        rows = db.execute("SELECT patternid, itemname, itemdesc, classuse FROM itempattern WHERE lower(itemname) = ? "
                          "OR lower(itemname) = ? ORDER BY patternid", (name, "ascended " + name)).fetchall()
        if not rows:
            refusals.append(f"item {name!r}: no record")
            continue
        for pid, iname, desc, cu in rows:
            tag = f"itempattern {pid} {iname!r}"
            want = desc
            if "add" in it:
                add = it["add"]
                if desc.endswith("\n" + add) or desc == add:
                    lines.append(f"{tag}: its words say {add!r} already")
                else:
                    if "proc" in desc.lower():
                        refusals.append(f"{tag}: a proc in its words already: {desc!r}")
                        continue
                    want = (desc.rstrip() + "\n" + add) if desc.strip() else add
            if "replace" in it:
                old, new = it["replace"]
                if old in desc:
                    want = desc.replace(old, new)
                elif new not in desc:
                    refusals.append(f"{tag}: its words are not the ones expected: {desc!r}")
                    continue
                else:
                    lines.append(f"{tag}: its words are the new ones already")
            if want != desc:
                if len(want) > 240:
                    refusals.append(f"{tag}: the description would be {len(want)} characters")
                    continue
                changes.append(("update", "itempattern", "patternid", pid, "itemdesc", desc, want))
                lines.append(f"{tag}: description + {want[len(desc):].strip()!r}" if want.startswith(desc)
                             else f"{tag}: description -> {want!r}")
            if "classuse" in it:
                b, a = it["classuse"]
                if cu == a:
                    lines.append(f"{tag}: classes {a} already")
                elif cu != b:
                    refusals.append(f"{tag}: classuse {cu}, want {b}")
                else:
                    changes.append(("update", "itempattern", "patternid", pid, "classuse", b, a))
                    lines.append(f"{tag}: classuse {b} -> {a} ({it['line']})")
    return changes, lines, refusals


def parse_kinds(s):
    """[(kind, own pct or 0)] of a resist_kind."""
    out = []
    for tok in (s or "").split(","):
        tok = tok.strip()
        if not tok:
            continue
        k, _, p = tok.partition(":")
        out.append((k.strip().upper(), int(p) if p.strip() else 0))
    return out


def page_kinds(s):
    """[(kind, figure)] of the page's list: a starred one 75, else 50."""
    out = []
    for tok in s.split():
        star = tok.startswith("*")
        out.append((tok.strip("*").upper(), STARRED if star else LISTED))
    return out


def merged(kind_s, pct, listed):
    """the template's resist_kind and resist_pct with the list's kinds in: (kind, pct, what changed)."""
    have = parse_kinds(kind_s)
    figs = {k: (own or pct) for k, own in have}
    order = [k for k, _ in have]
    if not have:
        # a template strong in nothing: the list's commonest figure is its resist_pct
        pct = LISTED if any(f == LISTED for _, f in listed) else max(f for _, f in listed)
    added = []
    for k, f in listed:
        cur = figs.get(k, 0)
        if cur >= f:
            continue
        figs[k] = f
        if k not in order:
            order.append(k)
        added.append(f"{k} {cur}->{f}" if cur else f"{k} {f}")
    toks = [k if figs[k] == pct else f"{k}:{figs[k]}" for k in order]
    return ",".join(toks), pct, added


def plan_mobs(db):
    changes, lines, refusals, places = [], [], [], []
    done_templates = {}  # template name -> (kind, pct) planned, for shared rows
    copies = {}          # copy name -> (base, kind, pct)
    for page, res, where, npcs in MOBS:
        listed = page_kinds(res)
        if not npcs:
            lines.append(f"{page} [{res}] ({where}): no NPC of the name here")
            continue
        for n in npcs:
            spots = db.execute("SELECT world, zone, x_coord, z_coord, npc_level FROM npcs WHERE lower(trim(npc_name)) = ? "
                               "OR lower(trim(npc_name)) = ?", (n, n + "$")).fetchall()
            if not spots:
                refusals.append(f"{page}: no NPC {n!r}")
                continue
            sp = []
            for w, z, x, zz, lv in spots:
                place = WORLDS.get(w, f"world {w}")
                if w == 0 and x is not None and zz is not None:
                    place += " / " + TUNARIA.get((int(x // 2000) * 2000, int(zz // 2000) * 2000), f"cell {int(x // 2000) * 2000},{int(zz // 2000) * 2000}")
                sp.append(f"L{lv} {place} zone {z}")
            places.append(f"{page} ({where}): {n}: " + "; ".join(sorted(set(sp))))
            if n in LEFT:
                lines.append(f"{page} [{res}]: {n} left: {LEFT[n]}")
                continue
            t = db.execute("SELECT template FROM npctemplate WHERE name = ?", (n,)).fetchone()
            if t is None:
                refusals.append(f"{page}: {n!r} has no npctemplate row")
                continue
            tname = t[0]
            base = tname
            if tname in SHARED:
                cname = COPY_NAME.get(n, n)
                row = db.execute("SELECT resist_kind, resist_pct FROM classtemplates WHERE name = ?", (base,)).fetchone()
                if row is None:
                    refusals.append(f"{page}: template {base!r} missing")
                    continue
                kind, pct, added = merged(row[0], row[1], listed)
                if not added:
                    lines.append(f"{page} [{res}]: {n} on {base!r} ({row[0] or 'none'} {row[1]}): as listed already")
                    continue
                if cname not in copies:
                    ex = db.execute("SELECT resist_kind, resist_pct FROM classtemplates WHERE name = ?", (cname,)).fetchone()
                    if ex is not None:
                        refusals.append(f"{page}: a template {cname!r} is there already ({ex[0]} {ex[1]})")
                        continue
                    copies[cname] = (base, kind, pct)
                    changes.append(("insert", "classtemplates", "name", cname, "*", base, compact({"resist_kind": kind, "resist_pct": pct})))
                    lines.append(f"{page} [{res}]: classtemplates {cname!r} copied from {base!r} ({row[0] or 'none'} {row[1]}), "
                                 f"resists {kind} at {pct}")
                changes.append(("update", "npctemplate", "name", n, "template", base, cname))
                lines.append(f"  npctemplate {n!r}: {base!r} -> {cname!r}")
                continue
            row = db.execute("SELECT resist_kind, resist_pct FROM classtemplates WHERE name = ?", (tname,)).fetchone()
            if row is None:
                refusals.append(f"{page}: {n!r}'s template {tname!r} missing")
                continue
            if tname in done_templates:
                kind_s, pct0 = done_templates[tname]
            else:
                kind_s, pct0 = row
            kind, pct, added = merged(kind_s, pct0, listed)
            if not added:
                lines.append(f"{page} [{res}]: {n}'s template {tname!r} ({kind_s or 'none'} {pct0}): as listed already")
                continue
            done_templates[tname] = (kind, pct)
            lines.append(f"{page} [{res}]: {n}'s template {tname!r}: resists {row[0] or 'none'} {row[1]} -> {kind} {pct} "
                         f"({', '.join(added)})")
    # the in-place template updates, once each, from the row as it is
    for tname, (kind, pct) in done_templates.items():
        row = db.execute("SELECT resist_kind, resist_pct FROM classtemplates WHERE name = ?", (tname,)).fetchone()
        if row[0] != kind:
            changes.append(("update", "classtemplates", "name", tname, "resist_kind", row[0], kind))
        if row[1] != pct:
            changes.append(("update", "classtemplates", "name", tname, "resist_pct", row[1], pct))
    changes = [c for c in changes if not (c[0] == "update" and c[5] == c[6])]
    return changes, lines, refusals, places


def copied_row(db, base, cname, kind, pct):
    cols = [r[1] for r in db.execute("PRAGMA table_info(classtemplates)")]
    row = db.execute(f"SELECT {', '.join(cols)} FROM classtemplates WHERE name = ?", (base,)).fetchone()
    if row is None:
        raise RuntimeError(f"template {base!r} gone")
    d = dict(zip(cols, row))
    d.update(name=cname, resist_kind=kind, resist_pct=pct, source=SOURCE,
             note=f"{base!r} as it was on {datetime.date.today().isoformat()}, strong in {kind} at {pct} for "
                  f"QuestMasters' raid list (2006, id576): tools/questmasters_1112.py. {base!r}'s note: {d['note']}")
    return cols, d


def write(db, ch):
    op, tbl, keycol, keyval, col, before, after = ch
    if op == "update":
        cur = db.execute(f"UPDATE {tbl} SET {col} = ? WHERE {keycol} = ? AND {col} IS ?", (after, keyval, before))
        if cur.rowcount != 1:
            raise RuntimeError(f"{tbl} {keyval} {col} changed under the update")
        return audit(db, "update", tbl, keycol, keyval, {col: before}, {col: after})
    a = json.loads(after)
    cols, d = copied_row(db, before, keyval, a["resist_kind"], a["resist_pct"])
    db.execute(f"INSERT INTO classtemplates ({', '.join(cols)}) VALUES ({', '.join('?' for _ in cols)})", [d[c] for c in cols])
    return audit(db, "insert", tbl, keycol, keyval, None, d)


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, op, tbl, keycol, keyval, col, before, after, _ in db.execute(f"SELECT * FROM {LOG} ORDER BY seq DESC").fetchall():
        key = int(keyval) if keycol == "patternid" else keyval
        if op == "update":
            if tbl == "itempattern" and col == "classuse":
                before, after = int(before), int(after)
            elif tbl == "classtemplates" and col == "resist_pct":
                before, after = int(before), int(after)
            cur = db.execute(f"UPDATE {tbl} SET {col} = ? WHERE {keycol} = ? AND {col} IS ?", (before, key, after))
            if cur.rowcount != 1:
                print(f"left: {tbl} {keyval} {col} changed since")
                left += 1
                continue
            audit(db, "update", tbl, keycol, key, {col: after}, {col: before})
        else:
            a = json.loads(after)
            used = db.execute("SELECT COUNT(*) FROM npctemplate WHERE template = ?", (key,)).fetchone()[0]
            row = db.execute("SELECT resist_kind, resist_pct FROM classtemplates WHERE name = ?", (key,)).fetchone()
            if used or row is None or row[0] != a["resist_kind"] or row[1] != a["resist_pct"]:
                print(f"left: classtemplates {keyval} changed since or still in use ({used})")
                left += 1
                continue
            db.execute("DELETE FROM classtemplates WHERE name = ?", (key,))
            audit(db, "delete", tbl, keycol, key, {"name": key}, None)
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
    ic, il, ir = plan_items(db)
    mc, ml, mr, places = plan_mobs(db)
    print("== id407 Items That PROCs")
    for line in il:
        print(line)
    print("not applied (the list names fewer classes than the record has):")
    for n, said, cu in NARROWER:
        print(f"  {n}: the list's {said!r}; classuse {cu} kept")
    print("== id576 Raid Mobs and their Resists")
    for line in ml:
        print(line)
    print("== where they are (to compare with the list's places; nothing moved)")
    for p in places:
        print("  " + p)
    refusals = ir + mr
    if refusals:
        print("refused, nothing done:")
        for r in refusals:
            print("  " + r)
        return 1
    changes = ic + mc
    if not changes:
        print("nothing to do")
        return 0
    if not a.apply:
        print(f"report only: {len(changes)} changes would be written (--apply writes them)")
        return 0
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        for ch in changes:
            a_id = write(db, ch)
            op, tbl, keycol, keyval, col, before, after = ch
            db.execute(f"INSERT INTO {LOG} (op, tbl, keycol, keyval, col, before, after, audit) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                       (op, tbl, keycol, str(keyval), col, None if before is None else str(before), str(after), a_id))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"applied: {len(changes)} changes ({LOG})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
