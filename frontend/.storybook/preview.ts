// What every story shares: the library's theme switch, the English locale
// with the app's own strings registered, and the app's stylesheet.
import type { Decorator, Preview } from '@storybook/svelte';

import library from '../../packages/svelte/.storybook/preview';
// After the library's preview, which brings its own Tailwind build: the app's
// sheet comes later in the cascade, so its utilities win where both define a
// class, as `.hidden` against a container-query `block`.
import '../src/styles/global.css';
import { startLocale } from '../src/lib/i18n';
import FleetFrame from './fleet-frame.svelte';

startLocale(['en']);

const withFilaments: Decorator = () => ({ Component: FleetFrame });

const preview: Preview = {
  ...library,
  decorators: [...(library.decorators as Decorator[]), withFilaments],
  parameters: {
    options: {
      storySort: {
        order: ['Tour', 'Screens', 'Parts', 'Shell', 'Primitives'],
      },
    },
  },
};

export default preview;
