// The work board: who is working on what on this server (the GMs, their AI
// agents and dab's sessions), read as a document: the overview, then the
// entries in progress, blocked, planned and done in the last 14 days, the
// older done folded away. It reads and writes through work_board, post_work
// and update_work_overview, the operations an agent calls, so the page and
// every agent see one board, and every change is in the audit log under its
// GM, revertible (History). What people wrote is markdown built as elements
// (lib/md.js), never parsed as HTML. #/work/<id> opens on that entry.

import { h, mount, pageHead, button, toast, card, empty, dialog, formBox, store } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { state } from '../lib/nav.js';
import { ago, when } from '../lib/fmt.js';
import { markdown } from '../lib/md.js';
import { auditLine } from './audit.js';

const STATUS = {
  in_progress: { label: 'In progress', badge: 'warn', none: 'Nothing in progress.' },
  blocked: { label: 'Blocked', badge: 'danger', none: 'Nothing blocked.' },
  planned: { label: 'Planned', badge: 'info', none: 'Nothing planned.' },
  done: { label: 'Done', badge: 'ok', none: 'Nothing done lately.' },
};
const ORDER = ['in_progress', 'blocked', 'planned', 'done'];
const AREA = { code: 'Server code', data: 'Data and content', quests: 'Quests', client: 'Client', gm_tools: 'GM tools', other: 'Other' };

// notesIn is how many notes an entry's notes hold (a list item each).
const notesIn = (text) => (String(text || '').match(/^- /gm) || []).length;

export async function render(box, parts) {
  const focusID = Number(parts[0]) || 0;
  let board = null;
  let editing = false; // the overview's editor is open: a refresh leaves it alone
  let focused = false;

  const area = h('select', { 'aria-label': 'Area' }, h('option', { value: '' }, 'Every area'),
    ...Object.entries(AREA).map(([v, l]) => h('option', { value: v }, l)));
  const owner = h('input', { type: 'search', placeholder: 'Owner', 'aria-label': 'Owner' });
  const search = h('input', { type: 'search', placeholder: 'Text in the entries', 'aria-label': 'Search the entries' });
  const counts = h('span', { class: 'faint small' });
  const overviewBox = h('div', { class: 'work-overview' });
  const overviewFoot = h('div', { class: 'faint small' });
  const groupsBox = h('div', { class: 'stack' });
  const editBtn = button('Edit', { class: 'small', onclick: () => editOverview() });

  let timer;
  const reload = () => { clearTimeout(timer); timer = setTimeout(load, 250); };
  owner.addEventListener('input', reload);
  search.addEventListener('input', reload);
  area.addEventListener('change', load);

  mount(box,
    pageHead('Work board', 'Who is working on what on this server: the GMs, their AI agents and dab\'s sessions. Read it before you change anything, post what you are about to do with the tables or files it touches, and mark it done with a line saying what came of it.', null,
      button('New entry', { icon: 'plus', class: 'primary', onclick: () => editEntry(null) }),
      button('Refresh', { icon: 'refresh', onclick: () => load() })),
    card('Overview', h('div', { class: 'stack' }, overviewBox, overviewFoot), editBtn),
    h('div', { class: 'card' }, h('div', { class: 'grid-tools' }, area, owner, search, h('span', { class: 'grow' }), counts)),
    groupsBox);

  async function load() {
    const args = { full: true };
    if (area.value) args.area = area.value;
    if (owner.value.trim()) args.owner = owner.value.trim();
    if (search.value.trim()) args.search = search.value.trim();
    try {
      board = await op('work_board', args);
    } catch (e) {
      mount(groupsBox, h('p', { class: 'err' }, e.message));
      return;
    }
    if (!editing) drawOverview();
    drawGroups();
    if (focusID && !focused) {
      focused = true;
      const el = document.getElementById('work-' + focusID);
      if (el) el.scrollIntoView({ block: 'center' });
      else openEntry(focusID);
    }
  }

  // drawOverview shows the overview, a long one folded to its start until
  // Show all (the choice kept in this browser), so the entries stay in view.
  function drawOverview() {
    const ov = board.overview;
    editBtn.disabled = false;
    if (!ov.text) {
      mount(overviewBox, empty('No overview yet: Edit writes one.'));
    } else {
      const fold = h('div', { class: 'work-fold' }, markdown(ov.text));
      const toggle = button('', { class: 'small ghost', onclick: () => {
        const open = fold.classList.toggle('folded');
        store('work.overview', open ? 'folded' : 'open');
        toggle.lastChild.nodeValue = open ? 'Show all of it' : 'Fold it';
      } });
      mount(overviewBox, fold, h('div', { class: 'row hidden' }, toggle));
      requestAnimationFrame(() => {
        if (fold.scrollHeight < 380) return;
        const folded = store('work.overview') !== 'open';
        fold.classList.toggle('folded', folded);
        toggle.lastChild.nodeValue = folded ? 'Show all of it' : 'Fold it';
        toggle.parentNode.classList.remove('hidden');
      });
    }
    overviewFoot.textContent = ov.version ? `Version ${ov.version}, changed ${ago(ov.updated)} by ${ov.updated_by || 'nobody known'}` : '';
    overviewFoot.title = when(ov.updated);
  }

  function editOverview() {
    editing = true;
    editBtn.disabled = true;
    let base = board.overview.version;
    const text = h('textarea', { class: 'code work-editor', rows: '20', 'aria-label': 'The overview, in markdown' });
    text.value = board.overview.text;
    const preview = h('div', { class: 'work-preview hidden' });
    const msg = h('div');
    const against = h('span', { class: 'faint small' });
    const showBase = () => { against.textContent = `Markdown. It is saved against version ${base}: if someone saves first, you are shown theirs.`; };
    showBase();
    const previewBtn = button('Preview', {
      class: 'small ghost', onclick: () => {
        const on = preview.classList.toggle('hidden');
        if (!on) mount(preview, markdown(text.value));
        previewBtn.lastChild.nodeValue = on ? 'Preview' : 'Hide the preview';
      },
    });
    const done = () => { editing = false; drawOverview(); };
    const save = button('Save', {
      class: 'primary', icon: 'save', onclick: async () => {
        try {
          const out = await op('update_work_overview', { text: text.value, version: base });
          board.overview = out.overview;
          toast(out.message, 'ok');
          done();
        } catch (e) {
          if (e.status !== 409 || !e.data?.current) { toast(e.message, 'error'); return; }
          const cur = e.data.current;
          base = cur.version;
          showBase();
          mount(msg, h('div', { class: 'notice warn stack' },
            h('div', null, `${cur.updated_by || 'Someone'} changed the overview since you opened it (now version ${cur.version}, ${ago(cur.updated)}). `,
              'Your text is kept in the editor: put their change into it and Save again, or take theirs.'),
            h('details', { open: true }, h('summary', null, 'Their text'), markdown(cur.text)),
            h('div', { class: 'row' }, button('Take theirs', { class: 'small', onclick: () => { text.value = cur.text; mount(msg); } }))));
        }
      },
    });
    mount(overviewBox, h('div', { class: 'stack' }, msg, text, preview,
      h('div', { class: 'row' }, save, button('Cancel', { class: 'ghost', onclick: done }), previewBtn, against)));
    overviewFoot.textContent = '';
    text.focus();
  }

  function drawGroups() {
    const by = { in_progress: [], blocked: [], planned: [], done: [] };
    for (const e of board.entries) (by[e.status] || by.planned).push(e);
    const c = board.counts || {};
    const filtered = area.value || owner.value.trim() || search.value.trim();
    counts.textContent = (filtered ? `${board.matching} shown · ` : '') +
      ORDER.map((s) => `${c[s] || 0} ${STATUS[s].label.toLowerCase()}`).join(' · ') +
      (board.olderDone ? ` · ${board.olderDone} done earlier` : '');
    const groups = ORDER.map((st) => {
      const title = st === 'done' ? `Done in the last ${board.doneDays} days` : STATUS[st].label;
      return card(`${title} (${by[st].length})`, by[st].length ? h('div', { class: 'work-list' }, ...by[st].map(entryView)) : empty(STATUS[st].none));
    });
    if (board.olderDone) {
      const older = h('div', { class: 'work-list' }, h('p', { class: 'faint' }, 'Loading…'));
      const fold = h('details', { class: 'card work-older' },
        h('summary', null, `Done before ${new Date(board.doneSince).toLocaleDateString()} (${board.olderDone})`), older);
      fold.addEventListener('toggle', async () => {
        if (!fold.open || fold.dataset.loaded) return;
        fold.dataset.loaded = '1';
        try {
          const out = await op('work_board', { status: 'done', include_old_done: true, full: true, limit: 1000 });
          const list = out.entries.filter((e) => (e.done_at || e.updated) < board.doneSince);
          mount(older, ...(list.length ? list.map(entryView) : [empty('None.')]));
        } catch (e) { mount(older, h('p', { class: 'err' }, e.message)); }
      });
      groups.push(fold);
    }
    mount(groupsBox, ...groups);
  }

  function entryView(e) {
    const st = STATUS[e.status] || { label: e.status, badge: '' };
    const n = notesIn(e.notes);
    return h('article', { class: 'work-entry' + (e.id === focusID ? ' focus' : ''), id: 'work-' + e.id },
      h('div', { class: 'work-head' },
        h('a', { class: 'work-id mono', href: '#/work/' + e.id, title: 'A link to this entry' }, '#' + e.id),
        h('strong', { class: 'work-title' }, e.title),
        h('span', { class: 'badge ' + st.badge }, st.label),
        h('span', { class: 'badge' }, AREA[e.area] || e.area)),
      h('div', { class: 'work-meta faint small' },
        e.owner ? h('span', null, 'owner ', h('strong', null, e.owner)) : h('span', null, 'no owner'),
        ' · ', h('span', { title: when(e.updated) }, `changed ${ago(e.updated)} by ${e.updated_by}`),
        e.created !== e.updated ? h('span', { title: when(e.created) }, ` · posted ${ago(e.created)} by ${e.created_by}`) : null,
        e.status === 'done' && e.done_at ? h('span', { title: when(e.done_at) }, ` · done ${ago(e.done_at)}`) : null),
      e.touches.length
        ? h('div', { class: 'work-touches' }, h('span', { class: 'faint small' }, 'touches'), ...e.touches.map((t) => h('code', null, t)))
        : h('div', { class: 'faint small' }, 'touches nothing it names'),
      e.status === 'done' && e.result ? h('div', { class: 'work-result' }, h('strong', null, 'Result: '), e.result) : null,
      e.details ? markdown(e.details, 'md work-details') : null,
      e.notes ? h('details', { class: 'work-notes', open: e.status !== 'done' && n <= 4 },
        h('summary', null, `Notes (${n})`), markdown(e.notes)) : null,
      h('div', { class: 'row work-actions' }, ...actionsFor(e)));
  }

  function actionsFor(e) {
    const set = (status, extra, said) => post({ id: e.id, status, ...extra }, said);
    const out = [];
    if (e.status === 'planned' || e.status === 'blocked') {
      out.push(button(e.status === 'planned' ? 'Start' : 'Resume', { class: 'small', onclick: () => set('in_progress', {}, 'In progress') }));
    }
    if (e.status === 'planned' || e.status === 'in_progress') {
      out.push(button('Blocked…', {
        class: 'small', onclick: async () => {
          const v = await formBox({ title: `Blocked: #${e.id}`, intro: 'What it is waiting on: a note with your name.', submitLabel: 'Mark it blocked',
            fields: [{ name: 'note', label: 'Waiting on', type: 'textarea' }] });
          if (v) set('blocked', v.note.trim() ? { note: v.note } : {}, 'Blocked');
        },
      }));
    }
    if (e.status !== 'done') {
      out.push(button('Done…', {
        class: 'small', onclick: async () => {
          const v = await formBox({ title: `Done: #${e.id}`, intro: `${e.title}: one line saying what came of it.`, submitLabel: 'Mark it done',
            fields: [{ name: 'result', label: 'Result', value: e.result || '', placeholder: '18 guards moved to the new gate' },
              { name: 'note', label: 'A note too (optional)', type: 'textarea' }] });
          if (v) set('done', { result: v.result, ...(v.note.trim() ? { note: v.note } : {}) }, 'Done');
        },
      }));
    } else {
      out.push(button('Reopen', { class: 'small', onclick: () => set('in_progress', {}, 'Reopened') }));
    }
    out.push(button('Note…', {
      class: 'small', onclick: async () => {
        const v = await formBox({ title: `A note on #${e.id}`, intro: 'Added below the others, with the time and your name (markdown).', submitLabel: 'Add it',
          fields: [{ name: 'note', label: 'Note', type: 'textarea' }] });
        if (v && v.note.trim()) post({ id: e.id, note: v.note }, 'Note added');
      },
    }));
    out.push(button('Edit', { class: 'small ghost', onclick: () => editEntry(e) }));
    out.push(button('History', { class: 'small ghost', icon: 'history', onclick: () => showHistory(e) }));
    return out;
  }

  // post sends a post_work and says what came of it; a stale version shows
  // the entry as it is now.
  async function post(args, said) {
    try {
      const out = await op('post_work', args);
      toast(`${said}: #${out.entry.id} ${out.entry.title}`, 'ok');
      if (out.warning) toast(out.warning, 'warn', '', 15000);
      await load();
      return out;
    } catch (e) {
      if (e.status === 409 && e.data?.current) conflict(e, args);
      else toast(e.message, 'error');
      return null;
    }
  }

  function conflict(err, args) {
    const cur = err.data.current;
    dialog({
      title: 'Someone changed it first', wide: true,
      body: h('div', { class: 'stack' },
        h('p', null, `#${cur.id} ${cur.title} was changed by ${cur.updated_by} ${ago(cur.updated)} (now version ${cur.version}). Nothing of yours was saved.`),
        cur.details ? h('details', { open: true }, h('summary', null, 'Its details now'), markdown(cur.details)) : null),
      actions: [
        { label: 'Close', kind: 'ghost' },
        { label: 'Edit again with my changes', kind: 'primary', onClick: () => { editEntry(cur, { ...cur, ...args, version: cur.version }); } },
      ],
    });
    load();
  }

  // editEntry is the form for a new entry (e null) or a change to one; draft
  // fills it with what a refused save had (and the version to save against).
  async function editEntry(e, draft) {
    const cur = draft || e || {};
    const touches = Array.isArray(cur.touches) ? cur.touches.join(', ') : (cur.touches || '');
    const v = await formBox({
      title: e ? `Edit #${e.id}` : 'New entry', wide: true, submitLabel: e ? 'Save' : 'Post it',
      intro: e ? `Saved against version ${cur.version}: if someone changes it first, you are shown theirs.`
        : 'Post it before you change anything, with the tables and files it touches: the answer names any open entry touching the same things.',
      fields: [
        { name: 'title', label: 'What is being done (one line)', value: cur.title || '' },
        { name: 'status', label: 'Status', type: 'select', value: cur.status || 'in_progress', options: ORDER.map((s) => ({ value: s, label: STATUS[s].label })) },
        { name: 'area', label: 'Area', type: 'select', value: cur.area || 'other', options: Object.entries(AREA).map(([value, label]) => ({ value, label })) },
        { name: 'touches', label: 'The tables and files it touches', value: touches, placeholder: 'npcs, npcpaths, internal/gmweb/ops.go', hint: 'comma separated; a folder holds what is in it' },
        { name: 'owner', label: 'Owner', value: cur.owner ?? (state.session?.user?.username || ''), hint: 'a GM\'s account name, or dab\'s sessions' },
        { name: 'details', label: 'Details (markdown)', type: 'textarea', value: cur.details || '' },
        { name: 'result', label: 'Result (one line, once done)', value: cur.result || '' },
      ],
    });
    if (!v) return;
    const args = { title: v.title, status: v.status, area: v.area, touches: v.touches, owner: v.owner, details: v.details };
    if (v.result.trim() || (e && e.result)) args.result = v.result;
    if (e) {
      args.id = e.id;
      args.version = cur.version;
    }
    const out = await post(args, e ? 'Saved' : 'Posted');
    if (out && !e) {
      const el = document.getElementById('work-' + out.entry.id);
      if (el) { el.classList.add('focus'); el.scrollIntoView({ block: 'center' }); }
    }
  }

  async function openEntry(id) {
    try {
      const out = await op('work_board', { id });
      mount(groupsBox, card(`Entry #${id}`, entryView(out.entry)), ...groupsBox.childNodes);
    } catch (e) { toast(e.message, 'error'); }
  }

  async function showHistory(e) {
    const list = h('div', null, h('p', { class: 'faint' }, 'Loading…'));
    dialog({
      title: `History of #${e.id}`, wide: true,
      body: h('div', { class: 'stack' },
        h('p', { class: 'muted small' }, 'Every change to it, who made it and through what, newest first (the Audit log has them all). A change can be reverted.'),
        list),
    });
    try {
      const out = await op('audit_log', { table: 'workboard', key: { id: e.id }, limit: 100 });
      mount(list, out.entries.length ? h('div', null, ...out.entries.map((x) => auditLine(x))) : empty('No changes logged.'));
    } catch (err) { mount(list, h('p', { class: 'err' }, err.message)); }
  }

  await load();
  const poll = setInterval(() => { if (!editing && !document.querySelector('.overlay')) load(); }, 30000);
  return { leave: () => { clearInterval(poll); clearTimeout(timer); } };
}
