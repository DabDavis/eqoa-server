#!/usr/bin/env python3
"""Lord Valkanith's loot as work board 737 section 1 and Ronson's t299 give it, and the Rune of the Bonewing as the
owner ordered it (2026-11-18: "apply lord valkanith loot table, including the Rune of the Bonewing 100 charges pet
looks like death knight pet. it gives 40 damage shield 20 heal shield and pr/dr buff +45"). Run on each shard's
database; every record is found by NAME (the shards' ids differ), each new id taken on the shard.

SOURCES: eqoa.forumotion.com/t299-tal-thex-loot-table-envar (Ronson, 2010-07-30: the Thexian pieces and "Lord
Valkanith Loot Table", their stats) and t171-lord-valkanith-quest (the same: "the Bone Wing Sentinel (Rune of
Bonewing Charges:100, not rechargeable, Lvl 55+, All/All) ... summons a thexian bonewing familiar ... a lifetap damage
shield doing 40 damage, 20 heal. Also +40 PR/DR. It looks similiar to the Deathknight pet").

1. HIS TABLE, "boss lord valkanith" (kept: the "boss " name is what makes him a raid boss, need before greed,
   database/raidboss.go; tools/valkanith_20261117.py made it), 1 roll at 100% as it was, even weights, each piece of
   gear as its ASCENDED record in place of its base (petition 1446's rule, tools/ascend_boss_loot.py's, as Lord
   Nagafen's and Bertoxxulous's tables hold theirs on both shards):
     Ascended Crown of Tal'thex        (made: L60, all/all, LORE, AC 27, HoT 25; the copy AC 30, HoT 28)
     Ascended positronian energy shield (6067: AC 139 -> 153; its undead damage shield wornshield.go's, +10%)
     Ascended Positronian Mace          (made: 650 one-hand blunt, either hand, L58, t299's ten classes, LORE,
                                         "Procs a 500 point DD, works only on undead": the copy 715, its proc 550)
     Ascended phylactery of valkanith   (7982: PoT 25 -> 28)
     Rune of the Bonewing               (the "Bone Wing Sentinel" record, renamed: below)
     Rune of Defilement                 (made: below)
   The Bone Wing Sentinel row was the same item as t299's Rune of the Bonewing (t171 names the drop "Bone Wing
   Sentinel (Rune of Bonewing ...)"): its record is kept, renamed and made LORE, so a copy anyone holds becomes the
   rune; the 6067 and 7982 rows are pointed at their Ascended records. The shield's classuse is 32767 on both shards
   already (jibrish's 2026-11-12 fix and the 1117 tool): nothing to do. Their other fields stay as they are (t299's
   LORE for 6067/7982, their durability and the shield's description are left: not asked).

2. THE RUNE OF THE BONEWING (L55, all/all, LORE, 100 charges, not rechargeable). Its Use (table itemspell,
   handlers/itemspell.go: needs the server build of branch valk-loot) calls the Thexian Bonewing at once, a charge a
   call (none while it is out: the Use says so), in place of any pet out (a necromancer's or magician's goes, as at any
   summoning):
     - spell 760353 "Summon Thexian Bonewing" (summons; no power, no cast) and its petstats row: the Arch Familiar's
       numbers (700766, the wizard's L55 familiar: 4,540 HP, AC 1,650, passive: it fights only when told) under the
       DEATH KNIGHT'S body, the shadow knight's Death Knight Master Class pet the Undead Servant's (petstats 701236,
       model 1794738819 = 0x6AF98A83, its size), named "Thexian Bonewing";
     - its one ability, spell 760354 "Thexian Bonewing" on its owner, every 30 s while the owner lacks it, from 1 s,
       while_out (it comes off the moment the bonewing goes: dismissed, dead, replaced, the owner's logout; never kept
       over a logout; pet.go petBuffsGoLocked): a damage shield of 40 (every melee hit a creature lands on the owner),
       a heal of 20 at every such hit (the "heal shield": a defensive proc heal at 100%, workbook.go procHealsLocked),
       PR +45 and DR +45 (the owner's figures; t171 says 40, t299 a 20 lifetap proc, 40 DS and 65 DR). Its 30 minutes
       never run out while the bonewing is up: it is put back at once when it lacks it.
   The record's itemeffect stays (it holds the 100 charges a new copy is made with) with its line brought to the
   owner's figures; the rune's 3 held copies on Heirloom made before it had charges (remaincharge 0) are given 100.

3. THE RUNE OF DEFILEMENT (L55, magician, necromancer, enchanter, wizard and alchemist, LORE, 100 charges): its Use
   casts spell 760355 "Defilement" (itemspell, no power, no lockout, instant), "Target, Drain Power by 150, PB AE":
   each creature round its user (the area's own rules, area.go: within 10, the 8 nearest, line of sight) loses 150
   power and its user gains each 150 it took (lifetap.go drainLocked), so up to 1,200; resisted as any spell. Then the
   rune rests 60 s. t299 gives no figure: 150 a creature and the 60 s are ours.

4. THE WAVES' LOOT, "tal'thex armour", 10% x 1 on each of the three Tal'thex kinds (knight, soldier, sorcerer; no
   Ascended: wave trash): t299's thirteen Thexian pieces (737 says 14; t299 lists 13, the bracelet one of them), even
   weights. The Thexian bracelet (7770) exists: its classuse 32767 -> 15360 (t299: MAG NEC ENC WIZ; its stats are
   t299's already). Twelve made, L58, durability 11,600 and 65 (t299's HP and DUR), each in a sibling's look, icon,
   slot, worth and races (the etheric set's of its kind, else the closest L55-60 record), t299's stats, the rest 0:
     caster (MAG NEC ENC WIZ, 15360): Circlet (head, as Circlet of Loathing), Cloth Shirt (Terra Cloth Tunic), Cloth
       Gloves (Etheric cloth gloves), Cloth Legwraps (Etheric Cloth pants), Cloth Slippers (etheric cloth boots);
     leather (RAN MNK BRD RGE DRD SHA ALC, 16898): Leather Helm (badlands helm);
     plate (WAR PAL SK CL, 525): Plated Helm (Etheric plate helm), Breastplate (etheric breastplate), Plated Bracers
       (etheric plate bracers, t299's "2forearm"), Plated Gauntlets, Plated Greaves, Plated Boots (etheric ...).

Report only by default; --apply in one transaction, a gm_audit line a row and the log valkanith_loot_1118_log (every
row before and after); a second --apply does nothing; --applied PATH writes what was made (ids by name) as JSON;
--undo puts back exactly what it wrote (a row changed since is left and said; a made record held by anyone is left)
and drops the log. Refused whole if a row is not as this tool expects (a made name or spell id taken, a source record
missing). Copies the database first (<db>.bak-<when>-pre-valkloot) unless --no-backup.

WHEN IT TAKES EFFECT: the loot, the records and their effects at once (each corpse's first opening, each Use, the
Ascended list every minute); the spells and petstats at a start (and the runes need the valk-loot build): apply in
the keep restart's before.sh, with that build.

    python3 -E -s tools/valkanith_loot_20261118.py --db eqoa.db [--apply [--applied out.json] | --undo] [--no-backup]
"""
import argparse
import datetime
import json
import os
import shutil
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import ascend_boss_loot as abl  # noqa: E402  (the server's Ascended rule: TestAscendBossLootToolKeepsTheRule)

LOG = "valkanith_loot_1118_log"
WHO, VIA = "dab", "tool"
SRC = "valkloot1118"
DETAIL = ("Lord Valkanith's loot (work board 737, t299) and the Rune of the Bonewing (the owner, 2026-11-18): "
          "tools/valkanith_loot_20261118.py")

BOSS = "lord valkanith"
BOSS_TABLE = "boss lord valkanith"
BOSS_NOTE = ("t299 (Ronson, eqoa.forumotion.com/t299, work board 737): the Crown, Shield, Mace and Phylactery as their "
             "Ascended records (petition 1446), the Rune of the Bonewing (t171's Bone Wing Sentinel) and the Rune of "
             "Defilement; one roll, the owner's order 2026-11-18")
WAVES = ["tal'thex knight", "tal'thex soldier", "tal'thex sorcerer"]
WAVE_TABLE = "tal'thex armour"
WAVE_CHANCE, WAVE_ROLLS = 10, 1
WAVE_NOTE = "work board 737: t299's Thexian pieces off the Tal'thex waves, 10% x 1 (rate ours, jibrish's)"
MINLEVEL, MAXLEVEL = 1, 70

OLD_RUNE = "Bone Wing Sentinel"
RUNE = "Rune of the Bonewing"
RUNE_DESC = "A rune that when invoked summons a thexian bonewing familiar to aid you."
RUNE_LINE = "Self, Damage Shield Decrease Hitpoints by 40, Increase PR by 45, Increase DR by 45, 30min"
CHARGES = 100
SHIELD, PHYLACTERY, BRACELET = "positronian energy shield", "phylactery of valkanith", "Thexian bracelet"
CROWN, MACE, DEFILE_ITEM = "Crown of Tal'thex", "Positronian Mace", "Rune of Defilement"

# the spells, ours (the 76xxxx run of this server's own: Valkanith's Bloodtap is 760352)
SUMMON, BUFF, DEFILE = 760353, 760354, 760355
PET_NAME = "Thexian Bonewing"
PET_STATS_FROM, PET_BODY_FROM = "Arch Familiar", "Undead Servant"   # petstats names
SUMMON_FROM, BUFF_FROM = 701236, 907236                              # Summon Servant, Undead Servitude
DEFILE_POWER, DEFILE_REST = 150, 60.0


def js(v):
    return None if v is None else json.dumps(v, separators=(",", ":"), ensure_ascii=False, sort_keys=True)


SUMMON_EFFECTS = js({"v": 1, "scope": "self", "target_type": "Self", "harmful": False, "stack": 0, "level": 55,
                     "min_level": 1, "req_equipment": "Any", "sheet_type": "Summon Pet",
                     "effects": [{"id": "V001", "type": "Summon Pet", "kind": "summon",
                                  "pet": {"sheet_pet_id": "VALK", "name": PET_NAME}}]})
BUFF_EFFECTS = js({"v": 1, "scope": "target", "target_type": "Owner", "harmful": False, "duration_s": 1800, "stack": 0,
                   "level": 55, "min_level": 1, "req_equipment": "Any", "sheet_type": "Statistic Change",
                   "arch_type": "Pet Buff",
                   "effects": [
                       {"id": "V002", "type": "Defensive Proc", "kind": "def_proc", "procs": [
                           {"id": "V003", "type": "Dmg/Heal", "kind": "hp", "amount": -40, "on": "defense",
                            "scope": "target", "target_type": "Enemy"},
                           {"id": "V004", "type": "Dmg/Heal", "kind": "hp", "amount": 20, "on": "defense",
                            "scope": "self", "target_type": "Self"}]},
                       {"id": "V005", "type": "Statistic Change", "kind": "stat", "stat": "pr", "amount": 45},
                       {"id": "V006", "type": "Statistic Change", "kind": "stat", "stat": "dr", "amount": 45}]})
BUFF_LINE = ("Target, Damage Shield Decrease Hitpoints by 40, Heal 20 when hit, Increase PR by 45, Increase DR by 45, "
             "while the bonewing is out")
DEFILE_LINE = f"Target, Drain Power by {DEFILE_POWER}, PB AE"
PET_ABILITIES = js([{"every": 30.0, "first": 1.0, "kind": "buff", "spell_id": BUFF, "target": "owner",
                     "while_out": True}])

SPELLS = {  # id: (copied from, fields over it, line, effects, fxdict from)
    SUMMON: (SUMMON_FROM, {"spellname": "Summon Thexian Bonewing", "abilitylvl": 55, "casttime": 0, "power": 0,
                           "recast": 0,
                           "spelldesc": "Calls a thexian bonewing familiar: while it is out its master has a damage "
                                        "shield of 40, 20 hit points back at every blow taken, and 45 more poison and "
                                        "disease resist."},
             "Summons a Thexian Bonewing, a familiar", SUMMON_EFFECTS, SUMMON_FROM),
    BUFF: (BUFF_FROM, {"spellname": PET_NAME, "abilitylvl": 55,
                       "spelldesc": "The thexian bonewing's gift to its master while it serves: a lifetap damage "
                                    "shield (40 damage, 20 heal) and poison and disease resist."},
           BUFF_LINE, BUFF_EFFECTS, BUFF_FROM),
    DEFILE: (BUFF_FROM, {"spellname": "Defilement", "abilitylvl": 55, "casttime": 0, "power": 0, "recast": 0,
                         "scope": 0,
                         "spelldesc": "Energies from your surroundings are absorbed into your body: power drained "
                                      "from each creature around you."},
             DEFILE_LINE, "", None),  # visual: the server's stand-in for its kind
}

STAT_COLS = ("str", "sta", "agi", "wis", "dex", "cha", "intelligence", "HPMAX", "POWMAX", "PoT", "HoT", "AC", "PR", "DR",
             "FR", "CR", "LR", "AR")
CASTER, LEATHER, PLATE = 15360, 16898, 525
ALL_CLASSES, MACE_CLASSES, DEFILE_CLASSES = 32767, 1023, 31744
THEX_DESC = "This armor is finely crafted and in great even though it is very old."


def thex(sib, classes, stats):
    return (sib, {"classuse": classes, "levelreq": 58, "maxhp": 11600, "duration": 65, "lore": 0,
                  "itemdesc": THEX_DESC}, stats)


# made records: name -> (sibling name, fields over it, stats (the rest 0)); t299's figures
MADE = {
    CROWN: ("Circlet of Loathing",
            {"classuse": ALL_CLASSES, "levelreq": 60, "lore": 1, "maxhp": 12000, "duration": 80,
             "itemdesc": "The crown of the fallen Tal'Thex elven ruler, Lord Valkanith. The crown contains "
                         "unparalleled regenerative magics."},
            {"AC": 27, "HoT": 25}),
    MACE: ("Dazzling Mace",
           {"classuse": MACE_CLASSES, "levelreq": 58, "lore": 1, "weapondamage": 650, "equipslot": 14, "attacktype": 3, "maxhp": 11600, "duration": 60,
            "itemdesc": "This mace had been imbued long ago with vast amounts of positive energy by the tal'thex elves. "
                        "It causes great harm to undead. Procs a 500 point DD, works only on undead."},
           {}),
    DEFILE_ITEM: (OLD_RUNE,
                  {"classuse": DEFILE_CLASSES, "levelreq": 55, "lore": 1,
                   "itemdesc": "When invoked energies from your surroundings are absorbed into your body."},
                  {}),
    "Thexian Circlet": thex("Circlet of Loathing", CASTER,
                            {"dex": 30, "HPMAX": 80, "POWMAX": 45, "AC": 23, "intelligence": 10}),
    "Thexian Cloth Shirt": thex("Terra Cloth Tunic", CASTER,
                                {"sta": 30, "HPMAX": 80, "POWMAX": 45, "AC": 87, "intelligence": 10}),
    "Thexian Cloth Gloves": thex("Etheric cloth gloves", CASTER,
                                 {"agi": 30, "HPMAX": 80, "POWMAX": 45, "AC": 44, "intelligence": 10}),
    "Thexian Cloth Legwraps": thex("Etheric Cloth pants", CASTER,
                                   {"cha": 10, "HPMAX": 80, "POWMAX": 45, "AC": 58, "intelligence": 30}),
    "Thexian Cloth Slippers": thex("etheric cloth boots", CASTER,
                                   {"sta": 10, "HPMAX": 80, "POWMAX": 45, "AC": 29, "intelligence": 30}),
    "Thexian Leather Helm": thex("badlands helm", LEATHER, {"dex": 30, "HPMAX": 80, "POWMAX": 45, "AC": 46, "str": 10}),
    "Thexian Plated Helm": thex("Etheric plate helm", PLATE, {"str": 10, "HPMAX": 80, "POWMAX": 45, "AC": 93, "cha": 30}),
    "Thexian Breastplate": thex("etheric breastplate", PLATE,
                                {"sta": 30, "HPMAX": 80, "str": 10, "POWMAX": 45, "AC": 348}),
    "Thexian Plated Bracers": thex("etheric plate bracers", PLATE,
                                   {"str": 30, "intelligence": 10, "HPMAX": 80, "POWMAX": 45, "AC": 174}),
    "Thexian Plated Gauntlets": thex("etheric plate gauntlets", PLATE,
                                     {"sta": 30, "HPMAX": 80, "POWMAX": 45, "AC": 174, "cha": 10}),
    "Thexian Plated Greaves": thex("etheric plate greaves", PLATE,
                                   {"sta": 10, "HPMAX": 80, "POWMAX": 45, "AC": 232, "wis": 30}),
    "Thexian Plated Boots": thex("Etheric Plate Boots", PLATE,
                                 {"sta": 30, "HPMAX": 80, "POWMAX": 45, "AC": 128, "wis": 10}),
}
THEXIAN = [n for n in MADE if n.startswith("Thexian ")]
WAVE_ITEMS = THEXIAN[:2] + [BRACELET] + THEXIAN[2:]
ASCEND = [CROWN, SHIELD, MACE, PHYLACTERY]
BOSS_ITEMS = ASCEND + [RUNE, DEFILE_ITEM]
ITEM_SPELLS = {RUNE: (SUMMON, 0.0, "the owner's Rune of the Bonewing (2026-11-18): the Thexian Bonewing"),
               DEFILE_ITEM: (DEFILE, DEFILE_REST, "work board 737, t299: the Rune of Defilement's area power tap")}
ITEMSPELL_SCHEMA = """CREATE TABLE IF NOT EXISTS itemspell (
		patternid INTEGER PRIMARY KEY,
		spellid   INTEGER NOT NULL,
		rest_s    REAL NOT NULL DEFAULT 0,
		note      TEXT NOT NULL DEFAULT '',
		source    TEXT NOT NULL DEFAULT 'gm'
	)"""


class Refused(Exception):
    pass


def has(db, t):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone()[0] > 0


def row(db, tbl, key):
    where = " AND ".join(f'"{k}" = ?' for k in key)
    cur = db.execute(f'SELECT * FROM "{tbl}" WHERE {where}', list(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def copies(db):
    """Every record that is a copy (an import's or an Ascended one): never a base or a sibling."""
    out = set()
    for t in ("itemascended", "itemimported"):
        if has(db, t):
            out |= {r[0] for r in db.execute(f"SELECT patternid FROM {t}")}
    return out


def by_name(db, name, skip=()):
    """The record of that name (any case, trimmed), the lowest id that is no copy; None for none."""
    for (pid,) in db.execute("SELECT patternid FROM itempattern WHERE lower(trim(itemname)) = lower(?) ORDER BY patternid",
                             (name.strip(),)):
        if pid not in skip:
            return pid
    return None


def ascended_of(db, base):
    if not has(db, "itemascended"):
        return None
    r = db.execute("SELECT patternid FROM itemascended WHERE base = ?", (base,)).fetchone()
    return r[0] if r else None


def check(db):
    """What the shard holds, refused when it is not as this tool expects: a dict of the ids it works on."""
    for t in ("itempattern", "itemeffect", "loottables", "npcloot", "spellpattern", "spelleffect", "petstats",
              "gm_audit"):
        if not has(db, t):
            raise Refused(f"no table {t}")
    skip = copies(db)
    ids = {}
    for name in (OLD_RUNE, SHIELD, PHYLACTERY, BRACELET):
        pid = by_name(db, name, skip)
        if pid is None:
            raise Refused(f"no record {name!r}")
        ids[name] = pid
    for name in MADE:
        if by_name(db, name) is not None:
            raise Refused(f"a record {name!r} exists already")
        sib = MADE[name][0]
        if sib != OLD_RUNE and by_name(db, sib, skip) is None:
            raise Refused(f"no sibling record {sib!r} for {name!r}")
    if by_name(db, RUNE) is not None:
        raise Refused(f"a record {RUNE!r} exists already")
    for sid, (src, _, _, _, fxfrom) in SPELLS.items():
        if row(db, "spellpattern", {"spellid": sid}) or row(db, "spelleffect", {"spellid": sid}) or \
                row(db, "petstats", {"spellid": sid}):
            raise Refused(f"spell id {sid} is taken")
        if not row(db, "spellpattern", {"spellid": src}) or fxfrom and not row(db, "spelleffect", {"spellid": fxfrom}):
            raise Refused(f"no spell {src}/{fxfrom} to copy")
    for n in (PET_STATS_FROM, PET_BODY_FROM):
        if not db.execute("SELECT 1 FROM petstats WHERE name = ?", (n,)).fetchone():
            raise Refused(f"no petstats row {n!r}")
    rows = db.execute("SELECT patternid FROM loottables WHERE tablename = ? ORDER BY id", (BOSS_TABLE,)).fetchall()
    want = sorted([ids[OLD_RUNE], ids[SHIELD], ids[PHYLACTERY]])
    if sorted(r[0] for r in rows) != want:
        raise Refused(f"table {BOSS_TABLE!r} holds {[r[0] for r in rows]}, not the 1117 tool's {want}")
    b = row(db, "npcloot", {"npc_name": BOSS, "tablename": BOSS_TABLE})
    if b is None:
        raise Refused(f"{BOSS!r} is not bound to {BOSS_TABLE!r}")
    if db.execute("SELECT 1 FROM loottables WHERE tablename = ?", (WAVE_TABLE,)).fetchone() or \
            db.execute("SELECT 1 FROM npcloot WHERE tablename = ?", (WAVE_TABLE,)).fetchone():
        raise Refused(f"table {WAVE_TABLE!r} exists already")
    for name in (ids[SHIELD], ids[PHYLACTERY]):
        if ascended_of(db, name):
            raise Refused(f"record {name} has an Ascended copy already")
    if has(db, "itemspell") and db.execute("SELECT 1 FROM itemspell WHERE patternid = ?", (ids[OLD_RUNE],)).fetchone():
        raise Refused("the rune has an itemspell row already")
    return ids


def report(db, ids):
    print(f"  {BOSS_TABLE}: now {[r for r in db.execute('SELECT l.patternid, p.itemname FROM loottables l JOIN itempattern p USING (patternid) WHERE tablename = ? ORDER BY l.id', (BOSS_TABLE,))]}")
    print(f"  {BOSS_TABLE} after: " + ", ".join(("Ascended " + n if n in ASCEND else n) for n in BOSS_ITEMS) +
          " (1 roll at 100%, even weights)")
    print(f"  {OLD_RUNE} ({ids[OLD_RUNE]}) -> {RUNE}: LORE, {CHARGES} charges, Use: spell {SUMMON} (pet {PET_NAME})")
    held = db.execute("SELECT count(*) FROM charinventory WHERE patternid = ? AND remaincharge <= 0",
                      (ids[OLD_RUNE],)).fetchone()[0] if has(db, "charinventory") else 0
    print(f"  held copies of it with no charges, given {CHARGES}: {held}")
    print(f"  made: {', '.join(MADE)}")
    print(f"  {BRACELET} ({ids[BRACELET]}): classuse -> {CASTER}")
    print(f"  {WAVE_TABLE}: {len(WAVE_ITEMS)} pieces, {WAVE_CHANCE}% x {WAVE_ROLLS} on {', '.join(WAVES)}")
    print(f"  spells {SUMMON}, {BUFF}, {DEFILE}; petstats {SUMMON} ({PET_NAME}); itemspell rows for both runes")


def apply(db, ids):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() \
        if has(db, "accountinfo") else None
    db.execute(f"CREATE TABLE {LOG} (seq INTEGER PRIMARY KEY, tbl TEXT NOT NULL, rowkey TEXT NOT NULL, before TEXT, "
               f"after TEXT)")
    made = {}

    def done(tbl, key, before):
        after = row(db, tbl, key)
        action = "insert" if before is None else "delete" if after is None else "update"
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES (?, ?, ?, ?)",
                   (tbl, js(key), js(before), js(after)))
        if tbl == "sqlite_sequence":
            return  # the ids' counter, put back by --undo; no GM's row
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                   "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                   (at, acct[0] if acct else 0, WHO, VIA, action, tbl, js(key), js(before), js(after), DETAIL))

    def insert(tbl, r, key):
        db.execute(f'INSERT INTO "{tbl}" ({", ".join(chr(34) + c + chr(34) for c in r)}) VALUES '
                   f'({", ".join("?" * len(r))})', list(r.values()))
        done(tbl, key, None)

    def update(tbl, key, sets):
        before = row(db, tbl, key)
        where = " AND ".join(f'"{k}" = ?' for k in key)
        db.execute(f'UPDATE "{tbl}" SET {", ".join(chr(34) + c + chr(34) + " = ?" for c in sets)} WHERE {where}',
                   list(sets.values()) + list(key.values()))
        done(tbl, key, before)

    seqs = {t: row(db, "sqlite_sequence", {"name": t}) for t in ("itempattern", "loottables")}
    skip = copies(db)
    # the spells and the pet
    for sid, (src, fields, line, effects, fxfrom) in SPELLS.items():
        insert("spellpattern", dict(row(db, "spellpattern", {"spellid": src}), spellid=sid, **fields), {"spellid": sid})
        fx = row(db, "spelleffect", {"spellid": fxfrom})["fxdict"] if fxfrom else 0
        insert("spelleffect", {"spellid": sid, "line": line, "fxdict": fx, "source": f"{SRC}: {DETAIL}",
                               "effects": effects}, {"spellid": sid})
    stats = row_named(db, PET_STATS_FROM)
    body = row_named(db, PET_BODY_FROM)
    insert("petstats", dict(stats, spellid=SUMMON, sheet_pet_id="VALK", name=PET_NAME, model=body["model"],
                            size=body.get("size", 0), primary_model=0, secondary_model=0, level=55,
                            abilities=PET_ABILITIES, keep_template=0, source=SRC,
                            note=(f"the owner, 2026-11-18: the Rune of the Bonewing's familiar, 'pet looks like death "
                                  f"knight pet. it gives 40 damage shield 20 heal shield and pr/dr buff +45': the "
                                  f"{PET_STATS_FROM}'s numbers ({stats['spellid']}), the {PET_BODY_FROM}'s body "
                                  f"({body['spellid']}, the Death Knight's), its buff {BUFF} while it is out; "
                                  f"tools/valkanith_loot_20261118.py")),
           {"spellid": SUMMON})

    # the rune: renamed, LORE, its line, its charges for copies made before
    rune = ids[OLD_RUNE]
    update("itempattern", {"patternid": rune}, {"itemname": RUNE, "lore": 1, "itemdesc": RUNE_DESC})
    if row(db, "itemeffect", {"patternid": rune}):
        update("itemeffect", {"patternid": rune}, {"line": RUNE_LINE, "name": PET_NAME, "charges": CHARGES})
    else:
        insert("itemeffect", {"patternid": rune, "line": RUNE_LINE, "name": PET_NAME, "charges": CHARGES, "source": SRC},
               {"patternid": rune})
    if has(db, "charinventory"):
        for (iid,) in db.execute("SELECT itemid FROM charinventory WHERE patternid = ? AND remaincharge <= 0",
                                 (rune,)).fetchall():
            update("charinventory", {"itemid": iid}, {"remaincharge": CHARGES})
    made[RUNE] = rune

    # the records made, each a sibling's look
    for name, (sib, fields, st) in MADE.items():
        sib_id = ids[OLD_RUNE] if sib == OLD_RUNE else by_name(db, sib, skip)  # the rune renamed above
        r = dict(row(db, "itempattern", {"patternid": sib_id}), itemname=name, **fields)
        del r["patternid"]
        for c in STAT_COLS:
            r[c] = st.get(c, 0)
        cur = db.execute(f'INSERT INTO itempattern ({", ".join(chr(34) + c + chr(34) for c in r)}) VALUES '
                         f'({", ".join("?" * len(r))})', list(r.values()))
        made[name] = cur.lastrowid
        done("itempattern", {"patternid": cur.lastrowid}, None)
    insert("itemeffect", {"patternid": made[DEFILE_ITEM], "line": f"Self, Increase Power by {DEFILE_POWER}",
                          "name": "Defilement", "charges": CHARGES, "source": SRC}, {"patternid": made[DEFILE_ITEM]})
    db.execute(ITEMSPELL_SCHEMA)
    for name, (sid, rest, note) in ITEM_SPELLS.items():
        insert("itemspell", {"patternid": made[name], "spellid": sid, "rest_s": rest, "note": note, "source": SRC},
               {"patternid": made[name]})
    update("itempattern", {"patternid": ids[BRACELET]}, {"classuse": CASTER})

    # the Ascended records (the server's SQL: tools/ascend_boss_loot.py's)
    db.execute(abl.SCHEMA)
    sets, froms, _ = abl.copy_columns([r[1] for r in db.execute("PRAGMA table_info(itempattern)")])
    asc = {}
    for name in ASCEND:
        base = made.get(name) or ids[name]
        nid = db.execute("SELECT COALESCE(MAX(patternid), ?) + 1 FROM itempattern WHERE patternid BETWEEN ? AND ?",
                         (abl.FIRST_ID - 1, abl.FIRST_ID, abl.LAST_ID)).fetchone()[0]
        if nid > abl.LAST_ID:
            raise Refused("no Ascended id left")
        db.execute(f'INSERT INTO itempattern ("patternid", {sets}) SELECT ?, {froms} FROM itempattern WHERE patternid = ?',
                   (nid, base))
        done("itempattern", {"patternid": nid}, None)
        insert("itemascended", {"patternid": nid, "base": base, "made": at,
                                "source": f"tools/valkanith_loot_20261118.py {at}"}, {"patternid": nid})
        asc[name] = nid
        made["Ascended " + name] = nid

    # his table: the shield and the phylactery rows to their Ascended records, the rest added
    for name in (SHIELD, PHYLACTERY):
        (lid,) = db.execute("SELECT id FROM loottables WHERE tablename = ? AND patternid = ?",
                            (BOSS_TABLE, ids[name])).fetchone()
        update("loottables", {"id": lid}, {"patternid": asc[name], "source": SRC})
    for name in (CROWN, MACE):
        pid = asc[name]
        cur = db.execute("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) "
                         "VALUES (?, ?, 1, ?, ?, ?)", (BOSS_TABLE, pid, MINLEVEL, MAXLEVEL, SRC))
        done("loottables", {"id": cur.lastrowid}, None)
    cur = db.execute("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) "
                     "VALUES (?, ?, 1, ?, ?, ?)", (BOSS_TABLE, made[DEFILE_ITEM], MINLEVEL, MAXLEVEL, SRC))
    done("loottables", {"id": cur.lastrowid}, None)
    update("npcloot", {"npc_name": BOSS, "tablename": BOSS_TABLE}, {"note": BOSS_NOTE, "source": SRC})

    # the waves' table
    for name in WAVE_ITEMS:
        pid = ids[BRACELET] if name == BRACELET else made[name]
        cur = db.execute("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) "
                         "VALUES (?, ?, 1, ?, ?, ?)", (WAVE_TABLE, pid, MINLEVEL, MAXLEVEL, SRC))
        done("loottables", {"id": cur.lastrowid}, None)
    for npc in WAVES:
        insert("npcloot", {"npc_name": npc, "tablename": WAVE_TABLE, "chance": WAVE_CHANCE, "rolls": WAVE_ROLLS,
                           "note": WAVE_NOTE, "source": SRC}, {"npc_name": npc, "tablename": WAVE_TABLE})
    for t, before in seqs.items():
        if before is not None:
            done("sqlite_sequence", {"name": t}, before)  # --undo puts the counters back
    return made


def row_named(db, name):
    cur = db.execute("SELECT * FROM petstats WHERE name = ? ORDER BY spellid LIMIT 1", (name,))
    return dict(zip([d[0] for d in cur.description], cur.fetchone()))


def undo(db):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() \
        if has(db, "accountinfo") else None
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
            if tbl == "itempattern" and has(db, "charinventory"):
                held = db.execute("SELECT COUNT(*) FROM charinventory WHERE patternid = ?", (key["patternid"],)).fetchone()[0]
                if held:
                    print(f"  itempattern {key['patternid']}: held by {held}, left")
                    left += 1
                    continue
            db.execute(f'DELETE FROM "{tbl}" WHERE {where}', list(key.values()))
        elif after is None:
            b = json.loads(before)
            db.execute(f'INSERT INTO "{tbl}" ({", ".join(chr(34) + c + chr(34) for c in b)}) VALUES '
                       f'({", ".join("?" * len(b))})', list(b.values()))
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


def backup(path):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-valkloot"
    src = sqlite3.connect(path)
    out = sqlite3.connect(dst)
    with out:
        src.backup(out)
    src.close()
    out.close()
    print(f"  backup: {dst}")


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    ap.add_argument("--applied", help="with --apply: write the records made (name -> id) here as JSON")
    ap.add_argument("--no-backup", action="store_true")
    a = ap.parse_args(argv)
    if not os.path.exists(a.db):
        print(f"no database {a.db}", file=sys.stderr)
        return 2
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 10000")
    logged = has(db, LOG)
    if a.undo:
        if not logged:
            print("nothing to undo: no log")
            return 0
        if not a.no_backup:
            backup(a.db)
        db.execute("BEGIN IMMEDIATE")
        try:
            undo(db)
            db.execute("COMMIT")
        except Exception:
            db.execute("ROLLBACK")
            raise
        return 0
    if logged:
        print(f"applied already (log {LOG}): nothing to do")
        return 0
    try:
        ids = check(db)
    except Refused as e:
        print(f"refused: {e}", file=sys.stderr)
        return 1
    report(db, ids)
    if not a.apply:
        print("report only: --apply to write")
        return 0
    if not a.no_backup:
        backup(a.db)
    db.execute("BEGIN IMMEDIATE")
    try:
        made = apply(db, ids)
        db.execute("COMMIT")
    except Exception:
        db.execute("ROLLBACK")
        raise
    for name, pid in made.items():
        print(f"  {pid:>8}  {name}")
    if a.applied:
        with open(a.applied, "w") as f:
            json.dump({"db": a.db, "made": made, "spells": {"summon": SUMMON, "buff": BUFF, "defilement": DEFILE}}, f,
                      indent=1, sort_keys=True)
    print("applied; loot and records at once; the spells, petstats and the runes' Use at the next start (the valk-loot "
          "build)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
