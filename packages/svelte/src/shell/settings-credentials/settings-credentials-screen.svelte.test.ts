import { describe, expect, it, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import type { ApiCredential } from '../../data';
import { expectNoViolations } from '../../test/axe';
import SettingsCredentialsScreen from './settings-credentials-screen.svelte';
import type {
  CredentialsSource,
  SettingsCredentialsScreenProps,
} from './types';

const DAY_MS = 24 * 60 * 60 * 1000;

const at = (days: number): string =>
  new Date(Date.now() + days * DAY_MS).toISOString();

/** Nine days left, and presented yesterday. */
const runner: ApiCredential = {
  id: '7Kp2mQ9xLw4nRt8vBc3zYe',
  name: 'CI upload runner',
  subject: 'user:alice',
  scopes: ['operate'],
  origin: 'mint',
  created_at: at(-110),
  expires_at: at(9),
  last_used_at: at(-1),
};

/** Fifty days left, and never presented. */
const scraper: ApiCredential = {
  id: 'Xw3bN8qLp5tR2mKv7cJz1H',
  name: 'Grafana scraper',
  subject: 'user:alice',
  scopes: ['read'],
  origin: 'app_pairing',
  created_at: at(-40),
  expires_at: at(50),
};

const items: ApiCredential[] = [runner, scraper];

const source = (over: Partial<CredentialsSource> = {}): CredentialsSource => ({
  items,
  loading: false,
  loaded: true,
  error: undefined,
  minted: undefined,
  ...over,
});

/** What every case hands in, so a case names only what it is about. */
const screen = (
  over: Partial<SettingsCredentialsScreenProps> = {},
): SettingsCredentialsScreenProps => ({
  credentials: source(),
  onCreate: () => undefined,
  onDismiss: () => undefined,
  onRevoke: () => undefined,
  pairHref: '/pair',
  ...over,
});

describe('SettingsCredentialsScreen', () => {
  it('names the room and lists what can reach this instance', async () => {
    await render(SettingsCredentialsScreen, { props: screen() });

    await expect
      .element(page.getByRole('heading', { level: 2 }))
      .toHaveTextContent('API credentials');
    // Both width tiers sit in the DOM and the container query paints one, so
    // a value matches twice and which of the two the harness leaves visible
    // is its own width's business. What the row has to prove is that the
    // value was drawn at all.
    await expect
      .element(page.getByText('CI upload runner').first())
      .toBeInTheDocument();
    await expect
      .element(page.getByText('Grafana scraper').first())
      .toBeInTheDocument();
  });

  it('marks a credential inside a fortnight of its expiry', async () => {
    await render(SettingsCredentialsScreen, {
      props: screen({ credentials: source({ items: [runner] }) }),
    });

    await expect
      .element(page.getByText('Expires soon').first())
      .toBeInTheDocument();
  });

  it('leaves a credential with months left unmarked', async () => {
    await render(SettingsCredentialsScreen, {
      props: screen({ credentials: source({ items: [scraper] }) }),
    });

    expect(page.getByText('Expires soon').elements()).toHaveLength(0);
  });

  it('says a credential has never been presented', async () => {
    await render(SettingsCredentialsScreen, { props: screen() });

    await expect
      .element(page.getByText('Never used').first())
      .toBeInTheDocument();
  });

  const TOKEN = 'pcl_9f3b21c7ae4d05e8b6f14a72c3d09e5b';

  const minted = {
    id: 'Lm5cF8jR3wP7yH1tV6zQaB',
    name: 'Nightly export',
    subject: 'user:alice',
    created_at: at(0),
    expires_at: at(90),
    scopes: ['read'] as ApiCredential['scopes'],
    origin: 'mint' as const,
    token: TOKEN,
  };

  const showing = () =>
    render(SettingsCredentialsScreen, {
      props: screen({ credentials: source({ minted }) }),
    });

  it('hands the token over in a dialog, masked, and says it is shown once', async () => {
    await showing();

    await expect
      .element(page.getByRole('dialog', { name: 'Copy this credential now' }))
      .toBeVisible();
    await expect
      .element(
        page.getByText('This is the only time it is shown.', { exact: false }),
      )
      .toBeVisible();
    const field = page.getByLabelText('New credential', { exact: true });
    await expect.element(field).toHaveAttribute('type', 'password');

    await page.getByRole('button', { name: 'Show New credential' }).click();

    await expect.element(field).toHaveAttribute('type', 'text');
    await expect.element(field).toHaveValue(TOKEN);
  });

  it('copies the token while it is masked', async () => {
    const writeText = vi
      .spyOn(navigator.clipboard, 'writeText')
      .mockResolvedValue(undefined);
    await showing();

    await page.getByRole('button', { name: 'Copy' }).click();

    expect(writeText).toHaveBeenCalledWith(TOKEN);
    writeText.mockRestore();
  });

  it('drops the token when the reader is done', async () => {
    const onDismiss = vi.fn();
    const { rerender } = await render(SettingsCredentialsScreen, {
      props: screen({ onDismiss, credentials: source({ minted }) }),
    });

    await page.getByRole('button', { name: 'Done' }).click();
    expect(onDismiss).toHaveBeenCalledTimes(1);

    // The store answers a dismissal by forgetting the token.
    await rerender(screen({ onDismiss, credentials: source() }));

    await expect.poll(() => document.body.textContent).not.toContain(TOKEN);
    await expect
      .poll(() => document.querySelector('[role="dialog"]'))
      .toBeNull();
  });

  it('drops the token when the reader leaves the screen', async () => {
    const onDismiss = vi.fn();
    const { unmount } = await render(SettingsCredentialsScreen, {
      props: screen({ onDismiss }),
    });

    expect(onDismiss).not.toHaveBeenCalled();
    await unmount();

    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  it('names what a new credential is for before creating it', async () => {
    const onCreate = vi.fn();
    await render(SettingsCredentialsScreen, { props: screen({ onCreate }) });

    await page.getByRole('button', { name: 'New credential' }).click();
    await expect.element(page.getByText('New API credential')).toBeVisible();
    await page.getByRole('textbox', { name: 'Name' }).fill('Nightly export');
    await page.getByRole('button', { name: 'Create credential' }).click();

    expect(onCreate).toHaveBeenCalledWith('Nightly export', 90, 'read');
  });

  it('sends the chosen lifetime and refuses one past the maximum', async () => {
    const onCreate = vi.fn();
    await render(SettingsCredentialsScreen, { props: screen({ onCreate }) });

    await page.getByRole('button', { name: 'New credential' }).click();
    await page.getByRole('textbox', { name: 'Name' }).fill('Nightly export');
    const days = page.getByRole('spinbutton', { name: 'Valid for (days)' });
    await days.fill('366');
    // The button stays pressable, because a disabled one is the one state
    // that cannot say what is wrong with the value.
    await expect
      .element(page.getByRole('button', { name: 'Create credential' }))
      .toBeEnabled();
    await expect
      .element(page.getByText('Enter a whole number of days from 1 to 365.'))
      .toBeVisible();
    await page.getByRole('button', { name: 'Create credential' }).click();
    expect(onCreate).not.toHaveBeenCalled();

    await days.fill('30');
    await page.getByRole('button', { name: 'Create credential' }).click();

    expect(onCreate).toHaveBeenCalledWith('Nightly export', 30, 'read');
  });

  // 0 and a word are the other two the field has to answer for. A number
  // control holds no word at all, so a typed `abc` reaches the handler as the
  // empty value, which is what the second case is.
  it.each(['0', ''])('names the range for %s', async (typed) => {
    const onCreate = vi.fn();
    await render(SettingsCredentialsScreen, { props: screen({ onCreate }) });

    await page.getByRole('button', { name: 'New credential' }).click();
    await page.getByRole('textbox', { name: 'Name' }).fill('Nightly export');
    await page
      .getByRole('spinbutton', { name: 'Valid for (days)' })
      .fill(typed);

    await expect
      .element(page.getByText('Enter a whole number of days from 1 to 365.'))
      .toBeVisible();

    await page.getByRole('button', { name: 'Create credential' }).click();
    expect(onCreate).not.toHaveBeenCalled();
  });

  // A reader who types an illegal value and leaves comes back to an empty
  // form. Clearing the refusals and keeping the values put 366 back on screen
  // with nothing saying what was wrong with it, and the browser's own
  // validation then refused the press without a word of ours.
  it('empties the form when the reader leaves without creating one', async () => {
    await render(SettingsCredentialsScreen, { props: screen() });

    await page.getByRole('button', { name: 'New credential' }).click();
    await page.getByRole('textbox', { name: 'Name' }).fill('Nightly export');
    await page
      .getByRole('spinbutton', { name: 'Valid for (days)' })
      .fill('366');
    await expect
      .element(page.getByText('Enter a whole number of days from 1 to 365.'))
      .toBeVisible();

    await userEvent.keyboard('{Escape}');
    await expect
      .element(page.getByText('New API credential'))
      .not.toBeInTheDocument();

    await page.getByRole('button', { name: 'New credential' }).click();

    await expect
      .element(page.getByRole('spinbutton', { name: 'Valid for (days)' }))
      .toHaveValue(90);
    await expect
      .element(page.getByRole('textbox', { name: 'Name' }))
      .toHaveValue('');
    expect(
      page.getByText('Enter a whole number of days from 1 to 365.').elements(),
    ).toHaveLength(0);
  });

  it('says the range the field takes before anything is typed', async () => {
    await render(SettingsCredentialsScreen, { props: screen() });

    await page.getByRole('button', { name: 'New credential' }).click();

    await expect
      .element(
        page.getByText(
          'The credential stops working after this many days. Anything from 1 to 365.',
        ),
      )
      .toBeVisible();
  });

  it('asks for a name rather than refusing the press without a word', async () => {
    const onCreate = vi.fn();
    await render(SettingsCredentialsScreen, { props: screen({ onCreate }) });

    await page.getByRole('button', { name: 'New credential' }).click();
    await page.getByRole('button', { name: 'Create credential' }).click();

    await expect
      .element(page.getByText('Give the credential a name', { exact: false }))
      .toBeVisible();
    expect(onCreate).not.toHaveBeenCalled();
  });

  // Two refusals at once. Both have to be reachable from one press: the form
  // turns the browser's own validation off, because with it on an
  // out-of-range day count refused the submit in the browser's bubble and the
  // missing name was never mentioned.
  //
  // A panel with no cap also grows past the bottom of a short screen and
  // takes the button a person came to press with it, so the measurement is
  // taken with both lines drawn.
  it('draws both refusals from one press, and keeps the button on the screen', async () => {
    const onCreate = vi.fn();
    await render(SettingsCredentialsScreen, { props: screen({ onCreate }) });

    await page.getByRole('button', { name: 'New credential' }).click();
    await page
      .getByRole('spinbutton', { name: 'Valid for (days)' })
      .fill('366');
    await page.getByRole('button', { name: 'Create credential' }).click();

    expect(onCreate).not.toHaveBeenCalled();
    await expect
      .element(page.getByText('Give the credential a name', { exact: false }))
      .toBeVisible();
    await expect
      .element(page.getByText('Enter a whole number of days from 1 to 365.'))
      .toBeVisible();

    const button = page
      .getByRole('button', { name: 'Create credential' })
      .element();
    const box = button.getBoundingClientRect();

    expect(box.bottom).toBeLessThanOrEqual(window.innerHeight);
    expect(box.top).toBeGreaterThanOrEqual(0);
  });

  // A credential reads unless the person chose more, and the choice is sent.
  it('sends the scope the person chose', async () => {
    const onCreate = vi.fn();
    await render(SettingsCredentialsScreen, { props: screen({ onCreate }) });

    await page.getByRole('button', { name: 'New credential' }).click();
    await page.getByRole('textbox', { name: 'Name' }).fill('Farm runner');
    await page.getByRole('radio', { name: 'Operate' }).click();
    await page.getByRole('button', { name: 'Create credential' }).click();

    expect(onCreate).toHaveBeenCalledWith('Farm runner', 90, 'operate');
  });

  // A person tells a paired app from a token they copied, and reads what
  // each may do.
  it('shows the scope of each credential and marks a paired app', async () => {
    const { container } = await render(SettingsCredentialsScreen, {
      props: screen(),
    });

    await expect
      .element(page.getByRole('link', { name: 'Pair an app' }))
      .toHaveAttribute('href', '/pair');
    // The table and the stacked rows are both drawn, and the width picks one.
    expect(container.textContent).toContain('Operate');
    expect(container.textContent).toContain('Paired app');
  });

  it('asks before a revocation, because nothing brings one back', async () => {
    const onRevoke = vi.fn();
    await render(SettingsCredentialsScreen, { props: screen({ onRevoke }) });

    await page
      .getByRole('button', { name: 'Actions for CI upload runner' })
      .click();
    await page.getByRole('menuitem', { name: 'Revoke' }).click();

    await expect
      .element(page.getByText('Revoke CI upload runner?'))
      .toBeVisible();
    expect(onRevoke).not.toHaveBeenCalled();

    await page.getByRole('button', { name: 'Revoke' }).click();

    expect(onRevoke).toHaveBeenCalledWith(runner);
  });

  it('draws a listing shaped skeleton while the read is in flight', async () => {
    await render(SettingsCredentialsScreen, {
      props: screen({
        credentials: source({ items: [], loaded: false, loading: true }),
      }),
    });

    await expect
      .element(
        page.getByRole('status', { name: 'Loading the API credentials' }),
      )
      .toBeVisible();
    expect(page.getByText('No credentials yet').elements()).toHaveLength(0);
  });

  it('says the listing is empty rather than unread', async () => {
    await render(SettingsCredentialsScreen, {
      props: screen({ credentials: source({ items: [] }) }),
    });

    await expect.element(page.getByText('No credentials yet')).toBeVisible();
  });

  it('draws a refusal through the fault registry and waits for nothing', async () => {
    await render(SettingsCredentialsScreen, {
      props: screen({
        credentials: source({
          items: [],
          loaded: false,
          error: { code: 'core.auth/ACCESS_DENIED', status: 403 },
        }),
      }),
    });

    expect(
      page
        .getByRole('status', { name: 'Loading the API credentials' })
        .elements(),
    ).toHaveLength(0);
    await expect.element(page.getByRole('alert')).toBeVisible();
  });

  it('has nothing axe refuses', async () => {
    const { container } = await render(SettingsCredentialsScreen, {
      props: screen(),
    });

    await expectNoViolations(container);
  });
});
