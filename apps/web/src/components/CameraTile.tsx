import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from 'react'
import Hls from 'hls.js'
import { getAccessToken, type Camera } from '#/lib/api'

interface Props {
  camera: Camera
  hlsBaseUrl: string
  // For D-pad/keyboard navigation (TV remotes, arrow keys): the page that
  // renders the grid tracks which tile has "focus" and draws the ring here,
  // since a TV remote has no cursor to hover with.
  focused?: boolean
}

// Lets the parent grid trigger fullscreen on a specific tile — e.g. when
// the focused tile receives an Enter/OK press from a remote — without the
// parent needing to know how fullscreen is implemented.
export interface CameraTileHandle {
  requestFullscreen: () => void
}

export const CameraTile = forwardRef<CameraTileHandle, Props>(function CameraTile({ camera, hlsBaseUrl, focused }, ref) {
  const containerRef = useRef<HTMLDivElement>(null)
  const videoRef = useRef<HTMLVideoElement>(null)
  const [state, setState] = useState<'loading' | 'live' | 'offline'>('loading')
  // Starts muted so the browser's autoplay policy lets it play without a
  // click first; unmuting is itself a user gesture, so it's always allowed.
  const [muted, setMuted] = useState(true)
  const [isFullscreen, setIsFullscreen] = useState(false)

  useEffect(() => {
    const video = videoRef.current
    if (!video) return

    const src = `${hlsBaseUrl}/live/${camera.id}/index.m3u8`
    // MediaMTX validates this per-request against /api/mediamtx/auth: the
    // "password" half is our own JWT, checked against real workspace
    // membership for this camera — no shared live-view secret involved.
    const authHeader = 'Basic ' + btoa(`viewer:${getAccessToken() ?? ''}`)

    setState('loading')

    if (Hls.isSupported()) {
      const hls = new Hls({
        xhrSetup: (xhr) => {
          xhr.setRequestHeader('Authorization', authHeader)
        },
        manifestLoadingMaxRetry: 2,
        levelLoadingMaxRetry: 2,
      })
      hls.on(Hls.Events.MANIFEST_PARSED, () => setState('live'))
      hls.on(Hls.Events.ERROR, (_evt, data) => {
        if (data.fatal) setState('offline')
      })
      hls.loadSource(src)
      hls.attachMedia(video)
      return () => hls.destroy()
    }

    if (video.canPlayType('application/vnd.apple.mpegurl')) {
      // Safari: native HLS, but no way to attach auth headers to segment
      // requests — only works if MediaMTX read auth is disabled/IP-based.
      video.src = src
      video.addEventListener('loadedmetadata', () => setState('live'))
      video.addEventListener('error', () => setState('offline'))
    }
  }, [camera.id, hlsBaseUrl])

  useEffect(() => {
    function onFullscreenChange() {
      setIsFullscreen(document.fullscreenElement === containerRef.current)
    }
    document.addEventListener('fullscreenchange', onFullscreenChange)
    return () => document.removeEventListener('fullscreenchange', onFullscreenChange)
  }, [])

  function toggleFullscreen() {
    if (document.fullscreenElement) {
      document.exitFullscreen()
    } else {
      containerRef.current?.requestFullscreen()
    }
  }

  useImperativeHandle(ref, () => ({ requestFullscreen: toggleFullscreen }))

  function toggleMute(e: React.MouseEvent) {
    e.stopPropagation()
    setMuted((m) => !m)
  }

  return (
    <div
      ref={containerRef}
      onClick={toggleFullscreen}
      className={`relative bg-black rounded-lg overflow-hidden w-full h-full cursor-pointer ${
        focused ? 'ring-2 ring-white ring-offset-2 ring-offset-slate-50' : ''
      }`}
    >
      <video ref={videoRef} autoPlay muted={muted} playsInline className="w-full h-full object-contain" />
      {state !== 'live' && (
        <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
          <span className="text-white/70 text-sm">{state === 'loading' ? 'Menghubungkan…' : 'Kamera sedang offline'}</span>
        </div>
      )}
      {/* Every control lives in the top bar on purpose — cameras commonly
          burn their own OSD timestamp into the bottom-left/right of the
          frame, so the bottom of the tile has to stay clear of our UI. */}
      <div
        className={`absolute top-0 left-0 right-0 flex items-center justify-between gap-2 bg-gradient-to-b from-black/70 to-transparent ${
          isFullscreen ? 'px-4 py-3' : 'px-2 py-1'
        }`}
      >
        <div className="flex items-center gap-2 min-w-0">
          <span className={`text-white font-medium truncate ${isFullscreen ? 'text-base' : 'text-xs'}`}>{camera.name}</span>
          {state === 'live' && (
            <span className="flex items-center gap-1 shrink-0">
              <span className={`relative flex ${isFullscreen ? 'h-2.5 w-2.5' : 'h-1.5 w-1.5'}`}>
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75" />
                <span className="relative inline-flex rounded-full h-full w-full bg-green-500" />
              </span>
              <span className={`text-green-400 font-semibold tracking-wide ${isFullscreen ? 'text-sm' : 'text-[10px]'}`}>LIVE</span>
            </span>
          )}
        </div>
        <div className="flex items-center gap-1 shrink-0">
          <button
            onClick={toggleMute}
            title={muted ? 'Suarakan' : 'Bisukan'}
            className={`text-white/90 hover:text-white hover:bg-white/10 rounded-full transition-colors ${
              isFullscreen ? 'p-3' : 'p-1.5'
            }`}
          >
            {muted ? <MuteIcon size={isFullscreen ? 28 : 18} /> : <UnmuteIcon size={isFullscreen ? 28 : 18} />}
          </button>
        </div>
      </div>
    </div>
  )
})

function MuteIcon({ size = 18 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5" />
      <line x1="23" y1="9" x2="17" y2="15" />
      <line x1="17" y1="9" x2="23" y2="15" />
    </svg>
  )
}

function UnmuteIcon({ size = 18 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5" />
      <path d="M15.54 8.46a5 5 0 0 1 0 7.07" />
      <path d="M19.07 4.93a10 10 0 0 1 0 14.14" />
    </svg>
  )
}
