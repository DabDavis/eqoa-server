// A table's rows, a page at a time: search, filters, sort, columns, paging.
// The server does the work (query_rows); this draws what it sends.

import { h, mount, icon, button, formBox, debounce, store, empty } from './dom.js';
import { op } from './api.js';
import { cell, isNumeric } from './fmt.js';

const OPS = [
  ['=', 'is'], ['!=', 'is not'], ['<', 'below'], ['<=', 'at most'], ['>', 'above'], ['>=', 'at least'],
  ['like', 'contains'], ['prefix', 'starts with'], ['bits', 'has every bit of'], ['null', 'is empty'], ['notnull', 'is not empty'],
];

export class DataGrid {
  constructor({ table, meta, columns, onOpen, tools = [], filters = [], search = '', pageSize = 50, emptyText, onLoaded }) {
    this.table = table;
    this.meta = meta;
    this.onOpen = onOpen;
    this.onLoaded = onLoaded;
    this.filters = filters.slice();
    this.search = search;
    this.limit = pageSize;
    this.offset = 0;
    this.sort = '';
    this.desc = false;
    this.total = 0;
    this.emptyText = emptyText || 'Nothing matches.';
    const saved = store('cols.' + table);
    const known = new Set(meta.columns.map((c) => c.name));
    this.columns = (saved ? saved.split(',') : columns || meta.show || meta.columns.slice(0, 9).map((c) => c.name)).filter((c) => known.has(c));
    if (!this.columns.length) this.columns = meta.columns.slice(0, 9).map((c) => c.name);

    this.searchBox = h('input', { type: 'search', placeholder: meta.search?.length ? 'Search ' + meta.label.toLowerCase() + '…' : 'Search (no text columns)', value: search, 'aria-label': 'Search' });
    this.searchBox.addEventListener('input', debounce(() => { this.search = this.searchBox.value; this.offset = 0; this.load(); }, 250));
    this.filterBar = h('div', { class: 'filters hidden' });
    this.body = h('div', { class: 'grid-scroll' });
    this.foot = h('div', { class: 'grid-foot' });
    this.el = h('div', { class: 'grid-wrap' },
      h('div', { class: 'grid-tools' }, this.searchBox,
        button('Filter', { icon: 'plus', class: 'small', onclick: () => this.addFilter() }),
        button('Columns', { icon: 'table', class: 'small', onclick: () => this.chooseColumns() }),
        button('', { icon: 'refresh', class: 'small ghost', title: 'Reload', onclick: () => this.load() }),
        h('span', { class: 'grow' }), ...tools),
      this.filterBar, this.body, this.foot);
    this.drawFilters();
  }

  query() {
    return {
      table: this.table, search: this.search, filters: this.filters, sort: this.sort, desc: this.desc,
      limit: this.limit, offset: this.offset, columns: this.columns,
    };
  }

  async load() {
    const seq = (this.seq = (this.seq || 0) + 1);
    this.body.classList.add('loading');
    let out;
    try {
      out = await op('query_rows', this.query());
    } catch (e) {
      if (seq === this.seq) mount(this.body, h('div', { class: 'empty err' }, e.message));
      return;
    } finally {
      this.body.classList.remove('loading');
    }
    if (seq !== this.seq) return;
    this.rows = out.rows;
    this.total = out.total;
    this.draw();
    this.onLoaded?.(out);
  }

  draw() {
    const cols = this.columns.map((n) => this.meta.columns.find((c) => c.name === n)).filter(Boolean);
    if (!this.rows.length) {
      mount(this.body, empty(this.emptyText));
    } else {
      const head = h('tr', null, ...cols.map((c) => {
        const th = h('th', { class: this.sort === c.name ? 'sorted' : '', title: c.help || c.name, scope: 'col' },
          c.label || c.name, this.sort === c.name ? (this.desc ? ' ↓' : ' ↑') : '');
        th.addEventListener('click', () => {
          if (this.sort === c.name) this.desc = !this.desc; else { this.sort = c.name; this.desc = false; }
          this.offset = 0;
          this.load();
        });
        return th;
      }));
      const body = h('tbody');
      for (const r of this.rows) {
        const tr = h('tr', { tabindex: '0' }, ...cols.map((c) => h('td', {
          class: (isNumeric(c) ? 'num' : '') + (c.format === 'hex32' ? ' mono' : ''),
        }, cell(this.meta, c, r[c.name]))));
        tr.addEventListener('click', () => this.onOpen?.(r));
        tr.addEventListener('keydown', (e) => { if (e.key === 'Enter') this.onOpen?.(r); });
        body.appendChild(tr);
      }
      mount(this.body, h('table', { class: 'grid' }, h('thead', null, head), body));
    }
    const from = this.total ? this.offset + 1 : 0;
    const to = Math.min(this.offset + this.limit, this.total);
    const prev = button('Previous', { class: 'small', disabled: this.offset === 0, onclick: () => { this.offset = Math.max(0, this.offset - this.limit); this.load(); } });
    const next = button('Next', { class: 'small', disabled: to >= this.total, onclick: () => { this.offset += this.limit; this.load(); } });
    const size = h('select', { 'aria-label': 'Rows a page' }, ...[25, 50, 100, 250, 500].map((n) => h('option', { value: n, selected: n === this.limit }, n + ' a page')));
    size.addEventListener('change', () => { this.limit = Number(size.value); this.offset = 0; this.load(); });
    mount(this.foot, h('span', null, `${from.toLocaleString()}–${to.toLocaleString()} of ${this.total.toLocaleString()}`), h('span', { class: 'grow' }), size, prev, next);
  }

  drawFilters() {
    this.filterBar.classList.toggle('hidden', this.filters.length === 0);
    mount(this.filterBar, ...this.filters.map((f, i) => {
      const col = this.meta.columns.find((c) => c.name === f.column);
      const opLabel = (OPS.find((o) => o[0] === (f.op || '=')) || [, f.op])[1];
      const v = f.op === 'null' || f.op === 'notnull' ? '' : ' ' + (Array.isArray(f.value) ? f.value.join(', ') : f.value);
      return h('span', { class: 'chip' }, `${col?.label || f.column} ${opLabel}${v}`,
        h('button', { type: 'button', 'aria-label': 'Remove filter', onclick: () => { this.filters.splice(i, 1); this.drawFilters(); this.offset = 0; this.load(); } }, '×'));
    }));
  }

  setFilters(filters) {
    this.filters = filters.slice();
    this.offset = 0;
    this.drawFilters();
    return this.load();
  }

  async addFilter() {
    const cols = this.meta.columns.map((c) => ({ value: c.name, label: c.label ? `${c.label} (${c.name})` : c.name }));
    const v = await formBox({
      title: 'Filter ' + this.meta.label,
      fields: [
        { name: 'column', label: 'Column', type: 'select', options: cols, value: this.columns[1] || cols[0].value },
        { name: 'op', label: 'Condition', type: 'select', options: OPS.map(([value, label]) => ({ value, label })), value: '=' },
        { name: 'value', label: 'Value', hint: 'hex as 0x1A2B; for an enumeration its number' },
      ],
      submitLabel: 'Add filter',
    });
    if (!v) return;
    this.filters.push({ column: v.column, op: v.op, value: v.value });
    this.drawFilters();
    this.offset = 0;
    this.load();
  }

  async chooseColumns() {
    const boxes = this.meta.columns.map((c) => ({ c, box: h('input', { type: 'checkbox', checked: this.columns.includes(c.name) }) }));
    const { dialog } = await import('./dom.js');
    dialog({
      title: 'Columns of ' + this.meta.label,
      body: h('div', { class: 'flagset' }, ...boxes.map(({ c, box }) => h('label', null, box, c.label || c.name))),
      actions: [
        { label: 'Defaults', kind: 'ghost', onClick: () => { store('cols.' + this.table, ''); this.columns = (this.meta.show || this.meta.columns.slice(0, 9).map((c) => c.name)).slice(); this.load(); } },
        {
          label: 'Show', kind: 'primary', onClick: () => {
            const chosen = boxes.filter((b) => b.box.checked).map((b) => b.c.name);
            if (!chosen.length) return true;
            this.columns = chosen;
            store('cols.' + this.table, chosen.join(','));
            this.load();
          },
        },
      ],
    });
  }
}

export { icon };
