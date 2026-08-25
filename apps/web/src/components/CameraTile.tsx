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
  const hlsRef = useRef<Hls | null>(null)
  const [state, setState] = useState<'loading' | 'live' | 'offline'>('loading')
  // Starts muted so the browser's autoplay policy lets it play without a
  // click first; unmuting is itself a user gesture, so it's always allowed.
  const [muted, setMuted] = useState(true)
  const [isFullscreen, setIsFullscreen] = useState(false)
  const [isPaused, setIsPaused] = useState(false)
  const [isPiP, setIsPiP] = useState(false)
  // document.pictureInPictureEnabled doesn't exist during SSR, so this has
  // to be read client-side in an effect rather than at render time.
  const [pipSupported, setPipSupported] = useState(false)
  const [resolution, setResolution] = useState<'hd' | 'sd'>('hd')

  useEffect(() => {
    setPipSupported(typeof document !== 'undefined' && document.pictureInPictureEnabled)
  }, [])

  useEffect(() => {
    const video = videoRef.current
    if (!video) return

    const path = resolution === 'sd' && camera.local_rtsp_url_sub ? `live/${camera.id}_sub` : `live/${camera.id}`
    const src = `${hlsBaseUrl}/${path}/index.m3u8`
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
      hlsRef.current = hls
      hls.on(Hls.Events.MANIFEST_PARSED, () => setState('live'))
      hls.on(Hls.Events.ERROR, (_evt, data) => {
        if (data.fatal) setState('offline')
      })
      hls.loadSource(src)
      hls.attachMedia(video)
      return () => {
        hls.destroy()
        hlsRef.current = null
      }
    }

    if (video.canPlayType('application/vnd.apple.mpegurl')) {
      // Safari: native HLS, but no way to attach auth headers to segment
      // requests — only works if MediaMTX read auth is disabled/IP-based.
      video.src = src
      video.addEventListener('loadedmetadata', () => setState('live'))
      video.addEventListener('error', () => setState('offline'))
    }
  }, [camera.id, camera.local_rtsp_url_sub, hlsBaseUrl, resolution])

  useEffect(() => {
    function onFullscreenChange() {
      setIsFullscreen(document.fullscreenElement === containerRef.current)
    }
    document.addEventListener('fullscreenchange', onFullscreenChange)
    return () => document.removeEventListener('fullscreenchange', onFullscreenChange)
  }, [])

  useEffect(() => {
    const video = videoRef.current
    if (!video) return
    const onEnter = () => setIsPiP(true)
    const onLeave = () => setIsPiP(false)
    video.addEventListener('enterpictureinpicture', onEnter)
    video.addEventListener('leavepictureinpicture', onLeave)
    return () => {
      video.removeEventListener('enterpictureinpicture', onEnter)
      video.removeEventListener('leavepictureinpicture', onLeave)
    }
  }, [])

  function toggleFullscreen() {
    if (document.fullscreenElement) {
      document.exitFullscreen()
    } else {
      containerRef.current?.requestFullscreen()
    }
  }

  useImperativeHandle(ref, () => ({ requestFullscreen: toggleFullscreen }))

  // Roving tabindex: the parent grid tracks which tile is "focused" for
  // D-pad/arrow-key purposes, but that was purely a CSS ring — a screen
  // reader or a plain Tab press had no way to reach an individual tile at
  // all, since nothing here ever received real DOM focus. Syncing real
  // focus to whichever tile the grid marks as focused makes Tab order and
  // the AT's notion of "current item" match what's visually highlighted.
  useEffect(() => {
    if (focused) containerRef.current?.focus()
  }, [focused])

  function onContainerKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      toggleFullscreen()
    }
  }

  function toggleMute(e: React.MouseEvent) {
    e.stopPropagation()
    setMuted((m) => !m)
  }

  function togglePause(e: React.MouseEvent) {
    e.stopPropagation()
    const video = videoRef.current
    if (!video) return
    if (video.paused) {
      // Live HLS keeps buffering while paused, so resuming from wherever
      // playback left off would drift further behind live the longer it
      // was paused — jump back to the live edge instead of catching up.
      const live = hlsRef.current?.liveSyncPosition
      if (live) video.currentTime = live
      video.play()
      setIsPaused(false)
    } else {
      video.pause()
      setIsPaused(true)
    }
  }

  function toggleResolution(e: React.MouseEvent) {
    e.stopPropagation()
    setResolution((r) => (r === 'hd' ? 'sd' : 'hd'))
  }

  function togglePiP(e: React.MouseEvent) {
    e.stopPropagation()
    const video = videoRef.current
    if (!video) return
    if (document.pictureInPictureElement) {
      document.exitPictureInPicture()
    } else if (document.pictureInPictureEnabled) {
      video.requestPictureInPicture().catch(() => {})
    }
  }

  function takeSnapshot(e: React.MouseEvent) {
    e.stopPropagation()
    const video = videoRef.current
    if (!video || !video.videoWidth) return
    const canvas = document.createElement('canvas')
    canvas.width = video.videoWidth
    canvas.height = video.videoHeight
    canvas.getContext('2d')?.drawImage(video, 0, 0)
    canvas.toBlob((blob) => {
      if (!blob) return
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `${camera.name}-${new Date().toISOString().replace(/[:.]/g, '-')}.png`
      a.click()
      URL.revokeObjectURL(url)
    }, 'image/png')
  }

  return (
    <div
      ref={containerRef}
      onClick={toggleFullscreen}
      onKeyDown={onContainerKeyDown}
      role="group"
      aria-label={`${camera.name}, ${state === 'live' ? 'sedang live' : state === 'loading' ? 'menghubungkan' : 'kamera offline'}`}
      tabIndex={focused ? 0 : -1}
      className={`relative bg-black rounded-lg overflow-hidden w-full h-full cursor-pointer focus:outline-none ${
        focused ? 'ring-2 ring-white ring-offset-2 ring-offset-slate-50' : ''
      }`}
    >
      <video ref={videoRef} autoPlay muted={muted} playsInline className="w-full h-full object-contain" />
      {state !== 'live' && (
        <div aria-hidden="true" className="absolute inset-0 flex items-center justify-center pointer-events-none">
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
        <div className="flex items-center gap-0.5 shrink-0">
          <button
            onClick={togglePause}
            title={isPaused ? 'Lanjutkan' : 'Jeda'}
            aria-label={isPaused ? 'Lanjutkan' : 'Jeda'}
            className={`text-white/90 hover:text-white hover:bg-white/10 rounded-full transition-colors ${isFullscreen ? 'p-3' : 'p-1.5'}`}
          >
            {isPaused ? <PlayIcon size={isFullscreen ? 28 : 18} /> : <PauseIcon size={isFullscreen ? 28 : 18} />}
          </button>
          <button
            onClick={toggleMute}
            title={muted ? 'Suarakan' : 'Bisukan'}
            aria-label={muted ? 'Suarakan' : 'Bisukan'}
            className={`text-white/90 hover:text-white hover:bg-white/10 rounded-full transition-colors ${isFullscreen ? 'p-3' : 'p-1.5'}`}
          >
            {muted ? <MuteIcon size={isFullscreen ? 28 : 18} /> : <UnmuteIcon size={isFullscreen ? 28 : 18} />}
          </button>
          {camera.local_rtsp_url_sub && (
            <button
              onClick={toggleResolution}
              title="Atur resolusi"
              aria-label={`Atur resolusi, saat ini ${resolution === 'hd' ? 'HD' : 'SD'}`}
              className={`text-white/90 hover:text-white hover:bg-white/10 rounded font-semibold transition-colors ${
                isFullscreen ? 'text-sm px-2.5 py-1.5' : 'text-[10px] px-1.5 py-0.5'
              }`}
            >
              {resolution === 'hd' ? 'HD' : 'SD'}
            </button>
          )}
          {pipSupported && (
            <button
              onClick={togglePiP}
              title={isPiP ? 'Keluar Picture-in-Picture' : 'Picture-in-Picture'}
              aria-label={isPiP ? 'Keluar Picture-in-Picture' : 'Picture-in-Picture'}
              className={`text-white/90 hover:text-white hover:bg-white/10 rounded-full transition-colors ${isFullscreen ? 'p-3' : 'p-1.5'}`}
            >
              <PiPIcon size={isFullscreen ? 28 : 18} />
            </button>
          )}
          <button
            onClick={takeSnapshot}
            title="Ambil snapshot"
            aria-label="Ambil snapshot"
            className={`text-white/90 hover:text-white hover:bg-white/10 rounded-full transition-colors ${isFullscreen ? 'p-3' : 'p-1.5'}`}
          >
            <CameraIcon size={isFullscreen ? 28 : 18} />
          </button>
        </div>
      </div>
    </div>
  )
})

function MuteIcon({ size = 18 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5" />
      <line x1="23" y1="9" x2="17" y2="15" />
      <line x1="17" y1="9" x2="23" y2="15" />
    </svg>
  )
}

function UnmuteIcon({ size = 18 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5" />
      <path d="M15.54 8.46a5 5 0 0 1 0 7.07" />
      <path d="M19.07 4.93a10 10 0 0 1 0 14.14" />
    </svg>
  )
}

function PauseIcon({ size = 18 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <rect x="6" y="4" width="4" height="16" />
      <rect x="14" y="4" width="4" height="16" />
    </svg>
  )
}

function PlayIcon({ size = 18 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <polygon points="5 3 19 12 5 21 5 3" />
    </svg>
  )
}

function PiPIcon({ size = 18 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <rect x="2" y="4" width="20" height="16" rx="2" />
      <rect x="12" y="12" width="8" height="6" rx="1" fill="currentColor" />
    </svg>
  )
}

function CameraIcon({ size = 18 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2z" />
      <circle cx="12" cy="13" r="4" />
    </svg>
  )
}
