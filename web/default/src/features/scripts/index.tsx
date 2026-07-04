import { useEffect, useMemo, useState } from 'react'
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { api } from '@/lib/api'

type UserScript = {
  id: number
  user_id: number
  title: string
  description: string
  draft_code?: string
  published?: boolean
  published_at?: number
  created_at: number
  updated_at: number
  code_preview?: string
  preview_truncated?: boolean
}

const emptyForm = {
  id: 0,
  title: '',
  description: '',
  draft_code: '',
}

function formatTime(value?: number) {
  if (!value) return '-'
  return new Date(value * 1000).toLocaleString()
}

async function unwrap<T>(
  request: Promise<{ data: { success: boolean; data: T; message?: string } }>
) {
  const res = await request
  if (!res.data?.success) throw new Error(res.data?.message || 'Request failed')
  return res.data.data
}

export function ScriptsPage() {
  const { t } = useTranslation()
  const [squareScripts, setSquareScripts] = useState<UserScript[]>([])
  const [myScripts, setMyScripts] = useState<UserScript[]>([])
  const [loading, setLoading] = useState(false)
  const [previewScript, setPreviewScript] = useState<UserScript | null>(null)
  const [editing, setEditing] = useState(emptyForm)
  const [editorOpen, setEditorOpen] = useState(false)

  const editorTitle = useMemo(
    () =>
      editing.id
        ? t('Edit Script #{{id}}', { id: editing.id })
        : t('Create Script'),
    [editing.id, t]
  )

  async function loadAll() {
    setLoading(true)
    try {
      const [square, mine] = await Promise.all([
        unwrap<{ items: UserScript[]; total: number }>(
          api.get('/api/scripts/square', { params: { limit: 100 } })
        ),
        unwrap<UserScript[]>(api.get('/api/scripts/mine')),
      ])
      setSquareScripts(square.items || [])
      setMyScripts(mine || [])
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadAll().catch((err) => toast.error(String(err?.message || err)))
  }, [])

  async function openSquarePreview(id: number) {
    const script = await unwrap<UserScript>(api.get(`/api/scripts/square/${id}`))
    setPreviewScript(script)
  }

  async function openMineEditor(id: number) {
    const script = await unwrap<UserScript>(api.get(`/api/scripts/mine/${id}`))
    setEditing({
      id: script.id,
      title: script.title || '',
      description: script.description || '',
      draft_code: script.draft_code || '',
    })
    setEditorOpen(true)
  }

  async function saveDraft() {
    const payload = {
      title: editing.title,
      description: editing.description,
      code: editing.draft_code,
    }
    if (editing.id) {
      await unwrap(api.put(`/api/scripts/mine/${editing.id}`, payload))
    } else {
      await unwrap(api.post('/api/scripts/mine', payload))
    }
    toast.success(t('Draft saved'))
    setEditorOpen(false)
    setEditing(emptyForm)
    await loadAll()
  }

  async function publishScript(id: number) {
    await unwrap(api.post(`/api/scripts/mine/${id}/publish`))
    toast.success(t('Script published'))
    await loadAll()
  }

  async function deleteScript(id: number) {
    if (!window.confirm(t('Delete script #{{id}}?', { id }))) return
    await unwrap(api.delete(`/api/scripts/mine/${id}`))
    toast.success(t('Script deleted'))
    await loadAll()
  }

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('Script Square')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          type='button'
          variant='outline'
          onClick={() =>
            loadAll().catch((err) => toast.error(String(err?.message || err)))
          }
          disabled={loading}
        >
          {t('Refresh')}
        </Button>
        <Button
          type='button'
          onClick={() => {
            setEditing(emptyForm)
            setEditorOpen(true)
          }}
        >
          {t('Create Script')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <Tabs defaultValue='square' className='h-full'>
          <TabsList>
            <TabsTrigger value='square'>{t('Square')}</TabsTrigger>
            <TabsTrigger value='mine'>{t('My Scripts')}</TabsTrigger>
          </TabsList>

          <TabsContent value='square' className='min-h-0 overflow-auto'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>ID</TableHead>
                  <TableHead>{t('Title')}</TableHead>
                  <TableHead>{t('Description')}</TableHead>
                  <TableHead>{t('Created At')}</TableHead>
                  <TableHead>{t('Updated At')}</TableHead>
                  <TableHead className='text-right'>{t('Actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {squareScripts.map((script) => (
                  <TableRow key={script.id}>
                    <TableCell>{script.id}</TableCell>
                    <TableCell className='max-w-48 truncate'>
                      {script.title}
                    </TableCell>
                    <TableCell className='max-w-96 truncate'>
                      {script.description}
                    </TableCell>
                    <TableCell>{formatTime(script.created_at)}</TableCell>
                    <TableCell>{formatTime(script.updated_at)}</TableCell>
                    <TableCell className='text-right'>
                      <Button
                        type='button'
                        variant='outline'
                        size='sm'
                        onClick={() =>
                          openSquarePreview(script.id).catch((err) =>
                            toast.error(String(err?.message || err))
                          )
                        }
                      >
                        {t('View Code')}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TabsContent>

          <TabsContent value='mine' className='min-h-0 overflow-auto'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>ID</TableHead>
                  <TableHead>{t('Title')}</TableHead>
                  <TableHead>{t('Description')}</TableHead>
                  <TableHead>{t('Status')}</TableHead>
                  <TableHead>{t('Created At')}</TableHead>
                  <TableHead>{t('Updated At')}</TableHead>
                  <TableHead className='text-right'>{t('Actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {myScripts.map((script) => (
                  <TableRow key={script.id}>
                    <TableCell>{script.id}</TableCell>
                    <TableCell className='max-w-48 truncate'>
                      {script.title}
                    </TableCell>
                    <TableCell className='max-w-80 truncate'>
                      {script.description}
                    </TableCell>
                    <TableCell>
                      {script.published ? t('Published') : t('Draft')}
                    </TableCell>
                    <TableCell>{formatTime(script.created_at)}</TableCell>
                    <TableCell>{formatTime(script.updated_at)}</TableCell>
                    <TableCell className='space-x-2 text-right'>
                      <Button
                        type='button'
                        variant='outline'
                        size='sm'
                        onClick={() =>
                          openMineEditor(script.id).catch((err) =>
                            toast.error(String(err?.message || err))
                          )
                        }
                      >
                        {t('Edit')}
                      </Button>
                      <Button
                        type='button'
                        variant='secondary'
                        size='sm'
                        onClick={() =>
                          publishScript(script.id).catch((err) =>
                            toast.error(String(err?.message || err))
                          )
                        }
                      >
                        {t('Publish')}
                      </Button>
                      <Button
                        type='button'
                        variant='destructive'
                        size='sm'
                        onClick={() =>
                          deleteScript(script.id).catch((err) =>
                            toast.error(String(err?.message || err))
                          )
                        }
                      >
                        {t('Delete')}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TabsContent>
        </Tabs>
      </SectionPageLayout.Content>

      <Dialog
        open={!!previewScript}
        onOpenChange={(open) => {
          if (!open) setPreviewScript(null)
        }}
        title={`${previewScript?.title || ''} #${previewScript?.id || ''}`}
        description={previewScript?.description}
        contentClassName='sm:max-w-3xl'
        contentHeight='58vh'
      >
        <pre className='overflow-auto rounded-lg border bg-muted/40 p-3 font-mono text-xs whitespace-pre-wrap'>
          {previewScript?.code_preview || ''}
          {previewScript?.preview_truncated ? '\n\n/* preview truncated */' : ''}
        </pre>
      </Dialog>

      <Dialog
        open={editorOpen}
        onOpenChange={setEditorOpen}
        title={editorTitle}
        description={t(
          'Saving only updates the draft; publishing copies the current draft to the square version.'
        )}
        contentClassName='sm:max-w-4xl'
        contentHeight='520px'
        footer={
          <>
            <Button
              type='button'
              variant='outline'
              onClick={() => setEditorOpen(false)}
            >
              {t('Cancel')}
            </Button>
            <Button
              type='button'
              onClick={() =>
                saveDraft().catch((err) =>
                  toast.error(String(err?.message || err))
                )
              }
            >
              {t('Save Draft')}
            </Button>
          </>
        }
      >
        <div className='grid gap-3'>
          <Input
            value={editing.title}
            placeholder={t('Title')}
            onChange={(event) =>
              setEditing((prev) => ({ ...prev, title: event.target.value }))
            }
          />
          <Input
            value={editing.description}
            placeholder={t('Description')}
            onChange={(event) =>
              setEditing((prev) => ({
                ...prev,
                description: event.target.value,
              }))
            }
          />
          <Textarea
            value={editing.draft_code}
            placeholder={t('JavaScript code')}
            className='min-h-[360px] font-mono text-xs'
            onChange={(event) =>
              setEditing((prev) => ({
                ...prev,
                draft_code: event.target.value,
              }))
            }
          />
        </div>
      </Dialog>
    </SectionPageLayout>
  )
}
