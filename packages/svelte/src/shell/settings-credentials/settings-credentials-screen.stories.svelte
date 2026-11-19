<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import type { ApiCredential } from '../../data';
  import { fakeListingStore } from '../../test/fakes';
  import SettingsCredentialsScreen from './settings-credentials-screen.svelte';
  import type { CredentialsSource } from './types';

  const { Story } = defineMeta({
    title: 'Parts/Screen components/Settings Credentials',
    component: SettingsCredentialsScreen,
  });

  const DAY_MS = 24 * 60 * 60 * 1000;

  /** Dates are relative to the run rather than fixed. */
  const at = (days: number): string =>
    new Date(Date.now() + days * DAY_MS).toISOString();

  const items: ApiCredential[] = [
    {
      id: '7Kp2mQ9xLw4nRt8vBc3zYe',
      name: 'CI upload runner',
      subject: 'user:alice',
      scopes: ['operate'],
      origin: 'mint',
      created_at: at(-110),
      expires_at: at(9),
      last_used_at: at(-1),
    },
    {
      id: 'Xw3bN8qLp5tR2mKv7cJz1H',
      name: 'Kitchen display',
      subject: 'user:alice',
      scopes: ['read'],
      origin: 'app_pairing',
      created_at: at(-40),
      expires_at: at(50),
      last_used_at: at(0),
    },
    {
      id: 'Qz7vT1hY6sD9gK4bN2mXwE',
      name: 'Workshop laptop',
      subject: 'user:alice',
      scopes: ['administer'],
      origin: 'mint',
      created_at: at(-2),
      expires_at: at(88),
    },
  ];

  const source = (
    over: Partial<CredentialsSource> = {},
  ): CredentialsSource => ({
    items,
    loading: false,
    loaded: true,
    error: undefined,
    minted: undefined,
    ...over,
  });

  const ports = {
    store: fakeListingStore(),
    onCreate: () => undefined,
    onDismiss: () => undefined,
    onRevoke: () => undefined,
    pairHref: '/pair',
  };

  /** A store already holding the stacked shape, so the second one has a story. */
  const stackedStore = {
    read: () => JSON.stringify({ view: 'rows' }),
    write: () => undefined,
  };
</script>

<!-- The React specification's own states. -->

<Story name="Api Credentials" args={{ credentials: source(), ...ports }} />

<Story
  name="Duplicate Names"
  args={{
    credentials: source({
      items: items.map((item) => ({ ...item, name: 'demo' })),
    }),
    ...ports,
  }}
/>

<Story
  name="None Yet"
  args={{ credentials: source({ items: [] }), ...ports }}
/>

<!-- The shape the toolbar's other choice draws, at any width. -->
<Story
  name="Rows"
  args={{ credentials: source(), ...ports, store: stackedStore }}
/>

<!-- The token exists on screen and nowhere else: this server keeps a hash. -->
<Story
  name="A New Credential Is Shown Once"
  args={{
    credentials: source({
      minted: {
        id: 'Lm5cF8jR3wP7yH1tV6zQaB',
        name: 'Nightly export',
        subject: 'user:alice',
        scopes: ['read'],
        origin: 'mint',
        created_at: at(0),
        expires_at: at(90),
        token: 'pcl_9f3b21c7ae4d05e8b6f14a72c3d09e5b',
      },
    }),
    ...ports,
  }}
/>

<Story
  name="Still Arriving"
  args={{
    credentials: source({ items: [], loaded: false, loading: true }),
    ...ports,
  }}
/>

<Story
  name="Refused"
  args={{
    credentials: source({
      items: [],
      loaded: false,
      error: { code: 'core.auth/ACCESS_DENIED', status: 403 },
    }),
    ...ports,
  }}
/>
