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
import {
  ArrowDownToLine,
  ArrowUpFromLine,
  Braces,
  CheckCircle2,
  ChevronDown,
  ChevronUp,
  FileCode,
  History,
  ListTree,
  Loader2,
  Plus,
  Trash2,
  WalletCards,
  XCircle,
} from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { api, getSelf } from '@/lib/api'
import { formatQuotaWithCurrency } from '@/lib/currency'

import {
  cancelOrder,
  createOrder,
  getLedgerBalances,
  getOrder,
  listAvailableScriptVersions,
  listCategories,
  listScriptOffers,
  quoteOrder,
  rechargeAvailable,
  searchProviderGroups,
  withdrawAvailable,
  type ProviderGroup,
  type ScriptOffer,
} from './api'
import { ClientRelaySession } from './lib/client-relay-session'
import { displayToMicros, formatUnix, microsToCurrency } from './lib/format'
import type {
  LedgerBalances,
  Order,
  PriceBreakdown,
  ScriptVersion,
} from './types'

type PublishedScript = {
  id: number
  title: string
  description?: string
  latest_version?: number
}
type PurchaseDraft = {
  scriptId: number
  version: number
  configText: string
  consumeMultiplier: number
}
type ViewMode = 'form' | 'json'

// QueuedTask tracks one purchase+relay run entirely on the client side.
// Multiple tasks can be in-flight simultaneously; each is independent.
type QueuedTask = {
  localId: string
  scriptId: number
  scriptTitle: string
  version: number
  submittedAt: number
  status: 'submitting' | 'running' | 'success' | 'failed'
  order: Order | null
  relayStatus: string
  relayResult: string
  error?: string
  configText: string
  resultView: ViewMode
  // Set once the on-mount reconcile effect has picked up a reload-interrupted
  // task, so it isn't polled again. Absent on live tasks.
  reconciled?: boolean
}

// ClientTaskRecord is a locally-kept log of one purchase run. The sent config
// and returned result are plaintext that only ever exists in this browser (the
// control plane sees only hashes under E2EE), so the record lives in
// localStorage — there is no backend and no p2p change involved. Kept so the
// buyer can review what they sent and got back.
type ClientTaskRecord = {
  orderId: string
  scriptId: number
  scriptTitle: string
  version: number
  nodeId: string
  configText: string
  result: string
  status: 'SUCCESS' | 'FAILED'
  error?: string
  createdAt: number
}

// Keep the log small so localStorage never grows unbounded.
const TASK_RECORDS_LIMIT = 50

function getTaskRecordsStorageKey() {
  const userId = window.localStorage.getItem('uid') ?? 'anonymous'
  return `aitoken-task-records:${userId}`
}

function loadTaskRecords(): ClientTaskRecord[] {
  try {
    const saved = JSON.parse(
      window.localStorage.getItem(getTaskRecordsStorageKey()) ?? '[]'
    ) as unknown
    return Array.isArray(saved) ? (saved as ClientTaskRecord[]) : []
  } catch {
    return []
  }
}

// upsertTaskRecord prepends (or replaces by orderId) a record and trims to cap.
function upsertTaskRecord(
  records: ClientTaskRecord[],
  record: ClientTaskRecord
): ClientTaskRecord[] {
  const withoutDup = records.filter((r) => r.orderId !== record.orderId)
  return [record, ...withoutDup].slice(0, TASK_RECORDS_LIMIT)
}

// The task queue is persisted to localStorage so a refresh doesn't lose it.
// Cap it so storage never grows unbounded.
const TASK_QUEUE_LIMIT = 30

function getTaskQueueStorageKey() {
  const userId = window.localStorage.getItem('uid') ?? 'anonymous'
  return `aitoken-task-queue:${userId}`
}

// loadTaskQueue restores the persisted queue. Any task that was still in-flight
// (submitting/running) when the page unloaded can't be resumed — its relay
// socket is gone — so it is restored as interrupted. The relay result plaintext
// only ever lived in the old page's memory, so it can't be recovered; but the
// order itself keeps settling server-side (the provider already ran it), so the
// on-mount reconcile effect re-queries each order's real terminal state and
// rewrites the card to success/refunded accordingly.
function loadTaskQueue(): QueuedTask[] {
  try {
    const saved = JSON.parse(
      window.localStorage.getItem(getTaskQueueStorageKey()) ?? '[]'
    ) as unknown
    if (!Array.isArray(saved)) return []
    return (saved as QueuedTask[]).map((task) =>
      task.status === 'submitting' || task.status === 'running'
        ? { ...task, status: 'failed', relayStatus: '', error: task.error || 'Interrupted by page reload' }
        : task
    )
  } catch {
    return []
  }
}

const DEFAULT_CONFIG_TEXT = '{\n  "prompt": "a dog"\n}'
const OFFERS_PAGE_SIZE = 10

function configTextFromParams(scriptParams: string | undefined): string {
  const raw = (scriptParams ?? '').trim()
  if (!raw) return DEFAULT_CONFIG_TEXT
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

function getDraftStorageKey() {
  const userId = window.localStorage.getItem('uid') ?? 'anonymous'
  return `aitoken-purchase-draft:${userId}`
}

function getViewModeStorageKey(view: 'parameters') {
  const userId = window.localStorage.getItem('uid') ?? 'anonymous'
  return `aitoken-purchase-${view}-view:${userId}`
}

function loadViewMode(view: 'parameters'): ViewMode {
  try {
    return window.localStorage.getItem(getViewModeStorageKey(view)) === 'json'
      ? 'json'
      : 'form'
  } catch {
    return 'form'
  }
}

function updateJsonValue(
  source: unknown,
  path: (string | number)[],
  value: unknown
): unknown {
  if (path.length === 0) return value
  const [key, ...rest] = path
  const container = Array.isArray(source)
    ? [...source]
    : { ...(source as Record<string, unknown>) }
  container[key as never] = updateJsonValue(
    (source as Record<string | number, unknown>)?.[key],
    rest,
    value
  ) as never
  return container
}

function removeJsonArrayItem(
  source: unknown,
  path: (string | number)[],
  index: number
): unknown {
  const target = path.reduce<unknown>(
    (value, key) => (value as Record<string | number, unknown>)?.[key],
    source
  )
  if (!Array.isArray(target)) return source
  return updateJsonValue(
    source,
    path,
    target.filter((_, itemIndex) => itemIndex !== index)
  )
}

function appendJsonArrayItem(
  source: unknown,
  path: (string | number)[]
): unknown {
  const target = path.reduce<unknown>(
    (value, key) => (value as Record<string | number, unknown>)?.[key],
    source
  )
  if (!Array.isArray(target)) return source
  return updateJsonValue(source, path, [...target, ''])
}

function isEmptyArrayItem(value: unknown): boolean {
  if (value == null) return true
  if (typeof value === 'string') return value.trim() === ''
  if (Array.isArray(value)) return value.length === 0
  if (typeof value === 'object') {
    return Object.values(value).every(isEmptyArrayItem)
  }
  return false
}

function cleanEmptyArrayItems(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value
      .map(cleanEmptyArrayItems)
      .filter((item) => !isEmptyArrayItem(item))
  }
  if (value !== null && typeof value === 'object') {
    return Object.fromEntries(
      Object.entries(value).map(([key, item]) => [
        key,
        cleanEmptyArrayItems(item),
      ])
    )
  }
  return value
}

type JsonFormProps = {
  value: unknown
  onChange?: (path: (string | number)[], value: unknown) => void
  path?: (string | number)[]
  // compact: smaller text + tighter row spacing for read-only result display
  compact?: boolean
}

function JsonForm(props: JsonFormProps) {
  const { t } = useTranslation()
  const path = props.path ?? []
  const compact = props.compact ?? false
  if (Array.isArray(props.value)) {
    return (
      <div className='space-y-1'>
        {props.value.map((value, index) => (
          <div
            // eslint-disable-next-line react/no-array-index-key
            key={index}
            className='grid grid-cols-[2rem_minmax(0,1fr)_2rem] items-start gap-2'
          >
            <span className='text-muted-foreground pt-2 text-xs tabular-nums'>
              {index + 1}
            </span>
            <JsonForm value={value} path={[...path, index]} onChange={props.onChange} compact={compact} />
            {props.onChange ? (
              <Button
                type='button'
                size='icon-sm'
                variant='ghost'
                className='text-muted-foreground hover:text-destructive'
                title={t('Remove item')}
                aria-label={t('Remove item {{index}}', { index: index + 1 })}
                onClick={() =>
                  props.onChange?.(path, removeJsonArrayItem(props.value, [], index))
                }
              >
                <Trash2 className='h-4 w-4' aria-hidden='true' />
              </Button>
            ) : (
              <span />
            )}
          </div>
        ))}
        {props.onChange && (
          <Button
            type='button'
            size='sm'
            variant='outline'
            className='ml-10 w-[calc(100%_-_2.5rem)] border-dashed'
            onClick={() => props.onChange?.(path, appendJsonArrayItem(props.value, []))}
          >
            <Plus className='mr-2 h-4 w-4' />
            {t('Add item')}
          </Button>
        )}
      </div>
    )
  }

  if (props.value !== null && typeof props.value === 'object') {
    return (
      <div className='divide-y'>
        {Object.entries(props.value).map(([key, value]) => (
          <div
            key={key}
            className={compact
              ? 'grid gap-1 py-1 first:pt-0 last:pb-0 md:grid-cols-[minmax(6rem,9rem)_minmax(0,1fr)] md:gap-2'
              : 'grid gap-2 py-2.5 first:pt-0 last:pb-0 md:grid-cols-[minmax(8rem,11rem)_minmax(0,1fr)] md:gap-3'}
          >
            <div className={compact ? 'pt-0.5' : 'pt-1.5'}>
              <span className={compact ? 'text-muted-foreground text-[11px] break-words' : 'text-muted-foreground text-sm break-words'}>{key}</span>
            </div>
            <JsonForm value={value} path={[...path, key]} onChange={props.onChange} compact={compact} />
          </div>
        ))}
      </div>
    )
  }
  if (!props.onChange) {
    return (
      <div className={compact ? 'py-0.5 text-[11px] leading-4 break-words whitespace-pre-wrap' : 'min-h-9 py-1.5 text-sm break-words whitespace-pre-wrap'}>
        {props.value === null ? 'null' : String(props.value)}
      </div>
    )
  }
  if (typeof props.value === 'boolean') {
    return (
      <input
        type='checkbox'
        className='mt-2 h-4 w-4'
        checked={props.value}
        onChange={(event) => props.onChange?.(path, event.target.checked)}
      />
    )
  }
  const fieldName = path.at(-1)
  if (typeof fieldName === 'string' && fieldName.toLowerCase() === 'prompt') {
    return (
      <Textarea
        className='min-h-24 resize-y leading-6'
        value={props.value === null ? '' : String(props.value)}
        onChange={(event) => props.onChange?.(path, event.target.value)}
      />
    )
  }
  return (
    <Input
      type={typeof props.value === 'number' ? 'number' : 'text'}
      value={props.value === null ? '' : String(props.value)}
      onChange={(event) => {
        let value: string | number = event.target.value
        if (typeof props.value === 'number') {
          value = event.target.value === '' ? 0 : event.target.valueAsNumber
        }
        props.onChange?.(path, value)
      }}
    />
  )
}

function loadPurchaseDraft(): PurchaseDraft {
  try {
    const saved = JSON.parse(
      window.localStorage.getItem(getDraftStorageKey()) ?? '{}'
    ) as Partial<PurchaseDraft>
    return {
      scriptId:
        Number.isInteger(saved.scriptId) && (saved.scriptId ?? 0) > 0
          ? (saved.scriptId ?? 0)
          : 0,
      version:
        Number.isInteger(saved.version) && (saved.version ?? 0) > 0
          ? (saved.version ?? 1)
          : 1,
      configText:
        typeof saved.configText === 'string'
          ? saved.configText
          : DEFAULT_CONFIG_TEXT,
      consumeMultiplier:
        Number.isInteger(saved.consumeMultiplier) &&
        (saved.consumeMultiplier ?? 0) >= 1
          ? (saved.consumeMultiplier ?? 1)
          : 1,
    }
  } catch {
    return { scriptId: 0, version: 1, configText: DEFAULT_CONFIG_TEXT, consumeMultiplier: 1 }
  }
}

const TERMINAL_FAILURE_STATES = new Set([
  'FAILED', 'REFUNDED', 'TIMED_OUT', 'CANCELLED',
])

function describeOrderError(code: string | undefined): string {
  switch (code) {
    case 'ORIGIN_NOT_ALLOWED':
      return 'Provider has no open tab on the target site (origin not allowed).'
    case 'LEASE_EXPIRED':
      return 'The execution lease expired before the provider ran the task.'
    case 'SCRIPT_EXECUTION_FAILED':
      return 'The script failed while running on the provider.'
    case 'SCRIPT_NOT_FOUND':
    case 'SCRIPT_REVOKED':
      return 'The script version is no longer available for execution.'
    case 'PARAMS_SCHEMA_INVALID':
      return 'The task parameters did not match the script schema.'
    case 'INLINE_CODE_REJECTED':
      return 'The task was rejected by the provider safety gate.'
    case 'TARGET_NOT_READY':
      return 'The provider could not open the target tab.'
    default:
      return code || 'Task failed on the provider'
  }
}

async function sha256Hex(text: string): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(text))
  const hash = [...new Uint8Array(digest)]
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('')
  return `sha256:${hash}`
}

// TaskCard renders one queued task with live status and expandable result.
type TaskCardProps = {
  task: QueuedTask
  onResultViewChange: (localId: string, view: ViewMode) => void
  onCancel: (orderId: string) => void
}
function TaskCard({ task, onResultViewChange, onCancel }: TaskCardProps) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(true)
  const canCancel =
    task.order != null &&
    ['FUNDS_RESERVED', 'MATCHING', 'OFFERED'].includes(task.order.state)

  let statusIcon: React.ReactNode
  if (task.status === 'submitting' || task.status === 'running') {
    statusIcon = <Loader2 className='h-4 w-4 animate-spin text-blue-500' />
  } else if (task.status === 'success') {
    statusIcon = <CheckCircle2 className='h-4 w-4 text-emerald-500' />
  } else {
    statusIcon = <XCircle className='h-4 w-4 text-red-500' />
  }

  let resultNode: React.ReactNode = null
  if (task.relayResult) {
    try {
      const parsed = JSON.parse(task.relayResult) as unknown
      resultNode =
        task.resultView === 'json' ? (
          // whitespace-pre-wrap prevents horizontal overflow; the card stays within its column
          <pre className='bg-muted/30 max-h-60 overflow-auto rounded-md border p-2 text-xs whitespace-pre-wrap break-all'>
            {task.relayResult}
          </pre>
        ) : (
          // compact + scrollable container — result data can be large
          <div className='max-h-60 overflow-y-auto rounded-md border bg-muted/10 p-2'>
            <JsonForm value={parsed} compact />
          </div>
        )
    } catch {
      resultNode = (
        <pre className='bg-muted/30 max-h-60 overflow-auto rounded-md border p-2 text-xs whitespace-pre-wrap break-all'>
          {task.relayResult}
        </pre>
      )
    }
  }

  return (
    <div className='rounded-lg border text-sm overflow-hidden'>
      {/* Header */}
      <div className='flex items-center gap-2 px-3 py-2.5 bg-card'>
        {statusIcon}
        <div className='min-w-0 flex-1'>
          <div className='truncate font-medium'>
            #{task.scriptId} {task.scriptTitle} v{task.version}
          </div>
          {task.relayStatus && (
            <div className='text-muted-foreground truncate text-xs'>{task.relayStatus}</div>
          )}
          {task.error && (
            <div className='truncate text-xs text-red-600'>{task.error}</div>
          )}
        </div>
        <div className='text-muted-foreground shrink-0 text-xs'>
          {new Date(task.submittedAt).toLocaleTimeString()}
        </div>
        <button
          type='button'
          className='text-muted-foreground hover:text-foreground shrink-0 p-1'
          onClick={() => setExpanded((v) => !v)}
          aria-label={expanded ? t('Collapse') : t('Expand')}
        >
          {expanded ? <ChevronUp className='h-4 w-4' /> : <ChevronDown className='h-4 w-4' />}
        </button>
      </div>

      {/* Expanded body — min-w-0 + overflow-hidden prevent result content from blowing card width */}
      {expanded && (
        <div className='min-w-0 overflow-hidden border-t px-3 py-3 space-y-3'>
          {task.error && (
            // break-words + whitespace-pre-wrap so a long error wraps inside the
            // card instead of stretching the page.
            <div className='rounded-md border border-red-200 bg-red-50 p-2 text-xs break-words whitespace-pre-wrap text-red-600 dark:border-red-900/50 dark:bg-red-950/30'>
              {task.error}
            </div>
          )}
          {task.order && (
            <div className='text-xs space-y-1'>
              <div className='font-mono text-muted-foreground truncate'>{task.order.id}</div>
              <div>
                {t('State')}: <b>{task.order.state}</b>
                {task.order.chosen_node_id && (
                  <> · {t('Node')}: <span className='font-mono'>{task.order.chosen_node_id}</span></>
                )}
              </div>
              {canCancel && (
                <Button size='sm' variant='outline' onClick={() => onCancel(task.order!.id)}>
                  {t('Cancel order')}
                </Button>
              )}
            </div>
          )}
          {resultNode && (
            <div className='min-w-0'>
              {/* Header: Result label left, toggle buttons right — shrink-0 keeps buttons in frame */}
              <div className='mb-2 flex items-center gap-2'>
                <span className='text-xs font-medium flex-1'>{t('Result')}</span>
                <div className='flex shrink-0 gap-1' role='group'>
                  <Button
                    type='button' size='sm'
                    variant={task.resultView === 'form' ? 'secondary' : 'ghost'}
                    onClick={() => onResultViewChange(task.localId, 'form')}
                  >
                    <ListTree className='mr-1 h-3 w-3' />{t('Visual')}
                  </Button>
                  <Button
                    type='button' size='sm'
                    variant={task.resultView === 'json' ? 'secondary' : 'ghost'}
                    onClick={() => onResultViewChange(task.localId, 'json')}
                  >
                    <Braces className='mr-1 h-3 w-3' />JSON
                  </Button>
                </div>
              </div>
              {resultNode}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

export function AitokenPurchasePage() {
  const { t } = useTranslation()
  const [initialDraft] = useState(loadPurchaseDraft)
  const [bal, setBal] = useState<LedgerBalances | null>(null)
  const [scripts, setScripts] = useState<PublishedScript[]>([])
  const [scriptId, setScriptId] = useState(initialDraft.scriptId)
  const [version, setVersion] = useState(initialDraft.version)
  const [availableVersions, setAvailableVersions] = useState<ScriptVersion[]>([])
  const versions = availableVersions.map((item) => item.version).sort((a, b) => b - a)
  const selectedScript = scripts.find((s) => s.id === scriptId)
  const [offers, setOffers] = useState<ScriptOffer[]>([])
  const [nodeId, setNodeId] = useState('')
  const [autoSelect, setAutoSelect] = useState(true)
  const [groupQuery, setGroupQuery] = useState('')
  const [groupResults, setGroupResults] = useState<ProviderGroup[]>([])
  const [groupSearching, setGroupSearching] = useState(false)
  const [groupFilterId, setGroupFilterId] = useState('')
  const [groupFilterName, setGroupFilterName] = useState('')
  const [offersPage, setOffersPage] = useState(0)
  const [configText, setConfigText] = useState(initialDraft.configText)
  const [consumeMultiplier, setConsumeMultiplier] = useState(initialDraft.consumeMultiplier)
  const [parametersView, setParametersView] = useState<ViewMode>(() => loadViewMode('parameters'))
  const [quote, setQuote] = useState<PriceBreakdown | null>(null)
  const [offersLoading, setOffersLoading] = useState(false)
  // Multi-task queue: each purchase fires independently, results shown in right panel
  const [taskQueue, setTaskQueue] = useState<QueuedTask[]>(loadTaskQueue)
  const [walletQuota, setWalletQuota] = useState<number | null>(null)
  const [rechargeAmt, setRechargeAmt] = useState('1')
  const [recharging, setRecharging] = useState(false)
  const [withdrawAmt, setWithdrawAmt] = useState('10')
  const [withdrawing, setWithdrawing] = useState(false)
  const [taskRecords, setTaskRecords] = useState<ClientTaskRecord[]>(loadTaskRecords)
  const [recordsOpen, setRecordsOpen] = useState(false)
  const [expandedRecordId, setExpandedRecordId] = useState<string | null>(null)
  // Wallet interactions (recharge/withdraw) live in a dialog to keep the top bar compact.
  const [walletOpen, setWalletOpen] = useState(false)
  // Script description is collapsed to one line by default; expand for the full text.
  const [descExpanded, setDescExpanded] = useState(false)

  function updateTask(localId: string, patch: Partial<QueuedTask>) {
    setTaskQueue((prev) =>
      prev.map((t) => (t.localId === localId ? { ...t, ...patch } : t))
    )
  }

  function addTaskRecord(record: ClientTaskRecord) {
    setTaskRecords((current) => upsertTaskRecord(current, record))
  }

  useEffect(() => {
    try {
      window.localStorage.setItem(getTaskRecordsStorageKey(), JSON.stringify(taskRecords))
    } catch { /* best-effort */ }
  }, [taskRecords])

  // Persist the task queue so a refresh doesn't lose it (trimmed to cap).
  useEffect(() => {
    try {
      window.localStorage.setItem(
        getTaskQueueStorageKey(),
        JSON.stringify(taskQueue.slice(0, TASK_QUEUE_LIMIT))
      )
    } catch { /* best-effort */ }
  }, [taskQueue])

  useEffect(() => {
    try {
      window.localStorage.setItem(
        getDraftStorageKey(),
        JSON.stringify({ scriptId, version, configText, consumeMultiplier } satisfies PurchaseDraft)
      )
    } catch { /* best-effort */ }
  }, [scriptId, version, configText, consumeMultiplier])

  useEffect(() => {
    try { window.localStorage.setItem(getViewModeStorageKey('parameters'), parametersView) }
    catch { /* best-effort */ }
  }, [parametersView])

  async function loadBalance() {
    try {
      const [balances, self] = await Promise.all([getLedgerBalances(), getSelf()])
      setBal(balances)
      const quota = self?.data?.quota
      setWalletQuota(typeof quota === 'number' ? quota : null)
    } catch (e) { toast.error(String((e as Error).message)) }
  }

  async function onRecharge() {
    const amountMicros = displayToMicros(rechargeAmt)
    if (amountMicros <= 0) { toast.error(t('Enter an amount greater than zero')); return }
    setRecharging(true)
    try {
      await rechargeAvailable(amountMicros)
      toast.success(t('Recharged from wallet'))
      await loadBalance()
    } catch (e) { toast.error(String((e as Error).message)) }
    finally { setRecharging(false) }
  }

  async function onWithdraw() {
    const amountMicros = displayToMicros(withdrawAmt)
    if (amountMicros < 10_000_000) { toast.error(t('Minimum withdrawal is 10')); return }
    setWithdrawing(true)
    try {
      const res = await withdrawAvailable(amountMicros)
      toast.success(t('Withdrew to wallet ({{amount}} after 5% fee)', { amount: microsToCurrency(res.net_micros) }))
      await loadBalance()
    } catch (e) { toast.error(String((e as Error).message)) }
    finally { setWithdrawing(false) }
  }

  async function loadOffersFor(
    selectedScriptId: number, selectedVersion: number,
    groupId = groupFilterId, multiplier = consumeMultiplier
  ) {
    setOffersLoading(true); setOffersPage(0)
    try {
      const loaded = await listScriptOffers(selectedScriptId, selectedVersion, groupId || undefined, multiplier)
      const sorted = [...loaded].sort((a, b) => Number(b.owned) - Number(a.owned))
      setOffers(sorted); setAutoSelect(true); setNodeId('')
      try {
        const priced = await quoteOrder({ script_id: selectedScriptId, version: selectedVersion, provider_group_id: groupId || undefined, consume_multiplier: multiplier })
        setQuote(priced.breakdown)
      } catch { setQuote(null) }
      if (loaded.length === 0) toast.info(t('No provider offers yet for this version'))
    } finally { setOffersLoading(false) }
  }

  async function selectScript(value: number, preferredVersion?: number, fallbackVersion?: number, loadParams = true) {
    setScriptId(value); setOffers([]); setNodeId(''); setAutoSelect(true); setOffersPage(0); setQuote(null)
    if (!value) { setAvailableVersions([]); return }
    try {
      const available = await listAvailableScriptVersions(value)
      const values = available.map((item) => item.version).sort((a, b) => b - a)
      setAvailableVersions(available)
      const selectedVersion =
        (preferredVersion && values.includes(preferredVersion) ? preferredVersion : undefined) ??
        values[0] ?? fallbackVersion ?? 1
      setVersion(selectedVersion)
      if (loadParams) {
        const selected = available.find((item) => item.version === selectedVersion)
        setConfigText(configTextFromParams(selected?.script_params))
      }
      await loadOffersFor(value, selectedVersion)
    } catch (e) { toast.error(String((e as Error).message)) }
  }

  async function loadScripts() {
    try {
      const [res, categories] = await Promise.all([
        api.get('/api/scripts/square', { params: { limit: 100 } }),
        listCategories(),
      ])
      const items = (res.data?.data?.items ?? res.data?.items ?? res.data?.data ?? []) as PublishedScript[]
      const balanceScriptIds = new Set(categories.map((c) => c.balance_script_id).filter(Boolean))
      const list = items.filter((s) => !balanceScriptIds.has(s.id))
      setScripts(list)
      const savedScript = list.find((item) => item.id === initialDraft.scriptId)
      if (savedScript) {
        await selectScript(initialDraft.scriptId, initialDraft.version, savedScript.latest_version, false)
      } else if (initialDraft.scriptId) {
        setScriptId(0)
      }
    } catch (e) { toast.error(String((e as Error).message)) }
  }

  useEffect(() => {
    loadBalance()
    loadScripts()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Reconcile tasks interrupted by a page reload against their real backend
  // state. The relay result plaintext is gone, but the order keeps settling
  // server-side: the provider already ran it, so the stale-order sweep pays the
  // node/author and releases the buyer's remainder (or refunds an abandoned
  // one). Poll each interrupted order until it reaches a terminal state so the
  // card stops saying "interrupted" once the money has actually moved.
  useEffect(() => {
    const pending = taskQueue.filter(
      (task) => task.status === 'failed' && task.order?.id && !task.reconciled
    )
    if (pending.length === 0) return
    let cancelled = false
    // Mark them as reconciling up front so the same tasks aren't re-polled.
    for (const task of pending) updateTask(task.localId, { reconciled: true })

    async function reconcileOne(localId: string, orderId: string) {
      // The sweep settles delivered-but-unconfirmed orders ~2min after the
      // interruption; poll a bit longer than that before giving up.
      for (let attempt = 0; attempt < 30 && !cancelled; attempt++) {
        try {
          const latest = await getOrder(orderId)
          updateTask(localId, { order: latest })
          if (latest.state === 'SETTLED') {
            updateTask(localId, {
              status: 'success',
              error: undefined,
              relayStatus: t('Settled after reload (result not available on this device)'),
            })
            await loadBalance()
            return
          }
          if (['REFUNDED', 'CANCELLED', 'TIMED_OUT'].includes(latest.state)) {
            updateTask(localId, {
              status: 'failed',
              error: t('Interrupted by page reload — funds refunded'),
              relayStatus: '',
            })
            await loadBalance()
            return
          }
        } catch { /* transient; keep polling */ }
        await new Promise((r) => window.setTimeout(r, 6000))
      }
    }
    for (const task of pending) void reconcileOne(task.localId, task.order!.id)
    return () => { cancelled = true }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  function onMultiplierChange(raw: string) {
    const parsed = Math.floor(Number(raw))
    const next = Number.isFinite(parsed) && parsed >= 1 ? parsed : 1
    setConsumeMultiplier(next); setQuote(null)
    if (scriptId) void loadOffersFor(scriptId, version, groupFilterId, next)
  }

  async function onQuote() {
    if (!scriptId) { toast.error(t('Select a script first')); return }
    try {
      const q = await quoteOrder({ script_id: scriptId, version, node_id: autoSelect ? undefined : nodeId || undefined, provider_group_id: autoSelect ? groupFilterId || undefined : undefined, consume_multiplier: consumeMultiplier })
      setQuote(q.breakdown)
      if (!autoSelect && q.chosen_node_id) setNodeId(q.chosen_node_id)
    } catch (e) { toast.error(String((e as Error).message)) }
  }

  function selectAuto() {
    setAutoSelect(true); setNodeId('')
    if (scriptId) {
      void quoteOrder({ script_id: scriptId, version, provider_group_id: groupFilterId || undefined, consume_multiplier: consumeMultiplier })
        .then((p) => setQuote(p.breakdown)).catch(() => setQuote(null))
    }
  }

  function selectProvider(selectedNodeId: string) {
    setAutoSelect(false); setNodeId(selectedNodeId)
    void quoteOrder({ script_id: scriptId, version, node_id: selectedNodeId, consume_multiplier: consumeMultiplier })
      .then((p) => setQuote(p.breakdown)).catch(() => setQuote(null))
  }

  async function onSearchGroups() {
    const query = groupQuery.trim()
    if (!query) { toast.error(t('Enter a group name to search')); return }
    setGroupSearching(true)
    try { setGroupResults(await searchProviderGroups(query)) }
    catch (e) { toast.error(String((e as Error).message)) }
    finally { setGroupSearching(false) }
  }

  async function applyGroupFilter(groupId: string, groupName: string) {
    setGroupFilterId(groupId); setGroupFilterName(groupName)
    setGroupResults([]); setGroupQuery(groupName)
    if (scriptId) await loadOffersFor(scriptId, version, groupId)
  }

  // runTask executes one purchase+relay cycle for a queued task entry.
  // It runs fire-and-forget so multiple tasks can be in-flight at once.
  async function runTask(
    localId: string, taskScriptId: number, taskVersion: number,
    cleanedConfigText: string, inputHash: string,
    capturedAutoSelect: boolean, capturedNodeId: string,
    capturedGroupFilterId: string, capturedMultiplier: number
  ) {
    const upd = (patch: Partial<QueuedTask>) => updateTask(localId, patch)
    try {
      const key = `order-${Date.now()}-${Math.random().toString(36).slice(2)}`
      const { order: o } = await createOrder({
        script_id: taskScriptId, version: taskVersion,
        node_id: capturedAutoSelect ? undefined : capturedNodeId || undefined,
        provider_group_id: capturedAutoSelect ? capturedGroupFilterId || undefined : undefined,
        input_hash: inputHash, consume_multiplier: capturedMultiplier,
      }, key)
      upd({ order: o, status: 'running', relayStatus: t('Order created, connecting...') })
      if (o.state === 'REFUNDED') {
        upd({ status: 'failed', error: t('Provider rejected; funds refunded'), relayStatus: '' })
        await loadBalance(); return
      }
      await loadBalance()
      const config = JSON.parse(cleanedConfigText)
      const relayUrl = `${location.origin.replace(/^http/, 'ws')}/api/relay`
      const session = new ClientRelaySession({ relayUrl, taskId: o.id, attempt: 1, clientDeviceId: `client-${o.client_id}` })
      upd({ relayStatus: t('Connecting to relay...') })
      let cancelled = false
      const failFast = new Promise<never>((_, reject) => {
        const tick = async () => {
          while (!cancelled) {
            await new Promise((r) => window.setTimeout(r, 1500))
            if (cancelled) return
            try {
              const latest = await getOrder(o.id)
              upd({ order: latest })
              if (TERMINAL_FAILURE_STATES.has(latest.state)) { reject(new Error(describeOrderError(latest.last_error))); return }
            } catch { /* transient */ }
          }
        }
        void tick()
      })
      try {
        await session.connect()
        upd({ relayStatus: t('Waiting for provider handshake...') })
        await Promise.race([session.waitEstablished(), failFast])
        upd({ relayStatus: t('Sending config, waiting for result...') })
        await session.sendConfig(config)
        const result = await Promise.race([session.waitForResult(), failFast])
        if (result && typeof result === 'object' && (result as Record<string, unknown>).ok === false) {
          const scriptError = (result as Record<string, unknown>).error
          throw new Error(typeof scriptError === 'string' && scriptError ? scriptError : describeOrderError('SCRIPT_EXECUTION_FAILED'))
        }
        const resultText = JSON.stringify(result, null, 2)
        upd({ status: 'success', relayResult: resultText, relayStatus: t('Result received') })
        addTaskRecord({ orderId: o.id, scriptId: taskScriptId, scriptTitle: scripts.find((s) => s.id === taskScriptId)?.title ?? '', version: taskVersion, nodeId: o.chosen_node_id, configText: cleanedConfigText, result: resultText, status: 'SUCCESS', createdAt: Math.floor(Date.now() / 1000) })
        try {
          const resultHash = await sha256Hex(JSON.stringify(result ?? null))
          await api.post(`/api/orders/${o.id}/receipts`, { task_id: o.id, attempt: 1, party: 'client', order_id: o.id, result_hash: resultHash })
        } catch { /* best-effort receipt */ }
        upd({ order: await getOrder(o.id) })
        await loadBalance()
      } catch (e) {
        upd({ relayStatus: '' })
        try {
          const latest = await getOrder(o.id)
          upd({ order: latest })
          if (['FUNDS_RESERVED', 'MATCHING', 'OFFERED'].includes(latest.state)) {
            upd({ order: await cancelOrder(o.id) }); await loadBalance()
          } else if (TERMINAL_FAILURE_STATES.has(latest.state)) { await loadBalance() }
        } catch { /* preserve original error */ }
        addTaskRecord({ orderId: o.id, scriptId: taskScriptId, scriptTitle: scripts.find((s) => s.id === taskScriptId)?.title ?? '', version: taskVersion, nodeId: o.chosen_node_id, configText: cleanedConfigText, result: '', status: 'FAILED', error: String((e as Error).message), createdAt: Math.floor(Date.now() / 1000) })
        upd({ status: 'failed', error: String((e as Error).message) })
      } finally { cancelled = true; session.close() }
    } catch (e) {
      updateTask(localId, { status: 'failed', error: String((e as Error).message), relayStatus: '' })
    }
  }

  async function onPurchase() {
    if (!scriptId) { toast.error(t('Select a script first')); return }
    let inputHash = '', cleanedConfigText = ''
    try {
      const config = cleanEmptyArrayItems(JSON.parse(configText))
      cleanedConfigText = JSON.stringify(config, null, 2)
      setConfigText(cleanedConfigText)
      inputHash = await sha256Hex(JSON.stringify(config))
    } catch { toast.error(t('Config must be valid JSON')); return }
    // Capture current provider selection before async state changes
    const capturedAutoSelect = autoSelect
    const capturedNodeId = nodeId
    const capturedGroupFilterId = groupFilterId
    const capturedMultiplier = consumeMultiplier
    const localId = `task-${Date.now()}-${Math.random().toString(36).slice(2)}`
    setTaskQueue((prev) => [
      {
        localId, scriptId, scriptTitle: selectedScript?.title ?? '',
        version, submittedAt: Date.now(), status: 'submitting',
        order: null, relayStatus: t('Creating order...'),
        relayResult: '', configText: cleanedConfigText, resultView: 'form',
      },
      ...prev,
    ])
    // Fire and forget — doesn't block the UI for further submissions
    void runTask(localId, scriptId, version, cleanedConfigText, inputHash,
      capturedAutoSelect, capturedNodeId, capturedGroupFilterId, capturedMultiplier)
  }

  async function onCancelTask(orderId: string) {
    try {
      const cancelled = await cancelOrder(orderId)
      setTaskQueue((prev) =>
        prev.map((t) => (t.order?.id === orderId ? { ...t, order: cancelled } : t))
      )
      await loadBalance()
      toast.success(t('Order cancelled'))
    } catch (e) { toast.error(String((e as Error).message)) }
  }

  const insufficientBalance = quote != null && quote.MaxCustomerMicros > (bal?.client_available ?? 0)

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        <span className='inline-flex flex-wrap items-baseline gap-x-2 gap-y-0.5'>
          {t('AiToken P2P Marketplace')}
          {/* Refresh drops any in-flight run's encrypted relay connection. */}
          <span className='text-[11px] font-normal text-red-500'>
            {t('Do not refresh while running — it interrupts tasks')}
          </span>
        </span>
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        {/* Compact balance chip: available + reserved, click to manage funds */}
        <button
          type='button'
          onClick={() => setWalletOpen(true)}
          className='hover:bg-muted/50 flex items-center gap-3 rounded-md border px-3 py-1.5 text-left transition-colors'
        >
          <WalletCards className='text-muted-foreground h-4 w-4 shrink-0' />
          <div className='leading-tight'>
            <div className='text-[10px] text-muted-foreground'>{t('Available')}</div>
            <div className='text-sm font-semibold'>{microsToCurrency(bal?.client_available)}</div>
          </div>
          <div className='border-l pl-3 leading-tight'>
            <div className='text-[10px] text-muted-foreground'>{t('Reserved')}</div>
            <div className='text-sm font-semibold'>{microsToCurrency(bal?.client_reserved)}</div>
          </div>
        </button>
        <Button variant='outline' render={<a href='/aitoken-api-docs' target='_blank' rel='noopener noreferrer' />}>
          <FileCode className='mr-2 h-4 w-4' />{t('API docs')}
        </Button>
        <Button variant='outline' onClick={() => setRecordsOpen(true)}>
          <History className='mr-2 h-4 w-4' />{t('Task records')}
          {taskRecords.length > 0 && <Badge variant='secondary'>{taskRecords.length}</Badge>}
        </Button>
        <Button variant='outline' onClick={loadBalance}>{t('Refresh')}</Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        {/* Two-column layout: form left, task queue right.
            min-w-0 on both columns is required — grid tracks default to
            min-width:auto, so long text/URLs would otherwise blow a column
            past its fr share and squeeze the other. */}
        <div className='grid grid-cols-1 gap-4 lg:grid-cols-[3fr_2fr] lg:items-start'>
          {/* LEFT: configuration form */}
          <div className='flex min-w-0 flex-col gap-4'>

            {/* Script & Provider card */}
            <div className='rounded-lg border p-4'>
              <div className='mb-2 text-sm font-medium'>{t('Choose script & provider')}</div>
              <div className='flex flex-wrap items-center gap-2'>
                <select className='h-9 min-w-[220px] rounded-md border px-2 text-sm' value={scriptId} onChange={(e) => void selectScript(Number(e.target.value))}>
                  <option value={0}>{t('Select a script')}</option>
                  {scripts.map((s) => (<option key={s.id} value={s.id}>#{s.id} {s.title}</option>))}
                </select>
                <select className='h-9 w-28 rounded-md border px-2 text-sm' value={version} onChange={(e) => {
                  const v = Number(e.target.value); setVersion(v); setOffers([]); setNodeId(''); setAutoSelect(true); setOffersPage(0); setQuote(null)
                  const sel = availableVersions.find((item) => item.version === v)
                  setConfigText(configTextFromParams(sel?.script_params))
                  if (scriptId) void loadOffersFor(scriptId, v)
                }}>
                  {versions.map((item) => (<option key={item} value={item}>v{item}</option>))}
                </select>
                <Button variant='outline' onClick={() => { if (!scriptId) { toast.error(t('Select a script first')); return }; void loadOffersFor(scriptId, version) }} disabled={offersLoading}>
                  {offersLoading ? t('Loading...') : t('View offers')}
                </Button>
              </div>
              {selectedScript?.description && (
                <div className='bg-muted/30 mt-3 rounded-md border px-3 py-2 text-xs'>
                  <button
                    type='button'
                    className='flex w-full items-start gap-2 text-left'
                    onClick={() => setDescExpanded((v) => !v)}
                  >
                    <span className='text-muted-foreground shrink-0 font-medium'>{t('Script description')}</span>
                    <span className={descExpanded ? 'min-w-0 flex-1 break-words whitespace-pre-wrap' : 'min-w-0 flex-1 truncate'}>
                      {selectedScript.description}
                    </span>
                    {descExpanded
                      ? <ChevronUp className='text-muted-foreground mt-0.5 h-3.5 w-3.5 shrink-0' />
                      : <ChevronDown className='text-muted-foreground mt-0.5 h-3.5 w-3.5 shrink-0' />}
                  </button>
                </div>
              )}
              <div className='mt-3'>
                <div className='text-muted-foreground mb-2 text-[11px]'>{t('Provider offers (Auto picks the best idle provider, or choose one)')}</div>
                <div className='relative flex h-10 min-w-0 items-center gap-2 rounded-md border px-2 text-xs'>
                  <label className='flex min-w-0 items-center gap-2'>
                    <input type='radio' name='offer' checked={autoSelect} onChange={selectAuto} />
                    <span className='shrink-0 font-medium'>{t('Auto (recommended)')}</span>
                    <span className='text-muted-foreground hidden truncate text-xs md:inline'>{groupFilterId ? t('Auto-picks the best idle provider in this group') : t('Auto-picks the best idle provider')}</span>
                  </label>
                  <div className='relative ml-auto flex min-w-0 items-center gap-2'>
                    <Input className='h-8 w-40 sm:w-56' placeholder={t('Search group name')} value={groupQuery} onChange={(e) => setGroupQuery(e.target.value)} onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); void onSearchGroups() } }} />
                    <Button type='button' variant='outline' size='sm' className='h-8' onClick={onSearchGroups} disabled={groupSearching}>{groupSearching ? t('Searching...') : t('Search')}</Button>
                    {groupFilterId && (
                      <span className='flex max-w-36 items-center gap-1 text-xs'>
                        <span className='truncate font-medium'>{groupFilterName}</span>
                        <Button type='button' size='sm' variant='ghost' className='h-7 px-2' onClick={() => void applyGroupFilter('', '')}>{t('Clear')}</Button>
                      </span>
                    )}
                    {groupResults.length > 0 && (
                      <div className='bg-popover absolute top-full right-0 z-20 mt-1 flex w-72 flex-col gap-1 rounded-md border p-1 shadow-md'>
                        {groupResults.map((g) => (
                          <button key={g.id} type='button' className='hover:bg-muted/50 rounded px-2 py-1 text-left text-sm' onClick={() => void applyGroupFilter(g.id, g.name)}>
                            <span className='font-medium'>{g.name}</span>
                          </button>
                        ))}
                      </div>
                    )}
                  </div>
                </div>

                {offers.length > 0 && (
                  <div className='mt-2 grid grid-cols-1 gap-1 lg:grid-cols-2'>
                    {offers.slice(offersPage * OFFERS_PAGE_SIZE, offersPage * OFFERS_PAGE_SIZE + OFFERS_PAGE_SIZE).map((o) => {
                      const rate = o.executions > 0 ? `${Math.round((o.successes / o.executions) * 100)}% (${o.successes}/${o.executions})` : '-'
                      let statusLabel = t('Offline')
                      if (o.busy) statusLabel = t('Busy')
                      else if (o.online) statusLabel = t('Online')
                      return (
                        <label key={o.node_id} className='flex flex-wrap items-center gap-x-2 gap-y-1 rounded-md border p-2 text-[11px]'>
                          <input type='radio' name='offer' checked={!autoSelect && nodeId === o.node_id} disabled={!o.available && o.unavailable_reason !== 'BALANCE_CHECK_EXPIRED'} onChange={() => selectProvider(o.node_id)} />
                          <span className='font-mono'>{o.node_id}</span>
                          {o.provider_group_name && <span className='bg-muted rounded px-1.5 py-0.5'>{o.provider_group_name}</span>}
                          <span className='font-semibold'>{microsToCurrency(o.price_micros)}</span>
                          <span>{statusLabel}</span>
                          {o.owned && !o.enabled && <span className='rounded bg-amber-500/15 px-1.5 py-0.5 text-amber-700'>{t('Your node (disabled) — selectable for testing')}</span>}
                          <span className='text-muted-foreground'>{t('Success rate')}: {rate}</span>
                          <span className='text-muted-foreground'>{t('quota')}: {o.remaining_quota}</span>
                          {o.concurrency > 1 && <span className='text-muted-foreground'>{t('slots')}: {o.available_slots}/{o.total_slots}</span>}
                          {!o.available && o.unavailable_reason !== 'BALANCE_CHECK_EXPIRED' && (
                            <span className='text-red-600'>
                              {o.unavailable_reason === 'QUOTA_EXHAUSTED' && t('Quota exhausted')}
                              {o.unavailable_reason === 'NODE_OFFLINE' && t('Node offline')}
                              {o.unavailable_reason === 'NODE_DISABLED' && t('Provider disabled this node')}
                              {o.unavailable_reason === 'NODE_BUSY' && t('Provider is busy')}
                              {o.unavailable_reason === 'CAPABILITY_TEST_EXPIRED' && t('Capability test expired')}
                              {o.unavailable_reason === 'INSUFFICIENT_NODE_BALANCE' && t('Insufficient node balance for this amount')}
                            </span>
                          )}
                        </label>
                      )
                    })}
                  </div>
                )}
                {offers.length > OFFERS_PAGE_SIZE && (
                  <div className='mt-2 flex items-center justify-between text-xs'>
                    <span className='text-muted-foreground'>{t('{{count}} providers', { count: offers.length })}</span>
                    <div className='flex items-center gap-2'>
                      <Button size='sm' variant='outline' disabled={offersPage === 0} onClick={() => setOffersPage((p) => Math.max(0, p - 1))}>{t('Previous')}</Button>
                      <span>{offersPage + 1} / {Math.ceil(offers.length / OFFERS_PAGE_SIZE)}</span>
                      <Button size='sm' variant='outline' disabled={(offersPage + 1) * OFFERS_PAGE_SIZE >= offers.length} onClick={() => setOffersPage((p) => p + 1)}>{t('Next')}</Button>
                    </div>
                  </div>
                )}
              </div>
            </div>

            {/* Consume multiplier — single line: label + hint + input */}
            <div className='flex items-center gap-3 rounded-lg border px-4 py-2.5'>
              <span className='shrink-0 text-sm font-medium'>{t('Consume multiplier')}</span>
              <span className='text-muted-foreground min-w-0 flex-1 truncate text-xs' title={t('Units of work for one run (min 1). Fee = base price × this value.')}>
                {t('Units of work for one run (min 1). Fee = base price × this value.')}
              </span>
              <Input className='h-8 w-20 shrink-0' type='number' min={1} step={1} value={consumeMultiplier} onChange={(e) => onMultiplierChange(e.target.value)} aria-label={t('Consume multiplier')} />
            </div>

            {/* Parameters */}
            <div className='rounded-lg border p-4'>
              <div className='mb-2 flex flex-wrap items-center justify-between gap-2'>
                <div className='text-sm font-medium'>{t('Parameters')}</div>
                <div className='flex gap-1' role='group' aria-label={t('Parameters')}>
                  <Button type='button' size='sm' variant={parametersView === 'form' ? 'secondary' : 'ghost'} onClick={() => setParametersView('form')}>
                    <ListTree className='mr-2 h-4 w-4' />{t('Visual Mode')}
                  </Button>
                  <Button type='button' size='sm' variant={parametersView === 'json' ? 'secondary' : 'ghost'} onClick={() => setParametersView('json')}>
                    <Braces className='mr-2 h-4 w-4' />JSON
                  </Button>
                </div>
              </div>
              {parametersView === 'json' ? (
                <Textarea className='min-h-[140px] font-mono text-xs' value={configText} onChange={(e) => setConfigText(e.target.value)} />
              ) : (
                (() => {
                  try {
                    const config = JSON.parse(configText) as unknown
                    return (
                      <JsonForm value={config} compact onChange={(path, value) => setConfigText(JSON.stringify(updateJsonValue(config, path, value), null, 2))} />
                    )
                  } catch {
                    return <div className='text-destructive rounded-md border p-3 text-sm'>{t('Invalid JSON')}</div>
                  }
                })()
              )}
              <div className='text-muted-foreground mt-1 text-[11px]'>{t('Only the hash of these parameters crosses the control plane; the plaintext travels the encrypted data plane to the provider.')}</div>
            </div>
          </div>

          {/* RIGHT: Task queue panel — sticky, shows all in-flight and completed tasks */}
          <div className='sticky top-4 flex min-w-0 flex-col gap-3'>
            <div className='flex items-center justify-between'>
              <div className='text-sm font-medium'>{t('Task queue')}</div>
              {taskQueue.length > 0 && (
                <div className='flex items-center gap-2'>
                  <span className='text-muted-foreground text-xs'>
                    {taskQueue.filter((t) => t.status === 'running' || t.status === 'submitting').length > 0 && (
                      <>{taskQueue.filter((tk) => tk.status === 'running' || tk.status === 'submitting').length} {t('running')} · </>
                    )}
                    {taskQueue.length} {t('total')}
                  </span>
                  <Button size='sm' variant='ghost' className='h-7 px-2 text-xs' onClick={() => setTaskQueue((prev) => prev.filter((tk) => tk.status === 'running' || tk.status === 'submitting'))}>
                    {t('Clear done')}
                  </Button>
                </div>
              )}
            </div>
            {taskQueue.length === 0 ? (
              <div className='rounded-lg border border-dashed p-8 text-center'>
                <div className='text-muted-foreground text-sm'>{t('No tasks yet')}</div>
                <div className='text-muted-foreground mt-1 text-xs'>{t('Get a quote then click "Purchase and run" to start')}</div>
              </div>
            ) : (
              <div className='flex flex-col gap-2 max-h-[calc(100vh-16rem)] overflow-y-auto'>
                {taskQueue.map((task) => (
                  <TaskCard
                    key={task.localId}
                    task={task}
                    onResultViewChange={(localId, view) => updateTask(localId, { resultView: view })}
                    onCancel={onCancelTask}
                  />
                ))}
              </div>
            )}
          </div>
        </div>

        {/* Full-width sticky action bar — spans both columns, always visible at viewport bottom */}
        <div className='sticky bottom-0 z-10 mt-4 rounded-lg border border-t-2 border-t-primary/30 bg-muted/40 px-4 py-3 shadow-lg backdrop-blur-sm'>
          <div className='flex flex-wrap items-center gap-3'>
            <Button variant='outline' onClick={onQuote}>{t('Get quote')}</Button>
            <Button className='min-w-40' onClick={() => void onPurchase()} disabled={!quote || insufficientBalance}>
              {t('Purchase and run')}
            </Button>
            {quote && (
              <div className='text-sm'>
                <span className='text-muted-foreground'>{t('Total')}: </span>
                <span className='font-semibold'>{microsToCurrency(quote.MaxCustomerMicros)}</span>
                {insufficientBalance && <span className='ml-2 text-xs text-red-600'>{t('Insufficient balance')}</span>}
              </div>
            )}
            {quote && (
              <div className='ml-auto flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground'>
                <span>{t('Provider')}: {microsToCurrency(quote.ProviderMicros)}</span>
                <span>{t('Author')}: {microsToCurrency(quote.AuthorMicros)}</span>
                <span>{t('Platform fee')}: {microsToCurrency(quote.PlatformFeeMicros)}</span>
                <span>{t('Risk reserve')}: {microsToCurrency(quote.RiskReserveMicros)}</span>
              </div>
            )}
          </div>
        </div>

        {/* Wallet dialog — recharge/withdraw between main wallet and marketplace balance */}
        <Dialog open={walletOpen} onOpenChange={setWalletOpen}>
          <DialogContent className='sm:max-w-md'>
            <DialogHeader>
              <DialogTitle>{t('Manage balance')}</DialogTitle>
              <DialogDescription>{t('Move funds between your main wallet and the marketplace balance.')}</DialogDescription>
            </DialogHeader>
            <div className='space-y-4'>
              <div className='grid grid-cols-2 gap-3'>
                <div className='rounded-lg border p-3'>
                  <div className='text-muted-foreground text-xs'>{t('Available')}</div>
                  <div className='text-lg font-semibold'>{microsToCurrency(bal?.client_available)}</div>
                </div>
                <div className='rounded-lg border p-3'>
                  <div className='text-muted-foreground text-xs'>{t('Reserved')}</div>
                  <div className='text-lg font-semibold'>{microsToCurrency(bal?.client_reserved)}</div>
                </div>
              </div>
              <div className='rounded-lg border p-3'>
                <div className='mb-1 flex items-center gap-2 text-sm font-medium'>
                  <ArrowDownToLine className='h-4 w-4' />{t('Recharge')}
                </div>
                <div className='text-muted-foreground mb-2 text-xs'>{t('Wallet balance')}: {walletQuota != null ? formatQuotaWithCurrency(walletQuota) : '--'}</div>
                <div className='flex items-center gap-2'>
                  <Input className='h-9 flex-1' inputMode='decimal' value={rechargeAmt} onChange={(e) => setRechargeAmt(e.target.value)} aria-label={t('Recharge amount')} />
                  <Button className='h-9' onClick={onRecharge} disabled={recharging}>{recharging ? t('Recharging...') : t('Recharge')}</Button>
                </div>
              </div>
              <div className='rounded-lg border p-3'>
                <div className='mb-1 flex items-center gap-2 text-sm font-medium'>
                  <ArrowUpFromLine className='h-4 w-4' />{t('Withdraw to wallet')}
                </div>
                <div className='text-muted-foreground mb-2 text-xs'>{t('Minimum 10; 5% fee; wallet receives 95%')}</div>
                <div className='flex items-center gap-2'>
                  <Input className='h-9 flex-1' inputMode='decimal' value={withdrawAmt} onChange={(e) => setWithdrawAmt(e.target.value)} aria-label={t('Withdraw amount')} />
                  <Button className='h-9' variant='outline' onClick={onWithdraw} disabled={withdrawing}>{withdrawing ? t('Withdrawing...') : t('Withdraw')}</Button>
                </div>
              </div>
            </div>
          </DialogContent>
        </Dialog>

        {/* Task records dialog */}
        <Dialog open={recordsOpen} onOpenChange={setRecordsOpen}>
          <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-[min(1000px,calc(100vw-2rem))]'>
            <DialogHeader className='pr-8'>
              <DialogTitle>{t('Task records')}</DialogTitle>
              <DialogDescription>{t('Your past runs on this device (sent parameters and returned result). Stored locally in this browser only.')}</DialogDescription>
            </DialogHeader>
            {taskRecords.length > 0 && (
              <div className='flex justify-end'>
                <Button size='sm' variant='outline' onClick={() => { setTaskRecords([]); setExpandedRecordId(null) }}>{t('Clear records')}</Button>
              </div>
            )}
            <div className='space-y-2'>
              {taskRecords.map((record) => {
                const expanded = expandedRecordId === record.orderId
                return (
                  <div key={record.orderId} className='rounded-md border text-sm'>
                    <button type='button' className='hover:bg-muted/40 flex w-full flex-wrap items-center gap-2 px-3 py-2 text-left' onClick={() => setExpandedRecordId(expanded ? null : record.orderId)}>
                      <Badge variant={record.status === 'SUCCESS' ? 'secondary' : 'outline'} className={record.status === 'SUCCESS' ? 'text-emerald-600' : 'text-red-600'}>
                        {record.status === 'SUCCESS' ? t('Success') : t('Failed')}
                      </Badge>
                      <span className='font-medium'>#{record.scriptId}{record.scriptTitle ? ` ${record.scriptTitle}` : ''} v{record.version}</span>
                      <span className='text-muted-foreground text-xs'>{formatUnix(record.createdAt)}</span>
                      <span className='text-muted-foreground ml-auto font-mono text-xs'>{record.orderId}</span>
                    </button>
                    {expanded && (
                      <div className='space-y-3 border-t px-3 py-3'>
                        {record.nodeId && <div className='text-muted-foreground text-xs'>{t('Provider node')}: <span className='font-mono'>{record.nodeId}</span></div>}
                        {record.error && <div className='text-xs text-red-600'>{record.error}</div>}
                        <div>
                          <div className='mb-1 text-xs font-medium'>{t('Sent parameters')}</div>
                          <pre className='bg-muted/30 max-h-56 overflow-auto rounded-md border p-2 text-xs'>{record.configText}</pre>
                        </div>
                        {record.result && (
                          <div>
                            <div className='mb-1 text-xs font-medium'>{t('Returned result')}</div>
                            <pre className='bg-muted/30 max-h-56 overflow-auto rounded-md border p-2 text-xs'>{record.result}</pre>
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                )
              })}
              {taskRecords.length === 0 && <div className='text-muted-foreground py-10 text-center text-sm'>{t('No task records yet')}</div>}
            </div>
          </DialogContent>
        </Dialog>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}



















