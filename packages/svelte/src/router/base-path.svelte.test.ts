import { expect, test } from 'vitest';

import {
  basePathOf,
  keepLinksUnderBase,
  withBase,
  withoutBase,
} from './base-path';

const documentWith = (html: string): Document =>
  new DOMParser().parseFromString(
    `<!doctype html><html><head>${html}</head><body></body></html>`,
    'text/html',
  );

test('the prefix comes from the base element, without its trailing slash', () => {
  expect(
    basePathOf(documentWith('<base href="/api/hassio_ingress/t1/" />')),
  ).toBe('/api/hassio_ingress/t1');
  expect(basePathOf(documentWith('<base href="/" />'))).toBe('');
  expect(basePathOf(documentWith(''))).toBe('');
  expect(basePathOf(undefined)).toBe('');
});

test('a base that is not a path from the root is no prefix', () => {
  expect(
    basePathOf(documentWith('<base href="https://elsewhere.example/x/" />')),
  ).toBe('');
  expect(
    basePathOf(documentWith('<base href="//elsewhere.example/x/" />')),
  ).toBe('');
  expect(basePathOf(documentWith('<base href="relative/" />'))).toBe('');
});

test('a path from the root moves under the prefix once', () => {
  const base = '/ingress/t1';

  expect(withBase('/printers/p1?tab=files', base)).toBe(
    '/ingress/t1/printers/p1?tab=files',
  );
  expect(withBase('/', base)).toBe('/ingress/t1/');
  expect(withBase('/ingress/t1/printers', base)).toBe('/ingress/t1/printers');
  expect(withBase('/ingress/t1', base)).toBe('/ingress/t1');
  // A path that only starts with the same letters is not under the prefix.
  expect(withBase('/ingress/t10/x', base)).toBe('/ingress/t1/ingress/t10/x');
});

test('a full URL, a protocol-relative one and a relative address are left alone', () => {
  const base = '/ingress/t1';

  expect(withBase('https://example.com/x', base)).toBe('https://example.com/x');
  expect(withBase('//example.com/x', base)).toBe('//example.com/x');
  expect(withBase('assets/x.js', base)).toBe('assets/x.js');
  expect(withBase('#top', base)).toBe('#top');
});

test('at the root nothing moves', () => {
  expect(withBase('/printers', '')).toBe('/printers');
  expect(withoutBase('/printers', '')).toBe('/printers');
});

test('the prefix comes off the path the browser shows', () => {
  const base = '/ingress/t1';

  expect(withoutBase('/ingress/t1/printers/p1', base)).toBe('/printers/p1');
  expect(withoutBase('/ingress/t1/', base)).toBe('/');
  expect(withoutBase('/ingress/t1', base)).toBe('/');
  expect(withoutBase('/elsewhere', base)).toBe('/elsewhere');
  expect(withoutBase('/ingress/t10/x', base)).toBe('/ingress/t10/x');
});

test('a link from the root moves under the prefix when the pointer reaches it', () => {
  const anchor = document.createElement('a');
  anchor.setAttribute('href', '/printers/p1');
  const inside = document.createElement('span');
  anchor.append(inside);
  const outside = document.createElement('a');
  outside.setAttribute('href', 'https://example.com/');
  document.body.append(anchor, outside);

  const stop = keepLinksUnderBase(document, '/ingress/t1');
  inside.dispatchEvent(new PointerEvent('pointerover', { bubbles: true }));
  outside.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }));
  anchor.dispatchEvent(new FocusEvent('focusin', { bubbles: true }));

  expect(anchor.getAttribute('href')).toBe('/ingress/t1/printers/p1');
  expect(outside.getAttribute('href')).toBe('https://example.com/');

  stop();
  anchor.setAttribute('href', '/settings');
  anchor.dispatchEvent(new PointerEvent('pointerover', { bubbles: true }));
  expect(anchor.getAttribute('href')).toBe('/settings');

  anchor.remove();
  outside.remove();
});

test('at the root no link is touched', () => {
  const anchor = document.createElement('a');
  anchor.setAttribute('href', '/printers');
  document.body.append(anchor);

  const stop = keepLinksUnderBase(document, '');
  anchor.dispatchEvent(new PointerEvent('pointerover', { bubbles: true }));
  stop();

  expect(anchor.getAttribute('href')).toBe('/printers');
  anchor.remove();
});
