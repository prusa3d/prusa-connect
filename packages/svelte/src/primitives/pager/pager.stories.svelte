<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Pager from './pager.svelte';
  import type { PageSize } from './sizes';

  const { Story } = defineMeta({
    title: 'Primitives/Pager',
    component: Pager,
  });

  const pagesOf = (total: number, size: number): number =>
    Math.max(1, Math.ceil(total / size));

  /** Where one story's own listing is, and how much of it it draws. */
  interface Held {
    page: number;
    size: PageSize;
  }
</script>

<script lang="ts">
  // A pager that does not move when it is pressed shows half the component, so
  // each story holds its own page and its own size.
  const jobs: Held = $state({ page: 0, size: 25 });
  const last: Held = $state({ page: 2, size: 25 });
  const short: Held = $state({ page: 0, size: 25 });
  const search: Held = $state({ page: 0, size: 5 });
</script>

<!--
  Beyond the React set. React ships no pager.stories.tsx, so nothing here is a
  port. These are the states the props allow: the middle of a list, its last
  page, a list that fits on one page, and a listing that writes its own count.
-->

<Story name="Default" asChild>
  <div class="w-story-pager">
    <Pager
      page={jobs.page}
      pageCount={pagesOf(61, jobs.size)}
      pageSize={jobs.size}
      total={61}
      unit="jobs"
      onPageChange={(next: number) => (jobs.page = next)}
      onPageSizeChange={(next: PageSize) => {
        jobs.size = next;
        jobs.page = 0;
      }}
    />
  </div>
</Story>

<Story name="On The Last Page" asChild>
  <div class="w-story-pager">
    <Pager
      page={last.page}
      pageCount={pagesOf(61, last.size)}
      pageSize={last.size}
      total={61}
      unit="jobs"
      onPageChange={(next: number) => (last.page = next)}
      onPageSizeChange={(next: PageSize) => {
        last.size = next;
        last.page = 0;
      }}
    />
  </div>
</Story>

<Story name="One Page Only" asChild>
  <div class="w-story-pager">
    <Pager
      page={short.page}
      pageCount={pagesOf(4, short.size)}
      pageSize={short.size}
      total={4}
      unit="files"
      onPageChange={(next: number) => (short.page = next)}
      onPageSizeChange={(next: PageSize) => {
        short.size = next;
        short.page = 0;
      }}
    />
  </div>
</Story>

<Story name="The Listing Writes Its Own Count" asChild>
  <div class="w-story-pager">
    <Pager
      page={search.page}
      pageCount={pagesOf(12, search.size)}
      pageSize={search.size}
      onPageChange={(next: number) => (search.page = next)}
      onPageSizeChange={(next: PageSize) => {
        search.size = next;
        search.page = 0;
      }}
    >
      {#snippet count()}
        <p aria-live="polite" class="text-xs font-medium text-muted-foreground">
          12 of 4 208 files match “benchy”
        </p>
      {/snippet}
    </Pager>
  </div>
</Story>
