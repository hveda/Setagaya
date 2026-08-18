// Types and fetcher for the reservation calendar's data source: GET
// /api/tenants/{tenant_id}/reservations. Field names mirror
// internal/adapters/httpapi's reservationResponse JSON tags exactly.
import { apiClient } from './client';

export interface Reservation {
  id: number;
  tenant_id: number;
  cluster?: string;
  engine_count: number;
  start: string;
  end: string;
  execution_id: number;
}

export interface ListReservationsParams {
  from?: Date;
  to?: Date;
  cluster?: string;
}

export function listTenantReservations(tenantId: number, params: ListReservationsParams = {}): Promise<Reservation[]> {
  const query = new URLSearchParams();
  if (params.from) query.set('from', params.from.toISOString());
  if (params.to) query.set('to', params.to.toISOString());
  if (params.cluster) query.set('cluster', params.cluster);
  const suffix = query.toString() ? `?${query.toString()}` : '';
  return apiClient.get<Reservation[]>(`/tenants/${tenantId}/reservations${suffix}`);
}
