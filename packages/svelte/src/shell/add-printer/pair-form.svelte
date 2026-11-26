<!--
  The form that asks a printer on the network to pair: the printer's address,
  and behind one disclosure the address of this server that the printer
  stores. The server address starts as the one the settings file carries, so
  a person who never opens the disclosure sends the same address the file
  would.
-->
<script lang="ts">
  import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';

  import { isOwnAddress } from '../../domain/printer-address';
  import { t } from '../../i18n';
  import {
    Alert,
    AlertDescription,
    Button,
    Collapsible,
    CollapsibleContent,
    CollapsibleTrigger,
    Field,
    Input,
    Label,
    Switch,
  } from '../../primitives';
  import { storedHostFits } from './presentation';
  import type { AutoConnect, PairingRequest } from './types';

  let {
    connection,
    busy = false,
    onPair,
  }: {
    connection: AutoConnect;
    busy?: boolean;
    onPair: (request: PairingRequest) => void;
  } = $props();

  const id = $props.id();

  let printer = $state('');
  // The server address is the person's draft from here on. It starts as the
  // address the settings file carries.
  // svelte-ignore state_referenced_locally
  let host = $state(connection.host);
  // svelte-ignore state_referenced_locally
  let port = $state(String(connection.port));
  // svelte-ignore state_referenced_locally
  let tls = $state(connection.tls);
  let serverOpen = $state(false);

  const portNumber = $derived(Number(port));
  const portValid = $derived(
    Number.isInteger(portNumber) && portNumber >= 1 && portNumber <= 65535,
  );
  const own = $derived(isOwnAddress(host));
  const tooLong = $derived(!storedHostFits(host.trim()));
  const ready = $derived(
    printer.trim() !== '' && host.trim() !== '' && portValid && !own,
  );

  const submit = (event: SubmitEvent): void => {
    event.preventDefault();
    if (!ready || busy) return;
    onPair({
      printer: printer.trim(),
      connect: { host: host.trim(), port: portNumber, tls },
    });
  };
</script>

<form class="flex flex-col gap-4" data-part="pair-form" onsubmit={submit}>
  <Field
    label={t('addPrinter.pair.printer.label')}
    description={t('addPrinter.pair.printer.hint')}
    control={Input}
    controlProps={{
      value: printer,
      type: 'text',
      inputmode: 'url',
      autocomplete: 'off',
      spellcheck: false,
      placeholder: '192.168.1.20',
      class: 'font-mono',
      oninput: (event: Event & { currentTarget: HTMLInputElement }) => {
        printer = event.currentTarget.value;
      },
    }}
  />

  <Collapsible bind:open={serverOpen}>
    <CollapsibleTrigger variant="inline">
      {t('addPrinter.pair.server.toggle', { host, port })}
    </CollapsibleTrigger>
    <CollapsibleContent class="flex flex-col gap-4 pbs-3">
      <p class="text-sm text-muted-foreground">
        {t('addPrinter.pair.server.note')}
      </p>
      <div class="flex flex-wrap gap-4">
        <Field
          class="min-w-0 flex-1"
          label={t('addPrinter.pair.server.host')}
          control={Input}
          controlProps={{
            value: host,
            type: 'text',
            autocomplete: 'off',
            spellcheck: false,
            class: 'font-mono',
            'aria-invalid': own || tooLong,
            oninput: (event: Event & { currentTarget: HTMLInputElement }) => {
              host = event.currentTarget.value;
            },
          }}
        />
        <Field
          class="w-28"
          label={t('addPrinter.pair.server.port')}
          control={Input}
          controlProps={{
            value: port,
            type: 'text',
            inputmode: 'numeric',
            autocomplete: 'off',
            class: 'font-mono',
            'aria-invalid': !portValid,
            oninput: (event: Event & { currentTarget: HTMLInputElement }) => {
              port = event.currentTarget.value;
            },
          }}
        />
      </div>
      <div class="flex items-center gap-2">
        <Switch
          id={`${id}-tls`}
          checked={tls}
          onCheckedChange={(next: boolean) => {
            tls = next;
          }}
        />
        <Label for={`${id}-tls`}>{t('addPrinter.pair.server.tls')}</Label>
      </div>
    </CollapsibleContent>
  </Collapsible>

  {#if own}
    <Alert variant="warning" data-part="pair-own-host">
      <TriangleAlertIcon aria-hidden="true" />
      <AlertDescription>
        {t('addPrinter.pair.server.own', { host })}
      </AlertDescription>
    </Alert>
  {:else if tooLong}
    <Alert variant="warning" data-part="pair-long-host">
      <TriangleAlertIcon aria-hidden="true" />
      <AlertDescription>
        {t('addPrinter.pair.server.long', { host })}
      </AlertDescription>
    </Alert>
  {/if}

  <div>
    <Button type="submit" disabled={!ready || busy} aria-busy={busy}>
      {t('addPrinter.pair.start')}
    </Button>
  </div>
</form>
