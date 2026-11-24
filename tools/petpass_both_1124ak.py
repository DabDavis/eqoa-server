#!/usr/bin/env python3
"""Heirloom petition 2872 (Sontosegara, level 60 monk, 2026-11-24): "I finished up my 60 epic and I was only allowed to
choose one weapon when I am supposed to have 2."

The monk's level 60 epic reward ("The Monk's Epic Reward", quest 101119) ends at the assistant recruiter (step 2) with
a choice of two options, each ONE claw: The Ascended Claws of Insurrection, or The Ascended Claws of Mayhem. The claws
are a pair:
  - questmasters' 60 Epic Rewards page (id419, the owner's first source for epics) lists the good monk's weapon as one
    item, "Claws of Insurrection and Mayhem--30 AGI 150 HP 50 AR 801 DMG", written as the bard's "Blades of Lyrical
    Harmony and Rhythm" is; EQOA-Data's epics_60.json lists both claws for the monk.
  - the records are made to be worn together: Insurrection is primary (equipslot 12), Mayhem secondary (14), set so by
    Live petition 5491 (tools/pet1122_items_20261122.py: "so both are worn").
  - the other pairs already come whole in one option at the same step: the bard's Blades ("980083, 980084"), the
    ranger's Axes of Nature's Innocence and Purity, the rogue's and warrior's epic daggers/blades (*2).
So the two options become one, "The Ascended Claws of Insurrection and Mayhem", giving both Ascended claws ("Wear them
well."). Each shard's claws are found by name (Heirloom 980117/980118, Live 980157/980158); the step must hold the two
one-claw options exactly as below, else refused.

Report only by default; --apply in one transaction, a gm_audit line ([basis:era questmasters id419]), log
petpass_both_1124ak_log; --undo exact. Quests are read at the server's next start. The claw owed to anyone who
finished the quest with one is given apart (~/charmigrate/give-if-none.sh).

    python3 -E -s tools/petpass_both_1124ak.py --db eqoa.db [--apply | --undo]
"""
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_both_1124ak_log"
DETAIL = ("[basis:era questmasters id419] Heirloom petition 2872: the monk's 60 epic reward (quest 101119 step 2) gives "
          "both Ascended claws, Insurrection and Mayhem, as one option, as questmasters' \"Claws of Insurrection and "
          "Mayhem\" and the other classes' pairs (tools/petpass_both_1124ak.py)")
QUEST, STEP = 101119, 2
INS, MAY = "Ascended Claws of Insurrection", "Ascended Claws of Mayhem"
SAYS1, SAYS2 = "Wear it well.", "Wear them well."


def item(db, name):
    r = tr.rows(db, "SELECT patternid FROM itempattern WHERE itemname = ?", (name,))
    return r[0]["patternid"] if len(r) == 1 else None


def changes(db):
    out, lines, refusals = [], [], []
    ins, may = item(db, INS), item(db, MAY)
    if ins is None or may is None:
        return [], [], [f"no single {INS} / {MAY} record ({ins}, {may})"], False
    key = {"questid": QUEST, "step": STEP}
    st = tr.get(db, "queststeps", key)
    if st is None:
        return [], [], [f"no queststeps {key}"], False
    old = {"ask": "Which do you take?", "options": [
        {"answer": "The " + INS, "says": SAYS1, "items": str(ins)},
        {"answer": "The " + MAY, "says": SAYS1, "items": str(may)}]}
    new = {"ask": "Which do you take?", "options": [
        {"answer": "The Ascended Claws of Insurrection and Mayhem", "says": SAYS2, "items": f"{ins}, {may}"}]}
    have = json.loads(st["choices"] or "{}")
    if have == new:
        return [("queststeps", key, dict(st))], [], [], True
    if have != old:
        refusals.append(f"queststeps {key} choices are not the two one-claw options (a GM's edit since?): {st['choices']}")
        return [], [], refusals, False
    after = dict(st, choices=json.dumps(new, separators=(",", ":")))
    out.append(("queststeps", key, after))
    lines.append(f"queststeps {QUEST} step {STEP}: [The {INS} ({ins})] or [The {MAY} ({may})] -> "
                 f"[The Ascended Claws of Insurrection and Mayhem ({ins}, {may})]")
    lines.insert(0, "1 row to change")
    return out, lines, refusals, False


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("queststeps", "itempattern"), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, argv=sys.argv[1:], doc=__doc__))
