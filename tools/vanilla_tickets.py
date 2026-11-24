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
LANES = {"retail", "heirloom-only", "live-policy", "operations", "unreviewed"}
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
    rules = permanent["overlays"] + extra
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


def assess(c):
    """Certification is about a bounded service-era claim. Permission is a separate decision."""
    fp = digest(c)
    if nonblank(c.get("superseded_by")):
        return "superseded", "archived"
    if c["lane"] != "retail":
        return "unreviewed" if c["lane"] == "unreviewed" else "outside-retail", c["lane"]
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
    if c.get("conflicts") != []:
        return "needs-resolution", "hold"
    if not signed(c.get("review"), fp):
        return "needs-review", "hold"
    if c.get("holds") != []:
        return "certified", "held"
    if c.get("delivery") == "code":
        patch = c.get("patch", {})
        if not (isinstance(patch, dict) and re.fullmatch(r"[0-9a-f]{40}", str(patch.get("commit", "")))
                and nonblank(patch.get("scope")) and nonblank(patch.get("tests"))):
            return "certified", "needs-code-packet"
        return "certified", "owner-code-review"
    if c.get("delivery") != "data" or not isinstance(c.get("changes"), list) or not isinstance(c.get("checks"), list):
        return "certified", "needs-change-packet"
    if not c["changes"]:
        return "certified", "verify-only" if c["checks"] else "needs-change-packet"
    return "certified", "approved" if signed(c.get("approval"), fp) else "needs-approval"


def get_claim(data, claim_id, target):
    c = next((entry for entry in data["claims"] if entry["id"] == claim_id), None)
    require(c is not None, f"unknown claim {claim_id}")
    require(c["target"] == target, f"{claim_id} does not target {target}")
    return c


def competing_claims(data):
    """Find incompatible certified intents at the same destination field, including a row deletion."""
    owners, conflicts = {}, {}
    for c in data["claims"]:
        if assess(c)[0] != "certified" or c.get("delivery") != "data":
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


def lookup(db, spec, tables):
    """Resolve a real primary key on the destination. Never substitute a source row ID."""
    t, key = spec.get("table"), spec.get("key")
    require(t in tables and t not in CUSTOM_TABLES, f"{t} is outside retail content")
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


def matches_overlay(db, row, rule):
    if row is None:
        return False
    if "npc_from" in rule:
        col = rule["npc_from"]
        require(col in row, f"unknown NPC reference {col} in shard overlay")
        ref = row[col]
        if isinstance(ref, int) or (isinstance(ref, str) and re.fullmatch(r"#\d+", ref)):
            ident = ref if isinstance(ref, int) else int(ref[1:])
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


def projected(db, c, rules, tables):
    """Read all dependencies and all before-images before handing any operation to the writer."""
    identified = set()
    for check in c["checks"]:
        require(isinstance(check, dict), "invalid dependency")
        cols, row = lookup(db, check, tables)
        expect = check.get("expect")
        require(expect and valid_values(expect, cols) and subset(row, expect),
                f"destination prerequisite differs: {check['table']} {check['key']}")
        if check["table"] in IDENTITY_FIELDS and IDENTITY_FIELDS[check["table"]] <= expect.keys():
            identified.add((check["table"], canonical(check["key"])))
    ops, seen, quest_ids = [], set(), set()
    for change in c["changes"]:
        require(isinstance(change, dict), "invalid change")
        cols, row = lookup(db, change, tables)
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
        for rule in rules:
            if rule["target"] in {c["target"], "both"} and rule["table"] == t and (
                    "*" in rule["fields"] or fields.intersection(rule["fields"])):
                if any(matches_overlay(db, r, rule) for r in (before, after)):
                    allowed = rule.get("allow_after")
                    require(allowed and valid_values(allowed, cols) and subset(after, allowed),
                            f"protected {where}: {rule['reason']}")
        for col, kind in tables[t].get("trans", {}).items():
            if kind not in REFERENCE or col not in fields.union(key, match):
                continue
            table, pk = REFERENCE[kind]
            value = (after or before).get(col)
            if table != t and isinstance(value, int) and value > 0:
                require((table, canonical({pk: value})) in identified,
                        f"verify the destination identity of {table} {value} before {where}")
        if t in IDENTITY_FIELDS:
            if after is not None:
                identified.add(row_id)
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
            data["claims"].append({"id": base + "/intake", "lane": "unreviewed", "target": None,
                                   "claim": f"Review {base} and split its distinct changes."})
            known.add(base)
    return data


def queue_rows(data, args, db=None, tables=None):
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
        if db is not None and history == "certified" and c.get("delivery") == "data":
            try:
                require(isinstance(c.get("changes"), list) and isinstance(c.get("checks"), list)
                        and (c["changes"] or c["checks"]), "incomplete change packet")
                half = projected(db, c, rules, tables)
                state = "changes pending" if half["ops"] else "already present"
            except (ReviewError, sqlite3.Error, KeyError, TypeError) as exc:
                state, reason = "blocked", str(exc)
        if args.ready and state != "changes pending":
            continue
        rows.append({"id": c["id"], "target": c["target"], "history": history, "release": release,
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
    scope = load(Path(__file__).with_name("shard_overlays.json"))["transfer_scope"]
    excluded = {name.replace("’", "'").casefold() for feature in scope.values() if feature["scope"] != "shared"
                for name in feature.get("identities", [])}
    conflicts = competing_claims(data)
    rows = []
    for c in data["claims"]:
        if c["target"] != "live" or c["id"] in conflicts or assess(c)[1] not in {"needs-approval", "approved"}:
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
    if (args.snapshot and not args.target) or (args.ready and not args.snapshot):
        parser.error("--ready requires --snapshot and --target; a snapshot requires --target")
    if args.snapshot:
        import live_sync_20261122 as sync
        with contextlib.closing(sync.ss.readonly(args.snapshot)) as db:
            db.execute("BEGIN")
            require(sync.target_in_transaction(db) == args.target, "snapshot belongs to the other shard")
            rows = queue_rows(data, args, db, sync.ss.SPECS)
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
