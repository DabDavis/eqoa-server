#!/usr/bin/env python3
"""The Plane of Sky's keys and passages (petition 1608, the GM Jibrish, at the second isle of rain's essence: "This needs
to ask for a key from the boss and then teleport people to the next island"; petition 1612, at Granavik: "after granavik
dies he needs to drop 20 keys, and open a portal to the next island called Rain Key 3").

One guide-gaps set, `sky-keys` (tools/guidegaps/sky-keys.json), written by tools/seed_guide_gaps.py's machinery, every
row it changes logged in guide_set_log so the undo is exact:

  - 21 keys, each a copy of Liztin Misseliania's rain key under its own name and words: Rain Key 1-5, Day Key 1-4, Night
    Key 1-5, Snow Key 2-4, Desolation Key 1-4 (the names follow Jibrish's "Rain Key 3": the key of the isle it is used
    on); the main isle's three essences keep taking the keys Liztin sells;
  - twenty of each on its isle's boss's corpse, every kill (npcitem: one stack of 20; the bosses are those whose shard
    the sky-twins set has the isle drop, Granavik for the third isle of rain as Jibrish named him, the Arch Bishop
    Lancel for the third of night, which has none): Artiokanis, Kaglanor, Granavik, the Spirit of Narciliisisin,
    Chimareon the Tyrant; the brightstar princess, Imerzan, the torrential wind, Randarin the Relentless; the Ancient
    Jester, the iron knight, the Arch Bishop Lancel, the dark baroness, King Ruthra; the Whirlwind, the glacial
    champion, Enraged Aeyinar; the Erych Arbiter, the warlord of Narcillian, the elite harpy queen, the Quintessence;
  - the Rain 3 Portal (a spawn-only NPC with the essence of rain's body), which Granavik's death stands where he fell
    for 15 minutes (trigger slot 156, Rain 3 Portal Opens; npcdeath granavik), taking a Rain Key 3 to the fourth isle;
  - the chains' 23 essence steps lose their own teleport (the passage, handlers/skypass.go, moves the player and its
    group now, the key in hand); the snow chain's way back to the igloo village keeps its own;
  - questunlocks: the chain of snow (101307) opens the Mark of Desolation's trip to the first isle of desolation.

Refused, nothing written, unless the rain key is there as imported, none of the keys' names nor the portal's is taken,
trigger slot 156 is free, each boss stands in world 4, and each of the 23 steps has exactly its one teleport. Report only
by default (the set run and rolled back); --apply in one transaction with a gm_audit line; a second --apply refuses
(the set is in); --undo exact, with its own line. Keys characters hold go with an undo. The code (skypass.go) and this
go together: NPCs, quests, triggers and npcdeath are read at the server's start, so a restart after either.

    tools/sky_keys_20261111.py --db eqoa.db [--apply | --undo]
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

SET = "sky-keys"
WHO, VIA = "dab", "tool"
DETAIL = "The Plane of Sky's keys and passages (petitions 1608, 1612, the GM Jibrish): tools/sky_keys_20261111.py"


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
    if sgg.has(db, sgg.LOG) and db.execute(f"SELECT 1 FROM {sgg.LOG} WHERE setname = ?", (SET,)).fetchone():
        out.append(f"set {SET} is in already")
        return out
    if not db.execute("SELECT 1 FROM itempattern WHERE lower(itemname) = 'rain key'").fetchone():
        out.append("no record 'rain key' to copy the keys from")
    for it in s["items"]:
        if db.execute("SELECT 1 FROM itempattern WHERE lower(trim(itemname)) = lower(?)", (it["name"],)).fetchone():
            out.append(f"a record is named {it['name']!r} already")
    if db.execute("SELECT 1 FROM npcs WHERE lower(npc_name) = 'rain 3 portal'").fetchone():
        out.append("an NPC is named Rain 3 Portal already")
    for t in s["triggers"]:
        r = db.execute("SELECT name FROM gmtriggers WHERE slot = ?", (t["slot"],)).fetchone()
        if r:
            out.append(f"trigger slot {t['slot']} is taken: {r[0]!r}")
    for stmt, params in s["sql"]:
        if stmt.startswith("INSERT INTO npcitem"):
            if not db.execute("SELECT 1 FROM npcs WHERE world = 4 AND lower(npc_name) = lower(?)", (params[1],)).fetchone():
                out.append(f"no {params[1]!r} in world 4")
        elif stmt.startswith("UPDATE queststeps"):
            r = db.execute("SELECT events FROM queststeps WHERE questid = ? AND step = ?", params).fetchone()
            ev = json.loads(r[0]) if r and r[0] else []
            if len(ev) != 1 or ev[0].get("do") != "teleport" or ev[0].get("on") != "done":
                out.append(f"quest {params[0]} step {params[1]}'s events are not its one teleport: {r}")
    if not db.execute("SELECT 1 FROM quests WHERE questid = 101307").fetchone():
        out.append("no quest 101307 (The Chain of Snow) to unlock the Mark of Desolation's trip")
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
