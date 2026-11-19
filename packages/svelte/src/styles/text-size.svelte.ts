import {
  browserStorage,
  readRemembered,
  type RememberedStorage,
  writeRemembered,
} from '../remembered';

export const TEXT_SIZES = ['small', 'normal', 'large'] as const;
export type TextSize = (typeof TEXT_SIZES)[number];
export const TEXT_SIZE_STORAGE_KEY = 'text-size';

const isTextSize = (value: unknown): value is TextSize =>
  typeof value === 'string' &&
  (TEXT_SIZES as readonly string[]).includes(value);

/** Only tokens.css reads this attribute. */
export const applyTextSize = (value: TextSize): void => {
  if (typeof document === 'undefined') return;
  document.documentElement.dataset.textSize = value;
};

/** The browser's text preference, shared by every screen. */
export class TextSizeStore {
  #preference = $state<TextSize>('normal');
  readonly #storage: RememberedStorage | null;

  constructor({
    storage = browserStorage(),
  }: { storage?: RememberedStorage | null } = {}) {
    this.#storage = storage;
    const held = readRemembered(storage, TEXT_SIZE_STORAGE_KEY);
    this.#preference = isTextSize(held) ? held : 'normal';
    applyTextSize(this.#preference);
  }

  get preference(): TextSize {
    return this.#preference;
  }

  set preference(next: TextSize) {
    if (!isTextSize(next)) return;
    this.#preference = next;
    applyTextSize(next);
    writeRemembered(this.#storage, TEXT_SIZE_STORAGE_KEY, next);
  }
}

export const textSize = new TextSizeStore();
