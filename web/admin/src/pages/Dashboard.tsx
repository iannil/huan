import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { apiFetch } from '../lib/api'
import {
  FileText,
  Plus,
  ArrowRight,
  Edit3,
} from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'

interface ContentItem {
  title: string
  relPath: string
  section: string
  draft: boolean
  date: string
  language: string
  url: string
}

interface StatusResponse {
  title: string
  baseURL: string
  total: number
  published: number
  drafts: number
  sections: number
  languages: string[]
  mediaCount: number
  sectionBreakdown: Record<string, number>
  recentContent: ContentItem[]
}

export default function Dashboard() {
  const [status, setStatus] = useState<StatusResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const navigate = useNavigate()

  const fetchStatus = () => {
    apiFetch('/admin/api/status')
      .then((r) => (r.ok ? r.json() : null))
      .then((d) => setStatus(d))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    fetchStatus()
  }, [])

  if (loading)
    return (
      <div className="text-muted-foreground py-24 text-center text-sm">
        加载中...
      </div>
    )
  if (!status)
    return (
      <div className="text-destructive-foreground py-24 text-center text-sm">
        加载失败
      </div>
    )

  const total = status.total > 0 ? status.total : 1
  const publishedPct = Math.round((status.published / total) * 100)

  const stats = [
    { label: '内容总数', value: status.total },
    { label: '已发布', value: status.published, sub: `${publishedPct}%` },
    { label: '草稿', value: status.drafts },
    { label: '分类数', value: status.sections },
    { label: '语言', value: status.languages.length || 1 },
    { label: '媒体文件', value: status.mediaCount },
  ]

  const recent = status.recentContent || []

  return (
    <div>
      {/* Page header */}
      <div className="mb-8">
        <h2 className="text-xl font-semibold text-foreground tracking-tight">
          {status.title}
        </h2>
        <p className="text-sm text-muted-foreground mt-1">{status.baseURL}</p>
      </div>

      {/* Stat cards — 6 borderless metrics */}
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-px mb-6">
        {stats.map((s) => (
          <div key={s.label} className="px-4 py-3.5">
            <div className="text-2xl font-semibold text-foreground tracking-tight">
              {s.value}
            </div>
            <div className="text-sm text-muted-foreground mt-0.5">{s.label}</div>
            {s.sub && (
              <div className="text-xs text-muted-foreground/50 mt-0.5">
                {s.sub}
              </div>
            )}
          </div>
        ))}
      </div>

      {/* Published / Drafts progress bar */}
      {status.total > 0 && (
        <div className="mb-10">
          <div className="flex items-center gap-4 text-xs text-muted-foreground mb-2">
            <span className="flex items-center gap-1.5">
              <span className="h-2 w-2 rounded-full bg-foreground" />
              已发布 {status.published}
            </span>
            <span className="flex items-center gap-1.5">
              <span className="h-2 w-2 rounded-full bg-muted-foreground/25" />
              草稿 {status.drafts}
            </span>
          </div>
          <div className="h-1.5 bg-muted rounded-full overflow-hidden">
            <div
              className="h-full bg-foreground rounded-full transition-all"
              style={{ width: `${publishedPct}%` }}
            />
          </div>
        </div>
      )}

      {/* Content grid: 2/3 left + 1/3 right */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Left column */}
        <div className="lg:col-span-2 space-y-6">
          {/* Content distribution */}
          <Card>
            <CardHeader>
              <CardTitle>内容分布</CardTitle>
            </CardHeader>
            <CardContent className="p-0">
              {Object.keys(status.sectionBreakdown).length === 0 ? (
                <div className="px-4 py-6 text-sm text-muted-foreground text-center">
                  暂无内容
                </div>
              ) : (
                <div className="divide-y divide-border">
                  {Object.entries(status.sectionBreakdown)
                    .sort(([, a], [, b]) => b - a)
                    .map(([sec, count]) => {
                      const pct = Math.round((count / total) * 100)
                      return (
                        <button
                          key={sec}
                          onClick={() =>
                            navigate(`/admin/content?section=${sec}`)
                          }
                          className="flex w-full items-center gap-3 px-4 py-2.5 text-sm text-muted-foreground hover:text-foreground hover:bg-muted transition-colors text-left"
                        >
                          <span className="w-20 shrink-0 capitalize text-foreground font-medium truncate">
                            {sec === '_root' ? '根目录' : sec}
                          </span>
                          <div className="flex-1 h-1.5 bg-muted rounded-full overflow-hidden">
                            <div
                              className="h-full bg-foreground/60 rounded-full"
                              style={{ width: `${pct}%` }}
                            />
                          </div>
                          <span className="tabular-nums w-8 text-right text-muted-foreground">
                            {count}
                          </span>
                        </button>
                      )
                    })}
                </div>
              )}
            </CardContent>
          </Card>

          {/* Recent content */}
          <Card>
            <CardHeader>
              <CardTitle>最近内容</CardTitle>
            </CardHeader>
            <CardContent className="p-0">
              {recent.length === 0 ? (
                <div className="px-4 py-6 text-sm text-muted-foreground text-center">
                  暂无内容
                </div>
              ) : (
                <div className="divide-y divide-border">
                  {recent.map((item) => (
                    <button
                      key={item.relPath}
                      onClick={() =>
                        navigate(
                          `/admin/content/edit?path=${encodeURIComponent(item.relPath)}`,
                        )
                      }
                      className="flex w-full items-center gap-3 px-4 py-2.5 text-sm hover:bg-muted transition-colors text-left"
                    >
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="text-foreground font-medium truncate">
                            {item.title || item.relPath}
                          </span>
                          {item.draft && (
                            <Badge
                              variant="secondary"
                              className="shrink-0 leading-none"
                            >
                              草稿
                            </Badge>
                          )}
                        </div>
                        <div className="flex items-center gap-2 mt-0.5 text-xs text-muted-foreground">
                          <span className="capitalize">
                            {item.section === '_root' ? '根目录' : item.section}
                          </span>
                          {item.date && (
                            <>
                              <span>·</span>
                              <span>{item.date}</span>
                            </>
                          )}
                          {item.language && (
                            <>
                              <span>·</span>
                              <span>{item.language}</span>
                            </>
                          )}
                        </div>
                      </div>
                      <ArrowRight className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
                    </button>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </div>

        {/* Right column */}
        <div className="space-y-6">
          {/* Quick actions */}
          <Card>
            <CardHeader>
              <CardTitle>快速操作</CardTitle>
            </CardHeader>
            <CardContent className="p-0">
              <div className="divide-y divide-border">
                <button
                  onClick={() => navigate('/admin/content/new')}
                  className="flex w-full items-center gap-3 px-4 py-2.5 text-sm text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
                >
                  <Plus className="h-4 w-4 shrink-0" />
                  <span>新建内容</span>
                </button>
                <button
                  onClick={() => navigate('/admin/content')}
                  className="flex w-full items-center gap-3 px-4 py-2.5 text-sm text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
                >
                  <FileText className="h-4 w-4 shrink-0" />
                  <span>浏览内容</span>
                </button>
                <button
                  onClick={() => navigate('/admin/media')}
                  className="flex w-full items-center gap-3 px-4 py-2.5 text-sm text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
                >
                  <Edit3 className="h-4 w-4 shrink-0" />
                  <span>管理媒体</span>
                </button>
              </div>
            </CardContent>
          </Card>

          {/* Site info summary */}
          <Card>
            <CardHeader>
              <CardTitle>站点摘要</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2.5">
              <div className="flex justify-between text-sm">
                <span className="text-muted-foreground">总内容数</span>
                <span className="text-foreground font-medium tabular-nums">
                  {status.total}
                </span>
              </div>
              <div className="flex justify-between text-sm">
                <span className="text-muted-foreground">已发布</span>
                <span className="text-foreground font-medium tabular-nums">
                  {status.published}
                </span>
              </div>
              <div className="flex justify-between text-sm">
                <span className="text-muted-foreground">草稿</span>
                <span className="text-foreground font-medium tabular-nums">
                  {status.drafts}
                </span>
              </div>
              <div className="flex justify-between text-sm">
                <span className="text-muted-foreground">分类</span>
                <span className="text-foreground font-medium tabular-nums">
                  {status.sections}
                </span>
              </div>
              <div className="flex justify-between text-sm">
                <span className="text-muted-foreground">语言</span>
                <span className="text-foreground font-medium tabular-nums">
                  {status.languages.length || 1}
                </span>
              </div>
              <div className="flex justify-between text-sm">
                <span className="text-muted-foreground">媒体文件</span>
                <span className="text-foreground font-medium tabular-nums">
                  {status.mediaCount}
                </span>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  )
}
