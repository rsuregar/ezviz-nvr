import { createFileRoute, Link } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { AppShell } from '#/components/AppShell'
import { SearchPicker } from '#/components/SearchPicker'
import {
  api,
  ApiError,
  googleOAuthStartUrl,
  type Camera,
  type Membership,
  type NotificationChannel,
  type StorageTarget,
  type User,
  type Workspace,
} from '#/lib/api'
import { useAuth } from '#/lib/auth'

export const Route = createFileRoute('/workspaces/$workspaceId')({ component: WorkspacePage })

type Tab = 'cameras' | 'storage' | 'members' | 'notifications'

function WorkspacePage() {
  const { workspaceId } = Route.useParams()
  const [tab, setTab] = useState<Tab>('cameras')
  const [workspace, setWorkspace] = useState<Workspace | null>(null)
  const [members, setMembers] = useState<Membership[]>([])
  const [error, setError] = useState<string | null>(null)

  async function loadWorkspace() {
    try {
      const data = await api.getWorkspace(workspaceId)
      setWorkspace(data.workspace)
      setMembers(data.members)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal memuat workspace')
    }
  }

  useEffect(() => {
    loadWorkspace()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workspaceId])

  return (
    <AppShell>
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-semibold text-slate-900">{workspace?.name ?? '…'}</h1>
            <p className="text-sm text-slate-500">{workspace?.slug}</p>
          </div>
          <Link
            to="/workspaces/$workspaceId/live"
            params={{ workspaceId }}
            className="bg-slate-900 text-white rounded px-4 py-2 text-sm font-medium"
          >
            Live View
          </Link>
        </div>

        {error && <p className="text-sm text-red-600">{error}</p>}

        <div className="flex gap-1 border-b border-slate-200">
          {(['cameras', 'storage', 'members', 'notifications'] as Tab[]).map((t) => (
            <button
              key={t}
              onClick={() => setTab(t)}
              className={`px-3 py-2 text-sm font-medium border-b-2 -mb-px ${
                tab === t ? 'border-slate-900 text-slate-900' : 'border-transparent text-slate-500'
              }`}
            >
              {t === 'cameras' ? 'Kamera' : t === 'storage' ? 'Storage' : t === 'members' ? 'Anggota' : 'Notifikasi'}
            </button>
          ))}
        </div>

        {tab === 'cameras' && <CamerasTab workspaceId={workspaceId} />}
        {tab === 'storage' && <StorageTab workspaceId={workspaceId} />}
        {tab === 'members' && <MembersTab workspaceId={workspaceId} members={members} onChange={loadWorkspace} />}
        {tab === 'notifications' && <NotificationsTab workspaceId={workspaceId} />}
      </div>
    </AppShell>
  )
}

function CamerasTab({ workspaceId }: { workspaceId: string }) {
  const { user } = useAuth()
  const [cameras, setCameras] = useState<Camera[]>([])
  const [allCameras, setAllCameras] = useState<(Camera & { site_name: string })[]>([])
  const [storageTargets, setStorageTargets] = useState<StorageTarget[]>([])
  const [error, setError] = useState<string | null>(null)

  async function load() {
    try {
      const [cams, targets] = await Promise.all([
        api.listWorkspaceCameras(workspaceId),
        api.listStorageTargets(workspaceId),
      ])
      setCameras(cams)
      setStorageTargets(targets)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal memuat kamera')
    }
  }

  async function loadAllCameras() {
    if (!user?.is_superadmin) return
    try {
      setAllCameras(await api.listAllCameras())
    } catch {
      // picker just won't have anything to show; the tab itself still works
    }
  }

  useEffect(() => {
    load()
    loadAllCameras()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workspaceId])

  const assignedIds = new Set(cameras.map((c) => c.id))
  const availableCameras = allCameras.filter((c) => !assignedIds.has(c.id))

  async function onAssign(cameraId: string) {
    try {
      await api.assignCamera(workspaceId, cameraId)
      await load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal menambah kamera')
    }
  }

  return (
    <div className="space-y-4">
      {error && <p className="text-sm text-red-600">{error}</p>}

      <div className="bg-white border border-slate-200 rounded-lg divide-y divide-slate-100">
        {cameras.map((cam) => (
          <div key={cam.id} className="p-4 flex items-center justify-between gap-4">
            <div>
              <div className="font-medium text-slate-900">{cam.name}</div>
              <div className="text-sm text-slate-500">
                Serial {cam.ezviz_serial} · Channel {cam.channel_no}
              </div>
            </div>
            <div className="flex items-center gap-3">
              <StatusBadge status={cam.status} />
              <select
                value={cam.recording_storage_target_id ?? ''}
                onChange={async (e) => {
                  try {
                    await api.setCameraStorageTarget(workspaceId, cam.id, e.target.value)
                    await load()
                  } catch (err) {
                    setError(err instanceof ApiError ? err.message : 'Gagal mengubah storage')
                  }
                }}
                className="border border-slate-300 rounded px-2 py-1 text-xs"
              >
                <option value="">Belum ada storage</option>
                {storageTargets.map((t) => (
                  <option key={t.id} value={t.id}>
                    {t.name}
                  </option>
                ))}
              </select>
              <button
                onClick={async () => {
                  await api.unassignCamera(workspaceId, cam.id)
                  await load()
                }}
                className="text-xs text-red-600"
              >
                Lepas
              </button>
            </div>
          </div>
        ))}
        {cameras.length === 0 && <p className="p-4 text-sm text-slate-500">Belum ada kamera di workspace ini.</p>}
      </div>

      {user?.is_superadmin && (
        <div className="bg-white border border-slate-200 rounded-lg p-4 max-w-lg space-y-2">
          <h2 className="font-medium text-slate-900 text-sm">Tambahkan kamera</h2>
          <SearchPicker
            placeholder="Cari nama kamera atau site…"
            options={availableCameras.map((c) => ({ id: c.id, label: c.name, sublabel: `${c.site_name} · ${c.ezviz_serial}` }))}
            onSelect={onAssign}
            emptyText={allCameras.length === 0 ? 'Belum ada kamera terdaftar (buat dulu di tab Admin → Sites)' : 'Semua kamera sudah ada di workspace ini'}
          />
        </div>
      )}
    </div>
  )
}

function StatusBadge({ status }: { status: Camera['status'] }) {
  const color = status === 'online' ? 'bg-green-100 text-green-700' : status === 'offline' ? 'bg-red-100 text-red-700' : 'bg-slate-100 text-slate-600'
  return <span className={`text-xs px-2 py-0.5 rounded-full ${color}`}>{status}</span>
}

function StorageTab({ workspaceId }: { workspaceId: string }) {
  const [targets, setTargets] = useState<StorageTarget[]>([])
  const [error, setError] = useState<string | null>(null)
  const [gdriveName, setGdriveName] = useState('Google Drive')
  const [name, setName] = useState('')
  const [type, setType] = useState<'s3' | 'minio' | 'gdrive'>('minio')
  const [configText, setConfigText] = useState('{\n  "endpoint": "",\n  "access_key": "",\n  "secret_key": "",\n  "bucket": "",\n  "use_ssl": false\n}')

  async function load() {
    try {
      setTargets(await api.listStorageTargets(workspaceId))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal memuat storage target')
    }
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workspaceId])

  async function onCreate(e: React.FormEvent) {
    e.preventDefault()
    try {
      const config = JSON.parse(configText)
      await api.createStorageTarget(workspaceId, { name, type, config })
      setName('')
      await load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : err instanceof SyntaxError ? 'Config bukan JSON valid' : 'Gagal membuat storage target')
    }
  }

  const justConnectedGDrive =
    typeof window !== 'undefined' && new URLSearchParams(window.location.search).get('connected') === 'gdrive'

  return (
    <div className="space-y-4">
      {error && <p className="text-sm text-red-600">{error}</p>}
      {justConnectedGDrive && (
        <p className="text-sm text-green-700 bg-green-50 border border-green-200 rounded px-3 py-2">
          Google Drive berhasil terhubung.
        </p>
      )}

      <div className="bg-white border border-slate-200 rounded-lg p-4 max-w-lg space-y-2">
        <h2 className="font-medium text-slate-900 text-sm">Hubungkan Google Drive (disarankan)</h2>
        <p className="text-xs text-slate-500">
          Login Google lewat layar consent resmi — token akses tersimpan otomatis, tidak perlu tempel refresh token manual.
        </p>
        <div className="flex gap-2">
          <input
            placeholder="Nama koneksi"
            value={gdriveName}
            onChange={(e) => setGdriveName(e.target.value)}
            className="flex-1 border border-slate-300 rounded px-3 py-2 text-sm"
          />
          <a
            href={googleOAuthStartUrl(workspaceId, gdriveName)}
            className="bg-slate-900 text-white rounded px-4 py-2 text-sm font-medium whitespace-nowrap"
          >
            Hubungkan Google Drive
          </a>
        </div>
      </div>

      <div className="bg-white border border-slate-200 rounded-lg divide-y divide-slate-100">
        {targets.map((t) => (
          <div key={t.id} className="p-4 flex items-center justify-between">
            <div>
              <div className="font-medium text-slate-900">
                {t.name} <span className="text-xs text-slate-400">({t.type})</span>
              </div>
              <div className="text-sm text-slate-500">Retensi {t.retain_days} hari</div>
            </div>
            <button
              onClick={async () => {
                await api.deleteStorageTarget(t.id)
                await load()
              }}
              className="text-xs text-red-600"
            >
              Hapus
            </button>
          </div>
        ))}
        {targets.length === 0 && <p className="p-4 text-sm text-slate-500">Belum ada storage target.</p>}
      </div>

      <form onSubmit={onCreate} className="bg-white border border-slate-200 rounded-lg p-4 max-w-lg space-y-3">
        <h2 className="font-medium text-slate-900 text-sm">Tambah storage target manual (S3/MinIO, atau Google Drive fallback)</h2>
        <input
          required
          placeholder="Nama (mis. MinIO Utama)"
          value={name}
          onChange={(e) => setName(e.target.value)}
          className="w-full border border-slate-300 rounded px-3 py-2 text-sm"
        />
        <select value={type} onChange={(e) => setType(e.target.value as typeof type)} className="w-full border border-slate-300 rounded px-3 py-2 text-sm">
          <option value="minio">MinIO</option>
          <option value="s3">Amazon S3 (S3-compatible)</option>
          <option value="gdrive">Google Drive</option>
        </select>
        <textarea
          value={configText}
          onChange={(e) => setConfigText(e.target.value)}
          rows={6}
          className="w-full border border-slate-300 rounded px-3 py-2 text-xs font-mono"
        />
        <p className="text-xs text-slate-400">
          MinIO/S3: endpoint, access_key, secret_key, bucket, use_ssl. Google Drive: client_id, client_secret, refresh_token,
          folder_id.
        </p>
        <button type="submit" className="bg-slate-900 text-white rounded px-4 py-2 text-sm font-medium">
          Simpan
        </button>
      </form>
    </div>
  )
}

function MembersTab({
  workspaceId,
  members,
  onChange,
}: {
  workspaceId: string
  members: Membership[]
  onChange: () => void
}) {
  const { user } = useAuth()
  const [allUsers, setAllUsers] = useState<User[]>([])
  const [role, setRole] = useState<'admin' | 'viewer'>('viewer')
  const [manualUserId, setManualUserId] = useState('')
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!user?.is_superadmin) return
    api.listUsers().then(setAllUsers).catch(() => {})
  }, [user?.is_superadmin])

  const memberIds = new Set(members.map((m) => m.user_id))
  const availableUsers = allUsers.filter((u) => !memberIds.has(u.id))

  async function addMember(userId: string) {
    try {
      await api.setMembership(workspaceId, userId, role)
      onChange()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal menambah anggota')
    }
  }

  async function onManualAdd(e: React.FormEvent) {
    e.preventDefault()
    await addMember(manualUserId.trim())
    setManualUserId('')
  }

  return (
    <div className="space-y-4">
      {error && <p className="text-sm text-red-600">{error}</p>}

      <div className="bg-white border border-slate-200 rounded-lg divide-y divide-slate-100">
        {members.map((m) => (
          <div key={m.user_id} className="p-4 flex items-center justify-between">
            <div>
              <div className="font-medium text-slate-900">{m.User?.email ?? m.user_id}</div>
              <div className="text-sm text-slate-500">{m.role}</div>
            </div>
            <button
              onClick={async () => {
                await api.removeMembership(workspaceId, m.user_id)
                onChange()
              }}
              className="text-xs text-red-600"
            >
              Keluarkan
            </button>
          </div>
        ))}
        {members.length === 0 && <p className="p-4 text-sm text-slate-500">Belum ada anggota.</p>}
      </div>

      <div className="bg-white border border-slate-200 rounded-lg p-4 max-w-md space-y-2">
        <h2 className="font-medium text-slate-900 text-sm">Tambah anggota</h2>
        <div className="flex gap-2 items-start">
          <div className="flex-1">
            {user?.is_superadmin ? (
              <SearchPicker
                placeholder="Cari nama atau email…"
                options={availableUsers.map((u) => ({ id: u.id, label: u.name || u.email, sublabel: u.email }))}
                onSelect={addMember}
                emptyText="Semua user sudah jadi anggota, atau belum ada user (buat di tab Admin → Users)"
              />
            ) : (
              <form onSubmit={onManualAdd} className="flex gap-2">
                <input
                  required
                  placeholder="User ID (minta ke superadmin)"
                  value={manualUserId}
                  onChange={(e) => setManualUserId(e.target.value)}
                  className="flex-1 border border-slate-300 rounded px-3 py-2 text-sm"
                />
                <button type="submit" className="bg-slate-900 text-white rounded px-3 py-2 text-sm font-medium">
                  Tambah
                </button>
              </form>
            )}
          </div>
          <select
            value={role}
            onChange={(e) => setRole(e.target.value as typeof role)}
            className="border border-slate-300 rounded px-2 py-2 text-sm"
          >
            <option value="viewer">Viewer</option>
            <option value="admin">Admin</option>
          </select>
        </div>
      </div>
    </div>
  )
}

const NOTIFICATION_EVENTS = [
  { id: 'camera_offline', label: 'Kamera offline' },
  { id: 'upload_failed', label: 'Upload gagal' },
] as const

function NotificationsTab({ workspaceId }: { workspaceId: string }) {
  const [channels, setChannels] = useState<NotificationChannel[]>([])
  const [error, setError] = useState<string | null>(null)
  const [name, setName] = useState('')
  const [webhookUrl, setWebhookUrl] = useState('')
  const [events, setEvents] = useState<string[]>(['camera_offline', 'upload_failed'])

  async function load() {
    try {
      setChannels(await api.listNotificationChannels(workspaceId))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal memuat notifikasi')
    }
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workspaceId])

  async function onCreate(e: React.FormEvent) {
    e.preventDefault()
    try {
      await api.createNotificationChannel(workspaceId, { name, webhook_url: webhookUrl, events })
      setName('')
      setWebhookUrl('')
      await load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal membuat notification channel')
    }
  }

  return (
    <div className="space-y-4">
      {error && <p className="text-sm text-red-600">{error}</p>}

      <div className="bg-white border border-slate-200 rounded-lg divide-y divide-slate-100">
        {channels.map((ch) => (
          <div key={ch.id} className="p-4 flex items-center justify-between">
            <div>
              <div className="font-medium text-slate-900">{ch.name}</div>
              <div className="text-sm text-slate-500">{ch.webhook_url}</div>
              <div className="text-xs text-slate-400">{ch.events.split(',').join(', ')}</div>
            </div>
            <button
              onClick={async () => {
                await api.deleteNotificationChannel(workspaceId, ch.id)
                await load()
              }}
              className="text-xs text-red-600"
            >
              Hapus
            </button>
          </div>
        ))}
        {channels.length === 0 && <p className="p-4 text-sm text-slate-500">Belum ada webhook notifikasi.</p>}
      </div>

      <form onSubmit={onCreate} className="bg-white border border-slate-200 rounded-lg p-4 max-w-lg space-y-3">
        <h2 className="font-medium text-slate-900 text-sm">Tambah webhook notifikasi</h2>
        <p className="text-xs text-slate-500">
          Terima notifikasi kamera offline / upload gagal ke Slack, Discord, atau endpoint HTTP kustom apa pun
          (incoming webhook URL).
        </p>
        <input
          required
          placeholder="Nama (mis. Slack #cctv-alerts)"
          value={name}
          onChange={(e) => setName(e.target.value)}
          className="w-full border border-slate-300 rounded px-3 py-2 text-sm"
        />
        <input
          required
          type="url"
          placeholder="https://hooks.slack.com/services/..."
          value={webhookUrl}
          onChange={(e) => setWebhookUrl(e.target.value)}
          className="w-full border border-slate-300 rounded px-3 py-2 text-sm"
        />
        <div className="flex gap-4">
          {NOTIFICATION_EVENTS.map((ev) => (
            <label key={ev.id} className="flex items-center gap-2 text-sm text-slate-700">
              <input
                type="checkbox"
                checked={events.includes(ev.id)}
                onChange={(e) =>
                  setEvents((prev) => (e.target.checked ? [...prev, ev.id] : prev.filter((x) => x !== ev.id)))
                }
              />
              {ev.label}
            </label>
          ))}
        </div>
        <button type="submit" className="bg-slate-900 text-white rounded px-4 py-2 text-sm font-medium">
          Simpan
        </button>
      </form>
    </div>
  )
}
