// Building the page: elements from data with textContent only (names, chat and
// item text come from players and imports, so nothing is ever parsed as HTML),
// dialogs, toasts and icons.

export function h(tag, attrs, ...kids) {
  const el = document.createElement(tag);
  if (attrs) {
    for (const [k, v] of Object.entries(attrs)) {
      if (v === undefined || v === null || v === false) continue;
      if (k === 'class') el.className = v;
      else if (k === 'dataset') Object.assign(el.dataset, v);
      else if (k.startsWith('on') && typeof v === 'function') el.addEventListener(k.slice(2), v);
      else if (k === 'value') el.value = v;
      else if (k === 'checked' || k === 'disabled' || k === 'selected' || k === 'readOnly' || k === 'multiple') el[k] = !!v;
      else if (k === 'text') el.textContent = v;
      else el.setAttribute(k, v === true ? '' : String(v));
    }
  }
  append(el, kids);
  return el;
}

function append(el, kids) {
  for (const k of kids) {
    if (k === null || k === undefined || k === false) continue;
    if (Array.isArray(k)) append(el, k);
    else if (k instanceof Node) el.appendChild(k);
    else el.appendChild(document.createTextNode(String(k)));
  }
}

export function clear(el) {
  while (el.firstChild) el.removeChild(el.firstChild);
  return el;
}

export function mount(el, ...kids) {
  clear(el);
  append(el, kids);
  return el;
}

// Icons: a small set of strokes, drawn as SVG nodes (no markup strings).
const ICONS = {
  home: 'M3 10.5 12 3l9 7.5V21a1 1 0 0 1-1 1h-5v-7H9v7H4a1 1 0 0 1-1-1z',
  users: 'M16 19v-1a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v1M9 10a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7M22 19v-1a4 4 0 0 0-3-3.87M16 3.13a3.5 3.5 0 0 1 0 6.75',
  user: 'M20 21v-1a5 5 0 0 0-5-5H9a5 5 0 0 0-5 5v1M12 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8',
  globe: 'M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20M2 12h20M12 2a15 15 0 0 1 0 20M12 2a15 15 0 0 0 0 20',
  map: 'M9 4 3 6v14l6-2 6 2 6-2V4l-6 2zM9 4v14M15 6v14',
  skull: 'M12 3a8 8 0 0 0-8 8c0 2.8 1.4 4.6 3 5.6V20h10v-3.4c1.6-1 3-2.8 3-5.6a8 8 0 0 0-8-8M9 12h.01M15 12h.01M10 20v-2M14 20v-2',
  shop: 'M3 9l1.5-5h15L21 9M4 9v11h16V9M3 9h18M9 20v-6h6v6',
  sword: 'M14.5 4H20v5.5L9 20.5 3.5 15zM6 13l5 5M4 20l2-2',
  spark: 'M12 2v4M12 18v4M4.9 4.9l2.8 2.8M16.3 16.3l2.8 2.8M2 12h4M18 12h4M4.9 19.1l2.8-2.8M16.3 7.7l2.8-2.8',
  book: 'M4 4.5A2.5 2.5 0 0 1 6.5 2H20v17H6.5A2.5 2.5 0 0 0 4 21.5zM4 21.5A2.5 2.5 0 0 0 6.5 24',
  table: 'M3 4h18v16H3zM3 10h18M3 15h18M9 4v16',
  history: 'M3 12a9 9 0 1 0 3-6.7L3 8M3 3v5h5M12 7v5l3 2',
  terminal: 'M4 17l6-5-6-5M12 19h8',
  db: 'M12 3c4.4 0 8 1.3 8 3s-3.6 3-8 3-8-1.3-8-3 3.6-3 8-3M4 6v6c0 1.7 3.6 3 8 3s8-1.3 8-3V6M4 12v6c0 1.7 3.6 3 8 3s8-1.3 8-3v-6',
  cog: 'M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1',
  bot: 'M12 8V4H8M4 12h16v8H4zM2 14v2M22 14v2M9 16h.01M15 16h.01M8 8h8a2 2 0 0 1 2 2v2H6v-2a2 2 0 0 1 2-2',
  lock: 'M5 11h14v10H5zM8 11V7a4 4 0 0 1 8 0v4',
  disc: 'M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6',
  search: 'M11 19a8 8 0 1 0 0-16 8 8 0 0 0 0 16M21 21l-4.3-4.3',
  menu: 'M3 6h18M3 12h18M3 18h18',
  x: 'M18 6 6 18M6 6l12 12',
  plus: 'M12 5v14M5 12h14',
  copy: 'M9 9h11v11H9zM5 15H4V4h11v1',
  refresh: 'M21 12a9 9 0 1 1-2.6-6.4L21 8M21 3v5h-5',
  trash: 'M3 6h18M8 6V4h8v2M19 6l-1 14H6L5 6',
  undo: 'M9 14 4 9l5-5M4 9h11a5 5 0 0 1 0 10h-1',
  send: 'M22 2 11 13M22 2l-7 20-4-9-9-4z',
  log: 'M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01',
  sun: 'M12 17a5 5 0 1 0 0-10 5 5 0 0 0 0 10M12 1v2M12 21v2M4.2 4.2l1.4 1.4M18.4 18.4l1.4 1.4M1 12h2M21 12h2M4.2 19.8l1.4-1.4M18.4 5.6l1.4-1.4',
  out: 'M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9',
  save: 'M5 3h11l3 3v15H5zM8 3v6h8M8 21v-7h8v7',
  pin: 'M12 22s7-6.2 7-12a7 7 0 1 0-14 0c0 5.8 7 12 7 12M12 13a3 3 0 1 0 0-6 3 3 0 0 0 0 6',
  bolt: 'M13 2 3 14h9l-1 8 10-12h-9z',
  box: 'M21 8 12 3 3 8v8l9 5 9-5zM3 8l9 5 9-5M12 13v8',
};

export function icon(name) {
  const NS = 'http://www.w3.org/2000/svg';
  const svg = document.createElementNS(NS, 'svg');
  svg.setAttribute('viewBox', '0 0 24 24');
  svg.setAttribute('fill', 'none');
  svg.setAttribute('stroke', 'currentColor');
  svg.setAttribute('stroke-width', '1.8');
  svg.setAttribute('stroke-linecap', 'round');
  svg.setAttribute('stroke-linejoin', 'round');
  svg.setAttribute('aria-hidden', 'true');
  const p = document.createElementNS(NS, 'path');
  p.setAttribute('d', ICONS[name] || ICONS.box);
  svg.appendChild(p);
  return svg;
}

export function button(label, opts = {}) {
  const b = h('button', { type: 'button', class: opts.class || '', title: opts.title, disabled: opts.disabled },
    opts.icon ? icon(opts.icon) : null, label);
  if (opts.onclick) {
    b.addEventListener('click', async (ev) => {
      if (b.disabled) return;
      b.disabled = true;
      try { await opts.onclick(ev); } finally { b.disabled = false; }
    });
  }
  return b;
}

// Toasts
let toastBox = null;
export function toast(msg, kind = 'info', live = '', ms = 5000) {
  if (!toastBox) {
    toastBox = h('div', { class: 'toasts', role: 'status', 'aria-live': 'polite' });
    document.body.appendChild(toastBox);
  }
  const t = h('div', { class: 'toast ' + kind }, h('div', null, msg), live ? h('div', { class: 't-live' }, live) : null);
  toastBox.appendChild(t);
  setTimeout(() => t.remove(), kind === 'error' ? Math.max(ms, 9000) : ms);
}

// Dialogs
export function dialog({ title, body, actions = [], wide = false, onClose }) {
  const prev = document.activeElement;
  const box = h('div', { class: 'dialog' + (wide ? ' wide' : ''), role: 'dialog', 'aria-modal': 'true' });
  const overlay = h('div', { class: 'overlay' }, box);
  const close = () => {
    overlay.remove();
    document.removeEventListener('keydown', onKey);
    if (prev && prev.focus) prev.focus();
    if (onClose) onClose();
  };
  const onKey = (e) => { if (e.key === 'Escape') close(); };
  document.addEventListener('keydown', onKey);
  overlay.addEventListener('mousedown', (e) => { if (e.target === overlay) close(); });
  const foot = h('div', { class: 'dialog-foot' });
  for (const a of actions) {
    foot.appendChild(button(a.label, {
      class: a.kind || '', icon: a.icon,
      onclick: async () => { const keep = await a.onClick?.(close); if (keep !== true && a.closes !== false) close(); },
    }));
  }
  mount(box,
    h('div', { class: 'dialog-head' }, h('h2', null, title), h('button', { class: 'ghost icon', type: 'button', 'aria-label': 'Close', onclick: close }, icon('x'))),
    h('div', { class: 'dialog-body' }, body),
    actions.length ? foot : null);
  document.body.appendChild(overlay);
  const first = box.querySelector('input, select, textarea, button.primary');
  if (first) setTimeout(() => first.focus(), 0);
  return { close, box };
}

// confirmBox resolves true when confirmed. typeToConfirm asks for a word first.
export function confirmBox({ title, message, confirmLabel = 'Confirm', danger = false, typeToConfirm }) {
  return new Promise((resolve) => {
    let done = false;
    const input = typeToConfirm ? h('input', { type: 'text', autocomplete: 'off', placeholder: typeToConfirm }) : null;
    const body = h('div', { class: 'stack' },
      typeof message === 'string' ? h('p', null, message) : message,
      input ? h('label', null, `Type ${typeToConfirm} to confirm`, input) : null);
    dialog({
      title, body,
      onClose: () => { if (!done) resolve(false); },
      actions: [
        { label: 'Cancel', kind: 'ghost', onClick: () => { done = true; resolve(false); } },
        {
          label: confirmLabel, kind: danger ? 'danger' : 'primary', onClick: () => {
            if (input && input.value.trim() !== typeToConfirm) { input.focus(); return true; }
            done = true; resolve(true);
          },
        },
      ],
    });
  });
}

// formBox shows fields and resolves their values (or null).
export function formBox({ title, intro, fields, submitLabel = 'Save', wide = false }) {
  return new Promise((resolve) => {
    let done = false;
    const inputs = {};
    const body = h('div', { class: 'stack' }, intro ? h('p', { class: 'muted' }, intro) : null);
    for (const f of fields) {
      let el;
      if (f.type === 'select') {
        el = h('select', null, ...f.options.map((o) => h('option', { value: String(o.value), selected: String(o.value) === String(f.value) }, o.label)));
      } else if (f.type === 'textarea') {
        el = h('textarea', { placeholder: f.placeholder || '' });
        el.value = f.value ?? '';
      } else if (f.type === 'checkbox') {
        el = h('input', { type: 'checkbox', checked: !!f.value });
      } else {
        el = h('input', { type: f.type || 'text', value: f.value ?? '', placeholder: f.placeholder || '', step: f.type === 'number' ? 'any' : undefined, autocomplete: 'off' });
      }
      inputs[f.name] = el;
      body.appendChild(f.type === 'checkbox'
        ? h('label', { class: 'check' }, el, f.label)
        : h('label', null, f.label, el, f.hint ? h('span', { class: 'faint small' }, f.hint) : null));
    }
    const read = () => {
      const out = {};
      for (const f of fields) {
        const el = inputs[f.name];
        if (f.type === 'checkbox') out[f.name] = el.checked;
        else if (f.type === 'number') out[f.name] = el.value === '' ? null : Number(el.value);
        else out[f.name] = el.value;
      }
      return out;
    };
    body.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' && e.target.tagName === 'INPUT') { e.preventDefault(); body.closest('.dialog').querySelector('.dialog-foot button.primary')?.click(); }
    });
    dialog({
      title, body, wide,
      onClose: () => { if (!done) resolve(null); },
      actions: [
        { label: 'Cancel', kind: 'ghost', onClick: () => { done = true; resolve(null); } },
        { label: submitLabel, kind: 'primary', onClick: () => { done = true; resolve(read()); } },
      ],
    });
  });
}

export function copyButton(text, label = 'Copy') {
  const b = button(label, { icon: 'copy', class: 'small' });
  b.addEventListener('click', async () => {
    try { await navigator.clipboard.writeText(typeof text === 'function' ? text() : text); toast('Copied', 'ok', '', 1800); }
    catch { toast('Copy failed: select it and copy by hand', 'warn'); }
  });
  return b;
}

export function empty(text) {
  return h('div', { class: 'empty' }, text);
}

export function card(title, body, ...tools) {
  return h('section', { class: 'card' },
    title ? h('div', { class: 'card-head' }, h('h2', null, title), ...tools) : null,
    h('div', { class: 'card-body' }, body));
}

export function pageHead(title, sub, crumbs, ...actions) {
  return h('div', { class: 'page-head' },
    h('div', { class: 'titles' },
      crumbs ? h('div', { class: 'crumbs' }, crumbs) : null,
      h('h1', null, title),
      sub ? h('p', null, sub) : null),
    actions.length ? h('div', { class: 'actions' }, ...actions) : null);
}

export function debounce(fn, ms) {
  let t;
  return (...a) => { clearTimeout(t); t = setTimeout(() => fn(...a), ms); };
}

export function store(key, value) {
  try {
    if (value === undefined) return localStorage.getItem('eqoa-gm.' + key);
    localStorage.setItem('eqoa-gm.' + key, value);
  } catch { /* private window: nothing kept */ }
  return null;
}
