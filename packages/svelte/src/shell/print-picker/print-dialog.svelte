<!-- Send one file to the printers a person ticks. -->
<script lang="ts">
  import FileCode2Icon from '@lucide/svelte/icons/file-code-2';
  import { onDestroy, untrack } from 'svelte';

  import { runningTransferOf } from '../../data/running-transfers.svelte';
  import { FILE_NAME_TAKEN, printOnEach } from '../../data/services/print';
  import { PRINTER_STATE_LABEL_KEY } from '../../domain';
  import {
    describeFault,
    formatDuration,
    formatList,
    formatUnit,
    t,
    type FaultText,
  } from '../../i18n';
  import {
    Badge,
    Button,
    Checkbox,
    ConfirmDialog,
    Dialog,
    DIALOG_BODY,
    DIALOG_SCROLLS,
    DialogClose,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
    FaultAlert,
    FileThumbnail,
    KeyValueList,
    Label,
    ListingSearch,
    Selection,
    toast,
    type KeyValueRow,
    PrinterGlyphIcon,
    Progress,
  } from '../../primitives';
  import { fileRefOf, setPrintDialogContext, SlotOutlet } from '../../slots';
  import { stateBadgeVariant } from '../printers/presentation';
  import {
    type PrintPhase,
    printPhaseOf,
    printTargetsOf,
    slicedForOther,
  } from './presentation';
  import type { PrintDialogProps, PrintTarget } from './types';

  let {
    open,
    onOpenChange,
    file,
    devices,
    transitions,
    locked,
    noStorage,
    transfers,
    onPrint,
    onPrinted,
  }: PrintDialogProps = $props();

  const PERCENT = 100;
  const fileKey = $derived(
    file === undefined
      ? undefined
      : JSON.stringify([file.storage, file.source, file.name]),
  );

  const uid = $props.id();

  let busy = $state(false);
  let query = $state('');
  let selectedOnly = $state(false);

  /** The printers the last send did not start, each with why. */
  let failures = $state.raw<{ id: string; name: string; fault: FaultText }[]>(
    [],
  );

  /**
   * The printers that hold a different file under this name. The person is
   * asked once for all of them whether to replace it.
   */
  let taken = $state.raw<{ fileKey: string; ids: string[] }>();
  const conflict = $derived(
    open && taken?.fileKey === fileKey ? taken : undefined,
  );
  const replaceable = $derived(
    conflict?.ids.filter((id) => readyIds.has(id)) ?? [],
  );

  const unavailable = (id: string) => {
    const row = targets.find((target) => target.id === id);
    return {
      id,
      name: row?.name ?? id,
      fault: {
        code: undefined,
        title:
          row?.reasonKey === undefined
            ? t('printPicker.reason.unavailable')
            : reasonOf(row),
        detail: undefined,
      },
    };
  };
  const blockedReplacements = $derived(
    conflict?.ids.filter((id) => !readyIds.has(id)).map(unavailable) ?? [],
  );

  // The number of the send whose answer the panel still waits for.
  let listening = 0;
  onDestroy(() => {
    listening += 1;
  });

  // The panel's own memory of whether it was open.
  let opened = false;
  let openedFile: string | undefined;

  const targets = $derived(
    printTargetsOf(devices, { locked, noStorage, transitions }),
  );

  const free = $derived(targets.filter((row) => row.ready));
  const readyIds = $derived(new Set(free.map((row) => row.id)));
  const chosen = new Selection<string>({
    rows: () => targets.map((row) => row.id),
  });

  const searchable = $derived(targets.length > 8);
  const needle = $derived(query.trim().toLocaleLowerCase());
  const visible = $derived(
    targets.filter(
      (row) =>
        !searchable ||
        ((!selectedOnly || (row.ready && chosen.has(row.id))) &&
          (needle === '' ||
            `${row.name} ${row.model}`.toLocaleLowerCase().includes(needle))),
    ),
  );

  const none = $derived(free.length === 0);

  const details = $derived(file?.details);

  /** A fact the file did not carry is dropped rather than drawn with a dash. */
  const known = (rows: readonly KeyValueRow[]): KeyValueRow[] =>
    rows.filter((row) => row.value !== undefined && row.value !== '');

  const facts = $derived(
    details === undefined
      ? []
      : known([
          {
            id: 'time',
            label: t('printPicker.fact.time'),
            value:
              details.estimated_seconds === undefined
                ? undefined
                : formatDuration(details.estimated_seconds),
          },
          {
            id: 'filament',
            label: t('printPicker.fact.filament'),
            value:
              details.filament_g === undefined
                ? undefined
                : formatUnit(details.filament_g, 'g', {
                    maximumFractionDigits: 1,
                  }),
          },
          {
            id: 'material',
            label: t('printPicker.fact.material'),
            value: details.material,
          },
          {
            id: 'model',
            label: t('printPicker.fact.model'),
            value: details.printer_model,
          },
        ]),
  );

  const clear = (): void => {
    listening += 1;
    busy = false;
    failures = [];
    taken = undefined;
  };

  /** The pick the panel opens on, decided once. */
  $effect(() => {
    if (open === opened && fileKey === openedFile) return;
    opened = open;
    openedFile = fileKey;
    query = '';
    selectedOnly = false;

    clear();
    if (!open) return;

    const ready = untrack(() => free);
    chosen.clear();
    if (ready.length === 1 && ready[0] !== undefined)
      chosen.set([ready[0].id], true);
  });

  let panel = $state<HTMLElement | null>(null);

  // bits-ui focuses the first tabbable element, which is the file's thumbnail,
  // a control nobody opened this dialog for. Focus goes to the first printer
  // that can take the print instead. With none free, it goes to the panel
  // itself, which a screen reader announces by its title and note, as the
  // WAI-ARIA dialog pattern advises when no control is the obvious first one.
  const focusFirstFree = (event: Event): void => {
    event.preventDefault();
    const first = free[0];
    const target =
      first === undefined
        ? null
        : document.getElementById(`${uid}-${first.id}`);
    (target ?? panel)?.focus();
  };

  const shut = (next: boolean): void => {
    if (!next) clear();
    onOpenChange(next);
  };

  // A module's print-section reads the ticked printers and may close the
  // dialog, for instance after it has taken the print over. Getters, so the
  // section reads the pick as it is now and not as it was at mount.
  setPrintDialogContext({
    get selected() {
      return [...chosen.keys];
    },
    close: () => {
      shut(false);
    },
  });

  // A module's section is about a file of this server's storage, which a
  // file already on the printer is not.
  const fileRef = $derived(
    file?.storage === undefined
      ? undefined
      : fileRefOf(file.storage, file.source),
  );

  /** How far this file has got on its way to one printer, while it is on its way. */
  const phaseOf = (deviceId: string): PrintPhase | undefined =>
    file?.storage === undefined
      ? undefined
      : printPhaseOf(
          runningTransferOf(transfers ?? [], {
            deviceId,
            storage: file.storage,
            source: file.source,
          }),
        );

  /** The line a blocked printer shows, with the states its key names. */
  const reasonOf = (row: PrintTarget): string =>
    row.reasonKey === undefined
      ? ''
      : t(row.reasonKey, {
          states: formatList(row.reasonStates ?? []),
        });

  /**
   * The printers a press sends to: ticked, and still free now. A printer that
   * stops being free stays ticked, because unticking is the reader's own act,
   * so the button reads this list and not the size of `chosen`. Reading the
   * size alone leaves it enabled beside the line saying no printer can start
   * a print.
   */
  const sendable = $derived(
    targets
      .filter((row) => row.ready && chosen.has(row.id))
      .map((row) => row.id),
  );

  const send = (): Promise<void> => sendTo(sendable, false);

  // `open` is a live prop: it can turn false while a send awaits.
  const superseded = (token: number, key: string): boolean =>
    token !== listening || !open || fileKey !== key;

  const sendTo = async (ids: string[], replace: boolean): Promise<void> => {
    const sending = file;
    const key = fileKey;
    if (!open || sending === undefined || key === undefined || busy) return;

    const ready = ids.filter((id) => readyIds.has(id));
    const blocked = ids.filter((id) => !readyIds.has(id)).map(unavailable);
    failures = blocked;
    if (ready.length === 0) return;

    listening += 1;
    const token = listening;
    busy = true;
    const attempts = await printOnEach(
      (request) => {
        if (superseded(token, key)) throw new DOMException('', 'AbortError');
        if (!readyIds.has(request.deviceId))
          throw new Error(unavailable(request.deviceId).fault.title);
        return onPrint(request);
      },
      ready.map((deviceId) =>
        sending.storage === undefined
          ? { deviceId, path: sending.source }
          : {
              deviceId,
              // The file lands on the machine under its own name.
              path: sending.name,
              source: sending.source,
              storage: sending.storage,
              ...(replace ? { replace } : {}),
            },
      ),
    );
    if (superseded(token, key)) return;

    busy = false;
    const started = attempts.filter((one) => one.failure === undefined);
    chosen.set(
      started.map((one) => one.deviceId),
      false,
    );
    if (started.length > 0) {
      toast.success(t('printPicker.started', { count: started.length }));
    }
    if (started.length === attempts.length && blocked.length === 0) {
      listening += 1;
      onOpenChange(false);
      onPrinted?.(ready);

      return;
    }
    const refused = attempts.flatMap((one) =>
      one.failure === undefined
        ? []
        : [{ id: one.deviceId, fault: describeFault(one.failure) }],
    );
    // A name the printer holds is a question for the person, not a failure.
    const conflicts = refused
      .filter(
        (one) =>
          one.fault.code === FILE_NAME_TAKEN &&
          !replace &&
          readyIds.has(one.id),
      )
      .map((one) => one.id);
    taken =
      conflicts.length === 0 ? undefined : { fileKey: key, ids: conflicts };
    failures = [
      ...blocked,
      ...refused
        .filter((one) => !conflicts.includes(one.id))
        .map((one) =>
          one.fault.code === FILE_NAME_TAKEN &&
          !replace &&
          !readyIds.has(one.id)
            ? unavailable(one.id)
            : {
                ...one,
                name: targets.find((row) => row.id === one.id)?.name ?? one.id,
              },
        ),
    ];
  };
</script>

<Dialog {open} onOpenChange={shut}>
  <!-- Capped at the height of the page. -->
  <DialogContent
    bind:ref={panel}
    class={DIALOG_SCROLLS}
    onOpenAutoFocus={focusFirstFree}
  >
    <DialogHeader>
      <DialogTitle>{t('printPicker.title')}</DialogTitle>
      <DialogDescription>
        {t('printPicker.note', { name: file?.name ?? '' })}
      </DialogDescription>
    </DialogHeader>

    <div data-slot="dialog-body" class={['flex flex-col gap-4', DIALOG_BODY]}>
      {#if file}
        <div class="flex items-start gap-3">
          <FileThumbnail
            name={file.name}
            thumbnailUrl={file.thumbnailUrl}
            glyph={FileCode2Icon}
          />
          <div class="flex min-w-0 flex-1 flex-col gap-2">
            <span class="font-mono text-sm break-all">{file.name}</span>
            {#if facts.length > 0}
              <KeyValueList rows={facts} />
            {/if}
          </div>
        </div>
      {/if}

      {#each failures as failure (failure.id)}
        <div class="flex flex-col gap-1">
          <span class="text-sm font-medium">{failure.name}</span>
          <FaultAlert fault={failure.fault} />
        </div>
      {/each}

      {#if none}
        <p class="text-sm text-muted-foreground">
          {t('printPicker.noneReady')}
        </p>
      {/if}

      {#if searchable}
        <div class="flex flex-wrap items-center gap-3">
          <ListingSearch
            class="min-w-0 flex-1"
            value={query}
            onValue={(value: string) => {
              query = value;
            }}
            label={t('printPicker.selection.search')}
            placeholder={t('printPicker.selection.searchPlaceholder')}
          />
          <!-- eslint-disable-next-line svelte/no-restricted-html-elements -- Label's leading-none and select-none would change this checkbox row; design call pending -->
          <label class="flex items-center gap-2 text-sm">
            <Checkbox
              checked={selectedOnly}
              onCheckedChange={(value: boolean) => {
                selectedOnly = value;
              }}
            />
            {t('printPicker.selection.only')}
          </label>
        </div>
      {/if}

      {#if visible.length === 0 && searchable}
        <p class="text-sm text-muted-foreground">
          {t('printPicker.selection.noMatch')}
        </p>
      {/if}

      <div
        role="group"
        aria-label={t('printPicker.choice.label')}
        class="flex flex-col gap-1"
      >
        {#each visible as row (row.id)}
          {@const id = `${uid}-${row.id}`}
          {@const phase = phaseOf(row.id)}
          <div
            class={[
              'relative flex items-center gap-3 rounded-sm p-2',
              row.ready ? 'hover:bg-accent' : 'opacity-60',
            ]}
          >
            <Checkbox
              {id}
              class="relative z-10"
              aria-label={row.name}
              checked={row.ready && chosen.has(row.id)}
              disabled={!row.ready || busy}
              onCheckedChange={(checked: boolean) => {
                chosen.set([row.id], checked);
              }}
            />
            <div class="flex min-w-0 flex-1 flex-col gap-0.5">
              <span class="flex min-w-0 items-center gap-2">
                <PrinterGlyphIcon aria-hidden="true" class="size-4 shrink-0" />
                <Label
                  for={id}
                  class="min-w-0 cursor-pointer after:absolute after:inset-0"
                >
                  <span class="min-w-0 truncate">{row.name}</span>
                </Label>
                <Badge
                  variant={stateBadgeVariant(row.state)}
                  class="ml-auto shrink-0"
                >
                  {t(PRINTER_STATE_LABEL_KEY[row.state])}
                </Badge>
              </span>
              {#if phase}
                {@const percent =
                  phase.step === 'sending' && phase.progress !== null
                    ? phase.progress * PERCENT
                    : null}
                <span class="text-xs text-muted-foreground">
                  {t(
                    phase.step === 'writing'
                      ? 'printPicker.progress.writing'
                      : 'printPicker.progress.sending',
                  )}
                </span>
                <div class="flex items-center gap-3 text-xs">
                  <Progress
                    value={percent}
                    aria-label={t('printPicker.progress.label', {
                      printer: row.name,
                    })}
                    size="sm"
                    class="flex-1"
                  />
                  {#if percent !== null}
                    <span
                      class="shrink-0 font-mono text-muted-foreground tabular-nums"
                    >
                      {formatUnit(Math.round(percent), '%')}
                    </span>
                  {/if}
                </div>
              {:else if row.reasonKey}
                <span class="text-xs text-muted-foreground">
                  {reasonOf(row)}
                </span>
              {:else if slicedForOther(row, details?.printer_model)}
                <span class="text-xs text-warning-text">
                  {t('printPicker.otherModel', {
                    model: details?.printer_model ?? '',
                  })}
                </span>
              {/if}
            </div>
          </div>
        {/each}
      </div>

      {#if fileRef !== undefined}
        <SlotOutlet screen="print-dialog" name="print-section" {fileRef} />
      {/if}
    </div>

    <div class="flex min-w-0 flex-col gap-3">
      <p role="status" class="text-sm font-medium">
        {t('printPicker.selection.count', { count: sendable.length })}
      </p>
      <DialogFooter>
        <DialogClose>
          {#snippet child({ props }: { props: Record<string, unknown> })}
            <Button {...props} variant="secondary">
              {t('primitives.cancel')}
            </Button>
          {/snippet}
        </DialogClose>
        <Button
          disabled={busy || sendable.length === 0 || file === undefined}
          aria-busy={busy}
          onclick={() => {
            void send();
          }}
        >
          {#if busy}
            {t('printPicker.sending')}
          {:else if sendable.length > 0}
            {t('printPicker.selection.send', { count: sendable.length })}
          {:else}
            {t('printPicker.send')}
          {/if}
        </Button>
      </DialogFooter>
    </div>
  </DialogContent>
</Dialog>

<ConfirmDialog
  open={conflict !== undefined}
  confirmDisabled={replaceable.length === 0}
  onOpenChange={(next: boolean) => {
    if (!next) taken = undefined;
  }}
  title={t('printPicker.replace.title', {
    name: file?.name ?? '',
    count: conflict?.ids.length ?? 0,
  })}
  description={t('printPicker.replace.note', {
    count: replaceable.length,
  })}
  confirmLabel={t('printPicker.replace.confirm')}
  onConfirm={() => {
    const ids = conflict?.ids ?? [];
    taken = undefined;
    void sendTo(ids, true);
  }}
>
  {#if blockedReplacements.length > 0}
    <div role="status" class="flex flex-col gap-2 text-sm">
      <p>{t('printPicker.replace.available', { count: replaceable.length })}</p>
      <ul class="flex max-h-64 flex-col gap-2 overflow-y-auto">
        {#each blockedReplacements as blocked (blocked.id)}
          <li>
            <span class="block font-medium break-words">{blocked.name}</span>
            <span class="block break-words text-muted-foreground"
              >{blocked.fault.title}</span
            >
          </li>
        {/each}
      </ul>
    </div>
  {/if}
</ConfirmDialog>
