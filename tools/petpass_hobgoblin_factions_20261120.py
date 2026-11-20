#!/usr/bin/env python3
"""Kill factions for the Rathe Mountains' hobgoblins (petition 2465, Heirloom, 2026-11-20: "Seems like some creature
factions are on and some aren't. Is this something we should create tickets for? Like the Hobgoblins in Ogre
Ruins").

A creature's kill moves its killer's standings by the factionhits rows of its name; the Ogre Ruins' hobgoblins (world
1) had none, nor had any hobgoblin of the Rathe Mountains, the Ankextet clan's included, so a kill there printed no
faction line where the Freezeblood's, the gnolls' and most goblins' do. Prima's faction tables (2003, pp. 383-391;
claude-notes/factions-prima-20261119.md) have both:

  Hobgoblins of Rathe Mtns.  penalty 10 ("Hobgoblins of Rathe Mountains"); associates Craknek Warriors -10 and
                             Citizens of Oggok -10 (enemies: each gains 10% of the penalty, 1): every hobgoblin name of
                             world 1 (13 names: centurion, footman, guard, initiate, mercenary, mystic, oracle, scout,
                             seer, skulk, vandal, warrior, worker)
  Clan Ankextet              penalty 10 ("the Ankextet hobgoblin clan"), no associates: every ankextet name of
                             world 1 (10 names)

So a kill: Hobgoblins of Rathe Mtns. -10, Craknek Warriors +1, Citizens of Oggok +1; or Clan Ankextet -10. A name with
a factionhits row already is left. Report only by default; --apply in one transaction, a gm_audit line a row, log
petpass_hobgoblin_factions_log; --undo exact. factionhits is read at the server's start (after the next server
update). Both shards lack them.

    python3 -E -s tools/petpass_hobgoblin_factions_20261120.py --db eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_hobgoblin_factions_log"
DETAIL = "The Rathe Mountains' hobgoblins' kill factions, Prima's (petition 2465): " \
         "tools/petpass_hobgoblin_factions_20261120.py"
SOURCE = "prima-2003 petition 2465"
KINDS = (
    ("hobgoblin", (("Hobgoblins of Rathe Mtns.", -10), ("Craknek Warriors", 1), ("Citizens of Oggok", 1))),
    ("ankextet", (("Clan Ankextet", -10),)),
)


def changes(db):
    out, lines, refusals = [], [], []
    have = {r["name"] for r in tr.rows(db, "SELECT name FROM factions")}
    hit = {r["n"] for r in tr.rows(db, "SELECT DISTINCT lower(npc_name) AS n FROM factionhits WHERE source != ?",
                                   (SOURCE,))}
    for word, hits in KINDS:
        for f, _ in hits:
            if f not in have:
                refusals.append(f"no faction {f!r} here")
        names = sorted({r["n"] for r in tr.rows(db, "SELECT DISTINCT lower(trim(npc_name)) AS n FROM npcs "
                                                    "WHERE world = 1 AND (npc_type & 128) = 0 AND lower(npc_name) "
                                                    "LIKE ?", (f"%{word}%",))})
        skipped = [n for n in names if n in hit]
        names = [n for n in names if n not in hit]
        for n in names:
            for f, d in hits:
                key = {"npc_name": n, "faction": f}
                out.append(("factionhits", key, dict(key, delta=d, source=SOURCE)))
        lines.append(f"{word}: {len(names)} names, a kill " + ", ".join(f"{f} {d:+d}" for f, d in hits) + ": " +
                     ", ".join(names) + (f" (left, with hits already: {', '.join(skipped)})" if skipped else ""))
    applied = bool(out) and all(tr.get(db, t, k) is not None and all(tr.get(db, t, k).get(c) == v
                                                                      for c, v in a.items()) for t, k, a in out)
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("factionhits", "factions", "npcs"), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
