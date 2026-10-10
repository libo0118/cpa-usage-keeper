// @vitest-environment happy-dom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import i18n from '@/i18n'
import type { UsageIdentity, UsageQuotaRow } from '@/lib/types'
import { AuthFileCredentialsSection, AuthFileQuotaPanel } from '../AuthFileCredentialsSection'
import { CredentialDetailDrawer } from '../CredentialDetailDrawer'
import { buildAuthFileCredentialRows } from '../credentialViewModels'
import { createAuthFileSectionProps } from './credentialSectionFixtures'

globalThis.IS_REACT_ACT_ENVIRONMENT = true

const identity: UsageIdentity = {
  id: '1', name: '', auth_type: 1, auth_type_name: 'Auth File',
  identity: 'codex-balance-test', type: 'codex', provider: 'codex',
  total_requests: 0, success_count: 0, failure_count: 0,
  input_tokens: 0, output_tokens: 0, reasoning_tokens: 0, cache_read_tokens: 0,
  total_tokens: 0, last_aggregated_usage_event_id: '0', is_deleted: false,
  created_at: '2026-10-10T00:00:00Z', updated_at: '2026-10-10T00:00:00Z',
}
const windows: UsageQuotaRow[] = [
  { key: 'rate_limit.primary_window', label: '5h', scope: 'window', usedPercent: 12, window: { seconds: 18_000 } },
  { key: 'rate_limit.secondary_window', label: 'Weekly', scope: 'window', usedPercent: 22, window: { seconds: 604_800 } },
]
const balanceRow = (data: Partial<UsageQuotaRow>): UsageQuotaRow => ({
  key: 'credits.balance', label: 'Credits Balance', scope: 'credits', metric: 'credits', ...data,
})
const credential = (quota: UsageQuotaRow[]) => buildAuthFileCredentialRows([identity], new Map([
  [identity.identity, { id: identity.identity, quota, rateLimitResetCreditsAvailableCount: 3 }],
]))[0]

describe('Codex credit balance', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(async () => {
    await i18n.changeLanguage('en')
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
    await i18n.changeLanguage('en')
  })

  it.each([12345.6789012, 0])('keeps balance %s visible beside both main windows in list and detail', async (remaining) => {
    const row = credential([...windows, balanceRow({ remaining })])
    expect(row.displayQuotas).toHaveLength(3)
    expect(row.quotaResetCreditsAvailableCount).toBe(3)
    expect(row.displayQuotas[2]).toMatchObject({
      credits: true, remaining, percent: null, barPercent: null,
      used: undefined, limit: undefined, windowSeconds: undefined,
      windowUsage: undefined, billingUsage: undefined, status: 'unknown',
    })

    await act(async () => root.render(<AuthFileCredentialsSection {...createAuthFileSectionProps({ rows: [row], total: 1 })} />))
    expect(container.textContent).toContain('5h')
    expect(container.textContent).toContain('Weekly')
    expect(container.textContent).toContain('Codex Credits Balance')
    expect(container.textContent).toContain(`${remaining.toLocaleString()} Credits`)
    const reset = container.querySelector<HTMLButtonElement>('[aria-label="Reset quota, 3 available"]')
    expect(reset).not.toBeNull()
    expect(reset?.disabled).toBe(false)
    const block = [...container.querySelectorAll<HTMLElement>('[class*="credentialQuotaBarBlock"]')]
      .find((element) => element.textContent?.includes('Codex Credits Balance'))!
    expect(block.querySelector('[class*="credentialQuotaMeta"]')).toBeNull()
    expect(block.querySelector('[class*="credentialQuotaTrack"]')).toBeNull()

    await act(async () => root.render(<CredentialDetailDrawer open selection={{ kind: 'auth-file', row }} onClose={() => undefined} />))
    expect(document.body.textContent).toContain('5h')
    expect(document.body.textContent).toContain('Weekly')
    expect(document.body.textContent).toContain('Codex Credits Balance')
    expect(document.body.textContent).toContain(`${remaining.toLocaleString()} Credits`)
  })

  it.each([
    ['en', 'Codex Credits Balance', 'Unlimited'],
    ['zh', 'Codex 积分余额', '无限额'],
    ['zh-TW', 'Codex 點數餘額', '無限額'],
  ])('localizes an explicit unlimited balance in %s without inventing zero', async (language, label, unlimited) => {
    await i18n.changeLanguage(language)
    const row = credential([balanceRow({ unlimited: true })])
    expect(row.displayQuotas).toHaveLength(1)
    expect(row.displayQuotas[0].remaining).toBeUndefined()
    await act(async () => root.render(<AuthFileQuotaPanel row={row} quotaUsageMode="current" />))
    expect(container.textContent).toBe(`${label}${unlimited}`)
    expect(container.querySelector('[class*="credentialQuotaTrack"]')).toBeNull()
  })

  it.each([{}, { remaining: Number.NaN }, { remaining: Infinity }, { unlimited: false }])('omits missing or invalid balance %j', (data) => {
    const row = credential([...windows, balanceRow(data)])
    expect(row.displayQuotas.map((quota) => quota.key)).toEqual(windows.map((quota) => quota.key))
    expect(row.quotaResetCreditsAvailableCount).toBe(3)
  })

  it('preserves Qoder and WorkBuddy credit labels and known usage metadata', async () => {
    for (const key of ['qoder.plan', 'workbuddy.total']) {
      const row = credential([{ key, label: 'Provider Credits', metric: 'credits', remaining: 0, used: 20, limit: 20 }])
      await act(async () => root.render(<AuthFileQuotaPanel row={row} quotaUsageMode="current" />))
      expect(container.textContent).toContain('Provider Credits')
      expect(container.textContent).toContain('0 Credits')
      expect(container.textContent).toContain('20 / 20')
      expect(container.textContent).not.toContain('Codex Credits Balance')
    }
  })
})
