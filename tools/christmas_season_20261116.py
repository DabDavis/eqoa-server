#!/usr/bin/env python3
"""Freeport's Christmas season (the owner, 2026-11-16: "lets make a christmas season trigger. where dust is standing, a big
fir tree like a christmas tree. needs lights on trees in different colors. the lights can be a template so we can put christmas
lights on fences buildings etc. only tree for now"; then "for the giant fir tree make the stock cut off so the firs touch the
ground and takes up a big area like a town tree").

  - "Christmas Tree": an NPC in Freeport's north plaza (by where Dust stood), friendly (npc_type 128), standing, never
    attacking, a town tree's size, its art sunk so its lowest boughs rest on the plaza and no bare stem shows. Its npcs row's body is
    Oakhollow's (CHAR.ESF 0x46392747, a treant: what a PS2 can draw, every character file being resident); a client that
    draws decorations (vanilla) is sent the fir (decorart: TUNARIA.ESF's 0x0E318233, a LOD sprite of zone 32 that the client
    reads off its own disc). Not selectable by anyone. It stands only while the Christmas season is on (npcseason).
  - The lights, as templates (christmas_lights_20261116.json -> lighttemplates, lightstrings): multi-coloured twinkling bulbs
    wound round the tree on a thin dark green cord, a gold star on top, lit while the season is on. Vanilla draws them; a PS2 draws none.
  - GM trigger "Christmas Season" (the next free slot: #trigger <slot> or by name, the panel, vanilla's TRIGGER menu): its one
    step turns the season over (season toggle): fired once the tree and its lights come, fired again they go. The season is
    kept in gmseasons, so it outlasts restarts. A broadcast to the world each way.
  - The Christmas currency (the owner: "the weapon enchanter will cost christmas currency for our christmas event. the tree
    is the beginning of that event"): "Holly Token" (itempattern 716900, a copy of the Defaced Coin's record: stackable to
    999, tradeable, the Leaf Encrusted Gem's green icon), earned only while the season is on (seasonrewards): 4% of the kills
    of a creature not green to the killer, 25% of a named one's (a drop on the killer's view of the corpse); 3 a day from
    the Holiday Elf by the tree (a copy of Captain Scilond, an elf, friendly, of the season); 1 an hour in the world; two
    daily holiday quests from the Holiday Elf (quests 102901, 102902, repeatable 2: once a day), 3 each. An evening's play
    (some 3 hours) earns about 18 (CALIBRATION below): the price of one weapon aura at the Weapon Enchanter.
  - npcdialog (source christmas-2026): Freeport's people on the tree, while it stands... said all season (lines are read every
    30 s; --undo takes them).
The season starts OFF: nothing shows until a GM fires the trigger. Every table is read at the server's start (a keep restart's
before.sh) and by the panel's reload; a binary with the season code is needed (main + christmas). gm_audit rows.

    christmas_season_20261116.py --db eqoa.db [--apply | --undo] --applied FILE [--lights christmas_lights_20261116.json]
"""
import argparse, json, os, sqlite3, sys, time

SRC = "christmas-2026"
SEASON = "Christmas"
TREE = "Christmas Tree"
TRIGGER = "Christmas Season"
TEMPLATE_ROW = (113208, "Oakhollow")   # a treant's body: the PS2's tree
PS2_BODY = 0x46392747                   # Oakhollow (CHAR.ESF)
FIR = 0x0E318233                        # TUNARIA.ESF: a full fir, a LOD sprite
FIR_WORLD, FIR_ZONE = 0, 32             # held by zone 32 (and 6, 16, 30, 31, 43-45, 57, 58)
# where Dust stood: (25303.8, 54.1, 15774.6), the plaza's sand by the shore, south of the Announcements obelisk; its boughs
# (6.5 round at the sand) clear of every NPC round it (the nearest, Rehabilitator Sela, 12 north)
SIZE = 1.0                              # the fir at 1: some 35 high over the plaza, its boughs some 13 across at the sand
SINK = 13.0                             # the fir's bare stem and its first boughs' droop, in its own units: the art drawn that
                                        # much lower (decorart.sink; the NPC itself stands on the floor, as a creature does)
FLOOR = 54.094
TREE_AT = (25303.8, FLOOR, 15774.6, 0)
ELF, ELF_TEMPLATE = "Holiday Elf", (405, "Captain Scilond")
ELF_AT = (25296.0, FLOOR, 15764.0, 64)  # north-west of the tree, past its boughs (13 from its middle), 9.5 from Sela
TOKEN, TOKEN_NAME, TOKEN_COPY, TOKEN_ICON = 716900, "Holly Token", 700545, 140265582
TOKEN_DESC = ("A token of holly, its berries bright as the lights on Freeport's tree. Earned during the Christmas season, "
              "and spent at the Weapon Enchanter.")
# the earn rates; an evening (3 h, some 40 kills an hour of which some 3 named, both quests, the gift) is about
#   kills 117 x 4% = 4.7, named 3 x 25% = 0.75, gift 3, quests 2 x 3 = 6, playtime 3 x 1 = 3: 17.5, so X = 18
DROP_PCT, NAMED_PCT, GIFT, HOURLY, QUEST_TOKENS = 4.0, 25.0, 3, 1, 3
CALIBRATION = 18
# the holiday quests as work entry 1597 rewrote them (christmas_elf_quests_20261117.json: the lost fourteen years; 102901
# once only, 102902 daily, three daily gift runs 102904-102906 after 102901, one a day by day group; questions at the
# offer of 102901 and 102902)
ELF_QUESTS = json.load(open(os.path.join(os.path.dirname(os.path.abspath(__file__)), "christmas_elf_quests_20261117.json")))
QUEST_IDS = [q["questid"] for q in ELF_QUESTS["quests"]]
LINES = [  # (town, role, name, line)
    ("Freeport", "", "", "Have you seen the tree in the north plaza, %p? Lit up like the Tribunal's own lanterns."),
    ("Freeport", "", "", "A fir all the way from the northern woods, standing in Freeport. Whoever hauled it here earned their ale."),
    ("Freeport", "", "", "The lights on the plaza tree twinkle all night. My children will not go to bed."),
    ("Freeport", "", "", "Winter comes even to Freeport, %p. Come warm yourself by the tree's lights."),
    ("Freeport", "guard", "", "No climbing the plaza tree, %p. That goes for halflings too."),
    ("Freeport", "guard", "", "The Militia stands watch over the plaza tree. Orders from the top."),
    ("", "", ELF, "Merry Christmas, %p! Fourteen years went missing, but the gifts did not. Speak to me each day of the season."),
    ("", "", ELF, "Holly Tokens buy wonders at the Weapon Enchanter. Earn them while the season lasts!"),
] + [(d["town"], d["role"], d["name"], d["line"]) for d in ELF_QUESTS["dialog_insert"]]

ap = argparse.ArgumentParser()
ap.add_argument("--db", required=True)
ap.add_argument("--applied", required=True)
ap.add_argument("--lights", default=os.path.join(os.path.dirname(os.path.abspath(__file__)), "christmas_lights_20261116.json"))
g = ap.add_mutually_exclusive_group()
g.add_argument("--apply", action="store_true")
g.add_argument("--undo", action="store_true")
a = ap.parse_args()
db = sqlite3.connect(a.db, timeout=30)
now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def row(tbl, where, args):
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", args)
    cols = [d[0] for d in cur.description]
    r = cur.fetchone()
    return None if r is None else dict(zip(cols, r))


def audit(action, tbl, key, before, after):
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) VALUES (?, 1, 'dab', 'tool', '', ?, ?, ?, ?, ?, ?)",
               (now, action, tbl, json.dumps(key), None if before is None else json.dumps(before, sort_keys=True), None if after is None else json.dumps(after, sort_keys=True),
                "the owner: Freeport's Christmas season; tools/christmas_season_20261116.py"))


def need_tables():
    have = {r[0] for r in db.execute("SELECT name FROM sqlite_master WHERE type='table'")}
    missing = [t for t in ("gmseasons", "npcseason", "lighttemplates", "lightstrings", "decorart", "seasonrewards") if t not in have]
    if missing:
        sys.exit(f"tables {missing} are not here: start the christmas binary on this database once first (it makes them). Nothing done")


if a.undo:
    j = json.load(open(a.applied))
    for nid in j["npcs"]:
        r = row("npcs", "id = ?", (nid,))
        if r is not None and r["npc_name"] in (TREE, ELF):
            db.execute("DELETE FROM npcs WHERE id = ?", (nid,))
            audit("delete", "npcs", {"id": nid}, r, None)
    for sid in j["strings"]:
        r = row("lightstrings", "id = ?", (sid,))
        if r is not None:
            db.execute("DELETE FROM lightstrings WHERE id = ?", (sid,))
            audit("delete", "lightstrings", {"id": sid}, r, None)
    for qid in QUEST_IDS:
        for tbl in ("queststeps", "quests"):
            n = db.execute(f"DELETE FROM {tbl} WHERE questid = ?", (qid,)).rowcount
            audit("delete", tbl, {"questid": qid}, {"rows": n}, None)
    r = row("itempattern", "patternid = ?", (TOKEN,))
    if r is not None and r["itemname"] == TOKEN_NAME:
        db.execute("DELETE FROM itempattern WHERE patternid = ?", (TOKEN,))
        audit("delete", "itempattern", {"patternid": TOKEN}, r, None)
    for tbl, where, args in [("seasonrewards", "source = ?", (SRC,)), ("seasongifts", "season = ?", (SEASON.lower(),)),
                             ("seasonplay", "season = ?", (SEASON.lower(),)), ("lighttemplates", "source = ?", (SRC,)), ("npcseason", "source = ?", (SRC,)), ("decorart", "source = ?", (SRC,)),
                             ("npcmove", "source = ?", (SRC,)), ("npcaggro", "source = ?", (SRC,)), ("npcdialog", "source = ?", (SRC,)),
                             ("gmseasons", "name = ? COLLATE NOCASE", (SEASON,)), ("gmtriggers", "slot = ? AND name = ?", (j["slot"], TRIGGER))]:
        n = db.execute(f"DELETE FROM {tbl} WHERE {where}", args).rowcount
        audit("delete", tbl, {"where": where, "args": list(args)}, {"rows": n}, None)
    db.commit(); os.rename(a.applied, a.applied + ".undone"); print("undone"); sys.exit()

need_tables()
if a.apply and os.path.exists(a.applied):
    sys.exit("applied already")
lights = json.load(open(a.lights))
for q, args, what in [("SELECT count(*) FROM npcs WHERE npc_name IN (?, ?)", (TREE, ELF), "a Christmas Tree or Holiday Elf NPC"),
                      ("SELECT count(*) FROM itempattern WHERE patternid = ?", (TOKEN,), f"item {TOKEN}"),
                      ("SELECT count(*) FROM seasonrewards WHERE season = ? COLLATE NOCASE", (SEASON,), "the season's rewards"),
                      (f"SELECT count(*) FROM quests WHERE questid IN ({','.join(str(q) for q in QUEST_IDS)})", (), "a holiday quest's id"),
                      ("SELECT count(*) FROM gmtriggers WHERE name = ?", (TRIGGER,), "the Christmas Season trigger"),
                      ("SELECT count(*) FROM gmseasons WHERE name = ? COLLATE NOCASE", (SEASON,), "the Christmas season"),
                      ("SELECT count(*) FROM npcseason WHERE source = ?", (SRC,), "a Christmas NPC season"),
                      ("SELECT count(*) FROM decorart WHERE source = ?", (SRC,), "the tree's art"),
                      ("SELECT count(*) FROM npcdialog WHERE source = ?", (SRC,), "Christmas dialog"),
                      (f"SELECT count(*) FROM lighttemplates WHERE name IN ({','.join('?' * len(lights['templates']))}) COLLATE NOCASE",
                       tuple(t["name"] for t in lights["templates"]), "a light template of the same name")]:
    if db.execute(q, args).fetchone()[0]:
        sys.exit(f"{what} is here already: nothing done")
t = row("npcs", "id = ?", (TEMPLATE_ROW[0],))
if t is None or t["npc_name"] != TEMPLATE_ROW[1] or (t["modelid"] & 0xFFFFFFFF) != PS2_BODY:
    sys.exit(f"npcs {TEMPLATE_ROW[0]} is {t and (t['npc_name'], hex(t['modelid'] & 0xFFFFFFFF))}, not {TEMPLATE_ROW[1]}: nothing done")
for tid, name in (TEMPLATE_ROW, ELF_TEMPLATE):
    r = row("npcs", "id = ?", (tid,))
    if r is None or r["npc_name"] != name:
        sys.exit(f"npcs {tid} is {r and r['npc_name']}, not {name}: nothing done")
if row("itempattern", "patternid = ?", (TOKEN_COPY,)) is None:
    sys.exit(f"no item {TOKEN_COPY} to copy for the token: nothing done")
j = {"npcs": [], "strings": []}
# the tree
cols = [c for c in t if c != "id"]
nid = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) SELECT {', '.join(cols)} FROM npcs WHERE id = ?", (TEMPLATE_ROW[0],)).lastrowid
x, y, z, fb = TREE_AT
db.execute("UPDATE npcs SET npc_name = ?, zone = 0, unk_a = 0, unk_b = 0, world = 0, x_coord = ?, y_coord = ?, z_coord = ?, facing = ?, "
           "npc_type = 128, npc_level = 1, npc_id = ?, size = ?, hp = 100 WHERE id = ?", (TREE, x, y, z, fb, nid, SIZE, nid))
audit("insert", "npcs", {"id": nid}, None, row("npcs", "id = ?", (nid,)))
j["npcs"].append(nid)
db.execute("INSERT INTO npcmove (name, roam, radius, water, note, source) VALUES (?, 0, 0, '', 'stands (the Christmas tree)', ?)", (TREE.lower(), SRC))
db.execute("INSERT INTO npcaggro (npc_name, aggressive, radius, note, source) VALUES (?, 0, 0, 'the Christmas tree: a decoration', ?)", (TREE, SRC))
audit("insert", "npcmove", {"source": SRC}, None, {"names": [TREE]})
# the season (off until a GM fires the trigger) and the tree's place in it
db.execute("INSERT INTO gmseasons (name, active, changed, by, note) VALUES (?, 0, ?, 'tool', ?)",
           (SEASON, now, "Freeport's Christmas tree and its lights; the Christmas Season trigger turns it"))
db.execute("INSERT INTO npcseason (name, season, note, source) VALUES (?, ?, 'Freeport north plaza', ?)", (TREE, SEASON, SRC))
db.execute("INSERT INTO decorart (name, model, world, zone, sink, note, source) VALUES (?, ?, ?, ?, ?, ?, ?)",
           (TREE, FIR, FIR_WORLD, FIR_ZONE, SINK, "a full fir of Tunaria's north (LOD sprite), its bare stem sunk; a PS2 sees the row's treant", SRC))
audit("insert", "gmseasons", {"name": SEASON}, None, {"active": 0})
audit("insert", "npcseason", {"name": TREE}, None, {"season": SEASON})
audit("insert", "decorart", {"name": TREE}, None, {"model": FIR, "world": FIR_WORLD, "zone": FIR_ZONE, "sink": SINK})
# the lights
for tp in lights["templates"]:
    db.execute("INSERT INTO lighttemplates (name, colors, pattern, effect, period, spacing, size, bright, cord, cord_width, note, source) "
               "VALUES (?,?,?,?,?,?,?,?,?,?,?,?)",
               (tp["name"], tp["colors"], tp["pattern"], tp["effect"], tp["period"], tp["spacing"], tp["size"], tp["bright"],
                tp.get("cord", "1E3F1E"), tp.get("cord_width", 0.06), tp.get("note", ""), SRC))
    audit("insert", "lighttemplates", {"name": tp["name"]}, None, tp)
for st in lights["strings"]:
    sid = db.execute("INSERT INTO lightstrings (template, npc, world, shape, params, season, note, source) VALUES (?,?,?,?,?,?,?,?)",
                     (st["template"], st.get("npc", ""), st.get("world", 0), st["shape"], json.dumps(st["params"]), st.get("season", ""), st.get("note", ""), SRC)).lastrowid
    audit("insert", "lightstrings", {"id": sid}, None, st)
    j["strings"].append(sid)
# the Holiday Elf, of the season, friendly, standing
ecols = [c for c in row("npcs", "id = ?", (ELF_TEMPLATE[0],)) if c != "id"]
eid = db.execute(f"INSERT INTO npcs ({', '.join(ecols)}) SELECT {', '.join(ecols)} FROM npcs WHERE id = ?", (ELF_TEMPLATE[0],)).lastrowid
ex, ey, ez, efb = ELF_AT
db.execute("UPDATE npcs SET npc_name = ?, zone = 0, unk_a = 0, unk_b = 0, world = 0, x_coord = ?, y_coord = ?, z_coord = ?, facing = ?, "
           "npc_type = 128, npc_level = 20, npc_id = ? WHERE id = ?", (ELF, ex, ey, ez, efb, eid, eid))
audit("insert", "npcs", {"id": eid}, None, row("npcs", "id = ?", (eid,)))
j["npcs"].append(eid)
db.execute("INSERT INTO npcmove (name, roam, radius, water, note, source) VALUES (?, 0, 0, '', 'stands (by the Christmas tree)', ?)", (ELF.lower(), SRC))
db.execute("INSERT INTO npcaggro (npc_name, aggressive, radius, note, source) VALUES (?, 0, 0, 'the Christmas season: friendly', ?)", (ELF, SRC))
db.execute("INSERT INTO npcseason (name, season, note, source) VALUES (?, ?, 'by the tree: the gift of the day, the holiday quests', ?)", (ELF, SEASON, SRC))
# the currency
icols = [r[1] for r in db.execute("PRAGMA table_info(itempattern)")]
tr = row("itempattern", "patternid = ?", (TOKEN_COPY,))
tr.update(patternid=TOKEN, itemname=TOKEN_NAME, itemdesc=TOKEN_DESC, itemicon=TOKEN_ICON, maxstack=999, trade=0, itemcost=1, levelreq=0, lore=0)
db.execute(f"INSERT INTO itempattern ({','.join(icols)}) VALUES ({','.join('?' * len(icols))})", [tr[c] for c in icols])
audit("insert", "itempattern", {"patternid": TOKEN}, None, {"itemname": TOKEN_NAME, "maxstack": 999, "trade": 0})
db.execute("INSERT INTO seasonrewards (season, item, drop_pct, named_pct, gift_npc, gift_count, hourly, note, source) VALUES (?,?,?,?,?,?,?,?,?)",
           (SEASON, TOKEN, DROP_PCT, NAMED_PCT, ELF, GIFT, HOURLY,
            f"an evening (3 h) earns about {CALIBRATION}: one weapon aura at the Weapon Enchanter", SRC))
audit("insert", "seasonrewards", {"season": SEASON}, None, {"item": TOKEN, "drop_pct": DROP_PCT, "named_pct": NAMED_PCT, "gift": GIFT, "hourly": HOURLY})
# the holiday quests (work entry 1597's rows): the columns they use made if the server has not yet
for tbl, col, decl in (("quests", "asks", "TEXT NOT NULL DEFAULT ''"), ("quests", "daygroup", "TEXT NOT NULL DEFAULT ''"),
                       ("queststeps", "anyorder", "INTEGER NOT NULL DEFAULT 0")):
    if col not in {r[1] for r in db.execute(f"PRAGMA table_info({tbl})")}:
        db.execute(f"ALTER TABLE {tbl} ADD COLUMN {col} {decl}")
QCOLS = {r[1] for r in db.execute("PRAGMA table_info(quests)")}
SCOLS = {r[1] for r in db.execute("PRAGMA table_info(queststeps)")}
ASKS = json.dumps(ELF_QUESTS["asks"], ensure_ascii=False, separators=(",", ":"))
for q in ELF_QUESTS["quests"]:
    qr = dict(q["quest"], questid=q["questid"])
    qr.setdefault("name", {102901: "Gifts for Freeport", 102902: "Carols for the Guard"}.get(q["questid"], ""))
    for k, v in (("enabled", 1), ("giver", ELF), ("minlevel", 1), ("maxlevel", 60), ("repeatable", 2), ("source", SRC),
                 ("notes", "The Christmas season's quest (tools/christmas_season_20261116.py; work entry 1597)")):
        qr.setdefault(k, v)
    if q["questid"] in (102901, 102902):
        qr.setdefault("accept", {102901: "I will carry them.", 102902: "I will sing."}[q["questid"]])
        qr["asks"] = ASKS
    keys = [k for k in qr if k in QCOLS]
    db.execute(f"INSERT INTO quests ({', '.join(keys)}) VALUES ({', '.join('?' * len(keys))})", [qr[k] for k in keys])
    for st in q["steps"]:
        sr = dict(st, questid=q["questid"])
        keys = [k for k in sr if k in SCOLS]
        db.execute(f"INSERT INTO queststeps ({', '.join(keys)}) VALUES ({', '.join('?' * len(keys))})", [sr[k] for k in keys])
    audit("insert", "quests", {"questid": q["questid"]}, None, {"name": qr["name"], "steps": len(q["steps"])})
# the talk
for town, role, name, line in LINES:
    db.execute("INSERT INTO npcdialog (town, role, name, line, weight, source) VALUES (?, ?, ?, ?, 2, ?)", (town, role, name, line, SRC))
audit("insert", "npcdialog", {"source": SRC}, None, {"lines": len(LINES)})
# the trigger: the next free slot past the TEST menu's eight
used = {r[0] for r in db.execute("SELECT slot FROM gmtriggers")}
slot = next(s for s in range(9, 100000) if s not in used)
j["slot"] = slot
steps = [{"do": "season", "season": SEASON, "turn": "toggle",
          "text": "The Christmas season has come to Norrath! A great fir stands lit in Freeport's north plaza.",
          "offtext": "The Christmas season is over. The plaza's fir has been taken down until next year."}]
db.execute("INSERT INTO gmtriggers (slot, name, enabled, scope, radius, world, x, y, z, steps, notes) VALUES (?, ?, 1, 'all', 0, 0, ?, ?, ?, ?, ?)",
           (slot, TRIGGER, x, FLOOR, z, json.dumps(steps, separators=(",", ":")),
            "The owner, 2026-11-16: the Christmas season. Fire it to put Freeport's tree and its lights up (kept over restarts), fire it "
            "again to take them down. tools/christmas_season_20261116.py"))
audit("insert", "gmtriggers", {"slot": slot}, None, {"name": TRIGGER})
print(f"{ELF} npcs {eid}, {TOKEN_NAME} {TOKEN} (drops {DROP_PCT}%/{NAMED_PCT}% named, gift {GIFT}/day, {HOURLY}/hour, "
      f"quests {QUEST_IDS} {QUEST_TOKENS} each): an evening about {CALIBRATION}")
print(f"{TREE} npcs {nid} at ({x}, {y:.3f}, {z}) size {SIZE}, art sunk {SINK} (season {SEASON}, off), art {FIR:#010x} zone {FIR_ZONE}, "
      f"{len(lights['templates'])} light template(s), strings {j['strings']}, {len(LINES)} dialog lines, trigger {slot} '{TRIGGER}'")
if a.apply:
    json.dump(j, open(a.applied, "w")); db.commit(); print("applied")
else:
    db.rollback()
