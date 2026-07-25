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
import { searchUsers } from '@/features/users/api'
import type { User } from '@/features/users/types'
import { useDebounce } from '@/hooks/use-debounce'

type UserRuleFormProps = {
  disabled?: boolean
  onSubmit: (userId: number, maxConcurrency: number) => void
}

/**
 * 为指定用户添加并发规则：边输入边搜索用户 → 点击选中 → 填并发上限。
 * 并发上限填 0 表示该用户在此模型上不限制（覆盖「所有用户」规则）。
 * 搜索不到时也允许直接手填用户 ID，避免管理端被搜索接口卡住。
 */
export function UserRuleForm({ disabled, onSubmit }: UserRuleFormProps) {
  const { t } = useTranslation()
  const [keyword, setKeyword] = useState('')
  const [selectedUser, setSelectedUser] = useState<User | null>(null)
  const [manualUserId, setManualUserId] = useState('')
  const [limitInput, setLimitInput] = useState('1')

  // 边打边搜：去抖后直接发请求，不需要再点「搜索」按钮。
  const debouncedKeyword = useDebounce(keyword.trim(), 400)

  const {
    data: users = [],
    isFetching,
    error: searchError,
  } = useQuery({
    queryKey: ['model-concurrency-user-search', debouncedKeyword],
    queryFn: async () => {
      const res = await searchUsers({
        keyword: debouncedKeyword,
        page_size: 20,
      })
      // 后端失败时返回 HTTP 200 + success:false，不能兜底成空数组，否则错误被隐藏
      if (!res.success) {
        throw new Error(res.message || 'search failed')
      }
      return res.data?.items ?? []
    },
    enabled: debouncedKeyword !== '',
    // 搜索结果短时间内复用即可，避免每次聚焦都重新拉
    staleTime: 30_000,
  })

  const manualId = Number(manualUserId.trim())
  const manualIdValid =
    manualUserId.trim() !== '' && Number.isInteger(manualId) && manualId > 0
  const effectiveUserId = selectedUser?.id ?? (manualIdValid ? manualId : null)

  const limit = Number(limitInput)
  const canSubmit =
    !disabled &&
    effectiveUserId !== null &&
    Number.isInteger(limit) &&
    limit >= 0

  const resetSelection = () => {
    setSelectedUser(null)
    setManualUserId('')
    setKeyword('')
  }

  return (
    <div className='flex flex-col gap-3 rounded-md border p-4'>
      <div className='flex flex-col gap-2'>
        <Label htmlFor='concurrency-user-keyword'>{t('Search user')}</Label>
        <Input
          id='concurrency-user-keyword'
          className='w-72'
          value={keyword}
          disabled={disabled}
          autoComplete='off'
          placeholder={t('Username, display name or email')}
          onChange={(e) => {
            setKeyword(e.target.value)
            setSelectedUser(null)
          }}
        />
      </div>

      {searchError && (
        <p className='text-destructive text-sm'>
          {t('Search failed')}: {(searchError as Error).message}
        </p>
      )}

      {debouncedKeyword !== '' && !selectedUser && (
        <div className='max-h-56 overflow-y-auto rounded-md border'>
          {isFetching && users.length === 0 && (
            <p className='text-muted-foreground p-2 text-sm'>
              {t('Searching...')}
            </p>
          )}
          {!isFetching && !searchError && users.length === 0 && (
            <p className='text-muted-foreground p-2 text-sm'>
              {t('No users found')}
            </p>
          )}
          {users.map((user) => (
            <button
              key={user.id}
              type='button'
              disabled={disabled}
              className='hover:bg-accent flex w-full items-center justify-between gap-2 px-2 py-1.5 text-left text-sm'
              onClick={() => setSelectedUser(user)}
            >
              <span className='truncate'>
                {user.username}
                {user.display_name ? ` (${user.display_name})` : ''}
              </span>
              <span className='text-muted-foreground shrink-0'>#{user.id}</span>
            </button>
          ))}
        </div>
      )}

      <div className='flex flex-col gap-2 sm:flex-row sm:items-end'>
        <div className='flex flex-col gap-2'>
          <Label htmlFor='concurrency-user-id'>{t('User ID')}</Label>
          <Input
            id='concurrency-user-id'
            className='w-40'
            inputMode='numeric'
            value={selectedUser ? String(selectedUser.id) : manualUserId}
            disabled={disabled || selectedUser !== null}
            placeholder={t('Or type a user ID')}
            onChange={(e) => setManualUserId(e.target.value)}
          />
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
            if (effectiveUserId === null) return
            onSubmit(effectiveUserId, limit)
            resetSelection()
            setLimitInput('1')
          }}
        >
          {t('Add rule')}
        </Button>

        {selectedUser && (
          <Button
            type='button'
            variant='outline'
            disabled={disabled}
            onClick={resetSelection}
          >
            {t('Clear')}
          </Button>
        )}
      </div>

      {selectedUser && (
        <p className='text-muted-foreground text-sm'>
          {t('Selected user')}: {selectedUser.username}
          {selectedUser.display_name
            ? ` (${selectedUser.display_name})`
            : ''} #
          {selectedUser.id}
        </p>
      )}
    </div>
  )
}
