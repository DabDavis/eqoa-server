#!/usr/bin/env python3
"""npcclass's Return Home rows given the class their names say (F3
20261026-042344, jib: "Lord Nagafen should quad, he currently single hits";
the user: "at level 35 and above melee/war classes quad attack, and many
pets/mobs are missing that").

A creature's melee round is CLIENT's GetSwing for its npcclass class
(entity/rounds.go): a warrior's four hits from level 35, a caster's always
one. tools/import_npc_classes.py filled the table from two sources:
Sandstorm's entity records (byte 145, what its creatures really were: a
nasehir ranger 1, a nasehir cleric 9, a deathfist mystic 8, a crag wisp 10)
and Return Home's EQOA_Master.sql, whose npc_type column holds 0-14 like a
class but is not one for its creatures: Lord Nagafen, a crag spider, a kappa
warrior and an iron knight are magicians there, a large beetle, a green mamba
and an orc hunter necromancers, a mound of slime and reanimated armor
wizards, and 325 names, mostly merchants and craftsmen, bards. So every
caster-classed creature of Return Home's single-hit at any level, Nagafen
among them.

Each Return Home row takes the class its NAME says, else a warrior's:

- a caster's word, whole (CASTER_WORDS below: shaman, mystic, seer, oracle,
  augur, witch doctor; cleric, priest, priestess, bishop, acolyte, healer,
  chaplain; druid; wizard, sorcerer, sorceress, magus, mage, arcanist,
  evoker, sandcaster; magician, conjurer, summoner, elementalist, and the
  wisps and elementalkin Sandstorm's records class as magicians;
  necromancer, lich, warlock, witch, defiler; enchanter, illusionist;
  alchemist, chemist) or a paladin's or shadow knight's (paladin;
  shadowknight, deathknight), the one nearest the name's end deciding
  ("goblin witch doctor" a shaman, "arch bishop vel" a cleric); not when the
  name's head is a creature or a fighter the word only describes ("druidic
  cadaver", a mage hunter: NOT_CASTER_HEADS);
- anything else a warrior: a bard's, a ranger's, a rogue's or a monk's round
  is a warrior's at every level that matters here, and merchants and
  townsfolk, whose class only shows when they are attacked, fight as
  warriors as Sandstorm's do (641 of its 833 names).

Sandstorm's rows and any row a GM changed (gm_audit names it) are left as
they are; a name the table lacks whose words make it a caster gets a row
(the warrior's default would quad it). Each row written is source "by name"
and logged in npcclass_fix_log with what it held before; --undo puts back
every row still as this tool left it and says which a GM has changed since.
Both take a copy of the database first (<db>.bak-<when>-pre-classfix).
Report only by default. tools/import_npc_classes.py --apply replaces the
whole table: run this again after it. Restart the server after --apply or --undo, or edit
any npcclass row in the GM panel, for the running world to read them.

    tools/fix_npc_classes.py [--db eqoa.db] [--apply | --undo] [--list]
"""
import argparse
import collections
import datetime
import json
import os
import re
import sqlite3

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
CLASSES = "WAR RAN PAL SK MNK BRD RGE DRD SHA CL MAG NEC ENC WIZ ALC".split()
SOURCE = "by name"
RETURN_HOME = "return home"

# The words that make a name a class's, whole, by class (WAR=0 ... ALC=14).
CASTER_WORDS = {
    8: r"witch\s+doctors? witchdoctors? shamans? mystics? seers? oracles? augurs? soothsayers? spiritists? hexers?",
    9: r"clerics? priests? priestess(?:es)? bishops? archbishops? acolytes? healers? chaplains? deacons? curates? "
       r"hierophants? confessors?",
    7: r"druids? druidic",
    13: r"wizards? sorcerers? sorceress(?:es)? magus magi mages? archmages? arcanists? evokers? pyromancers? "
        r"sandcasters? spellcasters? stormcallers? thaumaturges?",
    10: r"magicians? conjurers? conjurors? summoners? elementalists? elementalkin wisps?",
    11: r"necromancers? liches? lich warlocks? witch(?:es)? deathcallers? defilers? diabolists? occultists?",
    12: r"enchanters? enchantress(?:es)? illusionists? mesmerists? beguilers?",
    14: r"alchemists? chemists? apothecar(?:y|ies)",
    2: r"paladins?",
    3: r"shadowknights? shadow\s+knights? deathknights? death\s+knights?",
}
WORD = {c: re.compile(r"\b(?:" + "|".join(ws.split()) + r")\b") for c, ws in CASTER_WORDS.items()}

# A name whose head is one of these is not a caster for a word before it.
NOT_CASTER_HEADS = set("""
hunter hunters slayer slayers killer killers guard guards guardian guardians servant servants slave slaves pet pets
familiar familiars minion minions protector protectors bodyguard bodyguards warrior warriors knight knights soldier
soldiers champion champions defender defenders sentry sentries sentinel sentinels golem golems construct constructs
hound hounds wolf wolves dog dogs steed steeds mount cadaver cadavers corpse corpses zombie zombies skeleton skeletons
spider spiders snake snakes bat bats rat rats beetle beetles bear bears drake drakes
""".split())


def key(name):
    """database.NPCClassKey: trimmed, any case, no trailing '$'."""
    n = (name or "").strip().lower()
    if n.endswith("$"):
        n = n[:-1]
    return n.strip()


def head(name):
    """The name's head noun: its last word, or the last before "of"
    ("magus of the stars": magus), a possessive's mark dropped."""
    n = re.sub(r"'s\b", "", name)
    if re.search(r"\bof\b", n):
        n = re.split(r"\bof\b", n)[0]
    w = re.findall(r"[a-z0-9'`]+", n)
    return w[-1] if w else ""


def class_by_name(name):
    """(class, why): the class the name's words say, and the word; a
    warrior's (0, None) when none does."""
    n = key(name)
    hits = [(m.end(), m.end() - m.start(), c, m.group(0)) for c, p in WORD.items() for m in p.finditer(n)]
    if not hits:
        return 0, None
    hits.sort()
    end, _, c, word = hits[-1]
    h = head(n)
    if h in NOT_CASTER_HEADS and not WORD[c].fullmatch(h):
        return 0, None
    return c, word


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-classfix"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


LOG = """CREATE TABLE IF NOT EXISTS npcclass_fix_log (
    name       TEXT PRIMARY KEY,
    old_class  INTEGER,            -- NULL: the tool made the row
    old_source TEXT,
    new_class  INTEGER NOT NULL,
    new_source TEXT NOT NULL,
    word       TEXT NOT NULL DEFAULT '',
    at         TEXT NOT NULL
)"""


def gm_names(db):
    """The npcclass names a GM has written through the panel (gm_audit's
    rowkey is the key as JSON, {"name": ...})."""
    out = set()
    try:
        rows = db.execute("SELECT rowkey FROM gm_audit WHERE tbl = 'npcclass'").fetchall()
    except sqlite3.OperationalError:
        return out
    for (rk,) in rows:
        try:
            out.add(key(json.loads(rk).get("name", "")))
        except (ValueError, AttributeError):
            continue
    return out


def plan(db):
    """The rows to write: name -> (old class or None, old source, new class,
    word), and the counts by (old, new)."""
    gm = gm_names(db)
    rows = {n: (c, s) for n, c, s in db.execute("SELECT name, class, source FROM npcclass")}
    spawns = collections.Counter(key(n) for (n,) in db.execute("SELECT npc_name FROM npcs"))
    spawns.pop("", None)
    out = {}
    for n, (c, s) in rows.items():
        if s != RETURN_HOME or n in gm:
            continue
        new, word = class_by_name(n)
        if new != c:
            out[n] = (c, s, new, word or "")
    for n in spawns:
        if n in rows or n in gm:
            continue
        new, word = class_by_name(n)
        if new != 0:
            out[n] = (None, None, new, word)
    return out, spawns


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--apply", action="store_true", help="write the rows")
    ap.add_argument("--undo", action="store_true", help="put back every row this tool wrote")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    ap.add_argument("--list", action="store_true", help="list every name it changes")
    a = ap.parse_args()
    db = sqlite3.connect(a.db)

    if a.undo:
        if not a.no_backup:
            print("copy:", backup(db, a.db))
        try:
            logged = db.execute("SELECT name, old_class, old_source, new_class, new_source FROM npcclass_fix_log").fetchall()
        except sqlite3.OperationalError:
            logged = []
        back = changed = 0
        for name, oc, os_, nc, ns in logged:
            now = db.execute("SELECT class, source FROM npcclass WHERE name = ?", (name,)).fetchone()
            if now != (nc, ns):
                print(f"  left: {name!r} is {now} since (a GM's change?), not this tool's ({nc}, {ns!r})")
                changed += 1
                continue
            if oc is None:
                db.execute("DELETE FROM npcclass WHERE name = ?", (name,))
            else:
                db.execute("UPDATE npcclass SET class = ?, source = ? WHERE name = ?", (oc, os_, name))
            db.execute("DELETE FROM npcclass_fix_log WHERE name = ?", (name,))
            back += 1
        db.commit()
        print(f"put back {back} rows; {changed} left as they are now")
        return

    out, spawns = plan(db)
    moves = collections.Counter()
    examples = collections.defaultdict(list)
    for n, (oc, _, nc, _) in sorted(out.items(), key=lambda kv: (-spawns[kv[0]], kv[0])):
        k = ("none" if oc is None else CLASSES[oc], CLASSES[nc])
        moves[k] += 1
        if len(examples[k]) < 8:
            examples[k].append(n)
    made = sum(1 for v in out.values() if v[0] is None)
    print(f"{len(out)} names to write ({sum(spawns[n] for n in out)} spawns): {len(out) - made} Return Home rows "
          f"given their names' classes, {made} new rows for caster-named creatures the table lacks")
    for (o, nw), k in moves.most_common():
        print(f"  {o:>4} -> {nw:<3} {k:4}  e.g. {', '.join(examples[(o, nw)])}")
    if a.list:
        for n, (oc, _, nc, word) in sorted(out.items()):
            print(f"  {n!r:40} {'-' if oc is None else CLASSES[oc]:>4} -> {CLASSES[nc]:<3} {word}")
    if not a.apply:
        print("(report only: --apply writes them, --list lists every name)")
        return
    if not a.no_backup:
        print("copy:", backup(db, a.db))
    db.execute(LOG)
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    for n, (oc, os_, nc, word) in out.items():
        if oc is None:
            db.execute("INSERT INTO npcclass (name, class, source) VALUES (?, ?, ?)", (n, nc, SOURCE))
        else:
            db.execute("UPDATE npcclass SET class = ?, source = ? WHERE name = ?", (nc, SOURCE, n))
        db.execute("INSERT OR REPLACE INTO npcclass_fix_log (name, old_class, old_source, new_class, new_source, word, at) "
                   "VALUES (?, ?, ?, ?, ?, ?, ?)", (n, oc, os_, nc, SOURCE, word, at))
    db.commit()
    print(f"wrote {len(out)} rows")


if __name__ == "__main__":
    main()
