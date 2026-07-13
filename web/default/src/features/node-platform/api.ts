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
import { api } from '@/lib/api'

import type {
  ApiEnvelope,
  Device,
  FeeQuote,
  LedgerBalances,
  NodeCapability,
  NodeInfo,
  Order,
  PriceBreakdown,
  ScriptVersion,
} from './types'

// unwrap returns the `data` field of the standard API envelope, throwing the
// server message on business failure so callers can toast it.
async function unwrap<T>(p: Promise<{ data: ApiEnvelope<T> }>): Promise<T> {
  const res = await p
  if (!res.data?.success) {
    throw new Error(res.data?.message || 'request failed')
  }
  return res.data.data as T
}

// --- Script versions (author + admin) --------------------------------------

export function submitScriptForReview(scriptId: number) {
  return unwrap(api.post(`/api/scripts/mine/${scriptId}/submit-review`))
}

export function publishScriptVersion(scriptId: number, pricingTemplateId?: number) {
  return unwrap(
    api.post(`/api/scripts/mine/${scriptId}/publish-version`, {
      pricing_template_id: pricingTemplateId ?? 0,
    })
  )
}

export function listScriptVersions(scriptId: number) {
  return unwrap<ScriptVersion[]>(api.get(`/api/scripts/mine/${scriptId}/versions`))
}

// Admin: approve/reject a pending draft; revoke a published version.
export function listPendingScripts() {
  return unwrap<
    Array<{
      id: number
      user_id: number
      author_username: string
      title: string
      description: string
      script_params?: string
      draft_code: string
      previous_title?: string
      previous_description?: string
      previous_script_params?: string
      previous_code?: string
      review_status: string
      latest_version?: number
    }>
  >(api.get('/api/scripts/pending'))
}

export function reviewScript(scriptId: number, approve: boolean, note: string) {
  return unwrap(api.post(`/api/scripts/${scriptId}/review`, { approve, note }))
}

export function revokeScriptVersion(
  scriptId: number,
  version: number,
  reason: string,
  severity: string
) {
  return unwrap(
    api.post(`/api/scripts/${scriptId}/versions/${version}/revoke`, { reason, severity })
  )
}

// --- Devices & nodes --------------------------------------------------------

export function listMyDevices() {
  return unwrap<Device[]>(api.get('/api/devices/mine'))
}

export function listMyNodes() {
  return unwrap<NodeInfo[]>(api.get('/api/nodes/mine'))
}

export function revokeDevice(deviceId: string) {
  return unwrap(api.delete(`/api/devices/${deviceId}`))
}

export function listNodeCapabilities(nodeId: string) {
  return unwrap<NodeCapability[]>(api.get(`/api/nodes/${nodeId}/capabilities`))
}

export function disableCapability(nodeId: string, scriptId: number, version: number) {
  return unwrap(
    api.delete(`/api/nodes/${nodeId}/capabilities/${scriptId}?version=${version}`)
  )
}

// --- Orders -----------------------------------------------------------------

export function quoteOrder(body: {
  script_id: number
  version: number
  bid_micros: number
  relay_gb?: number
  storage_gb_hours?: number
}) {
  return unwrap<PriceBreakdown>(api.post('/api/orders/quote', body))
}

export function getOrder(id: string) {
  return unwrap<Order>(api.get(`/api/orders/${id}`))
}

export function cancelOrder(id: string) {
  return unwrap<Order>(api.post(`/api/orders/${id}/cancel`))
}

// --- Ledger & payment -------------------------------------------------------

export function getLedgerBalances() {
  return unwrap<LedgerBalances>(api.get('/api/ledger/balances'))
}

export function simulateDeposit(amountMicros: number, reference: string) {
  return unwrap(
    api.post('/api/ledger/deposit/simulate', {
      amount_micros: amountMicros,
      reference,
    })
  )
}

export function estimateWithdrawalFee(amountMicros: number) {
  return unwrap<FeeQuote>(
    api.post('/api/payment/withdrawals/estimate', { amount_micros: amountMicros })
  )
}

export function requestWithdrawal(body: {
  owner_type: 'provider' | 'author'
  to_address: string
  amount_micros: number
}) {
  return unwrap(api.post('/api/payment/withdrawals', body))
}
