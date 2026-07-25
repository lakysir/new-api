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
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  NativeSelect,
  NativeSelectOption,
} from '@/components/ui/native-select'

import { searchUsers } from '@/features/users/api'

type UserRuleFormProps = {
  disabled?: boolean
  onSubmit: (userId: number, maxConcurrency: number) => void
}

/**
 * 为指定用户添加并发规则：按关键字搜索用户 → 选中 → 填并发上限。
 * 并发上限填 0 表示该用户在此模型上不限制（覆盖「所有用户」规则）。
 */
export function UserRuleForm({ disabled, onSubmit }: UserRuleFormProps) {
  const { t } = useTranslation()
  const [keyword, setKeyword] = useState('')
  const [submittedKeyword, setSubmittedKeyword] = useState('')
  const [selectedUserId, setSelectedUserId] = useState('')
  const [limitInput, setLimitInput] = useState('1')

  const { data: users = [], isFetching } = useQuery({
    queryKey: ['model-concurrency-user-search', submittedKeyword],
    queryFn: async () => {
      const res = await searchUsers({
        keyword: submittedKeyword,
        page_size: 20,
      })
      return res.data?.items ?? []
    },
    enabled: submittedKeyword !== '',
  })

  const limit = Number(limitInput)
  const canSubmit =
    !disabled &&
    selectedUserId !== '' &&
    Number.isInteger(limit) &&
    limit >= 0

  return (
    <div className='flex flex-col gap-3 rounded-md border p-4'>
      <div className='flex flex-col gap-2 sm:flex-row sm:items-end'>
        <div className='flex flex-col gap-2'>
          <Label htmlFor='concurrency-user-keyword'>
            {t('Search user')}
          </Label>
          <Input
            id='concurrency-user-keyword'
            className='w-56'
            value={keyword}
            disabled={disabled}
            placeholder={t('Username, display name or email')}
            onChange={(e) => setKeyword(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                setSubmittedKeyword(keyword.trim())
              }
            }}
          />
        </div>
        <Button
          type='button'
          variant='outline'
          disabled={disabled || keyword.trim() === ''}
          onClick={() => setSubmittedKeyword(keyword.trim())}
        >
          {isFetching ? t('Searching...') : t('Search')}
        </Button>
      </div>

      <div className='flex flex-col gap-2 sm:flex-row sm:items-end'>
        <div className='flex flex-col gap-2'>
          <Label htmlFor='concurrency-user-select'>{t('User')}</Label>
          <NativeSelect className='w-56'>
            <select
              id='concurrency-user-select'
              value={selectedUserId}
              disabled={disabled || users.length === 0}
              onChange={(e) => setSelectedUserId(e.target.value)}
            >
              <NativeSelectOption value=''>
                {users.length === 0
                  ? t('Search for a user first')
                  : t('Select a user')}
              </NativeSelectOption>
              {users.map((user) => (
                <NativeSelectOption key={user.id} value={String(user.id)}>
                  {user.username}
                  {user.display_name ? ` (${user.display_name})` : ''}
                </NativeSelectOption>
              ))}
            </select>
          </NativeSelect>
        </div>

        <div className='flex flex-col gap-2'>
          <Label htmlFor='concurrency-user-limit'>
            {t('Concurrency limit')}
          </Label>
          <Input
            id='concurrency-user-limit'
            className='w-32'
            type='number'
            min={0}
            value={limitInput}
            disabled={disabled}
            onChange={(e) => setLimitInput(e.target.value)}
          />
        </div>

        <Button
          type='button'
          disabled={!canSubmit}
          onClick={() => {
            onSubmit(Number(selectedUserId), limit)
            setSelectedUserId('')
            setLimitInput('1')
          }}
        >
          {t('Add rule')}
        </Button>
      </div>
    </div>
  )
}
