import { useTranslation } from 'react-i18next'
import type { ReactNode } from 'react'
import type { UsageEvent } from '@/lib/types'
import styles from './RequestModelCell.module.scss'

export function RequestModelCell({ event, renderText }: {
  event: Pick<UsageEvent, 'model' | 'model_alias' | 'upstream_response_model'>
  renderText?: (as: 'strong' | 'small', tooltipText: string, children?: ReactNode, className?: string) => ReactNode
}) {
  const { t } = useTranslation()
  const forwarded = event.model?.trim() || '-'
  const requested = event.model_alias?.trim() || forwarded
  const reported = event.upstream_response_model?.trim() || ''
  const different = Boolean(reported && forwarded !== '-' && reported.toLowerCase() !== forwarded.toLowerCase())
  const details = `${t('usage_stats.model_requested')}: ${requested}\n${t('usage_stats.model_forwarded')}: ${forwarded}\n${t('usage_stats.model_reported')}: ${reported || t('usage_stats.model_not_reported')}`
  const reportText = reported ? `${t('usage_stats.model_reported')} ${reported}` : t('usage_stats.model_not_reported')
  const render: NonNullable<typeof renderText> = renderText ?? ((Tag, text, children, className) => <Tag title={text} className={className}>{children ?? text}</Tag>)
  return (
    <span className={styles.model} role="group" aria-label={details} title={details} data-model-mismatch={different || undefined}>
      {render('strong', details, requested)}
      {render('small', different ? `${reportText}\n${t('usage_stats.model_report_differs')}` : reportText, <>{reportText}{different ? ' ≠' : ''}</>, styles.reported)}
    </span>
  )
}
