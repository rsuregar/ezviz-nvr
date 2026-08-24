import { Link, useNavigate } from '@tanstack/react-router'
import { type ReactNode, useEffect } from 'react'
import { useAuth } from '#/lib/auth'

export function AppShell({ children }: { children: ReactNode }) {
  const { user, loading, logout } = useAuth()
  const navigate = useNavigate()

  useEffect(() => {
    if (!loading && !user) navigate({ to: '/login' })
  }, [loading, user, navigate])

  if (loading) {
    return <div className="min-h-screen flex items-center justify-center text-slate-400">Memuat…</div>
  }
  if (!user) return null

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
            <button
              onClick={() => {
                logout()
                navigate({ to: '/login' })
              }}
              className="text-slate-600 hover:text-slate-900"
            >
              Keluar
            </button>
          </nav>
        </div>
      </header>
      <main className="max-w-5xl mx-auto px-4 py-6">{children}</main>
    </div>
  )
}
