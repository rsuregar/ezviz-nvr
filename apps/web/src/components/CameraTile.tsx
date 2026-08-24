import { useEffect, useRef, useState } from 'react'
import Hls from 'hls.js'
import { getAccessToken, type Camera } from '#/lib/api'

interface Props {
  camera: Camera
  hlsBaseUrl: string
}

export function CameraTile({ camera, hlsBaseUrl }: Props) {
  const videoRef = useRef<HTMLVideoElement>(null)
  const [state, setState] = useState<'loading' | 'live' | 'offline'>('loading')

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

  return (
    <div className="relative bg-black rounded-lg overflow-hidden aspect-video">
      <video ref={videoRef} autoPlay muted playsInline className="w-full h-full object-contain" />
      <div className="absolute top-0 left-0 right-0 flex items-center justify-between px-2 py-1 bg-gradient-to-b from-black/70 to-transparent">
        <span className="text-xs text-white font-medium truncate">{camera.name}</span>
        {state !== 'live' && (
          <span className="text-xs text-white/70">{state === 'loading' ? 'Menghubungkan…' : 'Offline'}</span>
        )}
      </div>
    </div>
  )
}
