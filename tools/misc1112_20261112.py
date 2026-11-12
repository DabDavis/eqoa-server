#!/usr/bin/env python3
"""Lane misc1112's data (Heirloom's petitions of 2026-11-12), one transaction, one log, one exact undo:

  sirens (petitions 1739/1741/1742; the GM Jibrish: "Disperse the sirens in groups of 4 that are agro, but do not
    assist. Spread them out."): the enchanting sirens' six stacked groups of four (Vastly Deep, West Toxx, Kerra Isle),
    each with its waterlogged chest, moved apart (npcs x/y/z/facing; tools/misc1112_sirens_plan.json, cmd/sirenplan's:
    each camp's two groups pushed 16 further from the camp's middle along the line between them, so the groups' middles
    stand 55-88 apart, past two aggro radii, 21 each; the four of a group on a ring of 5 round their chest, each spot one
    its walk could reach from where it stood, under the water as before); npcaggro "enchanting siren" aggressive 1 (they
    attack on sight, as they did by nature); npcsocial "enchanting siren" social 0 (they help no one: as they did, now
    written). Hoofbite's "none of them aggro" was his safe mode (/testinvis, on 02:56-04:52 UTC): nothing to change.
  /reward (petition 1740, Jibrish: "the /reward ability needs to HEAL the player and damage the attacking monster when
    hit"): GM's Reward (spell 905121) heals its bearer 200 and deals 200 back at every hit taken, where it was 25 and 25
    (a level 60 creature's 48 a hit outweighed it; Jibrish stood at 1 HP in GM test mode with it on). Its other
    effects, its hour and its look as they were. spelleffect.effects and line, spellpattern.spelldesc.
  Jebodan (petition 1751, Hoofbite: "When Mervol tells you to pay jebodan in the house, he will not talk to you or
    progress step. however, there was another Jebodan outside between the coach and the river that talks to you"):
    the one in the manor house (row 101608) was spelt "Jeboban", so the step (Jebodan's Price, 101050) took the quest
    import's Jebodan beside the coach (row 122661) alone. The house's is named Jebodan (and npcclass "jebodan" as
    "jeboban"'s); the coach's is deleted with its npcrespawn row (as Bobble-by-Water's second Kayle, petition 1712).
  plague swine (petition 1763, Skywalker: "make all Plague Swine NPC in the game drop Plague Swine tusk at a 75% drop
    rate per kill, also ... rotten hoof at 15% drop rate per kill"): every plague swine (16, Plane of Disease) rolls
    loot table "plague swine tusk" (plagueswine tusk, 701458) at 75 and "plague swine rotten hoof" at 15, one roll each
    a kill (npcloot; loottables rows); "rotten hoof" is item 709310, made as a copy of the plagueswine tusk (its icon
    master centaur's hoof's).
  titles (petitions 1765/1766, Jibrish): npctitle 163008 (Testing Heroes Vendor) "XP Grinding Rewards", 162568
    (Dabrish) "XP/CM's" (table npctitle made when the database has none: entity/npctitle.go).
  Ascended Strategic Blow (petition 1774, Poppapump: "should be a kick animation if possible"): its visual
    (spelleffect 906796 fxdict) the Kick's, 0x0A286E2E (its caster's SetAction 43, the kick), where it was Strategic
    Blow's 0x1292786E (PlayAttack, the weapon's swing). Strategic Blow itself as it was.

Nothing is written unless every row is as this tool found it on Heirloom of 2026-11-12 05:52 UTC (refused, named);
report only by default; --apply in one transaction, a gm_audit line a row, log misc1112_log; a second --apply does
nothing; --undo puts back every row still as written (a row changed since is left and named) and the ids it took.

Restart: NPC rows, npcaggro/npcsocial (the panel reloads those at once), npctitle (the panel's edit reloads it) and
the binary for the titles; the loot, the reward and the visual are read at each kill / #reward / cast.

    tools/misc1112_20261112.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

LOG = "misc1112_log"
WHO, VIA = "dab", "tool"
SOURCE = "misc1112"
DETAIL = "lane misc1112 (petitions 1739-1774 of 2026-11-12): tools/misc1112_20261112.py"
HERE = os.path.dirname(os.path.abspath(__file__))
PLAN = os.path.join(HERE, "misc1112_sirens_plan.json")
EPS = 0.01

KEYS = {"npcs": ("id",), "npcaggro": ("npc_name",), "npcsocial": ("npc_name",), "spelleffect": ("spellid",),
        "spellpattern": ("spellid",), "npcrespawn": ("npc_row",), "npcclass": ("name",), "itempattern": ("patternid",),
        "loottables": ("id",), "npcloot": ("npc_name", "tablename"), "npctitle": ("npcid",)}
SEQS = ("loottables", "itempattern")

# --- /reward: the record as gm_reward_20261111.py wrote it, and as it is now (internal/entity/gmreward_test.go reads
# REWARD_EFFECTS)
REWARD = 905121
REWARD_EFFECTS_WAS = '{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"duration_s":3600,"stack":905121,"level":60,"min_level":1,"req_equipment":"Any","sheet_type":"Statistic Change","arch_type":"GM Reward","effects":[{"id":"GRAC","type":"Statistic Change","kind":"stat","stat":"ac","amount":100},{"id":"GRST","type":"Statistic Change","kind":"stat","stat":"str","amount":50},{"id":"GRSA","type":"Statistic Change","kind":"stat","stat":"sta","amount":50},{"id":"GRDX","type":"Statistic Change","kind":"stat","stat":"dex","amount":50},{"id":"GRWS","type":"Statistic Change","kind":"stat","stat":"wis","amount":50},{"id":"GRIN","type":"Statistic Change","kind":"stat","stat":"int","amount":50},{"id":"GRCH","type":"Statistic Change","kind":"stat","stat":"cha","amount":50},{"id":"GRDP","type":"Defensive Proc","kind":"def_proc","procs":[{"id":"GRDS","type":"Dmg/Heal","kind":"hp","amount":-25,"on":"defense","scope":"target","target_type":"Enemy"},{"id":"GRHL","type":"Dmg/Heal","kind":"hp","amount":25,"on":"defense","scope":"self","target_type":"Self"}]}]}'
REWARD_EFFECTS = '{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"duration_s":3600,"stack":905121,"level":60,"min_level":1,"req_equipment":"Any","sheet_type":"Statistic Change","arch_type":"GM Reward","effects":[{"id":"GRAC","type":"Statistic Change","kind":"stat","stat":"ac","amount":100},{"id":"GRST","type":"Statistic Change","kind":"stat","stat":"str","amount":50},{"id":"GRSA","type":"Statistic Change","kind":"stat","stat":"sta","amount":50},{"id":"GRDX","type":"Statistic Change","kind":"stat","stat":"dex","amount":50},{"id":"GRWS","type":"Statistic Change","kind":"stat","stat":"wis","amount":50},{"id":"GRIN","type":"Statistic Change","kind":"stat","stat":"int","amount":50},{"id":"GRCH","type":"Statistic Change","kind":"stat","stat":"cha","amount":50},{"id":"GRDP","type":"Defensive Proc","kind":"def_proc","procs":[{"id":"GRDS","type":"Dmg/Heal","kind":"hp","amount":-200,"on":"defense","scope":"target","target_type":"Enemy"},{"id":"GRHL","type":"Dmg/Heal","kind":"hp","amount":200,"on":"defense","scope":"self","target_type":"Self"}]}]}'
REWARD_LINE_WAS = "Target, Damage Shield 25, Heal 25 when hit, Increase AC by 100, Increase STR STA DEX WIS INT CHA by 50, 1hr"
REWARD_LINE = "Target, Damage Shield 200, Heal 200 when hit, Increase AC by 100, Increase STR STA DEX WIS INT CHA by 50, 1hr"
REWARD_DESC_WAS = ("A GM's reward: a damage shield of 25 and a heal of 25 at every hit taken, 100 more AC and 50 more "
                   "STR, STA, DEX, WIS, INT and CHA, for an hour of play.")
REWARD_DESC = ("A GM's reward: a damage shield of 200 and a heal of 200 at every hit taken, 100 more AC and 50 more "
               "STR, STA, DEX, WIS, INT and CHA, for an hour of play.")

# --- Jebodan
JEB_HOUSE, JEB_SEED = 101608, 122661

# --- plague swine
SWINE = "plague swine"
TUSK, HOOF = 701458, 709310
HOOF_ICON = -720619736  # master centaur's hoof's
TUSK_TABLE, HOOF_TABLE = "plague swine tusk", "plague swine rotten hoof"
TUSK_CHANCE, HOOF_CHANCE = 75, 15

# --- titles
TITLES = [(163008, "Testing Heroes Vendor", "XP Grinding Rewards", "petition 1765 (Jibrish)"),
          (162568, "Dabrish", "XP/CM's", "petition 1766 (Jibrish)")]
TITLE_SCHEMA = """CREATE TABLE IF NOT EXISTS npctitle (
		npcid  INTEGER PRIMARY KEY,
		title  TEXT NOT NULL DEFAULT '',
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm'
	)"""

# --- Ascended Strategic Blow
ASC_BLOW = 906796
FX_BLOW, FX_KICK = 311588974, 170421806

SIREN = "enchanting siren"


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
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def dumps(m):
    return json.dumps(m, sort_keys=True, default=str)


def plan(db):
    """(changes [(tbl, before|None, after|None)], lines, refusals, make_title_table)."""
    ch, lines, no = [], [], []

    # sirens
    with open(PLAN) as f:
        spots = json.load(f)
    moved = 0
    for s in spots:
        r = row_of(db, "npcs", {"id": s["id"]})
        if r is None or r["npc_name"] != s["name"] or r["world"] != 2:
            no.append(f"npcs {s['id']}: not the {s['name']} of world 2 the plan read ({None if r is None else r['npc_name']!r})")
            continue
        after = dict(r, x_coord=round(s["x"], 3), y_coord=round(s["y"], 3), z_coord=round(s["z"], 3), facing=s["facing"])
        ch.append(("npcs", r, after))
        moved += 1
    # each spot as the plan found it: within 25 of its group's chest, and no GM moved it since (gm_audit)
    ids = [s["id"] for s in spots]
    q = ",".join("?" * len(ids))
    if table(db, "gm_audit"):
        for (k,) in db.execute(f"SELECT rowkey FROM gm_audit WHERE tbl = 'npcs' AND at > '2026-11-12T05:52' AND rowkey IN "
                               f"({','.join(chr(39) + go_json({'id': i}) + chr(39) for i in ids)})"):
            no.append(f"npcs {k}: changed by a GM since the plan (gm_audit)")
    lines.append(f"sirens: {moved} rows moved ({sum(1 for s in spots if s['name'] == SIREN)} enchanting sirens, "
                 f"{sum(1 for s in spots if s['name'] != SIREN)} waterlogged chests) into six groups of four, 55-88 apart")
    for tbl, row in (("npcaggro", {"npc_name": SIREN, "aggressive": 1, "radius": 0.0, "leash": 0.0, "holding": 0,
                                   "note": "petition 1741 (Jibrish): the siren camps' groups of four attack on sight",
                                   "source": SOURCE}),
                     ("npcsocial", {"npc_name": SIREN, "social": 0, "faction": "", "radius": 0.0,
                                    "note": "petition 1741 (Jibrish): \"agro, but do not assist\"", "source": SOURCE})):
        if row_of(db, tbl, row) is not None:
            no.append(f"{tbl} {SIREN!r}: a row is there already")
        ch.append((tbl, None, row))
    lines.append(f"  npcaggro {SIREN!r} aggressive 1; npcsocial {SIREN!r} social 0")

    # /reward
    e = row_of(db, "spelleffect", {"spellid": REWARD})
    p = row_of(db, "spellpattern", {"spellid": REWARD})
    if e is None or p is None:
        no.append(f"GM's Reward ({REWARD}) has no record")
    else:
        if e["effects"] != REWARD_EFFECTS_WAS or e["line"] != REWARD_LINE_WAS:
            no.append(f"spelleffect {REWARD}: not gm_reward_20261111's (effects or line changed)")
        if p["spelldesc"] != REWARD_DESC_WAS or p["spellname"] != "GM's Reward":
            no.append(f"spellpattern {REWARD}: not gm_reward_20261111's")
        ch.append(("spelleffect", e, dict(e, effects=REWARD_EFFECTS, line=REWARD_LINE)))
        ch.append(("spellpattern", p, dict(p, spelldesc=REWARD_DESC)))
    lines.append("GM's Reward: 25 -> 200 back at every hit taken, 25 -> 200 healed")

    # Jebodan
    h = row_of(db, "npcs", {"id": JEB_HOUSE})
    sd = row_of(db, "npcs", {"id": JEB_SEED})
    if h is None or h["npc_name"] != "Jeboban" or h["world"] != 0 or abs(h["x_coord"] - 13548.4) > 1 or abs(h["z_coord"] - 14730.1) > 1:
        no.append(f"npcs {JEB_HOUSE}: not the manor house's Jeboban")
    else:
        ch.append(("npcs", h, dict(h, npc_name="Jebodan")))
    if sd is None or sd["npc_name"] != "Jebodan" or abs(sd["x_coord"] - 13712.1) > 1 or abs(sd["z_coord"] - 14813.5) > 1:
        no.append(f"npcs {JEB_SEED}: not the quest import's Jebodan by the coach")
    else:
        ch.append(("npcs", sd, None))
        for t, k in (("npcrespawn", {"npc_row": JEB_SEED}),):
            r = row_of(db, t, k)
            if r is not None:
                ch.append((t, r, None))
        for t in ("npcitem", "npcwander", "npcpaths", "npctitle"):
            if table(db, t) and db.execute(f"SELECT count(*) FROM {t} WHERE npcid = ?", (JEB_SEED,)).fetchone()[0]:
                no.append(f"{t} holds a row of npcs {JEB_SEED}")
    c = row_of(db, "npcclass", {"name": "jeboban"})
    if row_of(db, "npcclass", {"name": "jebodan"}) is not None:
        no.append("npcclass 'jebodan' is there already")
    elif c is not None:
        ch.append(("npcclass", None, dict(c, name="jebodan", source=SOURCE)))
    lines.append(f"Jebodan: row {JEB_HOUSE} (the manor house) Jeboban -> Jebodan; row {JEB_SEED} (by the coach) deleted with its respawn row")

    # plague swine
    tusk = row_of(db, "itempattern", {"patternid": TUSK})
    if tusk is None or tusk["itemname"] != "plagueswine tusk":
        no.append(f"itempattern {TUSK} is not the plagueswine tusk")
    elif row_of(db, "itempattern", {"patternid": HOOF}) is not None:
        no.append(f"itempattern {HOOF} is taken")
    elif db.execute("SELECT count(*) FROM itempattern WHERE lower(itemname) = 'rotten hoof'").fetchone()[0]:
        no.append("an item named rotten hoof is there already")
    else:
        ch.append(("itempattern", None, dict(tusk, patternid=HOOF, itemname="rotten hoof",
                                             itemdesc="A rotten hoof from a plague swine.", itemicon=HOOF_ICON)))
    n_swine = db.execute("SELECT count(*) FROM npcs WHERE lower(npc_name) = ?", (SWINE,)).fetchone()[0]
    for tname, item, chance in ((TUSK_TABLE, TUSK, TUSK_CHANCE), (HOOF_TABLE, HOOF, HOOF_CHANCE)):
        if db.execute("SELECT count(*) FROM loottables WHERE tablename = ?", (tname,)).fetchone()[0]:
            no.append(f"loot table {tname!r} is there already")
        if row_of(db, "npcloot", {"npc_name": SWINE, "tablename": tname}) is not None:
            no.append(f"npcloot ({SWINE!r}, {tname!r}) is there already")
        ch.append(("loottables", None, {"tablename": tname, "patternid": item, "weight": 1, "minlevel": 1, "maxlevel": 60,
                                        "source": SOURCE}))
        ch.append(("npcloot", None, {"npc_name": SWINE, "tablename": tname, "chance": chance, "rolls": 1,
                                     "note": "petition 1763 (Skywalker): every plague swine, a roll a kill", "source": SOURCE}))
    lines.append(f"plague swine ({n_swine} rows): plagueswine tusk at {TUSK_CHANCE}%, rotten hoof (new item {HOOF}) at {HOOF_CHANCE}%")

    # titles
    make = not table(db, "npctitle")
    for nid, name, title, why in TITLES:
        r = row_of(db, "npcs", {"id": nid})
        if r is None or r["npc_name"] != name:
            no.append(f"npcs {nid}: not {name!r}")
        if not make and row_of(db, "npctitle", {"npcid": nid}) is not None:
            no.append(f"npctitle {nid}: a row is there already")
        ch.append(("npctitle", None, {"npcid": nid, "title": title, "note": why, "source": SOURCE}))
        lines.append(f"title: {name} -> {title!r}")

    # Ascended Strategic Blow
    a = row_of(db, "spelleffect", {"spellid": ASC_BLOW})
    sp = row_of(db, "spellpattern", {"spellid": ASC_BLOW})
    if a is None or sp is None or sp["spellname"] != "Ascended Strategic Blow" or a["fxdict"] != FX_BLOW:
        no.append(f"spelleffect {ASC_BLOW}: not Ascended Strategic Blow on Strategic Blow's visual")
    else:
        ch.append(("spelleffect", a, dict(a, fxdict=FX_KICK)))
    lines.append("Ascended Strategic Blow: visual 0x1292786E (a weapon swing) -> 0x0A286E2E (Kick's: the kick)")
    return ch, lines, no, make


LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,
    rowkey  TEXT NOT NULL,      -- the row's key (JSON)
    before  TEXT,               -- the row before (JSON); NULL: made here
    after   TEXT,               -- the row as written (JSON); NULL: deleted here
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
    if not table(db, "sqlite_sequence"):
        return {}
    out = {}
    for t in SEQS:
        r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (t,)).fetchone()
        out[t] = None if r is None else r[0]
    return out


def apply(db, changes, make):
    db.execute(LOG_SCHEMA)
    before_seq = seqs(db)
    if make:
        db.execute(TITLE_SCHEMA)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES ('made', '\"npctitle\"', NULL, NULL)")
    n = 0
    for tbl, before, after in changes:
        if before is None:
            cols = list(after)
            cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [after[c] for c in cols])
            key = {"id": cur.lastrowid} if tbl == "loottables" else key_of(tbl, after)
            row = row_of(db, tbl, key)
            a = audit(db, "insert", tbl, key, None, row, DETAIL)
        elif after is None:
            key = key_of(tbl, before)
            db.execute(f"DELETE FROM {tbl} WHERE " + " AND ".join(f"{c} = ?" for c in KEYS[tbl]), [key[c] for c in KEYS[tbl]])
            row = None
            a = audit(db, "delete", tbl, key, before, None, DETAIL)
        else:
            key = key_of(tbl, before)
            sets = [c for c in after if after[c] != before.get(c)]
            db.execute(f"UPDATE {tbl} SET {', '.join(c + ' = ?' for c in sets)} WHERE "
                       + " AND ".join(f"{c} = ?" for c in KEYS[tbl]), [after[c] for c in sets] + [key[c] for c in KEYS[tbl]])
            row = row_of(db, tbl, key)
            a = audit(db, "update", tbl, key, before, row, DETAIL)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, json.dumps(key), None if before is None else dumps(before), None if row is None else dumps(row), a))
        n += 1
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES ('sqlite_sequence', '{{}}', ?, ?)",
               (json.dumps(before_seq), json.dumps(seqs(db))))
    return n


def undo(db):
    """Each logged row still as written put back, the last first, a gm_audit line each; then the ids the new rows took
    and the npctitle table this tool made, when nothing else moved them. Returns (back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    seq_row = db.execute(f"SELECT before, after FROM {LOG} WHERE tbl = 'sqlite_sequence'").fetchone()
    made = db.execute(f"SELECT count(*) FROM {LOG} WHERE tbl = 'made'").fetchone()[0] > 0
    for seq, tbl, key, before, after in db.execute(
            f"SELECT seq, tbl, rowkey, before, after FROM {LOG} WHERE tbl NOT IN ('sqlite_sequence', 'made') ORDER BY seq DESC").fetchall():
        key = json.loads(key)
        now = row_of(db, tbl, key)
        if (after is None and now is not None) or (after is not None and (now is None or dumps(now) != after)):
            left += 1
            print(f"  left: {tbl} {key}, changed since it was written")
            continue
        where = " AND ".join(f"{c} = ?" for c in KEYS[tbl])
        if before is None:
            db.execute(f"DELETE FROM {tbl} WHERE {where}", [key[c] for c in KEYS[tbl]])
            audit(db, "delete", tbl, key, now, None, "undo: " + DETAIL)
        elif after is None:
            b = json.loads(before)
            cols = list(b)
            db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [b[c] for c in cols])
            audit(db, "insert", tbl, key, None, row_of(db, tbl, key), "undo: " + DETAIL)
        else:
            b = json.loads(before)
            db.execute(f"UPDATE {tbl} SET {', '.join(c + ' = ?' for c in b)} WHERE {where}", [b[c] for c in b] + [key[c] for c in KEYS[tbl]])
            audit(db, "update", tbl, key, now, row_of(db, tbl, key), "undo: " + DETAIL)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if left == 0:
        if seq_row is not None:
            was, written = json.loads(seq_row[0]), json.loads(seq_row[1])
            cur = seqs(db)
            for t in SEQS:
                if cur.get(t) == written.get(t):
                    if was.get(t) is None:
                        db.execute("DELETE FROM sqlite_sequence WHERE name = ?", (t,))
                    else:
                        db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", (was[t], t))
        if made and table(db, "npctitle") and db.execute("SELECT count(*) FROM npctitle").fetchone()[0] == 0:
            db.execute("DROP TABLE npctitle")
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
        changes, lines, refusals, make = plan(db)
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
            n = apply(db, changes, make)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {n} rows, in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
