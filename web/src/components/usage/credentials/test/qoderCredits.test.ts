import { describe, expect, it } from 'vitest'
import type { UsageEvent } from '@/lib/types'
import { qoderCreditsCost, workbuddyCreditsCost } from '../qoderCredits'

const base: UsageEvent = {
  timestamp: '2026-09-20T13:00:00+08:00', model: 'lite', model_alias: 'qoder/Lite',
  source: 'Qoder', source_type: 'qoder', failed: false, latency_ms: 100,
  tokens: { input_tokens: 0, output_tokens: 0, reasoning_tokens: 0, cache_read_tokens: 0, cache_creation_tokens: 0, total_tokens: 0 },
}

it('WorkBuddy reports actual Credits without inventing original amount or free status', () => {
 const event = { ...base, source_type: 'workbuddy', model_alias: 'workbuddy/Hy3' }
 expect(workbuddyCreditsCost(event)).toEqual({cost:'—',status:'uncollected'})
 expect(workbuddyCreditsCost({...event,workbuddy_credits:{}})).toEqual({cost:'—',status:'unavailable'})
 expect(workbuddyCreditsCost({...event,workbuddy_credits:{credits:0}})).toEqual({cost:'0.00 Credits',status:'reported'})
 expect(workbuddyCreditsCost({...event,failed:true,workbuddy_credits:{credits:1.005}})).toEqual({cost:'1.01 Credits',status:'reported'})
 expect(workbuddyCreditsCost({...event,workbuddy_credits:{credits:-1}})?.status).toBe('unavailable')
 expect(workbuddyCreditsCost({...event,source_type:'codex',model_alias:'gpt-5'})).toBeNull()
})

describe('Qoder request cost', () => {
  it('separates free, billed, unknown and legacy requests without altering Codex costs', () => {
    expect(qoderCreditsCost(base)).toEqual({ cost: '—', status: 'uncollected' })
    expect(qoderCreditsCost({ ...base, qoder_credits: {} })).toEqual({ cost: '—', status: 'unavailable' })
    expect(qoderCreditsCost({ ...base, qoder_credits: { credits: 0.1179375, billable: false } })).toEqual({ cost: '0.00 Credits', status: 'free' })
    expect(qoderCreditsCost({ ...base, qoder_credits: { credits: 0.009917857, original_credits: 0.009917857, billable: false } })).toEqual({ cost: '0.00 Credits', status: 'free', original: '0.01' })
    expect(qoderCreditsCost({ ...base, qoder_credits: { credits: 1.25, original_credits: 2.5, billable: true } })).toEqual({ cost: '1.25 Credits', status: 'charged', original: '2.50' })
    expect(qoderCreditsCost({ ...base, qoder_credits: { credits: 2.5, original_credits: 2.5, billable: true } })?.original).toBeUndefined()
    expect(qoderCreditsCost({ ...base, qoder_credits: { credits: 2.5, original_credits: -1, billable: true } })?.original).toBeUndefined()
    expect(qoderCreditsCost({ ...base, qoder_credits: { credits: 2.5, original_credits: Infinity, billable: true } })?.original).toBeUndefined()
    expect(qoderCreditsCost({ ...base, failed: true, qoder_credits: { credits: 12.5, billable: true } })).toEqual({ cost: '12.50 Credits', status: 'charged' })
    expect(qoderCreditsCost({ ...base, qoder_credits: { credits: 0.000001, billable: true } })?.cost).toBe('0.00 Credits')
    expect(qoderCreditsCost({ ...base, qoder_credits: { credits: 1.005, billable: true } })?.cost).toBe('1.01 Credits')
    expect(qoderCreditsCost({ ...base, qoder_credits: { credits: 1.3133, billable: true } })?.cost).toBe('1.31 Credits')
    expect(qoderCreditsCost({ ...base, qoder_credits: { credits: 0, billable: true } })).toEqual({ cost: '0.00 Credits', status: 'charged' })
    for (const credits of [-1, NaN, Infinity]) {
      expect(qoderCreditsCost({ ...base, qoder_credits: { credits, billable: true } })?.status).toBe('unavailable')
    }
    expect(qoderCreditsCost({ ...base, qoder_credits: { credits: 1 } })?.status).toBe('unavailable')
    expect(qoderCreditsCost({ ...base, source_type: 'codex', model_alias: 'gpt-5', cost_available: true, cost_usd: 0.0037 })).toBeNull()
  })
})
