// Character transfers: players' requests to bring a character over from
// another server (transfer_list), the open ones first. Decide opens one
// (transfer_get): its gear list's lines matched to this server's records, each
// line's record picked (an exact match picked already, a near one to check), a
// copy of another record made for an item this server lacks, the level and
// mastery points set; Preview says what the plan does, Approve keeps it (it
// applies at the character's next login), Reject says why. An applied one
// can be undone while its character is out of the world. Player text is shown
// as text.

import { h, mount, pageHead, button, toast, card, empty, confirmBox } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { when, ago } from '../lib/fmt.js';

const FILTERS = [
  ['open', 'Open (waiting, approved)'],
  ['waiting', 'Waiting on a GM'],
  ['approved', 'Approved (next login)'],
  ['applied', 'Applied'],
  ['rejected', 'Rejected'],
  ['cancelled', 'Cancelled by the player'],
  ['undone', 'Undone'],
  ['all', 'All'],
];

const SLOTS = { 1: 'helm', 2: 'robe', 3: 'earring', 4: 'neck', 5: 'chest', 6: 'arms', 7: 'wrist', 8: 'ring', 9: 'belt', 10: 'legs',
  11: 'feet', 12: 'primary', 13: 'shield', 14: 'secondary', 15: 'two-handed', 16: 'ranged', 19: 'hands' };

export async function render(box) {
  const status = h('select', { 'aria-label': 'Which transfers' }, ...FILTERS.map(([v, l]) => h('option', { value: v }, l)));
  const countLine = h('span', { class: 'faint small' });
  const listBox = h('div', { class: 'stack' });

  async function act(name, args, done) {
    try {
      const out = await op(name, args);
      toast(done(out), 'ok', out.live || '');
      load();
      return out;
    } catch (e) {
      toast(e.message, 'error');
      return null;
    }
  }

  function candLabel(c) {
    return `${c.name} (#${c.patternid}, ${SLOTS[c.slot] || 'slot ' + c.slot}, level ${c.level}${c.classuse === 0 ? ', NO CLASS MASK' : ''})`;
  }

  // The decision form for one transfer, from transfer_get.
  function decide(t, lines, slot) {
    const level = h('input', { type: 'number', min: 1, max: 60, value: t.from_level, style: 'max-width: 6em' });
    const mastery = h('input', { type: 'number', min: 0, max: 1500, value: t.from_masteries, style: 'max-width: 7em' });
    const spells = h('input', { type: 'checkbox', checked: true });
    const vitals = h('input', { type: 'checkbox', checked: true });
    const answer = h('textarea', { rows: '3', maxlength: 2000, placeholder: 'A word to the player (on the transfer page and in the game)' });
    const preview = h('pre', { class: 'json hidden' });
    const rows = [];
    const lineRows = lines.map((l) => {
      l.candidates = l.candidates || [];
      const count = h('input', { type: 'number', min: 1, max: 20, value: l.count, style: 'max-width: 5em', 'aria-label': 'How many' });
      const pick = h('select', { 'aria-label': 'Record' },
        ...l.candidates.map((c) => h('option', { value: String(c.patternid) }, (c.score >= 0.99 ? '' : '≈ ') + candLabel(c))),
        h('option', { value: 'make' }, '— make a new record (a copy of another) —'),
        h('option', { value: 'skip' }, '— skip this line —'));
      const like = h('input', { type: 'number', min: 1, placeholder: 'copy record #', style: 'max-width: 9em' });
      const name = h('input', { type: 'text', maxlength: 32, value: l.name.slice(0, 32), placeholder: 'the new record\'s name' });
      const makeBox = h('span', { class: 'row small hidden' }, 'copy of #', like, 'named', name);
      const best = l.candidates[0];
      if (l.exact && best) pick.value = String(best.patternid);
      else if (best && best.score >= 0.8) pick.value = String(best.patternid);
      else { pick.value = 'make'; if (best) like.value = best.patternid; }
      const sync = () => makeBox.classList.toggle('hidden', pick.value !== 'make');
      pick.addEventListener('change', sync);
      sync();
      const flag = l.exact ? null : h('span', { class: 'badge warn' }, l.candidates.length ? 'check' : 'no record');
      rows.push({ count, pick, like, name });
      return h('div', { class: 'stack', style: 'border-top: 1px solid var(--line); padding-top: 6px' },
        h('div', { class: 'row small' }, h('strong', { text: l.text }), flag),
        h('div', { class: 'row' }, count, pick), makeBox);
    });
    const extraBox = h('div', { class: 'stack' });
    const addExtra = button('Add an item', {
      class: 'small ghost', onclick: () => {
        const count = h('input', { type: 'number', min: 1, max: 20, value: 1, style: 'max-width: 5em' });
        const pattern = h('input', { type: 'number', min: 1, placeholder: 'record #', style: 'max-width: 9em' });
        rows.push({ count, pattern });
        extraBox.appendChild(h('div', { class: 'row small' }, 'extra:', count, 'of record #', pattern));
      },
    });
    function plan() {
      const items = [];
      for (const r of rows) {
        const count = Number(r.count.value || 1);
        if (r.pattern) { if (r.pattern.value) items.push({ pattern: Number(r.pattern.value), count }); continue; }
        if (r.pick.value === 'skip') continue;
        if (r.pick.value === 'make') {
          if (!r.like.value) throw new Error('a new record needs a record to copy (copy record #)');
          items.push({ like: Number(r.like.value), name: r.name.value, count });
        } else items.push({ pattern: Number(r.pick.value), count });
      }
      return { level: Number(level.value), mastery_points: Number(mastery.value || 0), class_spells: spells.checked,
        full_vitals: vitals.checked, items };
    }
    const doPreview = async () => {
      try {
        const out = await op('transfer_preview', { id: t.id, plan: plan() });
        preview.textContent = out.preview;
        preview.classList.remove('hidden');
      } catch (e) { toast(e.message, 'error'); }
    };
    const approve = async () => {
      let p;
      try { p = plan(); } catch (e) { toast(e.message, 'error'); return; }
      const out = await act('transfer_approve', { id: t.id, plan: p, answer: answer.value }, (o) => o.message);
      if (out) { preview.textContent = out.preview; preview.classList.remove('hidden'); }
    };
    const reject = async () => {
      if (!answer.value.trim()) { toast('Write why first: the player reads it', 'error'); return; }
      if (!(await confirmBox({ title: `Reject transfer #${t.id}?`, message: 'The player reads your word on the transfer page.',
        confirmLabel: 'Reject', danger: true }))) return;
      act('transfer_reject', { id: t.id, answer: answer.value }, (o) => o.message);
    };
    const warn = t.classMatches === false && t.character
      ? h('div', { class: 'notice warn small' }, `The character here is a ${t.character.class}; the player says it was a ${t.from_class}.`) : null;
    mount(slot,
      warn,
      h('div', { class: 'row small' }, h('label', { class: 'row' }, 'Level', level), h('label', { class: 'row' }, 'Mastery points', mastery),
        h('label', { class: 'row' }, spells, 'the class\'s spells'), h('label', { class: 'row' }, vitals, 'whole at the login')),
      h('div', { class: 'faint small' }, 'Gear: into the pack, the bank when the pack is full. "≈" is a near match: check it.'),
      ...lineRows, extraBox, h('div', { class: 'row' }, addExtra),
      answer,
      h('div', { class: 'row' }, button('Preview', { class: 'small ghost', onclick: doPreview }),
        button('Approve', { class: 'small primary', title: 'It applies at the character\'s next login', onclick: approve }),
        h('span', { class: 'grow' }), button('Reject', { class: 'small ghost', onclick: reject })),
      preview);
  }

  function transferCard(t) {
    const slot = h('div', { class: 'stack' });
    const badges = [h('span', { class: 'badge' + (t.status === 'waiting' ? ' warn' : '') }, t.status)];
    if (t.online) badges.push(h('span', { class: 'badge' }, 'in the world'));
    const here = t.character ? `${t.charname} (level ${t.character.level} ${t.character.race} ${t.character.class})` : t.charname;
    const meta = h('div', { class: 'row small' },
      h('a', { href: '#/characters/' + t.serverid }, here), h('span', { class: 'faint' }, 'account ' + t.account),
      h('span', { class: 'grow' }), h('span', { class: 'faint', title: when(t.filed) }, 'asked ', ago(t.filed)));
    const what = h('div', { class: 'small' }, `${t.from_name} on ${t.from_server}: level ${t.from_level} ${t.from_class}, ${t.from_masteries} mastery points`);
    const tools = [];
    if (t.status === 'waiting' || t.status === 'approved') {
      tools.push(button(t.status === 'approved' ? 'Change the plan' : 'Decide', {
        class: 'small primary', onclick: async () => {
          try {
            const out = await op('transfer_get', { id: t.id });
            decide(out.transfer, out.lines || [], slot);
          } catch (e) { toast(e.message, 'error'); }
        },
      }));
    }
    if (t.status === 'applied') {
      tools.push(button('Undo', {
        class: 'small ghost', title: 'Take back every change it made (the character must be out of the world)', onclick: async () => {
          if (!(await confirmBox({ title: `Undo transfer #${t.id}?`, message: `Every change its run (${t.run}) made goes back.`,
            confirmLabel: 'Undo', danger: true }))) return;
          act('transfer_undo', { id: t.id }, (o) => o.message);
        },
      }));
    }
    return card(`Transfer #${t.id}`, h('div', { class: 'stack' },
      meta, what,
      h('div', { class: 'faint small' }, 'Gear, as the player wrote it:'), h('pre', { class: 'json' }, t.items || '(none)'),
      t.notes ? h('div', { class: 'faint small' }, 'Notes:') : null, t.notes ? h('pre', { class: 'json' }, t.notes) : null,
      t.answer ? h('div', { class: 'small' }, 'Our word: ', h('span', { text: t.answer })) : null,
      t.decided ? h('div', { class: 'faint small' }, `decided ${when(t.decided)} by ${t.decided_by}`) : null,
      t.applied ? h('div', { class: 'faint small' }, `applied ${when(t.applied)} (run ${t.run})`) : null,
      h('div', { class: 'row' }, ...tools), slot), ...badges);
  }

  async function load() {
    mount(listBox, h('p', { class: 'faint' }, 'Loading…'));
    try {
      const out = await op('transfer_list', { status: status.value });
      const c = out.counts || {};
      countLine.textContent = `${out.matching} shown · ${c.waiting || 0} waiting, ${c.approved || 0} approved, ${c.applied || 0} applied in all`;
      mount(listBox, out.transfers.length ? out.transfers.map(transferCard)
        : empty(status.value === 'open' ? 'No transfer is open. Players ask on the transfer page (/eqoa/transfer).' : 'None.'));
    } catch (e) {
      mount(listBox, h('p', { class: 'err' }, e.message));
    }
  }
  status.addEventListener('change', load);
  mount(box,
    pageHead('Character transfers', 'Players bringing a character over from another server.'),
    card('Show', h('div', { class: 'stack' }, h('div', { class: 'row' }, status), countLine,
      h('p', { class: 'muted small' }, 'Decide matches the player\'s gear to our records: an exact match is picked, a near one ("≈") is for you to check, and an item we have no record of becomes a copy of another record under its name. Approve keeps the plan; it applies at the character\'s next login (a player in the world is told to log out and in), in one step that Undo takes back. Nothing the player wrote is applied as they wrote it: only your plan.'))),
    listBox);
  load();
}
