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
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import {
  createCategory,
  deleteScriptVersion,
  generatePlatformSigningKey,
  getPlatformSigningKey,
  listCategories,
  listPendingScripts,
  listPublishedScriptVersions,
  getLatestPluginRelease,
  listScriptModelBindings,
  publishScriptAsModel,
  reviewScript,
  revokeScriptVersion,
  setCategoryBalanceScript,
  unpublishScriptModel,
  updateScriptVersionPricing,
  uploadPluginRelease,
  type PlatformSigningKey,
  type PluginRelease,
  type ScriptCategory,
  type ScriptModelBinding,
} from './api'
import { EarningsSummary } from './earnings-summary'
import { formatUnix } from './lib/format'
import type { ScriptVersion } from './types'

// ppm helpers: 10000 ppm = 1%.
function ppmToPercent(ppm?: number): string {
  return ((ppm ?? 0) / 10000).toFixed(2)
}
function percentToPpm(percent: string): number {
  const n = Number(percent)
  return Number.isFinite(n) ? Math.round(n * 10000) : 0
}

type PendingScript = {
  id: number
  user_id: number
  author_username: string
  title: string
  description: string
  script_params?: string
  draft_code: string
  previous_title?: string
  previous_description?: string
  previous_script_params?: string
  previous_code?: string
  review_status: string
  author_share_rate_ppm?: number
  category_id?: number
  /** Max simultaneous executions per node this script supports. */
  concurrency?: number
}

type DiffLine = {
  kind: 'same' | 'remove' | 'add'
  text: string
  number?: number
}
type PricingEdit = { author: string; platform: string }

function buildLineDiff(previous = '', next = ''): DiffLine[] {
  if (!previous) {
    return next.split('\n').map((text, index) => ({
      kind: 'add',
      text,
      number: index + 1,
    }))
  }
  const oldLines = previous.split('\n')
  const newLines = next.split('\n')
  let prefix = 0
  while (
    prefix < oldLines.length &&
    prefix < newLines.length &&
    oldLines[prefix] === newLines[prefix]
  ) {
    prefix++
  }
  let suffix = 0
  while (
    suffix < oldLines.length - prefix &&
    suffix < newLines.length - prefix &&
    oldLines[oldLines.length - 1 - suffix] ===
      newLines[newLines.length - 1 - suffix]
  ) {
    suffix++
  }

  return [
    ...oldLines.slice(0, prefix).map((text, index) => ({
      kind: 'same' as const,
      text,
      number: index + 1,
    })),
    ...oldLines.slice(prefix, oldLines.length - suffix).map((text, index) => ({
      kind: 'remove' as const,
      text,
      number: prefix + index + 1,
    })),
    ...newLines.slice(prefix, newLines.length - suffix).map((text, index) => ({
      kind: 'add' as const,
      text,
      number: prefix + index + 1,
    })),
    ...newLines.slice(newLines.length - suffix).map((text, index) => ({
      kind: 'same' as const,
      text,
      number: newLines.length - suffix + index + 1,
    })),
  ]
}

function ChangedField({
  label,
  previous,
  next,
}: {
  label: string
  previous?: string
  next?: string
}) {
  if ((previous || '') === (next || '')) return null
  return (
    <div className='grid gap-2 border-b pb-3 last:border-b-0 sm:grid-cols-[120px_1fr_1fr]'>
      <div className='text-sm font-medium'>{label}</div>
      <div className='rounded-md bg-red-500/10 p-2 text-sm whitespace-pre-wrap'>
        <span className='mr-2 text-red-600'>-</span>
        {previous || '-'}
      </div>
      <div className='rounded-md bg-green-500/10 p-2 text-sm whitespace-pre-wrap'>
        <span className='mr-2 text-green-700'>+</span>
        {next || '-'}
      </div>
    </div>
  )
}

export function ScriptReviewConsolePage() {
  const { t } = useTranslation()
  const [pending, setPending] = useState<PendingScript[]>([])
  const [loading, setLoading] = useState(false)
  // Bumped on every load() so the earnings cards refetch alongside the console.
  const [refreshTick, setRefreshTick] = useState(0)
  const [notes, setNotes] = useState<Record<number, string>>({})
  const [preview, setPreview] = useState<PendingScript | null>(null)
  const [previewMode, setPreviewMode] = useState<'changes' | 'code'>('changes')
  const [publishedVersions, setPublishedVersions] = useState<ScriptVersion[]>(
    []
  )
  const [publishedCategory, setPublishedCategory] = useState('all')
  const [historyScriptId, setHistoryScriptId] = useState(0)
  const [revokeReasons, setRevokeReasons] = useState<Record<number, string>>({})
  const [pricingEdits, setPricingEdits] = useState<Record<number, PricingEdit>>(
    {}
  )
  const [savingVersionId, setSavingVersionId] = useState(0)
  // Operator sets the platform fee (%) per pending script on approval.
  const [platformFees, setPlatformFees] = useState<Record<number, string>>({})
  // Category management state.
  const [categories, setCategories] = useState<ScriptCategory[]>([])
  const [newCatName, setNewCatName] = useState('')
  const [newCatSite, setNewCatSite] = useState('')
  const [balScript, setBalScript] = useState<Record<number, string>>({}) // catId -> "scriptId:version"
  // Platform signing key status. Signing is mandatory for publishing.
  const [signingKey, setSigningKey] = useState<PlatformSigningKey | null>(null)
  const [generatingKey, setGeneratingKey] = useState(false)
  // Marketplace model bindings: published scripts listed as callable new-api
  // models. Keyed for lookup by script id (a script has at most one binding in
  // the MVP — the latest published version it was listed under).
  const [modelBindings, setModelBindings] = useState<ScriptModelBinding[]>([])
  // Draft model name per script id, typed before clicking "List as model".
  const [modelNameDrafts, setModelNameDrafts] = useState<
    Record<number, string>
  >({})
  const [publishingScriptId, setPublishingScriptId] = useState(0)
  // Browser-extension release management. The operator registers a new release
  // by providing an external download URL, version, and filename. The newest
  // entry is what the extension update-checks against and what the node console
  // links to for download.
  const [pluginRelease, setPluginRelease] = useState<PluginRelease | null>(null)
  const [pluginVersion, setPluginVersion] = useState('')
  const [pluginDownloadUrl, setPluginDownloadUrl] = useState('')
  const [pluginFilename, setPluginFilename] = useState('')
  const [pluginReleaseNotes, setPluginReleaseNotes] = useState('')
  const [uploadingPlugin, setUploadingPlugin] = useState(false)

  const categoryNames = useMemo(
    () => new Map(categories.map((category) => [category.id, category.name])),
    [categories]
  )
  // Script ids designated as a category's balance-probe script. These read a
  // site's balance without generating, so they can never be listed as models.
  const balanceScriptIds = useMemo(
    () =>
      new Set(
        categories.map((category) => category.balance_script_id).filter(Boolean)
      ),
    [categories]
  )
  // Model binding per script id for quick "already listed as model" lookup.
  const bindingByScriptId = useMemo(
    () =>
      new Map(modelBindings.map((binding) => [binding.script_id, binding])),
    [modelBindings]
  )
  const publishedGroups = useMemo(() => {
    const groups = new Map<number, ScriptVersion[]>()
    for (const version of publishedVersions) {
      const versions = groups.get(version.script_id) || []
      versions.push(version)
      groups.set(version.script_id, versions)
    }
    return Array.from(groups.values())
      .map((versions) => versions.sort((a, b) => b.version - a.version))
      .filter(
        (versions) =>
          publishedCategory === 'all' ||
          versions[0].category_id === Number(publishedCategory)
      )
  }, [publishedCategory, publishedVersions])
  const historyVersions = useMemo(
    () =>
      publishedVersions
        .filter((version) => version.script_id === historyScriptId)
        .sort((a, b) => b.version - a.version),
    [historyScriptId, publishedVersions]
  )

  async function load() {
    setLoading(true)
    try {
      const [pendingScripts, versions, cats, key, bindings] =
        await Promise.all([
          listPendingScripts(),
          listPublishedScriptVersions(),
          listCategories(),
          getPlatformSigningKey(),
          listScriptModelBindings(),
        ])
      setPending(pendingScripts)
      setPublishedVersions(versions)
      setCategories(cats)
      setSigningKey(key)
      setModelBindings(bindings)
      getLatestPluginRelease()
        .then(setPluginRelease)
        .catch(() => {})
      setRefreshTick((n) => n + 1)
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
      await reviewScript(
        id,
        approve,
        notes[id] || '',
        approve ? percentToPpm(platformFees[id] || '0') : 0
      )
      toast.success(approve ? t('Approved') : t('Rejected'))
      await load()
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  async function onGenerateSigningKey() {
    const rotating = signingKey?.signing_enabled
    if (
      rotating &&
      !window.confirm(
        t(
          'A signing key already exists. Generating a new one invalidates all existing manifest signatures — every published version must be re-published to run again. Continue?'
        )
      )
    ) {
      return
    }
    setGeneratingKey(true)
    try {
      const key = await generatePlatformSigningKey()
      setSigningKey(key)
      toast.success(
        rotating ? t('Signing key rotated') : t('Signing key generated')
      )
    } catch (e) {
      toast.error(String((e as Error).message))
    } finally {
      setGeneratingKey(false)
    }
  }

  async function onCreateCategory() {
    if (!newCatName.trim()) {
      toast.error(t('Category name is required'))
      return
    }
    try {
      await createCategory(newCatName.trim(), newCatSite.trim())
      toast.success(t('Category created'))
      setNewCatName('')
      setNewCatSite('')
      await load()
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  async function onSetBalanceScript(catId: number) {
    const raw = (balScript[catId] || '').trim()
    const [sid, ver] = raw.split(':').map((x) => Number(x))
    if (!sid || !ver) {
      toast.error(t('Enter as scriptId:version'))
      return
    }
    try {
      await setCategoryBalanceScript(catId, sid, ver)
      toast.success(t('Balance script set'))
      await load()
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  async function onRevoke(version: ScriptVersion) {
    const reason = revokeReasons[version.id]?.trim()
    if (!reason) {
      toast.error(`${t('Reason')}: ${t('Required')}`)
      return
    }
    try {
      await revokeScriptVersion(
        version.script_id,
        version.version,
        reason,
        'normal'
      )
      toast.success(t('Version revoked'))
      setRevokeReasons((current) => ({ ...current, [version.id]: '' }))
      await load()
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  function pricingValue(version: ScriptVersion) {
    return (
      pricingEdits[version.id] || {
        author: ppmToPercent(version.author_share_rate_ppm),
        platform: ppmToPercent(version.platform_fee_rate_ppm),
      }
    )
  }

  function setPricingValue(
    version: ScriptVersion,
    patch: Partial<PricingEdit>
  ) {
    setPricingEdits((current) => ({
      ...current,
      [version.id]: { ...pricingValue(version), ...patch },
    }))
  }

  async function onSavePricing(version: ScriptVersion) {
    const value = pricingValue(version)
    const authorPercent = Number(value.author)
    const platformPercent = Number(value.platform)
    if (
      !Number.isFinite(authorPercent) ||
      authorPercent < 0 ||
      authorPercent > 100 ||
      !Number.isFinite(platformPercent) ||
      platformPercent < 0 ||
      platformPercent > 100
    ) {
      toast.error(t('Author and platform fee must be between 0% and 100%'))
      return
    }
    setSavingVersionId(version.id)
    try {
      await updateScriptVersionPricing(version.script_id, version.version, {
        author_share_rate_ppm: percentToPpm(value.author),
        platform_fee_rate_ppm: percentToPpm(value.platform),
      })
      toast.success(t('Pricing updated'))
      setPricingEdits((current) => {
        const next = { ...current }
        delete next[version.id]
        return next
      })
      await load()
    } catch (e) {
      toast.error(String((e as Error).message))
    } finally {
      setSavingVersionId(0)
    }
  }

  async function onDeleteVersion(version: ScriptVersion) {
    if (
      !window.confirm(
        t('Delete script #{{scriptId}} v{{version}}?', {
          scriptId: version.script_id,
          version: version.version,
        })
      )
    )
      return
    try {
      await deleteScriptVersion(version.script_id, version.version)
      toast.success(t('Version deleted'))
      await load()
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  // onPublishModel lists a published script version as a callable new-api
  // model. The model name must be unique; orders run under the operator's
  // marketplace balance. Uses the version shown in the row (latest published).
  async function onPublishModel(version: ScriptVersion) {
    const modelName = (modelNameDrafts[version.script_id] || '').trim()
    if (!modelName) {
      toast.error(t('Enter a model name first'))
      return
    }
    setPublishingScriptId(version.script_id)
    try {
      await publishScriptAsModel(version.script_id, version.version, {
        model_name: modelName,
      })
      toast.success(t('Listed on the model square'))
      setModelNameDrafts((current) => {
        const next = { ...current }
        delete next[version.script_id]
        return next
      })
      await load()
    } catch (e) {
      toast.error(String((e as Error).message))
    } finally {
      setPublishingScriptId(0)
    }
  }

  async function onUnpublishModel(binding: ScriptModelBinding) {
    if (
      !window.confirm(
        t('Remove model {{model}} from the model square?', {
          model: binding.model_name,
        })
      )
    )
      return
    try {
      await unpublishScriptModel(binding.model_name)
      toast.success(t('Removed from the model square'))
      // Restore the model name in the draft input so the operator can
      // immediately re-list without retyping it.
      setModelNameDrafts((current) => ({
        ...current,
        [binding.script_id]: binding.model_name,
      }))
      await load()
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  // Publish a new browser-extension release by registering its external
  // download URL. Validates required fields then refreshes the shown release.
  async function onUploadPlugin() {
    const version = pluginVersion.trim()
    const downloadUrl = pluginDownloadUrl.trim()
    const filename = pluginFilename.trim()

    if (!version) {
      toast.error(t('Enter a version number first'))
      return
    }
    if (!downloadUrl) {
      toast.error(t('Enter a download URL'))
      return
    }
    if (!filename) {
      toast.error(t('Enter a filename'))
      return
    }

    setUploadingPlugin(true)
    try {
      const release = await uploadPluginRelease({
        download_url: downloadUrl,
        version,
        filename,
        release_notes: pluginReleaseNotes.trim() || undefined,
      })
      setPluginRelease(release)
      setPluginVersion('')
      setPluginDownloadUrl('')
      setPluginFilename('')
      setPluginReleaseNotes('')
      toast.success(t('Plugin published'))
    } catch (e) {
      toast.error(String((e as Error).message))
    } finally {
      setUploadingPlugin(false)
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
        {/* Platform service-fee revenue (day/week/month/total). Reloads on the
            same refresh action as the rest of the console. */}
        <div className='mb-6'>
          <div className='mb-2 text-sm font-medium'>
            {t('Platform earnings')}
          </div>
          <EarningsSummary role='platform' refreshKey={refreshTick} />
        </div>

        {/* Platform signing key: publishing requires a configured key, and the
            plugin execution gate rejects any manifest without a valid signature. */}
        <div className='mb-6 rounded-lg border p-4'>
          <div className='mb-2 text-sm font-medium'>
            {t('Script signing key')}
          </div>
          {signingKey?.signing_enabled ? (
            <div className='flex flex-wrap items-center gap-x-6 gap-y-2 text-sm'>
              <div>
                {t('Status')}:{' '}
                <span className='font-medium text-green-600'>
                  {t('Enabled')}
                </span>
              </div>
              <div>
                {t('Key ID')}:{' '}
                <span className='font-mono text-xs'>{signingKey.key_id}</span>
              </div>
              <div className='flex min-w-0 items-center gap-2'>
                <span className='shrink-0'>{t('Public key')}:</span>
                <span
                  className='truncate font-mono text-xs'
                  title={signingKey.public_key}
                >
                  {signingKey.public_key}
                </span>
              </div>
              <Button
                size='sm'
                variant='outline'
                disabled={generatingKey}
                onClick={onGenerateSigningKey}
              >
                {generatingKey ? t('Working...') : t('Rotate key')}
              </Button>
            </div>
          ) : (
            <div className='flex flex-wrap items-center gap-3 text-sm'>
              <span className='text-red-600'>
                ⚠️{' '}
                {t(
                  'Signing is not configured. Scripts cannot be published or executed until a key is generated.'
                )}
              </span>
              <Button
                size='sm'
                disabled={generatingKey}
                onClick={onGenerateSigningKey}
              >
                {generatingKey ? t('Working...') : t('Generate signing key')}
              </Button>
            </div>
          )}
          {signingKey?.signing_enabled && (
            <p className='text-muted-foreground mt-2 text-xs'>
              {t(
                'Rotating the key invalidates existing signatures; already-published versions must be re-published to run again.'
              )}
            </p>
          )}
        </div>

        {/* Browser-extension release: upload a single packaged file (≤5MB) with
            a version number. The newest upload is the version the extension
            update-checks against and the node console offers for download. */}
        <div className='mb-6 rounded-lg border p-4'>
          <div className='mb-2 text-sm font-medium'>
            {t('Browser plugin release')}
          </div>
          <div className='text-muted-foreground mb-3 text-sm'>
            {t('Current version')}:{' '}
            {pluginRelease?.available ? (
              <span className='text-foreground font-medium'>
                v{pluginRelease.version}
              </span>
            ) : (
              <span>{t('none')}</span>
            )}
            {pluginRelease?.available && pluginRelease.filename ? (
              <span className='ml-2 font-mono text-xs'>
                {pluginRelease.filename}
              </span>
            ) : null}
          </div>

          {/* Mode toggle */}
          <div className='space-y-2'>
            <div className='flex flex-wrap items-center gap-2'>
              <Input
                className='h-9 flex-1 min-w-[300px]'
                placeholder={t('Download URL (https://...)')}
                value={pluginDownloadUrl}
                onChange={(e) => setPluginDownloadUrl(e.target.value)}
              />
              <Input
                className='h-9 w-32'
                placeholder={t('Version (e.g. 2.1.0)')}
                value={pluginVersion}
                onChange={(e) => setPluginVersion(e.target.value)}
              />
              <Input
                className='h-9 w-44'
                placeholder={t('Filename (e.g. plugin.zip)')}
                value={pluginFilename}
                onChange={(e) => setPluginFilename(e.target.value)}
              />
            </div>
            <Textarea
              className='min-h-[72px] w-full resize-none text-sm'
              placeholder={t('Release notes (optional) — describe what changed in this version')}
              value={pluginReleaseNotes}
              onChange={(e) => setPluginReleaseNotes(e.target.value)}
            />
            <Button
              disabled={
                uploadingPlugin ||
                !pluginDownloadUrl.trim() ||
                !pluginVersion.trim() ||
                !pluginFilename.trim()
              }
              onClick={onUploadPlugin}
            >
              {uploadingPlugin ? t('Publishing...') : t('Publish plugin')}
            </Button>
          </div>

          <p className='text-muted-foreground mt-2 text-xs'>
            {t('Provide an external URL where the plugin package is hosted. You can upload the file to any CDN or file hosting service.')}
          </p>
        </div>

        <div className='mb-2 text-sm font-medium'>{t('Pending review')}</div>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>ID</TableHead>
              <TableHead>{t('Title')}</TableHead>
              <TableHead>{t('Author')}</TableHead>
              <TableHead>{t('Category')}</TableHead>
              <TableHead>{t('Author share')}</TableHead>
              <TableHead>{t('Concurrency')}</TableHead>
              <TableHead>{t('Platform fee %')}</TableHead>
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
                  {categoryNames.get(s.category_id || 0) || t('Uncategorized')}
                </TableCell>
                <TableCell>{ppmToPercent(s.author_share_rate_ppm)}%</TableCell>
                <TableCell>{s.concurrency ?? 1}</TableCell>
                <TableCell>
                  <Input
                    className='h-8 w-20'
                    type='number'
                    placeholder='8'
                    value={platformFees[s.id] ?? ''}
                    onChange={(e) =>
                      setPlatformFees((p) => ({ ...p, [s.id]: e.target.value }))
                    }
                  />
                </TableCell>
                <TableCell>
                  <Button
                    size='sm'
                    variant='outline'
                    onClick={() => {
                      setPreviewMode('changes')
                      setPreview(s)
                    }}
                  >
                    {t('Changes')}
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
                <TableCell
                  colSpan={9}
                  className='text-muted-foreground text-center'
                >
                  {loading ? t('Loading...') : t('No pending scripts')}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>

        {/* Target-site categories: create + designate each category's audited
            balance-probe script (scriptId:version of a published script). */}
        <div className='mt-6 rounded-lg border p-4'>
          <div className='mb-2 text-sm font-medium'>{t('Site categories')}</div>
          <div className='mb-3 flex flex-wrap items-center gap-2'>
            <Input
              className='w-40'
              placeholder={t('Category name')}
              value={newCatName}
              onChange={(e) => setNewCatName(e.target.value)}
            />
            <Input
              className='w-56'
              placeholder={t('Target site (e.g. dreamina.com)')}
              value={newCatSite}
              onChange={(e) => setNewCatSite(e.target.value)}
            />
            <Button onClick={onCreateCategory}>{t('Create category')}</Button>
          </div>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>ID</TableHead>
                <TableHead>{t('Name')}</TableHead>
                <TableHead>{t('Site')}</TableHead>
                <TableHead>{t('Balance script')}</TableHead>
                <TableHead>
                  {t('Set balance script (scriptId:version)')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {categories.map((cat) => (
                <TableRow key={cat.id}>
                  <TableCell>{cat.id}</TableCell>
                  <TableCell>{cat.name}</TableCell>
                  <TableCell>{cat.site || '-'}</TableCell>
                  <TableCell>
                    {cat.balance_script_id
                      ? `#${cat.balance_script_id} v${cat.balance_script_version}`
                      : '⚠️ ' + t('none')}
                  </TableCell>
                  <TableCell className='space-x-2'>
                    <Input
                      className='inline-block h-8 w-32'
                      placeholder='12:1'
                      value={balScript[cat.id] || ''}
                      onChange={(e) =>
                        setBalScript((p) => ({
                          ...p,
                          [cat.id]: e.target.value,
                        }))
                      }
                    />
                    <Button
                      size='sm'
                      onClick={() => onSetBalanceScript(cat.id)}
                    >
                      {t('Set')}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
              {categories.length === 0 && (
                <TableRow>
                  <TableCell
                    colSpan={5}
                    className='text-muted-foreground text-center'
                  >
                    {t('No categories yet')}
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>

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
          <div className='mb-4 flex w-fit rounded-md border p-1'>
            <Button
              size='sm'
              variant={previewMode === 'changes' ? 'secondary' : 'ghost'}
              onClick={() => setPreviewMode('changes')}
            >
              {t('Changes')}
            </Button>
            <Button
              size='sm'
              variant={previewMode === 'code' ? 'secondary' : 'ghost'}
              onClick={() => setPreviewMode('code')}
            >
              {t('Full Code')}
            </Button>
          </div>

          {previewMode === 'changes' && preview ? (
            <div className='space-y-4'>
              {!preview.previous_code ? (
                <div className='bg-muted/30 rounded-md border p-3 text-sm'>
                  {t('Initial version')}
                </div>
              ) : null}
              <div className='space-y-3'>
                <ChangedField
                  label={t('Title')}
                  previous={preview.previous_title}
                  next={preview.title}
                />
                <ChangedField
                  label={t('Description')}
                  previous={preview.previous_description}
                  next={preview.description}
                />
                <ChangedField
                  label={t('Script Params')}
                  previous={preview.previous_script_params}
                  next={preview.script_params}
                />
              </div>
              <div className='overflow-auto rounded-md border font-mono text-xs'>
                {buildLineDiff(preview.previous_code, preview.draft_code).map(
                  (line, index) => (
                    <div
                      key={`${line.kind}-${index}`}
                      className={
                        line.kind === 'remove'
                          ? 'flex bg-red-500/10 text-red-800'
                          : line.kind === 'add'
                            ? 'flex bg-green-500/10 text-green-800'
                            : 'flex'
                      }
                    >
                      <span className='text-muted-foreground w-12 shrink-0 border-r px-2 py-0.5 text-right select-none'>
                        {line.number}
                      </span>
                      <span className='w-6 shrink-0 py-0.5 text-center select-none'>
                        {line.kind === 'remove'
                          ? '-'
                          : line.kind === 'add'
                            ? '+'
                            : ' '}
                      </span>
                      <span className='min-w-0 flex-1 px-2 py-0.5 whitespace-pre'>
                        {line.text || ' '}
                      </span>
                    </div>
                  )
                )}
              </div>
            </div>
          ) : (
            <pre className='bg-muted/40 overflow-auto rounded-lg border p-3 font-mono text-xs whitespace-pre-wrap'>
              {preview?.draft_code || ''}
            </pre>
          )}
        </Dialog>

        <div className='mt-6'>
          <div className='mb-2 flex flex-wrap items-center justify-between gap-2'>
            <div className='text-sm font-medium'>{t('Published Scripts')}</div>
            <label className='flex items-center gap-2 text-sm'>
              <span>{t('Category')}</span>
              <select
                className='bg-background h-8 rounded-md border px-2 text-sm'
                value={publishedCategory}
                onChange={(event) => setPublishedCategory(event.target.value)}
              >
                <option value='all'>{t('All categories')}</option>
                {categories.map((category) => (
                  <option key={category.id} value={category.id}>
                    {category.name}
                  </option>
                ))}
              </select>
            </label>
          </div>
          <div className='overflow-auto rounded-md border'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Script')}</TableHead>
                  <TableHead>{t('Version')}</TableHead>
                  <TableHead>{t('Category')}</TableHead>
                  <TableHead>{t('Author')}</TableHead>
                  <TableHead>{t('Concurrency')}</TableHead>
                  <TableHead>{t('Published')}</TableHead>
                  <TableHead>{t('Status')}</TableHead>
                  <TableHead>{t('Model square')}</TableHead>
                  <TableHead>{t('Reason')}</TableHead>
                  <TableHead className='text-right'>{t('Actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {publishedGroups.map((versions) => {
                  const version = versions[0]
                  return (
                    <TableRow key={version.id}>
                      <TableCell>
                        <div className='font-medium'>{version.title}</div>
                        <div className='text-muted-foreground text-xs'>
                          #{version.script_id}
                        </div>
                      </TableCell>
                      <TableCell>v{version.version}</TableCell>
                      <TableCell>
                        {categoryNames.get(version.category_id) ||
                          t('Uncategorized')}
                      </TableCell>
                      <TableCell>
                        {version.author_username || `#${version.author_id}`}
                      </TableCell>
                      <TableCell>{version.concurrency ?? 1}</TableCell>
                      <TableCell>{formatUnix(version.published_at)}</TableCell>
                      <TableCell>
                        {version.revoked_at ? t('Revoked') : t('Published')}
                      </TableCell>
                      {/* Model-square listing: set a unique model name and list
                          the script as a callable new-api model. Balance-probe
                          scripts are excluded (they only read site balance). */}
                      <TableCell>
                        {balanceScriptIds.has(version.script_id) ? (
                          <span className='text-muted-foreground text-xs'>
                            {t('Balance script')}
                          </span>
                        ) : bindingByScriptId.has(version.script_id) ? (
                          <div className='flex flex-col gap-1'>
                            <span className='font-mono text-xs'>
                              {
                                bindingByScriptId.get(version.script_id)
                                  ?.model_name
                              }
                            </span>
                            <Button
                              size='sm'
                              variant='ghost'
                              className='h-6 w-fit px-2 text-xs text-red-600'
                              onClick={() =>
                                onUnpublishModel(
                                  bindingByScriptId.get(version.script_id)!
                                )
                              }
                            >
                              {t('Unlist')}
                            </Button>
                          </div>
                        ) : version.revoked_at ? (
                          <span className='text-muted-foreground text-xs'>
                            -
                          </span>
                        ) : (
                          <div className='flex items-center gap-1'>
                            <Input
                              className='h-8 w-32'
                              placeholder={t('Model name')}
                              value={modelNameDrafts[version.script_id] || ''}
                              onChange={(event) =>
                                setModelNameDrafts((current) => ({
                                  ...current,
                                  [version.script_id]: event.target.value,
                                }))
                              }
                            />
                            <Button
                              size='sm'
                              disabled={
                                publishingScriptId === version.script_id ||
                                !modelNameDrafts[version.script_id]?.trim()
                              }
                              onClick={() => onPublishModel(version)}
                            >
                              {publishingScriptId === version.script_id
                                ? t('Listing...')
                                : t('List')}
                            </Button>
                          </div>
                        )}
                      </TableCell>
                      <TableCell>
                        {version.revoked_at ? (
                          version.revoked_reason || '-'
                        ) : (
                          <Input
                            className='h-8 min-w-48'
                            placeholder={`${t('Reason')} (${t('Required')})`}
                            value={revokeReasons[version.id] || ''}
                            onChange={(event) =>
                              setRevokeReasons((current) => ({
                                ...current,
                                [version.id]: event.target.value,
                              }))
                            }
                          />
                        )}
                      </TableCell>
                      <TableCell className='text-right'>
                        <div className='flex justify-end gap-2'>
                          <Button
                            size='sm'
                            variant='outline'
                            onClick={() =>
                              setHistoryScriptId(version.script_id)
                            }
                          >
                            {t('History')} ({versions.length})
                          </Button>
                          <Button
                            size='sm'
                            variant='destructive'
                            disabled={
                              !!version.revoked_at ||
                              !revokeReasons[version.id]?.trim()
                            }
                            onClick={() => onRevoke(version)}
                          >
                            {t('Revoke version')}
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  )
                })}
                {publishedGroups.length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={9}
                      className='text-muted-foreground py-8 text-center'
                    >
                      {loading ? t('Loading...') : t('No published versions')}
                    </TableCell>
                  </TableRow>
                ) : null}
              </TableBody>
            </Table>
          </div>
        </div>

        <Dialog
          open={historyScriptId > 0}
          onOpenChange={(open) => {
            if (!open) setHistoryScriptId(0)
          }}
          title={t('Version history')}
          description={historyVersions[0]?.title || ''}
        >
          <div className='max-h-[70vh] space-y-2 overflow-auto'>
            {historyVersions.map((version) => (
              <div
                key={version.id}
                className='grid gap-3 rounded-md border p-3 text-sm lg:grid-cols-[70px_1fr_3fr_auto] lg:items-center'
              >
                <div className='font-medium'>v{version.version}</div>
                <div>
                  <div>{formatUnix(version.published_at)}</div>
                  <div className='text-muted-foreground text-xs'>
                    {categoryNames.get(version.category_id) ||
                      t('Uncategorized')}
                    {' / '}
                    {version.author_username || `#${version.author_id}`}
                  </div>
                  {version.revoked_at ? (
                    <div className='text-destructive text-xs'>
                      {t('Revoked')}: {version.revoked_reason || '-'}
                    </div>
                  ) : null}
                </div>
                <div className='grid gap-2 sm:grid-cols-2'>
                  <label className='space-y-1'>
                    <span className='text-muted-foreground text-xs'>
                      {t('Author')} (%)
                    </span>
                    <Input
                      className='h-8'
                      value={pricingValue(version).author}
                      onChange={(event) =>
                        setPricingValue(version, { author: event.target.value })
                      }
                    />
                  </label>
                  <label className='space-y-1'>
                    <span className='text-muted-foreground text-xs'>
                      {t('Platform fee')} (%)
                    </span>
                    <Input
                      className='h-8'
                      value={pricingValue(version).platform}
                      onChange={(event) =>
                        setPricingValue(version, {
                          platform: event.target.value,
                        })
                      }
                    />
                  </label>
                </div>
                <div className='flex flex-wrap justify-end gap-2'>
                  <Button
                    size='sm'
                    variant='outline'
                    disabled={savingVersionId === version.id}
                    onClick={() => onSavePricing(version)}
                  >
                    {savingVersionId === version.id
                      ? t('Saving...')
                      : t('Save pricing')}
                  </Button>
                  <Button
                    size='sm'
                    variant='destructive'
                    disabled={version.version === historyVersions[0]?.version}
                    title={
                      version.version === historyVersions[0]?.version
                        ? t('The latest version cannot be deleted')
                        : undefined
                    }
                    onClick={() => onDeleteVersion(version)}
                  >
                    {t('Delete')}
                  </Button>
                </div>
              </div>
            ))}
          </div>
        </Dialog>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
