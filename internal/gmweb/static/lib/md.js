// Markdown as elements: what people and their agents write on the work board
// (views/work.js), built with createElement and text nodes only, so nothing
// they write is ever parsed as HTML. Headings, paragraphs, lists (nested by
// indent, task boxes), fenced code, quotes, rules and pipe tables; in a line,
// code, bold, italics, strike-through and links (http, https, mailto and the
// panel's own #/ pages: any other address stays text).

import { h } from './dom.js';

const LIST = /^( *)([-*+]|\d{1,9}[.)])(?: +(.*))?$/;
const FENCE = /^ {0,3}(`{3,}|~{3,})\s*([\w+.-]*)\s*$/;
const HEADING = /^ {0,3}(#{1,6})\s+(.*?)\s*#*\s*$/;
const RULE = /^ {0,3}([-*_])(?:\s*\1){2,}\s*$/;
const QUOTE = /^ {0,3}>/;
const TABLE_SEP = /^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$/;

// markdown is the text as a div.md of elements.
export function markdown(text, cls = 'md') {
  const box = h('div', { class: cls });
  const lines = String(text ?? '').replace(/\r\n?/g, '\n').replace(/\t/g, '    ').split('\n');
  for (const el of blocks(lines)) box.appendChild(el);
  return box;
}

const indentOf = (l) => l.length - l.trimStart().length;

function startsBlock(l) {
  return FENCE.test(l) || HEADING.test(l) || RULE.test(l) || QUOTE.test(l) || LIST.test(l);
}

function blocks(lines) {
  const out = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (!line.trim()) { i++; continue; }
    let m = line.match(FENCE);
    if (m) {
      const fence = m[1];
      const body = [];
      for (i++; i < lines.length && !lines[i].trim().startsWith(fence); i++) body.push(lines[i]);
      i++;
      out.push(h('pre', { class: 'md-code' }, h('code', null, body.join('\n'))));
      continue;
    }
    m = line.match(HEADING);
    if (m) {
      out.push(h('h' + Math.min(6, m[1].length + 2), null, ...inline(m[2])));
      i++;
      continue;
    }
    if (RULE.test(line)) { out.push(h('hr')); i++; continue; }
    if (QUOTE.test(line)) {
      const body = [];
      for (; i < lines.length && QUOTE.test(lines[i]); i++) body.push(lines[i].replace(/^ {0,3}> ?/, ''));
      out.push(h('blockquote', null, ...blocks(body)));
      continue;
    }
    if (LIST.test(line)) {
      const [el, next] = list(lines, i);
      out.push(el);
      i = next;
      continue;
    }
    if (line.includes('|') && i + 1 < lines.length && TABLE_SEP.test(lines[i + 1]) && lines[i + 1].includes('-')) {
      const [el, next] = table(lines, i);
      out.push(el);
      i = next;
      continue;
    }
    const para = [];
    for (; i < lines.length && lines[i].trim() && (para.length === 0 || !startsBlock(lines[i])); i++) para.push(lines[i]);
    out.push(h('p', null, ...paragraph(para)));
  }
  return out;
}

// paragraph is a paragraph's lines in a line: joined by spaces, or broken
// where a line ends in two spaces or a backslash.
function paragraph(lines) {
  const out = [];
  lines.forEach((l, n) => {
    const hard = n < lines.length - 1 && (/ {2,}$/.test(l) || /\\$/.test(l));
    out.push(...inline(l.trim().replace(/\\$/, '')));
    if (n < lines.length - 1) out.push(hard ? h('br') : ' ');
  });
  return out;
}

function list(lines, start) {
  const first = lines[start].match(LIST);
  const indent = first[1].length;
  const ordered = /\d/.test(first[2]);
  const el = h(ordered ? 'ol' : 'ul');
  if (ordered && parseInt(first[2], 10) !== 1) el.setAttribute('start', String(parseInt(first[2], 10)));
  let i = start;
  while (i < lines.length) {
    if (!lines[i].trim()) {
      // a blank line between two items of the list keeps it going
      let j = i;
      while (j < lines.length && !lines[j].trim()) j++;
      const nm = j < lines.length && lines[j].match(LIST);
      if (nm && nm[1].length === indent && /\d/.test(nm[2]) === ordered) { i = j; continue; }
      break;
    }
    const m = lines[i].match(LIST);
    if (!m || m[1].length !== indent || /\d/.test(m[2]) !== ordered) break;
    const inner = indent + m[2].length + 1;
    const body = [m[3] || ''];
    for (i++; i < lines.length; i++) {
      const l = lines[i];
      if (!l.trim()) {
        let j = i;
        while (j < lines.length && !lines[j].trim()) j++;
        if (j < lines.length && indentOf(lines[j]) >= inner) { body.push(''); continue; }
        break;
      }
      if (indentOf(l) > indent) { body.push(l.slice(Math.min(indentOf(l), inner))); continue; }
      if (startsBlock(l)) break;
      body.push(l.trim()); // a line carrying on the item's text
    }
    const li = h('li');
    const kids = blocks(body);
    if (kids[0] && kids[0].tagName === 'P') {
      // a task box: - [ ] or - [x]
      const t = kids[0].firstChild;
      if (t && t.nodeType === 3) {
        const tm = t.nodeValue.match(/^\[([ xX])\] /);
        if (tm) {
          t.nodeValue = t.nodeValue.slice(4);
          kids[0].insertBefore(h('span', { class: 'md-task' + (tm[1] === ' ' ? '' : ' done'), 'aria-label': tm[1] === ' ' ? 'to do' : 'done' }, tm[1] === ' ' ? '☐ ' : '☑ '), t);
        }
      }
    }
    if (kids.length === 1 && kids[0].tagName === 'P') {
      while (kids[0].firstChild) li.appendChild(kids[0].firstChild);
    } else {
      for (const k of kids) li.appendChild(k);
    }
    el.appendChild(li);
  }
  return [el, i];
}

function cells(line) {
  let t = line.trim();
  if (t.startsWith('|')) t = t.slice(1);
  if (t.endsWith('|') && !t.endsWith('\\|')) t = t.slice(0, -1);
  return t.split(/(?<!\\)\|/).map((c) => c.trim().replace(/\\\|/g, '|'));
}

function table(lines, start) {
  const head = cells(lines[start]);
  const aligns = cells(lines[start + 1]).map((c) => (c.endsWith(':') ? (c.startsWith(':') ? 'center' : 'right') : ''));
  const rows = [];
  let i = start + 2;
  for (; i < lines.length && lines[i].trim() && lines[i].includes('|'); i++) rows.push(cells(lines[i]));
  const cell = (tag, text, n) => {
    const c = h(tag, null, ...inline(text || ''));
    if (aligns[n]) c.style.textAlign = aligns[n];
    return c;
  };
  return [h('div', { class: 'table-scroll' }, h('table', { class: 'plain md-table' },
    h('thead', null, h('tr', null, ...head.map((t, n) => cell('th', t, n)))),
    h('tbody', null, ...rows.map((r) => h('tr', null, ...head.map((_, n) => cell('td', r[n], n))))))), i];
}

// link is an anchor to a safe address, or null.
function link(url, kids) {
  const u = String(url || '').trim();
  if (!/^(https?:\/\/|mailto:)/i.test(u) && !u.startsWith('#/')) return null;
  const out = !u.startsWith('#');
  return h('a', { href: u, target: out ? '_blank' : undefined, rel: out ? 'noopener noreferrer' : undefined }, ...kids);
}

// close is where a run of mark closes s from i on, or -1 (an escaped mark
// does not close).
function close(s, mark, i) {
  for (let j = s.indexOf(mark, i); j >= 0; j = s.indexOf(mark, j + 1)) {
    if (s[j - 1] !== '\\') return j;
  }
  return -1;
}

function bracket(s, i) {
  let depth = 0;
  for (let j = i; j < s.length; j++) {
    if (s[j] === '\\') { j++; continue; }
    if (s[j] === '[') depth++;
    else if (s[j] === ']' && --depth === 0) return j;
  }
  return -1;
}

// inline is a line's text as nodes.
function inline(s) {
  const out = [];
  let buf = '';
  const flush = () => { if (buf) { out.push(document.createTextNode(buf)); buf = ''; } };
  const word = (c) => !!c && /[\p{L}\p{N}_]/u.test(c);
  let i = 0;
  while (i < s.length) {
    const c = s[i];
    if (c === '\\' && i + 1 < s.length && /[\\`*_{}[\]()#+\-.!~|<>]/.test(s[i + 1])) { buf += s[i + 1]; i += 2; continue; }
    if (c === '`') {
      let n = 1;
      while (s[i + n] === '`') n++;
      const end = s.indexOf('`'.repeat(n), i + n);
      if (end > 0) {
        flush();
        let code = s.slice(i + n, end);
        if (/^ .* $/.test(code)) code = code.slice(1, -1);
        out.push(h('code', null, code));
        i = end + n;
        continue;
      }
      buf += '`'.repeat(n);
      i += n;
      continue;
    }
    if ((c === '*' || c === '_' || c === '~') && s[i + 1] === c && s[i + 2] && s[i + 2] !== ' ' && !(c === '_' && word(s[i - 1]))) {
      const end = close(s, c + c, i + 2);
      if (end > i + 2 && s[end - 1] !== ' ') {
        flush();
        out.push(h(c === '~' ? 's' : 'strong', null, ...inline(s.slice(i + 2, end))));
        i = end + 2;
        continue;
      }
    }
    if ((c === '*' || c === '_') && s[i + 1] && s[i + 1] !== ' ' && s[i + 1] !== c && !(c === '_' && word(s[i - 1]))) {
      const end = close(s, c, i + 1);
      if (end > i + 1 && s[end - 1] !== ' ' && !(c === '_' && word(s[end + 1]))) {
        flush();
        out.push(h('em', null, ...inline(s.slice(i + 1, end))));
        i = end + 1;
        continue;
      }
    }
    if (c === '[') {
      const end = bracket(s, i);
      if (end > 0 && s[end + 1] === '(') {
        const pe = s.indexOf(')', end + 2);
        if (pe > 0) {
          const a = link(s.slice(end + 2, pe).trim().split(/\s+/)[0], inline(s.slice(i + 1, end)));
          if (a) { flush(); out.push(a); i = pe + 1; continue; }
        }
      }
    }
    if (c === '<') {
      const end = s.indexOf('>', i + 1);
      const u = end > 0 ? s.slice(i + 1, end) : '';
      if (/^(https?:\/\/|mailto:)\S+$/i.test(u)) { flush(); out.push(link(u, [u])); i = end + 1; continue; }
    }
    if ((c === 'h' || c === 'H') && (i === 0 || /[\s(]/.test(s[i - 1]))) {
      const m = s.slice(i).match(/^https?:\/\/[^\s<>]*[^\s<>().,;:!?'"*_]/i);
      if (m) { flush(); out.push(link(m[0], [m[0]])); i += m[0].length; continue; }
    }
    buf += c;
    i++;
  }
  flush();
  return out;
}
