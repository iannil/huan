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

  const navActiveClass =
    'text-foreground font-semibold after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 after:bg-foreground'
  const navInactiveClass =
    'text-muted-foreground hover:text-foreground'

  return (
    <div className="min-h-screen bg-background">
      {/* Top Navigation */}
      <header className="border-b border-border bg-sidebar sticky top-0 z-10">
        <div className="max-w-6xl mx-auto px-6 h-14 flex items-center justify-between">
          {/* Left: Logo */}
          <h1 className="text-base font-semibold text-sidebar-foreground tracking-tight mr-8">
            huan
          </h1>

          {/* Center: Nav items */}
          <nav className="flex items-center gap-1">
            {navItems.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.end}
                className={({ isActive }) =>
                  `relative flex items-center gap-2 px-3 py-1.5 text-sm transition-colors ` +
                  (isActive ? navActiveClass : navInactiveClass)
                }
              >
                <item.icon className="h-4 w-4 shrink-0" />
                <span>{item.label}</span>
              </NavLink>
            ))}
          </nav>

          {/* Right: Actions */}
          <div className="flex items-center gap-2">
            <button
              onClick={triggerBuild}
              disabled={building}
              className="flex items-center gap-1.5 px-3 py-1.5 text-sm text-muted-foreground hover:text-sidebar-foreground transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            >
              <Hammer className="h-4 w-4 shrink-0" />
              <span>{building ? '构建中...' : buildMsg || '构建'}</span>
            </button>

            {serveURL && (
              <a
                href={serveURL}
                target="_blank"
                rel="noopener noreferrer"
                className="flex items-center gap-1.5 px-3 py-1.5 text-sm text-muted-foreground hover:text-sidebar-foreground transition-colors"
              >
                <ExternalLink className="h-4 w-4 shrink-0" />
                <span>预览</span>
              </a>
            )}

            <span className="text-xs text-muted-foreground ml-2">v0.3.0</span>
          </div>
        </div>
      </header>

      {/* Main content area */}
      <main className="min-h-0 bg-background">
        <div className="max-w-6xl mx-auto px-8 py-8">
          <Outlet />
        </div>
      </main>
    </div>
  )
}
