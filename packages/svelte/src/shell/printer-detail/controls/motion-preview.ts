import { getContext, setContext, type Snippet } from 'svelte';

import type { Device, PrinterState } from '../../../domain';

type MotionPreview = Snippet<
  [
    device: Device,
    state: PrinterState,
    commandPanel: Snippet<[Snippet]> | undefined,
  ]
>;

const MOTION_PREVIEW = Symbol('motion preview');

export const getMotionPreview = (): MotionPreview | undefined =>
  getContext<MotionPreview | undefined>(MOTION_PREVIEW);

export const provideMotionPreview = (preview: MotionPreview): void => {
  setContext(MOTION_PREVIEW, preview);
};
