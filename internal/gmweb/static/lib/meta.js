// What the tables are and what their columns mean (describe_table), and the
// names of the rows a column refers to (lookup), kept for the page's life.

import { op } from './api.js';

const described = new Map();
let tableList = null;

export async function tables(refresh = false) {
  if (!tableList || refresh) tableList = (await op('list_tables')).tables;
  return tableList;
}

export async function describe(name) {
  if (!described.has(name)) described.set(name, op('describe_table', { table: name }));
  try {
    return await described.get(name);
  } catch (e) {
    described.delete(name);
    throw e;
  }
}

// names of rows by key, batched per table and cached
const names = new Map(); // table -> Map(key -> name)
const pending = new Map(); // table -> {keys:Set, waiters:[]}

export function lookupName(table, key) {
  const m = names.get(table);
  if (m && m.has(String(key))) return Promise.resolve(m.get(String(key)));
  return new Promise((resolve) => {
    let p = pending.get(table);
    if (!p) {
      p = { keys: new Set(), waiters: [] };
      pending.set(table, p);
      setTimeout(() => flush(table), 30);
    }
    p.keys.add(String(key));
    p.waiters.push(() => resolve(names.get(table)?.get(String(key)) ?? null));
  });
}

async function flush(table) {
  const p = pending.get(table);
  pending.delete(table);
  if (!p) return;
  const keys = [...p.keys].filter((k) => k !== '' && k !== 'null');
  let m = names.get(table);
  if (!m) { m = new Map(); names.set(table, m); }
  for (let i = 0; i < keys.length; i += 500) {
    try {
      const out = await op('lookup', { table, keys: keys.slice(i, i + 500).map((k) => (/^-?\d+$/.test(k) ? Number(k) : k)) });
      for (const k of keys.slice(i, i + 500)) m.set(k, out.names[k] ?? null);
    } catch {
      for (const k of keys.slice(i, i + 500)) m.set(k, null);
    }
  }
  p.waiters.forEach((w) => w());
}

export function forgetNames(table) { names.delete(table); }

// the key of a row as the API carries it
export function keyOf(row) { return row.__key; }

export function keyText(key) {
  return Object.entries(key).map(([k, v]) => (Object.keys(key).length > 1 ? `${k}=${v}` : String(v))).join(', ');
}
