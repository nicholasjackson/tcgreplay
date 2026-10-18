// Sign-in state of a hosted board. The hosted playmat answers
// /auth/session with the signed-in account (or 401 when signed out); the
// desktop playmat has no such route, so the board shows no account there.
import { app } from './store.svelte.js';

// loadSession reads who is signed in. It returns 'signed-in', 'signed-out'
// (hosted, but the sign-in has ended) or 'none' (the desktop, or the
// server could not be reached).
export async function loadSession() {
  let r;
  try {
    r = await fetch('/auth/session', { credentials: 'same-origin', cache: 'no-store' });
  } catch {
    return 'none';
  }
  if (r.status === 200) {
    try {
      app.account = (await r.json()).email || null;
    } catch {
      app.account = null;
    }
    return 'signed-in';
  }
  if (r.status === 401) return 'signed-out';
  app.account = null;
  return 'none';
}

// signIn sends the browser through sign-in and back to this page.
export function signIn() {
  location.assign('/auth/login?next=' + encodeURIComponent(location.pathname + location.search));
}
