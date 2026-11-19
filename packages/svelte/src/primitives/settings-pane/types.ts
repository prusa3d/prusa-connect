import type { Snippet } from 'svelte';
import type { ClassValue } from 'svelte/elements';

export interface SettingsPaneProps {
  /** The name of the section: "People", "Fractal". */
  title: string;
  /** One line saying what the section changes. */
  description?: string | undefined;
  /** The rows: form controls, a table, a list. */
  children?: Snippet;
  /** The submit row. It sits in a tinted band at the foot of the pane. */
  actions?: Snippet | undefined;
  /** Short text at the end of the header band, beside the title. */
  headerEnd?: Snippet | undefined;
  /** A message about the last save, read out when it arrives. */
  message?: string | undefined;
  /** The message is a failure, so it is announced rather than queued. */
  messageIsFailure?: boolean;
  /** Draw a line around the pane, where its ground is painted the same colour. */
  edged?: boolean;
  class?: ClassValue | undefined;
}

export interface SettingsRowProps {
  /** A help control beside the label. */
  labelHelp?: Snippet | undefined;
  /** The name of the setting. */
  label: string;
  /** What it does, in one sentence. */
  description?: string | undefined;
  /** The control the label names. It must carry its own label or `id`. */
  control?: Snippet;
  /**
   * Gives the label the id `<id>-label` and the description `<id>-description`,
   * so a control can name itself by the label and be described by the line.
   */
  id?: string | undefined;
  /**
   * The columns on a wide pane. `form` gives the label a fixed column and the
   * control the rest; `switch` lets the label take the room and puts the
   * control at the end.
   */
  layout?: 'form' | 'switch' | undefined;
  class?: ClassValue | undefined;
}
