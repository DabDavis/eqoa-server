"""An item record's fields as a dict (net::dump::Item's VIItem fields) and the
itempattern columns each fills, in writeItemPattern's order; item_row maps one
such record to its itempattern row."""

# The record's VIItem fields (net::dump::Item) and the itempattern column each
# fills, in writeItemPattern's order.
ITEM_FIELDS = [
    ("id", "patternid"), ("value", "itemcost"), ("p08", "unk1"), ("icon", "itemicon"),
    ("icon2", "unk2"), ("wear_slot", "equipslot"), ("p18", "unk3"), ("no_trade", "trade"),
    ("rent", "rent"), ("p24", "unk4"), ("attack_type", "attacktype"), ("damage", "weapondamage"),
    ("range", "unk5"), ("level_req", "levelreq"), ("max_stack", "maxstack"), ("max_hp", "maxhp"),
    ("durability", "duration"), ("class_mask", "classuse"), ("race_mask", "raceuse"),
    ("p4c", "procanim"), ("lore", "lore"), ("p58", "unk6"), ("p54", "craft"),
    ("name", "itemname"), ("desc", "itemdesc"),
]
# Stat index -> column (itemPatternStats), and 28 -> fish.
STAT_COLUMNS = {
    0: "str", 1: "sta", 2: "agi", 3: "dex", 4: "wis", 5: "intelligence", 6: "cha",
    8: "HPMAX", 10: "POWMAX", 12: "PoT", 13: "HoT", 14: "AC",
    22: "PR", 23: "DR", 24: "FR", 25: "CR", 26: "LR", 27: "AR", 28: "fish",
}


def item_row(w):
    """The itempattern columns a record fills, and the stat indices it drops."""
    row = {col: w[key] for key, col in ITEM_FIELDS}
    dropped = []
    stats = {c: None for c in STAT_COLUMNS.values()}
    for idx, val in w["stats"]:
        col = STAT_COLUMNS.get(idx)
        if col is None:
            dropped.append(idx)
        else:
            stats[col] = val
    row.update(stats)
    return row, dropped
