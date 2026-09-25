import { BarChart3, Loader2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import dayjs from '@/lib/dayjs'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { formatQuota } from '@/lib/format'
import { getUserConsumption } from '../api'
import type { User } from '../types'

type Granularity = 'day' | 'week' | 'month'
type Dimension = 'model' | 'group'
type Point = { label: string; value: number; bucket: string; series: string }

const getRange = (g: Granularity) => {
  const end = dayjs().endOf('day')
  const start = g === 'month' ? end.startOf('month').subtract(11, 'month') : g === 'week' ? end.startOf('week').add(1, 'day').subtract(11, 'week') : end.subtract(28, 'day').startOf('day')
  return { start: start.unix(), end: end.unix() }
}

export function UserConsumptionDialog({ user, open, onOpenChange }: { user: User; open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useTranslation()
  const [g, setG] = useState<Granularity>('week')
  const [dimension, setDimension] = useState<Dimension>('model')
  const [points, setPoints] = useState<Point[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => {
    if (!open) return
    let cancelled = false
    const range = getRange(g)
    setLoading(true)
    getUserConsumption(user.username, range.start, range.end).then((res) => {
      if (cancelled) return
      const map = new Map<string, number>()
      for (const item of res.data || []) {
        const date = dayjs(item.created_at * 1000)
        const bucket = g === 'month' ? date.format('YYYY-MM') : g === 'week' ? date.startOf('week').add(1, 'day').format('MM-DD') : date.format('MM-DD')
        const series = dimension === 'model' ? (item.model_name || t('Unknown model')) : (item.use_group || t('Unspecified group'))
        const key = bucket + '|' + series
        map.set(key, (map.get(key) || 0) + (Number(item.quota) || 0))
      }
      setPoints(Array.from(map, ([key, value]) => { const [bucket, series] = key.split('|'); return { bucket, series, label: bucket, value } }))
      setError(res.success ? '' : (res.message || 'Failed to load consumption'))
    }).catch(() => { if (!cancelled) { setError('Failed to load consumption'); setPoints([]) } }).finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [dimension, g, open, t, user.username])
  const total = points.reduce((sum, point) => sum + point.value, 0)
  const buckets = Array.from(new Set(points.map((point) => point.bucket)))
  const series = Array.from(new Set(points.map((point) => point.series)))
  const max = Math.max(...buckets.map((bucket) => points.filter((point) => point.bucket === bucket).reduce((s, p) => s + p.value, 0)), 1)
  return <Dialog open={open} onOpenChange={onOpenChange} title={t('User consumption')} description={user.username} contentClassName='sm:max-w-5xl' contentHeight='min(86vh, 760px)' showCloseButton>
    <div className='space-y-5'>
      <div className='flex flex-wrap items-end justify-between gap-3 rounded-lg border bg-muted/20 p-4'>
        <div><div className='text-muted-foreground text-xs'>{t('Selected period total')}</div><div className='text-2xl font-semibold tabular-nums'>{formatQuota(total)}</div></div>
        <div className='bg-muted/60 inline-flex rounded-lg border p-1'>{(['day', 'week', 'month'] as Granularity[]).map((x) => <Button key={x} size='sm' variant={g === x ? 'default' : 'ghost'} onClick={() => setG(x)}>{x === 'day' ? t('Daily (29 days)') : x === 'week' ? t('Weekly (12 weeks)') : t('Monthly (12 months)')}</Button>)}</div>
      </div>
      <div className='bg-muted/40 grid grid-cols-2 rounded-lg border p-1'>{(['model', 'group'] as Dimension[]).map((x) => <Button key={x} size='sm' variant={dimension === x ? 'default' : 'ghost'} onClick={() => setDimension(x)}>{x === 'model' ? t('By model') : t('By group')}</Button>)}</div>
      {loading ? <div className='text-muted-foreground flex h-[32rem] items-center justify-center gap-2 text-sm'><Loader2 className='size-4 animate-spin' />{t('Loading')}</div> : error ? <div className='text-destructive flex h-[32rem] items-center justify-center text-sm'>{error}</div> : points.length === 0 ? <div className='text-muted-foreground flex h-[32rem] flex-col items-center justify-center gap-2 text-sm'><BarChart3 className='size-8 opacity-40' />{t('No consumption data')}</div> : <div className='space-y-4'><div className='flex min-h-[32rem] items-end gap-3 overflow-x-auto border-b px-4 pt-10'>{buckets.map((bucket) => <div key={bucket} className='flex min-w-24 flex-1 flex-col items-center justify-end gap-2' title={bucket + ': ' + formatQuota(points.filter((p) => p.bucket === bucket).reduce((s, p) => s + p.value, 0))}><div className='flex w-full flex-col justify-end' style={{ height: 380 }}>{series.map((name) => { const value = points.find((p) => p.bucket === bucket && p.series === name)?.value || 0; return value > 0 ? <div key={name} className='bg-primary/80 border-background w-full border-t' style={{ height: Math.max((value / max) * 380, 3) }} title={name + ': ' + formatQuota(value)} /> : null })}</div><span className='text-muted-foreground w-28 truncate text-center text-xs'>{bucket}</span></div>)}</div><div className='flex flex-wrap gap-x-4 gap-y-1 text-xs'>{series.map((name) => <span key={name} className='text-muted-foreground'>{name}</span>)}</div><div className='text-muted-foreground text-xs'>{t('Showing {{count}} time periods', { count: buckets.length })}</div></div>}
    </div>
  </Dialog>
}
