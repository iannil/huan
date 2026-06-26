import { useState, useEffect, useCallback, useMemo, useRef } from 'react'
import { useSearchParams, useNavigate } from 'react-router-dom'
import { ArrowLeft, Save, Trash2, Eye, PenLine, Columns3 } from 'lucide-react'
import { marked } from 'marked'

interface ContentDetail {
  title: string
  relPath: string
  section: string
  kind: string
  draft: boolean
  rawContent: string
  frontmatter: Record<string, unknown>
}

type PreviewMode = 'edit' | 'split' | 'preview'

/** GitHub-style slug for heading IDs (matches marked's default slugger). */
function slugify(text: string): string {
  return text
    .toLowerCase()
    .replace(/[^\w\s-]/g, '')
    .replace(/\s+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-+|-+$/g, '')
}

interface HeadingAnchor {
  line: number    // 0-based line in the editor
  slug: string    // GitHub-style slug matching the rendered <hN id="...">
  text: string    // raw heading text
}

export default function ContentEdit() {
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const path = searchParams.get('path') || ''

  const [detail, setDetail] = useState<ContentDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState(false)
  const [body, setBody] = useState('')
  const [title, setTitle] = useState('')
  const [draft, setDraft] = useState(false)
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const [previewMode, setPreviewMode] = useState<PreviewMode>('split')

  const previewRef = useRef<HTMLDivElement>(null)
  const editorRef = useRef<HTMLTextAreaElement>(null)
  const syncingRef = useRef(false) // guard against scroll-loop

  // Approximate line height for monospace textarea
  const LINE_HEIGHT = 21 // px (text-sm leading-relaxed ~1.625 * 13px ≈ 21px)

  useEffect(() => {
    if (!path) {
      setError('缺少 path 参数')
      setLoading(false)
      return
    }
    fetch(`/admin/api/content/${encodeURIComponent(path)}`)
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`)
        return r.json()
      })
      .then((d: ContentDetail) => {
        setDetail(d)
        setBody(d.rawContent)
        setTitle(d.title)
        setDraft(d.draft)
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [path])

  // Parse headings from markdown body
  const headings: HeadingAnchor[] = useMemo(() => {
    if (!body) return []
    const lines = body.split('\n')
    const result: HeadingAnchor[] = []
    lines.forEach((line, i) => {
      const m = line.match(/^(#{1,6})\s+(.+)$/)
      if (m) {
        result.push({ line: i, slug: slugify(m[2].trim()), text: m[2].trim() })
      }
    })
    return result
  }, [body])

  // Render Markdown to HTML (with heading IDs for scroll sync)
  const renderedHTML = useMemo(() => {
    if (!body) return ''
    try {
      // Custom renderer adds id attributes to headings for scroll sync
      const customRenderer = new marked.Renderer()
      customRenderer.heading = function({ tokens, depth }: { tokens: { raw?: string }[]; depth: number }) {
        const text = tokens.map(t => t.raw ?? '').join('')
        const id = slugify(text)
        return `<h${depth} id="${id}">${text}</h${depth}>\n`
      }
      return marked.parse(body, {
        async: false,
        renderer: customRenderer,
      }) as string
    } catch {
      return '<p style="color: var(--color-destructive-foreground);">渲染错误</p>'
    }
  }, [body])

  // ---- Heading-based scroll sync ----
  const HEADING_PAD = 8 // lines of visual padding before first heading

  const scrollToHeading = useCallback(
    (targetSlug: string) => {
      if (!previewRef.current || syncingRef.current) return
      syncingRef.current = true
      try {
        const el = previewRef.current.querySelector(`[id="${targetSlug}"]`)
        if (el) {
          el.scrollIntoView({ block: 'start', behavior: 'instant' })
        }
      } finally {
        // Deferred release to let the native scroll settle
        requestAnimationFrame(() => { syncingRef.current = false })
      }
    },
    []
  )

  const handleEditorScroll = useCallback(() => {
    if (previewMode !== 'split' || !editorRef.current || syncingRef.current) return

    const el = editorRef.current
    const firstVisibleLine = Math.floor(el.scrollTop / LINE_HEIGHT)

    // Find the nearest heading at or above firstVisibleLine
    let targetHeading: HeadingAnchor | null = null
    for (let i = headings.length - 1; i >= 0; i--) {
      if (headings[i].line <= firstVisibleLine + HEADING_PAD) {
        targetHeading = headings[i]
        break
      }
    }
    // Fall back to first heading
    if (!targetHeading && headings.length > 0) {
      targetHeading = headings[0]
    }

    if (targetHeading) {
      scrollToHeading(targetHeading.slug)
    }
  }, [previewMode, headings, scrollToHeading])

  // ---- API actions ----
  const handleSave = useCallback(async () => {
    if (!detail) return
    setSaving(true)
    setError(null)
    setSuccess(false)
    try {
      const frontmatter = { ...detail.frontmatter, title, draft }
      const res = await fetch(
        `/admin/api/content/${encodeURIComponent(detail.relPath)}`,
        {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ frontmatter, rawContent: body }),
        }
      )
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      setSuccess(true)
      setTimeout(() => setSuccess(false), 2000)
    } catch (e: any) {
      setError(e.message)
    } finally {
      setSaving(false)
    }
  }, [detail, body, title, draft])

  const handleDelete = useCallback(async () => {
    if (!detail) return
    try {
      const res = await fetch(
        `/admin/api/content/${encodeURIComponent(detail.relPath)}`,
        { method: 'DELETE' }
      )
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      navigate('/admin/content')
    } catch (e: any) {
      setError(e.message)
    }
  }, [detail, navigate])

  // ---- Loading / Error states ----
  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-[60vh]">
        <div className="flex flex-col items-center gap-3">
          <div className="w-5 h-5 rounded-full border border-border border-t-foreground animate-spin" />
          <p className="text-sm text-muted-foreground">加载中...</p>
        </div>
      </div>
    )
  }

  if (error && !detail) {
    return (
      <div className="flex items-center justify-center min-h-[60vh]">
        <div className="text-center">
          <p className="text-sm text-destructive-foreground">{error}</p>
          <button
            onClick={() => navigate('/admin/content')}
            className="mt-4 text-sm text-muted-foreground hover:text-foreground underline underline-offset-2 transition-colors"
          >
            返回内容列表
          </button>
        </div>
      </div>
    )
  }

  if (!detail) return null

  const previewModes: { value: PreviewMode; label: string; icon: typeof PenLine }[] = [
    { value: 'edit', label: '编辑', icon: PenLine },
    { value: 'split', label: '分屏', icon: Columns3 },
    { value: 'preview', label: '预览', icon: Eye },
  ]

  const bodyEmpty = !body || body.trim() === ''

  return (
    <div className="h-full flex flex-col">
      {/* ================================================================ */}
      {/* Minimal top bar                                                  */}
      {/* ================================================================ */}
      <div className="flex items-center justify-between shrink-0 mb-4">
        {/* Left cluster */}
        <div className="flex items-center gap-2 min-w-0">
          <button
            onClick={() => navigate('/admin/content')}
            className="text-xs text-muted-foreground hover:text-foreground transition-colors shrink-0 px-1"
            title="返回内容列表"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <span className="text-xs text-muted-foreground/60 select-none">/</span>
          <span className="text-xs text-muted-foreground truncate max-w-[280px]">
            {detail.relPath}
          </span>
        </div>

        {/* Right cluster */}
        <div className="flex items-center gap-1.5">
          {/* Mode toggle — compact icon buttons */}
          {previewModes.map((mode) => (
            <button
              key={mode.value}
              onClick={() => setPreviewMode(mode.value)}
              className={
                `p-1.5 rounded text-xs transition-colors ` +
                (previewMode === mode.value
                  ? 'bg-foreground text-background'
                  : 'text-muted-foreground hover:text-foreground')
              }
              title={mode.label}
            >
              <mode.icon className="h-3.5 w-3.5" />
            </button>
          ))}

          {/* Draft status — small label */}
          <div className="h-3 w-px bg-border mx-0.5" />

          <button
            onClick={() => setDraft(!draft)}
            className={
              `text-[10px] px-1.5 py-0.5 rounded-full border transition-colors ` +
              (draft
                ? 'border-muted-foreground/30 text-muted-foreground'
                : 'border-foreground/30 text-foreground')
            }
          >
            {draft ? '草稿' : '已发布'}
          </button>

          <div className="h-3 w-px bg-border mx-0.5" />

          {/* Delete + Save */}
          {showDeleteConfirm ? (
            <span className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
              <span>确认删除？</span>
              <button onClick={handleDelete} className="px-1.5 py-0.5 rounded bg-destructive-foreground text-background hover:opacity-90">
                确认
              </button>
              <button onClick={() => setShowDeleteConfirm(false)} className="px-1.5 py-0.5 rounded text-muted-foreground hover:text-foreground">
                取消
              </button>
            </span>
          ) : (
            <button
              onClick={() => setShowDeleteConfirm(true)}
              className="p-1.5 rounded text-muted-foreground hover:text-foreground transition-colors"
              title="删除"
            >
              <Trash2 className="h-3.5 w-3.5" />
            </button>
          )}

          <button
            onClick={handleSave}
            disabled={saving}
            className="flex items-center gap-1 px-2.5 py-1.5 text-xs rounded bg-foreground text-background hover:opacity-90 transition-opacity disabled:opacity-40"
          >
            <Save className="h-3 w-3" />
            <span>{saving ? '保存中...' : '保存'}</span>
          </button>
        </div>
      </div>

      {/* ================================================================ */}
      {/* Status toasts (inline, non-intrusive)                            */}
      {/* ================================================================ */}
      {error && (
        <div className="mb-2 px-2.5 py-1.5 rounded bg-muted text-[11px] text-destructive-foreground leading-tight">
          {error}
        </div>
      )}
      {success && (
        <div className="mb-2 px-2.5 py-1.5 rounded bg-muted text-[11px] text-muted-foreground leading-tight">
          已保存
        </div>
      )}

      {/* ================================================================ */}
      {/* Main content — editor + preview                                 */}
      {/* ================================================================ */}
      <div
        className={
          'flex-1 min-h-0 ' +
          (previewMode === 'split' ? 'grid grid-cols-[1.3fr_1fr] gap-5' : '')
        }
      >
        {/* -- Editor -- */}
        {previewMode !== 'preview' && (
          <div className="flex flex-col min-h-0 h-full">
            {/* Title — embedded at the top of the editor panel */}
            <input
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="无标题"
              className="w-full border-0 border-b border-border pb-2 mb-3 px-0 text-base font-medium text-foreground placeholder:text-muted-foreground/30 bg-transparent focus:outline-none focus:border-foreground transition-colors shrink-0"
            />

            {/* Textarea (scrollable area) */}
            <div className="flex-1 min-h-0 relative">
              {bodyEmpty && (
                <div className="absolute inset-0 pointer-events-none flex items-center justify-center z-0">
                  <p className="text-xs text-muted-foreground/25 select-none">
                    开始编写 Markdown...
                  </p>
                </div>
              )}
              <textarea
                ref={editorRef}
                value={body}
                onChange={(e) => setBody(e.target.value)}
                onScroll={handleEditorScroll}
                placeholder=""
                className="relative z-10 w-full h-full resize-none border-0 bg-transparent font-mono text-sm leading-relaxed text-foreground placeholder:text-muted-foreground/25 focus:outline-none"
                spellCheck={false}
              />
            </div>
          </div>
        )}

        {/* -- Preview -- */}
        {previewMode !== 'edit' && (
          <div className="flex flex-col min-h-0 h-full">
            <div
              ref={previewRef}
              className="flex-1 min-h-0 overflow-y-auto"
            >
              {bodyEmpty ? (
                <div className="flex items-center justify-center h-full">
                  <p className="text-xs text-muted-foreground/30">
                    暂无内容
                  </p>
                </div>
              ) : (
                <article
                  className="prose-preview"
                  dangerouslySetInnerHTML={{ __html: renderedHTML }}
                />
              )}
            </div>
          </div>
        )}
      </div>

      {/* ================================================================ */}
      {/* Preview styles — quiet, reading-focused                          */}
      {/* ================================================================ */}
      <style>{`
        .prose-preview {
          line-height: 1.8;
          color: var(--color-foreground);
          font-size: 0.9375rem;
        }
        .prose-preview > *:first-child { margin-top: 0; }

        .prose-preview h1 {
          font-size: 1.5rem; font-weight: 600;
          letter-spacing: -0.025em;
          margin: 2rem 0 0.6rem; line-height: 1.3;
        }
        .prose-preview h2 {
          font-size: 1.25rem; font-weight: 600;
          letter-spacing: -0.015em;
          margin: 1.5rem 0 0.5rem; line-height: 1.35;
        }
        .prose-preview h3 {
          font-size: 1.1rem; font-weight: 600;
          margin: 1.2rem 0 0.4rem; line-height: 1.4;
        }
        .prose-preview h4 {
          font-size: 1rem; font-weight: 600;
          margin: 1rem 0 0.3rem;
        }

        .prose-preview p { margin: 0.7rem 0; }
        .prose-preview a {
          color: var(--color-foreground);
          text-decoration: underline; text-underline-offset: 2px;
          text-decoration-thickness: 1px;
        }
        .prose-preview a:hover { opacity: 0.6; }
        .prose-preview strong { font-weight: 600; }
        .prose-preview em { font-style: italic; }

        .prose-preview blockquote {
          margin: 0.8rem 0;
          padding: 0.1rem 0 0.1rem 1.25rem;
          border-left: 2px solid var(--color-border);
          color: var(--color-muted-foreground);
        }
        .prose-preview blockquote p { margin: 0.3rem 0; }

        .prose-preview ul, .prose-preview ol {
          margin: 0.5rem 0; padding-left: 1.5rem;
        }
        .prose-preview li { margin: 0.2rem 0; }
        .prose-preview li > ul, .prose-preview li > ol { margin: 0.1rem 0; }

        .prose-preview pre {
          margin: 0.8rem 0; padding: 0.875rem 1rem;
          border-radius: 5px; background: var(--color-muted);
          overflow-x: auto; font-size: 0.8125rem; line-height: 1.6;
        }
        .prose-preview code {
          font-family: 'Geist Mono', 'SF Mono', 'Fira Code', 'Consolas', monospace;
          font-size: 0.85em;
        }
        .prose-preview :not(pre) > code {
          padding: 0.12em 0.3em; border-radius: 3px;
          background: var(--color-muted); font-size: 0.82em;
        }

        .prose-preview img {
          max-width: 100%; height: auto;
          border-radius: 4px; margin: 1.25rem 0;
        }
        .prose-preview hr {
          border: none; border-top: 1px solid var(--color-border);
          margin: 1.5rem 0;
        }

        .prose-preview table {
          width: 100%; border-collapse: collapse;
          margin: 0.8rem 0; font-size: 0.875rem;
        }
        .prose-preview th, .prose-preview td {
          padding: 0.4rem 0.7rem; border: 1px solid var(--color-border);
          text-align: left; vertical-align: top;
        }
        .prose-preview th { background: var(--color-muted); font-weight: 500; }
        .prose-preview input[type="checkbox"] {
          margin-right: 0.4em; accent-color: var(--color-foreground);
        }
      `}</style>
    </div>
  )
}
