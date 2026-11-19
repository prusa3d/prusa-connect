import { describe, expect, it } from 'vitest';

import { jobsRead } from './fixtures';
import {
  compareJobs,
  DEFAULT_SORT,
  filterJobs,
  jobFacetCounts,
  jobRowsOf,
  NO_JOBS_SELECTED,
  searchTerm,
} from './presentation';
import type { JobSort } from './types';

const rows = jobRowsOf(jobsRead());

const onlyCore = { ...NO_JOBS_SELECTED, printer: ['prn-core'] };

describe('the prints a source carries', () => {
  it('flattens every printer that answered onto one list', () => {
    expect(rows).toHaveLength(12);
    expect(rows.every((row) => row.printerName !== '')).toBe(true);
  });

  it('omits open idle records and keeps ended prints under their outcomes', () => {
    const source = jobsRead();
    const printer = source.printers.find((entry) => entry.id === 'prn-core');
    const job = printer?.jobs?.[0];
    if (printer === undefined || job === undefined)
      throw new Error('Missing sample print');

    const { ended, ...open } = job;
    const idle = { ...open, id: 'idle', state: 'IDLE' };
    const unknown = { ...idle, id: 'unknown', ended: ended ?? job.started };
    expect(
      jobRowsOf({
        ...source,
        printers: [
          {
            ...printer,
            jobs: [
              idle,
              { ...unknown, id: 'finished', outcome: 'finished' },
              { ...unknown, id: 'failed', outcome: 'failed' },
              { ...unknown, id: 'cancelled', outcome: 'cancelled' },
              unknown,
            ],
          },
        ],
      }).map(({ id, state }) => [id, state]),
    ).toEqual([
      ['finished', 'FINISHED'],
      ['failed', 'ERROR'],
      ['cancelled', 'STOPPED'],
      ['unknown', 'UNKNOWN'],
    ]);
  });

  it('files every ended print by its outcome, whatever state it ended in', () => {
    const source = jobsRead();
    const printer = source.printers.find((entry) => entry.id === 'prn-core');
    const job = printer?.jobs?.[0];
    if (printer === undefined || job === undefined)
      throw new Error('Missing sample print');

    const ended = { ...job, ended: job.ended ?? job.started };
    const record = (id: string, state: string, outcome?: string) =>
      ({ ...ended, id, state, outcome }) as typeof job;
    expect(
      jobRowsOf({
        ...source,
        printers: [
          {
            ...printer,
            jobs: [
              record('ready', 'READY', 'finished'),
              record('printing', 'PRINTING', 'finished'),
              record('failed', 'FINISHED', 'failed'),
              record('new-word', 'IDLE', 'aborted'),
              record('no-outcome', 'FINISHED'),
              // The core writes `unknown` for a driver that named no outcome.
              record('stopped-unknown', 'STOPPED', 'unknown'),
              record('printing-unknown', 'PRINTING', 'unknown'),
            ],
          },
        ],
      }).map(({ id, state }) => [id, state]),
    ).toEqual([
      ['ready', 'FINISHED'],
      ['printing', 'FINISHED'],
      ['failed', 'ERROR'],
      ['new-word', 'UNKNOWN'],
      ['no-outcome', 'FINISHED'],
      ['stopped-unknown', 'STOPPED'],
      ['printing-unknown', 'UNKNOWN'],
    ]);
  });

  it('gives a print still on the bed no span, and a finished one its own', () => {
    const running = rows.find((row) => row.running);
    const finished = rows.find((row) => row.id === 'prn-rack-3:41');

    expect(running?.durationSeconds).toBeUndefined();
    expect(finished?.durationSeconds).toBe(128 * 60);
  });
});

describe('what a chip would bring', () => {
  it('counts with its own dimension left out', () => {
    // Three prints ran on CORE One, and picking it must not reduce the
    // printer chips beside it to that one printer.
    const counts = jobFacetCounts(rows, onlyCore, '', 'printer');

    expect(counts.get('prn-core')).toBe(3);
    expect(counts.get('prn-mini')).toBe(2);
  });

  it('counts within the other dimension, and within the search', () => {
    const states = jobFacetCounts(rows, onlyCore, '', 'state');
    expect(states.get('FINISHED')).toBe(1);

    const searched = jobFacetCounts(
      rows,
      NO_JOBS_SELECTED,
      searchTerm(' Benchy '),
      'printer',
    );
    expect(searched.get('prn-mini')).toBe(1);
    expect(searched.get('prn-rack-3')).toBeUndefined();
  });

  it('reads zero for a printer the filters leave nothing on', () => {
    const counts = jobFacetCounts(
      rows,
      NO_JOBS_SELECTED,
      searchTerm('benchy'),
      'printer',
      ['prn-xl'],
    );

    expect(counts.get('prn-xl')).toBe(0);
  });
});

describe('narrowing and ordering', () => {
  it('applies both dimensions and the search at once', () => {
    const found = filterJobs(rows, onlyCore, searchTerm('benchy'));

    expect(found.map((row) => row.id)).toEqual(['prn-core:16']);
  });

  it('opens on the newest print, and puts the oldest first ascending', () => {
    const newest = [...rows].sort((a, b) => compareJobs(a, b, DEFAULT_SORT));
    expect(newest[0]?.id).toBe('prn-rack-3:42');

    const oldest: JobSort = { column: 'started', ascending: true };
    expect([...rows].sort((a, b) => compareJobs(a, b, oldest))[0]?.id).toBe(
      'prn-mk3s:89',
    );
  });

  it('orders by duration, longest first', () => {
    const byDuration: JobSort = { column: 'duration', ascending: false };

    expect([...rows].sort((a, b) => compareJobs(a, b, byDuration))[0]?.id).toBe(
      'prn-rack-3:40',
    );
  });
});
