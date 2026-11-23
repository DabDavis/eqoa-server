"""Stage the Heirloom Thanksgiving content on a server database. Dry run is the default."""
import argparse
import json
from pathlib import Path
import sqlite3

HERE = Path(__file__).resolve().parent
EVENT = json.loads((HERE / "event.json").read_text(encoding="utf-8"))
SOURCE = EVENT["source"]
RAID = EVENT["raid"]


def insert(db, table, **row):
    names = ", ".join('"' + key + '"' for key in row)
    values = [json.dumps(v, separators=(",", ":")) if isinstance(v, (list, dict)) else v for v in row.values()]
    db.execute(f"INSERT INTO {table} ({names}) VALUES ({','.join('?' for _ in row)})", values)


def one(db, table, column, value):
    row = db.execute(f"SELECT * FROM {table} WHERE {column}=?", (value,)).fetchone()
    if row is None:
        raise ValueError(f"Missing donor {table}.{column}={value}")
    return dict(row)


def scene():
    words = EVENT["scene"]
    king, santa, grinch = (words[k] for k in ("king", "santa", "grinch"))
    def spawn(actor, place):
        x, y, z = EVENT["positions"][place]
        return dict(do="spawn", npc=actor, world=0, x=x, y=y, z=z, spread=0.2, minutes=62,
                    untouchable=True, noloot=(actor != king), face="east" if actor == king else "west")
    def strike(who, target, damage=0, finish=False):
        return dict(do="scenestrike", npc=who, target=target, count=damage, dead=finish)
    def say(who, line):
        return dict(do="say", npc=who, text=words[line])
    def wait(sec):
        return dict(do="wait", seconds=sec)
    return [
        dict(do="lock", lock="thanksgiving west gate"), dict(do="clock", seconds=3600, empty=300),
        spawn(king, "arena"), dict(do="shout", npc=king, text=words["shout"]), wait(3),
        spawn(santa, "santa"), spawn(grinch, "grinch"), wait(1),
        strike(santa, king, 2400), wait(1), strike(grinch, king, 2400), wait(1),
        strike(king, santa, 12500), wait(1), strike(king, grinch, 12500), wait(2),
        strike(king, santa), strike(king, grinch), wait(2), say(santa, "apology"), wait(5),
        say(king, "pardon"), wait(4), say(grinch, "insult"), wait(3),
        strike(king, grinch, finish=True), dict(do="despawn", npc=santa), wait(2),
        say(king, "turn"), wait(4), dict(do="shout", npc=king, text=words["attack"]),
        dict(do="rouse", npc=king), dict(do="awaitdead", npc=king, seconds=3540, fail="The King has withdrawn. Cross the west gate again to challenge him."),
        spawn(santa, "santa"), say(santa, "after"), wait(12), dict(do="end"), dict(do="despawn")
    ]


def install(db):
    db.row_factory = sqlite3.Row
    # The season and the existing quest are Heirloom's actual Christmas content.
    if one(db, "quests", "questid", 102908)["name"] != "The Grinch and the Lost Years":
        raise ValueError("Expected Heirloom Grinch chain 102908")
    if one(db, "npcs", "id", 190715)["npc_name"] != "Santa Claus":
        raise ValueError("The tree Santa has changed")
    for name in ("Turkey King", "Leftovers", "Santa Claus of Freeport", *(a[1] for a in EVENT["actors"][1:6])):
        if db.execute("SELECT 1 FROM npcs WHERE npc_name=? COLLATE NOCASE", (name,)).fetchone():
            raise ValueError("Existing NPC: " + name)
    insert(db, "gmseasons", name=EVENT["season"], active=0, note=SOURCE)
    for id_, name, donor, world, x, y, z, size, role in EVENT["actors"]:
        row = one(db, "npcs", "id", donor)
        row.update(id=id_, npc_id=id_, npc_name=name, world=world, x_coord=x, y_coord=y, z_coord=z, size=size,
                   zone=0, facing=128, npc_type=0 if name=="Leftovers" else 128, npc_level=64 if role=="prototype" or id_==197601 else 30,
                   primary_weapon=0, secondary_weapon=0, shield=0, dt_kill=None)
        insert(db, "npcs", **row)
        if role=="prototype":
            insert(db, "npcspawnonly", name=name.lower(), note=SOURCE, source=SOURCE)
        else:
            insert(db, "npcseason", name=name.lower(), season=EVENT["season"], source=SOURCE)
        insert(db, "npcmove", name=name.lower(), roam=0, radius=0, source=SOURCE)
    for name, lines in EVENT["dialogs"].items():
        for line in lines:
            insert(db, "npcdialog", name=name, line=line, source=SOURCE)
    for record in EVENT["items"]:
        row = one(db, "itempattern", "patternid", record["base"])
        row.update(patternid=record["id"], itemname=record["name"], itemdesc=record["description"], trade=1,
                   rent=1, lore=record["lore"], maxstack=record["stack"], itemcost=0, levelreq=0, weaponproc=0)
        if "visual_from" in record:
            row["itemicon"] = one(db, "itempattern", "patternid", record["visual_from"])["itemicon"]
        else:
            row.update(equipslot=14, attacktype=3, weapondamage=790, classuse=32767, raceuse=1023)
            for stat in ("str","sta","agi","wis","dex","cha","intelligence","HPMAX","POWMAX","PoT","HoT","AC","PR","DR","FR","CR","LR","AR"):
                row[stat] = 0
        insert(db, "itempattern", **row)
    debuff = RAID["debuff"]
    spell = one(db, "spellpattern", "spellid", 906814)
    spell.update(spellid=debuff["id"], spellname=debuff["name"], spelldesc="Lowers Strength and Dexterity by 200 for ten minutes.",
                 recast=60, casttime=0, power=0, scope=1)
    insert(db, "spellpattern", **spell)
    effect = one(db, "spelleffect", "spellid", 906814)
    effect.update(spellid=debuff["id"], line="Target, Decrease STR by 200, Decrease DEX by 200, 10min",
                  source=SOURCE, effects={"v":1,"scope":"target","harmful":True,"duration_s":600,
                            "effects":[dict(kind="stat",stat=s,amount=-200) for s in ("str","dex")]})
    insert(db, "spelleffect", **effect)
    insert(db, "spellarea", spellid=debuff["id"], centre=2, radius=debuff["radius"], maxtargets=0, note=SOURCE)
    profile = one(db, "classtemplates", "name", RAID["melee_from"])
    if profile["class"] != 0 or profile["melee_pct"]<=0:
        raise ValueError("QoV warrior melee template missing")
    summons = RAID["adds"]
    calls = [dict(kind="summon", npc="Leftovers", share=summons["damage_percent"], count=summons["count"], below=p)
             for p in summons["thresholds"]]
    for name, abilities in [("turkey king", [dict(kind="debuff", spell_id=debuff["id"], first=1, every=60), *calls]), ("leftovers", [])]:
        row = dict(profile, name=name, hp_pct=100, abilities=abilities, note="Thanksgiving raid", source=SOURCE)
        if name=="leftovers":
            row.update(special="", special_chance=0, special_every=0, special_first=0, special_jitter=0,
                       special_from=0, special_damage=0, special_fixed=0, kick_damage=0, kick_every=0,
                       kick_first=0, shield_damage=0, proc_chance=0, fight_regen=0)
        insert(db, "classtemplates", **row)
        insert(db, "npctemplate", name=name, template=name, source=SOURCE)
    # Only this scene Santa uses the original Grinch's template, by reference.
    insert(db, "npctemplate", name="santa claus of freeport", template=RAID["santa_from"], source=SOURCE)
    insert(db, "raidbosslist", npc_name="turkey king", note=SOURCE, source=SOURCE)
    insert(db, "lootcreature", npc_name="turkey king", region="", profile="-", source=SOURCE)
    for index in range(RAID["stuffing"]):
        insert(db, "npcitem", npcid=197601, patternid=1000811, stackleft=1, remainHP=0, remaincharge=0,
               equiploc=-1, location=1, listnumber=index)
    insert(db, "seasonrewards", season=EVENT["season"], item=1000810, drop_pct=0, named_pct=0,
           gift_npc="", gift_count=0, hourly=0, note="One independent armor roll per eligible Envar corpse.", source=SOURCE)
    a=EVENT["armor_sources"]
    db.execute("CREATE TABLE thanksgiving_envar_trash (spawn INTEGER PRIMARY KEY, name TEXT NOT NULL)")
    db.execute("""INSERT INTO thanksgiving_envar_trash SELECT id,npc_name FROM npcs n
        WHERE world=? AND x_coord BETWEEN ? AND ? AND z_coord BETWEEN ? AND ? AND (npc_type & 128)=0
        AND lower(npc_name) IN (""" + ",".join("?" for _ in a["names"]) + """)
        AND NOT EXISTS (SELECT 1 FROM raidbosslist r WHERE lower(r.npc_name) IN (lower(n.npc_name),'#'||n.id))""",
        [a["world"], *a["bounds"], *a["names"]])
    if not db.execute("SELECT count(*) FROM thanksgiving_envar_trash").fetchone()[0]:
        raise ValueError("No reviewed Envar trash spawns")
    for quest in EVENT["quests"]:
        insert(db, "quests", questid=quest["id"], name=quest["title"], enabled=0, minlevel=1,maxlevel=60,
               giver=quest["giver"], offer=quest["offer"], accept=quest["accept"], decline="Another time.",
               started=quest["started"], repeatable=quest.get("repeatable",0), marks=0, source=SOURCE)
        for number, definition in enumerate(quest["steps"],1):
            step=dict(definition)
            also=step.pop("also",[])
            insert(db, "queststeps", questid=quest["id"], step=number, **step)
            for name in also: insert(db, "queststeptargets", questid=quest["id"], step=number, target=name)
    x,y,z=EVENT["positions"]["arena"]
    insert(db, "gmtriggers", slot=340,name=EVENT["title"],enabled=0,scope="near",radius=140,world=0,x=x,y=y,z=z,
           steps=scene(),notes=SOURCE)
    for slot,turn in [(341,"on"),(342,"off")]:
        insert(db, "gmtriggers", slot=slot,name="Thanksgiving "+turn,enabled=1,scope="gm",world=0,x=x,y=y,z=z,
               steps=[dict(do="season",season="Thanksgiving",turn=turn)],notes=SOURCE)
    # executescript commits an open transaction. Execute one complete statement at a time.
    statement = ""
    for line in (HERE/"transactions.sql").read_text(encoding="utf-8").splitlines(True):
        statement += line
        if sqlite3.complete_statement(statement):
            db.execute(statement)
            statement = ""
    if statement.strip(): raise ValueError("Incomplete seasonal trigger")


if __name__=="__main__":
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("database",type=Path)
    parser.add_argument("--apply",action="store_true",help="commit the staged, season-off content")
    args=parser.parse_args()
    db=sqlite3.connect(args.database.resolve().as_uri()+"?mode=rw",uri=True)
    try:
        db.execute("PRAGMA foreign_keys=ON")
        db.execute("BEGIN IMMEDIATE")
        install(db)
        n=db.execute("SELECT count(*) FROM thanksgiving_envar_trash").fetchone()[0]
        if args.apply: db.commit()
        else: db.rollback()
        print(f"{'Staged season OFF' if args.apply else 'Dry run rolled back'}: 3 quests, 8 NPCs, {n} armor sources, 20 Stuffing.")
    finally:
        db.close()
