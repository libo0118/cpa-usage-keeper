import { ApiError, apiFetch, apiPath } from '@/lib/api';

export interface KeyBudgetResponse {
  report: {
    keys: Array<{ key_id: string; key_preview: string; label: string; allow_all: boolean }>;
    resources: Array<{ resource_id: string; label: string; provider: string; kind: string; disabled: boolean }>;
    budgets: Array<{
      key_id: string; resource_id: string; period: string; limit_usd: string | null;
      used_usd: string; reserved_usd: string; remaining_usd: string | null;
      cycle_start: string; reset_at: string; status: string; unpriced_requests: number;
    }>;
    prices_updated_at?: string;
  };
  sync: { status: string; last_attempt: string; last_success: string; unavailable_models: string[] | null };
}

export async function fetchKeyBudgets(signal: AbortSignal): Promise<KeyBudgetResponse> {
  const response = await apiFetch(apiPath('/key-policies'), { signal });
  // Do not surface upstream error bodies: they may contain credential details.
  if (!response.ok) throw new ApiError('KEY_BUDGETS_UNAVAILABLE', response.status);
  return response.json();
}

export function keyPolicyLink(cpaURL: string): string {
  if (!cpaURL) return '';
  let url: URL;
  try { url = new URL(cpaURL); } catch { return ''; }
  if (url.protocol !== 'https:' && url.protocol !== 'http:') return '';
  url.hash = '/key-policies';
  return url.toString();
}
