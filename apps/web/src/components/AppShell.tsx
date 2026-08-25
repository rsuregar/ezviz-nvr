import { Link, useNavigate } from '@tanstack/react-router'
import { type ReactNode, useEffect } from 'react'
import { useAuth } from '#/lib/auth'

export function AppShell({ children, fullWidth = false }: { children: ReactNode; fullWidth?: boolean }) {
  const { user, loading, logout } = useAuth()
  const navigate = useNavigate()

  useEffect(() => {
    if (!loading && !user) navigate({ to: '/login' })
  }, [loading, user, navigate])

  if (loading) {
    return <div className="min-h-screen flex items-center justify-center text-slate-400">Memuat…</div>
  }
  if (!user) return null

  function doLogout() {
    logout()
    navigate({ to: '/login' })
  }

  // fullWidth pages (currently just Live View) use a sidebar instead of a
  // top header, and lock to the viewport height with no page scroll — the
  // video grid itself is what manages its own overflow (pagination/swipe),
  // not the browser.
  if (fullWidth) {
    return (
      <div className="h-screen flex bg-slate-50 overflow-hidden">
        <aside className="w-14 shrink-0 bg-white border-r border-slate-200 flex flex-col items-center py-3 gap-4">
          <Link to="/" title="NVR EZVIZ" className="font-semibold text-slate-900 text-xs">
            NVR
          </Link>
          <nav className="flex flex-col items-center gap-3 text-slate-500">
            <Link to="/" title="Workspace" className="hover:text-slate-900">
              <WorkspaceIcon />
            </Link>
            {user.is_superadmin && (
              <Link to="/admin" title="Admin" className="hover:text-slate-900">
                <AdminIcon />
              </Link>
            )}
          </nav>
          <div className="flex-1" />
          <span title={user.email} className="w-8 h-8 rounded-full bg-slate-200 text-slate-600 text-xs flex items-center justify-center">
            {user.email[0]?.toUpperCase()}
          </span>
          <button onClick={doLogout} title="Keluar" className="text-slate-500 hover:text-slate-900">
            <LogoutIcon />
          </button>
        </aside>
        <main className="flex-1 min-w-0 h-screen overflow-hidden p-2">{children}</main>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-slate-50">
      <header className="border-b border-slate-200 bg-white">
        <div className="max-w-5xl mx-auto px-4 py-3 flex items-center justify-between">
          <Link to="/" className="font-semibold text-slate-900">
            NVR EZVIZ
          </Link>
          <nav className="flex items-center gap-4 text-sm">
            <Link to="/" className="text-slate-600 hover:text-slate-900">
              Workspace
            </Link>
            {user.is_superadmin && (
              <Link to="/admin" className="text-slate-600 hover:text-slate-900">
                Admin
              </Link>
            )}
            <span className="text-slate-400">{user.email}</span>
            <button onClick={doLogout} className="text-slate-600 hover:text-slate-900">
              Keluar
            </button>
          </nav>
        </div>
      </header>
      <main className="max-w-5xl mx-auto px-4 py-6">{children}</main>
    </div>
  )
}

function WorkspaceIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <rect x="3" y="3" width="7" height="7" />
      <rect x="14" y="3" width="7" height="7" />
      <rect x="14" y="14" width="7" height="7" />
      <rect x="3" y="14" width="7" height="7" />
    </svg>
  )
}

function AdminIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M12 2l8 4v6c0 5-3.5 8.5-8 10-4.5-1.5-8-5-8-10V6l8-4z" />
    </svg>
  )
}

function LogoutIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
      <polyline points="16 17 21 12 16 7" />
      <line x1="21" y1="12" x2="9" y2="12" />
    </svg>
  )
}
