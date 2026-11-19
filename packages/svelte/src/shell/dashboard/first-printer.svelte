<!--
  What the dashboard says above everything else while the server has no
  printer: the first thing a person wants after the setup is to add one.
-->
<script lang="ts">
  import { t } from '../../i18n';
  import { Button, PrinterGlyphIcon } from '../../primitives';

  let { onadd }: { onadd: () => void } = $props();
</script>

<div
  class="flex flex-col gap-4 rounded-md border border-primary/40 bg-primary/5 p-4 @md/dashboard:flex-row @md/dashboard:items-center"
  data-part="first-printer"
>
  <PrinterGlyphIcon class="size-8 shrink-0 text-primary" aria-hidden="true" />
  <div class="flex flex-1 flex-col gap-1">
    <h2 class="text-base font-semibold">
      {t('dashboard.firstPrinter.heading')}
    </h2>
    <p class="text-sm text-muted-foreground">
      {t('dashboard.firstPrinter.note')}
    </p>
  </div>
  <Button class="w-fit" onclick={onadd}>
    {t('dashboard.firstPrinter.add')}
  </Button>
</div>
