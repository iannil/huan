import { useEffect, useState, useCallback, useRef } from 'react'
import { FileCode2, Check, Loader2 } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'

interface SiteSettings {
  title: string
  enableEmoji: boolean
  minify: boolean
  paginate: number
  summaryLength: number
  params: {
    subTitle: string
    footerSlogan: string
    description: string
    copyrights: string
    googleAnalytics: string
    cdnURL: string
    enableMathJax: boolean
    enableSummary: boolean
  }
}

const initial: SiteSettings = {
  title: '',
  enableEmoji: true,
  minify: true,
  paginate: 10,
  summaryLength: 120,
  params: {
    subTitle: '',
    footerSlogan: '',
    description: '',
    copyrights: '',
    googleAnalytics: '',
    cdnURL: '',
    enableMathJax: false,
    enableSummary: true,
  },
}

type SaveState = 'idle' | 'saving' | 'saved' | 'error'

interface NavItem {
  id: string
  label: string
}

const navItems: NavItem[] = [
  { id: 'info', label: '站点信息' },
  { id: 'toggles', label: '功能开关' },
  { id: 'numeric', label: '数值设置' },
]

export default function Settings() {
  const [settings, setSettings] = useState<SiteSettings>(initial)
  const [loading, setLoading] = useState(true)
  const [saveState, setSaveState] = useState<SaveState>('idle')
  const [errorMsg, setErrorMsg] = useState('')
  const [activeNav, setActiveNav] = useState('info')
  const hasChanges = useRef(false)
  const scrollContainerRef = useRef<HTMLDivElement>(null)

  const fetchSettings = useCallback(() => {
    setLoading(true)
    fetch('/admin/api/settings')
      .then((r) => (r.ok ? r.json() : null))
      .then((d) => {
        if (d) {
          setSettings(d)
          hasChanges.current = false
        }
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    fetchSettings()
  }, [fetchSettings])

  // IntersectionObserver to highlight active nav item on scroll
  useEffect(() => {
    const sections = navItems
      .map((n) => document.getElementById('section-' + n.id))
      .filter(Boolean) as HTMLElement[]

    if (sections.length === 0) return

    const observer = new IntersectionObserver(
      (entries) => {
        // Find the first visible section (closest to top)
        const visible = entries
          .filter((e) => e.isIntersecting)
          .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top)
        if (visible.length > 0) {
          const id = visible[0].target.id.replace('section-', '')
          setActiveNav(id)
        }
      },
      {
        rootMargin: '-80px 0px -60% 0px',
        threshold: 0,
      }
    )

    sections.forEach((s) => observer.observe(s))
    return () => observer.disconnect()
  }, [loading])

  const scrollToSection = (id: string) => {
    setActiveNav(id)
    const el = document.getElementById('section-' + id)
    if (el) {
      el.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }
  }

  const update = <K extends keyof SiteSettings>(
    key: K,
    value: SiteSettings[K]
  ) => {
    setSettings((prev) => ({ ...prev, [key]: value }))
    hasChanges.current = true
  }

  const updateParam = <K extends keyof SiteSettings['params']>(
    key: K,
    value: SiteSettings['params'][K]
  ) => {
    setSettings((prev) => ({
      ...prev,
      params: { ...prev.params, [key]: value },
    }))
    hasChanges.current = true
  }

  const save = async () => {
    setSaveState('saving')
    setErrorMsg('')
    try {
      const res = await fetch('/admin/api/settings', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(settings),
      })
      if (res.ok) {
        setSaveState('saved')
        hasChanges.current = false
        setTimeout(() => setSaveState('idle'), 2000)
      } else {
        const err = await res.json()
        setSaveState('error')
        setErrorMsg(err.error || '保存失败')
      }
    } catch {
      setSaveState('error')
      setErrorMsg('网络错误')
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center py-32 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 mr-2 animate-spin" />
        加载中...
      </div>
    )
  }

  return (
    <div className="max-w-4xl mx-auto">
      {/* Page header */}
      <div className="mb-8 flex items-start justify-between">
        <div>
          <h1 className="text-lg font-semibold text-foreground tracking-tight">
            站点设置
          </h1>
          <p className="text-sm text-muted-foreground mt-1">
            管理站点基本信息和功能开关
          </p>
        </div>
        <a
          href="/admin/api/settings/yaml"
          target="_blank"
          rel="noopener noreferrer"
          className="flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
        >
          <FileCode2 className="h-3.5 w-3.5 shrink-0" />
          <span>编辑 YAML</span>
        </a>
      </div>

      <div className="flex gap-8">
        {/* ============ Nav sidebar ============ */}
        <aside className="w-36 shrink-0">
          <div className="sticky top-20 space-y-0.5">
            {navItems.map((item) => (
              <button
                key={item.id}
                onClick={() => scrollToSection(item.id)}
                className={
                  `w-full text-left px-2.5 py-1.5 text-sm rounded-md transition-colors ` +
                  (activeNav === item.id
                    ? 'bg-muted text-foreground font-medium'
                    : 'text-muted-foreground hover:text-foreground hover:bg-muted')
                }
              >
                {item.label}
              </button>
            ))}
          </div>
        </aside>

        {/* ============ Settings content ============ */}
        <div ref={scrollContainerRef} className="flex-1 min-w-0">
          {/* Section: Site Info */}
          <SectionCard id="info" title="站点信息" description="站点名称、描述、版权信息等基础配置">
            <FieldRow label="站点标题">
              <Input
                value={settings.title}
                onChange={(e) => update('title', e.target.value)}
                placeholder="我的站点"
              />
            </FieldRow>
            <FieldRow label="副标题">
              <Input
                value={settings.params.subTitle}
                onChange={(e) => updateParam('subTitle', e.target.value)}
              />
            </FieldRow>
            <FieldRow label="页脚标语">
              <Input
                value={settings.params.footerSlogan}
                onChange={(e) => updateParam('footerSlogan', e.target.value)}
              />
            </FieldRow>
            <FieldRow label="站点描述" last>
              <textarea
                value={settings.params.description}
                onChange={(e) => updateParam('description', e.target.value)}
                rows={2}
                className="h-14 w-full min-w-0 border border-input bg-transparent px-2 py-1 text-xs transition-none outline-none rounded-md focus-visible:border-primary placeholder:text-muted-foreground resize-none"
              />
            </FieldRow>
            <FieldRow label="版权信息">
              <Input
                value={settings.params.copyrights}
                onChange={(e) => updateParam('copyrights', e.target.value)}
              />
            </FieldRow>
            <FieldRow label="Google Analytics">
              <Input
                value={settings.params.googleAnalytics}
                onChange={(e) => updateParam('googleAnalytics', e.target.value)}
                placeholder="G-XXXXXXXXXX"
              />
            </FieldRow>
            <FieldRow label="CDN URL" last>
              <Input
                value={settings.params.cdnURL}
                onChange={(e) => updateParam('cdnURL', e.target.value)}
                placeholder="https://cdn.example.com"
              />
            </FieldRow>
          </SectionCard>

          {/* Section: Switches */}
          <SectionCard id="toggles" title="功能开关" description="控制站点生成和渲染行为">
            <ToggleRow
              label="启用 Emoji"
              description="支持 emoji 短代码 :smile: → 😄"
              checked={settings.enableEmoji}
              onChange={(v) => update('enableEmoji', v)}
            />
            <ToggleRow
              label="压缩 HTML"
              description="移除输出 HTML 中的多余空格和换行"
              checked={settings.minify}
              onChange={(v) => update('minify', v)}
            />
            <ToggleRow
              label="启用摘要"
              description="在列表页显示自动生成的摘要"
              checked={settings.params.enableSummary}
              onChange={(v) => updateParam('enableSummary', v)}
            />
            <ToggleRow
              label="启用 MathJax"
              description="渲染 LaTeX 数学公式"
              checked={settings.params.enableMathJax}
              onChange={(v) => updateParam('enableMathJax', v)}
            />
          </SectionCard>

          {/* Section: Numeric */}
          <SectionCard id="numeric" title="数值设置" description="分页和摘要相关的数值参数">
            <FieldRow label="分页数">
              <Input
                type="number"
                min={1}
                value={settings.paginate}
                onChange={(e) => update('paginate', parseInt(e.target.value, 10) || 1)}
                className="w-24"
              />
            </FieldRow>
            <FieldRow label="摘要长度" last>
              <Input
                type="number"
                min={0}
                value={settings.summaryLength}
                onChange={(e) => update('summaryLength', parseInt(e.target.value, 10) || 0)}
                className="w-24"
              />
            </FieldRow>
          </SectionCard>

          {/* Save bar */}
          <div className="sticky bottom-0 mt-10 -mx-6 px-6 py-4 bg-background/90 backdrop-blur-sm border-t border-border flex items-center justify-between rounded-b-lg">
            <div className="flex items-center gap-2">
              {hasChanges.current && (
                <span className="text-xs text-muted-foreground">有未保存的更改</span>
              )}
              {saveState === 'saved' && (
                <span className="flex items-center gap-1.5 text-xs text-green-600">
                  <Check className="h-3.5 w-3.5 shrink-0" />
                  已保存
                </span>
              )}
              {saveState === 'error' && (
                <span className="text-xs text-red-500">{errorMsg}</span>
              )}
            </div>
            <button
              onClick={save}
              disabled={saveState === 'saving' || !hasChanges.current}
              className="flex items-center gap-1.5 px-4 py-1.5 rounded-md text-sm bg-primary text-primary-foreground hover:opacity-90 transition-opacity disabled:opacity-30 disabled:cursor-not-allowed"
            >
              {saveState === 'saving' ? (
                <>
                  <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin" />
                  <span>保存中...</span>
                </>
              ) : (
                <span>保存设置</span>
              )}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

// --- Sub-components ---

function SectionCard({
  id,
  title,
  description,
  children,
}: {
  id: string
  title: string
  description?: string
  children: React.ReactNode
}) {
  return (
    <div id={'section-' + id} className="mb-6 border border-border rounded-lg bg-background scroll-mt-24">
      {/* Left-bordered header area */}
      <div className="px-5 pt-4 pb-3 border-l-[3px] border-foreground rounded-tl-lg">
        <h2 className="text-sm font-medium text-foreground tracking-tight">
          {title}
        </h2>
        {description && (
          <p className="text-xs text-muted-foreground mt-1">{description}</p>
        )}
      </div>
      <div className="px-5 pb-1">{children}</div>
    </div>
  )
}

function FieldRow({
  label,
  children,
  last = false,
}: {
  label: string
  children: React.ReactNode
  last?: boolean
}) {
  return (
    <div
      className={`flex items-center gap-4 py-3 ${
        !last ? 'border-b border-border' : ''
      }`}
    >
      <label className="w-28 shrink-0 text-xs text-muted-foreground">
        {label}
      </label>
      <div className="flex-1">{children}</div>
    </div>
  )
}

function ToggleRow({
  label,
  description,
  checked,
  onChange,
  last,
}: {
  label: string
  description?: string
  checked: boolean
  onChange: (v: boolean) => void
  last?: boolean
}) {
  return (
    <div
      className={`flex items-center justify-between py-3 ${
        !last ? 'border-b border-border' : ''
      }`}
    >
      <div>
        <span className="text-sm text-foreground">{label}</span>
        {description && (
          <p className="text-xs text-muted-foreground mt-0.5">{description}</p>
        )}
      </div>
      <Switch checked={checked} onCheckedChange={(v) => onChange(v)} />
    </div>
  )
}
