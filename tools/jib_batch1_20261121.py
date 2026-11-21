#!/usr/bin/env python3
"""jibrish's work board asks of 2026-11-10..20, batch 1 (the owner approves his asks as submitted): small data
changes, one part an entry, each run alone with --only / --skip. Every row is found by NAME and checked against what
the entry read; a part whose rows are neither as read nor as asked REFUSES (nothing is written). A part already in
place writes nothing, so the same run does each shard (Live's residue of what Heirloom already has).

  670 673 674 677  the epic bosses' loot on a shard that lacks the tables their npcloot rows name (Live): the tables
        "epic ancient corpse", "epic cyrcalis", "epic graal" and "epic boss armour" copied from --source (Heirloom,
        read only): each row's item mapped to this shard's record of the same name, an Ascended record to this shard's
        Ascended record of the same base (itemascended), made by the server's rule (database/ascended.go, as
        tools/ascend_boss_loot.py makes one) where the shard has none (part "ascended", taken with them). 677: Teledic's
        respawn is already 1,800 s on both shards; his path (question 2) is not built.
  668   Snowgrin's remainder: Grilmesh Frostcackle is a friendly NPC (npc_type 128), and "boss snowgrin" as --source has
        it (its Ascended rows).
  1832  lootcreature: the 128 capitalised keys lower-cased (CategoryRolls looks up NPCClassKey); refused whole when a
        lower-case twin exists.
  656   The Guardian (118859, 118860) attackable (npc_type 0) and quest 101311 step 5's teleport on the doorway floor
        (petition 2524's fix on Heirloom). Count and scent removal (questions 1-2) untouched.
  1764  Slagbreath attackable (npc_type 0). He stands in Freeport (a town: entity/aggro.go's nature rule) and has no
        npcfaction row, so he attacks no one on sight.
  1819  Merchant Samwe (106253) sells the Small Gear Wheel 700368 (one npcitem row, her next list number); her steps'
        targets checked.
  709   board #190-#191: Sarek the Betrayer's kill hit (Sarek the Betrayer -505, as Heirloom's 1818) and on Live his
        respawn 259,200 s (Heirloom keeps 1820's 82,800 s); refused when the name has another row or world.
  1718  High Guard Ianant L37 -> L38 (his spot left: this shard's differs from the entry's "from").
  1763  quest 100600 steps 1-2: the iron ore and ivory are bought, not handed out (rewarditems '', journal lines).
  1785  quest 101073 step 3 xp 73,000,000 -> 100,000,000.
  1767  Call Lightning 20% DEX; Captivate Animal "up to level 53" (the cap is read from the line's words,
        entity/spelljson.go); Wild Growth radius 15.
  1768  Radiance 900491 range 16880 -> 16544 (Influence and Divinity are already 16544).
  1769  Curse of Bertoxxulous 901501 STR 25% -> 20%; Cursed Touch 700273 STR 20% and an effects JSON as 901444's.
  1770  Caltrops 700027: one hit of 15 (the DoT's total, a stand-in) for the DoT, the snare kept.
  1771  Crazy Contraption recast 3600; Destroy Contraption 1 returns 50 power to the group (Contraption 2's, a stand-in).
  1774  Sacred Heart 701066: a self buff, a defensive proc HoT of 250 + 150% CHA, 90 s, recast 900.
  640   Qual'Tin not KoS (npcaggro 0), his own resists cleared, Graumord +11 a kill. Held: the 3,500 AoE and 200k
        (question 1), loot (2), the copies (3); "Denizens -11" names no faction; the popup and the 6,500 kick are in.
  1599  (Heirloom) the Spirit Maker's Robe from the two Tower of Phantoms elder spirits (#190044, #190045) at 0.1%, the
        rest of 1598's stopgap out (lootrate, lootpool 12158), in the entry's order. Himlar's 1% (entry 1600) stays: so
        tools/jib1117_20261117.py elder, which refuses a table bound to another name, is not used.
  1625  rations: nine stacks to 20, the races start with 20, Freeport humans with desert rations (eatable).
  1047  Poacher's Leggings: skin 0.18, carapace 0.08, the beach crabs count (queststeptargets).
  1060  rotbone skeletons drop the Ash Covered Shortsword (2%).
  1066  young rattlers L11-15 -> 8-10 (39 rows), "cooperhead" 115887 -> copperhead.
  992   Nasehir Sandcaster (2040) -> "Nasehir sandcaster", L14.
  964   "Fereal Spirit" 100518 -> Feral Spirit; the five "fereal spirit" rule rows deleted.
  1014  sections 1 and 3: one Ziliq (Zilq 108621 and its rule rows deleted, its npcclass row renamed); four
        "Decaying Minion" (section 2 was built 2026-11-21).
  821 792 841 867 873 884 894  the renames the Live copy missed (each new name's rule rows are there already):
        Guard Joradson; Cartographer Leianya; Merchant Moira, Shavir Thanan, stable boy (+ its two kill hits' keys);
        Merchant Opplefiz; Bruun Coalcarver, Armorer Twoforger, Guardian Bukel, Guardian Gunga; murk bone (50) and
        firebeetle (the 77 rows round Qeynos); Costume Vendor, Tailor Foderhoffen.
  1117 1129  a troll shadowknight's and a troll shaman's starting standings (factionstart, 3 rows each).

What takes effect when: loot tables, npcloot, lootcreature and merchants' stock at once; spells at the next cast (a
spell book at its owner's login); item records at a client's login; startitems at the next character made; npcs
(names, levels, types, deletions) at the panel's reload_npcs; quests at the panel's quest reload; factionhits,
factionstart, npcaggro and npcrespawn at the server's next start (or a panel edit of a row of that table).

Dry run by default. --apply writes in one transaction, a gm_audit line a row (accountid 1, dab, via tool), every row's
before and after in jib_batch1_1121_log; a second --apply writes nothing. --undo puts back what the log holds for the
parts named (default all), latest first; a row changed since is left and said, and an Ascended record something holds
or names is kept.

    python3 -E -s tools/jib_batch1_20261121.py --db eqoa.db --shard heirloom|live [--source HEIRLOOM.db]
        [--only 670,1832 | --skip 1832] [--apply | --undo]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import ascend_boss_loot as asc  # noqa: E402  (the server's Ascended rule)

LOG = "jib_batch1_1121_log"
WHO, ACCOUNT, VIA = "dab", 1, "tool"
TOOL = "tools/jib_batch1_20261121.py"
PARTS = ("670", "673", "674", "677", "668", "1832", "656", "1764", "1819", "709", "1718", "1763", "1785", "1767",
         "1768", "1769", "1770", "1771", "1774", "640", "1599", "1625", "1047", "1060", "1066", "992", "964", "1014",
         "821", "792", "841", "867", "873", "884", "894", "1117", "1129")
NEEDS_SOURCE = ("670", "673", "674", "677", "668")
ASC = "ascended"  # the part the Ascended records a loot part makes are logged under

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    part   TEXT NOT NULL,
    tbl    TEXT NOT NULL,
    rid    INTEGER NOT NULL,  -- the row's rowid
    rowkey TEXT NOT NULL,     -- its key (JSON)
    before TEXT,              -- the whole row before (JSON; NULL: none)
    after  TEXT,              -- the whole row after (JSON; NULL: removed)
    audit  INTEGER NOT NULL,
    at     TEXT NOT NULL
)"""


def detail(part):
    if part == ASC:
        return f"an Ascended record a mirrored epic loot table names (database/ascended.go's rule): {TOOL}"
    return f"jibrish's work board entry {part} (the owner approves his asks as submitted): {TOOL}"


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def dumps(v):
    s = json.dumps(v, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def jdump(obj):
    """An effects JSON as the rows keep it (key order kept)."""
    return json.dumps(obj, separators=(",", ":"), ensure_ascii=False)


class Refused(Exception):
    pass


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def cols(db, tbl):
    return [r[1] for r in db.execute(f'PRAGMA table_info("{tbl}")')]


def pk(db, tbl):
    p = sorted((r[5], r[1]) for r in db.execute(f'PRAGMA table_info("{tbl}")') if r[5])
    return [c for _, c in p]


def rows(db, tbl, where):
    w = " AND ".join(f'"{c}" = ?' for c in where) or "1"
    cur = db.execute(f'SELECT rowid AS _rowid_, * FROM "{tbl}" WHERE {w} ORDER BY rowid', tuple(where.values()))
    names = [d[0] for d in cur.description]
    return [dict(zip(names, r)) for r in cur.fetchall()]


def by_rowid(db, tbl, rid):
    r = rows(db, tbl, {"rowid": rid})
    return r[0] if r else None


def plain(r):
    return None if r is None else {k: v for k, v in r.items() if k != "_rowid_"}


def same(a, b):
    if isinstance(a, (int, float)) and isinstance(b, (int, float)) and not isinstance(a, bool):
        return abs(float(a) - float(b)) < 1e-9
    return a == b


class Plan:
    def __init__(self, db, shard, src):
        self.db, self.shard, self.src = db, shard, src
        self.part = ""
        self.ops = []        # (part, tbl, rid or None, before row or None, after values or None)
        self.lines = []
        self.refusals = []
        self.next_asc = None
        self.asc_made = {}   # base -> planned Ascended id
        self.world = None

    def say(self, s):
        self.lines.append(f"  {self.part}: {s}")

    # ---- reading
    def one(self, tbl, where, what=None):
        r = rows(self.db, tbl, where)
        if len(r) != 1:
            raise Refused(f"{what or tbl} {dumps(where)}: {len(r)} rows, not one")
        return r[0]

    # ---- planning
    def set(self, tbl, row, changes, what):
        """changes {col: (old, new)}: each column as read (old; a tuple of olds allowed) or already as asked."""
        new = {}
        for c, (old, want) in changes.items():
            have = row[c]
            olds = old if isinstance(old, tuple) else (old,)
            if same(have, want):
                continue
            if not any(same(have, o) for o in olds):
                raise Refused(f"{what}: {tbl}.{c} is {have!r}, neither as read ({old!r}) nor as asked ({want!r})")
            new[c] = want
        if not new:
            self.say(f"{what}: already as asked")
            return False
        self.ops.append((self.part, tbl, row["_rowid_"], row, new))
        self.say(f"{what}: " + ", ".join(f"{c} {row[c]!r} -> {v!r}" for c, v in new.items()))
        return True

    def add(self, tbl, values, match, what):
        """Insert values unless a row matching match is there: as values (done) or otherwise (refused)."""
        have = rows(self.db, tbl, {c: values[c] for c in match})
        if have:
            if len(have) == 1 and all(same(have[0][c], v) for c, v in values.items()):
                self.say(f"{what}: already there")
                return False
            raise Refused(f"{what}: {tbl} has {dumps([plain(h) for h in have])}, not {dumps(values)}")
        self.ops.append((self.part, tbl, None, None, dict(values)))
        self.say(f"{what}: insert {tbl} {dumps(values)}")
        return True

    def drop(self, tbl, where, expect, what):
        have = rows(self.db, tbl, where)
        if not have:
            self.say(f"{what}: already gone")
            return False
        if len(have) != 1:
            raise Refused(f"{what}: {len(have)} rows of {dumps(where)}")
        r = have[0]
        for c, v in expect.items():
            if not same(r[c], v):
                raise Refused(f"{what}: {tbl}.{c} is {r[c]!r}, not {v!r} as read")
        self.ops.append((self.part, tbl, r["_rowid_"], r, None))
        self.say(f"{what}: delete {tbl} {dumps(plain(r))}")
        return True

    def rename(self, rid, old, new, what=None, world=None):
        r = self.one("npcs", {"id": rid}, f"npcs {rid}")
        if world is not None and r["world"] != world:
            raise Refused(f"npcs {rid} stands in world {r['world']}, not {world}")
        return self.set("npcs", r, {"npc_name": (old, new)}, what or f"npcs {rid} {old!r}")

    # ---- the Ascended records (database/ascended.go's rule, tools/ascend_boss_loot.py's SQL)
    def ascended_of(self, base, need):
        db = self.db
        r = db.execute("SELECT patternid FROM itemascended WHERE base = ?", (base,)).fetchone()
        if r:
            return r[0]
        if base in self.asc_made:
            return self.asc_made[base]
        if self.world is None:
            self.world = asc.World(db)
        rec = self.world.rec(base)
        if rec is None or not asc.is_gear(rec):
            raise Refused(f"{need}: base {base} is no gear this shard can make Ascended")
        if self.next_asc is None:
            self.next_asc = db.execute("SELECT COALESCE(MAX(patternid), ?) + 1 FROM itempattern WHERE patternid BETWEEN ? AND ?",
                                       (asc.FIRST_ID - 1, asc.FIRST_ID, asc.LAST_ID)).fetchone()[0]
        nid = self.next_asc
        if nid > asc.LAST_ID:
            raise Refused(f"no id left for an Ascended record ({asc.FIRST_ID}-{asc.LAST_ID})")
        self.next_asc += 1
        vals = dict(self.world.would_be(base), patternid=nid)
        part, self.part = self.part, ASC
        self.ops.append((ASC, "itempattern", None, None, vals))
        self.say(f"make {nid} {vals['itemname']!r} (base {base})")
        for e in rows(db, "itemeffect", {"patternid": base}) if table(db, "itemeffect") else []:
            e = plain(e)
            e["patternid"] = nid
            for c in ("line", "target_line"):
                if c in e:
                    e[c] = asc.scaled_line(e[c])
            self.ops.append((ASC, "itemeffect", None, None, e))
        self.ops.append((ASC, "itemascended", None, None,
                         {"patternid": nid, "base": base, "made": now(), "source": TOOL}))
        self.part = part
        self.asc_made[base] = nid
        return nid


# ---------------------------------------------------------------------------------------------- loot mirror

def item_name(db, pid):
    r = db.execute("SELECT itemname FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
    return None if r is None else r[0]


def map_record(p, pid, name, need):
    """This shard's record of a source record: the same id under the same name, else the one record of the name."""
    if item_name(p.db, pid) == name:
        return pid
    found = [r[0] for r in p.db.execute("SELECT patternid FROM itempattern WHERE itemname = ?", (name,))]
    if len(found) != 1:
        raise Refused(f"{need}: {len(found)} records named {name!r} on this shard")
    return found[0]


def mirror_table(p, tname, binder, entry):
    """loottables tname as the source has it, mapped to this shard's records; bound by npcloot (binder) already."""
    src, db = p.src, p.db
    if not rows(db, "npcloot", {"npc_name": binder, "tablename": tname}):
        raise Refused(f"npcloot ({binder!r}, {tname!r}) is not on this shard: binding it is not this part's")
    srows = src.execute("SELECT patternid, weight, minlevel, maxlevel, source FROM loottables WHERE tablename = ? "
                        "ORDER BY id", (tname,)).fetchall()
    if not srows:
        raise Refused(f"the source has no table {tname!r}")
    want = []
    for pid, weight, lo, hi, source in srows:
        need = f"{tname!r} item {pid}"
        b = src.execute("SELECT base FROM itemascended WHERE patternid = ?", (pid,)).fetchone()
        if b:
            bname = item_name(src, b[0])
            tid = p.ascended_of(map_record(p, b[0], bname, need), need)
        else:
            tid = map_record(p, pid, item_name(src, pid), need)
        want.append((tid, weight, lo, hi, source))
    have = [(r["patternid"], r["weight"], r["minlevel"], r["maxlevel"]) for r in rows(db, "loottables", {"tablename": tname})]
    left = list(have)
    adds = []
    for w in want:
        if w[:4] in left:
            left.remove(w[:4])
        else:
            adds.append(w)
    if left:
        raise Refused(f"{tname!r} on this shard has rows the source has not: {left}")
    for tid, weight, lo, hi, source in adds:
        p.ops.append((p.part, "loottables", None, None, {"tablename": tname, "patternid": tid, "weight": weight,
                                                          "minlevel": lo, "maxlevel": hi,
                                                          "source": f"{source} (Heirloom's, work board {entry})"}))
    p.say(f"{tname!r}: {len(want)} rows as the source, {len(want) - len(adds)} here, {len(adds)} to insert")


def part_epic(p, tname, binder):
    mirror_table(p, tname, binder, p.part)


# ---------------------------------------------------------------------------------------------- the parts

def part_668(p):
    r = p.one("npcs", {"npc_name": "Grilmesh Frostcackle", "world": 0}, "Grilmesh Frostcackle")
    p.set("npcs", r, {"npc_type": (0, 128)}, "Grilmesh Frostcackle a friendly NPC")
    mirror_table(p, "boss snowgrin", "snowgrin", "668")


LC_OLD = ("Acolyte Satyr", "Ancient Ratwere", "Ancient Weregator", "Arch Familiar", "Arclight hydra", "Atesh-Ah embalmer",
          "Atesh-Ah fisher", "Atesh-Ah guard", "Atesh-Ah ritualist", "Atesh-Ah scout", "Atesh-Ah soldier",
          "Atesh-Ah spiritist", "Atesh-Ah warrior", "Azzroth Tzk", "Blackhide rhinoceros", "Bordag", "Bouncer Baka",
          "Bouncer Bonk", "Bouncer Bozlum", "Bouncer Glapma", "Bouncer Gronk", "Bouncer Klob", "Bouncer Klunk",
          "Bouncer Lork", "Bouncer Mildrag", "Bouncer Mooglan", "Bouncer Pulp", "Bouncer Ruksa", "Bouncer Smush",
          "Bouncer Spak", "Bouncer Tigma", "Bouncer Trurg", "Bouncer Uglug", "Bouncer Ungra", "Captain Marglar",
          "Cawtou Corruptor", "Cawtou Windstrider", "Cawtou archer", "Cawtou aviak", "Cawtou myrmidon",
          "Cawtou spiritist", "Crazy Contraption", "Cyclops Visionary", "Darkswamp piranha", "Diseased Tempest Panthe",
          "Dooga", "Dwarven Thug", "Elder Ratwere", "Elder Weregator", "Elder Wolfwere", "Envaric skeleton",
          "Envaric zombie", "Farstone miner", "Fetid Hydra", "Fishmaster Ratman", "Fuga", "Gigantic Skeletal",
          "Goldfeather raider", "Gorvak One-Eye", "Grasten Bootstrutter", "Guard Lando", "Haunting Death",
          "Head Foreman Wade", "Hingedback tortoise", "Hollowhorn", "Hunter Garrick", "Ironhorn rhinoceros", "Ithwutet",
          "Jal'Raeth champion", "Jal'Raeth channeler", "Jal'Raeth knight", "Jal'Raeth noble", "Kappa Tunneler",
          "Kejekan Ghazi", "Kejekan Ghulam", "Kejekan Ispusar", "Kejekan Pasdar", "Kelzahaz", "Kharatok of Cawtou",
          "Klug", "Leatherback turtle", "Lizardman Dreg", "Master Craftsman Johns", "Menkafex", "Murkwood hog",
          "Murkwood spider", "Nazfexah", "Neskar the Deep", "Octoravager", "Old Frostjaw", "Pestiferous Spider Quee",
          "Pestiferous spider", "Plagued Hydra", "Rabid Timberwolf", "Ragna", "Rancid Murkwood", "Ratman Trickster",
          "Rax", "Red Swarming Death", "Rimeheart", "Saftemoot", "Servant of Tal'Thir", "Splynter",
          "Stoneshell Seaturtle", "Swarming Death", "Tae Ew Weaponmaker", "Tae Ew warder", "Target",
          "Tendora-Buki Ghulam", "Tendora-Buki Ispusar", "Tendora-Buki Mamluk", "Tendora-Buki Pasdar",
          "Tendora-Buki Puzhal", "Tendora-Buki Spahi", "Tendora-Buki amir", "Tendora-Buki fisherman",
          "Ti'Rizeth artisan", "Ti'Rizeth guardian", "Trawn Nopsi", "Vineweb recluse", "Waca", "Welahi-nai Ademzada",
          "Welahi-nai Mujahed", "Welahi-nai Puzhal", "Welahi-nai Spahi", "Whitebolt hydra", "Xexhariz", "Xinnalanagus")


def part_1832(p):
    twins, n = [], 0
    for name in LC_OLD:
        have = rows(p.db, "lootcreature", {"npc_name": name})
        if not have:
            continue
        if rows(p.db, "lootcreature", {"npc_name": name.lower()}):
            twins.append(name.lower())
            continue
        p.ops.append((p.part, "lootcreature", have[0]["_rowid_"], have[0], {"npc_name": name.lower()}))
        n += 1
    if twins:
        raise Refused(f"lower-case twins exist (nothing changed): {twins}")
    p.say(f"{n} of the {len(LC_OLD)} keys lower-cased (the rest already are, or this shard lacks the name)")
    left = p.db.execute("SELECT count(*) FROM lootcreature WHERE npc_name <> lower(trim(npc_name))").fetchone()[0] - n
    p.say(f"{left} capitalised keys left after it (entry 1832 step 2's read)")


GUARDIAN_OLD = ('[{"on":"done","do":"popup","text":"The Guardian scents Cazic\'s Scent on you. The golem stands aside '
                'and the Estate\'s door opens."},{"on":"done","do":"teleport","world":0,"x":25712.0,"y":49.5,"z":3222.0}]')
GUARDIAN_NEW = GUARDIAN_OLD.replace('"x":25712.0,"y":49.5,"z":3222.0', '"x":25715.5,"y":49.0,"z":3229.5')


def part_656(p):
    for rid in (118859, 118860):
        r = p.one("npcs", {"id": rid, "npc_name": "The Guardian"}, f"The Guardian {rid}")
        p.set("npcs", r, {"npc_type": (128, 0)}, f"The Guardian {rid} attackable")
    st = p.one("queststeps", {"questid": 101311, "step": 5})
    p.set("queststeps", st, {"events": (GUARDIAN_OLD, GUARDIAN_NEW)}, "101311 step 5 teleport on the doorway floor")


def part_1764(p):
    r = p.one("npcs", {"npc_name": "Slagbreath"}, "Slagbreath")
    p.set("npcs", r, {"npc_type": (128, 0)}, f"Slagbreath {r['id']} attackable")


SAMWE_STEPS = ((100312, 1), (100340, 1), (100370, 2), (100372, 1), (100320, 2), (100330, 1), (100350, 2), (100360, 2),
               (100362, 1))


def part_1819(p):
    p.one("npcs", {"id": 106253, "npc_name": "Merchant Samwe"}, "Merchant Samwe 106253 (entry 867 section 1 first)")
    for q, s in SAMWE_STEPS:
        st = p.one("queststeps", {"questid": q, "step": s})
        if st["target"] != "Merchant Samwe":
            raise Refused(f"queststeps {q}/{s} target {st['target']!r}: apply entry 867 section 1 first")
    if item_name(p.db, 700368) != "Small Gear Wheel":
        raise Refused("700368 is not the Small Gear Wheel here")
    have = rows(p.db, "npcitem", {"npcid": 106253, "patternid": 700368})
    if have:
        p.say("Samwe sells 700368 already")
        return
    nxt = p.db.execute("SELECT COALESCE(MAX(listnumber), -1) + 1 FROM npcitem WHERE npcid = 106253").fetchone()[0]
    p.add("npcitem", {"npcid": 106253, "stackleft": 1, "remainHP": 0, "remaincharge": 0, "patternid": 700368,
                      "equiploc": -1, "location": 1, "listnumber": nxt}, ("npcid", "patternid"),
          "Samwe sells the Small Gear Wheel (its own itemcost)")


def part_709(p):
    rs = rows(p.db, "npcs", {"npc_name": "Sarek the Betrayer"})
    if len(rs) != 1 or rs[0]["world"] != 1:
        raise Refused(f"Sarek the Betrayer: {len(rs)} rows {[(r['id'], r['world']) for r in rs]}: stop and tell jibrish")
    s = rs[0]
    if not p.db.execute("SELECT 1 FROM factions WHERE name = 'Sarek the Betrayer'").fetchone():
        raise Refused("no faction 'Sarek the Betrayer'")
    p.add("factionhits", {"npc_name": "Sarek the Betrayer", "faction": "Sarek the Betrayer", "delta": -505,
                          "source": "entry 709 (board 1818)"}, ("npc_name", "faction"), "his kill hit -505")
    rr = rows(p.db, "npcrespawn", {"npc_row": s["id"]})
    if p.shard == "live":
        note = ("Sarek the Betrayer, the random spawn: rare, 3 days on Live (23 h on Heirloom). jibrish, 2026-11-20; "
                "was petfix259's named 90 s")
        if rr:
            p.set("npcrespawn", rr[0], {"seconds": (90, 259200), "note": (rr[0]["note"], note),
                                        "source": (rr[0]["source"], "board 1820")}, f"respawn of {s['id']}")
        else:
            p.add("npcrespawn", {"npc_row": s["id"], "seconds": 259200, "note": note, "source": "board 1820"},
                  ("npc_row",), f"respawn of {s['id']}")
    else:
        p.say(f"respawn of {s['id']}: {rr[0]['seconds'] if rr else 'none'} (Heirloom keeps 1820's 82,800 s)")
    # board #190 asks how he is set up on Live: read for the reply
    t = p.db.execute("SELECT n.template, c.hp_pct, c.special, c.special_damage, c.resist_kind, c.resist_pct FROM npctemplate n "
                     "LEFT JOIN classtemplates c ON c.name = n.template WHERE n.name = 'sarek the betrayer'").fetchone()
    loot = p.db.execute("SELECT tablename, chance FROM npcloot WHERE npc_name = 'sarek the betrayer'").fetchall()
    st = p.db.execute("SELECT q.enabled, s.events FROM queststeps s JOIN quests q ON q.questid = s.questid "
                      "WHERE s.questid = 101029 AND s.step = 19").fetchone()
    arch = p.db.execute("SELECT id, world, x_coord, y_coord, z_coord, npc_level, npc_type FROM npcs "
                        "WHERE npc_name = 'Archon Seradesh' ORDER BY id").fetchall()
    held = p.db.execute("SELECT step, done, count(*) FROM charquests WHERE questid = 101029 GROUP BY 1, 2").fetchall()
    p.say(f"setup: npcs {s['id']} world {s['world']} ({s['x_coord']:.2f}, {s['y_coord']:.2f}, {s['z_coord']:.2f}) "
          f"L{s['npc_level']} npc_type {s['npc_type']}; template {t}; npcloot {loot or 'none'}; respawn "
          f"{rr[0]['seconds'] if rr else 'none'}; 101029 step 19 (enabled, events) {st}; Archon Seradesh {arch}; "
          f"101029 holders (step, done, n) {held or 'none'}")


def part_1718(p):
    rs = rows(p.db, "npcs", {"npc_name": "High Guard Ianant"})
    if not rs:
        raise Refused("no High Guard Ianant")
    for r in rs:
        p.set("npcs", r, {"npc_level": (37, 38)}, f"High Guard Ianant {r['id']} at "
              f"({r['x_coord']:.1f}, {r['y_coord']:.1f}, {r['z_coord']:.1f}): level only")


def part_1763(p):
    s1 = p.one("queststeps", {"questid": 100600, "step": 1})
    s2 = p.one("queststeps", {"questid": 100600, "step": 2})
    if (s1["target"], s2["target"]) != ("Smithy Wella", "Merchant Haster"):
        raise Refused(f"100600 steps 1-2 talk to {s1['target']!r}, {s2['target']!r}")
    p.set("queststeps", s1, {"rewarditems": ("8318", ""), "journal": (
        "Get iron ore from Smithy Wella near the guardhouse in Qeynos, south of the Office of Qeynos.",
        "Buy iron ore from Smithy Wella near the guardhouse in Qeynos, south of the Office of Qeynos.")},
          "100600 step 1: iron ore bought")
    p.set("queststeps", s2, {"rewarditems": ("700572", ""), "journal": (
        "Get ivory from Merchant Haster in the Qeynos Marketplace.",
        "Buy ivory from Merchant Haster in the Qeynos Marketplace.")}, "100600 step 2: ivory bought")


def part_1785(p):
    st = p.one("queststeps", {"questid": 101073, "step": 3})
    p.set("queststeps", st, {"xp": (73000000, 100000000)}, "101073 step 3 xp")


def effect_json(p, sid):
    r = p.one("spelleffect", {"spellid": sid})
    s = r["effects"]
    if s and jdump(json.loads(s)) != s:
        raise Refused(f"spelleffect {sid}: its effects JSON does not read back as written")
    return r, (json.loads(s) if s else None)


def part_1767(p):
    r, e = effect_json(p, 701115)
    if e["effects"][0]["scale_pct"] in (15, 20):
        e["effects"][0]["scale_pct"] = 20
    p.set("spelleffect", r, {
        "line": ("Target, Decrease Hitpoints by 1368 ; Increase 15% based on DEX",
                 "Target, Decrease Hitpoints by 1368 ; Increase 20% based on DEX"),
        "effects": (r["effects"], jdump(e))}, "Call Lightning 20% DEX")
    r = p.one("spelleffect", {"spellid": 701116})
    p.set("spelleffect", r, {"line": ("Target, Charm ; Increase 25% based on CHA, 2min",
                                      "Target, Charm up to level 53 ; Increase 25% based on CHA, 2min")},
          "Captivate Animal up to level 53 (the cap read from the words)")
    r, e = effect_json(p, 701257)
    if e.get("radius") in (20, 15):
        e["radius"] = 15
    p.set("spelleffect", r, {"effects": (r["effects"], jdump(e))}, "Wild Growth radius 15")


def part_1768(p):
    for sid, name in ((900488, "Influence"), (900487, "Divinity"), (900491, "Radiance")):
        r = p.one("spellpattern", {"spellid": sid, "spellname": name}, name)
        p.set("spellpattern", r, {"range": (16880, 16544)}, f"{name} {sid} range 5.0")


DRAIN_TEMPLATE_ID = 901444


def part_1769(p):
    r, e = effect_json(p, 901501)
    if e["effects"][0]["scale_pct"] in (25, 20):
        e["effects"][0]["scale_pct"] = 20
    p.set("spelleffect", r, {
        "line": ("Target, Decrease Hitpoints by 1250 ; Increase 25% based on STR",
                 "Target, Decrease Hitpoints by 1250 ; Increase 20% based on STR"),
        "effects": (r["effects"], jdump(e))}, "Curse of Bertoxxulous 20% STR")
    lvl = p.one("spellpattern", {"spellid": 700273})["abilitylvl"]
    _, t = effect_json(p, DRAIN_TEMPLATE_ID)
    t["level"] = lvl
    t["effects"][0]["scale_pct"] = 20
    r = p.one("spelleffect", {"spellid": 700273})
    p.set("spelleffect", r, {
        "line": ("Target, Drain HP by 162 ; Increase 10% based on STR", "Target, Drain HP by 162 ; Increase 20% based on STR"),
        "effects": ("", jdump(t))}, "Cursed Touch 20% STR, effects as 901444's")


CALTROPS_HIT = {"id": "05EC", "type": "Dmg/Heal", "kind": "hp", "amount": -15, "resist": "pierce"}


def part_1770(p):
    r, e = effect_json(p, 700027)
    first = e["effects"][0]
    if first.get("kind") == "hp_tick" and first.get("amount") == -5:
        e["effects"][0] = CALTROPS_HIT
    note = "; work board 1770: one direct hit (SOE 2006-04-28), 15 a stand-in (the DoT's total)"
    p.set("spelleffect", r, {
        "line": ("Target, Decrease HP per tic by 5, Decrease Movement Speed by 50, 18sec",
                 "Target, Decrease Hitpoints by 15, Decrease Movement Speed by 50, 18sec"),
        "effects": (r["effects"], jdump(e)),
        "source": (r["source"], r["source"] if r["source"].endswith(note) else r["source"] + note)},
          "Caltrops one hit of 15, snare kept")


CONTRAPTION_POWER = {"id": "0030", "type": "Dmg/Heal", "kind": "power", "amount": 50, "level_pct": 300, "duration_s": 24,
                     "scope": "group", "min_level": 14}


def part_1771(p):
    for sid in (900044, 900086, 900110):
        r = p.one("spellpattern", {"spellid": sid})
        if not r["spellname"].startswith("Crazy Contraption"):
            raise Refused(f"spellpattern {sid} is {r['spellname']!r}")
        p.set("spellpattern", r, {"recast": (300, 3600)}, f"{r['spellname']} recast 1 hour")
    r, e = effect_json(p, 900045)
    if not any(x.get("kind") == "power" for x in e["effects"]):
        e["effects"].insert(1, CONTRAPTION_POWER)
    note = "; work board 1771: group power 50 a stand-in (Contraption 2's)"
    p.set("spelleffect", r, {
        "line": ("Pet, Decrease Hitpoints by 20000, Increase HP per tic by 50, 24sec",
                 "Pet, Decrease Hitpoints by 20000, Increase Power by 50, Increase HP per tic by 50, 24sec"),
        "effects": (r["effects"], jdump(e)),
        "source": (r["source"], r["source"] if r["source"].endswith(note) else r["source"] + note)},
          "Destroy Contraption 1 returns power")


SACRED_HEART = ('{"v":1,"scope":"self","target_type":"Self","harmful":false,"duration_s":90,"stack":2,"level":51,'
                '"min_level":30,"req_equipment":"Any","sheet_type":"Defensive Proc","effects":[{"id":"0518","type":'
                '"Defensive Proc","kind":"def_proc","procs":[{"id":"0D56","type":"Dmg/Heal","kind":"hp_tick","amount":250,'
                '"scale":"cha","scale_pct":150,"on":"defense","scope":"self","target_type":"Self","duration_s":18}]}]}')
SACRED_LINE = ("Self, Add Defensive Proc: Increase HP per tic by 250 ; Increase 150% based on CHA to Self when hit, "
               "90sec")


def part_1774(p):
    r = p.one("spellpattern", {"spellid": 701066, "spellname": "Sacred Heart"}, "Sacred Heart")
    p.set("spellpattern", r, {"scope": (2, 0), "recast": (1800, 900)}, "Sacred Heart self, recast 900")
    r, e = effect_json(p, 701066)
    if r["effects"] != SACRED_HEART and not (e and e.get("scope") == "group" and e.get("duration_s") == 300):
        raise Refused(f"spelleffect 701066 is not the group proc read on 2026-11-19: {r['effects'][:80]}...")
    p.set("spelleffect", r, {"line": ("Group, Add Offensive Proc: Increase Hitpoints by 5 to Self  Increase 150% based "
                                      "on CHA when it hits, Radius 30, 5min", SACRED_LINE),
                             "effects": (r["effects"], SACRED_HEART)}, "Sacred Heart a defensive-proc HoT")


def part_640(p):
    p.one("npcs", {"npc_name": "Qual'Tin"}, "Qual'Tin")
    if rows(p.db, "npcaggro", {"npc_name": "qual'tin"}):
        raise Refused("an npcaggro row 'qual'tin' (lower case) is there")
    p.add("npcaggro", {"npc_name": "Qual'Tin", "aggressive": 0, "radius": 0,
                       "note": "not KoS upon entering the room (forumotion t196, work board 640)",
                       "source": "workboard 640"}, ("npc_name",), "Qual'Tin not KoS")
    t = p.one("npctemplate", {"name": "qual'tin"})["template"]
    if t != "iod qual'tin":
        raise Refused(f"Qual'Tin's template is {t!r}")
    c = p.one("classtemplates", {"name": t})
    p.set("classtemplates", c, {"resist_kind": ("LR,AR,DR,PR", ""), "resist_pct": (50, 0)},
          "his own resists cleared (t281 is the players' list)")
    h = p.one("factionhits", {"npc_name": "Qual'Tin", "faction": "Citizens of Graumord"})
    p.set("factionhits", h, {"delta": (5, 11)}, "Graumord +11 a kill")
    p.say("held: the 3,500 AoE and 200k (question 1), loot (2), the L63 copies (3); 'Denizens -11' names no faction; "
          "the step 5 popup and the 6,500 kick at 20 s / 120 s are there already")


ROBE_TABLE = "rare spirit makers robe"


def part_1599(p):
    if p.shard != "heirloom":
        p.say("Heirloom only (entry 1599): nothing here")
        return
    if not rows(p.db, "loottables", {"tablename": ROBE_TABLE, "patternid": 5209}):
        raise Refused(f"no table {ROBE_TABLE!r} with 5209")
    if rows(p.db, "npcloot", {"npc_name": "elder spirit", "tablename": ROBE_TABLE}):
        raise Refused("elder spirit is still bound to the robe by name (robes5117's row): not as the entry read")
    for rid in (190044, 190045):
        p.one("npcs", {"id": rid, "npc_name": "elder spirit", "world": 2}, f"elder spirit {rid}")
        p.add("npcloot", {"npc_name": f"#{rid}", "tablename": ROBE_TABLE, "chance": 0.1, "rolls": 1,
                          "note": "jibrish 2026-11-17: the Tower of Phantoms elder spirit only, 0.1%",
                          "source": "workboard 1599"}, ("npc_name", "tablename"), f"#{rid} rolls the robe at 0.1%")
    p.drop("lootrate", {"preset": "jib", "profile": "xp", "category": "spirit makers robe"},
           {"rate": 0.001, "source": "workboard 1598"}, "1598's lootrate stopgap")
    p.drop("lootpool", {"id": 12158}, {"category": "spirit makers robe", "npc_name": "elder spirit", "patternid": 5209,
                                       "source": "workboard 1598"}, "1598's lootpool 12158")
    others = [r[0] for r in p.db.execute("SELECT npc_name FROM npcloot WHERE tablename = ? AND npc_name NOT IN "
                                         "('#190044', '#190045')", (ROBE_TABLE,))]
    p.say(f"the robe's table is bound to these too, left as they are: {others or 'none'} (Himlar 1%: entry 1600)")


RATIONS = {8391: "tundra rations", 8392: "travel cuisine", 8393: "mine rations", 8394: "trail rations",
           8395: "plains rations", 8396: "Gnomish Ration", 8397: "Thicket Rations", 8399: "jungle rations",
           26435: "toad snacks"}
RACE_RATION = {1: 8394, 2: 8392, 3: 8396, 4: 8393, 5: 26435, 6: 8391, 7: 8397, 8: 8395, 9: 8399}
START_IDS = {1: (44, 46, 49, 50, 53, 55, 56, 58, 60, 63), 2: (64, 66, 68, 71, 72, 74, 76, 78, 81),
             3: (82, 84, 87, 88, 90, 93, 94, 97), 4: (98, 100, 102, 105), 5: (106, 109, 111), 6: (112, 114, 116),
             7: (118, 120, 123, 125), 8: (126, 128, 131, 132, 134, 137, 138, 141), 9: (142, 144, 146, 149)}
FREEPORT_IDS = (2, 10, 13, 17, 23, 27, 31, 33, 36, 41)


def part_1625(p):
    for pid, name in RATIONS.items():
        r = p.one("itempattern", {"patternid": pid, "itemname": name}, name)
        p.set("itempattern", r, {"maxstack": (999, 20)}, f"{name} stacks to 20")
    for race, ids in START_IDS.items():
        n = 0
        for i in ids:
            r = p.one("startitems", {"id": i, "race": race, "patternid": RACE_RATION[race]}, f"startitems {i}")
            n += p.set("startitems", r, {"count": (99, 20)}, f"startitems {i}") or 0
    for i in FREEPORT_IDS:
        r = p.one("startitems", {"id": i, "race": 0, "humtype": 1}, f"startitems {i} (Freeport human)")
        if r["count"] != 20:
            raise Refused(f"startitems {i} count {r['count']}, not 20")
        p.set("startitems", r, {"patternid": (8398, 8390)}, f"startitems {i} desert rations")
    r = p.one("itempattern", {"patternid": 8390, "itemname": "desert rations"}, "desert rations")
    p.set("itempattern", r, {"levelreq": (0, 1), "procanim": (0, 2), "raceuse": (0, 1023), "classuse": (0, 32767)},
          "desert rations eatable")
    added = rows(p.db, "startitems", {"count": 99})
    added = [r for r in added if r["race"] >= 11 and r["patternid"] in RATIONS]
    for r in added:
        p.ops.append((p.part, "startitems", r["_rowid_"], r, {"count": 20}))
    p.say(f"the added races' {len(added)} ration rows at 99 -> 20")


LEGGINGS = (100129, 100139, 100149, 100159, 100475)


def part_1047(p):
    for q in LEGGINGS:
        s2 = p.one("queststeps", {"questid": q, "step": 2, "target": "sidewinder"}, f"{q}/2")
        s3 = p.one("queststeps", {"questid": q, "step": 3, "target": "sand skipper"}, f"{q}/3")
        p.set("queststeps", s2, {"chance": (0.5, 0.18)}, f"{q}/2 sidewinder skin")
        p.set("queststeps", s3, {"chance": (0.5, 0.08)}, f"{q}/3 sand skipper carapace")
        p.add("queststeptargets", {"questid": q, "step": 3, "target": "crab"}, ("questid", "step", "target"),
              f"{q}/3 counts the beach crabs")


def part_1060(p):
    if item_name(p.db, 5499) != "Ash Covered Shortsword":
        raise Refused("5499 is not the Ash Covered Shortsword here")
    p.add("loottables", {"tablename": "rotbone skeleton", "patternid": 5499, "weight": 1, "minlevel": 1, "maxlevel": 100,
                         "source": "workboard 1060"}, ("tablename", "patternid"), "table 'rotbone skeleton'")
    p.add("npcloot", {"npc_name": "rotbone skeleton", "tablename": "rotbone skeleton", "chance": 2, "rolls": 1,
                      "note": "Allakhazam beast 1393: Known Drops Ash Covered Shortsword; 2% ours (work board 1060)",
                      "source": "workboard 1060"}, ("npc_name", "tablename"), "rotbone skeletons roll it at 2%")


RATTLER = {11: 8, 12: 8, 13: 9, 14: 10, 15: 10}


def part_1066(p):
    rs = rows(p.db, "npcs", {"npc_name": "young rattler"})
    if len(rs) != 39:
        raise Refused(f"{len(rs)} young rattlers, not 39")
    n = 0
    for r in rs:
        if r["npc_level"] in RATTLER.values() and r["npc_level"] not in RATTLER:
            continue
        if r["npc_level"] not in RATTLER:
            raise Refused(f"young rattler {r['id']} at L{r['npc_level']}")
        p.ops.append((p.part, "npcs", r["_rowid_"], r, {"npc_level": RATTLER[r["npc_level"]]}))
        n += 1
    p.say(f"young rattlers: {n} of 39 to L8-10")
    r = p.one("npcs", {"id": 115887}, "115887")
    p.set("npcs", r, {"npc_name": ("cooperhead", "copperhead")}, "115887 copperhead")


def part_992(p):
    r = p.one("npcs", {"id": 2040}, "2040")
    p.set("npcs", r, {"npc_name": ("Nasehir Sandcaster", "Nasehir sandcaster"), "npc_level": (15, 14)},
          "Nasehir sandcaster L14")


def part_964(p):
    p.rename(100518, "Fereal Spirit", "Feral Spirit", world=0)
    p.drop("npcclass", {"name": "fereal spirit"}, {"class": 0}, "npcclass 'fereal spirit'")
    p.drop("npctemplate", {"name": "fereal spirit"}, {"template": "roamer"}, "npctemplate 'fereal spirit'")
    p.drop("lootcreature", {"npc_name": "fereal spirit"}, {"region": "desert"}, "lootcreature 'fereal spirit'")
    p.drop("lootpool", {"id": 5566}, {"npc_name": "fereal spirit", "category": "trash", "patternid": 707939}, "lootpool 5566")
    p.drop("lootpool", {"id": 5567}, {"npc_name": "fereal spirit", "category": "trash", "patternid": 707670}, "lootpool 5567")


def part_1014(p):
    p.one("npcs", {"id": 122601, "npc_name": "Ziliq"}, "Ziliq 122601 (the one kept)")
    for t in ("npcitem", "npcpaths", "campspawns"):
        if table(p.db, t) and rows(p.db, t, {"npcid": 108621}):
            raise Refused(f"{t} names npcs 108621")
    zc = rows(p.db, "npcclass", {"name": "zilq"})
    if zc:
        if rows(p.db, "npcclass", {"name": "ziliq"}):
            raise Refused("npcclass has both zilq and ziliq")
        p.set("npcclass", zc[0], {"name": ("zilq", "ziliq")}, "npcclass zilq -> ziliq")
    else:
        p.say("npcclass zilq: already renamed" if rows(p.db, "npcclass", {"name": "ziliq"}) else "npcclass: no zilq")
    p.drop("lootcreature", {"npc_name": "zilq"}, {"region": "forest"}, "lootcreature zilq")
    p.drop("lootpool", {"id": 11778}, {"npc_name": "zilq", "patternid": 707264}, "lootpool 11778")
    p.drop("lootpool", {"id": 11779}, {"npc_name": "zilq", "patternid": 707265}, "lootpool 11779")
    p.drop("npcrespawn", {"npc_row": 108621}, {"seconds": 90}, "npcrespawn 108621")
    p.drop("npcs", {"id": 108621}, {"npc_name": "Zilq", "world": 0, "npc_level": 29}, "Zilq 108621")
    for rid in (108575, 108578, 108580, 108588):
        p.rename(rid, "decaying minion", "Decaying Minion")


def part_821(p):
    p.rename(105537, "Guard Jorad", "Guard Joradson")


def part_792(p):
    p.rename(106045, "Geographer Leianya", "Cartographer Leianya")


def part_841(p):
    p.rename(105757, "Merchant Moria", "Merchant Moira")
    p.rename(105708, "Shavir Tharian", "Shavir Thanan")
    p.rename(100560, "Stable Boy", "stable boy")
    for hid in (700, 2518):
        h = p.one("factionhits", {"id": hid}, f"factionhits {hid}")
        p.set("factionhits", h, {"npc_name": ("Stable Boy", "stable boy")}, f"factionhits {hid} key")


def part_867(p):
    p.rename(106182, "Merchant Opplefix", "Merchant Opplefiz")


def part_873(p):
    p.rename(104063, "Brunn Coalcarver", "Bruun Coalcarver")
    p.rename(104065, "Armorer Twoforge", "Armorer Twoforger")
    p.rename(106084, "Guardian Bunkel", "Guardian Bukel")
    p.rename(106086, "Guardian Gunja", "Guardian Gunga")


FIREBEETLES = (137108, 137138, 137139, 137140, 137141, 137144, 137145, 137149, 137152, 137156, 137157, 137159, 137160,
               137161, 137162, 137163, 137166, 137177, 137191, 137212, 137216, 137218, 137219, 137222, 137229, 137234,
               137239, 137240, 137241, 137242, 137243, 137244, 137263, 137264, 137268, 137281, 137294, 137331, 137346,
               137355, 137356, 137395, 137401, 137421, 137442, 137446, 137452, 137479, 137512, 137518, 137523, 137526,
               137530, 137548, 137556, 137557, 137570, 137575, 137580, 137586, 137593, 137600, 137619, 137636, 143527,
               144569, 145030, 145031, 148767, 152975, 153049, 154723, 157326, 157838, 160048, 160115, 160300)
FIREBEETLE_BODY, ZOMBIE_BODY = 403102954, 465275276


def part_884(p):
    burk = rows(p.db, "npcs", {"npc_name": "Burk bone"})
    for r in burk:
        if r["world"] != 2 or not 132111 <= r["id"] <= 133856:
            raise Refused(f"Burk bone {r['id']} in world {r['world']}")
        p.ops.append((p.part, "npcs", r["_rowid_"], r, {"npc_name": "murk bone"}))
    p.say(f"Burk bone: {len(burk)} rows -> murk bone")
    for t, c in (("npcclass", "name"), ("npctemplate", "name"), ("lootcreature", "npc_name"), ("npcmove", "name")):
        for k in ("Burk bone", "burk bone"):
            for r in rows(p.db, t, {c: k}):
                if not rows(p.db, t, {c: "murk bone"}):
                    raise Refused(f"{t} has {k!r} and no 'murk bone' row (entry 749 step 32 not in)")
                p.ops.append((p.part, t, r["_rowid_"], r, None))
                p.say(f"delete {t} {k!r} (749 step 32's murk bone row serves)")
    n = 0
    for rid in FIREBEETLES:
        r = p.one("npcs", {"id": rid}, f"npcs {rid}")
        if r["world"] != 0 or r["npc_name"] not in ("fire beetle", "firebeetle"):
            raise Refused(f"npcs {rid}: {r['npc_name']!r} in world {r['world']}")
        new = {}
        if r["npc_name"] != "firebeetle":
            new["npc_name"] = "firebeetle"
        if rid == 137162 and r["modelid"] != FIREBEETLE_BODY:
            if r["modelid"] != ZOMBIE_BODY:
                raise Refused(f"137162 wears {r['modelid']}")
            new["modelid"] = FIREBEETLE_BODY
        if new:
            p.ops.append((p.part, "npcs", r["_rowid_"], r, new))
            n += 1
    p.say(f"firebeetles round Qeynos: {n} of {len(FIREBEETLES)} rows to change")


def part_894(p):
    p.rename(1017, "Costume Vender", "Costume Vendor")
    p.rename(676, "Tailor Fodderhoffen", "Tailor Foderhoffen")


def starts(p, cls, rows_, note):
    for faction, standing in rows_:
        if not p.db.execute("SELECT 1 FROM factions WHERE name = ?", (faction,)).fetchone():
            raise Refused(f"no faction {faction!r}")
        p.add("factionstart", {"race": 5, "class": cls, "humtype": -1, "faction": faction, "standing": standing,
                               "note": note, "source": "video-audit"}, ("race", "class", "humtype", "faction"),
              f"troll class {cls} starts {faction} {standing}")


def part_1117(p):
    starts(p, 3, (("The Dark Ones", 300), ("Undead Frogloks", -2000), ("Citizens of Honjour", 1910)),
           "live: -qeneV5ko70 1172 s")


def part_1129(p):
    starts(p, 8, (("The Dark Ones", 400), ("Undead Frogloks", -2000), ("Citizens of Honjour", 0)),
           "live: Hidq-mXUQw4 1810 s")


BUILD = {
    "670": lambda p: part_epic(p, "epic ancient corpse", "an ancient corpse"),
    "673": lambda p: part_epic(p, "epic cyrcalis", "cyrcalis"),
    "674": lambda p: part_epic(p, "epic graal", "graal the leviathan"),
    "677": lambda p: part_epic(p, "epic boss armour", "teledic"),
    "668": part_668, "1832": part_1832, "656": part_656, "1764": part_1764, "1819": part_1819, "709": part_709,
    "1718": part_1718, "1763": part_1763, "1785": part_1785, "1767": part_1767, "1768": part_1768, "1769": part_1769,
    "1770": part_1770, "1771": part_1771, "1774": part_1774, "640": part_640, "1599": part_1599, "1625": part_1625,
    "1047": part_1047, "1060": part_1060, "1066": part_1066, "992": part_992, "964": part_964, "1014": part_1014,
    "821": part_821, "792": part_792, "841": part_841, "867": part_867, "873": part_873, "884": part_884,
    "894": part_894, "1117": part_1117, "1129": part_1129,
}


# ---------------------------------------------------------------------------------------------- writing

def keyof(db, tbl, row, rid):
    k = pk(db, tbl)
    return {c: row[c] for c in k} if k else {"rowid": rid}


def audit(db, part, action, tbl, key, before, after):
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (now(), ACCOUNT, WHO, VIA, action, tbl, dumps(key), None if before is None else dumps(before),
                      None if after is None else dumps(after), detail(part)))
    return cur.lastrowid


def apply(db, p):
    db.execute(LOG_DDL)
    at, n = now(), 0
    for part, tbl, rid, before, new in p.ops:
        if before is None:
            cs = list(new)
            cur = db.execute(f'INSERT INTO "{tbl}" ({", ".join(chr(34) + c + chr(34) for c in cs)}) '
                             f'VALUES ({", ".join("?" * len(cs))})', [new[c] for c in cs])
            rid = cur.lastrowid
            action = "insert"
        elif new is None:
            if plain(by_rowid(db, tbl, rid)) != plain(before):
                raise Refused(f"{tbl} rowid {rid} changed while planning")
            db.execute(f'DELETE FROM "{tbl}" WHERE rowid = ?', (rid,))
            action = "delete"
        else:
            if plain(by_rowid(db, tbl, rid)) != plain(before):
                raise Refused(f"{tbl} rowid {rid} changed while planning")
            db.execute(f'UPDATE "{tbl}" SET {", ".join(chr(34) + c + chr(34) + " = ?" for c in new)} WHERE rowid = ?',
                       [*new.values(), rid])
            action = "update"
        after = None if new is None else plain(by_rowid(db, tbl, rid))
        b = plain(before)
        key = keyof(db, tbl, after or b, rid)
        a = audit(db, part, action, tbl, key, b, after)
        db.execute(f"INSERT INTO {LOG} (part, tbl, rid, rowkey, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                   (part, tbl, rid, dumps(key), None if b is None else dumps(b), None if after is None else dumps(after),
                    a, at))
        n += 1
    return n


HOLDERS = (("charinventory", "patternid"), ("npcitem", "patternid"), ("loottables", "patternid"),
           ("lootpool", "patternid"), ("npcheld", "patternid"))


def held(db, pid):
    for t, c in HOLDERS:
        if table(db, t) and db.execute(f"SELECT 1 FROM {t} WHERE {c} = ? LIMIT 1", (pid,)).fetchone():
            return t
    return None


def undo(db, parts):
    if not table(db, LOG):
        return 0, []
    back, left, kept = 0, [], set()
    marks = ",".join("?" * len(parts))
    for seq, part, tbl, rid, rowkey, before, after in db.execute(
            f"SELECT seq, part, tbl, rid, rowkey, before, after FROM {LOG} WHERE part IN ({marks}) ORDER BY seq DESC",
            tuple(parts)).fetchall():
        have = plain(by_rowid(db, tbl, rid))
        if (have is None) != (after is None) or (have is not None and dumps(have) != after):
            left.append(f"{part} {tbl} {rowkey}: changed since, left as it is")
            continue
        if part == ASC and before is None and tbl in ("itempattern", "itemeffect", "itemascended"):
            pid = json.loads(after)["patternid"]
            h = pid in kept or held(db, pid)
            if h:
                kept.add(pid)
                left.append(f"{tbl} {pid}: an Ascended record {h if isinstance(h, str) else 'kept'} names, kept")
                continue
        b = None if before is None else json.loads(before)
        if b is None:
            db.execute(f'DELETE FROM "{tbl}" WHERE rowid = ?', (rid,))
            action = "delete"
        elif have is None:
            cs = list(b)
            db.execute(f'INSERT INTO "{tbl}" (rowid, {", ".join(chr(34) + c + chr(34) for c in cs)}) '
                       f'VALUES (?, {", ".join("?" * len(cs))})', [rid, *[b[c] for c in cs]])
            action = "insert"
        else:
            db.execute(f'UPDATE "{tbl}" SET {", ".join(chr(34) + c + chr(34) + " = ?" for c in b)} WHERE rowid = ?',
                       [*b.values(), rid])
            action = "update"
        audit(db, part, action, tbl, json.loads(rowkey), have, b)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if not db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--shard", required=True, choices=("heirloom", "live"))
    ap.add_argument("--source", help="Heirloom's database, read only (parts 668, 670, 673, 674, 677)")
    ap.add_argument("--only", help="comma list of entries (default: every part)")
    ap.add_argument("--skip", help="comma list of entries left out")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    split = lambda s: [x.strip() for x in (s or "").split(",") if x.strip()]  # noqa: E731
    parts = split(a.only) or list(PARTS) + ([ASC] if a.undo else [])
    for x in parts + split(a.skip):
        if x not in PARTS and x != ASC:
            ap.error(f"no part {x}")
    parts = [x for x in parts if x not in split(a.skip)]
    if not a.undo and any(x in NEEDS_SOURCE for x in parts) and not a.source:
        ap.error("parts 668, 670, 673, 674, 677 need --source (Heirloom's database), or --skip them")
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("PRAGMA busy_timeout = 5000")
    db.execute("BEGIN IMMEDIATE")
    src = None
    try:
        if a.undo:
            back, left = undo(db, parts)
            db.execute("COMMIT")
            print(f"undone ({','.join(parts)}): {back} rows put back" + "".join(f"\n  left: {w}" for w in left))
            return 0
        if a.source:
            src = sqlite3.connect(f"file:{a.source}?mode=ro", uri=True)
        p = Plan(db, a.shard, src)
        for x in parts:
            p.part = x
            mark = len(p.ops)
            try:
                BUILD[x](p)
            except Refused as e:
                del p.ops[mark:]
                p.refusals.append(f"{x}: {e}")
                p.lines.append(f"  {x}: REFUSED: {e}")
        print("\n".join(p.lines))
        counts = {}
        for op in p.ops:
            act = "insert" if op[3] is None else "delete" if op[4] is None else "update"
            counts[(op[0], op[1], act)] = counts.get((op[0], op[1], act), 0) + 1
        print("totals: " + (", ".join(f"{k[0]} {k[2]} {k[1]} {v}" for k, v in
                                      sorted(counts.items(), key=lambda kv: (kv[0][0].zfill(8), kv[0][1:]))) or "nothing"))
        if p.refusals:
            db.execute("ROLLBACK")
            print("REFUSED, nothing written (--skip these to apply the rest):\n  " + "\n  ".join(p.refusals))
            return 1
        if not a.apply:
            db.execute("ROLLBACK")
            print("dry run: nothing written (--apply to write)")
            return 0
        if not p.ops:
            db.execute("ROLLBACK")
            print("nothing to write: all in place")
            return 0
        n = apply(db, p)
        db.execute("COMMIT")
        print(f"applied: {n} rows, a gm_audit line each, logged in {LOG}")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise
    finally:
        if src is not None:
            src.close()
        db.close()


if __name__ == "__main__":
    sys.exit(main())
