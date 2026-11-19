<!--
  Create custom material, from the page's header: a material the catalogue
  does not carry belongs in the list whether or not a spool of it exists yet.
  It makes the material, says so, and closes; adding spools of it is the row's
  +.
-->
<script lang="ts">
  import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
    toast,
  } from '@prusa/connect-ui/primitives';

  import CustomMaterialForm from '../add-dialog/custom-material-form.svelte';
  import { type MaterialFields, materialsOn } from '../api';
  import { heldHost } from '../host';
  import { OWN_MODULE } from '../own-module';

  let {
    open,
    onclose,
    oncreated,
  }: {
    open: boolean;
    onclose: () => void;
    oncreated: () => void;
  } = $props();

  const host = heldHost();
  const t = host.t;
  const service = materialsOn(host);

  let busy = $state(false);
  let fault = $state.raw<unknown>(undefined);

  const save = async (fields: MaterialFields): Promise<void> => {
    busy = true;
    fault = undefined;
    try {
      await service.createCustom(fields);
      toast.success(t('materials.custom.created'), {
        description: fields.name,
      });
      oncreated();
      onclose();
    } catch (thrown) {
      fault = thrown;
    } finally {
      busy = false;
    }
  };
</script>

<Dialog
  {open}
  onOpenChange={(next: boolean) => {
    if (!next) onclose();
  }}
>
  <!-- Portalled, so the panel carries the module's name for the utilities
       inside it. A scoped rule never matches the scope's root, so the
       panel's own cap is the host's. -->
  <DialogContent
    data-module={OWN_MODULE}
    size="lg"
    class="max-h-dialog-height overflow-y-auto"
  >
    <DialogHeader>
      <DialogTitle>{t('materials.custom.title')}</DialogTitle>
    </DialogHeader>
    {#if open}
      <CustomMaterialForm
        submitLabel={t('materials.custom.save')}
        {busy}
        {fault}
        onsave={(fields: MaterialFields) => void save(fields)}
        oncancel={onclose}
      />
    {/if}
  </DialogContent>
</Dialog>
