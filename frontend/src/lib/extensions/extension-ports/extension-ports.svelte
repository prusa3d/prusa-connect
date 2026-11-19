<!--
  The ports an installed extension listens on, and a way to change them. Each
  port the manifest asks for shows whether it is allowed and on which number.
  Saving installs the same release again with the ports allowed at the numbers
  typed here (ADR-20261019-3ns rules 7 and 10), so a port left out at install can be
  allowed later too.
-->
<script lang="ts">
  import { translateFor } from '@prusa/connect-ui/i18n';
  import {
    Badge,
    Button,
    FaultAlert,
    Input,
    Label,
  } from '@prusa/connect-ui/primitives';

  import { APP_NAMESPACE } from '../../i18n';
  import { describeExtensionFault } from '../fault';
  import { type ModuleListen, movedPorts, portOfText } from '../requests';

  let {
    asked,
    listening,
    locked,
    busy,
    fault,
    onsave,
  }: {
    /** The ports the manifest asks for, with the number it asks for. */
    asked: readonly ModuleListen[];
    /** The ports allowed now, at the number each listens on. */
    listening: readonly ModuleListen[];
    /** Why the ports cannot be changed here, already worded. */
    locked?: string | undefined;
    busy: boolean;
    fault?: unknown;
    /** Installs again with these moves; resolves true once the server applied them. */
    onsave: (listen: Record<string, number>) => Promise<boolean>;
  } = $props();

  const t = translateFor(APP_NAMESPACE);
  const id = $props.id();

  const current = (port: ModuleListen): ModuleListen | undefined =>
    listening.find((held) => held.name === port.name);

  // What the person typed, by port name. A port not typed shows the number
  // it listens on now, or the manifest's when it is not allowed.
  let typed = $state<Record<string, string>>({});
  const typedFor = (port: ModuleListen): string =>
    typed[port.name] ?? String(current(port)?.port ?? port.port);

  const allValid = $derived(
    asked.every((port) => portOfText(typedFor(port)) !== undefined),
  );
  // Every port is saved, so an allowed port keeps the number it holds even
  // when the person typed nothing for it.
  const chosen = $derived(
    Object.fromEntries(asked.map((port) => [port.name, typedFor(port)])),
  );
  const changed = $derived(
    asked.some(
      (port) =>
        current(port) === undefined ||
        portOfText(typedFor(port)) !== current(port)?.port,
    ),
  );
  const shownFault = $derived(
    fault === undefined ? undefined : describeExtensionFault(fault, t),
  );

  const save = async (): Promise<void> => {
    if (await onsave(movedPorts(asked, chosen))) typed = {};
  };
</script>

<div class="flex flex-col gap-section-gap" data-part="ports">
  <p class="text-sm text-muted-foreground">
    {t('app.settings.extensions.ports.note')}
  </p>

  <ul class="flex flex-col gap-3">
    {#each asked as port (port.name)}
      {@const held = current(port)}
      {@const valid = portOfText(typedFor(port)) !== undefined}
      <li class="flex flex-wrap items-end gap-3" data-part="port">
        <div class="flex min-w-48 flex-1 flex-col gap-1">
          <Label for={`${id}-${port.name}`}>
            {t('app.settings.extensions.confirm.portNumber', {
              network: port.network.toUpperCase(),
              why: port.why,
            })}
          </Label>
          <Input
            id={`${id}-${port.name}`}
            type="number"
            inputmode="numeric"
            min="1"
            max="65535"
            size="sm"
            class="w-32"
            value={typedFor(port)}
            disabled={locked !== undefined || busy}
            aria-invalid={!valid}
            oninput={(event: Event) => {
              typed = {
                ...typed,
                [port.name]: (event.currentTarget as HTMLInputElement).value,
              };
            }}
          />
          {#if !valid}
            <span class="text-xs text-destructive" role="status">
              {t('app.settings.extensions.ports.invalid')}
            </span>
          {/if}
        </div>
        {#if held === undefined}
          <Badge variant="warning">
            {t('app.settings.extensions.ports.notAllowed')}
          </Badge>
        {:else}
          <Badge variant="secondary">
            {t('app.settings.extensions.ports.listening', {
              port: String(held.port),
            })}
          </Badge>
        {/if}
      </li>
    {/each}
  </ul>

  {#if locked !== undefined}
    <p class="text-sm" data-part="ports-locked">{locked}</p>
  {:else}
    <p class="text-xs text-muted-foreground">
      {t('app.settings.extensions.confirm.portsMove')}
    </p>
    <div class="flex justify-end">
      <Button
        size="control"
        data-settings-action
        disabled={busy || !allValid || !changed}
        aria-busy={busy}
        onclick={() => {
          void save();
        }}
      >
        {t('app.settings.extensions.ports.save')}
      </Button>
    </div>
  {/if}

  <FaultAlert fault={shownFault} />
</div>
