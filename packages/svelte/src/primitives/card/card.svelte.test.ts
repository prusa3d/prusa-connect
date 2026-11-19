import { afterEach, describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import CardProbe from '../../test/card-probe.svelte';
import { expectNoViolations } from '../../test/axe';

interface Props {
  title: string;
  description: string;
  body: string;
  footer: string;
  action?: string;
  landmark?: boolean;
  tone?: 'default' | 'critical' | 'attention';
}

const words: Props = {
  title: 'MK4S',
  description: 'Ready to print',
  body: 'Last job finished 12 minutes ago.',
  footer: 'Print',
};

const hosts: HTMLElement[] = [];

/**
 * A card of a known width. The padding step reads the card's own container and
 * not the viewport, so the width of the box around it is what the test sets.
 */
const drawIn = async (
  width: string,
  props: Props = words,
): Promise<Element> => {
  const host = document.createElement('div');
  host.style.inlineSize = width;
  document.body.append(host);
  hosts.push(host);

  await render(CardProbe, { target: host, props });

  const card = host.querySelector('[data-slot="card"]');
  if (card === null) throw new Error('no card rendered');
  return card;
};

const slot = (card: Element, name: string): HTMLElement => {
  const part = card.querySelector<HTMLElement>(`[data-slot="${name}"]`);
  if (part === null) throw new Error(`no ${name} rendered`);
  return part;
};

afterEach(() => {
  for (const host of hosts.splice(0)) host.remove();
});

describe('Card', () => {
  // Header, body and footer share one inset, so their edges line up at both
  // steps of the padding.
  it.each(['320px', '480px'])('pads every part alike at %s', async (width) => {
    const card = await drawIn(width);
    const inset = (name: string): string =>
      getComputedStyle(slot(card, name)).paddingLeft;

    expect(inset('card-header')).toBe(inset('card-content'));
    expect(inset('card-footer')).toBe(inset('card-content'));
  });

  // The step keys off the card and not the viewport, so a narrow card on a
  // wide screen pads like a narrow card.
  it('reads its own width and not the window', async () => {
    const narrow = await drawIn('320px');
    const wide = await drawIn('480px');

    const inset = (card: Element): number =>
      parseFloat(getComputedStyle(slot(card, 'card-content')).paddingLeft);

    expect(inset(narrow)).toBeLessThan(inset(wide));
  });

  // A two-column split in a narrow header left the title a 40px column and
  // wrapped its word one letter per line.
  it('drops the action under the title in a narrow header', async () => {
    const card = await drawIn('320px', { ...words, action: 'Online' });

    const title = slot(card, 'card-title').getBoundingClientRect();
    const action = slot(card, 'card-action').getBoundingClientRect();

    expect(action.top).toBeGreaterThanOrEqual(title.bottom);
    expect(action.left).toBeCloseTo(title.left, 0);
  });

  it('sets the action beside the title in a wide header', async () => {
    const card = await drawIn('480px', { ...words, action: 'Online' });

    const title = slot(card, 'card-title').getBoundingClientRect();
    const action = slot(card, 'card-action').getBoundingClientRect();

    expect(action.left).toBeGreaterThan(title.right);
    expect(action.top).toBeCloseTo(title.top, 0);
    // The action ends at the header's right edge, not beside the title.
    const header = slot(card, 'card-header');
    expect(action.right).toBeCloseTo(
      header.getBoundingClientRect().right -
        parseFloat(getComputedStyle(header).paddingRight),
      0,
    );
  });

  // The card carries no role of its own. A caller that makes it a landmark
  // names it, and the name is the title.
  it('becomes a named landmark when the caller asks for one', async () => {
    await drawIn('480px', { ...words, landmark: true });

    await expect
      .element(page.getByRole('region', { name: words.title }))
      .toBeInTheDocument();
  });

  it('has no accessibility violation', async () => {
    const card = await drawIn('480px', {
      ...words,
      action: 'Online',
      landmark: true,
    });

    await expectNoViolations(card);
  });
});

describe('Card tone', () => {
  it('tints the border of a critical card and leaves a default one', async () => {
    await render(CardProbe, { props: { ...words, tone: 'critical' } });
    await render(CardProbe, { props: { ...words } });
    const [critical, plain] = [
      ...document.querySelectorAll<HTMLElement>('[data-slot="card"]'),
    ].map((card) => getComputedStyle(card).borderColor);
    expect(critical).not.toBe(plain);
  });
});
