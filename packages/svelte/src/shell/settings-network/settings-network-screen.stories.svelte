<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import type {
    AllowedOriginsSource,
    TimeServerSource,
    VisibilitySource,
  } from './types';
  import SettingsNetworkScreen from './settings-network-screen.svelte';

  const { Story } = defineMeta({
    title: 'Parts/Screen components/Settings Network',
    component: SettingsNetworkScreen,
  });

  const sourceOf = (
    over: Partial<VisibilitySource> = {},
  ): VisibilitySource => ({
    hidden: false,
    announcement: 'announcing',
    described: {
      hidden: false,
      announcement: 'announcing',
      name: 'Workshop',
      host: 'workshop-server.local',
      port: 8080,
      services: ['_prusalink._tcp', '_prusa-connect._tcp'],
      addresses: ['192.168.1.10', 'fe80::1c2b:3aff:fe4d:5e6f'],
      bridged: false,
    },
    loaded: true,
    saving: false,
    error: undefined,
    setHidden: () => Promise.resolve(),
    ...over,
  });

  const timeOf = (over: Partial<TimeServerSource> = {}): TimeServerSource => ({
    enabled: false,
    state: 'off',
    reason: undefined,
    cause: undefined,
    port: 123,
    loaded: true,
    saving: false,
    error: undefined,
    setEnabled: () => Promise.resolve(),
    ...over,
  });

  const originsOf = (
    over: Partial<AllowedOriginsSource> = {},
  ): AllowedOriginsSource => ({
    items: [
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
</script>

<Story name="Announcing" asChild>
  <SettingsNetworkScreen source={sourceOf()} />
</Story>

<Story name="Hidden" asChild>
  <SettingsNetworkScreen
    source={sourceOf({ hidden: true, announcement: 'hidden' })}
  />
</Story>

<Story name="Discovery Is Off" asChild>
  <SettingsNetworkScreen source={sourceOf({ announcement: 'off' })} />
</Story>

<Story name="Announcer Failed" asChild>
  <SettingsNetworkScreen source={sourceOf({ announcement: 'failed' })} />
</Story>

<Story name="In A Bridged Container" asChild>
  <SettingsNetworkScreen
    source={sourceOf({
      described: {
        hidden: false,
        announcement: 'announcing',
        name: 'Workshop',
        host: 'a1b2c3d4e5f6.local',
        port: 8080,
        services: ['_prusalink._tcp', '_prusa-connect._tcp'],
        addresses: ['172.18.0.2'],
        bridged: true,
      },
    })}
  />
</Story>

<Story name="Saving" asChild>
  <SettingsNetworkScreen source={sourceOf({ saving: true })} />
</Story>

<Story name="Still Reading" asChild>
  <SettingsNetworkScreen
    source={sourceOf({
      loaded: false,
      announcement: undefined,
      described: undefined,
    })}
  />
</Story>

<Story name="Refused" asChild>
  <SettingsNetworkScreen
    source={sourceOf({
      error: {
        code: 'core.transport/NETWORK',
        status: undefined,
        params: {},
        message: 'the server could not be reached',
        traceId: undefined,
      },
    })}
  />
</Story>

<Story name="Time Server Off" asChild>
  <SettingsNetworkScreen source={sourceOf()} timeServer={timeOf()} />
</Story>

<Story name="Time Server Serving" asChild>
  <SettingsNetworkScreen
    source={sourceOf()}
    timeServer={timeOf({ enabled: true, state: 'serving' })}
  />
</Story>

<Story name="Time Server Failed" asChild>
  <SettingsNetworkScreen
    source={sourceOf()}
    timeServer={timeOf({
      enabled: true,
      state: 'failed',
      reason: 'listen udp4 :123: bind: cannot assign requested address',
    })}
  />
</Story>

<Story name="Time Server Has No Permission" asChild>
  <SettingsNetworkScreen
    source={sourceOf()}
    docsHref="https://docs.example/"
    timeServer={timeOf({
      enabled: true,
      state: 'failed',
      reason: 'listen udp4 :123: bind: permission denied',
      cause: 'permission',
    })}
  />
</Story>

<Story name="Time Server Port In Use" asChild>
  <SettingsNetworkScreen
    source={sourceOf()}
    timeServer={timeOf({
      enabled: true,
      state: 'failed',
      reason: 'listen udp4 :123: bind: address already in use',
      cause: 'in_use',
    })}
  />
</Story>

<Story name="Time Server Still Reading" asChild>
  <SettingsNetworkScreen
    source={sourceOf()}
    timeServer={timeOf({ loaded: false, state: undefined })}
  />
</Story>

<Story name="With Allowed Origins" asChild>
  <SettingsNetworkScreen
    source={sourceOf()}
    timeServer={timeOf()}
    origins={originsOf()}
  />
</Story>
