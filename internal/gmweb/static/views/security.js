import { h, mount, pageHead, button, toast, card, copyButton } from '../lib/dom.js';
import { api } from '../lib/api.js';
import { state } from '../lib/nav.js';

export async function render(box) {
  const body = h('div', { class: 'stack' });
  mount(box, pageHead('Security', 'A second step at sign-in: the six-digit code from an authenticator app (Google Authenticator, Aegis, 1Password, Authy...).'),
    card('Authenticator', body));
  const draw = async () => {
    const s = await api('api/auth/session');
    state.session.totp = s.totp;
    state.session.user = s.user;
    if (s.totp.enabled) {
      const code = h('input', { inputmode: 'numeric', maxlength: '7', placeholder: '123456' });
      mount(body, h('p', { class: 'notice ok' }, 'On: signing in asks for a code, and so does an AI agent\'s sign-in.'),
        s.user.adminNeedsTOTP ? h('p', { class: 'notice warn' }, 'Sign out and in again with a code to use an admin\'s tools.') : null,
        h('div', { class: 'row' }, h('label', null, 'A current code, to remove it', code),
          button('Remove the authenticator', { class: 'danger', onclick: async () => {
            try { await api('api/auth/totp', { action: 'disable', code: code.value }); toast('Removed', 'ok'); draw(); } catch (e) { toast(e.message, 'error'); }
          } })));
      return;
    }
    mount(body,
      s.totp.required ? h('p', { class: 'notice warn' }, 'This server asks every GM for one: until it is on, the panel does nothing else.') : h('p', null, 'Off. Turning it on protects this account if its password is ever seen.'),
      s.user.adminNeedsTOTP ? h('p', { class: 'notice warn' }, 'You are an admin: an admin\'s tools (SQL, settings, backups, restart, account levels) need a sign-in with a code, so they wait for this.') : null,
      button('Set it up', { class: 'primary', onclick: async () => {
        try {
          const out = await api('api/auth/totp', { action: 'begin' });
          const code = h('input', { inputmode: 'numeric', maxlength: '7', placeholder: '123456', autocomplete: 'one-time-code' });
          mount(body,
            h('ol', { class: 'steps' },
              h('li', null, 'In the app, add an account by entering a setup key (or open the link on your phone):'),
              h('li', null, h('div', { class: 'secret-box' }, h('code', null, out.secret.replace(/(.{4})/g, '$1 ').trim()), copyButton(out.secret))),
              h('li', null, h('a', { href: out.uri }, 'Open in the authenticator app'), h('span', { class: 'muted' }, ' (on the phone that has it)')),
              h('li', null, 'Enter the code it shows:')),
            h('div', { class: 'row' }, code, button('Turn it on', { class: 'primary', onclick: async () => {
              try {
                await api('api/auth/totp', { action: 'confirm', code: code.value });
                toast('Authenticator on', 'ok');
                const was = state.session.user.admin;
                await draw();
                if (!was && state.session.user.admin) location.reload(); // the admin's pages appear
              } catch (e) { toast(e.message, 'error'); }
            } })));
          code.focus();
        } catch (e) { toast(e.message, 'error'); }
      } }));
  };
  await draw();
}
