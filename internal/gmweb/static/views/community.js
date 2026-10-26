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

  // Many at once (the user: "add a checkbox for that screen also for accept/delete"): a tick on each
  // waiting suggestion, by spell and visual; what is ticked is accepted or removed in one go. The
  // ticks are only ever the rows on the page: a reload drops the rest.
  const picked = new Map(); // spellid:visual -> { sp, v }
  let shownRows = []; // the waiting rows on the page: { sp, v, box }
  const keyOf = (sp, v) => sp.spellid + ':' + v.visual;
  const selLine = h('span', { class: 'faint small' });
  const acceptSel = button('Accept selected', { class: 'small primary', onclick: () => bulk('accept') });
  const removeSel = button('Remove selected', { class: 'small', title: 'Take them out of the waiting queue (kept as dismissed)', onclick: () => bulk('dismiss') });
  const allSel = button('Select all shown', { class: 'small ghost', onclick: () => { for (const r of shownRows) picked.set(keyOf(r.sp, r.v), r); syncBoxes(); } });
  const clearSel = button('Clear', { class: 'small ghost', onclick: () => { picked.clear(); syncBoxes(); } });
  function drawSel() {
    const n = picked.size;
    selLine.textContent = n ? `${n} selected` : 'None selected';
    acceptSel.disabled = !n;
    removeSel.disabled = !n;
    clearSel.hidden = !n;
    allSel.hidden = !shownRows.length;
  }
  function syncBoxes() {
    for (const r of shownRows) r.box.checked = picked.has(keyOf(r.sp, r.v));
    drawSel();
  }
  async function bulk(kind) {
    const rows = [...picked.values()];
    if (!rows.length) return;
    if (kind === 'accept') {
      const per = new Map();
      for (const r of rows) per.set(r.sp.spellid, (per.get(r.sp.spellid) || 0) + 1);
      const twice = [...per.values()].filter((n) => n > 1).length;
      if (twice) {
        toast(`Pick one visual a spell to accept: ${twice} spell${twice === 1 ? ' has' : 's have'} two or more ticked.`, 'error');
        return;
      }
    }
    const verb = kind === 'accept' ? 'Accept' : 'Remove';
    const message = kind === 'accept'
      ? 'Each spell gets its ticked visual from its next cast (each revertible in the Audit log); its other waiting suggestions are dismissed, and its namesakes on a stand-in get the visual too.'
      : 'They leave the waiting queue, kept as dismissed (the Dismissed list shows them).';
    if (!(await confirmBox({ title: `${verb} ${rows.length} selected?`, message, confirmLabel: verb, danger: kind !== 'accept' }))) return;
    let ok = 0;
    const failed = [];
    for (const r of rows) {
      try {
        await op(kind === 'accept' ? 'accept_visual_suggestion' : 'dismiss_visual_suggestion', { spell: r.sp.spellid, visual: r.v.visual });
        ok++;
      } catch (e) {
        failed.push(`${r.sp.name}: ${e.message}`);
      }
    }
    picked.clear();
    toast(`${kind === 'accept' ? 'Accepted' : 'Removed'} ${ok} of ${rows.length}` + (failed.length ? ` · not done: ${failed.slice(0, 3).join('; ')}${failed.length > 3 ? '…' : ''}` : ''), failed.length ? 'error' : 'ok');
    load();
  }

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
    let box = null;
    if (waiting) {
      box = h('input', { type: 'checkbox', 'aria-label': `Select ${v.visual} for ${sp.name}` });
      const key = keyOf(sp, v);
      box.checked = picked.has(key);
      box.addEventListener('change', () => {
        if (box.checked) picked.set(key, { sp, v });
        else picked.delete(key);
        drawSel();
      });
      shownRows.push({ sp, v, box });
    }
    // the width through the style object: the CSP refuses a style attribute
    const fill = h('span');
    fill.style.width = Math.max(2, v.share) + '%';
    const bar = h('div', { class: 'share-bar', title: v.share + '%' }, fill);
    return h('tr', null,
      h('td', null, box),
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
        h('thead', null, h('tr', null, ...['', 'Visual', 'Sent by', 'Names and notes', ''].map((t) => h('th', { scope: 'col' }, t)))),
        h('tbody', null, ...sp.visuals.map((v) => visualRow(sp, v)))))));
  }

  async function load() {
    const args = { status: status.value };
    if (cls.value !== '') args.class = Number(cls.value);
    if (standIn.checked) args.standin_only = true;
    mount(listBox, h('p', { class: 'faint' }, 'Loading…'));
    shownRows = [];
    try {
      const out = await op('visual_suggestions', args);
      countLine.textContent = `${out.matching} spell${out.matching === 1 ? '' : 's'} with suggestions${out.matching > out.spells.length ? `, the ${out.spells.length} newest shown` : ''} · ${out.waiting} waiting in all`;
      mount(listBox, out.spells.length ? out.spells.map(spellCard)
        : empty(status.value === 'new' ? 'No suggestions waiting. Players send them from the public spell-visual page.' : 'None.'));
    } catch (e) {
      mount(listBox, h('p', { class: 'err' }, e.message));
    }
    // ticks only on rows the page shows now
    const onPage = new Set(shownRows.map((r) => keyOf(r.sp, r.v)));
    for (const k of [...picked.keys()]) if (!onPage.has(k)) picked.delete(k);
    for (const r of shownRows) if (picked.has(keyOf(r.sp, r.v))) picked.set(keyOf(r.sp, r.v), r);
    syncBoxes();
  }
  for (const el of [status, cls, standIn]) el.addEventListener('change', load);

  const tools = card('Show', h('div', { class: 'stack' },
    h('div', { class: 'row' }, status, cls, h('label', { class: 'check' }, standIn, 'Only spells still on a stand-in')),
    countLine,
    h('div', { class: 'row' }, selLine, allSel, clearSel, h('span', { class: 'grow' }), acceptSel, removeSel),
    h('p', { class: 'muted small' }, 'Players pick, on the public spell-visual page, which visual they think a spell plays; anyone may send one, no sign-in, one per spell per address. Preview plays a suggestion here; Accept gives the spell that visual from its next cast (revertible in the Audit log).')));
  const cards = [tools, listBox];
  mount(box,
    pageHead('Community visuals', 'What players suggest each spell\'s visual is, to compare and accept.'),
    preview ? h('div', { class: 'fx-layout' }, h('div', { class: 'fx-main' }, ...cards), h('div', { class: 'fx-side' }, preview.card)) : null,
    ...(preview ? [] : cards));
  load();
}
