<!--
  A zone that takes files, by drop or by pick.

  The file input is hidden with `sr-only` and not with `display:none`. Some
  browsers refuse a scripted `click()` on an input they do not render, and
  `display:none` also takes the input out of the accessibility tree.

  `accept` is a hint. The browser does not enforce it, and a drop skips it
  altogether, so every file is checked again here.
-->
<script lang="ts">
  import UploadIcon from '@lucide/svelte/icons/upload';

  import { t } from '../../i18n';
  import Button from '../button/button.svelte';
  import type { FileDropzoneProps } from './types';

  let {
    onFiles,
    accept,
    disabled = false,
    compact = false,
    label,
    class: className,
    ...rest
  }: FileDropzoneProps = $props();

  const inputId = $props.id();

  let input: HTMLInputElement | undefined = $state();
  let draggingOver = $state(false);
  let refused = $state<string[]>([]);

  const zoneState = $derived.by(() => {
    if (disabled) return 'disabled';
    if (draggingOver) return 'dragging-over';
    if (refused.length > 0) return 'rejected';
    return 'idle';
  });

  const patterns = $derived(
    (accept ?? '')
      .split(',')
      .map((pattern) => pattern.trim())
      .filter(Boolean),
  );
  // `accept` comes without spaces, and a run with no spaces cannot wrap, so a
  // long list shown as given would run out of the zone.
  const formats = $derived(patterns.join(', '));

  // Every word is read from the catalogue while the component draws, so a
  // change of language rewrites it (frontend-i18n.md rule 1).
  const invitation = $derived(t('primitives.dropzone.invitation'));
  const chooseWords = $derived(t('primitives.dropzone.choose'));
  const inputLabel = $derived.by(() => {
    if (label !== undefined) return label;
    return accept === undefined
      ? t('primitives.dropzone.label')
      : t('primitives.dropzone.labelAccepting', { accept: formats });
  });
  const acceptedLine = $derived(
    accept === undefined
      ? ''
      : t('primitives.dropzone.accepted', { accept: formats }),
  );
  const refusedLine = $derived.by(() => {
    if (refused.length === 0) return '';
    const said = t('primitives.dropzone.rejected', {
      count: refused.length,
      name: refused[0] ?? '',
    });
    if (accept === undefined) return said;
    return `${said} ${t('primitives.dropzone.expected', { accept: formats })}`;
  });

  /** Whether one file answers one `accept` entry: `.gcode`, `image/*` or `text/plain`. */
  const answers = (file: File, pattern: string): boolean => {
    if (pattern.startsWith('.')) {
      return file.name.toLowerCase().endsWith(pattern.toLowerCase());
    }
    if (pattern.endsWith('/*'))
      return file.type.startsWith(pattern.slice(0, -1));
    return file.type === pattern;
  };

  const take = (files: FileList | null | undefined): void => {
    if (!files || files.length === 0) return;
    const taken: File[] = [];
    const turnedDown: string[] = [];
    for (const file of Array.from(files)) {
      const fits =
        patterns.length === 0 ||
        patterns.some((pattern) => answers(file, pattern));
      if (fits) taken.push(file);
      else turnedDown.push(file.name);
    }
    refused = turnedDown;
    if (taken.length > 0) onFiles(taken);
  };

  const dragOver = (event: DragEvent): void => {
    // A browser allows a drop here only when the dragover default is stopped.
    event.preventDefault();
    if (disabled) return;
    draggingOver = true;
  };

  const dragLeave = (event: DragEvent): void => {
    // Moving the pointer onto a child fires dragleave on the zone as well, so
    // the highlight goes out only once the pointer has left the zone's bounds.
    const zone = event.currentTarget;
    const to = event.relatedTarget;
    if (zone instanceof Node && to instanceof Node && zone.contains(to)) return;
    draggingOver = false;
  };

  const drop = (event: DragEvent): void => {
    event.preventDefault();
    draggingOver = false;
    if (disabled) return;
    take(event.dataTransfer?.files);
  };

  const pick = (): void => {
    if (!input) return;
    take(input.files);
    // So picking the same file again still fires a change event.
    input.value = '';
  };
</script>

<div
  {...rest}
  data-slot="file-dropzone"
  data-state={zoneState}
  ondragover={dragOver}
  ondragleave={dragLeave}
  ondrop={drop}
  class={[
    'rounded-md border border-dashed border-border transition-colors',
    compact
      ? 'flex flex-wrap items-center gap-x-3 gap-y-2 px-3 py-2'
      : 'flex flex-col items-center gap-4 p-6 text-center',
    'state-dragging-over:border-primary state-dragging-over:bg-accent',
    'state-rejected:border-destructive',
    'state-disabled:border-disabled-foreground',
    className,
  ]}
>
  <UploadIcon
    aria-hidden="true"
    class={['text-muted-foreground', compact ? 'size-4 shrink-0' : 'size-8']}
  />
  <p class="text-sm text-muted-foreground">{invitation}</p>
  <label for={inputId} class="sr-only">{inputLabel}</label>
  <input
    bind:this={input}
    id={inputId}
    type="file"
    multiple
    {accept}
    {disabled}
    tabindex={-1}
    onchange={pick}
    class="sr-only"
  />
  <Button
    type="button"
    variant="secondary"
    size={compact ? 'sm' : 'md'}
    {disabled}
    class={compact ? 'ml-auto shrink-0' : undefined}
    onclick={() => input?.click()}
  >
    {chooseWords}
  </Button>
  {#if accept !== undefined && !compact}
    <p class="text-xs text-muted-foreground">{acceptedLine}</p>
  {/if}
  {#if refused.length > 0}
    <!-- eslint-disable-next-line no-restricted-syntax -- the dropzone announces the files it refused in place; its test queries them by role -->
    <p role="alert" class="text-sm text-destructive-text">{refusedLine}</p>
  {/if}
</div>
