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
// Close anyway. Both are in the Audit log, revertible. Server's view
// (petition_get's server_view) is what the server held when it was filed:
// the character, its place and target with #stats' lines, what it was shown
// around it and in the last minutes, its quests, the client and its note,
// and a GM's marks (/mark), each a link to its place on the World map. Camp
// report from this petition (campfix_from_petition) files a spawn request it
// makes ("a guard spawns here", "this camp is not populated") as a waiting
// camp report at the character's place and facing when it filed: nothing
// spawns until a GM approves it on the Camp reports page.

import { h, mount, pageHead, button, toast, card, empty, confirmBox, formBox } from '../lib/dom.js';
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

  // The server's view of the moment a petition was filed, as GMs read it:
  // every value shown as text (the lines and the note are players' words).
  function viewBox(v) {
    if (!v) return h('p', { class: 'faint small' }, 'No server view: filed before the server kept one.');
    const c = v.character || {};
    const kv = (label, val) => (val === undefined || val === null || val === '' ? null
      : h('div', { class: 'row small' }, h('span', { class: 'faint', style: 'min-width: 8em' }, label), h('span', null, String(val))));
    const place = (e) => `world ${e.world ?? ''} (${Math.round(e.x)}, ${Math.round(e.y)}, ${Math.round(e.z)})`;
    const lines = (title, list) => (list && list.length
      ? h('details', null, h('summary', { class: 'small' }, `${title} (${list.length})`), h('pre', { class: 'json' }, list.join('\n'))) : null);
    const t = v.target;
    const parts = [
      kv('Filed', `${when(v.filed)} via ${v.via === 'chat' ? '/bug' : v.via === 'crash report' ? "the client's crash report (sent at its next login)" : "Support's form"}`),
      v.stopped ? kv('Client stopped', `last heard ${when(v.stopped.heard)}, ${v.stopped.how} (the server's record)`) : null,
      v.crash ? h('details', { open: true }, h('summary', { class: 'small' }, "The client's crash report"), h('pre', { class: 'json' }, v.crash)) : null,
      kv('Server', `build ${v.server?.build}, up ${v.server?.up}, ${v.server?.players} in the world`),
      kv('Client', `${v.client?.disc}${v.client?.f3 ? ', vanilla with F3' : ''}${v.client?.mailForms ? ', live mail forms' : ''}`),
      kv('Character', `${c.name}, level ${c.level} ${c.race} ${c.class}${c.dead ? ', dead' : ''}`),
      kv('Place', `${place(c)}, facing ${Number(c.facing || 0).toFixed(2)}`),
      kv('Health', `HP ${c.hp}/${c.maxHP}, power ${c.power}/${c.maxPower}, ${c.effects} effects`),
      kv('Fighting', c.fighting),
      kv('Tunar', `${c.tunar} on them, ${c.bankTunar} in the bank`),
      t ? kv('Target', `${t.name} (${t.kind}${t.npcid ? ' ' + t.npcid : ''}${t.serverid ? ' char ' + t.serverid : ''}), level ${t.level}, HP ${t.hp}/${t.maxHP}, ${t.distance} away${t.dead ? ', dead' : ''}${t.fighting ? ', fighting ' + t.fighting : ''}${t.spawn ? `, spawn (${t.spawn.map(Math.round).join(', ')})` : ''}`) : kv('Target', 'none'),
      lines("The character's #stats", v.stats),
      t ? lines("The target's #stats", t.stats) : null,
      v.marks?.length ? h('details', { open: true }, h('summary', { class: 'small' }, `The filer's marks, /mark (${v.marks.length})`),
        h('div', { class: 'stack' }, ...v.marks.map((m) => h('div', { class: 'row small' },
          h('a', { href: `#/map/${m.world}/@${Math.round(m.x)},${Math.round(m.z)}`, title: 'On the World map' }, `Mark ${m.n}`),
          h('span', { class: 'mono' }, `world ${m.world} (${Number(m.x).toFixed(1)}, ${Number(m.y).toFixed(1)}, ${Number(m.z).toFixed(1)}), facing ${Number(m.facing || 0).toFixed(2)}`),
          m.note ? h('span', null, m.note) : null)))) : null,
      v.quests?.length ? h('details', null, h('summary', { class: 'small' }, `Quests under way (${v.quests.length})`),
        h('pre', { class: 'json' }, v.quests.map((q) => `${q.name} (${q.questid}): step ${q.step} of ${q.steps}${q.kind ? ' ' + q.kind : ''}${q.need ? ` ${q.count}/${q.need}` : ''}${q.journal ? ' - ' + q.journal : ''}`).join('\n'))) : null,
      v.shown?.length ? h('details', null, h('summary', { class: 'small' }, `Shown around them (${v.shown.length})`),
        h('pre', { class: 'json' }, v.shown.map((e) => `${e.distance}\t${e.name} (${e.kind}${e.npcid ? ' ' + e.npcid : ''}), level ${e.level}, HP ${e.hp}/${e.maxHP}${e.dead ? ', dead' : ''}${e.fighting ? ', fighting ' + e.fighting : ''}`).join('\n'))) : null,
      v.lines?.length ? h('details', { open: true }, h('summary', { class: 'small' }, `What they were shown, last minutes (${v.lines.length})`),
        h('pre', { class: 'json' }, v.lines.map((l) => `${l.ago.toFixed(1)}s\t${l.kind}\t${l.text}`).join('\n'))) : null,
      v.client?.note ? h('details', null, h('summary', { class: 'small' }, "The client's note"), h('pre', { class: 'json' }, v.client.note)) : null,
    ];
    return h('div', { class: 'stack' }, ...parts);
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
    const viewSlot = h('div');
    const viewBtn = button("Server's view", {
      class: 'small ghost', title: 'What the server held when it was filed',
      onclick: async () => {
        if (viewSlot.firstChild) { mount(viewSlot); return; }
        try {
          const out = await op('petition_get', { id: p.id });
          mount(viewSlot, viewBox(out.server_view));
        } catch (e) {
          toast(e.message, 'error');
        }
      },
    });
    const campSlot = h('div');
    const campBtn = button('Camp report from this petition', {
      class: 'small ghost', title: "File its spawn request as a camp report, at the character's place: a GM approves it on the Camp reports page",
      onclick: async () => {
        let target = '';
        try {
          const got = await op('petition_get', { id: p.id });
          const v = got.server_view;
          if (!v || !v.character) { toast('This petition has no server view: there is no place to mark', 'error'); return; }
          if (v.target && v.target.kind === 'npc') target = v.target.name || '';
        } catch (e) { toast(e.message, 'error'); return; }
        const f = await formBox({
          title: `Camp report from ticket #${p.id}`,
          intro: `The mark is where ${p.character} stood and faced when filing it. The report waits for a GM's approval: nothing spawns now.`,
          submitLabel: 'File it',
          fields: [
            { name: 'kind', label: 'What it asks', type: 'select', value: 'add', options: [
              { value: 'add', label: 'Add: more of a creature should stand there' },
              { value: 'move', label: 'Move: a spawn of it should stand there' },
              { value: 'remove', label: 'Remove: the spawn there (or its target) should go' },
              { value: 'turn', label: 'Turn: a spawn of it should face that way' }] },
            { name: 'name', label: 'The creature', value: target, placeholder: 'as /campfix takes a name', hint: 'any case; a leading a/an/the left off' },
            { name: 'count', label: 'How many (an add)', type: 'number', value: 1 },
            { name: 'note', label: 'Note', placeholder: 'why (kept after "From petition N:")' },
          ],
        });
        if (!f) return;
        try {
          const out = await op('campfix_from_petition', { petition: p.id, kind: f.kind, name: f.name,
            count: f.kind === 'add' ? Number(f.count || 1) : 0, note: f.note });
          toast(out.message, 'ok', out.live || '');
          mount(campSlot, h('div', { class: 'notice ok small' }, (out.said || []).join(' ') + ' ',
            h('a', { href: '#/campfix/' + out.report.id }, `Camp report #${out.report.id}`), ' waits on the Camp reports page.'));
        } catch (e) { toast(e.message, 'error'); }
      },
    });
    return card(`Ticket #${p.id}`, h('div', { class: 'stack' },
      meta,
      h('pre', { class: 'json' }, p.text),
      h('div', { class: 'row' }, viewBtn, campBtn),
      viewSlot,
      campSlot,
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
