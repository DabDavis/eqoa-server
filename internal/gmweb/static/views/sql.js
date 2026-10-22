import { h, mount, pageHead, button, toast, confirmBox, card, store } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { isAdmin } from '../lib/nav.js';

export async function render(box) {
  if (!isAdmin()) {
    mount(box, pageHead('SQL', 'Raw SQL is an admin\'s.'));
    return;
  }
  const text = h('textarea', { class: 'code', spellcheck: 'false', placeholder: 'SELECT npc_name, COUNT(*) FROM npcs GROUP BY npc_name ORDER BY 2 DESC LIMIT 20' });
  text.value = store('sql') || '';
  const out = h('div');
  const run = button('Run (read-only)', { class: 'primary', icon: 'terminal', onclick: async () => {
    store('sql', text.value);
    try {
      const r = await op('sql_query', { sql: text.value, limit: 1000 });
      mount(out, card(`${r.rows.length}${r.more ? '+' : ''} rows · ${r.ms} ms`, h('div', { class: 'grid-scroll' }, h('table', { class: 'grid' },
        h('thead', null, h('tr', null, ...r.columns.map((c) => h('th', null, c)))),
        h('tbody', null, ...r.rows.map((row) => h('tr', null, ...row.map((v) => h('td', null, v === null ? '—' : String(v))))))))));
    } catch (e) { mount(out, h('div', { class: 'notice danger' }, e.message)); }
  } });
  const exec = button('Execute changes…', { class: 'danger', onclick: async () => {
    if (!(await confirmBox({
      title: 'Run statements that change the database?', danger: true, confirmLabel: 'Back up and run',
      message: 'The whole database is backed up first. The audit log keeps the statements but not the rows they changed, so undoing this is restoring that backup. NPCs changed this way reach the world with "Reload every NPC".',
      typeToConfirm: 'change',
    }))) return;
    store('sql', text.value);
    try {
      const r = await op('sql_execute', { sql: text.value });
      mount(out, h('div', { class: 'notice ok' }, `${r.rowsAffected} rows changed. Backup: ${r.backup}. ${r.note}`));
    } catch (e) { mount(out, h('div', { class: 'notice danger' }, e.message)); }
  } });
  text.addEventListener('keydown', (e) => { if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') run.click(); });
  mount(box, pageHead('SQL', 'Questions the other pages do not answer. A query runs read-only, at most 5 s and 1,000 rows; Ctrl+Enter runs it.'),
    h('div', { class: 'card' }, h('div', { class: 'card-body stack' }, text, h('div', { class: 'row' }, run, h('span', { class: 'grow' }), exec))), out);
}
