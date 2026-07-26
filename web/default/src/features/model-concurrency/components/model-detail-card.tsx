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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

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

import { MODEL_CONCURRENCY_BLOCKED } from '../types'
import type { ModelConcurrencyRule } from '../types'
import { UserRuleForm } from './user-rule-form'

type ModelDetailCardProps = {
  modelName: string
  defaultRule: ModelConcurrencyRule | null
  userRules: ModelConcurrencyRule[]
  isLoading: boolean
  upsertPending: boolean
  deletePending: boolean
  onClose: () => void
  onSaveDefault: (maxConcurrency: number) => void
  onSaveUserRule: (userId: number, maxConcurrency: number) => void
  onDeleteRule: (id: number) => void
}

/** 单个模型的并发配置：「所有用户」默认上限 + 指定用户覆盖规则。 */
export function ModelDetailCard({
  modelName,
  defaultRule,
  userRules,
  isLoading,
  upsertPending,
  deletePending,
  onClose,
  onSaveDefault,
  onSaveUserRule,
  onDeleteRule,
}: ModelDetailCardProps) {
  const { t } = useTranslation()
  const [defaultLimitInput, setDefaultLimitInput] = useState('0')

  // 切换模型或规则刷新后，把「所有用户」的当前值同步到输入框。
  useEffect(() => {
    setDefaultLimitInput(String(defaultRule?.max_concurrency ?? 0))
  }, [defaultRule, modelName])

  const parsedDefaultLimit = Number(defaultLimitInput)
  const canSaveDefault =
    Number.isInteger(parsedDefaultLimit) &&
    parsedDefaultLimit >= MODEL_CONCURRENCY_BLOCKED &&
    !upsertPending

  return (
    <Card>
      <CardHeader className='flex flex-row items-start justify-between gap-2'>
        <div className='flex flex-col gap-1.5'>
          <CardTitle>{modelName}</CardTitle>
          <CardDescription>
            {t(
              'A rule for a specific user overrides the all-users limit on this model. 0 means unlimited, -1 forbids using the model.'
            )}
          </CardDescription>
        </div>
        <Button type='button' variant='outline' size='sm' onClick={onClose}>
          {t('Close')}
        </Button>
      </CardHeader>
      <CardContent className='flex flex-col gap-6'>
        <div className='flex flex-col gap-2'>
          <p className='text-sm font-medium'>{t('All users')}</p>
          <div className='flex flex-col gap-2 sm:flex-row sm:items-end'>
            <div className='flex flex-col gap-2'>
              <Label htmlFor='concurrency-default-limit'>
                {t('Concurrency limit')}
              </Label>
              <Input
                id='concurrency-default-limit'
                className='w-32'
                type='number'
                min={MODEL_CONCURRENCY_BLOCKED}
                value={defaultLimitInput}
                onChange={(e) => setDefaultLimitInput(e.target.value)}
              />
            </div>
            <Button
              type='button'
              disabled={!canSaveDefault}
              onClick={() => onSaveDefault(parsedDefaultLimit)}
            >
              {t('Save')}
            </Button>
            {parsedDefaultLimit === 0 && (
              <p className='text-muted-foreground pb-2 text-sm'>
                {t('0 means unlimited')}
              </p>
            )}
            {parsedDefaultLimit === MODEL_CONCURRENCY_BLOCKED && (
              <p className='text-destructive pb-2 text-sm'>
                {t(
                  '-1 forbids all users from using this model, except users with their own rule below'
                )}
              </p>
            )}
          </div>
        </div>

        <div className='flex flex-col gap-4'>
          <p className='text-sm font-medium'>{t('Specific users')}</p>
          <UserRuleForm disabled={upsertPending} onSubmit={onSaveUserRule} />

          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('User')}</TableHead>
                <TableHead>{t('Concurrency limit')}</TableHead>
                <TableHead>{t('In progress')}</TableHead>
                <TableHead className='text-right'>{t('Actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {userRules.length === 0 && (
                <TableRow>
                  <TableCell
                    colSpan={4}
                    className='text-muted-foreground text-center'
                  >
                    {isLoading ? t('Loading...') : t('No rules yet')}
                  </TableCell>
                </TableRow>
              )}
              {userRules.map((rule) => (
                <TableRow key={rule.id}>
                  <TableCell>{rule.username || `#${rule.user_id}`}</TableCell>
                  <TableCell>
                    {rule.max_concurrency === MODEL_CONCURRENCY_BLOCKED ? (
                      <span className='text-destructive'>
                        {t('Not allowed')}
                      </span>
                    ) : rule.max_concurrency === 0 ? (
                      t('Unlimited')
                    ) : (
                      rule.max_concurrency
                    )}
                  </TableCell>
                  <TableCell>{rule.current}</TableCell>
                  <TableCell className='text-right'>
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      disabled={deletePending}
                      onClick={() => onDeleteRule(rule.id)}
                    >
                      {t('Delete')}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </CardContent>
    </Card>
  )
}
