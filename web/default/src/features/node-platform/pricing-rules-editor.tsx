import { Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'

import type { PricingRule } from './types'

// extractParamNames pulls top-level keys from a JSON-encoded script_params
// string so the dropdown in the rule editor can offer them as suggestions.
export function extractParamNames(scriptParamsJson: string): string[] {
  try {
    const parsed = JSON.parse(scriptParamsJson)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return Object.keys(parsed)
    }
  } catch {
    // ignore
  }
  return []
}

// computeParamsMultiplier calculates the combined price multiplier for the
// given config object based on the script's pricing rules.
// Called client-side to preview price before order submission.
export function computeParamsMultiplier(
  config: unknown,
  rules: PricingRule[]
): number {
  if (!rules || rules.length === 0) return 1
  let multiplier = 1
  const cfg = config as Record<string, unknown>
  for (const rule of rules) {
    const value = cfg?.[rule.param]
    if (rule.type === 'enum_multiplier' && rule.values) {
      const factor = rule.values[String(value ?? '')]
      if (factor != null) multiplier *= factor
    } else if (rule.type === 'linear_range' && rule.unit_multiplier) {
      const n = Number(value)
      if (Number.isFinite(n) && n > 0) {
        multiplier *= n * rule.unit_multiplier
      }
    }
  }
  return multiplier
}

// --- Sub-editors -----------------------------------------------------------

type EnumValuesEditorProps = {
  values: Record<string, number>
  onChange: (values: Record<string, number>) => void
  readonly?: boolean
}

function EnumValuesEditor({ values, onChange, readonly }: EnumValuesEditorProps) {
  const { t } = useTranslation()
  const entries = Object.entries(values)

  function setKey(oldKey: string, newKey: string) {
    const next: Record<string, number> = {}
    for (const [k, v] of entries) {
      next[k === oldKey ? newKey : k] = v
    }
    onChange(next)
  }

  function setValue(key: string, raw: string) {
    const n = Number(raw)
    onChange({ ...values, [key]: Number.isFinite(n) ? n : 1 })
  }

  function addEntry() {
    let key = 'option'
    let i = 1
    while (key in values) key = `option${i++}`
    onChange({ ...values, [key]: 1 })
  }

  function removeEntry(key: string) {
    const next = { ...values }
    delete next[key]
    onChange(next)
  }

  return (
    <div className='space-y-1.5'>
      <div className='text-muted-foreground grid grid-cols-[1fr_80px_auto] gap-1.5 text-[11px] font-medium'>
        <span>{t('Option value')}</span>
        <span>{t('Multiplier ×')}</span>
        <span />
      </div>
      {entries.map(([key, val]) => (
        <div key={key} className='grid grid-cols-[1fr_80px_auto] items-center gap-1.5'>
          <Input
            className='h-7 text-xs'
            value={key}
            disabled={readonly}
            onChange={(e) => setKey(key, e.target.value)}
          />
          <Input
            className='h-7 text-xs'
            type='number'
            min={0.01}
            step={0.1}
            value={val}
            disabled={readonly}
            onChange={(e) => setValue(key, e.target.value)}
          />
          {!readonly ? (
            <Button
              type='button'
              size='icon-sm'
              variant='ghost'
              className='text-muted-foreground hover:text-destructive h-7 w-7'
              onClick={() => removeEntry(key)}
            >
              <Trash2 className='h-3 w-3' />
            </Button>
          ) : (
            <span />
          )}
        </div>
      ))}
      {!readonly && (
        <Button
          type='button'
          size='sm'
          variant='outline'
          className='h-7 w-full border-dashed text-xs'
          onClick={addEntry}
        >
          <Plus className='mr-1 h-3 w-3' />
          {t('Add option')}
        </Button>
      )}
    </div>
  )
}

// --- Single rule card -------------------------------------------------------

type RuleCardProps = {
  rule: PricingRule
  index: number
  availableParams: string[]
  onUpdate: (index: number, rule: PricingRule) => void
  onRemove: (index: number) => void
  readonly?: boolean
}

function RuleCard({ rule, index, availableParams, onUpdate, onRemove, readonly }: RuleCardProps) {
  const { t } = useTranslation()

  function patch(partial: Partial<PricingRule>) {
    onUpdate(index, { ...rule, ...partial })
  }

  function changeType(type: PricingRule['type']) {
    if (type === 'enum_multiplier') {
      onUpdate(index, { ...rule, type, values: {}, unit_multiplier: undefined })
    } else {
      onUpdate(index, { ...rule, type, values: undefined, unit_multiplier: 1, min: undefined, max: undefined })
    }
  }

  return (
    <div className='rounded-md border bg-muted/20 p-3 space-y-3'>
      <div className='flex items-start justify-between gap-2'>
        <div className='grid flex-1 gap-3 sm:grid-cols-[1fr_1fr_1fr]'>
          {/* Parameter */}
          <label className='space-y-1'>
            <span className='text-muted-foreground text-xs font-medium'>{t('Parameter')}</span>
            {availableParams.length > 0 ? (
              <select
                className='text-foreground h-8 w-full rounded-md border bg-transparent px-2 text-sm disabled:opacity-50'
                value={rule.param}
                disabled={readonly}
                onChange={(e) => patch({ param: e.target.value })}
              >
                <option value=''>{t('Select parameter')}</option>
                {availableParams.map((p) => (
                  <option key={p} value={p}>{p}</option>
                ))}
                {!availableParams.includes(rule.param) && rule.param && (
                  <option value={rule.param}>{rule.param}</option>
                )}
              </select>
            ) : (
              <Input
                className='h-8 text-sm'
                placeholder='model'
                value={rule.param}
                disabled={readonly}
                onChange={(e) => patch({ param: e.target.value })}
              />
            )}
          </label>

          {/* Display label */}
          <label className='space-y-1'>
            <span className='text-muted-foreground text-xs font-medium'>{t('Label (optional)')}</span>
            <Input
              className='h-8 text-sm'
              placeholder={t('e.g. Model')}
              value={rule.label ?? ''}
              disabled={readonly}
              onChange={(e) => patch({ label: e.target.value || undefined })}
            />
          </label>

          {/* Type */}
          <label className='space-y-1'>
            <span className='text-muted-foreground text-xs font-medium'>{t('Type')}</span>
            <select
              className='text-foreground h-8 w-full rounded-md border bg-transparent px-2 text-sm disabled:opacity-50'
              value={rule.type}
              disabled={readonly}
              onChange={(e) => changeType(e.target.value as PricingRule['type'])}
            >
              <option value='enum_multiplier'>{t('Enum options (dropdown)')}</option>
              <option value='linear_range'>{t('Numeric × unit cost')}</option>
            </select>
          </label>
        </div>

        {!readonly && (
          <Button
            type='button'
            size='icon-sm'
            variant='ghost'
            className='text-muted-foreground hover:text-destructive mt-5 shrink-0'
            title={t('Remove rule')}
            onClick={() => onRemove(index)}
          >
            <Trash2 className='h-4 w-4' />
          </Button>
        )}
      </div>

      {/* Type-specific config */}
      {rule.type === 'enum_multiplier' && (
        <EnumValuesEditor
          values={rule.values ?? {}}
          onChange={(values) => patch({ values })}
          readonly={readonly}
        />
      )}

      {rule.type === 'linear_range' && (
        <div className='grid gap-3 sm:grid-cols-3'>
          <label className='space-y-1'>
            <span className='text-muted-foreground text-xs font-medium'>
              {t('Cost per unit ×')}
            </span>
            <Input
              className='h-8 text-sm'
              type='number'
              min={0.001}
              step={0.1}
              placeholder='1'
              value={rule.unit_multiplier ?? 1}
              disabled={readonly}
              onChange={(e) => patch({ unit_multiplier: Number(e.target.value) || 1 })}
            />
          </label>
          <label className='space-y-1'>
            <span className='text-muted-foreground text-xs font-medium'>{t('Min value')}</span>
            <Input
              className='h-8 text-sm'
              type='number'
              placeholder={t('optional')}
              value={rule.min ?? ''}
              disabled={readonly}
              onChange={(e) => patch({ min: e.target.value ? Number(e.target.value) : undefined })}
            />
          </label>
          <label className='space-y-1'>
            <span className='text-muted-foreground text-xs font-medium'>{t('Max value')}</span>
            <Input
              className='h-8 text-sm'
              type='number'
              placeholder={t('optional')}
              value={rule.max ?? ''}
              disabled={readonly}
              onChange={(e) => patch({ max: e.target.value ? Number(e.target.value) : undefined })}
            />
          </label>
        </div>
      )}
    </div>
  )
}

// --- Main editor component --------------------------------------------------

export type PricingRulesEditorProps = {
  value: PricingRule[]
  onChange?: (rules: PricingRule[]) => void
  availableParams?: string[]
  readonly?: boolean
}

export function PricingRulesEditor({
  value,
  onChange,
  availableParams = [],
  readonly = false,
}: PricingRulesEditorProps) {
  const { t } = useTranslation()
  const [viewMode, setViewMode] = useState<'visual' | 'json'>('visual')
  const [jsonText, setJsonText] = useState('')
  const [jsonError, setJsonError] = useState('')

  function handleSetViewMode(mode: 'visual' | 'json') {
    if (mode === 'json') {
      setJsonText(JSON.stringify(value, null, 2))
      setJsonError('')
    }
    setViewMode(mode)
  }

  function handleJsonChange(text: string) {
    setJsonText(text)
    try {
      const parsed = JSON.parse(text)
      if (Array.isArray(parsed)) {
        setJsonError('')
        onChange?.(parsed as PricingRule[])
      } else {
        setJsonError(t('Must be a JSON array'))
      }
    } catch (e) {
      setJsonError(String((e as Error).message))
    }
  }

  function addRule() {
    onChange?.([
      ...value,
      { param: '', type: 'enum_multiplier', values: {} },
    ])
  }

  function updateRule(index: number, rule: PricingRule) {
    const next = [...value]
    next[index] = rule
    onChange?.(next)
  }

  function removeRule(index: number) {
    onChange?.(value.filter((_, i) => i !== index))
  }

  return (
    <div className='rounded-lg border'>
      {/* Toolbar */}
      <div className='flex items-center justify-between border-b px-3 py-2'>
        <div className='flex gap-1'>
          <Button
            type='button'
            size='sm'
            variant={viewMode === 'visual' ? 'secondary' : 'ghost'}
            onClick={() => handleSetViewMode('visual')}
          >
            {t('Visual')}
          </Button>
          <Button
            type='button'
            size='sm'
            variant={viewMode === 'json' ? 'secondary' : 'ghost'}
            onClick={() => handleSetViewMode('json')}
          >
            JSON
          </Button>
        </div>
        {!readonly && viewMode === 'visual' && (
          <Button type='button' size='sm' variant='outline' onClick={addRule}>
            <Plus className='mr-1 h-3.5 w-3.5' />
            {t('Add rule')}
          </Button>
        )}
      </div>

      {/* Content */}
      <div className='p-3'>
        {viewMode === 'visual' ? (
          value.length === 0 ? (
            <div className='text-muted-foreground py-6 text-center text-sm'>
              {readonly
                ? t('No pricing rules defined.')
                : t('No rules yet. Click "Add rule" to define which parameters affect the price.')}
            </div>
          ) : (
            <div className='space-y-3'>
              {value.map((rule, index) => (
                <RuleCard
                  key={index}
                  rule={rule}
                  index={index}
                  availableParams={availableParams}
                  onUpdate={updateRule}
                  onRemove={removeRule}
                  readonly={readonly}
                />
              ))}
            </div>
          )
        ) : (
          <div className='space-y-1'>
            <Textarea
              className='min-h-[160px] font-mono text-xs'
              value={jsonText}
              readOnly={readonly}
              onChange={(e) => handleJsonChange(e.target.value)}
            />
            {jsonError && (
              <p className='text-destructive text-xs'>{jsonError}</p>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
