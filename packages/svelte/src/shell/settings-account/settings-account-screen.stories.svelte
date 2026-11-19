<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import SettingsAccountScreen from './settings-account-screen.svelte';
  import type { AccountSource } from './types';

  const { Story } = defineMeta({
    title: 'Parts/Screen components/Settings Account',
    component: SettingsAccountScreen,
  });

  const signedIn: AccountSource = {
    principal: { id: 'user:alice', grants: [] },
    status: 'signed-in',
  };

  const reading: AccountSource = { principal: undefined, status: 'unknown' };

  const onChangePassword = () => undefined;
</script>

<Story name="Default" args={{ source: signedIn, onChangePassword }} />

<Story
  name="Populated"
  args={{
    source: signedIn,
    identity: {
      username: 'alice',
      displayName: 'Alice Doe',
      role: 'admin',
      createdAt: '2026-08-19T17:05:00Z',
    },
    onChangePassword,
  }}
/>

<Story
  name="Reading The Principal"
  args={{ source: reading, onChangePassword }}
/>

<Story
  name="Changing"
  args={{ source: signedIn, changing: true, onChangePassword }}
/>

<!-- The core's own wording for a wrong current password, carried by the
     registry code rather than written here. -->
<Story
  name="Refused"
  args={{
    source: signedIn,
    fault: { code: 'core.auth/LOGIN_REFUSED', status: 401 },
    onChangePassword,
  }}
/>

<Story
  name="Changed"
  args={{ source: signedIn, changed: true, onChangePassword }}
/>

<Story
  name="Profile Image Unavailable"
  args={{
    source: signedIn,
    identity: {
      username: 'alice',
      displayName: 'Alice Doe',
      avatarUrl: 'data:image/png;base64,invalid',
      role: 'admin',
      createdAt: '2026-08-19T17:05:00Z',
    },
    onChangePassword,
  }}
/>

<!-- Signed in at an identity provider, which keeps the password. -->
<Story
  name="Passwordless"
  args={{
    source: signedIn,
    identity: { displayName: 'Alice Doe' },
    passwordless: true,
    onChangePassword,
  }}
/>
