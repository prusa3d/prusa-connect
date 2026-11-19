import { describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { t } from '../../i18n';
import LongLine from '../../test/long-line.svelte';
import PlainPane from '../../test/plain-pane.svelte';
import PlainSection from '../../test/plain-section.svelte';
import PropNamesSection from '../../test/prop-names-section.svelte';
import { readyLoader } from '../../test/fakes';
import CameraSection from '../../test/camera-section.svelte';
import ThrowingPane from '../../test/throwing-pane.svelte';
import ThrowingSection from '../../test/throwing-section.svelte';
import { SlotRegistry } from '../registry.svelte';
import type { ModuleContributes } from '../types';
import SlotOutlet from './slot-outlet.svelte';

const contributes: ModuleContributes = {
  device_section: [
    { id: 'broken', title: 'Broken' },
    { id: 'spool-status', title: 'Loaded spool' },
  ],
  settings: [
    { id: 'broken', title: 'Broken' },
    { id: 'spools', title: 'Spools' },
  ],
};

/** A `ModuleContributes` with one device-card entry per id, titled `Line <id>`. */
const withDeviceCards = (ids: string[]): ModuleContributes => ({
  device_card: ids.map((id) => ({ id, title: `Line ${id}` })),
});

const registryFor = (block: ModuleContributes = contributes) => {
  const registry = new SlotRegistry();
  registry.declare('spools', block);

  return registry;
};

describe('SlotOutlet', () => {
  it('renders a device-section section and hands it the device id', async () => {
    const registry = registryFor();
    registry.registerSection(
      'spools',
      'device-section',
      'spool-status',
      PlainSection,
    );

    await render(SlotOutlet, {
      props: {
        screen: 'printer-detail',
        name: 'device-section',
        deviceId: 'printer-1',
        registry,
      },
    });

    await expect
      .element(page.getByRole('heading', { name: 'Loaded spool' }))
      .toBeInTheDocument();
    await expect
      .element(page.getByText('plain section for printer-1'))
      .toBeInTheDocument();
  });

  // ADR-20261106-xm4 rule 5: no heading, and no room for a camera the module
  // does not feed.
  it('draws a camera section bare, and takes no room where it draws nothing', async () => {
    const registry = registryFor({
      camera_section: [{ id: 'buddy3d', title: 'Buddy3D camera' }],
    });
    registry.registerSection(
      'spools',
      'camera-section',
      'buddy3d',
      CameraSection,
    );

    const mine = await render(SlotOutlet, {
      props: {
        screen: 'printer-detail',
        name: 'camera-section',
        camera: 'bed-cam',
        registry,
      },
    });
    await expect.element(page.getByText('status of bed-cam')).toBeVisible();
    expect(page.getByRole('heading', { name: 'Buddy3D camera' }).query()).toBe(
      null,
    );
    await mine.unmount();

    const { container } = await render(SlotOutlet, {
      props: {
        screen: 'printer-detail',
        name: 'camera-section',
        camera: 'door-cam',
        registry,
      },
    });
    const outlet = container.querySelector('[data-slot="camera-section"]');
    expect(outlet).not.toBe(null);
    expect(outlet?.getBoundingClientRect().height).toBe(0);
  });

  // ADR-20261106-km2: a camera's live view is drawn by the module that offers
  // its type and by no other.
  it('draws only the registrations of the namespace it is given', async () => {
    const registry = new SlotRegistry();
    registry.declare('one', { camera_view: [{ id: 'view', title: 'View' }] });
    registry.declare('two', { camera_view: [{ id: 'view', title: 'View' }] });
    registry.registerSection('one', 'camera-view', 'view', CameraSection);
    registry.registerSection('two', 'camera-view', 'view', CameraSection);

    const { container } = await render(SlotOutlet, {
      props: {
        screen: 'printer-detail',
        name: 'camera-view',
        camera: 'bed-cam',
        namespace: 'two',
        registry,
      },
    });

    await expect.element(page.getByText('status of bed-cam')).toBeVisible();
    const drawn = [...container.querySelectorAll('[data-slot="camera-view"]')];
    expect(drawn.map((node) => node.getAttribute('data-module'))).toEqual([
      'two',
    ]);
  });

  // ADR-20260914-244 rule 14. moduleBuild() wraps a module's CSS in this scope, and
  // the outlet's wrapper is the element it has to match. The host's rule and
  // the module's sit in one layer, so only the scope keeps the module's later
  // rule off the host.
  it('keeps a module rule scoped by moduleBuild inside the section', async () => {
    const registry = registryFor();
    registry.registerSection(
      'spools',
      'device-section',
      'spool-status',
      PlainSection,
    );
    const host = document.createElement('p');
    document.body.append(host);
    const sheets = [
      '@layer utilities{p{order:1}}',
      '@scope ([data-module]){@layer utilities{p{order:2}}}',
    ].map((css) => {
      const style = document.createElement('style');
      style.textContent = css;
      document.head.append(style);
      return style;
    });

    await render(SlotOutlet, {
      props: {
        screen: 'printer-detail',
        name: 'device-section',
        deviceId: 'printer-1',
        registry,
      },
    });

    await expect
      .element(page.getByTestId('plain-section'))
      .toHaveStyle({ order: '2' });
    expect(getComputedStyle(host).order).toBe('1');

    host.remove();
    for (const style of sheets) style.remove();
  });

  // ADR-20260911-4p6 rule 26: a section that throws is dropped without taking the card.
  it('keeps a throwing section from taking the others, and records why', async () => {
    const registry = registryFor();
    registry.registerSection(
      'spools',
      'device-section',
      'broken',
      ThrowingSection,
    );
    registry.registerSection(
      'spools',
      'device-section',
      'spool-status',
      PlainSection,
    );

    await render(SlotOutlet, {
      props: {
        screen: 'printer-detail',
        name: 'device-section',
        deviceId: 'printer-1',
        registry,
      },
    });

    await expect.element(page.getByTestId('plain-section')).toBeInTheDocument();
    expect(document.querySelector('[data-section="broken"]')).toBeNull();
    expect(registry.refusals()[0]).toMatchObject({
      namespace: 'spools',
      reason: 'threw',
    });
  });

  it('renders settings panes and no device-section section', async () => {
    const registry = registryFor();
    registry.registerSection(
      'spools',
      'device-section',
      'spool-status',
      PlainSection,
    );
    registry.registerSection('spools', 'settings', 'spools', PlainPane);

    await render(SlotOutlet, {
      props: { screen: 'settings', name: 'settings', registry },
    });

    await expect.element(page.getByTestId('plain-pane')).toBeInTheDocument();
    expect(document.querySelector('[data-slot="device-section"]')).toBeNull();
  });

  it('keeps a throwing pane from taking the others', async () => {
    const registry = registryFor();
    registry.registerSection('spools', 'settings', 'broken', ThrowingPane);
    registry.registerSection('spools', 'settings', 'spools', PlainPane);

    await render(SlotOutlet, {
      props: { screen: 'settings', name: 'settings', registry },
    });

    await expect.element(page.getByTestId('plain-pane')).toBeInTheDocument();
    expect(document.querySelector('[data-pane="broken"]')).toBeNull();
  });

  it('renders nothing when no module filled the slot', async () => {
    const registry = registryFor();

    const screen = await render(SlotOutlet, {
      props: { screen: 'settings', name: 'settings', registry },
    });

    await expect
      .element(page.getByTestId('plain-pane'))
      .not.toBeInTheDocument();
    expect(screen.container.querySelector('section')).toBeNull();
  });

  it('draws one line per device-card registration, cut to one line, with the title as its name', async () => {
    const registry = registryFor(withDeviceCards(['first', 'second', 'third']));
    registry.registerSection('spools', 'device-card', 'first', LongLine);
    registry.registerSection('spools', 'device-card', 'second', LongLine);
    registry.registerSection('spools', 'device-card', 'third', LongLine);
    await render(SlotOutlet, {
      props: {
        screen: 'printers',
        name: 'device-card',
        deviceId: 'printer-1',
        registry,
      },
    });
    const lines = [
      ...document.querySelectorAll<HTMLElement>('[data-slot="device-card"]'),
    ];
    expect(lines).toHaveLength(3);
    for (const line of lines) {
      expect(line.getBoundingClientRect().height).toBeCloseTo(
        Number.parseFloat(getComputedStyle(line).lineHeight),
        1,
      );
      expect(line.getAttribute('aria-label')).not.toBeNull();
      expect(line.querySelector('h3')).toBeNull();
    }
  });

  it('draws nothing for a device-card whose component throws', async () => {
    const registry = registryFor(withDeviceCards(['broken']));
    registry.registerSection(
      'spools',
      'device-card',
      'broken',
      ThrowingSection,
    );
    await render(SlotOutlet, {
      props: {
        screen: 'printers',
        name: 'device-card',
        deviceId: 'printer-1',
        registry,
      },
    });
    await expect.poll(() => registry.refusals()).toHaveLength(1);
    expect(
      document.querySelector('[data-slot="device-card"]')?.textContent.trim() ??
        '',
    ).toBe('');
  });

  it('draws only the tab the entry names', async () => {
    const registry = registryFor({
      device_tab: [
        { id: 'a', title: 'A' },
        { id: 'b', title: 'B' },
      ],
    });
    registry.registerSection('spools', 'device-tab', 'a', PlainSection);
    registry.registerSection('spools', 'device-tab', 'b', ThrowingSection);

    await render(SlotOutlet, {
      props: {
        screen: 'printer-detail',
        name: 'device-tab',
        deviceId: 'p',
        entry: 'spools.a',
        registry,
      },
    });

    await expect.element(page.getByTestId('plain-section')).toBeInTheDocument();
    expect(registry.refusals()).toHaveLength(0);
  });

  it('shows the module-failed text for a declared tab nothing registered', async () => {
    const registry = registryFor({ device_tab: [{ id: 'a', title: 'A' }] });

    await render(SlotOutlet, {
      props: {
        screen: 'printer-detail',
        name: 'device-tab',
        deviceId: 'p',
        entry: 'spools.a',
        registry,
        loader: readyLoader(),
      },
    });

    await expect
      .element(page.getByText(t('modulePage.failed')))
      .toBeInTheDocument();
  });

  // I1: a throwing tab component must show the same failed panel as a page
  // that throws, not a blank tab (rows.length is still 1, so the "nothing
  // registered" fallback above does not fire).
  it('shows the module-failed panel when a registered tab throws, and records why', async () => {
    const registry = registryFor({
      device_tab: [
        { id: 'a', title: 'A' },
        { id: 'b', title: 'B' },
      ],
    });
    registry.registerSection('spools', 'device-tab', 'a', ThrowingSection);
    registry.registerSection('spools', 'device-tab', 'b', PlainSection);

    await render(SlotOutlet, {
      props: {
        screen: 'printer-detail',
        name: 'device-tab',
        deviceId: 'p',
        entry: 'spools.a',
        registry,
      },
    });

    await expect
      .element(page.getByText(t('modulePage.failed')))
      .toBeInTheDocument();
    expect(registry.refusals()[0]).toMatchObject({
      namespace: 'spools',
      reason: 'threw',
    });

    // The other tab is unaffected: switching to it still renders normally.
    await render(SlotOutlet, {
      props: {
        screen: 'printer-detail',
        name: 'device-tab',
        deviceId: 'p',
        entry: 'spools.b',
        registry,
      },
    });

    await expect.element(page.getByTestId('plain-section')).toBeInTheDocument();
  });

  // ADR-20261110-5np.
  describe('an entry with a devices selector', () => {
    const residentRegistry = () => {
      const registry = registryFor({
        device_section: [
          { id: 'display', title: 'Display', devices: { resident: true } },
        ],
        device_tab: [
          { id: 'display', title: 'Display', devices: { resident: true } },
        ],
      });
      registry.registerSection(
        'spools',
        'device-section',
        'display',
        PlainSection,
      );
      registry.registerSection('spools', 'device-tab', 'display', PlainSection);

      return registry;
    };

    it('hands the component the device id and not the record', async () => {
      const registry = registryFor({
        device_section: [
          { id: 'display', title: 'Display', devices: { resident: true } },
        ],
      });
      registry.registerSection(
        'spools',
        'device-section',
        'display',
        PropNamesSection,
      );

      await render(SlotOutlet, {
        props: {
          screen: 'printer-detail',
          name: 'device-section',
          deviceId: 'host',
          device: { resident: true },
          registry,
        },
      });

      const names = page.getByTestId('prop-names');
      await expect.element(names).toBeInTheDocument();
      expect(names.element().textContent.trim()).toBe('deviceId');
    });

    it('draws a section for a resident device', async () => {
      const registry = residentRegistry();

      await render(SlotOutlet, {
        props: {
          screen: 'printer-detail',
          name: 'device-section',
          deviceId: 'host',
          device: { resident: true },
          registry,
        },
      });

      await expect
        .element(page.getByTestId('plain-section'))
        .toHaveTextContent('plain section for host');
    });

    it('draws no section for another device, or for one with no record', async () => {
      const registry = residentRegistry();

      await render(SlotOutlet, {
        props: {
          screen: 'printer-detail',
          name: 'device-section',
          deviceId: 'mk4',
          device: {},
          registry,
        },
      });
      await render(SlotOutlet, {
        props: {
          screen: 'printer-detail',
          name: 'device-section',
          deviceId: 'mk4',
          registry,
        },
      });

      expect(page.getByTestId('plain-section').query()).toBeNull();
    });

    it('draws the tab only for a resident device', async () => {
      const registry = residentRegistry();

      await render(SlotOutlet, {
        props: {
          screen: 'printer-detail',
          name: 'device-tab',
          deviceId: 'mk4',
          device: { resident: false },
          entry: 'spools.display',
          registry,
        },
      });
      expect(page.getByTestId('plain-section').query()).toBeNull();

      await render(SlotOutlet, {
        props: {
          screen: 'printer-detail',
          name: 'device-tab',
          deviceId: 'host',
          device: { resident: true },
          entry: 'spools.display',
          registry,
        },
      });
      await expect
        .element(page.getByTestId('plain-section'))
        .toBeInTheDocument();
    });
  });
});
