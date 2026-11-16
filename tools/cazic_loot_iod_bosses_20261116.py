#!/usr/bin/env python3
"""Cazic Thule's loot built from the records, and the Isle of Dread's Impale and Contagion made tougher (Heirloom
petitions 2218, 2205 and 2207, Hoofbite, 2026-11-16; the owner's choices: "Build from records" and "+50% HP, +25% AC").

1. CAZIC THULE (npcloot "cazicthule", the IoD raid boss, L65, both of his rows). Petition 2218: "Cazic Thule only drops
   Bracelet of Doubt. Mob needs appropriate loot table." He rolled one table, "wiki cazicthule": one item a kill, at
   100%, of eleven at even weights (ten class items and Puppet Strings), so a kill gave one piece and one class in
   thirteen saw nothing of its own. The records (eqoa.forumotion.com t287, Ronson 2010-07, "Cazic Thule Loot Table";
   work board 630, jibrish's IoD loot research from it, t198 and Allakhazam 2003-04) give him a class item for every
   class ("of Doubt" / "of Horror", sixteen in all), six misc drops and Puppet Strings. No era source gives a rate:
   the rates are ours (board 630's proposal, with two class rolls for one):

     "boss cazicthule"                100%, 2 rolls: the class items, each class alike (weight 2 a class; the cleric's
                                      two items 1 each): his ten existing records, the enchanter's Bracelet of Horror
                                      7746, and three made here from t287's stats (below).
     "boss cazicthule misc"            50%, 1 roll: Amulet of Necropotence 7959, Bile Etched Obsidian Choker 1030,
                                      Bone-Clasped Girdle 7034, Crimson Robe 4943, Red Fire 5410, Shield of Rainbow
                                      Hues 6064 (t287's "Misc Drops" under CT), even weights.
     "boss cazicthule puppet strings"  10%, 1 roll: Puppet Strings 713085 (t287's charge items; board 630: under CT).

   "wiki cazicthule" is unbound from him (its npcloot row deleted; its loottables rows are left as they are). The
   class items are taken from that table AS IT STANDS: each of its eleven rows is the base record or that record's
   Ascended copy (itemascended), and the new tables use whichever it holds, so work board 1536 (jibrish, in progress:
   Ascended copies 981200-981210 in place of the eleven, Heirloom) carries over when it is applied first. Applied
   after this tool, 1536 would change an unbound table: apply this one after it, or re-point 1536 at the new tables.

   Made here (t287's figures; model, icon, slot, worth, rent and level from the sibling board 630 names; trade and
   lore as his ten wiki records, 0):
     713099 Helm of Doubt (Paladin, 4): AC 124 CHA 20 INT -50 HP 80, as 6228 Helm of Disbelief's plate helm.
     713100 Boots of Doubt (Wizard, 8192; the ranger's is 713055): AC 31 DEX 50 CHA -50 HP 60 POW 80 PR 30, as 7407
            Boots of Loathing's cloth boots.
     713101 DawnFire, Morning Star of Light (Cleric, 512), the CT version: WIS 20 CHA 10 POW 125, else as 5385 (t287
            lists DawnFire twice, WIS 20 CHA 10 and WIS 10 CHA 20; 5385 is the second, Collector Stacy's).
   Left out, no era source printing them (board 630 question 2): the Necromancer's Gloves of Horror and the
   Shadowknight's Breastplate of Doubt (960222 "Breastplate of Doubt" is Rallos Zek's mage piece, t287, not his).
   Not changed here (board 630's record fixes, jibrish's open questions): the ten wiki records' stats (t287's differ in
   every one), 7746's STR +63 for t287's -50 and its all-class mask (t287: Enchanter), the misc records' stats.

   Cazic Thule is a raid boss already (his template's hp_pct 8050), so the "boss " names change no raid rule.

2. IMPALE AND CONTAGION (petitions 2205 and 2207; classtemplates "iod impale" and "iod contagion", which npctemplate
   gives the names "impale" and "contagion" alone: each shard's two of a name, the island's L55 and the trial's L63):
     iod impale:    hp_pct 655 -> 983 (+50%).
     iod contagion: hp_pct 819 -> 1229 (+50%); taken_pct 100 -> 80 (+25% AC).
   A creature has no AC of its own on this server: a player's melee roll on it loses its level's defence
   (entity/offence.go targetDefenceAt), and a template's only armour is taken_pct, the share of a player's or a pet's
   melee hit that lands (melee only, as AC). A quarter more AC is read as 1/1.25 = 80% of a hit landing. (The roll's
   own lines with a quarter more of the level's defence give 69-86% at L55 for a L60 tank, melee, priest or caster
   with typical weapons, the floor taking over at L63: 80% is that spread's middle.)

Report only by default; --apply in one transaction with a gm_audit line a row and the log cazic_loot_iod_1116_log
(every row before and after); a second --apply does nothing; --undo puts back exactly what it wrote (a row changed
since is left and said; a made record still held by anyone is left) and drops the log. Refused whole if a row is not
as this tool expects (a template's figures, the wiki table's eleven, a made id or table taken).

WHEN IT TAKES EFFECT. Loot: at once (loottables, npcloot and itempattern are read at each corpse's first opening).
The templates: the GM panel's reload_npcs (it reads the class templates first) or a restart; a fight under way keeps
its hit points' share.

    python3 -E -s tools/cazic_loot_iod_bosses_20261116.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "cazic_loot_iod_1116_log"
WHO, VIA = "dab", "tool"
SRC = "cazicloot1116"
DETAIL = ("Cazic Thule's loot from the records (petition 2218), Impale +50% HP and Contagion +50% HP, +25% AC "
          "(petitions 2205, 2207; the owner 2026-11-16): tools/cazic_loot_iod_bosses_20261116.py")

NPC = "cazicthule"
WIKI = "wiki cazicthule"
T_CLASS, T_MISC, T_PUPPET = "boss cazicthule", "boss cazicthule misc", "boss cazicthule puppet strings"

# his wiki table's eleven: base record -> its class (and its weight in the class table; None: Puppet Strings)
WIKI_BASES = {
    713064: ("Warrior", 2), 713055: ("Ranger", 2), 713082: ("Monk", 2), 713046: ("Bard", 2), 713056: ("Rogue", 2),
    713057: ("Druid", 2), 713081: ("Shaman", 2), 713065: ("Cleric", 1), 713097: ("Magician", 2), 713047: ("Alchemist", 2),
    713085: ("Puppet Strings", None),
}
PUPPET = 713085
OTHER_CLASS = {7746: ("Enchanter", 2)}  # Bracelet of Horror, an existing record
MISC = [7959, 1030, 7034, 4943, 5410, 6064]
STATS = ("str", "sta", "agi", "wis", "dex", "cha", "intelligence", "HPMAX", "POWMAX", "PoT", "HoT", "AC", "PR", "DR",
         "FR", "CR", "LR", "AR")
# made records: id -> (sibling, class, weight, fields over the sibling's, its stats (the rest 0))
MADE = {
    713099: (6228, "Paladin", 2,
             {"itemname": "Helm of Doubt", "classuse": 4, "trade": 0, "lore": 0,
              "itemdesc": "A magical plate helm. It is steeped in doubt. It carries a potent enchantment of health."},
             {"AC": 124, "cha": 20, "intelligence": -50, "HPMAX": 80}),
    713100: (7407, "Wizard", 2,
             {"itemname": "Boots of Doubt", "classuse": 8192, "trade": 0, "lore": 0,
              "itemdesc": "These cloth boots are superbly crafted. They are steeped in doubt. They hold a potent power "
                          "enchantment."},
             {"AC": 31, "dex": 50, "cha": -50, "HPMAX": 60, "POWMAX": 80, "PR": 30}),
    713101: (5385, "Cleric", 1,
             {"classuse": 512, "trade": 0, "lore": 0},
             {"wis": 20, "cha": 10, "POWMAX": 125}),
}
TEMPLATES = {  # name: {column: (was, now)}
    "iod impale": {"hp_pct": (655, 983)},
    "iod contagion": {"hp_pct": (819, 1229), "taken_pct": (100, 80)},
}
TEMPLATE_NOTE = {
    "iod impale": " (2026-11-16, petition 2205, the owner's +50% HP: hp_pct 655 -> 983; "
                  "tools/cazic_loot_iod_bosses_20261116.py)",
    "iod contagion": " (2026-11-16, petition 2207, the owner's +50% HP, +25% AC: hp_pct 819 -> 1229, taken_pct 100 -> 80, "
                     "a quarter more AC read as 80% of a melee hit landing; tools/cazic_loot_iod_bosses_20261116.py)",
}
BINDINGS = [  # (table, chance, rolls, note)
    (T_CLASS, 100, 2, "petition 2218 (Hoofbite), the owner's 'Build from records': t287's class items, two a kill "
                      "(rate ours, board 630)"),
    (T_MISC, 50, 1, "petition 2218: t287's misc drops under CT (rate ours, board 630)"),
    (T_PUPPET, 10, 1, "petition 2218: Puppet Strings, t287's charge item (rate ours, board 630)"),
]


class Refused(Exception):
    pass


def has(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def row(db, tbl, key):
    where = " AND ".join(f'"{k}" = ?' for k in key)
    cur = db.execute(f'SELECT * FROM "{tbl}" WHERE {where}', list(key.values()))
    r = cur.fetchone()
    return dict(zip([d[0] for d in cur.description], r)) if r else None


def js(v):
    return json.dumps(v, separators=(",", ":"), sort_keys=True) if v is not None else None


def wiki_choices(db):
    """His wiki table's rows as (row id, patternid, base), or Refused."""
    rows = db.execute("SELECT id, patternid FROM loottables WHERE tablename = ? ORDER BY id", (WIKI,)).fetchall()
    asc = dict(db.execute("SELECT patternid, base FROM itemascended").fetchall()) if has(db, "itemascended") else {}
    out = []
    for rid, pid in rows:
        base = pid if pid in WIKI_BASES else asc.get(pid)
        if base not in WIKI_BASES:
            raise Refused(f"loottables {WIKI!r} row {rid}: patternid {pid} is none of the eleven (nor an Ascended copy)")
        out.append((rid, pid, base))
    bases = sorted(b for _, _, b in out)
    if bases != sorted(WIKI_BASES):
        raise Refused(f"loottables {WIKI!r} holds {len(out)} rows for bases {bases}, want the eleven {sorted(WIKI_BASES)}")
    return out


def check(db):
    """What is wrong with the database for this tool, [] for nothing; and the wiki table's choices."""
    bad, wiki = [], []
    try:
        wiki = wiki_choices(db)
    except Refused as e:
        bad.append(str(e))
    b = row(db, "npcloot", {"npc_name": NPC, "tablename": WIKI})
    if b is None or (b["chance"], b["rolls"]) != (100, 1):
        bad.append(f"npcloot {NPC}/{WIKI}: {b}, want chance 100, 1 roll")
    for t, _, _, _ in BINDINGS:
        if db.execute("SELECT 1 FROM loottables WHERE tablename = ?", (t,)).fetchone():
            bad.append(f"loottables {t!r} has rows already")
        if db.execute("SELECT 1 FROM npcloot WHERE tablename = ?", (t,)).fetchone():
            bad.append(f"npcloot binds {t!r} already")
    for pid in list(OTHER_CLASS) + MISC:
        if row(db, "itempattern", {"patternid": pid}) is None:
            bad.append(f"no itempattern {pid}")
    for pid, (sib, _, _, _, _) in MADE.items():
        if row(db, "itempattern", {"patternid": pid}):
            bad.append(f"itempattern {pid} is taken")
        if row(db, "itempattern", {"patternid": sib}) is None:
            bad.append(f"no itempattern {sib} (the sibling of {pid})")
    for name, cols in TEMPLATES.items():
        t = row(db, "classtemplates", {"name": name})
        if t is None:
            bad.append(f"no classtemplates {name!r}")
            continue
        for c, (was, _) in cols.items():
            if t[c] != was:
                bad.append(f"classtemplates {name!r} {c} {t[c]}, want {was}")
    return bad, wiki


def level_hp(db, level):
    """An NPC's hit points at its level under the balance before its template (entity/balance.go NPCMaxHP)."""
    base = max(level * 60, 10)
    rows = db.execute("SELECT level, hp_pct FROM balancelevel ORDER BY level").fetchall() if has(db, "balancelevel") else []
    p = 100.0
    if rows:
        below = [r for r in rows if r[0] <= level]
        above = [r for r in rows if r[0] >= level]
        if not above:
            p = rows[-1][1]
        elif not below:
            p = rows[0][1]
        else:
            (a, pa), (c, pc) = below[-1], above[0]
            p = pa if a == c else pa + (pc - pa) * (level - a) / (c - a)
    return base if p == 100 else max(round(base * p / 100), 1)


def report(db):
    b = row(db, "npcloot", {"npc_name": NPC, "tablename": WIKI})
    print(f"  {NPC}: {WIKI} {b and (b['chance'], b['rolls'])}")
    for rid, pid, wbase in db.execute(
            "SELECT t.id, t.patternid, p.itemname FROM loottables t LEFT JOIN itempattern p USING (patternid) "
            "WHERE t.tablename = ? ORDER BY t.id", (WIKI,)).fetchall():
        print(f"    {rid}: {pid} {wbase}")
    for name, cols in TEMPLATES.items():
        t = row(db, "classtemplates", {"name": name})
        npc = name.removeprefix("iod ")
        print(f"  {name}: " + (", ".join(f"{c} {t[c]}" for c in cols) if t else "none"))
        if t:
            for (lvl,) in db.execute("SELECT DISTINCT npc_level FROM npcs WHERE lower(trim(npc_name)) = ? ORDER BY 1", (npc,)):
                hp = level_hp(db, lvl)
                now = max(1, round(hp * t["hp_pct"] / 100))
                new = max(1, round(hp * cols["hp_pct"][1] / 100)) if t["hp_pct"] == cols["hp_pct"][0] else now
                taken = cols.get("taken_pct", (t["taken_pct"], t["taken_pct"]))[1]
                print(f"    {npc} L{lvl}: {now:,} HP, taken_pct {t['taken_pct']} -> {new:,} HP, taken_pct {taken}")


def plan(db, wiki):
    """The three tables' rows: [(table, patternid, weight, what)]."""
    rows = []
    for _, pid, base in wiki:
        cls, w = WIKI_BASES[base]
        if w is not None:
            rows.append((T_CLASS, pid, w, cls))
    for pid, (cls, w) in OTHER_CLASS.items():
        rows.append((T_CLASS, pid, w, cls))
    for pid, (_, cls, w, _, _) in MADE.items():
        rows.append((T_CLASS, pid, w, cls))
    rows += [(T_MISC, pid, 1, "misc") for pid in MISC]
    rows += [(T_PUPPET, pid, 1, "charge item") for _, pid, base in wiki if base == PUPPET]
    return rows


def print_plan(db, rows):
    for t, chance, rolls, _ in BINDINGS:
        mine = [r for r in rows if r[0] == t]
        total = sum(r[2] for r in mine)
        print(f"  {t}: {chance}% x {rolls}")
        for _, pid, w, what in mine:
            p = row(db, "itempattern", {"patternid": pid})
            name = p["itemname"] if p else MADE[pid][3].get("itemname") or row(db, "itempattern", {"patternid": MADE[pid][0]})["itemname"]
            made = " (made here)" if pid in MADE and not p else ""
            each = chance / 100 * w / total
            kill = 1 - (1 - each) ** rolls
            print(f"    {pid} {name}{made} [{what}] weight {w}: {kill * 100:.1f}% a kill")


def apply(db, wiki):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    db.execute(f"CREATE TABLE {LOG} (seq INTEGER PRIMARY KEY, tbl TEXT NOT NULL, rowkey TEXT NOT NULL, before TEXT, after TEXT)")

    def done(tbl, key, before):
        after = row(db, tbl, key)
        action = "insert" if before is None else "delete" if after is None else "update"
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES (?, ?, ?, ?)", (tbl, js(key), js(before), js(after)))
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                   "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                   (at, acct[0] if acct else 0, WHO, VIA, action, tbl, js(key), js(before), js(after), DETAIL))

    for pid, (sib, _, _, fields, stats) in MADE.items():
        r = dict(row(db, "itempattern", {"patternid": sib}), patternid=pid, **fields)
        for c in STATS:
            r[c] = stats.get(c, 0)
        db.execute(f'INSERT INTO itempattern ({", ".join(chr(34) + c + chr(34) for c in r)}) VALUES ({", ".join("?" * len(r))})',
                   list(r.values()))
        done("itempattern", {"patternid": pid}, None)

    for t, pid, w, _ in plan(db, wiki):
        cur = db.execute("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) "
                         "VALUES (?, ?, ?, 1, 100, ?)", (t, pid, w, SRC))
        done("loottables", {"id": cur.lastrowid}, None)
    for t, chance, rolls, note in BINDINGS:
        db.execute("INSERT INTO npcloot (npc_name, tablename, chance, rolls, note, source) VALUES (?, ?, ?, ?, ?, ?)",
                   (NPC, t, chance, rolls, note, SRC))
        done("npcloot", {"npc_name": NPC, "tablename": t}, None)
    key = {"npc_name": NPC, "tablename": WIKI}
    before = row(db, "npcloot", key)
    db.execute("DELETE FROM npcloot WHERE npc_name = ? AND tablename = ?", (NPC, WIKI))
    done("npcloot", key, before)

    for name, cols in TEMPLATES.items():
        key = {"name": name}
        before = row(db, "classtemplates", key)
        sets = ", ".join(f"{c} = ?" for c in cols)
        db.execute(f"UPDATE classtemplates SET {sets}, note = note || ? WHERE name = ?",
                   [now for _, now in cols.values()] + [TEMPLATE_NOTE[name], name])
        done("classtemplates", key, before)


def undo(db):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    n, left = 0, 0
    for tbl, rowkey, before, after in db.execute(f"SELECT tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(rowkey)
        now = row(db, tbl, key)
        if js(now) != after:
            print(f"  {tbl} {rowkey}: changed since this tool wrote it, left as it is")
            left += 1
            continue
        where = " AND ".join(f'"{k}" = ?' for k in key)
        if before is None:
            if tbl == "itempattern":
                held = db.execute("SELECT COUNT(*) FROM charinventory WHERE patternid = ?", (key["patternid"],)).fetchone()[0] \
                    if has(db, "charinventory") else 0
                if held:
                    print(f"  itempattern {key['patternid']}: held by {held}, left")
                    left += 1
                    continue
            db.execute(f'DELETE FROM "{tbl}" WHERE {where}', list(key.values()))
        elif after is None:
            b = json.loads(before)
            db.execute(f'INSERT INTO "{tbl}" ({", ".join(chr(34) + c + chr(34) for c in b)}) VALUES ({", ".join("?" * len(b))})',
                       list(b.values()))
        else:
            b = json.loads(before)
            db.execute(f'UPDATE "{tbl}" SET {", ".join(chr(34) + c + chr(34) + " = ?" for c in b)} WHERE {where}',
                       list(b.values()) + list(key.values()))
        n += 1
    db.execute(f"DROP TABLE {LOG}")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', ?, '{}', NULL, NULL, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, LOG, f"undone, {n} rows put back, {left} left: " + DETAIL))
    print(f"  {n} rows put back, {left} left")


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        report(db)
        if a.undo:
            if not has(db, LOG):
                print("nothing to undo")
                return 0
            db.execute("BEGIN IMMEDIATE")
            undo(db)
            db.execute("COMMIT")
            print("loot: at once; the templates: at the GM panel's reload_npcs or a restart")
            return 0
        if has(db, LOG):
            print(f"applied already ({LOG} is there): nothing to do (--undo to take it back)")
            return 0
        bad, wiki = check(db)
        if bad:
            print("refused, nothing done:\n  " + "\n  ".join(bad))
            return 1
        print_plan(db, plan(db, wiki))
        if not a.apply:
            print("report only: --apply to write it")
            return 0
        db.execute("BEGIN IMMEDIATE")
        apply(db, wiki)
        db.execute("COMMIT")
        print("applied; loot at once (each corpse's first opening); the templates at the GM panel's reload_npcs or a restart")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
