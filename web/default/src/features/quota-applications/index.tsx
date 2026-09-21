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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Check,
  ChevronLeft,
  ChevronRight,
  Eye,
  Plus,
  Search,
  X,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import {
  formatQuota,
  formatTimestamp,
  parseQuotaFromDollars,
} from '@/lib/format'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  createQuotaApplication,
  listQuotaApplications,
  reviewQuotaApplication,
  searchQuotaApplicationTargets,
} from './api'
import type {
  QuotaApplication,
  QuotaApplicationStatus,
  QuotaApplicationTarget,
} from './types'

const statusLabels: Record<QuotaApplicationStatus, string> = {
  pending: 'Pending review',
  approved: 'Credited',
  rejected: 'Rejected',
}

function ApplicationStatusBadge(props: { status: QuotaApplicationStatus }) {
  const { t } = useTranslation()
  let variant: 'default' | 'destructive' | 'secondary' = 'secondary'
  if (props.status === 'approved') variant = 'default'
  if (props.status === 'rejected') variant = 'destructive'
  return <Badge variant={variant}>{t(statusLabels[props.status])}</Badge>
}

function ApplicationDetails(props: {
  application: QuotaApplication | null
  onClose: () => void
}) {
  const { t } = useTranslation()
  const item = props.application
  return (
    <Dialog
      open={item !== null}
      onOpenChange={(open) => !open && props.onClose()}
      title={t('Quota application details')}
      contentHeight='auto'
      footer={
        <Button variant='outline' onClick={props.onClose}>
          {t('Close')}
        </Button>
      }
    >
      {item && (
        <div className='space-y-5 text-sm'>
          <div className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
            <div>
              <p className='text-muted-foreground'>{t('Application ID')}</p>
              <p className='font-medium'>#{item.id}</p>
            </div>
            <div>
              <p className='text-muted-foreground'>{t('Status')}</p>
              <ApplicationStatusBadge status={item.status} />
            </div>
            <div>
              <p className='text-muted-foreground'>{t('Applicant')}</p>
              <p className='font-medium'>{item.applicant_username}</p>
            </div>
            <div>
              <p className='text-muted-foreground'>{t('Target user')}</p>
              <p className='font-medium'>{item.target_username}</p>
              <p className='text-muted-foreground'>
                {item.target_display_name && `${item.target_display_name} · `}
                ID {item.target_user_id}
              </p>
            </div>
            <div>
              <p className='text-muted-foreground'>{t('Credit amount')}</p>
              <p className='font-medium'>{formatQuota(item.quota_amount)}</p>
            </div>
            <div>
              <p className='text-muted-foreground'>
                {t('Actual invoiceable amount received (CNY)')}
              </p>
              <p className='font-medium'>
                CNY {(item.invoice_amount_cents / 100).toFixed(2)}
              </p>
            </div>
            <div>
              <p className='text-muted-foreground'>{t('Submitted at')}</p>
              <p className='font-medium'>{formatTimestamp(item.created_at)}</p>
            </div>
          </div>
          <div>
            <p className='text-muted-foreground mb-1'>
              {t('Application information')}
            </p>
            <p className='bg-muted/50 rounded-md border p-3 whitespace-pre-wrap'>
              {item.application_info}
            </p>
          </div>
          {item.status !== 'pending' && (
            <div className='border-t pt-4'>
              <p className='font-medium'>{t('Review result')}</p>
              <p className='text-muted-foreground mt-1'>
                {item.reviewer_username || `ID ${item.reviewed_by}`} ·{' '}
                {formatTimestamp(item.reviewed_at)}
              </p>
              {item.review_comment && (
                <p className='mt-2 whitespace-pre-wrap'>
                  {item.review_comment}
                </p>
              )}
              {item.status === 'approved' && (
                <p className='mt-2'>
                  {formatQuota(item.quota_before)} →{' '}
                  {formatQuota(item.quota_after)}
                </p>
              )}
            </div>
          )}
        </div>
      )}
    </Dialog>
  )
}

function CreateApplicationDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: () => void
}) {
  const { t } = useTranslation()
  const [keywordInput, setKeywordInput] = useState('')
  const [keyword, setKeyword] = useState('')
  const [selectedUser, setSelectedUser] =
    useState<QuotaApplicationTarget | null>(null)
  const [amount, setAmount] = useState('')
  const [invoiceAmount, setInvoiceAmount] = useState('')
  const [applicationInfo, setApplicationInfo] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const { data: targets = [], isFetching } = useQuery({
    queryKey: ['quota-application-targets', keyword],
    queryFn: () => searchQuotaApplicationTargets(keyword),
    enabled: props.open && keyword.length > 0,
  })

  const reset = () => {
    setKeywordInput('')
    setKeyword('')
    setSelectedUser(null)
    setAmount('')
    setInvoiceAmount('')
    setApplicationInfo('')
  }
  const close = () => {
    reset()
    props.onOpenChange(false)
  }
  const submit = async () => {
    const quotaAmount = parseQuotaFromDollars(Number(amount))
    const invoiceAmountCents = Math.round(Number(invoiceAmount) * 100)
    if (!selectedUser) {
      toast.error(t('Select a target user'))
      return
    }
    if (quotaAmount <= 0) {
      toast.error(t('Enter a valid credit amount'))
      return
    }
    if (invoiceAmountCents <= 0) {
      toast.error(t('Enter the actual invoiceable CNY amount received.'))
      return
    }
    if (!applicationInfo.trim()) {
      toast.error(t('Application information is required'))
      return
    }
    setSubmitting(true)
    try {
      const result = await createQuotaApplication({
        target_user_id: selectedUser.id,
        quota_amount: quotaAmount,
        invoice_amount_cents: invoiceAmountCents,
        application_info: applicationInfo.trim(),
      })
      if (result.success) {
        toast.success(t('Quota application submitted'))
        close()
        props.onCreated()
      } else {
        toast.error(result.message || t('Failed to submit quota application'))
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => (open ? props.onOpenChange(true) : close())}
      title={t('New quota application')}
      description={t('Submit a credit request for an enabled normal user.')}
      contentHeight='min(620px, calc(100vh - 14rem))'
      footer={
        <>
          <Button variant='outline' onClick={close}>
            {t('Cancel')}
          </Button>
          <Button disabled={submitting} onClick={submit}>
            {submitting ? t('Submitting...') : t('Submit application')}
          </Button>
        </>
      }
    >
      <div className='space-y-5'>
        <div className='space-y-2'>
          <Label>{t('Target user')}</Label>
          <div className='flex gap-2'>
            <Input
              value={keywordInput}
              onChange={(event) => setKeywordInput(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter') setKeyword(keywordInput.trim())
              }}
              placeholder={t('Search by username, display name, or user ID')}
            />
            <Button
              type='button'
              variant='outline'
              size='icon'
              title={t('Search')}
              onClick={() => setKeyword(keywordInput.trim())}
            >
              <Search />
            </Button>
          </div>
          {selectedUser ? (
            <div className='flex items-center justify-between rounded-md border p-3'>
              <div>
                <p className='font-medium'>{selectedUser.username}</p>
                <p className='text-muted-foreground text-xs'>
                  {selectedUser.display_name &&
                    `${selectedUser.display_name} · `}
                  ID {selectedUser.id} · {formatQuota(selectedUser.quota)}
                </p>
              </div>
              <Button
                variant='ghost'
                size='icon'
                title={t('Clear selection')}
                onClick={() => setSelectedUser(null)}
              >
                <X />
              </Button>
            </div>
          ) : null}
          {!selectedUser && keyword ? (
            <div className='max-h-44 overflow-y-auto rounded-md border'>
              {isFetching && (
                <p className='text-muted-foreground p-3 text-sm'>
                  {t('Searching...')}
                </p>
              )}
              {!isFetching && targets.length === 0 && (
                <p className='text-muted-foreground p-3 text-sm'>
                  {t('No users found')}
                </p>
              )}
              {targets.map((target) => (
                <button
                  key={target.id}
                  type='button'
                  className='hover:bg-muted flex w-full items-center justify-between border-b p-3 text-left last:border-b-0'
                  onClick={() => setSelectedUser(target)}
                >
                  <span>
                    <span className='block font-medium'>{target.username}</span>
                    <span className='text-muted-foreground text-xs'>
                      {target.display_name && `${target.display_name} · `}
                      ID {target.id}
                    </span>
                  </span>
                  <span className='text-muted-foreground text-xs'>
                    {formatQuota(target.quota)}
                  </span>
                </button>
              ))}
            </div>
          ) : null}
        </div>

        <div className='grid gap-4 sm:grid-cols-2'>
          <div className='space-y-2'>
            <Label>
              {t('Credit amount')} ({currencyLabel})
            </Label>
            <Input
              type='number'
              min={currencyMeta.kind === 'tokens' ? 1 : 0.000001}
              step={currencyMeta.kind === 'tokens' ? 1 : 0.000001}
              value={amount}
              onChange={(event) => setAmount(event.target.value)}
            />
          </div>
          <div className='space-y-2'>
            <Label>{t('Actual invoiceable amount received (CNY)')}</Label>
            <Input
              type='number'
              min={0.01}
              step={0.01}
              value={invoiceAmount}
              onChange={(event) => setInvoiceAmount(event.target.value)}
            />
          </div>
        </div>

        <div className='space-y-2'>
          <Label>{t('Application information')}</Label>
          <Textarea
            rows={6}
            maxLength={2000}
            value={applicationInfo}
            onChange={(event) => setApplicationInfo(event.target.value)}
            placeholder={t(
              'Enter company, payment, contact, and other relevant information'
            )}
          />
          <p className='text-muted-foreground text-right text-xs'>
            {applicationInfo.length}/2000
          </p>
        </div>
      </div>
    </Dialog>
  )
}

export function QuotaApplications() {
  const { t } = useTranslation()
  const client = useQueryClient()
  const user = useAuthStore((state) => state.auth.user)
  const isRoot = user?.role === ROLE.SUPER_ADMIN
  const [status, setStatus] = useState('all')
  const [searchInput, setSearchInput] = useState('')
  const [keyword, setKeyword] = useState('')
  const [page, setPage] = useState(1)
  const [createOpen, setCreateOpen] = useState(false)
  const [details, setDetails] = useState<QuotaApplication | null>(null)
  const [reviewing, setReviewing] = useState<QuotaApplication | null>(null)
  const [approved, setApproved] = useState(true)
  const [reviewComment, setReviewComment] = useState('')
  const [reviewingBusy, setReviewingBusy] = useState(false)
  const pageSize = 20
  const { data, isLoading, isError } = useQuery({
    queryKey: ['quota-applications', status, keyword, page],
    queryFn: () =>
      listQuotaApplications({
        status: status === 'all' ? '' : status,
        keyword,
        page,
        pageSize,
      }),
  })
  const items = data?.items ?? []
  const totalPages = Math.max(1, Math.ceil((data?.total ?? 0) / pageSize))
  let emptyMessage = t('No quota applications')
  if (isLoading) emptyMessage = t('Loading...')
  if (isError) emptyMessage = t('Failed to load quota applications')
  let reviewButtonLabel = approved ? t('Approve and credit') : t('Reject')
  if (reviewingBusy) reviewButtonLabel = t('Processing...')
  const refresh = () =>
    client.invalidateQueries({ queryKey: ['quota-applications'] })

  const openReview = (item: QuotaApplication, shouldApprove: boolean) => {
    setReviewing(item)
    setApproved(shouldApprove)
    setReviewComment('')
  }
  const submitReview = async () => {
    if (!reviewing) return
    if (!approved && !reviewComment.trim()) {
      toast.error(t('Rejection reason is required'))
      return
    }
    setReviewingBusy(true)
    try {
      const result = await reviewQuotaApplication(
        reviewing.id,
        approved,
        reviewComment.trim()
      )
      if (result.success) {
        toast.success(
          approved
            ? t('Quota credited successfully')
            : t('Quota application rejected')
        )
        setReviewing(null)
        refresh()
      } else {
        toast.error(result.message || t('Failed to review quota application'))
      }
    } finally {
      setReviewingBusy(false)
    }
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Quota Applications')}
      </SectionPageLayout.Title>
      {!isRoot && (
        <SectionPageLayout.Actions>
          <Button onClick={() => setCreateOpen(true)}>
            <Plus />
            {t('New application')}
          </Button>
        </SectionPageLayout.Actions>
      )}
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <div className='flex flex-col gap-2 sm:flex-row'>
            <Select
              value={status}
              onValueChange={(value) => {
                if (!value) return
                setStatus(value)
                setPage(1)
              }}
            >
              <SelectTrigger className='w-full sm:w-52'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='all'>{t('All statuses')}</SelectItem>
                <SelectItem value='pending'>{t('Pending review')}</SelectItem>
                <SelectItem value='approved'>{t('Credited')}</SelectItem>
                <SelectItem value='rejected'>{t('Rejected')}</SelectItem>
              </SelectContent>
            </Select>
            <div className='flex w-full max-w-md gap-2'>
              <Input
                value={searchInput}
                onChange={(event) => setSearchInput(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') {
                    setKeyword(searchInput.trim())
                    setPage(1)
                  }
                }}
                placeholder={t('Search applicant or target user')}
              />
              <Button
                variant='outline'
                size='icon'
                title={t('Search')}
                onClick={() => {
                  setKeyword(searchInput.trim())
                  setPage(1)
                }}
              >
                <Search />
              </Button>
            </div>
          </div>

          <div className='hidden rounded-md border md:block'>
            <Table className='min-w-[900px]'>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Application ID')}</TableHead>
                  {isRoot && <TableHead>{t('Applicant')}</TableHead>}
                  <TableHead>{t('Target user')}</TableHead>
                  <TableHead>{t('Credit amount')}</TableHead>
                  <TableHead>{t('Submitted at')}</TableHead>
                  <TableHead>{t('Status')}</TableHead>
                  <TableHead className='text-right'>{t('Actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(isLoading || isError || items.length === 0) && (
                  <TableRow>
                    <TableCell
                      colSpan={isRoot ? 7 : 6}
                      className={
                        isError
                          ? 'text-destructive h-28 text-center'
                          : 'text-muted-foreground h-28 text-center'
                      }
                    >
                      {emptyMessage}
                    </TableCell>
                  </TableRow>
                )}
                {items.map((item) => (
                  <TableRow key={item.id}>
                    <TableCell className='font-medium'>#{item.id}</TableCell>
                    {isRoot && <TableCell>{item.applicant_username}</TableCell>}
                    <TableCell>
                      <span className='block font-medium'>
                        {item.target_username}
                      </span>
                      <span className='text-muted-foreground text-xs'>
                        {item.target_display_name &&
                          `${item.target_display_name} · `}
                        ID {item.target_user_id}
                      </span>
                    </TableCell>
                    <TableCell className='font-medium'>
                      {formatQuota(item.quota_amount)}
                    </TableCell>
                    <TableCell>{formatTimestamp(item.created_at)}</TableCell>
                    <TableCell>
                      <ApplicationStatusBadge status={item.status} />
                    </TableCell>
                    <TableCell>
                      <div className='flex justify-end gap-1'>
                        <Button
                          variant='ghost'
                          size='icon'
                          title={t('View details')}
                          onClick={() => setDetails(item)}
                        >
                          <Eye />
                        </Button>
                        {isRoot && item.status === 'pending' && (
                          <>
                            <Button
                              variant='ghost'
                              size='icon'
                              title={t('Approve')}
                              onClick={() => openReview(item, true)}
                            >
                              <Check />
                            </Button>
                            <Button
                              variant='ghost'
                              size='icon'
                              title={t('Reject')}
                              onClick={() => openReview(item, false)}
                            >
                              <X />
                            </Button>
                          </>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>

          <div className='space-y-2 md:hidden'>
            {(isLoading || isError || items.length === 0) && (
              <div
                className={
                  isError
                    ? 'text-destructive rounded-md border px-4 py-10 text-center text-sm'
                    : 'text-muted-foreground rounded-md border px-4 py-10 text-center text-sm'
                }
              >
                {emptyMessage}
              </div>
            )}
            {items.map((item) => (
              <div key={item.id} className='rounded-md border p-3'>
                <div className='flex items-start justify-between gap-3'>
                  <div className='min-w-0'>
                    <p className='truncate font-medium'>
                      {item.target_username}
                    </p>
                    <p className='text-muted-foreground truncate text-xs'>
                      {item.target_display_name &&
                        `${item.target_display_name} · `}
                      ID {item.target_user_id}
                    </p>
                  </div>
                  <ApplicationStatusBadge status={item.status} />
                </div>

                <div className='mt-3 grid grid-cols-2 gap-x-3 gap-y-2 text-sm'>
                  <div>
                    <p className='text-muted-foreground text-xs'>
                      {t('Credit amount')}
                    </p>
                    <p className='font-medium'>
                      {formatQuota(item.quota_amount)}
                    </p>
                  </div>
                  <div>
                    <p className='text-muted-foreground text-xs'>
                      {t('Application ID')}
                    </p>
                    <p className='font-medium'>#{item.id}</p>
                  </div>
                  {isRoot && (
                    <div>
                      <p className='text-muted-foreground text-xs'>
                        {t('Applicant')}
                      </p>
                      <p className='truncate'>{item.applicant_username}</p>
                    </div>
                  )}
                  <div className={isRoot ? '' : 'col-span-2'}>
                    <p className='text-muted-foreground text-xs'>
                      {t('Submitted at')}
                    </p>
                    <p>{formatTimestamp(item.created_at)}</p>
                  </div>
                </div>

                <div className='mt-3 flex justify-end gap-1 border-t pt-2'>
                  <Button
                    variant='ghost'
                    size='sm'
                    onClick={() => setDetails(item)}
                  >
                    <Eye />
                    {t('View details')}
                  </Button>
                  {isRoot && item.status === 'pending' && (
                    <>
                      <Button
                        variant='ghost'
                        size='icon'
                        title={t('Approve')}
                        onClick={() => openReview(item, true)}
                      >
                        <Check />
                      </Button>
                      <Button
                        variant='ghost'
                        size='icon'
                        title={t('Reject')}
                        onClick={() => openReview(item, false)}
                      >
                        <X />
                      </Button>
                    </>
                  )}
                </div>
              </div>
            ))}
          </div>

          <div className='flex items-center justify-between text-sm'>
            <span className='text-muted-foreground'>
              {t('{{count}} applications', { count: data?.total ?? 0 })}
            </span>
            <div className='flex items-center gap-2'>
              <Button
                variant='outline'
                size='icon'
                title={t('Previous page')}
                disabled={page <= 1}
                onClick={() => setPage((value) => value - 1)}
              >
                <ChevronLeft />
              </Button>
              <span>
                {page} / {totalPages}
              </span>
              <Button
                variant='outline'
                size='icon'
                title={t('Next page')}
                disabled={page >= totalPages}
                onClick={() => setPage((value) => value + 1)}
              >
                <ChevronRight />
              </Button>
            </div>
          </div>
        </div>

        <CreateApplicationDialog
          open={createOpen}
          onOpenChange={setCreateOpen}
          onCreated={refresh}
        />
        <ApplicationDetails
          application={details}
          onClose={() => setDetails(null)}
        />
        <Dialog
          open={reviewing !== null}
          onOpenChange={(open) => !open && setReviewing(null)}
          title={
            approved
              ? t('Approve quota application')
              : t('Reject quota application')
          }
          description={
            reviewing
              ? `${reviewing.target_username} · ${formatQuota(reviewing.quota_amount)}`
              : undefined
          }
          contentHeight='auto'
          footer={
            <>
              <Button variant='outline' onClick={() => setReviewing(null)}>
                {t('Cancel')}
              </Button>
              <Button
                variant={approved ? 'default' : 'destructive'}
                disabled={reviewingBusy}
                onClick={submitReview}
              >
                {reviewButtonLabel}
              </Button>
            </>
          }
        >
          <div className='space-y-2'>
            <Label>
              {approved ? t('Review comment') : t('Rejection reason')}
            </Label>
            <Textarea
              rows={4}
              maxLength={500}
              value={reviewComment}
              onChange={(event) => setReviewComment(event.target.value)}
            />
          </div>
        </Dialog>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
