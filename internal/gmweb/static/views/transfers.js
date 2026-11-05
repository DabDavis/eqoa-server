// Character transfers: players' requests to bring a character over from
// another server (transfer_list), the open ones first. Decide opens one
// (transfer_get): its gear list's lines matched to this server's records, each
// line's record picked (an exact match picked already, a near one to check), a
// copy of another record made for an item this server lacks, the level and
// mastery points set; Preview says what the plan does, Approve keeps it (it
// applies at the character's next login), Reject says why. An applied one
// can be undone while its character is out of the world. Player text is shown
// as text. The quests part lays out the class's catalog as the player saw it
// (paths' and the epic's choices, the lines, the flags) with the player's
// claims picked and ticked: what stays picked is marked done at the login.

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

  // The quests part of the decision: the class's catalog (transfer_get) with
  // the player's claims picked and ticked; plan() is its quests and vampire.
  function questsPart(t, catalog, questNames) {
    const claims = t.claims || {};
    const picks = claims.picks || [];
    const claimed = new Set(claims.quests || []);
    const choices = [];
    const node = h('div', { class: 'stack' });
    const tick = (q, name) => h('label', { class: 'row small' },
      h('input', { type: 'checkbox', dataset: { quest: String(q) }, checked: claimed.has(q) }), name || questNames[q] || ('quest ' + q),
      claimed.has(q) ? h('span', { class: 'badge' }, 'claimed') : null);
    const listed = new Set();
    if (catalog) {
      for (const c of catalog.choices || []) {
        const mine = picks.find((p) => p.step === c.step && (p.quest === c.quest || (c.also || []).includes(p.quest)));
        const quest = mine ? mine.quest : c.quest;
        listed.add(quest);
        const sel = h('select', { 'aria-label': c.ask || c.name }, h('option', { value: '' }, 'not done'),
          ...c.options.map((o) => h('option', { value: String(o.option) }, o.answer)));
        if (mine) sel.value = String(mine.option);
        const chain = (c.chains || {})[quest] || [];
        const withChain = h('input', { type: 'checkbox', checked: true });
        choices.push({ sel, quest, step: c.step, chain, withChain });
        node.appendChild(h('div', { class: 'row small' },
          h('span', { text: `${c.kind === 'epic' ? 'Epic (60)' : 'Path ' + c.level}: ${questNames[quest] || c.name}: ${c.ask}` }), sel,
          chain.length ? h('label', { class: 'row' }, withChain, `and its ${chain.length} earlier quests`) : null,
          mine ? h('span', { class: 'badge' }, 'claimed') : null));
      }
      const lineSel = h('select', { 'aria-label': 'Line' }, h('option', { value: '' }, 'neither'),
        ...(catalog.lines || []).map((l) => h('option', { value: l.line }, l.label)));
      lineSel.value = claims.line || '';
      const lineBox = h('div', { class: 'stack' });
      const drawLine = () => {
        const l = (catalog.lines || []).find((x) => x.line === lineSel.value);
        mount(lineBox, l ? l.quests.map((q) => tick(q.quest, q.name)) : []);
      };
      lineSel.addEventListener('change', drawLine);
      drawLine();
      for (const l of catalog.lines || []) l.quests.forEach((q) => listed.add(q.quest));
      node.appendChild(h('div', { class: 'row small' }, 'Line:', lineSel, claims.line ? h('span', { class: 'badge' }, 'claimed: ' + claims.line) : null));
      node.appendChild(lineBox);
      const flags = (catalog.flags || []);
      flags.forEach((g) => g.quests.forEach((q) => listed.add(q.quest)));
      const anyFlag = flags.some((g) => g.quests.some((q) => claimed.has(q.quest)));
      if (flags.length) {
        node.appendChild(h('details', { open: anyFlag }, h('summary', { class: 'small' }, 'Flags (the Portal quests)'),
          ...flags.map((g) => h('div', { class: 'stack' }, h('strong', { class: 'small', text: g.label }), ...g.quests.map((q) => tick(q.quest, q.name))))));
      }
    } else {
      node.appendChild(h('div', { class: 'faint small' }, 'No catalog: the character here has no class this server knows.'));
    }
    const strays = [...claimed].filter((q) => !listed.has(q));
    if (strays.length) node.appendChild(h('div', { class: 'stack' }, h('div', { class: 'faint small' }, 'Claimed, not in the class\'s catalog:'),
      ...strays.map((q) => tick(q))));
    const vampire = h('input', { type: 'checkbox', checked: claims.line === 'vampire' });
    node.appendChild(h('label', { class: 'row small' }, vampire, 'make a vampire (the sun burns it by day)'));
    if (claims.other) node.appendChild(h('div', { class: 'stack' }, h('div', { class: 'faint small' }, 'Other quests, in the player\'s words:'),
      h('pre', { class: 'json' }, claims.other)));
    const extra = h('input', { type: 'text', placeholder: 'more quest ids done, comma separated (Quests page)' });
    node.appendChild(extra);
    function plan() {
      const byQuest = new Map();
      const add = (q) => { if (!byQuest.has(q)) byQuest.set(q, new Map()); return byQuest.get(q); };
      for (const c of choices) {
        if (c.sel.value === '') continue;
        add(c.quest).set(c.step, Number(c.sel.value));
        if (c.withChain.checked) c.chain.forEach(add);
      }
      node.querySelectorAll('input[data-quest]').forEach((b) => { if (b.checked) add(Number(b.dataset.quest)); });
      for (const part of extra.value.split(',')) {
        const id = Number(part.trim());
        if (part.trim() && !(id > 0)) throw new Error(`"${part.trim()}" is not a quest id`);
        if (id > 0) add(id);
      }
      const quests = [...byQuest].map(([quest, steps]) => ({ quest,
        picks: [...steps].sort((a, b) => a[0] - b[0]).map(([st, o]) => `${st}:${o}`).join(', ') }));
      return { quests, vampire: vampire.checked };
    }
    return { node, plan };
  }

  function candLabel(c) {
    return `${c.name} (#${c.patternid}, ${SLOTS[c.slot] || 'slot ' + c.slot}, level ${c.level}${c.classuse === 0 ? ', NO CLASS MASK' : ''})`;
  }

  // The decision form for one transfer, from transfer_get.
  function decide(t, lines, catalog, questNames, slot) {
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
    const quests = questsPart(t, catalog, questNames || {});
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
        full_vitals: vitals.checked, items, ...quests.plan() };
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
      h('div', { class: 'faint small' }, 'Quests: what stays picked or ticked is marked done at the login, with the spells it gives the class (no experience, tunar or items).'),
      quests.node,
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
    const cl = t.claims || null;
    const says = cl ? h('div', { class: 'small' }, `Says it did: ${(cl.picks || []).length} path or epic picks, ${(cl.quests || []).length} quests` +
      (cl.line ? `, the ${cl.line} line` : '') + (cl.other ? ', and others in its words' : '') + ' (Decide lists them)') : null;
    const tools = [];
    if (t.status === 'waiting' || t.status === 'approved') {
      tools.push(button(t.status === 'approved' ? 'Change the plan' : 'Decide', {
        class: 'small primary', onclick: async () => {
          try {
            const out = await op('transfer_get', { id: t.id });
            decide(out.transfer, out.lines || [], out.catalog || null, out.questNames || {}, slot);
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
      meta, what, says,
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
