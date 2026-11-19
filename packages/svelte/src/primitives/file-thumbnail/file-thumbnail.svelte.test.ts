import FolderIcon from '@lucide/svelte/icons/folder';
import { describe, expect, it, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import FileThumbnail from './file-thumbnail.svelte';
import { takeThumbnailSlot } from './slots';
import type { FileThumbnailProps } from './types';

const plate =
  'data:image/svg+xml;utf8,' +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8 8">' +
      '<rect width="8" height="8" fill="orange"/></svg>',
  );

const name = 'Bracket_Left_Reinforced_v4_FINAL.gcode';

const draw = async (
  props: Pick<FileThumbnailProps, 'thumbnailUrl'> = {},
): Promise<HTMLElement> => {
  const { container } = await render(FileThumbnail, {
    props: { name, glyph: FolderIcon, thumbnailUrl: plate, ...props },
  });

  return container;
};

/** The preview surface, which bits-ui portals to the end of the document. */
const preview = (): HTMLElement | null =>
  document.body.querySelector<HTMLElement>('[data-slot="popover-content"]');

/**
 * Long enough for the preview to be portalled, so a case that asserts the
 * preview is absent has waited as long as one that finds it present.
 */
const settle = async (): Promise<void> => {
  for (let frame = 0; frame < 10; frame += 1) {
    await new Promise<void>((resolve) => {
      requestAnimationFrame(() => {
        resolve();
      });
    });
  }
};

const box = (container: HTMLElement): HTMLElement => {
  const found = container.querySelector<HTMLElement>(
    '[data-slot="file-thumbnail"]',
  );
  if (found === null) throw new Error('no thumbnail rendered');
  return found;
};

describe('FileThumbnail', () => {
  it('is inert with no preview to show', async () => {
    const container = await draw({ thumbnailUrl: undefined });

    expect(box(container).tagName).toBe('DIV');
    expect(container.querySelector('img')).toBeNull();
    expect(container.querySelector('svg')).not.toBeNull();
    await expectNoViolations(container);
  });

  it('opens the preview on hover and shuts it when the pointer leaves', async () => {
    const container = await draw();

    await userEvent.hover(box(container));
    await expect.poll(preview).not.toBeNull();
    expect(preview()?.textContent).toContain(name);

    await userEvent.unhover(box(container));
    await expect.poll(preview).toBeNull();
  });

  // A print dialog opens under the cursor that clicked the row's print icon,
  // and the browser dispatches pointerenter for the thumbnail that lands
  // there. Nobody pointed at it, so nothing opens over the dialog.
  //
  // The hover at the end is what makes the assertion above it mean something:
  // the preview is portalled on a later frame, so "it is not there yet" and
  // "it is never coming" look the same until a case that does open it has
  // waited the same way and found it.
  it('stays shut for a box that arrives under a pointer standing still', async () => {
    const container = await draw();

    box(container).dispatchEvent(
      new PointerEvent('pointerenter', { bubbles: false }),
    );
    box(container).dispatchEvent(
      new PointerEvent('pointerover', { bubbles: true }),
    );
    await settle();

    expect(preview()).toBeNull();

    await userEvent.hover(box(container));
    await settle();
    expect(preview()).not.toBeNull();
  });

  // Escape shuts the preview with the pointer still on the box, and the box
  // goes on receiving moves there, so a dismissal has to outlast the next
  // twitch of the hand. Leaving the box is what makes the next arrival fresh.
  it('stays shut after Escape until the pointer leaves the box', async () => {
    const container = await draw();

    await userEvent.hover(box(container));
    await settle();
    expect(preview()).not.toBeNull();

    await userEvent.keyboard('{Escape}');
    await settle();
    expect(preview()).toBeNull();

    box(container).dispatchEvent(
      new PointerEvent('pointermove', { bubbles: true }),
    );
    await settle();
    expect(preview()).toBeNull();

    await userEvent.unhover(box(container));
    await userEvent.hover(box(container));
    await settle();
    expect(preview()).not.toBeNull();
  });

  // The dismissal holds back hover alone. A press on the box, by mouse or by
  // Enter on the focused trigger, is somebody asking to see the picture.
  it('opens on a press after Escape with the pointer still on the box', async () => {
    const container = await draw();

    await userEvent.hover(box(container));
    await settle();
    await userEvent.keyboard('{Escape}');
    await settle();
    expect(preview()).toBeNull();

    await userEvent.click(box(container));
    await settle();
    expect(preview()).not.toBeNull();
  });

  it('opens on keyboard focus, and shuts when focus leaves', async () => {
    const container = await draw();

    await userEvent.tab();
    expect(document.activeElement).toBe(box(container));
    await expect.poll(preview).not.toBeNull();

    await userEvent.tab();
    await expect.poll(preview).toBeNull();
  });

  // A dialog moves focus onto its first control as it opens, and this box is
  // often that control. A preview nobody pointed at would cover the dialog.
  it('stays shut when focus is moved by script after a press', async () => {
    const container = await draw();
    const elsewhere = document.createElement('button');
    elsewhere.textContent = 'elsewhere';
    document.body.append(elsewhere);

    await page.elementLocator(elsewhere).click();
    box(container).focus();
    await new Promise((resolve) => setTimeout(resolve, 50));

    expect(document.activeElement).toBe(box(container));
    expect(preview()).toBeNull();
    elsewhere.remove();
  });

  it('never shuts what the same press opened', async () => {
    const container = await draw();

    // A touch browser dispatches pointerenter and focus before the press, so a
    // toggling trigger would read the press as "already open".
    await page.elementLocator(box(container)).click();

    await expect.poll(preview).not.toBeNull();
  });

  it('names the preview for a reader who cannot see it', async () => {
    const container = await draw();

    expect(box(container).getAttribute('aria-label')).toBe(
      `Preview of ${name}`,
    );

    await userEvent.hover(box(container));
    await expect.poll(preview).not.toBeNull();
    // The image is decoration: the caption below it carries the name.
    const image = preview()?.querySelector('img');
    expect(image?.getAttribute('alt')).toBe('');

    const surface = preview();
    if (surface !== null) await expectNoViolations(surface);
  });

  it('falls back to the glyph when the picture will not load', async () => {
    // The server answers 404 for a file that carries no picture, and nothing
    // in a listing says which files those are.
    const container = await draw({ thumbnailUrl: 'data:image/png;base64,zz' });

    await expect.poll(() => box(container).tagName).toBe('DIV');
    expect(container.querySelector('svg')).not.toBeNull();
    expect(container.querySelector('img')).toBeNull();
  });

  // Both slots of the page's queue are held here, as two pictures from a busy
  // printer would hold them.
  const holdBothSlots = (): (() => void)[] => [
    takeThumbnailSlot(() => undefined),
    takeThumbnailSlot(() => undefined),
  ];

  it('shows the glyph until a slot frees, then the picture', async () => {
    const held = holdBothSlots();
    const container = await draw();
    await settle();

    expect(box(container).tagName).toBe('BUTTON');
    expect(container.querySelector('img')).toBeNull();
    expect(container.querySelector('svg')).not.toBeNull();

    held[0]?.();
    await expect.poll(() => container.querySelector('img')).not.toBeNull();
    // The picture loads and hands its slot on.
    await expect.poll(() => box(container).querySelector('svg')).toBeNull();
    held[1]?.();
  });

  it('shows the artwork, not an empty box, while it waits for a slot', async () => {
    const artwork =
      'data:image/svg+xml;utf8,<svg xmlns="http://www.w3.org/2000/svg"/>';
    const held = holdBothSlots();
    try {
      const { container } = await render(FileThumbnail, {
        props: { name, glyphSrc: artwork, thumbnailUrl: plate },
      });
      await settle();

      expect(box(container).tagName).toBe('BUTTON');
      expect(box(container).querySelector('img')?.getAttribute('src')).toBe(
        artwork,
      );
    } finally {
      for (const give of held) give();
    }
  });

  it('leaves the queue when unmounted before its turn', async () => {
    const held = holdBothSlots();
    const { unmount } = await render(FileThumbnail, {
      props: { name, glyph: FolderIcon, thumbnailUrl: plate },
    });
    await settle();
    await unmount();

    held[0]?.();
    let started = false;
    const next = takeThumbnailSlot(() => {
      started = true;
    });

    // A slot still queued for the unmounted box would have taken the one
    // just freed.
    expect(started).toBe(true);
    next();
    held[1]?.();
  });

  // A printer that never answers the picture. Its address is held back on the
  // element, so the request neither loads nor fails.
  it('drops a picture that has not answered in 20 s, and its slot', async () => {
    HTMLImageElement.prototype.setAttribute = function (attr, value) {
      Element.prototype.setAttribute.call(
        this,
        attr === 'src' ? 'data-asked' : attr,
        value,
      );
    };
    HTMLImageElement.prototype.removeAttribute = function (attr) {
      Element.prototype.removeAttribute.call(
        this,
        attr === 'src' ? 'data-asked' : attr,
      );
    };
    const other = takeThumbnailSlot(() => undefined);
    try {
      vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
      let stuck: HTMLImageElement | null = null;
      try {
        const container = await draw();
        await expect.poll(() => container.querySelector('img')).not.toBeNull();
        stuck = container.querySelector('img');
        expect(stuck?.getAttribute('data-asked')).toBe(plate);
        vi.advanceTimersByTime(20_000);
      } finally {
        vi.useRealTimers();
      }

      // Dropping the address is what cancels the request.
      await expect.poll(() => stuck?.getAttribute('data-asked')).toBeNull();
      expect(stuck?.isConnected).toBe(false);
      let started = false;
      const next = takeThumbnailSlot(() => {
        started = true;
      });
      expect(started).toBe(true);
      next();
    } finally {
      other();
      Reflect.deleteProperty(HTMLImageElement.prototype, 'setAttribute');
      Reflect.deleteProperty(HTMLImageElement.prototype, 'removeAttribute');
    }
  });

  // Paging or leaving the page before the picture answered.
  it('cancels a picture that has not answered when it is unmounted', async () => {
    HTMLImageElement.prototype.setAttribute = function (attr, value) {
      Element.prototype.setAttribute.call(
        this,
        attr === 'src' ? 'data-asked' : attr,
        value,
      );
    };
    HTMLImageElement.prototype.removeAttribute = function (attr) {
      Element.prototype.removeAttribute.call(
        this,
        attr === 'src' ? 'data-asked' : attr,
      );
    };
    try {
      const { container, unmount } = await render(FileThumbnail, {
        props: { name, glyph: FolderIcon, thumbnailUrl: plate },
      });
      await expect.poll(() => container.querySelector('img')).not.toBeNull();
      const pending = container.querySelector('img');
      expect(pending?.getAttribute('data-asked')).toBe(plate);

      await unmount();
      expect(pending?.getAttribute('data-asked')).toBeNull();
    } finally {
      Reflect.deleteProperty(HTMLImageElement.prototype, 'setAttribute');
      Reflect.deleteProperty(HTMLImageElement.prototype, 'removeAttribute');
    }
  });

  it('leaves the focus on the trigger while the preview is up', async () => {
    const container = await draw();

    await userEvent.tab();
    await expect.poll(preview).not.toBeNull();

    // Nothing inside the preview can be reached, so moving focus into it would
    // take a hovering reader's keyboard somewhere they never asked to be.
    expect(document.activeElement).toBe(box(container));
  });
});
