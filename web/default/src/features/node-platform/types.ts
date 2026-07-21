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

// Shared types for the P2P node-platform frontend (script versions, devices,
// nodes, orders, ledger, payments). Field names mirror the Go JSON tags in
// new-api/model so responses map 1:1.

export type ApiEnvelope<T> = {
  success: boolean
  message?: string
  data?: T
}

// --- Script versions (Stage B) ---------------------------------------------

export type ScriptReviewStatus = 'draft' | 'pending' | 'approved' | 'rejected'

// PricingRule defines how one script parameter affects the execution price.
// The final price multiplier is the product of all matched rule multipliers.
//
// enum_multiplier: each discrete value maps to a fixed multiplier.
//   e.g. model="vision" -> 5×, model="mini_lite" -> 1×
// linear_range: a numeric value multiplied by unit_multiplier.
//   e.g. duration=10s with unit_multiplier=1 -> 10× base units
export type PricingRule = {
  param: string           // parameter key in the script's config object
  type: 'enum_multiplier' | 'linear_range'
  label?: string          // human-readable label for the UI
  // enum_multiplier fields
  values?: Record<string, number>
  // linear_range fields
  unit_multiplier?: number   // price units per 1 increment of the param value
  min?: number
  max?: number
}

// pricing_rules is stored as a JSON string in the DB (model PricingRules is a
// Go `string`), so API responses deliver it as a string, not an array. Parse it
// at the boundary so downstream code can rely on the PricingRule[] type.
export function parsePricingRules(raw: unknown): PricingRule[] {
  if (!raw) return []
  if (Array.isArray(raw)) return raw as PricingRule[]
  if (typeof raw === 'string') {
    try {
      const parsed = JSON.parse(raw)
      return Array.isArray(parsed) ? (parsed as PricingRule[]) : []
    } catch {
      return []
    }
  }
  return []
}

export type ScriptVersion = {
  id: number
  script_id: number
  version: number
  author_id: number
  author_username?: string
  title: string
  category_id: number
  // Author-configured default task params (JSON object) for this version. The
  // purchase page loads these into the config editor when the script/version
  // is selected so the client sends real params, not a generic placeholder.
  script_params?: string
  code_sha256: string
  signature_key_id?: string
  signature?: string
  review_status: string
  published_at: number
  revoked_at: number
  revoked_reason?: string
  revoke_severity?: string
  pricing_template_id?: number
  author_share_rate_ppm?: number
  platform_fee_rate_ppm?: number
  concurrency?: number
  /** Minimum seconds between consecutive task submissions (API rate limit). */
  min_interval_seconds?: number
  /** Base price per execution unit in micro-USD, set by the script author. */
  base_price_micros?: number
  /** Structured pricing rules mapping param values to price multipliers. */
  pricing_rules?: PricingRule[]
}

// --- Devices & nodes (Stage C) ---------------------------------------------

export type Device = {
  id: string
  user_id: number
  public_key: string
  name: string
  status: string
  last_seen_at: number
  revoked_at: number
  created_at: number
}

export type NodeInfo = {
  id: string
  device_id: string
  user_id: number
  state: string
  /** Provider's scheduling switch: only enabled nodes are dispatched/offered. */
  enabled: boolean
  region: string
  version: string
  last_seen_at: number
}

export type NodeCapability = {
  id: number
  node_id: string
  script_id: number
  version: number
  /** Denormalized target-site category; 0 when the script has no category. */
  category_id: number
  /** Provider's price multiplier applied on top of the script's base price.
   *  Range 0.5–10, default 1.0. Replaces the old flat price_micros field. */
  price_multiplier: number
  /** @deprecated Use price_multiplier. Kept for backend transition compatibility. */
  price_micros?: number
  /** Max simultaneous executions for this script on this node (from script version). */
  concurrency: number
  /** Max executions per day; 0 = unlimited. Resets at midnight CST (UTC+8). */
  daily_limit: number
  /** Executions today since the last Beijing-midnight reset. */
  daily_used: number
  /** Unix timestamp (s) of the last Beijing-midnight reset. */
  daily_reset_at: number
  /** Balance on the target-site account as reported by the last successful execution. */
  remaining_quota: number
  /** Minimum seconds between consecutive tasks, read from the script version. */
  min_interval_seconds: number
  work_window: string
  status: string
  test_expires_at: number
}

// --- Orders (Stage D) ------------------------------------------------------

export type Order = {
  id: string
  client_id: number
  script_id: number
  version: number
  state: string
  input_hash: string
  max_amount_micros: number
  final_amount_micros: number
  chosen_node_id: string
  created_at: number
  // Present only on read when the order is in a failure state: the latest
  // attempt's error_code (e.g. ORIGIN_NOT_ALLOWED, SCRIPT_EXECUTION_FAILED).
  last_error?: string
}

export type PriceBreakdown = {
  Currency: string
  ProviderMicros: number
  AuthorMicros: number
  PlatformFeeMicros: number
  RelayFeeMicros: number
  StorageFeeMicros: number
  RiskReserveMicros: number
  MaxCustomerMicros: number
  RuleVersion: string
}

// --- Ledger & payment (Stage F/G) ------------------------------------------

export type LedgerBalances = {
  currency: string
  client_available: number
  client_reserved: number
  provider_payable: number
  author_payable: number
}

export type FeeQuote = {
  network: string
  fee_micros: number
  estimated_confirmation_seconds: number
}

// EarningsSummary is a role's current balance plus gross credits over calendar
// windows (today / this week / this month) and lifetime. All amounts are micros.
export type EarningsSummary = {
  currency: string
  balance_micros: number
  day_micros: number
  week_micros: number
  month_micros: number
  total_micros: number
}

// CapabilityStat is one node's per-script-version execution summary: how many
// attempts ran, how many settled, and the gross provider revenue earned.
export type CapabilityStat = {
  node_id: string
  script_id: number
  version: number
  executions: number
  successes: number
  revenue_micros: number
}

// ProviderTaskAttempt is one execution record on a provider's node, joined to
// its order. Params/results are E2EE and never reach the control plane, so this
// carries only what the server can know: attempt state, failure code, the
// target-site balance the plugin reported, and timing. Providers use it to
// debug their nodes' behavior.
export type ProviderTaskAttempt = {
  task_id: string
  order_id: string
  node_id: string
  attempt: number
  state: string
  error_code?: string
  script_balance?: number
  script_id: number
  version: number
  input_hash?: string
  created_at: number
  updated_at: number
}
