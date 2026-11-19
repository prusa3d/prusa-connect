// The channel a person reads for a version: stable, or the word of its
// prerelease label, such as alpha in 3.0.0-alpha12.

export type ChannelWord = 'stable' | 'alpha' | 'beta' | 'rc' | 'prerelease';

export const channelOf = (version: string, channel: string): ChannelWord => {
  if (channel === 'stable') return 'stable';
  const label = /-([a-z]+)/u.exec(version)?.[1];
  if (label === 'alpha' || label === 'beta' || label === 'rc') return label;
  return 'prerelease';
};
