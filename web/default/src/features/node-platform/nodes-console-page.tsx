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
import { Ban, ChevronDown, Cpu, Layers3, Server, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
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
import { api } from '@/lib/api'

import {
  createCapabilityTest,
  deleteDevice,
  deleteNode,
  enableCapability,
  getMyProviderGroup,
  listAvailableScriptVersions,
  listBalanceChecks,
  listCategories,
  listMyDevices,
  listMyNodes,
  listNodeCapabilities,
  listProviderCapabilityStats,
  removeCapability,
  requestBalanceCheck,
  revokeDevice,
} from './api'
import type { NodeBalanceCheck, ProviderGroup, ScriptCategory } from './api'
import { EarningsSummary } from './earnings-summary'
import { displayToMicros, formatUnix, microsToCurrency } from './lib/format'
import type {
  CapabilityStat,
  Device,
  NodeCapability,
  NodeInfo,
  ScriptVersion,
} from './types'

type PublishedScript = {
  id: number
  title: string
  category_id?: number
}

type EnableFormValue = {
  scriptId: string
  version: string
  price: string
  dailyLimit: string
}
type NodesConsoleDraft = {
  hideInactive: boolean
  enableForm: Record<string, EnableFormValue>
  openNodeIds: string[]
}

const DEFAULT_ENABLE_FORM: EnableFormValue = {
  scriptId: '',
  version: '1',
  price: '',
  dailyLimit: '0',
}

function getDraftStorageKey() {
  const userId = window.localStorage.getItem('uid') ?? 'anonymous'
  return `nodes-console-draft:${userId}`
}

function loadNodesConsoleDraft(): NodesConsoleDraft {
  try {
    const saved = JSON.parse(
      window.localStorage.getItem(getDraftStorageKey()) ?? '{}'
    ) as Partial<NodesConsoleDraft>
    const enableForm = Object.fromEntries(
      Object.entries(saved.enableForm ?? {}).flatMap(([nodeId, value]) => {
        if (!value || typeof value !== 'object') return []
        const form = value as Partial<EnableFormValue>
        return [
          [
            nodeId,
            {
              scriptId: typeof form.scriptId === 'string' ? form.scriptId : '',
              version: typeof form.version === 'string' ? form.version : '',
              price: typeof form.price === 'string' ? form.price : '',
              dailyLimit:
                typeof (form as any).dailyLimit === 'string'
                  ? (form as any).dailyLimit
                  : '0',
            },
          ] as const,
        ]
      })
    ) as Record<string, EnableFormValue>
    return {
      hideInactive:
        typeof saved.hideInactive === 'boolean' ? saved.hideInactive : true,
      enableForm,
      openNodeIds: Array.isArray(saved.openNodeIds)
        ? saved.openNodeIds.filter((id): id is string => typeof id === 'string')
        : [],
    }
  } catch {
    return { hideInactive: true, enableForm: {}, openNodeIds: [] }
  }
}

function nodeOnline(n: NodeInfo): boolean {
  return (
    n.state !== 'OFFLINE' &&
    n.last_seen_at >= Math.floor(Date.now() / 1000) - 45
  )
}

export function NodesConsolePage() {
  const { t } = useTranslation()
  const [initialDraft] = useState(loadNodesConsoleDraft)
  const [devices, setDevices] = useState<Device[]>([])
  const [nodes, setNodes] = useState<NodeInfo[]>([])
  const [caps, setCaps] = useState<Record<string, NodeCapability[]>>({})
  const [loading, setLoading] = useState(false)
  // A user may register dozens/hundreds of devices; hide revoked/offline by
  // default to keep the list readable.
  const [hideInactive, setHideInactive] = useState(initialDraft.hideInactive)
  // Published scripts to pick from when listing a capability.
  const [pubScripts, setPubScripts] = useState<PublishedScript[]>([])
  const [scriptVersions, setScriptVersions] = useState<
    Record<number, ScriptVersion[]>
  >({})
  const [categories, setCategories] = useState<ScriptCategory[]>([])
  const [balanceChecks, setBalanceChecks] = useState<
    Record<string, NodeBalanceCheck[]>
  >({})
  const [checking, setChecking] = useState('')
  // Per-(node, script, version) execution stats, keyed "nodeId:scriptId:version".
  const [capStats, setCapStats] = useState<Record<string, CapabilityStat>>({})
  // Bumped on each loadAll() so the provider earnings cards refetch.
  const [refreshTick, setRefreshTick] = useState(0)
  // Per-node enable form: script id + version + price + quota.
  const [enableForm, setEnableForm] = useState<Record<string, EnableFormValue>>(
    initialDraft.enableForm
  )
  const [openNodeIds, setOpenNodeIds] = useState(initialDraft.openNodeIds)
  // The caller's provider group (all their nodes belong to it). Created on first
  // load from the username; every node defaults into this group.
  const [providerGroup, setProviderGroup] = useState<ProviderGroup | null>(null)

  const visibleDevices = hideInactive
    ? devices.filter((d) => d.status === 'active')
    : devices
  const visibleNodes = hideInactive ? nodes.filter((n) => nodeOnline(n)) : nodes

  const nodesByDevice = visibleNodes.reduce<Record<string, NodeInfo[]>>(
    (groups, node) => {
      ;(groups[node.device_id] ??= []).push(node)
      return groups
    },
    {}
  )
  const visibleDeviceIds = new Set(visibleDevices.map((device) => device.id))
  const ungroupedNodes = visibleNodes.filter(
    (node) => !visibleDeviceIds.has(node.device_id)
  )

  async function loadAll(restoreSavedDraft = false) {
    setLoading(true)
    try {
      const [d, n, sq, categoryList, stats] = await Promise.all([
        listMyDevices(),
        listMyNodes(),
        api.get('/api/scripts/square', { params: { limit: 200 } }),
        listCategories(),
        listProviderCapabilityStats().catch(() => [] as CapabilityStat[]),
      ])
      const deviceList = Array.isArray(d) ? d : []
      const nodeList = Array.isArray(n) ? n : []
      const safeCategoryList = Array.isArray(categoryList) ? categoryList : []
      const statList = Array.isArray(stats) ? stats : []
      setDevices(deviceList)
      setNodes(nodeList)
      // Resolve (and lazily create) the caller's provider group so all their
      // nodes are grouped under it. Best-effort: the console still works if it
      // fails.
      getMyProviderGroup()
        .then(setProviderGroup)
        .catch(() => {})
      const items = (sq.data?.data?.items ??
        sq.data?.items ??
        sq.data?.data ??
        []) as PublishedScript[]
      const balanceScriptIds = new Set(
        safeCategoryList
          .map((category) => category.balance_script_id)
          .filter(Boolean)
      )
      const listableItems = (Array.isArray(items) ? items : []).filter(
        (script) => !balanceScriptIds.has(script.id)
      )
      setPubScripts(listableItems)
      setCategories(safeCategoryList)
      setCapStats(
        Object.fromEntries(
          statList.map((s) => [`${s.node_id}:${s.script_id}:${s.version}`, s])
        )
      )
      setRefreshTick((tick) => tick + 1)

      const validNodeIds = new Set(nodeList.map((node) => node.id))
      const validScriptIds = new Set(listableItems.map((script) => script.id))
      const formsToRestore = restoreSavedDraft
        ? initialDraft.enableForm
        : enableForm
      const restoredForms = Object.fromEntries(
        Object.entries(formsToRestore).filter(
          ([nodeId, form]) =>
            validNodeIds.has(nodeId) &&
            (!form.scriptId || validScriptIds.has(Number(form.scriptId)))
        )
      )
      const selectedScriptIds = [
        ...new Set(
          Object.values(restoredForms)
            .map((form) => Number(form.scriptId))
            .filter(Boolean)
        ),
      ]
      const versionEntries = await Promise.all(
        selectedScriptIds.map(async (scriptId) => {
          try {
            const versions = await listAvailableScriptVersions(scriptId)
            return [
              scriptId,
              (Array.isArray(versions) ? versions : []).filter(
                (item) =>
                  !safeCategoryList.some(
                    (category) =>
                      category.balance_script_id === item.script_id &&
                      category.balance_script_version === item.version
                  )
              ),
            ] as const
          } catch {
            return [scriptId, []] as const
          }
        })
      )
      const restoredVersions = Object.fromEntries(versionEntries) as Record<
        number,
        ScriptVersion[]
      >
      setScriptVersions(restoredVersions)
      setEnableForm(
        Object.fromEntries(
          Object.entries(restoredForms).map(([nodeId, form]) => {
            if (!form.scriptId) return [nodeId, form]
            const versions = restoredVersions[Number(form.scriptId)] ?? []
            const version = versions.some(
              (item) => String(item.version) === form.version
            )
              ? form.version
              : versions[0]
                ? String(versions[0].version)
                : ''
            return [nodeId, { ...form, version }]
          })
        )
      )

      const nodeIdsToRestore = restoreSavedDraft
        ? initialDraft.openNodeIds
        : openNodeIds
      const restoredOpenNodeIds = nodeIdsToRestore.filter((id) =>
        validNodeIds.has(id)
      )
      setOpenNodeIds(restoredOpenNodeIds)
      await Promise.all(restoredOpenNodeIds.map((id) => loadCaps(id)))
    } catch (e) {
      toast.error(String((e as Error).message))
    } finally {
      setLoading(false)
    }
  }

  // Enable a capability: run the challenge test to get a window, then list the
  // script version on the node with the provider's price and daily quota.
  async function onEnableCapability(nodeId: string) {
    const f = enableForm[nodeId]
    if (!f || !f.scriptId || !f.version) {
      toast.error(t('Select a script and version'))
      return
    }
    const scriptId = Number(f.scriptId)
    const version = Number(f.version)
    const script = pubScripts.find((item) => item.id === scriptId)
    if (script?.category_id) {
      const status = (balanceChecks[nodeId] || []).find(
        (item) => item.category_id === script.category_id
      )
      if (!status?.balance_ok || status.expires_at <= Date.now() / 1000) {
        toast.error(t('Check the site balance before listing this capability'))
        document.getElementById(`balance-checks-${nodeId}`)?.scrollIntoView({
          behavior: 'smooth',
          block: 'center',
        })
        return
      }
    }
    try {
      const test = await createCapabilityTest(nodeId, scriptId, version)
      await enableCapability(nodeId, scriptId, {
        version,
        price_micros: displayToMicros(f.price || '0'),
        daily_limit: Number(f.dailyLimit || '0'),
        test_expires_at: test.test_expires_at,
      })
      toast.success(t('Capability listed'))
      await loadCaps(nodeId)
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  function setForm(nodeId: string, patch: Partial<EnableFormValue>) {
    setEnableForm((p) => {
      const current = p[nodeId] ?? DEFAULT_ENABLE_FORM
      return { ...p, [nodeId]: { ...current, ...patch } }
    })
  }

  async function selectScript(nodeId: string, value: string) {
    if (!value) {
      setForm(nodeId, { scriptId: '', version: '' })
      return
    }
    const scriptId = Number(value)
    try {
      const loadedVersions =
        scriptVersions[scriptId] ||
        (await listAvailableScriptVersions(scriptId))
      const versions = (
        Array.isArray(loadedVersions) ? loadedVersions : []
      ).filter(
        (item) =>
          !categories.some(
            (category) =>
              category.balance_script_id === item.script_id &&
              category.balance_script_version === item.version
          )
      )
      setScriptVersions((current) => ({ ...current, [scriptId]: versions }))
      setForm(nodeId, {
        scriptId: value,
        version: versions[0] ? String(versions[0].version) : '',
      })
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  useEffect(() => {
    loadAll(true)
  }, [])

  useEffect(() => {
    try {
      window.localStorage.setItem(
        getDraftStorageKey(),
        JSON.stringify({
          hideInactive,
          enableForm,
          openNodeIds,
        } satisfies NodesConsoleDraft)
      )
    } catch {
      // Storage may be unavailable or full; the console remains usable without persistence.
    }
  }, [hideInactive, enableForm, openNodeIds])

  async function onRevokeDevice(id: string) {
    // Revoking is irreversible: it kills the device's tokens, takes its node
    // offline, and the browser must re-register as a NEW device. Confirm first.
    if (
      !window.confirm(
        t(
          'Revoke device {{id}}? This is irreversible — its tokens are invalidated and its node goes offline. The browser must re-register as a new device.',
          { id }
        )
      )
    ) {
      return
    }
    try {
      await revokeDevice(id)
      toast.success(t('Device revoked'))
      await loadAll()
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  async function onDeleteDevice(id: string) {
    if (
      !window.confirm(
        t('Permanently delete revoked device {{id}} and its nodes?', { id })
      )
    ) {
      return
    }
    try {
      await deleteDevice(id)
      toast.success(t('Device deleted'))
      await loadAll()
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  async function onDeleteNode(id: string) {
    if (!window.confirm(t('Permanently delete offline node {{id}}?', { id }))) {
      return
    }
    try {
      await deleteNode(id)
      toast.success(t('Node deleted'))
      await loadAll()
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  async function loadCaps(nodeId: string) {
    try {
      const list = await listNodeCapabilities(nodeId)
      setCaps((p) => ({ ...p, [nodeId]: Array.isArray(list) ? list : [] }))
      setOpenNodeIds((current) =>
        current.includes(nodeId) ? current : [...current, nodeId]
      )
      const checks = await listBalanceChecks(nodeId)
      setBalanceChecks((current) => ({
        ...current,
        [nodeId]: Array.isArray(checks) ? checks : [],
      }))
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  function toggleNodeCapabilities(nodeId: string) {
    if (openNodeIds.includes(nodeId)) {
      setOpenNodeIds((current) => current.filter((id) => id !== nodeId))
      return
    }
    void loadCaps(nodeId)
  }

  async function onBalanceCheck(nodeId: string, categoryId: number) {
    const key = `${nodeId}:${categoryId}`
    const previousCheckedAt =
      (balanceChecks[nodeId] || []).find(
        (item) => item.category_id === categoryId
      )?.checked_at ?? 0
    setChecking(key)
    try {
      await requestBalanceCheck(nodeId, categoryId)
      toast.success(t('Balance check sent to the provider plugin'))
      for (let attempt = 0; attempt < 20; attempt += 1) {
        await new Promise((resolve) => window.setTimeout(resolve, 1500))
        const checks = await listBalanceChecks(nodeId)
        const checkList = Array.isArray(checks) ? checks : []
        setBalanceChecks((current) => ({ ...current, [nodeId]: checkList }))
        const result = checkList.find((item) => item.category_id === categoryId)
        if (result && result.checked_at > previousCheckedAt) {
          if (result.balance_ok) {
            toast.success(t('Balance check passed'))
          } else {
            toast.error(result.error_message || t('Balance check failed'))
          }
          return
        }
      }
      toast.error(t('Balance check timed out'))
    } catch (e) {
      toast.error(String((e as Error).message))
    } finally {
      setChecking('')
    }
  }

  async function onRemove(nodeId: string, scriptId: number, version: number) {
    if (
      !window.confirm(
        t('Unlist script #{{scriptId}} v{{version}} from this node?', {
          scriptId,
          version,
        })
      )
    ) {
      return
    }
    try {
      await removeCapability(nodeId, scriptId, version)
      toast.success(t('Capability unlisted'))
      await loadCaps(nodeId)
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  function renderCapabilities(node: NodeInfo) {
    const list = caps[node.id] ?? []
    return (
      <div className='bg-muted/20 space-y-5 border-t p-4 sm:p-5'>
        <section id={`balance-checks-${node.id}`}>
          <div className='mb-3 flex items-center gap-2'>
            <div className='bg-background flex size-7 items-center justify-center rounded-md border'>
              <Layers3 className='size-4' />
            </div>
            <div>
              <h4 className='text-sm font-medium'>
                {t('Site balance checks')}
              </h4>
              <p className='text-muted-foreground text-xs'>
                {t('Verify account balance before listing a capability')}
              </p>
            </div>
          </div>
          <div className='grid gap-2 md:grid-cols-2 xl:grid-cols-3'>
            {categories.map((category) => {
              const status = (balanceChecks[node.id] || []).find(
                (item) => item.category_id === category.id
              )
              const valid = Boolean(
                status?.balance_ok && status.expires_at > Date.now() / 1000
              )
              const key = `${node.id}:${category.id}`
              return (
                <div
                  key={category.id}
                  className='bg-background flex min-w-0 items-center gap-3 rounded-md border p-3'
                >
                  <div className='min-w-0 flex-1'>
                    <div className='truncate text-sm font-medium'>
                      {category.name}
                    </div>
                    <div
                      className={`text-xs ${valid ? 'text-emerald-600' : 'text-muted-foreground'}`}
                    >
                      {valid ? t('Passed') : t('Not checked')}
                    </div>
                    {!valid && status?.error_message && (
                      <div
                        className='mt-1 truncate text-xs text-red-600'
                        title={status.error_message}
                      >
                        {status.error_message}
                      </div>
                    )}
                  </div>
                  <Button
                    size='sm'
                    variant='outline'
                    disabled={!category.balance_script_id || checking === key}
                    onClick={() => onBalanceCheck(node.id, category.id)}
                  >
                    {checking === key ? t('Checking...') : t('Check balance')}
                  </Button>
                </div>
              )
            })}
          </div>
        </section>

        <section>
          <div className='mb-3'>
            <h4 className='text-sm font-medium'>{t('List capability')}</h4>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Select a script version and configure its pricing and daily limit'
              )}
            </p>
          </div>
          <div className='bg-background grid gap-3 rounded-md border p-3 sm:grid-cols-2 xl:grid-cols-[minmax(220px,2fr)_100px_130px_130px_auto]'>
            <label className='text-muted-foreground space-y-1 text-xs'>
              {t('Script')}
              <select
                className='text-foreground h-9 w-full rounded-md border bg-transparent px-2 text-sm'
                value={enableForm[node.id]?.scriptId || ''}
                onChange={(e) => selectScript(node.id, e.target.value)}
              >
                <option value=''>{t('Select a script')}</option>
                {pubScripts.map((s) => (
                  <option key={s.id} value={s.id}>
                    #{s.id} {s.title}
                  </option>
                ))}
              </select>
            </label>
            <label className='text-muted-foreground space-y-1 text-xs'>
              {t('Version')}
              <select
                className='text-foreground h-9 w-full rounded-md border bg-transparent px-2 text-sm'
                disabled={!enableForm[node.id]?.scriptId}
                value={enableForm[node.id]?.version || ''}
                onChange={(e) => setForm(node.id, { version: e.target.value })}
              >
                <option value=''>{t('Version')}</option>
                {(
                  scriptVersions[Number(enableForm[node.id]?.scriptId)] || []
                ).map((version) => (
                  <option key={version.id} value={version.version}>
                    v{version.version}
                  </option>
                ))}
              </select>
            </label>
            <label className='text-muted-foreground space-y-1 text-xs'>
              {t('Price')}
              <Input
                value={enableForm[node.id]?.price ?? ''}
                onChange={(e) => setForm(node.id, { price: e.target.value })}
              />
            </label>
            <label className='text-muted-foreground space-y-1 text-xs'>
              {t('Daily limit')}
              <Input
                value={enableForm[node.id]?.dailyLimit ?? '0'}
                onChange={(e) =>
                  setForm(node.id, { dailyLimit: e.target.value })
                }
              />
            </label>
            <Button
              className='self-end'
              onClick={() => onEnableCapability(node.id)}
            >
              {t('List capability')}
            </Button>
          </div>
        </section>

        <section>
          <div className='mb-3 flex items-center justify-between'>
            <h4 className='text-sm font-medium'>{t('Listed capabilities')}</h4>
            <Badge variant='secondary'>{list.length}</Badge>
          </div>
          <div className='bg-background overflow-x-auto rounded-md border'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Script')}</TableHead>
                  <TableHead>{t('Version')}</TableHead>
                  <TableHead>{t('Price')}</TableHead>
                  <TableHead>{t('Balance')}</TableHead>
                  <TableHead>{t('Today')}</TableHead>
                  <TableHead>{t('Success rate')}</TableHead>
                  <TableHead>{t('Revenue')}</TableHead>
                  <TableHead>{t('Status')}</TableHead>
                  <TableHead className='text-right'>{t('Actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {list.map((c) => {
                  const stat =
                    capStats[`${node.id}:${c.script_id}:${c.version}`]
                  const title = pubScripts.find(
                    (script) => script.id === c.script_id
                  )?.title
                  const rate =
                    stat && stat.executions > 0
                      ? `${Math.round((stat.successes / stat.executions) * 100)}% (${stat.successes}/${stat.executions})`
                      : '-'
                  const daily =
                    c.daily_limit > 0
                      ? `${c.daily_used ?? 0}/${c.daily_limit}`
                      : `${c.daily_used ?? 0}/∞`
                  return (
                    <TableRow key={c.id}>
                      <TableCell className='min-w-48'>
                        #{c.script_id}
                        {title ? ` ${title}` : ''}
                      </TableCell>
                      <TableCell>v{c.version}</TableCell>
                      <TableCell>{microsToCurrency(c.price_micros)}</TableCell>
                      <TableCell>{c.remaining_quota}</TableCell>
                      <TableCell>{daily}</TableCell>
                      <TableCell>{rate}</TableCell>
                      <TableCell>
                        {microsToCurrency(stat?.revenue_micros)}
                      </TableCell>
                      <TableCell>
                        <Badge variant='outline'>{c.status}</Badge>
                      </TableCell>
                      <TableCell className='text-right'>
                        <Button
                          size='icon-sm'
                          variant='ghost'
                          title={t('Unlist')}
                          onClick={() =>
                            onRemove(node.id, c.script_id, c.version)
                          }
                        >
                          <Trash2 className='text-destructive size-4' />
                        </Button>
                      </TableCell>
                    </TableRow>
                  )
                })}
                {list.length === 0 && (
                  <TableRow>
                    <TableCell
                      colSpan={9}
                      className='text-muted-foreground h-20 text-center'
                    >
                      {t('No capabilities')}
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </div>
        </section>
      </div>
    )
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Devices & Nodes')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <label className='text-muted-foreground mr-3 flex items-center gap-1 text-sm'>
          <input
            type='checkbox'
            checked={hideInactive}
            onChange={(e) => setHideInactive(e.target.checked)}
          />
          {t('Hide revoked/offline')}
        </label>
        <Button variant='outline' onClick={() => loadAll()} disabled={loading}>
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        {/* Provider group: all of the caller's nodes belong to this group. */}
        {providerGroup && (
          <div className='mb-4 rounded-lg border p-3'>
            <div className='text-muted-foreground text-xs'>
              {t('Provider group')}
            </div>
            <div className='mt-1 flex flex-wrap items-center gap-x-3 gap-y-1'>
              <span className='text-base font-semibold'>
                {providerGroup.name}
              </span>
              <span className='text-muted-foreground font-mono text-xs'>
                {providerGroup.id}
              </span>
            </div>
            <div className='text-muted-foreground mt-1 text-xs'>
              {t('All your nodes belong to this group by default.')}
            </div>
          </div>
        )}

        {/* Money earned running nodes (provider payable), day/week/month/total. */}
        <div className='mb-6'>
          <div className='mb-2 text-sm font-medium'>
            {t('Provider earnings')}
          </div>
          <EarningsSummary role='provider' refreshKey={refreshTick} />
        </div>

        <div className='mb-3 flex items-end justify-between gap-3'>
          <div>
            <h2 className='font-medium'>{t('Devices & Nodes')}</h2>
            <p className='text-muted-foreground text-sm'>
              {t(
                'Manage each device, its nodes, and the scripts provided by each node'
              )}
            </p>
          </div>
          <div className='text-muted-foreground shrink-0 text-xs'>
            {visibleDevices.length}/{devices.length} {t('Devices')} ·{' '}
            {visibleNodes.length}/{nodes.length} {t('Nodes')}
          </div>
        </div>
        <div className='space-y-4'>
          {visibleDevices.map((device) => {
            const deviceNodes = nodesByDevice[device.id] ?? []
            return (
              <section
                key={device.id}
                className='overflow-hidden rounded-lg border'
              >
                <div className='bg-muted/30 flex flex-col gap-4 p-4 sm:flex-row sm:items-center'>
                  <div className='flex min-w-0 flex-1 items-center gap-3'>
                    <div className='bg-background flex size-10 shrink-0 items-center justify-center rounded-md border'>
                      <Server className='size-5' />
                    </div>
                    <div className='min-w-0'>
                      <div className='flex flex-wrap items-center gap-2'>
                        <h3 className='font-medium'>
                          {device.name || t('Unnamed device')}
                        </h3>
                        <Badge
                          variant={
                            device.status === 'active' ? 'default' : 'secondary'
                          }
                        >
                          {device.status}
                        </Badge>
                      </div>
                      <div
                        className='text-muted-foreground mt-1 truncate font-mono text-xs'
                        title={device.id}
                      >
                        {device.id}
                      </div>
                    </div>
                  </div>
                  <div className='text-muted-foreground flex items-center gap-6 text-xs'>
                    <div>
                      <span className='block'>{t('Nodes')}</span>
                      <strong className='text-foreground text-sm'>
                        {deviceNodes.length}
                      </strong>
                    </div>
                    <div>
                      <span className='block'>{t('Last Seen')}</span>
                      <span className='text-foreground text-sm'>
                        {formatUnix(device.last_seen_at)}
                      </span>
                    </div>
                  </div>
                  {device.status === 'active' ? (
                    <Button
                      size='sm'
                      variant='outline'
                      onClick={() => onRevokeDevice(device.id)}
                    >
                      <Ban className='size-4' />
                      {t('Revoke')}
                    </Button>
                  ) : (
                    <Button
                      size='sm'
                      variant='outline'
                      onClick={() => onDeleteDevice(device.id)}
                    >
                      <Trash2 className='size-4' />
                      {t('Delete')}
                    </Button>
                  )}
                </div>
                <div className='divide-y border-t'>
                  {deviceNodes.map((node) => {
                    const open = openNodeIds.includes(node.id)
                    return (
                      <div key={node.id}>
                        <div className='flex flex-col gap-3 p-4 sm:flex-row sm:items-center'>
                          <div className='flex min-w-0 flex-1 items-center gap-3 sm:pl-3'>
                            <Cpu className='text-muted-foreground size-5 shrink-0' />
                            <div className='min-w-0'>
                              <div className='flex items-center gap-2'>
                                <span className='text-sm font-medium'>
                                  {t('Node')}
                                </span>
                                <span
                                  className={`size-2 rounded-full ${nodeOnline(node) ? 'bg-emerald-500' : 'bg-muted-foreground/50'}`}
                                />
                                <span className='text-muted-foreground text-xs'>
                                  {nodeOnline(node)
                                    ? t('Online')
                                    : t('Offline')}
                                </span>
                              </div>
                              <div
                                className='text-muted-foreground truncate font-mono text-xs'
                                title={node.id}
                              >
                                {node.id}
                              </div>
                            </div>
                          </div>
                          <div className='text-muted-foreground flex flex-wrap items-center gap-x-5 gap-y-1 text-xs'>
                            <span>{node.region || t('No region')}</span>
                            <Badge variant='outline'>{node.state}</Badge>
                            <span>{formatUnix(node.last_seen_at)}</span>
                          </div>
                          <div className='flex items-center gap-1'>
                            {!nodeOnline(node) && (
                              <Button
                                size='icon-sm'
                                variant='ghost'
                                title={t('Delete')}
                                onClick={() => onDeleteNode(node.id)}
                              >
                                <Trash2 className='size-4' />
                              </Button>
                            )}
                            <Button
                              size='sm'
                              variant={open ? 'secondary' : 'outline'}
                              aria-expanded={open}
                              onClick={() => toggleNodeCapabilities(node.id)}
                            >
                              {t('Capabilities')}
                              <Badge variant='secondary'>
                                {caps[node.id]?.length ?? 0}
                              </Badge>
                              <ChevronDown
                                className={`size-4 transition-transform ${open ? 'rotate-180' : ''}`}
                              />
                            </Button>
                          </div>
                        </div>
                        {open && renderCapabilities(node)}
                      </div>
                    )
                  })}
                  {deviceNodes.length === 0 && (
                    <div className='text-muted-foreground p-6 text-center text-sm'>
                      {t('No nodes on this device')}
                    </div>
                  )}
                </div>
              </section>
            )
          })}
          {ungroupedNodes.map((node) => (
            <section
              key={node.id}
              className='overflow-hidden rounded-lg border'
            >
              <div className='flex items-center gap-3 p-4'>
                <Cpu className='size-5' />
                <div className='min-w-0 flex-1'>
                  <div className='text-sm font-medium'>{t('Node')}</div>
                  <div className='text-muted-foreground truncate font-mono text-xs'>
                    {node.id}
                  </div>
                </div>
                <Button
                  size='sm'
                  variant='outline'
                  onClick={() => toggleNodeCapabilities(node.id)}
                >
                  {t('Capabilities')}
                  <ChevronDown
                    className={`size-4 ${openNodeIds.includes(node.id) ? 'rotate-180' : ''}`}
                  />
                </Button>
              </div>
              {openNodeIds.includes(node.id) && renderCapabilities(node)}
            </section>
          ))}
          {visibleDevices.length === 0 && ungroupedNodes.length === 0 && (
            <div className='text-muted-foreground rounded-lg border border-dashed p-10 text-center'>
              {loading ? t('Loading...') : t('No devices')}
            </div>
          )}
        </div>

        {visibleDevices.length < 0 && (
          <>
            <div className='mb-2 text-sm font-medium'>
              {t('Devices')} ({visibleDevices.length}/{devices.length})
            </div>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>ID</TableHead>
                  <TableHead>{t('Name')}</TableHead>
                  <TableHead>{t('Status')}</TableHead>
                  <TableHead>{t('Last Seen')}</TableHead>
                  <TableHead>{t('Actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {visibleDevices.map((d) => (
                  <TableRow key={d.id}>
                    <TableCell className='max-w-[180px] truncate font-mono text-xs'>
                      {d.id}
                    </TableCell>
                    <TableCell>{d.name}</TableCell>
                    <TableCell>
                      {d.status === 'active' ? '🟢 active' : '⛔ revoked'}
                    </TableCell>
                    <TableCell>{formatUnix(d.last_seen_at)}</TableCell>
                    <TableCell className='space-x-2'>
                      {d.status === 'active' ? (
                        <Button
                          size='sm'
                          variant='destructive'
                          onClick={() => onRevokeDevice(d.id)}
                        >
                          {t('Revoke')}
                        </Button>
                      ) : (
                        <Button
                          size='sm'
                          variant='outline'
                          onClick={() => onDeleteDevice(d.id)}
                        >
                          {t('Delete')}
                        </Button>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
                {visibleDevices.length === 0 && (
                  <TableRow>
                    <TableCell
                      colSpan={5}
                      className='text-muted-foreground text-center'
                    >
                      {loading ? t('Loading...') : t('No devices')}
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>

            <div className='mt-6 mb-2 text-sm font-medium'>
              {t('Nodes')} ({visibleNodes.length}/{nodes.length})
            </div>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>ID</TableHead>
                  <TableHead>{t('State')}</TableHead>
                  <TableHead>{t('Online')}</TableHead>
                  <TableHead>{t('Region')}</TableHead>
                  <TableHead>{t('Last Seen')}</TableHead>
                  <TableHead>{t('Actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {visibleNodes.map((n) => (
                  <TableRow key={n.id}>
                    <TableCell className='max-w-[180px] truncate font-mono text-xs'>
                      {n.id}
                    </TableCell>
                    <TableCell>{n.state}</TableCell>
                    <TableCell>{nodeOnline(n) ? '🟢' : '⚪'}</TableCell>
                    <TableCell>{n.region || '-'}</TableCell>
                    <TableCell>{formatUnix(n.last_seen_at)}</TableCell>
                    <TableCell className='space-x-2'>
                      <Button
                        size='sm'
                        variant='ghost'
                        onClick={() => loadCaps(n.id)}
                      >
                        {t('Capabilities')}
                      </Button>
                      {!nodeOnline(n) && (
                        <Button
                          size='sm'
                          variant='outline'
                          onClick={() => onDeleteNode(n.id)}
                        >
                          {t('Delete')}
                        </Button>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
                {visibleNodes.length === 0 && (
                  <TableRow>
                    <TableCell
                      colSpan={6}
                      className='text-muted-foreground text-center'
                    >
                      {loading ? t('Loading...') : t('No nodes')}
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>

            {Object.entries(caps).map(([nodeId, list]) => (
              <div key={nodeId} className='mt-6'>
                <div className='mb-2 text-sm font-medium'>
                  {t('Capabilities of node {{id}}', { id: nodeId })}
                </div>

                <div
                  id={`balance-checks-${nodeId}`}
                  className='mb-3 border-y py-3'
                >
                  <div className='mb-2 text-sm font-medium'>
                    {t('Site balance checks')}
                  </div>
                  <div className='flex flex-wrap gap-2'>
                    {categories.map((category) => {
                      const status = (balanceChecks[nodeId] || []).find(
                        (item) => item.category_id === category.id
                      )
                      const valid = Boolean(
                        status?.balance_ok &&
                        status.expires_at > Date.now() / 1000
                      )
                      const key = `${nodeId}:${category.id}`
                      return (
                        <div
                          key={category.id}
                          className='flex items-center gap-2 border px-3 py-2 text-sm'
                        >
                          <span>{category.name}</span>
                          <span
                            className={
                              valid
                                ? 'text-emerald-600'
                                : 'text-muted-foreground'
                            }
                          >
                            {valid ? t('Passed') : t('Not checked')}
                          </span>
                          <Button
                            size='sm'
                            variant='outline'
                            disabled={
                              !category.balance_script_id || checking === key
                            }
                            onClick={() => onBalanceCheck(nodeId, category.id)}
                          >
                            {checking === key
                              ? t('Checking...')
                              : t('Check balance')}
                          </Button>
                          {!valid && status?.error_message && (
                            <span className='max-w-80 text-xs text-red-600'>
                              {status.error_message}
                            </span>
                          )}
                        </div>
                      )
                    })}
                  </div>
                </div>

                {/* List a capability: pick a published script + version, set price and
                daily quota, then enable (requires the category balance check). */}
                <div className='mb-3 flex flex-wrap items-center gap-2 rounded-lg border p-3'>
                  <span className='text-muted-foreground text-xs'>
                    {t('List capability')}:
                  </span>
                  <select
                    className='h-9 min-w-[200px] rounded-md border px-2 text-sm'
                    value={enableForm[nodeId]?.scriptId || ''}
                    onChange={(e) => selectScript(nodeId, e.target.value)}
                  >
                    <option value=''>{t('Select a script')}</option>
                    {pubScripts.map((s) => (
                      <option key={s.id} value={s.id}>
                        #{s.id} {s.title}
                      </option>
                    ))}
                  </select>
                  <select
                    className='h-9 w-24 rounded-md border px-2 text-sm'
                    disabled={!enableForm[nodeId]?.scriptId}
                    value={enableForm[nodeId]?.version || ''}
                    onChange={(e) =>
                      setForm(nodeId, { version: e.target.value })
                    }
                  >
                    <option value=''>{t('Version')}</option>
                    {(
                      scriptVersions[Number(enableForm[nodeId]?.scriptId)] || []
                    ).map((version) => (
                      <option key={version.id} value={version.version}>
                        v{version.version}
                      </option>
                    ))}
                  </select>
                  <Input
                    className='w-28'
                    placeholder={t('Price')}
                    value={enableForm[nodeId]?.price ?? ''}
                    onChange={(e) => setForm(nodeId, { price: e.target.value })}
                  />
                  <Input
                    className='w-24'
                    placeholder={t('Daily limit')}
                    value={enableForm[nodeId]?.dailyLimit ?? '0'}
                    onChange={(e) =>
                      setForm(nodeId, { dailyLimit: e.target.value })
                    }
                  />
                  <Button size='sm' onClick={() => onEnableCapability(nodeId)}>
                    {t('List capability')}
                  </Button>
                </div>

                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('Script')}</TableHead>
                      <TableHead>{t('Version')}</TableHead>
                      <TableHead>{t('Price')}</TableHead>
                      <TableHead>{t('Balance')}</TableHead>
                      <TableHead>{t('Today')}</TableHead>
                      <TableHead>{t('Success rate')}</TableHead>
                      <TableHead>{t('Revenue')}</TableHead>
                      <TableHead>{t('Status')}</TableHead>
                      <TableHead>{t('Actions')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {list.map((c) => {
                      const stat =
                        capStats[`${nodeId}:${c.script_id}:${c.version}`]
                      const scriptTitle = pubScripts.find(
                        (script) => script.id === c.script_id
                      )?.title
                      const rate =
                        stat && stat.executions > 0
                          ? `${Math.round((stat.successes / stat.executions) * 100)}% (${stat.successes}/${stat.executions})`
                          : '-'
                      // Daily usage: "used / limit" or "used / ∞" when limit is 0.
                      const dailyUsed = c.daily_used ?? 0
                      const dailyLimit = c.daily_limit ?? 0
                      const dailyDisplay =
                        dailyLimit > 0
                          ? `${dailyUsed}/${dailyLimit}`
                          : `${dailyUsed}/∞`
                      return (
                        <TableRow key={c.id}>
                          <TableCell>
                            #{c.script_id}
                            {scriptTitle ? ` ${scriptTitle}` : ''}
                          </TableCell>
                          <TableCell>v{c.version}</TableCell>
                          <TableCell>
                            {microsToCurrency(c.price_micros)}
                          </TableCell>
                          <TableCell
                            title={t('Balance from last execution result')}
                          >
                            {c.remaining_quota}
                          </TableCell>
                          <TableCell
                            title={t(
                              'Executions today / daily limit (resets at midnight CST)'
                            )}
                          >
                            {dailyDisplay}
                          </TableCell>
                          <TableCell>{rate}</TableCell>
                          <TableCell>
                            {microsToCurrency(stat?.revenue_micros)}
                          </TableCell>
                          <TableCell>{c.status}</TableCell>
                          <TableCell>
                            <Button
                              size='sm'
                              variant='destructive'
                              onClick={() =>
                                onRemove(nodeId, c.script_id, c.version)
                              }
                            >
                              {t('Unlist')}
                            </Button>
                          </TableCell>
                        </TableRow>
                      )
                    })}
                    {list.length === 0 && (
                      <TableRow>
                        <TableCell
                          colSpan={9}
                          className='text-muted-foreground text-center'
                        >
                          {t('No capabilities')}
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              </div>
            ))}
          </>
        )}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
