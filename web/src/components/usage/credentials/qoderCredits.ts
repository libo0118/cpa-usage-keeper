import type { UsageEvent } from '@/lib/types'

const creditsFormatter = new Intl.NumberFormat('en-US', {
  minimumFractionDigits: 2, maximumFractionDigits: 2, useGrouping: false,
})

export function qoderCreditsCost(event: UsageEvent): { cost: string; status: string; original?: string } | null {
  const data = event.qoder_credits
  const qoder = event.source_type?.toLowerCase() === 'qoder' ||
    event.model_alias?.toLowerCase().startsWith('qoder/') || data != null
  if (!qoder) return null
  if (data == null) return { cost: '—', status: 'uncollected' }
  const value = data.billable === false ? 0 : data.credits
  if (typeof data.billable !== 'boolean' || typeof value !== 'number' || !Number.isFinite(value) || value < 0) {
    return { cost: '—', status: 'unavailable' }
  }
  const format = (amount: number) => creditsFormatter.format(amount)
  const result: { cost: string; status: string; original?: string } = {
    cost: `${format(value)} Credits`, status: data.billable ? 'charged' : 'free',
  }
  // Only strike out a verified reduction; missing original values are never inferred.
  if (typeof data.original_credits === 'number' && Number.isFinite(data.original_credits) && data.original_credits > value) {
    result.original = format(data.original_credits)
  }
  return result
}
