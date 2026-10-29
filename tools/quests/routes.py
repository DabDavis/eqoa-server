"""The travel NPCs' routes a quest's `unlocks` may name (tools/quests/FORMAT.md),
read off internal/handlers/waysender.go's wayRoutes, the one list there is
(the server's handlers.TravelRoutes gives the same), and an unlock matched to
one as the server matches it (handlers/questtrips.go: the NPC's name any
case, the place's routeKey).

    import routes
    routes.travel_routes()            # [(npc, to), ...], or None without the file
    routes.route_of(npc, to, rs)      # the route (npc, to) an unlock names, or None
"""
import os
import re

HERE = os.path.dirname(os.path.abspath(__file__))
WAYSENDER = os.path.join(HERE, "..", "..", "internal", "handlers", "waysender.go")


def route_key(s):
    """A place as an unlock's matches a route's label (questtrips.go's
    routeKey): case, a leading "the" and all but letters and digits aside."""
    s = str(s or "").strip().lower()
    if s.startswith("the "):
        s = s[4:]
    return "".join(ch for ch in s if ch.isalnum())


def npc_key(s):
    """A travel NPC's name as the server matches one (foldName): any case."""
    return str(s or "").strip().lower()


def travel_routes(path=WAYSENDER):
    """wayRoutes as [(npc, to)] in the file's order; None when the file is
    not there (the tool run outside the server's tree)."""
    try:
        with open(path, encoding="utf-8") as f:
            src = f.read()
    except OSError:
        return None
    m = re.search(r"^var wayRoutes = map\[string\]\[\]wayRoute\{\n(.*?)^\}", src, re.S | re.M)
    if not m:
        return None
    out, npc = [], None
    for line in m.group(1).splitlines():
        k = re.match(r'\s*"((?:[^"\\]|\\.)*)":\s*\{\s*$', line)
        if k:
            npc = k.group(1)
            continue
        for to in re.findall(r'\{\s*To:\s*"((?:[^"\\]|\\.)*)"', line):
            if npc is not None:
                out.append((npc, to))
    return out


def route_of(npc, to, routes):
    """The route (npc, to) of routes an unlock of npc's trip to `to` names, or
    None."""
    want = (npc_key(npc), route_key(to))
    for r in routes or []:
        if (npc_key(r[0]), route_key(r[1])) == want:
            return r
    return None
