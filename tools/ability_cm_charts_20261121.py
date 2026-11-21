#!/usr/bin/env python3
"""Abilities and class masteries set to the players' forum charts (2026-11-21; the owner's pointer to the forum section
"Abilitiy & CM Charts", eqoa.forumotion.com f52: the class spell lists t309-t323, the CM database t324-t370, the level 60
epic abilities t305, the Zaoran Plateau abilities t308; posted 2010 from the Allakhazam records).

The whole section was read and compared with spellpattern / classspells / cmcatalog on both shards (the full per-item
diff: the audit's report.md). Most of our data came from the same records and matches; this tool corrects only what
differs on rows that EXIST, by NAME (a spell by its name and a class that has it, a mastery by its name, rank and class),
each field from the value read to the chart's. No spell, mastery or rank is made here; what the charts list and we lack
is in the report.

Two groups:
  apply   chart-stated corrections that make nobody weaker: a level lowered, a cost lowered, a stat moved to the one the
          chart names (HoT -> PoT on a caster, WISMax -> INTMax on a magician, INT -> CHA on an enchanter's Empathy), a
          requirement loosened, a rank's ability swapped to the one the chart names.
  review  chart-stated changes that would make a class weaker or a mastery harder to reach (a level raised, a stat or
          figure lowered, a requirement tightened), or whose chart is incomplete: skipped unless --with-review, for the
          owner to choose (the owner: top-tier strength is intended; never nerf on one's own).

A change whose row reads neither the value this tool expects nor its new value is refused and reported (the row was
changed since it was read; e.g. a GM's edit). A change already in place is skipped. --skip KEY leaves a change out.

Report only by default; --apply writes, a gm_audit line a row, the old values kept in ability_cm_charts_20261121_log;
--undo puts back each field still as this tool left it. A second --apply finds nothing to do.

WHEN IT TAKES EFFECT: a spell's power, cast and recast at its next cast; its level in a spell book at the player's next
login. cmcatalog is read at the server's start, or at once when any class mastery is saved in the GM panel (which reads
the whole catalog again).

    tools/ability_cm_charts_20261121.py --db eqoa.db [--apply | --undo] [--with-review] [--skip KEY ...]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "ability_cm_charts_20261121_log"
DETAIL = "the forum's ability and CM charts (eqoa.forumotion.com f52, %s): tools/ability_cm_charts_20261121.py"
ACCOUNT, WHO, VIA = 1, "dab", "tool"

WAR, RAN, PAL, SK, MNK, BRD, RGE, DRD, SHA, CLR, MAG, NEC, ENC, WIZ, ALC = range(15)


def S(key, group, cite, name, cls, fields, note=""):
    """A spellpattern row: the spell of that name a class has (classspells)."""
    return dict(key=key, group=group, cite=cite, kind="spell", name=name, cls=cls, fields=fields, note=note)


def C(key, group, cite, mastery, rank, cls, fields, note=""):
    """A cmcatalog row: the mastery's rank whose classes include the class."""
    return dict(key=key, group=group, cite=cite, kind="cm", name=mastery, rank=rank, cls=cls, fields=fields, note=note)


SAGE_OLD = "STAMax:50,DEXMax:50,INTMax:50,HP:300,Pow:500,PoT:25"
SAGE_NEW = "STAMax:50,DEXMax:50,HP:300,Pow:300,PoT:25"
ELDER_STATS = "CHA:50,WISMax:50,CHAMax:50,HP:250"

CHANGES = [
    # --- apply: spells ---------------------------------------------------------------------------------------------
    S("circle-barren-coast", "apply", "t312", "Circle of Barren Coast", DRD, {"abilitylvl": (60, 50)}),
    S("circle-rathe", "apply", "t312", "Circle of Rathe", DRD, {"abilitylvl": (60, 50)}),
    S("circle-stonebrunt", "apply", "t312", "Circle of Stonebrunt", DRD, {"abilitylvl": (60, 55)}),
    S("portal-rathe", "apply", "t323", "Rathe Mountains Portal", WIZ, {"abilitylvl": (60, 50)}),
    S("portal-stonebrunt", "apply", "t323", "Stonebrunt Portal", WIZ, {"abilitylvl": (60, 55)}),
    S("portal-barren-coast", "apply", "t323", "Barrens Coast Portal", WIZ, {"abilitylvl": (60, 50)}),
    S("binding-grasp", "apply", "t313", "Binding Grasp", ENC, {"abilitylvl": (60, 55)}),
    S("mana-flow", "apply", "t313", "Mana Flow", ENC, {"abilitylvl": (60, 57)}),
    S("divine-slash", "apply", "t317", "Divine Slash", PAL, {"abilitylvl": (13, 10)}),
    S("improved-invisibility-mag", "apply", "t314", "Improved Invisibility", MAG, {"abilitylvl": (35, 33)}),
    S("sprite-form", "apply", "t312, t348", "Sprite Form", DRD, {"abilitylvl": (51, 49)},
      "Nature Sway 3 (level 50) gives it; at 51 it was unusable for a level 50 buyer"),
    S("cocktail-of-torment", "apply", "t305", "Severupidus' Cocktail of Torment", ALC,
      {"power": (500, 140), "casttime": (2, 1), "recast": (2, 1)}),
    S("infectious-aura", "apply", "t305", "Infectious Aura", SK, {"recast": (300, 90)}),
    S("rage-of-zek", "apply", "t308", "Rage of Zek", WAR, {"power": (500, 100)}),
    S("planar-fury", "apply", "t308", "Planar Fury", WAR, {"power": (500, 422)}),
    # --- apply: class masteries ----------------------------------------------------------------------------------
    C("guardian", "apply", "t357", "Guardian", 1, PAL,
      {"effects": ("AGI:50,STAMax:50,AC:300,Def Mod:2,HP Factor:2", "AGI:50,STAMax:50,AC:550,Def Mod:2,HP Factor:2"),
       "minstats": ("STA:200,AGI:125", "STA:200")}),
    C("death-knight-ac", "apply", "t363", "Death Knight", 1, SK,
      {"effects": ("STA:50,STAMax:50,AGIMax:50,AC:300,Def Mod:2,FR:25,LR:25,CR:25",
                   "STA:50,STAMax:50,AGIMax:50,AC:550,Def Mod:2,FR:25,LR:25,CR:25")},
      "AC only; the rest of the chart's Death Knight is in review"),
    C("storm-caller", "apply", "t347", "Storm Caller", 1, DRD,
      {"effects": ("DEX:50,DEXMax:50,Pow:500,HoT:5,LR:25", "DEX:50,DEXMax:50,Pow:500,PoT:5,LR:25")}),
    C("aeromancer", "apply", "t351", "Aeromancer", 1, MAG,
      {"effects": ("AGI:25,AGIMax:25,WISMax:50,PoT:20,LR:50,PR:-15,DR:-15,MovRate:15",
                   "AGI:25,AGIMax:25,INTMax:50,PoT:20,LR:50,PR:-15,DR:-15,MovRate:15")}),
] + [
    C("empathy-%d" % r, "apply", "t350", "Emphathy", r, ENC, {"effects": ("INT:5", "CHA:5")}) for r in range(1, 6)
] + [
    C("malignant-shroud-1", "apply", "t364", "Malignant Shroud", 1, SK, {"level": (20, 12)},
      "it gives Veil of Death + (level 12)"),
] + [
    C("primal-scream-%d" % r, "apply", "t364", "Primal Scream", r, SK, {"cost": (25, 20)}) for r in range(1, 5)
] + [
    C("hero", "apply", "t367", "Hero", 1, WAR, {"minstats": ("STR:200,STA:250", "STR:200,STA:200")}),
    C("elder-1", "apply", "t365", "Elder", 1, SHA, {"effects": ("", ELDER_STATS)},
      "the chart's Elder stats sit on rank 1; ours were on rank 2 (Tribal Ward)"),
    C("elder-2", "apply", "t365", "Elder", 2, SHA, {"effects": (ELDER_STATS, "")}),
    C("breathy-ballad-1", "apply", "t344, t310", "Breathy Ballad", 1, BRD,
      {"adds": ("Ballad of Breath@40", "Ballad of Breathing@20"), "add_spells": ("700983", "700883")},
      "rank 1 (level 20) gave the level 40 song and rank 3 the level 20 one"),
    C("breathy-ballad-3", "apply", "t344, t310", "Breathy Ballad", 3, BRD,
      {"adds": ("Ballad of Breathing@20", "Ballad of Breath@40"), "add_spells": ("700883", "700983")}),
    # --- review: weaker, tighter, or the chart incomplete ---------------------------------------------------------
    C("sage-alc", "review", "t341", "Sage", 1, ALC, {"effects": (SAGE_OLD, SAGE_NEW)}, "loses INTMax 50 and 200 power"),
    C("sage-enc", "review", "t349", "Sage", 1, ENC, {"effects": (SAGE_OLD, SAGE_NEW)}, "loses INTMax 50 and 200 power"),
    C("sage-mag", "review", "t351", "Sage", 1, MAG, {"effects": (SAGE_OLD, SAGE_NEW)}, "loses INTMax 50 and 200 power"),
    C("sage-nec", "review", "t355", "Sage", 1, NEC, {"effects": (SAGE_OLD, SAGE_NEW)}, "loses INTMax 50 and 200 power"),
    C("sage-wiz", "review", "t369", "Sage", 1, WIZ, {"effects": (SAGE_OLD, SAGE_NEW)},
      "loses INTMax 50 and 200 power (already so on Heirloom by a GM's edit 2026-11-16)"),
    C("gypsy", "review", "t343", "Gypsy", 1, BRD,
      {"effects": ("AGI:10,DEX:50,AGIMax:50,DEXMax:50,Off Mod:2,MovRate:10",
                   "AGI:10,DEX:50,AGIMax:50,DEXMax:25,Off Mod:2,MovRate:10")}, "DEXMax 50 -> 25"),
    C("exorcist", "review", "t345", "Exorcist", 1, CLR,
      {"effects": ("WISMax:100,Pow:200,PoT:10,Off Mod:2,PR:25,DR:25", "WISMax:100,Pow:200,PoT:10,PR:25,DR:25")},
      "loses Off Mod 2"),
    C("death-knight-rest", "review", "t363", "Death Knight", 1, SK,
      {"effects": ("STA:50,STAMax:50,AGIMax:50,AC:550,Def Mod:2,FR:25,LR:25,CR:25",
                   "STAMax:50,AGIMax:50,HP:300,AC:550,Def Mod:2,FR:25,LR:25,CR:25"),
       "minstats": ("STA:200,AGI:150", "STR:150,STA:200,AGI:150")},
      "STA 50 for HP 300 (the chart's HPMax), and STR 150 required; needs death-knight-ac first"),
    C("minstrel", "review", "t343", "Minstrel", 1, BRD, {"minstats": ("AGI:150,CHA:150", "AGI:200,CHA:150")},
      "requirement tightened"),
    C("animator", "review", "t349", "Animator", 1, ENC, {"minstats": ("INT:150,CHA:200", "INT:200,CHA:150")},
      "requirement swapped"),
    C("slayer", "review", "t363", "Slayer", 1, SK, {"minstats": ("STR:200,INT:150", "STR:200,STA:150,INT:150")},
      "requirement tightened"),
    C("sorcerer", "review", "t369", "Sorcerer", 1, WIZ, {"minstats": ("INT:200", "DEX:200,INT:200")},
      "requirement tightened"),
    C("sensei", "review", "t353", "Sensei", 1, MNK,
      {"minstats": ("STR:150,STA:150,WIS:125", "STA:150,AGI:150,WIS:125")}, "requirement swapped STR -> AGI"),
    C("mystic", "review", "t365", "Mystic", 1, SHA,
      {"effects": ("WISMax:50,Pow:500,PoT:20", "STAMax:25,WISMax:75,Pow:500")}, "PoT 20 for STAMax 25 + WISMax 25"),
    C("hero-shout-3", "review", "t368", "Hero Shout", 3, WAR, {"level": (44, 45)}, "a level later"),
] + [
    C("icy-glare-cm-%d" % r, "review", "t350", "Icy Glare", r, ENC, {"level": (o, n)},
      "earlier; the chart gives no spell levels, so the spells (icy-glare-%d) move with it" % r)
    for r, o, n in ((1, 24, 20), (2, 34, 30), (3, 44, 40), (4, 55, 50))
] + [
    S("icy-glare-%d" % r, "review", "t350", "Icy Glare %d" % r, ENC, {"abilitylvl": (o, n)}, "with icy-glare-cm-%d" % r)
    for r, o, n in ((1, 24, 20), (2, 34, 30), (3, 44, 40), (4, 55, 50))
] + [
    S("lava-strike", "review", "t314", "Lava Strike", MAG, {"abilitylvl": (12, 20)}, "8 levels later"),
    S("mortal-mask", "review", "t319", "Mortal Mask", RGE, {"abilitylvl": (3, 27)}, "24 levels later; chart typo?"),
    S("predacious-instinct", "review", "t318", "Predacious Instinct", RAN, {"abilitylvl": (55, 60)}, "5 levels later"),
    S("cure-mystical-plague", "review", "t311", "Cure Mystical Plague", CLR, {"abilitylvl": (55, 60)},
      "5 levels later, for druids and shamans too (one record)"),
    S("ahriannas-generous-gift", "review", "t305", "Ahrianna's Generous Gift", ENC,
      {"power": (400, 780), "casttime": (3, 4), "recast": (24, 1)}, "dearer and slower, but recast 24 -> 1"),
    S("strike-of-the-serpent", "review", "t305", "Strike of the Serpent", MNK, {"casttime": (0, 6)}, "a 6 s cast"),
    S("melinis-tranquil-stance", "review", "t305", "Melinis' Tranquil Stance", MNK, {"recast": (150, 600)},
      "recast 4x longer (the chart's power 0 left as ours)"),
    S("graals-venomus-strike", "review", "t305", "Graal's Venomus Strike", RAN, {"power": (480, 658)}, "dearer"),
    S("sothimus-critical-wounding", "review", "t305", "Sothimus Critical Wounding", RGE, {"recast": (120, 300)},
      "recast 2.5x longer"),
    S("punt", "review", "t305", "Punt", WAR, {"casttime": (0, 1)}, "a 1 s cast"),
    S("vibrance", "review", "t308", "Vibrance", CLR, {"casttime": (6, 2), "recast": (1, 3)}, "faster cast, longer recast"),
    S("divinity", "review", "t308", "Divinity", CLR,
      {"casttime": (6, 2), "recast": (1, 3), "range": (16880, 16544)}, "faster cast, longer recast, range 30 -> 5"),
    S("influence", "review", "t308", "Influence", CLR, {"range": (16880, 16544)}, "range 30 -> 5"),
] + [
    S("fingers-of-%s" % e.lower(), "review", "t308", "Fingers of %s" % e, WIZ, {"casttime": (1, 2), "recast": (6, 3)},
      "slower cast, shorter recast") for e in ("Flame", "Frost", "Thunder")
] + [
    S("fury-of-the-akessan", "review", "t308", "Fury of the Akessan", WAR, {"recast": (30, 90)}, "recast 3x longer"),
]

KEYS = [c["key"] for c in CHANGES]
assert len(KEYS) == len(set(KEYS)), "duplicate change keys"


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def audit(db, tbl, rowkey, before, after, cite):
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', ?, ?, ?, ?, ?)",
               (now(), ACCOUNT, WHO, VIA, tbl, go_json(rowkey), go_json(before), go_json(after), DETAIL % cite))


def table_of(c):
    return ("spellpattern", "spellid") if c["kind"] == "spell" else ("cmcatalog", "id")


def locate(db, c):
    """The one row a change names, or (None, why)."""
    if c["kind"] == "spell":
        rows = db.execute("SELECT DISTINCT p.spellid FROM spellpattern p JOIN classspells cs ON cs.spellid = p.spellid "
                          "WHERE lower(p.spellname) = lower(?) AND cs.class = ?", (c["name"], c["cls"])).fetchall()
        what = "spell %r of class %d" % (c["name"], c["cls"])
    else:
        rows = db.execute("SELECT id FROM cmcatalog WHERE lower(mastery) = lower(?) AND rank = ? AND (classes >> ?) & 1 "
                          "AND enabled = 1", (c["name"], c["rank"], c["cls"])).fetchall()
        what = "mastery %r rank %d of class %d" % (c["name"], c["rank"], c["cls"])
    if len(rows) != 1:
        return None, "%s: %d rows" % (what, len(rows))
    return rows[0][0], None


def status(db, c):
    """('todo'|'done'|'refused', rowid, current values, why)."""
    rid, why = locate(db, c)
    if rid is None:
        return "refused", None, None, why
    tbl, pk = table_of(c)
    cols = list(c["fields"])
    cur = dict(zip(cols, db.execute(f"SELECT {', '.join(cols)} FROM {tbl} WHERE {pk} = ?", (rid,)).fetchone()))
    if all(cur[f] == new for f, (old, new) in c["fields"].items()):
        return "done", rid, cur, ""
    bad = [f"{f} reads {cur[f]!r}, not {old!r}" for f, (old, new) in c["fields"].items() if cur[f] not in (old, new)]
    if bad:
        return "refused", rid, cur, "changed since read: " + "; ".join(bad)
    return "todo", rid, cur, ""


def chosen(with_review, skip):
    unknown = [k for k in skip if k not in KEYS]
    if unknown:
        raise SystemExit("unknown --skip keys: " + ", ".join(unknown))
    return [c for c in CHANGES if c["key"] not in skip and (c["group"] == "apply" or with_review)]


def ensure_log(db):
    db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (tbl TEXT NOT NULL, rowid_ INTEGER NOT NULL, field TEXT NOT NULL, "
               "key TEXT NOT NULL, old TEXT, new TEXT, at TEXT NOT NULL, PRIMARY KEY (tbl, rowid_, field))")


def apply(db, changes):
    ensure_log(db)
    done = 0
    for c in changes:
        st, rid, cur, why = status(db, c)
        if st != "todo":
            continue
        tbl, pk = table_of(c)
        sets = {f: new for f, (old, new) in c["fields"].items() if cur[f] != new}
        db.execute(f"UPDATE {tbl} SET {', '.join(f + ' = ?' for f in sets)} WHERE {pk} = ?", (*sets.values(), rid))
        for f, new in sets.items():
            # a second change of the same field (death-knight-ac, then -rest) keeps the first old value
            db.execute(f"INSERT INTO {LOG} (tbl, rowid_, field, key, old, new, at) VALUES (?, ?, ?, ?, ?, ?, ?) "
                       "ON CONFLICT (tbl, rowid_, field) DO UPDATE SET new = excluded.new, key = key || '+' || excluded.key",
                       (tbl, rid, f, c["key"], json.dumps(cur[f]), json.dumps(new), now()))
        audit(db, tbl, {pk: rid}, {f: cur[f] for f in sets}, sets, c["cite"])
        done += 1
    return done


def undo(db):
    if db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (LOG,)).fetchone() is None:
        return 0, 0
    back, left = 0, 0
    rows = db.execute(f"SELECT tbl, rowid_, field, key, old, new FROM {LOG} ORDER BY at DESC, rowid DESC").fetchall()
    for tbl, rid, f, key, old, new in rows:
        pk = "spellid" if tbl == "spellpattern" else "id"
        cur = db.execute(f"SELECT {f} FROM {tbl} WHERE {pk} = ?", (rid,)).fetchone()
        if cur is None or cur[0] != json.loads(new):
            left += 1
            print(f"  {key}: {tbl} {rid} {f} changed since ({cur[0] if cur else 'gone'!r}), left")
            continue
        db.execute(f"UPDATE {tbl} SET {f} = ? WHERE {pk} = ?", (json.loads(old), rid))
        audit(db, tbl, {pk: rid}, {f: json.loads(new)}, {f: json.loads(old)}, "undo " + key)
        db.execute(f"DELETE FROM {LOG} WHERE tbl = ? AND rowid_ = ? AND field = ?", (tbl, rid, f))
        back += 1
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    ap.add_argument("--with-review", action="store_true", help="also the review group (weaker or tighter)")
    ap.add_argument("--skip", action="append", default=[], metavar="KEY", help="leave a change out (repeatable)")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, timeout=5)
    try:
        if a.undo:
            back, left = undo(db)
            db.commit()
            print(f"undo: {back} fields put back, {left} left as changed since")
            return 0
        changes = chosen(a.with_review, set(a.skip))
        counts = {}
        for c in CHANGES:
            st, rid, cur, why = status(db, c)
            on = c in changes
            tag = st if on else ("skipped" if c["key"] in a.skip else st + ", not chosen")
            counts[tag] = counts.get(tag, 0) + 1
            what = ", ".join(f"{f} {old!r} -> {new!r}" for f, (old, new) in c["fields"].items())
            print(f"  [{c['group']:6}] {tag:20} {c['key']:28} {c['name']}"
                  f"{' ' + str(c['rank']) if c['kind'] == 'cm' else ''} (#{rid}): {what}"
                  f"{'  -- ' + why if why else ''}{'  (' + c['note'] + ')' if c['note'] else ''}")
        print("summary: " + ", ".join(f"{k} {v}" for k, v in sorted(counts.items())))
        if a.apply:
            n = apply(db, changes)
            db.commit()
            print(f"applied: {n} rows changed")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
