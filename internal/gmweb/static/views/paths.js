// The path editor on the world map (views/map.js): a GM gives an NPC a path to
// walk (entity/path.go on the server, table npcpaths). Pick an NPC on the map
// and press Path, or "Path on the map" on its page; then click the map to add
// a point (after the selected one), drag a point to move it, click a line to
// put a point between two, and Delete takes the selected one away. Save gives
// the NPC the path at once (audited, revertible from the Audit log), and its
// place is drawn as it walks. The world's other paths are drawn faint.

import { h, mount, button, toast, confirmBox } from '../lib/dom.js';
import { op } from '../lib/api.js';

const MODES = [['loop', 'Loop'], ['pingpong', 'Back and forth'], ['once', 'Once, then stand']];
const SPEEDS = [['walk', 'Walk'], ['run', 'Run']];
const STATES = {
  walking: 'walking to point', paused: 'standing at point', holding: 'standing at its end, point', fighting: 'fighting (off its path)',
  rooted: 'rooted', dead: 'dead', away: 'in another world than its path', home: 'walking back to its spawn point',
  starting: 'about to set off', none: 'no path',
};

// how near (CSS pixels) a press must be to a point or a line to take it
const HIT_POINT = 10;
const HIT_LEG = 7;

export class PathEditor {
  // o: { wrap, ctx, toScreen(x, z) -> canvas pixels, toWorld(cssX, cssY) -> [x, z], dpr(), redraw(), world(),
  //      fit(x0, z0, x1, z1): the view on that ground }
  constructor(o) {
    Object.assign(this, o);
    this.others = [];
    this.ed = null;
    this.live = null;
    this.dragging = -1;
    this.timer = null;
    this.panel = h('div', { class: 'path-panel hidden', role: 'region', 'aria-label': 'Path editor' });
    this.wrap.appendChild(this.panel);
    this.onKey = (e) => this.key(e);
    document.addEventListener('keydown', this.onKey);
  }

  get editing() { return !!this.ed; }

  // the world's paths, for drawing (the edited one excepted)
  async loadWorld() {
    try {
      const out = await op('npc_paths', { world: this.world() });
      this.others = out.paths || [];
    } catch (e) { this.others = []; toast(e.message, 'error'); }
    this.redraw();
  }

  // start editing one NPC's path
  async edit(id) {
    if (this.ed && this.ed.dirty && this.ed.npcid !== id) {
      if (!(await confirmBox({ title: 'Leave this path?', message: `${this.ed.name}'s path has changes that are not saved.`, confirmLabel: 'Leave it' }))) return;
    }
    let out;
    try { out = await op('npc_path', { id }); } catch (e) { toast(e.message, 'error'); return; }
    if (out.world !== this.world()) {
      toast(`${out.name} is in another world than this map's`, 'warn');
      return;
    }
    const p = out.path;
    const was = this.ed;
    this.ed = {
      npcid: out.npcid, name: out.name, spawn: out.spawn, had: !!p,
      mode: p ? p.mode : 'loop', pause: p ? p.pause : 0, speed: p ? p.speed : 'walk',
      points: p ? p.points.map((q) => ({ x: q.x, y: q.y, z: q.z, pause: q.pause ?? null })) : [],
      selected: -1, dirty: false,
    };
    const first = !was || was.npcid !== out.npcid;
    this.live = out.live || null;
    this.wrap.classList.add('path-editing');
    if (out.error) toast(`Its stored path does not read: ${out.error}`, 'warn');
    if (first) this.focus();
    this.poll();
    this.showPanel();
    this.redraw();
  }

  // the view on the path and the NPC, near enough to place points by
  focus() {
    const xs = this.ed.points.map((p) => p.x), zs = this.ed.points.map((p) => p.z);
    const at = this.live || this.ed.spawn;
    if (at) { xs.push(at.x); zs.push(at.z); }
    if (!xs.length || !this.fit) return;
    this.fit(Math.min(...xs), Math.min(...zs), Math.max(...xs), Math.max(...zs));
  }

  async close() {
    if (this.ed && this.ed.dirty) {
      if (!(await confirmBox({ title: 'Close without saving?', message: `${this.ed.name}'s path has changes that are not saved.`, confirmLabel: 'Close' }))) return false;
    }
    this.ed = null;
    this.live = null;
    clearInterval(this.timer);
    this.timer = null;
    this.panel.classList.add('hidden');
    this.wrap.classList.remove('path-editing');
    this.redraw();
    return true;
  }

  leave() {
    clearInterval(this.timer);
    document.removeEventListener('keydown', this.onKey);
  }

  // the edited NPC's place, once a second
  poll() {
    clearInterval(this.timer);
    const id = this.ed.npcid;
    this.timer = setInterval(async () => {
      if (!this.ed || this.ed.npcid !== id || document.hidden) return;
      try {
        const out = await op('npc_path', { id });
        if (this.ed && this.ed.npcid === id) { this.live = out.live || null; this.showLive(); this.redraw(); }
      } catch { /* the next time */ }
    }, 1000);
  }

  changed() {
    this.ed.dirty = true;
    this.showPanel();
    this.redraw();
  }

  // ---- the canvas

  // the edited path's point and leg nearest a CSS-pixel place
  hit(cx, cy) {
    if (!this.ed) return null;
    const dpr = this.dpr();
    const px = cx * dpr, py = cy * dpr;
    const pts = this.ed.points.map((p) => this.toScreen(p.x, p.z));
    let best = null, bd = (HIT_POINT * dpr) ** 2;
    pts.forEach(([x, y], i) => {
      const d = (x - px) ** 2 + (y - py) ** 2;
      if (d < bd) { bd = d; best = { point: i }; }
    });
    if (best) return best;
    let ld = HIT_LEG * dpr;
    for (const [a, b] of this.legs()) {
      const [x1, y1] = pts[a], [x2, y2] = pts[b];
      const dx = x2 - x1, dy = y2 - y1, l2 = dx * dx + dy * dy;
      if (!l2) continue;
      const t = Math.max(0, Math.min(1, ((px - x1) * dx + (py - y1) * dy) / l2));
      const d = Math.hypot(px - (x1 + t * dx), py - (y1 + t * dy));
      if (d < ld) { ld = d; best = { leg: [a, b] }; }
    }
    return best;
  }

  // the legs walked, as point index pairs
  legs() {
    const n = this.ed ? this.ed.points.length : 0;
    const out = [];
    for (let i = 0; i + 1 < n; i++) out.push([i, i + 1]);
    if (this.ed && this.ed.mode === 'loop' && n > 2) out.push([n - 1, 0]);
    return out;
  }

  // a press: true when the editor takes it (a point to drag)
  down(cx, cy) {
    const hit = this.hit(cx, cy);
    if (!hit || hit.point === undefined) return false;
    this.ed.selected = hit.point;
    this.dragging = hit.point;
    this.dragMoved = false;
    this.showPanel();
    this.redraw();
    return true;
  }

  move(cx, cy) {
    if (this.dragging < 0) return false;
    const [x, z] = this.toWorld(cx, cy);
    const p = this.ed.points[this.dragging];
    p.x = Math.round(x * 10) / 10;
    p.z = Math.round(z * 10) / 10;
    p.y = null; // the ground's, at the save
    this.dragMoved = true;
    this.ed.dirty = true;
    this.redraw();
    return true;
  }

  // a press let go: true when the editor took it
  up() {
    if (this.dragging < 0) return false;
    this.dragging = -1;
    if (this.dragMoved) this.changed();
    return true;
  }

  // a click on the map (no drag) while editing: a point between two on a
  // line, else one after the selected point (or at the end)
  click(cx, cy) {
    if (!this.ed) return false;
    const hit = this.hit(cx, cy);
    if (hit && hit.point !== undefined) return true; // down() selected it
    if (this.ed.points.length >= 256) { toast('A path holds at most 256 points', 'warn'); return true; }
    const [x, z] = this.toWorld(cx, cy);
    const p = { x: Math.round(x * 10) / 10, y: null, z: Math.round(z * 10) / 10, pause: null };
    let at;
    if (hit && hit.leg) at = hit.leg[1] === 0 ? this.ed.points.length : hit.leg[1];
    else at = this.ed.selected >= 0 ? this.ed.selected + 1 : this.ed.points.length;
    this.ed.points.splice(at, 0, p);
    this.ed.selected = at;
    this.changed();
    return true;
  }

  key(e) {
    if (!this.ed || ['INPUT', 'TEXTAREA', 'SELECT'].includes(document.activeElement?.tagName)) return;
    if ((e.key === 'Delete' || e.key === 'Backspace') && this.ed.selected >= 0) {
      e.preventDefault();
      this.removeSelected();
    } else if (e.key === 'Escape' && this.ed.selected >= 0) {
      this.ed.selected = -1;
      this.showPanel();
      this.redraw();
    }
  }

  removeSelected() {
    const i = this.ed.selected;
    if (i < 0) return;
    this.ed.points.splice(i, 1);
    this.ed.selected = Math.min(i, this.ed.points.length - 1);
    this.changed();
  }

  draw() {
    const ctx = this.ctx, dpr = this.dpr();
    const line = (pts, closed) => {
      if (pts.length < 2) return;
      ctx.beginPath();
      ctx.moveTo(...pts[0]);
      for (const p of pts.slice(1)) ctx.lineTo(...p);
      if (closed) ctx.closePath();
      ctx.stroke();
    };
    // the world's other paths, faint
    ctx.lineWidth = 1.5 * dpr;
    ctx.strokeStyle = 'rgba(143, 180, 255, 0.55)';
    ctx.fillStyle = 'rgba(143, 180, 255, 0.8)';
    for (const o of this.others) {
      if (this.ed && o.npcid === this.ed.npcid) continue;
      const pts = (o.path?.points || []).map((p) => this.toScreen(p.x, p.z));
      line(pts, o.path.mode === 'loop' && pts.length > 2);
      for (const [x, y] of pts) { ctx.beginPath(); ctx.arc(x, y, 2.5 * dpr, 0, Math.PI * 2); ctx.fill(); }
    }
    if (!this.ed) return;
    const pts = this.ed.points.map((p) => this.toScreen(p.x, p.z));
    // the legs, the way they are walked, an arrow at each middle
    ctx.strokeStyle = '#f0c36a';
    ctx.fillStyle = '#f0c36a';
    ctx.lineWidth = 2.5 * dpr;
    for (const [a, b] of this.legs()) {
      const [x1, y1] = pts[a], [x2, y2] = pts[b];
      ctx.setLineDash(this.ed.mode === 'loop' && b === 0 ? [6 * dpr, 5 * dpr] : []);
      ctx.beginPath(); ctx.moveTo(x1, y1); ctx.lineTo(x2, y2); ctx.stroke();
      const mx = (x1 + x2) / 2, my = (y1 + y2) / 2, ang = Math.atan2(y2 - y1, x2 - x1), s = 6 * dpr;
      if (Math.hypot(x2 - x1, y2 - y1) > 30 * dpr) {
        ctx.beginPath();
        ctx.moveTo(mx + Math.cos(ang) * s, my + Math.sin(ang) * s);
        ctx.lineTo(mx + Math.cos(ang + 2.5) * s, my + Math.sin(ang + 2.5) * s);
        ctx.lineTo(mx + Math.cos(ang - 2.5) * s, my + Math.sin(ang - 2.5) * s);
        ctx.closePath(); ctx.fill();
      }
    }
    ctx.setLineDash([]);
    // where it is and where it walks to
    const lv = this.live;
    if (lv && lv.world === this.world()) {
      const [lx, ly] = this.toScreen(lv.x, lv.z);
      const next = this.ed.points[lv.next];
      if (next && !this.ed.dirty && (lv.state === 'walking' || lv.state === 'starting')) {
        const [nx, ny] = this.toScreen(next.x, next.z);
        ctx.strokeStyle = 'rgba(69, 195, 138, 0.8)';
        ctx.lineWidth = 1.5 * dpr;
        ctx.setLineDash([3 * dpr, 4 * dpr]);
        ctx.beginPath(); ctx.moveTo(lx, ly); ctx.lineTo(nx, ny); ctx.stroke();
        ctx.setLineDash([]);
      }
      ctx.fillStyle = '#45c38a';
      ctx.strokeStyle = '#ffffff';
      ctx.lineWidth = 2 * dpr;
      ctx.beginPath(); ctx.arc(lx, ly, 6 * dpr, 0, Math.PI * 2); ctx.fill(); ctx.stroke();
    }
    // the points, numbered
    ctx.font = `600 ${11 * dpr}px system-ui, sans-serif`;
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    pts.forEach(([x, y], i) => {
      const sel = i === this.ed.selected;
      ctx.beginPath();
      ctx.arc(x, y, 8 * dpr, 0, Math.PI * 2);
      ctx.fillStyle = sel ? '#f0c36a' : '#12151c';
      ctx.fill();
      ctx.lineWidth = 2 * dpr;
      ctx.strokeStyle = '#f0c36a';
      ctx.stroke();
      ctx.fillStyle = sel ? '#1c1405' : '#f0c36a';
      ctx.fillText(String(i + 1), x, y + 0.5 * dpr);
    });
    ctx.textAlign = 'start';
    ctx.textBaseline = 'alphabetic';
  }

  // ---- the panel

  showLive() {
    if (!this.liveLine) return;
    const lv = this.live;
    let text = 'not in the running world';
    if (lv) {
      text = STATES[lv.state] || lv.state;
      if (['walking', 'paused', 'holding'].includes(lv.state)) text += ' ' + (lv.next + 1);
      if (this.ed?.dirty && lv.state !== 'none') text += ' (of the saved path)';
    }
    this.liveLine.textContent = 'Now: ' + text;
  }

  showPanel() {
    const ed = this.ed;
    if (!ed) return;
    this.panel.classList.remove('hidden');
    const select = (opts, value, onchange, label) => {
      const s = h('select', { 'aria-label': label }, ...opts.map(([v, l]) => h('option', { value: v, selected: v === value }, l)));
      s.addEventListener('change', () => onchange(s.value));
      return s;
    };
    const num = (value, onchange, label, attrs = {}) => {
      const i = h('input', { type: 'number', step: 'any', value: value ?? '', 'aria-label': label, ...attrs });
      i.addEventListener('change', () => onchange(i.value === '' ? null : Number(i.value)));
      return i;
    };
    const sel = ed.selected >= 0 ? ed.points[ed.selected] : null;
    this.liveLine = h('div', { class: 'faint small' });
    mount(this.panel,
      h('div', { class: 'path-head' },
        h('strong', null, ed.name), h('span', { class: 'faint small' }, ` NPC ${ed.npcid}`),
        h('button', { class: 'ghost icon', type: 'button', 'aria-label': 'Close the path editor', title: 'Close', onclick: () => this.close() }, '×')),
      h('div', { class: 'path-row' },
        h('label', null, 'Mode', select(MODES, ed.mode, (v) => { ed.mode = v; this.changed(); }, 'Mode')),
        h('label', null, 'Pause (s)', num(ed.pause, (v) => { ed.pause = v ?? 0; this.changed(); }, 'Pause at each point', { min: 0, max: 600 })),
        h('label', null, 'Speed', select(SPEEDS, ed.speed, (v) => { ed.speed = v; this.changed(); }, 'Speed'))),
      h('div', { class: 'small' }, `${ed.points.length} point${ed.points.length === 1 ? '' : 's'}`,
        ed.dirty ? h('span', { class: 'badge warn' }, 'not saved') : ed.had ? h('span', { class: 'badge ok' }, 'saved') : h('span', { class: 'badge' }, 'no path yet')),
      sel ? h('div', { class: 'path-point' },
        h('div', { class: 'small' }, h('strong', null, `Point ${ed.selected + 1}`)),
        h('div', { class: 'path-row' },
          h('label', null, 'X', num(sel.x, (v) => { if (v !== null) { sel.x = v; sel.y = null; this.changed(); } }, 'X')),
          h('label', null, 'Y', num(sel.y, (v) => { sel.y = v; this.changed(); }, 'Height', { placeholder: 'ground' })),
          h('label', null, 'Z', num(sel.z, (v) => { if (v !== null) { sel.z = v; sel.y = null; this.changed(); } }, 'Z')),
          h('label', null, 'Pause', num(sel.pause, (v) => { sel.pause = v; this.changed(); }, 'Pause at this point', { min: 0, max: 600, placeholder: 'path' }))),
        button('Delete point', { class: 'small danger', onclick: () => this.removeSelected() })) : null,
      h('div', { class: 'row' },
        button('Save', { class: 'small primary', onclick: () => this.save() }),
        button('Revert', { class: 'small', title: 'Discard the changes', onclick: () => this.revert() }),
        ed.had ? button('Remove path', { class: 'small danger', onclick: () => this.remove() }) : null),
      this.liveLine,
      h('p', { class: 'faint small path-hint' },
        'Click the map to add a point (after the selected one), drag a point to move it, click a line to put one between two; Delete removes the selected point. The height is the ground\'s unless you set one. Saving gives it the path at once.'));
    this.showLive();
  }

  async save() {
    const ed = this.ed;
    if (!ed.points.length) { toast('Add a point first, or Remove path', 'warn'); return; }
    const points = ed.points.map((p) => {
      const o = { x: p.x, z: p.z };
      if (p.y !== null && p.y !== undefined) o.y = p.y;
      if (p.pause !== null && p.pause !== undefined) o.pause = p.pause;
      return o;
    });
    try {
      const out = await op('set_npc_path', { id: ed.npcid, points, mode: ed.mode, pause: ed.pause || 0, speed: ed.speed });
      toast(`${ed.name}'s path saved`, 'ok', out.live || '');
      ed.dirty = false;
      await this.edit(ed.npcid);
      this.loadWorld();
    } catch (e) { toast(e.message, 'error'); }
  }

  async revert() {
    if (this.ed.dirty && !(await confirmBox({ title: 'Discard the changes?', message: 'The path goes back to what is saved.', confirmLabel: 'Discard' }))) return;
    this.ed.dirty = false;
    await this.edit(this.ed.npcid);
  }

  async remove() {
    const ed = this.ed;
    if (!(await confirmBox({ title: `Take ${ed.name}'s path away?`, message: 'It walks back to its spawn point and is as before. The Audit log can put the path back.', confirmLabel: 'Remove path', danger: true }))) return;
    try {
      const out = await op('clear_npc_path', { id: ed.npcid });
      toast('Path removed', 'ok', out.live || '');
      ed.dirty = false;
      await this.edit(ed.npcid);
      this.loadWorld();
    } catch (e) { toast(e.message, 'error'); }
  }
}
