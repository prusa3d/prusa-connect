<!--
  The panel that adds one person.

  It asks for a password, which the React specification does not: this server
  creates the account itself and sends no mail, so the first password has to be
  set here or the account cannot be signed in to at all.

  It holds no draft of its own beyond the open panel. The screen resets the
  fields through `onOpenChange`, so a username typed and abandoned never comes
  back filled in.
-->
<script lang="ts">
  import type { AccountRoleName, NewAccount } from '../../data';
  import { t } from '../../i18n';
  import {
    Button,
    Dialog,
    DialogClose,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
    Field,
    Input,
    PasswordInput,
    Label,
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
  } from '../../primitives';
  import { roleLabelKey } from './presentation';

  /** The core refuses anything shorter, so the field says so before the send. */
  const MINIMUM_PASSWORD = 8;

  /** The role a new person starts with. */
  const DEFAULT_ROLE: AccountRoleName = 'operator';

  let {
    open,
    onOpenChange,
    roles,
    taken,
    busy,
    onAdd,
  }: {
    open: boolean;
    onOpenChange: (open: boolean) => void;
    roles: readonly AccountRoleName[];
    /** The usernames already on this server, compared case-folded as the core does. */
    taken: readonly string[];
    busy: boolean;
    onAdd: (account: NewAccount) => void;
  } = $props();

  const uid = $props.id();
  const roleId = `${uid}-role`;
  const roleNoteId = `${uid}-role-note`;

  let username = $state('');
  let displayName = $state('');
  let password = $state('');
  // Operator, because the person being added is almost always somebody who
  // will run prints. A reader who wants a weaker role changes it here.
  let role = $state<AccountRoleName>(DEFAULT_ROLE);
  // An empty field is only wrong once someone has tried to send it.
  let attempted = $state(false);

  const options = $derived(
    roles.map((name) => ({ value: name, label: t(roleLabelKey(name)) })),
  );

  const isTaken = $derived(
    taken.some(
      (held) => held.toLowerCase() === username.trim().toLowerCase(),
    ) && username.trim() !== '',
  );

  // `Field` takes a string, and an empty one is no error at all.
  const usernameError = $derived.by(() => {
    if (isTaken) return t('settingsPeople.add.taken');
    if (attempted && username.trim() === '') {
      return t('settingsPeople.add.missingUsername');
    }

    return '';
  });

  const passwordError = $derived(
    password !== '' && password.length < MINIMUM_PASSWORD
      ? t('settingsPeople.add.shortPassword')
      : '',
  );

  const blocked = $derived(
    busy ||
      isTaken ||
      username.trim() === '' ||
      password.length < MINIMUM_PASSWORD,
  );

  const shut = (next: boolean): void => {
    if (!next) {
      username = '';
      displayName = '';
      password = '';
      role = DEFAULT_ROLE;
      attempted = false;
    }
    onOpenChange(next);
  };

  const submit = (event: SubmitEvent): void => {
    event.preventDefault();
    attempted = true;
    if (blocked) return;

    onAdd({
      username: username.trim(),
      password,
      display_name: displayName.trim(),
      role,
    });
  };
</script>

<Dialog {open} onOpenChange={shut}>
  <DialogContent>
    <form class="flex flex-col gap-4" onsubmit={submit}>
      <DialogHeader>
        <DialogTitle>{t('settingsPeople.add.title')}</DialogTitle>
        <DialogDescription>{t('settingsPeople.add.note')}</DialogDescription>
      </DialogHeader>
      <Field
        label={t('settingsPeople.username')}
        error={usernameError}
        control={Input}
        controlProps={{
          value: username,
          type: 'text',
          autocomplete: 'off',
          oninput: (event: Event & { currentTarget: HTMLInputElement }) => {
            username = event.currentTarget.value;
          },
        }}
      />
      <Field
        label={t('settingsPeople.add.displayName')}
        control={Input}
        controlProps={{
          value: displayName,
          type: 'text',
          autocomplete: 'off',
          oninput: (event: Event & { currentTarget: HTMLInputElement }) => {
            displayName = event.currentTarget.value;
          },
        }}
      />
      <Field
        label={t('settingsPeople.add.password')}
        error={passwordError}
        control={PasswordInput}
        controlProps={{
          value: password,
          label: t('settingsPeople.add.password'),
          fill: true,
          type: 'password',
          autocomplete: 'new-password',
          oninput: (event: Event & { currentTarget: HTMLInputElement }) => {
            password = event.currentTarget.value;
          },
        }}
      />
      <!-- Not a `Field`: it wraps one control and writes its description onto
           the element it names, and a select's name belongs on the trigger. -->
      <div class="flex flex-col gap-2">
        <Label for={roleId}>{t('settingsPeople.role')}</Label>
        <Select
          value={role}
          items={options}
          onValueChange={(next: string) => {
            // The options are built from `roles`, so the value is one of them.
            role = next as AccountRoleName;
          }}
        >
          <SelectTrigger id={roleId} aria-describedby={roleNoteId}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {#each options as option (option.value)}
              <SelectItem value={option.value} label={option.label}>
                {option.label}
              </SelectItem>
            {/each}
          </SelectContent>
        </Select>
        <!-- What the role reaches. Every role on this server is granted over
             the whole organisation, so a person added with one reaches every
             printer, and the choice must not read as a choice about one. -->
        <p id={roleNoteId} class="text-xs text-muted-foreground">
          {t('settingsPeople.add.roleScope')}
        </p>
      </div>
      <DialogFooter>
        <DialogClose>
          {#snippet child({ props })}
            <Button {...props} variant="secondary">
              {t('primitives.cancel')}
            </Button>
          {/snippet}
        </DialogClose>
        <Button type="submit" disabled={blocked}>
          {t('settingsPeople.add')}
        </Button>
      </DialogFooter>
    </form>
  </DialogContent>
</Dialog>
