import { Link, useLocation, useNavigate } from '@tanstack/react-router'
import { type ReactNode, useEffect, useId, useRef, useState } from 'react'
import { api, type Workspace } from '#/lib/api'
import { useAuth } from '#/lib/auth'
import { getLastWorkspaceId } from '#/lib/lastWorkspace'

export function AppShell({ children, fullWidth = false }: { children: ReactNode; fullWidth?: boolean }) {
  const { user, loading, logout } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()

  useEffect(() => {
    if (!loading && !user) navigate({ to: '/login' })
  }, [loading, user, navigate])

  if (loading) {
    return (
      <div role="status" className="min-h-screen flex items-center justify-center text-slate-500">
        Memuat…
      </div>
    )
  }
  if (!user) return null

  function doLogout() {
    logout()
    navigate({ to: '/login' })
  }

  const isWorkspaceActive = location.pathname === '/'
  const isAdminActive = location.pathname.startsWith('/admin')

  // fullWidth pages (currently just Live View) use a sidebar instead of a
  // top header, and lock to the viewport height with no page scroll — the
  // video grid itself is what manages its own overflow (pagination/swipe),
  // not the browser.
  if (fullWidth) {
    return (
      <div className="h-screen flex bg-slate-50 overflow-hidden">
        <SkipLink />
        <aside className="w-14 shrink-0 bg-white border-r border-slate-200 flex flex-col items-center py-3 gap-4">
          <Link to="/" title="NVR EZVIZ" className="font-semibold text-slate-900 text-xs">
            NVR
          </Link>
          <nav aria-label="Navigasi utama" className="flex flex-col items-center gap-3 text-slate-500">
            <Link
              to="/"
              title="Workspace"
              aria-label="Workspace"
              aria-current={isWorkspaceActive ? 'page' : undefined}
              className="hover:text-slate-900"
            >
              <WorkspaceIcon />
            </Link>
            <WorkspaceShortcutMenu segment="recordings" label="Rekaman" icon={<RecordingsIcon />} variant="icon" />
            {user.is_superadmin && (
              <Link
                to="/admin"
                title="Admin"
                aria-label="Admin"
                aria-current={isAdminActive ? 'page' : undefined}
                className="hover:text-slate-900"
              >
                <AdminIcon />
              </Link>
            )}
          </nav>
          <div className="flex-1" />
          <span
            title={user.email}
            aria-label={user.email}
            className="w-8 h-8 rounded-full bg-slate-200 text-slate-600 text-xs flex items-center justify-center"
          >
            {user.email[0]?.toUpperCase()}
          </span>
          <button onClick={doLogout} title="Keluar" aria-label="Keluar dari akun" className="text-slate-500 hover:text-slate-900">
            <LogoutIcon />
          </button>
        </aside>
        <main id="main-content" tabIndex={-1} className="flex-1 min-w-0 h-screen overflow-hidden p-2 focus:outline-none">
          {children}
        </main>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-slate-50">
      <SkipLink />
      <header className="border-b border-slate-200 bg-white">
        <div className="max-w-5xl mx-auto px-4 py-3 flex items-center justify-between">
          <Link to="/" className="font-semibold text-slate-900">
            NVR EZVIZ
          </Link>
          <nav aria-label="Navigasi utama" className="flex items-center gap-4 text-sm">
            <WorkspaceShortcutMenu segment="live" label="Live View" icon={<LiveIcon />} />
            <WorkspaceShortcutMenu segment="recordings" label="Rekaman" icon={<RecordingsIcon />} />
            <Link to="/" aria-current={isWorkspaceActive ? 'page' : undefined} className="text-slate-600 hover:text-slate-900">
              Workspace
            </Link>
            {user.is_superadmin && (
              <Link to="/admin" aria-current={isAdminActive ? 'page' : undefined} className="text-slate-600 hover:text-slate-900">
                Admin
              </Link>
            )}
            <span className="text-slate-500">{user.email}</span>
            <button onClick={doLogout} className="text-slate-600 hover:text-slate-900">
              Keluar
            </button>
          </nav>
        </div>
      </header>
      <main id="main-content" tabIndex={-1} className="max-w-5xl mx-auto px-4 py-6 focus:outline-none">
        {children}
      </main>
    </div>
  )
}

// Skips the header/sidebar nav for keyboard users — invisible until it
// receives focus (first Tab press on the page), per WCAG 2.4.1 "Bypass
// Blocks". Points at <main id="main-content">, which is focusable so the
// skip actually moves keyboard focus there, not just the visual viewport.
function SkipLink() {
  return (
    <a
      href="#main-content"
      className="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:bg-slate-900 focus:text-white focus:rounded focus:px-4 focus:py-2 focus:text-sm focus:font-medium"
    >
      Lewati ke konten utama
    </a>
  )
}

// Always-visible shortcut straight to Live View or Rekaman, from anywhere in
// the app (Admin included) — without it, reaching either page means
// Workspace list -> pick a workspace -> find the right button, which is
// what made both hard to find. One click if a workspace was visited
// recently (remembered in localStorage) or there's only one workspace at
// all; otherwise a small menu to pick which workspace to jump into.
function WorkspaceShortcutMenu({
  segment,
  label,
  icon,
  variant = 'nav',
}: {
  segment: 'live' | 'recordings'
  label: string
  icon: ReactNode
  variant?: 'nav' | 'icon'
}) {
  const navigate = useNavigate()
  const [workspaces, setWorkspaces] = useState<Workspace[] | null>(null)
  const [loadingList, setLoadingList] = useState(false)
  const [open, setOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const menuId = useId()

  useEffect(() => {
    if (!open) return
    function onClickOutside(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) setOpen(false)
    }
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        setOpen(false)
        triggerRef.current?.focus()
      }
    }
    document.addEventListener('mousedown', onClickOutside)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('mousedown', onClickOutside)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [open])

  function goTo(workspaceId: string) {
    setOpen(false)
    if (segment === 'live') navigate({ to: '/workspaces/$workspaceId/live', params: { workspaceId } })
    else navigate({ to: '/workspaces/$workspaceId/recordings', params: { workspaceId } })
  }

  async function onClick() {
    const remembered = getLastWorkspaceId()
    if (remembered) {
      goTo(remembered)
      return
    }

    if (workspaces) {
      if (workspaces.length === 1) goTo(workspaces[0].id)
      else setOpen(true)
      return
    }

    setLoadingList(true)
    try {
      const list = await api.listWorkspaces()
      setWorkspaces(list)
      if (list.length === 1) goTo(list[0].id)
      else setOpen(true)
    } catch {
      setOpen(true)
    } finally {
      setLoadingList(false)
    }
  }

  return (
    <div ref={containerRef} className="relative">
      <button
        ref={triggerRef}
        onClick={onClick}
        disabled={loadingList}
        title={variant === 'icon' ? label : undefined}
        aria-label={variant === 'icon' ? label : undefined}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? menuId : undefined}
        className={
          variant === 'icon'
            ? 'text-slate-500 hover:text-slate-900 disabled:opacity-50'
            : 'flex items-center gap-1.5 bg-slate-900 text-white rounded px-3 py-1.5 font-medium hover:bg-slate-700 disabled:opacity-50'
        }
      >
        {icon}
        {variant === 'nav' && label}
      </button>
      {open && (
        <div
          id={menuId}
          role="menu"
          aria-label={label}
          className={`absolute w-56 bg-white border border-slate-200 rounded-lg shadow-lg py-1 z-20 ${
            variant === 'icon' ? 'left-full ml-2 top-0' : 'right-0 top-full mt-1'
          }`}
        >
          {workspaces === null && (
            <div role="status" className="px-3 py-2 text-slate-500">
              Memuat…
            </div>
          )}
          {workspaces?.length === 0 && <div className="px-3 py-2 text-slate-500">Belum ada workspace.</div>}
          {workspaces?.map((ws) => (
            <button key={ws.id} role="menuitem" onClick={() => goTo(ws.id)} className="w-full text-left px-3 py-2 hover:bg-slate-50 text-slate-700">
              {ws.name}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

function WorkspaceIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <rect x="3" y="3" width="7" height="7" />
      <rect x="14" y="3" width="7" height="7" />
      <rect x="14" y="14" width="7" height="7" />
      <rect x="3" y="14" width="7" height="7" />
    </svg>
  )
}

function LiveIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <rect x="2" y="5" width="20" height="14" rx="2" />
      <path d="M8 21h8M12 17v4" />
      <circle cx="17" cy="9" r="1.5" fill="currentColor" stroke="none" />
    </svg>
  )
}

function RecordingsIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <circle cx="12" cy="12" r="9" />
      <polygon points="10 8 16 12 10 16 10 8" fill="currentColor" stroke="none" />
    </svg>
  )
}

function AdminIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M12 2l8 4v6c0 5-3.5 8.5-8 10-4.5-1.5-8-5-8-10V6l8-4z" />
    </svg>
  )
}

function LogoutIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
      <polyline points="16 17 21 12 16 7" />
      <line x1="21" y1="12" x2="9" y2="12" />
    </svg>
  )
}
