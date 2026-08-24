import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { AppShell } from '#/components/AppShell'
import { api, ApiError, type AuditLogEntry, type Camera, type Site, type User } from '#/lib/api'
import { useAuth } from '#/lib/auth'

export const Route = createFileRoute('/admin')({ component: AdminPage })

type Tab = 'users' | 'sites' | 'health' | 'audit'

function AdminPage() {
  const { user, loading } = useAuth()
  const navigate = useNavigate()
  const [tab, setTab] = useState<Tab>('users')

  useEffect(() => {
    if (!loading && user && !user.is_superadmin) navigate({ to: '/' })
  }, [loading, user, navigate])

  return (
    <AppShell>
      <div className="space-y-6">
        <h1 className="text-2xl font-semibold text-slate-900">Admin</h1>

        <div className="flex gap-1 border-b border-slate-200">
          {(['users', 'sites', 'health', 'audit'] as Tab[]).map((t) => (
            <button
              key={t}
              onClick={() => setTab(t)}
              className={`px-3 py-2 text-sm font-medium border-b-2 -mb-px ${
                tab === t ? 'border-slate-900 text-slate-900' : 'border-transparent text-slate-500'
              }`}
            >
              {t === 'users' ? 'Users' : t === 'sites' ? 'Sites & Kamera' : t === 'health' ? 'Health' : 'Audit Log'}
            </button>
          ))}
        </div>

        {tab === 'users' && <UsersTab />}
        {tab === 'sites' && <SitesTab />}
        {tab === 'health' && <HealthTab />}
        {tab === 'audit' && <AuditTab />}
      </div>
    </AppShell>
  )
}

function UsersTab() {
  const [users, setUsers] = useState<User[]>([])
  const [error, setError] = useState<string | null>(null)
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [name, setName] = useState('')
  const [isSuperAdmin, setIsSuperAdmin] = useState(false)

  async function load() {
    try {
      setUsers(await api.listUsers())
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal memuat users')
    }
  }

  useEffect(() => {
    load()
  }, [])

  async function onCreate(e: React.FormEvent) {
    e.preventDefault()
    try {
      await api.createUser({ email, password, name, is_superadmin: isSuperAdmin })
      setEmail('')
      setPassword('')
      setName('')
      setIsSuperAdmin(false)
      await load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal membuat user')
    }
  }

  return (
    <div className="space-y-4">
      {error && <p className="text-sm text-red-600">{error}</p>}

      <div className="bg-white border border-slate-200 rounded-lg divide-y divide-slate-100">
        {users.map((u) => (
          <div key={u.id} className="p-4 flex items-center justify-between">
            <div>
              <div className="font-medium text-slate-900">
                {u.name} <span className="text-slate-400">({u.email})</span>
              </div>
              <div className="text-xs text-slate-500 font-mono">{u.id}</div>
            </div>
            <div className="flex items-center gap-3">
              {u.is_superadmin && <span className="text-xs px-2 py-0.5 rounded-full bg-amber-100 text-amber-700">superadmin</span>}
              <button
                onClick={async () => {
                  await api.deleteUser(u.id)
                  await load()
                }}
                className="text-xs text-red-600"
              >
                Hapus
              </button>
            </div>
          </div>
        ))}
      </div>

      <form onSubmit={onCreate} className="bg-white border border-slate-200 rounded-lg p-4 max-w-md space-y-3">
        <h2 className="font-medium text-slate-900 text-sm">Buat user baru</h2>
        <input required placeholder="Nama" value={name} onChange={(e) => setName(e.target.value)} className="w-full border border-slate-300 rounded px-3 py-2 text-sm" />
        <input required type="email" placeholder="Email" value={email} onChange={(e) => setEmail(e.target.value)} className="w-full border border-slate-300 rounded px-3 py-2 text-sm" />
        <input required type="password" placeholder="Password" value={password} onChange={(e) => setPassword(e.target.value)} className="w-full border border-slate-300 rounded px-3 py-2 text-sm" />
        <label className="flex items-center gap-2 text-sm text-slate-700">
          <input type="checkbox" checked={isSuperAdmin} onChange={(e) => setIsSuperAdmin(e.target.checked)} />
          Superadmin (bisa kelola semua workspace)
        </label>
        <button type="submit" className="bg-slate-900 text-white rounded px-4 py-2 text-sm font-medium">
          Buat user
        </button>
      </form>
    </div>
  )
}

function SitesTab() {
  const [sites, setSites] = useState<Site[]>([])
  const [error, setError] = useState<string | null>(null)
  const [siteName, setSiteName] = useState('')
  const [newAgentToken, setNewAgentToken] = useState<{ site: string; token: string } | null>(null)
  const [expandedSite, setExpandedSite] = useState<string | null>(null)

  async function load() {
    try {
      setSites(await api.listSites())
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal memuat sites')
    }
  }

  useEffect(() => {
    load()
  }, [])

  async function onCreateSite(e: React.FormEvent) {
    e.preventDefault()
    try {
      const res = await api.createSite(siteName)
      setNewAgentToken({ site: res.site.name, token: res.agent_token })
      setSiteName('')
      await load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal membuat site')
    }
  }

  return (
    <div className="space-y-4">
      {error && <p className="text-sm text-red-600">{error}</p>}

      {newAgentToken && (
        <div className="bg-amber-50 border border-amber-200 rounded-lg p-4 text-sm space-y-1">
          <p className="font-medium text-amber-800">
            Agent token untuk "{newAgentToken.site}" (simpan sekarang, tidak ditampilkan lagi):
          </p>
          <code className="block bg-white border border-amber-200 rounded px-2 py-1 break-all">{newAgentToken.token}</code>
          <p className="text-amber-700">Set sebagai env AGENT_TOKEN di edge agent lokasi ini.</p>
        </div>
      )}

      <div className="bg-white border border-slate-200 rounded-lg divide-y divide-slate-100">
        {sites.map((s) => (
          <div key={s.id}>
            <div className="p-4 flex items-center justify-between">
              <div>
                <div className="font-medium text-slate-900">{s.name}</div>
                <div className="text-xs text-slate-500">
                  {s.last_seen_at ? `Terakhir online: ${new Date(s.last_seen_at).toLocaleString('id-ID')}` : 'Belum pernah terhubung'}
                </div>
              </div>
              <div className="flex items-center gap-3">
                <button onClick={() => setExpandedSite(expandedSite === s.id ? null : s.id)} className="text-xs text-slate-600">
                  {expandedSite === s.id ? 'Tutup' : 'Kelola kamera'}
                </button>
                <button
                  onClick={async () => {
                    const res = await api.regenerateSiteToken(s.id)
                    setNewAgentToken({ site: s.name, token: res.agent_token })
                  }}
                  className="text-xs text-slate-600"
                >
                  Reset token
                </button>
                <button
                  onClick={async () => {
                    await api.deleteSite(s.id)
                    await load()
                  }}
                  className="text-xs text-red-600"
                >
                  Hapus
                </button>
              </div>
            </div>
            {expandedSite === s.id && <SiteCameras siteId={s.id} />}
          </div>
        ))}
        {sites.length === 0 && <p className="p-4 text-sm text-slate-500">Belum ada site.</p>}
      </div>

      <form onSubmit={onCreateSite} className="bg-white border border-slate-200 rounded-lg p-4 max-w-sm space-y-2">
        <h2 className="font-medium text-slate-900 text-sm">Tambah site (gedung/lokasi)</h2>
        <div className="flex gap-2">
          <input
            required
            placeholder="Nama site (mis. Gedung A)"
            value={siteName}
            onChange={(e) => setSiteName(e.target.value)}
            className="flex-1 border border-slate-300 rounded px-3 py-2 text-sm"
          />
          <button type="submit" className="bg-slate-900 text-white rounded px-4 py-2 text-sm font-medium">
            Tambah
          </button>
        </div>
      </form>
    </div>
  )
}

// A site counts as "online" if its edge agent has heartbeat-ed recently.
// Default agent poll interval is 30s, so 3x that gives slack for a missed
// beat without flickering to "offline" on every jittery cycle.
const SITE_ONLINE_THRESHOLD_MS = 90_000

function HealthTab() {
  const [sites, setSites] = useState<Site[]>([])
  const [cameras, setCameras] = useState<(Camera & { site_name: string })[]>([])
  const [error, setError] = useState<string | null>(null)

  async function load() {
    try {
      const [s, c] = await Promise.all([api.listSites(), api.listAllCameras()])
      setSites(s)
      setCameras(c)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal memuat status')
    }
  }

  useEffect(() => {
    load()
    const interval = setInterval(load, 30_000)
    return () => clearInterval(interval)
  }, [])

  const sitesOnline = sites.filter((s) => isSiteOnline(s)).length
  const camerasByStatus = {
    online: cameras.filter((c) => c.status === 'online').length,
    offline: cameras.filter((c) => c.status === 'offline').length,
    unknown: cameras.filter((c) => c.status === 'unknown').length,
  }

  return (
    <div className="space-y-4">
      {error && <p className="text-sm text-red-600">{error}</p>}

      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <StatCard label="Site online" value={`${sitesOnline} / ${sites.length}`} />
        <StatCard label="Kamera online" value={String(camerasByStatus.online)} tone="green" />
        <StatCard label="Kamera offline" value={String(camerasByStatus.offline)} tone="red" />
        <StatCard label="Status belum diketahui" value={String(camerasByStatus.unknown)} />
      </div>

      <div className="bg-white border border-slate-200 rounded-lg divide-y divide-slate-100">
        {sites.map((s) => {
          const online = isSiteOnline(s)
          const siteCameras = cameras.filter((c) => c.site_id === s.id)
          return (
            <div key={s.id} className="p-4 flex items-center justify-between">
              <div>
                <div className="font-medium text-slate-900">{s.name}</div>
                <div className="text-xs text-slate-500">
                  {s.last_seen_at
                    ? `Heartbeat terakhir: ${new Date(s.last_seen_at).toLocaleString('id-ID')}`
                    : 'Belum pernah terhubung'}
                  {' · '}
                  {siteCameras.length} kamera
                </div>
              </div>
              <span
                className={`text-xs px-2 py-0.5 rounded-full ${online ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-700'}`}
              >
                {online ? 'online' : 'offline'}
              </span>
            </div>
          )
        })}
        {sites.length === 0 && <p className="p-4 text-sm text-slate-500">Belum ada site.</p>}
      </div>
    </div>
  )
}

function isSiteOnline(site: Site) {
  if (!site.last_seen_at) return false
  return Date.now() - new Date(site.last_seen_at).getTime() < SITE_ONLINE_THRESHOLD_MS
}

function StatCard({ label, value, tone }: { label: string; value: string; tone?: 'green' | 'red' }) {
  const color = tone === 'green' ? 'text-green-700' : tone === 'red' ? 'text-red-700' : 'text-slate-900'
  return (
    <div className="bg-white border border-slate-200 rounded-lg p-4">
      <div className={`text-2xl font-semibold ${color}`}>{value}</div>
      <div className="text-xs text-slate-500 mt-1">{label}</div>
    </div>
  )
}

function AuditTab() {
  const [entries, setEntries] = useState<AuditLogEntry[]>([])
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api.listAuditLog().then(setEntries).catch((err) => setError(err instanceof ApiError ? err.message : 'Gagal memuat audit log'))
  }, [])

  return (
    <div className="space-y-4">
      {error && <p className="text-sm text-red-600">{error}</p>}
      <div className="bg-white border border-slate-200 rounded-lg divide-y divide-slate-100">
        {entries.map((e) => (
          <div key={e.id} className="p-3 text-sm flex items-start justify-between gap-4">
            <div>
              <span className="font-medium text-slate-900">{e.actor_email || 'system'}</span>{' '}
              <span className="text-slate-600">{e.action}</span>{' '}
              <span className="text-slate-400">
                {e.target_type}
                {e.target_id ? `:${e.target_id.slice(0, 8)}` : ''}
              </span>
              {e.detail && <span className="text-slate-500"> — {e.detail}</span>}
            </div>
            <span className="text-xs text-slate-400 whitespace-nowrap">{new Date(e.created_at).toLocaleString('id-ID')}</span>
          </div>
        ))}
        {entries.length === 0 && <p className="p-4 text-sm text-slate-500">Belum ada aktivitas tercatat.</p>}
      </div>
    </div>
  )
}

function SiteCameras({ siteId }: { siteId: string }) {
  const [cameras, setCameras] = useState<Camera[]>([])
  const [error, setError] = useState<string | null>(null)
  const [name, setName] = useState('')
  const [serial, setSerial] = useState('')
  const [verCode, setVerCode] = useState('')
  const [rtspUrl, setRtspUrl] = useState('')

  async function load() {
    try {
      setCameras(await api.listSiteCameras(siteId))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal memuat kamera')
    }
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [siteId])

  async function onCreate(e: React.FormEvent) {
    e.preventDefault()
    try {
      await api.createCamera(siteId, { name, ezviz_serial: serial, ezviz_verification_code: verCode, local_rtsp_url: rtspUrl })
      setName('')
      setSerial('')
      setVerCode('')
      setRtspUrl('')
      await load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal membuat kamera')
    }
  }

  return (
    <div className="px-4 pb-4 bg-slate-50 border-t border-slate-100 space-y-3">
      {error && <p className="text-sm text-red-600 pt-3">{error}</p>}

      <div className="divide-y divide-slate-200">
        {cameras.map((cam) => (
          <div key={cam.id} className="py-2 flex items-center justify-between text-sm">
            <div>
              <span className="font-medium text-slate-900">{cam.name}</span>{' '}
              <span className="text-slate-400 font-mono text-xs">{cam.id}</span>
            </div>
            <button
              onClick={async () => {
                await api.deleteCamera(cam.id)
                await load()
              }}
              className="text-xs text-red-600"
            >
              Hapus
            </button>
          </div>
        ))}
        {cameras.length === 0 && <p className="py-2 text-sm text-slate-500">Belum ada kamera di site ini.</p>}
      </div>

      <form onSubmit={onCreate} className="grid grid-cols-2 gap-2 pt-2">
        <input required placeholder="Nama kamera" value={name} onChange={(e) => setName(e.target.value)} className="border border-slate-300 rounded px-2 py-1.5 text-sm" />
        <input required placeholder="EZVIZ serial" value={serial} onChange={(e) => setSerial(e.target.value)} className="border border-slate-300 rounded px-2 py-1.5 text-sm" />
        <input placeholder="Verification code" value={verCode} onChange={(e) => setVerCode(e.target.value)} className="border border-slate-300 rounded px-2 py-1.5 text-sm" />
        <input placeholder="rtsp://... (opsional, lokal)" value={rtspUrl} onChange={(e) => setRtspUrl(e.target.value)} className="border border-slate-300 rounded px-2 py-1.5 text-sm" />
        <button type="submit" className="col-span-2 bg-slate-900 text-white rounded px-4 py-1.5 text-sm font-medium">
          Tambah kamera
        </button>
      </form>
    </div>
  )
}
