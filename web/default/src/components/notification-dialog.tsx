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
import { Bell, Megaphone } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import {
  AnnouncementsContent,
  type AnnouncementItem,
  NoticeContent,
} from '@/components/notification-popover'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

interface NotificationDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  activeTab: 'notice' | 'announcements'
  onTabChange: (tab: 'notice' | 'announcements') => void
  notice: string
  announcements: AnnouncementItem[]
  loading: boolean
}

export function NotificationDialog({
  open,
  onOpenChange,
  activeTab,
  onTabChange,
  notice,
  announcements,
  loading,
}: NotificationDialogProps) {
  const { t } = useTranslation()

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={
        <span className='flex items-center gap-2'>
          {activeTab === 'announcements' ? (
            <Megaphone className='text-primary size-5' />
          ) : (
            <Bell className='text-primary size-5' />
          )}
          {t('System Announcements')}
        </span>
      }
      description={t('Latest platform updates and notices')}
      contentClassName='border-primary/20 shadow-2xl shadow-primary/10 sm:max-w-2xl'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={<Button onClick={() => onOpenChange(false)}>{t('Close')}</Button>}
    >
      <Tabs
        value={activeTab}
        onValueChange={onTabChange as (value: string) => void}
      >
        <TabsList className='grid w-full grid-cols-2'>
          <TabsTrigger value='notice' className='gap-1.5'>
            <Bell className='size-3.5' />
            {t('Notice')}
          </TabsTrigger>
          <TabsTrigger value='announcements' className='gap-1.5'>
            <Megaphone className='size-3.5' />
            {t('Timeline')}
          </TabsTrigger>
        </TabsList>

        <TabsContent value='notice' className='mt-4'>
          <NoticeContent notice={notice} loading={loading} t={t} />
        </TabsContent>

        <TabsContent value='announcements' className='mt-4'>
          <AnnouncementsContent
            announcements={announcements}
            loading={loading}
            t={t}
          />
        </TabsContent>
      </Tabs>
    </Dialog>
  )
}
