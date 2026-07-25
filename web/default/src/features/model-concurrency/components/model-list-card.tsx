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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
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

import type { ModelConcurrencySummary } from '../types'

/** null = 该模型只有指定用户规则，没有「所有用户」默认规则 */
function DefaultLimitCell({ limit }: { limit: number | null }) {
  const { t } = useTranslation()
  if (limit === null) {
    return <span className='text-muted-foreground'>{t('Not set')}</span>
  }
  if (limit === 0) {
    return <span>{t('Unlimited')}</span>
  }
  return <span>{limit}</span>
}

function EmptyListMessage({
  isLoading,
  hasAnyModel,
}: {
  isLoading: boolean
  hasAnyModel: boolean
}) {
  const { t } = useTranslation()
  if (isLoading) {
    return <span>{t('Loading...')}</span>
  }
  // 有规则但被筛选条件过滤掉了，和「一条规则都没有」要给不同提示
  if (hasAnyModel) {
    return <span>{t('No matching model')}</span>
  }
  return <span>{t('No model is limited yet. Add one above.')}</span>
}

type ModelListCardProps = {
  summaries: ModelConcurrencySummary[]
  isLoading: boolean
  selectedModel: string | null
  addPending: boolean
  deletePending: boolean
  onSelectModel: (modelName: string) => void
  onAddModel: (modelName: string, defaultLimit: number) => void
  onDeleteModel: (modelName: string) => void
}

/**
 * 已配置模型的总览列表 + 新增模型入口。
 * 新增时立刻写一条「所有用户」规则，因此模型会被持久化，下次进页面能直接看到。
 */
export function ModelListCard({
  summaries,
  isLoading,
  selectedModel,
  addPending,
  deletePending,
  onSelectModel,
  onAddModel,
  onDeleteModel,
}: ModelListCardProps) {
  const { t } = useTranslation()
  const [newModelName, setNewModelName] = useState('')
  const [newModelLimit, setNewModelLimit] = useState('0')
  const [filter, setFilter] = useState('')
  const [pendingDeleteModel, setPendingDeleteModel] = useState<string | null>(
    null
  )

  const trimmedNewModel = newModelName.trim()
  const parsedNewLimit = Number(newModelLimit)
  const alreadyConfigured = summaries.some(
    (summary) => summary.model_name === trimmedNewModel
  )
  const canAdd =
    trimmedNewModel !== '' &&
    !alreadyConfigured &&
    Number.isInteger(parsedNewLimit) &&
    parsedNewLimit >= 0 &&
    !addPending

  const normalizedFilter = filter.trim().toLowerCase()
  const visibleSummaries =
    normalizedFilter === ''
      ? summaries
      : summaries.filter((summary) =>
          summary.model_name.toLowerCase().includes(normalizedFilter)
        )

  const handleAdd = () => {
    if (!canAdd) return
    onAddModel(trimmedNewModel, parsedNewLimit)
    setNewModelName('')
    setNewModelLimit('0')
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Models with a concurrency rule')}</CardTitle>
        <CardDescription>
          {t(
            'Limits the number of async tasks (video, Suno, etc.) a user can have in progress at the same time on one model. A limit of 0 means unlimited. A rule for a specific user overrides the rule for all users. Models not listed here are not limited.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='flex flex-col gap-4'>
        <div className='flex flex-col gap-2 sm:flex-row sm:items-end'>
          <div className='flex flex-col gap-2'>
            <Label htmlFor='concurrency-new-model'>{t('Add a model')}</Label>
            <Input
              id='concurrency-new-model'
              className='w-72'
              value={newModelName}
              autoComplete='off'
              placeholder={t('Type a model name, e.g. sora-2')}
              onChange={(e) => setNewModelName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault()
                  handleAdd()
                }
              }}
            />
          </div>
          <div className='flex flex-col gap-2'>
            <Label htmlFor='concurrency-new-model-limit'>
              {t('Limit for all users')}
            </Label>
            <Input
              id='concurrency-new-model-limit'
              className='w-32'
              type='number'
              min={0}
              value={newModelLimit}
              onChange={(e) => setNewModelLimit(e.target.value)}
            />
          </div>
          <Button type='button' disabled={!canAdd} onClick={handleAdd}>
            {t('Add')}
          </Button>
        </div>

        {alreadyConfigured && (
          <p className='text-muted-foreground text-sm'>
            {t('This model already has a rule. Select it below to edit.')}
          </p>
        )}

        {summaries.length > 3 && (
          <Input
            className='w-72'
            value={filter}
            placeholder={t('Filter configured models')}
            onChange={(e) => setFilter(e.target.value)}
          />
        )}

        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Model')}</TableHead>
              <TableHead>{t('Limit for all users')}</TableHead>
              <TableHead>{t('User overrides')}</TableHead>
              <TableHead>{t('In progress')}</TableHead>
              <TableHead className='text-right'>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {visibleSummaries.length === 0 && (
              <TableRow>
                <TableCell
                  colSpan={5}
                  className='text-muted-foreground text-center'
                >
                  <EmptyListMessage
                    isLoading={isLoading}
                    hasAnyModel={summaries.length > 0}
                  />
                </TableCell>
              </TableRow>
            )}
            {visibleSummaries.map((summary) => (
              <TableRow
                key={summary.model_name}
                data-state={
                  selectedModel === summary.model_name ? 'selected' : undefined
                }
              >
                <TableCell className='font-medium'>
                  {summary.model_name}
                </TableCell>
                <TableCell>
                  <DefaultLimitCell limit={summary.default_limit} />
                </TableCell>
                <TableCell>
                  {summary.user_rule_count > 0 ? (
                    <Badge variant='secondary'>{summary.user_rule_count}</Badge>
                  ) : (
                    <span className='text-muted-foreground'>0</span>
                  )}
                </TableCell>
                <TableCell>{summary.current_total}</TableCell>
                <TableCell className='space-x-2 text-right'>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={() => onSelectModel(summary.model_name)}
                  >
                    {selectedModel === summary.model_name
                      ? t('Editing')
                      : t('Configure')}
                  </Button>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    disabled={deletePending}
                    onClick={() => setPendingDeleteModel(summary.model_name)}
                  >
                    {t('Remove limit')}
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>

      <AlertDialog
        open={pendingDeleteModel !== null}
        onOpenChange={(open) => {
          if (!open) setPendingDeleteModel(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Remove limit')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'This deletes the all-users rule and every user override for this model. The model goes back to unlimited.'
              )}
              {pendingDeleteModel ? ` (${pendingDeleteModel})` : ''}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                if (pendingDeleteModel) onDeleteModel(pendingDeleteModel)
                setPendingDeleteModel(null)
              }}
            >
              {t('Delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  )
}
