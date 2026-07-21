import { BookOpen, ChevronRight, DollarSign, Info, Layers, Timer, Zap } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'

// Section: a titled content block with an anchor id for deep-linking.
function Section({
  id,
  icon: Icon,
  title,
  children,
}: {
  id: string
  icon: React.ComponentType<{ className?: string }>
  title: string
  children: React.ReactNode
}) {
  return (
    <section id={id} className='scroll-mt-20 space-y-4'>
      <div className='flex items-center gap-2 border-b pb-2'>
        <Icon className='text-primary h-5 w-5 shrink-0' aria-hidden='true' />
        <h2 className='text-xl font-semibold'>{title}</h2>
      </div>
      <div className='space-y-3 text-sm leading-relaxed'>{children}</div>
    </section>
  )
}

function CodeBlock({ code, language = 'json' }: { code: string; language?: string }) {
  return (
    <pre className='bg-muted/40 overflow-x-auto rounded-lg border p-3 font-mono text-xs leading-relaxed whitespace-pre'>
      <code data-language={language}>{code}</code>
    </pre>
  )
}

function Callout({ type, children }: { type: 'info' | 'tip'; children: React.ReactNode }) {
  return (
    <div
      className={`flex gap-2 rounded-lg border p-3 text-sm ${
        type === 'info'
          ? 'border-blue-200 bg-blue-50 text-blue-900 dark:border-blue-900/40 dark:bg-blue-950/30 dark:text-blue-200'
          : 'border-emerald-200 bg-emerald-50 text-emerald-900 dark:border-emerald-900/40 dark:bg-emerald-950/30 dark:text-emerald-200'
      }`}
    >
      <Info className='mt-0.5 h-4 w-4 shrink-0' aria-hidden='true' />
      <div>{children}</div>
    </div>
  )
}

const EXAMPLE_PRICING_RULES = JSON.stringify(
  [
    {
      param: 'model',
      type: 'enum_multiplier',
      label: '模型',
      values: {
        Seedance_2_0_mini_lite: 1.0,
        Seedance_2_0_mini: 2.0,
        'seedance2.0_vision': 5.0,
      },
    },
    {
      param: 'resolution',
      type: 'enum_multiplier',
      label: '分辨率',
      values: {
        '480p': 1.0,
        '720p': 1.5,
        '1080p': 2.5,
      },
    },
    {
      param: 'duration',
      type: 'linear_range',
      label: '视频时长（秒）',
      unit_multiplier: 1.0,
      min: 4,
      max: 15,
    },
  ],
  null,
  2
)

const EXAMPLE_SCRIPT_PARAMS = JSON.stringify(
  {
    prompt: '一只在草地上奔跑的金毛猎犬',
    model: 'Seedance_2_0_mini_lite',
    resolution: '480p',
    duration: 5,
  },
  null,
  2
)

const EXAMPLE_SCRIPT_CODE = `async function runGeneratedTest(config) {
  // Script Params is passed to this function as config.
  const prompt = String(config.prompt || '')
  if (!prompt) {
    return { status: 'failed', balance: 0, error: 'prompt is required' }
  }

  // The script runs in the target page's MAIN world. It can use document,
  // location, and credentialed fetch requests for the signed-in account.
  const response = await fetch('/api/generate', {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      prompt,
      model: config.model,
      resolution: config.resolution,
      duration: config.duration,
    }),
  })
  const result = await response.json()

  // Return serializable data. balance updates the node's remaining quota.
  return {
    status: 'success',
    balance: Number(result.balance || 0),
    video_url: result.video_url,
  }
}`

const EXAMPLE_PRICE_CALC = `基础单价: $0.001 (由脚本创作者设定)
节点倍率: 1.2× (由节点提供者设定)
参数倍率计算:
  duration=5s   → 5 × 1.0 = 5.0×
  model="mini"  → 2.0×
  resolution="720p" → 1.5×

最终价格 = $0.001 × 1.2 × 5.0 × 2.0 × 1.5 = $0.018`

export function ScriptCreatorGuidePage() {
  const { t } = useTranslation()

  const sections = [
    { id: 'overview', label: t('概述') },
    { id: 'params', label: t('脚本参数设计') },
    { id: 'concurrency', label: t('并发数与最小间隔') },
    { id: 'base-price', label: t('基础价格') },
    { id: 'pricing-rules', label: t('定价规则') },
    { id: 'review', label: t('审核与发布流程') },
    { id: 'example', label: t('完整示例') },
  ]

  return (
    <PublicLayout>
      <div className='mx-auto flex w-full max-w-5xl gap-8 px-4 py-8'>
        {/* Sticky sidebar TOC (desktop only) */}
        <aside className='hidden w-48 shrink-0 lg:block'>
          <div className='sticky top-20'>
            <div className='mb-2 flex items-center gap-1.5 text-sm font-medium'>
              <BookOpen className='h-4 w-4' aria-hidden='true' />
              {t('目录')}
            </div>
            <nav className='space-y-1'>
              {sections.map((s) => (
                <a
                  key={s.id}
                  href={`#${s.id}`}
                  className='text-muted-foreground hover:text-foreground flex items-center gap-1 rounded px-2 py-1 text-xs transition-colors hover:bg-muted/50'
                >
                  <ChevronRight className='h-3 w-3 shrink-0' aria-hidden='true' />
                  {s.label}
                </a>
              ))}
            </nav>
          </div>
        </aside>

        {/* Main content */}
        <article className='min-w-0 flex-1 space-y-12'>
          <header className='space-y-2'>
            <h1 className='text-3xl font-bold'>{t('脚本创作指南')}</h1>
            <p className='text-muted-foreground text-base'>
              {t('了解如何创作、配置并发布高质量的可执行脚本，从定价规则到审核流程一一说明。')}
            </p>
          </header>

          {/* ── 1. 概述 ── */}
          <Section id='overview' icon={Layers} title={t('概述')}>
            <p>
              {t('脚本市场是一个 P2P 执行平台：你（脚本创作者）编写浏览器脚本，节点提供者在其本地浏览器里运行，买家支付费用获得结果。')}
            </p>
            <p>
              {t('整个流程的参数与结果经过端对端加密（E2EE），控制平面只能看到哈希值，原文仅在买家浏览器和节点提供者之间传输。')}
            </p>
            <div className='rounded-lg border'>
              <div className='grid divide-y text-sm'>
                {[
                  [t('脚本创作者'), t('编写脚本代码、设定基础价格与定价规则、提交审核')],
                  [t('平台审核员'), t('审核代码安全性和定价合理性，设置平台手续费')],
                  [t('节点提供者'), t('将自己的浏览器账号接入，设置价格倍率，收取执行报酬')],
                  [t('买家'), t('选择脚本、填写参数、支付费用、获得结果')],
                ].map(([role, desc]) => (
                  <div key={role as string} className='grid grid-cols-[9rem_1fr] gap-3 px-3 py-2'>
                    <span className='font-medium'>{role}</span>
                    <span className='text-muted-foreground'>{desc}</span>
                  </div>
                ))}
              </div>
            </div>
            <p>
              {t('收益分配：每笔订单按百分比分配给脚本创作者、节点提供者和平台，具体比例在审核时确认。')}
            </p>
          </Section>

          {/* ── 2. 脚本参数设计 ── */}
          <Section id='params' icon={Layers} title={t('脚本参数设计')}>
            <p>
              {t('Script Params（脚本参数）是一个 JSON 对象，定义了买家运行你的脚本时可以填写哪些参数，也作为买家看到的默认表单初始值。')}
            </p>
            <p>{t('脚本运行时，这个 JSON 会整体作为 config 传入你的函数：')}</p>
            <CodeBlock
              language='javascript'
              code={`async function runGeneratedTest(config) {\n  const prompt = config.prompt\n  const model = config.model || 'default'\n  // ...\n}`}
            />
            <Callout type='info'>
              {t('参数中影响价格的字段（如 model、resolution、duration）最好使用清晰的英文键名，方便在定价规则里对应。')}
            </Callout>
            <p>{t('示例参数：')}</p>
            <CodeBlock code={EXAMPLE_SCRIPT_PARAMS} />
            <div className='space-y-2 pt-2'>
              <div className='font-medium'>{t('函数契约与返回值')}</div>
              <p className='text-muted-foreground'>
                {t('Script Params 中的 JSON 会作为 config 参数传入 runGeneratedTest。脚本运行在目标页面的 MAIN world，可访问 document、location，以及携带当前登录凭证的 fetch。')}
              </p>
              <p className='text-muted-foreground'>
                {t('返回值必须是可序列化对象。status 表示执行状态，balance 表示账号当前剩余额度；平台会使用 balance 更新节点的剩余额度显示。图片、视频、音频地址和其他业务字段可以按需返回。')}
              </p>
              <CodeBlock language='javascript' code={EXAMPLE_SCRIPT_CODE} />
            </div>
          </Section>

          {/* ── 3. 并发数与最小间隔 ── */}
          <Section id='concurrency' icon={Timer} title={t('并发数与最小间隔')}>
            <div className='grid gap-4 sm:grid-cols-2'>
              <div className='space-y-2 rounded-lg border p-3'>
                <div className='font-medium'>{t('并发数（Concurrency）')}</div>
                <p className='text-muted-foreground'>
                  {t('同一节点上可以同时运行的任务数量。这个值取决于目标网站 API 允许的同时并发任务数。')}
                </p>
                <p className='text-muted-foreground'>
                  {t('例：目标 API 只允许同时有 2 个生成任务 → 设为 2。默认值 1 最安全。')}
                </p>
              </div>
              <div className='space-y-2 rounded-lg border p-3'>
                <div className='font-medium'>{t('最小间隔（Min Interval）')}</div>
                <p className='text-muted-foreground'>
                  {t('两次任务提交之间的最短等待秒数。目标 API 如有速率限制（如每90秒一次），需在此填写。')}
                </p>
                <p className='text-muted-foreground'>
                  {t('例：API 要求两次提交之间至少间隔 90 秒 → 设为 90。默认 30 秒是保守估计。')}
                </p>
              </div>
            </div>
            <Callout type='tip'>
              {t('这两个值由你（脚本创作者）设定，因为你最了解目标网站的限制。节点提供者只能看到这些值，无法修改。')}
            </Callout>
          </Section>

          {/* ── 4. 基础价格 ── */}
          <Section id='base-price' icon={DollarSign} title={t('基础价格')}>
            <p>
              {t('基础价格（Base Price）是你作为创作者建议的每"执行单位"费用（单位：美元）。')}
            </p>
            <p>
              {t('节点提供者在上架你的脚本时，会在此基础上设置一个价格倍率（0.5× ～ 10×，默认 1×），以反映他们的账号成本。')}
            </p>
            <p>
              {t('买家最终支付的价格由三部分相乘决定：基础价格 × 节点倍率 × 参数倍率（由定价规则决定）。')}
            </p>
            <Callout type='info'>
              {t('建议参考目标 API 的实际调用成本来设定基础价格，以确保节点提供者有动力上架你的脚本。')}
            </Callout>
          </Section>

          {/* ── 5. 定价规则 ── */}
          <Section id='pricing-rules' icon={Zap} title={t('定价规则')}>
            <p>
              {t('定价规则（Pricing Rules）定义了脚本参数如何影响最终价格。规则是一个数组，每条规则对应一个参数。')}
            </p>

            <div className='space-y-2'>
              <div className='font-medium'>{t('规则类型一：枚举倍率（enum_multiplier）')}</div>
              <p className='text-muted-foreground'>
                {t('适用于有限选项的参数，如 model、resolution。为每个可选值指定一个价格倍率。')}
              </p>
              <CodeBlock
                code={JSON.stringify(
                  {
                    param: 'model',
                    type: 'enum_multiplier',
                    label: '模型',
                    values: { mini_lite: 1.0, mini: 2.0, vision: 5.0 },
                  },
                  null,
                  2
                )}
              />
            </div>

            <div className='space-y-2'>
              <div className='font-medium'>{t('规则类型二：线性倍率（linear_range）')}</div>
              <p className='text-muted-foreground'>
                {t('适用于数值参数，如 duration（秒）、steps（步数）。每单位值对应一个倍率系数。')}
              </p>
              <CodeBlock
                code={JSON.stringify(
                  {
                    param: 'duration',
                    type: 'linear_range',
                    label: '视频时长（秒）',
                    unit_multiplier: 1.0,
                    min: 4,
                    max: 15,
                  },
                  null,
                  2
                )}
              />
              <p className='text-muted-foreground text-xs'>
                {t('上例：duration=10 时，价格倍率 = 10 × 1.0 = 10×')}
              </p>
            </div>

            <div className='space-y-2'>
              <div className='font-medium'>{t('价格计算公式')}</div>
              <CodeBlock language='text' code={EXAMPLE_PRICE_CALC} />
            </div>

            <Callout type='tip'>
              {t('定价规则由平台审核员审核，若不合理会被要求修改。请确保规则反映真实的 API 成本结构。')}
            </Callout>

            <p>
              {t('在编辑器里，你可以用可视化面板添加规则，也可以切换到 JSON 模式直接编辑。')}
            </p>
          </Section>

          {/* ── 6. 审核与发布流程 ── */}
          <Section id='review' icon={BookOpen} title={t('审核与发布流程')}>
            <ol className='space-y-3'>
              {[
                [t('① 编写草稿'), t('在"我的脚本"页创建或编辑脚本，填写代码、参数、并发设置、基础价格和定价规则，保存为草稿。')],
                [t('② 提交审核'), t('点击"Submit review"，填写你建议的收益分成比例（0% ～ 5%）和目标站点分类，提交给管理员审核。')],
                [t('③ 等待审核'), t('管理员会审查代码安全性、定价规则合理性，可能修改分成比例或定价规则，然后通过或驳回。')],
                [t('④ 发布版本'), t('审核通过后，你会看到"Publish version"按钮，点击后脚本正式发布，节点提供者可以上架该版本。')],
                [t('⑤ 更新迭代'), t('如需更新脚本，编辑后再次提交审核。新版本和旧版本独立存在，买家和节点可以选择版本。')],
              ].map(([step, desc]) => (
                <li key={step as string} className='flex gap-3'>
                  <span className='font-semibold shrink-0'>{step}</span>
                  <span className='text-muted-foreground'>{desc}</span>
                </li>
              ))}
            </ol>
            <Callout type='info'>
              {t('每次提交审核时，编辑器会自动预填上一个已发布版本的定价规则和基础价格，你无需每次从头填写。')}
            </Callout>
          </Section>

          {/* ── 7. 完整示例 ── */}
          <Section id='example' icon={Layers} title={t('完整示例（视频生成脚本）')}>
            <p>{t('以下是一个视频生成脚本的完整配置示例，展示了各字段如何协同工作。')}</p>

            <div className='space-y-2'>
              <div className='font-medium'>{t('Script Params（买家默认表单值）')}</div>
              <CodeBlock code={EXAMPLE_SCRIPT_PARAMS} />
            </div>

            <div className='space-y-2'>
              <div className='font-medium'>{t('Pricing Rules（定价规则）')}</div>
              <CodeBlock code={EXAMPLE_PRICING_RULES} />
            </div>

            <div className='space-y-2'>
              <div className='font-medium'>{t('关键设置')}</div>
              <div className='rounded-lg border'>
                <div className='grid divide-y text-sm'>
                  {[
                    [t('并发数'), '1', t('视频生成 API 通常不支持并行提交')],
                    [t('最小间隔'), '90s', t('该 API 要求两次提交间隔 90 秒')],
                    [t('基础价格'), '$0.001', t('每执行单位基础费用')],
                  ].map(([field, value, desc]) => (
                    <div key={field as string} className='grid grid-cols-[8rem_5rem_1fr] gap-2 px-3 py-2'>
                      <span className='font-medium'>{field}</span>
                      <span className='font-mono text-xs font-semibold'>{value}</span>
                      <span className='text-muted-foreground'>{desc}</span>
                    </div>
                  ))}
                </div>
              </div>
            </div>

            <div className='space-y-2'>
              <div className='font-medium'>{t('价格计算示例')}</div>
              <CodeBlock language='text' code={EXAMPLE_PRICE_CALC} />
            </div>
          </Section>
        </article>
      </div>
    </PublicLayout>
  )
}
