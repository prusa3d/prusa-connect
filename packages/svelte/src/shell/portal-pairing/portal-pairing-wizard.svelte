<!--
  Pairs this server with the Beta Portal (RFC 8628 device flow). The steps run
  down the side: start, confirm the code on the portal, the team's approval
  when the product asks for it, and paired. The server polls the portal; this
  only asks the server, on the interval the server names, while it is shown.
-->
<script lang="ts">
  import PauseCircleIcon from '@lucide/svelte/icons/circle-pause';
  import { onMount } from 'svelte';

  import type { PortalPairingPhase } from '../../data';
  import { describeFault, t } from '../../i18n';
  import {
    Alert,
    AlertDescription,
    AlertTitle,
    Badge,
    type BadgeVariant,
    Button,
    ConfirmDialog,
    FaultAlert,
    Skeleton,
    StepList,
  } from '../../primitives';
  import PairingCode from './pairing-code.svelte';
  import type { PortalPairingWizardProps } from './types';

  let { pairing }: PortalPairingWizardProps = $props();

  const headingId = $props.id();
  let now = $state(Date.now());

  const phase = $derived(pairing.phase);
  const status = $derived(pairing.status);
  const working = $derived(pairing.busy !== undefined);
  const fault = $derived(
    pairing.error === undefined ? undefined : describeFault(pairing.error),
  );
  // These phases explain the refusal in their own words and show its detail.
  const ownFault = $derived(phase === 'clock' || phase === 'unreachable');

  const badges: Record<
    Exclude<PortalPairingPhase, 'loading'>,
    { label: string; variant: BadgeVariant }
  > = $derived({
    intro: { label: t('portalPairing.badge.intro'), variant: 'secondary' },
    code: { label: t('portalPairing.badge.waiting'), variant: 'warning' },
    confirm: { label: t('portalPairing.badge.waiting'), variant: 'warning' },
    approval: { label: t('portalPairing.badge.approval'), variant: 'warning' },
    paired: { label: t('portalPairing.badge.paired'), variant: 'success' },
    expired: { label: t('portalPairing.badge.expired'), variant: 'secondary' },
    denied: { label: t('portalPairing.badge.denied'), variant: 'destructive' },
    revoked: { label: t('portalPairing.badge.revoked'), variant: 'warning' },
    clock: { label: t('portalPairing.badge.clock'), variant: 'destructive' },
    unreachable: {
      label: t('portalPairing.badge.unreachable'),
      variant: 'destructive',
    },
  });
  const suspended = $derived(
    phase === 'paired' && status?.access_suspended === true,
  );
  const badge = $derived.by(() => {
    if (phase === 'loading') return undefined;
    if (suspended)
      return {
        label: t('portalPairing.badge.suspended'),
        variant: 'warning' as const,
      };

    return badges[phase];
  });

  const approving = $derived(status?.step === 'awaiting_approval');
  const steps = $derived([
    { id: 'start', label: t('portalPairing.steps.start') },
    { id: 'confirm', label: t('portalPairing.steps.confirm') },
    ...(approving
      ? [{ id: 'approve', label: t('portalPairing.steps.approve') }]
      : []),
    { id: 'done', label: t('portalPairing.steps.done') },
  ]);
  const current = $derived.by(() => {
    if (status?.state === 'paired') return steps.length;
    if (status?.state === 'pending') return approving ? 2 : 1;

    return 0;
  });

  const remaining = $derived.by(() => {
    if (status?.expires_at === undefined) return undefined;
    const seconds = Math.ceil(
      (new Date(status.expires_at).getTime() - now) / 1000,
    );

    return Number.isFinite(seconds) ? Math.max(0, seconds) : undefined;
  });
  const portalHref = $derived(
    status?.verification_uri_complete ?? status?.verification_uri ?? '',
  );

  const pairedAs = $derived.by(() => {
    const name = status?.identity?.name;
    if (name) return t('portalPairing.paired.as', { name });
    const product = status?.product?.name;
    const device = status?.device?.identifier;
    if (product && device)
      return t('portalPairing.paired.device', { device, product });
    if (product) return t('portalPairing.paired.product', { product });

    return t('portalPairing.paired.unknown');
  });

  let confirming = $state(false);

  onMount(() => pairing.watch());

  // The countdown under the code. It only redraws, it asks the server nothing.
  $effect(() => {
    if (status?.state !== 'pending') return;
    const ticker = setInterval(() => {
      now = Date.now();
    }, 1000);

    return () => {
      clearInterval(ticker);
    };
  });
</script>

{#snippet panel(title: string, body: string, extra?: string)}
  <h5 class="text-sm font-medium">{title}</h5>
  <p class="text-sm text-muted-foreground">{body}</p>
  {#if extra}<p class="text-sm text-muted-foreground">{extra}</p>{/if}
{/snippet}

{#snippet again(label: string)}
  <Button
    class="self-start"
    disabled={working}
    onclick={() => void pairing.start()}
  >
    {label}
  </Button>
{/snippet}

<section
  class="@container/pairing"
  aria-labelledby={headingId}
  aria-busy={phase === 'loading'}
>
  <div class="flex flex-wrap items-center justify-between gap-3">
    <div>
      <h3 id={headingId} class="text-body font-medium">
        {t('portalPairing.heading')}
      </h3>
      <p class="text-caption text-muted-foreground">
        {t('portalPairing.description')}
      </p>
    </div>
    {#if badge}
      <Badge variant={badge.variant} data-phase={phase}>{badge.label}</Badge>
    {/if}
    <!-- Always in the page, so a screen reader hears each step change. A
         region drawn together with its text is not announced. -->
    <span class="sr-only" role="status">{badge?.label ?? ''}</span>
  </div>

  {#if phase === 'loading'}
    <div class="mt-4" aria-label={t('portalPairing.loading')}>
      <Skeleton class="h-24 w-full" />
    </div>
  {:else}
    <div class="mt-4 grid gap-4 @md/pairing:grid-cols-3">
      <StepList
        label={t('portalPairing.steps.label')}
        {steps}
        {current}
        connected
      />
      <div
        class="flex flex-col gap-3 @md/pairing:col-span-2"
        data-slot="pairing-phase"
        data-phase={phase}
      >
        {#if !ownFault}
          <FaultAlert {fault} />
        {/if}

        {#if phase === 'intro'}
          {@render panel(
            t('portalPairing.intro.title'),
            t('portalPairing.intro.body'),
            t('portalPairing.intro.private'),
          )}
          {@render again(t('portalPairing.intro.start'))}
        {:else if phase === 'code' || phase === 'confirm'}
          <div class="flex flex-col gap-1">
            {#if phase === 'confirm'}
              {@render panel(
                t('portalPairing.confirm.title'),
                t('portalPairing.confirm.body'),
              )}
            {:else}
              <h5 class="text-sm font-medium">
                {t('portalPairing.code.title')}
              </h5>
            {/if}
          </div>
          <PairingCode
            code={status?.user_code ?? ''}
            href={portalHref}
            address={status?.verification_uri}
            {remaining}
            compact={phase === 'confirm'}
            openLabel={phase === 'confirm'
              ? t('portalPairing.confirm.reopen')
              : t('portalPairing.code.open')}
            onopen={() => {
              pairing.markOpened();
            }}
          />
          <Button
            class="self-start"
            variant="ghost"
            disabled={working}
            onclick={() => void pairing.disconnect()}
          >
            {t('portalPairing.code.cancel')}
          </Button>
        {:else if phase === 'approval'}
          <div class="flex flex-col gap-1">
            {@render panel(
              t('portalPairing.approval.title'),
              t('portalPairing.approval.body'),
              t('portalPairing.approval.expiry'),
            )}
          </div>
          <Button
            class="self-start"
            variant="ghost"
            disabled={working}
            onclick={() => void pairing.disconnect()}
          >
            {t('portalPairing.code.cancel')}
          </Button>
        {:else if phase === 'paired'}
          <h5 class="text-sm font-medium">{pairedAs}</h5>
          {#if status?.identity?.name && status.product?.name}
            <p class="text-xs text-muted-foreground">{status.product.name}</p>
          {/if}
          {#if suspended}
            <Alert variant="warning" data-part="access-suspended">
              <PauseCircleIcon aria-hidden="true" />
              <AlertTitle>{t('portalPairing.suspended.title')}</AlertTitle>
              <AlertDescription
                >{t('portalPairing.suspended.body')}</AlertDescription
              >
            </Alert>
          {:else}
            <p class="text-sm text-muted-foreground">
              {t('portalPairing.paired.body')}
            </p>
          {/if}
          <ConfirmDialog
            title={t('portalPairing.disconnect.title')}
            description={t('portalPairing.disconnect.description')}
            confirmLabel={t('portalPairing.paired.disconnect')}
            open={confirming}
            onOpenChange={(open: boolean) => {
              confirming = open;
            }}
            onConfirm={() => {
              confirming = false;
              void pairing.disconnect();
            }}
          />
          <Button
            class="self-start"
            variant="destructive"
            disabled={working}
            onclick={() => {
              confirming = true;
            }}
          >
            {t('portalPairing.paired.disconnect')}
          </Button>
        {:else if phase === 'expired'}
          {@render panel(
            t('portalPairing.expired.title'),
            t('portalPairing.expired.body'),
          )}
          {@render again(t('portalPairing.again'))}
        {:else if phase === 'denied'}
          {@render panel(
            t('portalPairing.denied.title'),
            t('portalPairing.denied.body'),
          )}
          {@render again(t('portalPairing.again'))}
        {:else if phase === 'revoked'}
          {@render panel(
            t('portalPairing.revoked.title'),
            t('portalPairing.revoked.body'),
            status?.identity?.name
              ? t('portalPairing.revoked.was', { name: status.identity.name })
              : undefined,
          )}
          {@render again(t('portalPairing.revoked.pair'))}
        {:else}
          {@render panel(
            phase === 'clock'
              ? t('portalPairing.clock.title')
              : t('portalPairing.unreachable.title'),
            phase === 'clock'
              ? t('portalPairing.clock.body')
              : t('portalPairing.unreachable.body'),
          )}
          {#if fault?.detail}
            <p class="text-xs text-muted-foreground">{fault.detail}</p>
          {/if}
          {#if fault?.code}
            <code class="text-xs text-muted-foreground">{fault.code}</code>
          {/if}
          <Button
            class="self-start"
            disabled={working}
            onclick={() => void pairing.retry()}
          >
            {t('portalPairing.retry')}
          </Button>
        {/if}
      </div>
    </div>
  {/if}
</section>
