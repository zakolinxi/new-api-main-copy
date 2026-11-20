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
import { useState } from 'react'
import { getRouteApi } from '@tanstack/react-router'
import { Download, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useIsAdmin } from '@/hooks/use-admin'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { exportLogs } from '../api'

const route = getRouteApi('/_authenticated/usage-logs/$section')

function toSeconds(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value)
    ? Math.floor(value / 1000)
    : undefined
}

function firstSelectedType(value: unknown): number | undefined {
  // The list query already honours a single type (both the UI filter and the
  // backend read one value), so export matches the rows on screen.
  const selected = Array.isArray(value) ? value[0] : value
  if (selected == null || selected === '') return undefined
  const parsed = Number(selected)
  return Number.isFinite(parsed) ? parsed : undefined
}

/**
 * Downloads the logs matching the filters currently in the URL as NDJSON.
 *
 * The server keeps the export bounded (one export per user, a maximum time
 * range and a row cap), so this is a plain download button rather than a
 * progress-driven job: a failure comes back as a JSON body and is surfaced as
 * a toast instead of a corrupt file.
 */
export function ExportLogsButton() {
  const { t } = useTranslation()
  const isAdmin = useIsAdmin()
  const searchParams = route.useSearch()
  const [exporting, setExporting] = useState(false)

  const handleExport = async () => {
    if (exporting) return
    setExporting(true)
    try {
      const blob = await exportLogs(
        {
          type: firstSelectedType(searchParams.type),
          start_timestamp: toSeconds(searchParams.startTime),
          end_timestamp: toSeconds(searchParams.endTime),
          model_name: searchParams.model,
          token_name: searchParams.token,
          group: searchParams.group,
          request_id: searchParams.requestId,
          upstream_request_id: searchParams.upstreamRequestId,
          username: isAdmin ? searchParams.username : undefined,
          channel: isAdmin ? searchParams.channel : undefined,
        },
        isAdmin
      )

      // A rejected export answers with the usual JSON envelope, which axios
      // hands back as a blob because of responseType. Detecting it here keeps
      // the user from saving an error page as .ndjson.
      if (blob.type.includes('application/json')) {
        const payload = JSON.parse(await blob.text()) as { message?: string }
        toast.error(payload.message || t('Failed to export logs'))
        return
      }

      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = `usage-logs-${new Date()
        .toISOString()
        .replace(/[:.]/g, '-')}.ndjson`
      document.body.appendChild(link)
      link.click()
      link.remove()
      URL.revokeObjectURL(url)
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to export logs')
      )
    } finally {
      setExporting(false)
    }
  }

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant='ghost'
            size='icon'
            onClick={handleExport}
            disabled={exporting}
            aria-label={t('Export Logs')}
            className='text-muted-foreground hover:text-foreground size-7'
          />
        }
      >
        {exporting ? <Loader2 className='animate-spin' /> : <Download />}
      </TooltipTrigger>
      <TooltipContent>{t('Export Logs')}</TooltipContent>
    </Tooltip>
  )
}
