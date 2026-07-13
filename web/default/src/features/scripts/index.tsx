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
import {
  listScriptVersions,
  publishScriptVersion,
  submitScriptForReview,
} from '@/features/node-platform/api'
import { formatUnix } from '@/features/node-platform/lib/format'
import type { ScriptVersion } from '@/features/node-platform/types'
import { api } from '@/lib/api'

type UserScript = {
  id: number
  user_id: number
  title: string
  description: string
  script_params?: string
  draft_code?: string
  published?: boolean
  published_at?: number
  created_at: number
  updated_at: number
  code_preview?: string
  preview_truncated?: boolean
  review_status?: 'draft' | 'pending' | 'approved' | 'rejected' | 'publishing'
  review_note?: string
  latest_version?: number
  has_unpublished_changes: boolean
}

const emptyForm = {
  id: 0,
  title: '',
  description: '',
  script_params: JSON.stringify(
    {
      prompt: 'a dog',
      referenceImageUrls: [
        'https://oss.aimh8.com/international/2026/07/05/233ac22e11714a66abf40de604f88fb3.jpg',
      ],
      model: 'NARWHAL',
      resolution: '2K',
      aspectRatio: '1:1',
      timeoutMs: 120000,
    },
    null,
    2
  ),
  draft_code: '',
}

function formatTime(value?: number) {
  if (!value) return '-'
  return new Date(value * 1000).toLocaleString()
}

function reviewStatusKey(status: UserScript['review_status']) {
  switch (status) {
    case 'pending':
      return 'Submitted for review'
    case 'approved':
      return 'Approved'
    case 'rejected':
      return 'Rejected'
    case 'publishing':
      return 'Publishing...'
    default:
      return 'Draft'
  }
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
      {script?.script_params ? (
        <div className='mb-3 rounded-lg border bg-muted/30 p-3'>
          <div className='text-muted-foreground mb-2 text-xs font-medium'>
            {t('Script Params')}
          </div>
          <pre className='overflow-auto font-mono text-xs whitespace-pre-wrap'>
            {script.script_params}
          </pre>
        </div>
      ) : null}
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
  const [busyId, setBusyId] = useState(0)
  const [historyScript, setHistoryScript] = useState<UserScript | null>(null)
  const [versions, setVersions] = useState<ScriptVersion[]>([])

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
      script_params: script.script_params || '',
      draft_code: script.draft_code || '',
    })
    setEditorOpen(true)
  }

  async function saveDraft() {
    if (editing.script_params.trim()) {
      const parsed = JSON.parse(editing.script_params)
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        throw new Error(t('Script Params must be a JSON object.'))
      }
    }
    const payload = {
      title: editing.title,
      description: editing.description,
      script_params: editing.script_params,
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

  async function deleteScript(id: number) {
    if (!window.confirm(t('Delete script #{{id}}?', { id }))) return
    await unwrap(api.delete(`/api/scripts/mine/${id}`))
    toast.success(t('Script deleted'))
    await loadMine()
  }

  async function submitReview(id: number) {
    setBusyId(id)
    try {
      await submitScriptForReview(id)
      toast.success(t('Submitted for review'))
      await loadMine()
    } finally {
      setBusyId(0)
    }
  }

  async function publishVersion(id: number) {
    setBusyId(id)
    try {
      const result = (await publishScriptVersion(id)) as { version?: number }
      toast.success(t('Published version {{v}}', { v: result?.version ?? '?' }))
      await loadMine()
    } finally {
      setBusyId(0)
    }
  }

  async function openHistory(script: UserScript) {
    setBusyId(script.id)
    try {
      setVersions(await listScriptVersions(script.id))
      setHistoryScript(script)
    } finally {
      setBusyId(0)
    }
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
                <TableHead>{t('Review')}</TableHead>
                <TableHead>{t('Latest Version')}</TableHead>
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
                    <div className='space-y-1'>
                      <div>{t(reviewStatusKey(script.review_status))}</div>
                      {script.review_note ? (
                        <div className='text-muted-foreground max-w-48 truncate text-xs'>
                          {script.review_note}
                        </div>
                      ) : null}
                    </div>
                  </TableCell>
                  <TableCell>
                    {script.latest_version ? `v${script.latest_version}` : '-'}
                    {script.published && script.has_unpublished_changes ? (
                      <div className='text-muted-foreground text-xs'>
                        {t('Unpublished changes')}
                      </div>
                    ) : null}
                  </TableCell>
                  <TableCell>{formatTime(script.updated_at)}</TableCell>
                  <TableCell>
                    <div className='flex min-w-80 flex-wrap justify-end gap-2'>
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
                    {script.review_status === 'approved' ? (
                      <Button
                        type='button'
                        size='sm'
                        disabled={busyId === script.id}
                        onClick={() =>
                          publishVersion(script.id).catch((err) =>
                            toast.error(String(err?.message || err))
                          )
                        }
                      >
                        {t('Publish version')}
                      </Button>
                    ) : script.review_status !== 'pending' &&
                      script.review_status !== 'publishing' ? (
                      <Button
                        type='button'
                        variant='secondary'
                        size='sm'
                        disabled={
                          busyId === script.id || !script.has_unpublished_changes
                        }
                        onClick={() =>
                          submitReview(script.id).catch((err) =>
                            toast.error(String(err?.message || err))
                          )
                        }
                      >
                        {t('Submit review')}
                      </Button>
                    ) : null}
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      disabled={busyId === script.id || !script.latest_version}
                      onClick={() =>
                        openHistory(script).catch((err) =>
                          toast.error(String(err?.message || err))
                        )
                      }
                    >
                      {t('History')}
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
                    </div>
                  </TableCell>
                </TableRow>
              ))}
              {myScripts.length === 0 ? (
                <TableRow>
                  <TableCell
                    colSpan={7}
                    className='text-muted-foreground py-10 text-center'
                  >
                    {loading ? t('Loading...') : t('No scripts yet')}
                  </TableCell>
                </TableRow>
              ) : null}
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
          open={!!historyScript}
          onOpenChange={(open) => {
            if (!open) {
              setHistoryScript(null)
              setVersions([])
            }
          }}
          title={
            historyScript
              ? t('Version history for script #{{id}}', { id: historyScript.id })
              : ''
          }
          description={historyScript?.title}
          contentClassName='sm:max-w-4xl'
          contentHeight='56vh'
        >
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
              {versions.map((version) => (
                <TableRow key={version.id}>
                  <TableCell>v{version.version}</TableCell>
                  <TableCell className='max-w-64 truncate font-mono text-xs'>
                    {version.code_sha256}
                  </TableCell>
                  <TableCell>{version.signature ? t('Yes') : t('No')}</TableCell>
                  <TableCell>{formatUnix(version.published_at)}</TableCell>
                  <TableCell>
                    {version.revoked_at
                      ? version.revoked_reason || t('Revoked')
                      : '-'}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Dialog>

        <Dialog
          open={editorOpen}
          onOpenChange={setEditorOpen}
          title={editorTitle}
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
              value={editing.script_params}
              placeholder={t('Script Params')}
              rows={8}
              className='min-h-[180px] resize-y font-mono text-xs'
              onChange={(event) =>
                setEditing((prev) => ({
                  ...prev,
                  script_params: event.target.value,
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
