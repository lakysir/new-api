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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { api } from '@/lib/api'

import {
  createOrder,
  cancelOrder,
  getLedgerBalances,
  getOrder,
  listCategories,
  listScriptOffers,
  listAvailableScriptVersions,
  quoteOrder,
  type ScriptOffer,
} from './api'
import { ClientRelaySession } from './lib/client-relay-session'
import { microsToDisplay } from './lib/format'
import type { LedgerBalances, Order, PriceBreakdown } from './types'

type PublishedScript = { id: number; title: string; description?: string; latest_version?: number }
type PurchaseDraft = { scriptId: number; version: number; configText: string }

const DEFAULT_CONFIG_TEXT = '{\n  "prompt": "a dog"\n}'

function getDraftStorageKey() {
  const userId = window.localStorage.getItem('uid') ?? 'anonymous'
  return `aitoken-purchase-draft:${userId}`
}

function loadPurchaseDraft(): PurchaseDraft {
  try {
    const saved = JSON.parse(window.localStorage.getItem(getDraftStorageKey()) ?? '{}') as Partial<PurchaseDraft>
    return {
      scriptId: Number.isInteger(saved.scriptId) && (saved.scriptId ?? 0) > 0 ? saved.scriptId! : 0,
      version: Number.isInteger(saved.version) && (saved.version ?? 0) > 0 ? saved.version! : 1,
      configText: typeof saved.configText === 'string' ? saved.configText : DEFAULT_CONFIG_TEXT,
    }
  } catch {
    return { scriptId: 0, version: 1, configText: DEFAULT_CONFIG_TEXT }
  }
}

// Terminal order states that mean execution will never produce a result, so the
// client should stop waiting on the relay and report the reason.
const TERMINAL_FAILURE_STATES = ['FAILED', 'REFUNDED', 'TIMED_OUT', 'CANCELLED']

// describeOrderError maps a provider/gate error_code to a readable reason. Falls
// back to the raw code (or a generic message) for anything not enumerated.
function describeOrderError(code: string | undefined): string {
  switch (code) {
    case 'ORIGIN_NOT_ALLOWED':
      return 'Provider has no open tab on the target site (origin not allowed). The provider must open and log into the target site, then retry.'
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

// sha256Hex hashes the config text so only the input_hash crosses the control
// plane (the plaintext config travels the E2EE data plane at execution time).
async function sha256Hex(text: string): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(text))
  return (
    'sha256:' +
    [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, '0')).join('')
  )
}

export function AitokenPurchasePage() {
  const { t } = useTranslation()
  const [initialDraft] = useState(loadPurchaseDraft)
  const [bal, setBal] = useState<LedgerBalances | null>(null)
  const [scripts, setScripts] = useState<PublishedScript[]>([])
  const [scriptId, setScriptId] = useState(initialDraft.scriptId)
  const [version, setVersion] = useState(initialDraft.version)
  const [versions, setVersions] = useState<number[]>([])
  const [offers, setOffers] = useState<ScriptOffer[]>([])
  const [nodeId, setNodeId] = useState('') // chosen offer; empty = cheapest
  const [configText, setConfigText] = useState(initialDraft.configText)
  const [quote, setQuote] = useState<PriceBreakdown | null>(null)
  const [order, setOrder] = useState<Order | null>(null)
  const [busy, setBusy] = useState(false)
  const [running, setRunning] = useState(false)
  const [relayResult, setRelayResult] = useState<string>('')
  const [relayStatus, setRelayStatus] = useState<string>('')
  const [offersLoading, setOffersLoading] = useState(false)

  async function selectScript(
    value: number,
    preferredVersion?: number,
    fallbackVersion?: number
  ) {
    setScriptId(value)
    setOffers([])
    setNodeId('')
    setQuote(null)
    setOrder(null)
    if (!value) {
      setVersions([])
      return
    }
    try {
      const available = await listAvailableScriptVersions(value)
      const values = available.map((item) => item.version).sort((a, b) => b - a)
      setVersions(values)
      const selectedVersion =
        (preferredVersion && values.includes(preferredVersion) ? preferredVersion : undefined) ??
        values[0] ??
        fallbackVersion ??
        1
      setVersion(selectedVersion)
      await loadOffersFor(value, selectedVersion)
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  async function loadOffersFor(selectedScriptId: number, selectedVersion: number) {
    setOffersLoading(true)
    try {
      const loaded = await listScriptOffers(selectedScriptId, selectedVersion)
      setOffers(loaded)
      const selectedNodeId = loaded.find((item) => item.available)?.node_id ?? ''
      setNodeId(selectedNodeId)
      if (selectedNodeId) {
        const priced = await quoteOrder({
          script_id: selectedScriptId,
          version: selectedVersion,
          node_id: selectedNodeId,
        })
        setQuote(priced.breakdown)
      } else {
        setQuote(null)
      }
      if (loaded.length === 0) toast.info(t('No provider offers yet for this version'))
    } finally {
      setOffersLoading(false)
    }
  }

  async function loadBalance() {
    try {
      setBal(await getLedgerBalances())
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  async function loadScripts() {
    try {
      const [res, categories] = await Promise.all([
        api.get('/api/scripts/square', { params: { limit: 100 } }),
        listCategories(),
      ])
      const items = (res.data?.data?.items ??
        res.data?.items ??
        res.data?.data ??
        []) as PublishedScript[]
      const balanceScriptIds = new Set(
        categories.map((category) => category.balance_script_id).filter(Boolean)
      )
      const list = items.filter((script) => !balanceScriptIds.has(script.id))
      setScripts(list)
      const savedScript = list.find((item) => item.id === initialDraft.scriptId)
      if (savedScript) {
        await selectScript(initialDraft.scriptId, initialDraft.version, savedScript.latest_version)
      } else if (initialDraft.scriptId) {
        setScriptId(0)
      }
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  useEffect(() => {
    loadBalance()
    loadScripts()
  }, [])

  useEffect(() => {
    try {
      window.localStorage.setItem(
        getDraftStorageKey(),
        JSON.stringify({ scriptId, version, configText } satisfies PurchaseDraft)
      )
    } catch {
      // Storage may be unavailable or full; the page remains usable without persistence.
    }
  }, [scriptId, version, configText])

  async function loadOffers() {
    if (!scriptId) {
      toast.error(t('Select a script first'))
      return
    }
    try {
      await loadOffersFor(scriptId, version)
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  async function onQuote() {
    if (!scriptId) {
      toast.error(t('Select a script first'))
      return
    }
    try {
      const q = await quoteOrder({ script_id: scriptId, version, node_id: nodeId || undefined })
      setQuote(q.breakdown)
      if (q.chosen_node_id) setNodeId(q.chosen_node_id)
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  async function onQuoteForNode(selectedNodeId: string) {
    try {
      const priced = await quoteOrder({ script_id: scriptId, version, node_id: selectedNodeId })
      setQuote(priced.breakdown)
    } catch (e) {
      setQuote(null)
      toast.error(String((e as Error).message))
    }
  }

  async function onPurchase() {
    if (!scriptId) {
      toast.error(t('Select a script first'))
      return
    }
    // Validate config is JSON before ordering.
    let inputHash = ''
    try {
      const config = JSON.parse(configText)
      inputHash = await sha256Hex(JSON.stringify(config))
    } catch {
      toast.error(t('Config must be valid JSON'))
      return
    }
    setBusy(true)
    try {
      const key = `order-${Date.now()}-${Math.random().toString(36).slice(2)}`
      const { order: o } = await createOrder(
        { script_id: scriptId, version, node_id: nodeId || undefined, input_hash: inputHash },
        key
      )
      setOrder(o)
      if (o.state === 'REFUNDED') {
        toast.error(t('Provider rejected the task; reserved funds were refunded'))
        await loadBalance()
        return
      }
      toast.success(t('Order created and funds reserved'))
      await loadBalance()
      if (['RESERVED', 'DATA_READY', 'RUNNING'].includes(o.state)) {
        await runViaRelay(o)
      }
    } catch (e) {
      toast.error(String((e as Error).message))
    } finally {
      setBusy(false)
    }
  }

  async function refreshOrder() {
    if (!order) return
    try {
      setOrder(await getOrder(order.id))
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  async function onCancel() {
    if (!order) return
    try {
      setOrder(await cancelOrder(order.id))
      await loadBalance()
      toast.success(t('Order cancelled'))
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  // Send the config to the executing provider over the E2EE relay and wait for
  // the encrypted result. task_id == order id in the MVP; attempt 1.
  async function runViaRelay(targetOrder: Order | null = order) {
    if (!targetOrder) {
      toast.error(t('Create an order first'))
      return
    }
    let config: unknown
    try {
      config = JSON.parse(configText)
    } catch {
      toast.error(t('Config must be valid JSON'))
      return
    }
    const relayUrl = `${location.origin.replace(/^http/, 'ws')}/api/relay`
    const session = new ClientRelaySession({
      relayUrl,
      taskId: targetOrder.id,
      attempt: 1,
      clientDeviceId: `client-${targetOrder.client_id}`,
    })
    setRelayStatus(t('Connecting to relay...'))
    setRelayResult('')
    setRunning(true)
    // Poll the control plane while waiting for the encrypted result. If the
    // provider fails the task (e.g. no target tab / origin not allowed), the
    // order reaches a terminal failure state; we reject fast with the real
    // reason instead of blocking on the 120s result timeout.
    let cancelled = false
    const failFast = new Promise<never>((_, reject) => {
      const tick = async () => {
        while (!cancelled) {
          await new Promise((r) => window.setTimeout(r, 1500))
          if (cancelled) return
          try {
            const latest = await getOrder(targetOrder.id)
            setOrder(latest)
            if (TERMINAL_FAILURE_STATES.includes(latest.state)) {
              reject(new Error(describeOrderError(latest.last_error)))
              return
            }
          } catch {
            /* transient read error; keep polling */
          }
        }
      }
      void tick()
    })
    try {
      await session.connect()
      setRelayStatus(t('Waiting for provider handshake...'))
      await Promise.race([session.waitEstablished(), failFast])
      setRelayStatus(t('Sending config, waiting for result...'))
      await session.sendConfig(config)
      const result = await Promise.race([session.waitForResult(), failFast])
      setRelayResult(JSON.stringify(result, null, 2))
      setRelayStatus(t('Result received'))
      // Submit the client receipt (result hash) so the control plane can compare
      // both parties' receipts and settle on a match.
      try {
        const resultHash = await sha256Hex(JSON.stringify(result ?? null))
        await api.post(`/api/orders/${targetOrder.id}/receipts`, {
          task_id: targetOrder.id,
          attempt: 1,
          party: 'client',
          order_id: targetOrder.id,
          result_hash: resultHash,
        })
      } catch {
        /* receipt submit best-effort; reconciliation retries on next receipt */
      }
      setOrder(await getOrder(targetOrder.id))
      await loadBalance()
    } catch (e) {
      setRelayStatus('')
      try {
        const latest = await getOrder(targetOrder.id)
        setOrder(latest)
        // Still cancellable (never accepted by a provider): refund now.
        if (['FUNDS_RESERVED', 'MATCHING', 'OFFERED'].includes(latest.state)) {
          setOrder(await cancelOrder(targetOrder.id))
          await loadBalance()
          toast.error(t('Task was not accepted; reserved funds were refunded'))
        } else if (TERMINAL_FAILURE_STATES.includes(latest.state)) {
          await loadBalance()
        }
      } catch {
        // Preserve the original relay error when the order can no longer be read.
      }
      toast.error(String((e as Error).message))
    } finally {
      cancelled = true
      session.close()
      setRunning(false)
    }
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('AiToken P2P Marketplace')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button variant='outline' onClick={loadBalance}>
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        {/* Wallet */}
        <div className='mb-4 grid grid-cols-2 gap-3 md:grid-cols-4'>
          {[
            ['Available', bal?.client_available],
            ['Reserved', bal?.client_reserved],
          ].map(([label, v]) => (
            <div key={String(label)} className='rounded-lg border p-3'>
              <div className='text-muted-foreground text-xs'>{t(label as string)}</div>
              <div className='mt-1 text-lg font-semibold'>
                {microsToDisplay(v as number)} {bal?.currency || ''}
              </div>
            </div>
          ))}
        </div>

        {/* Script + version + offers */}
        <div className='rounded-lg border p-4'>
          <div className='mb-2 text-sm font-medium'>{t('Choose script & provider')}</div>
          <div className='flex flex-wrap items-center gap-2'>
            <select
              className='h-9 min-w-[220px] rounded-md border px-2 text-sm'
              value={scriptId}
              onChange={(e) => void selectScript(Number(e.target.value))}
            >
              <option value={0}>{t('Select a script')}</option>
              {scripts.map((s) => (
                <option key={s.id} value={s.id}>
                  #{s.id} {s.title}
                </option>
              ))}
            </select>
            <select
              className='h-9 w-28 rounded-md border px-2 text-sm'
              value={version}
              onChange={(e) => {
                const selectedVersion = Number(e.target.value)
                setVersion(selectedVersion)
                setOffers([])
                setNodeId('')
                setQuote(null)
                if (scriptId) void loadOffersFor(scriptId, selectedVersion)
              }}
            >
              {versions.map((item) => <option key={item} value={item}>v{item}</option>)}
            </select>
            <Button variant='outline' onClick={loadOffers} disabled={offersLoading}>
              {offersLoading ? t('Loading...') : t('View offers')}
            </Button>
          </div>

          {offers.length > 0 && (
            <div className='mt-3'>
              <div className='text-muted-foreground mb-1 text-xs'>
                {t('Provider offers (choose one, or leave to auto-pick cheapest)')}
              </div>
              <div className='flex flex-col gap-1'>
                {offers.map((o) => (
                  <label key={o.node_id} className='flex items-center gap-2 text-sm'>
                    <input
                      type='radio'
                      name='offer'
                      checked={nodeId === o.node_id}
                      disabled={!o.available}
                      onChange={() => {
                        setNodeId(o.node_id)
                        void onQuoteForNode(o.node_id)
                      }}
                    />
                    <span className='font-mono text-xs'>{o.node_id}</span>
                    <span className='font-semibold'>{microsToDisplay(o.price_micros)}</span>
                    <span>{o.online ? t('Online') : t('Offline')}</span>
                    <span className='text-muted-foreground text-xs'>
                      {t('quota')}: {o.remaining_quota}
                    </span>
                    {!o.available && (
                      <span className='text-xs text-red-600'>
                        {o.unavailable_reason === 'QUOTA_EXHAUSTED' && t('Quota exhausted')}
                        {o.unavailable_reason === 'NODE_OFFLINE' && t('Node offline')}
                        {o.unavailable_reason === 'CAPABILITY_TEST_EXPIRED' && t('Capability test expired')}
                        {o.unavailable_reason === 'BALANCE_CHECK_EXPIRED' && t('Balance check expired')}
                      </span>
                    )}
                  </label>
                ))}
              </div>
            </div>
          )}
        </div>

        {/* Config params */}
        <div className='mt-4 rounded-lg border p-4'>
          <div className='mb-2 text-sm font-medium'>{t('Task parameters (config JSON)')}</div>
          <Textarea
            className='min-h-[140px] font-mono text-xs'
            value={configText}
            onChange={(e) => setConfigText(e.target.value)}
          />
          <div className='text-muted-foreground mt-1 text-xs'>
            {t(
              'Only the hash of these parameters crosses the control plane; the plaintext travels the encrypted data plane to the provider.'
            )}
          </div>
        </div>

        {/* Quote + purchase */}
        <div className='mt-4 flex flex-wrap items-center gap-2'>
          <Button variant='outline' onClick={onQuote}>
            {t('Get quote')}
          </Button>
          <Button onClick={onPurchase} disabled={busy || running || !quote || quote.MaxCustomerMicros > (bal?.client_available ?? 0)}>
            {busy || running ? t('Running...') : t('Purchase and run')}
          </Button>
        </div>

        {quote && (
          <div className='mt-3 rounded-lg border p-4 text-sm'>
            <div className='mb-1 font-medium'>{t('Price breakdown')}</div>
            <div className='grid grid-cols-2 gap-x-6 gap-y-1 md:grid-cols-3'>
              <div>{t('Provider')}: {microsToDisplay(quote.ProviderMicros)}</div>
              <div>{t('Author')}: {microsToDisplay(quote.AuthorMicros)}</div>
              <div>{t('Platform fee')}: {microsToDisplay(quote.PlatformFeeMicros)}</div>
              <div>{t('Risk reserve')}: {microsToDisplay(quote.RiskReserveMicros)}</div>
              <div className='font-semibold'>
                {t('Total')}: {microsToDisplay(quote.MaxCustomerMicros)} {quote.Currency}
              </div>
            </div>
          </div>
        )}

        {order && (
          <div className='mt-4 rounded-lg border p-4 text-sm'>
            <div className='mb-1 flex items-center justify-between'>
              <span className='font-medium'>{t('Order')}</span>
              <Button size='sm' variant='ghost' onClick={refreshOrder}>
                {t('Refresh status')}
              </Button>
            </div>
            <div className='font-mono text-xs'>{order.id}</div>
            <div className='mt-1'>
              {t('State')}: <b>{order.state}</b> · {t('Reserved')}:{' '}
              {microsToDisplay(order.max_amount_micros)}
            </div>
            {order.chosen_node_id && (
              <div className='mt-1 text-xs'>
                {t('Provider node')}: <span className='font-mono'>{order.chosen_node_id}</span>
              </div>
            )}
            {['FUNDS_RESERVED', 'MATCHING', 'OFFERED'].includes(order.state) && (
              <Button className='mt-2' size='sm' variant='outline' onClick={onCancel}>
                {t('Cancel order')}
              </Button>
            )}

            {/* Dashboard sessions authenticate the E2EE relay automatically. */}
            <div className='mt-3 border-t pt-3'>
              <div className='mb-2 text-sm font-medium'>{t('Run task (send config over encrypted relay)')}</div>
              <div className='flex flex-wrap items-center gap-2'>
                <Button
                  onClick={() => void runViaRelay()}
                  disabled={running || !['OFFERED', 'RESERVED', 'DATA_READY', 'RUNNING'].includes(order.state)}
                >
                  {running ? t('Running...') : t('Send & run')}
                </Button>
                {relayStatus && <span className='text-muted-foreground text-xs'>{relayStatus}</span>}
              </div>
              {relayResult && (
                <pre className='mt-2 max-h-64 overflow-auto rounded-md border bg-muted/30 p-2 text-xs'>
                  {relayResult}
                </pre>
              )}
            </div>
          </div>
        )}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
