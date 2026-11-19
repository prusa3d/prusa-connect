<!--
  One configuration key, drawn by its kind: a switch, a list of values, a
  closed set, or text. A key the operator's file sets is shown and not
  editable, because their value wins (ADR-20261015-ejz rule 12).
-->
<script lang="ts">
  import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
  import InfoIcon from '@lucide/svelte/icons/info';
  import RotateCcwIcon from '@lucide/svelte/icons/rotate-ccw';

  import type { SetupKey } from '../../data';
  import { t } from '../../i18n';
  import {
    Button,
    Field,
    Input,
    Popover,
    PopoverContent,
    PopoverDescription,
    PopoverHeader,
    PopoverTitle,
    PopoverTrigger,
    Label,
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
    SettingsRow,
    Switch,
    Textarea,
  } from '../../primitives';
  import { hintOf, labelOf } from './labels';
  import { type Answer, errorOf, startOf } from './model';

  interface ConfigFieldProps {
    key: SetupKey;
    /** What the field shows now. */
    shown: Answer;
    /** Whether the person changed it. */
    answered: boolean;
    /** The operator's file, named when the key is locked. */
    file: string;
    /** The configuration reference, opened from the field's info. */
    /** The documentation's address, which the key's pages are relative to. */
    docsHref: string | undefined;
    showError: boolean;
    onanswer: (value: Answer) => void;
    onforget: () => void;
  }

  let {
    key,
    shown,
    answered,
    file,
    docsHref,
    showError,
    onanswer,
    onforget,
  }: ConfigFieldProps = $props();

  const id = $props.id();
  const label = $derived(labelOf(key));
  const start = $derived(startOf(key));

  const ERROR = $derived({
    integer: t('setupWizard.field.error.integer'),
    duration: t('setupWizard.field.error.duration'),
    choice: t('setupWizard.field.error.choice'),
  });

  const error = $derived.by(() => {
    const found = showError ? errorOf(key, shown) : undefined;

    return found === undefined ? '' : ERROR[found];
  });

  const defaultNote = $derived.by(() => {
    if (key.locked) {
      return key.secret
        ? t('setupWizard.field.lockedSecret', { file })
        : t('setupWizard.field.locked', { file });
    }
    if (start === '') return t('setupWizard.field.noDefault');
    if (typeof start === 'boolean') {
      return t('setupWizard.field.default', {
        value: start ? t('setupWizard.field.on') : t('setupWizard.field.off'),
      });
    }

    return t('setupWizard.field.default', {
      value: start.replaceAll('\n', ', '),
    });
  });

  const description = $derived(
    key.kind === 'text_list'
      ? `${hintOf(key)} ${t('setupWizard.field.list')}`
      : hintOf(key),
  );

  const choices = $derived(key.choices ?? []);
  // A language is named in itself and a theme by the words the toolbar uses;
  // every other closed set is shown as the value the file holds.
  const choiceLabel = (choice: string): string => {
    if (key.path === 'core.frontend.default_language') {
      try {
        const name = new Intl.DisplayNames([choice], { type: 'language' }).of(
          choice,
        );
        if (name !== undefined) {
          return name.charAt(0).toLocaleUpperCase(choice) + name.slice(1);
        }
      } catch {
        return choice;
      }
    }
    if (key.path === 'core.frontend.theme') {
      if (choice === 'light') return t('setupWizard.theme.light');
      if (choice === 'dark') return t('setupWizard.theme.dark');
      if (choice === 'system') return t('setupWizard.theme.system');
    }

    return choice;
  };

  const items = $derived(
    choices.map((choice: string) => ({
      value: choice,
      label: choiceLabel(choice),
    })),
  );
</script>

<div
  class="flex flex-col gap-1.5"
  data-part="config-field"
  data-path={key.path}
  data-locked={key.locked}
>
  {#if key.kind === 'switch'}
    <SettingsRow layout="switch" {label} {description}>
      {#snippet control()}
        <Switch
          aria-label={label}
          checked={shown === true}
          disabled={key.locked}
          onCheckedChange={(next: boolean) => {
            onanswer(next);
          }}
        />
      {/snippet}
    </SettingsRow>
  {:else if choices.length > 0}
    <div class="flex flex-col gap-2">
      <Label for={`${id}-select`}>{label}</Label>
      <Select
        value={String(shown)}
        {items}
        disabled={key.locked}
        onValueChange={(next: string) => {
          onanswer(next);
        }}
      >
        <SelectTrigger id={`${id}-select`} class="w-full">
          <SelectValue placeholder={label} />
        </SelectTrigger>
        <SelectContent>
          {#each items as item (item.value)}
            <SelectItem value={item.value}>{item.label}</SelectItem>
          {/each}
        </SelectContent>
      </Select>
      <p class="text-sm text-muted-foreground">{description}</p>
    </div>
  {:else}
    <Field
      {label}
      {description}
      {error}
      control={key.kind === 'text_list' ? Textarea : Input}
      controlProps={{
        value: key.secret && key.locked ? '' : String(shown),
        type: key.secret ? 'password' : 'text',
        rows: key.kind === 'text_list' ? 3 : undefined,
        inputmode: key.kind === 'number' ? 'numeric' : undefined,
        class: key.kind === 'text' ? undefined : 'font-mono',
        readonly: key.locked,
        spellcheck: false,
        oninput: (event: Event & { currentTarget: HTMLInputElement }) => {
          onanswer(event.currentTarget.value);
        },
      }}
    />
  {/if}
  <div class="flex min-h-8 flex-wrap items-center gap-2">
    <span class="me-auto font-mono text-xs text-muted-foreground"
      >{defaultNote}</span
    >
    <Popover>
      <PopoverTrigger>
        {#snippet child({ props })}
          <Button {...props} variant="ghost" size="sm" data-part="field-info">
            <InfoIcon aria-hidden="true" />
            {t('setupWizard.field.more')}
          </Button>
        {/snippet}
      </PopoverTrigger>
      <PopoverContent class="w-80">
        <PopoverHeader>
          <PopoverTitle>{label}</PopoverTitle>
          <PopoverDescription>{key.doc}</PopoverDescription>
        </PopoverHeader>
        <p class="mt-3 font-mono text-xs text-muted-foreground">
          {t('setupWizard.field.inFile', { key: key.path })}
        </p>
        {#if docsHref !== undefined}
          {@const docs = docsHref}
          <div class="mt-3 flex flex-col items-start gap-2">
            {#if key.guide !== undefined && key.guide !== ''}
              {@const guide = key.guide}
              <Button variant="link" size="inline-icon">
                {#snippet child({ props }: { props: Record<string, unknown> })}
                  <a
                    {...props}
                    href={`${docs}${guide}`}
                    target="_blank"
                    rel="noopener noreferrer"
                    data-part="field-guide"
                  >
                    {t('setupWizard.field.guide')}
                    <ExternalLinkIcon class="size-3.5" aria-hidden="true" />
                  </a>
                {/snippet}
              </Button>
            {/if}
            <Button variant="link" size="inline-icon">
              {#snippet child({ props }: { props: Record<string, unknown> })}
                <a
                  {...props}
                  href={`${docs}${key.reference}`}
                  target="_blank"
                  rel="noopener noreferrer"
                  data-part="field-reference"
                >
                  {t('setupWizard.field.reference')}
                  <ExternalLinkIcon class="size-3.5" aria-hidden="true" />
                </a>
              {/snippet}
            </Button>
          </div>
        {/if}
      </PopoverContent>
    </Popover>
    {#if answered && !key.locked && shown !== start}
      <Button variant="ghost" size="sm" onclick={onforget}>
        <RotateCcwIcon aria-hidden="true" />
        {t('setupWizard.field.reset')}
      </Button>
    {/if}
  </div>
</div>
