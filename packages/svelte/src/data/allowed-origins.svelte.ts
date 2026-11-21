// The browser origins that may call this server, for the settings page. The
// list is the server's, and a change is shown once the server applied it.
import { type DataFault, faultOf } from './fault';
import {
  type AllowedOrigin,
  type AllowedOriginsClient,
  readAllowedOrigins,
  writeAllowedOrigins,
} from './services/origins';

/** A shared store, so a class (svelte-5.md). */
export class AllowedOrigins {
  readonly #client: AllowedOriginsClient;

  #items = $state.raw<readonly AllowedOrigin[] | undefined>(undefined);
  #saving = $state(false);
  #error = $state<DataFault | undefined>(undefined);
  #flight: Promise<void> | undefined;
  // Counts the resets, so an answer to a request sent before one records
  // nothing.
  #resets = 0;
  // Counts the writes, so a read sent before one cannot draw the list back.
  #writes = 0;

  constructor(client: AllowedOriginsClient) {
    this.#client = client;
  }

  /** Every allowed origin with its source. Empty until the first answer. */
  get items(): readonly AllowedOrigin[] {
    return this.#items ?? [];
  }

  /** True once a read has answered. */
  get loaded(): boolean {
    return this.#items !== undefined;
  }

  /** True while a change is on its way to the server. */
  get saving(): boolean {
    return this.#saving;
  }

  get error(): DataFault | undefined {
    return this.#error;
  }

  /** A second call while one read is in flight is given that read. */
  load(): Promise<void> {
    if (this.#flight) return this.#flight;

    const flight = this.#read().finally(() => {
      if (this.#flight === flight) this.#flight = undefined;
    });
    this.#flight = flight;

    return flight;
  }

  /** Forgets the list, when the person signed in changes. */
  reset(): void {
    this.#resets += 1;
    this.#flight = undefined;
    this.#items = undefined;
    this.#saving = false;
    this.#error = undefined;
  }

  /** Adds one origin to the list kept in Settings. True when it was stored. */
  add(origin: string): Promise<boolean> {
    return this.#write([...this.#stored(), origin]);
  }

  /** Takes one origin out of the list kept in Settings. */
  remove(origin: string): Promise<boolean> {
    return this.#write(this.#stored().filter((held) => held !== origin));
  }

  #stored(): string[] {
    return this.items
      .filter((item) => item.source === 'settings')
      .map((item) => item.origin);
  }

  /**
   * A change asked for while another is on its way is dropped, and a refusal
   * keeps the list the server holds.
   */
  async #write(origins: readonly string[]): Promise<boolean> {
    if (this.#saving) return false;

    const resets = this.#resets;
    this.#writes += 1;
    this.#saving = true;
    this.#error = undefined;
    try {
      const answer = await writeAllowedOrigins(this.#client, origins);
      if (resets === this.#resets) this.#items = answer.items ?? [];

      return true;
    } catch (thrown) {
      if (resets === this.#resets) this.#error = faultOf(thrown);

      return false;
    } finally {
      if (resets === this.#resets) this.#saving = false;
    }
  }

  async #read(): Promise<void> {
    const resets = this.#resets;
    const writes = this.#writes;
    try {
      const answer = await readAllowedOrigins(this.#client);
      if (resets !== this.#resets || writes !== this.#writes) return;
      this.#items = answer.items ?? [];
      this.#error = undefined;
    } catch (thrown) {
      if (resets === this.#resets) this.#error = faultOf(thrown);
    }
  }
}
