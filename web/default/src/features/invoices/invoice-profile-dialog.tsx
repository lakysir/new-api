import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import { deleteInvoiceProfile, saveInvoiceProfile } from './api'
import type { InvoiceProfile } from './types'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  profiles: InvoiceProfile[]
  onSaved: () => void
}
const emptyProfile: Partial<InvoiceProfile> = {
  title: '',
  tax_number: '',
  registered_address: '',
  registered_phone: '',
  bank_name: '',
  bank_account: '',
  email: '',
  is_default: false,
}

export function InvoiceProfileDialog(props: Props) {
  const { t } = useTranslation()
  const [editing, setEditing] = useState<Partial<InvoiceProfile> | null>(null)
  const [saving, setSaving] = useState(false)
  const update = (key: keyof InvoiceProfile, value: string | boolean) =>
    setEditing((current) => ({ ...current, [key]: value }))
  const save = async () => {
    if (
      !editing?.title?.trim() ||
      !editing.tax_number?.trim() ||
      !editing.email?.trim()
    ) {
      toast.error(t('Company name, tax number and email are required.'))
      return
    }
    setSaving(true)
    try {
      const result = await saveInvoiceProfile(editing)
      if (result.success) {
        setEditing(null)
        props.onSaved()
      }
    } finally {
      setSaving(false)
    }
  }
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='max-h-[85vh] overflow-y-auto sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>{t('Enterprise invoice profiles')}</DialogTitle>
        </DialogHeader>
        {!editing ? (
          <div className='space-y-3'>
            <Button size='sm' onClick={() => setEditing({ ...emptyProfile })}>
              <Plus />
              {t('Add enterprise profile')}
            </Button>
            <div className='divide-y rounded-md border'>
              {props.profiles.length === 0 && (
                <p className='text-muted-foreground p-4 text-sm'>
                  {t('No enterprise invoice profiles')}
                </p>
              )}
              {props.profiles.map((profile) => (
                <div
                  key={profile.id}
                  className='flex items-start justify-between gap-3 p-3'
                >
                  <div className='min-w-0 text-sm'>
                    <p className='font-medium'>
                      {profile.title}
                      {profile.is_default && (
                        <span className='text-muted-foreground ml-2'>
                          ({t('Default')})
                        </span>
                      )}
                    </p>
                    <p className='text-muted-foreground'>
                      {profile.tax_number}
                    </p>
                    <p className='text-muted-foreground'>{profile.email}</p>
                  </div>
                  <div className='flex gap-1'>
                    <Button
                      size='icon'
                      variant='ghost'
                      title={t('Edit')}
                      onClick={() => setEditing({ ...profile })}
                    >
                      <Pencil />
                    </Button>
                    <Button
                      size='icon'
                      variant='ghost'
                      title={t('Delete')}
                      onClick={async () => {
                        const result = await deleteInvoiceProfile(profile.id)
                        if (result.success) props.onSaved()
                      }}
                    >
                      <Trash2 />
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          </div>
        ) : (
          <div className='grid gap-3 sm:grid-cols-2'>
            <div className='sm:col-span-2'>
              <Label>{t('Company name')}</Label>
              <Input
                className='mt-1'
                value={editing.title ?? ''}
                onChange={(e) => update('title', e.target.value)}
              />
            </div>
            <div className='sm:col-span-2'>
              <Label>{t('Tax identification number')}</Label>
              <Input
                className='mt-1'
                value={editing.tax_number ?? ''}
                onChange={(e) => update('tax_number', e.target.value)}
              />
            </div>
            <div>
              <Label>{t('Registered address')}</Label>
              <Input
                className='mt-1'
                value={editing.registered_address ?? ''}
                onChange={(e) => update('registered_address', e.target.value)}
              />
            </div>
            <div>
              <Label>{t('Registered phone')}</Label>
              <Input
                className='mt-1'
                value={editing.registered_phone ?? ''}
                onChange={(e) => update('registered_phone', e.target.value)}
              />
            </div>
            <div>
              <Label>{t('Bank name')}</Label>
              <Input
                className='mt-1'
                value={editing.bank_name ?? ''}
                onChange={(e) => update('bank_name', e.target.value)}
              />
            </div>
            <div>
              <Label>{t('Bank account')}</Label>
              <Input
                className='mt-1'
                value={editing.bank_account ?? ''}
                onChange={(e) => update('bank_account', e.target.value)}
              />
            </div>
            <div className='sm:col-span-2'>
              <Label>{t('Delivery email')}</Label>
              <Input
                className='mt-1'
                type='email'
                value={editing.email ?? ''}
                onChange={(e) => update('email', e.target.value)}
              />
            </div>
            <label className='flex items-center gap-2 sm:col-span-2'>
              <Checkbox
                checked={editing.is_default === true}
                onCheckedChange={(checked) =>
                  update('is_default', checked === true)
                }
              />
              {t('Set as default profile')}
            </label>
            <div className='flex justify-end gap-2 sm:col-span-2'>
              <Button variant='outline' onClick={() => setEditing(null)}>
                {t('Cancel')}
              </Button>
              <Button onClick={save} disabled={saving}>
                {saving ? t('Saving...') : t('Save')}
              </Button>
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
