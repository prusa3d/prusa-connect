<!--
  Puts a spool on a printer tool. Step 1 lists the printers the person may
  observe, each as Core's printer list draws it: its picture, its name and
  model, and its state. One they may not operate is disabled and says why. A
  fleet can hold hundreds, so the list is searched by name, model or id, and
  read a page at a time as it scrolls. A printer with one tool takes the spool
  on a click. One with more tools or MMU slots opens them, and a tool the
  rules refuse is disabled with the reason.
-->
<script lang="ts">
  import { PRINTER_STATE_LABEL_KEY } from '@prusa/connect-ui/domain';
  import { t as coreT } from '@prusa/connect-ui/i18n';
  import {
    Alert,
    AlertTitle,
    Badge,
    Button,
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
    ListingSkeleton,
    PrinterIcon,
    printerModelLabel,
    Tile,
    ListingSearch,
  } from '@prusa/connect-ui/primitives';

  import { untrack } from 'svelte';

  import type {
    MaterialsService,
    Printer,
    PrinterTools,
    Spool,
    Tool,
  } from '../api';
  import { heldHost } from '../host';
  import FaultNote from '../materials-page/fault-note.svelte';
  import { OWN_MODULE } from '../own-module';
  import { artworkOf, stateBadge, stateOf } from '../printer-row';
  import { reasonText, refusalText } from '../printer-material/reasons';
  import { toolLabel } from '../printer-material/tool-label';

  let {
    service,
    spool,
    onclose,
    onassigned,
  }: {
    service: MaterialsService;
    /** The spool to assign; the dialog is open while there is one. */
    spool: Spool | undefined;
    onclose: () => void;
    onassigned: () => void;
  } = $props();

  const t = heldHost().t;

  let printers = $state.raw<readonly Printer[] | undefined>(undefined);
  /** Where the next page of printers starts, undefined after the last. */
  let next = $state<number | undefined>(undefined);
  let reading = $state(false);
  /** What the search box holds. */
  let typed = $state('');
  /** Bumped by each new search, so a page of an older one is dropped. */
  let generation = 0;
  /**
   * Bumped each time the dialog closes, so a printer's tools that land after
   * it closed are not shown when it opens for another spool.
   */
  let opening = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;
  /** The list's scrolling box, and the mark at its end that reads more. */
  let scroller = $state<HTMLElement | null>(null);
  let end = $state<HTMLElement | null>(null);

  const DEBOUNCE_MS = 250;
  /** The printer whose tools are open. */
  let printer = $state.raw<PrinterTools | undefined>(undefined);
  /** The tool the last assign went to, for the material it reports. */
  let target = $state.raw<Tool | undefined>(undefined);
  let busy = $state(false);
  let fault = $state.raw<unknown>(undefined);
  /** The fault is a page of printers that could not be read. */
  let pageFailed = $state(false);

  const refusal = $derived(
    refusalText(t, fault, 'assign', target?.reported?.abbreviation ?? ''),
  );

  /** Reads the next page, or the first of a new search when fresh. */
  const readPage = (fresh: boolean): void => {
    if (!fresh && (reading || next === undefined)) return;
    if (fresh) generation += 1;
    const mine = generation;
    if (fresh || pageFailed) fault = undefined;
    pageFailed = false;
    reading = true;
    service.printerPage(typed.trim(), fresh ? 0 : (next ?? 0)).then(
      (page) => {
        if (mine !== generation) return;
        printers = fresh
          ? page.printers
          : [...(printers ?? []), ...page.printers];
        next = page.next;
        reading = false;
      },
      (thrown: unknown) => {
        if (mine !== generation) return;
        fault = thrown;
        pageFailed = true;
        reading = false;
      },
    );
  };

  // The printers are read each time the dialog opens, so whether the person
  // may operate each one is the module's answer of now.
  $effect(() => {
    if (spool === undefined) return;
    untrack(() => readPage(true));
  });

  // The next page is read when the end of the list scrolls into view.
  $effect(() => {
    // bind:this answers null once the element is gone.
    if (scroller === null || end === null) return;
    const watch = new IntersectionObserver(
      (seen) => {
        if (seen.some((entry) => entry.isIntersecting)) readPage(false);
      },
      { root: scroller },
    );
    watch.observe(end);
    return () => watch.disconnect();
  });

  const search = (text: string): void => {
    typed = text;
    clearTimeout(timer);
    timer = setTimeout(() => readPage(true), DEBOUNCE_MS);
  };

  const close = (): void => {
    clearTimeout(timer);
    generation += 1;
    opening += 1;
    typed = '';
    next = undefined;
    reading = false;
    printers = undefined;
    printer = undefined;
    target = undefined;
    fault = undefined;
    pageFailed = false;
    onclose();
  };

  const assign = async (device: string, tool: Tool): Promise<void> => {
    if (spool === undefined || busy) return;
    busy = true;
    fault = undefined;
    target = tool;
    try {
      await service.assign(device, tool.id, { spool_id: spool.id });
      onassigned();
      close();
    } catch (thrown) {
      fault = thrown;
    } finally {
      busy = false;
    }
  };

  const open = async (picked: Printer): Promise<void> => {
    if (busy) return;
    const mine = opening;
    busy = true;
    fault = undefined;
    try {
      const answer = await service.tools(picked.id);
      if (mine !== opening) return;
      busy = false;
      const [only] = answer.tools;
      if (answer.tools.length === 1 && only !== undefined) {
        await assign(picked.id, only);
      } else {
        printer = answer;
      }
    } catch (thrown) {
      if (mine === opening) fault = thrown;
    } finally {
      busy = false;
    }
  };
</script>

<Dialog
  open={spool !== undefined}
  onOpenChange={(next: boolean) => {
    if (!next) close();
  }}
>
  <!-- The panel is portalled, so it carries the module's name for its
       utilities. -->
  <DialogContent data-module={OWN_MODULE}>
    <DialogHeader>
      <DialogTitle>
        {t('materials.assignPrinter.title', { id: spool?.id ?? 0 })}
      </DialogTitle>
    </DialogHeader>

    <div class="body flex flex-col gap-4" bind:this={scroller}>
      {#if refusal}
        <Alert variant="destructive" data-part="refusal">
          <AlertTitle>{refusal}</AlertTitle>
        </Alert>
      {:else}
        <FaultNote {fault} />
      {/if}
      {#if pageFailed}
        <div>
          <Button
            variant="secondary"
            size="sm"
            onclick={() => readPage(printers === undefined)}
          >
            {t('materials.assignPrinter.retry')}
          </Button>
        </div>
      {/if}

      {#if printer !== undefined}
        {@const shown = printer}
        <div class="flex flex-col gap-3">
          <div>
            <Button
              variant="ghost"
              size="sm"
              onclick={() => {
                printer = undefined;
                fault = undefined;
              }}
            >
              {t('materials.assignPrinter.back')}
            </Button>
          </div>
          <p class="text-base font-semibold">{shown.name}</p>
          {#if shown.tools.length === 0}
            <p class="text-sm text-muted-foreground">
              {t('materials.assignPrinter.noTools')}
            </p>
          {:else}
            <ul class="flex flex-col gap-2">
              {#each shown.tools as tool (tool.id)}
                {@const reason = reasonText(
                  t,
                  tool.assign,
                  'assign',
                  shown.state,
                  tool.reported?.abbreviation ?? '',
                )}
                <li>
                  <Tile
                    size="sm"
                    disabled={reason !== undefined || busy}
                    onclick={() => void assign(shown.id, tool)}
                  >
                    <span class="flex flex-col">
                      <span class="font-medium"
                        >{toolLabel(t, tool.id, shown.tools.length === 1)}</span
                      >
                      {#if reason !== undefined}
                        <span class="text-xs text-muted-foreground"
                          >{reason}</span
                        >
                      {/if}
                    </span>
                  </Tile>
                </li>
              {/each}
            </ul>
          {/if}
        </div>
      {:else}
        <ListingSearch
          label={t('materials.assignPrinter.search')}
          placeholder={t('materials.assignPrinter.searchPlaceholder')}
          value={typed}
          onValue={search}
        />
        {#if printers === undefined}
          {#if fault === undefined}
            <ListingSkeleton
              label={t('materials.assignPrinter.reading')}
              rows={3}
            />
          {/if}
        {:else if printers.length === 0 && !reading}
          <p class="text-sm text-muted-foreground">
            {typed.trim() === ''
              ? t('materials.assignPrinter.none')
              : t('materials.assignPrinter.noMatch')}
          </p>
        {:else}
          <ul class="flex flex-col gap-2">
            {#each printers as each (each.id)}
              {@const artwork = artworkOf(each.model)}
              {@const state = stateOf(each.connected, each.state)}
              <li>
                <Tile
                  size="sm"
                  data-part="printer"
                  disabled={!each.may_operate || busy}
                  onclick={() => void open(each)}
                >
                  <span class="flex items-center gap-3">
                    <!-- Decoration: the row's words name the printer and model. -->
                    <PrinterIcon
                      aria-hidden="true"
                      model={artwork}
                      reported={each.model}
                      class="size-10 shrink-0"
                    />
                    <span class="flex min-w-0 flex-1 flex-col">
                      <span class="truncate font-medium">{each.name}</span>
                      <span class="truncate text-xs text-muted-foreground">
                        {artwork === 'unknown' && each.model !== ''
                          ? each.model
                          : printerModelLabel(artwork)}
                      </span>
                      {#if !each.may_operate}
                        <span class="text-xs text-muted-foreground">
                          {t('materials.printer.notOperator')}
                        </span>
                      {/if}
                    </span>
                    <Badge variant={stateBadge(state)} class="shrink-0">
                      {coreT(PRINTER_STATE_LABEL_KEY[state])}
                    </Badge>
                  </span>
                </Tile>
              </li>
            {/each}
          </ul>
        {/if}
        {#if reading && printers !== undefined}
          <p class="text-sm text-muted-foreground">
            {t('materials.assignPrinter.readingMore')}
          </p>
        {/if}
        <span bind:this={end} aria-hidden="true" class="block h-px"></span>
      {/if}
    </div>
  </DialogContent>
</Dialog>

<style>
  /* A long list scrolls inside the screen, under the title. */
  .body {
    max-height: calc(100dvh - var(--spacing) * 32);
    overflow-y: auto;
  }
</style>
