// Password resets: requests from the account pages' Forgot password form
// (pwreset_list), waiting on a GM. Each is a card: the account as typed, the
// contact and note the player gave (as text), and the account's characters
// (name, level, class, race, world) and last login, to check the contact's
// claim against. Approve (pwreset_approve) sets a temporary password and shows
// it ONCE in a dialog, with a line to pass on: it is kept nowhere. Decline
// (pwreset_decline) sets nothing. "Reset a password" (pwreset_start) is a
// GM's reset without the form, from a Discord ask. #/pwresets/<id> shows one.

import { h, mount, pageHead, button, toast, card, empty, confirmBox, dialog, copyButton } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { when, ago } from '../lib/fmt.js';

const FILTERS = [
  ['waiting', 'Waiting on a GM'],
  ['done', 'Done (a temporary password set)'],
  ['declined', 'Declined'],
  ['all', 'All'],
];

// showPassword is the one place a temporary password is ever shown.
function showPassword(out) {
  const pw = h('code', { class: 'mono', style: 'font-size:1.3em;user-select:all', text: out.password });
  const tell = h('textarea', { rows: '4', readonly: true, style: 'width:100%' });
  tell.value = out.tell;
  dialog({
    title: 'Temporary password for ' + out.account,
    body: h('div', { class: 'stack' },
      h('p', { class: 'notice warn' }, 'Shown this once: it is kept nowhere. Pass it on now, in a direct message.'),
      h('div', { class: 'row' }, pw, copyButton(out.password, 'Copy password')),
      h('label', null, 'A line to send them', tell),
      h('div', { class: 'row' }, copyButton(out.tell, 'Copy the line')),
      h('p', { class: 'faint small' }, 'They log in with it (the game takes it too); the website asks them for their own first, and the game reminds them until they set it.')),
    actions: [{ label: 'I have passed it on', kind: 'primary' }],
  });
}

function accountBox(acc) {
  if (!acc || acc.missing) return h('p', { class: 'err small' }, 'The account it names is gone.');
  const chars = acc.characters || [];
  return h('div', { class: 'stack' },
    h('div', { class: 'row small' },
      h('a', { href: '#/accounts/' + acc.accountid }, h('strong', { text: acc.username })),
      acc.level > 0 ? h('span', { class: 'badge warn' }, 'GM level ' + acc.level) : null,
      acc.banned ? h('span', { class: 'badge danger' }, 'banned') : null,
      acc.temporaryPassword ? h('span', { class: 'badge info' }, 'temporary password now') : null,
      h('span', { class: 'faint' }, 'created ' + (acc.created || '?')),
      h('span', { class: 'faint' }, '· last login ' + (acc.lastLogin || 'never'))),
    chars.length ? h('div', { class: 'table-scroll' }, h('table', { class: 'grid' },
      h('thead', null, h('tr', null, ...['Character', 'Level', 'Class', 'Race', 'World'].map((t) => h('th', { scope: 'col' }, t)))),
      h('tbody', null, ...chars.map((c) => h('tr', null,
        h('td', null, h('a', { href: '#/characters/' + c.serverid, text: c.name })),
        h('td', { class: 'num' }, String(c.level)),
        h('td', null, c.class), h('td', null, c.race), h('td', null, c.worldName)))))) :
      h('p', { class: 'faint small' }, 'No characters.'));
}

export async function render(box, parts) {
  const one = parts && /^\d+$/.test(parts[0] || '') ? Number(parts[0]) : 0;
  const status = h('select', { 'aria-label': 'Which requests' }, ...FILTERS.map(([v, l]) => h('option', { value: v }, l)));
  const countLine = h('span', { class: 'faint small' });
  const listBox = h('div', { class: 'stack' });

  function requestCard(r) {
    const waiting = r.status === 'waiting';
    const note = h('textarea', { rows: '2', maxlength: 2000, 'aria-label': 'Your note',
      placeholder: 'Your note: how you confirmed it is theirs (kept in the request\'s review)' });
    const approve = async () => {
      if (!(await confirmBox({ title: `Reset ${r.account?.username || r.typed}'s password?`,
        message: 'A temporary password replaces theirs now, shown to you once to pass on. Only do this once the contact has shown it is their account.',
        confirmLabel: 'Set a temporary password', danger: true }))) return;
      try {
        const out = await op('pwreset_approve', { id: r.id, note: note.value });
        showPassword(out);
        load();
      } catch (e) { toast(e.message, 'error'); }
    };
    const decline = async () => {
      if (!(await confirmBox({ title: `Decline request #${r.id}?`, message: 'No password changes.', confirmLabel: 'Decline', danger: true }))) return;
      try {
        const out = await op('pwreset_decline', { id: r.id, note: note.value });
        toast(out.message, 'ok');
        load();
      } catch (e) { toast(e.message, 'error'); }
    };
    return card(`Request #${r.id}`, h('div', { class: 'stack' },
      h('div', { class: 'row small' },
        h('span', null, 'Account as typed: ', h('strong', { text: r.typed })),
        r.filedBy ? h('span', { class: 'badge info', text: 'started by ' + r.filedBy }) : null,
        h('span', { class: 'grow' }),
        r.board ? h('a', { href: '#/work/' + r.board, class: 'faint' }, 'board entry ' + r.board) : null,
        h('span', { class: 'faint', title: when(r.created) }, ' · sent ', ago(r.created))),
      h('div', null, 'Contact: ', h('strong', { text: r.contact })),
      r.note ? h('div', { class: 'small' }, 'Note: ', h('span', { text: '“' + r.note + '”' })) : h('div', { class: 'faint small' }, 'no note'),
      accountBox(r.account),
      waiting ? h('div', { class: 'stack' },
        h('p', { class: 'muted small' }, 'Reach them at the contact and have them show it is their account (a character\'s name, level and class from the list above, where they last were) before you approve.'),
        note,
        h('div', { class: 'row' },
          button('Approve: set a temporary password', { class: 'small primary', onclick: approve }),
          h('span', { class: 'grow' }),
          button('Decline', { class: 'small ghost', onclick: decline })))
        : h('div', { class: 'stack' },
          h('div', { class: 'faint small' }, `${r.status} ${when(r.decided)} by ${r.decidedBy}`),
          r.review ? h('pre', { class: 'json', text: r.review }) : null)),
    h('span', { class: 'badge' + (waiting ? ' warn' : '') }, r.status));
  }

  function startCard() {
    const account = h('input', { placeholder: 'Account name or id', 'aria-label': 'Account' });
    const contact = h('input', { placeholder: 'Who asked, and where (their Discord name)', 'aria-label': 'Contact', maxlength: 200 });
    const note = h('input', { placeholder: 'How you confirmed it is theirs', 'aria-label': 'Note' });
    const go = async () => {
      if (!account.value.trim() || !contact.value.trim()) { toast('Name the account and who asked', 'error'); return; }
      if (!(await confirmBox({ title: `Reset ${account.value.trim()}'s password?`,
        message: 'A temporary password replaces theirs now, shown to you once to pass on.', confirmLabel: 'Reset', danger: true }))) return;
      try {
        const out = await op('pwreset_start', { account: account.value.trim(), contact: contact.value, note: note.value });
        showPassword(out);
        account.value = contact.value = note.value = '';
        load();
      } catch (e) { toast(e.message, 'error'); }
    };
    return card('Reset a password', h('div', { class: 'stack' },
      h('p', { class: 'muted small' }, 'For a player who asked another way (Discord): once you are sure it is their account, this files a request in your name and sets a temporary password at once.'),
      h('div', { class: 'row' }, account, contact, note, button('Reset', { class: 'small', onclick: go }))));
  }

  async function load() {
    mount(listBox, h('p', { class: 'faint' }, 'Loading…'));
    try {
      if (one) {
        const out = await op('pwreset_get', { id: one });
        countLine.textContent = '';
        mount(listBox, requestCard(out.request));
        return;
      }
      const out = await op('pwreset_list', { status: status.value, limit: 20 });
      const c = out.counts || {};
      countLine.textContent = `${out.matching} shown by the filter${out.matching > out.requests.length ? `, the first ${out.requests.length} here` : ''} · ` +
        `${c.waiting || 0} waiting, ${c.done || 0} done, ${c.declined || 0} declined in all`;
      mount(listBox, out.requests.length ? out.requests.map(requestCard)
        : empty(status.value === 'waiting' ? 'No request waits. Players send them from the website\'s Forgot password form.' : 'None.'));
    } catch (e) {
      mount(listBox, h('p', { class: 'err' }, e.message));
    }
  }
  status.addEventListener('change', load);

  mount(box,
    pageHead('Password resets', 'Players who forgot their password. Confirm it is them, then set a temporary password they must change.'),
    one ? h('p', null, h('a', { href: '#/pwresets' }, 'All password resets')) : h('div', { class: 'stack' },
      startCard(),
      card('Show', h('div', { class: 'stack' }, h('div', { class: 'row' }, status), countLine))),
    listBox);
  load();
}
