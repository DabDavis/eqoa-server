import { h, mount, pageHead, button, toast, empty, confirmBox, debounce } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { ago, when } from '../lib/fmt.js';
import { showChange } from '../lib/form.js';

const ACTION_BADGE = { insert: 'ok', update: 'info', delete: 'danger', live: 'accent', settings: 'warn', sql: 'warn', account: 'warn', security: 'warn' };

function changedKeys(e) {
  const b = e.before || {}, a = e.after || {};
  return [...new Set([...Object.keys(b), ...Object.keys(a)])].filter((k) => JSON.stringify(b[k]) !== JSON.stringify(a[k]));
}

function rowLink(e) {
  if (!e.table || !e.key) return null;
  const vals = Object.values(e.key);
  const pages = { npcs: 'npcs', itempattern: 'items', spellpattern: 'spells', characters: 'characters', accountinfo: 'accounts' };
  const href = pages[e.table] && vals.length === 1 ? `#/${pages[e.table]}/${vals[0]}`
    : `#/tables/${e.table}/${encodeURIComponent(vals.length === 1 ? String(vals[0]) : JSON.stringify(e.key))}`;
  return h('a', { href }, `${e.table} ${vals.join(', ')}`);
}

// auditLine is one entry: who, what, when; open for the change and a revert.
export function auditLine(e, compact = false) {
  const keys = changedKeys(e);
  const what = e.detail || (e.action === 'update' ? 'changed ' + keys.join(', ') : e.action === 'insert' ? 'added' : e.action === 'delete' ? 'deleted' : e.action);
  const line = h('div', { class: 'line' },
    h('span', { class: 'badge ' + (ACTION_BADGE[e.action] || '') }, e.action),
    h('strong', null, e.username),
    e.via && e.via !== 'web' ? h('span', { class: 'badge info', title: 'through ' + e.via }, e.via) : null,
    rowLink(e),
    h('span', { class: 'grow muted' }, what.length > 160 ? what.slice(0, 160) + '…' : what),
    e.revertedBy ? h('span', { class: 'badge' }, 'reverted') : null,
    h('span', { class: 'faint small', title: when(e.at) }, ago(e.at)));
  const box = h('div', { class: 'audit-entry' }, line);
  if (compact) return box;
  const canRevert = !!e.table && ['insert', 'update', 'delete'].includes(e.action) && !e.revertedBy;
  line.appendChild(h('span', { class: 'actions' },
    e.before || e.after ? button('Show', { class: 'small ghost', onclick: () => showChange({ ...e, live: '' }, `Entry ${e.id}: ${e.action} by ${e.username}`) }) : null,
    canRevert ? button('Revert', { class: 'small', icon: 'undo', onclick: async () => {
      try {
        const dry = await op('revert_change', { id: e.id, dry_run: true });
        const ok = await confirmBox({
          title: `Revert entry ${e.id}?`,
          message: `${dry.action} on ${dry.table} ${Object.values(dry.key || {}).join(', ')}: the row is put back as it was before ${e.username}'s change.`,
          confirmLabel: 'Revert',
        });
        if (!ok) return;
        const out = await op('revert_change', { id: e.id });
        toast('Reverted', 'ok', out.live || '');
        box.querySelector('.actions')?.remove();
        line.appendChild(h('span', { class: 'badge' }, 'reverted'));
      } catch (err) {
        if (err.status === 409 && await confirmBox({ title: 'The row changed since', message: err.message + ' Revert anyway, over the newer change?', confirmLabel: 'Revert anyway', danger: true })) {
          try { const out = await op('revert_change', { id: e.id, force: true }); toast('Reverted', 'ok', out.live || ''); } catch (e2) { toast(e2.message, 'error'); }
        } else if (err.status !== 409) toast(err.message, 'error');
      }
    } }) : null));
  return box;
}

export async function render(box) {
  const list = h('div', { class: 'card' });
  const user = h('input', { type: 'search', placeholder: 'GM' });
  const table = h('input', { type: 'search', placeholder: 'table' });
  const action = h('select', null, ...['', 'insert', 'update', 'delete', 'live', 'account', 'settings', 'sql', 'token', 'security', 'login', 'backup', 'server']
    .map((a) => h('option', { value: a }, a || 'any action')));
  const text = h('input', { type: 'search', placeholder: 'text in the entry' });
  let offset = 0;
  const more = button('Older', { class: 'small', onclick: () => { offset += 50; load(); } });
  const newer = button('Newer', { class: 'small', onclick: () => { offset = Math.max(0, offset - 50); load(); } });
  const count = h('span', { class: 'muted small' });
  const reload = debounce(() => { offset = 0; load(); }, 250);
  [user, table, text].forEach((el) => el.addEventListener('input', reload));
  action.addEventListener('change', reload);
  mount(box, pageHead('Audit log', 'Every change made through the panel or by an AI agent, who made it and through what. A row change can be reverted.'),
    h('div', { class: 'card' }, h('div', { class: 'grid-tools' }, user, table, action, text, h('span', { class: 'grow' }), count, newer, more)), list);
  async function load() {
    try {
      const out = await op('audit_log', { username: user.value.trim(), table: table.value.trim(), action: action.value, search: text.value.trim(), limit: 50, offset });
      count.textContent = out.total ? `${offset + 1}–${Math.min(offset + 50, out.total)} of ${out.total}` : '';
      more.disabled = offset + 50 >= out.total;
      newer.disabled = offset === 0;
      mount(list, out.entries.length ? h('div', null, ...out.entries.map((e) => auditLine(e))) : empty('No entries.'));
    } catch (e) { toast(e.message, 'error'); }
  }
  await load();
}
