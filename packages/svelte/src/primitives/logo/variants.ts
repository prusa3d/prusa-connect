import type { LogoSize } from './types';

// Static strings chosen by key (svelte-5.md). Tailwind reads this file, so a
// utility written here is emitted; one assembled at run time is not.

// `w-fit` is load-bearing rather than tidiness: as a flex or grid item this
// root would otherwise stretch to its parent, and the mark would no longer be
// what sets the lockup's width.
export const base = 'inline-flex w-fit min-w-0 flex-col leading-none';

/**
 * One ratio serves all three sizes, because a width match is a proportion and
 * a proportion does not care how big the thing is. `lg` is `--text-2xl` times
 * 1.6, so whatever changes the type scale carries the wordmark with it.
 */
export const sizes: Record<LogoSize, string> = {
  sm: 'text-lg',
  md: 'text-2xl',
  lg: 'text-logo-lg',
};
