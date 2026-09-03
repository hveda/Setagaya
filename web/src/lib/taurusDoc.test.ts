import { describe, expect, it } from 'vitest';
import { applyFields, readFields } from './taurusDoc';

// The fixture mirrors a real stored fragment: comments, an uncompiled key
// (think-time -- G6 flags it as info), and execution-level telemetry.
const fragment = `# deployment defaults live here
execution:
    - executor: jmeter
      concurrency: 50
      ramp-up: 30s # hand-written
      hold-for: 10m
      scenario: checkout-11
scenarios:
    checkout-11:
        # the target the ops team pinned
        default-address: http://checkout.svc
        think-time: 150ms
        headers:
            X-Auth: fragment-token
        requests:
            - method: GET
              url: http://checkout.svc/api/orders
`;

describe('readFields', () => {
  it('reads address, method, path, and headers from the first scenario', () => {
    const f = readFields(fragment);
    expect(f.defaultAddress).toBe('http://checkout.svc');
    expect(f.method).toBe('GET');
    expect(f.path).toBe('/api/orders');
    expect(f.headers).toEqual([{ name: 'X-Auth', value: 'fragment-token' }]);
  });

  it('returns empty fields for malformed YAML (R6 owns the error)', () => {
    expect(readFields('execution: [')).toEqual({ defaultAddress: '', method: '', path: '', headers: [] });
  });
});

describe('applyFields', () => {
  it('is byte-identical when every field round-trips unchanged', () => {
    const f = readFields(fragment);
    expect(applyFields(fragment, f)).toBe(fragment);
  });

  it('changes ONLY the edited node: comments, key order, think-time survive', () => {
    const f = readFields(fragment);
    f.headers.push({ name: 'Cookie', value: 'session=abc' });
    const out = applyFields(fragment, f);
    // Comments survive.
    expect(out).toContain('# deployment defaults live here');
    expect(out).toContain('# the target the ops team pinned');
    expect(out).toContain('ramp-up: 30s # hand-written');
    // Uncompiled key untouched (byte-equal, not just semantically).
    expect(out).toContain('think-time: 150ms');
    // New header present, old header untouched.
    expect(out).toContain('X-Auth: fragment-token');
    expect(out).toContain('Cookie: session=abc');
    // Everything else byte-equal to the original.
    const strip = (s: string) =>
      s.split('\n').filter((l) => !l.includes('Cookie:')).join('\n');
    expect(strip(out)).toBe(strip(fragment));
  });

  it('writes default-address and re-joins the request url against it', () => {
    const f = readFields(fragment);
    f.defaultAddress = 'http://checkout2.svc';
    const out = applyFields(fragment, f);
    expect(out).toContain('default-address: http://checkout2.svc');
    expect(out).toContain('url: http://checkout2.svc/api/orders');
  });

  it('deleting every header removes the headers key entirely', () => {
    const f = readFields(fragment);
    f.headers = [];
    const out = applyFields(fragment, f);
    expect(out).not.toContain('headers:');
  });

  it('refuses to write into malformed YAML', () => {
    expect(() => applyFields('execution: [', readFields('execution: ['))).toThrow(/invalid YAML/);
  });
});
