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

import { Input } from '@/components/ui/input'
import { api } from '@/lib/api'

import {
  createCapabilityTest,
  deleteDevice,
  deleteNode,
  enableCapability,
  listAvailableScriptVersions,
  listMyDevices,
  listMyNodes,
  listNodeCapabilities,
  removeCapability,
  revokeDevice,
} from './api'
import { displayToMicros, formatUnix, microsToDisplay } from './lib/format'
import type { Device, NodeCapability, NodeInfo, ScriptVersion } from './types'

type PublishedScript = {
  id: number
  title: string
  category_id?: number
}

function nodeOnline(n: NodeInfo): boolean {
  return n.state !== 'OFFLINE' && n.last_seen_at >= Math.floor(Date.now() / 1000) - 45
}

export function NodesConsolePage() {
  const { t } = useTranslation()
  const [devices, setDevices] = useState<Device[]>([])
  const [nodes, setNodes] = useState<NodeInfo[]>([])
  const [caps, setCaps] = useState<Record<string, NodeCapability[]>>({})
  const [loading, setLoading] = useState(false)
  // A user may register dozens/hundreds of devices; hide revoked/offline by
  // default to keep the list readable.
  const [hideInactive, setHideInactive] = useState(true)
  // Published scripts to pick from when listing a capability.
  const [pubScripts, setPubScripts] = useState<PublishedScript[]>([])
  const [scriptVersions, setScriptVersions] = useState<Record<number, ScriptVersion[]>>({})
  // Per-node enable form: script id + version + price + quota.
  const [enableForm, setEnableForm] = useState<
    Record<string, { scriptId: string; version: string; price: string; quota: string }>
  >({})

  const visibleDevices = hideInactive
    ? devices.filter((d) => d.status === 'active')
    : devices
  const visibleNodes = hideInactive ? nodes.filter((n) => nodeOnline(n)) : nodes

  async function loadAll() {
    setLoading(true)
    try {
      const [d, n, sq] = await Promise.all([
        listMyDevices(),
        listMyNodes(),
        api.get('/api/scripts/square', { params: { limit: 200 } }),
      ])
      setDevices(d)
      setNodes(n)
      const items = (sq.data?.data?.items ?? sq.data?.items ?? sq.data?.data ?? []) as PublishedScript[]
      setPubScripts(items)
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
    try {
      const test = await createCapabilityTest(nodeId, scriptId, version)
      await enableCapability(nodeId, scriptId, {
        version,
        price_micros: displayToMicros(f.price || '0'),
        daily_quota: Number(f.quota || '0'),
        test_expires_at: test.test_expires_at,
      })
      toast.success(t('Capability listed'))
      await loadCaps(nodeId)
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  function setForm(nodeId: string, patch: Partial<{ scriptId: string; version: string; price: string; quota: string }>) {
    setEnableForm((p) => {
      const current = p[nodeId] ?? { scriptId: '', version: '1', price: '', quota: '10' }
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
      const versions = scriptVersions[scriptId] || (await listAvailableScriptVersions(scriptId))
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
    loadAll()
  }, [])

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
    if (!window.confirm(t('Permanently delete revoked device {{id}} and its nodes?', { id }))) {
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
      setCaps((p) => ({ ...p, [nodeId]: list }))
    } catch (e) {
      toast.error(String((e as Error).message))
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
        <Button variant='outline' onClick={loadAll} disabled={loading}>
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
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
                <TableCell>{d.status === 'active' ? '🟢 active' : '⛔ revoked'}</TableCell>
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
                <TableCell colSpan={5} className='text-muted-foreground text-center'>
                  {loading ? t('Loading...') : t('No devices')}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>

        <div className='mt-6 mb-2 text-sm font-medium'>{t('Nodes')} ({visibleNodes.length}/{nodes.length})</div>
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
                  <Button size='sm' variant='ghost' onClick={() => loadCaps(n.id)}>
                    {t('Capabilities')}
                  </Button>
                  {!nodeOnline(n) && (
                    <Button size='sm' variant='outline' onClick={() => onDeleteNode(n.id)}>
                      {t('Delete')}
                    </Button>
                  )}
                </TableCell>
              </TableRow>
            ))}
            {visibleNodes.length === 0 && (
              <TableRow>
                <TableCell colSpan={6} className='text-muted-foreground text-center'>
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

            {/* Balance checks must run in the browser plugin (it holds the
                target-site session). Do them there before listing a category. */}
            <div className='mb-3 rounded-lg border border-amber-500/40 bg-amber-500/5 p-3 text-xs'>
              {t(
                'Run the site balance check in the browser plugin (popup → 本节点能力 → 读余额检查) before listing a category. Listing requires a passing balance check.'
              )}
            </div>

            {/* List a capability: pick a published script + version, set price and
                daily quota, then enable (requires the category balance check). */}
            <div className='mb-3 flex flex-wrap items-center gap-2 rounded-lg border p-3'>
              <span className='text-muted-foreground text-xs'>{t('List capability')}:</span>
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
                onChange={(e) => setForm(nodeId, { version: e.target.value })}
              >
                <option value=''>{t('Version')}</option>
                {(scriptVersions[Number(enableForm[nodeId]?.scriptId)] || []).map(
                  (version) => (
                    <option key={version.id} value={version.version}>
                      v{version.version}
                    </option>
                  )
                )}
              </select>
              <Input
                className='w-28'
                placeholder={t('Price')}
                value={enableForm[nodeId]?.price ?? ''}
                onChange={(e) => setForm(nodeId, { price: e.target.value })}
              />
              <Input
                className='w-24'
                placeholder={t('Daily quota')}
                value={enableForm[nodeId]?.quota ?? '10'}
                onChange={(e) => setForm(nodeId, { quota: e.target.value })}
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
                  <TableHead>{t('Quota')}</TableHead>
                  <TableHead>{t('Status')}</TableHead>
                  <TableHead>{t('Actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {list.map((c) => (
                  <TableRow key={c.id}>
                    <TableCell>#{c.script_id}</TableCell>
                    <TableCell>v{c.version}</TableCell>
                    <TableCell>{microsToDisplay(c.price_micros)}</TableCell>
                    <TableCell>
                      {c.remaining_quota}/{c.daily_quota}
                    </TableCell>
                    <TableCell>{c.status}</TableCell>
                    <TableCell>
                      <Button
                        size='sm'
                        variant='destructive'
                        onClick={() => onRemove(nodeId, c.script_id, c.version)}
                      >
                        {t('Unlist')}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
                {list.length === 0 && (
                  <TableRow>
                    <TableCell colSpan={6} className='text-muted-foreground text-center'>
                      {t('No capabilities')}
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </div>
        ))}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
