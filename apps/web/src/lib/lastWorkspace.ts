// Remembers the workspace the user was last looking at so the header's Live
// View shortcut can jump straight there instead of asking which workspace
// every time — the common case is one workspace (or the same one repeatedly).
const KEY = 'nvr:lastWorkspaceId'

export function getLastWorkspaceId(): string | null {
  return localStorage.getItem(KEY)
}

export function setLastWorkspaceId(id: string) {
  localStorage.setItem(KEY, id)
}
