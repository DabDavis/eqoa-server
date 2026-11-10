// The GM's own game disc, for the map's terrain, the models and the spell
// visuals: picked here, read in place by the disc worker, never sent anywhere.
// The models, icons and visuals need four small files that are kept in this
// site's private storage, so they work on later visits without the disc; the
// map's zones are kept as pictures.

import { h, mount, pageHead, button, toast, card, confirmBox, empty } from '../lib/dom.js';
import { Viewer } from '../lib/viewer.js';

let worker = null, seq = 0;
const waiting = new Map();
const st = { open: false, label: '', kept: [], sizes: {}, checked: false };

function call(op, args, transfer) {
  if (!worker) {
    worker = new Worker(new URL('../disc/worker.js', import.meta.url), { type: 'module' });
    worker.onmessage = ({ data }) => {
      const w = waiting.get(data.id);
      if (!w) return;
      waiting.delete(data.id);
      if (data.error) w.reject(new Error(data.error));
      else w.resolve(data.result);
    };
    worker.onerror = (e) => { for (const w of waiting.values()) w.reject(new Error(e.message || 'the disc worker failed')); waiting.clear(); };
  }
  const id = ++seq;
  return new Promise((resolve, reject) => {
    waiting.set(id, { resolve, reject });
    worker.postMessage({ id, op, args }, transfer || []);
  });
}

async function checkKept() {
  if (st.checked) return;
  try {
    const s = await call('status');
    st.kept = s.kept;
    st.sizes = s.sizes || {};
    st.open = s.open;
    st.label = s.label;
  } catch { /* no storage */ }
  st.checked = true;
}

export function discState() {
  const has = (f) => st.open || st.kept.includes(f);
  return { loaded: st.open, label: st.label, models: has('DATA/CHAR.ESF'), icons: has('DATA/ITEMICON.ESF'), items: has('DATA/ITEM.ESF'), visuals: has('DATA2/SPELLFX.CSF'),
    ui: has('DATA2/UI.CSF'), cust: has('DATA2/CHARCUST.CSF'), faces: has('DATA2/CHARFACE.CSF') };
}

// discReady is discState once the kept files are known.
export async function discReady() {
  await checkKept();
  return discState();
}

async function openDisc(file) {
  toast('Reading ' + file.name + '…', 'info', 'It stays on this computer.');
  const out = await call('open', { file });
  visualsOnce = null;
  forgetLooks();
  st.open = true;
  st.label = out.label;
  st.kept = out.kept;
  st.checked = true;
  try { st.sizes = (await call('status')).sizes || {}; } catch { /* kept sizes unknown */ }
  return out;
}

// ---- the page

export async function render(box) {
  await checkKept();
  const input = h('input', { type: 'file', accept: '.iso,.ISO,.bin', class: 'hidden' });
  const statusBox = h('div');
  const pick = button('Choose the disc image…', { class: 'primary', icon: 'disc', onclick: () => input.click() });
  input.addEventListener('change', async () => {
    const f = input.files[0];
    if (!f) return;
    try {
      const out = await openDisc(f);
      toast('Disc loaded: ' + out.label, 'ok', out.frontiers ? 'EverQuest Online Adventures: Frontiers' : 'Not the Frontiers disc: some ids may not match');
      draw();
    } catch (e) { toast(e.message, 'error'); }
  });
  const draw = () => {
    const s = discState();
    mount(statusBox, h('dl', { class: 'kv' },
      h('dt', null, 'This visit'), h('dd', null, s.loaded ? h('span', { class: 'badge ok' }, 'loaded: ' + st.label) : h('span', { class: 'badge' }, 'not loaded')),
      h('dt', null, 'Models'), h('dd', null, s.models ? 'available (character bodies and items)' : 'need the disc once'),
      h('dt', null, 'Item icons'), h('dd', null, s.icons ? 'available' : 'need the disc once'),
      h('dt', null, 'Spell icons, armour'), h('dd', null, s.ui && s.cust ? 'available (the pickers show them)' : 'need the disc once'),
      h('dt', null, 'Spell visuals'), h('dd', null, s.visuals ? 'available (Spell visuals lists them)' : 'need the disc once'),
      h('dt', null, 'World map'), h('dd', null, s.loaded ? 'terrain is drawn zone by zone and kept' : 'zones drawn before are shown; new ones need the disc')));
  };
  draw();
  mount(box,
    pageHead('Game disc', 'The map\'s terrain, the NPC and item models and the item icons come from your own copy of the game, read here in the browser. Nothing is uploaded: the server has no game data.'),
    h('div', { class: 'cols' },
      card('Your disc image', h('div', { class: 'stack' },
        h('p', null, 'Choose the image of your EverQuest Online Adventures: Frontiers disc (an .iso, the same one the game\'s web page takes).'),
        h('div', { class: 'row' }, pick, input),
        statusBox)),
      card('What is kept', h('div', { class: 'stack' },
        h('p', null, 'So the models, the icons, the spell visuals and their preview work on your next visit without the disc, eight files are kept in this site\'s private storage in your browser (about 160 MB): CHAR.ESF, ITEM.ESF, ITEMICON.ESF, UI.CSF, SPELLFX.CSF, FX.CSF, CHARCUST.CSF and CHARFACE.CSF. Map zones and the pickers\' model pictures are kept as pictures once drawn.'),
        button('Forget them', { class: 'danger', onclick: async () => {
          if (!(await confirmBox({ title: 'Forget the kept files?', message: 'The models need the disc again next time, and the map is drawn again.', confirmLabel: 'Forget', danger: true }))) return;
          await call('forget');
          visualsOnce = null;
          forgetLooks();
          await clearTiles();
          st.kept = []; st.open = false;
          draw();
          toast('Forgotten', 'ok');
        } })))));
}

// ---- the spell visuals, for the Spell visuals page

let visualsOnce = null;

// spellVisualsOnDisc is every spell visual on the disc (or the kept
// SPELLFX.CSF): [{id, version, lists: [{events, kinds}] x3}].
export async function spellVisualsOnDisc() {
  await checkKept();
  if (!visualsOnce) {
    visualsOnce = call('spellVisuals').then((out) => out.visuals);
    visualsOnce.catch(() => { visualsOnce = null; });
  }
  return visualsOnce;
}

// previewFiles is the files the spell-visual preview asks for, as
// [[name, Blob]], and the names neither the disc nor the kept copies have.
export async function previewFiles(names) {
  await checkKept();
  return call('previewFiles', { names });
}

// ---- models and icons for other pages

const modelCache = new Map();

function model(id, kind, swaps) {
  const key = kind + ':' + id + (swaps ? ':' + JSON.stringify(swaps) : '');
  if (!modelCache.has(key)) {
    modelCache.set(key, call('model', { id, kind, swaps }));
    modelCache.get(key).catch(() => modelCache.delete(key));
  }
  return modelCache.get(key);
}

function viewerCard(title, id, kind, note, opts) {
  return card(title, viewerBox(title, id, kind, note, opts));
}

// viewerBox is a model on the turntable. opts.swaps dress a body (lib/look.js
// npcSwaps); opts.watch(cb) hands cb {id, swaps} whenever the form changes
// them, and the model is drawn again as it would look, the view kept.
export function viewerBox(title, id, kind, note, opts = {}) {
  const canvas = h('canvas', { 'aria-label': title });
  const msg = h('div', { class: 'viewer-note' }, 'Loading…');
  const holder = h('div', { class: 'viewer' }, canvas, msg);
  const body = h('div', { class: 'stack' }, holder, note ? h('p', { class: 'faint small' }, note) : null);
  let v = null, seq = 0;
  const draw = async (mid, swaps, keep) => {
    const my = ++seq;
    try {
      const out = await model(mid, kind, swaps);
      if (my !== seq) return;
      if (out.missing) { msg.textContent = 'This id is not a model on the disc.'; return; }
      if (!v) v = new Viewer(canvas);
      v.setModel(out, keep);
      msg.textContent = keep ? '' : 'Drag to turn, wheel to zoom';
      if (!keep) setTimeout(() => { if (msg.textContent.startsWith('Drag')) msg.textContent = ''; }, 2500);
    } catch (e) { msg.textContent = e.message; }
  };
  draw(id, opts.swaps, false);
  opts.watch?.(({ id: nid, swaps }) => { if (nid) draw(nid, swaps, true); });
  return body;
}

function needDisc(title, what) {
  return card(title, h('div', { class: 'stack' }, h('p', { class: 'muted' }, `Load your game disc to see ${what}.`),
    button('Game disc page', { class: 'small', onclick: () => { location.hash = '#/disc'; } })));
}

// modelPanel is an NPC's body, off CHAR.ESF: dressed in its armour, robe and
// tint as a PS2 dresses it when opts.swaps are given (and kept up with the
// form by opts.watch), else as modelled.
export async function modelPanel(npc, opts = {}) {
  await checkKept();
  if (!npc.modelid) return h('div');
  if (!discState().models) return needDisc('Model', 'this NPC\'s model');
  const dressed = opts.swaps && discState().cust;
  return viewerCard('Model', npc.modelid, 'char', dressed
    ? 'Dressed in its armour sets, robe and tint as a PS2 dresses it, as the form has them now (save to keep them). Held items, the helm and race scaling are not drawn.'
    : 'The body as modelled: worn armour, held items and race scaling are not drawn.', dressed ? opts : {});
}

// itemPanel is an item's icon and its look, off ITEMICON.ESF and ITEM.ESF: a
// held item's model; an armour piece or a robe on a body, dressed as a PS2
// dresses it (dress: lib/look.js itemDress, of the form's values when
// watch(cb) hands them), a helm its sprite.
export async function itemPanel(item, { dress, watch } = {}) {
  await checkKept();
  const s = discState();
  if (!s.icons && !s.items) return needDisc('Look', 'this item\'s icon and model');
  const parts = [];
  if (item.itemicon && s.icons) {
    const c = h('canvas', { class: 'icon-cell plate', width: 1, height: 1 }); // a 128x64 plate, at its own shape
    parts.push(h('div', { class: 'row' }, c, h('span', { class: 'muted small' }, 'Its icon, as the inventory shows it')));
    iconInto(c, item.itemicon);
  }
  const box = h('div', { class: 'stack' }, ...parts);
  const d = dress ? dress(item) : null;
  if (d?.swaps && s.models && s.cust) {
    box.appendChild(viewerBox('Worn', d.body, 'char', 'Worn on a body as a PS2 dresses it, as the form has it now; the rest of the body bare.', {
      swaps: d.swaps, watch: watch ? (cb) => watch((v) => { const nd = dress(v); if (nd?.swaps) cb({ id: nd.body, swaps: nd.swaps }); }) : null,
    }));
  } else if (d?.helm && s.cust) {
    box.appendChild(viewerBox('Helm', d.helm, 'helm', 'The helm of its set, as attached over the head.'));
  } else if (item.model && s.items && !d) {
    box.appendChild(viewerBox('Model', item.model, 'item'));
  }
  return card('Look', box);
}

const iconCache = new Map();
export async function iconInto(canvas, id) {
  try {
    if (!iconCache.has(id)) iconCache.set(id, call('icon', { id }));
    const img = await iconCache.get(id);
    if (img.missing) return;
    // the part the inventory shows: VIWndList::DrawIcon blits its top-left 114x44
    canvas.width = Math.min(114, img.w);
    canvas.height = Math.min(44, img.h);
    canvas.getContext('2d').putImageData(new ImageData(new Uint8ClampedArray(img.data), img.w, img.h), 0, 0);
  } catch { /* no disc */ }
}

// ---- the pickers' pictures (lib/look.js): what is on the disc for a kind of
// look, any surface of the picture files, and models drawn small (kept in
// IndexedDB beside the map's zones, so a picker fills at once next time)

const catalogs = new Map(), surfaces = new Map(), thumbs = new Map();

function forgetLooks() {
  catalogs.clear();
  surfaces.clear();
  thumbs.clear();
}

// lookCatalog is every id on the disc of a kind ('itemicon', 'ui', 'char',
// 'item'): [{id, w, h, type}], ids unsigned.
export async function lookCatalog(kind) {
  await checkKept();
  if (!catalogs.has(kind)) {
    const p = call('catalog', { kind }).then((o) => o.ids);
    catalogs.set(kind, p);
    p.catch(() => catalogs.delete(kind));
  }
  return catalogs.get(kind);
}

// surfaceBitmap is a picture of a picture file ('ITEMICON', 'UI',
// 'CHARCUST', 'CHARFACE', 'CHAR') by DictID as an ImageBitmap, or null; its
// box is the part of it that is not clear ([x0, y0, x1, y1]).
export function surfaceBitmap(file, id) {
  const key = file + ':' + (Number(id) >>> 0);
  if (!surfaces.has(key)) {
    const p = (async () => {
      await checkKept();
      const img = await call('surface', { file, id: Number(id) | 0 });
      if (!img || img.missing) return null;
      const px = new Uint8ClampedArray(img.data);
      let x0 = img.w, y0 = img.h, x1 = 0, y1 = 0;
      for (let y = 0; y < img.h; y++) for (let x = 0; x < img.w; x++) {
        if (px[(y * img.w + x) * 4 + 3] === 0) continue;
        if (x < x0) x0 = x;
        if (y < y0) y0 = y;
        if (x >= x1) x1 = x + 1;
        if (y >= y1) y1 = y + 1;
      }
      const bmp = await createImageBitmap(new ImageData(px, img.w, img.h));
      bmp.box = x1 > x0 ? [x0, y0, x1, y1] : [0, 0, img.w, img.h];
      return bmp;
    })();
    surfaces.set(key, p);
    p.catch(() => surfaces.delete(key));
  }
  return surfaces.get(key).catch(() => null);
}

const THUMB_FILE = { char: 'DATA/CHAR.ESF', item: 'DATA/ITEM.ESF', helm: 'DATA2/CHARCUST.CSF' };

// modelThumb is a model ('char' a body, 'item' a held item, 'helm' a helm)
// drawn small, as an ImageBitmap, or null: kept from before, else drawn by
// the worker one at a time and kept.
export function modelThumb(kind, id) {
  const key = kind + ':' + (Number(id) >>> 0);
  if (!thumbs.has(key)) {
    const p = (async () => {
      await checkKept();
      const size = st.sizes[THUMB_FILE[kind]] || 0;
      const ikey = `thumb|2|${kind}|${size}|${Number(id) >>> 0}`;
      const kept = await tileGet(ikey);
      if (kept?.missing) return null;
      if (kept?.blob) return createImageBitmap(kept.blob);
      if (!discState().models && kind !== 'helm') return null;
      const out = await call('thumb', { kind, id: Number(id) | 0, size: 128 });
      if (!out || out.missing) { if (size) tilePut(ikey, { missing: true }); return null; }
      if (out.blob && size) tilePut(ikey, { blob: out.blob });
      return out.bitmap;
    })();
    thumbs.set(key, p);
    p.catch(() => thumbs.delete(key));
  }
  return thumbs.get(key).catch(() => null);
}

// ---- the stables' horseshoe, for the map's coaches

// The texture of the stable sign by Coachman Ronks in Freeport (the 0x1000
// record 0x9BE57886 of TUNARIA.ESF's zone round it, on no other file of the
// disc: found by eqoa-rust-client-vanilla's map, which draws its coaches with
// it), and where it stands.
const HORSESHOE = { world: 0, x: 25281, z: 15707, id: 0x9BE57886 | 0 };
let horseshoe = null;

// stableHorseshoe is that texture as an ImageBitmap: off the GM's disc once,
// then kept in IndexedDB beside the map's zones; null without either.
export function stableHorseshoe() {
  if (!horseshoe) {
    horseshoe = (async () => {
      const kept = await tileGet('icon|horseshoe');
      if (kept?.blob) return createImageBitmap(kept.blob);
      const img = await call('zoneSurface', HORSESHOE);
      if (!img || img.missing) return null;
      const c = new OffscreenCanvas(img.w, img.h);
      c.getContext('2d').putImageData(new ImageData(new Uint8ClampedArray(img.data), img.w, img.h), 0, 0);
      const blob = await c.convertToBlob();
      tilePut('icon|horseshoe', { blob });
      return createImageBitmap(blob);
    })().then((b) => { if (!b) horseshoe = null; return b; }, () => { horseshoe = null; return null; });
  }
  return horseshoe;
}

// ---- the map's terrain: zone pictures, kept in IndexedDB

function idb() {
  return new Promise((resolve, reject) => {
    const r = indexedDB.open('eqoa-gm', 1);
    r.onupgradeneeded = () => r.result.createObjectStore('tiles');
    r.onsuccess = () => resolve(r.result);
    r.onerror = () => reject(r.error);
  });
}

async function tileGet(key) {
  try {
    const db = await idb();
    return await new Promise((resolve) => {
      const q = db.transaction('tiles').objectStore('tiles').get(key);
      q.onsuccess = () => resolve(q.result || null);
      q.onerror = () => resolve(null);
    });
  } catch { return null; }
}

async function tilePut(key, value) {
  try {
    const db = await idb();
    db.transaction('tiles', 'readwrite').objectStore('tiles').put(value, key);
  } catch { /* not kept */ }
}

async function clearTiles() {
  try { const db = await idb(); db.transaction('tiles', 'readwrite').objectStore('tiles').clear(); } catch { /* nothing kept */ }
}

// discTerrain is a world's zone pictures as {tiles: [{x0, z0, x1, z1, img}]},
// returned at once and filled as they come: kept ones first, the rest drawn by
// the worker (with the disc) one zone after another. onTile redraws; onStatus
// says how far. cancel() stops the drawing (the map was left).
export function discTerrain(world, onTile, onStatus) {
  const out = { tiles: [], cancelled: false, cancel() { this.cancelled = true; } };
  fill(out, world, onTile, onStatus).catch((e) => onStatus('Terrain: ' + e.message));
  return out;
}

async function fill(out, world, onTile, onStatus) {
  await checkKept();
  const index = await tileGet(`index|${world}`);
  // zones kept from an earlier visit show even without the disc
  const kept = new Set();
  if (index) {
    for (const k of index.keys) {
      if (out.cancelled) return;
      const t = await tileGet(k);
      if (t?.empty) { kept.add(k); continue; }
      if (t) { out.tiles.push({ ...t.rect4, img: await createImageBitmap(t.blob) }); kept.add(k); }
    }
    onTile();
  }
  if (!st.open) {
    onStatus(index ? `${out.tiles.length} zones kept from before` : 'Load your disc to draw the terrain');
    return;
  }
  const zl = await call('zones', { world });
  if (!zl.zones.length) { onStatus(zl.note || 'no zones'); return; }
  let n = 0;
  for (const z of zl.zones) {
    n++;
    const key = `tile|${zl.key}|${z.index}`;
    if (kept.has(key)) continue;
    if (out.cancelled) break;
    onStatus(`Drawing the terrain: zone ${n} of ${zl.zones.length}`);
    try {
      const t = await call('tile', { world, zone: z.index, size: 512 });
      if (t.empty) { await tilePut(key, { empty: true }); kept.add(key); continue; }
      const rect4 = { x0: t.rect[0], z0: t.rect[1], x1: t.rect[2], z1: t.rect[3] };
      out.tiles.push({ ...rect4, img: t.bitmap });
      if (t.blob) { await tilePut(key, { rect4, blob: t.blob }); kept.add(key); }
      onTile();
    } catch (e) {
      console.warn('zone', z.index, e);
    }
    await tilePut(`index|${world}`, { keys: [...kept] });
  }
  await tilePut(`index|${world}`, { keys: [...kept] });
  onStatus(`${out.tiles.length} zones of terrain`);
}

export { empty };
