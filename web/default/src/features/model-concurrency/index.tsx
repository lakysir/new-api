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
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { ModelDetailCard } from './components/model-detail-card'
import { ModelListCard } from './components/model-list-card'
import {
  useAllModelConcurrencyRules,
  useDeleteModelConcurrencyModel,
  useDeleteModelConcurrencyRule,
  useUpsertModelConcurrencyRule,
  useDeleteUserAsyncConcurrencyRule,
  useUpsertUserAsyncConcurrencyRule,
  useUserAsyncConcurrencyRules,
} from './hooks/use-model-concurrency'
import { UserTotalConcurrencyCard } from './components/user-total-concurrency-card'
import {
  MODEL_CONCURRENCY_ALL_USERS,
  type ModelConcurrencyRule,
  type ModelConcurrencySummary,
} from './types'

export function ModelConcurrency() {
  const { t } = useTranslation()
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Async Task Concurrency')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <ModelConcurrencyContent />
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

type ModelGroup = {
  defaultRule: ModelConcurrencyRule | null
  userRules: ModelConcurrencyRule[]
}

/** 把扁平的规则列表按模型分组，列表页和明细页共用这一份数据。 */
function groupRulesByModel(rules: ModelConcurrencyRule[]) {
  const groups = new Map<string, ModelGroup>()
  for (const rule of rules) {
    let group = groups.get(rule.model_name)
    if (!group) {
      group = { defaultRule: null, userRules: [] }
      groups.set(rule.model_name, group)
    }
    if (rule.user_id === MODEL_CONCURRENCY_ALL_USERS) {
      group.defaultRule = rule
    } else {
      group.userRules.push(rule)
    }
  }
  return groups
}

function ModelConcurrencyContent() {
  const { t } = useTranslation()
  const [selectedModel, setSelectedModel] = useState<string | null>(null)

  const { data: rules = [], isLoading } = useAllModelConcurrencyRules()
  const upsertRule = useUpsertModelConcurrencyRule()
  const deleteRule = useDeleteModelConcurrencyRule()
  const deleteModel = useDeleteModelConcurrencyModel()
  const { data: userRules = [], isLoading: userRulesLoading } = useUserAsyncConcurrencyRules()
  const upsertUserRule = useUpsertUserAsyncConcurrencyRule()
  const deleteUserRule = useDeleteUserAsyncConcurrencyRule()

  const groups = useMemo(() => groupRulesByModel(rules), [rules])

  const summaries = useMemo<ModelConcurrencySummary[]>(
    () =>
      [...groups.entries()]
        .map(([modelName, group]) => ({
          model_name: modelName,
          default_limit: group.defaultRule?.max_concurrency ?? null,
          user_rule_count: group.userRules.length,
          // 「所有用户」行的 current 由后端填成该模型全部用户的进行中任务总数；
          // 只配了指定用户规则时退化为这些用户的求和。
          current_total:
            group.defaultRule?.current ??
            group.userRules.reduce((sum, rule) => sum + rule.current, 0),
        }))
        .sort((a, b) => a.model_name.localeCompare(b.model_name)),
    [groups]
  )

  const selectedGroup = selectedModel ? groups.get(selectedModel) : undefined

  const handleAddModel = (modelName: string, defaultLimit: number) => {
    // 立刻落库一条「所有用户」规则，模型从此出现在列表里（0 也会存，表示已配置但不限制）
    upsertRule.mutate({
      model_name: modelName,
      user_id: MODEL_CONCURRENCY_ALL_USERS,
      max_concurrency: defaultLimit,
    })
    setSelectedModel(modelName)
  }

  const handleDeleteModel = (modelName: string) => {
    deleteModel.mutate(modelName)
    if (selectedModel === modelName) {
      setSelectedModel(null)
    }
  }

  return (
    <Tabs defaultValue='models' className='flex flex-col gap-6'>
      <TabsList>
        <TabsTrigger value='models'>{t('Model concurrency')}</TabsTrigger>
        <TabsTrigger value='users'>{t('User total concurrency')}</TabsTrigger>
      </TabsList>
      <TabsContent value='models' className='flex flex-col gap-6'>
      <ModelListCard
        summaries={summaries}
        isLoading={isLoading}
        selectedModel={selectedModel}
        addPending={upsertRule.isPending}
        deletePending={deleteModel.isPending}
        onSelectModel={setSelectedModel}
        onAddModel={handleAddModel}
        onDeleteModel={handleDeleteModel}
      />

      {selectedModel !== null && (
        <ModelDetailCard
          modelName={selectedModel}
          defaultRule={selectedGroup?.defaultRule ?? null}
          userRules={selectedGroup?.userRules ?? []}
          isLoading={isLoading}
          upsertPending={upsertRule.isPending}
          deletePending={deleteRule.isPending}
          onClose={() => setSelectedModel(null)}
          onSaveDefault={(maxConcurrency) =>
            upsertRule.mutate({
              model_name: selectedModel,
              user_id: MODEL_CONCURRENCY_ALL_USERS,
              max_concurrency: maxConcurrency,
            })
          }
          onSaveUserRule={(userId, maxConcurrency) =>
            upsertRule.mutate({
              model_name: selectedModel,
              user_id: userId,
              max_concurrency: maxConcurrency,
            })
          }
          onDeleteRule={(id) => deleteRule.mutate(id)}
        />
      )}
      </TabsContent>
      <TabsContent value='users'>
        <UserTotalConcurrencyCard
          rules={userRules}
          isLoading={userRulesLoading}
          pending={upsertUserRule.isPending}
          deletePending={deleteUserRule.isPending}
          onSave={(userId, maxConcurrency) => upsertUserRule.mutate({ user_id: userId, max_concurrency: maxConcurrency })}
          onDelete={(userId) => deleteUserRule.mutate(userId)}
        />
      </TabsContent>
    </Tabs>
  )
}
