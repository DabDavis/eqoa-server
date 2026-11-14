#!/usr/bin/env python3
"""The Dracoliche on the Isle of Dread (2026-11-14; the owner: "dracoliche is not in our database but we have the quest
steps"; the wiki, https://wiki.eqoa.live/index.php?title=Dracoliche, saved in ~/.cache/iodaudit/wiki/: Warrior,
Dragons, specials "IoD Lifetap Proc, Rotting Disease(ADD), Rampage, Lifetap, Critical Hit, Invulnerable", "Guarding
The Faceless/CazicThule Chamber", level 70, 578,380 HP, "+2 Of" seven Primal Isle weapons, respawn "3 Days +/- 16
Hours", A, S(I), faction Denizens of Dread, opposing Citizens of Graumord). The quest steps: no quest or step of ours
names him; the wiki's Cazic Thule Trials page does ("If Dracoliche is active, he blocks the path to the third
Kithgrin. Dracoliche must be defeated before the wall is removed"), between Qual'Tin and the trial of contention.

What it writes (every row in gm_audit and its log, dracoliche_1114_log; --undo exact):

  npcs: "Dracoliche", a copy of the Estate's dead rot (118873: zone 1132, race 6, as the island's rows) at
    (25550.0, 19.875, 3098.0) world 0, facing 64 (east, at the hall he guards): the alcove room of the west hall
    between the Estate's halls and the main chamber with the great lava pool, west of the chamber's quest wall
    (ct_trials' "wall chamber", 163623, open past step 7 of 101312) and east of the chamber: "guarding the Faceless/
    CazicThule chamber". The floor there 19.875 off the ground file (the room x 25540-25560, z 3090-3106, its roof
    ~31). Level 70; body 0xED55F32A (CHAR.ESF 473: the brown winged bone dragon, Dagladar's body, by sight against the
    wiki's picture), size 1.5 (ours: the plague stalkers' biggest of the same skeleton).
  npcs + npcspawnonly: "Rotting Disease", a copy of the same dead rot (level 55, its body 0x3153A50A), held out of the
    world: the add his summons call ("Rotting Disease(ADD)"; the island's rot's body: ours). npcclass 0; npctemplate
    "iod native" (the island's natives', board 430) where that template is there.
  npcclass "dracoliche" 0 (the wiki's Warrior). classtemplates "dracoliche" + npctemplate: hit points for 578,380 at 70
    (hp_pct of the level's balance), melee 749 (Megaladon's, board 430: ~1,000 a hit on a capped L60 warrior at 65;
    ours), a 3,000 Critical Hit every 25-35 s from 15 s (ours, between CazicThule's 2,500 and Megaladon's 4,000), the
    isle's AR 50 (board 430's rule for every IoD mob; the wiki's "strongagainst None" as on Megaladon's page);
    abilities: "IoD Lifetap Proc" a veil (each landed melee hit taps 0.05% of his hit points, ~289, AR, into him: the
    server's only melee tap; every hit, not a chance), Lifetap Life's Bane (700722, 550 AR drain) at 300% every 15 s
    from 5 s (Zin's clock, board 430), Rampage an enrage below 100% (from his first wound) with rampage 50 (Mr.
    Moogie's 50%), Rotting Disease two summons of 2 adds each, below 75% and below 40%. "Invulnerable": not built (no
    mechanic of ours says what the wiki means: see the report).
  npcaggro "dracoliche": aggressive, sees invisible (A, S(I)). npcfaction "dracoliche" anywhere: Denizens of Dread.
  factionhits "Dracoliche": Citizens of Graumord +5 (the IoD named's, quest-seed), Denizens of Dread -5 (ours, mirror).
  npcrespawn: 3 days (259,200 s). The wiki's "+/- 16 hours" is not built: npcrespawn has no random window.
  itempattern: the seven Primal Isle weapons (no stats of theirs are saved: L60 raid weapons' records, below), level 60;
    loottables "boss dracoliche" (one each, weight 1, levels 1-70: his level is 70); npcloot "dracoliche" 100%, 2 rolls
    ("+2 Of").

Rows are found by key and checked; anything not as the 2026-11-13 data had it refuses the whole run. Report only by
default; --apply in one transaction (a second --apply finds its log and does nothing); --undo exact, refused while a
player, bank, merchant or NPC holds one of the weapons (a row changed since is left, said).

    tools/dracoliche_20261114.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import math
import sqlite3
import sys

LOG = "dracoliche_1114_log"
WHO = "dracoliche"
VIA = "tool"
SRC = "dracoliche1114"
DETAIL = "the Dracoliche on the Isle of Dread (the owner, 2026-11-14; wiki.eqoa.live): tools/dracoliche_20261114.py"

NAME, KEY = "Dracoliche", "dracoliche"
ADD_NAME, ADD_KEY = "Rotting Disease", "rotting disease"
SOURCE_ROW = 118873  # the Estate's dead rot (zone 1132, level 55, body 0x3153A50A)
SOURCE_NAME = "dead rot"
AT = (25550.0, 19.875, 3098.0)
FACING = 64
BODY = 0xED55F32A
SIZE = 1.5
LEVEL = 70
HP = 578_380
MELEE = 749
CRIT = 3000
RESPAWN = 3 * 86400
TABLE = "boss dracoliche"
ADD_TEMPLATE = "iod native"
FACTION, OPPOSING = "Denizens of Dread", "Citizens of Graumord"
ABILITIES = [
    {"kind": "veil", "name": "IoD Lifetap Proc", "tap_pct": 0.05, "resist": "AR"},
    {"kind": "lifetap", "spell_id": 700722, "damage_pct": 300, "every": 15, "first": 5},
    {"kind": "enrage", "below": 100, "rampage": 50},
    {"kind": "summon", "npc": ADD_NAME, "count": 2, "below": 75},
    {"kind": "summon", "npc": ADD_NAME, "count": 2, "below": 40},
]

# The seven weapons. No Primal Isle weapon's stats are saved (the wiki's item pages were not fetched), so each is a
# L60 raid weapon of its kind: its look (model, icon, slot, attack type, classes) from an item of the same kind
# named in brackets, its figures from the hand's top L60 record: one-handers Sword of the Contender (5379), two-handers
# Sword of the Challenger (5373), the bow Fistaglin's Bow (4704) at the longbow of toxicity's damage (5447).
ONE_HAND = dict(itemcost=4792, weapondamage=801, maxhp=10578, duration=75, str=35, sta=35, HPMAX=180, HoT=10, AC=100)
TWO_HAND = dict(itemcost=4792, weapondamage=928, maxhp=10578, duration=65, sta=45, HPMAX=200, HoT=20, AC=100)
BOW = dict(itemcost=4792, weapondamage=824, maxhp=12600, duration=65, agi=26, dex=38)
WEAPONS = [  # name, look from (id, its name), figures, slot, attack type, range, classuse, model, icon
    ("Primal Isle Battlehammer", (4158, "Bloodgale Battlehammer"), TWO_HAND, 15, 4, 0, 1023, 488528354, -133430711),
    ("Primal Isle Warsword", (713024, "Elfbone Warsword"), ONE_HAND, 12, 1, 0, 111, 1205436896, 445518418),
    ("Primal Isle Spear", (5396, "Blood Drenched Spear"), ONE_HAND, 14, 5, 0, 112, -1072778059, 1872350256),
    ("Primal Isle Brawl Stick", (1025, "Superior Corrosive Stick"), ONE_HAND, 14, 3, 0, 111, -1817771540, -1540768975),
    ("Primal Isle Claymore", (1305, "Rotblade Claymore"), TWO_HAND, 15, 2, 0, 15, -1903315937, -955781738),
    ("Primal Isle War Lance", (713078, "Hulthu's Lance"), TWO_HAND, 15, 6, 0, 14, 1127123123, -907954022),
    ("Primal Isle Reinforced Bow", (4704, "Fistaglin's Bow"), BOW, 16, 7, 25, 111, -276002250, 1282767813),
]
STATS = ("str", "sta", "agi", "wis", "dex", "cha", "intelligence", "HPMAX", "POWMAX", "PoT", "HoT", "AC", "PR", "DR", "FR",
         "CR", "LR", "AR")

KEYS = {"npcs": ("id",), "npcspawnonly": ("name",), "npcclass": ("name",), "classtemplates": ("name",),
        "npctemplate": ("name",), "npcaggro": ("npc_name",), "npcfaction": ("id",), "factionhits": ("id",),
        "npcrespawn": ("npc_row",), "npcloot": ("npc_name", "tablename"), "loottables": ("id",),
        "itempattern": ("patternid",)}
SEQS = ("npcs", "loottables", "itempattern", "npcfaction", "factionhits")
AUTO = {"npcs": "id", "loottables": "id", "itempattern": "patternid", "npcfaction": "id", "factionhits": "id"}


def rnd(x):
    """Go's math.Round: halves away from zero."""
    return int(math.floor(x + 0.5)) if x >= 0 else -int(math.floor(-x + 0.5))


def table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def row_of(db, tbl, key):
    where = " AND ".join(f"{c} = ?" for c in KEYS[tbl])
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", tuple(key[c] for c in KEYS[tbl]))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def key_of(tbl, row):
    return {c: row[c] for c in KEYS[tbl]}


def go_json(m):
    """A row as Go's json.Marshal writes a map (keys sorted, <>& escaped), as the panel's audit has it."""
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def between(rows, level):
    """balance.go's Level: the row of the level, the last past the end, the first before it, else the line between."""
    rows = sorted(rows)
    if not rows:
        return 100.0
    for i, (lv, v) in enumerate(rows):
        if lv >= level:
            if lv == level or i == 0:
                return v
            plv, pv = rows[i - 1]
            return pv + (v - pv) * (level - plv) / (lv - plv)
    return rows[-1][1]


def level_hp(db, level):
    """An NPC's hit points at its level's balance before its template (level x 60 at balancelevel's hp_pct)."""
    base = max(level * 60, 10)
    bal = between(list(db.execute("SELECT level, hp_pct FROM balancelevel")) if table(db, "balancelevel") else [], level)
    return max(rnd(base * bal / 100), 1) if bal != 100 else base


def max_hp(db, level, hp_pct):
    return max(1, rnd(level_hp(db, level) * hp_pct / 100))


def holders(db):
    """The tables (besides the item records, loot tables and logs) whose rows name an item by patternid."""
    out = []
    for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table'").fetchall():
        if t in ("itempattern", "loottables") or t.endswith("_log"):
            continue
        cols = [r[1] for r in db.execute(f"PRAGMA table_info('{t}')")]
        if "patternid" in cols:
            out.append(t)
    return out


def plan(db):
    """(changes, lines, refusals). A change is (tbl, before row or None, after row, token or None); a value ("$", tok)
    in a row is the id the row of that token took."""
    changes, lines, refusals = [], [], []
    for n in (NAME, ADD_NAME):
        if db.execute("SELECT count(*) FROM npcs WHERE lower(trim(npc_name)) = ?", (n.lower(),)).fetchone()[0]:
            refusals.append(f"an NPC called {n!r} is there already")
    src = row_of(db, "npcs", {"id": SOURCE_ROW})
    if src is None or src["npc_name"] != SOURCE_NAME or src["world"] != 0 or src["modelid"] != 0x3153A50A or \
            math.hypot(src["x_coord"] - 25691.5, src["z_coord"] - 3322.0) > 2:
        refusals.append(f"npcs {SOURCE_ROW} is not the Estate's dead rot: {src}")
    if not db.execute("SELECT count(*) FROM factions WHERE name = ?", (FACTION,)).fetchone()[0] or \
            not db.execute("SELECT count(*) FROM factions WHERE name = ?", (OPPOSING,)).fetchone()[0]:
        refusals.append(f"no faction {FACTION!r} or {OPPOSING!r}")
    if db.execute("SELECT count(*) FROM spellpattern WHERE spellid = 700722 AND spellname = ?", ("Life's Bane",)).fetchone()[0] != 1:
        refusals.append("spell 700722 is not Life's Bane")
    for tbl, k in (("npcclass", {"name": KEY}), ("npcclass", {"name": ADD_KEY}), ("classtemplates", {"name": KEY}),
                   ("npctemplate", {"name": KEY}), ("npctemplate", {"name": ADD_KEY}), ("npcaggro", {"npc_name": KEY}),
                   ("npcspawnonly", {"name": ADD_NAME}), ("npcloot", {"npc_name": KEY, "tablename": TABLE})):
        if row_of(db, tbl, k) is not None:
            refusals.append(f"{tbl} {k} is there already")
    if db.execute("SELECT count(*) FROM npcspawnonly WHERE lower(name) = ?", (ADD_KEY,)).fetchone()[0]:
        refusals.append(f"npcspawnonly {ADD_KEY!r} is there already")
    if db.execute("SELECT count(*) FROM npcfaction WHERE lower(name) = ?", (KEY,)).fetchone()[0]:
        refusals.append(f"npcfaction {KEY!r} has rows already")
    if db.execute("SELECT count(*) FROM factionhits WHERE lower(npc_name) = ?", (KEY,)).fetchone()[0]:
        refusals.append(f"factionhits {KEY!r} has rows already")
    if db.execute("SELECT count(*) FROM loottables WHERE tablename = ?", (TABLE,)).fetchone()[0]:
        refusals.append(f"loottables {TABLE!r} has rows already")
    for w in WEAPONS:
        if db.execute("SELECT count(*) FROM itempattern WHERE lower(itemname) = ?", (w[0].lower(),)).fetchone()[0]:
            refusals.append(f"an item called {w[0]!r} is there already")
    if refusals:
        return changes, lines, refusals

    # the two NPCs
    npc = {c: v for c, v in src.items() if c != "id"}
    npc.update(npc_name=NAME, x_coord=AT[0], y_coord=AT[1], z_coord=AT[2], facing=FACING, modelid=BODY, size=SIZE,
               npc_level=LEVEL, npc_id=0)
    changes.append(("npcs", None, npc, "D"))
    lines.append(f"npcs: {NAME!r} at ({AT[0]}, {AT[1]}, {AT[2]}) world 0 facing {FACING}, body 0x{BODY:08X} size {SIZE}, "
                 f"level {LEVEL} (a copy of the dead rot {SOURCE_ROW}: zone {src['zone']}, race {src['race']})")
    add = {c: v for c, v in src.items() if c != "id"}
    add.update(npc_name=ADD_NAME, x_coord=AT[0], y_coord=AT[1], z_coord=AT[2], facing=FACING, npc_id=0)
    changes.append(("npcs", None, add, "A"))
    changes.append(("npcspawnonly", None, {"name": ADD_NAME, "note": "the Dracoliche's add (the wiki's 'Rotting Disease(ADD)'): "
                                           "copied by his template's summons only", "source": SRC}, None))
    lines.append(f"npcs: {ADD_NAME!r} (the dead rot's level {src['npc_level']} and body), held out of the world "
                 f"(npcspawnonly), the add his summons copy")

    # class, template, aggro, faction, respawn
    changes.append(("npcclass", None, {"name": KEY, "class": 0, "source": SRC}, None))
    changes.append(("npcclass", None, {"name": ADD_KEY, "class": 0, "source": SRC}, None))
    hp_pct = rnd(HP * 100 / level_hp(db, LEVEL))
    tmpl = {"name": KEY, "class": 0, "hp_pct": hp_pct, "taken_pct": 100, "melee_pct": MELEE, "special": "Critical Hit",
            "special_chance": 0, "special_damage": CRIT / LEVEL, "special_fx": 0, "special_every": 30.0, "special_first": 15.0,
            "special_jitter": 5.0, "special_from": 10, "resist_kind": "AR", "resist_pct": 50,
            "abilities": json.dumps(ABILITIES, separators=(",", ":"), ensure_ascii=False),
            "note": (f"The Dracoliche, L{LEVEL}, guarding the Cazic-Thule chamber (the wiki, the owner 2026-11-14): "
                     f"{max_hp(db, LEVEL, hp_pct):,} hit points (the wiki's 578,380), melee {MELEE} (Megaladon's, ours), a "
                     f"{CRIT:,} Critical Hit every 25-35 s (ours), the isle's AR 50 (board 430's rule). Specials: IoD "
                     f"Lifetap Proc a veil of 0.05% a landed hit, AR; Lifetap Life's Bane at 300% every 15 s (Zin's clock); "
                     f"Rampage 50% on his second from his first wound (an enrage below 100); Rotting Disease 2 adds below "
                     f"75% and 2 below 40%. Invulnerable not built. tools/dracoliche_20261114.py"),
            "source": SRC}
    changes.append(("classtemplates", None, tmpl, None))
    changes.append(("npctemplate", None, {"name": KEY, "template": KEY, "note": "the Dracoliche, a raid boss, level 70",
                                          "source": SRC}, None))
    if row_of(db, "classtemplates", {"name": ADD_TEMPLATE}) is not None:
        changes.append(("npctemplate", None, {"name": ADD_KEY, "template": ADD_TEMPLATE,
                                              "note": "the Dracoliche's add: the island's natives' profile", "source": SRC}, None))
        add_t = f"npctemplate {ADD_KEY!r} -> {ADD_TEMPLATE!r}"
    else:
        add_t = f"no template for {ADD_KEY!r} (no {ADD_TEMPLATE!r} here: its level's balance)"
    lines.append(f"classtemplates {KEY!r}: hp_pct {hp_pct} ({max_hp(db, LEVEL, hp_pct):,} hit points at {LEVEL}), melee "
                 f"{MELEE}, Critical Hit {CRIT:,} every 25-35 s, AR 50; abilities {tmpl['abilities']}; {add_t}")
    changes.append(("npcaggro", None, {"npc_name": KEY, "aggressive": 1, "radius": 0.0, "leash": 0.0, "holding": 0,
                                       "see_invis": 1, "note": "the wiki: A, S(I)", "source": SRC}, None))
    changes.append(("npcfaction", None, {"name": KEY, "world": -1, "x": 0.0, "z": 0.0, "radius": 0.0, "faction": FACTION,
                                         "note": "the wiki: Denizens of Dread", "source": SRC}, None))
    changes.append(("factionhits", None, {"npc_name": NAME, "faction": OPPOSING, "delta": 5, "source": SRC}, None))
    changes.append(("factionhits", None, {"npc_name": NAME, "faction": FACTION, "delta": -5, "source": SRC}, None))
    lines.append(f"npcaggro {KEY!r}: aggressive, sees invisible; npcfaction: {FACTION}; factionhits: {OPPOSING} +5, "
                 f"{FACTION} -5")
    changes.append(("npcrespawn", None, {"npc_row": ("$", "D"), "seconds": float(RESPAWN),
                                         "note": "the Dracoliche: the wiki's 3 days (its +/- 16 hours: no window in npcrespawn)",
                                         "source": SRC}, None))
    lines.append(f"npcrespawn: {RESPAWN} s (3 days; the wiki's +/- 16 h not built)")

    # the weapons and their table
    for name, (look_id, look_name), fig, slot, at, rng, cu, model, icon in WEAPONS:
        it = {"itemcost": fig["itemcost"], "unk1": 0, "itemicon": icon, "unk2": 0, "equipslot": slot, "unk3": 0, "trade": 0,
              "rent": 1, "unk4": 0, "attacktype": at, "weapondamage": fig["weapondamage"], "unk5": rng, "levelreq": 60,
              "maxstack": 1, "maxhp": fig["maxhp"], "duration": fig["duration"], "classuse": cu, "raceuse": 1023, "procanim": 0,
              "lore": 0, "unk6": 0, "craft": 0, "itemname": name,
              "itemdesc": f"A weapon of the Primal Isle, carried by the Dracoliche on the Isle of Dread." +
                          (f"\nRange: {rng}" if rng else ""),
              "model": model, "color": 4294967295, "weaponproc": 0, "fish": 0}
        for s in STATS:
            it[s] = fig.get(s, 0)
        changes.append(("itempattern", None, it, "I:" + name))
        changes.append(("loottables", None, {"tablename": TABLE, "patternid": ("$", "I:" + name), "weight": 1, "minlevel": 1,
                                             "maxlevel": 70, "source": SRC}, None))
        lines.append(f"item {name!r}: L60, damage {fig['weapondamage']}, slot {slot}, attack type {at}"
                     + (f", range {rng}" if rng else "") + f", classes {cu}; look of {look_name} ({look_id})")
    changes.append(("npcloot", None, {"npc_name": KEY, "tablename": TABLE, "chance": 100, "rolls": 2,
                                      "note": "the wiki: '+2 Of' the seven Primal Isle weapons", "source": SRC}, None))
    lines.append(f"npcloot {KEY!r} -> {TABLE!r}: 100%, 2 rolls of the 7")
    return changes, lines, refusals


LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,
    rowkey  TEXT NOT NULL,      -- the row's key (JSON)
    before  TEXT,               -- the row before (JSON), NULL: made here
    after   TEXT,               -- the row as written (JSON)
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def audit(db, action, tbl, key, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key),
                      None if before is None else go_json(before), None if after is None else go_json(after), detail))
    return cur.lastrowid


def seqs(db):
    return {t: (lambda r: None if r is None else r[0])(db.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (t,)).fetchone())
            for t in SEQS}


def apply(db, changes):
    """The plan written, in the caller's transaction: each row and its gm_audit line, its own line in the log. Returns
    (rows, the token ids)."""
    db.execute(LOG_SCHEMA)
    before_seq = seqs(db)
    ids = {}
    n = 0
    for tbl, before, after, tok in changes:
        after = {c: (ids[v[1]] if isinstance(v, tuple) and v[0] == "$" else v) for c, v in after.items()}
        cols = list(after)
        cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [after[c] for c in cols])
        if tbl in AUTO:
            key = {AUTO[tbl]: cur.lastrowid}
            if tbl == "npcs":
                db.execute("UPDATE npcs SET npc_id = ? WHERE id = ?", (cur.lastrowid, cur.lastrowid))
        else:
            key = key_of(tbl, after)
        if tok:
            ids[tok] = cur.lastrowid
        row = row_of(db, tbl, key)
        a = audit(db, "insert", tbl, key, None, row, DETAIL)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, NULL, ?, ?)",
                   (tbl, json.dumps(key), json.dumps(row, sort_keys=True, default=str), a))
        n += 1
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES ('sqlite_sequence', '{{}}', ?, ?)",
               (json.dumps(before_seq), json.dumps(seqs(db))))
    return n, ids


def undo(db):
    """Each logged row still as written taken out, the last first, its gm_audit line; then the ids the new rows took,
    when no other row took one since. Refused while anything holds a weapon it made. Returns (back, left)."""
    if not table(db, LOG):
        return 0, 0
    made = [json.loads(k)["patternid"] for (k,) in db.execute(f"SELECT rowkey FROM {LOG} WHERE tbl = 'itempattern'")]
    if made:
        marks = ",".join("?" * len(made))
        held = []
        for t in holders(db):
            c = db.execute(f"SELECT count(*) FROM {t} WHERE patternid IN ({marks})", made).fetchone()[0]
            if c:
                held.append(f"{t} {c}")
        if held:
            raise SystemExit("refused, nothing undone: the weapons it made are held (" + ", ".join(held) + ")")
    back = left = 0
    seq_row = db.execute(f"SELECT before, after FROM {LOG} WHERE tbl = 'sqlite_sequence'").fetchone()
    for seq, tbl, key, after in db.execute(
            f"SELECT seq, tbl, rowkey, after FROM {LOG} WHERE tbl != 'sqlite_sequence' ORDER BY seq DESC").fetchall():
        key = json.loads(key)
        now = row_of(db, tbl, key)
        if now is None or json.dumps(now, sort_keys=True, default=str) != after:
            left += 1
            print(f"  left: {tbl} {key}, changed since it was written")
            continue
        db.execute(f"DELETE FROM {tbl} WHERE " + " AND ".join(f"{c} = ?" for c in KEYS[tbl]), [key[c] for c in KEYS[tbl]])
        audit(db, "delete", tbl, key, now, None, "undo: " + DETAIL)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if left == 0:
        if seq_row is not None:
            was, written = json.loads(seq_row[0]), json.loads(seq_row[1])
            cur = seqs(db)
            for t in SEQS:
                if was.get(t) is not None and cur.get(t) == written.get(t):
                    db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", (was[t], t))
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
    try:
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
        if table(db, LOG):
            print(f"applied already ({LOG} is there): nothing to do (--undo first to write it again)")
            return 0
        changes, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not a.apply:
            print(f"report only: {len(changes)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            n, ids = apply(db, changes)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {n} rows, each in gm_audit and {LOG}; the Dracoliche's row {ids['D']}, the add's {ids['A']}, "
              f"weapons {', '.join(str(ids['I:' + w[0]]) for w in WEAPONS)}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
