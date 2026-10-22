// Showing values as the catalog says to: enumerations by name, bit sets as
// their flags, asset ids in hex, times as times.

import { h } from './dom.js';
import { lookupName } from './meta.js';

export function hex32(v) {
  if (v === null || v === undefined || v === '') return '';
  let n = Number(v);
  if (!Number.isFinite(n)) return String(v);
  n = ((n % 4294967296) + 4294967296) % 4294967296;
  return '0x' + n.toString(16).toUpperCase().padStart(8, '0');
}

export function num(v) {
  if (v === null || v === undefined) return '';
  const n = Number(v);
  if (!Number.isFinite(n)) return String(v);
  return Number.isInteger(n) ? n.toLocaleString() : n.toFixed(2);
}

export function coord(v) {
  const n = Number(v);
  return Number.isFinite(n) ? n.toFixed(1) : '';
}

export function enumLabel(meta, col, v) {
  const list = meta.enums?.[col.enum];
  const e = list?.find((x) => String(x.value) === String(v));
  return e ? e.label : v;
}

export function flagLabels(meta, col, v) {
  const bits = meta.flags?.[col.flags] || [];
  const n = Number(v) || 0;
  const on = bits.filter((b) => (n & b.bit) === b.bit).map((b) => b.label);
  const known = bits.reduce((a, b) => a | b.bit, 0);
  const rest = n & ~known;
  if (rest) on.push('0x' + rest.toString(16));
  return on;
}

export function ago(stamp) {
  if (!stamp) return '';
  const t = Date.parse(String(stamp).includes('T') ? stamp : String(stamp).replace(' ', 'T') + 'Z');
  if (!Number.isFinite(t)) return String(stamp);
  const s = Math.round((Date.now() - t) / 1000);
  if (s < 60) return s <= 5 ? 'just now' : s + ' s ago';
  if (s < 3600) return Math.round(s / 60) + ' min ago';
  if (s < 86400) return Math.round(s / 3600) + ' h ago';
  return Math.round(s / 86400) + ' d ago';
}

export function when(stamp) {
  if (!stamp) return '';
  const t = Date.parse(String(stamp).includes('T') ? stamp : String(stamp).replace(' ', 'T') + 'Z');
  if (!Number.isFinite(t)) return String(stamp);
  return new Date(t).toLocaleString();
}

export function bytes(n) {
  if (!Number.isFinite(n)) return '';
  if (n < 1024) return n + ' B';
  if (n < 1048576) return (n / 1024).toFixed(0) + ' KB';
  if (n < 1073741824) return (n / 1048576).toFixed(1) + ' MB';
  return (n / 1073741824).toFixed(2) + ' GB';
}

export function duration(sec) {
  sec = Math.round(sec);
  const d = Math.floor(sec / 86400), hh = Math.floor((sec % 86400) / 3600), m = Math.floor((sec % 3600) / 60);
  if (d) return `${d}d ${hh}h`;
  if (hh) return `${hh}h ${m}m`;
  return `${m}m ${sec % 60}s`;
}

// cell is a grid cell's content for a value of a column.
export function cell(meta, col, v) {
  if (v === null || v === undefined) return h('span', { class: 'faint' }, '—');
  if (col.enum) return String(enumLabel(meta, col, v));
  if (col.flags) {
    const labels = flagLabels(meta, col, v);
    return labels.length ? labels.join(', ') : h('span', { class: 'faint' }, '0');
  }
  if (col.format === 'hex32') return h('span', { class: 'mono' }, hex32(v));
  if (col.format === 'stamp') return h('span', { title: when(v) }, ago(v));
  if (col.ref) {
    const span = h('span', null, String(v), ' ', h('span', { class: 'ref' }, ''));
    lookupName(col.ref, v).then((name) => { if (name) span.lastChild.textContent = '· ' + name; });
    return span;
  }
  if (typeof v === 'number') return Number.isInteger(v) ? String(v) : coord(v);
  return String(v).length > 120 ? String(v).slice(0, 120) + '…' : String(v);
}

export function isNumeric(col) {
  return (col.affinity === 'INTEGER' || col.affinity === 'REAL') && !col.enum && !col.flags && col.format !== 'hex32' && !col.ref;
}
