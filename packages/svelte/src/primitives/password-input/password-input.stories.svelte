<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PasswordInput from './password-input.svelte';
  const { Story } = defineMeta({
    title: 'Primitives/Password Input',
    component: PasswordInput,
    args: {
      label: 'Password',
      'aria-label': 'Password',
      value: 'example-password',
      autocomplete: 'new-password',
    },
  });
</script>

<Story name="Default" />
<Story name="Disabled" args={{ disabled: true }} />
