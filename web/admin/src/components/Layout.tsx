import { Outlet, NavLink } from 'react-router-dom'
import { useState, useEffect } from 'react'
import {
  LayoutDashboard,
  FileText,
  Image,
  Hammer,
  ExternalLink,
} from 'lucide-react'

const navItems = [
  { to: '/admin/', label: '概览', icon: LayoutDashboard, end: true },
  { to: '/admin/content', label: '内容', icon: FileText, end: false },
  { to: '/admin/media', label: '媒体', icon: Image, end: false },
]

export default function Layout() {
  const [building, setBuilding] = useState(false)
  const [buildMsg, setBuildMsg] = useState('')
  const [serveURL, setServeURL] = useState('')

  useEffect(() => {
    fetch('/admin/api/status')
      .then((r) => (r.ok ? r.json() : null))
      .then((d) => {
        if (d?.serveURL) setServeURL(d.serveURL)
      })
      .catch(() => {})
  }, [])

  const triggerBuild = async () => {
    setBuilding(true)
    setBuildMsg('')
    try {
      const res = await fetch('/admin/api/build', { method: 'POST' })
      const data = await res.json()
      if (res.ok) {
        setBuildMsg('已触发')
      } else {
        setBuildMsg(data.error || '失败')
      }
    } catch {
      setBuildMsg('网络错误')
    }
    setTimeout(() => {
      setBuilding(false)
      setBuildMsg('')
    }, 2000)
  }

  const sidebarActiveClass =
    'text-sidebar-foreground font-semibold border-l-2 border-sidebar-foreground'
  const sidebarInactiveClass =
    'text-muted-foreground hover:text-sidebar-foreground border-l-2 border-transparent hover:border-sidebar-border ml-0'

  return (
    <div className="flex min-h-screen bg-background">
      {/* Sidebar */}
      <aside className="w-56 shrink-0 border-r border-border bg-sidebar flex flex-col">
        {/* Logo — larger, more confident */}
        <div className="px-5 pt-6 pb-5">
          <h1 className="text-base font-semibold text-sidebar-foreground tracking-tight">
            huan
          </h1>
        </div>

        {/* Navigation — left-bar active indicator, no bg hover */}
        <nav className="flex-1 px-3 space-y-0.5">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                `flex items-center gap-2.5 px-3 py-1.5 text-sm transition-colors ml-0 ` +
                (isActive ? sidebarActiveClass : sidebarInactiveClass)
              }
            >
              <item.icon className="h-4 w-4 shrink-0" />
              <span>{item.label}</span>
            </NavLink>
          ))}
        </nav>

        {/* Divider */}
        <div className="mx-5 my-3 h-px bg-sidebar-border" />

        {/* Actions */}
        <div className="px-3 pb-2 space-y-0.5">
          <button
            onClick={triggerBuild}
            disabled={building}
            className="flex w-full items-center gap-2.5 px-3 py-1.5 text-sm border-l-2 border-transparent text-muted-foreground hover:text-sidebar-foreground hover:border-sidebar-border transition-colors disabled:opacity-40 disabled:cursor-not-allowed ml-0"
          >
            <Hammer className="h-4 w-4 shrink-0" />
            <span>{building ? '构建中...' : buildMsg || '构建'}</span>
          </button>

          {serveURL && (
            <a
              href={serveURL}
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-2.5 px-3 py-1.5 text-sm border-l-2 border-transparent text-muted-foreground hover:text-sidebar-foreground hover:border-sidebar-border transition-colors ml-0"
            >
              <ExternalLink className="h-4 w-4 shrink-0" />
              <span>预览</span>
            </a>
          )}
        </div>

        {/* Version footer */}
        <div className="px-5 py-3 border-t border-sidebar-border">
          <span className="text-xs text-muted-foreground">v0.3.0</span>
        </div>
      </aside>

      {/* Main content area — wider for more breathing room */}
      <main className="flex-1 min-w-0 bg-background">
        <div className="max-w-6xl mx-auto px-8 py-8">
          <Outlet />
        </div>
      </main>
    </div>
  )
}
