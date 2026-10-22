// The world map: every NPC of a world as the running world has it, the players
// online, and (with the game disc loaded, views/disc.js) the terrain under
// them. Pan with a drag, zoom with the wheel or a pinch, click a marker for
// what it is.

import { h, mount, pageHead, button, toast, store, debounce } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { describe } from '../lib/meta.js';
import { go } from '../lib/nav.js';
import { discTerrain } from './disc.js';


const CELL = 2000;

export async function render(box, [worldSeg, focusSeg]) {
  const meta = await describe('npcs');
  const worlds = (meta.enums.world || []).slice(0, 10);
  let world = worldSeg !== undefined ? Number(worldSeg) : Number(store('map.world') || 0);
  const focus = focusSeg !== undefined ? Number(focusSeg) : null;

  const worldSel = h('select', { 'aria-label': 'World' }, ...worlds.map((w) => h('option', { value: w.value, selected: w.value === world }, `${w.label} (${w.value})`)));
  const find = h('input', { type: 'search', placeholder: 'find an NPC or player', 'aria-label': 'Find' });
  const showNPC = h('input', { type: 'checkbox', checked: true });
  const showPlayers = h('input', { type: 'checkbox', checked: true });
  const showTerrain = h('input', { type: 'checkbox', checked: true });
  const canvas = h('canvas', { 'aria-label': 'World map' });
  const tip = h('div', { class: 'map-tip hidden' });
  const info = h('div', { class: 'map-info hidden' });
  const status = h('span', { class: 'badge' }, '…');
  const legend = h('div', { class: 'map-legend' });
  const wrap = h('div', { class: 'map-wrap' }, canvas, legend,
    h('div', { class: 'map-hud' }, worldSel, find,
      h('label', { class: 'check badge' }, showNPC, 'NPCs'), h('label', { class: 'check badge' }, showPlayers, 'players'),
      h('label', { class: 'check badge' }, showTerrain, 'terrain'), status),
    tip, info);
  mount(box, pageHead('World map', 'NPCs as they stand and players online, over the terrain off your game disc: load it once on the Game disc page and this computer keeps the map.'), wrap);

  queueMicrotask(() => mount(legend, ...KINDS.map(([label, colour]) => {
    const dot = h('i');
    dot.style.background = colour;
    return h('span', null, dot, label);
  }), h('span', null, h('strong', null, '▲'), ' player')));
  const ctx = canvas.getContext('2d');
  let npcs = [], names = [], players = [];
  let view = { cx: 0, cz: 0, scale: 0.05 }; // world units -> pixels
  let selected = null, moving = null, hover = null;
  let terrain = null; // {tiles:[{x0,z0,x1,z1,img}], cancel}
  let dpr = Math.min(2, window.devicePixelRatio || 1);

  const resize = () => {
    dpr = Math.min(2, window.devicePixelRatio || 1);
    const r = wrap.getBoundingClientRect();
    canvas.width = Math.max(1, Math.round(r.width * dpr));
    canvas.height = Math.max(1, Math.round(r.height * dpr));
    draw();
  };
  const ro = new ResizeObserver(resize);
  ro.observe(wrap);

  const toScreen = (x, z) => [(x - view.cx) * view.scale * dpr + canvas.width / 2, (z - view.cz) * view.scale * dpr + canvas.height / 2];
  const toWorld = (sx, sy) => [(sx * dpr - canvas.width / 2) / (view.scale * dpr) + view.cx, (sy * dpr - canvas.height / 2) / (view.scale * dpr) + view.cz];

  // what a marker's colour says (the legend draws the same table)
  const KINDS = [
    ['dead', '#5a6070'], ['GM spawn', '#c084fc'], ['merchant', '#f0c36a'], ['banker / smith', '#7cc4ff'],
    ['friendly', '#8fd19e'], ['attackable', '#ff7a6b'],
  ];
  function kindOf(n) {
    const [, , , , type, flags] = n;
    if (flags & 1) return 0;
    if (flags & 2) return 1;
    if (type & 1) return 2;
    if (type & 0x0a) return 3;
    if (type & 0x80) return 4;
    return 5;
  }
  const colourOf = (n) => KINDS[kindOf(n)][1];

  function draw() {
    const W = canvas.width, H = canvas.height;
    ctx.setTransform(1, 0, 0, 1, 0, 0);
    ctx.fillStyle = '#0a0c10';
    ctx.fillRect(0, 0, W, H);
    // terrain
    if (terrain && showTerrain.checked) {
      ctx.imageSmoothingEnabled = view.scale < 0.2;
      for (const t of terrain.tiles) {
        const [x0, y0] = toScreen(t.x0, t.z0);
        const [x1, y1] = toScreen(t.x1, t.z1);
        if (x1 < 0 || y1 < 0 || x0 > W || y0 > H) continue;
        ctx.drawImage(t.img, x0, y0, x1 - x0, y1 - y0);
      }
    }
    // the zone grid
    const [wx0, wz0] = toWorld(0, 0);
    const [wx1, wz1] = toWorld(W / dpr, H / dpr);
    ctx.strokeStyle = 'rgba(255,255,255,0.07)';
    ctx.lineWidth = 1;
    ctx.font = `${11 * dpr}px system-ui, sans-serif`;
    ctx.fillStyle = 'rgba(255,255,255,0.25)';
    // a label every few lines, so they never run into each other
    const every = Math.max(1, Math.ceil(70 / (CELL * view.scale)));
    for (let gx = Math.floor(wx0 / CELL) * CELL; gx <= wx1; gx += CELL) {
      const [sx] = toScreen(gx, 0);
      ctx.beginPath(); ctx.moveTo(sx, 0); ctx.lineTo(sx, H); ctx.stroke();
      if (Math.round(gx / CELL) % every === 0) ctx.fillText(String(gx), sx + 3 * dpr, H - 6 * dpr);
    }
    for (let gz = Math.floor(wz0 / CELL) * CELL; gz <= wz1; gz += CELL) {
      const [, sy] = toScreen(0, gz);
      ctx.beginPath(); ctx.moveTo(0, sy); ctx.lineTo(W, sy); ctx.stroke();
      if (Math.round(gz / CELL) % every === 0) ctx.fillText(String(gz), W - 50 * dpr, sy - 3 * dpr);
    }
    // NPCs
    // small and see-through when far out, where thousands overlap the terrain
    const r = Math.max(1.1, Math.min(5, view.scale * 40)) * dpr;
    ctx.globalAlpha = view.scale < 0.08 ? 0.7 : 1;
    const q = find.value.trim().toLowerCase();
    if (showNPC.checked) {
      for (const n of npcs) {
        const [sx, sy] = toScreen(n[1], n[2]);
        if (sx < -10 || sy < -10 || sx > W + 10 || sy > H + 10) continue;
        const hit = q && names[n[6]].toLowerCase().includes(q);
        ctx.fillStyle = hit ? '#ffffff' : colourOf(n);
        ctx.beginPath();
        ctx.arc(sx, sy, hit ? r * 1.8 : r, 0, Math.PI * 2);
        ctx.fill();
        if (view.scale > 0.6) {
          ctx.fillStyle = 'rgba(231,233,239,0.85)';
          ctx.fillText(names[n[6]], sx + r + 3 * dpr, sy + 4 * dpr);
        }
      }
    }
    ctx.globalAlpha = 1;
    // players
    if (showPlayers.checked) {
      for (const p of players) {
        const [sx, sy] = toScreen(p.x, p.z);
        const a = p.facing || 0;
        ctx.save();
        ctx.translate(sx, sy);
        ctx.rotate(-a + Math.PI);
        ctx.fillStyle = p.dead ? '#888' : '#ffffff';
        ctx.strokeStyle = '#000';
        ctx.lineWidth = 2 * dpr;
        ctx.beginPath();
        ctx.moveTo(0, -9 * dpr); ctx.lineTo(6 * dpr, 7 * dpr); ctx.lineTo(0, 4 * dpr); ctx.lineTo(-6 * dpr, 7 * dpr); ctx.closePath();
        ctx.stroke(); ctx.fill();
        ctx.restore();
        ctx.font = `600 ${12 * dpr}px system-ui, sans-serif`;
        ctx.lineWidth = 3 * dpr;
        ctx.strokeStyle = 'rgba(0,0,0,0.8)';
        ctx.strokeText(p.name, sx + 10 * dpr, sy - 8 * dpr);
        ctx.fillStyle = '#ffe680';
        ctx.fillText(p.name, sx + 10 * dpr, sy - 8 * dpr);
        ctx.font = `${11 * dpr}px system-ui, sans-serif`;
      }
    }
    // the selection
    if (selected) {
      const [sx, sy] = toScreen(selected.x, selected.z);
      ctx.strokeStyle = '#f0c36a';
      ctx.lineWidth = 2 * dpr;
      ctx.beginPath(); ctx.arc(sx, sy, 10 * dpr, 0, Math.PI * 2); ctx.stroke();
    }
  }

  // nearest marker to a screen point
  function pick(sx, sy) {
    let best = null, bd = (10 * dpr) ** 2;
    const px = sx * dpr, py = sy * dpr;
    if (showPlayers.checked) {
      for (const p of players) {
        const [x, y] = toScreen(p.x, p.z);
        const d = (x - px) ** 2 + (y - py) ** 2;
        if (d < bd) { bd = d; best = { kind: 'player', p, x: p.x, z: p.z }; }
      }
    }
    if (showNPC.checked && !best) {
      for (const n of npcs) {
        const [x, y] = toScreen(n[1], n[2]);
        const d = (x - px) ** 2 + (y - py) ** 2;
        if (d < bd) { bd = d; best = { kind: 'npc', n, x: n[1], z: n[2] }; }
      }
    }
    return best;
  }

  function showInfo(sel) {
    if (!sel) { info.classList.add('hidden'); return; }
    info.classList.remove('hidden');
    if (sel.kind === 'npc') {
      const [id, x, z, level, type, flags] = sel.n;
      mount(info, h('strong', null, names[sel.n[6]]), h('div', null, `NPC ${id} · level ${level}${flags & 1 ? ' · dead' : ''}${flags & 2 ? ' · GM spawn' : ''}`),
        h('div', { class: 'mono' }, `${x}, ${z}`),
        h('div', { class: 'row' },
          flags & 2 ? null : button('Open', { class: 'small', onclick: () => go('npcs/' + id) }),
          flags & 2 ? null : button('Move it…', { class: 'small', onclick: () => { moving = { id, name: names[sel.n[6]] }; toast(`Click where ${moving.name} should stand (its height stays; "Move to a character" on its page sets all three)`, 'info'); } })));
    } else {
      const p = sel.p;
      mount(info, h('strong', null, p.name), h('div', null, `level ${p.level}${p.dead ? ' · dead' : ''}`), h('div', { class: 'mono' }, `${p.x}, ${p.z}`),
        h('div', { class: 'row' }, button('Open', { class: 'small', onclick: () => go('characters/' + p.serverid) })));
    }
  }

  // input
  let drag = null;
  const pointers = new Map();
  canvas.addEventListener('pointerdown', (e) => {
    canvas.setPointerCapture(e.pointerId);
    pointers.set(e.pointerId, { x: e.offsetX, y: e.offsetY });
    drag = { x: e.offsetX, y: e.offsetY, cx: view.cx, cz: view.cz, moved: false };
  });
  canvas.addEventListener('pointermove', (e) => {
    if (pointers.has(e.pointerId)) pointers.set(e.pointerId, { x: e.offsetX, y: e.offsetY });
    if (pointers.size === 2) {
      const [a, b] = [...pointers.values()];
      const d = Math.hypot(a.x - b.x, a.y - b.y);
      if (drag.pinch) { zoomAt((a.x + b.x) / 2, (a.y + b.y) / 2, d / drag.pinch); }
      drag.pinch = d;
      drag.moved = true;
      return;
    }
    if (drag && pointers.size === 1) {
      const dx = e.offsetX - drag.x, dy = e.offsetY - drag.y;
      if (Math.abs(dx) + Math.abs(dy) > 3) drag.moved = true;
      if (drag.moved) {
        canvas.classList.add('dragging');
        view.cx = drag.cx - dx / view.scale;
        view.cz = drag.cz - dy / view.scale;
        draw();
      }
      return;
    }
    const sel = pick(e.offsetX, e.offsetY);
    hover = sel;
    if (sel) {
      tip.classList.remove('hidden');
      tip.textContent = sel.kind === 'npc' ? `${names[sel.n[6]]} (${sel.n[3]})` : `${sel.p.name} (${sel.p.level})`;
      tip.style.left = e.offsetX + 14 + 'px';
      tip.style.top = e.offsetY + 10 + 'px';
    } else tip.classList.add('hidden');
  });
  const up = async (e) => {
    pointers.delete(e.pointerId);
    canvas.classList.remove('dragging');
    if (drag && !drag.moved && pointers.size === 0) {
      if (moving) {
        const [x, z] = toWorld(e.offsetX, e.offsetY);
        const m = moving;
        moving = null;
        try {
          const out = await op('update_row', { table: 'npcs', key: { id: m.id }, values: { x_coord: Math.round(x * 10) / 10, z_coord: Math.round(z * 10) / 10, world } });
          toast(m.name + ' moved', 'ok', out.live || '');
          setTimeout(loadMarkers, 1500);
        } catch (err) { toast(err.message, 'error'); }
      } else {
        selected = pick(e.offsetX, e.offsetY);
        showInfo(selected);
        draw();
      }
    }
    if (pointers.size === 0) drag = null;
  };
  canvas.addEventListener('pointerup', up);
  canvas.addEventListener('pointercancel', up);
  canvas.addEventListener('pointerleave', () => tip.classList.add('hidden'));
  function zoomAt(sx, sy, f) {
    const [wx, wz] = toWorld(sx, sy);
    view.scale = Math.max(0.002, Math.min(8, view.scale * f));
    const [nx, nz] = toWorld(sx, sy);
    view.cx += wx - nx;
    view.cz += wz - nz;
    draw();
  }
  canvas.addEventListener('wheel', (e) => { e.preventDefault(); zoomAt(e.offsetX, e.offsetY, e.deltaY < 0 ? 1.25 : 0.8); }, { passive: false });

  function fit() {
    if (!npcs.length && !players.length) return;
    let x0 = Infinity, z0 = Infinity, x1 = -Infinity, z1 = -Infinity;
    for (const n of npcs) { x0 = Math.min(x0, n[1]); x1 = Math.max(x1, n[1]); z0 = Math.min(z0, n[2]); z1 = Math.max(z1, n[2]); }
    for (const p of players) { x0 = Math.min(x0, p.x); x1 = Math.max(x1, p.x); z0 = Math.min(z0, p.z); z1 = Math.max(z1, p.z); }
    const r = wrap.getBoundingClientRect();
    view.cx = (x0 + x1) / 2;
    view.cz = (z0 + z1) / 2;
    view.scale = Math.min(r.width / Math.max(500, x1 - x0), r.height / Math.max(500, z1 - z0)) * 0.9;
  }

  async function loadMarkers(first = false) {
    try {
      const out = await op('map_markers', { world });
      npcs = out.npcs; names = out.names; players = out.players;
      showStatus();
      if (first) {
        const f = focus !== null ? npcs.find((n) => n[0] === focus) : null;
        if (f) { view.cx = f[1]; view.cz = f[2]; view.scale = 0.8; selected = { kind: 'npc', n: f, x: f[1], z: f[2] }; showInfo(selected); }
        else fit();
      }
      draw();
    } catch (e) { toast(e.message, 'error'); }
  }
  async function loadPlayers() {
    try {
      const out = await op('map_markers', { world });
      players = out.players;
      draw();
    } catch { /* next time */ }
  }
  let terrainNote = '';
  function loadTerrain() {
    terrain?.cancel();
    terrain = discTerrain(world, () => draw(), (msg) => { terrainNote = msg; showStatus(); });
  }
  function showStatus() {
    status.textContent = `${npcs.length.toLocaleString()} NPCs · ${players.length} players` + (terrainNote ? ' · ' + terrainNote : '');
  }

  worldSel.addEventListener('change', async () => {
    world = Number(worldSel.value);
    store('map.world', String(world));
    selected = null; showInfo(null);
    await loadMarkers(true);
    loadTerrain();
  });
  find.addEventListener('input', debounce(() => {
    draw();
    const q = find.value.trim().toLowerCase();
    if (!q) return;
    const p = players.find((x) => x.name.toLowerCase().includes(q));
    const n = npcs.find((x) => names[x[6]].toLowerCase().includes(q));
    const t = p ? { x: p.x, z: p.z } : n ? { x: n[1], z: n[2] } : null;
    if (t) { view.cx = t.x; view.cz = t.z; view.scale = Math.max(view.scale, 0.3); draw(); }
  }, 300));
  [showNPC, showPlayers, showTerrain].forEach((c) => c.addEventListener('change', draw));

  await loadMarkers(true);
  loadTerrain();
  const timer = setInterval(loadPlayers, 3000);
  return { leave: () => { clearInterval(timer); ro.disconnect(); terrain?.cancel(); } };
}
