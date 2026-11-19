import { describe, expect, it } from 'vitest';

import { EASYPRINT_ORIGIN, isPlainHttp, pairHref } from './easyprint';

describe('what this module knows about EasyPrint', () => {
  it('asks for the origin module.yaml asks for', async () => {
    const manifest = await import('../../module.yaml?raw');

    expect(manifest.default).toContain(
      `allowed_origins:\n  - ${EASYPRINT_ORIGIN}\n`,
    );
  });

  it('hands EasyPrint the address of the server, escaped', () => {
    expect(pairHref('http://192.168.1.10:8080')).toBe(
      'https://www.printables.com/slice?pair_connect_local=http%3A%2F%2F192.168.1.10%3A8080',
    );
    expect(pairHref('https://connect.example')).toBe(
      'https://www.printables.com/slice?pair_connect_local=https%3A%2F%2Fconnect.example',
    );
  });

  it('tells plain HTTP from HTTPS', () => {
    expect(isPlainHttp('http:')).toBe(true);
    expect(isPlainHttp('https:')).toBe(false);
  });
});
