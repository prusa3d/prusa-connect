<!--
  The first run as a wizard (ADR-20261015-ejz): a welcome, the setup code, the basic
  or the advanced path, a summary, and the wait while the server restarts on
  the answers. What the person types is kept across a reload.
-->
<script lang="ts" module>
  import { addWizardTexts } from './texts';

  // Before the first line is drawn: the wizard's texts are in its own chunk.
  addWizardTexts();
</script>

<script lang="ts">
  import ArrowLeftIcon from '@lucide/svelte/icons/arrow-left';
  import ArrowRightIcon from '@lucide/svelte/icons/arrow-right';
  import BookOpenIcon from '@lucide/svelte/icons/book-open';
  import { onMount } from 'svelte';

  import { t } from '../../i18n';
  import { applyTheme, theme } from '../../styles/theme.svelte';
  import {
    Alert,
    AlertDescription,
    AlertTitle,
    Button,
    ConnectLogo,
    PrinterIcon,
    Progress,
    StepList,
  } from '../../primitives';
  import { oidcAnswers, stepIsComplete } from './checks';
  import { encryptionDecision } from './encryption';
  import { sectionLead, sectionShort, sectionTitle } from './labels';
  import {
    configurationOf,
    IDENTITY_KEYS,
    keysOf,
    movedOrigin,
    OIDC_CALLBACK,
    reachedOf,
    sectionOfStep,
    settleLocked,
    shownOf,
    type StepId,
    stepsOf,
  } from './model';
  import { SetupDraft } from './setup-draft.svelte';
  import StepAdmission from './step-admission.svelte';
  import StepCode from './step-code.svelte';
  import StepEncryption from './step-encryption.svelte';
  import StepIdentity from './step-identity.svelte';
  import StepPath from './step-path.svelte';
  import StepSection from './step-section.svelte';
  import StepStorage from './step-storage.svelte';
  import StepSummary from './step-summary.svelte';
  import StepWait from './step-wait.svelte';
  import StepWelcome from './step-welcome.svelte';
  import WizardToolbar from './wizard-toolbar.svelte';
  import type { SetupWizardProps } from './types';

  let {
    flow,
    host,
    version,
    docsHref,
    lastFailure,
    onlanguage,
    onfinish,
  }: SetupWizardProps = $props();

  const draft = new SetupDraft();
  let attempted = $state(false);
  let finishing = $state(false);
  let heading = $state<HTMLHeadingElement | null>(null);
  // The failure the server came back with, kept while the person corrects it.
  let failure = $state<string | undefined>(undefined);
  let credentials: { username: string; password: string } | undefined;

  const contract = $derived(flow.contract);
  const steps = $derived(stepsOf(contract, draft.path, draft.mode));
  // A reload forgets the configuration until the code is read again, so a
  // step past the code waits on the code step meanwhile.
  const step = $derived<StepId>(
    steps.includes(draft.step) ? draft.step : (steps.at(-1) ?? 'welcome'),
  );
  const index = $derived(steps.indexOf(step));
  // The furthest step the person may open again without pressing Next.
  const reached = $derived(reachedOf(steps, draft.visited, index));
  const summaryReached = $derived(
    step !== 'summary' &&
      steps.at(-1) === 'summary' &&
      reached === steps.length - 1,
  );
  const section = $derived(sectionOfStep(step));
  const waiting = $derived(flow.phase !== 'editing' && flow.phase !== 'failed');

  const current = $derived(
    typeof location === 'undefined' ? undefined : new URL(location.href),
  );
  // A server confirmed with its web app off serves no page after the restart,
  // so the wizard says so before the confirm and ends without the fleet.
  const webOff = $derived.by(() => {
    const key = contract?.keys?.find(
      (each) => each.path === 'core.frontend.enabled',
    );

    return key !== undefined && shownOf(key, draft.answers) === false;
  });

  // The address the web app listens on, which printers dial while they have
  // no listener of their own. Undefined when the operator's file declares how
  // printers connect.
  const printersApi = $derived.by(() => {
    if (contract?.printers !== true) return undefined;
    const key = contract.keys?.find((each) => each.path === 'core.api.addr');

    return key === undefined ? undefined : String(shownOf(key, draft.answers));
  });
  // The browser sees a container's published web port, while the server sees
  // only the port it listens on inside the container. An explicit new
  // listener address wins because it is where the server starts next.
  const webPort = $derived(
    draft.answers['core.api.addr'] === undefined
      ? location.port || (location.protocol === 'https:' ? '443' : '80')
      : undefined,
  );
  const encrypted = $derived(
    encryptionDecision(draft.encryption, printersApi, webPort),
  );
  // The address the add-printer page gives printers, from the encryption
  // step, unless the person wrote one of their own on the advanced path.
  const printerAddress = $derived(
    draft.answers['core.frontend.printer_address'] === undefined
      ? encrypted.printerAddress
      : undefined,
  );

  const moves = $derived.by(() => {
    if (contract === undefined || current === undefined) return undefined;

    return movedOrigin(
      contract,
      { ...draft.answers, ...encrypted.answers },
      current,
    );
  });

  // Where the issuer sends people back to: the address the server answers on
  // after the restart, which is this page's unless the answers move it.
  const redirectFallback = $derived(
    current === undefined
      ? undefined
      : `${moves ?? current.origin}${OIDC_CALLBACK}`,
  );

  // What the confirm sends, after the operator's file has had its say. The
  // summary names what the file keeps, so it reads the same value.
  const settled = $derived.by(() => {
    if (contract === undefined) return undefined;
    const answers = Object.fromEntries(
      Object.entries(
        configurationOf(contract, draft.answers, draft.mode),
      ).filter(([path]) => !IDENTITY_KEYS.includes(path)),
    );

    return settleLocked(contract, {
      ...answers,
      ...oidcAnswers(draft, redirectFallback),
      ...encrypted.answers,
      ...(printerAddress === undefined
        ? {}
        : { 'core.frontend.printer_address': printerAddress }),
      ...(draft.encryption.printers &&
      draft.answers['core.frontend.printer_custom_ca'] === undefined
        ? { 'core.frontend.printer_custom_ca': true }
        : {}),
    });
  });

  const STEP_LABEL = $derived<Record<string, string>>({
    welcome: t('setupWizard.steps.welcome'),
    code: t('setupWizard.steps.code'),
    path: t('setupWizard.steps.path'),
    identity: t('setupWizard.steps.identity'),
    admission: t('setupWizard.steps.admission'),
    storage: t('setupWizard.steps.storage'),
    encryption: t('setupWizard.steps.encryption'),
    summary: t('setupWizard.steps.summary'),
  });

  const labelOfStep = (id: StepId): string => {
    const of = sectionOfStep(id);

    return of === undefined ? (STEP_LABEL[id] ?? id) : sectionShort(of);
  };

  const stepper = $derived(steps.map((id) => ({ id, label: labelOfStep(id) })));

  const TITLE = $derived<Record<string, string>>({
    welcome: t('setupWizard.welcome.title'),
    code: t('setupWizard.code.title'),
    path: t('setupWizard.path.title'),
    identity: t('setupWizard.identity.title'),
    admission: t('setupWizard.admission.title'),
    storage: t('setupWizard.storage.title'),
    encryption: t('setupWizard.encryption.title'),
    summary: t('setupWizard.summary.title'),
  });

  const LEAD = $derived<Record<string, string>>({
    welcome: t('setupWizard.welcome.lead'),
    code: t('setupWizard.code.lead'),
    path: t('setupWizard.path.lead'),
    identity: t('setupWizard.identity.lead'),
    admission: t('setupWizard.admission.lead'),
    storage: t('setupWizard.storage.lead'),
    encryption: t('setupWizard.encryption.lead'),
    summary: t('setupWizard.summary.lead', {
      file: contract?.setup_file.path ?? '',
    }),
  });

  const waitTitle = $derived.by(() => {
    if (flow.phase === 'ready') return t('setupWizard.done.title');
    if (flow.phase === 'timeout') return t('setupWizard.timeout.title');

    return t('setupWizard.wait.title');
  });

  const title = $derived(
    section === undefined ? (TITLE[step] ?? '') : sectionTitle(section),
  );
  const lead = $derived(
    section === undefined ? (LEAD[step] ?? '') : sectionLead(section),
  );

  // The page of the configuration section each step is about, relative to
  // the documentation's address (ADR-20261015-ejz rule 41).
  const GUIDE: Readonly<Record<string, string>> = {
    welcome: 'config/',
    identity: 'config/sign-in/',
    admission: 'config/printers/',
    storage: 'config/resources/',
    encryption: 'config/encryption/',
    'section:network': 'config/network/',
    'section:look': 'config/web-app/',
    'section:access': 'config/sign-in/',
    'section:database': 'config/database/',
    'section:logging': 'config/logging/',
    'section:server': 'config/server/',
  };
  const guide = $derived(
    docsHref === undefined || GUIDE[step] === undefined
      ? undefined
      : `${docsHref}${GUIDE[step]}`,
  );

  onMount(() => {
    failure = lastFailure;
    if (draft.code.trim() !== '' && draft.step !== 'welcome') {
      void flow.readContract(draft.code.trim());
    }
  });

  // The wizard is drawn before the app shell, so it applies the theme itself.
  $effect(() => {
    applyTheme(theme.dark);
  });

  // The heading takes the focus on every step, so a screen reader starts
  // reading the new question and the keyboard starts at the top of it.
  $effect(() => {
    // Read so the effect runs again when the step changes.
    const focused = step;
    if (focused.length > 0) heading?.focus();
  });

  // The server moved to another address, which this page cannot read from.
  $effect(() => {
    if (flow.phase !== 'moved' || moves === undefined) return;
    const target = moves;
    const timer = setTimeout(() => {
      location.assign(target);
    }, 4000);

    return () => {
      clearTimeout(timer);
    };
  });

  // The server came back on the configuration it had: a new code is needed
  // and the answers stay as they were.
  $effect(() => {
    if (flow.phase !== 'failed') return;
    failure = flow.failure?.message;
    draft.code = '';
    draft.step = 'code';
    flow.edit();
  });

  // The language a person picks for the wizard is a good guess for the web
  // app's own default, so it becomes the answer until they give another.
  const chooseLanguage = (tag: string): void => {
    onlanguage(tag);
    if (draft.answers['core.frontend.default_language'] === undefined) {
      draft.answer('core.frontend.default_language', tag);
    }
  };

  const goTo = (target: StepId | undefined): void => {
    if (target === undefined) return;
    attempted = false;
    draft.step = target;
  };

  // Opens a step already reached. The checks Next runs still hold on the way
  // forward: the first step they now refuse is where the jump ends, with its
  // faults shown. The storage folder is checked again by the confirm.
  const jump = (target: StepId | undefined): void => {
    if (target === undefined) return;
    const to = steps.indexOf(target);
    if (to < 0 || to > reached) return;
    const refused = steps
      .slice(index, to)
      .find((each) => !stepIsComplete(each, draft, contract));
    goTo(refused ?? target);
    attempted = refused !== undefined;
  };

  const next = async (): Promise<void> => {
    attempted = true;
    if (!stepIsComplete(step, draft, contract)) return;
    if (step === 'code' && !(await flow.readContract(draft.code.trim())))
      return;
    if (
      step === 'storage' &&
      contract !== undefined &&
      !contract.storage.declared &&
      !(await flow.checkStorage(
        draft.code.trim(),
        (draft.storageRoot ?? contract.storage.root).trim(),
      ))
    ) {
      return;
    }
    const after = stepsOf(flow.contract, draft.path, draft.mode);
    goTo(after[after.indexOf(step) + 1]);
  };

  const confirm = (): void => {
    if (
      contract === undefined ||
      settled === undefined ||
      settled.blocked.length > 0
    ) {
      return;
    }
    const required = draft.mode === 'required';
    credentials = required
      ? { username: draft.username.trim(), password: draft.password }
      : undefined;
    void flow.confirm(
      {
        code: draft.code.trim(),
        // The server knows two modes: a provider is people signing in.
        mode: draft.mode === 'open' ? 'open' : 'required',
        admission: draft.admission,
        ...(credentials === undefined ? {} : { account: credentials }),
        configuration: settled.configuration,
        ...(encrypted.printers === undefined || !contract.printers
          ? {}
          : { printers: encrypted.printers }),
        ...(draft.historyOn
          ? { history: { retention: draft.historyRetention } }
          : {}),
        ...(draft.mqttOn
          ? {
              mqtt: {
                url: draft.mqttUrl.trim(),
                ...(draft.mqttUser.trim() === ''
                  ? {}
                  : { username: draft.mqttUser.trim() }),
                ...(draft.mqttPassword === ''
                  ? {}
                  : { password: draft.mqttPassword }),
              },
            }
          : {}),
        ...(contract.storage.declared
          ? {}
          : {
              storage: {
                root: (draft.storageRoot ?? contract.storage.root).trim(),
              },
            }),
      },
      moves === undefined,
    );
  };

  const finish = async (): Promise<void> => {
    finishing = true;
    draft.clear();
    await onfinish(credentials);
  };
</script>

<div
  class="@container/setup min-h-svh bg-background text-foreground not-dark:bg-shell-canvas"
  data-part="setup-wizard"
>
  <div class="flex min-h-svh">
    <aside
      class="hidden w-88 shrink-0 flex-col justify-between gap-10 border-e border-border bg-muted px-10 py-12 not-dark:border-panel-outline not-dark:bg-shell-chrome @4xl/setup:flex"
    >
      <div class="flex flex-col gap-10">
        <div class="flex flex-col gap-3">
          <ConnectLogo size="md" label="Prusa Connect" />
          <p class="text-sm text-muted-foreground">
            {t('setupWizard.aside.tagline')}
          </p>
        </div>
        <StepList
          label={t('setupWizard.steps.label')}
          steps={stepper}
          current={index}
          {reached}
          onpick={(id: string) => jump(steps.find((each) => each === id))}
        />
      </div>
      <div class="flex flex-col gap-4">
        <PrinterIcon model="core_one" class="w-40 opacity-80" />
        <p class="font-mono text-xs text-muted-foreground">
          {#if host !== undefined && host !== ''}{host}{/if}
          {#if version !== undefined}
            <span class="block"
              >{t('setupWizard.aside.version', { version })}</span
            >
          {/if}
        </p>
      </div>
    </aside>

    <main
      class="flex min-w-0 flex-1 justify-center px-4 py-8 @4xl/setup:px-16 @4xl/setup:py-16"
    >
      <div class="flex w-full max-w-xl flex-col gap-8">
        <div class="flex items-center justify-between gap-4">
          <span class="shrink-0 @4xl/setup:invisible"
            ><ConnectLogo size="sm" label="" /></span
          >
          <WizardToolbar onlanguage={chooseLanguage} />
        </div>
        <div class="flex flex-col gap-2 @4xl/setup:hidden">
          <span class="text-sm text-muted-foreground">
            {t('setupWizard.steps.position', {
              current: index + 1,
              total: steps.length,
            })}
          </span>
          <Progress
            size="sm"
            value={Math.round(((index + 1) / steps.length) * 100)}
            aria-label={t('setupWizard.steps.label')}
          />
        </div>

        <header class="flex flex-col gap-3">
          <h1
            bind:this={heading}
            tabindex="-1"
            class="text-2xl font-semibold outline-none"
          >
            {waiting ? waitTitle : title}
          </h1>
          {#if !waiting}
            <p class="text-muted-foreground">{lead}</p>
            {#if guide !== undefined}
              <Button variant="link" size="inline-icon">
                {#snippet child({ props }: { props: Record<string, unknown> })}
                  <a
                    {...props}
                    href={guide}
                    target="_blank"
                    rel="noopener noreferrer"
                    data-part="step-guide"
                  >
                    <BookOpenIcon class="size-4" aria-hidden="true" />
                    {t('setupWizard.steps.guide')}
                  </a>
                {/snippet}
              </Button>
            {/if}
          {/if}
        </header>

        {#if failure !== undefined && !waiting && step !== 'summary'}
          <Alert variant="destructive">
            <AlertTitle>{t('setupWizard.failed.title')}</AlertTitle>
            <AlertDescription>
              <p>{t('setupWizard.failed.lead')}</p>
              <p class="mt-2 font-mono text-xs">{failure}</p>
            </AlertDescription>
          </Alert>
        {/if}

        {#if waiting}
          <StepWait
            phase={flow.phase}
            elapsed={flow.elapsed}
            {moves}
            busy={finishing}
            failure={flow.failure?.message}
            {webOff}
            onretry={() => void flow.keepWaiting()}
            onedit={() => {
              flow.edit();
              goTo('summary');
            }}
            onfinish={() => void finish()}
          />
        {:else if step === 'welcome'}
          <StepWelcome {docsHref} onstart={() => goTo('code')} />
        {:else if step === 'code'}
          <StepCode {draft} error={flow.readError} showErrors={attempted} />
        {:else if step === 'path'}
          <StepPath {draft} />
        {:else if step === 'identity'}
          <StepIdentity
            {draft}
            {docsHref}
            {redirectFallback}
            {flow}
            showErrors={attempted}
          />
        {:else if step === 'admission'}
          <StepAdmission {draft} />
        {:else if step === 'storage' && contract !== undefined}
          <StepStorage
            {draft}
            fallback={contract.storage.root}
            showErrors={attempted}
            check={flow.storage}
            declared={contract.storage.declared}
            error={flow.storageError}
            {docsHref}
          />
        {:else if step === 'encryption' && contract !== undefined}
          <StepEncryption
            {draft}
            {flow}
            {contract}
            {docsHref}
            showErrors={attempted}
          />
        {:else if section !== undefined && contract !== undefined}
          <StepSection
            {draft}
            {contract}
            keys={keysOf(contract, section, draft.path ?? 'basic', draft.mode)}
            file={contract.config_file.path}
            {docsHref}
            showErrors={attempted}
            mqtt={section === 'network'}
            mqttGuide={docsHref === undefined ? undefined : `${docsHref}mqtt/`}
          />
        {:else if step === 'summary' && contract !== undefined}
          <StepSummary
            {draft}
            {contract}
            error={flow.error}
            {failure}
            {moves}
            {printerAddress}
            {webOff}
            kept={settled?.kept ?? []}
            blocked={settled?.blocked ?? []}
            onedit={goTo}
          />
        {/if}

        {#if !waiting && step !== 'welcome'}
          <nav
            class="flex items-center justify-between gap-3 border-t border-border pt-6"
          >
            <Button variant="ghost" onclick={() => goTo(steps[index - 1])}>
              <ArrowLeftIcon aria-hidden="true" />
              {t('setupWizard.nav.back')}
            </Button>
            {#if step === 'summary'}
              <Button
                size="lg"
                disabled={(settled?.blocked.length ?? 0) > 0}
                onclick={confirm}>{t('setupWizard.summary.confirm')}</Button
              >
            {:else}
              <div class="flex flex-wrap items-center justify-end gap-3">
                {#if summaryReached}
                  <Button
                    variant="secondary"
                    data-part="back-to-summary"
                    disabled={flow.reading || flow.checking}
                    onclick={() => jump('summary')}
                  >
                    {t('setupWizard.nav.summary')}
                  </Button>
                {/if}
                <Button
                  disabled={flow.reading || flow.checking}
                  onclick={() => void next()}
                >
                  {t('setupWizard.nav.next')}
                  <ArrowRightIcon aria-hidden="true" />
                </Button>
              </div>
            {/if}
          </nav>
        {/if}
      </div>
    </main>
  </div>
</div>
