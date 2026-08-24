import { createFileRoute } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { AppShell } from '#/components/AppShell'
import { CameraTile } from '#/components/CameraTile'
import { api, ApiError, type Camera } from '#/lib/api'

export const Route = createFileRoute('/workspaces/$workspaceId/live')({ component: LivePage })

type GridSize = 1 | 4 | 9

function LivePage() {
  const { workspaceId } = Route.useParams()
  const [cameras, setCameras] = useState<Camera[]>([])
  const [liveConfig, setLiveConfig] = useState<{ hls_base_url: string } | null>(null)
  const [grid, setGrid] = useState<GridSize>(4)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    Promise.all([api.listWorkspaceCameras(workspaceId), api.getLiveConfig(workspaceId)])
      .then(([cams, cfg]) => {
        setCameras(cams)
        setLiveConfig(cfg)
      })
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Gagal memuat live view'))
  }, [workspaceId])

  const cols = grid === 1 ? 1 : grid === 4 ? 2 : 3
  const visibleCameras = cameras.slice(0, grid)

  return (
    <AppShell>
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h1 className="text-2xl font-semibold text-slate-900">Live View</h1>
          <div className="flex gap-1">
            {([1, 4, 9] as GridSize[]).map((g) => (
              <button
                key={g}
                onClick={() => setGrid(g)}
                className={`px-3 py-1.5 text-sm rounded border ${
                  grid === g ? 'bg-slate-900 text-white border-slate-900' : 'bg-white text-slate-600 border-slate-300'
                }`}
              >
                {g === 1 ? '1x1' : g === 4 ? '2x2' : '3x3'}
              </button>
            ))}
          </div>
        </div>

        {error && <p className="text-sm text-red-600">{error}</p>}

        {liveConfig && (
          <div
            className="grid gap-3"
            style={{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))` }}
          >
            {visibleCameras.map((cam) => (
              <CameraTile key={cam.id} camera={cam} hlsBaseUrl={liveConfig.hls_base_url} />
            ))}
            {Array.from({ length: Math.max(0, grid - visibleCameras.length) }).map((_, i) => (
              <div key={`empty-${i}`} className="bg-slate-100 rounded-lg aspect-video flex items-center justify-center text-slate-400 text-sm">
                Slot kosong
              </div>
            ))}
          </div>
        )}

        {cameras.length === 0 && !error && (
          <p className="text-sm text-slate-500">Belum ada kamera di workspace ini.</p>
        )}
        {cameras.length > grid && (
          <p className="text-sm text-slate-500">
            Menampilkan {grid} dari {cameras.length} kamera. Ganti tata letak grid untuk melihat lebih banyak.
          </p>
        )}
      </div>
    </AppShell>
  )
}
