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
  deleteModelConcurrencyRulesByModel,
  getModelConcurrencyRules,
  upsertModelConcurrencyRule,
} from '../api'
import type { UpsertModelConcurrencyRequest } from '../types'

const RULES_KEY = 'model-concurrency-rules'

/**
 * 后端 common.ApiError 返回的是 HTTP 200 + success:false，若直接兜底成空数组，
 * 真实错误会被伪装成「没有数据」而无法排查。这里显式抛出后端消息。
 */
function unwrap<T>(res: {
  success: boolean
  message?: string
  data?: T
}): T | undefined {
  if (!res.success) {
    throw new Error(res.message || 'request failed')
  }
  return res.data
}

/**
 * 一次拉回全部规则（不带 model 过滤），页面进来即可列出所有「已配置」的模型。
 * 单个模型的明细在前端按 model_name 分组得到，切换模型无需再发请求。
 */
export function useAllModelConcurrencyRules() {
  return useQuery({
    queryKey: [RULES_KEY],
    queryFn: async () => {
      const res = await getModelConcurrencyRules()
      return unwrap(res) ?? []
    },
  })
}

function useInvalidateRules() {
  const queryClient = useQueryClient()
  return () => {
    queryClient.invalidateQueries({ queryKey: [RULES_KEY] })
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

export function useDeleteModelConcurrencyModel() {
  const invalidate = useInvalidateRules()

  return useMutation({
    mutationFn: (modelName: string) =>
      deleteModelConcurrencyRulesByModel(modelName),
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
