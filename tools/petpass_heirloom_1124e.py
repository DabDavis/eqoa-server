#!/usr/bin/env python3
"""Heirloom petition 2832 (Deathknell, 2026-11-24, Spell or ability): "After my 60 epic, I chose Quil Eynes Unspeakable
hunger but I received Quil Eynes cruel embrace instead". Both shards.

  The two shards number the Ascended epic spells apart: Heirloom's are spellpattern 906724-906813, Live's the same
  90 spells, by the same names, at 906605-906694 (Heirloom's id less 119; PAIRS below, read off both shards'
  spellpattern 2026-11-24). Each shard's level 60 epic reward menus (quests 101114-101136, Ediopolus, step 1's
  choices) were seeded from one list, so some name the OTHER shard's ids, which this shard does not have:
    Heirloom: the Ranger's (101118), Wizard's (101120, 101132), Alchemist's (101121, 101133), Enchanter's (101122,
              101134) and Shadowknight's (101128) menus, all their options;
    Live:     the Paladin's (101115), Bard's (101116), Monk's (101119), Druid's (101126) and Necromancer's (101131).
  The pick was taken right (Deathknell's charquests picks "1:0", the log "picked Ascended Quil'Eyne's Unspeakable
  Hunger") and the option's spell written: 906663, Live's Unspeakable Hunger, a spell Heirloom has no record of, so
  his book showed something else. No index fault, no swapped pair: a dangling id.

  - Each such option's spell becomes this shard's id for the same spell (its partner in PAIRS, refused unless this
    shard's record has the pair's name).
  - A character's spell row holding a dangling paired id is moved to this shard's id (its book place kept), or deleted
    when the character holds that spell already. On Heirloom that is Deathknell's 906663 -> 906782 (Ascended Quil`Eyne's
    Unspeakable Hunger); no one else on either shard holds one. He holds no Cruel Embrace row, plain or Ascended.

Refused, nothing written, when a row is not as expected. Report only by default; --apply in one transaction, a
gm_audit line a row, log petpass_heirloom_1124e_log; --undo exact. A menu change is read at the next talk to
Ediopolus (quests reload at a restart, per the server's quest cache); a spell row at the character's next login.

    python3 -E -s tools/petpass_heirloom_1124e.py --db eqoa.db [--apply | --undo]
"""
import json
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_heirloom_1124e_log"
DETAIL = ("Heirloom petition 2832: the level 60 epic reward menus give this shard's Ascended epic spell ids, and spell "
          "rows of the other shard's ids moved to them (tools/petpass_heirloom_1124e.py)")
QUESTS = (101114, 101136)
# (Live id, Heirloom id, the name both shards give it)
PAIRS = [
    (906605, 906724, 'Ascended Warcry'),
    (906606, 906725, 'Ascended Atrophic Wind'),
    (906607, 906726, 'Ascended Blessed Bastion'),
    (906608, 906727, 'Ascended Blood Vortex'),
    (906609, 906728, 'Ascended Doom Tower'),
    (906610, 906729, 'Ascended Flowering Health'),
    (906611, 906730, 'Ascended Freeport Cocktail'),
    (906612, 906731, 'Ascended Mortal Wound'),
    (906613, 906732, 'Ascended Natureblade'),
    (906614, 906733, 'Ascended Towering Mind'),
    (906615, 906734, 'Ascended Freezing Mixture'),
    (906616, 906735, 'Ascended Power Flux'),
    (906617, 906736, 'Ascended Abolish Death'),
    (906618, 906737, 'Ascended Agonize Death'),
    (906619, 906738, "Ascended Ahrianna's Blessed Boon"),
    (906620, 906739, "Ascended Ahrianna's Generous Gift"),
    (906621, 906740, "Ascended Aktrana's Cerebral Fortress"),
    (906622, 906741, "Ascended Aktrana's Tower of Will"),
    (906623, 906742, "Ascended Boldunder's Mountain Roots"),
    (906624, 906743, "Ascended Boldunder's Personal Mine"),
    (906625, 906744, "Ascended Calidux's Equitable Division"),
    (906626, 906745, "Ascended Calidux's Insatiable Appetite"),
    (906627, 906746, "Ascended Celedonicus' Grand Finale"),
    (906628, 906747, "Ascended Celedonicus' Inspiring Performance"),
    (906629, 906748, 'Ascended Celestial Light'),
    (906630, 906749, 'Ascended Debilitating Hex'),
    (906631, 906750, 'Ascended Ethereal Healing'),
    (906632, 906751, 'Ascended Fine Red Wine'),
    (906633, 906752, "Ascended Gelledyn's Feral Imitation"),
    (906634, 906753, "Ascended Gelledyn's Perfect Vision"),
    (906635, 906754, 'Ascended Glimmering Blades'),
    (906636, 906755, 'Ascended Goading Ridicule'),
    (906637, 906756, "Ascended Graal's Final Strike"),
    (906638, 906757, "Ascended Graal's Venomus Strike"),
    (906639, 906758, "Ascended Jakai's Invigorating Bellow"),
    (906640, 906759, "Ascended Jakai's Rallying Cry"),
    (906641, 906760, "Ascended Jen'Tylis' Lust for Power"),
    (906642, 906761, "Ascended Jen'Tylis' Overarching Greed"),
    (906643, 906762, "Ascended Magi'Kot's Good Breeding"),
    (906644, 906763, "Ascended Magi`Kot's Sacrificial Breeding"),
    (906645, 906764, "Ascended Mai'adra's Steadfast Assistance"),
    (906646, 906765, "Ascended Mai'adra's Unrelenting Hope"),
    (906647, 906766, 'Ascended Manaburn'),
    (906648, 906767, "Ascended Mega's Clutch Ward"),
    (906649, 906768, "Ascended Mega's Mighty Carapace"),
    (906650, 906769, "Ascended Melinis' Tranquil Stance"),
    (906651, 906770, "Ascended Melinus' Harmonius Thoughts"),
    (906652, 906771, "Ascended Ozmo's Careful Teasing"),
    (906653, 906772, "Ascended Ozmo's Echoing Mockery"),
    (906654, 906773, "Ascended Ozmo's Infuriating Jeer"),
    (906655, 906774, "Ascended Ozmo's Maternal Asperation"),
    (906656, 906775, "Ascended Ozmo's Unrelenting Ridicule"),
    (906657, 906776, 'Ascended Pandemonium'),
    (906658, 906777, 'Ascended Penetrating Blow'),
    (906659, 906778, 'Ascended Power Groove'),
    (906660, 906779, 'Ascended Pulsing Flurry'),
    (906661, 906780, 'Ascended Punt'),
    (906662, 906781, "Ascended Quil'Eyne's Cruel Embrace"),
    (906663, 906782, "Ascended Quil`Eyne's Unspeakable Hunger"),
    (906664, 906783, "Ascended Rashiroz's Perfect Evasion"),
    (906665, 906784, "Ascended Rashiroz's Persistent Vanishing"),
    (906666, 906785, 'Ascended Refreshing Concoction'),
    (906667, 906786, "Ascended Ruddish's Inspiring Piety"),
    (906668, 906787, "Ascended Ruddish's Unassailable Faith"),
    (906669, 906788, "Ascended Severupidus' Cocktail of Death"),
    (906670, 906789, "Ascended Severupidus' Cocktail of Torment"),
    (906671, 906790, 'Ascended Shield of 1000 Needles'),
    (906672, 906791, 'Ascended Sinister Cantrip'),
    (906673, 906792, 'Ascended Sothimus Artiteral Targetting'),
    (906674, 906793, 'Ascended Sothimus Critical Wounding'),
    (906675, 906794, 'Ascended Staccato Flurry'),
    (906676, 906795, 'Ascended Staggering Shot'),
    (906677, 906796, 'Ascended Strategic Blow'),
    (906678, 906797, 'Ascended Strike of the Serpent'),
    (906679, 906798, "Ascended Synder's Burning Hatred"),
    (906680, 906799, "Ascended Synder's Malevolent Mindset"),
    (906681, 906800, "Ascended Talimus's Adamantine Barrier"),
    (906682, 906801, "Ascended Talimus's Diamond Barrier"),
    (906683, 906802, "Ascended Teledic's Chilling Breath"),
    (906684, 906803, 'Ascended Visage'),
    (906685, 906804, "Ascended Vylinnuza's Tidal Destruction"),
    (906686, 906805, 'Ascended Ward of Blessings'),
    (906687, 906806, "Ascended Yalgima's Fearsome Companion"),
    (906688, 906807, "Ascended Yalgima's Menacing Defender"),
    (906689, 906808, "Ascended Severupidus' Fire Potion"),
    (906690, 906809, 'Ascended Eldrich Rune'),
    (906691, 906810, 'Ascended Infectious Aura'),
    (906692, 906811, "Ascended Vylinnuza's Merciless Blizzard"),
    (906693, 906812, "Ascended Vylinnuza's Merciless Inferno"),
    (906694, 906813, 'Ascended Superior Mana Stone'),
]
PARTNER = {}
for _l, _h, _n in PAIRS:
    PARTNER[_l] = (_h, _n)
    PARTNER[_h] = (_l, _n)


def norm(s):
    return re.sub(r"[`'’]", "'", s or "")


def changes(db):
    out, lines, refusals = [], [], []
    names = {r["spellid"]: r["spellname"] for r in tr.rows(db, "SELECT spellid, spellname FROM spellpattern")}

    def here(x):
        """This shard's id for x, or None when x is this shard's own (or not a pair's)."""
        if x in names or x not in PARTNER:
            return None
        p, n = PARTNER[x]
        if p not in names:
            refusals.append(f"{x}: neither it nor its partner {p} ({n}) is a spell here")
            return None
        if norm(names[p]) != norm(n):
            refusals.append(f"{x}: its partner {p} here is {names[p]!r}, not {n!r}")
            return None
        return p

    moved = {}
    for st in tr.rows(db, "SELECT questid, step, choices FROM queststeps WHERE questid BETWEEN ? AND ? AND choices != ''"
                          " ORDER BY questid, step", QUESTS):
        ch = json.loads(st["choices"])
        changed = False
        for o in ch.get("options", []):
            ids = [x for x in re.split(r"[ ,]+", str(o.get("spells", "")).strip()) if x]
            new = []
            for x in ids:
                p = here(int(x))
                new.append(str(p) if p else x)
                if p:
                    moved[int(x)] = p
            if new != ids:
                lines.append(f"  {st['questid']} step {st['step']} {o.get('answer')!r}: spells {','.join(ids)} -> "
                             f"{','.join(new)}")
                o["spells"] = ",".join(new)
                changed = True
        if changed:
            key = {"questid": st["questid"], "step": st["step"]}
            row = tr.get(db, "queststeps", key)
            out.append(("queststeps", key, dict(row, choices=json.dumps(ch, ensure_ascii=False, separators=(",", ":")))))

    held = tr.rows(db, "SELECT s.id, s.serverid, s.spellid, c.charName AS name FROM spells s "
                       "LEFT JOIN characters c ON c.serverid = s.serverid WHERE s.spellid IN (%s) ORDER BY s.id"
                   % ",".join(str(x) for x in PARTNER))
    for h in held:
        p = here(h["spellid"])
        if not p:
            continue
        key = {"id": h["id"]}
        has = tr.rows(db, "SELECT id FROM spells WHERE serverid = ? AND spellid = ?", (h["serverid"], p))
        if has:
            out.append(("spells", key, None))
            lines.append(f"  spells {h['id']} {h['name']} ({h['serverid']}): {h['spellid']} deleted, holds {p} "
                         f"({names[p]}) already")
        else:
            out.append(("spells", key, dict(tr.get(db, "spells", key), spellid=p)))
            lines.append(f"  spells {h['id']} {h['name']} ({h['serverid']}): {h['spellid']} -> {p} ({names[p]})")

    if refusals:
        out = []
    applied = not out and not refusals
    lines.insert(0, f"{len(out)} rows ({sum(1 for t, _, _ in out if t == 'queststeps')} menus, "
                    f"{sum(1 for t, _, _ in out if t == 'spells')} spell rows)")
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("queststeps", "spellpattern", "spells", "characters"),
        "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
