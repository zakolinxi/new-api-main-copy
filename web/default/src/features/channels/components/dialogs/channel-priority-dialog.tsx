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
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowDown, ArrowUp, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Dialog } from '@/components/dialog'
import {
  getChannelPriorityOrder,
  getChannels,
  reorderChannelPriority,
} from '../../api'
import { channelsQueryKeys } from '../../lib'
import type { ChannelPriorityOrder } from '../../types'

type ChannelPriorityDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

/**
 * Reorders channel routing priority with one-step moves.
 *
 * The backend returns entries in ascending id order, so the list is re-sorted
 * here into the routing order the operator actually sees: priority descending,
 * then id descending. That also matches the channel table's default sort, which
 * keeps the two views from disagreeing.
 *
 * Every move carries the revision read with the list. A concurrent edit makes
 * the move fail instead of silently overwriting it, so a conflict reloads the
 * order and asks the operator to retry.
 */
export function ChannelPriorityDialog({
  open,
  onOpenChange,
}: ChannelPriorityDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [order, setOrder] = useState<ChannelPriorityOrder | null>(null)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)

  const { data: channelsData } = useQuery({
    queryKey: [...channelsQueryKeys.all, 'priority-names'],
    queryFn: () => getChannels({ p: 1, page_size: 1000 }),
    enabled: open,
    staleTime: 30_000,
  })

  const nameOf = useMemo(() => {
    const names = new Map<number, string>()
    for (const channel of channelsData?.data?.items ?? []) {
      names.set(channel.id, channel.name || `#${channel.id}`)
    }
    return names
  }, [channelsData])

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await getChannelPriorityOrder()
      if (res.success && res.data) {
        setOrder(res.data)
      } else {
        toast.error(res.message || t('Failed to load channel priority'))
      }
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to load channel priority')
      )
    } finally {
      setLoading(false)
    }
  }, [t])

  useEffect(() => {
    if (open) {
      void load()
    }
  }, [open, load])

  const sorted = useMemo(() => {
    const items = order?.items ?? []
    return [...items].sort((left, right) => {
      const leftPriority = left.priority ?? 0
      const rightPriority = right.priority ?? 0
      if (leftPriority !== rightPriority) {
        return rightPriority - leftPriority
      }
      return right.id - left.id
    })
  }, [order])

  const move = async (index: number, delta: number) => {
    const current = sorted[index]
    const target = sorted[index + delta]
    if (!current || !target || !order || saving) return
    setSaving(true)
    try {
      const res = await reorderChannelPriority({
        revision: order.revision,
        id: current.id,
        target_id: target.id,
        position: delta < 0 ? 'before' : 'after',
        preview: false,
      })
      if (res.success && res.data) {
        setOrder(res.data)
        // Routing changed, so the table's priority column is now stale.
        queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() })
      } else {
        toast.error(res.message || t('Failed to reorder channels'))
        void load()
      }
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to reorder channels')
      )
      void load()
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Channel Priority Order')}
      description={t(
        'Top to bottom is the actual routing order: earlier wins. Changes apply immediately.'
      )}
      contentClassName='sm:max-w-xl'
      footer={<Button onClick={() => onOpenChange(false)}>{t('Close')}</Button>}
    >
      {loading && !order ? (
        <div className='text-muted-foreground flex items-center justify-center gap-2 py-10 text-sm'>
          <Loader2 className='h-4 w-4 animate-spin' />
          {t('Loading')}
        </div>
      ) : sorted.length === 0 ? (
        <div className='text-muted-foreground py-10 text-center text-sm'>
          {t('No channels')}
        </div>
      ) : (
        <ScrollArea className='max-h-[420px] pr-2'>
          <ul className='divide-y'>
            {sorted.map((item, index) => (
              <li
                key={item.id}
                className='flex items-center gap-2 py-1.5 text-sm'
              >
                <span className='text-muted-foreground w-7 shrink-0 tabular-nums'>
                  {index + 1}
                </span>
                <span className='min-w-0 flex-1 truncate'>
                  {nameOf.get(item.id) || `#${item.id}`}
                </span>
                <span className='text-muted-foreground shrink-0 tabular-nums'>
                  {item.priority ?? 0}
                </span>
                <Button
                  variant='ghost'
                  size='icon'
                  className='size-7'
                  aria-label={t('Move up')}
                  disabled={index === 0 || saving}
                  onClick={() => move(index, -1)}
                >
                  <ArrowUp />
                </Button>
                <Button
                  variant='ghost'
                  size='icon'
                  className='size-7'
                  aria-label={t('Move down')}
                  disabled={index === sorted.length - 1 || saving}
                  onClick={() => move(index, 1)}
                >
                  <ArrowDown />
                </Button>
              </li>
            ))}
          </ul>
        </ScrollArea>
      )}
    </Dialog>
  )
}
