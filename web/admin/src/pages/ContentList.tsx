import { useEffect, useState, useCallback } from 'react'
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
  ToggleLeft,
  ToggleRight,
  X,
} from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogClose,
} from '@/components/ui/dialog'

interface ContentItem {
  title: string
  relPath: string
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

type SortField = 'title' | 'date' | 'status'
type SortDir = 'asc' | 'desc'
type DraftFilter = 'all' | 'draft' | 'published'

const PAGE_SIZES = [20, 50, 100] as const

// Recursive tree node renderer — standalone component outside ContentList.
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
                  ? 'bg-muted text-foreground font-medium'
                  : 'text-muted-foreground hover:text-foreground hover:bg-muted')
              }
            >
              {node.type === 'folder' && (
                <ChevronRight
                  className={
                    `h-3 w-3 shrink-0 transition-transform text-muted-foreground/50 ` +
                    (isExpanded ? 'rotate-90' : '')
                  }
                />
              )}
              {node.type === 'file' && (
                <FileText className="h-3 w-3 shrink-0 text-muted-foreground/50" />
              )}
              <span className="truncate flex-1">{node.name}</span>
              {node.type === 'folder' && (
                <span className="text-muted-foreground text-xs tabular-nums">
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

export default function ContentList() {
  const [data, setData] = useState<ContentListResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const navigate = useNavigate()

  // Pure local state for the active path — no URL-sync interference.
  // URL is updated via history.replaceState for shareability without
  // triggering React Router's async re-render cycle.
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
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState<(typeof PAGE_SIZES)[number]>(20)

  // --- batch selection state ---
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [batchBusy, setBatchBusy] = useState(false)
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)

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

  // --- data processing (inline, no memo — avoids React 19 stale-closure issues) ---
  const allItems = (() => {
    if (!data) return []
    const items = Object.values(data.sections).flat()
    if (!activePath) return items
    const prefix = activePath.endsWith('/') ? activePath : activePath + '/'
    return items.filter((it) => it.relPath.startsWith(prefix))
  })()

  let processed = allItems
  if (query.trim()) {
    const q = query.toLowerCase()
    processed = processed.filter(
      (it) =>
        it.title.toLowerCase().includes(q) ||
        it.relPath.toLowerCase().includes(q)
    )
  }
  if (draftFilter === 'draft') processed = processed.filter((it) => it.draft)
  else if (draftFilter === 'published')
    processed = processed.filter((it) => !it.draft)

  processed = [...processed].sort((a, b) => {
    let cmp = 0
    if (sortField === 'title') {
      cmp = a.title.localeCompare(b.title)
    } else if (sortField === 'date') {
      cmp = (a.date || '').localeCompare(b.date || '')
    } else if (sortField === 'status') {
      cmp = Number(a.draft) - Number(b.draft)
    }
    return sortDir === 'asc' ? cmp : -cmp
  })

  const totalPages = Math.max(1, Math.ceil(processed.length / pageSize))
  const safePage = Math.min(page, totalPages)
  const paginatedItems = processed.slice((safePage - 1) * pageSize, safePage * pageSize)

  // Reset page when filters change (including folder switch)
  useEffect(() => {
    setPage(1)
  }, [query, draftFilter, sortField, sortDir, activePath])

  // Reset selection when data changes
  useEffect(() => {
    setSelected(new Set())
  }, [data])

  // --- tree view logic ---
  // Auto-expand ancestors of the active path.
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
      // Update URL silently for shareability — no React Router re-render.
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
    if (sortField !== field) return <ArrowUpDown className="h-3 w-3 opacity-40" />
    return sortDir === 'asc' ? (
      <ArrowUp className="h-3 w-3" />
    ) : (
      <ArrowDown className="h-3 w-3" />
    )
  }

  // --- selection helpers ---
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
            if (!r.ok) throw new Error(`delete ${path} failed: HTTP ${r.status}`)
          })
        )
      )
      // Refetch
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
    // Determine target draft state: if all selected are draft → publish; else → draft
    const allDraft = paths.every(
      (p) => allItems.find((it) => it.relPath === p)?.draft
    )
    const newDraft = !allDraft
    try {
      await Promise.all(
        paths.map(async (path) => {
          // Fetch current frontmatter, then update draft
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
      // Refetch
      const res = await fetch('/admin/api/content')
      const d: ContentListResponse = await res.json()
      setData(d)
    } catch (e: any) {
      setError(e.message)
    } finally {
      setBatchBusy(false)
    }
  }, [selected, allItems])

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
    <div>
      {/* ============ Page header ============ */}
      <div className="flex items-center justify-between mb-5">
        <div>
          <h2 className="text-xl font-semibold text-foreground tracking-tight">
            内容管理
          </h2>
          <p className="text-sm text-muted-foreground mt-0.5">
            {activePath
              ? `${activePath} · ${allItems.length} 篇`
              : `全部 · ${data.total} 篇`}
          </p>
        </div>
        <button
          onClick={() => navigate('/admin/content/new')}
          className="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md bg-primary text-primary-foreground hover:opacity-90 transition-opacity"
        >
          <Plus className="h-4 w-4" />
          <span>新建</span>
        </button>
      </div>

      <div className="flex gap-5">
        {/* ============ Folder tree sidebar ============ */}
        <aside className="w-48 shrink-0">
          <div className="space-y-0.5">
            <button
              onClick={() => selectFolder('')}
              className={
                `w-full text-left px-2.5 py-1.5 text-sm rounded-md transition-colors flex items-center gap-2 ` +
                (!activePath
                  ? 'bg-muted text-foreground font-medium'
                  : 'text-muted-foreground hover:text-foreground hover:bg-muted')
              }
            >
              <FileText className="h-3.5 w-3.5 shrink-0 text-muted-foreground/70" />
              <span className="flex-1">全部内容</span>
              <span className="text-muted-foreground text-xs tabular-nums font-normal">
                {data.total}
              </span>
            </button>
            <div className="border-t border-border my-1.5" />
            <div className="text-[11px] font-medium text-muted-foreground/60 uppercase tracking-wider px-2.5 pb-1">
              分类
            </div>
            <TreeView
              nodes={data.tree}
              activePath={activePath}
              expandedPaths={expandedPaths}
              onToggle={toggleFolder}
              onSelect={selectFolder}
            />
          </div>
        </aside>

        {/* ============ Main content area ============ */}
        <div className="flex-1 min-w-0">
          {/* ---- Toolbar: search + draft filter ---- */}
          <div className="flex items-center gap-2 mb-4">
            {/* Search */}
            <div className="relative flex-1 max-w-xs">
              <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground pointer-events-none" />
              <input
                type="text"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="搜索..."
                className="w-full pl-8 pr-7 py-1.5 text-sm rounded-md border border-border bg-transparent text-foreground placeholder:text-muted-foreground focus:outline-none focus:border-foreground transition-colors"
              />
              {query && (
                <button
                  onClick={() => setQuery('')}
                  className="absolute right-1.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                >
                  <X className="h-3.5 w-3.5" />
                </button>
              )}
            </div>

            {/* Draft filter segmented control */}
            <div className="flex items-center border border-border rounded-md overflow-hidden">
              {(
                [
                  { value: 'all', label: '全部' },
                  { value: 'draft', label: '草稿' },
                  { value: 'published', label: '已发布' },
                ] as { value: DraftFilter; label: string }[]
              ).map((opt, i) => (
                <button
                  key={opt.value}
                  onClick={() => setDraftFilter(opt.value)}
                  className={
                    `px-3 py-1.5 text-sm transition-colors ` +
                    (i > 0 ? 'border-l border-border ' : '') +
                    (draftFilter === opt.value
                      ? 'bg-muted text-foreground font-medium'
                      : 'text-muted-foreground hover:text-foreground')
                  }
                >
                  {opt.label}
                </button>
              ))}
            </div>
          </div>

          {processed.length === 0 ? (
            <div className="text-muted-foreground py-24 text-center text-sm">
              暂无内容
            </div>
          ) : (
            <>
              {/* ---- List table ---- */}
              <div className="border border-border rounded-md overflow-hidden">
                {/* Header row */}
                <div className="grid grid-cols-[auto_1fr_auto_auto_auto_auto] gap-3 px-4 py-2.5 text-xs font-medium text-muted-foreground bg-muted/40 border-b border-border items-center">
                  {/* Checkbox (select all) */}
                  <div>
                    <input
                      type="checkbox"
                      checked={allSelected}
                      onChange={toggleAll}
                      className="h-3.5 w-3.5 accent-foreground cursor-pointer"
                    />
                  </div>

                  {/* Title (sortable) */}
                  <button
                    onClick={() => toggleSort('title')}
                    className="flex items-center gap-1 text-left hover:text-foreground transition-colors"
                  >
                    <span>标题</span>
                    <SortIcon field="title" />
                  </button>

                  {/* Kind */}
                  <div>类型</div>

                  {/* Status (sortable) */}
                  <button
                    onClick={() => toggleSort('status')}
                    className="flex items-center gap-1 hover:text-foreground transition-colors"
                  >
                    <span>状态</span>
                    <SortIcon field="status" />
                  </button>

                  {/* Date (sortable) */}
                  <button
                    onClick={() => toggleSort('date')}
                    className="flex items-center gap-1 hover:text-foreground transition-colors"
                  >
                    <span>日期</span>
                    <SortIcon field="date" />
                  </button>

                  {/* Edit action */}
                  <div></div>
                </div>

                {/* Data rows */}
                <div className="divide-y divide-border">
                  {paginatedItems.map((item) => {
                    const isSelected = selected.has(item.relPath)
                    return (
                      <div
                        key={`${item.relPath}:${item.language || 'default'}`}
                        className={
                          `grid grid-cols-[auto_1fr_auto_auto_auto_auto] gap-3 px-4 py-2.5 items-center text-sm transition-colors group ` +
                          (isSelected ? 'bg-muted/50' : 'hover:bg-muted/30')
                        }
                      >
                        {/* Checkbox */}
                        <div>
                          <input
                            type="checkbox"
                            checked={isSelected}
                            onChange={() => toggleOne(item.relPath)}
                            className="h-3.5 w-3.5 accent-foreground cursor-pointer"
                          />
                        </div>

                        {/* Title + path */}
                        <div className="min-w-0">
                          <div className="flex items-center gap-2">
                            <FileText className="h-3.5 w-3.5 shrink-0 text-muted-foreground/60" />
                            <button
                              onClick={() =>
                                navigate(
                                  `/admin/content/edit?path=${encodeURIComponent(item.relPath)}`
                                )
                              }
                              className="text-foreground hover:text-muted-foreground transition-colors truncate text-left"
                            >
                              {item.title || '(无标题)'}
                            </button>
                          </div>
                          <div className="text-xs text-muted-foreground/60 mt-0.5 truncate ml-5.5">
                            {item.relPath}
                          </div>
                        </div>

                        {/* Kind badge */}
                        <div>
                          {item.kind === 'section' ? (
                            <Badge variant="outline" className="text-[11px] px-1.5 py-0">section</Badge>
                          ) : (
                            <span className="text-xs text-muted-foreground/60">
                              page
                            </span>
                          )}
                        </div>

                        {/* Status */}
                        <div className="flex items-center">
                          {item.draft ? (
                            <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-[11px] border border-border text-muted-foreground">
                              <span className="w-1.5 h-1.5 rounded-full bg-muted-foreground/50" />
                              草稿
                            </span>
                          ) : (
                            <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-[11px] border border-border/60 text-foreground/80">
                              <span className="w-1.5 h-1.5 rounded-full bg-foreground/70" />
                              已发布
                            </span>
                          )}
                        </div>

                        {/* Date */}
                        <div className="text-xs text-muted-foreground tabular-nums">
                          {item.date ? item.date.substring(0, 10) : '—'}
                        </div>

                        {/* Edit button — appears on row hover */}
                        <div className="opacity-0 group-hover:opacity-100 transition-opacity">
                          <button
                            onClick={() =>
                              navigate(
                                `/admin/content/edit?path=${encodeURIComponent(item.relPath)}`
                              )
                            }
                            className="text-xs text-muted-foreground hover:text-foreground transition-colors"
                          >
                            编辑
                          </button>
                        </div>
                      </div>
                    )
                  })}
                </div>
              </div>

              {/* ---- Pagination ---- */}
              <div className="flex items-center justify-between mt-4">
                <div className="text-xs text-muted-foreground tabular-nums">
                  {processed.length > 0
                    ? `${(safePage - 1) * pageSize + 1}–${Math.min(safePage * pageSize, processed.length)} / ${processed.length}`
                    : '0 条'}
                </div>

                <div className="flex items-center gap-3">
                  {/* Page size selector */}
                  <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
                    <span>每页</span>
                    <select
                      value={pageSize}
                      onChange={(e) => {
                        setPageSize(Number(e.target.value) as typeof pageSize)
                        setPage(1)
                      }}
                      className="text-xs bg-transparent border border-border rounded px-1.5 py-0.5 text-foreground focus:outline-none focus:border-foreground"
                    >
                      {PAGE_SIZES.map((s) => (
                        <option key={s} value={s}>
                          {s}
                        </option>
                      ))}
                    </select>
                  </div>

                  {/* Page navigation */}
                  <div className="flex items-center gap-1">
                    <button
                      onClick={() => setPage((p) => Math.max(1, p - 1))}
                      disabled={safePage <= 1}
                      className="p-1 rounded text-muted-foreground hover:text-foreground disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
                    >
                      <ChevronLeft className="h-3.5 w-3.5" />
                    </button>

                    <div className="flex items-center gap-0.5">
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
                                `min-w-[24px] h-6 text-xs rounded transition-colors ` +
                                (p === safePage
                                  ? 'bg-muted text-foreground font-medium'
                                  : 'text-muted-foreground hover:text-foreground')
                              }
                            >
                              {p}
                            </button>
                          )
                        }
                      )}
                    </div>

                    <button
                      onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                      disabled={safePage >= totalPages}
                      className="p-1 rounded text-muted-foreground hover:text-foreground disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
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
            <div className="sticky bottom-0 mt-4 px-4 py-2.5 rounded-md bg-background border border-border flex items-center justify-between shadow-xs">
              <span className="text-sm text-foreground font-medium tabular-nums">
                已选 {selected.size} 项
              </span>
              <div className="flex items-center gap-2">
                <button
                  onClick={() => setShowDeleteConfirm(true)}
                  disabled={batchBusy}
                  className="flex items-center gap-1.5 px-2.5 py-1 text-sm rounded-md border border-border text-muted-foreground hover:text-foreground hover:bg-muted transition-colors disabled:opacity-40"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                  <span>删除</span>
                </button>
                <button
                  onClick={doBatchToggleDraft}
                  disabled={batchBusy}
                  className="flex items-center gap-1.5 px-2.5 py-1 text-sm rounded-md border border-border text-muted-foreground hover:text-foreground hover:bg-muted transition-colors disabled:opacity-40"
                >
                  {allItems.every(
                    (it) => !selected.has(it.relPath) || it.draft
                  ) ? (
                    <ToggleRight className="h-3.5 w-3.5" />
                  ) : (
                    <ToggleLeft className="h-3.5 w-3.5" />
                  )}
                  <span>切换草稿</span>
                </button>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* ============ Delete confirmation dialog ============ */}
      <Dialog open={showDeleteConfirm} onOpenChange={setShowDeleteConfirm}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>确认删除</DialogTitle>
            <DialogDescription>
              将删除 {selected.size} 篇内容，此操作不可撤销。
            </DialogDescription>
          </DialogHeader>

          <div className="max-h-32 overflow-y-auto text-xs text-muted-foreground space-y-1">
            {[...selected].slice(0, 10).map((path) => (
              <div key={path} className="truncate">
                {path}
              </div>
            ))}
            {selected.size > 10 && (
              <div className="text-muted-foreground">
                ...及其他 {selected.size - 10} 项
              </div>
            )}
          </div>

          <DialogFooter>
            <DialogClose className="px-2.5 py-1 text-sm text-muted-foreground hover:text-foreground transition-colors">
              取消
            </DialogClose>
            <button
              onClick={doBatchDelete}
              disabled={batchBusy}
              className="px-3 py-1 text-sm rounded-md bg-primary text-primary-foreground hover:opacity-90 transition-opacity disabled:opacity-40"
            >
              {batchBusy ? '删除中...' : '确认删除'}
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
