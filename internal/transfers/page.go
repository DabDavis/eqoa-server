package transfers

// page is the transfer page: sign in, the form, the account's transfers. Every
// value from the server goes into the page as text (textContent), never as
// markup. Its routes are relative to where it is served: /eqoa/transfer on the
// public site (nginx), /transfer on the account server itself.
const page = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Bring a character over - EQOA</title>
<style>
  * { box-sizing: border-box; }
  body { font-family: 'Segoe UI', Arial, sans-serif; background: #1a1a2e; color: #e8e8e8; margin: 0; padding: 24px 12px; }
  main { max-width: 720px; margin: 0 auto; }
  h1 { color: #e0c070; font-size: 1.6em; margin: 0 0 8px; }
  h2 { color: #e0c070; font-size: 1.15em; margin: 0 0 12px; }
  .card { background: #16213e; border: 1px solid #2a3a5e; border-radius: 8px; padding: 18px; margin: 16px 0; }
  p, li { line-height: 1.5; color: #c8c8d8; }
  ol { padding-left: 20px; margin: 8px 0; }
  label { display: block; margin: 10px 0 4px; font-size: .92em; color: #b8b8c8; }
  input, select, textarea { width: 100%; padding: 8px 10px; background: #0f1830; color: #f0f0f0; border: 1px solid #34466e;
    border-radius: 5px; font: inherit; }
  textarea { min-height: 110px; resize: vertical; }
  .row { display: flex; gap: 12px; flex-wrap: wrap; }
  .row > div { flex: 1 1 140px; }
  button { margin-top: 14px; padding: 9px 18px; background: #c9a646; color: #1a1a2e; border: 0; border-radius: 5px; font-weight: 600;
    cursor: pointer; font: inherit; }
  button.ghost { background: transparent; color: #c9a646; border: 1px solid #c9a646; padding: 5px 12px; margin: 0; }
  button:disabled { opacity: .6; cursor: default; }
  .msg { min-height: 1.2em; margin-top: 10px; }
  .err { color: #ff8a80; }
  .ok { color: #8be78b; }
  .hidden { display: none; }
  .who { display: flex; justify-content: space-between; align-items: center; gap: 8px; flex-wrap: wrap; }
  .t { border-top: 1px solid #2a3a5e; padding: 10px 0; }
  .t:first-child { border-top: 0; }
  .status { font-weight: 600; text-transform: capitalize; }
  .status.waiting, .status.approved { color: #e0c070; }
  .status.applied { color: #8be78b; }
  .status.rejected, .status.undone { color: #ff8a80; }
  .status.cancelled { color: #999; }
  pre { white-space: pre-wrap; margin: 6px 0; font-family: inherit; color: #c8c8d8; }
  .small { font-size: .88em; color: #9aa0b8; }
</style>
</head>
<body>
<main>
  <h1>Bring a character over</h1>
  <div class="card">
    <p>Played on another server, Sandstorm or elsewhere? Ask for your character here and a GM brings it across.</p>
    <ol>
      <li>Make the character on this server first, in the game: the same class, and the race and looks you want.</li>
      <li>Sign in below with this server's account and tell us what the character was: its level, its combat mastery points
        and its gear.</li>
      <li>A GM checks it. Once approved, it arrives the next time you log that character in: its level, its class's spells,
        its mastery points to spend again, and its gear in your pack (the bank when the pack is full).</li>
    </ol>
    <p class="small">Items this server has no record of are made by a GM as close as we can; tell a GM in the game (type
      /bug) if one is off. Quests are not brought over.</p>
  </div>

  <div class="card" id="signin">
    <h2>Sign in</h2>
    <form id="loginForm">
      <label for="u">Account name (this server's)</label>
      <input id="u" autocomplete="username" required maxlength="32">
      <label for="p">Password</label>
      <input id="p" type="password" autocomplete="current-password" required maxlength="64">
      <button type="submit">Sign in</button>
      <div class="msg" id="loginMsg" role="alert"></div>
    </form>
  </div>

  <div id="signedIn" class="hidden">
    <div class="card who">
      <span>Signed in as <strong id="acct"></strong></span>
      <button class="ghost" id="logout" type="button">Sign out</button>
    </div>

    <div class="card">
      <h2>Ask for a transfer</h2>
      <form id="form">
        <label for="char">Your character on this server</label>
        <select id="char" required></select>
        <p class="small" id="noChars" hidden>You have no character on this server yet: make one in the game first (the same class as the
          one you are bringing), then come back.</p>
        <div class="row">
          <div><label for="fromServer">From server</label><input id="fromServer" value="Sandstorm" maxlength="32" required></div>
          <div><label for="fromName">Its name there</label><input id="fromName" maxlength="32" required></div>
        </div>
        <div class="row">
          <div><label for="fromClass">Its class there</label><select id="fromClass" required></select></div>
          <div><label for="fromLevel">Its level</label><input id="fromLevel" type="number" min="1" max="60" required></div>
          <div><label for="fromCM">Combat mastery points</label><input id="fromCM" type="number" min="0" max="1500" value="0"></div>
        </div>
        <label for="items">Its gear, one item per line (write "x2" after an item you had two of)</label>
        <textarea id="items" maxlength="4000" placeholder="The Rotting Fist&#10;Slayer's Claw Earring x2&#10;Tower of Woe"></textarea>
        <label for="notes">Anything else a GM should know (a link to a screenshot helps)</label>
        <textarea id="notes" maxlength="2000"></textarea>
        <button type="submit" id="send">Send to the GMs</button>
        <div class="msg" id="formMsg" role="alert"></div>
      </form>
    </div>

    <div class="card">
      <h2>Your transfers</h2>
      <div id="list"><p class="small">None yet.</p></div>
    </div>
  </div>
</main>
<script>
(function () {
  'use strict';
  var base = location.pathname.indexOf('/eqoa/') === 0 ? '/eqoa/account/api/transfer/' : '/api/transfer/';
  var classes = ['Warrior', 'Ranger', 'Paladin', 'Shadow Knight', 'Monk', 'Bard', 'Rogue', 'Druid', 'Shaman', 'Cleric',
    'Magician', 'Necromancer', 'Enchanter', 'Wizard', 'Alchemist'];
  var $ = function (id) { return document.getElementById(id); };
  function el(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text !== undefined && text !== null) e.textContent = String(text);
    return e;
  }
  function call(path, body) {
    var opts = { credentials: 'same-origin', headers: {} };
    if (body !== undefined) {
      opts.method = 'POST';
      opts.headers['Content-Type'] = 'application/json';
      opts.headers['X-EQOA-Transfer'] = '1';
      opts.body = JSON.stringify(body);
    }
    return fetch(base + path, opts).then(function (r) {
      return r.json().catch(function () { return { ok: false, error: 'the server did not answer: try again later' }; });
    });
  }
  function say(id, text, ok) {
    var m = $(id);
    m.textContent = text || '';
    m.className = 'msg ' + (ok ? 'ok' : 'err');
  }
  classes.forEach(function (c) { $('fromClass').appendChild(el('option', null, c)); });

  function render(state) {
    $('signin').classList.add('hidden');
    $('signedIn').classList.remove('hidden');
    $('acct').textContent = state.account;
    var sel = $('char');
    sel.textContent = '';
    (state.characters || []).forEach(function (c) {
      var o = el('option', null, c.name + ' - level ' + c.level + ' ' + c.race + ' ' + c.class);
      o.value = c.serverid;
      sel.appendChild(o);
    });
    $('noChars').hidden = (state.characters || []).length > 0;
    $('send').disabled = (state.characters || []).length === 0;
    var list = $('list');
    list.textContent = '';
    if (!(state.transfers || []).length) { list.appendChild(el('p', 'small', 'None yet.')); return; }
    state.transfers.forEach(function (t) {
      var d = el('div', 't');
      var head = el('div');
      head.appendChild(el('strong', null, t.from_name + ' (' + t.from_server + ', level ' + t.from_level + ' ' + t.from_class + ')'));
      head.appendChild(document.createTextNode(' to ' + t.charname + ': '));
      head.appendChild(el('span', 'status ' + t.status, t.status === 'approved' ? 'approved, arrives at your next login' : t.status));
      d.appendChild(head);
      d.appendChild(el('div', 'small', 'Asked ' + new Date(t.filed).toLocaleString()));
      if (t.answer) { d.appendChild(el('div', 'small', 'A GM says:')); d.appendChild(el('pre', null, t.answer)); }
      if (t.status === 'waiting') {
        var b = el('button', 'ghost', 'Take it back');
        b.type = 'button';
        b.onclick = function () {
          b.disabled = true;
          call('cancel', { id: t.id }).then(function (r) { if (!r.ok) { alert(r.error); } load(); });
        };
        d.appendChild(b);
      }
      list.appendChild(d);
    });
  }
  function load() {
    return call('state').then(function (s) {
      if (s.ok) { render(s); } else {
        $('signedIn').classList.add('hidden');
        $('signin').classList.remove('hidden');
      }
    });
  }
  $('loginForm').addEventListener('submit', function (e) {
    e.preventDefault();
    say('loginMsg', '');
    call('login', { username: $('u').value, password: $('p').value }).then(function (r) {
      if (!r.ok) { say('loginMsg', r.error); return; }
      $('p').value = '';
      load();
    });
  });
  $('logout').addEventListener('click', function () { call('logout', {}).then(load); });
  $('form').addEventListener('submit', function (e) {
    e.preventDefault();
    say('formMsg', '');
    $('send').disabled = true;
    call('submit', {
      serverid: Number($('char').value), from_server: $('fromServer').value, from_name: $('fromName').value,
      from_class: $('fromClass').value, from_level: Number($('fromLevel').value), from_masteries: Number($('fromCM').value || 0),
      items: $('items').value, notes: $('notes').value
    }).then(function (r) {
      $('send').disabled = false;
      if (!r.ok) { say('formMsg', r.error); return; }
      say('formMsg', 'Sent: a GM will look at it. Its status is below.', true);
      $('items').value = ''; $('notes').value = ''; $('fromName').value = '';
      load();
    });
  });
  load();
})();
</script>
</body>
</html>
`
