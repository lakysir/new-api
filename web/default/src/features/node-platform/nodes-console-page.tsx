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
import {
  Ban,
  Cpu,
  Download,
  ExternalLink,
  History,
  Settings2,
  Trash2,
} from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
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
  getLatestPluginRelease,
  getMyProviderGroup,
  listAvailableScriptVersions,
  listBalanceChecks,
  listCategories,
  listMyDevices,
  listMyNodes,
  listMyTaskAttempts,
  listNodeCapabilities,
  listProviderCapabilityStats,
  PLUGIN_DOWNLOAD_URL,
  removeCapability,
  requestBalanceCheck,
  revokeDevice,
  setNodeEnabled,
  type NodeBalanceCheck,
  type PluginRelease,
  type ProviderGroup,
  type ScriptCategory,
} from './api'
import { EarningsSummary } from './earnings-summary'
import { displayToMicros, formatUnix, microsToCurrency } from './lib/format'
import type {
  CapabilityStat,
  Device,
  NodeCapability,
  NodeInfo,
  ProviderTaskAttempt,
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
  enableFormDefaultsVersion: number
  hideInactive: boolean
  enableForm: Record<string, EnableFormValue>
  openNodeIds: string[]
}

const DEFAULT_ENABLE_FORM: EnableFormValue = {
  scriptId: '',
  version: '1',
  price: '0.1',
  dailyLimit: '100',
}

const ENABLE_FORM_DEFAULTS_VERSION = 1

function getDraftStorageKey() {
  const userId = window.localStorage.getItem('uid') ?? 'anonymous'
  return `nodes-console-draft:${userId}`
}

function loadNodesConsoleDraft(): NodesConsoleDraft {
  try {
    const saved = JSON.parse(
      window.localStorage.getItem(getDraftStorageKey()) ?? '{}'
    ) as Partial<NodesConsoleDraft>
    const usesCurrentDefaults =
      saved.enableFormDefaultsVersion === ENABLE_FORM_DEFAULTS_VERSION
    const enableForm = Object.fromEntries(
      Object.entries(saved.enableForm ?? {}).flatMap(([nodeId, value]) => {
        if (!value || typeof value !== 'object') return []
        const form = value as Partial<EnableFormValue>
        let price = form.price || DEFAULT_ENABLE_FORM.price
        let dailyLimit = form.dailyLimit || DEFAULT_ENABLE_FORM.dailyLimit
        if (usesCurrentDefaults) {
          if (typeof form.price === 'string') price = form.price
          if (typeof form.dailyLimit === 'string') dailyLimit = form.dailyLimit
        } else if (form.dailyLimit === '0') {
          dailyLimit = DEFAULT_ENABLE_FORM.dailyLimit
        }
        return [
          [
            nodeId,
            {
              scriptId: typeof form.scriptId === 'string' ? form.scriptId : '',
              version: typeof form.version === 'string' ? form.version : '',
              price,
              dailyLimit,
            },
          ] as const,
        ]
      })
    ) as Record<string, EnableFormValue>
    return {
      enableFormDefaultsVersion: ENABLE_FORM_DEFAULTS_VERSION,
      hideInactive:
        typeof saved.hideInactive === 'boolean' ? saved.hideInactive : true,
      enableForm,
      openNodeIds: Array.isArray(saved.openNodeIds)
        ? saved.openNodeIds.filter((id): id is string => typeof id === 'string')
        : [],
    }
  } catch {
    return {
      enableFormDefaultsVersion: ENABLE_FORM_DEFAULTS_VERSION,
      hideInactive: true,
      enableForm: {},
      openNodeIds: [],
    }
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
  // Node id currently being toggled on/off, to disable its switch mid-flight.
  const [togglingNodeId, setTogglingNodeId] = useState('')
  // Per-(node, script, version) execution stats, keyed "nodeId:scriptId:version".
  const [capStats, setCapStats] = useState<Record<string, CapabilityStat>>({})
  // Bumped on each loadAll() so the provider earnings cards refetch.
  const [refreshTick, setRefreshTick] = useState(0)
  // Per-node enable form: script id + version + price + quota.
  const [enableForm, setEnableForm] = useState<Record<string, EnableFormValue>>(
    initialDraft.enableForm
  )
  const [openNodeIds, setOpenNodeIds] = useState(initialDraft.openNodeIds)
  const [capabilityNodeId, setCapabilityNodeId] = useState<string | null>(null)
  // The caller's provider group (all their nodes belong to it). Created on first
  // load from the username; every node defaults into this group.
  const [providerGroup, setProviderGroup] = useState<ProviderGroup | null>(null)
  // Per-node execution records (task attempts), shown in a dialog so the console
  // stays uncluttered. Params/results are E2EE and not stored server-side; this
  // is the most the control plane can show for debugging.
  const [recordsOpen, setRecordsOpen] = useState(false)
  const [taskAttempts, setTaskAttempts] = useState<ProviderTaskAttempt[]>([])
  const [attemptsLoading, setAttemptsLoading] = useState(false)
  // Latest published extension release, if any. Backs the "Download plugin"
  // button so it only appears once the operator has uploaded a release.
  const [pluginRelease, setPluginRelease] = useState<PluginRelease | null>(null)
  const [pluginDialogOpen, setPluginDialogOpen] = useState(false)

  useEffect(() => {
    getLatestPluginRelease()
      .then((release) => {
        if (release.available) setPluginRelease(release)
      })
      .catch(() => {})
  }, [])

  async function loadTaskAttempts() {
    setAttemptsLoading(true)
    try {
      const list = await listMyTaskAttempts()
      setTaskAttempts(Array.isArray(list) ? list : [])
    } catch (e) {
      toast.error(String((e as Error).message))
    } finally {
      setAttemptsLoading(false)
    }
  }

  function openTaskRecords() {
    setRecordsOpen(true)
    void loadTaskAttempts()
  }

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
            let version = ''
            if (
              versions.some((item) => String(item.version) === form.version)
            ) {
              version = form.version
            } else if (versions[0]) {
              version = String(versions[0].version)
            }
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
    // Listing is unconditional now: the provider lists the script first, runs
    // the per-capability balance check from the listed row, then enables the
    // node once all its capabilities pass.
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    const refreshNodes = () => {
      void listMyNodes()
        .then((list) => setNodes(Array.isArray(list) ? list : []))
        .catch(() => {})
    }
    const timer = window.setInterval(refreshNodes, 3000)
    return () => window.clearInterval(timer)
  }, [])

  useEffect(() => {
    try {
      window.localStorage.setItem(
        getDraftStorageKey(),
        JSON.stringify({
          enableFormDefaultsVersion: ENABLE_FORM_DEFAULTS_VERSION,
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
    setCapabilityNodeId(nodeId)
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

  // capabilityBalanceOk reports whether a capability's category balance check is
  // passing and unexpired. Capabilities with no category (category_id 0) need no
  // check and are always OK.
  function capabilityBalanceOk(nodeId: string, categoryId: number): boolean {
    if (!categoryId) return true
    const status = (balanceChecks[nodeId] || []).find(
      (item) => item.category_id === categoryId
    )
    return Boolean(status?.balance_ok && status.expires_at > Date.now() / 1000)
  }

  // nodeCanEnable reports whether every active capability on the node has a
  // passing balance check (and there is at least one), mirroring the server-side
  // gate so the switch is only offered when enabling would succeed.
  function nodeCanEnable(nodeId: string): boolean {
    const list = (caps[nodeId] ?? []).filter((c) => c.status === 'active')
    if (list.length === 0) return false
    return list.every((c) => capabilityBalanceOk(nodeId, c.category_id))
  }

  function nodeToggleTitle(node: NodeInfo, capabilitiesLoaded: boolean) {
    if (node.enabled) return t('Enabled: this node can be scheduled')
    if (!capabilitiesLoaded) {
      return t('Open capabilities and pass every balance check to enable')
    }
    if (!nodeCanEnable(node.id)) {
      return t(
        'All listed capabilities must pass their balance check before enabling'
      )
    }
    return t('Enable this node for scheduling')
  }

  async function onToggleEnabled(node: NodeInfo, next: boolean) {
    setTogglingNodeId(node.id)
    try {
      await setNodeEnabled(node.id, next)
      setNodes((current) =>
        current.map((n) => (n.id === node.id ? { ...n, enabled: next } : n))
      )
      toast.success(next ? t('Node enabled') : t('Node disabled'))
    } catch (e) {
      toast.error(String((e as Error).message))
    } finally {
      setTogglingNodeId('')
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
      <div className='space-y-5'>
        <section>
          <div className='mb-3'>
            <h4 className='text-sm font-medium'>{t('List capability')}</h4>
            <p className='text-muted-foreground text-xs'>
              {t(
                'List the script first, then run its balance check from the row below'
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
                value={enableForm[node.id]?.price ?? DEFAULT_ENABLE_FORM.price}
                onChange={(e) => setForm(node.id, { price: e.target.value })}
              />
            </label>
            <label className='text-muted-foreground space-y-1 text-xs'>
              {t('Daily limit')}
              <Input
                value={
                  enableForm[node.id]?.dailyLimit ??
                  DEFAULT_ENABLE_FORM.dailyLimit
                }
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
                  <TableHead>{t('Concurrency')}</TableHead>
                  <TableHead>{t('Price')}</TableHead>
                  <TableHead>{t('Balance')}</TableHead>
                  <TableHead>{t('Today')}</TableHead>
                  <TableHead>{t('Success rate')}</TableHead>
                  <TableHead>{t('Revenue')}</TableHead>
                  <TableHead>{t('Balance check')}</TableHead>
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
                  // Per-capability balance check: probes the script's category.
                  // Capabilities with no category need no check.
                  const checkStatus = (balanceChecks[node.id] || []).find(
                    (item) => item.category_id === c.category_id
                  )
                  const checkValid = capabilityBalanceOk(node.id, c.category_id)
                  const checkKey = `${node.id}:${c.category_id}`
                  let checkClassName = 'text-muted-foreground'
                  let checkLabel = t('Not checked')
                  if (checkValid) {
                    checkClassName = 'text-emerald-600'
                    checkLabel = t('Passed')
                  } else if (checkStatus && !checkStatus.balance_ok) {
                    checkClassName = 'text-red-600'
                    checkLabel = t('Failed')
                  }
                  return (
                    <TableRow key={c.id}>
                      <TableCell className='min-w-48'>
                        #{c.script_id}
                        {title ? ` ${title}` : ''}
                      </TableCell>
                      <TableCell>v{c.version}</TableCell>
                      <TableCell>{c.concurrency ?? 1}</TableCell>
                      <TableCell>{microsToCurrency(c.price_micros)}</TableCell>
                      <TableCell>{c.remaining_quota}</TableCell>
                      <TableCell>{daily}</TableCell>
                      <TableCell>{rate}</TableCell>
                      <TableCell>
                        {microsToCurrency(stat?.revenue_micros)}
                      </TableCell>
                      <TableCell>
                        {c.category_id ? (
                          <div className='flex items-center gap-2'>
                            <Button
                              size='sm'
                              variant='outline'
                              disabled={checking === checkKey}
                              onClick={() =>
                                onBalanceCheck(node.id, c.category_id)
                              }
                            >
                              {checking === checkKey
                                ? t('Checking...')
                                : t('Detect')}
                            </Button>
                            <span
                              className={`text-xs ${checkClassName}`}
                              title={
                                !checkValid
                                  ? checkStatus?.error_message
                                  : undefined
                              }
                            >
                              {checkLabel}
                            </span>
                          </div>
                        ) : (
                          <span className='text-muted-foreground text-xs'>
                            {t('N/A')}
                          </span>
                        )}
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
                      colSpan={11}
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
        <Button
          variant='outline'
          render={
            <a href='/my-scripts' target='_blank' rel='noopener noreferrer' />
          }
        >
          {t('My Scripts')}
          <ExternalLink className='size-4' />
        </Button>
        {pluginRelease !== null && (
          <Button
            variant='outline'
            onClick={() => setPluginDialogOpen(true)}
          >
            <Download className='size-4' />
            {t('Download plugin')}
            {pluginRelease.version && (
              <Badge variant='secondary'>v{pluginRelease.version}</Badge>
            )}
          </Button>
        )}
        <label className='text-muted-foreground mr-3 flex items-center gap-1 text-sm'>
          <input
            type='checkbox'
            checked={hideInactive}
            onChange={(e) => setHideInactive(e.target.checked)}
          />
          {t('Hide revoked/offline')}
        </label>
        <Button variant='outline' onClick={openTaskRecords}>
          <History className='size-4' />
          {t('Task records')}
        </Button>
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
        <div className='overflow-hidden rounded-md border'>
          {visibleDevices.map((device) => {
            const node = nodesByDevice[device.id]?.[0]
            const capabilitiesLoaded = node
              ? openNodeIds.includes(node.id)
              : false
            return (
              <div
                key={device.id}
                className='hover:bg-muted/20 flex min-h-14 flex-col gap-2 border-b px-3 py-2 last:border-b-0 lg:flex-row lg:items-center lg:gap-4'
              >
                <div className='flex min-w-0 flex-1 items-center gap-2.5'>
                  <Cpu className='text-muted-foreground size-4 shrink-0' />
                  <div className='min-w-0 flex-1 lg:max-w-72'>
                    <div className='flex min-w-0 items-center gap-2'>
                      <h3 className='truncate text-sm font-medium'>
                        {device.name || t('Unnamed device')}
                      </h3>
                      <span
                        className={`size-2 shrink-0 rounded-full ${node && nodeOnline(node) ? 'bg-emerald-500' : 'bg-muted-foreground/40'}`}
                      />
                      <span className='text-muted-foreground shrink-0 text-xs'>
                        {node && nodeOnline(node) ? t('Online') : t('Offline')}
                      </span>
                    </div>
                    <div
                      className='text-muted-foreground truncate font-mono text-[11px] leading-4'
                      title={`${device.id}${node ? ` / ${node.id}` : ''}`}
                    >
                      {device.id}
                      {node ? ` / ${node.id}` : ''}
                    </div>
                  </div>
                </div>

                <div className='text-muted-foreground flex min-w-0 flex-wrap items-center gap-x-4 gap-y-1 text-xs lg:w-[360px] lg:flex-nowrap'>
                  <Badge variant='outline' className='h-5 max-w-28 truncate'>
                    {node?.region || t('No region')}
                  </Badge>
                  <span className='min-w-0 truncate'>{node?.state || '-'}</span>
                  <span className='shrink-0'>
                    {formatUnix(node?.last_seen_at ?? device.last_seen_at)}
                  </span>
                </div>

                <div className='flex items-center justify-between gap-2 lg:justify-end'>
                  {node && (
                    <label
                      className='flex items-center gap-1.5 text-xs'
                      title={nodeToggleTitle(node, capabilitiesLoaded)}
                    >
                      <Switch
                        size='sm'
                        checked={node.enabled}
                        disabled={
                          togglingNodeId === node.id ||
                          (!node.enabled &&
                            (!capabilitiesLoaded || !nodeCanEnable(node.id)))
                        }
                        onCheckedChange={(next) => onToggleEnabled(node, next)}
                      />
                      <span
                        className={
                          node.enabled
                            ? 'text-emerald-600'
                            : 'text-muted-foreground'
                        }
                      >
                        {node.enabled ? t('Enabled') : t('Disabled')}
                      </span>
                    </label>
                  )}
                  {node && (
                    <Button
                      size='sm'
                      variant='outline'
                      onClick={() => toggleNodeCapabilities(node.id)}
                    >
                      <Settings2 className='size-4' />
                      {t('Capabilities')}
                      {capabilitiesLoaded && (
                        <Badge variant='secondary'>
                          {caps[node.id]?.length ?? 0}
                        </Badge>
                      )}
                    </Button>
                  )}
                  {!node && (
                    <span className='text-muted-foreground text-xs'>
                      {t('No nodes on this device')}
                    </span>
                  )}
                  {node && !nodeOnline(node) && (
                    <Button
                      size='icon-sm'
                      variant='ghost'
                      title={t('Delete')}
                      onClick={() => onDeleteNode(node.id)}
                    >
                      <Trash2 className='size-4' />
                    </Button>
                  )}
                  {device.status === 'active' ? (
                    <Button
                      size='icon-sm'
                      variant='ghost'
                      title={t('Revoke')}
                      onClick={() => onRevokeDevice(device.id)}
                    >
                      <Ban className='size-4' />
                    </Button>
                  ) : (
                    <Button
                      size='icon-sm'
                      variant='ghost'
                      title={t('Delete')}
                      onClick={() => onDeleteDevice(device.id)}
                    >
                      <Trash2 className='size-4' />
                    </Button>
                  )}
                </div>
              </div>
            )
          })}
          {ungroupedNodes.map((node) => (
            <div
              key={node.id}
              className='hover:bg-muted/20 flex min-h-14 items-center gap-3 border-b px-3 py-2 last:border-b-0'
            >
              <Cpu className='text-muted-foreground size-4 shrink-0' />
              <div className='min-w-0 flex-1'>
                <div className='flex items-center gap-2 text-sm font-medium'>
                  {t('Node')}
                  <span
                    className={`size-2 rounded-full ${nodeOnline(node) ? 'bg-emerald-500' : 'bg-muted-foreground/40'}`}
                  />
                </div>
                <div
                  className='text-muted-foreground truncate font-mono text-[11px]'
                  title={node.id}
                >
                  {node.id}
                </div>
              </div>
              <Button
                size='sm'
                variant='outline'
                onClick={() => toggleNodeCapabilities(node.id)}
              >
                <Settings2 className='size-4' />
                {t('Capabilities')}
              </Button>
            </div>
          ))}
          {visibleDevices.length === 0 && ungroupedNodes.length === 0 && (
            <div className='text-muted-foreground p-10 text-center'>
              {loading ? t('Loading...') : t('No devices')}
            </div>
          )}
        </div>

        <Dialog
          open={Boolean(capabilityNodeId)}
          onOpenChange={(open) => !open && setCapabilityNodeId(null)}
        >
          <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-[min(1200px,calc(100vw-2rem))]'>
            <DialogHeader className='pr-8'>
              <DialogTitle>{t('Capabilities')}</DialogTitle>
              <DialogDescription className='truncate font-mono text-xs'>
                {capabilityNodeId}
              </DialogDescription>
            </DialogHeader>
            {capabilityNodeId &&
              (() => {
                const node = nodes.find((item) => item.id === capabilityNodeId)
                return node ? renderCapabilities(node) : null
              })()}
          </DialogContent>
        </Dialog>

        {/* Task records: per-node execution attempts (success/failure) across
            all the caller's nodes. Params/results are E2EE and never stored, so
            this shows the state, failure reason and target-site balance the
            control plane does have — enough to debug node behavior. */}
        <Dialog open={recordsOpen} onOpenChange={setRecordsOpen}>
          <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-[min(1200px,calc(100vw-2rem))]'>
            <DialogHeader className='pr-8'>
              <DialogTitle>{t('Task records')}</DialogTitle>
              <DialogDescription>
                {t(
                  'Recent task executions on your nodes. Parameters and results are end-to-end encrypted and not stored — this shows the outcome and failure reason for debugging.'
                )}
              </DialogDescription>
            </DialogHeader>
            <div className='flex items-center justify-between'>
              <span className='text-muted-foreground text-xs'>
                {t('{{count}} records', { count: taskAttempts.length })}
              </span>
              <Button
                size='sm'
                variant='outline'
                onClick={loadTaskAttempts}
                disabled={attemptsLoading}
              >
                {attemptsLoading ? t('Loading...') : t('Refresh')}
              </Button>
            </div>
            <div className='overflow-x-auto rounded-md border'>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('Time')}</TableHead>
                    <TableHead>{t('Node')}</TableHead>
                    <TableHead>{t('Script')}</TableHead>
                    <TableHead>{t('Status')}</TableHead>
                    <TableHead>{t('Reason')}</TableHead>
                    <TableHead>{t('Balance')}</TableHead>
                    <TableHead>{t('Task')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {taskAttempts.map((a) => {
                    let stateClass = 'text-muted-foreground'
                    if (a.state === 'SUCCEEDED') {
                      stateClass = 'text-emerald-600'
                    } else if (a.state === 'FAILED' || a.state === 'EXPIRED') {
                      stateClass = 'text-red-600'
                    }
                    return (
                      <TableRow key={`${a.task_id}:${a.attempt}`}>
                        <TableCell className='text-xs whitespace-nowrap'>
                          {formatUnix(a.updated_at || a.created_at)}
                        </TableCell>
                        <TableCell className='max-w-[160px] truncate font-mono text-xs'>
                          {a.node_id}
                        </TableCell>
                        <TableCell className='whitespace-nowrap'>
                          #{a.script_id} v{a.version}
                        </TableCell>
                        <TableCell>
                          <span className={`text-xs font-medium ${stateClass}`}>
                            {a.state}
                          </span>
                        </TableCell>
                        <TableCell className='text-xs text-red-600'>
                          {a.error_code || '-'}
                        </TableCell>
                        <TableCell className='text-xs'>
                          {a.script_balance ?? '-'}
                        </TableCell>
                        <TableCell className='max-w-[160px] truncate font-mono text-xs'>
                          {a.task_id}
                        </TableCell>
                      </TableRow>
                    )
                  })}
                  {taskAttempts.length === 0 && (
                    <TableRow>
                      <TableCell
                        colSpan={7}
                        className='text-muted-foreground h-20 text-center'
                      >
                        {attemptsLoading
                          ? t('Loading...')
                          : t('No task records yet')}
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </div>
          </DialogContent>
        </Dialog>

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
                    value={
                      enableForm[nodeId]?.price ?? DEFAULT_ENABLE_FORM.price
                    }
                    onChange={(e) => setForm(nodeId, { price: e.target.value })}
                  />
                  <Input
                    className='w-24'
                    placeholder={t('Daily limit')}
                    value={
                      enableForm[nodeId]?.dailyLimit ??
                      DEFAULT_ENABLE_FORM.dailyLimit
                    }
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
                      <TableHead>{t('Concurrency')}</TableHead>
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
                          <TableCell>{c.concurrency ?? 1}</TableCell>
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

      {/* Plugin download dialog: shows version, release notes, and download link */}
      <Dialog open={pluginDialogOpen} onOpenChange={setPluginDialogOpen}>
        <DialogContent className='max-w-md'>
          <DialogHeader>
            <DialogTitle>
              {t('Browser Plugin')}
              {pluginRelease?.version && (
                <Badge variant='secondary' className='ml-2'>
                  v{pluginRelease.version}
                </Badge>
              )}
            </DialogTitle>
            {pluginRelease?.release_notes ? (
              <DialogDescription className='whitespace-pre-wrap text-left'>
                {pluginRelease.release_notes}
              </DialogDescription>
            ) : (
              <DialogDescription>
                {t('Download the latest browser extension package.')}
              </DialogDescription>
            )}
          </DialogHeader>
          <div className='flex justify-end pt-2'>
            <Button
              render={
                <a
                  href={PLUGIN_DOWNLOAD_URL}
                  target='_blank'
                  rel='noopener noreferrer'
                />
              }
              onClick={() => setPluginDialogOpen(false)}
            >
              <Download className='size-4' />
              {t('Download')}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </SectionPageLayout>
  )
}
