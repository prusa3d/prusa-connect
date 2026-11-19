<!-- The name the printer announces itself under on the network. -->
<script lang="ts">
  import type { CommandProjection } from '../../../domain';
  import { t } from '../../../i18n';
  import { Button, PromptDialog } from '../../../primitives';
  import {
    blocked,
    HOSTNAME_MAX,
    hostnameProblem,
    VERBS,
    type SectionProps,
  } from './machine-controls';

  let {
    projection,
    busy,
    onCommand,
  }: Pick<SectionProps, 'busy' | 'onCommand'> & {
    projection: CommandProjection;
  } = $props();

  let open = $state(false);
  let draft = $state('');

  const typed = $derived(draft.trim());
  const problem = $derived(hostnameProblem(typed));

  const close = () => {
    open = false;
    draft = '';
  };
</script>

<Button
  variant="secondary"
  size="md"
  disabled={blocked(projection, busy)}
  onclick={() => {
    open = true;
  }}
>
  {t('printerDetailControls.hostname.action')}
</Button>

<!-- The printer reports no hostname, so the field starts empty. -->
<PromptDialog
  {open}
  onOpenChange={(next: boolean) => {
    if (next) open = true;
    else close();
  }}
  title={t('printerDetailControls.hostname.title')}
  description={t('printerDetailControls.hostname.form')}
  label={t('printerDetailControls.hostname.field')}
  value={draft}
  onValueChange={(next: string) => {
    draft = next;
  }}
  error={problem === undefined ? undefined : t(problem, { max: HOSTNAME_MAX })}
  confirmLabel={t('printerDetailControls.hostname.confirm')}
  onConfirm={() => {
    void onCommand(VERBS.hostname, { hostname: typed });
    close();
  }}
/>
