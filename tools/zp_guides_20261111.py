#!/usr/bin/env python3
"""Zaoran Plateau as questmasters1.com's guide has it (the owner, 2026-11-11, pointing to its archived Zaoran Plateau
pages of 2006-07), beside #494 (the GM Jibrish's 2008-2012 plateau, which stands: its numbers win where both give one).

One guide-gaps set, `zp-factions` (tools/guidegaps/zp-factions.json, its quests tools/quests/data/zp-factions.json),
written by tools/seed_guide_gaps.py's machinery, every row it changes logged in guide_set_log so the undo is exact:

  - the passage: The Way to Zaoran Plateau's show step takes 5,000 tunar ("With the five Ruins and 5,000 tunar you can
    talk to portal NPC and be ported"; the route's 5,000 a trip is the code's, handlers/waysender.go);
  - the three factions (Soldiers of Rallos Zek, Mithaniel Marr's Crusaders, and the Akessan Collective it adds), their
    camps on the plateau (spots ours), their people tagged by faction (npcfaction: they attack only a player below -350
    with them, so a newcomer walks among them, as the guide's "the creatures roaming are all neutral"), a camp soldier's
    kill -5 with its faction (ours);
  - the quests: each side's introduction and alignment (+500 or +499 with it, -2000 with the other two), the Akessan
    Liason's and the Akessans' chain, each side's repeatable weapon turn-in (+16 and 2.4M a stack of four, the Akessan
    Collector's measure), the Master Spellsmiths' abilities at 2,000 (a mystical parchment for #494's scroll of the
    class's archetype);
  - the camps' named and the guide's loot table for each, the armour gems from every named (#494's "zp armour gems");
  - #494's 18 ZP armour turn-ins gated by side (the owner, 2026-11-11): each needs its side's alignment quest finished
    (Soldiers of Rallos Zek, Mithaniel Marr's Crusaders, The Captains' Weapons for the Akessans) and a standing of 0 or
    better with the side; a player holding the gems who has not is refused in the lieutenant's voice (handlers/quests.go).

Refused, nothing written, unless #494's zp-armour set is in (its gem table) with its 18 armour quests as imported, the portal quest's show step is as
imported (tunar 0), and none of the set's NPC names is in world 5 yet. Report only by default (the set run and rolled
back: the counts it would write); --apply in one transaction with a gm_audit line; --undo exact, with its own line.
NPCs, quests, factions, factionhits and npcfaction are read at the server's start: a restart after either.

    tools/zp_guides_20261111.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import seed_guide_gaps as sgg  # noqa: E402

SET = "zp-factions"
WHO, VIA = "dab", "tool"
DETAIL = "Zaoran Plateau's factions and passage as questmasters1.com's guide (the owner, 2026-11-11): tools/zp_guides_20261111.py"
PORTAL_KEY = "Portal/zaoran-plateau/portal"


def spec():
    s = json.load(open(os.path.join(HERE, "guidegaps", SET + ".json")))
    s["set"] = SET
    return s


def audit(db, action, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if sgg.has(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', ?, 'guide_set_log', ?, NULL, ?, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, action, json.dumps({"set": SET}), json.dumps(after, sort_keys=True), DETAIL))


def refusals(db, s):
    out = []
    if not (sgg.has(db, sgg.LOG) and db.execute(f"SELECT 1 FROM {sgg.LOG} WHERE setname = 'zp-armour'").fetchone()):
        out.append("#494's zp-armour set is not in (its armour gem table): apply it first")
    if sgg.has(db, sgg.LOG) and db.execute(f"SELECT 1 FROM {sgg.LOG} WHERE setname = ?", (SET,)).fetchone():
        out.append(f"set {SET} is in already")
    row = db.execute("""SELECT s.kind, s.tunar FROM queststeps s JOIN quest_import_quests i ON i.questid = s.questid
        WHERE i.key = ? AND s.step = 1""", (PORTAL_KEY,)).fetchone() if sgg.has(db, "quest_import_quests") else None
    if not row or row[0] != "show" or row[1] != 0:
        out.append(f"the portal quest's step 1 is not the imported show step with no tunar: {row}")
    for n in s["npcs"]:
        if not isinstance(n.get("facing", 0), int) or not 0 <= n.get("facing", 0) < 256:
            out.append(f"{n['name']}: facing {n.get('facing')!r} is not a whole 0-255 (npcs.facing, 256 a turn)")
    if sgg.has(db, "quest_import_quests"):
        n = db.execute("""SELECT count(*) FROM quests q JOIN quest_import_quests i ON i.questid = q.questid
            WHERE i.key LIKE 'ZP/armour/%' AND q.requires = '' AND q.factionreq = ''""").fetchone()[0]
        if n != 18:
            out.append(f"#494's 18 ZP armour quests are not all as imported (no requires, no standing): {n} are")
    names = sorted({n["name"] for n in s["npcs"]})
    here = [r[0] for r in db.execute(f"SELECT DISTINCT npc_name FROM npcs WHERE world = 5 AND npc_name IN ({','.join('?' * len(names))})", names)]
    if here:
        out.append(f"already in world 5: {here}")
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, timeout=5, isolation_level=None)
    say = print
    if a.undo:
        sgg.undo(db, SET, say, extra=lambda d, n: audit(d, "undo", {"rows": n}))
        return
    s = spec()
    bad = refusals(db, s)
    if bad:
        for b in bad:
            print("refused:", b)
        sys.exit(1)
    sgg.apply(db, s, a.apply, say, extra=lambda d, counts: audit(d, "apply", counts))


if __name__ == "__main__":
    main()
