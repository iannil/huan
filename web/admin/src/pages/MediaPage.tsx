import { useEffect, useState, useRef } from 'react'
import { Upload, Trash2, FolderOpen } from 'lucide-react'

interface MediaItem {
  name: string
  path: string
  size: number
  ext: string
}

interface MediaResponse {
  files: MediaItem[]
  groups: Record<string, MediaItem[]>
  total: number
}

export default function MediaPage() {
  const [data, setData] = useState<MediaResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [activeGroup, setActiveGroup] = useState<string>('')
  const [uploading, setUploading] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  const fetchMedia = () => {
    fetch('/admin/api/media')
      .then((r) => (r.ok ? r.json() : null))
      .then((d) => {
        setData(d)
        if (d && !activeGroup && d.groups)
          setActiveGroup(Object.keys(d.groups)[0])
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    fetchMedia()
  }, [])

  const handleUpload = async () => {
    const file = fileRef.current?.files?.[0]
    if (!file) return
    setUploading(true)
    const form = new FormData()
    form.append('file', file)
    try {
      await fetch('/admin/api/media', { method: 'POST', body: form })
      fileRef.current!.value = ''
      fetchMedia()
    } catch (e: any) {
      setError(e.message)
    } finally {
      setUploading(false)
    }
  }

  const handleDelete = async (path: string) => {
    try {
      const res = await fetch(`/admin/api/media/${encodeURIComponent(path)}`, {
        method: 'DELETE',
      })
      if (!res.ok) throw new Error('delete failed')
      setDeleteTarget(null)
      fetchMedia()
    } catch (e: any) {
      setError(e.message)
    }
  }

  const fmtSize = (bytes: number) => {
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  }

  const isImage = (ext: string) =>
    ['.jpg', '.jpeg', '.png', '.gif', '.webp', '.svg', '.avif'].includes(ext)

  if (loading)
    return (
      <div className="text-muted-foreground py-24 text-center text-sm">
        加载中...
      </div>
    )
  if (error)
    return (
      <div className="text-destructive-foreground py-24 text-center text-sm">
        {error}
      </div>
    )
  if (!data) return null

  const groups = Object.entries(data.groups).sort(([a], [b]) =>
    a.localeCompare(b)
  )
  const activeItems = data.groups[activeGroup] || []

  return (
    <div>
      {/* Page header */}
      <div className="flex items-center justify-between mb-6">
        <div>
          <h2 className="text-xl font-semibold text-foreground tracking-tight">
            媒体库
          </h2>
          <p className="text-sm text-muted-foreground mt-0.5">
            {data.total} 个文件
          </p>
        </div>

        {/* Upload */}
        <label className="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md bg-primary text-primary-foreground hover:opacity-90 transition-opacity cursor-pointer">
          <Upload className="h-4 w-4" />
          <span>{uploading ? '上传中...' : '上传'}</span>
          <input
            type="file"
            ref={fileRef}
            className="hidden"
            onChange={handleUpload}
            disabled={uploading}
          />
        </label>
      </div>

      <div className="flex gap-6">
        {/* Directory sidebar */}
        <aside className="w-44 shrink-0">
          <div className="space-y-0.5">
            {groups.map(([group]) => (
              <button
                key={group}
                onClick={() => setActiveGroup(group)}
                className={
                  `flex w-full items-center gap-2 px-2.5 py-1.5 text-sm rounded-md transition-colors ` +
                  (activeGroup === group
                    ? 'bg-muted text-foreground font-medium'
                    : 'text-muted-foreground hover:text-foreground hover:bg-muted')
                }
              >
                <FolderOpen className="h-3.5 w-3.5 shrink-0" />
                <span>{group}</span>
              </button>
            ))}
          </div>
        </aside>

        {/* Media grid */}
        <div className="flex-1 min-w-0">
          {activeItems.length === 0 ? (
            <div className="text-muted-foreground py-24 text-center text-sm">
              此目录暂无媒体文件
            </div>
          ) : (
            <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-4">
              {activeItems.map((item) => (
                <div
                  key={item.path}
                  className="group relative"
                >
                  {/* Thumbnail */}
                  <div className="aspect-square bg-muted rounded-md flex items-center justify-center overflow-hidden">
                    {isImage(item.ext) ? (
                      <img
                        src={`/${item.path}`}
                        alt={item.name}
                        className="w-full h-full object-cover"
                        loading="lazy"
                      />
                    ) : (
                      <span className="text-xs text-muted-foreground">
                        {item.ext.replace('.', '').toUpperCase()}
                      </span>
                    )}
                  </div>

                  {/* Info overlay on hover */}
                  <div className="p-2.5">
                    <div className="text-xs text-foreground truncate">
                      {item.name}
                    </div>
                    <div className="text-[11px] text-muted-foreground mt-0.5">
                      {fmtSize(item.size)}
                    </div>
                  </div>

                  {/* Delete button on hover */}
                  <div className="absolute top-1.5 right-1.5 opacity-0 group-hover:opacity-100 transition-opacity">
                    {deleteTarget === item.path ? (
                      <span className="flex items-center gap-1 bg-background border border-border rounded-md px-1.5 py-1 text-[11px]">
                        <span className="text-muted-foreground">确认？</span>
                        <button
                          onClick={() => handleDelete(item.path)}
                          className="text-foreground hover:text-muted-foreground"
                        >
                          是
                        </button>
                        <button
                          onClick={() => setDeleteTarget(null)}
                          className="text-muted-foreground hover:text-foreground"
                        >
                          否
                        </button>
                      </span>
                    ) : (
                      <button
                        onClick={() => setDeleteTarget(item.path)}
                        className="bg-background border border-border rounded-md p-1 text-muted-foreground hover:text-foreground transition-colors"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
