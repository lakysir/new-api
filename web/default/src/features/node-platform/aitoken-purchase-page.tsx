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
import { api } from '@/lib/api'

import {
  createOrder,
  getLedgerBalances,
  getOrder,
  listScriptOffers,
  quoteOrder,
  type ScriptOffer,
} from './api'
import { ClientRelaySession } from './lib/client-relay-session'
import { microsToDisplay } from './lib/format'
import type { LedgerBalances, Order, PriceBreakdown } from './types'

type PublishedScript = { id: number; title: string; description?: string }

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
  const [bal, setBal] = useState<LedgerBalances | null>(null)
  const [scripts, setScripts] = useState<PublishedScript[]>([])
  const [scriptId, setScriptId] = useState(0)
  const [version, setVersion] = useState(1)
  const [offers, setOffers] = useState<ScriptOffer[]>([])
  const [nodeId, setNodeId] = useState('') // chosen offer; empty = cheapest
  const [configText, setConfigText] = useState('{\n  "prompt": "a dog"\n}')
  const [quote, setQuote] = useState<PriceBreakdown | null>(null)
  const [order, setOrder] = useState<Order | null>(null)
  const [busy, setBusy] = useState(false)
  // Client relay needs an API key to authenticate to the data-plane relay and a
  // stable client device id for the HKDF context.
  const [apiKey, setApiKey] = useState('')
  const [relayResult, setRelayResult] = useState<string>('')
  const [relayStatus, setRelayStatus] = useState<string>('')

  async function loadBalance() {
    try {
      setBal(await getLedgerBalances())
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  async function loadScripts() {
    try {
      const res = await api.get('/api/scripts/square', { params: { limit: 100 } })
      const list = (res.data?.data?.items ?? res.data?.items ?? res.data?.data ?? []) as PublishedScript[]
      setScripts(list)
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  useEffect(() => {
    loadBalance()
    loadScripts()
  }, [])

  async function loadOffers() {
    if (!scriptId) {
      toast.error(t('Select a script first'))
      return
    }
    try {
      const o = await listScriptOffers(scriptId, version)
      setOffers(o)
      setNodeId('')
      if (o.length === 0) toast.info(t('No provider offers yet for this version'))
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

  async function onPurchase() {
    if (!scriptId) {
      toast.error(t('Select a script first'))
      return
    }
    // Validate config is JSON before ordering.
    let inputHash = ''
    try {
      JSON.parse(configText)
      inputHash = await sha256Hex(configText)
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
      toast.success(t('Order created and funds reserved'))
      await loadBalance()
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

  // Send the config to the executing provider over the E2EE relay and wait for
  // the encrypted result. task_id == order id in the MVP; attempt 1.
  async function runViaRelay() {
    if (!order) {
      toast.error(t('Create an order first'))
      return
    }
    if (!apiKey) {
      toast.error(t('API key is required to connect the relay'))
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
      deviceToken: apiKey,
      taskId: order.id,
      attempt: 1,
      clientDeviceId: `client-${order.client_id}`,
    })
    setRelayStatus(t('Connecting to relay...'))
    setRelayResult('')
    try {
      await session.connect()
      setRelayStatus(t('Waiting for provider handshake...'))
      await session.waitEstablished()
      setRelayStatus(t('Sending config, waiting for result...'))
      await session.sendConfig(config)
      const result = await session.waitForResult()
      setRelayResult(JSON.stringify(result, null, 2))
      setRelayStatus(t('Result received'))
      await refreshOrder()
    } catch (e) {
      setRelayStatus('')
      toast.error(String((e as Error).message))
    } finally {
      session.close()
    }
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Buy AI Token')}</SectionPageLayout.Title>
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
              onChange={(e) => setScriptId(Number(e.target.value))}
            >
              <option value={0}>{t('Select a script')}</option>
              {scripts.map((s) => (
                <option key={s.id} value={s.id}>
                  #{s.id} {s.title}
                </option>
              ))}
            </select>
            <Input
              className='w-24'
              type='number'
              value={version}
              onChange={(e) => setVersion(Number(e.target.value))}
              placeholder={t('Version')}
            />
            <Button variant='outline' onClick={loadOffers}>
              {t('View offers')}
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
                      onChange={() => setNodeId(o.node_id)}
                    />
                    <span className='font-mono text-xs'>{o.node_id}</span>
                    <span className='font-semibold'>{microsToDisplay(o.price_micros)}</span>
                    <span>{o.online ? '🟢' : '⚪'}</span>
                    <span className='text-muted-foreground text-xs'>
                      {t('quota')}: {o.remaining_quota}
                    </span>
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
          <Button onClick={onPurchase} disabled={busy}>
            {t('Purchase (reserve funds)')}
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

            {/* Execute via the E2EE relay: send config to the provider, get result. */}
            <div className='mt-3 border-t pt-3'>
              <div className='mb-2 text-sm font-medium'>{t('Run task (send config over encrypted relay)')}</div>
              <div className='flex flex-wrap items-center gap-2'>
                <Input
                  className='w-72'
                  type='password'
                  placeholder={t('API key (for relay auth)')}
                  value={apiKey}
                  onChange={(e) => setApiKey(e.target.value)}
                />
                <Button onClick={runViaRelay}>{t('Send & run')}</Button>
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
