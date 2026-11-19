<!--
  One object of a 3MF project in a dialog over the page: a large picture of it
  on one side, and on the other its size, the extruders it prints with, its
  parts and layer ranges, and each of its copies with a box to tick it. On a
  narrow screen the two sides stack.

  It opens on the object the browser names in `opened`, and closing it gives
  the name back. A copy ticked here is ticked on the plan behind it. The
  dialog is drawn outside the module's slot, so it names the module itself,
  which is what lets this module's own utilities reach it.
-->
<script lang="ts">
  import { formatList, formatNumber, NAMESPACE, t } from '../i18n';
  import {
    Badge,
    SelectCheckbox,
    Skeleton,
    ToggleGroup,
    ToggleGroupItem,
    KeyValueList,
    type KeyValueRow,
    Dialog,
    DialogContent,
    DialogDescription,
    DialogHeader,
    DialogTitle,
  } from '@prusa/connect-ui/primitives';
  import { getProjectBrowser } from './browser.svelte';
  import ExtruderChips from './extruder-chips.svelte';
  import { bedLabel, copySize, objectName, PLACEMENT_KEY } from './labels';
  import {
    bedsOfObject,
    instanceKey,
    filamentsFor,
    objectColour,
    placementOf,
    type Project,
  } from './model';
  import type { MeshDescription } from '../projects.svelte';
  import ProjectObject3d from './project-object-3d.svelte';
  import ProjectObjectOutline from './project-object-outline.svelte';
  import ProjectObjectParts from './project-object-parts.svelte';

  interface Props {
    project: Project;
    /**
     * The object at an index as triangles, once the server has answered.
     * Absent where this page cannot ask for one, and the panel then shows the
     * object from above only.
     */
    mesh?: ((object: number) => MeshDescription | undefined) | undefined;
  }

  let { project, mesh }: Props = $props();

  // How the picture shows the object. 3D where a mesh can be asked for.
  let look = $state<'3d' | 'above'>('3d');
  // The browser here cannot draw in 3D.
  let flat = $state(false);

  const browser = getProjectBrowser();

  const index = $derived(browser.opened);
  const object = $derived(
    index === undefined ? undefined : project.objects[index],
  );
  const name = $derived(objectName(object?.name));
  const colour = $derived(
    object === undefined ? undefined : objectColour(project, object),
  );
  const filaments = $derived(filamentsFor(project, object));
  const first = $derived(object?.instances[0]);

  const solid = $derived(
    mesh === undefined || index === undefined || flat ? undefined : mesh(index),
  );
  const showSolid = $derived(mesh !== undefined && !flat && look === '3d');
  const simplified = $derived(
    solid?.mesh !== undefined &&
      solid.mesh.source_triangles > solid.mesh.triangles.length / 3,
  );

  const beds = $derived(
    object === undefined
      ? ''
      : formatList(bedsOfObject(object).map((ref) => bedLabel(project, ref))),
  );

  const facts: KeyValueRow[] = $derived(
    object === undefined || first === undefined
      ? []
      : (
          [
            {
              id: 'size',
              label: t('projectView.panel.size'),
              value: copySize(first),
            },
            {
              id: 'copies',
              label: t('projectView.objects.copies'),
              value: t('projectView.objects.printableOf', {
                printable: formatNumber(
                  object.instances.filter((at) => at.printable).length,
                ),
                count: formatNumber(object.instances.length),
              }),
            },
            {
              id: 'beds',
              label: t('projectView.objects.beds'),
              value: beds,
            },
          ] satisfies KeyValueRow[]
        ).filter((row) => row.value !== ''),
  );

  const PAINT_KEYS = [
    ['multimaterial', 'projectView.paint.colours'],
    ['supports', 'projectView.paint.supports'],
    ['seams', 'projectView.paint.seams'],
  ] as const;

  const painting = $derived(
    PAINT_KEYS.filter(([field]) => object?.painting?.[field] === true).map(
      ([, key]) => key,
    ),
  );

  const copyState = (at: number): string => {
    const copy = object?.instances[at];
    if (copy === undefined) return '';
    const placement = placementOf(copy);

    return [
      copy.bed === undefined ? '' : bedLabel(project, copy.bed),
      copy.printable ? '' : t('projectView.state.nonPrintable'),
      placement === 'inside' || placement === 'unknown'
        ? ''
        : t(PLACEMENT_KEY[placement]),
    ]
      .filter((part) => part !== '')
      .join(', ');
  };
</script>

<Dialog
  open={object !== undefined}
  onOpenChange={(open: boolean) => {
    if (!open) browser.opened = undefined;
  }}
>
  <DialogContent
    size="xl"
    data-module={NAMESPACE}
    class="max-h-dvh content-start gap-6 overflow-y-auto"
  >
    {#if object !== undefined && index !== undefined}
      <DialogHeader class="pe-8">
        <DialogTitle class="break-words">{name}</DialogTitle>
        <DialogDescription>
          {t('projectView.panel.description', {
            count: object.instances.length,
          })}
        </DialogDescription>
      </DialogHeader>

      <div
        class="grid grid-cols-1 content-start gap-6 md:grid-cols-2"
        data-slot="project-object-panel"
      >
        {#if first !== undefined}
          <div class="grid grid-cols-1 content-start gap-2">
            {#if mesh !== undefined && !flat}
              <ToggleGroup
                type="single"
                size="sm"
                variant="outline"
                value={look}
                onValueChange={(value: string) => {
                  if (value === '3d' || value === 'above') look = value;
                }}
                aria-label={t('projectView.panel.look')}
              >
                <ToggleGroupItem value="3d">
                  {t('projectView.panel.look3d')}
                </ToggleGroupItem>
                <ToggleGroupItem value="above">
                  {t('projectView.panel.lookAbove')}
                </ToggleGroupItem>
              </ToggleGroup>
            {/if}
            <div
              class="aspect-square w-full overflow-hidden rounded-md border border-border bg-muted/40 p-2"
            >
              {#if showSolid && solid === undefined}
                <div
                  class="size-full"
                  role="status"
                  aria-label={t('projectView.panel.meshLoading')}
                >
                  <Skeleton class="size-full" />
                </div>
              {:else if showSolid && solid?.mesh !== undefined}
                <ProjectObject3d
                  mesh={solid.mesh}
                  {filaments}
                  label={t('projectView.panel.solid', { name })}
                  onunavailable={() => {
                    flat = true;
                  }}
                />
              {:else}
                <ProjectObjectOutline
                  instance={first}
                  {colour}
                  label={t('projectView.panel.outline', { name })}
                />
              {/if}
            </div>
            {#if showSolid && solid?.fault !== undefined}
              <p class="text-xs text-muted-foreground">
                {t('projectView.panel.meshFailed')}
              </p>
            {:else if showSolid && solid?.mesh?.box === true}
              <p class="text-xs text-muted-foreground">
                {t('projectView.panel.meshBox')}
              </p>
            {:else if showSolid && solid?.mesh !== undefined}
              <p class="text-xs text-muted-foreground">
                {simplified
                  ? t('projectView.panel.meshSimplified', {
                      count: solid.mesh.source_triangles,
                    })
                  : t('projectView.panel.meshHint')}
              </p>
            {/if}
          </div>
        {/if}

        <div class="grid min-w-0 grid-cols-1 content-start gap-6">
          <KeyValueList rows={facts} labelWidth="14rem" />

          {#if (object.extruders ?? []).length > 0 || painting.length > 0}
            <section class="grid grid-cols-1 content-start gap-2">
              <h3 class="text-sm font-medium">
                {t('projectView.panel.printsWith')}
              </h3>
              <div class="flex flex-wrap items-center gap-2">
                <ExtruderChips
                  extruders={object.extruders ?? []}
                  {filaments}
                  label={t('projectView.objects.extruders', { name })}
                />
                {#each painting as key (key)}
                  <Badge variant="info">{t(key)}</Badge>
                {/each}
              </div>
            </section>
          {/if}

          <section class="grid grid-cols-1 content-start gap-2">
            <h3 class="text-sm font-medium">{t('projectView.panel.parts')}</h3>
            <ProjectObjectParts {project} object={index} />
          </section>

          <section class="grid grid-cols-1 content-start gap-2">
            <h3 class="text-sm font-medium">
              {t('projectView.panel.copies')}
            </h3>
            <ul class="grid grid-cols-1 content-start gap-1">
              {#each object.instances.keys() as at (at)}
                {@const label = t('projectView.copy.number', {
                  number: formatNumber(at + 1),
                })}
                <li class="flex min-w-0 items-center gap-3 py-1 text-sm">
                  {#if browser.ticking}
                    <SelectCheckbox
                      selection={browser.selection}
                      key={instanceKey(index, at)}
                      disabled={!browser.canSelect(index, at)}
                      label={t('projectView.list.select', {
                        name: `${name}, ${label}`,
                      })}
                    />
                  {/if}
                  <span class="shrink-0 font-medium">{label}</span>
                  <span class="min-w-0 truncate text-muted-foreground">
                    {copyState(at)}
                  </span>
                </li>
              {/each}
            </ul>
          </section>
        </div>
      </div>
    {/if}
  </DialogContent>
</Dialog>
