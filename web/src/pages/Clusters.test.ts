import { describe, expect, it } from 'vitest';
import { clusterCapacity, formatClusterTime, originDescription } from './Clusters';
import type { Cluster } from '../api/clusters';

describe('originDescription', () => {
  // Both wire values need a credential-ownership explanation -- the page
  // renders it as the row's title attribute.
  it('explains both origins', () => {
    expect(originDescription('operator')).toBe('home-cluster Secret managed by the platform operator');
    expect(originDescription('byoc')).toBe('customer-supplied kubeconfig (bring your own cluster)');
  });
});

describe('clusterCapacity', () => {
  const registered: Cluster = {
    name: 'honryu',
    api_url: 'https://kubernetes.default.svc:443',
    ingest_url: 'https://honryu.pve.heri.life/api/ingest',
    sidecar_image: 'registry.pve.heri.life/honryu/honryu-sidecar:phase16',
    namespace: 'honryu',
    secret_ref: 'cluster-honryu-explicit-creds',
    origin: 'operator',
    created_by: 'alice',
    created_time: '2026-09-05T10:29:02.848672Z',
  };

  // Pins the phase 22 honest scope: GET /api/clusters' clusterResponse
  // carries registration fields only -- no engine counts, no ceiling -- so
  // no cluster carries capacity numbers and every row renders the meter's
  // "no capacity reported" state. The day the backend grows real fields
  // (phase 23 candidate), this test is the one to flip.
  it('reports no capacity numbers for a registered cluster', () => {
    expect(clusterCapacity(registered)).toEqual({});
  });
});

describe('formatClusterTime', () => {
  it('formats a valid ISO timestamp via the locale', () => {
    const iso = '2026-08-17T00:00:00Z';
    expect(formatClusterTime(iso)).toBe(new Date(iso).toLocaleString());
  });

  it('passes an unparseable value through unchanged rather than rendering "Invalid Date"', () => {
    expect(formatClusterTime('not-a-date')).toBe('not-a-date');
  });
});
