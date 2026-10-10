import { useTranslation } from 'react-i18next'
import type { ReactNode } from 'react'
import type { UsageEvent } from '@/lib/types'
import styles from './RequestModelCell.module.scss'

type RequestModelEvent = Pick<UsageEvent, 'model' | 'model_alias' | 'upstream_response_model' | 'response_model'>

export function getRequestModelDetails(event: RequestModelEvent, t: (key: string) => string) {
  const forwarded = event.model?.trim() || '-'
  const requested = event.model_alias?.trim() || forwarded
  const reported = event.upstream_response_model?.trim() || event.response_model?.trim() || ''
  const different = Boolean(reported && forwarded !== '-' && reported.toLowerCase() !== forwarded.toLowerCase())
  const tooltipLines = [
    `${t('usage_stats.model_requested')}: ${requested}`,
    `${t('usage_stats.model_forwarded')}: ${forwarded}`,
    `${t('usage_stats.model_reported')}: ${reported || t('usage_stats.model_not_reported')}`,
  ]
  if (different) tooltipLines.push(t('usage_stats.model_report_differs'))
  return { requested, reported, different, tooltipLines }
}

export function RequestModelCell({ event, renderText, tooltipManaged = false }: {
  event: RequestModelEvent
  renderText?: (as: 'strong' | 'small', tooltipText: string, children?: ReactNode, className?: string) => ReactNode
  tooltipManaged?: boolean
}) {
  const { t } = useTranslation()
  const { requested, reported, different, tooltipLines } = getRequestModelDetails(event, t)
  const details = tooltipLines.join('\n')
  const reportText = reported ? `${t('usage_stats.model_reported')} ${reported}` : t('usage_stats.model_not_reported')
  const render: NonNullable<typeof renderText> = renderText ?? ((Tag, text, children, className) => <Tag title={tooltipManaged ? undefined : text} className={className}>{children ?? text}</Tag>)
  return (
    <span className={styles.model} role={tooltipManaged ? undefined : 'group'} aria-label={tooltipManaged ? undefined : details} title={tooltipManaged ? undefined : details} data-model-mismatch={different || undefined}>
      {render('strong', details, requested)}
      {render('small', different ? `${reportText}\n${t('usage_stats.model_report_differs')}` : reportText, <>{reportText}{different ? ' ≠' : ''}</>, styles.reported)}
    </span>
  )
}
