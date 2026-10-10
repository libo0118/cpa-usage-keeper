import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import { RequestModelCell } from '../RequestModelCell'

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }))

describe('request model provenance', () => {
  it('shows a reported difference without replacing the client model', () => {
    const html = renderToStaticMarkup(<RequestModelCell event={{model:'gpt-6-astra', model_alias:'client-alias', upstream_response_model:'gpt-5.6-luna'}} />)
    expect(html).toContain('>client-alias</strong>')
    expect(html).toContain('gpt-5.6-luna')
    expect(html).toContain('data-model-mismatch="true"')
    expect(html).toContain('model_forwarded: gpt-6-astra')
  })
  it('does not flag a display alias or invent missing historical metadata', () => {
    const matching = renderToStaticMarkup(<RequestModelCell event={{model:'smodel', model_alias:'qoder/Sonus', upstream_response_model:'smodel'}} />)
    expect(matching).not.toContain('data-model-mismatch')
    const missing = renderToStaticMarkup(<RequestModelCell event={{model:'sent'}} />)
    expect(missing).toContain('usage_stats.model_not_reported')
    expect(missing).not.toContain('data-model-mismatch')
  })
  it('prefers the custom report and falls back to legacy upstream reports without duplicate metadata', () => {
    const both = renderToStaticMarkup(<RequestModelCell event={{ model: 'sent', upstream_response_model: 'actual', response_model: 'legacy' }} tooltipManaged />)
    expect(both).toContain('actual')
    expect(both).not.toContain('legacy')
    expect(both).not.toContain('title=')
    expect(both).not.toContain('aria-label=')
    const legacy = renderToStaticMarkup(<RequestModelCell event={{ model: 'sent', response_model: 'legacy' }} />)
    expect(legacy).toContain('legacy')
    const blankCustom = renderToStaticMarkup(<RequestModelCell event={{ model: 'sent', upstream_response_model: ' ', response_model: 'legacy' }} />)
    expect(blankCustom).toContain('legacy')
  })
})
