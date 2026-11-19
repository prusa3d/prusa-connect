<!--
  Encryption for the web app and for the printers, each a switch with its
  certificate under it. A printer speaks TLS 1.2 and the web app TLS 1.3, so
  printers get a port of their own as soon as either is on (ADR-20261015-ejz rule 36).
  The address printers use is asked whether or not either is on: it is what
  the add-printer page writes into the printer's settings file.
-->
<script lang="ts">
  import UsbIcon from '@lucide/svelte/icons/usb';
  import { onMount } from 'svelte';

  import type { SetupConfiguration, SetupFlow } from '../../data';
  import { isOwnAddress } from '../../domain/printer-address';
  import { t } from '../../i18n';
  import {
    Alert,
    AlertDescription,
    Card,
    CardContent,
    Field,
    Input,
    SettingsRow,
    Switch,
  } from '../../primitives';
  import CertificateGuide from './certificate-guide.svelte';
  import CertificateMaker from './certificate-maker.svelte';
  import CertificateSource from './certificate-source.svelte';
  import {
    encryptionErrors,
    needsListener,
    printerHostPort,
  } from './encryption';
  import type {
    CertificateSource as CertificateSourceName,
    SetupDraft,
  } from './setup-draft.svelte';

  interface StepEncryptionProps {
    draft: SetupDraft;
    flow: SetupFlow;
    contract: SetupConfiguration;
    docsHref: string | undefined;
    showErrors: boolean;
  }

  let { draft, flow, contract, docsHref, showErrors }: StepEncryptionProps =
    $props();

  const encryption = $derived(draft.encryption);
  // A file that declares the connectors locks the switch, and it shows what
  // the file decided rather than the draft's default.
  const printersOn = $derived(
    contract.printers ? encryption.printers : contract.printers_tls,
  );
  const errors = $derived(encryptionErrors(encryption, contract.printers));
  const shown = (error: string): string => (showErrors ? error : '');
  const generating = $derived(
    (encryption.web && encryption.webSource === 'generate') ||
      (encryption.printers && encryption.printersSource === 'generate'),
  );
  const own = $derived(
    (encryption.web && encryption.webSource === 'own') ||
      (encryption.printers && encryption.printersSource === 'own'),
  );

  // The names a certificate needs start from the address this page was opened
  // on and the ones the server knows it answers on.
  onMount(() => {
    // A printer never reaches this machine at a loopback name, so a page
    // opened on one offers the first address the machine has on the network.
    const network = (contract.hosts ?? []).find((name) =>
      /^\d+\.\d+\.\d+\.\d+$/.test(name),
    );
    const here =
      isOwnAddress(location.hostname) && network !== undefined
        ? network
        : location.hostname;
    if (encryption.names.trim() === '') {
      const names = [here, ...(contract.hosts ?? []), location.hostname].filter(
        (name, index, all) => name !== '' && all.indexOf(name) === index,
      );
      draft.encrypt({ names: names.join('\n') });
    }
    if (encryption.printerHost.trim() === '')
      draft.encrypt({ printerHost: here });
  });

  const make = async (): Promise<void> => {
    const names = encryption.names
      .split('\n')
      .map((name) => name.trim())
      .filter((name) => name !== '');
    if (await flow.createCertificates(draft.code.trim(), names)) {
      const made = flow.certificates;
      if (made !== undefined) {
        draft.encrypt({
          made: {
            web: [made.web_certificate, made.web_key],
            printer: [made.printer_certificate, made.printer_key],
          },
        });
      }
    }
  };
</script>

{#snippet toggle(
  label: string,
  hint: string,
  on: boolean,
  disabled: boolean,
  change: (next: boolean) => void,
)}
  <SettingsRow layout="switch" {label} description={hint}>
    {#snippet control()}
      <Switch
        aria-label={label}
        checked={on}
        {disabled}
        onCheckedChange={change}
      />
    {/snippet}
  </SettingsRow>
{/snippet}

<div class="flex flex-col gap-6" data-part="encryption">
  {@render toggle(
    t('setupWizard.encryption.web.label'),
    t('setupWizard.encryption.web.hint'),
    encryption.web,
    false,
    (next: boolean) => {
      draft.encrypt({ web: next });
    },
  )}
  {#if encryption.web}
    <div class="border-s-2 border-primary/40 ps-4" data-part="dependents">
      <CertificateSource
        label={t('setupWizard.encryption.web.label')}
        source={encryption.webSource}
        certificate={encryption.webCertificate}
        key={encryption.webKey}
        error={shown(errors.web)}
        onsource={(source: CertificateSourceName) => {
          draft.encrypt({ webSource: source });
        }}
        oncertificate={(path: string) => {
          draft.encrypt({ webCertificate: path });
        }}
        onkey={(path: string) => {
          draft.encrypt({ webKey: path });
        }}
      />
    </div>
  {/if}

  {@render toggle(
    t('setupWizard.encryption.printers.label'),
    t('setupWizard.encryption.printers.hint'),
    printersOn,
    !contract.printers,
    (next: boolean) => {
      draft.encrypt({ printers: next });
    },
  )}
  {#if !contract.printers}
    <p class="text-sm text-muted-foreground">
      {t('setupWizard.encryption.printers.declared')}
    </p>
  {/if}
  {#if encryption.printers}
    <div class="border-s-2 border-primary/40 ps-4" data-part="dependents">
      <CertificateSource
        label={t('setupWizard.encryption.printers.label')}
        source={encryption.printersSource}
        certificate={encryption.printerCertificate}
        key={encryption.printerKey}
        error={shown(errors.printers)}
        onsource={(source: CertificateSourceName) => {
          draft.encrypt({ printersSource: source });
        }}
        oncertificate={(path: string) => {
          draft.encrypt({ printerCertificate: path });
        }}
        onkey={(path: string) => {
          draft.encrypt({ printerKey: path });
        }}
      />
    </div>
  {/if}

  {#if contract.printers}
    <Card data-part="printer-listener">
      <CardContent class="flex flex-col gap-4">
        {#if encryption.web && !encryption.printers}
          <Alert variant="info">
            <AlertDescription
              >{t('setupWizard.encryption.listener.plain')}</AlertDescription
            >
          </Alert>
        {/if}
        {#if needsListener(encryption)}
          <Field
            label={t('setupWizard.encryption.listener.port')}
            description={t('setupWizard.encryption.listener.portHint')}
            error={shown(errors.port)}
            control={Input}
            controlProps={{
              value: encryption.printerPort,
              type: 'text',
              class: 'font-mono',
              spellcheck: false,
              oninput: (event: Event & { currentTarget: HTMLInputElement }) => {
                draft.encrypt({ printerPort: event.currentTarget.value });
              },
            }}
          />
        {/if}
        <Field
          label={t('setupWizard.encryption.listener.host')}
          description={t('setupWizard.encryption.listener.hostHint')}
          error={shown(errors.host)}
          control={Input}
          controlProps={{
            value: encryption.printerHost,
            type: 'text',
            class: 'font-mono',
            spellcheck: false,
            oninput: (event: Event & { currentTarget: HTMLInputElement }) => {
              draft.encrypt({ printerHost: event.currentTarget.value });
            },
          }}
        />
      </CardContent>
    </Card>
  {/if}

  {#if generating}
    <CertificateMaker
      names={encryption.names}
      busy={flow.making}
      made={flow.certificates}
      madeBefore={encryption.made !== undefined}
      error={flow.certificatesError}
      onnames={(names: string) => {
        draft.encrypt({ names, made: undefined });
      }}
      onmake={() => void make()}
    />
  {/if}

  {#if encryption.printers}
    <div
      class="flex gap-3 rounded-md bg-muted p-4 text-sm"
      data-part="printer-usb"
    >
      <UsbIcon class="mt-0.5 size-5 shrink-0" aria-hidden="true" />
      <div class="flex flex-col gap-2">
        <p class="font-medium">{t('setupWizard.encryption.usb.title')}</p>
        <ol class="flex list-decimal flex-col gap-1 ps-5 text-muted-foreground">
          <li>{t('setupWizard.encryption.usb.copy')}</li>
          <li>{t('setupWizard.encryption.usb.ini')}</li>
          <li>{t('setupWizard.encryption.usb.load')}</li>
        </ol>
      </div>
    </div>
  {/if}

  {#if own}
    <CertificateGuide
      host={printerHostPort(encryption.printerHost).host}
      {docsHref}
    />
  {/if}
</div>
