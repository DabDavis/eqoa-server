"""Author the disc-relative cut for the Neriak arena (Tunaria zone 45).

Input is the operator's Frontiers disc image. The plan contains offsets into the zone
and authored doorway polygons. No disc textures or native vertex buffers
are distributed. Both world composers apply the identical checked-in plan.
"""
import argparse
import gzip
import math
import struct
import zlib
from pathlib import Path

SOURCE_CRC = 0xFEC7DE9C
ARENA_SPRITE = 0x636CD613
CX, CZ = 24637.0, 8679.25


def u32(data, pos):
    return struct.unpack_from("<I", data, pos)[0]


def children(data, parent):
    cursor = parent + 12
    for _ in range(u32(data, parent + 8)):
        kind, _, length, _ = struct.unpack_from("<HHII", data, cursor)
        yield kind, cursor
        cursor += 12 + length


def child(data, parent, kind):
    return next(pos for tag, pos in children(data, parent) if tag == kind)


def zone_source(path):
    """Read the one zone directly from the ISO9660 disc, without extracting Tunaria."""
    with path.open("rb") as disc:
        def read(at, count):
            disc.seek(at)
            data = disc.read(count)
            if len(data) != count:
                raise ValueError("short disc read")
            return data

        def directories(entry):
            data = read(u32(entry, 2) * 2048, u32(entry, 10))
            pos = 0
            while pos < len(data):
                length = data[pos]
                if not length:
                    pos = (pos // 2048 + 1) * 2048
                    continue
                rec = data[pos:pos + length]
                pos += length
                name = rec[33:33+rec[32]].partition(b";")[0].upper()
                if name not in (b"\x00", b"\x01"):
                    yield name, rec

        def find_world(entry):
            for name, rec in directories(entry):
                if name == b"TUNARIA.ESF":
                    return u32(rec, 2) * 2048
                if rec[25] & 2:
                    result = find_world(rec)
                    if result is not None:
                        return result
            return None

        volume = read(16 * 2048, 2048)
        if volume[1:6] != b"CD001":
            raise ValueError("expected an ISO9660 Frontiers disc")
        base = find_world(volume[156:190])
        if base is None:
            raise ValueError("no TUNARIA.ESF on disc")

        def records(parent):
            pos = parent + 12
            for _ in range(u32(read(base + parent, 12), 8)):
                kind, _, size, _ = struct.unpack("<HHII", read(base + pos, 12))
                yield kind, pos, size
                pos += 12 + size

        root = u32(read(base + 16, 4), 0)
        world = next(pos for tag, pos, _ in records(root) if tag == 0x8100)
        zones = [(pos, size) for tag, pos, size in records(world) if tag == 0x3000]
        pos, size = zones[45]
        data = read(base + pos, size + 12)
    if zlib.crc32(data) != SOURCE_CRC:
        raise ValueError("expected the original TUNARIA.ESF zone 45")
    return data


class Plan:
    def __init__(self):
        self.code = bytearray()

    def zero(self, dest, size):
        self.code.extend(struct.pack("<BII", 0, dest, size))

    def copy(self, dest, source, size):
        if source != dest:
            self.code.extend(struct.pack("<BIII", 1, dest, size, source))

    def put(self, dest, value):
        self.code.extend(struct.pack("<BII", 2, dest, len(value)) + value)

    def count(self, dest, value):
        self.put(dest, struct.pack("<I", value))


def inside(point):
    x, y, z = point
    dx, dz = abs(x - CX), abs(z - CZ)
    return dx <= 48.35 and dz <= 48.35 and dx + dz <= 68.4 and -25 <= y <= 62


def sealed_walls():
    # Full width of the four flat faces. The diagonal masonry stays native.
    # Textured inward and solid from either side.
    for nx, nz in ((1, 0), (0, 1), (-1, 0), (0, -1)):
        x, z = CX + nx * 47.93, CZ + nz * 47.93
        yield (nx,nz), [(x - nz * t, y, z + nx * t) for t,y in
                       ((-20, -0.35), (-20, 28.25), (20, -0.35), (20, 28.25))]


def collect_buffer(data, rec, draw, translations, plan):
    start = rec + 12
    header, step, line = (32, 19, 8) if draw else (20, 8, 12)
    lists_at, verts_at, group_at = (12, 16, 17) if draw else (8, 12, 6)
    assert u32(data, start + (4 if draw else 0)) == (4 if draw else 3)
    exponent = u32(data, start + (20 if draw else 16))
    scale = 2 ** exponent
    src = dst = start + header
    nlists = nverts = 0
    def put(value):
        nonlocal dst
        plan.put(dst, value)
        dst += len(value)
    def copy(at, n):
        nonlocal dst
        plan.copy(dst, at, n)
        dst += n
    for _ in range(u32(data, start + lists_at)):
        count = u32(data, src)
        verts = src + line
        included = []
        for i in range(count):
            pos = verts + i * step
            loc = struct.unpack_from("<3h", data, pos)
            group = struct.unpack_from("<h" if draw else "<b", data, pos + group_at)[0]
            delta = translations[group] if group >= 0 else (0,0,0)
            included.append(inside(tuple(a / scale + b for a,b in zip(loc,delta))))
        # A run of inside vertices forms exactly the original strip's
        # inside triangles. Preserve its winding across a cut at an odd vertex.
        first = 0
        while first < count:
            if not included[first]:
                first += 1
                continue
            end = first + 1
            while end < count and included[end]:
                end += 1
            if end - first >= 3:
                pad = first % 2
                length = end - first + pad
                put(struct.pack("<I",length))
                copy(src + 4, line - 4)
                if pad:
                    copy(verts + step * first, step)
                copy(verts + step * first, step * (end-first))
                nlists += 1
                nverts += length
            first = end
        src = verts + count * step

    for (nx,nz), points in sealed_walls():
        # Front faces face inward. Emit a reversed strip for the outward face,
        # using the room's native stone material 2 and a neutral opaque tint.
        for side in (1,-1):
            put(struct.pack("<II",4,2) if draw else struct.pack("<III",4,0,0))
            for i in ([0,1,2,3] if side==1 else [1,0,3,2]):
                v = points[i]
                group = min(range(min(128,len(translations))), key=lambda g: math.dist(v, translations[g]))
                xyz = [round((a-b)*scale) for a,b in zip(v, translations[group])]
                if draw:
                    put(struct.pack("<3h2h3b4Bh",*xyz, (i//2)*4096, (i%2)*3072, -nx*127*side,0,-nz*127*side,128,128,128,255,group))
                else:
                    put(struct.pack("<3hbB",*xyz,group,0))
            nlists += 1
            nverts += 4
    if dst > start + u32(data,rec+4):
        raise ValueError("arena mesh exceeds its source record")
    plan.count(start+lists_at,nlists)
    plan.count(start+verts_at,nverts)


def build(data):
    base = child(data,0,0x3200)
    pt = child(data,base,0x3250)+12
    translations = [struct.unpack_from("<3f",data,pt+4+12*i) for i in range(u32(data,pt))]
    plan = Plan()
    found = 0
    for kind, spr in children(data,child(data,0,0x3100)):
        if kind != 0x2310:
            continue
        selected = u32(data,child(data,spr,0x2311)+12)==ARENA_SPRITE
        found += selected
        for tag, buffer in children(data,spr):
            if tag not in (0x1200,0x4200):
                continue
            draw = tag == 0x1200
            if selected:
                collect_buffer(data,buffer,draw,translations,plan)
            else:
                plan.zero(buffer+12+(12 if draw else 8),8)
    assert found==1
    for _,room in children(data,child(data,base,0x3230)):
        plan.zero(room+12+36,8)    # a sealed room has no portals
        if u32(data,room+12)!=ARENA_SPRITE:
            plan.zero(room+12,4)
    # Strip actor lists and their baked lights together. Keep the native BSP
    # and its environment leaves, including all four lava basins.
    for tag,pos in children(data,0):
        if tag in (0x3290,0x32B0):
            plan.zero(pos+8,4)
    return gzip.compress(bytes(plan.code),mtime=0)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("disc", type=Path)
    parser.add_argument("output", type=Path, nargs="+")
    args=parser.parse_args()
    out = build(zone_source(args.disc))
    for target in args.output:
        target.write_bytes(out)
