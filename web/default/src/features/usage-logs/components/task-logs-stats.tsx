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

import { Skeleton } from '@/components/ui/skeleton'
import { useIsAdmin } from '@/hooks/use-admin'
import { formatLogQuota } from '@/lib/format'
import { cn } from '@/lib/utils'

import { getTaskLogStats, getUserTaskLogStats } from '../api'
import { buildTaskStatParams } from '../lib/utils'
import type { TaskLogStatistics } from '../types'
import { useUsageLogsContext } from './usage-logs-provider'

const route = getRouteApi('/_authenticated/usage-logs/$section')

const DEFAULT_TASK_STATS: TaskLogStatistics = {
  success_quota: 0,
  failure_quota: 0,
  running_quota: 0,
}

function StatBadge(props: { label: string; value: string; accent: string }) {
  return (
    <span className='border-border/60 bg-muted/25 inline-flex h-7 items-center gap-2 rounded-md border px-2.5 text-xs shadow-xs'>
      <span className={cn('h-3.5 w-0.5 rounded-full', props.accent)} />
      <span className='text-muted-foreground'>{props.label}</span>
      <span className='text-foreground/85 font-mono font-semibold tabular-nums'>
        {props.value}
      </span>
    </span>
  )
}

export function TaskLogsStats() {
  const { t } = useTranslation()
  const isAdmin = useIsAdmin()
  const searchParams = route.useSearch()
  const { sensitiveVisible } = useUsageLogsContext()

  const { data: stats, isLoading } = useQuery({
    queryKey: ['task-logs-stats', isAdmin, searchParams],
    queryFn: async () => {
      const params = buildTaskStatParams({ searchParams, isAdmin })
      const result = isAdmin
        ? await getTaskLogStats(params)
        : await getUserTaskLogStats(params)
      return result.success
        ? result.data || DEFAULT_TASK_STATS
        : DEFAULT_TASK_STATS
    },
    placeholderData: (previousData) => previousData,
  })

  if (isLoading) {
    return (
      <div className='flex items-center gap-2'>
        <Skeleton className='h-7 w-[130px] rounded-md' />
        <Skeleton className='h-7 w-[130px] rounded-md' />
        <Skeleton className='h-7 w-[130px] rounded-md' />
      </div>
    )
  }

  const mask = (value: number) =>
    sensitiveVisible ? formatLogQuota(value || 0) : '••••'

  return (
    <div className='flex flex-wrap items-center gap-2'>
      <StatBadge
        label={t('Success Cost')}
        value={mask(stats?.success_quota || 0)}
        accent='bg-emerald-500/70'
      />
      <StatBadge
        label={t('Failure Cost')}
        value={mask(stats?.failure_quota || 0)}
        accent='bg-rose-500/70'
      />
      <StatBadge
        label={t('Running Cost')}
        value={mask(stats?.running_quota || 0)}
        accent='bg-sky-500/70'
      />
    </div>
  )
}
