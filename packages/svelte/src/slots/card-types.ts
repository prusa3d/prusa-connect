// The contract of a dashboard card (ADR-20261008-sww rules 15 to 23): what a module
// declares for a card, what the card's component receives, and what an
// editor of the module's own receives.
//
// A module declares a card under `contributes.cards` in its manifest. The
// core checks the declaration at install and publishes it on the listing, so
// the page can draw the card picker and the settings form before any bundle
// loads. The bundle registers the component with `host.register.card`.
import type { Component } from 'svelte';

/** A size in cells of a 12-column grid. */
export interface CardSize {
  readonly w: number;
  readonly h: number;
}

/**
 * A card's settings, as the page hands them to the card: an object with one
 * value for every field of the card's schema, each checked against it.
 */
export type CardSettings = Readonly<Record<string, unknown>>;

/** One value of a choice or choices field. */
export interface SettingOption {
  readonly value: string;
  readonly label: string;
}

/** What every field of a card's settings carries. */
interface SettingBase {
  /** The name the value is kept under in the card's settings. */
  readonly key: string;
  /** What the form shows beside the control. */
  readonly label: string;
}

/**
 * One field of a card's settings schema. The kinds are a closed set
 * (ADR-20261008-sww rule 18), so the form has a control for every field any module
 * declares.
 */
export type CardSetting =
  | (SettingBase & {
      /** One printer, one tag or one camera, by its id or name. */
      readonly kind: 'printer' | 'tag' | 'camera';
      readonly default: string;
    })
  | (SettingBase & {
      /** A list of printers, by their ids. */
      readonly kind: 'printers';
      readonly default: readonly string[];
    })
  | (SettingBase & {
      /** One value out of the options. */
      readonly kind: 'choice';
      readonly options: readonly SettingOption[];
      readonly default: string;
    })
  | (SettingBase & {
      /** Any of the options, as a list. */
      readonly kind: 'choices';
      readonly options: readonly SettingOption[];
      readonly default: readonly string[];
      /**
       * Also keeps a value no option names, such as the id of a reading one
       * printer describes. Only a core card sets it: a listing never does.
       */
      readonly open?: boolean | undefined;
    })
  | (SettingBase & {
      /** A number between min and max, in steps of step. */
      readonly kind: 'number';
      readonly min?: number | undefined;
      readonly max?: number | undefined;
      readonly step?: number | undefined;
      readonly default: number;
    })
  | (SettingBase & {
      /** One line of text, or several lines for a note. */
      readonly kind: 'text' | 'note';
      /** The longest the value may be, counted as UTF-8 bytes. */
      readonly max_bytes: number;
      readonly default: string;
    })
  | (SettingBase & {
      /** On or off. */
      readonly kind: 'switch';
      readonly default: boolean;
    });

/** Every kind a card setting may be. */
export type SettingKind = CardSetting['kind'];

/** What a module declares for one card, as the listing publishes it. */
export interface CardDeclaration {
  /** The card's name within its module, one DNS label. */
  readonly id: string;
  /** What the card picker and the card's default heading read. */
  readonly title: string;
  /** One sentence the card picker shows under the title. */
  readonly description: string;
  /** An icon the library publishes, and undefined for the default. */
  readonly icon?: string | undefined;
  /** The size a new card starts at. */
  readonly size: CardSize;
  /** The smallest the card may be made. */
  readonly least: CardSize;
  /** The schema of the card's settings. */
  readonly settings: readonly CardSetting[];
}

/**
 * What a card's component receives, and nothing else (ADR-20261008-sww rule 21).
 * Everything else, the fleet among it, the card reads from the host it was
 * registered with. It opens no stream of its own.
 */
export interface CardProps {
  /** The card's settings, every value checked against its schema. */
  readonly config: CardSettings;
  /**
   * The heading a person wrote over the card, or '' when they wrote none. A
   * card draws it, and draws a heading of its own when it is empty, which it
   * may work out from its settings, such as "Temperatures of MK4 by the door"
   * or "9 printers".
   */
  readonly title: string;
  /** The card's size on the grid, in cells. */
  readonly size: CardSize;
  /** True while the dashboard is being changed. A card draws no link then. */
  readonly editing: boolean;
}

/**
 * What a card's own editor receives (ADR-20261008-sww rule 20). The page checks what
 * the editor hands back against the card's schema before it keeps it.
 */
export interface CardEditorProps {
  readonly config: CardSettings;
  readonly onchange: (next: CardSettings) => void;
}

/** What a module may pass beside a card's component. */
export interface CardOptions {
  /** An editor for settings a schema cannot express. */
  readonly editor?: Component<CardEditorProps> | undefined;
}

/** A card the host admitted, with the type it is placed on a dashboard by. */
export interface RegisteredCard {
  /** The namespace, a slash and the card's id, such as `core/printer`. */
  readonly type: string;
  readonly namespace: string;
  readonly declaration: CardDeclaration;
  readonly component: Component<CardProps>;
  readonly editor?: Component<CardEditorProps> | undefined;
}

/** A card a module declared, before its bundle registers anything. */
export interface DeclaredCard {
  readonly type: string;
  readonly namespace: string;
  readonly declaration: CardDeclaration;
}

/** The namespace the core registers its own cards under (ADR-20261008-sww rule 17). */
export const CORE_CARD_NAMESPACE = 'core';
