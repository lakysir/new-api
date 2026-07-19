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
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import {
  getScriptVersionCode,
  listCategories,
  listScriptVersions,
  publishScriptVersion,
  submitScriptForReview,
} from '@/features/node-platform/api'
import { EarningsSummary } from '@/features/node-platform/earnings-summary'
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
  /** Max simultaneous executions this script supports on a single node. Default 1. */
  concurrency?: number
  review_status?:
    | 'draft'
    | 'pending'
    | 'approved'
    | 'rejected'
    | 'publishing'
    | 'published'
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
  concurrency: 1,
}

// EXAMPLE_SCRIPT is the complete reference implementation shown in the editor
// to help authors understand parameter passing and the required return shape.
const EXAMPLE_PARAMS = JSON.stringify(
  {
    prompt: '一个男孩和一只狗。',
    referenceImageUrls: [
      'https://oss.aimh8.com/international/2026/07/05/233ac22e11714a66abf40de604f88fb3.jpg',
    ],
    model: 'NARWHAL',
    resolution: '1K',
    aspectRatio: '16:9',
    timeoutMs: 120000,
  },
  null,
  2
)

const EXAMPLE_CODE = `async function runGeneratedTest(config) {
  /*
  runGeneratedTest(config) parameter contract:
  The JSON in the Script Params box is passed as config.
  - config.prompt: 图片生成提示词，不能为空。
  - config.referenceImageUrls: 可选，参考图 URL 数组。
  - config.model: 可选，默认 "NARWHAL"。
  - config.resolution: 可选，"1K"/"2K"/"4K"，默认 "1K"。
  - config.aspectRatio: 可选，"16:9"/"1:1"/"9:16"，默认 "16:9"。
  - config.timeoutMs: 可选，超时毫秒，默认 120000。
  */

  // ── 脚本运行于目标站点 MAIN world（浏览器扩展注入）
  // ── 可访问 document / location / fetch（with credentials）

  // 1. 获取 session token（浏览器已登录，凭 cookie 直接读取）
  var token = null;
  for (var path of ["/api/auth/session", "/fx/api/auth/session"]) {
    try {
      var r = await fetch(path, { credentials: "include", cache: "no-store" });
      if (!r.ok) continue;
      var j = await r.json();
      token = j && (j.accessToken || j.access_token || j.token);
      if (token) break;
    } catch (_) {}
  }
  if (!token) {
    return {
      status: "failed",
      stage: "session",
      balance: 0,
      error: "无法取得 access_token，请确认已登录目标站点"
    };
  }

  // 2. 读取账户余额（供平台更新节点 remaining_quota）
  var balance = 0;
  try {
    var cr = await fetch("https://aisandbox-pa.googleapis.com/v1/credits", {
      headers: { Authorization: "Bearer " + token }
    });
    if (cr.ok) {
      var cd = await cr.json();
      balance = parseInt(cd && cd.credits || 0, 10) || 0;
    }
  } catch (_) {}

  // 3. 调用生图接口（示意，替换为真实接口）
  var prompt = String(config.prompt || "");
  var model  = String(config.model  || "NARWHAL");

  // ... 此处填入真实请求逻辑 ...

  // ── 返回值规范 ────────────────────────────────────────────────────────────
  // status   (必填) "success" | "failed" | "error"
  // balance  (必填) 账户当前剩余额度（整数）；平台读此字段更新节点 remaining_quota
  // 其余字段自定义，可放图片 URL、种子、模型名等任意可序列化数据
  return {
    status: "success",
    type: "flow_image",
    balance: balance,           // ← 平台读此字段，务必填写
    prompt: prompt,
    model: model,
    image_url: "",              // 替换为真实生图结果 URL
    page: {
      title: typeof document !== "undefined" ? document.title : null,
      url:   typeof location  !== "undefined" ? location.href  : null
    }
  };
}
`

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
    case 'published':
      return 'Published'
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
        <div className='bg-muted/30 mb-3 rounded-lg border p-3'>
          <div className='text-muted-foreground mb-2 text-xs font-medium'>
            {t('Script Params')}
          </div>
          <pre className='overflow-auto font-mono text-xs whitespace-pre-wrap'>
            {script.script_params}
          </pre>
        </div>
      ) : null}
      <pre className='bg-muted/40 overflow-auto rounded-lg border p-3 font-mono text-xs whitespace-pre-wrap'>
        {script?.code_preview || ''}
        {script?.preview_truncated ? `\n\n/* ${t('Preview truncated')} */` : ''}
      </pre>
    </Dialog>
  )
}

// A single code column: label + scrollable monospace body (or a status note).
function CodePane({
  code,
  loading,
  error,
  emptyText,
}: {
  code: string
  loading?: boolean
  error?: string
  emptyText: string
}) {
  const { t } = useTranslation()
  return (
    <pre className='bg-muted/40 h-full min-h-[280px] overflow-auto rounded-lg border p-3 font-mono text-xs whitespace-pre-wrap'>
      {loading
        ? t('Loading...')
        : error
          ? `/* ${error} */`
          : code || `/* ${emptyText} */`}
    </pre>
  )
}

// MyScriptCodeDialog lets an author compare the current draft against the last
// published version. The draft is passed in; the published code is fetched
// lazily from the script's latest_version when the dialog opens. Three views:
// Draft, Published, and side-by-side Compare.
function MyScriptCodeDialog({
  script,
  onOpenChange,
}: {
  script: UserScript | null
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const draftCode = script?.draft_code || ''
  const publishedVersion = script?.latest_version || 0
  const hasPublished = !!script?.published && publishedVersion > 0

  const [publishedCode, setPublishedCode] = useState('')
  const [loadingPublished, setLoadingPublished] = useState(false)
  const [publishedError, setPublishedError] = useState('')

  // Fetch the last published version's code when a script that has one is
  // opened. Keyed on script id + version so switching scripts refetches.
  useEffect(() => {
    if (!script || !hasPublished) {
      setPublishedCode('')
      setPublishedError('')
      return
    }
    let cancelled = false
    setLoadingPublished(true)
    setPublishedError('')
    getScriptVersionCode(script.id, publishedVersion)
      .then((res) => {
        if (!cancelled) setPublishedCode(res.code || '')
      })
      .catch((err) => {
        if (!cancelled)
          setPublishedError(String((err as Error)?.message || err))
      })
      .finally(() => {
        if (!cancelled) setLoadingPublished(false)
      })
    return () => {
      cancelled = true
    }
  }, [script, hasPublished, publishedVersion])

  const publishedLabel = hasPublished
    ? t('Published v{{v}}', { v: publishedVersion })
    : t('Published')

  return (
    <Dialog
      open={!!script}
      onOpenChange={onOpenChange}
      title={`${script?.title || ''} #${script?.id || ''}`}
      description={
        script?.has_unpublished_changes
          ? t('Draft has unpublished changes.')
          : script?.description
      }
      contentClassName='sm:max-w-4xl'
      contentHeight='58vh'
    >
      <Tabs defaultValue='draft' className='h-full'>
        <TabsList>
          <TabsTrigger value='draft'>{t('Draft')}</TabsTrigger>
          {hasPublished ? (
            <>
              <TabsTrigger value='published'>{publishedLabel}</TabsTrigger>
              <TabsTrigger value='compare'>{t('Compare')}</TabsTrigger>
            </>
          ) : null}
        </TabsList>

        <TabsContent value='draft'>
          <CodePane code={draftCode} emptyText={t('No draft code.')} />
        </TabsContent>

        {hasPublished ? (
          <>
            <TabsContent value='published'>
              <CodePane
                code={publishedCode}
                loading={loadingPublished}
                error={publishedError}
                emptyText={t('No published code.')}
              />
            </TabsContent>

            <TabsContent value='compare'>
              <div className='grid h-full gap-3 lg:grid-cols-2'>
                <div className='flex min-h-0 flex-col gap-1'>
                  <span className='text-muted-foreground text-xs font-medium'>
                    {t('Draft')}
                  </span>
                  <CodePane code={draftCode} emptyText={t('No draft code.')} />
                </div>
                <div className='flex min-h-0 flex-col gap-1'>
                  <span className='text-muted-foreground text-xs font-medium'>
                    {publishedLabel}
                  </span>
                  <CodePane
                    code={publishedCode}
                    loading={loadingPublished}
                    error={publishedError}
                    emptyText={t('No published code.')}
                  />
                </div>
              </div>
            </TabsContent>
          </>
        ) : null}
      </Tabs>
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
    const script = await unwrap<UserScript>(
      api.get(`/api/scripts/square/${id}`)
    )
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
              className='bg-card text-card-foreground flex min-h-48 flex-col gap-4 rounded-lg border p-4 shadow-xs'
            >
              <div className='min-w-0 space-y-2'>
                <div className='text-muted-foreground text-xs'>
                  #{script.id}
                </div>
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
  // Submit-for-review dialog: author picks a target-site category and proposes
  // their share (% of the provider execution price).
  const [submitTarget, setSubmitTarget] = useState<number>(0)
  const [submitCategory, setSubmitCategory] = useState<string>('')
  const [submitShare, setSubmitShare] = useState<string>('3')
  const [categories, setCategories] = useState<
    { id: number; name: string; site: string }[]
  >([])
  // Controls whether the reference example panel is expanded in the editor.
  const [showExample, setShowExample] = useState(false)

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

  // Load the full record (including draft_code) so the code dialog can show the
  // draft and fetch the last published version for comparison.
  async function openMinePreview(id: number) {
    const script = await unwrap<UserScript>(api.get(`/api/scripts/mine/${id}`))
    setPreviewScript(script)
  }

  async function openMineEditor(id: number) {
    const script = await unwrap<UserScript>(api.get(`/api/scripts/mine/${id}`))
    setEditing({
      id: script.id,
      title: script.title || '',
      description: script.description || '',
      script_params: script.script_params || '',
      draft_code: script.draft_code || '',
      concurrency: script.concurrency ?? 1,
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
      concurrency: editing.concurrency ?? 1,
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

  // Open the submit dialog (collect category + author share before submitting).
  // Category and share % are persisted to localStorage so they survive reopens.
  async function openSubmitDialog(id: number) {
    setSubmitTarget(id)
    setSubmitCategory(localStorage.getItem('script_submit_category') ?? '')
    setSubmitShare(localStorage.getItem('script_submit_share') ?? '3')
    try {
      setCategories(await listCategories())
    } catch {
      /* categories optional */
    }
  }

  async function confirmSubmitReview() {
    const id = submitTarget
    const sharePercent = Number(submitShare)
    if (
      !Number.isFinite(sharePercent) ||
      sharePercent < 0 ||
      sharePercent > 5
    ) {
      toast.error(t('Your share must be between 0% and 5%.'))
      return
    }
    setBusyId(id)
    try {
      await submitScriptForReview(id, {
        author_share_rate_ppm: Math.round(sharePercent * 10000),
        category_id: submitCategory ? Number(submitCategory) : 0,
      })
      toast.success(t('Submitted for review'))
      setSubmitTarget(0)
      await loadMine()
    } catch (err) {
      toast.error(String((err as Error)?.message || err))
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
        {/* Money earned as a script provider (author payable), with day/week/
            month breakdown. Refreshes when the script list reloads. */}
        <div className='mb-4'>
          <div className='mb-2 text-sm font-medium'>{t('Script earnings')}</div>
          <EarningsSummary role='author' refreshKey={myScripts.length} />
        </div>
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
                            busyId === script.id ||
                            !script.has_unpublished_changes
                          }
                          onClick={() =>
                            openSubmitDialog(script.id).catch((err) =>
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
                        disabled={
                          busyId === script.id || !script.latest_version
                        }
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

        <MyScriptCodeDialog
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
              ? t('Version history for script #{{id}}', {
                  id: historyScript.id,
                })
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
                  <TableCell>
                    {version.signature ? t('Yes') : t('No')}
                  </TableCell>
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
            <Textarea
              value={editing.description}
              placeholder={t('Description')}
              rows={3}
              className='resize-y'
              onChange={(event) =>
                setEditing((prev) => ({
                  ...prev,
                  description: event.target.value,
                }))
              }
            />

            {/* Concurrency: how many tasks this script can run simultaneously on a
                single node. Matches the target site's parallel task capacity. */}
            <div className='flex items-center gap-3'>
              <div className='min-w-0 flex-1'>
                <div className='text-sm font-medium'>{t('Concurrency')}</div>
                <div className='text-muted-foreground mt-0.5 text-xs'>
                  {t(
                    'Max simultaneous executions on one node (matches target site capacity, default 1)'
                  )}
                </div>
              </div>
              <Input
                type='number'
                min={1}
                step={1}
                className='h-9 w-24'
                value={editing.concurrency ?? 1}
                onChange={(event) => {
                  const parsed = Math.floor(Number(event.target.value))
                  setEditing((prev) => ({
                    ...prev,
                    concurrency: Number.isFinite(parsed) && parsed >= 1 ? parsed : 1,
                  }))
                }}
                aria-label={t('Concurrency')}
              />
            </div>

            {/* ── Collapsible reference example ───────────────────────── */}
            <div className='rounded-lg border'>
              <button
                type='button'
                className='hover:bg-muted/50 flex w-full items-center justify-between px-3 py-2 text-left text-sm font-medium transition-colors'
                onClick={() => setShowExample((v) => !v)}
              >
                <span className='flex items-center gap-2'>
                  <span>{showExample ? '▾' : '▸'}</span>
                  {t('Parameter & return value example')}
                </span>
                {!showExample && (
                  <span className='text-muted-foreground text-xs font-normal'>
                    {t('Click to expand')}
                  </span>
                )}
              </button>

              {showExample && (
                <div className='border-t px-3 pt-2 pb-3'>
                  {/* Brief guide */}
                  <p className='text-muted-foreground mb-3 text-xs leading-relaxed'>
                    {t(
                      "The sandbox exposes params (your Script Params object) and context.apiKey. Your script must return { output, balance } — balance updates the node's remaining display after each run."
                    )}
                  </p>

                  {/* Side-by-side: params + code */}
                  <div className='grid gap-3 lg:grid-cols-2'>
                    <div>
                      <div className='mb-1 flex items-center justify-between'>
                        <span className='text-muted-foreground text-xs font-medium'>
                          {t('Script Params (JSON)')}
                        </span>
                        <Button
                          type='button'
                          variant='ghost'
                          size='sm'
                          className='h-6 px-2 text-xs'
                          onClick={() =>
                            setEditing((prev) => ({
                              ...prev,
                              script_params: EXAMPLE_PARAMS,
                            }))
                          }
                        >
                          {t('Load params')}
                        </Button>
                      </div>
                      <pre className='bg-muted/40 overflow-auto rounded-md p-2 font-mono text-xs leading-relaxed'>
                        {EXAMPLE_PARAMS}
                      </pre>
                    </div>

                    <div>
                      <div className='mb-1 flex items-center justify-between'>
                        <span className='text-muted-foreground text-xs font-medium'>
                          {t('Script code (JavaScript)')}
                        </span>
                        <Button
                          type='button'
                          variant='ghost'
                          size='sm'
                          className='h-6 px-2 text-xs'
                          onClick={() =>
                            setEditing((prev) => ({
                              ...prev,
                              draft_code: EXAMPLE_CODE,
                            }))
                          }
                        >
                          {t('Load code')}
                        </Button>
                      </div>
                      <pre className='bg-muted/40 overflow-auto rounded-md p-2 font-mono text-xs leading-relaxed'>
                        {EXAMPLE_CODE}
                      </pre>
                    </div>
                  </div>

                  <div className='mt-2 flex justify-end'>
                    <Button
                      type='button'
                      variant='secondary'
                      size='sm'
                      onClick={() => {
                        setEditing((prev) => ({
                          ...prev,
                          script_params: EXAMPLE_PARAMS,
                          draft_code: EXAMPLE_CODE,
                        }))
                        setShowExample(false)
                        toast.success(t('Example loaded'))
                      }}
                    >
                      {t('Load full example')}
                    </Button>
                  </div>
                </div>
              )}
            </div>
            {/* ────────────────────────────────────────────────────────── */}

            <div>
              <div className='text-muted-foreground mb-1 text-xs'>
                {t('Script Params')}
                <span className='ml-1 font-normal opacity-70'>(JSON)</span>
              </div>
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
            </div>

            <div>
              <div className='text-muted-foreground mb-1 text-xs'>
                {t('Script code')}
                <span className='ml-1 font-normal opacity-70'>
                  (JavaScript)
                </span>
              </div>
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
          </div>
        </Dialog>

        {/* Submit-for-review: author assigns a target-site category and proposes
            their revenue share (% of the provider execution price). */}
        <Dialog
          open={submitTarget > 0}
          onOpenChange={(open) => {
            if (!open) setSubmitTarget(0)
          }}
          title={t('Submit for review')}
          description={t('Assign a category and propose your revenue share.')}
        >
          <div className='space-y-3'>
            <div>
              <label className='mb-1 block text-sm'>
                {t('Target-site category')}
              </label>
              <select
                className='h-9 w-full rounded-md border px-2 text-sm'
                value={submitCategory}
                onChange={(e) => {
                  setSubmitCategory(e.target.value)
                  localStorage.setItem('script_submit_category', e.target.value)
                }}
              >
                <option value=''>{t('(none)')}</option>
                {categories.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.name} {c.site ? `(${c.site})` : ''}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className='mb-1 block text-sm'>{t('Your share %')}</label>
              <Input
                type='number'
                min='0'
                max='5'
                step='0.01'
                value={submitShare}
                onChange={(e) => {
                  setSubmitShare(e.target.value)
                  localStorage.setItem('script_submit_share', e.target.value)
                }}
                placeholder='3'
              />
            </div>
            <div className='flex justify-end gap-2'>
              <Button variant='outline' onClick={() => setSubmitTarget(0)}>
                {t('Cancel')}
              </Button>
              <Button
                disabled={busyId === submitTarget}
                onClick={() => confirmSubmitReview()}
              >
                {t('Submit')}
              </Button>
            </div>
          </div>
        </Dialog>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
