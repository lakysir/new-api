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

export type ScriptVersion = {
  id: number
  script_id: number
  version: number
  author_id: number
  author_username?: string
  title: string
  category_id: number
  code_sha256: string
  signature_key_id?: string
  signature?: string
  review_status: string
  published_at: number
  revoked_at: number
  revoked_reason?: string
  revoke_severity?: string
  pricing_template_id?: number
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
  region: string
  version: string
  last_seen_at: number
}

export type NodeCapability = {
  id: number
  node_id: string
  script_id: number
  version: number
  price_micros: number
  daily_quota: number
  remaining_quota: number
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
  created_at: number
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
