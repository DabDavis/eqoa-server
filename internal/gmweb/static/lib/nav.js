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
