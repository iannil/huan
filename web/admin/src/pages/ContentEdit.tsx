import { useState, useEffect, useCallback } from 'react'
import { useSearchParams, useNavigate } from 'react-router-dom'
import { ArrowLeft, Save, Trash2, FileText } from 'lucide-react'
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

  return (
    <div>
      {/* Toolbar — borderless */}
      <div className="flex items-center gap-3 pb-4 mb-6">
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
              <span>删除</span>
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

      {/* Status messages */}
      {error && (
        <div className="mb-4 px-3 py-2 rounded-md bg-muted text-sm text-destructive-foreground">
          {error}
        </div>
      )}
      {success && (
        <div className="mb-4 px-3 py-2 rounded-md bg-muted text-sm text-muted-foreground">
          保存成功
        </div>
      )}

      {/* Editor */}
      <div className="space-y-5 max-w-3xl">
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
            rows={28}
            className="font-mono text-sm border-0 border-b-2 border-border rounded-none px-0 leading-relaxed focus-visible:border-foreground"
          />
        </div>
      </div>
    </div>
  )
}
