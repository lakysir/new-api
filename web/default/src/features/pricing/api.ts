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

import type { PricingData } from './types'

// ----------------------------------------------------------------------------
// Pricing APIs
// ----------------------------------------------------------------------------

// Get model pricing data
export async function getPricing(): Promise<PricingData> {
  const res = await api.get('/api/pricing')
  return res.data
}

// ----------------------------------------------------------------------------
// Marketplace bridge model docs
// ----------------------------------------------------------------------------

// A bridged marketplace model's caller-facing API documentation: the script's
// declared parameter/result JSON Schemas, the operator's default template, and
// consume-multiplier semantics. Absent for non-bridge models.
export type MarketplaceModelDoc = {
  model_name: string
  script_id: number
  version: number
  title: string
  description: string
  task_type: string
  script_params: string
  result_schema: string
  param_template: string
  consume_multiplier: number
  timeout_seconds: number
}

// getMarketplaceModelDoc fetches per-model bridge docs. Resolves to null when
// the model is not a marketplace bridge model (or on any error), so callers can
// simply skip rendering the bridge doc section.
export async function getMarketplaceModelDoc(
  modelName: string
): Promise<MarketplaceModelDoc | null> {
  try {
    const res = await api.get(
      `/api/scripts/model-doc/${encodeURIComponent(modelName)}`
    )
    if (res.data?.success && res.data?.data) {
      return res.data.data as MarketplaceModelDoc
    }
    return null
  } catch {
    return null
  }
}
