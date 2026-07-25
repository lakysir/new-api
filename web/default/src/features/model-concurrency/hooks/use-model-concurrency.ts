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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { toast } from 'sonner'

import {
  deleteModelConcurrencyRule,
  getModelConcurrencyCandidateModels,
  getModelConcurrencyRules,
  upsertModelConcurrencyRule,
} from '../api'
import type { UpsertModelConcurrencyRequest } from '../types'

const RULES_KEY = 'model-concurrency-rules'
const MODELS_KEY = 'model-concurrency-models'

export function useModelConcurrencyRules(modelName: string) {
  return useQuery({
    queryKey: [RULES_KEY, modelName],
    queryFn: async () => {
      const res = await getModelConcurrencyRules(modelName)
      return res.data ?? []
    },
    enabled: modelName !== '',
  })
}

export function useModelConcurrencyCandidateModels() {
  return useQuery({
    queryKey: [MODELS_KEY],
    queryFn: async () => {
      const res = await getModelConcurrencyCandidateModels()
      return res.data ?? []
    },
  })
}

function useInvalidateRules() {
  const queryClient = useQueryClient()
  return () => {
    queryClient.invalidateQueries({ queryKey: [RULES_KEY] })
    queryClient.invalidateQueries({ queryKey: [MODELS_KEY] })
  }
}

export function useUpsertModelConcurrencyRule() {
  const invalidate = useInvalidateRules()

  return useMutation({
    mutationFn: (request: UpsertModelConcurrencyRequest) =>
      upsertModelConcurrencyRule(request),
    onSuccess: (res) => {
      if (res.success) {
        toast.success(i18next.t('Setting updated successfully'))
        invalidate()
        return
      }
      toast.error(res.message || i18next.t('Failed to update setting'))
    },
    onError: (error: Error) => {
      toast.error(error.message || i18next.t('Failed to update setting'))
    },
  })
}

export function useDeleteModelConcurrencyRule() {
  const invalidate = useInvalidateRules()

  return useMutation({
    mutationFn: (id: number) => deleteModelConcurrencyRule(id),
    onSuccess: (res) => {
      if (res.success) {
        toast.success(i18next.t('Deleted successfully'))
        invalidate()
        return
      }
      toast.error(res.message || i18next.t('Failed to delete'))
    },
    onError: (error: Error) => {
      toast.error(error.message || i18next.t('Failed to delete'))
    },
  })
}
