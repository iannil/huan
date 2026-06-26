import { useEffect, useState, useCallback, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  FileText,
  Plus,
  Search,
  ArrowUpDown,
  ArrowUp,
  ArrowDown,
  ChevronLeft,
  ChevronRight,
  Trash2,
  Check,
  X,
  ChevronRight as ChevronRightIcon,
  Folder,
  File,
} from 'lucide-react'

interface ContentItem {
  title: string
  relPath: string
  filePath: string
  section: string
  kind: string
  draft: boolean
  date: string
  tags: string[]
  url: string
  language: string
}

interface ContentListResponse {
  sections: Record<string, ContentItem[]>
  tree: TreeNode[]
  total: number
}

interface TreeNode {
  name: string
  path: string
  type: 'folder' | 'file'
  count: number
  children: TreeNode[]
}

type SortField = 'title' | 'date'
type SortDir = 'asc' | 'desc'
type DraftFilter = 'all' | 'draft' | 'published'

const PAGE_SIZES = [20, 50, 100] as const

// ---------- LanguageBadge — subtle, per-language tint. ----------
function LanguageBadge({ language }: { language: string }) {
  if (!language) {
    return (
      <span className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-mono font-medium uppercase tracking-wider bg-muted text-muted-foreground shrink-0">
        默认
      </span>
    )
  }
  const colorClass =
    language === 'en'
      ? 'bg-[#eef2ff] text-[#4f46e5]'
      : language === 'zh-CN' || language === 'zh'
        ? 'bg-[#fff7ed] text-[#d97706]'
        : 'bg-muted text-muted-foreground'
  return (
    <span
      className={`inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-mono font-medium uppercase tracking-wider shrink-0 ${colorClass}`}
    >
      {language}
    </span>
  )
}

// ---------- TreeView — collapsible folder tree. ----------
function TreeView({
  nodes,
  depth = 0,
  activePath,
  expandedPaths,
  onToggle,
  onSelect,
}: {
  nodes: TreeNode[]
  depth?: number
  activePath: string
  expandedPaths: Set<string>
  onToggle: (path: string) => void
  onSelect: (path: string) => void
}) {
  return (
    <div className={depth > 0 ? 'ml-2' : ''}>
      {nodes.map((node) => {
        const isExpanded = expandedPaths.has(node.path)
        const isActive = activePath === node.path
        return (
          <div key={node.path}>
            <button
              onClick={() => {
                if (node.type === 'folder') {
                  onToggle(node.path)
                }
                onSelect(node.path)
              }}
              className={
                `w-full text-left px-2.5 py-1 text-sm rounded-md transition-colors flex items-center gap-1.5 ` +
                (isActive
                  ? 'bg-neutral-100 text-foreground font-medium'
                  : 'text-muted-foreground hover:text-foreground hover:bg-neutral-50')
              }
            >
              {node.type === 'folder' && (
                <ChevronRightIcon
                  className={
                    `h-3 w-3 shrink-0 transition-transform text-muted-foreground/40 ` +
                    (isExpanded ? 'rotate-90' : '')
                  }
                />
              )}
              {node.type === 'file' && (
                <FileText className="h-3 w-3 shrink-0 text-muted-foreground/40" />
              )}
              <span className="truncate flex-1">{node.name}</span>
              {node.type === 'folder' && (
                <span className="text-muted-foreground/40 text-xs tabular-nums">
                  {node.count}
                </span>
              )}
            </button>
            {node.type === 'folder' && isExpanded && node.children.length > 0 && (
              <TreeView
                nodes={node.children}
                depth={depth + 1}
                activePath={activePath}
                expandedPaths={expandedPaths}
                onToggle={onToggle}
                onSelect={onSelect}
              />
            )}
          </div>
        )
      })}
    </div>
  )
}

// ---------- Page header breadcrumb. ----------
function Breadcrumb({ path }: { path: string }) {
  if (!path) return null
  const parts = path.split('/')
  return (
    <div className="flex items-center gap-1 text-xs text-muted-foreground mb-1">
      <span>全部</span>
      {parts.map((part, i) => (
        <span key={i} className="flex items-center gap-1">
          <span className="text-muted-foreground/30">/</span>
          <span className={i === parts.length - 1 ? 'text-foreground font-medium' : ''}>
            {part}
          </span>
        </span>
      ))}
    </div>
  )
}

// ---------- Filter chip. ----------
function FilterChip({
  active,
  label,
  count,
  onClick,
}: {
  active: boolean
  label: string
  count?: number
  onClick: () => void
}) {
  return (
    <button
      onClick={onClick}
      className={
        `px-2.5 py-1 text-sm rounded-md transition-colors flex items-center gap-1.5 ` +
        (active
          ? 'bg-neutral-100 text-foreground font-medium'
          : 'text-muted-foreground hover:text-foreground hover:bg-neutral-50')
      }
    >
      {label}
      {count !== undefined && (
        <span className="text-xs tabular-nums text-muted-foreground/50">{count}</span>
      )}
    </button>
  )
}

// ========================= Page component =========================
export default function ContentList() {
  const [data, setData] = useState<ContentListResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const navigate = useNavigate()

  // Pure local state for the active path — no URL-sync interference.
  const [activePath, setActivePath] = useState(() => {
    if (typeof window === 'undefined') return ''
    const params = new URLSearchParams(window.location.search)
    return params.get('path') || ''
  })

  // Collapsible tree state
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(new Set())

  // --- search / sort / filter state ---
  const [query, setQuery] = useState('')
  const [sortField, setSortField] = useState<SortField>('date')
  const [sortDir, setSortDir] = useState<SortDir>('desc')
  const [draftFilter, setDraftFilter] = useState<DraftFilter>('all')
  const [languageFilter, setLanguageFilter] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState<(typeof PAGE_SIZES)[number]>(20)

  // --- batch selection state (keyed by relPath) ---
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [batchBusy, setBatchBusy] = useState(false)
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)

  // --- row hover tracking for hover-reveal checkboxes ---
  const [hoveredRow, setHoveredRow] = useState<string | null>(null)

  // --- fetch ---
  useEffect(() => {
    fetch('/admin/api/content')
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        return res.json()
      })
      .then((d: ContentListResponse) => setData(d))
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  // --- data processing (inline, no memo) ---
  const allItems = (() => {
    if (!data) return []
    const items = Object.values(data.sections).flat()
    if (!activePath) return items
    const prefix = activePath.endsWith('/') ? activePath : activePath + '/'
    return items.filter((it) => it.relPath.startsWith(prefix))
  })()

  // Unique languages from all items (includes default/empty as '__default__')
  const languages = (() => {
    if (!data) return [] as string[]
    const set = new Set<string>()
    let hasDefault = false
    for (const item of Object.values(data.sections).flat()) {
      if (item.language) set.add(item.language)
      else hasDefault = true
    }
    const result = [...set].sort()
    if (hasDefault) result.push('__default__')
    return result
  })()

  // Filter items directly (no language merging)
  let filteredItems = allItems

  if (query.trim()) {
    const q = query.toLowerCase()
    filteredItems = filteredItems.filter(
      (it) =>
        it.title.toLowerCase().includes(q) ||
        it.relPath.toLowerCase().includes(q)
    )
  }
  if (draftFilter === 'draft')
    filteredItems = filteredItems.filter((it) => it.draft)
  else if (draftFilter === 'published')
    filteredItems = filteredItems.filter((it) => !it.draft)
  if (languageFilter === '__default__')
    filteredItems = filteredItems.filter((it) => !it.language)
  else if (languageFilter)
    filteredItems = filteredItems.filter((it) => it.language === languageFilter)

  filteredItems = [...filteredItems].sort((a, b) => {
    let cmp = 0
    if (sortField === 'title') {
      cmp = a.title.localeCompare(b.title)
    } else if (sortField === 'date') {
      cmp = (a.date || '').localeCompare(b.date || '')
    }
    return sortDir === 'asc' ? cmp : -cmp
  })

  const totalPages = Math.max(1, Math.ceil(filteredItems.length / pageSize))
  const safePage = Math.min(page, totalPages)
  const paginatedItems = filteredItems.slice((safePage - 1) * pageSize, safePage * pageSize)

  // Reset page when filters change
  useEffect(() => {
    setPage(1)
  }, [query, draftFilter, languageFilter, sortField, sortDir, activePath])

  // Reset selection when data changes
  useEffect(() => {
    setSelected(new Set())
  }, [data])

  // --- tree view logic ---
  useEffect(() => {
    if (!activePath || !data) return
    const parts = activePath.split('/')
    const ancestors: string[] = []
    for (let i = 1; i <= parts.length; i++) {
      ancestors.push(parts.slice(0, i).join('/'))
    }
    setExpandedPaths((prev) => {
      const next = new Set(prev)
      ancestors.forEach((a) => next.add(a))
      return next
    })
  }, [activePath, data])

  const toggleFolder = useCallback((path: string) => {
    setExpandedPaths((prev) => {
      const next = new Set(prev)
      if (next.has(path)) {
        next.delete(path)
      } else {
        next.add(path)
      }
      return next
    })
  }, [])

  const selectFolder = useCallback(
    (path: string) => {
      setActivePath(path)
      const newUrl = path
        ? `/admin/content?path=${encodeURIComponent(path)}`
        : '/admin/content'
      window.history.replaceState(null, '', newUrl)
    },
    []
  )

  // --- sort toggle ---
  const toggleSort = useCallback(
    (field: SortField) => {
      if (sortField === field) {
        setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
      } else {
        setSortField(field)
        setSortDir(field === 'date' ? 'desc' : 'asc')
      }
    },
    [sortField]
  )

  const SortIcon = ({ field }: { field: SortField }) => {
    if (sortField !== field) return <ArrowUpDown className="h-3 w-3 opacity-30" />
    return sortDir === 'asc' ? (
      <ArrowUp className="h-3 w-3" />
    ) : (
      <ArrowDown className="h-3 w-3" />
    )
  }

  // --- selection helpers (by relPath) ---
  const allSelected =
    paginatedItems.length > 0 &&
    paginatedItems.every((it) => selected.has(it.relPath))

  const toggleAll = () => {
    if (allSelected) {
      setSelected(new Set())
    } else {
      setSelected(new Set(paginatedItems.map((it) => it.relPath)))
    }
  }

  const toggleOne = (relPath: string) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(relPath)) next.delete(relPath)
      else next.add(relPath)
      return next
    })
  }

  // --- batch operations ---
  const doBatchDelete = useCallback(async () => {
    setBatchBusy(true)
    const paths = [...selected]
    try {
      await Promise.all(
        paths.map((path) =>
          fetch(`/admin/api/content/${encodeURIComponent(path)}`, {
            method: 'DELETE',
          }).then((r) => {
            if (!r.ok) throw new Error(`delete ${path} failed`)
          })
        )
      )
      const res = await fetch('/admin/api/content')
      const d: ContentListResponse = await res.json()
      setData(d)
      setShowDeleteConfirm(false)
    } catch (e: any) {
      setError(e.message)
    } finally {
      setBatchBusy(false)
    }
  }, [selected])

  const doBatchToggleDraft = useCallback(async () => {
    setBatchBusy(true)
    const paths = [...selected]
    const allDraft = paths.every(
      (p) => allItems.find((it) => it.relPath === p)?.draft
    )
    const newDraft = !allDraft
    try {
      await Promise.all(
        paths.map(async (path) => {
          const getRes = await fetch(
            `/admin/api/content/${encodeURIComponent(path)}`
          )
          if (!getRes.ok) throw new Error(`get ${path} failed`)
          const detail = await getRes.json()
          const frontmatter = { ...detail.frontmatter, draft: newDraft }
          const putRes = await fetch(
            `/admin/api/content/${encodeURIComponent(path)}`,
            {
              method: 'PUT',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({
                frontmatter,
                rawContent: detail.rawContent,
              }),
            }
          )
          if (!putRes.ok) throw new Error(`update ${path} failed`)
        })
      )
      const res = await fetch('/admin/api/content')
      const d: ContentListResponse = await res.json()
      setData(d)
    } catch (e: any) {
      setError(e.message)
    } finally {
      setBatchBusy(false)
    }
  }, [selected, allItems])

  // Dismiss selection with Escape
  const containerRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setSelected(new Set())
        setShowDeleteConfirm(false)
      }
    }
    const el = containerRef.current
    el?.addEventListener('keydown', handler)
    return () => el?.removeEventListener('keydown', handler)
  }, [])

  // ---------- Status counts for filter chips ----------
  const totalCount = allItems.length
  const draftCount = allItems.filter((it) => it.draft).length
  const publishedCount = allItems.filter((it) => !it.draft).length

  const draftOptions = [
    { value: 'all' as DraftFilter, label: '全部' },
    { value: 'draft' as DraftFilter, label: '草稿' },
    { value: 'published' as DraftFilter, label: '已发布' },
  ]
  const chipCounts: Record<string, number> = {
    all: totalCount,
    draft: draftCount,
    published: publishedCount,
  }

  // --- render ---
  if (loading)
    return (
      <div className="text-muted-foreground py-24 text-center text-sm">
        加载中...
      </div>
    )
  if (error)
    return (
      <div className="text-destructive-foreground py-24 text-center text-sm">
        加载失败: {error}
      </div>
    )
  if (!data) return null

  return (
    <div ref={containerRef}>
      {/* ============ Page header ============ */}
      <div className="flex items-center justify-between mb-5">
        <div>
          <Breadcrumb path={activePath} />
          <h2 className="text-xl font-semibold text-foreground tracking-tight">
            {activePath
              ? activePath.split('/').pop()
              : '内容管理'}
          </h2>
          <p className="text-sm text-muted-foreground mt-0.5">
            {activePath
              ? `${activePath} · ${filteredItems.length} 篇`
              : `全部 · ${filteredItems.length} 篇`}
          </p>
        </div>
        <button
          onClick={() => navigate('/admin/content/new')}
          className="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md bg-foreground text-background hover:bg-neutral-800 transition-colors"
        >
          <Plus className="h-4 w-4" />
          <span>新建</span>
        </button>
      </div>

      <div className="flex gap-6">
        {/* ============ Left sidebar — tree only ============ */}
        <aside className="w-52 shrink-0">
          {/* Root "全部" entry */}
          <div className="mb-2">
            <button
              onClick={() => selectFolder('')}
              className={
                `w-full text-left px-2.5 py-1.5 text-sm rounded-md transition-colors flex items-center gap-2 ` +
                (!activePath && !languageFilter && draftFilter === 'all'
                  ? 'bg-neutral-100 text-foreground font-medium'
                  : 'text-muted-foreground hover:text-foreground hover:bg-neutral-50')
              }
            >
              <Folder className="h-3.5 w-3.5 text-muted-foreground/50" />
              <span className="truncate flex-1">全部内容</span>
              <span className="text-muted-foreground/40 text-xs tabular-nums">
                {totalCount}
              </span>
            </button>
          </div>

          {/* Folder tree */}
          {data.tree?.length > 0 && (
            <div className="border-t border-border pt-2">
              <TreeView
                nodes={data.tree}
                activePath={activePath}
                expandedPaths={expandedPaths}
                onToggle={toggleFolder}
                onSelect={selectFolder}
              />
            </div>
          )}
        </aside>

        {/* ============ Main content area ============ */}
        <div className="flex-1 min-w-0">
          {/* ---- Toolbar: search + filter chips + sort + language ---- */}
          <div className="flex items-center gap-3 mb-4 flex-wrap">
            {/* Search */}
            <div className="relative flex-1 max-w-xs">
              <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground/50 pointer-events-none" />
              <input
                type="text"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="搜索..."
                className="w-full pl-8 pr-7 py-1.5 text-sm rounded-lg border border-border bg-transparent text-foreground placeholder:text-muted-foreground/50 focus:outline-none focus:border-foreground/40 transition-colors"
              />
              {query && (
                <button
                  onClick={() => setQuery('')}
                  className="absolute right-1.5 top-1/2 -translate-y-1/2 text-muted-foreground/50 hover:text-foreground transition-colors"
                >
                  <X className="h-3.5 w-3.5" />
                </button>
              )}
            </div>

            {/* Filter chips */}
            <div className="flex items-center gap-1">
              {draftOptions.map((opt) => (
                <FilterChip
                  key={opt.value}
                  active={draftFilter === opt.value && !languageFilter}
                  label={opt.label}
                  count={chipCounts[opt.value]}
                  onClick={() => setDraftFilter(opt.value)}
                />
              ))}
            </div>

            {/* Language filter */}
            {languages.length > 0 && (
              <select
                value={languageFilter}
                onChange={(e) => setLanguageFilter(e.target.value)}
                className="text-xs bg-transparent border border-border rounded-md px-2 py-1.5 text-muted-foreground focus:outline-none focus:border-foreground/40 transition-colors"
              >
                <option value="">所有语言</option>
                {languages.map((lang) => (
                  <option key={lang} value={lang}>
                    {lang === '__default__' ? '默认' : lang}
                  </option>
                ))}
              </select>
            )}
          </div>

          {filteredItems.length === 0 ? (
            <div className="text-muted-foreground py-24 text-center text-sm">
              暂无内容
            </div>
          ) : (
            <>
              {/* ---- List (Linear-style rows) ---- */}
              <div>
                {/* Column headers (subtle, not a box) */}
                <div className="flex items-center gap-3 px-3 pb-2 text-xs text-muted-foreground/50 border-b border-border">
                  <div className="w-4 shrink-0">
                    {/* Invisible spacer for checkbox alignment */}
                  </div>
                  <button
                    onClick={() => toggleSort('title')}
                    className="flex items-center gap-1 hover:text-foreground transition-colors flex-1 min-w-0"
                  >
                    <span>内容</span>
                    <SortIcon field="title" />
                  </button>
                  <button
                    onClick={() => toggleSort('date')}
                    className="flex items-center gap-1 justify-end hover:text-foreground transition-colors w-24 shrink-0"
                  >
                    <span>日期</span>
                    <SortIcon field="date" />
                  </button>
                </div>

                {/* Rows */}
                <div className="divide-y divide-border">
                  {paginatedItems.map((item) => {
                    const rowKey = item.relPath + '\x00' + item.language
                    const isSelected = selected.has(item.relPath)
                    const isHovered = hoveredRow === rowKey
                    const showCheckbox = isSelected || isHovered
                    return (
                      <div
                        key={rowKey}
                        onMouseEnter={() => setHoveredRow(rowKey)}
                        onMouseLeave={() => setHoveredRow(null)}
                        className={
                          `flex items-center gap-3 px-3 py-2.5 text-sm transition-colors rounded-sm ` +
                          (isSelected
                            ? 'bg-neutral-50'
                            : 'hover:bg-neutral-50/60')
                        }
                      >
                        {/* Checkbox — visible on hover or when selected */}
                        <div className="w-4 shrink-0 flex items-center justify-center">
                          <button
                            onClick={() => toggleOne(item.relPath)}
                            className={
                              `w-4 h-4 rounded border transition-all flex items-center justify-center ` +
                              (isSelected
                                ? 'bg-foreground border-foreground'
                                : showCheckbox
                                  ? 'border-border hover:border-foreground/50'
                                  : 'border-transparent')
                            }
                          >
                            {isSelected && (
                              <Check className="h-3 w-3 text-background stroke-[2.5]" />
                            )}
                          </button>
                        </div>

                        {/* Content info */}
                        <div className="flex-1 min-w-0 flex items-center gap-2 flex-wrap">
                          {/* Language badge */}
                          <LanguageBadge language={item.language} />

                          {/* Title (clickable row) */}
                          <button
                            onClick={() =>
                              navigate(
                                `/admin/content/edit?path=${encodeURIComponent(item.filePath)}`
                              )
                            }
                            className="text-foreground hover:text-muted-foreground/70 transition-colors truncate max-w-[280px] text-left"
                          >
                            {item.title || '(无标题)'}
                          </button>

                          {/* Status dot */}
                          <span
                            className={
                              `inline-block w-1.5 h-1.5 rounded-full shrink-0 ` +
                              (item.draft ? 'bg-muted-foreground/20' : 'bg-foreground/50')
                            }
                            title={item.draft ? '草稿' : '已发布'}
                          />

                          {/* File path — visible on hover or when selected */}
                          {(isSelected || isHovered) && (
                            <span className="text-[11px] text-muted-foreground/30 truncate font-mono hidden sm:inline">
                              {item.filePath}
                            </span>
                          )}

                          {/* Kind badge */}
                          {item.kind === 'section' && (
                            <span className="text-[10px] px-1.5 py-0.5 rounded border border-border text-muted-foreground/50 font-mono uppercase tracking-wider">
                              section
                            </span>
                          )}
                        </div>

                        {/* Date */}
                        <div className="w-24 shrink-0 text-xs text-muted-foreground tabular-nums text-right">
                          {item.date ? item.date.substring(0, 10) : '—'}
                        </div>
                      </div>
                    )
                  })}
                </div>
              </div>

              {/* ---- Pagination ---- */}
              <div className="flex items-center justify-between mt-5">
                <div className="text-xs text-muted-foreground/50 tabular-nums">
                  {filteredItems.length > 0
                    ? `${(safePage - 1) * pageSize + 1}–${Math.min(safePage * pageSize, filteredItems.length)} / ${filteredItems.length}`
                    : '0 条'}
                </div>

                <div className="flex items-center gap-4">
                  {/* Page size */}
                  <div className="flex items-center gap-1.5 text-xs text-muted-foreground/50">
                    <span>每页</span>
                    <select
                      value={pageSize}
                      onChange={(e) => {
                        setPageSize(Number(e.target.value) as typeof pageSize)
                        setPage(1)
                      }}
                      className="text-xs bg-transparent border border-border rounded px-1.5 py-0.5 text-foreground focus:outline-none focus:border-foreground/40"
                    >
                      {PAGE_SIZES.map((s) => (
                        <option key={s} value={s}>
                          {s}
                        </option>
                      ))}
                    </select>
                  </div>

                  {/* Pagination buttons */}
                  <div className="flex items-center gap-1">
                    <button
                      onClick={() => setPage((p) => Math.max(1, p - 1))}
                      disabled={safePage <= 1}
                      className="w-7 h-7 flex items-center justify-center rounded text-muted-foreground/50 hover:text-foreground hover:bg-neutral-50 disabled:opacity-20 disabled:cursor-not-allowed transition-colors"
                    >
                      <ChevronLeft className="h-3.5 w-3.5" />
                    </button>

                    {Array.from(
                      { length: Math.min(5, totalPages) },
                      (_, i) => {
                        const start = Math.max(1, Math.min(safePage - 2, totalPages - 4))
                        const p = start + i
                        if (p > totalPages) return null
                        return (
                          <button
                            key={p}
                            onClick={() => setPage(p)}
                            className={
                              `w-7 h-7 text-xs rounded transition-colors ` +
                              (p === safePage
                                ? 'bg-neutral-100 text-foreground font-medium'
                                : 'text-muted-foreground/50 hover:text-foreground hover:bg-neutral-50')
                            }
                          >
                            {p}
                          </button>
                        )
                      }
                    )}

                    <button
                      onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                      disabled={safePage >= totalPages}
                      className="w-7 h-7 flex items-center justify-center rounded text-muted-foreground/50 hover:text-foreground hover:bg-neutral-50 disabled:opacity-20 disabled:cursor-not-allowed transition-colors"
                    >
                      <ChevronRight className="h-3.5 w-3.5" />
                    </button>
                  </div>
                </div>
              </div>
            </>
          )}

          {/* ---- Batch action bar ---- */}
          {selected.size > 0 && !loading && (
            <div className="sticky bottom-0 mt-4 px-4 py-2.5 rounded-lg bg-background border border-border flex items-center justify-between shadow-sm">
              <span className="text-sm text-foreground font-medium tabular-nums">
                已选 {selected.size} 项
              </span>
              <div className="flex items-center gap-2">
                <button
                  onClick={() => setShowDeleteConfirm(true)}
                  disabled={batchBusy}
                  className="flex items-center gap-1.5 px-2.5 py-1.5 text-sm rounded-md text-muted-foreground hover:text-foreground hover:bg-neutral-50 transition-colors disabled:opacity-40"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                  <span>删除</span>
                </button>
                <button
                  onClick={doBatchToggleDraft}
                  disabled={batchBusy}
                  className="flex items-center gap-1.5 px-2.5 py-1.5 text-sm rounded-md text-muted-foreground hover:text-foreground hover:bg-neutral-50 transition-colors disabled:opacity-40"
                >
                  <span>
                    {[...selected].every(
                      (relPath) => allItems.find((it) => it.relPath === relPath)?.draft
                    )
                      ? '发布'
                      : '设为草稿'}
                  </span>
                </button>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* ============ Delete confirmation ============ */}
      {showDeleteConfirm && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/10"
          onClick={() => setShowDeleteConfirm(false)}
        >
          <div
            className="bg-background border border-border rounded-xl shadow-lg max-w-sm w-full mx-4"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="px-5 pt-5 pb-2">
              <h3 className="text-base font-semibold text-foreground tracking-tight">
                确认删除
              </h3>
              <p className="text-sm text-muted-foreground mt-1">
                将删除 {selected.size} 篇内容，此操作不可撤销。
              </p>
            </div>

            <div className="max-h-28 overflow-y-auto px-5 py-2 text-xs text-muted-foreground/60 space-y-0.5">
              {[...selected].slice(0, 10).map((path) => (
                <div key={path} className="truncate font-mono">
                  {path}
                </div>
              ))}
              {selected.size > 10 && (
                <div className="text-muted-foreground/40">
                  ...及其他 {selected.size - 10} 项
                </div>
              )}
            </div>

            <div className="flex items-center justify-end gap-2 px-5 pb-4 pt-2">
              <button
                onClick={() => setShowDeleteConfirm(false)}
                className="px-3 py-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors"
              >
                取消
              </button>
              <button
                onClick={doBatchDelete}
                disabled={batchBusy}
                className="px-3 py-1.5 text-sm rounded-md bg-foreground text-background hover:bg-neutral-800 transition-colors disabled:opacity-40"
              >
                {batchBusy ? '删除中...' : '确认删除'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
