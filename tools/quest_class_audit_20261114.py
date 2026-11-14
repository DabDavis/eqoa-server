#!/usr/bin/env python3
"""The quests' class gates and class rewards against their EQOA-Data guides (the owner, 2026-11-14: "yes find the
gaps"). Read-only: it writes a plan for tools/quest_class_gaps_20261114.py and a report.

For every quest whose source is an EQOA-Data guide (quests.source "EQOA-Data <key> (tools/import_quests.py)", its
notes naming the guide file and section: "From EQOA-Data Quests/<file>, <section> (github..."):

  the gate   the section's (else the file's) "**Class:**" line, narrowed by the alignment the guide gives ("Good",
             "Evil only"), against quests.classes ('' is every class). A quest split by archetype (the weres' L40-tank,
             -melee, ...) is judged by the union of its file's quests of its name.
  rewards    the section's class reward list ("- **Clerics:** Helm of Radiance", "Tank: Khal Robe of War", an item
             block's "Class: WAR PAL SK") against each reward step's queststeps.byclass.

Each class the guide does not allow is legitimate. Each class it allows that our gate blocks, or that the guide names a
reward for and our step gives nothing, is real. Each class it allows that gets nothing and that the guide names no
reward for, on a step that gives other classes theirs, is guide-silent.

    tools/quest_class_audit_20261114.py --db Live=live.db --db Heirloom=heirloom.db --guides external/EQOA-Data/Quests
        [--plan tools/quest_class_gaps_20261114.json] [--report report.md]

The plan unites the shards' audits; the report lists each shard's gaps and the fix tool's picks on each (report only).
"""
import argparse
import json
import os
import re
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)

NAMES = ["WAR", "RAN", "PAL", "SK", "MNK", "BRD", "RGE", "DRD", "SHA", "CLR", "MAG", "NEC", "ENC", "WIZ", "ALC"]
ALL = set(range(15))
GOOD_ONLY = {1, 2, 7}   # ranger, paladin, druid
EVIL_ONLY = {3, 11}     # shadow knight, necromancer
ARCHETYPE = {"tank": {0, 2, 3}, "melee": {1, 4, 5, 6}, "caster": {10, 11, 12, 13, 14}, "healer": {7, 8, 9}}
WORDS = {
    "war": 0, "warrior": 0, "ran": 1, "rng": 1, "ranger": 1, "pal": 2, "paladin": 2, "sk": 3, "shadowknight": 3,
    "shadow knight": 3, "mnk": 4, "monk": 4, "brd": 5, "bard": 5, "rog": 6, "rge": 6, "rogue": 6, "drd": 7,
    "druid": 7, "sha": 8, "shm": 8, "shaman": 8, "cl": 9, "clr": 9, "cleric": 9, "mag": 10, "mage": 10,
    "magician": 10, "nec": 11, "necro": 11, "necromancer": 11, "enc": 12, "ench": 12, "enchanter": 12, "wiz": 13,
    "wizard": 13, "alc": 14, "alch": 14, "alchemist": 14,
}
SKIP_KEYS = ("ZP/", "PlaneOfSkyArmor/", "tradeskills/")  # labelled EQOA-Data but from other sources: no guide file


def fold(s):
    s = str(s or "").lower().replace("’", "'").replace("`", "'").replace("'", "")
    return " ".join(re.sub(r"[^\w\s-]", " ", s).split())


def word_classes(w):
    """A class word, plural, or archetype as class ids (None: not one)."""
    w = fold(w).replace("-", " ")
    w = re.sub(r"\s+", " ", w).strip()
    for cand in (w, w[:-1] if w.endswith("s") else None, w[:-2] if w.endswith("es") else None):
        if not cand:
            continue
        if cand in WORDS:
            return {WORDS[cand]}
        if cand in ARCHETYPE:
            return set(ARCHETYPE[cand])
    return None


def class_list(text):
    """'Drd, Nec, Wiz' / 'WAR PAL SK' / 'Wiz/Drd' / 'Monk Druid Alc' as ids, or None if any token is not a class."""
    toks = [t for t in re.split(r"[\s,/&]+|\band\b", text.strip()) if t]
    out = set()
    i = 0
    while i < len(toks):
        t = toks[i]
        if i + 1 < len(toks) and fold(t + " " + toks[i + 1]) in ("shadow knight", "shadow knights"):
            out.add(3)
            i += 2
            continue
        c = word_classes(t)
        if c is None:
            return None
        out |= c
        i += 1
    return out or None


# ---- the guides ---------------------------------------------------------------------------------------------------

def guide_text(guides, rel):
    p = os.path.join(guides, rel)
    if not os.path.exists(p):
        return None
    with open(p) as f:
        d = json.load(f)
    g = d.get("guide") if isinstance(d, dict) else None
    return g if isinstance(g, str) else None


def strip_md(s):
    s = re.sub(r"[*_`>]", "", s)
    s = re.sub(r"[^\x00-\x7f]", " ", s)  # emoji
    return " ".join(s.split())


def heading(l):
    """(level, text) of a heading line: a Markdown one, or a bold PART line (werehunter's "**PART 2 - Optional**")."""
    m = re.match(r"^(#+)\s*(.*)$", l.strip())
    if m:
        return len(m.group(1)), m.group(2)
    m = re.match(r"^\*\*\s*(PART\b[^*]*)\*\*\s*$", l.strip(), re.I)
    return (2, m.group(1)) if m else None


def section(text, label):
    """(lines of the guide's section the label names, whether one matched); the whole guide when none does."""
    lines = text.splitlines()
    if not label:
        return lines, False
    want = fold(strip_md(label.split(",")[0]))
    best = None
    for i, l in enumerate(lines):
        hd = heading(l)
        if not hd:
            continue
        h = fold(strip_md(hd[1]))
        if not h or not want:
            continue
        if want == h or want in h or (len(h) > 6 and h in want):
            best = (i, hd[0])
            break
    if best is None:
        return lines, False
    i, lvl = best
    j = i + 1
    while j < len(lines):
        hd = heading(lines[j])
        if hd and hd[0] <= lvl:
            break
        j += 1
    return lines[i:j], True


CLASS_LINE = re.compile(r"^\s*[-*]*\s*\*\*\s*Class(?:es)?\s*:?\s*\*\*\s*:?\s*(.*)$", re.I)


def class_line(sec, whole):
    """The guide's class line for a section: the section's own, else the file's first."""
    for src in (sec, whole):
        for l in src:
            m = CLASS_LINE.match(l)
            if m:
                return m.group(1).strip()
    return None


def allowed_from(line, sec, whole):
    """(class ids the guide allows, how it reads) or (None, why)."""
    if line is None:
        return None, "no Class line"
    t = strip_md(line)
    if not t:
        return None, "empty Class line"
    low = t.lower()
    head = " ".join(strip_md(l) for l in whole[:15]).lower()
    first = re.split(r"[(]", low)[0]
    if re.match(r"^(all|any)\b", first):
        allowed, how = set(ALL), "All"
    elif re.match(r"^(good|evil)\b", first):
        allowed, how = set(ALL), t
    else:
        cl = class_list(re.sub(r"\bonly\b", "", first))
        if cl is None:
            return None, f"unread Class line {t!r}"
        allowed, how = cl, t
    good = re.search(r"\bgood\b", low) is not None
    evil = re.search(r"\bevil\b", low) is not None
    if not good and not evil:
        if re.search(r"\bevil only\b", head):
            evil = True
        elif re.search(r"\bgood only\b", head):
            good = True
    if evil and not good:
        allowed -= GOOD_ONLY
        how += " (evil)"
    elif good and not evil:
        allowed -= EVIL_ONLY
        how += " (good)"
    return allowed, how


ITEM_FIELD = re.compile(r"^(equip|hp|race|races|level|lvl|lore|no trade|no rent|description|appearance|att|dmg|ac|"
                        r"str|sta|agi|dex|wis|int|cha|power|pow|hpmax|ar|pr|dr|fr|cr|lr|dur|charges|effect|cast|"
                        r"target|recast|duration|class)\b", re.I)
NOT_NAMES = {"secondary", "primary", "held", "shield", "ntl", "lore", "no trade", "range", "ranged"}


def rewards_in(sec):
    """{class id: [names]} the section's class reward lines give ([] for a class named with no item), {} if none.
    Read from the first line naming a reward: "- **Clerics:** Helm of Radiance", "Melee (Rge Brd Ran): The
    Bloodsabre", an item block's "Class: WAR PAL SK" or bare "Ran Rge Brd Sha" under its name line."""
    out = {}
    for start, l in enumerate(sec):
        if re.search(r"reward", l, re.I):
            break
    else:
        return out
    prev_names = []
    for raw in sec[start:]:
        for seg in raw.split("\t"):
            l = seg.strip()
            if not l:
                continue
            m = re.match(r"^Class\s*:\s*(.+)$", strip_md(l), re.I)
            if m and not CLASS_LINE.match(l):
                cl = class_list(m.group(1))
                if cl is not None and prev_names:
                    for c in cl:
                        out.setdefault(c, []).append(prev_names[-1])
                continue
            s = re.sub(r"^[-*\d.)\s]+", "", strip_md(l))
            toks = [t for t in re.split(r"[\s,/&]+", s) if t]
            lead = []
            for t in toks:
                c = word_classes(t)
                if c is None:
                    break
                lead.append(c)
            if len(lead) >= 2 and ":" not in s:
                if prev_names:
                    for c in set().union(*lead):
                        out.setdefault(c, []).append(prev_names[-1])
                continue
            m = re.match(r"^([A-Za-z][A-Za-z ,/&'()]{1,60}?)\s*:\s*(.*)$", s)
            if m:
                lab = m.group(1)
                inner = re.search(r"\(([^)]*)\)", lab)
                cl = class_list(inner.group(1)) if inner and class_list(inner.group(1)) else \
                    class_list(re.sub(r"\(.*?\)", "", lab))
                if cl is not None:
                    text = m.group(2).strip()
                    if re.search(r"\bOR\b", text):
                        for c in cl:
                            out.setdefault(c, []).append("<choice> " + text)
                        continue
                    names = item_names(text)
                    for c in cl:
                        out.setdefault(c, []).extend(names)
                    continue
            if not ITEM_FIELD.match(s) and len(s) < 60 and not s.endswith(":") and re.match(r"^[A-Za-z]", s):
                prev_names.append(re.sub(r"\s*\(.*$", "", s).strip())
    return out


def item_names(text):
    """'Leggings and Tunic of Shadow', 'Goldlight Cap, Weaken Death' -> names; 'Lvl 51' -> [];
    'Thorn of Deatharn !HS, Secondary, Damage 720' -> ['Thorn of Deatharn']."""
    t = re.sub(r"\(.*?\)", " ", text)
    t = re.split(r"\s[-–]\s|\s–|!|\s{2,}|\b[Ll]v?l\.?\s*\d|\bLevel\s*\d|\bNTL\b", t)[0]
    t = " ".join(t.split()).strip(" .;:")
    if not t or re.match(r"^(lvl|level)\b", t, re.I) or t == "?":
        return []
    parts = [p.strip() for p in t.split(",") if p.strip()]
    parts = [p for p in parts if not re.search(r"\d", p) and fold(p) not in NOT_NAMES and
             all(w[:1].isupper() or w.lower() in ("of", "the", "and", "a") for w in p.split())]
    out = []
    for p in parts:
        m = re.match(r"^(\w+) and (\w+) (of .+)$", p)
        if m:
            out += [f"{m.group(1)} {m.group(3)}", f"{m.group(2)} {m.group(3)}"]
        else:
            out.append(p)
    return out


ORDINAL = {"first": 0, "second": 1, "third": 2, "fourth": 3, "fifth": 4}


# Steps whose guide text the parse cannot read, judged by hand from the guide and the import's notes:
# {(quest, step): {class: (kind, why, [the guide's names])}, "silent": why for every other class}.
_NEXUS, _SOUL, _WILD = "The Nexus", "The Soulstone", "The Wildheart"
OVERRIDES = {
    # Plane of Disease Spell Quest: each class's ability; the cleric's Purifying Winds of Nife has no spellpattern row
    (101294, 9): {9: ("real", "the guide's spell is not on this server (no spellpattern row)", ["Purifying Winds of Nife"])},
    # Grum's drop: the guide's Fetid helm/tunic for rangers, rogues and bards have no itempattern rows
    (101294, 10): {c: ("real", "the guide's item is not on this server", ["Fetid Scaled Flesh Helm"]) for c in (1, 5, 6)},
    # Plane of Disease Weapon Quest: The Bloodsabre (Rge Brd Ran), Thorn of Deatharn (monk) are not on this server
    (101295, 5): {**{c: ("real", "the guide's item is not on this server", ["The Bloodsabre"]) for c in (1, 5, 6)},
                  4: ("real", "the guide's item is not on this server", ["Thorn of Deatharn"])},
    # Acolyte of Bristlebane, the handle: the guide's named items are not on this server; four classes' are unnamed
    (101277, 3): {2: ("real", "not on this server", ["Gloves of Mischief"]), 3: ("real", "not on this server", ["Boots of Trickery"]),
                  6: ("real", "not on this server", ["Trickster's Gloves"]), 9: ("real", "not on this server", ["Leggings of the Trickster"]),
                  11: ("real", "not on this server", ["Leggings of Trickery"]), 12: ("real", "not on this server", ["Pants of the Trickster"]),
                  13: ("real", "not on this server", ["Gloves of Trickery"]), 14: ("real", "not on this server", ["Mischievous Pants"]),
                  "silent": "the guide gives the class's level only ('Lvl 51'), no item"},
    # Highbourne's level 60 ring: the casters', healers' and melees' rings are not on this server
    (101237, 7): {**{c: ("real", "the guide's ring is not on this server", [_NEXUS]) for c in (10, 11, 12, 13, 14)},
                  **{c: ("real", "the guide's ring is not on this server", [_SOUL]) for c in (7, 8, 9)},
                  **{c: ("real", "the guide's ring is not on this server", [_WILD]) for c in (1, 4, 5, 6)}},
    # Were-hunter PART 2: the guide names the healers' Charm of Gemile only ("Rewards will vary by class")
    (101387, 9): {"silent": "the guide names the healers' charm only (\"Rewards will vary by class\")"},
}


# Gate gaps that are real but held back from the fix: {quest: why}.
HOLD = {101322: "the guide's Dead Hills Port is a druid's and a wizard's; this server has no wizard record of it, so the "
                "import kept it the druid's (its notes)",
        **{q: "a GM (jibrish, GM panel) set this gate without the bard on Heirloom at 19:08 UTC 2026-11-14: theirs to say"
           for q in (100767, 100769, 100770)}}


OPEN = [
    "**The level 17 class sets are used up.** Every class but the bard, the necromancer (whose one spare ring is a "
    "second record of the ring Neriak gives) and the alchemist already gets every piece of its level 17 set from some "
    "level 17 quest, so the rule of petition 5092 (no class the same piece from two quests of a level) leaves 33 "
    "guide-silent gaps on the twelve level 17 quests unfilled on Live (32 on Heirloom, where a wizard's Ring of "
    "Insight is free); the alchemist's eight Blue Aura pieces fill eight of its twelve. "
    "Filling them needs a choice: a repeat, a class-only level 17 item outside the set (the monk's Iron Fist and "
    "Seisen Robe, the alchemist's spirit enriched robe), or nothing.",
    "**Alignment.** The guides' Class lines are read as written; \"Good\" or \"Evil only\" (Halas, Grobb) takes "
    "away RAN/PAL/DRD (evil) or SK/NEC (good) and nothing else. So a necromancer counts as allowed on Qeynos's and "
    "Rivervale's quests, whose guides say All/All, and the bard and the alchemist on Grobb's (the tool opens Froglok "
    "Liver to both; petition 5088 opened it to five classes and called that every evil class).",
    "**Named items not on this server** stay gaps the tool cannot fill: the Plane of Disease rewards (the cleric's "
    "Purifying Winds of Nife is only a level 60 record; The Bloodsabre, Thorn of Deatharn and Grum's Fetid helms are "
    "not here), the Acolyte of Bristlebane's eight Trickster/Mischief pieces, and Highbourne's level 60 rings (The "
    "Nexus, The Soulstone, The Wildheart; 101237's guide has no Class line, so it is listed, not counted). Also the "
    "ZP armour sets (101475 Akessan Leggings for WAR/PAL/SK, 101480/101482 Honor Guard gloves and boots) and the "
    "Master Spellsmith's gifts, whose sources are not EQOA-Data.",
    "**Ability steps** (23-47's Goblin Poison for the necromancer, Margun Leafstrider for the rogue) and **archetype "
    "steps** (53-59's three quests for the alchemist, whose casters' items are not the alchemist's; the Siloth "
    "Shipment's healers' charm) are guide-silent but not set steps: left.",
    "**Gates held:** Fendren's Dead Hills Port stays the druid's (no wizard record of the port); on Heirloom a GM "
    "(jibrish) took the bard off three level 30 quests (100767, 100769, 100770) at 19:08 UTC today, left as theirs.",
    "**Found on the way, not changed:** on Live the Gypsy Honor Ring (701541), which The Werehunter's Brand (101390) "
    "gives every class, has classuse 1024 (the magician only) and level 1; Heirloom's has 32767 and level 30.",
    "Characters who finished a filled quest before are not given the new piece (the picks list them by class).",
    "A guide-silent fill assumes the owner's earlier picks' rule (a class with nothing gets a piece of its own set); "
    "the 23-47 Hodstock parts give class items from the guide's list and now every class one of its set.",
]


# ---- the database ------------------------------------------------------------------------------------------------

def ids(s):
    return [int(x) for x in str(s or "").split(",") if x.strip().lstrip("-").isdigit()]


def gate(classes):
    c = set(ids(classes))
    return set(ALL) if not c else c


def load(db):
    qs = db.execute("SELECT questid, name, classes, races, minlevel, source, notes FROM quests "
                    "WHERE source LIKE 'EQOA-Data %' ORDER BY questid").fetchall()
    steps = {}
    for q, s, kind, target, byclass, ritems in db.execute(
            "SELECT questid, step, kind, target, byclass, rewarditems FROM queststeps ORDER BY questid, step"):
        steps.setdefault(q, []).append(dict(step=s, kind=kind, target=target, byclass=json.loads(byclass) if byclass else None,
                                            rewarditems=ritems))
    return qs, steps


def audit(db, guides):
    qs, steps = load(db)
    items = {}
    for pid, name, cu in db.execute("SELECT patternid, itemname, classuse FROM itempattern"):
        items.setdefault(fold(name), []).append((pid, name, cu))
    rows, plan = [], []
    by_file_name = {}
    meta = {}
    for qid, name, classes, races, minlevel, source, notes in qs:
        key = re.sub(r"^EQOA-Data (.*?) \(tools/import_quests\.py\)$", r"\1", source)
        m = re.match(r"From EQOA-Data Quests/([^,]+?\.json)(?:, (.*?))? \(github", notes or "")
        rel, label = (m.group(1), m.group(2)) if m else (None, None)
        meta[qid] = (key, rel, label, name, classes, minlevel, notes)
        if rel:
            by_file_name.setdefault((rel, name), set()).update(gate(classes))
    for qid, (key, rel, label, name, classes, minlevel, notes) in meta.items():
        r = dict(questid=qid, name=name, key=key, file=rel, label=label, level=minlevel, gaps=[], notes=[])
        rows.append(r)
        if key.startswith(SKIP_KEYS):
            r["skip"] = "no EQOA-Data guide file (labelled so, from another source)"
            continue
        if rel is None:
            r["skip"] = "notes name no guide file"
            continue
        whole_text = guide_text(guides, rel)
        if whole_text is None:
            r["skip"] = f"guide {rel} not found"
            continue
        whole = whole_text.splitlines()
        sec, matched = section(whole_text, label)
        m120 = re.match(r"^1-20/[^/]+/([^/]+)/", key)
        if m120:
            c = word_classes(m120.group(1))
            allowed, how = (c, f"class guide {m120.group(1)}") if c else (None, "class guide")
        else:
            allowed, how = allowed_from(class_line(sec, whole), sec, whole)
        r["allowed"] = sorted(allowed) if allowed is not None else None
        r["how"] = how
        r["section"] = "matched" if matched else "whole guide"
        ours = gate(classes)
        union = by_file_name.get((rel, name), ours)
        r["gate"] = sorted(ours)
        listed = rewards_in(sec) if not m120 else {}
        # a list shared by a guide's parts: the import gave each part the Nth of each class's list (its notes say)
        mo = re.search(r"the (first|second|third|fourth|fifth) of (?:each|the) class's (?:reward )?list", notes or "")
        if mo and listed:
            n = ORDINAL[mo.group(1)]
            listed = {c: ([v[n]] if len(v) > n else []) for c, v in listed.items()}
            r["notes"].append(f"the guide's class list is shared by its parts: this part's is the {mo.group(1)} of each")
        r["listed"] = {str(c): v for c, v in sorted(listed.items())}
        if allowed is None:
            r["skip"] = how
            continue
        # the gate
        widen = []
        for c in sorted(ALL):
            if c in allowed and c not in union:
                r["gaps"].append(dict(kind="real", what="gate", cls=c, why=HOLD.get(qid)))
                if qid not in HOLD:
                    widen.append(c)
            elif c not in allowed and c not in ours and not m120:
                r["gaps"].append(dict(kind="legitimate", what="gate", cls=c))
        # the rewards: each step with a byclass, judged against the guide's list
        rsteps = [s for s in steps.get(qid, []) if s["byclass"]]
        named = {c for c, v in listed.items() if v and any(not x.startswith("<choice>") for x in v)}
        choice = {c for c, v in listed.items() if any(x.startswith("<choice>") for x in v)}
        if not rsteps and named and not choice:
            r["notes"].append(f"to check by hand: the guide's {r['section']} names class rewards "
                              f"({', '.join(NAMES[c] for c in sorted(named))}) and no step gives any by class")
        fills = {}
        eff = ours | set(widen)
        for s in rsteps:
            bc = s["byclass"]
            ov = OVERRIDES.get((qid, s["step"]))

            def has(c):
                e = bc.get(str(c)) or {}
                return bool(str(e.get("items") or "").strip() or str(e.get("spells") or "").strip())
            given = {c for c in ALL if has(c)}
            for c in sorted(ALL):
                if c in given:
                    continue
                if c not in allowed:
                    r["gaps"].append(dict(kind="legitimate", what="reward", cls=c, step=s["step"]))
                    continue
                if c not in eff:
                    continue  # another quest of the guide's split takes the class
                if ov is not None:
                    kind, why, names = ov.get(c, ("guide-silent", ov.get("silent", "the guide names nothing"), []))
                    r["gaps"].append(dict(kind=kind, what="reward", cls=c, step=s["step"], guide=names, why=why,
                                          resolved=resolve(items, names, c) if names else None))
                    fills.setdefault(s["step"], {})[str(c)] = {"guide": names} if names else {"set": True}
                    continue
                if c in named:
                    want = [x for x in listed[c] if not x.startswith("<choice>")]
                    r["gaps"].append(dict(kind="real", what="reward", cls=c, step=s["step"], guide=want,
                                          resolved=resolve(items, want, c)))
                    fills.setdefault(s["step"], {})[str(c)] = {"guide": want}
                elif c in choice:
                    r["gaps"].append(dict(kind="real", what="reward (a choice)", cls=c, step=s["step"], guide=listed[c]))
                elif given:
                    r["gaps"].append(dict(kind="guide-silent", what="reward", cls=c, step=s["step"]))
                    fills.setdefault(s["step"], {})[str(c)] = {"set": True}
        if widen or fills:
            plan.append(dict(questid=qid, name=name, key=key, level=minlevel, allowed=sorted(allowed), widen=widen,
                             steps=[dict(step=k, fill=v) for k, v in sorted(fills.items())]))
    return rows, plan


def resolve(items, names, c):
    """[(name, patternid or None, why)] the guide's names as items the class may wear."""
    out = []
    for n in names:
        k = fold(n)
        cands = items.get(k) or items.get(k[:-1] if k.endswith("s") else k + "s") or []
        ok = [p for p in cands if (p[2] >> c) & 1]
        if ok:
            out.append((n, ok[0][0], "ok"))
        elif cands:
            out.append((n, None, f"{cands[0][0]} not for {NAMES[c]} (classuse {cands[0][2]})"))
        else:
            out.append((n, None, "no item of the name"))
    return out


# ---- the report --------------------------------------------------------------------------------------------------

def merge(plans):
    """One plan from each shard's: a quest's widen and each step's fills, united."""
    out = {}
    for plan in plans:
        for q in plan:
            m = out.setdefault(q["questid"], dict(q, widen=[], steps=[]))
            m["widen"] = sorted(set(m["widen"]) | set(q["widen"]))
            steps = {s["step"]: s["fill"] for s in m["steps"]}
            for s in q["steps"]:
                steps.setdefault(s["step"], {}).update({k: v for k, v in s["fill"].items() if k not in steps.get(s["step"], {})})
            m["steps"] = [dict(step=k, fill=dict(sorted(v.items(), key=lambda kv: int(kv[0])))) for k, v in sorted(steps.items())]
    return [out[k] for k in sorted(out)]


def counts(rows):
    from collections import Counter
    cnt = {k: Counter() for k in ("real", "guide-silent", "legitimate")}
    for r in rows:
        for g in r["gaps"]:
            cnt[g["kind"]][g["cls"]] += 1
    return cnt


def db_gaps(db):
    """Steps where a class the quest's gate lets in gets nothing while others get something (the database alone)."""
    out = []
    for q, cl, name, step, bc in db.execute("SELECT q.questid, q.classes, q.name, s.step, s.byclass FROM queststeps s "
                                            "JOIN quests q USING (questid) WHERE s.byclass != '' ORDER BY q.questid, s.step"):
        b = json.loads(bc)
        miss = [c for c in sorted(gate(cl)) if not ((b.get(str(c)) or {}).get("items") or (b.get(str(c)) or {}).get("spells"))]
        if miss:
            out.append((q, name, step, miss))
    return out


def report(shards, plan, picks, path):
    L = ["# Quest class gaps against the EQOA-Data guides (2026-11-14)", "",
         "Made by `tools/quest_class_audit_20261114.py` (read-only) from copies of both shards' databases. Fixed by "
         "`tools/quest_class_gaps_20261114.py` from the plan `tools/quest_class_gaps_20261114.json` (the two shards' "
         "audits united).", "",
         "- **real**: the guide allows the class; our gate blocks it, or the guide names its reward and our step "
         "gives it nothing.",
         "- **guide-silent**: the guide allows the class, names no reward for it, and our step gives it nothing while "
         "it gives other classes theirs.",
         "- **legitimate**: the guide does not allow the class (its Class line, or the alignment it gives: evil has no "
         "RAN/PAL/DRD, good no SK/NEC); our gate or step leaves it out. The 1-20 class guides' quests (one class each, "
         "matching) are not counted.", "",
         "A class counts as rewarded on a step when its byclass entry gives an item or an ability. The guides' "
         "\"Races:\" lines are not judged.", ""]
    for label, rows, dbg in shards:
        cnt = counts(rows)
        L += [f"## {label}: gaps by class", "",
              f"Quests with an EQOA-Data source: {len(rows)}; judged: {sum(1 for r in rows if 'skip' not in r)}; "
              f"skipped (no class line to judge by): {sum(1 for r in rows if 'skip' in r)}.", "",
              "| class | real | guide-silent | legitimate |", "|---|---|---|---|"]
        for c in range(15):
            L.append(f"| {c} {NAMES[c]} | {cnt['real'][c]} | {cnt['guide-silent'][c]} | {cnt['legitimate'][c]} |")
        L.append(f"| **all** | {sum(cnt['real'].values())} | {sum(cnt['guide-silent'].values())} | "
                 f"{sum(cnt['legitimate'].values())} |")
        L += ["", f"### {label}: steps where a class the gate lets in gets nothing (the database alone)", "",
              "| quest | step | classes with nothing | verdict |", "|---|---|---|---|"]
        byq = {r["questid"]: r for r in rows}
        for q, name, step, miss in dbg:
            r = byq.get(q)
            if r is None:
                v = "not an EQOA-Data quest"
            elif "skip" in r:
                v = f"not judged: {r['skip']}"
            else:
                kinds = {}
                for g in r["gaps"]:
                    if g.get("step") == step:
                        kinds.setdefault(g["kind"], []).append(NAMES[g["cls"]])
                v = "; ".join(f"{k} {', '.join(v)}" for k, v in sorted(kinds.items())) or "-"
            L.append(f"| {q} {name} | {step} | {', '.join(NAMES[c] for c in miss)} | {v} |")
        L += ["", f"### {label}: real and guide-silent gaps, quest by quest", ""]
        for r in rows:
            g = [x for x in r["gaps"] if x["kind"] != "legitimate"]
            if not g:
                continue
            L.append(f"#### {r['questid']} {r['name']} (`{r['key']}`, level {r['level']})")
            L.append("")
            L.append(f"Guide `{r['file']}`, section {r['label']!r} ({r.get('section')}); Class line: {r['how']}; allows "
                     f"{', '.join(NAMES[c] for c in r['allowed'])}; our gate: "
                     f"{'every class' if len(r['gate']) == 15 else ', '.join(NAMES[c] for c in r['gate'])}.")
            L.append("")
            by = {}
            for x in g:
                if x["what"] == "gate":
                    k = ("real: the gate blocks", x.get("why"))
                elif x["kind"] == "guide-silent":
                    k = (f"guide-silent: step {x['step']} gives nothing, the guide names none", x.get("why"))
                else:
                    k = (f"real: step {x['step']} gives nothing; the guide's " + ", ".join(x.get("guide") or []), x.get("why"))
                by.setdefault(k, []).append(NAMES[x["cls"]])
            for (what, why), cs in by.items():
                L.append(f"- {what}: {', '.join(cs)}{' (' + why + ')' if why else ''}")
            for n in r["notes"]:
                L.append(f"- note: {n}")
            L.append("")
        checks = [r for r in rows if any(n.startswith("to check by hand") for n in r["notes"]) and
                  not any(x["kind"] != "legitimate" for x in r["gaps"])]
        if checks:
            L += [f"### {label}: named class rewards with no class-reward step (read by hand)", "",
                  "The section (or, where none matched, the whole guide) names rewards by class and the quest gives "
                  "none by class. Read against the guides and the import's notes: the whole-guide ones are another "
                  "part's list, and the rest are items the notes record as not on this server. Not counted.", ""]
            for r in checks:
                L.append(f"- {r['questid']} {r['name']} (`{r['key']}`, {r.get('section')})")
            L.append("")
        L += [f"### {label}: legitimate gaps", "", "| quest | class line | left out |", "|---|---|---|"]
        for r in rows:
            gl = sorted({x["cls"] for x in r["gaps"] if x["kind"] == "legitimate"})
            if gl:
                L.append(f"| {r['questid']} {r['name']} | {r['how']} | {', '.join(NAMES[c] for c in gl)} |")
        L += ["", f"### {label}: not judged", "", "| why | quests |", "|---|---|"]
        sk = {}
        for r in rows:
            if "skip" in r:
                sk.setdefault(r["skip"], []).append(r)
        for why, rs in sorted(sk.items(), key=lambda kv: -len(kv[1])):
            keys = sorted({"/".join(x["key"].split("/")[:2]) for x in rs})
            L.append(f"| {why} | {len(rs)}: {', '.join(keys[:12])}{' ...' if len(keys) > 12 else ''} |")
        L.append("")
    if picks:
        L += ["## The fix tool's picks, by shard", "",
              "`tools/quest_class_gaps_20261114.py --db <shard>` (report only), as it reads each copy. Each shard's "
              "item ids are its own; a pick is a class-only wearable item of the quest's level carrying the class's "
              "set word, one no quest of that level gives the class already.", "",
              "| shard | pieces given | gates widened | left (no piece free, or not a set step) |", "|---|---|---|---|"]
        for label, lines in picks:
            given = sum(1 for x in lines if re.search(r" step \d+, [A-Z]{2,3}: (?!left)", x))
            widened = sum(1 for x in lines if ": gate widened to " in x)
            left = sum(1 for x in lines if re.search(r" step \d+, [A-Z]{2,3}: left", x))
            L.append(f"| {label} | {given} | {widened} | {left} |")
        L.append("")
        for label, lines in picks:
            L.append(f"### {label}")
            L.append("")
            L += [f"- {x}" for x in lines]
            L.append("")
    L += ["## For the owner", ""] + [f"- {x}" for x in OPEN] + [""]
    with open(path, "w") as f:
        f.write("\n".join(L) + "\n")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", action="append", required=True, help="LABEL=PATH (or PATH), once a shard")
    ap.add_argument("--guides", required=True)
    ap.add_argument("--plan")
    ap.add_argument("--report")
    ap.add_argument("--json", help="every row as JSON (debugging)")
    a = ap.parse_args()
    shards, plans = [], []
    for spec in a.db:
        label, _, path = spec.rpartition("=")
        label = label or os.path.basename(path)
        db = sqlite3.connect(f"file:{path}?mode=ro", uri=True)
        rows, plan = audit(db, a.guides)
        shards.append((label, rows, db_gaps(db), path))
        plans.append(plan)
        db.close()
        n = {}
        for r in rows:
            for g in r["gaps"]:
                n[g["kind"]] = n.get(g["kind"], 0) + 1
        print(f"{label}: {len(rows)} quests, {sum(1 for r in rows if 'skip' not in r)} judged; gaps {n}; plan {len(plan)} quests")
    plan = merge(plans)
    if a.plan:
        with open(a.plan, "w") as f:
            json.dump({"made": "tools/quest_class_audit_20261114.py", "quests": plan}, f, indent=1, ensure_ascii=False)
            f.write("\n")
    if a.json:
        with open(a.json, "w") as f:
            json.dump({lab: rows for lab, rows, _, _ in shards}, f, indent=1, default=str)
    picks = []
    if a.report:
        import quest_class_gaps_20261114 as fix
        for label, _, _, path in shards:
            d = sqlite3.connect(f"file:{path}?mode=ro", uri=True)
            _, lines, refusals = fix.plan(d, plan)
            d.close()
            picks.append((label, lines + [f"REFUSED: {x}" for x in refusals]))
        report([(lab, rows, dbg) for lab, rows, dbg, _ in shards], plan, picks, a.report)
    print(f"plan: {len(plan)} quests")


if __name__ == "__main__":
    sys.exit(main())
