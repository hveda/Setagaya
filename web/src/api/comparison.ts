// Types and fetcher for GET /api/campaigns/{campaign_id}/comparison.
// Field names mirror internal/adapters/httpapi/campaign_handlers.go's
// campaignComparisonResponse/serviceComparisonResponse JSON tags exactly.
import { apiClient } from './client';

// The seven classifications of domain/campaign's ComparisonStatus.
export type ComparisonStatus =
  | 'improved'
  | 'regressed'
  | 'newly_at_risk'
  | 'still_at_risk'
  | 'steady'
  | 'new'
  | 'dropped';

export interface ServiceComparison {
  project_id: number;
  status: ComparisonStatus;
  has_current: boolean;
  go?: boolean;
  has_baseline: boolean;
  baseline_go?: boolean;
}

export interface CampaignComparison {
  campaign_id: number;
  /** False = no resolvable baseline (tenant's first campaign, or none given): services is empty, not an error. */
  has_baseline: boolean;
  baseline_campaign_id?: number;
  services: ServiceComparison[];
}

/**
 * `go`/`baseline_go` are omitempty on the wire -- absent, not false, when
 * unset (see campaign_handlers.go) -- so they stay optional. `services` is
 * always built with make() server-side, but normalized here anyway so
 * callers can always treat it as an array, same as campaigns.ts's verdict
 * normalization.
 */
export async function getCampaignComparison(campaignId: number, baselineId?: number): Promise<CampaignComparison> {
  const query = baselineId ? `?baseline=${baselineId}` : '';
  const got = await apiClient.get<CampaignComparison>(`/campaigns/${campaignId}/comparison${query}`);
  return { ...got, services: got.services ?? [] };
}
