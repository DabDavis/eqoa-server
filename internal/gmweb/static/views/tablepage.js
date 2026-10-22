// A table's list page and a row's page, shared by every view: the lists are
// DataGrids, the rows RecordForms, with whatever a view adds around them.

import { h, mount, pageHead, button, toast, card, empty, confirmBox } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { describe } from '../lib/meta.js';
import { DataGrid } from '../lib/grid.js';
import { RecordForm, showChange } from '../lib/form.js';
import { go, isAdmin } from '../lib/nav.js';
import { auditLine } from './audit.js';

export function canWrite(meta) {
  return meta.write === 'gm' || (meta.write === 'admin' && isAdmin());
}

// keyPath is a row's key as a route segment: the value for a one-column key,
// else its JSON.
export function keyPath(meta, key) {
  if (meta.key.length === 1) return encodeURIComponent(String(key[meta.key[0]]));
  return encodeURIComponent(JSON.stringify(key));
}

export function keyFromPath(meta, seg) {
  const text = decodeURIComponent(seg);
  if (meta.key.length === 1) {
    const k = meta.key[0];
    return { [k]: /^-?\d+$/.test(text) ? Number(text) : text };
  }
  return JSON.parse(text);
}

// listPage draws a table's rows; opening one goes to base/<key>.
export async function listPage(box, { table, title, sub, base, filters = [], columns, tools = [], allowNew = true, crumbs, onLoaded }) {
  const meta = await describe(table);
  const grid = new DataGrid({
    table, meta, columns, filters, onLoaded,
    onOpen: (row) => go(base + '/' + keyPath(meta, row.__key)),
    tools: [
      ...tools,
      allowNew && canWrite(meta) ? button('New', { icon: 'plus', class: 'small primary', onclick: () => go(base + '/new') }) : null,
    ],
  });
  mount(box, pageHead(title || meta.label, sub ?? meta.help, crumbs), grid.el);
  if (meta.live) box.insertBefore(h('p', { class: 'notice info small' }, h('strong', null, 'When an edit takes effect: '), meta.live), grid.el);
  await grid.load();
  return { grid, meta };
}

// recordPage draws one row: its form, the view's own panels beside it, and
// the row's history.
export async function recordPage(box, { table, keySeg, base, crumbs, title, sub, side, below, actions, onDeleted, newDefaults }) {
  const meta = await describe(table);
  if (keySeg === 'new') {
    const form = new RecordForm({
      meta, mode: 'insert', row: newDefaults || {}, canWrite: canWrite(meta),
      onSaved: (out) => go(base + '/' + keyPath(meta, out.key)),
    });
    mount(box, pageHead('New ' + (meta.label || table).replace(/s$/, ''), meta.help, crumbs), h('div', { class: 'card' }, h('div', { class: 'card-body' }, form.el)));
    return { meta };
  }
  const key = keyFromPath(meta, keySeg);
  let row;
  try {
    row = (await op('get_row', { table, key })).row;
  } catch (e) {
    mount(box, pageHead('Not found', e.message, crumbs));
    return { meta };
  }
  const form = new RecordForm({
    meta, row, canWrite: canWrite(meta),
    onSaved: () => refreshSide(),
    onDeleted: () => (onDeleted ? onDeleted() : go(base)),
  });
  const sideBox = h('div', { class: 'stack' });
  const belowBox = h('div', { class: 'stack' });
  const historyBox = h('div');
  const refreshSide = async () => {
    if (side) mount(sideBox, await side(form.row, meta, refreshSide));
    if (below) mount(belowBox, await below(form.row, meta, refreshSide));
    loadHistory();
  };
  const loadHistory = async () => {
    try {
      const out = await op('audit_log', { table, key, limit: 15 });
      mount(historyBox, out.entries.length ? h('div', null, ...out.entries.map((e) => auditLine(e))) : empty('No changes through the panel yet.'));
    } catch (e) { mount(historyBox, h('p', { class: 'err' }, e.message)); }
  };
  const acts = [
    ...(actions ? actions(row, meta, refreshSide) : []),
    canWrite(meta) && table !== 'accountinfo' && table !== 'characters' ? button('Copy', { icon: 'copy', onclick: async () => {
      try {
        const out = await op('clone_row', { table, key: row.__key, changes: {} });
        toast('Copied', 'ok', out.change.live || '');
        go(base + '/' + keyPath(meta, out.change.key));
      } catch (e) { toast(e.message, 'error'); }
    } }) : null,
    canWrite(meta) ? button('Delete', { icon: 'trash', class: 'danger', onclick: () => form.remove() }) : null,
  ].filter(Boolean);
  const t = typeof title === 'function' ? title(row, meta) : title || (meta.title ? String(row[meta.title] ?? '') : '') || meta.label + ' ' + Object.values(key).join(', ');
  const s = typeof sub === 'function' ? sub(row, meta) : sub;
  mount(box,
    pageHead(t, s, crumbs, ...acts),
    h('div', { class: side ? 'cols-2' : '' },
      h('div', { class: 'stack' },
        meta.live ? h('p', { class: 'notice info small' }, h('strong', null, 'Takes effect: '), meta.live) : null,
        h('section', { class: 'card' }, h('div', { class: 'card-body' }, form.el)),
        belowBox),
      side ? sideBox : null),
    card('History of this row', historyBox));
  await refreshSide();
  return { meta, form, row };
}

// tablePanel is a small grid of related rows inside another page.
export async function relatedGrid({ table, filters, columns, base, emptyText, tools = [], pageSize = 25 }) {
  const meta = await describe(table);
  const grid = new DataGrid({
    table, meta, filters, columns, pageSize, emptyText, tools,
    onOpen: (row) => go((base || 'tables/' + table) + '/' + keyPath(meta, row.__key)),
  });
  await grid.load();
  return grid;
}

export { showChange, confirmBox };
