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
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useDebounce } from '@/hooks/use-debounce'

import { UserRuleForm } from './components/user-rule-form'
import {
  useDeleteModelConcurrencyRule,
  useModelConcurrencyRules,
  useUpsertModelConcurrencyRule,
} from './hooks/use-model-concurrency'
import { MODEL_CONCURRENCY_ALL_USERS } from './types'

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

function ModelConcurrencyContent() {
  const { t } = useTranslation()
  const [modelName, setModelName] = useState('')
  const [defaultLimitInput, setDefaultLimitInput] = useState('0')

  const trimmedModelName = modelName.trim()
  // 手输模型名时按去抖拉规则，避免每敲一个字符就打一次接口。
  const queriedModelName = useDebounce(trimmedModelName, 400)
  const { data: rules = [], isFetching } =
    useModelConcurrencyRules(queriedModelName)

  const upsertRule = useUpsertModelConcurrencyRule()
  const deleteRule = useDeleteModelConcurrencyRule()

  const defaultRule = useMemo(
    () =>
      rules.find((rule) => rule.user_id === MODEL_CONCURRENCY_ALL_USERS) ?? null,
    [rules]
  )
  const userRules = useMemo(
    () => rules.filter((rule) => rule.user_id !== MODEL_CONCURRENCY_ALL_USERS),
    [rules]
  )

  // 切换模型或规则加载完成后，把「所有用户」的当前值同步到输入框。
  useEffect(() => {
    setDefaultLimitInput(String(defaultRule?.max_concurrency ?? 0))
  }, [defaultRule, modelName])

  const parsedDefaultLimit = Number(defaultLimitInput)
  const canSaveDefault =
    trimmedModelName !== '' &&
    Number.isInteger(parsedDefaultLimit) &&
    parsedDefaultLimit >= 0 &&
    !upsertRule.isPending

  return (
    <div className='flex flex-col gap-6'>
      <Card>
        <CardHeader>
          <CardTitle>{t('Model')}</CardTitle>
          <CardDescription>
            {t(
              'Limits the number of async tasks (video, Suno, etc.) a user can have in progress at the same time on one model. A limit of 0 means unlimited. A rule for a specific user overrides the rule for all users.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='flex flex-col gap-4'>
          <div className='flex flex-col gap-2'>
            <Label htmlFor='concurrency-model-input'>{t('Model name')}</Label>
            <Input
              id='concurrency-model-input'
              className='w-72'
              value={modelName}
              placeholder={t('Type a model name, e.g. sora-2')}
              onChange={(e) => setModelName(e.target.value)}
            />
            <p className='text-muted-foreground text-sm'>
              {t(
                'Type the model name exactly as it appears in requests. Models without a rule are not limited.'
              )}
            </p>
          </div>
        </CardContent>
      </Card>

      {/* 去抖期间先不渲染，避免把上一个模型的规则错配到正在输入的模型名上 */}
      {trimmedModelName !== '' && queriedModelName === trimmedModelName && (
        <>
          <Card>
            <CardHeader>
              <CardTitle>{t('All users')}</CardTitle>
              <CardDescription>
                {t(
                  'Default concurrency limit applied to every user on this model.'
                )}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className='flex flex-col gap-2 sm:flex-row sm:items-end'>
                <div className='flex flex-col gap-2'>
                  <Label htmlFor='concurrency-default-limit'>
                    {t('Concurrency limit')}
                  </Label>
                  <Input
                    id='concurrency-default-limit'
                    className='w-32'
                    type='number'
                    min={0}
                    value={defaultLimitInput}
                    onChange={(e) => setDefaultLimitInput(e.target.value)}
                  />
                </div>
                <Button
                  type='button'
                  disabled={!canSaveDefault}
                  onClick={() =>
                    upsertRule.mutate({
                      model_name: trimmedModelName,
                      user_id: MODEL_CONCURRENCY_ALL_USERS,
                      max_concurrency: parsedDefaultLimit,
                    })
                  }
                >
                  {t('Save')}
                </Button>
                {parsedDefaultLimit === 0 && (
                  <p className='text-muted-foreground pb-2 text-sm'>
                    {t('0 means unlimited')}
                  </p>
                )}
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t('Specific users')}</CardTitle>
              <CardDescription>
                {t(
                  'A rule here overrides the all-users limit for that user on this model.'
                )}
              </CardDescription>
            </CardHeader>
            <CardContent className='flex flex-col gap-4'>
              <UserRuleForm
                disabled={upsertRule.isPending}
                onSubmit={(userId, maxConcurrency) =>
                  upsertRule.mutate({
                    model_name: trimmedModelName,
                    user_id: userId,
                    max_concurrency: maxConcurrency,
                  })
                }
              />

              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('User')}</TableHead>
                    <TableHead>{t('Concurrency limit')}</TableHead>
                    <TableHead>{t('In progress')}</TableHead>
                    <TableHead className='text-right'>
                      {t('Actions')}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {userRules.length === 0 && (
                    <TableRow>
                      <TableCell
                        colSpan={4}
                        className='text-muted-foreground text-center'
                      >
                        {isFetching ? t('Loading...') : t('No rules yet')}
                      </TableCell>
                    </TableRow>
                  )}
                  {userRules.map((rule) => (
                    <TableRow key={rule.id}>
                      <TableCell>
                        {rule.username || `#${rule.user_id}`}
                      </TableCell>
                      <TableCell>
                        {rule.max_concurrency === 0
                          ? t('Unlimited')
                          : rule.max_concurrency}
                      </TableCell>
                      <TableCell>{rule.current}</TableCell>
                      <TableCell className='text-right'>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          disabled={deleteRule.isPending}
                          onClick={() => deleteRule.mutate(rule.id)}
                        >
                          {t('Delete')}
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </>
      )}
    </div>
  )
}
