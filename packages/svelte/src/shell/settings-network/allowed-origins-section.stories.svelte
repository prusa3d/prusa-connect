<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import AllowedOriginsSection from './allowed-origins-section.svelte';
  import type { AllowedOriginsSource } from './types';

  const { Story } = defineMeta({
    title: 'Parts/Screen components/Settings Network/Allowed Origins',
    component: AllowedOriginsSection,
  });

  const sourceOf = (
    over: Partial<AllowedOriginsSource> = {},
  ): AllowedOriginsSource => ({
    items: [
      { origin: 'https://farm.example', source: 'file' },
      { origin: 'https://dashboard.workshop.example', source: 'settings' },
      {
        origin: 'https://slicer.example',
        source: 'extension',
        extension: 'com.github.acme.slicer',
        title: 'Web slicer',
      },
    ],
    loaded: true,
    saving: false,
    error: undefined,
    add: () => Promise.resolve(true),
    remove: () => Promise.resolve(true),
    ...over,
  });

  const extensionHref = (namespace: string): string =>
    `/settings/extensions/${namespace}`;
</script>

<Story name="Three Sources" asChild>
  <AllowedOriginsSection source={sourceOf()} {extensionHref} />
</Story>

<Story name="None" asChild>
  <AllowedOriginsSection source={sourceOf({ items: [] })} />
</Story>

<Story name="Saving" asChild>
  <AllowedOriginsSection source={sourceOf({ saving: true })} />
</Story>

<Story name="Still Reading" asChild>
  <AllowedOriginsSection source={sourceOf({ items: [], loaded: false })} />
</Story>

<Story name="Refused" asChild>
  <AllowedOriginsSection
    source={sourceOf({
      error: {
        code: 'core.auth/ACCESS_DENIED',
        status: 403,
        params: {},
        message:
          'this caller holds no admin role on the organization, so it may not administer the allowed browser origins',
        traceId: undefined,
      },
    })}
  />
</Story>
