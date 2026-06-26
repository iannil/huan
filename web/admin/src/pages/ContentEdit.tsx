import { useState, useEffect, useCallback, useMemo, useRef } from 'react'
import { useSearchParams, useNavigate } from 'react-router-dom'
import { ArrowLeft, Save } from 'lucide-react'
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

/** GitHub-style slug matching marked's default. */
function slugify(text: string): string {
  return text
    .toLowerCase()
    .replace(/[^\w\s-]/g, '')
    .replace(/\s+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-+|-+$/g, '')
}

interface HeadingAnchor {
  line: number
  slug: string
  text: string
}

export default function ContentEdit() {
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const path = searchParams.get('path') || ''

  const [detail, setDetail] = useState<ContentDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [body, setBody] = useState('')
  const [title, setTitle] = useState('')
  const [draft, setDraft] = useState(false)
  const [dirty, setDirty] = useState(false)

  // Only show error if we can't recover
  const [fatalError, setFatalError] = useState<string | null>(null)

  const previewRef = useRef<HTMLDivElement>(null)
  const editorRef = useRef<HTMLTextAreaElement>(null)
  const syncing = useRef(false)

  const LINE_HEIGHT = 21
  const HEADING_PAD = 8

  // ---- Load ----
  useEffect(() => {
    if (!path) { setFatalError('Missing path'); setLoading(false); return }
    fetch(`/admin/api/content/${encodeURIComponent(path)}`)
      .then(r => { if (!r.ok) throw new Error(`HTTP ${r.status}`); return r.json() })
      .then(d => { setDetail(d); setBody(d.rawContent); setTitle(d.title); setDraft(d.draft) })
      .catch(e => setFatalError(e.message))
      .finally(() => setLoading(false))
  }, [path])

  // ---- Heading extraction ----
  const headings: HeadingAnchor[] = useMemo(() => {
    if (!body) return []
    return body.split('\n').reduce<HeadingAnchor[]>((acc, line, i) => {
      const m = line.match(/^(#{1,6})\s+(.+)$/)
      if (m) acc.push({ line: i, slug: slugify(m[2].trim()), text: m[2].trim() })
      return acc
    }, [])
  }, [body])

  // ---- Rendered HTML with heading IDs ----
  const renderedHTML = useMemo(() => {
    if (!body) return ''
    try {
      const r = new marked.Renderer()
      r.heading = function({ tokens, depth }) {
        const text = tokens.map(t => (t as any).raw ?? '').join('')
        return `<h${depth} id="${slugify(text)}">${text}</h${depth}>\n`
      }
      return marked.parse(body, { async: false, renderer: r }) as string
    } catch { return '' }
  }, [body])

  // ---- Heading-based scroll sync ----
  const scrollToHeading = useCallback((slug: string) => {
    if (!previewRef.current || syncing.current) return
    syncing.current = true
    const el = previewRef.current.querySelector(`[id="${slug}"]`)
    if (el) el.scrollIntoView({ block: 'start', behavior: 'instant' })
    requestAnimationFrame(() => { syncing.current = false })
  }, [])

  const onEditorScroll = useCallback(() => {
    if (!editorRef.current || syncing.current) return
    const firstLine = Math.floor(editorRef.current.scrollTop / LINE_HEIGHT)
    let target: HeadingAnchor | null = null
    for (let i = headings.length - 1; i >= 0; i--) {
      if (headings[i].line <= firstLine + HEADING_PAD) { target = headings[i]; break }
    }
    if (!target && headings.length) target = headings[0]
    if (target) scrollToHeading(target.slug)
  }, [headings, scrollToHeading])

  // ---- Dirty tracking ----
  const onBodyChange = useCallback((v: string) => { setBody(v); setDirty(true) }, [])
  const onTitleChange = useCallback((v: string) => { setTitle(v); setDirty(true) }, [])

  // ---- Save ----
  const handleSave = useCallback(async () => {
    if (!detail) return
    setSaving(true)
    try {
      const fm = { ...detail.frontmatter, title, draft }
      const res = await fetch(`/admin/api/content/${encodeURIComponent(detail.relPath)}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ frontmatter: fm, rawContent: body }),
      })
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      setDirty(false)
    } catch (e: any) {
      // Non-fatal — just flash the save button
    } finally {
      setSaving(false)
    }
  }, [detail, body, title, draft])

  // ---- States ----
  if (loading) {
    return (
      <div className="flex items-center justify-center h-screen bg-background">
        <div className="w-4 h-4 rounded-full border border-border border-t-foreground animate-spin" />
      </div>
    )
  }

  if (fatalError) {
    return (
      <div className="flex items-center justify-center h-screen bg-background">
        <div className="text-center">
          <p className="text-sm text-muted-foreground">{fatalError}</p>
          <button onClick={() => navigate('/admin/content')}
            className="mt-3 text-xs text-muted-foreground hover:text-foreground underline underline-offset-2">
            返回内容列表
          </button>
        </div>
      </div>
    )
  }

  if (!detail) return null
  const empty = !body || !body.trim()

  return (
    <div className="h-screen flex flex-col bg-background">
      {/* ================================================================ */}
      {/* Top bar — as few things as possible                             */}
      {/* ================================================================ */}
      <div className="flex items-center justify-between shrink-0 h-10 px-4 border-b border-border">
        <div className="flex items-center gap-2 min-w-0">
          <button onClick={() => navigate('/admin/content')}
            className="text-muted-foreground/50 hover:text-foreground transition-colors shrink-0">
            <ArrowLeft className="h-4 w-4" />
          </button>
          <span className="text-[11px] text-muted-foreground/40 truncate max-w-[240px] select-none">
            {detail.relPath}
          </span>
        </div>

        <div className="flex items-center gap-2">
          {/* Draft indicator — small dot */}
          <span className={`inline-block w-1.5 h-1.5 rounded-full ${draft ? 'bg-muted-foreground/40' : 'bg-foreground/60'}`}
            title={draft ? '草稿' : '已发布'} />

          {/* Save */}
          <button onClick={handleSave} disabled={saving}
            className={`flex items-center gap-1 px-2.5 py-1 text-[11px] rounded transition-all ${
              dirty && !saving
                ? 'bg-foreground text-background'
                : saving
                  ? 'bg-muted text-muted-foreground'
                  : 'text-muted-foreground/40 cursor-default'
            }`}
          >
            <Save className="h-3 w-3" />
            <span className={saving ? '' : dirty ? 'inline' : 'hidden'}>{saving ? '保存中' : '保存'}</span>
          </button>
        </div>
      </div>

      {/* ================================================================ */}
      {/* Split: editor (warm) / preview (pure white)                     */}
      {/* ================================================================ */}
      <div className="flex-1 min-h-0 grid grid-cols-[1.2fr_1fr]">
        {/* ============ Editor panel ============ */}
        <div className="flex flex-col min-h-0 h-full bg-[#fafafa] border-r border-border">
          {/* Title — part of the editor, not a separate toolbar */}
          <div className="shrink-0 px-5 pt-5 pb-0">
            <input value={title}
              onChange={e => onTitleChange(e.target.value)}
              placeholder="无标题"
              className="w-full border-0 bg-transparent text-lg font-medium text-foreground placeholder:text-muted-foreground/20 px-0 pb-2
                focus:outline-none border-b border-border/50 focus:border-foreground transition-colors" />
          </div>

          {/* Editor body */}
          <div className="flex-1 min-h-0 relative px-5 pt-3 pb-5">
            {empty && (
              <div className="absolute inset-0 flex items-center justify-center pointer-events-none z-0 px-5 pb-5">
                <p className="text-xs text-muted-foreground/20 select-none">开始编写</p>
              </div>
            )}
            <textarea ref={editorRef}
              value={body}
              onChange={e => onBodyChange(e.target.value)}
              onScroll={onEditorScroll}
              className="relative z-10 w-full h-full resize-none border-0 bg-transparent
                font-mono text-sm leading-relaxed text-foreground
                placeholder:text-muted-foreground/20 focus:outline-none"
              spellCheck={false} />
          </div>
        </div>

        {/* ============ Preview panel ============ */}
        <div className="flex flex-col min-h-0 h-full bg-white">
          <div ref={previewRef} className="flex-1 min-h-0 overflow-y-auto px-8 py-10">
            {empty ? (
              <div className="flex items-center justify-center h-full">
                <p className="text-xs text-muted-foreground/20">预览</p>
              </div>
            ) : (
              <article className="prose-preview max-w-[42rem] mx-auto"
                dangerouslySetInnerHTML={{ __html: renderedHTML }} />
            )}
          </div>
        </div>
      </div>

      {/* ================================================================ */}
      {/* Styles                                                          */}
      {/* ================================================================ */}
      <style>{`
        .prose-preview {
          line-height: 1.8;
          color: #111;
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
        .prose-preview h4 { font-size: 1rem; font-weight: 600; margin: 1rem 0 0.3rem; }
        .prose-preview p { margin: 0.7rem 0; }
        .prose-preview a {
          color: #111; text-decoration: underline;
          text-underline-offset: 2px; text-decoration-thickness: 1px;
        }
        .prose-preview a:hover { opacity: 0.6; }
        .prose-preview strong { font-weight: 600; }
        .prose-preview em { font-style: italic; }
        .prose-preview blockquote {
          margin: 0.8rem 0;
          padding: 0.1rem 0 0.1rem 1.25rem;
          border-left: 2px solid #e5e5e5;
          color: #6b6b6b;
        }
        .prose-preview blockquote p { margin: 0.3rem 0; }
        .prose-preview ul, .prose-preview ol {
          margin: 0.5rem 0; padding-left: 1.5rem;
        }
        .prose-preview li { margin: 0.2rem 0; }
        .prose-preview li > ul, .prose-preview li > ol { margin: 0.1rem 0; }
        .prose-preview pre {
          margin: 0.8rem 0; padding: 0.875rem 1rem;
          border-radius: 5px; background: #f2f2f2;
          overflow-x: auto; font-size: 0.8125rem; line-height: 1.6;
        }
        .prose-preview code {
          font-family: 'Geist Mono', 'SF Mono', 'Fira Code', 'Consolas', monospace;
          font-size: 0.85em;
        }
        .prose-preview :not(pre) > code {
          padding: 0.12em 0.3em; border-radius: 3px;
          background: #f2f2f2; font-size: 0.82em;
        }
        .prose-preview img {
          max-width: 100%; height: auto;
          border-radius: 4px; margin: 1.25rem 0;
        }
        .prose-preview hr {
          border: none; border-top: 1px solid #e5e5e5;
          margin: 1.5rem 0;
        }
        .prose-preview table {
          width: 100%; border-collapse: collapse;
          margin: 0.8rem 0; font-size: 0.875rem;
        }
        .prose-preview th, .prose-preview td {
          padding: 0.4rem 0.7rem; border: 1px solid #e5e5e5;
          text-align: left; vertical-align: top;
        }
        .prose-preview th { background: #f2f2f2; font-weight: 500; }
        .prose-preview input[type="checkbox"] {
          margin-right: 0.4em; accent-color: #111;
        }
      `}</style>
    </div>
  )
}
