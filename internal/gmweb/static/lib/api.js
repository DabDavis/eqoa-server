// Talking to the panel's server: every change carries the session's CSRF
// token; a 401 anywhere means the session ended and the sign-in comes back.

let csrf = '';
let onSignedOut = () => {};

export function setCSRF(token) { csrf = token || ''; }
export function onUnauthorized(fn) { onSignedOut = fn; }

export class ApiError extends Error {
  constructor(status, message, data) {
    super(message);
    this.status = status;
    this.data = data;
  }
}

export async function api(path, body, method = body === undefined ? 'GET' : 'POST') {
  const opts = { method, headers: {}, credentials: 'same-origin' };
  if (method !== 'GET') {
    opts.headers['Content-Type'] = 'application/json';
    opts.headers['X-CSRF-Token'] = csrf;
    opts.body = JSON.stringify(body ?? {});
  }
  let res;
  try {
    res = await fetch(path, opts);
  } catch (e) {
    throw new ApiError(0, 'The server cannot be reached (' + e.message + ')');
  }
  let data = null;
  const text = await res.text();
  try { data = text ? JSON.parse(text) : null; } catch { data = { error: text }; }
  if (!res.ok) {
    if (res.status === 401 && !path.startsWith('api/auth/')) onSignedOut();
    throw new ApiError(res.status, (data && data.error) || res.statusText, data);
  }
  return data;
}

// op calls an operation: the same ones an AI agent calls.
export function op(name, args) {
  return api('api/op/' + name, args || {});
}
