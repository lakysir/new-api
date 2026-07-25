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
  ModelConcurrencyApiResponse,
  ModelConcurrencyRule,
  UpsertModelConcurrencyRequest,
} from './types'

export async function getModelConcurrencyRules(modelName?: string) {
  const res = await api.get<ModelConcurrencyApiResponse<ModelConcurrencyRule[]>>(
    '/api/model_concurrency/',
    { params: modelName ? { model: modelName } : undefined }
  )
  return res.data
}

export async function upsertModelConcurrencyRule(
  request: UpsertModelConcurrencyRequest
) {
  const res = await api.put<ModelConcurrencyApiResponse<ModelConcurrencyRule>>(
    '/api/model_concurrency/',
    request
  )
  return res.data
}

export async function deleteModelConcurrencyRule(id: number) {
  const res = await api.delete<ModelConcurrencyApiResponse<null>>(
    `/api/model_concurrency/${id}`
  )
  return res.data
}
