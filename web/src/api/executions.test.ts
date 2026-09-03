import { describe, expect, it, vi } from 'vitest';
import { listExecutions } from './executions';
import { apiClient } from './client';

describe('listExecutions', () => {
  it('GETs /executions and returns the summaries', async () => {
    const rows = [{ id: 7, name: 'supersale', project_id: 3, created_time: '2026-09-01T00:00:00Z' }];
    const spy = vi.spyOn(apiClient, 'get').mockResolvedValueOnce(rows);
    await expect(listExecutions()).resolves.toEqual(rows);
    expect(spy).toHaveBeenCalledWith('/executions');
    spy.mockRestore();
  });
});
