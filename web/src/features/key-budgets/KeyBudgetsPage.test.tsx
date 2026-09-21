// @vitest-environment happy-dom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { renderToStaticMarkup } from 'react-dom/server';
import { afterEach, describe, expect, it, vi } from 'vitest';
import '@/i18n';
import { KeyBudgetReport, KeyBudgetsPage } from './KeyBudgetsPage';
import { fetchKeyBudgets, keyPolicyLink, type KeyBudgetResponse } from './api';
import { keyBudgetTranslations } from './translations';

const data: KeyBudgetResponse = {
  report: { keys: [{ key_id: 'fingerprint', key_preview: '…abcd', label: 'Team', allow_all: false }],
    resources: [{ resource_id: 'resource-1', label: 'Account', provider: 'codex', kind: 'oauth', disabled: false }],
    budgets: [{ key_id: 'fingerprint', resource_id: 'resource-1', period: 'month', limit_usd: null, used_usd: '0.000001', reserved_usd: '1.23', remaining_usd: null, cycle_start: '', reset_at: '', status: 'ready', unpriced_requests: 2 }] },
  sync: { status: 'unsupported_prices', last_attempt: '', last_success: '', unavailable_models: ['conditional-model'] },
};
afterEach(() => vi.unstubAllGlobals());

describe('key budgets', () => {
  it('shows safe identities, exact decimal values, unlimited and pricing warnings', () => {
    const html = renderToStaticMarkup(<KeyBudgetReport data={data} />);
    for (const text of ['Team', '…abcd', 'Account', 'OAuth', 'Unlimited', '$0.000001', '$1.23', 'conditional-model', 'Unpriced requests: 2']) expect(html).toContain(text);
    expect(html).not.toContain('fingerprint');
    expect(html).not.toContain('<input');
    expect(Object.keys(keyBudgetTranslations.zh)).toEqual(Object.keys(keyBudgetTranslations.en));
    expect(Object.keys(keyBudgetTranslations['zh-TW'])).toEqual(Object.keys(keyBudgetTranslations.en));
  });
  it('uses base path, session credentials and abort signal without exposing error bodies', async () => {
    window.__APP_BASE_PATH__ = '/keeper';
    const signal = new AbortController().signal;
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(data)));
    vi.stubGlobal('fetch', fetchMock);
    await fetchKeyBudgets(signal);
    expect(fetchMock).toHaveBeenCalledWith('/keeper/api/v1/key-policies', expect.objectContaining({ signal, credentials: 'include' }));
    fetchMock.mockResolvedValue(new Response('secret-upstream-error', { status: 503 }));
    await expect(fetchKeyBudgets(signal)).rejects.toThrow('KEY_BUDGETS_UNAVAILABLE');
    delete window.__APP_BASE_PATH__;
    expect(keyPolicyLink('https://cpa.example/base/management.html#/old')).toBe('https://cpa.example/base/management.html#/key-policies');
  });
  it('aborts stale loads and shows a retryable error', async () => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
    let resolveFirst!: (value: Response) => void;
    const fetchMock = vi.fn().mockImplementationOnce(() => new Promise<Response>((resolve) => { resolveFirst = resolve; }))
      .mockResolvedValueOnce(new Response('failure', { status: 503 }));
    vi.stubGlobal('fetch', fetchMock);
    const host = document.createElement('div');
    const root = createRoot(host);
    await act(async () => { root.render(<KeyBudgetsPage cpaURL="" refreshKey={0} />); });
    expect(host.textContent).toContain('Loading');
    await act(async () => { root.render(<KeyBudgetsPage cpaURL="" refreshKey={1} />); });
    expect(fetchMock.mock.calls[0][1].signal.aborted).toBe(true);
    await act(async () => { resolveFirst(new Response(JSON.stringify(data))); });
    expect(host.textContent).toContain('Unable to load key budgets');
    expect(host.textContent).not.toContain('Team');
    expect(host.querySelector('button')?.textContent).toBe('Retry');
    await act(async () => root.unmount());
  });
});
