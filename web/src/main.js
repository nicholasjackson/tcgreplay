import { mount } from 'svelte';
import './app.css';
import App from './App.svelte';
import { connect } from './lib/ws.js';
import { loadSession } from './lib/session.js';

loadSession();
connect();
export default mount(App, { target: document.getElementById('app') });
