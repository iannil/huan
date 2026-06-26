import { useState, useEffect, useCallback, useMemo, useRef } from 'react'
import { useSearchParams, useNavigate } from 'react-router-dom'
import { ArrowLeft, Save, FileText } from 'lucide-react'
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

/** Escape HTML entities so user text is safe for innerHTML. */
function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

/**
 * Lightweight Markdown syntax highlighting via regex.
 * Returns HTML with <span> colour tags — safe because input is escaped first.
 */
function highlightMarkdown(text: string): string {
  if (!text) return ''
  let html = escapeHtml(text)

  // Block-level rules (applied line-by-line)
  const lines = html.split('\n')
  html = lines
    .map((line) => {
      // Headings (H1–H6)
      if (/^(#{1,6})\s/.test(line)) {
        return `<span class="hl-heading">${line}</span>`
      }
      // Horizontal rules
      if (/^(---|\*\*\*|___)\s*$/.test(line)) {
        return `<span class="hl-hr">${line}</span>`
      }
      // Blockquotes
      if (/^(&gt;+)/.test(line)) {
        return line.replace(
          /^(&gt;+)/,
          '<span class="hl-quote">$1</span>'
        )
      }
      // Unordered list markers
      if (/^(\s*)([-*+])\s/.test(line)) {
        return line.replace(
          /^(\s*)([-*+])(\s)/,
          '$1<span class="hl-bullet">$2</span>$3'
        )
      }
      // Ordered list markers
      if (/^(\s*)(\d+)(\.)\s/.test(line)) {
        return line.replace(
          /^(\s*)(\d+)(\.\s)/,
          '$1<span class="hl-list-num">$2$3</span>'
        )
      }
      return line
    })
    .join('\n')

  // Inline rules
  // Inline code
  html = html.replace(
    /(`[^`]+`)/g,
    '<span class="hl-code">$1</span>'
  )
  // Images ![alt](url)
  html = html.replace(
    /(!\[[^\]]*\]\([^)]*\))/g,
    '<span class="hl-image">$1</span>'
  )
  // Links [text](url) – only if not already inside an image
  html = html.replace(
    /(\[[^\]]*\]\([^)]*\))/g,
    '<span class="hl-link">$1</span>'
  )
  // Bold **text**
  html = html.replace(/(\*\*[^*]+\*\*)/g, '<span class="hl-bold">$1</span>')
  // Italic *text*
  html = html.replace(/(\*[^*]+\*)/g, '<span class="hl-italic">$1</span>')
  // Strikethrough ~~text~~
  html = html.replace(
    /(~~[^~]+~~)/g,
    '<span class="hl-strike">$1</span>'
  )

  return html
}

/**
 * ContentEdit — full‑screen editor with syntax highlighting,
 * live preview, and a clean toolbar.
 */
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
  const [fatalError, setFatalError] = useState<string | null>(null)

  const previewRef = useRef<HTMLDivElement>(null)
  const editorRef = useRef<HTMLTextAreaElement>(null)
  const highlightRef = useRef<HTMLPreElement>(null)
  const gutterRef = useRef<HTMLDivElement>(null)
  const syncing = useRef(false)

  const LINE_HEIGHT = 24
  const HEADING_PAD = 8

  // ---- Word & line counts ----
  const wordCount = useMemo(() => {
    if (!body || !body.trim()) return 0
    return body.trim().split(/\s+/).length
  }, [body])

  const lineCount = useMemo(() => {
    if (!body) return 0
    return body.split('\n').length
  }, [body])

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

  // ---- Syntax-highlighted body ----
  const highlightedBody = useMemo(() => {
    return highlightMarkdown(body || '')
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

  // ---- Sync gutter + highlight overlay scroll with editor ----
  const syncEditorScroll = useCallback(() => {
    if (!editorRef.current) return
    const st = editorRef.current.scrollTop
    if (gutterRef.current) gutterRef.current.scrollTop = st
    if (highlightRef.current) highlightRef.current.scrollTop = st
  }, [])

  const handleEditorScroll = useCallback(() => {
    syncEditorScroll()
    onEditorScroll()
  }, [syncEditorScroll, onEditorScroll])

  // ---- Dirty tracking ----
  const onBodyChange = useCallback((v: string) => { setBody(v); setDirty(true) }, [])
  const onTitleChange = useCallback((v: string) => { setTitle(v); setDirty(true) }, [])

  // ---- Draft toggle ----
  const toggleDraft = useCallback(() => {
    setDraft(d => !d)
    setDirty(true)
  }, [])

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
      // non-fatal
    } finally {
      setSaving(false)
    }
  }, [detail, body, title, draft])

  // ---- Keyboard shortcut: Ctrl/Cmd + S ----
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 's') {
        e.preventDefault()
        if (dirty && !saving) handleSave()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [dirty, saving, handleSave])

  // ---- Build line-number array ----
  const lineNumbers = useMemo(() => {
    if (!body) return ['1']
    const n = body.split('\n').length
    return Array.from({ length: Math.max(1, n) }, (_, i) => String(i + 1))
  }, [body])

  const empty = !body || !body.trim()

  // ---- Loading ----
  if (loading) {
    return (
      <div className="fixed inset-0 flex items-center justify-center bg-background">
        <div className="w-4 h-4 rounded-full border border-border border-t-foreground animate-spin" />
      </div>
    )
  }

  // ---- Fatal error ----
  if (fatalError) {
    return (
      <div className="fixed inset-0 flex items-center justify-center bg-background">
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

  return (
    <div className="fixed inset-0 flex flex-col bg-background select-none">
      {/* ================================================================ */}
      {/* Toolbar — clean, solid, aligned with top-nav style               */}
      {/* ================================================================ */}
      <div className="shrink-0 flex items-center justify-between h-12 px-5 bg-background border-b border-border z-20">
        {/* Left: back + file icon + path */}
        <div className="flex items-center gap-3 min-w-0">
          <button
            onClick={() => navigate('/admin/content')}
            className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors shrink-0"
          >
            <ArrowLeft className="h-3.5 w-3.5" />
            <span className="hidden sm:inline">返回</span>
          </button>
          <span className="hidden sm:block h-3 w-px bg-border" />
          <FileText className="h-3.5 w-3.5 text-muted-foreground/40 shrink-0 hidden sm:block" />
          <span className="text-xs text-muted-foreground/50 font-mono truncate max-w-[360px] select-none">
            {detail.relPath}
          </span>
        </div>

        {/* Right: draft toggle + save */}
        <div className="flex items-center gap-4">
          {/* Draft toggle */}
          <label
            className="flex items-center gap-2 cursor-pointer select-none"
            onClick={toggleDraft}
          >
            <span className="text-xs text-muted-foreground hover:text-foreground transition-colors">
              草稿
            </span>
            <span className={`relative inline-flex h-4 w-7 items-center rounded-full transition-colors duration-150 ${
              draft ? 'bg-muted-foreground/40' : 'bg-border'
            }`}>
              <span className={`inline-block h-3 w-3 rounded-full bg-white shadow-sm transform transition-transform duration-150 ${
                draft ? 'translate-x-0.5' : 'translate-x-[14px]'
              }`} />
            </span>
          </label>

          {/* Save button */}
          <button
            onClick={handleSave}
            disabled={saving}
            className={`flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-md transition-all duration-150 ${
              dirty && !saving
                ? 'bg-foreground text-background hover:opacity-85 active:opacity-70'
                : saving
                  ? 'bg-muted text-muted-foreground cursor-wait'
                  : 'bg-transparent text-muted-foreground/40 cursor-default'
            }`}
          >
            <Save className={`h-3.5 w-3.5 ${saving ? 'animate-pulse' : ''}`} />
            {saving ? '保存中…' : dirty ? '保存' : '已保存'}
          </button>
        </div>
      </div>

      {/* ================================================================ */}
      {/* Main split: editor (55%) / preview (45%)                         */}
      {/* ================================================================ */}
      <div className="flex-1 min-h-0 flex">
        {/* ==================== Editor ==================== */}
        <div className="flex min-h-0 h-full w-[55%] bg-[#fafafa] border-r border-border">
          {/* Line-number gutter */}
          <div
            ref={gutterRef}
            className="shrink-0 w-11 overflow-hidden border-r border-border/20 bg-[#f5f5f5] select-none"
          >
            <div className="pt-[100px] pb-4">
              {lineNumbers.map((n, i) => (
                <div
                  key={i}
                  className="text-right pr-3 text-xs leading-[24px] text-muted-foreground/25 tabular-nums"
                >
                  {n}
                </div>
              ))}
            </div>
          </div>

          {/* Editor content */}
          <div className="flex flex-col flex-1 min-w-0">
            {/* Title */}
            <div className="shrink-0 px-8 pt-6 pb-0">
              <input
                value={title}
                onChange={e => onTitleChange(e.target.value)}
                placeholder="无标题"
                className="w-full border-0 bg-transparent text-xl font-semibold text-foreground tracking-tight placeholder:text-muted-foreground/12 px-0 pb-4
                  focus:outline-none border-b border-border/20 focus:border-foreground/30 transition-colors"
              />
            </div>

            {/* Body with syntax-highlight overlay */}
            <div className="flex-1 min-h-0 relative px-8 pt-5 pb-4">
              {empty && (
                <div className="absolute inset-0 flex items-center justify-center pointer-events-none z-0 px-8 pb-4">
                  <p className="text-sm text-muted-foreground/12 select-none tracking-wider">开始编写</p>
                </div>
              )}
              {/* Highlighted overlay — mirrors textarea content */}
              <pre
                ref={highlightRef}
                className="absolute inset-0 z-0 overflow-hidden px-8 pt-5 pb-4 m-0 font-mono text-sm leading-[24px] whitespace-pre-wrap break-words pointer-events-none"
                aria-hidden="true"
              >
                <code
                  className="font-mono text-sm leading-[24px]"
                  dangerouslySetInnerHTML={{ __html: highlightedBody || '&nbsp;' }}
                />
              </pre>
              {/* Transparent textarea on top */}
              <textarea
                ref={editorRef}
                value={body}
                onChange={e => onBodyChange(e.target.value)}
                onScroll={handleEditorScroll}
                className="relative z-10 w-full h-full resize-none border-0 bg-transparent
                  font-mono text-sm leading-[24px] text-transparent caret-foreground
                  placeholder:text-muted-foreground/20 focus:outline-none"
                spellCheck={false}
              />
            </div>
          </div>
        </div>

        {/* ==================== Preview ==================== */}
        <div className="flex flex-col min-h-0 h-full w-[45%] bg-white">
          <div
            ref={previewRef}
            className="flex-1 min-h-0 overflow-y-auto px-12 py-10"
          >
            {empty ? (
              <div className="flex items-center justify-center h-full">
                <p className="text-sm text-muted-foreground/15 tracking-wider">预览</p>
              </div>
            ) : (
              <article
                className="prose-preview max-w-[38rem] mx-auto"
                dangerouslySetInnerHTML={{ __html: renderedHTML }}
              />
            )}
          </div>
        </div>
      </div>

      {/* ================================================================ */}
      {/* Status bar — compact but readable                                 */}
      {/* ================================================================ */}
      <div className="shrink-0 h-7 px-5 border-t border-border bg-background flex items-center gap-5 text-[11px] text-muted-foreground/35">
        <span className="tabular-nums">{lineCount} 行</span>
        <span className="tabular-nums">{wordCount} 词</span>
        <span className="ml-auto text-muted-foreground/20">⌘S 保存</span>
        {dirty && (
          <span className="text-foreground/25 tabular-nums">未保存</span>
        )}
      </div>

      {/* ================================================================ */}
      {/* Preview typography + syntax highlighting styles                  */}
      {/* ================================================================ */}
      <style>{`
        /* ---- Preview typography (unchanged, still minimal) ---- */
        .prose-preview {
          line-height: 1.85;
          color: #111;
          font-size: 0.9375rem;
        }
        .prose-preview > *:first-child { margin-top: 0; }
        .prose-preview h1 {
          font-size: 1.5rem; font-weight: 600;
          letter-spacing: -0.025em;
          margin: 2.5rem 0 0.6rem; line-height: 1.3;
        }
        .prose-preview h2 {
          font-size: 1.25rem; font-weight: 600;
          letter-spacing: -0.015em;
          margin: 2rem 0 0.5rem; line-height: 1.35;
        }
        .prose-preview h3 {
          font-size: 1.1rem; font-weight: 600;
          margin: 1.5rem 0 0.4rem; line-height: 1.4;
        }
        .prose-preview h4 { font-size: 1rem; font-weight: 600; margin: 1.25rem 0 0.3rem; }
        .prose-preview p { margin: 0.8rem 0; }
        .prose-preview a {
          color: #111; text-decoration: underline;
          text-underline-offset: 2px; text-decoration-thickness: 1px;
        }
        .prose-preview a:hover { opacity: 0.6; }
        .prose-preview strong { font-weight: 600; }
        .prose-preview em { font-style: italic; }
        .prose-preview blockquote {
          margin: 1rem 0;
          padding: 0.15rem 0 0.15rem 1.25rem;
          border-left: 2px solid #e5e5e5;
          color: #6b6b6b;
        }
        .prose-preview blockquote p { margin: 0.3rem 0; }
        .prose-preview ul, .prose-preview ol {
          margin: 0.6rem 0; padding-left: 1.5rem;
        }
        .prose-preview li { margin: 0.25rem 0; }
        .prose-preview li > ul, .prose-preview li > ol { margin: 0.1rem 0; }
        .prose-preview pre {
          margin: 1rem 0; padding: 0.875rem 1rem;
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
          border-radius: 4px; margin: 1.5rem 0;
        }
        .prose-preview hr {
          border: none; border-top: 1px solid #e5e5e5;
          margin: 2rem 0;
        }
        .prose-preview table {
          width: 100%; border-collapse: collapse;
          margin: 1rem 0; font-size: 0.875rem;
        }
        .prose-preview th, .prose-preview td {
          padding: 0.4rem 0.7rem; border: 1px solid #e5e5e5;
          text-align: left; vertical-align: top;
        }
        .prose-preview th { background: #f2f2f2; font-weight: 500; }
        .prose-preview input[type="checkbox"] {
          margin-right: 0.4em; accent-color: #111;
        }

        /* ---- Syntax highlighting in editor ---- */
        .hl-heading { color: #7c3aed; font-weight: 600; }
        .hl-bold { color: #2563eb; font-weight: 600; }
        .hl-italic { color: #059669; font-style: italic; }
        .hl-code { color: #d97706; }
        .hl-link { color: #2563eb; text-decoration: underline; text-underline-offset: 2px; }
        .hl-image { color: #059669; }
        .hl-quote { color: #6b7280; }
        .hl-bullet { color: #d97706; font-weight: 600; }
        .hl-list-num { color: #d97706; font-weight: 600; }
        .hl-hr { color: #d4d4d4; }
        .hl-strike { color: #9ca3af; text-decoration: line-through; }
      `}</style>
    </div>
  )
}
