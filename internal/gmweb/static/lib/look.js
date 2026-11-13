// Look columns (the catalog's look, looks.go): a number that IS a picture, a
// colour or a palette entry, shown as what it is and chosen by sight. Bodies,
// held items, helms and icons are drawn off the GM's own disc in the browser
// (views/disc.js; nothing of the disc is sent anywhere); without the disc a
// field still takes a number, in hex or decimal, and says how to see more.
//
//   - an id (a body, a held item, an item icon, a spell icon, a visual): its
//     picture, the id in hex with its stored decimal, what wears it, Choose…
//     opens a grid of every one on the disc; the column's store (int32 or
//     uint32) is the form written, so 0xB44E7A96 is kept as -1269925226 where
//     the column keeps ids signed;
//   - a colour (0xRRGGBBAA): RGBA sliders, a swatch, a hex box, the Armor
//     Dyers' colours and the disc's tints; an item's colour also a disc tint
//     (0-14) or plain white, with what a PS2 draws it as;
//   - a palette index (a tint, an armour set, a robe, a hair colour): the
//     entries as swatches or pictures.

import { h, mount, button, dialog, debounce, icon } from './dom.js';
import { op } from './api.js';

const disc = () => import('../views/disc.js');

// ---- numbers

const U32 = 4294967296;

export function u32(n) {
  n = Number(n);
  if (!Number.isFinite(n)) return NaN;
  return ((Math.trunc(n) % U32) + U32) % U32;
}

export function i32(n) {
  const u = u32(n);
  return Number.isNaN(u) ? NaN : u | 0;
}

export function hex8(n) {
  const u = u32(n);
  return Number.isNaN(u) ? '' : '0x' + u.toString(16).toUpperCase().padStart(8, '0');
}

// parseId reads an id as a person types one: hex with 0x, or a whole number,
// signed or not, within 32 bits; NaN when it is neither.
export function parseId(text) {
  const t = String(text ?? '').trim().replace(/[_\s]/g, '');
  if (t === '') return NaN;
  const m = /^(-?)0x([0-9a-f]{1,8})$/i.exec(t);
  if (m) {
    const v = parseInt(m[2], 16);
    return m[1] ? -v : v;
  }
  if (/^[-+]?\d{1,10}$/.test(t)) {
    const v = Number(t);
    if (v >= -2147483648 && v <= 4294967295) return v;
  }
  return NaN;
}

// stored is an id in the form its column keeps (Column.store).
export function stored(col, n) {
  if (col.store === 'int32') return i32(n);
  if (col.store === 'uint32') return u32(n);
  return n;
}

// ---- colours: 0xRRGGBBAA, red in the high byte (chardyes, itempattern.color)

export const rgbaOf = (u) => [(u >>> 24) & 255, (u >>> 16) & 255, (u >>> 8) & 255, u & 255];
export const packRGBA = ([r, g, b, a]) => (((r & 255) << 24) | ((g & 255) << 16) | ((b & 255) << 8) | (a & 255)) >>> 0;

export function cssColour(u) {
  const [r, g, b, a] = rgbaOf(u32(u));
  return `rgba(${r}, ${g}, ${b}, ${(a / 255).toFixed(3)})`;
}

// parseColour reads RRGGBB (opaque) or RRGGBBAA, with or without # or 0x.
export function parseColour(text) {
  let t = String(text ?? '').trim().replace(/^#/, '').replace(/^0x/i, '');
  if (/^[0-9a-f]{6}$/i.test(t)) t += 'FF';
  if (!/^[0-9a-f]{8}$/i.test(t)) return NaN;
  return parseInt(t, 16) >>> 0;
}

const hexRGBA = (u) => u32(u).toString(16).toUpperCase().padStart(8, '0');

// The disc tints a grey is matched among (entity's greyTints): white, dark
// grey, silver, so black is drawn dark grey and not navy.
const GREY_TINTS = new Set([0, 1, 10]);

// nearestTint is the disc tint a PS2 draws a colour in (entity.nearestTint).
export function nearestTint(u, tints) {
  const [r, g, b] = rgbaOf(u32(u));
  const grey = Math.max(r, g, b) - Math.min(r, g, b) <= 24;
  let best = 0, bestD = -1;
  tints.forEach((t, i) => {
    if (grey && !GREY_TINTS.has(i)) return;
    const [tr, tg, tb] = rgbaOf(parseInt(t.rgba, 16) >>> 0);
    const d = (r - tr) ** 2 + (g - tg) ** 2 + (b - tb) ** 2;
    if (bestD < 0 || d < bestD) { best = i; bestD = d; }
  });
  return best;
}

// ---- the disc's customisation tables, as DictIDs: VICSpriteCust's (decomp
// vicspritecust.cpp; eqoa-rust-client-vanilla crates/vi/src/cust.rs, which
// resolves them as here). Identifiers, not art: the art is the GM's disc's.

// Armor[set]: the set's textures in CHARCUST.CSF, chest, boots, bracer, legs, gloves
const ARMOR = [null,
  [0xBF4B7E0A, 0x62444D1A, 0x84D2F275, 0x2CB53288, 0x7AD7D1B1],
  [0x72D073EB, 0xB99BD955, 0x3857E856, 0xA73D0D53, 0xF55FAC7C],
  [0x581046F4, 0xC0EC54C8, 0x1D97BB5F, 0x06CA3DD6, 0x54ECDCFF],
  [0x40B0DDA7, 0xEB416A29, 0x06385212, 0xF50617E7, 0x4328B710],
  [0x39BCF85F, 0xE60EEF91, 0xFF446CCA, 0x75F2A4CF, 0xC41543F8],
  [0xF94BD113, 0xA0A2488D, 0xBED3457E, 0x6F97E30B, 0xBDBA8234],
  [0x2603BB5F, 0x238E0091, 0xEB8B2FCA, 0x8D3F65CF, 0xDB6204F8],
  [0x3FDB0DDB, 0x81343DA5, 0x05628246, 0x0E6C08A3, 0x5C8EA7CC]];
const ARMOR_SLOT = { chest: 0, boots: 1, bracer: 2, legs: 3, gloves: 4 };
// Helm[set]: the helm sprites in CHARCUST.CSF
const HELM = [0, 0x0C6F2FDA, 0x330D43A7, 0x079EAC92, 0x03B8E453, 0x5A51ACC1, 0x7184EC3D, 0x91E411BB, 0x1E25A25F];
// Robe[style]: its chest and legs textures
const ROBE = [[0x455F0D60, 0x7DDA79FA], [0xA89BE30B, 0x0D4477B3], [0x5D2F97E1, 0x455D0BA5], [0x2720732E, 0xB64B5794]];
// Hair[colour]: the hair texture
const HAIR = [0x98F86489, 0x221E7298, 0x6DCEF2D0, 0xFEBCE4D5, 0xD499494B, 0x8931CEE6, 0x9F87C7AF, 0xDC79B7C3];

const SLOT_WORD = { chest: 'chest', boots: 'feet', bracer: 'forearms', legs: 'legs', gloves: 'hands', helm: 'head' };

// The wear slots (itempattern.equipslot) whose model is an armour set, a
// robe, a cape, or an ITEM.ESF model (handlers wornLook, capes.go).
const WEAR_ARMOR = { 1: 'helm', 5: 'chest', 7: 'bracer', 10: 'legs', 11: 'boots', 19: 'gloves' };
const WEAR_ROBE = 2, WEAR_CAPE = 28;
const WEAR_HELD = new Set([12, 13, 14, 15, 16, 17, 18, 20]);

// ---- what wears each id (look_names), once a page's life

const namesOf = new Map();
// the kinds look_names answers (looks.go lookSources)
const NAMED = new Set(['char', 'item', 'itemicon', 'uiicon', 'uiback', 'visual']);

export function lookNames(kind) {
  if (!NAMED.has(kind)) return Promise.resolve(new Map());
  if (!namesOf.has(kind)) {
    const p = op('look_names', { kind }).then((out) => {
      const m = new Map();
      for (const u of out.ids || []) m.set(u.id >>> 0, u);
      return m;
    }, () => new Map());
    namesOf.set(kind, p);
  }
  return namesOf.get(kind);
}

function namesText(u) {
  if (!u) return '';
  const more = u.rows - u.names.length;
  return u.names.join(', ') + (u.rows > 1 ? ` · ${u.rows} rows` : '') + (more > 0 && u.names.length >= 6 ? ' and more' : '');
}

// ---- pictures

// draws only once a canvas is on screen (a picker's grid, a list's cells);
// a canvas taken off the page unseen is let go at the next sweep
let seen = null;
const pending = new Map();
function lazy(el, fn) {
  if (!('IntersectionObserver' in window)) { fn(); return; }
  if (!seen) {
    seen = new IntersectionObserver((entries) => {
      for (const e of entries) {
        if (!e.isIntersecting) continue;
        seen.unobserve(e.target);
        const f = pending.get(e.target);
        pending.delete(e.target);
        if (f) f();
      }
    }, { rootMargin: '200px' });
  }
  // swept after the render that made it (its canvases are on the page by then)
  if (pending.size > 800 && !sweeping) {
    sweeping = true;
    setTimeout(() => {
      sweeping = false;
      for (const old of pending.keys()) if (!old.isConnected) { seen.unobserve(old); pending.delete(old); }
    }, 2000);
  }
  pending.set(el, fn);
  seen.observe(el);
}
let sweeping = false;

// fit draws a picture (or its part crop, [x, y, w, h]) into a canvas of
// w x h CSS pixels, keeping its shape
function fit(canvas, img, w, h, crop) {
  const dpr = Math.min(2, window.devicePixelRatio || 1);
  canvas.width = Math.round(w * dpr);
  canvas.height = Math.round(h * dpr);
  canvas.style.width = w + 'px';
  canvas.style.height = h + 'px';
  const g = canvas.getContext('2d');
  g.clearRect(0, 0, canvas.width, canvas.height);
  if (!img) return;
  const [sx, sy, sw, sh] = crop || [0, 0, img.width, img.height];
  const s = Math.min(canvas.width / sw, canvas.height / sh);
  const dw = sw * s, dh = sh * s;
  g.imageSmoothingEnabled = s < 1.5;
  g.drawImage(img, sx, sy, sw, sh, (canvas.width - dw) / 2, (canvas.height - dh) / 2, dw, dh);
}

// cropOf is the part of a kind's picture the game shows: an item icon's
// top-left 114x44 (VIWndList::DrawIcon), a UI surface's drawn part
function cropOf(kind, img) {
  if (kind === 'itemicon') return [0, 0, Math.min(114, img.width), Math.min(44, img.height)];
  if ((kind === 'uiicon' || kind === 'uiback' || kind === 'cust' || kind === 'texture') && img.box) {
    const [x0, y0, x1, y1] = img.box;
    return [x0, y0, x1 - x0, y1 - y0];
  }
  return null;
}

// pictureOf is the picture of an id of a kind, as an ImageBitmap, or null.
async function pictureOf(kind, id) {
  const d = await disc();
  switch (kind) {
    case 'itemicon': return d.surfaceBitmap('ITEMICON', id);
    case 'uiicon': case 'uiback': return d.surfaceBitmap('UI', id);
    case 'char': return d.modelThumb('char', id);
    case 'item': return d.modelThumb('item', id);
    case 'helm': return d.modelThumb('helm', id);
    case 'cust': return d.surfaceBitmap('CHARCUST', id);
    case 'texture': return (await d.surfaceBitmap('CHARCUST', id)) || (await d.surfaceBitmap('CHARFACE', id)) || d.surfaceBitmap('CHAR', id);
    default: return null;
  }
}

// the shape a kind's picture is shown at
const SHAPE = { itemicon: [76, 30], uiicon: [40, 40], uiback: [46, 34], char: [64, 64], item: [64, 64], helm: [64, 64], cust: [48, 48], texture: [48, 48] };

// hasFile is whether the disc (or its kept copy) has the file a kind's
// pictures come off
async function hasFile(kind) {
  const s = await (await disc()).discReady();
  switch (kind) {
    case 'itemicon': return s.icons;
    case 'uiicon': case 'uiback': return s.ui;
    case 'char': return s.models;
    case 'item': return s.items;
    case 'visual': return s.visuals;
    default: return s.cust;
  }
}

// thumb is a canvas showing an id's picture (0: none), drawn when on screen:
// marked missing when the disc has no such picture, left plain without a disc
function thumb(kind, id, size) {
  const [w, h2] = size || SHAPE[kind] || [48, 48];
  const c = h('canvas', { class: 'look-pic', width: 1, height: 1, 'aria-hidden': 'true' });
  c.style.width = w + 'px';
  c.style.height = h2 + 'px';
  const n = u32(id);
  if (!n) { c.classList.add('none'); return c; }
  lazy(c, async () => {
    if (!(await hasFile(kind))) { c.classList.add('nodisc'); return; }
    const img = await pictureOf(kind, n).catch(() => null);
    if (img) fit(c, img, w, h2, cropOf(kind, img));
    else { c.classList.add('missing'); c.title = 'not on your disc'; }
  });
  return c;
}

// a spell's icon as the spell book draws it (spellbook DrawIcon): its back
// whole, its front's top-left 22x22 at (13, 6) over it, the part the back
// covers (46x34) shown
function spellIcon(back, front, scale = 1.25) {
  const W = 46, H = 34;
  const c = h('canvas', { class: 'look-pic', width: 1, height: 1, 'aria-hidden': 'true' });
  c.style.width = Math.round(W * scale) + 'px';
  c.style.height = Math.round(H * scale) + 'px';
  (async () => {
    const d = await disc();
    if (!(await d.discReady()).ui) { c.classList.add('nodisc'); return; }
    const [b, f] = await Promise.all([u32(back) ? d.surfaceBitmap('UI', u32(back)) : null, u32(front) ? d.surfaceBitmap('UI', u32(front)) : null]);
    if (!b && !f) { c.classList.add(u32(back) || u32(front) ? 'missing' : 'none'); return; }
    const dpr = Math.min(2, window.devicePixelRatio || 1);
    const k = scale * dpr;
    c.width = Math.round(W * k);
    c.height = Math.round(H * k);
    const g = c.getContext('2d');
    g.imageSmoothingEnabled = true;
    if (b) g.drawImage(b, 0, 0, W, H, 0, 0, W * k, H * k);
    if (f) g.drawImage(f, 0, 0, 22, 22, 13 * k, 6 * k, 22 * k, 22 * k);
  })().catch(() => c.classList.add('missing'));
  return c;
}

async function noDiscHint(kind) {
  return (await hasFile(kind)) ? null : h('span', { class: 'hint look-nodisc' }, 'Load your game disc (',
    h('a', { href: '#/disc' }, 'Game disc'), ') to see the pictures and choose from them.');
}

// ---- the picker: every id of a kind, as a grid of pictures

const KIND_WORDS = {
  char: ['body', 'bodies', 'CHAR.ESF'], item: ['model', 'models', 'ITEM.ESF'], itemicon: ['icon', 'icons', 'ITEMICON.ESF'],
  uiicon: ['icon', 'icons', 'UI.CSF'], uiback: ['icon back', 'icon backs', 'UI.CSF'], visual: ['visual', 'visuals', 'SPELLFX.CSF'],
};

async function entriesOf(kind) {
  const d = await disc();
  const s = await d.discReady();
  const names = await lookNames(kind === 'uiback' ? 'uiback' : kind);
  const list = new Map();
  let onDisc = false;
  try {
    if (kind === 'visual') {
      if (s.visuals) {
        for (const v of await d.spellVisualsOnDisc()) list.set(v.id >>> 0, { id: v.id >>> 0, disc: true, lists: v.lists });
        onDisc = true;
      }
    } else {
      const cat = kind === 'uiicon' || kind === 'uiback' ? 'ui' : kind;
      const ok = cat === 'ui' ? s.ui : cat === 'itemicon' ? s.icons : cat === 'item' ? s.items : s.models;
      if (ok) {
        for (const e of await d.lookCatalog(cat)) list.set(e.id >>> 0, { ...e, disc: true });
        onDisc = true;
      }
    }
  } catch { /* no disc after all */ }
  for (const [id, u] of names) {
    const e = list.get(id) || { id, disc: false };
    e.rows = u.rows;
    e.names = u.names;
    list.set(id, e);
  }
  return { entries: [...list.values()], onDisc };
}

function visualText(lists) {
  if (!lists) return '';
  const words = [];
  for (const l of lists) for (const [k, n] of Object.entries(l.kinds || {})) if (!/end$|^wait$/.test(k) && !words.includes(k)) words.push(k + (n > 1 ? ' ×' + n : ''));
  return words.slice(0, 4).join(', ');
}

// pickLook asks for an id of a kind by sight; resolves it (unsigned) or null.
export function pickLook(kind, { title, current } = {}) {
  return new Promise((resolve) => {
    let done = false, dlg;
    const [one, many, file] = KIND_WORDS[kind] || ['id', 'ids', 'the disc'];
    const search = h('input', { type: 'search', placeholder: `Search by what wears it, or by its id (0x…)`, autocomplete: 'off', 'aria-label': 'Search' });
    const used = h('select', { 'aria-label': 'Which' }, h('option', { value: 'all' }, 'All'), h('option', { value: 'used' }, 'In use'), h('option', { value: 'unused' }, 'Not in use'));
    const sizeSel = kind === 'uiicon' || kind === 'uiback'
      ? h('select', { 'aria-label': 'Size' }, h('option', { value: kind === 'uiicon' ? '32' : '64' }, kind === 'uiicon' ? '32×32 (fronts)' : '64×64 (backs)'), h('option', { value: '' }, 'Every size'))
      : null;
    const grid = h('div', { class: 'look-grid' + (kind === 'itemicon' ? ' wide' : ''), role: 'listbox', 'aria-label': many });
    const foot = h('div', { class: 'faint small' }, 'Loading…');
    const pick = (id) => { done = true; resolve(id >>> 0); dlg.close(); };
    let all = [];
    const draw = () => {
      const q = search.value.trim().toLowerCase();
      const qid = parseId(q);
      let list = all.filter((e) => {
        if (used.value === 'used' && !e.rows) return false;
        if (used.value === 'unused' && e.rows) return false;
        if (sizeSel && sizeSel.value && e.disc && e.w !== undefined && String(e.w) !== sizeSel.value) return false;
        if (!q) return true;
        if (!Number.isNaN(qid) && u32(qid) === e.id) return true;
        return hex8(e.id).toLowerCase().includes(q) || (e.names || []).some((n) => n.toLowerCase().includes(q));
      });
      list.sort((a, b) => (u32(current) === a.id ? -1 : u32(current) === b.id ? 1 : 0) || (b.rows || 0) - (a.rows || 0) || a.id - b.id);
      const shown = list.slice(0, 600);
      mount(grid, ...shown.map((e) => {
        // an item icon twice the game's size, the rest as they were: the picture is what tells them apart, the id
        // only in the tooltip (the GMs' word, 2026-11-10: testers "can't tell icons apart", "the numbers" confused them)
        const pic = kind === 'visual' ? h('div', { class: 'look-pic visual' }, icon('play')) : thumb(kind, e.id, kind === 'itemicon' ? [208, 80] : kind === 'uiicon' ? [44, 44] : kind === 'uiback' ? [69, 51] : [96, 96]);
        const tip = [hex8(e.id), (e.names || []).length ? 'used by ' + e.names.join(', ') : 'used by nothing'].join(' · ');
        const b = h('button', { type: 'button', class: 'look-cell' + (u32(current) === e.id ? ' on' : '') + (e.disc ? '' : ' offdisc'), role: 'option', 'aria-selected': u32(current) === e.id ? 'true' : 'false', title: tip },
          pic,
          u32(current) === e.id ? h('span', { class: 'badge' }, 'current') : null,
          e.names?.length ? h('span', { class: 'look-names' }, e.names.slice(0, 2).join(', ')) : h('span', { class: 'look-names faint' }, 'not used yet'),
          kind === 'visual' && e.lists ? h('span', { class: 'look-names faint' }, visualText(e.lists)) : null,
          e.rows ? h('span', { class: 'faint small' }, e.rows + (e.rows === 1 ? ' row' : ' rows')) : null,
          !e.disc && all.some((x) => x.disc) ? h('span', { class: 'badge warn' }, 'not on the disc') : null);
        b.addEventListener('click', () => pick(e.id));
        return b;
      }));
      mount(foot, `${shown.length < list.length ? shown.length + ' shown of ' : ''}${list.length} ${list.length === 1 ? one : many}`,
        all.some((x) => x.disc) ? ` · pictures off your disc (${file})` : '');
      const on = grid.querySelector('.look-cell.on');
      if (on) on.scrollIntoView({ block: 'nearest' });
    };
    search.addEventListener('input', debounce(draw, 150));
    used.addEventListener('change', draw);
    sizeSel?.addEventListener('change', draw);
    const notice = h('div');
    dlg = dialog({
      title: title || 'Choose ' + one, wide: true,
      body: h('div', { class: 'stack look-picker' }, h('div', { class: 'row' }, h('div', { class: 'grow' }, search), used, sizeSel), notice, grid, foot),
      onClose: () => { if (!done) resolve(null); },
      actions: [{ label: 'Cancel', kind: 'ghost' }],
    });
    (async () => {
      const { entries, onDisc } = await entriesOf(kind);
      all = entries;
      if (!onDisc) {
        const hint = await noDiscHint(kind);
        mount(notice, h('div', { class: 'notice warn' },
          h('p', null, h('b', null, 'No pictures yet. '), 'The ' + many + ' are drawn from your own game disc, so load it once on the ',
            h('a', { href: '#/disc', onclick: () => dlg.close() }, 'Game disc'), ' page and come back. Until then the list shows only what each one is used by.')));
      }
      draw();
      search.focus();
    })().catch((e) => mount(foot, h('span', { class: 'err' }, e.message)));
  });
}

// viewModel opens a model on the turntable
async function viewModel(kind, id, title) {
  const d = await disc();
  dialog({ title: title + ' ' + hex8(id), wide: true, body: d.viewerBox(title, u32(id) | 0, kind === 'helm' ? 'helm' : kind, 'Drag to turn, wheel to zoom. As modelled: worn armour and held items are not drawn.') });
}

// ---- the widgets

// lookField builds a look column's control: {input, control}, input the
// element the form reads (its value is what is written), control what is
// shown (holding input). ctx: {locked, meta, form}.
export function lookField(col, value, ctx) {
  const look = col.look || '';
  if (look.startsWith('armor:')) return setField(col, value, ctx, look.slice(6));
  switch (look) {
    case 'char': case 'item': case 'itemicon': case 'uiicon': case 'uiback': case 'visual':
      return idField(col, value, ctx, look);
    case 'rgba': return colourField(col, value, ctx, false);
    case 'itemcolor': return colourField(col, value, ctx, true);
    case 'tint': return tintField(col, value, ctx);
    case 'robe': return robeField(col, value, ctx, true);
    case 'hair': return hairField(col, value, ctx);
    case 'itemlook': return itemLookField(col, value, ctx);
    case 'racelook': return raceLookField(col, value, ctx);
    default: return null;
  }
}

const fire = (el) => { el.dispatchEvent(new Event('input', { bubbles: true })); el.dispatchEvent(new Event('change', { bubbles: true })); };

// the form's input of another column of the row, once the form is built
function sibling(ctx, name, fn) {
  const go = () => {
    const el = ctx.form?.inputs?.get(name);
    if (el) fn(el);
  };
  if (ctx.form?.after) ctx.form.after.push(go);
  else queueMicrotask(go);
}

// an id: picture, hex box, stored decimal, what wears it, Choose…
function idField(col, value, ctx, kind, opts = {}) {
  const input = opts.input || h('input', { type: 'text', class: 'mono', spellcheck: 'false', autocomplete: 'off', inputmode: 'text' });
  if (!opts.input) input.value = value === null || value === undefined || value === '' ? '' : hex8(value);
  input.disabled = ctx.locked;
  const picBox = h('div', { class: 'look-pic-box', role: kind === 'visual' ? null : 'button', tabindex: kind === 'visual' ? null : '0' });
  const dec = h('span', { class: 'hint mono' });
  const names = h('span', { class: 'look-names' });
  let spellRow = null; // a spell's icon is its front over its back (spellpattern)
  const isSpell = (kind === 'uiicon' || kind === 'uiback') && ctx.meta?.name === 'spellpattern';
  const show = () => {
    const n = parseId(input.value);
    input.classList.toggle('bad', input.value.trim() !== '' && Number.isNaN(n));
    if (input.value.trim() === '') {
      dec.textContent = col.notnull ? 'empty: 0' : 'empty';
      mount(picBox, h('canvas', { class: 'look-pic none' }));
      names.textContent = '';
      return;
    }
    if (Number.isNaN(n)) {
      dec.textContent = 'not an id: type 0x1A2B3C4D or a whole number';
      return;
    }
    const s = stored(col, n);
    dec.textContent = col.store ? '= ' + s + ' stored' : '= ' + n;
    dec.title = col.store === 'int32' ? 'this column keeps ids signed (a zigzag field needs it); the game reads the low 32 bits' : col.store === 'uint32' ? 'this column keeps ids unsigned' : '';
    if (kind === 'visual') {
      mount(picBox, h('div', { class: 'look-pic visual' }, icon('play')));
    } else if (isSpell && spellRow) {
      const other = parseId(spellRow.value);
      mount(picBox, kind === 'uiback' ? spellIcon(n, other) : spellIcon(other, n));
    } else {
      mount(picBox, thumb(kind, n, kind === 'itemicon' ? [152, 60] : undefined));
    }
    names.textContent = '';
    if (u32(n)) {
      lookNames(kind).then((m) => {
        if (parseId(input.value) !== n) return;
        const u = m.get(u32(n));
        names.textContent = u ? 'used by ' + namesText(u) : '';
        if (kind === 'visual' && !u) names.textContent = '';
      });
      if (kind === 'visual') disc().then((d) => d.discReady().then((st) => st.visuals && d.spellVisualsOnDisc())).then((vs) => {
        const v = (vs || []).find((x) => (x.id >>> 0) === u32(n));
        if (v && parseId(input.value) === n) mount(picBox, h('div', { class: 'look-pic visual', title: visualText(v.lists) }, icon('play')));
      }).catch(() => {});
    } else names.textContent = 'none';
  };
  input.addEventListener('input', show);
  input.addEventListener('blur', () => {
    const n = parseId(input.value);
    if (!Number.isNaN(n) && !/^0x/i.test(input.value.trim())) { input.value = hex8(n); fire(input); }
  });
  if (isSpell) sibling(ctx, kind === 'uiicon' ? 'iconColor' : 'icon', (el) => { spellRow = el; el.addEventListener('input', show); show(); });
  const choose = async () => {
    const id = await pickLook(kind, { title: 'Choose ' + (col.label || col.name).toLowerCase(), current: parseId(input.value) });
    if (id === null) return;
    input.value = hex8(id);
    fire(input);
  };
  const view = () => {
    const n = parseId(input.value);
    if (!u32(n)) return choose();
    if (kind === 'char' || kind === 'item') return viewModel(kind, n, col.label || col.name);
    return ctx.locked ? null : choose();
  };
  picBox.addEventListener('click', view);
  picBox.addEventListener('keydown', (e) => { if (e.key === 'Enter') view(); });
  picBox.title = kind === 'char' || kind === 'item' ? 'Turn it round' : 'Choose';
  show();
  const hint = h('div');
  if (ctx.form && !ctx.form.discHinted) {
    ctx.form.discHinted = true;
    noDiscHint(kind).then((x) => { if (x) mount(hint, x); });
  } else if (!ctx.form) noDiscHint(kind).then((x) => { if (x) mount(hint, x); });
  // a visual is seen played on the Spell visuals page (its preview, or in the
  // game), a spell's own there for that spell; a new tab keeps this form
  const spell = ctx.form?.row?.spellid;
  const preview = kind === 'visual' ? h('a', { class: 'small', href: '#/visuals' + (spell !== undefined && ctx.meta?.name === 'spelleffect' ? '/' + spell : ''), target: '_blank', rel: 'noopener' }, 'See it played') : null;
  // the picture and Choose lead; the id (a hex box, and the number the column stores) folds away under "Id",
  // opened when it is not an id (the GMs' word, 2026-11-10: the numbers on the field confused the testers)
  const one = (KIND_WORDS[kind] || ['id'])[0];
  const idBox = h('details', { class: 'look-idbox' }, h('summary', null, 'Id'), h('div', { class: 'with' }, input, dec));
  input.addEventListener('input', () => { if (input.classList.contains('bad')) idBox.open = true; });
  const control = h('div', { class: 'look look-id' },
    h('div', { class: 'with' }, picBox,
      ctx.locked ? null : button('Choose ' + (/^[aeiou]/.test(one) ? 'an ' : 'a ') + one + '…', { class: 'small', onclick: choose }),
      names),
    idBox, preview, hint);
  return { input, control, wide: 'pair' };
}

// a colour: swatch, RGBA sliders, hex, the dyers' colours and the disc
// tints; an item's colour also a disc tint or plain. Each pane is built once
// and only shown or hidden (a slider rebuilt under the pointer stops its drag).
function colourField(col, value, ctx, item) {
  const pal = ctx.meta?.palettes || {};
  const tints = pal.tints || [];
  const input = h('input', { type: 'text', class: 'mono look-raw', spellcheck: 'false', autocomplete: 'off', 'aria-label': (col.label || col.name) + ' (value)' });
  input.disabled = ctx.locked;
  const v0 = value === null || value === undefined || value === '' ? null : u32(value);
  input.value = v0 === null ? '' : item && v0 < 15 ? String(v0) : hex8(v0);
  // alpha 0 is refused for a dye (it would vanish); an item's colour below 15
  // would read as a tint
  const minAlpha = item ? 16 : 1;
  const tintRGBA = (i) => parseInt(tints[i]?.rgba || 'FFFFFFFF', 16) >>> 0;
  // white is plain unless Colour was chosen (white then a colour like any)
  let forced = '';
  const mode = () => {
    const n = parseId(input.value);
    if (Number.isNaN(n)) return 'colour';
    const u = u32(n);
    if (item && u < 15) return 'tint';
    if (item && u === 0xFFFFFFFF && forced !== 'colour') return 'plain';
    return 'colour';
  };
  const swatch = h('span', { class: 'look-swatch big' });
  const caption = h('div', { class: 'hint' });
  const set = (u) => { input.value = item && u < 15 ? String(u) : hex8(u); fire(input); };
  const current = () => { const n = parseId(input.value); return Number.isNaN(n) || input.value.trim() === '' ? 0xFFFFFFFF : u32(n); };
  const swatchBtn = (u, label, onPick) => {
    const b = h('button', { type: 'button', class: 'look-swatch', title: label, 'aria-label': label, disabled: ctx.locked });
    b.style.setProperty('--sw', cssColour(u));
    b.addEventListener('click', onPick);
    return b;
  };
  // the colour pane: sliders, hex, the native picker, the palettes
  const slider = (label, i) => {
    const min = i === 3 ? minAlpha : 0;
    const r = h('input', { type: 'range', min: String(min), max: '255', step: '1', 'aria-label': label, disabled: ctx.locked });
    const n = h('input', { type: 'number', min: String(min), max: '255', class: 'look-num', 'aria-label': label + ' (number)', disabled: ctx.locked });
    const change = (src) => {
      const c = rgbaOf(current());
      c[i] = Math.max(min, Math.min(255, Math.round(Number(src.value)) || 0));
      if (c[3] < minAlpha) c[3] = minAlpha;
      set(packRGBA(c));
    };
    r.addEventListener('input', () => change(r));
    n.addEventListener('change', () => change(n));
    return { r, n, row: h('div', { class: 'look-slider' }, h('span', { class: 'look-ch' }, label), r, n) };
  };
  const sliders = [slider('R', 0), slider('G', 1), slider('B', 2), slider('A', 3)];
  const hexBox = h('input', { type: 'text', class: 'mono look-hex', placeholder: 'RRGGBBAA', 'aria-label': 'Colour in hex (RRGGBB or RRGGBBAA)', spellcheck: 'false', disabled: ctx.locked });
  hexBox.addEventListener('change', () => {
    const u = parseColour(hexBox.value);
    if (Number.isNaN(u)) { hexBox.classList.add('bad'); return; }
    hexBox.classList.remove('bad');
    const c = rgbaOf(u);
    if (c[3] < minAlpha) c[3] = minAlpha;
    set(packRGBA(c));
  });
  const native = h('input', { type: 'color', 'aria-label': 'Pick a colour', disabled: ctx.locked });
  native.addEventListener('input', () => {
    const [r, g, b] = [1, 3, 5].map((k) => parseInt(native.value.slice(k, k + 2), 16));
    set(packRGBA([r, g, b, Math.max(minAlpha, rgbaOf(current())[3])]));
  });
  const dyeRows = (pal.dyes || []).map((f) => h('div', { class: 'look-pal-row' }, h('span', { class: 'look-pal-name' }, f.label),
    ...f.colours.map((c) => swatchBtn(parseInt(c.rgba, 16) >>> 0, `${c.label} (${c.rgba})`, () => set(parseInt(c.rgba, 16) >>> 0)))));
  const tintAsColour = tints.length ? h('div', { class: 'look-pal-row' }, h('span', { class: 'look-pal-name' }, 'The disc\'s tints'),
    ...tints.map((t, i) => swatchBtn(tintRGBA(i), `${t.label} (${t.rgba})`, () => set(tintRGBA(i))))) : null;
  const colourPane = h('div', { class: 'look-colour' },
    h('div', { class: 'look-sliders' }, ...sliders.map((s) => s.row),
      h('div', { class: 'look-slider' }, h('span', { class: 'look-ch' }, '#'), hexBox, native)),
    h('div', { class: 'look-pals' }, ...dyeRows, tintAsColour));
  // the tint pane (an item's): the fifteen
  const tintPane = h('div', { class: 'look-pal-row' }, ...tints.map((t, i) => {
    const b = swatchBtn(tintRGBA(i), `Tint ${t.value}: ${t.label}`, () => set(t.value));
    b.dataset.tint = String(t.value);
    b.appendChild(h('span', { class: 'look-swatch-n' }, String(t.value)));
    return b;
  }));
  const plainPane = h('p', { class: 'muted small' }, 'Drawn as its art has it. Colour gives it one; Disc tint a robe\'s tint.');
  const body = h('div', { class: 'look-colour-body' });
  const tabs = item ? h('div', { class: 'look-tabs', role: 'tablist' }) : null;
  let shown = '';
  const draw = () => {
    const m = mode();
    const n = parseId(input.value);
    if (tabs) for (const b of tabs.children) b.classList.toggle('on', b.dataset.mode === m);
    if (m !== shown) { mount(body, m === 'tint' ? tintPane : m === 'plain' ? plainPane : colourPane); shown = m; }
    if (m === 'tint') {
      const i = u32(n);
      swatch.style.setProperty('--sw', cssColour(tintRGBA(i)));
      for (const b of tintPane.children) b.classList.toggle('on', b.dataset.tint === String(i));
      caption.textContent = `Tint ${i}${tints[i] ? ': ' + tints[i].label : ''}. A robe is drawn in it; armour shows nothing of it.`;
    } else if (m === 'plain') {
      swatch.style.setProperty('--sw', '#ffffff');
      caption.textContent = '0xFFFFFFFF: plain, the art\'s own colours.';
    } else {
      const ok = !Number.isNaN(n) && input.value.trim() !== '';
      const u = current();
      const c = rgbaOf(u);
      swatch.style.setProperty('--sw', cssColour(u));
      sliders.forEach((s, i) => { if (document.activeElement !== s.n) s.n.value = String(c[i]); s.r.value = String(c[i]); });
      if (document.activeElement !== hexBox) { hexBox.value = hexRGBA(u); hexBox.classList.remove('bad'); }
      native.value = '#' + hexRGBA(u).slice(0, 6).toLowerCase();
      // an item's colour: a PS2 draws the nearest tint (entity's nearestTint);
      // a dye (chardyes) a PS2 never sees
      const near = item && tints.length ? nearestTint(u, tints) : null;
      caption.textContent = (ok ? hex8(u) + (col.store ? ' = ' + stored(col, u) + ' stored' : '') : 'empty: choose a colour') +
        (near !== null && ok ? ` · on a PS2: drawn as tint ${near} (${tints[near].label})` : '') +
        (!item && ok ? ' · drawn by a client of the live game\'s look; a PS2 never sees a dye' : '');
    }
  };
  if (tabs) {
    for (const [m, label] of [['tint', 'Disc tint'], ['plain', 'Plain'], ['colour', 'Colour']]) {
      const b = h('button', { type: 'button', class: 'small look-tab', role: 'tab', disabled: ctx.locked, dataset: { mode: m } }, label);
      b.addEventListener('click', () => {
        const was = mode();
        if (was === m) return;
        forced = m;
        if (m === 'tint') set(0);
        else if (m === 'plain') set(0xFFFFFFFF);
        else set(was === 'tint' ? tintRGBA(u32(parseId(input.value))) : 0xFFFFFFFF);
      });
      tabs.appendChild(b);
    }
  }
  input.addEventListener('input', draw);
  draw();
  const control = h('div', { class: 'look look-colour-field' },
    h('div', { class: 'row' }, swatch, tabs, h('span', { class: 'grow' }), h('span', { class: 'faint small' }, 'value'), input),
    caption, body);
  return { input, control, wide: true };
}

// a disc tint index (npcs.unk_o): the fifteen as swatches
function tintField(col, value, ctx) {
  const tints = ctx.meta?.palettes?.tints || [];
  const input = h('input', { type: 'text', inputmode: 'numeric', class: 'look-num', autocomplete: 'off' });
  input.value = value === null || value === undefined ? '' : String(value);
  input.disabled = ctx.locked;
  const caption = h('span', { class: 'hint' });
  const row = h('div', { class: 'look-pal-row' }, ...tints.map((t) => {
    const b = h('button', { type: 'button', class: 'look-swatch', title: `${t.value}: ${t.label}`, 'aria-label': `Tint ${t.value}: ${t.label}`, disabled: ctx.locked, dataset: { tint: String(t.value) } });
    b.style.setProperty('--sw', cssColour(parseInt(t.rgba, 16)));
    b.appendChild(h('span', { class: 'look-swatch-n' }, String(t.value)));
    b.addEventListener('click', () => { input.value = String(t.value); fire(input); });
    return b;
  }));
  const draw = () => {
    const n = Number(input.value);
    for (const b of row.children) b.classList.toggle('on', b.dataset.tint === String(n) && input.value.trim() !== '');
    caption.textContent = input.value.trim() === '' ? 'empty' : !Number.isInteger(n) ? 'a tint is 0-14' : tints[n] ? `${n}: ${tints[n].label}${n === 0 ? ' (none)' : ''}` : `${n}: outside the disc's 15, drawn as 0`;
  };
  input.addEventListener('input', draw);
  draw();
  return { input, control: h('div', { class: 'look' }, h('div', { class: 'row' }, row, input), caption), wide: true };
}

// an index into a set of pictures, as a select with the chosen one's picture
// and Choose… for all of them side by side
function indexField(col, value, ctx, { options, picture, title, text = false, noneValue = '0' }) {
  const input = h('input', { type: 'text', class: 'look-num', autocomplete: 'off', inputmode: text ? undefined : 'numeric' });
  input.value = value === null || value === undefined ? '' : String(value);
  input.disabled = ctx.locked;
  const sel = h('select', { 'aria-label': (col.label || col.name) + ' (by name)', disabled: ctx.locked });
  const picBox = h('div', { class: 'look-pic-box', role: 'button', tabindex: '0', title: 'Choose' });
  const caption = h('span', { class: 'hint' });
  const draw = () => {
    const v = input.value.trim();
    const known = options.find((o) => String(o.value) === v || (o.alias || []).includes(v));
    mount(sel, ...options.map((o) => h('option', { value: String(o.value) }, o.label)),
      known ? null : h('option', { value: '__' }, v === '' ? '(empty)' : v + ' (not one of these)'));
    sel.value = known ? String(known.value) : '__';
    mount(picBox, known ? picture(known.value) : h('canvas', { class: 'look-pic none' }));
    caption.textContent = known?.note || (known ? '' : v === '' ? 'empty' : 'not one the disc has');
  };
  sel.addEventListener('change', () => { if (sel.value !== '__') { input.value = sel.value; fire(input); } });
  input.addEventListener('input', draw);
  const choose = () => {
    if (ctx.locked) return;
    let dlg;
    const cells = options.map((o) => {
      const b = h('button', { type: 'button', class: 'look-cell' + (String(o.value) === input.value.trim() ? ' on' : '') }, picture(o.value, true), h('span', { class: 'small' }, o.label), o.note ? h('span', { class: 'faint small' }, o.note) : null);
      b.addEventListener('click', () => { input.value = String(o.value); fire(input); dlg.close(); });
      return b;
    });
    dlg = dialog({ title, wide: true, body: h('div', { class: 'stack' }, h('div', { class: 'look-grid' }, ...cells)), actions: [{ label: 'Cancel', kind: 'ghost' }] });
  };
  picBox.addEventListener('click', choose);
  picBox.addEventListener('keydown', (e) => { if (e.key === 'Enter') choose(); });
  draw();
  void noneValue;
  picBox.title = ctx.locked ? '' : 'Choose: every one side by side';
  return { input, control: h('div', { class: 'look look-index' }, h('div', { class: 'with' }, picBox, sel, input), caption), wide: false };
}

// an armour set on one slot: the set's texture for it (the helm: its sprite)
function setField(col, value, ctx, slot, opts = {}) {
  const options = [{ value: 0, label: 'None' }];
  for (let s = 1; s <= 8; s++) options.push({ value: s, label: 'Set ' + s });
  options.push({ value: 9, label: 'Set 9 (ornate plate)', note: 'The ornate gold plate CHAR.ESF holds (as Sandstorm\'s S2 draws it): vanilla draws it on this server; a PS2 is sent set 4, plain plate' });
  const picture = (set, big) => {
    const sz = big ? [96, 96] : [48, 48];
    if (slot === 'helm') return HELM[set] ? thumb('helm', HELM[set], sz) : h('canvas', { class: 'look-pic none' });
    const tex = ARMOR[set]?.[ARMOR_SLOT[slot]];
    return tex ? thumb('cust', tex, sz) : h('canvas', { class: 'look-pic none' });
  };
  return indexField(col, value, ctx, { options, picture, title: `Choose the armour on the ${SLOT_WORD[slot] || slot}`, ...opts });
}

// a robe: its chest texture
function robeField(col, value, ctx, text) {
  const options = text ? [{ value: '255', label: 'None', alias: [''] }] : [];
  for (let r = 0; r < 4; r++) options.push({ value: text ? String(r) : r, label: 'Robe ' + r });
  options.push({ value: text ? '4' : 4, label: 'Robe 4 (Sandstorm\'s)', note: 'Sandstorm\'s S2: no art on the retail disc' });
  options.push({ value: text ? '5' : 5, label: 'Robe 5 (Sandstorm\'s)', note: 'Sandstorm\'s S2: no art on the retail disc' });
  const picture = (r, big) => {
    const tex = ROBE[Number(r)]?.[0];
    return tex ? thumb('cust', tex, big ? [96, 96] : [48, 48]) : h('canvas', { class: 'look-pic none' });
  };
  return indexField(col, value, ctx, { options, picture, title: 'Choose the robe', text });
}

// a hair colour: which hair texture
function hairField(col, value, ctx) {
  const options = HAIR.map((_, i) => ({ value: i, label: 'Hair ' + i }));
  const picture = (i, big) => thumb('cust', HAIR[i], big ? [96, 96] : [48, 48]);
  return indexField(col, value, ctx, { options, picture, title: 'Choose the hair' });
}

// itempattern.model: by the row's wear slot, an armour set, a robe, a cape's
// style or an ITEM.ESF model. input is what the form reads; a set's or a
// robe's own select writes into it.
function itemLookField(col, value, ctx) {
  const input = h('input', { type: 'text', class: 'mono', spellcheck: 'false', autocomplete: 'off', 'aria-label': (col.label || col.name) + ' (value)' });
  input.disabled = ctx.locked;
  input.value = value === null || value === undefined || value === '' ? '' : String(value);
  const box = h('div');
  let wear = null, mode = '', part = null;
  // a part with its own input (a set's, a robe's) follows this one
  input.addEventListener('input', () => {
    if (part && part.input !== input && part.input.value !== input.value) { part.input.value = input.value; part.input.dispatchEvent(new Event('input')); }
  });
  const render = () => {
    const w = wear ? Number(wear.value) : NaN;
    const m = WEAR_ARMOR[w] ? 'armor:' + WEAR_ARMOR[w] : w === WEAR_ROBE ? 'robe' : w === WEAR_CAPE ? 'cape' : WEAR_HELD.has(w) ? 'item' : 'other';
    if (m === mode) return;
    mode = m;
    const cur = parseId(input.value);
    const val = input.value.trim() === '' ? '' : Number.isNaN(cur) ? input.value : cur;
    if (m === 'item') {
      input.value = val === '' ? '' : hex8(val);
      part = idField(col, val, ctx, 'item', { input });
    } else {
      input.value = val === '' ? '' : String(val);
      part = m.startsWith('armor:') ? setField(col, val, ctx, m.slice(6)) : m === 'robe' ? robeField(col, val, ctx, false) : null;
      if (part) {
        part.input.addEventListener('input', () => { if (input.value !== part.input.value) { input.value = part.input.value; fire(input); } });
      }
    }
    const note = m === 'cape' ? 'A cape\'s style (1-255; 0 draws none), drawn by a client that draws capes.'
      : m === 'other' ? (wear ? 'Not drawn for this wear slot: kept as it is.' : '') : '';
    mount(box, part ? part.control : h('div', { class: 'with' }, input), note ? h('span', { class: 'hint' }, note) : null,
      part && part.input !== input ? h('div', { class: 'hidden' }, input) : null);
  };
  sibling(ctx, 'equipslot', (el) => { wear = el; el.addEventListener('change', render); el.addEventListener('input', render); render(); });
  render();
  return { input, control: h('div', { class: 'look' }, box), wide: true };
}

// charlook.value: a colour of a custom race's part (mode, hue, saturation,
// lightness: entity.Recolour) or a texture, by the row's kind
function raceLookField(col, value, ctx) {
  const input = h('input', { type: 'text', class: 'mono', spellcheck: 'false', autocomplete: 'off' });
  input.value = value === null || value === undefined || value === '' ? '' : hex8(value);
  input.disabled = ctx.locked;
  const box = h('div');
  let kindEl = null, mode = '';
  const render = () => {
    const k = kindEl ? Number(kindEl.value) : 2;
    const m = k === 3 ? 'texture' : 'colour';
    if (m === mode) return;
    mode = m;
    if (m === 'texture') {
      const part = idField(col, parseId(input.value), ctx, 'texture', { input });
      mount(box, part.control);
      return;
    }
    const modeSel = h('select', { 'aria-label': 'Mode', disabled: ctx.locked }, h('option', { value: '0' }, 'Its own colours'), h('option', { value: '1' }, 'Dye (one colour over its shading)'), h('option', { value: '2' }, 'Tone (its colours shifted)'));
    const mk = (label, max) => {
      const r = h('input', { type: 'range', min: '0', max: String(max), step: '1', 'aria-label': label, disabled: ctx.locked });
      const n = h('span', { class: 'mono small look-val' });
      return { r, n, row: h('label', { class: 'look-slider' }, h('span', { class: 'look-ch wide' }, label), r, n) };
    };
    const hue = mk('Hue', 255), sat = mk('Saturation', 255), light = mk('Lightness', 255);
    const swatch = h('span', { class: 'look-swatch big' });
    const read = () => { const u = u32(parseId(input.value)) || 0; return [u >>> 24, (u >>> 16) & 255, (u >>> 8) & 255, u & 255]; };
    const write = () => {
      const md = Number(modeSel.value);
      const u = md === 0 ? 0 : ((md << 24) | (Number(hue.r.value) << 16) | (Number(sat.r.value) << 8) | Number(light.r.value)) >>> 0;
      input.value = hex8(u);
      fire(input);
    };
    const draw = () => {
      const [md, hh, ss, ll] = read();
      modeSel.value = String(md > 2 ? 0 : md);
      hue.r.value = hh; sat.r.value = ss; light.r.value = ll;
      const deg = Math.round((hh * 360) / 256);
      hue.n.textContent = md === 2 ? `turned ${deg}°` : `${deg}°`;
      sat.n.textContent = md === 1 ? `${Math.round((ss / 255) * 100)}%` : `×${(ss / 128).toFixed(2)}`;
      light.n.textContent = `×${(ll / 128).toFixed(2)}`;
      for (const s of [hue, sat, light]) s.r.disabled = ctx.locked || md === 0;
      swatch.style.setProperty('--sw', md === 1 ? `hsl(${deg}, ${Math.round((ss / 255) * 100)}%, ${Math.min(95, Math.round((ll / 128) * 50))}%)` : 'transparent');
      swatch.title = md === 1 ? 'about how a dye looks on a mid-grey part' : '';
    };
    for (const s of [hue, sat, light]) s.r.addEventListener('input', write);
    modeSel.addEventListener('change', () => {
      if (modeSel.value === '1' && read()[0] !== 1) { hue.r.value = 0; sat.r.value = 140; light.r.value = 128; }
      if (modeSel.value === '2' && read()[0] !== 2) { hue.r.value = 0; sat.r.value = 128; light.r.value = 128; }
      write();
    });
    input.addEventListener('input', draw);
    draw();
    mount(box, h('div', { class: 'stack' }, h('div', { class: 'row' }, swatch, modeSel, h('span', { class: 'grow' }), input),
      h('div', { class: 'look-sliders' }, hue.row, sat.row, light.row)));
  };
  sibling(ctx, 'kind', (el) => { kindEl = el; el.addEventListener('input', render); el.addEventListener('change', render); mode = ''; render(); });
  render();
  return { input, control: h('div', { class: 'look' }, box), wide: true };
}

// ---- dressing a body as a PS2 does (VICSprite::ApplyLook: SetRobe, then
// SetArmorSlot for the five worn slots; vanilla's cust::apply_look): the swaps
// the disc worker's model op takes

// npcSwaps is an NPC row's look (its columns' values, as the form holds
// them) as text slot swaps: a robe on the chest and legs in tint 0, each
// armour set's texture (retail's sets 0-8; 9 and past are none) in the row's
// tint (unk_o, 0-14), a bare slot its own texture in that tint.
export function npcSwaps(v, tints) {
  const col = (i) => rgbaOf(parseInt((tints[i] || tints[0] || { rgba: 'FFFFFFFF' }).rgba, 16) >>> 0);
  const out = [], robed = new Set();
  const robe = String(v.robe_style ?? '').trim() === '' ? -1 : Number(v.robe_style);
  if (Number.isInteger(robe) && robe >= 0 && robe < 255) {
    const r = robe < ROBE.length ? robe : 0;
    for (const [slot, half] of [[0, 0], [3, 1]]) { out.push({ slot, tex: ROBE[r][half] | 0, col: col(0) }); robed.add(slot); }
  }
  let tint = Number(v.unk_o) || 0;
  if (tint < 0 || tint > 14) tint = 0;
  const sets = { chest: v.torso, boots: v.feet, bracer: v.forearm, legs: v.legs, gloves: v.gloves };
  for (const [name, slot] of Object.entries(ARMOR_SLOT)) {
    if (robed.has(slot)) continue;
    let set = Number(sets[name]) || 0;
    if (set < 0 || set >= ARMOR.length) set = 0;
    out.push({ slot, tex: set ? ARMOR[set][slot] | 0 : 0, col: col(tint) });
  }
  return out;
}

// itemDress is how an item's own row shows on a body: an armour piece its set
// on its slot (a helm its sprite), a robe on the chest and legs, each in the
// tint a PS2 draws the item's colour in (handlers wornLook: an armour piece
// the tint nearest its own colour, none for a tint index or white; a robe its
// tint index, else the nearest); null for any other wear slot.
export function itemDress(v, tints) {
  const wear = Number(v.equipslot), model = Number(v.model) || 0;
  const c = u32(parseId(String(v.color ?? '0')));
  const col = (i) => rgbaOf(parseInt((tints[i] || tints[0] || { rgba: 'FFFFFFFF' }).rgba, 16) >>> 0);
  const nearest = (u) => (tints.length ? nearestTint(u, tints) : 0);
  if (WEAR_ARMOR[wear]) {
    const slot = WEAR_ARMOR[wear];
    const tint = Number.isNaN(c) || c < 15 || c === 0xFFFFFFFF ? 0 : nearest(c);
    if (slot === 'helm') return { helm: HELM[model >= 0 && model < HELM.length ? model : 0] || 0 };
    const set = model >= 0 && model < ARMOR.length ? model : 0;
    return { swaps: [{ slot: ARMOR_SLOT[slot], tex: set ? ARMOR[set][ARMOR_SLOT[slot]] | 0 : 0, col: col(tint) }] };
  }
  if (wear === WEAR_ROBE) {
    const r = model >= 0 && model < ROBE.length ? model : 0;
    const tint = Number.isNaN(c) ? 0 : c < 15 ? c : c === 0xFFFFFFFF ? 0 : nearest(c);
    return { swaps: [0, 1].map((half) => ({ slot: half ? 3 : 0, tex: ROBE[r][half] | 0, col: col(tint) })) };
  }
  return null;
}

// the body an item is shown on: the human male's (charactermodel)
export const SHOW_BODY = 0x70D898C6 | 0;

// ---- a list's cells

// lookCell is a grid cell for a look column's value: its picture or colour
// beside the number.
export function lookCell(meta, col, v) {
  const look = col.look || '';
  const n = u32(v);
  if (look === 'rgba' || look === 'itemcolor') {
    const tints = meta.palettes?.tints || [];
    if (look === 'itemcolor' && n < 15) {
      const t = tints[n];
      const sw = h('span', { class: 'look-swatch tiny' });
      sw.style.setProperty('--sw', t ? cssColour(parseInt(t.rgba, 16)) : 'transparent');
      return h('span', { class: 'look-cellval' }, sw, t ? `tint ${n} ${t.label}` : `tint ${n}`);
    }
    const sw = h('span', { class: 'look-swatch tiny' });
    sw.style.setProperty('--sw', cssColour(n));
    return h('span', { class: 'look-cellval mono' }, sw, look === 'itemcolor' && n === 0xFFFFFFFF ? 'plain' : hexRGBA(n));
  }
  if (look === 'tint') {
    const t = (meta.palettes?.tints || [])[Number(v)];
    const sw = h('span', { class: 'look-swatch tiny' });
    sw.style.setProperty('--sw', t ? cssColour(parseInt(t.rgba, 16)) : 'transparent');
    return h('span', { class: 'look-cellval' }, sw, t ? `${v} ${t.label}` : String(v));
  }
  if (look === 'itemicon' || look === 'uiicon' || look === 'uiback' || look === 'char' || look === 'item') {
    const size = look === 'itemicon' ? [52, 20] : look === 'char' || look === 'item' ? [28, 28] : look === 'uiback' ? [30, 22] : [22, 22];
    return h('span', { class: 'look-cellval mono' }, n ? thumb(look, n, size) : null, hex8(v));
  }
  return null;
}
