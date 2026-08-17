import { describe, expect, it } from 'vitest';
import { parseShard } from './Reports';

describe('parseShard', () => {
  it('parses 0-indexed shard numbers', () => {
    expect(parseShard('0')).toBe(0);
    expect(parseShard('3')).toBe(3);
    expect(parseShard(' 2 ')).toBe(2);
  });

  // Number('') is 0 in JS -- the empty-string guard keeps an empty input
  // from silently loading shard 0.
  it('rejects an empty input rather than treating it as shard 0', () => {
    expect(parseShard('')).toBeNull();
    expect(parseShard('  ')).toBeNull();
  });

  it('rejects negatives, fractions, and junk', () => {
    expect(parseShard('-1')).toBeNull();
    expect(parseShard('1.5')).toBeNull();
    expect(parseShard('abc')).toBeNull();
  });
});
