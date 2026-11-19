<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import KeyValueList from './key-value-list.svelte';
  import type { KeyValueRow } from './types';

  const { Story } = defineMeta({
    title: 'Primitives/KeyValueList',
    component: KeyValueList,
  });

  const rows: KeyValueRow[] = [
    { id: 'model', label: 'Model', value: 'CORE One' },
    { id: 'firmware', label: 'Firmware', value: '6.2.4' },
    {
      id: 'serial',
      label: 'Serial',
      value: 'SN-1',
      mono: true,
      copyable: true,
    },
    { id: 'ip', label: 'Address', value: '192.168.1.24', mono: true },
  ];

  const withGaps: KeyValueRow[] = [
    { id: 'model', label: 'Model', value: 'CORE One' },
    { id: 'firmware', label: 'Firmware' },
    { id: 'serial', label: 'Serial' },
  ];
</script>

<Story name="Default" args={{ rows }} />

<Story name="Nothing reported" args={{ rows: withGaps }} />

<Story name="Empty" args={{ rows: [] }} />

<Story
  name="Value hierarchy"
  args={{
    rows: rows.map((row) => ({
      ...row,
      emphasis: row.id === 'model' ? 'strong' : 'muted',
    })),
  }}
/>
