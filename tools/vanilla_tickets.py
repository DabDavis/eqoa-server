"""Route EQOA ticket claims and prepare explicit changes for one destination."""
import argparse
import calendar
import contextlib
import csv
import datetime
import hashlib
import json
import re
import sqlite3
import sys
from pathlib import Path


class ReviewError(ValueError):
    pass


TARGETS = {"live", "heirloom"}
BASES = {"era", "decomp", "shared", "owner", "estimate", "test"}
LANES = {"retail", "shared-fix", "heirloom-only", "live-policy", "operations", "unreviewed"}
REVIEWED = {"certified", "reviewed-shared", "reviewed-policy"}
DIRECT = {"soe", "original-recording", "original-disc", "official-guide", "era-database", "era-guide"}
TICKET = re.compile(r"(?:work|(?:heirloom|live):petition):[1-9][0-9]*")
AUDIT_CASE = re.compile(r"audit:[0-9]{8}:(?:C[0-9]{2}|G[1-9][0-9]*)")
IDENT = re.compile(r"[A-Za-z_][A-Za-z_0-9]*")
FORMAT = "eqoa-reviewed-ticket/1"
RELEASES = "vanilla_ticket_release"
CUSTOM_TABLES = {"heirlooms", "itemascended", "spellascended", "itemimported", "heroes_stock", "familiars"}
LATER_SERVER = re.compile(r"(?<![\w.-])(?:[\w-]+\.)*(?:eqoa\.live|eqoaemu\.com)(?=[:/?#\s]|$)", re.I)
IDENTITY_FIELDS = {"itempattern": {"itemname"}, "spellpattern": {"spellname"}, "quests": {"name", "giver"},
                   "npcs": {"npc_name", "world", "x_coord", "z_coord"}}
REFERENCE = {"item": ("itempattern", "patternid"), "spell": ("spellpattern", "spellid"),
             "quest": ("quests", "questid"), "npc": ("npcs", "id")}


def require(test, message):
    if not test:
        raise ReviewError(message)


def pairs(entries):
    obj = {}
    for key, value in entries:
        require(key not in obj, f"duplicate JSON field {key}")
        obj[key] = value
    return obj


def load(path):
    def bad_number(value):
        raise ReviewError(f"invalid JSON number {value}")
    return json.loads(Path(path).read_text(encoding="utf-8-sig"), object_pairs_hook=pairs, parse_constant=bad_number)


def canonical(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"), allow_nan=False)


def digest(claim):
    facts = {k: v for k, v in claim.items() if k not in {"review", "approval", "observation"}}
    return hashlib.sha256(canonical(facts).encode("utf-8")).hexdigest()


def nonblank(value):
    return isinstance(value, str) and bool(value.strip())


def config_binding(path):
    """Read the two plain or quoted scalars in the server's deployed block-style YAML."""
    values, child_indent, section = {}, None, None
    scalar = re.compile(r"""("[^"\\]*"|'(?:[^']|'')*'|[A-Za-z0-9_./:\\-]+)(?:\s+#.*)?$""")
    for line in path.read_text(encoding="utf-8-sig").splitlines():
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        indent = len(line) - len(line.lstrip(" "))
        key, sep, value = line.strip().partition(":")
        if not indent:
            section = key if sep else None
            child_indent = None
        elif child_indent is None:
            child_indent = indent
        # The deployed configs keep db_path under server: (internal/config); a top-level one is accepted too.
        if not ((not indent and key == "db_path") or (section == "server" and indent == child_indent and key == "db_path")
                or (section == "gm" and indent == child_indent and key == "shard_id")):
            continue
        match = scalar.fullmatch(value.strip())
        require(sep and match and key not in values, f"invalid or repeated {key} in shard config")
        value = match[1]
        values[key] = value[1:-1].replace("''", "'") if value[:1] in {"'", '"'} else value
    require(values.get("shard_id") in TARGETS and nonblank(values.get("db_path")), "config needs db_path and gm.shard_id")
    return values["shard_id"], (path.parent / values["db_path"]).resolve()


def bound_shard(db, config=None):
    path = Path(db.execute("PRAGMA database_list").fetchone()[2]).resolve()
    cfg = Path(config).resolve() if config else path.parent / "config.yaml"
    if config or cfg.is_file():
        shard, expected = config_binding(cfg)
        if path == expected:
            return shard
        require(not config, "destination database differs from the explicit server config")
    require(db.execute("SELECT 1 FROM sqlite_master WHERE type='table' AND name='shard_label'").fetchone(),
            "unbound database: use its server config or label the rehearsal copy at creation")
    labels = db.execute("SELECT shard FROM shard_label LIMIT 2").fetchall()
    require(len(labels) == 1 and labels[0][0] in TARGETS, "invalid rehearsal copy shard_label")
    return labels[0][0]


def ledger(path, *, allow_audit=False):
    data = load(path)
    require(isinstance(data, dict) and data.get("version") == 1 and isinstance(data.get("claims"), list),
            "expected a version 1 ticket ledger")
    seen = set()
    for c in data["claims"]:
        require(isinstance(c, dict) and isinstance(c.get("id"), str), "claim needs an id")
        base, sep, part = c["id"].partition("/")
        require((TICKET.fullmatch(base) or (allow_audit and AUDIT_CASE.fullmatch(base))) and sep
                and re.fullmatch(r"[a-z0-9][a-z0-9-]*", part),
                f"invalid shard-qualified petition or shared work id: {c['id']}")
        require(c["id"] not in seen and c.get("lane") in LANES and nonblank(c.get("claim")),
                f"duplicate or invalid claim: {c['id']}")
        require(c.get("target") in TARGETS or (c["lane"] == "unreviewed" and c.get("target") is None),
                f"{c['id']} needs a destination")
        lane_target = {"heirloom-only": "heirloom", "live-policy": "live"}.get(c["lane"])
        require(not lane_target or c["target"] == lane_target, f"{c['id']} belongs on {lane_target}")
        if "basis_ref" in c:
            ref = c["basis_ref"]
            require(nonblank(ref) and len(ref) <= 120 and len(ref.splitlines()) == 1,
                    "basis_ref needs one short reference; keep the underlying evidence at its source")
        for field in ("conflicts", "holds"):
            if field in c:
                require(isinstance(c[field], list) and all(nonblank(v) for v in c[field]),
                        f"{c['id']} needs a list for {field}")
        seen.add(c["id"])
    return data


def overlay_rules(data):
    permanent = load(Path(__file__).with_name("shard_overlays.json"))
    require(permanent.get("version") == 1 and isinstance(permanent.get("overlays"), list), "invalid shard policy")
    extra = data.get("overlays", [])
    require(isinstance(extra, list), "invalid ledger overlays")
    rules = list(permanent["overlays"])
    vendors = next(r["where"]["npc_name"] for r in rules if r["target"] == "both" and r["table"] == "npcs")
    excluded = permanent["live_excluded"]
    names = set(excluded["names"])
    for group in permanent.get("transfer_scope", {}).values():
        if group["scope"] == "heirloom-only":
            names.update(group.get("identities", []))
    rules.append({"target": "live", "table": "*", "fields": ["*"], "where": {"identity": sorted(names)},
                  "reason": "Heirloom-only identity or season"})
    for table in ("quests", "queststeps", "queststeptargets", "questunlocks", "questwalls", "questmarks"):
        rules.append({"target": "live", "table": table, "fields": ["*"], "where": {"questid": excluded["quest_ids"]},
                      "reason": "Heirloom holiday quest"})
    for table, columns in permanent["npc_links"].items():
        for column in columns:
            for target, actors in (("both", vendors), ("live", sorted(names))):
                if any(r.get("npc_from") == column and r["table"] == table and r["target"] == target for r in rules):
                    continue
                rules.append({"target": target, "table": table, "fields": ["*"], "where": {"npc_name": actors},
                              "npc_from": column, "reason": "shard-owned NPC dependency"})
    rules.extend(extra)
    for rule in rules:
        require(isinstance(rule, dict) and rule.get("target") in TARGETS | {"both"} and nonblank(rule.get("table"))
                and isinstance(rule.get("where"), dict) and bool(rule["where"])
                and isinstance(rule.get("fields"), list) and bool(rule["fields"])
                and all(nonblank(f) for f in rule["fields"]) and nonblank(rule.get("reason")), "invalid shard overlay")
    return rules


def era_date(value):
    """The dated original observation can be a day, month or year. Preserve its precision."""
    if not isinstance(value, str) or not re.fullmatch(r"\d{4}(?:-\d{2}(?:-\d{2})?)?", value):
        return False
    parts = [int(p) for p in value.split("-")]
    try:
        year = parts[0]
        month = parts[1] if len(parts) > 1 else 1
        first = datetime.date(year, month, parts[2] if len(parts) == 3 else 1)
        end_month = month if len(parts) > 1 else 12
        last = first if len(parts) == 3 else datetime.date(year, end_month, calendar.monthrange(year, end_month)[1])
        return first >= datetime.date(2003, 1, 1) and last <= datetime.date(2012, 3, 29)
    except ValueError:
        return False


def signed(record, fingerprint):
    if not isinstance(record, dict) or record.get("digest") != fingerprint:
        return False
    try:
        datetime.date.fromisoformat(record.get("date", ""))
    except (TypeError, ValueError):
        return False
    return nonblank(record.get("by")) and nonblank(record.get("ref"))


def change_basis(c):
    """Old retail ledgers already contain their evidence. Do not duplicate it or change their digests."""
    if "basis" in c:
        return c["basis"]
    if c["lane"] == "retail":
        return "decomp" if c.get("sources") and all(
            s.get("kind") == "original-disc" for s in c["sources"]) else "era"
    return None


def basis_reference(c):
    if c.get("basis_ref"):
        return c["basis_ref"]
    # The release already stores the claim ID and digest. This is a pointer within that exact revision.
    return "sources" if c["lane"] == "retail" and c.get("sources") else None


def verify_basis_record(db, c, board_db=None, releasing=False):
    """Shared work numbers live on Heirloom. Local board and audit numbers name their shard."""
    ref = c.get("basis_ref", "")
    board = re.fullmatch(r"(?:board |(heirloom|live):board[: ])([1-9][0-9]*)(?: v([1-9][0-9]*))?", ref, re.I)
    if re.match(r"^(?:(?:heirloom|live):)?board\b", ref, re.I):
        require(board, "invalid work-board basis reference")
    if board:
        source, number, version = board.groups()
        source = (source or "heirloom").casefold()
        require(not releasing or version, "pin the board version in basis_ref before releasing")
        sql = "SELECT basis,basis_ref,version FROM workboard WHERE id=?"
        if source == c["target"]:
            row = db.execute(sql, (int(number),)).fetchone()
        else:
            require(source == "heirloom", "a Live-local board reference belongs to Live")
            require(board_db, "shared board reference on Live requires --board-db for Heirloom")
            uri = Path(board_db).resolve().as_uri() + "?mode=ro"
            with contextlib.closing(sqlite3.connect(uri, uri=True)) as shared:
                shared.execute("BEGIN")
                require(bound_shard(shared) == "heirloom", "--board-db must be bound to Heirloom's shared board")
                row = shared.execute(sql, (int(number),)).fetchone()
        require(row and row[0] == change_basis(c) and nonblank(row[1]), "board basis differs or lacks a reference")
        require(not version or row[2] == int(version), "board basis version changed")
    audit = re.fullmatch(r"(live|heirloom):audit:([1-9][0-9]*)", ref, re.I)
    if re.match(r"^(?:live|heirloom):audit\b", ref, re.I):
        require(audit, "invalid shard audit basis reference")
    if audit:
        require(audit[1].casefold() == c["target"], "basis audit belongs to the other shard")
        row = db.execute("SELECT detail FROM gm_audit WHERE id=?", (int(audit[2]),)).fetchone()
        tag = re.match(r"\[basis:([a-z]+)(?: [^\]]*)?\]", row[0] or "") if row else None
        require(tag and tag[1] == change_basis(c), "audit basis differs or is missing")


def assess(c):
    """Certification is about a bounded service-era claim. Permission is a separate decision."""
    fp = digest(c)
    if nonblank(c.get("superseded_by")):
        return "superseded", "archived"
    lane = c["lane"]
    basis = change_basis(c)
    if lane in {"shared-fix", "heirloom-only", "live-policy"} and basis:
        allowed = {"shared"} if lane == "shared-fix" else {"owner", "estimate"}
        if basis not in allowed or not nonblank(c.get("basis_ref")):
            return "needs-basis", "hold"
        history = "reviewed-shared" if lane == "shared-fix" else "reviewed-policy"
        return release_route(c, fp, history)
    if lane != "retail":
        return "unreviewed" if lane == "unreviewed" else "outside-retail", lane
    if basis not in {"era", "decomp"}:
        return "needs-basis", "hold"
    sources = c.get("sources", [])
    require(isinstance(sources, list) and all(isinstance(s, dict) for s in sources), "invalid sources")
    dated = [s for s in sources if era_date(s.get("as_of")) and
             all(nonblank(s.get(k)) for k in ("locator", "supports", "lineage")) and not LATER_SERVER.search(s["locator"])]
    direct = any(s.get("kind") in DIRECT for s in dated)
    witnesses = [(s["lineage"].strip().casefold(), s["locator"].strip().casefold())
                 for s in dated if s.get("kind") == "era-testimony"]
    independent = any(a != b and x != y for a, x in witnesses for b, y in witnesses)
    if not nonblank(c.get("period")) or not (direct or independent):
        return "needs-evidence", "hold"
    if basis == "decomp" and not any(s.get("kind") == "original-disc" for s in dated):
        return "needs-evidence", "hold"
    return release_route(c, fp, "certified")


def release_route(c, fp, history):
    if c.get("conflicts") != []:
        return "needs-resolution", "hold"
    if not signed(c.get("review"), fp):
        return "needs-review", "hold"
    if c.get("holds") != []:
        return history, "held"
    if c.get("delivery") == "code":
        patch = c.get("patch", {})
        if not (isinstance(patch, dict) and re.fullmatch(r"[0-9a-f]{40}", str(patch.get("commit", "")))
                and nonblank(patch.get("scope")) and nonblank(patch.get("tests"))):
            return history, "needs-code-packet"
        return history, "owner-code-review"
    if c.get("delivery") != "data" or not isinstance(c.get("changes"), list) or not isinstance(c.get("checks"), list):
        return history, "needs-change-packet"
    if not c["changes"]:
        return history, "verify-only" if c["checks"] else "needs-change-packet"
    return history, "approved" if signed(c.get("approval"), fp) else "needs-approval"


def get_claim(data, claim_id, target):
    c = next((entry for entry in data["claims"] if entry["id"] == claim_id), None)
    require(c is not None, f"unknown claim {claim_id}")
    require(c["target"] == target, f"{claim_id} does not target {target}")
    return c


def competing_claims(data):
    """Find incompatible certified intents at the same destination field, including a row deletion."""
    owners, conflicts = {}, {}
    for c in data["claims"]:
        if assess(c)[0] not in REVIEWED or c.get("delivery") != "data":
            continue
        changes = c.get("changes")
        if not isinstance(changes, list):
            continue
        for op in changes:
            if not (isinstance(op, dict) and nonblank(op.get("table")) and isinstance(op.get("key"), dict)
                    and "after" in op and (op["after"] is None or isinstance(op["after"], dict))):
                continue
            row = (c["target"], op["table"], canonical(op["key"]))
            desired = op["after"]
            for other, other_value in owners.get(row, []):
                if desired is None or other_value is None:
                    differs = desired != other_value
                else:
                    differs = any(canonical(desired[k]) != canonical(other_value[k]) for k in desired.keys() & other_value.keys())
                if differs and other != c["id"]:
                    conflicts.setdefault(c["id"], set()).add(other)
                    conflicts.setdefault(other, set()).add(c["id"])
            owners.setdefault(row, []).append((c["id"], desired))
    return {k: ", ".join(sorted(v)) for k, v in conflicts.items()}


def receipt(c):
    return {"format": FORMAT, "claim": c["id"], "target": c["target"], "digest": digest(c)}


def lookup(db, spec, tables, heirloom_feature=False):
    """Resolve a real primary key on the destination. Never substitute a source row ID."""
    t, key = spec.get("table"), spec.get("key")
    require(t in tables and (heirloom_feature or t not in CUSTOM_TABLES), f"{t} is outside retail content")
    schema = db.execute(f'PRAGMA table_info("{t}")').fetchall()
    columns = {r[1] for r in schema}
    pk = {r[1] for r in schema if r[5]}
    require(isinstance(key, dict) and bool(key) and set(key) == pk, f"{t} needs its complete destination primary key")
    require(all(IDENT.fullmatch(k) and isinstance(v, (int, float, str)) and not isinstance(v, bool)
                for k, v in key.items()), f"{t} has an invalid key")
    where = " AND ".join(f'"{k}" = ?' for k in key)
    cur = db.execute(f'SELECT * FROM "{t}" WHERE {where}', tuple(key.values()))
    row = cur.fetchone()
    return columns, dict(zip([d[0] for d in cur.description], row)) if row is not None else None


def subset(row, wanted):
    return row is not None and all(k in row and canonical(row[k]) == canonical(v) for k, v in wanted.items())


def valid_values(values, columns):
    return isinstance(values, dict) and values.keys() <= columns and all(
        v is None or (type(v) in {int, float, str}) for v in values.values())


def matches_overlay(db, row, rule, npc_names):
    if row is None:
        return False
    if rule["table"] == "*":
        excluded = {s.replace("’", "'").casefold() for s in rule["where"]["identity"]}
        if protected_identity(row, excluded):
            return True
        return any(re.search(r"\b(?:christmas|thanksgiving)\b", str(row.get(k, "")), re.I)
                   for k in ("season", "source", "npc_name", "name", "itemname", "spellname", "tablename"))
    if "npc_from" in rule:
        col = rule["npc_from"]
        require(col in row, f"unknown NPC reference {col} in shard overlay")
        ref = row[col]
        if isinstance(ref, int) or (isinstance(ref, str) and re.fullmatch(r"#\d+", ref)):
            ident = ref if isinstance(ref, int) else int(ref[1:])
            if ident == 0:
                return False
            ref = npc_names.get(ident)
            if ref is None:
                result = db.execute("SELECT npc_name FROM npcs WHERE id=?", (ident,)).fetchone()
                require(result is not None, f"unresolved destination NPC {ident} in shard overlay")
                ref = result[0]
        row = {"npc_name": ref}
    for key, expected in rule["where"].items():
        require(key in row, f"unknown field {key} in shard overlay")
        options = expected if isinstance(expected, list) else [expected]
        value = row[key]
        if isinstance(value, str):
            value = value.strip().casefold()
            options = [x.strip().casefold() if isinstance(x, str) else x for x in options]
        if value not in options:
            return False
    return True


def changes_progress(table, before, after):
    if table == "quests":
        return after is None
    if table == "queststeptargets":
        return True
    return table == "queststeps" and (before is None or after is None or
           any(before.get(k) != after.get(k) for k in ("kind", "target", "items")))


def check_progress(db, quest_ids):
    for q in quest_ids:
        busy = db.execute("SELECT 1 FROM charquests WHERE questid=? AND done=0 LIMIT 1", (q,)).fetchone()
        require(not busy, f"quest {q} has active progress; hold the whole ticket")


def check_races(target, table, before, after, fields):
    if before is None or after is None:
        return
    earlier, later = (before, after) if target == "heirloom" else (after, before)
    if table == "itempattern" and "raceuse" in fields:
        def mask(row):
            value = row["raceuse"]
            require(type(value) is int, "invalid item race mask")
            if value == 0:
                return -1  # equip accepts any race
            return value | 1024 if value & 1023 == 1023 else value  # raceUseForWire
        allowed = mask(earlier) & ~mask(later) == 0
    elif table == "quests" and "races" in fields:
        def race_set(row):
            value = row["races"] or ""
            require(isinstance(value, str) and re.fullmatch(r"[0-9,\s|]*", value), "invalid quest race list")
            return set(re.findall(r"[0-9]+", value))
        a, b = race_set(earlier), race_set(later)
        allowed = not b or bool(a) and a <= b  # empty means any race
    else:
        return
    require(allowed, f"protected race eligibility on {target}: {table}")


def embedded_refs(column, kind, value):
    """Read IDs in the game's item-count, integer-list and structured quest formats."""
    if value is None or value == "":
        return
    if column in {"events", "steps", "choices", "byclass", "ingredients", "abilities"}:
        data = json.loads(value, object_pairs_hook=pairs)
        if data is None and column == "abilities":
            return
        if column in {"events", "steps", "ingredients", "abilities"}:
            entries = data
        elif column == "choices":
            require(isinstance(data, dict), "invalid quest choices")
            entries = data.get("options")
        else:
            require(isinstance(data, dict), "invalid byclass rewards")
            entries = list(data.values())
        require(isinstance(entries, list), f"invalid encoded {column}")
        for entry in entries:
            require(isinstance(entry, dict), f"invalid encoded {column} entry")
            if column == "ingredients":
                ident = entry.get("item")
                require(type(ident) is int and ident > 0, "invalid recipe ingredient")
                yield "item", ident
                continue
            if column == "abilities":
                ident = entry.get("spell_id", 0)
                require(type(ident) is int and ident >= 0, "invalid template spell_id")
                if ident:
                    yield "spell", ident
                npc = entry.get("npc")
                if isinstance(npc, str) and re.fullmatch(r"#[0-9]+", npc):
                    yield "npc", int(npc[1:])
                continue
            for field, ref_kind in (("items", "itext"), ("spells", "stext"), ("requires", "qtext")):
                yield from embedded_refs(field, ref_kind, entry.get(field))
            for field, ref_kind in (("spell", "spell"), ("quest", "quest"), ("item", "item")):
                ident = entry.get(field, 0)
                require(type(ident) is int and ident >= 0, f"invalid {field} in {column}")
                if ident:
                    yield ref_kind, ident
            for field in ("npc", "target"):
                ident = entry.get(field)
                if isinstance(ident, str) and re.fullmatch(r"#[0-9]+", ident):
                    yield "npc", int(ident[1:])
        return
    require(isinstance(value, str), f"invalid encoded {column}")
    if kind == "itext":
        for part in value.split(","):
            if part.strip():
                item = re.fullmatch(r"\s*([0-9]+)(?:\s*[*x×]\s*[1-9][0-9]*)?\s*", part)
                require(item and int(item[1]) > 0, f"invalid item/count in {column}")
                yield "item", int(item[1])
    else:
        require(bool(re.fullmatch(r"[0-9,\s|;/+-]+", value)), f"invalid ID list in {column}")
        for ident in re.findall(r"-?[0-9]+", value):
            if int(ident) > 0:
                yield ("spell" if kind == "stext" else "quest"), int(ident)


def projected(db, c, rules, tables, board_db=None, releasing=False):
    """Read all dependencies and all before-images before handing any operation to the writer."""
    verify_basis_record(db, c, board_db, releasing)
    identified, npc_names = set(), {}
    heirloom_feature = c["target"] == "heirloom" and c["lane"] == "heirloom-only"
    for check in c["checks"]:
        require(isinstance(check, dict), "invalid dependency")
        cols, row = lookup(db, check, tables, heirloom_feature)
        expect = check.get("expect")
        require(expect and valid_values(expect, cols) and subset(row, expect),
                f"destination prerequisite differs: {check['table']} {check['key']}")
        if c["target"] == "live":
            for rule in rules:
                if rule["target"] == "live" and rule["table"] in {"*", check["table"]} and (
                        rule["table"] == "*" or rule["reason"] == "Heirloom holiday quest"):
                    require(not matches_overlay(db, row, rule, npc_names), "protected Heirloom-only dependency")
        if check["table"] in IDENTITY_FIELDS and IDENTITY_FIELDS[check["table"]] <= expect.keys():
            identified.add((check["table"], canonical(check["key"])))
    ops, seen, quest_ids = [], set(), set()
    changes = c["changes"]
    require(all(isinstance(ch, dict) and isinstance(ch.get("key"), dict) for ch in changes), "invalid change")
    deleted = {(ch.get("table"), canonical(ch["key"])) for ch in changes if ch.get("after") is None}
    # Parents supplied by this ticket are checked and written before their dependent rows.
    for change in sorted(changes, key=lambda ch: ch.get("table") not in IDENTITY_FIELDS or ch.get("before") is not None):
        require(isinstance(change, dict), "invalid change")
        cols, row = lookup(db, change, tables, heirloom_feature)
        t, key = change["table"], change["key"]
        where = f"{t} {key}"
        row_id = (t, canonical(key))
        require(row_id not in seen, f"combine both changes to {where}")
        seen.add(row_id)
        require("before" in change and "after" in change, f"missing before/after for {where}")
        b, a = change["before"], change["after"]
        match = change.get("match", {})
        require(valid_values(match, cols), f"invalid identity guard for {where}")
        if b is None or a is None:
            full = a if b is None else b
            require(valid_values(full, cols) and full.keys() == cols and subset(full, key)
                    and subset(full, match), f"insert/delete requires a complete keyed row for {where}")
            require(row is None or canonical(row) == canonical(full), f"different row occupies {where}")
            before, after = b, a
            fields = set(cols)
        else:
            require(valid_values(b, cols) and valid_values(a, cols) and bool(b) and b.keys() == a.keys()
                    and b.keys().isdisjoint(key) and all(canonical(b[k]) != canonical(a[k]) for k in b),
                    f"declare exactly the changed non-key fields for {where}")
            identity_fields = IDENTITY_FIELDS.get(t, set(tables[t].get("key", [])) - key.keys())
            require(identity_fields <= match.keys(), f"missing destination identity for {where}")
            require(all(canonical(v) == canonical(b[k]) for k, v in match.items() if k in b),
                    f"inconsistent identity guard for {where}")
            match_after = {k: a.get(k, v) for k, v in match.items()}
            require((subset(row, match) and subset(row, b)) or (subset(row, match_after) and subset(row, a)),
                    f"destination drift or wrong identity at {where}")
            before, after = {**row, **b}, {**row, **a}
            fields = set(b)
        check_races(c["target"], t, before, after, fields)
        for rule in rules:
            if rule["target"] in {c["target"], "both"} and rule["table"] in {t, "*"} and (
                    "*" in rule["fields"] or fields.intersection(rule["fields"])):
                if any(matches_overlay(db, r, rule, npc_names) for r in (before, after)):
                    if (rule["target"] == "both" and "npc_name" in rule["where"] and
                            c["lane"] in {"heirloom-only", "live-policy"} and change_basis(c) == "owner"):
                        continue  # reviewed shard-specific vendor policy
                    allowed = rule.get("allow_after")
                    require(allowed and valid_values(allowed, cols) and subset(after, allowed),
                            f"protected {where}: {rule['reason']}")
        trans = dict(tables[t].get("trans", {}))
        if t == "quests":
            trans["excludes"] = "qtext"
        for col, kind in trans.items():
            if col not in fields.union(key, match) or after is None:
                continue
            value = after.get(col)
            if kind in REFERENCE:
                refs = [(kind, value)] if type(value) is int and value > 0 else []
            else:
                refs = embedded_refs(col, kind, value)
            for ref_kind, ident in refs:
                table, pk = REFERENCE[ref_kind]
                ref = (table, canonical({pk: ident}))
                if ref == row_id:
                    continue
                require(ref in identified and ref not in deleted,
                        f"verify the destination identity of {table} {ident} before {where}")
        if t in IDENTITY_FIELDS:
            if after is not None:
                identified.add(row_id)
                if t == "npcs":
                    npc_names[key["id"]] = after["npc_name"]
            else:
                identified.discard(row_id)
        if canonical(row) == canonical(after):
            continue
        if changes_progress(t, before, after):
            quest_ids.add(key["questid"])
        ops.append({"t": t, "pk": key, "before": before, "after": after, "unit": c["id"], "why": c["id"]})
    check_progress(db, quest_ids)
    if db.execute("SELECT 1 FROM sqlite_master WHERE type='table' AND name=?", (RELEASES,)).fetchone():
        previous = db.execute(f"SELECT run, undone FROM {RELEASES} WHERE target=? AND claim=? AND digest=?",
                              (c["target"], c["id"], digest(c))).fetchone()
        require(not previous or (not ops and not previous[1]),
                f"previously released revision of {c['id']}; reconcile its receipt before a new release")
    return {"ops": ops, "sigchange": [], "already": len(seen) - len(ops)}


def intake(data, batch, source):
    if isinstance(batch, list):
        rows = batch
    elif "columns" in batch and "rows" in batch:
        rows = [dict(zip(batch["columns"], row, strict=True)) for row in batch["rows"]]
    else:
        rows = batch["entries" if source == "work" else "petitions"]
    known = {c["id"].split("/")[0] for c in data["claims"]}
    for row in rows:
        number = row.get("id")
        require(type(number) is int and number > 0, "exported ticket needs a positive integer id")
        base = f"{source}:{number}"
        if base not in known:
            # Free text may contain personal data, replies or instructions. Keep only the reference.
            claim = {"id": base + "/intake", "lane": "unreviewed", "target": None,
                     "claim": f"Review {base} and split its distinct changes."}
            if source == "work" and row.get("basis") in BASES:
                version = row.get("version")
                pin = f" v{version}" if type(version) is int and version > 0 else ""
                claim.update(basis=row["basis"], basis_ref=f"board {number}{pin}")
            data["claims"].append(claim)
            known.add(base)
    return data


def queue_rows(data, args, db=None, tables=None, board_db=None):
    rows = []
    rules = overlay_rules(data) if db is not None else []
    conflicts = competing_claims(data)
    for c in data["claims"]:
        history, release = assess(c)
        if c["id"] in conflicts:
            history, release = "needs-resolution", "conflicting-tickets"
        if (args.target and c["target"] != args.target) or (args.certified and history != "certified"):
            continue
        if args.ready and release != "approved":
            continue
        state, reason = None, conflicts.get(c["id"])
        if db is not None and history in REVIEWED and c.get("delivery") == "data":
            try:
                require(isinstance(c.get("changes"), list) and isinstance(c.get("checks"), list)
                        and (c["changes"] or c["checks"]), "incomplete change packet")
                half = projected(db, c, rules, tables, board_db, releasing=args.ready)
                state = "changes pending" if half["ops"] else "already present"
            except (ReviewError, sqlite3.Error, KeyError, TypeError) as exc:
                state, reason = "blocked", str(exc)
        if args.ready and state != "changes pending":
            continue
        rows.append({"id": c["id"], "target": c["target"], "basis": change_basis(c), "history": history, "release": release,
                     "claim": c["claim"], "holds": c.get("holds", []), "observation": c.get("observation"),
                     "destination": state, "reason": reason, "digest": digest(c)})
    return rows


def transfer_value(field, source, desired, npc_ids):
    """Compare a source field to a proposed Live value, resolving event NPCs on Live."""
    if field != "events":
        return canonical(source) == canonical(desired)
    source_events = json.loads(source or "[]")
    for event in source_events:
        for key in ("npc", "target"):
            ref = event.get(key)
            if isinstance(ref, str) and re.fullmatch(r"#\d+", ref):
                if ref not in npc_ids:
                    return False
                event[key] = npc_ids[ref]
    return source_events == json.loads(desired or "[]")


def protected_identity(value, excluded):
    """Recognize exact entity names in rows and encoded events, independent of the claim's label."""
    if isinstance(value, dict):
        for key, part in value.items():
            if key in {"events", "steps", "choices", "byclass"} and isinstance(part, str) and part.strip():
                part = json.loads(part)
            if protected_identity(part, excluded):
                return True
        return False
    if isinstance(value, list):
        return any(protected_identity(v, excluded) for v in value)
    if isinstance(value, str):
        return value.strip().replace("’", "'").casefold() in excluded
    return False


def transfer_rows(claim, scope, excluded):
    """Return the pending row count for a reviewed, two-shard observation."""
    comparison = claim.get("transfer", {})
    features = comparison.get("features")
    require(isinstance(features, list) and features and
            all(scope.get(f, {}).get("scope") == "shared" for f in features), "unverified or shard-specific feature")
    require(all(nonblank(comparison.get(k)) for k in ("checked_utc", "case", "subject", "basis", "limits")) and
            all(nonblank(comparison.get("builds", {}).get(s)) for s in TARGETS), "missing shard readback")
    require(not protected_identity([comparison["rows"], comparison["live_dependencies"], claim["changes"],
                                    comparison.get("quest_parents", [])], excluded),
            "protected content or dependency")
    observed = {(r["table"], canonical(r["key"])): r for r in comparison["rows"]}
    changes = claim["changes"]
    require(len(observed) == len(comparison["rows"]) == len(changes) and
            set(observed) == {(c["table"], canonical(c["key"])) for c in changes}, "unmatched destination rows")
    dependencies = {(r["table"], canonical(r["key"])): r["values"] for r in comparison["live_dependencies"]}
    for check in claim["checks"]:
        require(check["expect"] and subset(dependencies.get((check["table"], canonical(check["key"]))),
                                           check["expect"]), "destination dependency differs")
    parent_quests = set()
    for pair in comparison.get("quest_parents", []):
        src, dst = pair["heirloom"], pair["live"]
        identity = {k: dst[k] for k in ("name", "giver")}
        require(all(nonblank(v) for v in identity.values()) and subset(src, identity) and
                subset(dependencies.get(("quests", canonical({"questid": dst["questid"]}))), identity),
                "different parent quest")
        parent_quests.add(dst["questid"])
    npc_ids = {}
    for pair in comparison.get("npc_pairs", []):
        src, dst = pair["heirloom"], pair["live"]
        require(nonblank(src["npc_name"]) and src["npc_name"] == dst["npc_name"] and src["world"] == dst["world"]
                and subset(dependencies.get(("npcs", canonical({"id": dst["id"]}))), dst),
                "unverified NPC identity")
        src_id, dst_id = f"#{src['id']}", f"#{dst['id']}"
        require(src_id not in npc_ids and dst_id not in npc_ids.values(), "ambiguous NPC mapping")
        npc_ids[src_id] = dst_id
    pending = 0
    for change in changes:
        reading = observed[change["table"], canonical(change["key"])]
        before, after = change["before"], change["after"]
        require(isinstance(after, dict) and after, "cleanup is outside the transfer shortlist")
        table = change["table"]
        allowed = {field for feature in features for field in scope[feature].get("fields", {}).get(table, [])}
        require(after.keys() <= allowed, "field lacks shared scope")
        require(table != "queststeps" or change["key"]["questid"] in parent_quests, "unverified quest parent")
        if before is not None and table in IDENTITY_FIELDS:
            identity = {k: reading["live"][k] for k in IDENTITY_FIELDS[table]}
            require(subset(reading["heirloom"], identity), "different source and destination identities")
        match = change.get("match", {})
        if before is None:
            require(subset(after, change["key"]) and subset(after, match), "unverified insert identity")
        else:
            require(before and before.keys() == after.keys() and not (after.keys() & change["key"].keys()) and
                    all(canonical(before[k]) != canonical(v) for k, v in after.items()), "invalid field change")
            require(all(canonical(match[k]) == canonical(before[k]) for k in match.keys() & before.keys()),
                    "inconsistent destination identity")
            require(match and (subset(reading["live"], {**match, **before}) or
                               subset(reading["live"], {**match, **after})), "destination identity or fields changed")
        require(all(transfer_value(k, reading["heirloom"][k], v, npc_ids) for k, v in after.items()),
                "Heirloom does not supply the reviewed Live behavior")
        if subset(reading["live"], after):
            continue
        require((before is None and reading["live"] is None) or
                (before is not None and subset(reading["live"], before)), "destination drift")
        pending += 1
    return pending


def transfer_shortlist(data):
    policy = load(Path(__file__).with_name("shard_overlays.json"))
    scope = policy["transfer_scope"]
    excluded = {name.replace("’", "'").casefold() for feature in scope.values() if feature["scope"] != "shared"
                for name in feature.get("identities", [])}
    excluded.update(name.casefold() for name in policy["live_excluded"]["names"])
    conflicts = competing_claims(data)
    rows = []
    for c in data["claims"]:
        history, release = assess(c)
        if (c["target"] != "live" or c["id"] in conflicts or history != "certified" or
                release not in {"needs-approval", "approved"}):
            continue
        try:
            n = transfer_rows(c, scope, excluded)
        except (ValueError, TypeError, KeyError, AttributeError):
            continue
        if n:
            t = c["transfer"]
            rows.append({"Case": t["case"], "Subject": t["subject"], "Live correction": c["claim"],
                         "Live rows": n, "Scope and evidence": t["basis"], "Field limits": t["limits"],
                         "Checked UTC": t["checked_utc"]})
    return rows


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    queue = sub.add_parser("queue", help="show historical certification and release routing")
    queue.add_argument("ledger")
    queue.add_argument("--target", choices=sorted(TARGETS))
    queue.add_argument("--certified", action="store_true")
    queue.add_argument("--snapshot", help="check the named target on a read-only destination snapshot")
    queue.add_argument("--config", help="trusted server config when checking its bound database")
    queue.add_argument("--board-db", help="bound Heirloom database for shared work references on Live")
    queue.add_argument("--ready", action="store_true", help="show approved tickets with verified pending changes")
    queue.add_argument("--json", action="store_true")
    shortlist = sub.add_parser("shortlist", help="filter recorded Heirloom-to-Live comparisons for review")
    shortlist.add_argument("ledger")
    shortlist.add_argument("--csv", action="store_true", help="print the candidate table as CSV")
    add = sub.add_parser("intake", help="merge a compact GM export into the ledger and print the JSON")
    add.add_argument("ledger")
    add.add_argument("export")
    add.add_argument("--source", required=True, choices=["work", "heirloom:petition", "live:petition"])
    add.add_argument("--output", help="write a new ledger file, preserving the input")
    args = parser.parse_args()
    data = ledger(args.ledger, allow_audit=args.command == "shortlist")
    if args.command == "shortlist":
        rows = transfer_shortlist(data)
        if args.csv:
            columns = ["Case", "Subject", "Live correction", "Live rows", "Scope and evidence", "Field limits", "Checked UTC"]
            writer = csv.DictWriter(sys.stdout, fieldnames=columns, lineterminator="\n")
            writer.writeheader()
            writer.writerows(rows)
        else:
            print(f"{len(rows)} Live transfer candidates")
            for row in rows:
                print(f"{row['Case']}  {row['Live correction']}")
        return
    if args.command == "intake":
        result = json.dumps(intake(data, load(args.export), args.source), indent=2, ensure_ascii=False)
        if args.output:
            with open(args.output, "x", encoding="utf-8") as f:
                print(result, file=f)
        else:
            print(result)
        return
    if (args.snapshot and not args.target) or ((args.ready or args.config or args.board_db) and not args.snapshot):
        parser.error("--ready requires --snapshot and --target; a snapshot requires --target")
    if args.snapshot:
        import live_sync_20261122 as sync
        with contextlib.closing(sync.ss.readonly(args.snapshot)) as db:
            db.execute("BEGIN")
            require(sync.target_in_transaction(db, args.config) == args.target, "snapshot belongs to the other shard")
            rows = queue_rows(data, args, db, sync.ss.SPECS, args.board_db)
    else:
        rows = queue_rows(data, args)
    if args.json:
        print(json.dumps(rows, indent=2, ensure_ascii=False))
    else:
        if not rows:
            print("0 matching claims")
        for row in rows:
            print(f"{row['id']}  {row['target'] or '?'}  {row['history']} / {row['release']}  {row['claim']}")
            if row["destination"]:
                print("  Destination:", row["destination"], row["reason"] or "")
            elif row["reason"]:
                print("  Competing claim:", row["reason"])
            for hold in row["holds"]:
                print(f"  Hold: {hold}")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError, KeyError, TypeError, sqlite3.Error) as exc:
        sys.exit(f"REFUSED: {exc}")
