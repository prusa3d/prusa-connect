<!-- The one page a signed-out browser gets. -->
<script lang="ts">
  import { describeFault, t } from '../../i18n';
  import {
    Alert,
    AlertDescription,
    AlertTitle,
    Button,
    Card,
    CardContent,
    CardHeader,
    ConnectLogo,
    FaultAlert,
    Field,
    Input,
    PasswordInput,
  } from '../../primitives';
  import { applyTheme, theme } from '../../styles/theme.svelte';
  import type { SignInFailure, SignInScreenProps } from './types';

  // Each reason the server names, and the sentence that says it.
  const FAILURE_NOTES = {
    provider: 'signIn.failed.provider',
    id_token: 'signIn.failed.idToken',
    subject: 'signIn.failed.subject',
    expired: 'signIn.failed.expired',
    failed: 'signIn.failed.failed',
  } as const satisfies Record<SignInFailure, string>;

  let {
    host,
    instanceName,
    ended,
    busy = false,
    failure,
    error,
    redirectProvider,
    onRedirect,
    onSignIn,
  }: SignInScreenProps = $props();

  let username = $state('');
  let password = $state('');
  // An empty field is only wrong once someone has tried to send it.
  let attempted = $state(false);

  // `Field` takes a string, and an empty one is no error at all.
  const usernameError = $derived(
    attempted && username === '' ? t('signIn.missingUsername') : '',
  );
  const passwordError = $derived(
    attempted && password === '' ? t('signIn.missingPassword') : '',
  );

  const fault = $derived(
    error === undefined ? undefined : describeFault(error),
  );

  // The page is drawn before the app shell, so it applies the theme itself.
  $effect(() => {
    applyTheme(theme.dark);
  });

  const submit = (event: SubmitEvent): void => {
    event.preventDefault();
    attempted = true;
    if (username === '' || password === '') return;
    onSignIn(username, password);
  };
</script>

<!-- The shell's canvas and header strip, so signing in already looks like the
     app it opens. -->
<div
  class="@container/sign-in flex min-h-svh flex-col bg-background text-foreground not-dark:bg-shell-canvas"
>
  <header
    class="flex items-center justify-between gap-x-4 border-b border-border bg-muted px-4 py-4 not-dark:border-panel-outline not-dark:bg-shell-chrome @xl/sign-in:px-6"
  >
    <ConnectLogo size="sm" label="Prusa Connect" />
    {#if instanceName && instanceName !== 'Prusa Connect'}
      <p class="min-w-0 truncate text-sm text-muted-foreground">
        {instanceName}
      </p>
    {/if}
  </header>
  <!-- Not a <main>: that landmark is the signed-in shell, and the browser
       journeys wait for it as the sign that a session has started. -->
  <div class="flex flex-1 items-center justify-center px-4 py-10">
    <div class="w-full max-w-md">
      <Card>
        <CardHeader>
          <h1 class="text-xl leading-none font-semibold">
            {t('signIn.title')}
          </h1>
          {#if host !== undefined && host !== ''}
            <p class="font-mono text-xs text-muted-foreground">{host}</p>
          {/if}
        </CardHeader>
        <CardContent class="flex flex-col gap-4">
          {#if ended === 'refused'}
            <Alert variant="warning">
              <AlertTitle>{t('signIn.expired.title')}</AlertTitle>
              <AlertDescription>{t('signIn.expired.note')}</AlertDescription>
            </Alert>
          {/if}
          {#if failure !== undefined}
            <Alert variant="destructive">
              <AlertTitle>{t('signIn.failed.title')}</AlertTitle>
              <AlertDescription>{t(FAILURE_NOTES[failure])}</AlertDescription>
            </Alert>
          {/if}
          {#if fault !== undefined}
            <FaultAlert {fault} />
          {/if}
          {#if redirectProvider !== undefined}
            <Button
              class="w-full"
              disabled={busy}
              onclick={() => onRedirect?.()}
            >
              {t('signIn.redirect', { provider: redirectProvider })}
            </Button>
          {:else}
            <form
              class="flex flex-col gap-4 [&_[data-slot=password-input]]:max-w-none"
              onsubmit={submit}
            >
              <Field
                label={t('signIn.username')}
                error={usernameError}
                control={Input}
                controlProps={{
                  value: username,
                  type: 'text',
                  autocomplete: 'username',
                  oninput: (
                    event: Event & { currentTarget: HTMLInputElement },
                  ) => {
                    username = event.currentTarget.value;
                  },
                }}
              />
              <Field
                label={t('signIn.password')}
                error={passwordError}
                control={PasswordInput}
                controlProps={{
                  value: password,
                  label: t('signIn.password'),
                  autocomplete: 'current-password',
                  oninput: (
                    event: Event & { currentTarget: HTMLInputElement },
                  ) => {
                    password = event.currentTarget.value;
                  },
                }}
              />
              <Button type="submit" class="w-full" disabled={busy}>
                {t('signIn.submit')}
              </Button>
            </form>
          {/if}
        </CardContent>
      </Card>
    </div>
  </div>
</div>
