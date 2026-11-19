<script module lang="ts">
  import EllipsisVerticalIcon from '@lucide/svelte/icons/ellipsis-vertical';
  import FileCode2Icon from '@lucide/svelte/icons/file-code-2';
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Badge from '../badge/badge.svelte';
  import type { BadgeVariant } from '../badge/types';
  import Button from '../button/button.svelte';
  import DataRow from './data-row.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/DataRow',
    component: DataRow,
    args: { primary: 'benchy.3mf' },
  });

  const statusVariant: Record<string, BadgeVariant> = {
    Ready: 'outline',
    Printing: 'outline',
    Queued: 'outline',
    Failed: 'destructive',
  };

  const files: {
    name: string;
    size?: string;
    modified?: string;
    status?: string;
  }[] = [
    {
      name: 'firmware_update_MK4S_verbose_release_candidate_final.gcode',
      size: '18.2 MB',
      modified: '2026-08-12',
      status: 'Ready',
    },
    {
      name: 'benchy.3mf',
      size: '4.1 MB',
      modified: '2026-08-14',
      status: 'Printing',
    },
    { name: 'bracket_v2.stl', size: '820 KB' },
    {
      name: 'enclosure_lid.stl',
      size: '1.1 MB',
      modified: '2026-08-11',
      status: 'Ready',
    },
    {
      name: 'test_cube_20mm.3mf',
      size: '96 KB',
      modified: '2026-08-10',
      status: 'Failed',
    },
    {
      name: 'nozzle_holder.step',
      size: '2.3 MB',
      modified: '2026-08-09',
      status: 'Ready',
    },
    {
      name: 'calibration_temp_tower.gcode',
      size: '3.4 MB',
      modified: '2026-08-08',
      status: 'Queued',
    },
    {
      name: 'phone_stand.3mf',
      size: '640 KB',
      modified: '2026-08-07',
      status: 'Ready',
    },
    {
      name: 'hinge_prototype_r4.stl',
      size: '410 KB',
      modified: '2026-08-06',
      status: 'Ready',
    },
    {
      name: 'vase_mode_test.gcode',
      size: '1.8 MB',
      modified: '2026-08-05',
      status: 'Ready',
    },
    {
      name: 'drone_arm_left.stl',
      size: '2.9 MB',
      modified: '2026-08-04',
      status: 'Ready',
    },
    {
      name: 'drone_arm_right.stl',
      size: '2.9 MB',
      modified: '2026-08-04',
      status: 'Ready',
    },
    {
      name: 'keychain_batch_12.3mf',
      size: '210 KB',
      modified: '2026-08-03',
      status: 'Ready',
    },
    { name: 'gear_20t.step', size: '540 KB' },
    {
      name: 'camera_mount.stl',
      size: '1.4 MB',
      modified: '2026-08-01',
      status: 'Ready',
    },
    {
      name: 'raspi_case_v3.3mf',
      size: '780 KB',
      modified: '2026-07-31',
      status: 'Ready',
    },
    {
      name: 'wall_hook.stl',
      size: '95 KB',
      modified: '2026-07-30',
      status: 'Ready',
    },
    {
      name: 'filament_guide.stl',
      size: '160 KB',
      modified: '2026-07-29',
      status: 'Ready',
    },
    {
      name: 'led_diffuser.3mf',
      size: '330 KB',
      modified: '2026-07-28',
      status: 'Ready',
    },
    {
      name: 'spool_holder.step',
      size: '1.0 MB',
      modified: '2026-07-27',
      status: 'Ready',
    },
  ];
</script>

{#snippet listedFile()}
  <div class="w-full max-w-96 rounded-sm border border-border">
    <DataRow
      primary="benchy.3mf"
      href="#benchy"
      secondaryValues={['4.1 MB', '2026-08-14']}
    >
      {#snippet actions()}
        <Button variant="ghost" size="icon-md" label="Actions for benchy.3mf">
          <EllipsisVerticalIcon />
        </Button>
      {/snippet}
    </DataRow>
  </div>
{/snippet}

<Story name="Default" asChild children={listedFile} />

<Story name="With A Leading Glyph" asChild>
  <DataRow
    primary="retract_tune.bgcode"
    secondaryValues={['1.1 MB', '1 d ago']}
  >
    {#snippet leading()}
      <FileCode2Icon class="size-4 text-muted-foreground" aria-hidden="true" />
    {/snippet}
  </DataRow>
</Story>

<Story name="Roomy" asChild>
  <DataRow primary="benchy.bgcode" secondaryValues={['Printed', 'MK4S']} roomy>
    {#snippet leading()}
      <FileCode2Icon class="size-4 text-muted-foreground" aria-hidden="true" />
    {/snippet}
  </DataRow>
</Story>

<Story name="Realistic Listing" asChild>
  <div class="rounded-sm border border-border">
    {#each files as file (file.name)}
      {#snippet status()}
        <Badge variant={statusVariant[file.status ?? ''] ?? 'outline'}>
          {file.status}
        </Badge>
      {/snippet}
      <DataRow
        primary={file.name}
        href={`#${file.name}`}
        secondaryValues={[file.status && status, file.size, file.modified]}
      >
        {#snippet actions()}
          <Button
            variant="ghost"
            size="icon-md"
            label={`Actions for ${file.name}`}
          >
            <EllipsisVerticalIcon />
          </Button>
        {/snippet}
      </DataRow>
    {/each}
  </div>
</Story>

<Story
  name="Actions Menu Does Not Swallow Row Activation"
  tags={['!dev', '!autodocs']}
  asChild
  children={listedFile}
/>

<Story name="A Pathologically Long Value Clips" asChild>
  <DataRow
    primary="calibration_temp_tower_PLA_v3.gcode"
    secondaryValues={[
      'Ready',
      'This value is far longer than any row is wide and would push the actions off the end if the value span could not shrink',
      '2026-08-19',
    ]}
  >
    {#snippet actions()}
      <Button variant="ghost" size="icon-md" label="Actions for the row">
        <EllipsisVerticalIcon />
      </Button>
    {/snippet}
  </DataRow>
</Story>

<Story
  name="A Nowrap Control Truncates Inside Its Slot"
  tags={['!dev', '!autodocs']}
  asChild
>
  <div class="max-w-story-narrow-row">
    {#snippet printer()}
      <a
        href="#/printers/mk4s-3"
        class="inline-flex min-w-0 items-center gap-1.5 truncate underline"
      >
        <span class="min-w-0 truncate">
          MK4S, Print Farm Rack 3, Shelf B, beside the filament dryer
        </span>
      </a>
    {/snippet}
    <DataRow
      primary="Bracket_Left_Reinforced_v4_FINAL.gcode"
      secondaryValues={['Finished', printer]}
    >
      {#snippet actions()}
        <Button variant="ghost" size="icon-md" label="Actions for the row">
          <EllipsisVerticalIcon />
        </Button>
      {/snippet}
    </DataRow>
  </div>
</Story>
