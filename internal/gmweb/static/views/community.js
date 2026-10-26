// Community visuals: what players suggest each spell's visual is, sent from
// the public spell-visual page with no sign-in (internal/spellfx;
// eqoa-rust-client-vanilla's web/spellfx.html), compared here spell by spell:
// each suggested visual with how many sent it, the names and notes they gave
// (as text: what a player typed is data), a Preview in the Spell visuals
// page's pane (a human casting it off the GM's own disc), Accept (the spell's
// spelleffect.fxdict, set_spell_visual's audited row edit, revertible in the
// audit log) and Dismiss. A sender is the first 8 characters of a salted
// hash of its address; a spammer's rows go with Delete sender.

import { h, mount, pageHead, button, toast, card, empty, confirmBox } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { previewPane } from './visuals.js';

const CLASSES = ['Warrior', 'Ranger', 'Paladin', 'Shadow Knight', 'Monk', 'Bard', 'Rogue', 'Druid', 'Shaman', 'Cleric',
  'Magician', 'Necromancer', 'Enchanter', 'Wizard', 'Alchemist'];

export async function render(box) {
  mount(box, pageHead('Community visuals', 'Loading…'));
  const preview = previewPane();

  const status = h('select', { 'aria-label': 'Which suggestions' },
    h('option', { value: 'new' }, 'Waiting'),
    h('option', { value: 'accepted' }, 'Accepted'),
    h('option', { value: 'dismissed' }, 'Dismissed'),
    h('option', { value: 'all' }, 'All'));
  const cls = h('select', { 'aria-label': 'Class' },
    h('option', { value: '' }, 'Every class'), ...CLASSES.map((n, i) => h('option', { value: String(i) }, n)));
  // every spell by default: a suggestion may correct a visual already given
  const standIn = h('input', { type: 'checkbox' });
  const countLine = h('span', { class: 'faint small' });
  const listBox = h('div', { class: 'stack' });

  const playIt = (sp, visual) => {
    if (!preview) return;
    preview.play(visual, (sp.castSeconds || 0) * 1000, { harmful: sp.harmful });
  };

  async function decide(name, args, done) {
    try {
      const out = await op(name, args);
      toast(done(out), 'ok', out.change && out.change.live ? out.change.live : '');
      load();
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  function noteLine(n) {
    return h('div', { class: 'small' },
      h('span', null, '“', n.note, '”'),
      h('span', { class: 'faint' }, ' — ', n.name || 'no name', ' · sender ', h('span', { class: 'mono' }, n.sender)));
  }

  function senderButtons(senders) {
    return h('div', { class: 'row small' }, h('span', { class: 'faint' }, 'Senders:'),
      ...senders.map((s) => button(s, {
        class: 'small ghost', title: 'Delete every suggestion this sender made',
        onclick: async () => {
          if (!(await confirmBox({ title: 'Delete sender ' + s + '?', message: 'Every suggestion this sender made, for every spell, is deleted. This cannot be undone.', confirmLabel: 'Delete', danger: true }))) return;
          decide('clear_visual_suggestions', { sender: s }, (out) => `Deleted ${out.deleted} suggestion${out.deleted === 1 ? '' : 's'} of ${s}`);
        },
      })));
  }

  function visualRow(sp, v) {
    const waiting = (v.statuses && v.statuses.new) || 0;
    // the width through the style object: the CSP refuses a style attribute
    const fill = h('span');
    fill.style.width = Math.max(2, v.share) + '%';
    const bar = h('div', { class: 'share-bar', title: v.share + '%' }, fill);
    return h('tr', null,
      h('td', { class: 'mono' }, v.visual, v.current ? h('span', { class: 'badge info', title: 'what the spell plays now' }, ' now') : null),
      h('td', null, h('div', { class: 'stack' }, h('strong', null, `${v.count} (${v.share}%)`), bar)),
      h('td', null, h('div', { class: 'stack' },
        v.names.length ? h('div', { class: 'small' }, v.names.join(', ')) : h('div', { class: 'faint small' }, 'no names'),
        ...v.notes.map(noteLine),
        senderButtons(v.senders))),
      h('td', null, h('div', { class: 'row' },
        preview ? button('Preview', { class: 'small', onclick: () => playIt(sp, v.visual) }) : null,
        waiting ? button('Accept', {
          class: 'small primary', title: `Give ${sp.name} this visual`,
          onclick: async () => {
            if (!(await confirmBox({ title: `Give ${sp.name} visual ${v.visual}?`, message: `From its next cast. ${waiting} waiting suggestion${waiting === 1 ? '' : 's'} of it become accepted and the spell's others dismissed; the audit log can revert the visual.`, confirmLabel: 'Accept' }))) return;
            decide('accept_visual_suggestion', { spell: sp.spellid, visual: v.visual }, (out) => out.message);
          },
        }) : null,
        waiting ? button('Dismiss', {
          class: 'small ghost', title: 'Set these suggestions aside',
          onclick: () => decide('dismiss_visual_suggestion', { spell: sp.spellid, visual: v.visual }, (out) => `Dismissed ${out.dismissed}`),
        }) : null,
        !waiting ? h('span', { class: 'faint small' }, Object.entries(v.statuses || {}).map(([k, n]) => `${n} ${k}`).join(', ')) : null)));
  }

  function spellCard(sp) {
    const cur = sp.current || {};
    const now = cur.own ? `plays ${cur.own}` : `plays the ${cur.standIn} stand-in ${cur.plays}`;
    const classes = (sp.classes || []).map((c) => CLASSES[c] || c).join(', ');
    return card(`${sp.name} (Lv ${sp.level})`, h('div', { class: 'stack' },
      h('div', { class: 'row small' },
        h('span', { class: 'faint' }, classes || 'no class'),
        h('span', { class: 'faint' }, '·'), h('span', null, now),
        h('span', { class: 'faint' }, '·'), h('span', null, `${sp.total} suggestion${sp.total === 1 ? '' : 's'}`),
        h('span', { class: 'grow' }),
        preview ? button('Preview what it plays now', { class: 'small ghost', onclick: () => playIt(sp, cur.plays) }) : null,
        h('a', { href: '#/visuals/' + sp.spellid }, 'Choose another…')),
      h('div', { class: 'table-scroll' }, h('table', { class: 'grid' },
        h('thead', null, h('tr', null, ...['Visual', 'Sent by', 'Names and notes', ''].map((t) => h('th', { scope: 'col' }, t)))),
        h('tbody', null, ...sp.visuals.map((v) => visualRow(sp, v)))))));
  }

  async function load() {
    const args = { status: status.value };
    if (cls.value !== '') args.class = Number(cls.value);
    if (standIn.checked) args.standin_only = true;
    mount(listBox, h('p', { class: 'faint' }, 'Loading…'));
    try {
      const out = await op('visual_suggestions', args);
      countLine.textContent = `${out.matching} spell${out.matching === 1 ? '' : 's'} with suggestions${out.matching > out.spells.length ? `, the ${out.spells.length} newest shown` : ''} · ${out.waiting} waiting in all`;
      mount(listBox, out.spells.length ? out.spells.map(spellCard)
        : empty(status.value === 'new' ? 'No suggestions waiting. Players send them from the public spell-visual page.' : 'None.'));
    } catch (e) {
      mount(listBox, h('p', { class: 'err' }, e.message));
    }
  }
  for (const el of [status, cls, standIn]) el.addEventListener('change', load);

  const tools = card('Show', h('div', { class: 'stack' },
    h('div', { class: 'row' }, status, cls, h('label', { class: 'check' }, standIn, 'Only spells still on a stand-in')),
    countLine,
    h('p', { class: 'muted small' }, 'Players pick, on the public spell-visual page, which visual they think a spell plays; anyone may send one, no sign-in, one per spell per address. Preview plays a suggestion here; Accept gives the spell that visual from its next cast (revertible in the Audit log).')));
  const cards = [tools, listBox];
  mount(box,
    pageHead('Community visuals', 'What players suggest each spell\'s visual is, to compare and accept.'),
    preview ? h('div', { class: 'fx-layout' }, h('div', { class: 'fx-main' }, ...cards), h('div', { class: 'fx-side' }, preview.card)) : null,
    ...(preview ? [] : cards));
  load();
}
