import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import { LoadingSpinner } from '@/components/ui/LoadingSpinner';
import { EmptyState } from '@/components/ui/EmptyState';
import { Select } from '@/components/ui/Select';
import { ProviderBrandIcon } from '@/components/ProviderBrandIcon';
import { CredentialBadge } from '@/components/usage/credentials/CredentialSectionShell';
import { ApiError } from '@/lib/api';
import { fetchKeyBudgets, keyPolicyLink, type KeyBudgetResponse } from './api';
import styles from './KeyBudgetsPage.module.scss';

export function KeyBudgetReport({ data }: { data: KeyBudgetResponse }) {
  const { t, i18n } = useTranslation();
  const [selectedKey, setSelectedKey] = useState('');
  const keys = data.report.keys ?? [];
  const visibleKeys = keys.filter((key) => !selectedKey || key.key_id === selectedKey);
  const date = (value?: string) => !value || value.startsWith('0001-') || !Number.isFinite(Date.parse(value)) ? '—' : new Date(value).toLocaleString(i18n.language);
  const money = (value: string | null) => value === null ? t('key_budgets.unlimited') : `$${value}`;
  const label = (group: string, value: string) => t(`key_budgets.${group}.${value}`, { defaultValue: value });
  return <>
    <div className={styles.toolbar}>
      <Select value={selectedKey} onChange={setSelectedKey} ariaLabel={t('key_budgets.filter_key')} className={styles.keyFilter}
        options={[{ value: '', label: t('key_budgets.all_keys', { count: keys.length }) }, ...keys.map((key) => ({ value: key.key_id, label: key.label ? `${key.label} · ${key.key_preview}` : key.key_preview }))]} />
      <span className={styles.syncStatus}>{t('key_budgets.sync')} <CredentialBadge>{label('statuses', data.sync.status || 'pending')}</CredentialBadge></span>
    </div>
    <div className={styles.sync}>
      <span>{t('key_budgets.last_success')}: {date(data.sync.last_success)}</span>
      <span>{t('key_budgets.prices_updated')}: {date(data.report.prices_updated_at)}</span>
    </div>
    {data.sync.status !== 'ready' && data.sync.status !== 'ok' && data.sync.status !== 'synced' && <p role="status" className={styles.warning}>{t('key_budgets.sync_warning')}</p>}
    {!!data.sync.unavailable_models?.length && <p className={styles.warning}>{t('key_budgets.price_warning')} {data.sync.unavailable_models.join(', ')}</p>}
    {!visibleKeys.length && <EmptyState title={t('key_budgets.empty')} />}
    {visibleKeys.map((key) => {
      const budgets = (data.report.budgets ?? []).filter((row) => row.key_id === key.key_id);
      return <section key={key.key_id} className={styles.key}>
        <div className={styles.keyHeader}>
          <h4>{key.label || key.key_preview} {key.label && <code>{key.key_preview}</code>}</h4>
          <p className={styles.hint}>{t(key.allow_all ? 'key_budgets.allow_all' : 'key_budgets.selected')}</p>
        </div>
        {!budgets.length ? <EmptyState title={t('key_budgets.no_budgets')} /> : <div className={styles.scroll} tabIndex={0} role="region" aria-label={key.label || key.key_preview}>
          <table><thead><tr>{['resource', 'period', 'limit', 'used', 'reserved', 'remaining', 'reset', 'status'].map((column) => <th key={column} scope="col">{t(`key_budgets.${column}`)}</th>)}</tr></thead>
            <tbody>{budgets.map((row, index) => {
              const resource = (data.report.resources ?? []).find((item) => item.resource_id === row.resource_id);
              return <tr key={`${row.resource_id}-${row.period}-${row.cycle_start}-${index}`}>
                <td className={styles.resourceCell}><div className={styles.resourceIdentity}><ProviderBrandIcon providerType={resource?.provider} size={30} /><div className={styles.resourceText}><strong>{resource?.label || row.resource_id}</strong><small>{resource?.provider} · {resource?.kind === 'oauth' ? 'OAuth' : t('key_budgets.upstream_key')}</small><code>{row.resource_id}</code>{resource?.disabled && <small><CredentialBadge>{t('key_budgets.disabled')}</CredentialBadge></small>}</div></div></td>
                <td data-label={t('key_budgets.period')}>{label('periods', row.period)}</td>
                <td data-label={t('key_budgets.limit')}>{money(row.limit_usd)}</td>
                <td data-label={t('key_budgets.used')} className={styles.amount}>{money(row.used_usd)}</td>
                <td data-label={t('key_budgets.reserved')}>{money(row.reserved_usd)}</td>
                <td data-label={t('key_budgets.remaining')} className={styles.amount}>{money(row.remaining_usd)}</td>
                <td data-label={t('key_budgets.reset')}>{date(row.reset_at)}</td>
                <td data-label={t('key_budgets.status')}><CredentialBadge tone={row.status === 'exhausted' ? 'danger' : 'neutral'}>{label('statuses', row.status)}</CredentialBadge>{row.unpriced_requests > 0 && <small className={styles.warning}>{t('key_budgets.unpriced', { count: row.unpriced_requests })}</small>}</td>
              </tr>;
            })}</tbody>
          </table>
        </div>}
      </section>;
    })}
  </>;
}

export function KeyBudgetsPage({ cpaURL, refreshKey = 0, onAuthRequired }: { cpaURL: string; refreshKey?: number; onAuthRequired?: () => void }) {
  const { t } = useTranslation();
  const [retry, setRetry] = useState(0);
  const [result, setResult] = useState<{ refreshKey: number; retry: number; data: KeyBudgetResponse | null } | null>(null);
  const loading = !result || result.refreshKey !== refreshKey || result.retry !== retry;
  const data = result?.data;
  useEffect(() => {
    const controller = new AbortController();
    void fetchKeyBudgets(controller.signal).then((result) => {
      if (!controller.signal.aborted) setResult({ refreshKey, retry, data: result });
    }).catch((cause: unknown) => {
      if (controller.signal.aborted) return;
      setResult({ refreshKey, retry, data: null });
      if (cause instanceof ApiError && cause.status === 401) onAuthRequired?.();
    });
    return () => controller.abort();
  }, [refreshKey, retry, onAuthRequired]);
  const href = keyPolicyLink(cpaURL);
  return <Card className={styles.page} title={t('key_budgets.title')} subtitle={t('key_budgets.read_only')} extra={href && <a className="btn btn-secondary btn-action btn-sm" href={href} target="_blank" rel="noopener noreferrer">{t('key_budgets.open_cpa')} ↗</a>}>
    <p className={styles.hint}>{t('key_budgets.soft_cap')}</p>
    {loading ? <div role="status" className={styles.loading}><LoadingSpinner /> {t('common.loading')}</div> : !data ? <div role="alert"><EmptyState title={t('key_budgets.error')} action={<Button variant="secondary" size="sm" onClick={() => setRetry((value) => value + 1)}>{t('common.retry')}</Button>} /></div> : <KeyBudgetReport data={data} />}
  </Card>;
}
