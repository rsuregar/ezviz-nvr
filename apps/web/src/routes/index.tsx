import { createFileRoute, Link } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { AppShell } from '#/components/AppShell'
import { api, ApiError, type Workspace } from '#/lib/api'
import { useAuth } from '#/lib/auth'

export const Route = createFileRoute('/')({ component: Home })

function Home() {
  return (
    <AppShell>
      <WorkspaceList />
    </AppShell>
  )
}

function WorkspaceList() {
  const { user } = useAuth()
  const [workspaces, setWorkspaces] = useState<Workspace[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [name, setName] = useState('')
  const [creating, setCreating] = useState(false)

  async function load() {
    try {
      setWorkspaces(await api.listWorkspaces())
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal memuat workspace')
    }
  }

  useEffect(() => {
    load()
  }, [])

  async function onCreate(e: React.FormEvent) {
    e.preventDefault()
    setCreating(true)
    try {
      await api.createWorkspace(name)
      setName('')
      await load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal membuat workspace')
    } finally {
      setCreating(false)
    }
  }

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold text-slate-900">Workspace</h1>

      {error && <p className="text-sm text-red-600">{error}</p>}

      <div className="grid gap-3 sm:grid-cols-2">
        {workspaces?.map((ws) => (
          <Link
            key={ws.id}
            to="/workspaces/$workspaceId"
            params={{ workspaceId: ws.id }}
            className="block bg-white border border-slate-200 rounded-lg p-4 hover:border-slate-400 transition"
          >
            <div className="font-medium text-slate-900">{ws.name}</div>
            <div className="text-sm text-slate-500">{ws.slug}</div>
          </Link>
        ))}
        {workspaces?.length === 0 && (
          <p className="text-sm text-slate-500">Belum ada workspace.</p>
        )}
      </div>

      {user?.is_superadmin && (
        <form onSubmit={onCreate} className="bg-white border border-slate-200 rounded-lg p-4 max-w-sm space-y-3">
          <h2 className="font-medium text-slate-900">Buat workspace baru</h2>
          <input
            required
            placeholder="Nama workspace"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="w-full border border-slate-300 rounded px-3 py-2 text-sm"
          />
          <button
            type="submit"
            disabled={creating}
            className="bg-slate-900 text-white rounded px-4 py-2 text-sm font-medium disabled:opacity-50"
          >
            {creating ? 'Membuat…' : 'Buat'}
          </button>
        </form>
      )}
    </div>
  )
}
