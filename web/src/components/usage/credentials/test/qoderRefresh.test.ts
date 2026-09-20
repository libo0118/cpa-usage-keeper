import { expect, it } from 'vitest'
import { buildQuotaRefreshSubmissionUpdate } from '../useQuotaRefreshTasks'

it('accepts legacy null task arrays without crashing', () => {
  const response = JSON.parse('{"tasks":null,"rejected":null,"accepted":0,"skipped":1,"limit":1}')
  expect(buildQuotaRefreshSubmissionUpdate(response, 'row')).toEqual({ pendingTasks: [], stateUpdates: {} })
})
