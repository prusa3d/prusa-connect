<script lang="ts">
  import { t } from '../../i18n';
  import Button from '../button/button.svelte';
  import Input from '../input/input.svelte';
  import visibleIcon from './assets/password-visible.svg';
  import hiddenIcon from './assets/password-hidden.svg';
  import type { PasswordInputProps } from './types';

  let {
    label = t('passwordInput.label'),
    fill = false,
    class: className,
    ...rest
  }: PasswordInputProps = $props();
  let revealed = $state(false);

  // A successful form reset clears the controlled value without an input event.
  $effect(() => {
    if (rest.value === '') revealed = false;
  });
</script>

<div
  data-slot="password-input"
  class={['relative w-full min-w-0', !fill && 'max-w-form-control']}
>
  <Input
    {...rest}
    type={revealed ? 'text' : 'password'}
    class={[
      className,
      'pe-password-end @max-3xl/app-shell:pe-touch-target pointer-coarse:pe-touch-target',
    ]}
  />
  <Button
    type="button"
    variant="ghost"
    size="icon-md"
    disabled={rest.disabled}
    label={t(revealed ? 'passwordInput.hide' : 'passwordInput.show', {
      field: label,
    })}
    aria-controls={rest.id ?? undefined}
    class="absolute end-0 top-0 h-full w-password-end p-0 hover:bg-transparent active:bg-transparent"
    onclick={() => {
      revealed = !revealed;
    }}
  >
    <span
      data-slot="password-visibility-icon"
      aria-hidden="true"
      class="size-password-icon bg-current mask-(--password-icon) mask-contain mask-center mask-no-repeat"
      style:--password-icon={`url("${revealed ? hiddenIcon : visibleIcon}")`}
    ></span>
  </Button>
</div>
