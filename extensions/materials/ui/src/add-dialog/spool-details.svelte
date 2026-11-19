<!--
  Step 2's fields: how many spools, the package, the weights, what is left,
  where they are kept, and a summary of what the submit will make.

  Every field writes straight onto the draft, which is what the dialog posts.
  A package fills the weights and the length; the person may still change
  them.
-->
<script lang="ts">
  import { formatNumber } from '@prusa/connect-ui/i18n';
  import {
    Field,
    Input,
    Label,
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
    Textarea,
  } from '@prusa/connect-ui/primitives';

  import { heldHost } from '../host';
  import LocationField from '../location-field.svelte';
  import { typedWeight } from '../materials-page/weights';
  import type { SpoolDraftForm } from './draft.svelte';

  let {
    draft,
    section = 'all',
  }: { draft: SpoolDraftForm; section?: 'all' | 'remaining' | 'optional' } =
    $props();

  const t = heldHost().t;
  const uid = $props.id();

  const NONE = '*';
  const packages = $derived(draft.material?.packages ?? []);
  const packageItems = $derived([
    { value: NONE, label: t('materials.spool.noPackage') },
    ...packages.map((p) => ({
      value: p.uuid,
      label: packageLabel(p),
    })),
  ]);

  // A package's label: its name, then the filament weight, the empty
  // spool and the diameter, each only when the catalogue knows it.
  const packageLabel = (p: (typeof packages)[number]): string => {
    const spec = [
      p.net_weight_g === undefined
        ? undefined
        : t('materials.spool.packageNet', { g: Math.round(p.net_weight_g) }),
      p.empty_weight_g === undefined
        ? undefined
        : t('materials.spool.packageEmpty', {
            g: Math.round(p.empty_weight_g),
          }),
      p.diameter_um === undefined
        ? undefined
        : t('materials.spool.packageDiameter', {
            mm: formatNumber(p.diameter_um / 1000, {
              minimumFractionDigits: 2,
              maximumFractionDigits: 2,
            }),
          }),
    ].filter((part) => part !== undefined);

    return [p.container_name || p.slug, spec.join(' · ')]
      .filter((part) => part !== '')
      .join(' — ');
  };

  const typed = (event: Event): string =>
    (event.currentTarget as HTMLInputElement).value;
  /** A number field: empty is unknown. */
  const grams = (event: Event): number | undefined => {
    const value = typed(event);

    return value === '' ? undefined : Number(value);
  };
  const weight = (event: Event): number | undefined =>
    typedWeight(typed(event));
  const weightProps = (value: number | undefined) => ({
    type: 'text',
    inputmode: 'decimal',
    value: value ?? '',
  });
</script>

{#snippet remaining()}
  <Field
    label={t('materials.spool.remaining')}
    control={Input}
    controlProps={{
      name: 'remaining_weight_g',
      ...weightProps(draft.remainingG),
      oninput: (e: Event) => (draft.remainingG = weight(e)),
    }}
  />
{/snippet}

{#if section === 'remaining'}
  {@render remaining()}
{:else}
  <div
    data-part="spool-details"
    class={['fields grid gap-4', { compact: section === 'optional' }]}
  >
    {#if packages.length > 0}
      <!-- A row of its own and first: a package names its weights in full, which
         a narrow cell cut through, and choosing it fills the fields below. -->
      <div class="col-span-full flex min-w-0 flex-col gap-2">
        <Label for="{uid}-package">{t('materials.spool.package')}</Label>
        <Select
          value={draft.packageUuid ?? NONE}
          onValueChange={(chosen: string) => {
            draft.choosePackage(chosen === NONE ? undefined : chosen);
          }}
          items={packageItems}
        >
          <SelectTrigger id="{uid}-package" class="w-full min-w-0">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {#each packageItems as item (item.value)}
              <SelectItem value={item.value} label={item.label}>
                {item.label}
              </SelectItem>
            {/each}
          </SelectContent>
        </Select>
      </div>
    {/if}

    {#if section === 'all'}
      <Field
        label={t('materials.spool.quantity')}
        control={Input}
        controlProps={{
          name: 'quantity',
          type: 'number',
          inputmode: 'numeric',
          min: 1,
          max: 100,
          step: 1,
          required: true,
          value: draft.quantity,
          oninput: (e: Event) => (draft.quantity = grams(e) ?? 0),
        }}
      />
    {/if}

    <Field
      label={t('materials.spool.full')}
      control={Input}
      controlProps={{
        name: 'full_weight_g',
        ...weightProps(draft.fullWeightG),
        oninput: (e: Event) => (draft.fullWeightG = weight(e)),
      }}
    />
    <Field
      label={t('materials.spool.empty')}
      control={Input}
      controlProps={{
        name: 'empty_weight_g',
        ...weightProps(draft.emptyWeightG),
        oninput: (e: Event) => (draft.emptyWeightG = weight(e)),
      }}
    />
    <Field
      label={t('materials.spool.length')}
      control={Input}
      controlProps={{
        name: 'length_mm',
        type: 'number',
        inputmode: 'numeric',
        min: 0,
        step: 1,
        value: draft.lengthMM ?? '',
        oninput: (e: Event) => (draft.lengthMM = grams(e)),
      }}
    />

    {#if section === 'all'}
      {@render remaining()}
    {/if}

    <LocationField
      class="col-span-full"
      value={draft.locationId}
      onpick={(id: number | undefined) => (draft.locationId = id)}
    />
    <div class="col-span-full flex flex-col gap-2">
      <Label for="{uid}-notes">{t('materials.spool.notes')}</Label>
      <Textarea id="{uid}-notes" rows={2} bind:value={draft.notes} />
    </div>

    {#if section === 'all'}
      <dl
        data-part="summary"
        class="col-span-full grid grid-cols-2 gap-x-4 gap-y-1 rounded-sm bg-muted p-3 text-sm"
      >
        <dt class="text-muted-foreground">
          {t('materials.spool.summary.count')}
        </dt>
        <dd>{Number.isFinite(draft.quantity) ? draft.quantity : '–'}</dd>
        <dt class="text-muted-foreground">
          {t('materials.spool.summary.each')}
        </dt>
        <dd>
          {draft.remaining === undefined
            ? '–'
            : t('materials.spool.grams', { g: draft.remaining })}
        </dd>
        <dt class="text-muted-foreground">
          {t('materials.spool.summary.total')}
        </dt>
        <dd>
          {draft.remaining === undefined || !Number.isFinite(draft.quantity)
            ? '–'
            : t('materials.spool.grams', {
                g: draft.remaining * draft.quantity,
              })}
        </dd>
      </dl>
    {/if}
  </div>
{/if}

<style>
  .fields.compact {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .fields {
    grid-template-columns: repeat(
      auto-fit,
      minmax(calc(var(--spacing) * 56), 1fr)
    );
  }
</style>
