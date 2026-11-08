// What the views share with the frame: who is signed in, and moving to a page.
// (A view must not import app.js: the page loads it with the build in its
// address, so a second import by another name would start the panel twice.)

export const state = { session: null };

export function go(path) {
  location.hash = '#/' + path;
}

export function isAdmin() {
  return !!state.session?.user?.admin;
}

// Below a GM, the roles the account holds (roles.go): a tester (the item and
// spell records, tester.go), a storyteller (NPC dialog and the quests' words,
// storyteller.go), or both ("tester+storyteller"), each with only the pages of
// its own.
export function roles() {
  const r = state.session?.user?.role || '';
  return r === 'gm' || r === 'admin' ? [] : r.split(/[+ ,]/).filter(Boolean);
}

export function isLimited() {
  const r = state.session?.user?.role;
  return !!state.session && r !== 'gm' && r !== 'admin';
}

export function isTester() {
  return roles().includes('tester');
}

export function isStoryteller() {
  return roles().includes('storyteller');
}
