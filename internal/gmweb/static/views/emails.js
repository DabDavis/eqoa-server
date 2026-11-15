// E-mail: the accounts' e-mail state (email_counts, email_list). An address
// is its owner's alone to see: it is shown masked (d***@g***.com). Resend
// (email_resend) mails the account its verify link again; Mark verified
// (email_mark_verified) is a GM's audited override for a player who cannot
// get the mail. Both are also on an account's own page (accounts.js).

import { h, mount, pageHead, button, toast, card, empty, formBox } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { ago } from '../lib/fmt.js';

const FILTERS = [
  ['unverified', 'Not verified (pending or none)'],
  ['pending', 'An address waiting for its link'],
  ['none', 'No address'],
  ['verified', 'Verified'],
  ['all', 'All'],
];

export function emailBadge(e) {
  if (!e) return null;
  if (e.state === 'verified') return h('span', { class: 'badge ok', title: `${e.verifiedAt} by ${e.verifiedBy}` }, e.verifiedBy && e.verifiedBy.startsWith('GM ') ? 'verified by a GM' : 'verified');
  if (e.state === 'pending') return h('span', { class: 'badge warn' }, 'not verified yet');
  return h('span', { class: 'badge danger' }, 'no e-mail');
}

export async function resendLink(e) {
  try {
    const out = await op('email_resend', { account: e.accountid });
    toast(out.message, 'ok');
    return true;
  } catch (err) { toast(err.message, 'error'); return false; }
}

export async function markVerified(e) {
  const v = await formBox({
    title: 'Mark ' + e.username + "'s e-mail verified",
    intro: 'Only for a player who has shown the account is theirs and cannot receive the mail. It is kept in the audit log with your note.',
    submitLabel: 'Mark verified',
    fields: [{ name: 'note', label: 'How you confirmed it, and why the mail cannot reach them', type: 'textarea' }],
  });
  if (!v) return false;
  try {
    const out = await op('email_mark_verified', { account: e.accountid, note: v.note });
    toast(out.message, 'ok');
    return true;
  } catch (err) { toast(err.message, 'error'); return false; }
}

export function emailActions(e, refresh) {
  if (!e) return [];
  return [
    e.state === 'pending' ? button('Send the link again', { class: 'small', onclick: async () => { if (await resendLink(e)) refresh(); } }) : null,
    e.state !== 'verified' ? button('Mark verified', { class: 'small ghost', onclick: async () => { if (await markVerified(e)) refresh(); } }) : null,
  ].filter(Boolean);
}

export async function render(box) {
  const filter = h('select', { 'aria-label': 'Which accounts' }, ...FILTERS.map(([v, l]) => h('option', { value: v }, l)));
  const countBox = h('div', { class: 'row small' });
  const listBox = h('div', { class: 'stack' });
  let offset = 0;

  async function load() {
    mount(listBox, h('p', { class: 'faint' }, 'Loading…'));
    try {
      const [out, cnt] = await Promise.all([op('email_list', { filter: filter.value, limit: 100, offset }), op('email_counts', {})]);
      const c = cnt.counts || {};
      const st = cnt.settings || {};
      mount(countBox,
        h('span', { class: 'badge ok' }, `${c.verified || 0} verified`),
        c.verifiedByGM ? h('span', { class: 'badge info' }, `${c.verifiedByGM} of them by a GM`) : null,
        h('span', { class: 'badge warn' }, `${c.pending || 0} waiting for their link`),
        h('span', { class: 'badge danger' }, `${c.noAddress || 0} with no address`),
        h('span', { class: 'faint' }, `of ${c.accounts || 0} accounts`),
        h('span', { class: 'grow' }),
        h('span', { class: 'faint' }, st.accounts ? (st.sending ? 'mail is sent' : 'mail is logged only') : 'e-mail is off on this server'),
        st.requireVerified ? h('span', { class: 'faint' }, st.requireAfter ? `· required from ${st.requireAfter}` : '· required (reminders only)') : null);
      if (!out.accounts.length) { mount(listBox, empty('None.')); return; }
      mount(listBox,
        h('div', { class: 'table-scroll' }, h('table', { class: 'grid' },
          h('thead', null, h('tr', null, ...['Account', 'E-mail', 'State', 'Last login', ''].map((t) => h('th', { scope: 'col' }, t)))),
          h('tbody', null, ...out.accounts.map((e) => h('tr', null,
            h('td', null, h('a', { href: '#/accounts/' + e.accountid, text: e.username }), e.level > 0 ? h('span', { class: 'badge warn' }, 'GM') : null),
            h('td', { class: 'mono' }, e.email || '—'),
            h('td', null, emailBadge(e)),
            h('td', { class: 'faint' }, ago(e.lastLogin) || 'never'),
            h('td', null, h('div', { class: 'row' }, ...emailActions(e, load)))))))),
        h('div', { class: 'row small' },
          offset > 0 ? button('Previous', { class: 'small ghost', onclick: () => { offset = Math.max(0, offset - 100); load(); } }) : null,
          h('span', { class: 'faint' }, `${offset + 1}–${offset + out.accounts.length} of ${out.matching}`),
          offset + out.accounts.length < out.matching ? button('Next', { class: 'small ghost', onclick: () => { offset += 100; load(); } }) : null));
    } catch (e) {
      mount(listBox, h('p', { class: 'err' }, e.message));
    }
  }
  filter.addEventListener('change', () => { offset = 0; load(); });
  mount(box,
    pageHead('E-mail', 'Every account needs a verified e-mail address. Addresses are shown masked: they are their owners\' alone.'),
    card('Accounts', h('div', { class: 'stack' }, countBox, h('div', { class: 'row' }, filter))),
    listBox);
  load();
}
