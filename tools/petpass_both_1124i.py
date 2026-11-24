#!/usr/bin/env python3
"""Heirloom petition 2822 (Wanuven, 2026-11-24, Quest), both shards: "I completed Mistmoore bit and Tserrina kill.
It has me flagged up to collect were parts. The quest giver mentioned gathering were parts, but after doing so, he
doesn't offer anything."

The trials branch of the were-hunter line (EQOA-Data Quests/Weres/werehunter.json, "Quest Part II - Hunt + Gather
for Faction") is, on this server: A Token of Dedication (101389, the insignia; Djivan's offer ends "Then the hunt for
the beasts' parts begins"), then A Set of Tails (101392, repeatable, +50 Gypsies of Tunaria a set, offered at
Gypsies 1851..1999), then The Were-Hunter's Ring (101388, offered at Gypsies 2000), then Elder Tchekof (101381) and
the trials (101377-101380).

The ring required A Set of Tails done once, but a character already at Gypsies 2000 when the insignia is given is
never offered A Set of Tails (its range stops at 1999): Djivan offers nothing, the line stops (the log: "standing
with Gypsies of Tunaria (2000) keeps "A Set of Tails" from Djivan Mertshak"). The guide asks only the standing:
"you need to increase it to +2000 ... The reward is a ring". So the ring now requires the insignia (101389) and,
as before, Gypsies 2000; the tails remain the way to climb from 1851 to 2000. tools/quests/data/weres.json says the
same (trials-ring requires trials-insignia), so a re-import keeps it.

Refused, nothing written, when the row is not as expected (a GM's edit since). Report only by default; --apply in
one transaction, a gm_audit line, log petpass_both_1124i_log; --undo exact. The quests are read at a server start
or a panel quest change (a no-op update_row reloads them at once).

    python3 -E -s tools/petpass_both_1124i.py --db eqoa.db [--apply | --undo]

Applied 2026-11-24: Live by --apply (board #57 there; read at its next restart); Heirloom through the GM panel's
update_row with the same change (audit 162068, board #2098), live at once, since a no-op update_row reloads nothing.
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_both_1124i_log"
DETAIL = ("Heirloom petition 2822: The Were-Hunter's Ring (101388) requires the insignia (101389), not A Set of "
          "Tails, which is not offered at Gypsies 2000 (tools/petpass_both_1124i.py)")
RING, SET, INSIGNIA = 101388, 101392, 101389


def changes(db):
    out, lines, refusals = [], [], []
    want = {RING: ("The Were-Hunter's Ring", "Gypsies of Tunaria:2000"),
            SET: ("A Set of Tails", "Gypsies of Tunaria:1851..1999"),
            INSIGNIA: ("A Token of Dedication", "")}
    for qid, (name, freq) in want.items():
        q = tr.get(db, "quests", {"questid": qid})
        if q is None or q["name"] != name or q["giver"] != "Djivan Mertshak" or q["factionreq"] != freq:
            got = q and (q["name"], q["giver"], q["factionreq"])
            refusals.append(f"quest {qid} is not {name!r} from Djivan Mertshak with factionreq {freq!r}: {got}")
    if not refusals:
        q = tr.get(db, "quests", {"questid": RING})
        if q["requires"] not in (str(SET), str(INSIGNIA)):
            refusals.append(f"quest {RING} requires {q['requires']!r}, not {SET} (a GM's edit since?)")
        else:
            out.append(("quests", {"questid": RING}, dict(q, requires=str(INSIGNIA))))
            lines.append(f"quests {RING} The Were-Hunter's Ring requires {q['requires']} -> {INSIGNIA} "
                         "(factionreq Gypsies of Tunaria:2000 kept)")
        stuck = tr.rows(db, """SELECT c.charName AS n FROM charquests i JOIN characters c ON c.serverid = i.serverid
            JOIN charfactions f ON f.serverid = i.serverid AND f.faction = 'Gypsies of Tunaria'
            WHERE i.questid = ? AND i.done > 0 AND f.standing >= 2000
              AND NOT EXISTS (SELECT 1 FROM charquests r WHERE r.serverid = i.serverid AND r.questid = ?)""",
                        (INSIGNIA, RING))
        lines.append(f"note: {len(stuck)} characters past the insignia at Gypsies 2000 with no ring quest: "
                     + (", ".join(r["n"] for r in stuck) or "none"))
    if refusals:
        out = []
    applied = bool(out) and all(tr.get(db, t, k) == a for t, k, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("quests", "charquests", "charfactions"), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
