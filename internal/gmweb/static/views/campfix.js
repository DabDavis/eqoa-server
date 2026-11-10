// Camp reports: players' /campfix reports and petitions' spawn requests filed
// as reports (campfix_list), waiting on a GM: nothing in one is applied until
// a GM approves it. Each report is a card: who sent it, where and when, its
// note (as text), and its marks in a table, each with what applying it does
// now (its plan: the row moved, turned, copied or deleted, the height on the
// ground) or why it cannot (stale: its row moved, was changed or deleted
// since), the flags the report recorded, and the review rules' colour as
// guidance (red: decide it on its facts; yellow: look at it; green: the rules
// call it safe) with every reason. Each mark has a checkbox: red and stale
// ones start unchecked, and a stale one cannot be checked. Approve checked
// (campfix_approve) applies the checked marks, each an audited change the
// world follows at once (revertible in the Audit log), marks the report
// applied with your note and closes its work board entry; Decline
// (campfix_decline) applies none, as abuse when ticked. Preview is the dry
// run. A decided report shows its review. #/campfix/<id> shows one report.

import { h, mount, pageHead, button, toast, card, empty, confirmBox } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { when, ago } from '../lib/fmt.js';

const FILTERS = [
  ['waiting', 'Waiting on a GM'],
  ['applied', 'Applied'],
  ['declined', 'Declined'],
  ['partial', 'Partial (an older review)'],
  ['all', 'All'],
];

const KIND = { move: 'Move', add: 'Add', remove: 'Remove', turn: 'Turn' };
const COLOUR = { red: 'danger', yellow: 'warn', green: 'ok' };

const num = (v, d = 1) => Number(v || 0).toFixed(d);
const at = (x, y, z) => `(${num(x)}, ${num(y)}, ${num(z)})`;

export async function render(box, parts) {
  const one = parts && /^\d+$/.test(parts[0] || '') ? Number(parts[0]) : 0;
  const status = h('select', { 'aria-label': 'Which reports' }, ...FILTERS.map(([v, l]) => h('option', { value: v }, l)));
  const countLine = h('span', { class: 'faint small' });
  const listBox = h('div', { class: 'stack' });

  // a mark's guidance: its colour, then every rule that holds by its colour
  function guidance(m) {
    if (m.stale) return h('div', { class: 'stack small' }, h('span', { class: 'badge' }, 'stale'), h('span', { text: m.stale }));
    const lists = [['red', m.red], ['yellow', m.yellow], ['green', m.green]].filter(([, l]) => l && l.length);
    return h('div', { class: 'stack small' },
      m.colour ? h('span', { class: 'badge ' + COLOUR[m.colour] }, m.colour) : null,
      lists.length ? h('ul', null, ...lists.flatMap(([c, l]) => l.map((r) => h('li', { class: c === m.colour ? '' : 'faint' },
        c === m.colour ? '' : c + ': ', h('span', { text: r }))))) : null);
  }

  function marksTable(r, waiting) {
    const boxes = [];
    const rows = r.marks.map((m) => {
      const cb = h('input', { type: 'checkbox', checked: waiting && m.checked, disabled: !waiting || !!m.stale,
        'aria-label': `Apply mark ${m.n}`, dataset: { mark: String(m.n) } });
      boxes.push(cb);
      const p = m.plan || {};
      const row = m.kind === 'add'
        ? (p.template || m.template ? h('span', null, 'a copy of ', h('a', { href: '#/npcs/' + (p.template || m.template) }, 'row ' + (p.template || m.template))) : '-')
        : (m.row ? h('a', { href: '#/npcs/' + m.row }, 'row ' + m.row) : h('span', { class: 'faint' }, 'none free'));
      const from = m.row || m.template ? `${at(m.from_x, m.from_y, m.from_z)}, facing ${m.from_facing}` : '-';
      const flags = (m.flags || []).map((f) => h('div', { class: 'small' }, h('span', { class: 'badge' }, f.flag), ' ', h('span', { text: f.text })));
      return h('tr', null,
        h('td', null, cb),
        h('td', { class: 'num' }, String(m.n)),
        h('td', null, KIND[m.kind] || m.kind),
        h('td', null, h('strong', { text: m.name }), m.level ? ` (L${m.level})` : '',
          m.typed && m.typed.toLowerCase() !== m.name.toLowerCase() ? h('div', { class: 'faint small' }, 'typed: ', h('span', { text: m.typed })) : null),
        h('td', { class: 'mono' }, h('a', { href: `#/map/${m.world}/@${Math.round(m.x)},${Math.round(m.z)}`, title: 'On the World map' },
          at(m.x, m.y, m.z)), h('div', { class: 'faint small' }, `facing ${m.facing_byte}`)),
        h('td', null, row, h('div', { class: 'faint small mono' }, from)),
        h('td', { class: 'num' }, m.row || m.template ? num(m.distance) : '-'),
        h('td', null, flags.length ? flags : h('span', { class: 'faint' }, 'none')),
        h('td', null, guidance(m)),
        h('td', { class: 'small' }, m.stale ? h('span', { class: 'faint' }, 'cannot be applied')
          : h('div', { class: 'stack' }, h('span', { text: p.words || '' }), p.ground ? h('span', { class: 'faint', text: 'height: ' + p.ground }) : null,
            p.fighting ? h('span', { class: 'err', text: `fighting ${p.fighting} now: the change makes it anew, whole, on its spawn` }) : null)));
    });
    const table = h('div', { class: 'table-scroll' }, h('table', { class: 'grid campfix' },
      h('thead', null, h('tr', null, ...['', '#', 'Kind', 'Creature', 'Mark (place, facing)', 'Row', 'Distance', 'Flags (the report\'s)',
        'Guidance', 'Applying it'].map((t) => h('th', { scope: 'col' }, t)))),
      h('tbody', null, ...rows)));
    return { table, checked: () => boxes.filter((b) => b.checked && !b.disabled).map((b) => Number(b.dataset.mark)) };
  }

  function reportCard(r) {
    const waiting = r.status === 'waiting';
    const counts = r.counts || {};
    const badges = [h('span', { class: 'badge' + (waiting ? ' warn' : '') }, r.status)];
    if (waiting) {
      for (const [c, cls] of [['red', 'danger'], ['yellow', 'warn'], ['green', 'ok'], ['stale', '']]) {
        if (counts[c]) badges.push(h('span', { class: 'badge ' + cls }, `${counts[c]} ${c}`));
      }
    }
    const meta = h('div', { class: 'row small' },
      h('span', null, h('strong', { text: r.character }), ` (level ${r.level} ${r.className})`),
      h('span', { class: 'faint' }, 'account ' + (r.account || r.accountid)),
      h('span', { class: 'faint' }, '·'),
      h('a', { href: `#/map/${r.world}/@${Math.round(r.x)},${Math.round(r.z)}` }, `${r.worldName} (${Math.round(r.x)}, ${Math.round(r.z)})`),
      r.petition ? h('span', { class: 'badge info' }, 'from petition ' + r.petition) : null,
      h('span', { class: 'grow' }),
      r.board ? h('a', { href: '#/work/' + r.board, class: 'faint' }, 'board entry ' + r.board) : null,
      h('span', { class: 'faint', title: when(r.created) }, ' · sent ', ago(r.created)));
    const marks = marksTable(r, waiting);
    const note = h('textarea', { rows: '2', maxlength: 2000, 'aria-label': 'Your note', placeholder: 'Your note (kept in the report\'s review)' });
    const abuse = h('input', { type: 'checkbox' });
    const preview = h('pre', { class: 'json hidden' });
    const act = async (name, args, done) => {
      try {
        const out = await op(name, args);
        toast(done(out), 'ok', out.live || '');
        load();
        return out;
      } catch (e) {
        toast(e.message, 'error');
        return null;
      }
    };
    const approve = async () => {
      const list = marks.checked();
      if (!list.length) { toast('Check the marks to apply first (or Decline it)', 'error'); return; }
      if (!(await confirmBox({ title: `Approve report #${r.id}?`,
        message: `Marks ${list.join(', ')} are applied now, each an audited change the world follows at once; the rest are declined.`,
        confirmLabel: 'Approve' }))) return;
      act('campfix_approve', { id: r.id, marks: list, note: note.value }, (o) => o.message);
    };
    const dryRun = async () => {
      try {
        const out = await op('campfix_approve', { id: r.id, marks: marks.checked(), note: note.value, dry_run: true });
        preview.textContent = out.results.map((x) => `mark ${x.n} (${x.kind} ${x.name}): ${x.result}${x.why ? ': ' + x.why : ''}${x.result === 'would apply' ? ': ' + x.words : ''}`).join('\n');
        preview.classList.remove('hidden');
      } catch (e) { toast(e.message, 'error'); }
    };
    const decline = async () => {
      if (!(await confirmBox({ title: `Decline report #${r.id}?`, message: abuse.checked
        ? 'No NPC changes. Declined as abuse, every mark of this account\'s later reports is red.' : 'No NPC changes.',
      confirmLabel: 'Decline', danger: true }))) return;
      act('campfix_decline', { id: r.id, note: note.value, abuse: abuse.checked }, (o) => o.message);
    };
    return card(`Report #${r.id}`, h('div', { class: 'stack' },
      meta,
      r.note ? h('div', { class: 'small' }, 'Note: ', h('span', { text: '“' + r.note + '”' })) : h('div', { class: 'faint small' }, 'no note'),
      ...(r.notes || []).map((n) => h('div', { class: 'notice warn small', text: n })),
      marks.table,
      waiting ? h('div', { class: 'stack' },
        note,
        h('div', { class: 'row' },
          button('Approve checked', { class: 'small primary', title: 'Apply the checked marks now', onclick: approve }),
          button('Preview', { class: 'small ghost', title: 'What Approve would do', onclick: dryRun }),
          h('span', { class: 'grow' }),
          h('label', { class: 'check small' }, abuse, 'as abuse'),
          button('Decline', { class: 'small ghost', title: 'Apply none of it', onclick: decline })),
        preview)
        : h('div', { class: 'stack' },
          h('div', { class: 'faint small' }, `${r.status} ${when(r.reviewed)}`),
          r.review ? h('pre', { class: 'json', text: r.review }) : null)), ...badges);
  }

  async function load() {
    mount(listBox, h('p', { class: 'faint' }, 'Loading…'));
    try {
      if (one) {
        const out = await op('campfix_get', { id: one });
        countLine.textContent = '';
        mount(listBox, reportCard(out.report));
        return;
      }
      const out = await op('campfix_list', { status: status.value, limit: 50 });
      const c = out.counts || {};
      countLine.textContent = `${out.matching} shown by the filter${out.matching > out.reports.length ? `, the first ${out.reports.length} here` : ''} · ` +
        `${c.waiting || 0} waiting, ${c.applied || 0} applied, ${c.declined || 0} declined in all`;
      mount(listBox, out.reports.length ? out.reports.map(reportCard)
        : empty(status.value === 'waiting' ? 'No camp report waits. Players send them with /campfix in the game; a petition\'s spawn request becomes one from its page.' : 'None.'));
    } catch (e) {
      mount(listBox, h('p', { class: 'err' }, e.message));
    }
  }
  status.addEventListener('change', load);

  mount(box,
    pageHead('Camp reports', 'Players\' camp reports and petitions\' spawn requests, waiting on a GM: nothing is applied before you approve it.'),
    one ? h('p', null, h('a', { href: '#/campfix' }, 'All camp reports')) : card('Show', h('div', { class: 'stack' },
      h('div', { class: 'row' }, status),
      countLine,
      h('p', { class: 'muted small' }, 'Each mark says what applying it does now, or why it cannot (stale: its row moved, was changed or deleted since the report was sent). The colours are the review rules\' guidance: red, decide it on its facts (unchecked to begin with); yellow, look at it; green, the rules call it safe. Approve checked applies the checked marks, each an audited change the world follows at once (revertible in the Audit log), and closes the report\'s board entry; Decline applies none. What players write is shown as text.'))),
    listBox);
  load();
}
