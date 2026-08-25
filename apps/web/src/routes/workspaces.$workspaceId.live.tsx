import { createFileRoute } from '@tanstack/react-router'
import { useEffect, useMemo, useRef, useState } from 'react'
import { AppShell } from '#/components/AppShell'
import { CameraTile, type CameraTileHandle } from '#/components/CameraTile'
import { api, ApiError, type Camera } from '#/lib/api'

export const Route = createFileRoute('/workspaces/$workspaceId/live')({ component: LivePage })

const GRID_SIZES = [1, 4, 9, 16, 25] as const
type GridSize = (typeof GRID_SIZES)[number]
const GRID_LABELS: Record<GridSize, string> = { 1: '1x1', 4: '2x2', 9: '3x3', 16: '4x4', 25: '5x5' }
const GRID_DIM: Record<GridSize, number> = { 1: 1, 4: 2, 9: 3, 16: 4, 25: 5 }

const SWIPE_THRESHOLD_PX = 50

function LivePage() {
  const { workspaceId } = Route.useParams()
  const [cameras, setCameras] = useState<Camera[]>([])
  const [liveConfig, setLiveConfig] = useState<{ hls_base_url: string } | null>(null)
  const [grid, setGrid] = useState<GridSize>(4)
  const [selectedSite, setSelectedSite] = useState<string | null>(null)
  const [page, setPage] = useState(0)
  const [error, setError] = useState<string | null>(null)
  const [focusedIndex, setFocusedIndex] = useState(0)
  const touchStartX = useRef<number | null>(null)
  const tileRefs = useRef<(CameraTileHandle | null)[]>([])

  useEffect(() => {
    Promise.all([api.listWorkspaceCameras(workspaceId), api.getLiveConfig(workspaceId)])
      .then(([cams, cfg]) => {
        setCameras(cams)
        setLiveConfig(cfg)
      })
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Gagal memuat live view'))
  }, [workspaceId])

  const sites = useMemo(() => {
    const names = new Set(cameras.map((c) => c.site_name).filter((n): n is string => !!n))
    return Array.from(names).sort()
  }, [cameras])

  const filteredCameras = useMemo(
    () => (selectedSite ? cameras.filter((c) => c.site_name === selectedSite) : cameras),
    [cameras, selectedSite],
  )

  const dim = GRID_DIM[grid]
  const totalPages = Math.max(1, Math.ceil(filteredCameras.length / grid))
  const clampedPage = Math.min(page, totalPages - 1)
  const pageCameras = filteredCameras.slice(clampedPage * grid, clampedPage * grid + grid)

  function goToSite(site: string | null) {
    setSelectedSite(site)
    setPage(0)
  }

  function prevPage() {
    setPage((p) => Math.max(0, p - 1))
  }
  function nextPage() {
    setPage((p) => Math.min(totalPages - 1, p + 1))
  }

  // Reset focus whenever the visible tile set changes, so it never points
  // at a tile that no longer exists in this grid/page/site.
  useEffect(() => {
    setFocusedIndex(0)
  }, [clampedPage, grid, selectedSite])

  // D-pad / arrow-key navigation (TV remotes, keyboards): arrows move focus
  // between tiles — wrapping to the next/previous page at the grid's edge —
  // and Enter/Space (a remote's "OK") triggers fullscreen on whichever tile
  // is focused, mirroring how the EZVIZ TV app itself works.
  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (pageCameras.length === 0) return

      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault()
        tileRefs.current[focusedIndex]?.requestFullscreen()
        return
      }

      const row = Math.floor(focusedIndex / dim)
      const col = focusedIndex % dim

      if (e.key === 'ArrowRight') {
        e.preventDefault()
        if (col === dim - 1 || focusedIndex + 1 >= pageCameras.length) {
          if (clampedPage < totalPages - 1) {
            nextPage()
            setFocusedIndex(0)
          }
        } else {
          setFocusedIndex(focusedIndex + 1)
        }
      } else if (e.key === 'ArrowLeft') {
        e.preventDefault()
        if (col === 0) {
          if (clampedPage > 0) {
            prevPage()
            setFocusedIndex(dim - 1)
          }
        } else {
          setFocusedIndex(focusedIndex - 1)
        }
      } else if (e.key === 'ArrowDown') {
        e.preventDefault()
        const next = focusedIndex + dim
        if (next < pageCameras.length) setFocusedIndex(next)
      } else if (e.key === 'ArrowUp') {
        e.preventDefault()
        if (row > 0) setFocusedIndex(focusedIndex - dim)
      }
    }

    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [focusedIndex, dim, pageCameras.length, clampedPage, totalPages])

  function onTouchStart(e: React.TouchEvent) {
    touchStartX.current = e.touches[0].clientX
  }
  function onTouchEnd(e: React.TouchEvent) {
    if (touchStartX.current === null) return
    const delta = e.changedTouches[0].clientX - touchStartX.current
    if (delta > SWIPE_THRESHOLD_PX) prevPage()
    else if (delta < -SWIPE_THRESHOLD_PX) nextPage()
    touchStartX.current = null
  }

  return (
    <AppShell fullWidth>
      <div className="h-full flex gap-2">
        <div className="w-44 shrink-0 bg-white rounded-lg border border-slate-200 overflow-y-auto p-2 space-y-1">
          <button
            onClick={() => goToSite(null)}
            className={`w-full text-left px-3 py-2 rounded text-sm font-medium ${
              selectedSite === null ? 'bg-slate-900 text-white' : 'text-slate-700 hover:bg-slate-100'
            }`}
          >
            Semua Kamera
          </button>
          {sites.map((site) => (
            <button
              key={site}
              onClick={() => goToSite(site)}
              className={`w-full text-left px-3 py-2 rounded text-sm truncate ${
                selectedSite === site ? 'bg-slate-900 text-white' : 'text-slate-700 hover:bg-slate-100'
              }`}
            >
              {site}
            </button>
          ))}
          {sites.length === 0 && <p className="px-3 py-2 text-xs text-slate-400">Belum ada site</p>}
        </div>

        <div className="flex-1 min-w-0 flex flex-col gap-2">
          <div className="flex items-center justify-between">
            <h1 className="text-lg font-semibold text-slate-900">{selectedSite ?? 'Semua Kamera'}</h1>
            <div className="flex gap-1">
              {GRID_SIZES.map((g) => (
                <button
                  key={g}
                  onClick={() => {
                    setGrid(g)
                    setPage(0)
                  }}
                  className={`px-3 py-1.5 text-sm rounded border ${
                    grid === g ? 'bg-slate-900 text-white border-slate-900' : 'bg-white text-slate-600 border-slate-300'
                  }`}
                >
                  {GRID_LABELS[g]}
                </button>
              ))}
            </div>
          </div>

          {error && <p className="text-sm text-red-600">{error}</p>}

          {liveConfig && (
            <div className="flex-1 min-h-0 relative" onTouchStart={onTouchStart} onTouchEnd={onTouchEnd}>
              <div
                className="grid gap-1 h-full"
                style={{ gridTemplateColumns: `repeat(${dim}, minmax(0, 1fr))`, gridTemplateRows: `repeat(${dim}, minmax(0, 1fr))` }}
              >
                {pageCameras.map((cam, i) => (
                  <CameraTile
                    key={cam.id}
                    ref={(el) => {
                      tileRefs.current[i] = el
                    }}
                    camera={cam}
                    hlsBaseUrl={liveConfig.hls_base_url}
                    focused={i === focusedIndex}
                  />
                ))}
                {Array.from({ length: Math.max(0, grid - pageCameras.length) }).map((_, i) => (
                  <div key={`empty-${i}`} className="bg-slate-100 rounded-lg flex items-center justify-center text-slate-400 text-sm">
                    Slot kosong
                  </div>
                ))}
              </div>

              {totalPages > 1 && (
                <>
                  <button
                    onClick={prevPage}
                    disabled={clampedPage === 0}
                    className="absolute left-1 top-1/2 -translate-y-1/2 bg-black/40 hover:bg-black/60 disabled:opacity-30 text-white rounded-full p-2"
                    title="Sebelumnya"
                  >
                    <ChevronLeft />
                  </button>
                  <button
                    onClick={nextPage}
                    disabled={clampedPage === totalPages - 1}
                    className="absolute right-1 top-1/2 -translate-y-1/2 bg-black/40 hover:bg-black/60 disabled:opacity-30 text-white rounded-full p-2"
                    title="Selanjutnya"
                  >
                    <ChevronRight />
                  </button>
                  <div className="absolute bottom-2 left-1/2 -translate-x-1/2 bg-black/50 text-white text-xs px-2 py-1 rounded-full">
                    {clampedPage + 1} / {totalPages} · geser untuk kamera lain
                  </div>
                </>
              )}
            </div>
          )}

          {cameras.length === 0 && !error && <p className="text-sm text-slate-500">Belum ada kamera di workspace ini.</p>}
        </div>
      </div>
    </AppShell>
  )
}

function ChevronLeft() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="15 18 9 12 15 6" />
    </svg>
  )
}

function ChevronRight() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="9 18 15 12 9 6" />
    </svg>
  )
}
