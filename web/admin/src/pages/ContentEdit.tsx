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

  const renderedHTML = useMemo(() => {
    if (!body) return ''
    try {
      return marked.parse(body, { async: false }) as string
    } catch {
      return '<p style="color: var(--color-destructive-foreground);">渲染错误</p>'
    }
  }, [body])

  // Sync editor scroll → preview scroll
  const handleEditorScroll = useCallback(() => {
    if (previewMode !== 'split' || !previewRef.current || !editorRef.current) return
    const el = editorRef.current
    const ratio = el.scrollTop / (el.scrollHeight - el.clientHeight)
    previewRef.current.scrollTop =
      ratio * (previewRef.current.scrollHeight - previewRef.current.clientHeight)
  }, [previewMode])

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

  // Detect if body is empty (placeholder treatment)
  const bodyEmpty = !body || body.trim() === ''

  return (
    <div className="h-full flex flex-col">
      {/* ================================================================ */}
      {/* Top bar                                                          */}
      {/* ================================================================ */}
      <div className="flex items-center justify-between pb-3 mb-5 border-b border-border">
        {/* Left: back + path */}
        <div className="flex items-center gap-3 min-w-0">
          <button
            onClick={() => navigate('/admin/content')}
            className="flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors shrink-0"
            title="返回内容列表"
          >
            <ArrowLeft className="h-4 w-4" />
            <span className="hidden sm:inline">返回</span>
          </button>
          <div className="h-3 w-px bg-border shrink-0" />
          <span className="text-sm text-muted-foreground truncate max-w-[320px]">
            {detail.relPath}
          </span>
        </div>

        {/* Right: controls */}
        <div className="flex items-center gap-2">
          {/* Preview mode */}
          <div className="flex items-center border border-border rounded-md overflow-hidden">
            {previewModes.map((mode) => (
              <button
                key={mode.value}
                onClick={() => setPreviewMode(mode.value)}
                className={
                  `flex items-center gap-1 px-2.5 py-1.5 text-xs transition-colors ` +
                  (previewMode === mode.value
                    ? 'bg-foreground text-background'
                    : 'text-muted-foreground hover:text-foreground')
                }
                title={mode.label}
              >
                <mode.icon className="h-3.5 w-3.5" />
              </button>
            ))}
          </div>

          <div className="h-4 w-px bg-border" />

          {/* Delete */}
          {showDeleteConfirm ? (
            <span className="flex items-center gap-2 text-xs text-muted-foreground">
              <span>确认删除？</span>
              <button
                onClick={handleDelete}
                className="px-2 py-1 text-xs rounded bg-destructive-foreground text-background hover:opacity-90 transition-opacity"
              >
                确认
              </button>
              <button
                onClick={() => setShowDeleteConfirm(false)}
                className="px-2 py-1 text-xs text-muted-foreground hover:text-foreground transition-colors"
              >
                取消
              </button>
            </span>
          ) : (
            <button
              onClick={() => setShowDeleteConfirm(true)}
              className="flex items-center gap-1 px-2 py-1.5 text-xs rounded border border-border text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
              title="删除此内容"
            >
              <Trash2 className="h-3.5 w-3.5" />
            </button>
          )}

          {/* Save */}
          <button
            onClick={handleSave}
            disabled={saving}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded bg-foreground text-background hover:opacity-90 transition-opacity disabled:opacity-40 disabled:cursor-not-allowed"
          >
            <Save className="h-3.5 w-3.5" />
            <span>{saving ? '保存中...' : '保存'}</span>
          </button>
        </div>
      </div>

      {/* ================================================================ */}
      {/* Status messages                                                  */}
      {/* ================================================================ */}
      {error && (
        <div className="mb-3 px-3 py-2 rounded bg-muted text-xs text-destructive-foreground">
          {error}
        </div>
      )}
      {success && (
        <div className="mb-3 px-3 py-2 rounded bg-muted text-xs text-muted-foreground">
          已保存
        </div>
      )}

      {/* ================================================================ */}
      {/* Editor grid                                                      */}
      {/* ================================================================ */}
      <div
        className={
          'flex-1 min-h-0 ' +
          (previewMode === 'split' ? 'grid grid-cols-2 gap-6' : '')
        }
      >
        {/* -- Editor panel -- */}
        {previewMode !== 'preview' && (
          <div className="flex flex-col min-h-0 h-full gap-4">
            {/* Title */}
            <input
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="无标题"
              className="w-full border-0 border-b border-border pb-2 px-0 text-lg font-medium text-foreground placeholder:text-muted-foreground/40 bg-transparent focus:outline-none focus:border-foreground transition-colors"
            />

            {/* Draft toggle */}
            <div className="flex items-center gap-2.5">
              <button
                onClick={() => setDraft(!draft)}
                className={
                  `relative w-7 h-4 rounded-full border transition-colors ` +
                  (draft ? 'bg-foreground border-foreground' : 'bg-transparent border-border')
                }
                aria-label={draft ? '标记为已发布' : '标记为草稿'}
              >
                <div
                  className={
                    `w-2.5 h-2.5 rounded-full bg-white absolute top-0.5 transition-all ` +
                    (draft ? 'left-[14px]' : 'left-0.5')
                  }
                />
              </button>
              <span className="text-xs text-muted-foreground">
                {draft ? '草稿' : '已发布'}
              </span>
            </div>

            {/* Textarea */}
            <div className="flex-1 min-h-0 relative">
              {bodyEmpty && (
                <div className="absolute inset-0 pointer-events-none flex items-center justify-center z-0">
                  <p className="text-xs text-muted-foreground/30 select-none">
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
                className="relative z-10 w-full h-full resize-none border-0 bg-transparent font-mono text-sm leading-relaxed text-foreground placeholder:text-muted-foreground/30 focus:outline-none"
                spellCheck={false}
              />
            </div>
          </div>
        )}

        {/* -- Preview panel -- */}
        {previewMode !== 'edit' && (
          <div className="flex flex-col min-h-0 h-full">
            <div className="text-xs text-muted-foreground mb-2 shrink-0">
              预览
            </div>
            <div
              ref={previewRef}
              className="flex-1 min-h-0 overflow-y-auto"
            >
              {bodyEmpty ? (
                <div className="flex items-center justify-center h-full">
                  <p className="text-xs text-muted-foreground/40">
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
      {/* Preview styles — designed to match actual rendered page feel      */}
      {/* ================================================================ */}
      <style>{`
        /* === Article preview — clean reading experience === */
        .prose-preview {
          line-height: 1.8;
          color: var(--color-foreground);
          font-size: 0.9375rem;
        }

        .prose-preview > *:first-child {
          margin-top: 0;
        }

        /* Headings */
        .prose-preview h1 {
          font-size: 1.625rem;
          font-weight: 600;
          letter-spacing: -0.025em;
          margin: 2rem 0 0.75rem;
          line-height: 1.3;
        }
        .prose-preview h2 {
          font-size: 1.375rem;
          font-weight: 600;
          letter-spacing: -0.02em;
          margin: 1.75rem 0 0.6rem;
          line-height: 1.35;
        }
        .prose-preview h3 {
          font-size: 1.125rem;
          font-weight: 600;
          margin: 1.25rem 0 0.5rem;
          line-height: 1.4;
        }
        .prose-preview h4 {
          font-size: 1rem;
          font-weight: 600;
          margin: 1rem 0 0.4rem;
        }
        .prose-preview h5, .prose-preview h6 {
          font-size: 0.9375rem;
          font-weight: 600;
          margin: 0.75rem 0 0.3rem;
          color: var(--color-muted-foreground);
        }

        /* Body */
        .prose-preview p {
          margin: 0.75rem 0;
        }

        /* Links */
        .prose-preview a {
          color: var(--color-foreground);
          text-decoration: underline;
          text-underline-offset: 2px;
          text-decoration-thickness: 1px;
          transition: opacity 0.15s;
        }
        .prose-preview a:hover {
          opacity: 0.6;
        }

        /* Inline formatting */
        .prose-preview strong { font-weight: 600; }
        .prose-preview em { font-style: italic; }

        /* Blockquote */
        .prose-preview blockquote {
          margin: 1rem 0;
          padding: 0.25rem 0 0.25rem 1.25rem;
          border-left: 2px solid var(--color-foreground);
          color: var(--color-muted-foreground);
        }
        .prose-preview blockquote p {
          margin: 0.4rem 0;
        }

        /* Lists */
        .prose-preview ul,
        .prose-preview ol {
          margin: 0.6rem 0;
          padding-left: 1.5rem;
        }
        .prose-preview li {
          margin: 0.25rem 0;
        }
        .prose-preview li > ul,
        .prose-preview li > ol {
          margin: 0.2rem 0;
        }

        /* Code */
        .prose-preview pre {
          margin: 1rem 0;
          padding: 1rem 1.125rem;
          border-radius: 6px;
          background: var(--color-muted);
          overflow-x: auto;
          font-size: 0.8125rem;
          line-height: 1.6;
          -webkit-font-smoothing: auto;
        }
        .prose-preview code {
          font-family: 'Geist Mono', 'SF Mono', 'Fira Code', 'Fira Mono', 'Consolas', monospace;
          font-size: 0.85em;
        }
        .prose-preview :not(pre) > code {
          padding: 0.15em 0.35em;
          border-radius: 3px;
          background: var(--color-muted);
          font-size: 0.82em;
        }

        /* Images */
        .prose-preview img {
          max-width: 100%;
          height: auto;
          border-radius: 4px;
          margin: 1.25rem 0;
        }

        /* Horizontal rule */
        .prose-preview hr {
          border: none;
          border-top: 1px solid var(--color-border);
          margin: 2rem 0;
        }

        /* Tables */
        .prose-preview table {
          width: 100%;
          border-collapse: collapse;
          margin: 1rem 0;
          font-size: 0.875rem;
        }
        .prose-preview th,
        .prose-preview td {
          padding: 0.5rem 0.75rem;
          border: 1px solid var(--color-border);
          text-align: left;
          vertical-align: top;
        }
        .prose-preview th {
          background: var(--color-muted);
          font-weight: 500;
        }

        /* Task list (GFM) */
        .prose-preview input[type="checkbox"] {
          margin-right: 0.4em;
          accent-color: var(--color-foreground);
        }
      `}</style>
    </div>
  )
}
