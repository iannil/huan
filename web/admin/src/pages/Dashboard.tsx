import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { FileText, Plus, ArrowRight } from 'lucide-react'

interface StatusResponse {
  title: string
  baseURL: string
  total: number
  drafts: number
  sections: number
  languages: string[]
  sectionBreakdown: Record<string, number>
}

export default function Dashboard() {
  const [status, setStatus] = useState<StatusResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const navigate = useNavigate()

  const fetchStatus = () => {
    fetch('/admin/api/status')
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

  const stats = [
    { label: '内容总数', value: status.total },
    { label: '分类数', value: status.sections },
    { label: '草稿', value: status.drafts },
    { label: '语言', value: status.languages.length || 1 },
  ]

  return (
    <div>
      {/* Page header */}
      <div className="mb-8">
        <h2 className="text-xl font-semibold text-foreground tracking-tight">
          {status.title}
        </h2>
        <p className="text-sm text-muted-foreground mt-1">{status.baseURL}</p>
      </div>

      {/* Stat cards — borderless, clean */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-px mb-10">
        {stats.map((s) => (
          <div
            key={s.label}
            className="px-5 py-4"
          >
            <div className="text-2xl font-semibold text-foreground tracking-tight">
              {s.value}
            </div>
            <div className="text-sm text-muted-foreground mt-1">{s.label}</div>
          </div>
        ))}
      </div>

      {/* Two-column lower section — borderless cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-6">
        {/* Content distribution */}
        <div>
          <h3 className="text-sm font-medium text-foreground mb-3">
            内容分布
          </h3>
          <div className="space-y-0.5">
            {Object.entries(status.sectionBreakdown)
              .sort(([, a], [, b]) => b - a)
              .map(([sec, count]) => (
                <button
                  key={sec}
                  onClick={() => navigate(`/admin/content?section=${sec}`)}
                  className="flex w-full items-center justify-between px-2 py-1.5 rounded-md text-sm text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
                >
                  <span className="capitalize">
                    {sec === '_root' ? '根目录' : sec}
                  </span>
                  <span className="tabular-nums text-muted-foreground">
                    {count}
                  </span>
                </button>
              ))}
          </div>
        </div>

        {/* Quick actions */}
        <div>
          <h3 className="text-sm font-medium text-foreground mb-3">
            快速操作
          </h3>
          <div className="space-y-0.5">
            <button
              onClick={() => navigate('/admin/content/new')}
              className="flex w-full items-center gap-3 px-2 py-1.5 rounded-md text-sm text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
            >
              <Plus className="h-4 w-4 shrink-0" />
              <span>新建内容</span>
            </button>
            <button
              onClick={() => navigate('/admin/content')}
              className="flex w-full items-center gap-3 px-2 py-1.5 rounded-md text-sm text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
            >
              <FileText className="h-4 w-4 shrink-0" />
              <span>浏览内容</span>
            </button>
            <button
              onClick={() => navigate('/admin/media')}
              className="flex w-full items-center gap-3 px-2 py-1.5 rounded-md text-sm text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
            >
              <ArrowRight className="h-4 w-4 shrink-0" />
              <span>管理媒体</span>
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
