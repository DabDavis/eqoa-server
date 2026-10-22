// The GM's own game disc, for the map's terrain and the models: picked here,
// read in place by the disc worker, never sent anywhere. The models and icons
// need three small files that are kept in this site's private storage, so they
// work on later visits without the disc; the map's zones are kept as pictures.

import { h, mount, pageHead, button, toast, card, confirmBox, empty } from '../lib/dom.js';
import { Viewer } from '../lib/viewer.js';

let worker = null, seq = 0;
const waiting = new Map();
const st = { open: false, label: '', kept: [], checked: false };

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
    st.open = s.open;
    st.label = s.label;
  } catch { /* no storage */ }
  st.checked = true;
}

export function discState() {
  return { loaded: st.open, label: st.label, models: st.open || st.kept.includes('DATA/CHAR.ESF'), icons: st.open || st.kept.includes('DATA/ITEMICON.ESF'), items: st.open || st.kept.includes('DATA/ITEM.ESF') };
}

async function openDisc(file) {
  toast('Reading ' + file.name + '…', 'info', 'It stays on this computer.');
  const out = await call('open', { file });
  st.open = true;
  st.label = out.label;
  st.kept = out.kept;
  st.checked = true;
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
        h('p', null, 'So the models work on your next visit without the disc, three files are kept in this site\'s private storage in your browser (about 160 MB): CHAR.ESF, ITEM.ESF and ITEMICON.ESF. Map zones are kept as pictures once drawn.'),
        button('Forget them', { class: 'danger', onclick: async () => {
          if (!(await confirmBox({ title: 'Forget the kept files?', message: 'The models need the disc again next time, and the map is drawn again.', confirmLabel: 'Forget', danger: true }))) return;
          await call('forget');
          await clearTiles();
          st.kept = []; st.open = false;
          draw();
          toast('Forgotten', 'ok');
        } })))));
}

// ---- models and icons for other pages

const modelCache = new Map();

function model(id, kind) {
  const key = kind + ':' + id;
  if (!modelCache.has(key)) {
    modelCache.set(key, call('model', { id, kind }));
    modelCache.get(key).catch(() => modelCache.delete(key));
  }
  return modelCache.get(key);
}

function viewerCard(title, id, kind, note) {
  return card(title, viewerBox(title, id, kind, note));
}

function viewerBox(title, id, kind, note) {
  const canvas = h('canvas', { 'aria-label': title });
  const msg = h('div', { class: 'viewer-note' }, 'Loading…');
  const holder = h('div', { class: 'viewer' }, canvas, msg);
  const body = h('div', { class: 'stack' }, holder, note ? h('p', { class: 'faint small' }, note) : null);
  (async () => {
    try {
      const out = await model(id, kind);
      if (out.missing) { msg.textContent = 'This id is not a model on the disc.'; return; }
      const v = new Viewer(canvas);
      v.setModel(out);
      msg.textContent = 'Drag to turn, wheel to zoom';
      setTimeout(() => { msg.textContent = ''; }, 2500);
    } catch (e) { msg.textContent = e.message; }
  })();
  return body;
}

function needDisc(title, what) {
  return card(title, h('div', { class: 'stack' }, h('p', { class: 'muted' }, `Load your game disc to see ${what}.`),
    button('Game disc page', { class: 'small', onclick: () => { location.hash = '#/disc'; } })));
}

// modelPanel is an NPC's body, off CHAR.ESF.
export async function modelPanel(npc) {
  await checkKept();
  if (!npc.modelid) return h('div');
  if (!discState().models) return needDisc('Model', 'this NPC\'s model');
  return viewerCard('Model', npc.modelid, 'char', 'The body as modelled: worn armour, held items and race scaling are not drawn.');
}

// itemPanel is an item's icon and its model, off ITEMICON.ESF and ITEM.ESF.
export async function itemPanel(item) {
  await checkKept();
  const s = discState();
  if (!s.icons && !s.items) return needDisc('Look', 'this item\'s icon and model');
  const parts = [];
  if (item.itemicon && s.icons) {
    const c = h('canvas', { class: 'icon-cell big', width: 1, height: 1 });
    parts.push(h('div', { class: 'row' }, c, h('span', { class: 'muted small' }, 'Its icon, as the inventory shows it')));
    iconInto(c, item.itemicon);
  }
  const box = h('div', { class: 'stack' }, ...parts);
  if (item.model && s.items) box.appendChild(viewerBox('Model', item.model, 'item'));
  return card('Look', box);
}

const iconCache = new Map();
export async function iconInto(canvas, id) {
  try {
    if (!iconCache.has(id)) iconCache.set(id, call('icon', { id }));
    const img = await iconCache.get(id);
    if (img.missing) return;
    canvas.width = img.w;
    canvas.height = img.h;
    canvas.getContext('2d').putImageData(new ImageData(new Uint8ClampedArray(img.data), img.w, img.h), 0, 0);
  } catch { /* no disc */ }
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
