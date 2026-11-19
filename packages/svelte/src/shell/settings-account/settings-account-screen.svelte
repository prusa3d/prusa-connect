<script lang="ts">
  import { Avatar } from 'bits-ui';
  import InfoIcon from '@lucide/svelte/icons/info';
  import editIcon from './assets/profile-edit.svg';
  import PhotoCropDialog from './photo-crop-dialog.svelte';
  import { subjectInitials } from '../user-menu/initials';
  import { describeFault, formatDate, t } from '../../i18n';
  import {
    Button,
    FaultAlert,
    Field,
    Input,
    PasswordInput,
    SettingsPane,
    SettingsRow,
    Skeleton,
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
  } from '../../primitives';
  import type { AccountIdentity, SettingsAccountScreenProps } from './types';

  let {
    source,
    onSaveIdentity,
    identityPreview = false,
    identity,
    identityLoading = false,
    changing = false,
    fault,
    changed = false,
    passwordless = false,
    onChangePassword,
  }: SettingsAccountScreenProps = $props();

  // What `PasswordBody` documents on `set-account-password`.
  const SHORTEST = 8;
  const LONGEST = 72;

  let roleHelpOpen = $state(false);
  let current = $state('');
  let next = $state('');
  let confirm = $state('');

  const waiting = $derived(source.status === 'unknown' || identityLoading);
  const subject = $derived(source.principal?.id);
  let identityEditor = $state<{ owner: string | undefined; open: boolean }>({
    owner: undefined,
    open: false,
  });
  const editingIdentity = $derived(
    identityEditor.open && identityEditor.owner === subject,
  );
  let draftName = $state('');
  let draftUsername = $state('');
  let savingIdentity = $state(false);
  let identityFault = $state<unknown>();

  const editIdentity = () => {
    draftName = identity?.displayName ?? '';
    draftUsername = identity?.username ?? '';
    identityFault = undefined;
    identityEditor.owner = subject;
    identityEditor.open = true;
  };
  const saveIdentity = async (event: SubmitEvent) => {
    event.preventDefault();
    if (
      !editingIdentity ||
      savingIdentity ||
      !onSaveIdentity ||
      draftUsername.trim() === ''
    )
      return;
    savingIdentity = true;
    identityFault = undefined;
    try {
      await onSaveIdentity({
        displayName: draftName.trim(),
        username: draftUsername.trim(),
      });
      identityEditor.open = false;
    } catch (error) {
      identityFault = error;
    } finally {
      savingIdentity = false;
    }
  };

  const avatarName = $derived(
    [identity?.displayName, identity?.username, subject].find(
      (name) => name !== undefined && name.trim() !== '',
    ) ?? '',
  );
  const initials = $derived(subjectInitials(avatarName));
  const photoId = $props.id();
  const photoInputId = `${photoId}-input`;
  const photoNoticeId = `${photoId}-notice`;
  let photoInput: HTMLInputElement | undefined = $state();
  let selectedPhoto = $state<{ owner: string | undefined; file: File }>();
  let draftPhoto = $state<{ owner: string | undefined; file: File }>();
  $effect(() => {
    if (draftPhoto !== undefined && draftPhoto.owner !== subject)
      draftPhoto = undefined;
  });
  let previewUrl = $state<string>();
  let photoError = $state<'type' | 'read'>();
  const PHOTO_MESSAGES = {
    type: 'settingsAccount.identity.photoTypeError',
    read: 'settingsAccount.identity.photoReadError',
  };
  const localPreview = $derived(
    selectedPhoto?.owner === subject && previewUrl !== undefined,
  );
  const photoNoticeRole = $derived.by(() => {
    if (photoError !== undefined) return 'alert';
    return localPreview ? 'status' : undefined;
  });
  const photoNoticeKey = $derived.by(() => {
    if (photoError !== undefined) return PHOTO_MESSAGES[photoError];
    return localPreview ? 'settingsAccount.identity.photoPreview' : undefined;
  });
  const avatarSource = $derived(
    localPreview ? previewUrl : identity?.avatarUrl,
  );

  $effect(() => {
    const file = selectedPhoto?.file;
    if (file === undefined) return;
    const url = URL.createObjectURL(file);
    previewUrl = url;
    return () => URL.revokeObjectURL(url);
  });

  const pickPhoto = (): void => {
    const file = photoInput?.files?.[0];
    if (photoInput !== undefined) photoInput.value = '';
    if (file === undefined) return;
    if (
      !['image/jpeg', 'image/png', 'image/webp', 'image/gif'].includes(
        file.type,
      )
    ) {
      photoError = 'type';
      return;
    }
    photoError = undefined;
    draftPhoto = { owner: subject, file };
  };

  // The roles pane's own words.
  const ROLES: Readonly<Record<string, string>> = {
    admin: 'settingsPeople.role.admin',
    operator: 'settingsPeople.role.operator',
    viewer: 'settingsPeople.role.viewer',
  };

  const added = $derived(
    identity?.createdAt === undefined
      ? undefined
      : formatDate(identity.createdAt),
  );

  const ROWS = [
    {
      id: 'name',
      label: () => t('settingsAccount.identity.name'),
      value: (of?: AccountIdentity) => of?.displayName,
    },
    {
      id: 'username',
      label: () => t('settingsAccount.identity.username'),
      value: (of?: AccountIdentity) => of?.username,
    },
    {
      id: 'role',
      label: () => t('settingsAccount.identity.role'),
      value: () => roleText,
    },
    {
      id: 'added',
      label: () => t('settingsAccount.identity.added'),
      value: () => added,
    },
  ] as const;

  const roleText = $derived.by(() => {
    const key = identity?.role === undefined ? undefined : ROLES[identity.role];

    // A role this build has no word for is printed as the core wrote it.
    return key === undefined ? identity?.role : t(key);
  });

  // `Field` takes a string, and an empty one is no error at all.
  const lengthError = $derived(
    next !== '' && (next.length < SHORTEST || next.length > LONGEST)
      ? t('settingsAccount.password.length')
      : '',
  );
  const mismatchError = $derived(
    confirm !== '' && confirm !== next
      ? t('settingsAccount.password.mismatch')
      : '',
  );

  const blocked = $derived(
    changing ||
      current === '' ||
      next === '' ||
      confirm === '' ||
      lengthError !== '' ||
      mismatchError !== '',
  );

  const refusal = $derived(
    fault === undefined ? undefined : describeFault(fault),
  );

  // A refusal keeps what was typed.
  $effect(() => {
    if (changed) {
      current = '';
      next = '';
      confirm = '';
    }
  });

  const submit = (event: SubmitEvent): void => {
    event.preventDefault();
    if (blocked) return;
    onChangePassword(current, next);
  };
</script>

{#snippet subjectRow()}
  <SettingsRow
    class={[
      !identityLoading &&
        added === undefined &&
        'mt-form-gap border-t border-border pt-section-gap',
    ]}
    label={t('settingsAccount.identity.subject')}
  >
    {#snippet control()}
      {#if waiting}
        <Skeleton
          class="h-5 w-40"
          role="status"
          aria-label={t('settingsAccount.loading')}
        />
      {:else if subject !== undefined}
        <span
          class="min-w-0 font-mono text-xs break-words text-muted-foreground"
          >{subject}</span
        >
      {/if}
    {/snippet}
  </SettingsRow>
{/snippet}

{#snippet roleHelp()}
  <span class="inline-flex">
    <TooltipProvider>
      <Tooltip bind:open={roleHelpOpen} disableCloseOnTriggerClick>
        <TooltipTrigger>
          {#snippet child({ props })}
            <Button
              {...props}
              variant="ghost"
              size="sm"
              aria-label={t('settingsAccount.identity.role')}
              class="size-control-height p-0"
              onclick={() => {
                roleHelpOpen = true;
              }}
            >
              <InfoIcon class="size-role-help-icon" aria-hidden="true" />
            </Button>
          {/snippet}
        </TooltipTrigger>
        <TooltipContent class="block max-w-xs">
          {t('settingsAccount.identity.roleNote')}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  </span>
{/snippet}

{#snippet identitySection()}
  <form
    onsubmit={saveIdentity}
    class="flex flex-col gap-form-gap p-panel-padding-mobile @lg/pane:p-panel-padding"
  >
    <div
      data-slot="account-identity-layout"
      class="grid min-w-0 gap-section-gap @lg/pane:grid-cols-account-identity @lg/pane:gap-x-section-block"
    >
      <div class="flex justify-center @lg/pane:justify-start">
        {#if waiting}
          <Skeleton
            class="size-profile-avatar rounded-full"
            aria-hidden="true"
          />
        {:else}
          <!-- eslint-disable-next-line svelte/no-restricted-html-elements -- FileDropzone draws a drop zone; this picker opens from the avatar button -->
          <input
            bind:this={photoInput}
            id={photoInputId}
            data-slot="avatar-file-input"
            type="file"
            accept="image/jpeg,image/png,image/webp,image/gif"
            aria-label={t('settingsAccount.identity.editPhoto')}
            tabindex="-1"
            class="sr-only"
            onchange={pickPhoto}
          />
          <Button
            variant="ghost"
            size="avatar"
            data-slot="avatar-edit"
            label={`${initials === '' ? '?' : initials}: ${t('settingsAccount.identity.editPhoto')}`}
            aria-controls={photoInputId}
            aria-describedby={photoNoticeKey !== undefined
              ? photoNoticeId
              : undefined}
            radius="full"
            class="group/avatar relative overflow-hidden"
            onclick={() => photoInput?.click()}
          >
            {#key avatarSource}
              <Avatar.Root
                onLoadingStatusChange={(status) => {
                  if (localPreview && status === 'error') photoError = 'read';
                  if (status === 'loaded' && photoError === 'read')
                    photoError = undefined;
                }}
              >
                {#snippet child({ props })}
                  <span
                    {...props}
                    data-slot="account-avatar"
                    aria-hidden="true"
                    class="grid size-profile-avatar shrink-0 place-items-center overflow-hidden rounded-full border border-border bg-secondary text-secondary-foreground"
                  >
                    <Avatar.Image src={avatarSource} alt="">
                      {#snippet child({ props })}
                        <img
                          {...props}
                          data-slot="account-avatar-image"
                          src={avatarSource}
                          alt=""
                          class="size-full object-cover"
                        />
                      {/snippet}
                    </Avatar.Image>
                    <Avatar.Fallback>
                      {#snippet child({ props })}
                        <span
                          {...props}
                          data-slot="account-avatar-fallback"
                          class="text-3xl font-medium"
                          >{initials === '' ? '?' : initials}</span
                        >
                      {/snippet}
                    </Avatar.Fallback>
                  </span>
                {/snippet}
              </Avatar.Root>
            {/key}
            <span
              aria-hidden="true"
              data-slot="avatar-edit-overlay"
              class="pointer-events-none absolute inset-0 grid place-items-center bg-background/80 opacity-0 transition-opacity group-hover/avatar:opacity-100 group-focus-visible/avatar:opacity-100 pointer-coarse:opacity-100"
            >
              <img
                src={editIcon}
                alt=""
                class="size-profile-edit-icon invert dark:invert-0"
              />
            </span>
          </Button>
        {/if}
      </div>
      <div
        data-slot="account-identity-details"
        class="flex min-w-0 flex-col gap-form-gap"
      >
        <!-- While the record loads, each of the four rows holds a skeleton in its place. -->
        {#each ROWS as row (row.id)}
          {#if identityLoading || row.value(identity) !== undefined || (editingIdentity && row.id === 'name')}
            <SettingsRow
              class={[
                row.id === 'role' &&
                  '[&_[data-slot=settings-row-description]]:hidden',
                row.id === 'added' &&
                  'mt-form-gap border-t border-border pt-section-gap',
              ]}
              label={row.label()}
              labelHelp={row.id === 'role' ? roleHelp : undefined}
              description={row.id === 'role'
                ? t('settingsAccount.identity.roleNote')
                : undefined}
            >
              {#snippet control()}
                {#if identityLoading}
                  <Skeleton class="h-5 w-32" />
                {:else if editingIdentity && (row.id === 'name' || row.id === 'username')}
                  {#if row.id === 'name'}
                    <Input
                      aria-label={row.label()}
                      value={draftName}
                      oninput={(
                        event: Event & { currentTarget: HTMLInputElement },
                      ) => {
                        draftName = event.currentTarget.value;
                      }}
                      maxlength={128}
                      disabled={savingIdentity}
                      autocomplete="name"
                    />
                  {:else}
                    <Input
                      aria-label={row.label()}
                      value={draftUsername}
                      oninput={(
                        event: Event & { currentTarget: HTMLInputElement },
                      ) => {
                        draftUsername = event.currentTarget.value;
                      }}
                      maxlength={254}
                      required
                      disabled={savingIdentity}
                      autocomplete="username"
                    />
                  {/if}
                {:else}
                  <span
                    class={[
                      'min-w-0 break-words',
                      row.id === 'name' && 'text-base font-semibold',
                      row.id === 'added' && 'text-xs text-muted-foreground',
                      (row.id === 'username' || row.id === 'role') &&
                        'text-identity-value font-semibold',
                    ]}
                  >
                    {row.value(identity)}
                  </span>
                {/if}
              {/snippet}
            </SettingsRow>
          {/if}
        {/each}
        {@render subjectRow()}
        {#if onSaveIdentity && !waiting && identity}
          <div class="flex flex-wrap justify-end gap-form-gap pt-form-gap">
            {#if editingIdentity}
              <Button
                type="button"
                variant="secondary"
                disabled={savingIdentity}
                onclick={() => {
                  identityEditor.open = false;
                  identityFault = undefined;
                }}>{t('primitives.cancel')}</Button
              >
              <Button
                type="submit"
                disabled={savingIdentity || draftUsername.trim() === ''}
                >{t('settingsAccount.identity.save')}</Button
              >
            {:else}
              <Button type="button" variant="secondary" onclick={editIdentity}
                >{t('settingsAccount.identity.edit')}</Button
              >
            {/if}
          </div>
          {#if editingIdentity && identityPreview}<p
              class="text-sm text-muted-foreground"
            >
              {t('settingsAccount.identity.preview')}
            </p>{/if}
          {#if identityFault}<FaultAlert
              fault={describeFault(identityFault)}
            />{/if}
        {/if}
      </div>
      {#if photoNoticeKey !== undefined}
        <p
          id={photoNoticeId}
          role={photoNoticeRole}
          class="col-span-full block text-sm text-muted-foreground"
        >
          {t(photoNoticeKey)}
        </p>
      {/if}
    </div>
  </form>
{/snippet}

{#snippet passwordSection()}
  <!-- The submit sits in the flow under the fields. -->
  <form
    onsubmit={submit}
    class="flex flex-col gap-section-gap p-panel-padding-mobile @lg/pane:p-panel-padding"
  >
    <div class="flex flex-col gap-form-note">
      <p class="text-sm text-muted-foreground">
        {t('settingsAccount.password.note')}
      </p>
    </div>

    <FaultAlert fault={refusal} />

    <div class="flex flex-col gap-form-gap @lg/pane:ms-form-indent">
      <Field
        layout="row"
        label={t('settingsAccount.password.current')}
        control={PasswordInput}
        controlProps={{
          class: 'max-w-form-control',
          label: t('settingsAccount.password.current'),
          value: current,
          autocomplete: 'current-password',
          oninput: (event: Event & { currentTarget: HTMLInputElement }) => {
            current = event.currentTarget.value;
          },
        }}
      />
      <Field
        layout="row"
        label={t('settingsAccount.password.next')}
        error={lengthError}
        control={PasswordInput}
        controlProps={{
          class: 'max-w-form-control',
          label: t('settingsAccount.password.next'),
          value: next,
          autocomplete: 'new-password',
          oninput: (event: Event & { currentTarget: HTMLInputElement }) => {
            next = event.currentTarget.value;
          },
        }}
      />
      <Field
        layout="row"
        label={t('settingsAccount.password.confirm')}
        error={mismatchError}
        control={PasswordInput}
        controlProps={{
          class: 'max-w-form-control',
          label: t('settingsAccount.password.confirm'),
          value: confirm,
          autocomplete: 'new-password',
          oninput: (event: Event & { currentTarget: HTMLInputElement }) => {
            confirm = event.currentTarget.value;
          },
        }}
      />
    </div>

    <div class="grid @lg/pane:ms-form-indent @lg/pane:grid-cols-settings-form">
      <div
        class="flex w-full max-w-form-control justify-end @lg/pane:col-start-2"
      >
        <Button type="submit" class="self-start" disabled={blocked}>
          {t('settingsAccount.password.submit')}
        </Button>
      </div>
    </div>
  </form>
{/snippet}

<div class="@container/account flex min-w-0 flex-col gap-section-gap">
  <h2 data-slot="settings-section-title" class="text-lead font-semibold">
    {t('settingsAccount.title')}
  </h2>
  <div
    data-slot="account-panels"
    class="grid min-w-0 grid-cols-settings-panels gap-x-section-gap gap-y-form-gap @appearance-columns/account:gap-y-section-gap"
  >
    <SettingsPane title={t('settingsAccount.identity.title')} class="min-w-0">
      {@render identitySection()}
    </SettingsPane>
    {#if !passwordless}
      <SettingsPane
        title={t('settingsAccount.password.title')}
        class="min-w-0"
        message={changed ? t('settingsAccount.password.changed') : undefined}
      >
        {@render passwordSection()}
      </SettingsPane>
    {/if}
  </div>
</div>

{#if draftPhoto !== undefined && draftPhoto.owner === subject}
  {#key draftPhoto.file}
    <PhotoCropDialog
      file={draftPhoto.file}
      onCancel={() => {
        draftPhoto = undefined;
      }}
      onConfirm={(file: File) => {
        selectedPhoto = { owner: subject, file };
        draftPhoto = undefined;
        photoError = undefined;
      }}
    />
  {/key}
{/if}

<style lang="postcss">
  @reference '../../styles/index.css';

  [data-slot='account-identity-details'] {
    :global([data-slot='settings-row']) {
      @apply flex-col items-stretch @lg/pane:grid-cols-identity-fields @lg/pane:items-center @lg/pane:gap-x-identity-column;
    }

    :global([data-slot='settings-row'] > div:first-child > span) {
      @apply text-xs font-normal text-muted-foreground;
    }
  }
</style>
