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
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { api, getSelf } from '@/lib/api'

import {
  createOrder,
  cancelOrder,
  getLedgerBalances,
  getOrder,
  listCategories,
  listScriptOffers,
  listAvailableScriptVersions,
  quoteOrder,
  rechargeAvailable,
  searchProviderGroups,
  type ProviderGroup,
  type ScriptOffer,
} from './api'
import { ClientRelaySession } from './lib/client-relay-session'
import { displayToMicros, microsToCurrency } from './lib/format'
import type { LedgerBalances, Order, PriceBreakdown, ScriptVersion } from './types'

type PublishedScript = { id: number; title: string; description?: string; latest_version?: number }
type PurchaseDraft = { scriptId: number; version: number; configText: string }

const DEFAULT_CONFIG_TEXT = '{\n  "prompt": "a dog"\n}'

// Providers per page in the offers list. The market can list many providers, so
// the list is paginated to stay readable (requirement: paginate when many).
const OFFERS_PAGE_SIZE = 10

// configTextFromParams turns a version's author-configured script_params into
// pretty-printed editor text. Falls back to the raw string (or the generic
// default) when params are absent or not valid JSON, so the editor is never
// left with placeholder content that would be sent to the provider verbatim.
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
  const [availableVersions, setAvailableVersions] = useState<ScriptVersion[]>([])
  const versions = availableVersions.map((item) => item.version).sort((a, b) => b - a)
  const [offers, setOffers] = useState<ScriptOffer[]>([])
  const [nodeId, setNodeId] = useState('') // chosen offer; empty when auto
  // Auto mode (default): the platform auto-picks the busiest, highest-success
  // idle provider (within the group filter, if any). Turning it off means the
  // client picks a specific provider below.
  const [autoSelect, setAutoSelect] = useState(true)
  // Provider-group filter: search a group by name to get its id, then offers and
  // the auto-pick are restricted to that group.
  const [groupQuery, setGroupQuery] = useState('')
  const [groupResults, setGroupResults] = useState<ProviderGroup[]>([])
  const [groupSearching, setGroupSearching] = useState(false)
  const [groupFilterId, setGroupFilterId] = useState('')
  const [groupFilterName, setGroupFilterName] = useState('')
  // Zero-based page index into the offers list.
  const [offersPage, setOffersPage] = useState(0)
  const [configText, setConfigText] = useState(initialDraft.configText)
  const [quote, setQuote] = useState<PriceBreakdown | null>(null)
  const [order, setOrder] = useState<Order | null>(null)
  const [busy, setBusy] = useState(false)
  const [running, setRunning] = useState(false)
  const [relayResult, setRelayResult] = useState<string>('')
  const [relayStatus, setRelayStatus] = useState<string>('')
  const [offersLoading, setOffersLoading] = useState(false)
  // Recharge (top up the marketplace available balance from the main wallet).
  const [walletQuota, setWalletQuota] = useState<number | null>(null)
  const [rechargeAmt, setRechargeAmt] = useState('1')
  const [recharging, setRecharging] = useState(false)

  // selectScript switches the active script and picks a version. When
  // loadParams is true (an explicit user switch) the config editor is reset to
  // that version's author-configured params; on the initial restore we keep the
  // user's saved draft instead of clobbering it.
  async function selectScript(
    value: number,
    preferredVersion?: number,
    fallbackVersion?: number,
    loadParams = true
  ) {
    setScriptId(value)
    setOffers([])
    setNodeId('')
    setAutoSelect(true)
    setOffersPage(0)
    setQuote(null)
    setOrder(null)
    if (!value) {
      setAvailableVersions([])
      return
    }
    try {
      const available = await listAvailableScriptVersions(value)
      const values = available.map((item) => item.version).sort((a, b) => b - a)
      setAvailableVersions(available)
      const selectedVersion =
        (preferredVersion && values.includes(preferredVersion) ? preferredVersion : undefined) ??
        values[0] ??
        fallbackVersion ??
        1
      setVersion(selectedVersion)
      if (loadParams) {
        const selected = available.find((item) => item.version === selectedVersion)
        setConfigText(configTextFromParams(selected?.script_params))
      }
      await loadOffersFor(value, selectedVersion)
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  async function loadOffersFor(
    selectedScriptId: number,
    selectedVersion: number,
    groupId = groupFilterId
  ) {
    setOffersLoading(true)
    setOffersPage(0)
    try {
      const loaded = await listScriptOffers(selectedScriptId, selectedVersion, groupId || undefined)
      setOffers(loaded)
      // Default to Auto: let the platform pick the best idle provider. Price the
      // group as a whole so the client still sees a representative quote.
      setAutoSelect(true)
      setNodeId('')
      try {
        const priced = await quoteOrder({
          script_id: selectedScriptId,
          version: selectedVersion,
          provider_group_id: groupId || undefined,
        })
        setQuote(priced.breakdown)
      } catch {
        setQuote(null)
      }
      if (loaded.length === 0) toast.info(t('No provider offers yet for this version'))
    } finally {
      setOffersLoading(false)
    }
  }

  async function loadBalance() {
    try {
      const [balances, self] = await Promise.all([getLedgerBalances(), getSelf()])
      setBal(balances)
      // getSelf returns the standard API envelope; the wallet quota lives on data.
      const quota = self?.data?.quota
      setWalletQuota(typeof quota === 'number' ? quota : null)
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  // onRecharge transfers funds from the main /wallet balance into the
  // marketplace available balance (1:1 in USD). The backend debits the wallet
  // quota and credits the available-balance ledger atomically.
  async function onRecharge() {
    const amountMicros = displayToMicros(rechargeAmt)
    if (amountMicros <= 0) {
      toast.error(t('Enter an amount greater than zero'))
      return
    }
    setRecharging(true)
    try {
      await rechargeAvailable(amountMicros)
      toast.success(t('Recharged from wallet'))
      await loadBalance()
    } catch (e) {
      toast.error(String((e as Error).message))
    } finally {
      setRecharging(false)
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
        // Restore the saved draft config as-is; don't overwrite it with the
        // version's default params on page load.
        await selectScript(initialDraft.scriptId, initialDraft.version, savedScript.latest_version, false)
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
      const q = await quoteOrder({
        script_id: scriptId,
        version,
        node_id: autoSelect ? undefined : nodeId || undefined,
        provider_group_id: autoSelect ? groupFilterId || undefined : undefined,
      })
      setQuote(q.breakdown)
      if (!autoSelect && q.chosen_node_id) setNodeId(q.chosen_node_id)
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

  // selectAuto switches back to platform auto-pick (busiest, highest-success
  // idle provider within the group filter). Prices the group as a whole.
  function selectAuto() {
    setAutoSelect(true)
    setNodeId('')
    if (scriptId) {
      void quoteOrder({
        script_id: scriptId,
        version,
        provider_group_id: groupFilterId || undefined,
      })
        .then((priced) => setQuote(priced.breakdown))
        .catch(() => setQuote(null))
    }
  }

  // selectProvider pins a specific provider offer (turns Auto off).
  function selectProvider(selectedNodeId: string) {
    setAutoSelect(false)
    setNodeId(selectedNodeId)
    void onQuoteForNode(selectedNodeId)
  }

  // onSearchGroups resolves a provider group by name so offers and the auto-pick
  // can be filtered to a single provider.
  async function onSearchGroups() {
    const query = groupQuery.trim()
    if (!query) {
      toast.error(t('Enter a group name to search'))
      return
    }
    setGroupSearching(true)
    try {
      setGroupResults(await searchProviderGroups(query))
    } catch (e) {
      toast.error(String((e as Error).message))
    } finally {
      setGroupSearching(false)
    }
  }

  // applyGroupFilter restricts offers + auto-pick to the chosen group and
  // reloads the offers. Passing '' clears the filter (all providers).
  async function applyGroupFilter(groupId: string, groupName: string) {
    setGroupFilterId(groupId)
    setGroupFilterName(groupName)
    setGroupResults([])
    setGroupQuery(groupName)
    if (scriptId) await loadOffersFor(scriptId, version, groupId)
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
        {
          script_id: scriptId,
          version,
          node_id: autoSelect ? undefined : nodeId || undefined,
          provider_group_id: autoSelect ? groupFilterId || undefined : undefined,
          input_hash: inputHash,
        },
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
        {/* Wallet balances and the available-balance recharge action. */}
        <div className='mb-4 grid grid-cols-1 gap-3 sm:grid-cols-2'>
          <div className='rounded-lg border p-3'>
            <div className='flex flex-wrap items-end justify-between gap-3'>
              <div>
                <div className='text-muted-foreground text-xs'>{t('Available')}</div>
                <div className='mt-1 text-lg font-semibold'>
                  {microsToCurrency(bal?.client_available)}
                </div>
              </div>
              <div className='flex items-center gap-2'>
                <Input
                  className='h-9 w-28'
                  value={rechargeAmt}
                  onChange={(e) => setRechargeAmt(e.target.value)}
                  aria-label={t('Recharge amount')}
                />
                <Button className='h-9' onClick={onRecharge} disabled={recharging}>
                  {recharging ? t('Recharging...') : t('Recharge')}
                </Button>
              </div>
            </div>
            <div className='text-muted-foreground mt-2 text-xs'>
              {t('Recharge available balance')} · {t('Wallet balance')}:{' '}
              {walletQuota != null ? formatQuotaWithCurrency(walletQuota) : '--'}
            </div>
          </div>
          <div className='rounded-lg border p-3'>
            <div className='text-muted-foreground text-xs'>{t('Reserved')}</div>
            <div className='mt-1 text-lg font-semibold'>
              {microsToCurrency(bal?.client_reserved)}
            </div>
          </div>
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
                setAutoSelect(true)
                setOffersPage(0)
                setQuote(null)
                const selected = availableVersions.find((item) => item.version === selectedVersion)
                setConfigText(configTextFromParams(selected?.script_params))
                if (scriptId) void loadOffersFor(scriptId, selectedVersion)
              }}
            >
              {versions.map((item) => <option key={item} value={item}>v{item}</option>)}
            </select>
            <Button variant='outline' onClick={loadOffers} disabled={offersLoading}>
              {offersLoading ? t('Loading...') : t('View offers')}
            </Button>
          </div>

          {/* Provider-group filter: search a group by name to restrict offers +
              the auto-pick to a single provider. Helps when there are many. */}
          <div className='mt-3 rounded-md border p-3'>
            <div className='text-muted-foreground mb-1 text-xs'>
              {t('Filter by provider group (optional)')}
            </div>
            <div className='flex flex-wrap items-center gap-2'>
              <Input
                className='h-9 w-56'
                placeholder={t('Search group name')}
                value={groupQuery}
                onChange={(e) => setGroupQuery(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault()
                    void onSearchGroups()
                  }
                }}
              />
              <Button variant='outline' className='h-9' onClick={onSearchGroups} disabled={groupSearching}>
                {groupSearching ? t('Searching...') : t('Search')}
              </Button>
              {groupFilterId && (
                <span className='flex items-center gap-1 text-xs'>
                  <span className='text-muted-foreground'>{t('Filtered')}:</span>
                  <span className='font-medium'>{groupFilterName}</span>
                  <Button
                    size='sm'
                    variant='ghost'
                    className='h-6 px-2'
                    onClick={() => void applyGroupFilter('', '')}
                  >
                    {t('Clear')}
                  </Button>
                </span>
              )}
            </div>
            {groupResults.length > 0 && (
              <div className='mt-2 flex flex-col gap-1'>
                {groupResults.map((g) => (
                  <button
                    key={g.id}
                    type='button'
                    className='flex items-center gap-2 rounded px-2 py-1 text-left text-sm hover:bg-muted/50'
                    onClick={() => void applyGroupFilter(g.id, g.name)}
                  >
                    <span className='font-medium'>{g.name}</span>
                    <span className='font-mono text-xs text-muted-foreground'>{g.id}</span>
                  </button>
                ))}
              </div>
            )}
          </div>

          <div className='mt-3'>
            <div className='text-muted-foreground mb-2 text-xs'>
              {t('Provider offers (Auto picks the best idle provider, or choose one)')}
            </div>
            {/* Auto is the default: platform picks the busiest, highest-success
                idle provider (within the group filter, if set). */}
            <label className='flex items-center gap-2 rounded-md border p-2 text-sm'>
              <input type='radio' name='offer' checked={autoSelect} onChange={selectAuto} />
              <span className='font-medium'>{t('Auto (recommended)')}</span>
              <span className='text-muted-foreground text-xs'>
                {groupFilterId
                  ? t('Auto-picks the best idle provider in this group')
                  : t('Auto-picks the best idle provider')}
              </span>
            </label>

            {offers.length > 0 && (
              <div className='mt-2 flex flex-col gap-1'>
                {offers
                  .slice(offersPage * OFFERS_PAGE_SIZE, offersPage * OFFERS_PAGE_SIZE + OFFERS_PAGE_SIZE)
                  .map((o) => {
                    const rate =
                      o.executions > 0
                        ? `${Math.round((o.successes / o.executions) * 100)}% (${o.successes}/${o.executions})`
                        : '-'
                    let statusLabel = t('Offline')
                    if (o.busy) statusLabel = t('Busy')
                    else if (o.online) statusLabel = t('Online')
                    return (
                      <label
                        key={o.node_id}
                        className='flex flex-wrap items-center gap-2 rounded-md border p-2 text-sm'
                      >
                        <input
                          type='radio'
                          name='offer'
                          checked={!autoSelect && nodeId === o.node_id}
                          disabled={!o.available}
                          onChange={() => selectProvider(o.node_id)}
                        />
                        <span className='font-mono text-xs'>{o.node_id}</span>
                        {o.provider_group_name && (
                          <span className='rounded bg-muted px-1.5 py-0.5 text-xs'>
                            {o.provider_group_name}
                          </span>
                        )}
                        {o.provider_group_id && (
                          <span className='font-mono text-xs text-muted-foreground'>
                            {o.provider_group_id}
                          </span>
                        )}
                        <span className='font-semibold'>{microsToCurrency(o.price_micros)}</span>
                        <span>{statusLabel}</span>
                        <span className='text-muted-foreground text-xs'>
                          {t('Success rate')}: {rate}
                        </span>
                        <span className='text-muted-foreground text-xs'>
                          {t('quota')}: {o.remaining_quota}
                        </span>
                        {!o.available && (
                          <span className='text-xs text-red-600'>
                            {o.unavailable_reason === 'QUOTA_EXHAUSTED' && t('Quota exhausted')}
                            {o.unavailable_reason === 'NODE_OFFLINE' && t('Node offline')}
                            {o.unavailable_reason === 'NODE_BUSY' && t('Provider is busy')}
                            {o.unavailable_reason === 'CAPABILITY_TEST_EXPIRED' && t('Capability test expired')}
                            {o.unavailable_reason === 'BALANCE_CHECK_EXPIRED' && t('Balance check expired')}
                          </span>
                        )}
                      </label>
                    )
                  })}
              </div>
            )}

            {/* Pagination: only when the offer list spills past one page. */}
            {offers.length > OFFERS_PAGE_SIZE && (
              <div className='mt-2 flex items-center justify-between text-xs'>
                <span className='text-muted-foreground'>
                  {t('{{count}} providers', { count: offers.length })}
                </span>
                <div className='flex items-center gap-2'>
                  <Button
                    size='sm'
                    variant='outline'
                    disabled={offersPage === 0}
                    onClick={() => setOffersPage((p) => Math.max(0, p - 1))}
                  >
                    {t('Previous')}
                  </Button>
                  <span>
                    {offersPage + 1} / {Math.ceil(offers.length / OFFERS_PAGE_SIZE)}
                  </span>
                  <Button
                    size='sm'
                    variant='outline'
                    disabled={(offersPage + 1) * OFFERS_PAGE_SIZE >= offers.length}
                    onClick={() => setOffersPage((p) => p + 1)}
                  >
                    {t('Next')}
                  </Button>
                </div>
              </div>
            )}
          </div>
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
              <div>{t('Provider')}: {microsToCurrency(quote.ProviderMicros)}</div>
              <div>{t('Author')}: {microsToCurrency(quote.AuthorMicros)}</div>
              <div>{t('Platform fee')}: {microsToCurrency(quote.PlatformFeeMicros)}</div>
              <div>{t('Risk reserve')}: {microsToCurrency(quote.RiskReserveMicros)}</div>
              <div className='font-semibold'>
                {t('Total')}: {microsToCurrency(quote.MaxCustomerMicros)}
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
              {microsToCurrency(order.max_amount_micros)}
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
