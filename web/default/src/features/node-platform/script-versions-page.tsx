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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { api } from '@/lib/api'

import {
  listScriptVersions,
  publishScriptVersion,
  submitScriptForReview,
} from './api'
import { formatUnix } from './lib/format'
import type { ScriptVersion } from './types'

type MyScript = {
  id: number
  title: string
  review_status?: string
  latest_version?: number
  published?: boolean
}

function statusBadge(status?: string): string {
  switch (status) {
    case 'approved':
      return '✅ approved'
    case 'pending':
      return '⏳ pending'
    case 'rejected':
      return '❌ rejected'
    default:
      return '📝 draft'
  }
}

export function ScriptVersionsPage() {
  const { t } = useTranslation()
  const [scripts, setScripts] = useState<MyScript[]>([])
  const [versions, setVersions] = useState<Record<number, ScriptVersion[]>>({})
  const [loading, setLoading] = useState(false)
  const [busyId, setBusyId] = useState(0)

  async function loadScripts() {
    setLoading(true)
    try {
      const res = await api.get('/api/scripts/mine')
      const list = (res.data?.data ?? res.data ?? []) as MyScript[]
      setScripts(list)
    } catch (e) {
      toast.error(String((e as Error).message))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadScripts()
  }, [])

  async function onSubmitReview(id: number) {
    setBusyId(id)
    try {
      await submitScriptForReview(id)
      toast.success(t('Submitted for review'))
      await loadScripts()
    } catch (e) {
      toast.error(String((e as Error).message))
    } finally {
      setBusyId(0)
    }
  }

  async function onPublish(id: number) {
    setBusyId(id)
    try {
      const r = (await publishScriptVersion(id)) as {
        version?: number
        signed?: boolean
      }
      toast.success(
        t('Published version {{v}} (signed: {{s}})', {
          v: r?.version ?? '?',
          s: r?.signed ? 'yes' : 'no',
        })
      )
      await loadScripts()
      await loadVersions(id)
    } catch (e) {
      toast.error(String((e as Error).message))
    } finally {
      setBusyId(0)
    }
  }

  async function loadVersions(id: number) {
    try {
      const v = await listScriptVersions(id)
      setVersions((prev) => ({ ...prev, [id]: v }))
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Script Versions')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button variant='outline' onClick={loadScripts} disabled={loading}>
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='text-muted-foreground mb-4 text-sm'>
          {t(
            'Submit a draft for review, then publish an immutable, signed version. Published versions cannot be modified — publish a new version instead.'
          )}
        </div>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>ID</TableHead>
              <TableHead>{t('Title')}</TableHead>
              <TableHead>{t('Review')}</TableHead>
              <TableHead>{t('Latest Version')}</TableHead>
              <TableHead>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {scripts.map((s) => (
              <TableRow key={s.id}>
                <TableCell>{s.id}</TableCell>
                <TableCell>{s.title}</TableCell>
                <TableCell>{statusBadge(s.review_status)}</TableCell>
                <TableCell>{s.latest_version || '-'}</TableCell>
                <TableCell className='space-x-2'>
                  <Button
                    size='sm'
                    variant='outline'
                    disabled={busyId === s.id || s.review_status === 'pending'}
                    onClick={() => onSubmitReview(s.id)}
                  >
                    {t('Submit review')}
                  </Button>
                  <Button
                    size='sm'
                    disabled={busyId === s.id || s.review_status !== 'approved'}
                    onClick={() => onPublish(s.id)}
                  >
                    {t('Publish version')}
                  </Button>
                  <Button
                    size='sm'
                    variant='ghost'
                    onClick={() => loadVersions(s.id)}
                  >
                    {t('History')}
                  </Button>
                </TableCell>
              </TableRow>
            ))}
            {scripts.length === 0 && (
              <TableRow>
                <TableCell colSpan={5} className='text-muted-foreground text-center'>
                  {loading ? t('Loading...') : t('No scripts yet')}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>

        {Object.entries(versions).map(([sid, list]) => (
          <div key={sid} className='mt-6'>
            <div className='mb-2 text-sm font-medium'>
              {t('Version history for script #{{id}}', { id: sid })}
            </div>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Version')}</TableHead>
                  <TableHead>{t('Code hash')}</TableHead>
                  <TableHead>{t('Signed')}</TableHead>
                  <TableHead>{t('Published')}</TableHead>
                  <TableHead>{t('Revoked')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {list.map((v) => (
                  <TableRow key={v.id}>
                    <TableCell>v{v.version}</TableCell>
                    <TableCell className='max-w-[220px] truncate font-mono text-xs'>
                      {v.code_sha256}
                    </TableCell>
                    <TableCell>{v.signature ? '🔏' : '—'}</TableCell>
                    <TableCell>{formatUnix(v.published_at)}</TableCell>
                    <TableCell>
                      {v.revoked_at ? `⛔ ${v.revoked_reason || ''}` : '—'}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        ))}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
