#!/usr/bin/env python3
"""Creatures summon their own pets (the owner, 2026-11-25: "any mob with a pet should summon their own pets so dont
need any pets placed", and "change their class if needed to the proper shaman, mage, etc.").

Until now a creature's pet was a row of npcs of its own, paired with its owner at the spawns by distance
(entity/companion.go: a pet's name naming its owner within 40, else the nearest creature within 4). Placement drift
broke the pairs (Heirloom petition 2977, 2026-11-25: two Kindred Spirits 4.2 and 20 off their Freezeblood mystic,
never paired). Now a creature's pets are table npcpets's, by the OWNER's name (as database.NPCClassKey keys it): the
server makes them beside it at its spawn and at every respawn, pairs them with it, and takes them away with it
(entity/creaturepets.go). This tool moves the placed pets there:

  1. pairs every placed pet row with its owner as companion.go does today (a pet's name naming its owner within 40,
     else the nearest creature within 4), then each pet still alone with the nearest creature within 25 (STRAY_REACH:
     the drift of petition 2977), refused as ambiguous when a creature of another name stands within 1 of as near;
  2. per owner name: the pets its creatures had (the commonest set of pet names among its creatures with any, and how
     many of each), and one of those pet rows kept as the copy every such pet is made from (the commonest look: name,
     body, size, level relative to its owner's), held out of the world (npcspawnonly "#row", spawn-only by its row);
     an owner name is taken only when at least MIN_SHARE of its creatures had a pet or its name is a caster's
     (CASTER_WORDS): a goblin that stood within 4 of a stray minion is no caster (those pets stay rows, reported);
  3. writes the npcpets rows and deletes every other placed pet row of a taken owner, with its npcrespawn, npcwander,
     campspawns, npcrare, npcrarecycle, npcstands and "#row" rule rows; a row something else names (a path, a portal,
     a quest step, a door, a wall, a sign, a title, a merchant's item, any other "#row" rule) is refused, nothing
     written;
  4. gives each taken owner name the class of the pets it summons (npcclass): shaman for spirits, necromancer for
     minions and the dead, magician for elementals and familiars (PET_CLASS), and the 'creature caster' class
     template (npctemplate; class -1, its own class, the 'dungeon trash' template's spells for its class without its
     melee raise or crits): a name with another template than 'roamer' (or none) keeps it and is said.

Pets with no owner within 25 (or an owner name not taken) stay rows, paired as before (companion.go still reads
rows), and are listed. Report only by default; --apply in one transaction, a gm_audit line a row, log
creature_pets_1125_log; --undo exact (a row changed since is left and said). The server reads npcpets, npcclass and
npctemplate at its start and at a GM panel edit of them; the rows deleted go at the next start (a keep restart
reads rows too). Needs the server with entity/creaturepets.go (an older one would leave those creatures with no pets).

    python3 -E -s tools/creature_pets_20261125.py --db eqoa.db [--apply | --undo]
"""
import collections
import json
import math
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "creature_pets_1125_log"
DETAIL = "Creatures summon their own pets (the owner, 2026-11-25): tools/creature_pets_20261125.py"
SOURCE = "creaturepets-1125"

REACH = 4.0          # entity/companion.go's companionReach
NAMED_REACH = 40.0   # companionNamedReach
STRAY_REACH = 25.0   # a pet still alone pairs with the nearest creature this near (placement drift)
STRAY_ABOVE = 5      # a stray's owner is at most this many levels below it
AMBIGUOUS = 1.0      # another name's creature this much nearer at most: refused as ambiguous
MIN_SHARE = 0.5      # an owner name is taken when this share of its creatures had a pet (or its name is a caster's)

# entity/companion.go's rules, as they stand (companionNotPets, companionWords, companionOwnerName, companionKinds,
# companionOwnerOnly, companionPetKinds)
NOT_PETS = {"blood thirsty minion", "village pet"}
WORDS = re.compile(r"(?i)(^|[\s'])(pet|minion|familiar|thrall|companion spirit|primal spirit)$")
OWNER_NAME = re.compile(r"(?i)^(?:(.+)'s (?:pet|minion|familiar|servant|thrall)|(?:pet|minion|familiar|servant|thrall) of (.+))$")
KINDS = {"mummified guardian": "necromancer", "elder spirit": "shaman"}
OWNER_ONLY = {"elder spirit"}
PET_KINDS = {"frenzied spirit", "vigilant spirit", "feral spirit", "howling death", "ancient death", "kindred spirit",
             "fire elemental", "fire elementaling", "fire elementalkin", "decaying flesh", "wilting flesh",
             "swarming bones"}

# the words of a caster's name: an owner name with one is taken whatever share of its creatures had a pet
CASTER_WORDS = re.compile(r"\b(shaman|mystic|oracle|augur|prophet|seer|spiritist|spiritmaster|witch ?doctor|"
                          r"necromancer|bonecaster|mage|magician|summoner|conjurer|elementalist|hierophant|embalmer|"
                          r"skullseer|ispusar|priest|priestess|cleric|wizard|sorcerer|sorceress|warlock|channeler)\b")

SHAMAN, MAGICIAN, NECROMANCER = 8, 10, 11
CLASS_NAMES = {0: "warrior", 1: "ranger", 2: "paladin", 3: "shadowknight", 4: "monk", 5: "bard", 6: "rogue",
               7: "druid", 8: "shaman", 9: "cleric", 10: "magician", 11: "necromancer", 12: "enchanter",
               13: "wizard", 14: "alchemist"}
TEMPLATE = "creature caster"
REPLACEABLE_TEMPLATES = ("roamer",)

# the tables a placed pet row's own rules live in, deleted with it: (table, its row column)
ALONG = (("npcrespawn", "npc_row"), ("npcwander", "npcid"), ("campspawns", "npcid"), ("npcrare", "npc_row"),
         ("npcrarecycle", "npc_row"), ("npcstands", "npc_row"))
# the tables whose naming a pet row refuses its deletion: (table, column)
REFUSE = (("npcpaths", "npcid"), ("gmportals", "npc_id"), ("gmportals", "exit_npc_id"), ("questdoors", "npcid"),
          ("questwalls", "npcid"), ("queststeps", "npcid"), ("signtext", "npcid"), ("npctitle", "npcid"),
          ("npcitem", "npcid"), ("npc_mirror_inbox", "npc_row"))


def key_of(name):
    """database.NPCClassKey."""
    return (name or "").strip().lower().rstrip("$").strip()


def pet_class(pet_key):
    """The class whose pets a pet of that name is: a spirit a shaman's, an elemental or a familiar a magician's, a
    minion or one of the dead a necromancer's; None for a servant (a named creature's own, whose class it keeps)."""
    if re.search(r"servant", pet_key):
        return None
    if re.search(r"spirit", pet_key):
        return SHAMAN
    if re.search(r"elemental|familiar", pet_key):
        return MAGICIAN
    return NECROMANCER  # minions, the dead, flesh, bones, guardians, thralls, a "<name>'s pet"


def pet_rule(name):
    """(pet, named, owner_only) as companionPetRuleUncachedLocked (no triggers': their pets are spawn-only)."""
    k = key_of(name)
    if k in NOT_PETS:
        return False, "", False
    if not (k in KINDS or k in PET_KINDS or WORDS.search((name or "").strip()) or OWNER_NAME.match((name or "").strip())):
        return False, "", False
    m = OWNER_NAME.match((name or "").strip())
    named = (m.group(1) or m.group(2)).strip() if m else KINDS.get(k, "")
    return True, named, k in OWNER_ONLY


def fold(s):
    return " ".join(re.findall(r"[^\W_]+", (s or "").lower()))


def names_owner(named, owner):
    n, o = fold(named), fold(owner)
    return bool(n and o) and (n == o or o.startswith(n + " ") or o.endswith(" " + n))


def score(pet, o, named, owner_only, reach):
    d = math.hypot(pet["x_coord"] - o["x_coord"], pet["z_coord"] - o["z_coord"])
    if named and d <= NAMED_REACH and names_owner(named, o["npc_name"]):
        return d - NAMED_REACH - REACH, d
    if d <= reach and not owner_only:
        return d, d
    return None, d


def spawn_only(db):
    """(names, rows): npcspawnonly's names (keys) and its "#n" rows, less npcstands's rows for a name."""
    names, rows = set(), set()
    if tr.table(db, "npcspawnonly"):
        for (n,) in db.execute("SELECT name FROM npcspawnonly"):
            n = (n or "").strip()
            if re.fullmatch(r"#\d+", n):
                rows.add(int(n[1:]))
            else:
                names.add(key_of(n))
    stands = {r for (r,) in db.execute("SELECT npc_row FROM npcstands")} if tr.table(db, "npcstands") else set()
    return names, rows, stands


def fighters(db):
    """Every npcs row that takes part in a pair (companionFighter's, for a row): no NPC bit, no dummy, not spawn-only."""
    names, rows, stands = spawn_only(db)
    out = []
    for r in tr.rows(db, "SELECT * FROM npcs ORDER BY id"):
        k = key_of(r["npc_name"])
        if (r["npc_type"] or 0) & 0x80 or k.startswith(("training dummy", "regular dummy", "sparring dummy")):
            continue
        if r["id"] in rows or (k in names and r["id"] not in stands):
            continue
        out.append(r)
    return out




# what a pet looks like (its row's columns but its place, level and own life's): the pets of one look are made from
# one row of it
NOT_LOOK = {"id", "zone", "unk_a", "unk_b", "x_coord", "y_coord", "z_coord", "facing", "world", "dt_kill", "hp",
            "hp_percent", "npc_level", "npc_id"}


def look(r):
    return tuple(sorted((k, v) for k, v in r.items() if k not in NOT_LOOK))


def dist(a, b):
    return math.hypot(a["x_coord"] - b["x_coord"], a["z_coord"] - b["z_coord"])


class Grid:
    def __init__(self, rows, cell=NAMED_REACH):
        self.cell = cell
        self.m = collections.defaultdict(list)
        for r in rows:
            self.m[self.at(r)].append(r)

    def at(self, r):
        return r["world"], math.floor(r["x_coord"] / self.cell), math.floor(r["z_coord"] / self.cell)

    def near(self, r):
        w, cx, cz = self.at(r)
        for dx in (-1, 0, 1):
            for dz in (-1, 0, 1):
                yield from self.m.get((w, cx + dx, cz + dz), ())


def pair(rows):
    """The pairs read off the rows (the file's doc, step 1): (pets, owners, owner_of, taken, ambiguous, left) with
    owner_of pet row id -> (owner row, distance, how), taken the owner names (keys) taken, ambiguous and left lists of
    (pet row, why)."""
    pets, others, rules = [], [], {}
    for r in rows:
        rule = pet_rule(r["npc_name"])
        if rule[0]:
            pets.append(r)
            rules[r["id"]] = rule
        else:
            others.append(r)
    grid = Grid(others)
    first = {}
    for p in pets:  # today's rule
        _, named, owner_only = rules[p["id"]]
        best = None
        for o in grid.near(p):
            s, d = score(p, o, named, owner_only, REACH)
            if s is not None and (best is None or (s, o["id"]) < (best[0], best[1]["id"])):
                best = (s, o, d)
        if best is not None:
            first[p["id"]] = (best[1], best[2])
    all_of = collections.Counter(key_of(r["npc_name"]) for r in others)
    with_pet = collections.defaultdict(set)
    for o, _ in first.values():
        with_pet[key_of(o["npc_name"])].add(o["id"])
    taken = {k for k, ids in with_pet.items() if len(ids) >= MIN_SHARE * all_of[k] or CASTER_WORDS.search(k)}
    owner_of, ambiguous, left = {}, [], []
    for p in pets:
        _, named, owner_only = rules[p["id"]]
        f = first.get(p["id"])
        if f is not None and key_of(f[0]["npc_name"]) in taken:
            owner_of[p["id"]] = (f[0], f[1], "today's rule")
            continue
        # the nearest creature within STRAY_REACH of a taken name or a caster's (its name's owner first)
        cands = []
        for o in grid.near(p):
            k = key_of(o["npc_name"])
            d = dist(p, o)
            if d > STRAY_REACH or not (k in taken or CASTER_WORDS.search(k)):
                continue
            if owner_only and not names_owner(named, o["npc_name"]):
                continue
            if (p["npc_level"] or 0) > (o["npc_level"] or 0) + STRAY_ABOVE:
                continue  # a creature far above it is no one's pet here (a level 43 spirit by level 9 shamans)
            cands.append((0 if named and names_owner(named, o["npc_name"]) else 1, d, o["id"], o))
        cands.sort(key=lambda c: c[:3])
        if not cands:
            if f is not None:
                left.append((p, f"paired as before with {f[0]['npc_name']} #{f[0]['id']} ({f[1]:.1f} off), a name not "
                                f"taken ({len(with_pet[key_of(f[0]['npc_name'])])} of its "
                                f"{all_of[key_of(f[0]['npc_name'])]} had a pet)"))
            else:
                left.append((p, "no creature of a caster's or a taken name within %d" % STRAY_REACH))
            continue
        c = cands[0]
        rival = [x for x in cands[1:] if x[0] == c[0] and x[1] - c[1] <= AMBIGUOUS and
                 key_of(x[3]["npc_name"]) != key_of(c[3]["npc_name"])]
        if rival:
            r = rival[0]
            ambiguous.append((p, f"{c[3]['npc_name']} #{c[2]} {c[1]:.1f} off and {r[3]['npc_name']} #{r[2]} "
                                 f"{r[1]:.1f}"))
            continue
        owner_of[p["id"]] = (c[3], c[1], "today's rule" if f is not None and f[0]["id"] == c[2] else "stray")
        taken.add(key_of(c[3]["npc_name"]))
    return pets, others, owner_of, taken, ambiguous, left


def commonest(counter):
    """The commonest of a Counter, ties to the least by sort order."""
    return sorted(counter.items(), key=lambda kv: (-kv[1], repr(kv[0])))[0][0]


def references(db, ids):
    """(along, rowrules, refused): the rows of ALONG naming each of ids ({id: [(table, key)]}), the other rule tables'
    "#row" rows naming it ({id: ["table.column"]}: its own loot, template, aggro, ...; such a pet is its owner's copy
    itself, so they hold for the pets made from it), and the reasons one may not go ({id: ["table.column"]})."""
    along, rowrules, refused = (collections.defaultdict(list) for _ in range(3))
    if not ids:
        return along, rowrules, refused
    q = ",".join("?" * len(ids))
    for tbl, col in ALONG:
        if tr.table(db, tbl) and tr.column(db, tbl, col):
            for (i,) in db.execute(f"SELECT {col} FROM {tbl} WHERE {col} IN ({q})", list(ids)):
                along[i].append((tbl, {col: i}))
    if tr.table(db, "npcspawnonly"):
        for (n,) in db.execute(f"SELECT name FROM npcspawnonly WHERE name IN ({q})", [f"#{i}" for i in ids]):
            along[int(n[1:])].append(("npcspawnonly", {"name": n}))
    for tbl, col in REFUSE:
        if tr.table(db, tbl) and tr.column(db, tbl, col):
            for (i,) in db.execute(f"SELECT {col} FROM {tbl} WHERE {col} IN ({q})", list(ids)):
                refused[i].append(f"{tbl}.{col}")
    # any other rule table's "#row" (npcloot, npctemplate, npcaggro, ...): a name column holding it
    for (tbl,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name").fetchall():
        if tbl in ("npcspawnonly", "npcpets", "npcs", "gm_audit") or tbl.endswith(("_log", "_seed", "_meta")) or \
                tbl.startswith("bak_"):
            continue
        for c in tr.rows(db, f"PRAGMA table_info({tbl})"):
            if c["name"] in ("name", "npc_name") and "TEXT" in (c["type"] or "").upper():
                for (n,) in db.execute(f"SELECT DISTINCT {c['name']} FROM {tbl} WHERE {c['name']} IN ({q})",
                                       [f"#{i}" for i in ids]):
                    rowrules[int(n[1:])].append(f"{tbl}.{c['name']}")
    return along, rowrules, refused


NPCPETS_SCHEMA = """CREATE TABLE IF NOT EXISTS npcpets (
    owner        TEXT NOT NULL,
    pet          TEXT NOT NULL,
    template_row INTEGER NOT NULL,
    count        INTEGER NOT NULL DEFAULT 1,
    level_offset INTEGER NOT NULL DEFAULT 0,
    note         TEXT NOT NULL DEFAULT '',
    source       TEXT NOT NULL DEFAULT 'gm',
    PRIMARY KEY (owner, template_row)
)"""


def keyed(db, tbl, key):
    """The row of a rule table keyed by name whose name is key as database.NPCClassKey keys it, None for none."""
    if not tr.table(db, tbl):
        return None
    r = tr.rows(db, f"SELECT * FROM {tbl} WHERE trim(rtrim(lower(trim(name)), '$')) = ? ORDER BY name", (key,))
    return r[0] if r else None


def plan(db):
    """Everything the tool would write (the file's doc): (out, lines, refusals, summary)."""
    rows = fighters(db)
    pets, others, owner_of, taken, ambiguous, left = pair(rows)
    pet_by = {p["id"]: p for p in pets}
    of_owner = collections.defaultdict(list)
    owner_row = {}
    for pid, (o, d, how) in sorted(owner_of.items()):
        of_owner[o["id"]].append(pid)
        owner_row[o["id"]] = o
    by_key = collections.defaultdict(list)
    for oid in sorted(of_owner):
        by_key[key_of(owner_row[oid]["npc_name"])].append(oid)

    def pet_set(oid):
        return tuple(sorted(key_of(pet_by[i]["npc_name"]) for i in of_owner[oid]))

    # one row of each look kept as the copy its pets are made from (one with no rule rows of its own, the least id);
    # a pet with "#row" rules of its own (its loot, its template) is its owner's copy itself, so they hold for the
    # pets made from it (entity/npcrow.go: a row's key holds for its copies)
    ids = sorted(owner_of)
    along, rowrules, refused = references(db, ids)
    by_look = collections.defaultdict(list)
    for i in ids:
        if not rowrules.get(i):
            by_look[look(pet_by[i])].append(i)
    template = {lk: sorted(m, key=lambda i: (bool(along.get(i)) or bool(refused.get(i)), i))[0]
                for lk, m in by_look.items()}

    def copy_of(i):
        return i if rowrules.get(i) else template[look(pet_by[i])]

    def entry(members, own):
        """{template row: (pet key, count, level offset)} for owner rows members (own: each pet as it is)."""
        e = {}
        for pk, n in collections.Counter(pet_set(members[0])).items():
            offs, copies = collections.Counter(), collections.Counter()
            for o in members:
                for i in of_owner[o]:
                    p = pet_by[i]
                    if key_of(p["npc_name"]) == pk:
                        offs[(p["npc_level"] or 0) - (owner_row[o]["npc_level"] or 0)] += 1
                        copies[copy_of(i)] += 1
            if own:
                for t, c in sorted(copies.items()):
                    e[t] = (pk, c, commonest(offs))
            else:
                e[commonest(copies)] = (pk, n, commonest(offs))
        return e

    # the entries: owner ("key" or "#row") -> entry; a name's are its commonest set's (its creatures with no pet of
    # rules of its own), every other creature's its own by its row
    entries, defaults = {}, {}
    for k, oids in sorted(by_key.items()):
        plain = [o for o in oids if not any(rowrules.get(i) for i in of_owner[o])]
        default = commonest(collections.Counter(pet_set(o) for o in (plain or oids)))
        defaults[k] = (default, collections.Counter(pet_set(o) for o in oids))
        mine = [o for o in plain if pet_set(o) == default]
        entries[k] = entry(mine or [o for o in oids if pet_set(o) == default], not mine)
        for o in oids:
            if o not in mine:
                entries[f"#{o}"] = entry([o], True)
    templates = {t for e in entries.values() for t in e}

    out, lines, refusals = [], [], []
    note = "the owner, 2026-11-25: creatures summon their own pets"
    for owner, e in sorted(entries.items()):
        for t, (pk, n, off) in sorted(e.items()):
            out.append(("npcpets", {"owner": owner, "template_row": t},
                        {"owner": owner, "pet": pk, "template_row": t, "count": n, "level_offset": off,
                         "note": note, "source": SOURCE}))
    for t in sorted(templates):
        out.append(("npcspawnonly", {"name": f"#{t}"},
                    {"name": f"#{t}", "note": f"creature pets' copy ({pet_by[t]['npc_name']}): npcpets", "source": SOURCE}))
        for tbl, key in along.get(t, ()):
            if tbl in ("campspawns", "npcrare", "npcrarecycle"):
                out.append((tbl, key, None))  # a camp's or a rare's spot would turn the copy into another creature
    deleted = []
    for i in ids:
        if i in templates:
            continue
        if refused.get(i):
            refusals.append(f"pet row #{i} {pet_by[i]['npc_name']} is named by {', '.join(refused[i])}")
            continue
        for tbl, key in along.get(i, ()):
            out.append((tbl, key, None))
        out.append(("npcs", {"id": i}, None))
        deleted.append(i)

    # the owners' classes and templates (step 4)
    tmpl_rows = {r["name"]: r for r in tr.rows(db, "SELECT * FROM classtemplates")}
    caster = {SHAMAN: "shaman", NECROMANCER: "necromancer"}

    def figs(t):
        r = tmpl_rows.get(t)
        if not r:
            return "hp 100% melee 100%"
        ab = ", ".join(sorted({a.get("kind", "?") for a in json.loads(r["abilities"] or "[]")})) or "no casts"
        sp = f", {r['special']} every {r['special_every']:g} s" if r["special"] and r["special_every"] else ""
        return f"hp {r['hp_pct']}% melee {r['melee_pct']}%{sp}, {ab}"

    class_lines, classes = [], {}
    for k in sorted(by_key):
        default, _ = defaults[k]
        want = commonest(collections.Counter(pet_class(pk) for pk in default))
        have = keyed(db, "npcclass", k)
        old = have["class"] if have else 0
        name = have["name"] if have else k
        th = keyed(db, "npctemplate", k)
        forced = tmpl_rows.get(th["template"], {}).get("class", -1) \
            if th and th["template"] not in REPLACEABLE_TEMPLATES else -1
        if want is None or forced not in (-1, want):
            # a servant's named owner, or a template whose class wins (a chain wizard): its class as it is
            why = "its pet a servant" if want is None else f"its template {th['template']}'s class " \
                f"{CLASS_NAMES.get(forced, forced)} wins"
            classes[k] = (old, old, th["template"] if th else "", th["template"] if th else "")
            class_lines.append(f"  {k} (x{len(by_key[k])} with pets): class {CLASS_NAMES.get(old, old)} kept ({why})")
            continue
        if old != want:
            out.append(("npcclass", {"name": name}, {"name": name, "class": want, "source": SOURCE}))
        old_t = th["template"] if th else ""
        new_t = old_t
        tname = th["name"] if th else k
        if old_t in ("",) + REPLACEABLE_TEMPLATES:
            ct = caster.get(want)
            if ct and ct in tmpl_rows:
                new_t = ct
                out.append(("npctemplate", {"name": tname},
                            {"name": tname, "template": ct, "note": note, "source": SOURCE}))
            elif th is not None:
                new_t = ""
                out.append(("npctemplate", {"name": tname}, None))  # its 'roamer' would name the warrior's class
        why = " (kept)" if new_t == old_t and old_t else ""
        classes[k] = (old, want, old_t, new_t)
        class_lines.append(f"  {k} (x{len(by_key[k])} with pets): class {CLASS_NAMES.get(old, old)} -> "
                           f"{CLASS_NAMES.get(want, want)}; template {old_t or 'none'} [{figs(old_t)}] -> "
                           f"{new_t or 'none'} [{figs(new_t)}]{why}")

    n_keys = sum(1 for o in entries if not o.startswith("#"))
    n_rows = sum(1 for o in entries if o.startswith("#"))
    n_pets = sum(len(e) for e in entries.values())
    n_along = sum(len(along.get(i, ())) for i in deleted)
    lines.append(f"placed pets: {len(pets)}; paired with a taken owner: {len(owner_of)} "
                 f"({sum(1 for v in owner_of.values() if v[2] == 'stray')} strays within {STRAY_REACH:g}); "
                 f"left as rows: {len(left) + len(ambiguous)} ({len(ambiguous)} ambiguous)")
    lines.append(f"owners: {n_keys} names ({len(owner_row)} creatures had a pet); {n_rows} creatures whose pets are "
                 f"their own (#row); npcpets rows: {n_pets}; pets' copies kept (spawn-only by row): {len(templates)} "
                 f"({sum(1 for t in templates if rowrules.get(t))} with rules of their own); pet rows deleted: "
                 f"{len(deleted)}, with {n_along} rule rows of theirs")
    lines.append("npcpets by owner name:")
    for owner, e in sorted(entries.items()):
        if owner.startswith("#"):
            continue
        var = {",".join(s): c for s, c in defaults[owner][1].items() if s != defaults[owner][0]}
        lines.append(f"  {owner}: " + ", ".join(f"{pk} x{n} (lvl {off:+d}, copy #{t})"
                                                for t, (pk, n, off) in sorted(e.items())) +
                     (f"; other sets: {var}" if var else ""))
    lines.append("npcpets by row (a creature whose pets are not its name's, or have rules of their own):")
    for owner, e in sorted(entries.items()):
        if owner.startswith("#"):
            o = owner_row[int(owner[1:])]
            lines.append(f"  {owner} {o['npc_name']} w{o['world']}: " +
                         ", ".join(f"{pk} x{n} (lvl {off:+d}, copy #{t}{' own rules' if rowrules.get(t) else ''})"
                                   for t, (pk, n, off) in sorted(e.items())))
    lines.append("classes and templates:")
    lines.extend(class_lines)
    lines.append("left as rows (paired as companion.go pairs them, if at all):")
    for p, why in sorted(left, key=lambda t: t[0]["id"]):
        lines.append(f"  #{p['id']} {p['npc_name']} w{p['world']} L{p['npc_level']}: {why}")
    for p, why in sorted(ambiguous, key=lambda t: t[0]["id"]):
        lines.append(f"  #{p['id']} {p['npc_name']} w{p['world']} L{p['npc_level']}: ambiguous, {why}")
    summary = {"owners": n_keys, "row_owners": n_rows, "pets_paired": len(owner_of), "deleted": len(deleted),
               "along": n_along, "templates": len(templates), "left": len(left), "ambiguous": len(ambiguous),
               "npcpets": n_pets, "classes": classes}
    return out, lines, refusals, summary


def changes(db):
    out, lines, refusals, _ = plan(db)
    applied = all((tr.get(db, tbl, key) is None) if after is None else
                  (tr.get(db, tbl, key) is not None and all(tr.get(db, tbl, key).get(c) == v for c, v in after.items()))
                  for tbl, key, after in out) if tr.table(db, "npcpets") else False
    return out, lines, refusals, applied


def prepare(db):
    db.execute(NPCPETS_SCHEMA)
    return []


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("npcs", "npcspawnonly", "npcclass", "npctemplate", "classtemplates"),
        "keys": {}, "changes": changes, "prepare": prepare}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
