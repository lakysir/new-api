import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { PublicLayout, SectionPageLayout } from '@/components/layout'
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

function CodePreviewDialog({
  script,
  onOpenChange,
}: {
  script: UserScript | null
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()

  return (
    <Dialog
      open={!!script}
      onOpenChange={onOpenChange}
      title={`${script?.title || ''} #${script?.id || ''}`}
      description={script?.description}
      contentClassName='sm:max-w-3xl'
      contentHeight='58vh'
    >
      <pre className='overflow-auto rounded-lg border bg-muted/40 p-3 font-mono text-xs whitespace-pre-wrap'>
        {script?.code_preview || ''}
        {script?.preview_truncated ? `\n\n/* ${t('Preview truncated')} */` : ''}
      </pre>
    </Dialog>
  )
}

function DescriptionDialog({
  script,
  onOpenChange,
}: {
  script: UserScript | null
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()

  return (
    <Dialog
      open={!!script}
      onOpenChange={onOpenChange}
      title={`${script?.title || ''} #${script?.id || ''}`}
      description={t('Description')}
      contentClassName='sm:max-w-2xl'
      contentHeight='360px'
    >
      <div className='text-sm leading-relaxed whitespace-pre-wrap'>
        {script?.description || t('No description available.')}
      </div>
    </Dialog>
  )
}

export function ScriptSquarePage() {
  const { t } = useTranslation()
  const [scripts, setScripts] = useState<UserScript[]>([])
  const [loading, setLoading] = useState(false)
  const [previewScript, setPreviewScript] = useState<UserScript | null>(null)
  const [descriptionScript, setDescriptionScript] = useState<UserScript | null>(
    null
  )

  async function loadSquare() {
    setLoading(true)
    try {
      const square = await unwrap<{ items: UserScript[]; total: number }>(
        api.get('/api/scripts/square', { params: { limit: 100 } })
      )
      setScripts(square.items || [])
    } finally {
      setLoading(false)
    }
  }

  async function openSquarePreview(id: number) {
    const script = await unwrap<UserScript>(api.get(`/api/scripts/square/${id}`))
    setPreviewScript(script)
  }

  useEffect(() => {
    loadSquare().catch((err) => toast.error(String(err?.message || err)))
  }, [])

  return (
    <PublicLayout>
      <div className='mx-auto flex w-full max-w-6xl flex-col gap-6'>
        <div className='flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between'>
          <div className='space-y-2'>
            <h1 className='text-2xl font-semibold tracking-normal'>
              {t('Script Square')}
            </h1>
            <p className='text-muted-foreground max-w-2xl text-sm'>
              {t('Browse published scripts shared by users.')}
            </p>
          </div>
          <Button
            type='button'
            variant='outline'
            onClick={() =>
              loadSquare().catch((err) =>
                toast.error(String(err?.message || err))
              )
            }
            disabled={loading}
          >
            {t('Refresh')}
          </Button>
        </div>

        <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-3'>
          {scripts.map((script) => (
            <article
              key={script.id}
              className='flex min-h-48 flex-col gap-4 rounded-lg border bg-card p-4 text-card-foreground shadow-xs'
            >
              <div className='min-w-0 space-y-2'>
                <div className='text-muted-foreground text-xs'>#{script.id}</div>
                <h2 className='truncate text-base font-medium'>
                  {script.title}
                </h2>
                <p className='text-muted-foreground line-clamp-3 text-sm'>
                  {script.description || '-'}
                </p>
                <Button
                  type='button'
                  variant='link'
                  className='h-auto p-0 text-sm'
                  onClick={() => setDescriptionScript(script)}
                >
                  {t('Details')}
                </Button>
              </div>
              <div className='text-muted-foreground mt-auto grid gap-1 text-xs'>
                <div>
                  {t('Created At')}: {formatTime(script.created_at)}
                </div>
                <div>
                  {t('Updated At')}: {formatTime(script.updated_at)}
                </div>
              </div>
              <Button
                type='button'
                variant='outline'
                className='w-full'
                onClick={() =>
                  openSquarePreview(script.id).catch((err) =>
                    toast.error(String(err?.message || err))
                  )
                }
              >
                {t('View Code')}
              </Button>
            </article>
          ))}
        </div>
      </div>

      <CodePreviewDialog
        script={previewScript}
        onOpenChange={(open) => {
          if (!open) setPreviewScript(null)
        }}
      />
      <DescriptionDialog
        script={descriptionScript}
        onOpenChange={(open) => {
          if (!open) setDescriptionScript(null)
        }}
      />
    </PublicLayout>
  )
}

export function MyScriptsPage() {
  const { t } = useTranslation()
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

  async function loadMine() {
    setLoading(true)
    try {
      const mine = await unwrap<UserScript[]>(api.get('/api/scripts/mine'))
      setMyScripts(mine || [])
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadMine().catch((err) => toast.error(String(err?.message || err)))
  }, [])

  async function openMinePreview(id: number) {
    const script = await unwrap<UserScript>(api.get(`/api/scripts/mine/${id}`))
    setPreviewScript({
      ...script,
      code_preview: script.draft_code || '',
      preview_truncated: false,
    })
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
    await loadMine()
  }

  async function publishScript(id: number) {
    await unwrap(api.post(`/api/scripts/mine/${id}/publish`))
    toast.success(t('Script published'))
    await loadMine()
  }

  async function deleteScript(id: number) {
    if (!window.confirm(t('Delete script #{{id}}?', { id }))) return
    await unwrap(api.delete(`/api/scripts/mine/${id}`))
    toast.success(t('Script deleted'))
    await loadMine()
  }

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('My Scripts')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          type='button'
          variant='outline'
          onClick={() =>
            loadMine().catch((err) => toast.error(String(err?.message || err)))
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
        <div className='min-h-0 overflow-auto'>
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
                        openMinePreview(script.id).catch((err) =>
                          toast.error(String(err?.message || err))
                        )
                      }
                    >
                      {t('View Code')}
                    </Button>
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
        </div>

        <CodePreviewDialog
          script={previewScript}
          onOpenChange={(open) => {
            if (!open) setPreviewScript(null)
          }}
        />

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
            <Textarea
              value={editing.description}
              placeholder={t('Description')}
              rows={10}
              className='min-h-[240px] resize-y'
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
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
