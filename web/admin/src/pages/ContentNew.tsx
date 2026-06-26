import { useState, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { ArrowLeft, Plus } from 'lucide-react'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

const sections = [
  { value: 'posts', label: 'posts' },
  { value: 'books', label: 'books' },
  { value: 'practices', label: 'practices' },
  { value: 'products', label: 'products' },
  { value: 'gallery', label: 'gallery' },
  { value: 'developer', label: 'developer' },
  { value: '_root', label: '根目录' },
]

export default function ContentNew() {
  const navigate = useNavigate()
  const [title, setTitle] = useState('')
  const [section, setSection] = useState('posts')
  const [language, setLanguage] = useState('en')
  const [draft, setDraft] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleCreate = useCallback(async () => {
    if (!title.trim()) {
      setError('标题不能为空')
      return
    }
    setSaving(true)
    setError(null)
    try {
      const filename = title
        .toLowerCase()
        .replace(/[^a-z0-9\u4e00-\u9fff]+/g, '-')
        .replace(/^-+|-+$/g, '')
        .substring(0, 80)

      // Append language suffix for multi-language filename convention
      const langFilename = language ? `${filename}.${language}` : filename

      const res = await fetch('/admin/api/content', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title: title.trim(), section, filename: langFilename, draft }),
      })
      const data = await res.json()
      if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`)
      navigate(`/admin/content/edit?path=${encodeURIComponent(data.relPath)}`)
    } catch (e: any) {
      setError(e.message)
    } finally {
      setSaving(false)
    }
  }, [title, section, language, draft, navigate])

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

        <h2 className="text-sm text-foreground flex-1">新建内容</h2>

        <button
          onClick={handleCreate}
          disabled={saving}
          className="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md bg-primary text-primary-foreground hover:opacity-90 transition-opacity disabled:opacity-40 disabled:cursor-not-allowed"
        >
          <Plus className="h-3.5 w-3.5" />
          <span>{saving ? '创建中...' : '创建'}</span>
        </button>
      </div>

      {/* Status messages */}
      {error && (
        <div className="mb-4 px-3 py-2 rounded-md bg-muted text-sm text-destructive-foreground">
          {error}
        </div>
      )}

      {/* Form */}
      <div className="max-w-lg space-y-5">
        <div>
          <label className="block text-sm text-muted-foreground mb-1.5">
            标题
          </label>
          <Input
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="输入文章标题..."
            className="border-0 border-b-2 border-border rounded-none px-0 h-8 text-base focus-visible:border-foreground"
          />
        </div>

        <div>
          <label className="block text-sm text-muted-foreground mb-1.5">
            分类
          </label>
          <Select value={section} onValueChange={(v) => v && setSection(v)}>
            <SelectTrigger className="w-full border-border">
              <SelectValue placeholder="选择分类" />
            </SelectTrigger>
            <SelectContent>
              {sections.map((s) => (
                <SelectItem key={s.value} value={s.value}>
                  {s.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div>
          <label className="block text-sm text-muted-foreground mb-1.5">
            语言
          </label>
          <Select value={language} onValueChange={(v) => v && setLanguage(v)}>
            <SelectTrigger className="w-full border-border">
              <SelectValue placeholder="选择语言" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="en">EN — 英语</SelectItem>
              <SelectItem value="zh-CN">ZH-CN — 简体中文</SelectItem>
              <SelectItem value="zh-TW">ZH-TW — 繁体中文</SelectItem>
              <SelectItem value="ja">JA — 日语</SelectItem>
              <SelectItem value="ko">KO — 韩语</SelectItem>
              <SelectItem value="es">ES — 西班牙语</SelectItem>
              <SelectItem value="fr">FR — 法语</SelectItem>
              <SelectItem value="de">DE — 德语</SelectItem>
              <SelectItem value="pt">PT — 葡萄牙语</SelectItem>
            </SelectContent>
          </Select>
        </div>

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
      </div>
    </div>
  )
}
