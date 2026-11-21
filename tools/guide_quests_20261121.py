#!/usr/bin/env python3
"""Quest fixes from the quest audit against MasterVG782's GameFAQs guide (the owner, 2026-11-21: four groups of
fixes approved from the audit's report). Shard-aware: each fix is planned from the database given
and only where its problem is there; nothing is read from the other shard at run time (what it has is written below).

1. UNCOMPLETABLE: NAMES, SPAWNS, SPELLS
   a. NPC rows whose name a quest step or giver does not match (Live). A talk, show or give step, and a quest's giver,
      match an NPC by its exact name, any case (handlers/quests.go stepNPC; the giver index foldName(qu.Giver)):
      queststeptargets (QuestStep.Also) counts only for kill and loot steps (database.Hunts). So for a giver or a talk/
      give/spawn name the NPC row is renamed to the spelling the quest uses, which is Heirloom's name for the same row
      (same id, same spot): Rigo Darter -> Rigo Dartar (100782), Jeboban -> Jebodan (101050), Waitress Jitzle ->
      Waitress Jitzie (101221), and the other Live renames the server's checker found (Keeper Toshei, Alchemist
      Ambrakma, Covelbun Andelfoof, Provisioner Oomgig, Selathan Weirdling, Seamstress Wyndham, Domik and Yilinyaka
      Novandear (spawn events' names), The Watchman (row 118868, which the Trial of Contention spawns and quest 101312
      step 7 kills)). Every name-keyed table (npcclass, npcfaction, lootcreature, factionhits, lootpool, npctemplate)
      already has the quest's spelling on Live as on Heirloom, so the row alone changes. A row is renamed only if it
      still has the old name at the spot written here and no NPC of the new name stands.
   b. Kill/loot steps whose creature is spelt otherwise: a queststeptargets row (the alias mechanism): 101227 step 1
      "Arctic Spiritcaller" + "Artic Spiritcaller" (Live has no Arctic row; Heirloom's own 122923 is left), 101294 step
      5 "Rawgl the Carnage Hound" + "Rawlg the Carnage Hound" (Live's row 122067; Heirloom's 101294 has no such step).
   c. Trigger spawns of "#row" keys missing from the database: "The Trial of Celerity" names Heirloom's rows
      163618-163621 (3 Bloodletters and Impale at the pool); Live has the same four at the same spots as 191269, 191538,
      191430 and 191552, so the trigger's npc keys are pointed at those (found here by name and spot). Any other missing
      "#row" a trigger or a quest event spawns is listed (none on 2026-11-21's backups).
   d. Reward spells with no spellpattern record: the Ascended spells were made per shard (spellascended), the same
      spell under different ids (Live 32 of Heirloom's ids, Heirloom 6 of Live's: the Necromancer's 101131). Copying
      the records would make a second copy of each spell (and break spellascended's unique base), so the quest step's
      reference (queststeps.spells and the "spells" of byclass/choices) is pointed at this database's own record of the
      same name (unique), instead.
   (101312 step 4, "Trial of Celerity": its signtext row is of kind "quest" (petition 2537), so the step's quest answers
   before the sign's words (handlers/signs.go); the checker's "sign post" is stale. Left as it is.)

2. UNCOMPLETABLE: HOSTILE TALK / HAND-IN TARGETS (attacked on sight: entity/aggro.go threatensLocked; no npcfaction,
   creature bodies outside a town, so hostile by nature at the quest's level)
   - Archon Seradesh (101026 s24, 101029 s20; rows 120969, 120975): talk-only in every quest, nothing kills or loots her,
     the guide has her speak (Arios 57). The NPC bit (npc_type |= 0x80: "no one may attack it, so it attacks no one",
     hostileNatureLocked) as Kithgrin and the other quest NPCs have.
   - Lord Pickclaw (101264 s6, then s7 loot him; 101315 s7 loots him), golem of strength (100919 s2 give, s3 loot it),
     Chareek the Tactician (100946 s1 talk, s2 loot him): each is spoken to and then fought ("End me? Then come!").
     An npcaggro row aggressive 0 (the GM rule by name; it overrides faction and nature): it no longer attacks on
     sight, still fights whoever attacks it, and its kills and loot stay as they are. Heirloom's Chareek copy in a
     dungeon world (row 165992, world 14) takes the rule too.

3. MONK PATH GROUP 3 AND THE RING OF VEESHAN
   - Every quest of EQOA-Data's group 3 path chain (30/40/49: 100896-100911, 16 quests) with races "0, 1, 3, 7, 8"
     gets no race mask, as its entry 100852 (monks of every race) and the class entries (100796 ...) have: the chain is
     reached only through those entries (requires), so no class outside the group gets in. (Opening only 100896, 100902
     and 100908 would still stop a Dark Elf monk at 100897.)
   - The Ring of Veeshan (713088, Armour of the Sky 101447/101453/101459/101465): nothing to write. Heirloom's
     quintessence of Veeshan drops the Ascended Ring of Veeshan (980025, ascend_boss_loot 2026-11-10, loottables 3457),
     and a give step's item is met by its Ascended record (handlers/questascended.go, petition 1446: exactly "ring of
     veeshan or ascended ring of veeshan"). The checker does not read Ascended records; the quests can be finished.

4. REWARDS TO THE GUIDE (experience; the guide names no item)
   - The Ring of Madrias 1-3 (the guide's Evil'ution, 101233-101235): 0 -> 1,500,000 / 4,000,000 / 6,500,000 XP on the
     last step. The guide's ring-summoning scrolls (levels 20 and 30) exist neither as items nor as spells here: not made.
   - Poisontongue (Rigo, 100782) 2,000,000 -> 4,100,000; Redsen's Errands (Blakedown 33, 100774) 1,300,000 -> 2,679,818;
     The Seal of Deiphobus (Highpass #6, 100776) 1,800,000 -> 3,768,632. The class gear the guide mentions is not named
     by the guide nor by EQOA-Data ("the reward is ours"): not written (see the report's proposal).

Dry run by default: every change printed. --apply writes it in one transaction, each row with a gm_audit line; a
second --apply finds nothing to do. Every row written is logged in guide_quests_1121_log (before and after); --undo puts
each back (an update's columns as they were, an insert deleted; a row changed since is left and said), each with a
gm_audit line, and drops the log. Takes effect at the next restart (NPC rows, npcaggro, quests and triggers are read at
start).

    python3 -E -s tools/guide_quests_20261121.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import re
import sqlite3
import sys

TOOL = "tools/guide_quests_20261121.py"
SRC = "guide-quests-20261121"
LOG = "guide_quests_1121_log"
ACCOUNT, WHO, VIA = 1, "dab", "tool"
EPS = 1.0

# 1a. (row, the name the quest uses, the name it has where it is wrong, x, z): Heirloom's rows of 2026-11-21
RENAMES = [
    (105170, "Rigo Dartar", "Rigo Darter", 21077, 25268, "Poisontongue (100782)"),
    (101608, "Jebodan", "Jeboban", 13548, 14730, "Jebodan's Price (101050)"),
    (113718, "Waitress Jitzie", "Waitress Jitzle", 13704, 11398, "Razzle Dazzle: Sweggy's Ring (101221)"),
    (106002, "Keeper Toshei", "Caller Toshei", 4507, 21830, "Toshel's Errands / Greyweed (100246, 100254, 100255)"),
    (106224, "Alchemist Ambrakma", "Chemist Ambrakma", 23851, 6212, "Supplies for Ambrakma (100300)"),
    (106225, "Covelbun Andelfoof", "Tovelbun Andelfoof", 23866, 6238, "Eagglefoot's Notebooks (100323)"),
    (106067, "Provisioner Oomgig", "Provivisioner Oomgig", 14768, 29287, "Stolen Supplies (100545)"),
    (105715, "Selathan Weirdling", "Stelathan Weirdling", 4239, 17068, "A Test of Strength / A New Robe / A Muddy Paw (100619-100621)"),
    (101429, "Seamstress Wyndham", "Seamster Wyndham", 9182, 12778, "Tailoring (Seamstress Wyndham)"),
    (110802, "Domik Novandear", "Domnik Novandear", 25046, 13546, "The Man by the Broken Bridge (100473): its spawn"),
    (110805, "Yilinyaka Novandear", "Yilinaka Novandear", 25046, 13540, "The Novandear Gang (100157): its spawn"),
    (118868, "The Watchman", "The Watchmen", 25603, 3096, "The Trials of Cazic Thule (101312 step 7; the Trial of Contention's spawn)"),
]
# 1b. (quest, step, the step's target, the other name to count, the quest)
ALIASES = [
    (101227, 1, "Arctic Spiritcaller", "Artic Spiritcaller", "Sundro's Enchantment (101227)"),
    (101294, 5, "Rawgl the Carnage Hound", "Rawlg the Carnage Hound", "Yarfsnoogle's Anatomy (101294)"),
]
# 1c. Heirloom's trigger rows: (row, name, world, x, z)
TRIGGER_ROWS = {
    163618: ("Bloodletter", 0, 25672, 3312), 163619: ("Bloodletter", 0, 25678, 3312),
    163620: ("Bloodletter", 0, 25684, 3314), 163621: ("Impale", 0, 25690, 3316),
}
# 1d. the reward spells a shard lacks, by the other shard's name of them (spellpattern.spellname)
SPELLS = {
    906728: "Ascended Doom Tower", 906730: "Ascended Freeport Cocktail", 906732: "Ascended Natureblade",
    906733: "Ascended Towering Mind", 906734: "Ascended Freezing Mixture", 906735: "Ascended Power Flux",
    906738: "Ascended Ahrianna's Blessed Boon", 906739: "Ascended Ahrianna's Generous Gift",
    906740: "Ascended Aktrana's Cerebral Fortress", 906741: "Ascended Aktrana's Tower of Will",
    906752: "Ascended Gelledyn's Feral Imitation", 906753: "Ascended Gelledyn's Perfect Vision",
    906754: "Ascended Glimmering Blades", 906755: "Ascended Goading Ridicule", 906756: "Ascended Graal's Final Strike",
    906757: "Ascended Graal's Venomus Strike", 906760: "Ascended Jen'Tylis' Lust for Power",
    906761: "Ascended Jen'Tylis' Overarching Greed", 906766: "Ascended Manaburn", 906772: "Ascended Ozmo's Echoing Mockery",
    906781: "Ascended Quil'Eyne's Cruel Embrace", 906782: "Ascended Quil`Eyne's Unspeakable Hunger",
    906785: "Ascended Refreshing Concoction", 906788: "Ascended Severupidus' Cocktail of Death",
    906789: "Ascended Severupidus' Cocktail of Torment", 906803: "Ascended Visage",
    906804: "Ascended Vylinnuza's Tidal Destruction", 906808: "Ascended Severupidus' Fire Potion",
    906809: "Ascended Eldrich Rune", 906810: "Ascended Infectious Aura", 906811: "Ascended Vylinnuza's Merciless Blizzard",
    906812: "Ascended Vylinnuza's Merciless Inferno",
    906608: "Ascended Blood Vortex", 906618: "Ascended Agonize Death", 906623: "Ascended Boldunder's Mountain Roots",
    906624: "Ascended Boldunder's Personal Mine", 906625: "Ascended Calidux's Equitable Division",
    906626: "Ascended Calidux's Insatiable Appetite",
}
# 2.
PEACEFUL_BIT = ["Archon Seradesh"]
NOT_ON_SIGHT = [
    ("Lord Pickclaw", "Lord Pickclaw (101264 step 6: talk, then step 7 kill)"),
    ("golem of strength", "The Test of Strength (100919 step 2: give, then step 3 kill)"),
    ("Chareek the Tactician", "The Froglok Tactician (100946 step 1: talk, then step 2 kill)"),
]
# 3.
GROUP3_SOURCE = "EQOA-Data 30,40,49/group-3-good/"
GROUP3_RACES = "0, 1, 3, 7, 8"
# 4. (quest, its name, step, experience it has, the guide's)
XP = [
    (101233, "The Ring of Madrias: Earth and Stone", 11, 0, 1500000, "Evil'ution part 1"),
    (101234, "The Ring of Madrias: The Master of the Forge", 6, 0, 4000000, "Evil'ution part 2"),
    (101235, "The Ring of Madrias: Magus Niska's Books", 7, 0, 6500000, "Evil'ution part 3"),
    (100782, "Poisontongue", 13, 2000000, 4100000, "Rigo Darter's quest"),
    (100774, "Redsen's Errands", 8, 1300000, 2679818, "Blakedown's 33 quest"),
    (100776, "The Seal of Deiphobus", 10, 1800000, 3768632, "Highpass quest #6"),
]


def detail(what):
    return f"the owner 2026-11-21: quest audit vs MasterVG782's guide: {what} ({TOOL})"


def js(v):
    return json.dumps(v, separators=(",", ":"), sort_keys=True) if v is not None else None


def row(db, tbl, key):
    where = " AND ".join(f'"{k}" = ?' for k in key)
    cur = db.execute(f'SELECT * FROM "{tbl}" WHERE {where}', list(key.values()))
    r = cur.fetchone()
    return dict(zip([d[0] for d in cur.description], r)) if r else None


def has_table(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


class Plan:
    def __init__(self):
        self.changes = []  # dict(fix, what, tbl, key, set (update) or row (insert))
        self.notes = []    # said, not written

    def update(self, fix, what, tbl, key, cols):
        self.changes.append(dict(fix=fix, what=what, tbl=tbl, key=key, set=cols))

    def insert(self, fix, what, tbl, key, r):
        self.changes.append(dict(fix=fix, what=what, tbl=tbl, key=key, row=r))


def npc_named(db, name):
    return db.execute("SELECT id FROM npcs WHERE lower(npc_name) = lower(?)", (name,)).fetchall()


def walk_spells(o, fn):
    """o with every "spells" value of a byclass/choices JSON rewritten by fn (a str -> str); whether any changed."""
    changed = [False]

    def w(x):
        if isinstance(x, dict):
            out = {}
            for k, v in x.items():
                if k == "spells" and isinstance(v, str):
                    nv = fn(v)
                    changed[0] |= nv != v
                    out[k] = nv
                else:
                    out[k] = w(v)
            return out
        if isinstance(x, list):
            return [w(v) for v in x]
        return x
    return w(o), changed[0]


def plan_all(db):
    p = Plan()
    # ---- 1a renames
    for rid, new, old, x, z, why in RENAMES:
        r = row(db, "npcs", {"id": rid})
        if r is None or r["npc_name"] == new:
            continue
        if npc_named(db, new):
            continue  # an NPC of the quest's name stands: no problem here
        if r["npc_name"] != old or abs(r["x_coord"] - x) > EPS or abs(r["z_coord"] - z) > EPS:
            p.notes.append(f"1a: row {rid} is {r['npc_name']!r} at ({r['x_coord']:.0f}, {r['z_coord']:.0f}), not {old!r} at "
                           f"({x}, {z}): left (the quest wants {new!r}: {why})")
            continue
        p.update("1a", f"rename NPC row {rid} {old!r} -> {new!r}: {why}", "npcs", {"id": rid}, {"npc_name": new})
    # ---- 1b aliases
    for q, s, target, other, why in ALIASES:
        st = row(db, "queststeps", {"questid": q, "step": s})
        if not st or st["target"] != target or st["kind"] not in ("kill", "loot"):
            continue
        if npc_named(db, target) or not npc_named(db, other):
            continue
        if row(db, "queststeptargets", {"questid": q, "step": s, "target": other}):
            continue
        p.insert("1b", f"quest {q} step {s} ({target!r}): + other creature {other!r}: {why}", "queststeptargets",
                 {"questid": q, "step": s, "target": other}, {"questid": q, "step": s, "target": other})
    # ---- 1c trigger and quest event "#row" spawns
    ids = {r[0] for r in db.execute("SELECT id FROM npcs")}
    for slot, name, steps in db.execute("SELECT slot, name, steps FROM gmtriggers").fetchall():
        try:
            st = json.loads(steps or "[]")
        except ValueError:
            continue
        changed, said = False, []
        for t in st:
            m = re.match(r"#(\d+)$", str(t.get("npc", "")).strip())
            if not m or int(m.group(1)) in ids or t.get("do") not in ("spawn", "resummon"):
                continue
            old = int(m.group(1))
            if old not in TRIGGER_ROWS:
                p.notes.append(f"1c: trigger {name!r} (slot {slot}) spawns row #{old}, which this database lacks; not one "
                               "this tool knows: left")
                continue
            nm, w, x, z = TRIGGER_ROWS[old]
            cand = db.execute("SELECT id FROM npcs WHERE npc_name = ? AND world = ? AND abs(x_coord - ?) <= ? AND "
                              "abs(z_coord - ?) <= ?", (nm, w, x, EPS, z, EPS)).fetchall()
            if len(cand) != 1:
                p.notes.append(f"1c: trigger {name!r} spawns missing #{old} ({nm} at {x}, {z}); {len(cand)} rows of that "
                               "name stand there: left")
                continue
            t["npc"] = f"#{cand[0][0]}"
            changed = True
            said.append(f"#{old} -> #{cand[0][0]} ({nm})")
        if changed:
            p.update("1c", f"trigger {name!r} (slot {slot}): spawns {', '.join(said)}", "gmtriggers", {"slot": slot},
                     {"steps": json.dumps(st, separators=(",", ":"))})
    for q, s, ev in db.execute("SELECT questid, step, events FROM queststeps WHERE events LIKE '%\"#%'").fetchall():
        for e in json.loads(ev):
            m = re.match(r"#(\d+)$", str(e.get("npc", "")).strip())
            if m and int(m.group(1)) not in ids:
                p.notes.append(f"1c: quest {q} step {s} spawns row #{m.group(1)}, which this database lacks: left")
    # ---- 1d reward spells
    have = {r[0] for r in db.execute("SELECT spellid FROM spellpattern")}
    byname = {}
    for sid, nm in db.execute("SELECT spellid, spellname FROM spellpattern"):
        byname.setdefault(nm, []).append(sid)
    repoint, unknown = {}, set()

    def fix_ids(v):
        out = []
        for tok in re.split(r"(\d+)", v):
            if tok.isdigit() and int(tok) not in have:
                sid = int(tok)
                nm = SPELLS.get(sid)
                ids_ = byname.get(nm, []) if nm else []
                if len(ids_) == 1:
                    repoint[sid] = ids_[0]
                    tok = str(ids_[0])
                else:
                    unknown.add(sid)
            out.append(tok)
        return "".join(out)
    for q, s, spells, bc, ch in db.execute("SELECT questid, step, spells, byclass, choices FROM queststeps WHERE "
                                           "spells <> '' OR byclass LIKE '%spells%' OR choices LIKE '%spells%'").fetchall():
        cols = {}
        if spells:
            ns = fix_ids(spells)
            if ns != spells:
                cols["spells"] = ns
        for c, blob in (("byclass", bc), ("choices", ch)):
            if blob and "spells" in blob:
                o, changed = walk_spells(json.loads(blob), fix_ids)
                if changed:
                    cols[c] = json.dumps(o, separators=(",", ":"), ensure_ascii=False)
        if cols:
            qn = db.execute("SELECT name FROM quests WHERE questid = ?", (q,)).fetchone()
            p.update("1d", f"quest {q} {qn[0] if qn else ''!r} step {s}: reward spells pointed at this database's own records",
                     "queststeps", {"questid": q, "step": s}, cols)
    if repoint:
        p.notes.append("1d: spells " + ", ".join(f"{a}->{b} {SPELLS[a]!r}" for a, b in sorted(repoint.items())))
    for sid in sorted(unknown):
        p.notes.append(f"1d: reward spell {sid} has no record here and no single record of its name: left")
    # 101312 step 4 (no change): the sign answers with its quest first when its signtext row is of kind quest
    st = row(db, "queststeps", {"questid": 101312, "step": 4})
    if st and has_table(db, "signtext"):
        sg = row(db, "signtext", {"npcid": st["npcid"]}) if st["npcid"] else None
        if not sg or sg["kind"] != "quest":
            p.notes.append(f"1: 101312 step 4's sign (row {st['npcid']}) has no signtext row of kind 'quest': the step "
                           "cannot be talked to (not changed here)")
    # ---- 2 hostile talk targets
    for nm in PEACEFUL_BIT:
        for rid, t in db.execute("SELECT id, npc_type FROM npcs WHERE npc_name = ?", (nm,)).fetchall():
            if (t or 0) & 0x80:
                continue
            p.update("2", f"NPC row {rid} {nm!r}: npc_type {t} -> {(t or 0) | 0x80} (the NPC bit: talked to, never attacked, "
                     "attacks no one)", "npcs", {"id": rid}, {"npc_type": (t or 0) | 0x80})
    for nm, why in NOT_ON_SIGHT:
        if not npc_named(db, nm):
            continue
        if db.execute("SELECT 1 FROM npcaggro WHERE lower(npc_name) = lower(?)", (nm,)).fetchone():
            continue
        p.insert("2", f"npcaggro {nm!r}: aggressive 0 (no attack on sight; fights when attacked): {why}", "npcaggro",
                 {"npc_name": nm},
                 {"npc_name": nm, "aggressive": 0, "radius": 0, "leash": 0, "holding": 0, "see_invis": 0, "holds": 0,
                  "note": f"spoken to before its fight: {why}; MasterVG782's guide quest audit (the owner 2026-11-21)",
                  "source": SRC})
    # ---- 3 monk path group 3
    for q, nm, races in db.execute("SELECT questid, name, races FROM quests WHERE source LIKE ? ORDER BY questid",
                                   (GROUP3_SOURCE + "%",)).fetchall():
        if races.strip() == GROUP3_RACES:
            p.update("3", f"quest {q} {nm!r}: races {races!r} -> '' (every monk's race, as its entries)", "quests",
                     {"questid": q}, {"races": ""})
    # ---- 4 experience
    for q, nm, s, old, new, guide in XP:
        qq = row(db, "quests", {"questid": q})
        st = row(db, "queststeps", {"questid": q, "step": s})
        if not qq or not st or qq["name"] != nm:
            p.notes.append(f"4: quest {q} {nm!r} step {s} not found as written: left")
            continue
        if st["xp"] == new:
            continue
        if st["xp"] != old:
            p.notes.append(f"4: quest {q} step {s} gives {st['xp']} XP, not {old}: left (the guide: {new})")
            continue
        p.update("4", f"quest {q} {nm!r} step {s}: XP {old:,} -> {new:,} (the guide's {guide})", "queststeps",
                 {"questid": q, "step": s}, {"xp": new})
    return p


def report(db, p):
    fixes = {"1a": "1a NPC names the quests use", "1b": "1b kill/loot step aliases", "1c": "1c trigger spawn rows",
             "1d": "1d reward spells", "2": "2 hostile talk/hand-in targets", "3": "3 monk path group 3",
             "4": "4 rewards to the guide"}
    for f, title in fixes.items():
        ch = [c for c in p.changes if c["fix"] == f]
        print(f"{title}: {len(ch)} change(s)")
        for c in ch:
            print(f"  {c['what']}")
            if "set" in c:
                now = row(db, c["tbl"], c["key"])
                for k, v in c["set"].items():
                    b = now.get(k) if now else None
                    if isinstance(v, str) and len(v) > 160:
                        print(f"    {c['tbl']} {js(c['key'])} {k}: ({len(str(b))} chars) -> ({len(v)} chars)")
                    else:
                        print(f"    {c['tbl']} {js(c['key'])} {k}: {b!r} -> {v!r}")
            else:
                print(f"    {c['tbl']} + {js(c['row'])}")
    if p.notes:
        print("notes (not written):")
        for n in p.notes:
            print(f"  {n}")


def apply(db, p):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY, tbl TEXT NOT NULL, rowkey TEXT NOT NULL, "
               "before TEXT, after TEXT, fix TEXT NOT NULL, at TEXT NOT NULL)")
    n = 0
    for c in p.changes:
        before = row(db, c["tbl"], c["key"])
        if "set" in c:
            if before is None:
                raise SystemExit(f"{c['tbl']} {js(c['key'])} is gone")
            sets = ", ".join(f'"{k}" = ?' for k in c["set"])
            where = " AND ".join(f'"{k}" = ?' for k in c["key"])
            db.execute(f'UPDATE "{c["tbl"]}" SET {sets} WHERE {where}', list(c["set"].values()) + list(c["key"].values()))
            action = "update"
        else:
            if before is not None:
                raise SystemExit(f"{c['tbl']} {js(c['key'])} exists")
            r = c["row"]
            db.execute(f'INSERT INTO "{c["tbl"]}" ({", ".join(chr(34) + k + chr(34) for k in r)}) '
                       f'VALUES ({", ".join("?" * len(r))})', list(r.values()))
            action = "insert"
        after = row(db, c["tbl"], c["key"])
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, fix, at) VALUES (?, ?, ?, ?, ?, ?)",
                   (c["tbl"], js(c["key"]), js(before), js(after), c["fix"], at))
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                   "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                   (at, ACCOUNT, WHO, VIA, action, c["tbl"], js(c["key"]), js(before), js(after), detail(c["what"])))
        n += 1
    return n


def undo(db):
    """Puts back every row the log says was written, newest first (an update's row as it was, an insert deleted), each
    with a gm_audit line; a row changed since is left and said; then drops the log."""
    if not has_table(db, LOG):
        print("nothing to undo: no log")
        return 0
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    n = left = 0
    db.execute("BEGIN IMMEDIATE")
    try:
        for tbl, key, before, after in db.execute(f"SELECT tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
            k = json.loads(key)
            now = row(db, tbl, k)
            if js(now) != after:
                print(f"  left (changed since): {tbl} {key}")
                left += 1
                continue
            where = " AND ".join(f'"{c}" = ?' for c in k)
            if before is None:
                db.execute(f'DELETE FROM "{tbl}" WHERE {where}', list(k.values()))
                action = "delete"
            else:
                b = json.loads(before)
                cols = [c for c in b if c not in k]
                db.execute(f'UPDATE "{tbl}" SET {", ".join(chr(34) + c + chr(34) + " = ?" for c in cols)} WHERE {where}',
                           [b[c] for c in cols] + list(k.values()))
                action = "update"
            db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                       "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                       (at, ACCOUNT, WHO, VIA, action, tbl, key, after, before, "undone: " + detail("all four groups")))
            n += 1
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"undone: {n} rows put back, {left} left")
    return 0


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true", help="put back exactly the rows the log says were written")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db)
    db.execute("PRAGMA busy_timeout = 5000")
    db.isolation_level = None
    if a.undo:
        return undo(db)
    p = plan_all(db)
    report(db, p)
    if not a.apply:
        print("\ndry run: nothing written (--apply to write)")
        return 0
    if not p.changes:
        print("\nnothing to write: all in place")
        return 0
    try:
        db.execute("BEGIN IMMEDIATE")
        n = apply(db, p)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"\napplied: {n} rows, {n} gm_audit lines (log {LOG})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
