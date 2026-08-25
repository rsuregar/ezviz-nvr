import { createFileRoute, Link } from '@tanstack/react-router'
import { useEffect, useMemo, useState } from 'react'
import { AppShell } from '#/components/AppShell'
import { api, ApiError, recordingStreamUrl, type Camera, type Recording } from '#/lib/api'

export const Route = createFileRoute('/workspaces/$workspaceId/recordings')({ component: RecordingsPage })

function RecordingsPage() {
  const { workspaceId } = Route.useParams()
  const [cameras, setCameras] = useState<Camera[]>([])
  const [selectedCameraId, setSelectedCameraId] = useState<string>('')
  const [recordings, setRecordings] = useState<Recording[]>([])
  const [selected, setSelected] = useState<Recording | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api
      .listWorkspaceCameras(workspaceId)
      .then((cams) => {
        setCameras(cams)
        if (cams.length > 0) setSelectedCameraId(cams[0].id)
      })
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Gagal memuat kamera'))
  }, [workspaceId])

  function loadRecordings() {
    if (!selectedCameraId) return
    api
      .listCameraRecordings(workspaceId, selectedCameraId)
      .then(setRecordings)
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Gagal memuat rekaman'))
  }

  useEffect(() => {
    setSelected(null)
    loadRecordings()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workspaceId, selectedCameraId])

  async function deleteRecording(rec: Recording) {
    try {
      await api.deleteRecording(workspaceId, selectedCameraId, rec.id)
      if (selected?.id === rec.id) setSelected(null)
      loadRecordings()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal menghapus rekaman')
    }
  }

  const groups = useMemo(() => {
    const byDate = new Map<string, Recording[]>()
    for (const rec of recordings) {
      const date = new Date(rec.started_at).toLocaleDateString('id-ID', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' })
      if (!byDate.has(date)) byDate.set(date, [])
      byDate.get(date)!.push(rec)
    }
    return Array.from(byDate.entries())
  }, [recordings])

  return (
    <AppShell>
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h1 className="text-2xl font-semibold text-slate-900">Rekaman</h1>
          <div className="flex items-center gap-2">
            <select
              value={selectedCameraId}
              onChange={(e) => setSelectedCameraId(e.target.value)}
              className="border border-slate-300 rounded px-3 py-2 text-sm"
            >
              {cameras.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </select>
            <Link
              to="/workspaces/$workspaceId/live"
              params={{ workspaceId }}
              className="bg-slate-900 text-white rounded px-4 py-2 text-sm font-medium whitespace-nowrap"
            >
              Kembali ke Live View
            </Link>
          </div>
        </div>

        {error && <p className="text-sm text-red-600">{error}</p>}

        {selected && (
          <div className="bg-black rounded-lg overflow-hidden">
            <video
              key={selected.id}
              src={recordingStreamUrl(workspaceId, selectedCameraId, selected.id)}
              controls
              autoPlay
              className="w-full max-h-[70vh]"
            />
            <div className="px-3 py-2 text-sm text-white/80 bg-slate-900">
              {new Date(selected.started_at).toLocaleString('id-ID')}
              {' · '}
              {formatSize(selected.size_bytes)}
            </div>
          </div>
        )}

        {cameras.length === 0 && !error && (
          <p className="text-sm text-slate-500">Belum ada kamera di workspace ini.</p>
        )}

        <div className="space-y-4">
          {groups.map(([date, recs]) => (
            <div key={date}>
              <h2 className="text-sm font-medium text-slate-500 mb-2">{date}</h2>
              <div className="bg-white border border-slate-200 rounded-lg divide-y divide-slate-100">
                {recs.map((rec) => (
                  <div
                    key={rec.id}
                    className={`flex items-center justify-between text-sm hover:bg-slate-50 ${selected?.id === rec.id ? 'bg-slate-50' : ''}`}
                  >
                    <button onClick={() => setSelected(rec)} className="flex-1 text-left px-4 py-3">
                      <span className="text-slate-900">
                        {new Date(rec.started_at).toLocaleTimeString('id-ID')}
                        {rec.ended_at && ` – ${new Date(rec.ended_at).toLocaleTimeString('id-ID')}`}
                      </span>
                      <span className="text-slate-400 ml-2">{formatSize(rec.size_bytes)}</span>
                    </button>
                    <button
                      onClick={() => deleteRecording(rec)}
                      className="text-xs text-red-600 px-4"
                      title="Hapus rekaman"
                    >
                      Hapus
                    </button>
                  </div>
                ))}
              </div>
            </div>
          ))}
          {selectedCameraId && recordings.length === 0 && !error && (
            <p className="text-sm text-slate-500">Belum ada rekaman untuk kamera ini.</p>
          )}
        </div>
      </div>
    </AppShell>
  )
}

function formatSize(bytes: number) {
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}
