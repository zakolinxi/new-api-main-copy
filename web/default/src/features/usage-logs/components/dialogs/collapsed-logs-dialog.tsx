/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import {
  formatLogQuota,
  formatTimestampToDate,
  formatTokens,
  formatUseTime,
} from '@/lib/format'
import { cn } from '@/lib/utils'
import { getAllLogs, getUserLogs } from '../../api'
import type { UsageLog } from '../../data/schema'
import { parseLogOther } from '../../lib/format'
import { buildApiParams } from '../../lib/utils'

const route = getRouteApi('/_authenticated/usage-logs/$section')

const MEMBER_PAGE_SIZE = 50

interface CollapsedLogsDialogProps {
  /** The folded row to expand; null keeps the dialog closed. */
  log: UsageLog | null
  isAdmin: boolean
  open: boolean
  onOpenChange: (open: boolean) => void
}

/**
 * Expansion target for a folded usage log row. The folded row only carries the
 * aggregate, so the individual calls are fetched on demand with the filters the
 * list is already using (same time range, channel, group).
 */
export function CollapsedLogsDialog({
  log,
  isAdmin,
  open,
  onOpenChange,
}: CollapsedLogsDialogProps) {
  const { t } = useTranslation()
  const searchParams = route.useSearch()

  const usernameMissing = isAdmin && !log?.username

  const { data, isLoading } = useQuery({
    queryKey: [
      'log-collapsed-members',
      log?.collapsed_key,
      isAdmin,
      searchParams,
    ],
    enabled: open && !!log && !usernameMissing,
    queryFn: async () => {
      const params = buildApiParams({
        page: 1,
        pageSize: MEMBER_PAGE_SIZE,
        searchParams,
        columnFilters: [],
        isAdmin,
      })
      // Members are the raw calls the folded row summarises, never a folded view.
      delete params.collapse
      delete params.collapse_scan
      params.model_name = log?.model_name
      if (isAdmin) params.username = log?.username
      const result = isAdmin
        ? await getAllLogs(params)
        : await getUserLogs(params)
      return result?.data
    },
  })

  const members = (data?.items ?? []) as UsageLog[]
  const total = data?.total ?? 0

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Merged calls')}
      description={
        log
          ? `${log.username || t('Unknown')} · ${log.model_name}`
          : undefined
      }
      contentClassName='sm:max-w-4xl'
      bodyClassName='space-y-3'
    >
      <p className='text-muted-foreground text-xs'>
        {t(
          'This row merges {{count}} calls to the same model. Tokens, cache and cost are summed, while time, IP and first-token latency follow the newest call.',
          { count: log?.collapsed_count ?? 0 }
        )}
      </p>

      {usernameMissing ? (
        <p className='text-muted-foreground text-sm'>
          {t(
            'This log predates the username column, so its individual calls cannot be listed.'
          )}
        </p>
      ) : isLoading ? (
        <p className='text-muted-foreground text-sm'>{t('Loading...')}</p>
      ) : members.length === 0 ? (
        <p className='text-muted-foreground text-sm'>{t('No Logs Found')}</p>
      ) : (
        <div className='max-h-[calc(82vh-170px)] overflow-auto rounded-md border'>
          <table className='w-full text-xs'>
            <thead className='bg-muted/50 sticky top-0 text-left'>
              <tr className='[&>th]:text-muted-foreground [&>th]:px-3 [&>th]:py-2 [&>th]:font-medium'>
                <th className='whitespace-nowrap'>{t('Time')}</th>
                <th className='whitespace-nowrap'>{t('Tokens')}</th>
                <th className='whitespace-nowrap'>{t('First Token')}</th>
                <th className='whitespace-nowrap'>{t('IP')}</th>
                <th className='whitespace-nowrap'>{t('Cost')}</th>
              </tr>
            </thead>
            <tbody>
              {members.map((member) => {
                const other = parseLogOther(member.other || '')
                const cacheRead = other?.cache_tokens || 0
                const cacheWrite =
                  (other?.cache_creation_tokens_5m || 0) +
                  (other?.cache_creation_tokens_1h || 0) ||
                  other?.cache_creation_tokens ||
                  0
                const firstToken = other?.frt
                return (
                  <tr
                    key={`${member.id}-${member.created_at}`}
                    className='[&>td]:px-3 [&>td]:py-2 border-t'
                  >
                    <td className='whitespace-nowrap font-mono tabular-nums'>
                      {formatTimestampToDate(member.created_at)}
                    </td>
                    <td className='whitespace-nowrap font-mono tabular-nums'>
                      <div className='flex flex-col gap-0.5'>
                        <span>
                          {formatTokens(member.prompt_tokens || 0)} /{' '}
                          {formatTokens(member.completion_tokens || 0)}
                        </span>
                        {(cacheRead > 0 || cacheWrite > 0) && (
                          <span className='text-muted-foreground/60 flex items-center gap-1 text-[11px]'>
                            {cacheRead > 0 && (
                              <span>
                                {t('Cache')}↓ {formatTokens(cacheRead)}
                              </span>
                            )}
                            {cacheWrite > 0 && (
                              <span>↑ {formatTokens(cacheWrite)}</span>
                            )}
                          </span>
                        )}
                      </div>
                    </td>
                    <td
                      className={cn(
                        'whitespace-nowrap font-mono tabular-nums',
                        firstToken != null && firstToken > 0
                          ? 'text-foreground'
                          : 'text-muted-foreground/60'
                      )}
                    >
                      {firstToken != null && firstToken > 0
                        ? formatUseTime(firstToken / 1000)
                        : '-'}
                    </td>
                    <td className='text-muted-foreground whitespace-nowrap font-mono'>
                      {member.ip || '-'}
                    </td>
                    <td className='whitespace-nowrap font-mono tabular-nums'>
                      {formatLogQuota(member.quota || 0)}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}

      {total > MEMBER_PAGE_SIZE && (
        <div className='flex items-center gap-2'>
          <StatusBadge
            label={t('Latest {{count}} of {{total}}', {
              count: MEMBER_PAGE_SIZE,
              total,
            })}
            variant='neutral'
            size='sm'
            copyable={false}
          />
        </div>
      )}
    </Dialog>
  )
}
