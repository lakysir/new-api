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
  CapabilityStat,
  Device,
  EarningsSummary,
  LedgerBalances,
  NodeCapability,
  NodeInfo,
  Order,
  PriceBreakdown,
  ProviderTaskAttempt,
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

// Author proposes their share (ppm of provider price) and target-site category
// when submitting for review.
export function submitScriptForReview(
  scriptId: number,
  opts?: { author_share_rate_ppm?: number; category_id?: number }
) {
  return unwrap(
    api.post(`/api/scripts/mine/${scriptId}/submit-review`, opts ?? {})
  )
}

export type ScriptCategory = {
  id: number
  name: string
  site: string
  balance_script_id: number
  balance_script_version: number
}

export function listCategories() {
  return unwrap<ScriptCategory[]>(api.get('/api/scripts/categories'))
}

export function createCategory(name: string, site: string) {
  return unwrap<ScriptCategory>(
    api.post('/api/scripts/categories', { name, site })
  )
}

export function setCategoryBalanceScript(
  categoryId: number,
  scriptId: number,
  version: number
) {
  return unwrap(
    api.post(`/api/scripts/categories/${categoryId}/balance-script`, {
      script_id: scriptId,
      version,
    })
  )
}

// Provider reports a node's balance-probe result for a category (plugin runs the
// probe script; this records the passing window that lets it list capabilities).
export function reportBalanceCheck(
  nodeId: string,
  body: {
    category_id: number
    balance_ok: boolean
    balance_micros?: number
    tier?: string
  }
) {
  return unwrap(api.post(`/api/nodes/${nodeId}/balance-check`, body))
}

export type NodeBalanceCheck = {
  category_id: number
  balance_ok: boolean
  balance_micros: number
  tier: string
  error_message?: string
  checked_at: number
  expires_at: number
}

export function requestBalanceCheck(nodeId: string, categoryId: number) {
  return unwrap<{ event_id: string; dispatched: boolean }>(
    api.post(`/api/nodes/${nodeId}/balance-check/request`, {
      category_id: categoryId,
    })
  )
}

export function listBalanceChecks(nodeId: string) {
  return unwrap<NodeBalanceCheck[]>(
    api.get(`/api/nodes/${nodeId}/balance-checks`)
  )
}

export function publishScriptVersion(
  scriptId: number,
  pricingTemplateId?: number
) {
  return unwrap(
    api.post(`/api/scripts/mine/${scriptId}/publish-version`, {
      pricing_template_id: pricingTemplateId ?? 0,
    })
  )
}

export function listScriptVersions(scriptId: number) {
  return unwrap<ScriptVersion[]>(
    api.get(`/api/scripts/mine/${scriptId}/versions`)
  )
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

export function listPublishedScriptVersions() {
  return unwrap<ScriptVersion[]>(api.get('/api/scripts/versions/published'))
}

export function listAvailableScriptVersions(scriptId: number) {
  return unwrap<ScriptVersion[]>(
    api.get(`/api/scripts/${scriptId}/versions/available`)
  )
}

// Operator approves/rejects; on approve sets the platform service fee (ppm).
export function reviewScript(
  scriptId: number,
  approve: boolean,
  note: string,
  platformFeeRatePpm?: number
) {
  return unwrap(
    api.post(`/api/scripts/${scriptId}/review`, {
      approve,
      note,
      platform_fee_rate_ppm: platformFeeRatePpm ?? 0,
    })
  )
}

export function revokeScriptVersion(
  scriptId: number,
  version: number,
  reason: string,
  severity: string
) {
  return unwrap(
    api.post(`/api/scripts/${scriptId}/versions/${version}/revoke`, {
      reason,
      severity,
    })
  )
}

export function deleteScriptVersion(scriptId: number, version: number) {
  return unwrap(api.delete(`/api/scripts/${scriptId}/versions/${version}`))
}

export function updateScriptVersionPricing(
  scriptId: number,
  version: number,
  body: { author_share_rate_ppm: number; platform_fee_rate_ppm: number }
) {
  return unwrap<ScriptVersion>(
    api.put(`/api/scripts/${scriptId}/versions/${version}/pricing`, body)
  )
}

// --- Platform script signing key --------------------------------------------

export type PlatformSigningKey = {
  key_id: string
  public_key: string
  signing_enabled: boolean
}

// Public: current platform signing key status (no secret). Plugins use this to
// verify manifest signatures; the console uses it to show whether signing is on.
export function getPlatformSigningKey() {
  return unwrap<PlatformSigningKey>(api.get('/api/scripts/platform-key'))
}

// Admin: generate (or rotate) the platform Ed25519 signing key. Rotating
// invalidates existing signatures, so published versions must be re-published.
export function generatePlatformSigningKey(keyId?: string) {
  return unwrap<PlatformSigningKey>(
    api.post('/api/scripts/signing-key/generate', { key_id: keyId ?? '' })
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

export function deleteDevice(deviceId: string) {
  return unwrap(api.delete(`/api/devices/${deviceId}/purge`))
}

export function deleteNode(nodeId: string) {
  return unwrap(api.delete(`/api/nodes/${nodeId}`))
}

// setNodeEnabled flips a node's scheduling switch. Enabling requires every
// active capability's category balance check to have passed (server-enforced);
// the rejection message names the categories that still need checking.
export function setNodeEnabled(nodeId: string, enabled: boolean) {
  return unwrap<{ node_id: string; enabled: boolean }>(
    api.post(`/api/nodes/${nodeId}/enabled`, { enabled })
  )
}

export function listNodeCapabilities(nodeId: string) {
  return unwrap<NodeCapability[]>(api.get(`/api/nodes/${nodeId}/capabilities`))
}

export function removeCapability(
  nodeId: string,
  scriptId: number,
  version: number
) {
  return unwrap(
    api.delete(
      `/api/nodes/${nodeId}/capabilities/${scriptId}?version=${version}`
    )
  )
}

// createCapabilityTest validates the script version is executable and returns a
// test window (test_expires_at) required to enable the capability.
export function createCapabilityTest(
  nodeId: string,
  scriptId: number,
  version: number
) {
  return unwrap<{ test_expires_at: number }>(
    api.post(
      `/api/nodes/${nodeId}/capabilities/${scriptId}/test?version=${version}`
    )
  )
}

// enableCapability lists a script version on a node with the provider's price
// and daily execution limit. The initial balance defaults to 10 on first
// listing and is updated from actual execution results — providers do not set it.
export function enableCapability(
  nodeId: string,
  scriptId: number,
  body: {
    version: number
    price_micros: number
    daily_limit: number
    test_expires_at: number
  }
) {
  return unwrap<NodeCapability>(
    api.put(`/api/nodes/${nodeId}/capabilities/${scriptId}`, body)
  )
}

// --- Orders -----------------------------------------------------------------

export type ScriptOffer = {
  node_id: string
  provider_group_id?: string
  provider_group_name?: string
  price_micros: number
  online: boolean
  /** Online but currently running a task, so it can't take a new one. */
  busy: boolean
  remaining_quota: number
  state: string
  /** Lifetime task attempts (success + failure) on this node. */
  executions: number
  /** Lifetime successful task attempts on this node. */
  successes: number
  available: boolean
  unavailable_reason?: string
  /** The provider's scheduling switch for this node. */
  enabled: boolean
  /** True when this node belongs to the requesting user. A disabled node is only
   *  ever returned to its owner, so they can test it end-to-end. */
  owned: boolean
}

// A provider group is the logical group a provider's nodes belong to (one per
// user, named after the username).
export type ProviderGroup = {
  id: string
  user_id: number
  name: string
  created_at: number
  updated_at: number
}

export function listScriptOffers(
  scriptId: number,
  version: number,
  providerGroupId?: string,
  consumeMultiplier?: number
) {
  const params = new URLSearchParams({ version: String(version) })
  if (providerGroupId) params.set('provider_group_id', providerGroupId)
  // A node's remaining balance must exceed the coefficient to be offered.
  if (consumeMultiplier && consumeMultiplier > 1) {
    params.set('consume_multiplier', String(consumeMultiplier))
  }
  return unwrap<ScriptOffer[]>(
    api.get(`/api/scripts/${scriptId}/offers?${params.toString()}`)
  )
}

// getMyProviderGroup returns (creating on first use) the caller's provider group
// and backfills their existing nodes into it.
export function getMyProviderGroup() {
  return unwrap<ProviderGroup>(api.get('/api/nodes/provider-group/mine'))
}

// searchProviderGroups finds provider groups by name so a client can filter
// offers to a single provider. Empty query returns nothing.
export function searchProviderGroups(query: string) {
  return unwrap<ProviderGroup[]>(
    api.get('/api/nodes/provider-groups/search', { params: { q: query } })
  )
}

export function quoteOrder(body: {
  script_id: number
  version: number
  node_id?: string
  provider_group_id?: string
  relay_gb?: number
  storage_gb_hours?: number
  consume_multiplier?: number
}) {
  return unwrap<{ breakdown: PriceBreakdown; chosen_node_id: string }>(
    api.post('/api/orders/quote', body)
  )
}

export function createOrder(
  body: {
    script_id: number
    version: number
    node_id?: string
    provider_group_id?: string
    input_hash: string
    relay_gb?: number
    storage_gb_hours?: number
    consume_multiplier?: number
  },
  idempotencyKey: string
) {
  return unwrap<{ order: Order; created: boolean }>(
    api.post('/api/orders', body, {
      headers: { 'Idempotency-Key': idempotencyKey },
    })
  )
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

// Earnings for a payable role (provider = money earned running nodes, author =
// money earned from published scripts): balance + day/week/month/lifetime.
export function getEarnings(role: 'provider' | 'author') {
  return unwrap<EarningsSummary>(
    api.get('/api/ledger/earnings', { params: { role } })
  )
}

// Platform service-fee revenue summary (admin only).
export function getPlatformEarnings() {
  return unwrap<EarningsSummary>(api.get('/api/scripts/platform-earnings'))
}

// Per-(node, script version) execution stats across the caller's nodes.
export function listProviderCapabilityStats() {
  return unwrap<CapabilityStat[]>(api.get('/api/nodes/capability-stats'))
}

// Recent per-node execution records (task attempts) across the caller's nodes,
// newest first. Providers use these to see which tasks succeeded/failed and why.
export function listMyTaskAttempts(limit = 100, offset = 0) {
  return unwrap<ProviderTaskAttempt[]>(
    api.get('/api/nodes/task-attempts', { params: { limit, offset } })
  )
}

// rechargeAvailable funds the caller's marketplace available balance by
// deducting the equivalent amount (1:1 in USD) from their main /wallet quota.
// This is a real transfer, not a simulation.
export function rechargeAvailable(amountMicros: number) {
  return unwrap<{
    transaction_id: number
    type: string
    quota_deducted: number
  }>(api.post('/api/ledger/recharge', { amount_micros: amountMicros }))
}

// withdrawEarnings transfers the caller's payable balance for a role (provider =
// node earnings, author = script earnings) into their main /wallet quota (1:1 in
// USD). The inverse of rechargeAvailable — a real transfer, not a simulation.
export function withdrawEarnings(
  role: 'provider' | 'author',
  amountMicros: number
) {
  return unwrap<{
    transaction_id: number
    type: string
    quota_credited: number
  }>(
    api.post(
      '/api/ledger/withdraw',
      { amount_micros: amountMicros },
      { params: { role } }
    )
  )
}

// withdrawPlatformEarnings transfers the platform's revenue balance into the
// calling admin's main /wallet quota (admin only), 1:1 in USD.
export function withdrawPlatformEarnings(amountMicros: number) {
  return unwrap<{
    transaction_id: number
    type: string
    quota_credited: number
  }>(
    api.post('/api/scripts/platform-earnings/withdraw', {
      amount_micros: amountMicros,
    })
  )
}
