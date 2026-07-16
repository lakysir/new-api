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
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { useStatus } from '@/hooks/use-status'
import { getAnnouncementKey } from '@/lib/announcements'
import { getNotice } from '@/lib/api'
import { useNotificationStore } from '@/stores/notification-store'

interface NotificationAnnouncement {
  id?: number | string
  type?: string
  content?: string
  extra?: string
  publishDate?: string | Date
  title?: string
  link?: string
}

/**
 * Hook to manage notifications (Notice + Announcements)
 * Provides unread counts and read status management
 */
export function useNotifications() {
  const [popoverOpen, setPopoverOpen] = useState(false)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [activeTab, setActiveTab] = useState<'notice' | 'announcements'>(
    'notice'
  )
  const autoOpenedSignatureRef = useRef<string | null>(null)

  // Fetch Notice from API
  const {
    data: noticeResponse,
    isLoading: noticeLoading,
    refetch: refetchNotice,
  } = useQuery({
    queryKey: ['notice'],
    queryFn: getNotice,
    staleTime: 1000 * 60 * 5, // 5 minutes
  })

  // Fetch Announcements from status
  const { status, loading: statusLoading } = useStatus()
  const announcementsEnabled = status?.announcements_enabled ?? false
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const announcements: NotificationAnnouncement[] = announcementsEnabled
    ? ((status?.announcements || []) as NotificationAnnouncement[]).slice(0, 20)
    : []

  // Notification store
  const {
    lastReadNotice,
    markNoticeRead,
    markAnnouncementsRead,
    isAnnouncementRead,
  } = useNotificationStore()

  // Extract notice content
  const noticeContent = noticeResponse?.success
    ? (noticeResponse.data || '').trim()
    : ''

  // Calculate unread counts
  const unreadCounts = useMemo(() => {
    const noticeUnread =
      noticeContent && noticeContent !== lastReadNotice ? 1 : 0

    const announcementsUnread = announcements.filter((item) => {
      const key = getAnnouncementKey(item)
      return !isAnnouncementRead(key)
    }).length

    return {
      notice: noticeUnread,
      announcements: announcementsUnread,
      total: noticeUnread + announcementsUnread,
    }
  }, [noticeContent, lastReadNotice, announcements, isAnnouncementRead])

  const unreadAnnouncementKeys = useMemo(
    () =>
      announcements
        .map((item) => getAnnouncementKey(item))
        .filter((key) => key && !isAnnouncementRead(key)),
    [announcements, isAnnouncementRead]
  )

  const markAnnouncementsAsRead = useCallback(() => {
    if (announcements.length > 0) {
      const allKeys = announcements.map((item) => getAnnouncementKey(item))
      markAnnouncementsRead(allKeys)
    }
  }, [announcements, markAnnouncementsRead])

  // Handle popover open
  const handleOpenPopover = useCallback(
    (tab?: 'notice' | 'announcements') => {
      const nextTab = tab || activeTab

      // Mark currently visible content as read when opening the notification center
      if (noticeContent) {
        markNoticeRead(noticeContent)
      }
      if (nextTab === 'announcements') {
        markAnnouncementsAsRead()
      }

      setActiveTab(nextTab)
      setPopoverOpen(true)
    },
    [
      activeTab,
      markAnnouncementsAsRead,
      markNoticeRead,
      noticeContent,
      setActiveTab,
      setPopoverOpen,
    ]
  )

  const handlePopoverOpenChange = useCallback(
    (open: boolean) => {
      if (open) {
        handleOpenPopover(activeTab)
        return
      }

      setPopoverOpen(false)
    },
    [activeTab, handleOpenPopover]
  )

  // Handle tab change - mark announcements as read when switching to that tab
  const handleTabChange = useCallback(
    (tab: 'notice' | 'announcements') => {
      setActiveTab(tab)

      if (tab === 'announcements') {
        markAnnouncementsAsRead()
      }
    },
    [markAnnouncementsAsRead]
  )

  const handleOpenDialog = useCallback(
    (tab: 'notice' | 'announcements') => {
      if (noticeContent) {
        markNoticeRead(noticeContent)
      }
      if (tab === 'announcements') {
        markAnnouncementsAsRead()
      }

      setActiveTab(tab)
      setDialogOpen(true)
    },
    [markAnnouncementsAsRead, markNoticeRead, noticeContent]
  )

  useEffect(() => {
    if (noticeLoading || statusLoading || popoverOpen || dialogOpen) return
    if (unreadCounts.total <= 0) return

    const targetTab: 'notice' | 'announcements' =
      unreadCounts.announcements > 0 ? 'announcements' : 'notice'
    const signature = [
      targetTab,
      noticeContent && noticeContent !== lastReadNotice ? noticeContent : '',
      unreadAnnouncementKeys.join('|'),
    ].join('::')

    if (!signature || autoOpenedSignatureRef.current === signature) return

    autoOpenedSignatureRef.current = signature
    handleOpenDialog(targetTab)
  }, [
    dialogOpen,
    handleOpenDialog,
    lastReadNotice,
    noticeContent,
    noticeLoading,
    popoverOpen,
    statusLoading,
    unreadAnnouncementKeys,
    unreadCounts.announcements,
    unreadCounts.total,
  ])

  return {
    // Data
    notice: noticeContent,
    announcements,
    loading: noticeLoading || statusLoading,

    // Unread counts
    unreadCount: unreadCounts.total,
    unreadNoticeCount: unreadCounts.notice,
    unreadAnnouncementsCount: unreadCounts.announcements,

    // Popover state
    popoverOpen,
    setPopoverOpen: handlePopoverOpenChange,
    dialogOpen,
    setDialogOpen,
    activeTab,
    setActiveTab: handleTabChange,

    // Actions
    openPopover: handleOpenPopover,
    closePopover: () => setPopoverOpen(false),
    refetchNotice,
  }
}
