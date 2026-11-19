// The bundle's entry. The app imports this file and calls what it default
// exports, handing it the one object a module receives.
import { defineModule } from '@prusa/connect-ui/slots';

import { holdHost } from './host';
import Settings from './settings.svelte';
import './styles.css';

export default defineModule((host) => {
  // The host arrives before any component is mounted, so it is held rather
  // than threaded down.
  holdHost(host);

  // Held to `contributes` in module.yaml: a name the manifest does not
  // declare is refused at run time and the app drops this module.
  host.register.settings('easyprint', Settings);
});
