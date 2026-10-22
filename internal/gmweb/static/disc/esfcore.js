// Reading the game's own files (ESF containers) off the GM's disc, in the
// browser. Ported from esfviewer's esf.js and zone.js (themselves ported from
// pkg/esf and checked against it byte for byte); trimmed to what the panel
// draws: a model in its bind pose with its textures, an icon, a zone's
// terrain. No DOM: it runs in the disc worker.
//
// A container is reversed "OBJF", a header whose int64 at +16 is where the
// records begin, then nested records of
//   type u16 | version i16 | size i32 | children i32 | body... children...
// A record's children sit inside its size.

export const T = {
  SURFACE: 0x1000, MATERIAL: 0x1100, MATERIAL_ARRAY: 0x1101, MATPAL: 0x1110, MATPAL_HDR: 0x1111,
  PRIMBUF: 0x1200, SKINPRIMBUF: 0x1210, SIMPLESPRITE: 0x2000, SIMPLESPRITE_HDR: 0x2001,
  HSPRITE: 0x2200, SUBSPRITE: 0x2310, SUBSPRITE_HDR: 0x2311, SKINSUBSPRITE: 0x2320, SKINSUBSPRITE2: 0x2321,
  CSPRITE: 0x2700, CSPRITE_HDR: 0x2710, CSPRITE_ARRAY: 0x2800, CSPRITE_VARIANT: 0x2a40, CSPRITE_VARIANT_MESHES: 0x2a60,
  GROUPSPRITE: 0x2c00, GROUPSPRITE_HDR: 0x2c10, GROUPSPRITE_MEMBERS: 0x2c30,
  ZONE: 0x3000, ZONE_RESOURCES: 0x3100, ZONE_BASE: 0x3200, ZONE_PRETRANS: 0x3250,
  WORLD_ROOT: 0x8000, WORLD: 0x8100, WORLD_BASE: 0x8200, ZONE_PROXIES: 0x8210,
};

// Types whose body starts with a DictID, and the headers among them that name
// their PARENT (pkg/esf objTypeRegistry).
const HAS_DICTID = new Set([0x1000, 0x1111, 0x1200, 0x1220, 0x1230, 0x2001, 0x2210, 0x2320, 0x5000, 0x2710,
  0x2a10, 0x2c10, 0x2e00, 0x2f01, 0x4200, 0xb101, 0xc010, 0xc101, 0xc210, 0xc310]);
const IS_ID_CONTAINER = new Set([0x1111, 0x2001, 0x2210, 0x2710, 0x2c10, 0x2f01, 0xb101, 0xc010, 0xc101, 0xc210, 0xc310]);

export function magicOK(buf) {
  const u8 = new Uint8Array(buf, 0, 4);
  return String.fromCharCode(u8[3], u8[2], u8[1], u8[0]) === 'OBJF';
}

export function parseESF(buf) {
  if (!magicOK(buf)) throw new Error('not an ESF container (no OBJF)');
  return parseFrom(buf, Number(new DataView(buf).getBigInt64(16, true)));
}

// parseFrom walks the record forest from an offset (0 for a slice cut out of
// a file, which has no header).
export function parseFrom(buf, dataOffset) {
  const dv = new DataView(buf);
  const dict = new Map();
  const roots = [];
  const stack = [];
  let pos = dataOffset;
  const end = buf.byteLength;
  for (;;) {
    while (stack.length && stack[stack.length - 1].left === 0) {
      const top = stack.pop();
      if (pos < top.endPos) pos = top.endPos;
    }
    if (pos + 12 > end) break;
    const type = dv.getUint16(pos, true);
    if (type === 0 && stack.length === 0) break;
    const version = dv.getInt16(pos + 2, true);
    const size = dv.getInt32(pos + 4, true);
    const nSub = dv.getInt32(pos + 8, true);
    const body = pos + 12;
    if (size < 0 || body + size > end) break;
    const node = { type, version, size, body, children: [], parent: null };
    if (stack.length) {
      const top = stack[stack.length - 1];
      top.left--;
      node.parent = top.node;
      top.node.children.push(node);
    } else roots.push(node);
    if (HAS_DICTID.has(type) && size >= 4) {
      const id = dv.getInt32(body, true);
      if (id !== 0) {
        const target = IS_ID_CONTAINER.has(type) && node.parent ? node.parent : node;
        if (!dict.has(id)) dict.set(id, target);
      }
    }
    if (nSub > 0) {
      stack.push({ left: nSub, endPos: body + size, node });
      pos = body;
    } else pos = body + size;
  }
  return { dict, roots, dv, buf };
}

export function child(node, type) {
  for (const c of node.children) if (c.type === type) return c;
  return null;
}

export function childrenOf(node, type) {
  return node.children.filter((c) => c.type === type);
}

// ---- surfaces

export function readSurfaceMeta(dv, body) {
  const dictID = dv.getInt32(body, true);
  const w = dv.getInt32(body + 4, true);
  const h = dv.getInt32(body + 8, true);
  const depth = dv.getInt32(body + 12, true);
  const mip = dv.getInt32(body + 16, true);
  let p = body + 20;
  let palOff = 0, palEntries = 0;
  if (depth < 2) {
    palEntries = dv.getInt32(p, true);
    p += 4;
    palOff = p;
    p += palEntries * 4;
  }
  let rowBytes = 0, pixOff = 0;
  if (mip > 0) {
    rowBytes = dv.getInt32(p, true);
    p += 4;
    pixOff = p;
  }
  const bpp = palEntries > 0 && palEntries <= 16 && rowBytes < w ? 4 : 8;
  return { dictID, w, h, depth, mip, palOff, palEntries, rowBytes, pixOff, bpp };
}

// decodeSurface is a surface's base mip as RGBA bytes (palette entries are
// little-endian 0xAABBGGRR, alpha 255 opaque; 4 bpp packs the high nibble
// first).
export function decodeSurface(dv, s) {
  if (s.mip <= 0 || s.palEntries === 0 || s.w <= 0 || s.h <= 0 || s.w > 4096 || s.h > 4096) return null;
  const px = new Uint8ClampedArray(s.w * s.h * 4);
  const pal = new Uint8Array(4 * s.palEntries);
  for (let i = 0; i < s.palEntries; i++) {
    const v = dv.getUint32(s.palOff + i * 4, true);
    pal[i * 4] = v & 0xff;
    pal[i * 4 + 1] = (v >>> 8) & 0xff;
    pal[i * 4 + 2] = (v >>> 16) & 0xff;
    pal[i * 4 + 3] = (v >>> 24) & 0xff;
  }
  for (let y = 0; y < s.h; y++) {
    const row = s.pixOff + y * s.rowBytes;
    if (row + s.rowBytes > dv.byteLength) break;
    const lim = s.bpp === 4 ? s.w : Math.min(s.w, s.rowBytes);
    for (let x = 0; x < lim; x++) {
      let idx;
      if (s.bpp === 4) {
        const b = dv.getUint8(row + (x >> 1));
        idx = x & 1 ? b & 0x0f : (b >>> 4) & 0x0f;
      } else idx = dv.getUint8(row + x);
      if (idx >= s.palEntries) continue;
      const o = (y * s.w + x) * 4, q = idx * 4;
      px[o] = pal[q];
      px[o + 1] = pal[q + 1];
      px[o + 2] = pal[q + 2];
      px[o + 3] = pal[q + 3];
    }
  }
  return { w: s.w, h: s.h, data: px };
}

// surfaceByID decodes the surface a DictID names in a parsed container.
export function surfaceByID(parsed, id) {
  const n = parsed.dict.get(id);
  if (!n || n.type !== T.SURFACE) return null;
  return decodeSurface(parsed.dv, readSurfaceMeta(parsed.dv, n.body));
}

// ---- materials and geometry

export function readMaterial(dv, body, version) {
  const out = [];
  let p = body;
  const nLayers = dv.getInt32(p, true);
  p += 4;
  if (version > 1) p += 4;
  if (version > 2) p += 4;
  if (nLayers < 0 || nLayers > 64) return out;
  for (let i = 0; i < nLayers; i++) {
    p += 4;
    const tex = dv.getInt32(p, true);
    p += 4;
    p += 4;
    const blend = dv.getInt32(p, true);
    p += 4;
    const col = [dv.getUint8(p), dv.getUint8(p + 1), dv.getUint8(p + 2), dv.getUint8(p + 3)];
    p += 4;
    const uvt = new Float32Array(9);
    for (let j = 0; j < 9; j++) uvt[j] = dv.getFloat32(p + j * 4, true);
    p += 36 + 12;
    out.push({ tex, col, uvt, blend, index: i });
  }
  return out;
}

// uvCoeffs: u' = u*m0 + v*m3 + m6, v' = u*m1 + v*m4 + m7, a zero diagonal
// meaning identity (terrain.go).
export function uvCoeffs(uvt) {
  if (!uvt) return [1, 0, 0, 1, 0, 0];
  let m0 = uvt[0], m1 = uvt[1], m3 = uvt[3], m4 = uvt[4];
  if (m0 === 0 && m4 === 0) { m0 = 1; m4 = 1; }
  return [m0, m1, m3, m4, uvt[6], uvt[7]];
}

const VSIZE = { 2: 17, 4: 19, 5: 21 };

// readPrimBuffer: {lists:[{material, count, pos, uv, nrm, col}]}; preTrans
// adds a zone's coordinate island to each terrain vertex (pbtype 4's vgroup).
export function readPrimBuffer(dv, node, preTrans) {
  const ver = node.version;
  let p = node.body;
  const out = { lists: [] };
  let pbtype = 0, nfaces = 0, pack1 = 1, pack2 = 1;
  if (ver === 0) {
    p += 4;
    nfaces = dv.getInt32(p, true);
    p += 8;
  } else {
    if (ver > 1) p += 4;
    pbtype = dv.getInt32(p, true);
    p += 8;
    nfaces = dv.getInt32(p, true);
    p += 8;
    const p1 = dv.getInt32(p, true);
    const p2 = dv.getInt32(p + 4, true);
    p += 12;
    pack1 = 1 / Math.pow(2, p1);
    pack2 = 1 / Math.pow(2, p2);
    if (!VSIZE[pbtype]) return out;
  }
  if (nfaces < 0 || nfaces > 100000) return out;
  const stride = ver === 0 ? 36 : VSIZE[pbtype];
  for (let fi = 0; fi < nfaces; fi++) {
    if (p + 8 > dv.byteLength) break;
    const nverts = dv.getInt32(p, true);
    const mat = dv.getInt32(p + 4, true);
    p += 8;
    if (nverts < 0 || nverts > 200000 || p + nverts * stride > dv.byteLength) break;
    const pos = new Float32Array(nverts * 3), uv = new Float32Array(nverts * 2);
    const nrm = new Float32Array(nverts * 3), col = new Float32Array(nverts * 3);
    for (let i = 0; i < nverts; i++) {
      let x, y, z, u, v, nx, ny, nz, r, g, b;
      if (ver === 0) {
        x = dv.getFloat32(p, true); y = dv.getFloat32(p + 4, true); z = dv.getFloat32(p + 8, true);
        u = dv.getFloat32(p + 12, true); v = dv.getFloat32(p + 16, true);
        nx = dv.getFloat32(p + 20, true); ny = dv.getFloat32(p + 24, true); nz = dv.getFloat32(p + 28, true);
        r = dv.getUint8(p + 32) / 255; g = dv.getUint8(p + 33) / 255; b = dv.getUint8(p + 34) / 255;
      } else {
        x = dv.getInt16(p, true) * pack1; y = dv.getInt16(p + 2, true) * pack1; z = dv.getInt16(p + 4, true) * pack1;
        u = dv.getInt16(p + 6, true) * pack2; v = dv.getInt16(p + 8, true) * pack2;
        nx = dv.getInt8(p + 10) / 127; ny = dv.getInt8(p + 11) / 127; nz = dv.getInt8(p + 12) / 127;
        if (pbtype === 5) { r = g = b = 1; } else { r = dv.getUint8(p + 13) / 255; g = dv.getUint8(p + 14) / 255; b = dv.getUint8(p + 15) / 255; }
        if (preTrans) {
          const vg = pbtype === 4 ? dv.getInt16(p + 17, true) : 0;
          if (vg >= 0 && vg < preTrans.length) { x += preTrans[vg][0]; y += preTrans[vg][1]; z += preTrans[vg][2]; }
        }
      }
      p += stride;
      pos[i * 3] = x; pos[i * 3 + 1] = y; pos[i * 3 + 2] = z;
      uv[i * 2] = u; uv[i * 2 + 1] = v;
      nrm[i * 3] = nx; nrm[i * 3 + 1] = ny; nrm[i * 3 + 2] = nz;
      col[i * 3] = r; col[i * 3 + 1] = g; col[i * 3 + 2] = b;
    }
    out.lists.push({ material: mat, count: nverts, pos, uv, nrm, col });
  }
  return out;
}

// spriteMaterials is a sprite's palette: material index -> its layers.
export function spriteMaterials(parsed, sprite) {
  const dv = parsed.dv;
  let mp = child(sprite, T.MATPAL);
  if (!mp) {
    const hdr = child(sprite, T.SUBSPRITE_HDR) || child(sprite, T.SKINSUBSPRITE2);
    if (hdr) {
      const n = parsed.dict.get(dv.getInt32(hdr.body + 4, true));
      if (n && n.type === T.MATPAL) mp = n;
    }
  }
  if (!mp) return [];
  const arr = child(mp, T.MATERIAL_ARRAY);
  if (!arr) return [];
  return childrenOf(arr, T.MATERIAL).map((m) => readMaterial(dv, m.body, m.version).filter((l) => l.tex !== 0));
}

function stripIndices(count, base, out) {
  for (let k = 0; k + 2 < count; k++) {
    if (k % 2 === 0) out.push(base + k, base + k + 2, base + k + 1);
    else out.push(base + k, base + k + 1, base + k + 2);
  }
}

// buildModel flattens a sprite into one mesh and its draw batches (one a
// texture and tint): a CSprite's body parts, a GroupSprite's members placed by
// their (angles, scale, translation), or any record's own mesh. A hierarchical
// sprite's parts come unposed (their bones are not read here).
export function buildModel(parsed, node) {
  const dv = parsed.dv;
  const pos = [], uv = [], nrm = [], col = [];
  const batches = new Map();
  const bb = [Infinity, Infinity, Infinity, -Infinity, -Infinity, -Infinity];
  let verts = 0;
  const seen = new Set();
  const addSprite = (sprite, xform) => {
    if (!sprite || seen.has(sprite)) return;
    const pb = child(sprite, T.PRIMBUF) || child(sprite, T.SKINPRIMBUF);
    if (!pb) return;
    if (!xform) seen.add(sprite);
    const geom = readPrimBuffer(dv, pb);
    const mats = spriteMaterials(parsed, sprite);
    for (const L of geom.lists) {
      const base = verts;
      for (let i = 0; i < L.count; i++) {
        let x = L.pos[i * 3], y = L.pos[i * 3 + 1], z = L.pos[i * 3 + 2];
        let nx = L.nrm[i * 3], ny = L.nrm[i * 3 + 1], nz = L.nrm[i * 3 + 2];
        if (xform) {
          x *= xform.s; y *= xform.s; z *= xform.s;
          if (xform.ry) {
            const c = Math.cos(xform.ry), sn = Math.sin(xform.ry);
            const x2 = z * sn + x * c, z2 = z * c - x * sn;
            x = x2; z = z2;
            const nx2 = nz * sn + nx * c, nz2 = nz * c - nx * sn;
            nx = nx2; nz = nz2;
          }
          x += xform.t[0]; y += xform.t[1]; z += xform.t[2];
        }
        pos.push(x, y, z);
        bb[0] = Math.min(bb[0], x); bb[1] = Math.min(bb[1], y); bb[2] = Math.min(bb[2], z);
        bb[3] = Math.max(bb[3], x); bb[4] = Math.max(bb[4], y); bb[5] = Math.max(bb[5], z);
        uv.push(L.uv[i * 2], L.uv[i * 2 + 1]);
        nrm.push(nx, ny, nz);
        col.push(L.col[i * 3], L.col[i * 3 + 1], L.col[i * 3 + 2]);
      }
      const layer = (mats[L.material] || [])[0] || { tex: 0, col: [255, 255, 255, 255], uvt: null };
      const key = `${layer.tex}|${layer.col.join(',')}`;
      let bt = batches.get(key);
      if (!bt) { bt = { tex: layer.tex, col: layer.col, uv: uvCoeffs(layer.uvt), idx: [] }; batches.set(key, bt); }
      stripIndices(L.count, base, bt.idx);
      verts += L.count;
    }
  };
  const addTree = (n) => {
    // every sprite under a record (a hierarchical sprite's parts)
    const st = [n];
    while (st.length) {
      const x = st.pop();
      if (child(x, T.PRIMBUF) || child(x, T.SKINPRIMBUF)) addSprite(x, null);
      for (const c of x.children) st.push(c);
    }
  };
  if (node.type === T.GROUPSPRITE) {
    const members = child(node, T.GROUPSPRITE_MEMBERS);
    if (members) {
      let p = members.body;
      const n = dv.getInt32(p, true);
      p += 4;
      for (let i = 0; i < n && i < 4096; i++) {
        // a member is (angles, scale, translation): Transform(m_loc, ..., m_hpr)
        // with the angles first (claude-notes/groupsprite-member-order-20261019.md)
        const id = dv.getInt32(p, true);
        const ry = dv.getFloat32(p + 4, true);
        const sc = dv.getFloat32(p + 16, true);
        const tx = dv.getFloat32(p + 20, true), ty = dv.getFloat32(p + 24, true), tz = dv.getFloat32(p + 28, true);
        p += 32;
        const m = parsed.dict.get(id);
        if (!m) continue;
        if (m.type === T.GROUPSPRITE || m.type === T.HSPRITE) {
          const sub = buildModel(parsed, m);
          if (sub) mergeInto(sub, { s: sc || 1, ry, t: [tx, ty, tz] });
        } else addSprite(m, { s: sc || 1, ry, t: [tx, ty, tz] });
      }
    }
    addSprite(node, null);
  } else if (node.type === T.CSPRITE) {
    const arr = child(node, T.CSPRITE_ARRAY);
    for (const c of arr ? arr.children : []) {
      if (c.type === T.CSPRITE_VARIANT) {
        const meshes = child(c, T.CSPRITE_VARIANT_MESHES);
        for (const m of meshes ? meshes.children : []) addSprite(m, null);
      } else addSprite(c, null);
    }
  } else if (node.type === T.HSPRITE) {
    addTree(node);
  } else {
    addSprite(node, null);
    if (!verts) addTree(node);
  }

  function mergeInto(sub, xform) {
    const base = verts;
    for (let i = 0; i < sub.pos.length / 3; i++) {
      let x = sub.pos[i * 3] * xform.s, y = sub.pos[i * 3 + 1] * xform.s, z = sub.pos[i * 3 + 2] * xform.s;
      if (xform.ry) {
        const c = Math.cos(xform.ry), sn = Math.sin(xform.ry);
        const x2 = z * sn + x * c, z2 = z * c - x * sn;
        x = x2; z = z2;
      }
      x += xform.t[0]; y += xform.t[1]; z += xform.t[2];
      pos.push(x, y, z);
      bb[0] = Math.min(bb[0], x); bb[1] = Math.min(bb[1], y); bb[2] = Math.min(bb[2], z);
      bb[3] = Math.max(bb[3], x); bb[4] = Math.max(bb[4], y); bb[5] = Math.max(bb[5], z);
      uv.push(sub.uv[i * 2], sub.uv[i * 2 + 1]);
      nrm.push(sub.nrm[i * 3], sub.nrm[i * 3 + 1], sub.nrm[i * 3 + 2]);
      col.push(sub.col[i * 3], sub.col[i * 3 + 1], sub.col[i * 3 + 2]);
    }
    for (const b of sub.batches) {
      const key = `${b.tex}|${b.col.join(',')}`;
      let bt = batches.get(key);
      if (!bt) { bt = { tex: b.tex, col: b.col, uv: b.uv, idx: [] }; batches.set(key, bt); }
      for (const i of b.idx) bt.idx.push(i + base);
    }
    verts += sub.pos.length / 3;
  }

  if (!pos.length) return null;
  let any = false;
  for (let i = 0; i < col.length; i++) if (col[i] > 0) { any = true; break; }
  if (!any) col.fill(0.8);
  return {
    pos: new Float32Array(pos), uv: new Float32Array(uv), nrm: new Float32Array(nrm), col: new Float32Array(col),
    batches: [...batches.values()].map((b) => ({ tex: b.tex, col: b.col, uv: b.uv, idx: new Uint32Array(b.idx) })),
    bbox: bb,
  };
}

// ---- worlds and zones

// zoneProxies reads a world file's zone table, which is in the world base at
// its end: [{index, offset, bbox:[x0,y0,z0,x1,y1,z1]}]. read(off, len) reads
// the file.
export async function zoneProxies(read, size) {
  const head = new DataView(await read(0, 64));
  if (!magicOK(head.buffer)) throw new Error('not an ESF world');
  const data = Number(head.getBigInt64(16, true));
  const root = new DataView(await read(data, 24));
  const rootSize = root.getInt32(4, true);
  const worldSize = root.getInt32(16, true);
  const tail = data + 12 + 12 + worldSize;
  const end = Math.min(size, data + 12 + rootSize);
  if (end - tail <= 0 || end - tail > (8 << 20)) throw new Error('the world base is not where it should be');
  const buf = await read(tail, end - tail);
  const parsed = parseFrom(buf, 0);
  const st = [...parsed.roots];
  let prox = null;
  while (st.length) {
    const n = st.pop();
    if (n.type === T.ZONE_PROXIES) { prox = n; break; }
    for (const c of n.children) st.push(c);
  }
  if (!prox) throw new Error('no zone table in this world');
  const dv = parsed.dv;
  const count = dv.getInt32(prox.body, true);
  const out = [];
  for (let i = 0; i < count; i++) {
    const p = prox.body + 4 + i * 56;
    if (p + 56 > dv.byteLength) break;
    const offset = Number(dv.getBigInt64(p, true));
    const bbox = [];
    for (let k = 0; k < 6; k++) bbox.push(dv.getFloat32(p + 32 + k * 4, true));
    out.push({ index: i, offset, bbox });
  }
  return out;
}

export function readPreTranslations(dv, zoneNode) {
  const base = child(zoneNode, T.ZONE_BASE);
  const pt = base ? child(base, T.ZONE_PRETRANS) : null;
  if (!pt) return [];
  const n = dv.getInt32(pt.body, true);
  if (n < 0 || n > 4096) return [];
  const out = [];
  for (let i = 0; i < n; i++) {
    const p = pt.body + 4 + i * 12;
    out.push([dv.getFloat32(p, true), dv.getFloat32(p + 4, true), dv.getFloat32(p + 8, true)]);
  }
  return out;
}

// zoneTerrain is a zone record's terrain as draw batches, each with its
// base layer's texture and UV transform.
export function zoneTerrain(parsed) {
  const zone = parsed.roots.find((r) => r.type === T.ZONE) || parsed.roots[0];
  if (!zone) return null;
  const dv = parsed.dv;
  const pre = readPreTranslations(dv, zone);
  const res = child(zone, T.ZONE_RESOURCES);
  if (!res) return null;
  const pos = [], nrm = [], uv = [], col = [];
  const batches = new Map();
  let verts = 0;
  let ymin = Infinity, ymax = -Infinity;
  for (const ss of childrenOf(res, T.SUBSPRITE)) {
    const pb = child(ss, T.PRIMBUF);
    if (!pb) continue;
    const geom = readPrimBuffer(dv, pb, pre);
    const mats = spriteMaterials(parsed, ss);
    for (const L of geom.lists) {
      const base = verts;
      for (let i = 0; i < L.count; i++) {
        pos.push(L.pos[i * 3], L.pos[i * 3 + 1], L.pos[i * 3 + 2]);
        ymin = Math.min(ymin, L.pos[i * 3 + 1]);
        ymax = Math.max(ymax, L.pos[i * 3 + 1]);
        nrm.push(L.nrm[i * 3], L.nrm[i * 3 + 1], L.nrm[i * 3 + 2]);
        uv.push(L.uv[i * 2], L.uv[i * 2 + 1]);
        col.push(L.col[i * 3], L.col[i * 3 + 1], L.col[i * 3 + 2]);
      }
      const layer = (mats[L.material] || [])[0] || { tex: 0, col: [255, 255, 255, 255], uvt: null };
      const key = `${layer.tex}|${layer.col.join(',')}|${uvCoeffs(layer.uvt).join(',')}`;
      let bt = batches.get(key);
      if (!bt) { bt = { tex: layer.tex, col: layer.col, uv: uvCoeffs(layer.uvt), idx: [] }; batches.set(key, bt); }
      stripIndices(L.count, base, bt.idx);
      verts += L.count;
    }
  }
  if (!verts) return null;
  return {
    pos: new Float32Array(pos), nrm: new Float32Array(nrm), uv: new Float32Array(uv), col: new Float32Array(col),
    batches: [...batches.values()].map((b) => ({ tex: b.tex, col: b.col, uv: b.uv, idx: new Uint32Array(b.idx) })),
    ymin, ymax,
  };
}
