#!/usr/bin/env python3
"""jibrish's work board asks, batch 2 (the owner approves his asks as submitted): data-only entries, one part an
entry, each run alone with --only / --skip. Every row is found by NAME (or by an id checked against its name) and
checked against what the entry read; a part whose rows are neither as read nor as asked REFUSES (nothing is written).
A part already in place writes nothing, so the same run does each shard (Live's residue of what Heirloom has).

  1783  the 41 'skeleton' rows of the snowfield north-west of Fayspires' lake (107295-107368) -> 'decaying skeleton'
        (the name's rule rows are there; the 'skeleton' rule rows serve the name's other rows and stay).
  1777  Zaoran Plateau: 101313-101318 from level 55; the Liason's 4 weapons +12 Akessan Collective; the first island's
        eight kinds roll their weapon table at 100%.
  1011  root spirit L21; the wailing spirit's Spirit Charm (table 'wailing spirit', 1%).
  929   gnoll strongfangs L23-27; table 'gnoll strongfang' (4 drops, 2%). Kill hits and the two class fixes: in.
  918   talamir wardens L26-30; Talamirs Regiment -10 a kill; table 'talamir warden' (7 drops, 3%); 8251 Cleric Druid
        Shaman (5542, 6058: in).
  917   Blackwater's eight 'fungusman' -> 'elite fungusman' (rule rows moved), L36-40, template 'elite fungusman'
        (roamer with PR 50), 900 s, table (4 drops, 5%), 4724 and 1981 Cleric Druid Shaman.
  898   frosteye shaman 3122 L8 -> 7 (the entry's other rows are in).
  936   aggravated spirit spawn-only, raised by a new reach step 5 of 100786 (5-8 -> 6-9); Tobul wanders 40; Troi
        Respin undead (Svenle: in).
  935   Black Ants -10 on five names; High Guard Ianant spawn-only, raised by two new reach steps 8-9 of 100781 (the
        hive mouth's pop-up, then his tunnel at the spot of 2026-11-19; 8-11 -> 10-13).
  1018  Arcanablade Zerin turns at 101029 step 22; '#123541' spawn-only (his lost spawn's original); 122830 and its
        respawn deleted. Section 4 (123562) is refused if the row is no longer stacked on 123554.
  954   Denizens of Arcadin +1 on seven ratman kinds; 101021 step 8 also off gargoyle soldiers and watchers.
  908   Sylhilthis' Slaves -5 on 11 kappa kinds, Escaped Kappa -5; Klack/Klick 1,800 s, Ryzzich 90 s; Klick/Klack
        class 13 (their roamer template's class still decides how they fight).
  970   the caimans' table (3 drops, 2%); 3405 Damask Robe the four casters, races 271.
  986   coast and road beasts' records: two robes 32656, gloves 17407, the stick 15360/271, the Runetrimmed Robe, the
        sash and ring races 271 (the three slots: in).
  984   gnoll spiritpaw, gnoll shaman, gnoll berserker tables (2%); 3578, 5786, 7069 records (3506 and the berserker
        hits: in).
  933   'cyclops visionary' (rule rows moved); Captain Marglar's three kill hits (the 13 hobgoblin names': in); tables
        'hobgoblin mercenary' 2%, 'cyclops' 1%; 4133 adds Monk (its stats: in); a new trash record 'dented hobgoblin
        ring' in the mercenaries' pool.
  1052  altar guardian spawn-only, raised by a new reach step 3 of 101030 (3-28 -> 4-29); the sea nymph roams 400 on
        land and water; 'indigo artifact smuggler' (rule rows, the loot step); one Khonza Meysla (122837 deleted).
  784   sly ice giant spawn-only, raised at 101272 step 3's start; Creepyman roams 75; 101277 step 3 gives the
        alchemist pants of mischief 7177.
  1256  a 'wolf spider' (a clone of 116991 on the large wolf spider's body) at (17810, 10720) on the ground's floor,
        roaming. Step 2's journal is refused when 100661/1 is no longer as the entry read it.
  865   a night-only 'needle fang' (a clone of 113409) at (14800, 8450) on the ground's floor, its rule rows copied
        from the needlefang spider's, white to all (npcaggro 0), npctime night.
  678   (Live) Lady Vox's Frost Breath 905140: two hits of 357 cold (Heirloom's open-01-678).
  785   (Live) the seven filmed rats by Tethelin's road 100261-100267 -> 'Pestilence Rat'.
  1175  (Live) 133746 'Xinnalagus' -> 'Xinnalanagus' (its rule rows, path and respawn are there).
  1026  Bloodpaw a wolf (122557: body, 1,800 s, class, wolf trash) and the hunt's 'gnoll bloodpaw' (122558-122560,
        15 s, rule rows, the gnoll trash, 101172's target); table 'bloodpaw' 25%.
  1038  'King Xorbb' (rule rows moved), 1,800 s, 'boss xorbb' 25%; The Eye -10 on him and his two thrall kinds.
        Glonkal waits on entry 1037 (goblin prisoner 123004 in his alcove).
  937   the Ankexfen King (a clone of 124120 on the pyramid's top floor, L35, 1,800 s, rule rows); table 'ankexfen
        king' 4915 at 25% and 4915 out of 'ankexfen camp' (7053 stays: entry 815's table is not built); 8 records.
  1048  one silvermist alpha: 108839 and its rule rows deleted, 122576 on the pack's body and spawn-only, raised by a
        new reach step in 100767 (2 -> 3 steps, Jack's belt) and in 100869 (3 -> 4).
  995   the mansion's thrall back (102976 as the shard's own gm_audit holds it, L44, red to all); a second marionette.
        Also entry 1062 section 3.
  892   Paineel's quest 101228: the doubled Naris and agents deleted; Balidos turns at the talk, then gives the papers;
        a pop-up reach before Naris; the agents L63 at half melee; General Dorganun spawn-only L61.
  1043  Roj Eir Sew' Eil spawn-only, raised by a new reach step 3 in the five Real Mark quests; his kill hits; 1531
        Mercury Bead WAR RNG PAL SK MNK BRD ROG.
  1059  Heyokah trial: the Eidolon of Malice L47 and spawn-only, a 'vengeance spirit' row (spawn-only), 100876 step 1
        raises four spirits and the eidolon.
  1046  Vaelian -> Vaelan Wyndsorrow (row, class, faction, 100747's words); the new quest "Helania's Letters" with its
        four notes and letters.

Not here: the Ascended records the entries 902, 903, 1026, 1038 and 937 ask for (tools/ascend_boss_loot.py --boss,
after this tool); the quest files under tools/quests/ (the entries' repository mirrors).

What takes effect when: loot tables, npcloot and lootpool at once; item records at a client's login; npcs (names,
levels, types, bodies, new and deleted rows), npcspawnonly, npcmove, npctime, npcundead, npcclass, npctemplate,
lootcreature and classtemplates at the panel's reload_npcs; quests and quest steps at the panel's quest reload;
factionhits, npcaggro and npcrespawn at the server's next start (or a panel edit of a row of that table).

Dry run by default. --apply writes in one transaction, a gm_audit line a row (accountid 1, dab, via tool), every row's
before and after in jib_batch2_1122_log; a second --apply writes nothing. --undo puts back what the log holds for the
parts named (default all), latest first; a row changed since is left and said.

    python3 -E -s tools/jib_batch2_20261122.py --db eqoa.db --shard heirloom|live
        [--only 1783,1777 | --skip 1018] [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "jib_batch2_1122_log"
WHO, ACCOUNT, VIA = "dab", 1, "tool"
TOOL = "tools/jib_batch2_20261122.py"
SRC = "workboard {}"

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
    return f"jibrish's work board entry {part} (the owner approves his asks as submitted): {TOOL}"


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def dumps(v):
    s = json.dumps(v, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def ev(*events):
    """A step's or quest's events JSON as the rows keep it (key order kept)."""
    return json.dumps(list(events), separators=(",", ":"), ensure_ascii=False)


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
        return abs(float(a) - float(b)) < 1e-6
    return a == b


def item_name(db, pid):
    r = db.execute("SELECT itemname FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
    return None if r is None else r[0]


class Plan:
    def __init__(self, db, shard):
        self.db, self.shard = db, shard
        self.part = ""
        self.ops = []        # (part, tbl, rid or None, before row or None, after values or None)
        self.lines = []
        self.refusals = []
        self.ids = {}        # table -> next id handed out in this plan

    def say(self, s):
        self.lines.append(f"  {self.part}: {s}")

    # ---- reading
    def one(self, tbl, where, what=None):
        r = rows(self.db, tbl, where)
        if len(r) != 1:
            raise Refused(f"{what or tbl} {dumps(where)}: {len(r)} rows, not one")
        return r[0]

    def npc(self, rid, name, what=None):
        r = rows(self.db, "npcs", {"id": rid})
        if len(r) != 1 or r[0]["npc_name"] != name:
            raise Refused(f"{what or 'npcs'} {rid}: {r[0]['npc_name'] if r else 'no row'!r}, not {name!r}")
        return r[0]

    def new_id(self, tbl, col, floor=0):
        if tbl not in self.ids:
            seq = self.db.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (tbl,)).fetchone()
            top = self.db.execute(f'SELECT COALESCE(MAX("{col}"), 0) FROM "{tbl}"').fetchone()[0]
            self.ids[tbl] = max(seq[0] if seq else 0, top, floor - 1) + 1
        n = self.ids[tbl]
        self.ids[tbl] += 1
        return n

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
        self.upd(tbl, row, new)
        self.say(f"{what}: " + ", ".join(f"{c} {row[c]!r} -> {v!r}" for c, v in new.items()))
        return True

    def upd(self, tbl, row, new):
        """An update of a row, merged into this plan's update of the same row when there is one."""
        for i, op in enumerate(self.ops):
            if op[1] == tbl and op[2] == row["_rowid_"] and op[3] is not None and op[4] is not None:
                # merged, and moved to the end: a step's move must come in the order the plan made it
                del self.ops[i]
                self.ops.append((op[0], tbl, op[2], op[3], {**op[4], **new}))
                return
        self.ops.append((self.part, tbl, row["_rowid_"], row, dict(new)))

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

    def rename(self, rid, old, new, world=None):
        r = rows(self.db, "npcs", {"id": rid})
        if len(r) != 1:
            raise Refused(f"npcs {rid}: no row")
        if world is not None and r[0]["world"] != world:
            raise Refused(f"npcs {rid} stands in world {r[0]['world']}, not {world}")
        return self.set("npcs", r[0], {"npc_name": (old, new)}, f"npcs {rid} {old!r}")

    def level(self, rid, name, old, new):
        return self.set("npcs", self.npc(rid, name), {"npc_level": (old, new)}, f"{name} {rid} level")

    def rekey(self, tbl, col, old, new, what=None):
        """A name-keyed rule row moved from old to new: refused when both keys have a row."""
        o, n = rows(self.db, tbl, {col: old}), rows(self.db, tbl, {col: new})
        what = what or f"{tbl} {old!r} -> {new!r}"
        if not o:
            self.say(f"{what}: already moved" if n else f"{what}: no row of either key")
            return False
        if n:
            raise Refused(f"{what}: {tbl} has both keys")
        if len(o) != 1:
            raise Refused(f"{what}: {len(o)} rows of {old!r}")
        return self.set(tbl, o[0], {col: (old, new)}, what)

    def item(self, pid, name, changes):
        r = self.one("itempattern", {"patternid": pid}, f"item {pid}")
        if r["itemname"] not in (name,) + tuple(v[1] for k, v in changes.items() if k == "itemname"):
            raise Refused(f"item {pid} is {r['itemname']!r}, not {name!r}")
        return self.set("itempattern", r, changes, f"{pid} {name}")

    def loot_table(self, tname, items, lo=1, hi=100):
        """loottables tname holding items (patternid -> its name), weight 1: as asked or refused."""
        have = rows(self.db, "loottables", {"tablename": tname})
        want = []
        for pid, name in items:
            if item_name(self.db, pid) != name:
                raise Refused(f"table {tname!r}: {pid} is {item_name(self.db, pid)!r}, not {name!r}")
            want.append((pid, lo, hi))
        got = [(r["patternid"], r["minlevel"], r["maxlevel"]) for r in have]
        if have and sorted(got) == sorted(want):
            self.say(f"table {tname!r}: already as asked ({len(want)} rows)")
            return
        if have:
            if any(g not in want for g in got):
                raise Refused(f"table {tname!r} holds {got}, not only {want}")
        for pid, lo_, hi_ in want:
            if (pid, lo_, hi_) in got:
                continue
            self.ops.append((self.part, "loottables", None, None,
                             {"tablename": tname, "patternid": pid, "weight": 1, "minlevel": lo_, "maxlevel": hi_,
                              "source": SRC.format(self.part)}))
        self.say(f"table {tname!r}: {len(want) - len(got)} rows to insert ({', '.join(str(p) for p, _, _ in want)}; "
                 f"creature levels {lo}-{hi})")

    def bind(self, npc_name, tname, chance, note):
        self.add("npcloot", {"npc_name": npc_name, "tablename": tname, "chance": chance, "rolls": 1, "note": note,
                             "source": SRC.format(self.part)}, ("npc_name", "tablename"),
                 f"npcloot {npc_name!r} -> {tname!r} {chance}%")

    def hit(self, npc_name, faction, delta, source=None):
        if not self.db.execute("SELECT 1 FROM factions WHERE name = ?", (faction,)).fetchone():
            raise Refused(f"no faction {faction!r}")
        self.add("factionhits", {"npc_name": npc_name, "faction": faction, "delta": delta,
                                 "source": source or SRC.format(self.part)}, ("npc_name", "faction"),
                 f"kill hit {npc_name!r} {faction} {delta:+d}")

    def spawnonly(self, name, note):
        k = name.strip().lower()
        self.add("npcspawnonly", {"name": k, "note": note, "source": SRC.format(self.part)}, ("name",),
                 f"spawn-only {k!r}")

    def respawn(self, rid, old, new, note):
        rr = rows(self.db, "npcrespawn", {"npc_row": rid})
        if rr:
            return self.set("npcrespawn", rr[0], {"seconds": (old, new)}, f"respawn of {rid}") and \
                self.set_note(rr[0], note)
        if old is not None:
            raise Refused(f"respawn of {rid}: no row (read {old} s)")
        return self.add("npcrespawn", {"npc_row": rid, "seconds": new, "note": note, "source": SRC.format(self.part)},
                        ("npc_row",), f"respawn of {rid} {new} s")

    def set_note(self, row, note):
        # the respawn row's note follows its new figure, in the same op
        op = self.ops[-1]
        if op[2] == row["_rowid_"] and op[1] == "npcrespawn":
            op[4]["note"] = note
            op[4]["source"] = SRC.format(self.part)
        return True

    def clone_npc(self, src_id, src_name, changes, what):
        """A new npcs row: every column as src_id's but the changes; already there when a row of the new name stands
        within 2 of the spot in the world."""
        s = self.npc(src_id, src_name, f"{what}'s source")
        name, world = changes["npc_name"], changes.get("world", s["world"])
        for r in rows(self.db, "npcs", {"npc_name": name}):
            if r["world"] == world and abs(r["x_coord"] - changes["x_coord"]) < 2 and \
                    abs(r["z_coord"] - changes["z_coord"]) < 2:
                self.say(f"{what}: already there (npcs {r['id']})")
                return r["id"]
        vals = plain(s)
        vals.update(changes)
        vals["id"] = self.new_id("npcs", "id")
        if vals.get("npc_id") == "self":
            vals["npc_id"] = vals["id"]
        self.ops.append((self.part, "npcs", None, None, vals))
        self.say(f"{what}: insert npcs {vals['id']} {name!r} L{vals['npc_level']} world {world} "
                 f"({vals['x_coord']}, {vals['y_coord']}, {vals['z_coord']}), clone of {src_id}")
        return vals["id"]

    def copy_rule(self, tbl, col, src, dst, what, **over):
        s = rows(self.db, tbl, {col: src})
        if len(s) != 1:
            raise Refused(f"{what}: {tbl} {src!r}: {len(s)} rows")
        v = plain(s[0])
        v[col] = dst
        v.update(over)
        if "source" in v and "source" not in over:
            v["source"] = SRC.format(self.part)
        return self.add(tbl, v, (col,), what)

    # ---- quest steps
    def step(self, q, n):
        return self.one("queststeps", {"questid": q, "step": n}, f"quest {q} step {n}")

    def relayout(self, q, old_count, mapping, inserts, what):
        """Quest q's steps 1..old_count moved by mapping (old -> new, never lower) and the new steps of inserts
        ({step: values}) put in, with every table a step number keys and the characters part-way. Already in place
        when every inserted step is there as asked and the count is the new one."""
        st = sorted(rows(self.db, "queststeps", {"questid": q}), key=lambda r: r["step"])
        new_count = old_count + len(inserts)
        if len(st) == new_count and all(
                any(r["step"] == n and all(same(r[c], v) for c, v in vals.items() if c in sig) for r in st)
                for n, (vals, sig) in inserts.items()):
            self.say(f"{what}: already laid out ({new_count} steps)")
            return False
        if [r["step"] for r in st] != list(range(1, old_count + 1)):
            raise Refused(f"{what}: quest {q} has steps {[r['step'] for r in st]}, not 1-{old_count} as read")
        for old in sorted(mapping, reverse=True):
            new = mapping[old]
            if new == old:
                continue
            r = next(x for x in st if x["step"] == old)
            self.upd("queststeps", r, {"step": new})
            for t in ("queststeptargets", "questwalls"):
                if table(self.db, t):
                    for x in rows(self.db, t, {"questid": q, "step": old}):
                        self.upd(t, x, {"step": new})
        moved = 0
        for c in rows(self.db, "charquests", {"questid": q, "done": 0}):
            if c["step"] in mapping and mapping[c["step"]] != c["step"]:
                self.upd("charquests", c, {"step": mapping[c["step"]]})
                moved += 1
        c = cols(self.db, "queststeps")
        for n, (vals, _) in sorted(inserts.items()):
            row = {k: v for k, v in vals.items() if k in c}
            row.update(questid=q, step=n)
            self.ops.append((self.part, "queststeps", None, None, row))
        self.say(f"{what}: {old_count} -> {new_count} steps ({', '.join(f'{o}->{n}' for o, n in sorted(mapping.items()) if o != n)}), "
                 f"new {sorted(inserts)}; {moved} characters part-way moved with their step")
        return True


def reach(world, x, y, z, radius, journal, events):
    v = {"kind": "reach", "target": "", "world": world, "x": x, "y": y, "z": z, "radius": radius, "journal": journal,
         "events": events}
    return v, ("kind", "world", "x", "z", "events")


def mapping(old_count, at, by=1):
    return {s: (s + by if s >= at else s) for s in range(1, old_count + 1)}


# ---------------------------------------------------------------------------------------------- the parts

def part_1783(p):
    rs = [r for r in rows(p.db, "npcs", {"npc_name": "skeleton"}) if 107295 <= r["id"] <= 107368]
    done = [r for r in rows(p.db, "npcs", {"npc_name": "decaying skeleton"}) if 107295 <= r["id"] <= 107368]
    if len(rs) + len(done) != 41:
        raise Refused(f"{len(rs)} 'skeleton' and {len(done)} 'decaying skeleton' rows in 107295-107368, not 41")
    for r in rs:
        if r["world"] != 0 or not (16200 <= r["x_coord"] <= 16900 and 4270 <= r["z_coord"] <= 4680):
            raise Refused(f"skeleton {r['id']} stands at world {r['world']} ({r['x_coord']}, {r['z_coord']})")
        p.ops.append((p.part, "npcs", r["_rowid_"], r, {"npc_name": "decaying skeleton"}))
    for t, c in (("npcclass", "name"), ("npctemplate", "name"), ("lootcreature", "npc_name"), ("npcundead", "name")):
        if not rows(p.db, t, {c: "decaying skeleton"}):
            raise Refused(f"{t} has no 'decaying skeleton' row")
    p.say(f"{len(rs)} of the 41 rows -> 'decaying skeleton' ({len(done)} already); the name's rule rows are there, "
          f"the 'skeleton' rows stay for its other rows")


ZP_QUESTS = (101313, 101314, 101315, 101316, 101317, 101318)
ZP_LOOT = (("honor guard veteran", "zp honor weapons"), ("honor guard spellsmith", "zp honor weapons"),
           ("zaoran guard", "zp zaoran weapons"), ("zaoran spellsmith", "zp zaoran weapons"),
           ("akessan nihilist", "zp akessan weapons"), ("akessan arcanist", "zp akessan weapons"),
           ("akessan destroyer", "zp akessan weapons"), ("akessan adept", "zp akessan weapons"))


def part_1777(p):
    for q in ZP_QUESTS:
        p.set("quests", p.one("quests", {"questid": q}), {"minlevel": (60, 55)}, f"quest {q} from level 55")
    qs = rows(p.db, "quests", {"name": "Weapons for the Liason"})
    if len(qs) != 1:
        raise Refused(f"{len(qs)} quests 'Weapons for the Liason'")
    s = p.step(qs[0]["questid"], 1)
    if s["target"] != "Akessan Liason" or not s["items"].endswith("*4"):
        raise Refused(f"quest {qs[0]['questid']} step 1 is {s['target']!r} {s['items']!r}")
    p.set("queststeps", s, {"factions": ("Akessan Collective:+4", "Akessan Collective:+12")},
          f"quest {qs[0]['questid']} step 1 (the Liason's 4 weapons) +12")
    for name, t in ZP_LOOT:
        r = p.one("npcloot", {"npc_name": name, "tablename": t})
        p.set("npcloot", r, {"chance": (50, 100), "rolls": (1, 1)}, f"{name} -> {t!r} every kill")


def part_1011(p):
    p.level(108503, "root spirit", 20, 21)
    p.npc(108500, "wailing spirit")
    p.loot_table("wailing spirit", ((8116, "Spirit Charm"),))
    p.bind("wailing spirit", "wailing spirit", 1, "Allakhazam beast 1557 Known Drops (item 2516); 1% ours (work board 1011)")


STRONGFANG = {106625: 23, 106636: 23, 106650: 23, 106628: 24, 106637: 24, 106651: 24, 106629: 25, 106638: 25,
              106656: 25, 106630: 26, 106643: 26, 106631: 27, 106649: 27}


def part_929(p):
    for rid, lv in STRONGFANG.items():
        p.level(rid, "gnoll strongfang", 20, lv)
    p.loot_table("gnoll strongfang", ((7796, "Bear Hide War Bracers"), (5555, "Centaur's Lance"),
                                      (5332, "Feathered Wand"), (6148, "Gnollhide Shield")), 19, 27)
    p.bind("gnoll strongfang", "gnoll strongfang", 2, "Allakhazam beast 859 Known Drops; 2% ours (work board 929)")


WARDENS = {107638: 29, 107641: 29, 107645: 29, 107639: 28, 107644: 28, 107640: 27, 107646: 27, 107642: 26,
           107647: 26}


def part_918(p):
    for rid, lv in WARDENS.items():
        p.level(rid, "talamir warden", 30, lv)
    for rid in (107636, 107637, 107643):
        p.level(rid, "talamir warden", 30, 30)
    p.hit("talamir warden", "Talamirs Regiment", -10, "prima-2003")
    p.loot_table("talamir warden", ((5603, "Dirt Encrusted Staff"), (5328, "Earthflow Orb"),
                                    (7889, "Multi-colored Bracers"), (8251, "Petrified Evergreen Ring"),
                                    (6146, "Earthen Shield"), (5542, "Bright Mossy Spear"),
                                    (6058, "Layered Bark Dagger")))
    p.bind("talamir warden", "talamir warden", 3, "Allakhazam beasts 1447, 1463 Known Drops; 3% ours (work board 918)")
    p.item(8251, "Petrified Evergreen Ring", {"classuse": (32767, 896)})
    p.item(5542, "Bright Mossy Spear", {"classuse": (334, 335)})
    p.item(6058, "Layered Bark Dagger", {"classuse": (110, 111)})


FUNGUS = {106730: 36, 106731: 37, 106732: 38, 106733: 39, 106734: 40, 106735: 37, 106736: 38, 106737: 39}


def part_917(p):
    for rid, lv in FUNGUS.items():
        r = rows(p.db, "npcs", {"id": rid})
        if not r or r[0]["npc_name"] not in ("fungusman", "elite fungusman") or r[0]["world"] != 0:
            raise Refused(f"npcs {rid}: {r[0]['npc_name'] if r else 'no row'!r}")
        p.set("npcs", r[0], {"npc_name": ("fungusman", "elite fungusman"), "npc_level": (36, lv)},
              f"npcs {rid} elite fungusman L{lv}")
    left = rows(p.db, "npcs", {"npc_name": "fungusman"})
    if [r["id"] for r in left if r["id"] not in FUNGUS]:
        raise Refused(f"other 'fungusman' rows stand: {[r['id'] for r in left if r['id'] not in FUNGUS]}")
    p.rekey("npctemplate", "name", "fungusman", "elite fungusman")
    p.rekey("npcclass", "name", "fungusman", "elite fungusman")
    p.rekey("lootcreature", "npc_name", "fungusman", "elite fungusman")
    for lid, pid in ((6020, 707718), (6021, 707427)):
        r = p.one("lootpool", {"id": lid})
        if r["patternid"] != pid:
            raise Refused(f"lootpool {lid} holds {r['patternid']}")
        p.set("lootpool", r, {"npc_name": ("fungusman", "elite fungusman")}, f"lootpool {lid}")
    roamer = plain(p.one("classtemplates", {"name": "roamer"}))
    t = dict(roamer, name="elite fungusman", resist_kind="PR", resist_pct=50, resist_fail=0,
             note="Blackwater's elite fungusmen: roamer, strong against poison (Prima p. 667's Strong Against; 50 "
                  "ours, as 'pod roamer'); work board 917", source=SRC.format(p.part))
    have = rows(p.db, "classtemplates", {"name": "elite fungusman"})
    if have:
        if any(not same(have[0][c], t[c]) for c in t if c not in ("note", "source")):
            raise Refused("classtemplates 'elite fungusman' is there and differs")
        p.say("template 'elite fungusman': already there")
    else:
        p.ops.append((p.part, "classtemplates", None, None, t))
        p.say("template 'elite fungusman': insert (roamer, PR 50)")
    tm = rows(p.db, "npctemplate", {"name": "elite fungusman"})
    if tm:
        p.set("npctemplate", tm[0], {"template": ("roamer", "elite fungusman")}, "npctemplate elite fungusman")
    else:  # moved in this plan: the op before holds it
        op = next(o for o in p.ops if o[1] == "npctemplate" and o[4] and o[4].get("name") == "elite fungusman")
        op[4]["template"] = "elite fungusman"
        p.say("npctemplate elite fungusman -> template 'elite fungusman'")
    for rid in FUNGUS:
        p.respawn(rid, None, 900, "elite fungusman: 900 s ours (AZ 2004 'they don't spawn fast'), work board 917")
    p.loot_table("elite fungusman", ((4724, "Beaming Mace"), (1313, "earring of might"), (1981, "Morrowfyre Defender"),
                                     (1301, "morrowtear shield")))
    p.bind("elite fungusman", "elite fungusman", 5, "Allakhazam beast 1659 Known Drops; 5% ours (work board 917)")
    p.item(4724, "Beaming Mace", {"classuse": (1023, 896)})
    p.item(1981, "Morrowfyre Defender", {"classuse": (527, 896)})


def part_898(p):
    p.level(3122, "frosteye shaman", 8, 7)


AGG_EV = ev({"on": "done", "do": "spawn", "npc": "aggravated spirit", "hostile": True})


def part_936(p):
    p.npc(122481, "aggravated spirit")
    p.spawnonly("aggravated spirit", "the spawn of quest 100786 The Restless, raised by its reach step")
    s5 = p.step(100786, 5) if rows(p.db, "queststeps", {"questid": 100786, "step": 5}) else None
    if s5 and s5["kind"] == "loot" and s5["target"] != "aggravated spirit":
        raise Refused(f"100786 step 5 loots {s5['target']!r}")
    p.relayout(100786, 8, mapping(8, 5), {5: reach(0, 13583.0, 54.0, 23626.0, 20.0,
                                                   "Go to the ruins in Widow's Peak where Tobul roams.", AGG_EV)},
               "quest 100786 a reach step 5 raises the aggravated spirit")
    p.npc(113818, "Tobul")
    p.add("npcmove", {"name": "tobul", "roam": 1, "radius": 40.0, "water": "",
                      "note": "'Tobul can be found wandering around here' (forum t109); the radius ours (work board 936)",
                      "source": SRC.format(p.part)}, ("name",), "Tobul wanders 40")
    p.npc(113865, "Troi Respin")
    p.add("npcundead", {"name": "troi respin", "undead": 1,
                        "note": "'Ghost Troi' (forum t111), 'the ghost of Troi Respin' (AZ quest 602); work board 936",
                        "source": SRC.format(p.part)}, ("name",), "Troi Respin undead")


ANTS = ("black ant scout", "black ant carpenter", "black ant leafcutter", "black ant guard", "High Guard Ianant")
TRAP_EV = ev({"on": "done", "do": "popup", "text": "A trap! The colony stirs. Its High Guard knows you have come."})
IANANT_EV = ev({"on": "done", "do": "spawn", "npc": "High Guard Ianant", "hostile": True})


def part_935(p):
    for n in ANTS:
        if not rows(p.db, "npcs", {"npc_name": n}):
            raise Refused(f"no row named {n!r}")
        p.hit(n, "Black Ants", -10, "prima-2003")
    s8 = rows(p.db, "queststeps", {"questid": 100781, "step": 8})
    if s8 and s8[0]["kind"] == "loot" and s8[0]["target"] != "High Guard Ianant":
        raise Refused(f"100781 step 8 is {s8[0]['target']!r}")
    p.spawnonly("High Guard Ianant", "the spawn of quest 100781 Min's Treasure, raised by its reach step")
    p.relayout(100781, 11, mapping(11, 8, 2), {
        8: reach(0, 21066.0, 72.0, 29653.0, 15.0,
                 "Enter the southern ant colony, the great ant hill south of the gypsy camp.", TRAP_EV),
        9: reach(0, 21077.0, -6.8, 29686.0, 15.0,
                 "Go down to the bottom of the mound and into the tunnel right of the ramp.", IANANT_EV)},
               "quest 100781 the hive mouth's pop-up and his tunnel (steps 8-9)")
    for r in rows(p.db, "npcs", {"npc_name": "High Guard Ianant"}):
        p.say(f"High Guard Ianant row {r['id']} at ({r['x_coord']:.1f}, {r['y_coord']:.1f}, {r['z_coord']:.1f}) "
              f"(the reach point: (21077, -6.8, 29686))")


ZERIN_EV = ev({"on": "done", "do": "spawn", "npc": "Arcanablade Zerin", "hostile": True})


def part_1018(p):
    p.npc(123536, "Arcanablade Zerin")
    p.npc(123541, "Arcanablade Zerin")
    s = p.step(101029, 22)
    if (s["kind"], s["target"]) != ("talk", "Arcanablade Zerin"):
        raise Refused(f"101029 step 22 is {s['kind']} {s['target']!r}")
    p.set("queststeps", s, {"events": ("", ZERIN_EV)}, "101029 step 22: he turns at the talk")
    s23 = p.step(101029, 23)
    if (s23["kind"], s23["target"]) != ("loot", "Arcanablade Zerin"):
        raise Refused(f"101029 step 23 is {s23['kind']} {s23['target']!r}")
    p.add("npcspawnonly", {"name": "#123541", "note": "the twin of talker 123536: quest 101029 step 23's lost spawn "
                                                      "comes again within 21 of it", "source": SRC.format(p.part)},
          ("name",), "spawn-only #123541")
    p.drop("npcrespawn", {"npc_row": 122830}, {"seconds": 90}, "npcrespawn 122830")
    p.drop("npcs", {"id": 122830}, {"npc_name": "Arcanablade Zerin", "npc_type": 0, "world": 1}, "npcs 122830")
    a = rows(p.db, "npcs", {"id": 123554})
    b = rows(p.db, "npcs", {"id": 123562})
    if b:
        if not a or any(not same(a[0][c], b[0][c]) for c in ("npc_name", "x_coord", "y_coord", "z_coord", "npc_level",
                                                            "modelid", "world")):
            p.say("section 4: cyclops guard 123562 is no longer stacked on 123554 (moved since the entry's read): "
                  "left, not deleted")
        else:
            p.drop("npcs", {"id": 123562}, {"npc_name": "cyclops guard"}, "the doubled cyclops guard 123562")
    else:
        p.say("section 4: 123562 already gone")


RATMEN = ("ratman scout", "ratman wizard", "ratman warrior", "ratman channeler", "elite ratman", "ratman fighter",
          "ratman scavenger")
STATUE_OLD = "Take a gargoyle statue from the statue guardian in the first main room of the gargoyle fortress."
STATUE_NEW = ("Take a gargoyle statue from the gargoyle soldiers and watchers of the gargoyle fortress, or from its "
              "statue guardian.")


def part_954(p):
    for n in RATMEN:
        p.hit(n, "Denizens of Arcadin", 1, "forum t280 (size ours), work board 954")
    s = p.step(101021, 8)
    if (s["kind"], s["target"]) != ("loot", "statue guardian"):
        raise Refused(f"101021 step 8 is {s['kind']} {s['target']!r}")
    for t in ("gargoyle soldier", "gargoyle watcher"):
        p.add("queststeptargets", {"questid": 101021, "step": 8, "target": t}, ("questid", "step", "target"),
              f"101021 step 8 also off {t}")
    p.set("queststeps", s, {"journal": (STATUE_OLD, STATUE_NEW)}, "101021 step 8 journal")


KAPPA = ("kappa", "kappa drudge", "kappa slave", "kappa worker", "kappa guard", "kappa elite guard", "kappa warrior",
         "kappa wizard", "kappa shaman", "kappa brute", "kappa tunneler")


def part_908(p):
    for n in KAPPA:
        p.hit(n, "Sylhilthis' Slaves", -5, "prima-2003")
    p.hit("escaped kappa", "Escaped Kappa", -5, "prima-2003")
    for rid, name, sec in ((118997, "Klack", 1800), (118998, "Klick", 1800), (118995, "Ryzzich", 90)):
        p.npc(rid, name)
        p.respawn(rid, None, sec, f"{name}: {sec} s ours (work board 908)")
    for n in ("klick", "klack"):
        p.set("npcclass", p.one("npcclass", {"name": n}), {"class": (0, 13)}, f"npcclass {n} wizard")


def part_970(p):
    p.loot_table("caiman", ((3386, "Bronze Circlet"), (3405, "Damask Robe"), (3955, "Tattered Harness")))
    p.bind("caiman", "caiman", 2, "Allakhazam beast 853 Known Drops and a 2003 comment; 2% ours (work board 970)")
    p.item(3405, "Damask Robe", {"classuse": (32640, 15360), "raceuse": (1023, 271)})


def part_986(p):
    p.item(5089, "Sharkskin Robe", {"classuse": (30976, 32656)})
    p.item(5156, "Tidal Robe", {"classuse": (30976, 32656)})
    p.item(3260, "Confluent Gloves", {"classuse": (879, 17407)})
    p.item(4227, "Wondrous Walking Stick", {"classuse": (32767, 15360), "raceuse": (1023, 271)})
    p.item(5083, "runetrimmed robe", {"raceuse": (1023, 271), "itemname": ("runetrimmed robe", "Runetrimmed Robe")})
    p.item(7100, "Lifeline Sash", {"raceuse": (1023, 271)})
    p.item(8286, "Stone Covered Ring", {"raceuse": (1023, 271)})
    for pid in (5969, 5931, 5977):
        if p.one("itempattern", {"patternid": pid})["equipslot"] != 14:
            raise Refused(f"{pid} is not in slot 14 (entry 980's fix)")
    p.say("5969, 5931, 5977 in slot 14 already (entry 980)")


def part_984(p):
    p.loot_table("gnoll spiritpaw", ((3506, "Sunsteel Coif"), (3578, "Sunsteel Leggings")), 19, 27)
    p.bind("gnoll spiritpaw", "gnoll spiritpaw", 2, "Allakhazam items 1586, 1587; 2% ours (work board 984)")
    sh = rows(p.db, "loottables", {"tablename": "gnoll shaman"})
    want = [(7115, "Reinforced Spiritwolf Belt"), (7069, "Belt of the Spider"), (7052, "Adorberg Abbot Scarf"),
            (7567, "Prairie Trotter Boots"), (8191, "Band of Elemental Ice"), (6181, "Runed Shield")]
    full = [(pid, 33, 39) for pid, _ in want] + [(5786, 23, 27)]
    got = sorted((r["patternid"], r["minlevel"], r["maxlevel"]) for r in sh)
    if got == sorted(full):
        p.say("table 'gnoll shaman': already as asked")
    elif got:
        raise Refused(f"table 'gnoll shaman' holds {got}")
    else:
        for pid, name in want + [(5786, "Quarterstaff of Understanding")]:
            if item_name(p.db, pid) != name:
                raise Refused(f"{pid} is {item_name(p.db, pid)!r}, not {name!r}")
        for pid, lo, hi in full:
            p.ops.append((p.part, "loottables", None, None, {"tablename": "gnoll shaman", "patternid": pid, "weight": 1,
                                                              "minlevel": lo, "maxlevel": hi,
                                                              "source": SRC.format(p.part)}))
        p.say("table 'gnoll shaman': 7 rows (six at creature levels 33-39, the quarterstaff at 23-27)")
    p.bind("gnoll shaman", "gnoll shaman", 2, "Allakhazam beast 861 Known Drops by capture; 2% ours (work board 984)")
    p.loot_table("gnoll berserker", ((8066, "Firehide Choker"),), 33, 39)
    p.bind("gnoll berserker", "gnoll berserker", 2, "Allakhazam beast 1779; 2% ours (work board 984)")
    p.item(3578, "Sunsteel Leggings", {"classuse": (879, 512), "raceuse": (1023, 415)})
    p.item(5786, "Quarterstaff of Understanding", {"raceuse": (1023, 271)})
    p.item(7069, "Belt of the Spider", {"raceuse": (1023, 271)})


HOB_RING_DESC = "A dented hobgoblin ring. A merchant will pay for it."


def part_933(p):
    p.rename(123865, "Cyclops Visionary", "cyclops visionary", world=1)
    p.rekey("npctemplate", "name", "Cyclops Visionary", "cyclops visionary")
    p.rekey("npcclass", "name", "Cyclops Visionary", "cyclops visionary")
    p.rekey("lootcreature", "npc_name", "Cyclops Visionary", "cyclops visionary")
    if not rows(p.db, "npcs", {"npc_name": "Captain Marglar"}):
        raise Refused("no Captain Marglar")
    for f, d in (("Hobgoblins of Rathe Mtns.", -10), ("Craknek Warriors", 1), ("Citizens of Oggok", 1)):
        p.hit("Captain Marglar", f, d, "prima-2003")
    p.loot_table("hobgoblin mercenary", ((4683, "Green Silken Necklace"), (7877, "Hobgoblin Mercenary Bracers")))
    p.bind("hobgoblin mercenary", "hobgoblin mercenary", 2, "Allakhazam beast 1916 Known Drops; 2% ours (work board 933)")
    p.loot_table("cyclops", ((4133, "Butterfly Winged Robe"),))
    p.bind("cyclops", "cyclops", 1, "forum t291 (butterfly winged robe | cyclops | 44); 1% ours (work board 933)")
    r = p.one("itempattern", {"patternid": 4133})
    if (r["wis"], r["cha"], r["AC"], r["AR"], r["str"], r["sta"], r["agi"], r["dex"]) != (24, 17, 28, 6, 0, 0, 0, 0):
        raise Refused("4133's stats are not the forum's (the entry's relabel)")
    p.item(4133, "Butterfly Winged Robe", {"classuse": (32640, 32656)})
    have = rows(p.db, "itempattern", {"itemname": "dented hobgoblin ring"})
    if len(have) > 1:
        raise Refused("two records 'dented hobgoblin ring'")
    if have:
        nid = have[0]["patternid"]
        p.say(f"'dented hobgoblin ring' {nid}: already there")
    else:
        src = plain(p.one("itempattern", {"patternid": 707489}, "707489 hobgoblin belt"))
        if src["itemname"].lower() != "hobgoblin belt":
            raise Refused(f"707489 is {src['itemname']!r}")
        ring = plain(p.one("itempattern", {"patternid": 8209}, "a ring's icon (8209)"))
        nid = p.new_id("itempattern", "patternid", 1000000)
        vals = dict(src, patternid=nid, itemname="dented hobgoblin ring", itemdesc=HOB_RING_DESC, itemcost=130,
                    itemicon=ring["itemicon"])
        p.ops.append((p.part, "itempattern", None, None, vals))
        p.say(f"make {nid} 'dented hobgoblin ring' (707489's trash record, 130 tunar, a ring's icon {ring['itemicon']})")
    lp = [r for r in rows(p.db, "lootpool", {"npc_name": "hobgoblin mercenary"}) if r["patternid"] == nid]
    if lp:
        p.say("its lootpool row: already there")
    else:
        p.ops.append((p.part, "lootpool", None, None, {"category": "trash", "region": "", "npc_name": "hobgoblin mercenary",
                                                        "patternid": nid, "weight": 1, "minlevel": 1, "maxlevel": 100,
                                                        "source": SRC.format(p.part)}))
        p.say("lootpool trash: hobgoblin mercenary -> the ring")


ALTAR_EV = ev({"on": "done", "do": "spawn", "npc": "altar guardian", "hostile": True})
SMUG_OLD = "Take the gem from the artifact smuggler in the indigo pirates' camp in Cape Dreg (W)."
SMUG_NEW = "Take the gem from the indigo artifact smuggler in the indigo pirates' camp in Cape Dreg (W)."


def part_1052(p):
    p.npc(122834, "altar guardian")
    p.spawnonly("altar guardian", "raised by quest 101030's reach step on the bridge to Saftemoot's isle "
                                  "(Allakhazam quest 815)")
    p.rename(118742, "artifact smuggler", "indigo artifact smuggler", world=2)
    p.rekey("npcclass", "name", "artifact smuggler", "indigo artifact smuggler")
    p.rekey("lootcreature", "npc_name", "artifact smuggler", "indigo artifact smuggler")
    st = {r["step"]: r for r in rows(p.db, "queststeps", {"questid": 101030})}
    smug = st.get(22) if len(st) == 28 else st.get(23)
    if smug is None or smug["kind"] != "loot":
        raise Refused("101030's smuggler loot step is not where read")
    p.set("queststeps", smug, {"target": ("artifact smuggler", "indigo artifact smuggler"),
                               "journal": (SMUG_OLD, SMUG_NEW)}, f"101030 step {smug['step']} the smuggler's name")
    mv = p.one("npcmove", {"name": "sea nymph"})
    p.set("npcmove", mv, {"radius": (0, 400), "water": ("", "both")}, "the sea nymph roams 400")
    p.drop("npcrespawn", {"npc_row": 122837}, {"seconds": 90}, "npcrespawn 122837")
    p.drop("npcs", {"id": 122837}, {"npc_name": "Khonza Meysla", "world": 2}, "Khonza Meysla 122837")
    p.npc(131616, "Khonza Meysla")
    kill = st.get(3) if len(st) == 28 else st.get(4)
    if kill is None or (kill["kind"], kill["target"]) != ("kill", "altar guardian"):
        raise Refused("101030's altar guardian kill is not where read")
    p.relayout(101030, 28, mapping(28, 3), {3: reach(2, 11000.0, 64.7, 3000.0, 20.0,
                                                     "Cross the bridge to Saftemoot's isle in Erud's Crossing (C).",
                                                     ALTAR_EV)},
               "quest 101030 a reach step 3 raises the altar guardian")


GIANT_EV = ev({"on": "start", "do": "spawn", "npc": "sly ice giant", "hostile": True})


def part_784(p):
    p.npc(123025, "sly ice giant")
    p.spawnonly("sly ice giant", "raised at the start of quest 101272 step 3 (forum t157: a triggered spawn)")
    s = p.step(101272, 3)
    if (s["kind"], s["target"]) != ("loot", "sly ice giant"):
        raise Refused(f"101272 step 3 is {s['kind']} {s['target']!r}")
    p.set("queststeps", s, {"events": ("", GIANT_EV)}, "101272 step 3 raises the sly ice giant")
    p.npc(123027, "Creepyman")
    p.add("npcmove", {"name": "creepyman", "roam": 1, "radius": 75.0, "water": "",
                      "note": "'a delayed spawn, and he roams' (forum t157); the radius ours (work board 784)",
                      "source": SRC.format(p.part)}, ("name",), "Creepyman roams 75")
    s = p.step(101277, 3)
    bc = json.loads(s["byclass"]) if s["byclass"] else {}
    if bc.get("14") == {"items": "7177"}:
        p.say("101277 step 3: the alchemist's pants already")
        return
    if "14" in bc:
        raise Refused(f"101277 step 3 gives class 14 {bc['14']}")
    if item_name(p.db, 7177) != "pants of mischief":
        raise Refused("7177 is not the pants of mischief")
    bc["14"] = {"items": "7177"}
    p.set("queststeps", s, {"byclass": (s["byclass"], json.dumps(bc, separators=(",", ":"), sort_keys=True))},
          "101277 step 3: the alchemist gets pants of mischief 7177")


JOURNAL_100661_READ = "Kill goblin raiders west of the unfinished wall for a raider's cap."
JOURNAL_100661_FILM = "Kill a goblin raider. You will need help. Return with its cap"


def part_1256(p):
    body = p.one("npcs", {"id": 1812}, "large wolf spider 1812")
    if body["npc_name"] != "large wolf spider":
        raise Refused("1812 is not the large wolf spider")
    p.clone_npc(116991, "large spider", {"npc_name": "wolf spider", "npc_id": 0, "x_coord": 17810.0,
                                         "y_coord": 56.94, "z_coord": 10720.0, "modelid": body["modelid"]},
                "the wolf spider (Misty Thicket, best guess +/-300, the ground's floor 56.94)")
    p.add("npcmove", {"name": "wolf spider", "roam": 1, "radius": 0.0, "water": "",
                      "note": "video-audit B9vtLlOVZuY: best-guess wildlife placement; movement unobserved",
                      "source": "video-audit"}, ("name",), "the wolf spider roams")
    s = p.step(100661, 1)
    if s["journal"] not in (JOURNAL_100661_READ, JOURNAL_100661_FILM):
        p.say(f"step 2 held: 100661 step 1's journal is no longer the entry's read ({s['journal']!r})")
        return
    p.set("queststeps", s, {"journal": (JOURNAL_100661_READ, JOURNAL_100661_FILM)}, "100661 step 1 journal as filmed")


def part_865(p):
    nid = p.clone_npc(113409, "needlefang spider", {"npc_name": "needle fang", "npc_id": "self", "x_coord": 14800.0,
                                                     "y_coord": 179.9, "z_coord": 8450.0},
                      "the needle fang (north-west of Moradhim, +/-400, the ground's floor 179.9)")
    for t, c in (("npcclass", "name"), ("npctemplate", "name"), ("lootcreature", "npc_name")):
        p.copy_rule(t, c, "needlefang spider", "needle fang", f"{t} needle fang as the needlefang spider's")
    p.add("npcaggro", {"npc_name": "needle fang", "aggressive": 0, "radius": 0.0, "leash": 0.0, "holding": 0,
                       "see_invis": 0, "holds": 0,
                       "note": "white face to a level 5 on live footage (0SOa-U6DmpE 146-156 s)",
                       "source": "video-audit"}, ("npc_name",), "needle fang not KoS")
    p.add("npctime", {"name": "needle fang", "time": "night",
                      "note": "filmed only at night (0SOa-U6DmpE 146-156 s); work board 865", "source": "video-audit"},
          ("name",), f"needle fang night only (row {nid})")


VOX_OLD = "Self, aoe Decrease Hitpoints by 2000, Radius 80"
VOX_NEW = "Self, aoe Decrease Hitpoints by 357, Radius 80"
VOX_E_OLD = ('{"v":1,"scope":"player_aoe","target_type":"Enemy","harmful":true,"stack":0,"level":60,"min_level":1,'
             '"radius":80,"req_equipment":"Any","sheet_type":"Dmg/Heal","effects":[{"type":"Dmg/Heal","kind":"hp2",'
             '"amount":-2000,"resist":"cr"}]}')
VOX_E_NEW = VOX_E_OLD.replace('"amount":-2000', '"amount":-357')


def part_678(p):
    t = p.one("classtemplates", {"name": "lady vox"})
    if '"every":53' not in t["abilities"] or t["special"] != "":
        raise Refused("classtemplates 'lady vox' is not open-01-678's (53 s breath, no crit)")
    p.set("spellpattern", p.one("spellpattern", {"spellid": 905140}), {"spelldesc": (VOX_OLD, VOX_NEW)},
          "Frost Breath 905140 description")
    p.set("spelleffect", p.one("spelleffect", {"spellid": 905140}), {"line": (VOX_OLD, VOX_NEW),
                                                                     "effects": (VOX_E_OLD, VOX_E_NEW)},
          "Frost Breath 905140 two hits of 357 cold")
    for r in rows(p.db, "npcs", {"npc_name": "Lady Vox"}) + rows(p.db, "npcs", {"npc_name": "Enraged Lady Vox"}):
        rr = rows(p.db, "npcrespawn", {"npc_row": r["id"]})
        p.say(f"{r['npc_name']} {r['id']} respawn {rr[0]['seconds'] if rr else 'none'} (kept)")


def part_785(p):
    for rid in range(100261, 100268):
        p.rename(rid, "pestilence rat", "Pestilence Rat", world=0)


def part_1175(p):
    p.rename(133746, "Xinnalagus", "Xinnalanagus", world=2)
    for t, c in (("npctemplate", "name"), ("lootcreature", "npc_name"), ("npcmove", "name")):
        if not p.db.execute(f"SELECT 1 FROM {t} WHERE lower({c}) = 'xinnalanagus'").fetchone():
            raise Refused(f"{t} has no 'xinnalanagus' row")
    if not rows(p.db, "npcpaths", {"npcid": 133746}):
        raise Refused("no npcpaths 133746")
    p.say("its template, loot region, move row and path are there")


def part_1026(p):
    wolf = p.npc(142864, "wolf")
    r = p.npc(122557, "Bloodpaw")
    p.set("npcs", r, {"modelid": (3477165409, wolf["modelid"]), "race": ("6", wolf["race"])},
          "Bloodpaw 122557 on wolf 142864's body")
    p.respawn(122557, 90, 1800, "Bloodpaw the wolf, a standing named: 1,800 s ours (work board 1026)")
    p.add("npcclass", {"name": "bloodpaw", "class": 0, "source": "gm"}, ("name",), "npcclass bloodpaw 0")
    for rid in (122558, 122559, 122560):
        r = rows(p.db, "npcs", {"id": rid})
        if not r or r[0]["npc_name"] not in ("Bloodpaw", "gnoll bloodpaw"):
            raise Refused(f"npcs {rid}: {r[0]['npc_name'] if r else 'no row'!r}")
        p.set("npcs", r[0], {"npc_name": ("Bloodpaw", "gnoll bloodpaw")}, f"npcs {rid} gnoll bloodpaw")
        p.drop("npcrespawn", {"npc_row": rid}, {"seconds": 90}, f"npcrespawn {rid} (back in 15 s)")
    for t, c in (("npctemplate", "name"), ("lootcreature", "npc_name"), ("npcclass", "name")):
        p.copy_rule(t, c, "gnoll strongfang", "gnoll bloodpaw", f"{t} gnoll bloodpaw as gnoll strongfang's")
    for lid, pid in ((4210, 707437), (4211, 707438)):
        lp = p.one("lootpool", {"id": lid})
        if lp["patternid"] != pid:
            raise Refused(f"lootpool {lid} holds {lp['patternid']}")
        p.set("lootpool", lp, {"npc_name": ("bloodpaw", "gnoll bloodpaw")}, f"lootpool {lid} to the gnolls")
    have = {r["patternid"] for r in rows(p.db, "lootpool", {"npc_name": "bloodpaw"})}
    for lid, pid in ((11634, 708019), (11635, 708018)):
        w = p.one("lootpool", {"id": lid})
        if (w["npc_name"], w["patternid"]) != ("wolf", pid):
            raise Refused(f"lootpool {lid} is not wolf's {pid}")
        if pid in have:
            p.say(f"lootpool bloodpaw {pid}: already there")
            continue
        p.ops.append((p.part, "lootpool", None, None, {"category": "trash", "region": "", "npc_name": "bloodpaw",
                                                        "patternid": pid, "weight": 1, "minlevel": 1, "maxlevel": 100,
                                                        "source": "gm"}))
        p.say(f"lootpool trash: bloodpaw -> {pid} (as wolf's {lid})")
    s = p.step(101172, 1)
    p.set("queststeps", s, {"target": ("Bloodpaw", "gnoll bloodpaw"),
                            "journal": ("Kill Bloodpaw gnolls for four blood paw collars.",
                                        "Kill gnoll bloodpaws for four blood paw collars.")}, "101172 step 1")
    p.loot_table("bloodpaw", ((7476, "Bloodfur Boots"), (4926, "Bloodfur Robe")), 18, 22)
    p.bind("bloodpaw", "bloodpaw", 25, "Allakhazam beast 1339 Known Drops; 25% ours (work board 1026)")


def part_1038(p):
    r = rows(p.db, "npcs", {"id": 121848})
    if not r or r[0]["npc_name"] not in ("Xorbb", "King Xorbb"):
        raise Refused("121848 is not Xorbb")
    p.rename(121848, "Xorbb", "King Xorbb", world=0)
    p.rekey("npcclass", "name", "xorbb", "king xorbb")
    p.rekey("npctemplate", "name", "xorbb", "king xorbb")
    p.rekey("lootcreature", "npc_name", "xorbb", "king xorbb")
    nl = rows(p.db, "npcloot", {"npc_name": "xorbb", "tablename": "boss xorbb"}) or \
        rows(p.db, "npcloot", {"npc_name": "king xorbb", "tablename": "boss xorbb"})
    if len(nl) != 1:
        raise Refused("no npcloot -> 'boss xorbb'")
    p.set("npcloot", nl[0], {"npc_name": ("xorbb", "king xorbb"), "chance": (100, 25)}, "his npcloot 25%")
    p.respawn(121848, 90, 1800, "King Xorbb, a standing named: 1,800 s ours (work board 1038)")
    for n in ("King Xorbb", "enthralled halfling", "enthralled minotaur"):
        p.hit(n, "The Eye", -10, "prima-2003")
    p.say("Glonkal (section 2) waits on entry 1037: goblin prisoner 123004 still stands in his alcove")


KING_SPOT = {"x_coord": 9420.0, "y_coord": 67.35, "z_coord": 4605.0}


def part_937(p):
    nid = p.clone_npc(124120, "Ankexfen defender", dict(KING_SPOT, npc_name="Ankexfen King", npc_level=35),
                      "the Ankexfen King (the pyramid's top floor, the ground's floor 67.35)")
    p.add("npcclass", {"name": "ankexfen king", "class": 0, "source": SRC.format(p.part)}, ("name",), "npcclass")
    p.add("npctemplate", {"name": "ankexfen king", "template": "roamer", "note": "named, level 35",
                          "source": SRC.format(p.part)}, ("name",), "npctemplate roamer")
    p.copy_rule("lootcreature", "npc_name", "ankexfen defender", "ankexfen king", "lootcreature as the defender's")
    if isinstance(nid, int) and rows(p.db, "npcs", {"id": nid}):
        p.respawn(nid, None, 1800, "the Ankexfen King: 1,800 s ours, a stopgap (work board 937)")
    else:
        p.ops.append((p.part, "npcrespawn", None, None, {"npc_row": nid, "seconds": 1800,
                                                          "note": "the Ankexfen King: 1,800 s ours, a stopgap (work board 937)",
                                                          "source": SRC.format(p.part)}))
        p.say(f"respawn of {nid} 1800 s")
    p.loot_table("ankexfen king", ((4915, "Ankexfen Death Shroud"),))
    p.bind("ankexfen king", "ankexfen king", 25, "forum t291 (Ankexfen Death Shroud, Ankexfen King); 25% ours "
                                                 "(work board 937)")
    camp = [r for r in rows(p.db, "loottables", {"tablename": "ankexfen camp"}) if r["patternid"] == 4915]
    if camp:
        p.drop("loottables", {"id": camp[0]["id"]}, {"tablename": "ankexfen camp", "patternid": 4915},
               "4915 out of 'ankexfen camp'")
    else:
        p.say("4915 out of 'ankexfen camp': already")
    p.say("7053 stays in 'ankexfen camp' (entry 815's 'xexhariz' table is not built)")
    p.item(4915, "Ankexfen Death Shroud", {"classuse": (30976, 32656)})
    p.item(5495, "Ankexfen War Sceptre", {"raceuse": (1023, 271), "maxhp": (6000, 7600), "duration": (50, 60)})
    for pid, name in ((5881, "Tarnished Ankexfen Dagger"), (5882, "Tarnished Ankexfen Naginata"),
                      (5883, "Tarnished Ankexfen Scimitar"), (5924, "Worn Ankexfen Battlestaff")):
        p.item(pid, name, {"maxhp": (5200, 5700), "duration": (50, 65)})
    p.item(6855, "Ankexfen Chain Gloves", {"maxhp": (5200, 6200), "duration": (50, 65)})
    p.item(7053, "Ankexfen Engraved Belt", {"maxhp": (6000, 6600), "duration": (50, 65)})


ALPHA_EV = ev({"on": "done", "do": "spawn", "npc": "silvermist alpha", "hostile": True})
NOTES_767 = ("The silvermist alpha is raised by a reach step at the back of the cave (Prima p.368, forum t127). Jack's "
             "belt is Allakhazam 442's and 660's.")
NOTE_869_OLD = "here she stands in the cave where FrontiersForge saw her"
NOTE_869_NEW = "she is raised by a reach step at the back of the cave (Prima p.368, forum t127)"


def part_1048(p):
    if p.one("itempattern", {"patternid": 7139})["classuse"] != 32767:
        raise Refused("7139 is not all classes yet (entry 999's line first)")
    a = p.npc(122576, "silvermist alpha")
    wolf = rows(p.db, "npcs", {"id": 108839})
    body = wolf[0]["modelid"] if wolf else 2118838847
    p.set("npcs", a, {"modelid": (2925832801, body)}, "122576 on the pack's body")
    p.spawnonly("silvermist alpha", "raised by quests 100767 and 100869 at the back of the wolf cave north of Trail's "
                                    "End (Prima p.368, forum t127)")
    s767 = {r["step"]: r for r in rows(p.db, "queststeps", {"questid": 100767})}
    if len(s767) == 2:
        if (s767[1]["kind"], s767[1]["target"]) != ("loot", "silvermist wolf alpha"):
            raise Refused(f"100767 step 1 is {s767[1]['target']!r}")
        p.set("queststeps", s767[1], {"target": ("silvermist wolf alpha", "silvermist alpha"),
                                      "journal": (s767[1]["journal"], "Kill the silvermist alpha and take Max's journal.")},
              "100767 the loot step on silvermist alpha")
        p.set("queststeps", s767[2], {"rewarditems": ("", "7139")}, "100767 Jack's belt")
    p.relayout(100767, 2, mapping(2, 1), {1: reach(0, 17360.0, 96.0, 16764.0, 20.0,
                                                   "Fight through the silvermist wolves to the back of the first cave "
                                                   "north of Trail's End.", ALPHA_EV)},
               "100767 a reach step 1 raises her")
    s869 = {r["step"]: r for r in rows(p.db, "queststeps", {"questid": 100869})}
    loot = s869.get(2) if len(s869) == 3 else s869.get(3)
    if loot is None or (loot["kind"], loot["target"]) != ("loot", "silvermist alpha"):
        raise Refused("100869's loot step is not where read")
    p.set("queststeps", loot, {"journal": (loot["journal"], "Kill the silvermist alpha for Maximillian's journal and "
                                                            "the runed stone tablet.")}, "100869 the loot step's journal")
    p.relayout(100869, 3, mapping(3, 2), {2: reach(0, 17360.0, 96.0, 16764.0, 20.0,
                                                   "Journey to the back of the wolf cave on the road north of Trail's "
                                                   "End.", ALPHA_EV)},
               "100869 a reach step 2 raises her")
    q = p.one("quests", {"questid": 100767})
    p.set("quests", q, {"notes": (q["notes"], NOTES_767)}, "100767 notes")
    q = p.one("quests", {"questid": 100869})
    if NOTE_869_OLD in q["notes"]:
        p.set("quests", q, {"notes": (q["notes"], q["notes"].replace(NOTE_869_OLD, NOTE_869_NEW))}, "100869 notes")
    elif NOTE_869_NEW in q["notes"]:
        p.say("100869 notes: already")
    else:
        raise Refused("100869's notes hold neither line")
    if wolf:
        if wolf[0]["npc_name"] != "silvermist wolf alpha":
            raise Refused("108839 is not the silvermist wolf alpha")
        for t in ("npcrespawn", "npcpaths"):
            if rows(p.db, t, {"npc_row" if t == "npcrespawn" else "npcid": 108839}):
                raise Refused(f"{t} names 108839")
        p.drop("npcs", {"id": 108839}, {"npc_name": "silvermist wolf alpha"}, "108839 deleted")
    else:
        p.say("108839: already gone")
    p.drop("npcclass", {"name": "silvermist wolf alpha"}, {"class": 0}, "npcclass silvermist wolf alpha")
    p.drop("npctemplate", {"name": "silvermist wolf alpha"}, {"template": "roamer"}, "npctemplate silvermist wolf alpha")
    p.drop("lootcreature", {"npc_name": "silvermist wolf alpha"}, {"region": "forest"},
           "lootcreature silvermist wolf alpha")


def part_995(p):
    have = rows(p.db, "npcs", {"id": 102976})
    if have:
        if have[0]["npc_name"] != "thrall":
            raise Refused(f"npcs 102976 is {have[0]['npc_name']!r}")
        p.set("npcs", have[0], {"npc_level": (15, 44)}, "the thrall 102976 L44")
    else:
        a = [r for r in rows(p.db, "gm_audit", {"tbl": "npcs", "action": "delete", "rowkey": '{"id":102976}'})
             if r["before"] and TOOL not in (r["detail"] or "")]  # not this tool's own undo
        if len(a) != 1:
            raise Refused(f"{len(a)} gm_audit deletes of npcs 102976")
        b = json.loads(a[0]["before"])
        if (b["npc_name"], b["world"], b["npc_level"]) != ("thrall", 0, 15):
            raise Refused(f"gm_audit {a[0]['id']} holds {b['npc_name']!r} L{b['npc_level']}")
        c = cols(p.db, "npcs")
        vals = {k: b.get(k) for k in c}
        vals["npc_level"] = 44
        p.ops.append((p.part, "npcs", None, None, vals))
        p.say(f"the thrall 102976 back as gm_audit {a[0]['id']} holds it, L15 -> 44")
    p.add("npcaggro", {"npc_name": "thrall", "aggressive": 1, "radius": 0.0, "leash": 0.0, "holding": 0,
                       "see_invis": 0, "holds": 0, "note": "red face on live footage (f3FNEeEhH8w, 95-117 s)",
                       "source": "video-audit"}, ("npc_name",), "the thrall attacks on sight")
    m = p.npc(102985, "marionette")
    p.clone_npc(102985, "marionette", {"npc_name": "marionette", "x_coord": 7250.3, "y_coord": m["y_coord"],
                                       "z_coord": m["z_coord"], "npc_id": "self"},
                "a second marionette 5 east of 102985 (L%d as it)" % m["npc_level"])


BALIDOS_EV = ev({"on": "done", "do": "spawn", "npc": "Balidos", "hostile": True})
PAPERS_EV = ev({"on": "done", "do": "give", "items": "701292"})
NARIS_EV = ev({"on": "done", "do": "popup", "text": "Three Paineel recovery agents close ranks around Naris as you approach."})
NARIS_J = "Find Naris in Paineel (SE), guarded by three Paineel recovery agents."


def part_892(p):
    for rid in (133558, 133559, 133560):
        p.drop("npcs", {"id": rid}, {"npc_name": "paineel recovery agent", "world": 2}, f"the doubled agent {rid}")
    p.drop("npcs", {"id": 133561}, {"npc_name": "Naris", "world": 2}, "the doubled Naris 133561")
    p.set("npcs", p.npc(121057, "Balidos"), {"npc_type": (0, 128), "npc_level": (43, 45)}, "Balidos a talker, L45")
    for rid in (121059, 121060, 121061):
        p.level(rid, "paineel recovery agent", 48, 63)
    t = rows(p.db, "npctemplate", {"name": "paineel recovery agent"})
    if t and t[0]["template"] != "paineel recovery agent":
        raise Refused(f"the agents fight by template {t[0]['template']!r} (the entry read none)")
    p.add("classtemplates", {"name": "paineel recovery agent", "melee_pct": 50,
                             "note": "Paineel's recovery agents: 'don't hit hard' (forum t173); 50 ours; otherwise as "
                                     "they fought with no template (work board 892)",
                             "source": SRC.format(p.part)}, ("name",), "template 'paineel recovery agent' melee 50%")
    p.add("npctemplate", {"name": "paineel recovery agent", "template": "paineel recovery agent",
                          "note": "work board 892", "source": SRC.format(p.part)}, ("name",), "npctemplate agents")
    p.set("npcs", p.npc(121062, "General Dorganun"), {"npc_level": (47, 61)}, "General Dorganun L61")
    p.spawnonly("General Dorganun", "raised by quest 101228's kill step (forum t173: Naris tricks you and he spawns)")
    st = {r["step"]: r for r in rows(p.db, "queststeps", {"questid": 101228})}
    if len(st) == 14:
        if [st[n]["target"] for n in (2, 3, 5)] != ["Balidos", "Balidos", "Naris"]:
            raise Refused("101228's steps 2, 3, 5 are not as read")
        p.set("queststeps", st[2], {"events": ("", BALIDOS_EV)}, "101228 step 2: Balidos turns")
        p.set("queststeps", st[3], {"kind": ("loot", "kill"), "items": ("701292", ""),
                                    "journal": ("Defeat Balidos and take his papers.", "Defeat Balidos.")},
              "101228 step 3 a kill")
    m = {1: 1, 2: 2, 3: 3, 4: 5}
    m.update({s: s + 2 for s in range(5, 15)})
    p.relayout(101228, 14, m, {
        4: ({"kind": "talk", "target": "Balidos", "journal": "Speak with Balidos again for his papers.",
             "says": "Enough. Take the papers; they name the one you want.", "events": PAPERS_EV},
            ("kind", "target", "events")),
        6: reach(2, 7845.0, 0.0, 9469.0, 30.0, NARIS_J, NARIS_EV)},
               "101228: Balidos's papers at a second talk (4), the pop-up before Naris (6)")


ROJ_QUESTS = (100130, 100140, 100150, 100160, 100476)
ROJ_EV = ev({"on": "done", "do": "spawn", "npc": "Roj Eir Sew' Eil"})


def part_1043(p):
    p.npc(2222, "Roj Eir Sew' Eil")
    p.spawnonly("Roj Eir Sew' Eil", "raised by quests 100130, 100140, 100150, 100160, 100476 at his tent")
    for q in ROJ_QUESTS:
        st = sorted(rows(p.db, "queststeps", {"questid": q}), key=lambda r: r["step"])
        n = len(st)
        loot = next((r for r in st if r["kind"] == "loot" and r["target"] == "Roj Eir Sew' Eil"), None)
        if loot is None:
            raise Refused(f"quest {q} has no loot step on Roj Eir")
        old = n if loot["step"] == 3 else n - 1
        p.relayout(q, old, mapping(old, 3), {3: reach(0, 24592.0, 58.0, 17142.0, 15.0,
                                                      "Find Roj Eir Sew' Eil's tent in the nasehir camp south-west of "
                                                      "Freeport.", ROJ_EV)},
                   f"quest {q} a reach step 3 raises Roj Eir")
    for f, d in (("Nasehir Cutthroats", -10), ("The Taksehir Nation", 1), ("Tunaria Gypsies", 5),
                 ("Smugglers of Temby", 1)):
        p.hit("Roj Eir Sew' Eil", f, d, "prima-2003")
    p.item(1531, "Mercury Bead", {"classuse": (32767, 127)})


TRIAL_EV = ev({"on": "done", "do": "spawn", "npc": "vengeance spirit", "hostile": True, "count": 4},
              {"on": "done", "do": "spawn", "npc": "Eidolon of Malice", "hostile": True})
NOTE_876_OLD = ("and the timed trial are not in: the Caretaker asks to see the amulet. The Caretaker and the Eidolon "
                "stand where")
NOTE_876_NEW = ("are not in. Showing the amulet raises the trial (Prima p.370): four vengeance spirits and the Eidolon of "
                "Malice, at once until spawn events can wait. The Caretaker stands where")


def part_1059(p):
    p.level(122582, "Eidolon of Malice", 46, 47)
    p.spawnonly("Eidolon of Malice", "raised by quest 100876's trial at the Caretaker (Prima p.370; AZ 694)")
    p.clone_npc(122582, "Eidolon of Malice", {"npc_name": "vengeance spirit", "npc_level": 46, "size": 1.0,
                                              "npc_type": 0, "x_coord": 11623.0, "y_coord": 83.0, "z_coord": 11510.0,
                                              "npc_id": "self"}, "the vengeance spirit row (spawn-only)")
    p.spawnonly("vengeance spirit", "raised by quest 100876's trial (Prima p.370)")
    s = p.step(100876, 1)
    if (s["kind"], s["target"]) != ("show", "The Caretaker"):
        raise Refused(f"100876 step 1 is {s['kind']} {s['target']!r}")
    p.set("queststeps", s, {"events": ("", TRIAL_EV)}, "100876 step 1 raises the trial")
    q = p.one("quests", {"questid": 100876})
    if NOTE_876_OLD in q["notes"]:
        p.set("quests", q, {"notes": (q["notes"], q["notes"].replace(NOTE_876_OLD, NOTE_876_NEW))}, "100876 notes")
    elif NOTE_876_NEW in q["notes"]:
        p.say("100876 notes: already")
    else:
        raise Refused("100876's notes hold neither line")


V_OLD, V_NEW = "Vaelian Wyndsorrow", "Vaelan Wyndsorrow"
SAYS2_OLD = ("Good riddance. Now Vaelian Wyndsorrow, in the enchanters' guild, the west building on the lake, wants "
             "paying. Go to him.")
SAYS2_NEW = "Good riddance. Now Vaelan Wyndsorrow, in the College of High Magic, wants paying. Go to him."
J3_NEW = "Pay Vaelan Wyndsorrow in the College of High Magic 600 tunar."
NOTES = (("note to Adan Stormbear", "A sealed note from Helania Keldworth."),
         ("note to Dristan Blane", "A sealed note from Helania Keldworth."),
         ("Adan Stormbear's confirmation letter", "Adan Stormbear's answer to Helania."),
         ("Dristan Blane's confirmation letter", "Dristan Blane's answer to Helania."))
ZEROED = ("levelreq", "classuse", "raceuse", "itemcost", "model", "weapondamage", "attacktype", "AC", "str", "sta",
          "agi", "wis", "dex", "cha", "intelligence", "HPMAX", "POWMAX", "PoT", "HoT", "PR", "DR", "FR", "CR", "LR",
          "AR")


def part_1046(p):
    p.rename(104482, V_OLD, V_NEW, world=0)
    p.rekey("npcclass", "name", "vaelian wyndsorrow", "vaelan wyndsorrow")
    nf = rows(p.db, "npcfaction", {"name": "vaelian wyndsorrow"}) or rows(p.db, "npcfaction", {"name": "vaelan wyndsorrow"})
    if len(nf) != 1 or nf[0]["faction"] != "Citizens of Fayspires":
        raise Refused("npcfaction vaelian wyndsorrow is not as read")
    p.set("npcfaction", nf[0], {"name": ("vaelian wyndsorrow", "vaelan wyndsorrow")}, "npcfaction rekeyed")
    s2, s3 = p.step(100747, 2), p.step(100747, 3)
    p.set("queststeps", s2, {"says": (SAYS2_OLD, SAYS2_NEW)}, "100747 step 2 says")
    if s3["journal"] != J3_NEW and "Vaelian Wyndsorrow 600 tunar in the enchanters' guild" not in s3["journal"]:
        raise Refused(f"100747 step 3's journal is not as read: {s3['journal']!r}")
    p.set("queststeps", s3, {"target": (V_OLD, V_NEW), "journal": (s3["journal"], J3_NEW)}, "100747 step 3")
    q = p.one("quests", {"questid": 100747})
    if "the game's Vaelian Wyndsorrow" in q["notes"]:
        p.set("quests", q, {"notes": (q["notes"], q["notes"].replace("the game's Vaelian Wyndsorrow",
                                                                     "the game's Vaelan Wyndsorrow"))}, "100747 notes")
    # the new quest
    for rid, name in ((105920, "Helania Keldworth"), (101137, "Adan Stormbear"), (108721, "Dristan Blane")):
        p.npc(rid, name)
    have = rows(p.db, "quests", {"name": "Helania's Letters"})
    if have:
        p.say(f"quest \"Helania's Letters\" {have[0]['questid']}: already there")
        return
    src = plain(p.one("itempattern", {"patternid": 8434}, "8434 note from wilkinson"))
    ids = []
    for name, desc in NOTES:
        if rows(p.db, "itempattern", {"itemname": name}):
            raise Refused(f"a record {name!r} is there with no quest")
        nid = p.new_id("itempattern", "patternid", 1000000)
        v = dict(src, patternid=nid, itemname=name, itemdesc=desc, equipslot=-1, maxstack=1, itemicon=1678411793,
                 color=4294967295, trade=1, lore=1, maxhp=0, duration=0)
        for c in ZEROED:
            v[c] = 0
        p.ops.append((p.part, "itempattern", None, None, v))
        p.say(f"make {nid} {name!r}")
        ids.append(nid)
    qid = p.new_id("quests", "questid")
    qv = {"questid": qid, "name": "Helania's Letters", "enabled": 1, "giver": "Helania Keldworth", "minlevel": 16,
          "maxlevel": 50, "classes": "", "races": "", "requires": "", "repeatable": 0,
          "offer": "%p, I have two letters that must not go astray. Will you carry them for me?",
          "accept": "I will carry them.", "decline": "Not now.", "declined": "Then go. Come back when you are ready.",
          "started": "Adan Stormbear in Highpass first, then Dristan Blane in Temby. Bring me back their answers.",
          "source": "AZ quest 592", "notes": "Allakhazam quest 592 (2007); words and item names ours (work board 1046).",
          "events": ev({"on": "accept", "do": "give", "items": f"{ids[0]}, {ids[1]}"}), "factionreq": ""}
    p.ops.append((p.part, "quests", None, None, qv))
    p.say(f"quest {qid} \"Helania's Letters\" (16-50, Helania Keldworth)")
    steps = (
        {"step": 1, "kind": "give", "target": "Adan Stormbear", "items": str(ids[0]),
         "journal": "Take Helania's note to Adan Stormbear, by the grocers at Highpass's north entrance.",
         "says": "Tell her it is done. Here is my word on it.",
         "events": ev({"on": "done", "do": "give", "items": str(ids[2])})},
        {"step": 2, "kind": "give", "target": "Dristan Blane", "items": str(ids[1]),
         "journal": "Take Helania's note to Dristan Blane, in the middle shack on the shore north of Freeport.",
         "says": "So she still writes. Here, take her my answer.",
         "events": ev({"on": "done", "do": "give", "items": str(ids[3])})},
        {"step": 3, "kind": "give", "target": "Helania Keldworth", "items": f"{ids[2]}, {ids[3]}",
         "journal": "Bring both confirmation letters back to Helania Keldworth in Highbourne.",
         "says": "Both of them. You have my thanks, %p.", "xp": 400000, "rewardtunar": 800})
    c = cols(p.db, "queststeps")
    for s in steps:
        row = {"questid": qid, "count": 1, "npcid": 0, "chance": 0.0, "radius": 15.0}
        row.update(s)
        p.ops.append((p.part, "queststeps", None, None, {k: v for k, v in row.items() if k in c}))
    p.say(f"quest {qid}: 3 give steps (Adan, Dristan, Helania: 400,000 XP and 800 tunar)")


PARTS = ("1783", "1777", "1011", "929", "918", "917", "898", "936", "935", "1018", "954", "908", "970", "986", "984",
         "933", "1052", "784", "1256", "865", "678", "785", "1175", "1026", "1038", "937", "1048", "995", "892", "1043",
         "1059", "1046")
LIVE_ONLY = ("678", "785", "1175")


def build(p, x):
    if x in LIVE_ONLY and p.shard != "live":
        p.say("Live's residue only (Heirloom has it): checked, nothing written here")
        if x == "678":
            t = p.one("spelleffect", {"spellid": 905140})
            if t["line"] != VOX_NEW:
                raise Refused("Heirloom's 905140 is not 357 (open-01-678)")
        return
    globals()[f"part_{x}"](p)


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


def undo(db, parts):
    if not table(db, LOG):
        return 0, []
    back, left = 0, []
    marks = ",".join("?" * len(parts))
    for seq, part, tbl, rid, rowkey, before, after in db.execute(
            f"SELECT seq, part, tbl, rid, rowkey, before, after FROM {LOG} WHERE part IN ({marks}) ORDER BY seq DESC",
            tuple(parts)).fetchall():
        have = plain(by_rowid(db, tbl, rid))
        if (have is None) != (after is None) or (have is not None and dumps(have) != after):
            left.append(f"{part} {tbl} {rowkey}: changed since, left as it is")
            continue
        if tbl == "itempattern" and before is None:
            pid = json.loads(after)["patternid"]
            h = next((t for t, c in (("charinventory", "patternid"), ("npcitem", "patternid"),
                                     ("auction_lots", "patternid")) if table(db, t) and
                      c in cols(db, t) and db.execute(f"SELECT 1 FROM {t} WHERE {c} = ? LIMIT 1", (pid,)).fetchone()),
                     None)
            if h:
                left.append(f"itempattern {pid}: {h} holds it, kept")
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
    ap.add_argument("--only", help="comma list of entries (default: every part)")
    ap.add_argument("--skip", help="comma list of entries left out")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    split = lambda s: [x.strip() for x in (s or "").split(",") if x.strip()]  # noqa: E731
    parts = split(a.only) or list(PARTS)
    for x in parts + split(a.skip):
        if x not in PARTS:
            ap.error(f"no part {x}")
    parts = [x for x in parts if x not in split(a.skip)]
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("PRAGMA busy_timeout = 5000")
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db, parts)
            db.execute("COMMIT")
            print(f"undone ({','.join(parts)}): {back} rows put back" + "".join(f"\n  left: {w}" for w in left))
            return 0
        p = Plan(db, a.shard)
        for x in parts:
            p.part = x
            mark = len(p.ops)
            try:
                build(p, x)
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
        db.close()


if __name__ == "__main__":
    sys.exit(main())
