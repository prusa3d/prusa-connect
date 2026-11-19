// The bundle's entry. The app imports this file and calls what it default
// exports, handing it the one object a module receives.
//
// `defineModule` is an identity function. It exists so an author gets the
// host's types at this call site without importing the host.
import { defineModule } from '@prusa/connect-ui/slots';

import { mayOperate } from './access';
import { materialsOn } from './api';
import { Catalogue, holdCatalogue } from './catalogue.svelte';
import { CatalogueSettings } from './catalogue-settings';
import { LocationsSettings } from './locations-settings';
import { holdHost } from './host';
import { holdList, MaterialList } from './list.svelte';
import { MaterialPage } from './material-page';
import { MaterialsPage } from './materials-page';
import { PrinterMaterial } from './printer-material';
import './styles.css';

export default defineModule((host) => {
  // Everything a component of this module needs is built here, from the host
  // and from nothing else. The host arrives before any component is mounted,
  // so it is held rather than threaded down.
  holdHost(host);
  const service = materialsOn(host);
  holdList(new MaterialList(service, mayOperate(host)), host.signal);
  holdCatalogue(new Catalogue(service), host.signal);

  // host.signal fires when the person who signed in leaves. Every request of
  // host.client ends with it. Pass it to a call that takes a signal, and stop
  // every timer and stream you start when it fires, for example:
  //
  //   const timer = setInterval(refresh, 5000);
  //   host.signal.addEventListener('abort', () => clearInterval(timer));
  //   await client.listThings({}, host.signal);

  // Every registration below is held to `contributes` in module.yaml: a name
  // the manifest does not declare is refused at run time and the app drops
  // this module. The generator wrote both from one answer, so they agree; keep
  // them in step by hand after that.
  host.register.page('materials', MaterialsPage);
  host.register.page('material', MaterialPage);
  host.register.settings('catalogue', CatalogueSettings);
  host.register.settings('locations', LocationsSettings);
  host.register.deviceTab('tools', PrinterMaterial);
  host.register.component('printer-positions', PrinterMaterial);
});
