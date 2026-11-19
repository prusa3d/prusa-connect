<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import {
    catalogueOf,
    inState,
    instanceOf,
    noop,
    offerOf,
  } from '../../../../test/extension-fixtures';
  import { extensionRowsOf } from '../rows';
  import ExtensionsListing from './extensions-listing.svelte';

  const { Story } = defineMeta({
    title: 'Parts/Extensions/Extensions Listing',
    component: ExtensionsListing,
  });

  const catalogue = catalogueOf();
  const shows = () => true;

  const tracker = offerOf({
    trust: 'official',
    installed: true,
    installed_version: '1.4.0',
    instance: 'driver:spool-tracker',
  });
  const update = offerOf({
    module: 'github.com/prusa3d/fleet-stats',
    name: 'fleet-stats',
    title: 'Fleet stats',
    description: 'Shows how busy the fleet was this week.',
    namespace: 'com.github.prusa3d.fleet-stats',
    version: '2.1.0',
    installed: true,
    installed_version: '2.0.0',
    instance: 'driver:fleet-stats',
    trust: 'verified',
  });
  const community = offerOf({
    module: 'github.com/acme/queue-board',
    name: 'queue-board',
    title: 'Queue board',
    description: 'A wall display of what prints next.',
    author: 'Acme',
    namespace: 'com.github.acme.queue-board',
    trust: 'community',
    channel: 'beta',
    version: '0.9.0',
  });
  const local = offerOf({
    module: 'example.com/local/notes',
    name: 'notes',
    title: 'Printer notes',
    description: '',
    author: '',
    namespace: 'com.example.local.notes',
    origin: 'local',
    version: '0.1.0',
  });

  const installed = [
    instanceOf(),
    instanceOf({
      name: 'fleet-stats',
      namespace: 'com.github.prusa3d.fleet-stats',
      uid: 'u2',
      ...inState('ready'),
    }),
    instanceOf({
      name: 'hand-made',
      namespace: 'com.example.hand-made',
      uid: 'u3',
      source: 'file',
      module: { asked: {}, title: 'Hand made', version: '0.0.1' },
    }),
  ];

  // Many installed modules, three of which another one requires, so their
  // switches are locked with a reason.
  const many = Array.from({ length: 9 }, (_, index) => {
    const name = `helper-${String(index + 1)}`;

    return {
      offer: offerOf({
        module: `github.com/prusa3d/${name}`,
        name,
        title: `Helper extension with a longer name ${String(index + 1)}`,
        namespace: `com.github.prusa3d.${name}`,
        installed: true,
        installed_version: '1.4.0',
        instance: `driver:${name}`,
        trust: 'official',
      }),
      instance: instanceOf({
        name,
        namespace: `com.github.prusa3d.${name}`,
        uid: `m${String(index)}`,
        module: {
          asked: {},
          title: `Helper extension with a longer name ${String(index + 1)}`,
          version: '1.4.0',
          ...(index < 3
            ? {
                required_by: [
                  {
                    instance: 'sink:planner',
                    module: 'com.github.prusa3d.planner',
                    version: '^1.0',
                    enabled: true,
                  },
                ],
              }
            : {}),
        },
      }),
    };
  });
  // A service module reading well, one close to its budget with a failure
  // behind it, and one whose use the host could not read.
  const MIB = 1_048_576;
  const memoryOf = (over: Record<string, unknown>) => ({
    budget_bytes: 128 * MIB,
    budget_source: 'default' as const,
    failures_in_window: 0,
    held_until_restart: false,
    near_budget: false,
    on_over_memory: 'stop' as const,
    over_budget: false,
    switch_off_failures: 3,
    switch_off_source: 'default' as const,
    switch_off_window_seconds: 3600,
    ...over,
  });
  const withMemory = (
    name: string,
    uid: string,
    memory: Record<string, unknown>,
  ) =>
    instanceOf({
      name,
      namespace: `com.github.prusa3d.${name}`,
      uid,
      status: { ...instanceOf().status, memory: memoryOf(memory) },
      module: { asked: {}, title: name, version: '1.0.0' },
    });
  const memoryRows = extensionRowsOf(
    [],
    [
      withMemory('spools', 'w1', { used_bytes: 40 * MIB }),
      withMemory('nozzle-watch', 'w2', {
        used_bytes: 115 * MIB,
        near_budget: true,
        failures_in_window: 2,
        switch_off_window_seconds: 600,
      }),
      withMemory('buddy-logs', 'w3', {}),
      {
        ...withMemory('slicer', 'w4', { budget_bytes: 256 * MIB }),
        enabled: false,
        status: {
          ...instanceOf().status,
          state: 'disabled',
          memory: memoryOf({ budget_bytes: 256 * MIB }),
        },
      },
    ],
    [],
  );
  const manyRows = extensionRowsOf(
    many.map((one) => one.offer),
    many.map((one) => one.instance),
    [],
  );

  const rows = extensionRowsOf([tracker, update, community, local], installed, [
    {
      namespace: 'com.github.prusa3d.spool-tracker',
      path: 'spools',
      title: 'Spools',
      hidden: false,
      href: '/m/com.github.prusa3d.spool-tracker/spools',
    },
  ]);
</script>

<Story name="Table" asChild>
  <ExtensionsListing
    {rows}
    view="table"
    {catalogue}
    {shows}
    go={noop}
    ask={noop}
  />
</Story>

<Story name="Memory Cells" asChild>
  <ExtensionsListing
    rows={memoryRows}
    view="table"
    {catalogue}
    {shows}
    go={noop}
    ask={noop}
  />
</Story>

<Story name="Memory In Stacked Rows" asChild>
  <ExtensionsListing
    rows={memoryRows}
    view="rows"
    {catalogue}
    {shows}
    go={noop}
    ask={noop}
  />
</Story>

<Story name="Stacked Rows" asChild>
  <ExtensionsListing
    {rows}
    view="rows"
    {catalogue}
    {shows}
    go={noop}
    ask={noop}
  />
</Story>

<Story name="Narrow Update And Install" asChild>
  <div class="max-w-xs">
    <ExtensionsListing
      rows={rows.map((row) => ({
        ...row,
        title: `${row.title} for the workshop printers`,
        author:
          'WorkshopAutomationAndManufacturingTeamCentralEuropeanOperations',
        version:
          row.state === 'update'
            ? '1.1.0-beta.printerfarm.integration.release-candidate.20261025.124500'
            : row.version,
      }))}
      view="rows"
      {catalogue}
      {shows}
      go={noop}
      ask={noop}
    />
  </div>
</Story>

<Story name="Row A Link Pointed At" asChild>
  <ExtensionsListing
    {rows}
    view="table"
    {catalogue}
    {shows}
    go={noop}
    ask={noop}
    highlighted={rows[1]?.key}
  />
</Story>

<Story name="Columns Put Away" asChild>
  <ExtensionsListing
    {rows}
    view="table"
    {catalogue}
    shows={(field) => field === 'module' || field === 'actions'}
    go={noop}
    ask={noop}
  />
</Story>

<Story name="Stacked Rows With Columns Put Away" asChild>
  <ExtensionsListing
    {rows}
    view="rows"
    {catalogue}
    shows={(field) => field === 'module' || field === 'actions'}
    go={noop}
    ask={noop}
  />
</Story>

<!-- Many installed modules at phone width: the words of a locked switch wrap
     in the row, and nothing pushes the page sideways. -->
<Story name="Many Extensions At Phone Width" asChild>
  <div class="max-w-90">
    <ExtensionsListing
      rows={manyRows}
      view="table"
      {catalogue}
      {shows}
      go={noop}
      ask={noop}
    />
  </div>
</Story>

<!-- The same modules in a table: the author and the source columns give way
     before the table runs wider than its box. -->
<Story name="Many Extensions In A Table" asChild>
  <ExtensionsListing
    rows={manyRows}
    view="table"
    {catalogue}
    {shows}
    go={noop}
    ask={noop}
  />
</Story>
