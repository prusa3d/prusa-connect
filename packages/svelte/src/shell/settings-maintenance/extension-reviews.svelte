<!--
  What each extension in an offline bundle asks for, as preparation found it.
  A request the installed release did not make and a version older than the
  installed one are marked, because applying needs the operator's approval
  for both.
-->
<script lang="ts">
  import { t } from '../../i18n';
  import { Badge } from '../../primitives';
  import type { ExtensionReview } from '../../data';

  let { reviews }: { reviews: readonly ExtensionReview[] } = $props();

  const isNew = (review: ExtensionReview, ask: string): boolean =>
    (review.new_asks ?? []).includes(ask);
</script>

<div data-part="extension-reviews" class="text-body">
  <h3 class="font-medium">{t('settingsMaintenance.reviews.heading')}</h3>
  <p class="mt-1 text-muted-foreground">
    {t('settingsMaintenance.reviews.description')}
  </p>
  <ul class="mt-3 flex flex-col gap-3">
    {#each reviews as review (review.module)}
      {@const asks = review.asks ?? []}
      {@const added = (review.new_asks ?? []).length > 0}
      <li
        class="flex flex-col gap-2 border-t border-border pt-3 first:border-t-0 first:pt-0"
      >
        <div class="flex flex-wrap items-center gap-2">
          <span class="font-mono font-medium break-all">{review.module}</span>
          {#if review.downgrade}
            <Badge variant="warning">
              {t('settingsMaintenance.reviews.older')}
            </Badge>
          {/if}
          {#if added}
            <Badge variant="warning">
              {t('settingsMaintenance.reviews.newAsks')}
            </Badge>
          {/if}
        </div>
        <p class="text-xs text-muted-foreground">
          {#if review.installed_version}
            {t('settingsMaintenance.reviews.versions', {
              installed: review.installed_version,
              version: review.version,
            })}
          {:else}
            {t('settingsMaintenance.reviews.firstInstall', {
              version: review.version,
            })}
          {/if}
        </p>
        {#if asks.length === 0}
          <p class="text-muted-foreground">
            {t('settingsMaintenance.reviews.noAsks')}
          </p>
        {:else}
          <ul
            class="flex flex-col gap-1"
            aria-label={t('settingsMaintenance.reviews.asks', {
              module: review.module,
            })}
          >
            {#each asks as ask (ask)}
              <li class="flex flex-wrap items-center gap-2">
                <span class="font-mono text-xs break-all">{ask}</span>
                {#if isNew(review, ask)}
                  <Badge variant="warning">
                    {t('settingsMaintenance.reviews.new')}
                  </Badge>
                {/if}
              </li>
            {/each}
          </ul>
        {/if}
      </li>
    {/each}
  </ul>
</div>
