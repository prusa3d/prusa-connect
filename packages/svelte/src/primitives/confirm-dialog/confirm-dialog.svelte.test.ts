import { describe, expect, it } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import ConfirmDialogProbe from '../../test/confirm-dialog-probe.svelte';

/** The panel bits-ui portals to the end of the document. */
const panelOf = (): HTMLElement => {
  const found = document.body.querySelector<HTMLElement>(
    '[role="alertdialog"]',
  );
  if (found === null) throw new Error('no panel opened');
  return found;
};

const openIt = async (): Promise<HTMLElement> => {
  await page.getByRole('button', { name: 'Delete' }).click();
  return panelOf();
};

const inPanel = (panel: HTMLElement, name: string): HTMLButtonElement => {
  const found = [...panel.querySelectorAll('button')].find(
    (button) => button.textContent.trim() === name,
  );
  if (found === undefined) throw new Error(`no ${name} button`);
  return found;
};

describe('ConfirmDialog', () => {
  it('gives both footer buttons the same height', async () => {
    await render(ConfirmDialogProbe);
    const panel = await openIt();

    const cancel = inPanel(panel, 'Cancel');
    const confirm = inPanel(panel, 'Delete');

    // The one thing the hand-built copies disagreed about, four at 44px and
    // five at 36. 44px is the floor for a finger on a wall tablet.
    expect(parseFloat(getComputedStyle(cancel).height)).toBeGreaterThanOrEqual(
      44,
    );
    expect(getComputedStyle(confirm).height).toBe(
      getComputedStyle(cancel).height,
    );
  });

  it('confirms with the red button unless the act loses nothing', async () => {
    await render(ConfirmDialogProbe);
    const destructive = await openIt();
    expect(inPanel(destructive, 'Delete').dataset.variant).toBe('destructive');
  });

  it('confirms an act that loses nothing with the primary button', async () => {
    await render(ConfirmDialogProbe, { props: { variant: 'confirm' } });
    const panel = await openIt();

    expect(inPanel(panel, 'Delete').dataset.variant).toBe('default');
  });

  it('calls back once when the act is confirmed', async () => {
    let confirms = 0;
    await render(ConfirmDialogProbe, {
      props: {
        onconfirm: () => {
          confirms += 1;
        },
      },
    });
    const panel = await openIt();

    inPanel(panel, 'Delete').click();

    expect(confirms).toBe(1);
    // Nothing outside holds a panel its own trigger opened, so it shuts. Reset
    // printer stayed up after the printer had restarted (CL-252).
    await expect
      .poll(() => document.body.querySelector('[role="alertdialog"]'))
      .toBeNull();
  });

  it('leaves a panel the screen holds open up when the act is confirmed', async () => {
    await render(ConfirmDialogProbe, {
      props: { open: true, withTrigger: false },
    });

    inPanel(panelOf(), 'Delete').click();

    // A slow act needs somewhere to say so, and the screen shuts it.
    expect(document.body.querySelector('[role="alertdialog"]')).not.toBeNull();
  });

  it('holds the act back while it is disabled', async () => {
    let confirms = 0;
    await render(ConfirmDialogProbe, {
      props: {
        confirmDisabled: true,
        onconfirm: () => {
          confirms += 1;
        },
      },
    });
    const panel = await openIt();

    const confirm = inPanel(panel, 'Delete');
    expect(confirm.disabled).toBe(true);
    confirm.click();

    expect(confirms).toBe(0);
  });

  it('does nothing at all when the answer is no', async () => {
    let confirms = 0;
    await render(ConfirmDialogProbe, {
      props: {
        onconfirm: () => {
          confirms += 1;
        },
      },
    });
    const panel = await openIt();

    inPanel(panel, 'Cancel').click();

    expect(confirms).toBe(0);
    await expect
      .poll(() => document.body.querySelector('[role="alertdialog"]'))
      .toBeNull();
  });

  it('takes the focus and hands it back to the trigger', async () => {
    await render(ConfirmDialogProbe);
    const panel = await openIt();

    // bits-ui focuses the panel itself rather than the first control, which is
    // what tells a screen reader the question before the answers.
    await expect.poll(() => panel.contains(document.activeElement)).toBe(true);

    await userEvent.keyboard('{Escape}');

    await expect
      .poll(() => document.activeElement?.textContent.trim())
      .toBe('Delete');
  });

  it('opens with no trigger when the screen holds it open', async () => {
    await render(ConfirmDialogProbe, {
      props: { open: true, withTrigger: false },
    });

    const panel = panelOf();

    expect(panel.textContent).toContain('This cannot be undone.');
    expect(page.getByRole('button', { name: 'Delete' })).toBeDefined();
  });

  it('keeps a long file name and the buttons inside the panel', async () => {
    // The name from the report: one word, wider than the panel.
    await render(ConfirmDialogProbe, {
      props: {
        open: true,
        withTrigger: false,
        title:
          'Delete 3dbenchy(3)_0.4n_0.2mm_PETG_COREONEINDX_51m_with_a_much_longer_suffix.bgcode?',
      },
    });
    const panel = panelOf();
    const edge = panel.getBoundingClientRect();
    const title = panel.querySelector('[data-slot="alert-dialog-title"]');
    if (title === null) throw new Error('no title');

    for (const part of [
      title,
      inPanel(panel, 'Cancel'),
      inPanel(panel, 'Delete'),
    ]) {
      expect(part.getBoundingClientRect().right).toBeLessThanOrEqual(
        edge.right,
      );
    }
  });

  it('has nothing axe objects to', async () => {
    await render(ConfirmDialogProbe);
    const panel = await openIt();

    await expectNoViolations(panel);
  });
});
