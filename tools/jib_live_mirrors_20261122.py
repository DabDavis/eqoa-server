#!/usr/bin/env python3
"""jibrish's Live mirrors of 2026-11-22 (owner-approved as submitted): the class chains he rebuilt on Heirloom copied to
Live, and his board message #257 (Solusek's Eye's trash loot keyed by row). Source: Heirloom's database (--source, read
only), its CURRENT rows; target: Live's (--db).

Work board entries, applied in this order (ENTRIES; each its own part of the log):
  1965 dark elf alchemist L1-L20 + cleric L1-L15      2011 dark elf enchanter + warrior L1-L15 (after 1965)
  1982 dark elf rogue L1-L20                          1986 halfling cleric + druid    1987 halfling rogue + warrior
  1945 Erudite alchemist                              1992 Erudite arcane five (after 1945)   1953 Erudite cleric + paladin
  1963 gnome cleric L7-L20    1974 enchanter    1975 magician    1981 necromancer    1994 warrior    1995 wizard
  1933 gnome alchemist L7-L20                         2010 the gnome offers' decline "" (after the gnome copies)
  257  (his board message) SE kobold slayers, kobold augurs, stone spiders and Primal Spirits bound by row to the SE
       trash tables (npcloot "#<id>", 198 rows)
Not here: 1973 (the ogre chains) needs 1939 on Live first; 1920 was applied on Live by jibrish's own sessions. The code
asks of 1933 and 1981 (waysender routes) are not data.

What each entry does is its function below, read off the entry's text: new item records copied whole at Heirloom's ids
(the quests name them) with Live's era race list where the entry gives one; the listed columns of existing records;
NPC renames with the rows keyed by the old name; NPC, faction, stock, dialog and loot rows copied from Heirloom under
Live's own ids; quests, queststeps and queststeptargets copied whole (the entry's Live-only overrides kept); characters
part-way moved by the entry's step map, with what the old step had handed them given back.

Refused (that entry and every entry after it that needs it; nothing of it written): an id taken by another record, a
row not as Live had it before (a column neither Heirloom's value before the entry's audits nor its value now, an
expected old value the entry quotes, a quest whose steps' kinds, items or count differ from Heirloom's before), a name
not found, a character part-way where the entry says to ask.

Report only by default (every entry planned and checked in one transaction, rolled back); --apply writes in one
transaction, a gm_audit line a row, every row's before and after in jib_live_mirrors_1122_log; an entry in the log is
not done again; --undo puts back every row as it was (its rowid too), a row changed since left and listed. --only
names entries (comma separated) for any of these.

Quests load at the server's start: after --apply, reload them (the GM panel's quest_set_enabled off and on) and the NPCs
(reload_npcs). The server holds a player's pack and quests while they play: a part-way character online is listed.

    python3 -E -s tools/jib_live_mirrors_20261122.py --db LIVE.db --shard live --source HEIRLOOM.db [--apply | --undo]
        [--only 1965,2011]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "jib_live_mirrors_1122_log"
SEQ = LOG + "_seq"
TOOL = "tools/jib_live_mirrors_20261122.py"
WHO, VIA = "dab", "tool"
INV_CAP = 100  # the pack (config pack_slots)

KEYS = {"itempattern": ("patternid",), "npcitem": ("itemid",), "quests": ("questid",),
        "queststeps": ("questid", "step"), "queststeptargets": ("questid", "step", "target"), "npcs": ("id",),
        "npcfaction": ("id",), "npcclass": ("name",), "lootpool": ("id",), "lootcreature": ("npc_name",),
        "factionhits": ("id",), "npcspawnonly": ("name",), "npcdialog": ("id",), "loottables": ("id",),
        "npcloot": ("npc_name", "tablename"), "npcaggro": ("npc_name",), "npcmove": ("name",),
        "spellpattern": ("spellid",), "charquests": ("serverid", "questid"), "charinventory": ("itemid",)}
AUTO = {t for t, k in KEYS.items() if k in (("id",), ("itemid",))}  # rows a copy makes under Live's own id

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    part   TEXT NOT NULL,   -- the work board entry
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,   -- the key (JSON)
    before TEXT,            -- the whole row before, its rowid as _rowid_ (JSON; NULL: there was none)
    after  TEXT,            -- the whole row after (JSON; NULL: removed)
    audit  INTEGER NOT NULL,
    at     TEXT NOT NULL
)"""
SEQ_DDL = f"CREATE TABLE IF NOT EXISTS {SEQ} (part TEXT NOT NULL, tbl TEXT NOT NULL, before INTEGER, after INTEGER, PRIMARY KEY (part, tbl))"

RENAMES = {}
T_HOLD = set()   # --hold: characters online, left as they are
FROGLOK = 1024  # the owner's froglok on full race lists (2026-11-22) reached Live's 271s: 271 and 1295 are one old value
ANY = object()   # an old value: Heirloom's before (its audits in the entry's range), or the new value already
HL = object()    # a new value: Heirloom's current one


class Refused(Exception):
    pass


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def dumps(v):
    return json.dumps(v, sort_keys=True, separators=(",", ":"), ensure_ascii=False)


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def columns(db, tbl):
    return [r[1] for r in db.execute(f"PRAGMA table_info({tbl})")]


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    cols = [d[0] for d in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def where(tbl):
    return " AND ".join(f'"{c}" = ?' for c in KEYS[tbl])


def keyof(tbl, r):
    return tuple(r[c] for c in KEYS[tbl])


def full(db, tbl, key):
    r = rows(db, f"SELECT rowid AS _rowid_, * FROM {tbl} WHERE {where(tbl)}", tuple(key))
    return r[0] if r else None


def plain(r):
    return None if r is None else {c: v for c, v in r.items() if c != "_rowid_"}


def loose(a, b):
    """the same value (gm_audit's JSON writes a whole REAL as an integer)."""
    if isinstance(a, (int, float)) and isinstance(b, (int, float)) and not isinstance(a, bool):
        return abs(a - b) < 1e-6
    return a == b


def same(a, b):
    return type(a) is type(b) and a == b


def items_of(s):
    """a step's items string -> [(id, n)] ("8362, 981971", "993041*2")."""
    out = []
    for x in (s or "").split(","):
        x = x.strip()
        if x:
            a, _, n = x.partition("*")
            out.append((int(a), int(n or 1)))
    return out


class Plan:
    """one entry's rows: checked against Live as it stands (with this entry's own rows before them), written by apply."""

    def __init__(self, db, src, part, lo, hi=None):
        self.db, self.src, self.part, self.lo, self.hi = db, src, part, lo, hi or 10 ** 12
        self.ops = []          # (tbl, key, before row | None, after row | None)
        self.overlay = {}      # (tbl, key) -> row after this entry's ops (None: removed)
        self.lines, self.refusals = [], []
        self.alloc = {}        # tbl -> next id this entry hands out
        self.sym = {}          # a name -> a new row's Live id
        self.renames = RENAMES # old name (lower) -> new (lower), for the quests' step check (every entry's)

    # --- reading
    def say(self, s):
        self.lines.append(s)

    def refuse(self, s):
        self.refusals.append(s)

    def live(self, tbl, key):
        k = (tbl, tuple(key))
        if k in self.overlay:
            return self.overlay[k]
        return plain(full(self.db, tbl, key))

    def heir(self, tbl, key):
        return plain(full(self.src, tbl, key))

    def before(self, tbl, key):
        """Heirloom's row before its first audit in the entry's range: (found, row or None)."""
        rk = dumps(dict(zip(KEYS[tbl], key)))
        r = self.src.execute("SELECT before, action FROM gm_audit WHERE tbl = ? AND rowkey = ? AND id BETWEEN ? AND ? "
                             "AND action IN ('update', 'delete', 'insert') ORDER BY id LIMIT 1",
                             (tbl, rk, self.lo, self.hi)).fetchone()
        if r is None:
            return False, None
        return True, (json.loads(r[0]) if r[0] else None)

    # --- writing (each checked against what this entry has done so far)
    def put(self, tbl, key, after):
        key = tuple(key)
        have = self.live(tbl, key)
        if after is not None and have is not None and all(same(have.get(c), after[c]) for c in after):
            return False
        if after is None and have is None:
            return False
        self.ops.append((tbl, key, have, None if after is None else dict(after)))
        self.overlay[(tbl, key)] = None if after is None else dict(after)
        return True

    def new_id(self, tbl):
        if tbl not in self.alloc:
            kc = KEYS[tbl][0]
            m = self.db.execute(f"SELECT COALESCE(MAX({kc}), 0) FROM {tbl}").fetchone()[0]
            s = self.db.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (tbl,)).fetchone() \
                if table(self.db, "sqlite_sequence") else None
            self.alloc[tbl] = max(m, s[0] if s else 0) + 1
        i = self.alloc[tbl]
        self.alloc[tbl] += 1
        return i

    # --- the operations an entry is made of
    def must_exist(self, ids):
        for i in ids:
            l, h = self.live("itempattern", (i,)), self.heir("itempattern", (i,))
            if l is None:
                self.refuse(f"itempattern {i}: not on Live")
            elif h is not None and l["itemname"].lower() != h["itemname"].lower():
                self.refuse(f"itempattern {i}: Live's {l['itemname']!r}, Heirloom's {h['itemname']!r}")

    def new_items(self, spec):
        """{id: Live's raceuse | None (Heirloom's)}: copied whole at the same id."""
        made = 0
        for i, race in spec.items():
            h = self.heir("itempattern", (i,))
            if h is None:
                self.refuse(f"itempattern {i}: Heirloom has no such record")
                continue
            want = dict(h)
            if race is not None:
                want["raceuse"] = race
            l = self.live("itempattern", (i,))
            if l is not None and not all(loose(l[c], want[c]) for c in want):
                self.refuse(f"itempattern {i}: taken on Live by {l['itemname']!r} (not Heirloom's {h['itemname']!r} as it is)")
                continue
            if l is None:
                self.put("itempattern", (i,), want)
                made += 1
        self.say(f"items: {made} of {len(spec)} new records made at Heirloom's ids"
                 + (f" ({len(spec) - made} there already)" if made < len(spec) else ""))

    def edit(self, tbl, key, cols, what=None):
        """cols {col: (old, new)}: old ANY (Heirloom's before) or a value; new HL (Heirloom's now) or a value."""
        key = tuple(key)
        l = self.live(tbl, key)
        if l is None:
            self.refuse(f"{tbl} {key}: not on Live")
            return
        h = self.heir(tbl, key) if any(n is HL for _, n in cols.values()) or any(o is ANY for o, _ in cols.values()) else None
        found, b = self.before(tbl, key) if any(o is ANY for o, _ in cols.values()) else (False, None)
        new, bad = dict(l), []
        for c, (old, nv) in cols.items():
            if c not in l:
                bad.append(f"no column {c}")
                continue
            if nv is HL:
                if h is None:
                    bad.append(f"{c}: Heirloom has no row")
                    continue
                nv = h[c]
            if loose(l[c], nv):
                new[c] = nv if type(l[c]) is not type(nv) else l[c]
                continue
            if old is ANY:
                if not found or b is None:
                    bad.append(f"{c} is {l[c]!r}: not Heirloom's {nv!r}, and Heirloom has no audit before it to check")
                    continue
                if not (loose(l[c], b.get(c)) or (c == "raceuse" and isinstance(b.get(c), int) and l[c] == b[c] | FROGLOK)):
                    bad.append(f"{c} is {l[c]!r}, neither Heirloom's {nv!r} nor its before {b.get(c)!r}")
                    continue
            elif callable(old):
                if not old(l[c]):
                    bad.append(f"{c} is {l[c]!r}, not as the entry says")
                    continue
            elif not (loose(l[c], old) or (c == "raceuse" and l[c] == old | FROGLOK)):
                bad.append(f"{c} is {l[c]!r}, the entry says {old!r}")
                continue
            new[c] = nv
        name = what or l.get("itemname") or l.get("npc_name") or l.get("name") or ""
        if bad:
            self.refuse(f"{tbl} {key[0] if len(key) == 1 else key} {name}: " + "; ".join(bad))
            return
        ch = [c for c in cols if not same(l.get(c), new.get(c))]
        if self.put(tbl, key, new):
            self.say(f"  {tbl} {key[0] if len(key) == 1 else key} {name}: " + ", ".join(
                f"{c} {short(l[c])} -> {short(new[c])}" for c in ch))

    def items(self, spec):
        """{id: {col: (old, new)}} on itempattern."""
        for i, cols in spec.items():
            self.edit("itempattern", (i,), cols)

    def rename_key(self, tbl, old, new, extra=None):
        """a row keyed by a name (npcclass, lootcreature, npcspawnonly, npcaggro): its key old -> new."""
        l = self.live(tbl, (old,))
        if l is None:
            if self.live(tbl, (new,)) is not None:
                return
            self.refuse(f"{tbl} {old!r}: not on Live")
            return
        if self.live(tbl, (new,)) is not None:
            self.refuse(f"{tbl} {new!r}: there already beside {old!r}")
            return
        kc = KEYS[tbl][0]
        self.put(tbl, (old,), None)
        self.put(tbl, (new,), dict(l, **{kc: new}, **(extra or {})))
        self.say(f"  {tbl} {old!r} -> {new!r}")

    def rename_npc(self, nid, old, new):
        self.renames[old.lower()] = new.lower()
        self.edit("npcs", (nid,), {"npc_name": (old, new)})

    def copy_new(self, tbl, hkey, override=None, sym=None, src_db=None):
        """Heirloom's row (or Live's, src_db='live') copied under a new Live id."""
        h = plain(full(self.db, tbl, hkey)) if src_db == "live" else self.heir(tbl, hkey)
        if h is None:
            self.refuse(f"{tbl} {hkey}: {'Live' if src_db == 'live' else 'Heirloom'} has no such row")
            return None
        nid = self.new_id(tbl)
        v = dict(h, **(override or {}))
        v[KEYS[tbl][0]] = nid
        self.put(tbl, (nid,), v)
        if sym:
            self.sym[sym] = nid
        label = h.get("npc_name") or h.get("name") or h.get("tablename") or h.get("patternid") or ""
        self.say(f"  {tbl} {nid}: a copy of {'Live' if src_db == 'live' else 'Heirloom'}'s {hkey[0]} {label}"
                 + (f" ({', '.join(f'{k} {short(x)}' for k, x in (override or {}).items())})" if override else ""))
        return nid

    def copy_same(self, tbl, key, override=None):
        """Heirloom's row at the same key (a name-keyed row)."""
        h = self.heir(tbl, key)
        if h is None:
            self.refuse(f"{tbl} {key}: Heirloom has no such row")
            return
        v = dict(h, **(override or {}))
        l = self.live(tbl, key)
        if l is not None:
            if all(loose(l[c], v[c]) for c in v):
                return
            self.refuse(f"{tbl} {key}: Live has a row of its own")
            return
        self.put(tbl, key, v)
        self.say(f"  {tbl} {key}: copied from Heirloom")

    def insert(self, tbl, key, values):
        l = self.live(tbl, key)
        if l is not None:
            if all(loose(l[c], values[c]) for c in values):
                return
            self.refuse(f"{tbl} {key}: Live has a row of its own")
            return
        self.put(tbl, key, values)

    def delete(self, tbl, key, expect):
        l = self.live(tbl, key)
        if l is None:
            self.say(f"  {tbl} {key}: gone already")
            return
        bad = [f"{c} {l[c]!r} (not {v!r})" for c, v in expect.items() if not loose(l[c], v)]
        if bad:
            self.refuse(f"{tbl} {key}: not the row the entry deletes: " + ", ".join(bad))
            return
        self.put(tbl, key, None)
        self.say(f"  {tbl} {key}: deleted ({', '.join(f'{c} {short(v)}' for c, v in expect.items())})")

    def stock(self, hrow, npcid, listnumber):
        """Heirloom's npcitem row copied to the Live merchant npcid (its own id), the list number the entry gives."""
        h = self.heir("npcitem", (hrow,))
        if h is None:
            self.refuse(f"npcitem {hrow}: Heirloom has no such row")
            return
        if self.live("npcs", (npcid,)) is None:
            self.refuse(f"npcitem {hrow}: Live has no npcs {npcid}")
            return
        hn = self.heir("npcs", (h["npcid"],))
        ln = self.live("npcs", (npcid,))
        if hn is None or hn["npc_name"] != ln["npc_name"]:
            self.refuse(f"npcitem {hrow}: Heirloom's merchant {h['npcid']} is not Live's {npcid} {ln['npc_name']!r}")
            return
        have = rows(self.db, "SELECT itemid, listnumber FROM npcitem WHERE npcid = ? AND patternid = ?", (npcid, h["patternid"]))
        if have:
            self.say(f"  npcitem: {ln['npc_name']} sells {h['patternid']} already (row {have[0]['itemid']})")
            return
        taken = rows(self.db, "SELECT itemid, patternid FROM npcitem WHERE npcid = ? AND listnumber = ?", (npcid, listnumber))
        taken += [r for (t, _), r in self.overlay.items() if t == "npcitem" and r and r["npcid"] == npcid
                  and r["listnumber"] == listnumber]
        if taken or listnumber != h["listnumber"]:
            self.refuse(f"npcitem {hrow}: {ln['npc_name']}'s list number {listnumber} is taken on Live or not Heirloom's "
                        f"{h['listnumber']}")
            return
        self.copy_new("npcitem", (hrow,), {"npcid": npcid})

    def step_target(self, q, s, old, new):
        self.renames[old.lower()] = new.lower()
        self.edit("queststeps", (q, s), {"target": (old, new)}, what="target")

    # --- quests
    def old_steps(self, q):
        """Heirloom's steps of q before the entry's audits: {step: row} (None when it has no audit for the quest)."""
        audited = rows(self.src, "SELECT rowkey, before, action, id FROM gm_audit WHERE tbl = 'queststeps' AND rowkey LIKE ? "
                                 "AND id BETWEEN ? AND ? ORDER BY id", (f'{{"questid":{q},%', self.lo, self.hi))
        if not audited:
            return None
        first = {}
        for a in audited:
            k = json.loads(a["rowkey"])
            if k.get("questid") == q and k["step"] not in first:
                first[k["step"]] = json.loads(a["before"]) if a["before"] else None
        out = {}
        for s in rows(self.src, "SELECT * FROM queststeps WHERE questid = ?", (q,)):
            if s["step"] not in first:
                out[s["step"]] = s
        for st, b in first.items():
            if b is not None:
                out[st] = b
        return out

    def quest_check(self, q, live_steps):
        """Live's steps of q are Heirloom's before (kinds, items, targets by the renames; words may differ)."""
        old = self.old_steps(q)
        if old is None:
            return []
        bad = []
        if sorted(old) != sorted(live_steps):
            bad.append(f"steps {sorted(live_steps)} on Live, Heirloom had {sorted(old)}")
            return bad
        for st in sorted(old):
            o, l = old[st], live_steps[st]
            for c in ("kind", "items"):
                if not loose(o.get(c), l.get(c)):
                    bad.append(f"step {st} {c} {short(l.get(c))} (Heirloom had {short(o.get(c))})")
            ot, lt = (o.get("target") or "").lower(), (l.get("target") or "").lower()
            if ot != lt and self.renames.get(ot) != lt and self.renames.get(lt) != ot and ot != self.renames.get(lt, lt) \
                    and self.renames.get(ot, ot) != self.renames.get(lt, lt):
                bad.append(f"step {st} target {l.get('target')!r} (Heirloom had {o.get('target')!r})")
        return bad

    def quests(self, qids, overrides=None, check=True, nocheck=()):
        """quests, queststeps, queststeptargets of qids made Heirloom's; overrides {(q, step): {col: value | fn}}."""
        overrides = overrides or {}
        nq = ns = nd = nt = 0
        for q in qids:
            hq = self.heir("quests", (q,))
            if hq is None:
                self.refuse(f"quest {q}: Heirloom has none")
                continue
            hsteps = {s["step"]: plain(s) for s in rows(self.src, "SELECT * FROM queststeps WHERE questid = ?", (q,))}
            for (oq, ost), ov in overrides.items():
                if oq != q:
                    continue
                if ost not in hsteps:
                    self.refuse(f"quest {q}: no step {ost} on Heirloom for the entry's Live change")
                    continue
                for c, v in ov.items():
                    hsteps[ost][c] = v(hsteps[ost][c]) if callable(v) else v
            lsteps = {s["step"]: plain(s) for s in rows(self.db, "SELECT * FROM queststeps WHERE questid = ?", (q,))}
            for (t, k), r in self.overlay.items():
                if t == "queststeps" and k[0] == q:
                    if r is None:
                        lsteps.pop(k[1], None)
                    else:
                        lsteps[k[1]] = r
            ht = set(self.src.execute("SELECT step, target FROM queststeptargets WHERE questid = ?", (q,)).fetchall())
            lt = set(self.db.execute("SELECT step, target FROM queststeptargets WHERE questid = ?", (q,)).fetchall())
            lq = self.live("quests", (q,))
            done = (lq is not None and all(loose(lq[c], hq[c]) for c in hq) and sorted(lsteps) == sorted(hsteps)
                    and all(all(loose(lsteps[s][c], hsteps[s][c]) for c in hsteps[s]) for s in hsteps) and ht == lt)
            if done:
                continue
            if check and lq is not None and q not in nocheck:
                bad = self.quest_check(q, lsteps)
                if bad:
                    self.refuse(f"quest {q} {lq['name']!r}: not as Heirloom had it before: " + "; ".join(bad))
                    continue
            nq += self.put("quests", (q,), hq)
            for st in sorted(hsteps):
                ns += self.put("queststeps", (q, st), hsteps[st])
            for st in sorted(lsteps):
                if st not in hsteps:
                    nd += self.put("queststeps", (q, st), None)
            for st, tg in sorted(lt - ht):
                nt += self.put("queststeptargets", (q, st, tg), None)
            for st, tg in sorted(ht - lt):
                nt += self.put("queststeptargets", (q, st, tg), {"questid": q, "step": st, "target": tg})
            gone, added = sorted(lt - ht), sorted(ht - lt)
            self.say(f"  quest {q} {hq['name']!r}: {len(lsteps)} -> {len(hsteps)} steps"
                     + (f"; targets removed {gone}" if gone else "") + (f"; added {added}" if added else ""))
        self.say(f"quests: {nq} quests rows, {ns} steps written, {nd} removed, {nt} step targets")

    def quest_cols(self, q, qcols=(), steps=None):
        """only these columns of quest q (and of its steps) from Heirloom."""
        hq, lq = self.heir("quests", (q,)), self.live("quests", (q,))
        if hq is None or lq is None:
            self.refuse(f"quest {q}: missing on {'Heirloom' if hq is None else 'Live'}")
            return
        if qcols and self.put("quests", (q,), dict(lq, **{c: hq[c] for c in qcols})):
            self.say(f"  quest {q}: " + ", ".join(f"{c} {short(lq[c])} -> {short(hq[c])}" for c in qcols if lq[c] != hq[c]))
        for st, cols in (steps or {}).items():
            h, l = self.heir("queststeps", (q, st)), self.live("queststeps", (q, st))
            if h is None or l is None:
                self.refuse(f"queststeps {q}/{st}: missing on {'Heirloom' if h is None else 'Live'}")
                continue
            if self.put("queststeps", (q, st), dict(l, **{c: h[c] for c in cols})):
                self.say(f"  quest {q} step {st}: {', '.join(cols)} as Heirloom's")

    # --- characters
    def chars(self, quests, moves=None, keep=(), ask=(), ask_all=False, gives=(), takes=()):
        """characters part-way on quests (done 0): moves {q: {old: new | (new, [gives], keep_count)}}; keep: the
        (serverid, q, step) the entry read; ask: quests (or (q, steps)) where anyone else refuses; ask_all: anyone not in
        keep refuses; gives: [(serverid, q, step, patternid, n)] for named characters; takes: [(serverid, itemid, pattern)]."""
        moves = moves or {}
        kept = {(s, q): st for s, q, st in keep}
        asks = {}
        for a in ask:
            if isinstance(a, tuple):
                asks[a[0]] = set(a[1])
            else:
                asks[a] = None
        marks = ",".join("?" * len(quests))
        for c in rows(self.db, f"SELECT * FROM charquests WHERE questid IN ({marks}) AND done = 0 AND step > 0 "
                               "ORDER BY questid, serverid", list(quests)):
            sid, q, step = c["serverid"], c["questid"], c["step"]
            who = f"character {sid}"
            if sid in T_HOLD:
                self.say(f"  {who} on {q} step {step}: ONLINE, left as it is (a GM finishes it)")
                continue
            if (sid, q) in kept and kept[(sid, q)] != step:
                self.refuse(f"{who} on {q} step {step}: the entry read step {kept[(sid, q)]}")
                continue
            # Live's steps before this entry (its rows are not written until the entry is planned whole)
            old = {s["step"]: s for s in rows(self.db, "SELECT * FROM queststeps WHERE questid = ?", (q,))}
            new = {s["step"]: s for s in rows(self.db, "SELECT * FROM queststeps WHERE questid = ?", (q,))}
            for (t, k), r in self.overlay.items():
                if t == "queststeps" and k[0] == q:
                    if r is None:
                        new.pop(k[1], None)
                    else:
                        new[k[1]] = r
            if (sid, q) not in kept:
                if q in asks and (asks[q] is None or step in asks[q]):
                    self.refuse(f"{who} on {q} step {step}: the entry says to ask jibrish")
                    continue
                if ask_all:
                    o, n = old.get(step), new.get(step)
                    if not (o is not None and n is not None and all(loose(o.get(k), n.get(k)) for k in ("kind", "target", "items"))):
                        self.refuse(f"{who} on {q} step {step}: not read by the entry, and the step changes: ask jibrish")
                        continue
            m = moves.get(q, {}).get(step, step)
            if m == "ask":
                self.refuse(f"{who} on {q} step {step}: the entry says to ask jibrish")
                continue
            to, give, keepc = (m, [], False) if isinstance(m, int) else (m[0], list(m[1]), m[2] if len(m) > 2 else False)
            if to not in new:
                self.refuse(f"{who} on {q} step {step}: no step {to} after the copy")
                continue
            o, n = old.get(step), new[to]
            samestep = o is not None and all(loose(o.get(k), n.get(k)) for k in ("kind", "target", "items"))
            count = c["count"] if (keepc or (to == step and (samestep or q not in moves))) else 0
            for gs, gq, gst, gp, gn in gives:
                if gs == sid and gq == q and gst == step:
                    give.append((gp, gn))
            if to != step or count != c["count"]:
                self.put("charquests", (sid, q), dict(plain(c), step=to, count=count))
                self.say(f"  {who}: {q} step {step} (count {c['count']}) -> step {to} (count {count})")
            else:
                self.say(f"  {who}: {q} step {step}: stays")
            for g in give:
                pid, n_ = (g, 1) if isinstance(g, int) else g
                self.give(sid, pid, n_)
        for sid, itemid, pid in takes:
            if sid in T_HOLD:
                self.say(f"  character {sid}: ONLINE, charinventory {itemid} left")
                continue
            r = self.live("charinventory", (itemid,))
            if r is None:
                self.say(f"  character {sid}: charinventory {itemid} gone already")
            elif r["serverid"] != sid or r["patternid"] != pid:
                self.refuse(f"charinventory {itemid}: not character {sid}'s {pid}")
            else:
                self.put("charinventory", (itemid,), None)
                self.say(f"  character {sid}: item {pid} (charinventory {itemid}) taken away")

    def give(self, sid, pid, n=1):
        if self.live("itempattern", (pid,)) is None:
            self.refuse(f"give {pid} to {sid}: no such record on Live (after this entry)")
            return
        held = self.db.execute("SELECT COALESCE(SUM(stackleft), 0) FROM charinventory WHERE serverid = ? AND patternid = ?",
                               (sid, pid)).fetchone()[0]
        held += sum(r["stackleft"] for (t, _), r in self.overlay.items()
                    if t == "charinventory" and r and r["serverid"] == sid and r["patternid"] == pid)
        if held >= n:
            self.say(f"  character {sid}: holds {pid} already")
            return
        pack = self.db.execute("SELECT count(*), COALESCE(MAX(listnumber), -1) FROM charinventory WHERE serverid = ? "
                               "AND location = 0", (sid,)).fetchone()
        mine = [r for (t, _), r in self.overlay.items() if t == "charinventory" and r and r["serverid"] == sid]
        if pack[0] + len(mine) >= INV_CAP:
            self.say(f"  character {sid}: pack full, NOT given {pid} (a GM gives it)")
            return
        it = self.live("itempattern", (pid,))
        nid = self.new_id("charinventory")
        self.put("charinventory", (nid,), {"itemid": nid, "serverid": sid, "stackleft": n - held,
                                            "remainHP": it["maxhp"] or 0, "remaincharge": 0, "patternid": pid,
                                            "equiploc": -1, "location": 0,
                                            "listnumber": max([pack[1]] + [r["listnumber"] for r in mine]) + 1})
        self.say(f"  character {sid}: given {pid} x{n - held}")


def short(v):
    s = repr(v)
    return s if len(s) <= 60 else s[:57] + "...'"


# ---------------------------------------------------------------- the entries
R_DE, R_HALF, R_ERU, R_GNO = 4, 128, 256, 8
TOME = lambda s: s.replace("997854", "981971")  # Heirloom's alchemical tome -> Live's


def e1965(p):
    p.new_items({991700: R_DE, 991701: None, 991702: None, 991703: None, 991728: R_DE, 991723: None, 991725: None,
                 991729: None})
    p.items({
        700421: {"itemcost": (189, 105)},
        700433: {"itemname": ("giant widow fang", "giant widow venom"), "itemdesc": (ANY, HL)},
        700434: {"itemname": ("leafcutter ant mandible", "giant leafcutter ant saliva"), "itemdesc": (ANY, HL)},
        7942: {"classuse": (32767, 16384), "raceuse": (1023, 4)},
        700414: {"classuse": (4608, 512), "trade": (0, 1), "lore": (0, 1), "intelligence": (3, 0), "wis": (0, 3)},
        8304: {"itemdesc": ("Increases the Strength and Stamina of the target.", "This allows you to scribe Minor Blessing.")},
        700442: {"itemdesc": (ANY, HL)},
        7271: {"raceuse": (1023, 4)}, 5277: {"raceuse": (1023, 4)}, 6360: {"raceuse": (1023, 4)},
        5735: {"raceuse": (1023, 6)},
    })
    p.must_exist([5277, 5735, 5834, 6055, 6360, 7271, 981971])
    p.rename_npc(101815, "Denouncer R'Nox", "Denouncer R`Noxt")
    p.rename_key("npcclass", "denouncer r'nox", "denouncer r`noxt")
    if p.live("npcfaction", (496,)) is None or p.live("npcfaction", (496,))["name"] != "denouncer r`noxt":
        p.refuse("npcfaction 496: not \"denouncer r`noxt\" on Live")
    p.delete("npcfaction", (495,), {"name": "denouncer r'nox", "faction": "Citizens of Hazinak", "world": -1})
    p.rename_npc(122112, "Thrun", "Tharn")
    p.edit("lootpool", (11024,), {"npc_name": ("thrun", "tharn"), "patternid": (707409, 707409)})
    p.edit("lootpool", (11025,), {"npc_name": ("thrun", "tharn"), "patternid": (707914, 707914)})
    p.rename_key("lootcreature", "thrun", "tharn")
    # Lord U`Dedne: spawn-only first, so he never stands
    p.copy_same("npcspawnonly", ("Lord U`Dedne",))
    p.copy_new("npcs", (196152,))
    for f in (7098, 7099, 7100):
        p.copy_new("factionhits", (f,))
    p.copy_new("npcfaction", (2980,))
    p.stock(58739, 100400, 17)
    p.quests(range(100417, 100436), {(100426, 9): {"choices": TOME}})
    p.chars(range(100417, 100436), moves={
        100417: {2: 1}, 100420: {3: 2}, 100422: {2: (2, [991701])},
        100424: {2: (3, [991702]), 3: (4, [991702]), 4: (5, [991702]), 5: "ask", 6: 7},
        100426: {6: (6, [700437, 991703])},
        100427: {s: 1 for s in range(1, 20)}, 100429: {s: 1 for s in range(1, 20)},
        100430: {s: 1 for s in range(1, 20)}, 100431: {s: 1 for s in range(1, 20)},
        100432: {1: (1, [991729])},
        100433: {6: (6, [700444]), 8: 7, 9: 7},
        100434: {7: 6, 8: 7, 9: 8},
    }, keep=[(751, 100423, 4), (208, 100429, 1), (231, 100432, 1)])


def e2011(p):
    p.new_items({991750: R_DE, 991751: R_DE, 991752: R_DE, 991753: None, 991770: R_DE, 991771: R_DE, 991732: None,
                 991724: None, 991726: None, 991727: None})
    p.items({
        700415: {"itemdesc": (lambda v: v.startswith("A silken sash dyed a light blue to represent maastery"), HL)},
        8418: {"itemdesc": (lambda v: v.startswith("This allows yout to scribe"), "This allows you to scribe Lumbering Arms.")},
        700449: {"itemname": ("sea of chains", "Sea of Chains"), "itemicon": (-2061508594, 1856243648),
                 "itemdesc": ("A tangle of fine chains.", HL)},
        700445: {"equipslot": (8, -1), "levelreq": (1, 0), "classuse": (1024, 0), "raceuse": (1, 0), "POWMAX": (5, 0),
                 "maxhp": (200, 0), "itemdesc": ("The ring Dominary K'Jartan gave Xeruin.", HL)},
        700453: {"itemname": ("Du'Klot's scroll", "Du`Klot's scroll"), "itemdesc": ("Counselor Du'Klot's scroll.", HL)},
        5184: {"levelreq": (5, 4), "AC": (2, 0), "FR": (0, 2)},
        700424: {"itemcost": (0, 40)},
        700420: {"trade": (0, 1), "lore": (0, 1)},
        700481: {"itemdesc": ("Scornmaster U'Dedne's orders.", HL)},
        700482: {"itemname": ("J'Naris's response", "J`Narus's response"), "itemdesc": ("Scornblade J'Naris's response.", HL)},
    })
    p.must_exist([4893, 8415, 700423, 4889, 700425, 8416, 8417, 700444, 700446, 700447, 700450, 4852, 8317, 8318, 707000,
                  8565, 5702, 8566, 713015, 8567])
    p.rename_npc(122131, "Xeruin", "Xeruin Du`Klot")
    p.edit("lootpool", (11662,), {"npc_name": ("xeruin", "xeruin du`klot"), "patternid": (707398, 707398)})
    p.edit("lootpool", (11663,), {"npc_name": ("xeruin", "xeruin du`klot"), "patternid": (707903, 707903)})
    p.rename_key("lootcreature", "xeruin", "xeruin du`klot")
    p.copy_new("npcs", (196275,), sym="vodleth")
    p.copy_new("npcfaction", (2983,))
    p.copy_new("npcs", (196276,))
    p.delete("npcitem", (3079,), {"npcid": 1906, "patternid": 700449, "listnumber": 7})
    p.edit("quests", (100446,), {"giver": ("Dominary K`Jartan", "Enchanter T`Crit"), "decline": ("Not now.", ""),
                                 "declined": ("Then go. Come back when you are ready.", "")})
    p.edit("quests", (100496,), {"giver": ("Scornblade D`Eleth", "Scornblade Vo`Dleth"), "decline": (ANY, ""),
                                 "declined": (ANY, "")})
    p.quests(list(range(100437, 100446)) + list(range(100487, 100496)))
    p.chars(list(range(100437, 100446)) + list(range(100487, 100496)), moves={
        100437: {2: 1}, 100439: {2: 1}, 100440: {2: (1, [700423, 700424]), 3: (1, [700423, 700424])}, 100441: {2: 1},
        100443: {3: 2, 6: (6, [700444]), 8: 7, 9: 8}, 100444: {7: 6, 8: 7},
        100487: {2: 1}, 100489: {2: 1}, 100490: {2: (1, [8317, 8318]), 3: (1, [8317, 8318])}, 100491: {2: 1},
        100493: {3: 2, 6: (6, [700444]), 8: 7, 9: (8, [991724])}, 100494: {7: 6, 8: 6},
    }, keep=[(732, 100440, 1), (185, 100492, 1)])


def e1982(p):
    p.new_items({991855: R_DE, 991857: R_DE, 991859: R_DE, 991856: None, 991858: None})
    p.items({
        700418: {"itemname": ("thug scarf", "thug's scarf"), "levelreq": (23, 1), "maxhp": (4600, 200),
                 "itemcost": (1065, 0), "classuse": (32767, 64), "raceuse": (1023, 4), "lore": (0, 1), "PR": (10, 0),
                 "AR": (9, 0), "HPMAX": (0, 21), "itemdesc": ("A rogue's scarf.", HL)},
        700479: {"itemname": ("Biljoe's taxes", "fisherman's taxes"), "itemdesc": (ANY, HL)},
        5590: {"maxhp": (2000, 2600), "raceuse": (1023, 4)},
        7314: {"maxhp": (3000, 3600), "raceuse": (1023, 4)},
        6015: {"maxhp": (4000, 4600), "raceuse": (1023, 4)},
    })
    p.must_exist([4853, 4882, 8316, 8318, 700425, 707018, 8541, 707019, 8431, 8433, 8434, 8435, 8545]
                 + list(range(8436, 8445)) + [8547, 8548, 8549])
    p.copy_new("npcs", (196251,))
    p.stock(58981, 1666, 12)
    if rows(p.db, "SELECT id FROM npcdialog WHERE name = 'Guildmaster Kriyn'"):
        p.refuse("npcdialog: Live has a row for Guildmaster Kriyn already")
    else:
        p.copy_new("npcdialog", (3116,))
    p.quests(range(100467, 100477), nocheck={100476})  # Live's step 3 is 1043's tent reach, which the copy keeps
    p.chars(range(100467, 100477), moves={
        100467: {2: 1}, 100469: {2: 1}, 100470: {2: (1, [8316]), 3: (1, [8316])}, 100471: {2: 1},
        100472: {1: (1, [991856]), 2: 6, 3: 11}, 100473: {2: 1, 3: (3, [991858])},
        100474: {1: (1, [8433]), 2: 3, 3: 4, 4: 5, 5: (6, [8435])},
        100475: {4: 5, 5: 7, 6: 9, 7: 10},
        100476: {s: (s, [8441]) for s in range(6, 11)},
    }, keep=[(804, 100472, 1)])


def e1986(p):
    p.new_items({991900: None, 991901: None, 991904: None, 991905: None, 991906: None, 991909: None, 991910: None,
                 991940: None, 991941: None, 991942: None, 991902: R_HALF, 991903: R_HALF, 991907: R_HALF,
                 991908: R_HALF, 991911: R_HALF, 991912: R_HALF, 991943: R_HALF, 991944: R_HALF})
    p.items({
        5860: {"maxhp": (800, 1000), "raceuse": (1023, 128)}, 6056: {"maxhp": (2000, 2600), "raceuse": (1023, 128)},
        6705: {"maxhp": (3000, 3600), "raceuse": (1023, 128)}, 7382: {"raceuse": (1023, 128)},
        8509: {"raceuse": (3, 131)},
        700638: {"itemname": ("Bag of Old Items", HL), "itemdesc": (ANY, HL)},
        700641: {"itemname": ("Cleric Regents", HL), "itemdesc": (ANY, HL)},
        700658: {"itemname": ("Kithicor Pinecone", "Kithicor Pine Cone")},
        700664: {"itemname": ("Suspicious Looking Stone", HL), "itemdesc": (ANY, HL)},
        700652: {"itemname": ("Goblin Shaman's Staff", HL), "itemdesc": (ANY, HL)},
        700639: {"itemdesc": (ANY, HL)},
        700663: {"itemname": ("Standing Stone Scrapings", HL), "itemdesc": (ANY, HL)},
        700648: {"itemname": ("Goblin Foreman's Mallet", HL), "itemdesc": (ANY, HL)},
        700650: {"itemname": ("Goblin Hoarder's Totem", HL), "itemdesc": (ANY, HL)},
    })
    p.rename_npc(2063, "Orial Magal", "Orial Maga")
    p.rename_npc(122376, "Barnimus Giggletoes", "Barnibus Giggletoe")
    p.rename_npc(122378, "Stalis Slithertonuge", "Manium Slithertongue")
    p.rename_npc(2225, "Rondo Thistlefoot", "Ronlo Thistlefoot")
    p.rename_npc(2989, "Rondo Thistlefoot", "Ronlo Thistlefoot")
    p.rename_key("npcclass", "orial magal", "orial maga")
    p.rename_key("npcclass", "rondo thistlefoot", "ronlo thistlefoot")
    p.rename_key("lootcreature", "rondo thistlefoot", "ronlo thistlefoot")
    p.edit("lootpool", (9922,), {"npc_name": ("rondo thistlefoot", "ronlo thistlefoot")})
    p.edit("lootpool", (9923,), {"npc_name": ("rondo thistlefoot", "ronlo thistlefoot")})
    p.copy_new("npcs", (196149,))
    priest = p.copy_new("npcs", (196150,), sym="priest")
    h = p.heir("npcspawnonly", ("#196150",))
    if h is None:
        p.refuse("npcspawnonly '#196150': Heirloom has none")
    elif priest:
        p.insert("npcspawnonly", (f"#{priest}",), dict(h, name=f"#{priest}"))
        p.say(f"  npcspawnonly '#{priest}' (the goblin priest)")
    p.stock(58738, 106507, 3)
    lt = p.copy_new("loottables", (5078,))
    rats = (148259, 2175, 2176, 2177, 2178, 2179, 2180, 2181, 116928, 137752, 139441, 140911, 141996, 142074)
    for r in rats:
        hn, ln = p.heir("npcs", (r,)), p.live("npcs", (r,))
        if ln is None or hn is None or ln["npc_name"] != "rat" or ln["world"] != 0 or ln["npc_level"] != hn["npc_level"] \
                or abs(ln["x_coord"] - hn["x_coord"]) > 1 or abs(ln["z_coord"] - hn["z_coord"]) > 1:
            p.refuse(f"npcs {r}: not Heirloom's Misty Thicket rat on Live")
            continue
        h = p.heir("npcloot", (f"#{r}", "rivervale druid rat fur"))
        if h is None:
            p.refuse(f"npcloot #{r}: Heirloom has no rat fur row")
            continue
        p.insert("npcloot", (f"#{r}", "rivervale druid rat fur"), h)
    if lt:
        p.say(f"  npcloot: {len(rats)} rats bound to 'rivervale druid rat fur'")
    p.quests(range(100647, 100667))
    p.chars(range(100647, 100667), keep=[(620, 100648, 2), (771, 100648, 3), (34, 100657, 1), (684, 100658, 3),
                                          (772, 100658, 3)],
            gives=[(620, 100648, 2, 720001, 1)], ask=(100650, 100653, 100654, 100660))


def e1987(p):
    g = R_HALF
    p.new_items({991980: 991, 991981: g, 991982: g, 991983: g, 991984: g, 991985: g, 992020: g, 992021: g, 992028: g,
                 992031: g, 992042: g, 992045: g, 992022: None, 992023: None, 992024: None, 992025: None, 992026: None,
                 992027: None, 992029: None, 992030: None, 992032: None, 992033: None, 992040: None, 992041: None,
                 992043: None, 992044: None})
    p.items({
        5775: {"maxhp": (4000, 4600), "raceuse": (1023, 128)},
        700667: {"itemname": ("Thief's Loot Bag", "Melvin's Money"), "itemdesc": (ANY, HL)},
        700642: {"itemname": ("Extra Loot Bag", "Extra Tunar"), "itemdesc": (ANY, HL)},
        700647: {"itemname": ("Goblin Courier's Plans", "Goblin Attack Plans"), "itemdesc": (ANY, HL)},
        700649: {"itemname": ("Goblin Hoarder's Chalice", "Gold-Flecked Chalice"), "itemdesc": (ANY, HL)},
    })
    for i in (5833, 6694):
        l = p.live("itempattern", (i,))
        if l is None or l["raceuse"] != 128:
            p.refuse(f"itempattern {i}: raceuse {None if l is None else l['raceuse']} on Live (128 expected, kept)")
    p.edit("spellpattern", (700020,), {"abilitylvl": (27, 3)})
    p.stock(58988, 106507, 4)
    p.stock(58989, 106507, 5)
    names = ["Sheriff Hopper", "Spiritmaster Rory", "Marshall Drake", "Tonrin Fopfoot", "Marlo Steeltoe", "Captain Rolley",
             "Commander Krilshek", "Goblin Slaver", "goblin champion", "bixie", "bixie drone", "Tailor Sylle",
             "Spiritmaster Willa", "Coachman Huff", "Walt Deeppockets", "Thom Gubbin", "Deputy Jonkers", "Grocer Sloan",
             "Zale Earblower", "Donpo Risslemit", "Sophie Ghobber", "Grandma Gubbin", "Fanny Diggs", "Zarene Earblower",
             "goblin courier", "goblin surveyor", "goblin digger", "goblin warrior", "Nena Bimberry", "Brend Groomer",
             "Commander Artheg", "Deputy Pendy", "Dark Elf spoiler", "goblin hoarder", "Orial Maga",
             "Barnibus Giggletoe", "Manium Slithertongue"]
    for n in names:
        if not p.db.execute("SELECT 1 FROM npcs WHERE npc_name = ? COLLATE NOCASE LIMIT 1", (n,)).fetchone():
            p.refuse(f"npcs: no {n!r} on Live")
    p.quests(range(100667, 100687))
    p.chars(range(100667, 100687), keep=[(769, 100668, 3), (770, 100678, 3)],
            ask=(100670, 100673, 100679, 100680, 100682, 100683, 100684))


def e1945(p):
    p.new_items({993500: R_ERU, 993501: R_ERU, 993502: None, 993503: None, 993504: None, 993505: None})
    A = (ANY, HL)
    p.items({
        700241: {"itemdesc": A}, 700246: {"itemdesc": A, "itemicon": A},
        700248: {"itemname": A, "itemdesc": A, "itemicon": A, "procanim": A},
        700253: {"itemname": A, "itemdesc": A}, 700255: {"itemdesc": A, "itemicon": A, "procanim": A},
        700259: {"itemdesc": A}, 700264: {"itemname": A, "itemdesc": A, "itemicon": A, "procanim": A},
        700270: {"itemname": A, "itemdesc": A}, 700272: {"itemname": A, "itemdesc": A},
        700287: {"itemname": A, "itemdesc": A, "itemicon": A, "procanim": A}, 8361: {"itemdesc": A},
        5492: {"raceuse": (1023, 256)},
    })
    p.must_exist([981971])
    p.rename_npc(105900, "Tiltam Nebaulm", "Tilliam Nebaulm")
    p.rename_npc(122258, "Mayor Reively", "Mayor Reivley")
    for q, s in ((100247, 10), (100256, 10), (100266, 10), (100266, 12), (100286, 10), (100296, 10)):
        p.step_target(q, s, "Mayor Reively", "Mayor Reivley")
    rad = p.copy_new("npcs", (196194,), sym="radavex")
    for r in range(196196, 196202):
        p.copy_new("npcs", (r,))
    if rad:
        p.quests(range(100221, 100231), {(100230, 14): {"npcid": rad, "choices": TOME}})
    p.chars(range(100221, 100231), keep=[(783, 100224, 2)], ask_all=True)


ERU_ARC = list(range(100241, 100267)) + list(range(100277, 100287)) + list(range(100292, 100297)) + list(range(105210, 105214))


def e1992(p):
    tok = [993570, 993571, 993572, 993573, 993574, 993575, 993576, 993577, 993582, 993583, 993586, 993587, 993592,
           993593, 993650]
    gear = [993578, 993579, 993580, 993581, 993584, 993585, 993588, 993589, 993590, 993591, 993594, 993600, 993601,
            993635, 993636, 993651, 993665]
    p.new_items({**{i: None for i in tok}, **{i: R_ERU for i in gear}})
    A = (ANY, HL)
    ed = {700273: {"lore": A, "itemdesc": A}, 700266: {"itemname": A, "itemdesc": A},
          700289: {"itemname": A, "itemicon": A, "itemdesc": A}, 700238: {"itemname": A, "itemdesc": A},
          700280: {"levelreq": A, "itemdesc": A},
          707115: {"sta": A, "POWMAX": A, "AR": A, "DR": A, "itemdesc": A}, 5843: {"maxhp": A}, 7544: {"maxhp": A}}
    for i in (700260, 700256, 700244):
        ed[i] = {"lore": A, "itemdesc": A}
    for i in (700249, 700235, 700251, 700268):
        ed[i] = {"itemicon": A, "itemdesc": A}
    for i in (700267, 700254, 700240, 700283, 707116, 700282, 700261, 700277, 700263, 707114, 700262, 700271, 707117,
              700281):
        ed[i] = {"itemdesc": A}
    for i, old in ((7544, 1023), (5598, 1023), (5919, 1023), (5920, 1023), (707102, 1023), (707114, 271),
                   (707115, 1023), (707116, 1023), (707117, 271)):
        ed.setdefault(i, {})["raceuse"] = (old, 256)
    p.items(ed)
    p.edit("npcitem", (6155,), {"npcid": (106034, 106034), "patternid": (5151, 993635)}, what="Merchant Shansha")
    ren = ((106001, "Spiritmaster Keystone", "Spiritmaster Keyston"), (106002, "Keeper Toshei", "Keeper Toshel"),
           (105948, "Retainer Alishal", "Retainer Alishai"), (121685, "Hagar Loftin", "Hagar Loftlin"),
           (102154, "Nalai Belzia", "Nalej Belzia"))
    low = {o.lower(): n.lower() for _, o, n in ren}
    for nid, o, n in ren:
        p.rename_npc(nid, o, n)
        p.rename_key("npcclass", o.lower(), n.lower())
    for fid in (2346, 2872, 1982, 1836):
        l = p.live("npcfaction", (fid,))
        if l is None or l["name"] not in low:
            p.refuse(f"npcfaction {fid}: {None if l is None else l['name']!r} is none of the old names")
            continue
        p.edit("npcfaction", (fid,), {"name": (l["name"], low[l["name"]])})
    p.rename_key("lootcreature", "hagar loftin", "hagar loftlin")
    p.rename_key("lootcreature", "retainer alishal", "retainer alishai")
    for lp in (7778, 7779, 9874, 9875):
        l = p.live("lootpool", (lp,))
        if l is None or l["npc_name"] not in low:
            p.refuse(f"lootpool {lp}: {None if l is None else l['npc_name']!r} is none of the old names")
            continue
        p.edit("lootpool", (lp,), {"npc_name": (l["npc_name"], low[l["npc_name"]])})
    for fh in (646, 2435):
        p.edit("factionhits", (fh,), {"npc_name": ("Retainer Alishal", "Retainer Alishai")})
    p.edit("quests", (100819,), {"giver": ("Retainer Alishal", "Retainer Alishai")})
    near = lambda want: (lambda v: abs(v - want) < 0.2)
    p.edit("npcs", (122265,), {"x_coord": (near(5552.9), 7240.0), "y_coord": (near(57.7), 69.2),
                               "z_coord": (near(19114.5), 19240.0)})
    z = p.live("npcs", (3232,))
    if z is None or z["npc_name"] != "gangrenous zombie":
        p.refuse(f"npcs 3232: {None if z is None else z['npc_name']!r} on Live, not a gangrenous zombie")
    else:
        p.copy_new("npcs", (3232,), {"x_coord": 7025.2, "y_coord": 74.58, "z_coord": 20476.2}, src_db="live")
    p.copy_same("npcmove", ("shadow puma",))
    p.quests(ERU_ARC)
    for q in range(100287, 100292):
        p.quest_cols(q, ("name", "decline", "notes"), {1: ("journal",), 2: ("journal",)} if q in (100290, 100291) else None)
    for q in ERU_ARC + list(range(100287, 100292)):
        l = p.live("quests", (q,))
        if l is not None and (l["decline"] != "" or l["declined"] != ""):
            p.edit("quests", (q,), {"decline": (ANY, ""), "declined": (ANY, "")})
    p.chars(ERU_ARC + list(range(100287, 100292)),
            keep=[(334, 100248, 1), (283, 100249, 2), (641, 100260, 1), (134, 100279, 1), (50, 100287, 2)],
            ask_all=True)


def e1953(p):
    p.new_items({993520: R_ERU, 993521: R_ERU, 993522: None, 993523: None, 993550: R_ERU, 993551: None, 993552: None,
                 993553: R_ERU})
    A = (ANY, HL)
    p.items({700257: {"itemdesc": A}, 700252: {"itemdesc": A}, 700237: {"itemdesc": A},
             7349: {"maxhp": A, "raceuse": (1023, 256)}, 5755: {"maxhp": A, "itemdesc": A, "raceuse": (1023, 256)},
             5777: {"maxhp": A, "raceuse": (1023, 256)}})
    p.rename_npc(102107, "Zara", "Zara Blacksheaf")
    p.rename_npc(105797, "Merchant Greig", "Merchant Grieg")
    for q, s in ((100580, 1), (100596, 8), (100626, 8)):
        p.step_target(q, s, "Merchant Greig", "Merchant Grieg")
    for q, s in ((100594, 6), (100624, 5)):
        p.step_target(q, s, "Zara", "Zara Blacksheaf")
    qs = list(range(100231, 100241)) + list(range(100270, 100277))
    p.quests(qs)
    for q in (100267, 100268, 100269):
        p.quest_cols(q, ("name", "decline"))
    for q in qs + [100267, 100268, 100269]:
        l = p.live("quests", (q,))
        if l is not None and (l["decline"] != "" or l["declined"] != ""):
            p.edit("quests", (q,), {"decline": (ANY, ""), "declined": (ANY, "")})
    p.chars(qs + [100267, 100268, 100269], ask_all=True)


def e1963(p):
    p.new_items({993040: None, 993041: None, 993042: None, 993043: R_GNO, 993044: R_GNO, 993045: R_GNO, 993046: R_GNO})
    A = (ANY, HL)
    ed = {700359: {"levelreq": (75, 0), "itemdesc": A}, 700303: {"itemname": A, "itemdesc": A}}
    for i in (700374, 700302, 700333, 700304, 700357, 700317, 700329, 700358, 700382):
        ed[i] = {"itemdesc": A}
    p.items(ed)
    p.stock(58979, 106246, 6)
    p.rename_npc(108364, "Unit I", "Unit 1")
    for q in (100326, 100366, 100376):
        p.step_target(q, 12, "Unit I", "Unit 1")
    for n in ("coldpaw brawler", "coldpaw shaman", "coldpaw warrior"):
        p.copy_same("npcaggro", (n,))
    p.quests(range(100312, 100317))
    p.chars(range(100312, 100317), moves={
        100312: {6: (6, [993040])},
        100313: {4: (5, [5334]), 7: (8, [700374], True), 8: (9, [700374]), 9: (10, [707111])},
        100314: {7: (7, [(993041, 2)]), 9: (9, [993042]), 10: (9, [993042])},
        100316: {10: (10, [700382])},
    }, keep=[(125, 100312, 1)])


def e1974(p):
    p.new_items({993051: None, 993052: None, 993053: None, 993056: None, 993050: R_GNO, 993054: R_GNO, 993055: R_GNO})
    A = (ANY, HL)
    ed = {709202: {"itemname": A, "itemdesc": A, "raceuse": (271, 8)},
          700369: {"itemname": A, "itemcost": (ANY, 52), "itemdesc": A},
          700310: {"itemname": A, "itemdesc": A}, 700311: {"itemname": A, "itemdesc": A},
          5737: {"raceuse": (1023, 8)}, 5745: {"raceuse": (1023, 8)}}
    for i in (700373, 700387, 700342, 700343, 700299):
        ed[i] = {"itemdesc": A}
    p.items(ed)
    p.stock(58983, 106233, 13)
    p.stock(58984, 106253, 15)
    p.stock(58985, 106253, 16)
    p.stock(58986, 106253, 17)
    p.stock(58987, 106246, 7)
    p.edit("npcs", (106252,), {"npc_type": (0, 128)}, what="Engineer Macklewok")
    p.quests(range(100317, 100327))
    p.chars(range(100317, 100327), keep=[(124, 100322, 1)], ask_all=True)


def e1975(p):
    p.new_items({993104: None, 993161: R_GNO, 993100: R_GNO, 993101: R_GNO, 993102: R_GNO, 993103: R_GNO})
    p.items({5643: {"maxhp": (2000, 2600), "raceuse": (ANY_RACE, 8)}, 700384: {"itemdesc": (ANY, HL)}})
    p.quests(range(100327, 100337))
    p.chars(range(100327, 100337), moves={100330: {3: (1, [], True), 4: (2, [], True), 5: (3, [], True)}, 100335: {11: 10}},
            keep=[(120, 100327, 1), (224, 100327, 1), (333, 100327, 1), (665, 100328, 3), (813, 100328, 1),
                  (713, 100329, 2), (714, 100329, 1), (715, 100329, 1), (716, 100329, 1), (311, 100330, 1),
                  (122, 100332, 1)])


def ANY_RACE(v):
    return True


def e1981(p):
    p.new_items({993162: None, 993122: None, 993123: None, 993126: None, 993127: None, 993120: R_GNO, 993121: R_GNO,
                 993124: R_GNO, 993125: R_GNO})
    A = (ANY, HL)
    whole = {c: A for c in columns(p.src, "itempattern") if c != "patternid"}
    p.items({709200: {"itemname": A, "itemdesc": A, "raceuse": (ANY_RACE, 8)},
             700322: {"itemdesc": A}, 700323: {"itemdesc": A}, 700347: {"itemdesc": A}, 700377: {"itemdesc": A},
             700451: whole, 700452: whole})
    p.rename_npc(106265, "Bal Noonlittle", "Bal Noonittle")
    p.rename_key("npcclass", "bal noonlittle", "bal noonittle")
    p.edit("npcfaction", (124,), {"name": ("bal noonlittle", "bal noonittle")})
    p.edit("factionhits", (13,), {"npc_name": ("Bal Noonlittle", "Bal Noonittle")})
    p.edit("factionhits", (1446,), {"npc_name": ("Bal Noonlittle", "Bal Noonittle")})
    p.edit("npcs", (122113,), {"modelid": (1893243078, 2858091591)}, what="Croc")
    p.stock(58990, 106253, 18)
    p.quests(range(100337, 100347))
    p.chars(range(100337, 100347), moves={100337: {2: 1}, 100340: {2: 1, 3: 1, 4: (2, [], True), 5: (3, [], True)}},
            keep=[(396, 100338, 1), (807, 100339, 1), (626, 100341, 1), (123, 100342, 1)],
            gives=[(123, 100342, 1, 993123, 1)], takes=[(123, 9692, 700322)], ask=(100344, 100345, 100346))


def e1994(p):
    p.new_items({993070: None, 993071: R_GNO, 993072: R_GNO, 993073: R_GNO})
    A = (ANY, HL)
    ed = {8611: {"itemcost": (105, 35)}, 700312: {"itemcost": (5, 39)}, 700306: {"levelreq": (20, 0), "itemdesc": A},
          700379: {"itemname": A, "itemdesc": A}}
    for i in (700376, 700291, 700331, 700351, 700344, 700363, 700364, 700300, 700290, 700356):
        ed[i] = {"itemdesc": A}
    for i in (707100, 7261, 5748, 7874):
        ed[i] = {"raceuse": (ANY_RACE, 8)}
    p.items(ed)
    p.quests(range(100357, 100367))
    p.chars(range(100357, 100367), moves={
        100357: {2: 1}, 100360: {2: 1, 3: 1, 4: (2, [], True), 5: (3, [], True)},
        100362: {4: 5, 5: 6, 6: 7}}, keep=[(176, 100359, 1), (588, 100359, 1)], ask=(100363, 100364, 100365, 100366))


def e1995(p):
    p.new_items({993085: R_GNO, 993086: R_GNO})
    A = (ANY, HL)
    ed = {5922: {"maxhp": (800, 1000), "raceuse": (ANY_RACE, 8)}, 700332: {"levelreq": (20, 0), "itemdesc": A},
          700380: {"itemname": A, "itemdesc": A}}
    for i in (700386, 700336, 700345, 700352, 700330, 700340, 700307, 700298, 700294):
        ed[i] = {"itemdesc": A}
    for i in (7339, 5746, 5200):
        ed[i] = {"raceuse": (ANY_RACE, 8)}
    p.items(ed)
    p.quests(range(100367, 100377))
    p.chars(range(100367, 100377), moves={100367: {2: 1}, 100370: {2: 1, 3: 1, 4: (2, [], True)},
                                          100376: {1: (1, [700357])}},
            keep=[(801, 100368, 1)], ask=(100372, 100373, 100374, 100375, (100376, range(2, 20))))


def e1933(p):
    p.new_items({993000: R_GNO, 993001: R_GNO, 993002: None, 993003: None, 993004: None})
    A = (ANY, HL)
    ed = {5141: {"classuse": A},
          707123: {c: A for c in ("levelreq", "weapondamage", "maxhp", "classuse", "intelligence", "wis", "POWMAX", "PR",
                                  "itemdesc")},
          700296: {"itemcost": A, "itemdesc": A}, 700348: {"itemname": A, "itemdesc": A},
          700365: {"itemname": A, "itemdesc": A}}
    ed[707123]["raceuse"] = (ANY_RACE, 8)
    for i in (700388, 700308, 700334, 700338, 700316, 700360, 700324, 700353, 700295, 700372, 700371):
        ed[i] = {"itemdesc": A}
    p.items(ed)
    p.rename_npc(108433, "Groben Grenard", "Goben Gernhard")
    p.copy_new("npcs", (196151,))
    p.quests(range(100302, 100307))
    p.chars(range(100302, 100307), moves={
        100304: {3: (4, [700316], True), 4: (5, [700316]), 5: (7, [993002])},
        100305: {6: (6, [700353]), 7: (11, [700353, 993003])},
        100306: {2: 3, 3: (4, [], True), 4: 5, 5: (6, [], True), 6: 7}})


def e2010(p):
    n = 0
    for q in range(100302, 100377):
        if q in (100299, 100309, 100311):
            continue
        l = p.live("quests", (q,))
        if l is None or l["decline"] == "":
            continue
        p.edit("quests", (q,), {"decline": (lambda v: True, "")})
        n += 1
    p.say(f"decline \"\" on {n} quests")


SE_IDS = {
    "kobold slayer": [120736, 121093, 121095, 121102, 121103, 121107, 121112, 121113, 121114, 121117, 121120, 121121,
                      121123, 121124, 121126, 121129, 121132, 121138, 121139, 121141, 121156, 121165, 121224, 121227,
                      121228, 121234, 121237, 121239, 121240, 121244, 121245, 121355, 121357, 121359, 121360, 121362],
    "kobold augur": [121098, 121100, 121105, 121110, 121116, 121119, 121133, 121146, 121229, 121233],
    "stone spider": [120244, 121171, 121174, 121221, 121235, 121246, 121252, 121256, 121354, 121363],
    "Primal Spirit": [120247, 121099, 121101, 121106, 121111, 121118, 121134, 121147, 121230, 121232],
}
SE_TABLES = (("se common sets", 8, 1), ("se uncommon", 3, 1), ("se rare", 2, 1))
SE_SOURCE = "board 1552 se row keys"


def e257(p):
    for tn, _, _ in SE_TABLES:
        n = p.db.execute("SELECT count(*) FROM loottables WHERE tablename = ?", (tn,)).fetchone()[0]
        if n == 0:
            p.refuse(f"loottables {tn!r}: none on Live")
    total = sum(p.db.execute("SELECT count(*) FROM loottables WHERE tablename = ?", (tn,)).fetchone()[0]
                for tn, _, _ in SE_TABLES)
    p.say(f"SE trash tables on Live: {total} rows")
    for kind in SE_IDS:
        named = p.db.execute("SELECT npc_name, tablename FROM npcloot WHERE npc_name = ? COLLATE NOCASE", (kind,)).fetchall()
        if named:
            p.refuse(f"npcloot: a name binding for {kind!r} on Live: {named}")
    for kind, ids in SE_IDS.items():
        for i in ids:
            n = p.live("npcs", (i,))
            if n is None or n["npc_name"].lower() != kind.lower() or n["world"] != 3 or not 27 <= (n["zone"] or 0) <= 35:
                p.refuse(f"npcs {i}: {None if n is None else (n['npc_name'], n['world'], n['zone'])} on Live, not an SE {kind}")
                continue
            have = p.db.execute("SELECT tablename FROM npcloot WHERE npc_name = ?", (f"#{i}",)).fetchall()
            if have:
                p.refuse(f"npcloot #{i}: bound already ({[h[0] for h in have]})")
                continue
            for tn, ch, ro in SE_TABLES:
                p.insert("npcloot", (f"#{i}", tn), {"npc_name": f"#{i}", "tablename": tn, "chance": ch, "rolls": ro,
                                                     "note": "", "source": SE_SOURCE})
    n = sum(1 for o in p.ops if o[0] == "npcloot")
    p.say(f"npcloot: {n} row bindings (#<id> x {len(SE_TABLES)} tables)")


# (entry, function, the Heirloom audit its work starts at, the entries it needs)
ENTRIES = (
    ("1965", e1965, 141019, ()),
    ("2011", e2011, 144300, ("1965",)),
    ("1982", e1982, 144281, ()),
    ("1986", e1986, 140646, ()),
    ("1987", e1987, 146671, ("1986",)),
    ("1945", e1945, 141380, ()),
    ("1992", e1992, 145360, ("1945",)),
    ("1953", e1953, 143200, ()),
    ("1963", e1963, 142539, ()),
    ("1974", e1974, 145757, ("1963",)),
    ("1975", e1975, 146292, ("1974",)),
    ("1981", e1981, 146783, ("1974",)),
    ("1994", e1994, 148601, ("1963", "1974")),
    ("1995", e1995, 148903, ("1963", "1974", "1981", "1994")),
    ("1933", e1933, 140774, ()),
    ("2010", e2010, 149946, ()),
    ("257", e257, 0, ()),
)


# ---------------------------------------------------------------- writing
def audit(db, action, tbl, key, before, after, part):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    detail = (f"work board {part} (jibrish's Live request, owner-approved as submitted): {TOOL}" if part != "257" else
              f"jib board message #257 (jibrish, owner-approved): SE trash loot by row: {TOOL}")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (now(), acct[0] if acct else 0, WHO, VIA, action, tbl, dumps(dict(zip(KEYS[tbl], key))),
                      None if before is None else dumps(before), None if after is None else dumps(after), detail))
    return cur.lastrowid


def write_row(db, tbl, values, rowid=None):
    cols = list(values)
    vals = [values[c] for c in cols]
    if rowid is not None and "rowid" not in cols:
        cols, vals = ["rowid"] + cols, [rowid] + vals
    db.execute(f"INSERT INTO {tbl} ({', '.join(c if c == 'rowid' else chr(34) + c + chr(34) for c in cols)}) "
               f"VALUES ({', '.join('?' * len(cols))})", vals)


def sequence(db, tbl):
    if not table(db, "sqlite_sequence"):
        return None
    r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (tbl,)).fetchone()
    return None if r is None else r[0]


def apply(db, p):
    db.execute(LOG_DDL)
    db.execute(SEQ_DDL)
    grows = sorted({o[0] for o in p.ops if o[2] is None and o[0] in AUTO})
    seqs = {t: sequence(db, t) for t in grows}
    for tbl, key, before, after in p.ops:
        have = full(db, tbl, key)
        if (have is None) != (before is None) or (have is not None and not all(loose(have.get(c), v) for c, v in before.items())):
            raise RuntimeError(f"{tbl} {key}: not as planned")
        if after is not None and tuple(after[c] for c in KEYS[tbl]) != key:  # a name-keyed row's new key: an insert
            raise RuntimeError(f"{tbl} {key}: a key change")
        if before is None:
            write_row(db, tbl, after)
        elif after is None:
            db.execute(f"DELETE FROM {tbl} WHERE {where(tbl)}", key)
        else:
            sets = [c for c in after if not same(before.get(c), after[c])]
            db.execute(f"UPDATE {tbl} SET {', '.join(chr(34) + c + chr(34) + ' = ?' for c in sets)} WHERE {where(tbl)}",
                       [after[c] for c in sets] + list(key))
        row = full(db, tbl, key) if after is not None else None
        action = "insert" if before is None else ("delete" if after is None else "update")
        a = audit(db, action, tbl, key, plain(have), plain(row), p.part)
        db.execute(f"INSERT INTO {LOG} (part, tbl, rowkey, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?, ?)",
                   (p.part, tbl, dumps(dict(zip(KEYS[tbl], key))), None if have is None else dumps(have),
                    None if row is None else dumps(row), a, now()))
    for t, b in seqs.items():
        a = sequence(db, t)
        if a != b:
            db.execute(f"INSERT OR IGNORE INTO {SEQ} (part, tbl, before, after) VALUES (?, ?, ?, ?)", (p.part, t, b, a))
            db.execute(f"UPDATE {SEQ} SET after = ? WHERE part = ? AND tbl = ?", (a, p.part, t))
    return len(p.ops)


def logged(db):
    if not table(db, LOG):
        return set()
    return {r[0] for r in db.execute(f"SELECT DISTINCT part FROM {LOG}")}


def undo(db, only=None):
    """(rows put back, [left])."""
    if not table(db, LOG):
        return 0, []
    back, left = 0, []
    q = f"SELECT seq, part, tbl, rowkey, before, after FROM {LOG}"
    for seq, part, tbl, rowkey, before, after in db.execute(q + " ORDER BY seq DESC").fetchall():
        if only and part not in only:
            continue
        k = json.loads(rowkey)
        key = tuple(k[c] for c in KEYS[tbl])
        have = full(db, tbl, key)
        if (have is None) != (after is None) or (have is not None and dumps(have) != after):
            left.append(f"{part}: {tbl} {rowkey}: changed since, left as it is")
            continue
        b = None if before is None else json.loads(before)
        if have is not None:
            db.execute(f"DELETE FROM {tbl} WHERE {where(tbl)}", key)
        if b is not None:
            rid = b.pop("_rowid_")
            write_row(db, tbl, b, rid)
        action = "delete" if b is None else ("insert" if have is None else "update")
        audit(db, action, tbl, key, plain(have), b, part)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    still = logged(db)
    if table(db, SEQ):
        for part, t, b, a in db.execute(f"SELECT part, tbl, before, after FROM {SEQ} ORDER BY rowid DESC").fetchall():
            if (only and part not in only) or part in still:
                continue
            if sequence(db, t) == a:
                if b is None:
                    db.execute("DELETE FROM sqlite_sequence WHERE name = ?", (t,))
                else:
                    db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", (b, t))
            db.execute(f"DELETE FROM {SEQ} WHERE part = ? AND tbl = ?", (part, t))
    if not db.execute(f"SELECT 1 FROM {LOG} LIMIT 1").fetchone():
        db.execute(f"DROP TABLE {LOG}")
        if table(db, SEQ) and not db.execute(f"SELECT 1 FROM {SEQ} LIMIT 1").fetchone():
            db.execute(f"DROP TABLE {SEQ}")
    return back, left


def run(db, src, only=None, write=False, out=print):
    """plan and write each entry in order: {entry: 'applied' | 'done before' | 'refused' | 'nothing'}. write: each entry
    in a transaction of its own (Live's server waits on one entry's rows at most); else in the caller's (rolled back)."""
    done = logged(db)
    status = {}
    for part, fn, lo, needs in ENTRIES:
        if only and part not in only:
            status[part] = "skipped"
            continue
        if part in done:
            status[part] = "done before"
            out(f"== {part}: in the log already (applied before): left")
            continue
        missing = [n for n in needs if status.get(n) not in ("applied", "done before", "nothing")
                   and not (status.get(n) == "skipped" and n in done)]
        out(f"== {part}")
        if missing:
            status[part] = "refused"
            out(f"REFUSED {part}: needs entr{'y' if len(missing) == 1 else 'ies'} {', '.join(missing)} first")
            continue
        if write:
            db.execute("BEGIN IMMEDIATE")
        p = Plan(db, src, part, lo)
        try:
            fn(p)
        except Refused as e:
            p.refuse(str(e))
        except BaseException:
            if write:
                db.execute("ROLLBACK")
            raise
        for line in p.lines:
            out(line)
        if p.refusals:
            status[part] = "refused"
            out(f"REFUSED {part}, nothing of it written:\n  " + "\n  ".join(p.refusals))
            if write:
                db.execute("ROLLBACK")
            continue
        if not p.ops:
            status[part] = "nothing"
            out(f"{part}: nothing to do")
            if write:
                db.execute("ROLLBACK")
            continue
        try:
            n = apply(db, p)
        except BaseException:
            if write:
                db.execute("ROLLBACK")
            raise
        if write:
            db.execute("COMMIT")
        status[part] = "applied"
        counts = {}
        for o in p.ops:
            counts[o[0]] = counts.get(o[0], 0) + 1
        out(f"{part}: {n} rows {'written' if write else 'to write'} ({', '.join(f'{t} {c}' for t, c in sorted(counts.items()))})")
    return status


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True, help="Live's database (written)")
    ap.add_argument("--shard", required=True, choices=("live",), help="the shard --db is (Live only)")
    ap.add_argument("--source", help="Heirloom's database (read only)")
    ap.add_argument("--only", help="entries, comma separated (e.g. 1965,2011,257)")
    ap.add_argument("--hold", help="characters online (serverids, comma separated): their quests and packs are left")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    only = set(a.only.split(",")) if a.only else None
    if only and not only <= {e[0] for e in ENTRIES}:
        ap.error(f"--only: unknown entries {sorted(only - {e[0] for e in ENTRIES})}")
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("PRAGMA busy_timeout = 30000")
    T_HOLD.update(int(x) for x in (a.hold or "").split(",") if x.strip())
    if not a.apply:
        db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db, only)
            db.execute("COMMIT")
            print(f"undone: {back} rows put back" + "".join(f"\n  left: {w}" for w in left))
            return 0 if not left else 1
        if not a.source:
            ap.error("--source names Heirloom's database")
        src = sqlite3.connect(f"file:{a.source}?mode=ro", uri=True)
        for t in set(KEYS) | {"gm_audit"}:
            for side, d in (("Live", db), ("Heirloom", src)):
                if not table(d, t):
                    print(f"REFUSED: {side} has no {t} table")
                    return 1
            if t not in ("gm_audit", "charquests", "charinventory") and columns(db, t) != columns(src, t):
                print(f"REFUSED: {t}: Live's columns differ from Heirloom's")
                return 1
        try:
            status = run(db, src, only, a.apply)
        finally:
            src.close()
        print("\nentries: " + ", ".join(f"{k} {v if a.apply or v != 'applied' else 'planned'}" for k, v in status.items()
                                       if v != "skipped"))
        if not a.apply:
            print("(report only: nothing written; --apply to write)")
            return 0
        print(f"applied, each row in gm_audit and {LOG}; reload the quests (quest_set_enabled off and on) and the NPCs "
              "(reload_npcs) on the server")
        return 0 if "refused" not in status.values() else 2
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
