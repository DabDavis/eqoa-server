#!/usr/bin/env python3
"""Live petitions 5575 and 5576 (Xylith, 2026-11-24, by the Storyteller's well in Baga Village): "The stroyteller
Mercants should sale items for Storyteller faction" and "Please add the faction to these quests and Mobs for the
storyteller ... and back cap and give faction to players that have killed", both pointing at the players' guide
https://eqoa.forumotion.com/t306-storyteller-abilities (Ronson, 2010-07-31). The owner approved all three parts.

What the guide says, and what this writes (Live only):

  1. Creatures (factionhits, The Storyteller): the guide's named kills with a number, by Live's spelling: Mayong
     Mistmoore 25, Lord Nagafen 25, CazicThule 50 ("Cazic Thule"), Lady Vox 25, King Daran 25, Guh Stonthro 25,
     Kyha Flintnails 25 ("Khya"), Ahsleg 25, Abyssaxis 25, Growl Thornpaw 25, Bhark Mudpaw 25, Yip Snageltoof 25,
     Lil'snapa 25 ("Lil Snapa"), Ole' Fangtooth 25 ("Old Fangtooth"), Gurig 10, Jonathan Hesper 25 ("Jonathon").
     Growl Thornpaw's line has no number of its own: the guide breaks the Blackburrow line "Growl Thornpaw / Bhark
     Mudpaw Yip Snageltoof (lvl 44) +25" in two, read as one. A kill moves the killer and the group members who share
     it (entity.factionKillLocked), as the guide's "ks group".
     Left out: named with no number (Veeshan's Bulwark and Talon groups, the Plane of Sky constructs, Fzal Fingergnaw,
     Nizbrei Bloodbreath, General Gnarr, Oveed the Unwashed, "Sid Akista"), and not on Live (Gragh Skulsplit, Durga
     Thundafut, Bzinel, Fhaziini, Kagriggle, Griegle, Mari Hesper, Kieran Douglas, Tristan Daren, Jake Jenkins).
  2. Quests: the imported guides already give the guide's amounts (Binding an Elemental, both books, +25; Escaping
     Kerran +50; The Temple of Light +75; The Ring of Madrias: The Dark Lord +240; The Illusionist +50; Act 4 +50;
     The Dawn Maiden +25) but The Scale of Veeshan (101439) gives 50 where the guide's "Scales from Bulwark" is 100:
     its step 2 now gives +100.
  3. Records for the shop: Stormsoul (375, the guide's figures, Heirloom's record) and the Flawless Rejuvenative
     Belt (150, Heirloom's record, its classes the guide's melee and alchemists) copied under new ids; the Flawless
     Cathartic and Restorative Belts made from the Flawless Regenerative Belt (1033) with the guide's figures; the
     scroll of Minotaur's Persistence (spell 6257 is here, no scroll was) from Leopard's Elegance's (900490), for
     tanks as Minotaur Form's (901616). Ids 716960-716964, free on both shards.
  4. Back-pay (charfactions, The Storyteller, capped at 2000): the kills above in the game log since Live opened
     (2026-11-07 20:45, the oldest rotated log), each credited to its killer and to whoever took tunar from its corpse
     (a split goes to the group members by it), a kill with no split credited to the group seen at that creature's kill
     within 5 minutes before it; and The Scale of Veeshan's 50 more for Xylith, the only one to finish it. Characters
     in the world at the apply are left to --late NAME at their logout (watch-addr.sh): a logout saves over
     charfactions.
  5. --shop (with the server of branch pet1124r, whose token shops take "faction:<name>"): the four Storyteller
     Merchants sell what the guide lists and Live has, priced in Storyteller standing (spent): the four show-only form
     scrolls 10, the four flawless belts 150, Martyr's Breastplate 300, Stormsoul 375; the Luxury Storyteller Merc (the
     guide's "Luxury Merchants open at 400") the four form abilities at 400. Missing from Live and Heirloom, not made
     (no record to copy their looks and effects from): Fairy Jug, Steam Suit, Spirit Shield (25); Augur's Tunic,
     Charlatan's Gloves, Gloves of Divination, Helm of the Thaumaturge, Mercenary Armguards, Seer's Bracelet, Medic's
     Boots (125); Dagger of Shielding, Exploding Shot Bow, Force Burst Totem, Greater Charged Pike, Heavy Hammer of
     Frostbite (175); Biohazard in a Bottle and the six Books (200); Frostwind, Hatecrux, Harbinger, Lifescourge (375).

Report only by default; --apply in one transaction, a gm_audit line a row, its log table; --undo exact (each part its
own log: petpass_live_1124r_log, _shop_log, _late_<name>_log). WHEN: factionhits at the next start or the panel's
factions reload (a running server keeps its hits); the quest step at a start; records at a login; charfactions at the
character's next login; the shop rows at once, but only a pet1124r server sells for faction.

    python3 -E -s tools/petpass_live_1124r.py --db eqoa.db [--online A,B] [--apply | --undo]
    python3 -E -s tools/petpass_live_1124r.py --db eqoa.db --late NAME [--apply | --undo]
    python3 -E -s tools/petpass_live_1124r.py --db eqoa.db --shop [--apply | --undo]
"""
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

FACTION = "The Storyteller"
CAP = 2000
SRC = "Live petition 5576: eqoa.forumotion.com t306"
DETAIL = "Live petitions 5575/5576: the Storyteller's faction, shop and back-pay (tools/petpass_live_1124r.py)"

HITS = {"Mayong Mistmoore": 25, "Lord Nagafen": 25, "CazicThule": 50, "Lady Vox": 25, "King Daran": 25,
        "Guh Stonthro": 25, "Kyha Flintnails": 25, "Ahsleg": 25, "Abyssaxis": 25, "Growl Thornpaw": 25,
        "Bhark Mudpaw": 25, "Yip Snageltoof": 25, "Lil'snapa": 25, "Ole' Fangtooth": 25, "Gurig": 10,
        "Jonathan Hesper": 25}

SCALE_QUEST, SCALE_STEP, SCALE_WAS, SCALE_NOW = 101439, 2, "The Storyteller:+50", "The Storyteller:+100"

# The kills in Live's game log 2026-11-07 20:45 .. 2026-11-24 03:00 UTC (every gameserver.log*, grepped for the
# names alone): (UTC, creature, entity id, credited). Credited: the killing blow's player and every taker of the
# corpse's tunar; * a kill with no split, given the group seen at that creature 0-5 minutes before.
KILLS = [
    ("2026-11-12T23:16:35", "Ahsleg", 100650, ["Areo", "Battery", "Horizon", "Xylith"]),
    ("2026-11-19T09:26:04", "Yip Snageltoof", 103643, ["Ebony", "Greenhealz", "Kumok", "Magepetbest"]),
    ("2026-11-19T10:30:48", "Yip Snageltoof", 103643, ["Magepetbest"]),
    ("2026-11-19T23:44:37", "Lord Nagafen", 121253, ["Sonos"]),
    ("2026-11-19T23:46:27", "Lady Vox", 103173, ["Sonos"]),
    ("2026-11-20T00:23:09", "Guh Stonthro", 104283, ["Smoke", "Smokey"]),
    ("2026-11-20T00:48:05", "Guh Stonthro", 104283, ["Smoke", "Smokey"]),
    ("2026-11-20T00:57:39", "Kyha Flintnails", 103963, ["Smoke", "Smokey"]),
    ("2026-11-20T00:59:33", "Kyha Flintnails", 103963, ["Smoke", "Smokey"]),
    ("2026-11-20T01:01:33", "Kyha Flintnails", 103963, ["Smoke", "Smokey"]),
    ("2026-11-20T01:05:00", "Kyha Flintnails", 103963, ["Smoke", "Smokey"]),
    ("2026-11-20T01:06:50", "Kyha Flintnails", 103963, ["Smoke", "Smokey"]),
    ("2026-11-20T01:08:50", "Kyha Flintnails", 103963, ["Smoke", "Smokey"]),  # *
    ("2026-11-21T11:00:39", "Lady Vox", 103173, ["Innoruuk", "Lanys", "Odium", "Valizrae"]),
    ("2026-11-22T10:10:27", "Lord Nagafen", 121253, ["Innoruuk", "Jibrish"]),
    ("2026-11-22T10:14:15", "King Daran", 121213, ["Innoruuk", "Jibrish"]),
    ("2026-11-22T10:29:49", "CazicThule", 2861, ["Innoruuk", "Jibrish"]),
    ("2026-11-22T10:31:35", "CazicThule", 2862, ["Innoruuk", "Jibrish"]),
    ("2026-11-23T06:31:39", "Lil'snapa", 105047, ["Kittysong"]),
    ("2026-11-23T16:04:36", "Ahsleg", 100650, ["Kittysong"]),
    ("2026-11-23T16:08:13", "Ahsleg", 100650, ["Kittysong"]),
    ("2026-11-23T18:44:52", "Ahsleg", 100650, ["Kittysong"]),
    ("2026-11-23T18:47:20", "Ahsleg", 100650, ["Kittysong"]),
    ("2026-11-23T23:36:42", "CazicThule", 2788, ["Areo", "Cloar", "Horizon", "Jibrish", "Xylith"]),
    ("2026-11-23T23:38:07", "Lord Nagafen", 2792, ["Areo", "Cloar", "Horizon", "Jibrish", "Xylith"]),
    ("2026-11-23T23:38:17", "King Daran", 2793, ["Areo", "Cloar", "Horizon", "Jibrish", "Xylith"]),  # * 1 tunar
]
# The Scale of Veeshan finished at +50 (charquests done): what the guide's 100 owes them.
QUEST_OWED = {"Xylith": SCALE_QUEST}

# New records: (id, copied from (Heirloom's record, or Live's id), changes).
STORMSOUL = {"patternid": 716960, "itemcost": 4792, "unk1": 0, "itemicon": 724322735, "unk2": 0, "equipslot": 14,
             "unk3": 0, "trade": 0, "rent": 1, "unk4": 0, "attacktype": 1, "weapondamage": 595, "unk5": 0,
             "levelreq": 55, "maxstack": 1, "maxhp": 12000, "duration": 75, "classuse": 111, "raceuse": 1023,
             "procanim": 0, "lore": 0, "unk6": 0, "craft": 0, "itemname": "Stormsoul",
             "itemdesc": "A blade that hums before the weather turns.", "model": 1712912562, "color": 4294967295,
             "str": 20, "sta": 0, "agi": 5, "wis": 0, "dex": 20, "cha": 0, "intelligence": 0, "HPMAX": 125,
             "POWMAX": 0, "PoT": 0, "HoT": 0, "AC": 0, "PR": 0, "DR": 0, "FR": 0, "CR": 0, "LR": 0, "AR": 0,
             "weaponproc": None, "fish": 0}  # Heirloom 960253 as it stands
REJUV = {"patternid": 716961, "itemcost": 6468, "unk1": 0, "itemicon": -1686939223, "unk2": 0, "equipslot": 9,
         "unk3": 0, "trade": 1, "rent": 1, "unk4": 0, "attacktype": 0, "weapondamage": 0, "unk5": 0, "levelreq": 60,
         "maxstack": 1, "maxhp": 12000, "duration": 75, "classuse": 16498, "raceuse": 1023, "procanim": 0, "lore": 1,
         "unk6": 0, "craft": 0, "itemname": "Flawless Rejuvenative Belt",
         "itemdesc": "A flawless belt. Its wearer's wounds close a little faster.", "model": 0, "color": 4294967295,
         "str": 0, "sta": 0, "agi": 0, "wis": 0, "dex": 25, "cha": 0, "intelligence": 0, "HPMAX": 150, "POWMAX": 0,
         "PoT": 0, "HoT": 25, "AC": 20, "PR": 0, "DR": 0, "FR": 0, "CR": 0, "LR": 0, "AR": 0, "weaponproc": 0,
         "fish": 0}  # Heirloom 960079, classuse 32767 -> the guide's Melee/Alc
REGEN = 1033  # Live's Flawless Regenerative Belt, the template of the two below
CATHARTIC = {"patternid": 716962, "itemname": "Flawless Cathartic Belt", "itemdesc": "This belt has cathartic properties.",
             "classuse": 896, "str": 0, "sta": 0, "wis": 25, "intelligence": 0, "HPMAX": 0, "POWMAX": 150, "PoT": 25,
             "HoT": 0, "AC": 14, "duration": 65}
RESTOR = {"patternid": 716963, "itemname": "Flawless Restorative Belt", "itemdesc": "This belt has restorative properties.",
          "classuse": 15360, "str": 0, "sta": 0, "wis": 0, "intelligence": 25, "HPMAX": 0, "POWMAX": 150, "PoT": 25,
          "HoT": 0, "AC": 8, "duration": 75}
LEOPARD_ELEGANCE = 900490
MINO_SCROLL = {"patternid": 716964, "itemname": "Minotaur's Persistence", "classuse": 13,
               "itemdesc": "This scroll will allow you to learn Minotaur's Persistence."}
MINO_SPELL = 6257

FORMS = {900489: "Leopard Form", 901616: "Minotaur Form", 901208: "Panther Form", 900781: "Scorpion Form"}
ABILITIES = {716964: "Minotaur's Persistence", 900490: "Leopard's Elegance", 901209: "Panther's Alacrity",
             900782: "Scorpion's Pestilence"}
MERCHANT, LUXURY = "Storyteller Merchant", "Luxury Storyteller Merc"
TOKEN = "faction:" + FACTION
SHOP = [  # (shop, patternid, name, price, says)
    (MERCHANT, 900489, "Leopard Form", 10, "For show alone, no stats: the leopard's look. Healers only. Spent faction is not given back."),
    (MERCHANT, 901616, "Minotaur Form", 10, "For show alone, no stats: the minotaur's look. Tanks only. Spent faction is not given back."),
    (MERCHANT, 901208, "Panther Form", 10, "For show alone, no stats: the panther's look. Melee and alchemists only. Spent faction is not given back."),
    (MERCHANT, 900781, "Scorpion Form", 10, "For show alone, no stats: the scorpion's look. Casters only. Spent faction is not given back."),
    (MERCHANT, 1033, "Flawless Regenerative Belt", 150, ""),
    (MERCHANT, 716961, "Flawless Rejuvenative Belt", 150, ""),
    (MERCHANT, 716962, "Flawless Cathartic Belt", 150, ""),
    (MERCHANT, 716963, "Flawless Restorative Belt", 150, ""),
    (MERCHANT, 1051, "Martyr's Breastplate", 300, ""),
    (MERCHANT, 716960, "Stormsoul", 375, ""),
    (LUXURY, 716964, "Minotaur's Persistence", 400, "Tanks: twelve more hit point factors. Level 60."),
    (LUXURY, 900490, "Leopard's Elegance", 400, "Healers: 50 DEX, 50 WIS and 50 CHA. Level 60."),
    (LUXURY, 901209, "Panther's Alacrity", 400, "Melee and alchemists: 50 DEX and 50 AGI, and a quicker run. Level 60."),
    (LUXURY, 900782, "Scorpion's Pestilence", 400, "Casters: 50 DEX, 50 INT and 25 power over time. Level 60."),
]


def owed():
    """{name: (amount, [why])}: the kills' and the quest's back-pay."""
    out = {}
    for at, npc, _eid, who in KILLS:
        for n in who:
            a, why = out.get(n, (0, []))
            out[n] = (a + HITS[npc], why + [f"{npc} {at[:16]} +{HITS[npc]}"])
    for n, q in QUEST_OWED.items():
        a, why = out.get(n, (0, []))
        out[n] = (a + 50, why + [f"The Scale of Veeshan ({q}) +50 more"])
    return out


LOG = "petpass_live_1124r_log"


def late_log(name):
    return "petpass_live_1124r_late_" + "".join(c for c in name.lower() if c.isalnum()) + "_log"


def char(db, name):
    r = tr.rows(db, "SELECT serverid FROM characters WHERE charName = ?", (name,))
    return r[0]["serverid"] if len(r) == 1 else None


def logged_before(db, log, key):
    """The row's standing before this tool's write, when its log holds it (a second run pays from there)."""
    if not tr.table(db, log):
        return None
    r = tr.rows(db, f"SELECT before FROM {log} WHERE tbl = 'charfactions' AND rowkey = ? ORDER BY seq LIMIT 1",
                (json.dumps(key, sort_keys=True),))
    if not r:
        return None
    return ("none", None) if r[0]["before"] is None else ("row", json.loads(r[0]["before"])["standing"])


def backpay(db, names, refusals, lines, log):
    out = []
    start = tr.rows(db, "SELECT standing FROM factions WHERE name = ?", (FACTION,))
    base = start[0]["standing"] if start else 0
    pay = owed()
    for n in names:
        sid = char(db, n)
        if sid is None:
            refusals.append(f"no one character named {n}")
            continue
        amt, why = pay[n]
        key = {"serverid": sid, "faction": FACTION}
        have = tr.get(db, "charfactions", key)
        was = have["standing"] if have else base
        prior = logged_before(db, log, key)
        if prior is not None:
            was = base if prior[0] == "none" else prior[1]
        now = min(CAP, was + amt)
        out.append(("charfactions", {"serverid": sid, "faction": FACTION}, {"serverid": sid, "faction": FACTION, "standing": now}))
        lines.append(f"back-pay {n} ({sid}): {was} -> {now} (+{amt}: {'; '.join(why)})")
    return out


def check_live(db, refusals):
    if tr.rows(db, "SELECT 1 FROM npcs WHERE npc_name = 'Spell Archivist' LIMIT 1"):
        refusals.append("this database has Spell Archivists (Heirloom): Live only")
    if not tr.rows(db, "SELECT 1 FROM factions WHERE name = ?", (FACTION,)):
        refusals.append(f"no faction {FACTION!r} here")


def changes_data(db, online):
    out, lines, refusals = [], [], []
    check_live(db, refusals)
    for npc, d in HITS.items():
        if not tr.rows(db, "SELECT 1 FROM npcs WHERE npc_name = ? LIMIT 1", (npc,)):
            refusals.append(f"no NPC named {npc!r} here")
        other = tr.rows(db, "SELECT npc_name, delta FROM factionhits WHERE lower(npc_name) = lower(?) AND faction = ? AND npc_name != ?",
                        (npc, FACTION, npc))
        if other:
            refusals.append(f"another spelling's row for {npc}: {other}")
        out.append(("factionhits", {"npc_name": npc, "faction": FACTION},
                    {"npc_name": npc, "faction": FACTION, "delta": d, "source": SRC}))
    lines.append(f"factionhits: {len(HITS)} creatures for {FACTION}")
    st = tr.get(db, "queststeps", {"questid": SCALE_QUEST, "step": SCALE_STEP})
    if not st or st["target"] != "The Storyteller" or st["factions"] not in (SCALE_WAS, SCALE_NOW):
        refusals.append(f"quest {SCALE_QUEST} step {SCALE_STEP} is not as read: {st and (st['target'], st['factions'])}")
    out.append(("queststeps", {"questid": SCALE_QUEST, "step": SCALE_STEP}, {"factions": SCALE_NOW}))
    lines.append(f"queststeps {SCALE_QUEST}/{SCALE_STEP} (The Scale of Veeshan): {SCALE_WAS} -> {SCALE_NOW}")
    # the records
    regen = tr.get(db, "itempattern", {"patternid": REGEN})
    leo = tr.get(db, "itempattern", {"patternid": LEOPARD_ELEGANCE})
    if not regen or regen["itemname"] != "Flawless Regenerative Belt":
        refusals.append(f"{REGEN} is not the Flawless Regenerative Belt")
    if not leo or leo["itemname"] != "Leopard's Elegance":
        refusals.append(f"{LEOPARD_ELEGANCE} is not Leopard's Elegance's scroll")
    if not tr.rows(db, "SELECT 1 FROM spellpattern WHERE spellid = ? AND spellname = ?", (MINO_SPELL, "Minotaur's Persistence")):
        refusals.append("no spell Minotaur's Persistence here")
    recs = [dict(STORMSOUL), dict(REJUV)]
    if regen and leo:
        recs.append(dict(regen, **CATHARTIC))
        recs.append(dict(regen, **RESTOR))
        recs.append(dict(leo, **MINO_SCROLL))
    for r in recs:
        have = tr.get(db, "itempattern", {"patternid": r["patternid"]})
        if have and have["itemname"] != r["itemname"]:
            refusals.append(f"pattern {r['patternid']} is another record here: {have['itemname']!r}")
        dup = tr.rows(db, "SELECT patternid FROM itempattern WHERE itemname = ? AND patternid != ?", (r["itemname"], r["patternid"]))
        if dup:
            refusals.append(f"another {r['itemname']} record here: {dup}")
        out.append(("itempattern", {"patternid": r["patternid"]}, r))
        lines.append(f"itempattern {r['patternid']} {r['itemname']}")
    names = sorted(n for n in owed() if n not in online)
    out += backpay(db, names, refusals, lines, LOG)
    for n in sorted(set(owed()) & set(online)):
        lines.append(f"back-pay {n}: in the world, left to --late {n} at the logout")
    return out, lines, refusals


def changes_late(db, name):
    out, lines, refusals = [], [], []
    check_live(db, refusals)
    if name not in owed():
        refusals.append(f"{name} is owed nothing here")
        return out, lines, refusals
    out = backpay(db, [name], refusals, lines, late_log(name))
    return out, lines, refusals


def changes_shop(db):
    out, lines, refusals = [], [], []
    check_live(db, refusals)
    for shop in (MERCHANT, LUXURY):
        if not tr.rows(db, "SELECT 1 FROM npcs WHERE npc_name = ? AND world = 0", (shop,)):
            refusals.append(f"no {shop} here")
        if tr.rows(db, "SELECT 1 FROM npcitem i JOIN npcs n ON n.id = i.npcid WHERE n.npc_name = ? LIMIT 1", (shop,)):
            refusals.append(f"{shop} has a tunar shop (npcitem): the talk would be the token shop's")
    for shop, pid, name, price, says in SHOP:
        rec = tr.get(db, "itempattern", {"patternid": pid})
        if not rec or rec["itemname"] != name:
            refusals.append(f"{pid} is not {name} here (run the data part first): {rec and rec['itemname']}")
        out.append(("tokenshop", {"shop": shop, "patternid": pid},
                    {"shop": shop, "patternid": pid, "price": price, "token": TOKEN, "says": says, "sort": 0,
                     "source": "Live petition 5575: eqoa.forumotion.com t306"}))
        lines.append(f"{shop}: {name} ({pid}) for {price} {FACTION}")
    return out, lines, refusals


def spec_for(part, arg):
    def wrap(f):
        def changes(db):
            out, lines, refusals = f(db)
            applied = all(tr.get(db, t, k) is not None and all(tr.get(db, t, k).get(c) == v for c, v in a.items())
                          for t, k, a in out)
            lines.insert(0, f"{len(out)} rows")
            return out, lines, refusals, applied
        return changes
    if part == "shop":
        return {"log": "petpass_live_1124r_shop_log", "detail": DETAIL + " [shop]",
                "needs": ("tokenshop", "itempattern", "npcs", "npcitem", "factions"), "changes": wrap(changes_shop)}
    if part == "late":
        return {"log": late_log(arg), "detail": DETAIL + f" [late {arg}]",
                "needs": ("charfactions", "characters", "factions", "npcs"), "changes": wrap(lambda db: changes_late(db, arg))}
    online = {n.strip() for n in arg.split(",") if n.strip()}
    return {"log": LOG, "detail": DETAIL,
            "needs": ("factionhits", "queststeps", "itempattern", "spellpattern", "charfactions", "characters",
                      "factions", "npcs"),
            "changes": wrap(lambda db: changes_data(db, online))}


def main(argv):
    part, arg, rest = "data", "", []
    i = 0
    while i < len(argv):
        a = argv[i]
        if a == "--shop":
            part = "shop"
        elif a in ("--late", "--online"):
            if i + 1 >= len(argv):
                sys.exit(f"{a} needs a value")
            part = "late" if a == "--late" else part
            arg = argv[i + 1]
            i += 1
        else:
            rest.append(a)
        i += 1
    return tr.main(spec_for(part, arg), argv=rest, doc=__doc__)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
