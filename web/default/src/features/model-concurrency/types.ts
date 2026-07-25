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

/** user_id === 0 表示该模型对所有用户生效的默认规则 */
export const MODEL_CONCURRENCY_ALL_USERS = 0

export type ModelConcurrencyApiResponse<T> = {
  success: boolean
  message?: string
  data?: T
}

export type ModelConcurrencyRule = {
  id: number
  model_name: string
  user_id: number
  /** 0 表示不限制 */
  max_concurrency: number
  created_time: number
  updated_time: number
  username?: string
  /** 该用户在该模型上进行中的异步任务数 */
  current: number
}

/** 列表页每行一个模型，由全部规则在前端按 model_name 聚合得到 */
export type ModelConcurrencySummary = {
  model_name: string
  /** 「所有用户」规则的上限；null 表示该模型只配了指定用户规则 */
  default_limit: number | null
  /** 指定用户规则条数 */
  user_rule_count: number
  /** 该模型上所有用户进行中的异步任务总数 */
  current_total: number
}

export type UpsertModelConcurrencyRequest = {
  model_name: string
  user_id: number
  max_concurrency: number
}
