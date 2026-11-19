<!--
  What to check when a printer does not appear. It is closed while a printer
  still has time to ask, and the waiting step opens it once that time has
  passed. The status words are the ones the printer's own Prusa Connect
  screen shows, so a person can match what they read there.
-->
<script lang="ts">
  import { t } from '../../i18n';
  import {
    Collapsible,
    CollapsibleContent,
    CollapsibleTrigger,
  } from '../../primitives';

  let { open = $bindable(false) }: { open?: boolean } = $props();

  // The words are the printer's own, and its display shows them in English
  // unless its language is switched, so they are keys like any other text.
  const statuses = $derived([
    {
      word: t('addPrinter.help.status.refused.word'),
      means: t('addPrinter.help.status.refused.means'),
    },
    {
      word: t('addPrinter.help.status.unauthorized.word'),
      means: t('addPrinter.help.status.unauthorized.means'),
    },
    {
      word: t('addPrinter.help.status.server.word'),
      means: t('addPrinter.help.status.server.means'),
    },
    {
      word: t('addPrinter.help.status.dns.word'),
      means: t('addPrinter.help.status.dns.means'),
    },
    {
      word: t('addPrinter.help.status.network.word'),
      means: t('addPrinter.help.status.network.means'),
    },
  ]);
</script>

<Collapsible bind:open data-part="waiting-help">
  <CollapsibleTrigger variant="inline">
    {t('addPrinter.help.heading')}
  </CollapsibleTrigger>
  <CollapsibleContent>
    <div class="flex flex-col gap-4 pt-3 text-sm">
      <ul class="flex list-inside list-disc flex-col gap-2">
        <li>{t('addPrinter.help.firewall')}</li>
        <li>{t('addPrinter.help.address')}</li>
        <li>{t('addPrinter.help.network')}</li>
        <li>{t('addPrinter.help.again')}</li>
      </ul>
      <div class="flex flex-col gap-2">
        <h3 class="font-medium">{t('addPrinter.help.status.heading')}</h3>
        <dl class="flex flex-col gap-2 rounded-md border border-border p-3">
          {#each statuses as status (status.word)}
            <div class="flex flex-wrap gap-x-4 gap-y-0.5">
              <dt class="w-28 shrink-0 font-mono font-semibold">
                {status.word}
              </dt>
              <dd class="min-w-48 flex-1 text-muted-foreground">
                {status.means}
              </dd>
            </div>
          {/each}
        </dl>
      </div>
    </div>
  </CollapsibleContent>
</Collapsible>
