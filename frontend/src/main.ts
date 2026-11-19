import { ApiError, NetworkError } from '@prusa/connect';
import { mount } from 'svelte';

import App from './app.svelte';
import { startLocale } from './lib/i18n';
import { log } from './lib/log';
import './styles/global.css';

// The language is chosen before the first screen draws, so nothing flashes in
// English and is then replaced (frontend-i18n.md rule 8).
startLocale();

const target = document.getElementById('app');
if (target === null) throw new Error('the page has no #app');

const pageLog = log.scope('page');

window.addEventListener('error', (event) => {
  pageLog.error(event.message, {
    stack: event.error instanceof Error ? event.error.stack : undefined,
  });
});
window.addEventListener('unhandledrejection', (event) => {
  const reason: unknown = event.reason;
  // The client's onFailure has already recorded these.
  if (reason instanceof ApiError || reason instanceof NetworkError) return;
  pageLog.error(reason instanceof Error ? reason.message : String(reason), {
    stack: reason instanceof Error ? reason.stack : undefined,
  });
});

mount(App, { target, props: { useServerAppearance: true } });
