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
  Eye,
  FileAudio,
  FileCode,
  History,
  ListTree,
  Loader2,
  Plus,
  RefreshCw,
  Trash2,
  WalletCards,
  X,
  XCircle,
  ZoomIn,
} from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
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
import { computeParamsMultiplier } from './pricing-rules-editor'
import { AssetLibraryDialog } from './asset-library-dialog'
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
}
type ViewMode = 'form' | 'json'
// Result panel adds a 'preview' tab (media) on top of the parameter view modes.
type ResultView = 'preview' | 'form' | 'json'

type MediaKind = 'image' | 'video' | 'audio'
type FoundMedia = { kind: MediaKind; url: string }

const MEDIA_EXTENSIONS: Record<MediaKind, string[]> = {
  image: ['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'svg', 'avif'],
  video: ['mp4', 'webm', 'mov', 'm4v', 'ogv'],
  audio: ['mp3', 'wav', 'ogg', 'm4a', 'flac', 'aac', 'opus'],
}

// classifyMediaUrl inspects a string for an http(s)/data URL that points at a
// supported media file and returns its kind, or null if it isn't media.
function classifyMediaUrl(raw: string): MediaKind | null {
  const value = raw.trim()
  if (!value) return null
  const dataMatch = /^data:(image|video|audio)\//i.exec(value)
  if (dataMatch) return dataMatch[1].toLowerCase() as MediaKind
  if (!/^https?:\/\//i.test(value)) return null
  let pathname = value
  try {
    pathname = new URL(value).pathname
  } catch {
    // fall back to the raw string (query strings still get stripped below)
  }
  const ext = pathname.split(/[?#]/)[0].split('.').pop()?.toLowerCase() ?? ''
  for (const kind of ['image', 'video', 'audio'] as MediaKind[]) {
    if (MEDIA_EXTENSIONS[kind].includes(ext)) return kind
  }
  return null
}

function collectUrlCandidates(value: unknown, candidates: string[] = []): string[] {
  if (typeof value === 'string') {
    const url = value.trim()
    if (/^(https?:\/\/|data:(image|video|audio)\/)/i.test(url)) {
      candidates.push(url)
    }
    return candidates
  }
  if (Array.isArray(value)) {
    for (const item of value) collectUrlCandidates(item, candidates)
    return candidates
  }
  if (value !== null && typeof value === 'object') {
    for (const item of Object.values(value)) collectUrlCandidates(item, candidates)
  }
  return candidates
}

// Probe extensionless signed URLs with native media elements. This does not
// require reading cross-origin response headers, so it also works without CORS.
function probeMediaUrl(url: string): Promise<MediaKind | null> {
  return new Promise((resolve) => {
    const media = document.createElement('video')
    const image = new Image()
    let pending = 2
    let settled = false

    function finish(kind: MediaKind | null) {
      if (settled) return
      if (kind == null && --pending > 0) return
      settled = true
      window.clearTimeout(timeoutId)
      media.removeAttribute('src')
      media.load()
      image.src = ''
      resolve(kind)
    }

    media.preload = 'metadata'
    media.addEventListener(
      'loadedmetadata',
      () => finish(media.videoWidth > 0 || media.videoHeight > 0 ? 'video' : 'audio'),
      { once: true }
    )
    media.addEventListener('error', () => finish(null), { once: true })
    image.addEventListener('load', () => finish('image'), { once: true })
    image.addEventListener('error', () => finish(null), { once: true })
    const timeoutId = window.setTimeout(() => {
      pending = 1
      finish(null)
    }, 8000)
    media.src = url
    image.src = url
  })
}

async function detectFirstMedia(value: unknown): Promise<FoundMedia | null> {
  for (const url of collectUrlCandidates(value)) {
    const knownKind = classifyMediaUrl(url)
    const kind = knownKind ?? await probeMediaUrl(url)
    if (kind) return { kind, url }
  }
  return null
}

// findFirstMedia walks a parsed result (depth-first, preserving key/array order)
// and returns the first image/video/audio URL it can find, or null.
function findFirstMedia(value: unknown): FoundMedia | null {
  if (typeof value === 'string') {
    const kind = classifyMediaUrl(value)
    return kind ? { kind, url: value.trim() } : null
  }
  if (Array.isArray(value)) {
    for (const item of value) {
      const found = findFirstMedia(item)
      if (found) return found
    }
    return null
  }
  if (value !== null && typeof value === 'object') {
    for (const item of Object.values(value)) {
      const found = findFirstMedia(item)
      if (found) return found
    }
  }
  return null
}

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
  resultView: ResultView
  expanded?: boolean
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

function getDescriptionExpandedStorageKey() {
  const userId = window.localStorage.getItem('uid') ?? 'anonymous'
  return `aitoken-purchase-description-expanded:${userId}`
}

function loadDescriptionExpanded(): boolean {
  try {
    return window.localStorage.getItem(getDescriptionExpandedStorageKey()) !== 'false'
  } catch {
    return true
  }
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

// isMediaUrlArray reports whether an array holds only strings and at least one
// of them is a media URL — the trigger for the inline preview gallery. Empty
// strings are allowed so freshly-added slots stay as inputs.
function isMediaUrlArray(value: unknown): value is string[] {
  return (
    Array.isArray(value) &&
    value.length > 0 &&
    value.every((item) => typeof item === 'string') &&
    value.some((item) => classifyMediaUrl(item as string) != null)
  )
}

// MediaThumb renders one editable media URL as a small preview box. It shows a
// clear badge (empties the value → the caller restores an input) and, in array
// context, a delete badge (removes the element). Image/video open a fullscreen
// zoom. Fixed width keeps the form column from changing size.
// onClear/onDelete are omitted in read-only contexts (task-queue params
// preview), where the thumb is display-only. size='sm' shrinks it further so a
// row of params previews stays short and doesn't inflate the card height.
type MediaThumbProps = {
  url: string
  kind: MediaKind
  onClear?: () => void
  onDelete?: () => void
  size?: 'md' | 'sm'
}
function MediaThumb({ url, kind, onClear, onDelete, size = 'md' }: MediaThumbProps) {
  const { t } = useTranslation()
  const [zoomOpen, setZoomOpen] = useState(false)
  const small = size === 'sm'
  let boxWidth = small ? 'w-20' : 'w-28'
  if (kind === 'audio') boxWidth = small ? 'w-36' : 'w-44'
  const boxHeight = small ? 'h-14' : 'h-20'
  return (
    <div className={`relative shrink-0 overflow-hidden rounded-md border bg-muted/40 ${boxWidth}`}>
      <div className={`flex items-center justify-center overflow-hidden ${boxHeight}`}>
        {kind === 'image' && (
          <button type='button' className='h-full w-full cursor-zoom-in' aria-label={t('Preview')} onClick={() => setZoomOpen(true)}>
            <img className='h-full w-full object-cover' src={url} alt='' loading='lazy' />
          </button>
        )}
        {kind === 'video' && (
          <video className='h-full w-full cursor-zoom-in object-cover' src={url} preload='metadata' muted onClick={() => setZoomOpen(true)} />
        )}
        {kind === 'audio' && (
          <div className='flex w-full flex-col items-center gap-1 px-1.5'>
            <FileAudio className={small ? 'h-4 w-4 text-muted-foreground' : 'h-5 w-5 text-muted-foreground'} aria-hidden='true' />
            <audio className='h-7 w-full' src={url} controls preload='metadata' />
          </div>
        )}
      </div>
      {/* Action badges: delete element (if array) then clear value. Read-only
          previews pass neither, so no badges render. */}
      {(onDelete || onClear) && (
        <div className='absolute right-0.5 top-0.5 flex gap-0.5'>
          {onDelete && (
            <button
              type='button'
              className='flex h-5 w-5 items-center justify-center rounded bg-black/60 text-white hover:bg-destructive'
              title={t('Remove item')}
              aria-label={t('Remove item')}
              onClick={onDelete}
            >
              <Trash2 className='h-3 w-3' aria-hidden='true' />
            </button>
          )}
          {onClear && (
            <button
              type='button'
              className='flex h-5 w-5 items-center justify-center rounded bg-black/60 text-white hover:bg-black/80'
              title={t('Clear')}
              aria-label={t('Clear')}
              onClick={onClear}
            >
              <X className='h-3 w-3' aria-hidden='true' />
            </button>
          )}
        </div>
      )}
      {(kind === 'image' || kind === 'video') && (
        <Dialog open={zoomOpen} onOpenChange={setZoomOpen}>
          <DialogContent closeLabel={t('Close')} className='flex h-[calc(100vh-2rem)] w-[calc(100vw-2rem)] items-center justify-center overflow-hidden bg-black/95 p-4 text-white sm:max-w-[calc(100vw-2rem)]' aria-label={t('Preview')}>
            <DialogTitle className='sr-only'>{t('Preview')}</DialogTitle>
            {kind === 'image' ? (
              <img className='max-h-full max-w-full object-contain' src={url} alt='' />
            ) : (
              <video className='max-h-full max-w-full object-contain' src={url} controls autoPlay />
            )}
          </DialogContent>
        </Dialog>
      )}
    </div>
  )
}

type JsonFormProps = {
  value: unknown
  onChange?: (path: (string | number)[], value: unknown) => void
  path?: (string | number)[]
  // compact: smaller text + tighter row spacing for read-only result display
  compact?: boolean
  // previewMedia: in read-only mode, render media URLs as small preview thumbs
  // instead of plain text (used by the task-queue parameters preview).
  previewMedia?: boolean
}

function JsonForm(props: JsonFormProps) {
  const { t } = useTranslation()
  const path = props.path ?? []
  const compact = props.compact ?? false
  const previewMedia = props.previewMedia ?? false
  // Editable array of media URLs: render each entry as an inline preview box
  // (or an input while it isn't yet a media URL), wrapping in a single row, with
  // an Add button at the end. min-w-0 keeps it inside the value column.
  if (props.onChange && isMediaUrlArray(props.value)) {
    const items = props.value
    return (
      <div className='flex min-w-0 flex-wrap items-center gap-2'>
        {items.map((item, index) => {
          const kind = classifyMediaUrl(item)
          if (kind) {
            return (
              <MediaThumb
                // eslint-disable-next-line react/no-array-index-key
                key={index}
                url={item.trim()}
                kind={kind}
                onClear={() => props.onChange?.([...path, index], '')}
                onDelete={() => props.onChange?.(path, removeJsonArrayItem(items, [], index))}
              />
            )
          }
          return (
            <div
              // eslint-disable-next-line react/no-array-index-key
              key={index}
              className='flex w-56 items-center gap-1'
            >
              <Input
                className='h-7 min-w-0 flex-1 px-2 text-xs'
                value={item}
                onChange={(event) => props.onChange?.([...path, index], event.target.value)}
              />
              <Button
                type='button'
                size='icon-sm'
                variant='ghost'
                className='text-muted-foreground hover:text-destructive shrink-0'
                title={t('Remove item')}
                aria-label={t('Remove item {{index}}', { index: index + 1 })}
                onClick={() => props.onChange?.(path, removeJsonArrayItem(items, [], index))}
              >
                <Trash2 className='h-4 w-4' aria-hidden='true' />
              </Button>
            </div>
          )
        })}
        <Button
          type='button'
          size='sm'
          variant='outline'
          className='h-20 w-16 shrink-0 flex-col gap-1 border-dashed'
          title={t('Add item')}
          aria-label={t('Add item')}
          onClick={() => props.onChange?.(path, appendJsonArrayItem(items, []))}
        >
          <Plus className='h-4 w-4' />
          <span className='text-[10px]'>{t('Add')}</span>
        </Button>
      </div>
    )
  }

  // Read-only array of media URLs (params preview): a wrapping row of small
  // display-only thumbs. Falls through to the generic renderer otherwise.
  if (previewMedia && !props.onChange && isMediaUrlArray(props.value)) {
    return (
      <div className='flex min-w-0 flex-wrap items-center gap-1.5'>
        {props.value.map((item, index) => {
          const kind = classifyMediaUrl(item)
          return kind ? (
            // eslint-disable-next-line react/no-array-index-key
            <MediaThumb key={index} url={item.trim()} kind={kind} size='sm' />
          ) : (
            <span
              // eslint-disable-next-line react/no-array-index-key
              key={index}
              className='text-muted-foreground max-w-full truncate text-[11px]'
            >
              {item}
            </span>
          )
        })}
      </div>
    )
  }

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
            <JsonForm value={value} path={[...path, index]} onChange={props.onChange} compact={compact} previewMedia={previewMedia} />
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
            <JsonForm value={value} path={[...path, key]} onChange={props.onChange} compact={compact} previewMedia={previewMedia} />
          </div>
        ))}
      </div>
    )
  }
  if (!props.onChange) {
    // Params preview: a single media URL renders as a small display-only thumb.
    if (previewMedia && typeof props.value === 'string') {
      const kind = classifyMediaUrl(props.value)
      if (kind) {
        return (
          <div className='py-0.5'>
            <MediaThumb url={props.value.trim()} kind={kind} size='sm' />
          </div>
        )
      }
    }
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
  // A single string field holding a media URL collapses into a preview box;
  // clearing it (setting it empty) restores the input on the next render.
  if (typeof props.value === 'string') {
    const kind = classifyMediaUrl(props.value)
    if (kind) {
      return (
        <MediaThumb
          url={props.value.trim()}
          kind={kind}
          onClear={() => props.onChange?.(path, '')}
        />
      )
    }
  }
  return (
    <Input
      type={typeof props.value === 'number' ? 'number' : 'text'}
      className={compact ? 'h-7 px-2 text-xs' : undefined}
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
    }
  } catch {
    return { scriptId: 0, version: 1, configText: DEFAULT_CONFIG_TEXT }
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
  onChange: (localId: string, patch: Partial<QueuedTask>) => void
  onCancel: (orderId: string) => void
  onDelete: (localId: string) => void
}
function TaskCard({ task, onChange, onCancel, onDelete }: TaskCardProps) {
  const { t } = useTranslation()
  const expanded = task.expanded === true
  // Fullscreen media preview (image/video) opened from the compact thumbnail.
  const [mediaOpen, setMediaOpen] = useState(false)
  const [detectedMedia, setDetectedMedia] = useState<FoundMedia | null>(null)
  // Expanded body is split into two top-level tabs: Result (default) and
  // Parameters. The Parameters tab has its own visual/JSON sub-toggle.
  const [activeTab, setActiveTab] = useState<'result' | 'params'>('result')
  const [paramsView, setParamsView] = useState<ViewMode>('form')
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

  // Parse the relay result once, then derive both the media preview and the
  // structured views from it.
  let parsedResult: unknown
  let parseOk = false
  if (task.relayResult) {
    try {
      parsedResult = JSON.parse(task.relayResult)
      parseOk = true
    } catch {
      parseOk = false
    }
  }
  const knownMedia = parseOk ? findFirstMedia(parsedResult) : null
  const media = knownMedia ?? detectedMedia

  useEffect(() => {
    setDetectedMedia(null)
    if (!parseOk || knownMedia) return
    let cancelled = false
    void detectFirstMedia(parsedResult).then((found) => {
      if (cancelled) return
      setDetectedMedia(found)
      if (found) {
        onChange(task.localId, { expanded: true, resultView: 'preview' })
      }
    })
    return () => { cancelled = true }
    // parsedResult and knownMedia are derived from this serialized value.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [task.relayResult])
  // Fall back off the preview tab if it's selected but there's nothing to show.
  const resultView: ResultView =
    task.resultView === 'preview' && !media ? 'form' : task.resultView

  let previewNode: React.ReactNode = null
  if (media) {
    previewNode = (
      <div className='flex flex-col items-center gap-2 rounded-md border bg-muted/10 p-3'>
        <div className='flex max-w-[220px] items-center justify-center overflow-hidden rounded-md bg-muted/40'>
          {media.kind === 'image' && (
            <button
              type='button'
              className='group relative block cursor-zoom-in'
              aria-label={t('Preview image')}
              onClick={() => setMediaOpen(true)}
            >
              <img className='max-h-40 w-auto object-contain' src={media.url} alt='' loading='lazy' />
              <span className='absolute right-1.5 bottom-1.5 flex h-7 w-7 items-center justify-center rounded-md bg-black/65 text-white opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100'>
                <ZoomIn className='h-4 w-4' aria-hidden='true' />
              </span>
            </button>
          )}
          {media.kind === 'video' && (
            <video className='max-h-40 w-auto object-contain' src={media.url} controls preload='metadata' onClick={() => setMediaOpen(true)} />
          )}
          {media.kind === 'audio' && (
            <div className='flex w-[200px] flex-col items-center gap-2 px-2 py-3'>
              <FileAudio className='h-7 w-7 text-muted-foreground' />
              <audio className='h-8 w-full' src={media.url} controls preload='metadata' />
            </div>
          )}
        </div>
      </div>
    )
  }

  let structuredNode: React.ReactNode = null
  if (task.relayResult) {
    if (parseOk) {
      structuredNode =
        resultView === 'json' ? (
          // whitespace-pre-wrap prevents horizontal overflow; the card stays within its column
          <pre className='bg-muted/30 max-h-60 overflow-auto rounded-md border p-2 text-xs whitespace-pre-wrap break-all'>
            {task.relayResult}
          </pre>
        ) : (
          // compact + scrollable container — result data can be large
          <div className='max-h-60 overflow-y-auto rounded-md border bg-muted/10 p-2'>
            <JsonForm value={parsedResult} compact />
          </div>
        )
    } else {
      structuredNode = (
        <pre className='bg-muted/30 max-h-60 overflow-auto rounded-md border p-2 text-xs whitespace-pre-wrap break-all'>
          {task.relayResult}
        </pre>
      )
    }
  }

  let resultNode: React.ReactNode = null
  if (task.relayResult) {
    resultNode = resultView === 'preview' ? previewNode : structuredNode
  }

  // Sent parameters — parsed once for the visual view; falls back to raw text.
  let paramsParsed: unknown
  let paramsOk = false
  if (task.configText) {
    try {
      paramsParsed = JSON.parse(task.configText)
      paramsOk = true
    } catch {
      paramsOk = false
    }
  }

  return (
    <div className='shrink-0 overflow-hidden rounded-lg border text-sm'>
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
        {task.status !== 'submitting' && task.status !== 'running' && (
          <button
            type='button'
            className='text-muted-foreground shrink-0 p-1 hover:text-destructive'
            onClick={() => onDelete(task.localId)}
            aria-label={t('Delete task')}
            title={t('Delete task')}
          >
            <Trash2 className='h-4 w-4' aria-hidden='true' />
          </button>
        )}
        <button
          type='button'
          className='text-muted-foreground hover:text-foreground shrink-0 p-1'
          onClick={() => onChange(task.localId, { expanded: !expanded })}
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
            // Order id + state share one line; the node/cancel affordances follow.
            <div className='text-xs space-y-1'>
              <div className='flex flex-wrap items-center gap-x-3 gap-y-1'>
                <span className='font-mono text-muted-foreground min-w-0 truncate'>{task.order.id}</span>
                <span className='shrink-0'>
                  {t('State')}: <b>{task.order.state}</b>
                </span>
              </div>
              {task.order.chosen_node_id && (
                <div className='truncate'>
                  {t('Node')}: <span className='font-mono'>{task.order.chosen_node_id}</span>
                </div>
              )}
              {canCancel && (
                <Button size='sm' variant='outline' onClick={() => { if (task.order) onCancel(task.order.id) }}>
                  {t('Cancel order')}
                </Button>
              )}
            </div>
          )}

          {/* Two top-level tabs: Parameters and Result. Result keeps its own
              preview/visual/JSON sub-toggle. Splitting them keeps the card short
              so neither section gets clipped by the card's overflow. */}
          <div className='flex gap-1 border-b' role='tablist'>
            <button
              type='button'
              role='tab'
              aria-selected={activeTab === 'params'}
              className={`-mb-px border-b-2 px-3 py-1.5 text-xs font-medium transition-colors ${
                activeTab === 'params'
                  ? 'border-foreground text-foreground'
                  : 'border-transparent text-muted-foreground hover:text-foreground'
              }`}
              onClick={() => setActiveTab('params')}
            >
              {t('Parameters')}
            </button>
            <button
              type='button'
              role='tab'
              aria-selected={activeTab === 'result'}
              className={`-mb-px border-b-2 px-3 py-1.5 text-xs font-medium transition-colors ${
                activeTab === 'result'
                  ? 'border-foreground text-foreground'
                  : 'border-transparent text-muted-foreground hover:text-foreground'
              }`}
              onClick={() => setActiveTab('result')}
            >
              {t('Result')}
            </button>
          </div>

          {/* Result tab */}
          {activeTab === 'result' && (
            <div className='min-w-0'>
              {resultNode ? (
                <>
                  {/* Sub-toggle: preview (media) / visual / JSON */}
                  <div className='mb-2 flex justify-end'>
                    <div className='flex shrink-0 gap-1' role='group'>
                      {media && (
                        <Button
                          type='button' size='sm'
                          variant={resultView === 'preview' ? 'secondary' : 'ghost'}
                          onClick={() => onChange(task.localId, { resultView: 'preview' })}
                        >
                          <Eye className='mr-1 h-3 w-3' />{t('Preview')}
                        </Button>
                      )}
                      <Button
                        type='button' size='sm'
                        variant={resultView === 'form' ? 'secondary' : 'ghost'}
                        onClick={() => onChange(task.localId, { resultView: 'form' })}
                      >
                        <ListTree className='mr-1 h-3 w-3' />{t('Visual')}
                      </Button>
                      <Button
                        type='button' size='sm'
                        variant={resultView === 'json' ? 'secondary' : 'ghost'}
                        onClick={() => onChange(task.localId, { resultView: 'json' })}
                      >
                        <Braces className='mr-1 h-3 w-3' />JSON
                      </Button>
                    </div>
                  </div>
                  {resultNode}
                </>
              ) : (
                <div className='text-muted-foreground py-6 text-center text-xs'>
                  {t('No result yet')}
                </div>
              )}
            </div>
          )}

          {/* Parameters tab — visual (with media previews) or JSON view. */}
          {activeTab === 'params' && (
            <div className='min-w-0'>
              {task.configText ? (
                <>
                  {paramsOk && (
                    <div className='mb-2 flex justify-end'>
                      <div className='flex shrink-0 gap-1' role='group'>
                        <Button
                          type='button' size='sm'
                          variant={paramsView === 'form' ? 'secondary' : 'ghost'}
                          onClick={() => setParamsView('form')}
                        >
                          <ListTree className='mr-1 h-3 w-3' />{t('Visual')}
                        </Button>
                        <Button
                          type='button' size='sm'
                          variant={paramsView === 'json' ? 'secondary' : 'ghost'}
                          onClick={() => setParamsView('json')}
                        >
                          <Braces className='mr-1 h-3 w-3' />JSON
                        </Button>
                      </div>
                    </div>
                  )}
                  {paramsOk && paramsView === 'form' ? (
                    <div className='max-h-60 overflow-y-auto rounded-md border bg-muted/10 p-2'>
                      <JsonForm value={paramsParsed} compact previewMedia />
                    </div>
                  ) : (
                    <pre className='bg-muted/30 max-h-60 overflow-auto rounded-md border p-2 text-xs whitespace-pre-wrap break-all'>
                      {task.configText}
                    </pre>
                  )}
                </>
              ) : (
                <div className='text-muted-foreground py-6 text-center text-xs'>
                  {t('No parameters')}
                </div>
              )}
            </div>
          )}
        </div>
      )}

      {/* Fullscreen preview for image/video — opened by clicking the thumbnail */}
      {media && (media.kind === 'image' || media.kind === 'video') && (
        <Dialog open={mediaOpen} onOpenChange={setMediaOpen}>
          <DialogContent closeLabel={t('Close')} className='flex h-[calc(100vh-2rem)] w-[calc(100vw-2rem)] items-center justify-center overflow-hidden bg-black/95 p-4 text-white sm:max-w-[calc(100vw-2rem)]' aria-label={t('Preview')}>
            <DialogTitle className='sr-only'>{t('Preview')}</DialogTitle>
            {media.kind === 'image' ? (
              <img className='max-h-full max-w-full object-contain' src={media.url} alt='' />
            ) : (
              // autoPlay so opening the big view starts playback immediately
              <video className='max-h-full max-w-full object-contain' src={media.url} controls autoPlay />
            )}
          </DialogContent>
        </Dialog>
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
  const [assetLibraryOpen, setAssetLibraryOpen] = useState(false)
  // Usage notes are open by default; the user's preference persists across visits.
  const [descExpanded, setDescExpanded] = useState(loadDescriptionExpanded)

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
        JSON.stringify({ scriptId, version, configText } satisfies PurchaseDraft)
      )
    } catch { /* best-effort */ }
  }, [scriptId, version, configText])

  useEffect(() => {
    try { window.localStorage.setItem(getViewModeStorageKey('parameters'), parametersView) }
    catch { /* best-effort */ }
  }, [parametersView])

  useEffect(() => {
    try { window.localStorage.setItem(getDescriptionExpandedStorageKey(), String(descExpanded)) }
    catch { /* best-effort */ }
  }, [descExpanded])

  // Re-quote when rules become available or any pricing input changes. On a
  // refresh, the saved params exist before the version metadata is loaded.
  useEffect(() => {
    if (!scriptId) return
    const scriptVer = availableVersions.find((v) => v.version === version)
    if (!scriptVer?.pricing_rules?.length) return
    let cancelled = false
    let mult = 1
    try {
      const config = JSON.parse(configText) as unknown
      mult = Math.max(
        1,
        Math.round(computeParamsMultiplier(config, scriptVer.pricing_rules))
      )
    } catch { /* invalid JSON uses the base price */ }
    void quoteOrder({
      script_id: scriptId, version,
      node_id: !autoSelect && nodeId ? nodeId : undefined,
      provider_group_id: autoSelect ? groupFilterId || undefined : undefined,
      consume_multiplier: mult,
    })
      .then((p) => { if (!cancelled) setQuote(p.breakdown) })
      .catch(() => { if (!cancelled) setQuote(null) })
    return () => { cancelled = true }
  }, [
    scriptId,
    version,
    availableVersions,
    configText,
    autoSelect,
    nodeId,
    groupFilterId,
  ])

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
    groupId = groupFilterId,
    preserveSelection = false,
    multiplierOverride?: number
  ) {
    setOffersLoading(true); setOffersPage(0); setQuote(null)
    try {
      const loaded = await listScriptOffers(selectedScriptId, selectedVersion, groupId || undefined)
      const sorted = [...loaded].sort((a, b) => Number(b.owned) - Number(a.owned))
      const selectedNodeId = preserveSelection && !autoSelect && loaded.some((offer) => offer.node_id === nodeId) ? nodeId : ''
      setOffers(sorted)
      if (!selectedNodeId) { setAutoSelect(true); setNodeId('') }
      try {
        const mult = multiplierOverride ?? getEffectiveMultiplier()
        const priced = await quoteOrder({
          script_id: selectedScriptId, version: selectedVersion,
          node_id: selectedNodeId || undefined,
          provider_group_id: selectedNodeId ? undefined : groupId || undefined,
          consume_multiplier: mult,
        })
        setQuote(priced.breakdown)
      } catch { setQuote(null) }
      if (loaded.length === 0) { toast.info(t('No provider offers yet for this version')) }
    } finally { setOffersLoading(false) }
  }

  // Mirrors the currently-displayed selection so an async task that finishes
  // long after it was fired can tell whether the offers panel still shows the
  // same script/version before refreshing it (the user may have switched away).
  const viewedSelectionRef = useRef({ scriptId, version, groupFilterId })
  useEffect(() => {
    viewedSelectionRef.current = { scriptId, version, groupFilterId }
  }, [scriptId, version, groupFilterId])

  // refreshOffersAfterTask re-fetches provider offers for a task's script once it
  // settles, so availability (busy/idle, remaining quota, free slots) reflects
  // the run that just finished. Skips the refresh if the panel has since moved
  // to a different script/version, so it never clobbers what the user is viewing.
  function refreshOffersAfterTask(taskScriptId: number, taskVersion: number) {
    const viewed = viewedSelectionRef.current
    if (viewed.scriptId !== taskScriptId || viewed.version !== taskVersion) return
    void loadOffersFor(viewed.scriptId, viewed.version, viewed.groupFilterId, true)
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
      const selected = available.find((item) => item.version === selectedVersion)
      let selectedConfigText = configText
      if (loadParams) {
        selectedConfigText = configTextFromParams(selected?.script_params)
        setConfigText(selectedConfigText)
      }
      let multiplier = 1
      if (selected?.pricing_rules?.length) {
        try {
          multiplier = Math.max(
            1,
            Math.round(
              computeParamsMultiplier(
                JSON.parse(selectedConfigText) as unknown,
                selected.pricing_rules
              )
            )
          )
        } catch { /* invalid JSON uses the base price */ }
      }
      await loadOffersFor(value, selectedVersion, groupFilterId, false, multiplier)
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
      } else if (list[0]) {
        await selectScript(list[0].id, undefined, list[0].latest_version)
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
    for (const task of pending) {
      if (task.order?.id) void reconcileOne(task.localId, task.order.id)
    }
    return () => { cancelled = true }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // getEffectiveMultiplier computes the combined params multiplier from the
  // current configText and the selected script version's pricing rules.
  // Returns 1 when there are no rules or the config is invalid JSON.
  function getEffectiveMultiplier(): number {
    const scriptVer = availableVersions.find((v) => v.version === version)
    if (!scriptVer?.pricing_rules?.length) return 1
    try {
      const cfg = JSON.parse(configText) as unknown
      return Math.max(1, Math.round(computeParamsMultiplier(cfg, scriptVer.pricing_rules)))
    } catch { return 1 }
  }

  function selectAuto() {
    setAutoSelect(true); setNodeId(''); setQuote(null)
    if (scriptId) {
      void quoteOrder({ script_id: scriptId, version, provider_group_id: groupFilterId || undefined, consume_multiplier: getEffectiveMultiplier() })
        .then((p) => setQuote(p.breakdown)).catch(() => setQuote(null))
    }
  }

  function selectProvider(selectedNodeId: string) {
    setAutoSelect(false); setNodeId(selectedNodeId); setQuote(null)
    void quoteOrder({ script_id: scriptId, version, node_id: selectedNodeId, consume_multiplier: getEffectiveMultiplier() })
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
      // No provider matched: dispatch found no idle candidate (all providers busy
      // or offline), so the order stays in a pre-offer state with no chosen node.
      // A dispatched order is OFFERED or beyond by now (the backend waits briefly
      // for the provider to accept). Fail fast with a clear message and refund
      // immediately, rather than connecting the relay and waiting out the provider
      // handshake — which would otherwise surface as a confusing "handshake timeout".
      if ((o.state === 'MATCHING' || o.state === 'FUNDS_RESERVED') && !o.chosen_node_id) {
        try { upd({ order: await cancelOrder(o.id) }) } catch { /* already terminal; refund still lands */ }
        upd({
          status: 'failed',
          error: t('No idle provider available right now (all providers are busy or offline). Please try again shortly.'),
          relayStatus: '',
        })
        await loadBalance(); return
      }
      await loadBalance()
      const config = JSON.parse(cleanedConfigText)
      // NOTE: timeoutMs from the user config is intentionally NOT used to
      // control the relay result timeout. Allowing user-supplied timeouts would
      // create a refund exploit: a client could send an extremely short timeout,
      // receive a refund, and still have the provider consume resources executing
      // the task. The relay session always uses its own fixed 60-minute default.
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
        // Known extensions can switch to preview immediately. TaskCard probes
        // extensionless signed URLs in the background after showing the result.
        const hasMedia = findFirstMedia(result) != null
        upd({
          status: 'success',
          relayResult: resultText,
          relayStatus: t('Result received'),
          ...(hasMedia ? { expanded: true, resultView: 'preview' as ResultView } : {}),
        })
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
    } finally {
      // Task settled (success/fail/refund) — refresh offers so provider
      // availability reflects the run that just finished.
      refreshOffersAfterTask(taskScriptId, taskVersion)
    }
  }

  async function onPurchase() {
    if (!scriptId) { toast.error(t('Select a script first')); return }
    let inputHash = '', cleanedConfigText = ''
    let config: unknown
    try {
      config = cleanEmptyArrayItems(JSON.parse(configText))
      cleanedConfigText = JSON.stringify(config, null, 2)
      setConfigText(cleanedConfigText)
      inputHash = await sha256Hex(JSON.stringify(config))
    } catch { toast.error(t('Config must be valid JSON')); return }
    // Capture current provider selection before async state changes
    const capturedAutoSelect = autoSelect
    const capturedNodeId = nodeId
    const capturedGroupFilterId = groupFilterId
    // Compute effective multiplier from pricing rules
    const scriptVer = availableVersions.find((v) => v.version === version)
    const capturedMultiplier = scriptVer?.pricing_rules?.length
      ? Math.max(1, Math.round(computeParamsMultiplier(config, scriptVer.pricing_rules)))
      : 1
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
    // Refresh offers now that a run is starting — the chosen provider will show
    // as busy / with reduced free slots. Preserve the current selection so the
    // quote and picked provider aren't reset out from under the user.
    void loadOffersFor(scriptId, version, groupFilterId, true)
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
  const runningTaskCount = taskQueue.filter((task) => task.status === 'running' || task.status === 'submitting').length

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>
        <span className='inline-flex flex-wrap items-baseline gap-x-2 gap-y-0.5'>
          {t('AiToken P2P Marketplace')}
          <span className='text-[11px] font-normal text-red-500'>
            {t('All P2P data is cached only in your browser and is not stored on the server')}
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
        {/* Fixed-height content: a scrollable body over a pinned action bar.
            The parent content region is overflow-hidden (fixedContent), so this
            column owns the height and hands scrolling to each panel below. On
            mobile the whole body scrolls as one; on lg each column scrolls
            independently so a long task queue never hides behind the bar. */}
        <div className='flex h-full min-h-0 flex-col gap-4'>
        {/* Two-column layout: form left, task queue right.
            min-w-0 on both columns is required — grid tracks default to
            min-width:auto, so long text/URLs would otherwise blow a column
            past its fr share and squeeze the other. */}
        <div className='grid min-h-0 flex-1 grid-cols-1 gap-4 overflow-y-auto lg:grid-cols-[3fr_2fr] lg:grid-rows-[minmax(0,1fr)] lg:items-stretch lg:overflow-hidden'>
          {/* LEFT: configuration form — scrolls on its own at lg */}
          <div className='flex min-w-0 flex-col gap-4 lg:min-h-0 lg:overflow-y-auto lg:pr-1'>

            {/* Provider selection card */}
            <div className='rounded-lg border p-4'>
              <div className='mb-2 flex flex-wrap items-center justify-between gap-2'>
                <div className='text-sm font-medium'>{t('Provider offers (Auto picks the best idle provider, or choose one)')}</div>
                <Button type='button' variant='outline' size='sm' onClick={() => {
                  if (!scriptId) { toast.error(t('Select a script first')); return }
                  void loadOffersFor(scriptId, version, groupFilterId, true)
                }} disabled={offersLoading}>
                  <RefreshCw className={`mr-2 h-4 w-4 ${offersLoading ? 'animate-spin' : ''}`} aria-hidden='true' />
                  {t('Refresh')}
                </Button>
              </div>
              <div>
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

          {/* RIGHT: Task queue panel — owns its own scroll at lg so a long
              queue scrolls inside the panel instead of behind the action bar */}
          <div className='flex min-w-0 flex-col gap-3 lg:min-h-0'>
            <div className='flex shrink-0 flex-wrap items-center justify-between gap-2'>
              <div className='inline-flex flex-wrap items-baseline gap-x-2 gap-y-0.5'>
                <span className='text-sm font-medium'>{t('Task queue')}</span>
                {/* Refresh drops any in-flight run's encrypted relay connection. */}
                <span className='text-[11px] text-red-500'>
                  {t('Keep this page open and do not refresh while tasks are running, as this will interrupt them')}
                </span>
              </div>
              {taskQueue.length > 0 && (
                <div className='flex items-center gap-2'>
                  <span className='text-muted-foreground text-xs'>
                    {runningTaskCount > 0 && (
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
                <div className='text-muted-foreground mt-1 text-xs'>{t('Select a script and provider, then click "Purchase and run" to start')}</div>
              </div>
            ) : (
              <div className='flex flex-col gap-2 lg:h-0 lg:min-h-0 lg:flex-1 lg:overflow-y-auto lg:overscroll-contain lg:pr-1'>
                {taskQueue.map((task) => (
                  <TaskCard
                    key={task.localId}
                    task={task}
                    onChange={updateTask}
                    onCancel={onCancelTask}
                    onDelete={(localId) => {
                      setTaskQueue((current) =>
                        current.filter((item) => item.localId !== localId)
                      )
                    }}
                  />
                ))}
              </div>
            )}
          </div>
        </div>

        {/* Full-width action bar — pinned below the scrollable body as a flex
            sibling (shrink-0), so it's always visible without overlapping. */}
        <div className='shrink-0 rounded-lg border border-white/15 bg-black/80 px-4 py-4 text-white shadow-xl backdrop-blur-xl'>
          <div className='grid gap-4 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-start'>
            <div className='grid min-w-0 grid-cols-[6rem_minmax(0,1fr)] gap-3'>
              <button
                type='button'
                className='group flex min-h-[5.75rem] w-24 flex-col items-center justify-center gap-2 rounded-md border border-white/25 bg-white/[0.06] px-2 shadow-[inset_0_1px_0_rgba(255,255,255,0.08),0_8px_24px_rgba(0,0,0,0.24)] transition-colors hover:border-white/45 hover:bg-white/[0.1] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/70 focus-visible:ring-offset-2 focus-visible:ring-offset-black'
                onClick={() => setAssetLibraryOpen(true)}
                aria-label={t('Open resource library')}
                title={t('Open resource library')}
              >
                <span className='flex h-9 w-9 items-center justify-center rounded-md border border-white/20 bg-black/25 text-white/80 shadow-[0_5px_14px_rgba(0,0,0,0.28)] transition-colors group-hover:border-white/35 group-hover:bg-white/10 group-hover:text-white'>
                  <Plus className='h-4 w-4' aria-hidden='true' />
                </span>
                <span className='text-xs font-medium text-white/80 group-hover:text-white'>
                  {t('Resource upload')}
                </span>
              </button>
              <div className='min-w-0 space-y-3'>
                <div className='flex flex-wrap items-center gap-2'>
                  <span className='mr-1 text-sm font-medium'>{t('Script')}</span>
                  <select className='h-9 w-64 max-w-full min-w-0 shrink rounded-md border border-white/20 bg-white/10 px-2 text-sm text-white outline-none focus:border-white/50 [&>option]:bg-white [&>option]:text-black' value={scriptId} onChange={(e) => void selectScript(Number(e.target.value))}>
                    <option value={0}>{t('Select a script')}</option>
                    {scripts.map((s) => (<option key={s.id} value={s.id}>#{s.id} {s.title}</option>))}
                  </select>
                  <select className='h-9 w-28 rounded-md border border-white/20 bg-white/10 px-2 text-sm text-white outline-none focus:border-white/50 disabled:text-white/40 [&>option]:bg-white [&>option]:text-black' value={version} disabled={!scriptId || versions.length === 0} aria-label={t('Version')} onChange={(e) => {
                    const v = Number(e.target.value); setVersion(v); setOffers([]); setNodeId(''); setAutoSelect(true); setOffersPage(0); setQuote(null)
                    const sel = availableVersions.find((item) => item.version === v)
                    const nextConfigText = configTextFromParams(sel?.script_params)
                    setConfigText(nextConfigText)
                    let multiplier = 1
                    if (sel?.pricing_rules?.length) {
                      try {
                        multiplier = Math.max(1, Math.round(computeParamsMultiplier(JSON.parse(nextConfigText) as unknown, sel.pricing_rules)))
                      } catch { /* invalid JSON uses the base price */ }
                    }
                    if (scriptId) void loadOffersFor(scriptId, v, groupFilterId, false, multiplier)
                  }}>
                    {versions.map((item) => (<option key={item} value={item}>v{item}</option>))}
                  </select>
                </div>
                <div className='flex items-start gap-2 rounded-md border border-white/15 bg-white/5 px-3 py-2 text-xs'>
                  {selectedScript?.description ? (
                    <>
                      <span className='shrink-0 font-medium text-white/60'>{t('Script description')}</span>
                      <span className={descExpanded ? 'min-w-0 flex-1 select-text break-words whitespace-pre-wrap' : 'min-w-0 flex-1 select-text truncate'}>{selectedScript.description}</span>
                      <button type='button' className='shrink-0 rounded p-0.5 text-white/60 hover:bg-white/10 hover:text-white' onClick={() => setDescExpanded((value) => !value)} aria-expanded={descExpanded} aria-label={descExpanded ? t('Collapse') : t('Expand')}>
                        {descExpanded
                          ? <ChevronUp className='h-3.5 w-3.5' aria-hidden='true' />
                          : <ChevronDown className='h-3.5 w-3.5' aria-hidden='true' />}
                      </button>
                    </>
                  ) : (
                    <span className='text-white/60'>{t('Upload resources and copy URLs for script parameters')}</span>
                  )}
                </div>
              </div>
            </div>

            <div className='flex min-w-64 flex-col items-stretch gap-2 lg:items-end'>
              <div className='flex flex-wrap items-center justify-between gap-x-4 gap-y-2 lg:justify-end'>
                <div className='text-sm'>
                  <span className='text-white/60'>{t('Total')}: </span>
                  <span className='text-lg font-semibold'>{quote ? microsToCurrency(quote.MaxCustomerMicros) : '-'}</span>
                </div>
                <Button className='min-w-40 bg-white text-black hover:bg-white/90 disabled:bg-white/20 disabled:text-white/40' onClick={() => void onPurchase()} disabled={!quote || insufficientBalance}>
                  {t('Purchase and run')}
                </Button>
              </div>
              {insufficientBalance && <div className='text-xs text-red-300'>{t('Insufficient balance')}</div>}
              {quote && (
                <div className='flex flex-wrap justify-end gap-x-4 gap-y-1 text-xs text-white/60'>
                  <span>{t('Provider')}: {microsToCurrency(quote.ProviderMicros)}</span>
                  <span>{t('Author')}: {microsToCurrency(quote.AuthorMicros)}</span>
                  <span>{t('Platform fee')}: {microsToCurrency(quote.PlatformFeeMicros)}</span>
                  {(() => {
                    const scriptVer = availableVersions.find((v) => v.version === version)
                    if (!scriptVer?.pricing_rules?.length) return null
                    try {
                      const cfg = JSON.parse(configText)
                      const mult = computeParamsMultiplier(cfg, scriptVer.pricing_rules)
                      if (mult <= 1) return null
                      return (
                        <span className='text-white/80 font-medium'>
                          {t('Params')}: ×{mult.toFixed(1)}
                        </span>
                      )
                    } catch { return null }
                  })()}
                </div>
              )}
            </div>
          </div>
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

        <AssetLibraryDialog open={assetLibraryOpen} onOpenChange={setAssetLibraryOpen} />

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
