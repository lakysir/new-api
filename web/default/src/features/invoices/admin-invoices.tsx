import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, MailCheck, X } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { listInvoices, markInvoiceSent, reviewInvoice } from './api'
import type { InvoiceApplication } from './types'

const statuses = {
  pending: 'Pending review',
  approved: 'Approved, awaiting delivery',
  sent: 'Sent',
  rejected: 'Rejected',
  cancelled: 'Cancelled',
} as const

export function AdminInvoices() {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [status, setStatus] = useState('all')
  const [rejecting, setRejecting] = useState<InvoiceApplication | null>(null)
  const [reason, setReason] = useState('')
  const { data, isLoading, isError } = useQuery({
    queryKey: ['admin-invoices', status],
    queryFn: () => listInvoices(status === 'all' ? '' : status),
  })
  const applications = Array.isArray(data) ? data : []
  const refresh = () =>
    client.invalidateQueries({ queryKey: ['admin-invoices'] })
  const approve = async (id: number) => {
    const res = await reviewInvoice(id, true)
    if (res.success) {
      toast.success(t('Invoice application approved'))
      refresh()
    }
  }
  const reject = async () => {
    if (!rejecting || !reason.trim()) {
      toast.error(t('Rejection reason is required'))
      return
    }
    const res = await reviewInvoice(rejecting.id, false, reason)
    if (res.success) {
      setRejecting(null)
      setReason('')
      refresh()
    }
  }
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Invoice approval')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <Select value={status} onValueChange={setStatus}>
            <SelectTrigger className='w-60'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value='all'>{t('All statuses')}</SelectItem>
              {Object.entries(statuses).map(([value, label]) => (
                <SelectItem key={value} value={value}>
                  {t(label)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <div className='overflow-x-auto rounded-md border'>
            <table className='w-full min-w-[980px] text-sm'>
              <thead className='bg-muted/50 text-left'>
                <tr>
                  <th className='p-3'>{t('Applicant')}</th>
                  <th className='p-3'>{t('Invoice information')}</th>
                  <th className='p-3'>{t('Amount')}</th>
                  <th className='p-3'>{t('Delivery email')}</th>
                  <th className='p-3'>{t('Status')}</th>
                  <th className='p-3 text-right'>{t('Actions')}</th>
                </tr>
              </thead>
              <tbody className='divide-y'>
                {isError && (
                  <tr>
                    <td
                      colSpan={6}
                      className='text-destructive p-8 text-center'
                    >
                      {t('Failed to load invoice applications')}
                    </td>
                  </tr>
                )}
                {!isLoading && !isError && applications.length === 0 && (
                  <tr>
                    <td
                      colSpan={6}
                      className='text-muted-foreground p-8 text-center'
                    >
                      {t('No invoice applications')}
                    </td>
                  </tr>
                )}
                {applications.map((item) => (
                  <tr key={item.id}>
                    <td className='p-3'>
                      {item.username}
                      <span className='text-muted-foreground block'>
                        ID {item.user_id}
                      </span>
                    </td>
                    <td className='p-3'>
                      <span className='font-medium'>{item.title}</span>
                      <span className='text-muted-foreground block'>
                        {item.invoice_type === 'enterprise'
                          ? item.tax_number
                          : t('Personal')}
                      </span>
                      {item.remark && (
                        <span className='text-muted-foreground block'>
                          {item.remark}
                        </span>
                      )}
                    </td>
                    <td className='p-3 font-medium'>
                      CNY {(item.amount_cents / 100).toFixed(2)}
                    </td>
                    <td className='p-3'>{item.email}</td>
                    <td className='p-3'>
                      {t(statuses[item.status])}
                      {item.reject_reason && (
                        <span className='block text-destructive'>
                          {item.reject_reason}
                        </span>
                      )}
                    </td>
                    <td className='p-3'>
                      <div className='flex justify-end gap-1'>
                        {item.status === 'pending' && (
                          <>
                            <Button
                              size='icon'
                              variant='ghost'
                              title={t('Approve')}
                              onClick={() => approve(item.id)}
                            >
                              <Check />
                            </Button>
                            <Button
                              size='icon'
                              variant='ghost'
                              title={t('Reject')}
                              onClick={() => setRejecting(item)}
                            >
                              <X />
                            </Button>
                          </>
                        )}
                        {item.status === 'approved' && (
                          <Button
                            size='sm'
                            onClick={async () => {
                              const res = await markInvoiceSent(item.id)
                              if (res.success) refresh()
                            }}
                          >
                            <MailCheck />
                            {t('Mark sent')}
                          </Button>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {rejecting && (
            <div className='fixed inset-0 z-50 grid place-items-center bg-black/40 p-4'>
              <div className='w-full max-w-md rounded-md bg-background p-5 shadow-xl'>
                <h2 className='font-semibold'>
                  {t('Reject invoice application')}
                </h2>
                <p className='text-muted-foreground mt-1 text-sm'>
                  {rejecting.title} - CNY{' '}
                  {(rejecting.amount_cents / 100).toFixed(2)}
                </p>
                <Input
                  className='mt-4'
                  value={reason}
                  onChange={(e) => setReason(e.target.value)}
                  placeholder={t('Enter rejection reason')}
                  maxLength={500}
                />
                <div className='mt-4 flex justify-end gap-2'>
                  <Button variant='outline' onClick={() => setRejecting(null)}>
                    {t('Cancel')}
                  </Button>
                  <Button variant='destructive' onClick={reject}>
                    {t('Reject')}
                  </Button>
                </div>
              </div>
            </div>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
