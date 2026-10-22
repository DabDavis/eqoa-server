// One row as a form, laid out by the catalog: enumerations as lists, bit sets
// as checkboxes, asset ids in hex, references with the name they point at and
// a picker. Save sends only what changed, with what it was, so a row moved
// meanwhile is a conflict rather than overwritten.

import { h, mount, button, toast, dialog, confirmBox, debounce } from './dom.js';
import { op, ApiError } from './api.js';
import { hex32 } from './fmt.js';
import { lookupName, describe } from './meta.js';

// the value of an input as sent back, by column
function readInput(col, el) {
  if (el.type === 'checkbox') return el.checked ? 1 : 0;
  const v = el.value.trim();
  if (v === '') return col.notnull ? (col.affinity === 'TEXT' ? '' : 0) : null;
  if (col.format === 'hex32') return v;
  if (col.affinity === 'INTEGER' || col.affinity === 'REAL') return /^-?0x/i.test(v) ? v : Number(v);
  return el.value;
}

function same(a, b) {
  if (a === null || a === undefined || a === '') return b === null || b === undefined || b === '';
  if (typeof a === 'string' && /^-?0x/i.test(a)) a = parseInt(a, 16);
  if (typeof b === 'string' && /^-?0x/i.test(b)) b = parseInt(b, 16);
  const na = Number(a), nb = Number(b);
  if (Number.isFinite(na) && Number.isFinite(nb) && String(a).trim() !== '' && String(b).trim() !== '') {
    return na === nb || ((na % 4294967296) + 4294967296) % 4294967296 === ((nb % 4294967296) + 4294967296) % 4294967296;
  }
  return String(a) === String(b);
}

export class RecordForm {
  // mode "update" edits row (with its __key); "insert" makes a new one
  constructor({ meta, row, mode = 'update', canWrite = true, onSaved, onDeleted, extraActions = [] }) {
    this.meta = meta;
    this.mode = mode;
    this.row = row || {};
    this.canWrite = canWrite && meta.write !== 'none';
    this.onSaved = onSaved;
    this.onDeleted = onDeleted;
    this.inputs = new Map();
    this.savebar = h('div', { class: 'savebar hidden' });
    this.el = h('div', { class: 'record' });
    this.extraActions = extraActions;
    this.build();
  }

  build() {
    const sections = new Map();
    for (const c of this.meta.columns) {
      const s = c.section || (c.pk ? 'Key' : 'Other');
      if (!sections.has(s)) sections.set(s, []);
      sections.get(s).push(c);
    }
    const order = [...sections.keys()].sort((a, b) => (a === 'Key' ? -1 : b === 'Key' ? 1 : a === 'Other' ? 1 : b === 'Other' ? -1 : 0));
    const box = h('div', { class: 'form-sections' });
    for (const name of order) {
      const fields = h('div', { class: 'fields' });
      for (const c of sections.get(name)) fields.appendChild(this.field(c));
      box.appendChild(h('div', { class: 'form-section' }, sections.size > 1 ? h('h3', null, name) : null, fields));
    }
    mount(this.el, box, this.savebar);
    this.refresh();
  }

  field(c) {
    const v = this.row[c.name];
    const locked = !this.canWrite || (c.readonly && !(this.mode === 'insert' && c.pk));
    const wrap = h('div', { class: 'field' + (c.format === 'multiline' || c.format === 'json' || c.flags ? ' wide' : '') });
    let input, extra = null;
    const hint = [];
    if (c.help) hint.push(c.help);
    if (c.enum) {
      const list = this.meta.enums?.[c.enum] || [];
      const opts = list.map((e) => h('option', { value: String(e.value) }, `${e.label} (${e.value})`));
      if (v !== null && v !== undefined && !list.some((e) => String(e.value) === String(v))) opts.unshift(h('option', { value: String(v) }, String(v)));
      if (!c.notnull) opts.unshift(h('option', { value: '' }, '—'));
      input = h('select', null, ...opts);
      input.value = v === null || v === undefined ? '' : String(v);
    } else if (c.format === 'multiline' || c.format === 'json') {
      input = h('textarea', { class: c.format === 'json' ? 'code' : '' });
      input.value = v ?? '';
    } else if (c.format === 'hex32') {
      input = h('input', { type: 'text', class: 'mono', spellcheck: 'false', autocomplete: 'off' });
      input.value = v === null || v === undefined ? '' : hex32(v);
      const dec = h('span', { class: 'hint' });
      const show = () => { const n = parseInt(input.value, 16); dec.textContent = Number.isFinite(n) && /^0x/i.test(input.value.trim()) ? '= ' + n : ''; };
      input.addEventListener('input', show);
      show();
      extra = dec;
    } else {
      const numeric = c.affinity === 'INTEGER' || c.affinity === 'REAL';
      input = h('input', { type: numeric ? 'text' : 'text', inputmode: numeric ? 'decimal' : undefined, autocomplete: 'off', spellcheck: numeric ? 'false' : undefined });
      input.value = v === null || v === undefined ? '' : String(v);
      if (c.format === 'angle256') {
        const deg = h('span', { class: 'hint' });
        const show = () => { const n = Number(input.value); deg.textContent = Number.isFinite(n) ? `≈ ${Math.round((n / 256) * 360)}°` : ''; };
        input.addEventListener('input', show);
        show();
        extra = deg;
      } else if (c.format === 'radians') {
        const deg = h('span', { class: 'hint' });
        const show = () => { const n = Number(input.value); deg.textContent = Number.isFinite(n) ? `≈ ${Math.round((n * 180) / Math.PI)}°` : ''; };
        input.addEventListener('input', show);
        show();
        extra = deg;
      }
    }
    input.disabled = locked;
    input.setAttribute('aria-label', c.label || c.name);
    this.inputs.set(c.name, input);

    let refLine = null;
    if (c.ref) {
      refLine = h('div', { class: 'refname' });
      const show = debounce(() => {
        const key = input.value.trim();
        refLine.textContent = '';
        if (key === '') return;
        lookupName(c.ref, key).then((name) => { refLine.textContent = name ? '→ ' + name : '→ (none)'; });
      }, 200);
      input.addEventListener('input', show);
      show();
    }
    let flagBox = null;
    if (c.flags) {
      const bits = this.meta.flags?.[c.flags] || [];
      flagBox = h('div', { class: 'flagset' });
      const boxes = bits.map((b) => {
        const box = h('input', { type: 'checkbox', disabled: locked });
        box.addEventListener('change', () => {
          let n = Number(input.value) || 0;
          n = box.checked ? n | b.bit : n & ~b.bit;
          input.value = String(n);
          this.refresh();
        });
        flagBox.appendChild(h('label', { title: b.help || '' }, box, b.label));
        return { b, box };
      });
      const sync = () => { const n = Number(input.value) || 0; boxes.forEach(({ b, box }) => { box.checked = (n & b.bit) === b.bit; }); };
      input.addEventListener('input', sync);
      sync();
    }
    input.addEventListener('input', () => this.refresh());
    input.addEventListener('change', () => this.refresh());

    const control = c.ref && !locked
      ? h('div', { class: 'with' }, input, button('Pick', { class: 'small', onclick: async () => {
        const key = await pickRow(c.ref, c.label || c.name);
        if (key !== null) { input.value = String(key); input.dispatchEvent(new Event('input')); }
      } }))
      : input;
    wrap.appendChild(h('label', null,
      h('span', null, c.label || c.name, c.label && c.label !== c.name ? h('span', { class: 'faint' }, ' · ' + c.name) : null),
      control, extra, flagBox, refLine,
      hint.length ? h('span', { class: 'hint' }, hint.join(' ')) : null));
    return wrap;
  }

  changes() {
    const out = {};
    for (const [name, el] of this.inputs) {
      const c = this.meta.columns.find((x) => x.name === name);
      if (el.disabled) continue;
      const nv = readInput(c, el);
      if (this.mode === 'insert') {
        if (!(nv === null || nv === '')) out[name] = nv;
      } else if (!same(nv, this.row[name])) out[name] = nv;
    }
    return out;
  }

  refresh() {
    const ch = this.changes();
    const n = Object.keys(ch).length;
    for (const [name, el] of this.inputs) el.closest('.field')?.classList.toggle('dirty', name in ch && this.mode !== 'insert');
    if (!this.canWrite) { this.savebar.classList.add('hidden'); return; }
    if (this.mode === 'insert') {
      this.savebar.classList.remove('hidden');
      mount(this.savebar, h('span', { class: 'grow muted' }, 'A new row of ' + this.meta.label),
        button('Add', { class: 'primary', icon: 'save', onclick: () => this.save() }));
      return;
    }
    this.savebar.classList.toggle('hidden', n === 0);
    if (n) {
      mount(this.savebar, h('span', { class: 'grow' }, `${n} change${n > 1 ? 's' : ''}: ${Object.keys(ch).join(', ')}`),
        button('Discard', { class: 'ghost', onclick: () => this.build() }),
        button('Preview', { class: 'small', onclick: () => this.save(true) }),
        button('Save', { class: 'primary', icon: 'save', onclick: () => this.save() }));
    }
  }

  async save(dry = false) {
    const values = this.changes();
    try {
      let out;
      if (this.mode === 'insert') {
        out = await op('insert_row', { table: this.meta.name, values, dry_run: dry });
      } else {
        const expect = {};
        for (const k of Object.keys(values)) expect[k] = this.row[k] ?? null;
        out = await op('update_row', { table: this.meta.name, key: this.row.__key, values, expect, dry_run: dry });
      }
      if (dry) {
        showChange(out, 'Preview: nothing is written');
        return;
      }
      toast(this.mode === 'insert' ? 'Added' : 'Saved', 'ok', out.live || '');
      if (out.after) {
        this.row = { ...out.after, __key: out.key };
        this.mode = 'update';
        this.build();
      }
      this.onSaved?.(out);
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) {
        const reload = await confirmBox({
          title: 'The row changed since you opened it',
          message: e.message + ' Reload it to see the change (your edits are lost), or cancel to keep editing.',
          confirmLabel: 'Reload',
        });
        if (reload && e.data?.current) { this.row = { ...e.data.current, __key: this.row.__key }; this.build(); }
        return;
      }
      toast(e.message, 'error');
    }
  }

  async remove() {
    if (!(await confirmBox({ title: 'Delete this row?', message: `It goes from ${this.meta.label}. The audit log keeps it, and Revert puts it back.`, confirmLabel: 'Delete', danger: true }))) return;
    try {
      const out = await op('delete_row', { table: this.meta.name, key: this.row.__key });
      toast('Deleted', 'ok', out.live || out.change?.live || '');
      this.onDeleted?.(out);
    } catch (e) {
      toast(e.message, 'error');
    }
  }
}

// showChange shows a change (or a preview of one) as a before/after table.
export function showChange(ch, title = 'The change') {
  const before = ch.before || {};
  const after = ch.after || {};
  const keys = [...new Set([...Object.keys(before), ...Object.keys(after)])].filter((k) => k !== '__key' && !same(before[k], after[k]));
  dialog({
    title, wide: true,
    body: h('div', { class: 'stack' },
      ch.action ? h('p', null, `${ch.action} on ${ch.table}`) : null,
      keys.length
        ? h('div', { class: 'diff' }, ...keys.flatMap((k) => [h('span', { class: 'k' }, k), h('span', { class: 'b' }, fmtV(before[k])), h('span', { class: 'a' }, fmtV(after[k]))]))
        : h('p', { class: 'muted' }, 'No column changes.'),
      ch.live ? h('p', { class: 'notice info' }, ch.live) : null),
    actions: [{ label: 'Close' }],
  });
}

function fmtV(v) {
  if (v === undefined) return '';
  if (v === null) return 'null';
  return typeof v === 'object' ? JSON.stringify(v) : String(v);
}

// pickRow asks for a row of a table by searching it; resolves its key (one
// column) or null.
export async function pickRow(table, what = 'row') {
  const meta = await describe(table);
  return new Promise((resolve) => {
    let done = false;
    const search = h('input', { type: 'search', placeholder: `Search ${meta.label.toLowerCase()} by name or id`, autocomplete: 'off' });
    const list = h('div', { class: 'stack' });
    const title = meta.title || meta.key[0];
    let dlg;
    const run = debounce(async () => {
      const q = search.value.trim();
      try {
        const out = await op('query_rows', { table, search: q, limit: 30, columns: [meta.key[0], title, ...(meta.show || []).slice(0, 4)] });
        mount(list, ...out.rows.map((r) => {
          const b = h('button', { type: 'button', class: 'ghost' }, h('strong', null, String(r[title] ?? '')), ' ', h('span', { class: 'muted mono' }, '#' + r[meta.key[0]]));
          b.addEventListener('click', () => { done = true; resolve(r[meta.key[0]]); dlg.close(); });
          return b;
        }), out.total > out.rows.length ? h('p', { class: 'faint small' }, `${out.total - out.rows.length} more: type more of the name`) : null);
      } catch (e) {
        mount(list, h('p', { class: 'err' }, e.message));
      }
    }, 200);
    search.addEventListener('input', run);
    dlg = dialog({ title: 'Pick ' + what, body: h('div', { class: 'stack' }, search, list), onClose: () => { if (!done) resolve(null); } });
    run();
  });
}
