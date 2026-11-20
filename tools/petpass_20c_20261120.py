#!/usr/bin/env python3
"""Petition pass 20c's data (Heirloom petitions 2504, 2515, 2516, 2517, 2026-11-20). Each part is checked against
the rows as read on 2026-11-20 and left out (said) where a shard's differ; nothing is written unless --apply.

  2517 (Waldorf): "Trissen doesn't speak to you when Yindrius sends you there. I was able to kill him but he was a low
       level green when he should have spawned red to the character that talked to him." Board 901 section B
       (jibrish, owner-approved): Trissen (101360) is raised by his quest. A new step 1 (reach Bobble-by-Water, 60 of
       his place) raises him hostile, for the player's group, his con red to whoever raises him ("player+3", the
       spawn event's con: e48559cd), with a line of his [ours]; old steps 1-2 become 2-3, a character on the quest
       moved with its step. npcspawnonly "trissen". Board 901 asked factionhits 715 deleted: since then petfix581
       made it Prima's town hit (-10), so it is kept.
  2516 (Deathknell): "Mobs for Heroic were Hunter quest are spawning as Level 63 separate mobs and the original NPC is
       still standing there in human form." The Heroic Werehunter (101391): each step whose done spawns a lycanthrope
       hides the human it stands for (the hide event, e48559cd), as long as the lycanthrope stays (its minutes; Slick
       and Bucktooth 20, Bucktooth's beast coming at step 3). Their levels (63, 65) are left as they are.
  2515 (Waldorf): "Ratwere/Wererats aren't dropping any were parts ... might just be the high level ones". A tails
       step counts its target's exact name: the elder, ancient, great and vicious weres of every line (and werelion,
       19 of them, 33-44) counted for none. Each line's other names go to queststeptargets for the five tails
       quests (101348, 101355, 101362, 101369, 101376), Elder Tchekof (101381, steps 3-7) and A Set of Tails (101392,
       steps 1-5); the werebear witchking (a named) is left out.
  2504 (Tolas), with --gator only: "Both GW forms are the small baby alligator form." Primordial Gator Form (Feral
       Gator Form 2's spell) wore the workbook's sprite 0xE2201291, the strapped lizardman (bodyLizardman, the
       Lizardman race's own body, so it took that race's size too), where every other line's second Feral form is
       its first's creature's bigger kin: it wears the gharial's body (0x015C345E, sprite 0x5E345C01), the blue
       long-snouted crocodilian of Mudscale and the ancient gharials. OURS, a choice of body: offered, not decided.

Quests and npcspawnonly are read at the server's start, and parts 2516/2517 need the binary with the hide event and
the con (e48559cd): run it as that keep restart's before.sh. Part 2504's spell at the next cast after the restart.
One transaction; each row change logged in petpass_20c_log (before and after, whole) and gm_audit; a second --apply
finds what is done; --undo puts back every row exactly (refused when one changed since).

    tools/petpass_20c_20261120.py --db eqoa.db [--apply | --undo] [--gator]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "petpass_20c_log"
DETAIL = "petition pass 20c (2504, 2515, 2516, 2517): tools/petpass_20c_20261120.py"
WHO, VIA = "claude", "tool"

TRISSEN_Q, TRISSEN_NAME = 101360, "Trissen"
TRISSEN_OLD = [("kill", "Trissen"), ("talk", "Yindrius")]
TRISSEN_PLACE = (0, 24308.0, 54.0, 11402.0)
TRISSEN_REACH = dict(
    kind="reach", target="", world=0, x=24308.0, y=54.0, z=11402.0, radius=60.0, count=1,
    journal="Look for Trissen in Bobble-by-Water.",
    events=json.dumps([
        {"on": "done", "do": "spawn", "npc": "Trissen", "hostile": True, "con": "player+3", "group": True, "minutes": 15},
        {"on": "done", "do": "say", "npc": "Trissen", "text": "Yindrius sent a rat-friend after me? You will not leave Bobble-by-Water."},
    ], separators=(",", ":")))

WEREHUNT_Q, WEREHUNT_NAME = 101391, "Heroic Werehunter"
# step -> (the lycanthrope its done spawns, the humans to hide, minutes)
HIDES = {
    1: (("Lycanthrope Svenson", "Lycanthrope Stevenson"), ("Slick Svenson", "Bucktooth"), 20),
    5: (("Lycanthrope Lynda",), ("Lynda Elmquist",), 10),
    10: (("Lycanthrope Ipstok",), ("Necrologist Ipstok",), 10),
}

# a were line's other names (npcs on 2026-11-20), the tails steps' targets each
LINE_NAMES = {
    "wolfwere": ["werewolf", "elder wolfwere", "great wolfwere", "vicious wolfwere", "elder werewolf", "ancient werewolf"],
    "bearwere": ["werebear", "elder bearwere", "great bearwere", "ancient bearwere", "elder werebear"],
    "wererat": ["ratwere", "elder wererat", "elder ratwere", "ancient wererat", "ancient ratwere"],
    "lionwere": ["werelion", "elder lionwere", "great lionwere", "great werelion", "elder werelion", "ancient werelion"],
    "gatorwere": ["weregator", "elder gatorwere", "great gatorwere", "elder weregator", "ancient gatorwere", "ancient weregator"],
}
TAILS = [(101348, 1, "wolfwere"), (101355, 1, "bearwere"), (101362, 1, "wererat"), (101369, 1, "lionwere"),
         (101376, 1, "gatorwere"), (101381, 3, "wolfwere"), (101381, 4, "bearwere"), (101381, 5, "wererat"),
         (101381, 6, "lionwere"), (101381, 7, "gatorwere"), (101392, 1, "wolfwere"), (101392, 2, "bearwere"),
         (101392, 3, "wererat"), (101392, 4, "lionwere"), (101392, 5, "gatorwere")]

GATOR_NAME, GATOR_OLD, GATOR_NEW = "Primordial Gator Form", "0xE2201291", "0x5E345C01"


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    cols = [d[0] for d in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def audit(db, action, tbl, key, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key), go_json(before), go_json(after), detail))


class Plan:
    """Row changes, in order: (tbl, key, before, after), a whole row each (None: none)."""

    def __init__(self):
        self.changes, self.notes, self.refusals = [], [], []

    def put(self, tbl, key, before, after):
        self.changes.append((tbl, key, before, after))


def done_already(db):
    return table(db, LOG) and db.execute(f"SELECT COUNT(*) FROM {LOG}").fetchone()[0] > 0


def plan_trissen(db, p):
    q = rows(db, "SELECT name FROM quests WHERE questid = ?", (TRISSEN_Q,))
    if not q or q[0]["name"] != TRISSEN_NAME:
        p.notes.append(f"2517: quest {TRISSEN_Q} is {q and q[0]['name']!r} here: left out")
        return
    old = rows(db, "SELECT * FROM queststeps WHERE questid = ? ORDER BY step", (TRISSEN_Q,))
    if [(r["kind"], r["target"]) for r in old] != TRISSEN_OLD:
        p.notes.append(f"2517: Trissen's steps are {[(r['kind'], r['target']) for r in old]}, not the import's two: left out")
        return
    for t in ("queststeptargets", "questwalls", "questdoors"):
        if table(db, t) and db.execute(f"SELECT COUNT(*) FROM {t} WHERE questid = ?", (TRISSEN_Q,)).fetchone()[0]:
            p.refusals.append(f"2517: {t} has rows of quest {TRISSEN_Q}: their steps would need moving too")
            return
    if not db.execute("SELECT COUNT(*) FROM npcs WHERE npc_name = 'Trissen'").fetchone()[0]:
        p.refusals.append("2517: no NPC Trissen to raise")
        return
    for r in old:
        p.put("queststeps", {"questid": TRISSEN_Q, "step": r["step"]}, r, None)
    blank = {k: ("" if isinstance(v, str) else 0) for k, v in old[0].items()}
    first = dict(blank, questid=TRISSEN_Q, step=1, **TRISSEN_REACH)
    p.put("queststeps", {"questid": TRISSEN_Q, "step": 1}, None, first)
    for r in old:
        n = dict(r, step=r["step"] + 1)
        p.put("queststeps", {"questid": TRISSEN_Q, "step": n["step"]}, None, n)
    on = rows(db, "SELECT * FROM charquests WHERE questid = ? AND step > 0", (TRISSEN_Q,))
    for r in on:
        p.put("charquests", {"serverid": r["serverid"], "questid": TRISSEN_Q}, r, dict(r, step=r["step"] + 1, count=0))
    if not rows(db, "SELECT * FROM npcspawnonly WHERE name = 'trissen'"):
        p.put("npcspawnonly", {"name": "trissen"}, None,
              {"name": "trissen", "note": "petition 2517 / board 901: raised by his quest (101360 step 1), red to its raiser", "source": "gm"})
    p.notes.append(f"2517: Trissen 2 steps -> 3 (a reach in Bobble-by-Water raises him, con player+3); {len(on)} on it moved; spawn-only")


def plan_werehunter(db, p):
    q = rows(db, "SELECT name FROM quests WHERE questid = ?", (WEREHUNT_Q,))
    if not q:
        p.notes.append(f"2516: no quest {WEREHUNT_Q} here: left out")
        return
    for step, (beasts, humans, minutes) in HIDES.items():
        r = rows(db, "SELECT * FROM queststeps WHERE questid = ? AND step = ?", (WEREHUNT_Q, step))
        if not r:
            continue
        r = r[0]
        evs = json.loads(r["events"] or "[]")
        if not any(e.get("on") == "done" and e.get("do") == "spawn" and e.get("npc") in beasts for e in evs):
            p.notes.append(f"2516: step {step} raises none of {beasts} here: left out")
            continue
        have = {e.get("npc") for e in evs if e.get("do") == "hide"}
        add = [{"on": "done", "do": "hide", "npc": h, "minutes": minutes} for h in humans if h not in have]
        if not add:
            continue
        for h in humans:
            if not db.execute("SELECT COUNT(*) FROM npcs WHERE npc_name = ?", (h,)).fetchone()[0]:
                p.refusals.append(f"2516: no NPC {h!r} to hide")
                return
        p.put("queststeps", {"questid": WEREHUNT_Q, "step": step}, r, dict(r, events=json.dumps(add + evs, separators=(",", ":"))))
        p.notes.append(f"2516: step {step} hides {', '.join(a['npc'] for a in add)} for {minutes} minutes")


def plan_tails(db, p):
    n = 0
    for quest, step, line in TAILS:
        st = rows(db, "SELECT kind, target FROM queststeps WHERE questid = ? AND step = ?", (quest, step))
        if not st or st[0]["kind"] not in ("loot", "kill") or st[0]["target"].strip().lower() != line:
            p.notes.append(f"2515: {quest} step {step} is {st and (st[0]['kind'], st[0]['target'])}, not a {line} hunt: left out")
            continue
        have = {r["target"].lower() for r in rows(db, "SELECT target FROM queststeptargets WHERE questid = ? AND step = ?", (quest, step))}
        for name in LINE_NAMES[line]:
            if name in have or not db.execute("SELECT COUNT(*) FROM npcs WHERE lower(trim(npc_name)) = ?", (name,)).fetchone()[0]:
                continue
            row = {"questid": quest, "step": step, "target": name}
            p.put("queststeptargets", row, None, row)
            n += 1
    p.notes.append(f"2515: {n} other were names counted by the tails steps")


def plan_gator(db, p):
    r = rows(db, "SELECT e.* FROM spelleffect e WHERE e.spellid IN (SELECT spellid FROM spellpattern WHERE spellname = ?)", (GATOR_NAME,))
    if len(r) != 1:
        p.notes.append(f"2504: {len(r)} spelleffect rows of {GATOR_NAME!r} here: left out")
        return
    r = r[0]
    fx = json.loads(r["effects"])
    ill = [e for e in fx.get("effects", []) if e.get("kind") == "illusion"]
    if len(ill) != 1 or ill[0].get("sprite", "").lower() != GATOR_OLD.lower():
        p.notes.append(f"2504: {GATOR_NAME}'s illusion is {[e.get('sprite') for e in ill]}, not {GATOR_OLD}: left out")
        return
    ill[0]["sprite"] = GATOR_NEW
    p.put("spelleffect", {"spellid": r["spellid"]}, dict(r), dict(r, effects=json.dumps(fx, separators=(",", ":"))))
    p.notes.append(f"2504: {GATOR_NAME} ({r['spellid']}) the gharial's body ({GATOR_NEW})")


def plan(db, gator):
    p = Plan()
    for t in ("quests", "queststeps", "queststeptargets", "charquests", "npcspawnonly", "npcs", "gm_audit"):
        if not table(db, t):
            p.refusals.append(f"no {t} table here")
    if p.refusals:
        return p
    plan_trissen(db, p)
    plan_werehunter(db, p)
    plan_tails(db, p)
    if gator:
        plan_gator(db, p)
    return p


def insert(db, tbl, r):
    db.execute(f"INSERT INTO {tbl} ({', '.join(r)}) VALUES ({', '.join('?' for _ in r)})", list(r.values()))


def where(key):
    return " AND ".join(f"{k} = ?" for k in key), list(key.values())


def apply(db, p):
    db.execute(f"""CREATE TABLE IF NOT EXISTS {LOG} (
        seq INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT NOT NULL, rowkey TEXT NOT NULL, before TEXT, after TEXT)""")
    for tbl, key, before, after in p.changes:
        w, args = where(key)
        if before is not None:
            if db.execute(f"DELETE FROM {tbl} WHERE {w}", args).rowcount != 1:
                raise RuntimeError(f"{tbl} {key}: gone under us")
        if after is not None:
            insert(db, tbl, after)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES (?, ?, ?, ?)",
                   (tbl, json.dumps(key, sort_keys=True), json.dumps(before, sort_keys=True), json.dumps(after, sort_keys=True)))
        audit(db, "insert" if before is None else "delete" if after is None else "update", tbl, key, before, after, DETAIL)


def now_row(db, tbl, key):
    w, args = where(key)
    r = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)
    return r[0] if r else None


def undo(db):
    if not table(db, LOG):
        return 0
    entries = rows(db, f"SELECT * FROM {LOG} ORDER BY seq DESC")
    # the rows as written last stand, by key (a key written twice, delete then insert, is its last write)
    last = {}
    for e in reversed(entries):
        last[(e["tbl"], e["rowkey"])] = json.loads(e["after"])
    changed = [f"{t} {k}" for (t, k), after in last.items() if now_row(db, t, json.loads(k)) != after]
    if changed:
        raise SystemExit("REFUSED: changed since it was written: " + "; ".join(changed))
    for e in entries:
        tbl, key = e["tbl"], json.loads(e["rowkey"])
        before, after = json.loads(e["before"]), json.loads(e["after"])
        w, args = where(key)
        if after is not None:
            db.execute(f"DELETE FROM {tbl} WHERE {w}", args)
        if before is not None:
            insert(db, tbl, before)
        audit(db, "undo", tbl, key, after, before, "undo: " + DETAIL)
    db.execute(f"DROP TABLE {LOG}")
    return len(entries)


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    ap.add_argument("--gator", action="store_true", help="also part 2504 (Primordial Gator Form's body): a choice")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            n = undo(db)
            db.execute("COMMIT")
            print(f"put back {n} row changes" if n else "nothing to undo")
            return 0
        if done_already(db):
            db.execute("ROLLBACK")
            print(f"applied already ({LOG} holds it): nothing to do")
            return 0
        p = plan(db, a.gator)
        for r in p.refusals:
            print("REFUSED:", r)
        for n in p.notes:
            print(n)
        print(f"{len(p.changes)} row changes")
        if p.refusals or not a.apply or not p.changes:
            db.execute("ROLLBACK")
            print("" if p.refusals else "report only (--apply to write)" if p.changes else "nothing to do")
            return 1 if p.refusals else 0
        apply(db, p)
        db.execute("COMMIT")
        print("applied")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
