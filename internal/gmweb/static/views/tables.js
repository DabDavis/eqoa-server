import { h, mount, pageHead } from '../lib/dom.js';
import { tables } from '../lib/meta.js';
import { listPage, recordPage } from './tablepage.js';

export async function render(box, params) {
  const [name, keySeg] = params;
  if (name && keySeg !== undefined) {
    return recordPage(box, {
      table: name, keySeg, base: 'tables/' + name,
      crumbs: [h('a', { href: '#/tables' }, 'All tables'), ' / ', h('a', { href: '#/tables/' + name }, name)],
    });
  }
  if (name) {
    return listPage(box, { table: name, base: 'tables/' + name, crumbs: h('a', { href: '#/tables' }, 'All tables') });
  }
  const list = await tables(true);
  const groups = new Map();
  for (const t of list) {
    if (!groups.has(t.group)) groups.set(t.group, []);
    groups.get(t.group).push(t);
  }
  const who = { gm: 'GMs edit', admin: 'admins edit', none: 'read-only' };
  mount(box, pageHead('All tables', 'Every table of the database, editable as the catalog allows. The purpose-built pages (NPCs, Items...) are the same tables with more around them.'),
    ...[...groups].map(([g, ts]) => h('section', { class: 'card' },
      h('div', { class: 'card-head' }, h('h2', null, g)),
      h('div', { class: 'card-body tight' }, h('table', { class: 'plain' },
        h('tbody', null, ...ts.map((t) => h('tr', null,
          h('td', null, h('a', { href: '#/tables/' + t.name }, t.label), h('span', { class: 'faint small mono' }, '  ' + t.name),
            t.help ? h('div', { class: 'muted small' }, t.help) : null),
          h('td', { class: 'num nowrap' }, t.rows.toLocaleString() + ' rows'),
          h('td', null, h('span', { class: 'badge' + (t.write === 'none' ? '' : t.write === 'admin' ? ' warn' : ' ok') }, who[t.write] || t.write))))))))));
}
