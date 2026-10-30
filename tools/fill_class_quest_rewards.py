#!/usr/bin/env python3
"""The city class quests' missing rewards, filled from the same class's quest of the same level in
the other cities (the user, 2026-10-30: Dust finished Neriak's shadow knight "The Fire Bats" and got
no reward, "should be harm touch scroll"; then, of every class quest: "Fill all class quests").

EQOA-Data's guides (tools/quests/data/, the 1-20 city files) name no reward but the experience for
many class quests in some cities, where the same class's quest of the same level in the other cities
names a spell (Malice at 3, Harm Touch at 5 for a shadow knight) or an item. Those spells are QUEST
spells (levels 3, 5, 7, 13, 15 and the level-20 quest's two paths: EQOA-Data's class lists, and no
Sandstorm vendor sells them), so a city whose quest names none leaves its players without the spell
(the user: "3, 5 etc. are quest spells not purchasable"). A quest whose steps give no spell and whose
last step gives no item is given, on its last step:

  - the spell the other cities' quests of its class and level teach, AS ITS SCROLL (the guides say
    "& a Hardening Agent scroll"; the engine teaches a step's spells outright, a scroll is scribed as
    a merchant's is: handlers/scribe.go). Where the cities disagree, the most of them; a vote counts
    only for a spell of the class's own (classspells), so Freeport's shadow knight "Heavy Arms" (an
    enchanter's spell) does not outvote Highbourne's Harm Touch (the user's word for that one too).
    A scroll is one handlers/scribe.go scribes into the class's spell: its art, not worn, its name the
    spell's (spellKey: "Sneak Scroll" names no spell), the class in its mask, the races that start in
    that city as the class in its mask, its level not above the quest's, and the record scribe.go picks
    the class's own or its twin (one spell numbered twice, the same level). A spell with no such
    scroll, a Combat Mastery (never a scroll) and a tie are reported, not filled;
  - at level 20, the guides' choice of the two paths (Bellow or Pillar of Might ...: L20_PATHS), both
    as scrolls, the step's choice (the 30/40/49 path quests' form); each path's weapon is its own
    city's and is left out. A level-20 quest that kept one path is left as it is (reported);
  - the item the other cities' quests give, as it is, when most of the cities naming a reward give an
    item and the most of those give the same record; not when the quest's own guide names an item
    this server lacks ("Ironsun Morningstar": proposed to make, as the New Gear items were), when the
    item was made for another city's quest (its line names that city's smith), or when the quest's
    title names another kind ("Ghelwin's Staff" is not given a robe). --no-items leaves items out.

A quest with a reward of its own is left as it is (Dust's Fire Bats: the Harm Touch scroll since
2026-10-30). Each filled quest's notes say the reward is ours and where from. A character of the
quest's class who finished it (charquests done) is given the scroll or item the way a quest gives one
(remainHP the record's maxhp, the inventory's next place) unless it holds the item or, for a scroll,
has the spell in its book; a full inventory (the client's 40 records) and a level-20 choice (the
player picks) are reported, not given.

Every row written or changed is logged in class_quest_fill_log; --undo puts back only what is still
as this left it. One transaction. A report by default (--tsv FILE writes the review list);
--write-data DIR names the same rewards in the quest files there (text, the files' own layout), so a
re-import writes what --apply does; --coverage FILE --eqoa-data DIR [--vendors TSV] lists every
class's quest spells, the quests that give each one and where it is still unobtainable. The quests
load at the server's start: a restart after --apply.

    tools/fill_class_quest_rewards.py [--db eqoa.db] [--apply | --undo] [--no-items] [--tsv FILE]
                                      [--write-data tools/quests/data]
                                      [--coverage FILE --eqoa-data ../external/EQOA-Data [--vendors TSV]]
"""

import argparse
import collections
import csv
import json
import os
import re
import sqlite3
import sys

CLASSES = ["warrior", "ranger", "paladin", "shadowknight", "monk", "bard", "rogue", "druid", "shaman",
           "cleric", "magician", "necromancer", "enchanter", "wizard", "alchemist"]
SCROLL_ICON = -607649328              # handlers/scribe.go's scrollIcon
INV_CAP = 40                          # handlers/itemlist.go's invListCap
VENDOR_LEVELS = {1, 4, 8, 12, 16, 20, 24, 29, 34, 39, 44, 49}  # Sandstorm's spell vendors, every class
QUEST_SPELL_LEVELS = {3, 5, 7, 13, 15, 20}                     # the city class quests' spells
KEY = re.compile(r"^1-20/([^/]+)/([^/]+)/L(\d+)$")
CLASS_TAG = re.compile(r"\s*\((?:[A-Za-z]{2,3}(?:\s*[,/]\s*)?)+\)\s*$")
CM_SOURCE = re.compile(r"\b(cm|epic|master|mca)\b")
STAMP = "tools/fill_class_quest_rewards.py, 2026-10-30"

# the user's word where the cities disagree: (class, level) -> the spell (its spellKey)
DECISIONS = {
    (3, 5): "harm touch",   # "should be harm touch scroll" (Freeport's guide says Heavy Arms, an enchanter's)
}

# the level-20 quest's two paths, by class: every city's guide offers a choice of two spells (each
# with a weapon of its own city's), e.g. Freeport's warrior "Choose between: Bellow ... Pillar of
# Might"; tools/import_quests.py kept one path where it kept any. Both are quest spells (EQOA-Data's
# class lists; no Sandstorm vendor sells any of the thirty), so a level-20 quest with no reward is
# given the choice of the two scrolls (the weapons differ by city and are left out).
# The shaman's pet path is Spiritual Guide (Oggok's guide; Grobb's names Feral Spirit, which vendors sell).
L20_PATHS = {
    0: ("bellow", "pillar of might"), 1: ("sparrow eye", "strikeblade"), 2: ("fortify", "shining bastion"),
    3: ("bloodwasp", "shadow tower"), 4: ("stone stance", "stasis strike"), 5: ("power dance", "sweeping combat"),
    6: ("avoidance", "minor wound"), 7: ("enkindle", "creeping health"), 8: ("decaying wind", "spiritual guide"),
    9: ("disease ward", "field dress"), 10: ("frozen mark", "lava stone"), 11: ("blood gale", "power gale"),
    12: ("spacious mind", "power boon"), 13: ("blazing clash", "mental focus"), 14: ("student's knack", "klik'anon cocktail"),
}
CHOICE_ASK, CHOICE_SAYS = "Which do you take?", "Then it is yours."  # the path quests' (30-40-49.json)
NPC_KINDS = {"talk", "show", "give"}

LOG = "class_quest_fill_log"
LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl TEXT NOT NULL,
    rid TEXT NOT NULL,
    col TEXT NOT NULL,
    old TEXT,
    new TEXT
)"""


def spell_key(name):
    """handlers/scribe.go's spellKey: the name a scroll and its spell share."""
    n = (name or "").strip()
    if n.endswith("$"):
        n = n[:-1].strip()
    n = CLASS_TAG.sub("", n)
    return n.replace("+", "").replace("`", "'").strip().lower()


def fold(s):
    """tools/import_quests.py's fold: how a quest file's item name is matched."""
    s = str(s or "").lower().replace("’", "'").replace("`", "'").replace("'", "")
    return " ".join(re.sub(r"[^\w\s-]", " ", s).split())


def ids(s):
    """A rewards column's ids ("8554, 706000*2"), in order."""
    out = []
    for part in str(s or "").replace(" ", "").split(","):
        m = re.match(r"(\d+)(?:\*(\d+))?$", part)
        if m:
            out.append(int(m.group(1)))
    return out


def has_table(db, t):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone()[0] > 0


class Records:
    """What the database holds: spells, the classes' own, items and scrolls, by name."""

    def __init__(self, db):
        self.spell = {sid: (nm, lv) for sid, nm, lv in db.execute("SELECT spellid, spellname, abilitylvl FROM spellpattern")}
        self.by_key = collections.defaultdict(list)  # spell key -> spell ids, in id order (SpellNames' order)
        for sid in sorted(self.spell):
            self.by_key[spell_key(self.spell[sid][0])].append(sid)
        self.own = collections.defaultdict(lambda: collections.defaultdict(list))  # class -> key -> [(id, source)]
        for c, sid, src in db.execute("SELECT class, spellid, source FROM classspells ORDER BY class, spellid"):
            if sid in self.spell:
                self.own[c][spell_key(self.spell[sid][0])].append((sid, src))
        self.item = {}
        for pid, nm, icon, slot, lv, cu, ru, mhp, desc in db.execute(
                "SELECT patternid, itemname, itemicon, equipslot, levelreq, classuse, raceuse, maxhp, itemdesc FROM itempattern"):
            self.item[pid] = dict(name=nm, scroll=icon == SCROLL_ICON and slot == -1, level=lv, classes=cu, races=ru,
                                  maxhp=mhp or 0, slot=slot, desc=desc or "")
        self.by_fold = collections.defaultdict(list)  # import_quests.py's item lookup
        for pid in sorted(self.item):
            self.by_fold[fold(self.item[pid]["name"])].append(pid)
        # the races that can be each class (a scroll's race mask names them: handlers/scribe.go)
        self.races = collections.defaultdict(int)
        if has_table(db, "defaultcharacter"):
            for c, r in db.execute("SELECT DISTINCT tclass, race FROM defaultcharacter"):
                if 0 <= r < 10:
                    self.races[c] |= 1 << r
        self.sold = collections.defaultdict(set)
        if has_table(db, "npcitem"):
            for pid, nm in db.execute("SELECT n.patternid, c.npc_name FROM npcitem n JOIN npcs c ON c.id = n.npcid"):
                self.sold[pid].add(nm)

    def spell_of_scroll(self, pid):
        """The spell handlers/scribe.go scribes from this scroll (scrollSpell): the first of its
        name at the scroll's level, else the first of its name; None for an item no spell names."""
        it = self.item[pid]
        if not it["scroll"]:
            return None
        cands = self.by_key.get(spell_key(it["name"])) or []
        for sid in cands:
            if self.spell[sid][1] == it["level"]:
                return sid
        return cands[0] if cands else None

    def is_own(self, key, c):
        return bool(self.own[c].get(key))

    def kind(self, key, c):
        """A class's spell, by kind: a Combat Mastery (its rows all a mastery's, an epic's or a
        master class's), else ordinary."""
        rows = self.own[c].get(key) or []
        return "CM" if rows and all(CM_SOURCE.search(src) for _, src in rows) else "ordinary"

    def city_races(self, db, quests):
        """(city, class) -> the race mask of the characters that start there as that class: each
        defaultcharacter spot is a few steps from its class's first quest giver (fix_start_spots.py),
        so a spot's city is the one whose level-1 giver of that class stands nearest."""
        self.at = {}
        if not has_table(db, "defaultcharacter"):
            return
        givers = collections.defaultdict(list)  # class -> [(city, world, x, z)]
        for q in quests.values():
            if q["level"] != 1:
                continue
            for w, x, z in db.execute("SELECT world, x_coord, z_coord FROM npcs WHERE npc_name = ?", (q["giver"],)):
                givers[q["c"]].append((q["city"], w, x, z))
        for c, r, w, x, z in db.execute("SELECT tclass, race, world, x, z FROM defaultcharacter"):
            near = [(abs(x - gx) + abs(z - gz), city) for city, gw, gx, gz in givers.get(c, []) if gw == w]
            if near and 0 <= r < 10:
                city = min(near)[1]
                self.at[(city, c)] = self.at.get((city, c), 0) | 1 << r

    def scrolls_for(self, key, c, level, city=None):
        """Every scroll of the spell a class-c character could scribe into the class's own spell,
        lowest id first, with why each other one is not (for the report)."""
        good, bad = [], []
        for pid in sorted(self.item):
            it = self.item[pid]
            if not it["scroll"] or spell_key(it["name"]) != key:
                continue
            teaches = self.spell_of_scroll(pid)
            why = []
            if not it["classes"] >> c & 1:
                why.append("another class's")
            own_levels = {self.spell[sid][1] for sid, _ in self.own[c].get(key, [])}
            if teaches is None or self.spell[teaches][1] not in own_levels:
                # scribe.go takes the first record of the name at the scroll's level: one of the
                # class's own, or its twin (one spell numbered twice, Return Home's and Sandstorm's)
                why.append("scribes a spell not the class's")
            if it["level"] > level:
                why.append(f"level {it['level']}")
            need = getattr(self, "at", {}).get((city, c)) or self.races.get(c) or 1023
            if it["races"] & need != need:
                why.append(f"races {it['races']} (the class's are {need})")
            (bad if why else good).append((pid, why))
        return [p for p, _ in good], bad

    def file_name(self, pid):
        """The name a quest file gives the item so tools/import_quests.py resolves it to pid, or None."""
        nm = self.item[pid]["name"]
        k = fold(nm)
        got = self.by_fold.get(k) or (self.by_fold.get(k[:-1]) if k.endswith("s") else None) or self.by_fold.get(k + "s")
        return nm if got and got[0] == pid else None


def tag(level, rec, key, c, quest_level):
    """quest spell, vendor spell or ability/CM (the user's three, 2026-10-30)."""
    if rec.kind(key, c) == "CM":
        return "ability/CM"
    sid = (rec.own[c].get(key) or [(None, "")])[0][0]
    lv = rec.spell[sid][1] if sid in rec.spell else level
    if lv == quest_level and quest_level in QUEST_SPELL_LEVELS:
        return "quest spell"
    if lv in VENDOR_LEVELS:
        return "vendor spell"
    return "ability/CM"


def load(db, rec):
    """The city class quests (tools/import_quests.py's keys 1-20/<city>/<class>/L<n>) and what each
    gives a player of its class."""
    out = {}
    for key, qid, fname in db.execute(
            "SELECT k.key, k.questid, k.file FROM quest_import_quests k JOIN quests q ON q.questid = k.questid"):
        m = KEY.match(key)
        if not m or m.group(2) not in CLASSES:
            continue
        c = CLASSES.index(m.group(2))
        name, notes, giver = db.execute("SELECT name, notes, giver FROM quests WHERE questid = ?", (qid,)).fetchone()
        steps = [dict(zip(("step", "kind", "spells", "rewarditems", "byclass", "choices"), r)) for r in db.execute(
            "SELECT step, kind, spells, rewarditems, byclass, choices FROM queststeps WHERE questid = ? ORDER BY step", (qid,))]
        if not steps:
            continue
        spells, items = [], []  # [(key, spell id or scroll id, how, step)], the last step's other items
        for st in steps:
            sp = [(s, "") for s in ids(st["spells"])]
            it = [(p, "") for p in ids(st["rewarditems"])]
            if st["byclass"]:
                e = json.loads(st["byclass"]).get(str(c)) or {}
                sp += [(s, " by class") for s in ids(e.get("spells"))]
                it += [(p, " by class") for p in ids(e.get("items"))]
            if st["choices"]:
                for o in json.loads(st["choices"]).get("options") or []:
                    sp += [(s, " a choice") for s in ids(o.get("spells"))]
                    it += [(p, " a choice") for p in ids(o.get("items"))]
            for s, how in sp:
                if s in rec.spell:
                    spells.append((spell_key(rec.spell[s][0]), s, "spell" + how, st["step"]))
            for p, how in it:
                if p not in rec.item:
                    continue
                if rec.spell_of_scroll(p) is not None:
                    spells.append((spell_key(rec.item[p]["name"]), p, "scroll" + how, st["step"]))
                elif st is steps[-1]:
                    items.append(p)
        out[qid] = dict(qid=qid, key=key, file=fname, city=m.group(1), c=c, level=int(m.group(3)), name=name, giver=giver,
                        notes=notes or "", last=steps[-1], spells=spells, items=items)
    return out


GUIDE_ITEM = [re.compile(r'left out of the rewards: the item "([^"]+)"'),
              re.compile(r"Reward left out \(not in the database\): item ([^.;]+?)\.(?:\s|$)")]


def guide_item(notes):
    """The item reward the quest's guide names that the database lacked at its import, as its notes
    say (tools/import_quests.py's words), or None."""
    for rx in GUIDE_ITEM:
        m = rx.search(notes or "")
        if m:
            return m.group(1).strip()
    return None


WEAPON_SLOTS = {12, 14, 15, 16}
TITLE_WEAPON = re.compile(r"(?i)\b(staff|weapon|sword|longsword|mace|mallet|hammer|shank|dirk|blade|spear|club|dagger|axe|foil)\b")
TITLE_GARMENT = re.compile(r"(?i)\b(robe|gloves|bracers|leggings|tunic|boots|vest|cap|pants|belt)\b")
MADE_FOR_A_QUEST = re.compile(r"^(Forged|Sewn) by ")  # tools/seed_newgear_rewards.py's items, named for their quests


def item_misfit(rec, q, pid):
    """Why another city's item does not fit this quest, or None: one of the items made for another
    city's quest (named for it, its line naming that city's smith), or a kind the quest's own title
    contradicts ("Ghelwin's Staff" given a robe)."""
    it = rec.item[pid]
    if MADE_FOR_A_QUEST.match(it.get("desc") or ""):
        return f"{it['name']!r} was made for another city's quest ({it['desc']})"
    weapon = it.get("slot") in WEAPON_SLOTS
    if TITLE_WEAPON.search(q["name"] or "") and not weapon:
        return f"the quest's title names a weapon and {it['name']!r} is not one"
    if TITLE_GARMENT.search(q["name"] or "") and weapon:
        return f"the quest's title names a garment and {it['name']!r} is a weapon"
    return None


def majority(counter):
    """(the winner, its count, tied) of a Counter."""
    if not counter:
        return None, 0, False
    top = counter.most_common()
    return top[0][0], top[0][1], len(top) > 1 and top[1][1] == top[0][1]


def plan(db, items=True):
    rec = Records(db)
    quests = load(db, rec)
    rec.city_races(db, quests)
    groups = collections.defaultdict(list)
    for q in quests.values():
        groups[(q["c"], q["level"])].append(q)
    fills, reports = [], []
    for (c, lv), qs in sorted(groups.items()):
        for q in sorted(qs, key=lambda x: x["city"]):
            others = [o for o in qs if o is not q]
            own = {k for k, *_ in q["spells"] if rec.is_own(k, c)}
            foreign = sorted({k for k, *_ in q["spells"] if not rec.is_own(k, c)})
            base = dict(qid=q["qid"], key=q["key"], file=q["file"], city=q["city"], cls=CLASSES[c], c=c, level=lv,
                        name=q["name"])
            if foreign:
                reports.append(dict(base, what="another class's spell",
                                    detail=f"teaches {', '.join(foreign)}, no spell of the class's own"
                                    + (f" (and {', '.join(sorted(own))})" if own else "") + ": left as it is"))
            if q["spells"]:
                continue  # it teaches a spell: its reward is set
            if lv == 20 and c in L20_PATHS and not q["items"]:
                # the guides' choice of two paths: both scrolls, or the one there is (below)
                paths = []
                for k in L20_PATHS[c]:
                    good, bad = rec.scrolls_for(k, c, lv, q["city"]) if rec.is_own(k, c) else ([], [])
                    paths.append((k, good[0] if good else None))
                if all(p for _, p in paths) and q["last"]["kind"] in NPC_KINDS and not q["last"]["choices"]:
                    choice = []
                    for k, pid in paths:
                        sid = rec.own[c][k][0][0]
                        choice.append(dict(spell=k, spell_id=sid, spell_name=rec.spell[sid][0], spell_level=rec.spell[sid][1],
                                           scroll=pid, scroll_name=rec.item[pid]["name"], scroll_file_name=rec.file_name(pid),
                                           tag=tag(rec.spell[sid][1], rec, k, c, lv)))
                    fill = dict(base, notes_before=q["notes"], last_step=q["last"]["step"], rewards_before=q["last"]["rewarditems"],
                                choices_before=q["last"]["choices"] or "", spell=None, scroll=None, item=None, choice=choice,
                                problems=[f"the quest file cannot name scroll {o['scroll']} {o['scroll_name']!r}"
                                          for o in choice if o["scroll_file_name"] is None],
                                votes="", disagreements="", item_votes="")
                    fill["owed"] = owed(db, rec, q, fill)
                    fill["note"] = note(fill)
                    fills.append(fill)
                    continue
                missing = [k for k, p in paths if not p]
                if missing:
                    reports.append(dict(base, what="no scroll (level-20 path)",
                                        detail=f"the guides' level-20 path{'s' if len(missing) > 1 else ''} "
                                        f"{' and '.join(missing)} {'have' if len(missing) > 1 else 'has'} no scroll the class "
                                        "can scribe: " + ("no choice; the other path as the other cities give it, if any"
                                                          if len(missing) == 1 else "not filled")))
            # the other cities' quests that name a reward: their own spells, their items
            namers = [o for o in others if any(rec.is_own(k, c) for k, *_ in o["spells"]) or o["items"]]
            if not namers:
                continue
            votes = collections.Counter()
            spell_cities = collections.defaultdict(list)
            for o in namers:
                for k in sorted({k for k, *_ in o["spells"] if rec.is_own(k, c)}):
                    votes[k] += 1
                    spell_cities[k].append(o["city"])
            fill = dict(base, notes_before=q["notes"], last_step=q["last"]["step"], rewards_before=q["last"]["rewarditems"],
                        spell=None, scroll=None, item=None, problems=[])
            # the spell
            k, n, tied = majority(votes)
            decided = DECISIONS.get((c, lv))
            if decided and decided in votes:
                k, tied = decided, False
            disagree = sorted(f"{kk} ({', '.join(spell_cities[kk])})" for kk in votes if kk != k)
            # what the other cities teach that is not a spell of the class's (a guide's slip): said, never counted
            for o in others:
                for kk in sorted({kk for kk, *_ in o["spells"] if not rec.is_own(kk, c)}):
                    disagree.append(f"{kk} ({o['city']}, not a spell of the class's)")
            fill["votes"] = "; ".join(f"{kk}: {', '.join(spell_cities[kk])}" for kk, _ in votes.most_common())
            fill["disagreements"] = "; ".join(disagree)
            if k and tied:
                reports.append(dict(base, what="tie", detail=f"the other cities teach {fill['votes']}: not filled"))
                k = None
            if k:
                sid = rec.own[c][k][0][0]
                t = tag(rec.spell[sid][1], rec, k, c, lv)
                fill.update(spell=k, spell_id=sid, spell_name=rec.spell[sid][0], spell_level=rec.spell[sid][1], tag=t,
                            spell_from=spell_cities[k], decided=bool(decided))
                good, bad = rec.scrolls_for(k, c, lv, q["city"])
                if t == "ability/CM" and rec.kind(k, c) == "CM":
                    reports.append(dict(base, what="a Combat Mastery", detail=f"{rec.spell[sid][0]} is a Combat Mastery "
                                        "of the class (never a scroll): not filled"))
                elif not good:
                    why = "; ".join(f"{p} {rec.item[p]['name']!r}: {', '.join(w)}" for p, w in bad) or "none of its name"
                    teach = ", ".join(sorted({str(s) for o in namers for kk, s, how, _ in o["spells"] if kk == k and how.startswith("spell")}))
                    reports.append(dict(base, what="no scroll", spell=rec.spell[sid][0], tag=t,
                                        detail=f"{rec.spell[sid][0]} (level {rec.spell[sid][1]}, {t}) has no scroll the "
                                        f"class can scribe ({why}); the other cities teach it outright (spell {teach or sid}): "
                                        "proposed, not filled"))
                else:
                    pid = good[0]
                    fill.update(scroll=pid, scroll_name=rec.item[pid]["name"], scroll_file_name=rec.file_name(pid),
                                sold_by=sorted(rec.sold.get(pid, ())))
                    if fill["scroll_file_name"] is None:
                        fill["problems"].append(f"the quest file cannot name scroll {pid} {rec.item[pid]['name']!r}: "
                                                "another item of its name comes first")
            # the item
            named = guide_item(q["notes"])
            if items and not q["items"] and named:
                others_items = sorted({rec.item[p]["name"] for o in namers for p in o["items"]})
                reports.append(dict(base, what="the guide's item", detail=f"the guide names {named!r}, not on this server: "
                                    f"no other city's item given in its place (the others give {', '.join(others_items) or 'none'}); "
                                    "proposed: make it, as the New Gear items were made"))
            elif items and not q["items"]:
                with_item = [o for o in namers if o["items"]]
                ic = collections.Counter(tuple(o["items"]) for o in with_item)
                pids, n, tied = majority(ic)
                fill["item_votes"] = "; ".join(f"{', '.join(rec.item[p]['name'] for p in kk)}: "
                                               f"{', '.join(o['city'] for o in with_item if tuple(o['items']) == kk)}"
                                               for kk, _ in ic.most_common())
                if with_item and 2 * len(with_item) > len(namers):
                    if tied:
                        reports.append(dict(base, what="tie (item)", detail=f"the other cities give {fill['item_votes']}: no item"))
                    elif any(item_misfit(rec, q, p) for p in pids):
                        reports.append(dict(base, what="item misfit", detail="; ".join(filter(None, (item_misfit(rec, q, p) for p in pids)))
                                            + f" (the others give {fill['item_votes']}): no item"))
                    else:
                        fill.update(item=list(pids), item_names=[rec.item[p]["name"] for p in pids],
                                    item_file_names=[rec.file_name(p) for p in pids],
                                    item_from=[o["city"] for o in with_item if tuple(o["items"]) == pids])
                        for p in pids:
                            if rec.file_name(p) is None:
                                fill["problems"].append(f"the quest file cannot name item {p} {rec.item[p]['name']!r}")
            if fill["scroll"] is None and fill["item"] is None:
                continue
            fill["owed"] = owed(db, rec, q, fill)
            fill["note"] = note(fill)
            fills.append(fill)
    return rec, fills, reports


def owed(db, rec, q, fill):
    """The characters of the quest's class who finished it and lack what it now gives:
    [(serverid, name, [patternid], full)]."""
    out = []
    give = ([fill["scroll"]] if fill["scroll"] else []) + (fill["item"] or [])
    if fill.get("choice"):
        give = [o["scroll"] for o in fill["choice"]]
    for sid, name, cls in db.execute(
            "SELECT c.serverid, h.charName, h.tclass FROM charquests c JOIN characters h ON h.serverid = c.serverid "
            "WHERE c.questid = ? AND c.done = 1 ORDER BY c.serverid", (q["qid"],)):
        if cls != q["c"]:
            continue
        mine = []
        for pid in give:
            if db.execute("SELECT 1 FROM charinventory WHERE serverid = ? AND patternid = ?", (sid, pid)).fetchone():
                continue
            key = fill["spell"] if pid == fill["scroll"] else next((o["spell"] for o in fill.get("choice") or [] if o["scroll"] == pid), None)
            if key:
                book = {spell_key(n) for (n,) in db.execute(
                    "SELECT p.spellname FROM spells s JOIN spellpattern p ON p.spellid = s.spellid WHERE s.serverid = ?", (sid,))}
                if key in book:
                    continue
            mine.append(pid)
        if mine:
            held = db.execute("SELECT count(*) FROM charinventory WHERE serverid = ? AND location = 0", (sid,)).fetchone()[0]
            out.append((sid, name, mine, held + len(mine) > INV_CAP))
    return out


def note(f):
    """The sentence a filled quest's notes gain: the reward is ours, and where from."""
    parts = []
    if f.get("choice"):
        names = " or the ".join(o["spell_name"] for o in f["choice"])
        return (f"The reward is ours: the choice of the {names} scroll, the two level-20 paths every city's guide "
                f"offers (their weapons, each city's own, left out) ({STAMP}).")
    if f["scroll"]:
        src = " and ".join(_cities(f["spell_from"]))
        s = f"the {f['spell_name']} scroll, as {src} quest{'s' if len(f['spell_from']) > 1 else ''} of this level teach{'' if len(f['spell_from']) > 1 else 'es'} it"
        if f["disagreements"]:
            s += f" (the most of the cities; others teach {f['disagreements']})"
        if f.get("decided"):
            s += " (the user's word)"
        parts.append(s)
    if f["item"]:
        src = " and ".join(_cities(f["item_from"]))
        parts.append(f"{' and '.join(f['item_names'])}, as {src} quest{'s' if len(f['item_from']) > 1 else ''} of this level give{'' if len(f['item_from']) > 1 else 's'}")
    return f"The reward is ours: {', and '.join(parts)} ({STAMP})."


CITY = {"fayspire": "Fayspire", "freeport": "Freeport", "grobb": "Grobb", "halas": "Halas", "highbourne": "Highbourne",
        "klick-anon": "Klick'Anon", "moradhim": "Moradhim", "neriak-nektulos": "Neriak", "oggok": "Oggok",
        "qeynos": "Qeynos", "rivervale": "Rivervale", "surefall-glade": "Surefall Glade", "tethelin": "Tethelin"}


def _cities(cs):
    names = [CITY.get(c, c) + "'s" for c in cs]
    if len(names) > 2:
        return [", ".join(names[:-1]), names[-1]]
    return names


def log(db, tbl, rid, col, old, new):
    db.execute(f"INSERT INTO {LOG} (tbl, rid, col, old, new) VALUES (?, ?, ?, ?, ?)",
               (tbl, str(rid), col, None if old is None else json.dumps(old), None if new is None else json.dumps(new)))


def choices_column(choice):
    """The choices column for a path choice, as tools/import_quests.py's step_choices writes one."""
    opts = [dict(answer=o["spell_name"], says=CHOICE_SAYS, items=str(o["scroll"])) for o in choice]
    return json.dumps(dict(ask=CHOICE_ASK, options=opts), separators=(",", ":"))


def apply(db, fills):
    db.execute(LOG_DDL)
    n = collections.Counter()
    for f in fills:
        cur, cur_choices = db.execute("SELECT rewarditems, choices FROM queststeps WHERE questid = ? AND step = ?",
                                      (f["qid"], f["last_step"])).fetchone()
        if cur != f["rewards_before"] or (f.get("choice") and (cur_choices or "") != f["choices_before"]):
            raise SystemExit(f"quest {f['qid']} step {f['last_step']} changed under the plan")
        if f.get("choice"):
            new = choices_column(f["choice"])
            db.execute("UPDATE queststeps SET choices = ? WHERE questid = ? AND step = ?", (new, f["qid"], f["last_step"]))
            log(db, "queststeps", f"{f['qid']}/{f['last_step']}", "choices", cur_choices or "", new)
        else:
            add = ([f["scroll"]] if f["scroll"] else []) + (f["item"] or [])
            new = ", ".join([cur] + [str(p) for p in add]) if cur else ", ".join(str(p) for p in add)
            db.execute("UPDATE queststeps SET rewarditems = ? WHERE questid = ? AND step = ?", (new, f["qid"], f["last_step"]))
            log(db, "queststeps", f"{f['qid']}/{f['last_step']}", "rewarditems", cur, new)
        n["rewards"] += 1
        old_notes = db.execute("SELECT notes FROM quests WHERE questid = ?", (f["qid"],)).fetchone()[0]
        notes = (old_notes + " " if old_notes else "") + f["note"]
        db.execute("UPDATE quests SET notes = ? WHERE questid = ?", (notes, f["qid"]))
        log(db, "quests", f["qid"], "notes", old_notes, notes)
        for sid, _, pids, full in f["owed"]:
            if f.get("choice"):
                n["choice"] += 1  # the player picks: a GM gives the one they choose
                continue
            if full:
                n["full"] += 1
                continue
            for pid in pids:
                cur = db.execute(
                    "INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, "
                    "listnumber) VALUES (?, 1, (SELECT COALESCE(maxhp, 0) FROM itempattern WHERE patternid = ?), 0, ?, -1, 0, "
                    "(SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = 0))",
                    (sid, pid, pid, sid))
                log(db, "charinventory", cur.lastrowid, "*", None, {"serverid": sid, "patternid": pid, "questid": f["qid"]})
                n["given"] += 1
    return n


def undo(db):
    if not has_table(db, LOG):
        print("nothing logged: nothing to undo")
        return
    back = kept = 0
    for seq, tbl, rid, col, old, new in db.execute(f"SELECT seq, tbl, rid, col, old, new FROM {LOG} ORDER BY seq DESC").fetchall():
        old = json.loads(old) if old is not None else None
        new = json.loads(new) if new is not None else None
        if tbl == "queststeps" and col in ("rewarditems", "choices"):
            q, st = rid.split("/")
            cur = db.execute(f"UPDATE queststeps SET {col} = ? WHERE questid = ? AND step = ? AND {col} = ?",
                             (old, int(q), int(st), new))
        elif tbl == "quests":
            cur = db.execute("UPDATE quests SET notes = ? WHERE questid = ? AND notes = ?", (old, int(rid), new))
        elif tbl == "charinventory":
            cur = db.execute("DELETE FROM charinventory WHERE itemid = ? AND serverid = ? AND patternid = ?",
                             (int(rid), new["serverid"], new["patternid"]))
        else:
            continue
        back, kept = (back + 1, kept) if cur.rowcount else (back, kept + 1)
    db.execute(f"DROP TABLE {LOG}")
    print(f"put back {back}; left {kept} changed since")


# --- the quest files

def _span(text, key):
    """(start, end, indent) of the quest object whose "key" is key, in a file json.dump(indent=1) wrote."""
    m = re.search(r'\n( *)\{\n\1 "key": ' + re.escape(json.dumps(key)) + r",\n", text)
    if not m:
        return None
    start, ind = m.start() + 1, m.group(1)
    end = text.index("\n" + ind + "}", m.end()) + 1 + len(ind) + 1
    return start, end, ind


def _dump(obj, ind):
    lines = json.dumps(obj, indent=1).split("\n")
    return "\n".join([ind + lines[0]] + [ind + l for l in lines[1:]])


def write_data(datadir, fills):
    """Name each fill's reward on its quest's last step in its city file, and its note in the quest's
    notes: the quest's own text alone rewritten, every other byte as it was."""
    by_file = collections.defaultdict(list)
    for f in fills:
        by_file[f["file"]].append(f)
    changed = 0
    for fname, fs in sorted(by_file.items()):
        path = os.path.join(datadir, fname)
        with open(path, encoding="utf-8") as fh:
            text = fh.read()
        for f in fs:
            sp = _span(text, f["key"])
            if sp is None:
                raise SystemExit(f"{fname}: no quest {f['key']!r}")
            s, e, ind = sp
            q = json.loads(text[s:e])
            if _dump(q, ind) != text[s:e]:
                raise SystemExit(f"{fname}: {f['key']!r} is not laid out as json.dump(indent=1) writes it: left")
            last = q["steps"][-1]
            if f.get("choice"):
                if last.get("choices"):
                    raise SystemExit(f"{fname}: {f['key']!r}: its last step asks a choice already")
                if any(o["scroll_file_name"] is None for o in f["choice"]):
                    raise SystemExit(f"{fname}: {f['key']!r}: a reward the file cannot name")
                last["choose"] = CHOICE_ASK
                last["choices"] = [dict(answer=o["spell_name"], says=CHOICE_SAYS, rewards=dict(items=[o["scroll_file_name"]]))
                                   for o in f["choice"]]
            else:
                names = last.setdefault("rewards", {}).setdefault("items", [])
                for nm in ([f["scroll_file_name"]] if f["scroll"] else []) + (f["item_file_names"] if f["item"] else []):
                    if nm is None:
                        raise SystemExit(f"{fname}: {f['key']!r}: a reward the file cannot name")
                    if nm not in names:
                        names.append(nm)
            if f["note"] not in (q.get("notes") or ""):
                q["notes"] = ((q.get("notes") or "") + " " + f["note"]).strip()
            text = text[:s] + _dump(q, ind) + text[e:]
            changed += 1
        json.loads(text)  # still a whole file
        with open(path, "w", encoding="utf-8") as out:
            out.write(text)
    return changed


# --- the report

TSV_COLS = ["questid", "key", "city", "class", "level", "quest", "reward", "patternid", "tag", "spell", "spell_level",
            "from", "votes", "disagreements", "item_votes", "sold_by", "given_to", "problems"]


def rows(fills, reports):
    out = []
    for f in fills:
        reward = []
        if f.get("choice"):
            reward.append("a choice: " + " or ".join(f"scroll {o['scroll_name']}" for o in f["choice"]))
        if f["scroll"]:
            reward.append(f"scroll {f['scroll_name']}")
        if f["item"]:
            reward += f["item_names"]
        ch = f.get("choice") or []
        out.append(dict(questid=f["qid"], key=f["key"], city=f["city"], **{"class": f["cls"]}, level=f["level"], quest=f["name"],
                        reward="; ".join(reward),
                        patternid=", ".join(str(p) for p in ([o["scroll"] for o in ch] or [f["scroll"]] if f["scroll"] or ch else [])
                                            + (f["item"] or [])),
                        tag=" / ".join(o["tag"] for o in ch) if ch else f.get("tag", "") if f["scroll"] else "item",
                        spell=" / ".join(o["spell_name"] for o in ch) if ch else f.get("spell_name", "") if f["scroll"] else "",
                        spell_level=" / ".join(str(o["spell_level"]) for o in ch) if ch else f.get("spell_level", "") if f["scroll"] else "",
                        **{"from": "; ".join(filter(None, [", ".join(f.get("spell_from") or []) if f["scroll"] else "",
                                                           ("item: " + ", ".join(f["item_from"])) if f["item"] else ""]))},
                        votes=f.get("votes", ""), disagreements=f.get("disagreements", ""), item_votes=f.get("item_votes", ""),
                        sold_by=", ".join(f.get("sold_by") or []),
                        given_to=", ".join(f"{n} ({s}){' (a choice: not given, a GM gives the one they pick)' if ch else ' INVENTORY FULL: not given' if full else ''}"
                                           for s, n, _, full in f["owed"]),
                        problems="; ".join(f["problems"])))
    for r in reports:
        out.append(dict(questid=r["qid"], key=r["key"], city=r["city"], **{"class": r["cls"]}, level=r["level"], quest=r["name"],
                        reward=f"NOT FILLED: {r['what']}", patternid="", tag=r.get("tag", ""), spell=r.get("spell", ""),
                        spell_level="", **{"from": ""}, votes="", disagreements="", item_votes="", sold_by="", given_to="",
                        problems=r["detail"]))
    return out


# --- every class's quest spells, and who gives them (--coverage: a report alone)

QUEST_LEVELS = {3, 5, 7, 13, 15, 17, 20, 23, 25, 27, 30, 33, 35, 37, 40, 43, 45, 47, 48, 49, 57}
CITY_LEVELS = {3, 5, 7, 13, 15, 20}  # the city class quests'


def quest_spells(eqd, vendors):
    """class -> [(name, level, how)]: EQOA-Data's class lists' Quest spells; a list with no Quest
    source (the shaman's) its spells at a quest level that Sandstorm's vendors do not sell and no
    mastery adds."""
    out = collections.defaultdict(list)
    cm = collections.defaultdict(set)
    for f in os.listdir(os.path.join(eqd, "cms")):
        with open(os.path.join(eqd, "cms", f), encoding="utf-8") as fh:
            d = json.load(fh)

        def walk(x):
            if isinstance(x, dict):
                for a in (x.get("effects") or {}).get("addAbilities") or []:
                    yield a.get("name") or ""
                for v in x.values():
                    yield from walk(v)
            elif isinstance(x, list):
                for v in x:
                    yield from walk(v)
        cm[(d.get("class") or "").lower().replace(" ", "")].update(spell_key(n) for n in walk(d))
    for c, cn in enumerate(CLASSES):
        with open(os.path.join(eqd, "spells", f"{cn}.json"), encoding="utf-8") as fh:
            d = json.load(fh)
        lst = [(a["name"], lv["level"], a.get("source")) for lv in d["list"] for a in lv["abilities"]]
        if any(s == "Quest" for _, _, s in lst):
            out[c] = [(n, lv, "EQOA-Data: Quest") for n, lv, s in lst if s == "Quest"]
        else:
            out[c] = [(n, lv, "inferred: a quest level, no vendor sells it, no mastery adds it") for n, lv, s in lst
                      if lv in QUEST_LEVELS and spell_key(n) not in vendors.get(c, set()) and spell_key(n) not in cm[cn]]
    return out


def all_gives(db, rec):
    """(class, spell key) -> [(questid, enabled, how)] over EVERY quest: steps' spells, by class,
    choices, and scrolls among their items (rewards, by class, choices, give events)."""
    keys = dict(db.execute("SELECT questid, key FROM quest_import_quests")) if has_table(db, "quest_import_quests") else {}
    quests = {qid: (en, [int(x) for x in str(cl).replace(" ", "").split(",") if x.isdigit()], name, keys.get(qid, ""))
              for qid, en, cl, name in db.execute("SELECT questid, enabled, classes, name FROM quests")}
    gives = collections.defaultdict(list)
    for qid, step, sp, ri, bc, ch, ev in db.execute(
            "SELECT questid, step, spells, rewarditems, byclass, choices, events FROM queststeps ORDER BY questid, step"):
        if qid not in quests:
            continue
        en, allowed, _, _ = quests[qid]
        allowed = allowed or list(range(15))

        def add(classes, key, how):
            for c in classes:
                gives[(c, key)].append((qid, en, how))

        def spells(v, classes, how):
            for s in ids(v):
                if s in rec.spell:
                    add(classes, spell_key(rec.spell[s][0]), how)

        def scrolls(v, classes, how):
            for p in ids(v):
                if p in rec.item and rec.spell_of_scroll(p) is not None:
                    add([c for c in classes if rec.item[p]["classes"] >> c & 1], spell_key(rec.item[p]["name"]), f"scroll {p}{how}")

        spells(sp, allowed, "")
        scrolls(ri, allowed, "")
        for ck, e in (json.loads(bc) if bc else {}).items():
            spells(e.get("spells"), [int(ck)], " by class")
            scrolls(e.get("items"), [int(ck)], " by class")
        for o in (json.loads(ch).get("options") or []) if ch else []:
            spells(o.get("spells"), allowed, " a choice")
            scrolls(o.get("items"), allowed, " a choice")
        for e in json.loads(ev) if ev else []:
            if e.get("do") == "give":
                scrolls(e.get("items"), allowed, " an event")
    return gives, quests


def where(quests, qid):
    k = quests[qid][3]
    m = KEY.match(k)
    return f"{m.group(1)} L{m.group(3)}" if m else (k or f"quest {qid}")


def coverage(db, rec, fills, eqd, vendors, guides=None):
    """The rows of --coverage: per class, each quest spell, its level, the quests that give it, and
    its status in each city (for the city quests' levels) before and after the fills."""
    qs = quest_spells(eqd, vendors)
    gives, quests = all_gives(db, rec)
    chains = collections.defaultdict(set)  # class -> cities with its chain
    for k, in db.execute("SELECT key FROM quest_import_quests"):
        m = KEY.match(k)
        if m and m.group(2) in CLASSES:
            chains[CLASSES.index(m.group(2))].add(m.group(1))
    filled = collections.defaultdict(set)  # (class, key) -> cities a fill gives its scroll in
    for f in fills:
        if f["scroll"]:
            filled[(f["c"], f["spell"])].add(f["city"])
        for o in f.get("choice") or []:
            filled[(f["c"], o["spell"])].add(f["city"])
    texts = {}
    if guides:
        for root, _, files in os.walk(guides):
            for fn in files:
                if fn.endswith(".json") and not fn.startswith("_"):
                    p = os.path.join(root, fn)
                    with open(p, encoding="utf-8") as fh:
                        texts[os.path.relpath(p, guides)] = fh.read()
    out = []
    for c in range(15):
        for name, lv, how in sorted(qs[c], key=lambda x: (x[1], x[0])):
            k = spell_key(name)
            own = rec.own[c].get(k) or []
            alias = ALIASES.get((c, k))
            g = gives.get((c, k), []) + (gives.get((c, alias), []) if alias else [])
            enabled = [x for x in g if x[1]]
            sc, _ = rec.scrolls_for(k, c, lv)
            if alias and not sc:
                sc, _ = rec.scrolls_for(alias, c, lv)
            sold = sorted({n for p in sc for n in rec.sold.get(p, ())})
            row = dict(cls=CLASSES[c], level=lv, spell=name, how=how,
                       our_spell=", ".join(f"{sid} {rec.spell[sid][0]} (level {rec.spell[sid][1]})" for sid, _ in
                                           own + (rec.own[c].get(alias) or [] if alias else [])) or "none",
                       scroll=", ".join(f"{p} {rec.item[p]['name']}" for p in sc) or "none",
                       sold_by=", ".join(sold),
                       given_by="; ".join(f"{where(quests, q)} #{q}{'' if en else ' (a draft)'} ({h})" for q, en, h in g),
                       named_here=f"also as {quests_name(alias, rec, c)}" if alias else "")
            if lv in CITY_LEVELS and chains[c]:
                cities = {}
                for city in sorted(chains[c]):
                    mine = [x for x in enabled if KEY.match(quests[x[0]][3] or "") and KEY.match(quests[x[0]][3]).group(1) == city]
                    cities[city] = "obtainable" if mine else ("filled by the tool" if city in filled[(c, k)] | filled[(c, alias)]
                                                              else "UNOBTAINABLE")
                row["before"] = ", ".join(f"{ct}: {'obtainable' if st == 'obtainable' else 'UNOBTAINABLE'}" for ct, st in cities.items())
                row["after"] = ", ".join(f"{ct}: {st}" for ct, st in cities.items())
                bad = [ct for ct, st in cities.items() if st == "UNOBTAINABLE"]
                fil = [ct for ct, st in cities.items() if st == "filled by the tool"]
                row["status"] = ("still UNOBTAINABLE in " + ", ".join(bad) if bad else
                                 "filled by the tool in " + ", ".join(fil) if fil else "obtainable in every city")
            else:
                row["before"] = row["after"] = ""
                row["status"] = "obtainable" if enabled else ("only a draft gives it" if g else "UNOBTAINABLE: no quest gives it")
            found = []
            if not enabled and texts:
                rx = re.compile(r"(?<![\w'])" + re.escape(name) + r"(?![\w'])", re.I)
                found = sorted(p for p, t in texts.items() if rx.search(t))
            row["guides_naming_it"] = ", ".join(found)
            out.append(row)
    return out


def quests_name(key, rec, c):
    sid = (rec.own[c].get(key) or [(None, "")])[0][0]
    return f"{rec.spell[sid][0]} (spell {sid}, level {rec.spell[sid][1]})" if sid else key


# one spell under two names: EQOA-Data's (the key) and a record of ours (the alias)
ALIASES = {
    (9, "blessing"): "minor blessing",                # the dump's Minor Blessing (236): CHA, level 3, what the guides say
    (13, "arcane bindings"): "arcane bindings-wiz",   # the dump's wizard record (1629, level 3) of the root 767 (level 4)
}


COVERAGE_COLS = ["cls", "level", "spell", "how", "our_spell", "scroll", "sold_by", "given_by", "named_here", "before",
                 "after", "status", "guides_naming_it"]


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default="eqoa.db")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    ap.add_argument("--no-items", action="store_true", help="fill the spells alone")
    ap.add_argument("--tsv", help="write the review list here")
    ap.add_argument("--write-data", metavar="DIR", help="name the rewards in the quest files of DIR")
    ap.add_argument("--coverage", metavar="FILE", help="write every class's quest spells and who gives them here (needs --eqoa-data)")
    ap.add_argument("--eqoa-data", metavar="DIR", help="EQOA-Data's checkout (its Information/databases and Quests)")
    ap.add_argument("--vendors", metavar="TSV", help="Sandstorm's vendor scrolls (harvests/.../scrolls_by_class.tsv): "
                    "a class list with no Quest source (the shaman's) is read without them")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 10000")
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        try:
            undo(db)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        return
    if a.apply and has_table(db, LOG) and db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]:
        print(f"{LOG} holds an earlier apply: --undo it first, or this adds to it")
    _, fills, reports = plan(db, items=not a.no_items)
    for f in fills:
        what = []
        if f.get("choice"):
            what.append("a choice: " + " or ".join(f"scroll {o['scroll']} {o['scroll_name']!r} ({o['tag']})" for o in f["choice"])
                        + " (the guides' level-20 paths)")
        if f["scroll"]:
            what.append(f"scroll {f['scroll']} {f['scroll_name']!r} ({f['tag']}, level {f['spell_level']}; from "
                        f"{', '.join(f['spell_from'])}{'; others: ' + f['disagreements'] if f['disagreements'] else ''})")
        if f["item"]:
            what.append(f"item {', '.join(map(str, f['item']))} {' + '.join(f['item_names'])} (from {', '.join(f['item_from'])})")
        given = ", ".join(f"{n} ({s}){' FULL' if full else ''}" for s, n, _, full in f["owed"])
        print(f"{f['cls']} L{f['level']} {f['city']} {f['qid']} {f['name']!r}: {'; '.join(what)}"
              + (f"; {'owed a pick (not given)' if f.get('choice') else 'given to'} {given}" if given else ""))
        for p in f["problems"]:
            print("  PROBLEM:", p)
    for r in reports:
        print(f"NOT FILLED ({r['what']}): {r['cls']} L{r['level']} {r['city']} {r['qid']} {r['name']!r}: {r['detail']}")
    n_owed = sum(len(f["owed"]) for f in fills)
    print(f"{len(fills)} quests to fill ({sum(1 for f in fills if f['scroll'])} scrolls, {sum(1 for f in fills if f.get('choice'))} "
          "level-20 choices, "
          f"{sum(1 for f in fills if f['item'])} items), {n_owed} characters owed, {len(reports)} reported")
    if a.tsv:
        with open(a.tsv, "w", newline="") as out:
            w = csv.DictWriter(out, fieldnames=TSV_COLS, delimiter="\t", lineterminator="\n")
            w.writeheader()
            w.writerows(rows(fills, reports))
    if a.write_data:
        print(f"named in the quest files: {write_data(a.write_data, fills)}")
    if a.coverage:
        if not a.eqoa_data:
            sys.exit("--coverage needs --eqoa-data")
        vendors = collections.defaultdict(set)
        if a.vendors:
            abbr = ["WAR", "RAN", "PAL", "SK", "MNK", "BRD", "RGE", "DRD", "SHA", "CL", "MAG", "NEC", "ENC", "WIZ", "ALC"]
            with open(a.vendors, newline="") as fh:
                for r in csv.DictReader(fh, delimiter="\t"):
                    vendors[abbr.index(r["class"])].add(spell_key(r["name"]))
        rec = Records(db)
        cov = coverage(db, rec, fills, os.path.join(a.eqoa_data, "Information", "databases"), vendors,
                       os.path.join(a.eqoa_data, "Quests"))
        with open(a.coverage, "w", newline="") as out:
            w = csv.DictWriter(out, fieldnames=COVERAGE_COLS, delimiter="\t", lineterminator="\n")
            w.writeheader()
            w.writerows(cov)
        st = collections.Counter(("UNOBTAINABLE" if "UNOBTAINABLE" in r["status"] else
                                  "filled" if r["status"].startswith("filled") else "obtainable") for r in cov)
        print(f"quest spells: {len(cov)} ({st['obtainable']} obtainable, {st['filled']} filled by the tool, "
              f"{st['UNOBTAINABLE']} unobtainable somewhere)")
    if not a.apply:
        print("report only: --apply to write")
        return
    problems = [p for f in fills for p in f["problems"]]
    if problems:
        sys.exit("refused: the problems above first")
    db.execute("BEGIN IMMEDIATE")
    try:
        n = apply(db, fills)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"wrote {n['rewards']} rewards, {n['given']} given ({n['full']} not: a full inventory; {n['choice']} owed a "
          "level-20 pick, not given); "
          "restart the server (quests load at its start)")


if __name__ == "__main__":
    main()
