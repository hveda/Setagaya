import { afterEach, describe, expect, it, vi } from 'vitest';
import { getScenarioRequests, setScenarioRequests } from './scenarios';
import { apiClient } from './client';

afterEach(() => vi.restoreAllMocks());

describe('scenario fragment api', () => {
  it('get returns the body text verbatim', async () => {
    const yaml = '# keep me\\nmulti-test:\\n  scenario: false\\n';
    vi.spyOn(apiClient, 'text').mockResolvedValue(yaml);
    await expect(getScenarioRequests(5)).resolves.toBe(yaml);
    expect(apiClient.text).toHaveBeenCalledWith('/scenarios/5/requests');
  });

  it('put sends text/yaml with the body untouched', async () => {
    const spy = vi.spyOn(apiClient, 'putRaw').mockResolvedValue(undefined);
    const body = 'execution:\\n  ramp-up: 30s # hand-written\\n';
    await setScenarioRequests(5, body);
    expect(spy).toHaveBeenCalledWith('/scenarios/5/requests', 'text/yaml; charset=utf-8', body);
  });
});
