<!--
  One page of parts as cards: the picture, the name, the line under it, the
  tags, and the newest version. The whole card is the link, with the picture
  and the text read as one name by a screen reader.
-->
<script lang="ts">
  import { formatDateTime } from '@prusa/connect-ui/i18n';
  import {
    Badge,
    Tile,
    type TilePressProps,
  } from '@prusa/connect-ui/primitives';

  import { pictureUrl } from '../presentation';
  import { heldSession } from '../session';
  import { CARD_GRID } from './layout';
  import FolderLink from './folder-link.svelte';
  import PartPicture from './part-picture.svelte';

  const { catalogue, navigation, host, files } = heldSession();
  const t = host.t;

  /** How many tags a card shows before it says how many more there are. */
  const TAGS_SHOWN = 3;
</script>

<ul class={CARD_GRID} aria-label={t('parts.listing.parts')} data-part="cards">
  {#each catalogue.rows as part (part.id)}
    <li>
      <Tile size="sm" class="h-full">
        {#snippet child({ props }: { props: TilePressProps })}
          <FolderLink
            {...props}
            spot={{
              folder: navigation.spot.folder,
              part: part.id,
              q: navigation.spot.q,
              tag: navigation.spot.tag,
            }}
            class={[props.class, 'grid content-start gap-y-2']}
          >
            <PartPicture
              url={pictureUrl(files, part.picture)}
              class="aspect-square w-full"
            />
            <span class="grid min-w-0 gap-y-1 px-1 pb-1" data-part="card">
              <span class="truncate text-sm font-medium" data-part="part-name"
                >{part.name}</span
              >
              {#if part.summary}
                <span class="line-clamp-2 text-xs text-muted-foreground"
                  >{part.summary}</span
                >
              {/if}
              {#if part.tags.length > 0}
                <span class="flex flex-wrap gap-1">
                  {#each part.tags.slice(0, TAGS_SHOWN) as tag (tag)}
                    <Badge variant="outline">{tag}</Badge>
                  {/each}
                  {#if part.tags.length > TAGS_SHOWN}
                    <Badge variant="secondary">
                      {t('parts.card.moreTags', {
                        count: part.tags.length - TAGS_SHOWN,
                      })}
                    </Badge>
                  {/if}
                </span>
              {/if}
              <span
                class="flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground"
              >
                {#if part.latest}
                  <span class="font-mono"
                    >{t('parts.version.short', {
                      number: part.latest.number,
                    })}</span
                  >
                  <span
                    >{t('parts.card.files', { count: part.latest.files })}</span
                  >
                {:else}
                  <span>{t('parts.card.noFiles')}</span>
                {/if}
                <span class="whitespace-nowrap"
                  >{formatDateTime(part.changed)}</span
                >
              </span>
            </span>
          </FolderLink>
        {/snippet}
      </Tile>
    </li>
  {/each}
</ul>
