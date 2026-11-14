// The Economy page (ops_economy.go): tunar made (faucets), spent (sinks) and
// moved between players (transfers), from the ledger the server keeps; the
// trend, the money supply and its spread, who made and spent the most, and
// the lines that say what it means. Charts are inline SVG (no library).

import { h, mount, card, pageHead, button, toast, empty } from '../lib/dom.js';
import { op } from '../lib/api.js';

const NS = 'http://www.w3.org/2000/svg';
function s(tag, attrs = {}, ...kids) {
  const el = document.createElementNS(NS, tag);
  for (const [k, v] of Object.entries(attrs)) if (v !== null && v !== undefined) el.setAttribute(k, v);
  for (const k of kids) if (k) el.appendChild(k);
  return el;
}

// 950, 12.4k, 3.1M
export function tunar(n) {
  if (n === null || n === undefined) return '—';
  const a = Math.abs(n), sign = n < 0 ? '-' : '';
  if (a >= 1e9) return sign + (a / 1e9).toFixed(2) + 'B';
  if (a >= 1e6) return sign + (a / 1e6).toFixed(1) + 'M';
  if (a >= 1e4) return sign + (a / 1e3).toFixed(1) + 'k';
  return sign + Math.round(a).toLocaleString();
}
const signed = (n) => (n > 0 ? '+' : '') + tunar(n);
const full = (n) => (n ?? 0).toLocaleString();

const KIND_LABEL = { faucet: 'Made', sink: 'Spent', transfer: 'Moved' };

// lineChart draws series over points [{t, ...}] on one axis, with a hover
// crosshair and a tooltip listing every value at that point.
function lineChart(points, series, opts) {
  const wrap = h('div', { class: 'econ-chart' });
  // drawn at the width it is given, so its text stays the page's size; again
  // when that width changes
  let drawn = 0;
  const paint = () => {
    const w = Math.round(wrap.clientWidth);
    if (w > 0 && w !== drawn) { drawn = w; drawChart(wrap, points, series, { ...opts, width: w }); }
  };
  requestAnimationFrame(paint);
  if (window.ResizeObserver) new ResizeObserver(paint).observe(wrap);
  return wrap;
}

function drawChart(wrap, points, series, { fmtT, tip, height = 220, width = 720 }) {
  const W = Math.max(width, 280), H = height, L = 56, R = 14, T = 12, B = 26;
  const vals = points.flatMap((p) => series.map((se) => se.get(p)).filter((v) => v !== null && v !== undefined));
  if (!vals.length) {
    mount(wrap, empty('Nothing counted in this range yet.'));
    return;
  }
  let lo = Math.min(0, ...vals), hi = Math.max(0, ...vals);
  if (hi === lo) hi = lo + 1;
  const pad = (hi - lo) * 0.06; hi += pad; if (lo < 0) lo -= pad;
  const x = (i) => L + (points.length === 1 ? (W - L - R) / 2 : (i * (W - L - R)) / (points.length - 1));
  const y = (v) => T + ((hi - v) * (H - T - B)) / (hi - lo);
  const svg = s('svg', { viewBox: `0 0 ${W} ${H}`, width: W, height: H, class: 'econ-svg', role: 'img', 'aria-label': series.map((se) => se.label).join(', ') });
  // grid: 4 lines, recessive
  for (let k = 0; k <= 4; k++) {
    const v = lo + ((hi - lo) * k) / 4;
    svg.appendChild(s('line', { x1: L, x2: W - R, y1: y(v), y2: y(v), class: 'econ-grid' }));
    const tx = s('text', { x: L - 6, y: y(v) + 4, class: 'econ-axis', 'text-anchor': 'end' }); tx.textContent = tunar(Math.round(v));
    svg.appendChild(tx);
  }
  if (lo < 0) svg.appendChild(s('line', { x1: L, x2: W - R, y1: y(0), y2: y(0), class: 'econ-zero' }));
  const every = Math.max(1, Math.ceil(points.length / Math.max(2, Math.floor((W - L - R) / 90))));
  points.forEach((p, i) => {
    const last = i === points.length - 1;
    if (i % every && !last) return;
    if (!last && points.length - 1 - i < every * 0.75) return; // no label over the last one's
    const tx = s('text', { x: x(i), y: H - 8, class: 'econ-axis', 'text-anchor': 'middle' }); tx.textContent = fmtT(p.t);
    svg.appendChild(tx);
  });
  for (const se of series) {
    let d = '', on = false;
    points.forEach((p, i) => {
      const v = se.get(p);
      if (v === null || v === undefined) { if (!se.spanGaps) on = false; return; }
      d += (on ? 'L' : 'M') + x(i).toFixed(1) + ' ' + y(v).toFixed(1);
      on = true;
    });
    if (d) svg.appendChild(s('path', { d, class: 'econ-line ' + se.cls, ...(se.dash ? { 'stroke-dasharray': '4 4' } : {}) }));
    // the last value labelled at the end (selective direct label)
    for (let i = points.length - 1; i >= 0; i--) {
      const v = se.get(points[i]);
      if (v !== null && v !== undefined) {
        svg.appendChild(s('circle', { cx: x(i), cy: y(v), r: 4, class: 'econ-dot ' + se.cls }));
        break;
      }
    }
  }
  const cross = s('line', { y1: T, y2: H - B, class: 'econ-cross', visibility: 'hidden' });
  svg.appendChild(cross);
  const marks = series.map((se) => { const c = s('circle', { r: 4, class: 'econ-dot ' + se.cls, visibility: 'hidden' }); svg.appendChild(c); return c; });
  const tipBox = h('div', { class: 'econ-tip', hidden: true });
  const hit = s('rect', { x: L, y: T, width: W - L - R, height: H - T - B, fill: 'transparent' });
  svg.appendChild(hit);
  const at = (ev) => {
    const r = svg.getBoundingClientRect();
    const px = ((ev.clientX - r.left) / r.width) * W;
    const i = Math.max(0, Math.min(points.length - 1, Math.round(((px - L) / (W - L - R)) * (points.length - 1))));
    const p = points[i];
    cross.setAttribute('x1', x(i)); cross.setAttribute('x2', x(i)); cross.setAttribute('visibility', 'visible');
    series.forEach((se, k) => {
      const v = se.get(p);
      if (v === null || v === undefined) { marks[k].setAttribute('visibility', 'hidden'); return; }
      marks[k].setAttribute('cx', x(i)); marks[k].setAttribute('cy', y(v)); marks[k].setAttribute('visibility', 'visible');
    });
    mount(tipBox, ...tip(p));
    tipBox.hidden = false;
    const left = (x(i) / W) * r.width;
    tipBox.style.left = Math.min(Math.max(left + 12, 0), r.width - 200) + 'px';
  };
  hit.addEventListener('pointermove', at);
  hit.addEventListener('pointerleave', () => { tipBox.hidden = true; cross.setAttribute('visibility', 'hidden'); marks.forEach((m) => m.setAttribute('visibility', 'hidden')); });
  const legend = series.length > 1 ? h('div', { class: 'econ-legend' }, ...series.map((se) => h('span', null, h('i', { class: 'swatch ' + se.cls + (se.dash ? ' dash' : '') }), se.label))) : null;
  mount(wrap, legend, h('div', { class: 'econ-plot' }, svg, tipBox));
}

const hourFmt = (t) => new Date(t * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
const dayFmt = (t) => new Date(t * 1000).toLocaleDateString([], { month: 'short', day: 'numeric', timeZone: 'UTC' });

export async function render(box, id) {
  let range = 'daily';
  let shard = 'here';
  let days = 30;
  let data = null;
  const body = h('div');
  const shardTabs = h('div', { class: 'tabs' });
  mount(box, pageHead('Economy', 'Tunar made, spent and held: every movement counted where it happens, written each minute; the wealth measured each hour. Days are UTC.', null,
    button('Refresh', { icon: 'refresh', onclick: () => load() })), shardTabs, body);

  async function load() {
    try {
      data = await op('economy', { days, shard });
      draw();
    } catch (e) {
      mount(body, h('p', { class: 'notice warn' }, e.message));
    }
  }

  function tabsFor() {
    if (!data?.other) { mount(shardTabs); shardTabs.hidden = true; return; }
    shardTabs.hidden = false;
    const t = (key, label) => h('button', { type: 'button', class: shard === key ? 'on' : '', onclick: () => { shard = key; load(); } }, label);
    mount(shardTabs, t('here', shard === 'here' ? data.shard : 'This shard'), t('other', data.other.name + ' (read-only)'));
  }

  function draw() {
    tabsFor();
    const w = data.windows;
    const wealth = data.wealth;
    const kpi = (label, value, sub, cls = '') => h('div', { class: 'kpi ' + cls }, h('div', { class: 'label' }, label),
      h('div', { class: 'value' }, value), sub ? h('div', { class: 'sub' }, sub) : null);
    const netCls = (n) => (n > 0 ? 'econ-up' : n < 0 ? 'econ-down' : '');
    const kpis = h('div', { class: 'kpis' },
      kpi('Net today', signed(w.today.net), `${tunar(w.today.faucet)} made, ${tunar(w.today.sink)} spent`, netCls(w.today.net)),
      kpi('Net 7 days', signed(w['7d'].net), `${tunar(w['7d'].faucet)} made, ${tunar(w['7d'].sink)} spent`, netCls(w['7d'].net)),
      kpi('Net 30 days', signed(w['30d'].net), `${tunar(w['30d'].faucet)} made, ${tunar(w['30d'].sink)} spent`, netCls(w['30d'].net)),
      kpi('Moved between players, 7 days', tunar(w['7d'].transfer), 'trades, auction sales, duel wagers'),
      kpi('Money supply', wealth ? tunar(wealth.supply) : '—',
        wealth?.week ? `${wealth.week.pct >= 0 ? '+' : ''}${wealth.week.pct}% since ${dayFmt(wealth.week.hour)}` : 'purses, banks, escrow, GMs\' too'),
      kpi('Median character', wealth ? tunar(wealth.latest.median) : '—', wealth ? `${wealth.latest.characters} players' characters` : ''));

    const callouts = h('ul', { class: 'econ-callouts' }, ...(data.callouts || []).map((c) => h('li', null, c)));

    const pts = range === 'hourly' ? data.hourly : data.daily;
    const fmtT = range === 'hourly' ? hourFmt : dayFmt;
    const tipFlow = (p) => [
      h('div', { class: 'econ-tip-t' }, range === 'hourly' ? new Date(p.t * 1000).toLocaleString() : dayFmt(p.t)),
      h('div', null, h('i', { class: 'swatch s-made' }), 'Made ', h('b', null, full(p.faucet))),
      h('div', null, h('i', { class: 'swatch s-spent' }), 'Spent ', h('b', null, full(p.sink))),
      h('div', null, 'Net ', h('b', null, (p.net > 0 ? '+' : '') + full(p.net))),
      h('div', { class: 'faint' }, 'Moved between players ', full(p.transfer))];
    const flow = lineChart(pts, [
      { label: 'Made (faucets)', cls: 's-made', get: (p) => p.faucet },
      { label: 'Spent (sinks)', cls: 's-spent', get: (p) => p.sink },
    ], { fmtT, tip: tipFlow });
    const net = lineChart(pts, [{ label: 'Net (made minus spent)', cls: 's-net', get: (p) => p.net }],
      { fmtT, height: 150, tip: (p) => [h('div', { class: 'econ-tip-t' }, fmtT(p.t)), h('div', null, 'Net ', h('b', null, (p.net > 0 ? '+' : '') + full(p.net)))] });
    const supply = lineChart(pts, [
      { label: 'All tunar held', cls: 's-made', spanGaps: true, get: (p) => p.supply ?? null },
      { label: 'Players\' (no GMs)', cls: 's-spent', dash: true, spanGaps: true, get: (p) => p.playerSupply ?? null },
    ], { fmtT, height: 180, tip: (p) => [h('div', { class: 'econ-tip-t' }, fmtT(p.t)),
      h('div', null, 'All held ', h('b', null, p.supply == null ? '—' : full(p.supply))),
      h('div', null, 'Players\' ', h('b', null, p.playerSupply == null ? '—' : full(p.playerSupply)))] });
    const rangeTabs = h('div', { class: 'tabs econ-range' },
      ...[['hourly', 'Last 48 hours'], ['daily', `Last ${days} days`]].map(([k, l]) => h('button', { type: 'button', class: range === k ? 'on' : '', onclick: () => { range = k; draw(); } }, l)),
      ...(range === 'daily' ? [7, 30, 90].map((d) => h('button', { type: 'button', class: 'small ' + (days === d ? 'on' : ''), onclick: () => { days = d; load(); } }, d + 'd')) : []));

    mount(body, kpis,
      card('What it says', callouts.children.length ? callouts : empty('Nothing to say yet.')),
      rangeTabs,
      card('Tunar made and spent', flow),
      h('div', { class: 'cols-2' }, card('Net: tunar the economy gained', net), card('Money supply', supply)),
      sourcesCard(w),
      h('div', { class: 'cols-2' }, topCard(data.top), wealthCard(wealth)));
  }

  function sourcesCard(w) {
    let pick = 'today';
    const holder = h('div');
    const paint = () => {
      const win = w[pick];
      const kinds = ['faucet', 'sink', 'transfer'];
      const totals = { faucet: win.faucet, sink: win.sink, transfer: win.transfer };
      const tabs = h('div', { class: 'tabs' }, ...[['today', 'Today'], ['7d', '7 days'], ['30d', '30 days']].map(([k, l]) =>
        h('button', { type: 'button', class: pick === k ? 'on' : '', onclick: () => { pick = k; paint(); } }, l)));
      const byKind = (k) => win.sources.filter((e) => e.kind === k);
      const known = new Set(win.sources.map((e) => e.source));
      const rows = [];
      for (const k of kinds) {
        rows.push(h('tr', { class: 'econ-kind' }, h('th', { colspan: 2 }, `${KIND_LABEL[k]} — ${k}s`), h('th', { class: 'num' }, tunar(totals[k])), h('th', { class: 'num' }, ''), h('th', null, '')));
        const list = byKind(k);
        for (const e of list) {
          const help = data.sources.find((x) => x.source === e.source)?.help || '';
          rows.push(h('tr', null,
            h('td', null, h('span', { title: help }, e.label)),
            h('td', { class: 'num faint' }, full(e.count) + '×'),
            h('td', { class: 'num' }, full(e.amount)),
            h('td', { class: 'num' }, e.share + '%'),
            h('td', { class: 'econ-barcell' }, h('div', { class: 'econ-bar ' + (k === 'faucet' ? 's-made' : k === 'sink' ? 's-spent' : 's-moved'), style: `width:${Math.max(1, e.share)}%` }))));
        }
        const none = data.sources.filter((x) => x.kind === k && !known.has(x.source));
        if (none.length) rows.push(h('tr', null, h('td', { colspan: 5, class: 'faint' }, 'None: ' + none.map((x) => x.label).join(', '))));
      }
      mount(holder, tabs,
        h('p', { class: 'faint' }, `Net ${signed(win.net)}: ${tunar(win.faucet)} made, ${tunar(win.sink)} spent (${win.faucet ? Math.round((win.sink * 100) / win.faucet) : 0}% of it), ${tunar(win.transfer)} moved. The ledger covers ${win.days} days of it.`),
        h('div', { class: 'table-scroll' }, h('table', { class: 'plain econ-sources' },
          h('thead', null, h('tr', null, h('th', null, 'Source'), h('th', { class: 'num' }, 'Times'), h('th', { class: 'num' }, 'Tunar'), h('th', { class: 'num' }, 'Share'), h('th', null, ''))),
          h('tbody', null, ...rows))));
    };
    paint();
    return card('Sources', holder);
  }

  function topCard(top) {
    let pick = 'today';
    let list = 'earners';
    const holder = h('div');
    const label = (src) => data.sources.find((x) => x.source === src)?.label || src;
    const paint = () => {
      const rows = top[pick][list] || [];
      const tabs = h('div', { class: 'tabs' },
        ...[['earners', 'Earners'], ['spenders', 'Spenders'], ['traders', 'Traders']].map(([k, l]) => h('button', { type: 'button', class: list === k ? 'on' : '', onclick: () => { list = k; paint(); } }, l)),
        ...[['today', 'Today'], ['7d', '7 days']].map(([k, l]) => h('button', { type: 'button', class: 'small ' + (pick === k ? 'on' : ''), onclick: () => { pick = k; paint(); } }, l)));
      const main = (e) => list === 'earners' ? e.made : list === 'spenders' ? e.spent : e.in + e.out;
      const srcs = (e) => Object.entries(e.sources).filter(([, n]) => n !== 0).sort((a, b) => Math.abs(b[1]) - Math.abs(a[1])).slice(0, 3)
        .map(([k, n]) => `${label(k)} ${signed(n)}`).join(' · ');
      mount(holder, tabs, rows.length
        ? h('div', { class: 'table-scroll' }, h('table', { class: 'plain' },
          h('thead', null, h('tr', null, h('th', null, 'Character'), h('th', { class: 'num' }, list === 'traders' ? 'In / out' : 'Tunar'), h('th', { class: 'num' }, 'Net'), h('th', null, 'Mostly'))),
          h('tbody', null, ...rows.map((e) => h('tr', null,
            h('td', null, shard === 'here' ? h('a', { href: '#/characters/' + e.serverid }, e.name || ('#' + e.serverid)) : (e.name || ('#' + e.serverid)), e.staff ? h('span', { class: 'badge' }, 'GM') : null),
            h('td', { class: 'num' }, list === 'traders' ? `${tunar(e.in)} / ${tunar(e.out)}` : full(main(e))),
            h('td', { class: 'num' }, signed(e.net)),
            h('td', { class: 'faint' }, srcs(e)))))))
        : empty('Nobody yet.'));
    };
    paint();
    return card('Who made and spent the most', holder, h('span', { class: 'faint' }, `${top[pick]?.active ?? 0} characters active`));
  }

  function wealthCard(wealth) {
    if (!wealth) return card('Wealth', empty('No snapshot yet: the first is taken within a minute of the server starting.'));
    const l = wealth.latest;
    const players = l.purse + l.bank;
    const dl = h('dl', { class: 'kv' },
      h('dt', null, 'Measured'), h('dd', null, new Date(l.hour * 1000).toLocaleString()),
      h('dt', null, 'All tunar'), h('dd', null, full(wealth.supply), wealth.day ? h('span', { class: 'faint' }, ` (${wealth.day.pct >= 0 ? '+' : ''}${wealth.day.pct}% in a day)`) : null),
      h('dt', null, 'Players: purses'), h('dd', null, full(l.purse)),
      h('dt', null, 'Players: banks'), h('dd', null, full(l.bank)),
      h('dt', null, 'Auction escrow'), h('dd', null, full(l.escrow)),
      h('dt', null, 'GMs\' characters'), h('dd', null, full(l.staff), h('span', { class: 'faint' }, ' (left out of the spread)')),
      h('dt', null, 'Median / p90 / p99'), h('dd', null, `${tunar(l.median)} / ${tunar(l.p90)} / ${tunar(l.p99)}`),
      h('dt', null, 'The 10 richest'), h('dd', null, `${tunar(l.top10)} (${players ? Math.round((l.top10 * 100) / players) : 0}% of players' tunar)`),
      h('dt', null, 'Over 100k / 1M / 10M'), h('dd', null, `${l.over100k} / ${l.over1m} / ${l.over10m} of ${l.characters}`));
    const richest = h('table', { class: 'plain' },
      h('thead', null, h('tr', null, h('th', null, 'Richest'), h('th', { class: 'num' }, 'Purse'), h('th', { class: 'num' }, 'Bank'), h('th', { class: 'num' }, 'Total'))),
      h('tbody', null, ...(l.top || []).slice(0, 10).map((t) => h('tr', null,
        h('td', null, shard === 'here' ? h('a', { href: '#/characters/' + t.serverid }, t.name) : t.name, ' ', h('span', { class: 'faint' }, 'L' + t.level), t.staff ? h('span', { class: 'badge' }, 'GM') : null),
        h('td', { class: 'num' }, tunar(t.purse)), h('td', { class: 'num' }, tunar(t.bank)), h('td', { class: 'num' }, tunar(t.total))))));
    return card('Wealth', h('div', { class: 'stack' }, dl, h('div', { class: 'table-scroll' }, richest)));
  }

  await load();
  const timer = setInterval(load, 60000);
  return { leave: () => clearInterval(timer) };
}
