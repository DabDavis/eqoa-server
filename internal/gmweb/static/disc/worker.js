// The disc worker: everything that reads the GM's own game disc, off the
// page's thread. The image is read in place (a File is a reference, never
// copied whole); the four small files the models, the icons and the spell
// visuals need are kept in this site's private storage (OPFS), so after one
// load they work without the disc, with the three more the spell-visual
// preview reads. Nothing read here leaves the browser (the preview is a page
// in this same browser, handed its files as Blobs).

import { readTOC, WORLD_FILES } from './iso.js';
import * as esf from './esfcore.js';

let disc = null; // {file, files: Map, label}
const containers = new Map(); // name -> Promise<parsed>
const zoneTables = new Map(); // world file -> Promise<zones>
const KEEP = ['DATA/CHAR.ESF', 'DATA/ITEM.ESF', 'DATA/ITEMICON.ESF', 'DATA2/SPELLFX.CSF', 'DATA2/FX.CSF', 'DATA2/CHARCUST.CSF', 'DATA2/CHARFACE.CSF'];
// the files the spell-visual preview asks for, by the names it asks with
// (eqoa-rust-client-vanilla's fxview::FILES): nothing else is handed to it
const PREVIEW_FILES = { 'CHARCUST.CSF': 'DATA2/CHARCUST.CSF', 'CHARFACE.CSF': 'DATA2/CHARFACE.CSF', 'CHAR.ESF': 'DATA/CHAR.ESF', 'SPELLFX.CSF': 'DATA2/SPELLFX.CSF', 'FX.CSF': 'DATA2/FX.CSF', 'ITEM.ESF': 'DATA/ITEM.ESF' };
const OPFS_DIR = 'gm-disc';

const read = (file, off, len) => file.slice(off, off + len).arrayBuffer();

async function opfsDir(create) {
  const root = await navigator.storage.getDirectory();
  return root.getDirectoryHandle(OPFS_DIR, { create });
}

async function opfsFile(name) {
  try {
    const dir = await opfsDir(false);
    const fh = await dir.getFileHandle(name.replace(/\//g, '_'));
    return await fh.getFile();
  } catch {
    return null;
  }
}

async function keepInOPFS(name, entry) {
  try {
    const have = await opfsFile(name);
    if (have && have.size === entry.size) return true;
    const dir = await opfsDir(true);
    const fh = await dir.getFileHandle(name.replace(/\//g, '_'), { create: true });
    const w = await fh.createWritable();
    await w.write(disc.file.slice(entry.offset, entry.offset + entry.size));
    await w.close();
    return true;
  } catch {
    return false; // a browser without writable OPFS: the disc is needed each visit
  }
}

// container is a parsed ESF by its disc path: off the disc when it is open,
// else the copy kept in OPFS.
function container(name) {
  if (!containers.has(name)) {
    containers.set(name, (async () => {
      let buf = null;
      const e = disc?.files.get(name);
      if (e) buf = await read(disc.file, e.offset, e.size);
      else {
        const f = await opfsFile(name);
        if (f) buf = await f.arrayBuffer();
      }
      if (!buf) throw new Error('load your game disc first (' + name + ')');
      // a .CSF is the same container compressed
      if (!esf.magicOK(buf)) buf = await esf.inflateCSF(buf);
      return esf.parseESF(buf);
    })());
    containers.get(name).catch(() => containers.delete(name));
  }
  return containers.get(name);
}

const ops = {
  async open({ file }) {
    const toc = await readTOC((off, len) => read(file, off, len), file.size);
    const files = toc.files;
    const frontiers = files.has('SLUS_207.44');
    const has = (p) => files.has(p);
    if (!has('DATA/CHAR.ESF') || !has('DATA/TUNARIA.ESF')) {
      throw new Error('this disc image is not EverQuest Online Adventures (no DATA/CHAR.ESF)');
    }
    disc = { file, files, label: toc.label };
    containers.clear();
    zoneTables.clear();
    const kept = [];
    for (const k of KEEP) if (files.has(k) && await keepInOPFS(k, files.get(k))) kept.push(k);
    return { label: toc.label, frontiers, files: files.size, kept, worlds: Object.entries(WORLD_FILES).filter(([, f]) => has(f)).map(([w]) => Number(w)) };
  },

  async status() {
    const kept = [];
    for (const k of KEEP) if (await opfsFile(k)) kept.push(k);
    return { open: !!disc, label: disc?.label || '', kept };
  },

  async forget() {
    disc = null;
    containers.clear();
    try { const root = await navigator.storage.getDirectory(); await root.removeEntry(OPFS_DIR, { recursive: true }); } catch { /* nothing kept */ }
    return { ok: true };
  },

  // model: a character body (CHAR.ESF) or an item (ITEM.ESF) by DictID, as a
  // mesh with its textures
  async model({ id, kind }) {
    const parsed = await container(kind === 'item' ? 'DATA/ITEM.ESF' : 'DATA/CHAR.ESF');
    const node = parsed.dict.get(id | 0);
    if (!node) return { missing: true };
    const mesh = esf.buildModel(parsed, node);
    if (!mesh) return { missing: true, type: node.type };
    const textures = {};
    const transfer = [mesh.pos.buffer, mesh.uv.buffer, mesh.nrm.buffer, mesh.col.buffer];
    for (const b of mesh.batches) {
      transfer.push(b.idx.buffer);
      if (b.tex && !(b.tex in textures)) {
        const img = esf.surfaceByID(parsed, b.tex);
        if (img) { textures[b.tex] = img; transfer.push(img.data.buffer); }
      }
    }
    return { mesh, textures, type: node.type, __transfer: transfer };
  },

  async icon({ id }) {
    const parsed = await container('DATA/ITEMICON.ESF');
    const img = esf.surfaceByID(parsed, id | 0);
    if (!img) return { missing: true };
    return { ...img, __transfer: [img.data.buffer] };
  },

  // previewFiles: the files the spell-visual preview asks for (only those
  // PREVIEW_FILES names), as Blobs: off the disc when it is open, else the
  // copies kept; the ones neither has are listed as missing
  async previewFiles({ names }) {
    const files = [], missing = [];
    for (const n of names || []) {
      const path = PREVIEW_FILES[String(n).toUpperCase()];
      if (!path) continue;
      const e = disc?.files.get(path);
      const blob = e ? disc.file.slice(e.offset, e.offset + e.size) : await opfsFile(path);
      if (blob) files.push([n, blob]);
      else missing.push(n);
    }
    return { files, missing };
  },

  // spellVisuals: every spell visual on the disc (SPELLFX.CSF's 0xC200
  // records, which 0x00B1 names by DictID) and what each one's event lists
  // hold; only the DictIDs ever leave the browser (the page plays and sets
  // them by id)
  async spellVisuals() {
    const parsed = await container('DATA2/SPELLFX.CSF');
    return { visuals: esf.spellVisuals(parsed) };
  },

  async zones({ world }) {
    const name = WORLD_FILES[world];
    if (!name) return { zones: [], note: 'this world is not on the disc' };
    if (!disc) throw new Error('load your game disc first');
    const e = disc.files.get(name);
    if (!e) return { zones: [], note: name + ' is not on this disc' };
    if (!zoneTables.has(name)) {
      zoneTables.set(name, esf.zoneProxies((off, len) => read(disc.file, e.offset + off, len), e.size));
      zoneTables.get(name).catch(() => zoneTables.delete(name));
    }
    const zones = await zoneTables.get(name);
    return { zones, file: name, size: e.size, key: `${name}|${e.size}|${disc.label}` };
  },

  // zoneSurface: a surface a world's zones hold, by its DictID, out of the
  // zones round (x, z): the stable signs' horseshoe for the map's coaches
  async zoneSurface({ world, x, z, id }) {
    const name = WORLD_FILES[world];
    if (!disc || !name) return { missing: true };
    const e = disc.files.get(name);
    if (!e) return { missing: true };
    const zl = await ops.zones({ world });
    for (const q of zl.zones) {
      if (!q.bbox || x < q.bbox[0] || x > q.bbox[3] || z < q.bbox[2] || z > q.bbox[5]) continue;
      const head = new DataView(await read(disc.file, e.offset + q.offset, 12));
      const len = 12 + head.getInt32(4, true);
      if (len <= 12 || len > 128 << 20) continue;
      const img = esf.surfaceByID(esf.parseFrom(await read(disc.file, e.offset + q.offset, len), 0), id | 0);
      if (img) return { ...img, __transfer: [img.data.buffer] };
    }
    return { missing: true };
  },

  async tile({ world, zone, size = 512 }) {
    const name = WORLD_FILES[world];
    if (!disc || !name) throw new Error('load your game disc first');
    const e = disc.files.get(name);
    const zl = await ops.zones({ world });
    const z = zl.zones[zone];
    if (!z) throw new Error('no zone ' + zone);
    const head = new DataView(await read(disc.file, e.offset + z.offset, 12));
    const len = 12 + head.getInt32(4, true);
    if (len <= 12 || len > 128 << 20) throw new Error('zone ' + zone + ' does not read');
    const parsed = esf.parseFrom(await read(disc.file, e.offset + z.offset, len), 0);
    const terrain = esf.zoneTerrain(parsed);
    if (!terrain) return { empty: true, bbox: z.bbox };
    // the tile is the zone's 2,000-unit cell, so neighbouring tiles meet
    const cx = (z.bbox[0] + z.bbox[3]) / 2, cz = (z.bbox[2] + z.bbox[5]) / 2;
    const x0 = Math.floor(cx / 2000) * 2000, z0 = Math.floor(cz / 2000) * 2000;
    const rect = [x0, z0, x0 + 2000, z0 + 2000];
    const { bitmap, blob } = await drawTile(parsed, terrain, rect, size);
    return { bitmap, blob, rect, __transfer: [bitmap] };
  },
};

// ---- the tile renderer: a zone's terrain from straight above, north (-z)
// up, the light from the north-west, into an offscreen canvas

let gl = null, prog = null, canvasGL = null;

const VS = `#version 300 es
in vec3 aPos; in vec3 aNrm; in vec2 aUV;
uniform vec4 uRect; uniform vec2 uY; uniform vec3 uU; uniform vec3 uV;
out vec2 vUV; out vec3 vN;
void main() {
  float x = (aPos.x - uRect.x) / (uRect.z - uRect.x) * 2.0 - 1.0;
  float y = 1.0 - (aPos.z - uRect.y) / (uRect.w - uRect.y) * 2.0;
  float d = 1.0 - (aPos.y - uY.x) / max(1.0, uY.y - uY.x) * 2.0;
  gl_Position = vec4(x, y, clamp(d, -1.0, 1.0), 1.0);
  vUV = vec2(aUV.x * uU.x + aUV.y * uU.y + uU.z, aUV.x * uV.x + aUV.y * uV.y + uV.z);
  vN = aNrm;
}`;
const FS = `#version 300 es
precision mediump float;
in vec2 vUV; in vec3 vN;
uniform sampler2D uTex; uniform vec4 uTint; uniform float uHasTex;
out vec4 o;
void main() {
  vec4 t = uHasTex > 0.5 ? texture(uTex, vUV) : vec4(0.42, 0.47, 0.38, 1.0);
  vec3 n = normalize(vN);
  if (n.y < 0.0) n = -n;
  float lit = 0.38 + 0.62 * max(dot(n, normalize(vec3(-0.5, 1.0, -0.5))), 0.0);
  o = vec4(t.rgb * uTint.rgb * lit, 1.0);
}`;

function initGL(size) {
  if (gl && canvasGL.width === size) return;
  canvasGL = new OffscreenCanvas(size, size);
  gl = canvasGL.getContext('webgl2', { antialias: true, preserveDrawingBuffer: true });
  if (!gl) throw new Error('this browser has no WebGL2 for drawing the map');
  const sh = (type, src) => {
    const s = gl.createShader(type);
    gl.shaderSource(s, src);
    gl.compileShader(s);
    if (!gl.getShaderParameter(s, gl.COMPILE_STATUS)) throw new Error(gl.getShaderInfoLog(s));
    return s;
  };
  prog = gl.createProgram();
  gl.attachShader(prog, sh(gl.VERTEX_SHADER, VS));
  gl.attachShader(prog, sh(gl.FRAGMENT_SHADER, FS));
  gl.linkProgram(prog);
  if (!gl.getProgramParameter(prog, gl.LINK_STATUS)) throw new Error(gl.getProgramInfoLog(prog));
}

async function drawTile(parsed, t, rect, size) {
  initGL(size);
  gl.viewport(0, 0, size, size);
  gl.clearColor(0.09, 0.11, 0.15, 1);
  gl.clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT);
  gl.enable(gl.DEPTH_TEST);
  gl.depthFunc(gl.LEQUAL);
  gl.disable(gl.CULL_FACE);
  gl.useProgram(prog);
  const vao = gl.createVertexArray();
  gl.bindVertexArray(vao);
  const bufs = [];
  const attr = (name, data, n) => {
    const b = gl.createBuffer();
    bufs.push(b);
    gl.bindBuffer(gl.ARRAY_BUFFER, b);
    gl.bufferData(gl.ARRAY_BUFFER, data, gl.STATIC_DRAW);
    const loc = gl.getAttribLocation(prog, name);
    gl.enableVertexAttribArray(loc);
    gl.vertexAttribPointer(loc, n, gl.FLOAT, false, 0, 0);
  };
  attr('aPos', t.pos, 3);
  attr('aNrm', t.nrm, 3);
  attr('aUV', t.uv, 2);
  const ibo = gl.createBuffer();
  bufs.push(ibo);
  gl.uniform4f(gl.getUniformLocation(prog, 'uRect'), rect[0], rect[1], rect[2], rect[3]);
  gl.uniform2f(gl.getUniformLocation(prog, 'uY'), t.ymin, t.ymax);
  const uU = gl.getUniformLocation(prog, 'uU'), uV = gl.getUniformLocation(prog, 'uV');
  const uTint = gl.getUniformLocation(prog, 'uTint'), uHas = gl.getUniformLocation(prog, 'uHasTex');
  gl.uniform1i(gl.getUniformLocation(prog, 'uTex'), 0);
  const texs = new Map();
  for (const b of t.batches) {
    let tex = texs.get(b.tex);
    if (tex === undefined) {
      tex = null;
      const img = b.tex ? esf.surfaceByID(parsed, b.tex) : null;
      if (img) {
        tex = gl.createTexture();
        gl.bindTexture(gl.TEXTURE_2D, tex);
        gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, img.w, img.h, 0, gl.RGBA, gl.UNSIGNED_BYTE, new Uint8Array(img.data.buffer));
        gl.generateMipmap(gl.TEXTURE_2D);
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR_MIPMAP_LINEAR);
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.REPEAT);
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.REPEAT);
      }
      texs.set(b.tex, tex);
    }
    gl.activeTexture(gl.TEXTURE0);
    gl.bindTexture(gl.TEXTURE_2D, tex);
    gl.uniform1f(uHas, tex ? 1 : 0);
    gl.uniform4f(uTint, b.col[0] / 255, b.col[1] / 255, b.col[2] / 255, 1);
    gl.uniform3f(uU, b.uv[0], b.uv[2], b.uv[4]);
    gl.uniform3f(uV, b.uv[1], b.uv[3], b.uv[5]);
    gl.bindBuffer(gl.ELEMENT_ARRAY_BUFFER, ibo);
    gl.bufferData(gl.ELEMENT_ARRAY_BUFFER, b.idx, gl.DYNAMIC_DRAW);
    gl.drawElements(gl.TRIANGLES, b.idx.length, gl.UNSIGNED_INT, 0);
  }
  // kept as a picture for the next visit (WebP where the browser writes it)
  let blob = null;
  try { blob = await canvasGL.convertToBlob({ type: 'image/webp', quality: 0.85 }); } catch { /* not kept */ }
  const bitmap = canvasGL.transferToImageBitmap();
  for (const tx of texs.values()) if (tx) gl.deleteTexture(tx);
  for (const b of bufs) gl.deleteBuffer(b);
  gl.deleteVertexArray(vao);
  return { bitmap, blob };
}

self.onmessage = async ({ data }) => {
  const { id, op, args } = data;
  try {
    const out = await ops[op](args || {});
    const transfer = out?.__transfer || [];
    if (out) delete out.__transfer;
    self.postMessage({ id, result: out }, transfer);
  } catch (e) {
    self.postMessage({ id, error: e.message || String(e) });
  }
};
