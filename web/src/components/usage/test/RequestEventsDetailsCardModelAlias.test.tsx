import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { RequestEventsTestCard, extractTableHeaders, extractFirstTableRowCells } from './requestEventsFixtures';
import type { RequestEventsDetailsCard } from '../RequestEventsDetailsCard';
import type { UsageEvent } from '@/lib/types';

const events: UsageEvent[] = [
  {
    id: '101',
    timestamp: '2026-04-23T02:00:00.000Z',
    api_key: 'Production Key',
    model: 'claude-sonnet',
    model_alias: 'sonnet-business',
    source: 'Provider A',
    source_raw: 'source-a',
    source_type: 'openai',
    auth_index: '1',
    failed: false,
    latency_ms: 120,
    ttft_ms: 45,
    speed_tps: 30,
    tokens: {
      input_tokens: 100,
      output_tokens: 60,
      reasoning_tokens: 20,
      cache_read_tokens: 20,
      cache_creation_tokens: 0,
      total_tokens: 200,
    },
    cost_usd: 0.1234,
    cost_available: true,
    pricing_style: 'claude',
  },
];

const renderCard = (props: Partial<React.ComponentProps<typeof RequestEventsDetailsCard>> = {}) =>
  renderToStaticMarkup(
    <RequestEventsTestCard
      events={events}
      modelOptions={['claude-sonnet']}
      sourceOptions={[{ value: 'source-a', label: 'Provider A' }]}
      {...props}
    />,
  );

describe('RequestEventsDetailsCard model stack', () => {
  it('shows the requested alias and preserves unknown upstream provenance', () => {
    const html = renderCard();
    const headers = extractTableHeaders(html);
    const cells = extractFirstTableRowCells(html);
    const modelHeaderIndex = headers.indexOf('Model');
    const effortHeaderIndex = headers.indexOf('Effort');

    expect(modelHeaderIndex).toBeGreaterThanOrEqual(0);
    expect(effortHeaderIndex).toBeGreaterThanOrEqual(0);
    expect(modelHeaderIndex).toBeLessThan(effortHeaderIndex);
    expect(headers).not.toContain('Model Alias');
    expect(cells[modelHeaderIndex]).toBe('sonnet-businessUpstream did not report a model');
    expect(html).toContain('CPA forwarded model: claude-sonnet');
  });

  it.each(['', 'claude-sonnet'])('hides a missing or duplicate alias %j', (modelAlias) => {
    const html = renderCard({ events: [{ ...events[0], model_alias: modelAlias }] });
    const headers = extractTableHeaders(html);
    const cells = extractFirstTableRowCells(html);
    expect(cells[headers.indexOf('Model')]).toBe('claude-sonnetUpstream did not report a model');
  });

  it('falls back to the upstream response_model field for reporting', () => {
    const html = renderCard({ events: [{ ...events[0], response_model: 'claude-sonnet-4-6' }] });
    const headers = extractTableHeaders(html);
    const cells = extractFirstTableRowCells(html);

    expect(cells[headers.indexOf('Model')]).toBe('sonnet-businessUpstream reported claude-sonnet-4-6 ≠');
  });

  it('does not flag a reported model matching the forwarded model ignoring case', () => {
    const html = renderCard({ events: [{ ...events[0], response_model: ' CLAUDE-SONNET ' }] });
    const headers = extractTableHeaders(html);
    const cells = extractFirstTableRowCells(html);

    expect(cells[headers.indexOf('Model')]).toBe('sonnet-businessUpstream reported CLAUDE-SONNET');
    expect(html).not.toContain('data-model-mismatch');
  });

  it('keeps matching response and alias values in the whole-field tooltip', () => {
    const html = renderCard({
      events: [{ ...events[0], response_model: 'claude-sonnet', model_alias: 'CLAUDE-SONNET' }],
    });
    const headers = extractTableHeaders(html);
    const cells = extractFirstTableRowCells(html);

    expect(cells[headers.indexOf('Model')]).toBe('CLAUDE-SONNETUpstream reported claude-sonnet');
    expect(html).toContain('aria-label="Requested model: CLAUDE-SONNET; CPA forwarded model: claude-sonnet; Upstream reported: claude-sonnet"');
    expect(html).not.toContain('title="Requested model:');
  });
});
