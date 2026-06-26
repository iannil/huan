import { useState, useEffect, useCallback, useMemo, useRef } from 'react'
import { useSearchParams, useNavigate } from 'react-router-dom'
import { ArrowLeft, Save, Trash2, FileText, Eye, PenLine, Columns3 } from 'lucide-react'
import { marked } from 'marked'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'

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

  // Render Markdown to HTML
  const renderedHTML = useMemo(() => {
    if (!body) return '<p style="color: var(--color-muted-foreground);">（空内容）</p>'
    try {
      return marked.parse(body, { async: false }) as string
    } catch {
      return '<p style="color: var(--color-destructive-foreground);">渲染错误</p>'
    }
  }, [body])

  // Auto-scroll preview to match editor scroll position (when in split mode)
  const handleEditorScroll = useCallback(
    (e: React.UIEvent<HTMLTextAreaElement>) => {
      if (previewMode !== 'split' || !previewRef.current) return
      const textarea = e.currentTarget
      const ratio =
        textarea.scrollTop / (textarea.scrollHeight - textarea.clientHeight)
      previewRef.current.scrollTop =
        ratio *
        (previewRef.current.scrollHeight - previewRef.current.clientHeight)
    },
    [previewMode]
  )

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

  if (loading)
    return (
      <div className="text-muted-foreground py-24 text-center text-sm">
        加载中...
      </div>
    )
  if (error && !detail)
    return (
      <div className="text-destructive-foreground py-24 text-center text-sm">
        {error}
      </div>
    )
  if (!detail) return null

  const previewModes: { value: PreviewMode; label: string; icon: typeof PenLine }[] = [
    { value: 'edit', label: '编辑', icon: PenLine },
    { value: 'split', label: '分屏', icon: Columns3 },
    { value: 'preview', label: '预览', icon: Eye },
  ]

  return (
    <div>
      {/* ============ Toolbar ============ */}
      <div className="flex items-center gap-3 pb-3 mb-4">
        <button
          onClick={() => navigate('/admin/content')}
          className="flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors"
        >
          <ArrowLeft className="h-4 w-4" />
          <span>返回</span>
        </button>

        <div className="flex items-center gap-2.5 flex-1 min-w-0">
          <FileText className="h-4 w-4 shrink-0 text-muted-foreground" />
          <span className="text-sm text-foreground truncate">
            {detail.relPath}
          </span>
        </div>

        {/* Preview mode selector */}
        <div className="flex items-center border border-border rounded-md overflow-hidden">
          {previewModes.map((mode) => (
            <button
              key={mode.value}
              onClick={() => setPreviewMode(mode.value)}
              className={
                `flex items-center gap-1 px-2.5 py-1 text-sm transition-colors ` +
                (previewMode === mode.value
                  ? 'bg-muted text-foreground'
                  : 'text-muted-foreground hover:text-foreground')
              }
            >
              <mode.icon className="h-3.5 w-3.5" />
              <span className="hidden sm:inline">{mode.label}</span>
            </button>
          ))}
        </div>

        <div className="flex items-center gap-2">
          {showDeleteConfirm ? (
            <span className="flex items-center gap-2 text-sm text-muted-foreground">
              <span>确认删除？</span>
              <button
                onClick={handleDelete}
                className="px-2 py-1 text-sm rounded-md bg-primary text-primary-foreground hover:opacity-90 transition-opacity"
              >
                确认
              </button>
              <button
                onClick={() => setShowDeleteConfirm(false)}
                className="px-2 py-1 text-sm text-muted-foreground hover:text-foreground transition-colors"
              >
                取消
              </button>
            </span>
          ) : (
            <button
              onClick={() => setShowDeleteConfirm(true)}
              className="flex items-center gap-1.5 px-2.5 py-1.5 text-sm rounded-md border border-border text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
            >
              <Trash2 className="h-3.5 w-3.5" />
              <span className="hidden sm:inline">删除</span>
            </button>
          )}
          <button
            onClick={handleSave}
            disabled={saving}
            className="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md bg-primary text-primary-foreground hover:opacity-90 transition-opacity disabled:opacity-40 disabled:cursor-not-allowed"
          >
            <Save className="h-3.5 w-3.5" />
            <span>{saving ? '保存中...' : '保存'}</span>
          </button>
        </div>
      </div>

      {/* ============ Status messages ============ */}
      {error && (
        <div className="mb-3 px-3 py-2 rounded-md bg-muted text-sm text-destructive-foreground">
          {error}
        </div>
      )}
      {success && (
        <div className="mb-3 px-3 py-2 rounded-md bg-muted text-sm text-muted-foreground">
          保存成功
        </div>
      )}

      {/* ============ Editor area ============ */}
      <div className={previewMode === 'split' ? 'grid grid-cols-2 gap-4' : ''}>
        {/* Editor panel (hidden in preview mode) */}
        {previewMode !== 'preview' && (
          <div className="space-y-4">
            {/* Title */}
            <div>
              <label className="block text-sm text-muted-foreground mb-1.5">
                标题
              </label>
              <Input
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                className="border-0 border-b-2 border-border rounded-none px-0 h-8 text-base focus-visible:border-foreground"
              />
            </div>

            {/* Draft toggle */}
            <div className="flex items-center gap-2.5">
              <div
                className={`w-8 h-5 rounded-full border transition-colors cursor-pointer relative ${
                  draft
                    ? 'bg-foreground border-foreground'
                    : 'bg-transparent border-border'
                }`}
                onClick={() => setDraft(!draft)}
              >
                <div
                  className={`w-3.5 h-3.5 rounded-full bg-white absolute top-0.5 transition-all ${
                    draft ? 'left-4' : 'left-0.5'
                  }`}
                />
              </div>
              <span className="text-sm text-foreground">草稿</span>
            </div>

            {/* Markdown body */}
            <div>
              <label className="block text-sm text-muted-foreground mb-1.5">
                Markdown 内容
              </label>
              <Textarea
                value={body}
                onChange={(e) => setBody(e.target.value)}
                onScroll={handleEditorScroll}
                rows={28}
                className={
                  `font-mono text-sm border-0 border-b-2 border-border rounded-none px-0 leading-relaxed focus-visible:border-foreground ` +
                  (previewMode === 'edit' ? '' : 'h-[calc(100vh-280px)] min-h-[400px]')
                }
              />
            </div>
          </div>
        )}

        {/* Preview panel (hidden in edit mode) */}
        {previewMode !== 'edit' && (
          <div>
            <label className="block text-sm text-muted-foreground mb-1.5">
              预览
            </label>
            <div
              ref={previewRef}
              className={
                `prose prose-sm max-w-none overflow-y-auto border-0 border-b-2 border-border pb-2 ` +
                (previewMode === 'preview'
                  ? ''
                  : 'h-[calc(100vh-280px)] min-h-[400px]')
              }
            >
              <div
                className="markdown-preview"
                dangerouslySetInnerHTML={{ __html: renderedHTML }}
              />
            </div>
          </div>
        )}
      </div>

      {/* ============ CSS for rendered Markdown ============ */}
      <style>{`
        .markdown-preview {
          line-height: 1.7;
          color: var(--color-foreground);
        }
        .markdown-preview h1 { font-size: 1.5rem; font-weight: 600; margin: 1.5rem 0 0.75rem; letter-spacing: -0.02em; }
        .markdown-preview h2 { font-size: 1.25rem; font-weight: 600; margin: 1.25rem 0 0.6rem; letter-spacing: -0.01em; }
        .markdown-preview h3 { font-size: 1.1rem; font-weight: 600; margin: 1rem 0 0.5rem; }
        .markdown-preview h4 { font-size: 1rem; font-weight: 600; margin: 0.75rem 0 0.4rem; }
        .markdown-preview p { margin: 0.5rem 0; }
        .markdown-preview a { color: var(--color-foreground); text-decoration: underline; text-underline-offset: 2px; }
        .markdown-preview a:hover { opacity: 0.7; }
        .markdown-preview strong { font-weight: 600; }
        .markdown-preview em { font-style: italic; }
        .markdown-preview blockquote {
          margin: 0.75rem 0;
          padding: 0.25rem 1rem;
          border-left: 2px solid var(--color-border);
          color: var(--color-muted-foreground);
        }
        .markdown-preview ul, .markdown-preview ol {
          margin: 0.5rem 0;
          padding-left: 1.5rem;
        }
        .markdown-preview li { margin: 0.2rem 0; }
        .markdown-preview pre {
          margin: 0.75rem 0;
          padding: 1rem;
          border-radius: 6px;
          background: var(--color-muted);
          overflow-x: auto;
          font-size: 0.85rem;
          line-height: 1.5;
        }
        .markdown-preview code {
          font-family: 'Geist Mono', 'SF Mono', 'Fira Code', 'Fira Mono', monospace;
          font-size: 0.85em;
        }
        .markdown-preview p > code, .markdown-preview li > code {
          padding: 0.15em 0.3em;
          border-radius: 3px;
          background: var(--color-muted);
        }
        .markdown-preview img {
          max-width: 100%;
          height: auto;
          border-radius: 6px;
          margin: 1rem 0;
        }
        .markdown-preview hr {
          border: none;
          border-top: 1px solid var(--color-border);
          margin: 1.5rem 0;
        }
        .markdown-preview table {
          width: 100%;
          border-collapse: collapse;
          margin: 0.75rem 0;
          font-size: 0.9rem;
        }
        .markdown-preview th, .markdown-preview td {
          padding: 0.4rem 0.75rem;
          border: 1px solid var(--color-border);
          text-align: left;
        }
        .markdown-preview th {
          background: var(--color-muted);
          font-weight: 500;
        }
        .markdown-preview tr:nth-child(even) {
          background: var(--color-muted);
        }
      `}</style>
    </div>
  )
}
