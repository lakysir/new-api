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
import { type Table } from '@tanstack/react-table'
import { Ban, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataTableBulkActions as BulkActionsToolbar } from '@/components/data-table'
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
import { Button } from '@/components/ui/button'

import { manageUser } from '../api'
import { type User } from '../types'
import { useUsers } from './users-provider'

interface DataTableBulkActionsProps {
  table: Table<User>
}

export function DataTableBulkActions({ table }: DataTableBulkActionsProps) {
  const { t } = useTranslation()
  const { triggerRefresh } = useUsers()
  const [pendingAction, setPendingAction] = useState<'disable' | 'delete' | null>(null)
  const [loading, setLoading] = useState(false)
  const selectedUsers = table.getFilteredSelectedRowModel().rows.map((row) => row.original)

  const handleConfirm = async () => {
    if (!pendingAction || selectedUsers.length === 0) return
    setLoading(true)
    const results = await Promise.allSettled(
      selectedUsers.map((user) => manageUser(user.id, pendingAction))
    )
    const successCount = results.filter(
      (result) => result.status === 'fulfilled' && result.value.success
    ).length
    const failureCount = results.length - successCount
    if (successCount > 0) {
      toast.success(
        t('Bulk user action completed', { count: successCount })
      )
      triggerRefresh()
    }
    if (failureCount > 0) {
      toast.error(t('Some users could not be processed', { count: failureCount }))
    }
    table.resetRowSelection()
    setPendingAction(null)
    setLoading(false)
  }

  return (
    <>
      <BulkActionsToolbar table={table} entityName='user'>
        <Button
          variant='outline'
          size='sm'
          onClick={() => setPendingAction('disable')}
          disabled={loading}
          title={t('Disable selected users')}
        >
          <Ban className='mr-1 h-4 w-4' />
          <span className='hidden sm:inline'>{t('Disable')}</span>
        </Button>
        <Button
          variant='destructive'
          size='sm'
          onClick={() => setPendingAction('delete')}
          disabled={loading}
          title={t('Delete selected users')}
        >
          <Trash2 className='mr-1 h-4 w-4' />
          <span className='hidden sm:inline'>{t('Delete')}</span>
        </Button>
      </BulkActionsToolbar>
      <AlertDialog
        open={pendingAction !== null}
        onOpenChange={(open) => !open && !loading && setPendingAction(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Are you sure?')}</AlertDialogTitle>
            <AlertDialogDescription>
              {pendingAction === 'delete'
                ? t('This will permanently delete the selected users. This action cannot be undone.')
                : t('The selected users will be disabled and unable to use the service.')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={loading}>{t('Cancel')}</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleConfirm}
              disabled={loading}
              className={pendingAction === 'delete' ? 'bg-destructive text-destructive-foreground hover:bg-destructive/90' : undefined}
            >
              {loading ? t('Processing...') : t('Confirm')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
