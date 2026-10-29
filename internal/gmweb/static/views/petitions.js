// Petitions: the players' requests to customer service, filed from Support in
// the game (petition_list): the open ones by default (Waiting on a GM: those
// filed or replied to since the last answer), newest first. Each is a card:
// the ticket number, the character (in the world or not), its category, when
// filed and changed, and its whole text as the player reads it (the request,
// their replies, the answers; player text shown as text). No message of the
// game answers a petition: Answer (petition_answer) writes the text below into
// it, signed with the name given, and marks it unread for the player, who
// reads it in Support's inbox after getting new headers (told in chat when in
// the world). Close (petition_close) once it is dealt with: the player is then
// told it has been addressed; it is refused while an answer is unread, with
// Close anyway. Both are in the Audit log, revertible.

import { h, mount, pageHead, button, toast, card, empty, confirmBox } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { when, ago } from '../lib/fmt.js';

const FILTERS = [
  ['open', 'Open'],
  ['waiting', 'Waiting on a GM'],
  ['closed', 'Closed'],
  ['deleted', 'Deleted by the player'],
  ['all', 'All'],
];

export async function render(box) {
  const status = h('select', { 'aria-label': 'Which petitions' },
    ...FILTERS.map(([v, label]) => h('option', { value: v }, label)));
  const search = h('input', { type: 'search', placeholder: 'Name or text', 'aria-label': 'Search' });
  const countLine = h('span', { class: 'faint small' });
  const listBox = h('div', { class: 'stack' });
  let page = 0;
  const per = 50;
  const pager = h('div', { class: 'row' });

  async function act(name, args, done) {
    try {
      const out = await op(name, args);
      toast(done(out), 'ok', out.live || '');
      load();
      return true;
    } catch (e) {
      toast(e.message, 'error');
      return false;
    }
  }

  function petitionCard(p) {
    const open = p.status === 'open';
    const answer = h('textarea', { rows: '4', maxlength: 4000, 'aria-label': 'Your answer', placeholder: 'Your answer, as the player will read it' });
    const signed = h('input', { type: 'text', maxlength: 32, value: 'GM', 'aria-label': 'Signed', style: 'max-width: 12em' });
    const badges = [];
    if (p.waiting) badges.push(h('span', { class: 'badge warn' }, 'waiting'));
    if (p.answerUnread) badges.push(h('span', { class: 'badge' }, 'answer unread'));
    if (!open) badges.push(h('span', { class: 'badge' }, p.status));
    const cat = [p.categoryName || `category ${p.category}`, p.subcategoryName || (p.subcategory ? `subcategory ${p.subcategory}` : '')]
      .filter(Boolean).join(' / ');
    const meta = h('div', { class: 'row small' },
      h('a', { href: '#/characters/' + p.serverid }, p.character), h('span', { class: 'faint' }, p.online ? 'in the world' : 'not in the world'),
      h('span', { class: 'faint' }, '·'), h('span', null, cat),
      h('span', { class: 'grow' }),
      h('span', { class: 'faint', title: when(p.filed) }, 'filed ', ago(p.filed)),
      p.updated && p.updated !== p.filed ? h('span', { class: 'faint', title: when(p.updated) }, ' · changed ', ago(p.updated)) : null);
    const history = [];
    if (p.answered) history.push(`answered ${when(p.answered)} by ${p.answered_by}`);
    if (p.replied) history.push(`the player replied ${when(p.replied)}`);
    if (p.closed) history.push(`closed ${when(p.closed)} by ${p.closed_by}`);
    const close = async () => {
      if (!p.answerUnread) {
        act('petition_close', { id: p.id }, (out) => out.message);
        return;
      }
      if (!(await confirmBox({ title: `Close ticket #${p.id} anyway?`,
        message: `${p.character} has not read the answer yet. Closed, they are told it has been addressed and cannot open it to read the answer.`,
        confirmLabel: 'Close anyway', danger: true }))) return;
      act('petition_close', { id: p.id, force: true }, (out) => out.message);
    };
    return card(`Ticket #${p.id}`, h('div', { class: 'stack' },
      meta,
      h('pre', { class: 'json' }, p.text),
      history.length ? h('div', { class: 'faint small' }, history.join(' · ')) : null,
      open ? h('div', { class: 'stack' },
        answer,
        h('div', { class: 'row' },
          h('label', { class: 'row small' }, 'Signed', signed),
          button('Answer', {
            class: 'small primary', title: "Write it into the ticket and mark it unread for the player",
            onclick: async () => {
              if (!answer.value.trim()) { toast('Write the answer first', 'error'); return; }
              if (await act('petition_answer', { id: p.id, text: answer.value, signed: signed.value }, (out) => out.message)) answer.value = '';
            },
          }),
          h('span', { class: 'grow' }),
          button('Close', { class: 'small ghost', title: 'Dealt with: the player is told it has been addressed', onclick: close }))) : null),
      ...badges);
  }

  async function load() {
    const args = { status: status.value, limit: per, offset: page * per, full: true };
    if (search.value.trim()) args.search = search.value.trim();
    mount(listBox, h('p', { class: 'faint' }, 'Loading…'));
    try {
      const out = await op('petition_list', args);
      const c = out.counts || {};
      countLine.textContent = `${out.matching} shown by the filter · ${c.waiting || 0} waiting, ${c.open || 0} open, ${c.closed || 0} closed, ${c.deleted || 0} deleted in all`;
      mount(listBox, out.petitions.length ? out.petitions.map(petitionCard)
        : empty(status.value === 'open' || status.value === 'waiting'
          ? 'No petition is open. Players file them from Support in the game.' : 'None.'));
      mount(pager,
        page > 0 ? button('Newer', { class: 'small ghost', onclick: () => { page--; load(); } }) : null,
        (page + 1) * per < out.matching ? button('Older', { class: 'small ghost', onclick: () => { page++; load(); } }) : null);
    } catch (e) {
      mount(listBox, h('p', { class: 'err' }, e.message));
    }
  }
  status.addEventListener('change', () => { page = 0; load(); });
  search.addEventListener('change', () => { page = 0; load(); });

  mount(box,
    pageHead('Petitions', "Players' requests to customer service, to answer and close."),
    card('Show', h('div', { class: 'stack' },
      h('div', { class: 'row' }, status, search),
      countLine,
      h('p', { class: 'muted small' }, "No message of the game answers a petition: Answer writes yours into the ticket and marks it unread, and the player reads it in Support's inbox after getting new headers (told in chat when in the world, else at the next login). Close it once they have read it: from then they are told it has been addressed, and it leaves their inbox in a week. Both are in the Audit log, revertible. What players write is shown as text."))),
    listBox,
    pager);
  load();
}
