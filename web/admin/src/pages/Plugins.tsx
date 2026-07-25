import { useEffect, useState } from 'react'
import { apiFetch } from '../lib/api'
import { Plus, X, RefreshCw, AlertCircle } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Dialog, DialogTrigger, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter, DialogClose } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'

interface PluginInfo {
  name: string
  version: string
  source: string
  capability: string
  status: string
  loadedAt: string
  error: string
  author: string
  repoURL: string
  license: string
  tags: string[]
}

interface PluginListResponse {
  plugins: PluginInfo[]
  total: number
}

interface PluginManageResponse {
  status: string
  plugin?: PluginInfo
  error?: string
}

export default function Plugins() {
  const [plugins, setPlugins] = useState<PluginInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [actionLoading, setActionLoading] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)
  const [loadPath, setLoadPath] = useState('')
  const [loadDialogOpen, setLoadDialogOpen] = useState(false)
  const [reloadPath, setReloadPath] = useState('')
  const [reloadTarget, setReloadTarget] = useState<string | null>(null)
  const [reloadDialogOpen, setReloadDialogOpen] = useState(false)
  const [expandedErrors, setExpandedErrors] = useState<Set<string>>(new Set())

  const fetchPlugins = () => {
    setLoading(true)
    setError(null)
    apiFetch('/admin/api/plugins')
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`)
        return r.json()
      })
      .then((data: PluginListResponse) => {
        setPlugins(data.plugins || [])
      })
      .catch((e) => {
        setError(e.message || '加载失败')
      })
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    fetchPlugins()
  }, [])

  const handleLoad = async () => {
    if (!loadPath.trim()) return
    setActionLoading('load')
    setActionError(null)
    try {
      const res = await apiFetch('/admin/api/plugins/load', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: loadPath.trim() }),
      })
      const data: PluginManageResponse = await res.json()
      if (!res.ok) {
        setActionError(data.error || '加载失败')
        return
      }
      setLoadPath('')
      setLoadDialogOpen(false)
      fetchPlugins()
    } catch (e: any) {
      setActionError(e.message || '网络错误')
    } finally {
      setActionLoading(null)
    }
  }

  const handleUnload = async (name: string) => {
    setActionLoading(`unload-${name}`)
    setActionError(null)
    try {
      const res = await apiFetch('/admin/api/plugins/unload', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name }),
      })
      if (!res.ok) {
        const data = await res.json()
        setActionError(data.error || '卸载失败')
        return
      }
      fetchPlugins()
    } catch (e: any) {
      setActionError(e.message || '网络错误')
    } finally {
      setActionLoading(null)
    }
  }

  const openReloadDialog = (name: string) => {
    setReloadTarget(name)
    setReloadPath('')
    setReloadDialogOpen(true)
  }

  const handleReload = async () => {
    if (!reloadTarget || !reloadPath.trim()) return
    setActionLoading(`reload-${reloadTarget}`)
    setActionError(null)
    try {
      const res = await apiFetch('/admin/api/plugins/reload', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: reloadTarget, path: reloadPath.trim() }),
      })
      const data = await res.json()
      if (!res.ok) {
        setActionError(data.error || '重载失败')
        return
      }
      setReloadDialogOpen(false)
      setReloadTarget(null)
      fetchPlugins()
    } catch (e: any) {
      setActionError(e.message || '网络错误')
    } finally {
      setActionLoading(null)
    }
  }

  const toggleError = (name: string) => {
    setExpandedErrors((prev) => {
      const next = new Set(prev)
      if (next.has(name)) {
        next.delete(name)
      } else {
        next.add(name)
      }
      return next
    })
  }

  // Loading state
  if (loading) {
    return (
      <div className="text-muted-foreground py-24 text-center text-sm">
        加载中...
      </div>
    )
  }

  // Error state
  if (error && plugins.length === 0) {
    return (
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-xl font-semibold text-foreground tracking-tight">插件管理</h2>
        </div>
        <div className="border border-destructive/50 rounded-md px-4 py-6 text-sm text-destructive text-center">
          {error}
        </div>
        <div className="flex justify-center">
          <Button variant="outline" onClick={fetchPlugins}>重试</Button>
        </div>
      </div>
    )
  }

  return (
    <div>
      {/* Page header */}
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-xl font-semibold text-foreground tracking-tight">插件管理</h2>
        <Dialog open={loadDialogOpen} onOpenChange={setLoadDialogOpen}>
          <DialogTrigger>
            <Button variant="default" size="sm">
              <Plus className="h-3.5 w-3.5 mr-1" />
              加载新插件
            </Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>加载插件</DialogTitle>
              <DialogDescription>
                输入 .so 插件文件的路径
              </DialogDescription>
            </DialogHeader>
            <div className="py-2">
              <Input
                placeholder="/path/to/plugin.so"
                value={loadPath}
                onChange={(e) => setLoadPath(e.target.value)}
              />
            </div>
            {actionLoading === 'load' && (
              <p className="text-xs text-muted-foreground">加载中...</p>
            )}
            {actionError && actionLoading === null && (
              <p className="text-xs text-destructive">{actionError}</p>
            )}
            <DialogFooter>
              <DialogClose>
                <Button variant="outline" size="sm">取消</Button>
              </DialogClose>
              <Button
                variant="default"
                size="sm"
                onClick={handleLoad}
                disabled={!loadPath.trim() || actionLoading === 'load'}
              >
                加载
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>

      {/* Action error banner */}
      {actionError && (
        <div className="border border-destructive/50 rounded-md px-3 py-2 mb-4 text-xs text-destructive flex items-center gap-2">
          <AlertCircle className="h-3.5 w-3.5 shrink-0" />
          <span>{actionError}</span>
        </div>
      )}

      {/* Empty state */}
      {plugins.length === 0 ? (
        <div className="border border-border rounded-md px-4 py-12 text-sm text-muted-foreground text-center">
          暂无插件。点击"加载新插件"添加 .so 插件。
        </div>
      ) : (
        <Card>
          <div className="divide-y divide-border">
            {/* Table header */}
            <div className="grid grid-cols-[1fr_60px_80px_100px_80px_1fr_120px] gap-2 px-3 py-2 text-[11px] text-muted-foreground font-medium">
              <span>名称</span>
              <span>版本</span>
              <span>来源</span>
              <span>能力</span>
              <span>状态</span>
              <span>加载时间</span>
              <span className="text-center">操作</span>
            </div>

            {/* Table rows */}
            {plugins.map((p) => (
              <div key={p.name}>
                <div className="grid grid-cols-[1fr_60px_80px_100px_80px_1fr_120px] gap-2 px-3 py-2.5 text-xs items-center">
                  <div className="flex items-center gap-2 min-w-0">
                    <span className="text-foreground font-medium truncate">{p.name}</span>
                    {p.author === 'huan team' && (
                      <Badge variant="secondary" className="text-[9px] leading-none px-1 py-0 shrink-0">官方</Badge>
                    )}
                    {p.tags && p.tags.length > 0 && (
                      <div className="hidden lg:flex items-center gap-1">
                        {p.tags.slice(0, 3).map((tag) => (
                          <Badge key={tag} variant="outline" className="text-[9px] leading-none px-1 py-0">{tag}</Badge>
                        ))}
                      </div>
                    )}
                  </div>
                  <span className="text-muted-foreground truncate">{p.version || '-'}</span>
                  <span>
                    <Badge variant={p.source === 'compiled' ? 'secondary' : p.source === 'loaded' ? 'default' : 'outline'}>
                      {p.source}
                    </Badge>
                  </span>
                  <span className="text-muted-foreground truncate">{p.capability || '-'}</span>
                  <span className="flex items-center gap-1.5">
                    <span className={`h-1.5 w-1.5 rounded-full ${p.status === 'active' ? 'bg-green-500' : p.status === 'error' ? 'bg-red-500' : 'bg-muted-foreground/50'}`} />
                    <span className={p.status === 'error' ? 'text-destructive' : 'text-muted-foreground'}>
                      {p.status}
                    </span>
                  </span>
                  <span className="text-muted-foreground truncate">{p.loadedAt || '-'}</span>
                  <span className="flex items-center justify-center gap-1">
                    {p.source === 'compiled' ? (
                      <span className="text-[10px] text-muted-foreground">系统内置</span>
                    ) : (
                      <>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => openReloadDialog(p.name)}
                          disabled={actionLoading === `reload-${p.name}`}
                          title="重载"
                        >
                          <RefreshCw className={`h-3 w-3 ${actionLoading === `reload-${p.name}` ? 'animate-spin' : ''}`} />
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleUnload(p.name)}
                          disabled={actionLoading === `unload-${p.name}`}
                          title="卸载"
                        >
                          <X className="h-3 w-3" />
                        </Button>
                      </>
                    )}
                  </span>
                </div>
                {/* Error detail expandable */}
                {p.status === 'error' && (
                  <div className="px-3 pb-2">
                    <button
                      onClick={() => toggleError(p.name)}
                      className="text-[10px] text-destructive hover:text-destructive/80 transition-colors"
                    >
                      {expandedErrors.has(p.name) ? '收起' : '查看错误详情'}
                    </button>
                    {expandedErrors.has(p.name) && p.error && (
                      <pre className="mt-1 text-[10px] text-destructive/80 bg-muted rounded p-2 overflow-x-auto whitespace-pre-wrap">
                        {p.error}
                      </pre>
                    )}
                  </div>
                )}
              </div>
            ))}
          </div>
          <div className="px-3 py-2 text-[10px] text-muted-foreground border-t border-border">
            共 {plugins.length} 个插件
          </div>
        </Card>
      )}

      {/* Reload dialog */}
      <Dialog open={reloadDialogOpen} onOpenChange={setReloadDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>重载插件</DialogTitle>
            <DialogDescription>
              输入新的 .so 文件路径以重载 {reloadTarget}
            </DialogDescription>
          </DialogHeader>
          <div className="py-2">
            <Input
              placeholder="/path/to/plugin.so"
              value={reloadPath}
              onChange={(e) => setReloadPath(e.target.value)}
            />
          </div>
          {actionError && actionLoading === null && (
            <p className="text-xs text-destructive">{actionError}</p>
          )}
          <DialogFooter>
            <DialogClose>
              <Button variant="outline" size="sm">取消</Button>
            </DialogClose>
            <Button
              variant="default"
              size="sm"
              onClick={handleReload}
              disabled={!reloadPath.trim() || actionLoading === `reload-${reloadTarget}`}
            >
              重载
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}