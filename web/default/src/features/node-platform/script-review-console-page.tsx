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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import { listPendingScripts, reviewScript, revokeScriptVersion } from './api'

type PendingScript = {
  id: number
  user_id: number
  title: string
  description: string
  review_status: string
}

export function ScriptReviewConsolePage() {
  const { t } = useTranslation()
  const [pending, setPending] = useState<PendingScript[]>([])
  const [loading, setLoading] = useState(false)
  const [notes, setNotes] = useState<Record<number, string>>({})

  // Revoke form state.
  const [revScript, setRevScript] = useState('')
  const [revVersion, setRevVersion] = useState('')
  const [revReason, setRevReason] = useState('')

  async function load() {
    setLoading(true)
    try {
      setPending(await listPendingScripts())
    } catch (e) {
      toast.error(String((e as Error).message))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [])

  async function decide(id: number, approve: boolean) {
    try {
      await reviewScript(id, approve, notes[id] || '')
      toast.success(approve ? t('Approved') : t('Rejected'))
      await load()
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  async function onRevoke() {
    const sid = Number(revScript)
    const ver = Number(revVersion)
    if (!sid || !ver) {
      toast.error(t('Script id and version required'))
      return
    }
    try {
      await revokeScriptVersion(sid, ver, revReason || 'operator revoke', 'normal')
      toast.success(t('Version revoked'))
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Script Review')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button variant='outline' onClick={load} disabled={loading}>
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='mb-2 text-sm font-medium'>{t('Pending review')}</div>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>ID</TableHead>
              <TableHead>{t('Title')}</TableHead>
              <TableHead>{t('Author')}</TableHead>
              <TableHead>{t('Note')}</TableHead>
              <TableHead>{t('Decision')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {pending.map((s) => (
              <TableRow key={s.id}>
                <TableCell>{s.id}</TableCell>
                <TableCell>{s.title}</TableCell>
                <TableCell>#{s.user_id}</TableCell>
                <TableCell>
                  <Input
                    className='h-8 w-48'
                    placeholder={t('Optional note')}
                    value={notes[s.id] || ''}
                    onChange={(e) =>
                      setNotes((p) => ({ ...p, [s.id]: e.target.value }))
                    }
                  />
                </TableCell>
                <TableCell className='space-x-2'>
                  <Button size='sm' onClick={() => decide(s.id, true)}>
                    {t('Approve')}
                  </Button>
                  <Button
                    size='sm'
                    variant='destructive'
                    onClick={() => decide(s.id, false)}
                  >
                    {t('Reject')}
                  </Button>
                </TableCell>
              </TableRow>
            ))}
            {pending.length === 0 && (
              <TableRow>
                <TableCell colSpan={5} className='text-muted-foreground text-center'>
                  {loading ? t('Loading...') : t('No pending scripts')}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>

        <div className='mt-6 rounded-lg border p-4'>
          <div className='mb-2 text-sm font-medium'>{t('Revoke a published version')}</div>
          <div className='flex flex-wrap items-center gap-2'>
            <Input
              className='w-32'
              placeholder={t('Script id')}
              value={revScript}
              onChange={(e) => setRevScript(e.target.value)}
            />
            <Input
              className='w-28'
              placeholder={t('Version')}
              value={revVersion}
              onChange={(e) => setRevVersion(e.target.value)}
            />
            <Input
              className='w-64'
              placeholder={t('Reason')}
              value={revReason}
              onChange={(e) => setRevReason(e.target.value)}
            />
            <Button variant='destructive' onClick={onRevoke}>
              {t('Revoke version')}
            </Button>
          </div>
          <div className='text-muted-foreground mt-2 text-xs'>
            {t(
              'Revoking stops new tasks and suspends all node capabilities bound to that version. Frozen code and signature are never modified.'
            )}
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
