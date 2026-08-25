const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080'

export type WorkspaceRole = 'admin' | 'viewer'
export type StorageType = 's3' | 'minio' | 'gdrive'

export interface User {
  id: string
  email: string
  name: string
  is_superadmin: boolean
  created_at: string
}

export interface Workspace {
  id: string
  name: string
  slug: string
  created_at: string
}

export interface Membership {
  user_id: string
  workspace_id: string
  role: WorkspaceRole
  User?: User
}

export interface Site {
  id: string
  name: string
  last_seen_at: string | null
  created_at: string
}

export interface Camera {
  id: string
  site_id: string
  site_name?: string
  // Whether this camera's own site's edge agent is currently reachable
  // (heartbeat within the last ~90s) — only present on endpoints that join
  // site data (ListWorkspaceCameras, ListAllCameras). When false, `status`
  // below is stale (the agent that would update it has gone dark), so
  // display code should show "tidak diketahui" instead of trusting it.
  site_online?: boolean
  name: string
  ezviz_serial: string
  local_rtsp_url: string
  local_rtsp_url_sub?: string
  channel_no: number
  status: 'online' | 'offline' | 'unknown'
  recording_storage_target_id: string | null
}

export interface StorageTarget {
  id: string
  workspace_id: string
  name: string
  type: StorageType
  is_default: boolean
  retain_days: number
}

export type NotificationProvider = 'generic' | 'slack' | 'discord' | 'telegram'

export interface NotificationChannel {
  id: string
  workspace_id: string
  name: string
  provider: NotificationProvider
  webhook_url?: string
  telegram_chat_id?: string
  events: string
}

export interface AuditLogEntry {
  id: string
  created_at: string
  actor_email: string
  action: string
  target_type: string
  target_id: string
  workspace_id: string | null
  detail: string
}

export interface Recording {
  id: string
  camera_id: string
  object_key: string
  started_at: string
  ended_at: string | null
  size_bytes: number
  status: string
}

function getTokens() {
  if (typeof window === 'undefined') return null
  const raw = window.localStorage.getItem('nvr_tokens')
  return raw ? (JSON.parse(raw) as { access: string; refresh: string }) : null
}

function setTokens(access: string, refresh: string) {
  if (typeof window === 'undefined') return
  window.localStorage.setItem('nvr_tokens', JSON.stringify({ access, refresh }))
}

export function clearTokens() {
  if (typeof window === 'undefined') return
  window.localStorage.removeItem('nvr_tokens')
}

export function isAuthenticated() {
  return !!getTokens()?.access
}

// Exposed for flows that need a plain top-level navigation (window.location)
// instead of a fetch() call — see the Google OAuth "Connect" button.
export function getAccessToken() {
  return getTokens()?.access ?? null
}

export function googleOAuthStartUrl(workspaceId: string, name: string) {
  const token = getAccessToken()
  const params = new URLSearchParams({ name, access_token: token ?? '' })
  return `${API_BASE_URL}/api/workspaces/${workspaceId}/oauth/google/start?${params.toString()}`
}

// A <video src="..."> can't send an Authorization header, so like the OAuth
// start link above, this carries the token as a query param instead.
export function recordingStreamUrl(workspaceId: string, cameraId: string, recordingId: string) {
  const params = new URLSearchParams({ access_token: getAccessToken() ?? '' })
  return `${API_BASE_URL}/api/workspaces/${workspaceId}/cameras/${cameraId}/recordings/${recordingId}/stream?${params.toString()}`
}

class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message)
  }
}

async function request<T>(
  path: string,
  options: RequestInit = {},
  retry = true,
): Promise<T> {
  const tokens = getTokens()
  const headers = new Headers(options.headers)
  headers.set('Content-Type', 'application/json')
  if (tokens?.access) headers.set('Authorization', `Bearer ${tokens.access}`)

  const res = await fetch(`${API_BASE_URL}${path}`, { ...options, headers })

  if (res.status === 401 && retry && tokens?.refresh) {
    const refreshed = await tryRefresh(tokens.refresh)
    if (refreshed) return request<T>(path, options, false)
    clearTokens()
  }

  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    throw new ApiError(res.status, body.error ?? res.statusText)
  }
  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

async function tryRefresh(refreshToken: string): Promise<boolean> {
  const res = await fetch(`${API_BASE_URL}/api/auth/refresh`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ refresh_token: refreshToken }),
  })
  if (!res.ok) return false
  const data = await res.json()
  setTokens(data.access_token, data.refresh_token)
  return true
}

export const api = {
  async login(email: string, password: string) {
    const res = await fetch(`${API_BASE_URL}/api/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password }),
    })
    if (!res.ok) {
      const body = await res.json().catch(() => ({ error: 'login failed' }))
      throw new ApiError(res.status, body.error ?? 'login failed')
    }
    const data = await res.json()
    setTokens(data.access_token, data.refresh_token)
    return data.user as User
  },

  logout() {
    const tokens = getTokens()
    clearTokens()
    if (tokens?.refresh) {
      fetch(`${API_BASE_URL}/api/auth/logout`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token: tokens.refresh }),
      }).catch(() => {})
    }
  },

  me: () => request<User & { Memberships: Membership[] }>('/api/me'),

  // workspaces
  listWorkspaces: () => request<Workspace[]>('/api/workspaces'),
  getWorkspace: (id: string) =>
    request<{ workspace: Workspace; members: Membership[] }>(`/api/workspaces/${id}/`),
  createWorkspace: (name: string, slug?: string) =>
    request<Workspace>('/api/workspaces', { method: 'POST', body: JSON.stringify({ name, slug }) }),
  deleteWorkspace: (id: string) => request<void>(`/api/workspaces/${id}`, { method: 'DELETE' }),
  setMembership: (workspaceId: string, userId: string, role: WorkspaceRole) =>
    request<void>(`/api/workspaces/${workspaceId}/members/${userId}`, {
      method: 'PUT',
      body: JSON.stringify({ role }),
    }),
  removeMembership: (workspaceId: string, userId: string) =>
    request<void>(`/api/workspaces/${workspaceId}/members/${userId}`, { method: 'DELETE' }),

  // cameras
  listWorkspaceCameras: (workspaceId: string) =>
    request<Camera[]>(`/api/workspaces/${workspaceId}/cameras`),
  listSiteCameras: (siteId: string) => request<Camera[]>(`/api/sites/${siteId}/cameras`),
  listAllCameras: () => request<(Camera & { site_name: string })[]>('/api/cameras'),
  createCamera: (
    siteId: string,
    body: {
      name: string
      ezviz_serial: string
      ezviz_verification_code?: string
      local_rtsp_url?: string
      local_rtsp_url_sub?: string
      channel_no?: number
    },
  ) => request<Camera>(`/api/sites/${siteId}/cameras`, { method: 'POST', body: JSON.stringify(body) }),
  moveCameraToSite: (cameraId: string, siteId: string) =>
    request<void>(`/api/cameras/${cameraId}`, { method: 'PUT', body: JSON.stringify({ site_id: siteId }) }),
  deleteCamera: (id: string) => request<void>(`/api/cameras/${id}`, { method: 'DELETE' }),
  assignCamera: (workspaceId: string, cameraId: string) =>
    request<void>(`/api/workspaces/${workspaceId}/cameras/${cameraId}/assign`, { method: 'POST' }),
  unassignCamera: (workspaceId: string, cameraId: string) =>
    request<void>(`/api/workspaces/${workspaceId}/cameras/${cameraId}/assign`, { method: 'DELETE' }),
  setCameraStorageTarget: (workspaceId: string, cameraId: string, storageTargetId: string) =>
    request<void>(`/api/workspaces/${workspaceId}/cameras/${cameraId}/storage-target`, {
      method: 'PUT',
      body: JSON.stringify({ storage_target_id: storageTargetId }),
    }),
  listCameraRecordings: (workspaceId: string, cameraId: string) =>
    request<Recording[]>(`/api/workspaces/${workspaceId}/cameras/${cameraId}/recordings`),
  deleteRecording: (workspaceId: string, cameraId: string, recordingId: string) =>
    request<void>(`/api/workspaces/${workspaceId}/cameras/${cameraId}/recordings/${recordingId}`, { method: 'DELETE' }),

  // live view
  getLiveConfig: (workspaceId: string) =>
    request<{ hls_base_url: string }>(`/api/workspaces/${workspaceId}/live-config`),

  // storage targets
  listStorageTargets: (workspaceId: string) =>
    request<StorageTarget[]>(`/api/workspaces/${workspaceId}/storage-targets`),
  createStorageTarget: (
    workspaceId: string,
    body: { name: string; type: StorageType; config: Record<string, unknown>; is_default?: boolean; retain_days?: number },
  ) =>
    request<StorageTarget>(`/api/workspaces/${workspaceId}/storage-targets`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  deleteStorageTarget: (id: string) => request<void>(`/api/storage-targets/${id}`, { method: 'DELETE' }),

  // notification channels
  listNotificationChannels: (workspaceId: string) =>
    request<NotificationChannel[]>(`/api/workspaces/${workspaceId}/notification-channels`),
  createNotificationChannel: (
    workspaceId: string,
    body: {
      name: string
      provider: NotificationProvider
      webhook_url?: string
      telegram_bot_token?: string
      telegram_chat_id?: string
      events: string[]
    },
  ) =>
    request<NotificationChannel>(`/api/workspaces/${workspaceId}/notification-channels`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  deleteNotificationChannel: (workspaceId: string, id: string) =>
    request<void>(`/api/workspaces/${workspaceId}/notification-channels/${id}`, { method: 'DELETE' }),

  // superadmin: audit log
  listAuditLog: () => request<AuditLogEntry[]>('/api/audit-log'),

  // superadmin: users
  listUsers: () => request<User[]>('/api/users'),
  createUser: (body: { email: string; password: string; name: string; is_superadmin?: boolean }) =>
    request<User>('/api/users', { method: 'POST', body: JSON.stringify(body) }),
  deleteUser: (id: string) => request<void>(`/api/users/${id}`, { method: 'DELETE' }),

  // superadmin: sites
  listSites: () => request<Site[]>('/api/sites'),
  createSite: (name: string) =>
    request<{ site: Site; agent_token: string }>('/api/sites', { method: 'POST', body: JSON.stringify({ name }) }),
  deleteSite: (id: string) => request<void>(`/api/sites/${id}`, { method: 'DELETE' }),
  regenerateSiteToken: (id: string) =>
    request<{ agent_token: string }>(`/api/sites/${id}/regenerate-token`, { method: 'POST' }),
  generateSitePairingCode: (id: string) =>
    request<{ pairing_code: string; expires_at: string }>(`/api/sites/${id}/pairing-code`, { method: 'POST' }),
}

export { ApiError }
