#!/usr/bin/env python3
"""What a kill drops, worked out from the tables (the user, 2026-10-30: "we
need to look at drop rates for items, including quest drops"). Read only.

**Loot tables** (handlers/loot.go's rollLoot): a creature's corpse rolls each
table bound to its name (npcloot: chance in percent, rolls a kill) and every
table bound to "*", each roll that lands taking one of the table's items whose
level window (loottables minlevel..maxlevel) holds the creature's level, by
weight. So an item's chance a kill of a creature of level L is

    1 - (1 - chance/100 * weight / W(L)) ^ rolls

W(L) the weights of the table's items at L, summed; the tool takes the best
creature for each item (the highest chance of any spawn's level), and says how
many kills a player farming it there needs for a one-in-two and a nine-in-ten
chance to see one (n50 = ln 0.5 / ln(1 - p), n90 = ln 0.1 / ln(1 - p)).

**Quest loot steps** (handlers/questevents.go): each kill of a step's
creatures (its target and its others, queststeptargets) credited to a player
on it drops its item for that player with the step's chance (0, or above 1:
every kill), until the step's count is taken: count / chance kills on
average. Where it is farmed is the step's camp: the spawns of its creatures
within NEAR (tools/fix_quest_targets.py's) of the one nearest the quest's
places. A corpse's creature is back a minute after it is emptied
(entity/corpse.go), so a camp of n spawns gives a lone player a kill every
KILL_MINUTES / n minutes, and no player kills faster than one every
FASTEST_KILL: the minutes are the kills times the larger (ours, a rough
measure for ranking the steps, not a measured pace).

The report: per table, its items, how many no creature it is bound to ever
drops (no bound creature of a level its window holds) and the best chances'
spread; the items quests want that nothing drops or sells; the quest loot
steps by chance, those with few spawns near and the slowest to farm. --tsv
writes every table item's best source and chance.

    tools/drop_rates.py [--db eqoa.db] [--tsv items.tsv]
"""
import argparse
import collections
import math
import os
import sqlite3
import statistics
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fix_quest_targets as fqt  # noqa: E402

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
EVERY = "*"  # the npcloot name every creature's corpse rolls (handlers/loot.go lootEveryCreature)
KILL_MINUTES = 1.5  # a spawn's kill, loot and return (ours: the corpse goes when emptied, the creature a minute after)
FASTEST_KILL = 0.5  # minutes a kill at the least: the fight and the loot (ours)
RARE = 0.002  # an item a kill drops at less than this at its best creature is all but never seen (n50 over 346 kills)


def n_for(p, want):
    """Kills for a want (0.5, 0.9) chance of one at p a kill."""
    if p <= 0:
        return math.inf
    if p >= 1:
        return 1
    return math.log(1 - want) / math.log(1 - p)


def load(db):
    tables = collections.defaultdict(list)  # table -> [(patternid, weight, lo, hi)]
    for t, pid, w, lo, hi in db.execute("SELECT tablename, patternid, weight, minlevel, maxlevel FROM loottables WHERE weight > 0"):
        tables[t].append((pid, w, lo, hi))
    binds = collections.defaultdict(list)  # npcloot name -> [(table, chance, rolls)]
    for n, t, c, r in db.execute("SELECT npc_name, tablename, chance, rolls FROM npcloot"):
        binds[n].append((t, c, r))
    levels = collections.defaultdict(collections.Counter)  # npcclass key -> {level: spawns} (attackable ones)
    for name, lvl, typ, owner in db.execute("SELECT npc_name, npc_level, npc_type, 0 FROM npcs WHERE npc_name IS NOT NULL"):
        if (typ or 0) & 0x80:
            continue
        levels[" ".join(name.split()).lower().rstrip("$").strip()][lvl or 0] += 1
    names = {pid: n for pid, n in db.execute("SELECT patternid, itemname FROM itempattern")}
    return tables, binds, levels, names


def chances(tables, binds, levels):
    """Every (item, creature, level) a kill can drop: {patternid: [(p, name, level, table, chance, rolls, spawns)]}."""
    out = collections.defaultdict(list)
    every = binds.get(EVERY, [])
    for name, lv in levels.items():
        bound = binds.get(name, []) + every
        for lvl, spawns in lv.items():
            for t, c, r in bound:
                valid = [x for x in tables.get(t, []) if x[2] <= lvl <= x[3]]
                total = sum(x[1] for x in valid)
                if not total:
                    continue
                for pid, w, _, _ in valid:
                    p = 1 - (1 - c / 100 * w / total) ** max(r, 0)
                    out[pid].append((p, name, lvl, t, c, r, spawns))
    return out


def table_report(tables, binds, best):
    bound = collections.defaultdict(list)
    for n, bl in binds.items():
        for t, c, r in bl:
            bound[t].append(n)
    rows = []
    for t in sorted(tables, key=lambda t: -len(tables[t])):
        items = {pid for pid, *_ in tables[t]}
        seen = [best[(pid, t)][0] for pid in items if (pid, t) in best]
        never = len(items) - len(seen)
        ps = sorted(p for p in seen)
        med = statistics.median(ps) if ps else 0
        rows.append((t, len(items), len(bound[t]), never, med, sum(1 for p in ps if p < RARE)))
    return rows


def quest_loot(db):
    """Every loot step's farm: (quest, step, chance, count, names, its camp's
    spawns, kills, minutes, spawned by an event)."""
    w = fqt.World(db)
    quests = fqt.load_quests(db)
    out = []
    for q in quests.values():
        where = fqt.places(w, q)
        for st in q["steps"]:
            if st["kind"] != "loot":
                continue
            names = fqt.step_names(st)
            items = fqt.parse_items(st["items"])
            if not items:
                continue
            need = st["count"] if (st["count"] or 0) >= 1 else max(items[0][1], 1)
            c = st["chance"] if 0 < (st["chance"] or 0) <= 1 else 1.0
            spawned = any(k in fqt.spawned_by_events(q, st) for k in names)
            sp = [s for k in names for s in w.spawns.get(k, []) if s[5]]
            camp = 0
            if sp:
                anchor = min(sp, key=lambda s: fqt.nearest(s, where)) if where else sp[0]
                camp = sum(1 for s in sp if s[1] == anchor[1] and math.hypot(s[2] - anchor[2], s[3] - anchor[3]) <= fqt.NEAR)
            kills = need / c
            minutes = None if spawned or not camp else kills * max(KILL_MINUTES / camp, FASTEST_KILL)
            out.append((q, st, c, need, names, camp, kills, minutes, spawned))
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--tsv", help="write every table item's best creature and chance here")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(f"file:{a.db}?mode=ro", uri=True)
    tables, binds, levels, names = load(db)
    every = chances(tables, binds, levels)
    # each item's best creature in each table it is in: {(patternid, table): (p, name, level, table, chance, rolls, spawns)}
    best = {}
    for pid, lst in every.items():
        for x in lst:
            k = (pid, x[3])
            if k not in best or x[0] > best[k][0]:
                best[k] = x
    print("loot tables (a table: items, names bound, items no bound creature ever drops, median best chance a kill, "
          f"items under {RARE:.1%} at their best):")
    for t, n, nb, never, med, rare in table_report(tables, binds, best):
        print(f"  {t}: {n} items, {nb} names bound, {never} never dropped, median {med:.3%} a kill "
              f"(n50 {n_for(med, 0.5):.0f} kills), {rare} under {RARE:.1%}")
    # what a kill of a bound creature brings, by table binding
    print("bindings (npcloot): " + ", ".join(f"{k} {v}" for k, v in collections.Counter(
        f"{t} {c}%x{r}" for bl in binds.values() for t, c, r in bl).most_common()))
    own = [n for n in levels if n in binds]
    print(f"creatures with a table of their own: {len(own)} of {len(levels)} names that can be attacked "
          f"({sum(sum(levels[n].values()) for n in own)} of {sum(sum(c.values()) for c in levels.values())} spawns); "
          f"every other kill rolls only the tables of \"*\": " +
          ", ".join(f"{t} {c}% x{r}" for t, c, r in binds.get(EVERY, [])))
    # the quest loot steps
    steps = quest_loot(db)
    on = [x for x in steps if x[0]["enabled"]]
    by_chance = collections.Counter(round(x[2], 2) for x in on)
    print(f"quest loot steps in the game: {len(on)}; by chance: " +
          ", ".join(f"{c:g} {n}" for c, n in sorted(by_chance.items())))
    kills = [x[6] for x in on]
    print(f"  kills a step needs on average: median {statistics.median(kills):.1f}, 90th "
          f"{sorted(kills)[int(len(kills) * 0.9)]:.1f}, most {max(kills):.0f}")
    camps = collections.Counter(min(x[5], 5) for x in on if not x[8])
    print("  their camps (spawns of the step's creatures round the one nearest the quest; an event's spawn left out): " +
          ", ".join(f"{'5 or more' if k == 5 else k}: {n}" for k, n in sorted(camps.items())))
    mins = sorted(x[7] for x in on if x[7] is not None)
    print(f"  minutes alone: median {statistics.median(mins):.1f}, 90th {mins[int(len(mins) * 0.9)]:.1f}, most {mins[-1]:.0f}")
    slow = sorted((x for x in on if x[7] is not None), key=lambda x: -x[7])[:25]
    print(f"  the slowest to farm alone (a camp of n gives a kill every {KILL_MINUTES:g}/n minutes, one every "
          f"{FASTEST_KILL:g} at the most): quest, step, chance x count, creatures, camp, kills, minutes")
    for q, st, c, need, nm, camp, k, minutes, _ in slow:
        print(f"    {q['questid']} {q['name']!r} L{q['minlevel']} step {st['step']}: {c:g} x {need}, "
              f"{', '.join(nm)}, camp {camp}, {k:.0f} kills, {minutes:.0f} min")
    if a.tsv:
        with open(a.tsv, "w") as f:
            f.write("patternid\titem\ttable\tbest_chance\tn50\tn90\tcreature\tlevel\tspawns\tbinding\n")
            for t in sorted(tables):
                for pid, w, lo, hi in sorted(tables[t]):
                    b = best.get((pid, t))
                    if b:
                        p, name, lvl, tt, c, r, spawns = b
                        f.write(f"{pid}\t{names.get(pid, '')}\t{t}\t{p:.5f}\t{n_for(p, 0.5):.0f}\t{n_for(p, 0.9):.0f}\t"
                                f"{name}\t{lvl}\t{spawns}\t{c}% x{r}\n")
                    else:
                        f.write(f"{pid}\t{names.get(pid, '')}\t{t}\t0\tinf\tinf\t\t\t\tno bound creature of levels {lo}-{hi}\n")
        print(f"items: {a.tsv}")


if __name__ == "__main__":
    main()
