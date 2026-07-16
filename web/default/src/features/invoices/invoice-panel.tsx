import { useQuery, useQueryClient } from '@tanstack/react-query'
import { FileText, Settings2, X } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

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
import { Textarea } from '@/components/ui/textarea'

import {
  createInvoice,
  cancelInvoice,
  getInvoiceOverview,
  listInvoiceProfiles,
} from './api'
import { InvoiceProfileDialog } from './invoice-profile-dialog'

const statusLabels = {
  pending: 'Pending review',
  approved: 'Approved, awaiting delivery',
  sent: 'Sent',
  rejected: 'Rejected',
  cancelled: 'Cancelled',
} as const
const money = (cents: number) =>
  `CNY ${(cents / 100).toLocaleString(undefined, { minimumFractionDigits: 2 })}`

export function InvoicePanel() {
  const { t } = useTranslation()
  const client = useQueryClient()
  const { data } = useQuery({
    queryKey: ['invoice-overview'],
    queryFn: getInvoiceOverview,
  })
  const { data: profiles = [] } = useQuery({
    queryKey: ['invoice-profiles'],
    queryFn: listInvoiceProfiles,
  })
  const [type, setType] = useState<'personal' | 'enterprise'>('enterprise')
  const [title, setTitle] = useState('')
  const [email, setEmail] = useState('')
  const [amount, setAmount] = useState(1000)
  const [remark, setRemark] = useState('')
  const [saving, setSaving] = useState(false)
  const [profileId, setProfileId] = useState('')
  const [profilesOpen, setProfilesOpen] = useState(false)
  useEffect(() => {
    if (!profileId && profiles.length > 0) {
      setProfileId(
        String(
          (profiles.find((profile) => profile.is_default) ?? profiles[0]).id
        )
      )
    }
  }, [profileId, profiles])
  if (!data?.enabled) return null
  const refresh = () =>
    client.invalidateQueries({ queryKey: ['invoice-overview'] })
  const submit = async () => {
    if (
      (type === 'personal' && (!title.trim() || !email.trim())) ||
      amount < 1000 ||
      (type === 'enterprise' && !profileId)
    ) {
      toast.error(t('Please complete valid invoice information.'))
      return
    }
    setSaving(true)
    try {
      const result = await createInvoice({
        invoice_type: type,
        profile_id: type === 'enterprise' ? Number(profileId) : 0,
        title,
        email,
        amount_cents: Math.round(amount * 100),
        remark,
      })
      if (result.success) {
        toast.success(t('Invoice application submitted'))
        setTitle('')
        setRemark('')
        refresh()
      }
    } finally {
      setSaving(false)
    }
  }
  return (
    <section className='bg-card rounded-md border p-4 sm:p-5'>
      <div className='mb-4 flex flex-wrap items-start justify-between gap-3'>
        <div>
          <h2 className='flex items-center gap-2 font-semibold'>
            <FileText className='size-4' />
            {t('Invoice applications')}
          </h2>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('Available invoice amount')}: {money(data.available_cents)}
          </p>
        </div>
      </div>
      <div className='grid gap-4 lg:grid-cols-2'>
        <div className='grid gap-3'>
          <div>
            <Label>{t('Invoice type')}</Label>
            <Select
              value={type}
              onValueChange={(value) => setType(value as typeof type)}
            >
              <SelectTrigger className='mt-1 w-full'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='enterprise'>{t('Enterprise')}</SelectItem>
                <SelectItem value='personal'>{t('Personal')}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          {type === 'enterprise' ? (
            <div>
              <div className='flex items-center justify-between gap-2'>
                <Label>{t('Enterprise invoice profile')}</Label>
                <Button
                  size='sm'
                  variant='ghost'
                  onClick={() => setProfilesOpen(true)}
                >
                  <Settings2 />
                  {t('Manage')}
                </Button>
              </div>
              <Select value={profileId} onValueChange={setProfileId}>
                <SelectTrigger className='mt-1 w-full'>
                  <SelectValue
                    placeholder={t('Select a saved enterprise profile')}
                  />
                </SelectTrigger>
                <SelectContent>
                  {profiles.map((profile) => (
                    <SelectItem key={profile.id} value={String(profile.id)}>
                      {profile.title} - {profile.tax_number}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {profiles.length === 0 && (
                <p className='text-muted-foreground mt-1 text-xs'>
                  {t('Save an enterprise invoice profile before applying.')}
                </p>
              )}
            </div>
          ) : (
            <>
              <div>
                <Label>{t('Invoice title')}</Label>
                <Input
                  className='mt-1'
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                  maxLength={200}
                />
              </div>
              <div>
                <Label>{t('Delivery email')}</Label>
                <Input
                  className='mt-1'
                  type='email'
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                />
              </div>
            </>
          )}
          <div>
            <Label>{t('Invoice amount (CNY)')}</Label>
            <Input
              className='mt-1'
              type='number'
              min={1000}
              step='0.01'
              value={amount}
              onChange={(e) => setAmount(Number(e.target.value))}
            />
            <p className='text-muted-foreground mt-1 text-xs'>
              {t('Minimum invoice amount is CNY 1,000.')}
            </p>
          </div>
          <div>
            <Label>{t('Notes')}</Label>
            <Textarea
              className='mt-1'
              value={remark}
              onChange={(e) => setRemark(e.target.value)}
              maxLength={500}
            />
          </div>
          <Button
            onClick={submit}
            disabled={saving || amount * 100 > data.available_cents}
          >
            {saving ? t('Submitting...') : t('Submit application')}
          </Button>
        </div>
        <div className='min-w-0'>
          <h3 className='mb-2 text-sm font-medium'>
            {t('Application history')}
          </h3>
          <div className='divide-y rounded-md border'>
            {data.applications.length === 0 && (
              <p className='text-muted-foreground p-4 text-sm'>
                {t('No invoice applications')}
              </p>
            )}
            {data.applications.map((item) => (
              <div
                key={item.id}
                className='flex items-start justify-between gap-3 p-3 text-sm'
              >
                <div className='min-w-0'>
                  <p className='font-medium'>
                    {item.title} - {money(item.amount_cents)}
                  </p>
                  <p className='text-muted-foreground'>
                    {t(statusLabels[item.status])}
                  </p>
                  {item.reject_reason && (
                    <p className='text-destructive mt-1'>
                      {t('Reason')}: {item.reject_reason}
                    </p>
                  )}
                </div>
                {(item.status === 'pending' || item.status === 'rejected') && (
                  <Button
                    size='icon'
                    variant='ghost'
                    title={t('Cancel')}
                    onClick={async () => {
                      await cancelInvoice(item.id)
                      refresh()
                    }}
                  >
                    <X />
                  </Button>
                )}
              </div>
            ))}
          </div>
        </div>
      </div>
      <InvoiceProfileDialog
        open={profilesOpen}
        onOpenChange={setProfilesOpen}
        profiles={profiles}
        onSaved={() =>
          client.invalidateQueries({ queryKey: ['invoice-profiles'] })
        }
      />
    </section>
  )
}
