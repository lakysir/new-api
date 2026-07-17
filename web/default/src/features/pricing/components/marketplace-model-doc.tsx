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

// MarketplaceModelDoc renders the caller-facing API docs for a bridged
// marketplace model inside the model-details API tab. Because each script
// declares its own params, generic endpoint samples are useless here — this
// pulls the script's `script_params` JSON Schema and renders a real parameter
// table plus concrete /v1/videos submit + poll samples with the model's actual
// fields. Renders nothing when the model is not a marketplace bridge model.

import { useQuery } from '@tanstack/react-query'
import { Boxes, ScrollText, Sigma } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import {
  CodeBlock,
  CodeBlockCopyButton,
} from '@/components/ai-elements/code-block'
import {
  StaticDataTable,
  staticDataTableClassNames as tableStyles,
} from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { useStatus } from '@/hooks/use-status'

import { getMarketplaceModelDoc, type MarketplaceModelDoc } from '../api'

type SchemaParam = {
  name: string
  type: string
  required: boolean
  description: string
  enumValues?: string[]
  defaultValue?: unknown
}

// parseSchemaParams flattens a JSON Schema object's top-level properties into a
// table-friendly list. Tolerant of missing/invalid schemas (returns []).
function parseSchemaParams(schema: string): SchemaParam[] {
  if (!schema.trim()) return []
  let parsed: unknown
  try {
    parsed = JSON.parse(schema)
  } catch {
    return []
  }
  const obj = parsed as {
    properties?: Record<string, Record<string, unknown>>
    required?: string[]
  }
  if (!obj || typeof obj !== 'object' || !obj.properties) return []
  const requiredSet = new Set(obj.required ?? [])
  return Object.entries(obj.properties).map(([name, spec]) => {
    const s = spec as Record<string, unknown>
    const enumRaw = Array.isArray(s.enum) ? (s.enum as unknown[]) : undefined
    return {
      name,
      type: typeof s.type === 'string' ? s.type : 'any',
      required: requiredSet.has(name),
      description: typeof s.description === 'string' ? s.description : '',
      enumValues: enumRaw?.map((v) => String(v)),
      defaultValue: s.default,
    }
  })
}

// buildExampleBody constructs a realistic request body: the operator's param
// template merged under schema defaults/examples, always including the model.
function buildExampleBody(doc: MarketplaceModelDoc): Record<string, unknown> {
  const body: Record<string, unknown> = { model: doc.model_name }
  // Seed from the script's schema defaults.
  for (const p of parseSchemaParams(doc.script_params)) {
    if (p.defaultValue !== undefined) {
      body[p.name] = p.defaultValue
    } else if (p.enumValues && p.enumValues.length > 0) {
      body[p.name] = p.enumValues[0]
    } else if (p.name === 'prompt') {
      body[p.name] = 'a dog'
    }
  }
  // Overlay the operator's param template (its concrete defaults win).
  if (doc.param_template.trim()) {
    try {
      const tpl = JSON.parse(doc.param_template) as Record<string, unknown>
      for (const [k, v] of Object.entries(tpl)) {
        if (k !== 'model') body[k] = v
      }
    } catch {
      /* ignore invalid template */
    }
  }
  return body
}

function useBaseUrl(): string {
  const { status } = useStatus()
  return useMemo(() => {
    const candidate =
      (status as Record<string, unknown> | null)?.server_address ??
      (status?.data as Record<string, unknown> | undefined)?.server_address
    if (candidate && typeof candidate === 'string') {
      return candidate.replace(/\/$/, '')
    }
    if (typeof window !== 'undefined') return window.location.origin
    return 'https://your-newapi.com'
  }, [status])
}

function buildCurlSample(baseUrl: string, body: Record<string, unknown>): string {
  const bodyJson = JSON.stringify(body, null, 2)
  return [
    `# 1) Submit the task — returns { "id": "task_...", "status": "queued" }`,
    `curl ${baseUrl}/v1/videos \\`,
    `  -H "Authorization: Bearer $NEW_API_KEY" \\`,
    `  -H "Content-Type: application/json" \\`,
    `  -d '${bodyJson.replace(/\n/g, '\n     ')}'`,
    ``,
    `# 2) Poll until status is "completed" (or "failed"); the video URL is`,
    `#    returned under metadata.url`,
    `curl ${baseUrl}/v1/videos/task_xxxxxxxx \\`,
    `  -H "Authorization: Bearer $NEW_API_KEY"`,
  ].join('\n')
}

function SectionTitle(props: {
  children: React.ReactNode
  icon: React.ComponentType<{ className?: string }>
}) {
  const Icon = props.icon
  return (
    <h3 className='text-foreground mb-3 flex items-center gap-1.5 text-sm font-semibold'>
      <Icon className='text-muted-foreground/70 size-3.5' />
      {props.children}
    </h3>
  )
}

export function MarketplaceModelDoc(props: { modelName: string }) {
  const { t } = useTranslation()
  const baseUrl = useBaseUrl()
  const { data: doc } = useQuery({
    queryKey: ['marketplace-model-doc', props.modelName],
    queryFn: () => getMarketplaceModelDoc(props.modelName),
    enabled: Boolean(props.modelName),
    staleTime: 60_000,
  })

  const params = useMemo(
    () => (doc ? parseSchemaParams(doc.script_params) : []),
    [doc]
  )
  const curlSample = useMemo(
    () => (doc ? buildCurlSample(baseUrl, buildExampleBody(doc)) : ''),
    [doc, baseUrl]
  )

  if (!doc) return null

  return (
    <section className='border-primary/30 bg-primary/5 space-y-5 rounded-xl border p-4'>
      <div>
        <SectionTitle icon={Boxes}>
          {t('AiToken marketplace model')}
        </SectionTitle>
        <p className='text-muted-foreground text-xs leading-relaxed'>
          {t(
            'This model runs a marketplace script over the async video API. Submit a task to /v1/videos, then poll GET /v1/videos/{id} until it completes; the result URL is returned under metadata.url.'
          )}
        </p>
        {doc.description ? (
          <p className='mt-2 text-sm whitespace-pre-wrap'>{doc.description}</p>
        ) : null}
        <div className='text-muted-foreground mt-2 text-xs'>
          {t('Consume multiplier')}: {doc.consume_multiplier} ·{' '}
          {t('Timeout')}: {doc.timeout_seconds}s
          {doc.task_type ? ` · ${doc.task_type}` : ''}
        </div>
      </div>

      {params.length > 0 && (
        <div>
          <SectionTitle icon={Sigma}>{t('Request parameters')}</SectionTitle>
          <StaticDataTable
            className={tableStyles.sectionContainer}
            headerRowClassName={tableStyles.mutedHeaderRow}
            data={params}
            getRowKey={(p) => p.name}
            getRowClassName={() => 'hover:bg-muted/20'}
            columns={[
              {
                id: 'parameter',
                header: t('Parameter'),
                className: 'h-9 w-44',
                cellClassName: tableStyles.topCell,
                cell: (p) => (
                  <div className='flex items-center gap-1.5'>
                    <code className='font-mono text-sm font-medium'>
                      {p.name}
                    </code>
                    {p.required && (
                      <Badge
                        variant='outline'
                        className='h-6 border-rose-500/40 px-2 text-sm text-rose-600 dark:text-rose-400'
                      >
                        {t('required')}
                      </Badge>
                    )}
                  </div>
                ),
              },
              {
                id: 'type',
                header: t('Type'),
                className: 'h-9 w-24',
                cellClassName: tableStyles.topCell,
                cell: (p) => (
                  <Badge
                    variant='secondary'
                    className='h-7 rounded-full px-2.5 font-mono text-sm font-normal'
                  >
                    {p.type}
                  </Badge>
                ),
              },
              {
                id: 'description',
                header: t('Description'),
                className: 'h-9',
                cellClassName: tableStyles.topMutedCell,
                cell: (p) => (
                  <div className='space-y-1'>
                    <div>{p.description || '—'}</div>
                    {p.enumValues && p.enumValues.length > 0 && (
                      <div className='flex flex-wrap gap-0.5'>
                        {p.enumValues.map((v) => (
                          <code
                            key={v}
                            className='bg-muted text-muted-foreground rounded px-1.5 py-0.5 font-mono text-xs'
                          >
                            {v}
                          </code>
                        ))}
                      </div>
                    )}
                  </div>
                ),
              },
            ]}
          />
        </div>
      )}

      <div>
        <SectionTitle icon={ScrollText}>{t('Example')}</SectionTitle>
        <CodeBlock code={curlSample} language='bash'>
          <CodeBlockCopyButton />
        </CodeBlock>
      </div>

      {doc.result_schema.trim() ? (
        <div>
          <SectionTitle icon={ScrollText}>{t('Result schema')}</SectionTitle>
          <CodeBlock code={prettyJSON(doc.result_schema)} language='json'>
            <CodeBlockCopyButton />
          </CodeBlock>
        </div>
      ) : null}
    </section>
  )
}

// prettyJSON pretty-prints a JSON string, falling back to the raw text.
function prettyJSON(raw: string): string {
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}
