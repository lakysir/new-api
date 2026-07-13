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

import { Dialog } from '@/components/dialog'
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
  author_username: string
  title: string
  description: string
  script_params?: string
  draft_code: string
  review_status: string
}

export function ScriptReviewConsolePage() {
  const { t } = useTranslation()
  const [pending, setPending] = useState<PendingScript[]>([])
  const [loading, setLoading] = useState(false)
  const [notes, setNotes] = useState<Record<number, string>>({})
  const [preview, setPreview] = useState<PendingScript | null>(null)

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
    if (!approve && !notes[id]?.trim()) {
      toast.error(`${t('Reason')}: ${t('Required')}`)
      return
    }
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
              <TableHead>{t('View Code')}</TableHead>
              <TableHead>{t('Note')}</TableHead>
              <TableHead>{t('Decision')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {pending.map((s) => (
              <TableRow key={s.id}>
                <TableCell>{s.id}</TableCell>
                <TableCell>{s.title}</TableCell>
                <TableCell>{s.author_username || `#${s.user_id}`}</TableCell>
                <TableCell>
                  <Button size='sm' variant='outline' onClick={() => setPreview(s)}>
                    {t('View Code')}
                  </Button>
                </TableCell>
                <TableCell>
                  <Input
                    className='h-8 w-48'
                    placeholder={`${t('Reason')} (${t('Required')})`}
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
                    disabled={!notes[s.id]?.trim()}
                    onClick={() => decide(s.id, false)}
                  >
                    {t('Reject')}
                  </Button>
                </TableCell>
              </TableRow>
            ))}
            {pending.length === 0 && (
              <TableRow>
                <TableCell colSpan={6} className='text-muted-foreground text-center'>
                  {loading ? t('Loading...') : t('No pending scripts')}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>

        <Dialog
          open={!!preview}
          onOpenChange={(open) => {
            if (!open) setPreview(null)
          }}
          title={preview ? `${preview.title} #${preview.id}` : ''}
          description={preview?.description}
          contentClassName='sm:max-w-4xl'
          contentHeight='62vh'
        >
          {preview?.script_params ? (
            <div className='mb-3 rounded-lg border bg-muted/30 p-3'>
              <div className='text-muted-foreground mb-2 text-xs font-medium'>
                {t('Script Params')}
              </div>
              <pre className='overflow-auto font-mono text-xs whitespace-pre-wrap'>
                {preview.script_params}
              </pre>
            </div>
          ) : null}
          <pre className='overflow-auto rounded-lg border bg-muted/40 p-3 font-mono text-xs whitespace-pre-wrap'>
            {preview?.draft_code || ''}
          </pre>
        </Dialog>

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
