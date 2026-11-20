import { mount } from 'svelte';
// The shared design tokens (internal/design) come before the board's own styles.
import '../../internal/design/tokens.css';
import './app.css';
import App from './App.svelte';
import { connect } from './lib/ws.js';
import { loadSession } from './lib/session.js';

loadSession();
connect();
export default mount(App, { target: document.getElementById('app') });
